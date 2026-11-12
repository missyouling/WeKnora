// 费用清单「本期水量 / 本期水费」字段规范与派生计算（第 51 轮）。
//
// 背景：用户在「设置·能耗 → 费用清单」里用「复制字段」造出了这两列，
// 只改了 label（本期水量 / 本期水费），name 仍是 总电费_副本 / 总用水量_副本，
// 且没有 source。前端旧版静态列白名单会把这两条整条丢弃，表头因此不渲染。
//
// 本文件负责两件事：
//  1. 把存量 subs 里的副本字段幂等规范成固定契约键
//     current_water_usage / current_water_fee，并钉死在 本期电费 之后；
//  2. 提供纯函数从水表行聚合出本期水量 / 本期水费，供生成与重算共用。
package handler

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	waterUsageFieldKey   = "current_water_usage"
	waterFeeFieldKey     = "current_water_fee"
	waterUsageFieldLabel = "本期水量"
	waterFeeFieldLabel   = "本期水费"
	waterSourceTip       = `数据来源：水费核算「自来水总表 + 消防总表」`
)

// hrWaterFieldAliases 历史上出现过的字段身份别名。
//
// 复制字段只改 label 不改 name，所以存量里既有中文 label 也有「_副本」后缀的
// 随机数。这里按 label 优先、name 兜底，避免把用户自己新增的同名业务列误并。
var waterFieldAliases = map[string][]string{
	waterUsageFieldKey: {
		"本期水量", "总用水量_副本", "总电费_副本", "total_water_usage", "current_water", "water_usage",
	},
	waterFeeFieldKey: {
		"本期水费", "总水费_副本", "total_water_fee", "current_water_fee", "water_fee",
	},
}

// isDefaultGroupSub 只处理费用清单的「默认分组」（业主/重庆星达视角）。
func isDefaultGroupSub(s types.FleetCategorySub) bool {
	g := strings.TrimSpace(s.Group)
	return g == "" || g == "default" || g == "cols"
}

// matchWaterField 判断一个 sub 是不是「本期水量 / 本期水费」的某种历史写法。
// 返回规范后的 key；不匹配返回空串。
func matchWaterField(s types.FleetCategorySub, want string) bool {
	label := strings.TrimSpace(s.Label)
	name := strings.TrimSpace(s.Name)
	if name == want || label == (map[string]string{
		waterUsageFieldKey: waterUsageFieldLabel,
		waterFeeFieldKey:   waterFeeFieldLabel,
	}[want]) {
		return true
	}
	for _, alias := range waterFieldAliases[want] {
		if label == alias || name == alias {
			return true
		}
	}
	return false
}

// normalizeCostListWaterSubs 把副本字段规范成固定契约键，并钉在 total_fee 之后。
//
// 幂等：重复执行不产生变化。只在两个字段都缺失时才新建；用户手动改过的 label
// 会被保留（只修 name / 类型 / 宽度 / 分组），避免把「本期水量·实际」之类的
// 自定义文案冲掉。
func normalizeCostListWaterSubs(subs []types.FleetCategorySub) ([]types.FleetCategorySub, bool) {
	changed := false
	// 1) 原位规范：找到每个目标字段的第一条，改成固定 key / 类型 / 宽度 / 分组
	seen := map[string]bool{}
	for i := range subs {
		if !isDefaultGroupSub(subs[i]) {
			continue
		}
		for _, key := range []string{waterUsageFieldKey, waterFeeFieldKey} {
			if seen[key] || !matchWaterField(subs[i], key) {
				continue
			}
			seen[key] = true
			dataType := "number"
			label := waterUsageFieldLabel
			if key == waterFeeFieldKey {
				dataType = "amount"
				label = waterFeeFieldLabel
			}
			if subs[i].Name != key {
				subs[i].Name = key
				changed = true
			}
			if strings.TrimSpace(subs[i].Label) == "" || strings.Contains(subs[i].Label, "_副本") {
				subs[i].Label = label
				changed = true
			}
			if subs[i].DataType != dataType {
				subs[i].DataType = dataType
				changed = true
			}
			if subs[i].Width != 0 {
				subs[i].Width = 0
				changed = true
			}
			if !subs[i].Enabled {
				subs[i].Enabled = true
				changed = true
			}
			if !subs[i].IsDefault {
				subs[i].IsDefault = true
				changed = true
			}
			break
		}
	}
	// 2) 缺失的补齐（追加到末尾，随后统一重排到 total_fee 之后）
	for _, key := range []string{waterUsageFieldKey, waterFeeFieldKey} {
		if seen[key] {
			continue
		}
		dataType := "number"
		label := waterUsageFieldLabel
		if key == waterFeeFieldKey {
			dataType = "amount"
			label = waterFeeFieldLabel
		}
		subs = append(subs, types.FleetCategorySub{
			Name: key, Label: label, Enabled: true, IsDefault: true,
			DataType: dataType, Width: 0, Group: "default", GroupLabel: "默认分组",
		})
		seen[key] = true
		changed = true
	}
	// 3) 重排：两条水字段紧跟 total_fee；组内保持 users 拖拽的相对顺序
	keys := map[string]bool{waterUsageFieldKey: true, waterFeeFieldKey: true}
	waterRows := make([]types.FleetCategorySub, 0, 2)
	rest := make([]types.FleetCategorySub, 0, len(subs))
	for _, s := range subs {
		if isDefaultGroupSub(s) && keys[strings.TrimSpace(s.Name)] {
			waterRows = append(waterRows, s)
			continue
		}
		rest = append(rest, s)
	}
	// 固定顺序：本期水量 → 本期水费
	for i := range waterRows {
		for j := i + 1; j < len(waterRows); j++ {
			if waterRows[i].Name == waterFeeFieldKey && waterRows[j].Name == waterUsageFieldKey {
				waterRows[i], waterRows[j] = waterRows[j], waterRows[i]
			}
		}
	}
	at := -1
	for i, s := range rest {
		if strings.TrimSpace(s.Name) == "total_fee" && isDefaultGroupSub(s) {
			at = i
			break
		}
	}
	out := make([]types.FleetCategorySub, 0, len(subs))
	if at < 0 {
		out = append(out, waterRows...)
		out = append(out, rest...)
	} else {
		out = append(out, rest[:at+1]...)
		out = append(out, waterRows...)
		out = append(out, rest[at+1:]...)
	}
	// 4) 判定是否真的变了（顺序也算）
	if len(out) == len(subs) {
		same := true
		for i := range out {
			if out[i].Name != subs[i].Name {
				same = false
				break
			}
		}
		if same {
			return out, changed
		}
	}
	return out, true
}

// EnsureCostListWaterFields 启动时幂等规范费用清单的水字段定义。
//
// 只动「费用清单」分类；其余 10 个能耗分类不受影响。失败只返回 error，
// 由调用方决定告警级别，不 panic、不阻断服务启动。
func (h *UtilityHandler) EnsureCostListWaterFields(ctx context.Context) (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var cat types.FleetCategory
	if err := h.db.WithContext(ctx).
		Where("tenant_id = 0 AND scope = ? AND name = ? AND deleted_at IS NULL", EnergyScope, "费用清单").
		First(&cat).Error; err != nil {
		return 0, nil
	}
	next, changed := normalizeCostListWaterSubs(cat.Subs)
	if !changed {
		return 0, nil
	}
	cat.Subs = next
	if err := h.db.WithContext(ctx).Save(&cat).Error; err != nil {
		return 0, err
	}
	return len(next), nil
}

// waterPeriodTotals 从当月水表行聚合「本期水量 / 本期水费」。
//
// 口径（任务 §1.1）：只认自来水总表与消防总表两类，其余分表/宿舍表不进这个
// 总数，它们用于「用水量（工业）」的扣减。金额为 0 而用量非 0 时（台账只记了
// 抄表读数、没记金额）按「用量 × 该表单价」回算，避免出现 0 元水费。
func waterPeriodTotals(rows []waterRowWithPrice) (float64, float64) {
	usage, fee := 0.0, 0.0
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		isMain := strings.Contains(name, "自来水总表") || strings.Contains(name, "自来水")
		isFire := strings.Contains(name, "消防")
		if !isMain && !isFire {
			continue
		}
		u := r.Usage
		if u <= 0 && r.End > r.Start {
			u = r.End - r.Start
		}
		usage += u
		if r.Fee > 0 {
			fee += r.Fee
		} else if u > 0 && r.Price > 0 {
			fee += round2(u * r.Price)
		}
	}
	return round2(usage), round2(fee)
}

// waterRowWithPrice 是 waterPeriodTotals 的输入契约（与 waterRow 解耦，便于单测）。
type waterRowWithPrice struct {
	Name  string
	Usage float64
	Fee   float64
	Price float64
	Start float64
	End   float64
}
