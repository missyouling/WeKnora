// 第 59 轮：初始化只做一次 + 分组更名 + 胶囊隔离的安全修复。
//
// 背景：第 58 轮之前，NewHRHandler 每次启动都跑 hrSeed* / hrMigrate* / hrBackfill*，
// 用户删掉的字段与业务记录会在重启后被原样加回来。本文件提供三件事：
//
//  1. hr_initial_seeded 系统配置标记：首次初始化（迁移 + 种子）完成后写 true；
//     之后启动只做幂等修复（hrRepair58Once / hrRenameGroups59），不再回填。
//  2. hrRepair58Once：清同一胶囊内重复的「合同状态」、清「正常记录 / 补退记录」
//     胶囊里串入的五险参保状态字段 —— 只删错放的字段，不新增、不复活。
//  3. hrRenameGroups59：旧胶囊名 → 新胶囊名的幂等迁移（在职管理→在职人员、
//     离职管理→离职人员、异常记录→补退记录），保留字段顺序与用户配置。
//
// 边界：本文件只新增，不改任何上游核心代码；标识符全部以 hr 前缀收口。
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// hrInitialSeededKey 系统配置键：HR 首次初始化（字段迁移 + 业务种子）已完成。
//
// 复用平台已有的 system_settings 表（迁移 000053），不新建表；值类型 bool。
const hrInitialSeededKey = "hr_initial_seeded"

// hrSIStatusFields 五险参保状态字段（只有「回盘表」胶囊拥有）。
var hrSIStatusFields = []string{
	"养老参保状态", "失业参保状态", "医疗参保状态", "工伤参保状态", "生育参保状态",
}

// hrIsSIStatusField 是否五险参保状态字段。
func hrIsSIStatusField(name string) bool {
	n := strings.TrimSpace(name)
	for _, f := range hrSIStatusFields {
		if n == f {
			return true
		}
	}
	return false
}

// hrRosterGroupRenames59 第 59 轮旧胶囊名 → 新胶囊名（与前端同源）。
var hrRosterGroupRenames59 = map[string]string{
	"在职管理": "在职人员",
	"离职管理": "离职人员",
	"异常记录": "补退记录",
}

// hrInitialSeeded 读取「首次初始化已完成」标记；读不到 / 值非法一律按未完成处理。
func (h *HRHandler) hrInitialSeeded(ctx context.Context) bool {
	var row types.SystemSetting
	if err := h.db.WithContext(ctx).Where("key = ?", hrInitialSeededKey).First(&row).Error; err != nil {
		return false
	}
	if row.ValueType != "bool" {
		return false
	}
	ok, err := row.AsBool()
	return err == nil && ok
}

// hrMarkInitialSeeded 写入/更新标记为 true（幂等，按 key upsert）。
func (h *HRHandler) hrMarkInitialSeeded(ctx context.Context) error {
	value, err := json.Marshal(true)
	if err != nil {
		return err
	}
	row := types.SystemSetting{
		Key:         hrInitialSeededKey,
		Value:       types.JSON(value),
		ValueType:   "bool",
		Category:    "hr",
		Description: "人事模块首次初始化（字段迁移与样例数据）已完成，之后启动只做幂等修复",
	}
	return h.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "value_type", "updated_at"}),
	}).Create(&row).Error
}

// hrRepair58Once 幂等安全修复（不新增任何字段 / 记录）：
//
//	(a) 同一 group 内重复的「合同状态」按 name 去重，保留第一条；
//	(b) 「正常记录 / 补退记录」胶囊里的五险参保状态字段直接剔除（只应存在于回盘表）。
//
// 标记存在时每次启动都跑；标记缺失时由首次迁移重建字段配置，无需再修。
func (h *HRHandler) hrRepair58Once(ctx context.Context) error {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?", []string{
			hrScopeRoster, hrScopeResign, hrScopeContract, hrScopeSocial, hrScopeFund,
		}).
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		subs := cats[i].Subs
		next := make([]types.FleetCategorySub, 0, len(subs))
		seenStatus := map[string]bool{}
		changed := false
		for _, sub := range subs {
			name := strings.TrimSpace(sub.Name)
			if cats[i].Scope == hrScopeSocial && hrIsSIStatusField(name) {
				g := strings.TrimSpace(sub.Group)
				if g == hrSocNormalGroup || g == hrSocAbnormalGroup {
					changed = true
					continue
				}
			}
			if name == "合同状态" {
				key := strings.TrimSpace(sub.Group)
				if seenStatus[key] {
					changed = true
					continue
				}
				seenStatus[key] = true
				if !sub.Builtin {
					sub.Builtin = true
					changed = true
				}
			}
			next = append(next, sub)
		}
		if !changed {
			continue
		}
		cats[i].Subs = next
		cats[i].UpdatedAt = time.Now()
		// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "HR 字段配置安全修复完成（scope=%s tenant=%d）", cats[i].Scope, cats[i].TenantID)
	}
	return nil
}

// hrRenameGroups59 旧胶囊名 → 新胶囊名（幂等，保留字段顺序与其它配置）。
//
// 只映射 group / group_label，不动字段顺序、显示名、宽度、默认表头与启停；
// group_label 仅在它仍等于旧名（或为空）时跟随改名，用户自定义的胶囊标题保留。
func (h *HRHandler) hrRenameGroups59(ctx context.Context) error {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("deleted_at IS NULL AND scope IN ?", []string{hrScopeRoster, hrScopeSocial}).
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		changed := false
		for j := range cats[i].Subs {
			old := strings.TrimSpace(cats[i].Subs[j].Group)
			next, ok := hrRosterGroupRenames59[old]
			if !ok {
				continue
			}
			cats[i].Subs[j].Group = next
			label := strings.TrimSpace(cats[i].Subs[j].GroupLabel)
			if label == "" || label == old {
				cats[i].Subs[j].GroupLabel = next
			}
			changed = true
		}
		if !changed {
			continue
		}
		cats[i].UpdatedAt = time.Now()
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "HR 胶囊分组已更名（scope=%s tenant=%d）", cats[i].Scope, cats[i].TenantID)
	}
	return nil
}
