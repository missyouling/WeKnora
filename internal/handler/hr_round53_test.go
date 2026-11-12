// 第 53 轮后端回归：司龄全量派生/回填、租户分组补全、字段白名单。
package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestHRRound53SeniorityBackfill(t *testing.T) {
	db := hrTestDB(t)
	h := &HRHandler{db: db}
	rows := []HREmployee{
		{ID: "e1", TenantID: 0, Name: "在职", IDNumber: "51072319850217371X", HireDate: "2018-07-01", Status: "active", CustomData: map[string]any{}},
		{ID: "e2", TenantID: 0, Name: "离职", IDNumber: "440305199603051319", HireDate: "2020-03-15", ResignDate: "2022-05-20", Status: "resigned", CustomData: map[string]any{}},
		{ID: "e3", TenantID: 0, Name: "无入职日期", IDNumber: "110101199001011234", Status: "active", CustomData: map[string]any{}},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}
	require.NoError(t, h.hrBackfillSeniority(context.Background()))

	var got []HREmployee
	require.NoError(t, db.Order("id ASC").Find(&got).Error)
	// 在职：截至今天（不写死具体月数，只断言格式与「不为空」）
	assert.Regexp(t, `^\d+年\d+个月$`, got[0].CustomData["司龄"], "在职员工必须有司龄")
	// 离职：截至离职日期，恒为 2年2个月
	assert.Equal(t, "2年2个月", got[1].CustomData["司龄"])
	// 无入职日期：不造假值
	assert.Nil(t, got[2].CustomData["司龄"])

	// 幂等：再跑一次不产生变化
	require.NoError(t, h.hrBackfillSeniority(context.Background()))
	var again []HREmployee
	require.NoError(t, db.Order("id ASC").Find(&again).Error)
	assert.Equal(t, got[0].CustomData["司龄"], again[0].CustomData["司龄"])
}

func TestHRRound53AttachDerivedWritesSeniority(t *testing.T) {
	db := hrTestDB(t)
	emp := HREmployee{
		ID: "e9", TenantID: 0, Name: "张三", IDNumber: "51072319850217371X",
		HireDate: "2016-06-06", Status: "active", CustomData: map[string]any{},
	}
	require.NoError(t, db.Create(&emp).Error)
	items := []HREmployee{emp}
	hrAttachEmployeeDerived(context.Background(), db, 0, items)
	assert.NotEmpty(t, items[0].CustomData["司龄"], "列表响应必须实时带司龄")
	assert.Regexp(t, `^\d+年\d+个月$`, items[0].CustomData["司龄"])
	assert.NotEmpty(t, items[0].CustomData["年龄"], "身份证推导年龄不受影响")
}

func TestHRRound53RepairTenantGroupFields(t *testing.T) {
	db := hrTestDB(t)
	cat := &types.FleetCategory{
		ID: "cat-1", TenantID: 0, Scope: EnergyScope, Name: "费用清单",
		BuiltinKey: "cost-list", Enabled: true,
		Subs: []types.FleetCategorySub{
			// 模拟被 sanitizeFleetSubs 全局去重删光后的残局：只剩两条收口列
			{Name: "elec_total_fee", Label: "电费合计", Enabled: true, IsDefault: true, DataType: "number", Group: "tenant:t1", GroupLabel: "持睿汽车"},
			{Name: "water_total_fee", Label: "水费合计", Enabled: true, IsDefault: true, DataType: "number", Group: "tenant:t1", GroupLabel: "持睿汽车"},
			// 默认分组不受影响
			{Name: "month", Label: "账单周期", Enabled: true, IsDefault: true, DataType: "text", Group: "cols"},
		},
	}
	require.NoError(t, db.Create(cat).Error)

	uh := NewUtilityHandler(db)
	n, err := uh.RepairTenantGroupFields(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	var got types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-1").First(&got).Error)
	byName := map[string]types.FleetCategorySub{}
	tenantCount := 0
	for _, s := range got.Subs {
		if s.Group == "tenant:t1" {
			tenantCount++
			byName[s.Name] = s
		}
	}
	// 17 派生 + 2 收口 + 4 大类 = 23 条（本用例无 item_）
	assert.Equal(t, 21, tenantCount, "租户分组必须补全到 21 个规范字段（15 派生 + 2 收口 + 4 大类）")
	for _, key := range []string{"month", "unit", "ratio", "ind_kwh", "ind_fee", "ind_price",
		"dorm_kwh", "dorm_fee", "water_ind_usage", "grp_trans", "grp_market",
		"elec_total_fee", "water_total_fee"} {
		assert.Contains(t, byName, key, "缺少字段 "+key)
	}
	// label 与前端 TENANT_DERIVED_FIELDS 同源
	assert.Equal(t, "工业均价", byName["ind_price"].Label)
	assert.Equal(t, "零售输配电费", byName["grp_trans"].Label)
	assert.Equal(t, "持睿汽车", byName["ind_kwh"].GroupLabel)
	// 锚点前置：账单周期 / 使用单位 / 分摊比例必须是该分组前三个字段
	tenantNames := make([]string, 0, 3)
	for _, s := range got.Subs {
		if s.Group == "tenant:t1" {
			tenantNames = append(tenantNames, s.Name)
		}
	}
	assert.GreaterOrEqual(t, len(tenantNames), 3)
	assert.Equal(t, []string{"month", "unit", "ratio"}, tenantNames[:3], "锚点列必须排在租户分组最前")
	// 默认分组不被污染
	monthCount := 0
	for _, s := range got.Subs {
		if s.Name == "month" && s.Group == "cols" {
			monthCount++
		}
	}
	assert.Equal(t, 1, monthCount, "默认分组的 month 不能被复制")

	// 幂等：再跑一次不写入
	n2, err := uh.RepairTenantGroupFields(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n2)
}

func TestHRRound53TenantFieldAllowed(t *testing.T) {
	// 第 53 轮：补 grp_trans，否则每次启动都会删掉「零售输配电费」
	assert.True(t, tenantFieldAllowed("grp_trans"))
	assert.True(t, tenantFieldAllowed("grp_market"))
	// 跨分类引用字段是用户主动添加的合法列
	assert.True(t, tenantFieldAllowed("src_花名册_姓名"))
	// 分摊子项
	assert.True(t, tenantFieldAllowed("item_total"))
	// 默认分组的业主视角列仍然不允许混进租户分组
	assert.False(t, tenantFieldAllowed("total_kwh"))
	assert.False(t, tenantFieldAllowed("solar_gen"))
	assert.False(t, tenantFieldAllowed(""))
}

func TestHRRound53SeniorityFormulaBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	// 不满 1 年 → 0年Y个月
	assert.Equal(t, "0年3个月", hrSeniorityOf(&HREmployee{HireDate: "2026-08-12"}, now))
	// 入职当天
	assert.Equal(t, "0年0个月", hrSeniorityOf(&HREmployee{HireDate: "2026-11-12"}, now))
	// 未来入职日期 → 0年0个月（不出现负数）
	assert.Equal(t, "0年0个月", hrSeniorityOf(&HREmployee{HireDate: "2027-01-01"}, now))
	// 无入职日期 → 空
	assert.Equal(t, "", hrSeniorityOf(&HREmployee{}, now))
}
