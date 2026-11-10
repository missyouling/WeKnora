// 花名册「设置 → 组织机构」：公司 / 部门 / 岗位（第 47 轮）。
//
// 数据来源与持久化口径：
//
//	· 首次打开时按已导入花名册**自动生成默认数据**（公司取自「合同公司」，
//	  部门/岗位取自员工记录去重），编码按出现顺序生成 D01/D02…、P01/P02…；
//	· 用户增删改后点「保存」才持久化（保持中台禁用自动保存的原则）；
//	· 存储复用 fleet_categories 之外的独立表，避免污染字段定义。
package handler

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HRCompany 公司设置（单条）。
type HRCompany struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	TenantID  int64     `gorm:"index;uniqueIndex:uk_hr_company_tenant" json:"tenant_id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HRCompany) TableName() string { return "hr_company" }

// HRDepartment 部门。
type HRDepartment struct {
	ID       string `gorm:"primaryKey" json:"id"`
	TenantID int64  `gorm:"index" json:"tenant_id"`
	// UnitID 所属单位（第 49 轮多单位）；空 = 历史数据，读取时归入首个单位。
	UnitID    string    `gorm:"index" json:"unit_id"`
	Name      string    `gorm:"index" json:"name"`
	Code      string    `json:"code"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (HRDepartment) TableName() string { return "hr_departments" }

// HRPosition 岗位（可挂部门）。
type HRPosition struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	TenantID     int64     `gorm:"index" json:"tenant_id"`
	Name         string    `gorm:"index" json:"name"`
	Code         string    `json:"code"`
	DepartmentID string    `gorm:"index" json:"department_id"`
	Department   string    `json:"department"`
	SortOrder    int       `json:"sort_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (HRPosition) TableName() string { return "hr_positions" }

// hrOrgAutoMigrate 建 3 张组织机构表（best-effort）。
func hrOrgAutoMigrate(db *gorm.DB) {
	hrOrgUnitsAutoMigrate(db)
	if err := db.AutoMigrate(&HRCompany{}, &HRDepartment{}, &HRPosition{}); err != nil {
		// 与其它 HR 表同口径：失败只告警，不阻断服务
		_ = err
	}
}

// hrOrgCompanyDefaultCode 公司默认编码（任务指定 02）。
const hrOrgCompanyDefaultCode = "02"

// hrOrgDefaultCode 生成默认编码：D01 / P01（两位数，超出则自然增长）。
func hrOrgDefaultCode(prefix string, index int) string {
	return fmt.Sprintf("%s%02d", prefix, index)
}

// hrDeptCodeMap 从存量工号反推「部门 → 5 位编码」。
//
// 工号结构是 [职级]+[5位部门码]+[4位序号]，因此同一部门的工号第 2~6 位一致；
// 取该部门出现次数最多的 5 位码作为其编码（兼容少量脏工号）。
func hrDeptCodeMap(rows []struct {
	Department string
	EmployeeID string
}) map[string]string {
	counts := map[string]map[string]int{}
	for _, r := range rows {
		dept := strings.TrimSpace(r.Department)
		if i := strings.LastIndex(dept, "/"); i >= 0 {
			dept = dept[i+1:]
		}
		if dept == "" {
			continue
		}
		code := hrDeptCodeFromEmployeeID(r.EmployeeID)
		if code == "" {
			continue
		}
		if counts[dept] == nil {
			counts[dept] = map[string]int{}
		}
		counts[dept][code]++
	}
	out := map[string]string{}
	for dept, m := range counts {
		best, bestN := "", 0
		for code, n := range m {
			if n > bestN || (n == bestN && code < best) {
				best, bestN = code, n
			}
		}
		if best != "" {
			out[dept] = best
		}
	}
	return out
}

// hrCollectOrgFromRoster 从花名册去重提取公司 / 部门 / 岗位。
func (h *HRHandler) hrCollectOrgFromRoster(ctx context.Context, tid int64) (string, []string, []string) {
	var emps []HREmployee
	if err := h.db.WithContext(ctx).
		Select("department, position, custom_data").
		Where("tenant_id = ?", tid).Find(&emps).Error; err != nil {
		return "", nil, nil
	}
	company := ""
	deptSet := map[string]bool{}
	posSet := map[string]bool{}
	depts := make([]string, 0)
	positions := make([]string, 0)
	addDept := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" || deptSet[d] {
			return
		}
		deptSet[d] = true
		depts = append(depts, d)
	}
	addPos := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || posSet[p] {
			return
		}
		posSet[p] = true
		positions = append(positions, p)
	}
	for _, e := range emps {
		// 部门可能是「研发中心/质量管理部/测试一组」，取最末级作为部门名
		d := e.Department
		if i := strings.LastIndex(d, "/"); i >= 0 {
			d = d[i+1:]
		}
		addDept(d)
		addPos(e.Position)
		if company == "" && e.CustomData != nil {
			if v := strings.TrimSpace(toStringValue(e.CustomData["合同公司"])); v != "" {
				company = v
			}
			if v := strings.TrimSpace(toStringValue(e.CustomData["合同签约公司"])); v != "" && company == "" {
				company = v
			}
			if d2 := strings.TrimSpace(toStringValue(e.CustomData["部门"])); d2 != "" {
				if i := strings.LastIndex(d2, "/"); i >= 0 {
					d2 = d2[i+1:]
				}
				addDept(d2)
			}
			if p2 := strings.TrimSpace(toStringValue(e.CustomData["岗位"])); p2 != "" {
				addPos(p2)
			}
		}
	}
	sort.Strings(depts)
	sort.Strings(positions)
	return company, depts, positions
}

// hrEnsureOrgDefaults 首次访问时按花名册生成默认组织机构（幂等）。
func (h *HRHandler) hrEnsureOrgDefaults(ctx context.Context, tid int64) {
	var cnt int64
	h.db.WithContext(ctx).Model(&HRCompany{}).Where("tenant_id = ?", tid).Count(&cnt)
	var deptCnt int64
	h.db.WithContext(ctx).Model(&HRDepartment{}).Where("tenant_id = ?", tid).Count(&deptCnt)
	if cnt > 0 && deptCnt > 0 {
		return
	}
	company, depts, _ := h.hrCollectOrgFromRoster(ctx, tid)
	if cnt == 0 {
		if company == "" {
			company = "重庆星达铜业有限公司"
		}
		_ = h.db.WithContext(ctx).Create(&HRCompany{
			ID: uuid.New().String(), TenantID: tid, Name: company,
			Code: hrOrgCompanyDefaultCode,
		}).Error
	}
	if deptCnt == 0 && len(depts) > 0 {
		// 部门编码优先从存量工号第 2~6 位提取（C020610012 → 02061），
		// 提取不到的部门按序补 02080/02090…
		var idRows []struct {
			Department string
			EmployeeID string
		}
		_ = h.db.WithContext(ctx).Model(&HREmployee{}).
			Select("department, employee_id").
			Where("tenant_id = ?", tid).Scan(&idRows).Error
		codes := hrDeptCodeMap(idRows)
		used := map[string]bool{}
		for _, c := range codes {
			used[c] = true
		}
		nextSeq := 80
		rows := make([]HRDepartment, 0, len(depts))
		for i, d := range depts {
			code := codes[d]
			if code == "" {
				for used[fmt.Sprintf("02%03d", nextSeq)] {
					nextSeq += 10
				}
				code = fmt.Sprintf("02%03d", nextSeq)
				used[code] = true
				nextSeq += 10
			}
			rows = append(rows, HRDepartment{
				ID: uuid.New().String(), TenantID: tid, Name: d,
				Code: code, SortOrder: i + 1,
			})
		}
		_ = h.db.WithContext(ctx).Create(&rows).Error
	}
}

// hrMigrateOrgCodes 把第 47 轮的占位编码迁移成正式编码（幂等）：
// 公司 C01 → 02；部门 D01/D02… → 从存量工号提取的 5 位部门码。
//
// 只迁移「形如 C0x / D0x」的占位值，用户手改过的编码不动。
func (h *HRHandler) hrMigrateOrgCodes(ctx context.Context, tid int64) {
	var company HRCompany
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&company).Error; err == nil {
		code := strings.TrimSpace(company.Code)
		if code == "" || regexp.MustCompile(`^C0\d+$`).MatchString(code) {
			company.Code = hrOrgCompanyDefaultCode
			_ = h.db.WithContext(ctx).Save(&company).Error
		}
	}
	var depts []HRDepartment
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&depts).Error; err != nil || len(depts) == 0 {
		return
	}
	var idRows []struct {
		Department string
		EmployeeID string
	}
	_ = h.db.WithContext(ctx).Model(&HREmployee{}).
		Select("department, employee_id").Where("tenant_id = ?", tid).Scan(&idRows).Error
	codes := hrDeptCodeMap(idRows)
	placeholder := regexp.MustCompile(`^D0\d+$`)
	// 已占用的 5 位码，避免新分配的撞车
	used := map[string]bool{}
	for _, c := range codes {
		used[c] = true
	}
	for i := range depts {
		if !placeholder.MatchString(strings.TrimSpace(depts[i].Code)) {
			continue
		}
		if code, ok := codes[strings.TrimSpace(depts[i].Name)]; ok && len(code) == 5 {
			depts[i].Code = code
			_ = h.db.WithContext(ctx).Save(&depts[i]).Error
			continue
		}
		// 存量里没有该部门的工号（如「质量部」）：按序补 02xxx
		seq := 80
		for used[fmt.Sprintf("02%03d", seq)] {
			seq += 10
		}
		code := fmt.Sprintf("02%03d", seq)
		used[code] = true
		depts[i].Code = code
		_ = h.db.WithContext(ctx).Save(&depts[i]).Error
	}
}

// GetOrganization GET /org —— 公司 + 部门（第 48 轮起不含岗位）。
func (h *HRHandler) GetOrganization(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	h.hrEnsureOrgDefaults(ctx, tid)
	h.hrMigrateOrgCodes(ctx, tid)

	var company HRCompany
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&company).Error; err != nil {
		company = HRCompany{TenantID: tid, Name: "重庆星达铜业有限公司", Code: hrOrgCompanyDefaultCode}
	}
	var depts []HRDepartment
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("sort_order ASC, code ASC").Find(&depts).Error
	hrOK(c, gin.H{"company": company, "departments": depts})
}

// SaveOrganization PUT /org —— 整表覆盖保存（手存语义）。
func (h *HRHandler) SaveOrganization(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		Company     HRCompany      `json:"company"`
		Departments []HRDepartment `json:"departments"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 公司：单条 upsert
		name := strings.TrimSpace(req.Company.Name)
		code := strings.TrimSpace(req.Company.Code)
		var company HRCompany
		if err := tx.Where("tenant_id = ?", tid).First(&company).Error; err == nil {
			company.Name, company.Code = name, code
			if err := tx.Save(&company).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Create(&HRCompany{
				ID: uuid.New().String(), TenantID: tid, Name: name, Code: code,
			}).Error; err != nil {
				return err
			}
		}
		// 部门：整表替换（保留前端给的 id，缺 id 则新建）
		if req.Departments != nil {
			if err := tx.Where("tenant_id = ?", tid).Delete(&HRDepartment{}).Error; err != nil {
				return err
			}
			rows := make([]HRDepartment, 0, len(req.Departments))
			for i, d := range req.Departments {
				n := strings.TrimSpace(d.Name)
				if n == "" {
					continue
				}
				id := strings.TrimSpace(d.ID)
				if id == "" {
					id = uuid.New().String()
				}
				sortOrder := d.SortOrder
				if sortOrder <= 0 {
					sortOrder = i + 1
				}
				code := strings.TrimSpace(d.Code)
				if code == "" {
					code = hrOrgDefaultCode("D", i+1)
				}
				rows = append(rows, HRDepartment{
					ID: id, TenantID: tid, Name: n, Code: code, SortOrder: sortOrder,
				})
			}
			if len(rows) > 0 {
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		hrFail(c, "保存组织机构失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"saved": true})
}

// ListContracts GET /contracts —— 劳动合同视图（数据源=花名册合同字段）。
func (h *HRHandler) ListContracts(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	pageSize := hrRosterMaxPageSize
	q := h.db.WithContext(ctx).Model(&HREmployee{}).Where("tenant_id = ?", tid)
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		// 第 49 轮：与花名册同口径的全字段深度搜索（含 custom_data）
		like := "%" + kw + "%"
		q = q.Where(
			"name ILIKE ? OR id_number ILIKE ? OR employee_id ILIKE ? OR department ILIKE ?"+
				" OR position ILIKE ? OR phone ILIKE ? OR COALESCE(custom_data::text, '') ILIKE ?",
			like, like, like, like, like, like, like,
		)
	}
	if dept := strings.TrimSpace(c.Query("department")); dept != "" && dept != "全部" && dept != "all" {
		q = q.Where("department = ?", dept)
	}
	var items []HREmployee
	if err := q.Order("employee_id ASC").Limit(pageSize).Find(&items).Error; err != nil {
		hrFail(c, "查询劳动合同失败: "+err.Error())
		return
	}
	// 只保留有合同信息的人
	filtered := make([]HREmployee, 0, len(items))
	for _, e := range items {
		if e.CustomData == nil {
			continue
		}
		end := strings.TrimSpace(toStringValue(e.CustomData["现合同结束日期"]))
		ctype := strings.TrimSpace(toStringValue(e.CustomData["现合同类型"]))
		if end == "" && ctype == "" {
			continue
		}
		filtered = append(filtered, e)
	}
	hrSortEmployeesByEmployeeID(filtered)
	hrAttachEmployeeDerived(ctx, h.db, tid, filtered)
	// 第 49 轮：甲方（单位信用代码/住所/法人）+ 乙方（联系电话）派生
	hrAttachContractParty(h.hrOrgUnitsFor(ctx, tid), filtered)

	// 合同状态筛选（全部 / 正常 / 即将到期 / 到期）
	if st := strings.TrimSpace(c.Query("contract_status")); st != "" && st != "全部" && st != "all" {
		keep := make([]HREmployee, 0, len(filtered))
		for _, e := range filtered {
			if toStringValue(e.CustomData["合同状态"]) == st {
				keep = append(keep, e)
			}
		}
		filtered = keep
	}
	hrOK(c, gin.H{"items": filtered, "total": len(filtered)})
}
