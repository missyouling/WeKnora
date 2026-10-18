package service

import (
	"regexp"
	"strings"
	"testing"
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

	t.Run("护栏一：非后端 8 键一律 rejected_not_standard", func(t *testing.T) {
		for _, name := range []string{"totalAmount", "taxRate", "户号", "车牌号", "invoiceType"} {
			got := guardAIRegexPatchable(name)
			if got != regexRejectNotStandard {
				t.Fatalf("%s: got %q, want %q", name, got, regexRejectNotStandard)
			}
		}
	})

	t.Run("护栏一：8 键及其camelCase 形式通过", func(t *testing.T) {
		for _, name := range []string{
			"invoice_no", "invoice_code", "invoice_date", "buyer_name",
			"buyer_tax_no", "seller_name", "seller_tax_no", "issuer",
			"invoiceNo", "invoiceDate", "buyerTaxNo", "sellerName",
		} {
			if got := guardAIRegexPatchable(name); got != "" {
				t.Fatalf("%s: got %q, want accepted", name, got)
			}
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

	t.Run("字典与后端补洞白名单一致（8 键）", func(t *testing.T) {
		patchable := 0
		for _, f := range dict {
			if guardAIRegexPatchable(f.Key) == "" {
				patchable++
			}
		}
		if patchable != len(REGEX_PATCHABLE_INVOICE_KEYS) {
			t.Fatalf("dictionary has %d patchable keys, backend supports %d",
				patchable, len(REGEX_PATCHABLE_INVOICE_KEYS))
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
	t.Run("自定义名与契约键的 camelCase 变体也判冲突（canonical 归一后比对）", func(t *testing.T) {
		for _, name := range []string{"totalAmount", "total_amount", "TOTAL_AMOUNT"} {
			got := postProcessAISuggestFields(&AISuggestFieldsRaw{
				CustomFields: []AISuggestCustomFieldRaw{{Name: name, Sample: ""}},
			}, text, dict)
			if len(got.CustomFields) != 0 {
				t.Fatalf("%q should be rejected as a contract-key conflict, got %+v", name, got.CustomFields)
			}
			if len(got.Skipped) == 0 || !strings.Contains(got.Skipped[0].Reason, "标准字段") {
				t.Fatalf("%q must be reported as skipped with a reason, got %+v", name, got.Skipped)
			}
		}
	})

	t.Run("白名单外的标准字段键被丢弃但给出原因（不给静默失败）", func(t *testing.T) {
		got := postProcessAISuggestFields(&AISuggestFieldsRaw{
			StandardFields: []AISuggestStandardFieldRaw{{Key: "模型自造键", Enable: true, Sample: text}},
		}, text, dict)
		if len(got.StandardFields) != 0 {
			t.Fatalf("non-contract key must be dropped, got %+v", got.StandardFields)
		}
		if len(got.Skipped) != 1 || got.Skipped[0].Name != "模型自造键" {
			t.Fatalf("expected one skip entry naming the offending key, got %+v", got.Skipped)
		}
		if got.Skipped[0].Reason == "" {
			t.Fatalf("skip entry must carry a human-readable reason")
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
	if hu.Regex != "" || hu.RegexStatus != regexRejectNotStandard {
		t.Fatalf("custom field regex must be rejected as not-standard: %+v", hu)
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
// 前端只能按"有没有正则"重新猜，于是自定义字段明明该报
// rejected_not_standard，却显示成信息量为零的 rejected_empty。
// 这条测试锁定「结论必须跟着值流到最终响应」。
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
	// 自定义中文 key：必须报 not_standard，而不是被抹平成 rejected_empty
	if byName["车牌号"].RegexStatus != regexRejectNotStandard {
		t.Fatalf("车牌号 should report rejected_not_standard, got %q", byName["车牌号"].RegexStatus)
	}
	if byName["车牌号"].RegexReason == "" {
		t.Fatalf("车牌号 must carry a diagnostic reason")
	}
	// 标准键但补洞器不支持：同样报 not_standard，但原因文案必须与自定义字段区分开
	if byName["totalAmount"].RegexStatus != regexRejectNotStandard {
		t.Fatalf("totalAmount should report rejected_not_standard, got %q", byName["totalAmount"].RegexStatus)
	}
	if !strings.Contains(byName["totalAmount"].RegexReason, "标准字段") {
		t.Fatalf("totalAmount is a standard field; reason must say so, got %q", byName["totalAmount"].RegexReason)
	}
	if !strings.Contains(byName["车牌号"].RegexReason, "自定义扩展字段") {
		t.Fatalf("车牌号 is a custom key; reason must say so, got %q", byName["车牌号"].RegexReason)
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

	// 模型给的 key 与契约键冲突：必须换一个，不能覆盖标准字段
	conflict := postProcessAISuggestFields(&AISuggestFieldsRaw{
		CustomFields: []AISuggestCustomFieldRaw{{Key: "total_amount", Name: "含税总额"}},
	}, "含税总额", dict)
	if len(conflict.CustomFields) != 1 {
		t.Fatalf("conflicting-key field must still be kept, got %+v", conflict.CustomFields)
	}
	if conflict.CustomFields[0].Key == "total_amount" {
		t.Fatalf("custom key must not collide with contract key total_amount")
	}

	// 真正同名的字段仍必须去重（不能因为修 bug 就把去重整个去掉）
	dup := postProcessAISuggestFields(&AISuggestFieldsRaw{
		CustomFields: []AISuggestCustomFieldRaw{{Name: "户号"}, {Name: "户号"}},
	}, "户号", dict)
	if len(dup.CustomFields) != 1 {
		t.Fatalf("exact duplicate names must still be deduped, got %+v", dup.CustomFields)
	}
}
