// 第 64 轮回归：社保导入月份优先级、退宿可逆恢复、费用信息月度账单汇总。
package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 第 64 轮：账单月份优先级 = 显式手选 > 文件名 > 当前月。
func TestRound64ImportMonthPriority(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.Local)

	// 手选月份压倒文件名（线上 bug 的核心场景）
	require.Equal(t, "2026-10", hrResolveImportMonth("2026-10", "2026-11", now))
	// 未手选 → 用文件名里的费款所属期
	require.Equal(t, "2026-09", hrResolveImportMonth("", "2026-09", now))
	// 都没有 → 当前自然月
	require.Equal(t, "2026-11", hrResolveImportMonth("", "", now))
	// 脏值不落库：非法手选视同未填，回退文件名
	require.Equal(t, "2026-08", hrResolveImportMonth("2026-13", "2026-08", now))
	// 手选接受 2026/10 与 2026-10 两种写法
	require.Equal(t, "2026-10", hrResolveImportMonth("2026/10", "", now))
}

// 第 64 轮：退宿可逆 —— 恢复住宿必须重新校验容量、清空退宿信息、必要时重排床位。
func TestRound64RestoreStay(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HREmployee{}, &types.SystemSetting{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
		CustomData: map[string]any{
			"宿舍编号": "宿舍301", "床位数": 2, "房型": "双人间", "性别限制": "男",
			"宿舍状态": "部分入住",
		},
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", EmployeeID: "C1", Gender: "男", Status: "在职",
		IDNumber: "500101199001010011",
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e2", TenantID: 1, Name: "李丽", EmployeeID: "C2", Gender: "女", Status: "在职",
		IDNumber: "500101199202020022",
	}).Error)

	// 一条已退宿记录（床位仍写着 1号床），一条在住占用 1号床
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay1", TenantID: 1, Module: hrDormModuleStay, Code: "宿舍301", Status: "已退宿",
		CustomData: map[string]any{
			"员工姓名": "张三", "宿舍编号/房屋地址": "宿舍301", "床位号": "1号床",
			"住宿状态": "已退宿", "退宿日期": "2026-10-31", "退宿原因": "个人外住",
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay2", TenantID: 1, Module: hrDormModuleStay, Code: "宿舍301", Status: "在住",
		CustomData: map[string]any{
			"员工姓名": "李丽", "宿舍编号/房屋地址": "宿舍301", "床位号": "1号床",
			"住宿状态": "在住",
		},
	}).Error)

	bed, err := h.hrRestoreStay(ctx, 1, "stay1")
	require.NoError(t, err)
	require.Equal(t, "2号床", bed, "原床位被占用时必须重排到下一个空床")

	var restored HRDormRecord
	require.NoError(t, db.Where("id = ?", "stay1").First(&restored).Error)
	require.Equal(t, "在住", hrDormStr(restored.CustomData, "住宿状态"))
	require.Equal(t, "在住", restored.Status)
	require.Equal(t, "", hrDormStr(restored.CustomData, "退宿日期"))
	require.Equal(t, "", hrDormStr(restored.CustomData, "退宿原因"))
	require.Equal(t, "2号床", hrDormStr(restored.CustomData, "床位号"))

	// 房源已满：新建一条已退宿记录，房间 2/2 → 必须拒绝
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay3", TenantID: 1, Module: hrDormModuleStay, Code: "宿舍301", Status: "已退宿",
		CustomData: map[string]any{
			"员工姓名": "张三", "宿舍编号/房屋地址": "宿舍301", "床位号": "1号床", "住宿状态": "已退宿",
		},
	}).Error)
	_, err = h.hrRestoreStay(ctx, 1, "stay3")
	require.Error(t, err)
	require.Contains(t, err.Error(), "已住满")

	// 性别不符：女员工住「仅限男」房源 → 拒绝
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room2", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍302", Name: "宿舍302",
		CustomData: map[string]any{"宿舍编号": "宿舍302", "床位数": 2, "房型": "双人间", "性别限制": "男"},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay4", TenantID: 1, Module: hrDormModuleStay, Code: "宿舍302", Status: "已退宿",
		CustomData: map[string]any{
			"员工姓名": "李丽", "宿舍编号/房屋地址": "宿舍302", "床位号": "", "住宿状态": "已退宿",
		},
	}).Error)
	_, err = h.hrRestoreStay(ctx, 1, "stay4")
	require.Error(t, err)
	require.Contains(t, err.Error(), "仅限男性")

	// 非退宿记录：直接拒绝，不做无意义的状态改写
	_, err = h.hrRestoreStay(ctx, 1, "stay2")
	require.Error(t, err)
	require.Contains(t, err.Error(), "无需恢复")
}

// 第 64 轮：费用信息按月汇总 —— 每月 1 行，金额按分累加，住宿人数按房源去重。
func TestRound64ChargeBillsAggregation(t *testing.T) {
	rows := []HRDormCharge{
		{MonthLabel: "2026-10", EmployeeID: "C1", Name: "张三", RoomNo: "宿舍301",
			Deposit: 100.1, Rent: 0.2, Water: 12.34, Elec: 56.78, Gas: 9.99,
			Social: 1000.01, Fund: 500.02, Other: 3.03, Total: 1682.47, Refund: 10, Subsidy: 20},
		{MonthLabel: "2026-10", EmployeeID: "C2", Name: "李丽", RoomNo: "",
			Social: 1000.01, Fund: 500.02, Other: 0, Total: 1500.03},
		{MonthLabel: "2026-11", EmployeeID: "C1", Name: "张三", RoomNo: "宿舍301",
			Rent: 1200, Social: 1000, Fund: 500, Total: 2700},
		// 同月同人重复行：人数只算一次，金额仍累加
		{MonthLabel: "2026-10", EmployeeID: "C1", Name: "张三", RoomNo: "宿舍301", Other: 1, Total: 1},
	}
	bills := hrAggregateDormChargeBills(rows)
	require.Len(t, bills, 2, "每月仅 1 行")
	require.Equal(t, "2026-11", bills[0]["month_label"], "月份降序")

	oct := bills[1]
	require.Equal(t, 2, oct["总人数"])
	require.Equal(t, 1, oct["住宿人数"])
	require.InDelta(t, 100.10, oct["押金合计"], 0.001)
	require.InDelta(t, 0.20, oct["房租合计"], 0.001)
	require.InDelta(t, 12.34, oct["水费合计"], 0.001)
	require.InDelta(t, 56.78, oct["电费合计"], 0.001)
	require.InDelta(t, 9.99, oct["气费合计"], 0.001)
	require.InDelta(t, 2000.02, oct["社保个人合计"], 0.001)
	require.InDelta(t, 1000.04, oct["公积金个人合计"], 0.001)
	require.InDelta(t, 4.03, oct["其它扣款合计"], 0.001)
	require.InDelta(t, 3183.50, oct["扣款总计"], 0.001)
	require.InDelta(t, 10.00, oct["退款合计"], 0.001)
	require.InDelta(t, 20.00, oct["补贴合计"], 0.001)
}
