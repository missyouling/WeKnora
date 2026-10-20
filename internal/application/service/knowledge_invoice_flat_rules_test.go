package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 扁平格式确定性覆盖层：LLM 擅长购销方/地址等语义字段，但对「票面上并排的多个
// 数字串」判断不可靠——实测把『机器编号 499098302394』当成了 invoice_no，而真正的
// 『发票号码 64869484』就在同一行附近。规则层用明确标签做确定性覆盖，保证号码准确。
const flatSampleTwoInvoices = `发票代码: 050002400112

# <u>重庆增值税电子普通发票</u>

发票号码: 64869484

||﹊﹋﹌ 机器编号: 499098302394||||||||开票日期: 2025年11月04日 校 验 码: 13353 86372 62628 06587||
|---|---|---|---|---|---|---|---|---|---|---|
*经营租赁*通行费 渝C87567 货车 20251024 20251024 93.32 9% 8.40
499098302394
壹佰零壹元柒角贰分
陈思颖 袁银红 陈思颖
重庆星达铜业有限公司
91500227MA5U54AJ0A

发票代码: 050002400113

发票号码: 58816591

||﹊﹋﹌ 机器编号: 412345678901||||||||开票日期: 2025年11月05日 校 验 码: 11111 22222 33333 44444||
*经营租赁*通行费 渝C87567 货车 20251101 20251102 100.00 9% 9.00
412345678901
壹佰零玖元整
王如媛 李四 王如媛
重庆星达铜业有限公司
91500227MA5U54AJ0A`

func TestOverlayDeterministicInvoiceFieldsUsesLabelNotMachineNo(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{
			{Page: 1, InvoiceNo: "499098302394"}, // LLM 误取机器编号
			{Page: 2, InvoiceNo: "412345678901"}, // 同上
		},
	}
	chunks := []*types.Chunk{{ChunkType: types.ChunkTypeText, Content: flatSampleTwoInvoices}}

	overlaid := OverlayDeterministicInvoiceFields(res, chunks, "")
	if overlaid != 2 {
		t.Fatalf("应覆盖 2 张，实际 %d", overlaid)
	}
	if res.Invoices[0].InvoiceNo != "64869484" {
		t.Errorf("第 1 张发票号 = %q, want 64869484（不得取机器编号）", res.Invoices[0].InvoiceNo)
	}
	if res.Invoices[1].InvoiceNo != "58816591" {
		t.Errorf("第 2 张发票号 = %q, want 58816591", res.Invoices[1].InvoiceNo)
	}
	// 数据行应回填车牌/车型/通行起止
	it := res.Invoices[0].Items
	if len(it) == 0 {
		t.Fatal("第 1 张应回填明细行")
	}
	if v, _ := it[0].Extra["plate_no"].(string); v != "渝C87567" {
		t.Errorf("plate_no = %q, want 渝C87567", v)
	}
	if v, _ := it[0].Extra["vehicle_type"].(string); v != "货车" {
		t.Errorf("vehicle_type = %q, want 货车", v)
	}
	if v, _ := it[0].Extra["travel_from"].(string); v != "2025-10-24" {
		t.Errorf("travel_from = %q, want 2025-10-24", v)
	}
	if v, _ := it[0].Extra["travel_to"].(string); v != "2025-10-24" {
		t.Errorf("travel_to = %q, want 2025-10-24", v)
	}
}

// 数量不一致时不覆盖：LLM 多提/少提会导致按序对应错位，宁可不改（保守），
// 由用户复核，也好过把 A 票的号码写到 B 票上（历史踩过"张冠李戴"）。
func TestOverlayDeterministicInvoiceFieldsSkipsOnCountMismatch(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{Page: 1, InvoiceNo: "LLM_ONLY"}},
	}
	chunks := []*types.Chunk{{ChunkType: types.ChunkTypeText, Content: flatSampleTwoInvoices}}

	overlaid := OverlayDeterministicInvoiceFields(res, chunks, "")
	if overlaid != 0 {
		t.Fatalf("数量不一致（文本 2 张 / 结果 1 张）应不覆盖，实际覆盖 %d 张", overlaid)
	}
	if res.Invoices[0].InvoiceNo != "LLM_ONLY" {
		t.Errorf("不应改动原值，实际 %q", res.Invoices[0].InvoiceNo)
	}
}

// 文本里没有明确『发票号码』标签时不得猜测（机器编号/校验码都可能像发票号）。
func TestOverlayDeterministicInvoiceFieldsNoLabelLeavesAsIs(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{Page: 1, InvoiceNo: "KEEP_ME"}},
	}
	text := "机器编号: 499098302394\n校 验 码: 13353 86372\n金额 93.32"
	chunks := []*types.Chunk{{ChunkType: types.ChunkTypeText, Content: text}}

	if overlaid := OverlayDeterministicInvoiceFields(res, chunks, ""); overlaid != 0 {
		t.Fatalf("无『发票号码』标签时不应覆盖，实际覆盖 %d 张", overlaid)
	}
	if res.Invoices[0].InvoiceNo != "KEEP_ME" {
		t.Errorf("应保持原值，实际 %q", res.Invoices[0].InvoiceNo)
	}
}

// 完整规则提取器：不依赖 LLM 直接解析 docreader 的票据文本。
// 真实样本（单张票，取自生产 PDF 的 chunk）：发票代码/号码/开票日期独立成行，
// 明细为一行「项目 车牌 车型 通行起 通行止 金额 税率 税额」。
const flatSingleInvoiceSample = `发票代码: 050002400112

# <u>重庆增值税电子普通发票</u>

发票号码: 64869484

||﹊﹋﹌ 机器编号: 499098302394||||||||开票日期: 2025年11月04日 校 验 码: 13353 86372 62628 06587||
|---|---|---|---|---|---|---|---|---|---|---|
备 注
项目名称 车牌号 类型 通行日期起 通行日期止 金　额 税率 税　额
*经营租赁*通行费 渝C87567 货车 20251024 20251024 93.32 9% 8.40
499098302394
壹佰零壹元柒角贰分
陈思颖 袁银红 陈思颖
重庆星达铜业有限公司
91500227MA5U54AJ0A
重庆北新渝长高速公路建设有限公司
91500115MA5U599D9X
`

func TestExtractInvoicesByFlatLayoutParsesSingleInvoice(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(flatSingleInvoiceSample)
	if !ok {
		t.Fatal("单张票文本应命中规则提取")
	}
	if len(res.Invoices) != 1 {
		t.Fatalf("应解析 1 张，实际 %d", len(res.Invoices))
	}
	inv := res.Invoices[0]
	if inv.InvoiceNo != "64869484" {
		t.Errorf("invoice_no = %q, want 64869484（不得取机器编号 499098302394）", inv.InvoiceNo)
	}
	if inv.InvoiceCode != "050002400112" {
		t.Errorf("invoice_code = %q, want 050002400112", inv.InvoiceCode)
	}
	if inv.InvoiceDate != "2025-11-04" {
		t.Errorf("invoice_date = %q, want 2025-11-04", inv.InvoiceDate)
	}
	if len(inv.Items) != 1 {
		t.Fatalf("应有 1 行明细，实际 %d", len(inv.Items))
	}
	it := inv.Items[0]
	if it.Extra["plate_no"] != "渝C87567" {
		t.Errorf("plate_no = %v, want 渝C87567", it.Extra["plate_no"])
	}
	if it.Extra["vehicle_type"] != "货车" {
		t.Errorf("vehicle_type = %v, want 货车", it.Extra["vehicle_type"])
	}
	if it.Extra["travel_from"] != "2025-10-24" || it.Extra["travel_to"] != "2025-10-24" {
		t.Errorf("travel = %v~%v, want 2025-10-24~2025-10-24", it.Extra["travel_from"], it.Extra["travel_to"])
	}
	if it.Amount == nil || *it.Amount != 93.32 {
		t.Errorf("amount = %v, want 93.32", it.Amount)
	}
	if it.TaxRate == nil || *it.TaxRate != 0.09 {
		t.Errorf("tax_rate = %v, want 0.09", it.TaxRate)
	}
}

// 非票据文本不得命中（避免把合同/公告误当发票）。
func TestExtractInvoicesByFlatLayoutRejectsNonInvoice(t *testing.T) {
	if _, ok := ExtractInvoicesByFlatLayout("这是一份普通通知公告，没有任何票据字段。"); ok {
		t.Fatal("非票据文本不应命中")
	}
}

// OCR 引擎（paddleocr_vl_cloud）输出 HTML 表格：号码独立成行（全角冒号），
// 明细在 <tr><td> 里。此样本取自生产 OCR 文本。
const flatHTMLInvoiceSample = `发票代码：050002400112

发票号码：64869484

开票日期：2025年11月04日

<tr><td>项目名称</td><td>车牌号</td><td>类型</td><td>通行日期起</td><td>通行日期止</td><td>金 额</td><td>税率</td><td>税 额</td></tr>
<tr><td>$ ^{{*}}经营租赁^{{*}}通行费 $</td><td>渝C87567</td><td>货车</td><td>20251024</td><td>20251024</td><td>93.32</td><td>9%</td><td>8.40</td></tr>
<tr><td>合 计</td><td></td><td></td><td></td><td></td><td>¥93.32</td><td></td><td>¥8.40</td></tr>
<tr><td>价税合计（大写）</td><td colspan="7">壹佰零壹元柒角贰分 (小写) ¥101.72</td></tr>
`

func TestExtractInvoicesByFlatLayoutParsesHTMLTable(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(flatHTMLInvoiceSample)
	if !ok {
		t.Fatal("OCR HTML 表格文本应命中规则提取")
	}
	if len(res.Invoices) != 1 {
		t.Fatalf("应解析 1 张，实际 %d", len(res.Invoices))
	}
	inv := res.Invoices[0]
	if inv.InvoiceNo != "64869484" {
		t.Errorf("invoice_no = %q, want 64869484", inv.InvoiceNo)
	}
	if len(inv.Items) != 1 {
		t.Fatalf("应回填 1 行明细，实际 %d", len(inv.Items))
	}
	it := inv.Items[0]
	if it.Extra["plate_no"] != "渝C87567" {
		t.Errorf("plate_no = %v, want 渝C87567（HTML 表格必须解析）", it.Extra["plate_no"])
	}
	if it.Extra["vehicle_type"] != "货车" {
		t.Errorf("vehicle_type = %v, want 货车", it.Extra["vehicle_type"])
	}
	if it.Extra["travel_from"] != "2025-10-24" || it.Extra["travel_to"] != "2025-10-24" {
		t.Errorf("travel = %v~%v", it.Extra["travel_from"], it.Extra["travel_to"])
	}
	if it.Amount == nil || *it.Amount != 93.32 {
		t.Errorf("amount = %v, want 93.32", it.Amount)
	}
	if inv.TotalAmount == nil || *inv.TotalAmount != 101.72 {
		t.Errorf("total_amount = %v, want 101.72（取票面小写合计）", inv.TotalAmount)
	}
}
