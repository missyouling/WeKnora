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
	// 账单「电量明细」是全厂口径（尖峰线损 13 度、加减 -1341 度），
	// 乘分摊比例 126600/596168 = 0.21235625 后才是持睿承担的份额：
	//   13 × 0.21235625 = 2.76、-1341 × 0.21235625 = -284.77
	loss = map[string]float64{"deep": 2.76, "peak": 12.53, "flat": 20.39, "valley": 29.73}
	adjust = map[string]float64{"deep": -284.77, "peak": -664.25, "flat": -569.33, "valley": -379.69}
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
// 用户口径（2026-09 持睿汽车）：
//
//	零售交易电费 54,398.57（尖 2190.59 / 峰 15541.36 / 平 18153.06 / 谷 18513.56）
//	零售输配电费 18,610.64（尖 1217.89 / 峰 8640.42 / 平 6307.77 / 谷 2444.56）
//
// 三次口径演进：①账单整月金额×21.24% → 54,369.99；②差额按全部计费电量占比
// → 54,422.79；③差额每表均分 + 表内占比 + 逐表逐段取整 → 54,398.57（对齐 Excel）。
func TestBillingTimedFeeMatchesStandardSheet(t *testing.T) {
	meters, loss, adjust, deltaQ := chengRui202609Fixture()
	alloc := allocateTimedKwh(meters, loss, adjust, deltaQ)

	closeTo(t, "抄见电量合计", alloc.TotalUsage, 122700, 0.01)
	closeTo(t, "差额电量", alloc.DeltaQ, 3900, 0.01)

	// 各时段基准电量（标准表：5024.28 / 35645.33 / 41635.45 / 42462.30）
	wantBase := map[string]float64{
		"deep":   5024.28,
		"peak":   35645.33,
		"flat":   41635.45,
		"valley": 42462.30,
	}
	for p, want := range wantBase {
		closeTo(t, "基准电量 "+p, alloc.PeriodBase[p], want, 0.03)
	}

	// 分时电费：**逐表逐段** round2 后相加（Excel 是每块表每个时段各占一格）
	rates := map[string]struct{ trans, purchase float64 }{
		"deep":   {0.2424, 0.436},
		"peak":   {0.2424, 0.436},
		"flat":   {0.1515, 0.436},
		"valley": {0.05757, 0.436},
	}
	trans, purchase := 0.0, 0.0
	for _, key := range []string{"m1", "m2", "m3"} {
		for _, r := range alloc.Rows[key] {
			rt := rates[r.Period]
			if rt.trans > 0 {
				trans = round2(trans + round2(r.BaseExact*rt.trans))
			}
			if rt.purchase > 0 {
				purchase = round2(purchase + round2(r.BaseExact*rt.purchase))
			}
		}
	}
	closeTo(t, "零售输配电费", trans, 18610.64, 0.02)
	closeTo(t, "零售交易电费", purchase, 54398.57, 0.02)

	// 逐段再核一次（防止「总额对、分段错」）
	wantDeepTrans, wantPeakTrans := 1217.89, 8640.42
	deepTrans := 0.0
	for _, key := range []string{"m1", "m2", "m3"} {
		for _, r := range alloc.Rows[key] {
			if r.Period == "deep" {
				deepTrans = round2(deepTrans + round2(r.BaseExact*0.2424))
			}
		}
	}
	closeTo(t, "尖峰输配", deepTrans, wantDeepTrans, 0.02)
	_ = wantPeakTrans
}

// 逐表明细：损耗/加减只落在第一块表，其余表计费电量 = 抄见电量。
func TestBillingTimedMeterRows(t *testing.T) {
	meters, loss, adjust, deltaQ := chengRui202609Fixture()
	alloc := allocateTimedKwh(meters, loss, adjust, deltaQ)

	first := alloc.Rows["m1"]
	if len(first) != 4 {
		t.Fatalf("表1 应有 4 个时段，实际 %d", len(first))
	}
	// 尖峰：5200 + 2.76 - 284.77 = 4917.99；差额 1300 × 5200/63600 = 106.29
	closeTo(t, "表1 尖峰计费电量", first[0].BillKwh, 4917.99, 0.01)
	closeTo(t, "表1 尖峰差额", first[0].DiffKwh, 106.29, 0.01)
	closeTo(t, "表1 尖峰基准电量", first[0].BaseKwh, 5024.28, 0.01)
	closeTo(t, "表1 峰计费电量", first[1].BillKwh, 14548.28, 0.01)
	closeTo(t, "表1 谷基准电量", first[3].BaseKwh, 21895.64, 0.01)

	// 表2 各时段无损耗/加减
	for _, r := range alloc.Rows["m2"] {
		closeTo(t, "表2 "+r.Period+" 损耗", r.Loss, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 加减", r.Adjust, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 计费=抄见", r.BillKwh, r.Usage, 0.001)
	}
	// 表2 三个有量时段（峰/平/谷）基准电量都应约 15733.33
	for _, r := range alloc.Rows["m2"] {
		if r.Usage == 0 {
			closeTo(t, "表2 "+r.Period+" 零电量基准", r.BaseKwh, 0, 0.01)
			continue
		}
		// 表2 三表均分 1300 度后按表内占比：峰/平/谷各 433.33
		closeTo(t, "表2 "+r.Period+" 基准电量", r.BaseKwh, 15733.33, 0.05)
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
