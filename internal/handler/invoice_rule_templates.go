package handler

import (
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 发票提取规则模板：按**发票类型**补全 desc / rule / regex。
//
// 设计要点（对应验收口径「补全前须先校验发票类型是否匹配」）：
//  1. 每个模板声明 Keywords（分类名命中词）与 CoreFields（该类型票面必备字段）。
//  2. 只有「分类名命中 + subs 里确实存在该类型的核心字段」两个条件同时成立才套用；
//     任一不满足一律回落泛用模板——绝不把通行费的规则贴到普通发票上。
//  3. 正则只给「形状确定」的字段（号码/代码/日期/金额/税额/税率/税号/车牌），
//     名称类字段只给语义规则，避免宽泛正则误命中。

// invoiceRuleTemplate 一类发票的规则模板。
type invoiceRuleTemplate struct {
	// Key 模板标识（日志/测试用）
	Key string
	// Keywords 分类名命中词（按最长命中优先，避免「电子发票」误判为「专用发票」）
	Keywords []string
	// CoreFields 该类型票面的核心字段（语义键，见 invoiceCoreFieldAliases）
	CoreFields []string
	// MinCore 至少命中多少个核心字段才认为「类型匹配」
	MinCore int
}

// invoiceRuleTemplates 有序模板表：越靠前越具体，长度相同时靠前者优先。
var invoiceRuleTemplates = []invoiceRuleTemplate{
	// 通行费必须同时具备 发票号码 + 合计 + 车牌号 才算该类型：
	// 少车牌号说明只是名字像（或字段被删），套通行费规则会白配。
	{Key: "toll", Keywords: []string{"通行费"}, CoreFields: []string{"invoiceNo", "totalAmount", "plate_no"}, MinCore: 3},
	{Key: "utility", Keywords: []string{"水电", "电费", "水费"}, CoreFields: []string{"invoiceNo", "amount", "totalAmount"}, MinCore: 2},
	{Key: "vat", Keywords: []string{"增值税专用", "专用发票"}, CoreFields: []string{"invoiceNo", "taxRate", "totalAmount"}, MinCore: 2},
	{Key: "standard", Keywords: []string{"普通发票", "电子发票", "数电票"}, CoreFields: []string{"invoiceNo", "totalAmount"}, MinCore: 1},
	{Key: "misc", Keywords: []string{"其它票据", "其他票据", "三联", "收据"}, CoreFields: []string{"invoiceNo"}, MinCore: 1},
}

// invoiceCoreFieldAliases 语义键的等价写法（含中文自定义键）。
// 用于「该类型必备字段是否存在」的判定，不做字段映射。
var invoiceCoreFieldAliases = map[string][]string{
	"invoiceNo":   {"invoiceNo", "invoice_no", "发票号码", "发票号", "票据号码", "收据编号", "编号", "号码"},
	"totalAmount": {"totalAmount", "total_amount", "价税合计", "合计", "总金额"},
	"amount":      {"amount", "金额", "不含税金额", "小计"},
	"taxRate":     {"taxRate", "tax_rate", "税率", "征收率"},
	"tax":         {"tax", "税额"},
	"plate_no":    {"plate_no", "plateNo", "车牌号", "车牌"},
}

// isChineseToken 是否含中文（中文别名用包含匹配，英文键用相等匹配）。
func isChineseToken(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

// subFieldMatchesCore 判断某个 subs 字段是否属于语义键 core。
func subFieldMatchesCore(name, core string) bool {
	aliases, ok := invoiceCoreFieldAliases[core]
	if !ok {
		return false
	}
	cn := canonicalSubFieldKey(name)
	for _, a := range aliases {
		if isChineseToken(a) {
			if strings.Contains(strings.TrimSpace(name), a) {
				return true
			}
			continue
		}
		if cn == canonicalSubFieldKey(a) {
			return true
		}
	}
	return false
}

// countCoreFields 统计 subs 中命中的核心字段个数（同一 core 只计一次）。
func countCoreFields(subs []types.FleetCategorySub, cores []string) int {
	hit := 0
	for _, core := range cores {
		for _, s := range subs {
			if subFieldMatchesCore(s.Name, core) {
				hit++
				break
			}
		}
	}
	return hit
}

// matchInvoiceRuleTemplate 严格匹配发票类型模板。
//
// 返回 nil 表示「类型不确定/不匹配」：调用方必须回落到泛用模板（只给语义规则、
// 不给正则），不能凭分类名猜一个模板套上去。
func matchInvoiceRuleTemplate(catName string, subs []types.FleetCategorySub) *invoiceRuleTemplate {
	name := strings.TrimSpace(catName)
	if name == "" || len(subs) == 0 {
		return nil
	}
	best := -1
	bestLen := 0
	for i := range invoiceRuleTemplates {
		for _, kw := range invoiceRuleTemplates[i].Keywords {
			if !strings.Contains(name, kw) {
				continue
			}
			// 最长关键词命中者胜出：如「增值税专用发票」同时含「专用发票」与「发票」，
			// 取更具体的那个模板。
			if len([]rune(kw)) > bestLen {
				best = i
				bestLen = len([]rune(kw))
			}
		}
	}
	if best < 0 {
		return nil
	}
	tpl := &invoiceRuleTemplates[best]
	minCore := tpl.MinCore
	if minCore <= 0 {
		minCore = 1
	}
	// 类型校验：分类名对上了，但 subs 里没有该类型的核心字段 → 说明只是名字像
	// （用户自建的同名分类 / 字段被删光），仍按不匹配处理。
	if countCoreFields(subs, tpl.CoreFields) < minCore {
		return nil
	}
	return tpl
}

// invoiceFieldSemantic 字段名 → 语义类（用于挑正则）。空串表示「形状不确定，不给正则」。
func invoiceFieldSemantic(name string) string {
	n := strings.TrimSpace(name)
	// 英文契约键与中文自定义键统一归一后走同一套分支
	cn := canonicalSubFieldKey(n)
	switch cn {
	case "invoiceno", "invoicenumber":
		return "invoice_no"
	case "invoicecode":
		return "invoice_code"
	case "invoicedate", "billdate", "issuedate":
		return "invoice_date"
	case "totalamount", "total", "sumamount":
		return "total_amount"
	case "amount", "netamount", "subtotal":
		return "amount"
	case "taxamount", "tax", "taxamt":
		return "tax_amount"
	case "taxrate", "rate", "taxpercent":
		return "tax_rate"
	case "buyertaxno", "buyertaxid":
		return "buyer_tax_no"
	case "sellertaxno", "sellertaxid":
		return "seller_tax_no"
	case "plateno", "platenumber", "carno":
		return "plate_no"
	case "travelfrom", "entrydate":
		return "travel_from"
	case "travelto", "exitdate":
		return "travel_to"
	}
	// 中文自定义键：包含匹配（顺序敏感，先具体后笼统）
	switch {
	case strings.Contains(n, "车牌"):
		return "plate_no"
	case strings.Contains(n, "发票代码") || strings.Contains(n, "票据代码"):
		return "invoice_code"
	case strings.Contains(n, "发票号码") || strings.Contains(n, "发票号") ||
		strings.Contains(n, "票据号码") || strings.Contains(n, "收据编号"):
		return "invoice_no"
	case strings.Contains(n, "日期"):
		return "invoice_date"
	case strings.Contains(n, "价税合计") || strings.Contains(n, "合计") || strings.Contains(n, "总金额"):
		return "total_amount"
	case strings.Contains(n, "税率") || strings.Contains(n, "征收率"):
		return "tax_rate"
	case strings.Contains(n, "税额"):
		return "tax_amount"
	case strings.Contains(n, "金额") || strings.Contains(n, "小计"):
		return "amount"
	case strings.Contains(n, "税号") || strings.Contains(n, "纳税人识别号"):
		return "any_tax_no"
	case strings.Contains(n, "通行日期") && strings.Contains(n, "起"):
		return "travel_from"
	case strings.Contains(n, "通行日期") && strings.Contains(n, "止"):
		return "travel_to"
	}
	return ""
}

// invoiceFieldRegexBySemantic 语义类 → 正则模板（Go RE2，单一捕获组）。
//
// 只列「票面标签 + 形状」都确定的字段；每条都以「有且仅有一个捕获组」为约束，
// 配合后端 uniqueRegexCapture（全批唯一命中）才生效，写错最多是不生效，不会误填。
var invoiceFieldRegexBySemantic = map[string]string{
	"invoice_no":    `(?:发票号码|发票号|票据号码|收据编号|单据编号)[^0-9A-Za-z]{0,4}([0-9A-Za-z]{6,24})`,
	"invoice_code":  `(?:发票代码|票据代码)[^0-9]{0,4}([0-9]{10,12})`,
	"invoice_date":  `(?:开票日期|填开日期|开票时间|入账日期|单据日期)[^0-9]{0,6}([0-9]{4}[-/年][0-9]{1,2}[-/月][0-9]{1,2}日?)`,
	"total_amount":  `(?:价税合计|合计金额|总金额)[^0-9]{0,10}([0-9][0-9,]*\.[0-9]{2})`,
	"amount":        `(?:不含税金额|金\s*额|小计)[^0-9]{0,10}([0-9][0-9,]*\.[0-9]{2})`,
	"tax_amount":    `(?:税\s*额)[^0-9]{0,10}([0-9][0-9,]*\.[0-9]{2})`,
	"tax_rate":      `(?:税\s*率|征收率)[^0-9]{0,4}([0-9]{1,2}(?:\.[0-9]+)?)\s*%`,
	"buyer_tax_no":  `(?:购买方|购方|买方)[^0-9A-Z]{0,12}([0-9A-HJ-NPQRTUWXY]{18})`,
	"seller_tax_no": `(?:销售方|销方|卖方)[^0-9A-Z]{0,12}([0-9A-HJ-NPQRTUWXY]{18})`,
	"plate_no":      `(?:车牌号|车牌)[^0-9A-Z]{0,3}([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Z][A-Z0-9]{4,6})`,
	"travel_from":   `(?:通行日期起|入口日期|上站日期)[^0-9]{0,4}([0-9]{4}[-/年][0-9]{1,2}[-/月][0-9]{1,2}日?)`,
	"travel_to":     `(?:通行日期止|出口日期|下站日期)[^0-9]{0,4}([0-9]{4}[-/年][0-9]{1,2}[-/月][0-9]{1,2}日?)`,
}

// invoiceFieldRegexFor 给某个字段生成兜底正则；形状不确定时返回空串。
//
// catName 预留给按类型差异化的正则（当前票种差异只体现在字段集上，
// 正则本身与票种无关，故未使用）。
func invoiceFieldRegexFor(fieldName, catName string) string {
	sem := invoiceFieldSemantic(fieldName)
	if sem == "any_tax_no" {
		// 中文「税号」无法判断是购方还是销方：交给 LLM，不配正则
		return ""
	}
	_ = catName
	return invoiceFieldRegexBySemantic[sem]
}
