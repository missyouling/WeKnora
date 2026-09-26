package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

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
	filter := types.InvoiceListFilter{
		Keyword:     c.Query("q"),
		InvoiceType: c.Query("invoice_type"),
		TaxRate:     taxRate,
		Status:      c.Query("status"),
		DateFrom:    c.Query("date_from"),
		DateTo:      c.Query("date_to"),
		Page:        page,
		PageSize:    pageSize,
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
	filter := types.RegulationListFilter{
		Keyword:  c.Query("q"),
		RegType:  c.Query("reg_type"),
		Status:   c.Query("status"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
		Page:     page,
		PageSize: pageSize,
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
	filter := types.AwardPunishListFilter{
		Keyword:  c.Query("q"),
		ApType:   c.Query("ap_type"),
		Status:   c.Query("status"),
		DateFrom: c.Query("date_from"),
		DateTo:   c.Query("date_to"),
		Page:     page,
		PageSize: pageSize,
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
