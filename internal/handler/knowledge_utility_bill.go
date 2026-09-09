package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// utilityBillCustomMetadata mirrors the custom_metadata shape persisted on a
// knowledge row in the utility-bill KB.
type utilityBillCustomMetadata struct {
	Kind             string                           `json:"kind"`
	Records          []types.UtilityBillExtractionItem `json:"records"`
	ExtractStatus    string                           `json:"extract_status"`
	ExtractError     string                           `json:"extract_error"`
	AutoDeletedCount int                              `json:"auto_deleted_count"`
}

// ExtractUtilityBill godoc
// @Summary      提取电费账单字段
// @Description  读取已解析文档文本，调用提取模型（复用知识库 summary_model_id）提取电费账单四组字段并写入 custom_metadata。幂等：对同一知识重复调用会覆盖写。
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
// @Router       /knowledge-bases/{id}/knowledge/{knowledgeId}/extract-utility-bill [post]
func (h *KnowledgeHandler) ExtractUtilityBill(c *gin.Context) {
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
		c.Error(errors.NewForbiddenError("No permission to extract utility bill fields"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	knowledge, err := h.kgService.GetKnowledgeByIDOnly(effCtx, knowledgeID)
	if err != nil {
		logger.Error(ctx, "Failed to get knowledge for utility bill extraction", err)
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
	var curMeta utilityBillCustomMetadata
	_ = json.Unmarshal(knowledge.CustomMetadata, &curMeta)
	if curMeta.ExtractStatus == "manual" {
		logger.Infof(ctx, "Utility bill extraction skipped: knowledge %s is manual intake", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "待补录记录，无需重复提取",
			"data":    map[string]interface{}{"kind": "utility_bill", "extract_status": "manual", "removed": false},
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
		logger.Error(ctx, "Failed to list chunks for utility bill extraction", err)
		c.Error(errors.NewInternalServerError("list chunks failed: " + err.Error()))
		return
	}
	batches := service.BuildUtilityBillExtractionBatches(knowledge.FileName, knowledge.Description, chunks, 0)
	if !service.UtilityBillBatchesHaveText(batches) {
		noTextMeta, jerr := json.Marshal(utilityBillCustomMetadata{
			Kind:          "utility_bill",
			ExtractStatus: "failed",
			ExtractError:  "文档无可提取文本（疑似扫描件），已保留文件待人工处理",
		})
		if jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(noTextMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist utility bill no-text state: %v", serr)
			}
		}
		logger.Warnf(ctx, "Utility bill extraction skipped: no text layer for knowledge %s (scanned document), file kept", knowledgeID)
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "document has no extractable text (possible scanned document), file kept",
			"data":    map[string]interface{}{"kind": "utility_bill", "extract_status": "failed", "removed": false},
		})
		return
	}

	merged := &service.UtilityBillExtractionResult{Kind: "utility_bill"}
	var extractErr error
	sawBill := false
	for i, batch := range batches {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		batchRes, berr := service.ExtractUtilityBillsFromContent(effCtx, chatModel, batch)
		if berr != nil {
			if i == 0 && len(merged.Records) == 0 {
				extractErr = berr
				break
			}
			logger.Warnf(ctx, "Utility bill extraction batch %d failed (non-fatal): %v", i+1, berr)
			continue
		}
		if batchRes == nil || batchRes.Kind == "not_utility_bill" {
			continue
		}
		sawBill = true
		if len(batchRes.Records) == 0 {
			continue
		}
		if len(merged.Records) == 0 {
			merged.Records = append(merged.Records, batchRes.Records...)
			continue
		}
		// 一份账单一条记录：多批次结果合并进第一条（概况取首个非空，费用明细按序拼接）
		mergeUtilityBillRecords(&merged.Records[0], &batchRes.Records[0])
	}
	if !sawBill {
		merged.Kind = "not_utility_bill"
	}
	if extractErr != nil {
		if failMeta, jerr := json.Marshal(utilityBillCustomMetadata{
			Kind:          "utility_bill",
			ExtractStatus: "failed",
			ExtractError:  "utility bill extraction failed: " + extractErr.Error(),
		}); jerr == nil {
			if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(failMeta)); serr != nil {
				logger.Warnf(ctx, "Failed to persist utility bill extraction failed-state: %v", serr)
			}
		}
		logger.Error(ctx, "Utility bill extraction model call failed", extractErr)
		c.Error(errors.NewInternalServerError("utility bill extraction failed: " + extractErr.Error()))
		return
	}
	extracted := merged
	service.NormalizeUtilityBillExtractionResult(extracted)

	// 电费金额重算：提取只填基础要素（计费电量/计费标准），
	// 子项电费按 电量×标准 计算（提取的 rate 为准，可手动修正）。
	if h.db != nil {
		for i := range extracted.Records {
			service.RecalculateUtilityBillFees(&extracted.Records[i])
		}
	}

	meta := utilityBillCustomMetadata{
		Kind:          extracted.Kind,
		Records:       extracted.Records,
		ExtractStatus: "success",
		ExtractError:  extracted.ExtractError,
	}

	if extracted.Kind == "not_utility_bill" {
		// 自定义识别规则捞回：模型判非但包含规则命中 → 认定为电费账单，置 manual 待补录
		if recCfg, rerr := h.kgService.GetRecognitionConfig(effCtx, kbID); rerr == nil {
			if service.MatchIncludeRules(strings.Join(batches, "\n"), recCfg) {
				meta.Kind = "utility_bill"
				meta.Records = []types.UtilityBillExtractionItem{}
				meta.ExtractStatus = "manual"
				meta.ExtractError = "识别规则命中，已认定为电费账单，请编辑补录字段"
				if raw, merr := json.Marshal(meta); merr == nil {
					if serr := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(raw)); serr != nil {
						logger.Warnf(ctx, "Failed to persist rule-rescued utility bill meta: %v", serr)
					}
				}
				logger.Infof(ctx, "Utility bill rule-rescued by include rule, knowledge %s kept as manual", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success": true,
					"message": "识别规则命中，已认定为电费账单，请在列表中编辑补录字段",
					"data":    map[string]interface{}{"kind": "utility_bill", "extract_status": "manual", "removed": false},
				})
				return
			}
		}
		meta.ExtractStatus = "not_utility_bill"
		if meta.ExtractError == "" {
			meta.ExtractError = "document does not look like a utility bill"
		}
		// 防循环：曾被自动删除后人工恢复 → 不再自动删除，改为 failed 保留
		if h.autoDeleteCount(effCtx, knowledge) > 0 {
			meta.ExtractStatus = "failed"
			meta.ExtractError = "系统判定非电费账单文件，但该文件已被人工恢复，已保留待人工处理"
			logger.Warnf(ctx, "Utility bill re-extraction judged non-bill but row was restored before; keeping file %s", knowledgeID)
		} else {
			// 需求：非账单文件不能存在于电费知识库 → 自动删除（保留物理文件写入删除历史，可恢复）
			if delErr := h.kgService.AutoDeleteKnowledge(effCtx, knowledgeID, "not_utility_bill"); delErr != nil {
				logger.Warnf(ctx, "auto-delete non-utility-bill knowledge failed: %v", delErr)
			} else {
				logger.Infof(ctx, "auto-deleted non-utility-bill knowledge, ID: %s", knowledgeID)
				c.JSON(http.StatusOK, gin.H{
					"success":  true,
					"message":  "非电费账单文件已移至删除历史，可在删除历史中恢复",
					"data":     map[string]interface{}{"removed": true, "kind": "not_utility_bill"},
				})
				return
			}
		}
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		c.Error(errors.NewInternalServerError("failed to encode extraction result"))
		return
	}
	if err := h.kgService.SaveInvoiceCustomMetadata(effCtx, knowledgeID, types.JSON(metaJSON)); err != nil {
		logger.Error(ctx, "Failed to persist utility bill extraction result", err)
		c.Error(errors.NewInternalServerError("failed to save extraction result: " + err.Error()))
		return
	}

	logger.Infof(ctx, "Utility bill extraction succeeded, knowledge ID: %s, kind: %s, records: %d",
		knowledgeID, meta.Kind, len(meta.Records))
	if meta.ExtractStatus == "failed" {
		logger.Warnf(ctx, "Utility bill extraction returned an unusable result, knowledge ID: %s, err: %s", knowledgeID, meta.ExtractError)
		c.Error(errors.NewInternalServerError(meta.ExtractError))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Utility bill extraction succeeded",
		"data": map[string]interface{}{
			"knowledge_id":   knowledgeID,
			"kind":           meta.Kind,
			"extract_status": meta.ExtractStatus,
			"records":        meta.Records,
			"removed":        false,
		},
	})
}

// mergeUtilityBillRecords merges a batch-extraction record into the target so
// one bill always produces exactly one record: overview fields take the first
// non-empty value, fee_items are appended in order, remark concatenated.
func mergeUtilityBillRecords(target, src *types.UtilityBillExtractionItem) {
	if target == nil || src == nil {
		return
	}
	// 字符串字段：目标为空则取源值
	setStr := func(dst *string, v string) {
		if *dst == "" && strings.TrimSpace(v) != "" {
			*dst = v
		}
	}
	setStr(&target.BillPeriodStart, src.BillPeriodStart)
	setStr(&target.BillPeriodEnd, src.BillPeriodEnd)
	setStr(&target.AccountNo, src.AccountNo)
	setStr(&target.AccountName, src.AccountName)
	setStr(&target.Address, src.Address)
	setStr(&target.UsageCategory, src.UsageCategory)
	setStr(&target.VoltageLevel, src.VoltageLevel)
	setStr(&target.SupplyUnit, src.SupplyUnit)
	setStr(&target.MarketAttr, src.MarketAttr)
	setStr(&target.PrintDate, src.PrintDate)
	setStr(&target.MomChange, src.MomChange)
	setStr(&target.DueDate, src.DueDate)
	setStr(&target.Remark, src.Remark)
	if target.Remark != "" && src.Remark != "" && src.Remark != target.Remark {
		target.Remark = target.Remark + "\n" + src.Remark
	}
	// 数值字段：目标为 0 时取源值
	setNum := func(dst *float64, v float64) {
		if *dst == 0 && v != 0 {
			*dst = v
		}
	}
	setNum(&target.TotalKwh, src.TotalKwh)
	setNum(&target.TotalAmount, src.TotalAmount)
	setNum(&target.PrevKwh, src.PrevKwh)
	setNum(&target.AvgPrice, src.AvgPrice)
	setNum(&target.PowerFactor, src.PowerFactor)
	setNum(&target.IndustrialAmount, src.IndustrialAmount)
	setNum(&target.ResidentialAmount, src.ResidentialAmount)
	setNum(&target.PfAdjustAmount, src.PfAdjustAmount)
	setNum(&target.GrandTotal, src.GrandTotal)
	setNum(&target.DeepPeakKwh, src.DeepPeakKwh)
	setNum(&target.PeakKwh, src.PeakKwh)
	setNum(&target.FlatKwh, src.FlatKwh)
	setNum(&target.ValleyKwh, src.ValleyKwh)
	setNum(&target.ReactiveKwh, src.ReactiveKwh)
	setNum(&target.Capacity, src.Capacity)
	setNum(&target.CapacityPrice, src.CapacityPrice)
	setNum(&target.CapacityFee, src.CapacityFee)
	setNum(&target.Demand, src.Demand)
	setNum(&target.PfStandard, src.PfStandard)
	setNum(&target.AdjustRatio, src.AdjustRatio)
	// 费用明细按序拼接
	target.FeeItems = append(target.FeeItems, src.FeeItems...)
}

// ListUtilityBillRecords lives on KnowledgeBaseHandler in knowledgebase.go
// (it is a KB-scoped read route, same as ListInvoiceRecords).
