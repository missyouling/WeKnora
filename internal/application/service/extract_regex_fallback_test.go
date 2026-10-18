package service

import (
	"math"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// Phase B：后置字段级正则补洞器的测试。
// 命名以 TestExtract 开头，确保被 Daily-Affairs CI 白名单正则
// (-run "TestBusiness|TestExtract|TestDailyAffairs|TestGolden") 覆盖到。
//
// 定位：LLM（含自愈）跑完之后，对**仍为空**的字段，用确定性正则在**本批原文**里
// 回填。作用域限定单批，且只在"唯一明确命中"时回填——多张票混在一批时宁可空着，
// 也不能把 A 票的号码写到 B 票上（历史上踩过"张冠李戴"）。

func TestExtractRegexFallbackFillsMissingInvoiceNoFromLabel(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{InvoiceNo: ""}},
	}
	text := "重庆增值税电子普通发票\n发票代码: 050002400112\n发票号码: 26507904700100036197\n"
	patches := RegexFallbackInvoiceFields(res, text, nil)

	if got := res.Invoices[0].InvoiceNo; got != "26507904700100036197" {
		t.Fatalf("invoice_no 应被回填为 20 位全电票号，实际 %q", got)
	}
	if len(patches) == 0 {
		t.Fatal("应记录补洞来源")
	}
	if patches[0].Field != "invoice_no" || patches[0].Source == "" {
		t.Errorf("补洞记录异常: %+v", patches[0])
	}
	// 结果级溯源：前端据此展示"该字段由正则补齐"。
	// 文本里还有「发票代码」且同样为空，会一并补齐——按固定字段顺序排列。
	if len(res.PatchedByRegex) != 2 || res.PatchedByRegex[0] != "invoice_no" || res.PatchedByRegex[1] != "invoice_code" {
		t.Errorf("PatchedByRegex = %v, want [invoice_no invoice_code]", res.PatchedByRegex)
	}
}

func TestExtractRegexFallbackFillsFromBareTwentyDigit(t *testing.T) {
	// 票面只印了号码、没有「发票号码」标签时，靠 20 位全电票号形状兜底。
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "机器编号 499098302394\n全电票号 26507904700100036197\n"
	RegexFallbackInvoiceFields(res, text, nil)
	if got := res.Invoices[0].InvoiceNo; got != "26507904700100036197" {
		t.Fatalf("应按 20 位形状回填，实际 %q", got)
	}
}

func TestExtractRegexFallbackDoesNotOverwriteExisting(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{InvoiceNo: "64869484"}},
	}
	text := "发票号码: 26507904700100036197"
	patches := RegexFallbackInvoiceFields(res, text, nil)
	if res.Invoices[0].InvoiceNo != "64869484" {
		t.Errorf("已有值不得被覆盖，实际 %q", res.Invoices[0].InvoiceNo)
	}
	if len(patches) != 0 {
		t.Errorf("无空缺时不应产生补洞记录: %+v", patches)
	}
}

func TestExtractRegexFallbackSkipsAmbiguousMatches(t *testing.T) {
	// 同批两张票的号码都在文本里 → 唯一性不成立，禁止按序猜（防串号）。
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{}, {}},
	}
	text := "发票号码: 11111111\n发票号码: 22222222\n"
	patches := RegexFallbackInvoiceFields(res, text, nil)
	if len(patches) != 0 {
		t.Errorf("多值命中时不得回填: %+v", patches)
	}
	for i, inv := range res.Invoices {
		if inv.InvoiceNo != "" {
			t.Errorf("第 %d 张不应被回填: %q", i+1, inv.InvoiceNo)
		}
	}
}

func TestExtractRegexFallbackUsesConfiguredRegex(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "invoice_no", Regex: `票号[-:：]\s*([0-9]{6,20})`, Enabled: true},
		},
	}
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "本票据 票号-987654321 请妥善保管"
	patches := RegexFallbackInvoiceFields(res, text, cfg)
	if got := res.Invoices[0].InvoiceNo; got != "987654321" {
		t.Fatalf("应使用用户配置的正则并取捕获组，实际 %q", got)
	}
	if len(patches) != 1 || patches[0].Source != "config" {
		t.Errorf("应标记来源为 config: %+v", patches)
	}
}

func TestExtractRegexFallbackNormalizesInvoiceDate(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "开票日期: 2025年11月04日"
	RegexFallbackInvoiceFields(res, text, nil)
	if got := res.Invoices[0].InvoiceDate; got != "2025-11-04" {
		t.Fatalf("日期应归一化为 2025-11-04，实际 %q", got)
	}
}

func TestExtractRegexFallbackIgnoresNotInvoice(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "not_invoice"}
	patches := RegexFallbackInvoiceFields(res, "发票号码: 26507904700100036197", nil)
	if len(patches) != 0 {
		t.Errorf("非发票结果不应补洞: %+v", patches)
	}
}

// 端到端语义：校验器判定的 error（号码为空）应能被后置补洞消除——
// 这正是"提取成功但号码为空也能入库"这条隐性质量问题的修复点。
func TestExtractRegexFallbackClearsValidationError(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{TotalAmount: f64(101.72)}},
	}
	before := validateInvoiceExtraction(res, nil)
	if countIssues(before, issueSeverityError) == 0 {
		t.Fatalf("前置条件不成立：号码为空应产生 error 级问题，实际 %+v", before)
	}

	RegexFallbackInvoiceFields(res, "发票号码: 26507904700100036197", nil)
	after := validateInvoiceExtraction(res, nil)
	if countIssues(after, issueSeverityError) >= countIssues(before, issueSeverityError) {
		t.Fatalf("补洞后 error 应减少：before=%d after=%d",
			countIssues(before, issueSeverityError), countIssues(after, issueSeverityError))
	}
}

// ---- 2026-10-18 第二轮：正则补洞从 8 个文本键扩到「全部标准 + 扩展 + 明细子项」----
//
// 用户口径：只要配了合法 RE2 且在本批原文唯一命中，正式提取就要执行回填；
// 数值字段必须走统一清洗（货币符/千分位）再转 float64，转不了就放弃，
// 多行明细无法唯一归属时一律不猜。

func TestExtractRegexFallbackPatchesNumericStandardFields(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "价税合计: ¥1,234.56\n税额：¥101.94\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "totalAmount", Enabled: true, Regex: `价税合计\s*[:：]?\s*[¥￥]?\s*([\d,]+\.\d{2})`},
		{Name: "tax", Enabled: true, Regex: `税额\s*[:：]?\s*[¥￥]?\s*([\d,]+\.\d{2})`},
	}}
	patches := RegexFallbackInvoiceFields(res, text, cfg)

	got := res.Invoices[0]
	if got.TotalAmount == nil || math.Abs(*got.TotalAmount-1234.56) > 0.001 {
		t.Fatalf("total_amount 应被清洗千分位并回填 1234.56，实际 %v", got.TotalAmount)
	}
	if got.Tax == nil || math.Abs(*got.Tax-101.94) > 0.001 {
		t.Fatalf("tax 应回填 101.94，实际 %v", got.Tax)
	}
	if len(patches) != 2 {
		t.Fatalf("应记录 2 条补洞，实际 %d: %+v", len(patches), patches)
	}
}

func TestExtractRegexFallbackNumericSkipUnparsable(t *testing.T) {
	// 捕获到「壹仟贰佰」这类不能转数值的内容时，宁可留空也不写 0。
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "价税合计: 壹仟贰佰元整\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "totalAmount", Enabled: true, Regex: `价税合计\s*[:：]?\s*(.{1,12})`},
	}}
	RegexFallbackInvoiceFields(res, text, cfg)
	if res.Invoices[0].TotalAmount != nil {
		t.Fatalf("不可解析的金额必须放弃，实际 %v", *res.Invoices[0].TotalAmount)
	}
}

func TestExtractRegexFallbackPatchesCustomFieldBucket(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "户号: 5001234567\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "account_no", Enabled: true, Regex: `户号\s*[:：]?\s*(\d{7,12})`},
	}}
	patches := RegexFallbackInvoiceFields(res, text, cfg)

	fields := res.Invoices[0].Fields
	if fields == nil || fields["account_no"] != "5001234567" {
		t.Fatalf("自定义扩展字段应写入 Fields 桶，实际 %v", fields)
	}
	if len(patches) != 1 || patches[0].Field != "account_no" {
		t.Fatalf("补洞记录异常: %+v", patches)
	}
}

func TestExtractRegexFallbackPatchesSubFieldOnSingleItemRow(t *testing.T) {
	// 标准子项（spec/unit/qty…）必须落结构体字段，否则前端明细列读不到；
	// 自定义子项才落 Extra。两条路径都验。
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{Items: []InvoiceExtractionLine{{Name: "钢材"}}},
	}}
	text := "项目名称: 钢材\n规格型号: Q235B\n批次号: B2024-77\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "items", Type: "items", Enabled: true, SubFields: []types.FleetItemSubField{
			{Key: "spec", Label: "规格型号", Regex: `规格型号\s*[:：]?\s*([A-Za-z0-9]+)`},
			{Key: "batch_no", Label: "批次号", Regex: `批次号\s*[:：]?\s*([A-Za-z0-9-]+)`},
		}},
	}}
	patches := RegexFallbackInvoiceFields(res, text, cfg)

	line := res.Invoices[0].Items[0]
	if line.Spec != "Q235B" {
		t.Fatalf("标准子项 spec 应写入结构体字段，实际 %q", line.Spec)
	}
	if line.Extra == nil || line.Extra["batch_no"] != "B2024-77" {
		t.Fatalf("自定义子项应写入 Extra，实际 %v", line.Extra)
	}
	if len(patches) != 2 {
		t.Fatalf("应有两条子项补洞记录，实际 %+v", patches)
	}
	if patches[0].Field != "items.spec" || patches[1].Field != "items.batch_no" {
		t.Fatalf("子项补洞记录异常: %+v", patches)
	}
}

func TestExtractRegexFallbackSkipsSubFieldWithMultipleItemRows(t *testing.T) {
	// 两行明细无法判断该值属于哪一行——按序猜测就是串号。
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{Items: []InvoiceExtractionLine{{Name: "钢材"}, {Name: "铜板"}}},
	}}
	text := "规格型号: Q235B\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "items", Type: "items", Enabled: true, SubFields: []types.FleetItemSubField{
			{Key: "spec", Label: "规格型号", Regex: `规格型号\s*[:：]?\s*([A-Za-z0-9]+)`},
		}},
	}}
	patches := RegexFallbackInvoiceFields(res, text, cfg)
	for _, p := range patches {
		if strings.HasPrefix(p.Field, "items.") {
			t.Fatalf("多行明细不得补子项，实际 %+v", p)
		}
	}
	for _, line := range res.Invoices[0].Items {
		if line.Spec != "" || line.Extra["spec"] != nil {
			t.Fatalf("多行明细的子项必须保持为空，实际 Spec=%q Extra=%v", line.Spec, line.Extra)
		}
	}
}

func TestExtractRegexFallbackPatchesTextStandardField(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{}}}
	text := "备注: 车牌号 渝A12345 通行\n"
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "remark", Enabled: true, Regex: `备注\s*[:：]?\s*(.+?)\s*$`},
	}}
	patches := RegexFallbackInvoiceFields(res, text, cfg)
	if res.Invoices[0].Remark != "车牌号 渝A12345 通行" {
		t.Fatalf("remark 应被回填，实际 %q（patches=%+v）", res.Invoices[0].Remark, patches)
	}
}
