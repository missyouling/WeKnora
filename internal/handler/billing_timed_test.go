package handler

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

// 2026-09 持睿汽车标准核算表（用户提供的 Excel 核算口径）。
//
// **基准数据不在本文件里**，而是与前端 `billingExcelParity.test.ts` 共读
// `testdata/billing-golden-2026-09.json`（第 22 轮 P0-4）。
//
// 为什么必须共享：这套口径是业务的唯一验收基准。前后端各自维护一份 fixture 时，
// 改了一边忘了另一边，单测全绿但线上对不上账 —— 第 20 轮就发生过（前端按新口径
// 算出 54,398.57，Go 侧断言仍写着 54,422.79）。共享文件后，任何口径改动都必须
// 让两边同时变绿，改不动就是口径没对齐。

// goldenFile testdata/billing-golden-2026-09.json 的结构子集。
type goldenFile struct {
	Month      string `json:"month"`
	TenantName string `json:"tenantName"`
	Inputs     struct {
		BillTotalKwh   float64 `json:"billTotalKwh"`
		StarTotalKwh   float64 `json:"starTotalKwh"`
		DeltaQ         float64 `json:"deltaQ"`
		CapacityFee    float64 `json:"capacityFee"`
		PfAdjustAmount float64 `json:"pfAdjustAmount"`
		DormKwh        float64 `json:"dormKwh"`
		DormFee        float64 `json:"dormFee"`
		WaterFee       float64 `json:"waterFee"`
		TimedMeters    []struct {
			Name  string             `json:"name"`
			Usage map[string]float64 `json:"usage"`
		} `json:"timedMeters"`
		MeterReadings []struct {
			MeterType string  `json:"meter_type"`
			TransLoss float64 `json:"trans_loss"`
			LineLoss  float64 `json:"line_loss"`
			Adjust    float64 `json:"adjust"`
		} `json:"meterReadings"`
		FeeItems []struct {
			Name     string  `json:"name"`
			Period   string  `json:"period"`
			Qty      float64 `json:"qty"`
			Rate     float64 `json:"rate"`
			Fee      float64 `json:"fee"`
			Category string  `json:"category"`
		} `json:"feeItems"`
		PeriodRates map[string]struct {
			Trade        float64 `json:"trade"`
			Transmission float64 `json:"transmission"`
		} `json:"periodRates"`
	} `json:"inputs"`
	Expected struct {
		Ratio          float64            `json:"ratio"`
		LossByPeriod   map[string]float64 `json:"lossByPeriod"`
		AdjustByPeriod map[string]float64 `json:"adjustByPeriod"`
		BaseKwh        map[string]float64 `json:"baseKwh"`
		BaseKwhTotal   float64            `json:"baseKwhTotal"`
		Trade          struct {
			Periods map[string]float64 `json:"periods"`
			Total   float64            `json:"total"`
		} `json:"trade"`
		Transmission struct {
			Periods map[string]float64 `json:"periods"`
			Total   float64            `json:"total"`
		} `json:"transmission"`
		PlainFees []struct {
			Name  string  `json:"name"`
			Group string  `json:"group"`
			Qty   float64 `json:"qty"`
			Rate  float64 `json:"rate"`
			Fee   float64 `json:"fee"`
		} `json:"plainFees"`
		Summary map[string]float64 `json:"summary"`
	} `json:"expected"`
}

var cachedGolden *goldenFile

// loadGolden 读取共享黄金基准（go test 的 cwd 是 internal/handler）。
func loadGolden(t *testing.T) *goldenFile {
	t.Helper()
	if cachedGolden != nil {
		return cachedGolden
	}
	path := filepath.Join("..", "..", "testdata", "billing-golden-2026-09.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取黄金基准失败（%s）：%v", path, err)
	}
	var g goldenFile
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("解析黄金基准失败：%v", err)
	}
	cachedGolden = &g
	return cachedGolden
}

// goldenInputs 用黄金基准构造一份「与线上查库结果等价」的输入。
//
// 分时表读数用 Prev=0 / Curr=usage / Rate=1 反构，这样 allocateTimed 里
// `round2((Curr-Prev)*Rate)` 恰好还原抄见电量，能真正走一遍生产代码路径。
func goldenInputs(t *testing.T) *billingInputs {
	t.Helper()
	g := loadGolden(t)
	ratio := round8(g.Inputs.StarTotalKwh / g.Inputs.BillTotalKwh)
	if math.Abs(ratio-g.Expected.Ratio) > 1e-9 {
		t.Fatalf("ratio = %.10f，基准要求 %.10f", ratio, g.Expected.Ratio)
	}

	subRows := make([]subMeterRow, 0, len(g.Inputs.TimedMeters))
	for i, m := range g.Inputs.TimedMeters {
		periods := make(map[string]struct{ Prev, Curr float64 }, 4)
		for p, u := range m.Usage {
			periods[p] = struct{ Prev, Curr float64 }{Prev: 0, Curr: u}
		}
		subRows = append(subRows, subMeterRow{
			MeterID: fmt.Sprintf("m%d", i+1),
			Name:    m.Name,
			Rate:    1,
			Periods: periods,
		})
	}

	bill := &types.UtilityBillExtractionItem{
		TotalKwh:       g.Inputs.BillTotalKwh,
		CapacityFee:    g.Inputs.CapacityFee,
		PfAdjustAmount: g.Inputs.PfAdjustAmount,
	}
	for _, mr := range g.Inputs.MeterReadings {
		bill.MeterReadings = append(bill.MeterReadings, types.UtilityMeterReading{
			MeterType: mr.MeterType,
			TransLoss: mr.TransLoss,
			LineLoss:  mr.LineLoss,
			Adjust:    mr.Adjust,
		})
	}
	for _, f := range g.Inputs.FeeItems {
		bill.FeeItems = append(bill.FeeItems, types.UtilityBillFeeItem{
			Category: f.Category, Name: f.Name, Period: f.Period,
			Qty: f.Qty, Rate: f.Rate, Fee: f.Fee,
		})
	}

	loss, adjust := billLossAdjust(bill, ratio)
	return &billingInputs{
		TenantID:   "golden",
		TenantName: g.TenantName,
		Month:      g.Month,
		Setting:    types.BillingTenantSetting{DormPrice: 1, WaterPrice: 5.22},
		StarTotal:  g.Inputs.StarTotalKwh,
		SubTotal:   g.Inputs.StarTotalKwh - g.Inputs.DeltaQ,
		LineLoss:   round2(g.Inputs.DeltaQ),
		SubRows:    subRows,
		Bill:       bill,
		Ratio:      ratio,
		BillLoss:   loss,
		BillAdjust: adjust,
		DormKwh:    g.Inputs.DormKwh,
		DormFee:    g.Inputs.DormFee,
		WaterFee:   g.Inputs.WaterFee,
		// 力调开关在库里是「功率因数」（账单原文），代码历史上写「因素」，两种都要认
		Enabled: goldenEnabled(bill),
	}
}

func closeTo(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s = %.4f, 期望 %.4f（容差 %.4f）", name, got, want, tol)
	}
}

// 分时电量分摊：损耗/加减折算、四段基准电量、逐表计费电量。
func TestBillingTimedFeeMatchesStandardSheet(t *testing.T) {
	g := loadGolden(t)
	in := goldenInputs(t)

	for _, p := range timedPeriodOrder {
		closeTo(t, "损耗 "+p, in.BillLoss[p], g.Expected.LossByPeriod[p], 0.01)
		closeTo(t, "加减 "+p, in.BillAdjust[p], g.Expected.AdjustByPeriod[p], 0.01)
	}

	plan := allocateTimed(in, "rec", time.Now())
	closeTo(t, "抄见电量合计", plan.Alloc.TotalUsage, in.SubTotal, 0.01)
	closeTo(t, "差额电量", plan.Alloc.DeltaQ, in.LineLoss, 0.01)

	total := 0.0
	for _, p := range timedPeriodOrder {
		closeTo(t, "基准电量 "+p, plan.Alloc.PeriodBase[p], g.Expected.BaseKwh[p], 0.03)
		total += plan.Alloc.PeriodBase[p]
	}
	closeTo(t, "四段合计", round2(total), g.Expected.BaseKwhTotal, 0.02)

	// 电量明细卡片行：每块表 4 个时段，合计与 Allocation 同源
	if len(plan.MeterRows) != len(in.SubRows)*4 {
		t.Fatalf("明细行数 = %d，期望 %d", len(plan.MeterRows), len(in.SubRows)*4)
	}
}

// 逐表明细：损耗/加减只落在第一块表，其余表计费电量 = 抄见电量。
func TestBillingTimedMeterRows(t *testing.T) {
	in := goldenInputs(t)
	plan := allocateTimed(in, "rec", time.Now())

	first := plan.Alloc.Rows["m1"]
	if len(first) != 4 {
		t.Fatalf("表1 应有 4 个时段，实际 %d", len(first))
	}
	// 尖峰：5200 + 2.76 - 284.77 = 4917.99；差额 1300 × 5200/63600 = 106.29
	closeTo(t, "表1 尖峰计费电量", first[0].BillKwh, 4917.99, 0.01)
	closeTo(t, "表1 尖峰差额", first[0].DiffKwh, 106.29, 0.01)
	closeTo(t, "表1 尖峰基准电量", first[0].BaseKwh, 5024.28, 0.01)
	closeTo(t, "表1 峰计费电量", first[1].BillKwh, 14548.28, 0.01)
	closeTo(t, "表1 谷基准电量", first[3].BaseKwh, 21895.64, 0.01)

	for _, r := range plan.Alloc.Rows["m2"] {
		closeTo(t, "表2 "+r.Period+" 损耗", r.Loss, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 加减", r.Adjust, 0, 0.001)
		closeTo(t, "表2 "+r.Period+" 计费=抄见", r.BillKwh, r.Usage, 0.001)
	}
	// 表2 三表均分 1300 度后按表内占比：峰/平/谷各 433.33，基准 15733.33
	for _, r := range plan.Alloc.Rows["m2"] {
		if r.Usage == 0 {
			closeTo(t, "表2 "+r.Period+" 零电量基准", r.BaseKwh, 0, 0.01)
			continue
		}
		closeTo(t, "表2 "+r.Period+" 基准电量", r.BaseKwh, 15733.33, 0.05)
	}
}

// 费用明细：逐项断言到「分」，与 Excel 核算表逐格一致。
//
// 这是 P0-3 拆分带来的直接收益 —— 拆分前 buildFeeItems 埋在 430 行巨函数里，
// 只能起服务 + 真机对账；现在它是纯函数，可以在这里一次断言 20+ 项。
func TestBuildFeeItemsMatchesGolden(t *testing.T) {
	g := loadGolden(t)
	in := goldenInputs(t)
	plan := allocateTimed(in, "rec", time.Now())
	got := buildFeeItems(in, plan, "rec", time.Now())

	if len(got.Items) == 0 {
		t.Fatal("费用明细为空")
	}

	// 建立「子项名 + 时段」索引；平段项时段为空串
	byKey := make(map[string]float64, len(got.Items))
	byName := make(map[string][]float64, len(got.Items))
	for _, it := range got.Items {
		byKey[it.Name+"|"+it.Period] = it.Fee
		byName[it.Name] = append(byName[it.Name], it.Fee)
	}

	// ① 分时项：逐段金额
	for label, want := range g.Expected.Trade.Periods {
		if v := byKey["零售交易电费|"+label]; v != want {
			t.Fatalf("零售交易电费 %s = %.2f，期望 %.2f", label, v, want)
		}
	}
	for label, want := range g.Expected.Transmission.Periods {
		if v := byKey["零售输配电费|"+label]; v != want {
			t.Fatalf("零售输配电费 %s = %.2f，期望 %.2f", label, v, want)
		}
	}

	// ② 平段按度项：计费数量必须是工业总电量 126600
	for _, row := range g.Expected.PlainFees {
		if v := byKey[row.Name+"|"]; v != row.Fee {
			t.Fatalf("%s = %.2f，期望 %.2f", row.Name, v, row.Fee)
		}
	}
	// 「零售损益分摊电费」自第 39 轮起按 账单原值 × 分摊比例：
	//   计费数量 = 账单原值（20,880.13），单价 = ratio（0.21235625），
	//   金额 = round2(原值 × ratio) = 4,434.03（黄金基准已同步）。
	wantRaw := 0.0
	for _, f := range g.Inputs.FeeItems {
		if strings.Contains(f.Name, "零售损益分摊") {
			wantRaw += f.Fee
		}
	}
	for _, it := range got.Items {
		if it.Name != "零售损益分摊电费" {
			continue
		}
		if math.Abs(it.Qty-wantRaw) > 0.01 {
			t.Fatalf("零售损益分摊计费数量 = %.2f，必须是账单原值 %.2f", it.Qty, wantRaw)
		}
		if math.Abs(it.Rate-in.Ratio) > 1e-9 {
			t.Fatalf("零售损益分摊单价 = %.8f，必须是分摊比例 %.8f", it.Rate, in.Ratio)
		}
	}

	// ③ 纯金额返还类：一条都不能进明细（否则工业电费少 7,648）
	for _, name := range []string{
		"中长期偏差回收返还（用户）", "中长期超额申报费用返还", "零售交易发电侧收益返还电费",
	} {
		if _, ok := byName[name]; ok {
			t.Fatalf("%s 是纯金额返还项，不得计入分摊", name)
		}
	}

	// ④ 基本电费 / 力调电费
	s := g.Expected.Summary
	if v := byKey["基本电费|"]; v != s["capacityFee"] {
		t.Fatalf("基本电费 = %.2f，期望 %.2f", v, s["capacityFee"])
	}
	if v := byKey["功率因素调整电费|"]; v != s["pfAdjustFee"] {
		t.Fatalf("功率因素调整电费 = %.2f，期望 %.2f", v, s["pfAdjustFee"])
	}

	// ⑤ 大类汇总
	sumByName := func(group string) float64 {
		total := 0.0
		for _, it := range got.Items {
			if it.Category == group && it.Kind == "fee" {
				total += it.Fee
			}
		}
		return round2(total)
	}
	closeTo(t, "市场化购电费", sumByName("(1)市场化购电费"), s["marketPurchaseFee"], 0.02)
	closeTo(t, "上网环节线损费用", sumByName("(2)上网环节线损费用"), s["gridLineLossFee"], 0.02)
	closeTo(t, "零售输配电费", sumByName("(3)输配电量电费"), s["transmissionFee"], 0.02)
	closeTo(t, "系统运行费用", sumByName("(4)系统运行费"), s["systemOperationFee"], 0.02)
	closeTo(t, "政府性基金及附加", sumByName("(6)政府性基金及附加-工商业"), s["govFundFee"], 0.02)

	// ⑥ 工业电费 / 均价 / 电费合计
	industrialFee := round2(got.IndustrialFee)
	closeTo(t, "工业电费", industrialFee, s["industrialFee"], 0.02)
	closeTo(t, "均价", math.Round(industrialFee/in.StarTotal*10000)/10000, s["avgPrice"], 0.0001)
	elecTotal := round2(industrialFee + in.DormFee)
	closeTo(t, "电费合计", elecTotal, s["elecTotalFee"], 0.02)
	closeTo(t, "本期应付", round2(elecTotal+in.WaterFee), s["grandTotalFee"], 0.02)
}

// 边界：没有任何分表读数 / 差额为 0 时不得除零、不得产生 NaN。
func TestBillingTimedAllocationEdgeCases(t *testing.T) {
	alloc := allocateTimedKwh(nil, nil, nil, 0)
	closeTo(t, "空输入计费合计", alloc.TotalBill, 0, 0.001)
	closeTo(t, "空输入差额", alloc.DeltaQ, 0, 0.001)

	in := goldenInputs(t)
	zero := allocateTimedKwh(in.subUsages(), in.BillLoss, in.BillAdjust, 0)
	for _, p := range timedPeriodOrder {
		if math.IsNaN(zero.PeriodBase[p]) || math.IsInf(zero.PeriodBase[p], 0) {
			t.Fatalf("差额为 0 时 %s 基准电量异常：%v", p, zero.PeriodBase[p])
		}
	}
	// 无损耗无加减时，计费电量合计 = 抄见电量合计
	plain := allocateTimedKwh(in.subUsages(), map[string]float64{}, map[string]float64{}, 0)
	closeTo(t, "无损耗计费合计", plain.TotalBill, plain.TotalUsage, 0.01)
}

// subUsages 把输入里的分表读数转成 allocateTimedKwh 的入参（测试辅助）。
func (in *billingInputs) subUsages() []timedMeterUsage {
	out := make([]timedMeterUsage, 0, len(in.SubRows))
	for _, mr := range in.SubRows {
		usage := make(map[string]float64, 4)
		for _, p := range timedPeriodOrder {
			pr := mr.Periods[p]
			usage[p] = round2((pr.Curr - pr.Prev) * mr.Rate)
		}
		out = append(out, timedMeterUsage{Key: mr.MeterID, Name: mr.Name, Rate: mr.Rate, Usage: usage})
	}
	return out
}

// goldenEnabled 复刻 ensureDefaultItems 的结果：账单里出现的每个子项都开启。
//
// billingItemEnabled 对**缺失键返回 false**，所以测试不能只塞两三条开关，
// 否则整个费用明细会被跳过（假绿）。
func goldenEnabled(bill *types.UtilityBillExtractionItem) map[string]bool {
	m := map[string]bool{"基本电费": true}
	for _, f := range bill.FeeItems {
		m[f.Category+"|"+f.Name] = true
		if _, ok := m[f.Name]; !ok {
			m[f.Name] = true
		}
	}
	// 力调电费由 5.2 单独判定，键名用库里的「因数」写法
	m["功率因数调整电费|功率因数调整电费"] = true
	return m
}
