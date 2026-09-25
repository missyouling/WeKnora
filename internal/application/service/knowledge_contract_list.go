// 合同级聚合列表：把知识库下所有已提取合同平铺为"一行一份合同"的记录，
// 在服务端完成全字段搜索、类型/履约状态/签订日期筛选、排序、分页与金额聚合，
// 供合同管理页直接渲染。重复判断复用知识库文件上传层（file_hash / 文件名+大小），
// 列表层不再按合同编号去重。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// contractMetadata 与 handler.contractCustomMetadata 对齐，用于从 custom_metadata
// 反序列化合同提取结果。
type contractMetadata struct {
	Kind             string                   `json:"kind"`
	Contracts        []ContractExtractionItem `json:"contracts"`
	ExtractStatus    string                   `json:"extract_status"`
	ExtractError     string                   `json:"extract_error"`
	AutoDeletedCount int                      `json:"auto_deleted_count"`
}

// ListContractRecords 实现合同级聚合列表。
func (s *knowledgeService) ListContractRecords(ctx context.Context, kbID string, filter types.ContractListFilter) (*types.ContractListResult, error) {
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

	records := make([]types.ContractRecord, 0, 256)
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for contract records: %w", err)
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
			var meta contractMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count（曾被自动删除后
			// 人工恢复）→ 视作待补录占位行，避免这类记录刷新后从列表消失。
			legacyManual := meta.Kind != "contract" && meta.AutoDeletedCount > 0
			if meta.Kind != "contract" && !legacyManual {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 人工入库占位行：恢复（重新入库）后 extract_status=manual，字段待用户
			// 编辑补录 —— 列表必须立即显示该记录（用户可点开编辑）。
			// 同时兜底：提取/保存后仍无任何有效字段（success + 全空）的记录同样
			// 显示为待补录占位，避免"打开弹窗未编辑→自动保存空字段"后记录从列表消失。
			if contractItemsAllBlank(meta.Contracts) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				records = append(records, types.ContractRecord{
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
			if len(meta.Contracts) == 0 {
				continue
			}
			for _, ct := range meta.Contracts {
				// 过滤"空提取"记录（无编号、无名称、无双方、无金额）
				if contractExtractionItemBlank(ct) {
					continue
				}
				records = append(records, types.ContractRecord{
					ContractNo:       ct.ContractNo,
					ContractName:     ct.ContractName,
					ContractType:     ct.ContractType,
					SignDate:         ct.SignDate,
					EffectiveDate:    ct.EffectiveDate,
					ExpiryDate:       ct.ExpiryDate,
					SignPlace:        ct.SignPlace,
					PartyAName:       ct.PartyAName,
					PartyATaxNo:      ct.PartyATaxNo,
					PartyAAddress:    ct.PartyAAddress,
					PartyAPhone:      ct.PartyAPhone,
					PartyABank:       ct.PartyABank,
					PartyAAccount:    ct.PartyAAccount,
					PartyBName:       ct.PartyBName,
					PartyBTaxNo:      ct.PartyBTaxNo,
					PartyBAddress:    ct.PartyBAddress,
					PartyBPhone:      ct.PartyBPhone,
					PartyBBank:       ct.PartyBBank,
					PartyBAccount:    ct.PartyBAccount,
					ContractAmount:   ct.ContractAmount,
					TaxRate:          ct.TaxRate,
					PaymentMethod:    ct.PaymentMethod,
					QualityBond:      ct.QualityBond,
					LiquidatedDamages: ct.LiquidatedDamages,
					Subject:          ct.Subject,
					Quantity:         ct.Quantity,
					UnitPrice:        ct.UnitPrice,
					PerformancePeriod: ct.PerformancePeriod,
					Handler:          ct.Handler,
					Department:       ct.Department,
					Remark:           ct.Remark,
					FulfillStatus:    ct.FulfillStatus,
					Page:             ct.Page,
					KnowledgeID:      k.ID,
					KnowledgeTitle:   k.Title,
					FileName:         k.FileName,
					FileType:         k.FileType,
					Tags:             tags,
					ExtractStatus:    meta.ExtractStatus,
					ExtractError:     meta.ExtractError,
					KBID:             k.KnowledgeBaseID,
					CreatedAt:        k.CreatedAt,
				})
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	// 过滤（不做合同编号去重——文件层已按哈希去重）
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	var sumAmount, sumTotal float64
	for _, r := range records {
		if filter.ContractType != "" && r.ContractType != filter.ContractType {
			continue
		}
		if filter.FulfillStatus != "" && r.FulfillStatus != filter.FulfillStatus {
			continue
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !contractDateInRange(r.SignDate, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !contractRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
		if r.ContractAmount != nil {
			sumAmount += *r.ContractAmount
			sumTotal += *r.ContractAmount
		}
	}
	records = kept

	// 排序：源文件创建时间新在前；合同一份文件一份，同文件多份时保持提取顺序
	// （不按页排序——合同不区分页）。
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
	return &types.ContractListResult{
		Data:      records[start:end],
		Total:     total,
		SumAmount: sumAmount,
		SumTax:    0,
		SumTotal:  sumTotal,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
	}, nil
}

// contractExtractionItemBlank 判断合同提取项是否为"空提取"。
// contractItemsAllBlank 判断合同提取列表是否完全没有有效数据（空数组或每项都为空提取）。
func contractItemsAllBlank(cts []ContractExtractionItem) bool {
	if len(cts) == 0 {
		return true
	}
	for _, ct := range cts {
		if !contractExtractionItemBlank(ct) {
			return false
		}
	}
	return true
}

func contractExtractionItemBlank(ct ContractExtractionItem) bool {
	if strings.TrimSpace(ct.ContractNo) != "" || strings.TrimSpace(ct.ContractName) != "" ||
		strings.TrimSpace(ct.ContractType) != "" || strings.TrimSpace(ct.PartyAName) != "" ||
		strings.TrimSpace(ct.PartyBName) != "" {
		return false
	}
	if ct.ContractAmount != nil && *ct.ContractAmount != 0 {
		return false
	}
	return true
}

// contractDateInRange 判断日期（YYYY-MM-DD）是否落在 [from, to]（含，空端不限）。
func contractDateInRange(date, from, to string) bool {
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

// contractRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
func contractRecordMatchKeyword(r types.ContractRecord, kw string) bool {
	fields := []string{
		r.ContractNo, r.ContractName, r.ContractType,
		r.SignDate, r.EffectiveDate, r.ExpiryDate, r.SignPlace,
		r.PartyAName, r.PartyATaxNo, r.PartyAAddress, r.PartyAPhone, r.PartyABank, r.PartyAAccount,
		r.PartyBName, r.PartyBTaxNo, r.PartyBAddress, r.PartyBPhone, r.PartyBBank, r.PartyBAccount,
		r.PaymentMethod, r.Subject, r.PerformancePeriod, r.Handler, r.Department, r.Remark,
		r.FulfillStatus, r.FileName, r.KnowledgeTitle, r.ExtractStatus, r.ExtractError,
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
	for _, v := range []*float64{r.ContractAmount, r.QualityBond, r.LiquidatedDamages, r.Quantity, r.UnitPrice} {
		if v != nil && strings.Contains(fmt.Sprintf("%.2f", *v), kw) {
			return true
		}
	}
	// 税率匹配：输入"3%" → 0.03；输入 "0.03" 直接匹配小数
	if r.TaxRate != nil {
		pct := kw
		if strings.HasSuffix(pct, "%") {
			pct = strings.TrimSuffix(pct, "%")
			if num, err := strconv.ParseFloat(pct, 64); err == nil {
				if math.Abs(*r.TaxRate-num/100.0) < 1e-9 {
					return true
				}
			}
		} else if num, err := strconv.ParseFloat(pct, 64); err == nil && !strings.ContainsAny(pct, "%") {
			if math.Abs(*r.TaxRate-num) < 1e-9 {
				return true
			}
		}
	}
	return false
}

// ListContractTypes 返回该知识库下所有合同出现过的去重合同类型（含数量），
// 用于前端合同类型筛选下拉自动加载。
func (s *knowledgeService) ListContractTypes(ctx context.Context, kbID string) ([]types.ContractTypeCount, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	page := 1
	batch := 100
	seen := map[string]int{}
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for contract types: %w", err)
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			var meta contractMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "contract" {
				continue
			}
			for _, ct := range meta.Contracts {
				if ct.ContractType != "" {
					seen[ct.ContractType]++
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	types_ := make([]types.ContractTypeCount, 0, len(seen))
	for t, cnt := range seen {
		types_ = append(types_, types.ContractTypeCount{ContractType: t, Count: cnt})
	}
	sort.Slice(types_, func(i, j int) bool {
		if types_[i].Count != types_[j].Count {
			return types_[i].Count > types_[j].Count
		}
		return types_[i].ContractType < types_[j].ContractType
	})
	return types_, nil
}
