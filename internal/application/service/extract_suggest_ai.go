package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是「发票设置 → 智能字段生成 / 智能配置规则」的**建议计算层**。
//
// 硬约束（贯穿全文件）：
//  1. **纯建议、绝不落库**。本文件不 import gorm、不持有 repository，
//     也不接受任何"保存"参数。两个接口都只把建议返回给前端，
//     落库必须由用户点「保存」触发既有 SaveExtractConfig / UpdateFleetCategory。
//  2. **不信任模型输出**。模型产出的三类 key 与 sample 全部过后处理：
//     契约键白名单、子列键正则、中文名去重、sample 必须是原文子串。
//     防的是模型最常见的幻觉——把没出现过的值编出来当"样例"。
//  3. **正则三重护栏**（沿用 Phase B/D 边界，见 guardAIRegexPattern）：
//     白名单 → regexp.Compile(RE2) → 样票唯一命中。任何一关不过即丢弃，
//     并把拒绝原因回传前端展示，绝不下发一个"看着能跑实际会误补"的正则。

// ---- 正则护栏的判定结果（同时作为前端展示的 regex_status） ----
//
// 这些字符串同时出现在后端产出与前端 AiRuleSuggestPanel.vue 的展示映射里，
// 改名会让界面显示"未知状态"且不报错——属于静默漂移。
// 改动前请同步 handler/extract_config_ai_test.go 的词表断言。

const (
	// regexStatusAccepted 通过全部护栏，可下发。
	regexStatusAccepted = "accepted"
	// regexRejectNotStandard 非后端补洞白名单字段（自定义扩展 key，
	// 或虽是标准键但补洞器装不下，如 total_amount/tax_rate）。
	regexRejectNotStandard = "rejected_not_standard"
	// regexRejectCompile 未通过 Go RE2 语法编译。
	regexRejectCompile = "rejected_compile"
	// regexRejectNotUnique 在样票文本上无法唯一命中（多值或零命中）。
	regexRejectNotUnique = "rejected_not_unique"
	// regexRejectNoSample 没有样票文本，无法证明唯一命中。
	regexRejectNoSample = "rejected_no_sample"
	// regexRejectTooBroad 形态过于宽泛（如 \d+、.*），误补风险高。
	regexRejectTooBroad = "rejected_too_broad"
	// regexRejectEmpty 模型没给正则。
	regexRejectEmpty = "rejected_empty"
)

// 正则状态码的导出只读访问器（供 handler 层的词表回归测试使用）。
func RegexStatusAccepted() string    { return regexStatusAccepted }
func RegexRejectNotStandard() string { return regexRejectNotStandard }
func RegexRejectCompile() string     { return regexRejectCompile }
func RegexRejectNotUnique() string   { return regexRejectNotUnique }
func RegexRejectNoSample() string    { return regexRejectNoSample }
func RegexRejectTooBroad() string    { return regexRejectTooBroad }
func RegexRejectEmpty() string       { return regexRejectEmpty }

const (
	// maxAIRegexLen 正则长度上限：过长的正则基本是宽泛匹配的表现。
	maxAIRegexLen = 200
	// aiSubFieldKeyMaxLen 明细子列键长度上限（与后端列名存储口径一致）。
	aiSubFieldKeyMaxLen = 32
)

// aiSubFieldKeyRe 明细子列键：必须小写字母开头的小写下划线英文键。
// 依据：子列键会写进 items[].extra 的 JSON key 与前端列定义，
// 允许中文会让模型输出与列名校验两套口径打架。
var aiSubFieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

// broadAIRegexPatterns 形态宽泛、极易误补的表达式。命中即丢弃。
//
// 判定必须**精确锚定整串**：早期版本用 `^\W*\d+\W*$` 这类写法，
// 反而把 `\b\d{20}\b`（正确的全电票号兜底）一并误杀——`\W*` 会吃掉词边界 `\b`。
// 这里逐条拆开，每条只覆盖一种真正的宽泛形态。
var broadAIRegexPatterns = []*regexp.Regexp{
	// 纯字符类量词：\d+ / \w{3,} / \s*（单字符类 + 量词，整串没有别的结构）
	regexp.MustCompile(`^\W*\\?[dDwWsS](\+|\*|\{\d+(,\d*)?\}\+?)\W*$`),
	// 字符组量词：[0-9]+ / [a-z]*（整串就是一个 [...] 加量词）
	regexp.MustCompile(`^\W*\[[^\]]*\][+*]?\W*$`),
	// . 与 .* / .+：任意字符，几乎必然误补
	regexp.MustCompile(`^\W*\.\*?\W*$`),
	// \S+ / \W+ 这类"任意非空白/非词字符"整体量词
	regexp.MustCompile(`^\W*\\[SWsw](\+|\*|\{\d+(,\d*)?\}\+?)\W*$`),
}

// REGEX_PATCHABLE_INVOICE_KEYS 是「后端补洞器真正支持的发票键」的服务端镜像。
// 与 invoiceFieldOrder 严格同源（导出为切片以便单测断言二者一致）。
var REGEX_PATCHABLE_INVOICE_KEYS = invoiceFieldOrder

// guardAIRegexPatchable 护栏一：字段是否在后端补洞白名单内。
// 返回空串代表通过（accepted），否则返回拒绝码。
func guardAIRegexPatchable(field string) string {
	if _, _, ok := invoiceAccessorFor(field); !ok {
		return regexRejectNotStandard
	}
	return ""
}

// narrowEnoughAIRegex 形态检查：拒绝长度超限与典型宽泛表达式。
func narrowEnoughAIRegex(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" || len(p) > maxAIRegexLen {
		return false
	}
	for _, re := range broadAIRegexPatterns {
		if re.MatchString(p) {
			return false
		}
	}
	return true
}

// guardAIRegexPattern 护栏二 + 三：语法编译 + 样票唯一命中。
//
//	pattern 非法 RE2 → rejected_compile
//	形态宽泛       → rejected_too_broad
//	无样票文本     → rejected_no_sample（无法证明唯一命中，保守丢弃）
//	样票上多值/零命中 → rejected_not_unique
//	全过           → ""（accepted）
func guardAIRegexPattern(pattern, sampleText string) string {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return regexRejectEmpty
	}
	if len(p) > maxAIRegexLen {
		return regexRejectTooBroad
	}
	// 顺序要紧：**先编译再判形态**。反过来会把 "不合法语法" 误报成 "太宽泛"
	//（如 `(?=发票)\d{20}`：\W* 能吃掉前缀里的非单词字符，随后被形状规则命中），
	// 前端就会给用户一个驴唇不对马嘴的原因，掩盖真正的语法错误。
	re, err := regexp.Compile(p)
	if err != nil {
		return regexRejectCompile
	}
	for _, re := range broadAIRegexPatterns {
		if re.MatchString(p) {
			return regexRejectTooBroad
		}
	}
	if strings.TrimSpace(sampleText) == "" {
		return regexRejectNoSample
	}
	if _, ok := uniqueRegexCapture(sampleText, re); !ok {
		return regexRejectNotUnique
	}
	return ""
}

// ---- 接口 A：智能字段生成 ----

// InvoiceStandardField 发票标准字段字典条目。
type InvoiceStandardField struct {
	Key      string `json:"key"`       // 契约键（snake_case，与 InvoiceExtractionItem 的 json tag 同源）
	Label    string `json:"label"`     // 中文名
	DataType string `json:"data_type"` // text | number | date
	Hint     string `json:"hint"`      // 取值说明（喂给模型，降低误填）
}

// AISuggestStandardField 标准字段启用建议。
type AISuggestStandardField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"data_type"`
	Enable   bool   `json:"enable"`
	Sample   string `json:"sample"`
}

// AISuggestSubField 明细子列建议。
type AISuggestSubField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"data_type"`
	Width    int    `json:"width"`
}

// AISuggestCustomField 新增自定义字段建议。
type AISuggestCustomField struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	Sample     string `json:"sample"`
	Store      string `json:"store"` // fields（动态桶）| remark（并入备注片段）
	RemarkPart bool   `json:"remark_part"`
}

// AISuggestSkipInfo 一条被丢弃的建议及原因。
// 前端要能看到"我明明勾了却没进去"的原因——静默丢弃比报错更让人困惑。
type AISuggestSkipInfo struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// AISuggestFieldsResult 接口 A 的最终返回（已过后处理）。
type AISuggestFieldsResult struct {
	CertType            string                   `json:"cert_type"`
	SampleText          string                   `json:"sample_text"`
	SampleTextTruncated bool                     `json:"sample_text_truncated"`
	StandardFields      []AISuggestStandardField `json:"standard_fields"`
	ItemSubFields       []AISuggestSubField      `json:"item_sub_fields"`
	CustomFields        []AISuggestCustomField   `json:"custom_fields"`
	Skipped             []AISuggestSkipInfo      `json:"skipped"`
}

// ---- 模型原始输出（宽容解析用的宽松结构） ----

type AISuggestStandardFieldRaw struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"data_type"`
	Enable   bool   `json:"enable"`
	Sample   string `json:"sample"`
}

type AISuggestSubFieldRaw struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"data_type"`
	Width    int    `json:"width"`
}

type AISuggestCustomFieldRaw struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	Sample     string `json:"sample"`
	Store      string `json:"store"`
	RemarkPart bool   `json:"remark_part"`
}

// AISuggestFieldsRaw 模型返回的原始 JSON。
type AISuggestFieldsRaw struct {
	StandardFields []AISuggestStandardFieldRaw `json:"standard_fields"`
	ItemSubFields  []AISuggestSubFieldRaw      `json:"item_sub_fields"`
	CustomFields   []AISuggestCustomFieldRaw   `json:"custom_fields"`
}

// invoiceContractCanonicalKeys 契约键的 canonical 形态集合。
//
// 配置里的字段名可能是 camelCase（totalAmount）而契约键是 snake_case
// （total_amount），直接查会漏判，导致"标准字段但补洞器不支持"被说成
// "自定义扩展字段"——两句文案对用户意味着完全不同的下一步动作。
var invoiceContractCanonicalKeys = func() map[string]bool {
	out := map[string]bool{}
	for k := range invoiceContractKeys() {
		if c := canonicalFieldKey(k); c != "" {
			out[c] = true
		}
	}
	return out
}()

// invoiceContractKeys 反射 InvoiceExtractionItem 的 json tag，得到真正的落库契约键集合。
//
// 用反射而不是手抄常量：手抄清单迟早与结构体漂移，而模型被要求"只许用字典里的键"——
// 字典错了，AI 就会建议一个永远存不进去的字段名。
func invoiceContractKeys() map[string]bool {
	out := map[string]bool{}
	tp := reflect.TypeOf(InvoiceExtractionItem{})
	for i := 0; i < tp.NumField(); i++ {
		tag := tp.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

// InvoiceStandardFieldDictionary 发票标准字段字典（喂给模型的唯一可选键集合）。
//
// 中文名**显式写在这里**，不从 invoiceFieldAliases 反查：那张表是「中文 → 英文」的
// 别名表（收据编号/发票号码/票号…都映射到 invoice_no），反向取"第一条别名"得到的是
// 任意一个别名（可能正是"收据编号"），拿去当标准字段名会误导用户。
func InvoiceStandardFieldDictionary() []InvoiceStandardField {
	keys := invoiceContractKeys()
	type spec struct {
		key   string
		label string
		dtype string
		hint  string
	}
	specs := []spec{
		{"invoice_no", "发票号码", "text", "明确标注『发票号码/发票号/No.』后的数字串；机器编号、校验码、密码区一律不算"},
		{"invoice_code", "发票代码", "text", "发票代码（10~12 位）；全电发票通常无此栏"},
		{"invoice_date", "开票日期", "date", "开票日期，统一为 YYYY-MM-DD"},
		{"invoice_type", "发票类型", "text", "票面标题所示的票据种类"},
		{"total_amount", "价税合计", "number", "价税合计（小数），无货币符号"},
		{"amount", "不含税金额", "number", "不含税金额（小数）"},
		{"tax", "税额", "number", "税额（小数）"},
		{"tax_rate", "税率", "number", "税率（数字，13 表示 13%）"},
		{"seller_name", "销售方名称", "text", "销售方（开票方）名称"},
		{"seller_tax_no", "销售方纳税人识别号", "text", "销售方纳税人识别号/统一社会信用代码"},
		{"buyer_name", "购买方名称", "text", "购买方（受票方）名称"},
		{"buyer_tax_no", "购买方纳税人识别号", "text", "购买方纳税人识别号/统一社会信用代码"},
		{"issuer", "开票人", "text", "开票人/收款人/经办人姓名"},
		{"remark", "备注", "text", "票面备注栏原文"},
		{"category", "票据大类", "text", "票据大类，如增值税专用发票"},
		{"items", "项目明细", "array", "货物/服务明细行"},
	}
	out := make([]InvoiceStandardField, 0, len(specs))
	for _, s := range specs {
		if !keys[s.key] {
			// 结构体改名后立刻暴露，不静默产出一个无效字典
			logger.Warnf(context.Background(),
				"invoice standard dictionary: %q is not a contract key of InvoiceExtractionItem", s.key)
			continue
		}
		out = append(out, InvoiceStandardField{Key: s.key, Label: s.label, DataType: s.dtype, Hint: s.hint})
	}
	return out
}

// buildFieldSuggestPrompt 拼接口 A 的 Prompt。
func buildFieldSuggestPrompt(certType string, dict []InvoiceStandardField) string {
	var sb strings.Builder
	sb.WriteString("你是一个票据字段建模助手。给定一份「")
	sb.WriteString(certType)
	sb.WriteString("」票面的原文，判定该票面上真实存在哪些业务字段，并把能对上的映射到系统标准字段字典。\n\n")
	sb.WriteString("【标准字段字典】（standard_fields 只允许从中挑选，不得自造 key）\n")
	for _, d := range dict {
		sb.WriteString("- ")
		sb.WriteString(d.Key)
		sb.WriteString(" | ")
		sb.WriteString(d.Label)
		sb.WriteString(" | ")
		sb.WriteString(d.DataType)
		sb.WriteString(" | ")
		sb.WriteString(d.Hint)
		sb.WriteString("\n")
	}
	sb.WriteString("\n【输出要求】\n")
	sb.WriteString("1. 严格输出一个 JSON 对象，不要任何前后缀文字、不要 Markdown 围栏。\n")
	sb.WriteString("2. standard_fields：票面**真实出现**该信息的字段 enable=true 并给出 sample；")
	sb.WriteString("sample 必须逐字来自原文，禁止改写或臆造。未出现的 enable=false 且 sample 写空字符串。\n")
	sb.WriteString("3. item_sub_fields：仅在票面存在明细/项目行表格时输出。key 用小写下划线英文")
	sb.WriteString("（name/spec/unit/qty/price/amount/tax_rate/tax_amount 为通用键；额外的用业务语义英文）。\n")
	sb.WriteString("4. custom_fields：票面上有、但字典里没有的信息（如户号、缴费期、抄表读数）在此给出中文名与 sample。")
	sb.WriteString("若该信息没有独立存储位，store 填 remark，否则填 fields。\n")
	sb.WriteString("5. 不要输出字典中不存在、票面上也没出现的字段。\n")
	sb.WriteString("6. 输出结构固定为：")
	sb.WriteString(`{"standard_fields":[{"key":"","label":"","data_type":"","enable":false,"sample":""}],`)
	sb.WriteString(`"item_sub_fields":[{"key":"","label":"","data_type":"","width":0}],`)
	sb.WriteString(`"custom_fields":[{"name":"","data_type":"","sample":"","store":"fields","remark_part":false}]}`)
	sb.WriteString("\n【号码口径】invoice_no 只能取明确标注『发票号码/发票号/No.』标签后的数字串；")
	sb.WriteString("『机器编号』『校验码』『密码区』一律不算。\n")
	return sb.String()
}

// buildFieldSuggestUserPrompt 包样票原文。
func buildFieldSuggestUserPrompt(sampleText string) string {
	return "<document>\n" + sampleText + "\n</document>"
}

// tryParseAISuggestFieldsJSON 宽容解析：代码围栏、思考块、前后缀废话都尝试剥离。
func tryParseAISuggestFieldsJSON(raw string, out *AISuggestFieldsRaw) bool {
	cleaned := strings.NewReplacer("```json", "", "```", "").Replace(raw)
	if err := common.ParseLLMJsonResponse(cleaned, out); err != nil {
		return false
	}
	return out != nil
}

// suggestFieldsFromText 调模型生成字段建议（纯计算，不落库）。
func suggestFieldsFromText(
	ctx context.Context, model chat.Chat, certType, sampleText string, dict []InvoiceStandardField,
) (*AISuggestFieldsResult, error) {
	if strings.TrimSpace(sampleText) == "" {
		return nil, fmt.Errorf("sample text is empty")
	}
	systemPrompt := buildFieldSuggestPrompt(certType, dict)
	userPrompt := buildFieldSuggestUserPrompt(sampleText)
	thinking := false
	llmCtx := types.WithLLMCallMetadata(ctx, "extract_suggest_fields", "invoice")
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	attempts := []ExtractAttempt{
		{Label: "nonstream-jsonobject", Schema: json.RawMessage(`{"type":"json_object"}`)},
		{Label: "stream-free", Stream: true},
		{Label: "nonstream-free"},
	}

	var parsed AISuggestFieldsRaw
	raw, err := RunExtractWithRetry(llmCtx, model, messages, baseOpts, attempts,
		func(raw string) bool { return tryParseAISuggestFieldsJSON(raw, &parsed) },
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "field suggest unusable after retries: %v (len=%d)", lastErr, len(lastRaw))
		})
	if err != nil {
		return nil, err
	}
	if !tryParseAISuggestFieldsJSON(raw, &parsed) {
		return nil, errParseLLMOutput
	}
	return postProcessAISuggestFields(&parsed, sampleText, dict), nil
}

// postProcessAISuggestFields 防幻觉后处理（**不信任模型输出的唯一入口**）：
//
//	standard_fields：key 必须在落库契约键集合内；sample 必须是 sample_text 的真实子串。
//	item_sub_fields：key 必须匹配 ^[a-z][a-z0-9_]{1,31}$；按 key 去重。
//	custom_fields ：中文名去重、排除与标准契约键同名（canonical 归一后）、sample 必须是原文子串。
func postProcessAISuggestFields(
	raw *AISuggestFieldsRaw, sampleText string, dict []InvoiceStandardField,
) *AISuggestFieldsResult {
	out := &AISuggestFieldsResult{
		StandardFields: []AISuggestStandardField{},
		ItemSubFields:  []AISuggestSubField{},
		CustomFields:   []AISuggestCustomField{},
		SampleText:     sampleText,
	}
	if raw == nil {
		return out
	}
	contract := invoiceContractKeys()
	dictByKey := make(map[string]InvoiceStandardField, len(dict))
	for _, d := range dict {
		dictByKey[d.Key] = d
	}
	skipped := []AISuggestSkipInfo{}
	sampleNorm := sampleText

	// ① 标准字段
	seenStd := map[string]bool{}
	for _, f := range raw.StandardFields {
		key := strings.TrimSpace(f.Key)
		if key == "" || seenStd[key] {
			continue
		}
		if !contract[key] {
			// 白名单外的 key 一律丢弃，但要让用户知道为什么——
			// 模型自造 key 往往意味着它没读懂字典，提示出来用户才能自己纠正。
			skipped = append(skipped, AISuggestSkipInfo{
				Name:   key,
				Reason: "不是系统标准字段键，已忽略（如需保留请在扩展字段中填写）",
			})
			continue
		}
		seenStd[key] = true
		label := strings.TrimSpace(f.Label)
		dtype := strings.TrimSpace(f.DataType)
		if d, ok := dictByKey[key]; ok {
			if label == "" {
				label = d.Label
			}
			if dtype == "" {
				dtype = d.DataType
			}
		}
		if label == "" {
			// 模型没给 label 且字典里也没有：退回 key 本身。
			// 不再从 invoiceFieldAliases 反查——那张表反向取"第一条别名"得到的是
			// 任意别名（可能正是"收据编号"），当标准字段名会误导用户。
			label = key
		}
		out.StandardFields = append(out.StandardFields, AISuggestStandardField{
			Key:      key,
			Label:    label,
			DataType: dtype,
			Enable:   f.Enable,
			// 防幻觉：sample 必须能在原文里找到（逐字，非归一化后的模糊匹配）
			Sample: verifiedSample(f.Sample, sampleNorm, f.Enable),
		})
	}

	// ② 明细子列：键形态硬约束 + 去重
	seenSub := map[string]bool{}
	for _, s := range raw.ItemSubFields {
		key := strings.TrimSpace(s.Key)
		if key == "" {
			continue
		}
		// 键形态是硬约束：子列键会写进 items[].extra 的 JSON key 与前端列定义，
		// 放进中文/连字符会让模型输出与列名校验两套口径打架。丢弃时说明原因，
		// 否则用户看到"勾了没反应"无从下手。
		if len(key) > aiSubFieldKeyMaxLen || !aiSubFieldKeyRe.MatchString(key) {
			skipped = append(skipped, AISuggestSkipInfo{
				Name:   key,
				Reason: "子列键需为小写字母开头的小写下划线英文（如 name / qty / account_no）",
			})
			continue
		}
		if seenSub[key] {
			skipped = append(skipped, AISuggestSkipInfo{Name: key, Reason: "与前面建议的明细子列重名"})
			continue
		}
		seenSub[key] = true
		label := strings.TrimSpace(s.Label)
		if label == "" {
			label = key
		}
		dtype := strings.TrimSpace(s.DataType)
		if dtype != "number" && dtype != "date" {
			dtype = "text"
		}
		width := s.Width
		if width < 0 || width > 400 {
			width = 0
		}
		out.ItemSubFields = append(out.ItemSubFields, AISuggestSubField{
			Key: key, Label: label, DataType: dtype, Width: width,
		})
	}

	// ③ 自定义字段：去重 + 排除与标准契约键冲突 + sample 必须是原文子串
	seenCustom := map[string]bool{}
	for _, c := range raw.CustomFields {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		// 与本次已建议的扩展字段去重（canonical 归一，覆盖 invoiceNo / InvoiceNo 之类）
		ck := canonicalFieldKey(name)
		// 用 canonical 集合判冲突：契约键是 snake_case（total_amount），
		// 而自定义名可能是 camelCase（totalAmount）或中文，直接查 contract[name]
		// 会漏判，于是"标准字段但补洞器不支持"被误报为"自定义扩展字段"。
		if ck != "" && invoiceContractCanonicalKeys[ck] {
			// 与标准契约键同名：它应出现在 standard_fields，不该是扩展字段
			skipped = append(skipped, AISuggestSkipInfo{Name: name, Reason: "与系统标准字段同名，应归入标准字段"})
			continue
		}
		if seenCustom[ck] {
			skipped = append(skipped, AISuggestSkipInfo{Name: name, Reason: "与前面建议的扩展字段重名"})
			continue
		}
		seenCustom[ck] = true
		dtype := strings.TrimSpace(c.DataType)
		if dtype != "number" && dtype != "date" {
			dtype = "text"
		}
		store := strings.TrimSpace(c.Store)
		if store != "remark" {
			store = "fields"
		}
		out.CustomFields = append(out.CustomFields, AISuggestCustomField{
			Name:       name,
			DataType:   dtype,
			Sample:     verifiedSample(c.Sample, sampleNorm, true),
			Store:      store,
			RemarkPart: store == "remark" || c.RemarkPart,
		})
	}
	out.Skipped = skipped
	return out
}

// verifiedSample 防幻觉核心：模型给的样值必须在样票原文里**逐字出现**。
// 模型最常见的幻觉是编一个"看起来很像"的号码；不校验就会让用户误以为
// 票面上真有这个值。命中失败一律清空（用户仍能按字段名手工配置）。
func verifiedSample(sample, sampleText string, required bool) string {
	v := strings.TrimSpace(sample)
	if v == "" || !required {
		return ""
	}
	if strings.TrimSpace(sampleText) == "" {
		return ""
	}
	if !strings.Contains(sampleText, v) {
		return ""
	}
	return v
}

// ---- 接口 B：智能配置规则 ----

// AIRuleFieldInput 请求为哪些字段生成规则（字段名权威源仍是「字段定义」Tab 的 subs）。
type AIRuleFieldInput struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// AISuggestRule 单个字段的规则建议（模型原始输出 + 后端补全的护栏结论）。
type AISuggestRule struct {
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	Rule        string `json:"rule"`
	Required    bool   `json:"required"`
	Regex       string `json:"regex"`
	RegexStatus string `json:"regex_status"`
	RegexReason string `json:"regex_reason,omitempty"`
}

// AISuggestRules 模型返回的原始 JSON。
type AISuggestRules struct {
	Fields         []AISuggestRule `json:"fields"`
	PromptTemplate string          `json:"prompt_template"`
}

// AISuggestRuleValues 后端返回给前端的最终字段值（护栏结论已定稿）。
type AISuggestRuleValues struct {
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	Rule        string `json:"rule"`
	Required    bool   `json:"required"`
	Regex       string `json:"regex"`
	RegexStatus string `json:"regex_status"`
	RegexReason string `json:"regex_reason,omitempty"`
}

// AISuggestRulesResult 接口 B 的最终返回。
type AISuggestRulesResult struct {
	CertType string                `json:"cert_type"`
	Fields   []AISuggestRuleValues `json:"fields"`
	// PromptTemplate 全局高级模板建议；模型未给时为空串（前端保持自己已有的）。
	PromptTemplate string `json:"prompt_template"`
	// AppliedFieldCount 本次实际会发生实质变化的字段数（前端用于提示"将更新 N 个字段"）。
	AppliedFieldCount int `json:"applied_field_count"`
	// SampleTextUsed 是否带样票（无样票时正则全部按 rejected_no_sample 丢弃，前端据此提示）。
	SampleTextUsed bool `json:"sample_text_used"`
}

// AIRuleFieldValues 现有字段值（合并时的"现状"侧）。
//
// 注意 RegexStatus/RegexReason 必须随值一起流动：护栏结论如果只留在建议里，
// 合并阶段一丢，前端就只能看到"未配置正则兜底"这种无信息量的文案，
// 白白丢掉"自定义字段不参与补洞""样票上无法唯一命中"这些真正有用的诊断。
type AIRuleFieldValues struct {
	Desc     string
	Rule     string
	Required bool
	Regex    string
	// RegexStatus/RegexReason 是本轮护栏对该字段的结论（仅本轮建议涉及的字段会填）。
	RegexStatus string
	RegexReason string
}

// buildRuleSuggestPrompt 拼接口 B 的 Prompt。
func buildRuleSuggestPrompt(certType string, fields []AIRuleFieldInput, sampleText string) string {
	var sb strings.Builder
	sb.WriteString("你是一个票据提取规则配置助手。为「")
	sb.WriteString(certType)
	sb.WriteString("」的每个字段编写描述、自然语言提取规则、是否必填，并在必要时给出 Go RE2 兜底正则。\n\n")
	sb.WriteString("【字段清单】\n")
	for _, f := range fields {
		sb.WriteString("- ")
		sb.WriteString(strings.TrimSpace(f.Name))
		if strings.TrimSpace(f.DataType) != "" {
			sb.WriteString(" (")
			sb.WriteString(strings.TrimSpace(f.DataType))
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n【desc 要求】一句话说明该字段的业务含义，不超过 20 字。\n")
	sb.WriteString("【rule 要求】\n")
	sb.WriteString("- 中文自然语言，说明从票面哪里取、如何归一（日期统一 YYYY-MM-DD、金额两位小数、不要货币符号）。\n")
	sb.WriteString("- 原文没有时明确写「无则留空」，禁止让模型臆造。\n")
	sb.WriteString("- 不得与本分类其它字段的 rule 完全相同。\n")
	sb.WriteString("【required 要求】仅对「票面必然存在」的核心字段置 true（发票号码/开票日期/价税合计/销售方名称等）；可选字段一律 false。\n")
	sb.WriteString("【regex 要求】\n")
	sb.WriteString("- 仅对下列字段给出：")
	sb.WriteString(strings.Join(REGEX_PATCHABLE_INVOICE_KEYS, ", "))
	sb.WriteString("。其余字段的 regex 一律输出空字符串。\n")
	sb.WriteString("- 必须是 Go RE2 语法：可用字符类、量词、\\b 边界、捕获组；禁止环视 (?=)(?!)、反向引用、递归。\n")
	sb.WriteString("- 必须是「形状唯一」的，能在原文中唯一命中（如 \\b\\d{20}\\b），不要 (?s).* 这类宽泛式。\n")
	sb.WriteString("- 不需要或无法保证唯一时输出空字符串，不要硬凑。\n")
	sb.WriteString("\n【输出结构】")
	sb.WriteString(`{"fields":[{"name":"","desc":"","rule":"","required":false,"regex":""}],"prompt_template":""}`)
	sb.WriteString("\n")
	if strings.TrimSpace(sampleText) != "" {
		sb.WriteString("\n【样票原文】正则会在下述原文中做唯一命中校验，不唯一就会被系统丢弃。\n")
		sb.WriteString("<sample>\n")
		sb.WriteString(sampleText)
		sb.WriteString("\n</sample>\n")
	}
	return sb.String()
}

// tryParseAISuggestRulesJSON 宽容解析接口 B 的模型输出。
func tryParseAISuggestRulesJSON(raw string, out *AISuggestRules) bool {
	cleaned := strings.NewReplacer("```json", "", "```", "").Replace(raw)
	if err := common.ParseLLMJsonResponse(cleaned, out); err != nil {
		return false
	}
	return out != nil
}

// SuggestInvoiceFieldsFromText 调模型生成字段建议（**纯计算，不落库**）。
//
// 唯一的对外入口：handler 只认这一个函数，不接触 prompt / 解析 / 护栏细节。
func SuggestInvoiceFieldsFromText(
	ctx context.Context, model chat.Chat, certType, sampleText string, dict []InvoiceStandardField,
) (*AISuggestFieldsResult, error) {
	return suggestFieldsFromText(ctx, model, certType, sampleText, dict)
}

// SuggestInvoiceRulesForFields 调模型生成规则建议并跑完正则三重护栏（**纯计算，不落库**）。
func SuggestInvoiceRulesForFields(
	ctx context.Context, model chat.Chat, certType string, fields []AIRuleFieldInput, sampleText string,
) (*AISuggestRules, error) {
	return suggestRulesForFields(ctx, model, certType, fields, sampleText)
}

// MergeAIRuleSuggestions 按 overwrite 语义把建议合并进现状，返回 (合并值, 是否有实质变化)。
//
// 与内层 mergeAISuggestRules 的差别：这里多跑一遍正则护栏，并把
// regex_status/regex_reason 一起写进结果值——护栏结论必须跟着值流到前端，
// 否则界面只会显示"未配置正则兜底"，把"自定义字段不参与补洞"这类真正有用的
// 诊断抹平（实测踩过：车牌号明明该报 not_standard，却显示 rejected_empty）。
func MergeAIRuleSuggestions(
	fields []AIRuleFieldInput, suggested []AISuggestRule,
	existing map[string]AIRuleFieldValues, overwrite bool, sampleText string,
) (map[string]AIRuleFieldValues, bool) {
	// 先把护栏结论映射回建议（按 canonical 匹配，容忍模型回 camelCase 变体）
	guarded := make(map[string][2]string, len(suggested)) // canonical -> (status, reason)
	regexByName := make(map[string]string, len(suggested))
	for _, s := range suggested {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		_, status, reason := guardAIRuleRegex(n, s.Regex, sampleText)
		guarded[canonicalFieldKey(n)] = [2]string{status, reason}
		if status == regexStatusAccepted {
			regexByName[n] = strings.TrimSpace(s.Regex)
		}
	}
	merged, changed := mergeAISuggestRules(fields, suggested, existing, overwrite)
	for name, v := range merged {
		if g, ok := guarded[canonicalFieldKey(name)]; ok {
			v.RegexStatus, v.RegexReason = g[0], g[1]
			// 护栏没通过时不要把模型给的宽泛正则留在值里：前端只看 regex 字段，
			// 一个未通过校验的正则混进去就可能被保存进库。
			if g[0] != regexStatusAccepted {
				v.Regex = ""
			} else if re, ok := regexByName[name]; ok {
				v.Regex = re
			}
			merged[name] = v
		}
	}
	return merged, changed
}

// NewAISuggestRulesResult 组装接口 B 的响应：把合并后的字段值按请求顺序排列。
//
// regex_status/regex_reason 取自合并阶段携带的护栏结论（AIRuleFieldValues 里），
// 而不是在这里按"有没有正则"重新猜——后者会把"自定义字段不参与补洞"这类
// 真正有用的诊断抹平成"未配置正则兜底"。
// 兜底仅用于**本轮未被建议覆盖**的字段（沿用已保存的正则）。
func NewAISuggestRulesResult(
	certType string, fields []AIRuleFieldInput, values map[string]AIRuleFieldValues,
) *AISuggestRulesResult {
	out := &AISuggestRulesResult{
		CertType:          certType,
		Fields:            []AISuggestRuleValues{},
		AppliedFieldCount: 0,
	}
	for _, f := range fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			continue
		}
		v := values[name]
		status, reason := v.RegexStatus, v.RegexReason
		if status == "" {
			status, reason = regexStatusOf(v.Regex), regexReasonOf(v.Regex)
		}
		out.Fields = append(out.Fields, AISuggestRuleValues{
			Name:        name,
			Desc:        v.Desc,
			Rule:        v.Rule,
			Required:    v.Required,
			Regex:       v.Regex,
			RegexStatus: status,
			RegexReason: reason,
		})
	}
	return out
}

// regexStatusOf 现有正则的状态码（已有值不在本轮护栏流程内，按 accepted 记）。
func regexStatusOf(pattern string) string {
	if strings.TrimSpace(pattern) == "" {
		return regexRejectEmpty
	}
	return regexStatusAccepted
}

func regexReasonOf(pattern string) string {
	if strings.TrimSpace(pattern) == "" {
		return "未配置正则兜底"
	}
	return "沿用已保存的正则"
}

// suggestRulesForFields 调模型生成规则建议（纯计算，不落库）。
func suggestRulesForFields(
	ctx context.Context, model chat.Chat, certType string,
	fields []AIRuleFieldInput, sampleText string,
) (*AISuggestRules, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("no fields to suggest rules for")
	}
	thinking := false
	llmCtx := types.WithLLMCallMetadata(ctx, "extract_suggest_rules", "invoice")
	messages := []chat.Message{
		{Role: "system", Content: buildRuleSuggestPrompt(certType, fields, sampleText)},
		{Role: "user", Content: buildFieldSuggestUserPrompt(sampleText)},
	}
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	attempts := []ExtractAttempt{
		{Label: "nonstream-jsonobject", Schema: json.RawMessage(`{"type":"json_object"}`)},
		{Label: "stream-free", Stream: true},
		{Label: "nonstream-free"},
	}

	var parsed AISuggestRules
	raw, err := RunExtractWithRetry(llmCtx, model, messages, baseOpts, attempts,
		func(raw string) bool { return tryParseAISuggestRulesJSON(raw, &parsed) },
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "rule suggest unusable after retries: %v (len=%d)", lastErr, len(lastRaw))
		})
	if err != nil {
		return nil, err
	}
	if !tryParseAISuggestRulesJSON(raw, &parsed) {
		return nil, errParseLLMOutput
	}
	return &parsed, nil
}

// normalizeAISuggestRules 把模型建议对齐到请求字段并跑完正则三重护栏。
//
// 对齐是**双向**的：
//   - 模型自造的字段名（不在请求清单里）直接丢弃——字段名的唯一权威源是
//     「字段定义」Tab 的 subs，不是模型；
//   - 请求里有、模型没回的字段也要出现在结果里（值留空），
//     否则前端预览列表会短一截，用户以为字段被删了。
func normalizeAISuggestRules(
	requested []AIRuleFieldInput, suggested []AISuggestRule, sampleText string,
) *AISuggestRulesResult {
	byName := make(map[string]AISuggestRule, len(suggested))
	for _, s := range suggested {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		if _, dup := byName[n]; dup {
			continue
		}
		byName[n] = s
	}
	out := &AISuggestRulesResult{Fields: []AISuggestRuleValues{}}
	for _, req := range requested {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			continue
		}
		s, ok := byName[name]
		values := AISuggestRuleValues{
			Name:        name,
			RegexStatus: regexRejectEmpty,
		}
		if ok {
			values.Desc = strings.TrimSpace(s.Desc)
			values.Rule = differentiateAIRule(strings.TrimSpace(s.Rule))
			values.Required = s.Required
			values.RegexStatus = regexRejectEmpty
			values.Regex, values.RegexStatus, values.RegexReason = guardAIRuleRegex(name, s.Regex, sampleText)
		}
		out.Fields = append(out.Fields, values)
	}
	return out
}

// guardAIRuleRegex 跑完三重护栏，返回 (正则, 状态码, 原因)。
// 只有状态为 accepted 时正则才非空；其余一律返回空串，让前端展示原因而不是
// 一个"配了但不生效"的输入框。
func guardAIRuleRegex(field, pattern, sampleText string) (string, string, string) {
	// 顺序要紧：**白名单先于空值判定**。
	// 对"自定义扩展字段"而言，"模型没给正则"（rejected_empty）是**偶然**的，
	// 下次模型可能就给了；而"该字段永远不参与补洞"（rejected_not_standard）是
	// **结构性**事实。返回后者才真正回答了用户"我为什么配不了正则"。
	if st := guardAIRegexPatchable(field); st != "" {
		if st == regexRejectNotStandard {
			// 两种"不可配"要说不同的话：用户自定义的中文 key 与"系统标准字段但补洞器
			// 装不下"（如 total_amount / tax_rate）需要的下一步动作完全不同——
			// 前者只能改抽取方式，后者是有意不补洞。
			if invoiceContractCanonicalKeys[canonicalFieldKey(strings.TrimSpace(field))] {
				return "", st, "该字段是系统标准字段，但后端补洞器只支持发票号/发票代码/开票日期/购销方名称/购销方税号/开票人这 8 个字段"
			}
			return "", st, "自定义扩展字段不参与后置正则补洞"
		}
		return "", st, "该字段不在后端补洞白名单内"
	}
	raw := strings.TrimSpace(pattern)
	if raw == "" {
		return "", regexRejectEmpty, "模型未给出正则兜底"
	}
	// 护栏二 + 三
	st := guardAIRegexPattern(raw, sampleText)
	switch st {
	case "":
		return raw, regexStatusAccepted, "样票上唯一命中，可用于后置补洞"
	case regexRejectCompile:
		return "", st, "正则不符合 Go RE2 语法（禁止环视/反向引用/递归）"
	case regexRejectNotUnique:
		return "", st, "该正则在本张样票上无法唯一命中，可能误补到其它票"
	case regexRejectNoSample:
		return "", st, "缺少样票原文，无法验证唯一命中"
	case regexRejectTooBroad:
		return "", st, "正则形态过于宽泛（如 \\d+ 或 .*），误补风险高"
	default:
		return "", st, "正则未通过校验"
	}
}

// differentiateAIRule 防「重启覆盖」：AI 生成的 rule 不能与 invoiceGenericRuleFor
// 的泛用兜底文案**完全相同**。
//
// 原因：SeedInvoiceExtractRuleBackfill → mergeInvoiceRuleFields 会把
// "rule 等于泛用兜底文案" 判定为「seed 未精配」，按底稿覆盖一次。
// 若 AI 恰好产出了与兜底一字不差的句子，用户保存后重启服务配置就静默变了。
// 这里追加一句差异化约束，让"AI 建议"永远被当作用户精配。
func differentiateAIRule(rule string) string {
	r := strings.TrimSpace(rule)
	if r == "" {
		return ""
	}
	generic := InvoiceGenericRuleFor("")
	if r == generic || r == InvoiceGenericRuleFor("其它票据") {
		return r + "（本条为智能配置生成，如与系统兜底文案重复请手工细化）"
	}
	return r
}

// InvoiceGenericRuleFor 暴露 handler 层的泛用兜底文案，避免两处硬编码漂移。
// handler.invoiceGenericRuleFor 委托到此实现。
func InvoiceGenericRuleFor(catName string) string {
	name := strings.TrimSpace(catName)
	if strings.Contains(name, "其它票据") {
		return "此为非标准格式票据（如三联收据、手写票、非正规机打票等）。请尽最大能力识别并提取票面上的付款方（购买方）、收款方（销售方）、开票日期和合计金额。对于无法归入常规字段的有效业务信息（如事由、经办人等），请全部整合汇总提取至‘备注’字段。"
	}
	return "从票面及备注信息中尽力识别并提取该字段对应内容。"
}

// mergeAISuggestRules 按 overwrite 语义把建议合并进现状。
//
//	overwrite=false（默认，防覆盖铁律）：只填**空值**字段，
//	用户已手写 desc/rule 的字段原样保留；required 只在用户未表态时补。
//	overwrite=true：允许覆盖 desc/rule，但**永不覆盖**已保存的 regex
//	（regex 是用户针对具体票样反复调出来的，比模型建议更可信）。
//
// 返回合并结果与"是否发生实质变化"，供前端提示"本次将更新 N 个字段"。
func mergeAISuggestRules(
	requested []AIRuleFieldInput,
	suggested []AISuggestRule,
	existing map[string]AIRuleFieldValues,
	overwrite bool,
) (map[string]AIRuleFieldValues, bool) {
	merged := make(map[string]AIRuleFieldValues, len(requested))
	changed := false
	// 建立 canonical 索引：模型可能回 camelCase / snake_case 变体
	canon := make(map[string]string, len(suggested))
	for _, s := range suggested {
		n := strings.TrimSpace(s.Name)
		if n == "" {
			continue
		}
		canon[canonicalFieldKey(n)] = n
	}
	for _, req := range requested {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			continue
		}
		cur := existing[name]
		key := canonicalFieldKey(name)
		src, ok := canon[key]
		if !ok {
			merged[name] = cur
			continue
		}
		var s AISuggestRule
		for _, cand := range suggested {
			if strings.TrimSpace(cand.Name) == src {
				s = cand
				break
			}
		}
		next := cur
		desc, rule := strings.TrimSpace(s.Desc), differentiateAIRule(strings.TrimSpace(s.Rule))
		if overwrite {
			if desc != "" && next.Desc != desc {
				next.Desc = desc
				changed = true
			}
			if rule != "" && next.Rule != rule {
				next.Rule = rule
				changed = true
			}
		} else {
			if strings.TrimSpace(next.Desc) == "" && desc != "" {
				next.Desc = desc
				changed = true
			}
			if strings.TrimSpace(next.Rule) == "" && rule != "" {
				next.Rule = rule
				changed = true
			}
		}
		if !next.Required && s.Required {
			next.Required = true
			changed = true
		}
		merged[name] = next
	}
	return merged, changed
}
