package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// BusinessExtractHandler 承载 WeKnora 二次开发中「日常事务」模块的业务提取与记录管理。
type BusinessExtractHandler struct {
	*KnowledgeHandler
	db           *gorm.DB
	modelService interfaces.ModelService
	businessSvc  *service.BusinessExtractService
}

// NewBusinessExtractHandler 构造业务提取 Handler。
func NewBusinessExtractHandler(kh *KnowledgeHandler, db *gorm.DB, ms interfaces.ModelService, bsvc *service.BusinessExtractService) *BusinessExtractHandler {
	return &BusinessExtractHandler{KnowledgeHandler: kh, db: db, modelService: ms, businessSvc: bsvc}
}

// autoDeleteCount reads the auto_deleted_count marker from a knowledge row's
// custom_metadata (0 when absent).
func (h *BusinessExtractHandler) autoDeleteCount(ctx context.Context, knowledge *types.Knowledge) int {
	if knowledge == nil || len(knowledge.CustomMetadata) == 0 {
		return 0
	}
	var meta map[string]any
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil {
		return 0
	}
	if c, ok := meta["auto_deleted_count"].(float64); ok {
		return int(c)
	}
	return 0
}

func (h *BusinessExtractHandler) ExtractContract(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}

	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract contract fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract extraction", err)
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
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取、不参与任何自动删除判定。
	var curContractMeta contractCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curContractMeta)
	if curContractMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Contract extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
		})
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
		logger.Error(ctx, "Failed to list chunks for contract extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	// 分批提取：超长多合同文档单次 LLM 调用不可靠，按 chunk 分批独立调用后合并，
	// Normalize 时按文件内合同编号去重。
	batches := service.BuildContractExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史，避免"监测检测合同"这类扫描件被误删）。
	if !contractBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(contractCustomMetadata{
			Kind:          "contract",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist contract no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Contract extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "contract", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.ContractExtractionResult{Kind: "contract"}
	var extractErr error
	sawContract := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractContractsFromContent(effCtx, chatModel, batch)
		if berr != nil {
			if i == 0 && len(merged.Contracts) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Contract extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_contract" {
			continue
		}
		sawContract = true
		merged.Contracts = append(merged.Contracts, batchRes.Contracts...)
	}
	if !sawContract {
		merged.Kind = "not_contract"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(contractCustomMetadata{
			Kind:          "contract",
			ExtractStatus: "failed",
			ExtractError:  "contract extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist contract extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Contract extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("contract extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	// 自动编号去重：无合同编号的合同按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoContractSeq(effCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto contract seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeContractExtractionResult(extracted, autoSeq)

	meta := contractCustomMetadata{
		Kind:          extracted.Kind,
		Contracts:     extracted.Contracts,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：合同类型归类（用户可配置关键词/正则 → 合同类型），
	// 按模型提取出的合同类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		for i := range meta.Contracts {
			if t := service.ClassifyTypeFromRules(meta.Contracts[i].ContractType, recCfg); t != "" {
				meta.Contracts[i].ContractType = t
			}
		}
	}
	if extracted.Kind == "not_contract" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为合同，置 manual 待补录，
		// 不自动删除（解决"该是合同却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "contract"
				meta.Contracts = []service.ContractExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为合同，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued contract meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Contract rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为合同，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "contract", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_contract"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a contract"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非合同不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非合同文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Contract re-extraction judged non-contract but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非合同文件不能存在于合同知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_contract"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-contract knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-contract knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非合同文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_contract"},
				})
				return
			}
		}
	}
	// 空提取守卫：标记为合同但没有可用合同（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "contract" {
		if len(meta.Contracts) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何合同，请重试"
			}
		} else if service.ContractsAllEmpty(meta.Contracts) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist contract extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Contract extraction succeeded, knowledge ID: %s, kind: %s, contracts: %d",
		knowledgeID, meta.Kind, len(meta.Contracts))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Contract extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Contract extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"contracts":       meta.Contracts,
		},
	})
}

// regulationCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractRegulation. The frontend reads exactly
// these keys to render the regulation management list.
type regulationCustomMetadata struct {
	Kind          string                              `json:"kind"`
	Regulations   []types.RegulationExtractionItem    `json:"regulations"`
	ExtractStatus string                              `json:"extract_status"`
	ExtractError  string                              `json:"extract_error"`
}

// awardPunishCustomMetadata is the persisted shape written into a knowledge
// entry's custom_metadata by ExtractAwardPunish. The frontend reads exactly
// these keys to render the award/punish management list.
type awardPunishCustomMetadata struct {
	Kind          string                             `json:"kind"`
	Records       []types.AwardPunishExtractionItem  `json:"records"`
	ExtractStatus string                             `json:"extract_status"`
	ExtractError  string                             `json:"extract_error"`
}

// awardPunishBatchesHaveText reports whether any extraction batch carries usable
// text. A scanned document (no text layer) yields only empty batches; without
// this guard the extract endpoint would judge it not-award/punish and
// auto-delete a legitimate scanned notice.
func awardPunishBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// regulationBatchesHaveText reports whether any extraction batch carries usable
// text. A scanned document (no text layer) yields only empty batches; without
// this guard the extract endpoint would judge it not-a-regulation and
// auto-delete a legitimate scanned regulation.
func regulationBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// ExtractRegulation godoc
// @Summary      提取制度字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取制度字段并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Success      200  {object}  map[string]interface{}  "提取成功"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-regulation [post]


func (h *BusinessExtractHandler) ExtractContractPage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to extract contract fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "extract-contract-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract page extraction", err)
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
	var meta contractCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "contract" || len(meta.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("no contract extraction data, please run single-file extraction first"))
		return
	}
	if page > len(meta.Contracts) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Contracts))))
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
		logger.Error(ctx, "Failed to list chunks for contract page extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	content := service.BuildContractExtractionContent(knowledge.FileName, knowledge.Description, chunks)
	res, err := service.ExtractContractPageFromContent(effCtx, chatModel, content, page)
	if err != nil {
		logger.Error(ctx, "Contract page extraction model call failed", err)
		c.Error(errors.NewInternalServerError("contract page extraction failed: " + err.Error()))
		return
	}
	prevNo := meta.Contracts[page-1].ContractNo
	service.NormalizeContractExtractionResult(res, 0)
	if res == nil || res.Kind == "not_contract" || len(res.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("unable to re-extract this contract, please try single-file extraction"))
		return
	}

	upd := res.Contracts[0]
	upd.Page = page // 保持原页码
	// 保留原合同编号，避免单页重新提取时自动编号漂移产生新号
	if prevNo != "" {
		upd.ContractNo = prevNo
	}
	meta.Contracts[page-1] = upd
	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode contract page extraction result", jerr)
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist contract page extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Contract page extraction succeeded",
		"data":    upd,
	})
}

// DeleteContractPage godoc
// @Summary      删除指定合同记录
// @Description  按合同页码从 custom_metadata.contracts 中移除该份合同；若该文档仅剩这一份合同，则整份文档一并删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        request      body  object       true  "page：合同页码（文档内出现顺序，从 1 开始）"
// @Success      200  {object}  map[string]interface{}  "deleted_file: 是否整份删除"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/delete-contract-page [post]


func (h *BusinessExtractHandler) DeleteContractPage(c *gin.Context) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(errors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	kb, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	_ = kb
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(errors.NewForbiddenError("No permission to delete contract records"))
		return
	}
	if err := h.requireKBOwnershipOrAdmin(c, kbID); err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "delete-contract-page: failed to decode body", err)
		} else if req.Page >= 1 {
			page = req.Page
		}
	}
	if page < 1 {
		c.Error(errors.NewBadRequestError("page must be a positive integer"))
		return
	}

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for contract page delete", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	var meta contractCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "contract" || len(meta.Contracts) == 0 {
		c.Error(errors.NewBadRequestError("no contract extraction data to delete"))
		return
	}
	if page > len(meta.Contracts) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Contracts))))
		return
	}

	// 移除该份合同
	targetIdx := -1
	for i, ct := range meta.Contracts {
		if i == page-1 || ct.Page == page {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		targetIdx = page - 1
	}
	meta.Contracts = append(meta.Contracts[:targetIdx], meta.Contracts[targetIdx+1:]...)
	// 后续合同页码前移，保持页码=文档内出现顺序
	for i := range meta.Contracts {
		meta.Contracts[i].Page = i + 1
	}

	// 删空：整份文档一并删除
	if len(meta.Contracts) == 0 {
		if _, err := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, kbID, []string{knowledgeID}); err != nil {
			logger.Error(ctx, "Failed to enqueue knowledge delete after removing last contract", err)
			c.Error(errors.NewInternalServerError("failed to schedule delete: " + err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"message":     "Contract removed, source file scheduled for deletion",
			"deleted_file": true,
		})
		return
	}

	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode contract metadata after delete", jerr)
		c.Error(errors.NewInternalServerError("failed to encode metadata"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist contract metadata after delete", err)
		c.Error(errors.NewInternalServerError("failed to save metadata: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Contract record deleted",
		"deleted_file": false,
		"data":        meta.Contracts,
	})
}

// markInvoiceDuplicates scans the other documents of the same KB and flags
// invoices whose number already appears elsewhere. In-file duplicates are
// already collapsed by the model prompt; this pass only handles cross-file
// repetition.

// contractCustomMetadata mirrors the per-knowledge custom_metadata shape for contracts.
type contractCustomMetadata struct {
	Kind          string                          `json:"kind"`
	Contracts     []service.ContractExtractionItem `json:"contracts"`
	ExtractStatus string                          `json:"extract_status"`
	ExtractError  string                          `json:"extract_error"`
}

// contractBatchesHaveText reports whether any extraction batch carries usable text.
func contractBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}
