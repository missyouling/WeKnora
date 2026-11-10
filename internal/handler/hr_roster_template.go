// 花名册导入模板下载（第 46 轮）。
//
// 由 excelize 生成 3 个 Sheet，与《导入花名册模板.xlsx》对齐：
//
//	导入数据 —— 76 个标准表头 + 1 行示例员工（「姓名」表头标红加粗）
//	说明     —— 填写说明 + 11 大分组字段清单
//	枚举字段 —— 20 组标准下拉选项（A 列组名，B.. 为选项）
//
// 本文件由 .workbuddy/plans/gen_round46_template.py 生成，禁止手改示例数据。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"github.com/Tencent/WeKnora/internal/errors"
)

// hrRosterTemplateSample 是模板第 2 行的示例员工（76 个值，与表头一一对应）。
var hrRosterTemplateSample = []string{
	"000001",
	"示例员工",
	"张三",
	"Tom",
	"互联网部/产品部/产品一组",
	"组织负责人",
	"",
	"产品部经理",
	"产品经理",
	"B1",
	"13012340000",
	"身份证",
	"440305199603051319",
	"全职",
	"正式",
	"2018-07-01",
	"2个月",
	"2018-09-01",
	"广东/深圳",
	"广东省深圳市南山区",
	"XXXX有限公司",
	"11010101",
	"校招",
	"",
	"",
	"22020202",
	"2024-12-12",
	"女",
	"1996-03-05",
	"否",
	"未婚",
	"否",
	"中国",
	"汉族",
	"群众",
	"湖北/武汉",
	"广东/深圳",
	"城镇户口",
	"广东省深圳市南山区XXXX",
	"2017-07-01",
	"A型",
	"M",
	"2017-12-12",
	"123456",
	"wx888888",
	"123456@qq.com",
	"0755-2342343",
	"8803",
	"DDD@UBM.com",
	"广东省深圳市南山区XX小区A单元1203",
	"6216610188888888888",
	"中国银行",
	"中国银行高新区支行",
	"广东/深圳",
	"6217214888888888888",
	"6217003528888888888",
	"王小非",
	"父女",
	"13890780000",
	"广东省深圳市宝安区A小区C单元709",
	"本科",
	"深圳大学",
	"计算机科学与技术",
	"2013-09-01",
	"2017-06-30",
	"全日制",
	"学士",
	"点米科技",
	"产品经理",
	"XXXX有限公司",
	"201807010003",
	"固定期限劳动合同",
	"2018-07-01",
	"2020-06-30",
	"2年",
	"2018-07-01",
}

// hrRosterTemplateNotes 是「说明」Sheet 顶部的填写说明。
var hrRosterTemplateNotes = [][]string{
	{"填写说明"},
	{"1. 导入的 Excel 中表头名称建议和「花名册」表头字段的名称一致；合并单元格和复合表头无法识别"},
	{"2. 「姓名」必填且不可删除，已在表头中标红并加粗"},
	{"3. 「选项」类型字段，仅可选择模板支持的选项值，详细规则见「枚举字段」Sheet"},
	{"4. 公式无法识别，请确保单元格没有包含公式"},
	{"5. 日期类字段请填写 YYYY-MM-DD，系统会自动清洗带时间戳的写法"},
	{"6. 部门上下级用「/」隔开，请从一级部门开始填写，例如「研发中心/质量管理部/测试一组」"},
	{"7. 示例行（工号 000001 / 示例员工）导入时会自动跳过"},
}

// DownloadRosterTemplate GET /roster/template —— 返回《导入花名册模板.xlsx》。
func (h *HRHandler) DownloadRosterTemplate(c *gin.Context) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const dataSheet = "导入数据"
	if err := f.SetSheetName("Sheet1", dataSheet); err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}

	// Sheet 1：76 表头 + 1 行示例；「姓名」（第 2 列）标红加粗。
	for i, header := range hrRosterHeaders {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
			return
		}
		if err := f.SetCellValue(dataSheet, cell, header); err != nil {
			c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
			return
		}
		if header == "姓名" {
			styleID, serr := f.NewStyle(&excelize.Style{
				Font: &excelize.Font{Bold: true, Color: "FF0000"},
			})
			if serr == nil {
				_ = f.SetCellStyle(dataSheet, cell, cell, styleID)
			}
		}
		value := ""
		if i < len(hrRosterTemplateSample) {
			value = hrRosterTemplateSample[i]
		}
		sampleCell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(dataSheet, sampleCell, value)
	}
	_ = f.SetColWidth(dataSheet, "A", "BX", 18)

	// Sheet 2：说明（填写说明 + 11 大分组字段清单）
	const noteSheet = "说明"
	if _, err := f.NewSheet(noteSheet); err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}
	for r, row := range hrRosterTemplateNotes {
		cell, _ := excelize.CoordinatesToCellName(2, r+1)
		_ = f.SetCellValue(noteSheet, cell, row[0])
	}
	start := len(hrRosterTemplateNotes) + 2
	headerRow := start
	_ = f.SetCellValue(noteSheet, "B"+itoa(headerRow), "类型")
	_ = f.SetCellValue(noteSheet, "C"+itoa(headerRow), "表头名称")
	_ = f.SetCellValue(noteSheet, "D"+itoa(headerRow), "填写说明")
	cursor := headerRow + 1
	for _, g := range hrRosterFieldGroups {
		_ = f.SetCellValue(noteSheet, "B"+itoa(cursor), g.Group)
		for _, field := range g.Fields {
			_ = f.SetCellValue(noteSheet, "C"+itoa(cursor), field)
			if hint, ok := hrRosterEnumGroups[hrRosterEnumKeyOf(field)]; ok && len(hint) > 0 {
				joined := ""
				for i, v := range hint {
					if i >= 8 {
						joined += "…"
						break
					}
					if i > 0 {
						joined += "，"
					}
					joined += v
				}
				_ = f.SetCellValue(noteSheet, "D"+itoa(cursor), joined)
			}
			cursor++
		}
	}
	_ = f.SetColWidth(noteSheet, "B", "D", 36)

	// Sheet 3：枚举字段（20 组，A 列组名，B.. 选项）
	const enumSheet = "枚举字段"
	if _, err := f.NewSheet(enumSheet); err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}
	rowIdx := 0
	for _, group := range hrRosterEnumOrder {
		opts := hrRosterEnumGroups[group]
		if len(opts) == 0 {
			continue
		}
		rowIdx++
		_ = f.SetCellValue(enumSheet, "A"+itoa(rowIdx), group)
		for i, opt := range opts {
			cell, _ := excelize.CoordinatesToCellName(i+2, rowIdx)
			_ = f.SetCellValue(enumSheet, cell, opt)
		}
	}
	_ = f.SetColWidth(enumSheet, "A", "A", 18)
	_ = f.SetColWidth(enumSheet, "B", "BA", 22)

	buf, err := f.WriteToBuffer()
	if err != nil {
		c.Error(errors.NewInternalServerError("生成模板失败: " + err.Error()))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="roster-template.xlsx"; filename*=UTF-8''%E5%AF%BC%E5%85%A5%E8%8A%B1%E5%90%8D%E5%86%8C%E6%A8%A1%E6%9D%BF.xlsx`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}

// hrRosterEnumOrder 固定 20 组顺序（map 遍历顺序不稳定，模板必须可复现）。
var hrRosterEnumOrder = []string{
	"证件类型",
	"工作性质",
	"员工状态",
	"试用期",
	"离职原因",
	"性别",
	"是否农历生日",
	"是否已婚",
	"是否已育",
	"国家地区",
	"民族",
	"政治面貌",
	"户口性质",
	"血型",
	"最高学历",
	"学习形式",
	"学位类型",
	"合同类型",
	"现合同期限",
	"汇报上级类型",
}

// hrRosterEnumKeyOf 把 76 表头名映射到枚举组名（合同类型等旧名兼容）。
func hrRosterEnumKeyOf(field string) string {
	switch field {
	case "现合同类型":
		return "合同类型"
	case "员工状态", "在职状态":
		return "员工状态"
	default:
		return field
	}
}

func itoa(n int) string {
	if n <= 0 {
		return "1"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
