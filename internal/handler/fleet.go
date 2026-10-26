package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
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
	if err := db.AutoMigrate(&types.FleetCertGroup{}, &types.FleetCategory{}, &types.FleetGroupAlias{}); err != nil {
		logger.Warnf(context.Background(), "AutoMigrate fleet_cert_groups failed: %v", err)
	}
	h.seedBusinessCategories()
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
		"record_type":  req.RecordType,
		"vehicle_id":   req.VehicleID,
		"record_month": req.RecordMonth,
		"record_date":  req.RecordDate,
		"amount":       req.Amount,
		"mileage":      req.Mileage,
		"data":         string(dataJSON),
		"doc_type":     req.DocType,
		"remark":       req.Remark,
		"updated_at":   now,
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
	VehicleID       string  `json:"vehicle_id"`
	PlateNo         string  `json:"plate_no"`
	VehicleType     string  `json:"vehicle_type"`
	FuelAmount      float64 `json:"fuel_amount"`      // 加油充电
	MaintainAmount  float64 `json:"maintain_amount"`  // 维修保养费
	InsuranceAmount float64 `json:"insurance_amount"` // 保险理赔
	TireAmount      float64 `json:"tire_amount"`      // 轮胎
	MaterialAmount  float64 `json:"material_amount"`  // 辅材
	ViolationAmount float64 `json:"violation_amount"` // 违章罚款
	TollAmount      float64 `json:"toll_amount"`      // 路桥费
	TotalAmount     float64 `json:"total_amount"`     // 合计
	RecordCount     int     `json:"record_count"`
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

// isBusinessCategoryScope 判断是否为日常事务业务 scope（分类配置全局 tenant_id=0，不按 tenant 隔离）
func isBusinessCategoryScope(scope string) bool {
	return scope == "contract" || scope == "invoice" || scope == "regulation" ||
		scope == "award_punish" || scope == "energy"
}

// invoice 系统字段：列表/编辑抽屉/六同步依赖的列（提取状态、标签、票据类型）。
// 新建/复制发票分类时自动补齐，避免自定义分类缺失这三列导致渲染不齐。
var invoiceSystemFields = []types.FleetCategorySub{
	{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text", Builtin: true},
	{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text", Builtin: true},
	{Name: "tags", Enabled: true, IsDefault: true, DataType: "array", Builtin: true},
}

// ensureInvoiceSystemFields 保证发票分类 subs 始终包含系统字段（按 name 去重，保留用户已有定义）
func ensureInvoiceSystemFields(subs []types.FleetCategorySub) []types.FleetCategorySub {
	if subs == nil {
		subs = []types.FleetCategorySub{}
	}
	have := make(map[string]bool, len(subs))
	for _, s := range subs {
		have[s.Name] = true
	}
	for _, sys := range invoiceSystemFields {
		if !have[sys.Name] {
			subs = append(subs, sys)
		}
	}
	return subs
}

// invoiceDefaultStandardSubs 新建发票分类的默认字段集。
//
// 用户口径：新建分类不能是一张白纸——至少要有发票号码/日期/金额/价税合计等标准字段，
// 且「提取状态」「标签」必须默认启用（列表、筛选器、六同步都依赖这两列）。
// 与内置「普通发票」seed 保持同一份清单，避免新建分类与内置分类表现不一致。
//
// **只有 3 个系统字段标记 Builtin**（发票类型/提取状态/标签）：它们是列表渲染、
// 筛选器与六同步的硬依赖，删掉会让整条链路静默失效。其余标准字段（发票号码/金额…）
// 与扩展字段、明细子项一律由用户自由增删——用户口径：内置字段仅限这 3 个。
func invoiceDefaultStandardSubs() []types.FleetCategorySub {
	subs := []types.FleetCategorySub{
		{Name: "invoiceNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "invoiceDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "taxRate", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "tax", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "totalAmount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "buyerName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "sellerName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "issuer", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
		{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		{Name: "sellerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "buyerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "items", Enabled: true, IsDefault: true, DataType: "items", SubFields: invoiceItemSubFieldsStandard()},
		{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
	}
	markBuiltinFields(subs, invoiceSystemFields)
	return subs
}

var fleetCategoryScopes = map[string]bool{
	types.FleetCategoryScopeVehicle:  true,
	types.FleetCategoryScopeDriver:   true,
	types.FleetCategoryScopeMaintain: true,
	// P2-D: 日常事务业务模块（单大类，subs 即列表列配置）
	"contract":     true,
	"invoice":      true,
	"regulation":   true,
	"award_punish": true,
	"energy":       true,
}

// ListFleetCategories godoc
// @Summary      档案分类列表
// @Router       /fleet/categories [get]
func (h *FleetHandler) ListFleetCategories(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	scope := strings.TrimSpace(c.Query("scope"))
	q := h.db.WithContext(ctx).Where("deleted_at IS NULL")
	// P2-D: 日常事务业务模块（contract/invoice/...）使用全局 tenant_id=0 配置，不按 tenant 隔离
	if isBusinessCategoryScope(scope) {
		q = q.Where("tenant_id = 0")
	} else {
		q = q.Where("tenant_id = ?", tenantID)
	}
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
	// P2-D 一致：日常事务业务 scope 分类使用全局 tenant_id=0 配置（List/Update 同口径），
	// 否则新建/复制的分类重开列表时被 tenant 过滤而"消失"
	if isBusinessCategoryScope(req.Scope) {
		req.TenantID = 0
	} else {
		req.TenantID = int64(tenantID)
	}
	req.CreatedAt = now
	req.UpdatedAt = now
	if req.Subs == nil {
		req.Subs = []types.FleetCategorySub{}
	}
	// invoice scope 自动补齐系统字段（invoiceType/extractStatus/tags），
	// 保证新建/复制分类的列表、编辑抽屉、六同步立即可用
	if req.Scope == "invoice" {
		// 用户口径：新建分类直接给一套默认标准字段（含默认启用的提取状态/标签），
		// 而不是一张白纸——否则列表、筛选器、明细区在用户配字段之前全是空列。
		if len(req.Subs) == 0 {
			req.Subs = invoiceDefaultStandardSubs()
		}
		req.Subs = ensureInvoiceSystemFields(req.Subs)
		// 明细子列去重（同义键 tax/tax_amount、price/unit_price… 只留一条）
		req.Subs, _ = normalizeInvoiceCategorySubs(req.Scope, req.Subs)
	}
	queryTenant := tenantID
	if isBusinessCategoryScope(req.Scope) {
		queryTenant = 0
	}
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = ? AND scope = ? AND deleted_at IS NULL", queryTenant, req.Scope).
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
	// 日常事务业务 scope 分类使用全局 tenant_id=0 配置，不按 tenant 隔离（与 List 口径一致）
	var existing types.FleetCategory
	// 需要原名字做改名判定与传播起点，一并查出来
	if err := h.db.WithContext(ctx).Select("scope", "name").Where("id = ? AND deleted_at IS NULL", id).First(&existing).Error; err != nil {
		c.Error(errors.NewNotFoundError("分类不存在"))
		return
	}
	if isBusinessCategoryScope(existing.Scope) {
		tenantID = 0
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	var req types.FleetCategory
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
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
	// 缺失字段保护：请求体未显式传 enabled 时保留库中原值，
	// 防止「仅更新 subs/name」的局部 PUT 被 Go bool 零值(false)误关分类
	var reqRaw struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(bodyBytes, &reqRaw); err == nil && reqRaw.Enabled != nil {
		updates["enabled"] = *reqRaw.Enabled
	}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Subs != nil {
		// 列表系统列（fileName）不进 subs：它是文件属性，由列表侧显式渲染。
		// 前端旧版本/旧缓存可能仍把它提交上来，这里统一剥离，保证 subs 只含提取字段。
		req.Subs = stripListSystemSubs(req.Subs)
		// 明细子列去重：用户/AI 可能保存出同义子项（tax 与 tax_amount 都是「税额」），
		// 落库前统一收敛，避免明细浮层出现两列同名。
		if normalized, _ := normalizeInvoiceCategorySubs(existing.Scope, req.Subs); normalized != nil {
			req.Subs = normalized
			if raw, merr := json.Marshal(req.Subs); merr == nil {
				dataJSON = raw
			}
		}
		updates["subs"] = string(dataJSON)
	}
	// 改名传播起点：仅当名字真的变了才触发。分类与数据之间是分类名字符串关联
	// （custom_metadata 4 键 + knowledge_extract_configs.cert_type），只改
	// fleet_categories.name 会让存量票据全部掉进「其它票据」桶而新分类显示 0 张
	// （2026-10-20 回归）。传播放在与分类更新同一事务里：任一环节失败整体回滚，
	// 绝不留下「分类改了但票没改」的半截状态。
	oldName := strings.TrimSpace(existing.Name)
	needPropagate := req.Name != "" && req.Name != oldName
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if needPropagate {
			if err := h.propagateCategoryRename(ctx, tx, existing.Scope, oldName, req.Name); err != nil {
				return err
			}
		}
		return nil
	})
	switch {
	case stderrors.Is(err, gorm.ErrRecordNotFound):
		c.Error(errors.NewNotFoundError("分类不存在"))
		return
	case stderrors.Is(err, ErrRenameCertTypeConflict):
		// 目标知识库下已有同名提取配置：改名会产生两份配置、读取侧取哪份不可预期。
		// 已整体回滚，分类名保持原值；提示用户先处理目标分类的规则配置。
		c.Error(errors.NewConflictError(
			fmt.Sprintf("该知识库下已存在「%s」的提取规则配置，请先在提取规则中处理该分类后再改名", req.Name)))
		return
	case err != nil:
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
	// 日常事务业务 scope 分类使用全局 tenant_id=0 配置，不按 tenant 隔离（与 List 口径一致）
	var existing types.FleetCategory
	if err := h.db.WithContext(ctx).Select("scope, name, builtin_key").Where("id = ? AND deleted_at IS NULL", id).First(&existing).Error; err != nil {
		c.Error(errors.NewNotFoundError("分类不存在"))
		return
	}
	if isBusinessCategoryScope(existing.Scope) {
		tenantID = 0
	}
	// 内置分类（builtin_key 非空）禁止删除
	if existing.BuiltinKey != "" {
		c.Error(errors.NewBadRequestError("内置分类不可删除"))
		return
	}
	// 引用检查：分类名下已有数据记录时禁止删除，防止档案数据失联
	if refCount, err := h.countFleetCategoryRefs(ctx, existing.Scope, existing.Name); err != nil {
		logger.Errorf(ctx, "count fleet category refs failed: %v", err)
		c.Error(errors.NewInternalServerError("check category references failed"))
		return
	} else if refCount > 0 {
		c.Error(errors.NewConflictError(fmt.Sprintf("该分类下已有 %d 条记录引用，禁止删除", refCount)))
		return
	}
	// 分类软删与该分类名下的提取配置清理必须在同一事务：任一失败都回滚，
	// 否则会留下「分类没了、配置还在」的孤儿，或「分类删了、配置没删」的脏态。
	var cleanedCfg int64
	now := timeNowUTC()
	txErr := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&types.FleetCategory{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Update("deleted_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errFleetCategoryGone
		}
		// 级联清理该分类名对应的提取配置（knowledge_extract_configs 按
		// scope + cert_type=分类名 建键）。不清理的话，每删一次分类就留一条
		// 孤儿配置：同名分类日后重建时会直接读到上一轮的旧规则/旧字段，
		// 用户以为「新建的分类」其实继承了已删分类的配置。
		// 用软删而非物理删：与分类本身一致，误删可回滚；整体规则
		//（cert_type 为空）与其它分类的配置不受影响。
		n, cerr := softDeleteExtractConfigsOfCategoryTx(
			ctx, tx, existing.Scope, existing.Name, tenantID, isBusinessCategoryScope(existing.Scope), now)
		if cerr != nil {
			return cerr
		}
		cleanedCfg = n
		return nil
	})
	if txErr != nil {
		if stderrors.Is(txErr, errFleetCategoryGone) {
			c.Error(errors.NewNotFoundError("分类不存在"))
			return
		}
		logger.Errorf(ctx, "delete fleet category failed: %v", txErr)
		c.Error(errors.NewInternalServerError("delete category failed"))
		return
	}
	if cleanedCfg > 0 {
		logger.Infof(ctx, "cascade deleted %d extract config(s) for removed category %q (scope=%s)", cleanedCfg, existing.Name, existing.Scope)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// errFleetCategoryGone 表示删除时目标分类已不存在（并发删除 / 租户不匹配）。
var errFleetCategoryGone = stderrors.New("fleet category not found")

// extractConfigScopeForCategory 把分类 scope 映射到提取配置的 scope。
// notice 与 award_punish 共用同一份字段目录与配置键（保存时 notice 会被归一成
// award_punish），历史分类行若仍写 notice，清理时也要落到 award_punish 上。
func extractConfigScopeForCategory(scope string) string {
	if scope == "notice" {
		return "award_punish"
	}
	return scope
}

// softDeleteExtractConfigsOfCategoryTx 在同一事务内软删
// (scope, cert_type=分类名) 的提取配置，返回清理行数。
// cert_type 为空的行是 KB 级整体规则，不属于任何分类，必须保留。
//
// 租户口径与分类本身一致：日常事务四个业务 scope 的分类是全局行
// （tenant_id=0，各 KB 共用一套分类），配置也按 (scope, cert_type) 全局清理；
// 车队档案 scope 的分类按租户隔离，清理必须带 tenant_id，否则同名分类在别的
// 租户下会被连带删除。
func softDeleteExtractConfigsOfCategoryTx(
	ctx context.Context, tx *gorm.DB, scope, name string, tenantID uint64, global bool, now time.Time,
) (int64, error) {
	cfgScope := extractConfigScopeForCategory(scope)
	name = strings.TrimSpace(name)
	if cfgScope == "" || name == "" {
		return 0, nil
	}
	q := tx.WithContext(ctx).Model(&types.KbExtractConfig{}).
		Where("scope = ? AND cert_type = ? AND deleted_at IS NULL", cfgScope, name)
	if !global {
		q = q.Where("tenant_id = ?", int64(tenantID))
	}
	res := q.Update("deleted_at", now)
	return res.RowsAffected, res.Error
}

// countFleetCategoryRefs 统计分类名下被数据记录引用的数量：
//   - invoice：knowledge.custom_metadata->'invoices' 数组中 invoice_type == 分类名
//   - contract/regulation/award_punish：knowledge.custom_metadata->>'category' == 分类名
//   - vehicle/driver/maintain：fleet_records.doc_type == 分类名
//
// 先用 LIKE 粗筛缩小候选集，再在 Go 侧精确解析 JSON，避免依赖具体数据库 JSON 方言。
func (h *FleetHandler) countFleetCategoryRefs(ctx context.Context, scope, name string) (int, error) {
	if strings.TrimSpace(name) == "" {
		return 0, nil
	}
	switch scope {
	case "invoice":
		var rows []types.Knowledge
		if err := h.db.WithContext(ctx).
			Where("deleted_at IS NULL AND CAST(custom_metadata AS TEXT) LIKE ?", "%"+name+"%").
			Find(&rows).Error; err != nil {
			return 0, err
		}
		count := 0
		for i := range rows {
			var meta struct {
				Invoices []struct {
					InvoiceType string `json:"invoice_type"`
				} `json:"invoices"`
			}
			if len(rows[i].CustomMetadata) == 0 {
				continue
			}
			if err := json.Unmarshal(rows[i].CustomMetadata, &meta); err != nil {
				continue
			}
			for _, inv := range meta.Invoices {
				if inv.InvoiceType == name {
					count++
				}
			}
		}
		return count, nil
	case "contract", "regulation", "award_punish":
		var rows []types.Knowledge
		if err := h.db.WithContext(ctx).
			Where("deleted_at IS NULL AND CAST(custom_metadata AS TEXT) LIKE ?", "%"+name+"%").
			Find(&rows).Error; err != nil {
			return 0, err
		}
		count := 0
		for i := range rows {
			var meta struct {
				Category string `json:"category"`
			}
			if len(rows[i].CustomMetadata) == 0 {
				continue
			}
			if err := json.Unmarshal(rows[i].CustomMetadata, &meta); err != nil {
				continue
			}
			if meta.Category == name {
				count++
			}
		}
		return count, nil
	default:
		// 车队档案：fleet_records.doc_type 关联分类名
		var count int64
		if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("deleted_at IS NULL AND doc_type = ?", name).
			Count(&count).Error; err != nil {
			return 0, err
		}
		return int(count), nil
	}
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

// ListFleetGroupAliases godoc
// @Summary      内置分组显示别名列表
// @Router       /fleet/group-alias [get]
func (h *FleetHandler) ListFleetGroupAliases(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var items []types.FleetGroupAlias
	if err := h.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&items).Error; err != nil {
		logger.Errorf(ctx, "list fleet group alias failed: %v", err)
		c.Error(errors.NewInternalServerError("list group alias failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// UpsertFleetGroupAlias godoc
// @Summary      设置内置分组显示别名（upsert）
// @Router       /fleet/group-alias [put]
func (h *FleetHandler) UpsertFleetGroupAlias(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := fleetTenantID(c)
	var req struct {
		Scope    string `json:"scope"`
		GroupKey string `json:"group_key"`
		Name     string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Scope = strings.TrimSpace(req.Scope)
	req.GroupKey = strings.TrimSpace(req.GroupKey)
	req.Name = strings.TrimSpace(req.Name)
	if req.Scope == "" || req.GroupKey == "" {
		c.Error(errors.NewBadRequestError("scope 与 group_key 不能为空"))
		return
	}
	var row types.FleetGroupAlias
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND group_key = ?", tenantID, req.Scope, req.GroupKey).
		First(&row).Error
	if err != nil {
		row = types.FleetGroupAlias{
			ID:       "ga-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12],
			TenantID: int64(tenantID),
			Scope:    req.Scope,
			GroupKey: req.GroupKey,
			Name:     req.Name,
		}
		if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
			logger.Errorf(ctx, "create fleet group alias failed: %v", err)
			c.Error(errors.NewInternalServerError("create group alias failed"))
			return
		}
	} else {
		if err := h.db.WithContext(ctx).Model(&row).Update("name", req.Name).Error; err != nil {
			logger.Errorf(ctx, "update fleet group alias failed: %v", err)
			c.Error(errors.NewInternalServerError("update group alias failed"))
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": row})
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

// contractSeedSubs 合同内置分类（服务合同/租赁合同）字段全集。
// 2026-10-21 字段体系重置：与后端 contractCatalog() 严格同序 —— **11 标准 + 15 扩展**，
// 与前端 DEFAULT_CONTRACT_COLUMNS、编辑抽屉表单同源（单测锁定）。
// 旧键（partyAName/expiryDate/contractAmount…）不再进入 seed；存量 subs 由
// mergeContractSeedSubs 的旧键→契约键归一收敛（保留用户 enabled/is_default/width 调整）。
// 历史字段（税率/质保金等）也不再入 seed：新提取结果本就是新契约键，
// 老环境的旧键 subs 经归一后保留（contractCanonicalFieldKey 直通 snake 形态）。
func contractSeedSubs() []types.FleetCategorySub {
	return []types.FleetCategorySub{
		// ---- 标准 11（默认显示）----
		{Name: "contract_no", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "contract_name", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "contract_type", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "party_a", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "party_b", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "sign_date", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "expire_date", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "extract_status", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "performance_status", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		// ---- 扩展 15（默认不显示）----
		{Name: "party_a_contact", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_a_tax_no", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_contact", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_tax_no", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_a_address", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_a_phone", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_a_bank", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_a_account", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_address", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_phone", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_bank", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "party_b_account", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "payment_method", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "sign_location", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "summary", Enabled: true, IsDefault: false, DataType: "textarea"},
		// 注意：seed 里**不放** fileName。它是文件属性（列表系统列），不是提取字段；
		// seed 进去会让它渗进字段定义面板、编辑抽屉与提取规则表，成为用户看不到来历的
		// 幽灵字段（2026-10-21 真机验收）。列表侧的「文件名」列由前端
		// appendListSystemColumns 显式追加，不依赖 subs。
	}
}

// businessListSystemKeys 列表系统列：由文件属性提供、**不是提取字段**的键。
//
// fileName 曾被 seed 进 fleet_categories.subs，于是一路渗进「字段定义」面板、
// 编辑抽屉（末尾冒出「文件名称」输入框）与提取规则表——用户在字段定义里找不到
// 它的来历，看到的就是一个幽灵字段（2026-10-21 真机验收）。它也不该被模型提取：
// 文件名来自上传元数据，LLM 提取它只会得到臆造值。
//
// 现在的口径：
//
//	· subs / 提取规则 / 编辑抽屉 → 一律不含系统列（mergeXxxSeedSubs 会剥离存量）；
//	· 列表 → 由前端 appendListSystemColumns 显式追加「文件名」列。
var businessListSystemKeys = map[string]bool{"fileName": true}

// isBusinessListSystemKey 该 subs 键是否列表系统列（不参与提取与字段定义）。
func isBusinessListSystemKey(name string) bool {
	return businessListSystemKeys[strings.TrimSpace(name)]
}

// stripListSystemSubs 从 subs 中剥离列表系统列（幂等：没有则原样返回）。
// 用于 mergeXxxSeedSubs 的收口，让存量库里 seed 出来的 fileName 在下次启动时被清掉。
func stripListSystemSubs(subs []types.FleetCategorySub) []types.FleetCategorySub {
	if len(subs) == 0 {
		return subs
	}
	out := make([]types.FleetCategorySub, 0, len(subs))
	for _, s := range subs {
		if isBusinessListSystemKey(s.Name) {
			continue
		}
		out = append(out, s)
	}
	return out
}

// contractCanonicalFieldKey 把合同 subs 的字段键归一到契约键（snake_case）：
// 旧键（partyAName/expiryDate/contractAmount/signPlace/fulfillStatus 及 snake 旧形态）
// 经别名表映射；其余 camelCase 走 camel→snake。与前端 contractKeyOf / 后端
// contractLegacyFieldAliases 保持同一口径（三处同源，单测锁定）。
func contractCanonicalFieldKey(name string) string {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return raw
	}
	switch raw {
	case "partyAName", "party_a_name":
		return "party_a"
	case "partyBName", "party_b_name":
		return "party_b"
	case "expiryDate", "expiry_date":
		return "expire_date"
	case "contractAmount", "contract_amount":
		return "amount"
	case "signPlace", "sign_place":
		return "sign_location"
	case "fulfillStatus", "fulfill_status":
		return "performance_status"
	}
	return service.SnakeKey(raw)
}

// mergeContractSeedSubs 把存量 subs 收敛到契约键 seed 全集，并**按契约顺序重排**：
//  1. 旧键名归一（partyAName → party_a），保留用户的 enabled/is_default/width/data_type/sub_fields；
//  2. 顺序以 seed 为准（11 标准 → 15 扩展 → fileName），抽屉/列表/字段面板三处同序；
//  3. seed 以外的历史字段（税率/质保金/违约金/合同标的/中文名旧键等）不再进入展示字段集：
//     它们的值仍保留在 contracts[] JSON 里，需要时可在「字段设置」重新加回。
//
// 为什么必须重排：抽屉字段顺序 = subs 顺序（buildDetailFieldDefs 透传）。历史 subs 里
// 「税率」夹在金额与提取状态之间、扩展字段散落在末尾，会让抽屉顺序与产品清单不一致。
func mergeContractSeedSubs(existing []types.FleetCategorySub) []types.FleetCategorySub {
	seed := contractSeedSubs()
	byKey := make(map[string]types.FleetCategorySub, len(existing))
	for _, s := range existing {
		name := contractCanonicalFieldKey(s.Name)
		if name == "" {
			continue
		}
		if _, dup := byKey[name]; dup {
			continue
		}
		s.Name = name
		byKey[name] = s
	}
	out := make([]types.FleetCategorySub, 0, len(seed))
	for _, f := range seed {
		if s, ok := byKey[f.Name]; ok {
			out = append(out, s)
			continue
		}
		out = append(out, f)
	}
	// 剥离列表系统列（fileName）：存量库里的 fileName 是旧 seed 写入的，
	// 必须在这里收口——否则每次启动都会把用户删掉的「文件名称」字段补回抽屉。
	return stripListSystemSubs(out)
}

// regulationSeedSubs 为制度内置分类（人事/财务/生产/行政/安全/其它制度）的字段全集。
// subs.name 为前端 RegulationRow 的 camelCase key，与 types.RegulationExtractionItem
// JSON 字段同源；is_default 与前端 DEFAULT_REGULATION_COLUMNS 的 default 一致。
func regulationSeedSubs() []types.FleetCategorySub {
	return []types.FleetCategorySub{
		// 制度信息（默认显示）
		{Name: "regNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "regName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "regType", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "version", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "pageCount", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "issueDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "effectiveDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "scope", Enabled: true, IsDefault: true, DataType: "text"},
		// 制度内容（默认不显示）
		{Name: "dept", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "modifyCount", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "compiledBy", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "reviewedBy", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "approvedBy", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "summary", Enabled: true, IsDefault: false, DataType: "textarea"},
		{Name: "confidentiality", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
		// 系统字段
		{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		// 同上：fileName 是列表系统列，不进提取字段目录。
	}
}

// noticeSeedSubs 为通知公告内置分类（通知/公告/通报/决定/其它通知）的字段全集。
// subs.name 为前端 NoticeRow 的 camelCase key（snakeOf 后与后端 contractCatalog 同目录的
// 契约键一致）；is_default 与前端 DEFAULT_NOTICE_COLUMNS 的 default 一致。
// 2026-10-21 与后端 noticeCatalog()（notice 与 award_punish 共用同一目录）对齐：
// 标准 9 + 扩展 10；dept/position 退出 seed（不在目录内，老环境 subs 仍保留可显示）。
func noticeSeedSubs() []types.FleetCategorySub {
	return []types.FleetCategorySub{
		// ---- 标准 9（默认显示）----
		{Name: "apNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "apTitle", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "apType", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "issueDept", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "signDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "effectiveDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "scope", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		// ---- 扩展 10（默认不显示）----
		{Name: "signer", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "contact", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "contactPhone", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "urgency", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "validUntil", Enabled: true, IsDefault: false, DataType: "date"},
		{Name: "attachments", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "person", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "measure", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "basis", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "summary", Enabled: true, IsDefault: false, DataType: "textarea"},
		// 同上：fileName 是列表系统列，不进提取字段目录。
	}
}

// noticeSeedKeyAlias 把 snake 契约键归一到前端 subs 使用的 camelCase 形态
// （列表列定义 DEFAULT_NOTICE_COLUMNS 按 camelCase 键匹配，subs 必须保持 camel）。
func noticeSeedKeyAlias(name string) string {
	switch strings.TrimSpace(name) {
	case "ap_no":
		return "apNo"
	case "ap_title":
		return "apTitle"
	case "ap_type":
		return "apType"
	case "issue_dept":
		return "issueDept"
	case "sign_date":
		return "signDate"
	case "effective_date":
		return "effectiveDate"
	case "contact_phone":
		return "contactPhone"
	case "valid_until":
		return "validUntil"
	case "extract_status":
		return "extractStatus"
	}
	return strings.TrimSpace(name)
}

// mergeNoticeSeedSubs 把存量公告 subs 收敛到统一目录（notice/award_punish 共用一份），
// 并按目录顺序重排；目录外的历史字段（dept/position/remark）不再进入展示集，
// 值仍留在 custom_metadata 里。保留用户对既有字段的 enabled/is_default/width 调整。
func mergeNoticeSeedSubs(existing []types.FleetCategorySub) []types.FleetCategorySub {
	seed := noticeSeedSubs()
	byKey := make(map[string]types.FleetCategorySub, len(existing))
	for _, s := range existing {
		k := noticeSeedKeyAlias(s.Name)
		if k == "" {
			continue
		}
		if _, dup := byKey[k]; dup {
			continue
		}
		s.Name = k
		byKey[k] = s
	}
	out := make([]types.FleetCategorySub, 0, len(seed))
	for _, f := range seed {
		if s, ok := byKey[f.Name]; ok {
			out = append(out, s)
			continue
		}
		out = append(out, f)
	}
	// 剥离列表系统列（fileName）：与 mergeContractSeedSubs 同一收口口径。
	return stripListSystemSubs(out)
}

// seedBusinessCategories 为 P2-D 新业务模块（合同/发票/制度/奖惩）seed 默认列配置。
// 记录 tenant_id=0（全局共享），subs.name 为前端 row 的 camelCase key，data_type 标注类型。
func (h *FleetHandler) seedBusinessCategories() {
	ctx := context.Background()
	seeds := []struct {
		Scope      string
		Name       string
		BuiltinKey string
		SortOrder  int
		Subs       []types.FleetCategorySub
	}{
		{"invoice", "普通发票", "invoice-common", 1, []types.FleetCategorySub{
			{Name: "invoiceNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "invoiceDate", Enabled: true, IsDefault: true, DataType: "date"},
			{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "taxRate", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "tax", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "totalAmount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "buyerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "sellerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "issuer", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
			{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
			{Name: "sellerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
			{Name: "buyerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
			{Name: "items", Enabled: true, IsDefault: true, DataType: "items", SubFields: invoiceItemSubFieldsStandard()},
			{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
		}},
		{"invoice", "专用发票", "invoice-vat", 2, []types.FleetCategorySub{
			{Name: "invoiceNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "invoiceDate", Enabled: true, IsDefault: true, DataType: "date"},
			{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "taxRate", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "tax", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "totalAmount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "buyerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "sellerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "issuer", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
			{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
			{Name: "sellerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
			{Name: "buyerTaxNo", Enabled: true, IsDefault: false, DataType: "text"},
			{Name: "items", Enabled: true, IsDefault: true, DataType: "items", SubFields: invoiceItemSubFieldsStandard()},
			{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
		}},
		// 其它票据：系统内置兜底分类（任务 2）。废除 OCR 自动降级后，上传分类完全由用户
		// 手动选择驱动，无法匹配已启用分类的杂散票据归入此分类。SortOrder=99 保证在
		// 上传下拉/分类配置列表始终沉底；IsDefault=true 标记内置（防止被彻底物理删除）。
		// 字段复用普通发票字段集（发票号码/日期/类型/金额/税率/税额/价税合计/购买方含税号/
		// 销售方含税号/开票人/备注/状态/标签/明细/文件名），确保规则回退与字段配置一致。
		{"invoice", "其它票据", "invoice-misc", 99, []types.FleetCategorySub{
			{Name: "invoiceNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "invoiceDate", Enabled: true, IsDefault: true, DataType: "date"},
			{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "taxRate", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "tax", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "totalAmount", Enabled: true, IsDefault: true, DataType: "number"},
			{Name: "buyerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "buyerTaxNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "sellerName", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "sellerTaxNo", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "issuer", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
			{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
			{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
			{Name: "items", Enabled: true, IsDefault: false, DataType: "items", SubFields: invoiceItemSubFieldsStandard()},
			{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
		}},
		// 制度内置分类（人事/财务/生产/行政/安全/其它制度）统一走 reconcileRegulationCategories
		// 名称协调，不再在本 seed 块中创建笼统「制度」分类（其早期 subs 用了 title/docNumber/
		// category/publishDate/issuer 英文键，与 RegulationRow 字段不匹配）。
		// 通知公告内置分类（通知/公告/通报/决定/其它通知）统一走 reconcileNoticeCategories
		// 名称协调，不再在本 seed 块中创建笼统「奖惩」分类（其早期 subs 用了 title/eventDate/
		// reason/amount 等错误键，与 NoticeRow 字段不匹配）。
	}
	for _, s := range seeds {
		var cnt int64
		h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL", s.Scope, s.Name).Count(&cnt)
		if cnt > 0 {
			continue
		}
		cat := types.FleetCategory{
			ID:         "seed-" + s.Scope + "-" + s.BuiltinKey,
			TenantID:   0,
			Scope:      s.Scope,
			BuiltinKey: s.BuiltinKey,
			Name:       s.Name,
			Subs:       s.Subs,
			SortOrder:  s.SortOrder,
			Enabled:    true,
			CreatedAt:  timeNowUTC(),
			UpdatedAt:  timeNowUTC(),
		}
		if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
			logger.Warnf(ctx, "seed business category %s failed: %v", s.Scope, err)
		}
	}

	// 其它票据：已存在分类补齐缺失字段（复用普通发票字段集，幂等、不覆盖已有字段）
	h.seedInvoiceMiscSubsBackfill(ctx)

	// 发票明细子列：按分类名幂等补齐 items 字段的 sub_fields（通行费/水电发票等用户
	// 自建分类当前连 items 字段都没有；普通/专用/其它票据可能只有 items 无子列）。
	// 「防覆盖」原则：items 已有非空 sub_fields 的分类绝不更新，只补缺失项。
	h.seedInvoiceItemSubFields(ctx)

	// 合同内置分类（服务合同/租赁合同）走专门的名称协调逻辑，兼容历史脏数据：
	// 早期笼统「合同」分类退役、服务合同曾复用 builtin_key=contract、用户自建同名分类收编。
	h.reconcileContractCategories(ctx)

	// 制度内置分类（人事/财务/生产/行政/安全/其它制度）走同样的名称协调逻辑，
	// 退役笼统「制度」分类（subs 为 title/docNumber 等错误英文键），按类型建分类。
	h.reconcileRegulationCategories(ctx)

	// 通知公告内置分类（通知/公告/通报/决定/其它通知）走同样的名称协调逻辑，
	// 退役笼统「奖惩」分类（subs 为 title/eventDate 等错误键），按类型建分类。
	h.reconcileNoticeCategories(ctx)

	// 能耗内置分类（工业用电/生活用水/燃气费/供热费）：账期台账口径，
	// builtin_key 锚定（用户改名后不复活旧名），subs 按 seed 重排并补齐缺字段。
	h.reconcileEnergyCategories(ctx)

	// 能耗「账单类型」三分类（市电账单 / 光伏账单 / 能耗单）：字段定义与提取规则
	// 挂在这三个上，与上面的「能耗类型」是两条不同的轴。
	// 复用迁移逻辑：它会合并内存默认分组 + 旧表自定义字段，幂等且只补不改。
	if _, _, err := NewUtilityHandler(h.db).MigrateUtilityFieldConfigsToEnergyScope(ctx, 0, false); err != nil {
		logger.Warnf(ctx, "seed energy bill categories failed: %v", err)
	}
	// 账单概况收敛为 20 个业务字段（幂等）；老库上一版被并成 34 个，必须每次启动校正。
	uh := NewUtilityHandler(h.db)
	if _, err := uh.MigrateEnergyOverviewFields(ctx); err != nil {
		logger.Warnf(ctx, "reset energy overview fields failed: %v", err)
	}
	if _, err := uh.resetEnergyExtractConfigFields(ctx); err != nil {
		logger.Warnf(ctx, "reset energy extract config fields failed: %v", err)
	}
	// 「分时时段」细项（尖/峰/平/谷）沉淀为 subs，供字段定义页展开配置
	if _, err := uh.ensureEnergyPeriodSubItems(ctx); err != nil {
		logger.Warnf(ctx, "seed energy period sub-items failed: %v", err)
	}
	// 存量账单的账单周期刷正为 YYYY-MM（幂等；老数据没有 bill_period 字段）
	if _, err := uh.BackfillBillingPeriod(ctx); err != nil {
		logger.Warnf(ctx, "backfill billing period failed: %v", err)
	}
	// 顶部 6 张卡片 ↔ 设置分类 1:1：补齐电费/水费/气费核算与费用清单四类的默认列
	if _, err := uh.EnsureEnergyViewCategories(ctx); err != nil {
		logger.Warnf(ctx, "seed energy view categories failed: %v", err)
	}
	// 「能耗单」是台账分类，页面没有对应卡片：保留数据但排到末尾，避免打乱卡片顺序
	uh.hideEnergyLedgerCategory(ctx)
	// 能耗账单文件的「所属分类」回填：上传只写了 bill_kind、提取只写 kind，
	// 历史记录抽屉的分类列/分类筛选因此全空（真机实测 27 份文件全无分类）。
	if _, err := uh.BackfillEnergyBillCategories(ctx); err != nil {
		logger.Warnf(ctx, "backfill energy bill categories failed: %v", err)
	}

	var invSeeds []types.FleetCategory
	if err := h.db.WithContext(ctx).Where("tenant_id = 0 AND scope = ? AND builtin_key IN ? AND deleted_at IS NULL",
		types.FleetCategoryScopeInvoice, []string{"invoice-common", "invoice-vat"}).Find(&invSeeds).Error; err == nil {
		for i := range invSeeds {
			subs := invSeeds[i].Subs
			hasItems := false
			for _, s := range subs {
				if s.Name == "items" {
					hasItems = true
					break
				}
			}
			if hasItems {
				continue
			}
			subs = append(subs, types.FleetCategorySub{Name: "items", Enabled: true, IsDefault: true, DataType: "items"})
			raw, merr := json.Marshal(subs)
			if merr != nil {
				logger.Warnf(ctx, "backfill invoice items field marshal failed: %v", merr)
				continue
			}
			// 显式 ::jsonb 转换：GORM 对 jsonb 列直接 Update slice 会以 record 数组传参导致 SQLSTATE 42804
			if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).Where("id = ?", invSeeds[i].ID).
				Update("subs", gorm.Expr("?::jsonb", string(raw))).Error; err != nil {
				logger.Warnf(ctx, "backfill invoice items field failed: %v", err)
			} else {
				logger.Infof(ctx, "backfilled invoice category %q items field", invSeeds[i].Name)
			}
		}
	}
}

// seedInvoiceMiscSubsBackfill 幂等补齐「其它票据」内置分类字段（复用普通发票字段集）。
// 仅追加缺失字段，不覆盖已有字段配置（字段名、顺序、启用状态均保留），
// 与 fleetSeed 中「其它票据」的定义保持同一份字段清单。
func (h *FleetHandler) seedInvoiceMiscSubsBackfill(ctx context.Context) {
	expected := []types.FleetCategorySub{
		{Name: "invoiceNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "invoiceDate", Enabled: true, IsDefault: true, DataType: "date"},
		{Name: "invoiceType", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "amount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "taxRate", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "tax", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "totalAmount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "buyerName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "buyerTaxNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "sellerName", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "sellerTaxNo", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "issuer", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
		{Name: "extractStatus", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		{Name: "items", Enabled: true, IsDefault: false, DataType: "items"},
		{Name: "fileName", Enabled: true, IsDefault: false, DataType: "text"},
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL",
			types.FleetCategoryScopeInvoice, "其它票据").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "backfill misc invoice subs: list failed: %v", err)
		return
	}
	for i := range cats {
		existing := cats[i].Subs
		have := make(map[string]bool, len(existing))
		for _, s := range existing {
			have[s.Name] = true
		}
		added := 0
		for _, e := range expected {
			if !have[e.Name] {
				existing = append(existing, e)
				have[e.Name] = true
				added++
			}
		}
		if added == 0 {
			continue
		}
		raw, merr := json.Marshal(existing)
		if merr != nil {
			logger.Warnf(ctx, "backfill misc invoice subs marshal failed: %v", merr)
			continue
		}
		// 显式 ::jsonb 转换：GORM 对 jsonb 列直接 Update slice 会以 record 数组传参导致 SQLSTATE 42804
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).Where("id = ?", cats[i].ID).
			Update("subs", gorm.Expr("?::jsonb", string(raw))).Error; err != nil {
			logger.Warnf(ctx, "backfill misc invoice subs failed: %v", err)
		} else {
			logger.Infof(ctx, "backfilled misc invoice category %q subs (+%d fields)", cats[i].Name, added)
		}
	}
}

// invoiceItemSubFieldsStandard 标准票面明细子列（普通/专用/其它票据/水电/电费）：
// 项目名称/规格型号/单位/数量/单价/金额/税率(征收率)/税额，8 列。
// invoiceItemSubFieldsStandard 标准发票明细子列（内置子项：前端禁止删除）。
func invoiceItemSubFieldsStandard() []types.FleetItemSubField {
	return []types.FleetItemSubField{
		builtinItemSubField("name", "项目名称", "text", 140),
		builtinItemSubField("spec", "规格型号", "text", 110),
		builtinItemSubField("unit", "单位", "text", 70),
		builtinItemSubField("qty", "数量", "number", 80),
		builtinItemSubField("price", "单价", "number", 90),
		builtinItemSubField("amount", "金额", "number", 100),
		builtinItemSubField("tax_rate", "税率/征收率", "number", 90),
		builtinItemSubField("tax_amount", "税额", "number", 90),
	}
}

// builtinItemSubField 构造一个内置明细子项：默认启用 + builtin 标记。
// Enabled 用指针是为了兼容历史 JSON（无该键 = 启用），内置项显式写 true。
func builtinItemSubField(key, label, dataType string, width int) types.FleetItemSubField {
	enabled := true
	return types.FleetItemSubField{
		Key: key, Label: label, DataType: dataType, Width: width,
		Enabled: &enabled, Builtin: true,
	}
}

// markBuiltinSubFields 给存量子列补 builtin 标记（只认模板里出现过的键）。
// 返回是否发生变化，调用方据此决定是否需要回写。
// markBuiltinFields 把 subs 的 builtin 标记**对齐**到系统字段清单（双向同步）。
//
// 为什么是双向而不是"只加"：builtin 的口径变过两次——
//
//	第一版没有这个键 → 存量分类全是 false，前端得靠字典兜底；
//	第二版全量置 true（17 个字段都不可删）→ 用户反馈"只有一个字段删不掉"；
//	现在口径收敛到 3 个系统字段。只加不减会让第二版写下的错误标记永久留在库里，
//	所以这里按清单重算：命中清单 → true，未命中 → false。
//
// 只改 builtin 一个布尔位，不碰 label/enabled/width/顺序。
func markBuiltinFields(subs, tpl []types.FleetCategorySub) bool {
	builtinNames := make(map[string]bool, len(tpl))
	for _, t := range tpl {
		if t.Builtin {
			builtinNames[t.Name] = true
		}
	}
	changed := false
	for i := range subs {
		want := builtinNames[subs[i].Name]
		if subs[i].Builtin != want {
			subs[i].Builtin = want
			changed = true
		}
	}
	return changed
}

func markBuiltinSubFields(subs, tpl []types.FleetItemSubField) bool {
	builtinKeys := make(map[string]bool, len(tpl))
	for _, t := range tpl {
		if t.Builtin {
			builtinKeys[t.Key] = true
		}
	}
	enabled := true
	changed := false
	for i := range subs {
		if !builtinKeys[subs[i].Key] {
			continue
		}
		if !subs[i].Builtin {
			subs[i].Builtin = true
			changed = true
		}
		if subs[i].Enabled == nil {
			subs[i].Enabled = &enabled
			changed = true
		}
	}
	return changed
}

// invoiceItemSubFieldsToll 通行费发票明细子列（用户口径）：
// 项目名称/车牌号/车辆类型/通行日期起/通行日期止/金额/税率(征收率)/税额，8 列。
func invoiceItemSubFieldsToll() []types.FleetItemSubField {
	return []types.FleetItemSubField{
		builtinItemSubField("name", "项目名称", "text", 120),
		builtinItemSubField("plate_no", "车牌号", "text", 100),
		builtinItemSubField("vehicle_type", "车辆类型", "text", 90),
		builtinItemSubField("travel_from", "通行日期起", "date", 110),
		builtinItemSubField("travel_to", "通行日期止", "date", 110),
		builtinItemSubField("amount", "金额", "number", 100),
		builtinItemSubField("tax_rate", "税率/征收率", "number", 90),
		builtinItemSubField("tax_amount", "税额", "number", 90),
	}
}

// seedInvoiceItemSubFields 按分类名幂等补齐发票 items 字段的 sub_fields（子列口径）。
// 通行费发票 → 通行费子列；水电/电费/普通/专用/其它票据 → 标准子列。
// 规则：分类缺少 items 字段 → 追加 items（默认启用）；items 存在但 sub_fields 为空
// → 补子列；items 已有非空 sub_fields → 跳过（绝不覆盖用户编辑过的配置）。
func (h *FleetHandler) seedInvoiceItemSubFields(ctx context.Context) {
	tplByCategory := map[string][]types.FleetItemSubField{
		"普通发票":  invoiceItemSubFieldsStandard(),
		"专用发票":  invoiceItemSubFieldsStandard(),
		"其它票据":  invoiceItemSubFieldsStandard(),
		"水电发票":  invoiceItemSubFieldsStandard(),
		"电费发票":  invoiceItemSubFieldsStandard(),
		"通行费发票": invoiceItemSubFieldsToll(),
		"通行费":   invoiceItemSubFieldsToll(), // 兼容早期无「发票」后缀的自建分类名
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND deleted_at IS NULL", types.FleetCategoryScopeInvoice).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "seed invoice item sub_fields: list failed: %v", err)
		return
	}
	for i := range cats {
		tpl, ok := tplByCategory[cats[i].Name]
		if !ok {
			continue // 用户自建的非明细分类不动
		}
		subs := cats[i].Subs
		changed := false
		idx := -1
		for j := range subs {
			if subs[j].Name == "items" {
				idx = j
				break
			}
		}
		if idx < 0 {
			// 缺 items 字段 → 追加（默认启用，供详情抽屉明细区使用）
			subs = append(subs, types.FleetCategorySub{Name: "items", Enabled: true, IsDefault: true, DataType: "items", SubFields: tpl})
			changed = true
		} else if len(subs[idx].SubFields) == 0 {
			// items 存在但无子列 → 补子列
			subs[idx].SubFields = tpl
			changed = true
		} else {
			// 存量分类的子列是旧 seed 写的，没有 builtin/enabled 标记。
			// 按模板键打标（只加标记，不动用户的 label/width/顺序），
			// 前端据此禁用内置子项删除入口。
			if markBuiltinSubFields(subs[idx].SubFields, tpl) {
				changed = true
			}
		}
		// 系统字段补 builtin 标记（只认发票类型/提取状态/标签这 3 个）
		if markBuiltinFields(subs, invoiceSystemFields) {
			changed = true
		}
		// 存量重复子列（tax / tax_amount 同名「税额」）一次性收敛
		if normalized, ok := normalizeInvoiceCategorySubs(types.FleetCategoryScopeInvoice, subs); ok {
			subs = normalized
			changed = true
		}
		if !changed {
			continue
		}
		raw, merr := json.Marshal(subs)
		if merr != nil {
			logger.Warnf(ctx, "seed invoice item sub_fields marshal failed: %v", merr)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).Where("id = ?", cats[i].ID).
			Update("subs", gorm.Expr("?::jsonb", string(raw))).Error; err != nil {
			logger.Warnf(ctx, "seed invoice item sub_fields failed for %q: %v", cats[i].Name, err)
		} else {
			logger.Infof(ctx, "seeded invoice item sub_fields for category %q", cats[i].Name)
		}
	}
}

// reconcileContractCategories 协调合同内置分类（幂等，每次启动执行）。
// 终态：内置仅「服务合同 contract-service(sort=0)」「租赁合同 contract-lease(sort=1)」两个分类，
// 各挂 contractSeedSubs() 35 字段全集；早期笼统内置分类「合同」软删除。
// 兼容三类历史脏数据：
//  1. 早期服务合同复用 builtin_key=contract（id=seed-contract）——恢复并改锚为 contract-service；
//  2. 用户在前端自建、与内置同名的分类（UUID、builtin_key 为空）——收编为内置行；
//  3. 同名重复行——保留最优一行（未删除优先、锚点匹配优先），其余 live 行软删。
func (h *FleetHandler) reconcileContractCategories(ctx context.Context) {
	now := timeNowUTC()

	// 1) 退役笼统「合同」分类：按名称精确匹配，避免误伤同名锚点不同的服务合同
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL", "contract", "合同").
		Update("deleted_at", now).Error; err != nil {
		logger.Warnf(ctx, "retire legacy generic contract category failed: %v", err)
	}

	wanted := []struct {
		Name string
		Key  string
		Sort int
	}{
		{"服务合同", "contract-service", 0},
		{"租赁合同", "contract-lease", 1},
	}
	fullSubs := contractSeedSubs()

	for _, w := range wanted {
		// 按 builtin_key 定位存量行（含软删；DeletedAt 为普通 *time.Time，无 GORM 自动软删作用域）。
		//
		// wanted 只声明锚点与排序，**不声明名字**：用户把内置「服务合同」改名为
		// 「商务合同」后，按 name 查旧名必然查空 → 走创建分支 → 用固定 ID
		// `seed-contract-contract-service` 又建一个「服务合同」。于是每次重启都在
		// 「商务合同」旁边复活一个 0 份旧名分类，KPI 卡片/字段筛选器/提取规则全被污染
		// （2026-10-22 真机事故：「商务合同 0 份」就是这么来的）。
		//
		// 正确口径：builtin_key 是锚点，名字以库为准。只有 key 与历史遗留 ID 都查不到
		// （真正全新环境）时，才用 wanted 里的初始名创建。
		//
		// OR 条件必须用链式 Where：字符串条件里写 `(a = ? OR b = ?)` 在部分驱动
		// （sqlite）下占位符绑定错位，会查不到行进而误走创建分支。
		var rows []types.FleetCategory
		if err := h.db.WithContext(ctx).
			Where("tenant_id = 0 AND scope = ?", "contract").
			Where("builtin_key = ? OR id = ?", w.Key, "seed-contract").
			Find(&rows).Error; err != nil {
			logger.Warnf(ctx, "reconcile contract category %q query failed: %v", w.Key, err)
			continue
		}
		// 只保留与本次锚点相关的行：id=seed-contract 的兜底行只在没有 key 匹配行时才算
		if len(rows) > 1 {
			filtered := make([]types.FleetCategory, 0, len(rows))
			for _, r := range rows {
				if r.BuiltinKey == w.Key {
					filtered = append(filtered, r)
				}
			}
			if len(filtered) > 0 {
				rows = filtered
			}
		}

		if len(rows) == 0 {
			// 全新环境：直接创建内置分类（wanted 里的初始名只在这里用）
			cat := types.FleetCategory{
				ID:         "seed-contract-" + w.Key,
				TenantID:   0,
				Scope:      "contract",
				GroupID:    "",
				BuiltinKey: w.Key,
				Name:       w.Name,
				Subs:       fullSubs,
				SortOrder:  w.Sort,
				Enabled:    true,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
				logger.Warnf(ctx, "reconcile contract category %q create failed: %v", w.Name, err)
			} else {
				logger.Infof(ctx, "reconcile contract category %q created with %d subs", w.Name, len(fullSubs))
			}
			continue
		}

		// 挑选目标行：未删除优先，其次 builtin_key 匹配优先
		// rows 已按 builtin_key 筛过；名字以库为准——用户改名后 name 已是新名，
		// 绝不能用 wanted 里的旧名覆盖（w.Name 只在「全新环境创建」时用作初始名）。
		targetIdx := 0
		for i := range rows {
			cur, best := &rows[i], &rows[targetIdx]
			curLive, bestLive := cur.DeletedAt == nil, best.DeletedAt == nil
			if curLive && !bestLive {
				targetIdx = i
			} else if curLive == bestLive && cur.BuiltinKey == w.Key && best.BuiltinKey != w.Key {
				targetIdx = i
			}
		}
		target := rows[targetIdx]

		// 同名重复的其它 live 行软删
		for i := range rows {
			if i == targetIdx || rows[i].DeletedAt != nil {
				continue
			}
			if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
				Where("id = ?", rows[i].ID).Update("deleted_at", now).Error; err != nil {
				logger.Warnf(ctx, "reconcile contract category %q dedup failed: %v", w.Name, err)
			}
		}

		// 合并字段全集：旧键就地归一到契约键（保留用户对既有字段的 enabled/is_default/width 调整），
		// 归一后去重，再 append seed 中缺失的契约键（不覆盖用户已删字段——仅补缺）
		subs := mergeContractSeedSubs(target.Subs)
		raw, merr := json.Marshal(subs)
		if merr != nil {
			logger.Warnf(ctx, "reconcile contract category %q marshal failed: %v", w.Name, merr)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ?", target.ID).
			Updates(map[string]any{
				"deleted_at":  nil, // 恢复被误软删的行
				"builtin_key": w.Key,
				"sort_order":  w.Sort,
				"enabled":     true,
				"subs":        gorm.Expr("?::jsonb", string(raw)),
				"updated_at":  now,
			}).Error; err != nil {
			logger.Warnf(ctx, "reconcile contract category %q update failed: %v", w.Name, err)
		} else {
			logger.Infof(ctx, "reconcile contract category %q anchored to %q with %d subs", w.Name, w.Key, len(subs))
		}
	}
}

// reconcileRegulationCategories 协调制度内置分类（幂等，每次启动执行）。
// 终态：内置「人事管理 regulation-hr(sort=0)」「财务管理 regulation-finance(sort=1)」
// 「生产管理 regulation-production(sort=2)」「行政管理 regulation-admin(sort=3)」
// 「安全管理 regulation-safety(sort=4)」「其它制度 regulation-other(sort=99)」六个分类，
// 各挂 regulationSeedSubs() 19 字段全集；早期笼统内置分类「制度」（subs 用 title/docNumber/
// category/publishDate/issuer 英文键，与 RegulationRow 不匹配）软删除。
// 兼容三类历史脏数据（与合同一致）：
//  1. 早期笼统分类「制度」（id=seed-regulation、builtin_key=regulation）——退役，由各类型分类承接；
//  2. 用户在前端自建、与内置同名的分类（UUID、builtin_key 为空）——收编为内置行；
//  3. 同名重复行——保留最优一行（未删除优先、锚点匹配优先），其余 live 行软删。
func (h *FleetHandler) reconcileRegulationCategories(ctx context.Context) {
	now := timeNowUTC()

	// 1) 退役笼统「制度」分类：按名称精确匹配（scope=regulation）
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL", "regulation", "制度").
		Update("deleted_at", now).Error; err != nil {
		logger.Warnf(ctx, "retire legacy generic regulation category failed: %v", err)
	}

	wanted := []struct {
		Name string
		Key  string
		Sort int
	}{
		{"人事管理", "regulation-hr", 0},
		{"财务管理", "regulation-finance", 1},
		{"生产管理", "regulation-production", 2},
		{"行政管理", "regulation-admin", 3},
		{"安全管理", "regulation-safety", 4},
		{"其它制度", "regulation-other", 99},
	}
	fullSubs := regulationSeedSubs()

	for _, w := range wanted {
		// 查同名全部行（含软删；DeletedAt 为普通 *time.Time，无 GORM 自动软删作用域）
		var rows []types.FleetCategory
		if err := h.db.WithContext(ctx).
			Where("tenant_id = 0 AND scope = ? AND name = ?", "regulation", w.Name).
			Find(&rows).Error; err != nil {
			logger.Warnf(ctx, "reconcile regulation category %q query failed: %v", w.Name, err)
			continue
		}

		if len(rows) == 0 {
			// 全新环境：直接创建内置分类
			cat := types.FleetCategory{
				ID:         "seed-regulation-" + w.Key,
				TenantID:   0,
				Scope:      "regulation",
				GroupID:    "",
				BuiltinKey: w.Key,
				Name:       w.Name,
				Subs:       fullSubs,
				SortOrder:  w.Sort,
				Enabled:    true,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
				logger.Warnf(ctx, "reconcile regulation category %q create failed: %v", w.Name, err)
			} else {
				logger.Infof(ctx, "reconcile regulation category %q created with %d subs", w.Name, len(fullSubs))
			}
			continue
		}

		// 挑选目标行：未删除优先，其次 builtin_key 匹配优先。
		// rows 已按 builtin_key 筛过；**名字以库为准**——用户改名后 name 已是新名，
		// 绝不能用 wanted 里的旧名覆盖（w.Name 只在「全新环境创建」时用作初始名）。
		targetIdx := 0
		for i := range rows {
			cur, best := &rows[i], &rows[targetIdx]
			curLive, bestLive := cur.DeletedAt == nil, best.DeletedAt == nil
			if curLive && !bestLive {
				targetIdx = i
			} else if curLive == bestLive && cur.BuiltinKey == w.Key && best.BuiltinKey != w.Key {
				targetIdx = i
			}
		}
		target := rows[targetIdx]

		// 同名重复的其它 live 行软删
		for i := range rows {
			if i == targetIdx || rows[i].DeletedAt != nil {
				continue
			}
			if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
				Where("id = ?", rows[i].ID).Update("deleted_at", now).Error; err != nil {
				logger.Warnf(ctx, "reconcile regulation category %q dedup failed: %v", w.Name, err)
			}
		}

		// 合并字段全集：仅 append 缺失 name，保留用户对既有字段的 enabled/is_default/width 调整。
		// stripListSystemSubs 顺带把存量里 seed 出来的 fileName 剥掉（列表系统列不是提取字段）。
		subs := stripListSystemSubs(target.Subs)
		have := make(map[string]bool, len(subs))
		for _, s := range subs {
			have[s.Name] = true
		}
		for _, f := range fullSubs {
			if !have[f.Name] {
				subs = append(subs, f)
				have[f.Name] = true
			}
		}
		raw, merr := json.Marshal(subs)
		if merr != nil {
			logger.Warnf(ctx, "reconcile regulation category %q marshal failed: %v", w.Name, merr)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ?", target.ID).
			Updates(map[string]any{
				"deleted_at":  nil, // 恢复被误软删的行
				"builtin_key": w.Key,
				"sort_order":  w.Sort,
				"enabled":     true,
				"subs":        gorm.Expr("?::jsonb", string(raw)),
				"updated_at":  now,
			}).Error; err != nil {
			logger.Warnf(ctx, "reconcile regulation category %q update failed: %v", w.Name, err)
		} else {
			logger.Infof(ctx, "reconcile regulation category %q anchored to %q with %d subs", w.Name, w.Key, len(subs))
		}
	}
}

// reconcileNoticeCategories 协调通知公告内置分类（幂等，每次启动执行）。
// 终态：内置「通知 notice-notice(sort=0)」「公告 notice-announcement(sort=1)」
// 「通报 notice-bulletin(sort=2)」「决定 notice-decision(sort=3)」
// 「其它通知 notice-other(sort=99)」五个分类，
// 各挂 noticeSeedSubs() 17 字段全集；早期笼统内置分类「奖惩」（subs 用 title/eventDate/
// reason/amount 等错误键，与 NoticeRow 不匹配）软删除。
// 兼容三类历史脏数据（与合同/制度一致）：
//  1. 早期笼统分类「奖惩」（id=seed-award_punish-award_punish、builtin_key=award_punish）——退役，由各类型分类承接；
//  2. 用户在前端自建、与内置同名的分类（UUID、builtin_key 为空）——收编为内置行；
//  3. 同名重复行——保留最优一行（未删除优先、锚点匹配优先），其余 live 行软删。
func (h *FleetHandler) reconcileNoticeCategories(ctx context.Context) {
	now := timeNowUTC()

	// 1) 退役笼统「奖惩」分类：按名称精确匹配（scope=award_punish）
	if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL", "award_punish", "奖惩").
		Update("deleted_at", now).Error; err != nil {
		logger.Warnf(ctx, "retire legacy generic notice category failed: %v", err)
	}

	wanted := []struct {
		Name string
		Key  string
		Sort int
	}{
		{"通知", "notice-notice", 0},
		{"公告", "notice-announcement", 1},
		{"通报", "notice-bulletin", 2},
		{"决定", "notice-decision", 3},
		{"其它通知", "notice-other", 99},
	}
	fullSubs := noticeSeedSubs()

	for _, w := range wanted {
		// 查同名全部行（含软删；DeletedAt 为普通 *time.Time，无 GORM 自动软删作用域）
		var rows []types.FleetCategory
		if err := h.db.WithContext(ctx).
			Where("tenant_id = 0 AND scope = ? AND name = ?", "award_punish", w.Name).
			Find(&rows).Error; err != nil {
			logger.Warnf(ctx, "reconcile notice category %q query failed: %v", w.Name, err)
			continue
		}

		if len(rows) == 0 {
			// 全新环境：直接创建内置分类
			cat := types.FleetCategory{
				ID:         "seed-notice-" + w.Key,
				TenantID:   0,
				Scope:      "award_punish",
				GroupID:    "",
				BuiltinKey: w.Key,
				Name:       w.Name,
				Subs:       fullSubs,
				SortOrder:  w.Sort,
				Enabled:    true,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
				logger.Warnf(ctx, "reconcile notice category %q create failed: %v", w.Name, err)
			} else {
				logger.Infof(ctx, "reconcile notice category %q created with %d subs", w.Name, len(fullSubs))
			}
			continue
		}

		// 挑选目标行：未删除优先，其次 builtin_key 匹配优先
		targetIdx := 0
		for i := range rows {
			cur, best := &rows[i], &rows[targetIdx]
			curLive, bestLive := cur.DeletedAt == nil, best.DeletedAt == nil
			if curLive && !bestLive {
				targetIdx = i
			} else if curLive == bestLive && cur.BuiltinKey == w.Key && best.BuiltinKey != w.Key {
				targetIdx = i
			}
		}
		target := rows[targetIdx]

		// 同名重复的其它 live 行软删
		for i := range rows {
			if i == targetIdx || rows[i].DeletedAt != nil {
				continue
			}
			if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
				Where("id = ?", rows[i].ID).Update("deleted_at", now).Error; err != nil {
				logger.Warnf(ctx, "reconcile notice category %q dedup failed: %v", w.Name, err)
			}
		}

		// 合并字段全集：收敛到统一目录并按其顺序重排（notice/award_punish 同一份字段目录），
		// 保留用户对既有字段的 enabled/is_default/width 调整。
		subs := mergeNoticeSeedSubs(target.Subs)
		raw, merr := json.Marshal(subs)
		if merr != nil {
			logger.Warnf(ctx, "reconcile notice category %q marshal failed: %v", w.Name, merr)
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
			Where("id = ?", target.ID).
			Updates(map[string]any{
				"deleted_at":  nil, // 恢复被误软删的行
				"builtin_key": w.Key,
				"sort_order":  w.Sort,
				"enabled":     true,
				"subs":        gorm.Expr("?::jsonb", string(raw)),
				"updated_at":  now,
			}).Error; err != nil {
			logger.Warnf(ctx, "reconcile notice category %q update failed: %v", w.Name, err)
		} else {
			logger.Infof(ctx, "reconcile notice category %q anchored to %q with %d subs", w.Name, w.Key, len(subs))
		}
	}
}
