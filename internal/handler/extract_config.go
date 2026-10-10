package handler

import (
	"context"
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

// getExtractConfig 按知识库 × scope 读取生效中的提取规则配置；certType 非空时精确匹配类型，为空时回退整体规则。
func (h *BusinessExtractHandler) getExtractConfig(ctx context.Context, kbID, scope, certType string) *types.KbExtractConfig {
	var ec types.KbExtractConfig
	q := h.db.WithContext(ctx).
		Where("knowledge_base_id = ? AND scope = ? AND enabled = TRUE AND deleted_at IS NULL",
			kbID, strings.TrimSpace(scope))
	ct := strings.TrimSpace(certType)
	if ct != "" {
		q = q.Where("cert_type = ?", ct)
	} else {
		// 整体规则：cert_type 为空或 __all__ 哨兵
		q = q.Where("cert_type = ? OR cert_type = ?", "", "__all__")
	}
	if err := q.Order("updated_at DESC").First(&ec).Error; err != nil {
		return nil
	}
	return &ec
}

// resolveInvoiceRuleCfg 发票提取规则解析：优先按上传打标的分类规则（certType）读取，
// 分类规则缺失时回退「普通发票」规则（内置通用兜底），再缺失回退整体规则（'' / __all__）。
func (h *BusinessExtractHandler) resolveInvoiceRuleCfg(ctx context.Context, kbID, certType string) *types.KbExtractConfig {
	scope := types.FleetCategoryScopeInvoice
	if ct := strings.TrimSpace(certType); ct != "" {
		if cfg := h.getExtractConfig(ctx, kbID, scope, ct); cfg != nil {
			return cfg
		}
		if cfg := h.getExtractConfig(ctx, kbID, scope, "普通发票"); cfg != nil {
			return cfg
		}
	}
	return h.getExtractConfig(ctx, kbID, scope, "")
}

func validateExtractScope(scope string) bool {
	return scope == types.FleetCategoryScopeVehicle ||
		scope == types.FleetCategoryScopeDriver ||
		scope == types.FleetCategoryScopeMaintain ||
		scope == types.FleetCategoryScopeInvoice ||
		scope == "contract" ||
		scope == "regulation" ||
		scope == "award_punish"
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
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain|invoice|contract|regulation|award_punish"))
		return
	}
	certType := strings.TrimSpace(req.CertType)
	fields := normalizeExtractFields(req.Fields)
	promptTemplate := strings.TrimSpace(req.PromptTemplate)
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	now := time.Now().UTC()

	// 字段名权威校验：若该证照类型已在证照配置中入库，提交的每个字段名必须存在于其启用的 subs 中，
	// 杜绝提取规则与证照配置双写漂移（证照配置是字段名的唯一权威源）。
	// 整体规则（cert_type 为空）不按单证照类型校验字段归属，字段名权威由 categories[scope][0].subs 承担
	// tenant 口径：与 ListFleetCategories 完全一致——business scope 用全局 tenant_id=0 配置，
	// 其余 scope 用当前租户。禁止无 tenant 过滤 First()，否则会误读到其他租户的同名空分类（subs 为空）
	// 导致提交的字段被全部剔除、规则被静默清空。
	var cat types.FleetCategory
	catQ := h.db.WithContext(effCtx).
		Where("scope = ? AND name = ? AND deleted_at IS NULL", scope, certType)
	if isBusinessCategoryScope(scope) {
		catQ = catQ.Where("tenant_id = 0")
	} else {
		catQ = catQ.Where("tenant_id = ?", effectiveTenantID)
	}
	catFound := certType != "" && catQ.Order("sort_order ASC, created_at ASC").First(&cat).Error == nil
	if catFound {
		valid := map[string]bool{}
		for _, s := range cat.Subs {
			n := strings.TrimSpace(s.Name)
			if n != "" && s.Enabled {
				valid[n] = true
			}
		}
		// 容错降级：未知/已禁用字段仅记警告并忽略，不再整体 400 中断保存，
		// 避免「字段定义」调整后提取规则面板残留旧字段导致保存失败。
		for _, f := range fields {
			if !valid[f.Name] {
				logger.Warnf(ctx, "ignoring extract rule for unknown/disabled field: %s (scope=%s cert_type=%s)", f.Name, scope, certType)
			}
		}
		kept := fields[:0]
		for _, f := range fields {
			if valid[f.Name] {
				kept = append(kept, f)
			}
		}
		fields = kept
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
		// 结构体 Save：GORM serializer:json 将 Fields 序列化为 JSON 数组写入 jsonb 列，
		// 与读取路径（serializer 反序列化）严格一致；禁止再使用 mustJSON 字符串 + map Updates，
		// 否则写入形态与读取形态不一致会导致回填防覆盖判定误判。
		if err := h.db.WithContext(effCtx).Save(&ec).Error; err != nil {
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
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain|invoice|contract|regulation|award_punish"))
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

// invoiceRuleDescFor 按「字段名 × 分类名」生成内置提取规则底稿的字段描述/规则文案。
// 字段名以分类 subs.name 为权威（可能是中文名，如「车牌号」），用包含匹配归一判断。
func invoiceRuleDescFor(fieldName, catName string) (string, string) {
	name := strings.TrimSpace(fieldName)
	// 1) 英文键名精准分支：水电/通行费/其它票据等分类的 subs 使用英文键名
	//    （invoiceNo/invoiceDate/...），中文包含匹配无法命中，此前全部落入
	//    default「自定义字段」兜底。这里按字段语义 + 分类差异化给精准 desc/rule。
	switch name {
	case "invoiceNo":
		return "发票号码", "提取票面“发票号码 / 发票号”栏，数字或数字字母组合，去除多余空白。"
	case "invoiceDate":
		return "开票日期", "提取票面“开票日期”栏，统一为 YYYY-MM-DD。"
	case "invoiceType":
		switch {
		case strings.Contains(catName, "通行费"):
			return "发票类型", "识别为“通行费发票”；票面含“通行费”或高速/桥梁收费抬头即可判定。"
		case strings.Contains(catName, "水电") || strings.Contains(catName, "电费"):
			return "发票类型", "识别为“水电发票（电费/水费）”；票面含“电费/水费”或供电/供水公司抬头即可判定。"
		case strings.Contains(catName, "其它票据"):
			return "票据类型", "按票面标题识别：专用发票、普通发票、通行费发票、电费发票等；无法识别时保持为空。"
		default:
			return "发票类型", "从 专用发票、普通发票 中识别；含通行费/电费/水费抬头的归入对应细分分类。"
		}
	case "amount":
		return "不含税金额", "从票面“金额 / 不含税金额”栏提取数字金额，保留两位小数，不要货币符号。"
	case "taxRate":
		return "税率", "提取票面税率（如 13%、9%、6%、3%、1%）；一张票含多行明细且税率不同时，提取所有不重复税率并以英文逗号拼接（如：13%,9%,6%）。"
	case "tax":
		return "税额", "从票面“税额”栏提取数字金额，保留两位小数，不要货币符号。"
	case "totalAmount":
		return "价税合计", "从票面“价税合计 / 合计”栏提取数字金额，保留两位小数，不要货币符号。"
	case "buyerName":
		if strings.Contains(catName, "水电") || strings.Contains(catName, "电费") {
			return "购买方（受票方）名称", "购买方通常为用电/用水的企业名称（户号对应单位），从票面“购买方”或“户名”栏识别提取。"
		}
		return "购买方（受票方）名称", "从票面“购买方 / 付款方 / 户名”栏识别提取。"
	case "buyerTaxNo":
		return "购买方纳税人识别号", "提取“购买方纳税人识别号 / 统一社会信用代码”栏，18 位数字或字母数字组合。"
	case "sellerName":
		if strings.Contains(catName, "水电") || strings.Contains(catName, "电费") {
			return "销售方（开票方）名称", "销售方通常为国家电网、南方电网或当地自来水/燃气公司，从票面“销售方”栏识别提取。"
		}
		return "销售方（开票方）名称", "从票面“销售方 / 收款方”栏识别提取。"
	case "sellerTaxNo":
		return "销售方纳税人识别号", "提取“销售方纳税人识别号”栏，18 位数字或字母数字组合。"
	case "issuer":
		return "开票人", "从票面开票人、收款人或经办人栏识别提取姓名；原文无则留空。"
	case "fileName":
		return "源文件名", "取上传文件的原始文件名，无需识别。"
	case "extractStatus":
		return "提取状态", "系统自动判断：提取成功 / 解析失败 / 提取失败；无需人工填写。"
	case "tags":
		return "发票标签", "根据货物/劳务名称或销方类型归纳业务标签（如 差旅、办公用品、加油）；多个标签以数组返回。"
	case "remark":
		switch {
		case strings.Contains(catName, "水电") || strings.Contains(catName, "电费"):
			return "用电/用水明细汇总", "请从票面的明细行中，尝试提取总用电量/用水量、单价及计费起止日期，并将这些信息格式化后汇总至备注字段中返回。"
		case strings.Contains(catName, "通行费"):
			return "备注", "保留票面备注原文；若备注中包含车牌号、通行日期等关键信息，同时按字段口径输出到对应独立字段。"
		default:
			return "备注", "保留票面备注原文。"
		}
	case "items":
		return "项目明细行", "提取票面货物/服务明细：名称、数量、单价、税率；多行明细逐行输出。"
	}
	// 2) 中文名包含匹配（用户自定义中文键名，如 车牌号/购买方名称/收款事由）
	switch {
	case strings.Contains(name, "车牌号") || strings.Contains(name, "车牌"):
		// 通行费发票关键动态字段：票面常无独立表头，车牌号藏在备注栏文字中
		return "通行车辆的号牌号码",
			"格式通常为省份简称加字母和数字（如：渝C87567）。如果票面上没有独立的‘车牌号’表头，请务必仔细阅读‘备注’栏文字，从中分离并提取出车牌号，直接输出车牌号本身，不要带有‘车牌号：’等前缀。"
	case strings.Contains(name, "通行") || strings.Contains(name, "入口") || strings.Contains(name, "出口"):
		return "通行区间（入口/出口收费站）", "从票面出入口信息或备注中提取，格式如：重庆江北收费站-重庆大学城收费站。"
	case strings.Contains(name, "购买方") || strings.Contains(name, "购方") || strings.Contains(name, "付款方") || strings.Contains(name, "买方") || strings.Contains(name, "客户"):
		if strings.Contains(catName, "水电") || strings.Contains(catName, "电费") {
			return "用电/用水的企业名称", "购买方通常为用电/用水的企业名称（户号对应单位），从票面‘购买方’或‘户名’栏识别提取。"
		}
		return "购买方（付款方）名称", "从票面‘购买方’/‘付款方’/‘户名’栏识别提取。"
	case strings.Contains(name, "销售方") || strings.Contains(name, "销方") || strings.Contains(name, "收款方") || strings.Contains(name, "卖方"):
		if strings.Contains(catName, "水电") || strings.Contains(catName, "电费") {
			return "供电/供水单位名称", "销售方通常为国家电网、南方电网或当地自来水/燃气公司，从票面‘销售方’栏识别提取。"
		}
		return "销售方（收款方）名称", "从票面‘销售方’/‘收款方’栏识别提取。"
	case strings.Contains(name, "税号"):
		return "纳税人识别号", "从票面税号栏提取完整纳税人识别号（18 位数字或字母数字组合）。"
	case strings.Contains(name, "发票号码") || strings.Contains(name, "发票号") || strings.Contains(name, "票据编号") || strings.Contains(name, "编号") || strings.Contains(name, "号码"):
		return "票据号码", "从票面‘发票号码/票据号码’栏识别提取，去除多余空白。"
	case strings.Contains(name, "日期"):
		return "开票/入账日期", "从票面日期栏识别提取，格式统一为 YYYY-MM-DD。"
	case strings.Contains(name, "税率"):
		return "税率", "提取票面税率（如 13%、9%、6%、3%、1%）；一张票含多行明细且税率不同时，提取所有不重复税率并以英文逗号拼接（如：13%,9%,6%）。"
	case strings.Contains(name, "金额") || strings.Contains(name, "小计"):
		return "不含税金额", "从票面‘金额/小计’栏提取数字金额，保留两位小数，不要货币符号。"
	case strings.Contains(name, "税额"):
		return "税额", "从票面‘税额’栏提取数字金额，保留两位小数，不要货币符号。"
	case strings.Contains(name, "价税合计") || strings.Contains(name, "合计"):
		return "价税合计金额", "从票面‘价税合计/合计’栏提取数字金额，保留两位小数，不要货币符号。"
	case strings.Contains(name, "开票人") || strings.Contains(name, "收款人") || strings.Contains(name, "经办"):
		return "开票/经办人", "从票面开票人、收款人或经办人栏识别提取姓名。"
	case strings.Contains(name, "备注"):
		if strings.Contains(catName, "水电") || strings.Contains(catName, "电费") {
			return "用电/用水明细汇总", "请从票面的明细行中，尝试提取总用电量/用水量、单价及计费起止日期，并将这些信息格式化后汇总至备注字段中返回。"
		}
		if strings.Contains(catName, "通行费") {
			return "备注", "保留票面备注原文；若备注中包含车牌号、通行日期等关键信息，同时按字段口径输出到对应独立字段。"
		}
		return "备注", "保留票面备注原文。"
	case strings.Contains(name, "明细") || strings.Contains(name, "items"):
		return "项目明细行", "提取票面货物/服务明细：名称、数量、单价、税率；多行明细逐行输出。"
	default:
		if strings.Contains(catName, "其它票据") {
			return "非标准票据业务信息", "此为非标准格式票据（如三联收据、手写票、非正规机打票等）。请尽最大能力识别并提取票面上的付款方（购买方）、收款方（销售方）、开票日期和合计金额。对于无法归入常规字段的有效业务信息（如事由、经办人等），请全部整合汇总提取至‘备注’字段。"
		}
		return "自定义字段", "从票面及备注信息中尽力识别并提取该字段对应内容。"
	}
}

// defaultInvoicePromptTemplate 按分类生成默认高级 Prompt 模板。
// 模板使用 {{document_text}}、{{fields_schema}} 插槽，由提取引擎渲染；
// 仅在配置的 prompt_template 为空时回填（不覆盖用户已保存模板）。
func defaultInvoicePromptTemplate(catName string) string {
	name := strings.TrimSpace(catName)
	switch {
	case strings.Contains(name, "通行费"):
		return "你是一个通行费发票信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。\n" +
			"通行费发票票面通常包含：发票号码、开票日期、车牌号、通行区间、金额、税率、税额、价税合计、购买方、销售方、开票人。\n" +
			"注意：车牌号常常没有独立表头，而是隐藏在“备注”栏文字中（格式如“车牌号：渝C87567”），必须仔细阅读备注栏，将车牌号、通行日期起止等信息分离提取到对应独立字段，不要整体堆入备注。\n" +
			"需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON；未找到的字段输出 null，不臆造内容。"
	case strings.Contains(name, "水电") || strings.Contains(name, "电费"):
		return "你是一个水电发票信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。\n" +
			"水电发票的购买方通常为用电/用水的企业名称（户号对应单位），销售方通常为国家电网、南方电网或当地自来水/燃气公司。\n" +
			"请提取票面的发票号码、开票日期、金额、税率、税额、价税合计、购买方（含税号）、销售方（含税号）、开票人；\n" +
			"并从明细行中尝试提取总用电量/用水量、单价及计费起止日期，格式化汇总至备注字段。\n" +
			"需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON；未找到的字段输出 null，不臆造内容。"
	case strings.Contains(name, "其它票据"):
		return "你是一个票据信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。\n" +
			"此为非标准格式票据（如三联收据、手写票、非正规机打票等）。请尽最大能力识别并提取票面上的付款方（购买方）、收款方（销售方）、开票日期和合计金额。\n" +
			"对于无法归入常规字段的有效业务信息（如事由、经办人等），请全部整合汇总提取至“备注”字段。\n" +
			"需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON；未找到的字段输出 null，不臆造内容。"
	default:
		return "你是一个发票信息提取助手。请从以下文档中提取结构化信息：{{document_text}}。\n" +
			"需要提取的字段定义见 {{fields_schema}}，严格按字段名输出 JSON，未找到的字段输出 null，不臆造内容。"
	}
}

// invoiceGenericRuleFor 返回该分类底稿 default 分支的泛用兜底 rule 文案，
// 用于识别「未精配」的 seed 泛用规则（可升级为按字段精准规则），不误伤用户手写文案。
func invoiceGenericRuleFor(catName string) string {
	if strings.Contains(catName, "其它票据") {
		return "此为非标准格式票据（如三联收据、手写票、非正规机打票等）。请尽最大能力识别并提取票面上的付款方（购买方）、收款方（销售方）、开票日期和合计金额。对于无法归入常规字段的有效业务信息（如事由、经办人等），请全部整合汇总提取至‘备注’字段。"
	}
	return "从票面及备注信息中尽力识别并提取该字段对应内容。"
}

// mergeInvoiceRuleFields 字段级增量合并（防覆盖铁律的字段级实现）：
//   - 底稿有、现状缺失的字段 → 追加（desc/rule 按底稿）；
//   - 已有字段 rule 为空，或 rule 等于底稿泛用兜底文案（seed 未精配）→ 升级 desc/rule；
//   - 已有字段 rule 非空且非泛用兜底（用户手工精配）→ 严格保留，不覆盖。
// 返回合并后的字段列表与是否有变化。
func mergeInvoiceRuleFields(cur, draft []types.ExtractFieldConfig, catName string) ([]types.ExtractFieldConfig, bool) {
	generic := invoiceGenericRuleFor(catName)
	changed := false
	byName := make(map[string]*types.ExtractFieldConfig, len(cur)+len(draft))
	order := make([]string, 0, len(cur)+len(draft))
	for i := range cur {
		n := strings.TrimSpace(cur[i].Name)
		if n == "" {
			continue
		}
		// 全字段精准分支升级：不依赖底稿是否包含该字段（如 subs.enabled=false 的
		// fileName 不在底稿里），只要 invoiceRuleDescFor 对当前字段名有精准分支
		// （desc != 「自定义字段」），且现状是未精配（desc=自定义字段 / rule 空 /
		// rule=泛用兜底），就按精准分支升级 desc/rule。
		rule := strings.TrimSpace(cur[i].Rule)
		desc := strings.TrimSpace(cur[i].Desc)
		if desc == "自定义字段" || rule == "" || (generic != "" && rule == generic) {
			if d, r := invoiceRuleDescFor(n, catName); d != "自定义字段" && (cur[i].Desc != d || cur[i].Rule != r) {
				cur[i].Desc = d
				cur[i].Rule = r
				changed = true
			}
		}
		if _, ok := byName[n]; !ok {
			order = append(order, n)
		}
		byName[n] = &cur[i]
	}
	for _, d := range draft {
		n := strings.TrimSpace(d.Name)
		if n == "" {
			continue
		}
		c, ok := byName[n]
		if !ok {
			cp := d
			byName[n] = &cp
			order = append(order, n)
			changed = true
			continue
		}
		rule := strings.TrimSpace(c.Rule)
		desc := strings.TrimSpace(c.Desc)
		// 未精配判定：rule 为空、rule 为底稿泛用兜底文案、或 desc 仍为 default 分支的
		// 「自定义字段」（说明该字段从未被精准配置过，旧 seed 残留），一律按底稿升级。
		if rule == "" || desc == "自定义字段" || (generic != "" && rule == generic) {
			if c.Desc != d.Desc || c.Rule != d.Rule {
				c.Desc = d.Desc
				c.Rule = d.Rule
				changed = true
			}
		}
	}
	out := make([]types.ExtractFieldConfig, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, changed
}

// invoiceRuleTypeFor 将 FleetCategorySub.DataType 映射为 ExtractFieldConfig.Type。
func invoiceRuleTypeFor(dt string) string {
	switch strings.TrimSpace(dt) {
	case "number":
		return "number"
	case "array", "items":
		return "array"
	case "date":
		return "string"
	default:
		return "string"
	}
}

// defaultInvoiceRuleFields 按分类 subs（名称权威）生成内置提取规则底稿字段。
// 底稿覆盖：通行费发票（车牌号/通行区间）、水电发票（购/销方与用量明细）、其它票据（泛用兜底）。
// 其余分类返回 nil（保持现状，不主动建配置、不覆盖）。
func defaultInvoiceRuleFields(catName string, subs []types.FleetCategorySub) []types.ExtractFieldConfig {
	name := strings.TrimSpace(catName)
	if name == "" || len(subs) == 0 {
		return nil
	}
	covered := strings.Contains(name, "通行费") || strings.Contains(name, "水电") ||
		strings.Contains(name, "电费") || strings.Contains(name, "其它票据")
	if !covered {
		return nil
	}
	out := make([]types.ExtractFieldConfig, 0, len(subs))
	for _, s := range subs {
		if strings.TrimSpace(s.Name) == "" || !s.Enabled {
			continue
		}
		desc, rule := invoiceRuleDescFor(s.Name, name)
		out = append(out, types.ExtractFieldConfig{
			Name:    s.Name,
			Label:   s.Name,
			Desc:    desc,
			Type:    invoiceRuleTypeFor(s.DataType),
			Rule:    rule,
			Enabled: true,
		})
	}
	return out
}

// SeedInvoiceExtractRuleBackfill 幂等回填发票分类提取规则底稿（启动时调用一次）。
// 判定规则（防覆盖铁律）：
//   - 该 知识库×分类 已存在 extract-config 且 fields 非空（用户已配置）→ 严格跳过，
//     绝不 Update 覆盖，保证「用户保存的规则 → 重启后仍在」（验收点）。
//   - 记录存在但 fields 为空（历史空配置）→ 仅对底稿覆盖的分类填充底稿。
//   - 记录不存在 → 仅对底稿覆盖的分类创建；其余分类不主动建空配置。
func (h *BusinessExtractHandler) SeedInvoiceExtractRuleBackfill(ctx context.Context) {
	var kbs []types.KnowledgeBase
	if err := h.db.WithContext(ctx).Select("id").Find(&kbs).Error; err != nil {
		logger.Warnf(ctx, "seed invoice extract rules: list knowledge bases failed: %v", err)
		return
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND enabled = TRUE AND deleted_at IS NULL", types.FleetCategoryScopeInvoice).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "seed invoice extract rules: list categories failed: %v", err)
		return
	}
	now := time.Now().UTC()
	for _, kb := range kbs {
		for _, cat := range cats {
			name := strings.TrimSpace(cat.Name)
			if name == "" {
				continue
			}
			fields := defaultInvoiceRuleFields(name, cat.Subs)
			baseQ := "knowledge_base_id = ? AND scope = ? AND cert_type = ? AND deleted_at IS NULL"
			var exists int64
			if err := h.db.WithContext(ctx).Model(&types.KbExtractConfig{}).
				Where(baseQ, kb.ID, types.FleetCategoryScopeInvoice, name).Count(&exists).Error; err != nil {
				logger.Warnf(ctx, "seed invoice extract rules: count %s/%s failed: %v", kb.ID, name, err)
				continue
			}
			if exists > 0 {
				// 防覆盖铁律（字段级实现）：不覆盖用户已保存的非空规则；
				// 但对底稿覆盖分类做增量补齐——缺失字段追加、空规则/seed 泛用兜底
				// 规则升级为按字段精准规则、空高级模板回填默认模板。取最新一行合并
				// （与前端 getExtractConfig 读取口径一致）。
				if len(fields) == 0 {
					continue
				}
				var latest types.KbExtractConfig
				if lerr := h.db.WithContext(ctx).
					Where(baseQ, kb.ID, types.FleetCategoryScopeInvoice, name).
					Order("updated_at DESC").First(&latest).Error; lerr != nil {
					logger.Warnf(ctx, "seed invoice extract rules: load %s/%s failed: %v", kb.ID, name, lerr)
					continue
				}
				merged, changed := mergeInvoiceRuleFields(latest.Fields, fields, name)
				if strings.TrimSpace(latest.PromptTemplate) == "" {
					latest.PromptTemplate = defaultInvoicePromptTemplate(name)
					changed = true
				}
				if !changed {
					continue
				}
				latest.Fields = merged
				latest.Version++
				latest.Enabled = true
				latest.UpdatedAt = now
				if uerr := h.db.WithContext(ctx).Save(&latest).Error; uerr != nil {
					logger.Warnf(ctx, "seed invoice extract rules: backfill %s/%s failed: %v", kb.ID, name, uerr)
				}
				continue
			}
			if len(fields) == 0 {
				continue
			}
			ec := types.KbExtractConfig{
				ID: uuid.NewString(), TenantID: 0, KnowledgeBaseID: kb.ID,
				Scope: types.FleetCategoryScopeInvoice, CertType: name, Fields: fields,
				AdvancedEnabled: false, PromptTemplate: defaultInvoicePromptTemplate(name),
				Version: 1, Enabled: true, CreatedAt: now, UpdatedAt: now,
			}
			if cerr := h.db.WithContext(ctx).Create(&ec).Error; cerr != nil {
				logger.Warnf(ctx, "seed invoice extract rules: create %s/%s failed: %v", kb.ID, name, cerr)
			}
		}
	}
}
