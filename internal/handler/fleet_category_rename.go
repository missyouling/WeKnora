package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 发票/业务分类改名传播（回归 2026-10-20）。
//
// 事故：把「水电发票」改名为「能耗发票」后，原分类下 49 份存量票据全部掉进
// 「其它票据」，新分类显示 0 张。根因是分类与数据之间是**分类名字符串**
// 关联（custom_metadata.invoice_category / fleet_cert_type /
// invoices[].invoice_type / invoices[].category，加 knowledge_extract_configs
// 的 cert_type 键），而 UpdateFleetCategory 只更新 fleet_categories.name
// 一列，存量数据里的旧名没有任何代码去换，于是：
//   - 列表读数仍带旧名 → 概览卡按新名精确聚合取不到 → 计数归「其它票据」
//     （前端排除式桶：不在任何启用分类中的记录）；
//   - 提取配置按旧名建键 → 新分类现场合成一份泛用底稿，用户规则被静默丢弃。
//
// 本文件提供改名传播三件套（与 countFleetCategoryRefs 的匹配口径保持同源，
// 保证「能数到的」就一定「能改到」）：
//  1. renameInvoiceCategoryInMeta   —— 精确整值替换 metadata 里的 4 处键；
//  2. propagateCategoryRename      —— 按 scope 分发，覆盖各业务分类数据；
//  3. extractConfigTypeRename     —— knowledge_extract_configs.cert_type 改键，
//     目标键已占用时返回 ErrRenameCertTypeConflict 让调用方回滚并 409。

// ErrRenameCertTypeConflict 表示目标知识库下已存在同名（新名）提取配置：
// 此时改名会产生两份 cert_type 不同的配置，读取侧按 updated_at DESC 取第一份，
// 结果不可预期。调用方必须整体回滚并向用户明示冲突，不得静默覆盖。
var ErrRenameCertTypeConflict = errors.New("target extract config already exists")

// 发票文件级权威分类键 + 票内分类键：改名时必须整体改写，任一残留都会让
// 读数链在不同入口得出不同分类（ListInvoiceRecords 与 countFleetCategoryRefs
// 口径必须一致，否则「删不掉也找不到」）。
var invoiceCategoryMetaKeys = []string{"invoice_category", "fleet_cert_type"}

// renameInvoiceCategoryInMeta 把 raw 中的分类名从 oldName 精确改写为 newName。
//
// 返回改写后的 JSON 与是否发生改动。语义要点：
//   - 只整值相等才替换（strings.TrimSpace 后比较），绝不做子串替换 ——
//     否则把「水」改名为「水源」会把「水费发票」误改成「水源费发票」；
//   - 只改上述 4 个键，其它键（invoices[].invoice_no、金额、fields…）原样保留；
//   - raw 为空/非法 JSON 时原样返回、isChanged=false（调用方跳过该行）。
func renameInvoiceCategoryInMeta(raw types.JSON, oldName, newName string) (types.JSON, bool) {
	if len(raw) == 0 {
		return raw, false
	}
	if oldName = strings.TrimSpace(oldName); oldName == "" {
		return raw, false
	}
	if newName = strings.TrimSpace(newName); newName == "" || newName == oldName {
		return raw, false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return raw, false
	}
	changed := false
	for _, k := range invoiceCategoryMetaKeys {
		v, ok := m[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			continue
		}
		if strings.TrimSpace(s) != oldName {
			continue
		}
		nv, err := json.Marshal(newName)
		if err != nil {
			continue
		}
		m[k] = nv
		changed = true
	}
	if inv, ok := m["invoices"]; ok {
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(inv, &items); err != nil {
			items = nil
		}
		for _, it := range items {
			for _, k := range []string{"invoice_type", "category"} {
				v, exists := it[k]
				if !exists {
					continue
				}
				var s string
				if err := json.Unmarshal(v, &s); err != nil {
					continue
				}
				if strings.TrimSpace(s) != oldName {
					continue
				}
				nv, err := json.Marshal(newName)
				if err != nil {
					continue
				}
				it[k] = nv
				changed = true
			}
		}
		if changed {
			invNew, err := json.Marshal(items)
			if err != nil {
				return raw, false
			}
			m["invoices"] = invNew
		}
	}
	if !changed {
		return raw, false
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw, false
	}
	return types.JSON(out), true
}

// propagateCategoryRename 把分类改名传播到该 scope 下所有存量数据。
// 与 countFleetCategoryRefs 同口径：invoice 走 knowledges.custom_metadata，
// contract/regulation/award_punish 走 custom_metadata.category，
// 车队 scope 走 fleet_records.doc_type。
func (h *FleetHandler) propagateCategoryRename(ctx context.Context, tx *gorm.DB, scope, oldName, newName string) error {
	if strings.TrimSpace(oldName) == "" || strings.TrimSpace(newName) == "" {
		return nil
	}
	switch scope {
	case "invoice":
		// 先用 LIKE 粗筛缩小候选集，再在 Go 侧逐行精确改写（与 countFleetCategoryRefs 同策略）。
		var rows []types.Knowledge
		if err := tx.WithContext(ctx).
			Where("deleted_at IS NULL AND CAST(custom_metadata AS TEXT) LIKE ?", "%"+oldName+"%").
			Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			updated, ok := renameInvoiceCategoryInMeta(rows[i].CustomMetadata, oldName, newName)
			if !ok {
				continue
			}
			if err := tx.WithContext(ctx).Model(&types.Knowledge{}).
				Where("id = ?", rows[i].ID).
				Update("custom_metadata", updated).Error; err != nil {
				return fmt.Errorf("rename knowledge %s metadata: %w", rows[i].ID, err)
			}
		}
	case "contract", "regulation", "award_punish":
		var rows []types.Knowledge
		if err := tx.WithContext(ctx).
			Where("deleted_at IS NULL AND CAST(custom_metadata AS TEXT) LIKE ?", "%"+oldName+"%").
			Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(rows[i].CustomMetadata, &m); err != nil || m == nil {
				continue
			}
			v, ok := m["category"]
			if !ok {
				continue
			}
			var s string
			if err := json.Unmarshal(v, &s); err != nil || strings.TrimSpace(s) != oldName {
				continue
			}
			nv, err := json.Marshal(newName)
			if err != nil {
				continue
			}
			m["category"] = nv
			out, err := json.Marshal(m)
			if err != nil {
				continue
			}
			if err := tx.WithContext(ctx).Model(&types.Knowledge{}).
				Where("id = ?", rows[i].ID).
				Update("custom_metadata", types.JSON(out)).Error; err != nil {
				return fmt.Errorf("rename knowledge %s category: %w", rows[i].ID, err)
			}
		}
	default:
		// 车队档案：doc_type 关联分类名（vehicle/driver/maintain 等）。
		if err := tx.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("deleted_at IS NULL AND doc_type = ?", oldName).
			Update("doc_type", newName).Error; err != nil {
			return fmt.Errorf("rename fleet records doc_type: %w", err)
		}
	}
	// 提取配置按 kb × scope × cert_type 建键，改名必须同步，
	// 否则用户配好的字段/规则被孤立，新分类退回泛用底稿。
	if err := h.extractConfigTypeRename(ctx, tx, scope, oldName, newName); err != nil {
		return err
	}
	return nil
}

// extractConfigTypeRename 把指定 scope 下 cert_type=oldName 的提取配置改键为 newName。
// 目标 (kb, scope, newName) 已有配置时返回 ErrRenameCertTypeConflict（调用方回滚）。
func (h *FleetHandler) extractConfigTypeRename(ctx context.Context, tx *gorm.DB, scope, oldName, newName string) error {
	if scope == "" || strings.TrimSpace(oldName) == "" || strings.TrimSpace(newName) == "" || oldName == newName {
		return nil
	}
	var cfgs []types.KbExtractConfig
	if err := tx.WithContext(ctx).
		Where("scope = ? AND cert_type = ? AND deleted_at IS NULL", scope, oldName).
		Find(&cfgs).Error; err != nil {
		return err
	}
	for _, cfg := range cfgs {
		var clash int64
		if err := tx.WithContext(ctx).Model(&types.KbExtractConfig{}).
			Where("knowledge_base_id = ? AND scope = ? AND cert_type = ? AND enabled = TRUE AND deleted_at IS NULL",
				cfg.KnowledgeBaseID, scope, newName).
			Count(&clash).Error; err != nil {
			return err
		}
		if clash > 0 {
			logger.Warnf(ctx, "rename extract config conflict: kb=%s scope=%s old=%s new=%s", cfg.KnowledgeBaseID, scope, oldName, newName)
			return ErrRenameCertTypeConflict
		}
	}
	if len(cfgs) == 0 {
		return nil
	}
	if err := tx.WithContext(ctx).Model(&types.KbExtractConfig{}).
		Where("scope = ? AND cert_type = ? AND deleted_at IS NULL", scope, oldName).
		Update("cert_type", newName).Error; err != nil {
		return fmt.Errorf("rename extract config cert_type: %w", err)
	}
	return nil
}
