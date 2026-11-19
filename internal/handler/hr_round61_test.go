// 第 61 轮回归：宿舍五标签数据与分摊、离职证明二进制兜底与无后缀路由、明细三段表契约。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 第 61 轮：五胶囊字段契约（与前端 hrDorm.ts 同源）。
func TestRound61DormFieldContract(t *testing.T) {
	subs := hrDormSeedSubs()
	// 第 64 轮字段契约：宿舍 26 / 公租房 30 / 住宿 27 / 资产 16 / 费用 15（月度账单汇总）
	count := map[string]int{}
	for _, s := range subs {
		count[s.Group]++
	}
	require.Equal(t, len(subs), count[hrDormGroupRoom]+count[hrDormGroupPublic]+
		count[hrDormGroupStay]+count[hrDormGroupAsset]+count[hrDormGroupFee])
	require.Equal(t, 26, count[hrDormGroupRoom])
	require.Equal(t, 30, count[hrDormGroupPublic])
	require.Equal(t, 27, count[hrDormGroupStay])
	require.Equal(t, 16, count[hrDormGroupAsset])
	require.Equal(t, 15, count[hrDormGroupFee])

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
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormBill{}, &HRDormBillItem{},
		&HRDormReading{}, &HRDormCharge{}))
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
	require.Equal(t, int64(22), dorm, "22 间厂区宿舍（301~320 / 417 / 419）")
	require.Equal(t, int64(19), public, "两山秀苑 9 + 两山景苑 10")
	require.Equal(t, int64(40), stay, "38 名在住员工 + 夫妻房配偶 2 人")
	require.Equal(t, int64(8), asset)

	// 两山景苑 10 套：租金 2915 / 物业 486 / 面积 485.55 / 月费用 3401 / 季度 10203
	var publics []HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ?", 1, hrDormModulePublic).Find(&publics).Error)
	rent, prop, area := 0.0, 0.0, 0.0
	for _, p := range publics {
		if !strings.Contains(hrDormStr(p.CustomData, "房屋地址"), "两山景苑") {
			continue
		}
		rent += hrDormFloat(p.CustomData, "房屋租金(元/月)")
		prop += hrDormFloat(p.CustomData, "物业费(元/月)")
		area += hrDormFloat(p.CustomData, "建筑面积(㎡)")
	}
	require.InDelta(t, 2915, rent, 0.001)
	require.InDelta(t, 486, prop, 0.001)
	require.InDelta(t, 485.55, area, 0.001)
	require.InDelta(t, 3401, rent+prop, 0.001)
	require.InDelta(t, 10203, (rent+prop)*3, 0.001)

	// 床上用品：床位数按房型派生（夫妻房 2 / 单人间 1）
	var room HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND module = ? AND code = ?", 1, hrDormModuleRoom, "宿舍304").
		First(&room).Error)
	require.Equal(t, "夫妻房", hrDormStr(room.CustomData, "房型"))
	require.Equal(t, 2, int(hrDormFloat(room.CustomData, "床位数")))
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
//
// 第 63 轮变更：磁盘优先且**命中磁盘不回写库**（原来每次读 3.5MB 再跨区 upsert，预览 60s+）。
// 因此这里把附件目录切到临时目录，按「生成 → 库回退落盘 → 磁盘命中只读」三段验证。
func TestRound61ResignProofFallback(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRResignProofFile{}))
	h := &HRHandler{db: db}

	originDir := hrResignProofDir
	hrResignProofDir = t.TempDir()
	t.Cleanup(func() { hrResignProofDir = originDir })

	name := "daaf5f1e-0a4c-497f-a103-e15f8d20ff6d.png"
	data, ct := h.hrResignProofBytes(ctx, name)
	require.Equal(t, "image/png", ct)
	require.True(t, len(data) > 2000, "必须生成一张可读的 PNG")
	require.Equal(t, []byte{0x89, 'P', 'N', 'G'}, data[:4], "PNG 魔术字节")

	// 首次：磁盘与库都没有 → 现场生成并入库
	var n int64
	require.NoError(t, db.Model(&HRResignProofFile{}).Where("filename = ?", name).Count(&n).Error)
	require.Equal(t, int64(1), n)

	// 第二次：库里命中 → 同一份字节，同时补一份磁盘副本
	again, ct2 := h.hrResignProofBytes(ctx, name)
	require.Equal(t, "image/png", ct2)
	require.True(t, bytes.Equal(data, again))
	_, statErr := os.Stat(filepath.Join(hrResignProofDir, name))
	require.NoError(t, statErr, "读库命中后应补落磁盘")

	// 第三次：删掉库记录，磁盘仍能独立供数，且不得反向写库（第 63 轮性能契约）
	require.NoError(t, db.Where("filename = ?", name).Delete(&HRResignProofFile{}).Error)
	fromDisk, ct3 := h.hrResignProofBytes(ctx, name)
	require.Equal(t, "image/png", ct3)
	require.True(t, bytes.Equal(data, fromDisk), "磁盘副本应与库内字节一致")
	n = 0
	require.NoError(t, db.Model(&HRResignProofFile{}).Where("filename = ?", name).Count(&n).Error)
	require.Equal(t, int64(0), n, "磁盘命中不得回写数据库")
	require.NoError(t, h.hrSaveProofFile(ctx, name, "image/png", data))

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
