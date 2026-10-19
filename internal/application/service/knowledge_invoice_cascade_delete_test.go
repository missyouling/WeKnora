package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本组测试锁定「列表删除记录 ↔ 知识库源文件级联删除」铁律：
// 多票文件被逐张删空后，绝不允许留下 invoices: [] 的空壳孤儿记录
// —— 它既在列表里隐身，又会以「知识库中已有相同文件」永久挡住用户重传。

// alignFakeRepo 记录 UpdateKnowledgeColumns / DeleteKnowledge 调用，便于断言。
type alignFakeRepo struct {
	*taxRateFakeRepo
	row       *types.Knowledge
	colWrites []map[string]interface{}
	deleted   []string
	// deleteErr 注入 DeleteKnowledge 失败（用于验证降级路径）
	deleteErr error
	// exists 控制 CheckKnowledgeExists 返回值（验证空壳是否还挡重传）
	exists   bool
	existsKb *types.Knowledge
}

func (f *alignFakeRepo) GetKnowledgeByID(ctx context.Context, tenantID uint64, id string) (*types.Knowledge, error) {
	if f.row != nil && f.row.ID == id {
		return f.row, nil
	}
	return nil, assert.AnError
}

func (f *alignFakeRepo) UpdateKnowledgeColumn(ctx context.Context, id string, column string, value interface{}) error {
	f.colWrites = append(f.colWrites, map[string]interface{}{column: value})
	if f.row != nil && f.row.ID == id {
		if raw, ok := value.(types.JSON); ok && column == "custom_metadata" {
			f.row.CustomMetadata = raw
		}
		if v, ok := value.(string); ok && column == "parse_status" {
			f.row.ParseStatus = v
		}
	}
	return nil
}

func (f *alignFakeRepo) UpdateKnowledgeColumns(ctx context.Context, id string, values map[string]interface{}) error {
	cp := make(map[string]interface{}, len(values))
	for k, v := range values {
		cp[k] = v
	}
	f.colWrites = append(f.colWrites, cp)
	// 同步回写内存行，模拟落库
	if f.row != nil && f.row.ID == id {
		if raw, ok := values["custom_metadata"].(types.JSON); ok {
			f.row.CustomMetadata = raw
		}
		if v, ok := values["parse_status"].(string); ok {
			f.row.ParseStatus = v
		}
	}
	return nil
}

func (f *alignFakeRepo) DeleteKnowledge(ctx context.Context, tenantID uint64, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *alignFakeRepo) CheckKnowledgeExists(ctx context.Context, tenantID uint64, kbID string, params *types.KnowledgeCheckParams) (bool, *types.Knowledge, error) {
	return f.exists, f.existsKb, nil
}

func alignRow(t *testing.T, id string, invoices []interface{}, extra map[string]interface{}) *types.Knowledge {
	meta := map[string]interface{}{
		"kind":           "invoice",
		"extract_status": "success",
		"invoices":       invoices,
	}
	for k, v := range extra {
		meta[k] = v
	}
	return &types.Knowledge{
		ID:              id,
		TenantID:        1,
		KnowledgeBaseID: "kb1",
		ParseStatus:     "completed",
		FileName:        "通行费电子票据汇总单(发票).pdf",
		CustomMetadata:  mustJSON(t, meta),
	}
}

func inv(no, date string, page int) map[string]interface{} {
	return map[string]interface{}{
		"invoice_no": no, "invoice_date": date, "page": page,
		"category": "通行费发票", "total_amount": 100.0,
	}
}

// TestBusinessRemoveLastInvoiceCascadesSourceFileDelete 锁定核心铁律：
// 删掉一档多票文件的最后一张发票时，源文件必须在同一次调用内被彻底删除
// （repo.DeleteKnowledge），绝不能只写 invoices: [] 留下空壳。
func TestBusinessRemoveLastInvoiceCascadesSourceFileDelete(t *testing.T) {
	row := alignRow(t, "k_toll", []interface{}{
		inv("64299470", "2025-10-09", 1),
	}, nil)
	repo := &alignFakeRepo{taxRateFakeRepo: &taxRateFakeRepo{items: []*types.Knowledge{row}}, row: row}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	deletedFile, _, err := svc.RemoveInvoicePage(ctx, "kb1", "k_toll", 1)
	require.NoError(t, err)
	assert.True(t, deletedFile, "删掉最后一张必须整份删除源文件")
	require.Len(t, repo.deleted, 1, "必须调用 repo.DeleteKnowledge")
	assert.Equal(t, "k_toll", repo.deleted[0])
}

// TestBusinessRemoveInvoiceKeepsFileWhileOthersRemain：还有其它发票时只改
// metadata，绝不删除源文件。
func TestBusinessRemoveInvoiceKeepsFileWhileOthersRemain(t *testing.T) {
	row := alignRow(t, "k_toll", []interface{}{
		inv("64299470", "2025-10-09", 1),
		inv("64042300", "2025-10-09", 2),
		inv("41706947", "2025-10-09", 3),
	}, nil)
	repo := &alignFakeRepo{taxRateFakeRepo: &taxRateFakeRepo{items: []*types.Knowledge{row}}, row: row}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	deletedFile, _, err := svc.RemoveInvoicePage(ctx, "kb1", "k_toll", 3)
	require.NoError(t, err)
	assert.False(t, deletedFile, "还有剩余发票时不得删除源文件")
	assert.Empty(t, repo.deleted, "不得调用 repo.DeleteKnowledge")
	require.Len(t, repo.colWrites, 1, "必须持久化剩余 invoices")

	var after map[string]interface{}
	require.NoError(t, json.Unmarshal(row.CustomMetadata, &after))
	got := after["invoices"].([]interface{})
	require.Len(t, got, 2, "剩余 2 张")
	// 页码必须前移重排，保持 page = 文档内出现顺序
	for i, e := range got {
		assert.EqualValues(t, i+1, e.(map[string]interface{})["page"], "页码必须前移重排")
	}
	// 不得写入删除中/待删除标记
	assert.Equal(t, "completed", row.ParseStatus, "仍有剩余发票时 parse_status 不得被改写")
	_, pending := after["pending_file_delete"]
	assert.False(t, pending, "仍有剩余发票时不得写入 pending_file_delete")
}

// TestBusinessRemoveLastInvoiceMarksDeletingBeforeCascade 锁定降级路径：
// 即便 repo.DeleteKnowledge 因瞬时故障失败，也已把 parse_status 置为 deleting
// 并写入 pending_file_delete，使该文件不再被「知识库中已有相同文件」挡住重传，
// 且能被后续巡检兜底清理。
func TestBusinessRemoveLastInvoiceMarksDeletingBeforeCascade(t *testing.T) {
	row := alignRow(t, "k_toll", []interface{}{
		inv("64299470", "2025-10-09", 1),
	}, nil)
	repo := &alignFakeRepo{
		taxRateFakeRepo: &taxRateFakeRepo{items: []*types.Knowledge{row}},
		row:             row,
		deleteErr:       assert.AnError,
	}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	deletedFile, _, err := svc.RemoveInvoicePage(ctx, "kb1", "k_toll", 1)
	// 删除失败必须上报，但状态已降级为可恢复
	require.Error(t, err)
	assert.False(t, deletedFile)
	require.Len(t, repo.colWrites, 1, "删除前必须先落一次 columns")
	w := repo.colWrites[0]
	assert.Equal(t, "deleting", w["parse_status"], "必须先置 deleting 解除哈希锁")
	var meta map[string]interface{}
	require.NoError(t, json.Unmarshal(w["custom_metadata"].(types.JSON), &meta))
	assert.Equal(t, true, meta["pending_file_delete"], "必须标记 pending_file_delete 供巡检兜底")
	assert.Len(t, meta["invoices"], 0, "invoices 必须已置空")
}

// TestBusinessRemoveInvoicePageOutOfRange：页码越界必须报错，且不得动数据。
func TestBusinessRemoveInvoicePageOutOfRange(t *testing.T) {
	row := alignRow(t, "k_toll", []interface{}{inv("64299470", "2025-10-09", 1)}, nil)
	repo := &alignFakeRepo{taxRateFakeRepo: &taxRateFakeRepo{items: []*types.Knowledge{row}}, row: row}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	_, _, err := svc.RemoveInvoicePage(ctx, "kb1", "k_toll", 9)
	require.Error(t, err)
	assert.Empty(t, repo.colWrites)
	assert.Empty(t, repo.deleted)
}

// TestBusinessRemoveInvoiceKnowledgeMustBelongToKB：跨库越权删除必须被拦。
func TestBusinessRemoveInvoiceKnowledgeMustBelongToKB(t *testing.T) {
	row := alignRow(t, "k_other", []interface{}{inv("64299470", "2025-10-09", 1)}, nil)
	row.KnowledgeBaseID = "kb-other"
	repo := &alignFakeRepo{taxRateFakeRepo: &taxRateFakeRepo{}, row: row}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	_, _, err := svc.RemoveInvoicePage(ctx, "kb1", "k_other", 1)
	require.Error(t, err)
	assert.Empty(t, repo.deleted)
}

// TestBusinessSummarizeInvoiceAlignmentCountsAllOrphans 锁定巡检统计口径
// 必须与列表行展开一致：非票据 / 无 metadata / 空 invoices 都算孤儿，
// 解析中的文件不算（由状态条展示）。
func TestBusinessSummarizeInvoiceAlignmentCountsAllOrphans(t *testing.T) {
	healthy := alignRow(t, "k_ok", []interface{}{inv("64299470", "2025-10-09", 1)}, nil)
	emptyInv := alignRow(t, "k_empty", []interface{}{}, map[string]interface{}{"extract_status": "failed"})
	notInv := alignRow(t, "k_notinv", []interface{}{}, map[string]interface{}{"kind": "not_invoice"})
	noMeta := &types.Knowledge{ID: "k_nometa", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed"}
	inProgress := &types.Knowledge{ID: "k_run", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "processing"}

	repo := &taxRateFakeRepo{items: []*types.Knowledge{healthy, emptyInv, notInv, noMeta, inProgress}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	got, err := svc.SummarizeInvoiceAlignment(ctx, "kb1")
	require.NoError(t, err)
	assert.Equal(t, 5, got.TotalFiles)
	assert.Equal(t, 1, got.InProgressFiles, "解析中不计孤儿")
	// 非票据 + 无 metadata 两类都归入 NotInvoiceFiles
	assert.Equal(t, 2, got.NotInvoiceFiles)
	assert.Equal(t, 1, got.PlaceholderRows)
	assert.ElementsMatch(t, []string{"k_empty", "k_notinv", "k_nometa"}, got.OrphanKnowledge,
		"孤儿清单必须覆盖三种隐身形态，且不含健康/进行中文件")
}

// TestBusinessSweepStalePendingFileDelete 锁定巡检兜底：
// 只有 pending_file_delete + deleting 且超过阈值的记录才被清理；
// 正常发票文件绝不能进清理名单。
func TestBusinessSweepStalePendingFileDelete(t *testing.T) {
	stale := alignRow(t, "k_stale", []interface{}{}, map[string]interface{}{"pending_file_delete": true})
	stale.ParseStatus = "deleting"
	stale.UpdatedAt = stale.CreatedAt.AddDate(0, 0, -3)
	healthy := alignRow(t, "k_ok", []interface{}{inv("64299470", "2025-10-09", 1)}, nil)
	notPending := alignRow(t, "k_plain", []interface{}{}, map[string]interface{}{})
	notPending.ParseStatus = "deleting"

	repo := &taxRateFakeRepo{items: []*types.Knowledge{stale, healthy, notPending}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	ids, err := svc.SweepStalePendingFileDeletes(ctx, "kb1", 24*60*60) // 24h
	require.NoError(t, err)
	assert.Equal(t, []string{"k_stale"}, ids, "只清理超时的 pending_file_delete 空壳")
}
