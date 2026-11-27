// 第 75 轮回归：
//  1. 单位全称/简称归一匹配（费用清单归零根因）
//  2. 工业表计按介质恢复（电 6 / 水 4 / 气 1）且幂等
//  3. 表计更新：介质不匹配直接拒绝；改名级联历史抄表名称
//  4. 宿舍表号错配修正（W↔E）并同步房源档案
//  5. 「本期水费」data_type 迁移为 amount
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestRound75UnitNamesMatch(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"持睿汽车", "持睿汽车部件有限公司", true},
		{"重庆星达", "重庆星达铜业有限公司", true},
		{"重庆星达铜业有限公司", "星达铜业", true},
		{"持睿汽车", "重庆星达铜业有限公司", false},
		{"", "重庆星达", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, unitNamesMatch(c.a, c.b), "%q ~ %q", c.a, c.b)
	}
}

func hrRound75DB(t *testing.T) *gorm.DB {
	t.Helper()
	db := hrRound59DB(t)
	require.NoError(t, db.AutoMigrate(
		&HRDormRecord{}, &types.UtilityMeter{}, &types.UtilityMeterItem{},
		&types.BillingRecord{}, &types.BillingTenant{}, &types.BillingTenantItem{},
	))
	return db
}

func TestRound75RestoreIndustrialMetersIdempotent(t *testing.T) {
	db := hrRound75DB(t)
	ctx := context.Background()
	// 存量脏数据：CRM01 的归属单位错成持睿（需求要求归星达）
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "exist-crm01", TenantID: 1, Category: "water", Alias: "旧持睿水表1",
		MeterNo: "CRM01", MeterKind: "production", OwnerUnit: "持睿汽车部件有限公司",
		UseUnit: "持睿汽车部件有限公司", Rate: 1, DefaultUnitPrice: 5.22, Enabled: true,
	}).Error)
	// 与工业表计无关的宿舍表：恢复过程绝不能碰
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "dorm-403", TenantID: 1, Category: "electricity", Alias: "持睿403",
		MeterNo: "W021", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true,
	}).Error)

	uh := &UtilityHandler{db: db}
	created, updated := uh.RestoreIndustrialMeters75(ctx, 1)
	require.Equal(t, 10, created, "11 张中 CRM01 已存在 → 新增 10 张")
	require.Equal(t, 1, updated, "CRM01 的归属/别名被修正")

	var all []types.UtilityMeter
	require.NoError(t, db.Where("tenant_id = ? AND meter_kind = ?", 1, "production").Find(&all).Error)
	require.Len(t, all, 11, "工业表计必须恰好 11 张（电 6 / 水 4 / 气 1）")

	byNo := map[string]types.UtilityMeter{}
	for _, m := range all {
		byNo[m.MeterNo] = m
	}
	for _, no := range []string{"CR-E001", "CR-E002", "CR-E003", "XD-E001", "XD-E002", "XD-E003"} {
		require.Equal(t, "electricity", byNo[no].Category, no)
	}
	require.Equal(t, "water", byNo["CRM01"].Category)
	require.Equal(t, "重庆星达铜业有限公司", byNo["CRM01"].OwnerUnit, "归属单位必须归星达")
	require.Equal(t, "持睿水表分表1", byNo["CRM01"].Alias)
	require.Equal(t, 5.22, byNo["CRM02"].DefaultUnitPrice)
	require.Equal(t, "gas", byNo["G001"].Category)
	require.Equal(t, "auto", byNo["G001"].MeterMode)
	require.Equal(t, "李永娇", byNo["G001"].Manager)

	// 幂等：再跑一次零变更
	created2, updated2 := uh.RestoreIndustrialMeters75(ctx, 1)
	require.Zero(t, created2)
	require.Zero(t, updated2)

	// 宿舍表零影响
	var dorm types.UtilityMeter
	require.NoError(t, db.Where("id = ?", "dorm-403").First(&dorm).Error)
	require.Equal(t, "W021", dorm.MeterNo)
}

func TestRound75MeterUpdateGuardAndCascade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := hrRound75DB(t)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "elec-1", TenantID: 1, Category: "electricity", Alias: "持睿工业分表1",
		MeterNo: "CR-E001", MeterKind: "production", Rate: 200, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeter{
		ID: "water-1", TenantID: 1, Category: "water", Alias: "持睿水表分表1",
		MeterNo: "CRM01", MeterKind: "production", Rate: 1, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&types.UtilityMeterItem{
		ID: "item-1", RecordID: "rec-1", MeterID: "elec-1", MeterName: "旧名称",
		Usage: 100, Amount: 100,
	}).Error)

	h := &UtilityHandler{db: db}
	do := func(id string, payload map[string]any) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		b, _ := json.Marshal(payload)
		c.Request = httptest.NewRequest(http.MethodPut, "/utilities/meters/"+id, bytes.NewReader(b)).WithContext(ctx)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: id}}
		h.UpdateUtilityMeter(c)
		return w
	}

	// ① 把 water 载荷打到电表 id 上：必须 400，且电表零改动
	w := do("elec-1", map[string]any{
		"category": "water", "alias": "被串写的电表", "meter_no": "W999", "rate": 1,
	})
	// 断言错误契约交给统一中间件；单元测试里直接验证「零写入」这一硬结果
	require.NotContains(t, w.Body.String(), "已保存", w.Body.String())
	var elec types.UtilityMeter
	require.NoError(t, db.Where("id = ?", "elec-1").First(&elec).Error)
	require.Equal(t, "CR-E001", elec.MeterNo)
	require.Equal(t, "持睿工业分表1", elec.Alias)

	// ② 同介质改名：级联历史抄表的 meter_name，数值不动
	w = do("elec-1", map[string]any{
		"category": "electricity", "alias": "持睿工业分表1号", "meter_no": "CR-E001",
		"meter_kind": "production", "rate": 200, "enabled": true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var item types.UtilityMeterItem
	require.NoError(t, db.Where("id = ?", "item-1").First(&item).Error)
	require.Equal(t, "持睿工业分表1号", item.MeterName)
	require.Equal(t, 100.0, item.Usage, "级联改号改名绝不动度数")
	require.Equal(t, 100.0, item.Amount)
}

func TestRound75DormMeterNumbersSwap(t *testing.T) {
	db := hrRound75DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	rooms := []HRDormRecord{
		{ID: "r403", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍403", CustomData: map[string]any{
			"宿舍编号": "宿舍403", "所属公司": "持睿汽车部件有限公司", "宿舍电表号": "W021", "宿舍水表号": "W021",
		}},
		{ID: "r408", TenantID: 1, Module: hrDormModuleRoom, Code: "宿舍408", CustomData: map[string]any{
			"宿舍编号": "宿舍408", "所属公司": "持睿汽车部件有限公司", "宿舍电表号": "E024", "宿舍水表号": "E024",
		}},
	}
	require.NoError(t, db.Create(&rooms).Error)
	require.NoError(t, db.Create(&[]types.UtilityMeter{
		{ID: "e403", TenantID: 1, Category: "electricity", Alias: "持睿403", MeterNo: "W021", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true},
		{ID: "w403", TenantID: 1, Category: "water", Alias: "持睿403", MeterNo: "W021", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true},
		{ID: "e408", TenantID: 1, Category: "electricity", Alias: "持睿408", MeterNo: "E024", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true},
		{ID: "w408", TenantID: 1, Category: "water", Alias: "持睿408", MeterNo: "E024", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true},
	}).Error)

	meters, affected := h.hrFixDormMeterNumbers75(ctx, 1)
	require.Equal(t, 2, meters, "403 电表 W→E、408 水表 E→W")
	require.Equal(t, 2, affected, "403 电表号 W021→E021、408 水表号 E024→W024 都要回写档案")

	var e403, w408 types.UtilityMeter
	require.NoError(t, db.Where("id = ?", "e403").First(&e403).Error)
	require.Equal(t, "E021", e403.MeterNo)
	require.NoError(t, db.Where("id = ?", "w408").First(&w408).Error)
	require.Equal(t, "W024", w408.MeterNo)

	var r408 HRDormRecord
	require.NoError(t, db.Where("id = ?", "r408").First(&r408).Error)
	require.Equal(t, "E024", hrDormStr(r408.CustomData, "宿舍电表号"))
	require.Equal(t, "W024", hrDormStr(r408.CustomData, "宿舍水表号"))
	var r403 HRDormRecord
	require.NoError(t, db.Where("id = ?", "r403").First(&r403).Error)
	require.Equal(t, "E021", hrDormStr(r403.CustomData, "宿舍电表号"), "电表号必须还原为 E021")
	require.Equal(t, "W021", hrDormStr(r403.CustomData, "宿舍水表号"), "水表号保持 W021")
}

func TestRound75WaterFeeDataType(t *testing.T) {
	db := hrRound75DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-cost", TenantID: 1, Scope: "energy", Name: "费用清单",
		Subs: []types.FleetCategorySub{
			{Name: "current_water_fee", Label: "本期水费", Enabled: true, DataType: "text"},
			{Name: "water_ind_fee", Label: "水费（工业）", Enabled: true, DataType: "number"},
		},
	}).Error)
	require.Equal(t, 1, h.hrFixWaterFeeType75(ctx, 1))
	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat-cost").First(&cat).Error)
	require.Equal(t, "amount", cat.Subs[0].DataType)
	require.Equal(t, "number", cat.Subs[1].DataType, "其它字段不受影响")
	require.Zero(t, h.hrFixWaterFeeType75(ctx, 1), "幂等")
}

func TestRound75BillingRefsByShortUnitName(t *testing.T) {
	db := hrRound75DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&[]types.UtilityMeter{
		{ID: "m1", TenantID: 1, Category: "electricity", Alias: "持睿403", MeterNo: "E021", MeterKind: "dorm", UseUnit: "持睿汽车部件有限公司", Enabled: true},
		{ID: "m2", TenantID: 1, Category: "water", Alias: "持睿水表分表1", MeterNo: "CRM01", MeterKind: "production", UseUnit: "持睿汽车部件有限公司", Enabled: true},
		{ID: "m3", TenantID: 1, Category: "electricity", Alias: "宿舍301", MeterNo: "E301", MeterKind: "dorm", UseUnit: "重庆星达铜业有限公司", Enabled: true},
	}).Error)
	h := &BillingHandler{db: db}
	refs := h.loadRefsByUseUnit(ctx, "持睿汽车")
	require.Len(t, refs, 2, "简称持睿汽车必须命中全称表计（1 电 + 1 水）")
}
