// 存量发票「明细串页 + 税率错标」修复（第 27 轮）。
//
// 事故形态（真机 knowledge c9941d0c，19 页 19 票的通行费汇总单）：
//   - 第 2 张票票面 198.09 / 9% / 17.83，明细却是第 14 张票的 6.45 / 3% / 0.19；
//   - 第 1 张票票面 68.82，明细是第 12 张票的 2.62。
//
// 根因在 `OverlayDeterministicInvoiceFields` 把全文明细行按下标回填（已修为按页配对）。
// 本文件负责把**已经落库的错值**刷正：
//  1. 用每张票自己那一页的原文重建明细（页内唯一数据行才重建）；
//  2. 重算票级/明细税率与价税合计恒等式；
//  3. 无法闭环的记录追加告警，交给人工核对。
//
// 为什么用「启动幂等修复」而不是重跑提取：重提会烧 token、耗时十几分钟，还会
// 覆盖用户已手工核对过的字段（分类/开票人/备注）。这里的修复只动 items 与
// amount/tax/tax_rate/total_amount，其余键逐字节保留。
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// InvoiceReconcileStats 是一次存量修复的统计结果（同时写日志，便于在
// data/server_vm.log 里核对「到底改了多少」）。
type InvoiceReconcileStats struct {
	Scanned     int `json:"scanned"`      // 含发票的文件数
	Candidates  int `json:"candidates"`   // 命中「明细/金额可疑」的文件数
	Repaired    int `json:"repaired"`     // 实际写库的文件数
	ItemRebuilt int `json:"item_rebuilt"` // 重建明细的发票数
	RateFixed   int `json:"rate_fixed"`   // 税率被纠偏的发票数
	TotalFixed  int `json:"total_fixed"`  // 价税合计被补齐/重算的发票数
	Unresolved  int `json:"unresolved"`   // 仍无法闭环、已加告警的发票数
}

// RepairInvoiceItemsAndFinancials 扫描全库发票，修复串页明细与错标税率。
//
// 幂等：修好之后再跑，candidates 会归零（除「无法闭环」的记录，它们只加告警
// 不重复写库）。只对**元数据层面就能判定可疑**的文件加载 chunks，避免启动期
// 为 1600+ 文件全量读 chunk 表。
func (h *BusinessExtractHandler) RepairInvoiceItemsAndFinancials(ctx context.Context) (InvoiceReconcileStats, error) {
	var stats InvoiceReconcileStats
	if h == nil || h.db == nil {
		return stats, nil
	}
	var rows []types.Knowledge
	if err := h.db.WithContext(ctx).
		Where("CAST(custom_metadata AS TEXT) LIKE ?", "%\"invoices\"%").
		Find(&rows).Error; err != nil {
		return stats, fmt.Errorf("scan knowledges: %w", err)
	}
	for i := range rows {
		row := rows[i]
		if len(row.CustomMetadata) == 0 {
			continue
		}
		var meta service.InvoiceCustomMetadata
		if err := json.Unmarshal(row.CustomMetadata, &meta); err != nil {
			continue
		}
		if meta.Kind != "" && meta.Kind != "invoice" {
			continue
		}
		if len(meta.Invoices) == 0 {
			continue
		}
		stats.Scanned++
		// 日期格式归一是纯元数据操作（幂等）：历史上 HTML 规则落「20261031」、
		// 扁平规则落「2026-10-31」，同一列两种格式，筛选/排序都认不出来。
		dateFixed := service.NormalizeInvoiceItemDates(meta.Invoices)
		if !invoiceMetaNeedsReconcile(meta.Invoices) && dateFixed == 0 {
			continue
		}
		stats.Candidates++
		rebuilt, fixes, issues, changed := h.reconcileOneInvoiceFile(ctx, row, &meta)
		if dateFixed > 0 {
			changed = true
		}
		if !changed {
			continue
		}
		if err := h.persistInvoiceReconcile(ctx, row, meta); err != nil {
			logger.Warnf(ctx, "invoice reconcile: persist knowledge %s (%s) failed: %v", row.ID, row.FileName, err)
			continue
		}
		stats.Repaired++
		stats.ItemRebuilt += rebuilt
		fileRateFixed := 0
		for _, f := range fixes {
			if f.RateAfter != nil {
				stats.RateFixed++
				fileRateFixed++
			}
			if f.TotalFilled || f.AmountRebased {
				stats.TotalFixed++
			}
		}
		stats.Unresolved += len(issues)
		logger.Infof(ctx,
			"invoice reconcile: knowledge %s (%s) items_rebuilt=%d rate_fixed=%d unresolved=%d invoices=%d",
			row.ID, row.FileName, rebuilt, fileRateFixed, len(issues), len(meta.Invoices))
	}
	logger.Infof(ctx,
		"invoice reconcile summary: scanned=%d candidates=%d repaired=%d items_rebuilt=%d rate_fixed=%d total_fixed=%d unresolved=%d",
		stats.Scanned, stats.Candidates, stats.Repaired, stats.ItemRebuilt,
		stats.RateFixed, stats.TotalFixed, stats.Unresolved)
	return stats, nil
}

// invoiceMetaNeedsReconcile 只用元数据判断是否值得加载 chunks。
//
// 三类可疑：
//  1. 单行明细串到了另一张票的数据（金额与另一张票吻合，见 CrossPageSuspect）；
//  2. 税率解释不了「金额 × 税率 vs 税额」（错标档位）；
//  3. 金额 + 税额 ≠ 价税合计（三件套不自洽）。
//
// 注意不要用「明细合计 != 票面」当判据：折扣票/红冲票/合计行天然不等，
// 全库跑一遍会给几百张正常票加告警（真机实测 29 份误报）。
func invoiceMetaNeedsReconcile(invs []service.InvoiceExtractionItem) bool {
	for i := range invs {
		inv := &invs[i]
		if service.InvoiceItemsCrossPageSuspect(invs, i) {
			return true
		}
		// 通行费明细的车牌/通行日期起止缺项：按页内原文能补齐（规则已修，存量走这里）。
		if service.InvoiceItemExtrasIncomplete(inv) {
			return true
		}
		if service.InvoiceTollRemarkStale(inv) {
			return true
		}
		if inv.Amount != nil && inv.Tax != nil && *inv.Amount > 0 {
			if inv.TaxRate != nil && math.Abs(*inv.Amount**inv.TaxRate-*inv.Tax) > 0.05 {
				return true
			}
			if inv.TotalAmount != nil &&
				math.Abs(*inv.Amount+*inv.Tax-*inv.TotalAmount) > 0.02 {
				return true
			}
		}
	}
	return false
}

// reconcileOneInvoiceFile 加载该文件的 chunks，按页重建明细并做四元强校验。
// 返回（重建明细数，逐票纠偏结果，未闭环告警，是否发生改动）。
func (h *BusinessExtractHandler) reconcileOneInvoiceFile(
	ctx context.Context, row types.Knowledge, meta *service.InvoiceCustomMetadata,
) (int, []service.InvoiceFinancialFix, []service.ExtractFieldIssue, bool) {
	// chunk 服务按 ctx 里的租户过滤：后台任务没有请求上下文，必须显式补。
	tenantCtx := context.WithValue(ctx, types.TenantIDContextKey, row.TenantID)
	rebuilt := 0
	if chunks, err := h.chunkService.ListChunksByKnowledgeIDAndTypes(
		tenantCtx, row.ID, service.ExtractSourceChunkTypes()); err == nil && len(chunks) > 0 {
		src := service.BuildExtractSourceText(chunks)
		rebuilt = service.ReconcileInvoiceItemsWithSource(meta.Invoices, src)
	} else if err != nil {
		logger.Warnf(ctx, "invoice reconcile: load chunks for %s failed: %v", row.ID, err)
	}
	fixes := make([]service.InvoiceFinancialFix, len(meta.Invoices))
	for i := range meta.Invoices {
		fixes[i] = service.ReconcileInvoiceFinancials(&meta.Invoices[i])
	}
	issues := service.ReconcileInvoiceFinancialsForDocument(meta.Invoices)
	// 明细仍串页（页内数据行有歧义 / 无 chunks 可重建）→ 逐票告警。
	// 只对「能证明来自另一张票」的记录告警，避免折扣票/红冲票被批量误标。
	for i := range meta.Invoices {
		if service.InvoiceItemsCrossPageSuspect(meta.Invoices, i) {
			issues = append(issues, service.NewExtractInvoiceItemsMismatchIssue(i))
		}
	}
	// 先剔除上一轮遗留的「明细不一致」告警，再按本次结论追加，避免越堆越多。
	prevItemIssues := countItemIssues(meta.ExtractIssues)
	meta.ExtractIssues = dropStaleItemIssues(meta.ExtractIssues)
	meta.ExtractIssues = append(meta.ExtractIssues, issues...)

	changed := rebuilt > 0 || len(issues) != prevItemIssues
	if !changed {
		for _, f := range fixes {
			if f.Changed() {
				changed = true
				break
			}
		}
	}
	if !changed {
		return 0, fixes, issues, false
	}
	return rebuilt, fixes, issues, true
}

// countItemIssues 统计 items 字段的历史告警条数（用于判断本次是否真的新增/减少）。
func countItemIssues(issues []service.ExtractFieldIssue) int {
	n := 0
	for _, it := range issues {
		field := strings.ToLower(strings.TrimSpace(it.Field))
		if field == "items" || (field == "amount" && strings.Contains(it.Detail, "请核对")) {
			n++
		}
	}
	return n
}

// dropStaleItemIssues 删除「本修复自己产出」的历史告警（每轮按当前数据重算）。
// 只删 items 字段与 amount 字段里 Detail 带「请核对」的项，别的校验器产出的
// 告警一律保留，避免误删人工/其它规则的有效提示。
func dropStaleItemIssues(issues []service.ExtractFieldIssue) []service.ExtractFieldIssue {
	if len(issues) == 0 {
		return issues
	}
	out := issues[:0]
	for _, it := range issues {
		field := strings.ToLower(strings.TrimSpace(it.Field))
		if field == "items" {
			continue
		}
		if field == "amount" && strings.Contains(it.Detail, "请核对") {
			continue
		}
		out = append(out, it)
	}
	return out
}

// persistInvoiceReconcile 把修好的 invoices/extract_issues 写回 custom_metadata，
// 其余顶层键（kind/分类/开票人/auto_deleted_count 等）逐个保留。
func (h *BusinessExtractHandler) persistInvoiceReconcile(
	ctx context.Context, row types.Knowledge, meta service.InvoiceCustomMetadata,
) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(row.CustomMetadata, &root); err != nil || root == nil {
		return fmt.Errorf("decode custom_metadata: %w", err)
	}
	// 逐票 merge，而不是整数组重写：历史记录里可能带当前结构体没有的键
	// （早期版本的自定义字段），整数组 Marshal 会把它们静默丢掉。
	var existing []map[string]json.RawMessage
	if raw, ok := root["invoices"]; ok {
		if jerr := json.Unmarshal(raw, &existing); jerr != nil {
			existing = nil
		}
	}
	if len(existing) != len(meta.Invoices) {
		existing = nil
	}
	if existing == nil {
		invRaw, err := json.Marshal(meta.Invoices)
		if err != nil {
			return err
		}
		root["invoices"] = invRaw
	} else {
		for i := range meta.Invoices {
			fresh, err := json.Marshal(meta.Invoices[i])
			if err != nil {
				return err
			}
			var fm map[string]json.RawMessage
			if jerr := json.Unmarshal(fresh, &fm); jerr != nil {
				continue
			}
			for k, v := range fm {
				existing[i][k] = v
			}
		}
		invRaw, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		root["invoices"] = invRaw
	}
	if len(meta.ExtractIssues) > 0 {
		issueRaw, ierr := json.Marshal(meta.ExtractIssues)
		if ierr == nil {
			root["extract_issues"] = issueRaw
		}
	} else {
		delete(root, "extract_issues")
	}
	out, err := json.Marshal(root)
	if err != nil {
		return err
	}
	return h.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id = ?", row.ID).
		Update("custom_metadata", types.JSON(out)).Error
}
