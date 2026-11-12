// 人事模块 Excel 通用工具与三张中文表头别名表（源项目 service/processor.go 原样移植）。
//
// round2 不在这里定义：handler 包 utility.go:1310 已有带 1e-6 浮点修正的版本，
// 同包直接复用，避免重复定义导致编译失败。
package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// 五险明细表头（源项目 processor.go:107-120）。
var headerMap = map[string]string{
	"序号":      "seq",
	"姓名":      "name",
	"证件类型":    "id_type",
	"证件号码":    "id_number",
	"部门":      "department",
	"缴费工资":    "salary",
	"缴费基数":    "base",
	"费率":      "rate",
	"应缴费额":    "amount_due",
	"减免费额":    "deduction",
	"应补(退)费额": "amount_adjust",
	"人员编号":    "person_code",
}

// 花名册简表表头（源项目 processor.go:122-131）。
var rosterHeaderMap = map[string]string{
	"姓名":    "name",
	"证件号码":  "id_number",
	"身份证号码": "id_number",
	"身份证号":  "id_number",
	"部门":    "department",
	"岗位":    "title",
	"职务":    "title",
	"备注":    "remarks",
}

// employeeHeaderAliases 花名册中文表头 → 字段 key（源项目 processor.go:133-196 全量）。
var employeeHeaderAliases = map[string]string{
	"工号":            "employee_id",
	"员工编号":          "employee_id",
	"编号":            "employee_id",
	"姓名":            "name",
	"名称":            "name",
	"部门":            "department",
	"部门名称":          "department",
	"所在部门":          "department",
	"岗位":            "position",
	"职位":            "position",
	"岗位名称":          "position",
	"性别":            "gender",
	"入职时间":          "hire_date",
	"入职日期":          "hire_date",
	"年龄":            "age",
	"工龄":            "work_years",
	"出生月份":          "birth_month",
	"文化程度":          "education",
	"政治面貌":          "political_status",
	"工作服":           "work_clothing_size",
	"工作服尺码":         "work_clothing_size",
	"劳保鞋":           "safety_shoe_size",
	"劳保鞋尺码":         "safety_shoe_size",
	"户口性质":          "household_type",
	"民族":            "ethnicity",
	"籍贯":            "native_place",
	"身份证地址":         "id_address",
	"户籍地址":          "id_address",
	"身份证住址":         "id_address",
	"身份证号码":         "id_number",
	"身份证号":          "id_number",
	"证件号码":          "id_number",
	"证件号":           "id_number",
	"婚姻状况":          "marital_status",
	"社保":            "social_insurance",
	"是否缴纳社保":        "social_insurance",
	"是否生育":          "has_birth",
	"联系电话":          "phone",
	"联系方式":          "phone",
	"手机号":           "phone",
	"手机号码":          "phone",
	"社保编号":          "social_insurance_number",
	"社保个人编号":        "social_insurance_number",
	"个人编号":          "social_insurance_number",
	"公积金编号":         "provident_fund_number",
	"紧急联系人":         "emergency_contact",
	"紧急联系电话":        "emergency_phone",
	"紧急联系电话(家庭电话)":  "emergency_phone",
	"家庭电话/紧急情况联系电话": "emergency_phone",
	"现居住地址":         "current_address",
	"现住址":           "current_address",
	"毕业院校":          "graduate_school",
	"毕业学校":          "graduate_school",
	"专业":            "major",
	"毕业时间":          "graduation_time",
	"邮箱":            "email",
	"电子邮箱":          "email",
	"备注":            "remarks",
	"状态":            "status",
	"员工状态":          "status",
	"离职日期":          "resign_date",
	"离职时间":          "resign_date",
}

// normalizeEmployeeHeader 去全半角括号/空白并转小写，作为别名表的查询键。
func normalizeEmployeeHeader(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	replacer := strings.NewReplacer("（", "", "）", "", "(", "", ")", "", " ", "", "\t", "", "\n", "", "\r", "")
	return strings.ToLower(replacer.Replace(header))
}

// normalizeIDNumber 身份证去首尾空白与内部空格（Excel 常把长数字拆成带空格文本）。
func normalizeIDNumber(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ReplaceAll(value, " ", "")
}

// normalizeDateValue 兼容 Excel 序列日期、中文年月日、点/斜杠分隔与缺位格式。
func normalizeDateValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		if t, err := excelize.ExcelDateToTime(f, false); err == nil {
			return t.Format("2006-01-02")
		}
	}
	replacer := strings.NewReplacer("年", "-", "月", "-", "日", "", ".", "-", "/", "-")
	clean := replacer.Replace(value)
	for _, layout := range []string{"2006-1-2", "2006-01-02", "2006-1", "2006-01", "2006"} {
		if t, err := time.Parse(layout, clean); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return value
}

func stripBOM(value string) string { return strings.TrimPrefix(value, "\ufeff") }

func getCell(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(stripBOM(row[idx]))
}

// normalizeEmployeeStatus 在职/离职多写法归一。
func normalizeEmployeeStatus(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	switch strings.ToLower(v) {
	case "在职", "active", "就职", "在岗", "正常":
		return "active"
	case "离职", "resigned", "离岗", "已离职", "terminated":
		return "resigned"
	default:
		return v
	}
}

func toInt(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return 0
	}
	if i, err := strconv.Atoi(value); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return int(f)
	}
	return 0
}

func toFloat(value string) float64 {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ",", "")
	value = strings.TrimSuffix(value, "%")
	if value == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return 0
}

func maxFloat(a, b float64) float64 {
	if a >= b {
		return a
	}
	return b
}

// hrNormalizeMonthLabel 月份归一为 YYYY-MM。
func hrNormalizeMonthLabel(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("month is required")
	}
	parsed, err := time.Parse("2006-01", trimmed)
	if err != nil {
		return "", err
	}
	return parsed.Format("2006-01"), nil
}

// ---------------- 18 位身份证派生（性别 / 出生月份 / 年龄 / 工龄） ----------------

// hrIDCardGender 18 位身份证第 17 位：奇男偶女。
func hrIDCardGender(id string) string {
	if len(id) != 18 {
		return ""
	}
	if (int(id[16]-'0') % 2) == 1 {
		return "男"
	}
	return "女"
}

// hrIDCardBirthMonth 返回 "YYYY-MM"。
func hrIDCardBirthMonth(id string) string {
	if len(id) != 18 {
		return ""
	}
	return id[6:10] + "-" + id[10:12]
}

// hrIDCardAgeAt 计算指定日期下的周岁；非法身份证返回 0。
func hrIDCardAgeAt(id string, now time.Time) int {
	if len(id) != 18 {
		return 0
	}
	y, err := strconv.Atoi(id[6:10])
	if err != nil {
		return 0
	}
	m, err := strconv.Atoi(id[10:12])
	if err != nil {
		return 0
	}
	d, err := strconv.Atoi(id[12:14])
	if err != nil {
		return 0
	}
	age := now.Year() - y
	if int(now.Month()) < m || (int(now.Month()) == m && now.Day() < d) {
		age--
	}
	if age < 0 {
		age = 0
	}
	return age
}

// hrIDCardAge 周岁（整数串，仅用于兼容旧数据；列表/详情展示用 hrIDCardAgeDecimal）。
func hrIDCardAge(id string) string { return strconv.Itoa(hrIDCardAgeAt(id, time.Now())) }

// hrIDCardBirthDate 从 18 位身份证取出生日期 YYYY-MM-DD。
func hrIDCardBirthDate(id string) string {
	if len(id) != 18 {
		return ""
	}
	return normalizeHRDateValue(id[6:10] + "-" + id[10:12] + "-" + id[12:14])
}

// hrAgeDecimalAt 精确年龄，保留 1 位小数。
// 公式与任务一致：((today - birthDate) / (365.2425 * 86400000)).toFixed(1)。
func hrAgeDecimalAt(birthDate string, now time.Time) string {
	bd, err := time.Parse("2006-01-02", normalizeHRDateValue(birthDate))
	if err != nil {
		return ""
	}
	days := now.Sub(bd).Hours() / 24
	if days < 0 {
		return ""
	}
	age := days / 365.2425
	return strconv.FormatFloat(age, 'f', 1, 64)
}

// hrAgeDecimalOf 优先用出生日期，缺失时回落到身份证。
func hrAgeDecimalOf(e *HREmployee, now time.Time) string {
	birth := strings.TrimSpace(e.BirthMonth)
	// 出生月份可能是 YYYY-MM（旧数据），补齐成 YYYY-MM-DD 参与计算
	if len(birth) == 7 {
		birth += "-01"
	}
	if birth == "" || len(birth) < 10 {
		if bd := hrIDCardBirthDate(normalizeIDNumber(e.IDNumber)); bd != "" {
			birth = bd
		}
	}
	if len(birth) < 10 {
		return ""
	}
	return hrAgeDecimalAt(birth, now)
}

// hrWorkYearsAt 入职日期算工龄，返回 "X年Y个月"。
func hrWorkYearsAt(hireDate string, now time.Time) string {
	clean := normalizeDateValue(hireDate)
	t, err := time.Parse("2006-01-02", clean)
	if err != nil {
		return ""
	}
	months := (now.Year()-t.Year())*12 + int(now.Month()) - int(t.Month())
	if now.Day() < t.Day() {
		months--
	}
	if months < 0 {
		months = 0
	}
	return fmt.Sprintf("%d年%d个月", months/12, months%12)
}

func hrWorkYears(hireDate string) string { return hrWorkYearsAt(hireDate, time.Now()) }

// hrSeniorityOf 计算「司龄」：从入职日期起算，离职者截至离职日期（第 49 轮）。
//
// 与工龄的唯一区别就是截止日：工龄恒算到今天，司龄按在职状态取离职日。
// 前端展示统一叫「司龄」，工龄字段已下线。
func hrSeniorityOf(e *HREmployee, now time.Time) string {
	if e == nil {
		return ""
	}
	hire := strings.TrimSpace(e.HireDate)
	if hire == "" {
		return ""
	}
	end := now
	if resign := strings.TrimSpace(e.ResignDate); resign != "" {
		if t, err := time.Parse("2006-01-02", normalizeHRDateValue(resign)); err == nil {
			end = t
		}
	}
	return hrWorkYearsAt(hire, end)
}

// hrEnrichEmployee 用身份证自动补性别/出生月份/年龄，用入职日期补工龄（仅补空值，不覆盖手填）。
//
// 第 52 轮追加：① 录入/导入时统一清洗号码类字段；② 身份证联动写入
// custom_data 的出生日期/性别/年龄；③ 旧全量模板带合同信息时自动同步写入
// 劳动合同台账并重算三态合同状态。
func hrEnrichEmployee(e *HREmployee) {
	id := normalizeIDNumber(e.IDNumber)
	e.IDNumber = hrCleanImportValue("证件号码", id)
	if e.CustomData == nil {
		e.CustomData = map[string]any{}
	}
	custom := e.CustomData
	for _, key := range []string{"证件号码", "手机号码", "工资卡卡号", "紧急联系人电话", "工作电话"} {
		if v, ok := custom[key]; ok && v != nil {
			custom[key] = hrCleanImportValue(key, fmt.Sprint(v))
		}
	}
	if len(id) == 18 {
		if e.Gender == "" {
			e.Gender = hrIDCardGender(id)
		}
		if bd := hrIDCardBirthDate(id); bd != "" {
			// 出生日期是主口径；出生月份（YYYY-MM）作为兼容字段同步
			if e.BirthMonth == "" || len(e.BirthMonth) < 10 {
				e.BirthMonth = bd
			}
		}
		// 年龄统一按 1 位小数重算（第 47 轮），不再沿用整数旧值
		if age := hrAgeDecimalOf(e, time.Now()); age != "" {
			e.Age = age
		}
	}
	if e.WorkYears == "" && strings.TrimSpace(e.HireDate) != "" {
		e.WorkYears = hrWorkYears(e.HireDate)
	}
	// 第 49 轮：司龄是唯一对外口径（工龄字段已下线），随入职/离职日期重算。
	if seniority := hrSeniorityOf(e, time.Now()); seniority != "" {
		custom["司龄"] = seniority
	}
	// 第 52 轮：身份证自动推导（出生日期 / 性别 / 年龄），与前端抽屉同口径。
	if len(id) == 18 {
		if bd := hrIDCardBirthDate(id); bd != "" {
			custom["出生日期"] = bd
			custom["性别"] = hrIDCardGender(id)
		}
		if age := hrAgeDecimalOf(e, time.Now()); age != "" {
			custom["年龄"] = age
		}
	}
	// 旧全量花名册模板带「现合同…」列时，自动同步到劳动合同台账并重算合同状态。
	end := fmt.Sprint(customValueOrEmpty(custom, "合同结束日期", "现合同结束日期"))
	typ := fmt.Sprint(customValueOrEmpty(custom, "合同类型", "现合同类型"))
	if end != "" || typ != "" {
		custom["合同状态"] = hrContractStatusOf(end, typ, time.Now())
	}
}

// ---------------- Excel 读写辅助 ----------------

// hrReadExcelRows 从 multipart/文件 reader 读取第一个工作表全部行。
func hrReadExcelRows(r io.Reader) ([][]string, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("Excel 文件中没有工作表")
	}
	return f.GetRows(sheets[0])
}

// hrExcelDownload 以中文表头 + 数据行写出 xlsx 并作为附件响应。
func hrExcelDownload(c *gin.Context, filename string, headers []string, rows [][]string) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	sheet := f.GetSheetName(0)
	for col, h := range headers {
		axis, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = f.SetCellValue(sheet, axis, h)
	}
	for r, row := range rows {
		for col, val := range row {
			axis, _ := excelize.CoordinatesToCellName(col+1, r+2)
			_ = f.SetCellValue(sheet, axis, val)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		hrFail(c, "生成 Excel 失败: "+err.Error())
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}
