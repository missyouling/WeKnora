// 第 70 轮回归：
//  1. 宿舍「管理员/舍长」可清空（落库空串 + 记住手动清空标记）
//  2. 舍长默认首住人 / 退宿顺延 / 成员派生剔除舍长
//  3. 401–420 幂等补全 + 房间号自然升序
//  4. 12 类标准资产幂等补齐
//  5. 表计迁移：元数据回填房源、原 utility_meters 记录零删除
//  6. 公司改名：宿舍域 + 能耗表计事务级联
package handler

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// hrRound70DB 在第 59 轮测试库上补建宿舍记录与能耗表计两张表。
func hrRound70DB(t *testing.T) *gorm.DB {
	t.Helper()
	db := hrRound59DB(t)
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &types.UtilityMeter{}))
	return db
}

// 清空舍长：显式提交空串必须真实落库，并打上「手动清空」标记。
func TestRound70ManagerClearPersists(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room70", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍302", Name: "宿舍302",
		CustomData: map[string]any{
			"宿舍编号": "宿舍302", "房间号": "302", "管理员/舍长": "董友兵", "床位数": float64(2),
		},
	}).Error)

	merged := map[string]any{"宿舍编号": "宿舍302", "房间号": "302", "管理员/舍长": ""}
	var row HRDormRecord
	require.NoError(t, db.Where("id = ?", "room70").First(&row).Error)
	// 模拟 UpdateDormRecord 的核心逻辑（显式提交空值 → 打「手动清空」标记）
	require.Equal(t, "", merged["管理员/舍长"])
	row.CustomData["管理员/舍长"] = ""
	row.CustomData["舍长手动清空"] = "1"
	require.NoError(t, db.Model(&row).Select("custom_data").Updates(&row).Error)

	var got []HRDormRecord
	require.NoError(t, db.Where("id = ?", "room70").Find(&got).Error)
	require.Equal(t, "", hrDormStr(got[0].CustomData, "管理员/舍长"), "清空必须落库")
	require.Equal(t, "1", hrDormStr(got[0].CustomData, "舍长手动清空"))

	// 视图不能把首住人填回来
	view := h.hrDormRecordViewCtx(ctx, 1, hrDormModuleRoom, got[0])
	require.Equal(t, "", view["管理员/舍长"], "手动清空后不得被默认首住人覆盖")
}

// 默认舍长 = 最早入住的在住人员；退宿后顺延；成员剔除舍长。
func TestRound70ManagerDefaultsToFirstOccupant(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room71", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍304", Name: "宿舍304",
		CustomData: map[string]any{"宿舍编号": "宿舍304", "房间号": "304", "床位数": float64(2)},
	}).Error)
	mk := func(id, name, since string) {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: id, TenantID: 1, Module: hrDormModuleStay, Code: "STAY-" + id,
			CustomData: map[string]any{
				"员工姓名": name, "宿舍编号/房屋地址": "宿舍304", "入住日期": since, "住宿状态": "在住",
			},
		}).Error)
	}
	mk("s1", "吴英", "2026-09-01")
	mk("s2", "董友兵", "2026-10-01")

	var room HRDormRecord
	require.NoError(t, db.Where("id = ?", "room71").First(&room).Error)
	view := h.hrDormRecordViewCtx(ctx, 1, hrDormModuleRoom, room)
	require.Equal(t, "吴英", view["管理员/舍长"], "默认取最早入住者")
	require.Equal(t, "董友兵", view["成员"], "成员 = 在住人员剔除舍长")

	// 舍长退宿 → 顺延到下一位
	require.NoError(t, db.Model(&HRDormRecord{}).Where("id = ?", "s1").
		Update("custom_data", `{"员工姓名":"吴英","宿舍编号/房屋地址":"宿舍304","入住日期":"2026-09-01","住宿状态":"已退宿"}`).Error)
	var room2 HRDormRecord
	require.NoError(t, db.Where("id = ?", "room71").First(&room2).Error)
	view2 := h.hrDormRecordViewCtx(ctx, 1, hrDormModuleRoom, room2)
	require.Equal(t, "董友兵", view2["管理员/舍长"], "舍长退宿后顺延")
	require.Equal(t, "", view2["成员"], "只剩一人且是舍长 → 成员为空")
}

// 401–420 幂等补全；列表按房间号自然升序。
func TestRound70SeedDorm401To420(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	for _, no := range []string{"301", "320", "417", "419"} {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: "r" + no, TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍" + no,
			CustomData: map[string]any{"宿舍编号": "宿舍" + no, "房间号": no, "床位数": float64(2)},
		}).Error)
	}
	created := h.hrSeedDorm401To420_70(ctx, 1)
	require.Equal(t, 18, created, "补 401–416 / 418 / 420 共 18 间")
	require.Equal(t, 0, h.hrSeedDorm401To420_70(ctx, 1), "幂等：第二次零新增")

	var rooms []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, hrDormModuleRoom).Find(&rooms).Error)
	require.Len(t, rooms, 22)

	sort.SliceStable(rooms, func(i, j int) bool {
		return hrDormRoomNoValue(rooms[i]) < hrDormRoomNoValue(rooms[j])
	})
	nos := make([]int, 0, len(rooms))
	for _, r := range rooms {
		nos = append(nos, hrDormRoomNoValue(r))
	}
	require.Equal(t, 301, nos[0])
	require.Equal(t, 420, nos[len(nos)-1], "301→320→401→420（417/419 已有）")

	var one HRDormRecord
	require.NoError(t, db.Where("id = ?", "r301").First(&one).Error)
	_ = one
	var n401 []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, hrDormModuleRoom).
		Find(&n401).Error)
	var r401 *HRDormRecord
	for i := range n401 {
		if hrDormRoomNoValue(n401[i]) == 401 {
			r401 = &n401[i]
		}
	}
	require.NotNil(t, r401)
	require.Equal(t, "1号楼", hrDormStr(r401.CustomData, "宿舍楼/区域"))
	require.Equal(t, "双人间", hrDormStr(r401.CustomData, "房型"))
	require.Equal(t, "重庆星达铜业有限公司", hrDormStr(r401.CustomData, "所属公司"))
	require.Equal(t, "宿舍401", hrDormStr(r401.CustomData, "宿舍电表名称"))
	require.Equal(t, "E401", hrDormStr(r401.CustomData, "宿舍电表号"))
}

// 12 类标准资产：每间宿舍一份，幂等且不覆盖已有。
func TestRound70SeedDormAssets(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "roomA", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301",
		CustomData: map[string]any{"宿舍编号": "宿舍301", "房间号": "301"},
	}).Error)
	// 已有「床」不应重复
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "assetA", TenantID: 1, Module: hrDormModuleAsset, Code: "ZC-301-01", Name: "床",
		CustomData: map[string]any{"资产名称": "床", "所属宿舍": "宿舍301", "数量": float64(1)},
	}).Error)

	created := h.hrSeedDormAssets70(ctx, 1)
	require.Equal(t, 11, created, "12 类里「床」已存在 → 补 11")
	require.Equal(t, 0, h.hrSeedDormAssets70(ctx, 1), "幂等：第二次零新增")

	var assets []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, hrDormModuleAsset).Find(&assets).Error)
	names := map[string]bool{}
	for _, a := range assets {
		names[hrDormStr(a.CustomData, "资产名称")] = true
	}
	for _, want := range []string{"晾衣杆", "毛巾杆", "衣柜", "床柜", "床头柜", "空调遥控器",
		"空调", "电热水器", "床", "床垫", "钥匙", "窗帘"} {
		require.True(t, names[want], "缺资产 %s", want)
	}
}

// 表计迁移：宿舍电/水、公租房水/气回填到房源；原表计记录一条不删。
func TestRound70MigrateDormMeters(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HRDormRecord{
		ID: "roomB", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301",
		CustomData: map[string]any{"宿舍编号": "宿舍301", "房间号": "301"},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "pubB", TenantID: 1, Module: hrDormModulePublic, Code: "两山秀苑19-23-5",
		CustomData: map[string]any{"房屋地址": "两山秀苑19-23-5", "房屋编号": "两山秀苑19-23-5"},
	}).Error)

	meters := []types.UtilityMeter{
		{ID: "m1", TenantID: 1, Category: "electricity", Alias: "宿舍301", MeterNo: "E001",
			MeterKind: "dorm", Rate: 1, DefaultUnitPrice: 1.2, Enabled: true},
		{ID: "m2", TenantID: 1, Category: "water", Alias: "宿舍301", MeterNo: "W001",
			MeterKind: "dorm", Rate: 2, DefaultUnitPrice: 5, Enabled: true},
		{ID: "m3", TenantID: 1, Category: "water", Alias: "两山秀苑19-23-5", MeterNo: "W900",
			MeterKind: "custom1", Rate: 1, DefaultUnitPrice: 3.32, Enabled: true},
		{ID: "m4", TenantID: 1, Category: "gas", Alias: "两山秀苑19-23-5", MeterNo: "G002",
			MeterKind: "custom1", Rate: 1, DefaultUnitPrice: 2.196, Enabled: true},
	}
	for i := range meters {
		require.NoError(t, db.Create(&meters[i]).Error)
	}

	dormHit, publicHit, missed := h.hrMigrateDormMeters70(ctx, 1)
	// 计数单位是「表计张数」：宿舍有电+水 2 张，公租房有水+气 2 张
	require.Equal(t, 2, dormHit)
	require.Equal(t, 2, publicHit)
	require.Equal(t, 0, missed)

	var room, pub HRDormRecord
	require.NoError(t, db.Where("id = ?", "roomB").First(&room).Error)
	require.Equal(t, "宿舍301", hrDormStr(room.CustomData, "宿舍电表名称"))
	require.Equal(t, "E001", hrDormStr(room.CustomData, "宿舍电表号"))
	require.Equal(t, 1.2, hrDormFloat(room.CustomData, "电费标准(元/度)"))
	require.Equal(t, 2.0, hrDormFloat(room.CustomData, "水表倍率"))

	require.NoError(t, db.Where("id = ?", "pubB").First(&pub).Error)
	require.Equal(t, "W900", hrDormStr(pub.CustomData, "公租房水表号"))
	require.Equal(t, "G002", hrDormStr(pub.CustomData, "公租房气表号"))
	require.Equal(t, 2.196, hrDormFloat(pub.CustomData, "气费标准(元/m³)"))

	// 红线：原表计记录一条都不能少（历史核算按 meter_no 引用）
	var cnt int64
	require.NoError(t, db.Model(&types.UtilityMeter{}).Count(&cnt).Error)
	require.Equal(t, int64(4), cnt, "utility_meters 记录零删除")
}

// 公司改名：宿舍域 + 能耗表计事务级联；已是新名的记录不误改。
func TestRound70RenameCompanyCascade(t *testing.T) {
	db := hrRound70DB(t)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room417", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍417",
		CustomData: map[string]any{
			"宿舍编号": "宿舍417", "所属公司": "重庆持睿汽车部件有限公司",
			"租赁单位": "重庆持睿汽车部件有限公司",
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room418", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍418",
		CustomData: map[string]any{"宿舍编号": "宿舍418", "所属公司": "持睿汽车部件有限公司"},
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m417", TenantID: 1, Category: "electricity", Alias: "持睿403", MeterNo: "E403",
		MeterKind: "dorm", OwnerUnit: "重庆持睿汽车部件有限公司",
		UseUnit: "重庆持睿汽车部件有限公司", Enabled: true,
	}).Error)

	affected, e := hrSyncBusinessUnitName70(db, 1, "重庆持睿汽车部件有限公司", "持睿汽车部件有限公司")
	require.NoError(t, e)
	require.GreaterOrEqual(t, affected, int64(2))

	var r417, r418 HRDormRecord
	require.NoError(t, db.Where("id = ?", "room417").First(&r417).Error)
	require.NoError(t, db.Where("id = ?", "room418").First(&r418).Error)
	require.Equal(t, "持睿汽车部件有限公司", hrDormStr(r417.CustomData, "所属公司"))
	require.Equal(t, "持睿汽车部件有限公司", hrDormStr(r417.CustomData, "租赁单位"))
	require.Equal(t, "持睿汽车部件有限公司", hrDormStr(r418.CustomData, "所属公司"), "已是新名不重复改")

	var m types.UtilityMeter
	require.NoError(t, db.Where("id = ?", "m417").First(&m).Error)
	require.Equal(t, "持睿汽车部件有限公司", m.OwnerUnit)
	require.Equal(t, "持睿汽车部件有限公司", m.UseUnit)
}
