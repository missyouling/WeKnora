package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 存量费用清单的「零售损益分摊电费」重算（第 39 轮，幂等）。
//
// 口径变更：从「工业总电量 × 账单单价」改为「账单原值 × 分摊比例」。
//   · 2026-09：20,880.13 × 126600/596168 = 4,434.03（旧 4,501.52）
//   · 2026-10：51,418.66 × 172400/623932 = 14,207.60
//     （旧实现把这项当「纯金额返还」整项跳过，前端只能显示 0）
//
// 为什么整条记录重生成而不是只改那一行：
//   工业电费是「分时项 + 平段项 + 基本电费 + 力调」的**未取整累加**，
//   只改一行再按明细四舍五入重算，会和原值差 1 分；而且旧版把该项算错时，
//   工业电费/均价/总电费都已经写错了。用 buildFeeItems 整条重生成，
//   结果与用户点「生成账单」逐字一致，也顺手修正历史错误。
//
// 幂等：算出来的明细与合计跟库里一致时直接跳过，不重复写库。
func (h *BillingHandler) BackfillRetailLossAllocation(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var recs []types.BillingRecord
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&recs).Error; err != nil {
		return 0, err
	}
	patched := 0
	for i := range recs {
		rec := &recs[i]
		// 便宜判据先挡一层：已经对齐的记录不必再跑 prepareBillingInputs
		// （跨洋链路一条要 8s，每个启动都重算 4 条会把启动拖慢半分钟）。
		if h.retailLossLooksAligned(ctx, rec) {
			continue
		}
		in, err := h.prepareBillingInputs(ctx, rec.BillingTenantID, rec.Month)
		if err != nil || in == nil || in.Ratio <= 0 {
			continue
		}
		if !billHasRetailLoss(in.Bill.FeeItems) {
			continue
		}
		now := timeNowUTC()
		recordID := rec.ID
		plan := allocateTimed(in, recordID, now)
		fees := buildFeeItems(in, plan, recordID, now)
		industrialFee := round2(fees.IndustrialFee)
		if billingAlreadyAligned(ctx, h, rec, industrialFee, fees.Items) {
			continue
		}
		totalFee := round2(industrialFee + in.DormFee + in.WaterFee)
		next := types.BillingRecord{
			ID: recordID, BillingTenantID: rec.BillingTenantID, Month: rec.Month, Status: "generated",
			TotalKwh: round2(in.StarTotal), LineLoss: in.LineLoss, Ratio: in.Ratio,
			BillTotalAmount: in.Bill.TotalAmount,
			DormKwh:         round2(in.DormKwh), DormFee: in.DormFee,
			WaterUsage: round2(in.WaterUsage), WaterFee: in.WaterFee,
			IndustrialFee: industrialFee, TotalFee: totalFee,
			ElecTotalFee:  round2(industrialFee + in.DormFee),
			WaterTotalFee: in.WaterFee,
			CreatedAt:     rec.CreatedAt, UpdatedAt: now,
		}
		if err := h.persistRecord(ctx, next, fees.Items, plan.MeterRows, in.WaterRows); err != nil {
			logger.Warnf(ctx, "regenerate billing record %s/%s failed: %v", rec.Month, rec.BillingTenantID, err)
			continue
		}
		logger.Infof(ctx, "retail loss allocation refreshed: %s %s → industrial=%.2f",
			rec.Month, rec.BillingTenantID, industrialFee)
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "retail loss allocation backfilled: %d/%d records", patched, len(recs))
	}
	return patched, nil
}

// retailLossLooksAligned 快判：损益行已按比例 + 工业电费与明细自洽。
//
// 工业电费 = Σ(fee + base + pf)，逐项取整会与「未取整累加」差 1 分，故给 0.05 容差。
func (h *BillingHandler) retailLossLooksAligned(ctx context.Context, rec *types.BillingRecord) bool {
	var row struct {
		Qty  float64
		Rate float64
	}
	if err := h.db.WithContext(ctx).Model(&types.BillingRecordItem{}).
		Select("qty, rate").
		Where("record_id = ? AND kind = ? AND name LIKE ?", rec.ID, "fee", "%零售损益分摊%").
		First(&row).Error; err != nil {
		return false
	}
	if row.Qty <= 0 || row.Rate <= 0 || rec.Ratio <= 0 {
		return false
	}
	if d := row.Rate - rec.Ratio; d > 1e-9 || d < -1e-9 {
		return false
	}
	var sum float64
	if err := h.db.WithContext(ctx).Model(&types.BillingRecordItem{}).
		Where("record_id = ? AND kind IN ?", rec.ID, []string{"fee", "base", "pf"}).
		Select("COALESCE(SUM(fee), 0)").Scan(&sum).Error; err != nil {
		return false
	}
	return floatClose(sum, rec.IndustrialFee)
}

// billHasRetailLoss 该月账单里是否有「零售损益分摊电费」原值。
func billHasRetailLoss(items []types.UtilityBillFeeItem) bool {
	for _, f := range items {
		if !strings.Contains(f.Name, "零售损益分摊") {
			continue
		}
		if f.Fee != 0 || f.BillFee != 0 {
			return true
		}
	}
	return false
}

// billingAlreadyAligned 库里这条记录是否已经与新口径一致（幂等判据）。
func billingAlreadyAligned(
	ctx context.Context, h *BillingHandler, rec *types.BillingRecord,
	industrial float64, items []types.BillingRecordItem,
) bool {
	if !floatClose(rec.IndustrialFee, industrial) {
		return false
	}
	var old []types.BillingRecordItem
	if err := h.db.WithContext(ctx).
		Where("record_id = ? AND kind = ?", rec.ID, "fee").
		Find(&old).Error; err != nil {
		return false
	}
	want := make(map[string]float64, len(items))
	for _, it := range items {
		if it.Name != "" {
			want[it.Name+"|"+it.Period] = it.Fee
		}
	}
	for _, it := range old {
		if it.Name == "零售损益分摊电费" {
			if v, ok := want[it.Name+"|"+it.Period]; !ok || !floatClose(it.Fee, v) {
				return false
			}
			delete(want, it.Name+"|"+it.Period)
		}
	}
	return true
}

func floatClose(a, b float64) bool {
	d := a - b
	return d < 0.02 && d > -0.02
}
