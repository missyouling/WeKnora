package service

import (
	"testing"
)

// 成品油增值税专用发票解析（第 24 轮任务二）。
//
// 真机样本：2026.9-10油费发票.pdf（三张票，一页一票）。
// 事故现象（图 2）：
//   · 明细列错位：规格型号显示 `车用柴油(VI)`、单位显示 `0#柴油`，真正的单位 `升` 丢了；
//   · P2 合计被算成 3665.38（票面 6690.23）—— 折扣行被丢弃/被当垃圾行；
//   · P3 明细全空（整行压进一个 colspan=9 格）；
//   · P2 开票人漏提（段落切分把上一张票的尾部吃进来）。
//
// 本规则的核心是**从右往左锚定数值列**：税额 ← 税率 ← 金额 永远成立，
// 再往左的四段（单价/数量/单位/规格型号）只在有正数数量时存在 ——
// 折扣行天然只有右侧三段，左侧全空，不会错位。

// ---- 列锚定 ----

func TestParseFuelDetailRowFullLine(t *testing.T) {
	// 真机 P1 首行：9 列完整
	cells := []string{"*柴油*0号", "车用柴油(VI)", "0#柴油", "升", "7085.54", "7.15884181022", "50724.26", "13%", "6594.15"}
	r, ok := parseFuelDetailRow(cells)
	if !ok {
		t.Fatal("完整明细行必须解析成功")
	}
	if r.Name != "*柴油*0号" {
		t.Errorf("项目名称应为 *柴油*0号，实际 %q", r.Name)
	}
	// **这是图 2 错位的核心**：规格型号是 `0#柴油`，不是 `车用柴油(VI)`
	if r.Spec != "0#柴油" {
		t.Errorf("规格型号应为 0#柴油，实际 %q（错位回归）", r.Spec)
	}
	if r.Unit != "升" {
		t.Errorf("单位应为 升，实际 %q（错位回归）", r.Unit)
	}
	if r.Qty == nil || *r.Qty != 7085.54 {
		t.Errorf("数量应为 7085.54，实际 %v", r.Qty)
	}
	if r.Price == nil || *r.Price < 7.15 || *r.Price > 7.16 {
		t.Errorf("单价应约 7.1588，实际 %v", r.Price)
	}
	if r.Amount == nil || *r.Amount != 50724.26 {
		t.Errorf("金额应为 50724.26，实际 %v", r.Amount)
	}
	if r.TaxRate == nil || *r.TaxRate != 0.13 {
		t.Errorf("税率应为 0.13，实际 %v", r.TaxRate)
	}
	if r.TaxAmt == nil || *r.TaxAmt != 6594.15 {
		t.Errorf("税额应为 6594.15，实际 %v", r.TaxAmt)
	}
}

func TestParseFuelDetailRowDiscountLine(t *testing.T) {
	// 折扣行：只有 项目名称 + 规格 + 负金额 + 税率 + 负税额
	cells := []string{"*柴油*0号", "车用柴油(VI)", "-513.35", "13%", "-66.73"}
	r, ok := parseFuelDetailRow(cells)
	if !ok {
		t.Fatal("折扣行必须解析成功（旧实现把负数当非金额而整行丢弃）")
	}
	if r.Amount == nil || *r.Amount != -513.35 {
		t.Errorf("折扣金额应为 -513.35，实际 %v", r.Amount)
	}
	if r.TaxAmt == nil || *r.TaxAmt != -66.73 {
		t.Errorf("折扣税额应为 -66.73，实际 %v", r.TaxAmt)
	}
	if !r.Discount {
		t.Error("负金额行必须标记为折扣行")
	}
	// **折扣行的规格/单位/数量/单价必须留空**（不得错位）
	if r.Spec != "" || r.Unit != "" || r.Qty != nil || r.Price != nil {
		t.Errorf("折扣行左侧四段必须留空，实际 spec=%q unit=%q qty=%v price=%v",
			r.Spec, r.Unit, r.Qty, r.Price)
	}
	if r.Name != "*柴油*0号" {
		t.Errorf("折扣行名称应为 *柴油*0号，实际 %q", r.Name)
	}
}

func TestParseFuelDetailRowWithCommaThousands(t *testing.T) {
	// 千分位金额（票面偶有渲染成 50,724.26）
	cells := []string{"*柴油*0号", "0#柴油", "升", "100", "10", "1,000.00", "13%", "130.00"}
	r, ok := parseFuelDetailRow(cells)
	if !ok {
		t.Fatal("千分位金额必须解析成功")
	}
	if r.Amount == nil || *r.Amount != 1000 {
		t.Errorf("金额应为 1000，实际 %v", r.Amount)
	}
}

func TestParseFuelDetailRowSkipsHeader(t *testing.T) {
	// 表头行不得被当成明细
	cells := []string{"项目名称", "规格型号", "单位", "数量", "单价", "金额", "税率/征收率", "税额"}
	if _, ok := parseFuelDetailRow(cells); ok {
		t.Fatal("表头行不得解析为明细")
	}
}

func TestParseFuelDetailRowSkipsSummaryRows(t *testing.T) {
	// 合计行 / 价税合计行 / 备注行 / 购销方行都不得当明细
	for _, cells := range [][]string{
		{"合计", "¥51989.49", "¥6758.63"},
		{"价税合计(大写)", "伍万捌仟柒佰肆拾捌圆壹角贰分", "(小写) ¥58748.12"},
		{"备注", "购方地址:...;银行账号:50050118370000000047"},
		{"购买方信息", "名称:重庆星达铜业有限公司"},
		{"销售方信息", "名称:中国石油天然气股份有限公司"},
	} {
		if _, ok := parseFuelDetailRow(cells); ok {
			t.Errorf("非明细行被误判: %v", cells)
		}
	}
}

// ---- 跨行括号合并 ----

func TestMergeFuelContinuations(t *testing.T) {
	// 真机形态：规格型号 `车用汽油(VI` 被换行切断，下一行只有 `B)`
	rows := [][]string{
		{"*汽油*92号", "车用汽油(VI)", "92#汽油", "升", "115.67", "7.63", "882.79", "13%", "114.76"},
		{"B)", "", "", "", "", "", "", "", ""},
	}
	merged := mergeFuelContinuations(rows)
	if len(merged) != 1 {
		t.Fatalf("括号尾巴必须并回上一行，实际 %d 行", len(merged))
	}
	if merged[0][1] != "车用汽油(VIB)" {
		t.Errorf("规格型号应并成 车用汽油(VIB)，实际 %q", merged[0][1])
	}
}

func TestMergeFuelContinuationsDoesNotMergeRealRows(t *testing.T) {
	// 真实明细行（多格非空）不得被当成括号尾巴
	rows := [][]string{
		{"*柴油*0号", "0#柴油", "升", "100", "7.5", "750", "13%", "97.5"},
		{"*汽油*92号", "92#汽油", "升", "50", "7.6", "380", "13%", "49.4"},
	}
	merged := mergeFuelContinuations(rows)
	if len(merged) != 2 {
		t.Fatalf("真实明细行不得合并，实际 %d 行", len(merged))
	}
}

// ---- 整行压进单格 ----

func TestSplitFuelMergedRow(t *testing.T) {
	// 真机 P3：表头 + 两条明细全在一个 colspan=9 格里
	raw := "项目名称 规格型号 单位 数量 单价 金额 税率/征收率 税额" +
		"*柴油*0号 车用柴油(VI) 0#柴油 升 302.42 7.49649494081 2267.09 13% 294.72" +
		"*柴油*0号 车用柴油(VI) -54.79 13% -7.12"
	sub := splitFuelMergedRow(raw)
	if len(sub) != 2 {
		t.Fatalf("应切出 2 条明细，实际 %d", len(sub))
	}
	// 第一条：完整 9 列
	if len(sub[0]) != 9 {
		t.Errorf("第一条应为 9 格，实际 %d: %v", len(sub[0]), sub[0])
	}
	// 第二条：折扣行 5 格
	if len(sub[1]) != 5 {
		t.Errorf("第二条（折扣行）应为 5 格，实际 %d: %v", len(sub[1]), sub[1])
	}
	// 逐条解析验证
	r1, ok := parseFuelDetailRow(sub[0])
	if !ok || r1.Spec != "0#柴油" || r1.Unit != "升" {
		t.Errorf("第一条应 spec=0#柴油 unit=升，实际 spec=%q unit=%q", r1.Spec, r1.Unit)
	}
	r2, ok := parseFuelDetailRow(sub[1])
	if !ok || r2.Amount == nil || *r2.Amount != -54.79 {
		t.Errorf("第二条折扣金额应为 -54.79，实际 %v", r2.Amount)
	}
}

func TestSplitFuelMergedRowNoAnchor(t *testing.T) {
	// 没有 `*类别*` 锚点 → 不切（交给上层按普通行处理）
	if sub := splitFuelMergedRow("合计 ¥2212.30 ¥287.60"); len(sub) != 0 {
		t.Errorf("无锚点时应返回空，实际 %v", sub)
	}
}

// ---- 银行账号不得成为幽灵票 ----

func TestFuelRuleIgnoresBankAccountAsInvoiceNo(t *testing.T) {
	// 备注里的 20 位银行账号绝不能被当成第 4 张票（图 1 第 20 行的幽灵 P4）
	raw := `成品油
发票号码：26507000000327644626
开票日期：2026年10月31日
<table>
<tr><td>备注</td><td>银行账号:50050118370000000047;收款人:梁强;</td></tr></table>
开票人：叶南
`
	res, ok := ExtractInvoicesByFuelRules(raw)
	if !ok || res == nil {
		t.Fatal("成品油票必须命中")
	}
	if len(res.Invoices) != 1 {
		t.Fatalf("只能有 1 张票（银行账号不是票号），实际 %d", len(res.Invoices))
	}
	if res.Invoices[0].InvoiceNo != "26507000000327644626" {
		t.Errorf("票号应为 26507000000327644626，实际 %q", res.Invoices[0].InvoiceNo)
	}
}

func TestFuelInvNoRegexBindsLabel(t *testing.T) {
	// 正则必须绑定「发票号码：」上下文 —— 裸 20 位数字会把银行账号也算进来
	raw := "发票号码：26507000000315988591 银行账号:50050118370000000047"
	m := fuelInvNoLabelRe.FindAllStringSubmatch(raw, -1)
	if len(m) != 1 {
		t.Fatalf("只应命中真实票号，实际 %d 处", len(m))
	}
	if m[0][1] != "26507000000315988591" {
		t.Errorf("命中值应为真实票号，实际 %q", m[0][1])
	}
}

// ---- 票面合计优先于明细之和 ----

func TestFuelRulePrefersTicketTotal(t *testing.T) {
	// 票面合计是权威口径：明细可能因折扣行/跨行截断而残缺，
	// 绝不能用「残缺明细之和」覆盖票面真值（真机 P2 被写成 3665.38 的教训）。
	raw := `成品油
发票号码：26507000000325852049
开票日期：2026年10月30日
<table>
<tr><td colspan="9">项目名称 规格型号 单位 数量 单价 金额 税率/征收率 税额*柴油*0号 车用柴油(VI) 0#柴油 升 865.18 7.39193000301 6395.35 13% 831.39</td></tr>
<tr><td colspan="6">合计</td><td colspan="2">¥6690.23</td><td>¥869.72</td></tr>
<tr><td colspan="3">价税合计(大写)</td><td colspan="3">柒仟伍佰伍拾玖圆玖角伍分</td><td colspan="3">(小写)¥7559.95</td></tr>
</table>
开票人：叶南
`
	res, ok := ExtractInvoicesByFuelRules(raw)
	if !ok || res == nil {
		t.Fatal("必须命中")
	}
	inv := res.Invoices[0]
	if inv.Amount == nil || *inv.Amount != 6690.23 {
		t.Errorf("金额必须取票面合计 6690.23，实际 %v（明细之和只有 6395.35）", inv.Amount)
	}
	if inv.Tax == nil || *inv.Tax != 869.72 {
		t.Errorf("税额必须取票面合计 869.72，实际 %v", inv.Tax)
	}
	if inv.TotalAmount == nil || *inv.TotalAmount != 7559.95 {
		t.Errorf("价税合计必须取 7559.95，实际 %v", inv.TotalAmount)
	}
	if inv.Issuer != "叶南" {
		t.Errorf("开票人必须提取到，实际 %q", inv.Issuer)
	}
}

// ---- 段落切分不跨票 ----

func TestFuelRuleSegmentDoesNotCrossInvoice(t *testing.T) {
	// 段落 = [本票号码行, 下一票号码行)，**绝不往前扩** ——
	// 前扩会把上一张票的尾部（合计/开票人）吃进来（真机 P2 开票人变成 P1 的）。
	raw := `成品油
发票号码：26507000000315988591
<table><tr><td colspan="9">*柴油*0号 车用柴油(VI) 0#柴油 升 100 7 700 13% 91</td></tr>
<tr><td colspan="6">合计</td><td colspan="2">¥51989.49</td><td>¥6758.63</td></tr></table>
开票人：周琳杰

成品油
发票号码：26507000000325852049
<table><tr><td colspan="9">*柴油*0号 车用柴油(VI) 0#柴油 升 865 7.39 6395.35 13% 831.39</td></tr>
<tr><td colspan="6">合计</td><td colspan="2">¥6690.23</td><td>¥869.72</td></tr></table>
开票人：叶南
`
	res, ok := ExtractInvoicesByFuelRules(raw)
	if !ok || res == nil {
		t.Fatal("必须命中两张")
	}
	if len(res.Invoices) != 2 {
		t.Fatalf("应提取 2 张，实际 %d", len(res.Invoices))
	}
	// P2 的开票人必须是它自己的「叶南」，不能是 P1 的「周琳杰」
	if res.Invoices[1].Issuer != "叶南" {
		t.Errorf("第二张开票人应为 叶南，实际 %q（段落前扩会取到上一张的）", res.Invoices[1].Issuer)
	}
	// P1 的合计不能被 P2 覆盖
	if res.Invoices[0].Amount == nil || *res.Invoices[0].Amount != 51989.49 {
		t.Errorf("第一张金额应为 51989.49，实际 %v", res.Invoices[0].Amount)
	}
	if res.Invoices[1].Amount == nil || *res.Invoices[1].Amount != 6690.23 {
		t.Errorf("第二张金额应为 6690.23，实际 %v", res.Invoices[1].Amount)
	}
}

// ---- 明细求和自洽 ----

func TestFuelItemsSumMatchesTicketTotal(t *testing.T) {
	// P1 的 6 条明细之和必须等于票面合计（含折扣行）：
	//   50724.26 - 513.35 + 882.79 - 8.05 + 914.25 - 10.41 = 51989.49
	//   6594.15 - 66.73 + 114.76 - 1.05 + 118.85 - 1.35 = 6758.63
	items := []InvoiceExtractionLine{
		{Amount: moneyPtr(50724.26), TaxAmt: moneyPtr(6594.15)},
		{Amount: moneyPtr(-513.35), TaxAmt: moneyPtr(-66.73)},
		{Amount: moneyPtr(882.79), TaxAmt: moneyPtr(114.76)},
		{Amount: moneyPtr(-8.05), TaxAmt: moneyPtr(-1.05)},
		{Amount: moneyPtr(914.25), TaxAmt: moneyPtr(118.85)},
		{Amount: moneyPtr(-10.41), TaxAmt: moneyPtr(-1.35)},
	}
	sumA, sumT := 0.0, 0.0
	for _, it := range items {
		sumA += *it.Amount
		sumT += *it.TaxAmt
	}
	if got := roundMoney(sumA); got != 51989.49 {
		t.Errorf("明细金额之和应为 51989.49，实际 %v", got)
	}
	if got := roundMoney(sumT); got != 6758.63 {
		t.Errorf("明细税额之和应为 6758.63，实际 %v", got)
	}
}

func TestParsePlainAmountSupportsNegative(t *testing.T) {
	// 折扣行的金额是负数，parsePlainAmount 必须认（旧实现直接判「不是金额」）
	if v := parsePlainAmount("-513.35"); v == nil || *v != -513.35 {
		t.Errorf("负数金额必须解析，实际 %v", v)
	}
	if v := parsePlainAmount("-7.12"); v == nil || *v != -7.12 {
		t.Errorf("负数税额必须解析，实际 %v", v)
	}
	// 负号只允许在首位
	if v := parsePlainAmount("1-2"); v != nil {
		t.Errorf("1-2 不是合法金额，不应解析，实际 %v", v)
	}
	// 纯负号不是金额
	if v := parsePlainAmount("-"); v != nil {
		t.Errorf("单独负号不是金额，实际 %v", v)
	}
}
