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

// REGEX_PATCHABLE_INVOICE_KEYS 是「后端补洞器支持的发票标准键」的服务端镜像。
// 2026-10-18 起补洞范围放开到全部标准字段（见 ScopeSchema 的 StandardFields），
// 2026-10-19 起改从 ScopeSchema 单一权威源派生（蓝图 P2-1），仍导出给前端做
// 展示与单测断言。
var REGEX_PATCHABLE_INVOICE_KEYS = ScopeSchemaStandardKeys(types.FleetCategoryScopeInvoice)

// guardAIRegexPatchable 护栏一：字段是否在补洞器可执行范围内。
//
// 2026-10-18 起发票补洞器覆盖「全部标准字段 + 自定义扩展字段（Fields 桶）+
// 明细子项（items[].extra）」，不再有"这个键装不下"的结构性缺口，
// 因此这里恒通过；保留函数是为了让护栏链路顺序与诊断码保持稳定。
func guardAIRegexPatchable(field string) string {
	if strings.TrimSpace(field) == "" {
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
//
// Key 是**后端分配的稳定英文编码**（^[a-z][a-z0-9_]{1,31}$），前端只读展示。
// 为什么不让前端用中文名当键：提取结果落库按字段名索引，中文名一改列就错位；
// 而契约键（invoice_no）只覆盖标准字段，扩展字段必须有自己的 snake_case 身份。
type AISuggestCustomField struct {
	Key        string `json:"key"`
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
	Key        string `json:"key"`
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
// 中文名**显式声明在 ScopeSchema 里**（extract_schema_registry.go），不从
// invoiceFieldAliases 反查：那张表是「中文 → 英文」的别名表（收据编号/发票号码/
// 票号…都映射到 invoice_no），反向取"第一条别名"得到的是任意一个别名（可能正是
// "收据编号"），拿去当标准字段名会误导用户。
func InvoiceStandardFieldDictionary() []InvoiceStandardField {
	return StandardFieldDictionaryForScope(types.FleetCategoryScopeInvoice)
}

// StandardFieldDictionaryForScope 按 scope 的 ScopeSchema 生成标准字段字典
// （蓝图 P2-2：AI 字段/规则建议全面 Schema 驱动）。
//
// 发票：Schema 的补洞面 22 项 + 字典专属的 category/items（见注册表注释）。
// 其它已注册 scope：StandardFields 为空 → 返回空字典，模型只能产自定义字段
// （此前它们错拿发票字典，会建议一堆合同/制度里根本不存在的键）。
// 未注册 scope：nil（调用方按"无字典"处理）。
func StandardFieldDictionaryForScope(scope string) []InvoiceStandardField {
	schema, ok := GetScopeSchema(scope)
	if !ok {
		return nil
	}
	if len(schema.StandardFields) == 0 {
		return []InvoiceStandardField{}
	}
	fields := schema.StandardFields
	if schema.Kind == "invoice" {
		fields = append(append([]types.FieldDef(nil), fields...), invoiceDictionaryExtraFields()...)
	}
	out := make([]InvoiceStandardField, 0, len(fields))
	for _, fd := range fields {
		out = append(out, InvoiceStandardField{
			Key: fd.Key, Label: fd.Label,
			DataType: string(fd.Type), Hint: fd.Hint,
		})
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
	sb.WriteString("4. custom_fields：票面上有、但字典里没有的信息（如户号、缴费期、抄表读数）在此给出中文名 name、")
	sb.WriteString("规范英文 key（小写字母开头，仅 a-z0-9_，如 account_no / bank_account）与 sample。")
	sb.WriteString("若该信息没有独立存储位，store 填 remark，否则填 fields。\n")
	sb.WriteString("5. 不要输出字典中不存在、票面上也没出现的字段。\n")
	sb.WriteString("6. 输出结构固定为：")
	sb.WriteString(`{"standard_fields":[{"key":"","label":"","data_type":"","enable":false,"sample":""}],`)
	sb.WriteString(`"item_sub_fields":[{"key":"","label":"","data_type":"","width":0}],`)
	sb.WriteString(`"custom_fields":[{"key":"","name":"","data_type":"","sample":"","store":"fields","remark_part":false}]}`)
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
//	standard_fields：key 必须在落库契约键集合内——**契约外的 key 不再丢弃**，
//	                 而是降级成扩展字段（custom_fields），并尽量补一个中文名；
//	item_sub_fields：key 必须匹配 ^[a-z][a-z0-9_]{1,31}$；按 key 去重。
//	custom_fields ：中文名去重、与标准契约键 canonical 同义的**提升为标准字段**、
//	                 sample 必须是原文子串。
//
// 为什么「契约外就丢弃」是错的：模型经常给出 item_name/spec/unit/quantity/price
// 这类票面上真实存在、只是不在系统契约里的字段；丢掉它们等于让用户重填一遍，
// 还会在界面上堆一条"有 N 项建议未被采纳"的告警。降级为扩展字段既保住信息，
// 又不会污染标准列。
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
	// 被降级为标准字段的扩展建议：稍后与 raw.CustomFields 一起走同一套分配/去重逻辑
	demoted := make([]AISuggestCustomFieldRaw, 0, len(raw.StandardFields))

	// ① 标准字段
	seenStd := map[string]bool{}
	for _, f := range raw.StandardFields {
		key := strings.TrimSpace(f.Key)
		if key == "" || seenStd[key] {
			continue
		}
		if !contract[key] {
			// 契约外 key：**不丢弃**，降级为扩展字段（中文名优先取模型 label，
			// 其次按常见英文键查表，最后退回 key 本身）。
			demoted = append(demoted, AISuggestCustomFieldRaw{
				Key:      key,
				Name:     demotedFieldLabel(key, f.Label),
				DataType: strings.TrimSpace(f.DataType),
				Sample:   f.Sample,
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

	// ③ 自定义字段：先分配唯一英文 key，再按「真实中文名」去重，最后校验 sample。
	//
	// 历史 bug（实测 7 项全丢）：去重键曾取 canonicalFieldKey(name)，而该函数
	// 只保留 ASCII 字母数字——纯中文名一律归一成空串。第一个中文扩展字段把
	// seenCustom[""] 置真，其后**每个**中文名都命中"重复"而被丢弃。
	// 现在：空 key 不参与身份判定；同名才算重复；key 由 assignCustomFieldKey
	// 保证在「契约键 + 子列键 + 已分配 key」空间内唯一。
	usedKeys := make(map[string]bool, len(contract)+len(out.ItemSubFields))
	for k := range contract {
		usedKeys[k] = true
	}
	for _, s := range out.ItemSubFields {
		usedKeys[s.Key] = true
	}
	// canonical → 契约键：模型常把 invoice_code（snake）当扩展字段提出来，
	// 而分类里同义的很可能是 invoiceCode（camel）。两边 canonical 相同就说明是同一个字段，
	// 必须并回标准字段，否则界面会出现 invoice_code 与 invoiceCode 两行、或一行裸英文。
	contractByCanonical := make(map[string]string, len(contract))
	for k := range contract {
		if c := canonicalFieldKey(k); c != "" {
			contractByCanonical[c] = k
		}
	}
	seenCustomName := map[string]bool{}
	customQueue := make([]AISuggestCustomFieldRaw, 0, len(demoted)+len(raw.CustomFields))
	customQueue = append(customQueue, demoted...)
	customQueue = append(customQueue, raw.CustomFields...)
	for _, c := range customQueue {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			continue
		}
		// 与本次已建议的扩展字段去重（同名才合并）
		if seenCustomName[name] {
			skipped = append(skipped, AISuggestSkipInfo{Name: name, Reason: "与前面建议的扩展字段重名"})
			continue
		}
		// 用 canonical 集合判冲突：契约键是 snake_case（total_amount），
		// 而自定义名可能是 camelCase（totalAmount）或中文，直接查 contract[name]
		// 会漏判，于是"标准字段但补洞器不支持"被误报为"自定义扩展字段"。
		stdKey := contractByCanonical[canonicalFieldKey(name)]
		if stdKey == "" {
			stdKey = contractByCanonical[canonicalFieldKey(c.Key)]
		}
		if stdKey != "" {
			// 与标准契约键同义 → 提升为标准字段（而不是丢弃/重复建一行）
			if seenStd[stdKey] {
				continue
			}
			seenStd[stdKey] = true
			label := name
			if canonicalFieldKey(label) == canonicalFieldKey(stdKey) {
				label = "" // 传进来的"名字"其实就是键，換字典里的中文名
			}
			dtype := strings.TrimSpace(c.DataType)
			if d, ok := dictByKey[stdKey]; ok {
				if label == "" {
					label = d.Label
				}
				if dtype == "" {
					dtype = d.DataType
				}
			}
			if label == "" {
				label = stdKey
			}
			out.StandardFields = append(out.StandardFields, AISuggestStandardField{
				Key:      stdKey,
				Label:    label,
				DataType: dtype,
				Enable:   true,
				Sample:   verifiedSample(c.Sample, sampleNorm, true),
			})
			continue
		}
		seenCustomName[name] = true
		key := assignCustomFieldKey(c.Key, name, usedKeys)
		dtype := strings.TrimSpace(c.DataType)
		if dtype != "number" && dtype != "date" && dtype != "array" {
			dtype = "text"
		}
		store := strings.TrimSpace(c.Store)
		if store != "remark" {
			store = "fields"
		}
		out.CustomFields = append(out.CustomFields, AISuggestCustomField{
			Key:        key,
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

// nonContractKeyLabels 模型自造（契约外）的英文键 → 中文名。
//
// 这批键几乎都是票面上真实存在的栏位（明细列/通行信息），只是不在系统契约里。
// 降级成扩展字段时必须给个像样的中文名，否则用户看到一列 item_name 等于没降级。
// 表里没有的键：先用模型自己给的 label，再退回 key 本身。
var nonContractKeyLabels = map[string]string{
	"item_name":    "项目名称",
	"name":         "项目名称",
	"spec":         "规格型号",
	"unit":         "单位",
	"quantity":     "数量",
	"qty":          "数量",
	"price":        "单价",
	"unit_price":   "单价",
	"amount":       "金额",
	"tax_rate":     "税率",
	"tax_amount":   "税额",
	"tax":          "税额",
	"plate_no":     "车牌号",
	"vehicle_type": "车辆类型",
	"travel_from":  "通行日期起",
	"travel_to":    "通行日期止",
	"account_no":   "户号",
	"batch_no":     "批次号",
	"remark":       "备注",
	"category":     "票据大类",
}

// demotedFieldLabel 给降级为扩展字段的建议挑一个中文显示名。
// 优先级：中文名映射表 → 模型给的 label（且不能只是一个英文键）→ key 本身。
func demotedFieldLabel(key, modelLabel string) string {
	if zh, ok := nonContractKeyLabels[strings.ToLower(strings.TrimSpace(key))]; ok {
		return zh
	}
	label := strings.TrimSpace(modelLabel)
	if label != "" && canonicalFieldKey(label) != canonicalFieldKey(key) {
		return label
	}
	if label != "" && !isASCIIKeyLike(label) {
		return label
	}
	return key
}

// isASCIIKeyLike 判断一个字符串是否"就是英文键"（纯 ASCII 且无空格/中文）。
func isASCIIKeyLike(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r > 127 || r == ' ' {
			return false
		}
	}
	return true
}

// customFieldKeyHints 中文扩展字段名 → 规范英文键的映射表。
//
// 只覆盖发票/收据上高频出现的栏位；命中不到时退回 custom_field + 序号，
// 保证「每个扩展字段都拿得到唯一 key」这件事不依赖模型听话。
// 顺序敏感：更具体的词（银行账号）必须排在泛词（账号）之前。
var customFieldKeyHints = []struct {
	keywords []string
	key      string
}{
	{[]string{"开户行", "开户银行"}, "bank_name"},
	{[]string{"银行账号", "银行账户", "账号", "账户"}, "bank_account"},
	{[]string{"户号", "用户号", "客户号"}, "account_no"},
	{[]string{"业务名称", "业务类型"}, "business_name"},
	{[]string{"车牌号", "车牌"}, "plate_no"},
	{[]string{"办理时间", "受理时间", "办理日期"}, "handle_time"},
	{[]string{"复核人", "复核", "审核人"}, "reviewer"},
	{[]string{"抄表读数", "抄表", "读数"}, "meter_reading"},
	{[]string{"缴费期", "账期", "所属期"}, "billing_period"},
	{[]string{"用电量", "水量", "用量"}, "usage"},
	{[]string{"单价"}, "unit_price"},
	{[]string{"数量"}, "quantity"},
	{[]string{"规格型号", "规格"}, "spec"},
	{[]string{"地址", "住址", "经营地址"}, "address"},
	{[]string{"电话", "联系电话", "手机"}, "phone"},
	{[]string{"项目名称", "品名"}, "item_name"},
}

// customFieldKeyHint 按中文名查推荐键（未命中返回空串）。
func customFieldKeyHint(name string) string {
	for _, h := range customFieldKeyHints {
		for _, kw := range h.keywords {
			if strings.Contains(name, kw) {
				return h.key
			}
		}
	}
	return ""
}

// sanitizeCustomKey 把模型给的 key 规整成 ^[a-z][a-z0-9_]{1,31}$；不合规返回空串。
func sanitizeCustomKey(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		return ""
	}
	if len(out) > aiSubFieldKeyMaxLen {
		out = strings.TrimRight(out[:aiSubFieldKeyMaxLen], "_")
	}
	if !aiSubFieldKeyRe.MatchString(out) {
		return ""
	}
	return out
}

// assignCustomFieldKey 为一条扩展字段分配唯一英文 key（会写 used）。
//
// 优先级：模型给的合法 key → 中文名映射 → custom_field；
// 任一候选被占用（契约键/子列键/其它扩展字段）就加 _2/_3 后缀直到唯一。
// 唯一性是硬要求：key 会作为提取结果的 JSON 键，重复会让两个字段互相覆盖。
func assignCustomFieldKey(modelKey, name string, used map[string]bool) string {
	candidates := make([]string, 0, 3)
	if k := sanitizeCustomKey(modelKey); k != "" {
		candidates = append(candidates, k)
	}
	if k := customFieldKeyHint(name); k != "" {
		candidates = append(candidates, k)
	}
	candidates = append(candidates, "custom_field")

	for _, base := range candidates {
		if !used[base] {
			used[base] = true
			return base
		}
	}
	base := candidates[len(candidates)-1]
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if !used[cand] {
			used[cand] = true
			return cand
		}
	}
	// 理论上不可达（1000 个同名字段），保底返回不 panic。
	return base
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
	// 有无样票是两种任务：没有样票时"生成正则"是无根据的猜测（无法验证唯一命中），
	// 只会产出一批必然被护栏丢弃的垃圾；有样票才让它连同正则一起给。
	if strings.TrimSpace(sampleText) == "" {
		sb.WriteString("【regex 要求】本次**未提供样票**，无法验证唯一命中，因此所有字段的 regex 必须输出空字符串（不要尝试编造正则）。\n")
	} else {
		sb.WriteString("【regex 要求】\n")
		sb.WriteString("- 每个字段都可给出后置兜底正则（标准字段、扩展字段、明细子项同口径），不需要时输出空字符串。\n")
		sb.WriteString("- 必须是 Go RE2 语法：可用字符类、量词、\\b 边界、捕获组；禁止环视 (?=)(?!)、反向引用、递归。\n")
		sb.WriteString("- 必须锚定票面标签（如 `价税合计\\s*[:：]?\\s*[¥￥]?\\s*([\\d,]+\\.\\d{2})`），确保在下述原文中**唯一命中**；\n")
		sb.WriteString("  纯形状匹配（`\\d+`、`.*`、`\\d{2,}`）会被系统直接丢弃。\n")
		sb.WriteString("- 数值字段的捕获组只保留数字与千分位，不要带货币符号（清洗由系统做）。\n")
		sb.WriteString("- 无法保证唯一时输出空字符串，不要硬凑。\n")
	}
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
	// 顺序要紧：**范围判定先于空值判定**。
	// 「模型没给正则」是偶然的，而「这个字段根本不能补洞」是结构性事实；
	// 返回后者才真正回答了用户"我为什么配不了正则"。
	if st := guardAIRegexPatchable(field); st != "" {
		return "", st, "字段名为空，无法定位补洞目标"
	}
	// 无样票：本轮只出自然语言规则，正则一律留空。
	// 理由：正则的价值全在"唯一命中"，没有样票根本无法证明，生成出来必被丢弃；
	// 与其让用户看到一堆 rejected_not_unique，不如明确告知"这次不生成正则"。
	if strings.TrimSpace(sampleText) == "" {
		return "", regexRejectNoSample, "未提供样票，本次不生成正则（有样票才能校验唯一命中）"
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
