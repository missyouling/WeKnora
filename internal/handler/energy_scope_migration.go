package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/Tencent/WeKnora/internal/application/service"
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
			exist.UpdatedAt = timeNowUTC()
			if err := h.db.WithContext(ctx).Save(&exist).Error; err != nil {
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
	// 「账单概况」重置为 20 个业务字段（幂等），并同步提取规则字段集
	overviewFields, _ := h.MigrateEnergyOverviewFields(ctx)
	ruleFields, _ := h.resetEnergyExtractConfigFields(ctx)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"categories_written": written,
			"fields_written":     fields,
			"overview_fields":    overviewFields,
			"rule_fields":        ruleFields,
			"forced":             force,
		},
	})
}

// energyLegacyOverviewKeys 上一版误并入「账单概况」的字段（需在重置时清除）。
//
// 来源两处：旧 overview 27 项（起止日/打印日/分时电量/容需量）与目录里的
// 7 个费用汇总列（market_amount 等）。它们都不是"一眼看清这张账单"的必需项，
// 留在主列表只会把列数推到 34 并制造重复的「账单周期」。
var energyLegacyOverviewKeys = map[string]bool{
	"bill_period_start": true, "bill_period_end": true, "print_date": true,
	"capacity": true, "capacity_price": true, "capacity_fee": true,
	"demand": true, "pf_standard": true, "adjust_ratio": true,
	"deep_peak_kwh": true, "peak_kwh": true, "flat_kwh": true,
	"valley_kwh": true, "reactive_kwh": true,
	"market_amount": true, "line_amount": true, "trans_amount": true,
	"sys_amount": true, "catalog_amount": true, "govI_amount": true, "govR_amount": true,
}

// MigrateEnergyOverviewFields 把「市电账单 -> 账单概况」重置为严格 20 个业务字段。
//
// 幂等：当前 overview 的键序列与目标一致时直接返回 0，不写库。
// 其它分组（12 个费用分项 + 细项）一律不动。
func (h *UtilityHandler) MigrateEnergyOverviewFields(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL",
			EnergyScope, EnergyBillTypeUtility).
		Order("sort_order ASC").First(&cat).Error; err != nil {
		return 0, nil
	}
	want := electricityDefaultGroupFields("overview")
	// 目标 overview 序列
	target := make([]types.FleetCategorySub, 0, len(want))
	for _, d := range want {
		target = append(target, types.FleetCategorySub{
			Name:       d.key,
			Label:      d.label,
			Enabled:    true,
			IsDefault:  d.def,
			DataType:   normalizeUtilityDataType(d.typ),
			Group:      "overview",
			GroupLabel: migratedGroupLabels["overview"],
		})
	}
	// 现有 overview 是否已达标（键序列 + 默认可见位都一致才跳过）
	//
	// 为什么要比 is_default：20 个概况字段是主列表口径，全部默认可见；
	// 早期版本只把 8 个标成默认，导致升级后列表只显示 8 列。只比键序列会漏掉这次修正。
	var current []string
	same := true
	idx := 0
	for _, s := range cat.Subs {
		if strings.TrimSpace(s.Group) != "overview" {
			continue
		}
		current = append(current, strings.TrimSpace(s.Name))
		if idx < len(target) {
			if strings.TrimSpace(s.Name) != target[idx].Name || s.IsDefault != target[idx].IsDefault {
				same = false
			}
		}
		idx++
	}
	if len(current) == len(target) && same {
		return 0, nil
	}
	// 重建：非 overview 分组原样保留，overview 用目标序列前置（保证列顺序）
	next := make([]types.FleetCategorySub, 0, len(cat.Subs)+len(target))
	next = append(next, target...)
	for _, s := range cat.Subs {
		if strings.TrimSpace(s.Group) == "overview" {
			continue
		}
		next = append(next, s)
	}
	// 必须用结构体 Save：`Updates(map[string]any{"subs": slice})` 会被 pgx 当成
	// record 表达式直接报 `column "subs" is of type jsonb but expression is of type record`。
	// （与 memory 里记的 jsonb 写入陷阱同源，禁 gorm.Expr / map 传切片。）
	cat.Subs = next
	cat.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Save(&cat).Error; err != nil {
		return 0, err
	}
	logger.Infof(ctx, "energy overview reset: %d -> %d fields (category=%s)", len(current), len(target), cat.Name)
	return len(target), nil
}

// resetEnergyExtractConfigFields 把「市电账单」的提取规则字段重置为目录默认 20 项，
// 保留用户自建的非标准字段（不在 energyLegacyOverviewKeys 里的）。
//
// 为什么必须一起重置：提取规则页的字段集合来自 subs 对齐，但 align 会把
// "配置里有、subs 里没有"的字段追加回来——旧的 bill_period_start/end 等
// 会因此复活，规则页又变成 34 行。
func (h *UtilityHandler) resetEnergyExtractConfigFields(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var cfg types.KbExtractConfig
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND cert_type = ? AND deleted_at IS NULL", EnergyScope, EnergyBillTypeUtility).
		Order("updated_at DESC").First(&cfg).Error; err != nil {
		return 0, nil
	}
	defaults := service.DefaultExtractFields(EnergyScope, EnergyBillTypeUtility)
	byName := make(map[string]types.ExtractFieldConfig, len(defaults))
	for _, d := range defaults {
		byName[d.Name] = d
	}
	next := make([]types.ExtractFieldConfig, 0, len(defaults)+4)
	for _, d := range defaults {
		// 已有配置里的 desc/rule/regex 若用户改过则沿用
		if old, ok := findFieldByName(cfg.Fields, d.Name); ok {
			if strings.TrimSpace(old.Desc) != "" {
				d.Desc = old.Desc
			}
			if strings.TrimSpace(old.Rule) != "" {
				d.Rule = old.Rule
			}
			if strings.TrimSpace(old.Regex) != "" {
				d.Regex = old.Regex
			}
			d.Enabled = old.Enabled
		}
		next = append(next, d)
	}
	// 保留用户自建字段（既不在新目录、也不在历史遗留清单里）
	keptCustom := 0
	for _, f := range cfg.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" || byName[name].Name != "" || energyLegacyOverviewKeys[name] {
			continue
		}
		next = append(next, f)
		keptCustom++
	}
	if len(cfg.Fields) == len(next) && keptCustom == 0 {
		// 长度相同不代表内容相同，仍然写回（低成本、幂等）
	}
	// 同 subs：`Updates(map{"fields": slice})` 会被当成 record 表达式，必须走结构体 Save
	cfg.Fields = next
	cfg.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Save(&cfg).Error; err != nil {
		return 0, err
	}
	return len(next), nil
}

func findFieldByName(fields []types.ExtractFieldConfig, name string) (types.ExtractFieldConfig, bool) {
	for _, f := range fields {
		if strings.TrimSpace(f.Name) == name {
			return f, true
		}
	}
	return types.ExtractFieldConfig{}, false
}

// energyFeeGroups 拥有「费用组成 / 分时时段」细项的费用分项分组。
var energyFeeGroups = []string{
	"market", "line", "trans", "sys",
	"gov-industrial", "catalog", "gov-residential",
}

// energyPeriodItems 分时时段细项的默认集合（账单原文的四种时段）。
//
// 注意：这里用「尖 / 峰 / 平 / 谷」而不是账单表头的「尖峰」——
// 费用行 period 列的原文既有「尖峰」也有「尖」，两者按前缀匹配同一时段，
// 配置面用短名，分词与分组统计更稳定。
var energyPeriodItems = []string{"尖", "峰", "平", "谷"}

// ensureEnergyPeriodSubItems 为每个费用分组补齐 `<group>-periods` 细项。
//
// 为什么需要它：`-rows`（费用组成细项）在旧配置里已经存在，但「分时时段」
// 从未有独立的细项组——它此前是写死在渲染里的常量。产品要求时段也要可配
// （不同省份分时政策不同），因此沉淀成 subs：group = `<费用分组>-periods`。
//
// 幂等：分组已有条目时跳过，不覆盖用户改名/停用。
func (h *UtilityHandler) ensureEnergyPeriodSubItems(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL",
			EnergyScope, EnergyBillTypeUtility).
		Order("sort_order ASC").First(&cat).Error; err != nil {
		return 0, nil
	}
	has := map[string]bool{}
	for _, s := range cat.Subs {
		has[strings.TrimSpace(s.Group)] = true
	}
	added := 0
	next := make([]types.FleetCategorySub, 0, len(cat.Subs)+len(energyFeeGroups)*len(energyPeriodItems))
	next = append(next, cat.Subs...)
	for _, g := range energyFeeGroups {
		pg := g + "-periods"
		if has[pg] {
			continue
		}
		for _, p := range energyPeriodItems {
			next = append(next, types.FleetCategorySub{
				Name:       p,
				Label:      p,
				Enabled:    true,
				IsDefault:  true,
				DataType:   "text",
				Group:      pg,
				GroupLabel: migratedGroupLabels[g] + " · 时段",
			})
			added++
		}
	}
	if added == 0 {
		return 0, nil
	}
	cat.Subs = next
	cat.UpdatedAt = timeNowUTC()
	if err := h.db.WithContext(ctx).Save(&cat).Error; err != nil {
		return 0, err
	}
	logger.Infof(ctx, "energy period sub-items added: %d", added)
	return added, nil
}

// BackfillBillingPeriod 把存量账单的账单周期刷正为 YYYY-MM。
//
// 背景：改造前 bill_period 是前端拼的 `${start} ~ ${end}`，落库时根本没有该字段；
// 列表于是显示成 "2026-09-0..."。现在 bill_period 是后端派生字段，但老数据没有，
// 必须回填一次，否则用户升级后看到的还是老样子。
//
// 幂等：只在该记录 bill_period 与目标值不同时写库；重复执行无副作用。
func (h *UtilityHandler) BackfillBillingPeriod(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	type row struct {
		ID             string
		CustomMetadata types.JSON
	}
	var rows []row
	if err := h.db.WithContext(ctx).Raw(`
		SELECT id, custom_metadata FROM knowledges
		WHERE deleted_at IS NULL
		  AND custom_metadata->>'kind' IN ('utility_bill','solar_bill','energy')`).Scan(&rows).Error; err != nil {
		return 0, err
	}
	patched := 0
	for _, r := range rows {
		var meta map[string]any
		if err := json.Unmarshal([]byte(r.CustomMetadata), &meta); err != nil {
			continue
		}
		records, _ := meta["records"].([]any)
		if len(records) == 0 {
			continue
		}
		changed := false
		for _, rec := range records {
			m, ok := rec.(map[string]any)
			if !ok {
				continue
			}
			start, _ := m["bill_period_start"].(string)
			end, _ := m["bill_period_end"].(string)
			raw, _ := m["bill_period"].(string)
			want := service.NormalizeBillingPeriod(raw)
			if want == "" {
				want = service.NormalizeBillingPeriod(start)
			}
			if want == "" {
				want = service.NormalizeBillingPeriod(end)
			}
			if want == "" {
				continue
			}
			if raw != want {
				m["bill_period"] = want
				changed = true
			}
		}
		if !changed {
			continue
		}
		buf, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		if err := h.db.WithContext(ctx).Exec(
			`UPDATE knowledges SET custom_metadata = ?::jsonb, updated_at = now() WHERE id = ?`,
			string(buf), r.ID).Error; err != nil {
			logger.Warnf(ctx, "backfill billing period failed for %s: %v", r.ID, err)
			continue
		}
		patched++
	}
	if patched > 0 {
		logger.Infof(ctx, "billing period backfilled: %d records", patched)
	}
	return patched, nil
}
