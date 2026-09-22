package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// FleetHandler 车队管理：车辆/驾驶员/油卡配置与 8 类记录（维保、加油、车险、轮胎、辅材、违章、请车、通行费）CRUD，以及按车辆月度费用汇总。
type FleetHandler struct {
	db *gorm.DB
}

// NewFleetHandler creates a new FleetHandler.
func NewFleetHandler(db *gorm.DB) *FleetHandler {
	h := &FleetHandler{db: db}
	// best-effort ensure table exists for cert groups (no versioned migration yet)
	if err := db.AutoMigrate(&types.FleetCertGroup{}, &types.FleetCategory{}); err != nil {
		logger.Warnf(context.Background(), "AutoMigrate fleet_cert_groups failed: %v", err)
	}
	return h
}

func fleetTenantID(c *gin.Context) (uint64, error) {
	return types.MustTenantIDFromContext(c.Request.Context()), nil
}

var fleetRecordTypes = map[string]bool{
	types.FleetRecordVehicleArchive:  true,
	types.FleetRecordDriverArchive:   true,
	types.FleetRecordMaintainArchive: true,
	types.FleetRecordTire:            true,
	types.FleetRecordInspection:      true,
	types.FleetRecordFuelCharge:      true,
	types.FleetRecordRoadToll:        true,
	types.FleetRecordRepairCost:      true,
	types.FleetRecordInsuranceClaim:  true,
	types.FleetRecordViolation:       true,
	types.FleetRecordMaterial:        true,
	types.FleetRecordCarRequest:      true,
	// 历史类型（v1，兼容旧数据）
	types.FleetRecordMaintain:  true,
	types.FleetRecordFuel:      true,
	types.FleetRecordInsurance: true,
	types.FleetRecordToll:      true,
}

// ---------------------------------------------------------------------------
// 车辆
// ---------------------------------------------------------------------------

// ListFleetVehicles godoc
// @Summary      车辆列表
// @Router       /fleet/vehicles [get]
func (h *FleetHandler) ListFleetVehicles(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetVehicle
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet vehicles failed: %v", err)
		c.Error(errors.NewInternalServerError("list vehicles failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetVehicle godoc
// @Summary      新增车辆
// @Router       /fleet/vehicles [post]
func (h *FleetHandler) CreateFleetVehicle(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetVehicle
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.PlateNo = strings.TrimSpace(req.PlateNo)
	if req.PlateNo == "" {
		c.Error(errors.NewBadRequestError("车牌号不能为空"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetVehicle{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet vehicle failed: %v", err)
		c.Error(errors.NewInternalServerError("create vehicle failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetVehicle godoc
// @Summary      更新车辆
// @Router       /fleet/vehicles/:id [put]
func (h *FleetHandler) UpdateFleetVehicle(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetVehicle
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.PlateNo = strings.TrimSpace(req.PlateNo)
	if req.PlateNo == "" {
		c.Error(errors.NewBadRequestError("车牌号不能为空"))
		return
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetVehicle{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check fleet vehicle failed: %v", err)
		c.Error(errors.NewInternalServerError("check vehicle failed"))
		return
	}
	if cnt == 0 {
		c.Error(errors.NewNotFoundError("车辆不存在"))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.FleetVehicle{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(map[string]interface{}{
			"plate_no":      req.PlateNo,
			"vehicle_type":  req.VehicleType,
			"brand_model":   req.BrandModel,
			"load_tonnage":  req.LoadTonnage,
			"seat_count":    req.SeatCount,
			"purchase_date": req.PurchaseDate,
			"department":    req.Department,
			"manager":       req.Manager,
			"enabled":       req.Enabled,
			"remark":        req.Remark,
			"updated_at":    now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update fleet vehicle failed: %v", err)
		c.Error(errors.NewInternalServerError("update vehicle failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetVehicle godoc
// @Summary      删除车辆
// @Router       /fleet/vehicles/:id [delete]
func (h *FleetHandler) DeleteFleetVehicle(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var refCnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("vehicle_id = ? AND deleted_at IS NULL", id).Count(&refCnt).Error; err != nil {
		logger.Errorf(ctx, "check vehicle record reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check vehicle reference failed"))
		return
	}
	if refCnt > 0 {
		c.Error(errors.NewBadRequestError("该车辆已有记录，不能删除"))
		return
	}
	var cardCnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("vehicle_id = ? AND deleted_at IS NULL", id).Count(&cardCnt).Error; err != nil {
		logger.Errorf(ctx, "check vehicle card reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check vehicle reference failed"))
		return
	}
	if cardCnt > 0 {
		c.Error(errors.NewBadRequestError("该车辆已关联油卡，不能删除"))
		return
	}
	res := h.db.WithContext(ctx).Model(&types.FleetVehicle{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet vehicle failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete vehicle failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("车辆不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// SortFleetVehicles godoc
// @Summary      车辆排序
// @Router       /fleet/vehicles/sort [put]
func (h *FleetHandler) SortFleetVehicles(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	now := timeNowUTC()
	for i, id := range req.IDs {
		if err := h.db.WithContext(ctx).Model(&types.FleetVehicle{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(map[string]interface{}{"sort_order": i + 1, "updated_at": now}).Error; err != nil {
			logger.Errorf(ctx, "sort fleet vehicle failed: %v", err)
			c.Error(errors.NewInternalServerError("sort vehicle failed"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 驾驶员
// ---------------------------------------------------------------------------

// ListFleetDrivers godoc
// @Summary      驾驶员列表
// @Router       /fleet/drivers [get]
func (h *FleetHandler) ListFleetDrivers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetDriver
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet drivers failed: %v", err)
		c.Error(errors.NewInternalServerError("list drivers failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetDriver godoc
// @Summary      新增驾驶员
// @Router       /fleet/drivers [post]
func (h *FleetHandler) CreateFleetDriver(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetDriver
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("姓名不能为空"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetDriver{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet driver failed: %v", err)
		c.Error(errors.NewInternalServerError("create driver failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetDriver godoc
// @Summary      更新驾驶员
// @Router       /fleet/drivers/:id [put]
func (h *FleetHandler) UpdateFleetDriver(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetDriver
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("姓名不能为空"))
		return
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetDriver{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check fleet driver failed: %v", err)
		c.Error(errors.NewInternalServerError("check driver failed"))
		return
	}
	if cnt == 0 {
		c.Error(errors.NewNotFoundError("驾驶员不存在"))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.FleetDriver{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(map[string]interface{}{
			"name":         req.Name,
			"license_no":   req.LicenseNo,
			"license_type": req.LicenseType,
			"phone":        req.Phone,
			"hire_date":    req.HireDate,
			"enabled":      req.Enabled,
			"remark":       req.Remark,
			"updated_at":   now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update fleet driver failed: %v", err)
		c.Error(errors.NewInternalServerError("update driver failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetDriver godoc
// @Summary      删除驾驶员
// @Router       /fleet/drivers/:id [delete]
func (h *FleetHandler) DeleteFleetDriver(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var refCnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("data->>'driver_id' = ? AND deleted_at IS NULL", id).Count(&refCnt).Error; err != nil {
		logger.Errorf(ctx, "check driver record reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check driver reference failed"))
		return
	}
	if refCnt > 0 {
		c.Error(errors.NewBadRequestError("该驾驶员已有记录，不能删除"))
		return
	}
	var cardCnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("driver_id = ? AND deleted_at IS NULL", id).Count(&cardCnt).Error; err != nil {
		logger.Errorf(ctx, "check driver card reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check driver reference failed"))
		return
	}
	if cardCnt > 0 {
		c.Error(errors.NewBadRequestError("该驾驶员已关联油卡，不能删除"))
		return
	}
	res := h.db.WithContext(ctx).Model(&types.FleetDriver{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet driver failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete driver failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("驾驶员不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// SortFleetDrivers godoc
// @Summary      驾驶员排序
// @Router       /fleet/drivers/sort [put]
func (h *FleetHandler) SortFleetDrivers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	now := timeNowUTC()
	for i, id := range req.IDs {
		if err := h.db.WithContext(ctx).Model(&types.FleetDriver{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(map[string]interface{}{"sort_order": i + 1, "updated_at": now}).Error; err != nil {
			logger.Errorf(ctx, "sort fleet driver failed: %v", err)
			c.Error(errors.NewInternalServerError("sort driver failed"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ---------------------------------------------------------------------------
// 油卡
// ---------------------------------------------------------------------------

// ListFleetFuelCards godoc
// @Summary      油卡列表
// @Router       /fleet/fuel-cards [get]
func (h *FleetHandler) ListFleetFuelCards(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetFuelCard
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet fuel cards failed: %v", err)
		c.Error(errors.NewInternalServerError("list fuel cards failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetFuelCard godoc
// @Summary      新增油卡
// @Router       /fleet/fuel-cards [post]
func (h *FleetHandler) CreateFleetFuelCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetFuelCard
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.CardNo = strings.TrimSpace(req.CardNo)
	if req.CardNo == "" {
		c.Error(errors.NewBadRequestError("卡号不能为空"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet fuel card failed: %v", err)
		c.Error(errors.NewInternalServerError("create fuel card failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetFuelCard godoc
// @Summary      更新油卡
// @Router       /fleet/fuel-cards/:id [put]
func (h *FleetHandler) UpdateFleetFuelCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetFuelCard
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.CardNo = strings.TrimSpace(req.CardNo)
	if req.CardNo == "" {
		c.Error(errors.NewBadRequestError("卡号不能为空"))
		return
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check fleet fuel card failed: %v", err)
		c.Error(errors.NewInternalServerError("check fuel card failed"))
		return
	}
	if cnt == 0 {
		c.Error(errors.NewNotFoundError("油卡不存在"))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(map[string]interface{}{
			"alias":       req.Alias,
			"card_no":     req.CardNo,
			"card_type":   req.CardType,
			"brand":       req.Brand,
			"vehicle_id":  req.VehicleID,
			"driver_id":   req.DriverID,
			"card_status": req.CardStatus,
			"station":     req.Station,
			"face_value":  req.FaceValue,
			"balance":     req.Balance,
			"enabled":     req.Enabled,
			"remark":      req.Remark,
			"updated_at":  now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update fleet fuel card failed: %v", err)
		c.Error(errors.NewInternalServerError("update fuel card failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetFuelCard godoc
// @Summary      删除油卡
// @Router       /fleet/fuel-cards/:id [delete]
func (h *FleetHandler) DeleteFleetFuelCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var refCnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("record_type = ? AND data->>'card_id' = ? AND deleted_at IS NULL", types.FleetRecordFuel, id).Count(&refCnt).Error; err != nil {
		logger.Errorf(ctx, "check fuel card record reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check fuel card reference failed"))
		return
	}
	if refCnt > 0 {
		c.Error(errors.NewBadRequestError("该油卡已有加油记录，不能删除"))
		return
	}
	res := h.db.WithContext(ctx).Model(&types.FleetFuelCard{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet fuel card failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete fuel card failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("油卡不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// 记录
// ---------------------------------------------------------------------------

// ListFleetRecords godoc
// @Summary      记录列表（按类型/月份/车辆筛选）
// @Router       /fleet/records [get]
func (h *FleetHandler) ListFleetRecords(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	recordType := strings.TrimSpace(c.Query("type"))
	month := strings.TrimSpace(c.Query("month"))
	vehicleID := strings.TrimSpace(c.Query("vehicle_id"))
	docType := strings.TrimSpace(c.Query("doc_type"))
	q := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if recordType != "" {
		q = q.Where("record_type = ?", recordType)
	}
	if month != "" {
		q = q.Where("record_month = ?", month)
	}
	if vehicleID != "" {
		q = q.Where("vehicle_id = ?", vehicleID)
	}
	if docType != "" {
		q = q.Where("doc_type = ?", docType)
	}
	var items []types.FleetRecord
	if err := q.Order("record_date DESC, created_at DESC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet records failed: %v", err)
		c.Error(errors.NewInternalServerError("list records failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// normalizeFleetRecord 校验并补齐记录通用字段。
func normalizeFleetRecord(req *types.FleetRecord) error {
	if req.RecordType == "" || !fleetRecordTypes[req.RecordType] {
		return errors.NewBadRequestError("record type 不合法")
	}
	// 档案类记录（证照解析）不强制绑定车辆
	if !strings.HasSuffix(req.RecordType, "-archive") && req.VehicleID == "" {
		return errors.NewBadRequestError("请选择车辆")
	}
	if req.Data == nil {
		req.Data = map[string]any{}
	}
	// 月份缺省时由记录日期推导
	if req.RecordMonth == "" {
		if len(req.RecordDate) >= 7 {
			req.RecordMonth = req.RecordDate[:7]
		}
	}
	// 清理 data 中无效空串（避免 JSONB 冗余）
	for k, v := range req.Data {
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			delete(req.Data, k)
		}
	}
	req.Remark = strings.TrimSpace(req.Remark)
	return nil
}

// CreateFleetRecord godoc
// @Summary      新增记录
// @Router       /fleet/records [post]
func (h *FleetHandler) CreateFleetRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if err := normalizeFleetRecord(&req); err != nil {
		c.Error(err)
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet record failed: %v", err)
		c.Error(errors.NewInternalServerError("create record failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetRecord godoc
// @Summary      更新记录
// @Router       /fleet/records/:id [put]
func (h *FleetHandler) UpdateFleetRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if err := normalizeFleetRecord(&req); err != nil {
		c.Error(err)
		return
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check fleet record failed: %v", err)
		c.Error(errors.NewInternalServerError("check record failed"))
		return
	}
	if cnt == 0 {
		c.Error(errors.NewNotFoundError("记录不存在"))
		return
	}
	dataJSON, _ := json.Marshal(req.Data)
	now := timeNowUTC()
	updates := map[string]interface{}{
		"record_type":      req.RecordType,
		"vehicle_id":       req.VehicleID,
		"record_month":     req.RecordMonth,
		"record_date":      req.RecordDate,
		"amount":           req.Amount,
		"mileage":          req.Mileage,
		"data":             string(dataJSON),
		"doc_type":         req.DocType,
		"remark":           req.Remark,
		"updated_at":       now,
	}
	// doc_knowledge_id / file_name 由提取落库维护：编辑保存时空值不得覆盖已有关联，
	// 否则会丢失源文件预览关联并破坏按 doc_knowledge_id 的幂等 upsert。
	if strings.TrimSpace(req.FileName) != "" {
		updates["file_name"] = req.FileName
	}
	if strings.TrimSpace(req.DocKnowledgeID) != "" {
		updates["doc_knowledge_id"] = req.DocKnowledgeID
	}
	if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(updates).Error; err != nil {
		logger.Errorf(ctx, "update fleet record failed: %v", err)
		c.Error(errors.NewInternalServerError("update record failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetRecord godoc
// @Summary      删除记录
// @Router       /fleet/records/:id [delete]
func (h *FleetHandler) DeleteFleetRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	res := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet record failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete record failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("记录不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// 费用汇总
// ---------------------------------------------------------------------------

type fleetSummaryRow struct {
	VehicleID     string  `json:"vehicle_id"`
	PlateNo       string  `json:"plate_no"`
	VehicleType   string  `json:"vehicle_type"`
	FuelAmount    float64 `json:"fuel_amount"`    // 加油充电
	MaintainAmount float64 `json:"maintain_amount"` // 维修保养费
	InsuranceAmount float64 `json:"insurance_amount"` // 保险理赔
	TireAmount    float64 `json:"tire_amount"`    // 轮胎
	MaterialAmount float64 `json:"material_amount"` // 辅材
	ViolationAmount float64 `json:"violation_amount"` // 违章罚款
	TollAmount    float64 `json:"toll_amount"`    // 路桥费
	TotalAmount   float64 `json:"total_amount"`   // 合计
	RecordCount   int     `json:"record_count"`
}

// GetFleetSummary godoc
// @Summary      按车辆月度费用汇总
// @Router       /fleet/summary [get]
func (h *FleetHandler) GetFleetSummary(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	month := strings.TrimSpace(c.Query("month"))
	vehicleID := strings.TrimSpace(c.Query("vehicle_id"))
	if month == "" {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}

	type agg struct {
		VehicleID  string
		RecordType string
		SumAmount  float64
		Count      int
	}
	var rows []agg
	q := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
		Select("vehicle_id, record_type, SUM(amount) AS sum_amount, COUNT(*) AS count").
		Where("tenant_id = ? AND record_month = ? AND deleted_at IS NULL", tenantID, month)
	if vehicleID != "" {
		q = q.Where("vehicle_id = ?", vehicleID)
	}
	if err := q.Group("vehicle_id, record_type").Scan(&rows).Error; err != nil {
		logger.Errorf(ctx, "fleet summary aggregate failed: %v", err)
		c.Error(errors.NewInternalServerError("summary failed"))
		return
	}

	// 车辆信息（含软删车辆，保证历史汇总可查）
	var vehicles []types.FleetVehicle
	h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&vehicles)
	vMap := map[string]types.FleetVehicle{}
	for _, v := range vehicles {
		vMap[v.ID] = v
	}
	vehicleOrder := []string{}
	for _, r := range rows {
		if _, ok := vMap[r.VehicleID]; ok {
			vehicleOrder = append(vehicleOrder, r.VehicleID)
		}
	}
	// 按车辆 sort_order 排序
	sort.SliceStable(vehicleOrder, func(i, j int) bool {
		a, b := vMap[vehicleOrder[i]], vMap[vehicleOrder[j]]
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})

	rowMap := map[string]*fleetSummaryRow{}
	for _, vid := range vehicleOrder {
		v := vMap[vid]
		rowMap[vid] = &fleetSummaryRow{
			VehicleID:   vid,
			PlateNo:     v.PlateNo,
			VehicleType: v.VehicleType,
		}
	}
	for _, r := range rows {
		row, ok := rowMap[r.VehicleID]
		if !ok {
			continue
		}
		row.RecordCount += r.Count
		switch r.RecordType {
		case types.FleetRecordFuelCharge, types.FleetRecordFuel:
			row.FuelAmount = r.SumAmount
		case types.FleetRecordRepairCost, types.FleetRecordMaintain:
			row.MaintainAmount = r.SumAmount
		case types.FleetRecordInsuranceClaim, types.FleetRecordInsurance:
			row.InsuranceAmount = r.SumAmount
		case types.FleetRecordTire:
			row.TireAmount = r.SumAmount
		case types.FleetRecordMaterial:
			row.MaterialAmount = r.SumAmount
		case types.FleetRecordViolation:
			row.ViolationAmount = r.SumAmount
		case types.FleetRecordRoadToll, types.FleetRecordToll:
			row.TollAmount = r.SumAmount
		}
	}
	result := make([]*fleetSummaryRow, 0, len(vehicleOrder))
	for _, vid := range vehicleOrder {
		row := rowMap[vid]
		row.TotalAmount = row.FuelAmount + row.MaintainAmount + row.InsuranceAmount + row.TireAmount +
			row.MaterialAmount + row.ViolationAmount + row.TollAmount
		result = append(result, row)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ---------------------------------------------------------------------------
// 档案分类配置（大项-小项）：scope=vehicle|driver|maintain
// ---------------------------------------------------------------------------

var fleetCategoryScopes = map[string]bool{
	types.FleetCategoryScopeVehicle:  true,
	types.FleetCategoryScopeDriver:   true,
	types.FleetCategoryScopeMaintain: true,
}

// ListFleetCategories godoc
// @Summary      档案分类列表
// @Router       /fleet/categories [get]
func (h *FleetHandler) ListFleetCategories(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	scope := strings.TrimSpace(c.Query("scope"))
	q := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if scope != "" {
		if !fleetCategoryScopes[scope] {
			c.Error(errors.NewBadRequestError("scope 不合法"))
			return
		}
		q = q.Where("scope = ?", scope)
	}
	var items []types.FleetCategory
	if err := q.Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet categories failed: %v", err)
		c.Error(errors.NewInternalServerError("list categories failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetCategory godoc
// @Summary      新增档案分类（大项）
// @Router       /fleet/categories [post]
func (h *FleetHandler) CreateFleetCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetCategory
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if !fleetCategoryScopes[req.Scope] {
		c.Error(errors.NewBadRequestError("scope 不合法"))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("分类名称不能为空"))
		return
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	if req.GroupID != "" {
		var grp types.FleetCertGroup
		if err := h.db.WithContext(ctx).
			Where("id = ? AND tenant_id = ? AND parent_scope = ? AND deleted_at IS NULL", req.GroupID, tenantID, req.Scope).
			First(&grp).Error; err != nil {
			c.Error(errors.NewBadRequestError("证照分组不存在"))
			return
		}
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	if req.Subs == nil {
		req.Subs = []types.FleetCategorySub{}
	}
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = ? AND scope = ? AND deleted_at IS NULL", tenantID, req.Scope).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet category failed: %v", err)
		c.Error(errors.NewInternalServerError("create category failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetCategory godoc
// @Summary      更新档案分类（名称/小项启用）
// @Router       /fleet/categories/:id [put]
func (h *FleetHandler) UpdateFleetCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetCategory
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if req.Name != "" {
		req.Name = strings.TrimSpace(req.Name)
	}
	now := timeNowUTC()
	dataJSON, _ := json.Marshal(req.Subs)
	updates := map[string]interface{}{
		"enabled":    req.Enabled,
		"updated_at": now,
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Subs != nil {
		updates["subs"] = string(dataJSON)
	}
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(updates).Error; err != nil {
		logger.Errorf(ctx, "update fleet category failed: %v", err)
		c.Error(errors.NewInternalServerError("update category failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// SortFleetCategories godoc
// @Summary      档案分类排序
// @Router       /fleet/categories/sort [put]
func (h *FleetHandler) SortFleetCategories(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req struct {
		Scope string   `json:"scope"`
		IDs   []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	for i, id := range req.IDs {
		h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Update("sort_order", i+1)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetCategory godoc
// @Summary      删除档案分类
// @Router       /fleet/categories/:id [delete]
func (h *FleetHandler) DeleteFleetCategory(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	res := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet category failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete category failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("分类不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// 证照自定义分组
// ---------------------------------------------------------------------------

// ListFleetCertGroups godoc
// @Summary      证照分组列表
// @Router       /fleet/cert-groups [get]
func (h *FleetHandler) ListFleetCertGroups(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	parentScope := strings.TrimSpace(c.Query("parent_scope"))
	q := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if parentScope != "" {
		q = q.Where("parent_scope = ?", parentScope)
	}
	var items []types.FleetCertGroup
	if err := q.Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet cert groups failed: %v", err)
		c.Error(errors.NewInternalServerError("list cert groups failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetCertGroup godoc
// @Summary      新增证照分组
// @Router       /fleet/cert-groups [post]
func (h *FleetHandler) CreateFleetCertGroup(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetCertGroup
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.ID = secutils.SanitizeForLog(req.ID)
	if req.ID == "" {
		req.ID = "cg-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	}
	req.TenantID = int64(tenantID)
	req.Builtin = false
	if req.ParentScope == "" {
		req.ParentScope = "vehicle"
	}
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet cert group failed: %v", err)
		c.Error(errors.NewInternalServerError("create cert group failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetCertGroup godoc
// @Summary      更新证照分组
// @Router       /fleet/cert-groups/:id [put]
func (h *FleetHandler) UpdateFleetCertGroup(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetCertGroup
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	updates := map[string]any{"name": req.Name, "sort_order": req.SortOrder}
	res := h.db.WithContext(ctx).Model(&types.FleetCertGroup{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(updates)
	if res.Error != nil {
		logger.Errorf(ctx, "update fleet cert group failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("update cert group failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("分组不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已更新"})
}

// DeleteFleetCertGroup godoc
// @Summary      删除证照分组
// @Router       /fleet/cert-groups/:id [delete]
func (h *FleetHandler) DeleteFleetCertGroup(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	// 引用判定：该分组下还有分类则禁止删除
	var catCount int64
	h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("group_id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&catCount)
	if catCount > 0 {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("分组下还有%d个证照类型，请先移除或重新归类", catCount)))
		return
	}
	res := h.db.WithContext(ctx).Model(&types.FleetCertGroup{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet cert group failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete cert group failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("分组不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// 供应商
// ---------------------------------------------------------------------------

// ListFleetSuppliers godoc
// @Summary      供应商列表
// @Router       /fleet/suppliers [get]
func (h *FleetHandler) ListFleetSuppliers(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetSupplier
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet suppliers failed: %v", err)
		c.Error(errors.NewInternalServerError("list suppliers failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetSupplier godoc
// @Summary      新增供应商
// @Router       /fleet/suppliers [post]
func (h *FleetHandler) CreateFleetSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetSupplier
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.Error(errors.NewBadRequestError("供应商名称不能为空"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetSupplier{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet supplier failed: %v", err)
		c.Error(errors.NewInternalServerError("create supplier failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetSupplier godoc
// @Summary      更新供应商
// @Router       /fleet/suppliers/:id [put]
func (h *FleetHandler) UpdateFleetSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetSupplier
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.FleetSupplier{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]interface{}{
			"name":               req.Name,
			"supplier_type":      req.SupplierType,
			"qualification":      req.Qualification,
			"credit_code":        req.CreditCode,
			"legal_person":       req.LegalPerson,
			"contact":            req.Contact,
			"phone":              req.Phone,
			"address":            req.Address,
			"cooperation_status": req.CooperationStatus,
			"coop_start_date":    req.CoopStartDate,
			"settle_method":      req.SettleMethod,
			"tax_rate":           req.TaxRate,
			"invoice_type":       req.InvoiceType,
			"status":             req.Status,
			"enabled":            req.Enabled,
			"remark":             req.Remark,
			"updated_at":         now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update fleet supplier failed: %v", err)
		c.Error(errors.NewInternalServerError("update supplier failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetSupplier godoc
// @Summary      删除供应商
// @Router       /fleet/suppliers/:id [delete]
func (h *FleetHandler) DeleteFleetSupplier(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	res := h.db.WithContext(ctx).Model(&types.FleetSupplier{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet supplier failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete supplier failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("供应商不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// ETC 卡
// ---------------------------------------------------------------------------

// ListFleetETCCards godoc
// @Summary      ETC卡列表
// @Router       /fleet/etc-cards [get]
func (h *FleetHandler) ListFleetETCCards(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetETCCard
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet etc cards failed: %v", err)
		c.Error(errors.NewInternalServerError("list etc cards failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// CreateFleetETCCard godoc
// @Summary      新增ETC卡
// @Router       /fleet/etc-cards [post]
func (h *FleetHandler) CreateFleetETCCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req types.FleetETCCard
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.CardNo = strings.TrimSpace(req.CardNo)
	if req.CardNo == "" {
		c.Error(errors.NewBadRequestError("ETC卡号不能为空"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetETCCard{}).
		Where("tenant_id = ? AND deleted_at IS NULL", tenantID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	req.SortOrder = maxOrder + 1
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create fleet etc card failed: %v", err)
		c.Error(errors.NewInternalServerError("create etc card failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateFleetETCCard godoc
// @Summary      更新ETC卡
// @Router       /fleet/etc-cards/:id [put]
func (h *FleetHandler) UpdateFleetETCCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.FleetETCCard
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.FleetETCCard{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Updates(map[string]interface{}{
			"alias":       req.Alias,
			"card_no":     req.CardNo,
			"card_type":   req.CardType,
			"issuer":      req.Issuer,
			"bank":        req.Bank,
			"open_date":   req.OpenDate,
			"expire_date": req.ExpireDate,
			"vehicle_id":  req.VehicleID,
			"card_status": req.CardStatus,
			"enabled":     req.Enabled,
			"remark":      req.Remark,
			"updated_at":  now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update fleet etc card failed: %v", err)
		c.Error(errors.NewInternalServerError("update etc card failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteFleetETCCard godoc
// @Summary      删除ETC卡
// @Router       /fleet/etc-cards/:id [delete]
func (h *FleetHandler) DeleteFleetETCCard(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	res := h.db.WithContext(ctx).Model(&types.FleetETCCard{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete fleet etc card failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete etc card failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("ETC卡不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

var _ = time.Now
