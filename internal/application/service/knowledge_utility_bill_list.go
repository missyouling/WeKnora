package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// utilityBillMetadata mirrors the persisted custom_metadata shape for a
// knowledge row in the utility-bill KB.
type utilityBillMetadata struct {
	Kind             string                           `json:"kind"`
	Records          []types.UtilityBillExtractionItem `json:"records"`
	ExtractStatus    string                           `json:"extract_status"`
	ExtractError     string                           `json:"extract_error"`
	AutoDeletedCount int                              `json:"auto_deleted_count"`
}

// ListUtilityBillRecords returns the paginated utility-bill list of one KB.
func (s *knowledgeService) ListUtilityBillRecords(ctx context.Context, kbID string, filter types.UtilityBillListFilter) (*types.UtilityBillListResult, error) {
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

	records := make([]types.UtilityBillRecord, 0, 256)
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for utility bill records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
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
			var meta utilityBillMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			legacyManual := meta.Kind != "utility_bill" && meta.AutoDeletedCount > 0
			if meta.Kind != "utility_bill" && !legacyManual {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 待补录占位行（manual / 恢复后空提取）：必须显示，用户点开编辑补录
			if utilityBillItemsAllBlank(meta.Records) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				records = append(records, types.UtilityBillRecord{
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
				records = append(records, types.UtilityBillRecord{
					RowKey:         fmt.Sprintf("%s-%d", k.ID, idx),
					Item:           r,
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

	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	for _, r := range records {
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !utilityBillDateInRange(r.Item.BillPeriodStart, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !utilityBillRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
	}
	records = kept

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
	return &types.UtilityBillListResult{
		Data:     records[start:end],
		Total:    total,
		Page:     filter.Page,
		PageSize: filter.PageSize,
	}, nil
}

func utilityBillItemsAllBlank(recs []types.UtilityBillExtractionItem) bool {
	if len(recs) == 0 {
		return true
	}
	for _, r := range recs {
		if strings.TrimSpace(r.AccountNo) != "" || strings.TrimSpace(r.AccountName) != "" ||
			r.TotalKwh != 0 || r.TotalAmount != 0 {
			return false
		}
	}
	return true
}

func utilityBillDateInRange(date, from, to string) bool {
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

func utilityBillRecordMatchKeyword(r types.UtilityBillRecord, kw string) bool {
	it := r.Item
	fields := []string{
		it.BillPeriodStart, it.BillPeriodEnd, it.AccountNo, it.AccountName, it.Address,
		it.UsageCategory, it.VoltageLevel, it.SupplyUnit, it.MarketAttr, it.PrintDate,
		it.MomChange, it.DueDate, it.Remark,
		fmt.Sprintf("%v", it.TotalKwh), fmt.Sprintf("%v", it.TotalAmount),
		fmt.Sprintf("%v", it.AvgPrice), fmt.Sprintf("%v", it.PowerFactor),
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
