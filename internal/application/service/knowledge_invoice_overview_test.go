package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// invoiceOverviewFakeRepo 最小 fake：仅覆写分页拉取，其余接口方法由嵌入的 nil 接口兜底。
type invoiceOverviewFakeRepo struct {
	interfaces.KnowledgeRepository
	items []*types.Knowledge
}

func (f *invoiceOverviewFakeRepo) ListPagedKnowledgeByKnowledgeBaseID(
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

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return b
}

func f64(v float64) *float64 { return &v }

func TestInvoiceOverviewStats(t *testing.T) {
	now := time.Now()
	month := now.Format("2006-01")
	lastMonth := now.AddDate(0, -1, 0).Format("2006-01")

	am := f64(100)
	total := f64(113)
	total2 := f64(226)

	items := []*types.Knowledge{
		{
			ID:             "k1",
			KnowledgeBaseID: "kb1",
			ParseStatus:    "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{
					map[string]interface{}{"invoice_no": "A001", "invoice_date": month + "-01", "invoice_type": "专用发票", "category": "专用发票", "amount": am, "total_amount": total},
					map[string]interface{}{"invoice_no": "A002", "invoice_date": lastMonth + "-15", "invoice_type": "普通发票", "category": "电费发票", "total_amount": total2},
				},
			}),
			CreatedAt: now.Add(-time.Hour),
		},
		{
			ID:             "k2",
			KnowledgeBaseID: "kb1",
			ParseStatus:    "failed", // 文件级解析失败
			CustomMetadata: []byte(`{"kind":"invoice"}`),
			CreatedAt:      now.Add(-2 * time.Hour),
		},
		{
			ID:             "k3",
			KnowledgeBaseID: "kb1",
			ParseStatus:    "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{
					// A001 同号更晚版本（created_at 更新）→ 去重应保留它
					map[string]interface{}{"invoice_no": "A001", "invoice_date": month + "-20", "invoice_type": "专用发票", "category": "专用发票", "total_amount": f64(999)},
					// 空提取项 → 应被剔除
					map[string]interface{}{"invoice_no": "", "invoice_date": "", "invoice_type": ""},
				},
			}),
			CreatedAt: now,
		},
	}

	svc := &BusinessExtractService{repo: &invoiceOverviewFakeRepo{items: items}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	stats, err := svc.InvoiceOverviewStats(ctx, "kb1")
	if err != nil {
		t.Fatalf("InvoiceOverviewStats error: %v", err)
	}

	// 去重后：A001(最新 999)、A002 —— 2 张；空提取被剔除
	if stats.Total != 2 {
		t.Errorf("Total = %d, want 2", stats.Total)
	}
	if stats.ParseFailed != 1 {
		t.Errorf("ParseFailed = %d, want 1", stats.ParseFailed)
	}
	if stats.ExtractFailed != 0 {
		t.Errorf("ExtractFailed = %d, want 0", stats.ExtractFailed)
	}
	// 价税合计：999 + 226
	if stats.SumTotal != 1225 {
		t.Errorf("SumTotal = %v, want 1225", stats.SumTotal)
	}
	// 本月：A001（month-20 最新）→ 1 张，999
	if stats.CurrentMonth.Count != 1 {
		t.Errorf("CurrentMonth.Count = %d, want 1", stats.CurrentMonth.Count)
	}
	if stats.CurrentMonth.SumTotal != 999 {
		t.Errorf("CurrentMonth.SumTotal = %v, want 999", stats.CurrentMonth.SumTotal)
	}
	// 类型分组：固定三桶（专用发票1张999 + 普通发票1张226 + 其它票据0张），按张数降序
	if len(stats.ByInvoiceType) != 3 {
		t.Fatalf("ByInvoiceType len = %d, want 3", len(stats.ByInvoiceType))
	}
	if stats.ByInvoiceType[0].InvoiceType != "专用发票" || stats.ByInvoiceType[0].Count != 1 {
		t.Errorf("ByInvoiceType[0] = %+v, want 专用发票 x1", stats.ByInvoiceType[0])
	}
	// 空桶"其它票据"必须补齐（count=0），前端类型卡稳定展示
	foundOther := false
	for _, bt := range stats.ByInvoiceType {
		if bt.InvoiceType == "其它票据" && bt.Count == 0 {
			foundOther = true
		}
	}
	if !foundOther {
		t.Errorf("ByInvoiceType missing zero-count 其它票据 bucket: %+v", stats.ByInvoiceType)
	}
	// 分类分组：A001(category=专用发票, 999) + A002(category=电费发票, 226)，按张数降序
	if len(stats.ByCategory) != 2 {
		t.Fatalf("ByCategory len = %d, want 2: %+v", len(stats.ByCategory), stats.ByCategory)
	}
	if stats.ByCategory[0].Category != "专用发票" || stats.ByCategory[0].Count != 1 {
		t.Errorf("ByCategory[0] = %+v, want 专用发票 x1", stats.ByCategory[0])
	}
	catSum := map[string]float64{}
	for _, c := range stats.ByCategory {
		catSum[c.Category] = c.SumTotal
	}
	if catSum["电费发票"] != 226 {
		t.Errorf("ByCategory 电费发票 SumTotal = %v, want 226", catSum["电费发票"])
	}
}

func TestInvoiceOverviewStatsEmpty(t *testing.T) {
	svc := &BusinessExtractService{repo: &invoiceOverviewFakeRepo{items: nil}}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	stats, err := svc.InvoiceOverviewStats(ctx, "kb1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if stats.Total != 0 || stats.ParseFailed != 0 || stats.ExtractFailed != 0 {
		t.Errorf("empty stats = %+v, want all zero", stats)
	}
	// 空场景同样固定三桶（全部 0 值），保证前端类型卡始终渲染
	if len(stats.ByInvoiceType) != 3 {
		t.Fatalf("empty ByInvoiceType len = %d, want 3 fixed buckets, got %+v", len(stats.ByInvoiceType), stats.ByInvoiceType)
	}
	for _, bt := range stats.ByInvoiceType {
		if bt.Count != 0 {
			t.Errorf("empty bucket %s Count = %d, want 0", bt.InvoiceType, bt.Count)
		}
	}
	// 空场景分类桶为空数组（前端动态渲染，无强制桶）
	if stats.ByCategory == nil || len(stats.ByCategory) != 0 {
		t.Errorf("empty ByCategory = %+v, want empty slice", stats.ByCategory)
	}
}
