package handler

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// legacyFleetWidthDefaults 第 32 轮那段「按数据类型给初始列宽」遗留的三个值。
//
// 当时 withDefaultFieldWidths 在**每次打开设置面板**时把 width=0 覆盖成默认值，
// 用户只要点过一次保存，这些值就被写进 subs。现在默认口径回到 0（自适应），
// 需要把这些「被 bug 写进去的」值清掉 —— 否则设置面板一打开满屏 160，
// 用户以为 0 又没存住（第 44 轮真机图 3）。
var legacyFleetWidthDefaults = map[string]int{
	"text":     160,
	"textarea": 260,
	"number":   120,
	"date":     120,
	"array":    160,
	"items":    160,
}

// fleetWidthResetMigration 这一次性修正的标记名。
//
// 必须只跑一次：清完之后用户完全可能故意把某列设成 160，若每次启动都再清一遍，
// 「任意 0~600 的整数都要原样持久化」这条需求就被自己的修理工破坏了。
const fleetWidthResetMigration = "fleet_sub_width_reset_r44"

// NormalizeFleetSubWidths 把车队 12 个菜单里「被历史默认值污染的列宽」还原成 0（只跑一次）。
//
// 判据刻意收窄到「该值恰好等于它所属数据类型的旧默认值」：
//   - 160 的 text 字段 → 清 0（用户不会精确挑 160，这是 bug 写的）；
//   - 120 的 number 字段 → 清 0；
//   - 用户手填的 180/200/240 一律不动。
//
// 只碰 6 个车队 scope，不触达发票/合同/能耗（它们的 width 语义本就独立）。
func (h *FleetHandler) NormalizeFleetSubWidths(ctx context.Context) {
	if h == nil || h.db == nil {
		return
	}
	if !h.claimDataMigration(ctx, fleetWidthResetMigration) {
		return
	}
	scopes := []string{"vehicle", "driver", "maintain", "tire", "inspection", "fleet-cost"}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?", scopes).
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "fleet sub width normalize: query failed: %v", err)
		return
	}
	changed := 0
	for i := range cats {
		cat := &cats[i]
		dirty := false
		subs := make([]types.FleetCategorySub, len(cat.Subs))
		copy(subs, cat.Subs)
		for j := range subs {
			sub := &subs[j]
			want, ok := legacyFleetWidthDefaults[sub.DataType]
			if !ok || sub.Width != want {
				continue
			}
			sub.Width = 0
			dirty = true
		}
		if !dirty {
			continue
		}
		cat.Subs = subs
		// 只能整体 Save：Update("subs", slice) 在 PG 上会被静默忽略（历史踩坑）。
		if err := h.db.WithContext(ctx).Save(cat).Error; err != nil {
			logger.Warnf(ctx, "fleet sub width normalize: save %s failed: %v", cat.Name, err)
			continue
		}
		changed++
	}
	if changed > 0 {
		logger.Infof(ctx, "fleet sub width normalize: reset legacy widths on %d categories", changed)
	}
}

// claimDataMigration 领取一次一次性数据修正的执行权。
//
// 还没跑过 → 写入标记并返回 true（调用方随后执行修正）；
// 已跑过 → 返回 false。
//
// 标记表是二开自建的（不碰上游 schema_migrations，避免版本号错位）。
// 建表/读写失败时**返回 false**：宁可这次不修，也不能每次启动都改用户配置。
func (h *FleetHandler) claimDataMigration(ctx context.Context, name string) bool {
	if err := h.db.WithContext(ctx).Exec(
		`CREATE TABLE IF NOT EXISTS business_data_migrations (
			name VARCHAR(120) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL
		)`).Error; err != nil {
		logger.Warnf(ctx, "data migration marker table unavailable: %v", err)
		return false
	}
	var count int64
	if err := h.db.WithContext(ctx).Table("business_data_migrations").
		Where("name = ?", name).Count(&count).Error; err != nil {
		logger.Warnf(ctx, "data migration marker read failed: %v", err)
		return false
	}
	if count > 0 {
		return false
	}
	// 先落标记再执行：中途失败最多是「这次没修干净」，不会变成每次启动都重跑。
	if err := h.db.WithContext(ctx).Table("business_data_migrations").
		Create(map[string]any{"name": name, "applied_at": time.Now().UTC()}).Error; err != nil {
		logger.Warnf(ctx, "data migration marker write failed: %v", err)
		return false
	}
	return true
}
