// 第 68 轮：花名册「离职人员」胶囊 = 在职人员字段全集 + 离职专有字段。
//
// 背景：离职详情抽屉要能回显员工在职期间的完整档案（岗位、学历、紧急联系人…），
// 但旧实现离职胶囊只有 15 个字段，用户在设置里找不到、列表也开不出来。
//
// 口径（强约束）：
//   · 已有 15 个离职字段的 enabled / is_default 状态**原样保留**；
//   · 新并入的在职字段一律 `enabled=false`、`is_default=false` ——
//     默认不出现在离职列表与抽屉，用户按需在「花名册设置 → 离职人员」打开；
//   · 幂等：按字段名差集补齐，重复执行零变更；用户在在职侧新增的自定义字段
//     同样会被镜像过来（同样默认关闭）。
package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// hrResignMirrorSubs68 把「在职人员」胶囊里缺的字段镜像到「离职人员」胶囊。
//
// 返回新的 subs 与是否有变更（调用方据此决定要不要落库）。
func hrResignMirrorSubs68(subs []types.FleetCategorySub) ([]types.FleetCategorySub, bool) {
	out := append([]types.FleetCategorySub{}, subs...)
	resigned := map[string]bool{}
	for i := range out {
		if strings.TrimSpace(out[i].Group) != hrRosterGroupResigned {
			continue
		}
		resigned[strings.TrimSpace(out[i].Name)] = true
	}
	changed := false
	// 先收集在职侧字段（保持其在职顺序），再统一追加，避免边遍历边 append。
	active := make([]types.FleetCategorySub, 0, len(out))
	for _, s := range out {
		if strings.TrimSpace(s.Group) == hrRosterGroupActive {
			active = append(active, s)
		}
	}
	for _, s := range active {
		name := strings.TrimSpace(s.Name)
		if name == "" || resigned[name] {
			continue
		}
		mirror := s
		mirror.Group = hrRosterGroupResigned
		mirror.GroupLabel = hrRosterGroupResigned
		// 新并入字段默认关闭：默认表头 + 启用双 false。
		mirror.Enabled = false
		mirror.IsDefault = false
		// 列宽不继承（离职列表列宽由用户自行决定，避免把在职的宽列带过来）。
		mirror.Width = 0
		// 第 69 轮：标记为「跨视图引用」——设置面板显示「在职人员」来源胶囊，
		// 导入模板自动排除（这些列不是离职表自己的录入列）。
		mirror.RefSource = hrRosterGroupActive
		mirror.SourceModule = "roster_active"
		// 从在职侧继承来的 Source（社保账号等）保持原样，由面板显示原来源。
		out = append(out, mirror)
		resigned[name] = true
		changed = true
	}
	return out, changed
}

// hrResignFullFlag68 首次合并标记（只做一次「默认关闭」归一，之后尊重用户开关）。
// 键位带 b 后缀：v68 首次上线时「默认关闭」归一未生效（历史进程已先写过标记），
// 换新键让归一逻辑在所有存量租户上真跑一次；之后再改开关不会被回滚。
const hrResignFullFlag68 = "hr_resign_full_fields_v68c"

// hrMigrateResignFull68 启动幂等迁移：每个花名册分类的离职胶囊补齐在职字段。
//
// **首次执行**时，除 15 个离职专有字段外，一律强制 enabled/is_default=false ——
// 覆盖历史修复脚本可能已并入但开关为 true 的引用字段（个人社保账号等）；
// 标记落库后再次启动只补缺、不改任何开关，用户后续的手动开启不会被回滚。
func (h *HRHandler) hrMigrateResignFull68(ctx context.Context) error {
	firstRun := !h.hrSystemFlag(ctx, hrResignFullFlag68)
	core := map[string]bool{}
	for _, d := range hrResignFieldDefs() {
		core[d.Name] = true
	}
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", hrScopeRoster).
		Find(&cats).Error; err != nil {
		return err
	}
	for i := range cats {
		changed := false
		if firstRun {
			for j := range cats[i].Subs {
				sub := &cats[i].Subs[j]
				if strings.TrimSpace(sub.Group) != hrRosterGroupResigned {
					continue
				}
				if core[strings.TrimSpace(sub.Name)] {
					continue
				}
				if sub.Enabled || sub.IsDefault {
					sub.Enabled = false
					sub.IsDefault = false
					changed = true
				}
			}
		}
		/**
		 * 第 69 轮：历史合并进来的在职字段可能还没有 ref_source（第 68 轮只搬了
		 * 字段本身）。这里按「离职组 + 非离职专有 + 不是跨分类取值引用」补齐，
		 * 幂等：已有值不覆盖，用户改过的来源标识也不会被冲掉。
		 */
		for j := range cats[i].Subs {
			sub := &cats[i].Subs[j]
			if strings.TrimSpace(sub.Group) != hrRosterGroupResigned {
				continue
			}
			if core[strings.TrimSpace(sub.Name)] || sub.Source != nil {
				continue
			}
			if sub.RefSource == hrRosterGroupActive && sub.SourceModule == "roster_active" {
				continue
			}
			sub.RefSource = hrRosterGroupActive
			sub.SourceModule = "roster_active"
			changed = true
		}
		next, mirrored := hrResignMirrorSubs68(cats[i].Subs)
		if mirrored {
			cats[i].Subs = next
			changed = true
		}
		if !changed {
			continue
		}
		// 结构体 Save 走 JSON 序列化；Update("subs", 切片) 在 PG 会报 42804。
		if err := h.db.WithContext(ctx).Save(&cats[i]).Error; err != nil {
			return err
		}
		logger.Infof(ctx, "离职人员字段已补齐在职字段全集（tenant=%d）", cats[i].TenantID)
	}
	if firstRun {
		h.hrMarkSystemFlag(ctx, hrResignFullFlag68)
	}
	return nil
}
