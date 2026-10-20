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
	// InvoiceCategory 是新权威键（上传即持久化），FleetCertType 是历史键，
	// 取值走权威链（与 knowledge_invoice_category_lock.go 的 resolveInvoiceCertType
	// 同口径）：InvoiceCategory 优先、FleetCertType 兜底。缺任何一个都会让
	// 只写过其中一键的文件在列表里落到另一个名字下（2026-10-20 回归）。
	FleetCertType   string `json:"fleet_cert_type"`
	InvoiceCategory string `json:"invoice_category"`
}

// fileLevelInvoiceCategory 返回文件级权威分类（空串表示无）。
// 优先级与上传/提取链路完全一致：invoice_category > fleet_cert_type。
func (m invoiceMetadata) fileLevelInvoiceCategory() string {
	return strings.TrimSpace(resolveInvoiceCertType("", m.InvoiceCategory, m.FleetCertType))
}

// ListInvoiceRecords 实现发票级聚合列表。步骤：
//  1. 分页拉取知识库下全部 knowledge（含 tags）；
//  2. 平铺每份文档的 invoices，附上源文件上下文；
//  3. 按发票号去重（空号不去重；同号保留 created_at 最新）；
//  4. 按 filter 过滤、聚合、排序、分页。
func (s *BusinessExtractService) ListInvoiceRecords(ctx context.Context, kbID string, filter types.InvoiceListFilter) (*types.InvoiceListResult, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant id missing in context")
	}
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
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{
			// 本函数在内存里重算 total，仓储 COUNT 的结果被直接丢弃 —— 跳过它可
			// 省一次数据库往返（Supabase 跨区 pooler 下约 350ms）。
			SkipTotalCount: true,
			// description 是整篇摘要 Markdown，列表页一个字都不展示。
			OmitDescription: true,
		})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for invoice records: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		// 2. 平铺 invoices
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			// 解析/提取中的文件由列表顶部状态条展示，不以占位行占用列表
			// （既有交互契约，见 InvoiceManagement.vue 顶部 BusinessParseStatusBar）。
			if k.ParseStatus == "pending" || k.ParseStatus == "processing" || k.ParseStatus == "finalizing" {
				continue
			}
			tags := make([]string, 0, len(k.Tags))
			for _, t := range k.Tags {
				if t != nil && t.Name != "" {
					tags = append(tags, t.Name)
				}
			}
			// 铁律一（知识库 ↔ 列表 100% 对齐）：文件还在知识库里，就绝不允许从
			// 列表里静默消失。凡是无法产出任何发票行的文件，一律渲染文件级占位行，
			// 让它能被看见、能重新提取或删除 —— 否则它会变成"既看不见、又被相同
			// 文件哈希挡住重传"的孤儿数据。
			placeholderFor := func(reason string, extractStatus, extractError string) {
				records = append(records, types.InvoiceRecord{
					KnowledgeID:       k.ID,
					KnowledgeTitle:    k.Title,
					FileName:          k.FileName,
					FileType:          k.FileType,
					Tags:              tags,
					ExtractStatus:     extractStatus,
					ExtractError:      extractError,
					KBID:              k.KnowledgeBaseID,
					CreatedAt:         k.CreatedAt,
					IsPlaceholder:     true,
					PlaceholderReason: reason,
				})
			}
			if len(k.CustomMetadata) == 0 {
				placeholderFor(invoicePlaceholderReason("", "", k.ParseStatus), "", "")
				continue
			}
			var meta invoiceMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				placeholderFor("文件记录异常", "", "")
				continue
			}
			// 交互契约（2026-10-20）：**提取失败**的文件不进发票主列表（含「其它票据」）。
			// 背景：批量上传触发 429/1302 限流时，一批文件会同时提取失败并各自生成
			// 一条「提取失败：…」占位行，把「其它票据」列表整屏染成橙色占位行，
			// 真实的票据反而被淹没。这类文件的归宿是「历史记录」抽屉（那里以
			// 「提取失败」状态呈现并可一键重提），不在主列表占位。
			// 注意：仅当它确实没有任何有效发票时才跳过 —— 部分成功的文件仍要展示
			// 已提取出的票据（数据不能因为一次失败就整体隐身）。
			skipExtractFailed := invoiceExtractFailedWithoutInvoices(meta.ExtractStatus, meta.Invoices)
			if skipExtractFailed {
				continue
			}
			// 旧版恢复行兜底：meta 无 kind 但保留 auto_deleted_count（曾被自动删除后
			// 人工恢复）→ 视作待补录占位行，避免这类记录刷新后从列表消失。
			legacyManual := meta.Kind != "invoice" && meta.AutoDeletedCount > 0
			if meta.Kind != "invoice" && !legacyManual {
				// 发票库里被判定为非票据的文件（误传合同/收据等）同样必须可见，
				// 并明确标注原因，否则用户既找不到它、也无法通过重传纠正分类。
				reason := "未识别为发票"
				if meta.Kind == "not_invoice" && strings.TrimSpace(meta.ExtractError) != "" {
					reason = "未识别为发票：" + strings.TrimSpace(meta.ExtractError)
				}
				placeholderFor(reason, meta.ExtractStatus, meta.ExtractError)
				continue
			}
			// 人工入库占位行：恢复（重新入库）后 extract_status=manual，字段待用户
			// 编辑补录 —— 列表必须立即显示该记录（用户可点开编辑）。
			// 同时兜底：提取/保存后仍无任何有效字段（success + 全空）的记录同样
			// 显示为待补录占位，避免"打开弹窗未编辑→自动保存空字段"后记录从列表消失。
			if invoiceItemsAllBlank(meta.Invoices) &&
				(meta.ExtractStatus == "manual" || meta.ExtractStatus == "success" || legacyManual) {
				placeholderFor("待补录", "manual", meta.ExtractError)
				continue
			}
			if len(meta.Invoices) == 0 {
				// invoices 为空且不在上面的兜底分支里：提取失败 / 解析出 0 张 /
				// 最后一票被删后正待清理 —— 同样渲染文件级占位行。
				placeholderFor(
					invoicePlaceholderReason(meta.ExtractStatus, meta.ExtractError, k.ParseStatus),
					meta.ExtractStatus, meta.ExtractError)
				continue
			}
			appendedForFile := 0
			for _, inv := range meta.Invoices {
				// 过滤"空提取"记录：LLM 提取失败返回的空对象（无发票号且无任何
				// 有效内容）不作为发票展示，避免列表出现无意义的空行。
				if invoiceExtractionItemBlank(inv) {
					continue
				}
				// 归档细分分类：文件级权威分类（invoice_category 优先、fleet_cert_type
				// 历史兜底）优先，覆盖规则提取硬编码（如"通行费"→"通行费发票"），
				// 供细分筛选；文件级缺失时才回退票内 category。
				cat := meta.fileLevelInvoiceCategory()
				if cat == "" {
					cat = strings.TrimSpace(inv.Category)
				}
				items := make([]types.InvoiceExtractionItemItems, 0, len(inv.Items))
				for _, it := range inv.Items {
					items = append(items, types.InvoiceExtractionItemItems{
						Name:      it.Name,
						Spec:      invoiceItemSpec(it),
						Unit:      it.Unit,
						Qty:       it.Qty,
						Price:     it.Price,
						Amount:    it.Amount,
						TaxRate:   it.TaxRate,
						TaxAmount: it.TaxAmt,
						Extra:     it.Extra,
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
				appendedForFile++
			}
			// 铁律一（续）：invoices 非空但每张都是空白提取（LLM 空对象）时，
			// 上面的循环一条都没追加 —— 这正是"知识库里有文件、列表 0 条"的
			// 隐身路径。同样渲染文件级占位行，而不是静默丢弃整份文件。
			if appendedForFile == 0 {
				records = append(records, types.InvoiceRecord{
					KnowledgeID:       k.ID,
					KnowledgeTitle:    k.Title,
					FileName:          k.FileName,
					FileType:          k.FileType,
					Tags:              tags,
					ExtractStatus:     meta.ExtractStatus,
					ExtractError:      meta.ExtractError,
					KBID:              k.KnowledgeBaseID,
					CreatedAt:         k.CreatedAt,
					IsPlaceholder:     true,
					PlaceholderReason: invoicePlaceholderReason(meta.ExtractStatus, meta.ExtractError, k.ParseStatus),
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
		if !invoiceRecordDateInRange(r, dateFrom, dateTo) {
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
	pageRows := records[start:end]
	// 标签只对**当前页**回填：列表只展示 20 行，而按全部文件取标签会多传上百行
	// 数据；标签也不参与筛选/排序，放在分页之后是安全的。
	if len(pageRows) > 0 {
		ids := make([]string, 0, len(pageRows))
		for i := range pageRows {
			ids = append(ids, pageRows[i].KnowledgeID)
		}
		if tagMap, terr := s.repo.GetKnowledgeTags(ctx, ids); terr == nil {
			for i := range pageRows {
				names := make([]string, 0, 2)
				for _, t := range tagMap[pageRows[i].KnowledgeID] {
					if t != nil && t.Name != "" {
						names = append(names, t.Name)
					}
				}
				pageRows[i].Tags = names
			}
		}
	}

	return &types.InvoiceListResult{
		Data:      pageRows,
		Total:     total,
		SumAmount: sumAmount,
		SumTax:    sumTax,
		SumTotal:  sumTotal,
		Page:      filter.Page,
		PageSize:  filter.PageSize,
	}, nil
}

// invoicePlaceholderReason 生成占位行原因文案，直接可展示给用户。
// 优先级：文件删除中 > 解析失败 > 提取失败 > 提取中 > 未提取。
func invoicePlaceholderReason(extractStatus, extractError, parseStatus string) string {
	switch {
	case parseStatus == "deleting":
		return "文件删除中"
	case parseStatus == "failed":
		return "解析失败"
	case extractStatus == "failed":
		if strings.TrimSpace(extractError) != "" {
			return "提取失败：" + strings.TrimSpace(extractError)
		}
		return "提取失败"
	case extractStatus == "pending" || extractStatus == "processing":
		return "提取中"
	case strings.TrimSpace(extractError) != "":
		return "未提取出票据：" + strings.TrimSpace(extractError)
	default:
		return "未提取"
	}
}

// invoiceExtractFailedWithoutInvoices 判定「提取失败且无任何有效发票」。
//
// 这类文件按 2026-10-20 交互契约从发票主列表（含「其它票据」）排除，只在
// 「历史记录」抽屉里以「提取失败」状态呈现：批量上传触发限流时，一批文件会同时
// 提取失败，若各自生成占位行会把列表整屏污染，真实票据反而被淹没。
//
// 保守判定：只有 extract_status=="failed" **且** 确实没有任何有效发票时才排除；
// 部分成功（失败状态但仍提取出若干张）的文件必须照常展示已提取的票据。
func invoiceExtractFailedWithoutInvoices(extractStatus string, invs []InvoiceExtractionItem) bool {
	if strings.TrimSpace(extractStatus) != "failed" {
		return false
	}
	return invoiceItemsAllBlank(invs)
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
//
// 穿透范围（缺一不可，历史踩坑）：
//   - 顶层发票字段 + 标签 + 金额 + 税率；
//   - invoices[].fields 自定义动态字段桶（通行费「车牌号」就落在这里，
//     顶层 InvoiceNo/SellerName 全都不含它 → 只搜顶层等于搜不到）；
//   - items[] 的 Name/Spec/Unit/Extra 明细子列值（同一份文件的车牌号也可能
//     只在明细行里）。
//
// 曾因只搜顶层字段，用户按车牌 `87567` 查询 13 张已在库的通行费发票得到
// 「共 0 条」，误判数据丢失后重新上传又被「知识库中已有相同文件」挡住。
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
	// 发票级自定义动态字段（fields 桶）key + value：车牌号/自定义编码等
	if mapContainsKeyword(r.Fields, kw) {
		return true
	}
	// 明细子列（items）Name/Spec/Unit + Extra 桶：车牌号、通行日期、车辆类型等
	for _, it := range r.Items {
		for _, f := range []string{it.Name, it.Spec, it.Unit} {
			if strings.Contains(strings.ToLower(f), kw) {
				return true
			}
		}
		if mapContainsKeyword(it.Extra, kw) {
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

// mapContainsKeyword 判断 JSONB 明细 map 的 key 或 string/number/bool value
// 是否命中关键词。数字值按 %g/%f 双格式匹配，防止模型把车牌号写成 12345678
// （数字）后再次从搜索中隐身。
func mapContainsKeyword(m map[string]any, kw string) bool {
	for k, v := range m {
		if kw != "" && strings.Contains(strings.ToLower(k), kw) {
			return true
		}
		switch val := v.(type) {
		case string:
			if strings.Contains(strings.ToLower(val), kw) {
				return true
			}
		case float64:
			if strings.Contains(strconv.FormatFloat(val, 'f', -1, 64), kw) {
				return true
			}
			if strings.Contains(fmt.Sprintf("%.2f", val), kw) {
				return true
			}
		case bool:
			if strings.Contains(strconv.FormatBool(val), kw) {
				return true
			}
		case json.Number:
			if strings.Contains(strings.ToLower(val.String()), kw) {
				return true
			}
		case []any:
			for _, e := range val {
				if s, ok := e.(string); ok && strings.Contains(strings.ToLower(s), kw) {
					return true
				}
			}
		}
	}
	return false
}

// invoiceRecordDateInRange 判断记录是否落在日期区间 [from, to]（含，空端不限）。
//
// 两路命中（OR）：
//  1. 顶层开票日期 invoice_date 落在区间内；
//  2. 明细子列的通行日期起止（travel_from / travel_to，含中文同义键
//     「通行日期起/止」「入口/出口日期」）与区间**有交集**。
//
// 为什么需要第 2 路：通行费电子票据汇总单常按次月汇总开票（票面开票日期
// 2025-11-xx），而用户脑子里筛的是「通行日期在 2025-10」。只比顶层日期会
// 把已入库的票整批筛掉，造成「明明上传过却查不到」。
func invoiceRecordDateInRange(r types.InvoiceRecord, from, to string) bool {
	if invoiceDateInRange(r.InvoiceDate, from, to) {
		return true
	}
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" && to == "" {
		return true
	}
	// 明细通行日期：取该明细自身起止区间与筛选区间的交集
	for _, it := range r.Items {
		if it.Extra == nil {
			continue
		}
		itemFrom := firstNonEmptyExtra(it.Extra, travelDateFromKeys...)
		itemTo := firstNonEmptyExtra(it.Extra, travelDateToKeys...)
		if itemFrom == "" && itemTo == "" {
			continue
		}
		if dateRangeOverlaps(itemFrom, itemTo, from, to) {
			return true
		}
	}
	return false
}

// travelDateFromKeys / travelDateToKeys 通行日期起止的同义键（后端 snake_case
// 与中文自定义键都要覆盖）。
var (
	travelDateFromKeys = []string{"travel_from", "travelfrom", "通行日期起", "入口日期", "上站日期"}
	travelDateToKeys   = []string{"travel_to", "travelto", "通行日期止", "出口日期", "下站日期"}
)

// firstNonEmptyExtra 按候选键顺序取 Extra 桶里第一个非空字符串值。
func firstNonEmptyExtra(extra map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := extra[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// dateRangeOverlaps 判断明细起止区间 [a1,a2] 与筛选区间 [b1,b2] 是否有交集。
// 日期统一去横线成 YYYYMMDD 后按字典序比较；明细侧缺一端时视为单点。
func dateRangeOverlaps(aFrom, aTo, bFrom, bTo string) bool {
	norm := func(s string) string {
		return strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	}
	a1, a2 := norm(aFrom), norm(aTo)
	b1, b2 := norm(bFrom), norm(bTo)
	if a1 == "" {
		a1 = a2
	}
	if a2 == "" {
		a2 = a1
	}
	if a1 > a2 {
		a1, a2 = a2, a1
	}
	if b1 == "" {
		b1 = b2
	}
	if b2 == "" {
		b2 = b1
	}
	if b1 > b2 {
		b1, b2 = b2, b1
	}
	if a1 == "" || b1 == "" {
		return false
	}
	// 区间有交集：a1 <= b2 且 b1 <= a2（任一端为空表示该端不限）
	return a1 <= b2 && (b1 == "" || b1 <= a2)
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
	extractFailedFiles := 0
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
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				continue
			}
			// 文件级提取失败数：供「历史记录」KPI 卡片副标题显示「N 份提取失败」。
			// 必须按**文件**计数（不是按发票记录），否则 0 张票的失败文件永远统计不到 ——
			// 恰恰是限流批量失败时最需要被看见的那一类。
			if strings.TrimSpace(meta.ExtractStatus) == "failed" {
				extractFailedFiles++
			}
			if meta.Kind != "invoice" {
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
						Name:      it.Name,
						Spec:      invoiceItemSpec(it),
						Unit:      it.Unit,
						Qty:       it.Qty,
						Price:     it.Price,
						Amount:    it.Amount,
						TaxRate:   it.TaxRate,
						TaxAmount: it.TaxAmt,
						Extra:     it.Extra,
					})
				}
				// 与 ListInvoiceRecords 同一条权威链（invoice_category > fleet_cert_type
				// > 票内 category），否则改名后概览卡按旧名聚合、与列表口径分叉。
				cat := meta.fileLevelInvoiceCategory()
				if cat == "" {
					cat = strings.TrimSpace(inv.Category)
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
					Category:      cat,
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
	stats.ExtractFailedFiles = extractFailedFiles

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

// invoiceItemSpec 取明细行规格型号：显式字段优先，历史数据兜底 Extra 桶
// （模型在 Spec 显式字段落地前输出过 spec 键，被收集进 Extra）。
func invoiceItemSpec(it InvoiceExtractionLine) string {
	if it.Spec != "" {
		return it.Spec
	}
	if it.Extra != nil {
		if v, ok := it.Extra["spec"].(string); ok {
			return v
		}
	}
	return ""
}
