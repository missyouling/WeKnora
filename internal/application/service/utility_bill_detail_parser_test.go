package service

import (
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 市电账单「电量明细 / 输配容（需）量 / 功率因素调整」的确定性补全。
//
// 为什么必须有这一层：这三张表是**票面固定表格**，LLM 偶发漏抽（真机：2026-10 的
// 线损/加减全 0、容量与力调明细整段丢失），一旦漏了，账单概况的「电费」列就会
// 算成 0、用能分析四段电量也会串位。文本里的表格结构非常稳定：
//
//	||正向有功（尖峰）||238.31|250.14|3000||35490|0|18|-1403|34105|
//	输配容（需）量电费 ... 容量 2000 容量电价 22 输配容量电费 44000 ...
//
// 因此用确定性解析兜底，且**只补不改**（有值优先，无值才填）。

// 2026-10 账单原文片段（chunks 5 与 13）。
const utilityMeterSample202610 = `||正向有功（总）||4023.9|4231.77|3000||623610|0|0|0|0|
||正向有功（尖峰）||238.31|250.14|3000||35490|0|18|-1403|34105|
||正向有功（峰）||869.94|915.42|3000||136440|0|70|-3274|133236|
||正向有功（谷）||1613.04|1697.1|3000||252180|0|131|-1871|250440|
||正向有功（平）||1302.59|1369.11|3000||199560|0|103|-2866|196797|`

const utilityCapacityPfSample202610 = `输配容（需）量电费 功率因数调整电费 需量值 0 需量电价 0 输配需量电费 0 功率因数实际值 0.97 功率因数标准 0.9 调整系数-0.0075 月每千伏安用电量 0 折扣需量电费 0 参与调整有功电量 623932 参与调整无功电量 154697 容量 2000 容量电价 22 输配容量电费 44000 参与调整电费金额 452657.06 功率因数调整电费 -3394.92 小计： ￥44000 小计： ￥-3394.92`

func TestParseUtilityMeterReadingsFromMarkdownTable(t *testing.T) {
	rows := ParseUtilityMeterReadings(utilityMeterSample202610)
	if len(rows) != 5 {
		t.Fatalf("want 5 meter rows, got %d: %+v", len(rows), rows)
	}
	byType := map[string]types.UtilityMeterReading{}
	for _, r := range rows {
		byType[r.MeterType] = r
	}
	peak := byType["正向有功（尖峰）"]
	if peak.Prev != 238.31 || peak.Curr != 250.14 || peak.Multiplier != 3000 {
		t.Fatalf("尖峰示数解析错: %+v", peak)
	}
	if peak.ReadingKwh != 35490 || peak.LineLoss != 18 || peak.Adjust != -1403 || peak.BillKwh != 34105 {
		t.Fatalf("尖峰电量解析错: %+v", peak)
	}
	if got := byType["正向有功（平）"]; got.LineLoss != 103 || got.Adjust != -2866 || got.BillKwh != 196797 {
		t.Fatalf("平段解析错: %+v", got)
	}
	// 总表行：票面上「计费电量」列本来就是 0，只有抄见电量 623610
	if got := byType["正向有功（总）"]; got.ReadingKwh != 623610 || got.BillKwh != 0 {
		t.Fatalf("总表电量解析错: %+v", got)
	}
	// 幂等：同一段文本解析两次结果一致（不能因为反复回填而漂移）
	if again := ParseUtilityMeterReadings(utilityMeterSample202610); len(again) != len(rows) {
		t.Fatalf("重复解析行数不一致: %d vs %d", len(again), len(rows))
	}
}

func TestParseUtilityCapacityDetailFromFlatText(t *testing.T) {
	cap := ParseUtilityCapacityDetail(utilityCapacityPfSample202610)
	if cap == nil {
		t.Fatal("容量明细未解析出来")
	}
	if cap.Demand != 0 || cap.DemandPrice != 0 || cap.DemandFee != 0 {
		t.Fatalf("需量字段解析错: %+v", cap)
	}
	if cap.Capacity != 2000 || cap.CapacityPrice != 22 || cap.CapacityFee != 44000 {
		t.Fatalf("容量字段解析错: %+v", cap)
	}
	if cap.KwhPerKva != 0 || cap.DiscountFee != 0 {
		t.Fatalf("派生字段应为 0: %+v", cap)
	}
	// 「容量」不能误吃「容量电价 22」：这是最容易踩的标签前缀歧义
	if cap.Capacity == 22 {
		t.Fatal("容量标签误匹配了容量电价")
	}
}

func TestParseUtilityPfDetailFromFlatText(t *testing.T) {
	pf := ParseUtilityPfDetail(utilityCapacityPfSample202610)
	if pf == nil {
		t.Fatal("功率因素明细未解析出来")
	}
	if pf.PowerFactor != 0.97 || pf.Standard != 0.9 || pf.AdjustRatio != -0.0075 {
		t.Fatalf("功率因数三要素解析错: %+v", pf)
	}
	if pf.ActiveKwh != 623932 || pf.ReactiveKwh != 154697 {
		t.Fatalf("参与调整电量解析错: %+v", pf)
	}
	if pf.FeeBase != 452657.06 || pf.AdjustFee != -3394.92 {
		t.Fatalf("参与调整电费/调整电费解析错: %+v", pf)
	}
}

// 归一化：把「票面表格解析出来的值」写回记录，并重算派生字段。
func TestNormalizeUtilityBillDetailsRepairs202610(t *testing.T) {
	item := &types.UtilityBillExtractionItem{
		TotalKwh: 623932, TotalAmount: 481647.91, GrandTotal: 481647.91,
		Capacity: 2000, CapacityPrice: 22, CapacityFee: 44000,
		PowerFactor: 0.97, PfStandard: 0.9, AdjustRatio: -0.0075,
		PfAdjustAmount: -3394.92,
		MeterReadings: []types.UtilityMeterReading{
			{MeterType: "正向有功（总）", ReadingKwh: 623610, BillKwh: 623610, Multiplier: 3000},
			{MeterType: "正向有功（尖峰）", ReadingKwh: 35490, BillKwh: 35490, Multiplier: 3000},
		},
		// 四段电量被模型抄成了「尖峰重复」
		DeepPeakKwh: 35490, PeakKwh: 35490, FlatKwh: 199560, ValleyKwh: 250440,
	}
	text := utilityMeterSample202610 + "\n" + utilityCapacityPfSample202610
	if !NormalizeUtilityBillDetails(item, text) {
		t.Fatal("应当识别出需要补全")
	}
	if len(item.MeterReadings) != 5 {
		t.Fatalf("电量明细应为 5 行，实际 %d", len(item.MeterReadings))
	}
	var sumTOU float64
	for _, r := range item.MeterReadings {
		switch r.MeterType {
		case "正向有功（总）", "正向无功（总）":
			if r.BillKwh != 0 {
				t.Fatalf("%s 的计费电量应为 0（原单不参与累加），实际 %v", r.MeterType, r.BillKwh)
			}
		case "正向有功（尖峰）", "正向有功（峰）", "正向有功（平）", "正向有功（谷）":
			calc := r.ReadingKwh + r.TransLoss + r.LineLoss + r.Adjust
			if calc != r.BillKwh {
				t.Fatalf("%s 计费电量 != 抄见+变损+线损+加减: %v != %v", r.MeterType, r.BillKwh, calc)
			}
			sumTOU += r.BillKwh
		}
	}
	if sumTOU != 614578 {
		t.Fatalf("四段计费电量合计应为 614578，实际 %v", sumTOU)
	}
	if item.DeepPeakKwh != 34105 || item.PeakKwh != 133236 || item.FlatKwh != 196797 || item.ValleyKwh != 250440 {
		t.Fatalf("分时电量未按计费电量回填: %v/%v/%v/%v",
			item.DeepPeakKwh, item.PeakKwh, item.FlatKwh, item.ValleyKwh)
	}
	if item.CapacityDetail == nil || item.CapacityDetail.Capacity != 2000 || item.CapacityDetail.CapacityFee != 44000 {
		t.Fatalf("容量明细未写入: %+v", item.CapacityDetail)
	}
	if item.PfDetail == nil || item.PfDetail.AdjustFee != -3394.92 || item.PfDetail.PowerFactor != 0.97 {
		t.Fatalf("功率因素明细未写入: %+v", item.PfDetail)
	}
}

// 幂等：已经完整的记录再跑一次不应改动（否则每次启动回填都会抖动）。
func TestNormalizeUtilityBillDetailsIsIdempotent(t *testing.T) {
	text := utilityMeterSample202610 + "\n" + utilityCapacityPfSample202610
	item := &types.UtilityBillExtractionItem{TotalKwh: 623932}
	NormalizeUtilityBillDetails(item, text)
	first := *item
	if NormalizeUtilityBillDetails(item, text) {
		t.Fatalf("第二次归一化不应再改动: %+v", item)
	}
	if item.DeepPeakKwh != first.DeepPeakKwh || item.CapacityDetail.Capacity != first.CapacityDetail.Capacity {
		t.Fatal("幂等性被破坏：第二次结果与第一次不同")
	}
}

// 无原文（老数据没有 chunks）时不得凭空造明细。
func TestNormalizeUtilityBillDetailsSkipsWithoutText(t *testing.T) {
	item := &types.UtilityBillExtractionItem{TotalKwh: 100}
	if NormalizeUtilityBillDetails(item, "") {
		t.Fatal("无原文时不应改动记录")
	}
	if item.CapacityDetail != nil || item.PfDetail != nil {
		t.Fatal("无原文时不应造出明细")
	}
}

// 只有顶层容量/力调字段、原文里没有明细表时：用顶层字段合成单行明细，
// 让前端两张子表至少能显示「容量 2000 / 电价 22 / 电费 44000」。
func TestNormalizeUtilityBillDetailsSynthesizesFromOverviewFields(t *testing.T) {
	item := &types.UtilityBillExtractionItem{
		Capacity: 2000, CapacityPrice: 22, CapacityFee: 44000,
		PowerFactor: 0.97, PfStandard: 0.9, AdjustRatio: -0.0075, PfAdjustAmount: -2818.07,
	}
	if !NormalizeUtilityBillDetails(item, "账单文本没有明细表") {
		t.Fatal("顶层字段存在时应合成明细")
	}
	if item.CapacityDetail == nil || item.CapacityDetail.CapacityFee != 44000 {
		t.Fatalf("容量明细合成失败: %+v", item.CapacityDetail)
	}
	if item.PfDetail == nil || item.PfDetail.AdjustFee != -2818.07 || item.PfDetail.PowerFactor != 0.97 {
		t.Fatalf("功率因素明细合成失败: %+v", item.PfDetail)
	}
}

// 概况里的「输配容量电费 2000 44000」不能把明细表的 44000 覆盖成 2000。
// 真机：回填后 capacity_fee 变成 2,000（取到了概况里容量那一列）。
func TestParseUtilityCapacityDetailPrefersDetailTableOverOverview(t *testing.T) {
	text := `账单概况 其中： 输配电量电费 84796.04 输配容量电费 2000 44000
` + utilityCapacityPfSample202610
	cap := ParseUtilityCapacityDetail(text)
	if cap == nil {
		t.Fatal("容量明细未解析出来")
	}
	if cap.Capacity != 2000 || cap.CapacityPrice != 22 || cap.CapacityFee != 44000 {
		t.Fatalf("容量明细取错段: %+v", cap)
	}
}

// 「功率因数」与「功率因素」两种写法都要能解析（需求要求统一映射）。
func TestParseUtilityPfDetailAcceptsYinshuAlias(t *testing.T) {
	text := `输配容（需）量电费 功率因素调整电费 需量值 0 需量电价 0 输配需量电费 0
功率因素实际值 0.96 功率因素标准 0.9 调整系数 -0.01 参与调整有功电量 100
参与调整无功电量 20 容量 2000 容量电价 22 输配容量电费 44000 参与调整电费金额 500 功率因素调整电费 -5`
	pf := ParseUtilityPfDetail(text)
	if pf == nil {
		t.Fatal("因素写法未解析出明细")
	}
	if pf.PowerFactor != 0.96 || pf.Standard != 0.9 || pf.AdjustRatio != -0.01 || pf.AdjustFee != -5 {
		t.Fatalf("因素写法解析错: %+v", pf)
	}
}

// 多计量点账单：文本里有多张电量明细表，只解析到第一张时不得覆盖既有 12~27 行，
// 四段电量也不能被截断（真机：2025.06 被压成 6 行、四段 333,600 / 总数 711,127）。
func TestNormalizeUtilityBillDetailsKeepsMultiPointMeterRows(t *testing.T) {
	existing := []types.UtilityMeterReading{
		{MeterType: "正向有功（总）", ReadingKwh: 377130, BillKwh: 0},
		{MeterType: "正向有功（尖峰）", ReadingKwh: 24030, BillKwh: 24030},
		{MeterType: "正向有功（峰）", ReadingKwh: 73740, BillKwh: 73740},
		{MeterType: "正向有功（平）", ReadingKwh: 107280, BillKwh: 107280},
		{MeterType: "正向有功（谷）", ReadingKwh: 128550, BillKwh: 128550},
		{MeterType: "正向有功（尖峰）", ReadingKwh: 411, BillKwh: 411},
	}
	item := &types.UtilityBillExtractionItem{
		TotalKwh:      711127,
		MeterReadings: existing,
		FeeItems: []types.UtilityBillFeeItem{
			{Name: "零售交易电费", Period: "尖峰", Qty: 54000},
			{Name: "零售交易电费", Period: "峰", Qty: 163530},
			{Name: "零售交易电费", Period: "平", Qty: 227070},
			{Name: "零售交易电费", Period: "谷", Qty: 266070},
		},
	}
	NormalizeUtilityBillDetails(item, utilityMeterSample202610)
	if len(item.MeterReadings) != len(existing) {
		t.Fatalf("多计量点账单的电量明细不得被覆盖：%d → %d", len(existing), len(item.MeterReadings))
	}
	// 四段取费用明细（全账单口径），不是第一张表的局部值
	if item.DeepPeakKwh != 54000 || item.PeakKwh != 163530 || item.FlatKwh != 227070 || item.ValleyKwh != 266070 {
		t.Fatalf("四段电量应取费用明细: %v/%v/%v/%v",
			item.DeepPeakKwh, item.PeakKwh, item.FlatKwh, item.ValleyKwh)
	}
}
