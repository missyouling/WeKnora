// 多单位组织机构（第 49 轮）。
//
// hr_company 在 tenant_id 上带唯一索引，只能存一个公司；本轮改为
// 「单位」列表（hr_org_units），单位可单独存在（无下属部门也合法，用于
// 劳动合同甲方等场景），部门通过 unit_id 归属。
package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// HROrgUnit 单位（公司）定义。
type HROrgUnit struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	TenantID    int64     `gorm:"index" json:"tenant_id"`
	Name        string    `gorm:"index" json:"name"`
	Code        string    `json:"code"`
	CreditCode  string    `json:"credit_code"`
	Address     string    `json:"address"`
	LegalPerson string    `json:"legal_person"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (HROrgUnit) TableName() string { return "hr_org_units" }

// 星达铜业的信用代码是真实值，作为默认预置；其余单位留空待填。
const hrDefaultCreditCode = "91500227MA5U54AJ0A"

// hrOrgUnitsAutoMigrate 建单位表并给部门补 unit_id（幂等）。
func hrOrgUnitsAutoMigrate(db *gorm.DB) {
	if err := db.AutoMigrate(&HROrgUnit{}); err != nil {
		return
	}
	// 部门表补 unit_id（AutoMigrate 会加列）
	_ = db.AutoMigrate(&HRDepartment{})
}

// hrOrgUnitWithDepts 单位 + 其下属部门（GET/PUT 的统一载体）。
type hrOrgUnitWithDepts struct {
	HROrgUnit
	Departments []HRDepartment `json:"departments"`
}

// hrCollectUnitNamesFromRoster 从花名册的合同公司 / 现合同签约公司去重提取单位名。
func (h *HRHandler) hrCollectUnitNamesFromRoster(ctx context.Context, tid int64) []string {
	var emps []HREmployee
	if err := h.db.WithContext(ctx).Select("custom_data").
		Where("tenant_id = ?", tid).Find(&emps).Error; err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, e := range emps {
		if e.CustomData == nil {
			continue
		}
		add(toStringValue(e.CustomData["合同签约公司"]))
		add(toStringValue(e.CustomData["合同公司"]))
	}
	sort.Strings(out)
	return out
}

// hrEnsureOrgUnits 首次访问时初始化单位（幂等）。
//
// 规则：优先用花名册提取到的单位名；一个都没有时退回旧的 hr_company 或默认公司。
// 已有部门统一挂到第一个单位下（保持第 48 轮数据不丢）。
func (h *HRHandler) hrEnsureOrgUnits(ctx context.Context, tid int64) {
	var unitCnt int64
	h.db.WithContext(ctx).Model(&HROrgUnit{}).Where("tenant_id = ?", tid).Count(&unitCnt)
	if unitCnt > 0 {
		return
	}
	names := h.hrCollectUnitNamesFromRoster(ctx, tid)
	if len(names) == 0 {
		var legacy HRCompany
		if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&legacy).Error; err == nil {
			if n := strings.TrimSpace(legacy.Name); n != "" {
				names = []string{n}
			}
		}
	}
	if len(names) == 0 {
		names = []string{"重庆星达铜业有限公司"}
	}
	units := make([]HROrgUnit, 0, len(names))
	for i, name := range names {
		code := ""
		if i == 0 {
			code = hrOrgCompanyDefaultCode
		} else {
			code = fmt.Sprintf("%02d", i+1)
		}
		credit := ""
		if strings.Contains(name, "星达") {
			credit = hrDefaultCreditCode
		}
		units = append(units, HROrgUnit{
			ID: uuid.New().String(), TenantID: tid, Name: name, Code: code,
			CreditCode: credit, SortOrder: i + 1,
		})
	}
	if err := h.db.WithContext(ctx).Create(&units).Error; err != nil {
		return
	}
	// 旧部门挂到第一个单位
	firstID := units[0].ID
	_ = h.db.WithContext(ctx).Model(&HRDepartment{}).
		Where("tenant_id = ? AND (unit_id IS NULL OR unit_id = '')", tid).
		Update("unit_id", firstID).Error
}

// hrOrgUnitsFor 读单位 + 部门（按 sort_order 排）。
func (h *HRHandler) hrOrgUnitsFor(ctx context.Context, tid int64) []hrOrgUnitWithDepts {
	var units []HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("sort_order ASC, code ASC").Find(&units).Error; err != nil {
		return nil
	}
	var depts []HRDepartment
	_ = h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("sort_order ASC, code ASC").Find(&depts).Error
	byUnit := map[string][]HRDepartment{}
	orphan := make([]HRDepartment, 0)
	for _, d := range depts {
		if strings.TrimSpace(d.UnitID) == "" {
			orphan = append(orphan, d)
			continue
		}
		byUnit[d.UnitID] = append(byUnit[d.UnitID], d)
	}
	out := make([]hrOrgUnitWithDepts, 0, len(units))
	for i, u := range units {
		list := byUnit[u.ID]
		if list == nil {
			list = []HRDepartment{}
		}
		// 首个单位兜底收纳历史无归属部门，避免数据「消失」
		if i == 0 && len(orphan) > 0 {
			list = append(list, orphan...)
		}
		out = append(out, hrOrgUnitWithDepts{HROrgUnit: u, Departments: list})
	}
	if len(out) == 0 && len(orphan) > 0 {
		out = append(out, hrOrgUnitWithDepts{
			HROrgUnit:   HROrgUnit{TenantID: tid, Name: "重庆星达铜业有限公司", Code: hrOrgCompanyDefaultCode},
			Departments: orphan,
		})
	}
	return out
}

// GetOrgUnits GET /org/units —— 多单位 + 各自部门。
func (h *HRHandler) GetOrgUnits(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	h.hrEnsureOrgDefaults(ctx, tid)
	h.hrEnsureOrgUnits(ctx, tid)
	h.hrMigrateOrgCodes(ctx, tid)
	units := h.hrOrgUnitsFor(ctx, tid)
	// 兼容旧前端：同时返回 company（首个单位）与拍平的 departments
	var company any
	flat := make([]HRDepartment, 0)
	if len(units) > 0 {
		company = units[0].HROrgUnit
		for _, u := range units {
			flat = append(flat, u.Departments...)
		}
	}
	hrOK(c, gin.H{"units": units, "company": company, "departments": flat})
}

// SaveOrgUnits PUT /org/units —— 整表替换保存（手存语义）。
func (h *HRHandler) SaveOrgUnits(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		Units []hrOrgUnitWithDepts `json:"units"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", tid).Delete(&HROrgUnit{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ?", tid).Delete(&HRDepartment{}).Error; err != nil {
			return err
		}
		for ui, u := range req.Units {
			name := strings.TrimSpace(u.Name)
			if name == "" {
				continue
			}
			uid := strings.TrimSpace(u.ID)
			if uid == "" {
				uid = uuid.New().String()
			}
			code := strings.TrimSpace(u.Code)
			if code == "" {
				code = fmt.Sprintf("%02d", ui+1)
			}
			unit := HROrgUnit{
				ID: uid, TenantID: tid, Name: name, Code: code,
				CreditCode:  strings.TrimSpace(u.CreditCode),
				Address:     strings.TrimSpace(u.Address),
				LegalPerson: strings.TrimSpace(u.LegalPerson),
				SortOrder:   ui + 1,
			}
			if err := tx.Create(&unit).Error; err != nil {
				return err
			}
			// 部门顺序 = 数组下标（拖拽排序的直接结果），不再用数字输入框
			rows := make([]HRDepartment, 0, len(u.Departments))
			for di, d := range u.Departments {
				dn := strings.TrimSpace(d.Name)
				if dn == "" {
					continue
				}
				did := strings.TrimSpace(d.ID)
				if did == "" {
					did = uuid.New().String()
				}
				dc := strings.TrimSpace(d.Code)
				if dc == "" {
					dc = fmt.Sprintf("%05d", (ui+1)*1000+di+1)
				}
				rows = append(rows, HRDepartment{
					ID: did, TenantID: tid, Name: dn, Code: dc,
					UnitID: uid, SortOrder: di + 1,
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

// hrUnitByName 按单位名精确匹配（合同甲方派生用）。
func (h *HRHandler) hrUnitByName(ctx context.Context, tid int64, name string) (HROrgUnit, bool) {
	n := strings.TrimSpace(name)
	if n == "" {
		return HROrgUnit{}, false
	}
	var u HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND name = ?", tid, n).First(&u).Error; err == nil {
		return u, true
	}
	// 兜底：包含匹配（花名册里可能带括号后缀）
	var list []HROrgUnit
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).Find(&list).Error; err == nil {
		for _, x := range list {
			if strings.Contains(n, x.Name) || strings.Contains(x.Name, n) {
				return x, true
			}
		}
	}
	return HROrgUnit{}, false
}
