package service

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 市电账单「电量明细 / 输配容（需）量电费 / 功率因素调整电费」的确定性解析与补全。
//
// 这三张表在国网账单上是**固定表结构**，而 LLM 提取偶发漏抽：
// 真机事故 2026-10 账单的线损/加减全为 0、容量与力调明细整段丢失，直接导致
//   · 电量明细的计费电量退化成抄见电量；
//   · 输配容（需）量 / 功率因素两张子表「暂无数据」；
//   · 账单概况两项电费算成 0，与账单值对不上。
//
// 文本里的表结构非常稳定（markdown 表格 / 版面平铺都见过）：
//
//	||正向有功（尖峰）||238.31|250.14|3000||35490|0|18|-1403|34105|
//	输配容（需）量电费 ... 容量 2000 容量电价 22 输配容量电费 44000 ...
//
// 所以用确定性解析兜底。**只补不改**：票面能解析出来的值以票面为准，
// 解析不到的字段保留既有值；完全没原文时绝不凭空造数。

var (
	// 示数类型：只认正方向的有功/无功汇总与分时行。
	meterTypeRe = regexp.MustCompile(`正向(有功|无功)[（(](总|尖峰|峰|平|谷)[)）]`)
	// 任意数字（含负号、小数）。
	numberRe = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
)

// utilityLabelNumberRe 生成「标签 + 数字」的匹配器。
//
// 标签后面只允许空白与可选冒号，因此「容量」不会误吃「容量电价 22」，
// 「功率因数调整电费」也不会误吃表头里紧跟「功率因数实际值」的那次出现。
func utilityLabelNumberRe(label string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(label) + `\s*[:：]?\s*(-?\d+(?:\.\d+)?)`)
}

var (
	capacityValueRe       = utilityLabelNumberRe("容量")
	capacityPriceRe       = utilityLabelNumberRe("容量电价")
	capacityFeeRe         = utilityLabelNumberRe("输配容量电费")
	demandValueRe         = utilityLabelNumberRe("需量值")
	demandPriceRe         = utilityLabelNumberRe("需量电价")
	demandFeeRe           = utilityLabelNumberRe("输配需量电费")
	kwhPerKvaRe           = utilityLabelNumberRe("月每千伏安用电量")
	discountDemandFeeRe   = utilityLabelNumberRe("折扣需量电费")
	pfActualRe            = utilityLabelNumberRe("功率因数实际值")
	pfStandardRe          = utilityLabelNumberRe("功率因数标准")
	// 别名：票面/模型输出里「功率因数」与「功率因素」两种写法都出现过（需求明确要求统一映射），
	// 只认一种会在另一种写法上整段补洞失效。
	pfActualReAlt   = utilityLabelNumberRe("功率因素实际值")
	pfStandardReAlt = utilityLabelNumberRe("功率因素标准")
	pfAdjustRatioRe       = utilityLabelNumberRe("调整系数")
	pfActiveKwhRe         = utilityLabelNumberRe("参与调整有功电量")
	pfReactiveKwhRe       = utilityLabelNumberRe("参与调整无功电量")
	pfFeeBaseRe           = utilityLabelNumberRe("参与调整电费金额")
	pfAdjustFeeRe         = utilityLabelNumberRe("功率因数调整电费")
	pfAdjustFeeReAlt      = utilityLabelNumberRe("功率因素调整电费")
)

// ParseUtilityMeterReadings 从账单原文解析「电量明细」表。
//
// 返回顺序与原文一致（总表在前、分时在后），每行 8 个数字：
// 上期示数 / 本期示数 / 倍率 / 抄见电量 / 变损 / 线损 / 加减 / 计费电量。
// 数字不足 8 个的行（合并单元格拆碎的残行）直接跳过，不猜。
func ParseUtilityMeterReadings(text string) []types.UtilityMeterReading {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	out := make([]types.UtilityMeterReading, 0, 6)
	seen := make(map[string]bool, 6)
	for _, line := range strings.Split(text, "\n") {
		loc := meterTypeRe.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}
		meterType := line[loc[0]:loc[1]]
		// 同一示数类型只取第一次出现（账单里可能因分页重复表头/重复行）
		key := normalizeMeterType(meterType)
		if seen[key] {
			continue
		}
		nums := numberRe.FindAllString(line[loc[1]:], -1)
		if len(nums) < 8 {
			// 类型在行尾（如「示数类型 ... 正向有功（总）」）时，数字在类型之前
			nums = numberRe.FindAllString(line[:loc[0]], -1)
		}
		if len(nums) < 8 {
			continue
		}
		vals := make([]float64, 8)
		for i := 0; i < 8; i++ {
			vals[i] = parseUtilityFloat(nums[i])
		}
		out = append(out, types.UtilityMeterReading{
			MeterType:  meterType,
			Prev:       vals[0],
			Curr:       vals[1],
			Multiplier: vals[2],
			ReadingKwh: vals[3],
			TransLoss:  vals[4],
			LineLoss:   vals[5],
			Adjust:     vals[6],
			BillKwh:    vals[7],
		})
		seen[key] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseUtilityCapacityDetail 从账单原文解析「输配容（需）量电费」单行表。
// 一个字段都解析不到时返回 nil（调用方据此走顶层字段合成或保持原样）。
func ParseUtilityCapacityDetail(text string) *types.UtilityCapacityDetail {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	flat := utilityDetailSection(text)
	d := &types.UtilityCapacityDetail{
		Demand:       utilityMatchNumber(demandValueRe, flat),
		DemandPrice:  utilityMatchNumber(demandPriceRe, flat),
		DemandFee:    utilityMatchNumber(demandFeeRe, flat),
		KwhPerKva:    utilityMatchNumber(kwhPerKvaRe, flat),
		DiscountFee:  utilityMatchNumber(discountDemandFeeRe, flat),
		Capacity:     utilityMatchNumber(capacityValueRe, flat),
		CapacityPrice: utilityMatchNumber(capacityPriceRe, flat),
		CapacityFee:  utilityMatchNumber(capacityFeeRe, flat),
	}
	// 容量电费缺失但有容量与单价时按公式补（票面打印断行时偶发漏值）
	if d.CapacityFee == 0 && d.Capacity > 0 && d.CapacityPrice > 0 {
		d.CapacityFee = math.Round(d.Capacity*d.CapacityPrice*100) / 100
	}
	// 全部为 0 = 表里根本没有这块（不能当成「确实是 0」写进去，
	// 否则会把已有的人工值覆盖成 0）。
	if d.Demand == 0 && d.DemandFee == 0 && d.Capacity == 0 && d.CapacityFee == 0 {
		return nil
	}
	return d
}

// ParseUtilityPfDetail 从账单原文解析「功率因素调整电费」单行表。
func ParseUtilityPfDetail(text string) *types.UtilityPfAdjustDetail {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	flat := utilityDetailSection(text)
	d := &types.UtilityPfAdjustDetail{
		Project:     "功率因数调整电费",
		PowerFactor: utilityMatchFirstNumber(flat, pfActualRe, pfActualReAlt),
		Standard:    utilityMatchFirstNumber(flat, pfStandardRe, pfStandardReAlt),
		AdjustRatio: utilityMatchNumber(pfAdjustRatioRe, flat),
		ActiveKwh:   utilityMatchNumber(pfActiveKwhRe, flat),
		ReactiveKwh: utilityMatchNumber(pfReactiveKwhRe, flat),
		FeeBase:     utilityMatchNumber(pfFeeBaseRe, flat),
		AdjustFee:   utilityMatchFirstNumber(flat, pfAdjustFeeRe, pfAdjustFeeReAlt),
	}
	if d.PowerFactor == 0 && d.AdjustFee == 0 && d.AdjustRatio == 0 && d.FeeBase == 0 {
		return nil
	}
	return d
}

// NormalizeUtilityBillDetails 用票面原文补全一张市电账单的明细，并重算派生字段。
//
// 幂等：第二次调用（数据已完整）返回 false。启动回填可以反复执行。
func NormalizeUtilityBillDetails(item *types.UtilityBillExtractionItem, text string) bool {
	if item == nil {
		return false
	}
	changed := false

	// 1) 电量明细：票面解析结果覆盖/补齐同类型行。
	//
	// 前提是「解析出来的四段合计 == 工商业计费电量」——多计量点账单（如 2025.06/2025.10）
	// 文本里有多张电量明细表，只解析到第一张就替换会把 12~27 行压成 6 行、
	// 四段电量也被截断。判定不通过时只对空记录兜底，绝不覆盖既有数据。
	feeTOU, feeOK := touKwhFromFeeItems(item.FeeItems)
	if parsed := ParseUtilityMeterReadings(text); len(parsed) > 0 {
		parsedSum := 0.0
		for _, r := range parsed {
			if p := meterPeriodOf(r.MeterType); p != "" && p != "总" {
				parsedSum += r.BillKwh
			}
		}
		complete := false
		if feeOK {
			industrial := feeTOU.total()
			tol := math.Max(2, industrial*0.005)
			complete = industrial > 0 && math.Abs(parsedSum-industrial) <= tol
		} else if item.TotalKwh > 0 {
			complete = math.Abs(parsedSum-item.TotalKwh) <= math.Max(2, item.TotalKwh*0.02)
		}
		if complete {
			merged := mergeMeterReadings(item.MeterReadings, parsed)
			if !meterReadingsEqual(item.MeterReadings, merged) {
				item.MeterReadings = merged
				changed = true
			}
		} else if len(item.MeterReadings) == 0 {
			item.MeterReadings = parsed
			changed = true
		}
	}

	// 2) 逐行重算计费电量（票面公式：计费 = 抄见 + 变损 + 线损 + 加减）
	var touSum float64
	touKwh := map[string]float64{}
	for i := range item.MeterReadings {
		r := &item.MeterReadings[i]
		kind := meterPeriodOf(r.MeterType)
		if kind == "" {
			continue
		}
		if kind == "总" {
			// 汇总行（正向有功总/正向无功总）按原单不参与累加，计费电量置 0。
			// 否则列表合计会把「总 + 四段」重复相加（真机：1,401,690）。
			if r.BillKwh != 0 {
				r.BillKwh = 0
				changed = true
			}
			continue
		}
		if r.ReadingKwh != 0 {
			calc := roundUtility2(r.ReadingKwh + r.TransLoss + r.LineLoss + r.Adjust)
			if math.Abs(calc-r.BillKwh) > 0.5 {
				r.BillKwh = calc
				changed = true
			}
		}
		touKwh[kind] = r.BillKwh
		touSum += r.BillKwh
	}

	// 3) 分时电量（用能分析/概况口径）严格取四段计费电量。
	//
	// 取值优先级：费用明细的「零售交易电费」四段 qty（= 票面计费电量，全部账单都有）
	//  > 电量明细四段（仅当解析完整）。用 meter 行做唯一来源时，多计量点账单会
	// 只拿到第一张表，四段合计远小于账单总电量（真机：2025.06 得 333,600 / 711,127）。
	if feeOK {
		if setTOU(item, feeTOU) {
			changed = true
		}
	} else if touSum > 0 {
		if setTOU(item, touKwhFromMeter(touKwh)) {
			changed = true
		}
	}

	// 4) 输配容（需）量明细
	capacity := ParseUtilityCapacityDetail(text)
	if capacity != nil {
		if item.CapacityDetail == nil || !capacityDetailEqual(*item.CapacityDetail, *capacity) {
			item.CapacityDetail = capacity
			changed = true
		}
	} else if item.CapacityDetail == nil {
		// 原文没有明细表：用顶层字段合成一行，避免子表整块空白。
		if item.Capacity != 0 || item.CapacityFee != 0 || item.Demand != 0 {
			item.CapacityDetail = &types.UtilityCapacityDetail{
				Demand:        item.Demand,
				Capacity:      item.Capacity,
				CapacityPrice: item.CapacityPrice,
				CapacityFee:   item.CapacityFee,
			}
			changed = true
		}
	} else {
		// 已有明细但部分字段为 0：用顶层字段补空（不覆盖已有非零值）
		if fillCapacityFromOverview(item.CapacityDetail, item) {
			changed = true
		}
	}

	// 5) 功率因素调整明细
	pf := ParseUtilityPfDetail(text)
	if pf != nil {
		if item.PfDetail == nil || !pfDetailEqual(*item.PfDetail, *pf) {
			item.PfDetail = pf
			changed = true
		}
	} else if item.PfDetail == nil {
		if item.PfAdjustAmount != 0 || item.PowerFactor != 0 || item.AdjustRatio != 0 {
			item.PfDetail = &types.UtilityPfAdjustDetail{
				Project:     "功率因数调整电费",
				PowerFactor: item.PowerFactor,
				Standard:    item.PfStandard,
				AdjustRatio: item.AdjustRatio,
				AdjustFee:   item.PfAdjustAmount,
			}
			changed = true
		}
	} else if fillPfFromOverview(item.PfDetail, item) {
		changed = true
	}

	return changed
}

// touKwhSet 四段计费电量。
type touKwhSet struct {
	deep, peak, flat, valley float64
}

func (t touKwhSet) total() float64 { return t.deep + t.peak + t.flat + t.valley }

// touKwhFromFeeItems 从费用明细取「零售交易电费」四段计费电量。
//
// 这是票面直接给出的计费电量，比电量明细更可靠：多计量点账单的电量明细会被
// 拆成多张表，而费用明细的四段 qty 始终是**全账单口径**。
// 同一时段可能有多行（零售交易 + 偏差），按时段取最大，避免重复行累加。
func touKwhFromFeeItems(items []types.UtilityBillFeeItem) (touKwhSet, bool) {
	byPeriod := map[string]float64{}
	for _, it := range items {
		if !strings.Contains(it.Name, "零售交易电费") {
			continue
		}
		p := strings.TrimSpace(it.Period)
		if p != "尖峰" && p != "峰" && p != "平" && p != "谷" {
			continue
		}
		if it.Qty <= 0 {
			continue
		}
		if it.Qty > byPeriod[p] {
			byPeriod[p] = it.Qty
		}
	}
	if len(byPeriod) < 2 {
		return touKwhSet{}, false
	}
	return touKwhSet{
		deep:   byPeriod["尖峰"],
		peak:   byPeriod["峰"],
		flat:   byPeriod["平"],
		valley: byPeriod["谷"],
	}, true
}

func touKwhFromMeter(m map[string]float64) touKwhSet {
	return touKwhSet{deep: m["尖峰"], peak: m["峰"], flat: m["平"], valley: m["谷"]}
}

// setTOU 写回四段电量，返回是否有变化。
func setTOU(item *types.UtilityBillExtractionItem, t touKwhSet) bool {
	changed := false
	for _, f := range []struct {
		v   float64
		ptr *float64
	}{
		{t.deep, &item.DeepPeakKwh},
		{t.peak, &item.PeakKwh},
		{t.flat, &item.FlatKwh},
		{t.valley, &item.ValleyKwh},
	} {
		if *f.ptr != f.v {
			*f.ptr = f.v
			changed = true
		}
	}
	return changed
}

// ---- 合并与比较 ----

func mergeMeterReadings(existing, parsed []types.UtilityMeterReading) []types.UtilityMeterReading {
	out := make([]types.UtilityMeterReading, 0, len(parsed)+len(existing))
	index := make(map[string]int, len(existing))
	for i, r := range existing {
		index[normalizeMeterType(r.MeterType)] = i
	}
	// 以票面顺序为主：票面有的行用它，票面缺的行保留旧数据
	for _, p := range parsed {
		key := normalizeMeterType(p.MeterType)
		if i, ok := index[key]; ok {
			out = append(out, mergeMeterRow(existing[i], p))
			delete(index, key)
			continue
		}
		out = append(out, p)
	}
	for _, r := range existing {
		key := normalizeMeterType(r.MeterType)
		if _, ok := index[key]; ok {
			out = append(out, r)
		}
	}
	return out
}

// mergeMeterRow 票面值优先，但票面为 0 而旧值非 0 时保留旧值
// （某些账单单行确实是 0，靠「有值优先」避免把真实数据冲掉）。
func mergeMeterRow(old, fresh types.UtilityMeterReading) types.UtilityMeterReading {
	pick := func(a, b float64) float64 {
		if b != 0 {
			return b
		}
		return a
	}
	return types.UtilityMeterReading{
		MeterType:  fresh.MeterType,
		Prev:       pick(old.Prev, fresh.Prev),
		Curr:       pick(old.Curr, fresh.Curr),
		Multiplier: pick(old.Multiplier, fresh.Multiplier),
		ReadingKwh: pick(old.ReadingKwh, fresh.ReadingKwh),
		TransLoss:  pick(old.TransLoss, fresh.TransLoss),
		LineLoss:   pick(old.LineLoss, fresh.LineLoss),
		Adjust:     pick(old.Adjust, fresh.Adjust),
		BillKwh:    pick(old.BillKwh, fresh.BillKwh),
	}
}

func meterReadingsEqual(a, b []types.UtilityMeterReading) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func capacityDetailEqual(a, b types.UtilityCapacityDetail) bool { return a == b }

func pfDetailEqual(a, b types.UtilityPfAdjustDetail) bool { return a == b }

func fillCapacityFromOverview(d *types.UtilityCapacityDetail, item *types.UtilityBillExtractionItem) bool {
	changed := false
	fill := func(dst *float64, src float64) {
		if *dst == 0 && src != 0 {
			*dst = src
			changed = true
		}
	}
	fill(&d.Demand, item.Demand)
	fill(&d.Capacity, item.Capacity)
	fill(&d.CapacityPrice, item.CapacityPrice)
	fill(&d.CapacityFee, item.CapacityFee)
	return changed
}

func fillPfFromOverview(d *types.UtilityPfAdjustDetail, item *types.UtilityBillExtractionItem) bool {
	changed := false
	fill := func(dst *float64, src float64) {
		if *dst == 0 && src != 0 {
			*dst = src
			changed = true
		}
	}
	fill(&d.PowerFactor, item.PowerFactor)
	fill(&d.Standard, item.PfStandard)
	fill(&d.AdjustRatio, item.AdjustRatio)
	fill(&d.AdjustFee, item.PfAdjustAmount)
	if d.Project == "" {
		d.Project = "功率因数调整电费"
		changed = true
	}
	return changed
}

// ---- 工具函数 ----

// normalizeMeterType 归一示数类型（括号全半角、空白），用于合并去重。
func normalizeMeterType(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "(", "（")
	s = strings.ReplaceAll(s, ")", "）")
	return strings.ReplaceAll(s, " ", "")
}

// meterPeriodOf 取分时档位；非分时行返回空串。
func meterPeriodOf(meterType string) string {
	m := meterTypeRe.FindStringSubmatch(meterType)
	if m == nil {
		return ""
	}
	return m[2]
}

func flattenUtilityText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// utilityDetailSection 截出「输配容（需）量电费 / 功率因数调整电费」明细表那一段。
//
// 为什么必须截段：账单概况里也有「输配容量电费 2000 44000」这种并排数字
// （2000 是容量、44000 才是电费），全篇 FirstMatch 会拿到 2000，
// 真机回填就把 44,000 写成了 2,000。明细表的表头是
// 「输配容（需）量电费 功率因数调整电费」两个标题相邻出现，用它做锚点最稳。
func utilityDetailSection(text string) string {
	flat := flattenUtilityText(text)
	for _, anchor := range []string{"输配容（需）量电费", "输配容(需)量电费"} {
		if i := strings.Index(flat, anchor); i >= 0 {
			// 锚点后 200 字符内必须出现力调表头，否则是概况里的普通措辞
			if j := strings.Index(flat[i:], "功率因数调整电费"); j >= 0 && j < 200 {
				return flat[i:]
			}
		}
	}
	return flat
}

// utilityMatchFirstNumber 依次尝试多个标签正则，命中第一个非零值即返回。
func utilityMatchFirstNumber(text string, res ...*regexp.Regexp) float64 {
	for _, re := range res {
		if v := utilityMatchNumber(re, text); v != 0 {
			return v
		}
	}
	return 0
}

func utilityMatchNumber(re *regexp.Regexp, text string) float64 {
	m := re.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	return parseUtilityFloat(m[1])
}

func parseUtilityFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func roundUtility2(v float64) float64 {
	return math.Round(v*100) / 100
}
