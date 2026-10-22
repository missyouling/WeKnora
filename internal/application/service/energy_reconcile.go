package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/Tencent/WeKnora/internal/types"
	"strings"
)

// 能耗读数反算与用量守卫。
//
// 业务背景：能耗账单的核心是一条**数值连续性主线**——
//
//	实际用量 = (本期读数 − 上期读数) × 倍率
//
// 抄表记录/电费账单上这三个数可能缺一（有的只给「本期用量」不给读数，
// 有的只给读数不给倍率）。本文件把这套口径固化成纯函数：
//   - 能补的补（读数齐 → 反算用量）；
//   - 补不了的记告警（用量与读数推算值对不上 → 交给前端黄色告警面板）；
//   - 绝不臆造（没有读数就不编用量，没有倍率按 1 处理并标注）。
//
// 为什么是纯函数：反算是可单测的数值逻辑，且提取后处理、AI 建议、沙箱测试
// 三处都要复用同一套口径（历史上发票/合同就因为口径分散在多处而漂移）。

// energyReadingTolerance 用量校验的相对容差。
//
// 账单上的抄见电量与「(本期−上期)×倍率」经常有小差：变损/线损/加减项会被
// 并入计费电量，浮点累加也有尾差。1% 容差足够宽松（不会放过真实错误），
// 又足够严格（能抓住「倍率取错 / 读数取错行 / 用量被模型算错」）。
const energyReadingTolerance = 0.01

// energyUsageToleranceAbs 用量校验的绝对容差（元/单位）。
//
// 小用量场景（居民水表可能只有几吨）相对容差会过松，给一个绝对下限：
// 差值在 0.5 以内直接视为相等。
const energyUsageToleranceAbs = 0.5

// EnergyReading 一条主表读数（账期台账口径）。
type EnergyReading struct {
	PreviousReading float64
	CurrentReading  float64
	Multiplier      float64
	UsageAmount     float64
	// HasPrevious / HasCurrent / HasMultiplier / HasUsage 标记「该字段有值」。
	// float64 的 0 是合法读数（本期读数为 0 说明表已归零），不能用零值区分
	// 「没提取到」与「确实是 0」。
	HasPrevious   bool
	HasCurrent    bool
	HasMultiplier bool
	HasUsage      bool
}

// EnergyReconcileResult 反算结果。
type EnergyReconcileResult struct {
	// Reading 反算并补齐后的读数（指针字段的 Has* 标记补齐情况）
	Reading EnergyReading
	// ComputedUsage 由读数推算出的用量（读数不齐时为 0 且 Computed=false）
	ComputedUsage float64
	// Computed 标记 ComputedUsage 是否有效
	Computed bool
	// UsagePatched 标记 UsageAmount 是否被自动补齐（原本缺用量，现在有了）
	UsagePatched bool
	// Issues 用量校验发现的问题
	Issues []types.EnergyUsageIssue
}

// ReconcileEnergyReading 读数反算与用量守卫（核心纯函数）。
//
// 补齐策略（只补不缺，缺证据绝不臆造）：
//  1. 有上期 + 本期读数 → 推算 computedUsage = (本期 − 上期) × 倍率；
//     倍率缺省为 1（单表计场景）。
//  2. 缺用量但能推算 → 补齐 UsageAmount（UsagePatched=true）。
//  3. 有用量也有推算值 → 交叉校验，超容差记 usage_mismatch 告警，**不改用量**
//     （账单值优先：变损/线损会让推算值天然偏小）。
//  4. 有读数无用量且本期 < 上期 → 疑似换表/回抄，记 reading_regressed 告警，
//     不补用量（负用量是脏数据）。
//  5. 无读数但有用量 → 记 usage_missing 告警（无法交叉验证），不补读数。
//  6. 读数与用量全缺 → 记 reading_missing 告警。
func ReconcileEnergyReading(r EnergyReading) EnergyReconcileResult {
	out := EnergyReconcileResult{Reading: r}

	multiplier := r.Multiplier
	if !r.HasMultiplier {
		// 单表计无倍率，缺省 1
		multiplier = 1
	}
	// 倍率为 0 或负数是脏数据，按 1 处理并告警。
	// 告警必须在这里发出：后面的「缺用量 → 补齐」分支会提前 return，
	// 若把告警放在校验段，脏倍率 + 缺用量的组合就会静默通过。
	multiplierInvalid := r.HasMultiplier && (r.Multiplier <= 0)
	if multiplierInvalid {
		multiplier = 1
		out.Issues = append(out.Issues, types.EnergyUsageIssue{
			Code:     types.EnergyIssueUsageMismatch,
			Message:  "倍率为 0 或负数（脏数据），已按倍率 1 参与推算",
			Expected: -1,
			Actual:   -1,
		})
	}

	if !r.HasPrevious || !r.HasCurrent {
		// 读数不齐：无法推算，也无法交叉校验
		if r.HasUsage {
			out.Issues = append(out.Issues, types.EnergyUsageIssue{
				Code:     types.EnergyIssueUsageMissing,
				Message:  "缺少上期/本期读数，无法交叉校验实际用量",
				Expected: -1,
				Actual:   r.UsageAmount,
			})
		} else {
			out.Issues = append(out.Issues, types.EnergyUsageIssue{
				Code:     types.EnergyIssueReadingMissing,
				Message:  "缺少上期读数、本期读数与实际用量，请核对抄表记录",
				Expected: -1,
				Actual:   -1,
			})
		}
		return out
	}

	diff := r.CurrentReading - r.PreviousReading
	if diff < 0 {
		// 本期读数 < 上期读数：换表/回抄/提取串行。此处补出来的用量是负数，
		// 属于脏数据，绝不写入，只记告警。
		out.Issues = append(out.Issues, types.EnergyUsageIssue{
			Code: types.EnergyIssueReadingRegressed,
			Message: fmt.Sprintf(
				"本期读数（%g）小于上期读数（%g），疑似更换计量表或提取串行，请核对抄表记录",
				r.CurrentReading, r.PreviousReading),
			Expected: -1,
			Actual:   -1,
		})
		if !r.HasUsage {
			out.Issues = append(out.Issues, types.EnergyUsageIssue{
				Code:     types.EnergyIssueReadingMissing,
				Message:  "读数异常且缺少实际用量，未做自动补齐",
				Expected: -1,
				Actual:   -1,
			})
		}
		return out
	}

	computed := diff * multiplier
	// 浮点尾差修正：账单数字通常是两位小数，乘法后可能出 1e-9 级误差。
	computed = math.Round(computed*1e6) / 1e6
	out.ComputedUsage = computed
	out.Computed = true

	if !r.HasUsage {
		// 缺用量 → 补齐
		out.Reading.UsageAmount = computed
		out.Reading.HasUsage = true
		out.UsagePatched = true
		return out
	}

	// 有用量 → 交叉校验
	if !energyUsageEqual(r.UsageAmount, computed) {
		out.Issues = append(out.Issues, types.EnergyUsageIssue{
			Code: types.EnergyIssueUsageMismatch,
			Message: fmt.Sprintf(
				"实际用量（%g）与读数推算值（%g = (%g − %g) × %g）偏差超限，请核对倍率或抄表读数",
				r.UsageAmount, computed, r.CurrentReading, r.PreviousReading, multiplier),
			Expected: computed,
			Actual:   r.UsageAmount,
		})
	}
	return out
}

// energyUsageEqual 用量相等判定（相对容差 + 绝对容差）。
func energyUsageEqual(a, b float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	if diff <= energyUsageToleranceAbs {
		return true
	}
	base := math.Max(math.Abs(a), math.Abs(b))
	if base == 0 {
		return false
	}
	return diff/base <= energyReadingTolerance
}

// energyRecordFromMap 从模型输出的 map 构造记录并做读数反算。
//
// 模型的输出是 map（generic document 管线），数值字段可能是 json.Number /
// float64 / 字符串（"595860" / "595,860"）等多种形态，必须宽松解析。
// 这也是为什么不能直接用结构体接收。
func energyRecordFromMap(m map[string]any) types.EnergyRecord {
	rec := types.EnergyRecord{
		UtilityNo:     energyString(m, "utility_no"),
		BillingPeriod: energyString(m, "billing_period"),
		UtilityType:   energyString(m, "utility_type"),
		AccountName:   energyString(m, "account_name"),
		MeterNo:       energyString(m, "meter_no"),
		Address:       energyString(m, "address"),
		PeriodStart:   energyString(m, "period_start"),
		PeriodEnd:     energyString(m, "period_end"),
		ReadDate:      energyString(m, "read_date"),
		UsageCategory: energyString(m, "usage_category"),
		VoltageLevel:  energyString(m, "voltage_level"),
		Remark:        energyString(m, "remark"),
	}

	prev, hasPrev := energyNumber(m, "previous_reading")
	curr, hasCurr := energyNumber(m, "current_reading")
	mult, hasMult := energyNumber(m, "multiplier")
	usage, hasUsage := energyNumber(m, "usage_amount")
	price, hasPrice := energyNumber(m, "unit_price")
	total, hasTotal := energyNumber(m, "total_amount")

	// 记录级读数反算
	res := ReconcileEnergyReading(EnergyReading{
		PreviousReading: prev,
		CurrentReading:  curr,
		Multiplier:      mult,
		UsageAmount:     usage,
		HasPrevious:     hasPrev,
		HasCurrent:      hasCurr,
		HasMultiplier:   hasMult,
		HasUsage:        hasUsage,
	})
	if hasPrev {
		rec.PreviousReading = &prev
	}
	if hasCurr {
		rec.CurrentReading = &curr
	}
	if hasMult {
		rec.Multiplier = &mult
	}
	if res.Reading.HasUsage {
		u := res.Reading.UsageAmount
		rec.UsageAmount = &u
	}
	if hasPrice {
		rec.UnitPrice = &price
	}
	if hasTotal {
		rec.TotalAmount = &total
	}
	rec.UsagePatched = res.UsagePatched
	rec.Issues = append(rec.Issues, res.Issues...)

	// 明细（多表位读数）
	rec.Items = energyItemsFromMap(m)

	return rec
}

// energyItemsFromMap 从 map 里取 items[] 明细行。
func energyItemsFromMap(m map[string]any) []types.EnergyMeterItem {
	raw, ok := m["items"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]types.EnergyMeterItem, 0, len(arr))
	for _, it := range arr {
		im, ok := it.(map[string]any)
		if !ok {
			continue
		}
		item := types.EnergyMeterItem{
			MeterNo:       energyString(im, "meter_no"),
			MeterType:     energyString(im, "meter_type"),
			BillingPeriod: energyString(im, "billing_period"),
			Remarks:       energyString(im, "remarks"),
		}
		prev, hasPrev := energyNumber(im, "previous_reading")
		curr, hasCurr := energyNumber(im, "current_reading")
		mult, hasMult := energyNumber(im, "multiplier")
		usage, hasUsage := energyNumber(im, "usage_amount")

		res := ReconcileEnergyReading(EnergyReading{
			PreviousReading: prev,
			CurrentReading:  curr,
			Multiplier:      mult,
			UsageAmount:     usage,
			HasPrevious:     hasPrev,
			HasCurrent:      hasCurr,
			HasMultiplier:   hasMult,
			HasUsage:        hasUsage,
		})
		if hasPrev {
			item.PreviousReading = &prev
		}
		if hasCurr {
			item.CurrentReading = &curr
		}
		if hasMult {
			item.Multiplier = &mult
		}
		if res.Reading.HasUsage {
			u := res.Reading.UsageAmount
			item.UsageAmount = &u
		}
		if res.Computed {
			c := res.ComputedUsage
			item.ComputedUsage = &c
		}
		for _, iss := range res.Issues {
			if iss.Code == types.EnergyIssueUsageMismatch {
				item.UsageMismatch = true
			}
		}
		out = append(out, item)
	}
	return out
}

// energyString 从 map 取字符串（数值型也转成字符串）。
func energyString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	case bool:
		if t {
			return "true"
		}
		return ""
	default:
		return ""
	}
}

// energyNumber 从 map 取数值，返回值与「是否有值」标记。
//
// 容忍字符串形态（"595860" / "595,860" / "3825.28"）——模型常把数字输出成字符串。
func energyNumber(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return 0, false
		}
		s = strings.ReplaceAll(s, ",", "")
		s = strings.ReplaceAll(s, "，", "")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
