package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// fleetDocCustomMetadata mirrors the custom_metadata shape persisted on a
// knowledge row after fleet document extraction.
type fleetDocCustomMetadata struct {
	Kind          string `json:"kind"`
	Scope         string `json:"scope"`
	DocType       string `json:"doc_type"`
	FleetCertType string `json:"fleet_cert_type,omitempty"`
	ExtractStatus string `json:"extract_status"`
	ExtractError  string `json:"extract_error"`
}

func fleetArchiveRecordType(scope string) string {
	switch scope {
	case types.FleetCategoryScopeDriver:
		return types.FleetRecordDriverArchive
	case types.FleetCategoryScopeMaintain:
		return types.FleetRecordMaintainArchive
	default:
		return types.FleetRecordVehicleArchive
	}
}

// ExtractFleetDocument godoc
// @Summary      提取车队证照/维保文件字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取证照字段，写入 fleet_records（record_type=*-archive）并同步档案分类（大项-小项）。幂等：同一知识重复调用会覆盖更新。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        body         body  object{scope=string}  true  "scope: vehicle|driver|maintain"
// @Success      200  {object}  map[string]interface{}  "提取成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-fleet-document [post]
func (h *KnowledgeHandler) ExtractFleetDocument(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	var body struct {
		Scope    string `json:"scope"`
		CertType string `json:"cert_type"`
	}
	_ = c.ShouldBindJSON(&body)
	scope := strings.TrimSpace(body.Scope)
	if scope == "" {
		scope = types.FleetCategoryScopeVehicle
	}
	if scope != types.FleetCategoryScopeVehicle && scope != types.FleetCategoryScopeDriver && scope != types.FleetCategoryScopeMaintain {
		c.Error(errors.NewBadRequestError("scope 必须是 vehicle|driver|maintain"))
		return
	}

	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract fleet document fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for fleet document extraction", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	if knowledge.ParseStatus != types.ParseStatusCompleted {
		c.Error(errors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish"))
		return
	}

	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		c.Error(errors.NewBadRequestError("no extraction model configured for the knowledge base"))
		return
	}
	chatModel, err := h.modelService.GetChatModel(effCtx, modelID)
	if err != nil {
		logger.Error(ctx, "Failed to load extraction model", err)
		c.Error(errors.NewInternalServerError("get extraction model failed: " + err.Error()))
		return
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeID(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to list chunks for fleet document extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	// 与合同/发票提取一致：拼入文档摘要（VLM 生成的 description），
	// 否则图片类证照（仅图片引用 chunk）会因无 OCR 文本而提取为空。
	content := buildFleetExtractionContent(knowledge.FileName, knowledge.Description, chunks)

	// 上传时指定了证照类型：以用户所选类型为准（覆盖模型自动判定）。
	// 重试等场景下请求可能不带 cert_type：优先回读文件上已打标的类型。
	certType := strings.TrimSpace(body.CertType)
	if certType == "" {
		var prev fleetDocCustomMetadata
		if pm, err := json.Marshal(knowledge.CustomMetadata); err == nil {
			_ = json.Unmarshal(pm, &prev)
		}
		certType = strings.TrimSpace(prev.FleetCertType)
	}

	// 标记 processing：防止上传弹窗与列表轮询在 LLM 调用期间并发重复触发，
	// 导致同一文件被提取两次、fleet_records 产生重复记录。
	processingMeta, _ := json.Marshal(fleetDocCustomMetadata{
		Kind: "fleet_" + scope + "_document", Scope: scope, FleetCertType: certType, ExtractStatus: "processing",
	})
	_ = h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(processingMeta))

	if strings.TrimSpace(content) == "" {
		failMeta, _ := json.Marshal(fleetDocCustomMetadata{
			Kind: "fleet_" + scope + "_document", Scope: scope, FleetCertType: certType, ExtractStatus: "failed",
			ExtractError: "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist fleet doc no-text state: %v", serr)
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"scope": scope, "extract_status": "failed"},
		})
		return
	}

	// 提取字段与 Prompt：优先使用用户可配置的提取规则（知识库×scope×证照类型），
	// 无配置时回退到内置字段定义/分类小项 + 内置 Prompt 拼装。
	fieldNames := h.fleetFieldNames(effCtx, int64(effectiveTenantID), scope, certType)
	extractSystemPrompt := ""
	if ec := h.getExtractConfig(effCtx, kbID, scope, certType); ec != nil {
		if names := service.EnabledExtractFieldNames(ec.Fields); len(names) > 0 {
			fieldNames = names
		}
		extractSystemPrompt = service.BuildExtractSystemPrompt(ec)
	}

	result, xerr := service.ExtractFleetDocumentFromContent(effCtx, chatModel, content, scope, certType, fieldNames, extractSystemPrompt)
	if xerr != nil {
		failMeta, _ := json.Marshal(fleetDocCustomMetadata{
			Kind: "fleet_" + scope + "_document", Scope: scope, FleetCertType: certType, ExtractStatus: "failed", ExtractError: xerr.Error(),
		})
		if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist fleet doc failed-state: %v", serr)
		}
		logger.Error(ctx, "Fleet document extraction model call failed", xerr)
		c.Error(errors.NewInternalServerError("fleet document extraction failed: " + xerr.Error()))
		return
	}

	if certType != "" {
		result.DocType = certType
	}

	// 落库 fleet_records（幂等：按 doc_knowledge_id 更新）
	recordType := fleetArchiveRecordType(scope)
	now := time.Now().UTC()
	vehicleID := h.matchVehicleIDByPlate(effCtx, int64(effectiveTenantID), result.VehicleNo)
	var existing types.FleetRecord
	found := h.db.WithContext(effCtx).Where("tenant_id = ? AND record_type = ? AND doc_knowledge_id = ? AND deleted_at IS NULL",
		int64(effectiveTenantID), recordType, knowledgeID).First(&existing).Error == nil
	rec := types.FleetRecord{
		RecordType:     recordType,
		VehicleID:      vehicleID,
		RecordMonth:    "",
		RecordDate:     "",
		Amount:         0,
		Data:           result.Fields,
		DocType:        result.DocType,
		FileName:       knowledge.FileName,
		DocKnowledgeID: knowledgeID,
	}
	if found {
		rec.ID = existing.ID
		rec.TenantID = existing.TenantID
		if err := h.db.WithContext(effCtx).Model(&types.FleetRecord{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"vehicle_id":       rec.VehicleID,
			"data":             mustJSON(rec.Data),
			"doc_type":         rec.DocType,
			"file_name":        rec.FileName,
			"doc_knowledge_id": rec.DocKnowledgeID,
			"updated_at":       now,
		}).Error; err != nil {
			logger.Errorf(ctx, "update fleet record from extraction failed: %v", err)
			c.Error(errors.NewInternalServerError("update fleet record failed"))
			return
		}
	} else {
		rec.ID = uuid.NewString()
		rec.TenantID = int64(effectiveTenantID)
		rec.CreatedAt = now
		rec.UpdatedAt = now
		// 并发兜底：唯一索引 idx_fleet_records_kid_uq 冲突时转为更新，杜绝重复落库。
		if err := h.db.WithContext(effCtx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "record_type"}, {Name: "doc_knowledge_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"vehicle_id", "data", "doc_type", "file_name", "doc_knowledge_id", "updated_at"}),
		}).Create(&rec).Error; err != nil {
			logger.Errorf(ctx, "create fleet record from extraction failed: %v", err)
			c.Error(errors.NewInternalServerError("create fleet record failed"))
			return
		}
	}

	// 同步档案分类：大项=doc_type（不存在则建），字段名作为小项追加（去重）
	h.syncFleetCategory(effCtx, int64(effectiveTenantID), scope, result.DocType, result.Fields)

	meta, _ := json.Marshal(fleetDocCustomMetadata{
		Kind: "fleet_" + scope + "_document", Scope: scope, DocType: result.DocType, FleetCertType: certType, ExtractStatus: "success",
	})
	if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(meta)); serr != nil {
		logger.Warnf(ctx, "Failed to persist fleet doc success meta: %v", serr)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "提取成功",
		"data": map[string]interface{}{
			"scope": scope, "doc_type": result.DocType, "extract_status": "success",
		},
	})
}

// matchVehicleIDByPlate 按车牌号匹配现有车辆配置（模糊匹配：去除空格/汉字括号差异）。
func (h *KnowledgeHandler) matchVehicleIDByPlate(ctx context.Context, tenantID int64, plate string) string {
	plate = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(plate), " ", ""))
	if plate == "" {
		return ""
	}
	var vehicles []types.FleetVehicle
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&vehicles).Error; err != nil {
		return ""
	}
	for _, v := range vehicles {
		p := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(v.PlateNo), " ", ""))
		if p == plate {
			return v.ID
		}
	}
	return ""
}

// fleetFieldNames 返回指定证照类型的提取字段清单。
// 内置证照类型（车辆登记证书/行驶证/道路运输经营许可证/道路运输证/保险单/驾驶证/从业资格证等）
// 优先使用内置字段定义（与前端 BUILTIN_CERTS 一致，也是用户在配置里设置的默认字段）；
// 用户自建类型（无内置定义）回退到该类型分类下已启用的小项。
func (h *KnowledgeHandler) fleetFieldNames(ctx context.Context, tenantID int64, scope, certType string) []string {
	certType = strings.TrimSpace(certType)
	if certType == "" {
		return nil
	}
	if builtin := service.FleetBuiltinFieldNames(scope, certType); len(builtin) > 0 {
		return builtin
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL",
		tenantID, scope, certType).Find(&cats).Error; err != nil || len(cats) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for i := range cats {
		for _, s := range cats[i].Subs {
			name := strings.TrimSpace(s.Name)
			if name == "" || !s.Enabled || seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// syncFleetCategory 把提取出的证照类型（大项）同步到档案分类配置。
// 已有同名分类时不再追加模型字段（避免污染用户配置的小项），仅缺失时创建。
func (h *KnowledgeHandler) syncFleetCategory(ctx context.Context, tenantID int64, scope, docType string, fields map[string]any) {
	docType = strings.TrimSpace(docType)
	if docType == "" {
		return
	}
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL",
		tenantID, scope, docType).First(&cat).Error; err == nil {
		return
	}
	// 大项不存在 → 创建，并把提取出的字段名作为小项（去重、排序稳定）
	cat = types.FleetCategory{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Scope:     scope,
		Name:      docType,
		Subs:      []types.FleetCategorySub{},
		SortOrder: 0,
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	var maxOrder int
	h.db.WithContext(ctx).Model(&types.FleetCategory{}).
		Where("tenant_id = ? AND scope = ? AND deleted_at IS NULL", tenantID, scope).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder)
	cat.SortOrder = maxOrder + 1
	if len(fields) > 0 {
		names := make([]string, 0, len(fields))
		for name := range fields {
			if t := strings.TrimSpace(name); t != "" {
				names = append(names, t)
			}
		}
		names = sortedStrings(names)
		for _, name := range names {
			cat.Subs = append(cat.Subs, types.FleetCategorySub{Name: name, Enabled: true})
		}
	}
	if err := h.db.WithContext(ctx).Create(&cat).Error; err != nil {
		logger.Warnf(ctx, "create fleet category from extraction failed: %v", err)
	}
}

func sortedStrings(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// buildFleetExtractionContent 拼接文档名、摘要与 chunks 文本（限长，避免超长输入）。
func buildFleetExtractionContent(name, summary string, chunks []*types.Chunk) string {
	var sb strings.Builder
	limit := 120000 // 约 6 万 token 上限，超出截断
	if name = strings.TrimSpace(name); name != "" {
		sb.WriteString("Document name: ")
		sb.WriteString(name)
		sb.WriteString("\n")
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		sb.WriteString("Existing summary: ")
		sb.WriteString(summary)
		sb.WriteString("\n")
	}
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ChunkType != types.ChunkTypeText &&
			chunk.ChunkType != types.ChunkTypeImageOCR &&
			chunk.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		text := strings.TrimSpace(chunk.Content)
		if text == "" {
			continue
		}
		if sb.Len() >= limit {
			break
		}
		sb.WriteString(text)
		sb.WriteString("\n")
	}
	return sb.String()
}

var _ = gorm.ErrRecordNotFound
