package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

const maxExtractConfigVersions = 20

// getExtractConfig 按知识库 × scope × 证照类型读取生效中的提取规则配置；无则返回 nil。
func (h *BusinessExtractHandler) getExtractConfig(ctx context.Context, kbID, scope, certType string) *types.KbExtractConfig {
	var ec types.KbExtractConfig
	if err := h.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND scope = ? AND cert_type = ? AND enabled = TRUE AND deleted_at IS NULL",
			kbID, strings.TrimSpace(scope), strings.TrimSpace(certType)).
		First(&ec).Error; err != nil {
		return nil
	}
	return &ec
}

func validateExtractScope(scope string) bool {
	return scope == types.FleetCategoryScopeVehicle ||
		scope == types.FleetCategoryScopeDriver ||
		scope == types.FleetCategoryScopeMaintain
}

func normalizeExtractFields(fields []types.ExtractFieldConfig) []types.ExtractFieldConfig {
	seen := map[string]bool{}
	out := make([]types.ExtractFieldConfig, 0, len(fields))
	for _, f := range fields {
		name := strings.TrimSpace(f.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if f.Type == "" {
			f.Type = "string"
		}
		out = append(out, f)
	}
	return out
}

// GetExtractConfig godoc
// @Summary      获取提取规则配置
// @Description  按 kb_id + scope + cert_type 返回字段配置与高级 Prompt 模板；无配置时返回空结构。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id         path    string  true  "知识库ID"
// @Param        scope      query   string  false "vehicle|driver|maintain，默认 vehicle"
// @Param        cert_type  query   string  true  "证照类型"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/extract-config [get]
func (h *BusinessExtractHandler) GetExtractConfig(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	scope := strings.TrimSpace(c.Query("scope"))
	if scope == "" {
		scope = types.FleetCategoryScopeVehicle
	}
	certType := strings.TrimSpace(c.Query("cert_type"))
	if certType == "" {
		c.Error(errors.NewBadRequestError("cert_type is required"))
		return
	}
	if _, _, _, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID); err != nil {
		c.Error(err)
		return
	}
	ec := h.getExtractConfig(ctx, kbID, scope, certType)
	if ec == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"scope": scope, "cert_type": certType,
			"fields": []types.ExtractFieldConfig{}, "advanced_enabled": false, "prompt_template": "", "version": 0,
		}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ec})
}

// SaveExtractConfig godoc
// @Summary      保存提取规则配置
// @Description  保存字段配置与高级 Prompt 模板；每次保存 version+1 并写版本快照（保留最近 20 版）。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "知识库ID"
// @Param        body  body  object  true  "{scope, cert_type, fields[], advanced_enabled, prompt_template, remark}"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/extract-config [post]
func (h *BusinessExtractHandler) SaveExtractConfig(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, _, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	var req struct {
		Scope           string                    `json:"scope"`
		CertType        string                    `json:"cert_type"`
		Fields          []types.ExtractFieldConfig `json:"fields"`
		AdvancedEnabled bool                      `json:"advanced_enabled"`
		PromptTemplate  string                    `json:"prompt_template"`
		Remark          string                    `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = types.FleetCategoryScopeVehicle
	}
	if !validateExtractScope(scope) {
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain"))
		return
	}
	certType := strings.TrimSpace(req.CertType)
	if certType == "" {
		c.Error(errors.NewBadRequestError("cert_type is required"))
		return
	}
	fields := normalizeExtractFields(req.Fields)
	promptTemplate := strings.TrimSpace(req.PromptTemplate)
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	now := time.Now().UTC()

	// 字段名权威校验：若该证照类型已在证照配置中入库，提交的每个字段名必须存在于其启用的 subs 中，
	// 杜绝提取规则与证照配置双写漂移（证照配置是字段名的唯一权威源）。
	var cat types.FleetCategory
	catFound := h.db.WithContext(effCtx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", scope, certType).
		First(&cat).Error == nil
	if catFound {
		valid := map[string]bool{}
		for _, s := range cat.Subs {
			n := strings.TrimSpace(s.Name)
			if n != "" && s.Enabled {
				valid[n] = true
			}
		}
		for _, f := range fields {
			if !valid[f.Name] {
				c.Error(errors.NewBadRequestError(fmt.Sprintf("字段「%s」不在证照配置中，请先在证照配置中添加并启用该字段", f.Name)))
				return
			}
		}
	}

	var ec types.KbExtractConfig
	found := h.db.WithContext(effCtx).
		Where("knowledge_base_id = ? AND scope = ? AND cert_type = ? AND deleted_at IS NULL", kbID, scope, certType).
		First(&ec).Error == nil
	if found {
		ec.Version++
		ec.Fields = fields
		ec.AdvancedEnabled = req.AdvancedEnabled
		ec.PromptTemplate = promptTemplate
		ec.Enabled = true
		ec.UpdatedAt = now
		if err := h.db.WithContext(effCtx).Model(&types.KbExtractConfig{}).Where("id = ?", ec.ID).Updates(map[string]interface{}{
			"fields": mustJSON(fields), "advanced_enabled": req.AdvancedEnabled, "prompt_template": promptTemplate,
			"version": ec.Version, "enabled": true, "updated_at": now,
		}).Error; err != nil {
			logger.Errorf(ctx, "update extract config failed: %v", err)
			c.Error(errors.NewInternalServerError("保存提取规则失败"))
			return
		}
	} else {
		ec = types.KbExtractConfig{
			ID: uuid.NewString(), TenantID: int64(effectiveTenantID), KnowledgeBaseID: kbID,
			Scope: scope, CertType: certType, Fields: fields, AdvancedEnabled: req.AdvancedEnabled,
			PromptTemplate: promptTemplate, Version: 1, Enabled: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := h.db.WithContext(effCtx).Create(&ec).Error; err != nil {
			logger.Errorf(ctx, "create extract config failed: %v", err)
			c.Error(errors.NewInternalServerError("保存提取规则失败"))
			return
		}
	}

	// 版本快照 + 清理超出保留上限的历史
	ver := types.KbExtractConfigVersion{
		ID: uuid.NewString(), ConfigID: ec.ID, Version: ec.Version,
		Fields: fields, AdvancedEnabled: req.AdvancedEnabled, PromptTemplate: promptTemplate,
		Remark: strings.TrimSpace(req.Remark), CreatedAt: now,
	}
	if err := h.db.WithContext(effCtx).Create(&ver).Error; err != nil {
		logger.Warnf(ctx, "create extract config version failed: %v", err)
	}
	var keepIDs []string
	if err := h.db.WithContext(effCtx).Model(&types.KbExtractConfigVersion{}).
		Where("config_id = ?", ec.ID).Order("version DESC").Limit(maxExtractConfigVersions).Pluck("id", &keepIDs).Error; err == nil {
		if len(keepIDs) > 0 {
			_ = h.db.WithContext(effCtx).Where("config_id = ? AND id NOT IN ?", ec.ID, keepIDs).Delete(&types.KbExtractConfigVersion{}).Error
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "提取规则已保存",
		"data":    gin.H{"id": ec.ID, "scope": scope, "cert_type": certType, "version": ec.Version},
	})
}

// TestExtractConfig godoc
// @Summary      测试提取规则（沙箱）
// @Description  使用传入的（可为未保存的）字段配置与 Prompt 模板，对文本或知识库文件执行一次提取，不落库，返回结构化结果与缺失字段。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "知识库ID"
// @Param        body  body  object  true  "{scope, cert_type, text|knowledge_id, fields[], advanced_enabled, prompt_template}"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/extract-config/test [post]
func (h *BusinessExtractHandler) TestExtractConfig(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	if kbID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to test extract config"))
		return
	}
	var req struct {
		Scope           string                    `json:"scope"`
		CertType        string                    `json:"cert_type"`
		Text            string                    `json:"text"`
		KnowledgeID     string                    `json:"knowledge_id"`
		Fields          []types.ExtractFieldConfig `json:"fields"`
		AdvancedEnabled bool                      `json:"advanced_enabled"`
		PromptTemplate  string                    `json:"prompt_template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = types.FleetCategoryScopeVehicle
	}
	if !validateExtractScope(scope) {
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain"))
		return
	}
	certType := strings.TrimSpace(req.CertType)
	if certType == "" {
		c.Error(errors.NewBadRequestError("cert_type is required"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	// 测试内容：直接文本优先，否则复用知识库已解析文件
	var content string
	if strings.TrimSpace(req.Text) != "" {
		content = req.Text
	} else if kid := strings.TrimSpace(req.KnowledgeID); kid != "" {
		knowledge, gerr := h.kgService.GetKnowledgeByIDOnly(effCtx, kid)
		if gerr != nil {
			c.Error(errors.NewNotFoundError("knowledge not found"))
			return
		}
		if knowledge.KnowledgeBaseID != kbID {
			c.Error(errors.NewBadRequestError("knowledge does not belong to the given knowledge base"))
			return
		}
		if knowledge.ParseStatus != types.ParseStatusCompleted {
			c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
			return
		}
		chunks, cerr := h.chunkService.ListChunksByKnowledgeID(effCtx, kid)
		if cerr != nil {
			logger.Error(ctx, "Failed to list chunks for extract config test", cerr)
			c.Error(errors.NewInternalServerError("list chunks failed: " + cerr.Error()))
			return
		}
		content = buildFleetExtractionContent(knowledge.FileName, knowledge.Description, chunks)
	} else {
		c.Error(errors.NewBadRequestError("text 或 knowledge_id 至少提供一个"))
		return
	}
	if strings.TrimSpace(content) == "" {
		c.Error(errors.NewBadRequestError("无可用文本（文件可能为纯扫描件，无 OCR 文本）"))
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, merr := h.modelService.GetChatModel(effCtx, modelID)
	if merr != nil {
		logger.Error(ctx, "Failed to load extraction model for test", merr)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + merr.Error()))
		return
	}

	fields := normalizeExtractFields(req.Fields)
	ec := &types.KbExtractConfig{
		Scope: scope, CertType: certType, Fields: fields,
		AdvancedEnabled: req.AdvancedEnabled, PromptTemplate: strings.TrimSpace(req.PromptTemplate),
	}
	sysPrompt := service.BuildExtractSystemPrompt(ec)
	fieldNames := service.EnabledExtractFieldNames(fields)
	result, xerr := service.ExtractFleetDocumentFromContent(effCtx, chatModel, content, scope, certType, fieldNames, sysPrompt)
	if xerr != nil {
		logger.Error(ctx, "Extract config test model call failed", xerr)
		c.Error(errors.NewInternalServerError("提取失败: " + xerr.Error()))
		return
	}

	// 缺失字段：按配置字段顺序，空值/空数组/0 视为缺失
	missing := make([]string, 0)
	for _, f := range fields {
		if !f.Enabled {
			continue
		}
		if isEmptyExtractValue(result.Fields[f.Name]) {
			missing = append(missing, f.Name)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"doc_type": result.DocType,
			"fields":   result.Fields,
			"missing":  missing,
		},
	})
}

func isEmptyExtractValue(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	case float64:
		return t == 0
	case bool:
		return false
	default:
		return strings.TrimSpace(mustJSON(v)) == "" || mustJSON(v) == "null" || mustJSON(v) == "[]"
	}
}
