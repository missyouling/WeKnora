package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// BackfillEnergyBillCategories 幂等回填能耗账单文件的「所属分类」。
//
// 为什么必须回填：`FleetUploadHistoryDrawer` 的分类列取值顺序是
// `custom_metadata.invoice_category → fleet_cert_type`。而能耗账单文件上传时
// 前端只写了 `bill_kind`（electricity/solar），提取落库只写 `kind`
// （utility_bill/solar_bill）——两者都不是分类中文名，于是历史记录抽屉里
// 27 份文件的「账单类型」列全是空白，按分类筛选也筛不出任何数据（真机实测）。
//
// 口径：
//
//	kind=utility_bill → 市电账单
//	kind=solar_bill   → 光伏账单
//	无 kind（解析就失败、从未提取成功）→ 按文件名兜底：含「光伏」判光伏，其余判市电
//
// 只补 `fleet_cert_type` 为空的记录，绝不覆盖已有值（用户/其它流程写过的优先）。
// 作用范围限定在名字含「电费 / 能耗」的知识库，避免全表扫 custom_metadata。
func (h *UtilityHandler) BackfillEnergyBillCategories(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var kbIDs []string
	if err := h.db.WithContext(ctx).Model(&types.KnowledgeBase{}).
		Where("deleted_at IS NULL AND (name LIKE ? OR name LIKE ?)", "%电费%", "%能耗%").
		Pluck("id", &kbIDs).Error; err != nil {
		return 0, err
	}
	if len(kbIDs) == 0 {
		return 0, nil
	}
	var rows []types.Knowledge
	if err := h.db.WithContext(ctx).
		Select("id", "file_name", "custom_metadata").
		Where("knowledge_base_id IN ? AND deleted_at IS NULL", kbIDs).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	patched := 0
	for i := range rows {
		if len(rows[i].CustomMetadata) == 0 {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(rows[i].CustomMetadata, &meta); err != nil {
			continue
		}
		// 兼容历史双层结构：{"custom_metadata": {...}} 取内层
		if inner, ok := meta["custom_metadata"].(map[string]any); ok && len(meta) == 1 {
			meta = inner
		}
		if strings.TrimSpace(stringOfKey(meta, "fleet_cert_type")) != "" {
			continue
		}
		want := EnergyBillTypeOf(stringOfKey(meta, "kind"))
		if want == "" {
			want = energyBillTypeOfFileName(rows[i].FileName)
		}
		if want == "" {
			continue
		}
		meta["fleet_cert_type"] = want
		buf, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ?", rows[i].ID).
			Update("custom_metadata", types.JSON(buf)).Error; err != nil {
			logger.Warnf(ctx, "backfill energy bill category for %s failed: %v", rows[i].ID, err)
			continue
		}
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "energy bill categories backfilled: %d files", patched)
	}
	return patched, nil
}

// energyBillTypeOfFileName 从未提取成功（没有 kind）的文件名推断分类。
//
// 这是必要的兜底：解析就失败的文件永远不会有 kind，而它恰恰是最需要在
// 「历史记录 → 异常文件」里被认出属于哪一类账单的文件。
func energyBillTypeOfFileName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	if strings.Contains(n, "光伏") {
		return EnergyBillTypeSolar
	}
	if strings.Contains(n, "电费") || strings.Contains(n, "电表") {
		return EnergyBillTypeUtility
	}
	return ""
}

// stringOfKey 取 map 里的字符串值（非字符串一律当空）。
func stringOfKey(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
