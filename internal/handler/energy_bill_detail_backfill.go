package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BackfillUtilityBillDetails 幂等回填历史市电账单的三张明细表与派生电量。
//
// 背景（真机事故）：2026-10 账单的「电量明细」线损/加减全为 0、容量与力调明细
// 整段丢失，于是详情页两张子表「暂无数据」、账单概况两项电费算成 0、
// 用能分析把尖峰抄见电量复制到峰段。根因是 LLM 对固定表结构偶发漏抽。
//
// 处理方式：用**票面原文**（chunks）跑确定性解析（service.NormalizeUtilityBillDetails），
// 只补空/纠正与公式不符的值，已有正确值不动。幂等：无变化不写库。
func (h *UtilityHandler) BackfillUtilityBillDetails(ctx context.Context) (int, error) {
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
		first, ok := recs[0].(map[string]any)
		if !ok {
			continue
		}
		buf, err := json.Marshal(first)
		if err != nil {
			continue
		}
		var item types.UtilityBillExtractionItem
		if err := json.Unmarshal(buf, &item); err != nil {
			continue
		}
		text := h.utilityBillSourceText(ctx, rows[i].ID)
		if !service.NormalizeUtilityBillDetails(&item, text) {
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
		recs[0] = nextMap
		meta["records"] = recs
		out, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		logger.Infof(ctx, "backfill utility bill detail changed: %s", rows[i].FileName)
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ?", rows[i].ID).
			Update("custom_metadata", types.JSON(out)).Error; err != nil {
			logger.Warnf(ctx, "backfill utility bill details for %s failed: %v", rows[i].ID, err)
			continue
		}
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "utility bill details backfilled: %d/%d bills", patched, len(rows))
	}
	return patched, nil
}

// utilityBillSourceText 取账单原文（用于确定性补全）。
//
// 与提取链路同口径：只取文本/图片 OCR/图片描述三类 chunk，按 chunk_index 顺序拼接。
func (h *UtilityHandler) utilityBillSourceText(ctx context.Context, knowledgeID string) string {
	var chunks []types.Chunk
	if err := h.db.WithContext(ctx).
		Select("chunk_index", "chunk_type", "content").
		Where("knowledge_id = ? AND deleted_at IS NULL", knowledgeID).
		Order("chunk_index ASC").
		Find(&chunks).Error; err != nil {
		return ""
	}
	parts := make([]string, 0, len(chunks))
	for i := range chunks {
		switch chunks[i].ChunkType {
		case types.ChunkTypeText, types.ChunkTypeImageOCR, types.ChunkTypeImageCaption:
			if s := strings.TrimSpace(chunks[i].Content); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
