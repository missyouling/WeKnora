package service

import (
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取中台的**后置字段级正则补洞**：LLM（含自愈）跑完之后，对仍然为空的
// 字段，用确定性正则在**本批原文**里回填。
//
// 为什么需要它：发票的三套规则提取器是**前置抢跑**（命中就不调模型），一旦落入 LLM
// 路径，模型漏掉单个字段（如 invoice_no 输出了空串）就没有任何兜底——校验器只报
// error，字段照样空着入库。这里补上确定性的一层。
//
// 安全边界（比召回更重要）：
//   - 作用域限定"单批原文"，绝不跨批匹配（跨批必然串号）；
//   - 同批多张票、且多张都缺同一字段时**直接放弃**——按序猜测会把 A 票的号码写到
//     B 票上（历史上踩过"张冠李戴"）；
//   - 匹配出的值若已属于同批另一张票，同样放弃；
//   - 只碰空字段，绝不覆盖模型已给出的值。

// RegexFieldPatch 一次补洞记录，用于日志与前端"该字段由正则补齐"的溯源展示。
type RegexFieldPatch struct {
	Index  int    `json:"index"`  // 发票在结果数组中的下标
	Field  string `json:"field"`  // 字段 key，如 invoice_no
	Value  string `json:"value"`  // 回填值
	Source string `json:"source"` // config（用户配置的正则） | builtin（内置高置信度正则）
}

const (
	regexPatchSourceConfig  = "config"
	regexPatchSourceBuiltin = "builtin"
)

// builtinInvoiceFieldRegexes 内置兜底正则，按「标签优先、形状其次」排序。
//
// 关键的取舍：绝不用裸 `\b\d{8}\b` 去猜 8 位发票号——真实票据里 8 位数字串有 136 个
// （日期 20251024、税号片段 91500227、电话 41557211……），猜出来的号码比空着更有害
// （用户会当成真数据用）。所以 8 位只接受「发票号码」标签后的值；裸形状只认 20 位
// 全电票号（形状足够唯一）。
var builtinInvoiceFieldRegexes = map[string][]string{
	"invoice_no": {
		`发票号码\s*[:：]?\s*([0-9]{8,20})`,
		`\b(\d{20})\b`,
	},
	"invoice_code": {
		`发票代码\s*[:：]?\s*([0-9]{10,12})`,
	},
	"invoice_date": {
		`开票日期\s*[:：]?\s*(\d{4}\s*[-/年.]\s*\d{1,2}\s*[-/月.]\s*\d{1,2}\s*日?)`,
	},
	// 购销方税号：以标签定位，避免同批两个 18 位码互相干扰。
	"buyer_tax_no": {
		`购买方[\s\S]{0,120}?([0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10})`,
	},
	"seller_tax_no": {
		`销售方[\s\S]{0,120}?([0-9A-HJ-NPQRTUWXY]{2}\d{6}[0-9A-HJ-NPQRTUWXY]{10})`,
	},
}

// invoiceFieldAccessors 可补洞字段的读写器。只有出现在这里的字段才会被补洞——
// 明细子列（items[].extra）结构复杂且逐行对齐，不在此列。
// invoiceFieldAccessor 一个可补洞字段的读写器。
type invoiceFieldAccessor struct {
	Get func(*InvoiceExtractionItem) string
	Set func(*InvoiceExtractionItem, string)
}

var invoiceFieldAccessors = map[string]invoiceFieldAccessor{
	"invoice_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceNo },
		Set: func(i *InvoiceExtractionItem, v string) { i.InvoiceNo = v },
	},
	"invoice_code": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceCode },
		Set: func(i *InvoiceExtractionItem, v string) { i.InvoiceCode = v },
	},
	"invoice_date": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceDate },
		Set: func(i *InvoiceExtractionItem, v string) { i.InvoiceDate = v },
	},
	"buyer_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerTaxNo },
		Set: func(i *InvoiceExtractionItem, v string) { i.BuyerTaxNo = v },
	},
	"seller_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerTaxNo },
		Set: func(i *InvoiceExtractionItem, v string) { i.SellerTaxNo = v },
	},
	"buyer_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerName },
		Set: func(i *InvoiceExtractionItem, v string) { i.BuyerName = v },
	},
	"seller_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerName },
		Set: func(i *InvoiceExtractionItem, v string) { i.SellerName = v },
	},
	"issuer": {
		Get: func(i *InvoiceExtractionItem) string { return i.Issuer },
		Set: func(i *InvoiceExtractionItem, v string) { i.Issuer = v },
	},
}

// invoiceFieldOrder 补洞字段的固定遍历顺序：让日志与 PatchedByRegex 稳定可复现
// （map 迭代无序会让同一份输入产出不同顺序的补洞记录）。
var invoiceFieldOrder = []string{
	"invoice_no", "invoice_code", "invoice_date",
	"buyer_name", "buyer_tax_no", "seller_name", "seller_tax_no", "issuer",
}

// invoiceAccessorFor 按**用户配置里可能出现的任意键名**解析补洞读写器。
//
// 前端保存的是 camelCase（invoiceNo / buyerTaxNo），这里的内置字典是 snake_case。
// 不做 canonical 归一，用户在规则面板填的正则永远匹配不到字段。返回的 key 是契约键
// （snake_case），用于日志与补洞明细；issue 里仍用用户配置的原名，便于界面回显。
func invoiceAccessorFor(field string) (string, invoiceFieldAccessor, bool) {
	if acc, ok := invoiceFieldAccessors[field]; ok {
		return field, acc, true
	}
	want := canonicalFieldKey(field)
	if want == "" {
		return "", invoiceFieldAccessor{}, false
	}
	// 按固定顺序遍历，保证同一份输入的结果可复现
	for _, key := range invoiceFieldOrder {
		if canonicalFieldKey(key) == want {
			return key, invoiceFieldAccessors[key], true
		}
	}
	return "", invoiceFieldAccessor{}, false
}

// RegexFallbackInvoiceFields 对发票结果执行后置正则补洞，返回本次补洞明细。
//
// cfg 为用户提取配置（可为 nil）：其中 ExtractFieldConfig.Regex 若非空，优先于内置
// 正则使用（用户口径 > 系统默认）。补洞结果同时写入 res.PatchedByRegex（字段名去重）
// 与 res.RegexPatches（明细），供前端展示"该字段来源"。
func RegexFallbackInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var patches []RegexFieldPatch
	for _, field := range invoiceFieldOrder {
		acc, ok := invoiceFieldAccessors[field]
		if !ok {
			continue
		}
		// 只有**恰好一张**缺该字段时才补：多张都缺时无法判断匹配值属于谁。
		missingIdx := -1
		missingCount := 0
		for i := range res.Invoices {
			if strings.TrimSpace(acc.Get(&res.Invoices[i])) == "" {
				missingIdx = i
				missingCount++
			}
		}
		if missingCount != 1 {
			continue
		}
		for _, pattern := range fieldRegexPatterns(field, cfg) {
			re, err := regexp.Compile(pattern.re)
			if err != nil {
				continue // 用户配置的正则非法：跳过，不影响其它字段
			}
			value, ok := uniqueRegexCapture(text, re)
			if !ok {
				continue
			}
			value = normalizePatchedValue(field, value)
			if value == "" {
				continue
			}
			// 该值已属于同批另一张票 → 放弃（防重复/错位）。
			if valueUsedElsewhere(res, acc.Get, missingIdx, value) {
				continue
			}
			acc.Set(&res.Invoices[missingIdx], value)
			patches = append(patches, RegexFieldPatch{
				Index: missingIdx, Field: field, Value: value, Source: pattern.source,
			})
			break // 一个字段补一次即止
		}
	}
	for _, p := range patches {
		if !stringInSlice(res.PatchedByRegex, p.Field) {
			res.PatchedByRegex = append(res.PatchedByRegex, p.Field)
		}
	}
	res.RegexPatches = append(res.RegexPatches, patches...)
	return patches
}

// fieldRegexPattern 一条候选正则及其来源。
type fieldRegexPattern struct {
	re     string
	source string
}

// fieldRegexPatterns 返回该字段的候选正则：用户配置优先，其次内置字典。
func fieldRegexPatterns(field string, cfg *types.KbExtractConfig) []fieldRegexPattern {
	var out []fieldRegexPattern
	if cfg != nil {
		want := canonicalFieldKey(field)
		for _, f := range cfg.Fields {
			// 配置键可能是 camelCase（invoiceNo）：统一按 canonical 比对
			if !f.Enabled || canonicalFieldKey(strings.TrimSpace(f.Name)) != want {
				continue
			}
			if re := strings.TrimSpace(f.Regex); re != "" {
				out = append(out, fieldRegexPattern{re: re, source: regexPatchSourceConfig})
			}
		}
	}
	for _, re := range builtinInvoiceFieldRegexes[field] {
		out = append(out, fieldRegexPattern{re: re, source: regexPatchSourceBuiltin})
	}
	return out
}

// uniqueRegexCapture 在文本中匹配，且**全批只允许一个不同取值**时返回它。
// 取首个捕获组（有则用），否则取整个匹配。
func uniqueRegexCapture(text string, re *regexp.Regexp) (string, bool) {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", false
	}
	seen := make(map[string]struct{}, len(matches))
	value := ""
	for _, m := range matches {
		v := m[0]
		if len(m) > 1 && strings.TrimSpace(m[1]) != "" {
			v = m[1]
		}
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		seen[v] = struct{}{}
		value = v
	}
	if len(seen) != 1 {
		return "", false
	}
	return value, true
}

// normalizePatchedValue 按字段类型归一化（日期统一 YYYY-MM-DD；其余去空白）。
func normalizePatchedValue(field, value string) string {
	value = strings.TrimSpace(value)
	if field == "invoice_date" {
		if normalized, ok := normalizeDateString(value); ok {
			return normalized
		}
		return ""
	}
	return value
}

// valueUsedElsewhere 判断该值是否已被同批其它发票占用。
func valueUsedElsewhere(
	res *InvoiceExtractionResult,
	get func(*InvoiceExtractionItem) string,
	skipIdx int,
	value string,
) bool {
	for i := range res.Invoices {
		if i == skipIdx {
			continue
		}
		if strings.TrimSpace(get(&res.Invoices[i])) == value {
			return true
		}
	}
	return false
}

func stringInSlice(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
