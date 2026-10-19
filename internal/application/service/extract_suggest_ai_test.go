package service

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 「智能配置规则」（接口 B ai-suggest-rules）的**正则三重护栏**。
//
// 这是 Phase B/D 已确立的安全边界：正则兜底的价值在于确定性，代价是误补
// （把 A 票的号码写到 B 票上）。历史上真正出过事的不是"正则写错"，而是
// "正则太宽泛"——\b\d{8}\b 在真实票据里能命中 136 个 8 位数字串。
//
// 因此 AI 生成的任何正则都必须过三关，任何一关不过就丢弃并给出可见原因，
// 绝不下发给前端、绝不写库。
// ---------------------------------------------------------------------------

func TestExtractSuggestAIRegexGuards(t *testing.T) {
	const sample = "发票号码 24312000000012345678 开票日期 2026-07-05 销售方 国家电网"

	t.Run("护栏一：全字段放开（标准/扩展/明细都不再有白名单缺口）", func(t *testing.T) {
		// 2026-10-18 用户口径：补洞器已覆盖全部标准字段 + 自定义 Fields 桶 + 明细子项，
		// 因此不再有「这个键装不下」的结构性拒绝。
		for _, name := range []string{
			"totalAmount", "taxRate", "户号", "车牌号", "invoiceType",
			"invoice_no", "invoice_code", "invoice_date", "buyer_name",
			"buyer_tax_no", "seller_name", "seller_tax_no", "issuer",
			"invoiceNo", "invoiceDate", "buyerTaxNo", "sellerName",
		} {
			if got := guardAIRegexPatchable(name); got != "" {
				t.Fatalf("%s: got %q, want accepted", name, got)
			}
		}
		if got := guardAIRegexPatchable("   "); got != regexRejectNotStandard {
			t.Fatalf("空字段名应拒绝，got %q", got)
		}
	})

	t.Run("护栏二：非法 RE2 rejected_compile（不下发）", func(t *testing.T) {
		// 环视是 RE2 不支持的经典写法；未闭合括号同理
		for _, bad := range []string{
			`(?=发票)\d{20}`,
			`(\d{20}`,
			`\d{20}(?!x)`,
			`(?<name>\d{20})`,
		} {
			if _, err := regexp.Compile(bad); err == nil {
				// Go RE2 实际接受 (?P<name>..) 但不接受 (?<name>..)；这里只断言确实编译失败的
				continue
			}
			st := guardAIRegexPattern(bad, sample)
			if st != regexRejectCompile {
				t.Fatalf("%s: got %q, want %q", bad, st, regexRejectCompile)
			}
		}
	})

	t.Run("护栏二：合法 RE2 通过编译关", func(t *testing.T) {
		if st := guardAIRegexPattern(`\b\d{20}\b`, sample); st != "" {
			t.Fatalf("got %q, want accepted", st)
		}
	})

	t.Run("护栏三：样票上非唯一命中 rejected_not_unique", func(t *testing.T) {
		// 同一段文本里出现两个不同取值 → 无法判断匹配到的是哪个字段
		twoInvoices := "发票号码 24312000000012345678 另有一张 24312000000087654321"
		st := guardAIRegexPattern(`\b\d{20}\b`, twoInvoices)
		if st != regexRejectNotUnique {
			t.Fatalf("got %q, want %q", st, regexRejectNotUnique)
		}
	})

	t.Run("护栏三：无样票文本时按 not_unique 保守丢弃", func(t *testing.T) {
		// 没有样票就无法证明"唯一命中"，而宽泛正则的代价是误补→宁可不给。
		st := guardAIRegexPattern(`\b\d{20}\b`, "")
		if st != regexRejectNoSample {
			t.Fatalf("got %q, want %q", st, regexRejectNoSample)
		}
	})

	t.Run("护栏三：样票上唯一命中 accepted", func(t *testing.T) {
		if st := guardAIRegexPattern(`\b\d{20}\b`, sample); st != "" {
			t.Fatalf("got %q, want accepted", st)
		}
	})

	t.Run("宽泛表达式一律丢弃（形状不唯一 / 超长 / 裸通配）", func(t *testing.T) {
		long := strings.Repeat("a", maxAIRegexLen+1)
		for _, bad := range []string{
			`\d+`,
			`.*`,
			`\w+`,
			`[0-9]+`,
			long,
		} {
			if !narrowEnoughAIRegex(bad) {
				continue
			}
			t.Fatalf("broad pattern should be rejected: %q", bad)
		}
	})

	t.Run("形态合理的窄正则不被误杀", func(t *testing.T) {
		for _, ok := range []string{
			`\b\d{20}\b`,
			`发票号码\s*[:：]?\s*([0-9]{8,20})`,
			`\b[0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10}\b`,
			`\d{4}[-/年]\d{1,2}[-/月]\d{1,2}日?`,
		} {
			if !narrowEnoughAIRegex(ok) {
				t.Fatalf("should be considered narrow: %q", ok)
			}
		}
	})
}

// TestExtractSuggestAIFieldPostProcess 覆盖接口 A 的**防幻觉后处理**：
//   - standard_fields[].key 必须是落库契约键（InvoiceExtractionItem 的 json tag）；
//   - item_sub_fields[].key 必须是小写下划线英文键；
//   - custom_fields[].name 去重且不与标准键冲突；
//   - sample 必须是 sample_text 的真实子串（模型最常见的幻觉是编造样值）。
func TestExtractSuggestAIFieldPostProcess(t *testing.T) {
	const text = "发票号码 24312000000012345678\n开票日期 2026-07-05\n户号 5001234567\n项目名称 电费 用电量 100"
	dict := InvoiceStandardFieldDictionary()

	t.Run("字典与后端补洞范围一致（全部标准键均可补洞）", func(t *testing.T) {
		// 2026-10-18 起白名单放开：字典里每个标准键都必须能补洞，
		// 否则规则面板会出现「能配正则但不执行」的静默失效。
		patchable := 0
		for _, f := range dict {
			if guardAIRegexPatchable(f.Key) == "" {
				patchable++
			}
		}
		if patchable != len(dict) {
			t.Fatalf("dictionary has %d keys but only %d patchable", len(dict), patchable)
		}
		if len(REGEX_PATCHABLE_INVOICE_KEYS) == 0 {
			t.Fatal("补洞键镜像不应为空")
		}
	})

	t.Run("字典键全部是 InvoiceExtractionItem 的落库契约键", func(t *testing.T) {
		known := invoiceContractKeys()
		for _, f := range dict {
			if !known[f.Key] {
				t.Fatalf("dictionary key %q is not a contract key of InvoiceExtractionItem", f.Key)
			}
		}
	})

	t.Run("standard_fields 白名单过滤 + sample 必须命中原文", func(t *testing.T) {
		raw := &AISuggestFieldsRaw{
			StandardFields: []AISuggestStandardFieldRaw{
				{Key: "invoice_no", Enable: true, Sample: "24312000000012345678"},
				{Key: "invoice_date", Enable: true, Sample: "2026-07-05"},
				{Key: "幻觉键", Enable: true, Sample: "24312000000012345678"},
				{Key: "seller_name", Enable: true, Sample: "不存在的公司名"},
				{Key: "tax_rate", Enable: false, Sample: ""},
			},
			ItemSubFields: []AISuggestSubFieldRaw{
				{Key: "name", Label: "项目名称"},
				{Key: "account_no", Label: "户号"},
				{Key: "Bad-Key", Label: "非法"},
				{Key: "1abc", Label: "数字开头"},
				{Key: "a", Label: "过短"},
			},
			CustomFields: []AISuggestCustomFieldRaw{
				{Name: "户号", Sample: "5001234567"},
				{Name: "户号", Sample: "5001234567"},
				{Name: "invoice_no", Sample: "24312000000012345678"},
				{Name: "缴费期", Sample: "2026-06"},
				{Name: "抄表读数", Sample: "凭空捏造的读数"},
			},
		}
		got := postProcessAISuggestFields(raw, text, dict)

		// 标准字段：幻觉键被丢弃；sample 未命中原文时被清空
		byKey := map[string]AISuggestStandardField{}
		for _, f := range got.StandardFields {
			byKey[f.Key] = f
		}
		if _, bad := byKey["幻觉键"]; bad {
			t.Fatalf("non-contract key must be dropped, got %+v", bad)
		}
		if byKey["invoice_no"].Sample != "24312000000012345678" {
			t.Fatalf("invoice_no sample lost: %+v", byKey["invoice_no"])
		}
		if byKey["seller_name"].Sample != "" {
			t.Fatalf("hallucinated sample must be cleared, got %q", byKey["seller_name"].Sample)
		}

		// 明细子列：只保留 ^[a-z][a-z0-9_]{1,31}$ 的键
		var keys []string
		for _, s := range got.ItemSubFields {
			keys = append(keys, s.Key)
		}
		if len(keys) != 2 {
			t.Fatalf("expected 2 valid sub field keys, got %v", keys)
		}
		for _, k := range keys {
			if !aiSubFieldKeyRe.MatchString(k) {
				t.Fatalf("sub field key %q should have been rejected", k)
			}
		}

		// 自定义字段：同名去重 + 排除与标准键冲突 + sample 必须命中原文。
		// 注意：幻觉 sample **只清空 sample，不丢弃字段**（与标准字段同口径）——
		// 旧断言把「抄表读数」写成应被丢弃，其实是把去重 bug 的副产物当成了正确行为。
		var names []string
		byName := map[string]AISuggestCustomField{}
		for _, c := range got.CustomFields {
			names = append(names, c.Name)
			byName[c.Name] = c
		}
		if _, bad := byName["invoice_no"]; bad {
			t.Fatalf("custom field %q must be rejected as contract-key conflict", "invoice_no")
		}
		if c, ok := byName["抄表读数"]; !ok {
			t.Fatalf("Chinese custom field 抄表读数 must be kept (bug: dropped as duplicate)")
		} else if c.Sample != "" {
			t.Fatalf("hallucinated custom sample must be cleared, got %q", c.Sample)
		}
		dup := 0
		for _, n := range names {
			if n == "户号" {
				dup++
			}
		}
		if dup != 1 {
			t.Fatalf("custom field name must be deduped, got %d occurrences", dup)
		}
	})

	t.Run("nil 输入不 panic", func(t *testing.T) {
		got := postProcessAISuggestFields(nil, text, dict)
		if got == nil {
			t.Fatalf("must return non-nil result")
		}
	})

	// 实测踩到：契约键是 snake_case（total_amount），自定义名走的是 camelCase
	// （totalAmount）。直接查 contract[name] 会漏判，于是"标准字段但补洞器不支持"
	// 被说成"自定义扩展字段"——两句文案对用户意味着完全不同的下一步动作。
	// 2026-10-18 终轮：canonical 同义不再"丢弃"，而是**提升为标准字段**。
	// 否则模型把发票代码给成 invoice_code（snake）时会新增一行扩展字段，
	// 界面上就多出一条裸英文 invoice_code。
	t.Run("扩展字段 key 与契约键 canonical 同义时提升为标准字段", func(t *testing.T) {
		for _, name := range []string{"totalAmount", "total_amount", "TOTAL_AMOUNT"} {
			got := postProcessAISuggestFields(&AISuggestFieldsRaw{
				CustomFields: []AISuggestCustomFieldRaw{{Name: name, Sample: ""}},
			}, text, dict)
			if len(got.CustomFields) != 0 {
				t.Fatalf("%q 应提升为标准字段而不是扩展字段，实际 %+v", name, got.CustomFields)
			}
			if len(got.StandardFields) != 1 || got.StandardFields[0].Key != "total_amount" {
				t.Fatalf("%q 应归一到契约键 total_amount，实际 %+v", name, got.StandardFields)
			}
			if got.StandardFields[0].Label != "价税合计" {
				t.Fatalf("提升后应取字典中文名，实际 %q", got.StandardFields[0].Label)
			}
		}
	})

	// 2026-10-18 终轮：契约外 key **不丢弃**，降级为扩展字段并补中文名。
	// 模型经常给出票面上真实存在、只是不在系统契约里的栏位（item_name/spec/…）。
	t.Run("白名单外的标准字段键降级为扩展字段（不丢弃、不报错）", func(t *testing.T) {
		got := postProcessAISuggestFields(&AISuggestFieldsRaw{
			StandardFields: []AISuggestStandardFieldRaw{
				{Key: "item_name", Enable: true, Sample: text},
				{Key: "模型自造键", Enable: true, Sample: text},
			},
		}, text, dict)
		if len(got.StandardFields) != 0 {
			t.Fatalf("契约外 key 不应留在标准字段里，实际 %+v", got.StandardFields)
		}
		if len(got.Skipped) != 0 {
			t.Fatalf("契约外 key 不应再产生告警，实际 %+v", got.Skipped)
		}
		if len(got.CustomFields) != 2 {
			t.Fatalf("契约外 key 应全部降级为扩展字段，实际 %+v", got.CustomFields)
		}
		// 常见英文键自动补中文名
		if got.CustomFields[0].Name != "项目名称" || got.CustomFields[0].Key != "item_name" {
			t.Fatalf("item_name 应映射为「项目名称」，实际 %+v", got.CustomFields[0])
		}
		// 查不到映射时退回原 key，但绝不丢
		if got.CustomFields[1].Name != "模型自造键" {
			t.Fatalf("未命中映射时应保留原键，实际 %+v", got.CustomFields[1])
		}
	})

	t.Run("明细子列非法键被丢弃并说明原因", func(t *testing.T) {
		got := postProcessAISuggestFields(&AISuggestFieldsRaw{
			ItemSubFields: []AISuggestSubFieldRaw{
				{Key: "Bad-Key", Label: "含连字符"},
				{Key: "1abc", Label: "数字开头"},
				{Key: "a", Label: "过短"},
			},
		}, text, dict)
		if len(got.ItemSubFields) != 0 {
			t.Fatalf("all illegal sub field keys must be dropped, got %+v", got.ItemSubFields)
		}
		if len(got.Skipped) != 3 {
			t.Fatalf("expected 3 skip entries, got %d: %+v", len(got.Skipped), got.Skipped)
		}
	})
}

// TestExtractSuggestAIRuleDecodedGeneration 锁定「防重启覆盖」：
// AI 生成的 rule 不能与 invoiceGenericRuleFor 的泛用兜底文案完全相同，
// 否则服务重启时 SeedInvoiceExtractRuleBackfill → mergeInvoiceRuleFields
// 会把它当成"seed 未精配"再覆盖一次，用户的配置静默消失。
func TestExtractSuggestAIRuleDecodedGeneration(t *testing.T) {
	for _, cat := range []string{"其它票据", "普通发票", "水电发票"} {
		generic := InvoiceGenericRuleFor(cat)
		decoded := differentiateAIRule(generic)
		if decoded == generic {
			t.Fatalf("category %q: differentiated rule is identical to the generic fallback", cat)
		}
		if !strings.Contains(decoded, generic) {
			t.Fatalf("category %q: differentiated rule should keep the generic sentence as its base", cat)
		}
	}
	// 空输入不应凭空造文案
	if differentiateAIRule("  ") != "" {
		t.Fatalf("blank rule must stay blank")
	}
}

// TestExtractSuggestAIRuleJSONParsing 覆盖模型输出的宽容解析：
// 代码围栏、前后缀废话、中文引号都是历史实际出现过的形态。
func TestExtractSuggestAIRuleJSONParsing(t *testing.T) {
	const fenced = "好的，以下是结果：\n```json\n{\"fields\":[{\"name\":\"invoice_no\",\"desc\":\"发票号码\",\"rule\":\"提取发票号码栏\",\"required\":true,\"regex\":\"\\\\b\\\\d{20}\\\\b\"}],\"prompt_template\":\"模板\"}\n```"
	var out AISuggestRules
	if !tryParseAISuggestRulesJSON(fenced, &out) {
		t.Fatalf("fenced JSON should parse")
	}
	if len(out.Fields) != 1 || out.Fields[0].Name != "invoice_no" {
		t.Fatalf("unexpected parse result: %+v", out)
	}
	if out.Fields[0].Regex != `\b\d{20}\b` {
		t.Fatalf("regex mangled by escape handling: %q", out.Fields[0].Regex)
	}

	// 非 JSON 一律失败（不得返回半个结果）
	var bad AISuggestRules
	if tryParseAISuggestRulesJSON("这不是 JSON", &bad) {
		t.Fatalf("non-JSON should not parse")
	}
}

// TestExtractSuggestAIRuleNormalize 确认返回给前端的字段名与请求字段**逐一对齐**：
// 模型擅自增加的字段必须丢弃（字段名的唯一权威源是「字段定义」Tab 的 subs）。
func TestExtractSuggestAIRuleNormalize(t *testing.T) {
	requested := []AIRuleFieldInput{
		{Name: "invoiceNo", DataType: "text"},
		{Name: "户号", DataType: "text"},
	}
	suggested := []AISuggestRule{
		{Name: "invoiceNo", Desc: "发票号码", Rule: "提取号码栏", Regex: `\b\d{20}\b`},
		{Name: "户号", Desc: "用电户号", Rule: "提取户号栏"},
		{Name: "模型自造字段", Desc: "x", Rule: "y"},
	}
	got := normalizeAISuggestRules(requested, suggested, "发票号码 24312000000012345678")

	if len(got.Fields) != 2 {
		t.Fatalf("expected exactly 2 fields (matching request), got %d: %+v", len(got.Fields), got.Fields)
	}
	for _, f := range got.Fields {
		if f.Name == "模型自造字段" {
			t.Fatalf("model-invented field must be dropped")
		}
	}
	no := got.Fields[0]
	if no.Name != "invoiceNo" {
		t.Fatalf("field name must be echoed as requested, got %q", no.Name)
	}
	if no.RegexStatus != regexStatusAccepted {
		t.Fatalf("invoice_no regex should be accepted, got %q", no.RegexStatus)
	}
	hu := got.Fields[1]
	if hu.Name != "户号" {
		t.Fatalf("unexpected field %q", hu.Name)
	}
	if hu.Regex != "" || hu.RegexStatus != regexRejectEmpty {
		t.Fatalf("自定义字段未给正则时应报 rejected_empty: %+v", hu)
	}
	if hu.RegexReason == "" {
		t.Fatalf("rejected regex must carry a human-readable reason")
	}
}

// TestExtractSuggestAIRuleOverwriteSemantics 锁定 overwrite=false 的防覆盖语义：
// 后端只对**空值**字段填建议，用户已手写的内容原样保留。
func TestExtractSuggestAIRuleOverwriteSemantics(t *testing.T) {
	requested := []AIRuleFieldInput{{Name: "invoiceNo", DataType: "text"}}
	suggested := []AISuggestRule{{Name: "invoiceNo", Desc: "AI描述", Rule: "AI规则"}}
	existing := map[string]AIRuleFieldValues{
		"invoiceNo": {Desc: "用户手写描述", Rule: "用户手写规则"},
	}
	keep, changed := mergeAISuggestRules(requested, suggested, existing, false)
	if changed {
		t.Fatalf("overwrite=false must not report changes for hand-written fields")
	}
	if keep["invoiceNo"].Desc != "用户手写描述" || keep["invoiceNo"].Rule != "用户手写规则" {
		t.Fatalf("hand-written content must be preserved: %+v", keep["invoiceNo"])
	}

	// overwrite=true 时才允许覆盖
	_, changed2 := mergeAISuggestRules(requested, suggested, existing, true)
	if !changed2 {
		t.Fatalf("overwrite=true should report changes")
	}

	// 空值字段无论如何都会被填上（这是 AI 建议的核心价值）
	empty := map[string]AIRuleFieldValues{"invoiceNo": {}}
	keep3, changed3 := mergeAISuggestRules(requested, suggested, empty, false)
	if !changed3 || keep3["invoiceNo"].Desc != "AI描述" {
		t.Fatalf("empty fields must be filled regardless of overwrite: %+v", keep3)
	}
}

// TestExtractSuggestAIRuleStatusSurvivesMerge 回归一个实测踩到的缺陷：
// 护栏结论（regex_status/regex_reason）如果只停留在「建议」阶段、合并时被丢掉，
// 前端只能按"有没有正则"重新猜，把「形态太宽泛」这类真正有用的诊断
// 抹平成信息量为零的 rejected_empty。这条测试锁定「结论必须跟着值流到最终响应」。
func TestExtractSuggestAIRuleStatusSurvivesMerge(t *testing.T) {
	fields := []AIRuleFieldInput{
		{Name: "invoiceNo", DataType: "string"},
		{Name: "车牌号", DataType: "string"},
		{Name: "totalAmount", DataType: "number"},
	}
	suggested := []AISuggestRule{
		{Name: "invoiceNo", Desc: "发票号码", Rule: "提取号码栏", Regex: `\b\d{20}\b`},
		{Name: "车牌号", Desc: "车牌号", Rule: "从备注中分离车牌号", Regex: `\d{20}`},
		{Name: "totalAmount", Desc: "价税合计", Rule: "提取合计栏", Regex: `\d+`},
	}
	const sample = "发票号码 24312000000012345678 备注 车牌号：渝C87567"
	values, _ := MergeAIRuleSuggestions(fields, suggested, nil, false, sample)
	got := NewAISuggestRulesResult("通行费发票", fields, values)

	byName := map[string]AISuggestRuleValues{}
	for _, f := range got.Fields {
		byName[f.Name] = f
	}

	// 8 键白名单内的字段：正则过护栏 → accepted
	if byName["invoiceNo"].RegexStatus != regexStatusAccepted {
		t.Fatalf("invoiceNo should be accepted, got %q", byName["invoiceNo"].RegexStatus)
	}
	if byName["invoiceNo"].Regex == "" {
		t.Fatalf("invoiceNo should carry the validated regex")
	}
	// 自定义 key 现在也可补洞，但 `\d{20}` 这种纯形状式仍必须被形态护栏拦下
	//（它会命中票面上任意 20 位数字串，误补风险极高）。
	if byName["车牌号"].RegexStatus != regexRejectTooBroad {
		t.Fatalf("车牌号 should report rejected_too_broad, got %q", byName["车牌号"].RegexStatus)
	}
	if byName["车牌号"].RegexReason == "" {
		t.Fatalf("车牌号 must carry a diagnostic reason")
	}
	if byName["totalAmount"].RegexStatus != regexRejectTooBroad {
		t.Fatalf("totalAmount should report rejected_too_broad, got %q", byName["totalAmount"].RegexStatus)
	}
	if !strings.Contains(byName["totalAmount"].RegexReason, "宽泛") {
		t.Fatalf("totalAmount reason must explain broadness, got %q", byName["totalAmount"].RegexReason)
	}
	// 护栏未通过的值里绝不能残留正则（否则可能被保存进库）
	for _, f := range got.Fields {
		if f.RegexStatus != regexStatusAccepted && f.Regex != "" {
			t.Fatalf("field %q carries a regex that failed the guards: %q", f.Name, f.Regex)
		}
	}
}

// TestExtractSuggestAICustomFieldsChineseKeys 锁定实测 bug：
// 票面上的 7 个中文扩展字段（地址电话/业务名称/车牌/办理时间/复核…）
// 被误判为"与前面建议的扩展字段重名"全部丢弃。
//
// 根因：custom 去重键取 canonicalFieldKey(name)，而该函数只保留 ASCII
// 字母数字——纯中文名一律归一为空串。第一个中文扩展字段把 seenCustom[""]
// 置真，其后每个中文名都命中"重复"。空键绝不能当作身份参与去重。
func TestExtractSuggestAICustomFieldsChineseKeys(t *testing.T) {
	dict := InvoiceStandardFieldDictionary()
	names := []string{"地址、电话", "业务名称", "车牌", "办理时间", "复核", "户号", "抄表读数"}
	raw := &AISuggestFieldsRaw{}
	for _, n := range names {
		raw.CustomFields = append(raw.CustomFields, AISuggestCustomFieldRaw{Name: n})
	}
	sample := strings.Join(names, "\n")
	got := postProcessAISuggestFields(raw, sample, dict)

	if len(got.CustomFields) != len(names) {
		t.Fatalf("all %d Chinese custom fields must survive, got %d: %+v (skipped=%+v)",
			len(names), len(got.CustomFields), got.CustomFields, got.Skipped)
	}
	seenKey := map[string]bool{}
	for i, c := range got.CustomFields {
		if c.Name != names[i] {
			t.Fatalf("custom[%d] name = %q, want %q", i, c.Name, names[i])
		}
		// 每个扩展字段都必须拿到唯一、合法的 snake_case 编码（前端只读展示/落库按它索引）
		if !aiSubFieldKeyRe.MatchString(c.Key) || len(c.Key) > aiSubFieldKeyMaxLen {
			t.Fatalf("custom[%d] key %q is not a legal snake_case key", i, c.Key)
		}
		if seenKey[c.Key] {
			t.Fatalf("duplicate custom key %q", c.Key)
		}
		seenKey[c.Key] = true
	}
	// 语义映射：能认得的中文名给出有意义键，而不是清一色 custom_field_2/3
	wantKeys := map[string]string{
		"户号": "account_no", "车牌": "plate_no", "办理时间": "handle_time",
	}
	for _, c := range got.CustomFields {
		if want, ok := wantKeys[c.Name]; ok && c.Key != want {
			t.Fatalf("custom %q key = %q, want %q", c.Name, c.Key, want)
		}
	}

	// 模型自己给了合法 key：直接采用（尊重模型的语义命名）
	withModelKey := postProcessAISuggestFields(&AISuggestFieldsRaw{
		CustomFields: []AISuggestCustomFieldRaw{{Key: "bank_account", Name: "开户银行及账号"}},
	}, "开户银行及账号", dict)
	if len(withModelKey.CustomFields) != 1 || withModelKey.CustomFields[0].Key != "bank_account" {
		t.Fatalf("model-provided key must be kept, got %+v", withModelKey.CustomFields)
	}

	// 模型给的 key 与契约键冲突：**提升为标准字段**（同一个存储位，不能建两个字段），
	// 且中文名保留模型给的语义（含税总额），不被字典默认值覆盖。
	conflict := postProcessAISuggestFields(&AISuggestFieldsRaw{
		CustomFields: []AISuggestCustomFieldRaw{{Key: "total_amount", Name: "含税总额"}},
	}, "含税总额", dict)
	if len(conflict.CustomFields) != 0 {
		t.Fatalf("与契约键同义应提升为标准字段，实际仍留在扩展字段 %+v", conflict.CustomFields)
	}
	if len(conflict.StandardFields) != 1 || conflict.StandardFields[0].Key != "total_amount" {
		t.Fatalf("应提升为契约键 total_amount，实际 %+v", conflict.StandardFields)
	}
	if conflict.StandardFields[0].Label != "含税总额" {
		t.Fatalf("提升后应保留模型给的中文名，实际 %q", conflict.StandardFields[0].Label)
	}

	// 真正同名的字段仍必须去重（不能因为修 bug 就把去重整个去掉）
	dup := postProcessAISuggestFields(&AISuggestFieldsRaw{
		CustomFields: []AISuggestCustomFieldRaw{{Name: "户号"}, {Name: "户号"}},
	}, "户号", dict)
	if len(dup.CustomFields) != 1 {
		t.Fatalf("exact duplicate names must still be deduped, got %+v", dup.CustomFields)
	}
}

// ---- 2026-10-18 第二轮：有无样本差异化 + 全字段开放 ----

// TestExtractSuggestAIRulesNoSampleSkipsRegex 锁定「无样票不生成正则」：
// 正则的价值全在唯一命中，没有样票就无法证明，产出一批必然被丢弃的建议
// 只会让用户误以为系统坏了。
func TestExtractSuggestAIRulesNoSampleSkipsRegex(t *testing.T) {
	fields := []AIRuleFieldInput{
		{Name: "invoiceNo", DataType: "string"},
		{Name: "totalAmount", DataType: "number"},
		{Name: "户号", DataType: "string"},
	}
	suggested := []AISuggestRule{
		{Name: "invoiceNo", Desc: "发票号码", Rule: "提取号码栏", Regex: `发票号码\s*[:：]?\s*(\d{8,20})`},
		{Name: "totalAmount", Desc: "价税合计", Rule: "提取合计栏", Regex: `价税合计\s*[:：]?\s*([\d,.]+)`},
		{Name: "户号", Desc: "用电户号", Rule: "提取户号栏", Regex: `户号\s*[:：]?\s*(\d{7,12})`},
	}
	// sampleText 为空 = 用户没贴样票
	values, _ := MergeAIRuleSuggestions(fields, suggested, nil, false, "")
	got := NewAISuggestRulesResult("普通发票", fields, values)

	if len(got.Fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(got.Fields))
	}
	for _, f := range got.Fields {
		if f.Regex != "" {
			t.Fatalf("无样票时 %q 不得携带正则，实际 %q", f.Name, f.Regex)
		}
		if f.RegexStatus != regexRejectNoSample {
			t.Fatalf("无样票时 %q 应报 rejected_no_sample，实际 %q", f.Name, f.RegexStatus)
		}
		if f.RegexReason == "" {
			t.Fatalf("无样票时 %q 必须给出原因", f.Name)
		}
	}
	// 自然语言规则必须照常生成——无样票只是不生成正则，不是不生成规则。
	for _, f := range got.Fields {
		if f.Rule == "" || f.Desc == "" {
			t.Fatalf("%q 的 desc/rule 不应为空: %+v", f.Name, f)
		}
	}
}

// TestExtractSuggestAIRulesCustomFieldRegexAccepted 锁定「扩展字段也能配正则」：
// 补洞器已把自定义字段写进 Fields 桶，护栏不应再按老白名单拒绝。
func TestExtractSuggestAIRulesCustomFieldRegexAccepted(t *testing.T) {
	fields := []AIRuleFieldInput{
		{Name: "户号", DataType: "string"},
		{Name: "totalAmount", DataType: "number"},
	}
	suggested := []AISuggestRule{
		{Name: "户号", Desc: "用电户号", Rule: "提取户号栏", Regex: `户号\s*[:：]?\s*(\d{7,12})`},
		{Name: "totalAmount", Desc: "价税合计", Rule: "提取合计栏", Regex: `价税合计\s*[:：]?\s*[¥￥]?\s*([\d,]+\.\d{2})`},
	}
	const sample = "户号: 5001234567\n价税合计: ¥1,234.56\n"
	values, _ := MergeAIRuleSuggestions(fields, suggested, nil, false, sample)
	got := NewAISuggestRulesResult("水电发票", fields, values)
	byName := map[string]AISuggestRuleValues{}
	for _, f := range got.Fields {
		byName[f.Name] = f
	}
	if byName["户号"].RegexStatus != regexStatusAccepted || byName["户号"].Regex == "" {
		t.Fatalf("自定义字段的正则应被接受: %+v", byName["户号"])
	}
	if byName["totalAmount"].RegexStatus != regexStatusAccepted || byName["totalAmount"].Regex == "" {
		t.Fatalf("数值标准字段的正则应被接受: %+v", byName["totalAmount"])
	}
}

// TestExtractSuggestAIRulesPromptOmitsRegexWithoutSample 锁定 Prompt 层也要分叉：
// 无样票时若仍要求模型"给出正则"，模型一定会编，护栏再丢掉等于白烧 token。
func TestExtractSuggestAIRulesPromptOmitsRegexWithoutSample(t *testing.T) {
	fields := []AIRuleFieldInput{{Name: "invoiceNo", DataType: "string"}}

	noSample := buildRuleSuggestPrompt("普通发票", fields, "")
	if !strings.Contains(noSample, "未提供样票") || !strings.Contains(noSample, "空字符串") {
		t.Fatalf("无样票 Prompt 必须明确要求 regex 为空:\n%s", noSample)
	}
	if strings.Contains(noSample, "样票原文") {
		t.Fatalf("无样票 Prompt 不得包含样票原文段:\n%s", noSample)
	}

	withSample := buildRuleSuggestPrompt("普通发票", fields, "发票号码 24312000000012345678")
	if !strings.Contains(withSample, "样票原文") {
		t.Fatalf("有样票 Prompt 必须带样票原文段:\n%s", withSample)
	}
	if strings.Contains(withSample, "未提供样票") {
		t.Fatalf("有样票 Prompt 不得出现「未提供样票」:\n%s", withSample)
	}
}

// TestStandardFieldDictionaryForScope 锁定蓝图 P2-2：AI 字段字典全面 Schema 驱动。
//
// 三条红线：
//  1. 发票字典内容与重构前逐项一致（label/type/hint 都不能变，Prompt 契约）；
//  2. 非发票已注册 scope 字典为空（此前错拿发票字典，会给合同建议发票键）；
//  3. 未注册 scope 返回 nil（调用方按"无字典"处理，不静默回落发票）。
func TestStandardFieldDictionaryForScope(t *testing.T) {
	t.Run("发票字典与 Schema 补洞面 + 字典专属字段一致", func(t *testing.T) {
		dict := StandardFieldDictionaryForScope("invoice")
		require.NotEmpty(t, dict)

		// 键序 = Schema 声明序 + category/items 殿后（补洞遍历序可复现）。
		schemaKeys := ScopeSchemaStandardKeys("invoice")
		require.Len(t, dict, len(schemaKeys)+2, "字典 = 补洞面 + category/items")

		// 契约键必须真实存在于 InvoiceExtractionItem（reflect 校验，防手抄漂移）。
		contract := invoiceContractKeys()
		for _, f := range dict[:len(schemaKeys)] {
			assert.True(t, contract[f.Key], "字典键 %q 不是 InvoiceExtractionItem 的 json tag", f.Key)
			assert.NotEmpty(t, f.Label, "%q 缺中文名", f.Key)
		}
		assert.Equal(t, "category", dict[len(dict)-2].Key)
		assert.Equal(t, "items", dict[len(dict)-1].Key)
		assert.Equal(t, "array", dict[len(dict)-1].DataType, "items 字典项必须是 array 类型")
	})

	t.Run("document scope 字典为空", func(t *testing.T) {
		for _, scope := range []string{"contract", "regulation", "notice", "award_punish", "utility", "solar"} {
			dict := StandardFieldDictionaryForScope(scope)
			assert.NotNil(t, dict, "%s 已注册，空字典也应返回非 nil 空切片", scope)
			assert.Empty(t, dict, "%s 无标准字段，字典必须为空", scope)
		}
	})

	t.Run("未注册 scope 返回 nil", func(t *testing.T) {
		assert.Nil(t, StandardFieldDictionaryForScope("not-a-scope"))
	})

	t.Run("发票 schema 已注册且声明三件套", func(t *testing.T) {
		schema, ok := GetScopeSchema("invoice")
		require.True(t, ok)
		assert.Equal(t, "invoice", schema.Kind)
		assert.True(t, schema.RegexPatchable)
		assert.Equal(t, "Fields", schema.DynamicBucket)
		assert.Equal(t, "items", schema.ItemBucket)
		assert.Equal(t, "Extra", schema.ItemField)
	})
}
