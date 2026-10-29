package service

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 国网电费单「电费明细」表的**确定性行还原**。
//
// 为什么需要它（真机根因）：
//
//	PDF 里的「账单详情」是一张 12 列大宽表，费用类别与费用组成都是纵向合并单元格。
//	文档解析成 Markdown 后合并单元格被摊平成空串，还会把多行压成一行：
//
//	  |||||||峰||112221||0.436|48928.36|
//	  |||零售交易电费||||平||183205||0.436|79877.38|
//	  ||(3)输配电量电费|零售输配电费||||峰 平||112221 183205||0.2424 0.1515|27202.37 27755.56|
//
//	模型对这种"费用组成只在组中间出现一次、且一行里塞了多行数据"的文本极不稳定：
//	同一份 2026.09 账单连续多次提取，有时只输出 4 条（只有市场化购电费），
//	有时一条都没有 —— 表现为详情页 7 个分项全 0、子表「暂无数据」、
//	本期电费只剩容量电费与力调电费。
//
// 所以这里不看模型，直接从原文把行还原出来：列的位置是固定的
// （时段 / 计费电量 / 计费标准 / 电费），费用组成按「同单价继承」补齐，
// 类别按费用组成名映射（国网费用组成名是全行业统一的）。
// 结果先与账单概况核对（各行电费合计 == 账单电费 − 力调电费），对得上才采纳。

// ParsedFeeRow 一条还原出来的费用明细行。
type ParsedFeeRow struct {
	Category string
	Name     string
	Period   string
	Qty      float64
	Rate     float64
	Fee      float64
}

// utilityFeePeriodTokens 分时时段取值（"尖"是"尖峰"的简写）。
var utilityFeePeriodTokens = map[string]bool{
	"尖峰": true, "尖": true, "峰": true, "平": true, "谷": true,
}

// utilityFeeCategoryByName 费用组成 → 费用类别。
//
// 用字典而不是"按表格里的 (1)(2) 标记"：合并单元格让标记落在错误的行上
// （真机：`(6)政府性基金及附加` 那一行同时带着属于 (4) 的「煤电容量电费」）。
// 费用组成名在全行业统一，映射比位置可靠。
var utilityFeeCategoryByName = map[string]string{
	// (1) 市场化购电费
	"零售交易电费":          "(1)市场化购电费",
	"偏差电费":            "(1)市场化购电费",
	"绿电交易电费(省间)":      "(1)市场化购电费",
	"绿电交易电费(省内)":      "(1)市场化购电费",
	"绿电环境价值电费(省外)":    "(1)市场化购电费",
	"中长期偏差回收返还(用户)":   "(1)市场化购电费",
	"中长期超额申报费用返还(用户)": "(1)市场化购电费",
	"中长期超额申报回收费用":     "(1)市场化购电费",
	"零售交易发电侧收益返还电费":   "(1)市场化购电费",
	"零售损益分摊电费":        "(1)市场化购电费",
	// (2) 上网环节线损费用
	"上网环节线损费用": "(2)上网环节线损费用",
	// (3) 输配电量电费
	"零售输配电费": "(3)输配电量电费",
	// (4) 系统运行费
	"燃气机组容量电费":     "(4)系统运行费",
	"新能源机制差价分摊电费":  "(4)系统运行费",
	"电价交叉补贴新增损益":   "(4)系统运行费",
	"上网环节线损代理采购损益": "(4)系统运行费",
	"其他系统运行费用":     "(4)系统运行费",
	"抽水蓄能容量电费":     "(4)系统运行费",
	"辅助服务费用":       "(4)系统运行费",
	"煤电容量电费":       "(4)系统运行费",
	// (5) 目录电费
	"基础电费": "(5)目录电费",
	// (6) 政府性基金及附加（工商业/居民由前端按计费电量区分）
	"农网还贷":             "(6)政府性基金及附加",
	"可再生能源附加":          "(6)政府性基金及附加",
	"国家重大水利工程建设基金":     "(6)政府性基金及附加",
	"库区移民基金":           "(6)政府性基金及附加",
	"小型水库移民后期扶持资金(地方)": "(6)政府性基金及附加",
	"重大水利工程建设基金":       "(6)政府性基金及附加",
}

// utilityFeeCategoryPrefixRe 单元格里的费用类别标记，如 (1)市场化购电费。
var utilityFeeCategoryPrefixRe = regexp.MustCompile(`^\((\d)\)([^|]*)`)

// utilityFeeTupleRe 纯文本形态的一行：费用组成 + 时段 + 计费电量 + 计费标准 + 电费。
// 例：`农网还贷 平 587230 0.02 11744.6`
var utilityFeeTupleRe = regexp.MustCompile(
	`([^\d\s|]{2,40}?)\s+(尖峰|峰|平|谷)\s+(\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)`)
var utilityFeeNumberRe = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)

// ParseUtilityFeeRows 从账单全文还原费用明细行。
//
// 支持两种形态：Markdown 表格行（合并单元格被摊平）与纯文本行（表格解析降级时）。
// 返回行按账单出现顺序排列并已去重（相邻 chunk 有重叠，同一条行会出现在两块里）。
func ParseUtilityFeeRows(text string) []ParsedFeeRow {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var rows []ParsedFeeRow
	runningCategory := ""
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.Count(line, "|") >= 6 {
			parsed, cat := parseUtilityFeePipeLine(line, runningCategory)
			if cat != "" {
				runningCategory = cat
			}
			rows = append(rows, parsed...)
			continue
		}
		rows = append(rows, parseUtilityFeePlainLine(line, runningCategory)...)
	}
	injectUtilityFeeNames(rows)
	rows = dropZeroShadowRows(rows)
	out := make([]ParsedFeeRow, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		// 费用组成字典优先：合并单元格会让 (N) 标记落在相邻组的行上
		// （真机：「(6)政府性基金及附加」那行同时带着属于 (4) 的「煤电容量电费」）。
		if byName := utilityFeeCategoryOf(r.Name); byName != "" {
			r.Category = byName
		}
		key := utilityFeeRowKey(r)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	// 放在按名称定类别之后：否则字典里的「(6)政府性基金及附加」会把后缀冲掉。
	suffixGovFundRows(out)
	return out
}

// dropZeroShadowRows 去掉「同名同类别、全 0 的兄弟行」。
//
// 合并单元格把同一费用组成的多个时段压进一行时，会出现
//
//	||||中长期偏差回收返还（用户）|||谷 平||0 0||0 0|0 -31520.93|
//
// 解析结果是「谷 0 / 平 -31520.93」两行同名记录。前端的行配置按名称匹配
// 且只取第一条命中（find），拿到的是全 0 那行 → 账单概况少算这笔钱
// （真机：本期电费 464954 而不是 433433.07，差的正是 31520.93）。
//
// 判据收得很紧：同类别同名、且组内**恰好只有一行**有金额，其余全 0 —— 才丢零行。
// 绿电/偏差电费这类整组都是 0 的不会被误删（它们本来就没有非零兄弟）。
func dropZeroShadowRows(rows []ParsedFeeRow) []ParsedFeeRow {
	type grp struct {
		idx           []int
		nonZero, zero int
	}
	groups := make(map[string]*grp, len(rows))
	order := make([]string, 0, len(rows))
	for i, r := range rows {
		k := r.Category + "|" + normalizeUtilityFeeName(r.Name)
		g := groups[k]
		if g == nil {
			g = &grp{}
			groups[k] = g
			order = append(order, k)
		}
		g.idx = append(g.idx, i)
		if r.Fee != 0 || r.Qty != 0 {
			g.nonZero++
		} else {
			g.zero++
		}
	}
	drop := make(map[int]bool, len(rows))
	for _, k := range order {
		g := groups[k]
		if g.nonZero != 1 || g.zero == 0 {
			continue
		}
		for _, i := range g.idx {
			if rows[i].Fee == 0 && rows[i].Qty == 0 {
				drop[i] = true
			}
		}
	}
	if len(drop) == 0 {
		return rows
	}
	out := make([]ParsedFeeRow, 0, len(rows)-len(drop))
	for i, r := range rows {
		if drop[i] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// suffixGovFundRows 把「政府性基金及附加」按计费电量拆成工商业/居民两支。
//
// 与存量账单的类别命名保持一致（`(6)政府性基金及附加-工商业` / `-居民`）：
// 同一份账单里工商业计费电量（约 58 万）与居民目录电量（约 8 千）差两个量级，
// 取该类别下最大的计费电量作为工商业基准即可，不依赖账单总电量。
func suffixGovFundRows(rows []ParsedFeeRow) {
	const govFund = "(6)政府性基金及附加"
	// 第一步：找出该类别下的最大计费电量（= 工商业基准）
	var maxQty float64
	for _, r := range rows {
		if r.Category == govFund && r.Qty > maxQty {
			maxQty = r.Qty
		}
	}
	if maxQty <= 0 {
		return
	}
	for i := range rows {
		if rows[i].Category != govFund {
			continue
		}
		if rows[i].Qty >= maxQty*0.5 {
			rows[i].Category = govFund + "-工商业"
		} else {
			rows[i].Category = govFund + "-居民"
		}
	}
}

// parseUtilityFeePipeLine 解析一行 Markdown 表格。
//
// 只处理带「分时时段」的行——电量明细表和表头行没有时段，直接跳过。
func parseUtilityFeePipeLine(line, runningCategory string) ([]ParsedFeeRow, string) {
	cells := strings.Split(strings.Trim(line, "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	periodIdx := -1
	for i, c := range cells {
		if c == "" {
			continue
		}
		tokens := strings.Fields(c)
		allPeriods := len(tokens) > 0
		for _, t := range tokens {
			if !utilityFeePeriodTokens[t] {
				allPeriods = false
				break
			}
		}
		if allPeriods {
			periodIdx = i
		}
	}
	// 类别标记可能出现在时段列之前（含与名称同一格的情况）
	category := ""
	// 名称**按单元格收集**，不能直接摊成一个一维数组。
	//
	// 真机根因（2026-09 的 (4)系统运行费）：
	//
	//	||(4)系统运行费用|燃气机组容量电费 电价交叉补贴新增损益 其他系统运行费用|新能源机制差价分摊电费 上网环节线损代理采购损益|||平×5|...
	//
	// PDF 的「费用组成」列被拆成**两个物理列**，抽取后变成两个单元格：左边 3 个名字、
	// 右边 2 个名字。若按「先左格全部、再右格全部」摊平，得到
	// [燃气机组, 电价交叉补贴, 其他系统运行, 新能源机制差价, 上网线损代理]，
	// 而数值列是单行阅读顺序 [燃气机组, 新能源机制差价, 电价交叉补贴, 上网线损代理, 其他系统运行]，
	// 按位置一一对应就会张冠李戴：新能源机制差价拿到 -570.2、电价交叉补贴拿到 4431.24……
	// 正确做法是按**视觉行**交错取（左1, 右1, 左2, 右2, 左3），还原 PDF 的阅读顺序。
	var nameCells [][]string
	for i := 0; i < len(cells); i++ {
		if i == periodIdx {
			continue
		}
		c := cells[i]
		if c == "" || utilityFeeNumberRe.MatchString(c) {
			continue
		}
		if m := utilityFeeCategoryPrefixRe.FindStringSubmatch(c); m != nil {
			category = "(" + m[1] + ")" + strings.TrimSpace(m[2])
			c = strings.TrimSpace(strings.TrimPrefix(c, m[0]))
		}
		if c == "" || utilityFeePeriodTokens[c] || strings.Contains(c, "费用类别") ||
			strings.Contains(c, "费用组成") || strings.Contains(c, "其中") ||
			strings.Contains(c, "单位：") || strings.Contains(c, "示数类型") {
			continue
		}
		// 只取时段列**之前**的文本单元做费用组成；之后的都是数值列
		if periodIdx >= 0 && i > periodIdx {
			continue
		}
		if tokens := strings.Fields(c); len(tokens) > 0 {
			nameCells = append(nameCells, tokens)
		}
	}
	names := interleaveUtilityFeeNames(nameCells)
	if periodIdx < 0 {
		if category != "" {
			return nil, category
		}
		return nil, ""
	}
	periods := strings.Fields(cells[periodIdx])
	var numeric [3][]float64 // 依次为：计费电量、计费标准、电费
	slot := 0
	for i := periodIdx + 1; i < len(cells) && slot < 3; i++ {
		c := cells[i]
		if c == "" {
			continue
		}
		tokens := strings.Fields(c)
		vals := make([]float64, 0, len(tokens))
		ok := true
		for _, t := range tokens {
			v, okv := parseUtilityFeeNumber(t)
			if !okv {
				ok = false
				break
			}
			vals = append(vals, v)
		}
		if !ok || len(vals) == 0 {
			continue
		}
		numeric[slot] = vals
		slot++
	}
	if len(numeric[2]) == 0 {
		if category != "" {
			return nil, category
		}
		return nil, ""
	}
	n := len(periods)
	for _, list := range numeric {
		if len(list) > n {
			n = len(list)
		}
	}
	if len(names) > n {
		n = len(names)
	}
	rows := make([]ParsedFeeRow, 0, n)
	for i := 0; i < n; i++ {
		row := ParsedFeeRow{
			Category: category,
			Period:   pickUtilityFeeList(periods, i),
			Qty:      pickUtilityFeeNum(numeric[0], i),
			Rate:     pickUtilityFeeNum(numeric[1], i),
			Fee:      pickUtilityFeeNum(numeric[2], i),
		}
		if i < len(names) {
			row.Name = names[i]
		}
		rows = append(rows, row)
	}
	if category == "" {
		category = runningCategory
		for i := range rows {
			rows[i].Category = category
		}
	}
	return rows, category
}

// interleaveUtilityFeeNames 把「被拆成多列的名称单元格」按视觉行交错成一维。
//
// 每个单元格是 PDF 里一列（费用组成列被拆成 1~2 列），列内顺序即自上而下；
// 交错（左1 右1 左2 右2 …）才等价于「一行一行读」的真实顺序，
// 直接顺次拼接会让名称与数值整体错位（见 parseUtilityFeePipeLine 的注释）。
func interleaveUtilityFeeNames(cells [][]string) []string {
	if len(cells) == 0 {
		return nil
	}
	if len(cells) == 1 {
		return append([]string(nil), cells[0]...)
	}
	maxLen := 0
	total := 0
	for _, c := range cells {
		if len(c) > maxLen {
			maxLen = len(c)
		}
		total += len(c)
	}
	out := make([]string, 0, total)
	for i := 0; i < maxLen; i++ {
		for _, c := range cells {
			if i < len(c) {
				out = append(out, c[i])
			}
		}
	}
	return out
}

// parseUtilityFeePlainLine 解析纯文本形态的「费用组成 时段 电量 标准 电费」元组。
func parseUtilityFeePlainLine(line, runningCategory string) []ParsedFeeRow {
	matches := utilityFeeTupleRe.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	rows := make([]ParsedFeeRow, 0, len(matches))
	category := runningCategory
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if cm := utilityFeeCategoryPrefixRe.FindStringSubmatch(name); cm != nil {
			category = "(" + cm[1] + ")" + strings.TrimSpace(cm[2])
			name = strings.TrimSpace(strings.TrimPrefix(name, cm[0]))
		}
		name = strings.TrimSpace(strings.TrimSuffix(name, "其中:"))
		name = strings.TrimSpace(strings.TrimSuffix(name, "其中："))
		qty, _ := parseUtilityFeeNumber(m[3])
		rate, _ := parseUtilityFeeNumber(m[4])
		fee, _ := parseUtilityFeeNumber(m[5])
		rows = append(rows, ParsedFeeRow{
			Category: category,
			Name:     normalizeUtilityFeeName(name),
			Period:   m[2],
			Qty:      qty,
			Rate:     rate,
			Fee:      fee,
		})
	}
	return rows
}

// injectUtilityFeeNames 给"费用组成列被摊平"的行补名。
//
// 合并单元格的名称只在组的中间行出现（真机：偏差电费 4 行，名字落在第 3 行），
// 所以按"同单价优先"在前一行/后一行里找同组名称：
//  1. 前一行同名单价 → 直接继承（零售交易电费 尖峰/峰 在前、谷 在后）；
//  2. 否则后一行同单价 → 继承（组的首个数据行在名称行之前）；
//  3. 都不匹配则继承前一行的名称（0 金额行常见）。
func injectUtilityFeeNames(rows []ParsedFeeRow) {
	for i := range rows {
		if rows[i].Name != "" {
			continue
		}
		prev, next := -1, -1
		for j := i - 1; j >= 0; j-- {
			if rows[j].Name != "" {
				prev = j
				break
			}
		}
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Name != "" {
				next = j
				break
			}
		}
		switch {
		case prev >= 0 && utilityFeeRateEqual(rows[prev].Rate, rows[i].Rate):
			rows[i].Name = rows[prev].Name
			rows[i].Category = rows[prev].Category
		case next >= 0 && utilityFeeRateEqual(rows[next].Rate, rows[i].Rate):
			rows[i].Name = rows[next].Name
			rows[i].Category = rows[next].Category
		case prev >= 0:
			rows[i].Name = rows[prev].Name
			rows[i].Category = rows[prev].Category
		case next >= 0:
			rows[i].Name = rows[next].Name
			rows[i].Category = rows[next].Category
		}
	}
}

// ApplyParsedUtilityFeeRows 用还原结果替换模型输出的费用明细。
//
// 采纳条件（任一）：
//  1. 各行电费合计 == 账单电费 − 力调电费 − 容（需）量电费（差 0.5% 以内）→ 与概况自洽，直接采纳；
//  2. 模型漏提（模型行数 < 还原行数的一半）→ 采纳还原结果，至少不再是全 0。
//
// 返回是否采纳，供调用方记录日志。
func ApplyParsedUtilityFeeRows(item *types.UtilityBillExtractionItem, rows []ParsedFeeRow) bool {
	if item == nil || len(rows) < 8 {
		return false
	}
	var sum float64
	for _, r := range rows {
		sum += r.Fee
	}
	sum = math.Round(sum*100) / 100
	// 容量电费不是「电费明细」里的行（它是输配容（需）量电费字段），
	// 所以对账目标要把容量电费与力调电费都扣掉。
	target := math.Round((item.GrandTotal-item.PfAdjustAmount-item.CapacityFee)*100) / 100
	reconciled := item.GrandTotal > 0 && math.Abs(sum-target) <= math.Max(1, math.Abs(item.GrandTotal)*0.005)
	if !reconciled && len(item.FeeItems) >= len(rows)/2 {
		// 既不与概况对账、模型又给出了差不多的行数：不冒险覆盖模型结果
		return false
	}
	out := make([]types.UtilityBillFeeItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, types.UtilityBillFeeItem{
			Category: r.Category,
			Name:     r.Name,
			Period:   r.Period,
			Qty:      r.Qty,
			Rate:     r.Rate,
			Fee:      r.Fee,
			BillFee:  r.Fee,
		})
	}
	item.FeeItems = out
	return true
}

// SumUtilityFeeRows 返回还原行的电费合计（供日志与单测对账）。
func SumUtilityFeeRows(rows []ParsedFeeRow) float64 {
	var sum float64
	for _, r := range rows {
		sum += r.Fee
	}
	return math.Round(sum*100) / 100
}

func utilityFeeCategoryOf(name string) string {
	if c, ok := utilityFeeCategoryByName[normalizeUtilityFeeName(name)]; ok {
		return c
	}
	return ""
}

// normalizeUtilityFeeName 全半角括号归一：账单里两种写法混用。
// normalizeUtilityFeeName 归一费用组成名：全半角括号、空白，以及同义写法。
//
// 「其它 / 其他」「因素 / 因数」在国网账单与人工配置里都出现过，
// 不归一会导致按名称匹配时整项落空（→ 0）。
func normalizeUtilityFeeName(name string) string {
	return strings.TrimSpace(strings.NewReplacer(
		"（", "(", "）", ")", " ", "", "其它", "其他", "因素", "因数",
	).Replace(name))
}

func utilityFeeRowKey(r ParsedFeeRow) string {
	return strings.Join([]string{
		r.Category, r.Name, r.Period,
		formatUtilityFeeNum(r.Qty), formatUtilityFeeNum(r.Rate), formatUtilityFeeNum(r.Fee),
	}, "|")
}

func formatUtilityFeeNum(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func parseUtilityFeeNumber(t string) (float64, bool) {
	t = strings.TrimSpace(strings.NewReplacer(",", "", "，", "", "￥", "", "¥", "").Replace(t))
	if t == "" || !utilityFeeNumberRe.MatchString(t) {
		return 0, false
	}
	v, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func pickUtilityFeeNum(list []float64, i int) float64 {
	if len(list) == 0 {
		return 0
	}
	if i < len(list) {
		return list[i]
	}
	return list[len(list)-1]
}

func pickUtilityFeeList(list []string, i int) string {
	if len(list) == 0 {
		return ""
	}
	if i < len(list) {
		return list[i]
	}
	return list[len(list)-1]
}

func utilityFeeRateEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
