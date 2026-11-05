package handler

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BackfillUtilityBillTotals 把历史市电账单的「本期电费」总额归一到分项之和（第 38 轮）。
//
// 背景：`grand_total` / `total_amount` 是提取阶段由模型给的汇总值，历史上多次写错
// （真机 2026-10 落库 446,003.1，分项实算 481,647.91 —— 账单概况底部因此误报「异常」，
// 而上面 11 行分项全绿）。明细金额（fee_amount / capacity_fee / pf_adjust_amount）
// 是逐行提取的，可靠得多，所以总额一律**由分项重算**。
//
// 口径见 service.UtilityBillTotalOf：
//
//	本期电费 = Σ fee_amount（跳过与 pf_adjust_amount 重复的「功率因数调整电费」行）
//	         + capacity_fee + pf_adjust_amount
//
// 幂等：差值在 ±0.02 内不动；重复执行不会反复写库。
func (h *UtilityHandler) BackfillUtilityBillTotals(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var rows []types.Knowledge
	if err := h.db.WithContext(ctx).
		Select("id", "file_name", "custom_metadata").
		Where("deleted_at IS NULL AND custom_metadata->>'kind' = ?", "utility_bill").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	patched := 0
	for i := range rows {
		if len(rows[i].CustomMetadata) == 0 {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(rows[i].CustomMetadata, &meta); err != nil {
			continue
		}
		// 兼容历史双层结构：{"custom_metadata": {...}}
		if inner, ok := meta["custom_metadata"].(map[string]any); ok && len(meta) == 1 {
			meta = inner
		}
		recs, ok := meta["records"].([]any)
		if !ok || len(recs) == 0 {
			continue
		}
		changed := false
		for j := range recs {
			rec, ok := recs[j].(map[string]any)
			if !ok {
				continue
			}
			buf, err := json.Marshal(rec)
			if err != nil {
				continue
			}
			var item types.UtilityBillExtractionItem
			if err := json.Unmarshal(buf, &item); err != nil {
				continue
			}
			if !service.ReconcileUtilityBillTotals(&item) {
				continue
			}
			next, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var nextMap map[string]any
			if err := json.Unmarshal(next, &nextMap); err != nil {
				continue
			}
			recs[j] = nextMap
			changed = true
			logger.Infof(ctx, "reconcile utility bill total: %s %s → %.2f",
				rows[i].FileName, item.BillPeriod, item.GrandTotal)
		}
		if !changed {
			continue
		}
		meta["records"] = recs
		out, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ?", rows[i].ID).
			Update("custom_metadata", types.JSON(out)).Error; err != nil {
			logger.Warnf(ctx, "reconcile utility bill totals for %s failed: %v", rows[i].ID, err)
			continue
		}
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "utility bill totals reconciled: %d/%d bills", patched, len(rows))
	}
	return patched, nil
}
