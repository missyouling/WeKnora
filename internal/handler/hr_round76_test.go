// 第 76 轮回归：
//  1. 租户档案迁入公司设置（别名 / 编码 / 分摊方式 / 联系人 / 电话）且分摊子项零丢失
//  2. 费用清单租户列表返回别名（别名是全局简称唯一真相源）
//  3. 保险单重配：交强险 30 + 商业险 24 + 投保险别 7 + 险种类别 7，
//     提取规则与高级 Prompt 落库，存量保单旧字段映射补齐
//  4. 证照改名：doc_type 与 sub_category 同事务级联，存量记录 0 丢失
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

func hrRound76DB(t *testing.T) *gorm.DB {
	t.Helper()
	db := hrRound59DB(t)
	require.NoError(t, db.AutoMigrate(
		&HROrgUnit{}, &HRDepartment{}, &HRCompany{},
		&types.BillingTenant{}, &types.BillingTenantItem{},
		&types.FleetRecord{}, &types.KbExtractConfig{}, &types.KnowledgeBase{},
	))
	return db
}

func TestRound76MigrateTenantToOrg(t *testing.T) {
	db := hrRound76DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HROrgUnit{
		ID: "u-cuirui", TenantID: 1, Name: "持睿汽车部件有限公司", Code: "Z001", SortOrder: 2,
	}).Error)
	require.NoError(t, db.Create(&HROrgUnit{
		ID: "u-xingda", TenantID: 1, Name: "重庆星达铜业有限公司", Code: "01", SortOrder: 1,
	}).Error)
	require.NoError(t, db.Create(&types.BillingTenant{
		ID: "bt-cuirui", TenantID: 1, Name: "持睿汽车", TenantNo: "Z001",
		AllocationMode: "按比例分摊", Contact: "李锶雨", Phone: "13800000000",
	}).Error)
	// 分摊子项：迁移绝不能动它（金额波动的最大风险源）
	items := []types.BillingTenantItem{
		{ID: "it1", BillingTenantID: "bt-cuirui", Category: "市场化购电费", ItemKey: "市场化购电费|x", ItemName: "零售交易电费", Enabled: true, SourceCategory: "市电账单"},
		{ID: "it2", BillingTenantID: "bt-cuirui", Category: "输配电费", ItemKey: "输配电费|y", ItemName: "输配电量", Enabled: true, SourceCategory: "市电账单"},
	}
	for _, it := range items {
		require.NoError(t, db.Create(&it).Error)
	}

	h := &HRHandler{db: db}
	updated, linked := h.hrMigrateTenantToOrg76(ctx, 1)
	require.GreaterOrEqual(t, updated, 2, "两个单位都要参与档案对齐（星达补别名）")
	require.Equal(t, 1, linked, "持睿必须与 billing_tenant 绑定")
	require.NoError(t, h.hrSyncOrgTenants76(ctx, 1))

	var cuirui HROrgUnit
	require.NoError(t, db.Where("id = ?", "u-cuirui").First(&cuirui).Error)
	require.Equal(t, "持睿汽车", cuirui.Alias, "别名是租户简称唯一真相源")
	require.True(t, cuirui.IsTenant)
	require.Equal(t, "Z001", cuirui.TenantNo)
	require.Equal(t, "按比例分摊", cuirui.AllocationMode)
	require.Equal(t, "李锶雨", cuirui.Contact)
	require.Equal(t, "13800000000", cuirui.Phone)
	require.Equal(t, "bt-cuirui", cuirui.BillingTenantID)

	var xingda HROrgUnit
	require.NoError(t, db.Where("id = ?", "u-xingda").First(&xingda).Error)
	require.Equal(t, "重庆星达", xingda.Alias, "星达默认别名")
	require.False(t, xingda.IsTenant)

	var cnt int64
	require.NoError(t, db.Model(&types.BillingTenantItem{}).Where("billing_tenant_id = ?", "bt-cuirui").Count(&cnt).Error)
	require.Equal(t, int64(2), cnt, "分摊子项必须零丢失")

	// 幂等：再跑一次不新增租户、不改 id
	require.NoError(t, h.hrSyncOrgTenants76(ctx, 1))
	var tenants []types.BillingTenant
	require.NoError(t, db.Where("tenant_id = ?", 1).Find(&tenants).Error)
	require.Len(t, tenants, 1)
	require.Equal(t, "bt-cuirui", tenants[0].ID, "核算标识必须稳定（历史账单靠它归属）")
}

func TestRound76OrgSaveSyncsBillingTenant(t *testing.T) {
	db := hrRound76DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HROrgUnit{
		ID: "u1", TenantID: 1, Name: "持睿汽车部件有限公司", Alias: "持睿汽车",
		IsTenant: true, TenantNo: "Z001", AllocationMode: "按比例分摊",
		Contact: "李锶雨", Phone: "13800000000", BillingTenantID: "bt1", SortOrder: 1,
	}).Error)
	require.NoError(t, db.Create(&types.BillingTenant{
		ID: "bt1", TenantID: 1, Name: "持睿汽车", TenantNo: "OLD",
	}).Error)
	h := &HRHandler{db: db}
	require.NoError(t, h.hrSyncOrgTenants76(ctx, 1))
	var bt types.BillingTenant
	require.NoError(t, db.Where("id = ?", "bt1").First(&bt).Error)
	require.Equal(t, "持睿汽车", bt.Name)
	require.Equal(t, "Z001", bt.TenantNo, "租户编码以公司设置为准")
	require.Equal(t, "李锶雨", bt.Contact)
}

func TestRound76InsuranceReconfig(t *testing.T) {
	db := hrRound76DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-ins", TenantID: 1, Scope: "vehicle", Name: "保险单", Enabled: true, SortOrder: 5,
		Subs: []types.FleetCategorySub{
			{Name: "保单号", Enabled: true, IsDefault: true, DataType: "text", Width: 160},
		},
	}).Error)
	// 存量保单：旧字段名
	require.NoError(t, db.Create(&types.FleetRecord{
		ID: "rec1", TenantID: 1, RecordType: "vehicle-archive", DocType: "保险单", SubCategory: "保险单",
		Data: map[string]any{
			"保单号": "P123456", "保险公司": "平安财险", "车牌号": "渝A12345",
			"起保日期": "2025-11-01", "终保日期": "2026-10-31", "保费": "1130",
		},
	}).Error)
	require.NoError(t, db.Create(&types.KnowledgeBase{
		ID: "kb-fleet", Name: "日常事务-车队", TenantID: 1,
	}).Error)

	h := &HRHandler{db: db}
	fields, seeded := h.hrReconfigFleetInsurance76(ctx, 1)
	require.Equal(t, 68, fields, "13+11+6 交强险 30 ｜ 13+11+7 商业险 31 ｜ 险种类别 7")
	require.True(t, seeded, "保险单提取规则必须落库")

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-ins").First(&cat).Error)
	groupCount := map[string]int{}
	byName := map[string]types.FleetCategorySub{}
	for _, s := range cat.Subs {
		groupCount[s.Group]++
		byName[s.Name] = s
	}
	require.Equal(t, 13, groupCount["交强险/基本信息"])
	require.Equal(t, 11, groupCount["交强险/被保险机动车"])
	require.Equal(t, 6, groupCount["交强险/责任限额"])
	require.Equal(t, 13, groupCount["商业险/基本信息"])
	require.Equal(t, 11, groupCount["商业险/被保险机动车"])
	require.Equal(t, 7, groupCount["商业险/投保险别"])
	require.Equal(t, 7, groupCount["险种类别"])
	require.Equal(t, "amount", byName["死亡伤残赔偿限额"].DataType, "责任限额必须金额类型")
	require.Equal(t, "商业险_保险单号", byName["商业险_保险单号"].Name)
	require.Contains(t, byName, "商业险_车船税", "商业险 13 个基础字段必须齐全（含车船税）")
	require.Equal(t, "保险单号", byName["商业险_保险单号"].Label, "商业险展示名与交强险一致")
	require.Equal(t, 160, byName["保险单号"].Width, "同名字段保留用户列宽")

	// 存量记录：旧键保留 + 新键映射
	var rec types.FleetRecord
	require.NoError(t, db.Where("id = ?", "rec1").First(&rec).Error)
	require.Equal(t, "P123456", toStringValue(rec.Data["保险单号"]))
	require.Equal(t, "平安财险", toStringValue(rec.Data["保险公司名称"]))
	require.Equal(t, "渝A12345", toStringValue(rec.Data["号牌号码"]))
	require.Equal(t, "2025-11-01", toStringValue(rec.Data["保险起始日期"]))
	require.Equal(t, "1130", toStringValue(rec.Data["保费合计"]))
	require.Equal(t, "P123456", toStringValue(rec.Data["保单号"]), "旧键必须保留（不删数）")

	// 提取规则：68 条字段 + 高级 Prompt 区分交强险限额 / 商业险险别
	var cfg types.KbExtractConfig
	require.NoError(t, db.Where("knowledge_base_id = ? AND cert_type = ?", "kb-fleet", "保险单").First(&cfg).Error)
	require.Len(t, cfg.Fields, 68)
	require.True(t, cfg.AdvancedEnabled)
	require.Contains(t, cfg.PromptTemplate, "{{fields_schema}}")
	require.Contains(t, cfg.PromptTemplate, "交强险")
	require.Contains(t, cfg.PromptTemplate, "投保险别")
	ruleOf := map[string]string{}
	for _, f := range cfg.Fields {
		ruleOf[f.Name] = f.Regex
	}
	require.NotEmpty(t, ruleOf["保险单号"], "保单号必须有默认正则兜底")
	require.NotEmpty(t, ruleOf["车架号"], "VIN 必须有默认正则兜底")

	// 幂等：第二次只补不改，字段数不翻倍
	fields2, _ := h.hrReconfigFleetInsurance76(ctx, 1)
	require.Equal(t, 68, fields2)
	require.NoError(t, db.Where("knowledge_base_id = ? AND cert_type = ?", "kb-fleet", "保险单").First(&cfg).Error)
	require.Len(t, cfg.Fields, 68)
}

func TestRound76RenameCategoryKeepsRecords(t *testing.T) {
	db := hrRound76DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat1", TenantID: 1, Scope: "vehicle", Name: "道路运输证", Enabled: true,
	}).Error)
	for i, id := range []string{"r1", "r2", "r3"} {
		require.NoError(t, db.Create(&types.FleetRecord{
			ID: id, TenantID: 1, RecordType: "vehicle-archive",
			DocType: "道路运输证", SubCategory: "道路运输证",
			Data: map[string]any{"证照类型": "道路运输证", "编号": i},
		}).Error)
	}
	h := &FleetHandler{db: db}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return h.propagateCategoryRename(ctx, tx, "vehicle", "道路运输证", "道路运输许可证")
	}))

	var n int64
	require.NoError(t, db.Model(&types.FleetRecord{}).
		Where("sub_category = ?", "道路运输许可证").Count(&n).Error)
	require.Equal(t, int64(3), n, "改名后列表过滤口径（sub_category）必须仍能查到全部记录")
	require.NoError(t, db.Model(&types.FleetRecord{}).
		Where("doc_type = ?", "道路运输许可证").Count(&n).Error)
	require.Equal(t, int64(3), n, "KPI 卡片口径（doc_type）同样 0 丢失")

	var rec types.FleetRecord
	require.NoError(t, db.Where("id = ?", "r1").First(&rec).Error)
	require.Equal(t, "道路运输许可证", toStringValue(rec.Data["证照类型"]))
	require.Equal(t, float64(0), rec.Data["编号"], "其它业务字段不得被改写")

	// 卡片计数与列表同口径：改名后 refs 仍为 3（否则「卡片 N 条、点开 0 条」）
	refs, err := h.countFleetCategoryRefs(ctx, "vehicle", "道路运输许可证")
	require.NoError(t, err)
	require.Equal(t, 3, refs)
}
