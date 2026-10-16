package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
)

// 提取质量闭环测试（确定性，不调真实模型）。
// 覆盖：日期归一化、必填缺失判定、金额交叉校验、自愈择优。

func TestExtractValidateDateNormalization(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"2026-09-02", "2026-09-02", true},
		{"2026/9/2", "2026-09-02", true},
		{"2026.09.02", "2026-09-02", true},
		{"20260902", "2026-09-02", true},
		{"2026年09月02日", "2026-09-02", true},
		{"2026-9-2", "2026-09-02", true},
		{"开票日期见票面", "开票日期见票面", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeDateString(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeDateString(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func validateFloatPtr(f float64) *float64 { return &f }

func TestExtractValidateMissingRequired(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{
			{InvoiceDate: "2026年09月02日"}, // 缺发票号与金额
		},
	}
	issues := validateInvoiceExtraction(res, nil)
	if got := countIssues(issues, issueSeverityError); got != 2 {
		t.Errorf("error 级问题应为 2（缺标识 + 缺金额），实际 %d: %+v", got, issues)
	}
	// 日期应被就地归一化
	if res.Invoices[0].InvoiceDate != "2026-09-02" {
		t.Errorf("日期应被归一化为 2026-09-02，实际 %q", res.Invoices[0].InvoiceDate)
	}
}

func TestExtractValidateAmountMismatch(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo:   "12345678",
			InvoiceDate: "2026-09-02",
			Amount:      validateFloatPtr(100),
			Tax:         validateFloatPtr(13),
			TotalAmount: validateFloatPtr(200), // 100+13 ≠ 200
			Items: []InvoiceExtractionLine{
				{Name: "A", Amount: validateFloatPtr(60)},
				{Name: "B", Amount: validateFloatPtr(30)}, // 合计 90 ≠ 100
			},
		}},
	}
	issues := validateInvoiceExtraction(res, nil)
	if got := countIssues(issues, issueSeverityError); got != 0 {
		t.Errorf("标识与金额齐全时不应有 error 级问题，实际 %+v", issues)
	}
	// 两条交叉校验告警：明细合计 vs 不含税金额、金额+税额 vs 价税合计
	var mismatch int
	for _, it := range issues {
		if it.Kind == "mismatch" {
			mismatch++
		}
	}
	if mismatch != 2 {
		t.Errorf("应报 2 条金额不一致，实际 %d: %+v", mismatch, issues)
	}
}

func TestExtractValidateSkipsNonInvoice(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "not an invoice"}
	if issues := validateInvoiceExtraction(res, nil); len(issues) != 0 {
		t.Errorf("非发票文档不应产生校验问题，实际 %+v", issues)
	}
}

func TestExtractRepairAdoptsBetterResult(t *testing.T) {
	prev := `{"kind":"invoice","invoices":[{"invoice_date":"2026-09-02"}]}`
	fixed := `{"kind":"invoice","invoices":[{"invoice_no":"12345678","invoice_date":"2026-09-02","total_amount":113}]}`
	m := &fakeRetryChat{nonStreamOut: []string{fixed}}
	before := []ExtractFieldIssue{
		{Index: 0, Field: "invoice_no", Kind: "missing", Detail: "缺少发票号码", Severity: issueSeverityError},
		{Index: 0, Field: "total_amount", Kind: "missing", Detail: "缺少金额", Severity: issueSeverityError},
	}
	got, gotIssues, err := repairInvoiceExtraction(context.Background(), m,
		[]chat.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "doc"}},
		&chat.ChatOptions{}, nil, nil, prev, before)
	if err != nil {
		t.Fatalf("repair 不应失败: %v", err)
	}
	if len(got.Invoices) != 1 || got.Invoices[0].InvoiceNo != "12345678" {
		t.Fatalf("修正结果异常: %+v", got.Invoices)
	}
	if countIssues(gotIssues, issueSeverityError) >= countIssues(before, issueSeverityError) {
		t.Errorf("修正后 error 级问题应减少: before=%d after=%d",
			countIssues(before, issueSeverityError), countIssues(gotIssues, issueSeverityError))
	}
}

func TestExtractRepairDoesNotWorsen(t *testing.T) {
	// 模型修不出名堂（仍缺发票号）→ 调用方应据 countIssues 判断不采用。
	// 这里锁定 repair 本身不抛错、且如实返回问题清单，由主流程择优。
	prev := `{"kind":"invoice","invoices":[{"invoice_date":"2026-09-02"}]}`
	m := &fakeRetryChat{nonStreamOut: []string{prev}}
	before := []ExtractFieldIssue{
		{Index: 0, Field: "invoice_no", Kind: "missing", Detail: "缺少发票号码", Severity: issueSeverityError},
	}
	_, gotIssues, err := repairInvoiceExtraction(context.Background(), m,
		[]chat.Message{{Role: "system", Content: "sys"}}, &chat.ChatOptions{}, nil, nil, prev, before)
	if err != nil {
		t.Fatalf("repair 不应失败: %v", err)
	}
	if countIssues(gotIssues, issueSeverityError) < countIssues(before, issueSeverityError) {
		t.Error("未改善时不应被判为更好")
	}
}

func TestExtractRepairTruncatesLongPrevious(t *testing.T) {
	// 构造超长 previousRaw，确认不会把 8k 以上内容原样回灌（避免顶到上下文上限）
	long := `{"kind":"invoice","invoices":[{"remark":"` + validateRepeatString("x", 20000) + `"}]}`
	m := &fakeRetryChat{nonStreamOut: []string{`{"kind":"invoice","invoices":[]}`}}
	_, _, err := repairInvoiceExtraction(context.Background(), m,
		[]chat.Message{{Role: "system", Content: "sys"}}, &chat.ChatOptions{}, nil, nil,
		long, []ExtractFieldIssue{{Index: 0, Field: "invoice_no", Kind: "missing", Severity: issueSeverityError}})
	if err != nil {
		t.Fatalf("超长 previousRaw 不应导致失败: %v", err)
	}
	var assistantLen int
	for _, msg := range m.lastMessages {
		if msg.Role == "assistant" {
			assistantLen = len([]rune(msg.Content))
		}
	}
	if assistantLen > 8100 {
		t.Errorf("回灌的 assistant 轮次应被截断到 8k 以内，实际 %d 字符", assistantLen)
	}
}

func validateRepeatString(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
