// 车队记录子分类回填（第 29 轮）。
//
// 背景：KPI 卡片的数字与列表行数必须 100% 同源。卡片按 `fleet_records.sub_category`
// 聚合，列表按同一列过滤；但存量记录该列是空的（旧版只在 data/doc_type 里散着），
// 结果就是「卡片显示 2 份、点开列表 1 条甚至 0 条」。
//
// 本文件把旧记录的子分类从 data（证照类型/分类）与 doc_type 归一写回该列，
// 启动时幂等执行一次；用户重新提取/保存时也会自动带值。
package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// backfillFleetRecordSubCategory 把 sub_category 为空的记录从 data/doc_type 补齐。
//
// 只处理空值：用户已在设置里明确指定过分类的记录一律不动（避免覆盖人工口径）。
// 分批更新，避免一次性把几千条记录读进内存。
func (h *FleetHandler) backfillFleetRecordSubCategory(ctx context.Context) int {
	var rows []types.FleetRecord
	if err := h.db.WithContext(ctx).
		Select("id", "data", "doc_type", "sub_category").
		Where("deleted_at IS NULL AND (sub_category IS NULL OR sub_category = '')").
		Limit(5000).
		Find(&rows).Error; err != nil {
		logger.Warnf(ctx, "backfill fleet sub_category: load failed: %v", err)
		return 0
	}
	fixed := 0
	for i := range rows {
		name := fleetSubCategoryOf(rows[i].Data, rows[i].DocType)
		if name == "" {
			continue
		}
		if err := h.db.WithContext(ctx).Model(&types.FleetRecord{}).
			Where("id = ?", rows[i].ID).
			Update("sub_category", name).Error; err != nil {
			logger.Warnf(ctx, "backfill fleet sub_category %s failed: %v", rows[i].ID, err)
			continue
		}
		fixed++
	}
	if fixed > 0 {
		logger.Infof(ctx, "backfilled sub_category for %d fleet record(s)", fixed)
	}
	return fixed
}

// normalizeFleetSubCategoryName 把 builtin_key 形态的分类标识归一成中文分类名。
//
// 存量数据里 doc_type 可能是 builtin_key（vehicle-license / driver-license），
// 而卡片与筛选器用的是分类 name（道路运输证 / 驾驶证）。两者混用会让
// 「按卡片过滤」查不到历史记录，因此统一走这张映射表（未知键原样返回）。
func normalizeFleetSubCategoryName(key string, cats []types.FleetCategory) string {
	k := strings.TrimSpace(key)
	if k == "" {
		return ""
	}
	for i := range cats {
		if cats[i].BuiltinKey != "" && cats[i].BuiltinKey == k {
			return cats[i].Name
		}
		if cats[i].Name == k {
			return k
		}
	}
	return k
}

// fleetArchiveFilterableFields 档案类分类的默认可筛字段（每个分类最多 2~3 个）。
//
// 档案类分类的 subs 是用户在「设置 → 字段定义」里维护的，没有 seed 兜底，
// 因此这里按「字段名白名单」统一收敛：只有归属键（车牌号）、核心编号、
// 状态/持有人可筛，其余详情字段一律不可筛 —— 否则筛选栏会被 17 个字段铺满。
var fleetArchiveFilterableFields = map[string]bool{
	"车牌号": true, "车牌号码": true, "plate_no": true,
	"证件编号": true, "许可证号": true, "证号": true, "档案编号": true,
	"保单号": true, "工单号": true, "维护记录号": true, "编号": true, "单号": true,
	"证件状态": true, "状态": true, "保单状态": true,
	"持有人": true, "司机姓名": true, "业户名称": true, "被保险人名称": true,
}

// backfillFleetCategoryFilterable 给存量分类的 subs 补齐 filterable 标记（幂等）。
//
// 数据来源优先级：
//  1. 记录型 9 个分类：直接用 `fleetRecordCategorySeeds()` 里的 filt 标记
//     （种子是唯一真源，避免这里再维护一份白名单）；
//  2. 其他分类（档案类等，subs 由用户维护）：按 `fleetArchiveFilterableFields`
//     白名单判定。
//
// 判据是「已标记就跳过」：只要该分类已有任意字段带 filterable=true，
// 说明用户/上一轮已经调过，不再覆盖（禁用自动保存原则）。
func (h *FleetHandler) backfillFleetCategoryFilterable(ctx context.Context) int {
	// 种子期望值：(scope, name) → 字段名 → 是否可筛
	want := map[string]map[string]bool{}
	for _, seed := range fleetRecordCategorySeeds() {
		m := map[string]bool{}
		for _, sub := range seed.Subs {
			m[sub.Name] = sub.Filterable
		}
		want[seed.Scope+"|"+seed.Name] = m
	}

	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "backfill filterable: load categories failed: %v", err)
		return 0
	}
	fixed := 0
	for i := range cats {
		subs := cats[i].Subs
		if len(subs) == 0 {
			continue
		}
		marked := false
		for _, s := range subs {
			if s.Filterable {
				marked = true
				break
			}
		}
		if marked {
			continue
		}
		seedMap := want[cats[i].Scope+"|"+cats[i].Name]
		changed := false
		for j := range subs {
			should := false
			if seedMap != nil {
				should = seedMap[subs[j].Name]
			} else {
				should = fleetArchiveFilterableFields[subs[j].Name]
			}
			if should {
				subs[j].Filterable = true
				changed = true
			}
		}
		if !changed {
			continue
		}
		cats[i].Subs = subs
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			logger.Warnf(ctx, "backfill filterable for %s failed: %v", cats[i].Name, err)
			continue
		}
		fixed++
	}
	if fixed > 0 {
		logger.Infof(ctx, "backfilled filterable for %d fleet category(ies)", fixed)
	}
	return fixed
}
