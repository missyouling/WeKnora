package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
)

// InvoiceAlignmentSummaryHandler 发票知识库「源文件 ↔ 列表」对齐巡检（只读）。
//
// 用途：一次请求给出当前发票知识库里所有"文件还在但列表里看不见"的孤儿
// （提取失败 / 未提取出票据 / 删空待清理），供运维与自愈脚本决策：
// 有票据历史的文件重新提取归位，纯空壳残骸清理。
//
// 数据安全：只读，绝不自动删除任何文件。每项孤儿都带 knowledge_id 与原因，
// 处置动作（重新提取 / 删除）必须由用户在列表页显式触发。
func (h *BusinessExtractHandler) InvoiceAlignmentSummaryHandler(c *gin.Context) {
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

	summary, err := h.businessSvc.SummarizeInvoiceAlignment(effCtx, kbID)
	if err != nil {
		logger.Error(ctx, "Failed to summarize invoice alignment", err)
		c.Error(apperrors.NewInternalServerError("summarize invoice alignment failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"kb_id":             summary.KBID,
			"total_files":       summary.TotalFiles,
			"invoice_files":     summary.InvoiceFiles,
			"placeholder_rows":  summary.PlaceholderRows,
			"pending_deletes":   summary.PendingDeletes,
			"not_invoice_files": summary.NotInvoiceFiles,
			"in_progress_files": summary.InProgressFiles,
			// 需要处置的孤儿 knowledge_id 清单（by design：此处不含处置动作）
			"orphan_knowledge": summary.OrphanKnowledge,
			"hint":             "对孤儿执行重新提取或删除：在发票管理列表页使用「重新提取」/「删除」，或删除后重新上传。本接口只读，不会自动删除任何文件。",
		},
	})
}

// InvoicePendingDeleteSweepHandler 巡检超时未完成的「删空待清理」记录（只读）。
//
// 删空级联删除此刻是"先落 deleting 标记 + 入队异步清理"。若 asynq 任务丢失/
// 耗尽重试，记录会长期停在 deleting（既不显示发票、又不再被哈希锁挡住——
// 因为 CheckKnowledgeExists 已排除 deleting）。本接口把它们找出来，返回给运维
// 决定补删。
//
// maxAge 以小时为单位（query: max_age_hours，默认 24，上限 720）。
// 本接口不做任何删除，保证数据安全。
func (h *BusinessExtractHandler) InvoicePendingDeleteSweepHandler(c *gin.Context) {
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
		c.Error(apperrors.NewForbiddenError("No permission to sweep pending deletes"))
		return
	}
	effCtx := context.WithValue(ctx, types.TenantIDContextKey, effectiveTenantID)

	maxAgeHours := 24
	if v := c.Query("max_age_hours"); v != "" {
		if parsed, perr := strconv.Atoi(v); perr == nil && parsed > 0 && parsed <= 720 {
			maxAgeHours = parsed
		}
	}
	staleIDs, err := h.businessSvc.SweepStalePendingFileDeletes(effCtx, kbID,
		time.Duration(maxAgeHours)*time.Hour)
	if err != nil {
		logger.Error(ctx, "Failed to sweep stale pending invoice deletes", err)
		c.Error(apperrors.NewInternalServerError("sweep pending deletes failed: " + err.Error()))
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"kb_id":         kbID,
			"max_age_hours": maxAgeHours,
			"stale_ids":     staleIDs,
			"count":         len(staleIDs),
			"hint":          "这些是删空后异步清理未完成的源文件，可对其执行彻底删除或重新上传。本接口只读，不会自动删除。",
		},
	})
}
