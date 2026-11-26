package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 表计号全局查重：两间房写同一个电表号必须被拦，且提示里带冲突房源。
func TestRound72MeterNumberDuplicate(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r401", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍401", Name: "宿舍401",
		CustomData: map[string]any{"房间号": "401", "宿舍电表号": "E401", "宿舍电表名称": "宿舍401"},
	}).Error)
	h := &HRHandler{db: db}

	err := h.hrDormValidateMeters72(ctx, 1, hrDormModuleRoom, "",
		map[string]any{"房间号": "402", "宿舍电表号": "E401", "宿舍电表名称": "宿舍402"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "E401")
	require.Contains(t, err.Error(), "401")

	// 编辑自身不算冲突
	require.NoError(t, h.hrDormValidateMeters72(ctx, 1, hrDormModuleRoom, "r401",
		map[string]any{"房间号": "401", "宿舍电表号": "E401"}))

	// 空表号不参与查重
	require.NoError(t, h.hrDormValidateMeters72(ctx, 1, hrDormModuleRoom, "",
		map[string]any{"房间号": "403", "宿舍电表号": ""}))
}

// 空置：有在住人员时拒绝开启；无人员时状态=空置、可用床位=0；关闭后状态回四态。
func TestRound72VacantSwitch(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room301", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301",
		CustomData: map[string]any{"房间号": "301", "床位数": "2", "宿舍状态": "空闲"},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay301", TenantID: 1, Module: hrDormModuleStay,
		CustomData: map[string]any{"宿舍编号/房屋地址": "宿舍301", "员工姓名": "吴勇建", "住宿状态": "在住"},
	}).Error)
	h := &HRHandler{db: db}

	// 有人 → 拒绝
	err := h.hrDormValidateRoomMeta72(ctx, 1, "room301", hrDormModuleRoom,
		map[string]any{"房间号": "301", "床位数": "2", "空置": true}, map[string]any{"空置": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "在住")

	// 无人房间可以空置
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room420", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍420",
		CustomData: map[string]any{"房间号": "420", "床位数": "2", "宿舍状态": "空闲"},
	}).Error)
	merged := map[string]any{"房间号": "420", "床位数": "2", "空置": true}
	require.NoError(t, h.hrDormValidateRoomMeta72(ctx, 1, "room420", hrDormModuleRoom, merged, map[string]any{"空置": true}))
	require.Equal(t, "空置", merged["宿舍状态"])
	require.Equal(t, 0, merged["可用床位数"])

	// 关闭空置：显式提交 false 时必须恢复四态（不能停留在「空置」）
	off := map[string]any{"房间号": "420", "床位数": "2", "空置": false, "宿舍状态": "空置"}
	require.NoError(t, h.hrDormValidateRoomMeta72(ctx, 1, "room420", hrDormModuleRoom, off, map[string]any{"空置": false}))
	require.Equal(t, "空闲", off["宿舍状态"])
	require.Equal(t, 2, off["可用床位数"])
}

// 防缩容：床位数 < 当前在住人数必须报错。
func TestRound72CapacityShrink(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	for i, name := range []string{"甲", "乙"} {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: "s" + name, TenantID: 1, Module: hrDormModuleStay,
			CustomData: map[string]any{"宿舍编号/房屋地址": "宿舍305", "员工姓名": name, "住宿状态": "在住", "床位号": string(rune('1'+i)) + "号床"},
		}).Error)
	}
	merged := map[string]any{"房间号": "305", "床位数": "1"}
	err := h.hrDormValidateRoomMeta72(ctx, 1, "r305", hrDormModuleRoom, merged, map[string]any{"房型": "单人间"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "在住")

	// 房型 → 床位数 自动重算
	ok := map[string]any{"房间号": "306", "床位数": "4", "房型": "双人间"}
	require.NoError(t, h.hrDormValidateRoomMeta72(ctx, 1, "r306", hrDormModuleRoom, ok, map[string]any{"房型": "双人间"}))
	require.Equal(t, 2, ok["床位数"])
}

// 床位号唯一：同房同床位在住记录必须被拦；存量重复按入住顺序重排。
func TestRound72BedNoUniqueAndFix(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "b1", TenantID: 1, Module: hrDormModuleStay,
		CustomData: map[string]any{"宿舍编号/房屋地址": "宿舍309", "员工姓名": "刘明", "住宿状态": "在住", "床位号": "1号床", "入住日期": "2026-03-01"},
	}).Error)

	err := h.hrDormValidateBedNo72(ctx, 1, "", "宿舍309", "1号床")
	require.Error(t, err)
	require.Contains(t, err.Error(), "1号床")

	require.NoError(t, h.hrDormValidateBedNo72(ctx, 1, "", "宿舍309", "2号床"))

	// 存量重复：另一条同房同床位 → 迁移重排
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "b2", TenantID: 1, Module: hrDormModuleStay,
		CustomData: map[string]any{"宿舍编号/房屋地址": "宿舍309", "员工姓名": "黄美容", "住宿状态": "在住", "床位号": "1号床", "入住日期": "2026-03-02"},
	}).Error)
	h.hrFixDuplicateBedNo72(ctx, 1)
	var rows []HRDormRecord
	require.NoError(t, db.Where("module = ?", hrDormModuleStay).Find(&rows).Error)
	beds := map[string]string{}
	for _, r := range rows {
		beds[hrDormStr(r.CustomData, "员工姓名")] = hrDormStr(r.CustomData, "床位号")
	}
	require.Equal(t, "1号床", beds["刘明"])
	require.Equal(t, "2号床", beds["黄美容"])
	require.NotEqual(t, beds["刘明"], beds["黄美容"])
}

// 资产字典收敛：分房种子被清掉、同名去重、12 类数量按规范值。
func TestRound72ConsolidateAssetDict(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	// 两条同名「衣柜」（不同编号）模拟种子重复
	for i, code := range []string{"ZC-301-衣柜", "ZC-302-衣柜"} {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: "a" + code, TenantID: 1, Module: hrDormModuleAsset, Code: code, Name: "衣柜",
			CustomData: map[string]any{"资产编号": code, "资产名称": "衣柜", "数量": "1", "计量单位": "个"},
			CreatedAt:  time.Now().UTC().Add(time.Duration(i) * time.Minute),
		}).Error)
	}
	h := &HRHandler{db: db}
	h.hrConsolidateAssetDict72(ctx, 1)

	var rows []HRDormRecord
	require.NoError(t, db.Where("module = ?", hrDormModuleAsset).Find(&rows).Error)
	count := map[string]int{}
	for _, r := range rows {
		count[hrDormStr(r.CustomData, "资产名称")]++
	}
	require.Equal(t, 1, count["衣柜"], "同名只留一条")
	want := map[string]bool{"晾衣杆": true, "毛巾杆": true, "衣柜": true, "床柜": true, "床头柜": true,
		"空调遥控器": true, "空调": true, "电热水器": true, "床": true, "床垫": true, "钥匙": true, "窗帘": true}
	for name := range want {
		require.Equal(t, 1, count[name], "12 类标准物品缺 %s", name)
	}
	for _, r := range rows {
		if hrDormStr(r.CustomData, "资产名称") == "衣柜" {
			require.Equal(t, 6.0, hrDormFloat(r.CustomData, "数量"), "衣柜规范数量=6")
			require.Equal(t, "ZC-003", r.Code)
		}
	}
}

// 住宿记录 3 个「入住时*表度数」字段必须从配置里清掉（六同步）。
func TestRound72DropStayMeterFields(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "stay72", TenantID: 1, Scope: hrScopeDorm, Name: "住宿记录", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "员工姓名", Group: "住宿记录", Enabled: true, DataType: "text"},
			{Name: "入住时水表度数", Group: "住宿记录", Enabled: true, DataType: "number"},
			{Name: "入住时电表度数", Group: "住宿记录", Enabled: true, DataType: "number"},
			{Name: "入住时气表度数", Group: "住宿记录", Enabled: true, DataType: "number"},
			{Name: "住宿记录ID", Group: "住宿记录", Enabled: true, DataType: "text"},
		},
	}).Error)

	h := &HRHandler{db: db}
	h.hrDropStayMeterFields72(ctx, 1)

	var cats []types.FleetCategory
	require.NoError(t, db.Where("id = ?", "stay72").Find(&cats).Error)
	require.Len(t, cats, 1)
	names := []string{}
	for _, s := range cats[0].Subs {
		names = append(names, s.Name)
	}
	require.Equal(t, []string{"员工姓名"}, names, "度数与住宿记录ID 必须被清理")
}
