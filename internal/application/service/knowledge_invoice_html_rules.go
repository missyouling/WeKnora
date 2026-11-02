// MinerU HTML 表格版通行费汇总单规则（第 23 轮 issue 4）。
//
// 触发背景：真机「通行费电子票据汇总单(发票).pdf」里，MinerU 输出的是 **HTML 表格**
// 而不是 Markdown 管道表 ——
//
//	发票号码：26507903161100005511
//	开票日期：2026年11月02日
//	<table>
//	<tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司统一社会信用代码/纳税人识别号:91500227MA5U54AJ0A</td>…
//	<tr><td>*生产生活服务*通行费</td><td>渝A75R1C</td><td>客车</td><td colspan="2">20261022</td>
//	    <td>20261022</td><td>49.24</td><td>3%</td><td>1.48</td></tr>
//	<tr><td colspan="2">价税合计(大写)</td><td colspan="4">伍拾圆柒角贰分</td><td>(小写)</td><td colspan="2">¥50.72</td></tr>
//	开票人：杨希
//
// 三套既有规则全不命中：Line 要「号码行与金额段同行」、MarkdownTable 要管道符
// `|a|b|c|`、FlatLayout 要 `发票代码:` 锚点。于是整份文档落到 LLM —— 而在这种
// 一页一票 × 十几页的模板噪音里，模型只回得出票号，金额/税率/税额/价税合计/开票人
// 全空（真机实测 6 张票只落票号），用户还得逐张手填。
//
// 本规则把这种结构**确定性**解析出来：段落以「发票号码：」为界，段落内的 HTML
// 数据行给金额/税率/税额（并据「金额 + 税额」反算价税合计，因为小写价税合计在
// 文本层常被乱码吃掉），开票人取段落末尾的 `开票人：X`。
package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	// htmlInvNoLabelRe 抓「发票号码：<6~20 位>」。只认明确标签，不含校验码/机器编号。
	htmlInvNoLabelRe = regexp.MustCompile("发票号码\\s*[:：]\\s*(\\d{6,20})")
	// htmlInvDateLabelRe 抓「开票日期：2026年11月02日」。
	htmlInvDateLabelRe = regexp.MustCompile("开票日期\\s*[:：]\\s*(\\d{4})\\s*年\\s*(\\d{1,2})\\s*月\\s*(\\d{1,2})\\s*日")
	// htmlIssuerLabelRe 抓「开票人：杨希」。表单模板噪音里的「开票人:」（无值）不命中。
	htmlIssuerLabelRe = regexp.MustCompile("开票人\\s*[:：]\\s*([^\\s<]+)")
	// htmlTotalLowerRe 抓「(小写) ¥50.72」。
	htmlTotalLowerRe = regexp.MustCompile("[（(]\\s*小写\\s*[)）]\\s*[¥￥]?\\s*([\\d.]+)")
	// htmlTrRe / htmlTdRe 拆 HTML 行与单元格。
	htmlTrRe = regexp.MustCompile("(?is)<tr[^>]*>(.*?)</tr>")
	htmlTdRe = regexp.MustCompile("(?is)<td[^>]*>(.*?)</td>")
	// htmlPlateRe 车牌：省份简称 + 1 位字母 + 4~6 位字母数字。
	htmlPlateRe = regexp.MustCompile("^[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Za-z][A-Za-z0-9]{4,6}$")
	// htmlVehicleRe 车型：该模板固定枚举，避免把「20261022」或金额当车型。
	htmlVehicleRe = regexp.MustCompile("^(客车|货车|专项作业车|挂车|轿车|摩托车|其他)$")
	// htmlCompactDateRe 8 位紧凑日期。
	htmlCompactDateRe = regexp.MustCompile("^\\d{8}$")
)

// htmlTollRow 是一行的确定性解析结果（表格版汇总单的数据行）。
type htmlTollRow struct {
	Name      string
	Plate     string
	Vehicle   string
	From      string
	To        string
	Amount    *float64
	TaxRate   *float64
	TaxAmount *float64
	ItemName  string
}

// ExtractInvoicesByHTMLTableRules 解析 MinerU HTML 表格版电子票据。
//
// 命中判据（宁可不命中也不能误伤）：
//  1. 文本里出现至少一个「发票号码：<数字>」标签行；
//  2. 段落内至少解析出 1 个通行费数据行（车牌 + 双 8 位日期 + 金额/税额之一）。
//
// 段落 = 本号码行到下一个号码行之间。**绝不跨段配对**：段落里没有数据行就不产出
// 该票（缺字段的票交给调用方回退 LLM），而不是拿邻居的金额硬塞 —— 那正是
// 历史上「张冠李戴」的成因。
func ExtractInvoicesByHTMLTableRules(content string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "<tr") {
		return nil, false
	}
	locs := htmlInvNoLabelRe.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		return nil, false
	}
	invs := make([]InvoiceExtractionItem, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		inv, ok := parseHTMLTableInvoiceSegment(content[start:end])
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

// ExtractInvoicesByHTMLTableRulesFromChunks 按页序拼接 chunks 后执行本规则。
func ExtractInvoicesByHTMLTableRulesFromChunks(chunks []*types.Chunk) (*InvoiceExtractionResult, bool) {
	src := BuildExtractSourceText(chunks)
	if strings.TrimSpace(src.Content) == "" {
		return nil, false
	}
	res, ok := ExtractInvoicesByHTMLTableRules(src.Content)
	if ok && res != nil {
		AssignPhysicalPages(src, res.Invoices)
	}
	return res, ok
}

// parseHTMLTableInvoiceSegment 解析单张票的段落（号码行 → 下一号码行）。
func parseHTMLTableInvoiceSegment(seg string) (InvoiceExtractionItem, bool) {
	var inv InvoiceExtractionItem
	if m := htmlInvNoLabelRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.InvoiceNo = strings.TrimSpace(m[1])
	}
	if inv.InvoiceNo == "" {
		return inv, false
	}
	rows := collectHTMLTollRows(seg)
	if len(rows) == 0 {
		return inv, false
	}
	head := rows[0]

	inv.InvoiceDate = htmlInvoiceDateOf(seg)
	inv.InvoiceType = "普通发票" // 通行费电子发票均为增值税普通发票
	inv.Category = "通行费发票"
	inv.Amount = head.Amount
	inv.TaxRate = head.TaxRate
	inv.Tax = head.TaxAmount

	// 购销方：**按位置就近配对**，不在原始 HTML 上写跨格正则。
	//
	// 模板形态：<td>购买方信息</td><td colspan=3>名称:X统一社会信用代码/纳税人识别号:Y</td>
	// 「购买方」与「名称」之间隔着 </td><td>，用排除换行的正则会直接失配（实测全空）；
	// 放宽成跨行匹配又会贪婪地把「销售方」那格也吞进来。
	// 就近配对天然正确：某方标签之后的**第一个**「名称…税号」格就是它自己的资料。
	inv.BuyerName, inv.BuyerTaxNo = htmlPartyNear(seg, "购买方")
	if inv.BuyerName == "" {
		inv.BuyerName, inv.BuyerTaxNo = htmlPartyNear(seg, "购") // 「购 买 方」逐字断行形态
	}
	inv.SellerName, inv.SellerTaxNo = htmlPartyNear(seg, "销售方")
	if inv.SellerName == "" {
		inv.SellerName, inv.SellerTaxNo = htmlPartyNear(seg, "销")
	}
	// 开票人：段落末尾的「开票人：X」
	if m := htmlIssuerLabelRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.Issuer = mdCleanText(m[1])
	}
	// 价税合计：优先取「(小写) ¥X」；文本层乱码取不到时用 金额 + 税额 反算。
	// 反算必须有依据 —— 金额与税额都拿到才计算，任一缺失就留空（不猜）。
	if m := htmlTotalLowerRe.FindStringSubmatch(seg); len(m) > 1 {
		if v := parseAmountToFloat(m[1]); v != nil {
			inv.TotalAmount = v
		}
	}
	if inv.TotalAmount == nil && head.Amount != nil && head.TaxAmount != nil {
		total := roundMoney(*head.Amount + *head.TaxAmount)
		inv.TotalAmount = &total
	}

	// 明细行：与其它规则同一形态（名称 + 金额/税率/税额 + 分类子列 extra），
	// 让详情抽屉按分类子列（车牌号/车辆类型/通行日期起止）自适应回显。
	items := make([]InvoiceExtractionLine, 0, len(rows))
	sumAmount, sumTax := 0.0, 0.0
	hasAmount, hasTax := false, false
	for _, r := range rows {
		line := InvoiceExtractionLine{Name: coalesceStr(r.ItemName, "通行费"), Amount: r.Amount, TaxRate: r.TaxRate, TaxAmt: r.TaxAmount}
		extra := map[string]any{}
		if r.Plate != "" {
			extra["plate_no"] = r.Plate
		}
		if r.Vehicle != "" {
			extra["vehicle_type"] = r.Vehicle
		}
		if r.From != "" {
			extra["travel_from"] = r.From
		}
		if r.To != "" {
			extra["travel_to"] = r.To
		}
		if len(extra) > 0 {
			line.Extra = extra
		}
		items = append(items, line)
		if r.Amount != nil {
			sumAmount += *r.Amount
			hasAmount = true
		}
		if r.TaxAmount != nil {
			sumTax += *r.TaxAmount
			hasTax = true
		}
	}
	if len(items) > 1 {
		// 多行（个别页含两条通行记录）：票据级金额/税额取逐行合计
		if hasAmount {
			v := roundMoney(sumAmount)
			inv.Amount = &v
		}
		if hasTax {
			v := roundMoney(sumTax)
			inv.Tax = &v
		}
		if inv.TotalAmount == nil && hasAmount && hasTax {
			v := roundMoney(sumAmount + sumTax)
			inv.TotalAmount = &v
		}
	}
	inv.Items = items
	// 车牌写动态字段扩展桶（与另外两套规则同口径）：列表「车牌号」独立列读它
	if head.Plate != "" {
		inv.Fields = map[string]any{"车牌号": head.Plate}
	}
	if head.Plate != "" || head.From != "" || head.To != "" {
		inv.Remark = fmt.Sprintf("车牌号：%s；通行日期起：%s；通行日期止：%s", head.Plate, head.From, head.To)
	}
	return inv, true
}

// collectHTMLTollRows 解析段落里全部通行费数据行。
func collectHTMLTollRows(seg string) []htmlTollRow {
	out := make([]htmlTollRow, 0, 1)
	for _, tr := range htmlTrRe.FindAllStringSubmatch(seg, -1) {
		cells := make([]string, 0, 9)
		for _, c := range htmlTdRe.FindAllStringSubmatch(tr[1], -1) {
			cells = append(cells, strings.TrimSpace(stripHTMLTags(c[1])))
		}
		if len(cells) == 0 {
			continue
		}
		if row, ok := parseHTMLTollRow(cells); ok {
			out = append(out, row)
		}
	}
	return out
}

// parseHTMLTollRow 从一行的单元格里抽取通行费字段（标签优先，位置兜底）。
//
// 不用固定下标：该模板大量使用 colspan，行内实际格数在 8~9 之间浮动，
// 位置法会整体错位（历史教训）。
func parseHTMLTollRow(cells []string) (htmlTollRow, bool) {
	var row htmlTollRow
	var dates []string
	for _, cell := range cells {
		if row.Plate == "" {
			if m := htmlLabelPlateRe.FindStringSubmatch(cell); m != nil {
				if p := mdCleanText(m[1]); htmlPlateRe.MatchString(p) {
					row.Plate = p
					continue
				}
			}
			if htmlPlateRe.MatchString(cell) {
				row.Plate = cell
				continue
			}
		}
		if htmlCompactDateRe.MatchString(cell) {
			dates = append(dates, cell)
			continue
		}
		if m := htmlLabelFromRe.FindStringSubmatch(cell); m != nil && htmlCompactDateRe.MatchString(m[1]) {
			dates = append(dates, m[1])
			continue
		}
		if m := htmlLabelToRe.FindStringSubmatch(cell); m != nil && htmlCompactDateRe.MatchString(m[1]) {
			dates = append(dates, m[1])
		}
	}
	if row.Plate == "" || len(dates) < 2 {
		return row, false
	}
	row.From, row.To = dates[0], dates[1]

	// 金额 / 税率 / 税额：先按标签取（最稳），再按「末三格」位置兜底
	joined := strings.Join(cells, "\n")
	if m := htmlLabelAmountRe.FindStringSubmatch(joined); m != nil {
		row.Amount = parseAmountToFloat(m[1])
	}
	if m := htmlLabelRateRe.FindStringSubmatch(joined); m != nil {
		row.TaxRate = parsePercentToFloat(m[1])
	}
	if m := htmlLabelTaxRe.FindStringSubmatch(joined); m != nil {
		row.TaxAmount = parseAmountToFloat(m[1])
	}
	if row.Amount == nil || row.TaxAmount == nil {
		if len(cells) >= 3 {
			tail := cells[len(cells)-3:]
			if row.Amount == nil {
				row.Amount = parseAmountToFloat(tail[0])
			}
			if row.TaxRate == nil {
				row.TaxRate = parsePercentToFloat(tail[1])
			}
			if row.TaxAmount == nil {
				row.TaxAmount = parseAmountToFloat(tail[2])
			}
		}
	}
	// 金额与税额都拿不到 → 这不是数据行（避免把表头/购销方行当数据）
	if row.Amount == nil && row.TaxAmount == nil {
		return row, false
	}
	for _, cell := range cells {
		if row.Vehicle == "" && htmlVehicleRe.MatchString(cell) {
			row.Vehicle = cell
		}
		if row.ItemName == "" && strings.Contains(cell, "通行费") {
			row.ItemName = cell
		}
	}
	return row, true
}

// htmlPartyScanText 把段落转成「一行一格」的纯文本，供购销方正则匹配。
//
// 不能直接在原始 HTML 上匹配：`购买方信息</td><td colspan=3>名称:...` 里
// 「购买方」与「名称」之间隔着 `</td><td ...>` 标签，按「不能含 <」写正则会
// 直接失配（真机实测购销方全空）。这里先把单元格边界换成换行、再去标签，
// 正则只需排除换行即可精确锚定「同一格内的名称 + 税号」。
func htmlPartyScanText(seg string) string {
	s := strings.ReplaceAll(seg, "</td>", "\n")
	s = strings.ReplaceAll(s, "</tr>", "\n")
	return stripHTMLTags(s)
}

// htmlPartyNameTaxRe 抓「名称:X 统一社会信用代码/纳税人识别号:Y」形态的购销方格。
var htmlPartyNameTaxRe = regexp.MustCompile(
	"名称\\s*[:：]\\s*([^\\n]*?)\\s*(?:统一社会信用代码|纳税人识别号)\\s*[/／]?\\s*(?:纳税人识别号)?\\s*[:：]\\s*([0-9A-Za-z]{15,20})")

// htmlPartyNear 取「某方标签之后最近的第一个购销方格」的名称与税号。
//
// 就近而**不跨格贪婪**：购买方格的名称与销售方格的名称各自独立成格，
// 取该标签后第一次出现的「名称…税号」即正确归属。段已按发票号码切分，
// 因此绝不会拿到别的发票的购销方。
func htmlPartyNear(seg, label string) (name, taxNo string) {
	text := htmlPartyScanText(seg)
	at := strings.Index(text, label)
	if at < 0 {
		return "", ""
	}
	m := htmlPartyNameTaxRe.FindStringSubmatch(text[at:])
	if m == nil {
		return "", ""
	}
	// 名称里可能混入「信 息」这类逐字断行的标签残字：先剔除残字，再清洗中文伪空格
	n := strings.NewReplacer("信", "", "息", "").Replace(m[1])
	return strings.TrimSpace(joinCJKWords(n)), strings.TrimSpace(m[2])
}

// htmlInvoiceDateOf 取段落里的开票日期；缺失返回空（由调用方用通行日期起兜底）。
func htmlInvoiceDateOf(seg string) string {
	if m := htmlInvDateLabelRe.FindStringSubmatch(seg); len(m) > 3 {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return fmt.Sprintf("%04d-%02d-%02d", y, mo, d)
	}
	return ""
}

// roundMoney 金额两位小数（四舍五入，加 1e-6 抵消浮点表示误差）。
func roundMoney(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// coalesceStr 取第一个非空字符串。
func coalesceStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
