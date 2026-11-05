package handler

import (
	"context"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BackfillTimedMeterReadings 幂等回填分时电表的起度/止度。
//
// 背景（真机事故）：持睿工业分表 1~3 是分时表（meter_type=time），录入时只填
// 四段示数，`start_reading` / `end_reading` 恒为 0。费用清单「电费核算明细」
// 直接读这两列，详情抽屉与 PDF 打印预览里工业分表的起度/止度全部显示 0。
//
// 读取路径已经用 service.NormalizeTimedMeterReadings 兜住展示（历史记录不改库
// 也能正确显示）；本回填把值**真正落库**，保证导出/第三方消费也拿到正确读数。
//
// 幂等：已有非 0 起止度的行不动，第二次启动零改动。
func (h *UtilityHandler) BackfillTimedMeterReadings(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var items []types.UtilityMeterItem
	if err := h.db.WithContext(ctx).
		Where("start_reading = 0 AND end_reading = 0").
		Find(&items).Error; err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}
	// 只处理「确实有四段读数」的行：四段全 0 的占位行会被 Normalize 跳过。
	changedIDs := make([]string, 0, len(items))
	byID := make(map[string]types.UtilityMeterItem, len(items))
	if !service.NormalizeTimedMeterReadings(items, nil) {
		return 0, nil
	}
	for _, it := range items {
		if it.StartReading != 0 || it.EndReading != 0 {
			changedIDs = append(changedIDs, it.ID)
			byID[it.ID] = it
		}
	}
	for _, id := range changedIDs {
		it := byID[id]
		if err := h.db.WithContext(ctx).Model(&types.UtilityMeterItem{}).
			Where("id = ?", id).
			Updates(map[string]any{
				"start_reading": it.StartReading,
				"end_reading":   it.EndReading,
			}).Error; err != nil {
			logger.Warnf(ctx, "backfill timed meter reading %s failed: %v", id, err)
		}
	}
	if len(changedIDs) > 0 {
		logger.Infof(ctx, "timed meter readings backfilled: %d rows", len(changedIDs))
	}
	return len(changedIDs), nil
}
