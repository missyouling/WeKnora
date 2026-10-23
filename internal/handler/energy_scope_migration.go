package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗设置中台化迁移（P2）。
//
// 背景——为什么不能"只迁 DB 里那 56 条"：
//
//	`utility_field_configs` 实测只有 7 个分组（56 条），但界面上有 12 张卡片。
//	差额来自 `utilityDefaultFieldConfigs()` 的**内存兜底**：line / trans / sys /
//	gov-industrial / catalog / gov-residential / pf 这 7 个分组在库里一条记录都没有，
//	列表能正常显示全靠读取时现场生成。
//
//	若迁移时只读 DB，这 7 个分组会在切到中台后变成 0 字段——界面上卡片还在
//	（前端 GROUP_DEFS 硬编码），点进去却空白，用户会认为"迁移把配置搞丢了"。
//
// 正确算法：
//
//	基底 = utilityDefaultFieldConfigs() 的全部 21 组（含行分组）
//	覆盖 = DB 中用户已自定义的 56 条（按 field_key 逐字段覆盖 label/type/visible/order）
//	结果 = 完整 subs（带 group 标记）写入 fleet_categories
//
// 幂等性：以 (scope, name) 为锚点 upsert；重复执行结果一致；
// 只在 subs 为空或显式 force 时重建，避免覆盖用户迁移后的后续修改。

// migratedGroupLabels 12 个费用分项分组的展示名。
//
// 键必须与旧 `GROUP_DEFS`（UtilitySettingsDrawer.vue:214-227）和
// `electricityDefaultGroupFields` 的 group 值逐字一致——三处漂移会让
// 迁移出来的 subs 在界面上分组错位。
var migratedGroupLabels = map[string]string{
	"overview":          "账单概况",
	"market":            "市场化购电费",
	"line":              "上网环节线损费",
	"trans":             "输配电量电费",
	"sys":               "系统运行费",
	"gov-industrial":    "政府基金及附加（工商业）",
	"catalog":           "目录电费（居民）",
	"gov-residential":   "政府基金及附加（居民）",
	"capacity":          "输配容（需）量",
	"pf":                "功率因素调整",
	"meter":             "电量明细（工商业）",
	"resident-meter":    "电量明细（居民）",
	"meter-rows":        "电量明细（工商业）· 行",
	"resident-meter-rows": "电量明细（居民）· 行",
	"catalog-rows":      "目录电费（居民）· 行",
	"overview-rows":     "账单概况 · 行",
	"market-rows":       "市场化购电费 · 行",
	"trans-rows":        "输配电量电费 · 行",
	"sys-rows":          "系统运行费 · 行",
	"gov-industrial-rows": "政府基金及附加（工商业）· 行",
	"gov-residential-rows": "政府基金及附加（居民）· 行",
}

// energyMigratedGroups 迁移进 subs 的分组顺序（决定界面胶囊条顺序）。
//
// 与旧 GROUP_DEFS 的 12 张卡片顺序一致，行分组紧随其主分组之后
// （行分组不单独出卡片，是主分组详情页的「行字段」）。
var energyMigratedGroups = []string{
	"overview",
	"market", "market-rows",
	"line",
	"trans", "trans-rows",
	"sys", "sys-rows",
	"gov-industrial", "gov-industrial-rows",
	"catalog", "catalog-rows",
	"gov-residential", "gov-residential-rows",
	"capacity",
	"pf",
	"meter", "meter-rows",
	"resident-meter", "resident-meter-rows",
	"overview-rows",
}

// MigrateUtilityFieldConfigsToEnergyScope 把旧表字段配置迁移为 energy scope 的 subs。
//
// ⚠️ tenantID 必须传 **0**：energy 属于「业务配置 scope」，
// `ListFleetCategories` 对这类 scope 固定读 `tenant_id = 0` 的全局配置
// （见 fleet.go 的 isBusinessCategoryScope）。写成租户 ID 会让迁移结果
// "写进去了但界面永远读不到"——真机踩过一次。
//
// 返回 (写入的分类数, 字段总数, error)。幂等：`fleet_categories` 里已有同名分类
// 且 subs 非空时跳过（除非 force=true 显式重建）。
func (h *UtilityHandler) MigrateUtilityFieldConfigsToEnergyScope(
	ctx context.Context, _ uint64, force bool,
) (int, int, error) {
	if h.db == nil {
		return 0, 0, nil
	}
	const tenantID uint64 = 0 // 业务配置一律落全局
	subs := h.buildEnergySubs(ctx, 10000) // 旧表数据按实际租户读取
	if len(subs) == 0 {
		return 0, 0, nil
	}
	// 只写「市电账单」分类：光伏/能耗单没有旧表配置，由各自目录负责默认值
	cats := []struct {
		name string
		subs []types.FleetCategorySub
	}{
		{EnergyBillTypeUtility, subs},
		{EnergyBillTypeSolar, nil},
		{EnergyBillTypeLedger, nil},
	}
	written, fields := 0, 0
	for i, c := range cats {
		var exist types.FleetCategory
		err := h.db.WithContext(ctx).
			Where("tenant_id = ? AND scope = ? AND name = ? AND deleted_at IS NULL",
				tenantID, EnergyScope, c.name).
			First(&exist).Error
		exists := err == nil
		// 已有分类且 subs 非空：默认跳过（幂等保护，避免覆盖用户迁移后的修改）
		if exists && len(exist.Subs) > 0 && !force {
			continue
		}
		payload := c.subs
		if exists && len(payload) == 0 {
			// 光伏/能耗单没有旧配置：保留已有 subs（可能用户已在中台里配过）
			continue
		}
		if !exists {
			exist = types.FleetCategory{
				ID:        uuid.NewString(),
				TenantID:  int64(tenantID),
				Scope:     EnergyScope,
				Name:      c.name,
				SortOrder: i,
				Enabled:   true,
			}
		}
		exist.Subs = payload
		if exists {
			if err := h.db.WithContext(ctx).Model(&types.FleetCategory{}).
				Where("id = ?", exist.ID).
				Updates(map[string]any{"subs": payload, "updated_at": timeNowUTC()}).Error; err != nil {
				return written, fields, err
			}
		} else {
			if err := h.db.WithContext(ctx).Create(&exist).Error; err != nil {
				return written, fields, err
			}
		}
		written++
		fields += len(payload)
		logger.Infof(ctx, "energy migration: category %q written with %d subs", c.name, len(payload))
	}
	return written, fields, nil
}

// buildEnergySubs 组装完整的 subs：21 组默认基底 + DB 56 条覆盖。
//
// 这是 P2 的核心算法。**绝不能只读 DB**——理由见文件头注释。
func (h *UtilityHandler) buildEnergySubs(ctx context.Context, tenantID uint64) []types.FleetCategorySub {
	// 1) DB 已自定义字段，按 (group, field_key) 建索引
	dbRows := make([]types.UtilityFieldConfig, 0, 128)
	if err := h.db.WithContext(ctx).
		Where("tenant_id = ? AND category = ? AND deleted_at IS NULL", tenantID, "electricity").
		Order("sort_order ASC, created_at ASC").
		Find(&dbRows).Error; err != nil {
		logger.Warnf(ctx, "energy migration: read utility_field_configs failed: %v", err)
	}
	type key struct{ group, field string }
	override := make(map[key]types.UtilityFieldConfig, len(dbRows))
	for _, r := range dbRows {
		override[key{strings.TrimSpace(r.Group), strings.TrimSpace(r.FieldKey)}] = r
	}
	// 用户自建字段（DB 里有、默认组里没有）：必须追加进对应分组，不能丢
	customByGroup := make(map[string][]types.UtilityFieldConfig)
	defaultKeys := make(map[key]bool)

	out := make([]types.FleetCategorySub, 0, 160)
	for _, g := range energyMigratedGroups {
		defs := electricityDefaultGroupFields(g)
		for _, d := range defs {
			defaultKeys[key{g, d.key}] = true
			label := d.label
			typ := d.typ
			visible := d.def
			if o, ok := override[key{g, d.key}]; ok {
				// 用户改过的三项覆盖默认值（其余如排序由遍历顺序决定）
				if s := strings.TrimSpace(o.Label); s != "" {
					label = s
				}
				if s := strings.TrimSpace(o.FieldType); s != "" {
					typ = s
				}
				visible = o.DefaultVisible
			}
			out = append(out, types.FleetCategorySub{
				Name:      d.key,
				Label:     label,
				Enabled:   true,
				IsDefault: visible,
				DataType:  normalizeUtilityDataType(typ),
				// 原来的 group 值直接透传：它同时是「12 个费用分项」的归属键
				Group:      g,
				GroupLabel: migratedGroupLabels[g],
			})
		}
	}
	// 收集用户自建字段
	for _, r := range dbRows {
		g := strings.TrimSpace(r.Group)
		if g == "" {
			g = "overview" // 无分组的存量记录归入概况，避免成为孤儿字段
		}
		if defaultKeys[key{g, strings.TrimSpace(r.FieldKey)}] {
			continue
		}
		customByGroup[g] = append(customByGroup[g], r)
	}
	// 自建字段追加到所属分组末尾（保持组内顺序：默认字段在前）
	for _, g := range energyMigratedGroups {
		for _, r := range customByGroup[g] {
			out = append(out, types.FleetCategorySub{
				Name:      strings.TrimSpace(r.FieldKey),
				Label:     strings.TrimSpace(r.Label),
				Enabled:   true,
				IsDefault: r.DefaultVisible,
				DataType:  normalizeUtilityDataType(r.FieldType),
				Group:     g,
				GroupLabel: migratedGroupLabels[g],
			})
		}
		delete(customByGroup, g)
	}
	// 兜底：落在未预期分组里的自建字段也不能丢（宁可多一组，不可静默丢字段）
	for g, rows := range customByGroup {
		for _, r := range rows {
			out = append(out, types.FleetCategorySub{
				Name:      strings.TrimSpace(r.FieldKey),
				Label:     strings.TrimSpace(r.Label),
				Enabled:   true,
				IsDefault: r.DefaultVisible,
				DataType:  normalizeUtilityDataType(r.FieldType),
				Group:     g,
				GroupLabel: migratedGroupLabels[g],
			})
		}
	}
	return out
}

// normalizeUtilityDataType 旧表类型 → 中台 data_type。
//
// 旧表有 amount（金额），中台没有这个类型：金额本质是数值，归一到 number。
// 不归一会让前端 dataType 分支（列宽/折行/控件）匹配不到任何分支。
func normalizeUtilityDataType(t string) string {
	switch strings.TrimSpace(t) {
	case "amount", "number":
		return "number"
	case "date":
		return "date"
	case "textarea":
		return "textarea"
	case "array":
		return "array"
	default:
		return "text"
	}
}

// MigrateEnergyScope 迁移接口（POST /utilities/migrate-energy-scope）。
//
// 供运维/CI 在发布后触发一次；幂等，可安全重复调用。
// ?force=1 强制重建 subs（会覆盖用户在字段定义里对「市电账单」的后续改动，慎用）。
func (h *UtilityHandler) MigrateEnergyScope(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, _ := utilityTenantID(c)
	force := strings.TrimSpace(c.Query("force")) == "1" ||
		strings.EqualFold(strings.TrimSpace(c.Query("force")), "true")
	written, fields, err := h.MigrateUtilityFieldConfigsToEnergyScope(ctx, tenantID, force)
	if err != nil {
		logger.Errorf(ctx, "energy scope migration failed: %v", err)
		c.Error(errors.NewInternalServerError("migrate energy scope failed: " + err.Error()))
		return
	}
	// 提取规则底稿不在这里补：由设置抽屉首次打开时经
	// ensureBusinessExtractConfigDefaults 幂等落库（scope=energy 已在白名单里）。
	// 这样迁移接口保持"只碰分类与字段"的单一职责，出问题好定位。
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"categories_written": written,
			"fields_written":     fields,
			"forced":             force,
		},
	})
}
