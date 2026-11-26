package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 全字段搜索：`所属公司` / `宿舍楼/区域` / 表号这类「非列名」字段也要能命中。
func TestRound73MatchAllFields(t *testing.T) {
	row := map[string]any{
		"宿舍编号": "宿舍301", "所属公司": "重庆星达铜业有限公司",
		"宿舍楼/区域": "1号楼", "宿舍电表号": "E001", "员工姓名": "吴勇建",
		"id": "uuid-should-be-ignored",
	}
	require.True(t, hrMatchAllFields73("星达铜业", row))
	require.True(t, hrMatchAllFields73("持睿", row) == false)
	require.True(t, hrMatchAllFields73("1号楼", row))
	require.True(t, hrMatchAllFields73("e001", row), "表号大小写不敏感")
	require.True(t, hrMatchAllFields73("吴勇建", row))
	require.False(t, hrMatchAllFields73("不存在的值", row))
	require.True(t, hrMatchAllFields73("", row), "空关键词不过滤")
	require.False(t, hrMatchAllFields73("uuid-should", row), "内部 id 不参与搜索")
}

// 表名/表号对齐：档案写 E301，历史表计是 E001 → 迁移后两者一致，且抄表金额不动。
func TestRound73MeterNameSyncKeepsAmounts(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&types.UtilityMeterRecord{}, &types.UtilityMeterItem{}))
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room301", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301",
		CustomData: map[string]any{
			"房间号": "301", "宿舍编号": "宿舍301", "床位数": "2",
			"宿舍电表名称": "宿舍301", "宿舍电表号": "E301",
		},
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m-e001", TenantID: 1, Category: "electricity", MeterNo: "E001", Alias: "宿舍301",
		MeterKind: "dorm", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterRecord{
		ID: "rec1", TenantID: 1, Category: "electricity", Month: "2026-10",
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterItem{
		ID: "it1", RecordID: "rec1", MeterID: "m-e001",
		StartReading: 8293, EndReading: 8357, Usage: 64, Amount: 64, UnitPrice: 1, Rate: 1,
	}).Error)

	h := &HRHandler{db: db}
	meters, items := h.hrSyncEnergyHistoryMeterNames73(ctx, 1)
	require.Equal(t, 1, meters)
	require.Equal(t, 1, items)

	var m types.UtilityMeter
	require.NoError(t, db.Where("id = ?", "m-e001").First(&m).Error)
	require.Equal(t, "E301", m.MeterNo, "表号对齐到档案")
	require.Equal(t, "宿舍301", m.Alias)

	var it types.UtilityMeterItem
	require.NoError(t, db.Where("id = ?", "it1").First(&it).Error)
	require.Equal(t, 64.0, it.Amount, "金额严禁变动")
	require.Equal(t, 8293.0, it.StartReading)
	require.Equal(t, 8357.0, it.EndReading)
	require.Equal(t, "宿舍301", it.MeterName)
}

// 费用核算：电/水费必须来自真实抄表（不是 0→73 的占位值）。
func TestRound73ChargesUseRealMeterReading(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&types.UtilityMeterRecord{}, &types.UtilityMeterItem{},
		&HREmployee{}, &HRDormCharge{}))
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "吴勇建", EmployeeID: "C001", Department: "生产部", Status: "在职",
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room301", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301",
		CustomData: map[string]any{
			"房间号": "301", "宿舍编号": "宿舍301", "床位数": "2",
			"宿舍电表号": "E301", "宿舍水表号": "W301",
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay1", TenantID: 1, Module: hrDormModuleStay,
		CustomData: map[string]any{
			"宿舍编号/房屋地址": "宿舍301", "员工姓名": "吴勇建", "员工工号": "C001",
			"住宿状态": "在住", "入住日期": "2026-01-05", "分摊比例": "1",
		},
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m1", TenantID: 1, Category: "electricity", MeterNo: "E301", Alias: "宿舍301", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m2", TenantID: 1, Category: "water", MeterNo: "W301", Alias: "宿舍301", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterRecord{
		ID: "rec-e", TenantID: 1, Category: "electricity", Month: "2026-10",
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterRecord{
		ID: "rec-w", TenantID: 1, Category: "water", Month: "2026-10",
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterItem{
		ID: "ie", RecordID: "rec-e", MeterID: "m1",
		StartReading: 8293, EndReading: 8357, Usage: 64, Amount: 64, UnitPrice: 1, Rate: 1,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterItem{
		ID: "iw", RecordID: "rec-w", MeterID: "m2",
		StartReading: 260, EndReading: 264, Usage: 4, Amount: 20, UnitPrice: 5, Rate: 1,
	}).Error)

	h := &HRHandler{db: db}
	rows, err := h.hrBuildDormCharges(ctx, 1, "2026-10")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "宿舍301", rows[0].RoomNo)
	require.Equal(t, 64.0, rows[0].Elec, "电费=真实抄表金额")
	require.Equal(t, 8293.0, rows[0].ElecStart)
	require.Equal(t, 8357.0, rows[0].ElecEnd)
	require.Equal(t, 20.0, rows[0].Water, "水费=真实抄表金额")
	require.Equal(t, 260.0, rows[0].WaterStart)
	require.Equal(t, 264.0, rows[0].WaterEnd)
}

// 越界床位：双人间出现 3号床 → 迁移重排为 1/2；保存时 3号床 直接 400。
func TestRound73BedOverflow(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room305", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍305",
		CustomData: map[string]any{"房间号": "305", "宿舍编号": "宿舍305", "床位数": "2", "房型": "双人间"},
	}).Error)
	for i, name := range []string{"甲", "乙"} {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: "s" + name, TenantID: 1, Module: hrDormModuleStay,
			CustomData: map[string]any{
				"宿舍编号/房屋地址": "宿舍305", "员工姓名": name, "住宿状态": "在住",
				"床位号": "3号床", "入住日期": "2026-0" + string(rune('1'+i)) + "-01",
			},
		}).Error)
	}
	h := &HRHandler{db: db}
	require.Equal(t, 2, h.hrFixBedOverflow73(ctx, 1))

	var rows []HRDormRecord
	require.NoError(t, db.Where("module = ?", hrDormModuleStay).Find(&rows).Error)
	beds := map[string]string{}
	for _, r := range rows {
		beds[hrDormStr(r.CustomData, "员工姓名")] = hrDormStr(r.CustomData, "床位号")
	}
	require.Equal(t, "1号床", beds["甲"])
	require.Equal(t, "2号床", beds["乙"])

	// 保存时的上限校验
	err := h.hrDormValidateBedNo72(ctx, 1, "", "宿舍305", "3号床")
	require.Error(t, err)
	require.Contains(t, err.Error(), "3号床")
}
