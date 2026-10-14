package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 合成通行费汇总单文本：格式完全复刻系统生成的「通行费电子票据汇总单」，
// 包含号码行+金额行配对、模板噪音（localhost 水印/空购销方模板/图片引用）。
const syntheticTollInvoiceDoc = `开票日期：
购
买
方
信
息
名称：
统一社会信用代码/纳税人识别号：
销
售
方
信
息
名称：
统一社会信用代码/纳税人识别号：
项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额
合 计
价税合计（大写） （小写）
备
注
开票人:
2024/12/12 15:23 localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i…
localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i6n&_ij_reload=REL… 1/1

11110000000000000001 2026年05月07日 91500227MA5U54AJ0A 测试高速甲有限公司 9150000075307715XY *经营租赁*通行费 渝A10001 货车 20260402 225.98 3% 6.78
¥225.98 贰佰叁拾贰圆柒角陆分 宋力 ¥232.76 20260428 测试购买方有限公司 ¥6.78

![通行费电子票据汇总单(发票)_p1_img1.jpg](resource://abc123)

开票日期：
购
买
方
信
息
名称：
统一社会信用代码/纳税人识别号：
销
售
方
信
息
名称：
统一社会信用代码/纳税人识别号：
项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额
合 计
价税合计（大写） （小写）
备
注
开票人:
2024/12/12 15:23 localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i…

localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i6n&_ij_reload=REL… 1/1
11110000000000000002 2026年05月07日 91500227MA5U54AJ0A 测试高速乙有限公司 9150000075307715XY *经营租赁*通行费 渝A10002 货车 20260418 80.02 3% 2.40

¥80.02 捌拾贰圆肆角贰分 王琳琳 ¥82.42 20260426 测试购买方有限公司 ¥2.40

![通行费电子票据汇总单(发票)_p2_img1.jpg](resource://def456)

开票日期：
购
买
方
信
息
名称：
统一社会信用代码/纳税人识别号：
销
售
方
信
息
名称：
统一社会信用代码/纳税人识别号：
项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额
合 计
价税合计（大写） （小写）
备
注
开票人:
2024/12/12 15:23 localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i…

localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i6n&_ij_reload=REL… 1/1
11110000000000000003 2026年05月07日 91500227MA5U54AJ0A 测试高速丙有限公司 9150000075307715XY *经营租赁*通行费 渝A10003 货车 20260425 2.60 3% 0.08
¥2.60 贰圆陆角捌分 张莹月 ¥2.68 20260425 测试购买方有限公司 ¥0.08
`

func TestExtractInvoicesByLineRules_HitAll(t *testing.T) {
	res, ok := ExtractInvoicesByLineRules(syntheticTollInvoiceDoc)
	require.True(t, ok, "规则应命中该格式")
	require.NotNil(t, res)
	assert.Equal(t, "invoice", res.Kind)
	require.Len(t, res.Invoices, 3, "应提取全部 3 张发票")

	inv0 := res.Invoices[0]
	assert.Equal(t, "11110000000000000001", inv0.InvoiceNo)
	assert.Equal(t, "2026-05-07", inv0.InvoiceDate)
	assert.Equal(t, "普通发票", inv0.InvoiceType)
	assert.Equal(t, "测试高速甲有限公司", inv0.SellerName)
	assert.Equal(t, "9150000075307715XY", inv0.SellerTaxNo)
	assert.Equal(t, "91500227MA5U54AJ0A", inv0.BuyerTaxNo)
	assert.Equal(t, "测试购买方有限公司", inv0.BuyerName)
	assert.Equal(t, "宋力", inv0.Issuer)
	assert.Equal(t, "通行费发票", inv0.Category)
	require.NotNil(t, inv0.Amount)
	assert.InDelta(t, 225.98, *inv0.Amount, 0.001)
	require.NotNil(t, inv0.TotalAmount)
	assert.InDelta(t, 232.76, *inv0.TotalAmount, 0.001)
	require.NotNil(t, inv0.Tax)
	assert.InDelta(t, 6.78, *inv0.Tax, 0.001)
	require.NotNil(t, inv0.TaxRate)
	assert.InDelta(t, 0.03, *inv0.TaxRate, 0.0001)
	assert.Contains(t, inv0.Remark, "车牌号：渝A10001")
	assert.Contains(t, inv0.Remark, "通行日期起：20260402")
	assert.Contains(t, inv0.Remark, "通行日期止：20260428")
	// 车牌号必须同步落入动态字段扩展桶（fields），列表「车牌号」列才能读到值
	assert.Equal(t, "渝A10001", inv0.Fields["车牌号"])
	require.Len(t, inv0.Items, 1)
	assert.Equal(t, "经营租赁", inv0.Items[0].Name)

	// 第二张发票：号码行与金额行被噪音拆开（跨 chunk 场景）
	inv1 := res.Invoices[1]
	assert.Equal(t, "11110000000000000002", inv1.InvoiceNo)
	assert.Equal(t, "王琳琳", inv1.Issuer)
	assert.Contains(t, inv1.Remark, "通行日期止：20260426")
	require.NotNil(t, inv1.TaxRate)
	assert.InDelta(t, 0.03, *inv1.TaxRate, 0.0001)

	inv2 := res.Invoices[2]
	assert.Equal(t, "11110000000000000003", inv2.InvoiceNo)
	require.NotNil(t, inv2.TaxRate)
	assert.InDelta(t, 0.03, *inv2.TaxRate, 0.0001)
}

// 回归样本：复刻线上 19 页通行费汇总单的两类故障——
//  1. 分块 overlap 导致金额行在相邻 chunk 各出现一次（完全重复），旧实现两个数组
//     按下标配对，从第 2 张起金额整体串行错位；
//  2. 长销售方名称被 PDF 文本层折断成多行，旧实现整行正则失配导致整页漏提。
const syntheticTollInvoiceOverlapDoc = `11110000000000000001 2026年05月07日 91500227MA5U54AJ0A 测试高速甲有限公司 9150000075307715XY *经营租赁*通行费 渝A10001 货车 20260402 225.98 3% 6.78
¥225.98 贰佰叁拾贰圆柒角陆分 宋力 ¥232.76 20260428 测试购买方有限公司 ¥6.78
¥225.98 贰佰叁拾贰圆柒角陆分 宋力 ¥232.76 20260428 测试购买方有限公司 ¥6.78
22220000000000000002 2026年05月07日 91500227MA5U54AJ0A
四川成渝高速公路集团股份有限公司公路运营管
理一分公司 915100000983301971 *生产生活服务*通行费 渝C87567 货车 20260918 43.16 3% 1.29
¥43.16 肆拾肆圆肆角伍分 刘珊 ¥44.45 20260918 测试购买方有限公司 ¥1.29
33330000000000000003 2026年05月07日 91500227MA5U54AJ0A 测试高速丙有限公司 9150000075307715XY *经营租赁*通行费 渝A10003 货车 20260425 2.60 3% 0.08
¥2.60 贰圆陆角捌分 张莹月 ¥2.68 20260425 测试购买方有限公司 ¥0.08
¥2.60 贰圆陆角捌分 张莹月 ¥2.68 20260425 测试购买方有限公司 ¥0.08
`

func TestExtractInvoicesByLineRules_OverlapAndBrokenLine(t *testing.T) {
	res, ok := ExtractInvoicesByLineRules(syntheticTollInvoiceOverlapDoc)
	require.True(t, ok, "规则应命中该格式")
	require.NotNil(t, res)
	require.Len(t, res.Invoices, 3, "跨行断行不得漏页，必须提取全部 3 张")

	// 第 1 张：金额行重复两次不得造成配对错位
	inv0 := res.Invoices[0]
	assert.Equal(t, "11110000000000000001", inv0.InvoiceNo)
	assert.Equal(t, "宋力", inv0.Issuer)
	require.NotNil(t, inv0.Amount)
	assert.InDelta(t, 225.98, *inv0.Amount, 0.001)
	require.NotNil(t, inv0.TotalAmount)
	assert.InDelta(t, 232.76, *inv0.TotalAmount, 0.001)

	// 第 2 张：号码行被折断成 3 行，必须跨行合并命中，销售方名称完整
	inv1 := res.Invoices[1]
	assert.Equal(t, "22220000000000000002", inv1.InvoiceNo, "折断的号码行必须被跨行合并识别，不得漏页")
	assert.Equal(t, "四川成渝高速公路集团股份有限公司公路运营管理一分公司", inv1.SellerName)
	assert.Equal(t, "915100000983301971", inv1.SellerTaxNo)
	assert.Equal(t, "刘珊", inv1.Issuer, "金额必须各归各页，不得串行成第 1 张的宋力/225.98")
	require.NotNil(t, inv1.Amount)
	assert.InDelta(t, 43.16, *inv1.Amount, 0.001)
	require.NotNil(t, inv1.TotalAmount)
	assert.InDelta(t, 44.45, *inv1.TotalAmount, 0.001)
	assert.Contains(t, inv1.Remark, "车牌号：渝C87567")
	assert.Contains(t, inv1.Remark, "通行日期止：20260918")
	assert.Equal(t, "渝C87567", inv1.Fields["车牌号"], "断行合并路径的车牌号同样必须落 fields 桶")

	// 第 3 张：在两张重复金额行之后仍必须配对到自身金额（旧实现会错位成 225.98）
	inv2 := res.Invoices[2]
	assert.Equal(t, "33330000000000000003", inv2.InvoiceNo)
	assert.Equal(t, "张莹月", inv2.Issuer)
	require.NotNil(t, inv2.Amount)
	assert.InDelta(t, 2.60, *inv2.Amount, 0.001)
	require.NotNil(t, inv2.TotalAmount)
	assert.InDelta(t, 2.68, *inv2.TotalAmount, 0.001)
}

func TestExtractInvoicesByLineRules_Miss(t *testing.T) {
	// 非该固定格式的文本（普通合同文本）应返回未命中，交由模型兜底。
	res, ok := ExtractInvoicesByLineRules("这是一份合同文本，不包含通行费发票行格式。")
	assert.False(t, ok)
	assert.Nil(t, res)

	_, ok = ExtractInvoicesByLineRules("")
	assert.False(t, ok)
}

func TestCleanInvoiceTemplateNoise(t *testing.T) {
	lines := cleanInvoiceTemplateNoise(syntheticTollInvoiceDoc)
	for _, ln := range lines {
		assert.NotContains(t, ln, "localhost:63342", "localhost 水印必须被清洗")
		assert.NotContains(t, ln, "![", "markdown 图片引用必须被清洗")
	}
	// 噪音清洗后应只剩余有效行（号码行×3 + 金额行×3）
	headCount, amtCount := 0, 0
	for _, ln := range lines {
		if invoiceHeadLineRe.MatchString(ln) {
			headCount++
		}
		if invoiceAmountLineRe.MatchString(ln) {
			amtCount++
		}
	}
	assert.Equal(t, 3, headCount, "应保留 3 个号码行")
	assert.Equal(t, 3, amtCount, "应保留 3 个金额行")
}

// syntheticCompactTollInvoiceDoc 复刻 docreader 新输出的「单行紧凑」格式：
// 号码段与金额段合并为同一行（模板头 + 单行完整数据，无 | 表格符号）。
// 数据行取自真实通行费汇总单 chunk（含开发机路径水印噪音行）。
const syntheticCompactTollInvoiceDoc = `开票日期：
购
买
方
信
息
名称：
统一社会信用代码/纳税人识别号：
销
售
方
信
息
名称：
统一社会信用代码/纳税人识别号：
项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额
合 计
价税合计（大写） （小写）
备
注
开票人:
localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i6n&_ij_reload=REL… 1/1
通行费
26507903161000014240 2026年10月08日 91500227MA5U54AJ0A 重庆渝蓉高速公路有限公司 9150000055675000X0 *生产生活服务*通行费 渝C81022 货车 20260903 1418.36 3% 42.55 ¥1418.36 壹仟肆佰陆拾圆玖角壹分 杨希 ¥1460.91 20260930 重庆星达铜业有限公司 ¥42.55

![通行费电子票据汇总单(发票)_p1_img1.jpg](resource://oH-kEmAcTws2cLtgU21BFw)

开票日期：
购
买
方
信
息
名称：
统一社会信用代码/纳税人识别号：
销
售
方
信
息
名称：
统一社会信用代码/纳税人识别号：
项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额
合 计
价税合计（大写） （小写）
备
注
开票人:
localhost:63342/cloud/service/invoice/invoice-alle/src/main/resources/templates/invoice2.html?_ijt=ujph1nj9mes58bk0psc9hf4i6n&_ij_reload=REL… 1/1
通行费
26517905111000001775 2026年10月08日 91500227MA5U54AJ0A 内江城建总诚绕城高速公路投资有限公司 91511000095586330H *生产生活服务*通行费 渝C81022 货车 20260903 282.72 3% 8.48 ¥282.72 贰佰玖拾壹圆贰角整 敬希艳 ¥291.20 20260930 重庆星达铜业有限公司 ¥8.48
`

func TestExtractInvoicesByLineRules_CompactSingleLine(t *testing.T) {
	res, ok := ExtractInvoicesByLineRules(syntheticCompactTollInvoiceDoc)
	require.True(t, ok, "单行紧凑格式必须被规则命中")
	require.NotNil(t, res)
	assert.Equal(t, "invoice", res.Kind)
	require.Len(t, res.Invoices, 2, "应提取全部 2 张发票")

	inv0 := res.Invoices[0]
	assert.Equal(t, "26507903161000014240", inv0.InvoiceNo)
	assert.Equal(t, "2026-10-08", inv0.InvoiceDate)
	assert.Equal(t, "普通发票", inv0.InvoiceType)
	assert.Equal(t, "通行费发票", inv0.Category)
	assert.Equal(t, "重庆渝蓉高速公路有限公司", inv0.SellerName)
	assert.Equal(t, "9150000055675000X0", inv0.SellerTaxNo)
	assert.Equal(t, "91500227MA5U54AJ0A", inv0.BuyerTaxNo)
	assert.Equal(t, "重庆星达铜业有限公司", inv0.BuyerName)
	assert.Equal(t, "杨希", inv0.Issuer)
	require.NotNil(t, inv0.Amount)
	assert.InDelta(t, 1418.36, *inv0.Amount, 0.001)
	require.NotNil(t, inv0.TotalAmount)
	assert.InDelta(t, 1460.91, *inv0.TotalAmount, 0.001)
	require.NotNil(t, inv0.Tax)
	assert.InDelta(t, 42.55, *inv0.Tax, 0.001)
	require.NotNil(t, inv0.TaxRate)
	assert.InDelta(t, 0.03, *inv0.TaxRate, 0.0001)
	assert.Contains(t, inv0.Remark, "车牌号：渝C81022")
	assert.Contains(t, inv0.Remark, "通行日期起：20260903")
	assert.Contains(t, inv0.Remark, "通行日期止：20260930")
	// 车牌号必须同步落入动态字段扩展桶（fields）
	assert.Equal(t, "渝C81022", inv0.Fields["车牌号"])
	require.Len(t, inv0.Items, 1)
	assert.Equal(t, "生产生活服务", inv0.Items[0].Name)

	// 第二张：较长销售方名称、含「整」的大写金额
	inv1 := res.Invoices[1]
	assert.Equal(t, "26517905111000001775", inv1.InvoiceNo)
	assert.Equal(t, "内江城建总诚绕城高速公路投资有限公司", inv1.SellerName)
	assert.Equal(t, "敬希艳", inv1.Issuer)
	require.NotNil(t, inv1.TaxRate)
	assert.InDelta(t, 0.03, *inv1.TaxRate, 0.0001)
	require.NotNil(t, inv1.TotalAmount)
	assert.InDelta(t, 291.20, *inv1.TotalAmount, 0.001)
	assert.Equal(t, "渝C81022", inv1.Fields["车牌号"])
}

// cleanInvoiceItemSpec：票面规格列为空时模型常把单位列误填到 spec，
// spec 与 unit 文本相同必须清空 spec；不同或其一为空保持原值。
func TestCleanInvoiceItemSpec(t *testing.T) {
	items := []InvoiceExtractionLine{
		{Name: "供电*电费", Spec: "千瓦时", Unit: "千瓦时", Qty: ptrF64(666074), Price: ptrF64(0.6175)},
		{Name: "通行费", Spec: "", Unit: "次", Qty: ptrF64(1), Price: ptrF64(225.98)},
		{Name: "咨询服务", Spec: "A4", Unit: "份", Qty: ptrF64(2), Price: ptrF64(50)},
		{Name: "水费", Spec: "吨", Unit: "吨", Qty: ptrF64(100), Price: ptrF64(3.5)},
	}
	cleanInvoiceItemSpec(items)
	assert.Equal(t, "", items[0].Spec, "spec==unit（单位误填规格）时必须清空")
	assert.Equal(t, "", items[1].Spec, "spec 为空保持空")
	assert.Equal(t, "A4", items[2].Spec, "spec!=unit 保留原值")
	assert.Equal(t, "", items[3].Spec, "spec==unit（单位误填规格）时必须清空")
	assert.Equal(t, "吨", items[3].Unit, "unit 不受影响")

	// NormalizeInvoiceExtractionResult 链路：结果级清洗同样生效
	qty := 1.0
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo: "26507000000186528429",
			Items:     []InvoiceExtractionLine{{Name: "*供电*电费", Spec: "千瓦时", Unit: "千瓦时", Qty: &qty}},
		}},
	}
	NormalizeInvoiceExtractionResult(res)
	assert.Equal(t, "", res.Invoices[0].Items[0].Spec)
}

func ptrF64(v float64) *float64 { return &v }

