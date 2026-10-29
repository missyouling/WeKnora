package handler

import (
	"math"
	"testing"
)

// 2026-09 持睿汽车标准核算表（用户提供的 Excel 核算口径）。
//
// 三块分时表、四段读数；账单「电量明细（工商业）」的变损+线损与加减电量；
// 星达配电分表总电量 126,600 度。
func chengRui202609Fixture() (meters []timedMeterUsage, loss, adjust map[string]float64, deltaQ float64) {
	read := func(prev, curr, rate float64) float64 { return (curr - prev) * rate }
	meters = []timedMeterUsage{
		{Key: "m1", Name: "持睿工业分表1", Rate: 200, Usage: map[string]float64{
			"deep":   read(1833, 1859, 200),
			"peak":   read(5449, 5525, 200),
			"flat":   read(7304, 7411, 200),
			"valley": read(7284, 7393, 200),
		}},
		{Key: "m2", Name: "持睿工业分表2", Rate: 150, Usage: map[string]float64{
			"deep":   read(100, 100, 150),
			"peak":   read(946, 1048, 150),
			"flat":   read(1028, 1130, 150),
			"valley": read(1002, 1104, 150),
		}},
		{Key: "m3", Name: "持睿工业分表3", Rate: 200, Usage: map[string]float64{
			"deep":   read(70, 70, 200),
			"peak":   read(612, 635, 200),
			"flat":   read(683, 704, 200),
			"valley": read(673, 695, 200),
		}},
	}
	loss = map[string]float64{"deep": 2.68, "peak": 12.15, "flat": 19.77, "valley": 28.83}
	adjust = map[string]float64{"deep": -276.14, "peak": -644.12, "flat": -552.07, "valley": -368.19}
	// 星达配电分表 126,600 - 三表合计 122,700
	return meters, loss, adjust, 3900
}

func closeTo(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.4f, 期望 %.4f（容差 %.4f）", name, got, want, tol)
	}
}

// 分时电费（零售交易电费/零售输配电费）必须与标准核算表逐分对上。
//
// 用户口径：零售交易电费 54,422.79；零售输配电费 18,616.22。
// 旧实现把市电账单整月金额 × 分摊比例（21.24%）当作分时电费，
// 算出 54,369.99 / 16,204.46，与核算表对不上。
func TestBillingTimedFeeMatchesStandardSheet(t *testing.T) {
	meters, loss, adjust, deltaQ := chengRui202609Fixture()
	alloc := allocateTimedKwh(meters, loss, adjust, deltaQ)

	closeTo(t, "抄见电量合计", alloc.TotalUsage, 122700, 0.01)
	closeTo(t, "计费电量合计", alloc.TotalBill, 120922.91, 0.01)
	closeTo(t, "差额电量", alloc.DeltaQ, 3900, 0.01)

	// 各时段基准电量（明细表逐表累加）
	wantBase := map[string]float64{
		"deep":   5085.43,  // 表1 尖峰
		"peak":   15037.88 + 15793.45 + 4748.36,
		"flat":   21540.72 + 15793.45 + 4335.46,
		"valley": 22152.79 + 15793.45 + 4541.91,
	}
	for p, want := range wantBase {
		closeTo(t, "基准电量 "+p, alloc.PeriodBase[p], want, 0.03)
	}

	// 分时电费：Σ 基准电量 × 时段单价
	rates := map[string]struct{ trans, purchase float64 }{
		"deep":   {0.2424, 0.436},
		"peak":   {0.2424, 0.436},
		"flat":   {0.1515, 0.436},
		"valley": {0.05757, 0.436},
	}
	// 用**未取整**的时段基准电量乘单价（与生成账单同口径），再按段取整累加
	trans, purchase := 0.0, 0.0
	for _, p := range timedPeriodOrder {
		trans = round2(trans + round2(alloc.PeriodBaseExact[p]*rates[p].trans))
		purchase = round2(purchase + round2(alloc.PeriodBaseExact[p]*rates[p].purchase))
	}
	closeTo(t, "零售输配电费", trans, 18616.22, 0.02)
	closeTo(t, "零售交易电费", purchase, 54422.79, 0.02)
}

// 逐表明细：损耗/加减只落在第一块表，其余表计费电量 = 抄见电量。
func TestBillingTimedMeterRows(t *testing.T) {
	meters, loss, adjust, deltaQ := chengRui202609Fixture()
	alloc := allocateTimedKwh(meters, loss, adjust, deltaQ)

	first := alloc.Rows["m1"]
	if len(first) != 4 {
		t.Fatalf("表1 应有 4 个时段，实际 %d", len(first))
	}
	closeTo(t, "表1 尖峰计费电量", first[0].BillKwh, 4926.54, 0.01)
	closeTo(t, "表1 尖峰差额", first[0].DiffKwh, 158.89, 0.01)
	closeTo(t, "表1 尖峰基准电量", first[0].BaseKwh, 5085.43, 0.01)
	closeTo(t, "表1 峰计费电量", first[1].BillKwh, 14568.03, 0.01)
	closeTo(t, "表1 谷基准电量", first[3].BaseKwh, 22152.79, 0.01)

	// 表2 各时段无损耗/加减
	for _, r := range alloc.Rows["m2"] {
		closeTo(t, "表2 "+r.Period+" 损耗", r.Loss, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 加减", r.Adjust, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 计费=抄见", r.BillKwh, r.Usage, 0.001)
	}
	// 表2 三个有量时段（峰/平/谷）基准电量都应约 15793.45
	for _, r := range alloc.Rows["m2"] {
		if r.Usage == 0 {
			closeTo(t, "表2 "+r.Period+" 零电量基准", r.BaseKwh, 0, 0.01)
			continue
		}
		closeTo(t, "表2 "+r.Period+" 基准电量", r.BaseKwh, 15793.45, 0.05)
	}
}

// 边界：没有任何分表读数 / 差额为 0 时不得除零、不得产生 NaN。
func TestBillingTimedAllocationEdgeCases(t *testing.T) {
	alloc := allocateTimedKwh(nil, nil, nil, 0)
	closeTo(t, "空输入计费合计", alloc.TotalBill, 0, 0.001)
	closeTo(t, "空输入差额", alloc.DeltaQ, 0, 0.001)

	meters, loss, adjust, _ := chengRui202609Fixture()
	zero := allocateTimedKwh(meters, loss, adjust, 0)
	for _, p := range timedPeriodOrder {
		if math.IsNaN(zero.PeriodBase[p]) || math.IsInf(zero.PeriodBase[p], 0) {
			t.Fatalf("差额为 0 时 %s 基准电量异常：%v", p, zero.PeriodBase[p])
		}
	}
	// 无损耗无加减时，计费电量合计 = 抄见电量合计
	plain := allocateTimedKwh(meters, map[string]float64{}, map[string]float64{}, 0)
	closeTo(t, "无损耗计费合计", plain.TotalBill, plain.TotalUsage, 0.01)
}
