// 发票「金额-税率-税额-价税合计」四元强校验 + 明细页内重建（第 27 轮）。
//
// 两个真机事故驱动本文件：
//
//  1. 多页汇总单的明细串页：19 页 19 票的文档里，P1/P2 的明细被写成了 P12/P14 的
//     数据行（198.09/17.83 的票挂着 6.45/0.19 的明细）。根因在确定性覆盖层
//     `OverlayDeterministicInvoiceFields`：它把全文正则抓到的明细行按**数组下标**
//     回填给发票（rows[i] -> invoices[i]），而该文档只有 2 页的明细行能被扁平正则
//     命中（其余页是「一格一列」的 HTML 表格）——于是第 12、14 页的数据行被写到了
//     第 1、2 张票上。覆盖层已改为**按物理页码配对**，本文件负责把已污染的存量数据
//     用同一套页面文本重新解析回来。
//
//  2. 税率档位错标：198.09 × 9% = 17.83，票面也是 9%，但税额被写进明细时带上了
//     邻页的 3%。税率必须由「金额 × 标准档 == 税额」反推验证，文本里读到的税率
//     只是候选值，两者冲突时以数学恒等式为准。
package service

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// 我国现行增值税税率/征收率档位（含零税率与免税）。
// 顺序按「常见优先」，仅用于同误差时的稳定取档，不改变判定结果。
var standardVATRateTiers = []float64{0.13, 0.09, 0.06, 0.03, 0.05, 0.01, 0}

const (
	// invoiceAmountTolerance 金额恒等式的允许尾差（元）。
	// 0.02 是「两列各四舍五入一次」的最大累积误差，再放宽就会把真错判成对。
	invoiceAmountTolerance = 0.02
	// invoiceRateTolerance 判定「当前税率与金额/税额不自洽」的阈值（元）。
	// 必须显著大于尾差：3% 与 9% 在 198.09 上相差 11.88 元，任何大于 0.05 的
	// 偏差都不可能由四舍五入产生。
	invoiceRateTolerance = 0.05
)

// InvoiceFinancialFix 记录一张票的纠偏结果（供日志/质量面板使用）。
type InvoiceFinancialFix struct {
	// RateBefore/RateAfter 票级税率纠偏前后（nil 表示原本缺失）。
	RateBefore *float64
	RateAfter  *float64
	// ItemRatesFixed 被同步纠偏的明细行数。
	ItemRatesFixed int
	// TotalFilled total 缺失时由 amount+tax 补齐。
	TotalFilled bool
	// AmountRebased 以价税合计为锚重算了金额/税额。
	AmountRebased bool
}

// Changed 报告本次是否真的改写了任何字段。
func (f InvoiceFinancialFix) Changed() bool {
	return f.RateAfter != nil || f.ItemRatesFixed > 0 || f.TotalFilled || f.AmountRebased
}

// nearestStandardVATRate 在标准档位里找「金额 × 档位」最接近税额的那一档。
// 返回 (档位, 偏差, 是否在容差内)。
func nearestStandardVATRate(amount, tax float64) (float64, float64, bool) {
	if amount <= 0 || tax < 0 {
		return 0, math.MaxFloat64, false
	}
	bestRate, bestErr := 0.0, math.MaxFloat64
	for _, r := range standardVATRateTiers {
		// 逐档算「理论税额」的四舍五入值，再与票面税额比 —— 票面税额本身就是
		// 两位小数，用原始乘积比会把 17.8281 与 17.83 的 0.0019 当成误差之一，
		// 虽然仍能取到 9%，但边界档（如 0.01）会因累积误差被误判。
		e := math.Abs(round2(amount*r) - tax)
		if e < bestErr-1e-9 {
			bestRate, bestErr = r, e
		}
	}
	return bestRate, bestErr, bestErr <= invoiceAmountTolerance
}

// rateMatchesAmount 报告给定税率能否解释票面的金额与税额。
func rateMatchesAmount(rate, amount, tax float64) bool {
	return math.Abs(round2(amount*rate)-tax) <= invoiceAmountTolerance
}

// ReconcileInvoiceFinancials 对单张发票执行四元强校验与自动纠偏：
//
//	(1) 税率反推：|金额 × 标准档 - 税额| <= 0.02 且当前税率偏差 > 0.05 → 改税率；
//	    票级改完后，逐行同步「自己的金额/税额对不上当前税率」的明细行；
//	(2) 价税合计恒等式：amount + tax == total（±0.02），以 total 为锚重算；
//	(3) total 缺失时由 amount + tax 补齐。
//
// 只在有**数学证据**时改写：证据不足一律保持原值，交给质量面板标红。
func ReconcileInvoiceFinancials(inv *InvoiceExtractionItem) InvoiceFinancialFix {
	var fix InvoiceFinancialFix
	if inv == nil {
		return fix
	}
	// --- (1) 税率反推（票级）---
	if inv.Amount != nil && inv.Tax != nil && *inv.Amount > 0 && *inv.Tax >= 0 {
		amount, tax := *inv.Amount, *inv.Tax
		// 税额为 0 时**不做反推**：0 = 0 × 任何档位，数学上无法区分「免税/零税率」
		// 与「税额没提到（模型返回 0 占位）」。此时把 9% 改成 0% 是静默毁数据，
		// 宁可留着原税率让质量面板标红（真机 1680 张里有税额为空写 0 的历史票）。
		if tax > 0 {
			if std, _, ok := nearestStandardVATRate(amount, tax); ok {
				cur := -1.0
				if inv.TaxRate != nil {
					cur = *inv.TaxRate
				}
				// 当前税率缺失，或明显解释不了票面税额 → 纠偏到数学验证通过的档位。
				if cur < 0 || math.Abs(round2(amount*cur)-tax) > invoiceRateTolerance {
					before := inv.TaxRate
					rate := std
					inv.TaxRate = &rate
					fix.RateBefore = before
					fix.RateAfter = &rate
				}
			}
		}
	}
	// --- (1b) 明细税率同步 ---
	// 只在「票级税率已可信」时逐行纠正：一票多档是合法形态（13% 货物 + 9% 运费），
	// 所以每行都用**自己的**金额/税额反推，绝不把票级税率强加给所有行。
	// 单行明细且票级税率已纠偏时，该行的正确税率就是票级税率（一行 = 整张票）。
	fix.ItemRatesFixed = reconcileInvoiceItemRates(inv)

	// --- (2)(3) 价税合计恒等式 ---
	beforeAmount, beforeTax := inv.Amount, inv.Tax
	if inv.TotalAmount == nil && inv.Amount != nil && inv.Tax != nil {
		total := round2(*inv.Amount + *inv.Tax)
		inv.TotalAmount = &total
		fix.TotalFilled = true
	}
	if inv.TotalAmount != nil && *inv.TotalAmount > 0 {
		hadTotal := !fix.TotalFilled
		reconcileInvoiceAmounts(inv)
		if hadTotal && (inv.Amount != beforeAmount || inv.Tax != beforeTax) {
			fix.AmountRebased = true
		}
	}
	return fix
}

// reconcileInvoiceItemRates 逐行纠正明细税率，返回纠正行数。
//
// 判定顺序：
//  1. 该行自己的 amount/tax_amount 能反推出标准档，且当前税率解释不了 → 纠偏；
//  2. 单行明细 + 票级税率可用 + 行金额等于票面金额（同一张票只有一行）→ 用票级税率；
//  3. 其余（多行、缺金额、缺税额）保持原样。
func reconcileInvoiceItemRates(inv *InvoiceExtractionItem) int {
	if inv == nil || len(inv.Items) == 0 {
		return 0
	}
	fixed := 0
	singleRowMatchesTicket := len(inv.Items) == 1 &&
		inv.Amount != nil && inv.Items[0].Amount != nil &&
		math.Abs(*inv.Items[0].Amount-*inv.Amount) <= invoiceAmountTolerance
	for i := range inv.Items {
		line := &inv.Items[i]
		if line.Amount == nil || line.TaxAmt == nil || *line.Amount <= 0 || *line.TaxAmt < 0 {
			// 金额/税额不可用时，仅在「单行且等于票面」这种强约束下借票级税率。
			if singleRowMatchesTicket && inv.TaxRate != nil {
				if line.TaxRate == nil || math.Abs(*line.TaxRate-*inv.TaxRate) > 1e-6 {
					rate := *inv.TaxRate
					line.TaxRate = &rate
					fixed++
				}
			}
			continue
		}
		amount, tax := *line.Amount, *line.TaxAmt
		// 行税额为 0 同样不反推（免税行 / 税额未提取都会是 0），见票级同名注释。
		if tax <= 0 {
			continue
		}
		std, _, ok := nearestStandardVATRate(amount, tax)
		if !ok {
			continue
		}
		cur := -1.0
		if line.TaxRate != nil {
			cur = *line.TaxRate
		}
		if cur < 0 || math.Abs(round2(amount*cur)-tax) > invoiceAmountTolerance {
			rate := std
			line.TaxRate = &rate
			fixed++
		}
	}
	return fixed
}

// ReconcileInvoiceFinancialsForDocument 对整份文档的发票执行四元强校验，
// 返回按发票下标定位的告警（仅「无法自洽闭环」的记录）。
func ReconcileInvoiceFinancialsForDocument(invs []InvoiceExtractionItem) []ExtractFieldIssue {
	if len(invs) == 0 {
		return nil
	}
	issues := make([]ExtractFieldIssue, 0, 2)
	for i := range invs {
		fix := ReconcileInvoiceFinancials(&invs[i])
		if fix.Changed() {
			continue
		}
		if msg := invoiceFinancialConflict(&invs[i]); msg != "" {
			issues = append(issues, ExtractFieldIssue{
				Index:    i,
				Field:    "amount",
				Kind:     "mismatch",
				Detail:   msg,
				Severity: issueSeverityWarn,
			})
		}
	}
	return issues
}

// invoiceFinancialConflict 判断一张票是否「四元无法闭环」，返回人话说明。
// 只在确实无法用标准档位解释时返回非空，避免给正常票刷告警。
func invoiceFinancialConflict(inv *InvoiceExtractionItem) string {
	if inv == nil || inv.Amount == nil || inv.Tax == nil {
		return ""
	}
	amount, tax := *inv.Amount, *inv.Tax
	if amount <= 0 {
		return ""
	}
	// 一票多档（13% 设备 + 9% 基础电信 + 6% 增值电信）时，票级税率只是「金额最大
	// 那一行的税率」，用它乘总金额天然不等于总税额 —— 这不是错，不能告警。
	if invoiceDistinctItemRates(inv) > 1 {
		// 仍要校验三件套恒等式。
		if inv.TotalAmount != nil && math.Abs(round2(amount+tax)-*inv.TotalAmount) > invoiceAmountTolerance {
			return fmt.Sprintf("金额 %.2f + 税额 %.2f ≠ 价税合计 %.2f，请核对",
				amount, tax, *inv.TotalAmount)
		}
		return ""
	}
	if inv.TaxRate != nil && !rateMatchesAmount(*inv.TaxRate, amount, tax) {
		if _, _, ok := nearestStandardVATRate(amount, tax); !ok {
			return fmt.Sprintf("金额 %.2f × 税率 %.2f%% ≠ 税额 %.2f，且无法匹配标准税率档位，请核对",
				amount, *inv.TaxRate*100, tax)
		}
	}
	if inv.TotalAmount != nil && math.Abs(round2(amount+tax)-*inv.TotalAmount) > invoiceAmountTolerance {
		return fmt.Sprintf("金额 %.2f + 税额 %.2f ≠ 价税合计 %.2f，请核对",
			amount, tax, *inv.TotalAmount)
	}
	return ""
}

// invoiceDistinctItemRates 统计明细行里出现过的不同税率个数。
func invoiceDistinctItemRates(inv *InvoiceExtractionItem) int {
	if inv == nil || len(inv.Items) == 0 {
		return 0
	}
	seen := map[float64]struct{}{}
	for _, it := range inv.Items {
		if it.TaxRate == nil {
			continue
		}
		seen[math.Round(*it.TaxRate*10000)/10000] = struct{}{}
	}
	return len(seen)
}

// --- 明细页内重建 ---

// compactDateOnlyRe 匹配「整串就是 8 位紧凑日期」。
var compactDateOnlyRe = regexp.MustCompile(`^\d{8}$`)

// NormalizeInvoiceItemDates 把明细行里的紧凑日期（20261031）归一成 2026-10-31。
//
// 三套提取规则对通行日期的写法不一致：HTML 规则历史上直接落紧凑 8 位，扁平规则
// 落带分隔的形态，模型两种都会输出。同一列出现两种格式后，日期区间筛选/排序
// 都认不出来，抽屉里也显示成 "20261019"。这里统一成 YYYY-MM-DD（幂等）。
// 返回被改写的行数。
func NormalizeInvoiceItemDates(invs []InvoiceExtractionItem) int {
	fixed := 0
	for i := range invs {
		for j := range invs[i].Items {
			extra := invs[i].Items[j].Extra
			if extra == nil {
				continue
			}
			for _, key := range []string{"travel_from", "travel_to"} {
				v, ok := extra[key].(string)
				if !ok || !compactDateOnlyRe.MatchString(v) {
					continue
				}
				extra[key] = mdDateFromCompact(v)
				fixed++
			}
		}
	}
	return fixed
}

// InvoiceItemExtrasIncomplete 报告「通行费明细的分类子列不完整」。
//
// 真机形态：HTML 规则旧版在合并格里取到车牌后直接 continue，同一格里的
// 「通行日期起」永远取不到 —— 全库 1530 张通行费票里有 284 张该列为空。
// 规则已修（取到车牌后继续解析日期），这里负责把存量按页内原文补齐。
func InvoiceItemExtrasIncomplete(inv *InvoiceExtractionItem) bool {
	if inv == nil || len(inv.Items) != 1 {
		return false
	}
	line := inv.Items[0]
	if !strings.Contains(line.Name, "通行费") {
		return false
	}
	plate := extraString(line.Extra, "plate_no")
	if plate == "" {
		return true
	}
	return extraString(line.Extra, "travel_from") == "" || extraString(line.Extra, "travel_to") == ""
}

// InvoiceItemsCrossPageSuspect 判断「单行明细是否串到了文档里另一张票的数据」。
//
// 只认**铁证**：该票只有一行明细，行金额与票面金额不符，且这个行金额恰好等于
// 文档里另一张票的票面金额 —— 这正是「P2 的明细写着 P14 的 6.45」的形态。
//
// 为什么不用「Σitems.amount != 票面金额」一律判错：折扣票、红冲票、源文件里
// 重复行/合计行的金额本来就与票面不等（真机 1680 张里这类占绝大多数），
// 一律告警会重演第 25 轮「74 份历史记录全被标红」的误报事故。
func InvoiceItemsCrossPageSuspect(invs []InvoiceExtractionItem, idx int) bool {
	if idx < 0 || idx >= len(invs) {
		return false
	}
	inv := &invs[idx]
	if len(inv.Items) != 1 || inv.Amount == nil || inv.Items[0].Amount == nil {
		return false
	}
	itemAmount := *inv.Items[0].Amount
	if math.Abs(itemAmount-*inv.Amount) <= invoiceAmountTolerance {
		return false
	}
	for j := range invs {
		if j == idx || invs[j].Amount == nil || *invs[j].Amount <= 0 {
			continue
		}
		if math.Abs(*invs[j].Amount-itemAmount) <= invoiceAmountTolerance {
			return true
		}
	}
	return false
}

// InvoiceItemsAmountMismatch 报告「明细合计与票面金额/税额不一致」。
// 用于覆盖层判断「已有明细是否可信」（不值得为它单独告警，见上）。
func InvoiceItemsAmountMismatch(inv *InvoiceExtractionItem) bool {
	if inv == nil || len(inv.Items) == 0 {
		return false
	}
	sumAmount, sumTax := 0.0, 0.0
	hasAmount, hasTax := false, false
	for _, it := range inv.Items {
		if it.Amount != nil {
			sumAmount += *it.Amount
			hasAmount = true
		}
		if it.TaxAmt != nil {
			sumTax += *it.TaxAmt
			hasTax = true
		}
	}
	if hasAmount && inv.Amount != nil && math.Abs(round2(sumAmount)-*inv.Amount) > invoiceAmountTolerance {
		return true
	}
	if hasTax && inv.Tax != nil && math.Abs(round2(sumTax)-*inv.Tax) > invoiceAmountTolerance {
		return true
	}
	// 明细一行金额都没有，但票面有金额 —— 也是明细不完整（不算串页，但同样要修）。
	if !hasAmount && inv.Amount != nil && *inv.Amount > 0 {
		return true
	}
	return false
}

// PageTexts 把拼装文本按物理页码切开，返回 page -> 该页文本。
//
// 切片边界来自 BuildExtractSourceText 的 PageMarker：相邻两个标记之间属于前一个
// 标记的页码。首个标记之前的内容（汇总页/封面）页码为 0，不参与票据页匹配。
func (s ExtractSourceText) PageTexts() map[int]string {
	out := map[int]string{}
	if len(s.Markers) == 0 {
		return out
	}
	for i, m := range s.Markers {
		if m.Page <= 0 {
			continue
		}
		start := m.Offset
		end := len(s.Content)
		if i+1 < len(s.Markers) {
			end = s.Markers[i+1].Offset
		}
		if start < 0 || start >= len(s.Content) || end <= start {
			continue
		}
		if end > len(s.Content) {
			end = len(s.Content)
		}
		out[m.Page] += s.Content[start:end]
	}
	return out
}

// invoicePageOf 取发票的物理页码（PageNo 优先，缺失时退回 Page 序号）。
func invoicePageOf(inv *InvoiceExtractionItem) int {
	if inv == nil {
		return 0
	}
	if inv.PageNo > 0 {
		return inv.PageNo
	}
	return inv.Page
}

// invoicePagesDistinct 报告「页码能否唯一标识每张票」。
//
// 只有在 P1..PN 一页一票时，才能用页码把「明细数据行」与「发票」配对；
// 纯扁平版式常把多张票推成同一页（靠票面页眉推页码时尤其如此），
// 此时页码无法区分发票，必须退回「行数 == 发票数」的严格 1:1。
func invoicePagesDistinct(invs []InvoiceExtractionItem) bool {
	if len(invs) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(invs))
	for i := range invs {
		page := invoicePageOf(&invs[i])
		if page <= 0 {
			return false
		}
		if _, dup := seen[page]; dup {
			return false
		}
		seen[page] = struct{}{}
	}
	return true
}

// ReconcileInvoiceItemsWithSource 用**每张票自己那一页**的原文重建明细行。
//
// 这是串页污染的根治手段：明细只能来自本页文本块，绝不跨页借用。
// 只在该页能确定性解析出**唯一一行**通行费数据时才重建；解析不到或多行歧义时
// 保持原值（宁可留一条质量告警，也不能把别页的行写进来）。
// 返回被重建的发票数。
func ReconcileInvoiceItemsWithSource(invs []InvoiceExtractionItem, src ExtractSourceText) int {
	if len(invs) == 0 || !src.HasPages() {
		return 0
	}
	pages := src.PageTexts()
	if len(pages) == 0 {
		return 0
	}
	repaired := 0
	for i := range invs {
		page := invoicePageOf(&invs[i])
		if page <= 0 {
			continue
		}
		text := pages[page]
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !rebuildInvoiceItemsFromPageText(&invs[i], text) {
			continue
		}
		repaired++
	}
	return repaired
}

// rebuildInvoiceItemsFromPageText 用单页文本重建明细，返回是否改写。
//
// 数据源优先级：
//  1. 该页的 HTML/Markdown 表格数据行（`collectHTMLTollRows`，与提取规则同源）；
//  2. 该页的扁平数据行（`flatItemRowRe`，整行压进一个单元格的形态）。
//
// 只有「恰好一行」才重建 —— 一行对应整张票，这是多页汇总单的固定版式；
// 两行以上无法确定归属（可能一页两票），交给人工。
func rebuildInvoiceItemsFromPageText(inv *InvoiceExtractionItem, pageText string) bool {
	if inv == nil {
		return false
	}
	rows := collectHTMLTollRows(pageText)
	if len(rows) == 0 {
		if m := flatItemRowRe.FindStringSubmatch(pageText); len(m) > 8 {
			rows = []htmlTollRow{htmlTollRowFromFlatMatch(m)}
		}
	}
	if len(rows) != 1 {
		return false
	}
	row := rows[0]
	// 该行必须能解释票面金额或税额之一，否则可能是同页的「合计」行/串行。
	// 用票面金额做锚：金额对不上且税额也对不上，就不是这张票的行。
	amountOK := row.Amount != nil && inv.Amount != nil &&
		math.Abs(*row.Amount-*inv.Amount) <= invoiceAmountTolerance
	taxOK := row.TaxAmount != nil && inv.Tax != nil &&
		math.Abs(*row.TaxAmount-*inv.Tax) <= invoiceAmountTolerance
	if !amountOK && !taxOK {
		return false
	}
	line := InvoiceExtractionLine{Name: coalesceStr(row.ItemName, "通行费"), Amount: row.Amount, TaxRate: row.TaxRate, TaxAmt: row.TaxAmount}
	extra := map[string]any{}
	if row.Plate != "" {
		extra["plate_no"] = row.Plate
	}
	if row.Vehicle != "" {
		extra["vehicle_type"] = row.Vehicle
	}
	if row.From != "" {
		extra["travel_from"] = mdDateFromCompact(row.From)
	}
	if row.To != "" {
		extra["travel_to"] = mdDateFromCompact(row.To)
	}
	if len(extra) > 0 {
		line.Extra = extra
	}
	// 已经是同一条数据（金额/税额/车牌/日期都一致）且备注也同步时不重写：
	// 避免每次启动都产生写库。
	sameItems := len(inv.Items) == 1 && invoiceLineSameAsRow(inv.Items[0], line)
	nextRemark := refreshTollRemark(inv.Remark, row)
	sameRemark := nextRemark == inv.Remark
	if sameItems && sameRemark {
		return false
	}
	if !sameItems {
		inv.Items = []InvoiceExtractionLine{line}
	}
	inv.Remark = nextRemark
	// 明细是票面的行级明细：票面金额/税额/税率缺失时用行值补齐（不覆盖已有值）。
	if inv.Amount == nil && line.Amount != nil {
		v := *line.Amount
		inv.Amount = &v
	}
	if inv.Tax == nil && line.TaxAmt != nil {
		v := *line.TaxAmt
		inv.Tax = &v
	}
	if inv.TaxRate == nil && line.TaxRate != nil {
		v := *line.TaxRate
		inv.TaxRate = &v
	}
	if row.Plate != "" {
		if inv.Fields == nil {
			inv.Fields = map[string]any{}
		}
		if _, ok := inv.Fields["车牌号"]; !ok {
			inv.Fields["车牌号"] = row.Plate
		}
	}
	return true
}

// refreshTollRemark 用本页数据行重建备注里的「车牌号 / 通行日期起止」三段。
//
// 三套规则历史写法不一致：HTML 规则把日期写成紧凑 8 位、且「通行日期起」常为空
// （取到车牌就 continue 的老 bug），重建明细后备注仍是旧的 → 抽屉里同一张票
// 「明细写 2026-10-13、备注写 通行日期起：」自相矛盾。这里只替换这三段，
// 备注里的其它内容（业务流水号/购方地址…）原样保留。
func refreshTollRemark(remark string, row htmlTollRow) string {
	if !strings.Contains(remark, "车牌号：") {
		return remark
	}
	kept := make([]string, 0, 4)
	for _, part := range strings.Split(remark, "；") {
		p := strings.TrimSpace(part)
		if strings.HasPrefix(p, "车牌号：") || strings.HasPrefix(p, "通行日期起：") || strings.HasPrefix(p, "通行日期止：") {
			continue
		}
		if p != "" {
			kept = append(kept, p)
		}
	}
	head := []string{
		"车牌号：" + row.Plate,
		"通行日期起：" + mdDateFromCompact(row.From),
		"通行日期止：" + mdDateFromCompact(row.To),
	}
	return strings.Join(append(head, kept...), "；")
}

// InvoiceTollRemarkStale 报告「通行费备注里的车牌/通行日期与明细不一致」。
// 存量的备注由旧规则写入（起日为空的紧凑日期），需要按明细重刷一次。
func InvoiceTollRemarkStale(inv *InvoiceExtractionItem) bool {
	if inv == nil || len(inv.Items) != 1 || !strings.Contains(inv.Remark, "车牌号：") {
		return false
	}
	line := inv.Items[0]
	want := "车牌号：" + extraString(line.Extra, "plate_no") +
		"；通行日期起：" + extraString(line.Extra, "travel_from") +
		"；通行日期止：" + extraString(line.Extra, "travel_to")
	return !strings.Contains(inv.Remark, want)
}

// invoiceLineSameAsRow 判断已有明细行是否就是这条数据行。
func invoiceLineSameAsRow(line InvoiceExtractionLine, want InvoiceExtractionLine) bool {
	if (line.Amount == nil) != (want.Amount == nil) || (line.TaxAmt == nil) != (want.TaxAmt == nil) {
		return false
	}
	if line.Amount != nil && math.Abs(*line.Amount-*want.Amount) > 1e-9 {
		return false
	}
	if line.TaxAmt != nil && math.Abs(*line.TaxAmt-*want.TaxAmt) > 1e-9 {
		return false
	}
	// 分类子列必须一起比：只比金额/税额会让「缺通行日期起」的旧明细被判成
	// 「已一致」，重建被跳过（真机 284 张通行费票因此一直补不上）。
	return extraFingerprint(line.Extra) == extraFingerprint(want.Extra)
}

// extraFingerprint 把明细行关心的分类子列拼成可比较的指纹。
func extraFingerprint(extra map[string]any) string {
	var b strings.Builder
	for _, k := range []string{"plate_no", "vehicle_type", "travel_from", "travel_to"} {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(extraString(extra, k))
		b.WriteByte(';')
	}
	return b.String()
}

// extraString 读取 extra 里的字符串值。
func extraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	if v, ok := extra[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// htmlTollRowFromFlatMatch 把 flatItemRowRe 的匹配结果转成 htmlTollRow，
// 让「HTML 表格行」与「扁平数据行」共用同一套明细构造逻辑。
func htmlTollRowFromFlatMatch(m []string) htmlTollRow {
	row := htmlTollRow{ItemName: strings.TrimSpace(m[1])}
	row.Plate = strings.TrimSpace(m[2])
	row.Vehicle = strings.TrimSpace(m[3])
	row.From = mdDateFromCompact(m[4])
	row.To = mdDateFromCompact(m[5])
	row.Amount = parseAmountToFloat(m[6])
	row.TaxRate = parsePercentToFloat(m[7])
	row.TaxAmount = parseAmountToFloat(m[8])
	return row
}
