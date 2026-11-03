package service

import (
	"strings"
	"testing"
)

// 保存时重算 extract_issues（第 26 轮 T13a）。
//
// 背景：用户在编辑抽屉里补齐了缺失字段并保存，但顶部橙色「字段格式需核对」
// 还挂着 —— 因为 issues 只在**重新提取**时才重算。保存路径必须同步刷新，
// 否则「补齐了还报错」会让用户以为没保存成功。

func TestRevalidateIssuesClearsWhenFieldsComplete(t *testing.T) {
	meta := &InvoiceCustomMetadata{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo:   "26507000000149755376",
			InvoiceDate: "2026-09-02",
			Amount:      stateF64(948.26),
			Tax:         stateF64(85.34),
			TotalAmount: stateF64(1033.60),
			TaxRate:     stateF64(0.09),
			BuyerName:   "重庆星达铜业有限公司",
			SellerName:  "重庆璧山天然气有限责任公司",
			Issuer:      "璧山1",
		}},
		// 历史残留的虚假告警
		ExtractIssues: []ExtractFieldIssue{
			{Index: -1, Field: "invoiceNo", Kind: "missing", Severity: "warn",
				Detail: "原文共 2 张发票锚点，仅提取到 1 张，另有 1 张未能提取（可能版式异常或扫描不清），建议复核源文件"},
		},
	}
	got := RevalidateInvoiceIssues(meta)
	if len(got) != 0 {
		t.Fatalf("字段完整且仅剩锚点类告警时应清空，实际仍有 %d 条: %+v", len(got), got)
	}
}

func TestRevalidateIssuesKeepsRealProblems(t *testing.T) {
	// 真的缺字段时不能一并清空 —— 否则「补录提醒」就失效了
	meta := &InvoiceCustomMetadata{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo: "",
			Amount:    stateF64(100),
			// 缺日期 / 购销方
		}},
	}
	got := RevalidateInvoiceIssues(meta)
	if len(got) == 0 {
		t.Fatal("字段缺失时必须保留告警")
	}
	found := false
	for _, it := range got {
		// 校验器用的是 snake_case 字段名（invoice_no），不是前端 camelCase
		if strings.Contains(it.Field, "invoice_no") || strings.Contains(it.Field, "invoice") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应报出发票号缺失，实际: %+v", got)
	}
}

func TestRevalidateIssuesDropsAnchorIssue(t *testing.T) {
	// 锚点类告警需要原文才能判定，保存路径没有原文 → 必须剔除，
	// 否则「用户补齐字段后仍挂着锚点警告」永远清不掉。
	meta := &InvoiceCustomMetadata{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo: "1", InvoiceDate: "2026-01-01",
			Amount: stateF64(100), TotalAmount: stateF64(103),
			BuyerName: "甲", SellerName: "乙",
		}},
	}
	got := RevalidateInvoiceIssues(meta)
	for _, it := range got {
		if strings.Contains(it.Detail, "张发票锚点") {
			t.Fatalf("保存路径不应保留锚点类告警: %+v", it)
		}
	}
}

func TestRevalidateIssuesNoopOnEmpty(t *testing.T) {
	if got := RevalidateInvoiceIssues(nil); got != nil {
		t.Fatalf("nil meta 应返回 nil，实际 %+v", got)
	}
	if got := RevalidateInvoiceIssues(&InvoiceCustomMetadata{Kind: "invoice"}); got != nil {
		t.Fatalf("无发票行时应返回 nil，实际 %+v", got)
	}
}

func TestRevalidateIssuesNormalizesDate(t *testing.T) {
	// 校验器会原地归一化日期（2026年09月02日 → 2026-09-02），
	// 保存路径必须把归一后的值写回 meta，否则库里还是旧格式。
	meta := &InvoiceCustomMetadata{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo: "1", InvoiceDate: "2026年09月02日",
			Amount: stateF64(100), TotalAmount: stateF64(103),
			BuyerName: "甲", SellerName: "乙",
		}},
	}
	RevalidateInvoiceIssues(meta)
	if meta.Invoices[0].InvoiceDate != "2026-09-02" {
		t.Fatalf("日期应被归一化写回，实际 %q", meta.Invoices[0].InvoiceDate)
	}
}
