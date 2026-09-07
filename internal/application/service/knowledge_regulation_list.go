// 制度级聚合列表：把知识库下所有已提取制度平铺为"一行一份制度"的记录，
// 在服务端完成全字段搜索、类型筛选、排序与分页，供制度管理页直接渲染。
// 重复判断复用知识库文件上传层（file_hash / 文件名+大小），列表层不再按制度编号去重。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// regulationMetadata 与 handler.regulationCustomMetadata 对齐，用于从 custom_metadata
// 反序列化制度提取结果。
type regulationMetadata struct {
	Kind             string                            `json:"kind"`
	Regulations      []types.RegulationExtractionItem  `json:"regulations"`
	ExtractStatus    string                            `json:"extract_status"`
	ExtractError     string                            `json:"extract_error"`
	AutoDeletedCount int                               `json:"auto_deleted_count"`
}

// ListRegulationRecords 实现制度级聚合列表。
func (s *knowledgeService) ListRegulationRecords(ctx context.Context, kbID string, filter types.RegulationListFilter) (*types.RegulationListResult, error) {
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

	records := make([]types.RegulationRecord, 0, 256)
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for regulation records: %w", err)
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
			var meta regulationMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count → 视作待补录占位行。
			legacyManual := meta.Kind != "regulation" && meta.AutoDeletedCount > 0
			if meta.Kind != "regulation" && !legacyManual {
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
			if regulationItemsAllBlank(meta.Regulations) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				records = append(records, types.RegulationRecord{
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
			if len(meta.Regulations) == 0 {
				continue
			}
			for _, r := range meta.Regulations {
				// 过滤"空提取"记录（无编号、无名称、无类型、无部门）
				if regulationExtractionItemBlank(r) {
					continue
				}
				records = append(records, types.RegulationRecord{
					RegNo:          r.RegNo,
					RegName:        r.RegName,
					RegType:        r.RegType,
					Dept:           r.Dept,
					Version:        r.Version,
					IssueDate:      r.IssueDate,
					PageCount:      r.PageCount,
					ModifyCount:    r.ModifyCount,
					Scope:          r.Scope,
					EffectiveDate:  r.EffectiveDate,
					Confidentiality: r.Confidentiality,
					Remark:         r.Remark,
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

	// 过滤（不做制度编号去重——文件层已按哈希去重）
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	for _, r := range records {
		if filter.RegType != "" && r.RegType != filter.RegType {
			continue
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !regulationDateInRange(r.IssueDate, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !regulationRecordMatchKeyword(r, kw) {
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
	return &types.RegulationListResult{
		Data:     records[start:end],
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

// regulationItemsAllBlank 判断制度提取列表是否完全没有有效数据（空数组或每项都空提取）。
func regulationItemsAllBlank(regs []types.RegulationExtractionItem) bool {
	if len(regs) == 0 {
		return true
	}
	for _, r := range regs {
		if !regulationExtractionItemBlank(r) {
			return false
		}
	}
	return true
}

func regulationExtractionItemBlank(r types.RegulationExtractionItem) bool {
	return strings.TrimSpace(r.RegNo) == "" && strings.TrimSpace(r.RegName) == "" &&
		strings.TrimSpace(r.RegType) == "" && strings.TrimSpace(r.Dept) == "" &&
		strings.TrimSpace(r.Version) == ""
}

// regulationDateInRange 判断日期（YYYY-MM-DD）是否落在 [from, to]（含，空端不限）。
func regulationDateInRange(date, from, to string) bool {
	date = strings.TrimSpace(date)
	if from == "" && to == "" {
		return true
	}
	if date == "" {
		return false
	}
	normalize := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	}
	d := normalize(date)
	if from != "" && d < normalize(from) {
		return false
	}
	if to != "" && d > normalize(to) {
		return false
	}
	return true
}

// regulationRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
func regulationRecordMatchKeyword(r types.RegulationRecord, kw string) bool {
	fields := []string{
		r.RegNo, r.RegName, r.RegType, r.Dept, r.Version, r.IssueDate,
		r.PageCount, r.ModifyCount, r.Scope, r.EffectiveDate, r.Confidentiality, r.Remark,
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

// ListRegulationTypes 返回该知识库下所有制度出现过的去重制度类型（含数量），
// 用于前端制度类型筛选下拉自动加载。
func (s *knowledgeService) ListRegulationTypes(ctx context.Context, kbID string) ([]types.RegulationTypeCount, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	page := 1
	batch := 100
	seen := map[string]int{}
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for regulation types: %w", err)
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			var meta regulationMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "regulation" {
				continue
			}
			for _, r := range meta.Regulations {
				if r.RegType != "" {
					seen[r.RegType]++
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	types_ := make([]types.RegulationTypeCount, 0, len(seen))
	for t, cnt := range seen {
		types_ = append(types_, types.RegulationTypeCount{RegType: t, Count: cnt})
	}
	sort.Slice(types_, func(i, j int) bool {
		if types_[i].Count != types_[j].Count {
			return types_[i].Count > types_[j].Count
		}
		return types_[i].RegType < types_[j].RegType
	})
	return types_, nil
}
