// 发票级聚合列表：把知识库下所有已提取发票平铺为"一行一张发票"的记录，
// 在服务端完成按发票号去重（保留最新 created_at）、全字段搜索、类型/状态/
// 日期筛选、排序、分页与金额聚合，供发票管理页直接渲染。
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

// invoiceMetadata 与 handler.invoiceCustomMetadata 对齐，用于从 custom_metadata
// 反序列化发票提取结果。
type invoiceMetadata struct {
	Kind             string                  `json:"kind"`
	Invoices         []InvoiceExtractionItem `json:"invoices"`
	ExtractStatus    string                  `json:"extract_status"`
	ExtractError     string                  `json:"extract_error"`
	AutoDeletedCount int                     `json:"auto_deleted_count"`
}

// ListInvoiceRecords 实现发票级聚合列表。步骤：
//  1. 分页拉取知识库下全部 knowledge（含 tags）；
//  2. 平铺每份文档的 invoices，附上源文件上下文；
//  3. 按发票号去重（空号不去重；同号保留 created_at 最新）；
//  4. 按 filter 过滤、聚合、排序、分页。
func (s *knowledgeService) ListInvoiceRecords(ctx context.Context, kbID string, filter types.InvoiceListFilter) (*types.InvoiceListResult, error) {
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

	// 1. 拉全量 knowledge（分页循环，避免单页上限）
	records := make([]types.InvoiceRecord, 0, 256)
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for invoice records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		// 加载标签：repo 层分页列表不返回 tags，需单独补取（与文档列表保持一致）
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
		// 2. 平铺 invoices
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta invoiceMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count（曾被自动删除后
			// 人工恢复）→ 视作待补录占位行，避免这类记录刷新后从列表消失。
			legacyManual := meta.Kind != "invoice" && meta.AutoDeletedCount > 0
			if meta.Kind != "invoice" && !legacyManual {
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
			if invoiceItemsAllBlank(meta.Invoices) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				records = append(records, types.InvoiceRecord{
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
			if len(meta.Invoices) == 0 {
				continue
			}
			for _, inv := range meta.Invoices {
					// 过滤"空提取"记录：LLM 提取失败返回的空对象（无发票号且无任何
					// 有效内容）不作为发票展示，避免列表出现无意义的空行。
					if invoiceExtractionItemBlank(inv) {
						continue
					}
					items := make([]types.InvoiceExtractionItemItems, 0, len(inv.Items))
				for _, it := range inv.Items {
					items = append(items, types.InvoiceExtractionItemItems{
						Name:    it.Name,
						Qty:     it.Qty,
						Price:   it.Price,
						TaxRate: it.TaxRate,
					})
				}
				records = append(records, types.InvoiceRecord{
					InvoiceNo:      inv.InvoiceNo,
					InvoiceCode:    inv.InvoiceCode,
					InvoiceDate:    inv.InvoiceDate,
					InvoiceType:    inv.InvoiceType,
					TotalAmount:    inv.TotalAmount,
					Amount:         inv.Amount,
					Tax:            inv.Tax,
					TaxRate:        inv.TaxRate,
					SellerName:     inv.SellerName,
					SellerTaxNo:    inv.SellerTaxNo,
					SellerAddress:  inv.SellerAddr,
					SellerPhone:    inv.SellerPhone,
					SellerBank:     inv.SellerBank,
					SellerAccount:  inv.SellerAcct,
					BuyerName:      inv.BuyerName,
					BuyerTaxNo:     inv.BuyerTaxNo,
					BuyerAddress:   inv.BuyerAddr,
					BuyerPhone:     inv.BuyerPhone,
					BuyerBank:      inv.BuyerBank,
					BuyerAccount:   inv.BuyerAcct,
					Issuer:         inv.Issuer,
					Remark:         inv.Remark,
					Category:       inv.Category,
					Items:          items,
					VoidFlag:       inv.VoidFlag,
					Page:           inv.Page,
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

	// 3. 按发票号去重（保留 created_at 最新）
	records = dedupInvoiceRecords(records)

	// 4. 过滤
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	var sumAmount, sumTax, sumTotal float64
	for _, r := range records {
		if filter.InvoiceType != "" && r.InvoiceType != filter.InvoiceType {
			continue
		}
		if filter.TaxRate != nil && (r.TaxRate == nil || !approxEqualFloat(*r.TaxRate, *filter.TaxRate)) {
			continue
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !invoiceDateInRange(r.InvoiceDate, dateFrom, dateTo) {
			continue
		}
		if kw != "" && !invoiceRecordMatchKeyword(r, kw) {
			continue
		}
		kept = append(kept, r)
		if r.Amount != nil {
			sumAmount += *r.Amount
		}
		if r.Tax != nil {
			sumTax += *r.Tax
		}
		if r.TotalAmount != nil {
			sumTotal += *r.TotalAmount
		}
	}
	records = kept

	// 5. 排序：源文件创建时间新在前；同文件按发票页码升序
	sort.SliceStable(records, func(i, j int) bool {
		if !records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		}
		return records[i].Page < records[j].Page
	})

	// 6. 分页
	total := len(records)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	return &types.InvoiceListResult{
		Data:      records[start:end],
		Total:     total,
		SumAmount: sumAmount,
		SumTax:    sumTax,
		SumTotal:  sumTotal,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
	}, nil
}

// invoiceItemsAllBlank 判断提取列表是否完全没有有效数据（空数组或每项都为空提取）。
func invoiceItemsAllBlank(invs []InvoiceExtractionItem) bool {
	if len(invs) == 0 {
		return true
	}
	for _, inv := range invs {
		if !invoiceExtractionItemBlank(inv) {
			return false
		}
	}
	return true
}

// invoiceExtractionItemBlank 判断发票提取项是否为"空提取"（LLM 提取失败返回
// 的空对象）：发票号、日期、类型、购销方、开票人、金额、税额、价税合计全部为空。
func invoiceExtractionItemBlank(inv InvoiceExtractionItem) bool {
	if strings.TrimSpace(inv.InvoiceNo) != "" || strings.TrimSpace(inv.InvoiceDate) != "" ||
		strings.TrimSpace(inv.InvoiceType) != "" || strings.TrimSpace(inv.BuyerName) != "" ||
		strings.TrimSpace(inv.SellerName) != "" || strings.TrimSpace(inv.Issuer) != "" {
		return false
	}
	if inv.Amount != nil && *inv.Amount != 0 || inv.Tax != nil && *inv.Tax != 0 ||
		inv.TotalAmount != nil && *inv.TotalAmount != 0 {
		return false
	}
	return true
}

// dedupInvoiceRecords 按发票号去重：空号记录全部保留；同号保留 created_at
// 最新的一条。
func dedupInvoiceRecords(records []types.InvoiceRecord) []types.InvoiceRecord {
	best := make(map[string]int, len(records)) // invoice_no -> index
	out := make([]types.InvoiceRecord, 0, len(records))
	for _, r := range records {
		no := strings.TrimSpace(r.InvoiceNo)
		if no == "" {
			out = append(out, r)
			continue
		}
		if idx, ok := best[no]; ok {
			if r.CreatedAt.After(out[idx].CreatedAt) {
				out[idx] = r // 同号且更新，替换
			}
			continue
		}
		best[no] = len(out)
		out = append(out, r)
	}
	return out
}

// invoiceDateInRange 判断开票日期（YYYY-MM-DD）是否落在 [from, to]（含，空端不限）。
func invoiceDateInRange(date, from, to string) bool {
	date = strings.TrimSpace(date)
	if from == "" && to == "" {
		return true
	}
	if date == "" {
		return false // 无日期时若设置了日期筛选则不命中
	}
	// 日期格式兼容 YYYY-MM-DD 与 YYYYMMDD
	normalize := func(s string) string {
		s = strings.ReplaceAll(strings.TrimSpace(s), "-", "")
		return s
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

// invoiceRecordMatchKeyword 判断记录是否命中全字段搜索词（大小写不敏感）。
func invoiceRecordMatchKeyword(r types.InvoiceRecord, kw string) bool {
	fields := []string{
		r.InvoiceNo, r.InvoiceCode, r.InvoiceDate, r.InvoiceType,
		r.SellerName, r.SellerTaxNo, r.BuyerName, r.BuyerTaxNo,
		r.Issuer, r.Remark, r.Category, r.FileName, r.KnowledgeTitle,
		r.ExtractStatus, r.ExtractError,
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
	// 金额字段转字符串匹配
	for _, v := range []*float64{r.Amount, r.Tax, r.TotalAmount} {
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

// approxEqualFloat 浮点税率比较（小数，1e-9 精度）。
func approxEqualFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ListInvoiceTaxRates 返回该知识库下所有发票出现过的去重税率（含多档明细），
// 用于前端税率筛选下拉自动加载。税率为金额最大项税率 + 全部明细档位的并集。
func (s *knowledgeService) ListInvoiceTaxRates(ctx context.Context, kbID string) ([]types.InvoiceTaxRateCount, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	page := 1
	batch := 100
	seen := map[float64]int{} // 税率 -> 出现次数
	norm := func(v *float64) {
		if v == nil {
			return
		}
		key := math.Round(*v*1e9) / 1e9
		seen[key]++
	}
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for tax rates: %w", err)
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			var meta invoiceMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" {
				continue
			}
			for _, inv := range meta.Invoices {
				norm(inv.TaxRate)
				for _, it := range inv.Items {
					norm(it.TaxRate)
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	rates := make([]types.InvoiceTaxRateCount, 0, len(seen))
	for rate, cnt := range seen {
		rates = append(rates, types.InvoiceTaxRateCount{TaxRate: rate, Count: cnt})
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i].TaxRate < rates[j].TaxRate })
	return rates, nil
}
