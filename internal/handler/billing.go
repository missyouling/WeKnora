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
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tenant":  t,
			"setting": st,
			"items":   items,
			"refs":    refs,
			"meters":  meters,
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
		Name      string  `json:"name"`
		Remark    string  `json:"remark"`
		DormPrice float64 `json:"dorm_price"`
		WaterPrice float64 `json:"water_price"`
		BillKBID  string  `json:"bill_kb_id"`
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
	t := types.BillingTenant{
		ID: uuid.NewString(), TenantID: int64(tenantID), Name: req.Name,
		Remark: req.Remark, CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	st := types.BillingTenantSetting{
		ID: uuid.NewString(), BillingTenantID: t.ID,
		DormPrice: req.DormPrice, WaterPrice: req.WaterPrice, BillKBID: req.BillKBID,
		CreatedAt: timeNowUTC(), UpdatedAt: timeNowUTC(),
	}
	if st.DormPrice <= 0 {
		st.DormPrice = 1
	}
	if st.WaterPrice <= 0 {
		st.WaterPrice = 5.22
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

// UpdateBillingTenant godoc
// @Summary      更新租户与核算参数
// @Router       /billing/tenants/:id [put]
func (h *BillingHandler) UpdateBillingTenant(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := billingTenantID(c)
	id := c.Param("id")
	var req struct {
		Name       string  `json:"name"`
		Remark     string  `json:"remark"`
		DormPrice  float64 `json:"dorm_price"`
		WaterPrice float64 `json:"water_price"`
		BillKBID   string  `json:"bill_kb_id"`
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
	if req.Name != "" {
		t.Name = strings.TrimSpace(req.Name)
	}
	t.Remark = req.Remark
	t.UpdatedAt = timeNowUTC()
	var st types.BillingTenantSetting
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ?", id).First(&st).Error; err != nil {
		st = types.BillingTenantSetting{
			ID: uuid.NewString(), BillingTenantID: id, CreatedAt: timeNowUTC(),
		}
	}
	if req.DormPrice > 0 {
		st.DormPrice = req.DormPrice
	}
	if req.WaterPrice > 0 {
		st.WaterPrice = req.WaterPrice
	}
	st.BillKBID = req.BillKBID
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
		MeterType string  `json:"meter_type"`
		Name      string  `json:"name"`
		Rate      float64 `json:"rate"`
		Enabled   *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	if req.MeterType != "star" && req.MeterType != "sub" {
		c.Error(errors.NewBadRequestError("meter_type must be star or sub"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("name is required"))
		return
	}
	m := types.BillingTimeMeter{
		ID: uuid.NewString(), BillingTenantID: id, MeterType: req.MeterType,
		Name: req.Name, Rate: req.Rate, Enabled: true,
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
		Name    string  `json:"name"`
		Rate    float64 `json:"rate"`
		Enabled *bool   `json:"enabled"`
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
	if req.Name != "" {
		m.Name = strings.TrimSpace(req.Name)
	}
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
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("billing_tenant_id = ?", id).Delete(&types.BillingTenantItem{}).Error; err != nil {
			return err
		}
		for i, it := range req.Items {
			key := strings.TrimSpace(it.ItemKey)
			if key == "" {
				continue
			}
			item := types.BillingTenantItem{
				ID: uuid.NewString(), BillingTenantID: id,
				ItemKey: key, ItemName: it.ItemName, Enabled: it.Enabled,
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

	// 1) 分时读数：星达分表(star)合计、工业分表(sub)合计
	starPeriod, subPeriod, err := h.loadPeriodKwh(ctx, id, req.Month)
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
	dormKwh, err := h.sumRefUsage(ctx, refs, "electricity", req.Month)
	if err != nil {
		c.Error(err)
		return
	}
	waterUsage, err := h.sumRefUsage(ctx, refs, "water", req.Month)
	if err != nil {
		c.Error(err)
		return
	}
	dormFee := round2(dormKwh * st.DormPrice)
	waterFee := round2(waterUsage * st.WaterPrice)

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
		if !enabled[cat.Name] {
			continue
		}
		// 大类(category)开关控制;子项(name)分组,可同名多行(按时段)。
		// 同一子项内:分时行按时段电量×时段单价;非分时行按计费基数×单价;
		// 无电量无单价但带金额的固定项(返还类)按分摊比例计算。
		for _, sg := range groupFeeItemsByName(cat.Rows) {
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

// loadPeriodKwh 汇总该月星达/工业分表各时段电量(读数差×倍率)。
func (h *BillingHandler) loadPeriodKwh(ctx context.Context, tenantID, month string) (periodMap, periodMap, error) {
	star := make(periodMap, 4)
	sub := make(periodMap, 4)
	var meters []types.BillingTimeMeter
	if err := h.db.WithContext(ctx).Where("billing_tenant_id = ? AND deleted_at IS NULL AND enabled = ?", tenantID, true).
		Find(&meters).Error; err != nil {
		return nil, nil, err
	}
	for _, m := range meters {
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
		if m.MeterType == "star" {
			star["deep"] += dp
			star["peak"] += pk
			star["flat"] += fl
			star["valley"] += vl
		} else {
			sub["deep"] += dp
			sub["peak"] += pk
			sub["flat"] += fl
			sub["valley"] += vl
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

// ensureDefaultItems 首次生成账单时按账单 FeeItems 自动初始化分摊子项：
// 工商业类默认启用，居民/目录类默认关闭；基本电费、力调电费默认启用。
func (h *BillingHandler) ensureDefaultItems(ctx context.Context, tenantID string, feeItems []types.UtilityBillFeeItem) map[string]bool {
	names := make([]string, 0)
	seen := make(map[string]bool, 8)
	enabled := make(map[string]bool, 8)
	add := func(name string, on bool) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		names = append(names, name)
		enabled[name] = on
	}
	for _, f := range feeItems {
		cat := strings.TrimSpace(f.Category)
		if cat == "" {
			cat = "其他费用"
		}
		// 居民/目录电费按宿舍定额单列，不参与比例分摊
		on := !strings.Contains(cat, "居民") && !strings.Contains(cat, "目录电费")
		add(cat, on)
	}
	add("基本电费", true)
	add("力调电费", true)
	if len(names) == 0 {
		return enabled
	}
	items := make([]types.BillingTenantItem, 0, len(names))
	now := timeNowUTC()
	for i, name := range names {
		items = append(items, types.BillingTenantItem{
			ID: uuid.NewString(), BillingTenantID: tenantID,
			ItemKey: name, ItemName: name, Enabled: enabled[name], Sort: i, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := h.db.WithContext(ctx).Create(&items).Error; err != nil {
		logger.Errorf(ctx, "ensure default items failed: %v", err)
	}
	return enabled
}

// syncMissingItems 账单大类比已有开关新增时，自动补充新子项（居民/目录默认关闭，其余默认启用），并入库。
func (h *BillingHandler) syncMissingItems(ctx context.Context, tenantID string, feeItems []types.UtilityBillFeeItem, enabled map[string]bool) map[string]bool {
	now := timeNowUTC()
	var created []types.BillingTenantItem
	sort := 0
	for _, f := range feeItems {
		cat := strings.TrimSpace(f.Category)
		if cat == "" {
			cat = "其他费用"
		}
		if enabled[cat] {
			continue
		}
		on := !strings.Contains(cat, "居民") && !strings.Contains(cat, "目录电费")
		created = append(created, types.BillingTenantItem{
			ID: uuid.NewString(), BillingTenantID: tenantID,
			ItemKey: cat, ItemName: cat, Enabled: on, Sort: sort, CreatedAt: now, UpdatedAt: now,
		})
		enabled[cat] = on
		sort++
	}
	if len(created) > 0 {
		if err := h.db.WithContext(ctx).Create(&created).Error; err != nil {
			logger.Errorf(ctx, "sync missing items failed: %v", err)
		}
	}
	return enabled
}

// sumRefUsage 汇总引用表计该月用量(按 usage 或 end-start)。
func (h *BillingHandler) sumRefUsage(ctx context.Context, refs []types.BillingTenantMeterRef, category, month string) (float64, error) {
	meterIDs := make([]string, 0)
	for _, r := range refs {
		if r.Category == category {
			meterIDs = append(meterIDs, r.MeterID)
		}
	}
	if len(meterIDs) == 0 {
		return 0, nil
	}
	var records []types.UtilityMeterRecord
	if err := h.db.WithContext(ctx).
		Where("category = ? AND month = ? AND deleted_at IS NULL", category, month).
		Find(&records).Error; err != nil {
		return 0, err
	}
	set := make(map[string]bool, len(meterIDs))
	for _, id := range meterIDs {
		set[id] = true
	}
	total := 0.0
	for i := range records {
		var its []types.UtilityMeterItem
		if err := h.db.WithContext(ctx).Where("record_id = ?", records[i].ID).Find(&its).Error; err != nil {
			continue
		}
		for _, it := range its {
			if set[it.MeterID] {
				u := it.Usage
				if u <= 0 && it.EndReading >= it.StartReading {
					u = it.EndReading - it.StartReading
				}
				total += u
			}
		}
	}
	return round2(total), nil
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
