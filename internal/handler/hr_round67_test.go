// 第 67 轮回归：自定义字段六同步（模板 / 导入 / 抽屉保存）。
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 模板字段清单：顺序严格跟 subs，自定义字段必进，跨模块引用 / 停用 / system 必排除。
func TestRound67TemplateFieldsFollowSubs(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	cat := types.FleetCategory{
		ID: "cat-roster", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			// 用户新增的自定义字段（第 67 轮核心）
			{Name: "工作地点", Label: "工作地点", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "个人社保账号", Enabled: true, DataType: "text", Group: hrRosterGroupActive,
				Source: &types.FleetCategorySubSource{Category: "社保管理", Field: "个人社保账号"}},
			{Name: "停用字段", Enabled: false, DataType: "text", Group: hrRosterGroupActive},
			{Name: "合同状态", Enabled: true, DataType: "system", Group: hrRosterGroupActive},
		},
	}
	require.NoError(t, db.Create(&cat).Error)

	fields, ok := hrTemplateFieldsFor(ctx, db, 1, hrScopeRoster, hrRosterGroupActive)
	require.True(t, ok)
	headers := make([]string, 0, len(fields))
	for _, f := range fields {
		headers = append(headers, f.Header)
	}
	require.Equal(t, []string{"工号", "工作地点"}, headers,
		"模板表头必须按 subs 顺序且只保留可录入的原生字段")
}

// 社保记录保存：自定义字段落到 custom_data，身份维度不可改写。
func TestRound67SIRecordPersistsCustomFields(t *testing.T) {
	db := hrRound59DB(t)
	r := hrTestRouter(t, db)
	require.NoError(t, db.Create(&HRSIRecord{
		ID: "si67", TenantID: 1, BillMonth: "2026-11", Period: "2026-11", RecordType: "normal",
		Name: "张三", IDNumber: "500101199001010011", Scheme: "养老保险",
		Base: 5000, UnitAmount: 800, PersonalAmount: 400, TotalAmount: 1200,
		CustomData: map[string]any{"工作地点": "重庆璧山厂区"},
	}).Error)

	body := bytes.NewBufferString(`{"缴费基数":"6000","工作地点":"重庆璧山厂区A","姓名":"李四"}`)
	req := httptest.NewRequest(http.MethodPut, "/hr/social/records/si67", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var got HRSIRecord
	require.NoError(t, db.Where("id = ?", "si67").First(&got).Error)
	require.Equal(t, 6000.0, got.Base)
	require.Equal(t, "张三", got.Name, "身份维度（姓名）禁止被改写")
	require.Equal(t, "重庆璧山厂区A", got.CustomData["工作地点"])
	require.Equal(t, 6000.0, got.CustomData["缴费基数"])
}

// 导入：模板里的自定义字段列（工作地点）必须写进该员工的 custom_data。
func TestRound67ImportWritesCustomField(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat-roster67", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "姓名", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "证件号码", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
			{Name: "工作地点", Label: "工作地点", Enabled: true, DataType: "text", Group: hrRosterGroupActive},
		},
	}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "emp67", TenantID: 1, EmployeeID: "C0067", Name: "廖宏",
		IDNumber: "500101199001010011", Status: "在职", CustomData: map[string]any{},
	}).Error)

	h := &HRHandler{db: db}
	rows := [][]string{
		{"工号", "姓名", "证件号码", "工作地点"},
		{"C0067", "廖宏", "500101199001010011", "重庆璧山厂区"},
	}
	stats, err := h.applyHRModuleRows(ctx, 1, hrModuleRoster, rows)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Updated, "一行必须命中并保存")

	var got HREmployee
	require.NoError(t, db.Where("id = ?", "emp67").First(&got).Error)
	require.Equal(t, "重庆璧山厂区", got.CustomData["工作地点"])
}
