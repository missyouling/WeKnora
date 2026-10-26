package service

import (
	"math"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 真机 2026.09 国网电费账单「账单详情」表格被解析成的原文（节选，含合并单元格摊平、
// 多行压成一行、纯文本降级三种形态）。表格结构本身不可改，解析器必须能吃下去。
const utilityFeeParserSample = `||账单详情||||||||单位：千瓦时、千乏时、元、元/千瓦时、千伏安（千瓦）、元/千伏安.月、元/千瓦.月|||
|---|---|---|---|---|---|---|---|---|---|---|---|
||正向有功（总）||3825.28|4023.9|3000||595860|0|0|0|0|
||正向有功（尖峰）||229.66|238.31|3000||25950|0|13|-1341|24622|
||正向有功（谷）||1523.43|1613.04|3000||268830|0|140|-1788|267182|
||正向无功（总） 电费明细 费用类别|费用组成|978.53|1028.07|3000|分时时段|148620|0 计费电量|0|0 计费标准|0 电费|
|||||||尖峰||0||0|0|
|||||||峰||0||0|0|
|||偏差电费||||平||0||0|0|
|||||||谷||0||0|0|
|||||||尖峰||24622||0.436|10735.19|
|||||||峰||112221||0.436|48928.36|
|||零售交易电费||||平||183205||0.436|79877.38|
|||||||谷||267182||0.436|116491.35|
|||||||尖峰||0||0|0|
|||||||峰||0||0|0|
|||绿电交易电费（省间）||||平||0||0|0|
||(1)市场化购电费|||||谷||0||0|0|
||||中长期偏差回收返还（用户）|||谷 平||0 0||0 0|0 -31520.93|
||||绿电环境价值电费（省外） 中长期超额申报费用返还（用户）|||平 平||0 0||0 0|0 -392.08|
||||中长期超额申报回收费用 零售交易发电侧收益返还电费|||平 平||0 0||0 0|0 -4103.6|
||(2)上网环节线损费|零售损益分摊电费 上网环节线损费用||||平 平||587230 587230||0.035557 0.017144|20880.13 10067.47|
|||||||尖峰||24622||0.2424|5968.37|
||(3)输配电量电费|零售输配电费||||峰 平||112221 183205||0.2424 0.1515|27202.37 27755.56|
|||||||谷||267182||0.05757|15381.67|
||(4)系统运行费用|燃气机组容量电费 电价交叉补贴新增损益 其他系统运行费用|新能源机制差价分摊电费 上网环节线损代理采购损益|||平 平 平 平 平||587230 587230 587230 587230 587230||0.018983 0.007546 -0.005536 -0.000971 0.004506|11147.39 4431.24 -3250.91 -570.2 2646.06|
|||抽水蓄能容量电费||||平||587230||0.00731|4292.65|
|||辅助服务费用||||平||587230||0|0|
||(6)政府性基金及附加|煤电容量电费 农网还贷||||平 平||587230 587230||0.023056 0.02|13539.17 11744.6|

国家重大水利工程建设基金 平 587230 0.001969 1156.26 库区移民基金 平 587230 0.006225 3655.51

(6)政府性基金及附加
可再生能源附加 平 587230 0.019 11157.37 小型水库移民后期扶持资金（地方） 平 587230 0.0005 293.62 小计： ￥3 8 7 5 1 4

电费明细

费用类别 费用组成 分时时段 计费电量 计费标准 电费

(5)目录电费 基础电费 平 8938 0.500306 4471.73
小型水库移民后期扶持资金（地方） 平 8938 0.0005 4.47 农网还贷 平 8938 0.02 178.76

(6)政府性基金及附加 可再生能源附加 平 8938 0.001 8.94
国家重大水利工程建设基金 平 8938 0.001969 17.6 库区移民基金 平 8938 0.006225 55.64 小计： ￥4 7 3 7. 1 4
`

func utilityFeeSumByCategory(rows []ParsedFeeRow, cat string) float64 {
	var sum float64
	for _, r := range rows {
		if r.Category == cat {
			sum += r.Fee
		}
	}
	return math.Round(sum*100) / 100
}

// 这张账单的「账单概况」印刷值：7 个分项电费与合计 433433.07（力调 -2818.07）。
func TestParseUtilityFeeRowsReconcilesRealBill(t *testing.T) {
	rows := ParseUtilityFeeRows(utilityFeeParserSample)
	if len(rows) < 20 {
		t.Fatalf("还原行数过少: %d（真机账单应为 28 行左右）", len(rows))
	}
	cases := []struct {
		category string
		want     float64
	}{
		{"(1)市场化购电费", 240895.8},
		{"(2)上网环节线损费用", 10067.47},
		{"(3)输配电量电费", 76307.97},
		{"(4)系统运行费", 32235.4},
	}
	for _, c := range cases {
		if got := utilityFeeSumByCategory(rows, c.category); math.Abs(got-c.want) > 0.02 {
			t.Errorf("%s 合计 = %.2f，账单概况为 %.2f", c.category, got, c.want)
		}
	}
	// 政府性基金：工商业 28007.36 + 居民 265.41 = 28272.77（按类别前缀汇总）
	var govSum float64
	for _, r := range rows {
		if strings.HasPrefix(r.Category, "(6)政府性基金及附加") {
			govSum += r.Fee
		}
	}
	if got := math.Round(govSum*100) / 100; math.Abs(got-28272.77) > 0.02 {
		t.Errorf("政府性基金及附加合计 = %.2f，账单为 28272.77", got)
	}
	if got := utilityFeeSumByCategory(rows, "(5)目录电费"); math.Abs(got-4471.73) > 0.02 {
		t.Errorf("目录电费 = %.2f，账单为 4471.73", got)
	}
	// 全部行合计 = 账单电费 433433.07 − 容量电费 44000 − 力调 −2818.07 = 392251.14
	// （容量电费是独立字段 capacity_fee，不在「电费明细」行里）
	if got := SumUtilityFeeRows(rows); math.Abs(got-392251.14) > 0.05 {
		t.Errorf("还原行合计 = %.2f，应为 392251.14（账单电费 − 容量电费 − 力调电费）", got)
	}
}

// 模型只提出 4 条时，必须用还原结果覆盖；模型提了大部分时不覆盖。
func TestApplyParsedUtilityFeeRowsOnlyWhenIncomplete(t *testing.T) {
	rows := ParseUtilityFeeRows(utilityFeeParserSample)
	item := &types.UtilityBillExtractionItem{
		GrandTotal:     433433.07,
		PfAdjustAmount: -2818.07,
		CapacityFee:    44000,
		FeeItems:       make([]types.UtilityBillFeeItem, 4),
	}
	if !ApplyParsedUtilityFeeRows(item, rows) {
		t.Fatal("模型只有 4 条时应采纳还原结果")
	}
	if len(item.FeeItems) != len(rows) {
		t.Fatalf("采纳后行数 = %d，应为 %d", len(item.FeeItems), len(rows))
	}
	// 「全 0 兄弟行」必须被去掉：否则前端按名称取第一条会拿到 0，少算金额
	var shadow int
	for _, f := range item.FeeItems {
		if strings.HasPrefix(f.Name, "中长期偏差回收返还") {
			shadow++
			if f.Fee != -31520.93 {
				t.Errorf("中长期偏差回收返还金额 = %v，应为 -31520.93", f.Fee)
			}
		}
	}
	if shadow != 1 {
		t.Errorf("中长期偏差回收返还应只有 1 行，实际 %d 行", shadow)
	}

	// 与概况对不上、且模型行数足够时不得覆盖
	bad := &types.UtilityBillExtractionItem{
		GrandTotal:     1000,
		PfAdjustAmount: 0,
		FeeItems:       make([]types.UtilityBillFeeItem, len(rows)),
	}
	if ApplyParsedUtilityFeeRows(bad, rows) {
		t.Fatal("与账单概况对不上且模型行数足够时不应覆盖")
	}
}

// 分时电量回填：模型漏填尖/峰/平/谷时，按「电量明细」表的计费电量补齐。
func TestDeriveUtilityTOUKwhFromMeterReadings(t *testing.T) {
	r := &types.UtilityBillExtractionItem{
		MeterReadings: []types.UtilityMeterReading{
			{MeterType: "正向有功（总）", ReadingKwh: 595860, BillKwh: 595860},
			{MeterType: "正向有功（尖峰）", ReadingKwh: 25950, BillKwh: 24622},
			{MeterType: "正向有功（峰）", ReadingKwh: 115290, BillKwh: 112221},
			{MeterType: "正向有功（平）", ReadingKwh: 185790, BillKwh: 183205},
			{MeterType: "正向有功（谷）", ReadingKwh: 268830, BillKwh: 267182},
			{MeterType: "正向无功（总）", ReadingKwh: 148620, BillKwh: 148620},
		},
	}
	deriveUtilityTOUKwh(r)
	if r.DeepPeakKwh != 24622 || r.PeakKwh != 112221 || r.FlatKwh != 183205 || r.ValleyKwh != 267182 {
		t.Fatalf("分时电量回填错误: 尖=%v 峰=%v 平=%v 谷=%v", r.DeepPeakKwh, r.PeakKwh, r.FlatKwh, r.ValleyKwh)
	}
	if r.ReactiveKwh != 148620 {
		t.Fatalf("无功电量回填错误: %v", r.ReactiveKwh)
	}
	// 已有汇总值时不得被覆盖（用户可能手工改过）
	r2 := &types.UtilityBillExtractionItem{
		PeakKwh: 999,
		MeterReadings: []types.UtilityMeterReading{
			{MeterType: "正向有功（峰）", BillKwh: 112221},
		},
	}
	deriveUtilityTOUKwh(r2)
	if r2.PeakKwh != 999 {
		t.Fatalf("不应覆盖已有峰电量，得到 %v", r2.PeakKwh)
	}
}

// 光伏：上网关口首行电量明细必须镜像到顶层（抽屉依赖 meter_prev 等）。
func TestNormalizeSolarBillUpliftsMeterReading(t *testing.T) {
	res := &SolarBillExtractionResult{
		Kind: "solar_bill",
		Records: []types.SolarBillExtractionItem{{
			BillPeriodStart: "2026-06-01",
			BillPeriodEnd:   "2026-06-30",
			Gateways: []types.SolarGateway{{
				GatewayType: "上网关口",
				Readings: []types.SolarReading{{
					MeterType: "反向有功（总）", Prev: 16.91, Curr: 18.75,
					Multiplier: 3000, ReadingKwh: 5520, BillKwh: 5520,
				}},
			}},
		}},
	}
	NormalizeSolarBillExtractionResult(res)
	r := res.Records[0]
	if r.MeterPrev != 16.91 || r.MeterCurr != 18.75 || r.MeterRatio != 3000 || r.MeterReading != 5520 || r.MeterBill != 5520 {
		t.Fatalf("顶层镜像错误: prev=%v curr=%v ratio=%v reading=%v bill=%v",
			r.MeterPrev, r.MeterCurr, r.MeterRatio, r.MeterReading, r.MeterBill)
	}
}
