package service

import (
	"math"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 市电账单「本期电费」总额的口径（第 38 轮）。
//
// 真机事故（2026-10）：账单概况里 11 行分项逐个对得上（电费 = 账单电费，全绿），
// 底部「本期电费」的账单电费却是 446,003.1，而分项合计是 481,647.91 —— 底部
// 凭一个脏的汇总字段报「异常」。查库确认：`grand_total` / `total_amount`
// 在提取阶段就被模型写错并落库，明细（fee_amount / capacity_fee /
// pf_adjust_amount）反而是逐行提的、准确。
//
// 口径（与前端 utilityBillOverview.overviewBillTotalOf 逐字一致）：
//
//	本期电费 = Σ 各费用明细账单金额（fee_amount）
//	         + 输配容（需）量电费 capacity_fee
//	         + 功率因数调整电费 pf_adjust_amount
//
// 注意：`fee_items` 里还有一行 `功率因数调整电费`，与顶层 `pf_adjust_amount`
// 同源同值（2026-08 两者都是 -2,535.09）。这里必须跳过它，否则力调会被算两遍。
const utilityPfCategory = "功率因数调整"

// UtilityBillTotalOf 按分项重算账单应付总电费（元，保留 2 位）。
func UtilityBillTotalOf(item *types.UtilityBillExtractionItem) float64 {
	if item == nil {
		return 0
	}
	var feeSum, billSum float64
	for _, f := range item.FeeItems {
		if strings.Contains(f.Category, utilityPfCategory) || strings.Contains(f.Name, utilityPfCategory) {
			continue
		}
		feeSum += f.Fee
		billSum += f.BillFee
	}
	base := billSum
	// 老数据可能只有 fee 没有 fee_amount：两列同源时以账单列为准，
	// 账单列全空则退回计算列（否则总额会掉到只剩容需量）。
	if math.Abs(base) < 0.005 {
		base = feeSum
	}
	total := base + item.CapacityFee + item.PfAdjustAmount
	return math.Round(total*100) / 100
}

// ReconcileUtilityBillTotals 把 total_amount / grand_total 归一到分项之和。
//
// 两个字段都要写：列表「本期电费」列读 total_amount、「合计」列读 grand_total，
// 打印清单的概览卡片也读 total_amount —— 只改一个会立刻出现「列表 446,003.1、
// 概况 481,647.91」的新口径分裂。幂等：差值在 ±0.02 内不动。
func ReconcileUtilityBillTotals(item *types.UtilityBillExtractionItem) bool {
	total := UtilityBillTotalOf(item)
	if total == 0 {
		return false
	}
	changed := false
	if math.Abs(item.GrandTotal-total) > 0.02 {
		item.GrandTotal = total
		changed = true
	}
	if math.Abs(item.TotalAmount-total) > 0.02 {
		item.TotalAmount = total
		changed = true
	}
	return changed
}
