package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 回归（2026-10-22）：合同/制度/通知公告分类改名后存量记录掉进「其它*」桶。
//
// 真机事故：把「服务合同」改名为「商务合同」→「商务合同」卡片 0 份，
// 原记录全部落进「其它合同（2 份）」。
//
// 根因：propagateCategoryRename 对非发票 scope 只替换 custom_metadata.category，
// 而**真实库里根本没有这个顶层键**——合同/制度的权威键是 fleet_cert_type，
// 分类还同时冗余在 contracts[].contract_type / regulations[].regulation_type /
// records[].ap_type 里。改名一行都没传播，前端「其它合同」又是排除式桶
// （不在任何启用分类即归桶），于是旧名记录既不属于新分类也没被任何启用分类命中。
//
// 以下测试锁定修复契约：三模块改名必须在同一事务内把**顶层 fleet_cert_type
// 与嵌套数组内的分类键**整体改写，只改整值、不造键、不动别的分类。

// businessContractMetaForRename 造一份合同文件 metadata，形态与真机一致：
// 顶层 fleet_cert_type + contracts[].contract_type，**没有** category 顶层键。
func businessContractMetaForRename(cat string) types.JSON {
	return types.JSON(mustJSON(map[string]interface{}{
		"kind":            "contract",
		"extract_status":  "success",
		"fleet_cert_type": cat,
		"fleet_scope":     "contract",
		"contracts": []interface{}{
			map[string]interface{}{
				"contract_no":   "HT-20261022-001",
				"contract_name": "技术咨询协议",
				"contract_type": cat,
				"party_a":       "甲方公司",
				"party_b":       "乙方公司",
				"amount":        12000,
				"page":          1,
			},
		},
	}))
}

// businessRenameMetaKeys 取出业务 metadata 里与分类名相关的键值。
type businessRenameMetaKeys struct {
	FleetCertType string `json:"fleet_cert_type"`
	Category      string `json:"category"`
	Contracts     []struct {
		ContractNo   string `json:"contract_no"`
		ContractType string `json:"contract_type"`
		Category     string `json:"category"`
		PartyA       string `json:"party_a"`
		Amount       *int   `json:"amount"`
	} `json:"contracts"`
}

// TestBusinessRenameContractCategoryCarriesNestedItems 是本次事故的主回归：
// 合同分类改名必须同时改写顶层 fleet_cert_type 与 contracts[].contract_type。
func TestBusinessRenameContractCategoryCarriesNestedItems(t *testing.T) {
	db := fleetRenameTestDB(t)
	const oldName, newName = "服务合同", "商务合同"

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-svc", TenantID: 0, Scope: "contract",
		Name: oldName, Enabled: true, SortOrder: 1, Subs: []types.FleetCategorySub{},
	}).Error)

	k1 := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", Title: "服务合同1", FileName: "a.pdf", CustomMetadata: businessContractMetaForRename(oldName)}
	// k2 属于别的分类，绝不能被动
	k2 := types.Knowledge{ID: "k2", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", Title: "租赁合同1", FileName: "b.pdf", CustomMetadata: businessContractMetaForRename("租赁合同")}
	require.NoError(t, db.Create(&k1).Error)
	require.NoError(t, db.Create(&k2).Error)

	// 提取配置按 kb × scope × cert_type 建键，改名必须跟随
	require.NoError(t, db.Create(&types.KbExtractConfig{ID: "cfg1", TenantID: 1, KnowledgeBaseID: "kb1", Scope: "contract", CertType: oldName, Fields: []types.ExtractFieldConfig{}, Enabled: true, PromptTemplate: "tmpl-svc"}).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-svc", `{"name":"`+newName+`","enabled":true}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	raw := readRawMetadata(t, db, "k1")
	var got businessRenameMetaKeys
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, newName, got.FleetCertType, "顶层 fleet_cert_type 必须同步改写")
	require.Len(t, got.Contracts, 1)
	require.Equal(t, newName, got.Contracts[0].ContractType, "contracts[].contract_type 必须同步改写")
	// 合同自身字段不受影响
	require.Equal(t, "HT-20261022-001", got.Contracts[0].ContractNo)
	require.Equal(t, "甲方公司", got.Contracts[0].PartyA)
	require.NotNil(t, got.Contracts[0].Amount)
	require.Equal(t, 12000, *got.Contracts[0].Amount)
	require.NotContains(t, string(raw), oldName, "不得残留旧分类名")

	// 别的分类分毫未动
	other := string(readRawMetadata(t, db, "k2"))
	require.Contains(t, other, "租赁合同")
	require.NotContains(t, other, newName)

	// 提取配置跟随改名
	var cfgAfter types.KbExtractConfig
	require.NoError(t, db.Where("id = ?", "cfg1").First(&cfgAfter).Error)
	require.Equal(t, newName, cfgAfter.CertType)
	require.Equal(t, "tmpl-svc", cfgAfter.PromptTemplate, "改名不得丢规则底稿")
}

// TestBusinessRenameRegulationCategoryCarriesNestedItems 制度模块同构：
// 顶层 fleet_cert_type + regulations[].regulation_type。
func TestBusinessRenameRegulationCategoryCarriesNestedItems(t *testing.T) {
	db := fleetRenameTestDB(t)
	const oldName, newName = "行政管理", "综合管理"

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-reg", TenantID: 0, Scope: "regulation",
		Name: oldName, Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	k := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(map[string]interface{}{
		"kind":            "regulation",
		"extract_status":  "success",
		"fleet_cert_type": oldName,
		"regulations": []interface{}{
			map[string]interface{}{"reg_no": "ZD-001", "reg_title": "考勤制度", "regulation_type": oldName},
		},
	}))}
	require.NoError(t, db.Create(&k).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-reg", `{"name":"`+newName+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	raw := string(readRawMetadata(t, db, "k1"))
	require.Contains(t, raw, newName)
	require.NotContains(t, raw, oldName)
}

// TestBusinessRenameNoticeCategoryCarriesApType 通知公告模块：
// 顶层无 fleet_cert_type（真机形态），分类只冗余在 records[].ap_type。
func TestBusinessRenameNoticeCategoryCarriesApType(t *testing.T) {
	db := fleetRenameTestDB(t)
	const oldName, newName = "通报", "表彰通报"

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-notice", TenantID: 0, Scope: "award_punish",
		Name: oldName, Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	k := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(map[string]interface{}{
		"kind":           "award_punish",
		"extract_status": "success",
		"records": []interface{}{
			map[string]interface{}{"ap_no": "TZ-001", "ap_title": "关于xxx的通报", "ap_type": oldName},
		},
	}))}
	require.NoError(t, db.Create(&k).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-notice", `{"name":"`+newName+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 逐键断言（不能对整个 raw 做 NotContains("通报")——新名「表彰通报」
	// 本身含旧名子串，会误判）。
	var got struct {
		Records []struct {
			ApNo    string `json:"ap_no"`
			ApTitle string `json:"ap_title"`
			ApType  string `json:"ap_type"`
		} `json:"records"`
	}
	require.NoError(t, json.Unmarshal(readRawMetadata(t, db, "k1"), &got))
	require.Len(t, got.Records, 1)
	require.Equal(t, newName, got.Records[0].ApType, "records[].ap_type 必须同步改写")
	require.Equal(t, "TZ-001", got.Records[0].ApNo, "文号不得被改写")
	require.Equal(t, "关于xxx的通报", got.Records[0].ApTitle, "标题不得被改写")
}

// TestBusinessRenameCategorySubstringSafetyBusiness 锁定替换粒度：
// 只整值相等才替换，绝不把「服务合同2」误改成「商务合同2」。
func TestBusinessRenameCategorySubstringSafetyBusiness(t *testing.T) {
	db := fleetRenameTestDB(t)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-short", TenantID: 0, Scope: "contract",
		Name: "服务合同", Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	// 一条值恰为旧名，另一条值是「服务合同2」（仅子串命中）
	exact := types.Knowledge{ID: "k-exact", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: businessContractMetaForRename("服务合同")}
	longer := types.Knowledge{ID: "k-long", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: businessContractMetaForRename("服务合同2")}
	require.NoError(t, db.Create(&exact).Error)
	require.NoError(t, db.Create(&longer).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-short", `{"name":"商务合同"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	exactRaw := string(readRawMetadata(t, db, "k-exact"))
	require.Contains(t, exactRaw, "商务合同")
	require.NotContains(t, exactRaw, "服务合同")

	longRaw := string(readRawMetadata(t, db, "k-long"))
	require.Contains(t, longRaw, "服务合同2", "仅子串命中的值绝不能被误改")
	require.NotContains(t, longRaw, "商务合同")
}

// TestBusinessRenameDoesNotInventKeys 锁定「不造键」：
// 存量行只有 contracts[].contract_type、没有顶层 fleet_cert_type 时，
// 传播只改已有键，不得凭空写入 fleet_cert_type（否则无分类文件被误打上分类）。
func TestBusinessRenameDoesNotInventKeys(t *testing.T) {
	db := fleetRenameTestDB(t)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-nokey", TenantID: 0, Scope: "contract",
		Name: "服务合同", Enabled: true, Subs: []types.FleetCategorySub{},
	}).Error)
	k := types.Knowledge{ID: "k1", TenantID: 1, KnowledgeBaseID: "kb1", ParseStatus: "completed", CustomMetadata: types.JSON(mustJSON(map[string]interface{}{
		"kind":           "contract",
		"extract_status": "success",
		"contracts": []interface{}{
			map[string]interface{}{"contract_no": "HT-1", "contract_type": "服务合同"},
		},
	}))}
	require.NoError(t, db.Create(&k).Error)

	router := fleetRenameRouter(t, db)
	rec := putFleetCategoryRename(t, router, "cat-nokey", `{"name":"商务合同"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got businessRenameMetaKeys
	require.NoError(t, json.Unmarshal(readRawMetadata(t, db, "k1"), &got))
	require.Empty(t, got.FleetCertType, "不得凭空造 fleet_cert_type 键")
	require.Empty(t, got.Category, "不得凭空造 category 键")
	require.Len(t, got.Contracts, 1)
	require.Equal(t, "商务合同", got.Contracts[0].ContractType)
}

// ============ 改名后 reconcile 不得把旧名分类造回来 ============

// TestReconcileContractCategoryKeepsRenamedBuiltin 锁定启动协调逻辑：
// 用户把内置「服务合同」改名为「商务合同」后，重启触发的
// reconcileContractCategories 必须**认不出旧名就跳过**，绝不能按硬编码的
// wanted 列表再把「服务合同」建回来 —— 否则每次重启都会在「商务合同」
// 旁边复活一个 0 份的「服务合同」，KPI 卡片与筛选器全被污染。
func TestReconcileContractCategoryKeepsRenamedBuiltin(t *testing.T) {
	db := fleetRenameTestDB(t)
	// 用户改名后的现状：商务合同（builtin_key=contract-service）
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "seed-contract-contract-service", TenantID: 0, Scope: "contract",
		BuiltinKey: "contract-service", Name: "商务合同",
		Enabled: true, SortOrder: 0, Subs: []types.FleetCategorySub{},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "seed-contract-contract-lease", TenantID: 0, Scope: "contract",
		BuiltinKey: "contract-lease", Name: "租赁合同",
		Enabled: true, SortOrder: 1, Subs: []types.FleetCategorySub{},
	}).Error)

	h := NewFleetHandler(db)
	h.reconcileContractCategories(context.Background())

	var names []string
	require.NoError(t, db.Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND deleted_at IS NULL", "contract").
		Order("sort_order ASC").Pluck("name", &names).Error)
	assert.Equal(t, []string{"商务合同", "租赁合同"}, names,
		"reconcile 不得把用户改掉的旧名分类重新建回来")

	// 旧名必须彻底不存在
	var stale int64
	require.NoError(t, db.Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND name = ?", "contract", "服务合同").
		Count(&stale).Error)
	assert.Zero(t, stale, "旧名「服务合同」不得被 reconcile 复活")
}

// TestReconcileContractCategoryLegacyRowNotRevived 真机同构场景：
// 存量行的 ID 不是规范的 `seed-contract-<key>`（真机是历史遗留的 `seed-contract`），
// builtin_key 已锚定。此时按 name 查旧名必然查空 → 旧实现会用固定 ID 再建一个
// 「服务合同」，与已改名的「商务合同」并存（每次重启复现，KPI 计数归零）。
func TestReconcileContractCategoryLegacyRowNotRevived(t *testing.T) {
	db := fleetRenameTestDB(t)
	// 真机同构：ID=seed-contract（早期脏数据复用），builtin_key 已改锚，名字已被用户改过
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "seed-contract", TenantID: 0, Scope: "contract",
		BuiltinKey: "contract-service", Name: "商务合同",
		Enabled: true, SortOrder: 0, Subs: []types.FleetCategorySub{},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "seed-contract-contract-lease", TenantID: 0, Scope: "contract",
		BuiltinKey: "contract-lease", Name: "租赁合同",
		Enabled: true, SortOrder: 1, Subs: []types.FleetCategorySub{},
	}).Error)

	h := NewFleetHandler(db)
	h.reconcileContractCategories(context.Background())

	var names []string
	require.NoError(t, db.Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND deleted_at IS NULL", "contract").
		Order("sort_order ASC").Pluck("name", &names).Error)
	assert.Equal(t, []string{"商务合同", "租赁合同"}, names,
		"历史遗留 ID 的行也不得被旧名分类复活")

	// 行数不得增长（严禁重复创建）
	var total int64
	require.NoError(t, db.Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ?", "contract").Count(&total).Error)
	assert.Equal(t, int64(2), total, "reconcile 不得新增重复分类行")
}
