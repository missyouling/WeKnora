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

// BuildExtractSystemPrompt 由配置拼装 System Prompt。
// 高级模式（模板非空）→ 模板替换变量插槽；否则 → 结构化默认拼装 + JSON Schema。
// 兼容现有输出结构：{kind, doc_type, vehicle_no, fields}。
func BuildExtractSystemPrompt(cfg *types.KbExtractConfig) string {
	if cfg == nil {
		return ""
	}
	kind := "fleet_vehicle_document"
	switch cfg.Scope {
	case types.FleetCategoryScopeDriver:
		kind = "fleet_driver_document"
	case types.FleetCategoryScopeMaintain:
		kind = "fleet_maintain_document"
	}
	schema := BuildFieldsSchema(cfg.Fields)
	certType := strings.TrimSpace(cfg.CertType)

	// 高级模式：用户模板优先，替换变量后强制追加 JSON 输出兜底约束，
	// 防止用户模板未声明 JSON 输出格式（如仅写字段定义）导致模型输出非 JSON、解析失败。
	if strings.TrimSpace(cfg.PromptTemplate) != "" {
		r := strings.NewReplacer(
			"{{fields_schema}}", schema,
			"{{document_text}}", "<document>\n{{document_text}}\n</document>",
		)
		prompt := strings.TrimSpace(r.Replace(cfg.PromptTemplate))
		var sb strings.Builder
		sb.WriteString(prompt)
		sb.WriteString("\n\n【系统强制输出约束】(该约束优先级高于以上模板，必须无条件遵守)\n")
		sb.WriteString("1. 只能输出一个合法的 JSON 对象，禁止输出 JSON 以外的任何文字、注释、Markdown 代码块围栏或前后缀说明。\n")
		sb.WriteString("2. 输出结构固定为：{\"kind\": \"")
		sb.WriteString(kind)
		sb.WriteString("\", \"doc_type\": \"")
		sb.WriteString(certType)
		sb.WriteString("\", \"vehicle_no\": \"车牌号（没有则空字符串）\", \"fields\": { 字段名: 值, ... }}\n")
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
	sb.WriteString("\",\n  \"vehicle_no\": \"车牌号（如没有则空字符串）\",\n  \"fields\": {\n")
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
