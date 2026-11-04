package handler

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 车队归档类（车辆档案/司机档案/维保记录）的提取配置懒建。
//
// 为什么字段清单不写死在 service 目录里：这三类的字段由用户在「设置 → 字段定义」
// 维护（fleet_categories.subs），硬编码一份会导致界面加字段后提取链路仍用老清单。
// 因此这里读 subs 作为权威源，规则/正则由 service 注册表按证照类型富化。
//
// 幂等语义与 ensureBusinessExtractConfigDefaults 一致：
//   - 已有配置：只回填空的 desc/rule/regex 与空 PromptTemplate，绝不覆盖用户改过的文案；
//   - 无配置：按 subs 现场合成一份并落库。
func (h *BusinessExtractHandler) ensureFleetArchiveExtractConfigDefaults(
	ctx context.Context, kbID string, tenantID uint64, scope, certType string,
) *types.KbExtractConfig {
	scope = strings.TrimSpace(scope)
	certType = strings.TrimSpace(certType)
	if certType == "" {
		return h.getExtractConfig(ctx, kbID, scope, certType)
	}

	subs := h.loadFleetCategorySubs(ctx, int64(tenantID), scope, certType)
	fields := service.FleetArchiveFieldsFromSubs(subs, certType)
	if len(fields) == 0 {
		return h.getExtractConfig(ctx, kbID, scope, certType)
	}
	prompt := service.FleetArchivePromptTemplate(certType)
	now := time.Now().UTC()

	if existing := h.getExtractConfig(ctx, kbID, scope, certType); existing != nil {
		// 已有配置：补齐空规则（不覆盖用户改过的文案），并按需补 Prompt
		byName := make(map[string]types.ExtractFieldConfig, len(fields))
		for _, f := range fields {
			byName[f.Name] = f
		}
		changed := false
		for i := range existing.Fields {
			def, ok := byName[existing.Fields[i].Name]
			if !ok {
				continue
			}
			if strings.TrimSpace(existing.Fields[i].Desc) == "" && def.Desc != "" {
				existing.Fields[i].Desc = def.Desc
				changed = true
			}
			if strings.TrimSpace(existing.Fields[i].Rule) == "" && def.Rule != "" {
				existing.Fields[i].Rule = def.Rule
				changed = true
			}
			if strings.TrimSpace(existing.Fields[i].Regex) == "" && def.Regex != "" {
				existing.Fields[i].Regex = def.Regex
				changed = true
			}
		}
		if strings.TrimSpace(existing.PromptTemplate) == "" && prompt != "" {
			existing.PromptTemplate = prompt
			changed = true
		}
		if changed {
			existing.UpdatedAt = now
			if err := h.db.WithContext(ctx).Save(existing).Error; err != nil {
				logger.Warnf(ctx, "backfill fleet archive extract config failed: %v", err)
			}
		}
		return existing
	}

	// 「整体规则」（cert_type 为空）存在时不再新建分类级配置，避免两套规则并存
	if all := h.getExtractConfig(ctx, kbID, scope, ""); all != nil {
		return all
	}

	cfg := &types.KbExtractConfig{
		ID:              uuid.NewString(),
		TenantID:        int64(tenantID),
		KnowledgeBaseID: kbID,
		Scope:           scope,
		CertType:        certType,
		Fields:          fields,
		PromptTemplate:  prompt,
		Version:         1,
		Enabled:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.db.WithContext(ctx).Create(cfg).Error; err != nil {
		logger.Warnf(ctx, "create fleet archive extract config failed: %v", err)
		return nil
	}
	logger.Infof(ctx, "seeded fleet archive extract config %s/%s (fields=%d)", scope, certType, len(fields))
	return cfg
}

// loadFleetCategorySubs 读某租户某 scope 下指定分类的字段定义（停用的也读，
// 由 FleetArchiveFieldsFromSubs 统一过滤，保持与「字段定义」面板一致）。
func (h *BusinessExtractHandler) loadFleetCategorySubs(
	ctx context.Context, tenantID int64, scope, certType string,
) []types.FleetCategorySub {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL", tenantID, scope, certType).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "load fleet category subs %s/%s failed: %v", scope, certType, err)
		return nil
	}
	if len(cats) == 0 {
		return nil
	}
	return cats[0].Subs
}
