package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// 本文件是「发票设置 → 智能字段生成 / 智能配置规则」的两个 HTTP 入口。
//
// **不污染数据原则（贯穿全文件）**：两个接口都是纯建议计算接口。
// 它们读 database（取 KB / 分类现状）但**不写任何表**：
//   - ai-suggest-fields 不碰 fleet_categories（sub_fields 建议只回给前端，
//     由用户在「字段定义」Tab 点保存时才经UpdateFleetCategory 落库）；
//   - ai-suggest-rules 不碰 knowledge_extract_configs（规则建议只回给前端，
//     由用户在「提取规则」Tab 点保存时才经 SaveExtractConfig 落库）。
//
// 之所以坚持"建议不落库"：这两类配置一旦被 AI 写进库，用户就再也分不清
// 哪条是自己配的、哪条是模型猜的；下次重启的 seed 合并逻辑也无法正确判断
// 该不该覆盖。让人点一次保存，既是确认动作，也是留痕。

// maxSuggestSampleBytes 样票上传体积上限，与附件链路的GetMaxFileSizeMB 同源。
func maxSuggestSampleBytes() int64 {
	return int64(secutils.GetMaxFileSizeMB()) * 1024 * 1024
}

// AISuggestFields godoc
// @Summary      智能字段生成（样票 → 字段建议）
// @Description  上传一张样票（或直接传票面文本），由模型判定票面真实存在的字段并映射到系统标准字段字典，
//
//	返回三类建议（标准字段启用 / 明细子列 / 新增自定义字段）与缓存的样票文本。
//	**纯建议接口：不写任何表**，落库须由用户在「字段定义」Tab 点保存。
//
//	支持 multipart/form-data（上传样票）与 application/json（直传文本）两种 Content-Type。
//
// @Tags         知识管理
// @Accept       multipart/form-data, application/json
// @Produce      json
// @Param        id          path   string  true  "知识库ID"
// @Param        file        form   file    false "样票文件（PDF/图片/文本）"
// @Param        scope       form   string  false "固定 invoice"
// @Param        cert_type   form   string  true  "发票分类名"
// @Param        text        form   string  false "已有票面文本（与 file 二选一）"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/extract-config/ai-suggest-fields [post]
func (h *BusinessExtractHandler) AISuggestFields(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to generate field suggestions"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	scope, certType, text, truncated, ferr := h.bindSuggestSampleRequest(c)
	if ferr != nil {
		c.Error(ferr)
		return
	}
	if !isBusinessCategoryScope(scope) {
		c.Error(errors.NewBadRequestError("智能字段生成当前仅支持发票/合同/制度/通知 scope"))
		return
	}
	if certType == "" {
		c.Error(errors.NewBadRequestError("cert_type is required"))
		return
	}
	if strings.TrimSpace(text) == "" {
		c.Error(errors.NewBadRequestError("请上传样票文件，或直接传入票面文本"))
		return
	}
	// 文本直传也按同一上限截断，避免超大文本撑爆 Prompt
	if len([]rune(text)) > maxSuggestTextRunes() {
		r := []rune(text)
		text = string(r[:maxSuggestTextRunes()])
		truncated = true
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, merr := h.modelService.GetChatModel(effCtx, modelID)
	if merr != nil {
		logger.Error(ctx, "Failed to load extraction model for field suggest", merr)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + merr.Error()))
		return
	}

	dict := service.InvoiceStandardFieldDictionary()
	result, serr := service.SuggestInvoiceFieldsFromText(effCtx, chatModel, certType, text, dict)
	if serr != nil {
		logger.Error(ctx, "AI field suggest failed", serr)
		c.Error(errors.NewInternalServerError("智能字段生成失败: " + serr.Error()))
		return
	}
	result.CertType = certType
	result.SampleText = text
	result.SampleTextTruncated = truncated
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// AISuggestRules godoc
// @Summary      智能配置规则（字段 → 规则建议）
// @Description  按当前分类的字段清单生成 desc / 自然语言 rule / required / RE2 正则兜底与高级 Prompt 模板。
//
//	正则经三重护栏（后端补洞白名单 → regexp.Compile → 样票唯一命中）校验，
//	未通过的字段返回 regex_status 与 regex_reason 说明原因，绝不下发无效正则。
//	**纯建议接口：不写任何表**，落库须由用户在「提取规则」Tab 点保存。
//
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id   path  string  true  "知识库ID"
// @Param        body  body  object  true  "{scope, cert_type, fields[], sample_text, overwrite}"
// @Success      200  {object}  map[string]interface{}
// @Router       /knowledge-bases/{id}/extract-config/ai-suggest-rules [post]
func (h *BusinessExtractHandler) AISuggestRules(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to generate rule suggestions"))
		return
	}
	var req struct {
		Scope      string                     `json:"scope"`
		CertType   string                     `json:"cert_type"`
		Fields     []service.AIRuleFieldInput `json:"fields"`
		SampleText string                     `json:"sample_text"`
		Overwrite  bool                       `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError("invalid request body"))
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = types.FleetCategoryScopeInvoice
	}
	if !validateExtractScope(scope) {
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain|invoice|contract|regulation|award_punish"))
		return
	}
	certType := strings.TrimSpace(req.CertType)
	if certType == "" {
		c.Error(errors.NewBadRequestError("cert_type is required"))
		return
	}
	fields := normalizeAIRuleFieldInputs(req.Fields)
	if len(fields) == 0 {
		c.Error(errors.NewBadRequestError("fields is required"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	sampleText := strings.TrimSpace(req.SampleText)

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, merr := h.modelService.GetChatModel(effCtx, modelID)
	if merr != nil {
		logger.Error(ctx, "Failed to load extraction model for rule suggest", merr)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + merr.Error()))
		return
	}

	suggested, serr := service.SuggestInvoiceRulesForFields(effCtx, chatModel, certType, fields, sampleText)
	if serr != nil {
		logger.Error(ctx, "AI rule suggest failed", serr)
		c.Error(errors.NewInternalServerError("智能配置规则失败: " + serr.Error()))
		return
	}
	// 现状值来自已保存的 extract-config（只读）：用于 overwrite=false 时判断"哪些字段是空的"。
	// 注意此处**只读不写**：SaveExtractConfig 由用户点「保存」时另行触发。
	existing := loadSuggestExistingFieldValues(effCtx, h, kbID, scope, certType)
	values, changed := service.MergeAIRuleSuggestions(fields, suggested.Fields, existing, req.Overwrite, sampleText)

	result := service.NewAISuggestRulesResult(certType, fields, values)
	// changed 是布尔；AppliedFieldCount 需要数量。数量对用户没有额外信息量
	// （前端逐字段展示是否变化更实用），故只在发生变化时置 1 作为提示位。
	if changed {
		result.AppliedFieldCount = 1
	}
	result.SampleTextUsed = sampleText != ""
	if strings.TrimSpace(suggested.PromptTemplate) != "" {
		result.PromptTemplate = strings.TrimSpace(suggested.PromptTemplate)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// bindSuggestSampleRequest 同时兼容 multipart 与 JSON 两种取样方式。
// 返回 (scope, certType, 文本, 是否被截断)；无任何可用文本时返回错误。
func (h *BusinessExtractHandler) bindSuggestSampleRequest(c *gin.Context) (string, string, string, bool, error) {
	ct := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
	if strings.HasPrefix(ct, "multipart/form-data") {
		scope := strings.TrimSpace(c.PostForm("scope"))
		certType := strings.TrimSpace(c.PostForm("cert_type"))
		text := c.PostForm("text")
		fh, err := c.FormFile("file")
		if err != nil {
			if strings.TrimSpace(text) == "" {
				return "", "", "", false, errors.NewBadRequestError("请上传样票文件，或在 text 字段传入票面文本")
			}
			return scope, certType, strings.TrimSpace(text), false, nil
		}
		if fh.Size <= 0 || fh.Size > int64(maxSuggestSampleBytes()) {
			return "", "", "", false, errors.NewBadRequestError(
				fmt.Sprintf("样票文件大小需在 1 字节 ~ %dMB 之间", secutils.GetMaxFileSizeMB()))
		}
		f, oerr := fh.Open()
		if oerr != nil {
			return "", "", "", false, errors.NewBadRequestError("读取样票文件失败")
		}
		defer f.Close()
		data, rerr := io.ReadAll(io.LimitReader(f, maxSuggestSampleBytes()+1))
		if rerr != nil {
			return "", "", "", false, errors.NewBadRequestError("读取样票文件失败")
		}
		if int64(len(data)) > maxSuggestSampleBytes() {
			return "", "", "", false, errors.NewBadRequestError(
				fmt.Sprintf("样票文件超过 %dMB 上限", secutils.GetMaxFileSizeMB()))
		}
		// 文本直传优先：用户既传了 text 又传了 file 时，以 text 为准（更可控）
		if strings.TrimSpace(text) != "" {
			return scope, certType, strings.TrimSpace(text), false, nil
		}
		parsed, perr := service.ParseFileForExtractSuggest(ctxOf(c), service.ExtractSuggestParser{
			Documents: h.documentReader,
			Model:     h.modelService,
			Tenant:    h.tenantService,
		}, service.ExtractSuggestInput{
			FileName:           fh.Filename,
			Ext:                filepath.Ext(fh.Filename),
			Data:               data,
			ParserEngine:       strings.TrimSpace(c.PostForm("parser_engine")),
			VLMModelID:         h.suggestVLMModelID(ctxOf(c)),
			OCRMaxPages:        0,
			ImageUnderstanding: true,
		})
		if perr != nil {
			logger.Warnf(ctxOf(c), "sample parse failed: %v", perr)
			return "", "", "", false, errors.NewBadRequestError("样票解析失败: " + perr.Error())
		}
		if strings.TrimSpace(parsed.Text) == "" {
			return "", "", "", false, errors.NewBadRequestError(
				"样票未能解析出可用文本（可能是纯图片且未配置 OCR 模型）")
		}
		return scope, certType, parsed.Text, parsed.Truncated, nil
	}

	// JSON：直传票面文本
	var req struct {
		Scope        string `json:"scope"`
		CertType     string `json:"cert_type"`
		Text         string `json:"text"`
		ParserEngine string `json:"parser_engine"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return "", "", "", false, errors.NewBadRequestError("invalid request body")
	}
	return strings.TrimSpace(req.Scope), strings.TrimSpace(req.CertType), strings.TrimSpace(req.Text), false, nil
}

// ctxOf 取请求 context（保持 handler 层零魔法）。
func ctxOf(c *gin.Context) context.Context {
	return c.Request.Context()
}

// maxSuggestTextRunes 票面文本上限（与 service 层常量同源，避免 handler 侧另立口径）。
func maxSuggestTextRunes() int {
	return service.ExtractSuggestMaxTextRunes()
}

// suggestVLMModelID 选一个可用的 VLM 模型做扫描件 OCR 兜底。
// 优先取租户已配置的 VLLM 模型；没有就返回空串（跳过 OCR，不是错误——
// 纯文本 PDF 走 docreader 即可，OCR 只是扫描件的兜底）。
func (h *BusinessExtractHandler) suggestVLMModelID(ctx context.Context) string {
	if h.modelService == nil {
		return ""
	}
	models, err := h.modelService.ListModels(ctx)
	if err != nil {
		return ""
	}
	for _, m := range models {
		if m == nil || m.Type != types.ModelTypeVLLM {
			continue
		}
		if id := strings.TrimSpace(m.ID); id != "" {
			return id
		}
	}
	return ""
}

// normalizeAIRuleFieldInputs 去空去重（字段名权威源是请求方传入的分类 subs）。
func normalizeAIRuleFieldInputs(in []service.AIRuleFieldInput) []service.AIRuleFieldInput {
	seen := map[string]bool{}
	out := make([]service.AIRuleFieldInput, 0, len(in))
	for _, f := range in {
		name := strings.TrimSpace(f.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, service.AIRuleFieldInput{Name: name, DataType: strings.TrimSpace(f.DataType)})
	}
	return out
}

// loadSuggestExistingFieldValues 只读已保存的 extract-config，返回「字段名 → 现值」。
// 用于 overwrite=false 时识别"哪些字段是空的、可以安全填入建议"。
func loadSuggestExistingFieldValues(
	ctx context.Context, h *BusinessExtractHandler, kbID, scope, certType string,
) map[string]service.AIRuleFieldValues {
	ec := h.getExtractConfig(ctx, kbID, scope, certType)
	out := map[string]service.AIRuleFieldValues{}
	if ec == nil {
		return out
	}
	for _, f := range ec.Fields {
		n := strings.TrimSpace(f.Name)
		if n == "" {
			continue
		}
		out[n] = service.AIRuleFieldValues{
			Desc:     strings.TrimSpace(f.Desc),
			Rule:     strings.TrimSpace(f.Rule),
			Required: f.Required,
			Regex:    strings.TrimSpace(f.Regex),
		}
	}
	return out
}
