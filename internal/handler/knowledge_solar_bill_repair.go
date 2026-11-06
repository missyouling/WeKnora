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
		// 第 43 轮：概览四件套齐全 ≠ 字段齐全。旧快筛只写 4 个字段，存量记录
		// 因此缺发电户号/户名/纳税人类型/并网电压/消纳方式/发电方式/服务单位/
		// 发电地址、年累计、环比、关口明细与示数镜像。这里单独判一次「增强字段
		// 是否缺失」，只补空值、绝不覆盖已有值（用户手改的数据优先）。
		needsEnrich := terminal && complete && meta.ExtractStatus != "manual" &&
			solarRecordNeedsEnrich(&meta.Records[0])
		if complete && terminal && !needsEnrich {
			continue
		}

		changed := false
		if complete && needsEnrich {
			if fast, ok := solarFastPathFromChunks(ctx, h, row.ID, row.FileName); ok {
				if solarMergeMissing(&meta.Records[0], fast) {
					changed = true
					repaired++
				}
			}
		}
		if !complete {
			if fast, ok := solarFastPathFromChunks(ctx, h, row.ID, row.FileName); ok {
				meta.Kind = "solar_bill"
				meta.Records = []types.SolarBillExtractionItem{*fast}
				meta.ExtractStatus = "success"
				meta.ExtractError = ""
				changed = true
				repaired++
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

// solarFastPathFromChunks 从已解析 chunks 跑确定性快筛。
//
// 启动态没有 gin 上下文，chunkService / kgService 会对 TenantIDContextKey 做
// 强制 uint64 类型断言而 panic；这里直接用只读 SQL 取同一份 chunk 正文，
// 纯计算、不依赖租户上下文。
func solarFastPathFromChunks(
	ctx context.Context, h *BusinessExtractHandler, knowledgeID, fileName string,
) (*types.SolarBillExtractionItem, bool) {
	if h == nil || h.db == nil || knowledgeID == "" {
		return nil, false
	}
	var chunkRows []struct {
		Content    string `gorm:"column:content"`
		ChunkType  string `gorm:"column:chunk_type"`
		ChunkIndex int    `gorm:"column:chunk_index"`
	}
	err := h.db.WithContext(ctx).Table("chunks").
		Select("content, chunk_type, chunk_index").
		Where("knowledge_id = ?", knowledgeID).
		Order("chunk_index ASC").
		Find(&chunkRows).Error
	if err != nil || len(chunkRows) == 0 {
		return nil, false
	}
	chunks := make([]*types.Chunk, 0, len(chunkRows))
	for _, cr := range chunkRows {
		chunks = append(chunks, &types.Chunk{
			Content: cr.Content, ChunkType: cr.ChunkType, ChunkIndex: cr.ChunkIndex,
		})
	}
	return service.FastPathSolarBill(service.AggregateChunksForExtraction(chunks), fileName, nil)
}

// solarRecordNeedsEnrich 判断存量光伏记录是否缺「增强字段」。
//
// 只有 4 个概览字段（发电量/上网电量/结算金额/账单周期）的历史记录必须回填：
// 列表能显示、抽屉却一屏「请输入」正是这个原因。
func solarRecordNeedsEnrich(r *types.SolarBillExtractionItem) bool {
	if r == nil {
		return false
	}
	if r.AccountNo == "" || r.AccountName == "" || r.TaxpayerType == "" ||
		r.VoltageLevel == "" || r.ConsumptionMode == "" || r.GenerationMode == "" ||
		r.SupplyUnit == "" || r.Address == "" {
		return true
	}
	if r.CumulativeKwh == 0 || r.MomChange == "" {
		return true
	}
	if len(r.Gateways) == 0 || r.MeterPrev == 0 || r.MeterCurr == 0 || r.MeterRatio == 0 {
		return true
	}
	return false
}

// solarMergeMissing 把快筛结果里「源记录为空」的字段补进去，已有值一律保留。
//
// 只补空值而不是整体替换：存量记录可能已被用户手工修正过（金额、示数），
// 启动回填绝不能把人工结果冲掉。
func solarMergeMissing(dst, src *types.SolarBillExtractionItem) bool {
	if dst == nil || src == nil {
		return false
	}
	changed := false
	setStr := func(target *string, v string) {
		if *target == "" && v != "" {
			*target = v
			changed = true
		}
	}
	setF := func(target *float64, v float64) {
		if *target == 0 && v != 0 {
			*target = v
			changed = true
		}
	}
	setStr(&dst.AccountNo, src.AccountNo)
	setStr(&dst.AccountName, src.AccountName)
	setStr(&dst.SupplyUnit, src.SupplyUnit)
	setStr(&dst.Address, src.Address)
	setStr(&dst.TaxpayerType, src.TaxpayerType)
	setStr(&dst.ConsumptionMode, src.ConsumptionMode)
	setStr(&dst.VoltageLevel, src.VoltageLevel)
	setStr(&dst.GenerationMode, src.GenerationMode)
	setStr(&dst.MomChange, src.MomChange)
	setF(&dst.CumulativeKwh, src.CumulativeKwh)
	setF(&dst.MeterPrev, src.MeterPrev)
	setF(&dst.MeterCurr, src.MeterCurr)
	setF(&dst.MeterRatio, src.MeterRatio)
	setF(&dst.MeterReading, src.MeterReading)
	setF(&dst.MeterBill, src.MeterBill)
	if len(dst.Gateways) == 0 && len(src.Gateways) > 0 {
		dst.Gateways = src.Gateways
		changed = true
	}
	// 动态字段桶同样只补空键。
	for k, v := range src.Fields {
		if _, exists := dst.Fields[k]; exists {
			continue
		}
		if dst.Fields == nil {
			dst.Fields = make(map[string]any, len(src.Fields))
		}
		dst.Fields[k] = v
		changed = true
	}
	return changed
}
