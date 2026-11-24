// 第 65 轮回归：社保记录删除、入住准入校验、保存强类型校验、资产字段迁移。
package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 第 65 轮：保存强类型校验（金额/整数/月份）。
func TestRound65DormDataValidation(t *testing.T) {
	ok := map[string]any{
		"宿舍编号": "宿舍301", "床位数": "2", "房间面积(㎡)": "28.5",
		"租期起始月份": "2026-01", "租期截止月份": "2026-12",
		"电费标准(元/度)": "0.6185",
	}
	require.Equal(t, "", hrValidateDormData65(ok))

	bad := []map[string]any{
		{"房间面积(㎡)": "打撒'"},
		{"电费标准(元/度)": "饿243"},
		{"水费单价(元/m³)": "34￥￥@"},
		{"宿舍押金(元)": "-100"},
		{"床位数": "2.5"},
		{"数量": "3个"},
		{"租期起始月份": "53q"},
		{"租期截止月份": "5353"},
		{"账单月份": "2026-13"},
	}
	for _, m := range bad {
		require.NotEqual(t, "", hrValidateDormData65(m), "应拦截: %v", m)
	}

	// 起止月份逻辑校验
	require.Contains(t, hrValidateDormData65(map[string]any{
		"租期起始月份": "2026-06", "租期截止月份": "2026-01",
	}), "不能早于")

	// 空值一律放行（空值显示为空白，不做格式校验）
	require.Equal(t, "", hrValidateDormData65(map[string]any{"房间面积(㎡)": "", "床位数": ""}))
}

// 第 65 轮：入住准入 —— 满员 / 维修中 一律拒绝，调宿排除自身。
func TestRound65DormAdmitCheck(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HREmployee{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	mkRoom := func(id, code string, beds int, status string) {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: id, TenantID: 1, Module: hrDormModuleRoom, Code: code, Name: code,
			CustomData: map[string]any{"宿舍编号": code, "床位数": beds, "宿舍状态": status},
		}).Error)
	}
	mkStay := func(id, room, bed string) {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: id, TenantID: 1, Module: hrDormModuleStay, Code: room, Status: "在住",
			CustomData: map[string]any{
				"宿舍编号/房屋地址": room, "床位号": bed, "住宿状态": "在住",
			},
		}).Error)
	}

	mkRoom("r1", "宿舍401", 1, "")
	mkStay("s1", "宿舍401", "1号床")
	// 1/1 已满
	err := h.hrDormAdmitCheck(ctx, 1, "宿舍401", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "已住满")
	// 排除自身后允许（调宿到同一间）
	require.NoError(t, h.hrDormAdmitCheck(ctx, 1, "宿舍401", "s1"))

	// 维修中
	mkRoom("r2", "宿舍402", 4, "维修中")
	err = h.hrDormAdmitCheck(ctx, 1, "宿舍402", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "维修中")

	// 空闲可住
	mkRoom("r3", "宿舍403", 2, "")
	require.NoError(t, h.hrDormAdmitCheck(ctx, 1, "宿舍403", ""))

	// 房源不存在
	err = h.hrDormAdmitCheck(ctx, 1, "宿舍999", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "找不到房源")
}

// 第 68 轮起：删除社保记录**不再重算/删除账单**（账单是不可变快照，只打脏标记）。
// 该契约的回归用例迁移到 hr_round68_test.go 的 TestRound68BillSnapshotImmutable。

// 第 65 轮：资产字段迁移 —— 维修 4 字段清空、资产状态转可编辑 enum。
func TestRound65AssetFieldMigration(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&types.FleetCategory{}, &HRDormRecord{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "c1", TenantID: 1, Scope: hrScopeDorm, Name: "宿舍管理", BuiltinKey: "hr-dorm",
		Subs: []types.FleetCategorySub{
			{Name: "资产编号", Enabled: true, DataType: "text", Group: hrDormGroupAsset},
			{Name: "报修日期", Enabled: true, DataType: "date", Group: hrDormGroupAsset},
			{Name: "维修状态", Enabled: true, DataType: "enum", Group: hrDormGroupAsset},
			{Name: "维修费用(元)", Enabled: true, DataType: "number", Group: hrDormGroupAsset},
			{Name: "维修说明", Enabled: true, DataType: "textarea", Group: hrDormGroupAsset},
			{Name: "资产状态", Enabled: true, DataType: "system", Builtin: true, Group: hrDormGroupAsset},
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "a1", TenantID: 1, Module: hrDormModuleAsset, Code: "ZC-1", Name: "空调",
		CustomData: map[string]any{
			"资产编号": "ZC-1", "资产名称": "空调", "报修日期": "2026-09-18",
			"维修状态": "已修复", "维修费用(元)": 260.0, "维修说明": "换主板",
		},
	}).Error)

	require.NoError(t, h.hrMigrateAssetFields65(ctx))

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&cat).Error)
	names := map[string]types.FleetCategorySub{}
	for _, s := range cat.Subs {
		names[s.Name] = s
	}
	for _, gone := range []string{"报修日期", "维修状态", "维修费用(元)", "维修说明"} {
		_, exists := names[gone]
		require.False(t, exists, "维修字段 %s 必须被删除", gone)
	}
	st, ok := names["资产状态"]
	require.True(t, ok, "资产状态必须保留")
	require.Equal(t, "enum", st.DataType)
	require.False(t, st.Builtin, "资产状态必须解除内置锁定")
	require.True(t, st.Enabled)

	var rec HRDormRecord
	require.NoError(t, db.Where("id = ?", "a1").First(&rec).Error)
	for _, gone := range []string{"报修日期", "维修状态", "维修费用(元)", "维修说明"} {
		_, exists := rec.CustomData[gone]
		require.False(t, exists, "存量记录里的 %s 必须清除", gone)
	}
	require.Equal(t, "正常", hrDormStr(rec.CustomData, "资产状态"), "缺资产状态时补默认值")

	// 幂等：再跑一次不应重复插入
	require.NoError(t, h.hrMigrateAssetFields65(ctx))
	var cat2 types.FleetCategory
	require.NoError(t, db.Where("id = ?", "c1").First(&cat2).Error)
	require.Equal(t, len(cat.Subs), len(cat2.Subs))
}

// 第 65 轮：行内编辑后月度汇总口径 —— 其它扣款 +50 应使扣款总计 +50。
func TestRound65ChargeBillAfterInlineEdit(t *testing.T) {
	rows := []HRDormCharge{
		{MonthLabel: "2026-10", EmployeeID: "C1", Name: "张三", RoomNo: "宿舍301",
			Rent: 100, Water: 10, Total: 110},
	}
	before := hrAggregateDormChargeBills(rows)
	require.Len(t, before, 1)
	require.InDelta(t, 110.0, before[0]["扣款总计"], 0.001)

	// 模拟行内编辑：其它扣款 50 → total 160
	rows[0].Other = 50
	rows[0].Total = 160
	after := hrAggregateDormChargeBills(rows)
	require.InDelta(t, 50.0, after[0]["其它扣款合计"], 0.001)
	require.InDelta(t, 160.0, after[0]["扣款总计"], 0.001)
}
