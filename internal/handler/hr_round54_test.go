// 第 54 轮回归：公积金 18 字段契约、网厅导入、合同五态、模板隔离引用字段。
package handler

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

// TestRound54FundFieldContract 公积金必须是 18 个网厅原生字段。
func TestRound54FundFieldContract(t *testing.T) {
	defs := hrFundFieldDefs()
	require.Len(t, defs, 18)
	names := make([]string, 0, 18)
	for _, d := range defs {
		names = append(names, d.Name)
		assert.Nil(t, d.Source, "公积金字段不允许跨表引用：%s", d.Name)
	}
	assert.Equal(t, []string{
		"职工账号", "职工姓名", "证件号码", "账户状态",
		"缴存基数(元)", "月缴存额(元)", "单位月缴存额(元)", "个人月缴存额(元)", "账户余额(元)",
		"单位缴存比例(%)", "职工缴存比例(%)", "联名卡银行", "联名卡号", "绑定手机",
		"是否公贷", "是否冻结", "是否可在线提取公贷", "是否可在线提取商贷",
	}, names)

	// 默认表头 14 个（4 个「是否」默认关）
	headers := 0
	for _, d := range defs {
		if d.DefaultHeader {
			headers++
		}
	}
	assert.Equal(t, 14, headers)

	assert.Equal(t, []string{"正常", "封存", "提取销户"}, hrEnumOptions("账户状态"))
	assert.Equal(t, []string{"正常", "未签订", "即将到期", "到期", "已解除"}, hrEnumOptions("合同状态"))
}

// TestRound54TemplateExcludesRefFields 导入模板严禁包含跨页引用字段。
func TestRound54TemplateExcludesRefFields(t *testing.T) {
	roster := hrTemplateHeaders(hrModuleRoster)
	for _, banned := range []string{"个人社保账号", "个人公积金账号", "合同状态"} {
		assert.NotContains(t, roster, banned, "花名册模板不得包含引用字段 %s", banned)
	}
	assert.Contains(t, roster, "工号")
	assert.Contains(t, roster, "工资卡卡号")

	// 离职 / 合同模板只含匹配键 + 自身字段
	resign := hrTemplateHeaders(hrModuleResign)
	assert.Equal(t, []string{"工号", "姓名", "证件号码"}, resign[:3])
	assert.NotContains(t, resign, "部门")
	assert.NotContains(t, resign, "个人社保账号")

	contract := hrTemplateHeaders(hrModuleContract)
	assert.Equal(t, []string{"工号", "姓名", "证件号码"}, contract[:3])
	assert.NotContains(t, contract, "统一社会信用代码")
	assert.NotContains(t, contract, "联系电话")
	assert.NotContains(t, contract, "合同状态")

	fund := hrTemplateHeaders(hrModuleFund)
	assert.Len(t, fund, 18)
	assert.Equal(t, "职工账号", fund[0])

	// 表头 / 文件名 / 说明三处同源
	assert.Equal(t, "公积金管理导入模板.xlsx", hrTemplateFileName(hrModuleFund))
	spec := hrTemplateSpecOf(hrModuleFund)
	assert.Len(t, spec.Headers, 18)
	assert.Contains(t, spec.EnumGroups, "账户状态")
}

// TestRound54ContractStatus5 合同五态：已解除 / 未签订 / 正常 / 即将到期 / 到期。
func TestRound54ContractStatus5(t *testing.T) {
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, hrContractStatusUnsigned, hrContractStatusOf("", "", now))
	assert.Equal(t, hrContractStatusNormal, hrContractStatusOf("", "无固定期限劳动合同", now))
	assert.Equal(t, hrContractStatusNormal, hrContractStatusOf("2027-12-31", "固定期限劳动合同", now))
	assert.Equal(t, hrContractStatusExpiring, hrContractStatusOf("2026-12-01", "固定期限劳动合同", now))
	assert.Equal(t, hrContractStatusExpired, hrContractStatusOf("2026-10-01", "固定期限劳动合同", now))

	// 离职员工一律「已解除」：日期没到也不能算生效
	resigned := HREmployee{Status: "离职", CustomData: map[string]any{
		"合同结束日期": "2030-12-31", "合同类型": "固定期限劳动合同",
	}}
	assert.Equal(t, hrContractStatusTerminated, hrEmployeeContractStatus(resigned))

	active := HREmployee{Status: "在职", CustomData: map[string]any{}}
	assert.Equal(t, hrContractStatusUnsigned, hrEmployeeContractStatus(active))
}

// TestRound54FundWebhallImport 网厅文件（前 2 行标题 + 空合并列 + 末行验证码）解析。
func TestRound54FundWebhallImport(t *testing.T) {
	db := hrTestDB(t)
	h := &HRHandler{db: db}
	ctx := context.Background()
	tid := int64(0)

	// 花名册先放两个人，验证导入后「个人公积金账号」回写
	require.NoError(t, db.Create(&HREmployee{
		ID: "e1", TenantID: tid, Name: "李永娇", IDNumber: "500106198907240845",
		Status: "在职", CustomData: map[string]any{},
	}).Error)

	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	set := func(cell string, v any) { _ = f.SetCellValue(sheet, cell, v) }
	// 第 1 行标题 / 第 2 行单位信息（A 列留空模拟合并单元格）
	set("B1", "单位职工明细")
	set("B2", "单位账号：201005128130")
	set("C2", "单位名称：重庆星达铜业有限公司")
	// 第 3 行列头：故意在 A 列留空（网厅的合并空列），并把单位写在括号里
	headers := []string{"", "职工账号", "职工姓名", "证件号码", "账户状态",
		"缴存基数（元）", "月缴存额（元）", "单位月缴存额（元）", "个人月缴存额（元）", "账户余额（元）",
		"单位缴存比例(%)", "职工缴存比例(%)", "联名卡银行", "联名卡号", "绑定手机",
		"是否公贷", "是否冻结", "是否可在线提取公贷", "是否可在线提取商贷"}
	for i, v := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		set(cell, v)
	}
	// 数据行
	row1 := []string{"", "801364932880", "李永娇", "500106198907240845", "正常",
		"6,000", "600", "300", "300", "18,600", "5", "5", "中国建设银行", "6217003760153123456",
		"13800138000", "否", "否", "是", "否"}
	for i, v := range row1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		set(cell, v)
	}
	// 末行验证码
	set("A5", "验证码：8F2A")

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)
	rows, err := hrReadFundSheet(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)

	// 列头必须被定位到第 3 行，且空列不影响后续列序
	hi, idx := hrFindFundHeaderRow(rows)
	assert.Equal(t, 2, hi)
	assert.Equal(t, 1, idx["职工账号"])
	assert.Equal(t, 3, idx["证件号码"])

	stats, err := h.hrFundApplyRows(ctx, tid, rows)
	require.NoError(t, err)
	assert.Equal(t, 1, stats["inserted"])
	assert.GreaterOrEqual(t, stats["skipped"], 1, "验证码行必须被跳过")

	var rec HRPFRecord
	require.NoError(t, db.Where("tenant_id = ?", tid).First(&rec).Error)
	assert.Equal(t, "801364932880", rec.PersonalAccount)
	assert.Equal(t, "正常", hrFundStatusCN(rec.Status))
	assert.Equal(t, 6000.0, rec.PersonalBase)
	assert.Equal(t, 600.0, rec.TotalAmount)
	assert.Equal(t, 300.0, rec.CompanyAmount)
	assert.Equal(t, 300.0, rec.PersonalAmount)
	assert.Equal(t, 18600.0, toFloat(toStringValue(rec.CustomData["账户余额(元)"])))
	assert.Equal(t, "6217003760153123456", toStringValue(rec.CustomData["联名卡号"]))

	// 职工账号自动回写花名册
	var emp HREmployee
	require.NoError(t, db.Where("tenant_id = ? AND id_number = ?", tid, "500106198907240845").
		First(&emp).Error)
	assert.Equal(t, "801364932880", emp.ProvidentFundNumber)
	assert.Equal(t, "801364932880", toStringValue(emp.CustomData["个人公积金账号"]))
}

// TestRound54FundSeedDistribution 种子必须正好 87 条：74 正常 + 7 封存 + 6 提取销户。
func TestRound54FundSeedDistribution(t *testing.T) {
	// 前 3 个用网厅真实姓名（账号必须一一对上），随后 7 个封存名单，其余补足到 81
	names := []string{"寇江", "李永娇", "贺伟",
		"谢家芬", "刘正均", "曾均华", "陈洁", "龙远高", "秦克章", "刘开祥"}
	employees := make([]HREmployee, 0, 81)
	for i := 0; i < 81; i++ {
		name := "员工" + itoa(i+1)
		if i < len(names) {
			name = names[i]
		}
		employees = append(employees, HREmployee{
			EmployeeID: "C0205" + itoa(10000+i),
			Name:       name,
			IDNumber:   "510232198" + itoa(10000000+i),
		})
	}
	rows := hrBuildFundSeedRows(employees)
	require.Len(t, rows, 87)
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.status]++
	}
	assert.Equal(t, 74, counts[hrFundStatusActive])
	assert.Equal(t, 7, counts[hrFundStatusSealed])
	assert.Equal(t, 6, counts[hrFundStatusExtracted])

	// 三名真实员工的账号必须与网厅一致
	fixed := map[string]string{}
	for _, r := range rows {
		fixed[r.name] = r.account
	}
	assert.Equal(t, "801504241502", fixed["寇江"])
	assert.Equal(t, "801364932880", fixed["李永娇"])
	assert.Equal(t, "801507495053", fixed["贺伟"])
}
