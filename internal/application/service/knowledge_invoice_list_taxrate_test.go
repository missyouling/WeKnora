package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// taxRateFakeRepo 覆写分页拉取与标签加载，其余接口方法由嵌入的 nil 接口兜底。
type taxRateFakeRepo struct {
	interfaces.KnowledgeRepository
	items []*types.Knowledge
}

func (f *taxRateFakeRepo) ListPagedKnowledgeByKnowledgeBaseID(
	ctx context.Context, tenantID uint64, kbID string, page *types.Pagination, filter types.KnowledgeListFilter,
) ([]*types.Knowledge, int64, error) {
	start := (page.Page - 1) * page.PageSize
	if start > len(f.items) {
		start = len(f.items)
	}
	end := start + page.PageSize
	if end > len(f.items) {
		end = len(f.items)
	}
	return f.items[start:end], int64(len(f.items)), nil
}

func (f *taxRateFakeRepo) GetKnowledgeTags(ctx context.Context, knowledgeIDs []string) (map[string][]*types.KnowledgeTag, error) {
	return map[string][]*types.KnowledgeTag{}, nil
}

// TestInvoiceListTaxRateItemsMatch 验证税率筛选支持「发票级税率 或 明细(items)任意档位税率」命中，
// 兼容多明细多税率发票（如 13%,9%,6% 混开）按任一税率档筛选。
func TestInvoiceListTaxRateItemsMatch(t *testing.T) {
	mk := func(invNo string, invRate, itemRate9 *float64, otherItemRate *float64) *types.Knowledge {
		items := []interface{}{}
		if itemRate9 != nil {
			items = append(items, map[string]interface{}{"name": "明细A", "tax_rate": *itemRate9})
		}
		if otherItemRate != nil {
			items = append(items, map[string]interface{}{"name": "明细B", "tax_rate": *otherItemRate})
		}
		return &types.Knowledge{
			ID:              "k_" + invNo,
			KnowledgeBaseID: "kb1",
			ParseStatus:     "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{
					map[string]interface{}{
						"invoice_no": invNo, "invoice_type": "普通发票",
						"tax_rate": invRate, "amount": 100.0, "tax": 9.0, "total_amount": 109.0,
						"items": items,
					},
				},
			}),
		}
	}

	rate9 := f64(0.09)
	rate13 := f64(0.13)
	rate6 := f64(0.06)
	rate3 := f64(0.03)

	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		mk("INV-A", rate9, nil, nil),     // 发票级 9%：应命中
		mk("INV-B", rate13, rate9, rate6), // 发票级 13%，明细含 9% 与 6%：应命中（items 档位路径）
		mk("INV-C", rate3, nil, nil),     // 发票级 3%：不应命中
	}}
	svc := &BusinessExtractService{repo: repo}

	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		TaxRate: rate9, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	got := map[string]bool{}
	for _, r := range res.Data {
		got[r.InvoiceNo] = true
	}
	assert.True(t, got["INV-A"], "发票级税率命中")
	assert.True(t, got["INV-B"], "明细(items)档位税率命中")
	assert.False(t, got["INV-C"], "不相关税率不应命中")
}

// TestInvoiceListKeywordAndFilters 验证关键字、税率、日期筛选为 AND 复合查询交集：
// 关键词命中只是其中一个过滤条件，不再豁免/清空税率与日期筛选。
func TestInvoiceListKeywordAndFilters(t *testing.T) {
	mkDated := func(invNo, date string, rate *float64) *types.Knowledge {
		return &types.Knowledge{
			ID:              "k_" + invNo,
			KnowledgeBaseID: "kb1",
			ParseStatus:     "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{
					map[string]interface{}{
						"invoice_no": invNo, "invoice_type": "普通发票", "invoice_date": date,
						"tax_rate": rate, "amount": 100.0, "tax": 9.0, "total_amount": 109.0,
						"items": []interface{}{},
					},
				},
			}),
		}
	}
	rate9 := f64(0.09)
	rate3 := f64(0.03)
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		mkDated("INV-A1", "2026-01-05", rate9), // keyword + 税率 + 日期 全命中
		mkDated("INV-B1", "2026-06-05", rate9), // 关键词与税率命中但日期不命中
		mkDated("INV-C1", "2026-01-05", rate3), // 关键词与日期命中但税率不命中
		mkDated("INV-D1", "2026-01-05", rate9), // 税率与日期命中但关键词不命中
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		Keyword: "INV-A", TaxRate: rate9, DateFrom: "2026-01-01", DateTo: "2026-01-31",
		Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Data, 1, "三个条件必须同时命中才返回")
	assert.Equal(t, "INV-A1", res.Data[0].InvoiceNo)
}
