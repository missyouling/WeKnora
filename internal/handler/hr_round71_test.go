package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 第 71 轮：持睿 11 间「表房合一」—— 房档改用原能耗真实表计，虚拟表号退役。
func TestRound71AlignChiruiDormMeters(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()

	// 原能耗真实表计：持睿403 = E021/W021、持睿404 = E022/W022
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m-e21", TenantID: 1, Category: "electricity", Alias: "持睿403",
		MeterNo: "E021", MeterKind: "dorm", Rate: 1, DefaultUnitPrice: 1,
		UseUnit: "持睿汽车部件有限公司", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m-w21", TenantID: 1, Category: "water", Alias: "持睿403",
		MeterNo: "W021", MeterKind: "dorm", Rate: 1, DefaultUnitPrice: 5.22,
		UseUnit: "持睿汽车部件有限公司", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "m-e22", TenantID: 1, Category: "electricity", Alias: "持睿404",
		MeterNo: "E022", MeterKind: "dorm", Rate: 1, DefaultUnitPrice: 1,
		UseUnit: "持睿汽车部件有限公司", Enabled: true,
	}).Error)

	// 房档：403/404 已被第 70 轮写成虚拟表号 E403/W403，405 是星达房（不该被动到）
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d403", TenantID: 1, Module: "dorm", Code: "宿舍403",
		CustomData: map[string]any{
			"房间号": "403", "所属公司": "重庆星达铜业有限公司",
			"宿舍电表名称": "宿舍403", "宿舍电表号": "E403",
			"宿舍水表名称": "宿舍403", "宿舍水表号": "W403",
			"水费单价(元/m³)": 5,
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d404", TenantID: 1, Module: "dorm", Code: "宿舍404",
		CustomData: map[string]any{
			"房间号": "404", "所属公司": "重庆星达铜业有限公司",
			"宿舍电表名称": "宿舍404", "宿舍电表号": "E404",
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d405", TenantID: 1, Module: "dorm", Code: "宿舍405",
		CustomData: map[string]any{
			"房间号": "405", "所属公司": "重庆星达铜业有限公司",
			"宿舍电表名称": "宿舍405", "宿舍电表号": "E405",
		},
	}).Error)

	h := &HRHandler{db: db}
	n, err := h.hrAlignChiruiDormMeters71(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 2, n, "只有 403/404 命中持睿表计")

	var rooms []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, "dorm").Find(&rooms).Error)
	byNo := map[string]HRDormRecord{}
	for _, r := range rooms {
		byNo[hrDormStr(r.CustomData, "房间号")] = r
	}

	r403 := byNo["403"]
	require.Equal(t, "持睿汽车部件有限公司", hrDormStr(r403.CustomData, "所属公司"))
	require.Equal(t, "持睿403", hrDormStr(r403.CustomData, "宿舍电表名称"))
	require.Equal(t, "E021", hrDormStr(r403.CustomData, "宿舍电表号"), "虚拟表号 E403 必须被真实表号替换")
	require.Equal(t, "W021", hrDormStr(r403.CustomData, "宿舍水表号"))
	require.InDelta(t, 5.22, hrDormFloat(r403.CustomData, "水费单价(元/m³)"), 0.001,
		"单价要取原持睿表计（5.220），不是房档默认 5")

	r404 := byNo["404"]
	require.Equal(t, "E022", hrDormStr(r404.CustomData, "宿舍电表号"))

	// 非持睿房（405）不得被改动
	r405 := byNo["405"]
	require.Equal(t, "重庆星达铜业有限公司", hrDormStr(r405.CustomData, "所属公司"))
	require.Equal(t, "E405", hrDormStr(r405.CustomData, "宿舍电表号"))

	// 幂等：再跑一次零变更
	n2, err := h.hrAlignChiruiDormMeters71(ctx, 1)
	require.NoError(t, err)
	require.Zero(t, n2, "第二次必须零变更（幂等）")

	// 原能耗表计一条不删、金额/单价不变（数据安全红线）
	var meters []types.UtilityMeter
	require.NoError(t, db.Where("tenant_id = ?", 1).Find(&meters).Error)
	require.Len(t, meters, 3)
}

// 第 71 轮：表号为空的房源不得进入核算表计源（否则下拉出现空白项）。
func TestRound71SkipEmptyPropertyMeters(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()

	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d417", TenantID: 1, Module: "dorm", Code: "宿舍417",
		CustomData: map[string]any{"房间号": "417", "宿舍电表名称": "", "宿舍电表号": ""},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d403", TenantID: 1, Module: "dorm", Code: "宿舍403",
		CustomData: map[string]any{
			"房间号": "403", "宿舍电表名称": "持睿403", "宿舍电表号": "E021",
			"电费标准(元/度)": 1.0, "电表倍率": 1.0, "所属公司": "持睿汽车部件有限公司",
		},
	}).Error)
	// 表名有值但表号为空：同样不能进下拉（没有表号无法挂账）
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d419", TenantID: 1, Module: "dorm", Code: "宿舍419",
		CustomData: map[string]any{"房间号": "419", "宿舍电表名称": "宿舍419", "宿舍电表号": ""},
	}).Error)

	h := &UtilityHandler{db: db}
	out := h.appendPropertyMeters70(ctx, 1, "electricity", nil)
	require.Len(t, out, 1, "只应有 403 一张表")
	require.Equal(t, "E021", out[0].MeterNo)
	require.Equal(t, "持睿403", out[0].Alias)
	require.Equal(t, "持睿汽车部件有限公司", out[0].UseUnit)
}
