package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BillingHandler 租户费用核算：电费按市电账单子项比例分摊 + 宿舍定额 + 水费按表计。
type BillingHandler struct {
	db *gorm.DB
}

// NewBillingHandler creates a new BillingHandler.
func NewBillingHandler(db *gorm.DB) *BillingHandler {
	return &BillingHandler{db: db}
}

func billingTenantID(c *gin.Context) (uint64, error) {
	return types.MustTenantIDFromContext(c.Request.Context()), nil
}

// ---------------------------------------------------------------------------
// 租户 CRUD
// ---------------------------------------------------------------------------

// ListBillingTenants godoc
// @Summary      租户列表
// @Router       /billing/tenants [get]
func (h *BillingHandler) ListBillingTenants(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	var tenants []types.BillingTenant
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("created_at ASC").Find(&tenants).Error; err != nil {
		c.Error(errors.NewInternalServerError("list tenants failed"))
		return
	}
	settings := make(map[string]types.BillingTenantSetting, len(tenants))
	if len(tenants) > 0 {
		ids := make([]string, 0, len(tenants))
		for _, t := range tenants {
			ids = append(ids, t.ID)
		}
		var sts []types.BillingTenantSetting
		_ = h.db.WithContext(ctx).Where("billing_tenant_id IN ?", ids).Find(&sts).Error
		for _, s := range sts {
			settings[s.BillingTenantID] = s
		}
	}
	type row struct {
		types.BillingTenant
		Setting types.BillingTenantSetting `json:"setting"`
	}
	out := make([]row, 0, len(tenants))
	for _, t := range tenants {
		s, _ := settings[t.ID]
		out = append(out, row{BillingTenant: t, Setting: s})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": out})
}

// GetBillingTenant godoc
// @Summary      租户详情(含设置/子项/表计引用/分时电表)
// @Router       /billing/tenants/:id [get]
func (h *BillingHandler) GetBillingTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	id := c.Param("id")
	var t types.BillingTenant
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).First(&t).Error; err != nil {
		c.Error(errors.NewNotFoundError("租户不存在"))
		return
	}
	var st types.BillingTenantSetting
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).First(&st).Error
	var items []types.BillingTenantItem
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).Order("sort ASC, created_at ASC").Find(&items).Error
	var refs []types.BillingTenantMeterRef
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).Order("category ASC, created_at ASC").Find(&refs).Error
	var meters []types.BillingTimeMeter
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL", id).Order("meter_type ASC, created_at ASC").Find(&meters).Error
	var waterMeters []types.BillingWaterMeter
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL", id).Order("created_at ASC").Find(&waterMeters).Error
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tenant":       t,
			"setting":      st,
			"items":        items,
			"refs":         refs,
			"meters":       meters,
			"water_meters": waterMeters,
		},
	})
}

// CreateBillingTenant godoc
// @Summary      创建租户
// @Router       /billing/tenants [post]
func (h *BillingHandler) CreateBillingTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	var req struct {
		Name           string  `json:"name"`
		TenantNo       string  `json:"tenant_no"`
		AllocationMode string  `json:"allocation_mode"`
		LeaseStart     *string `json:"lease_start"`
		LeaseYears     int     `json:"lease_years"`
		Contact        string  `json:"contact"`
		Phone          string  `json:"phone"`
		Remark         string  `json:"remark"`
		BillKBID       string  `json:"bill_kb_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("name is required"))
		return
	}
	if strings.TrimSpace(req.AllocationMode) == "" {
		req.AllocationMode = "按比例分摊"
	}
	kbID := strings.TrimSpace(req.BillKBID)
	if kbID == "" {
		kbID = h.defaultBillKBID(ctx)
	}
	t := types.BillingTenant{
		ID: uuid.NewString(), TenantID: int64(tenantID), Name: req.Name,
		TenantNo: req.TenantNo, AllocationMode: req.AllocationMode,
		LeaseStart: req.LeaseStart, LeaseYears: req.LeaseYears,
		Contact: req.Contact, Phone: req.Phone, Remark: req.Remark,
		CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	st := types.BillingTenantSetting{
		ID: uuid.NewString(), BillingTenantID: t.ID,
		DormPrice: 1, WaterPrice: 5.22, BillKBID: kbID,
		CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&t).Error; err != nil {
			return err
		}
		return tx.Create(&st).Error
	}); err != nil {
		logger.Errorf(ctx, "create billing tenant failed: %v", err)
		c.Error(errors.NewInternalServerError("create tenant failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// defaultBillKBID 市电账单知识库默认:名称含"市电"优先,其次"电费",最后取第一个。
func (h *BillingHandler) defaultBillKBID(ctx context.Context) string {
	type kb struct {
		ID   string
		Name string
	}
	var list []kb
	if err := h.db.WithContext(ctx).Model(&types.KnowledgeBase{}).
		Where("deleted_at IS NULL").Order("created_at ASC").Limit(200).Find(&list).Error; err != nil {
		return ""
	}
	fallback := ""
	for _, k := range list {
		if fallback == "" {
			fallback = k.ID
		}
		if strings.Contains(k.Name, "市电") {
			return k.ID
		}
	}
	for _, k := range list {
		if strings.Contains(k.Name, "电费") {
			return k.ID
		}
	}
	return fallback
}

// UpdateBillingTenant godoc
// @Summary      更新租户与核算参数
// @Router       /billing/tenants/:id [put]
func (h *BillingHandler) UpdateBillingTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	id := c.Param("id")
	var req struct {
		Name           string  `json:"name"`
		TenantNo       string  `json:"tenant_no"`
		AllocationMode string  `json:"allocation_mode"`
		LeaseStart     *string `json:"lease_start"`
		LeaseYears     int     `json:"lease_years"`
		Contact        string  `json:"contact"`
		Phone          string  `json:"phone"`
		Remark         string  `json:"remark"`
		BillKBID       string  `json:"bill_kb_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	var t types.BillingTenant
	if err := h.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).First(&t).Error; err != nil {
		c.Error(errors.NewNotFoundError("租户不存在"))
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		t.Name = strings.TrimSpace(req.Name)
	}
	t.TenantNo = req.TenantNo
	if strings.TrimSpace(req.AllocationMode) != "" {
		t.AllocationMode = req.AllocationMode
	}
	t.LeaseStart = req.LeaseStart
	t.LeaseYears = req.LeaseYears
	t.Contact = req.Contact
	t.Phone = req.Phone
	t.Remark = req.Remark
	t.UpdatedAt = timeNowUTC()
	var st types.BillingTenantSetting
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).First(&st).Error; err != nil {
		st = types.BillingTenantSetting{
			ID: uuid.NewString(), BillingTenantID: id, CreatedAt: timeNowUTC(),
		}
	}
	if strings.TrimSpace(req.BillKBID) != "" {
		st.BillKBID = req.BillKBID
	} else if strings.TrimSpace(st.BillKBID) == "" {
		st.BillKBID = h.defaultBillKBID(ctx)
	}
	st.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&t).Error; err != nil {
			return err
		}
		return tx.Save(&st).Error
	}); err != nil {
		logger.Errorf(ctx, "update billing tenant failed: %v", err)
		c.Error(errors.NewInternalServerError("update tenant failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": t})
}

// DeleteBillingTenant godoc
// @Summary      删除租户
// @Router       /billing/tenants/:id [delete]
func (h *BillingHandler) DeleteBillingTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	id := c.Param("id")
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.BillingTenant{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Update("deleted_at", now).Error; err != nil {
		c.Error(errors.NewInternalServerError("delete tenant failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 分时电表 CRUD
// ---------------------------------------------------------------------------

// ListBillingTimeMeters godoc
// @Summary      分时电表列表
// @Router       /billing/tenants/:id/meters [get]
func (h *BillingHandler) ListBillingTimeMeters(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var meters []types.BillingTimeMeter
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL", id).
		Order("meter_type ASC, created_at ASC").Find(&meters).Error; err != nil {
		c.Error(errors.NewInternalServerError("list time meters failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": meters})
}

// CreateBillingTimeMeter godoc
// @Summary      新增分时电表
// @Router       /billing/tenants/:id/meters [post]
func (h *BillingHandler) CreateBillingTimeMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Name        string  `json:"name"`
		MeterNo     string  `json:"meter_no"`
		MeterKind   string  `json:"meter_kind"`
		OwnerUnit   string  `json:"owner_unit"`
		UseUnit     string  `json:"use_unit"`
		Manager     string  `json:"manager"`
		Contact     string  `json:"contact"`
		MeterMode   string  `json:"meter_mode"`
		InstallDate *string `json:"install_date"`
		Remark      string  `json:"remark"`
		Rate        float64 `json:"rate"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("name is required"))
		return
	}
	if req.MeterKind != "normal" {
		req.MeterKind = "time"
	}
	if req.MeterMode != "auto" {
		req.MeterMode = "manual"
	}
	m := types.BillingTimeMeter{
		ID: uuid.NewString(), BillingTenantID: id,
		Name: req.Name, MeterNo: req.MeterNo, MeterKind: req.MeterKind,
		OwnerUnit: req.OwnerUnit, UseUnit: req.UseUnit,
		Manager: req.Manager, Contact: req.Contact, MeterMode: req.MeterMode,
		InstallDate: req.InstallDate, Remark: req.Remark,
		Rate: req.Rate, Enabled: true,
		CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if m.Rate <= 0 {
		m.Rate = 1
	}
	if err := h.db.WithContext(ctx).Create(&m).Error; err != nil {
		c.Error(errors.NewInternalServerError("create time meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}

// UpdateBillingTimeMeter godoc
// @Summary      更新分时电表
// @Router       /billing/meters/:id [put]
func (h *BillingHandler) UpdateBillingTimeMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Name        string  `json:"name"`
		MeterNo     string  `json:"meter_no"`
		MeterKind   string  `json:"meter_kind"`
		OwnerUnit   string  `json:"owner_unit"`
		UseUnit     string  `json:"use_unit"`
		Manager     string  `json:"manager"`
		Contact     string  `json:"contact"`
		MeterMode   string  `json:"meter_mode"`
		InstallDate *string `json:"install_date"`
		Remark      string  `json:"remark"`
		Rate        float64 `json:"rate"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	var m types.BillingTimeMeter
	if err := h.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&m).Error; err != nil {
		c.Error(errors.NewNotFoundError("分时电表不存在"))
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		m.Name = strings.TrimSpace(req.Name)
	}
	m.MeterNo = req.MeterNo
	if req.MeterKind == "time" || req.MeterKind == "normal" {
		m.MeterKind = req.MeterKind
	}
	m.OwnerUnit = req.OwnerUnit
	m.UseUnit = req.UseUnit
	m.Manager = req.Manager
	m.Contact = req.Contact
	if req.MeterMode == "auto" || req.MeterMode == "manual" {
		m.MeterMode = req.MeterMode
	}
	m.InstallDate = req.InstallDate
	m.Remark = req.Remark
	if req.Rate > 0 {
		m.Rate = req.Rate
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	m.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Save(&m).Error; err != nil {
		c.Error(errors.NewInternalServerError("update time meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}

// DeleteBillingTimeMeter godoc
// @Summary      删除分时电表
// @Router       /billing/meters/:id [delete]
func (h *BillingHandler) DeleteBillingTimeMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.BillingTimeMeter{}).
		Where("id = ?", id).Update("deleted_at", now).Error; err != nil {
		c.Error(errors.NewInternalServerError("delete time meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 水表(总表/工业/宿舍/消防)
// ---------------------------------------------------------------------------

// ListBillingWaterMeters godoc
// @Summary      水表列表
// @Router       /billing/tenants/:id/water-meters [get]
func (h *BillingHandler) ListBillingWaterMeters(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var meters []types.BillingWaterMeter
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL", id).
		Order("created_at ASC").Find(&meters).Error; err != nil {
		c.Error(errors.NewInternalServerError("list water meters failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": meters})
}

// CreateBillingWaterMeter godoc
// @Summary      新增水表
// @Router       /billing/tenants/:id/water-meters [post]
func (h *BillingHandler) CreateBillingWaterMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Name        string  `json:"name"`
		MeterNo     string  `json:"meter_no"`
		MeterKind   string  `json:"meter_kind"` // total | industry | dorm | fire
		OwnerUnit   string  `json:"owner_unit"`
		UseUnit     string  `json:"use_unit"`
		Manager     string  `json:"manager"`
		Contact     string  `json:"contact"`
		MeterMode   string  `json:"meter_mode"`
		InstallDate *string `json:"install_date"`
		Remark      string  `json:"remark"`
		Rate        float64 `json:"rate"`
		Price       float64 `json:"price"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("name is required"))
		return
	}
	switch req.MeterKind {
	case "industry", "dorm", "fire":
	default:
		req.MeterKind = "total"
	}
	if req.MeterMode != "auto" {
		req.MeterMode = "manual"
	}
	m := types.BillingWaterMeter{
		ID: uuid.NewString(), BillingTenantID: id,
		Name: req.Name, MeterNo: req.MeterNo, MeterKind: req.MeterKind,
		OwnerUnit: req.OwnerUnit, UseUnit: req.UseUnit,
		Manager: req.Manager, Contact: req.Contact, MeterMode: req.MeterMode,
		InstallDate: req.InstallDate, Remark: req.Remark,
		Rate: req.Rate, Price: req.Price, Enabled: true,
		CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if m.Rate <= 0 {
		m.Rate = 1
	}
	if err := h.db.WithContext(ctx).Create(&m).Error; err != nil {
		c.Error(errors.NewInternalServerError("create water meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}

// UpdateBillingWaterMeter godoc
// @Summary      更新水表
// @Router       /billing/water-meters/:id [put]
func (h *BillingHandler) UpdateBillingWaterMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Name        string  `json:"name"`
		MeterNo     string  `json:"meter_no"`
		MeterKind   string  `json:"meter_kind"`
		OwnerUnit   string  `json:"owner_unit"`
		UseUnit     string  `json:"use_unit"`
		Manager     string  `json:"manager"`
		Contact     string  `json:"contact"`
		MeterMode   string  `json:"meter_mode"`
		InstallDate *string `json:"install_date"`
		Remark      string  `json:"remark"`
		Rate        float64 `json:"rate"`
		Price       float64 `json:"price"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	var m types.BillingWaterMeter
	if err := h.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&m).Error; err != nil {
		c.Error(errors.NewNotFoundError("水表不存在"))
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		m.Name = strings.TrimSpace(req.Name)
	}
	m.MeterNo = req.MeterNo
	switch req.MeterKind {
	case "industry", "dorm", "fire", "total":
		m.MeterKind = req.MeterKind
	}
	m.OwnerUnit = req.OwnerUnit
	m.UseUnit = req.UseUnit
	m.Manager = req.Manager
	m.Contact = req.Contact
	if req.MeterMode == "auto" || req.MeterMode == "manual" {
		m.MeterMode = req.MeterMode
	}
	m.InstallDate = req.InstallDate
	m.Remark = req.Remark
	if req.Rate > 0 {
		m.Rate = req.Rate
	}
	if req.Price > 0 {
		m.Price = req.Price
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	m.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Save(&m).Error; err != nil {
		c.Error(errors.NewInternalServerError("update water meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": m})
}

// DeleteBillingWaterMeter godoc
// @Summary      删除水表
// @Router       /billing/water-meters/:id [delete]
func (h *BillingHandler) DeleteBillingWaterMeter(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.BillingWaterMeter{}).
		Where("id = ?", id).Update("deleted_at", now).Error; err != nil {
		c.Error(errors.NewInternalServerError("delete water meter failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// GetBillingWaterReading godoc
// @Summary      水表某月读数
// @Router       /billing/water-meters/:id/readings [get]
func (h *BillingHandler) GetBillingWaterReading(c *gin.Context) {
	ctx := c.Request.Context()
	meterID := c.Param("id")
	month := strings.TrimSpace(c.Query("month"))
	var r types.BillingWaterReading
	err := h.db.WithContext(ctx).Where("meter_id = ? AND month = ?", meterID, month).First(&r).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// ListBillingWaterReadings godoc
// @Summary      水表读数列表(可按月份过滤)
// @Router       /billing/water-readings [get]
func (h *BillingHandler) ListBillingWaterReadings(c *gin.Context) {
	ctx := c.Request.Context()
	month := strings.TrimSpace(c.Query("month"))
	var list []types.BillingWaterReading
	q := h.db.WithContext(ctx)
	if month != "" {
		q = q.Where("month = ?", month)
	}
	if err := q.Order("month DESC, created_at ASC").Find(&list).Error; err != nil {
		c.Error(errors.NewInternalServerError("list water readings failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// SaveBillingWaterReading godoc
// @Summary      保存水表读数(upsert)
// @Router       /billing/water-meters/:id/readings [put]
func (h *BillingHandler) SaveBillingWaterReading(c *gin.Context) {
	ctx := c.Request.Context()
	meterID := c.Param("id")
	var req struct {
		Month string  `json:"month"`
		Prev  float64 `json:"prev"`
		Curr  float64 `json:"curr"`
		Price float64 `json:"price"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Month = strings.TrimSpace(req.Month)
	if len(req.Month) != 7 {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}
	now := timeNowUTC()
	var r types.BillingWaterReading
	err := h.db.WithContext(ctx).Where("meter_id = ? AND month = ?", meterID, req.Month).First(&r).Error
	if err == gorm.ErrRecordNotFound {
		r = types.BillingWaterReading{
			ID: uuid.NewString(), MeterID: meterID, Month: req.Month,
			Prev: req.Prev, Curr: req.Curr, Price: req.Price,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&r).Error; err != nil {
			c.Error(errors.NewInternalServerError("save water reading failed"))
			return
		}
	} else if err != nil {
		c.Error(errors.NewInternalServerError("query water reading failed"))
		return
	} else {
		r.Prev, r.Curr, r.Price = req.Prev, req.Curr, req.Price
		r.UpdatedAt = now
		if err := h.db.WithContext(ctx).Save(&r).Error; err != nil {
			c.Error(errors.NewInternalServerError("save water reading failed"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// ---------------------------------------------------------------------------
// 分时读数
// ---------------------------------------------------------------------------

// GetBillingTimeReading godoc
// @Summary      分时电表某月读数
// @Router       /billing/meters/:id/readings [get]
func (h *BillingHandler) GetBillingTimeReading(c *gin.Context) {
	ctx := c.Request.Context()
	meterID := c.Param("id")
	month := strings.TrimSpace(c.Query("month"))
	var r types.BillingTimeReading
	err := h.db.WithContext(ctx).Where("meter_id = ? AND month = ?", meterID, month).First(&r).Error
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// SaveBillingTimeReading godoc
// @Summary      保存分时读数(upsert)
// @Router       /billing/meters/:id/readings [put]
func (h *BillingHandler) SaveBillingTimeReading(c *gin.Context) {
	ctx := c.Request.Context()
	meterID := c.Param("id")
	var req struct {
		Month      string  `json:"month"`
		DeepPrev   float64 `json:"deep_prev"`
		DeepCurr   float64 `json:"deep_curr"`
		PeakPrev   float64 `json:"peak_prev"`
		PeakCurr   float64 `json:"peak_curr"`
		FlatPrev   float64 `json:"flat_prev"`
		FlatCurr   float64 `json:"flat_curr"`
		ValleyPrev float64 `json:"valley_prev"`
		ValleyCurr float64 `json:"valley_curr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Month = strings.TrimSpace(req.Month)
	if len(req.Month) != 7 {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}
	now := timeNowUTC()
	var r types.BillingTimeReading
	err := h.db.WithContext(ctx).Where("meter_id = ? AND month = ?", meterID, req.Month).First(&r).Error
	if err == gorm.ErrRecordNotFound {
		r = types.BillingTimeReading{
			ID: uuid.NewString(), MeterID: meterID, Month: req.Month,
			DeepPrev: req.DeepPrev, DeepCurr: req.DeepCurr,
			PeakPrev: req.PeakPrev, PeakCurr: req.PeakCurr,
			FlatPrev: req.FlatPrev, FlatCurr: req.FlatCurr,
			ValleyPrev: req.ValleyPrev, ValleyCurr: req.ValleyCurr,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(ctx).Create(&r).Error; err != nil {
			c.Error(errors.NewInternalServerError("save reading failed"))
			return
		}
	} else if err != nil {
		c.Error(errors.NewInternalServerError("query reading failed"))
		return
	} else {
		r.DeepPrev, r.DeepCurr = req.DeepPrev, req.DeepCurr
		r.PeakPrev, r.PeakCurr = req.PeakPrev, req.PeakCurr
		r.FlatPrev, r.FlatCurr = req.FlatPrev, req.FlatCurr
		r.ValleyPrev, r.ValleyCurr = req.ValleyPrev, req.ValleyCurr
		r.UpdatedAt = now
		if err := h.db.WithContext(ctx).Save(&r).Error; err != nil {
			c.Error(errors.NewInternalServerError("save reading failed"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": r})
}

// ---------------------------------------------------------------------------
// 子项开关 & 表计引用
// ---------------------------------------------------------------------------

// SaveBillingTenantItems godoc
// @Summary      保存分摊子项开关(全量)
// @Router       /billing/tenants/:id/items [put]
func (h *BillingHandler) SaveBillingTenantItems(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Items []struct {
			Category string `json:"category"`
			ItemKey  string `json:"item_key"`
			ItemName string `json:"item_name"`
			Enabled  bool   `json:"enabled"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	now := timeNowUTC()
	seenKey := make(map[string]bool, len(req.Items))
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("billing_tenant_id = ?", id).Delete(&types.BillingTenantItem{}).Error; err != nil {
			return err
		}
		for i, it := range req.Items {
			key := strings.TrimSpace(it.ItemKey)
			if key == "" || seenKey[key] {
				continue
			}
			seenKey[key] = true
			item := types.BillingTenantItem{
				ID: uuid.NewString(), BillingTenantID: id,
				Category: strings.TrimSpace(it.Category),
				ItemKey:  key, ItemName: it.ItemName, Enabled: it.Enabled,
				Sort: i, CreatedAt: now, UpdatedAt: now,
			}
			if item.ItemName == "" {
				item.ItemName = key
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "save billing items failed: %v", err)
		c.Error(errors.NewInternalServerError("save items failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// SaveBillingTenantRefs godoc
// @Summary      保存表计引用(全量)
// @Router       /billing/tenants/:id/refs [put]
func (h *BillingHandler) SaveBillingTenantRefs(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Refs []struct {
			Category string `json:"category"`
			MeterID  string `json:"meter_id"`
		} `json:"refs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	now := timeNowUTC()
	// 表计名称从 utility_meters 回填
	refs := make([]types.BillingTenantMeterRef, 0, len(req.Refs))
	for _, r := range req.Refs {
		if r.Category != "electricity" && r.Category != "water" {
			continue
		}
		if strings.TrimSpace(r.MeterID) == "" {
			continue
		}
		name := ""
		var m types.UtilityMeter
		if err := h.db.WithContext(ctx).Where("id = ?", r.MeterID).First(&m).Error; err == nil {
			name = m.Alias
		}
		refs = append(refs, types.BillingTenantMeterRef{
			ID: uuid.NewString(), BillingTenantID: id, Category: r.Category,
			MeterID: r.MeterID, MeterName: name, CreatedAt: now, UpdatedAt: now,
		})
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("billing_tenant_id = ?", id).Delete(&types.BillingTenantMeterRef{}).Error; err != nil {
			return err
		}
		for _, r := range refs {
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.Error(errors.NewInternalServerError("save refs failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 月度账单
// ---------------------------------------------------------------------------

// ListBillingRecords godoc
// @Summary      月度账单列表
// @Router       /billing/tenants/:id/records [get]
func (h *BillingHandler) ListBillingRecords(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var recs []types.BillingRecord
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL", id).
		Order("month DESC").Find(&recs).Error; err != nil {
		c.Error(errors.NewInternalServerError("list billing records failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": recs})
}

// GetBillingRecord godoc
// @Summary      账单详情(含子行)
// @Router       /billing/records/:id [get]
func (h *BillingHandler) GetBillingRecord(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var rec types.BillingRecord
	if err := h.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&rec).Error; err != nil {
		c.Error(errors.NewNotFoundError("账单不存在"))
		return
	}
	var items []types.BillingRecordItem
	_ = h.db.WithContext(ctx).Where("record_id = ?", id).Order("sort ASC, created_at ASC").Find(&items).Error
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"record": rec, "items": items}})
}

// DeleteBillingRecord godoc
// @Summary      删除账单
// @Router       /billing/records/:id [delete]
func (h *BillingHandler) DeleteBillingRecord(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.BillingRecord{}).
		Where("id = ?", id).Update("deleted_at", now).Error; err != nil {
		c.Error(errors.NewInternalServerError("delete billing record failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// utilityBillMeta mirrors knowledge custom_metadata for utility bill kind.
type utilityBillMeta struct {
	Kind    string                            `json:"kind"`
	Records []types.UtilityBillExtractionItem `json:"records"`
}

// loadUtilityBillByMonth 从市电账单知识库中按账单周期月份取电费提取数据。
func (h *BillingHandler) loadUtilityBillByMonth(ctx context.Context, kbID, month string) (*types.UtilityBillExtractionItem, error) {
	if kbID == "" {
		return nil, errors.NewBadRequestError("未配置市电账单知识库")
	}
	var knowledges []types.Knowledge
	if err := h.db.WithContext(ctx).Where("knowledge_base_id = ?", kbID).
		Find(&knowledges).Error; err != nil {
		return nil, err
	}
	for _, k := range knowledges {
		if len(k.CustomMetadata) == 0 {
			continue
		}
		var meta utilityBillMeta
		if err := json.Unmarshal([]byte(k.CustomMetadata), &meta); err != nil {
			continue
		}
		if meta.Kind != "utility_bill" || len(meta.Records) == 0 {
			continue
		}
		for i := range meta.Records {
			r := &meta.Records[i]
			if strings.HasPrefix(r.BillPeriodStart, month) || strings.HasPrefix(r.BillPeriodEnd, month) ||
				strings.Contains(r.BillPeriodStart+"~"+r.BillPeriodEnd, month) {
				return r, nil
			}
		}
	}
	return nil, errors.NewBadRequestError("未找到该月的市电账单(" + month + "),请先上传解析")
}

// GenerateBillingRecord godoc
// @Summary      生成月度账单
// @Router       /billing/tenants/:id/records [post]
func (h *BillingHandler) GenerateBillingRecord(c *gin.Context) {
	ctx := c.Request.Context()
	id := c.Param("id")
	var req struct {
		Month string `json:"month"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	req.Month = strings.TrimSpace(req.Month)
	if len(req.Month) != 7 {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}
	var tenant types.BillingTenant
	if err := h.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&tenant).Error; err != nil {
		c.Error(errors.NewNotFoundError("租户不存在"))
		return
	}
	var st types.BillingTenantSetting
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).First(&st).Error; err != nil {
		st = types.BillingTenantSetting{DormPrice: 1, WaterPrice: 5.22}
	}
	if st.DormPrice <= 0 {
		st.DormPrice = 1
	}
	if st.WaterPrice <= 0 {
		st.WaterPrice = 5.22
	}

	// 1) 分时读数:总表(星达铜业)合计、租户分表合计
	starPeriod, subPeriod, err := h.loadPeriodKwh(ctx, id, tenant.Name, req.Month)
	if err != nil {
		c.Error(err)
		return
	}
	starTotal := periodSum(starPeriod)
	subTotal := periodSum(subPeriod)
	if starTotal <= 0 {
		c.Error(errors.NewBadRequestError("请先录入该月星达分表读数"))
		return
	}
	lineLoss := round2(starTotal - subTotal)
	if lineLoss < 0 {
		c.Error(errors.NewBadRequestError("星达分表电量小于工业分表合计,请检查读数"))
		return
	}
	// 线损按工业分表分时占比分摊
	billPeriod := make(map[string]float64, 4)
	if subTotal > 0 {
		for _, p := range []string{"deep", "peak", "flat", "valley"} {
			billPeriod[p] = round2(subPeriod[p] + lineLoss*subPeriod[p]/subTotal)
		}
	} else {
		// 无工业分表读数时直接使用星达分表分时电量
		for _, p := range []string{"deep", "peak", "flat", "valley"} {
			billPeriod[p] = round2(starPeriod[p])
		}
	}
	billBaseTotal := round2(starTotal) // 比例分子

	// 2) 宿舍/水表引用读数（无显式引用时按使用单位=租户名自动匹配）
	refs := h.loadMeterRefs(ctx, id)
	if len(refs) == 0 {
		refs = h.loadRefsByUseUnit(ctx, tenant.Name)
	}
	dormKwh, dormFee, err := h.sumRefFee(ctx, refs, "electricity", req.Month)
	if err != nil {
		c.Error(err)
		return
	}
	waterUsage, waterFee, err := h.sumRefFee(ctx, refs, "water", req.Month)
	if err != nil {
		c.Error(err)
		return
	}

	// 3) 市电账单
	billItem, err := h.loadUtilityBillByMonth(ctx, st.BillKBID, req.Month)
	if err != nil {
		c.Error(err)
		return
	}
	if billItem.TotalKwh <= 0 {
		c.Error(errors.NewBadRequestError("市电账单本期电量为空,无法计算比例"))
		return
	}
	ratio := round6(billBaseTotal / billItem.TotalKwh)
	billBase := round2(starTotal - dormKwh) // 非分时子项计费基数(星达分表-宿舍)
	if billBase < 0 {
		billBase = 0
	}

	// 4) 子项开关：首次自动初始化，账单新增大类时自动补充
	feeItems := billItem.FeeItems
	enabled := h.loadEnabledItems(ctx, id)
	if len(enabled) == 0 {
		enabled = h.ensureDefaultItems(ctx, id, feeItems)
	} else {
		enabled = h.syncMissingItems(ctx, id, feeItems, enabled)
	}

	// 5) 费用明细(固化)
	now := timeNowUTC()
	recordID := uuid.NewString()
	var items []types.BillingRecordItem
	sortNo := 0
	industrialFee := 0.0

	// 5.1 FeeItems 子项(按时段单价/计费基数×单价)
	catRows := groupFeeItems(feeItems)
	for _, cat := range catRows {
		// 子项(name)开关控制:大类下同名子项可多行(分时时段)。
		// 同一子项内:分时行按时段电量×时段单价;非分时行按计费基数×单价;
		// 无电量无单价但带金额的固定项(返还类)按分摊比例计算。
		for _, sg := range groupFeeItemsByName(cat.Rows) {
			if !enabled[sg.Name] {
				continue
			}
			fee := 0.0
			hasPeriod := false
			rateByPeriod := make(map[string]float64, 4)
			flatSet := false
			var flatRate float64
			var fixedFee float64
			seenPeriod := make(map[string]bool, 4)
			for _, r := range sg.Rows {
				// 固定金额项(qty=0 且 rate=0 但 fee≠0,如返还类):按分摊比例承担
				if r.Qty <= 0 && r.Rate <= 0 && r.Fee != 0 {
					fixedFee += r.Fee
					continue
				}
				p := normalizePeriod(r.Period)
				if p != "" {
					seenPeriod[p] = true
					rate := r.Rate
					if rate <= 0 && r.Qty > 0 {
						rate = r.Fee / r.Qty
					}
					if rate != 0 {
						rateByPeriod[p] = rate
					}
				} else {
					rate := r.Rate
					if rate <= 0 && r.Qty > 0 {
						rate = r.Fee / r.Qty
					}
					if rate != 0 {
						flatRate = rate
						flatSet = true
					}
				}
			}
			// 真正分时子项:含尖峰/峰/谷任一时段(如零售交易、零售输配、偏差电费)
			hasPeriod = seenPeriod["deep"] || seenPeriod["peak"] || seenPeriod["valley"]
			if hasPeriod {
				for _, p := range []string{"deep", "peak", "flat", "valley"} {
					rate, ok := rateByPeriod[p]
					if !ok || rate == 0 {
						continue
					}
					f := round2(billPeriod[p] * rate)
					items = append(items, types.BillingRecordItem{
						ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
						Name: sg.Name, Period: periodLabel(p),
						Qty: billPeriod[p], Rate: rate, Fee: f, Sort: sortNo, CreatedAt: now,
					})
					sortNo++
					fee += f
				}
				// 分时子项内的非分时行(若有):按计费基数
				if flatSet && flatRate != 0 {
					f := round2(billBase * flatRate)
					items = append(items, types.BillingRecordItem{
						ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
						Name: sg.Name, Period: "", Qty: billBase, Rate: flatRate, Fee: f, Sort: sortNo, CreatedAt: now,
					})
					sortNo++
					fee += f
				}
			} else if flatSet {
				f := round2(billBase * flatRate)
				items = append(items, types.BillingRecordItem{
					ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
					Name: sg.Name, Period: "", Qty: billBase, Rate: flatRate, Fee: f, Sort: sortNo, CreatedAt: now,
				})
				sortNo++
				fee = f
			} else if flatRate0, ok := rateByPeriod["flat"]; ok && flatRate0 != 0 {
				// 仅"平"时段的子项(线损/系统运行/政府基金等):按计费基数×平段单价
				f := round2(billBase * flatRate0)
				items = append(items, types.BillingRecordItem{
					ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
					Name: sg.Name, Period: "平", Qty: billBase, Rate: flatRate0, Fee: f, Sort: sortNo, CreatedAt: now,
				})
				sortNo++
				fee = f
			}
			if fixedFee != 0 {
				f := round2(fixedFee * ratio)
				items = append(items, types.BillingRecordItem{
					ID: uuid.NewString(), RecordID: recordID, Kind: "fee",
					Name: sg.Name, Period: "", Qty: 0, Rate: ratio, Fee: f, Sort: sortNo, CreatedAt: now,
				})
				sortNo++
				fee += f
			}
			industrialFee += fee
		}
	}

	// 5.2 基本电费 / 力调电费(按比例)
	if enabled["基本电费"] && billItem.CapacityFee != 0 {
		f := round2(billItem.CapacityFee * ratio)
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "base",
			Name: "基本电费", Period: "", Qty: ratio, Rate: billItem.CapacityFee, Fee: f, Sort: sortNo, CreatedAt: now,
		})
		sortNo++
		industrialFee += f
	}
	if enabled["力调电费"] && billItem.PfAdjustAmount != 0 {
		f := round2(billItem.PfAdjustAmount * ratio)
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "pf",
			Name: "力调电费", Period: "", Qty: ratio, Rate: billItem.PfAdjustAmount, Fee: f, Sort: sortNo, CreatedAt: now,
		})
		sortNo++
		industrialFee += f
	}

	// 5.3 宿舍/水
	if dormFee != 0 {
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "dorm",
			Name: "宿舍电费", Period: "", Qty: dormKwh, Rate: st.DormPrice, Fee: dormFee, Sort: sortNo, CreatedAt: now,
		})
		sortNo++
	}
	if waterFee != 0 {
		items = append(items, types.BillingRecordItem{
			ID: uuid.NewString(), RecordID: recordID, Kind: "water",
			Name: "水费", Period: "", Qty: waterUsage, Rate: st.WaterPrice, Fee: waterFee, Sort: sortNo, CreatedAt: now,
		})
		sortNo++
	}

	industrialFee = round2(industrialFee)
	totalFee := round2(industrialFee + dormFee + waterFee)
	rec := types.BillingRecord{
		ID: recordID, BillingTenantID: id, Month: req.Month, Status: "generated",
		TotalKwh: round2(starTotal), LineLoss: lineLoss, Ratio: ratio,
		BillTotalAmount: billItem.TotalAmount,
		DormKwh: round2(dormKwh), DormFee: dormFee,
		WaterUsage: round2(waterUsage), WaterFee: waterFee,
		IndustrialFee: industrialFee, TotalFee: totalFee,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 同月已存在 → 覆盖(删除旧账单及子行)
		var old types.BillingRecord
		if err := tx.Where("billing_tenant_id = ? AND month = ?", id, req.Month).First(&old).Error; err == nil {
			if err := tx.Where("record_id = ?", old.ID).Delete(&types.BillingRecordItem{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&rec).Error; err != nil {
			return err
		}
		for _, it := range items {
			if err := tx.Create(&it).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		logger.Errorf(ctx, "generate billing record failed: %v", err)
		c.Error(errors.NewInternalServerError("generate billing record failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"record": rec, "items": items}})
}

// ---------------------------------------------------------------------------
// 核算辅助
// ---------------------------------------------------------------------------

type periodMap = map[string]float64

func periodSum(p periodMap) float64 {
	return round2(p["deep"] + p["peak"] + p["flat"] + p["valley"])
}

func periodLabel(p string) string {
	switch p {
	case "deep":
		return "尖峰"
	case "peak":
		return "峰"
	case "flat":
		return "平"
	case "valley":
		return "谷"
	}
	return ""
}

func normalizePeriod(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "尖") {
		return "deep"
	}
	if strings.Contains(s, "峰") {
		return "peak"
	}
	if strings.Contains(s, "平") {
		return "flat"
	}
	if strings.Contains(s, "谷") {
		return "valley"
	}
	return ""
}

func round6(v float64) float64 {
	return float64(int64(v*1000000+0.5)) / 1000000
}

// loadPeriodKwh 汇总该月分时电表各时段电量(读数差×倍率)。
// 总表组 = 归属单位「星达铜业」;分表组 = 归属单位 = 租户名。
func (h *BillingHandler) loadPeriodKwh(ctx context.Context, tenantID, tenantName, month string) (periodMap, periodMap, error) {
	star := make(periodMap, 4)
	sub := make(periodMap, 4)
	baseOwner := strings.TrimSpace(tenantName) // 兼容:旧数据 star 归属星达铜业
	var meters []types.BillingTimeMeter
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL AND enabled = ?", tenantID, true).
		Find(&meters).Error; err != nil {
		return nil, nil, err
	}
		for _, m := range meters {
		if m.MeterKind != "" && m.MeterKind != "time" {
			continue // 普通电表不参与分时核算
		}
		var r types.BillingTimeReading
		if err := h.db.WithContext(ctx).Where("meter_id = ? AND month = ?", m.ID, month).First(&r).Error; err != nil {
			continue
		}
		rate := m.Rate
		if rate <= 0 {
			rate = 1
		}
		dp := round2((r.DeepCurr - r.DeepPrev) * rate)
		pk := round2((r.PeakCurr - r.PeakPrev) * rate)
		fl := round2((r.FlatCurr - r.FlatPrev) * rate)
		vl := round2((r.ValleyCurr - r.ValleyPrev) * rate)
		owner := strings.TrimSpace(m.OwnerUnit)
		switch {
		case owner == "星达铜业":
			star["deep"] += dp
			star["peak"] += pk
			star["flat"] += fl
			star["valley"] += vl
		case owner != "" && owner == baseOwner:
			sub["deep"] += dp
			sub["peak"] += pk
			sub["flat"] += fl
			sub["valley"] += vl
		case owner == "" && m.MeterType == "star":
			star["deep"] += dp
			star["peak"] += pk
			star["flat"] += fl
			star["valley"] += vl
		case owner == "":
			sub["deep"] += dp
			sub["peak"] += pk
			sub["flat"] += fl
			sub["valley"] += vl
		default:
			// 其它归属单位(如其它租户分表)不计入当前租户核算
		}
	}
	return star, sub, nil
}

func (h *BillingHandler) loadMeterRefs(ctx context.Context, tenantID string) []types.BillingTenantMeterRef {
	var refs []types.BillingTenantMeterRef
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ?", tenantID).Find(&refs).Error
	return refs
}

// loadRefsByUseUnit 按使用单位(租户名)自动匹配宿舍电表与水表作为引用。
func (h *BillingHandler) loadRefsByUseUnit(ctx context.Context, useUnit string) []types.BillingTenantMeterRef {
	useUnit = strings.TrimSpace(useUnit)
	if useUnit == "" {
		return nil
	}
	var meters []types.UtilityMeter
	if err := h.db.WithContext(ctx).
		Where("use_unit = ? AND deleted_at IS NULL", useUnit).
		Find(&meters).Error; err != nil {
		logger.Errorf(ctx, "load refs by use unit failed: %v", err)
		return nil
	}
	refs := make([]types.BillingTenantMeterRef, 0, len(meters))
	for _, m := range meters {
		if m.Category == "electricity" || m.Category == "water" {
			refs = append(refs, types.BillingTenantMeterRef{ID: uuid.NewString(), Category: m.Category, MeterID: m.ID})
		}
	}
	return refs
}

func (h *BillingHandler) loadEnabledItems(ctx context.Context, tenantID string) map[string]bool {
	var items []types.BillingTenantItem
	_ = h.db.WithContext(ctx).Where("billing_tenant_id = ?", tenantID).Find(&items).Error
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it.ItemKey] = it.Enabled
	}
	return m
}

// ensureDefaultItems 首次生成账单时按账单 FeeItems 自动初始化分摊子项(子项级)：
// 工商业类默认启用，居民/目录类默认关闭；基本电费、力调电费默认启用。
func (h *BillingHandler) ensureDefaultItems(ctx context.Context, tenantID string, feeItems []types.UtilityBillFeeItem) map[string]bool {
	type row struct {
		cat  string
		name string
		on   bool
	}
	var rows []row
	seen := make(map[string]bool, 32)
	add := func(cat, name string, on bool) {
		if name == "" {
			return
		}
		// 按子项名全局去重(同一子项可能出现在多个大类/多个时段行),保证 item_key 唯一
		if seen[name] {
			return
		}
		seen[name] = true
		rows = append(rows, row{cat: cat, name: name, on: on})
	}
	for _, f := range feeItems {
		cat := strings.TrimSpace(f.Category)
		if cat == "" {
			cat = "其他费用"
		}
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = cat
		}
		// 居民/目录电费按宿舍定额单列，不参与比例分摊
		on := !strings.Contains(cat, "居民") && !strings.Contains(cat, "目录电费")
		add(cat, name, on)
	}
	add("基本电费", "基本电费", true)
	add("力调电费", "力调电费", true)
	enabled := make(map[string]bool, len(rows))
	if len(rows) == 0 {
		return enabled
	}
	items := make([]types.BillingTenantItem, 0, len(rows))
	now := timeNowUTC()
	for i, r := range rows {
		enabled[r.name] = r.on
		items = append(items, types.BillingTenantItem{
			ID: uuid.NewString(), BillingTenantID: tenantID,
			Category: r.cat, ItemKey: r.name, ItemName: r.name,
			Enabled: r.on, Sort: i, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := h.db.WithContext(ctx).Create(&items).Error; err != nil {
		logger.Errorf(ctx, "ensure default items failed: %v", err)
	}
	return enabled
}

// syncMissingItems 账单子项比已有开关新增时，自动补充新子项（居民/目录默认关闭，其余默认启用），并入库。
func (h *BillingHandler) syncMissingItems(ctx context.Context, tenantID string, feeItems []types.UtilityBillFeeItem, enabled map[string]bool) map[string]bool {
	now := timeNowUTC()
	var created []types.BillingTenantItem
	sort := 0
	add := func(cat, name string) {
		if name == "" {
			return
		}
		// 按子项名全局去重,保证 item_key 唯一
		if enabled[name] {
			return
		}
		on := !strings.Contains(cat, "居民") && !strings.Contains(cat, "目录电费")
		created = append(created, types.BillingTenantItem{
			ID: uuid.NewString(), BillingTenantID: tenantID,
			Category: cat, ItemKey: name, ItemName: name,
			Enabled: on, Sort: sort, CreatedAt: now, UpdatedAt: now,
		})
		enabled[name] = on
		sort++
	}
	for _, f := range feeItems {
		cat := strings.TrimSpace(f.Category)
		if cat == "" {
			cat = "其他费用"
		}
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = cat
		}
		add(cat, name)
	}
	if _, ok := enabled["基本电费"]; !ok {
		add("基本电费", "基本电费")
	}
	if _, ok := enabled["力调电费"]; !ok {
		add("力调电费", "力调电费")
	}
	if len(created) > 0 {
		if err := h.db.WithContext(ctx).Create(&created).Error; err != nil {
			logger.Errorf(ctx, "sync missing items failed: %v", err)
		}
	}
	return enabled
}

// sumRefFee 汇总引用表计该月用量(按 usage 或 end-start)并按表计单价计算费用。
// 宿舍/水费单价取电费/水费页面配置的表计默认单价;无单价表计费用按 0,用量照记。
func (h *BillingHandler) sumRefFee(ctx context.Context, refs []types.BillingTenantMeterRef, category, month string) (float64, float64, error) {
	meterIDs := make([]string, 0)
	priceOf := make(map[string]float64)
	for _, r := range refs {
		if r.Category != category {
			continue
		}
		meterIDs = append(meterIDs, r.MeterID)
		var m types.UtilityMeter
		if err := h.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", r.MeterID).First(&m).Error; err == nil {
			priceOf[r.MeterID] = m.DefaultUnitPrice
		}
	}
	if len(meterIDs) == 0 {
		return 0, 0, nil
	}
	var records []types.UtilityMeterRecord
	if err := h.db.WithContext(ctx).
		Where("category = ? AND month = ? AND deleted_at IS NULL", category, month).
		Find(&records).Error; err != nil {
		return 0, 0, err
	}
	set := make(map[string]bool, len(meterIDs))
	for _, id := range meterIDs {
		set[id] = true
	}
	total := 0.0
	fee := 0.0
	for i := range records {
		var its []types.UtilityMeterItem
		if err := h.db.WithContext(ctx).Where("record_id = ?", records[i].ID).Find(&its).Error; err != nil {
			continue
		}
		for _, it := range its {
			if !set[it.MeterID] {
				continue
			}
			u := it.Usage
			if u <= 0 && it.EndReading >= it.StartReading {
				u = it.EndReading - it.StartReading
			}
			total += u
			if p := priceOf[it.MeterID]; p > 0 {
				fee += u * p
			}
		}
	}
	return round2(total), round2(fee), nil
}

type feeCatGroup struct {
	Name  string
	Rows  []types.UtilityBillFeeItem
}

// groupFeeItems 按费用类别分组并保持账单顺序。
func groupFeeItems(feeItems []types.UtilityBillFeeItem) []feeCatGroup {
	order := make([]string, 0)
	groups := make(map[string]*feeCatGroup)
	for _, f := range feeItems {
		name := strings.TrimSpace(f.Category)
		if name == "" {
			name = "其他费用"
		}
		if _, ok := groups[name]; !ok {
			groups[name] = &feeCatGroup{Name: name}
			order = append(order, name)
		}
		groups[name].Rows = append(groups[name].Rows, f)
	}
	out := make([]feeCatGroup, 0, len(order))
	for _, n := range order {
		out = append(out, *groups[n])
	}
	return out
}

// groupFeeItemsByName 按子项名(name)分组,保留账单顺序,支持同名多行(分时时段)。
func groupFeeItemsByName(feeItems []types.UtilityBillFeeItem) []feeCatGroup {
	order := make([]string, 0)
	groups := make(map[string]*feeCatGroup)
	for _, f := range feeItems {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = "其他费用"
		}
		if _, ok := groups[name]; !ok {
			groups[name] = &feeCatGroup{Name: name}
			order = append(order, name)
		}
		groups[name].Rows = append(groups[name].Rows, f)
	}
	out := make([]feeCatGroup, 0, len(order))
	for _, n := range order {
		out = append(out, *groups[n])
	}
	return out
}
