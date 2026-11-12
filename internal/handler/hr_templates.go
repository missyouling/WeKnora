// 三套 Excel 导入模板生成器（第 52 轮）。
//
// 花名册 / 离职管理 / 劳动合同各自生成独立标准模板，均含 3 个 Sheet：
//
//	导入数据 —— 表头与各自精简字段 100% 匹配 + 1 行示例；枚举列配 Excel 下拉数据验证
//	说明     —— 填写说明 + 分组字段清单（枚举列给出候选值提示）
//	枚举字段 —— 该模块真实存在的枚举字典（A 列组名，B.. 为选项）
//
// 表头、枚举、分组全部来自 hr_field_schema.go 的字段契约，不另维护第二份清单。
package handler

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// hrWriteTemplate 把指定模块的模板写入工作簿（Sheet1 已存在）。
func hrWriteTemplate(f *excelize.File, module string) error {
	spec := hrTemplateSpecOf(module)
	const dataSheet = "导入数据"
	if err := f.SetSheetName("Sheet1", dataSheet); err != nil {
		return err
	}

	// ---- Sheet 1：表头 + 示例行 + 枚举列数据验证 ----
	boldRed, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FF0000"}})
	sample := hrTemplateSample(spec.Module, spec.Headers)
	for i, header := range spec.Headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(dataSheet, cell, header); err != nil {
			return err
		}
		if header == "姓名" && boldRed > 0 {
			_ = f.SetCellStyle(dataSheet, cell, cell, boldRed)
		}
		if i < len(sample) {
			sc, _ := excelize.CoordinatesToCellName(i+1, 2)
			_ = f.SetCellValue(dataSheet, sc, sample[i])
		}
	}
	if last, err := excelize.ColumnNumberToName(len(spec.Headers)); err == nil {
		_ = f.SetColWidth(dataSheet, "A", last, 18)
	}
	for i, header := range spec.Headers {
		opts := hrEnumOptions(header)
		if len(opts) == 0 || strings.Join(opts, ",") == "" {
			continue
		}
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			continue
		}
		dv := excelize.NewDataValidation(true)
		dv.SetSqref(fmt.Sprintf("%s2:%s1000", col, col))
		// 超过 Excel 255 字符内联列表上限时静默退化为「枚举字段」Sheet 说明，
		// 不阻断模板生成（如 42 项离职原因）。
		if err := dv.SetDropList(opts); err != nil {
			continue
		}
		_ = f.AddDataValidation(dataSheet, dv)
	}

	// ---- Sheet 2：说明 ----
	const noteSheet = "说明"
	if _, err := f.NewSheet(noteSheet); err != nil {
		return err
	}
	for r, line := range spec.Notes {
		cell, _ := excelize.CoordinatesToCellName(2, r+1)
		_ = f.SetCellValue(noteSheet, cell, line)
	}
	headerRow := len(spec.Notes) + 2
	_ = f.SetCellValue(noteSheet, "B"+itoa(headerRow), "类型")
	_ = f.SetCellValue(noteSheet, "C"+itoa(headerRow), "表头名称")
	_ = f.SetCellValue(noteSheet, "D"+itoa(headerRow), "填写说明")
	cursor := headerRow + 1
	for _, g := range spec.Groups {
		_ = f.SetCellValue(noteSheet, "B"+itoa(cursor), g.Key)
		for _, field := range g.Fields {
			_ = f.SetCellValue(noteSheet, "C"+itoa(cursor), field)
			if hint := hrEnumOptions(field); len(hint) > 0 {
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

	// ---- Sheet 3：枚举字段 ----
	const enumSheet = "枚举字段"
	if _, err := f.NewSheet(enumSheet); err != nil {
		return err
	}
	for rowIdx, group := range spec.EnumGroups {
		opts := hrEnumOptions(group)
		if len(opts) == 0 {
			continue
		}
		r := rowIdx + 1
		_ = f.SetCellValue(enumSheet, "A"+itoa(r), group)
		for i, opt := range opts {
			cell, _ := excelize.CoordinatesToCellName(i+2, r)
			_ = f.SetCellValue(enumSheet, cell, opt)
		}
	}
	_ = f.SetColWidth(enumSheet, "A", "A", 18)
	_ = f.SetColWidth(enumSheet, "B", "BA", 22)
	return nil
}
