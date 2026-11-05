package service

import (
	"math"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 2026-10 真实账单（库内数据，分项逐个对得上，只有汇总字段是脏的）。
//
//	工商业 310,189.88 + 8,991.28 + 84,796.04 + 2,796.33 + 29,311.68 = 436,085.21
//	居民   4,679.86 + 277.76 = 4,957.62
//	容需量 44,000 ；力调 -3,394.92
//	→ 481,647.91（而库里 grand_total 写的是 446,003.1）
func utilityBill202610() types.UtilityBillExtractionItem {
	return types.UtilityBillExtractionItem{
		BillPeriod:     "2026-10",
		TotalAmount:    446003.10,
		GrandTotal:     446003.10,
		CapacityFee:    44000,
		PfAdjustAmount: -3394.92,
		FeeItems: []types.UtilityBillFeeItem{
			{Category: "(1)市场化购电费", Name: "零售交易电费", Fee: 310189.88, BillFee: 310189.88},
			{Category: "(2)上网环节线损费用", Name: "上网环节线损费用", Fee: 8991.28, BillFee: 8991.28},
			{Category: "(3)输配电量电费", Name: "零售输配电费", Fee: 84796.04, BillFee: 84796.04},
			{Category: "(4)系统运行费", Name: "煤电容量电费", Fee: 2796.33, BillFee: 2796.33},
			{Category: "(5)目录电费", Name: "基础电费", Fee: 4679.86, BillFee: 4679.86},
			{Category: "(6)政府性基金及附加-居民", Name: "农网还贷", Fee: 277.76, BillFee: 277.76},
			{Category: "(6)政府性基金及附加-工商业", Name: "农网还贷", Fee: 29311.68, BillFee: 29311.68},
		},
	}
}

func TestUtilityBillTotalOf202610(t *testing.T) {
	item := utilityBill202610()
	got := UtilityBillTotalOf(&item)
	if math.Abs(got-481647.91) > 0.01 {
		t.Fatalf("2026-10 本期电费应为 481647.91，got %.2f", got)
	}
	if !ReconcileUtilityBillTotals(&item) {
		t.Fatal("脏的 grand_total 必须被判定为需要修改")
	}
	if math.Abs(item.GrandTotal-481647.91) > 0.01 || math.Abs(item.TotalAmount-481647.91) > 0.01 {
		t.Fatalf("两个总额字段都要归一：grand=%.2f total=%.2f", item.GrandTotal, item.TotalAmount)
	}
	// 幂等：第二次不再判脏
	if ReconcileUtilityBillTotals(&item) {
		t.Fatal("已归一的账单不应再被判定为需要修改")
	}
}

// 2026-08 覆盖「fee_items 里另有一行 功率因数调整电费」的场景：
// 该行与顶层 pf_adjust_amount 同值，算总额时必须跳过，否则力调算两遍。
func TestUtilityBillTotalOfSkipsDuplicatePfRow(t *testing.T) {
	item := types.UtilityBillExtractionItem{
		BillPeriod:     "2026-08",
		GrandTotal:     393459.72,
		TotalAmount:    393459.72,
		CapacityFee:    44000,
		PfAdjustAmount: -2535.09,
		FeeItems: []types.UtilityBillFeeItem{
			{Category: "(1)市场化购电费", Name: "零售交易电费", Fee: 213339.58, BillFee: 213339.58},
			{Category: "(2)上网环节线损费用", Name: "上网环节线损费用", Fee: 9453.84, BillFee: 9453.84},
			{Category: "(3)输配电量电费", Name: "零售输配电费", Fee: 67051.00, BillFee: 67051.00},
			{Category: "(4)系统运行费", Name: "燃气机组容量电费", Fee: 31633.19, BillFee: 31633.19},
			{Category: "(5)目录电费", Name: "基础电费", Fee: 4167.55, BillFee: 4167.55},
			{Category: "(6)政府性基金及附加-居民", Name: "农网还贷", Fee: 247.35, BillFee: 247.35},
			{Category: "(6)政府性基金及附加-工商业", Name: "农网还贷", Fee: 26102.30, BillFee: 26102.30},
			// 与 pf_adjust_amount 重复的一行（真实数据 fee = 0）
			{Category: "功率因数调整电费", Name: "功率因数调整电费", Fee: 0, BillFee: 0},
		},
	}
	got := UtilityBillTotalOf(&item)
	if math.Abs(got-393459.72) > 0.01 {
		t.Fatalf("2026-08 本期电费应为 393459.72，got %.2f", got)
	}
	if ReconcileUtilityBillTotals(&item) {
		t.Fatal("本来正确的账单不应被改写（幂等）")
	}
}

// 只有 fee、没有 fee_amount 的老数据：退回计算列求和，不能只剩容需量。
func TestUtilityBillTotalOfFallsBackToFee(t *testing.T) {
	item := types.UtilityBillExtractionItem{
		CapacityFee: 44000, PfAdjustAmount: -100,
		FeeItems: []types.UtilityBillFeeItem{
			{Category: "(1)市场化购电费", Name: "零售交易电费", Fee: 1000},
			{Category: "(5)目录电费", Name: "基础电费", Fee: 200},
		},
	}
	got := UtilityBillTotalOf(&item)
	if math.Abs(got-(1000+200+44000-100)) > 0.01 {
		t.Fatalf("应退回 fee 列求和 = 45100，got %.2f", got)
	}
}
