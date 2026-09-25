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


func (h *BusinessExtractHandler) ExtractInvoice(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to extract invoice fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for invoice extraction", err)
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
	var curInvoiceMeta invoiceCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curInvoiceMeta)
	if curInvoiceMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Invoice extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "invoice", "extract_status": "manual", "removed": false},
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
		logger.Error(ctx, "Failed to list chunks for invoice extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	// 分批提取：超长多发票文档（18/22 张）单次 LLM 调用不可靠（智谱 500 /
	// 输出截断），按 chunk 分批独立调用后合并，Normalize 时按发票号去重。
	batches := service.BuildInvoiceExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不自动删除，保留文件并标记
	// failed 让用户人工处理。
	if !invoiceBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(invoiceCustomMetadata{
			Kind:          "invoice",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist invoice no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Invoice extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "invoice", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.InvoiceExtractionResult{Kind: "invoice"}
	var extractErr error
	sawInvoice := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractInvoicesFromContent(effCtx, chatModel, batch)
		if berr != nil {
			// 首批失败且无任何结果 → 整体失败（记录 failed 状态可重试）；
			// 后续批失败仅告警跳过，保留已提取的部分。
			if i == 0 && len(merged.Invoices) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Invoice extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_invoice" {
			continue
		}
		sawInvoice = true
		merged.Invoices = append(merged.Invoices, batchRes.Invoices...)
	}
	if !sawInvoice {
		merged.Kind = "not_invoice"
	}
	if extractErr != nil {
		// Persist a failed state so the UI shows "提取失败" (retriable via the
		// floating toolbar) instead of looping forever as "提取中/待提取".
		if failMeta, jerr := json.Marshal(invoiceCustomMetadata{
			Kind:          "invoice",
			ExtractStatus: "failed",
			ExtractError:  "invoice extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist invoice extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Invoice extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("invoice extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	service.NormalizeInvoiceExtractionResult(extracted)

	meta := service.InvoiceCustomMetadata{
		Kind:          extracted.Kind,
		Invoices:      extracted.Invoices,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：类型归类（用户可配置关键词/正则 → 发票类型），
	// 按模型提取出的发票类型字段匹配，命中则覆盖内置关键字判定。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		fullText := strings.Join(batches, "\n")
		for i := range meta.Invoices {
			if t := service.ClassifyTypeFromRules(meta.Invoices[i].InvoiceType, recCfg); t != "" {
				meta.Invoices[i].InvoiceType = t
			} else if meta.Invoices[i].InvoiceType == "" {
				// 模型未识别出类型时，用全文兜底归类（避免"其它票据"误归类）
				if t := service.ClassifyTypeFromRules(fullText, recCfg); t != "" {
					meta.Invoices[i].InvoiceType = t
				}
			}
		}
	}
	if extracted.Kind == "not_invoice" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为发票，置 manual 待补录，
		// 不自动删除（避免"该是发票却没入库"）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "invoice"
				meta.Invoices = []service.InvoiceExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为发票，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued invoice meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Invoice rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为发票，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "invoice", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_invoice"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like an invoice"
		}
		// 防循环：auto_deleted_count >= 1（曾被自动删除后人工恢复）→ 不再自动删除，
		// 改为 failed 保留。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非发票文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Invoice re-extraction judged non-invoice but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 需求：非发票文件不能存在于发票知识库 → 提取判定后自动删除该文件
			// （保留物理文件写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）。
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_invoice"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-invoice knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-invoice knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非发票文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_invoice"},
				})
				return
			}
		}
	}
	// Extraction-result guard: a model reply marked as invoice but carrying no
	// usable invoice (empty list, or every invoice blank) is a garbage/truncated
	// reply — persist it as failed (not success) so the UI shows a retriable
	// state instead of an empty "successful" row.
	if meta.Kind == "invoice" {
		if len(meta.Invoices) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何发票，请重试"
			}
		} else if service.InvoicesAllEmpty(meta.Invoices) {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "提取结果字段为空，请重试"
		}
	}

	// Cross-file dedup: mark invoices whose number already exists in another
	// document of the same KB, so the UI can hide them by default.
	if len(meta.Invoices) > 0 {
		h.businessSvc.MarkInvoiceDuplicates(effCtx, kbID, knowledgeID, &meta)
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist invoice extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	// 自动打标签功能已弃用（需求决策：不再自动创建/关联大类标签，
	// 保留手动标签）。category 字段仍随提取结果保存，仅不再自动转标签。

	logger.Infof(ctx, "Invoice extraction succeeded, knowledge ID: %s, kind: %s, invoices: %d",
		knowledgeID, meta.Kind, len(meta.Invoices))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Invoice extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Invoice extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"invoices":        meta.Invoices,
		},
	})
}

// ExtractInvoicePage godoc
// @Summary      按页重新提取发票
// @Description  对该知识文档中指定的第 N 张发票（发票按文档内出现顺序编号，即提取结果中的 page）单独重新提取，仅替换该张发票的数据，不影响同文件其它发票。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        body         body  object  true  "{\"page\":1}"
// @Success      200  {object}  map[string]interface{}  "更新后的发票"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-invoice-page [post]


func (h *BusinessExtractHandler) ExtractInvoicePage(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to extract invoice fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		var req struct {
			Page int `json:"page"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
			logger.Warn(ctx, "extract-invoice-page: failed to decode body", err)
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
		logger.Error(ctx, "Failed to get knowledge for invoice page extraction", err)
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
	var meta service.InvoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("no invoice extraction data, please run single-file extraction first"))
		return
	}
	if page > len(meta.Invoices) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Invoices))))
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
		logger.Error(ctx, "Failed to list chunks for invoice page extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	content := service.BuildInvoiceExtractionContent(knowledge.FileName, knowledge.Description, chunks)
	res, err := service.ExtractInvoicePageFromContent(effCtx, chatModel, content, page)
	if err != nil {
		logger.Error(ctx, "Invoice page extraction model call failed", err)
		c.Error(errors.NewInternalServerError("invoice page extraction failed: " + err.Error()))
		return
	}
	service.NormalizeInvoiceExtractionResult(res)
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("unable to re-extract this invoice, please try single-file extraction"))
		return
	}

	upd := res.Invoices[0]
	upd.Page = page // 保持原页码
	meta.Invoices[page-1] = upd
	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode invoice page extraction result", jerr)
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist invoice page extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invoice page extraction succeeded",
		"data":    upd,
	})
}

// DeleteInvoicePage godoc
// @Summary      删除指定发票记录
// @Description  按发票页码从 custom_metadata.invoices 中移除该张发票；若该文档仅剩这一张发票，则整份文档一并删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        request      body  object       true  "page：发票页码（文档内出现顺序，从 1 开始）"
// @Success      200  {object}  map[string]interface{}  "deleted_file: 是否整份删除"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Failure      403  {object}  errors.AppError         "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/delete-invoice-page [post]


func (h *BusinessExtractHandler) DeleteInvoicePage(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to delete invoice records"))
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
			logger.Warn(ctx, "delete-invoice-page: failed to decode body", err)
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
		logger.Error(ctx, "Failed to get knowledge for invoice page delete", err)
		c.Error(errors.NewNotFoundError("Knowledge not found"))
		return
	}
	if knowledge.KnowledgeBaseID != kbID {
		c.Error(errors.NewBadRequestError("Knowledge does not belong to the given knowledge base"))
		return
	}
	var meta service.InvoiceCustomMetadata
	if err := json.Unmarshal(knowledge.CustomMetadata, &meta); err != nil || meta.Kind != "invoice" || len(meta.Invoices) == 0 {
		c.Error(errors.NewBadRequestError("no invoice extraction data to delete"))
		return
	}
	if page > len(meta.Invoices) {
		c.Error(errors.NewBadRequestError(fmt.Sprintf("page %d out of range (1-%d)", page, len(meta.Invoices))))
		return
	}

	// 移除该页发票
	targetIdx := -1
	for i, inv := range meta.Invoices {
		if i == page-1 || inv.Page == page {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		targetIdx = page - 1
	}
	meta.Invoices = append(meta.Invoices[:targetIdx], meta.Invoices[targetIdx+1:]...)
	// 后续发票页码前移，保持页码=文档内出现顺序
	for i := range meta.Invoices {
		meta.Invoices[i].Page = i + 1
	}

	// 删空：整份文档一并删除
	if len(meta.Invoices) == 0 {
		if _, err := h.enqueueKnowledgeListDelete(effCtx, effectiveTenantID, kbID, []string{knowledgeID}); err != nil {
			logger.Error(ctx, "Failed to enqueue knowledge delete after removing last invoice", err)
			c.Error(errors.NewInternalServerError("failed to schedule delete: " + err.Error()))
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":     true,
			"message":     "Invoice removed, source file scheduled for deletion",
			"deleted_file": true,
		})
		return
	}

	metaBytes, jerr := json.Marshal(meta)
	if jerr != nil {
		logger.Error(ctx, "Failed to encode invoice metadata after delete", jerr)
		c.Error(errors.NewInternalServerError("failed to encode metadata"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaBytes)); err != nil {
		logger.Error(ctx, "Failed to persist invoice metadata after delete", err)
		c.Error(errors.NewInternalServerError("failed to save metadata: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     "Invoice record deleted",
		"deleted_file": false,
		"data":        meta.Invoices,
	})
}

// ExtractContractPage godoc
// @Summary      按页重新提取合同
// @Description  对该知识文档中指定的第 N 份合同（合同按文档内出现顺序编号，即提取结果中的 page）单独重新提取，仅替换该份合同的数据，不影响同文件其它合同。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id           path  string  true  "知识库ID"
// @Param        knowledgeId  path  string  true  "知识ID"
// @Param        body         body  object  true  "{\"page\":1}"
// @Success      200  {object}  map[string]interface{}  "更新后的合同"
// @Failure      400  {object}  errors.AppError         "请求参数错误"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-contract-page [post]

type invoiceCustomMetadata struct {
	Kind          string                          `json:"kind"`
	Invoices      []service.InvoiceExtractionItem `json:"invoices"`
	ExtractStatus string                          `json:"extract_status"`
	ExtractError  string                          `json:"extract_error"`
}

func invoiceBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}


func (h *BusinessExtractHandler) ExtractRegulation(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to extract regulation fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for regulation extraction", err)
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
	var curRegMeta regulationCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curRegMeta)
	if curRegMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Regulation extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
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
		logger.Error(ctx, "Failed to list chunks for regulation extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	batches := service.BuildRegulationExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史）。
	if !regulationBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(regulationCustomMetadata{
			Kind:          "regulation",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist regulation no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Regulation extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "regulation", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.RegulationExtractionResult{Kind: "regulation"}
	var extractErr error
	sawRegulation := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractRegulationsFromContent(effCtx, chatModel, batch)
		if berr != nil {
			if i == 0 && len(merged.Regulations) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Regulation extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_regulation" {
			continue
		}
		sawRegulation = true
		merged.Regulations = append(merged.Regulations, batchRes.Regulations...)
	}
	if !sawRegulation {
		merged.Kind = "not_regulation"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(regulationCustomMetadata{
			Kind:          "regulation",
			ExtractStatus: "failed",
			ExtractError:  "regulation extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist regulation extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Regulation extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("regulation extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	// 自动编号去重：无制度编号的制度按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoRegulationSeq(effCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto regulation seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeRegulationExtractionResult(extracted, autoSeq)

	meta := regulationCustomMetadata{
		Kind:          extracted.Kind,
		Regulations:   extracted.Regulations,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：制度类型归类（用户可配置关键词/正则 → 制度类型），
	// 按模型提取出的制度类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		for i := range meta.Regulations {
			if t := service.ClassifyTypeFromRules(meta.Regulations[i].RegType, recCfg); t != "" {
				meta.Regulations[i].RegType = t
			}
		}
	}
	if extracted.Kind == "not_regulation" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为制度，置 manual 待补录，
		// 不自动删除（解决"该是制度却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "regulation"
				meta.Regulations = []types.RegulationExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为制度，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued regulation meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Regulation rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为制度，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "regulation", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_regulation"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a regulation"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非制度不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非制度文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Regulation re-extraction judged non-regulation but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非制度文件不能存在于制度知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_regulation"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-regulation knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-regulation knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非制度文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_regulation"},
				})
				return
			}
		}
	}
	// 空提取守卫：标记为制度但没有可用制度（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "regulation" {
		if len(meta.Regulations) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何制度，请重试"
			}
		} else if service.RegulationsAllEmpty(meta.Regulations) {
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
		logger.Error(ctx, "Failed to persist regulation extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Regulation extraction succeeded, knowledge ID: %s, kind: %s, regulations: %d",
		knowledgeID, meta.Kind, len(meta.Regulations))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Regulation extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Regulation extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"regulations":     meta.Regulations,
		},
	})
}

// ExtractAwardPunish godoc
// @Summary      提取奖惩字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取奖惩字段（按当事人拆分多条记录）并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
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
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-award-punish [post]


func (h *BusinessExtractHandler) ExtractAwardPunish(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to extract award/punish fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for award/punish extraction", err)
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
	var curAPMeta awardPunishCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curAPMeta)
	if curAPMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Award/punish extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "manual", "removed": false},
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
		logger.Error(ctx, "Failed to list chunks for award/punish extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	batches := service.BuildAwardPunishExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层（扫描件 PDF / 纯图片）时不要自动删除，保留文件并标记
	// failed 让用户人工处理（配合删除历史）。
	if !awardPunishBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(awardPunishCustomMetadata{
			Kind:          "award_punish",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist award/punish no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Award/punish extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "failed", "removed": false},
		})
		return
	}
	merged := &service.AwardPunishExtractionResult{Kind: "award_punish"}
	// 措施库约束：提取时 measure 仅从配置的措施列表中选取（用户自定义 + 默认兜底）
	extractMeasures := make([]string, 0, 16)
	measureSeen := map[string]bool{}
	if measureCfg, merr := h.kgService.GetRecognitionConfig(effCtx, kbID); merr == nil {
		for _, m := range measureCfg.Measures {
			if n := strings.TrimSpace(m.Name); n != "" && !measureSeen[n] {
				measureSeen[n] = true
				extractMeasures = append(extractMeasures, n)
			}
		}
	}
	if len(extractMeasures) == 0 {
		for _, m := range types.DefaultAwardPunishRecognitionConfig().Measures {
			if n := strings.TrimSpace(m.Name); n != "" && !measureSeen[n] {
				measureSeen[n] = true
				extractMeasures = append(extractMeasures, n)
			}
		}
	}
	var extractErr error
	sawRecord := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractAwardPunishesFromContent(effCtx, chatModel, batch, extractMeasures)
		if berr != nil {
			if i == 0 && len(merged.Records) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Award/punish extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_award_punish" {
			continue
		}
		sawRecord = true
		merged.Records = append(merged.Records, batchRes.Records...)
	}
	if !sawRecord {
		merged.Kind = "not_award_punish"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(awardPunishCustomMetadata{
			Kind:          "award_punish",
			ExtractStatus: "failed",
			ExtractError:  "award/punish extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist award/punish extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Award/punish extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("award/punish extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	// 自动编号去重：无文号的奖惩按当天已有最大序号 +1 顺序递增（跨文件唯一）
	autoSeq, seqErr := h.businessSvc.MaxAutoAwardPunishSeq(effCtx, kbID)
	if seqErr != nil {
		logger.Warnf(ctx, "Failed to compute auto award/punish seq for %s: %v", kbID, seqErr)
	}
	service.NormalizeAwardPunishExtractionResult(extracted, autoSeq)
	// 措施二次清洗：仅保留措施库命中的选项，避免 LLM 输出库外自由文本
	for i := range extracted.Records {
		extracted.Records[i].Measure = service.FilterMeasuresByLibrary(extracted.Records[i].Measure, extractMeasures)
	}

	meta := awardPunishCustomMetadata{
		Kind:          extracted.Kind,
		Records:       extracted.Records,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}
	// 自定义识别规则：奖惩类型归类（用户可配置关键词/正则 → 奖惩类型），
	// 按模型提取出的奖惩类型字段匹配，命中则覆盖模型结果。
	if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
		for i := range meta.Records {
			if t := service.ClassifyTypeFromRules(meta.Records[i].ApType, recCfg); t != "" {
				meta.Records[i].ApType = t
			}
		}
	}
	if extracted.Kind == "not_award_punish" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为奖惩，置 manual 待补录，
		// 不自动删除（解决"该是奖惩却没入库"的误判）。
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "award_punish"
				meta.Records = []types.AwardPunishExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为奖惩，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued award/punish meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Award/punish rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为奖惩，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "award_punish", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_award_punish"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like an award/punish notice"
		}
		// 防循环：auto_deleted_count >= 1 说明该文件曾被自动删除后由用户从删除历史
		// 恢复，再次判定非奖惩不再自动删除，改为 failed 保留（避免 删除→恢复→删除 死循环）。
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非奖惩文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Award/punish re-extraction judged non-award/punish but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 非奖惩文件不能存在于奖惩知识库 → 提取判定后自动删除该文件（保留物理文件，
			// 写入删除历史，可在"删除历史"抽屉中查看/恢复/永久删除）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_award_punish"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-award/punish knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-award/punish knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非奖惩文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_award_punish"},
				})
				return
			}
		}
	}
	// 空提取守卫：标记为奖惩但没有可用记录（空数组或全部空字段）→ failed 可重试
	if meta.Kind == "award_punish" {
		if len(meta.Records) == 0 {
			meta.ExtractStatus = "failed"
			if meta.ExtractError == "" {
				meta.ExtractError = "提取未返回任何奖惩记录，请重试"
			}
		} else if service.AwardPunishRecordsAllEmpty(meta.Records) {
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
		logger.Error(ctx, "Failed to persist award/punish extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Award/punish extraction succeeded, knowledge ID: %s, kind: %s, records: %d",
		knowledgeID, meta.Kind, len(meta.Records))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Award/punish extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "Award/punish extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":    knowledgeID,
			"kind":            meta.Kind,
			"extract_status":  meta.ExtractStatus,
			"records":         meta.Records,
		},
	})
}

// GetRecognitionConfig godoc
// @Summary      获取识别规则配置
// @Description  返回知识库级文档识别规则（包含判定规则 + 类型归类规则），发票/合同管理页的设置面板读取。
// @Tags         知识库
// @Accept       json
// @Produce      json
// @Param        id  path  string  true  "知识库ID"
// @Success      200  {object}  types.RecognitionConfig  "识别规则配置"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/recognition-config [get]
func (h *KnowledgeBaseHandler) GetRecognitionConfig(c *gin.Context) {
	ctx := c.Request.Context()
	_, kbID, effectiveTenantID, _, err := h.validateAndGetKnowledgeBase(c)
	if err != nil {
		c.Error(err)
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	cfg, err := h.knowledgeService.GetRecognitionConfig(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to get recognition config", err)
		c.Error(errors.NewInternalServerError("get recognition config failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// ListDeletedKnowledge godoc
// @Summary      删除历史列表
// @Description  返回知识库中所有被系统自动删除（判定非合同/非发票）且保留源文件的记录，供"删除历史"抽屉查看、恢复、永久删除。
// @Tags         知识管理
// @Accept       json
// @Produce      json
// @Param        id        path  string  true  "知识库ID"
// @Param        page      query int     false "页码"
// @Param        page_size query int     false "每页数量"
// @Param        q         query string  false "按文件名/标题搜索"
// @Success      200  {object}  types.DeletedKnowledgePage  "删除历史列表"
// @Failure      403  {object}  errors.AppError             "无权限"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /knowledge-bases/{id}/deleted-knowledge [get]
