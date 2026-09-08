package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// awardPunishMetadata mirrors the persisted custom_metadata shape for a
// knowledge row in the award/punish KB (see handler awardPunishCustomMetadata).
type awardPunishMetadata struct {
	Kind             string                            `json:"kind"`
	Records          []types.AwardPunishExtractionItem `json:"records"`
	ExtractStatus    string                            `json:"extract_status"`
	ExtractError     string                            `json:"extract_error"`
	AutoDeletedCount int                               `json:"auto_deleted_count"`
}

// ListAwardPunishRecords returns the paginated award/punish list of one KB.
// Records are NOT deduplicated by 文号 — the knowledge upload layer already
// deduplicates files by hash, and one notice legitimately shares one 文号
// across several persons.
func (s *knowledgeService) ListAwardPunishRecords(ctx context.Context, kbID string, filter types.AwardPunishListFilter) (*types.AwardPunishListResult, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 10000 {
		filter.PageSize = 10000
	}

	records := make([]types.AwardPunishRecord, 0, 256)
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for award/punish records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		// 加载标签（与文档列表保持一致）
		{
			ids := make([]string, 0, len(knowledges))
			for _, k := range knowledges {
				if k != nil {
					ids = append(ids, k.ID)
				}
			}
			if tagMap, err := s.repo.GetKnowledgeTags(ctx, ids); err == nil {
				for _, k := range knowledges {
					if k != nil {
						if tags, ok := tagMap[k.ID]; ok {
							k.Tags = tags
						}
					}
				}
			}
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count → 视作待补录占位行。
			legacyManual := meta.Kind != "award_punish" && meta.AutoDeletedCount > 0
			if meta.Kind != "award_punish" && !legacyManual {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 人工入库/空提取占位行：恢复后 extract_status=manual 或提取后仍无有效字段
			// 的记录必须显示在列表（用户可点开编辑补录），避免刷新后消失。
			if awardPunishItemsAllBlank(meta.Records) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				records = append(records, types.AwardPunishRecord{
					RowKey:         k.ID + "-manual",
					KnowledgeID:    k.ID,
					KnowledgeTitle: k.Title,
					FileName:       k.FileName,
					FileType:       k.FileType,
					Tags:           tags,
					ExtractStatus:  "manual",
					ExtractError:   meta.ExtractError,
					KBID:           k.KnowledgeBaseID,
					CreatedAt:      k.CreatedAt,
				})
				continue
			}
			if len(meta.Records) == 0 {
				continue
			}
			for idx, r := range meta.Records {
				// 过滤"空提取"记录（无文号、无标题、无类型、无当事人、无部门）
				if awardPunishItemBlank(r) {
					continue
				}
				records = append(records, types.AwardPunishRecord{
					RowKey:         fmt.Sprintf("%s-%d", k.ID, idx),
					ApNo:           r.ApNo,
					ApTitle:        r.ApTitle,
					ApType:         r.ApType,
					Person:         r.Person,
					Dept:           r.Dept,
					Position:       r.Position,
					Measure:        r.Measure,
					Basis:          r.Basis,
					Signer:         r.Signer,
					SignDate:       r.SignDate,
					EffectiveDate:  r.EffectiveDate,
					Remark:         r.Remark,
					Summary:        r.Summary,
					Page:           r.Page,
					KnowledgeID:    k.ID,
					KnowledgeTitle: k.Title,
					FileName:       k.FileName,
					FileType:       k.FileType,
					Tags:           tags,
					ExtractStatus:  meta.ExtractStatus,
					ExtractError:   meta.ExtractError,
					KBID:           k.KnowledgeBaseID,
					CreatedAt:      k.CreatedAt,
				})
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	// 过滤（不做文号去重——文件层已按哈希去重）
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	for _, r := range records {
		if filter.ApType != "" && r.ApType != filter.ApType {
			continue
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !awardPunishDateInRange(r.SignDate, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !awardPunishRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
	}
	records = kept

	// 排序：源文件创建时间新在前
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	total := len(records)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	return &types.AwardPunishListResult{
		Data:     records[start:end],
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// awardPunishItemsAllBlank 判断奖惩提取列表是否完全没有有效数据（空数组或每项都空提取）。
func awardPunishItemsAllBlank(recs []types.AwardPunishExtractionItem) bool {
	if len(recs) == 0 {
		return true
	}
	for _, r := range recs {
		if !awardPunishItemBlank(r) {
			return false
		}
	}
	return true
}

// awardPunishItemBlank 判断单条奖惩记录是否空提取（无文号、无标题、无类型、无当事人、无部门）。
func awardPunishItemBlank(r types.AwardPunishExtractionItem) bool {
	return strings.TrimSpace(r.ApNo) == "" &&
		strings.TrimSpace(r.ApTitle) == "" &&
		strings.TrimSpace(r.ApType) == "" &&
		strings.TrimSpace(r.Person) == "" &&
		strings.TrimSpace(r.Dept) == ""
}

// awardPunishDateInRange 判断签发日期是否落在 [from, to] 区间（含边界，空则不过滤）。
func awardPunishDateInRange(date, from, to string) bool {
	if from == "" && to == "" {
		return true
	}
	d := strings.TrimSpace(date)
	if d == "" {
		return false
	}
	if from != "" && d < from {
		return false
	}
	if to != "" && d > to {
		return false
	}
	return true
}

// awardPunishRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
func awardPunishRecordMatchKeyword(r types.AwardPunishRecord, kw string) bool {
	fields := []string{
		r.ApNo, r.ApTitle, r.ApType, r.Person, r.Dept, r.Position,
		r.Measure, r.Basis, r.Signer, r.SignDate, r.EffectiveDate,
		r.Remark, r.Summary,
		r.FileName, r.KnowledgeTitle, r.ExtractStatus, r.ExtractError,
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), kw) {
			return true
		}
	}
	for _, t := range r.Tags {
		if strings.Contains(strings.ToLower(t), kw) {
			return true
		}
	}
	return false
}

// ListAwardPunishTypes 返回该知识库下所有奖惩出现过的去重奖惩类型（含数量），
// 用于前端奖惩类型筛选下拉自动加载。
func (s *knowledgeService) ListAwardPunishTypes(ctx context.Context, kbID string) ([]types.AwardPunishTypeCount, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	page := 1
	batch := 100
	seen := map[string]int{}
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta awardPunishMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "award_punish" {
				continue
			}
			for _, r := range meta.Records {
				if t := strings.TrimSpace(r.ApType); t != "" {
					seen[t]++
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	// 固定枚举顺序优先，再补自定义类型
	ordered := []types.AwardPunishTypeCount{}
	for _, t := range []string{"处罚", "奖励", "通报", "其它奖惩"} {
		if c, ok := seen[t]; ok {
			ordered = append(ordered, types.AwardPunishTypeCount{ApType: t, Count: c})
			delete(seen, t)
		}
	}
	for t, c := range seen {
		ordered = append(ordered, types.AwardPunishTypeCount{ApType: t, Count: c})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Count > ordered[j].Count
	})
	return ordered, nil
}
