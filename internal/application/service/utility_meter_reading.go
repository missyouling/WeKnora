package service

import (
	"math"

	"github.com/Tencent/WeKnora/internal/types"
)

// NormalizeTimedMeterReadings 为「分时电表」（meter_type=time）补齐起度/止度。
//
// 背景（真机事故）：持睿工业分表 1~3 是分时表，录入时只填了四段（尖峰/峰/平/谷）
// 的上期/本期示数，`start_reading` / `end_reading` 两个字段始终为 0。
// 费用清单的「电费核算明细」表直接读这两列，于是详情抽屉与 PDF 打印预览里
// 工业分表的「起度 / 止度」全是 0（宿舍表是普通表，正常显示）。
//
// 口径（与票面一致，必须满足 (止度 − 起度) × 倍率 = 用电量）：
//   - 起度 = 四段上期示数之和（deep_prev + peak_prev + flat_prev + valley_prev）
//   - 止度 = 四段本期示数之和（deep_curr + peak_curr + flat_curr + valley_curr）
//   - 四段全为 0 时不动（避免把「确实是 0」的占位行写成假读数）
//
// 幂等：已经有非 0 起止度的行不覆盖（用户可能手工填过总表读数）。
// 返回是否有改动，供调用方决定是否落库。
func NormalizeTimedMeterReadings(items []types.UtilityMeterItem, rateOf func(meterID string) float64) bool {
	changed := false
	for i := range items {
		it := &items[i]
		if it.StartReading != 0 || it.EndReading != 0 {
			continue
		}
		start := it.DeepPrev + it.PeakPrev + it.FlatPrev + it.ValleyPrev
		end := it.DeepCurr + it.PeakCurr + it.FlatCurr + it.ValleyCurr
		if start == 0 && end == 0 {
			continue
		}
		it.StartReading = roundUtility2(start)
		it.EndReading = roundUtility2(end)
		changed = true
	}
	return changed
}

// TimedMeterReadingOf 计算分时表的「起度 / 止度」并返回是否可用。
//
// 供**读取路径**使用（不改库，只保证展示正确）：列表接口返回前调用一次，
// 历史记录无需迁移也能立刻正确显示。
func TimedMeterReadingOf(it types.UtilityMeterItem) (start, end float64, ok bool) {
	if it.StartReading != 0 || it.EndReading != 0 {
		return it.StartReading, it.EndReading, true
	}
	start = roundUtility2(it.DeepPrev + it.PeakPrev + it.FlatPrev + it.ValleyPrev)
	end = roundUtility2(it.DeepCurr + it.PeakCurr + it.FlatCurr + it.ValleyCurr)
	if start == 0 && end == 0 {
		return 0, 0, false
	}
	return start, end, true
}

// TimedMeterUsageMatches 校验 (止度 − 起度) × 倍率 是否等于用电量（容差 1 度）。
//
// 用于回填后的自检与单测断言：工业分表 1 的 (22635 − 22188) × 200 = 89,400。
func TimedMeterUsageMatches(start, end, rate, usage float64) bool {
	if rate <= 0 {
		return false
	}
	return math.Abs((end-start)*rate-usage) <= 1
}
