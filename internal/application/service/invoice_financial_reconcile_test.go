package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

func ptrf(v float64) *float64 { return &v }

// 真机样本（knowledge c9941d0c，19 页汇总单第 2 张票）：
// 金额 198.09、税额 17.83、价税合计 215.92，票面税率 9%，
// 但税率被邻页的 3% 覆盖。数学上 198.09 × 9% = 17.83 完美闭合。
func TestReconcileInvoiceFinancialsCorrectsMislabelledRate(t *testing.T) {
	inv := InvoiceExtractionItem{
		InvoiceNo:   "26507903331100005079",
		Amount:      ptrf(198.09),
		Tax:         ptrf(17.83),
		TotalAmount: ptrf(215.92),
		TaxRate:     ptrf(0.03),
		Items: []InvoiceExtractionLine{{
			Name:    "*生产生活服务*通行费",
			Amount:  ptrf(198.09),
			TaxAmt:  ptrf(17.83),
			TaxRate: ptrf(0.03),
		}},
	}
	fix := ReconcileInvoiceFinancials(&inv)
	require.True(t, fix.Changed())
	require.NotNil(t, inv.TaxRate)
	assert.Equal(t, 0.09, *inv.TaxRate, "198.09/17.83 必须纠偏为 9%%")
	assert.Equal(t, 0.09, *inv.Items[0].TaxRate, "明细行税率必须同步纠偏")
	assert.Equal(t, 1, fix.ItemRatesFixed)
	// 三件套本身自洽，金额不得被改动。
	assert.Equal(t, 198.09, *inv.Amount)
	assert.Equal(t, 17.83, *inv.Tax)
	assert.Equal(t, 215.92, *inv.TotalAmount)
}

// 正确数据必须零改动（避免每次启动都写库、也避免把对的改成错的）。
func TestReconcileInvoiceFinancialsKeepsValidInvoice(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount:      ptrf(64.08),
		Tax:         ptrf(1.92),
		TotalAmount: ptrf(66.00),
		TaxRate:     ptrf(0.03),
		Items: []InvoiceExtractionLine{{
			Amount: ptrf(64.08), TaxAmt: ptrf(1.92), TaxRate: ptrf(0.03),
		}},
	}
	fix := ReconcileInvoiceFinancials(&inv)
	assert.False(t, fix.Changed(), "正确发票不得被改写")
	assert.Equal(t, 0.03, *inv.TaxRate)
}

// 价税合计缺失时由金额 + 税额补齐；已有合计但不自洽时以合计为锚重算金额。
func TestReconcileInvoiceFinancialsClosesTotalIdentity(t *testing.T) {
	missingTotal := InvoiceExtractionItem{Amount: ptrf(100), Tax: ptrf(9)}
	fix := ReconcileInvoiceFinancials(&missingTotal)
	require.True(t, fix.TotalFilled)
	require.NotNil(t, missingTotal.TotalAmount)
	assert.Equal(t, 109.0, *missingTotal.TotalAmount)
	assert.Equal(t, 0.09, *missingTotal.TaxRate)

	broken := InvoiceExtractionItem{Amount: ptrf(90), Tax: ptrf(9), TotalAmount: ptrf(109)}
	fix = ReconcileInvoiceFinancials(&broken)
	assert.True(t, fix.Changed())
	assert.InDelta(t, 100.0, *broken.Amount, 0.001, "amount 必须按 109/1.09 反算")
	assert.InDelta(t, 9.0, *broken.Tax, 0.001)
	assert.InDelta(t, 109.0, *broken.Amount+*broken.Tax, 0.02)
}

// 无法用标准档位闭环时不得瞎改，只产出告警。
func TestReconcileInvoiceFinancialsLeavesUnresolvableAlone(t *testing.T) {
	invs := []InvoiceExtractionItem{{
		Amount: ptrf(198.09), Tax: ptrf(5.00), TotalAmount: ptrf(203.09), TaxRate: ptrf(0.03),
	}}
	issues := ReconcileInvoiceFinancialsForDocument(invs)
	assert.Equal(t, 0.03, *invs[0].TaxRate, "无标准档可解释时保留原税率")
	require.Len(t, issues, 1)
	assert.Equal(t, "amount", issues[0].Field)
	assert.Contains(t, issues[0].Detail, "标准税率档位")
}

// 明细合计与票面不一致的判定：6.45 挂在 198.09 的票上必须命中。
func TestInvoiceItemsAmountMismatch(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount: ptrf(198.09), Tax: ptrf(17.83),
		Items: []InvoiceExtractionLine{{Amount: ptrf(6.45), TaxAmt: ptrf(0.19)}},
	}
	assert.True(t, InvoiceItemsAmountMismatch(&inv))
	inv.Items = []InvoiceExtractionLine{{Amount: ptrf(198.09), TaxAmt: ptrf(17.83)}}
	assert.False(t, InvoiceItemsAmountMismatch(&inv))
}

// tollPageChunks 造「每页 1 票」的 HTML 表格页（与真机 MinerU 输出同形）。
func tollPageChunks(no string, page int, plate string, from, to, amount, rate, tax string, flatRow bool) []*types.Chunk {
	head := "## 电子发票（普通发票） 国家税务总局\n发票号码：" + no + "\n开票日期：2026年11月02日\n<table>\n"
	var row string
	if flatRow {
		// 整行压进一个 colspan 单元格：扁平正则能命中，HTML 行解析也能命中。
		row = `<tr><td colspan="9">*生产生活服务*通行费 ` + plate + ` 货车 ` + from + ` ` + to + ` ` + amount + ` ` + rate + ` ` + tax + `</td></tr>`
	} else {
		row = `<tr><td colspan="5">*生产生活服务*通行费 ` + plate + ` 货车 ` + from + `</td><td>` + to + `</td><td>` + amount + `</td><td>` + rate + `</td><td>` + tax + `</td></tr>`
	}
	return []*types.Chunk{
		{ChunkIndex: page * 2, ChunkType: types.ChunkTypeText,
			Content:        head + row + "\n</table>\n开票人：唐菲\n",
			SourceLocators: types.SourceLocators{{Type: "pdf", Page: page}}},
	}
}

// 串页明细必须按本页原文重建：第 1 页的票挂着第 3 页的 6.45 → 改回 68.82。
func TestReconcileInvoiceItemsWithSourceRebuildsCrossPageItems(t *testing.T) {
	var chunks []*types.Chunk
	chunks = append(chunks, tollPageChunks("26500000000000000001", 1, "渝C81022", "20261010", "20261031", "68.82", "3%", "2.06", false)...)
	chunks = append(chunks, tollPageChunks("26500000000000000002", 2, "渝C81022", "20261013", "20261027", "198.09", "9%", "17.83", false)...)
	chunks = append(chunks, tollPageChunks("26500000000000000003", 3, "渝C81022", "20261030", "20261030", "6.45", "3%", "0.19", true)...)
	src := BuildExtractSourceText(chunks)
	require.True(t, src.HasPages())

	invs := []InvoiceExtractionItem{
		{InvoiceNo: "26500000000000000001", PageNo: 1, Amount: ptrf(68.82), Tax: ptrf(2.06), TotalAmount: ptrf(70.88),
			Items: []InvoiceExtractionLine{{Amount: ptrf(6.45), TaxAmt: ptrf(0.19), TaxRate: ptrf(0.03),
				Extra: map[string]any{"travel_from": "2026-10-30", "travel_to": "2026-10-30"}}}},
		{InvoiceNo: "26500000000000000002", PageNo: 2, Amount: ptrf(198.09), Tax: ptrf(17.83), TotalAmount: ptrf(215.92),
			TaxRate: ptrf(0.03),
			Items:   []InvoiceExtractionLine{{Amount: ptrf(6.45), TaxAmt: ptrf(0.19), TaxRate: ptrf(0.03)}}},
	}
	rebuilt := ReconcileInvoiceItemsWithSource(invs, src)
	assert.Equal(t, 2, rebuilt)
	require.Len(t, invs[0].Items, 1)
	assert.InDelta(t, 68.82, *invs[0].Items[0].Amount, 0.001, "第 1 页必须拿回自己的 68.82")
	assert.Equal(t, "2026-10-10", extraString(invs[0].Items[0].Extra, "travel_from"))
	require.Len(t, invs[1].Items, 1)
	assert.InDelta(t, 198.09, *invs[1].Items[0].Amount, 0.001)
	require.NotNil(t, invs[1].Items[0].TaxRate)
	assert.Equal(t, 0.09, *invs[1].Items[0].TaxRate)
	// 重建后再过四元校验，票级税率也必须变成 9%。
	ReconcileInvoiceFinancials(&invs[1])
	assert.Equal(t, 0.09, *invs[1].TaxRate)
}

// 页内数据行与票面金额/税额都对不上时不得重建（防止把同页的「合计行」当明细）。
func TestReconcileInvoiceItemsWithSourceSkipsUnrelatedRow(t *testing.T) {
	chunks := tollPageChunks("26500000000000000009", 1, "渝C81022", "20261010", "20261031", "999.99", "9%", "90.00", false)
	src := BuildExtractSourceText(chunks)
	invs := []InvoiceExtractionItem{{
		InvoiceNo: "26500000000000000009", PageNo: 1, Amount: ptrf(68.82), Tax: ptrf(2.06),
		Items: []InvoiceExtractionLine{{Amount: ptrf(68.82), TaxAmt: ptrf(2.06)}},
	}}
	assert.Equal(t, 0, ReconcileInvoiceItemsWithSource(invs, src))
	assert.InDelta(t, 68.82, *invs[0].Items[0].Amount, 0.001)
}

// 回归：19 页汇总单里只有 2 页明细能被扁平正则命中，旧实现按下标把它们写进
// 第 1、2 张票（真机 198.09 的票显示 6.45 的明细）。修复后必须按页配对。
func TestOverlayDeterministicInvoiceFieldsDoesNotLeakItemsAcrossPages(t *testing.T) {
	var chunks []*types.Chunk
	// 第 1、2 页是「一格一列」的 HTML 表格（扁平正则打不中）。
	chunks = append(chunks, tollPageChunks("26500000000000000011", 1, "渝C81022", "20261010", "20261031", "68.82", "3%", "2.06", false)...)
	chunks = append(chunks, tollPageChunks("26500000000000000012", 2, "渝C81022", "20261013", "20261027", "198.09", "9%", "17.83", false)...)
	// 第 3 页是「整行压一格」的扁平行。
	chunks = append(chunks, tollPageChunks("26500000000000000013", 3, "渝C81022", "20261030", "20261030", "6.45", "3%", "0.19", true)...)
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "26500000000000000011", PageNo: 1, Amount: ptrf(68.82), Tax: ptrf(2.06), TotalAmount: ptrf(70.88)},
		{InvoiceNo: "26500000000000000012", PageNo: 2, Amount: ptrf(198.09), Tax: ptrf(17.83), TotalAmount: ptrf(215.92), TaxRate: ptrf(0.03)},
		{InvoiceNo: "26500000000000000013", PageNo: 3, Amount: ptrf(6.45), Tax: ptrf(0.19), TotalAmount: ptrf(6.64)},
	}}
	OverlayDeterministicInvoiceFields(res, chunks, "通行费电子票据汇总单(发票).pdf")

	require.Len(t, res.Invoices[0].Items, 1)
	require.NotNil(t, res.Invoices[0].Items[0].Amount)
	assert.InDelta(t, 68.82, *res.Invoices[0].Items[0].Amount, 0.001,
		"第 1 张票绝不能拿到第 3 页的 6.45（旧实现按下标回填就错在这里）")
	require.Len(t, res.Invoices[1].Items, 1)
	require.NotNil(t, res.Invoices[1].Items[0].Amount)
	assert.InDelta(t, 198.09, *res.Invoices[1].Items[0].Amount, 0.001)
	// 第 2 张票的税率必须被四元校验纠回 9%。
	assert.Equal(t, 0.09, *res.Invoices[1].TaxRate)
	assert.False(t, InvoiceItemsAmountMismatch(&res.Invoices[1]))
}

// 没有页码信息时退回「行数 == 发票数」的严格 1:1，绝不按下标硬套。
func TestOverlayDeterministicInvoiceFieldsRequiresOneToOneWithoutPages(t *testing.T) {
	chunks := []*types.Chunk{
		{ChunkIndex: 0, ChunkType: types.ChunkTypeText,
			Content: "发票号码：26500000000000000021\n" +
				"*生产生活服务*通行费 渝A11111 货车 20261001 20261002 100.00 9% 9.00\n" +
				"发票号码：26500000000000000022\n" +
				"*生产生活服务*通行费 渝A22222 货车 20261003 20261004 200.00 9% 18.00\n"},
	}
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{InvoiceNo: "26500000000000000021", Amount: ptrf(100), Tax: ptrf(9)},
	}}
	OverlayDeterministicInvoiceFields(res, chunks, "x.pdf")
	// 2 行 vs 1 张票：数不齐，明细不得按下标写入。
	assert.Empty(t, res.Invoices[0].Items)
}

// PageTexts 必须按标记切页，不能把相邻页内容粘进同一页。
func TestExtractSourceTextPageTexts(t *testing.T) {
	chunks := []*types.Chunk{
		{ChunkIndex: 0, ChunkType: types.ChunkTypeText, Content: "第1页正文", SourceLocators: types.SourceLocators{{Type: "pdf", Page: 1}}},
		{ChunkIndex: 1, ChunkType: types.ChunkTypeText, Content: "第2页正文", SourceLocators: types.SourceLocators{{Type: "pdf", Page: 2}}},
	}
	src := BuildExtractSourceText(chunks)
	pages := src.PageTexts()
	require.Contains(t, pages, 1)
	require.Contains(t, pages, 2)
	assert.True(t, strings.Contains(pages[1], "第1页正文"))
	assert.False(t, strings.Contains(pages[1], "第2页正文"))
	assert.True(t, strings.Contains(pages[2], "第2页正文"))
}

// 串页铁证：单行明细的金额恰好等于文档里另一张票的金额。
// 折扣票/单票文档的「明细不等于票面」不得判为串页（否则全库几百张正常票标红）。
func TestInvoiceItemsCrossPageSuspect(t *testing.T) {
	invs := []InvoiceExtractionItem{
		{Amount: ptrf(68.82), Items: []InvoiceExtractionLine{{Amount: ptrf(2.62)}}},
		{Amount: ptrf(198.09), Items: []InvoiceExtractionLine{{Amount: ptrf(6.45)}}},
		{Amount: ptrf(2.62)},
		{Amount: ptrf(6.45)},
	}
	assert.True(t, InvoiceItemsCrossPageSuspect(invs, 0))
	assert.True(t, InvoiceItemsCrossPageSuspect(invs, 1))

	discount := []InvoiceExtractionItem{{
		Amount: ptrf(1117.17),
		Items:  []InvoiceExtractionLine{{Amount: ptrf(1269.91)}},
	}}
	assert.False(t, InvoiceItemsCrossPageSuspect(discount, 0), "单票文档的折扣行不是串页")

	multi := []InvoiceExtractionItem{
		{Amount: ptrf(10), Items: []InvoiceExtractionLine{{Amount: ptrf(5)}, {Amount: ptrf(5)}}},
		{Amount: ptrf(5)},
	}
	assert.False(t, InvoiceItemsCrossPageSuspect(multi, 0), "多行明细不按单行串页判定")
}

// 一票多档（13% 设备 + 9% 基础电信 + 6% 增值电信）不得因为
// 「票级税率 × 总金额 ≠ 总税额」被标红 —— 票级税率只是金额最大那一行的税率。
func TestInvoiceFinancialConflictSkipsMultiRateInvoice(t *testing.T) {
	invs := []InvoiceExtractionItem{{
		Amount: ptrf(827.48), Tax: ptrf(65.52), TotalAmount: ptrf(893.00), TaxRate: ptrf(0.09),
		Items: []InvoiceExtractionLine{
			{Amount: ptrf(11.25), TaxAmt: ptrf(1.46), TaxRate: ptrf(0.13)},
			{Amount: ptrf(502.90), TaxAmt: ptrf(45.26), TaxRate: ptrf(0.09)},
			{Amount: ptrf(313.33), TaxAmt: ptrf(18.80), TaxRate: ptrf(0.06)},
		},
	}}
	assert.Empty(t, ReconcileInvoiceFinancialsForDocument(invs))
	// 三件套不自洽时以价税合计为锚自动重算（不刷告警），重算后必须闭合。
	invs[0].TotalAmount = ptrf(999.00)
	assert.Empty(t, ReconcileInvoiceFinancialsForDocument(invs))
	assert.InDelta(t, 999.00, *invs[0].Amount+*invs[0].Tax, 0.02)
}

// 通行日期格式归一：紧凑 8 位 → YYYY-MM-DD，已是标准格式的不动。
func TestNormalizeInvoiceItemDates(t *testing.T) {
	invs := []InvoiceExtractionItem{{
		Items: []InvoiceExtractionLine{
			{Extra: map[string]any{"travel_from": "20261010", "travel_to": "20261031"}},
			{Extra: map[string]any{"travel_from": "2026-10-01", "travel_to": "20261002"}},
		},
	}}
	assert.Equal(t, 3, NormalizeInvoiceItemDates(invs))
	assert.Equal(t, "2026-10-10", invs[0].Items[0].Extra["travel_from"])
	assert.Equal(t, "2026-10-31", invs[0].Items[0].Extra["travel_to"])
	assert.Equal(t, "2026-10-01", invs[0].Items[1].Extra["travel_from"])
	assert.Equal(t, "2026-10-02", invs[0].Items[1].Extra["travel_to"])
	// 幂等
	assert.Equal(t, 0, NormalizeInvoiceItemDates(invs))
}

// 通行费明细缺「通行日期起」必须被识别为待补（旧 HTML 规则取到车牌后 continue 的后遗症）。
func TestInvoiceItemExtrasIncomplete(t *testing.T) {
	inv := InvoiceExtractionItem{Items: []InvoiceExtractionLine{{
		Name:  "*生产生活服务*通行费",
		Extra: map[string]any{"plate_no": "渝C81022", "travel_to": "2026-10-19"},
	}}}
	assert.True(t, InvoiceItemExtrasIncomplete(&inv))
	inv.Items[0].Extra["travel_from"] = "2026-10-19"
	assert.False(t, InvoiceItemExtrasIncomplete(&inv))
	// 非通行费（办公用品多行明细）不在此列
	other := InvoiceExtractionItem{Items: []InvoiceExtractionLine{{Name: "打印纸", Extra: map[string]any{"plate_no": "x"}}}}
	assert.False(t, InvoiceItemExtrasIncomplete(&other))
}

// 重建明细后，备注里的「车牌号/通行日期起止」必须同步刷新（真机抽屉里
// 明细写 2026-10-13、备注却写「通行日期起：」）。
func TestRefreshTollRemarkAndStaleness(t *testing.T) {
	row := htmlTollRow{Plate: "渝C81022", From: "20261013", To: "20261027"}
	got := refreshTollRemark("车牌号：渝C81022；通行日期起：；通行日期止：20261027", row)
	assert.Equal(t, "车牌号：渝C81022；通行日期起：2026-10-13；通行日期止：2026-10-27", got)
	// 其它内容保留
	got = refreshTollRemark("车牌号：渝C81022；通行日期起：；通行日期止：20261027；业务流水号:123", row)
	assert.Contains(t, got, "业务流水号:123")

	inv := InvoiceExtractionItem{
		Remark: "车牌号：渝C81022；通行日期起：；通行日期止：20261027",
		Items: []InvoiceExtractionLine{{Extra: map[string]any{
			"plate_no": "渝C81022", "travel_from": "2026-10-13", "travel_to": "2026-10-27",
		}}},
	}
	assert.True(t, InvoiceTollRemarkStale(&inv))
	inv.Remark = got
	assert.False(t, InvoiceTollRemarkStale(&inv))
}
