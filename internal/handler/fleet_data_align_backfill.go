package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// 车队档案的数据对齐（第 39 轮）。
//
// 三个真机问题，根因都是「knowledge 文件」与「fleet_records 台账」两张表长期漂移：
//
//	① 车辆档案：抽屉「全部文件 13」，但「行驶证 / 保险单 / 车辆登记证书」卡片全是 0 份
//	   —— 那些文件早就 解析/提取=已完成，却从来没有对应的台账行；
//	② 维保记录：「已收录 1 份」，但「一般维修 0 + 合格证 0」
//	   —— 那 1 行的 sub_category 是历史值「维修工单」，不在分类里，任何卡片都装不下它；
//	③ 抽屉里长期挂着「提取中 / 待提取」的僵尸任务，卡片却显示「无异常文件」。
//
// 本文件做三件事，全部幂等：
//   - 补齐缺失的台账行（按文件元数据的 doc_type 落到对应分类）；
//   - 归一非法的 sub_category（doc_type 命中分类则用它，否则落该菜单第一个分类）；
//   - 把超过 stale 阈值仍未终态的文件刷成 failed（僵尸任务不能永远占着「正在执行」）。

// fleetDataAlignStats 回填结果（便于日志核对）。
type fleetDataAlignStats struct {
	RecordsCreated int
	SubCategoryFix int
	StaleFileFixed int
}

// BackfillFleetArchiveAlignment 车队档案三表对齐（幂等）。
func (h *FleetHandler) BackfillFleetArchiveAlignment(ctx context.Context) (fleetDataAlignStats, error) {
	var stats fleetDataAlignStats
	if h.db == nil {
		return stats, nil
	}

	// 1) 车队知识库（同名库可能多租户各一份）
	var kbs []types.KnowledgeBase
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND name = ?", "日常事务-车队").
		Find(&kbs).Error; err != nil {
		return stats, err
	}
	kbIDs := make([]string, 0, len(kbs))
	for i := range kbs {
		kbIDs = append(kbIDs, kbs[i].ID)
	}

	// 2) 分类（按 scope 排序，用于「非法 sub_category → 第一个分类」兜底）
	catsByScope, err := h.fleetCategoryNamesByScope(ctx)
	if err != nil {
		return stats, err
	}
	if len(kbIDs) == 0 && len(catsByScope) == 0 {
		return stats, nil
	}

	// 3) 归一存量记录的非 sub_category
	var records []types.FleetRecord
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&records).Error; err != nil {
		return stats, err
	}
	valid := make(map[string]bool, 32)
	for _, names := range catsByScope {
		for _, n := range names {
			valid[n] = true
		}
	}
	for i := range records {
		rec := &records[i]
		scope := fleetScopeOfRecordType(rec.RecordType)
		names := catsByScope[scope]
		if len(names) == 0 || valid[rec.SubCategory] {
			continue
		}
		want := normalizeFleetSubCategory(rec.SubCategory, rec.DocType, names)
		if want == "" || want == rec.SubCategory {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("id = ?", rec.ID).
			Update("sub_category", want).Error; err != nil {
			logger.Warnf(ctx, "normalize fleet sub_category %s failed: %v", rec.ID, err)
			continue
		}
		logger.Infof(ctx, "fleet sub_category normalized: %s %q → %q", rec.FileName, rec.SubCategory, want)
		stats.SubCategoryFix++
	}

	// 4) 已完成的文件补齐台账行
	if len(kbIDs) > 0 {
		var files []types.Knowledge
		if err := h.db.WithContext(ctx).
			Select("id", "tenant_id", "knowledge_base_id", "file_name", "parse_status", "custom_metadata", "created_at", "updated_at").
			Where("deleted_at IS NULL AND knowledge_base_id IN ?", kbIDs).
			Find(&files).Error; err != nil {
			return stats, err
		}
		existing := make(map[string]bool, len(records))
		for i := range records {
			if records[i].DocKnowledgeID != "" {
				existing[records[i].DocKnowledgeID] = true
			}
		}
		now := time.Now().UTC()
		for i := range files {
			f := &files[i]
			if existing[f.ID] {
				continue
			}
			var meta fleetDocCustomMetadata
			if len(f.CustomMetadata) > 0 {
				_ = json.Unmarshal(f.CustomMetadata, &meta)
			}
			scope := strings.TrimPrefix(strings.TrimSuffix(meta.Kind, "_document"), "fleet_")
			if !isFleetArchiveKind(meta.Kind) {
				continue
			}
			// 只补「已完成提取」的文件：失败/在途的留给用户重试，避免造出空行
			if strings.TrimSpace(meta.ExtractStatus) != "success" {
				continue
			}
			// 用户在历史记录里隐藏过的文件不再造台账：
			// 否则会出现「已收录 11 份、历史记录 10 份、抽屉里还看不到那份」的错位。
			if isFleetHistoryHidden(f.CustomMetadata) {
				continue
			}
			names := catsByScope[scope]
			if len(names) == 0 {
				continue
			}
			docType := strings.TrimSpace(meta.DocType)
			if docType == "" {
				docType = strings.TrimSpace(meta.FleetCertType)
			}
			sub := normalizeFleetSubCategory("", docType, names)
			if docType == "" {
				docType = sub
			}
			data := map[string]any{
				"源文件ID": f.ID,
				"文件名称":  f.FileName,
				"证照类型":  docType,
			}
			// 历史文件可能已把字段写在 metadata.fields 里，能带就带上
			var raw map[string]any
			if len(f.CustomMetadata) > 0 {
				_ = json.Unmarshal(f.CustomMetadata, &raw)
			}
			if fields, ok := raw["fields"].(map[string]any); ok {
				for k, v := range fields {
					if strings.TrimSpace(k) == "" {
						continue
					}
					if _, dup := data[k]; !dup {
						data[k] = v
					}
				}
			}
			rec := types.FleetRecord{
				ID:             uuid.NewString(),
				TenantID:       int64(f.TenantID),
				RecordType:     fleetArchiveRecordType(scope),
				Data:           data,
				DocType:        docType,
				SubCategory:    sub,
				FileName:       f.FileName,
				DocKnowledgeID: f.ID,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				// 并发/唯一索引冲突都跳过（下次启动再判）
				logger.Warnf(ctx, "create fleet record for %s failed: %v", f.FileName, err)
				continue
			}
			logger.Infof(ctx, "fleet record created from file: %s → %s/%s", f.FileName, rec.RecordType, sub)
			stats.RecordsCreated++
		}
	}

	// 5) 僵尸任务刷终态
	fixed, err := h.BackfillFleetStaleFileStates(ctx, 30*time.Minute)
	if err != nil {
		return stats, err
	}
	stats.StaleFileFixed = fixed
	return stats, nil
}

// fleetCategoryNamesByScope 各 scope 的启用分类名（按 sort_order）。
func (h *FleetHandler) fleetCategoryNamesByScope(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND enabled = ?", true).
		Order("scope ASC, sort_order ASC").
		Find(&cats).Error; err != nil {
		return out, err
	}
	for i := range cats {
		scope := strings.TrimSpace(cats[i].Scope)
		name := strings.TrimSpace(cats[i].Name)
		if scope == "" || name == "" {
			continue
		}
		// 同一 scope 下多租户会有同名分类，去重保序
		dup := false
		for _, n := range out[scope] {
			if n == name {
				dup = true
				break
			}
		}
		if !dup {
			out[scope] = append(out[scope], name)
		}
	}
	return out, nil
}

// normalizeFleetSubCategory 把历史/非法子分类归一到本菜单的真实分类名。
//
// 规则：docType 命中分类就用它；否则按历史别名映射；再否则落第一个分类
// （卡片与列表都按 sub_category 过滤，落空会让记录从所有卡片里消失）。
func normalizeFleetSubCategory(current, docType string, names []string) string {
	if len(names) == 0 {
		return strings.TrimSpace(current)
	}
	isValid := func(s string) bool {
		s = strings.TrimSpace(s)
		if s == "" {
			return false
		}
		for _, n := range names {
			if n == s {
				return true
			}
		}
		return false
	}
	if isValid(current) {
		return strings.TrimSpace(current)
	}
	if isValid(docType) {
		return strings.TrimSpace(docType)
	}
	// 历史别名：维修工单 → 一般维修（旧种子用的名字）
	legacy := map[string]string{
		"维修工单": "一般维修",
		"二级维护": "一般维修",
	}
	if v, ok := legacy[strings.TrimSpace(current)]; ok && isValid(v) {
		return v
	}
	if v, ok := legacy[strings.TrimSpace(docType)]; ok && isValid(v) {
		return v
	}
	return names[0]
}

// isFleetHistoryHidden 文件是否被用户在历史记录里隐藏（fleet_history_hidden）。
func isFleetHistoryHidden(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, _ := m["fleet_history_hidden"].(bool)
	return v
}

// isFleetArchiveKind 是否车队档案类文件（vehicle/driver/maintain）。
func isFleetArchiveKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "fleet_vehicle_document", "fleet_driver_document", "fleet_maintain_document":
		return true
	}
	return false
}

// fleetScopeOfRecordType 记录类型 → scope。
func fleetScopeOfRecordType(recordType string) string {
	switch recordType {
	case types.FleetRecordVehicleArchive:
		return types.FleetCategoryScopeVehicle
	case types.FleetRecordDriverArchive:
		return types.FleetCategoryScopeDriver
	case types.FleetRecordMaintainArchive:
		return types.FleetCategoryScopeMaintain
	}
	return ""
}

// BackfillFleetStaleFileStates 把长期卡在「解析中/提取中」的文件刷成终态（幂等）。
//
// 为什么必须做：在途文件会一直占着「正在执行 N」，绿色状态条永远不消失，
// 用户以为后台还在跑（真机：migration_fix_verify.txt 这类测试垃圾文件挂了几天）。
// 阈值取 updated_at，超过 stale 且没有任何进展才判定僵尸。
func (h *FleetHandler) BackfillFleetStaleFileStates(ctx context.Context, stale time.Duration) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-stale)
	var files []types.Knowledge
	if err := h.db.WithContext(ctx).
		Select("id", "file_name", "parse_status", "custom_metadata", "created_at", "updated_at").
		Where("deleted_at IS NULL AND parse_status IN ?", []string{"pending", "parsing", "processing", "finalizing"}).
		Where("updated_at < ?", cutoff).
		Find(&files).Error; err != nil {
		return 0, err
	}
	fixed := 0
	for i := range files {
		f := &files[i]
		var meta map[string]any
		if len(f.CustomMetadata) > 0 {
			_ = json.Unmarshal(f.CustomMetadata, &meta)
		}
		if meta == nil {
			meta = map[string]any{}
		}
		meta["parse_error"] = "解析超时（服务重启或任务中断），已标记失败，可重新解析"
		buf, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ?", f.ID).
			Updates(map[string]any{"parse_status": "failed", "custom_metadata": types.JSON(buf)}).Error; err != nil {
			logger.Warnf(ctx, "mark stale fleet file %s failed: %v", f.ID, err)
			continue
		}
		logger.Infof(ctx, "stale fleet file marked failed: %s", f.FileName)
		fixed++
	}

	// 提取阶段的僵尸：解析已完成，extract 停在 processing/pending 超过阈值
	var exts []types.Knowledge
	if err := h.db.WithContext(ctx).
		Select("id", "file_name", "parse_status", "custom_metadata", "created_at", "updated_at").
		Where("deleted_at IS NULL AND parse_status = ?", "completed").
		Where("updated_at < ?", cutoff).
		Find(&exts).Error; err != nil {
		return fixed, err
	}
	for i := range exts {
		f := &exts[i]
		var meta fleetDocCustomMetadata
		if len(f.CustomMetadata) > 0 {
			_ = json.Unmarshal(f.CustomMetadata, &meta)
		}
		if !isFleetArchiveKind(meta.Kind) {
			continue
		}
		st := strings.TrimSpace(meta.ExtractStatus)
		if st != "pending" && st != "processing" {
			continue
		}
		meta.ExtractStatus = "failed"
		meta.ExtractError = "提取超时（任务中断），已标记失败，可重新提取"
		buf, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ?", f.ID).
			Update("custom_metadata", types.JSON(buf)).Error; err != nil {
			logger.Warnf(ctx, "mark stale fleet extract %s failed: %v", f.ID, err)
			continue
		}
		logger.Infof(ctx, "stale fleet extract marked failed: %s", f.FileName)
		fixed++
	}
	if fixed > 0 {
		logger.Infof(ctx, "fleet stale file states fixed: %d", fixed)
	}
	return fixed, nil
}
