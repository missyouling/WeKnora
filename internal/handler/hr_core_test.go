// 人事三模块后端 TDD 用例（设计文档第 5 节 9 组），sqlite 内存库骨架照抄
// fleet_category_cascade_test.go。测试名统一 TestHR* 前缀，便于 -run TestHR 白名单跑。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
)

func hrTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.FleetCategory{},
		&HREmployee{}, &HRSIPeriod{}, &HRSIRawRecord{}, &HRSICharge{}, &HRSIChange{},
		&HRPFSettings{}, &HRPFRecord{}, &HRPFBill{}, &HRPFBillItem{},
	))
	return db
}

func hrTestRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(1))
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewHRHandler(db)
	// registerHRRoutes 内部已带 /hr 前缀（为兼容 router 包的 apiKeyRouteGroup），
	// 测试传根组即可；传 r.Group("/hr") 会变成 /hr/hr/... 导致 404。
	h.registerHRRoutes(r.Group(""))
	return r
}

func hrDecode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), "body=%s", w.Body.String())
	return m
}

// 1. 中文表头别名映射（工号/员工编号/编号 → employee_id；身份证号码/证件号 → id_number）。
func TestHRHeaderAliases(t *testing.T) {
	for _, raw := range []string{"工号", "员工编号", "编号"} {
		assert.Equal(t, "employee_id", employeeHeaderAliases[normalizeEmployeeHeader(raw)], raw)
	}
	for _, raw := range []string{"身份证号码", "身份证号", "证件号码", "证件号"} {
		assert.Equal(t, "id_number", employeeHeaderAliases[normalizeEmployeeHeader(raw)], raw)
	}
}

// 2. 18 位身份证 → 性别 / 年龄 / 出生月份；入职日期 → 工龄。
func TestHRIDCardDerivation(t *testing.T) {
	id := "500106199003072316"
	require.Len(t, id, 18)
	assert.Equal(t, "男", hrIDCardGender(id))
	// 第 47 轮：出生字段升级为完整日期（原 YYYY-MM 只够算年龄，不够展示）
	assert.Equal(t, "1990-03-07", hrIDCardBirthDate(id))
	assert.Equal(t, 36, hrIDCardAgeAt(id, time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "3年6个月", hrWorkYearsAt("2020-01-01", time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)))

	e := &HREmployee{IDNumber: id, HireDate: "2020-01-01"}
	hrEnrichEmployee(e)
	assert.Equal(t, "男", e.Gender)
	assert.Equal(t, "1990-03-07", e.BirthMonth)
	// 年龄保留 1 位小数（任务 §2.3）
	assert.Regexp(t, `^\d+\.\d$`, e.Age)
	assert.Contains(t, e.WorkYears, "年")
}

// 3. 花名册 merge/insert/update 三模式 + 离职冲突 + force_transition（纯函数）。
func TestHRRosterImportModes(t *testing.T) {
	rows := [][]string{
		{"工号", "姓名", "身份证号码", "部门", "状态"},
		{"E001", "张三", "500106199003072316", "生产部", "在职"},
		{"E002", "李四", "500106198512110024", "财务部", "在职"},
	}
	existing := []HREmployee{
		{ID: "e1", IDNumber: "500106199003072316", Name: "张三", Status: "active", Department: "生产部"},
	}

	plan, err := hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{Mode: "merge"})
	require.NoError(t, err)
	assert.Len(t, plan.Inserted, 1)
	assert.Len(t, plan.Updated, 1)
	assert.Empty(t, plan.Conflicts)

	plan, err = hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{Mode: "insert"})
	require.NoError(t, err)
	assert.Len(t, plan.Inserted, 1)
	assert.Len(t, plan.Updated, 0)
	assert.Equal(t, 1, plan.Skipped)

	plan, err = hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{Mode: "update"})
	require.NoError(t, err)
	assert.Len(t, plan.Inserted, 0)
	assert.Len(t, plan.Updated, 1)
	assert.Equal(t, 1, plan.Skipped)

	plan, err = hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{Mode: "merge", DefaultStatus: "resigned"})
	require.NoError(t, err)
	assert.Len(t, plan.Conflicts, 1)

	plan, err = hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{
		Mode: "merge", DefaultStatus: "resigned", ForceDefaultStatus: true,
	})
	require.NoError(t, err)
	assert.Empty(t, plan.Conflicts)
	assert.Len(t, plan.Updated, 1)
}

// 3b. 离职冲突走 HTTP 返回 409 {success:false, conflicts:[...]}。
func TestHRRosterImportConflictHTTP409(t *testing.T) {
	db := hrTestDB(t)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, IDNumber: "500106199003072316", Name: "张三", Status: "active",
	}).Error)
	r := hrTestRouter(t, db)

	f := excelize.NewFile()
	require.NoError(t, f.SetCellValue("Sheet1", "A1", "姓名"))
	require.NoError(t, f.SetCellValue("Sheet1", "B1", "身份证号码"))
	require.NoError(t, f.SetCellValue("Sheet1", "A2", "张三"))
	require.NoError(t, f.SetCellValue("Sheet1", "B2", "500106199003072316"))
	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "roster.xlsx")
	require.NoError(t, err)
	_, err = part.Write(buf.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.WriteField("mode", "merge"))
	require.NoError(t, mw.WriteField("default_status", "resigned"))
	require.NoError(t, mw.Close())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hr/roster/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	resp := hrDecode(t, w)
	assert.Equal(t, false, resp["success"])
	assert.NotEmpty(t, resp["conflicts"])
}

// 4. validateRequired 个人四险 / 单位五险缺一报错。
func TestHRValidateRequired(t *testing.T) {
	var full []HRSIRawRecord
	for _, s := range hrRequiredUploads[HRPartPersonal] {
		full = append(full, HRSIRawRecord{Part: HRPartPersonal, Scheme: s})
	}
	for _, s := range hrRequiredUploads[HRPartUnit] {
		full = append(full, HRSIRawRecord{Part: HRPartUnit, Scheme: s})
	}
	require.NoError(t, hrValidateRequired(full))

	assert.Error(t, hrValidateRequired(full[:len(full)-1])) // 缺单位工伤

	var missingPersonal []HRSIRawRecord
	for _, r := range full {
		if r.Part == HRPartPersonal && r.Scheme == HRSchemeSeriousIllness {
			continue
		}
		missingPersonal = append(missingPersonal, r)
	}
	assert.Error(t, hrValidateRequired(missingPersonal)) // 缺个人大病
}

// 5. buildAggregates 金额精确（单位医疗含大病、单位 base fallback、部门自动关联花名册）。
func TestHRBuildAggregates(t *testing.T) {
	empMap := map[string]HREmployee{
		"ID1": {IDNumber: "ID1", Name: "王五", Department: "行政部"},
	}
	mk := func(part, scheme string, amount, base float64) HRSIRawRecord {
		return HRSIRawRecord{IDNumber: "ID1", Name: "X", Part: part, Scheme: scheme, AmountDue: amount, PayBase: base}
	}
	raws := []HRSIRawRecord{
		mk(HRPartPersonal, HRSchemePension, 800, 5000),
		mk(HRPartPersonal, HRSchemeMedical, 100, 0),
		mk(HRPartPersonal, HRSchemeSeriousIllness, 20, 0),
		mk(HRPartPersonal, HRSchemeUnemployment, 50, 0),
		mk(HRPartUnit, HRSchemePension, 1600, 0),
		mk(HRPartUnit, HRSchemeMedical, 300, 0),
		mk(HRPartUnit, HRSchemeSeriousIllness, 30, 0),
		mk(HRPartUnit, HRSchemeInjury, 40, 0),
		mk(HRPartUnit, HRSchemeUnemployment, 70, 0),
	}
	charges := hrBuildAggregates(raws, empMap)
	require.Len(t, charges, 2)
	var pc, uc HRSICharge
	for _, ch := range charges {
		if ch.Part == HRPartPersonal {
			pc = ch
		} else {
			uc = ch
		}
	}
	assert.Equal(t, "王五", pc.Name)
	assert.Equal(t, "行政部", pc.Department)
	assert.Equal(t, 5000.0, pc.Base)
	assert.Equal(t, 970.0, pc.Subtotal)
	assert.Equal(t, 330.0, uc.MedicalMaternity) // 300 医疗 + 30 大病
	assert.Equal(t, 2040.0, uc.Subtotal)        // 1600 + 330 + 40 + 70
	assert.Equal(t, 5000.0, uc.Base)            // max(unitBase=0, personalBase=5000)
}

// 6. 补退累加（纯函数）。
func TestHRAdjustmentAccumulate(t *testing.T) {
	normal := []HRSICharge{
		{IDNumber: "ID1", Part: HRPartPersonal, Pension: 800, MedicalMaternity: 100, SeriousIllness: 20, Unemployment: 50, Subtotal: 970},
	}
	adj := []HRSICharge{
		{IDNumber: "ID1", Part: HRPartPersonal, Pension: 80, MedicalMaternity: 10, SeriousIllness: 2, Unemployment: 5, Subtotal: 97},
	}
	merged := hrMergeCharges(normal, adj)
	require.Len(t, merged, 1)
	assert.Equal(t, 880.0, merged[0].Pension)
	assert.Equal(t, 110.0, merged[0].MedicalMaternity)
	assert.Equal(t, 1067.0, merged[0].Subtotal)
}

// 6b. 补退累加（账期集成：正常 9 条 + 补退 2 条 → 聚合金额累加）。
func TestHRProcessAdjustmentsAccumulate(t *testing.T) {
	db := hrTestDB(t)
	h := NewHRHandler(db)
	ctx := context.Background()
	pid := "p-adj"
	require.NoError(t, db.Create(&HRSIPeriod{ID: pid, TenantID: 1, YearMonth: "2026-03"}).Error)
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: 1, IDNumber: "ID1", Name: "王五", Department: "行政部", Status: "active",
	}).Error)
	add := func(part, scheme, ft string, amount float64) {
		require.NoError(t, db.Create(&HRSIRawRecord{
			ID: uuid.New().String(), TenantID: 1, PeriodID: pid, IDNumber: "ID1", Name: "X",
			Part: part, Scheme: scheme, FileType: ft, AmountDue: amount, PayBase: 5000,
		}).Error)
	}
	for _, s := range hrRequiredUploads[HRPartPersonal] {
		add(HRPartPersonal, s, HRFileTypeNormal, 100)
	}
	for _, s := range hrRequiredUploads[HRPartUnit] {
		add(HRPartUnit, s, HRFileTypeNormal, 100)
	}
	add(HRPartPersonal, HRSchemePension, HRFileTypeAdjustment, 100)
	add(HRPartUnit, HRSchemeMedical, HRFileTypeAdjustment, 50)

	charges, hasAdj, err := h.hrComputeCharges(ctx, 1, pid)
	require.NoError(t, err)
	assert.True(t, hasAdj)
	require.Len(t, charges, 2)
	var pc, uc HRSICharge
	for _, ch := range charges {
		if ch.Part == HRPartPersonal {
			pc = ch
		} else {
			uc = ch
		}
	}
	assert.Equal(t, 200.0, pc.Pension)          // 100 正常 + 100 补退
	assert.Equal(t, 250.0, uc.MedicalMaternity) // (100+100) 正常医疗含大病 + 50 补退医疗
}

// 7. 公积金 total = personal + company（比例 6%）、自动派生。
func TestHRProvidentFund(t *testing.T) {
	r := HRPFRecord{PersonalBase: 5000, PersonalAmount: 300, CompanyAmount: 300}
	hrNormalizePFRecord(&r, hrDefaultPFRatio)
	assert.Equal(t, 6.0, r.ContributionRatio)
	assert.Equal(t, 600.0, r.TotalAmount)
	assert.Equal(t, "active", r.Status)

	r2 := HRPFRecord{PersonalBase: 5000}
	hrNormalizePFRecord(&r2, 0)
	assert.Equal(t, 6.0, r2.ContributionRatio)
	assert.Equal(t, 300.0, r2.PersonalAmount)
	assert.Equal(t, 300.0, r2.CompanyAmount)
	assert.Equal(t, 600.0, r2.TotalAmount)
}

// 7b. 封存/启封 + 账单只汇总 active。
func TestHRProvidentFundBillActiveOnly(t *testing.T) {
	db := hrTestDB(t)
	r := hrTestRouter(t, db)
	base := HRPFRecord{
		PersonalBase: 5000, ContributionRatio: 6, PersonalAmount: 300, CompanyAmount: 300, TotalAmount: 600,
	}
	active := base
	active.ID, active.TenantID, active.PersonalAccount = "r1", 1, "A1"
	active.Name, active.IdentityNumber, active.Status = "张三", "ID-A", "active"
	sealed := base
	sealed.ID, sealed.TenantID, sealed.PersonalAccount = "r2", 1, "A2"
	sealed.Name, sealed.IdentityNumber, sealed.Status = "李四", "ID-S", "sealed"
	require.NoError(t, db.Create(&active).Error)
	require.NoError(t, db.Create(&sealed).Error)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hr/fund/records/r1/seal", nil))
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var got HRPFRecord
	require.NoError(t, db.Where("id = ?", "r1").First(&got).Error)
	assert.Equal(t, "sealed", got.Status)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hr/fund/records/r1/unseal", nil))
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NoError(t, db.Where("id = ?", "r1").First(&got).Error)
	assert.Equal(t, "active", got.Status)

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hr/fund/bills", bytes.NewBufferString(`{"month":"2026-03"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	data := hrDecode(t, w)["data"].(map[string]any)
	assert.Equal(t, float64(1), data["record_count"])
	assert.Len(t, data["items"].([]any), 1)
}

// 8. 月份倒序（periods year_month DESC / bills month_label DESC）。
func TestHRMonthDescOrder(t *testing.T) {
	db := hrTestDB(t)
	for _, ym := range []string{"2026-01", "2026-03", "2026-02"} {
		require.NoError(t, db.Create(&HRSIPeriod{ID: uuid.New().String(), TenantID: 1, YearMonth: ym}).Error)
	}
	for _, ym := range []string{"2026-01", "2026-05", "2026-03"} {
		require.NoError(t, db.Create(&HRPFBill{ID: uuid.New().String(), TenantID: 1, MonthLabel: ym}).Error)
	}
	r := hrTestRouter(t, db)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hr/social/periods", nil))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	items := hrDecode(t, w)["data"].(map[string]any)["items"].([]any)
	var months []string
	for _, it := range items {
		months = append(months, it.(map[string]any)["year_month"].(string))
	}
	assert.Equal(t, []string{"2026-03", "2026-02", "2026-01"}, months)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/hr/fund/bills", nil))
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	items = hrDecode(t, w)["data"].(map[string]any)["items"].([]any)
	var labels []string
	for _, it := range items {
		labels = append(labels, it.(map[string]any)["month_label"].(string))
	}
	assert.Equal(t, []string{"2026-05", "2026-03", "2026-01"}, labels)
}

// 9. 9 表 AutoMigrate + 分类种子幂等（跑两次不重复）+ 单位默认配置。
func TestHRBootstrapIdempotent(t *testing.T) {
	db := hrTestDB(t)
	NewHRHandler(db)
	NewHRHandler(db) // 第二次：种子不得重复

	for _, m := range []any{
		&HREmployee{}, &HRSIPeriod{}, &HRSIRawRecord{}, &HRSICharge{}, &HRSIChange{},
		&HRPFSettings{}, &HRPFRecord{}, &HRPFBill{}, &HRPFBillItem{},
	} {
		assert.True(t, db.Migrator().HasTable(m), "%T", m)
	}
	for _, scope := range []string{hrScopeRoster, hrScopeSocial, hrScopeFund} {
		var n int64
		require.NoError(t, db.Model(&types.FleetCategory{}).
			Where("tenant_id = 0 AND scope = ?", scope).Count(&n).Error)
		assert.Equal(t, int64(1), n, "scope=%s", scope)
	}
	var s HRPFSettings
	require.NoError(t, db.Where("tenant_id = 0").First(&s).Error)
	assert.Equal(t, hrDefaultPFUnitName, s.UnitName)
	assert.Equal(t, hrDefaultPFUnitAccount, s.UnitAccount)
	assert.Equal(t, hrDefaultPFRatio, s.DefaultRatio)
	assert.True(t, fleetCategoryScopes[hrScopeRoster])
	assert.True(t, fleetCategoryScopes[hrScopeSocial])
	assert.True(t, fleetCategoryScopes[hrScopeFund])
}

// ---------------------------------------------------------------------------
// 第 46 轮：76 字段标准模板 / 四态 / 日期清洗 / 导入模板下载
// ---------------------------------------------------------------------------

func TestHRRosterTemplateHeaders(t *testing.T) {
	require.Len(t, hrRosterHeaders, 76)
	assert.Equal(t, "工号", hrRosterHeaders[0])
	assert.Equal(t, "姓名", hrRosterHeaders[1])
	assert.Equal(t, "现合同签订日期", hrRosterHeaders[75])
	seen := map[string]bool{}
	for _, h := range hrRosterHeaders {
		assert.False(t, seen[h], "重复表头 "+h)
		seen[h] = true
	}
	// 76 个表头自身 + 常见别名都能归一到标准名
	for _, h := range hrRosterHeaders {
		assert.Equal(t, h, hrRosterHeaderKey(h), "表头自身未命中: "+h)
	}
	for alias, want := range map[string]string{
		"身份证号码": "证件号码", "身份证号": "证件号码", "联系电话": "手机号码",
		"上家工作单位名称": "上家工作单位", "上家工作单位职务": "上家工作职务",
		"合同编号": "现合同编号", "合同类型": "现合同类型",
	} {
		assert.Equal(t, want, hrRosterHeaderKey(alias), alias)
	}
	assert.Len(t, hrRosterEnumGroups, 20)
	assert.Len(t, hrRosterEnumGroups["离职原因"], 42)
	assert.Equal(t, 11, len(hrRosterFieldGroups))
}

func TestHRRosterDateNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"2018-07-01 00:00:00":             "2018-07-01",
		"2018/7/1 0:00":                   "2018-07-01",
		"2018-07-01T00:00:00+08:00":       "2018-07-01",
		"2026-11-07T20:12:33.37663+08:00": "2026-11-07",
		"2018-07-01":                      "2018-07-01",
	} {
		assert.Equal(t, want, normalizeHRDateValue(in), in)
	}
	assert.Equal(t, "", normalizeHRDateValue("   "))
	assert.True(t, hrIsRosterDateField("入职日期"))
	assert.True(t, hrIsRosterDateField("身份证有效期"))
	assert.False(t, hrIsRosterDateField("工号"))
}

func TestHRRosterStatusNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"正式": hrRosterStatusActive, "在职": hrRosterStatusActive, "active": hrRosterStatusActive,
		"试用": hrRosterStatusProbation, "试用期": hrRosterStatusProbation,
		"已离职": hrRosterStatusResigned, "待离职": hrRosterStatusResigned, "resigned": hrRosterStatusResigned,
		"": hrRosterStatusActive,
	} {
		assert.Equal(t, want, normalizeHREmployeeStatus(in, "全职"), in)
	}
	assert.Equal(t, hrRosterStatusReemployed, normalizeHREmployeeStatus("正式", "退休返聘"))
	assert.Contains(t, hrStatusDBValues(hrRosterStatusResigned), "resigned")
	assert.Contains(t, hrStatusDBValues(hrRosterStatusActive), "active")
}

func TestHRRosterImportSkipsSampleAndKeepsAllFields(t *testing.T) {
	// 模板首行 + 示例行 + 一行真实数据
	header := append([]string{}, hrRosterHeaders...)
	sample := append([]string{}, hrRosterTemplateSample...)
	real := make([]string, len(hrRosterHeaders))
	for i := range real {
		real[i] = ""
	}
	set := func(name, value string) {
		for i, h := range hrRosterHeaders {
			if h == name {
				real[i] = value
				return
			}
		}
	}
	set("工号", "E001")
	set("姓名", "测试员工")
	set("证件号码", "110101199003072316")
	set("部门", "生产部")
	set("员工状态", "试用")
	set("工作性质", "全职")
	set("入职日期", "2018-07-01 00:00:00")
	rows := [][]string{header, sample, real}

	plan, err := hrPlanEmployeeImport(rows, nil, hrEmployeeImportOptions{Mode: "merge"})
	require.NoError(t, err)
	assert.Equal(t, 1, plan.Skipped, "示例行应被跳过")
	require.Len(t, plan.Inserted, 1)
	emp := plan.Inserted[0]
	assert.Equal(t, "测试员工", emp.Name)
	assert.Equal(t, "生产部", emp.Department)
	assert.Equal(t, hrRosterStatusProbation, emp.Status)
	assert.Equal(t, "2018-07-01", emp.HireDate)
	assert.Equal(t, "男", emp.Gender)
	// 76 个标准字段全部落进 custom_data（无损）
	for _, h := range hrRosterHeaders {
		_, ok := emp.CustomData[h]
		assert.True(t, ok, "custom_data 缺少字段 "+h)
	}
	assert.Equal(t, "2018-07-01", emp.CustomData["入职日期"])
	assert.Equal(t, hrRosterStatusProbation, emp.CustomData["在职状态"])
}

func TestHRRosterTemplateDownload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &HRHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.DownloadRosterTemplate(c)
	require.Equal(t, http.StatusOK, w.Code)
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	assert.Equal(t, []string{"导入数据", "说明", "枚举字段"}, f.GetSheetList())
	dataRows, err := f.GetRows("导入数据")
	require.NoError(t, err)
	require.NotEmpty(t, dataRows)
	assert.Len(t, dataRows[0], 76)
	assert.Equal(t, "工号", dataRows[0][0])
	assert.Equal(t, "现合同签订日期", dataRows[0][75])
	require.Len(t, dataRows, 2)
	assert.Equal(t, "000001", dataRows[1][0])
	assert.Equal(t, "示例员工", dataRows[1][1])
	enumRows, err := f.GetRows("枚举字段")
	require.NoError(t, err)
	assert.Len(t, enumRows, 20)
}

func TestHRResignProofIDWhitelist(t *testing.T) {
	valid := uuid.New().String() + ".pdf"
	assert.True(t, hrResignProofIDRe.MatchString(valid))
	for _, bad := range []string{
		"../../etc/passwd", "..\\secret.png", "abc.pdf", valid + "/x",
		uuid.New().String() + ".exe", "a/b/c.png",
	} {
		assert.False(t, hrResignProofIDRe.MatchString(bad), bad)
	}
	assert.Contains(t, hrResignProofURL(valid), "/roster/resign-proof/")
	assert.Equal(t, "", hrResignProofURL("../../etc/passwd"))
}
