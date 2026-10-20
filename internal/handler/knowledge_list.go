package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// invoiceCategoryHitSet 返回分类归属判定用的「已启用分类名集合」：复用
// enabledInvoiceCategoryNames（单一权威源），并剔除「其它票据」。
//
// 「其它票据」是系统内置兜底分类（seed invoice-misc），但它的统计/筛选口径是
// **排除式桶**而非真实分类，因此不能作为命中目标 —— 否则一份上传时选了
// 「其它票据」、票面却是通行费的票会永远困在桶里，表格列还显示通行费发票，
// 出现「列与桶分裂」（2026-10-20 回归）。
func (h *BusinessExtractHandler) invoiceCategoryHitSet(ctx context.Context) []string {
	all := h.enabledInvoiceCategoryNames(ctx)
	out := make([]string, 0, len(all))
	for _, n := range all {
		if n != "其它票据" {
			out = append(out, n)
		}
	}
	return out
}

// ListInvoiceRecords 发票级聚合列表
func (h *BusinessExtractHandler) ListInvoiceRecords(c *gin.Context) {
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
	var taxRate *float64
	if tr := c.Query("tax_rate"); tr != "" {
		if v, err := strconv.ParseFloat(tr, 64); err == nil {
			taxRate = &v
		}
	}
	sortBy := c.Query("sort_by")
	if sortBy != "" && sortBy != "invoice_no" && sortBy != "invoice_date" {
		c.Error(apperrors.NewBadRequestError("invalid sort_by: must be invoice_no or invoice_date"))
		return
	}
	sortOrder := c.Query("sort_order")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		c.Error(apperrors.NewBadRequestError("invalid sort_order: must be asc or desc"))
		return
	}
	// known_categories：启用分类名集合（逗号分隔），供「其它票据」排除式筛选使用。
	// 服务端同时会用 invoiceCategoryHitSet 实时加载一份作为分类归属判定的
	// 权威源；前端传参仅作为兼容兜底，不再承担口径责任。
	var knownCategories []string
	if kc := c.Query("known_categories"); kc != "" {
		for _, name := range strings.Split(kc, ",") {
			if name = strings.TrimSpace(name); name != "" {
				knownCategories = append(knownCategories, name)
			}
		}
	}
	filter := types.InvoiceListFilter{
		Keyword:           c.Query("q"),
		InvoiceType:       c.Query("invoice_type"),
		KnownCategories:   knownCategories,
		EnabledCategories: h.invoiceCategoryHitSet(ctx),
		TaxRate:           taxRate,
		Status:            c.Query("status"),
		DateFrom:          c.Query("date_from"),
		DateTo:            c.Query("date_to"),
		SortBy:            sortBy,
		SortOrder:         sortOrder,
		Page:              page,
		PageSize:          pageSize,
	}

	result, err := h.businessSvc.ListInvoiceRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list invoice records", err)
		c.Error(apperrors.NewInternalServerError("list invoice records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result.Data,
		"total":      result.Total,
		"sum_amount": result.SumAmount,
		"sum_tax":    result.SumTax,
		"sum_total":  result.SumTotal,
		"page":       result.Page,
		"page_size":  result.PageSize,
	})
}

// InvoiceOverviewStats 发票台账概览统计
func (h *BusinessExtractHandler) InvoiceOverviewStats(c *gin.Context) {
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
	stats, err := h.businessSvc.InvoiceOverviewStats(effCtx, kbID, h.invoiceCategoryHitSet(ctx))
	if err != nil {
		logger.Error(ctx, "Failed to load invoice overview stats", err)
		c.Error(apperrors.NewInternalServerError("load invoice overview stats failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// ListInvoiceTaxRates 发票税率去重列表
func (h *BusinessExtractHandler) ListInvoiceTaxRates(c *gin.Context) {
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
	rates, err := h.businessSvc.ListInvoiceTaxRates(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to list invoice tax rates", err)
		c.Error(apperrors.NewInternalServerError("list invoice tax rates failed: " + err.Error()))
		return
	}
	if rates == nil {
		rates = []types.InvoiceTaxRateCount{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rates})
}

// ListContractRecords 合同级聚合列表
func (h *BusinessExtractHandler) ListContractRecords(c *gin.Context) {
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
	filter := types.ContractListFilter{
		Keyword:       c.Query("q"),
		ContractType:  c.Query("contract_type"),
		FulfillStatus: c.Query("fulfill_status"),
		Status:        c.Query("status"),
		DateFrom:      c.Query("date_from"),
		DateTo:        c.Query("date_to"),
		Page:          page,
		PageSize:      pageSize,
	}

	result, err := h.businessSvc.ListContractRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list contract records", err)
		c.Error(apperrors.NewInternalServerError("list contract records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result.Data,
		"total":      result.Total,
		"sum_amount": result.SumAmount,
		"sum_tax":    result.SumTax,
		"sum_total":  result.SumTotal,
		"page":       result.Page,
		"page_size":  result.PageSize,
	})
}

// ContractOverviewStats 合同台账概览统计（全量只读聚合，不受列表筛选影响）
func (h *BusinessExtractHandler) ContractOverviewStats(c *gin.Context) {
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
	stats, err := h.businessSvc.ContractOverviewStats(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to load contract overview stats", err)
		c.Error(apperrors.NewInternalServerError("load contract overview stats failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// ListContractTypes 合同类型去重列表
func (h *BusinessExtractHandler) ListContractTypes(c *gin.Context) {
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
	types_, err := h.businessSvc.ListContractTypes(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to list contract types", err)
		c.Error(apperrors.NewInternalServerError("list contract types failed: " + err.Error()))
		return
	}
	if types_ == nil {
		types_ = []types.ContractTypeCount{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": types_})
}

// ListRegulationRecords 制度级聚合列表
func (h *BusinessExtractHandler) ListRegulationRecords(c *gin.Context) {
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
	// known_categories：启用分类名集合（逗号分隔），供「其它制度」排除式筛选使用
	var knownCategories []string
	if kc := c.Query("known_categories"); kc != "" {
		for _, name := range strings.Split(kc, ",") {
			if name = strings.TrimSpace(name); name != "" {
				knownCategories = append(knownCategories, name)
			}
		}
	}
	filter := types.RegulationListFilter{
		Keyword:         c.Query("q"),
		RegType:         c.Query("reg_type"),
		KnownCategories: knownCategories,
		Status:          c.Query("status"),
		DateFrom:        c.Query("date_from"),
		DateTo:          c.Query("date_to"),
		Page:            page,
		PageSize:        pageSize,
	}

	result, err := h.businessSvc.ListRegulationRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list regulation records", err)
		c.Error(apperrors.NewInternalServerError("list regulation records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ListRegulationTypes 制度类型去重列表
func (h *BusinessExtractHandler) ListRegulationTypes(c *gin.Context) {
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
	rates, err := h.businessSvc.ListRegulationTypes(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to list regulation types", err)
		c.Error(apperrors.NewInternalServerError("list regulation types failed: " + err.Error()))
		return
	}
	if rates == nil {
		rates = []types.RegulationTypeCount{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rates})
}

// ListAwardPunishRecords 奖惩记录聚合列表
func (h *BusinessExtractHandler) ListAwardPunishRecords(c *gin.Context) {
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
	// known_categories：启用分类名集合（逗号分隔），供「其它通知」排除式筛选使用
	var knownCategories []string
	if kc := c.Query("known_categories"); kc != "" {
		for _, name := range strings.Split(kc, ",") {
			if name = strings.TrimSpace(name); name != "" {
				knownCategories = append(knownCategories, name)
			}
		}
	}
	filter := types.AwardPunishListFilter{
		Keyword:         c.Query("q"),
		ApType:          c.Query("ap_type"),
		KnownCategories: knownCategories,
		Status:          c.Query("status"),
		DateFrom:        c.Query("date_from"),
		DateTo:          c.Query("date_to"),
		Page:            page,
		PageSize:        pageSize,
	}

	result, err := h.businessSvc.ListAwardPunishRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list award/punish records", err)
		c.Error(apperrors.NewInternalServerError("list award/punish records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ListAwardPunishTypes 奖惩类型去重列表
func (h *BusinessExtractHandler) ListAwardPunishTypes(c *gin.Context) {
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
	rates, err := h.businessSvc.ListAwardPunishTypes(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to list award/punish types", err)
		c.Error(apperrors.NewInternalServerError("list award/punish types failed: " + err.Error()))
		return
	}
	if rates == nil {
		rates = []types.AwardPunishTypeCount{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rates})
}

// ListUtilityBillRecords 电费账单记录列表
func (h *BusinessExtractHandler) ListUtilityBillRecords(c *gin.Context) {
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
	filter := types.UtilityBillListFilter{
		Keyword:  c.Query("q"),
		Status:   c.Query("status"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
		Kind:     c.Query("kind"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.businessSvc.ListUtilityBillRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list utility bill records", err)
		c.Error(apperrors.NewInternalServerError("list utility bill records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// ListSolarBillRecords 光伏账单记录列表
func (h *BusinessExtractHandler) ListSolarBillRecords(c *gin.Context) {
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
	filter := types.UtilityBillListFilter{
		Keyword:  c.Query("q"),
		Status:   c.Query("status"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.businessSvc.ListSolarBillRecords(effCtx, kbID, filter)
	if err != nil {
		logger.Error(ctx, "Failed to list solar bill records", err)
		c.Error(apperrors.NewInternalServerError("list solar bill records failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"data":      result.Data,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

// GetRecognitionConfig 获取识别规则配置
func (h *BusinessExtractHandler) GetRecognitionConfig(c *gin.Context) {
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
	cfg, err := h.kgService.GetRecognitionConfig(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to get recognition config", err)
		c.Error(apperrors.NewInternalServerError("get recognition config failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// SaveRecognitionConfig 保存识别规则配置
func (h *BusinessExtractHandler) SaveRecognitionConfig(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(apperrors.NewForbiddenError("No permission to edit recognition config"))
		return
	}
	var cfg types.RecognitionConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid recognition config body: " + err.Error()))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	if err := h.kgService.SaveRecognitionConfig(effCtx, kbID, &cfg); err != nil {
		logger.Error(ctx, "Failed to save recognition config", err)
		c.Error(apperrors.NewInternalServerError("save recognition config failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "识别规则已保存"})
}

// ReassessRecognition 按规则重新评估删除历史
func (h *BusinessExtractHandler) ReassessRecognition(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(apperrors.NewForbiddenError("No permission to reassess recognition"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	reassessable, ok := h.kgService.(interface {
		ReassessRecognition(ctx context.Context, kbID string) (int, error)
	})
	if !ok {
		c.Error(apperrors.NewInternalServerError("reassess not supported"))
		return
	}
	restored, err := reassessable.ReassessRecognition(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to reassess recognition", err)
		c.Error(apperrors.NewInternalServerError("reassess recognition failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "重新评估完成",
		"data":    map[string]interface{}{"restored": restored},
	})
}

// RegulationOverviewStats 制度台账概览统计（全量只读聚合，不受列表筛选影响）
func (h *BusinessExtractHandler) RegulationOverviewStats(c *gin.Context) {
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
	stats, err := h.businessSvc.RegulationOverviewStats(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to load regulation overview stats", err)
		c.Error(apperrors.NewInternalServerError("load regulation overview stats failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}

// AwardPunishOverviewStats 奖惩台账概览统计（全量只读聚合，不受列表筛选影响）
func (h *BusinessExtractHandler) AwardPunishOverviewStats(c *gin.Context) {
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
	stats, err := h.businessSvc.AwardPunishOverviewStats(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to load award_punish overview stats", err)
		c.Error(apperrors.NewInternalServerError("load award_punish overview stats failed"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": stats})
}
