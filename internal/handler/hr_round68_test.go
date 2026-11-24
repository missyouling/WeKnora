// 第 68 轮回归：社保按月隔离 / 账单不可变快照 / 原始记录变更提醒 / 离职字段全集。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/types"
)

// 导入 2026-10 工伤：2026-11 的养老 / 工伤记录必须 100% 零损失。
func TestRound68SocialImportMonthIsolation(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}

	seed := []HRSIRecord{
		{ID: "n11-p", TenantID: 1, BillMonth: "2026-11", Period: "2026-11", RecordType: "normal",
			Name: "张三", IDNumber: "500101199001010011", Scheme: "养老保险",
			UnitAmount: 962.4, PersonalAmount: 481.2, TotalAmount: 1443.6, Source: "import"},
		{ID: "n11-i", TenantID: 1, BillMonth: "2026-11", Period: "2026-11", RecordType: "normal",
			Name: "张三", IDNumber: "500101199001010011", Scheme: "工伤保险",
			UnitAmount: 99.25, TotalAmount: 99.25, Source: "import"},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}

	items := []HRSIRecord{{
		ID: "n10-i", TenantID: 1, BillMonth: "2026-10", Period: "2026-10", RecordType: "normal",
		Name: "张三", IDNumber: "500101199001010011", Scheme: "工伤保险",
		UnitAmount: 88.8, TotalAmount: 88.8, Source: "import",
	}}
	slices := []hrSISlice66{{Month: "2026-10", Kind: "normal", Scheme: "工伤保险", Part: HRPartUnit}}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := h.hrSIApplyImport66(ctx, tx, 1, slices, items,
			map[string]map[string]bool{"500101199001010011|工伤保险": {HRPartUnit: true}})
		return err
	}))

	var rows []HRSIRecord
	require.NoError(t, db.Where("tenant_id = ? AND bill_month = ?", 1, "2026-11").Find(&rows).Error)
	require.Len(t, rows, 2, "导入 2026-10 绝不能影响 2026-11")
	sum := 0.0
	for _, r := range rows {
		sum += r.UnitAmount + r.PersonalAmount
	}
	require.InDelta(t, 1443.6+99.25, sum, 0.001)

	var oct []HRSIRecord
	require.NoError(t, db.Where("tenant_id = ? AND bill_month = ?", 1, "2026-10").Find(&oct).Error)
	require.Len(t, oct, 1)
}

// 账单生成后：删记录 → 快照金额纹丝不动 + source_dirty；重新生成 → 更新并清标记。
func TestRound68BillSnapshotImmutable(t *testing.T) {
	db := hrRound59DB(t)
	router := hrTestRouter(t, db)

	seed := []HRSIRecord{
		{ID: "s9-p", TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
			Name: "张三", IDNumber: "500101199001010011", Scheme: "养老保险",
			Base: 5000, UnitAmount: 800, PersonalAmount: 400, TotalAmount: 1200, Source: "import"},
		{ID: "s9-i", TenantID: 1, BillMonth: "2026-09", Period: "2026-09", RecordType: "normal",
			Name: "李四", IDNumber: "500101199002020022", Scheme: "工伤保险",
			Base: 4000, UnitAmount: 60, TotalAmount: 60, Source: "import"},
	}
	for i := range seed {
		require.NoError(t, db.Create(&seed[i]).Error)
	}

	// 生成 2026-09 账单（写快照）
	body := bytes.NewBufferString(`{"month_label":"2026-09","overwrite":true}`)
	req := httptest.NewRequest(http.MethodPost, "/hr/social/bills", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	created := hrDecode(t, w)["data"].(map[string]any)
	billID := created["id"].(string)
	require.Equal(t, false, created["source_dirty"])

	var bill HRSIBill
	require.NoError(t, db.Where("id = ?", billID).First(&bill).Error)
	require.NotEmpty(t, hrSIBillSnapshotItems68(bill.DetailSnapshot), "必须固化明细快照")
	origGrand := bill.GrandTotal
	origItems := len(hrSIBillSnapshotItems68(bill.DetailSnapshot))
	require.Equal(t, 1260.0, origGrand)
	require.Equal(t, 2, origItems)

	// 删一条记录 → 只打脏标记，不重建账单
	delReq := httptest.NewRequest(http.MethodDelete, "/hr/social/records/s9-p", nil)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusOK, delW.Code, "body=%s", delW.Body.String())
	delData := hrDecode(t, delW)["data"].(map[string]any)
	require.Equal(t, true, delData["bill_dirty_warning"])

	// 详情：金额与明细条数保持快照原值，账单标记 source_dirty
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, httptest.NewRequest(http.MethodGet, "/hr/social/bills/"+billID, nil))
	require.Equal(t, http.StatusOK, getW.Code)
	detail := hrDecode(t, getW)["data"].(map[string]any)
	billView := detail["bill"].(map[string]any)
	require.Equal(t, true, billView["source_dirty"])
	require.Equal(t, origGrand, billView["应缴总计"])
	require.Len(t, detail["items"].([]any), origItems, "快照明细条数不得随记录删除而减少")
	require.Equal(t, 400.0, detail["items"].([]any)[0].(map[string]any)["个人小计"])

	// 重新生成（覆盖）→ 按最新记录更新且清标记
	body2 := bytes.NewBufferString(`{"month_label":"2026-09","overwrite":true}`)
	req2 := httptest.NewRequest(http.MethodPost, "/hr/social/bills", body2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, "body=%s", w2.Body.String())
	regen := hrDecode(t, w2)["data"].(map[string]any)
	require.Equal(t, false, regen["source_dirty"])
	require.Equal(t, 60.0, regen["应缴总计"], "重新生成必须按最新记录算账")
	var bill2 HRSIBill
	require.NoError(t, db.Where("tenant_id = ? AND month_label = ?", 1, "2026-09").First(&bill2).Error)
	require.Len(t, hrSIBillSnapshotItems68(bill2.DetailSnapshot), 1)
	require.False(t, bill2.SourceDirty)
}

// 快照回填：旧账单（无 detail_snapshot）启动后补快照，明细来自 hr_si_bill_items。
func TestRound68BackfillBillSnapshot(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&HRSIBill{
		ID: "old1", TenantID: 1, MonthLabel: "2026-07", Headcount: 1, GrandTotal: 100,
		CreatedAt: time.Now().Add(-time.Hour),
	}).Error)
	require.NoError(t, db.Create(&HRSIBillItem{
		ID: "oi1", TenantID: 1, BillID: "old1", Sequence: "1", Name: "张三",
		IDNumber: "500101199001010011", PTotal: 40, UTotal: 60,
	}).Error)
	require.NoError(t, h.hrBackfillSIBillSnapshots68(ctx))
	var bill HRSIBill
	require.NoError(t, db.Where("id = ?", "old1").First(&bill).Error)
	items := hrSIBillSnapshotItems68(bill.DetailSnapshot)
	require.Len(t, items, 1)
	require.Equal(t, 60.0, hrNum68(items[0]["单位小计"]))
	require.NotNil(t, bill.GeneratedAt)
}

// 离职胶囊镜像：补齐在职字段（默认关闭），保留离职字段既有状态，幂等。
func TestRound68ResignMirrorFullFields(t *testing.T) {
	subs := []types.FleetCategorySub{
		{Name: "工号", Enabled: true, IsDefault: true, DataType: "text", Group: hrRosterGroupActive},
		{Name: "学历", Enabled: true, IsDefault: true, DataType: "text", Group: hrRosterGroupActive},
		{Name: "工作地点", Enabled: true, IsDefault: false, DataType: "text", Group: hrRosterGroupActive},
		{Name: "离职日期", Enabled: true, IsDefault: true, DataType: "date", Group: hrRosterGroupResigned},
	}
	out, changed := hrResignMirrorSubs68(subs)
	require.True(t, changed)
	byName := map[string]types.FleetCategorySub{}
	for _, s := range out {
		if s.Group == hrRosterGroupResigned {
			byName[s.Name] = s
		}
	}
	require.Len(t, byName, 4, "离职胶囊 = 3 个在职字段 + 1 个离职字段")
	for _, name := range []string{"工号", "学历", "工作地点"} {
		s, ok := byName[name]
		require.True(t, ok, name+" 必须并入离职胶囊")
		require.False(t, s.Enabled, name+" 新并入字段默认不启用")
		require.False(t, s.IsDefault, name+" 新并入字段默认不进表头")
	}
	require.True(t, byName["离职日期"].Enabled, "原有离职字段状态不得改变")
	require.True(t, byName["离职日期"].IsDefault)

	again, changed2 := hrResignMirrorSubs68(out)
	require.False(t, changed2)
	require.Len(t, again, len(out), "幂等：重复执行零变更")
}

// 新租户种子：离职胶囊 = 57（48 在职 - 6 重叠 + 15 离职专有）。
func TestRound68RosterSeedResignFull(t *testing.T) {
	subs := hrRosterSeedSubs57()
	active, resigned := 0, 0
	for _, s := range subs {
		switch s.Group {
		case hrRosterGroupActive:
			active++
		case hrRosterGroupResigned:
			resigned++
			// 离职专有 15 个之外的并入字段必须默认关闭
			if s.Name != "离职日期" && s.Name != "离职证明" && s.Name != "离职类型" &&
				s.Name != "离职原因" && s.Name != "离职去向" && s.Name != "离职申请日期" &&
				s.Name != "离职原因说明" && s.Name != "薪资结算日" && s.Name != "是否试用期内离职" &&
				s.Name != "工号" && s.Name != "姓名" && s.Name != "部门" && s.Name != "岗位" &&
				s.Name != "签约公司" && s.Name != "入职日期" {
				require.False(t, s.Enabled, s.Name+" 属于新并入字段，默认关闭")
			}
		}
	}
	require.Equal(t, 48, active)
	require.Equal(t, 57, resigned, "离职胶囊必须包含在职全集（42）+ 离职专有（15）")
}

// 月份隔离的 JSON 覆盖：请求体里的 month 参数不会把 2026-10 文件写到 2026-11。
func TestRound68ImportUsesFileMonth(t *testing.T) {
	// hrResolveImportMonth 的口径：显式月份 > 文件名 > 当前月
	require.Equal(t, "2026-10", hrResolveImportMonth("", "2026-10", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "2026-09", hrResolveImportMonth("2026-09", "2026-10", time.Now()))
	require.Equal(t, "2026-11", hrResolveImportMonth("", "", time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)))
}

// 防止 json 包被误删（快照结构断言依赖编码稳定性）。
func TestRound68SnapshotJSONShape(t *testing.T) {
	bill := HRSIBill{ID: "b", MonthLabel: "2026-09", Headcount: 1, GrandTotal: 100}
	snap := hrSIBuildSnapshot68("2026-09", bill, []HRSIBillItem{{
		ID: "i1", Sequence: "1", Name: "张三", PPension: 40, UPension: 60, PTotal: 40, UTotal: 60,
	}})
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"items"`)
	require.Contains(t, string(raw), `"dept_summary"`)
}

// 首次迁移：已存在的非离职专有字段（引用字段）必须被强制关闭。
func TestRound68MigrateResetsNonCoreFlags(t *testing.T) {
	db := hrRound59DB(t)
	ctx := context.Background()
	h := &HRHandler{db: db}
	require.NoError(t, db.Create(&types.FleetCategory{
		ID: "cat68", TenantID: 1, Scope: hrScopeRoster, Name: "花名册", Enabled: true,
		Subs: []types.FleetCategorySub{
			{Name: "工号", Enabled: true, IsDefault: true, Group: hrRosterGroupActive},
			{Name: "学历", Enabled: true, IsDefault: true, Group: hrRosterGroupActive},
			{Name: "离职日期", Enabled: true, IsDefault: true, Group: hrRosterGroupResigned},
			{Name: "个人社保账号", Enabled: true, IsDefault: true, Group: hrRosterGroupResigned,
				Source: &types.FleetCategorySubSource{Category: "社保管理", Field: "个人社保账号"}},
		},
	}).Error)
	require.NoError(t, h.hrMigrateResignFull68(ctx))

	var cat types.FleetCategory
	require.NoError(t, db.Where("id = ?", "cat68").First(&cat).Error)
	flags := map[string][2]bool{}
	for _, s := range cat.Subs {
		if s.Group != hrRosterGroupResigned {
			continue
		}
		flags[strings.TrimSpace(s.Name)] = [2]bool{s.Enabled, s.IsDefault}
	}
	require.Equal(t, [2]bool{false, false}, flags["个人社保账号"], "非离职专有字段首次迁移必须关闭")
	require.Equal(t, [2]bool{false, false}, flags["学历"], "新并入字段默认关闭")
	require.Equal(t, [2]bool{true, true}, flags["离职日期"], "离职专有字段状态不变")

	// 第二次：用户手动打开后不得被回滚
	for i := range cat.Subs {
		if cat.Subs[i].Name == "学历" && cat.Subs[i].Group == hrRosterGroupResigned {
			cat.Subs[i].Enabled = true
			cat.Subs[i].IsDefault = true
		}
	}
	require.NoError(t, db.Save(&cat).Error)
	require.NoError(t, h.hrMigrateResignFull68(ctx))
	require.NoError(t, db.Where("id = ?", "cat68").First(&cat).Error)
	for _, s := range cat.Subs {
		if s.Name == "学历" && s.Group == hrRosterGroupResigned {
			require.True(t, s.Enabled, "第二次迁移不得回滚用户开关")
		}
	}
}
