package handler

import (
	"regexp"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// ---- 明细子列去重 ----

func TestBusinessDedupeItemSubFieldsAliasKeys(t *testing.T) {
	// 通行费发票实测形态：税额同时有 tax_amount 与 tax，单价有 price 与 unit_price
	in := []types.FleetItemSubField{
		{Key: "name", Label: "项目名称"},
		{Key: "price", Label: "单价"},
		{Key: "unit_price", Label: "单价(元)"},
		{Key: "tax_amount", Label: "税额"},
		{Key: "tax", Label: "税额"},
		{Key: "plate_no", Label: "车牌号"},
	}
	out, changed := dedupeItemSubFields(in)
	if !changed {
		t.Fatal("存在同义子项时必须返回 changed")
	}
	keys := make([]string, 0, len(out))
	for _, s := range out {
		keys = append(keys, s.Key)
	}
	if len(out) != 4 {
		t.Fatalf("应去重为 4 条，实际 %d：%v", len(out), keys)
	}
	for _, want := range []string{"name", "price", "tax_amount", "plate_no"} {
		found := false
		for _, k := range keys {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("应保留标准键 %s，实际 %v", want, keys)
		}
	}
	// 标准键优先：tax 被 tax_amount 顶掉，unit_price 被 price 顶掉
	for _, dropped := range []string{"tax", "unit_price"} {
		for _, k := range keys {
			if k == dropped {
				t.Fatalf("同义非标准键 %s 应被丢弃，实际 %v", dropped, keys)
			}
		}
	}
	// 顺序按首次出现位置：name, price, tax_amount, plate_no
	want := []string{"name", "price", "tax_amount", "plate_no"}
	for i := range want {
		if out[i].Key != want[i] {
			t.Fatalf("顺序应保持首次出现：want %v, got %v", want, keys)
		}
	}
}

func TestBusinessDedupeItemSubFieldsByLabel(t *testing.T) {
	// 不同键但同列头（「备注」两列）在浮层里无法区分 → 只留第一条
	in := []types.FleetItemSubField{
		{Key: "batch_no", Label: "备注"},
		{Key: "memo", Label: "备注 "},
		{Key: "line_no", Label: "行号"},
	}
	out, changed := dedupeItemSubFields(in)
	if !changed || len(out) != 2 {
		t.Fatalf("同列头应去重为 2 条，实际 %d: %+v", len(out), out)
	}
	if out[0].Key != "batch_no" {
		t.Fatalf("应保留先出现的一条，实际 %s", out[0].Key)
	}
}

func TestBusinessDedupeItemSubFieldsIdempotent(t *testing.T) {
	clean := []types.FleetItemSubField{
		{Key: "name", Label: "项目名称"},
		{Key: "spec", Label: "规格型号"},
		{Key: "amount", Label: "金额"},
	}
	out, changed := dedupeItemSubFields(clean)
	if changed {
		t.Fatal("无重复时不应报告 changed（避免每次启动都写库）")
	}
	if len(out) != len(clean) {
		t.Fatalf("无重复时不得丢条目，实际 %d", len(out))
	}
}

func TestBusinessNormalizeInvoiceCategorySubsScopeGuard(t *testing.T) {
	subs := []types.FleetCategorySub{
		{Name: "items", DataType: "items", SubFields: []types.FleetItemSubField{
			{Key: "tax", Label: "税额"}, {Key: "tax_amount", Label: "税额"},
		}},
	}
	// 非发票 scope 不动
	if out, changed := normalizeInvoiceCategorySubs("contract", subs); changed || len(out[0].SubFields) != 2 {
		t.Fatal("非发票 scope 不应去重")
	}
	out, changed := normalizeInvoiceCategorySubs(types.FleetCategoryScopeInvoice, subs)
	if !changed || len(out[0].SubFields) != 1 {
		t.Fatalf("发票 scope 应去重为 1 条，实际 %d", len(out[0].SubFields))
	}
}

// ---- 发票类型模板匹配（补全前校验）----

func TestBusinessMatchInvoiceRuleTemplateTypeGuard(t *testing.T) {
	standardSubs := []types.FleetCategorySub{
		{Name: "invoiceNo"}, {Name: "invoiceDate"}, {Name: "totalAmount"}, {Name: "taxRate"},
	}
	tollSubs := []types.FleetCategorySub{
		{Name: "invoiceNo"}, {Name: "totalAmount"}, {Name: "车牌号"}, {Name: "amount"},
	}

	cases := []struct {
		name    string
		subs    []types.FleetCategorySub
		wantKey string // 空串=不匹配
	}{
		{"通行费发票", tollSubs, "toll"},
		{"通行费", tollSubs, "toll"},
		{"水电发票", standardSubs, "utility"},
		{"电费发票", standardSubs, "utility"},
		{"增值税专用发票", standardSubs, "vat"},
		{"普通发票", standardSubs, "standard"},
		{"电子发票(普通)", standardSubs, "standard"},
		{"三联收据", []types.FleetCategorySub{{Name: "收据编号"}, {Name: "合计金额"}}, "misc"},
		// 名字像通行费，但字段是普通发票那套（没有车牌号）→ 必须判为不匹配
		{"通行费发票", standardSubs, ""},
		// 名字完全对不上任何模板
		{"测试分类", standardSubs, ""},
	}
	for _, c := range cases {
		tpl := matchInvoiceRuleTemplate(c.name, c.subs)
		got := ""
		if tpl != nil {
			got = tpl.Key
		}
		if got != c.wantKey {
			t.Fatalf("matchInvoiceRuleTemplate(%q) = %q, want %q", c.name, got, c.wantKey)
		}
	}
}

func TestBusinessDefaultInvoiceRuleFieldsRegexOnlyWhenTypeMatched(t *testing.T) {
	subs := []types.FleetCategorySub{
		{Name: "invoiceNo", Enabled: true, DataType: "text"},
		{Name: "invoiceDate", Enabled: true, DataType: "date"},
		{Name: "totalAmount", Enabled: true, DataType: "number"},
		{Name: "taxRate", Enabled: true, DataType: "number"},
	}
	// 类型匹配（普通发票）→ 号码/日期/金额都有正则
	fields := defaultInvoiceRuleFields("普通发票", subs)
	if len(fields) != len(subs) {
		t.Fatalf("应为每个启用字段产出规则，实际 %d", len(fields))
	}
	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range fields {
		byName[f.Name] = f
	}
	for _, k := range []string{"invoiceNo", "invoiceDate", "totalAmount", "taxRate"} {
		if strings.TrimSpace(byName[k].Regex) == "" {
			t.Fatalf("%s 类型匹配时应补正则，实际为空（%+v）", k, byName[k])
		}
		if strings.TrimSpace(byName[k].Desc) == "" || strings.TrimSpace(byName[k].Rule) == "" {
			t.Fatalf("%s 应同时补 desc/rule", k)
		}
	}

	// 类型不匹配（分类名对不上）→ 只有 desc/rule，没有正则
	unknown := defaultInvoiceRuleFields("测试分类", subs)
	for _, f := range unknown {
		if f.Regex != "" {
			t.Fatalf("类型未匹配时不得给正则，%s = %q", f.Name, f.Regex)
		}
		if strings.TrimSpace(f.Rule) == "" {
			t.Fatalf("%s 仍应有语义规则", f.Name)
		}
	}
}

func TestBusinessInvoiceFieldRegexCompilesAsRE2(t *testing.T) {
	fields := []string{
		"invoiceNo", "invoiceCode", "invoiceDate", "totalAmount", "amount", "tax",
		"taxRate", "buyerTaxNo", "sellerTaxNo", "plate_no", "travel_from", "travel_to",
		"发票号码", "价税合计", "税率", "车牌号",
	}
	for _, f := range fields {
		pat := invoiceFieldRegexFor(f, "通行费发票")
		if pat == "" {
			t.Fatalf("%s 应有正则", f)
		}
		if _, err := regexp.Compile(pat); err != nil {
			t.Fatalf("%s 正则编译失败: %v (%s)", f, err, pat)
		}
		// 只允许一个捕获组（与 uniqueRegexCapture 的 m[1] 约定一致）
		re := regexp.MustCompile(pat)
		if re.NumSubexp() != 1 {
			t.Fatalf("%s 必须恰好一个捕获组，实际 %d: %s", f, re.NumSubexp(), pat)
		}
	}
	// 名称类/备注类字段形状不确定 → 不给正则（避免误命中）
	for _, f := range []string{"buyerName", "sellerName", "issuer", "remark", "items", "fileName", "购买方"} {
		if pat := invoiceFieldRegexFor(f, "普通发票"); pat != "" {
			t.Fatalf("%s 不应生成正则，实际 %q", f, pat)
		}
	}
}

func TestBusinessInvoiceRegexMatchesRealTicketText(t *testing.T) {
	text := `电子发票（普通发票）
发票号码：24312000000012345678
发票代码：050002100111
开票日期：2025年07月07日
价税合计（小写）：¥205.44
税额：16.96
税率：9%
购买方纳税人识别号：91500227MA5U54AJ0A
备注：车牌号 渝A12345 通行日期 2025-07-07
`
	cases := map[string]string{
		"invoiceNo":   "24312000000012345678",
		"invoiceCode": "050002100111",
		"invoiceDate": "2025年07月07日",
		"totalAmount": "205.44",
		"tax":         "16.96",
		"taxRate":     "9",
		"plate_no":    "渝A12345",
		"buyerTaxNo":  "91500227MA5U54AJ0A",
	}
	for field, want := range cases {
		pat := invoiceFieldRegexFor(field, "通行费发票")
		re := regexp.MustCompile(pat)
		got, ok := captureUniqueInTest(text, re)
		if !ok {
			t.Fatalf("%s 未命中（pattern=%s）", field, pat)
		}
		if got != want {
			t.Fatalf("%s 抽取 %q, want %q", field, got, want)
		}
	}
}

// captureUniqueInTest 复刻 service.uniqueRegexCapture 的口径（全批唯一命中 + 优先第 1 组），
// 仅测试用——它不可导出，避免为了测试把内部实现提升为公共 API。
func captureUniqueInTest(text string, re *regexp.Regexp) (string, bool) {
	ms := re.FindAllStringSubmatch(text, -1)
	if len(ms) == 0 {
		return "", false
	}
	seen := map[string]struct{}{}
	last := ""
	for _, m := range ms {
		v := m[0]
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			v = m[1]
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		seen[v] = struct{}{}
		last = v
	}
	return last, len(seen) == 1
}

// ---- 合并：正则只在为空时回填 ----

func TestBusinessMergeInvoiceRuleFieldsFillsRegexOnlyWhenEmpty(t *testing.T) {
	cur := []types.ExtractFieldConfig{
		{Name: "invoiceNo", Desc: "发票号码", Rule: "自定义规则A", Regex: ""},          // 空正则 → 回填
		{Name: "totalAmount", Desc: "价税合计", Rule: "自定义规则B", Regex: `用户自己的正则`}, // 已有 → 保留
	}
	draft := []types.ExtractFieldConfig{
		{Name: "invoiceNo", Desc: "发票号码", Rule: "底稿规则A", Regex: `(?:发票号码)[^0-9A-Za-z]{0,4}([0-9A-Za-z]{6,24})`},
		{Name: "totalAmount", Desc: "价税合计", Rule: "底稿规则B", Regex: `(?:价税合计)([0-9,]+\.[0-9]{2})`},
	}
	out, changed := mergeInvoiceRuleFields(cur, draft, "普通发票")
	if !changed {
		t.Fatal("空正则应被回填并报告 changed")
	}
	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range out {
		byName[f.Name] = f
	}
	if byName["invoiceNo"].Regex != draft[0].Regex {
		t.Fatalf("空正则应回填底稿正则，实际 %q", byName["invoiceNo"].Regex)
	}
	if byName["totalAmount"].Regex != "用户自己的正则" {
		t.Fatalf("用户正则不得被覆盖，实际 %q", byName["totalAmount"].Regex)
	}
	// 用户手写的 rule 也不能被底稿覆盖
	if byName["invoiceNo"].Rule != "自定义规则A" {
		t.Fatalf("用户 rule 不得被覆盖，实际 %q", byName["invoiceNo"].Rule)
	}
	// 幂等：再合并一次不应产生变化
	if _, again := mergeInvoiceRuleFields(out, draft, "普通发票"); again {
		t.Fatal("第二次合并不应再报 changed")
	}
}
