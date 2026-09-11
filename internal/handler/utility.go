package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// UtilityHandler 水电气管理：字段配置（电费/水费/气费）与水/气月度记录 CRUD。
// 电费账单提取与记录列表走 KnowledgeHandler（复用知识库链路）。
type UtilityHandler struct {
	db *gorm.DB
}

// NewUtilityHandler creates a new UtilityHandler.
func NewUtilityHandler(db *gorm.DB) *UtilityHandler {
	return &UtilityHandler{db: db}
}

func utilityTenantID(c *gin.Context) (uint64, error) {
	return types.MustTenantIDFromContext(c.Request.Context()), nil
}

// ---------------------------------------------------------------------------
// 字段配置
// ---------------------------------------------------------------------------

// ListUtilityFieldConfigs godoc
// @Summary      获取字段配置
// @Description  按分类返回字段配置（electricity/water/gas），支持 group 过滤（电费分组）；未配置时返回内置默认字段。
// @Router       /utilities/field-configs [get]
func (h *UtilityHandler) ListUtilityFieldConfigs(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
	group := strings.TrimSpace(c.Query("group"))
	tenantID, _ := utilityTenantID(c)
	var cfgs []types.UtilityFieldConfig
	q := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category)
	if group != "" {
		q = q.Where(`"group" = ?`, group)
	}
	if err := q.Order("sort_order ASC, created_at ASC").
		Find(&cfgs).Error; err != nil {
		logger.Errorf(ctx, "list utility field configs failed: %v", err)
		c.Error(errors.NewInternalServerError("list field configs failed"))
		return
	}
	if len(cfgs) == 0 {
		// 未配置时返回默认字段；group 为空返回全部组默认（兼容旧调用返回概况组）
		cfgs = utilityDefaultFieldConfigs(tenantID, category, group)
	} else if group == "" && category == "electricity" {
		// 全量：按组合并，DB 已配置的组用 DB 数据，未配置的组用默认字段（保证行配置等新组始终可见）
		cfgs = h.mergeGroupDefaults(cfgs)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfgs})
}

// mergeGroupDefaults 将全量字段配置按分组补齐默认：DB 有数据的组保留 DB 数据，空组用默认字段填充。
func (h *UtilityHandler) mergeGroupDefaults(cfgs []types.UtilityFieldConfig) []types.UtilityFieldConfig {
	allGroups := []string{"overview", "market", "line", "trans", "sys", "gov-industrial", "catalog", "gov-residential", "capacity", "pf", "meter", "resident-meter", "meter-rows", "resident-meter-rows", "catalog-rows", "overview-rows", "market-rows", "trans-rows", "sys-rows", "gov-industrial-rows", "gov-residential-rows"}
	dbMap := make(map[string][]types.UtilityFieldConfig)
	for _, c := range cfgs {
		dbMap[c.Group] = append(dbMap[c.Group], c)
	}
	tenantID := uint64(0)
	if len(cfgs) > 0 {
		tenantID = uint64(cfgs[0].TenantID)
	}
	merged := make([]types.UtilityFieldConfig, 0, len(cfgs)+20)
	for _, g := range allGroups {
		if list, ok := dbMap[g]; ok && len(list) > 0 {
			merged = append(merged, list...)
			continue
		}
		defs := electricityDefaultGroupFields(g)
		now := timeNowUTC()
		for i, d := range defs {
			merged = append(merged, types.UtilityFieldConfig{
				ID:             uuid.NewString(),
				TenantID:       int64(tenantID),
				Category:       "electricity",
				Group:          g,
				FieldKey:       d.key,
				Label:          d.label,
				FieldType:      d.typ,
				DefaultVisible: d.def,
				SortOrder:      i,
				IsCustom:       false,
				CreatedAt:      now,
				UpdatedAt:      now,
			})
		}
	}
	return merged
}

// SaveUtilityFieldConfigs godoc
// @Summary      保存字段配置
// @Description  整组覆盖保存某分类的字段配置（先删后插，事务内完成）；传 group 时仅覆盖该分组。
// @Router       /utilities/field-configs [post]
func (h *UtilityHandler) SaveUtilityFieldConfigs(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
	group := strings.TrimSpace(c.Query("group"))
	tenantID, _ := utilityTenantID(c)
	var req []types.UtilityFieldConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if len(req) > 100 {
		c.Error(errors.NewBadRequestError("too many field configs (max 100)"))
		return
	}
	// 引用检测：本次保存被移除的字段 key 是否仍被历史记录引用（仅提示，不影响保存；删除配置不删历史数据）
	referenced := h.referencedFieldKeys(ctx, tenantID, category, group, req)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		del := tx.Where("tenant_id = ? AND category = ?", tenantID, category)
		if group != "" {
			del = del.Where(`"group" = ?`, group)
		}
		if err := del.Delete(&types.UtilityFieldConfig{}).Error; err != nil {
			return err
		}
		now := timeNowUTC()
		for i := range req {
			cfg := req[i]
			cfg.ID = uuid.NewString()
			cfg.TenantID = int64(tenantID)
			cfg.Category = category
			cfg.Group = group
			cfg.FieldKey = strings.TrimSpace(cfg.FieldKey)
			cfg.Label = strings.TrimSpace(cfg.Label)
			if cfg.FieldKey == "" || cfg.Label == "" {
				return errors.NewBadRequestError("field_key and label are required")
			}
			if cfg.FieldType == "" {
				cfg.FieldType = "text"
			}
			cfg.CreatedAt = now
			cfg.UpdatedAt = now
			cfg.DeletedAt = nil
			if err := tx.Create(&cfg).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "save utility field configs failed: %v", err)
		c.Error(errors.NewInternalServerError("save field configs failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "字段配置已保存", "referenced": referenced})
}

// referencedFieldKeys 统计被本次保存移除的字段 key 在历史记录（knowledges.custom_metadata.records[].item）中被引用的次数。
// 兼容历史双层 custom_metadata 写法；仅提示用，不阻断保存。
func (h *UtilityHandler) referencedFieldKeys(ctx context.Context, tenantID uint64, category, group string, req []types.UtilityFieldConfig) map[string]int {
	result := map[string]int{}
	var old []types.UtilityFieldConfig
	q := h.db.WithContext(ctx).Where("tenant_id = ? AND category = ?", int64(tenantID), category)
	if group != "" {
		q = q.Where(`"group" = ?`, group)
	}
	if err := q.Find(&old).Error; err != nil || len(old) == 0 {
		return result
	}
	oldKeys := map[string]bool{}
	for _, o := range old {
		oldKeys[o.FieldKey] = true
	}
	newKeys := map[string]bool{}
	for _, r := range req {
		newKeys[strings.TrimSpace(r.FieldKey)] = true
	}
	var removed []string
	for k := range oldKeys {
		if !newKeys[k] {
			removed = append(removed, k)
		}
	}
	if len(removed) == 0 {
		return result
	}
	var rows []struct {
		CustomMetadata json.RawMessage `json:"custom_metadata"`
	}
	if err := h.db.WithContext(ctx).Table("knowledges").
		Where("tenant_id = ? AND deleted_at IS NULL", int64(tenantID)).
		Select("custom_metadata").Scan(&rows).Error; err != nil {
		return result
	}
	for _, row := range rows {
		for _, rec := range utilityMetaRecords(row.CustomMetadata) {
			// 兼容两种记录结构：{item:{...}} 包装层（发票/合同）与 直接记录（电费：顶层 key + fee_items[]）
			var item map[string]json.RawMessage
			itemRaw := rec["item"]
			if len(itemRaw) > 0 {
				if err := json.Unmarshal(itemRaw, &item); err != nil {
					continue
				}
			} else {
				item = rec
			}
			for _, k := range removed {
				if _, ok := item[k]; ok {
					result[k]++
					continue
				}
				// 电费/光伏：费用明细子项（fee_items[]）中出现的列字段视为被引用
				if fi, ok := item["fee_items"]; ok && len(fi) > 0 {
					var fis []map[string]json.RawMessage
					if err := json.Unmarshal(fi, &fis); err == nil {
						for _, fiItem := range fis {
							if _, ok2 := fiItem[k]; ok2 {
								result[k]++
								break
							}
						}
					}
				}
			}
		}
	}
	return result
}

// utilityMetaRecords 从 custom_metadata JSON 中提取 records 数组，兼容双层 {custom_metadata:{records}} 历史写法。
func utilityMetaRecords(raw json.RawMessage) []map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var meta struct {
		Records []map[string]json.RawMessage `json:"records"`
	}
	if err := json.Unmarshal(raw, &meta); err == nil && len(meta.Records) > 0 {
		return meta.Records
	}
	var wrapped struct {
		CustomMetadata struct {
			Records []map[string]json.RawMessage `json:"records"`
		} `json:"custom_metadata"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.CustomMetadata.Records
	}
	return nil
}

// ---------------------------------------------------------------------------
// 水/气月度记录
// ---------------------------------------------------------------------------

// ListUtilityMeterRecords godoc
// @Summary      水/气月度记录列表
// @Description  按分类/月份/关键字筛选，返回月度记录（含表计子行）。
// @Router       /utilities/meter-records [get]
func (h *UtilityHandler) ListUtilityMeterRecords(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" || (category != "water" && category != "gas") {
		c.Error(errors.NewBadRequestError("category must be water or gas"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	q := strings.TrimSpace(c.Query("q"))
	month := strings.TrimSpace(c.Query("month"))
	meterID := strings.TrimSpace(c.Query("meter_id"))

	query := h.db.WithContext(ctx).Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category)
	if month != "" {
		query = query.Where("month = ?", month)
	}
	if q != "" {
		query = query.Where("month LIKE ? OR remark LIKE ?", "%"+q+"%", "%"+q+"%")
	}
	if meterID != "" {
		query = query.Where("id IN (?)", h.db.WithContext(ctx).Model(&types.UtilityMeterItem{}).
			Select("record_id").Where("meter_id = ?", meterID))
	}
	var records []types.UtilityMeterRecord
	if err := query.Order("month DESC, created_at DESC").Find(&records).Error; err != nil {
		logger.Errorf(ctx, "list utility meter records failed: %v", err)
		c.Error(errors.NewInternalServerError("list meter records failed"))
		return
	}
	for i := range records {
		var items []types.UtilityMeterItem
		if err := h.db.WithContext(ctx).Where("record_id = ?", records[i].ID).
			Order("created_at ASC").Find(&items).Error; err == nil {
			records[i].Items = items
		}
	}
	// 一并返回该分类表计配置，供前端映射别名/倍率/启用状态
	var meters []types.UtilityMeter
	_ = h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("created_at ASC").Find(&meters).Error
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"records": records, "meters": meters}})
}

// CreateUtilityMeterRecord godoc
// @Summary      新增水/气月度记录
// @Description  同类型同月份唯一；后端自动计算每个表计子行的用量与金额并汇总。
// @Router       /utilities/meter-records [post]
func (h *UtilityHandler) CreateUtilityMeterRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	var req types.UtilityMeterRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Category = strings.TrimSpace(req.Category)
	req.Month = strings.TrimSpace(req.Month)
	if req.Category != "water" && req.Category != "gas" {
		c.Error(errors.NewBadRequestError("category must be water or gas"))
		return
	}
	if req.Month == "" || len(req.Month) != 7 {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityMeterRecord{}).
		Where("tenant_id = ? AND category = ? AND month = ? AND deleted_at IS NULL", tenantID, req.Category, req.Month).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check meter record duplicate failed: %v", err)
		c.Error(errors.NewInternalServerError("check duplicate failed"))
		return
	}
	if cnt > 0 {
		c.Error(errors.NewBadRequestError("该月记录已存在，可编辑原记录"))
		return
	}

	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	if err := h.validateMeterItems(ctx, tenantID, req.Category, req.Items); err != nil {
		c.Error(err)
		return
	}
	rates, err := h.loadMeterRates(ctx, tenantID, req.Category)
	if err != nil {
		logger.Errorf(ctx, "load meter rates failed: %v", err)
		c.Error(errors.NewInternalServerError("load meter rates failed"))
		return
	}
	computeMeterRecordWithRates(&req, rates)
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&req).Error; err != nil {
			return err
		}
		for i := range req.Items {
			it := &req.Items[i]
			it.ID = uuid.NewString()
			it.RecordID = req.ID
			it.CreatedAt = now
			it.UpdatedAt = now
			if err := tx.Create(it).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "create utility meter record failed: %v", err)
		c.Error(errors.NewInternalServerError("create meter record failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateUtilityMeterRecord godoc
// @Summary      更新水/气月度记录
// @Description  整体更新记录与表计子行（子行先删后插），自动重算汇总。
// @Router       /utilities/meter-records/:id [put]
func (h *UtilityHandler) UpdateUtilityMeterRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.UtilityMeterRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Category = strings.TrimSpace(req.Category)
	req.Month = strings.TrimSpace(req.Month)
	if req.Category != "water" && req.Category != "gas" {
		c.Error(errors.NewBadRequestError("category must be water or gas"))
		return
	}
	if req.Month == "" || len(req.Month) != 7 {
		c.Error(errors.NewBadRequestError("month is required (YYYY-MM)"))
		return
	}
	// 月份唯一性（排除自身）
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityMeterRecord{}).
		Where("tenant_id = ? AND category = ? AND month = ? AND id <> ? AND deleted_at IS NULL", tenantID, req.Category, req.Month, id).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check meter record duplicate failed: %v", err)
		c.Error(errors.NewInternalServerError("check duplicate failed"))
		return
	}
	if cnt > 0 {
		c.Error(errors.NewBadRequestError("该月记录已存在，可编辑原记录"))
		return
	}

	now := timeNowUTC()
	if err := h.validateMeterItems(ctx, tenantID, req.Category, req.Items); err != nil {
		c.Error(err)
		return
	}
	rates, err := h.loadMeterRates(ctx, tenantID, req.Category)
	if err != nil {
		logger.Errorf(ctx, "load meter rates failed: %v", err)
		c.Error(errors.NewInternalServerError("load meter rates failed"))
		return
	}
	computeMeterRecordWithRates(&req, rates)
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&types.UtilityMeterRecord{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(map[string]interface{}{
				"month":        req.Month,
				"meter_count":  req.MeterCount,
				"total_usage":  req.TotalUsage,
				"total_amount": req.TotalAmount,
				"remark":       req.Remark,
				"updated_at":   now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.NewNotFoundError("记录不存在")
		}
		if err := tx.Where("record_id = ?", id).Delete(&types.UtilityMeterItem{}).Error; err != nil {
			return err
		}
		for i := range req.Items {
			it := &req.Items[i]
			it.ID = uuid.NewString()
			it.RecordID = id
			it.CreatedAt = now
			it.UpdatedAt = now
			if err := tx.Create(it).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "update utility meter record failed: %v", err)
		c.Error(errors.NewInternalServerError("update meter record failed: " + err.Error()))
		return
	}
	req.ID = id
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// DeleteUtilityMeterRecord godoc
// @Summary      删除水/气月度记录
// @Description  软删除记录（级联删除表计子行）。
// @Router       /utilities/meter-records/:id [delete]
func (h *UtilityHandler) DeleteUtilityMeterRecord(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&types.UtilityMeterRecord{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Update("deleted_at", timeNowUTC())
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.NewNotFoundError("记录不存在")
		}
		return tx.Where("record_id = ?", id).Delete(&types.UtilityMeterItem{}).Error
	})
	if err != nil {
		logger.Errorf(ctx, "delete utility meter record failed: %v", err)
		c.Error(errors.NewInternalServerError("delete meter record failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "记录已删除"})
}

// ---------------------------------------------------------------------------
// 水/气表计配置（utility_meters）
// ---------------------------------------------------------------------------

// ListUtilityMeters godoc
// @Summary      表计配置列表
// @Description  按分类返回水表/气表配置，支持 enabled 过滤（默认全部，enabled=true 仅启用）。
// @Router       /utilities/meters [get]
func (h *UtilityHandler) ListUtilityMeters(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" || (category != "water" && category != "gas") {
		c.Error(errors.NewBadRequestError("category must be water or gas"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	onlyEnabled := c.Query("enabled") == "true"
	query := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category)
	if onlyEnabled {
		query = query.Where("enabled = ?", true)
	}
	var meters []types.UtilityMeter
	if err := query.Order("created_at ASC").Find(&meters).Error; err != nil {
		logger.Errorf(ctx, "list utility meters failed: %v", err)
		c.Error(errors.NewInternalServerError("list meters failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": meters})
}

// CreateUtilityMeter godoc
// @Summary      新增表计配置
// @Description  创建水表/气表配置；别名必填。
// @Router       /utilities/meters [post]
func (h *UtilityHandler) CreateUtilityMeter(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	var req types.UtilityMeter
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Category = strings.TrimSpace(req.Category)
	req.Alias = strings.TrimSpace(req.Alias)
	if req.Category != "water" && req.Category != "gas" {
		c.Error(errors.NewBadRequestError("category must be water or gas"))
		return
	}
	if req.Alias == "" {
		c.Error(errors.NewBadRequestError("别名不能为空"))
		return
	}
	if req.Rate <= 0 {
		req.Rate = 1
	}
	if req.MeterMode == "" {
		req.MeterMode = "manual"
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	if err := h.db.WithContext(ctx).Create(&req).Error; err != nil {
		logger.Errorf(ctx, "create utility meter failed: %v", err)
		c.Error(errors.NewInternalServerError("create meter failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateUtilityMeter godoc
// @Summary      更新表计配置
// @Description  更新水表/气表配置；已引用该表的记录不受影响（仅后续录入引用新参数）。
// @Router       /utilities/meters/:id [put]
func (h *UtilityHandler) UpdateUtilityMeter(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.UtilityMeter
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Alias = strings.TrimSpace(req.Alias)
	if req.Alias == "" {
		c.Error(errors.NewBadRequestError("别名不能为空"))
		return
	}
	if req.Rate <= 0 {
		req.Rate = 1
	}
	if req.MeterMode == "" {
		req.MeterMode = "manual"
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check utility meter failed: %v", err)
		c.Error(errors.NewInternalServerError("check meter failed"))
		return
	}
	if cnt == 0 {
		c.Error(errors.NewNotFoundError("表计不存在"))
		return
	}
	now := timeNowUTC()
	if err := h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
		Where("id = ? AND tenant_id = ?", id, tenantID).
		Updates(map[string]interface{}{
			"alias":              req.Alias,
			"meter_no":           req.MeterNo,
			"rate":               req.Rate,
			"default_unit_price": req.DefaultUnitPrice,
			"use_unit":           req.UseUnit,
			"manager":            req.Manager,
			"contact":            req.Contact,
			"meter_mode":         req.MeterMode,
			"install_date":       req.InstallDate,
			"remark":             req.Remark,
			"enabled":            req.Enabled,
			"updated_at":         now,
		}).Error; err != nil {
		logger.Errorf(ctx, "update utility meter failed: %v", err)
		c.Error(errors.NewInternalServerError("update meter failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteUtilityMeter godoc
// @Summary      删除表计配置
// @Description  删除水表/气表配置；若已有月度记录引用该表计则禁止删除。
// @Router       /utilities/meters/:id [delete]
func (h *UtilityHandler) DeleteUtilityMeter(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityMeterItem{}).
		Where("meter_id = ?", id).Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "check meter reference failed: %v", err)
		c.Error(errors.NewInternalServerError("check meter reference failed"))
		return
	}
	if cnt > 0 {
		c.Error(errors.NewBadRequestError("该表计已有记录，不能删除"))
		return
	}
	res := h.db.WithContext(ctx).Model(&types.UtilityMeter{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete utility meter failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete meter failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("表计不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// ---------------------------------------------------------------------------
// 分时电价规则（尖峰平谷月份设定 + 各时段单价）
// ---------------------------------------------------------------------------

// ListUtilityTariffRules godoc
// @Summary      分时电价规则列表
// @Description  按分类返回分时电价规则（默认返回空数组）。
// @Router       /utilities/tariff-rules [get]
func (h *UtilityHandler) ListUtilityTariffRules(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	var rules []types.UtilityTariffRule
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("is_default DESC, sort_order ASC, created_at ASC").
		Find(&rules).Error; err != nil {
		logger.Errorf(ctx, "list utility tariff rules failed: %v", err)
		c.Error(errors.NewInternalServerError("list tariff rules failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rules})
}

func normalizeTariffRule(r *types.UtilityTariffRule, category string, tenantID uint64) error {
	r.Category = category
	r.Name = strings.TrimSpace(r.Name)
	r.Months = strings.TrimSpace(r.Months)
	// 校验月份：1-12 逗号分隔
	if r.Months != "" {
		for _, m := range strings.Split(r.Months, ",") {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			mm := 0
			fmt.Sscanf(m, "%d", &mm)
			if mm < 1 || mm > 12 {
				return errors.NewBadRequestError("months 须为 1-12 的逗号分隔数字")
			}
		}
	}
	if r.DeepPeakRate < 0 || r.PeakRate < 0 || r.FlatRate < 0 || r.ValleyRate < 0 {
		return errors.NewBadRequestError("单价不能为负数")
	}
	if r.Name == "" {
		r.Name = "分时电价规则"
	}
	return nil
}

// CreateUtilityTariffRule godoc
// @Summary      新增分时电价规则
// @Description  保存尖峰平谷月份设定与各时段单价；is_default 为兜底规则（同分类唯一）。
// @Router       /utilities/tariff-rules [post]
func (h *UtilityHandler) CreateUtilityTariffRule(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	var req types.UtilityTariffRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if err := normalizeTariffRule(&req, category, tenantID); err != nil {
		c.Error(err)
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	req.CreatedAt = now
	req.UpdatedAt = now
	req.DeletedAt = nil
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&types.UtilityTariffRule{}).
				Where("tenant_id = ? AND category = ? AND is_default = ? AND deleted_at IS NULL", tenantID, category, true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&req).Error
	})
	if err != nil {
		logger.Errorf(ctx, "create utility tariff rule failed: %v", err)
		c.Error(errors.NewInternalServerError("create tariff rule failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateUtilityTariffRule godoc
// @Summary      更新分时电价规则
// @Router       /utilities/tariff-rules/:id [put]
func (h *UtilityHandler) UpdateUtilityTariffRule(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.UtilityTariffRule
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if err := normalizeTariffRule(&req, category, tenantID); err != nil {
		c.Error(err)
		return
	}
	now := timeNowUTC()
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&types.UtilityTariffRule{}).
				Where("tenant_id = ? AND category = ? AND is_default = ? AND id <> ? AND deleted_at IS NULL", tenantID, category, true, id).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&types.UtilityTariffRule{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(map[string]interface{}{
				"name":            req.Name,
				"months":          req.Months,
				"deep_peak_rate":  req.DeepPeakRate,
				"peak_rate":       req.PeakRate,
				"flat_rate":       req.FlatRate,
				"valley_rate":     req.ValleyRate,
				"is_default":      req.IsDefault,
				"sort_order":      req.SortOrder,
				"updated_at":      now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.NewNotFoundError("规则不存在")
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "update utility tariff rule failed: %v", err)
		c.Error(errors.NewInternalServerError("update tariff rule failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "规则已更新"})
}

// DeleteUtilityTariffRule godoc
// @Summary      删除分时电价规则
// @Router       /utilities/tariff-rules/:id [delete]
func (h *UtilityHandler) DeleteUtilityTariffRule(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	res := h.db.WithContext(ctx).Model(&types.UtilityTariffRule{}).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		Update("deleted_at", timeNowUTC())
	if res.Error != nil {
		logger.Errorf(ctx, "delete utility tariff rule failed: %v", res.Error)
		c.Error(errors.NewInternalServerError("delete tariff rule failed"))
		return
	}
	if res.RowsAffected == 0 {
		c.Error(errors.NewNotFoundError("规则不存在"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "规则已删除"})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

// computeMeterItem calculates usage = end - start, amount = usage * unit price.
func computeMeterItem(it *types.UtilityMeterItem, rate float64) {
	if rate <= 0 {
		rate = 1
	}
	it.Usage = round2((it.EndReading - it.StartReading) * rate)
	it.Amount = round2(it.Usage * it.UnitPrice)
}

// loadMeterRates 返回该分类下所有表计配置 id→倍率 映射，供记录子行计算用量。
func (h *UtilityHandler) loadMeterRates(ctx context.Context, tenantID uint64, category string) (map[string]float64, error) {
	var meters []types.UtilityMeter
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Find(&meters).Error; err != nil {
		return nil, err
	}
	m := make(map[string]float64, len(meters))
	for i := range meters {
		rate := meters[i].Rate
		if rate <= 0 {
			rate = 1
		}
		m[meters[i].ID] = rate
	}
	return m, nil
}

// validateMeterItems 校验子行表计引用：meter_id 必须存在于配置且同一记录内不重复；
// 未引用配置的旧数据（meter_id 为空）放行以兼容存量记录。
func (h *UtilityHandler) validateMeterItems(ctx context.Context, tenantID uint64, category string, items []types.UtilityMeterItem) error {
	seen := map[string]bool{}
	for _, it := range items {
		if it.MeterID == "" {
			continue
		}
		if seen[it.MeterID] {
			return errors.NewBadRequestError("同一月份不能重复录入同一表计")
		}
		seen[it.MeterID] = true
	}
	return nil
}

// computeMeterRecordWithRates 按表计配置倍率计算用量与金额并汇总。
func computeMeterRecordWithRates(r *types.UtilityMeterRecord, rates map[string]float64) {
	var usage, amount float64
	for i := range r.Items {
		computeMeterItem(&r.Items[i], rates[r.Items[i].MeterID])
		usage += r.Items[i].Usage
		amount += r.Items[i].Amount
	}
	r.MeterCount = len(r.Items)
	r.TotalUsage = round2(usage)
	r.TotalAmount = round2(amount)
}

// computeMeterRecord recalculates meter_count / total_usage / total_amount.
func computeMeterRecord(r *types.UtilityMeterRecord) {
	computeMeterRecordWithRates(r, nil)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
// electricityDefaultGroupFields 返回电费某一分组的默认字段集。
func electricityDefaultGroupFields(group string) []utilityFieldDef {
	g := group
	switch group {
	case "overview":
		return []utilityFieldDef{
			{"bill_period_start", "账单周期起", "date", true, g},
			{"bill_period_end", "账单周期止", "date", false, g},
			{"account_no", "户号", "text", true, g},
			{"account_name", "户名", "text", true, g},
			{"usage_category", "用电类别", "text", true, g},
			{"voltage_level", "电压等级", "text", false, g},
			{"market_attr", "市场化属性", "text", false, g},
			{"supply_unit", "供电服务单位", "text", false, g},
			{"address", "用电地址", "text", false, g},
			{"meter_no", "电能表编号", "text", false, g},
			{"total_kwh", "本期电量", "number", true, g},
			{"total_amount", "本期电费", "amount", true, g},
			{"prev_kwh", "上期电量", "number", false, g},
			{"mom_change", "环比", "text", false, g},
			{"avg_price", "平均电价", "number", true, g},
			{"power_factor", "功率因数", "number", true, g},
			{"due_date", "交费截止", "date", false, g},
			{"industrial_amount", "工商业电费", "amount", false, g},
			{"residential_amount", "居民电费", "amount", false, g},
			{"pf_adjust_amount", "功率因数调整电费", "amount", false, g},
			{"grand_total", "合计", "amount", false, g},
			{"deep_peak_kwh", "尖峰电量", "number", false, g},
			{"peak_kwh", "峰电量", "number", false, g},
			{"flat_kwh", "平电量", "number", false, g},
			{"valley_kwh", "谷电量", "number", false, g},
			{"reactive_kwh", "正向无功电量", "number", false, g},
			{"print_date", "账单打印日期", "date", false, g},
		}
	case "market", "line", "trans", "sys", "gov-industrial", "catalog", "gov-residential":
		return []utilityFieldDef{
			{"name", "费用组成", "text", true, g},
			{"period", "时段", "text", true, g},
			{"qty", "计费电量", "number", true, g},
			{"rate", "计费标准", "number", true, g},
			{"fee", "电费", "amount", true, g},
		}
	case "capacity":
		return []utilityFieldDef{
			{"demand", "需量值", "number", true, g},
			{"demand_price", "需量电价", "number", true, g},
			{"demand_fee", "输配需量电费", "amount", true, g},
			{"kwh_per_kva", "月每千伏安用电量", "number", true, g},
			{"discount_demand_fee", "折扣需量电费", "amount", true, g},
			{"capacity", "容量", "number", true, g},
			{"capacity_price", "容量电价", "number", true, g},
			{"capacity_fee", "输配容量电费", "amount", true, g},
		}
	case "pf":
		return []utilityFieldDef{
			{"project", "项目", "text", true, g},
			{"power_factor", "功率因素实际值", "number", true, g},
			{"pf_standard", "功率因素标准", "number", true, g},
			{"adjust_ratio", "调整系数", "number", true, g},
			{"pf_active_kwh", "参与调整有功电量", "number", true, g},
			{"pf_reactive_kwh", "参与调整无功电量", "number", true, g},
			{"pf_fee_base", "参与调整电费金额", "amount", true, g},
			{"adjust_fee", "功率因素调整电费", "amount", true, g},
		}
	case "meter":
		return []utilityFieldDef{
			{"meter_type", "示数类型", "text", true, g},
			{"prev", "上期示数", "number", true, g},
			{"curr", "本期示数", "number", true, g},
			{"multiplier", "倍率", "number", true, g},
			{"reading_kwh", "抄见电量", "number", true, g},
			{"trans_loss", "变损", "number", true, g},
			{"line_loss", "线损", "number", true, g},
			{"adjust", "加减", "number", true, g},
			{"bill_kwh", "计费电量", "number", true, g},
		}
	case "resident-meter":
		// 居民电量明细列（与工商业电量明细一致 9 列；行数据由前端按 定比行 映射渲染）
		return []utilityFieldDef{
			{"meter_type", "示数类型", "text", true, g},
			{"prev", "上期示数", "number", true, g},
			{"curr", "本期示数", "number", true, g},
			{"multiplier", "倍率", "number", true, g},
			{"reading_kwh", "抄见电量", "number", true, g},
			{"trans_loss", "变损", "number", true, g},
			{"line_loss", "线损", "number", true, g},
			{"adjust", "加减", "number", true, g},
			{"bill_kwh", "计费电量", "number", true, g},
		}
	case "meter-rows":
		// 工商业电量明细行（示数类型行标题，field_key 为默认行文本，label 可改名）
		return []utilityFieldDef{
			{"正向有功（总）", "正向有功（总）", "text", true, g},
			{"正向有功（尖峰）", "正向有功（尖峰）", "text", true, g},
			{"正向有功（峰）", "正向有功（峰）", "text", true, g},
			{"正向有功（平）", "正向有功（平）", "text", true, g},
			{"正向有功（谷）", "正向有功（谷）", "text", true, g},
			{"正向无功（总）", "正向无功（总）", "text", true, g},
		}
	case "resident-meter-rows":
		// 居民电量明细行
		return []utilityFieldDef{
			{"定比0.015", "定比0.015", "text", true, g},
		}
	case "catalog-rows":
		// 目录电费（居民）明细行
		return []utilityFieldDef{
			{"基础电费", "基础电费", "text", true, g},
		}
	case "overview-rows":
		// 账单概况-基础信息行（field_key 为数据字段名，label 可改名；渲染 basicInfo[key]）
		return []utilityFieldDef{
			{"account_no", "户号", "text", true, g},
			{"account_name", "户名", "text", true, g},
			{"usage_category", "用电类别", "text", true, g},
			{"voltage_level", "电压等级", "text", true, g},
			{"market_attr", "市场化属性", "text", true, g},
			{"supply_unit", "供电服务单位", "text", true, g},
			{"address", "用电地址", "text", true, g},
		}
	case "market-rows":
		// 市场化购电费明细行（与账单电费明细一一对应；分时子项按 名称(时段) 区分，field_key 唯一）
		return []utilityFieldDef{
			{"偏差电费(尖峰)", "偏差电费(尖峰)", "text", true, g},
			{"偏差电费(峰)", "偏差电费(峰)", "text", true, g},
			{"偏差电费(平)", "偏差电费(平)", "text", true, g},
			{"偏差电费(谷)", "偏差电费(谷)", "text", true, g},
			{"零售交易电费(尖峰)", "零售交易电费(尖峰)", "text", true, g},
			{"零售交易电费(峰)", "零售交易电费(峰)", "text", true, g},
			{"零售交易电费(平)", "零售交易电费(平)", "text", true, g},
			{"零售交易电费(谷)", "零售交易电费(谷)", "text", true, g},
			{"绿电交易电费(省间)(尖峰)", "绿电交易电费(省间)(尖峰)", "text", true, g},
			{"绿电交易电费(省间)(峰)", "绿电交易电费(省间)(峰)", "text", true, g},
			{"绿电交易电费(省间)(平)", "绿电交易电费(省间)(平)", "text", true, g},
			{"绿电交易电费(省间)(谷)", "绿电交易电费(省间)(谷)", "text", true, g},
			{"绿电交易电费(省内)(尖峰)", "绿电交易电费(省内)(尖峰)", "text", true, g},
			{"绿电交易电费(省内)(峰)", "绿电交易电费(省内)(峰)", "text", true, g},
			{"绿电交易电费(省内)(平)", "绿电交易电费(省内)(平)", "text", true, g},
			{"绿电交易电费(省内)(谷)", "绿电交易电费(省内)(谷)", "text", true, g},
			{"中长期偏差回收返还(用户)", "中长期偏差回收返还(用户)", "text", true, g},
			{"绿电环境价值电费(省外)", "绿电环境价值电费(省外)", "text", true, g},
			{"中长期超额申报费用返还(用户)", "中长期超额申报费用返还(用户)", "text", true, g},
			{"中长期超额申报回收费用", "中长期超额申报回收费用", "text", true, g},
			{"零售交易发电侧收益返还电费", "零售交易发电侧收益返还电费", "text", true, g},
			{"零售损益分摊电费", "零售损益分摊电费", "text", true, g},
		}
	case "trans-rows":
		// 输配电量电费明细行（分时子项按 名称(时段) 区分）
		return []utilityFieldDef{
			{"零售输配电费(尖峰)", "零售输配电费(尖峰)", "text", true, g},
			{"零售输配电费(峰)", "零售输配电费(峰)", "text", true, g},
			{"零售输配电费(平)", "零售输配电费(平)", "text", true, g},
			{"零售输配电费(谷)", "零售输配电费(谷)", "text", true, g},
		}
	case "sys-rows":
		// 系统运行费明细行
		return []utilityFieldDef{
			{"辅助服务费用", "辅助服务费用", "text", true, g},
			{"抽水蓄能容量电费", "抽水蓄能容量电费", "text", true, g},
			{"新能源机制差价分摊电费", "新能源机制差价分摊电费", "text", true, g},
			{"上网环节线损代理采购损益", "上网环节线损代理采购损益", "text", true, g},
			{"电价交叉补贴新增损益", "电价交叉补贴新增损益", "text", true, g},
			{"其他系统运行费用", "其他系统运行费用", "text", true, g},
			{"煤电容量电费", "煤电容量电费", "text", true, g},
			{"燃气机组容量电费", "燃气机组容量电费", "text", true, g},
		}
	case "gov-industrial-rows":
		// 政府基金及附加（工商业）明细行
		return []utilityFieldDef{
			{"小型水库移民后期扶持资金(地方)", "小型水库移民后期扶持资金(地方)", "text", true, g},
			{"农网还贷", "农网还贷", "text", true, g},
			{"库区移民基金", "库区移民基金", "text", true, g},
			{"国家重大水利工程建设基金", "国家重大水利工程建设基金", "text", true, g},
			{"可再生能源附加", "可再生能源附加", "text", true, g},
		}
	case "gov-residential-rows":
		// 政府基金及附加（居民）明细行
		return []utilityFieldDef{
			{"小型水库移民后期扶持资金(地方)", "小型水库移民后期扶持资金(地方)", "text", true, g},
			{"农网还贷", "农网还贷", "text", true, g},
			{"库区移民基金", "库区移民基金", "text", true, g},
			{"国家重大水利工程建设基金", "国家重大水利工程建设基金", "text", true, g},
			{"可再生能源附加", "可再生能源附加", "text", true, g},
		}
	default:
		return nil
	}
}

type utilityFieldDef struct {
	key   string
	label string
	typ   string
	def   bool
	group string
}

func utilityDefaultFieldConfigs(tenantID uint64, category, group string) []types.UtilityFieldConfig {
	now := timeNowUTC()
	var defs []utilityFieldDef
	switch category {
	case "electricity":
		if group == "" {
			// 不传 group：返回全部组默认（概况 + 各费用菜单列 + 明细 + 行配置）
			groups := []string{"overview", "market", "line", "trans", "sys", "gov-industrial", "catalog", "gov-residential", "capacity", "pf", "meter", "resident-meter", "meter-rows", "resident-meter-rows", "catalog-rows", "overview-rows", "market-rows", "trans-rows", "sys-rows", "gov-industrial-rows", "gov-residential-rows"}
			for _, g := range groups {
				defs = append(defs, electricityDefaultGroupFields(g)...)
			}
			break
		}
		defs = electricityDefaultGroupFields(group)
	case "water":
		defs = []utilityFieldDef{
			{"month", "月份", "text", true, ""},
			{"meter_count", "表计数", "number", true, ""},
			{"total_usage", "总用量", "number", true, ""},
			{"total_amount", "总金额", "amount", true, ""},
			{"remark", "备注", "text", true, ""},
		}
	case "gas":
		defs = []utilityFieldDef{
			{"month", "月份", "text", true, ""},
			{"meter_count", "表计数", "number", true, ""},
			{"total_usage", "总用量", "number", true, ""},
			{"total_amount", "总金额", "amount", true, ""},
			{"remark", "备注", "text", true, ""},
		}
	}
	out := make([]types.UtilityFieldConfig, 0, len(defs))
	for i, d := range defs {
		out = append(out, types.UtilityFieldConfig{
			ID:             uuid.NewString(),
			TenantID:       int64(tenantID),
			Category:       category,
			Group:          d.group,
			FieldKey:       d.key,
			Label:          d.label,
			FieldType:      d.typ,
			DefaultVisible: d.def,
			SortOrder:      i,
			IsCustom:       false,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 基本户信息（电费，支持多户）
// ---------------------------------------------------------------------------

// migrateLegacyBasicInfo 惰性迁移：旧 utility_basic_info 单条记录迁移为新表首个默认户。
func (h *UtilityHandler) migrateLegacyBasicInfo(ctx context.Context, tenantID uint64, category string) error {
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityBasicAccount{}).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	var legacy types.UtilityBasicInfo
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ?", tenantID, category).
		First(&legacy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if legacy.AccountNo == "" && legacy.AccountName == "" {
		return nil
	}
	now := timeNowUTC()
	return h.db.WithContext(ctx).Create(&types.UtilityBasicAccount{
		ID:            uuid.NewString(),
		TenantID:      int64(tenantID),
		Category:      category,
		Name:          "默认户",
		AccountNo:     legacy.AccountNo,
		AccountName:   legacy.AccountName,
		UsageCategory: legacy.UsageCategory,
		VoltageLevel:  legacy.VoltageLevel,
		MarketAttr:    legacy.MarketAttr,
		SupplyUnit:    legacy.SupplyUnit,
		Address:       legacy.Address,
		IsDefault:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}).Error
}

// ListUtilityBasicAccounts godoc
// @Summary      基本户列表
// @Description  按分类返回电费基本户列表（每户含自定义名称/户号/电能表编号/倍率/是否默认户）。
// @Router       /utilities/basic-accounts [get]
func (h *UtilityHandler) ListUtilityBasicAccounts(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	if err := h.migrateLegacyBasicInfo(ctx, tenantID, category); err != nil {
		logger.Errorf(ctx, "migrate legacy basic info failed: %v", err)
	}
	var accs []types.UtilityBasicAccount
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("is_default DESC, created_at ASC").
		Find(&accs).Error; err != nil {
		logger.Errorf(ctx, "list utility basic accounts failed: %v", err)
		c.Error(errors.NewInternalServerError("list basic accounts failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": accs})
}

// CreateUtilityBasicAccount godoc
// @Summary      新增基本户
// @Description  新增一个电费基本户；首个账户自动设为默认户，显式设默认时清除其它默认。
// @Router       /utilities/basic-accounts [post]
func (h *UtilityHandler) CreateUtilityBasicAccount(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	var req types.UtilityBasicAccount
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.Category = category
	req.AccountNo = strings.TrimSpace(req.AccountNo)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = req.AccountNo
	}
	if req.Name == "" {
		req.Name = "未命名户"
	}
	var cnt int64
	if err := h.db.WithContext(ctx).Model(&types.UtilityBasicAccount{}).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Count(&cnt).Error; err != nil {
		logger.Errorf(ctx, "count basic accounts failed: %v", err)
		c.Error(errors.NewInternalServerError("count basic accounts failed"))
		return
	}
	now := timeNowUTC()
	req.ID = uuid.NewString()
	req.TenantID = int64(tenantID)
	if cnt == 0 {
		req.IsDefault = true
	}
	req.CreatedAt = now
	req.UpdatedAt = now
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&types.UtilityBasicAccount{}).
				Where("tenant_id = ? AND category = ? AND is_default = ? AND deleted_at IS NULL", tenantID, category, true).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&req).Error
	})
	if err != nil {
		logger.Errorf(ctx, "create utility basic account failed: %v", err)
		c.Error(errors.NewInternalServerError("create basic account failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": req})
}

// UpdateUtilityBasicAccount godoc
// @Summary      更新基本户
// @Description  更新基本户信息；设为默认户时清除其它默认。
// @Router       /utilities/basic-accounts/:id [put]
func (h *UtilityHandler) UpdateUtilityBasicAccount(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var req types.UtilityBasicAccount
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	req.AccountNo = strings.TrimSpace(req.AccountNo)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = req.AccountNo
	}
	if req.Name == "" {
		req.Name = "未命名户"
	}
	now := timeNowUTC()
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&types.UtilityBasicAccount{}).
				Where("tenant_id = ? AND category = ? AND is_default = ? AND id <> ? AND deleted_at IS NULL", tenantID, category, true, id).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&types.UtilityBasicAccount{}).
			Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
			Updates(map[string]interface{}{
				"name":           req.Name,
				"account_no":     req.AccountNo,
				"account_name":   req.AccountName,
				"usage_category": req.UsageCategory,
				"voltage_level":  req.VoltageLevel,
				"market_attr":    req.MarketAttr,
				"supply_unit":    req.SupplyUnit,
				"address":        req.Address,
				"meter_no":       req.MeterNo,
				"ratio":          req.Ratio,
				"is_default":     req.IsDefault,
				"updated_at":     now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.NewNotFoundError("基本户不存在")
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "update utility basic account failed: %v", err)
		c.Error(errors.NewInternalServerError("update basic account failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}

// DeleteUtilityBasicAccount godoc
// @Summary      删除基本户
// @Description  删除基本户；若删除的是默认户且仍有其它户，默认转移到剩余第一个。
// @Router       /utilities/basic-accounts/:id [delete]
func (h *UtilityHandler) DeleteUtilityBasicAccount(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	id := secutils.SanitizeForLog(c.Param("id"))
	var target types.UtilityBasicAccount
	if err := h.db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND deleted_at IS NULL", id, tenantID).
		First(&target).Error; err != nil {
		c.Error(errors.NewNotFoundError("基本户不存在"))
		return
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&types.UtilityBasicAccount{}).
			Where("id = ? AND tenant_id = ?", id, tenantID).
			Update("deleted_at", timeNowUTC()).Error; err != nil {
			return err
		}
		if target.IsDefault {
			var next types.UtilityBasicAccount
			if err := tx.WithContext(ctx).
				Where("tenant_id = ? AND category = ? AND id <> ? AND deleted_at IS NULL", tenantID, category, id).
				Order("created_at ASC").First(&next).Error; err == nil {
				return tx.Model(&types.UtilityBasicAccount{}).
					Where("id = ?", next.ID).
					Update("is_default", true).Error
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf(ctx, "delete utility basic account failed: %v", err)
		c.Error(errors.NewInternalServerError("delete basic account failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已删除"})
}

// GetUtilityBasicInfo godoc
// @Summary      获取基本户信息（兼容：返回默认户）
// @Description  按分类返回电费默认基本户信息，未配置时返回空对象。
// @Router       /utilities/basic-info [get]
func (h *UtilityHandler) GetUtilityBasicInfo(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	if err := h.migrateLegacyBasicInfo(ctx, tenantID, category); err != nil {
		logger.Errorf(ctx, "migrate legacy basic info failed: %v", err)
	}
	var info types.UtilityBasicAccount
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("is_default DESC, created_at ASC").
		First(&info).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{}})
			return
		}
		logger.Errorf(ctx, "get utility basic info failed: %v", err)
		c.Error(errors.NewInternalServerError("get basic info failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": info})
}

// SaveUtilityBasicInfo godoc
// @Summary      保存基本户信息（兼容：写入/更新默认户）
// @Description  整组覆盖保存某分类的默认基本户信息（无默认户时新建）。
// @Router       /utilities/basic-info [put]
func (h *UtilityHandler) SaveUtilityBasicInfo(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	var req types.UtilityBasicAccount
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body: " + err.Error()))
		return
	}
	if err := h.migrateLegacyBasicInfo(ctx, tenantID, category); err != nil {
		logger.Errorf(ctx, "migrate legacy basic info failed: %v", err)
	}
	req.Category = category
	req.AccountNo = strings.TrimSpace(req.AccountNo)
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "默认户"
	}
	var existing types.UtilityBasicAccount
	err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("is_default DESC, created_at ASC").
		First(&existing).Error
	now := timeNowUTC()
	if err == gorm.ErrRecordNotFound {
		req.ID = uuid.NewString()
		req.TenantID = int64(tenantID)
		req.IsDefault = true
		req.CreatedAt = now
		req.UpdatedAt = now
		if cerr := h.db.WithContext(ctx).Create(&req).Error; cerr != nil {
			logger.Errorf(ctx, "save utility basic info failed: %v", cerr)
			c.Error(errors.NewInternalServerError("save basic info failed: " + cerr.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
		return
	}
	if err != nil {
		logger.Errorf(ctx, "save utility basic info failed: %v", err)
		c.Error(errors.NewInternalServerError("save basic info failed"))
		return
	}
	if err := h.db.WithContext(ctx).Model(&types.UtilityBasicAccount{}).
		Where("id = ? AND tenant_id = ?", existing.ID, tenantID).
		Updates(map[string]interface{}{
			"name":           req.Name,
			"account_no":     req.AccountNo,
			"account_name":   req.AccountName,
			"usage_category": req.UsageCategory,
			"voltage_level":  req.VoltageLevel,
			"market_attr":    req.MarketAttr,
			"supply_unit":    req.SupplyUnit,
			"address":        req.Address,
			"meter_no":       req.MeterNo,
			"ratio":          req.Ratio,
			"is_default":     true,
			"updated_at":     now,
		}).Error; err != nil {
		logger.Errorf(ctx, "save utility basic info failed: %v", err)
		c.Error(errors.NewInternalServerError("save basic info failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已保存"})
}
