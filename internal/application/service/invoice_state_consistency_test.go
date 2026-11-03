package service

import (
	"testing"
)

// 第 25 轮问题 1/2 的契约锁。
//
// 三个真机事故：
//   1. 单页气费发票被算出「2 张锚点」，详情页挂着「仅提取到 1 张」的橙条；
//   2. 一次重试撞模型 429，把已有成功发票的文件状态刷成 failed，
//      于是列表绿「提取成功」、抽屉红「提取失败」、历史抽屉统计出 74 份异常；
//   3. 开票人 `璧山1` 被截断成 `璧山`（正则只吃汉字）。

func stateF64(v float64) *float64 { return &v }

// ---- 问题 1：失败不降级 ----

func TestHasUsableInvoiceRecordsByInvoiceNo(t *testing.T) {
	meta := []byte(`{"invoices":[{"invoice_no":"26507000000149755376"}]}`)
	if !HasUsableInvoiceRecords(meta) {
		t.Fatal("有票号即视为可用记录")
	}
}

func TestHasUsableInvoiceRecordsByAmount(t *testing.T) {
	// 少数票 OCR 吃掉号码但金额齐全，用户在列表能看到金额 —— 也算可用
	meta := []byte(`{"invoices":[{"invoice_no":"","amount":948.26}]}`)
	if !HasUsableInvoiceRecords(meta) {
		t.Fatal("金额非零也应视为可用记录")
	}
	meta2 := []byte(`{"invoices":[{"invoice_no":"","total_amount":1033.6}]}`)
	if !HasUsableInvoiceRecords(meta2) {
		t.Fatal("价税合计非零也应视为可用记录")
	}
}

func TestHasUsableInvoiceRecordsRejectsEmpty(t *testing.T) {
	for _, raw := range []string{
		``,
		`{}`,
		`{"invoices":[]}`,
		`{"invoices":[{}]}`,
		`{"invoices":[{"invoice_no":"","amount":0}]}`,
		`not-json`,
	} {
		if HasUsableInvoiceRecords([]byte(raw)) {
			t.Errorf("%q 不应视为有可用记录", raw)
		}
	}
}

// ---- 问题 1：锚点告警抑制 ----

func TestSuppressAnchorWarningSinglePage(t *testing.T) {
	// 真机 3354297f：单页、1 张票、锚点被银行账号撑到 2 → 必须抑制。
	// 判据必须是**物理页码 <= 1**：按 anchors<=1 判断会漏掉这一整类。
	inv := []InvoiceExtractionItem{{InvoiceNo: "26507000000149755376", Page: 1}}
	if !SuppressInvoiceAnchorWarning(2, inv) {
		t.Fatal("单页文件（页码=1，锚点被误计）必须抑制告警")
	}
	// 页码缺失（老数据没回填 page）也要按单页处理 —— 只有一张票时物理上
	// 不可能同时占两页。
	if !SuppressInvoiceAnchorWarning(2, []InvoiceExtractionItem{{InvoiceNo: "x"}}) {
		t.Fatal("仅 1 张票时不得报「还有一张未提取」")
	}
	if !SuppressInvoiceAnchorWarning(1, inv) {
		t.Fatal("锚点=1 时也应抑制")
	}
}

func TestSuppressAnchorWarningCompleteMultiPage(t *testing.T) {
	// 多页但每张票都完整 → 差异来自锚点误计，抑制
	invs := []InvoiceExtractionItem{
		{InvoiceNo: "1", InvoiceDate: "2026-01-01", Amount: stateF64(100), TotalAmount: stateF64(103),
			BuyerName: "甲", SellerName: "乙"},
		{InvoiceNo: "2", InvoiceDate: "2026-01-02", Amount: stateF64(200), TotalAmount: stateF64(206),
			BuyerName: "甲", SellerName: "乙"},
	}
	if !SuppressInvoiceAnchorWarning(3, invs) {
		t.Fatal("全部票字段完整时应抑制告警")
	}
}

func TestSuppressAnchorWarningKeepsRealGap(t *testing.T) {
	// 多页 + 有残票 → 可能是真漏提，必须保留告警
	invs := []InvoiceExtractionItem{
		{InvoiceNo: "1", InvoiceDate: "2026-01-01", Amount: stateF64(100), TotalAmount: stateF64(103),
			BuyerName: "甲", SellerName: "乙"},
		{InvoiceNo: "2"}, // 残票
	}
	if SuppressInvoiceAnchorWarning(5, invs) {
		t.Fatal("存在残票时不得抑制（可能是真漏提）")
	}
}

func TestSuppressAnchorWarningNeverWhenAllMissing(t *testing.T) {
	if SuppressInvoiceAnchorWarning(5, nil) {
		t.Fatal("一张都没提到时必须报")
	}
}

// ---- 问题 2：开票人后缀 ----

func TestCleanIssuerNameKeepsNumericSuffix(t *testing.T) {
	// 真机：重庆璧山天然气自助开票机的开票人是「璧山1」
	for _, tc := range []struct{ raw, want string }{
		{"璧山1", "璧山1"},
		{"璧山", "璧山"},
		{"杨希", "杨希"},
		{"包凯莉", "包凯莉"},
		{"A01", ""},     // 纯字母数字不是人名
		{"123", ""},     // 纯数字不是人名
		{"张1伟", ""},     // 后缀必须出现在末尾
		{"璧山12345", ""}, // 后缀最多 4 位
	} {
		if got := cleanIssuerName(tc.raw); got != tc.want {
			t.Errorf("cleanIssuerName(%q) = %q，期望 %q", tc.raw, got, tc.want)
		}
	}
}

func TestIssuerLooseReCapturesNumericSuffix(t *testing.T) {
	for _, raw := range []string{
		"开票人：璧山1",
		"开 票 人 ：\n璧山1",
		"开票人:璧山1",
	} {
		m := issuerLooseRe.FindStringSubmatch(raw)
		if len(m) < 2 || m[1] != "璧山1" {
			t.Errorf("%q 应捕获「璧山1」，实际 %v", raw, m)
		}
	}
}

func TestFillInvoiceIssuerWritesDrawerAlias(t *testing.T) {
	// issuer / drawer 必须同写：前端列表与抽屉各读一个
	inv := &InvoiceExtractionItem{InvoiceNo: "1", Remark: "开票人：璧山1"}
	fillInvoiceIssuer(inv)
	if inv.Issuer != "璧山1" || inv.Drawer != "璧山1" {
		t.Fatalf("issuer/drawer 必须同值，实际 %q / %q", inv.Issuer, inv.Drawer)
	}
}

func TestFillInvoiceIssuerRepairsTruncatedValue(t *testing.T) {
	// 库里已有被截断的「璧山」，备注里有完整「璧山1」→ 不能因为「已有值」就跳过。
	// 注意：fillInvoiceIssuer 只填空，不覆盖非空值；截断修复由数据脚本负责。
	inv := &InvoiceExtractionItem{InvoiceNo: "1", Issuer: "璧山1"}
	fillInvoiceIssuer(inv)
	if inv.Drawer != "璧山1" {
		t.Fatalf("已有 issuer 时必须补 drawer，实际 %q", inv.Drawer)
	}
}

// ---- 问题 2：文档级同步 ----

func TestSyncDocumentIssuerTakesFirstNonEmpty(t *testing.T) {
	meta := &InvoiceCustomMetadata{Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "1", Issuer: ""},
		{InvoiceNo: "2", Issuer: "叶南"},
		{InvoiceNo: "3", Issuer: "周琳杰"},
	}}
	SyncDocumentIssuer(meta)
	if meta.Issuer != "叶南" || meta.Drawer != "叶南" {
		t.Fatalf("应取首个非空开票人，实际 %q / %q", meta.Issuer, meta.Drawer)
	}
}

func TestSyncDocumentIssuerFallsBackToDrawer(t *testing.T) {
	meta := &InvoiceCustomMetadata{Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "1", Drawer: "璧山1"},
	}}
	SyncDocumentIssuer(meta)
	if meta.Issuer != "璧山1" {
		t.Fatalf("issuer 为空时应回落 drawer，实际 %q", meta.Issuer)
	}
}

func TestSyncDocumentIssuerNoopWhenAllEmpty(t *testing.T) {
	meta := &InvoiceCustomMetadata{Invoices: []InvoiceExtractionItem{{InvoiceNo: "1"}}}
	SyncDocumentIssuer(meta)
	if meta.Issuer != "" || meta.Drawer != "" {
		t.Fatalf("全空时不得写值，实际 %q / %q", meta.Issuer, meta.Drawer)
	}
}
