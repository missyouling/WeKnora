// 花名册：CRUD、Excel 导入（merge/insert/update + 离职冲突 409 + force_transition）、
// 导出、离职/恢复/批量删除、stats。解析与冲突判定抽成纯函数 hrPlanEmployeeImport 便于 TDD。
package handler

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	// 第 46 轮：76 个标准模板表头（中文标准名 → 列号），与第 45 轮英文别名索引并存。
	rosterCols := map[string]int{}
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
		if std := hrRosterHeaderKey(cell); std != "" {
			rosterCols[std] = idx
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
		// 模板自带的示例行（工号 000001 / 示例员工）不算真实数据。
		if empNoIdx, ok := rosterCols["工号"]; ok {
			if strings.TrimSpace(getCell(row, empNoIdx)) == "000001" && strings.Contains(name, "示例员工") {
				skipped++
				continue
			}
		}
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

		// 第 46 轮：76 个标准模板字段按中文原名无损写入 custom_data。
		// 日期/时间/有效期类统一清洗成 YYYY-MM-DD（含 "2018-07-01 00:00:00"）。
		custom := entry.CustomData
		for _, std := range hrRosterHeaders {
			idx, ok := rosterCols[std]
			if !ok {
				continue
			}
			v := getCell(row, idx)
			if v != "" && hrIsRosterDateField(std) {
				v = normalizeHRDateValue(v)
			}
			custom[std] = v
		}
		// 模板字段覆盖主列（模板是权威数据源，避免第 45 轮英文别名漏映射）。
		hrStringOf := func(key string) string {
			if v, ok := custom[key]; ok {
				return strings.TrimSpace(fmt.Sprint(v))
			}
			return ""
		}
		if v := hrStringOf("工号"); v != "" {
			entry.EmployeeID = v
		}
		if v := hrStringOf("姓名"); v != "" {
			entry.Name = v
		}
		if v := hrStringOf("部门"); v != "" {
			entry.Department = v
		}
		if v := hrStringOf("岗位"); v != "" {
			entry.Position = v
		}
		if v := hrStringOf("职务"); v != "" {
			entry.Position = v
		}
		if v := hrStringOf("性别"); v != "" {
			entry.Gender = v
		}
		if v := hrStringOf("年龄"); v != "" {
			entry.Age = v
		}
		if v := hrStringOf("工龄"); v != "" {
			entry.WorkYears = v
		}
		if v := hrStringOf("入职日期"); v != "" {
			entry.HireDate = v
		}
		if v := hrStringOf("离职日期"); v != "" {
			entry.ResignDate = v
		}
		if v := hrStringOf("手机号码"); v != "" {
			entry.Phone = v
		}
		if v := hrStringOf("证件号码"); v != "" {
			entry.IDNumber = normalizeIDNumber(v)
		}
		if v := hrStringOf("个人社保账号"); v != "" {
			entry.SocialInsuranceNumber = v
		}
		if v := hrStringOf("个人公积金账号"); v != "" {
			entry.ProvidentFundNumber = v
		}
		if v := hrStringOf("紧急联系人姓名"); v != "" {
			entry.EmergencyContact = v
		}
		if v := hrStringOf("紧急联系人电话"); v != "" {
			entry.EmergencyPhone = v
		}
		if v := hrStringOf("最高学历"); v != "" {
			entry.Education = v
		}
		if v := hrStringOf("政治面貌"); v != "" {
			entry.PoliticalStatus = v
		}
		if v := hrStringOf("籍贯"); v != "" {
			entry.NativePlace = v
		}
		if v := hrStringOf("民族"); v != "" {
			entry.Ethnicity = v
		}
		if v := hrStringOf("户口性质"); v != "" {
			entry.HouseholdType = v
		}
		if v := hrStringOf("户籍地址"); v != "" {
			entry.IDAddress = v
		}
		if v := hrStringOf("个人邮箱"); v != "" {
			entry.Email = v
		}
		if v := hrStringOf("离职原因"); v != "" {
			entry.ResignReasons = v
		}
		// 四态状态归一：工作性质=退休返聘 优先；模板「员工状态」兼容正式/试用/已离职。
		workNature := hrStringOf("工作性质")
		rawStatus := hrStringOf("员工状态")
		if rawStatus == "" && entry.Status != "" {
			rawStatus = entry.Status
		}
		entry.Status = normalizeHREmployeeStatus(rawStatus, workNature)
		custom["员工状态"] = entry.Status
		custom["在职状态"] = entry.Status

		hrEnrichEmployee(&entry)
		// 派生字段也回写 custom_data，保证列表「年龄 / 工龄 / 出生日期」有值。
		if entry.Age != "" {
			custom["年龄"] = entry.Age
		}
		if entry.WorkYears != "" {
			custom["工龄"] = entry.WorkYears
		}
		if entry.BirthMonth != "" {
			custom["出生月份"] = entry.BirthMonth
		}
		entry.CustomData = custom

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
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" && status != "全部" {
		// 四态筛选需兼容第 45 轮存量英文值与模板导入值（正式/试用/已离职）。
		q = q.Where("status IN ?", hrStatusDBValues(status))
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
	raw, err := hrBindRosterPayload(c, &req)
	if err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	// 中文表头键（76 字段模板）必须在必填校验之前并入，否则「证件号码」会丢失。
	hrApplyRosterRaw(&req, raw, nil)
	req.ID = uuid.New().String()
	req.TenantID = tid
	req.IDNumber = normalizeIDNumber(req.IDNumber)
	req.Name = strings.TrimSpace(req.Name)
	if req.IDNumber == "" || req.Name == "" {
		hrFail(c, "姓名和身份证号码必填")
		return
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
	raw, err := hrBindRosterPayload(c, &req)
	if err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	// 合并存量 custom_data，避免局部保存把 76 字段洗掉。
	hrApplyRosterRaw(&req, raw, &existing)
	req.ID = existing.ID
	req.TenantID = existing.TenantID
	req.CreatedAt = existing.CreatedAt
	req.IDNumber = normalizeIDNumber(req.IDNumber)
	if req.IDNumber == "" {
		req.IDNumber = existing.IDNumber
	}
	if req.ResignProofPath == "" {
		req.ResignProofPath = existing.ResignProofPath
	}
	if req.ResignRemark == "" {
		req.ResignRemark = existing.ResignRemark
	}
	// 局部保存时把未提交的 string 主列从存量补回，避免 Save 覆盖成空值。
	hrFillEmployeeFrom(&req, &existing)
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
		ProofPath  string   `json:"proof_path"`
		Remark     string   `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	if len(req.IDs) == 0 {
		hrFail(c, "ids 不能为空")
		return
	}
	date := normalizeHRDateValue(req.ResignDate)
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	// custom_data 是 serializer:json 字段，必须走结构体 Save；
	// Updates(map) 不经过 serializer，PG 下会报 42804。
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, req.IDs).Find(&employees).Error; err != nil {
		hrFail(c, "办理离职失败: "+err.Error())
		return
	}
	for i := range employees {
		employees[i].Status = hrRosterStatusResigned
		employees[i].ResignDate = date
		employees[i].ResignReasons = req.Reason
		if req.ProofPath != "" {
			employees[i].ResignProofPath = req.ProofPath
		}
		if req.Remark != "" {
			employees[i].ResignRemark = req.Remark
		}
		employees[i].CustomData = hrMergeEmployeeCustomData(employees[i].CustomData, map[string]string{
			"员工状态": hrRosterStatusResigned,
			"在职状态": hrRosterStatusResigned,
			"离职日期": date,
			"离职原因": req.Reason,
			"离职证明": employees[i].ResignProofPath,
			"离职备注": employees[i].ResignRemark,
		})
	}
	for i := range employees {
		if err := h.db.WithContext(ctx).Save(&employees[i]).Error; err != nil {
			hrFail(c, "办理离职失败: "+err.Error())
			return
		}
	}
	hrOK(c, gin.H{"updated": len(employees)})
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
	var employees []HREmployee
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tid, req.IDs).Find(&employees).Error; err != nil {
		hrFail(c, "恢复在职失败: "+err.Error())
		return
	}
	for i := range employees {
		employees[i].Status = hrRosterStatusActive
		employees[i].ResignDate = ""
		employees[i].ResignReasons = ""
		employees[i].ResignProofPath = ""
		employees[i].ResignRemark = ""
		employees[i].CustomData = hrMergeEmployeeCustomData(employees[i].CustomData, map[string]string{
			"员工状态": hrRosterStatusActive,
			"在职状态": hrRosterStatusActive,
			"离职日期": "", "离职原因": "", "离职证明": "", "离职备注": "",
		})
	}
	for i := range employees {
		if err := h.db.WithContext(ctx).Save(&employees[i]).Error; err != nil {
			hrFail(c, "恢复在职失败: "+err.Error())
			return
		}
	}
	hrOK(c, gin.H{"updated": len(employees)})
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
