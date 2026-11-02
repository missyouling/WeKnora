package service

import (
	"strconv"
	"testing"
)

// 发票字段补齐与交叉推算（第 24 轮任务二）。
//
// 真机抽样（图 2 三个红框）暴露三类缺项，本组测试逐类锁定：
//   A 车牌号整列为 `-`：车牌常直接出现在明细行（`*通行费 渝D95256 货车 20261006`），
//     没有 `车牌号：` 前缀；顶层 Fields 也可能没写。
//   B 开票人多数空白：票面常渲染成 `开 票 人 ：\n 杨希`（按字断行）。
//   C 金额/税率/税额/价税合计全 0：段落只有表头、数据行落在下一 chunk。

func moneyPtr(v float64) *float64 { return &v }

func fp(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

// ---- 车牌 ----

func TestFillPlateFromRemark(t *testing.T) {
	inv := &InvoiceExtractionItem{
		InvoiceNo: "1",
		Remark:    "车牌号：渝A75R1C；通行日期起：2026-10-22；通行日期止：2026-10-22",
	}
	completeInvoiceExtras(inv)
	if inv.Fields["车牌号"] != "渝A75R1C" {
		t.Fatalf("应从备注取到车牌，实际 %v", inv.Fields)
	}
	// 别名必须同写：前端按 plate_no || license_plate || vehicle_no 兜底
	if inv.Fields["plate_no"] != "渝A75R1C" {
		t.Fatalf("plate_no 别名必须同写，实际 %v", inv.Fields["plate_no"])
	}
}

func TestFillPlateFromItemNameWithoutLabel(t *testing.T) {
	// 图 2 左红框的真机形态：车牌在明细行里，没有「车牌号：」前缀
	inv := &InvoiceExtractionItem{
		InvoiceNo: "2",
		Items: []InvoiceExtractionLine{
			{Name: "*生产生活服务*通行费 渝D95256 货车 20261006"},
		},
	}
	completeInvoiceExtras(inv)
	if inv.Fields["车牌号"] != "渝D95256" {
		t.Fatalf("应从明细行裸车牌取到，实际 %v", inv.Fields)
	}
	if inv.Items[0].Extra["plate_no"] != "渝D95256" {
		t.Fatalf("明细行 extra 也必须回填，实际 %v", inv.Items[0].Extra)
	}
}

func TestFillPlateFromItemExtra(t *testing.T) {
	inv := &InvoiceExtractionItem{
		InvoiceNo: "3",
		Items: []InvoiceExtractionLine{
			{Name: "通行费", Extra: map[string]any{"plate_no": "渝C87567"}},
		},
	}
	completeInvoiceExtras(inv)
	if inv.Fields["车牌号"] != "渝C87567" {
		t.Fatalf("应从 items.extra 取到，实际 %v", inv.Fields)
	}
}

func TestFillPlateNoInventWhenAbsent(t *testing.T) {
	// 没有任何车牌依据 → 必须留空，绝不猜
	inv := &InvoiceExtractionItem{InvoiceNo: "4", Items: []InvoiceExtractionLine{{Name: "办公用品"}}}
	completeInvoiceExtras(inv)
	if inv.Fields["车牌号"] != nil {
		t.Fatalf("无依据时不得写车牌，实际 %v", inv.Fields)
	}
}

func TestNormalizePlateRejectsNoise(t *testing.T) {
	for _, bad := range []string{"", "20261022", "11.04", "通行费", "渝", "ABC123"} {
		if got := normalizePlate(bad); got != "" {
			t.Errorf("%q 不是车牌，却返回 %q", bad, got)
		}
	}
	if got := normalizePlate("车牌号渝A75R1C客车"); got != "渝A75R1C" {
		t.Errorf("应从混合串抠出车牌，实际 %q", got)
	}
}

// ---- 开票人 ----

func TestFillIssuerAcrossSpaceAndNewline(t *testing.T) {
	// 票面按字断行 + 冒号后换行：旧正则 `开票人\s*[:：]\s*(\S+)` 在这里失配
	for _, raw := range []string{
		"开票人：杨希",
		"开 票 人 ：\n杨希",
		"开 票 人 : 杨希",
		"开票人:\n  包凯莉",
	} {
		inv := &InvoiceExtractionItem{InvoiceNo: "5", Remark: raw}
		completeInvoiceExtras(inv)
		if inv.Issuer == "" {
			t.Errorf("%q 应取到开票人", raw)
		}
	}
}

func TestFillIssuerFiltersNoise(t *testing.T) {
	// 模板占位词不能当人名（否则列表出现「管理员」当开票人）
	for _, noise := range []string{"管理员", "系统", "电子发票", "备注", "开票"} {
		inv := &InvoiceExtractionItem{InvoiceNo: "6", Issuer: noise}
		completeInvoiceExtras(inv)
		if inv.Issuer != "" {
			t.Errorf("%q 是噪词，应被清空，实际 %q", noise, inv.Issuer)
		}
	}
}

func TestFillIssuerKeepsExisting(t *testing.T) {
	inv := &InvoiceExtractionItem{InvoiceNo: "7", Issuer: "王如媛", Remark: "开票人：别人"}
	completeInvoiceExtras(inv)
	if inv.Issuer != "王如媛" {
		t.Fatalf("已有值不得被覆盖，实际 %q", inv.Issuer)
	}
}

// ---- 金额四段交叉推算 ----

func TestReconcileAmountAndRateToTax(t *testing.T) {
	inv := &InvoiceExtractionItem{InvoiceNo: "8", Amount: moneyPtr(49.24), TaxRate: moneyPtr(0.03)}
	completeInvoiceExtras(inv)
	if fp(inv.Tax) != "1.48" {
		t.Fatalf("税额应为 1.48，实际 %s", fp(inv.Tax))
	}
	if fp(inv.TotalAmount) != "50.72" {
		t.Fatalf("价税合计应为 50.72，实际 %s", fp(inv.TotalAmount))
	}
}

func TestReconcileTotalMinusTax(t *testing.T) {
	// 图 2 里 P2/P14 的形态：只有价税合计与税额
	inv := &InvoiceExtractionItem{InvoiceNo: "9", TotalAmount: moneyPtr(784.38), Tax: moneyPtr(64.77)}
	completeInvoiceExtras(inv)
	if fp(inv.Amount) != "719.61" {
		t.Fatalf("金额应为 719.61，实际 %s", fp(inv.Amount))
	}
}

func TestReconcileTotalAndRate(t *testing.T) {
	inv := &InvoiceExtractionItem{InvoiceNo: "10", TotalAmount: moneyPtr(115.26), TaxRate: moneyPtr(0.09)}
	completeInvoiceExtras(inv)
	if fp(inv.Amount) != "105.74" || fp(inv.Tax) != "9.52" {
		t.Fatalf("应反算 105.74 / 9.52，实际 %s / %s", fp(inv.Amount), fp(inv.Tax))
	}
}

func TestReconcileNeverInventsFromSingleField(t *testing.T) {
	// 只有一项时必须留空 —— 单一字段的任何推算都是猜（把 0 填成 0 更糟）
	for _, inv := range []*InvoiceExtractionItem{
		{InvoiceNo: "11", Amount: moneyPtr(49.24)},
		{InvoiceNo: "12", TotalAmount: moneyPtr(50.72)},
		{InvoiceNo: "13", Tax: moneyPtr(1.48)},
	} {
		before := fp(inv.Tax) + fp(inv.Amount) + fp(inv.TotalAmount)
		completeInvoiceExtras(inv)
		after := fp(inv.Tax) + fp(inv.Amount) + fp(inv.TotalAmount)
		if before != after {
			t.Errorf("%s 只有一项时不得推算，%s → %s", inv.InvoiceNo, before, after)
		}
	}
}

func TestReconcileKeepsExistingValues(t *testing.T) {
	inv := &InvoiceExtractionItem{
		InvoiceNo: "14", Amount: moneyPtr(100), Tax: moneyPtr(3), TotalAmount: moneyPtr(103), TaxRate: moneyPtr(0.03),
	}
	completeInvoiceExtras(inv)
	if fp(inv.Amount) != "100.00" || fp(inv.Tax) != "3.00" || fp(inv.TotalAmount) != "103.00" {
		t.Fatalf("已有值不得被改写，实际 %s/%s/%s", fp(inv.Amount), fp(inv.Tax), fp(inv.TotalAmount))
	}
}

// ---- 全文兜底（chunks 双源融合）----

func TestApplyFromFullTextLocatesByInvoiceNo(t *testing.T) {
	// 段落内没有数据行（数据行被切到下一个 chunk），全文兜底应按票号定位
	inv := &InvoiceExtractionItem{InvoiceNo: "26507903331100005104"}
	full := `发票号码：26507903331100005104
开票日期：2026年11月02日
<table><tr><td colspan="5">*生产生活服务*通行费 渝D95256 货车 20261006</td><td>20261014</td><td>719.61</td><td>9%</td><td>64.77</td></tr></table>
开票人：唐菲
发票号码：26507904441100005581
开票人：郝敏`
	if n := applyPlateAndIssuerFromFullText(inv, full); n == 0 {
		t.Fatal("应从全文补到车牌/开票人")
	}
	if inv.Fields["车牌号"] != "渝D95256" {
		t.Errorf("车牌应为 渝D95256，实际 %v", inv.Fields)
	}
	if inv.Issuer != "唐菲" {
		t.Errorf("开票人应为 唐菲（不得取到下一张的郝敏），实际 %q", inv.Issuer)
	}
}

func TestApplyFromFullTextDoesNotCrossInvoice(t *testing.T) {
	// 票号在全文里找不到 → 什么都不做（绝不拿别的票的字段顶上）
	inv := &InvoiceExtractionItem{InvoiceNo: "99999999999999999999"}
	full := "发票号码：26507903331100005104\n开票人：唐菲"
	if n := applyPlateAndIssuerFromFullText(inv, full); n != 0 {
		t.Fatalf("票号不存在时不得补任何字段，实际补了 %d", n)
	}
	if inv.Issuer != "" {
		t.Fatalf("不得取到别的票的开票人，实际 %q", inv.Issuer)
	}
}

func TestNormalizeInvoiceExtrasIsIdempotent(t *testing.T) {
	res := &InvoiceExtractionResult{Kind: "invoice", Invoices: []InvoiceExtractionItem{
		{
			InvoiceNo: "15",
			Amount:    moneyPtr(49.24),
			TaxRate:   moneyPtr(0.03),
			Items:     []InvoiceExtractionLine{{Name: "*通行费 渝A75R1C 客车 20261022"}},
			Remark:    "开票人：杨希",
		},
	}}
	first := normalizeInvoiceExtras(res)
	if first == 0 {
		t.Fatal("首次应补齐若干字段")
	}
	// 第二次不得再改任何值（幂等：补齐只填空）
	inv := res.Invoices[0]
	before := fp(inv.Amount) + fp(inv.Tax) + fp(inv.TotalAmount) + inv.Issuer + fp(inv.TaxRate)
	if second := normalizeInvoiceExtras(res); second != 0 {
		t.Fatalf("第二次应为 0（幂等），实际 %d", second)
	}
	inv2 := res.Invoices[0]
	after := fp(inv2.Amount) + fp(inv2.Tax) + fp(inv2.TotalAmount) + inv2.Issuer + fp(inv2.TaxRate)
	if before != after {
		t.Fatalf("值被改动：%s → %s", before, after)
	}
}
