package handler

import (
	"context"
	"encoding/json"
	"math"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// RepairResidentBillingKwh 启动幂等回填居民（定比）计费电量（第 43 轮）。
//
// 真机事故：费用清单「电量（定比）」在 2026-07 显示 147.32（= 9821 × 0.015，
// 被乘了两次定比），2026-06/2026-04/2025-10 显示 0，2026-05/03/02/2025-12/11
// 显示乘过一次 0.015 的值。根因是 `residential_readings[].bill_kwh` 是派生缓存，
// 历史管线写坏了；而票面 `fee_items` 的 `(5)目录电费` qty 在 17 个月全部正确。
//
// 修复口径：把主行 bill_kwh 对齐到票面权威值（service.RepairResidentReadingBillKwh）。
//   - 幂等：已是正确值 → 不写库；多行明细（2025-07 电能表+定比）→ 不覆盖；
//   - 不猜：没有 `(5)目录电费` 的账单不动；
//   - 纯 DB：启动态没有 gin 上下文，不走 kgService（会对 TenantIDContextKey 强断言 panic）。
func (h *BusinessExtractHandler) RepairResidentBillingKwh(ctx context.Context) {
	if h == nil || h.db == nil {
		return
	}
	type repairRow struct {
		ID             string `gorm:"column:id"`
		FileName       string `gorm:"column:file_name"`
		TenantID       int64  `gorm:"column:tenant_id"`
		CustomMetadata []byte `gorm:"column:custom_metadata"`
	}
	var rows []repairRow
	if err := h.db.WithContext(ctx).
		Table("knowledges").
		Select("id, file_name, tenant_id, custom_metadata").
		Where("deleted_at IS NULL").
		Where("custom_metadata->>'kind' = ?", "utility_bill").
		Find(&rows).Error; err != nil {
		logger.Warnf(ctx, "Resident kwh repair: query failed: %v", err)
		return
	}

	repaired := 0
	for _, row := range rows {
		var meta utilityBillCustomMetadata
		if err := json.Unmarshal(row.CustomMetadata, &meta); err != nil {
			continue
		}
		if len(meta.Records) == 0 {
			continue
		}
		// manual（用户手录）严格跳过：人工填的数就是最终值。
		if meta.ExtractStatus == "manual" {
			continue
		}
		changed := false
		for i := range meta.Records {
			if service.RepairResidentReadingBillKwh(&meta.Records[i]) {
				changed = true
			}
		}
		if !changed {
			continue
		}
		b, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Table("knowledges").
			Where("id = ? AND tenant_id = ?", row.ID, row.TenantID).
			UpdateColumn("custom_metadata", types.JSON(b)).Error; err != nil {
			logger.Warnf(ctx, "Resident kwh repair: save failed for %s: %v", row.ID, err)
			continue
		}
		repaired++
		logger.Infof(ctx, "Resident kwh repair: %s fixed (bill_kwh realigned to catalog fee qty)", row.FileName)
	}
	if repaired > 0 {
		logger.Infof(ctx, "Resident kwh repair done: repaired=%d total=%d", repaired, len(rows))
	}
}

// residentKwhDelta 计算某月「修正后 − 修正前」的居民电量差（用于重算日志）。
// 保留为可测的纯函数，避免在回填里堆算数。
func residentKwhDelta(before, after float64) float64 {
	return math.Round((after-before)*100) / 100
}
