package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// 能耗台账（energy scope）的 HTTP 接口。
//
// 与合同/制度/通知同一形态（分页列表 + 概览统计 + 类型下拉），
// 但筛选参数是**月粒度账期**（period_from/period_to，YYYY-MM），
// 不是合同那样的日粒度日期区间。

// enabledEnergyCategoryNames 加载启用中的能耗分类名（scope=energy）。
//
// 分类归属的「第一命中」判定用它，与合同/制度同口径：服务端实时加载，
// 不依赖前端传参猜分类（前端漏传会让分类卡与表格列全错）。
func (h *BusinessExtractHandler) enabledEnergyCategoryNames(ctx context.Context) []string {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", "energy").
		Order("sort_order ASC, created_at ASC").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load enabled energy categories failed: %v", err)
		return nil
	}
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		if n := strings.TrimSpace(c.Name); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// ListEnergyRecords 能耗台账级聚合列表。
func (h *BusinessExtractHandler) ListEnergyRecords(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, pageSize := parseSandboxPagination(c)
	filter := types.EnergyListFilter{
		Keyword:       c.Query("q"),
		UtilityType:   c.Query("utility_type"),
		BillingPeriod: c.Query("billing_period"),
		PeriodFrom:    c.Query("period_from"),
		PeriodTo:      c.Query("period_to"),
		ReadDateFrom:  c.Query("read_date_from"),
		ReadDateTo:    c.Query("read_date_to"),
		Status:        c.Query("status"),
		Page:          page,
		PageSize:      pageSize,
		// 分类归属权威源：服务端实时加载启用分类
		EnabledCategories: h.enabledEnergyCategoryNames(ctx),
	}

	result, err := h.businessSvc.ListEnergyRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list energy records", err)
		c.Error(apperrors.NewInternalServerError("list energy records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"data":         result.Data,
		"total":        result.Total,
		"sum_usage":    result.SumUsage,
		"sum_total":    result.SumTotal,
		"sum_previous": result.SumPrevious,
		"sum_current":  result.SumCurrent,
		"issue_count":  result.IssueCount,
		"page":         result.Page,
		"page_size":    result.PageSize,
	})
}

// EnergyOverviewStats 能耗台账概览统计（全量只读聚合，不受列表筛选影响）。
func (h *BusinessExtractHandler) EnergyOverviewStats(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	stats, err := h.businessSvc.EnergyOverviewStats(effCtx, kbID, h.enabledEnergyCategoryNames(ctx)...)
	if err != nil {
		logger.Error(ctx, "Failed to load energy overview stats", err)
		c.Error(apperrors.NewInternalServerError("load energy overview stats failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// ListEnergyTypes 能耗类型去重列表（供筛选下拉）。
func (h *BusinessExtractHandler) ListEnergyTypes(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	types_, err := h.businessSvc.ListEnergyTypes(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to list energy types", err)
		c.Error(apperrors.NewInternalServerError("list energy types failed: " + err.Error()))
		return
	}
	if types_ == nil {
		types_ = []types.EnergyTypeCount{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": types_})
}
