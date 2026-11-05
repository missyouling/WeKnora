package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 真机数据（2026-10 持睿工业分表 1）：
//   尖峰 5525→5634、峰 7411→7565、平 7393→7538、谷 1859→1898，倍率 200，用电量 89,400。
//   起度 = 5525+7411+7393+1859 = 22,188
//   止度 = 5634+7565+7538+1898 = 22,635
//   (22635 − 22188) × 200 = 89,400 ✓
func timedMeterItem1() types.UtilityMeterItem {
	return types.UtilityMeterItem{
		MeterName: "持睿工业分表1",
		Rate:      200,
		Usage:     89400,
		DeepPrev:  1859, DeepCurr: 1898,
		PeakPrev:  5525, PeakCurr: 5634,
		FlatPrev:  7411, FlatCurr: 7565,
		ValleyPrev: 7393, ValleyCurr: 7538,
	}
}

func TestTimedMeterReadingOfSumsFourPeriods(t *testing.T) {
	it := timedMeterItem1()
	start, end, ok := TimedMeterReadingOf(it)
	if !ok {
		t.Fatal("分时表应能算出起止度")
	}
	if start != 22188 || end != 22635 {
		t.Fatalf("起止度算错: start=%.0f end=%.0f（期望 22188 / 22635）", start, end)
	}
	if !TimedMeterUsageMatches(start, end, it.Rate, it.Usage) {
		t.Fatalf("(止度−起度)×倍率 应等于用电量：(%.0f−%.0f)×%.0f != %.0f",
			end, start, it.Rate, it.Usage)
	}
}

func TestTimedMeterReadingOfKeepsExplicitReadings(t *testing.T) {
	// 已有总表读数时不覆盖（用户可能手工填了总表起止度）
	it := timedMeterItem1()
	it.StartReading = 1000
	it.EndReading = 1447
	start, end, ok := TimedMeterReadingOf(it)
	if !ok || start != 1000 || end != 1447 {
		t.Fatalf("显式起止度应原样返回，got %.0f / %.0f", start, end)
	}
}

func TestTimedMeterReadingOfAllZero(t *testing.T) {
	// 四段全 0 → 视为没有读数，返回 ok=false（不能凭空造 0/0 的假读数）
	it := types.UtilityMeterItem{MeterName: "空表", Rate: 200}
	if _, _, ok := TimedMeterReadingOf(it); ok {
		t.Fatal("四段全 0 时不应返回可用起止度")
	}
}

func TestNormalizeTimedMeterReadingsFillsAndIsIdempotent(t *testing.T) {
	items := []types.UtilityMeterItem{
		{ID: "a", DeepPrev: 1859, DeepCurr: 1898, PeakPrev: 5525, PeakCurr: 5634,
			FlatPrev: 7411, FlatCurr: 7565, ValleyPrev: 7393, ValleyCurr: 7538},
		// 普通表（宿舍）：本来就有起止度，不能被改
		{ID: "b", StartReading: 8293, EndReading: 8357, Usage: 64},
	}
	if !NormalizeTimedMeterReadings(items, nil) {
		t.Fatal("应报告有改动")
	}
	if items[0].StartReading != 22188 || items[0].EndReading != 22635 {
		t.Fatalf("分时表起止度未补齐: %.0f / %.0f", items[0].StartReading, items[0].EndReading)
	}
	if items[1].StartReading != 8293 || items[1].EndReading != 8357 {
		t.Fatalf("普通表起止度被改动: %.0f / %.0f", items[1].StartReading, items[1].EndReading)
	}
	// 幂等：第二次调用不应再有改动
	if NormalizeTimedMeterReadings(items, nil) {
		t.Fatal("第二次调用不应报告改动（幂等）")
	}
}
