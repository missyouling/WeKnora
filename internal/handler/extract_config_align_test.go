package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// 六同步（2026-10-21）后端侧回归：`fleet_categories.subs` 是字段集合 / 顺序 / 类型
// 的唯一权威源。用户在「字段定义」里删除、改名（摘要→摘要2）、改类型（text→array）
// 之后，`GET extract-config` 返回的字段必须与之严格对齐：
//   · 删掉的字段 → 不再返回；
//   · 改名 → 返回新名，且原 desc/rule/regex 不丢；
//   · 改类型 → type 跟随 subs.data_type；
//   · 顺序 → 与 subs 顺序一致。

func sixWaySyncDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.FleetCategory{}, &types.KbExtractConfig{}))
	return db
}

// 分类 subs：把「摘要 summary」改名为「摘要2」并把类型改成数组，
// 同时删掉「甲方 party_a」、禁用「合同名称 contract_name」。
func sixWaySyncCategory() types.FleetCategory {
	return types.FleetCategory{
		ID:       "cat-contract-1",
		TenantID: 0,
		Scope:    "contract",
		Name:     "服务合同",
		Enabled:  true,
		Subs: []types.FleetCategorySub{
			{Name: "contract_no", Enabled: true, IsDefault: true, DataType: "text"},
			// 用户关掉了「合同名称」：必须从提取规则里消失
			{Name: "contract_name", Label: "合同名称", Enabled: false, IsDefault: true, DataType: "text"},
			{Name: "sign_date", Enabled: true, IsDefault: true, DataType: "date"},
			// 用户改名 + 改类型
			{Name: "summary", Label: "摘要2", Enabled: true, IsDefault: false, DataType: "array"},
			// 存量库里的系统列残留：应对齐时被剥离
			{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
		},
	}
}

// 旧的提取配置：字段仍叫「摘要」、类型 text，且有用户配的口径。
func sixWaySyncConfig() *types.KbExtractConfig {
	return &types.KbExtractConfig{
		ID:              "cfg-1",
		TenantID:        0,
		KnowledgeBaseID: "kb1",
		Scope:           "contract",
		CertType:        "服务合同",
		Enabled:         true,
		Version:         3,
		Fields: []types.ExtractFieldConfig{
			{Name: "contract_no", Label: "合同编号", Type: "string", Enabled: true, Desc: "合同编号", Rule: "取合同编号栏"},
			{Name: "contract_name", Label: "合同名称", Type: "string", Enabled: true, Desc: "合同名称"},
			{Name: "party_a", Label: "甲方", Type: "string", Enabled: true, Desc: "甲方名称", Rule: "取甲方栏"},
			{Name: "summary", Label: "摘要", Type: "string", Enabled: true, Desc: "合同要点", Rule: "提取正文摘要", Regex: `\d+`},
			{Name: "fileName", Label: "文件名称", Type: "string", Enabled: true, Desc: "源文件名"},
		},
	}
}

func TestAlignExtractConfigWithRenamedAndRetypedField(t *testing.T) {
	db := sixWaySyncDB(t)
	require.NoError(t, db.Create(ptr(sixWaySyncCategory())).Error)
	h := &BusinessExtractHandler{db: db}

	aligned := h.alignExtractConfigWithCategory(context.Background(), sixWaySyncConfig(), "contract", "服务合同")
	require.NotNil(t, aligned)

	names := make([]string, 0, len(aligned.Fields))
	for _, f := range aligned.Fields {
		names = append(names, f.Name)
	}
	// 集合 / 顺序 = subs（禁用项与系统列都不进）
	assert.Equal(t, []string{"contract_no", "sign_date", "summary"}, names,
		"字段集合与顺序必须严格等于 subs 的启用项")

	byName := map[string]types.ExtractFieldConfig{}
	for _, f := range aligned.Fields {
		byName[f.Name] = f
	}

	// 改名：label 跟随 subs；desc/rule/regex 迁移过来不丢
	summary := byName["summary"]
	assert.Equal(t, "摘要2", summary.Label, "字段名必须跟随 subs.label 改名")
	assert.Equal(t, "array", summary.Type, "类型必须跟随 subs.data_type 改类型")
	assert.Equal(t, "合同要点", summary.Desc, "改名不得丢失字段描述")
	assert.Equal(t, "提取正文摘要", summary.Rule, "改名不得丢失提取规则")
	assert.Equal(t, `\d+`, summary.Regex, "改名不得丢失正则兜底")

	// 删除的字段不得出现
	assert.NotContains(t, names, "party_a", "字段定义里删除的字段必须从提取规则移除")
	// 禁用的字段不得出现
	assert.NotContains(t, names, "contract_name", "禁用字段必须从提取规则移除")
	// 系统列不得出现
	assert.NotContains(t, names, "fileName", "fileName 是列表系统列，不进提取规则")
}

func TestAlignExtractConfigKeepsUserRulesForUnchangedFields(t *testing.T) {
	db := sixWaySyncDB(t)
	require.NoError(t, db.Create(ptr(sixWaySyncCategory())).Error)
	h := &BusinessExtractHandler{db: db}

	aligned := h.alignExtractConfigWithCategory(context.Background(), sixWaySyncConfig(), "contract", "服务合同")
	require.NotNil(t, aligned)

	var contractNo *types.ExtractFieldConfig
	for i := range aligned.Fields {
		if aligned.Fields[i].Name == "contract_no" {
			contractNo = &aligned.Fields[i]
		}
	}
	require.NotNil(t, contractNo)
	assert.Equal(t, "取合同编号栏", contractNo.Rule, "未改动字段的用户规则必须原样保留")
	assert.True(t, contractNo.Enabled)
}

func TestAlignExtractConfigFallsBackWhenCategoryMissing(t *testing.T) {
	db := sixWaySyncDB(t)
	h := &BusinessExtractHandler{db: db}
	cfg := sixWaySyncConfig()

	aligned := h.alignExtractConfigWithCategory(context.Background(), cfg, "contract", "不存在的分类")
	// 分类不存在时原样返回，绝不能把用户配置清空
	assert.Equal(t, cfg.Fields, aligned.Fields)
}

func TestAlignExtractConfigIgnoresCamelCaseFieldNames(t *testing.T) {
	// 公告/制度分类的 subs 用 camelCase（apNo），提取配置可能存 snake（ap_no），
	// 两种写法都要能命中同一行，否则改名后口径会丢。
	db := sixWaySyncDB(t)
	cat := types.FleetCategory{
		ID: "cat-notice-1", TenantID: 0, Scope: "award_punish", Name: "通知", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "apNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "summary", Label: "摘要2", Enabled: true, DataType: "array"},
		},
	}
	require.NoError(t, db.Create(&cat).Error)
	cfg := &types.KbExtractConfig{
		ID: "cfg-2", Scope: "award_punish", CertType: "通知", Enabled: true, Version: 1,
		Fields: []types.ExtractFieldConfig{
			{Name: "ap_no", Label: "文号", Type: "string", Enabled: true, Desc: "发文编号"},
			{Name: "summary", Label: "摘要", Type: "string", Enabled: true, Desc: "要点"},
		},
	}
	h := &BusinessExtractHandler{db: db}
	aligned := h.alignExtractConfigWithCategory(context.Background(), cfg, "award_punish", "通知")
	require.NotNil(t, aligned)
	require.Len(t, aligned.Fields, 2)
	assert.Equal(t, "apNo", aligned.Fields[0].Name, "名字以 subs 为准")
	assert.Equal(t, "发文编号", aligned.Fields[0].Desc, "snake_case 旧配置的 desc 必须能迁移")
	assert.Equal(t, "摘要2", aligned.Fields[1].Label)
	assert.Equal(t, "array", aligned.Fields[1].Type)
}

// ---- 列表系统列（fileName）不得进入 subs/提取字段 ----

func TestSeedSubsExcludeListSystemColumns(t *testing.T) {
	for name, subs := range map[string][]types.FleetCategorySub{
		"contract":   contractSeedSubs(),
		"regulation": regulationSeedSubs(),
		"notice":     noticeSeedSubs(),
	} {
		for _, s := range subs {
			assert.False(t, isBusinessListSystemKey(s.Name),
				"%s seed 不得包含列表系统列 %s（会在抽屉/提取规则里变成幽灵字段）", name, s.Name)
		}
	}
}

func TestMergeSeedSubsStripsLegacyFileName(t *testing.T) {
	legacyContract := []types.FleetCategorySub{
		{Name: "contract_no", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
	}
	for _, s := range mergeContractSeedSubs(legacyContract) {
		assert.NotEqual(t, "fileName", s.Name, "存量 subs 里的 fileName 必须在合并时剥离")
	}

	legacyNotice := []types.FleetCategorySub{
		{Name: "apNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
	}
	for _, s := range mergeNoticeSeedSubs(legacyNotice) {
		assert.NotEqual(t, "fileName", s.Name, "公告存量 subs 里的 fileName 必须在合并时剥离")
	}
}

func TestStripListSystemSubsKeepsBusinessFields(t *testing.T) {
	in := []types.FleetCategorySub{
		{Name: "contract_no"}, {Name: "fileName"}, {Name: "summary"},
	}
	out := stripListSystemSubs(in)
	require.Len(t, out, 2)
	assert.Equal(t, "contract_no", out[0].Name)
	assert.Equal(t, "summary", out[1].Name)
	// 幂等：再剥一次不变化
	assert.Equal(t, out, stripListSystemSubs(out))
	// 空输入不 panic
	assert.Empty(t, stripListSystemSubs(nil))
}

func ptr[T any](v T) *T { return &v }

// ---- 存量清理：把 fileName 从提取规则里移除（幂等） ----

func TestStripListSystemFieldsFromConfigs(t *testing.T) {
	db := sixWaySyncDB(t)
	require.NoError(t, db.Create(&types.KbExtractConfig{
		ID: "cfg-with-file", KnowledgeBaseID: "kb1", Scope: "contract", CertType: "",
		Enabled: true, Version: 1,
		Fields: []types.ExtractFieldConfig{
			{Name: "contract_no", Type: "string", Enabled: true, Desc: "合同编号"},
			{Name: "fileName", Label: "文件名称", Type: "string", Enabled: true, Desc: "源文件名"},
			{Name: "summary", Type: "string", Enabled: true, Desc: "要点"},
		},
	}).Error)
	// 不含系统列的行必须原样保留（连 updated_at 都不该被推进）
	clean := &types.KbExtractConfig{
		ID: "cfg-clean", KnowledgeBaseID: "kb1", Scope: "regulation", CertType: "",
		Enabled: true, Version: 1,
		Fields: []types.ExtractFieldConfig{{Name: "regNo", Type: "string", Enabled: true}},
	}
	require.NoError(t, db.Create(clean).Error)

	h := &BusinessExtractHandler{db: db}
	h.StripListSystemFieldsFromConfigs(context.Background())

	var patched types.KbExtractConfig
	require.NoError(t, db.First(&patched, "id = ?", "cfg-with-file").Error)
	names := make([]string, 0, len(patched.Fields))
	for _, f := range patched.Fields {
		names = append(names, f.Name)
	}
	assert.Equal(t, []string{"contract_no", "summary"}, names, "fileName 必须被移除，其它字段原样保留")

	var untouched types.KbExtractConfig
	require.NoError(t, db.First(&untouched, "id = ?", "cfg-clean").Error)
	require.Len(t, untouched.Fields, 1, "不含系统列的行不得被改动")

	// 幂等：第二次执行不应再产生写库（Fields 不变）
	var again types.KbExtractConfig
	require.NoError(t, db.First(&again, "id = ?", "cfg-with-file").Error)
	h.StripListSystemFieldsFromConfigs(context.Background())
	var third types.KbExtractConfig
	require.NoError(t, db.First(&third, "id = ?", "cfg-with-file").Error)
	assert.Equal(t, again.Fields, third.Fields)
}
