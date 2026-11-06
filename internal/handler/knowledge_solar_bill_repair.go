package handler

import (
	"context"
	"encoding/json"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// RepairSolarBillOverviews 启动幂等回填光伏账单概览字段（第 42 轮）。
//
// 真机事故：2026.10光伏账单.pdf 上传后 extract_status 永久停在 processing、
// records 为 null，而 chunks 里完整存着「发电量 66026 / 上网电量 2970 /
// 结算金额 1125.14」。旧管线在模型调用前异常返回，既没落终态也没写记录；
// 前端每次刷新又把它当"待提取"重新 POST，形成无限重提死循环。
//
// 本修复不调模型：用阶段 1 归一化 + 阶段 2 确定性快筛从已解析 chunks 直接
// 补回字段，并把状态一次性收敛到 success；快筛未命中的僵尸 processing 也
// 统一落 failed（终态），从根上消除重复投递。
//
// 幂等：字段齐全且状态为终态的文件严格跳过，绝不覆盖用户手工修改的数据。
func (h *BusinessExtractHandler) RepairSolarBillOverviews(ctx context.Context) {
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
		Where("custom_metadata->>'kind' = ?", "solar_bill").
		Find(&rows).Error; err != nil {
		logger.Warnf(ctx, "Solar bill overview repair: query failed: %v", err)
		return
	}

	repaired, settled := 0, 0
	for _, row := range rows {
		var meta solarBillCustomMetadata
		if err := json.Unmarshal(row.CustomMetadata, &meta); err != nil {
			continue
		}
		complete := len(meta.Records) > 0 &&
			meta.Records[0].GenerationKwh != 0 &&
			meta.Records[0].GridKwh != 0 &&
			meta.Records[0].SettlementAmount != 0
		terminal := meta.ExtractStatus == "success" || meta.ExtractStatus == "failed" ||
			meta.ExtractStatus == "manual" || meta.ExtractStatus == "not_solar_bill"
		if complete && terminal {
			continue
		}

		changed := false
		if !complete {
			// 启动态没有 gin 上下文，chunkService / kgService 会对
			// TenantIDContextKey 做强制类型断言而 panic；这里直接用只读 SQL
			// 取同一份 chunk 正文，纯计算、不依赖租户上下文。
			var chunkRows []struct {
				Content    string `gorm:"column:content"`
				ChunkType  string `gorm:"column:chunk_type"`
				ChunkIndex int    `gorm:"column:chunk_index"`
			}
			err := h.db.WithContext(ctx).Table("chunks").
				Select("content, chunk_type, chunk_index").
				Where("knowledge_id = ?", row.ID).
				Order("chunk_index ASC").
				Find(&chunkRows).Error
			if err == nil {
				chunks := make([]*types.Chunk, 0, len(chunkRows))
				for _, cr := range chunkRows {
					chunks = append(chunks, &types.Chunk{
						Content: cr.Content, ChunkType: cr.ChunkType, ChunkIndex: cr.ChunkIndex,
					})
				}
				canonical := service.AggregateChunksForExtraction(chunks)
				if fast, ok := service.FastPathSolarBill(canonical, row.FileName, nil); ok {
					meta.Kind = "solar_bill"
					meta.Records = []types.SolarBillExtractionItem{*fast}
					meta.ExtractStatus = "success"
					meta.ExtractError = ""
					changed = true
					repaired++
				}
			}
		}
		// 没补上字段但状态卡在过程态 → 落终态，禁止前端无限重投。
		if !changed && !terminal {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未完成，已由启动修复收敛为终态"
			}
			changed = true
			settled++
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
			logger.Warnf(ctx, "Solar bill overview repair: save failed for %s: %v", row.ID, err)
		}
	}
	if repaired > 0 || settled > 0 {
		logger.Infof(ctx, "Solar bill overview repair done: repaired=%d settled=%d total=%d",
			repaired, settled, len(rows))
	}
}
