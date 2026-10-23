package service

import (
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// Prompt 融合层：把「用户配置」叠到「内置领域 Prompt」之上。
//
// 设计取舍——为什么不直接用用户配置整体替换内置 Prompt：
//
//	内置的 16,172 字国网电费 Prompt 承载了大量领域知识（12 个费用分项的
//	表格结构、峰平谷分时口径、输配容需量/功率因数调整的算法、多表合并规则），
//	一旦整体替换，用户只改了 3 个字段的规则就会把整份多表解析能力冲掉，
//	表现为"配了反而提不出来"。
//
// 所以采用三段式：
//
//	基底   = 用户高级模式模板（若非空）／内置常量
//	补充   = 用户配置了 rule 的字段指令（只追加，不替换）
//	收口   = 输出格式约束（自定义字段必须落在 records[] 的顶层或 fields 里）
//
// 这样"改一个字段"只影响那一个字段，领域精度不受损。

// utilityBillBuiltinPrompt 暴露内置 Prompt 供迁移脚本与单测读取（避免复制粘贴两份文本）。
func utilityBillBuiltinPrompt() string { return utilityBillExtractionSystemPrompt }

// solarBillBuiltinPrompt 同上（光伏）。
func solarBillBuiltinPrompt() string { return solarBillExtractionSystemPrompt }

// UtilityBillDefaultPromptTemplate 电费账单的默认高级模式模板（1.6 万字国网 Prompt）。
// 迁移时写入 knowledge_extract_configs.prompt_template，让用户在「高级模式」里
// 能直接看到并微调这份生产级提示词。
func UtilityBillDefaultPromptTemplate() string { return utilityBillBuiltinPrompt() }

// SolarBillDefaultPromptTemplate 光伏账单的默认高级模式模板。
func SolarBillDefaultPromptTemplate() string { return solarBillBuiltinPrompt() }

// BuildUtilityBillSystemPrompt 组装电费账单最终下发给模型的 system prompt。
//
// cfg == nil → 原样返回内置常量（**与改造前逐字节一致**，这是兼容性的硬要求）。
func BuildUtilityBillSystemPrompt(cfg *types.KbExtractConfig) string {
	return buildBillSystemPrompt(cfg, utilityBillExtractionSystemPrompt, utilityBillOutputContract)
}

// BuildSolarBillSystemPrompt 组装光伏账单最终下发的 system prompt（同上语义）。
func BuildSolarBillSystemPrompt(cfg *types.KbExtractConfig) string {
	return buildBillSystemPrompt(cfg, solarBillExtractionSystemPrompt, solarBillOutputContract)
}

// utilityBillOutputContract 电费账单自定义字段的输出收口指令。
const utilityBillOutputContract = `【自定义字段输出要求】
上述字段说明之外，若模板中还要求了其它字段（用户自定义扩展字段），
必须放在 records[] 每个对象的 "fields" 子对象里输出，例如：
  "fields": {"保证金": "0", "用电地址": "XX路1号"}
不要因为这些字段不在示例 JSON 里就忽略它们；提取不到时留空字符串，严禁编造。`

// solarBillOutputContract 光伏账单自定义字段的输出收口指令。
const solarBillOutputContract = `【自定义字段输出要求】
上述字段说明之外，若模板中还要求了其它字段（用户自定义扩展字段），
必须放在 records[] 每个对象的 "fields" 子对象里输出，例如：
  "fields": {"补贴金额": "1234.56"}
不要因为这些字段不在示例 JSON 里就忽略它们；提取不到时留空字符串，严禁编造。`

// buildBillSystemPrompt 三段式拼装。
func buildBillSystemPrompt(cfg *types.KbExtractConfig, builtin, contract string) string {
	if cfg == nil {
		return builtin
	}
	base := builtin
	// 高级模式自定义模板优先（用户显式接管 Prompt 时不再叠加内置底座，
	// 否则会得到两份互相矛盾的指令）
	if cfg.AdvancedEnabled && strings.TrimSpace(cfg.PromptTemplate) != "" {
		base = cfg.PromptTemplate
	}
	extra := buildConfiguredFieldRules(cfg)
	if extra == "" {
		return base
	}
	// 自定义字段指令必须放在**最前面**。
	//
	// 实测：把指令追加在底座之后，模型会严格照抄底座末尾那份 35 键的输出示例，
	// 自定义字段一个都不输出（连 fields 键都没有）。底座示例的"示范效应"远强于
	// 后置补充指令。前置 + 明确"覆盖下文示例"才有效。
	var b strings.Builder
	if custom := customFieldNames(cfg); len(custom) > 0 {
		b.WriteString("【最高优先级：必须输出以下自定义字段】\n")
		b.WriteString("在 records[] 每个对象的 \"fields\" 子对象里输出这些键：")
		b.WriteString(strings.Join(custom, "、"))
		b.WriteString("。\n即使下文的 JSON 结构示例中没有它们，也必须输出；提取不到时给空字符串。\n\n")
		// 同时把字段名注入示例本身——模型的「照抄示例」倾向极强，
		// 只给指令不给示例时实测三次都不产出（见 injectCustomFieldsIntoExample 注释）。
		base = injectCustomFieldsIntoExample(base, custom)
	}
	b.WriteString(strings.TrimRight(base, "\n"))
	b.WriteString("\n\n")
	b.WriteString(extra)
	b.WriteString("\n\n")
	b.WriteString(contract)
	return b.String()
}

// injectCustomFieldsIntoExample 把自定义字段塞进底座 Prompt 末尾那份输出示例里。
//
// 为什么必须动示例本身：
//
//	实测三次（追加指令 / 前置最高优先级指令 / 注入输出 schema）都没能让模型
//	产出 fields 键——它会严格照抄示例中的 35 个键，一字不多。示例的示范效应
//	强于任何指令，所以唯一稳的做法是让示例里就有这个键。
//
// 注入点选在示例最后一条记录的 `"remark":""` 前（内置 Prompt 的固定结尾），
// 失败则原样返回（宁可不注入，也不能把 Prompt 弄坏）。
func injectCustomFieldsIntoExample(base string, names []string) string {
	if len(names) == 0 {
		return base
	}
	const anchor = `"remark":""`
	at := strings.LastIndex(base, anchor)
	if at < 0 {
		return base
	}
	var b strings.Builder
	b.WriteString(`"fields":{`)
	for i, n := range names {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"`)
		b.WriteString(n)
		b.WriteString(`":""`)
	}
	b.WriteString(`},"remark":""`)
	return base[:at] + b.String() + base[at+len(anchor):]
}

// customFieldNames 列出配置里「不在固定结构体中」的启用字段名。
func customFieldNames(cfg *types.KbExtractConfig) []string {
	if cfg == nil {
		return nil
	}
	std := utilityBillStandardKeys()
	var out []string
	seen := map[string]bool{}
	for _, f := range cfg.Fields {
		if !f.Enabled {
			continue
		}
		// 与 buildConfiguredFieldRules 同口径：只算「配了 rule 或 desc」的字段。
		// 否则会给一个没有任何提取说明的字段强加 required，模型只能编造。
		if strings.TrimSpace(f.Rule) == "" && strings.TrimSpace(f.Desc) == "" {
			continue
		}
		n := strings.TrimSpace(f.Name)
		if n == "" || std[n] || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// buildConfiguredFieldRules 把用户在「提取规则」里配置的字段指令渲染成补充段落。
//
// 只输出**有 rule 或 desc** 的启用字段——没配规则的字段由内置 Prompt 覆盖，
// 重复罗列只会拉长 Prompt、稀释注意力。
//
// 动态字段（不在这套固定结构体里的）额外标注「自定义字段」并提示落到 fields，
// 因为模型对未知键的处置习惯不一致：有的放顶层、有的直接丢。
func buildConfiguredFieldRules(cfg *types.KbExtractConfig) string {
	if cfg == nil || len(cfg.Fields) == 0 {
		return ""
	}
	std := utilityBillStandardKeys()
	var lines []string
	for _, f := range cfg.Fields {
		if !f.Enabled {
			continue
		}
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		rule := strings.TrimSpace(f.Rule)
		desc := strings.TrimSpace(f.Desc)
		if rule == "" && desc == "" {
			continue
		}
		label := strings.TrimSpace(f.Label)
		if label == "" {
			label = name
		}
		// 组装一行：字段名（类型）— 描述；规则
		var line strings.Builder
		line.WriteString("- ")
		line.WriteString(name)
		if label != name {
			line.WriteString("（")
			line.WriteString(label)
			line.WriteString("）")
		}
		if t := strings.TrimSpace(f.Type); t != "" {
			line.WriteString("[")
			line.WriteString(t)
			line.WriteString("]")
		}
		if desc != "" {
			line.WriteString("：")
			line.WriteString(desc)
		}
		if rule != "" {
			line.WriteString(" 提取规则：")
			line.WriteString(rule)
		}
		if !std[name] {
			line.WriteString("  ← 自定义字段，请输出到 fields 子对象")
		}
		lines = append(lines, line.String())
	}
	if len(lines) == 0 {
		return ""
	}
	return "【字段提取规则（用户在设置中配置，优先级高于上文同名说明）】\n" + strings.Join(lines, "\n")
}

// utilityBillStandardKeys 电费账单固定结构体的 JSON 键集合。
// 不在这集合里的键 = 用户自定义字段。
func utilityBillStandardKeys() map[string]bool {
	return structJSONKeys(types.UtilityBillExtractionItem{})
}

// solarBillStandardKeys 光伏账单固定结构体的 JSON 键集合。
func solarBillStandardKeys() map[string]bool {
	return structJSONKeys(types.SolarBillExtractionItem{})
}

// CollectUtilityBillCustomFields 把模型输出里「不在固定结构体中」的字段收进 Fields 动态桶。
//
// obj 是**单条 record** 的原始键值对（由调用方在 records 循环里提供）。
//
// 两个来源合并：
//  1. 顶层未知键（模型习惯把自定义字段直接平铺）；
//  2. 显式 fields 子对象。
//
// 已有值不覆盖（显式 fields 与顶层同名时以先到者为准）。
// 返回 nil 表示没有自定义字段（json omitempty 会因此不输出该键，存量数据形态不变）。
func CollectUtilityBillCustomFields(item *types.UtilityBillExtractionItem, obj map[string]json.RawMessage) {
	if item == nil || len(obj) == 0 {
		return
	}
	item.Fields = collectCustomFields(obj, utilityBillStandardKeys(), item.Fields)
}

// CollectSolarBillCustomFields 光伏版（逻辑同上）。
func CollectSolarBillCustomFields(item *types.SolarBillExtractionItem, obj map[string]json.RawMessage) {
	if item == nil || len(obj) == 0 {
		return
	}
	item.Fields = collectCustomFields(obj, solarBillStandardKeys(), item.Fields)
}

// ---- 输出 Schema 注入 ----
//
// 为什么必须动 Schema，而不是只在 Prompt 里加一句话：
//
//	内置 Prompt 末尾有一份**完整的输出 JSON 示例**（35 个键）。实测模型会严格
//	照抄这份示例——即便后面追加了「请把自定义字段放进 fields」的指令，模型也
//	只输出示例里的键，自定义字段一个都不给（真机验证：record 的键集合与示例
//	逐字一致，完全没有 fields）。
//
//	所以正确做法是把自定义字段**写进输出 schema**，让「必须产出这个键」变成
//	结构约束而不是软提示。字段名来自用户在「字段定义 / 提取规则」里的配置。

// BuildUtilityBillOutputSchema 在内置 schema 基础上注入用户配置的自定义字段。
//
// cfg == nil 或没有自定义字段时原样返回内置常量（行为与改造前一致）。
func BuildUtilityBillOutputSchema(cfg *types.KbExtractConfig) []byte {
	return buildBillOutputSchema(utilityBillExtractionOutputSchema, cfg, utilityBillStandardKeys())
}

// BuildSolarBillOutputSchema 光伏版。
func BuildSolarBillOutputSchema(cfg *types.KbExtractConfig) []byte {
	return buildBillOutputSchema(solarBillExtractionOutputSchema, cfg, solarBillStandardKeys())
}

// buildBillOutputSchema 把 cfg 里「不在固定结构体中」的字段加进 records[].fields 的
// properties，使模型在结构化输出模式下必须承认它们的存在。
func buildBillOutputSchema(base string, cfg *types.KbExtractConfig, std map[string]bool) []byte {
	if cfg == nil || len(cfg.Fields) == 0 {
		return []byte(base)
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(base), &root); err != nil {
		return []byte(base)
	}
	props, _ := root["properties"].(map[string]any)
	if props == nil {
		return []byte(base)
	}
	recs, _ := props["records"].(map[string]any)
	items, _ := recs["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	if itemProps == nil {
		return []byte(base)
	}
	fieldsProp, _ := itemProps["fields"].(map[string]any)
	if fieldsProp == nil {
		fieldsProp = map[string]any{"type": "object"}
		itemProps["fields"] = fieldsProp
	}
	custom := map[string]any{}
	for _, f := range cfg.Fields {
		if !f.Enabled {
			continue
		}
		name := strings.TrimSpace(f.Name)
		if name == "" || std[name] || itemProps[name] != nil {
			continue
		}
		typ := "string"
		if strings.EqualFold(strings.TrimSpace(f.Type), "number") {
			typ = "number"
		}
		custom[name] = map[string]any{"type": typ}
	}
	if len(custom) == 0 {
		return []byte(base)
	}
	fieldsProp["properties"] = custom
	// 把 fields 加进 required：结构化输出模式下，只有 required 里的键模型才必须产出。
	// 仅加 properties 不够——实测模型会直接省略整个 fields 键。
	if req, ok := items["required"].([]any); ok {
		has := false
		for _, r := range req {
			if s, _ := r.(string); s == "fields" {
				has = true
				break
			}
		}
		if !has {
			items["required"] = append(req, "fields")
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return []byte(base)
	}
	return out
}
