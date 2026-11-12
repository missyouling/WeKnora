// 第 52 轮后端回归：6 组字段契约、三套独立模板、离职/合同导入解析器、
// 身份证联动与数据清洗、启动种子清理。
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/Tencent/WeKnora/internal/types"
)

func hrRound52Template(t *testing.T, module string) *excelize.File {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &HRHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/hr/roster/template?module="+module, nil)
	h.DownloadRosterTemplate(c)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), module+"-template.xlsx")
	f, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestHRRound52ThreeTemplates(t *testing.T) {
	for _, module := range []string{hrModuleRoster, hrModuleResign, hrModuleContract} {
		t.Run(module, func(t *testing.T) {
			f := hrRound52Template(t, module)
			assert.Equal(t, []string{"导入数据", "说明", "枚举字段"}, f.GetSheetList())
			rows, err := f.GetRows("导入数据")
			require.NoError(t, err)
			require.NotEmpty(t, rows)
			want := hrTemplateHeaders(module)
			assert.Equal(t, want, rows[0], "表头必须与精简字段 100% 匹配")
			require.Len(t, rows, 2, "模板应含 1 行示例")
			assert.Equal(t, "000001", rows[1][0])
			assert.Equal(t, "示例员工", rows[1][1])
			enumRows, err := f.GetRows("枚举字段")
			require.NoError(t, err)
			assert.Len(t, enumRows, len(hrTemplateSpecOf(module).EnumGroups))
			dvs, err := f.GetDataValidations("导入数据")
			require.NoError(t, err)
			assert.NotEmpty(t, dvs, "枚举列必须配置 Excel 下拉数据验证")
		})
	}
	// 三套表头互不相同
	r, s, c := hrTemplateHeaders(hrModuleRoster), hrTemplateHeaders(hrModuleResign), hrTemplateHeaders(hrModuleContract)
	assert.NotEqual(t, len(r), len(s))
	assert.NotEqual(t, len(s), len(c))
	// 花名册模板不得含已删除 / 已剥离字段
	for _, removed := range []string{"在职状态", "人员状态", "婚姻状况", "配偶姓名",
		"上家工作单位", "是否烈属", "现合同编号", "合同状态", "离职日期", "离职证明"} {
		assert.NotContains(t, r, removed)
	}
	// 离职 / 合同模板都带三个匹配键
	for _, headers := range [][]string{s, c} {
		assert.Equal(t, []string{"工号", "姓名", "证件号码"}, headers[:3])
	}
	assert.NotContains(t, s, "离职证明")
	assert.NotContains(t, c, "合同状态")
}

func TestHRRound52FieldSchema(t *testing.T) {
	assert.Len(t, hrRosterFieldDefs(), 48)
	assert.Len(t, hrResignFieldDefs(), 14)
	assert.Len(t, hrContractFieldDefs(), 17)
	assert.Len(t, hrRosterGroupDict52(), 6)

	// 日期字段全部是 date 类型
	for _, name := range []string{"入职日期", "转正日期", "身份证有效期", "出生日期",
		"毕业时间", "离职申请日期", "离职日期", "薪资结算日",
		"合同开始日期", "合同结束日期", "签订日期"} {
		found := false
		for _, defs := range [][]hrFieldDef{hrRosterFieldDefs(), hrResignFieldDefs(), hrContractFieldDefs()} {
			for _, d := range defs {
				if d.Name == name {
					assert.Equal(t, "date", hrDataTypeOfDef(d), name)
					found = true
				}
			}
		}
		assert.True(t, found, "日期字段 "+name+" 未登记")
	}
	// 枚举字典
	assert.Len(t, hrEnumOptions("离职原因"), 42)
	assert.Len(t, hrEnumOptions("试用期"), 13)
	assert.Len(t, hrEnumOptions("合同期限"), 11)
	// 引用字段带来源
	refs := 0
	for _, d := range hrResignFieldDefs() {
		if d.Source != nil {
			refs++
			assert.Equal(t, "花名册", d.Source.Category)
		}
	}
	assert.Equal(t, 5, refs)
	partyA := 0
	for _, d := range hrContractFieldDefs() {
		if d.Source != nil && d.Source.Category == "组织机构" {
			partyA++
		}
	}
	assert.Equal(t, 3, partyA)
}

func TestHRRound52SeedSubsAndBackfill(t *testing.T) {
	subs := hrRosterSeedSubs()
	assert.Len(t, subs, 48)
	assert.Equal(t, "工号", subs[0].Name)
	assert.Equal(t, "在职信息", subs[0].Group)
	assert.Equal(t, "date", subs[6].DataType)
	assert.Len(t, hrResignSeedSubs(), 14)
	assert.Len(t, hrContractSeedSubs52(), 17)
	// 引用来源写进 subs.Source
	found := false
	for _, s := range hrResignSeedSubs() {
		if s.Name == "姓名" && s.Source != nil {
			assert.Equal(t, "花名册", s.Source.Category)
			assert.Equal(t, "姓名", s.Source.Field)
			found = true
		}
	}
	assert.True(t, found)

	// 启动回填：已删除 / 已剥离的内置字段被剔除，用户自定义字段保留
	cat := &types.FleetCategory{BuiltinKey: "hr-roster", Subs: []types.FleetCategorySub{
		{Name: "姓名", Enabled: true},
		{Name: "家庭情况组字段", Enabled: true},
		{Name: "婚姻状况", Enabled: true},
		{Name: "现合同编号", Enabled: true},
		{Name: "自定义字段", Enabled: true},
	}}
	assert.True(t, hrBackfillRosterGroups([]*types.FleetCategory{cat}))
	names := []string{}
	for _, s := range cat.Subs {
		names = append(names, s.Name)
	}
	assert.Contains(t, names, "姓名")
	assert.Contains(t, names, "自定义字段")
	assert.NotContains(t, names, "婚姻状况")
	assert.NotContains(t, names, "现合同编号")
	assert.False(t, hrBackfillRosterGroups([]*types.FleetCategory{cat}), "回填必须幂等")

	// 劳动合同分类清理旧「现合同…」列
	cc := &types.FleetCategory{BuiltinKey: "hr-contract", Subs: []types.FleetCategorySub{
		{Name: "工号", Enabled: true},
		{Name: "现合同结束日期", Enabled: true},
		{Name: "职务", Enabled: true},
	}}
	hrBackfillRosterGroups([]*types.FleetCategory{cc})
	assert.Len(t, cc.Subs, 1)
	assert.Equal(t, "工号", cc.Subs[0].Name)
}

func TestHRRound52ImportResign(t *testing.T) {
	db := hrTestDB(t)
	h := &HRHandler{db: db}
	emp := HREmployee{
		ID: "emp-1", TenantID: 0, EmployeeID: "A020010001", Name: "张三",
		IDNumber: "51072319850217371X", Status: "active", CustomData: map[string]any{},
	}
	require.NoError(t, db.Create(&emp).Error)
	headers := hrTemplateHeaders(hrModuleResign)
	row := make([]string, len(headers))
	for i, hd := range headers {
		switch hd {
		case "工号":
			row[i] = "A020010001"
		case "姓名":
			row[i] = "张三"
		case "证件号码":
			row[i] = "51072319850217371X"
		case "离职日期":
			row[i] = "2026-10-31"
		case "离职类型":
			row[i] = "主动离职"
		case "离职原因":
			row[i] = "个人发展"
		case "是否试用期内离职":
			row[i] = "否"
		}
	}
	stats, err := h.applyHRModuleRows(context.Background(), 0, hrModuleResign, [][]string{headers, row})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Updated)
	var got HREmployee
	require.NoError(t, db.Where("id = ?", "emp-1").First(&got).Error)
	assert.Equal(t, "离职", got.Status)
	assert.Equal(t, "2026-10-31", got.ResignDate)
	assert.Equal(t, "主动离职", got.CustomData["离职类型"])
	assert.Equal(t, "离职", got.CustomData["员工状态"])

	// 未匹配员工的行被跳过；非法证件号的行被拒绝
	bad := make([]string, len(headers))
	copy(bad, row)
	bad[2] = "51072319851317371X"
	stats2, err := h.applyHRModuleRows(context.Background(), 0, hrModuleResign, [][]string{headers, bad})
	require.NoError(t, err)
	assert.Equal(t, 1, stats2.Invalid)
}

func TestHRRound52ImportContract(t *testing.T) {
	db := hrTestDB(t)
	h := &HRHandler{db: db}
	emp := HREmployee{
		ID: "emp-2", TenantID: 0, EmployeeID: "A020010002", Name: "李四",
		IDNumber: "440305199603051319", Status: "active", CustomData: map[string]any{},
	}
	require.NoError(t, db.Create(&emp).Error)
	headers := hrTemplateHeaders(hrModuleContract)
	row := make([]string, len(headers))
	for i, hd := range headers {
		switch hd {
		case "工号":
			row[i] = "A020010002"
		case "姓名":
			row[i] = "李四"
		case "证件号码":
			row[i] = "440305199603051319"
		case "合同类型":
			row[i] = "固定期限劳动合同"
		case "合同开始日期":
			row[i] = "2024-01-01"
		case "合同结束日期":
			row[i] = "2027-12-31"
		case "合同期限":
			row[i] = "3年"
		}
	}
	stats, err := h.applyHRModuleRows(context.Background(), 0, hrModuleContract, [][]string{headers, row})
	require.NoError(t, err)
	assert.Equal(t, 1, stats.Updated)
	var got HREmployee
	require.NoError(t, db.Where("id = ?", "emp-2").First(&got).Error)
	assert.Equal(t, "固定期限劳动合同", got.CustomData["合同类型"])
	assert.Equal(t, "2027-12-31", got.CustomData["合同结束日期"])
	// 新旧键双写，保证两种读法都命中
	assert.Equal(t, "2027-12-31", got.CustomData["现合同结束日期"])
	assert.Equal(t, hrContractStatusNormal, got.CustomData["合同状态"])
	assert.Equal(t, hrContractStatusNormal, hrEmployeeContractStatus(got))
}

func TestHRRound52EnrichEmployeeDerivation(t *testing.T) {
	e := &HREmployee{
		IDNumber: "5107 2319 8502 1737 1x",
		HireDate: "2018-07-01",
		CustomData: map[string]any{
			"手机号码":    "138-0013-8000",
			"工资卡卡号":   "6217 0037 6015",
			"现合同结束日期": "2020-01-01",
			"现合同类型":   "固定期限劳动合同",
		},
	}
	hrEnrichEmployee(e)
	assert.Equal(t, "51072319850217371X", e.IDNumber)
	assert.Equal(t, "13800138000", e.CustomData["手机号码"])
	assert.Equal(t, "621700376015", e.CustomData["工资卡卡号"])
	assert.Equal(t, "1985-02-17", e.CustomData["出生日期"])
	assert.Equal(t, "男", e.CustomData["性别"])
	assert.NotEmpty(t, e.CustomData["年龄"])
	assert.NotEmpty(t, e.CustomData["司龄"])
	// 旧全量模板带合同信息 → 自动同步劳动合同台账并重算状态
	assert.Equal(t, hrContractStatusExpired, e.CustomData["合同状态"])
}

func TestHRRound52CleanImportValue(t *testing.T) {
	assert.Equal(t, "51072319850217371X", hrCleanImportValue("证件号码", " 5107 2319 8502 1737 1x "))
	assert.Equal(t, "13800138000", hrCleanImportValue("手机号码", "138-0013-8000"))
	assert.Equal(t, "6217003760153123456", hrCleanImportValue("工资卡卡号", "6217 0037 6015 3123 456"))
	assert.Equal(t, "张三", hrCleanImportValue("姓名", "张\u200b三"))
	assert.True(t, hrIsValidIDCardImport("51072319850217371X"))
	assert.False(t, hrIsValidIDCardImport("51072319851317371X"))
	assert.False(t, hrIsValidIDCardImport("51072319851317371X"))
	assert.True(t, regexpMobile.MatchString("13800138000"))
	assert.False(t, regexpBankCard.MatchString("6217abc"))
}
