package service

// 成品油（石化/石油）增值税专用发票的确定性解析（第 24 轮任务二）。
//
// 为什么需要独立一套规则：
//
//	成品油票的明细表是 **9 列固定列序**（项目名称 / 规格型号 / 单位 / 数量 /
//	单价 / 金额 / 税率 / 税额），且存在两类特殊形态：
//	  ① 折扣行：只有「项目名称 + 负金额 + 税率 + 负税额」，规格/单位/数量/单价全空；
//	  ② 跨行括号：`车用汽油(VI` 与下一行的 `B)` 是同一个规格型号，被 PDF 换行切断。
//
//	既有三套规则（Line / MarkdownTable / FlatLayout）都按「名称在前、金额在后」
//	猜列位，遇到①会把负金额当成「项目名称」、遇到②会把括号尾巴当独立行；
//	HTML 表格规则是给通行费（车牌/车型/通行日期）写的，列语义完全不同。
//	实测（2026.9-10油费发票.pdf）三套规则全部未命中，整份文档落到 LLM ——
//	而模型在这种多行 + 折扣行的表里会算错合计（真机 P2 被写成 3665.38，
//	票面合计是 6690.23）。
//
// 本规则的核心是**从右往左锚定数值列**（见 parseFuelDetailRow）：
//	最右侧依次是 税额 → 税率 → 金额，再往左（仅当有正数数量与单价时）
//	是 单价 → 数量 → 单位 → 规格型号，最左侧整体保留为项目名称。
//	折扣行只有右侧三段，左侧全空 —— 天然不会错位。

import (
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	// fuelInvNoLabelRe 「发票号码：」+ 20 位全电票号（成品油票都是全电发票）。
	//
	// **必须绑定「发票号码：」上下文**：票面备注里还有 20 位的银行账号
	// （`银行账号:50050118370000000047`），裸匹配 20 位数字会把它当成第 4 张票，
	// 生成幽灵 `P4` 记录（真机图 1 第 20 行就是这么来的）。
	fuelInvNoLabelRe = regexp.MustCompile(`发票号码\s*[:：]\s*([0-9]{20})`)

	// fuelTotalLabelRe 「合 计 ¥X ¥Y」——票面合计行（金额 + 税额）。
	//
	// 空格允许出现（票面渲染成 `合 计` 或 `合计`），金额可带千分位。
	// fuelTotalLabelRe 「合 计 ¥X ¥Y」——票面合计行（金额 + 税额）。
	//
	// 必须容忍：① 空格（票面渲染成 `合 计`）；② 千分位逗号；
	// ③ 全角/半角 ¥ 混用；④ ¥ 与数字间的空白。
	// 真机形态：`<td colspan="6">合计</td><td colspan="2">¥51989.49</td><td>¥6758.63</td>`
	// —— 标签与金额在**不同的 td** 里，所以不能要求「标签紧跟 ¥」，
	// 而是「合计之后、下一行之前」的两个带 ¥ 的金额。
	fuelTotalLabelRe = regexp.MustCompile(`合\s*计[\s\S]{0,80}?[¥￥]\s*([0-9,]+(?:\.[0-9]+)?)[\s\S]{0,40}?[¥￥]\s*([0-9,]+(?:\.[0-9]+)?)`)

	// fuelTotalLowerRe 「(小写) ¥X」——价税合计。
	fuelTotalLowerRe = regexp.MustCompile(`(?:\(小写\)|（小写）|小写)\s*¥?\s*([0-9,]+(?:\.[0-9]+)?)`)

	// fuelIssuerRe 开票人（允许逐字断行）。
	fuelIssuerRe = regexp.MustCompile(`开\s*票\s*人\s*[:：]\s*([\p{Han}·]{2,8})`)

	// fuelUnitRe 计量单位白名单。
	//
	// 用白名单而不是「任意 1~2 个汉字」：规格型号里的 `(VI)` `0#柴油` 都含字母数字，
	// 而项目名称里常有「升」以外的汉字；白名单能把「单位」与「规格型号」稳稳分开。
	fuelUnitRe = regexp.MustCompile(`^(?:升|吨|个|项|次|度|桶|公斤|千克|kg|KG|L|立方|m3|米|件|套|台|张|本|箱|包)$`)

	// fuelRateRe 税率格：`13%` / `9%` / `3%` / `免税` / `不征税`。
	fuelRateRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*%$`)

	// fuelSpecRe 规格型号：`0#柴油` / `92#汽油` / `95#汽油` / `车用柴油(VI)`。
	//
	// 只要含 `#` 或含括号英文/数字，就大概率是规格而不是单位 —— 但仍以
	// 「单位白名单」为主判据，本正则只用于折扣行之后的补位判断。
	fuelSpecRe = regexp.MustCompile(`^(?:[0-9]+#[^\s]*|.*\([A-Za-z0-9]+(?:[A-Za-z0-9.)]*)?)$`)

	// fuelProjectAnchorRe 明细行的行首锚点：`*类别*` 形态的项目名称开头。
	//
	// 用于切分「整行压进一个 colspan 格」的形态（真机 P3：表头 + 两条明细
	// 全塞进一个 colspan=9 的格子里）。
	fuelProjectAnchorRe = regexp.MustCompile(`\*[^*\s]+\*`)

	// fuelContinuationRe 跨行括号的**尾巴**：单独一格的 `B)` / `VI)` / `)`。
	//
	// 只认「括号收尾」形态，避免把真正的明细行误并。
	fuelContinuationRe = regexp.MustCompile(`^[A-Za-z0-9]*[)）]$`)
)

// fuelSegmentAnchorRe 是成品油票的分段锚点。
//
// 用「电子发票（增值税专用发票）」标题 + 「成品油」标识定位，再用发票号码切段：
// 标题保证我们只处理成品油票（其它增值税专用发票不该走本规则）。
var fuelSegmentAnchorRe = regexp.MustCompile(`成品油[\s\S]{0,200}?发票号码\s*[:：]\s*[0-9]{20}`)

// ExtractInvoicesByFuelRules 解析成品油增值税专用发票（HTML 表格版）。
//
// 返回 (result, true) 表示本规则命中；命中判据是「至少一张票拿到了票号 +
// 票面合计」。**金额/税额一律以票面合计为准**，绝不用明细行之和覆盖 ——
// 明细可能因折扣行/跨行截断而残缺，票面合计才是财务口径。
func ExtractInvoicesByFuelRules(content string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "<tr") {
		return nil, false
	}
	if !strings.Contains(content, "成品油") {
		return nil, false
	}
	locs := fuelInvNoLabelRe.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		return nil, false
	}
	invs := make([]InvoiceExtractionItem, 0, len(locs))
	for i, loc := range locs {
		// 段落 = [本票号码行, 下一票号码行)。
		//
		// **绝不往前扩**：早期实现前扩 200 字想覆盖「成品油」标题与购销方，
		// 结果把**上一张票的尾部**（合计行 + 开票人）也吃进来 ——
		// P2 的开票人因此变成 P1 的「周琳杰」（真机实测）。
		// 购销方与合计都在号码行**之后**的表格里，不需要前扩。
		start := loc[0]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		inv, ok := parseFuelInvoiceSegment(content[start:end], loc)
		if !ok {
			continue
		}
		inv.Page = len(invs) + 1
		invs = append(invs, inv)
	}
	if len(invs) == 0 {
		return nil, false
	}
	return &InvoiceExtractionResult{Kind: "invoice", Invoices: invs}, true
}

// ExtractInvoicesByFuelRulesFromChunks 按页序拼接 chunks 后执行本规则。
func ExtractInvoicesByFuelRulesFromChunks(chunks []*types.Chunk) (*InvoiceExtractionResult, bool) {
	src := BuildExtractSourceText(chunks)
	if strings.TrimSpace(src.Content) == "" {
		return nil, false
	}
	res, ok := ExtractInvoicesByFuelRules(src.Content)
	if ok && res != nil {
		AssignPhysicalPages(src, res.Invoices)
	}
	return res, ok
}

// parseFuelInvoiceSegment 解析单张成品油票。
func parseFuelInvoiceSegment(seg string, noLoc []int) (InvoiceExtractionItem, bool) {
	var inv InvoiceExtractionItem
	// 票号：用当前段的**首个**「发票号码：」标签（noLoc 是全文偏移，
	// 段落被往前扩过 200 字，不能直接切全文，重新在本段里找）。
	if m := fuelInvNoLabelRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.InvoiceNo = strings.TrimSpace(m[1])
	}
	if inv.InvoiceNo == "" {
		return inv, false
	}
	// **防幽灵票**：票号若同时出现在备注的银行账号上下文里，说明这是误匹配。
	// 本规则已用「发票号码：」绑定，理论上不会命中；这里再挡一层，
	// 因为 `银行账号:50050118370000000047` 恰好 20 位，是同类事故的高危值。
	if isBankAccountContext(seg, inv.InvoiceNo) {
		return inv, false
	}

	inv.InvoiceDate = htmlInvoiceDateOf(seg)
	inv.InvoiceType = "增值税专用发票"

	// 票面合计：`合 计 ¥金额 ¥税额`。**这是权威口径**，不用明细之和。
	if m := fuelTotalLabelRe.FindStringSubmatch(seg); len(m) > 2 {
		if v := parseAmountToFloat(m[1]); v != nil {
			inv.Amount = v
		}
		if v := parseAmountToFloat(m[2]); v != nil {
			inv.Tax = v
		}
	}
	// 价税合计：`(小写) ¥X`；取不到时用 合计金额 + 合计税额 反算。
	if m := fuelTotalLowerRe.FindStringSubmatch(seg); len(m) > 1 {
		if v := parseAmountToFloat(m[1]); v != nil {
			inv.TotalAmount = v
		}
	}
	if inv.TotalAmount == nil && inv.Amount != nil && inv.Tax != nil {
		v := roundMoney(*inv.Amount + *inv.Tax)
		inv.TotalAmount = &v
	}
	// 税率：票面合计不带税率，取明细行的税率（多税率票取首个非零）。
	// 若明细全空则留空（不猜）。

	// 购销方：就近配对（与通行费规则同一套实现，已处理跨格/跨行形态）
	inv.BuyerName, inv.BuyerTaxNo = htmlPartyNear(seg, "购买方")
	if inv.BuyerName == "" {
		inv.BuyerName, inv.BuyerTaxNo = htmlPartyNear(seg, "购")
	}
	inv.SellerName, inv.SellerTaxNo = htmlPartyNear(seg, "销售方")
	if inv.SellerName == "" {
		inv.SellerName, inv.SellerTaxNo = htmlPartyNear(seg, "销")
	}
	// 开票人：票面底部「开票人：X」，允许逐字断行。
	if m := fuelIssuerRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.Issuer = mdCleanText(m[1])
	}

	// 明细：逐行解析 + 跨行括号合并
	inv.Items = parseFuelDetailRows(seg)
	// 税率取明细里出现最多的那一档（成品油票通常单一税率）
	if inv.TaxRate == nil {
		inv.TaxRate = dominantFuelRate(inv.Items)
	}
	return inv, true
}

// isBankAccountContext 判断票号是否出现在「银行/账号/帐号/卡号」上下文里。
//
// 判据：票号字符串在段落中出现，且**某次出现的前 30 字符内**含银行类关键词。
// 票面真正的「发票号码：」出现在票头，前面不会有这些词。
func isBankAccountContext(seg, no string) bool {
	idx := 0
	for {
		pos := strings.Index(seg[idx:], no)
		if pos < 0 {
			return false
		}
		abs := idx + pos
		from := abs - 30
		if from < 0 {
			from = 0
		}
		before := seg[from:abs]
		// 真正的票号前面是「发票号码：」
		if strings.Contains(before, "发票号码") {
			idx = abs + len(no)
			continue
		}
		for _, kw := range []string{"银行", "账号", "帐号", "卡号", "开户"} {
			if strings.Contains(before, kw) {
				return true
			}
		}
		idx = abs + len(no)
	}
}

// fuelDetailRow 是一行明细的确定性解析结果。
type fuelDetailRow struct {
	Name     string
	Spec     string
	Unit     string
	Qty      *float64
	Price    *float64
	Amount   *float64
	TaxRate  *float64
	TaxAmt   *float64
	Discount bool // 折扣行（只有右侧三段）
}

// parseFuelDetailRows 从段落的 HTML 表格里抽明细行。
func parseFuelDetailRows(seg string) []InvoiceExtractionLine {
	rows := collectHTMLCells(seg)
	merged := mergeFuelContinuations(rows)
	out := make([]InvoiceExtractionLine, 0, len(merged))
	for _, cells := range merged {
		// **整行压进单格**（真机 P3 形态）：
		//   <tr><td colspan="9">项目名称 规格型号 单位 数量 单价 金额 税率/征收率 税额
		//   *柴油*0号 车用柴油(VI) 0#柴油 升 302.42 7.49649494081 2267.09 13% 294.72
		//   *柴油*0号 车用柴油(VI) -54.79 13% -7.12</td></tr>
		// MinerU 把「表头 + 两行明细」全塞进一个 colspan=9 格。必须按 `*类别*`
		// 锚点切成「表头 / 明细1 / 明细2」，否则整格既不是表头也不是明细，直接丢弃
		// （真机 P3 items=0，明细列全空）。
		if len(cells) == 1 {
			if sub := splitFuelMergedRow(cells[0]); len(sub) > 0 {
				for _, one := range sub {
					if r, ok := parseFuelDetailRow(one); ok {
						out = append(out, fuelLineOf(r))
					}
				}
				continue
			}
		}
		r, ok := parseFuelDetailRow(cells)
		if !ok {
			continue
		}
		out = append(out, fuelLineOf(r))
	}
	return out
}

// fuelLineOf 把 fuelDetailRow 转成对外的明细行结构。
func fuelLineOf(r fuelDetailRow) InvoiceExtractionLine {
	return InvoiceExtractionLine{
		Name:    r.Name,
		Spec:    r.Spec,
		Unit:    r.Unit,
		Qty:     r.Qty,
		Price:   r.Price,
		Amount:  r.Amount,
		TaxRate: r.TaxRate,
		TaxAmt:  r.TaxAmt,
	}
}

// splitFuelMergedRow 把「整行压进一个 colspan 格」的串切成多行虚拟单元格。
//
// 输入（真机 P3 原文，一行内含表头 + 两条明细）：
//
//	`项目名称 规格型号 单位 数量 单价 金额 税率/征收率 税额*柴油*0号 车用柴油(VI)
//	 0#柴油 升 302.42 7.49649494081 2267.09 13% 294.72*柴油*0号 车用柴油(VI) -54.79 13% -7.12`
//
// 切法：以 `*类别*` 为行首锚点 —— 每个明细行的项目名称都以 `*` 开头。
// 表头段（第一个 `*` 之前）直接丢弃（parseFuelDetailRow 也认不出它）。
//
// 为什么不在切分时按列数硬切：折扣行只有 5 个 token（名称/规格/金额/税率/税额），
// 按固定列数切会把后续行整体错位。以行首锚点切是唯一稳的办法。
func splitFuelMergedRow(raw string) [][]string {
	t := strings.TrimSpace(raw)
	if !strings.Contains(t, "*") {
		return nil
	}
	locs := fuelProjectAnchorRe.FindAllStringIndex(t, -1)
	if len(locs) == 0 {
		return nil
	}
	out := make([][]string, 0, len(locs))
	for i, loc := range locs {
		end := len(t)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := strings.TrimSpace(t[loc[0]:end])
		if seg == "" {
			continue
		}
		cells := make([]string, 0, 9)
		for _, f := range strings.Fields(seg) {
			cells = append(cells, f)
		}
		if len(cells) > 0 {
			out = append(out, cells)
		}
	}
	return out
}

// collectHTMLCells 把段落里的每个 <tr> 拆成单元格文本数组。
func collectHTMLCells(seg string) [][]string {
	out := make([][]string, 0, 16)
	for _, tr := range htmlTrRe.FindAllStringSubmatch(seg, -1) {
		cells := make([]string, 0, 9)
		for _, c := range htmlTdRe.FindAllStringSubmatch(tr[1], -1) {
			cells = append(cells, strings.TrimSpace(stripHTMLTags(c[1])))
		}
		if len(cells) == 0 {
			continue
		}
		out = append(out, cells)
	}
	return out
}

// mergeFuelContinuations 合并跨行的括号尾巴。
//
// 真机形态（图 3 原文）：
//
//	<tr><td>*汽油*92号</td><td>车用汽油(VI)</td>...<td>882.79</td><td>13%</td><td>114.76</td></tr>
//	<tr><td>B)</td><td></td><td></td><td></td><td></td><td></td><td></td><td></td><td></td></tr>
//
// `B)` 是上一行规格型号 `车用汽油(VI` 的尾巴，必须并回上一行成为
// `车用汽油(VIB)`，否则它会被当成一条「名称=B)、金额为空」的垃圾明细。
//
// 判据（三条同时成立才合并，避免误并真实明细行）：
//  1. 当前行**只有一个非空格**；
//  2. 该格匹配「括号收尾」形态（`B)` / `VI)` / `)`）；
//  3. 上一行存在（有上一行可并）。
func mergeFuelContinuations(rows [][]string) [][]string {
	out := make([][]string, 0, len(rows))
	for _, cells := range rows {
		// 统计非空格
		nonEmpty := 0
		onlyIdx := -1
		onlyVal := ""
		for i, c := range cells {
			if strings.TrimSpace(c) != "" {
				nonEmpty++
				onlyIdx = i
				onlyVal = strings.TrimSpace(c)
			}
		}
		if nonEmpty == 1 && onlyIdx >= 0 && fuelContinuationRe.MatchString(onlyVal) && len(out) > 0 {
			prev := out[len(out)-1]
			// 并到上一行的**规格型号**那一格。
			//
			// 不能简单「并到第一个非空格」：折扣行的第一格是项目名称
			// （`*汽油*92号`），把 `B)` 并进去会变成 `*汽油*92号B)`。
			// 规格型号在 9 列模板里是第 2 格（下标 1），
			// 但折扣行/合并格会让下标漂移 —— 用「含括号未闭合」的那一格定位。
			target := -1
			unclosed := false
			for i, c := range prev {
				t := strings.TrimSpace(c)
				if t == "" {
					continue
				}
				if strings.Contains(t, "(") {
					target = i
					// 括号未闭合 → 直接拼接；已闭合 → 说明尾巴的右括号是重复的，
					// 只需并进中间内容（`车用汽油(VI)` + `B)` → `车用汽油(VIB)`）
					unclosed = !strings.Contains(t, ")")
					break
				}
			}
			if target < 0 && len(prev) > 1 {
				// 兜底：规格型号位置（第 2 格）
				target = 1
				unclosed = true
			}
			if target >= 0 && target < len(prev) {
				if unclosed {
					prev[target] = strings.TrimSpace(prev[target]) + onlyVal
				} else {
					// 已闭合：把尾巴的右括号去掉，插到闭合括号之前
					tail := strings.TrimRight(onlyVal, ")）")
					base := strings.TrimSpace(prev[target])
					if idx := strings.LastIndexAny(base, ")）"); idx >= 0 {
						prev[target] = base[:idx] + tail + base[idx:]
					} else {
						prev[target] = base + tail
					}
				}
			}
			continue
		}
		out = append(out, cells)
	}
	return out
}

// parseFuelDetailRow 按**从右往左锚定**的方式解析一行明细（本规则的核心）。
//
// 票面固定列序（9 列）：
//
//	项目名称 | 规格型号 | 单位 | 数量 | 单价 | 金额 | 税率/征收率 | 税额
//
// 但从右往左数**永远成立**的三段是：税额 ← 税率 ← 金额。
// 再往左的四段（单价 / 数量 / 单位 / 规格型号）**只在有正数数量时才存在** ——
// 折扣行（`-513.35 | 13% | -66.73`）没有这四段，左侧直接是项目名称。
//
// 所以算法是：
//  1. 从右侧找「税率格」（匹配 `N%` / 免税 / 不征税）；
//  2. 税率格右边第一个数字 = 税额；
//  3. 税率格左边第一个数字 = 金额；
//  4. 若金额左侧还有 ≥2 个数字（数量 + 单价），继续往左取：
//     单价（带多位小数的那个）、数量、单位（白名单）、规格型号（剩余）；
//  5. 剩下的最左侧**整体**作为项目名称。
//
// 这样折扣行天然只填右侧三段、左侧全空，绝不会把 `-513.35` 当项目名称。
func parseFuelDetailRow(cells []string) (fuelDetailRow, bool) {
	var r fuelDetailRow
	if len(cells) == 0 {
		return r, false
	}
	// 跳过表头行
	if len(cells) >= 2 && strings.Contains(cells[0], "项目名称") {
		return r, false
	}
	// 跳过合计行 / 价税合计行 / 备注行
	joined := strings.Join(cells, " ")
	if strings.Contains(joined, "合计") || strings.Contains(joined, "价税合计") ||
		strings.Contains(joined, "备注") || strings.Contains(joined, "购买方") ||
		strings.Contains(joined, "销售方") {
		return r, false
	}

	// ① 定位税率格（从右往左第一个匹配的）
	rateIdx := -1
	for i := len(cells) - 1; i >= 0; i-- {
		t := strings.TrimSpace(cells[i])
		if t == "" {
			continue
		}
		if fuelRateRe.MatchString(t) || t == "免税" || t == "不征税" || strings.Contains(t, "%") {
			rateIdx = i
			break
		}
	}
	if rateIdx < 0 {
		return r, false
	}
	if m := fuelRateRe.FindStringSubmatch(strings.TrimSpace(cells[rateIdx])); len(m) > 1 {
		r.TaxRate = parsePercentToFloat(m[1] + "%")
	} else {
		// 免税/不征税 → 0
		zero := 0.0
		r.TaxRate = &zero
	}

	// ② 税率右侧第一个纯数字 = 税额
	for i := rateIdx + 1; i < len(cells); i++ {
		if v := parsePlainAmount(cells[i]); v != nil {
			r.TaxAmt = v
			break
		}
	}
	// ③ 税率左侧第一个纯数字 = 金额
	amountIdx := -1
	for i := rateIdx - 1; i >= 0; i-- {
		if v := parsePlainAmount(cells[i]); v != nil {
			r.Amount = v
			amountIdx = i
			break
		}
	}
	if r.Amount == nil {
		// 金额缺失（表头/异常行）→ 不是明细行
		return r, false
	}

	// ④ 金额左侧若还有数字：单价（最右）→ 数量 → 单位 → 规格型号
	var leftNums []int // 金额左侧的纯数字格下标（从左到右）
	for i := 0; i < amountIdx; i++ {
		if parsePlainAmount(cells[i]) != nil {
			leftNums = append(leftNums, i)
		}
	}
	switch {
	case len(leftNums) >= 2:
		// 数量 + 单价（票面列序：数量在左、单价在右）
		qtyIdx := leftNums[len(leftNums)-2]
		priceIdx := leftNums[len(leftNums)-1]
		r.Qty = parsePlainAmount(cells[qtyIdx])
		r.Price = parsePlainAmount(cells[priceIdx])
		// 单价左侧：单位（白名单）→ 规格型号（再左）
		// 注意：**单位在规格型号的右边**（票面列序是 规格型号 | 单位）。
		unitIdx := -1
		for i := qtyIdx - 1; i >= 0; i-- {
			t := strings.TrimSpace(cells[i])
			if t == "" {
				continue
			}
			if fuelUnitRe.MatchString(t) {
				unitIdx = i
				break
			}
			// 遇到非单位、非空 → 再往左一格就是规格型号，单位可能为空
			break
		}
		if unitIdx >= 0 {
			r.Unit = strings.TrimSpace(cells[unitIdx])
		}
		// 规格型号：单位左侧最近的**非空**格（且不是项目名称）
		// 项目名称是最左侧那一格（含 `*` 号），规格型号在它右边。
		searchEnd := qtyIdx
		if unitIdx >= 0 {
			searchEnd = unitIdx
		}
		for i := searchEnd - 1; i >= 1; i-- {
			t := strings.TrimSpace(cells[i])
			if t == "" {
				continue
			}
			if isFuelProjectName(t) {
				break
			}
			r.Spec = t
			break
		}
		r.Name = fuelProjectName(cells, leftNums[0])
	case len(leftNums) == 1:
		// 只有一个数字：可能是「数量」但没有单价，也可能是异常；
		// 不猜，按折扣行处理（左侧全空）
		r.Name = fuelProjectName(cells, leftNums[0])
	default:
		// 无左侧数字：折扣行（只有 名称 + 负金额 + 税率 + 负税额）
		r.Discount = *r.Amount < 0
		r.Name = fuelProjectName(cells, amountIdx)
	}
	// 折扣行：规格/单位/数量/单价必须留空（不得错位）
	if r.Discount {
		r.Spec, r.Unit, r.Qty, r.Price = "", "", nil, nil
	}
	if r.Name == "" {
		return r, false
	}
	return r, true
}

// isFuelProjectName 判断一格是否像项目名称（含 `*类别*` 形态）。
func isFuelProjectName(t string) bool {
	t = strings.TrimSpace(t)
	return strings.HasPrefix(t, "*") || strings.Contains(t, "*")
}

// fuelProjectName 取「最左侧的项目名称」。
//
// 从 before 位置（金额/数量/单价所在格）往左，**跳过纯数字格**，
// 取最左边的非空文本格。项目名称可能跨两格（如 `*柴油*0号` 与 `车用柴油(VI)`），
// 但后者是规格型号 —— 用 `isFuelProjectName` 区分：含 `*` 的才是项目名称。
func fuelProjectName(cells []string, before int) string {
	// 先找含 `*` 的格（项目名称的确定性特征）
	for i := 0; i <= before && i < len(cells); i++ {
		t := strings.TrimSpace(cells[i])
		if isFuelProjectName(t) {
			return t
		}
	}
	// 没有 `*` 形态 → 取最左侧非空、非数字的格
	for i := 0; i <= before && i < len(cells); i++ {
		t := strings.TrimSpace(cells[i])
		if t == "" || parsePlainAmount(t) != nil {
			continue
		}
		return t
	}
	return ""
}

// dominantFuelRate 取明细行里出现最多的税率档（成品油票通常单一税率）。
func dominantFuelRate(items []InvoiceExtractionLine) *float64 {
	counts := map[float64]int{}
	for _, it := range items {
		if it.TaxRate == nil {
			continue
		}
		counts[*it.TaxRate]++
	}
	if len(counts) == 0 {
		return nil
	}
	best, bestN := 0.0, -1
	for r, n := range counts {
		if n > bestN || (n == bestN && r > best) {
			best, bestN = r, n
		}
	}
	return &best
}
