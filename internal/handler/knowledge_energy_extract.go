package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// energyCustomMetadata 是能耗提取结果写入 custom_metadata 的形态。
// 前端列表/抽屉按这些键读取。
type energyCustomMetadata struct {
	Kind          string           `json:"kind"`
	Records       []map[string]any `json:"records"`
	ExtractStatus string           `json:"extract_status"`
	ExtractError  string           `json:"extract_error"`
	FleetCertType string           `json:"fleet_cert_type,omitempty"`
	// 通用质量闭环元数据（与发票/合同同口径）
	ExtractIssues  []service.ExtractFieldIssue `json:"extract_issues,omitempty"`
	PatchedByRegex []string                    `json:"patched_by_regex,omitempty"`
	RegexPatches   []service.RegexFieldPatch   `json:"regex_patches,omitempty"`
}

// resolveEnergyRuleCfg 能耗提取规则解析：按 scope=energy 读取用户配置的提取规则。
//
// 回退链与合同一致：分类规则 → 整体规则 → 默认底稿（落一份带 desc/rule/regex 的配置，
// 保证提取用的字段口径与设置页看到的一致）。
func (h *BusinessExtractHandler) resolveEnergyRuleCfg(ctx context.Context, kbID, certType string) *types.KbExtractConfig {
	scope := "energy"
	if ct := strings.TrimSpace(certType); ct != "" {
		if cfg := h.getExtractConfig(ctx, kbID, scope, ct); cfg != nil {
			return service.WithSnakeFieldKeys(cfg)
		}
	}
	if cfg := h.getExtractConfig(ctx, kbID, scope, ""); cfg != nil {
		return service.WithSnakeFieldKeys(cfg)
	}
	return service.WithSnakeFieldKeys(h.ensureBusinessExtractConfigDefaults(ctx, kbID, 0, scope, certType))
}

// ExtractEnergy 能耗台账提取入口：投递异步任务后立即返回（与合同/制度/通知同构）。
//
// 真正的提取逻辑在 runEnergyCore（asynq worker 调用），
// 这样模型调用耗时长不会打爆网关读超时。
func (h *BusinessExtractHandler) ExtractEnergy(c *gin.Context, certType string) {
	ctx := c.Request.Context()
	kbID := secutils.SanitizeForLog(c.Param("id"))
	knowledgeID := secutils.SanitizeForLog(c.Param("knowledgeId"))
	if kbID == "" || knowledgeID == "" {
		c.Error(apperrors.NewBadRequestError("knowledge base id and knowledge id cannot be empty"))
		return
	}
	_, _, effectiveTenantID, permission, err := h.validateKnowledgeBaseAccessWithKBID(c, kbID)
	if err != nil {
		c.Error(err)
		return
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		c.Error(apperrors.NewForbiddenError("No permission to extract energy fields"))
		return
	}
	// 快速失败：同一文档已有任务在跑时直接 409，避免连点重复投递。
	if _, loaded := h.extractMu.Load(knowledgeID); loaded {
		c.Error(apperrors.NewConflictError("extraction already in progress for this document").
			WithDetails(extractInProgressDetails(knowledgeID, "energy")))
		return
	}
	taskID, err := h.enqueueBusinessExtract(ctx, effectiveTenantID, kbID, knowledgeID, "energy", certType)
	if err != nil {
		logger.Errorf(ctx, "Failed to enqueue energy extraction for %s: %v", knowledgeID, err)
		c.Error(apperrors.NewInternalServerError("enqueue energy extraction failed: " + err.Error()))
		return
	}
	logger.Infof(ctx, "Energy extraction enqueued: knowledge=%s task=%s", knowledgeID, taskID)
	c.JSON(200, gin.H{
		"success": true,
		"message": "Energy extraction submitted",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           "energy",
			"extract_status": "processing",
			"task_id":        taskID,
		},
	})
}

// runEnergyCore 是不依赖 gin 的能耗提取核心执行器（HTTP 投递即返，asynq worker 调用）。
//
// 与合同核心的差异：落库前多跑一步**读数反算与用量守卫**
// （ReconcileEnergyRecords），把「实际用量 = (本期 − 上期) × 倍率」固化进后处理。
func (h *BusinessExtractHandler) runEnergyCore(
	ctx context.Context, kbID, knowledgeID, certType string, effectiveTenantID uint64,
) (gin.H, error) {
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)
	// persistCtx 脱离任务生命周期：任务超时/取消后仍能落库提取结果或 failed 状态
	persistCtx := context.WithValue(context.Background(), types.TenantIDContextKey, effectiveTenantID)
	// extractCtx 30 分钟硬上限：耗时的大模型调用不被任务取消连带掐断
	extractCtx, extractCancel := newExtractionContext(effectiveTenantID)
	defer extractCancel()

	var knowledge *types.Knowledge
	// panic 兜底：任何 panic 都强制落 failed 状态（用独立 ctx），保证状态闭环
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf(ctx, "Energy extraction panic recovered: %v\n%s", r, debug.Stack())
			var panicPrev []byte
			if knowledge != nil {
				panicPrev = knowledge.CustomMetadata
			}
			failMeta := mergeFailedStateMeta(panicPrev, "energy", fmt.Sprintf("energy extraction panic: %v", r))
			if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
				logger.Warnf(ctx, "Failed to persist panic failed-state: %v", serr)
			}
		}
	}()

	var err error
	knowledge, err = h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		return nil, apperrors.NewNotFoundError("Knowledge not found")
	}
	if knowledge.KnowledgeBaseID != kbID {
		return nil, apperrors.NewBadRequestError("Knowledge does not belong to the given knowledge base")
	}
	if businessExtractionInProgress(knowledge.ParseStatus) {
		return nil, apperrors.NewBadRequestError("document has not been parsed yet, please wait for parsing to finish")
	}
	// 待补录豁免：manual 记录由用户人工维护，不自动重提取
	var curMeta energyCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curMeta)
	if curMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Energy extraction skipped: knowledge %s is manual intake", knowledgeID)
		return gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "energy", "extract_status": "manual", "removed": false},
		}, nil
	}

	kb, kberr := h.kbService.GetKnowledgeBaseByIDOnly(effCtx, kbID)
	if kberr != nil || kb == nil {
		return nil, apperrors.NewNotFoundError("Knowledge base not found")
	}
	modelID := strings.TrimSpace(kb.SummaryModelID)
	if modelID == "" {
		return nil, apperrors.NewBadRequestError("no extraction model configured for the knowledge base")
	}
	chatModel, err := h.modelService.GetChatModel(extractCtx, modelID)
	if err != nil {
		return nil, apperrors.NewInternalServerError("get extraction model failed: " + err.Error())
	}

	chunks, err := h.chunkService.ListChunksByKnowledgeIDAndTypes(extractCtx, knowledgeID, service.ExtractSourceChunkTypes())
	if err != nil {
		return nil, apperrors.NewInternalServerError("list chunks failed: " + err.Error())
	}
	batches := service.BuildEnergyExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	// 扫描件防御：无文本层时保留文件并标 failed，不自动删除
	if !energyBatchesHaveText(batches) {
		zhNotice, enNotice := noExtractableTextNotice(knowledge.ParseStatus)
		noTextMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "energy", zhNotice)
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, noTextMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist energy no-text state: %v", serr)
		}
		logger.Warnf(ctx, "Energy extraction skipped: no text layer for knowledge %s (parse_status=%s), file kept",
			knowledgeID, knowledge.ParseStatus)
		return gin.H{
			"success": true,
			"message": enNotice,
			"data":    map[string]interface{}{"kind": "energy", "extract_status": "failed", "removed": false},
		}, nil
	}

	// 提取进行中标记（前端轮询/状态列实时显示「正在提取」）
	procMeta := map[string]interface{}{"kind": "energy", "extract_status": "processing"}
	if len(knowledge.CustomMetadata) > 0 {
		var prev map[string]interface{}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &prev); jerr == nil {
			for k, v := range prev {
				if _, exists := procMeta[k]; !exists {
					procMeta[k] = v
				}
			}
		}
	}
	delete(procMeta, "extract_error")
	if pmeta, jerr := json.Marshal(procMeta); jerr == nil {
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(pmeta)); serr != nil {
			logger.Warnf(ctx, "Failed to persist energy processing-state: %v", serr)
		}
	}

	// 规则解析：上传弹窗选择的分类 → 文件级打标 → 整体规则
	ruleCertType := certType
	if ruleCertType == "" && len(knowledge.CustomMetadata) > 0 {
		var mdMeta struct {
			FleetCertType string `json:"fleet_cert_type"`
		}
		if jerr := json.Unmarshal(knowledge.CustomMetadata, &mdMeta); jerr == nil && mdMeta.FleetCertType != "" {
			ruleCertType = mdMeta.FleetCertType
		}
	}
	energyRuleCfg := h.resolveEnergyRuleCfg(extractCtx, kbID, ruleCertType)

	merged := &service.EnergyExtractionResult{Kind: "energy"}
	var extractErr error
	sawEnergy := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractEnergyFromContentWithRules(extractCtx, chatModel, batch, energyRuleCfg)
		if berr != nil {
			if i == 0 && len(merged.Records) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Energy extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_energy" {
			continue
		}
		sawEnergy = true
		merged.Records = append(merged.Records, batchRes.Records...)
	}
	if !sawEnergy {
		merged.Kind = "not_energy"
	}
	if extractErr != nil {
		failMeta := mergeFailedStateMeta(knowledge.CustomMetadata, "energy", "energy extraction failed: "+extractErr.Error())
		if serr := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, failMeta); serr != nil {
			logger.Warnf(ctx, "Failed to persist energy extraction failed-state: %v", serr)
		}
		logger.Error(ctx, "Energy extraction model call failed", extractErr)
		return nil, apperrors.NewInternalServerError("energy extraction failed: " + extractErr.Error())
	}

	// —— 核心收口：读数反算与用量守卫 ——
	// 必须在落库前跑：反算补齐的实际用量与用量告警都要进最终落库形态。
	records, issueCount := service.ReconcileEnergyRecords(merged.Records)
	if issueCount > 0 {
		logger.Infof(ctx, "Energy extraction usage issues: %d knowledge=%s", issueCount, knowledgeID)
	}

	// 通用质量闭环：标准日期归一 + 用户配置必填字段的正则补洞
	energyIssues, energyPatches := service.ValidateGenericExtraction(
		service.NewGenericRecords(records), strings.Join(batches, "\n\n"), energyRuleCfg)
	if len(energyIssues) > 0 || len(energyPatches) > 0 {
		logger.Infof(ctx, "Energy extraction quality: issues=%d patched=%v knowledge=%s",
			len(energyIssues), service.PatchedFieldNames(energyPatches), knowledgeID)
	}

	meta := energyCustomMetadata{
		Kind:           merged.Kind,
		Records:        records,
		ExtractStatus:  "success",
		ExtractError:   merged.ExtractError,
		ExtractIssues:  energyIssues,
		PatchedByRegex: service.PatchedFieldNames(energyPatches),
		RegexPatches:   energyPatches,
	}
	// 用户归档分类绝对优先（对齐发票/合同）
	h.applyEnergyUserCategoryPrecedence(extractCtx, &meta, ruleCertType)

	if merged.Kind == "not_energy" {
		meta.ExtractStatus = "not_energy"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like an energy bill"
		}
		// 防循环：曾被自动删除后人工恢复的记录不再自动删除
		if h.autoDeleteCount(extractCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非能耗单，但该文件已被人工恢复，已保留待人工处理"
		} else {
			if delErr := h.kgService.AutoDeleteKnowledge(extractCtx, knowledgeID, "not_energy"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-energy knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-energy knowledge, ID: %s", knowledgeID)
				return gin.H{
					"success": true,
					"message": "非能耗单文件已移至删除历史，可在删除历史中恢复",
					"data":    map[string]interface{}{"removed": true, "kind": "not_energy"},
				}, nil
			}
		}
	}
	// 空提取守卫：标记为能耗单但没有可用记录 → failed 可重试
	if meta.Kind == "energy" && len(meta.Records) == 0 {
		meta.ExtractStatus = "failed"
		if meta.ExtractError == "" {
			meta.ExtractError = "提取未返回任何能耗记录，请重试"
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, apperrors.NewInternalServerError("failed to encode extraction result")
	}
	// 合并而非覆盖：保留提取流程不管理的字段（如 auto_deleted_count）
	mergedJSON := metaJSON
	if len(knowledge.CustomMetadata) > 0 {
		var existing map[string]interface{}
		var fresh map[string]interface{}
		if jerr1 := json.Unmarshal(knowledge.CustomMetadata, &existing); jerr1 == nil {
			if jerr2 := json.Unmarshal(metaJSON, &fresh); jerr2 == nil {
				for k, v := range fresh {
					existing[k] = v
				}
				if mj, merr := json.Marshal(existing); merr == nil {
					mergedJSON = mj
				}
			}
		}
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(persistCtx, knowledgeID, types.JSON(mergedJSON)); err != nil {
		logger.Error(ctx, "Failed to persist energy extraction result", err)
		return nil, apperrors.NewInternalServerError("failed to save extraction result: " + err.Error())
	}

	logger.Infof(ctx, "Energy extraction succeeded, knowledge ID: %s, kind: %s, records: %d, usage_issues: %d",
		knowledgeID, meta.Kind, len(meta.Records), issueCount)
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Energy extraction returned an unusable result, knowledge ID: %s, err: %s",
			knowledgeID, meta.ExtractError)
		return nil, apperrors.NewInternalServerError(meta.ExtractError)
	}
	return gin.H{
		"success": true,
		"message": "Energy extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"records":        meta.Records,
		},
	}, nil
}

// applyEnergyUserCategoryPrecedence 用户归档分类绝对优先 + 停用分类隔离。
//
// 与合同/制度同口径：上传时显式选择的分类优先于模型猜的类型；
// 未打标时模型类型必须落在当前启用的能耗分类集合内，否则降级「其它能耗」。
func (h *BusinessExtractHandler) applyEnergyUserCategoryPrecedence(ctx context.Context, meta *energyCustomMetadata, certType string) {
	if meta == nil {
		return
	}
	enabled := map[string]struct{}{}
	for _, n := range h.enabledEnergyCategoryNames(ctx) {
		enabled[n] = struct{}{}
	}
	if len(enabled) == 0 {
		return
	}
	for i := range meta.Records {
		rec := meta.Records[i]
		cur, _ := rec["utility_type"].(string)
		cur = strings.TrimSpace(cur)
		if ct := strings.TrimSpace(certType); ct != "" {
			rec["utility_type"] = ct
			continue
		}
		if cur == "" {
			continue
		}
		if _, ok := enabled[cur]; !ok {
			rec["utility_type"] = "其它能耗"
		}
	}
}

// energyBatchesHaveText 判断批次里是否有可用文本（扫描件无文本层时全空）。
func energyBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}
