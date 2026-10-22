package handler

import (
	"context"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗（energy scope）内置分类的种子与幂等协调。
//
// 独立成文件的原因：fleet.go 已经 3000+ 行，再往里塞业务专属 seed 会让它
// 继续膨胀；能耗的分类语义（账期台账）与合同/制度/通知都不同，混在一起容易
// 在后续维护时互相污染。
//
// 幂等原则（与 reconcileContractCategories 一致）：
//   - builtin_key 是锚点，名字以库为准 —— 用户改名后不能每次启动都复活旧名分类；
//   - subs 以 seed 为准做重排，但保留用户改过的 enabled/is_default/width/data_type；
//   - 缺字段补齐，绝不删除用户自建字段。

// energySeedSubs 为能耗内置分类的字段全集。
//
// 键名与 internal/application/service/business_field_catalog_energy.go 的
// energyCatalog() 逐项一致（snake_case）；顺序 = 界面顺序。
// 注意：seed 里**不放** fileName —— 它是列表系统列，不是提取字段。
func energySeedSubs() []types.FleetCategorySub {
	return []types.FleetCategorySub{
		// ---- 标准 11（默认显示）----
		{Name: "utility_no", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "billing_period", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "utility_type", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "previous_reading", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "current_reading", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "multiplier", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "usage_amount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "unit_price", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "total_amount", Enabled: true, IsDefault: true, DataType: "number"},
		{Name: "extract_status", Enabled: true, IsDefault: true, DataType: "text"},
		{Name: "tags", Enabled: true, IsDefault: true, DataType: "array"},
		// ---- 扩展 9（默认不显示）----
		{Name: "account_name", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "meter_no", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "address", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "period_start", Enabled: true, IsDefault: false, DataType: "date"},
		{Name: "period_end", Enabled: true, IsDefault: false, DataType: "date"},
		{Name: "read_date", Enabled: true, IsDefault: false, DataType: "date"},
		{Name: "usage_category", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "voltage_level", Enabled: true, IsDefault: false, DataType: "text"},
		{Name: "remark", Enabled: true, IsDefault: false, DataType: "textarea"},
	}
}

// mergeEnergySeedSubs 以 seed 为准重排 subs，保留用户改过的展示属性。
//
// 规则与 mergeContractSeedSubs 一致：
//   1. seed 里的字段按 seed 顺序输出（抽屉/列表/字段面板三处同序）；
//   2. 已有同名项保留用户的 enabled/is_default/width/data_type/sub_fields；
//   3. seed 之外的历史字段不删除（用户自建字段不能被系统清掉）。
func mergeEnergySeedSubs(existing []types.FleetCategorySub) []types.FleetCategorySub {
	seed := energySeedSubs()
	byKey := make(map[string]types.FleetCategorySub, len(existing))
	for _, s := range existing {
		name := service.SnakeKey(s.Name)
		if name == "" {
			continue
		}
		if _, dup := byKey[name]; dup {
			continue
		}
		s.Name = name
		byKey[name] = s
	}
	out := make([]types.FleetCategorySub, 0, len(seed)+len(existing))
	seen := make(map[string]bool, len(seed))
	for _, f := range seed {
		if s, ok := byKey[f.Name]; ok {
			out = append(out, s)
		} else {
			out = append(out, f)
		}
		seen[f.Name] = true
	}
	// 用户自建字段追加在后面（不丢）
	for _, s := range existing {
		name := service.SnakeKey(s.Name)
		if name == "" || seen[name] {
			continue
		}
		out = append(out, s)
	}
	return out
}

// reconcileEnergyCategories 幂等协调能耗内置分类。
//
// 内置分类：工业用电 / 生活用水 / 燃气费 / 供热费（按业务常见形态铺）。
// 用户可改名/停用/删除；协调逻辑以 builtin_key 为锚点，不会复活旧名分类。
func (h *FleetHandler) reconcileEnergyCategories(ctx context.Context) {
	now := timeNowUTC()
	wanted := []struct {
		Name string
		Key  string
		Sort int
	}{
		{"工业用电", "energy-electricity", 0},
		{"生活用水", "energy-water", 1},
		{"燃气费", "energy-gas", 2},
		{"供热费", "energy-heat", 3},
	}
	for _, w := range wanted {
		var cat types.FleetCategory
		// 按 builtin_key 定位存量行（含软删）：名字以库为准，
		// 用户改名后按 name 查旧名会查空 → 每次重启复活一个 0 份旧名分类。
		err := h.db.WithContext(ctx).
			Where("tenant_id = 0 AND scope = ? AND builtin_key = ?", "energy", w.Key).
			First(&cat).Error
		if err == nil {
			// 分类已存在：只重排 subs（补齐缺字段、保留用户改过的属性），不改名。
			merged := mergeEnergySeedSubs(cat.Subs)
			if !fleetCategorySubsEqual(cat.Subs, merged) {
				cat.Subs = merged
				cat.UpdatedAt = now
				if uerr := h.db.WithContext(ctx).Save(&cat).Error; uerr != nil {
					logger.Warnf(ctx, "update energy category %s failed: %v", cat.Name, uerr)
				}
			}
			continue
		}
		// 全新环境：按固定 ID + builtin_key 创建
		cat = types.FleetCategory{
			ID:         "seed-energy-" + w.Key,
			TenantID:   0,
			Scope:      "energy",
			BuiltinKey: w.Key,
			Name:       w.Name,
			Subs:       energySeedSubs(),
			SortOrder:  w.Sort,
			Enabled:    true,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if cerr := h.db.WithContext(ctx).Create(&cat).Error; cerr != nil {
			logger.Warnf(ctx, "seed energy category %s failed: %v", w.Name, cerr)
		}
	}
}

// fleetCategorySubsEqual 比较两组 subs 是否等价（顺序 + 关键属性）。
func fleetCategorySubsEqual(a, b []types.FleetCategorySub) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Enabled != b[i].Enabled ||
			a[i].IsDefault != b[i].IsDefault ||
			a[i].DataType != b[i].DataType {
			return false
		}
	}
	return true
}
