package handler

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// 第 40 轮：分时明细行的「基准电量拆解」过程量。
//
// 恒等式（每个时段）：
//   billing_qty = meter_qty + diff_qty + line_loss_qty + adjust_qty
//
// 2026-10 持睿汽车实测（billing_record_meters 按 period 聚合）：
//   尖峰 7800 + 170.13 + 4.97 − 387.67 = 7587.43
//   峰   47200 + 1735.12 + 19.34 − 904.65 = 48049.81
//   平   56600 + 1970.43 + 28.46 − 791.91 = 57806.98
//   谷   54950 + 1974.31 + 36.20 − 516.98 = 56443.53
//   合计 166550 + 5850(逐段取整 5849.99) + 88.97 − 2601.21 = 169887.75

func breakdownPlan202610() *timedPlan {
	type seg struct {
		p                            string
		usage, loss, adjust, diff     float64
	}
	rows := []seg{
		{"deep", 7800, 4.97, -387.67, 170.13},
		{"peak", 47200, 19.34, -904.65, 1735.12},
		{"flat", 56600, 28.46, -791.91, 1970.43},
		{"valley", 54950, 36.20, -516.98, 1974.31},
	}
	alloc := timedAllocation{
		PeriodBase:      map[string]float64{},
		PeriodBaseExact: map[string]float64{},
		Rows:            map[string][]timedMeterRow{},
	}
	for _, s := range rows {
		alloc.Rows["m"] = append(alloc.Rows["m"], timedMeterRow{
			Period: s.p, Usage: s.usage, Loss: s.loss, Adjust: s.adjust,
			BillKwh: s.usage + s.loss + s.adjust, DiffKwh: s.diff,
			BaseKwh: s.usage + s.loss + s.adjust + s.diff,
			BaseExact: s.usage + s.loss + s.adjust + s.diff,
		})
	}
	return &timedPlan{Alloc: alloc, BillPeriod: map[string]float64{
		"deep": 7587.43, "peak": 48049.81, "flat": 57806.98, "valley": 56443.53,
	}}
}

func TestTimedBreakdownOfPeriod(t *testing.T) {
	plan := breakdownPlan202610()
	want := map[string][4]float64{
		"deep":   {7800, 170.13, 4.97, -387.67},
		"peak":   {47200, 1735.12, 19.34, -904.65},
		"flat":   {56600, 1970.43, 28.46, -791.91},
		"valley": {54950, 1974.31, 36.20, -516.98},
	}
	billingWant := map[string]float64{"deep": 7587.43, "peak": 48049.81, "flat": 57806.98, "valley": 56443.53}
	for _, p := range timedPeriodOrder {
		b := timedBreakdownOf(plan, p)
		closeTo(t, "电表电量 "+p, b.MeterQty, want[p][0], 0.01)
		closeTo(t, "差额电量 "+p, b.DiffQty, want[p][1], 0.01)
		closeTo(t, "线损 "+p, b.LineLossQty, want[p][2], 0.01)
		closeTo(t, "加减 "+p, b.AdjustQty, want[p][3], 0.01)
		closeTo(t, "基准电量 "+p, b.BillingQty, billingWant[p], 0.01)
		// 恒等式：四个分量之和必须严格等于该行展示的「账单原值/计费数量」
		closeTo(t, "拆解恒等式 "+p,
			b.MeterQty+b.DiffQty+b.LineLossQty+b.AdjustQty, b.BillingQty, 0.02)
	}
}

func TestTimedBreakdownTotals(t *testing.T) {
	plan := breakdownPlan202610()
	var meter, diff, loss, adjust, billing float64
	for _, p := range timedPeriodOrder {
		b := timedBreakdownOf(plan, p)
		meter += b.MeterQty
		diff += b.DiffQty
		loss += b.LineLossQty
		adjust += b.AdjustQty
		billing += b.BillingQty
	}
	closeTo(t, "电表电量合计", meter, 166550, 0.01)
	closeTo(t, "差额电量合计", diff, 5850, 0.02)
	closeTo(t, "基准电量合计", billing, 169887.75, 0.02)
	closeTo(t, "合计恒等式", meter+diff+loss+adjust, billing, 0.02)
}

// 系统运行费大类下的子项（含 rate=0 的「辅助服务费用」）必须强制走
// 「工业总电量 × 单价」，绝不掉进「账单原值 × 分摊比例」分支。
func TestBuildFeeItemsSystemRunningForcesQtyRate(t *testing.T) {
	starTotal := 172400.0
	in := &billingInputs{
		StarTotal: starTotal,
		Ratio:     0.27631216,
		Bill: &types.UtilityBillExtractionItem{
			FeeItems: []types.UtilityBillFeeItem{
				// 第 40 轮真机缺陷：这一行 rate=0/fee=0，旧实现整行不生成，
				// 前端展开面板只能按「账单原值 × 分摊比例」展示成 27.63% / 0。
				{Category: "(4)系统运行费", Name: "辅助服务费用", Qty: 614578, Rate: 0, Fee: 0},
				// 同大类有单价的项：仍是「工业总电量 × 单价」
				{Category: "(4)系统运行费", Name: "煤电容量电费", Qty: 614578, Rate: 0.027817, Fee: 17095.72},
			},
		},
		Enabled: map[string]bool{
			"(4)系统运行费|辅助服务费用": true,
			"(4)系统运行费|煤电容量电费": true,
		},
	}
	plan := &timedPlan{Alloc: timedAllocation{
		PeriodBase: map[string]float64{}, PeriodBaseExact: map[string]float64{},
		Rows: map[string][]timedMeterRow{},
	}, BillPeriod: map[string]float64{}}
	got := buildFeeItems(in, plan, "rec", time.Now())

	var aux *types.BillingRecordItem
	for i := range got.Items {
		if got.Items[i].Name == "辅助服务费用" {
			aux = &got.Items[i]
			break
		}
	}
	if aux == nil {
		t.Fatal("辅助服务费用明细行缺失（rate=0 时整行被跳过）")
	}
	closeTo(t, "辅助服务费用计费数量", aux.Qty, starTotal, 0.01)
	closeTo(t, "辅助服务费用单价", aux.Rate, 0, 1e-9)
	closeTo(t, "辅助服务费用金额", aux.Fee, 0, 1e-9)
	if aux.Period != "" {
		t.Fatalf("辅助服务费用应为平段行（period 空），got %q", aux.Period)
	}
	// 强制按度分支不能改变总金额：只有煤电容量电费那一笔进工业电费
	closeTo(t, "工业电费合计", got.IndustrialFee, starTotal*0.027817, 0.01)
}

func TestIsSystemRunningFeeCategory(t *testing.T) {
	for _, s := range []string{"(4)系统运行费", "(4)系统运行费用", "系统运行费"} {
		if !isSystemRunningFeeCategory(s) {
			t.Fatalf("%q 应识别为系统运行费大类", s)
		}
	}
	for _, s := range []string{"(1)市场化购电费", "(6)政府性基金及附加-工商业", "(2)上网环节线损费用"} {
		if isSystemRunningFeeCategory(s) {
			t.Fatalf("%q 不应识别为系统运行费大类", s)
		}
	}
}
