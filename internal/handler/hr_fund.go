// 公积金：单位设置、台账 CRUD（total=personal+company，比例默认 6）、封存/启封、
// Excel 导入导出、月度账单（只汇总 active、overwrite、倒序）、账单明细/删除。
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// hrNormalizePFRecord 金额口径：total = personal + company；比例默认 6；
// 仅给基数+比例时按 base*ratio/100 自动派生个人/单位金额（两边同额）。
func hrNormalizePFRecord(r *HRPFRecord, defaultRatio float64) {
	if defaultRatio <= 0 {
		defaultRatio = hrDefaultPFRatio
	}
	if r.ContributionRatio <= 0 {
		r.ContributionRatio = defaultRatio
	}
	if r.PersonalAmount == 0 && r.CompanyAmount == 0 && r.PersonalBase > 0 {
		amt := round2(r.PersonalBase * r.ContributionRatio / 100)
		r.PersonalAmount = amt
		r.CompanyAmount = amt
	}
	r.TotalAmount = round2(r.PersonalAmount + r.CompanyAmount)
	if r.Status == "" {
		r.Status = "active"
	}
}

// hrPFRatio 取租户默认比例，未配置回落 6。
func (h *HRHandler) hrPFRatio(ctx context.Context, tid int64) float64 {
	var s HRPFSettings
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&s).Error; err == nil && s.DefaultRatio > 0 {
		return s.DefaultRatio
	}
	return hrDefaultPFRatio
}

// hrBillWithItems 账单 + 明细快照（避免 GORM 关联/外键约束，明细独立查）。
type hrBillWithItems struct {
	HRPFBill
	Items []HRPFBillItem `json:"items"`
}

// ---------------- 单位设置 ----------------

func (h *HRHandler) GetPFSettings(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var s HRPFSettings
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&s).Error; err != nil {
		hrOK(c, gin.H{
			"unit_name": hrDefaultPFUnitName, "unit_account": hrDefaultPFUnitAccount,
			"default_ratio": hrDefaultPFRatio,
		})
		return
	}
	hrOK(c, s)
}

func (h *HRHandler) UpdatePFSettings(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		UnitName     string  `json:"unit_name"`
		UnitAccount  string  `json:"unit_account"`
		DefaultRatio float64 `json:"default_ratio"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.UnitName = strings.TrimSpace(req.UnitName)
	req.UnitAccount = strings.TrimSpace(req.UnitAccount)
	if req.UnitName == "" || req.UnitAccount == "" {
		hrFail(c, "unit_name 和 unit_account 必填")
		return
	}
	if req.DefaultRatio <= 0 {
		req.DefaultRatio = hrDefaultPFRatio
	}
	var s HRPFSettings
	err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).First(&s).Error
	if err != nil {
		s = HRPFSettings{
			ID: uuid.New().String(), TenantID: tid,
			UnitName: req.UnitName, UnitAccount: req.UnitAccount, DefaultRatio: req.DefaultRatio,
		}
		if cerr := h.db.WithContext(ctx).Create(&s).Error; cerr != nil {
			hrFail(c, "保存单位设置失败: "+cerr.Error())
			return
		}
		hrOK(c, s)
		return
	}
	if err := h.db.WithContext(ctx).Model(&s).Updates(map[string]any{
		"unit_name": req.UnitName, "unit_account": req.UnitAccount, "default_ratio": req.DefaultRatio,
	}).Error; err != nil {
		hrFail(c, "更新单位设置失败: "+err.Error())
		return
	}
	s.UnitName = req.UnitName
	s.UnitAccount = req.UnitAccount
	s.DefaultRatio = req.DefaultRatio
	hrOK(c, s)
}

// ---------------- 台账 ----------------

func (h *HRHandler) ListPFRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	q := h.db.WithContext(ctx).Model(&HRPFRecord{}).Where("tenant_id = ?", tid)
	if status := strings.TrimSpace(c.Query("status")); status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("name LIKE ? OR personal_account LIKE ? OR identity_number LIKE ?", like, like, like)
	}
	var items []HRPFRecord
	if err := q.Order("created_at DESC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询公积金台账失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"items": items, "total": len(items)})
}

func (h *HRHandler) CreatePFRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req HRPFRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	req.PersonalAccount = strings.TrimSpace(req.PersonalAccount)
	req.IdentityNumber = normalizeIDNumber(req.IdentityNumber)
	if req.PersonalAccount == "" || req.IdentityNumber == "" {
		hrFail(c, "personal_account 和 identity_number 必填")
		return
	}
	var cnt int64
	h.db.WithContext(ctx).Model(&HRPFRecord{}).
		Where("tenant_id = ? AND identity_number = ?", tid, req.IdentityNumber).Count(&cnt)
	if cnt > 0 {
		hrFail(c, "该身份证号码的公积金记录已存在")
		return
	}
	req.ID = uuid.New().String()
	req.TenantID = tid
	hrNormalizePFRecord(&req, h.hrPFRatio(ctx, tid))
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		hrFail(c, "创建公积金记录失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) UpdatePFRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var existing HRPFRecord
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&existing).Error; err != nil {
		hrFail(c, "公积金记录不存在")
		return
	}
	var req HRPFRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		hrFail(c, "invalid request body: "+err.Error())
		return
	}
	identity := normalizeIDNumber(req.IdentityNumber)
	if identity == "" {
		identity = existing.IdentityNumber
	}
	if identity != existing.IdentityNumber {
		var cnt int64
		h.db.WithContext(ctx).Model(&HRPFRecord{}).
			Where("tenant_id = ? AND identity_number = ?", tid, identity).Count(&cnt)
		if cnt > 0 {
			hrFail(c, "该身份证号码的公积金记录已存在")
			return
		}
	}
	req.ID = existing.ID
	req.TenantID = existing.TenantID
	req.CreatedAt = existing.CreatedAt
	req.IdentityNumber = identity
	if req.SealedAt == nil {
		req.SealedAt = existing.SealedAt
	}
	if req.UnsealedAt == nil {
		req.UnsealedAt = existing.UnsealedAt
	}
	if req.Status == "" {
		req.Status = existing.Status // 不因 PUT 缺省 status 意外解封
	}
	hrNormalizePFRecord(&req, h.hrPFRatio(ctx, tid))
	if err := h.db.WithContext(ctx).Save(&req).Error; err != nil {
		hrFail(c, "更新公积金记录失败: "+err.Error())
		return
	}
	hrOK(c, req)
}

func (h *HRHandler) DeletePFRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	res := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).Delete(&HRPFRecord{})
	if res.Error != nil {
		hrFail(c, "删除公积金记录失败: "+res.Error.Error())
		return
	}
	hrOK(c, gin.H{"deleted": res.RowsAffected})
}

func (h *HRHandler) SealPFRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var rec HRPFRecord
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&rec).Error; err != nil {
		hrFail(c, "公积金记录不存在")
		return
	}
	if rec.Status == "sealed" {
		hrFail(c, "记录已封存")
		return
	}
	var payload struct {
		Date string `json:"date"`
	}
	_ = c.ShouldBindJSON(&payload)
	sealedAt := time.Now()
	if d := strings.TrimSpace(payload.Date); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			hrFail(c, "封存日期格式应为 YYYY-MM-DD")
			return
		}
		sealedAt = parsed
	}
	if err := h.db.WithContext(ctx).Model(&rec).Updates(map[string]any{
		"status": "sealed", "sealed_at": &sealedAt, "unsealed_at": nil,
	}).Error; err != nil {
		hrFail(c, "封存失败: "+err.Error())
		return
	}
	rec.Status = "sealed"
	rec.SealedAt = &sealedAt
	rec.UnsealedAt = nil
	hrOK(c, rec)
}

func (h *HRHandler) UnsealPFRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var rec HRPFRecord
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&rec).Error; err != nil {
		hrFail(c, "公积金记录不存在")
		return
	}
	if rec.Status != "sealed" {
		hrFail(c, "记录未封存")
		return
	}
	var payload struct {
		Date string `json:"date"`
	}
	_ = c.ShouldBindJSON(&payload)
	unsealedAt := time.Now()
	if d := strings.TrimSpace(payload.Date); d != "" {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			hrFail(c, "启封日期格式应为 YYYY-MM-DD")
			return
		}
		unsealedAt = parsed
	}
	if err := h.db.WithContext(ctx).Model(&rec).Updates(map[string]any{
		"status": "active", "sealed_at": nil, "unsealed_at": &unsealedAt,
	}).Error; err != nil {
		hrFail(c, "启封失败: "+err.Error())
		return
	}
	rec.Status = "active"
	rec.SealedAt = nil
	rec.UnsealedAt = &unsealedAt
	hrOK(c, rec)
}

// 公积金导入/导出中文表头。
var hrPFHeaderMap = map[string]string{
	"个人账号":  "personal_account",
	"个人帐号":  "personal_account",
	"姓名":    "name",
	"身份证号码": "identity_number",
	"身份证号":  "identity_number",
	"部门":    "department",
	"部门名称":  "department",
	"缴存基数":  "personal_base",
	"比例":    "contribution_ratio",
	"缴存比例":  "contribution_ratio",
	"比例(%)": "contribution_ratio",
	"个人金额":  "personal_amount",
	"单位金额":  "company_amount",
	"合计":    "total_amount",
	"状态":    "status",
	"缴存状态":  "status",
	"备注":    "notes",
}

var hrPFExportHeaders = []string{
	"个人账号", "姓名", "身份证号码", "部门", "缴存基数", "缴存比例", "个人金额", "单位金额", "合计", "状态", "备注",
}

func (h *HRHandler) ImportPFRecords(c *gin.Context) {
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
	if len(rows) < 2 {
		hrFail(c, "Excel 中没有数据行")
		return
	}
	normalized := make(map[string]string, len(hrPFHeaderMap))
	for k, v := range hrPFHeaderMap {
		normalized[normalizeEmployeeHeader(k)] = v
	}
	indexMap := map[string]int{}
	for idx, cell := range rows[0] {
		if field, ok := normalized[normalizeEmployeeHeader(cell)]; ok {
			indexMap[field] = idx
		}
	}
	ratio := h.hrPFRatio(ctx, tid)
	inserted, updated := 0, 0
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		get := func(key string) string { return hrCell(row, indexMap, key) }
		rec := HRPFRecord{
			PersonalAccount:   strings.TrimSpace(get("personal_account")),
			Name:              strings.TrimSpace(get("name")),
			IdentityNumber:    normalizeIDNumber(get("identity_number")),
			Department:        strings.TrimSpace(get("department")),
			PersonalBase:      toFloat(get("personal_base")),
			ContributionRatio: toFloat(get("contribution_ratio")),
			PersonalAmount:    toFloat(get("personal_amount")),
			CompanyAmount:     toFloat(get("company_amount")),
			Notes:             strings.TrimSpace(get("notes")),
			Status:            normalizeEmployeeStatus(get("status")),
		}
		if rec.IdentityNumber == "" || rec.PersonalAccount == "" {
			continue
		}
		hrNormalizePFRecord(&rec, ratio)
		var existing HRPFRecord
		findErr := h.db.WithContext(ctx).
			Where("tenant_id = ? AND identity_number = ?", tid, rec.IdentityNumber).First(&existing).Error
		if findErr == nil {
			rec.ID = existing.ID
			rec.TenantID = tid
			rec.CreatedAt = existing.CreatedAt
			rec.SealedAt = existing.SealedAt
			rec.UnsealedAt = existing.UnsealedAt
			if err := h.db.WithContext(ctx).Save(&rec).Error; err != nil {
				hrFail(c, "导入公积金记录失败: "+err.Error())
				return
			}
			updated++
		} else {
			rec.ID = uuid.New().String()
			rec.TenantID = tid
			if err := h.db.WithContext(ctx).Create(&rec).Error; err != nil {
				hrFail(c, "导入公积金记录失败: "+err.Error())
				return
			}
			inserted++
		}
	}
	hrOK(c, gin.H{"inserted": inserted, "updated": updated})
}

func (h *HRHandler) ExportPFRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var items []HRPFRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("created_at DESC, id ASC").Find(&items).Error; err != nil {
		hrFail(c, "导出公积金台账失败: "+err.Error())
		return
	}
	rows := make([][]string, 0, len(items))
	for _, r := range items {
		rows = append(rows, []string{
			r.PersonalAccount, r.Name, r.IdentityNumber, r.Department, hrFloatStr(r.PersonalBase),
			hrFloatStr(r.ContributionRatio), hrFloatStr(r.PersonalAmount), hrFloatStr(r.CompanyAmount),
			hrFloatStr(r.TotalAmount), r.Status, r.Notes,
		})
	}
	hrExcelDownload(c, "hr_pf_records.xlsx", hrPFExportHeaders, rows)
}

// ---------------- 月度账单 ----------------

// ListPFBills GET 严格 ORDER BY month_label DESC；with_items=true 附带明细。
func (h *HRHandler) ListPFBills(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bills []HRPFBill
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tid).
		Order("month_label DESC").Find(&bills).Error; err != nil {
		hrFail(c, "查询账单失败: "+err.Error())
		return
	}
	if hrTruthy(c.Query("with_items")) {
		out := make([]hrBillWithItems, 0, len(bills))
		for _, b := range bills {
			var sub []HRPFBillItem
			if err := h.db.WithContext(ctx).Where("tenant_id = ? AND bill_id = ?", tid, b.ID).
				Order("id ASC").Find(&sub).Error; err != nil {
				hrFail(c, "查询账单明细失败: "+err.Error())
				return
			}
			out = append(out, hrBillWithItems{HRPFBill: b, Items: sub})
		}
		hrOK(c, gin.H{"items": out, "total": len(out)})
		return
	}
	hrOK(c, gin.H{"items": bills, "total": len(bills)})
}

// CreatePFBill month_label YYYY-MM，只汇总 status=active，支持 overwrite。
func (h *HRHandler) CreatePFBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var req struct {
		Month      string `json:"month"`
		MonthLabel string `json:"month_label"`
		Overwrite  bool   `json:"overwrite"`
	}
	_ = c.ShouldBindJSON(&req)
	// 前端与真机脚本统一发 month_label（YYYY-MM）；month 是源项目历史别名，
	// 两者都收，避免「按钮能点、接口 400」。
	month := strings.TrimSpace(req.MonthLabel)
	if month == "" {
		month = strings.TrimSpace(req.Month)
	}
	if month == "" {
		month = c.Query("month_label")
	}
	if month == "" {
		month = c.Query("month")
	}
	monthLabel, err := hrNormalizeMonthLabel(month)
	if err != nil {
		hrFail(c, "month 格式应为 YYYY-MM")
		return
	}
	var existing HRPFBill
	findErr := h.db.WithContext(ctx).
		Where("tenant_id = ? AND month_label = ?", tid, monthLabel).First(&existing).Error
	if findErr == nil {
		if !req.Overwrite {
			hrFail(c, "该月账单已存在，可传 overwrite=true 覆盖")
			return
		}
		if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("tenant_id = ? AND bill_id = ?", tid, existing.ID).Delete(&HRPFBillItem{}).Error; err != nil {
				return err
			}
			return tx.Where("id = ? AND tenant_id = ?", existing.ID, tid).Delete(&HRPFBill{}).Error
		}); err != nil {
			hrFail(c, "覆盖旧账单失败: "+err.Error())
			return
		}
	}
	// findErr != nil（含 gorm.ErrRecordNotFound 及被包装的变体）一律视为「本月尚无账单」，
	// 不依赖错误对象的相等比较；后续 Create 若真失败会被下面的事务捕获。

	var records []HRPFRecord
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND status = ?", tid, "active").
		Order("name ASC").Find(&records).Error; err != nil {
		hrFail(c, "加载缴存记录失败: "+err.Error())
		return
	}
	if len(records) == 0 {
		hrFail(c, "无在职（active）缴存记录可生成账单")
		return
	}
	bill := HRPFBill{
		ID: uuid.New().String(), TenantID: tid, MonthLabel: monthLabel, RecordCount: len(records),
	}
	items := make([]HRPFBillItem, 0, len(records))
	for _, r := range records {
		bill.PersonalAmountTotal += r.PersonalAmount
		bill.CompanyAmountTotal += r.CompanyAmount
		bill.CombinedAmountTotal += r.TotalAmount
		items = append(items, HRPFBillItem{
			ID: uuid.New().String(), TenantID: tid, BillID: bill.ID, RecordID: r.ID,
			PersonalAccount: r.PersonalAccount, Name: r.Name, IdentityNumber: r.IdentityNumber,
			PersonalAmount: r.PersonalAmount, CompanyAmount: r.CompanyAmount, TotalAmount: r.TotalAmount,
		})
	}
	bill.PersonalAmountTotal = round2(bill.PersonalAmountTotal)
	bill.CompanyAmountTotal = round2(bill.CompanyAmountTotal)
	bill.CombinedAmountTotal = round2(bill.CombinedAmountTotal)
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&bill).Error; err != nil {
			return err
		}
		if len(items) > 0 {
			return tx.Create(&items).Error
		}
		return nil
	}); err != nil {
		hrFail(c, "生成账单失败: "+err.Error())
		return
	}
	hrOK(c, hrBillWithItems{HRPFBill: bill, Items: items})
}

func (h *HRHandler) GetPFBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bill HRPFBill
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	var items []HRPFBillItem
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND bill_id = ?", tid, bill.ID).
		Order("id ASC").Find(&items).Error; err != nil {
		hrFail(c, "查询账单明细失败: "+err.Error())
		return
	}
	hrOK(c, hrBillWithItems{HRPFBill: bill, Items: items})
}

func (h *HRHandler) DeletePFBill(c *gin.Context) {
	ctx := c.Request.Context()
	tid := hrTenantID(c)
	var bill HRPFBill
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", c.Param("id"), tid).First(&bill).Error; err != nil {
		hrFail(c, "账单不存在")
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND bill_id = ?", tid, bill.ID).Delete(&HRPFBillItem{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND tenant_id = ?", bill.ID, tid).Delete(&HRPFBill{}).Error
	})
	if err != nil {
		hrFail(c, "删除账单失败: "+err.Error())
		return
	}
	hrOK(c, gin.H{"deleted": true})
}
