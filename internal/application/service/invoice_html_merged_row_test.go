package service

import (
	"strconv"
	"strings"
	"testing"
)

// 通行费 HTML 表格的**合并格形态**（第 24 轮任务二，图 2 三个红框的根因）。
//
// 真机抽样（19 张一页一票的通行费汇总单）暴露三类形态，每类都会让整票金额为 0
// 或车牌/开票人空白。这组测试逐类锁定，并**用真实票面原文**做夹具。
//
// 顺带修掉一个既有 bug：`compactDateInTextRe` 写在反引号原始串里却用了 `\\d`，
// 语义是「匹配字面反斜杠 + d」，日期从未命中过 —— 于是所有「日期挤在合并格」
// 的数据行都被判为「不是数据行」，整份文档 rows=0。

const (
	// 形态 A：整行压进一个 colspan=8 格（项目名/车牌/车型/两个日期/金额/税率/税额）
	mergedAllInOneRow = `<tr><td colspan="8">*生产生活服务*通行费 渝D95256 货车 20261031 20261031 5.28 3% 0.16</td></tr>`
	// 形态 B：项目名格带 MinerU 的 `$ ^{{*}}...$` 包裹 + 车牌独立格
	mergedProjectCellRow = `<tr><td>$ ^{{*}}经营租赁^{{*}}通行费 $</td><td>渝C87567</td><td>货车</td><td>20250701</td><td>20250729</td><td>778.17</td><td>3%</td><td>23.35</td></tr>`
	// 形态 C：税率与税额被压进同一格（末格 colspan 合并）
	rateAndTaxInOneCellRow = `<tr><td colspan="5">*生产生活服务*通行费 渝D95256 货车 20261006</td><td>20261031</td><td>516.85</td><td>3% 15.51</td></tr>`
)

func cellsOfRow(rowHTML string) []string {
	cells := make([]string, 0, 9)
	for _, c := range htmlTdRe.FindAllStringSubmatch(rowHTML, -1) {
		cells = append(cells, trimAll(stripHTMLTags(c[1])))
	}
	return cells
}

func trimAll(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 {
		last := s[len(s)-1]
		if last != ' ' && last != '\n' && last != '\t' && last != '\r' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func fp2(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

// ---- 形态 A：整行单格 ----

func TestSplitMergedTollCellKeepsProjectNameWithSpaces(t *testing.T) {
	// 项目名自身含空格（`$ ^{{*}}经营租赁^{{*}}通行费 $` 会被 Fields 拆成 4 个 token），
	// 必须在车牌锚点上归并，否则项目名被拆碎、金额被当成 1。
	got := splitMergedTollCell("$ ^{{*}}经营租赁^{{*}}通行费 $ 渝C87567 货车 20250701 20250729 778.17 3% 23.35")
	if len(got) < 7 {
		t.Fatalf("切分过少：%v", got)
	}
	if got[0] != "$ ^{{*}}经营租赁^{{*}}通行费 $" {
		t.Errorf("项目名必须整体保留，实际 %q", got[0])
	}
	if got[1] != "渝C87567" {
		t.Errorf("车牌必须是第二格，实际 %q", got[1])
	}
}

func TestSplitMergedTollCellWithoutPlateFallsBack(t *testing.T) {
	// 没有车牌 → 整串一格（宁可拿不到金额，也不能把项目名拆碎当金额）
	got := splitMergedTollCell("办公用品 笔记本 100 13% 13")
	if len(got) != 1 {
		t.Fatalf("无车牌时必须退化为整串一格，实际 %v", got)
	}
}

func TestParseMergedAllInOneRow(t *testing.T) {
	row, ok := parseHTMLTollRow(cellsOfRow(mergedAllInOneRow))
	if !ok {
		t.Fatal("整行单格形态必须能解析出数据行")
	}
	if row.Plate != "渝D95256" {
		t.Errorf("车牌应为 渝D95256，实际 %q", row.Plate)
	}
	if fp2(row.Amount) != "5.28" || fp2(row.TaxAmount) != "0.16" {
		t.Errorf("金额/税额应为 5.28 / 0.16，实际 %s / %s", fp2(row.Amount), fp2(row.TaxAmount))
	}
	if row.TaxRate == nil || *row.TaxRate != 0.03 {
		t.Errorf("税率应为 0.03，实际 %v", row.TaxRate)
	}
	// 通行日期起/止都在合并格里，必须两个都取到
	if row.From != "20261031" || row.To != "20261031" {
		t.Errorf("日期应为 20261031/20261031，实际 %q/%q", row.From, row.To)
	}
}

// ---- 形态 B：项目名带包裹 + 车牌独立格 ----

func TestParseMergedProjectCellRow(t *testing.T) {
	row, ok := parseHTMLTollRow(cellsOfRow(mergedProjectCellRow))
	if !ok {
		t.Fatal("应解析成功")
	}
	if row.Plate != "渝C87567" {
		t.Errorf("车牌应为 渝C87567，实际 %q", row.Plate)
	}
	if fp2(row.Amount) != "778.17" || fp2(row.TaxAmount) != "23.35" {
		t.Errorf("金额/税额应为 778.17 / 23.35，实际 %s / %s", fp2(row.Amount), fp2(row.TaxAmount))
	}
	if row.ItemName == "" || !strings.Contains(row.ItemName, "通行费") {
		t.Errorf("项目名必须保留含「通行费」的原文，实际 %q", row.ItemName)
	}
}

// ---- 形态 C：税率与税额同格 ----

func TestParseRateAndTaxInOneCellRow(t *testing.T) {
	row, ok := parseHTMLTollRow(cellsOfRow(rateAndTaxInOneCellRow))
	if !ok {
		t.Fatal("应解析成功")
	}
	if row.TaxRate == nil || *row.TaxRate != 0.03 {
		t.Errorf("同格 3%% 15.51 必须抠出税率 3%%，实际 %v", row.TaxRate)
	}
	if fp2(row.TaxAmount) != "15.51" {
		t.Errorf("同格里的税额应为 15.51，实际 %s", fp2(row.TaxAmount))
	}
	if fp2(row.Amount) != "516.85" {
		t.Errorf("金额应为 516.85，实际 %s", fp2(row.Amount))
	}
}

func TestPercentInCellReMatchesBothForms(t *testing.T) {
	for _, c := range []string{"3%", "3% 15.51", "税率/征收率 9%", "免税"} {
		got := percentInCellRe.MatchString(c)
		want := c != "免税"
		if got != want {
			t.Errorf("%q 命中应为 %v，实际 %v", c, want, got)
		}
	}
}

// ---- 既有 bug：日期正则双反斜杠 ----

func TestCompactDateInTextReMatchesRealText(t *testing.T) {
	// 回归锁：这条正则此前写成 `\\d`（反引号原始串里的双反斜杠 = 字面反斜杠 + d），
	// 于是「日期挤在合并格」的形态永远匹配不到 → 整份文档 rows=0。
	// 取**子组**（m[1]）：正则的边界符 `(?:^|[^0-9.])` 会被 FindString 一并返回，
	// 得到带前导空格的 `" 20261006"`。落库前必须拿到纯净的 8 位数字。
	m := compactDateInTextRe.FindStringSubmatch("*生产生活服务*通行费 渝D95256 货车 20261006")
	if m == nil || m[1] != "20261006" {
		t.Errorf("必须能从混合文本抠出纯净的 8 位日期，实际 %v", m)
	}
	// 带小数点的金额不能被当成日期
	if m := compactDateInTextRe.FindStringSubmatch("金额 719.61 税额 64.77"); m != nil {
		t.Errorf("金额不得被当成日期，实际 %v", m)
	}
}

// ---- 整份文档端到端（真实票面原文）----

func TestExtractMergedTollDocEndToEnd(t *testing.T) {
	// 拼一份含三种形态的迷你汇总单（票号行 + 表格 + 开票人）
	doc := `发票号码：26507906731100002860
开票日期：2026年11月02日
<table>
<tr><td>购买方信息</td><td colspan="3">名称:重庆星达铜业有限公司</td></tr>
` + mergedAllInOneRow + `
</table>
开票人：张莹月

发票号码：26507903751100004228
开票日期：2026年11月02日
<table>
` + rateAndTaxInOneCellRow + `
</table>
开票人：刘洋

发票号码：26507904441100005581
开票日期：2026年11月02日
<table>
` + mergedProjectCellRow + `
</table>
开票人：郝敏
`
	res, ok := ExtractInvoicesByHTMLTableRules(doc)
	if !ok || res == nil {
		t.Fatal("整份文档必须命中规则")
	}
	if len(res.Invoices) != 3 {
		t.Fatalf("应提取 3 张，实际 %d", len(res.Invoices))
	}
	for _, inv := range res.Invoices {
		if inv.Amount == nil || *inv.Amount == 0 {
			t.Errorf("%s 金额不得为 0", inv.InvoiceNo)
		}
		if inv.Issuer == "" {
			t.Errorf("%s 开票人不得为空", inv.InvoiceNo)
		}
		plate, _ := inv.Fields["车牌号"].(string)
		if plate == "" {
			t.Errorf("%s 车牌不得为空", inv.InvoiceNo)
		}
		// 价税合计必须与「金额 + 税额」一致（容差 0.02）
		if inv.TotalAmount != nil && inv.Tax != nil && inv.Amount != nil {
			diff := *inv.TotalAmount - (*inv.Amount + *inv.Tax)
			if diff > 0.02 || diff < -0.02 {
				t.Errorf("%s 价税合计 %v 与 金额+税额 %v 不符",
					inv.InvoiceNo, *inv.TotalAmount, *inv.Amount+*inv.Tax)
			}
		}
	}
}
