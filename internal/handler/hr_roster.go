// 花名册：CRUD、Excel 导入（merge/insert/update + 离职冲突 409 + force_transition）、
// 导出、离职/恢复/批量删除、stats。解析与冲突判定抽成纯函数 hrPlanEmployeeImport 便于 TDD。
package handler

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func hrFloatStr(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func hrParseEmployeeImportMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "insert":
		return "insert"
	case "update":
		return "update"
	default: // merge / upsert / all / 空
		return "merge"
	}
}

func hrTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

type hrEmployeeImportOptions struct {
	Mode               string
	DefaultStatus      string
	ForceDefaultStatus bool
}

type hrEmployeeImportConflict struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	IdentityNumber string `json:"identity_number"`
	Status         string `json:"status"`
	Department     string `json:"department"`
	Position       string `json:"position"`
}

type hrEmployeeImportPlan struct {
	Mode      string
	Inserted  []HREmployee
	Updated   []HREmployee
	Skipped   int
	Conflicts []hrEmployeeImportConflict
}

type hrEmployeeCandidate struct {
	record   HREmployee
	existing bool
}

// hrFillEmployeeFrom 用反射把 src 非空字符串字段补进 dst 的空字段（照源 599-700 的逐字段兜底）。
// ID 是 string 但已赋为 existing.ID（非空），不会被覆盖；时间/整型/JSON 字段自动跳过。
func hrFillEmployeeFrom(dst, src *HREmployee) {
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src).Elem()
	for i := 0; i < dv.NumField(); i++ {
		f := dv.Field(i)
		if f.Kind() == reflect.String && f.CanSet() && f.String() == "" {
			if s := sv.Field(i).String(); s != "" {
				f.SetString(s)
			}
		}
	}
}

// hrPlanEmployeeImport 解析 Excel 行、命中已有 active 员工、处理离职冲突与三模式筛选。
// 不落库：持久化由 ImportRoster 完成。额外列（非别名表头）写入 custom_data。
func hrPlanEmployeeImport(rows [][]string, existing []HREmployee, opts hrEmployeeImportOptions) (hrEmployeeImportPlan, error) {
	var plan hrEmployeeImportPlan
	plan.Mode = hrParseEmployeeImportMode(opts.Mode)
	defaultStatus := normalizeEmployeeStatus(opts.DefaultStatus)

	normalizedMap := make(map[string]string, len(employeeHeaderAliases))
	for raw, field := range employeeHeaderAliases {
		normalizedMap[normalizeEmployeeHeader(raw)] = field
	}
	if len(rows) < 2 {
		return plan, fmt.Errorf("员工文件中没有数据行，请检查模板内容")
	}

	header := rows[0]
	indexMap := map[string]int{}
	extraHeaders := map[int]string{}
	for idx, cell := range header {
		key := normalizeEmployeeHeader(cell)
		if key == "" {
			continue
		}
		if field, ok := normalizedMap[key]; ok {
			indexMap[field] = idx
		} else {
			extraHeaders[idx] = strings.TrimSpace(stripBOM(cell))
		}
	}
	for _, key := range []string{"id_number", "name"} {
		if _, ok := indexMap[key]; !ok {
			return plan, fmt.Errorf("导入文件缺少必需列：%s", key)
		}
	}

	// 已有员工：仅 active 参与去重（离职员工重新导入视为新行/由 OnConflict 覆盖）。
	existingMap := make(map[string]HREmployee, len(existing))
	for _, emp := range existing {
		if normalizeEmployeeStatus(emp.Status) != "active" {
			continue
		}
		if key := normalizeIDNumber(emp.IDNumber); key != "" {
			existingMap[key] = emp
		}
	}

	skipped := 0
	unique := make(map[string]hrEmployeeCandidate)
	var conflicts []hrEmployeeImportConflict

	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		idNumber := normalizeIDNumber(getCell(row, indexMap["id_number"]))
		if idNumber == "" {
			skipped++
			continue
		}
		name := strings.TrimSpace(getCell(row, indexMap["name"]))
		entry := HREmployee{IDNumber: idNumber, Name: name, CustomData: map[string]any{}}

		setField := func(key string, setter func(string)) {
			if idx, ok := indexMap[key]; ok {
				setter(getCell(row, idx))
			}
		}
		setField("employee_id", func(v string) { entry.EmployeeID = v })
		setField("department", func(v string) { entry.Department = v })
		setField("position", func(v string) { entry.Position = v })
		setField("gender", func(v string) { entry.Gender = v })
		setField("hire_date", func(v string) { entry.HireDate = normalizeDateValue(v) })
		setField("age", func(v string) { entry.Age = v })
		setField("work_years", func(v string) { entry.WorkYears = v })
		setField("birth_month", func(v string) { entry.BirthMonth = v })
		setField("education", func(v string) { entry.Education = v })
		setField("political_status", func(v string) { entry.PoliticalStatus = v })
		setField("work_clothing_size", func(v string) { entry.WorkClothingSize = v })
		setField("safety_shoe_size", func(v string) { entry.SafetyShoeSize = v })
		setField("household_type", func(v string) { entry.HouseholdType = v })
		setField("ethnicity", func(v string) { entry.Ethnicity = v })
		setField("native_place", func(v string) { entry.NativePlace = v })
		setField("id_address", func(v string) { entry.IDAddress = v })
		setField("marital_status", func(v string) { entry.MaritalStatus = v })
		setField("social_insurance", func(v string) { entry.SocialInsurance = v })
		setField("has_birth", func(v string) { entry.HasBirth = v })
		setField("phone", func(v string) { entry.Phone = v })
		setField("social_insurance_number", func(v string) { entry.SocialInsuranceNumber = v })
		setField("provident_fund_number", func(v string) { entry.ProvidentFundNumber = v })
		setField("emergency_contact", func(v string) { entry.EmergencyContact = v })
		setField("emergency_phone", func(v string) { entry.EmergencyPhone = v })
		setField("current_address", func(v string) { entry.CurrentAddress = v })
		setField("graduate_school", func(v string) { entry.GraduateSchool = v })
		setField("major", func(v string) { entry.Major = v })
		setField("graduation_time", func(v string) { entry.GraduationTime = normalizeDateValue(v) })
		setField("email", func(v string) { entry.Email = v })
		setField("remarks", func(v string) { entry.Remarks = v })
		setField("status", func(v string) { entry.Status = normalizeEmployeeStatus(v) })
		setField("resign_date", func(v string) { entry.ResignDate = normalizeDateValue(v) })
		for idx, h := range extraHeaders {
			if v := getCell(row, idx); v != "" {
				entry.CustomData[h] = v
			}
		}

		if entry.Name == "" {
			if ex, ok := existingMap[idNumber]; ok && ex.Name != "" {
				entry.Name = ex.Name
			}
		}
		if entry.Name == "" {
			skipped++
			continue
		}

		existingRecord, hasExisting := existingMap[idNumber]
		if hasExisting {
			// 离职冲突：默认状态 resigned 且非 force 且存量仍在职 → 收集冲突并跳过。
			if defaultStatus == "resigned" && !opts.ForceDefaultStatus && normalizeEmployeeStatus(existingRecord.Status) != "resigned" {
				conflicts = append(conflicts, hrEmployeeImportConflict{
					ID:             existingRecord.ID,
					Name:           existingRecord.Name,
					IdentityNumber: existingRecord.IDNumber,
					Status:         existingRecord.Status,
					Department:     existingRecord.Department,
					Position:       existingRecord.Position,
				})
				skipped++
				continue
			}
			entry.ID = existingRecord.ID
			hrFillEmployeeFrom(&entry, &existingRecord)
			if len(entry.CustomData) == 0 && len(existingRecord.CustomData) > 0 {
				entry.CustomData = existingRecord.CustomData
			}
			if defaultStatus != "" {
				if opts.ForceDefaultStatus {
					entry.Status = defaultStatus
				} else if entry.Status == "" && existingRecord.Status != "" {
					entry.Status = existingRecord.Status
				}
			} else if entry.Status == "" && existingRecord.Status != "" {
				entry.Status = existingRecord.Status
			}
		} else if defaultStatus != "" {
			entry.Status = defaultStatus
		}
		if entry.Status == "" {
			entry.Status = "active"
		}
		hrEnrichEmployee(&entry)
		unique[idNumber] = hrEmployeeCandidate{record: entry, existing: hasExisting}
	}

	if len(unique) == 0 && len(conflicts) == 0 {
		return plan, fmt.Errorf("未识别到有效的员工数据，请检查模板格式")
	}
	plan.Conflicts = conflicts
	plan.Skipped = skipped

	var insertedEntries, updatedEntries []HREmployee
	for _, cand := range unique {
		if cand.existing {
			updatedEntries = append(updatedEntries, cand.record)
		} else {
			insertedEntries = append(insertedEntries, cand.record)
		}
	}
	switch plan.Mode {
	case "insert":
		plan.Inserted = insertedEntries
		plan.Skipped += len(updatedEntries)
	case "update":
		plan.Updated = updatedEntries
		plan.Skipped += len(insertedEntries)
	default: // merge
		plan.Inserted = insertedEntries
		plan.Updated = updatedEntries
	}
	return plan, nil
}

// hrEmployeeUpdateColumns OnConflict DoUpdates 的列（不含主键/tenant_id/created_at）。
var hrEmployeeUpdateColumns = []string{
	"employee_id", "name", "department", "position", "gender", "hire_date", "age", "work_years",
	"birth_month", "education", "political_status", "work_clothing_size", "safety_shoe_size",
	"household_type", "ethnicity", "native_place", "id_address", "marital_status", "social_insurance",
	"has_birth", "phone", "social_insurance_number", "provident_fund_number", "emergency_contact",
	"emergency_phone", "current_address", "graduate_school", "major", "graduation_time", "email",
	"remarks", "status", "resign_date", "resign_reasons", "custom_data", "updated_at",
}

// ListEmployees 分页/搜索/状态过滤。
func (h *HRHandler) ListEmployees(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	q := h.db.WithContext(ctx).Model(&HREmployee{}).Where("tenant_id = ?", tid)
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR id_number LIKE ? OR employee_id LIKE ? OR department LIKE ?", like, like, like, like)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		hrFail(c, "查询花名册失败: "+err.Error())
		return
	}
	var items []HREmployee
	if err := q.Order("created_at DESC, id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		hrFail(c, "查询花名册失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"items": items, "total": total})
}

func (h *HRHandler) CreateEmployee(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req HREmployee
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.ID = uuid.New().String()
	req.TenantID = tid
	req.IDNumber = normalizeIDNumber(req.IDNumber)
	req.Name = strings.TrimSpace(req.Name)
	if req.IDNumber == "" || req.Name == "" {
		hrFail(c, "姓名和身份证号码必填")
		return
	}
	req.Status = normalizeEmployeeStatus(req.Status)
	if req.Status == "" {
		req.Status = "active"
	}
	hrEnrichEmployee(&req)
	if req.CustomData == nil {
		req.CustomData = map[string]any{}
	}
	var cnt int64
	h.db.WithContext(ctx).Model(&HREmployee{}).Where("tenant_id = ? AND id_number = ?", tid, req.IDNumber).Count(&cnt)
	if cnt > 0 {
		hrFail(c, "该身份证号码已存在")
		return
	}
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		hrFail(c, "创建员工失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) UpdateEmployee(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var existing HREmployee
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&existing).Error; err != nil {
		hrFail(c, "员工不存在")
		return
	}
	var req HREmployee
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.ID = existing.ID
	req.TenantID = existing.TenantID
	req.CreatedAt = existing.CreatedAt
	req.IDNumber = normalizeIDNumber(req.IDNumber)
	if req.IDNumber == "" {
		req.IDNumber = existing.IDNumber
	}
	if req.Status == "" {
		req.Status = existing.Status
	}
	hrEnrichEmployee(&req)
	if req.CustomData == nil {
		req.CustomData = existing.CustomData
	}
	if err := h.db.WithContext(ctx).Save(&req).Error; err != nil {
		hrFail(c, "更新员工失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) DeleteEmployee(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	res := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).Delete(&HREmployee{})
	if res.Error != nil {
		hrFail(c, "删除员工失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}

func (h *HRHandler) BatchDeleteEmployees(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.IDs) == 0 {
		hrFail(c, "ids 不能为空")
		return
	}
	res := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, req.IDs).Delete(&HREmployee{})
	if res.Error != nil {
		hrFail(c, "批量删除失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}

func (h *HRHandler) ResignEmployees(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs        []string `json:"ids"`
		ResignDate string   `json:"resign_date"`
		Reason     string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.IDs) == 0 {
		hrFail(c, "ids 不能为空")
		return
	}
	date := normalizeDateValue(req.ResignDate)
	updates := map[string]any{"status": "resigned", "resign_date": date, "resign_reasons": req.Reason}
	res := h.db.WithContext(ctx).Model(&HREmployee{}).
		Where("tenant_id = ? AND id IN ?", tid, req.IDs).Updates(updates)
	if res.Error != nil {
		hrFail(c, "办理离职失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"updated": res.RowsAffected})
}

func (h *HRHandler) RestoreEmployees(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.IDs) == 0 {
		hrFail(c, "ids 不能为空")
		return
	}
	updates := map[string]any{"status": "active", "resign_date": "", "resign_reasons": ""}
	res := h.db.WithContext(ctx).Model(&HREmployee{}).
		Where("tenant_id = ? AND id IN ?", tid, req.IDs).Updates(updates)
	if res.Error != nil {
		hrFail(c, "恢复在职失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"updated": res.RowsAffected})
}

func (h *HRHandler) RosterStats(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var rows []struct {
		Status string
		Count  int64
	}
	if err := h.db.WithContext(ctx).Model(&HREmployee{}).
		Select("status, count(*) as count").Where("tenant_id = ?", tid).Group("status").Scan(&rows).Error; err != nil {
		hrFail(c, "统计失败: "+err.Error())
		return
	}
	data := gin.H{"total": int64(0), "active": int64(0), "resigned": int64(0)}
	for _, r := range rows {
		data[r.Status] = r.Count
		data["total"] = data["total"].(int64) + r.Count
	}
	hrOK(c, data)
}

// ImportRoster multipart(file, mode, default_status, force_transition) → 409 {conflicts}。
func (h *HRHandler) ImportRoster(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	fh, err := c.FormFile("file")
	if err != nil {
		hrFail(c, "缺少 file 参数")
		return
	}
	src, err := fh.Open()
	if err != nil {
		hrFail(c, "打开上传文件失败: "+err.Error())
		return
	}
	defer func() { _ = src.Close() }()
	rows, err := hrReadExcelRows(src)
	if err != nil {
		hrFail(c, "解析 Excel 失败: "+err.Error())
		return
	}
	var existing []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&existing).Error; err != nil {
		hrFail(c, "加载存量员工失败: "+err.Error())
		return
	}
	plan, err := hrPlanEmployeeImport(rows, existing, hrEmployeeImportOptions{
		Mode:               c.PostForm("mode"),
		DefaultStatus:      c.PostForm("default_status"),
		ForceDefaultStatus: hrTruthy(c.PostForm("force_transition")),
	})
	if err != nil {
		hrFail(c, err.Error())
		return
	}
	if len(plan.Conflicts) > 0 {
		hrConflict(c, plan.Conflicts)
		return
	}

	insertedCount := len(plan.Inserted)
	updatedCount := len(plan.Updated)
	entries := make([]HREmployee, 0, insertedCount+updatedCount)
	entries = append(entries, plan.Inserted...)
	entries = append(entries, plan.Updated...)
	for i := range entries {
		entries[i].TenantID = tid
		if entries[i].ID == "" {
			entries[i].ID = uuid.New().String()
		}
	}
	if len(entries) > 0 {
		txErr := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			conflict := clause.OnConflict{
				Columns: []clause.Column{{Name: "tenant_id"}, {Name: "id_number"}},
			}
			if plan.Mode == "insert" {
				conflict.DoNothing = true
			} else {
				conflict.DoUpdates = clause.AssignmentColumns(hrEmployeeUpdateColumns)
			}
			return tx.Clauses(conflict).Create(&entries).Error
		})
		if txErr != nil {
			hrFail(c, "导入员工失败: "+txErr.Error())
			return
		}
	}
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("name ASC, id_number ASC").Find(&employees).Error; err != nil {
		hrFail(c, "加载员工失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{
		"mode": plan.Mode, "inserted": insertedCount, "updated": updatedCount,
		"skipped": plan.Skipped, "employees": employees,
	})
}

var hrRosterExportHeaders = []string{
	"工号", "姓名", "部门", "岗位", "性别", "身份证号码", "入职日期", "年龄",
	"工龄", "出生月份", "联系电话", "在职状态", "离职日期", "备注",
}

// ExportRoster GET /roster/export?ids=（逗号分隔，空=全部）。
func (h *HRHandler) ExportRoster(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	q := h.db.WithContext(ctx).Model(&HREmployee{}).Where("tenant_id = ?", tid)
	var ids []string
	for _, part := range strings.Split(c.Query("ids"), ",") {
		if v := strings.TrimSpace(part); v != "" {
			ids = append(ids, v)
		}
	}
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	var items []HREmployee
	if err := q.Order("created_at DESC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "导出失败: "+err.Error())
		return
	}
	rows := make([][]string, 0, len(items))
	for _, e := range items {
		rows = append(rows, []string{
			e.EmployeeID, e.Name, e.Department, e.Position, e.Gender, e.IDNumber, e.HireDate,
			e.Age, e.WorkYears, e.BirthMonth, e.Phone, e.Status, e.ResignDate, e.Remarks,
		})
	}
	hrExcelDownload(c, "hr_roster.xlsx", hrRosterExportHeaders, rows)
}
