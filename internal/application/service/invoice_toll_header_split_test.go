package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归（2026-10-20 真机 P4）：MinerU 把明细表头 4 个标签挤进同一个
// `colspan=5` 单元格，数据行却是 8 个独立单元格：
//
//	表头: <tr><td colspan="5">项目名称 车牌号 车辆类型 通行日期起</td>
//	      <td>通行日期止</td><td>金额</td><td>税率/征收率</td><td>税额</td></tr>
//	数据: <tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td>
//	      <td colspan="2">20260725</td><td>20260725</td><td>78.05</td>
//	      <td>3%</td><td>2.34</td></tr>
//
// 位置式对齐必然错位（表头 5 格 vs 数据 8 格），且 chunk 在表头行末尾被切断。
// 修复：把多标签表头格按标签拆成多个单标签格，使表头与数据格一一对应。

// tollP4Segment 复刻真机 chunk15+chunk16 拼成的票段原文。
func tollP4Segment() string {
	return `## 电子发票（普通发票） 国家税务总局

发票号码：26517905800800020367

开票日期：2026年08月07日

<table>
<tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司统一社会信用代码/纳税人识别号:91500227MA5U54AJ0A</td><td>销售方信息</td><td colspan="4">名称:四川雅康高速公路有限责任公司统一社会信用代码/纳税人识别号:915100000858101249</td></tr>
<tr><td colspan="5">项目名称 车牌号 车辆类型 通行日期起</td><td>通行日期止</td><td>金额</td><td>税率/征收率</td><td>税额</td></tr>
<tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td><td colspan="2">20260725</td><td>20260725</td><td>78.05</td><td>3%</td><td>2.34</td></tr>
<tr><td colspan="9">合计 ¥78.05 ¥2.34</td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="4">捌拾圆叁角玖分</td><td>(小写)</td><td colspan="2">¥80.39</td></tr>
</table>

开票人：石金洋`
}

// TestBusinessTollMultiLabelHeaderCellParsesItemRow 主回归：
// 多标签表头格不得导致明细行漏提。该变体走「表头定列 / 数据行纯值」路径
// （htmlTableDataRow），拆分后表头 8 格与数据 8 格一一对应，
// 车牌/车型/通行日期/金额/税率/税额必须全部取出。
func TestBusinessTollMultiLabelHeaderCellParsesItemRow(t *testing.T) {
	seg := tollP4Segment()
	// 先经表头格拆分归一（修复点）
	normalized := splitMultiLabelHeaderCells(seg)

	vals := htmlTableDataRow(normalized)
	require.NotEmpty(t, vals, "多标签表头格 + 数据行必须能被定位")
	require.Len(t, vals, 8, "数据行应有 8 个单元格")
	assert.Equal(t, "*生产生活服务*通行费", strings.TrimSpace(vals[0]), "项目名称")
	assert.Equal(t, "渝A75R1C", strings.TrimSpace(vals[1]), "车牌号必须取出")
	assert.Equal(t, "客车", strings.TrimSpace(vals[2]), "车辆类型必须取出")
	assert.Equal(t, "20260725", strings.TrimSpace(vals[3]), "通行日期起必须取出")
	assert.Equal(t, "20260725", strings.TrimSpace(vals[4]), "通行日期止必须取出")
	assert.Equal(t, "78.05", strings.TrimSpace(vals[5]), "金额必须取出")
	assert.Equal(t, "3%", strings.TrimSpace(vals[6]), "税率必须取出")
	assert.Equal(t, "2.34", strings.TrimSpace(vals[7]), "税额必须取出")

	// 端到端：走 parseFlatInvoiceSegment，票面字段必须落到发票对象上
	inv, ok := parseFlatInvoiceSegment(normalized)
	require.True(t, ok, "该票段必须能被解析")
	require.Len(t, inv.Items, 1, "必须解析出 1 行明细")
	line := inv.Items[0]
	require.NotNil(t, line.Amount)
	assert.InDelta(t, 78.05, *line.Amount, 1e-9, "明细金额必须落到 items")
	require.NotNil(t, line.TaxAmt)
	assert.InDelta(t, 2.34, *line.TaxAmt, 1e-9, "明细税额必须落到 items")
	require.NotNil(t, line.TaxRate)
	assert.InDelta(t, 0.03, *line.TaxRate, 1e-9, "明细税率必须落到 items")
	// 发票级回填（P4 原本只有价税合计，其余全空）
	require.NotNil(t, inv.Amount)
	assert.InDelta(t, 78.05, *inv.Amount, 1e-9, "发票级金额必须回填")
	require.NotNil(t, inv.Tax)
	assert.InDelta(t, 2.34, *inv.Tax, 1e-9, "发票级税额必须回填")
	assert.Equal(t, "渝A75R1C", line.Extra["plate_no"], "车牌号必须落到明细 extra")
	assert.Equal(t, "客车", line.Extra["vehicle_type"], "车辆类型必须落到明细 extra")
}

// TestBusinessTollMultiLabelHeaderSplitIsPureText 锁死拆分只影响表头格，
// 不动数据行的 colspan 结构（数据行 colspan=2 的通行日期起止必须保持原样）。
func TestBusinessTollMultiLabelHeaderSplitIsPureText(t *testing.T) {
	out := splitMultiLabelHeaderCells(tollP4Segment())
	// 表头行拆成 9 个单标签格
	assert.Equal(t, 1, strings.Count(out, `<td>项目名称</td>`), "项目名称应独立成格")
	assert.Equal(t, 1, strings.Count(out, `<td>车牌号</td>`), "车牌号应独立成格")
	assert.Equal(t, 1, strings.Count(out, `<td>车辆类型</td>`), "车辆类型应独立成格")
	assert.Equal(t, 1, strings.Count(out, `<td>通行日期起</td>`), "通行日期起应独立成格")
	// 数据行保持 colspan（跨两列的通行日期）
	assert.Contains(t, out, `<td colspan="2">20260725</td>`, "数据行 colspan 必须原样保留")
}

// 不含多标签表头格时必须是 no-op（防止误伤普通表格）。
func TestBusinessTollMultiLabelHeaderSplitNoopOnPlainTable(t *testing.T) {
	plain := `<table><tr><td>项目名称</td><td>车牌号</td></tr><tr><td>通行费</td><td>渝A1</td></tr></table>`
	assert.Equal(t, plain, splitMultiLabelHeaderCells(plain), "普通表格不得被改写")
}
