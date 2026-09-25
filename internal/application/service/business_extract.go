package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// BusinessExtractService 承载 WeKnora 二次开发中「日常事务」模块的业务提取序号生成、
// 自定义元数据持久化等业务逻辑。与核心 KnowledgeService 物理隔离，不污染上游接口。
type BusinessExtractService struct {
	repo interfaces.KnowledgeRepository
}

// NewBusinessExtractService 构造业务提取 Service。
func NewBusinessExtractService(repo interfaces.KnowledgeRepository) *BusinessExtractService {
	return &BusinessExtractService{repo: repo}
}

// MaxAutoContractSeq returns the highest trailing sequence number among
// contracts with an auto-generated number HT-YYYYMMDD-NNN for the current date
// inside the knowledge base.
func (s *BusinessExtractService) MaxAutoContractSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "HT-" + time.Now().Format("20060102") + "-"
	max := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return max, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta contractMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "contract" {
				continue
			}
			for _, ct := range meta.Contracts {
				if !strings.HasPrefix(ct.ContractNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(ct.ContractNo, prefix)
				if v, aerr := strconv.Atoi(n); aerr == nil && v > max {
					max = v
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return max, nil
}

func (s *BusinessExtractService) MaxAutoAwardPunishSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "JC-" + time.Now().Format("20060102") + "-"
	max := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return max, err
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
				if !strings.HasPrefix(r.ApNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(r.ApNo, prefix)
				if v, aerr := strconv.Atoi(n); aerr == nil && v > max {
					max = v
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return max, nil
}


func (s *BusinessExtractService) MaxAutoRegulationSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "ZD-" + time.Now().Format("20060102") + "-"
	max := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return max, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta regulationMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "regulation" {
				continue
			}
			for _, r := range meta.Regulations {
				if !strings.HasPrefix(r.RegNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(r.RegNo, prefix)
				if v, aerr := strconv.Atoi(n); aerr == nil && v > max {
					max = v
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return max, nil
}

// MarkInvoiceDuplicates scans sibling invoice records in the same KB and marks
// invoices whose invoice_no duplicates an existing record as Duplicate=true.
func (s *BusinessExtractService) MarkInvoiceDuplicates(ctx context.Context, kbID, currentKnowledgeID string, meta *InvoiceCustomMetadata) {
	if len(meta.Invoices) == 0 {
		return
	}
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	items, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, &types.Pagination{Page: 1, PageSize: 1000}, types.KnowledgeListFilter{})
	if err != nil {
		return
	}
	existing := make(map[string]struct{})
	for _, k := range items {
		if k == nil || k.ID == currentKnowledgeID || len(k.CustomMetadata) == 0 {
			continue
		}
		var sibling struct {
			Invoices []InvoiceExtractionItem `json:"invoices"`
		}
		if err := json.Unmarshal(k.CustomMetadata, &sibling); err != nil {
			continue
		}
		for _, inv := range sibling.Invoices {
			if no := strings.TrimSpace(inv.InvoiceNo); no != "" {
				existing[no] = struct{}{}
			}
		}
	}
	for i := range meta.Invoices {
		if no := strings.TrimSpace(meta.Invoices[i].InvoiceNo); no != "" {
			if _, dup := existing[no]; dup {
				meta.Invoices[i].Duplicate = true
			}
		}
	}
}

// InvoiceCustomMetadata is the persisted shape written into a knowledge entry's
// custom_metadata by ExtractInvoice.
type InvoiceCustomMetadata struct {
	Kind          string                  `json:"kind"`
	Invoices      []InvoiceExtractionItem `json:"invoices"`
	ExtractStatus string                  `json:"extract_status"`
	ExtractError  string                  `json:"extract_error"`
}

// ListDeletedKnowledge lists soft-deleted rows in a KB.
func (s *BusinessExtractService) ListDeletedKnowledge(ctx context.Context, tenantID uint64, kbID string, page, pageSize int, keyword string) (interface{}, int64, error) {
	items, total, err := s.repo.ListDeletedKnowledge(ctx, tenantID, kbID, page, pageSize, keyword)
	if err != nil {
		return nil, 0, err
	}
	return map[string]interface{}{"items": items, "total": total}, total, nil
}

// RestoreDeletedKnowledge clears the soft-delete tombstone.
func (s *BusinessExtractService) RestoreDeletedKnowledge(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.RestoreDeletedKnowledgeRow(ctx, tenantID, id)
}

// PurgeDeletedKnowledge hard-deletes a soft-deleted row.
func (s *BusinessExtractService) PurgeDeletedKnowledge(ctx context.Context, tenantID uint64, id string) error {
	return s.repo.HardDeleteKnowledge(ctx, tenantID, id)
}
