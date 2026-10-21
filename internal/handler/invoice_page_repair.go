package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 存量数据修复（2026-10-21）。
//
// 背景：6 页 6 票的 PDF 在列表里显示 P1、P2、P3、P4、P5、P5 —— 第 6 张票的
// page_no 被写成 5。根因在拼装层：一个 chunk 跨了第 5、6 页（chunk 15 的
// source_locators = [5,6,6,6,6,6,6]），旧实现只在 chunk 起点按第一个非零页
// 打 [Page 5]，整个 chunk（含第 6 页票头与票号）都被算作第 5 页。
//
// 算法已在 BuildExtractSourceText / AssignPhysicalPages 修复。本文件负责把
// **已经落库的错值**刷正：只重算页码，不重跑模型（重提会烧 token 且可能改动
// 其它已核对过的字段）。
//
// 为什么不是「全库重算」：重算需要为每个文件加载 chunks，133 个多票文件全量
// 加载在启动期代价过高。这里先用 metadata 就能判定的**错值特征**筛候选：
//   - 尾票与前一票同页，且前 N-1 张严格按 +1 递增（6 页 6 票的签名）；
//   - 页码非单调递增（后一张票页码小于前一张）。
//
// 两者都只可能由页码分配 bug 产生，正常数据不会命中。

// invoicePageRow 是修补用的最小发票信息（只读 page_no / invoice_no）。
type invoicePageRow struct {
	PageNo    int    `json:"page_no"`
	InvoiceNo string `json:"invoice_no"`
}

// invoicePagesSuspicious 判断一组已落库页码是否明显错误。
func invoicePagesSuspicious(pages []int) bool {
	n := len(pages)
	if n < 2 {
		return false
	}
	for _, p := range pages {
		if p <= 0 {
			return false // 缺页码的走前端序号兜底，不在此处猜
		}
	}
	// 非单调（后票页码更小）：无论如何都是错的。
	for i := 1; i < n; i++ {
		if pages[i] < pages[i-1] {
			return true
		}
	}
	// 尾票与前一票同页，且前 N-1 张严格 +1：典型的「跨块合并把尾票留在上一页」。
	if pages[n-1] != pages[n-2] {
		return false
	}
	for i := 1; i <= n-2; i++ {
		if pages[i] != pages[i-1]+1 {
			return false
		}
	}
	return true
}

// RepairInvoicePhysicalPages 重算存量发票的物理页码并落库。
//
// 返回 (扫描的多票文件数, 实际改写的文件数)。幂等：修好之后再跑没有任何写库。
func (h *BusinessExtractHandler) RepairInvoicePhysicalPages(ctx context.Context) (int, int) {
	if h == nil || h.db == nil {
		return 0, 0
	}
	var rows []types.Knowledge
	if err := h.db.WithContext(ctx).
		Where("CAST(custom_metadata AS TEXT) LIKE ?", "%\"invoices\"%").
		Find(&rows).Error; err != nil {
		logger.Warnf(ctx, "invoice page repair: scan knowledges failed: %v", err)
		return 0, 0
	}

	scanned, repaired := 0, 0
	for i := range rows {
		row := rows[i]
		var meta struct {
			Invoices []invoicePageRow `json:"invoices"`
		}
		if len(row.CustomMetadata) == 0 {
			continue
		}
		if err := json.Unmarshal(row.CustomMetadata, &meta); err != nil {
			continue
		}
		if len(meta.Invoices) < 2 {
			continue
		}
		scanned++
		pages := make([]int, len(meta.Invoices))
		for j := range meta.Invoices {
			pages[j] = meta.Invoices[j].PageNo
		}
		if !invoicePagesSuspicious(pages) {
			continue
		}
		fixed, ok := h.recomputeInvoicePages(ctx, row, meta.Invoices)
		if !ok {
			continue
		}
		if err := h.persistInvoicePageNos(ctx, row, fixed); err != nil {
			logger.Warnf(ctx, "invoice page repair: persist knowledge %s failed: %v", row.ID, err)
			continue
		}
		repaired++
		logger.Infof(ctx, "invoice page repair: knowledge %s (%s) pages %v -> %v", row.ID, row.FileName, pages, fixed)
	}
	if repaired > 0 {
		logger.Infof(ctx, "invoice page repair: scanned=%d repaired=%d", scanned, repaired)
	}
	return scanned, repaired
}

// recomputeInvoicePages 用源 chunk 重跑页码分配（新算法），返回修正后的页码。
// ok=false 表示没有可用依据（无 chunk / 无票号），此时不得臆改。
func (h *BusinessExtractHandler) recomputeInvoicePages(
	ctx context.Context, row types.Knowledge, stored []invoicePageRow,
) ([]int, bool) {
	// chunk 服务按 ctx 里的租户过滤：启动期没有请求上下文，必须显式补上。
	ctx = context.WithValue(ctx, types.TenantIDContextKey, row.TenantID)
	chunks, err := h.chunkService.ListChunksByKnowledgeIDAndTypes(ctx, row.ID, service.ExtractSourceChunkTypes())
	if err != nil || len(chunks) == 0 {
		return nil, false
	}
	src := service.BuildExtractSourceText(chunks)
	if !src.HasPages() || strings.TrimSpace(src.Content) == "" {
		return nil, false
	}
	invs := make([]service.InvoiceExtractionItem, len(stored))
	for i := range stored {
		invs[i].InvoiceNo = stored[i].InvoiceNo
		invs[i].PageNo = stored[i].PageNo
	}
	if service.AssignPhysicalPages(src, invs) == 0 {
		return nil, false
	}
	service.FixDuplicateTailPage(src, invs)

	out := make([]int, len(invs))
	changed := false
	for i := range invs {
		if invs[i].PageNo <= 0 {
			return nil, false
		}
		out[i] = invs[i].PageNo
		if out[i] != stored[i].PageNo {
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	return out, true
}

// persistInvoicePageNos 只改写 invoices[].page_no，其余键（票号/金额/明细/
// 用户自定义 fields）逐字节保留 —— 页码修复绝不能让已核对过的字段发生变化。
func (h *BusinessExtractHandler) persistInvoicePageNos(ctx context.Context, row types.Knowledge, pages []int) error {
	updated, ok := setInvoicePageNos(row.CustomMetadata, pages)
	if !ok {
		return nil
	}
	return h.db.WithContext(ctx).Model(&types.Knowledge{}).
		Where("id = ?", row.ID).
		Update("custom_metadata", updated).Error
}

// setInvoicePageNos 把 pages 写回 custom_metadata.invoices[i].page_no。
// 返回是否发生改动。数组长度不一致时不做任何改写（宁可不动，也不能错位）。
func setInvoicePageNos(raw types.JSON, pages []int) (types.JSON, bool) {
	if len(raw) == 0 || len(pages) == 0 {
		return raw, false
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return raw, false
	}
	invoicesRaw, exists := root["invoices"]
	if !exists {
		return raw, false
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(invoicesRaw, &items); err != nil || len(items) != len(pages) {
		return raw, false
	}
	changed := false
	for i := range items {
		if pages[i] <= 0 {
			continue
		}
		cur := 0
		if v, ok := items[i]["page_no"]; ok {
			_ = json.Unmarshal(v, &cur)
		}
		if cur == pages[i] {
			continue
		}
		buf, err := json.Marshal(pages[i])
		if err != nil {
			return raw, false
		}
		items[i]["page_no"] = buf
		changed = true
	}
	if !changed {
		return raw, false
	}
	invNew, err := json.Marshal(items)
	if err != nil {
		return raw, false
	}
	root["invoices"] = invNew
	out, err := json.Marshal(root)
	if err != nil {
		return raw, false
	}
	return types.JSON(out), true
}

// CleanupOrphanExtractConfigs 归档「分类已不存在」的提取配置。
//
// 背景：knowledge_extract_configs 按 (scope, cert_type=分类名) 建键，而删除
// 分类历史上只软删 fleet_categories 一行，配置表从不联动 —— 每删一次分类就
// 留一条孤儿（真机 KB 0049ce9e 的「服务合同的副本1」）。同名分类日后重建时
// 会直接读到上一轮的旧规则，用户以为新建的分类「自带」这些字段。
//
// 只处理四类业务 scope（invoice/contract/regulation/award_punish）且 cert_type
// 非空的配置：cert_type 为空的是 KB 级整体规则，不属于任何分类，必须保留；
// 车队 scope（vehicle/driver/maintain）的 cert_type 对应 fleet_cert_groups，
// 不在本修复范围。软删而非物理删，可回滚。
func (h *BusinessExtractHandler) CleanupOrphanExtractConfigs(ctx context.Context) int {
	if h == nil || h.db == nil {
		return 0
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).Select("scope", "name").
		Where("deleted_at IS NULL").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "orphan extract config cleanup: list categories failed: %v", err)
		return 0
	}
	alive := make(map[string]struct{}, len(cats))
	for _, c := range cats {
		alive[extractConfigScopeForCategory(c.Scope)+"\x00"+strings.TrimSpace(c.Name)] = struct{}{}
	}

	var cfgs []types.KbExtractConfig
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND cert_type <> '' AND scope IN ?",
			[]string{"invoice", "contract", "regulation", "award_punish"}).
		Find(&cfgs).Error; err != nil {
		logger.Warnf(ctx, "orphan extract config cleanup: list configs failed: %v", err)
		return 0
	}
	cleaned := 0
	for i := range cfgs {
		key := cfgs[i].Scope + "\x00" + strings.TrimSpace(cfgs[i].CertType)
		if _, ok := alive[key]; ok {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.KbExtractConfig{}).
			Where("id = ? AND deleted_at IS NULL", cfgs[i].ID).
			Update("deleted_at", timeNowUTC()).Error; err != nil {
			logger.Warnf(ctx, "orphan extract config cleanup: archive %s (%s/%s) failed: %v",
				cfgs[i].ID, cfgs[i].Scope, cfgs[i].CertType, err)
			continue
		}
		cleaned++
		logger.Infof(ctx, "orphan extract config archived: id=%s kb=%s scope=%s cert_type=%q",
			cfgs[i].ID, cfgs[i].KnowledgeBaseID, cfgs[i].Scope, cfgs[i].CertType)
	}
	if cleaned > 0 {
		logger.Infof(ctx, "orphan extract config cleanup: archived %d config(s)", cleaned)
	}
	return cleaned
}
