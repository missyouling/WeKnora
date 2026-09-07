package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// GetDeletedKnowledgeByID returns a single soft-deleted (auto-deleted) row by
// ID, read unscoped. Used by the delete-history preview to check ownership.
func (s *knowledgeService) GetDeletedKnowledgeByID(ctx context.Context, id string) (*types.Knowledge, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	return s.repo.GetDeletedKnowledgeByID(ctx, tenantID, id)
}

// localizeDeleteReason maps the stored English auto-delete reason to Chinese
// for the 删除历史 UI. Unknown reasons pass through unchanged.
func localizeDeleteReason(reason string) string {
	switch reason {
	case "not_contract":
		return "非合同"
	case "not_invoice":
		return "非发票"
	case "not_regulation":
		return "非制度"
	default:
		return reason
	}
}

// ListDeletedKnowledge returns the auto-deleted knowledge rows (soft-deleted
// with custom_metadata.auto_deleted=true) inside a knowledge base, newest first.
// These rows are the "删除历史" of the invoice / contract modules: files that
// were auto-removed because extraction judged them not an invoice / not a
// contract. The physical file is retained, so the UI can preview and restore.
func (s *knowledgeService) ListDeletedKnowledge(
	ctx context.Context,
	kbID string,
	page, pageSize int,
	keyword string,
) (*types.DeletedKnowledgePage, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	if page <= 0 {
		page = 1
	}
	items, total, err := s.repo.ListDeletedKnowledge(ctx, tenantID, kbID, page, pageSize, keyword)
	if err != nil {
		return nil, err
	}
	data := make([]types.DeletedKnowledgeItem, 0, len(items))
	for _, k := range items {
		if k == nil {
			continue
		}
		item := types.DeletedKnowledgeItem{
			ID:              k.ID,
			KnowledgeBaseID: k.KnowledgeBaseID,
			Title:           k.Title,
			FileName:        k.FileName,
			FileType:        k.FileType,
			FileSize:        k.FileSize,
			FileHash:        k.FileHash,
			FilePath:        k.FilePath,
			CreatedAt:       k.CreatedAt.Format(time.RFC3339),
		}
		if k.DeletedAt.Valid {
			item.DeletedAt = k.DeletedAt.Time.Format(time.RFC3339)
		}
		var meta map[string]any
		_ = json.Unmarshal(k.CustomMetadata, &meta)
		if meta != nil {
			if r, ok := meta["auto_deleted_reason"].(string); ok {
				item.Reason = localizeDeleteReason(r)
			}
			if c, ok := meta["auto_deleted_count"].(float64); ok {
				item.DeleteCount = int(c)
			}
		}
		data = append(data, item)
	}
	return &types.DeletedKnowledgePage{
		Data:     data,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
		Success:  true,
	}, nil
}

// RestoreDeletedKnowledge brings an auto-deleted row back into the knowledge
// base as a manually-intaken record. It does NOT re-run parsing/extraction:
// the row is marked kind=invoice|contract with extract_status="manual" so the
// module list shows a placeholder row immediately, and the user completes the
// fields through the existing detail drawer (auto-save persists them).
//
// The previously extracted fields (invoices/contracts arrays) are preserved as
// pre-fill hints when available.
func (s *knowledgeService) RestoreDeletedKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	knowledge, err := s.repo.GetDeletedKnowledgeByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	// Clear the auto-delete history markers (keep the count), clear the
	// soft-delete tombstone and put the row back into a live state.
	if err := s.repo.RestoreDeletedKnowledgeRow(ctx, tenantID, id); err != nil {
		return nil, err
	}

	// Drop the auto-delete markers, keep the count for the anti-loop guard, and
	// flip the kind to the module type with a manual (待编辑) extract state.
	var meta map[string]any
	_ = json.Unmarshal(knowledge.CustomMetadata, &meta)
	if meta == nil {
		meta = map[string]any{}
	}
	delete(meta, "auto_deleted")
	reason, _ := meta["auto_deleted_reason"].(string)
	delete(meta, "auto_deleted_reason")
	delete(meta, "auto_deleted_at")
	kind, _ := meta["kind"].(string)
	// 非合同/非发票文件在自动删除时未持久化提取结果（handler 提前返回），
	// meta 中只有 auto_deleted_* 标记 —— 恢复时按删除原因推断模块类型，
	// 置 extract_status=manual，让列表立即显示占位行供用户补录字段。
	if kind == "" {
		switch reason {
		case "not_invoice":
			kind = "invoice"
		case "not_contract":
			kind = "contract"
		case "not_regulation":
			kind = "regulation"
		}
	}
	switch kind {
	case "invoice", "not_invoice":
		meta["kind"] = "invoice"
		meta["extract_status"] = "manual"
		meta["extract_error"] = "人工入库待编辑，请补充字段"
		if _, ok := meta["invoices"]; !ok {
			meta["invoices"] = []any{}
		}
	case "contract", "not_contract":
		meta["kind"] = "contract"
		meta["extract_status"] = "manual"
		meta["extract_error"] = "人工入库待编辑，请补充字段"
		if _, ok := meta["contracts"]; !ok {
			meta["contracts"] = []any{}
		}
	case "regulation", "not_regulation":
		meta["kind"] = "regulation"
		meta["extract_status"] = "manual"
		meta["extract_error"] = "人工入库待编辑，请补充字段"
		if _, ok := meta["regulations"]; !ok {
			meta["regulations"] = []any{}
		}
	default:
		// Unknown kind: restore as-is, only clear auto-delete markers.
	}
	if raw, merr := json.Marshal(meta); merr == nil {
		if uerr := s.repo.UpdateKnowledgeColumn(ctx, id, "custom_metadata", types.JSON(raw)); uerr != nil {
			logger.Warnf(ctx, "RestoreDeletedKnowledge failed to clear auto-delete markers for %s: %v", id, uerr)
		}
	}
	// 重新入库不重解析：文件内容此前已解析完成，恢复后直接标记完成，
	// 避免文档列表长期显示"解析中"，字段由用户在编辑抽屉中人工补录。
	if uerr := s.repo.UpdateKnowledgeColumn(ctx, id, "parse_status", types.ParseStatusCompleted); uerr != nil {
		logger.Warnf(ctx, "RestoreDeletedKnowledge failed to mark parse completed for %s: %v", id, uerr)
	}

	logger.Infof(ctx, "Restored auto-deleted knowledge %s as manual intake (file=%s, kind=%s)", id, knowledge.FileName, kind)
	return knowledge, nil
}

// PurgeDeletedKnowledge permanently removes an auto-deleted row: hard-deletes
// the DB row and drops the retained physical file. There is no undo.
func (s *knowledgeService) PurgeDeletedKnowledge(ctx context.Context, id string) error {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	knowledge, err := s.repo.GetDeletedKnowledgeByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if err := s.repo.HardDeleteKnowledge(ctx, tenantID, id); err != nil {
		return err
	}
	// Drop the retained physical file (best-effort).
	if knowledge.FilePath != "" {
		kb, kerr := s.kbService.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
		if kerr == nil {
			kbFileSvc := s.resolveFileService(ctx, kb)
			if ferr := kbFileSvc.DeleteFile(ctx, knowledge.FilePath); ferr != nil {
				logger.GetLogger(ctx).WithField("error", ferr).Errorf("PurgeDeletedKnowledge delete file failed")
			}
		}
	}
	logger.Infof(ctx, "Purged auto-deleted knowledge %s permanently (file=%s)", id, knowledge.FileName)
	return nil
}
