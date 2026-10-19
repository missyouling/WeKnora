// 列表删除 ↔ 知识库源文件的级联一致性。
//
// 铁律：知识库源文件（knowledges）必须与发票列表数据 100% 对齐。用户在列表里
// 删除发票时，一档多票文件删到最后一张（invoices 被删空）必须**在同一流程内
// 彻底删除源文件**；绝不允许留下 invoices: [] 的空壳孤儿 —— 那种记录既在列表
// 里隐身（渲染不出任何行），又会以相同文件哈希把用户的重传永久挡在门外。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// pendingFileDeleteKey 标记「invoices 已删空、源文件待彻底删除」。
const pendingFileDeleteKey = "pending_file_delete"

// RemoveInvoicePage 从 custom_metadata.invoices 中移除一张发票（按页码），
// 并在删空时级联删除源文件。
//
// 返回 deletedFile 表示是否已触发/完成整份文件删除（调用方据此决定是否还要
// 入队异步清理 chunks / 向量 / 存储对象）；remaining 是删除后剩余的发票
// （页码已重排），删空时为空切片。
//
// 级联顺序（不可调换，实测踩坑）：
//  1. 先以**一次** UpdateKnowledgeColumns 同时写入 invoices=[] +
//     pending_file_delete + parse_status=deleting；
//  2. 再调用 repo.DeleteKnowledge 真正删行。
//
// 为什么必须分两步且第 1 步先落：第 1 步让「空壳」在库里不可能长期存在 ——
// parse_status=deleting 立刻解除 CheckKnowledgeExists 的哈希拦截（用户可立即
// 重传），pending_file_delete 让巡检兜底能识别并清理第 2 步执行失败的残留。
func (s *BusinessExtractService) RemoveInvoicePage(
	ctx context.Context, kbID, knowledgeID string, page int,
) (deletedFile bool, remaining []InvoiceExtractionItem, err error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	if tenantID == 0 {
		return false, nil, fmt.Errorf("tenant id missing in context")
	}
	if page < 1 {
		return false, nil, fmt.Errorf("page must be a positive integer")
	}
	knowledge, err := s.repo.GetKnowledgeByID(ctx, tenantID, knowledgeID)
	if err != nil {
		return false, nil, fmt.Errorf("knowledge not found: %w", err)
	}
	if knowledge.KnowledgeBaseID != kbID {
		return false, nil, fmt.Errorf("knowledge does not belong to the given knowledge base")
	}
	var meta invoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil ||
		meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		return false, nil, fmt.Errorf("no invoice extraction data to delete")
	}
	if page > len(meta.Invoices) {
		return false, nil, fmt.Errorf("page %d out of range (1-%d)", page, len(meta.Invoices))
	}

	// 目标下标：优先按发票自身页码命中，其次按文档内顺序
	targetIdx := -1
	for i, inv := range meta.Invoices {
		if i == page-1 || inv.Page == page {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		targetIdx = page - 1
	}

	removed := meta.Invoices[targetIdx]
	meta.Invoices = append(meta.Invoices[:targetIdx], meta.Invoices[targetIdx+1:]...)
	// 页码前移重排，保持 page = 文档内出现顺序
	for i := range meta.Invoices {
		meta.Invoices[i].Page = i + 1
	}

	if len(meta.Invoices) > 0 {
		// 仍有剩余发票：只更新 invoices，绝不动源文件
		raw, jerr := json.Marshal(meta)
		if jerr != nil {
			return false, nil, fmt.Errorf("encode invoice metadata after delete: %w", jerr)
		}
		if err := s.repo.UpdateKnowledgeColumn(ctx, knowledgeID, "custom_metadata", types.JSON(raw)); err != nil {
			return false, nil, fmt.Errorf("persist invoice metadata after delete: %w", err)
		}
		return false, meta.Invoices, nil
	}

	// —— 删空：级联删除源文件 ——
	// 第 1 步（一次落两列，避免半更新状态）
	var rawMeta map[string]any
	if err := json.Unmarshal(knowledge.CustomMetadata, &rawMeta); err != nil || rawMeta == nil {
		rawMeta = map[string]any{}
	}
	rawMeta["invoices"] = []interface{}{}
	rawMeta[pendingFileDeleteKey] = true
	rawMeta["pending_file_delete_at"] = time.Now().Format(time.RFC3339)
	if removed.InvoiceNo != "" {
		rawMeta["last_deleted_invoice_no"] = removed.InvoiceNo
	}
	encoded, jerr := json.Marshal(rawMeta)
	if jerr != nil {
		return false, nil, fmt.Errorf("encode invoice metadata for cascade delete: %w", jerr)
	}
	if err := s.repo.UpdateKnowledgeColumns(ctx, knowledgeID, map[string]interface{}{
		"custom_metadata": types.JSON(encoded),
		"parse_status":    "deleting",
	}); err != nil {
		return false, nil, fmt.Errorf("mark knowledge deleting before cascade: %w", err)
	}

	// 第 2 步：真正删行。失败时抛错让调用方感知，但状态已降级为可恢复
	if err := s.repo.DeleteKnowledge(ctx, tenantID, knowledgeID); err != nil {
		return false, nil, fmt.Errorf("cascade delete source file: %w", err)
	}
	return true, nil, nil
}

// invoiceCustomMetadata 与 handler 层结构对齐（删除路径只需 kind/invoices）。
type invoiceCustomMetadata struct {
	Kind          string                  `json:"kind"`
	Invoices      []InvoiceExtractionItem `json:"invoices"`
	ExtractStatus string                  `json:"extract_status"`
	ExtractError  string                  `json:"extract_error"`
}

// invoicePendingFileDelete 判断一条 metadata 是否处于「已删空待清理」状态。
func invoicePendingFileDelete(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m[pendingFileDeleteKey]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// SweepStalePendingFileDeletes 巡检某个知识库，返回超时未完成的「删空待清理」记录 ID。
//
// 幂等：只读不改。handler/脚本据此决定彻底删除或告警。只认三个条件同时成立：
//  1. custom_metadata.pending_file_delete == true（删空时打的标记）；
//  2. parse_status == 'deleting'（确实在删除流程里，排除误标）；
//  3. 距 updated_at 超过 maxAge（给 worker 留足执行窗口）。
//
// 正常发票文件、单纯的 invoices 空数组、删除中的其它记录都不会入选。
func (s *BusinessExtractService) SweepStalePendingFileDeletes(
	ctx context.Context, kbID string, maxAge time.Duration,
) ([]string, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant id missing in context")
	}
	var staleIDs []string
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(
			ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for pending-delete sweep: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || k.ParseStatus != "deleting" {
				continue
			}
			if !invoicePendingFileDelete(k.CustomMetadata) {
				continue
			}
			last := k.UpdatedAt
			if last.IsZero() {
				last = k.CreatedAt
			}
			if time.Since(last) > maxAge {
				staleIDs = append(staleIDs, k.ID)
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return staleIDs, nil
}

// InvoicePlaceholderSummary 汇总某知识库的源文件 ↔ 发票行对齐状况，供巡检脚本直接打印。
type InvoicePlaceholderSummary struct {
	KBID            string `json:"kb_id"`
	TotalFiles      int    `json:"total_files"`
	InvoiceFiles    int    `json:"invoice_files"`
	PlaceholderRows int    `json:"placeholder_rows"`
	PendingDeletes  int    `json:"pending_deletes"`
	// NotInvoiceFiles 被判定为非票据 / 无 metadata 的文件数（同样以占位行展示）
	NotInvoiceFiles int `json:"not_invoice_files"`
	// InProgressFiles 解析/提取中的文件（由顶部状态条展示，不计为孤儿）
	InProgressFiles int      `json:"in_progress_files"`
	OrphanKnowledge []string `json:"orphan_knowledge"`
}

// SummarizeInvoiceAlignment 扫描知识库，找出所有「在列表里看不到但文件还在」的
// 孤儿（提取失败 / 未提取出票据 / 非票据 / 删空待清理），用于存量自愈巡检。
//
// 统计口径必须与 ListInvoiceRecords 的行展开逻辑一致：列表里凡是渲染成占位行
// 的，这里就该出现在 orphan_knowledge 中（否则巡检看不见问题）。
func (s *BusinessExtractService) SummarizeInvoiceAlignment(ctx context.Context, kbID string) (*InvoicePlaceholderSummary, error) {
	tenantID, _ := ctx.Value(types.TenantIDContextKey).(uint64)
	if tenantID == 0 {
		return nil, fmt.Errorf("tenant id missing in context")
	}
	out := &InvoicePlaceholderSummary{KBID: kbID, OrphanKnowledge: []string{}}
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(
			ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return nil, fmt.Errorf("list knowledge for alignment summary: %w", err)
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil {
				continue
			}
			out.TotalFiles++
			// 解析/提取中：由状态条展示，不算孤儿
			if k.ParseStatus == "pending" || k.ParseStatus == "processing" || k.ParseStatus == "finalizing" {
				out.InProgressFiles++
				continue
			}
			if k.ParseStatus == "deleting" && invoicePendingFileDelete(k.CustomMetadata) {
				out.PendingDeletes++
				out.OrphanKnowledge = append(out.OrphanKnowledge, k.ID)
				continue
			}
			if len(k.CustomMetadata) == 0 {
				out.NotInvoiceFiles++
				out.OrphanKnowledge = append(out.OrphanKnowledge, k.ID)
				continue
			}
			var meta invoiceMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil {
				out.NotInvoiceFiles++
				out.OrphanKnowledge = append(out.OrphanKnowledge, k.ID)
				continue
			}
			legacyManual := meta.Kind != "invoice" && meta.AutoDeletedCount > 0
			if meta.Kind != "invoice" && !legacyManual {
				out.NotInvoiceFiles++
				out.OrphanKnowledge = append(out.OrphanKnowledge, k.ID)
				continue
			}
			out.InvoiceFiles++
			if invoiceItemsAllBlank(meta.Invoices) {
				out.PlaceholderRows++
				out.OrphanKnowledge = append(out.OrphanKnowledge, k.ID)
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return out, nil
}
