package handler

import (
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
// @Description  按分类返回字段配置（electricity/water/gas），未配置时返回内置默认字段。
// @Router       /utilities/field-configs [get]
func (h *UtilityHandler) ListUtilityFieldConfigs(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
	tenantID, _ := utilityTenantID(c)
	var cfgs []types.UtilityFieldConfig
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category).
		Order("sort_order ASC, created_at ASC").
		Find(&cfgs).Error; err != nil {
		logger.Errorf(ctx, "list utility field configs failed: %v", err)
		c.Error(errors.NewInternalServerError("list field configs failed"))
		return
	}
	if len(cfgs) == 0 {
		cfgs = utilityDefaultFieldConfigs(tenantID, category)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfgs})
}

// SaveUtilityFieldConfigs godoc
// @Summary      保存字段配置
// @Description  整组覆盖保存某分类的字段配置（先删后插，事务内完成）。
// @Router       /utilities/field-configs [post]
func (h *UtilityHandler) SaveUtilityFieldConfigs(c *gin.Context) {
	ctx := c.Request.Context()
	category := strings.TrimSpace(c.Query("category"))
	if category == "" {
		c.Error(errors.NewBadRequestError("category is required (electricity/water/gas)"))
		return
	}
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
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND category = ?", tenantID, category).
			Delete(&types.UtilityFieldConfig{}).Error; err != nil {
			return err
		}
		now := timeNowUTC()
		for i := range req {
			cfg := req[i]
			cfg.ID = uuid.NewString()
			cfg.TenantID = int64(tenantID)
			cfg.Category = category
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
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "字段配置已保存"})
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

	query := h.db.WithContext(ctx).Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, category)
	if month != "" {
		query = query.Where("month = ?", month)
	}
	if q != "" {
		query = query.Where("month LIKE ? OR remark LIKE ?", "%"+q+"%", "%"+q+"%")
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
	c.JSON(http.StatusOK, gin.H{"success": true, "data": records})
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
	computeMeterRecord(&req)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&req).Error; err != nil {
			return err
		}
		for i := range req.Items {
			it := &req.Items[i]
			it.ID = uuid.NewString()
			it.RecordID = req.ID
			it.CreatedAt = now
			it.UpdatedAt = now
			computeMeterItem(it)
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
	computeMeterRecord(&req)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			computeMeterItem(it)
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
func computeMeterItem(it *types.UtilityMeterItem) {
	it.Usage = round2(it.EndReading - it.StartReading)
	it.Amount = round2(it.Usage * it.UnitPrice)
}

// computeMeterRecord recalculates meter_count / total_usage / total_amount.
func computeMeterRecord(r *types.UtilityMeterRecord) {
	var usage, amount float64
	for i := range r.Items {
		computeMeterItem(&r.Items[i])
		usage += r.Items[i].Usage
		amount += r.Items[i].Amount
	}
	r.MeterCount = len(r.Items)
	r.TotalUsage = round2(usage)
	r.TotalAmount = round2(amount)
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
func utilityDefaultFieldConfigs(tenantID uint64, category string) []types.UtilityFieldConfig {
	now := timeNowUTC()
	var defs []struct {
		key   string
		label string
		typ   string
		def   bool
	}
	switch category {
	case "electricity":
		defs = []struct {
			key   string
			label string
			typ   string
			def   bool
		}{
			{"bill_period_start", "账单周期起", "date", true},
			{"bill_period_end", "账单周期止", "date", false},
			{"account_no", "户号", "text", true},
			{"account_name", "户名", "text", true},
			{"usage_category", "用电类别", "text", true},
			{"voltage_level", "电压等级", "text", false},
			{"supply_unit", "供电服务单位", "text", false},
			{"total_kwh", "本期电量", "number", true},
			{"total_amount", "本期电费", "amount", true},
			{"mom_change", "环比", "text", false},
			{"avg_price", "平均电价", "number", true},
			{"power_factor", "功率因数", "number", true},
			{"due_date", "交费截止", "date", false},
			{"industrial_amount", "工商业电费", "amount", false},
			{"residential_amount", "居民电费", "amount", false},
			{"pf_adjust_amount", "功率因数调整电费", "amount", false},
			{"grand_total", "合计", "amount", false},
			{"address", "用电地址", "text", false},
			{"market_attr", "市场化属性", "text", false},
			{"print_date", "账单打印日期", "date", false},
			{"prev_kwh", "上期电量", "number", false},
			{"deep_peak_kwh", "尖峰电量", "number", false},
			{"peak_kwh", "峰电量", "number", false},
			{"flat_kwh", "平电量", "number", false},
			{"valley_kwh", "谷电量", "number", false},
			{"reactive_kwh", "正向无功电量", "number", false},
			{"capacity", "容量", "number", false},
			{"capacity_price", "容量电价", "number", false},
			{"capacity_fee", "输配容量电费", "amount", false},
			{"demand", "需量值", "number", false},
			{"pf_standard", "功率因数标准", "number", false},
			{"adjust_ratio", "调整系数", "number", false},
		}
	case "water":
		defs = []struct {
			key   string
			label string
			typ   string
			def   bool
		}{
			{"month", "月份", "text", true},
			{"meter_count", "表计数", "number", true},
			{"total_usage", "总用量", "number", true},
			{"total_amount", "总金额", "amount", true},
			{"remark", "备注", "text", true},
		}
	case "gas":
		defs = []struct {
			key   string
			label string
			typ   string
			def   bool
		}{
			{"month", "月份", "text", true},
			{"meter_count", "表计数", "number", true},
			{"total_usage", "总用量", "number", true},
			{"total_amount", "总金额", "amount", true},
			{"remark", "备注", "text", true},
		}
	}
	out := make([]types.UtilityFieldConfig, 0, len(defs))
	for i, d := range defs {
		out = append(out, types.UtilityFieldConfig{
			ID:             uuid.NewString(),
			TenantID:       int64(tenantID),
			Category:       category,
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
