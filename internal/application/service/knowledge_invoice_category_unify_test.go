package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归（2026-10-20）：分类分桶错乱。
//
// 事故：真机「其它票据（201 张）」里混进大量通行费票——表格「发票类型」列写着
// `通行费发票`，记录却落在「其它票据」桶里。根因是构建 InvoiceRecord 时
// Category 与 InvoiceType 取自不同层级：文件级 meta.InvoiceCategory（如历史脏数据
// `水电发票`/空）决定分桶，票内 inv.InvoiceType（`通行费发票`）决定表格列显示，
// 二者不一致就出现「列里写着通行费、桶在其它票据」的分裂。
//
// 修复契约（列表与概览同一口径）：候选
//   [meta.InvoiceCategory, meta.FleetCertType, inv.InvoiceType, inv.Category]
// 取第一个命中「已启用分类名」的值，**同时赋给 Category 与 InvoiceType**，
// 保证「表格发票类型列显示什么，它就100%属于哪个分类卡片桶」。
// 「其它票据」是排除式桶而非真实分类，不作为命中目标。

// catCtxV1 tenant 上下文（与一致性测试同口径）。
func catCtxV1() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

// enabledCatsV1 启用分类集合（不含「其它票据」这个排除式桶）。
func enabledCatsV1() []string {
	return []string{"通行费发票", "能耗发票", "专用发票", "普通发票"}
}

// splitCatMeta 造一份「文件级分类与票内类型不一致」的 metadata：
// 文件级写死旧名（赃数据），票内 invoice_type/category 是真实类型。
// invNo 必须每份文件唯一，否则 dedupInvoiceRecords 会按票号合并（与真实数据形态一致）。
func splitCatMeta(invNo, fileIC, fileFCT, invType string) map[string]interface{} {
	return map[string]interface{}{
		"kind":             "invoice",
		"extract_status":   "success",
		"invoice_category": fileIC,
		"fleet_cert_type":  fileFCT,
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": invNo, "invoice_type": invType, "category": invType,
				"total_amount": 100,
			},
		},
	}
}

// TestBusinessInvoiceCategoryFirstHitUnifiesBucket 主回归：
// 文件级是失效旧名（水电发票）、票内是通行费发票 → 必须解析为通行费发票，
// 且 Category 与 InvoiceType 完全同步，绝不再出现「列显示通行费、桶在其它票据」。
func TestBusinessInvoiceCategoryFirstHitUnifiesBucket(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{
			ID: "k_split", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, splitCatMeta("INV-A", "水电发票", "通行费发票", "通行费发票"))),
		},
	}}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		Page: 1, PageSize: 20,
		EnabledCategories: enabledCatsV1(),
	})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	r := res.Data[0]
	assert.Equal(t, "通行费发票", r.Category, "文件级旧名已失效，必须回落到票内命中的启用分类")
	assert.Equal(t, "通行费发票", r.InvoiceType, "Category 与 InvoiceType 必须 100% 同步")

	// 按该分类筛选必须命中（列表口径 = 卡片桶口径）
	hit, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		InvoiceType: "通行费发票", Page: 1, PageSize: 20,
		EnabledCategories: enabledCatsV1(),
	})
	require.NoError(t, err)
	assert.Len(t, hit.Data, 1, "通行费发票卡片必须能筛到它")

	// 概览统计同口径：by_category 记在通行费发票下，不得出现"未分类"
	st, err := svc.InvoiceOverviewStats(catCtxV1(), "kb1", enabledCatsV1())
	require.NoError(t, err)
	byCat := map[string]int{}
	for _, c := range st.ByCategory {
		byCat[c.Category] = c.Count
	}
	assert.Equal(t, 1, byCat["通行费发票"], "概览必须按通行费发票聚合")
	assert.NotContains(t, byCat, "未分类", "命中启用分类后不得再落「未分类」")
}

// TestBusinessInvoiceCategoryFileLevelStillWins 文件级权威键优先：
// 文件级就是启用分类名时，仍然以文件级为准（用户上传时选定的分类是权威来源）。
func TestBusinessInvoiceCategoryFileLevelStillWins(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{
			ID: "k_file", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, splitCatMeta("INV-B", "能耗发票", "能耗发票", "通行费发票"))),
		},
	}}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		Page: 1, PageSize: 20,
		EnabledCategories: enabledCatsV1(),
	})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "能耗发票", res.Data[0].Category, "文件级命中启用分类时权威优先")
	assert.Equal(t, "能耗发票", res.Data[0].InvoiceType, "列与桶同步为能耗发票")
}

// TestBusinessInvoiceCategoryNoHitFallsBack 没有任何候选命中启用分类时，
// 保持原有兜底（文件级链 → 票内 category），记录落入排除式「其它票据」桶。
func TestBusinessInvoiceCategoryNoHitFallsBack(t *testing.T) {
	meta := map[string]interface{}{
		"kind": "invoice", "extract_status": "success",
		// 分类被停用/删除后的存量旧值，不是任何启用分类名
		"invoice_category": "水费发票",
		"invoices": []interface{}{
			map[string]interface{}{"invoice_no": "INV-X", "invoice_type": "其它票据", "category": "", "total_amount": 5},
		},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_none", KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(t, meta))},
	}}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		Page: 1, PageSize: 20,
		EnabledCategories: enabledCatsV1(),
	})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "水费发票", res.Data[0].Category, "无命中时保持文件级兜底")
	// 该记录不属于任何启用分类 → 归入「其它票据」排除桶
	hit, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		InvoiceType: "其它票据", Page: 1, PageSize: 20,
		EnabledCategories: enabledCatsV1(),
		KnownCategories:   enabledCatsV1(),
	})
	require.NoError(t, err)
	found := false
	for _, r := range hit.Data {
		if r.InvoiceNo == "INV-X" {
			found = true
		}
	}
	assert.True(t, found, "未命中启用分类的记录必须归入「其它票据」排除桶")
}

// TestBusinessInvoiceOverviewCategoryMatchesList 概览与列表的分类归属必须一致：
// 同一份数据下，列表按分类筛到的记录数 == 概览 by_category 中该分类的张数。
func TestBusinessInvoiceOverviewCategoryMatchesList(t *testing.T) {
	items := []*types.Knowledge{
		{ID: "k1", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, splitCatMeta("INV-A", "水电发票", "通行费发票", "通行费发票")))},
		{ID: "k2", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, splitCatMeta("INV-C", "通行费发票", "通行费发票", "通行费发票")))},
		{ID: "k3", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: types.JSON(mustJSON(t, splitCatMeta("INV-D", "能耗发票", "能耗发票", "能耗发票")))},
	}
	svc := &BusinessExtractService{repo: &taxRateFakeRepo{items: items}}

	listRes, err := svc.ListInvoiceRecords(catCtxV1(), "kb1", types.InvoiceListFilter{
		InvoiceType: "通行费发票", Page: 1, PageSize: 100,
		EnabledCategories: enabledCatsV1(),
	})
	require.NoError(t, err)

	st, err := svc.InvoiceOverviewStats(catCtxV1(), "kb1", enabledCatsV1())
	require.NoError(t, err)
	byCat := map[string]int{}
	for _, c := range st.ByCategory {
		byCat[c.Category] = c.Count
	}
	assert.Equal(t, len(listRes.Data), byCat["通行费发票"],
		"列表筛出的张数必须等于概览该分类的张数（两条读径同口径）")
	assert.Equal(t, 1, byCat["能耗发票"])
}
