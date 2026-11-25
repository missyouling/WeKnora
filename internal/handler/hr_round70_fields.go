package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

/**
 * 第 70 轮：宿舍域字段契约迁移（六同步的唯一真相源是 `fleet_categories.subs`）。
 *
 * 只改前端常量是不够的 —— 表头、`[☰ 字段]` 筛选器、设置抽屉、编辑抽屉、导入模板
 * 全部由 subs 派生。库里还留着第 65 轮那套字段：
 *   · 宿舍档案挂着「关联气表 / 气费标准 / 垃圾处理费 / 二次供水费 / 污水处理费 /
 *     房屋租金 / 物业费」—— 这些不属于员工宿舍；
 *   · 公租房档案挂着「关联电表 / 电费标准」—— 公租房不定义电表；
 *   · 住宿记录挂着「住宿记录ID」—— 内部主键不该出现在任何界面。
 *
 * 迁移原则：
 *   · **只增删字段定义，不动任何业务数据**（custom_data 里的历史键原样保留，
 *     只是不再有对应列/表单；需要时可回滚迁移重建字段）；
 *   · 幂等：重复执行结果一致；
 *   · 表计类字段与「成员」按需求补到指定位置（成员紧跟管理员/舍长）。
 */
func (h *HRHandler) hrEnsureDormFields70(ctx context.Context) {
	var cats []types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("scope = ? AND deleted_at IS NULL", "hr-dorm").
		Find(&cats).Error; err != nil {
		logger.Warnf(ctx, "读取宿舍字段配置失败: %v", err)
		return
	}
	for i := range cats {
		cat := &cats[i]
		changed := false
		kept := make([]types.FleetCategorySub, 0, len(cat.Subs))
		for _, sub := range cat.Subs {
			group := strings.TrimSpace(sub.Group)
			name := strings.TrimSpace(sub.Name)
			if hrDormFieldDrop70(name, group) {
				changed = true
				continue
			}
			kept = append(kept, sub)
		}

		// ---- 补字段：表计（费用项目）与「成员」（紧跟管理员/舍长） ----
		insertAfterManager := func(sub types.FleetCategorySub) {
			idx := -1
			for k, s := range kept {
				if strings.TrimSpace(s.Name) == "管理员/舍长" && strings.TrimSpace(s.Group) == sub.Group {
					idx = k
				}
			}
			if idx < 0 {
				kept = append(kept, sub)
				return
			}
			kept = append(kept[:idx+1], append([]types.FleetCategorySub{sub}, kept[idx+1:]...)...)
		}
		appendIfMissing := func(name, group, dataType string, isDefault bool) {
			for _, s := range kept {
				if strings.TrimSpace(s.Name) == name && strings.TrimSpace(s.Group) == group {
					return
				}
			}
			kept = append(kept, types.FleetCategorySub{
				Name: name, Group: group, GroupLabel: group,
				Enabled: true, IsDefault: isDefault, DataType: dataType,
			})
			changed = true
		}

		for _, g := range []string{"宿舍档案", "公租房档案"} {
			if g == "宿舍档案" {
				appendIfMissing("宿舍电表名称", g, "text", false)
				appendIfMissing("宿舍电表号", g, "text", false)
				appendIfMissing("电表倍率", g, "number", false)
				appendIfMissing("宿舍水表名称", g, "text", false)
				appendIfMissing("宿舍水表号", g, "text", false)
				appendIfMissing("水表倍率", g, "number", false)
			} else {
				appendIfMissing("公租房水表名称", g, "text", false)
				appendIfMissing("公租房水表号", g, "text", false)
				appendIfMissing("水表倍率", g, "number", false)
				appendIfMissing("公租房气表名称", g, "text", false)
				appendIfMissing("公租房气表号", g, "text", false)
				appendIfMissing("气表倍率", g, "number", false)
			}
			// 成员：只读派生列，默认进表头（需求 7 ③）
			exists := false
			for _, s := range kept {
				if strings.TrimSpace(s.Name) == "成员" && strings.TrimSpace(s.Group) == g {
					exists = true
				}
			}
			if !exists {
				insertAfterManager(types.FleetCategorySub{
					Name: "成员", Group: g, GroupLabel: g,
					Enabled: true, IsDefault: true, DataType: "text",
				})
				changed = true
			}
		}

		if !changed {
			continue
		}
		cat.Subs = kept
		if err := h.db.WithContext(ctx).Model(cat).Select("subs").Updates(cat).Error; err != nil {
			logger.Warnf(ctx, "宿舍字段契约迁移失败（%s）: %v", cat.Name, err)
			continue
		}
		logger.Infof(ctx, "第 70 轮已重构宿舍字段契约（%s，tenant=%d）：%d 个字段",
			cat.Name, cat.TenantID, len(kept))
	}
}

// hrDormFieldDrop70 该字段是否应从指定分组移除。
//
// 用「去掉括号单位后的基名」比较，兼容 `气费标准` 与 `气费标准(元/m³)` 两种历史写法；
// 宿舍保留 电费标准 / 水费单价 / 宿舍押金（员工宿舍真实存在），
// 公租房保留 水费单价 / 气费标准 / 垃圾 / 二次供水 / 污水 / 租金 / 物业。
func hrDormFieldDrop70(name, group string) bool {
	base := strings.TrimSpace(strings.SplitN(strings.TrimSpace(name), "(", 2)[0])
	switch group {
	case "宿舍档案":
		switch base {
		case "关联电表", "关联水表", "关联气表",
			"气费标准", "垃圾处理费", "二次供水费", "污水处理费", "房屋租金", "物业费":
			return true
		}
	case "公租房档案":
		switch base {
		case "关联电表", "关联水表", "关联气表", "电费标准":
			return true
		}
	case "住宿记录":
		// 内部主键不对用户展示（接口路由仍用 id）
		if strings.TrimSpace(name) == "住宿记录ID" {
			return true
		}
	}
	return false
}
