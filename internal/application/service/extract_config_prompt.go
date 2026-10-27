package service

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// EnabledExtractFieldNames 返回配置中启用字段的名称清单（按配置顺序）。
func EnabledExtractFieldNames(fields []types.ExtractFieldConfig) []string {
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		n := strings.TrimSpace(f.Name)
		if n == "" || !f.Enabled {
			continue
		}
		names = append(names, n)
	}
	return names
}

// BuildFieldsSchema 生成 JSON Schema 片段：严格约束输出字段名与类型，
// 避免模型自造字段名或把“地址”类表述输出为 schema 之外的 key。
func BuildFieldsSchema(fields []types.ExtractFieldConfig) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"type\": \"object\",\n  \"properties\": {\n")
	first := true
	for _, f := range fields {
		if !f.Enabled || strings.TrimSpace(f.Name) == "" {
			continue
		}
		if !first {
			sb.WriteString(",\n")
		}
		first = false
		sb.WriteString("    \"" + strings.TrimSpace(f.Name) + "\": {")
		switch f.Type {
		case "array":
			sb.WriteString(`"type": "array", "items": { "type": "string" }`)
		case "number":
			sb.WriteString(`"type": "number"`)
		case "date":
			sb.WriteString(`"type": "string", "format": "date"`)
		default:
			sb.WriteString(`"type": "string"`)
		}
		sb.WriteString("}")
	}
	sb.WriteString("\n  },\n  \"required\": [")
	first = true
	for _, f := range fields {
		if !f.Enabled || strings.TrimSpace(f.Name) == "" {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(`"` + strings.TrimSpace(f.Name) + `"`)
	}
	sb.WriteString("]\n}")
	return sb.String()
}

// EnsureFieldsSchemaSlot 幂等补齐高级模板的 `{{fields_schema}}` 插槽。
//
// 为什么必须做：高级模式下字段规则只通过 `{{fields_schema}}` 注入。存量配置
// （早期版本保存的自定义模板、迁移前 seed 的默认模板）常常没有这个插槽，
// 于是「用户在字段定义里配的一堆规则」在提示词里完全不出现 —— 界面上还会
// 挂一条黄色告警，但提取仍然静默降级。这里在**写入与渲染两侧**都做幂等补齐：
//   · 已含插槽 → 原样返回；
//   · 不含插槽 → 插到「字段与规则 / 字段说明」小节之后；找不到小节就追加一个。
func EnsureFieldsSchemaSlot(tpl string) string {
	if !strings.Contains(tpl, "{{fields_schema}}") {
		// 优先插到既有小节标题后，保持模板的阅读顺序
		for _, anchor := range []string{
			"【字段与规则】",
			"【字段说明】",
			"【字段定义】",
			"【提取字段】",
			"字段说明：",
		} {
			if idx := strings.Index(tpl, anchor); idx >= 0 {
				insertAt := idx + len(anchor)
				return tpl[:insertAt] + "\n{{fields_schema}}\n" + tpl[insertAt:]
			}
		}
		return strings.TrimRight(tpl, "\n") + "\n\n【字段与规则】\n{{fields_schema}}\n"
	}
	return tpl
}

// BuildExtractSystemPrompt 由配置拼装 System Prompt（提取中台的通用入口）。
// 高级模式（模板非空）→ 模板替换变量插槽；否则 → 结构化默认拼装 + JSON Schema。
// 输出结构：{kind, doc_type, fields}。
//
// 本函数不含任何业务概念：kind 由 scope 推导，fields 全部来自配置。
// 车队等需要附加键（如 vehicle_no）的业务，请使用各自的业务实现，
// 不要往这里加硬编码（历史上曾硬编码 fleet_* 与 vehicle_no，已清理）。
func BuildExtractSystemPrompt(cfg *types.KbExtractConfig) string {
	if cfg == nil {
		return ""
	}
	// kind 由 scope 通用推导，不再硬编码 fleet 前缀：
	// 本函数是提取中台的通用入口，不得内含任何具体业务概念。车队业务走自己的
	// 实现（knowledge_fleet_document.go），那里保留 vehicle_no 等车队字段。
	scopeKey := strings.TrimSpace(cfg.Scope)
	if scopeKey == "" {
		scopeKey = "document"
	}
	kind := scopeKey + "_document"
	schema := BuildFieldsSchema(cfg.Fields)
	certType := strings.TrimSpace(cfg.CertType)

	// 高级模式：用户模板优先，替换变量后强制追加 JSON 输出兜底约束，
	// 防止用户模板未声明 JSON 输出格式（如仅写字段定义）导致模型输出非 JSON、解析失败。
	if strings.TrimSpace(cfg.PromptTemplate) != "" {
		// 存量模板缺插槽时先补齐：否则用户配置的字段规则一条都进不了提示词
		tpl := EnsureFieldsSchemaSlot(strings.TrimSpace(cfg.PromptTemplate))
		r := strings.NewReplacer(
			"{{fields_schema}}", schema,
			"{{document_text}}", "<document>\n{{document_text}}\n</document>",
		)
		prompt := strings.TrimSpace(r.Replace(tpl))
		var sb strings.Builder
		sb.WriteString(prompt)
		sb.WriteString("\n\n【系统强制输出约束】(该约束优先级高于以上模板，必须无条件遵守)\n")
		sb.WriteString("1. 只能输出一个合法的 JSON 对象，禁止输出 JSON 以外的任何文字、注释、Markdown 代码块围栏或前后缀说明。\n")
		sb.WriteString("2. 输出结构固定为：{\"kind\": \"")
		sb.WriteString(kind)
		sb.WriteString("\", \"doc_type\": \"")
		sb.WriteString(certType)
		sb.WriteString("\", \"fields\": { 字段名: 值, ... }}\n")
		sb.WriteString("3. fields 必须直接以上述字段名作为键输出键值对，严禁把字段值嵌套进 properties/required/type 等 JSON Schema 结构。\n")
		sb.WriteString("4. doc_type 必须严格等于「")
		sb.WriteString(certType)
		sb.WriteString("」，严禁修改为其它类型。\n")
		sb.WriteString("5. 只输出字段清单中列出的字段：文档中真实出现的填值，未出现的填空字符串（数组填 []、数字填 0）；不要输出未列出的额外字段。\n")
		sb.WriteString("6. 必须严格执行每个字段的「规则」：文档中出现规则列举的同类表述时，统一归入该字段。")
		return sb.String()
	}

	// 默认模式：fields 直接输出键值对骨架（严禁输出 schema 包装 properties/required/type），
	// 避免模型把 JSON Schema 当模板、把值嵌套进 properties 导致解析不到。
	var sb strings.Builder
	sb.WriteString("你是一个档案信息提取助手。本文件证照类型已确定为「")
	sb.WriteString(certType)
	sb.WriteString("」。\n")
	sb.WriteString("输出严格 JSON（不要输出任何其他文字）：\n{\n  \"kind\": \"")
	sb.WriteString(kind)
	sb.WriteString("\",\n  \"doc_type\": \"")
	sb.WriteString(certType)
	sb.WriteString("\",\n  \"fields\": {\n")
	firstField := true
	for _, f := range cfg.Fields {
		if !f.Enabled || strings.TrimSpace(f.Name) == "" {
			continue
		}
		if !firstField {
			sb.WriteString(",\n")
		}
		firstField = false
		sb.WriteString("    \"" + strings.TrimSpace(f.Name) + "\": \"…\"")
	}
	sb.WriteString("\n  }\n}")
	if len(cfg.Fields) > 0 {
		sb.WriteString("\n字段说明：\n")
		for _, f := range cfg.Fields {
			if !f.Enabled || strings.TrimSpace(f.Name) == "" {
				continue
			}
			sb.WriteString("- 字段：「" + strings.TrimSpace(f.Name) + "」")
			if strings.TrimSpace(f.Desc) != "" {
				sb.WriteString("，描述：" + strings.TrimSpace(f.Desc))
			}
			if strings.TrimSpace(f.Rule) != "" {
				sb.WriteString("，规则：" + strings.TrimSpace(f.Rule))
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("注意：doc_type 必须严格等于「")
	sb.WriteString(certType)
	sb.WriteString("」，严禁修改为其它类型。")
	sb.WriteString("fields 必须直接输出上面列出的字段名作为键的键值对，严禁再嵌套 properties/required/type 等 schema 包装结构。")
	sb.WriteString("只提取上面 fields 中列出的字段：文档中真实出现的填值，未出现的内容填空字符串（数组填 []、数字填 0），不要输出未列出的额外字段。")
	sb.WriteString("必须严格执行每个字段的「规则」：文档中出现规则中列举的同类表述时，统一归入该字段。")
	return sb.String()
}
