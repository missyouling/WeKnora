package service

import (
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
