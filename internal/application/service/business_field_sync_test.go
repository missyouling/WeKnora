package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// ============ 任务 1（读数权威链）：分类归属唯一口径 ============

// TestContractListResolvesCategoryFromFleetCertType 锁定读数链：
// 顶层 fleet_cert_type 命中启用分类时，即使数组里的 contract_type 还是旧名，
// 记录的 ContractType 也必须取启用分类名（改名传播遗漏时的兜底），
// 从构造上消灭「列显示旧名、桶在新名」的分裂。
func TestContractListResolvesCategoryFromFleetCertType(t *testing.T) {
	stale := &types.Knowledge{
		ID: "k_stale", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "改名残留.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind":            "contract",
			"extract_status":  "success",
			"fleet_cert_type": "商务合同", // 新名（启用）
			"contracts": []interface{}{
				map[string]interface{}{
					"contract_no": "HT-1", "contract_name": "咨询协议",
					"contract_type": "服务合同", // 旧名（未传播）
					"party_a":       "甲公司", "party_b": "乙公司",
				},
			},
		}),
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{stale}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListContractRecords(ctx, "kb1", types.ContractListFilter{
		Page: 1, PageSize: 50,
		EnabledCategories: []string{"商务合同", "租赁合同"},
	})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	assert.Equal(t, "商务合同", res.Data[0].ContractType,
		"fleet_cert_type 命中启用分类时必须覆盖数组里的旧名")

	stats, err := svc.ContractOverviewStats(ctx, "kb1", "商务合同", "租赁合同")
	require.NoError(t, err)
	found := false
	for _, c := range stats.ByCategory {
		if c.Category == "商务合同" {
			found = true
			assert.Equal(t, 1, c.Count)
		}
	}
	assert.True(t, found, "概览必须把该记录计入「商务合同」桶，而不是「其它合同」")
}

// ============ 任务 2：在途/空壳文件禁入主列表 ============

// TestContractListExcludesInFlightAndBlankPlaceholders 锁定：
// 解析中/待提取的文件与「提取成功但全空」的空壳文件都不得进主列表
// （顶部的 BusinessParseStatusBar 与「历史记录」抽屉才是它们的归宿）。
func TestContractListExcludesInFlightAndBlankPlaceholders(t *testing.T) {
	inFlight := &types.Knowledge{
		ID: "k_parsing", KnowledgeBaseID: "kb1", ParseStatus: "processing",
		FileName: "上传中.pdf", CustomMetadata: types.JSON(`{}`),
	}
	pendingExtract := &types.Knowledge{
		ID: "k_pending", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "待提取.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "contract", "extract_status": "pending", "contracts": []interface{}{},
		}),
	}
	// 空壳：success 但一条有效合同都没有（无编号/无名称/无双方/无金额）
	blankShell := &types.Knowledge{
		ID: "k_blank", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "集体宿舍配租明细表（9间）.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "contract", "extract_status": "success", "contracts": []interface{}{},
		}),
	}
	// 人工恢复行（用户从删除历史点过「重新入库」）必须保留，否则恢复流程失效
	restored := &types.Knowledge{
		ID: "k_restored", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "恢复的合同.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "not_contract", "extract_status": "manual",
			"auto_deleted_count": 1, "contracts": []interface{}{},
		}),
	}
	real := &types.Knowledge{
		ID: "k_real", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "真实合同.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "contract", "extract_status": "success",
			"contracts": []interface{}{
				map[string]interface{}{"contract_no": "HT-9", "contract_name": "采购合同", "party_a": "甲公司"},
			},
		}),
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{inFlight, pendingExtract, blankShell, restored, real}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListContractRecords(ctx, "kb1", types.ContractListFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	ids := make([]string, 0, len(res.Data))
	for _, r := range res.Data {
		ids = append(ids, r.KnowledgeID)
	}
	assert.Contains(t, ids, "k_real", "有效合同必须展示")
	assert.Contains(t, ids, "k_restored", "人工恢复行必须保留（恢复流程依赖它可见）")
	assert.NotContains(t, ids, "k_parsing", "解析中的文件不得生成占位行")
	assert.NotContains(t, ids, "k_pending", "待提取的文件不得生成占位行")
	assert.NotContains(t, ids, "k_blank", "全空空壳文件不得生成占位行")
}

// TestRegulationAndNoticeListsExcludeInFlight 制度/通知公告同构锁定。
func TestRegulationAndNoticeListsExcludeInFlight(t *testing.T) {
	regInFlight := &types.Knowledge{
		ID: "r_parsing", KnowledgeBaseID: "kb1", ParseStatus: "processing",
		FileName: "制度上传中.pdf", CustomMetadata: types.JSON(`{}`),
	}
	regBlank := &types.Knowledge{
		ID: "r_blank", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "空壳制度.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "regulation", "extract_status": "success", "regulations": []interface{}{},
		}),
	}
	regReal := &types.Knowledge{
		ID: "r_real", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "真实制度.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "regulation", "extract_status": "success",
			"regulations": []interface{}{
				map[string]interface{}{"reg_no": "ZD-1", "reg_title": "考勤制度", "reg_type": "行政管理"},
			},
		}),
	}
	regRepo := &taxRateFakeRepo{items: []*types.Knowledge{regInFlight, regBlank, regReal}}
	regSvc := &BusinessExtractService{repo: regRepo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	regRes, err := regSvc.ListRegulationRecords(ctx, "kb1", types.RegulationListFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	regIDs := make([]string, 0, len(regRes.Data))
	for _, r := range regRes.Data {
		regIDs = append(regIDs, r.KnowledgeID)
	}
	assert.Contains(t, regIDs, "r_real")
	assert.NotContains(t, regIDs, "r_parsing", "解析中的制度文件不得占位")
	assert.NotContains(t, regIDs, "r_blank", "全空空壳制度不得占位")

	noticeInFlight := &types.Knowledge{
		ID: "n_parsing", KnowledgeBaseID: "kb1", ParseStatus: "processing",
		FileName: "公告上传中.pdf", CustomMetadata: types.JSON(`{}`),
	}
	noticeReal := &types.Knowledge{
		ID: "n_real", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "真实公告.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "award_punish", "extract_status": "success",
			"records": []interface{}{
				map[string]interface{}{"ap_no": "TZ-1", "ap_title": "表彰通报", "ap_type": "通报"},
			},
		}),
	}
	nRepo := &taxRateFakeRepo{items: []*types.Knowledge{noticeInFlight, noticeReal}}
	nSvc := &BusinessExtractService{repo: nRepo}
	nRes, err := nSvc.ListAwardPunishRecords(ctx, "kb1", types.AwardPunishListFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	nIDs := make([]string, 0, len(nRes.Data))
	for _, r := range nRes.Data {
		nIDs = append(nIDs, r.KnowledgeID)
	}
	assert.Contains(t, nIDs, "n_real")
	assert.NotContains(t, nIDs, "n_parsing", "解析中的公告文件不得占位")
}

// ============ 任务 3：重提编号锁定 ============

// TestNormalizeContractExtractionPreservesExistingNo 锁定重提编号铁律：
// 原文没有编号、但旧记录已有编号（首次生成或用户手填）时，
// 必须沿用旧编号，严禁再走自增序号生成新的 HT-YYYYMMDD-NNN。
func TestNormalizeContractExtractionPreservesExistingNo(t *testing.T) {
	res := &ContractExtractionResult{
		Kind: "contract",
		Contracts: []ContractExtractionItem{
			{ContractName: "补充协议", PartyA: "甲公司", PartyB: "乙公司"}, // 原文无编号
		},
	}
	// 与 handler 的真实顺序一致：先锁定旧编号，再走归一/自动编号
	LockReextractContractNo(res, "HT-20261021-003")
	NormalizeContractExtractionResult(res, 23) // 库里已有 23 个自增编号
	require.Len(t, res.Contracts, 1)
	assert.Equal(t, "HT-20261021-003", res.Contracts[0].ContractNo,
		"重提必须沿用旧记录已有的编号，不得生成 HT-20261022-024")

	// 原文确实有编号 → 采用真实原编号，不被旧编号覆盖
	res2 := &ContractExtractionResult{
		Kind: "contract",
		Contracts: []ContractExtractionItem{
			{ContractNo: "QC-XMIT20251003", ContractName: "框架协议"},
		},
	}
	NormalizeContractExtractionResult(res2, 5)
	assert.Equal(t, "QC-XMIT20251003", res2.Contracts[0].ContractNo,
		"原文有真实编号时必须优先使用原编号")
}

// TestNormalizeContractExtractionGeneratesWhenNoHistory 锁定首次提取：
// 旧记录也没有编号时才生成一次默认编号，且格式稳定。
func TestNormalizeContractExtractionGeneratesWhenNoHistory(t *testing.T) {
	res := &ContractExtractionResult{
		Kind:      "contract",
		Contracts: []ContractExtractionItem{{ContractName: "新合同", PartyA: "甲公司"}},
	}
	NormalizeContractExtractionResult(res, 0)
	require.Len(t, res.Contracts, 1)
	assert.Regexp(t, `^HT-\d{8}-001$`, res.Contracts[0].ContractNo,
		"真正的首次提取且原文无编号时才生成一次默认编号")
}

// TestLockReextractContractNo 锁定「自增编号不得覆盖真实编号」：
// 模型偶尔会把上次的自增编号当原文编号回读，此时若旧记录有真实编号，
// 必须以旧记录为准。
func TestLockReextractContractNo(t *testing.T) {
	res := &ContractExtractionResult{
		Kind: "contract",
		Contracts: []ContractExtractionItem{
			{ContractNo: "HT-20261022-024", ContractName: "补充协议"}, // 回读了自增编号
		},
	}
	LockReextractContractNo(res, "QC-XMIT20251003")
	require.Len(t, res.Contracts, 1)
	assert.Equal(t, "QC-XMIT20251003", res.Contracts[0].ContractNo,
		"旧记录有真实编号时，自增编号必须被替换回真实编号")

	// 旧记录本身就是自增编号 → 原样沿用（不改变用户已见的编号）
	res2 := &ContractExtractionResult{
		Kind:      "contract",
		Contracts: []ContractExtractionItem{{ContractName: "补充协议"}},
	}
	LockReextractContractNo(res2, "HT-20261021-003")
	assert.Equal(t, "HT-20261021-003", res2.Contracts[0].ContractNo)

	// 旧记录无编号 → 不动（交给 Normalize 的首次生成逻辑）
	res3 := &ContractExtractionResult{
		Kind:      "contract",
		Contracts: []ContractExtractionItem{{ContractName: "补充协议"}},
	}
	LockReextractContractNo(res3, "")
	assert.Empty(t, res3.Contracts[0].ContractNo)
}

// ============ 任务 4：自定义字段（保证金）端到端落桶 ============

// TestContractExtractionItemCollectsCustomFieldsIntoBucket 锁定核心根因：
// 用户自定义字段（如「保证金」）不在 Go 结构体的固定字段上，
// 必须经 UnmarshalJSON 收集进 Fields 动态桶，绝不在反序列化时被静默丢弃。
func TestContractExtractionItemCollectsCustomFieldsIntoBucket(t *testing.T) {
	var item ContractExtractionItem
	require.NoError(t, json.Unmarshal([]byte(`{
		"contract_no": "HT-1",
		"contract_name": "员工宿舍租赁合同",
		"party_a": "重庆星达铜业有限公司",
		"party_b": "重庆持恒模具有限公司",
		"保证金": "50000元",
		"租赁期限": "2026-01-01至2026-12-31",
		"fields": {"押金": "10000"}
	}`), &item))

	assert.Equal(t, "HT-1", item.ContractNo)
	assert.Equal(t, "重庆星达铜业有限公司", item.PartyA)
	require.NotNil(t, item.Fields, "自定义字段必须进 Fields 桶")
	assert.Equal(t, "50000元", item.Fields["保证金"], "顶层未知键必须收集进 Fields")
	assert.Equal(t, "2026-01-01至2026-12-31", item.Fields["租赁期限"],
		"结构体没有的顶层键同样进 Fields")
	assert.Equal(t, "10000", item.Fields["押金"], "显式 fields 子对象必须合并进 Fields")
	assert.Empty(t, item.Fields["contract_no"], "标准字段不得进 Fields 桶")
}

// TestValidateGenericExtractionPatchesCustomFieldFromRegex 锁定补洞链路：
// 用户给「保证金」配了正则，模型漏提取时 ValidateGenericExtraction 必须把
// 正则命中的值写进 item.Fields["保证金"]。
func TestValidateGenericExtractionPatchesCustomFieldFromRegex(t *testing.T) {
	// NewGenericRecords 持有切片元素指针，补洞写入必须能在原切片上观察到
	items := []ContractExtractionItem{{
		ContractNo:   "HT-1",
		ContractName: "员工宿舍租赁合同",
		PartyA:       "甲公司",
		PartyB:       "乙公司",
	}}
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "保证金", Enabled: true, Required: true, Regex: `保证金[为是]?\s*([0-9.]+)\s*元`},
		},
	}
	issues, patches := ValidateGenericExtraction(
		NewGenericRecords(items),
		"本合同保证金为 50000 元，乙方应于合同签订后十日内支付。",
		cfg,
	)
	require.NotNil(t, items[0].Fields)
	assert.Equal(t, "50000", items[0].Fields["保证金"],
		"正则补洞必须把值写进 Fields 动态桶")
	assert.Empty(t, issues, "补洞成功不应留下必填告警")
	require.Len(t, patches, 1)
	assert.Equal(t, "保证金", patches[0].Field)
	assert.Equal(t, "50000", patches[0].Value)
}

// TestContractListReturnsCustomFields 锁定列表回显：
// ListContractRecords 必须把 item.Fields 透传给前端（动态列渲染的数据源）。
func TestContractListReturnsCustomFields(t *testing.T) {
	k := &types.Knowledge{
		ID: "k_bond", KnowledgeBaseID: "kb1", ParseStatus: "completed",
		FileName: "员工宿舍租赁合同.pdf",
		CustomMetadata: mustJSON(t, map[string]interface{}{
			"kind": "contract", "extract_status": "success",
			"contracts": []interface{}{
				map[string]interface{}{
					"contract_no": "HT-1", "contract_name": "员工宿舍租赁合同",
					"party_a": "甲公司", "party_b": "乙公司",
					"fields": map[string]interface{}{"保证金": "50000元"},
				},
			},
		}),
	}
	repo := &taxRateFakeRepo{items: []*types.Knowledge{k}}
	svc := &BusinessExtractService{repo: repo}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))

	res, err := svc.ListContractRecords(ctx, "kb1", types.ContractListFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Len(t, res.Data, 1)
	require.NotNil(t, res.Data[0].Fields, "列表必须透传自定义字段")
	assert.Equal(t, "50000元", res.Data[0].Fields["保证金"])
}

// ============ 任务 3（制度/通知公告）：重提编号锁定同构 ============

// TestLockReextractRegulationAndNoticeNo 制度与通知公告必须遵循同一套编号铁律。
func TestLockReextractRegulationAndNoticeNo(t *testing.T) {
	reg := &RegulationExtractionResult{
		Kind:        "regulation",
		Regulations: []types.RegulationExtractionItem{{RegName: "考勤制度"}}, // 原文无编号
	}
	LockReextractRegulationNo(reg, "ZD-20261021-003")
	require.Len(t, reg.Regulations, 1)
	assert.Equal(t, "ZD-20261021-003", reg.Regulations[0].RegNo,
		"制度重提必须沿用旧编号，不得生成新的 ZD-YYYYMMDD-NNN")

	notice := &AwardPunishExtractionResult{
		Kind:    "award_punish",
		Records: []types.AwardPunishExtractionItem{{ApTitle: "表彰通报"}},
	}
	LockReextractNoticeNo(notice, "星达行政字〔2026〕11号")
	require.Len(t, notice.Records, 1)
	assert.Equal(t, "星达行政字〔2026〕11号", notice.Records[0].ApNo,
		"公告重提必须沿用旧文号，不得生成新的 TZ-YYYYMMDD-NNN")

	// 模型回读了上次的自动编号 → 仍以旧记录为准
	notice2 := &AwardPunishExtractionResult{
		Kind:    "award_punish",
		Records: []types.AwardPunishExtractionItem{{ApNo: "TZ-20261022-009"}},
	}
	LockReextractNoticeNo(notice2, "星达行政字〔2026〕11号")
	assert.Equal(t, "星达行政字〔2026〕11号", notice2.Records[0].ApNo)
}

// ============ 任务 4（制度/公告）：自定义字段同构落桶 ============

func TestRegulationAndNoticeItemsCollectCustomFields(t *testing.T) {
	var reg types.RegulationExtractionItem
	require.NoError(t, json.Unmarshal([]byte(`{
		"reg_no": "ZD-1", "reg_name": "考勤制度", "reg_type": "行政管理",
		"适用范围": "全公司", "fields": {"审批人": "张三"}
	}`), &reg))
	require.NotNil(t, reg.Fields)
	assert.Equal(t, "全公司", reg.Fields["适用范围"])
	assert.Equal(t, "张三", reg.Fields["审批人"])
	assert.Empty(t, reg.Fields["reg_no"], "标准字段不得进 Fields 桶")

	var notice types.AwardPunishExtractionItem
	require.NoError(t, json.Unmarshal([]byte(`{
		"ap_no": "TZ-1", "ap_title": "表彰通报", "ap_type": "通报",
		"奖金金额": "5000"
	}`), &notice))
	require.NotNil(t, notice.Fields)
	assert.Equal(t, "5000", notice.Fields["奖金金额"])
}

// ============ 任务 5：甲乙方按原文标签锚点提取，杜绝颠倒 ============

// TestReconcileContractPartiesPrefersExplicitLabels 锁定主回归：
// 原文明确写了「甲方：A / 乙方：B」时，必须严格按标签锚点提取，
// 模型把双方填反了也要被纠正过来。
func TestReconcileContractPartiesPrefersExplicitLabels(t *testing.T) {
	text := `厂房租赁协议书
甲方：重庆星达铜业有限公司
乙方：重庆持恒模具有限公司
……`
	items := []ContractExtractionItem{{
		ContractNo: "HT-1",
		// 模型填反了（补充协议里写的是出租方/承租方，模型按主合同角色猜）
		PartyA: "重庆持恒模具有限公司",
		PartyB: "重庆星达铜业有限公司",
	}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyA, "必须按原文「甲方」锚点纠正")
	assert.Equal(t, "重庆持恒模具有限公司", items[0].PartyB, "必须按原文「乙方」锚点纠正")
}

// TestReconcileContractPartiesHandlesRoleLabels 补充协议常见形态：
// 「出租方（甲方）：A / 承租方（乙方）：B」——括号里的甲乙方才是权威标签。
func TestReconcileContractPartiesHandlesRoleLabels(t *testing.T) {
	text := `补充协议
出租方（甲方）：重庆星达铜业有限公司
承租方（乙方）：重庆持恒模具有限公司`
	items := []ContractExtractionItem{{ContractNo: "HT-2", PartyA: "", PartyB: ""}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyA)
	assert.Equal(t, "重庆持恒模具有限公司", items[0].PartyB)
}

// TestReconcileContractPartiesRoleOnlyLabels 补充协议**成对**写出
// 「出租方：A / 承租方：B」且全文没有甲乙方标签时，按合同角色约定映射。
func TestReconcileContractPartiesRoleOnlyLabels(t *testing.T) {
	text := `厂房租赁协议之补充协议
出租方：重庆星达铜业有限公司
承租方：重庆持恒模具有限公司`
	items := []ContractExtractionItem{{ContractNo: "HT-3", PartyA: "", PartyB: ""}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyA, "成对出现时出租方映射为甲方")
	assert.Equal(t, "重庆持恒模具有限公司", items[0].PartyB, "成对出现时承租方映射为乙方")
}

// TestReconcileContractPartiesSingleRoleLabelDoesNotOverride 锁定关键边界：
// 只有**单边**角色标签（如补充合同只写「承租单位名称：X」）时不得覆盖模型值。
//
// 真机取证：补充合同原文只写「承租单位名称：重庆持睿」，而主合同是
// 「甲方：星达 乙方：持恒」——承租方根本不是主合同任何一方。
// 半份证据不能推翻完整提取，否则会把主体改错。
func TestReconcileContractPartiesSingleRoleLabelDoesNotOverride(t *testing.T) {
	text := `员工宿舍租赁补充合同
承租单位名称：重庆持睿汽车部件有限公司
增租集体宿舍410一间`
	items := []ContractExtractionItem{{
		ContractNo: "HT-7",
		PartyA:     "重庆持睿汽车部件有限公司",
		PartyB:     "重庆星达铜业有限公司",
	}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "重庆持睿汽车部件有限公司", items[0].PartyA, "单边角色标签不得覆盖模型值")
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyB)
}

// TestReconcileContractPartiesFallsBackToDescription 原文完全没有角色标签时，
// 用知识库摘要里**成对**的甲乙方标签兜底（「A与B签订」不是标签，不能用）。
func TestReconcileContractPartiesFallsBackToDescription(t *testing.T) {
	// 摘要里的公司名就是「星达铜业」「持恒模具」（知识库摘要惯用简称），
	// 断言必须按摘要原文写，不能拿全称来比。
	desc := "该合同为星达铜业（甲方）与持恒模具（乙方）签订的厂房租赁补充协议。"
	items := []ContractExtractionItem{{ContractNo: "HT-4", PartyA: "", PartyB: ""}}
	ReconcileContractParties("双方经友好协商，就增租宿舍事宜达成如下补充条款……", desc, items)
	assert.Equal(t, "星达铜业", items[0].PartyA)
	assert.Equal(t, "持恒模具", items[0].PartyB)
}

// TestReconcileContractPartiesIgnoresNarrativeDescription 摘要里只有叙述性
// 「A与B签订…」时不得据此覆盖（那不是甲乙方标签）。
func TestReconcileContractPartiesIgnoresNarrativeDescription(t *testing.T) {
	desc := "重庆持睿汽车部件有限公司与重庆星达铜业有限公司签订员工宿舍租赁补充合同。"
	items := []ContractExtractionItem{{ContractNo: "HT-8", PartyA: "重庆持睿汽车部件有限公司", PartyB: "重庆星达铜业有限公司"}}
	ReconcileContractParties("双方经友好协商达成如下条款……", desc, items)
	assert.Equal(t, "重庆持睿汽车部件有限公司", items[0].PartyA, "叙述性摘要不得覆盖模型值")
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyB)
}

// TestReconcileContractPartiesKeepsModelValueWhenNoAnchor 没有任何锚点时
// 保持模型结果不变（绝不臆造）。
func TestReconcileContractPartiesKeepsModelValueWhenNoAnchor(t *testing.T) {
	text := "本合同由双方友好协商签订，具体条款如下……"
	items := []ContractExtractionItem{{ContractNo: "HT-5", PartyA: "甲公司", PartyB: "乙公司"}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "甲公司", items[0].PartyA)
	assert.Equal(t, "乙公司", items[0].PartyB)
}

// TestReconcileContractPartiesSubstringSafety 只整值锚定：
// 公司名是子串关系时不能互相覆盖（如「重庆持恒模具有限公司」vs
// 「重庆持恒模具有限公司分公司」）。
func TestReconcileContractPartiesSubstringSafety(t *testing.T) {
	text := `甲方：重庆星达铜业有限公司
乙方：重庆持恒模具有限公司`
	items := []ContractExtractionItem{{ContractNo: "HT-6", PartyA: "重庆持恒模具有限公司分公司", PartyB: ""}}
	ReconcileContractParties(text, "", items)
	assert.Equal(t, "重庆星达铜业有限公司", items[0].PartyA, "原文锚点优先")
	assert.Equal(t, "重庆持恒模具有限公司", items[0].PartyB)
}
