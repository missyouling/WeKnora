package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归（2026-10-20）：发票分类改名后，分类取值必须走**权威链**。
//
// 背景：权威链定义在 knowledge_invoice_category_lock.go（
// requestCert > invoice_category > fleet_cert_type）。但 ListInvoiceRecords
// 的 invoiceMetadata 只声明了 fleet_cert_type，没声明 invoice_category ——
// 导致只带/只更新了 invoice_category 的文件在列表里落到旧分类名下（真机存在
// 3 份 invoice_category=水电发票 / fleet_cert_type=通行费发票 的不一致存量行）。
//
// 本测试锁定的新契约：文件级分类按 invoice_category 优先、fleet_cert_type
// 兜底、最后回退票内 category；改名后端已把 4 处存量键整体改写，此处保证
// 读取侧不会被任何一处残留的旧名牵着走。

// listCtxV1 与现有一致性测试同口径的 tenant 上下文。
func listCtxV1() context.Context {
	return context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
}

// renameAwareMeta 造一份「改名后」形态的 metadata：invoice_category 已是新名，
// fleet_cert_type 仍是旧名（模拟传播只完成一半或历史不一致存量行）。
func renameAwareMeta(newCat, staleCat string) map[string]interface{} {
	return map[string]interface{}{
		"kind":             "invoice",
		"extract_status":   "success",
		"invoice_category": newCat,
		"fleet_cert_type":  staleCat,
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": "INV-001", "invoice_date": "2025-10-09",
				"invoice_type": staleCat, "category": staleCat,
				"total_amount": 88.5,
			},
		},
	}
}

// TestBusinessInvoiceListCategoryUsesAuthorityChain：文件级 invoice_category
// 是权威源，fleet_cert_type 只是历史兜底；二者不一致时以 invoice_category 为准。
func TestBusinessInvoiceListCategoryUsesAuthorityChain(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{
			ID:              "k_auth",
			KnowledgeBaseID: "kb1",
			ParseStatus:     "completed",
			FileName:        "a.pdf",
			CustomMetadata:  types.JSON(mustJSON(t, renameAwareMeta("能耗发票", "水电发票"))),
		},
	}}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(listCtxV1(), "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "能耗发票", res.Data[0].Category,
		"文件级 invoice_category 是权威源，不得被过时的 fleet_cert_type 覆盖")

	// 按新分类名筛选必须命中
	hit, err := svc.ListInvoiceRecords(listCtxV1(), "kb1", types.InvoiceListFilter{
		InvoiceType: "能耗发票", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Len(t, hit.Data, 1, "改名后的分类名筛选必须命中")
}

// TestBusinessInvoiceListCategoryFallsBackToLegacyAndLine：没有文件级分类时
// 依次兜底 fleet_cert_type 与票内 category，保证老数据仍可归组。
func TestBusinessInvoiceListCategoryFallsBackToLegacyAndLine(t *testing.T) {
	legacyOnly := map[string]interface{}{
		"kind": "invoice", "extract_status": "success",
		"fleet_cert_type": "通行费发票",
		"invoices": []interface{}{
			map[string]interface{}{"invoice_no": "INV-002", "invoice_type": "通行费发票", "category": "通行费发票"},
		},
	}
	lineOnly := map[string]interface{}{
		"kind": "invoice", "extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{"invoice_no": "INV-003", "invoice_type": "水费发票", "category": "水费发票"},
		},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_legacy", KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(t, legacyOnly))},
		{ID: "k_line", KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(t, lineOnly))},
	}}
	svc := &BusinessExtractService{repo: repo}

	res, err := svc.ListInvoiceRecords(listCtxV1(), "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 2)
	byID := map[string]string{}
	for _, r := range res.Data {
		byID[r.KnowledgeID] = r.Category
	}
	assert.Equal(t, "通行费发票", byID["k_legacy"], "无 invoice_category 时兜底 fleet_cert_type")
	assert.Equal(t, "水费发票", byID["k_line"], "文件级两个键都空时兜底票内 category")
}

// TestBusinessInvoiceOverviewCategoryAfterRename：概览统计按分类精确聚合，
// 改名后（权威链读新名）旧票必须计入新分类，不得落到「未分类」。
func TestBusinessInvoiceOverviewCategoryAfterRename(t *testing.T) {
	meta := renameAwareMeta("能耗发票", "水电发票")
	meta["invoices"] = []interface{}{
		map[string]interface{}{"invoice_no": "INV-101", "invoice_type": "能耗发票", "category": "能耗发票", "total_amount": 10},
		map[string]interface{}{"invoice_no": "INV-102", "invoice_type": "能耗发票", "category": "能耗发票", "total_amount": 20},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k1", KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(t, meta))},
	}}
	svc := &BusinessExtractService{repo: repo}

	st, err := svc.InvoiceOverviewStats(listCtxV1(), "kb1", nil)
	require.NoError(t, err)
	counts := map[string]float64{}
	for _, c := range st.ByCategory {
		counts[c.Category] = float64(c.Count)
	}
	assert.Equal(t, float64(2), counts["能耗发票"], "改名后的 2 张票必须计入新分类")
	assert.NotContains(t, counts, "水电发票", "旧分类名不得再出现在统计里")
	assert.Equal(t, float64(2), sumOf(st), "分类统计总张数守恒")
}

func sumOf(st *types.InvoiceOverviewStats) float64 {
	total := float64(0)
	for _, c := range st.ByCategory {
		total += float64(c.Count)
	}
	return total
}
