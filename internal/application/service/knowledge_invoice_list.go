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
	"time"

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
	// 文件级归档分类：上传弹窗选择的票据类型（如"通行费发票"），列表平铺时
	// 覆盖规则提取硬编码分类，作为「细分分类」筛选的权威来源。
	FleetCertType string `json:"fleet_cert_type"`
}

// ListInvoiceRecords 实现发票级聚合列表。步骤：
//  1. 分页拉取知识库下全部 knowledge（含 tags）；
//  2. 平铺每份文档的 invoices，附上源文件上下文；
//  3. 按发票号去重（空号不去重；同号保留 created_at 最新）；
//  4. 按 filter 过滤、聚合、排序、分页。
func (s *BusinessExtractService) ListInvoiceRecords(ctx context.Context, kbID string, filter types.InvoiceListFilter) (*types.InvoiceListResult, error) {
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
				// 归档细分分类：文件级 fleet_cert_type（上传弹窗选择）优先，
				// 覆盖规则提取硬编码（如"通行费"→"通行费发票"），供细分筛选。
				cat := inv.Category
				if meta.FleetCertType != "" {
					cat = meta.FleetCertType
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
					Category:       cat,
					Items:          items,
					VoidFlag:       inv.VoidFlag,
					Fields:         inv.Fields,
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

	// 4. 过滤：关键字、类型、税率、状态、日期为 AND 组合（复合查询交集）。
	// 关键词命中（全字段/标签搜索）只是其中一个过滤条件，不再豁免类型/税率/日期筛选。
	kw := strings.ToLower(strings.TrimSpace(filter.Keyword))
	dateFrom, dateTo := strings.TrimSpace(filter.DateFrom), strings.TrimSpace(filter.DateTo)
	kept := records[:0]
	var sumAmount, sumTax, sumTotal float64
	for _, r := range records {
		if kw != "" && !invoiceRecordMatchKeyword(r, kw) {
			continue
		}
		if filter.InvoiceType != "" {
			if filter.InvoiceType == "其它票据" {
				// 「其它票据」为排除式桶，与 InvoiceOverviewStats 卡片口径一致：
				// category 不属于启用分类集合（含空/已删除分类旧值）的记录归入此筛选。
				// 前端卡片计数 = 总数 - 各启用分类卡之和（见 InvoiceManagement.vue overviewCards）。
				if len(filter.KnownCategories) > 0 {
					known := make(map[string]struct{}, len(filter.KnownCategories))
					for _, name := range filter.KnownCategories {
						// 「其它票据」已成为系统内置实体分类（seed invoice-misc）：
						// 它本身是排除式桶的归属目标，绝不能作为「已分类」从桶中剔除，
						// 否则 category=其它票据 的记录会被错误排除出该筛选。
						if name == "其它票据" {
							continue
						}
						known[name] = struct{}{}
					}
					if r.Category != "" {
						if _, ok := known[r.Category]; ok {
							continue
						}
					}
				} else {
					// 向后兼容：未传分类集合时回退 invoice_type 排除桶（非普通/专用）
					if r.InvoiceType == "普通发票" || r.InvoiceType == "专用发票" {
						continue
					}
				}
			} else if r.Category != filter.InvoiceType && !(r.Category != "" && strings.Contains(filter.InvoiceType, r.Category)) {
				// 分类筛选统一按归档分类 category 精确匹配（与概览卡口径一致）；
				// 规则产物可能用简称（如"通行费"），上传分类名（"通行费发票"）包含该简称时同样命中，兼容存量数据。
				continue
			}
		}
		if filter.TaxRate != nil {
			// 税率筛选：发票级税率精确命中 或 明细(items)任意档位命中。
			// 支持多明细多税率发票（如 13%,9%,6% 混开）按任一税率档筛选。
			matched := r.TaxRate != nil && approxEqualFloat(*r.TaxRate, *filter.TaxRate)
			if !matched {
				for _, it := range r.Items {
					if it.TaxRate != nil && approxEqualFloat(*it.TaxRate, *filter.TaxRate) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}
		if filter.Status != "" && r.ExtractStatus != filter.Status {
			continue
		}
		if !invoiceDateInRange(r.InvoiceDate, dateFrom, dateTo) {
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

	// 5. 排序：默认按文件分组（文件最新记录时间降序，新文件在前），同文件按页码升序，
	// 保证同一文件的多页发票（P1/P2...）连续排列、不被其它文件记录隔开。
	// 表头排序（invoice_no / invoice_date）时以该字段为主排序键，空值排后；
	// 主键相同仍先保证同文件连续，再按页升序。
	desc := filter.SortOrder != "asc"
	// 预计算每个文件的最新记录时间（分组排序键）
	fileLatest := map[string]time.Time{}
	for _, r := range records {
		if t, ok := fileLatest[r.KnowledgeID]; !ok || r.CreatedAt.After(t) {
			fileLatest[r.KnowledgeID] = r.CreatedAt
		}
	}
	groupBefore := func(i, j int) bool {
		if records[i].KnowledgeID == records[j].KnowledgeID {
			return false
		}
		a, b := fileLatest[records[i].KnowledgeID], fileLatest[records[j].KnowledgeID]
		if !a.Equal(b) {
			return a.After(b)
		}
		return records[i].KnowledgeID < records[j].KnowledgeID
	}
	if filter.SortBy == "invoice_no" || filter.SortBy == "invoice_date" {
		by := filter.SortBy
		sort.SliceStable(records, func(i, j int) bool {
			var a, b string
			if by == "invoice_no" {
				a, b = strings.ToLower(strings.TrimSpace(records[i].InvoiceNo)), strings.ToLower(strings.TrimSpace(records[j].InvoiceNo))
			} else {
				a, b = records[i].InvoiceDate, records[j].InvoiceDate
			}
			// 空值恒排后（无论升序还是降序）
			aEmpty, bEmpty := a == "", b == ""
			if aEmpty != bEmpty {
				return !aEmpty
			}
			if a != b {
				if desc {
					return a > b
				}
				return a < b
			}
			// 主键相同：同文件连续，再按页升序
			if records[i].KnowledgeID != records[j].KnowledgeID {
				return groupBefore(i, j)
			}
			return records[i].Page < records[j].Page
		})
	} else {
		sort.SliceStable(records, func(i, j int) bool {
			if records[i].KnowledgeID != records[j].KnowledgeID {
				return groupBefore(i, j)
			}
			return records[i].Page < records[j].Page
		})
	}

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

// InvoiceOverviewStats 计算发票台账概览统计。口径与 ListInvoiceRecords 完全一致：
// 遍历知识库下全部 knowledge → 平铺 invoices → 按发票号去重（保留最新）→ 聚合。
// 所有金额字段为空值时按 0 处理，不报类型转换错误（在内存中聚合，避免 JSONB
// 数组展开的 SQL 类型转换陷阱，同时兼容 Postgres 与 SQLite 双库）。
func (s *BusinessExtractService) InvoiceOverviewStats(ctx context.Context, kbID string) (*types.InvoiceOverviewStats, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)

	records := make([]types.InvoiceRecord, 0, 256)
	parseFailedFiles := 0
	fileCount := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for invoice overview: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			// 文件级解析失败计数：与 FleetOverviewStats 口径一致（parse_status=failed）。
			// 解析失败文件无论是否打标 kind=invoice，都算「已上传文件」，计入 file_count。
			if k.ParseStatus == "failed" {
				parseFailedFiles++
				fileCount++
			}
			if len(k.CustomMetadata) == 0 {
				continue
			}
			var meta invoiceMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" {
				continue
			}
			// 已打标 invoice 的文件计入已上传文件数（解析失败文件在上面已计，避免重复）
			if k.ParseStatus != "failed" {
				fileCount++
			}
			for _, inv := range meta.Invoices {
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
					InvoiceNo:     inv.InvoiceNo,
					InvoiceDate:   inv.InvoiceDate,
					InvoiceType:   inv.InvoiceType,
					TotalAmount:   inv.TotalAmount,
					Amount:        inv.Amount,
					Tax:           inv.Tax,
					TaxRate:       inv.TaxRate,
					SellerName:    inv.SellerName,
					BuyerName:     inv.BuyerName,
					Issuer:        inv.Issuer,
					Remark:        inv.Remark,
					Category:      inv.Category,
					Items:         items,
					VoidFlag:      inv.VoidFlag,
					Page:          inv.Page,
					KnowledgeID:   k.ID,
					ExtractStatus: meta.ExtractStatus,
					ExtractError:  meta.ExtractError,
					CreatedAt:     k.CreatedAt,
				})
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}

	// 按发票号去重（保留 created_at 最新），与列表口径一致
	records = dedupInvoiceRecords(records)

	stats := &types.InvoiceOverviewStats{
		ByInvoiceType: []types.InvoiceTypeStat{},
		ByCategory:    []types.InvoiceCategoryStat{},
	}
	stats.Total = len(records)
	stats.FileCount = fileCount
	stats.ParseFailed = parseFailedFiles

	// 当前月（基于开票日期 YYYY-MM-DD 或 YYYYMMDD）
	now := time.Now()
	monthPrefix := now.Format("2006-01") // YYYY-MM
	monthPrefixCompact := now.Format("200601")

	typeByMap := map[string]*types.InvoiceTypeStat{}
	for _, r := range records {
		if r.ExtractStatus == "failed" {
			stats.ExtractFailed++
		}
		if r.TotalAmount != nil {
			stats.SumTotal += *r.TotalAmount
		}
		// 本月看板：开票日期命中当前年月（兼容带横线与不带横线格式）
		date := strings.TrimSpace(r.InvoiceDate)
		if date != "" && (strings.HasPrefix(date, monthPrefix) || strings.HasPrefix(strings.ReplaceAll(date, "-", ""), monthPrefixCompact)) {
			stats.CurrentMonth.Count++
			if r.TotalAmount != nil {
				stats.CurrentMonth.SumTotal += *r.TotalAmount
			}
		}
		// 按发票类型分组
		typ := strings.TrimSpace(r.InvoiceType)
		if typ == "" {
			typ = "未分类"
		}
		// 其它票据桶：除专用发票/普通发票外的全部（含未分类/未知值）统一归入"其它票据"
		if typ != "专用发票" && typ != "普通发票" {
			typ = "其它票据"
		}
		t := typeByMap[typ]
		if t == nil {
			t = &types.InvoiceTypeStat{InvoiceType: typ}
			typeByMap[typ] = t
		}
		t.Count++
		if r.TotalAmount != nil {
			t.SumTotal += *r.TotalAmount
		}
	}
	for _, t := range typeByMap {
		stats.ByInvoiceType = append(stats.ByInvoiceType, *t)
	}
	// 按用户定义分类（category）分组：category 为空/空白归「未分类」；
	// 分类卡由前端按 categories(scope=invoice) enabled 列表动态渲染，计数精确匹配分类名。
	catByMap := map[string]*types.InvoiceCategoryStat{}
	for _, r := range records {
		cat := strings.TrimSpace(r.Category)
		if cat == "" {
			cat = "未分类"
		}
		t := catByMap[cat]
		if t == nil {
			t = &types.InvoiceCategoryStat{Category: cat}
			catByMap[cat] = t
		}
		t.Count++
		if r.TotalAmount != nil {
			t.SumTotal += *r.TotalAmount
		}
	}
	for _, t := range catByMap {
		stats.ByCategory = append(stats.ByCategory, *t)
	}
	// 分类桶按张数降序，稳定排序
	sort.Slice(stats.ByCategory, func(i, j int) bool {
		if stats.ByCategory[i].Count != stats.ByCategory[j].Count {
			return stats.ByCategory[i].Count > stats.ByCategory[j].Count
		}
		return stats.ByCategory[i].Category < stats.ByCategory[j].Category
	})
	// 保证专票/普票/其它票据三桶固定出现（空桶补 0），前端类型卡稳定展示
	seenType := map[string]bool{}
	for _, t := range stats.ByInvoiceType {
		seenType[t.InvoiceType] = true
	}
	for _, name := range []string{"专用发票", "普通发票", "其它票据"} {
		if !seenType[name] {
			stats.ByInvoiceType = append(stats.ByInvoiceType, types.InvoiceTypeStat{InvoiceType: name})
		}
	}
	// 按张数降序，稳定排序
	sort.Slice(stats.ByInvoiceType, func(i, j int) bool {
		return stats.ByInvoiceType[i].Count > stats.ByInvoiceType[j].Count
	})
	return stats, nil
}

// ListInvoiceTaxRates 返回该知识库下所有发票出现过的去重税率（含多档明细），
// 用于前端税率筛选下拉自动加载。税率为金额最大项税率 + 全部明细档位的并集。
func (s *BusinessExtractService) ListInvoiceTaxRates(ctx context.Context, kbID string) ([]types.InvoiceTaxRateCount, error) {
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
