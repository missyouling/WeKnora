package service

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============ 发票号清洗：严禁中文文件名/非票号中文字符写入 invoice_no ============
//
// 事故：真机第 91 行（销方 中国铁路上海局集团，金额 23.69）的 invoice_no 被填成
// 文件名「通行费电子票据汇总单(发票).pdf」——因为分段切分时把文件名当成了票头。
// 凡是以中文为主的值都不可能是票号（全电票号 20 位数字 / 传统票号 8~12 位），
// 必须清洗并回退为空，让上层决定是否重抓或走 LLM。

func TestBusinessInvoiceNoRejectsChineseFileName(t *testing.T) {
	bad := []string{
		"通行费电子票据汇总单(发票).pdf",
		"通行费电子票据汇总单(发票)",
		"电子发票",
		"汇总单",
		"增值税电子普通发票",
	}
	for _, v := range bad {
		got, _ := cleanInvoiceNoValue(v, "")
		assert.Empty(t, got, "中文文件名/中文描述不得作为票号: %q", v)
	}
}

func TestBusinessInvoiceNoAcceptsRealTicketNumbers(t *testing.T) {
	good := map[string]string{
		"26517905800800020367": "26517905800800020367",
		"26507904620300002930": "26507904620300002930",
		"  04286059  ":         "04286059",
		"No.2651790580080002":  "2651790580080002",
	}
	for in, want := range good {
		got, _ := cleanInvoiceNoValue(in, "")
		assert.Equal(t, want, got, "真实票号必须保留: %q", in)
	}
}

// 与文件名完全相同 → 必须拒绝（最常见的错填形态）
func TestBusinessInvoiceNoEqualsFileNameRejected(t *testing.T) {
	got, _ := cleanInvoiceNoValue("通行费电子票据汇总单(发票).pdf", "通行费电子票据汇总单(发票).pdf")
	assert.Empty(t, got, "invoice_no 等于文件名必须清空")
}

// 文件名仅去扩展名后相等也要拒绝（PDF 文本层常丢扩展名）
func TestBusinessInvoiceNoEqualsFileNameStemRejected(t *testing.T) {
	got, _ := cleanInvoiceNoValue("通行费电子票据汇总单(发票)", "通行费电子票据汇总单(发票).pdf")
	assert.Empty(t, got, "invoice_no 等于文件名主干必须清空")
}

// ============ 数学关系自动反算补齐（确定性后处理） ============
//
// 真机第 91 行：金额 23.69、税额 0.71、价税合计 24.40，税率空。
// round(0.71/23.69, 2) = 0.03 → 3%，与票面一致，必须自动补齐。

func TestBusinessInvoiceMathBackfillsTaxRateFromTaxAmount(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount:      f64ptr(23.69),
		Tax:         f64ptr(0.71),
		TotalAmount: f64ptr(24.40),
	}
	backfillInvoiceMath(&inv, false)
	assert.NotNil(t, inv.TaxRate, "税率必须按 round(tax/amount,2) 反算补齐")
	assert.InDelta(t, 0.03, *inv.TaxRate, 1e-9, "23.69/0.71 → 3%")
}

func TestBusinessInvoiceMathBackfillsAmountFromTotalMinusTax(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount:      nil,
		Tax:         f64ptr(6.78),
		TotalAmount: f64ptr(106.78),
	}
	backfillInvoiceMath(&inv, false)
	assert.NotNil(t, inv.Amount, "金额必须按 total-tax 反算补齐")
	assert.InDelta(t, 100.0, *inv.Amount, 1e-9)
}

// 不征税/免税通行费：只有价税合计 → amount=total, tax=0, rate=0%
func TestBusinessInvoiceMathBackfillsTaxFreeToll(t *testing.T) {
	inv := InvoiceExtractionItem{TotalAmount: f64ptr(635.80)}
	backfillInvoiceMath(&inv, true)
	assert.NotNil(t, inv.Amount)
	assert.InDelta(t, 635.80, *inv.Amount, 1e-9, "免税通行费金额=价税合计")
	assert.NotNil(t, inv.Tax)
	assert.InDelta(t, 0.0, *inv.Tax, 1e-9, "税额必须补 0 而不是留空")
	assert.NotNil(t, inv.TaxRate)
	assert.InDelta(t, 0.0, *inv.TaxRate, 1e-9, "税率补 0%")
}

// 税率已在合理区间时不得被改写（防止把 9% 改成 0.09 之类）
func TestBusinessInvoiceMathKeepsExistingTaxRate(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount:      f64ptr(100),
		Tax:         f64ptr(9),
		TotalAmount: f64ptr(109),
		TaxRate:     f64ptr(0.09),
	}
	backfillInvoiceMath(&inv, false)
	assert.InDelta(t, 0.09, *inv.TaxRate, 1e-9, "已有合理税率不得被反算覆盖")
}

// 已有金额时不得覆盖
func TestBusinessInvoiceMathKeepsExistingAmount(t *testing.T) {
	inv := InvoiceExtractionItem{
		Amount:      f64ptr(88.5),
		Tax:         f64ptr(2.34),
		TotalAmount: f64ptr(90.84),
	}
	backfillInvoiceMath(&inv, false)
	assert.InDelta(t, 88.5, *inv.Amount, 1e-9, "已有金额不得被 total-tax 覆盖")
}

// ============ 逐票完整性校验：不能直接短路跳过 LLM ============
//
// 真机 P4 记录：价税合计 635.00，但车牌号/金额/税率/税额全空。旧
// InvoiceRuleResultIncomplete 只查「三者全空 + 购销方全空」，这类
// 「有合计但缺金额/税」的半残票被判为完整 → 短路跳过 LLM → 字段永久缺失。

func TestBusinessInvoiceRuleIncompleteCatchesHalfFilledToll(t *testing.T) {
	res := &InvoiceExtractionResult{
		Invoices: []InvoiceExtractionItem{
			{
				InvoiceNo:   "26517905800800020367",
				InvoiceDate: "2026-08-07",
				BuyerName:   "重庆星达铜业有限公司",
				SellerName:  "四川雅康高速公路有限责任公司",
				TotalAmount: f64ptr(635.80), // 只有合计
			},
		},
	}
	assert.True(t, InvoiceRuleResultIncomplete(res),
		"有价税合计但缺金额/税额的半残票必须判为不完整（触发 LLM 补提）")
}

func TestBusinessInvoiceRuleIncompleteAllowsCompleteInvoice(t *testing.T) {
	res := &InvoiceExtractionResult{
		Invoices: []InvoiceExtractionItem{
			{
				InvoiceNo:   "26517905800800020367",
				InvoiceDate: "2026-08-07",
				BuyerName:   "重庆星达铜业有限公司",
				SellerName:  "四川雅康高速公路有限责任公司",
				Amount:      f64ptr(78.05),
				Tax:         f64ptr(2.34),
				TotalAmount: f64ptr(80.39),
			},
		},
	}
	assert.False(t, InvoiceRuleResultIncomplete(res), "字段齐备的票必须判为完整，避免无谓烧 token")
}

// 缺失票号/开票日期同样必须判不完整（真机第 91 行就是缺日期）
func TestBusinessInvoiceRuleIncompleteCatchesMissingNoAndDate(t *testing.T) {
	base := InvoiceExtractionItem{
		Amount: f64ptr(23.69), Tax: f64ptr(0.71), TotalAmount: f64ptr(24.40),
		BuyerName: "x", SellerName: "y",
	}
	noNo := InvoiceExtractionItem{}
	noNo = base
	noNo.InvoiceNo = ""
	assert.True(t, InvoiceRuleResultIncomplete(&InvoiceExtractionResult{Invoices: []InvoiceExtractionItem{noNo}}),
		"缺发票号必须判不完整")

	noDate := base
	noDate.InvoiceNo = "12345678"
	assert.True(t, InvoiceRuleResultIncomplete(&InvoiceExtractionResult{Invoices: []InvoiceExtractionItem{noDate}}),
		"缺开票日期必须判不完整")
}

// ============ math 边界：不应产生荒谬值 ============

func TestBusinessInvoiceMathNoNegativeAmount(t *testing.T) {
	// total 小于 tax 的坏数据：反算会出现负数，必须放弃而不是写负金额
	inv := InvoiceExtractionItem{Tax: f64ptr(50), TotalAmount: f64ptr(10)}
	backfillInvoiceMath(&inv, false)
	if inv.Amount != nil {
		assert.GreaterOrEqual(t, *inv.Amount, 0.0, "不得反算出负金额")
	}
}

func TestBusinessInvoiceMathRateSanityCap(t *testing.T) {
	// tax/amount = 1.7 → 不是合理税率，必须放弃而不是写 170%
	inv := InvoiceExtractionItem{Amount: f64ptr(10), Tax: f64ptr(17), TotalAmount: f64ptr(27)}
	backfillInvoiceMath(&inv, false)
	assert.Nil(t, inv.TaxRate, "税率超过 100% 的荒谬比例必须放弃补齐")
}

func f64ptr(v float64) *float64 { return &v }

var _ = math.Abs
var _ = strings.TrimSpace

// 回归（2026-10-20 自查发现）：无免税证据时绝不能把缺失的税额补成 0。
// 事故：26507000000067486566 是 13% 应税发票，重提后 LLM 只给了价税合计，
// 旧逻辑按「免税」补齐，把税额 145.23 静默改写成 0 —— 属于严重数据损坏。
func TestBusinessInvoiceMathDoesNotZeroTaxWithoutExemptEvidence(t *testing.T) {
	inv := InvoiceExtractionItem{TotalAmount: f64ptr(1262.40)}
	backfillInvoiceMath(&inv, false)
	assert.Nil(t, inv.Amount, "无免税证据时不得把金额写成 total")
	assert.Nil(t, inv.Tax, "无免税证据时不得把税额补成 0")
}

// 有免税证据时必须补齐（不征税通行费场景）。
func TestBusinessInvoiceMathBackfillsWhenExemptEvidence(t *testing.T) {
	inv := InvoiceExtractionItem{TotalAmount: f64ptr(635.80)}
	backfillInvoiceMath(&inv, true)
	assert.NotNil(t, inv.Amount)
	assert.InDelta(t, 635.80, *inv.Amount, 1e-9)
	assert.NotNil(t, inv.Tax)
	assert.InDelta(t, 0.0, *inv.Tax, 1e-9)
}

// invoiceTaxExemptEvidence：票面出现「不征税」等字样才算有证据。
func TestBusinessInvoiceTaxExemptEvidence(t *testing.T) {
	assert.False(t, invoiceTaxExemptEvidence(&InvoiceExtractionItem{}, "价税合计 ¥1262.40 税率 13%"))
	assert.True(t, invoiceTaxExemptEvidence(&InvoiceExtractionItem{}, "税率 不征税"))
	assert.True(t, invoiceTaxExemptEvidence(&InvoiceExtractionItem{}, "免税"))
	zero := 0.0
	assert.True(t, invoiceTaxExemptEvidence(&InvoiceExtractionItem{TaxRate: &zero}, ""))
}

// 回归（真机 26507000000067486566）：明细行取的是折扣前金额，与价税合计矛盾。
// 必须以价税合计为锚、按税率反推：1262.40 / 1.13 = 1117.17，税额 145.23。
func TestBusinessInvoiceAmountsReconciledWithTotal(t *testing.T) {
	rate := 0.13
	inv := InvoiceExtractionItem{
		Amount: f64ptr(1269.91), Tax: f64ptr(165.09), TotalAmount: f64ptr(1262.40), TaxRate: &rate,
	}
	reconcileInvoiceAmounts(&inv)
	require.NotNil(t, inv.Amount)
	require.NotNil(t, inv.Tax)
	assert.InDelta(t, 1117.17, *inv.Amount, 0.01, "金额必须按 total/(1+rate) 反推")
	assert.InDelta(t, 145.23, *inv.Tax, 0.01, "税额必须为 total-amount")
	assert.InDelta(t, 1262.40, *inv.Amount+*inv.Tax, 0.02, "三者必须自洽")
}

// 已自洽的票不得被改动。
func TestBusinessInvoiceAmountsKeepConsistentTriple(t *testing.T) {
	rate := 0.13
	inv := InvoiceExtractionItem{
		Amount: f64ptr(100), Tax: f64ptr(13), TotalAmount: f64ptr(113), TaxRate: &rate,
	}
	reconcileInvoiceAmounts(&inv)
	assert.InDelta(t, 100.0, *inv.Amount, 1e-9)
	assert.InDelta(t, 13.0, *inv.Tax, 1e-9)
}

// 落库守门必须先从明细补税率，再用「税率反推」而非「total - 折扣前税额」。
// 真机 26507000000067486566：明细带 13%、税额是折扣前 165.09，
// 期望 1117.17 / 145.23，绝不能用 total-tax 得 1097.31。
func TestBusinessInvoicePersistGatePrefersRateOverPreDiscountTax(t *testing.T) {
	rate := 0.13
	invs := []InvoiceExtractionItem{{
		Amount: f64ptr(1269.91), Tax: f64ptr(165.09), TotalAmount: f64ptr(1262.40),
		Items: []InvoiceExtractionLine{{TaxRate: &rate, Amount: f64ptr(1269.91)}},
	}}
	ReconcileInvoicePartiesForPersist(invs)
	require.NotNil(t, invs[0].Amount)
	assert.InDelta(t, 1117.17, *invs[0].Amount, 0.01)
	assert.InDelta(t, 145.23, *invs[0].Tax, 0.01)
	assert.InDelta(t, 1262.40, *invs[0].Amount+*invs[0].Tax, 0.02)
}
