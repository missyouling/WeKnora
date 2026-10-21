package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 一份源文件 = 一条记录：多批次结果必须按「非空优先」合并，而不是追加。
// 真机事故：厂房租赁协议书两批分别给出甲乙方/金额 与 签订日期，落库成
// HT-...-001 / -002 两条记录，已收录合同 3 条 > 历史记录 2 份。
func TestMergeContractBatchesIntoSingleRecord(t *testing.T) {
	amount := 729000.0
	items := []ContractExtractionItem{
		{ContractName: "厂房租赁协议书", PartyA: "重庆星达铜业有限公司", PartyB: "重庆持恒模具有限公司", Amount: &amount},
		{ContractName: "厂房租赁协议书", PartyA: "重庆星达铜业有限公司", SignDate: "2023-08-08"},
	}
	merged, ok := MergeExtractionItems(items)
	require.True(t, ok)
	assert.Equal(t, "重庆星达铜业有限公司", merged.PartyA)
	assert.Equal(t, "重庆持恒模具有限公司", merged.PartyB, "前批次非空字段不得被后批次空值覆盖")
	assert.Equal(t, "2023-08-08", merged.SignDate, "后批次的非空字段必须补齐前批次缺失")
	require.NotNil(t, merged.Amount)
	assert.Equal(t, 729000.0, *merged.Amount)

	res := &ContractExtractionResult{Kind: "contract", Contracts: items}
	NormalizeContractExtractionResult(res, 0)
	require.Len(t, res.Contracts, 1, "一份文件必须严格只落一条记录")
	assert.NotEmpty(t, res.Contracts[0].ContractNo)
	assert.Contains(t, res.Contracts[0].ContractNo, "HT-")
	assert.Equal(t, "2023-08-08", res.Contracts[0].SignDate)
	assert.Equal(t, "重庆持恒模具有限公司", res.Contracts[0].PartyB)
}

func TestMergeRegulationAndNoticeBatchesIntoSingleRecord(t *testing.T) {
	regs := []types.RegulationExtractionItem{
		{RegNo: "AD-028", RegName: "员工转岗管理制度", Dept: "行政部"},
		{RegNo: "AD-028", IssueDate: "2026-03-01"},
	}
	mergedReg, ok := MergeExtractionItems(regs)
	require.True(t, ok)
	assert.Equal(t, "行政部", mergedReg.Dept)
	assert.Equal(t, "2026-03-01", mergedReg.IssueDate)
	regRes := &RegulationExtractionResult{Kind: "regulation", Regulations: regs}
	NormalizeRegulationExtractionResult(regRes, 0)
	assert.Len(t, regRes.Regulations, 1, "制度一文件一档案，不得裂变")

	notices := []types.AwardPunishExtractionItem{
		{ApNo: "星达行政字〔2026〕09号", ApTitle: "关于对刘志鹏等人工作失职的通报", IssueDept: "行政部"},
		{Signer: "廖宏", SignDate: "2026-03-20"},
	}
	mergedNotice, ok := MergeExtractionItems(notices)
	require.True(t, ok)
	assert.Equal(t, "行政部", mergedNotice.IssueDept)
	assert.Equal(t, "2026-03-20", mergedNotice.SignDate)
	noticeRes := &AwardPunishExtractionResult{Kind: "award_punish", Records: notices}
	NormalizeAwardPunishExtractionResult(noticeRes, 0)
	assert.Len(t, noticeRes.Records, 1, "公告一文件一档案，不得裂变")
}
