// 第 61 轮回归：宿舍五标签数据与分摊、离职证明二进制兜底与无后缀路由、明细三段表契约。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// 第 61 轮：五胶囊字段契约（与前端 hrDorm.ts 同源）。
func TestRound61DormFieldContract(t *testing.T) {
	subs := hrDormSeedSubs()
	require.Len(t, subs, 66, "15 + 17 + 15 + 10 + 9")
	count := map[string]int{}
	for _, s := range subs {
		count[s.Group]++
	}
	require.Equal(t, 15, count[hrDormGroupRoom])
	require.Equal(t, 17, count[hrDormGroupPublic])
	require.Equal(t, 15, count[hrDormGroupStay])
	require.Equal(t, 10, count[hrDormGroupAsset])
	require.Equal(t, 9, count[hrDormGroupFee])

	// 状态列白名单：每个胶囊只允许自己的状态字段
	allowed, ok := hrAllowedStatusOf60(hrDormGroupRoom)
	require.True(t, ok)
	require.Equal(t, []string{"宿舍状态"}, allowed)
	allowed, ok = hrAllowedStatusOf60(hrDormGroupFee)
	require.True(t, ok)
	require.Equal(t, []string{"账单状态"}, allowed)
	allowed, ok = hrAllowedStatusOf60(hrDormGroupStay)
	require.True(t, ok)
	require.Equal(t, []string{"住宿状态"}, allowed)
}

// 第 61 轮：种子 + 首月账单 + 公租房合计（图 7 口径）。
func TestRound61DormSeedAndBill(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}))
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "李思远", EmployeeID: "C020300202",
		Department: "销售部", Status: hrRosterStatusActive,
	}).Error)

	h := &HRHandler{db: db}
	require.NoError(t, h.hrSeedDormForTenant(ctx, 1))
	// 幂等：再播一次不新增
	require.NoError(t, h.hrSeedDormForTenant(ctx, 1))

	var dorm, public, stay, asset int64
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", 1, hrDormModuleRoom).Count(&dorm)
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", 1, hrDormModulePublic).Count(&public)
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", 1, hrDormModuleStay).Count(&stay)
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND module = ?", 1, hrDormModuleAsset).Count(&asset)
	require.Equal(t, int64(18), dorm)
	require.Equal(t, int64(10), public)
	require.Equal(t, int64(9), stay)
	require.Equal(t, int64(8), asset)

	// 公租房合计 = 房租 2915 / 物业 486 / 面积 485.55 / 首月 3401 / 季度 10203
	var publics []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, hrDormModulePublic).Find(&publics).Error)
	rent, prop, area := 0.0, 0.0, 0.0
	for _, p := range publics {
		rent += hrDormFloat(p.CustomData, "租金(元/月)")
		prop += hrDormFloat(p.CustomData, "物业(元/月)")
		area += hrDormFloat(p.CustomData, "建筑面积(㎡)")
	}
	require.InDelta(t, 2915, rent, 0.001)
	require.InDelta(t, 486, prop, 0.001)
	require.InDelta(t, 485.55, area, 0.001)
	require.InDelta(t, 3401, rent+prop, 0.001)
	require.InDelta(t, 10203, (rent+prop)*3, 0.001)

	// 2026-10 账单：9 条个人明细，金额与房间汇总闭合
	var bill HRDormBill
	require.NoError(t, db.Where("tenant_id = ? AND month_label = ?", 1, "2026-10").First(&bill).Error)
	require.Equal(t, 9, bill.Headcount)
	var items []HRDormBillItem
	require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&items).Error)
	require.Len(t, items, 9)
	sum := 0.0
	for _, it := range items {
		require.InDelta(t, it.Rent+it.Property+it.Water+it.Elec+it.Gas, it.Total, 0.02)
		sum += it.Total
	}
	require.InDelta(t, bill.GrandTotal, sum, 0.05, "个人分摊合计必须与账单总额闭合（尾差 ≤ 0.05）")
	require.InDelta(t, bill.RentTotal+bill.PropertyTotal+bill.UtilityTotal, bill.GrandTotal, 0.02)
}

// 第 61 轮：按入住天数分摊 + 尾差闭合（同房间两人 20/10 天）。
func TestRound61DormBillProration(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModulePublic, Code: "5栋2-6", Name: "两山景苑公租房5栋2-6",
		CustomData: map[string]any{
			"房屋编号": "5栋2-6", "房屋地址": "两山景苑公租房5栋2-6",
			"租金(元/月)": 296, "物业(元/月)": 49,
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormReading{
		ID: "rd1", TenantID: 1, MonthLabel: "2026-09", RoomCode: "5栋2-6",
		ElecKwh: 100, ElecFee: 92, WaterTon: 10, WaterFee: 35, GasM3: 5, GasFee: 10.5,
	}).Error)
	mk := func(id, name, in string) {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: id, TenantID: 1, Module: hrDormModuleStay, Name: name, Status: "在住",
			CustomData: map[string]any{
				"员工姓名": name, "所属部门": "生产部", "住宿状态": "在住",
				"房源类型": "公租房", "宿舍编号/房屋地址": "两山景苑公租房5栋2-6",
				"入住日期": in,
			},
		}).Error)
	}
	mk("s1", "甲", "2026-09-01")
	mk("s2", "乙", "2026-09-21") // 10 天

	bill, items, err := h.hrBuildDormBill(ctx, 1, "2026-09")
	require.NoError(t, err)
	require.Equal(t, 2, len(items))
	// 总成本 = 296 + 49 + 92 + 35 + 10.5 = 482.5
	require.InDelta(t, 296, bill.RentTotal, 0.001)
	require.InDelta(t, 137.5, bill.UtilityTotal, 0.001)
	require.InDelta(t, 482.5, bill.GrandTotal, 0.01)

	var first, second HRDormBillItem
	for _, it := range items {
		if it.EmployeeName == "甲" {
			first = it
		} else {
			second = it
		}
	}
	require.Equal(t, 30, first.StayDays)
	require.Equal(t, 10, second.StayDays)
	// 甲 30/40 = 75%，乙 25%（最后一人吸收尾差）
	require.InDelta(t, 482.5*0.75, first.Total, 0.02)
	require.InDelta(t, 482.5*0.25, second.Total, 0.02)
	require.InDelta(t, 482.5, first.Total+second.Total, 0.02)
}

// 第 61 轮：没有住宿记录的月份不得生成空账单。
func TestRound61DormBillRequiresStay(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}))
	h := &HRHandler{db: db}
	_, _, err := h.hrBuildDormBill(ctx, 1, "2026-08")
	require.Error(t, err)
	require.Contains(t, err.Error(), "该月份暂无住宿记录")
}

// 第 61 轮：离职证明 → 库 / 磁盘 / 动态生成 PNG 三级兜底 + 无后缀路由 200。
func TestRound61ResignProofFallback(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRResignProofFile{}))
	h := &HRHandler{db: db}

	name := "daaf5f1e-0a4c-497f-a103-e15f8d20ff6d.png"
	data, ct := h.hrResignProofBytes(ctx, name)
	require.Equal(t, "image/png", ct)
	require.True(t, len(data) > 2000, "必须生成一张可读的 PNG")
	require.Equal(t, []byte{0x89, 'P', 'N', 'G'}, data[:4], "PNG 魔术字节")

	// 已入库存：第二次直接从库里取同一份字节
	again, ct2 := h.hrResignProofBytes(ctx, name)
	require.Equal(t, "image/png", ct2)
	require.True(t, bytes.Equal(data, again))

	var n int64
	require.NoError(t, db.Model(&HRResignProofFile{}).Where("filename = ?", name).Count(&n).Error)
	require.Equal(t, int64(1), n)

	// 无后缀路由：路径里没有 .png，返回 200 + image/png
	r := hrRound60Router(t, h)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hr/roster/resign-proof-view?id="+name, nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "image/png")
	require.Equal(t, data, w.Body.Bytes())

	// 旧路径兼容
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/hr/roster/resign-proof/"+name, nil))
	require.Equal(t, http.StatusOK, w2.Code)

	// 非法编号仍然拒绝
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/hr/roster/resign-proof-view?id=../../etc/passwd", nil))
	require.Equal(t, http.StatusBadRequest, w3.Code)
}

// 第 61 轮：宿舍记录 CRUD + 床位/状态派生（在住人数）。
func TestRound61DormRecordDerived(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{}, &HRDormReading{}))
	h := &HRHandler{db: db}

	rec := HRDormRecord{
		ID: "d1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍302", Name: "宿舍302",
		CustomData: map[string]any{"宿舍编号": "宿舍302", "床位数": 4, "宿舍状态": "空闲"},
	}
	require.NoError(t, db.Create(&rec).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "s1", TenantID: 1, Module: hrDormModuleStay, Name: "甲", Status: "在住",
		CustomData: map[string]any{
			"员工姓名": "甲", "住宿状态": "在住", "宿舍编号/房屋地址": "宿舍302",
		},
	}).Error)

	occ := h.hrDormOccupancy(ctx, 1)
	view := h.hrDormRecordView(hrDormModuleRoom, rec, occ)
	require.Equal(t, 3, view["可用床位数"])
	require.Equal(t, "部分入住", view["宿舍状态"])

	// 满员
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: "sx" + string(rune('a'+i)), TenantID: 1, Module: hrDormModuleStay, Status: "在住",
			CustomData: map[string]any{"住宿状态": "在住", "宿舍编号/房屋地址": "宿舍302"},
		}).Error)
	}
	view = h.hrDormRecordView(hrDormModuleRoom, rec, h.hrDormOccupancy(ctx, 1))
	require.Equal(t, 0, view["可用床位数"])
	require.Equal(t, "满员", view["宿舍状态"])

	// 维修中优先于满员
	rec.CustomData["宿舍状态"] = "维修中"
	view = h.hrDormRecordView(hrDormModuleRoom, rec, h.hrDormOccupancy(ctx, 1))
	require.Equal(t, "维修中", view["宿舍状态"])

	// 出参必须是可直接 JSON 序列化的扁平结构（前端按中文列名取值）
	_, err := json.Marshal(view)
	require.NoError(t, err)
}
