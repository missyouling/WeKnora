// 第 63 轮回归：花名册签约公司归一、单位设置补齐、宿舍楼/房型修正、宿舍字段拆分。
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestRound63RosterContractCompany(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HREmployee{}, &types.FleetCategory{}, &HROrgUnit{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", EmployeeID: "C1", Status: hrRosterStatusActive,
		IDNumber: "500101199001010011",
		CustomData: map[string]any{
			"合同公司": "重庆持睿汽车配件有限公司",
			"签约公司": "持睿汽车",
		},
	}).Error)
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", BuiltinKey: "hr-roster",
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, Group: "在职人员"},
			{Name: "合同公司", Label: "合同公司", Enabled: true, Group: "在职人员"},
			{Name: "签约公司", Enabled: true, Group: "在职人员"},
		},
	}).Error)

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c2", TenantID: 1, Scope: hrScopeResign, Name: "离职管理", BuiltinKey: "hr-resign",
		// v63b 的历史错误：签约公司被追加到末尾且落在独立分组「离职人员」
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, Group: "在职人员"},
			{Name: "岗位", Enabled: true, Group: "在职人员"},
			{Name: "离职日期", Enabled: true, Group: "离职信息"},
			{Name: "签约公司", Label: "签约公司", Enabled: true, Group: "离职人员"},
		},
	}).Error)

	require.NoError(t, h.hrFixRosterContractCompany63(ctx))

	var emp HREmployee
	require.NoError(t, db.Where("id = ?", "e1").First(&emp).Error)
	require.Equal(t, hrContractCompanyName, hrDormStr(emp.CustomData, "签约公司"))
	require.Equal(t, hrContractCompanyName, hrDormStr(emp.CustomData, "合同公司"))

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&cat).Error)
	names := map[string]bool{}
	for _, s := range cat.Subs {
		names[s.Name] = true
		require.NotEqual(t, "合同公司", s.Name, "合同公司必须改名")
	}
	require.True(t, names["签约公司"])
	// 在职胶囊：合同公司改名后与已有签约公司去重 → 工号 + 签约公司 = 2（不得塞离职字段）
	require.Equal(t, 2, len(cat.Subs), "在职胶囊不得混入离职字段")

	// 离职胶囊：签约公司必须紧跟「岗位」且沿用岗位分组，不得自建「离职人员」分组
	var resignCat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c2").First(&resignCat).Error)
	require.Equal(t, 4, len(resignCat.Subs))
	require.Equal(t, "岗位", resignCat.Subs[1].Name)
	require.Equal(t, "签约公司", resignCat.Subs[2].Name)
	require.Equal(t, resignCat.Subs[1].Group, resignCat.Subs[2].Group, "分组必须与岗位一致")
	for _, s := range resignCat.Subs {
		require.NotEqual(t, "离职人员", s.Group, "不得出现凭空多出的分组")
	}

	// 单位设置补齐：星达 + 持睿
	var units []HROrgUnit
	require.NoError(t, db.Where("tenant_id = ?", 1).Find(&units).Error)
	unitNames := map[string]bool{}
	for _, u := range units {
		unitNames[u.Name] = true
	}
	require.True(t, unitNames["重庆星达铜业有限公司"])
	require.True(t, unitNames["重庆持睿汽车部件有限公司"])

	// 幂等
	require.NoError(t, h.hrFixRosterContractCompany63(ctx))
}

func TestRound63DormBuildingAndType(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
		CustomData: map[string]any{
			"宿舍编号": "宿舍301", "宿舍楼/区域": "厂区宿舍", "房型": "单人间", "床位数": 1,
		},
	}).Error)
	require.NoError(t, h.hrFixDormBuildingAndType63(ctx))

	var room HRDormRecord
	require.NoError(t, db.Where("id = ?", "r1").First(&room).Error)
	require.Equal(t, "1号楼", hrDormStr(room.CustomData, "宿舍楼/区域"))
	require.Equal(t, "双人间", hrDormStr(room.CustomData, "房型"))
	require.Equal(t, 2, int(hrDormFloat(room.CustomData, "床位数")))

	// 新增归一：空值默认 1号楼 / 双人间 / 2 床
	data := map[string]any{"宿舍编号": "宿舍399"}
	hrDormNormalizeRecord(hrDormModuleRoom, data)
	require.Equal(t, "1号楼", hrDormStr(data, "宿舍楼/区域"))
	require.Equal(t, "双人间", hrDormStr(data, "房型"))
	require.Equal(t, 2, int(hrDormFloat(data, "床位数")))
}

func TestRound63DormSubsSplit(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &types.FleetCategory{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "d1", TenantID: 1, Scope: hrScopeDorm, Name: "宿舍管理", BuiltinKey: "hr-dorm",
		Subs: []types.FleetCategorySub{
			{Name: "宿舍编号", Enabled: true, Group: hrDormGroupRoom},
			{Name: "所属公司/部门", Enabled: true, Group: hrDormGroupRoom},
			{Name: "租赁单位", Enabled: true, Group: hrDormGroupPublic},
			{Name: "住宿记录ID", Enabled: true, Group: hrDormGroupStay},
			{Name: "资产编号", Enabled: true, Group: hrDormGroupAsset},
			{Name: "账单月份", Enabled: true, Group: hrDormGroupFee},
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
		CustomData: map[string]any{"宿舍编号": "宿舍301", "所属公司/部门": "重庆星达铜业有限公司"},
	}).Error)

	require.NoError(t, h.hrMigrateDormSubs63(ctx))

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "d1").First(&cat).Error)
	var comp, dept, lease, stay, fee int
	for _, s := range cat.Subs {
		require.NotEqual(t, "所属公司/部门", s.Name)
		if s.Group != hrDormGroupRoom {
			continue
		}
		switch s.Name {
		case "所属公司":
			comp++
		case "所属部门":
			dept++
		}
	}
	for _, s := range cat.Subs {
		switch s.Name {
		case "租赁单位":
			lease++
		case "住宿记录ID":
			stay++
		case "账单月份":
			fee++
		}
	}
	require.Equal(t, 1, comp, "宿舍档案必须有所属公司")
	require.Equal(t, 1, dept, "宿舍档案必须有所属部门")
	require.Equal(t, 1, lease)
	// 公租房分组没有所属公司 → 必须自动补两条
	pubComp, pubDept := 0, 0
	for _, s := range cat.Subs {
		if s.Group != hrDormGroupPublic {
			continue
		}
		if s.Name == "所属公司" {
			pubComp++
		}
		if s.Name == "所属部门" {
			pubDept++
		}
	}
	require.Equal(t, 1, pubComp)
	require.Equal(t, 1, pubDept)
	require.Equal(t, 1, stay, "其它分组不得被丢弃")
	require.Equal(t, 1, fee, "其它分组不得被丢弃")

	var rec HRDormRecord
	require.NoError(t, db.Where("id = ?", "r1").First(&rec).Error)
	require.Equal(t, "重庆星达铜业有限公司", hrDormStr(rec.CustomData, "所属公司"))
	require.Equal(t, "", hrDormStr(rec.CustomData, "所属部门"))
}
