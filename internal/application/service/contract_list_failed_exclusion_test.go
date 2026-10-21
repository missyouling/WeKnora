package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 维度 7 契约：整份提取失败且没有任何有效合同的文件，不得出现在主业务列表
// （含「其它合同」桶——该桶由前端对列表行按未命中分类聚合，行不存在即不会入桶），
// 只在「历史记录 -> 异常文件」里以失败态呈现。
//
// 同时守住「部分成功必须可见」：失败态但已提取出合同的文件照常展示，
// 否则重提失败会把用户已确认过的合同从列表里抹掉。
func TestContractListExcludesFailedFileWithoutRecords(t *testing.T) {
	failedNoRecord := &types.Knowledge{
		ID:              "k_failed_empty",
		KnowledgeBaseID: "kb1",
		ParseStatus:     "completed",
		FileName:        "限流失败.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind":           "contract",
			"extract_status": "failed",
			"extract_error":  "upstream 429",
			"contracts":      []interface{}{},
		}),
	}
	// 全空提取项（模型返回空对象）同样视为「没有有效合同」
	failedBlankItem := &types.Knowledge{
		ID:              "k_failed_blank",
		KnowledgeBaseID: "kb1",
		ParseStatus:     "completed",
		FileName:        "空提取.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind":           "contract",
			"extract_status": "failed",
			"contracts":      []interface{}{map[string]interface{}{"contract_no": "", "contract_name": ""}},
		}),
	}
	// 部分成功：失败态但已有一份完整合同 → 必须照常展示
	partial := &types.Knowledge{
		ID:              "k_partial",
		KnowledgeBaseID: "kb1",
		ParseStatus:     "completed",
		FileName:        "部分成功.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind":           "contract",
			"extract_status": "failed",
			"contracts": []interface{}{
				map[string]interface{}{
					"contract_no":   "HT-2026-001",
					"contract_name": "设备采购合同",
					"contract_type": "采购合同",
					"sign_date":     "2026-09-01",
				},
			},
		}),
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{failedNoRecord, failedBlankItem, partial}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListContractRecords(ctx, "kb1", types.ContractListFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Len(t, res.Data, 1, "只有部分成功的文件可进主列表")
	assert.Equal(t, "HT-2026-001", res.Data[0].ContractNo)
	assert.Equal(t, "k_partial", res.Data[0].KnowledgeID)
	for _, r := range res.Data {
		assert.NotEqual(t, "k_failed_empty", r.KnowledgeID, "提取失败且无有效合同的文件不得出现在主列表")
		assert.NotEqual(t, "k_failed_blank", r.KnowledgeID, "空提取项同样不得生成占位行")
	}

	stats, err := svc.ContractOverviewStats(ctx, "kb1")
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Total, "概览总份数只统计有有效合同的文件")
	// 失败计数是「记录口径」：被排除的失败文件没有任何记录，不会贡献计数；
	// 文件级失败数由「历史记录」抽屉（上传文件统计）承接，两套口径各司其职。
	assert.Equal(t, 1, stats.ExtractFailed, "只有部分成功那条记录计入失败记录数")
}
