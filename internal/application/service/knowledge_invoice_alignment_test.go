package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本组测试锁定「知识库源文件 ↔ 发票列表 100% 对齐」的三条铁律：
//  1. 关键词必须穿透 invoices[].fields 与 items[].extra（车牌号等明细子列值）；
//  2. 日期筛选必须兼容明细通行日期（travel_from / travel_to）所在区间；
//  3. 无任何有效发票明细的文件必须渲染「文件级占位行」，严禁静默跳过。

// tollFileMetadata 复刻库中 `72a6aae5-a980-4e1c-b77c-d48b810db1d1`
// （通行费电子票据汇总单(发票).pdf，528.7KB，13 张票，开票日期 2025-10-09）：
// 车牌号 `渝C87567` 只存在于 invoices[0].fields.plate_no 与
// invoices[1].items[0].extra.plate_no，**顶层 InvoiceNo/InvoiceCode 等字段都不含它**。
// 用户按车牌 `87567` 查询时必须命中。
func tollFileMetadata() map[string]interface{} {
	return map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": "64299470", "invoice_code": "050002400112",
				"invoice_date": "2025-10-09", "invoice_type": "通行费发票",
				"category": "通行费发票", "amount": 175.8, "tax": 5.27,
				"total_amount": 181.07, "page": 1,
				"buyer_name":  "重庆星达铜业有限公司",
				"seller_name": "重庆铜永高速公路有限公司",
				// 车牌号落在发票级 fields 桶
				"fields": map[string]interface{}{"plate_no": "渝C87567"},
				"items": []interface{}{
					map[string]interface{}{
						"name":   "*经营租赁*通行费",
						"amount": 175.8,
						// 未知键由 InvoiceExtractionLine.UnmarshalJSON 收进 Extra
						"plate_no": "", "travel_from": "", "travel_to": "",
					},
				},
			},
			map[string]interface{}{
				"invoice_no": "64042300", "invoice_code": "050002400113",
				"invoice_date": "2025-10-09", "invoice_type": "通行费发票",
				"category": "通行费发票", "amount": 343.32, "tax": 10.3,
				"total_amount": 353.62, "page": 2,
				"buyer_name":  "重庆星达铜业有限公司",
				"seller_name": "重庆江綦高速公路有限公司",
				// 车牌号落在明细子列（未知键 → Extra）
				"items": []interface{}{
					map[string]interface{}{
						"name":        "*经营租赁*通行费",
						"amount":      343.32,
						"plate_no":    "渝C87567",
						"travel_from": "2025-09-06", "travel_to": "2025-09-29",
						"vehicle_type": "货车",
					},
				},
			},
		},
	}
}

// TestBusinessInvoiceKeywordPenetratesFieldsAndItemsExtra 锁定穿透检索：
// 搜 `87567`（车牌号）时必须命中，搜明细项名称/规格也必须命中；
// 同时确认不相关关键词仍然不命中（防止穿透退化成万物命中）。
func TestBusinessInvoiceKeywordPenetratesFieldsAndItemsExtra(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{
			ID:              "k_toll",
			KnowledgeBaseID: "kb1",
			ParseStatus:     "completed",
			FileName:        "通行费电子票据汇总单(发票).pdf",
			CustomMetadata:  mustJSON(t, tollFileMetadata()),
		},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	for _, kw := range []string{"87567", "渝C87567", "经营租赁"} {
		res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
			Keyword: kw, Page: 1, PageSize: 20,
		})
		require.NoError(t, err)
		assert.Len(t, res.Data, 2, "关键词 %q 必须穿透 fields/items.extra 命中 2 张票", kw)
	}

	// 不相关词不得命中
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		Keyword: "ZZZ-NO-SUCH", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Data)
}

// TestBusinessInvoiceKeywordPenetratesCustomFieldsValue 锁定 fields 非字符串值
// （数字/布尔）也被纳入搜索，防止模型把车牌号写成数字后再次隐身。
func TestBusinessInvoiceKeywordPenetratesCustomFieldsValue(t *testing.T) {
	meta := map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": "INV-NUM", "invoice_date": "2025-10-09",
				"fields": map[string]interface{}{"plate_no": 12345678},
			},
		},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_num", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: mustJSON(t, meta)},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		Keyword: "12345678", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Len(t, res.Data, 1, "数字型 fields 值也必须可搜")
}

// TestBusinessInvoiceDateFilterMatchesTravelDates 锁定日期筛选兼容通行日期：
// 汇总单次月开票（2025-11-05）但通行日期落在 2025-10 区间 → 仍应命中，
// 避免用户按「通行月份」筛选时把已入库的票筛掉。
func TestBusinessInvoiceDateFilterMatchesTravelDates(t *testing.T) {
	meta := map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"invoices": []interface{}{
			map[string]interface{}{
				"invoice_no": "INV-TRAVEL", "invoice_date": "2025-11-05",
				"category": "通行费发票", "page": 1,
				"items": []interface{}{
					map[string]interface{}{
						"name": "*经营租赁*通行费",
						// 未知键（含中文同义键）由 UnmarshalJSON 收进 Extra
						"plate_no":    "渝C87654",
						"travel_from": "2025-10-06", "travel_to": "2025-10-28",
					},
				},
			},
		},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_travel", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: mustJSON(t, meta)},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	// 顶层 invoice_date 是 2025-11-05，2025-10 区间不命中它，但通行日期命中
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		DateFrom: "2025-10-01", DateTo: "2025-10-31", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Len(t, res.Data, 1, "按通行月份筛选必须命中次月开票的汇总单")

	// 完全不相关的月份不得命中
	res, err = svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		DateFrom: "2025-08-01", DateTo: "2025-08-31", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Data, "通行日期与顶层日期都不在区间内时不得命中")
}

// TestBusinessInvoiceEmptyInvoicesRenderPlaceholderRow 锁定铁律一（非失败态）：
// kind=invoice、invoices 为空但**未标记提取失败**（解析出 0 张 / 删空残留）的文件，
// 必须渲染一条文件级占位行，严禁静默跳过造成孤儿数据。
//
// 2026-10-20 契约变更：extract_status="failed" 的文件改由「历史记录」抽屉承载
// （见 TestBusinessInvoiceExtractFailedExcludedFromMainList），不再在此占位。
func TestBusinessInvoiceEmptyInvoicesRenderPlaceholderRow(t *testing.T) {
	meta := map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"extract_error":  "解析出 0 张发票",
		"invoices":       []interface{}{},
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_orphan", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName:       "通行费电子票据汇总单(发票).pdf",
			CustomMetadata: mustJSON(t, meta)},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 1, "非失败态的空 invoices 文件必须渲染占位行，不得静默跳过")
	row := res.Data[0]
	assert.True(t, row.IsPlaceholder, "该行必须是文件级占位行")
	assert.Equal(t, "k_orphan", row.KnowledgeID)
	assert.Equal(t, "通行费电子票据汇总单(发票).pdf", row.FileName)
	assert.NotEmpty(t, row.PlaceholderReason, "占位行必须给出原因（未提取出票据）")
}

// TestBusinessInvoicePlaceholderRowSearchableByFileName 锁定占位行仍可被搜索：
// 用户靠文件名找回上传过的文件（重复上传报错后的第一反应）。
func TestBusinessInvoicePlaceholderRowSearchableByFileName(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_orphan", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "通行费电子票据汇总单(发票).pdf",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success", "invoices": []interface{}{},
			})},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		Keyword: "通行费电子票据汇总单", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Len(t, res.Data, 1, "非失败态占位行必须能按文件名搜到")
}

// TestBusinessInvoiceExtractFailedExcludedFromMainList：提取失败的文件
// **不得**出现在发票主列表（含「其它票据」），也不得被搜索命中 ——
// 批量上传触发 429/1302 限流时，一批文件会同时提取失败，若各自占位会把
// 「其它票据」整屏染成橙色失败行。它们的归宿是「历史记录」抽屉。
func TestBusinessInvoiceExtractFailedExcludedFromMainList(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_failed", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "通行费电子票据汇总单(发票).pdf",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "failed",
				"extract_error": "API request failed with status 429", "invoices": []interface{}{},
			})},
		// 全空白提取（LLM 空对象）+ failed：同样属于"一张票都没提出来"
		{ID: "k_failed_blank", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "空白提取.pdf",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "failed",
				"invoices": []interface{}{map[string]interface{}{}},
			})},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Empty(t, res.Data, "提取失败且无有效发票的文件不得进入主列表")

	res, err = svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{
		Keyword: "通行费电子票据汇总单", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Data, "提取失败文件不得被主列表搜索命中")
}

// TestBusinessInvoiceExtractFailedPartialStillVisible：失败状态但仍提取出有效票据
// 的文件（部分成功）必须照常展示 —— 数据不得因一次失败整体隐身。
func TestBusinessInvoiceExtractFailedPartialStillVisible(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_partial", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "部分成功.pdf",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "failed",
				"invoices": []interface{}{map[string]interface{}{
					"invoice_no": "12345678", "invoice_date": "2025-02-08",
				}},
			})},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 1, "部分成功的文件必须展示已提取出的票据")
	assert.Equal(t, "12345678", res.Data[0].InvoiceNo)
}

// TestBusinessInvoiceOverviewCountsExtractFailedFiles：概览统计必须给出
// **文件级**提取失败数（供「历史记录」卡片显示「N 份提取失败」）。
func TestBusinessInvoiceOverviewCountsExtractFailedFiles(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_f1", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "failed", "invoices": []interface{}{},
			})},
		{ID: "k_f2", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "failed", "invoices": []interface{}{},
			})},
		{ID: "k_ok", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "invoice", "extract_status": "success",
				"invoices": []interface{}{map[string]interface{}{"invoice_no": "111", "invoice_type": "通行费发票"}},
			})},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	stats, err := svc.InvoiceOverviewStats(ctx, "kb1", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.ExtractFailedFiles, "文件级提取失败数应为 2")
	assert.Equal(t, 1, stats.Total, "有效发票总数应为 1（失败文件不计数）")
	// 失败文件无发票 → 不进「其它票据」桶
	for _, stat := range stats.ByInvoiceType {
		if stat.InvoiceType == "其它票据" {
			assert.Equal(t, 1, stat.Count, "其它票据只应计入真实票据，不含失败占位")
		}
	}
}

// TestBusinessInvoicePlaceholderForNonInvoiceKind 铁律一（补齐）：
// 发票知识库里被判定为「非票据」或从未提取的文件同样必须出现在列表，
// 否则它照样占着哈希锁却看不见 —— 与 invoices 为空的孤儿是同一类问题。
func TestBusinessInvoicePlaceholderForNonInvoiceKind(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_notinv", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "误传的合同.pdf",
			CustomMetadata: mustJSON(t, map[string]interface{}{
				"kind": "not_invoice", "extract_status": "success", "invoices": []interface{}{},
			})},
		{ID: "k_nometa", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName: "刚上传未提取.pdf"},
		{ID: "k_parsefail", KnowledgeBaseID: "kb1", ParseStatus: "failed",
			FileName: "坏文件.pdf"},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 3, "非票据/无 metadata/解析失败的文件都必须占位显示，不得隐身")

	byFile := map[string]types.InvoiceRecord{}
	for _, r := range res.Data {
		byFile[r.FileName] = r
	}
	for name, wantReason := range map[string]string{
		"误传的合同.pdf":  "未识别为发票",
		"刚上传未提取.pdf": "未提取",
		"坏文件.pdf":    "解析失败",
	} {
		row, ok := byFile[name]
		require.True(t, ok, "%s 必须出现在列表", name)
		assert.True(t, row.IsPlaceholder, "%s 必须是占位行", name)
		assert.Contains(t, row.PlaceholderReason, wantReason, "%s 的占位原因应包含 %q", name, wantReason)
	}
}

// TestBusinessInvoicePlaceholderNotDuplicatedForHealthyFiles：健康文件不得被
// 额外造出占位行（一条发票一张行，行数与发票数严格一致）。
func TestBusinessInvoicePlaceholderNotDuplicatedForHealthyFiles(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_ok", KnowledgeBaseID: "kb1", ParseStatus: "completed",
			FileName:       "通行费电子票据汇总单(发票).pdf",
			CustomMetadata: mustJSON(t, tollFileMetadata())},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, res.Data, 2, "2 张票 → 恰好 2 行")
	for _, r := range res.Data {
		assert.False(t, r.IsPlaceholder, "健康文件不得产生占位行")
	}
}

// TestBusinessInvoiceInProgressFileNotPlaceholder：解析/提取中的文件仍由顶部
// 状态条展示，不以占位行占用列表（保持既有交互契约）。
func TestBusinessInvoiceInProgressFileNotPlaceholder(t *testing.T) {
	repo := &taxRateFakeRepo{items: []*types.Knowledge{
		{ID: "k_run", KnowledgeBaseID: "kb1", ParseStatus: "processing",
			FileName: "解析中.pdf"},
	}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	res, err := svc.ListInvoiceRecords(ctx, "kb1", types.InvoiceListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Empty(t, res.Data, "解析中的文件由状态条展示，不进列表")
}
