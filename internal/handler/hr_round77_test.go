// 第 77 轮回归：
//  1. 宿舍/公租房档案补「表计类型」字段与存量取值（幂等、只填缺）
//  2. 房档表计查重只认工商业表计（不再撞历史宿舍/公租房残留）
//  3. 提取引擎不再因 required 开关阻断（正则兜底仍在）
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestRound77SeedMeterType(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-dorm", TenantID: 1, Scope: "hr-dorm", Name: "宿舍管理", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "房间号", Enabled: true, IsDefault: true, DataType: "text", Group: "宿舍档案"},
			{Name: "公租房编号", Enabled: true, IsDefault: true, DataType: "text", Group: "公租房档案"},
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍403",
		CustomData: map[string]any{"房间号": "403"},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "p1", TenantID: 1, Module: hrDormModulePublic, Code: "公租房1号",
		CustomData: map[string]any{"房间号": "1号"},
	}).Error)
	// 用户已手工改过的类型不应被覆盖
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r2", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍404",
		CustomData: map[string]any{"房间号": "404", "表计类型": "自用"},
	}).Error)

	h := &HRHandler{db: db}
	dorm, public := h.hrSeedMeterType77(ctx, 1)
	require.Equal(t, 1, dorm)
	require.Equal(t, 1, public)

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-dorm").First(&cat).Error)
	require.True(t, hrHasSubInGroup77(cat.Subs, "宿舍档案", "表计类型"))
	require.True(t, hrHasSubInGroup77(cat.Subs, "公租房档案", "表计类型"))

	var r1, r2, p1 HRDormRecord
	require.NoError(t, db.Where("id = ?", "r1").First(&r1).Error)
	require.Equal(t, "宿舍", hrDormStr(r1.CustomData, "表计类型"))
	require.NoError(t, db.Where("id = ?", "r2").First(&r2).Error)
	require.Equal(t, "自用", hrDormStr(r2.CustomData, "表计类型"), "用户手工改过的类型不得被覆盖")
	require.NoError(t, db.Where("id = ?", "p1").First(&p1).Error)
	require.Equal(t, "公租房", hrDormStr(p1.CustomData, "表计类型"))

	// 幂等：再跑一次不重复加字段、不重复改记录
	dorm2, public2 := h.hrSeedMeterType77(ctx, 1)
	require.Equal(t, 0, dorm2)
	require.Equal(t, 0, public2)
	require.NoError(t, db.Where("id = ?", "cat-dorm").First(&cat).Error)
	count := 0
	for _, s := range cat.Subs {
		if s.Name == "表计类型" && s.Group == "宿舍档案" {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func TestRound77MeterPoolOnlyIndustrial(t *testing.T) {
	db := hrRound70DB(t)
	ctx := context.Background()
	// 历史残留：utility_meters 里还有一条宿舍表（持睿403），别名与表号都和房档相同
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "u403", TenantID: 1, Category: "electricity", Alias: "持睿403",
		MeterNo: "E021", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "u-e301", TenantID: 1, Category: "electricity", Alias: "星达配电分表1",
		MeterNo: "E301", MeterKind: "工商业", UseUnit: "重庆星达铜业有限公司", Enabled: true,
	}).Error)
	h := &HRHandler{db: db}

	// 编辑房间时填回自己的表名/表号：不得被 utility_meters 的宿舍残留挡住
	require.NoError(t, h.hrDormValidateMeters72(ctx, 1, hrDormModuleRoom, "r403",
		map[string]any{"房间号": "403", "宿舍电表号": "E021", "宿舍电表名称": "持睿403"}))

	// 新增房间写工商业已占用的表号：仍然拦截
	err := h.hrDormValidateMeters72(ctx, 1, hrDormModuleRoom, "",
		map[string]any{"房间号": "405", "宿舍电表号": "E301", "宿舍电表名称": "宿舍405"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "E301")
}
