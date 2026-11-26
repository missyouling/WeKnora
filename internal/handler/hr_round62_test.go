// 第 62 轮回归：房型→床位、费用归一、住宿联动匹配、导入三模式、子表删除路由、签约公司归一。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestRound62BedsAndNormalize(t *testing.T) {
	cases := map[string]int{
		"单人间": 1, "双人间": 2, "夫妻房": 2, "四人间": 4, "六人间": 6, "八人间": 8, "": 1,
	}
	for rt, want := range cases {
		require.Equal(t, want, hrDormBedsOfType(rt), rt)
	}

	// 公租房：月费用单价 = 租金 + 物业；季度 = ×3
	pub := map[string]any{"房屋租金(元/月)": 296.0, "物业费(元/月)": 49.0, "房型": "夫妻房"}
	hrDormNormalizeRecord(hrDormModulePublic, pub)
	require.InDelta(t, 345.0, hrDormFloat(pub, "月费用单价(元/月)"), 0.001)
	require.InDelta(t, 1035.0, hrDormFloat(pub, "季度租金(元/季)"), 0.001)
	require.Equal(t, 2, int(hrDormFloat(pub, "床位数")))
	require.Equal(t, "不限", hrDormStr(pub, "性别限制"), "夫妻房自动不限性别")

	// 资产：编号为空自动生成
	asset := map[string]any{"资产名称": "空调"}
	hrDormNormalizeRecord(hrDormModuleAsset, asset)
	require.Contains(t, hrDormStr(asset, "资产编号"), "ZC-")
}

func TestRound62Identity(t *testing.T) {
	require.Equal(t, "room:宿舍301", hrDormIdentityOf(hrDormModuleRoom,
		map[string]any{"宿舍编号": "宿舍301"}, "", ""))
	require.Equal(t, "public:两山景苑5-2-6", hrDormIdentityOf(hrDormModulePublic,
		map[string]any{"房屋地址": "两山景苑5-2-6"}, "", ""))
	require.Equal(t, "stay:吴勇建@宿舍301", hrDormIdentityOf(hrDormModuleStay,
		map[string]any{"员工姓名": "吴勇建", "宿舍编号/房屋地址": "宿舍301"}, "", ""))
	require.Equal(t, "asset:ZC-1", hrDormIdentityOf(hrDormModuleAsset,
		map[string]any{"资产编号": "ZC-1"}, "", ""))
}

// 住宿记录工号为空时，仍要按姓名命中房间（否则整张费用表房间号全空）。
func TestRound62ChargesMatchStayByName(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	// 第 73 轮：费用改读能耗「真实抄表」，测试同步用 utility_meter 构造。
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormCharge{},
		&types.UtilityMeter{}, &types.UtilityMeterRecord{}, &types.UtilityMeterItem{}))
	h := &HRHandler{db: db}

	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "吴勇建", EmployeeID: "C020610001",
		Department: "生产部", Status: hrRosterStatusActive,
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
		CustomData: map[string]any{
			"宿舍编号": "宿舍301", "房型": "单人间", "床位数": 1,
			"电费标准(元/度)": 1.0, "水费单价(元/m³)": 5.0, "宿舍押金(元)": 200.0,
		},
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "s1", TenantID: 1, Module: hrDormModuleStay, Name: "吴勇建", Status: "在住",
		CustomData: map[string]any{
			"员工姓名": "吴勇建", "住宿状态": "在住", "房源类型": "公司宿舍",
			"宿舍编号/房屋地址": "宿舍301", "入住日期": "2026-01-05",
			"入住时电表度数": 100.0, "入住时水表度数": 10.0,
		},
	}).Error)
	// 房档声明表号，能耗侧才有可引用的真实抄表
	room := HRDormRecord{}
	require.NoError(t, db.Where("id = ?", "r1").First(&room).Error)
	room.CustomData["宿舍电表号"] = "E301"
	room.CustomData["宿舍水表号"] = "W301"
	require.NoError(t, db.Model(&room).Select("custom_data").Updates(&room).Error)
	seedMeter73(t, db, "electricity", "E301", "宿舍301", 10000, 10073, 73, 73)
	seedMeter73(t, db, "water", "W301", "宿舍301", 10, 15, 5, 25)

	rows, err := h.hrBuildDormCharges(ctx, 1, "2026-10")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, "宿舍301", row.RoomNo)
	require.Equal(t, "员工宿舍", row.RoomType)
	require.InDelta(t, 73.0, row.Elec, 0.01)
	require.InDelta(t, 25.0, row.Water, 0.01)
	require.InDelta(t, 10000.0, row.ElecStart, 0.01, "起度取真实抄表")
	require.InDelta(t, 10073.0, row.ElecEnd, 0.01)
}

// 夫妻房配偶分摊比例显式 0：不得回落成 1（否则配偶被重复收全款）。
func TestRound62SpouseRatioZero(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}, &HRDormCharge{},
		&types.UtilityMeter{}, &types.UtilityMeterRecord{}, &types.UtilityMeterItem{}))
	h := &HRHandler{db: db}

	for _, e := range []HREmployee{
		{ID: "e1", TenantID: 1, Name: "董友兵", EmployeeID: "C1", Department: "生产部", Status: hrRosterStatusActive, IDNumber: "500101199001010011"},
		{ID: "e2", TenantID: 1, Name: "吴英", EmployeeID: "C2", Department: "生产部", Status: hrRosterStatusActive, IDNumber: "500101199202020022"},
	} {
		require.NoError(t, db.Create(&e).Error)
	}
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "r1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍304", Name: "宿舍304",
		CustomData: map[string]any{
			"宿舍编号": "宿舍304", "房型": "夫妻房", "床位数": 2,
			"电费标准(元/度)": 1.0, "水费单价(元/m³)": 5.0,
		},
	}).Error)
	for i, p := range []struct {
		id, name string
		ratio    float64
	}{{"s1", "董友兵", 1}, {"s2", "吴英", 0}} {
		require.NoError(t, db.Create(&HRDormRecord{
			ID: p.id, TenantID: 1, Module: hrDormModuleStay, Name: p.name, Status: "在住",
			CustomData: map[string]any{
				"员工姓名": p.name, "住宿状态": "在住", "宿舍编号/房屋地址": "宿舍304",
				"入住日期": "2026-02-10", "分摊比例": p.ratio,
			},
		}).Error)
		_ = i
	}
	room304 := HRDormRecord{}
	require.NoError(t, db.Where("id = ?", "r1").First(&room304).Error)
	room304.CustomData["宿舍电表号"] = "E304"
	require.NoError(t, db.Model(&room304).Select("custom_data").Updates(&room304).Error)
	seedMeter73(t, db, "electricity", "E304", "宿舍304", 2000, 2100, 100, 100)

	rows, err := h.hrBuildDormCharges(ctx, 1, "2026-10")
	require.NoError(t, err)
	byName := map[string]HRDormCharge{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	require.InDelta(t, 1.0, byName["董友兵"].Ratio, 0.001)
	require.InDelta(t, 0.0, byName["吴英"].Ratio, 0.001, "配偶显式 0 必须保留")
	require.InDelta(t, 100.0, byName["董友兵"].Elec, 0.01)
	require.InDelta(t, 0.0, byName["吴英"].Elec, 0.01)
}

// 导入三模式：merge 覆盖同身份行、insert 跳过、update 不新增。
func TestRound62ImportModes(t *testing.T) {
	db := hrRound59DB(t)
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}))
	h := &HRHandler{db: db}
	r := hrRound60Router(t, h)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "d1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
		CustomData: map[string]any{"宿舍编号": "宿舍301", "房型": "四人间"},
	}).Error)

	upload := func(mode string, rows [][]string) *httptest.ResponseRecorder {
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		fw, err := w.CreateFormFile("file", "dorm.xlsx")
		require.NoError(t, err)
		f := excelize.NewFile()
		sh := f.GetSheetName(0)
		for i, row := range rows {
			for j, v := range row {
				cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
				require.NoError(t, f.SetCellValue(sh, cell, v))
			}
		}
		require.NoError(t, f.Write(fw))
		require.NoError(t, w.WriteField("mode", mode))
		require.NoError(t, w.Close())

		req := httptest.NewRequest(http.MethodPost,
			"/hr/dorm/records/import?module=dorm&mode="+mode, buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	// 仅新增：已存在的宿舍301 跳过，只加 宿舍302
	rec := upload("insert", [][]string{
		{"宿舍编号", "房型"},
		{"宿舍301", "单人间"},
		{"宿舍302", "双人间"},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data struct {
			Created int `json:"created"`
			Updated int `json:"updated"`
			Skipped int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 1, body.Data.Created)
	require.Equal(t, 0, body.Data.Updated)
	require.Equal(t, 1, body.Data.Skipped)

	var got HRDormRecord
	require.NoError(t, db.Where("tenant_id = ? AND code = ?", 1, "宿舍301").First(&got).Error)
	require.Equal(t, "四人间", hrDormStr(got.CustomData, "房型"), "insert 模式不得覆盖已存在行")

	// 仅更新：只改 宿舍301，不新增 宿舍999
	rec = upload("update", [][]string{
		{"宿舍编号", "房型"},
		{"宿舍301", "双人间"},
		{"宿舍999", "单人间"},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, db.Where("tenant_id = ? AND code = ?", 1, "宿舍301").First(&got).Error)
	require.Equal(t, "双人间", hrDormStr(got.CustomData, "房型"))
	var n int64
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND code = ?", 1, "宿舍999").Count(&n)
	require.Equal(t, int64(0), n)

	// 合并更新：命中覆盖 + 未命中新增
	rec = upload("merge", [][]string{
		{"宿舍编号", "房型"},
		{"宿舍301", "单人间"},
		{"宿舍303", "四人间"},
	})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, db.Where("tenant_id = ? AND code = ?", 1, "宿舍301").First(&got).Error)
	require.Equal(t, "单人间", hrDormStr(got.CustomData, "房型"))
	db.Model(&HRDormRecord{}).Where("tenant_id = ? AND code = ?", 1, "宿舍303").Count(&n)
	require.Equal(t, int64(1), n)
}

// 子表删除：stays 路由只能删住宿记录，不能删宿舍档案。
func TestRound62TypedDelete(t *testing.T) {
	db := hrRound59DB(t)
	require.NoError(t, db.AutoMigrate(&HRDormRecord{}))
	h := &HRHandler{db: db}
	r := hrRound60Router(t, h)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "room1", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍301", Name: "宿舍301",
	}).Error)
	require.NoError(t, db.Create(&HRDormRecord{
		ID: "stay1", TenantID: 1, Module: hrDormModuleStay, Name: "吴勇建",
	}).Error)

	// 用 stays 路由删宿舍档案 → 拒绝
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/hr/dorm/stays/room1", nil))
	require.NotEqual(t, http.StatusOK, rec.Code)
	var n int64
	db.Model(&HRDormRecord{}).Where("id = ?", "room1").Count(&n)
	require.Equal(t, int64(1), n, "跨子表删除必须被拦截")

	// 正常删除住宿记录
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/hr/dorm/stays/stay1", nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	db.Model(&HRDormRecord{}).Where("id = ?", "stay1").Count(&n)
	require.Equal(t, int64(0), n)
}

// 签约公司归一：全部合同类记录必须是「重庆星达铜业有限公司」。
func TestRound62ContractCompany(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.AutoMigrate(&HREmployee{}))
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, Name: "张三", EmployeeID: "C1", Status: hrRosterStatusActive,
		CustomData: map[string]any{"签约公司": "", "合同编号": "HT-001"},
	}).Error)
	require.NoError(t, h.hrFixContractCompany62(ctx))
	var emp HREmployee
	require.NoError(t, db.Where("id = ?", "e1").First(&emp).Error)
	require.Equal(t, hrContractCompanyName, hrDormStr(emp.CustomData, "签约公司"))
}

// seedMeter73 造一条能耗真实抄表（电/水/气），供费用核算测试引用。
func seedMeter73(t *testing.T, db *gorm.DB, category, meterNo, alias string,
	start, end, usage, amount float64) {
	t.Helper()
	id := "m-" + meterNo
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: id, TenantID: 1, Category: category, MeterNo: meterNo, Alias: alias, Enabled: true,
	}).Error)
	recID := "rec-" + category
	var n int64
	_ = db.Model(&types.UtilityMeterRecord{}).Where("id = ?", recID).Count(&n).Error
	if n == 0 {
		require.NoError(t, db.Create(&types.UtilityMeterRecord{
			ID: recID, TenantID: 1, Category: category, Month: "2026-10",
		}).Error)
	}
	require.NoError(t, db.Create(&types.UtilityMeterItem{
		ID: "it-" + meterNo, RecordID: recID, MeterID: id,
		StartReading: start, EndReading: end, Usage: usage, Amount: amount,
		UnitPrice: 1, Rate: 1,
	}).Error)
}
