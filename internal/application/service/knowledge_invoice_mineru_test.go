package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本组测试锁定 MinerU API 解析产物的适配契约。
//
// 真实来源：VM 库 knowledges.id=ad740ba3-7e8b-4c4e-b479-075d96fa141e
// （通行费电子票据汇总单(发票).pdf，27 张票，全库 chunks 89 条）。
// MinerU 输出与被测代码的两处关键差异（均已实测确认）：
//
//  1. 明细行把「标签 + 数值」合并进同一个 <td>：
//     <td>金额234.25</td> / <td>税率3%</td> / <td>税额7.03</td>
//     / <td>车牌号渝C12345</td> / <td>通行日期起20250505</td>
//     而 htmlTableDataRow 的数据行特征判定要求 vals[1] 是纯车牌（plateLike）、
//     vals[3]/vals[4] 是纯 8 位日期（isCompact8Date）→ 全部不匹配 → 明细行为空
//     → 金额/税率/税额全丢，只剩 flatHTMLTotalRe 捞到的价税合计。
//
//  2. 购销方标签的空格形态不稳定：同一份 27 张票里，12 张是 <td>购买方</td>、
//     15 张是 <td>购 买 方</td>（销售方同理 12/15）。flatPartyCellRe 只匹配
//     连续「购买方」→ 带空格的 15 张购销方整体丢失（与库中 12 有 / 15 空完全吻合）。
//
// 以下 fixture 保留真实结构（含空格、colspan/rowspan、合并标签、¥ 重复），
// 仅将公司名/税号/车牌脱敏。

// mineruInvoiceVariantB 复刻票号 49367445：购销方标签**带空格**、明细行「标签+数值同格」。
func mineruInvoiceVariantB() string {
	return `发票代码: 050002400112

发票号码: 49367445

开票日期: 2025年06月04日

校验码: 05307 14389 04001 03859

<table>
<tr><td>购 买 方</td><td colspan="5">名 称:重庆示例铜业有限公司纳税人识别号:91500000MA5U54AJ0A地址、电话:重庆市示例区示例街道示例路106号023-41557211开户行及账号:中国建设银行示例支行50050118370000000047</td><td>密 码 区</td><td colspan="4">03+9&lt;3&gt;862-&gt;87-0-4-+5-1004//08&lt;1-855++4&gt;&lt;9&gt;001586493574/5+63+8*+976/&gt;73*3/76740243622&gt;56&lt;+/-3201&gt;3*0197&lt;-9934&gt;37</td></tr>
<tr><td rowspan="2" colspan="2">项目名称*经营租赁*通行费合 计</td><td rowspan="2">车牌号渝C12345</td><td rowspan="2">类型货车</td><td rowspan="2">通行日期起20250505</td><td rowspan="2" colspan="2">通行日期止20250528</td><td>金额234.25</td><td rowspan="2">税率3%</td><td>税额7.03</td><td></td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="8">贰佰肆拾壹元贰角捌分 (小写) ¥241.28</td><td></td></tr>
<tr><td>销 售 方</td><td colspan="5">名 称:示例高速公路有限公司乙纳税人识别号:91500000696569675Y地址、电话:重庆市示例区示例路666号023-85716000开户行及账号:中国建设银行示例支行50001033600050212812</td><td>备 注</td><td colspan="3">汇总开具</td><td></td></tr></table>

开票人: 施煌煌`
}

// mineruInvoiceVariantA 复刻票号 48627547：购销方标签**无空格**、明细行同样合并标签。
func mineruInvoiceVariantA() string {
	return `发票代码: 050002400112

发票号码: 48627547

开票日期: 2025年06月04日

校验码: 03159 89600 68801 14936

<table>
<tr><td>购买方</td><td colspan="4">名称:重庆示例铜业有限公司纳税人识别号:91500000MA5U54AJ0A地址、电话:重庆市示例区示例街道示例路106号023-41557211开户行及账号:中国建设银行示例支行50050118370000000047</td><td>密码区</td><td colspan="4">03&lt;9353867&lt;2*0&gt;975--&lt;1+00871&gt;&gt;9228&lt;&gt;43/7+141924+8*++57--&lt;*3*82+3/8-79/+/5&lt;9061+9*85-&lt;3++3&gt;+1301&gt;72-19/&gt;880++94-</td></tr>
<tr><td colspan="2">项目名称*经营租赁*通行费合计</td><td>车牌号渝C12345</td><td>类型货车</td><td>通行日期起20250519</td><td>通行日期止20250521</td><td>金额158.51¥158.51</td><td>税率9%</td><td colspan="2">税额14.27¥14.27</td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="8">壹佰柒拾贰元柒角捌分(小写)¥172.78</td></tr>
<tr><td>销售方</td><td colspan="4">名称:示例高速公路有限公司甲纳税人识别号:91500000MACGNJL47X地址、电话:重庆市示例区示例路37号023-89139275开户行及账号:中国邮政储蓄银行示例支行950007010032618904</td><td>备注</td><td colspan="4">汇总开具91500000MACGNJL47X</td></tr></table>

开票人: 唐菲`
}

// TestExtractInvoiceMineruMergedLabelItemRowFillsAmountRateTax 锁定明细行「标签+数值同格」。
// 修复前：amount/tax_rate/tax 全为 nil（列表显示「-」），只有 total_amount。
func TestExtractInvoiceMineruMergedLabelItemRowFillsAmountRateTax(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(mineruInvoiceVariantB())
	require.True(t, ok, "扁平布局规则必须命中")
	require.Len(t, res.Invoices, 1)
	inv := res.Invoices[0]

	assert.Equal(t, "49367445", inv.InvoiceNo)
	assert.Equal(t, "2025-06-04", inv.InvoiceDate)
	require.NotNil(t, inv.Amount, "不含税金额必须提取")
	assert.InDelta(t, 234.25, *inv.Amount, 0.001)
	require.NotNil(t, inv.TaxRate, "税率必须提取")
	assert.InDelta(t, 0.03, *inv.TaxRate, 1e-9, "3%% 应归一为 0.03")
	require.NotNil(t, inv.Tax, "税额必须提取")
	assert.InDelta(t, 7.03, *inv.Tax, 0.001)
	require.NotNil(t, inv.TotalAmount, "价税合计必须提取")
	assert.InDelta(t, 241.28, *inv.TotalAmount, 0.001)

	// 明细行子列
	require.Len(t, inv.Items, 1)
	line := inv.Items[0]
	assert.Equal(t, "渝C12345", line.Extra["plate_no"])
	assert.Equal(t, "货车", line.Extra["vehicle_type"])
	assert.Equal(t, "2025-05-05", line.Extra["travel_from"])
	assert.Equal(t, "2025-05-28", line.Extra["travel_to"])
	// 项目名不得残留「项目名称」前缀与合并进来的「合 计」
	assert.NotContains(t, line.Name, "项目名称")
	assert.NotContains(t, line.Name, "合 计")
	assert.Contains(t, line.Name, "通行费")
}

// TestExtractInvoiceMineruSpacedPartyLabelsFillBuyerSeller 锁定购销方标签空格容错。
// 修复前：`购 买 方` / `销 售 方` 不匹配 → 购销方四字段（名称/税号）全空。
func TestExtractInvoiceMineruSpacedPartyLabelsFillBuyerSeller(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(mineruInvoiceVariantB())
	require.True(t, ok)
	inv := res.Invoices[0]

	assert.Equal(t, "重庆示例铜业有限公司", inv.BuyerName, "带空格的「购 买 方」也必须解析出名称")
	assert.Equal(t, "91500000MA5U54AJ0A", inv.BuyerTaxNo)
	assert.Equal(t, "示例高速公路有限公司乙", inv.SellerName)
	assert.Equal(t, "91500000696569675Y", inv.SellerTaxNo)

	// 名称不得吞掉税号/地址/开户行（修复前 buyer_name 是整块拼接文本）
	for _, leaked := range []string{"纳税人识别号", "地址", "开户行", "91500000MA5U54AJ0A"} {
		assert.NotContains(t, inv.BuyerName, leaked, "购买方名称不得混入 %q", leaked)
		assert.NotContains(t, inv.SellerName, leaked, "销售方名称不得混入 %q", leaked)
	}
}

// TestExtractInvoiceMineruNonSpacedPartyLabelsStillWork 回归保护：无空格形态不得被改坏。
func TestExtractInvoiceMineruNonSpacedPartyLabelsStillWork(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(mineruInvoiceVariantA())
	require.True(t, ok)
	inv := res.Invoices[0]
	assert.Equal(t, "重庆示例铜业有限公司", inv.BuyerName)
	assert.Equal(t, "91500000MA5U54AJ0A", inv.BuyerTaxNo)
	assert.Equal(t, "示例高速公路有限公司甲", inv.SellerName)
	assert.Equal(t, "91500000MACGNJL47X", inv.SellerTaxNo)
	require.NotNil(t, inv.Amount)
	assert.InDelta(t, 158.51, *inv.Amount, 0.001)
	require.NotNil(t, inv.Tax)
	assert.InDelta(t, 14.27, *inv.Tax, 0.001)
	require.NotNil(t, inv.TaxRate)
	assert.InDelta(t, 0.09, *inv.TaxRate, 1e-9)
}

// TestExtractInvoiceMineruFullDocumentEveryInvoiceComplete 锁定端到端完整性：
// 两种空格形态混排时，每张票的 10 个核心字段都必须完整。
func TestExtractInvoiceMineruFullDocumentEveryInvoiceComplete(t *testing.T) {
	doc := mineruInvoiceVariantB() + "\n\n" + mineruInvoiceVariantA()
	res, ok := ExtractInvoicesByFlatLayout(doc)
	require.True(t, ok)
	require.Len(t, res.Invoices, 2, "两张票都要解析出来")

	for i, inv := range res.Invoices {
		assert.NotEmpty(t, inv.InvoiceNo, "第 %d 张缺发票号", i+1)
		assert.NotEmpty(t, inv.InvoiceDate, "第 %d 张缺开票日期", i+1)
		assert.NotNil(t, inv.Amount, "第 %d 张缺金额", i+1)
		assert.NotNil(t, inv.TaxRate, "第 %d 张缺税率", i+1)
		assert.NotNil(t, inv.Tax, "第 %d 张缺税额", i+1)
		assert.NotNil(t, inv.TotalAmount, "第 %d 张缺价税合计", i+1)
		assert.NotEmpty(t, inv.BuyerName, "第 %d 张缺购买方", i+1)
		assert.NotEmpty(t, inv.BuyerTaxNo, "第 %d 张缺购买方税号", i+1)
		assert.NotEmpty(t, inv.SellerName, "第 %d 张缺销售方", i+1)
		assert.NotEmpty(t, inv.SellerTaxNo, "第 %d 张缺销售方税号", i+1)
	}
}

// TestExtractInvoiceRuleCompletenessGate 锁定「半残不得静默短路」：
// 规则产出的发票若缺核心字段，必须被识别为不完整，供上层决定回退 LLM。
//
// 2026-10-20 收严：发票号/开票日期也是必填（发票六要素），缺失同样判不完整 ——
// 真机出现过「只有价税合计、缺票号/日期」的半残票被当成完整而静默跳过 LLM。
// 因此下面 complete 样本必须带 InvoiceDate。
func TestExtractInvoiceRuleCompletenessGate(t *testing.T) {
	complete := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{
		InvoiceNo: "1", InvoiceDate: "2026-08-07",
		Amount: f64(1), Tax: f64(0.09), TotalAmount: f64(1.09),
		BuyerName: "甲", BuyerTaxNo: "91500000000000000A",
		SellerName: "乙", SellerTaxNo: "91500000000000000B",
	}}}
	assert.False(t, InvoiceRuleResultIncomplete(complete), "字段齐全不应判为不完整")

	// 缺开票日期 → 同样不完整（2026-10-20 新增判据）
	noDate := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{
		InvoiceNo: "1",
		Amount: f64(1), Tax: f64(0.09), TotalAmount: f64(1.09),
		BuyerName: "甲", BuyerTaxNo: "91500000000000000A",
		SellerName: "乙", SellerTaxNo: "91500000000000000B",
	}}}
	assert.True(t, InvoiceRuleResultIncomplete(noDate), "缺开票日期必须判为不完整")

	partial := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{{
		InvoiceNo: "1", TotalAmount: f64(1.09),
	}}}
	assert.True(t, InvoiceRuleResultIncomplete(partial), "缺金额/购销方必须判为不完整")

	// 空结果不算「半残」（由调用方按 ruleOK=false 处理）
	assert.False(t, InvoiceRuleResultIncomplete(&InvoiceExtractionResult{Kind: "invoice"}))
}

// mineruInvoiceTaxExempt 复刻票号 34013247：**表头行 + 数值行分离**的老格式，
// 且该票为「不征税」（税率列字面写「不征税」、税额列是 OCR 乱码）。
// 这类票税额本来就应为 0，不能因为解析不出数字就留空。
func mineruInvoiceTaxExempt() string {
	return `发票代码: 050002400112

发票号码: 34013247

开票日期: 2025年02月08日

<table border=1>
<tr><td>购买方</td><td colspan="3">名 称：示例铜业有限公司\n纳税人识别号：91500000MA5U54AJ0A\n地 址、电 话：示例市示例路1号 023-00000000\n开户行及账号：示例银行 50050118370000000047</td></tr>
<tr><td>项目名称</td><td>车牌号</td><td>类型</td><td>通行日期起</td><td>通行日期止</td><td>金 额</td><td>税率</td><td>税 额</td></tr>
<tr><td>经营租赁代收通行费</td><td>渝C12345</td><td>货车</td><td>20250114</td><td>20250119</td><td>73.28</td><td>不征税</td><td>$ ^{{***}} $</td></tr>
<tr><td>合 计</td><td></td><td></td><td></td><td></td><td>¥73.28</td><td></td><td></td></tr>
<tr><td>价税合计（大写）</td><td colspan="7">柒拾叁元贰角捌分 （小写） ¥73.28</td></tr>
<tr><td>销售方</td><td colspan="3">名 称：示例科技公司\n纳税人识别号：91500000327752319D\n地 址、电 话：示例市示例路2号023-00000000\n开户行及账号：示例银行 3100038109000016982</td></tr>
</table>`
}

// TestExtractInvoiceMineruTaxExemptIsZero 锁定免税票税率/税额为 0 而非空。
func TestExtractInvoiceMineruTaxExemptIsZero(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(mineruInvoiceTaxExempt())
	require.True(t, ok)
	inv := res.Invoices[0]
	assert.Equal(t, "34013247", inv.InvoiceNo)
	require.NotNil(t, inv.Amount)
	assert.InDelta(t, 73.28, *inv.Amount, 0.001)
	require.NotNil(t, inv.TaxRate, "「不征税」必须解析为 0%% 而不是空")
	assert.InDelta(t, 0.0, *inv.TaxRate, 1e-9)
	require.NotNil(t, inv.Tax, "免税票税额必须为 0 而不是空")
	assert.InDelta(t, 0.0, *inv.Tax, 1e-9)
	require.NotNil(t, inv.TotalAmount)
	assert.InDelta(t, 73.28, *inv.TotalAmount, 0.001)
}

// TestExtractInvoiceMineruLiteralEscapesStripped 锁定单元格内**字面量 `\n`**
// 必须被清掉：MinerU 偶尔把换行转义成两个字符，会污染购销方名称。
func TestExtractInvoiceMineruLiteralEscapesStripped(t *testing.T) {
	res, ok := ExtractInvoicesByFlatLayout(mineruInvoiceTaxExempt())
	require.True(t, ok)
	inv := res.Invoices[0]
	assert.Equal(t, "示例铜业有限公司", inv.BuyerName, "名称不得带字面量 \\n")
	assert.Equal(t, "91500000MA5U54AJ0A", inv.BuyerTaxNo)
	assert.Equal(t, "示例科技公司", inv.SellerName)
	assert.Equal(t, "91500000327752319D", inv.SellerTaxNo)
	for _, v := range []string{inv.BuyerName, inv.SellerName, inv.BuyerAddr, inv.SellerAddr, inv.BuyerBank, inv.SellerBank} {
		assert.NotContains(t, v, `\n`, "任何字段都不得残留字面量 \\n")
	}
}
