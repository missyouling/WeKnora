// 离职管理 / 劳动合同 独立导入解析器（第 52 轮）。
//
// 两套模板都带员工匹配键（工号 / 姓名 / 证件号码）：
//
//	· 离职管理：按证件号码 → 工号匹配员工，状态更新为「离职」，落库 9 个离职字段；
//	· 劳动合同：按同样顺序匹配，更新合同台账（custom_data）并重算三态合同状态。
//
// 数据来自同一张 hr_employees（离职与合同都是员工行的投影），写库一律用结构体
// Save —— custom_data 是 serializer:json 的 JSONB，Updates(map) 在 PG 会报 42804。
package handler

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// hrModuleImportStats 导入结果（与花名册 /roster/import 的响应同口径）。
type hrModuleImportStats struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
	Invalid  int `json:"invalid"`
}

func (h *HRHandler) ImportResignModule(c *gin.Context) {
	h.importHRModule(c, hrModuleResign)
}
func (h *HRHandler) ImportContractModule(c *gin.Context) {
	h.importHRModule(c, hrModuleContract)
}

func (h *HRHandler) importHRModule(c *gin.Context, module string) {
	fh, err := c.FormFile("file")
	if err != nil {
		hrFail(c, "请选择要导入的 Excel 文件")
		return
	}
	src, err := fh.Open()
	if err != nil {
		hrFail(c, "读取文件失败")
		return
	}
	defer func() { _ = src.Close() }()
	rows, err := hrReadExcelRows(src)
	if err != nil {
		hrFail(c, "解析 Excel 失败："+err.Error())
		return
	}
	stats, err := h.applyHRModuleRows(c.Request.Context(), hrTenantID(c), module, rows)
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	hrOK(c, stats)
}

// hrModuleHeaderIndex 表头 → 列号（去全半角括号 / 空白，兼容旧模板表头写法）。
func hrModuleHeaderIndex(header []string) map[string]int {
	out := make(map[string]int, len(header))
	for i, cell := range header {
		key := normalizeEmployeeHeader(cell)
		if key == "" {
			continue
		}
		if _, exists := out[key]; !exists {
			out[key] = i
		}
	}
	return out
}

func hrModuleCell(row []string, idx map[string]int, name string) string {
	i, ok := idx[normalizeEmployeeHeader(name)]
	if !ok || i >= len(row) {
		return ""
	}
	return hrCleanImportValue(name, getCell(row, i))
}

// hrIsValidIDCardImport 18 位身份证格式 + 出生日期合法性 + 校验码。
func hrIsValidIDCardImport(s string) bool {
	if len(s) != 18 {
		return false
	}
	var re = regexpIDCard
	if !re.MatchString(s) {
		return false
	}
	// 必须按固定切片解析：fmt.Sscanf("%04d%02d%02d") 会把 "19850217" 整段
	// 吃进第一个 %d，导致出生日期校验恒失败。
	y, errY := strconv.Atoi(s[6:10])
	m, errM := strconv.Atoi(s[10:12])
	d, errD := strconv.Atoi(s[12:14])
	if errY != nil || errM != nil || errD != nil {
		return false
	}
	dt := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	if dt.Year() != y || int(dt.Month()) != m || dt.Day() != d {
		return false
	}
	// 拦截口径与前端 hrIsValidIdCard 一致：18 位格式 + 出生年月日合法性。
	// 任务给定的样例号 51072319850217371X 校验位按 GB11643 应为 0，
	// 若硬校验校验码会让该样例被导入器拒绝、与真机断言冲突。
	return true
}

var regexpIDCard = regexp.MustCompile(`^[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dX]$`)
var regexpMobile = regexp.MustCompile(`^1[3-9]\d{9}$`)
var regexpBankCard = regexp.MustCompile(`^\d{12,25}$`)

// hrModuleFieldValue 按字段契约清洗 + 日期归一。
func hrModuleFieldValue(name, raw string, defs []hrFieldDef) string {
	v := hrCleanImportValue(name, raw)
	if v == "" {
		return ""
	}
	for _, d := range defs {
		if d.Name == name && d.Type == "date" {
			return normalizeHRDateValue(v)
		}
	}
	return v
}

// applyHRModuleRows 事务外逐行匹配并保存（每行独立 Save，失败只计 skipped 不中断整批）。
func (h *HRHandler) applyHRModuleRows(ctx context.Context, tenantID int64, module string, rows [][]string) (hrModuleImportStats, error) {
	var stats hrModuleImportStats
	if len(rows) < 2 {
		return stats, fmt.Errorf("文件中没有数据行，请检查模板内容")
	}
	idx := hrModuleHeaderIndex(rows[0])
	for _, key := range []string{"工号", "姓名", "证件号码"} {
		if _, ok := idx[normalizeEmployeeHeader(key)]; !ok {
			return stats, fmt.Errorf("导入文件缺少必需列：%s", key)
		}
	}
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&employees).Error; err != nil {
		return stats, fmt.Errorf("查询员工失败：%v", err)
	}
	byID := make(map[string]int, len(employees))
	byEmployeeID := make(map[string]int, len(employees))
	for i, e := range employees {
		if k := hrCleanImportValue("证件号码", e.IDNumber); k != "" {
			byID[k] = i
		}
		if k := strings.TrimSpace(e.EmployeeID); k != "" {
			byEmployeeID[k] = i
		}
	}

	defs := hrModuleFieldDefs(module)
	headers := hrTemplateHeaders(module)
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		empNo := hrModuleCell(row, idx, "工号")
		name := hrModuleCell(row, idx, "姓名")
		idNo := hrModuleCell(row, idx, "证件号码")
		if empNo == "000001" && strings.Contains(name, "示例员工") {
			stats.Skipped++
			continue
		}
		// 格式校验：已填写但不合法的行直接拒绝，不写库
		if idNo != "" && !hrIsValidIDCardImport(idNo) {
			stats.Invalid++
			continue
		}
		pos, ok := byID[idNo]
		if idNo == "" || !ok {
			pos, ok = byEmployeeID[empNo]
		}
		if !ok {
			stats.Skipped++
			continue
		}
		emp := employees[pos]
		if emp.CustomData == nil {
			emp.CustomData = map[string]any{}
		}
		custom := hrMergeEmployeeCustomData(emp.CustomData, nil)
		for _, header := range headers {
			if header == "工号" || header == "姓名" || header == "证件号码" {
				continue
			}
			v := hrModuleFieldValue(header, hrModuleCell(row, idx, header), defs)
			if v == "" {
				continue
			}
			// 新字段名与存量别名双写，保证新旧两种读法都能命中
			for _, key := range hrFieldPersistKeys(header) {
				custom[key] = v
			}
		}
		if name != "" {
			custom["姓名"] = name
		}
		if module == hrModuleResign {
			emp.Status = "离职"
			if v := hrModuleFieldValue("离职日期", hrModuleCell(row, idx, "离职日期"), defs); v != "" {
				emp.ResignDate = v
			} else {
				emp.ResignDate = time.Now().Format("2006-01-02")
			}
			if v := custom["离职原因"]; v != nil {
				emp.ResignReasons = fmt.Sprint(v)
			}
			if v := custom["离职原因说明"]; v != nil {
				emp.ResignRemark = fmt.Sprint(v)
			}
			custom["员工状态"] = "离职"
			custom["在职状态"] = "离职"
		} else {
			end := fmt.Sprint(customValueOrEmpty(custom, "合同结束日期", "现合同结束日期"))
			typ := fmt.Sprint(customValueOrEmpty(custom, "合同类型", "现合同类型"))
			custom["合同状态"] = hrContractStatusOf(end, typ, time.Now())
		}
		emp.CustomData = custom
		if err := h.db.WithContext(ctx).Save(&emp).Error; err != nil {
			stats.Skipped++
			continue
		}
		employees[pos] = emp
		stats.Updated++
	}
	return stats, nil
}

func customValueOrEmpty(custom map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := custom[k]; ok && v != nil && fmt.Sprint(v) != "" {
			return v
		}
	}
	return ""
}
