package service

import (
	"strconv"
	"testing"
)

// MinerU HTML 表格版通行费汇总单规则（第 23 轮 issue 4）。
//
// 真机背景：一页一票的汇总单，MinerU 输出 HTML `<tr><td>` 表格；三套既有规则
// 全不命中（各自要求同行号码段/管道符/发票代码锚点），整份文档落到 LLM，
// 而模型在这种模板噪音里只回得出票号，金额/税率/税额/价税合计/开票人全空。
//
// 本测试锁死：金额/税率/税额从数据行取到、价税合计由「金额 + 税额」反算、
// 开票人取到、且**绝不跨段落错配**（段落内无数据行时宁可不产出该票）。

// htmlTollFixture 复刻真机 chunk 的形态（号码行 → 表格 → 数据行 → 价税合计 → 开票人）。
const htmlTollFixture = `## 电子发票（普通发票） 国家税务总局

发票号码：26507903161100005511

开票日期：2026年11月02日

<table>
<tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司统一社会信用代码/纳税人识别号:91500227MA5U54AJ0A</td><td>销售方信息</td><td colspan="4">名称:重庆渝蓉高速公路有限公司统一社会信用代码/纳税人识别号:9150000055675000X0</td></tr>
<tr><td colspan="5">项目名称 车牌号 车辆类型 通行日期起</td><td>通行日期止</td><td>金额</td><td>税率/征收率</td><td>税额</td></tr>
<tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td><td colspan="2">20261022</td><td>20261022</td><td>49.24</td><td>3%</td><td>1.48</td></tr>
<tr><td colspan="9">合计 ¥49.24 ¥1.48</td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="4">伍拾圆柒角贰分</td><td>(小写)</td><td colspan="2">¥50.72</td></tr>
<tr><td>备注</td><td colspan="8"></td></tr></table>

开票人：杨希

![](resource://WAK3QAoAK9mpHjhDfaGX2Q)

通行费
`

func TestExtractInvoicesByHTMLTableRulesFullFields(t *testing.T) {
	res, ok := ExtractInvoicesByHTMLTableRules(htmlTollFixture)
	if !ok || res == nil {
		t.Fatal("规则必须命中 HTML 表格版汇总单")
	}
	if len(res.Invoices) != 1 {
		t.Fatalf("应解析出 1 张，实际 %d", len(res.Invoices))
	}
	inv := res.Invoices[0]
	if inv.InvoiceNo != "26507903161100005511" {
		t.Errorf("发票号错：%q", inv.InvoiceNo)
	}
	if inv.InvoiceDate != "2026-11-02" {
		t.Errorf("开票日期应归一为 2026-11-02，实际 %q", inv.InvoiceDate)
	}
	// 图 3 的四项空白（金额/税率/税额/价税合计）必须全部补齐
	if inv.Amount == nil || *inv.Amount != 49.24 {
		t.Errorf("金额应为 49.24，实际 %v", fmtPtr(inv.Amount))
	}
	if inv.TaxRate == nil || *inv.TaxRate != 0.03 {
		t.Errorf("税率应为 0.03，实际 %v", fmtPtr(inv.TaxRate))
	}
	if inv.Tax == nil || *inv.Tax != 1.48 {
		t.Errorf("税额应为 1.48，实际 %v", fmtPtr(inv.Tax))
	}
	if inv.TotalAmount == nil || *inv.TotalAmount != 50.72 {
		t.Errorf("价税合计应为 50.72，实际 %v", fmtPtr(inv.TotalAmount))
	}
	if inv.Issuer != "杨希" {
		t.Errorf("开票人应为 杨希，实际 %q", inv.Issuer)
	}
	if inv.Category != "通行费发票" || inv.InvoiceType != "普通发票" {
		t.Errorf("分类/类型错：%q / %q", inv.Category, inv.InvoiceType)
	}
	if inv.BuyerName != "重庆星达铜业有限公司" || inv.BuyerTaxNo != "91500227MA5U54AJ0A" {
		t.Errorf("购买方错：%q / %q", inv.BuyerName, inv.BuyerTaxNo)
	}
	if inv.SellerName != "重庆渝蓉高速公路有限公司" || inv.SellerTaxNo != "9150000055675000X0" {
		t.Errorf("销售方错：%q / %q", inv.SellerName, inv.SellerTaxNo)
	}
	if len(inv.Items) != 1 || inv.Items[0].Extra["travel_to"] != "20261022" {
		t.Errorf("明细行应带通行日期止，实际 %+v", inv.Items)
	}
}

// 价税合计小写被乱码吃掉时，用「金额 + 税额」反算；金额或税额缺一就不猜。
func TestExtractInvoicesByHTMLTableRulesTotalFallback(t *testing.T) {
	noTotal := `发票号码：26507903161100005511
开票日期：2026年11月02日
<table><tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司统一社会信用代码/纳税人识别号:91500227MA5U54AJ0A</td></tr>
<tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td><td>20261022</td><td>20261022</td><td>49.24</td><td>3%</td><td>1.48</td></tr>
<tr><td colspan="2">价税合计(大写)</td><td colspan="4">伍拾圆柒角贰分</td></tr></table>
`
	res, ok := ExtractInvoicesByHTMLTableRules(noTotal)
	if !ok || len(res.Invoices) != 1 {
		t.Fatal("无小写价税合计时仍应命中")
	}
	if res.Invoices[0].TotalAmount == nil || *res.Invoices[0].TotalAmount != 50.72 {
		t.Errorf("应由 49.24 + 1.48 反算出 50.72，实际 %v", fmtPtr(res.Invoices[0].TotalAmount))
	}
}

// 段落内没有数据行 → 不产出该票（宁可少一张，也不跨段拿邻居的金额）。
func TestExtractInvoicesByHTMLTableRulesNoCrossSegment(t *testing.T) {
	seg := `发票号码：26507903161100005511
开票日期：2026年11月02日
<table><tr><td colspan="5">项目名称 车牌号 车辆类型 通行日期起</td><td>通行日期止</td><td>金额</td><td>税率/征收率</td><td>税额</td></tr></table>
发票号码：26507903161100005512
开票日期：2026年11月03日
<table><tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td><td>20261023</td><td>20261023</td><td>12.00</td><td>3%</td><td>0.36</td></tr></table>
开票人：张三
`
	res, ok := ExtractInvoicesByHTMLTableRules(seg)
	if !ok {
		t.Fatal("第二段有数据行，应命中")
	}
	if len(res.Invoices) != 1 {
		t.Fatalf("只应产出有数据行的那一张，实际 %d", len(res.Invoices))
	}
	if res.Invoices[0].InvoiceNo != "26507903161100005512" {
		t.Errorf("不得把第二段的金额塞给第一张，实际票号 %q", res.Invoices[0].InvoiceNo)
	}
}

// 非 HTML / 无号码标签的文本一律不命中（该规则不能抢别的格式的活）。
func TestExtractInvoicesByHTMLTableRulesMissesOtherFormats(t *testing.T) {
	for _, s := range []string{
		"",
		"发票号码：12345678",             // 没有表格
		"<tr><td>渝A75R1C</td></tr>", // 没有号码标签
		"|项目|车牌|\n|通行费|渝A75R1C|",    // markdown 管道表交给 MarkdownTable 规则
	} {
		if _, ok := ExtractInvoicesByHTMLTableRules(s); ok {
			t.Errorf("不应命中：%q", s)
		}
	}
}

// fmtPtr 打印可空 float 指针（断言失败时给出「nil」而不是 <nil> 的含糊输出）。
func fmtPtr(v *float64) string {
	if v == nil {
		return "nil"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}
