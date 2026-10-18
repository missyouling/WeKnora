package service

import (
	"regexp"
	"strconv"
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
//
// 2026-10-18 扩展（用户口径）：补洞范围从"8 个文本键"放开到**全部标准字段 +
// 自定义扩展字段 + 明细子项**。代价是必须自己守住类型安全：
//   - 数值字段（total_amount/amount/tax/tax_rate）捕获值要先清洗货币符/千分位，
//     再 ParseFloat，转不出来就放弃（宁可空着，也不写 0 误导用户）；
//   - 自定义扩展字段写进 InvoiceExtractionItem.Fields 动态桶；
//   - 明细子项只在「本批单票 + 单据单行明细」时回填，多行无法唯一归属一律不猜。

// RegexFieldPatch 一次补洞记录，用于日志与前端"该字段由正则补齐"的溯源展示。
type RegexFieldPatch struct {
	Index  int    `json:"index"`  // 发票在结果数组中的下标
	Field  string `json:"field"`  // 字段 key，如 invoice_no；明细子项为 items.<key>
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
//
// 金额类字段**故意不给内置正则**：一张票上至少有三个金额（金额/税额/价税合计），
// 任何不锚定标签的形状匹配都会串位。这类字段只认用户自己配的、带标签锚点的正则。
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

// invoiceFieldAccessor 一个可补洞字段的读写器。
// Set 返回 false 代表"值拿到了但不能安全落位"（如金额转不成数值），调用方应放弃。
type invoiceFieldAccessor struct {
	Get func(*InvoiceExtractionItem) string
	Set func(*InvoiceExtractionItem, string) bool
}

// textSetter 文本字段的统一 setter（永远成功）。
func textSetter(target func(*InvoiceExtractionItem) *string) func(*InvoiceExtractionItem, string) bool {
	return func(i *InvoiceExtractionItem, v string) bool {
		*target(i) = v
		return true
	}
}

// moneySetter 数值字段的统一 setter：清洗货币符/千分位 → ParseFloat，失败即放弃。
func moneySetter(target func(*InvoiceExtractionItem) **float64) func(*InvoiceExtractionItem, string) bool {
	return func(i *InvoiceExtractionItem, v string) bool {
		f, ok := parsePatchedNumber(v)
		if !ok {
			return false
		}
		*target(i) = &f
		return true
	}
}

var invoiceFieldAccessors = map[string]invoiceFieldAccessor{
	// ---- 文本标准字段 ----
	"invoice_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceNo },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceNo }),
	},
	"invoice_code": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceCode },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceCode }),
	},
	"invoice_date": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceDate },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceDate }),
	},
	"invoice_type": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceType },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceType }),
	},
	"buyer_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerName },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerName }),
	},
	"buyer_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerTaxNo },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerTaxNo }),
	},
	"buyer_address": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerAddr },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerAddr }),
	},
	"buyer_phone": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerPhone },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerPhone }),
	},
	"buyer_bank": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerBank },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerBank }),
	},
	"buyer_account": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerAcct },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerAcct }),
	},
	"seller_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerName },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerName }),
	},
	"seller_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerTaxNo },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerTaxNo }),
	},
	"seller_address": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerAddr },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerAddr }),
	},
	"seller_phone": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerPhone },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerPhone }),
	},
	"seller_bank": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerBank },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerBank }),
	},
	"seller_account": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerAcct },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerAcct }),
	},
	"issuer": {
		Get: func(i *InvoiceExtractionItem) string { return i.Issuer },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.Issuer }),
	},
	"remark": {
		Get: func(i *InvoiceExtractionItem) string { return i.Remark },
		Set: textSetter(func(i *InvoiceExtractionItem) *string { return &i.Remark }),
	},
	// ---- 数值标准字段（清洗后转 float64）----
	"total_amount": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.TotalAmount) },
		Set: moneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.TotalAmount }),
	},
	"amount": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.Amount) },
		Set: moneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.Amount }),
	},
	"tax": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.Tax) },
		Set: moneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.Tax }),
	},
	"tax_rate": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.TaxRate) },
		Set: moneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.TaxRate }),
	},
}

// invoiceFieldOrder 补洞字段的固定遍历顺序：让日志与 PatchedByRegex 稳定可复现
// （map 迭代无序会让同一份输入产出不同顺序的补洞记录）。
// 文本字段在前、数值字段在后：金额类正则误命中代价更高，让确定性更强的字段先落位。
var invoiceFieldOrder = []string{
	"invoice_no", "invoice_code", "invoice_date", "invoice_type",
	"buyer_name", "buyer_tax_no", "buyer_address", "buyer_phone", "buyer_bank", "buyer_account",
	"seller_name", "seller_tax_no", "seller_address", "seller_phone", "seller_bank", "seller_account",
	"issuer", "remark",
	"total_amount", "amount", "tax", "tax_rate",
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
	patches := make([]RegexFieldPatch, 0, 4)
	patches = append(patches, patchStandardInvoiceFields(res, text, cfg)...)
	patches = append(patches, patchConfiguredInvoiceFields(res, text, cfg)...)
	patches = append(patches, patchInvoiceItemSubFields(res, text, cfg)...)
	for _, p := range patches {
		if !stringInSlice(res.PatchedByRegex, p.Field) {
			res.PatchedByRegex = append(res.PatchedByRegex, p.Field)
		}
	}
	res.RegexPatches = append(res.RegexPatches, patches...)
	return patches
}

// patchStandardInvoiceFields 结构体内置字段的补洞（文本 + 数值）。
func patchStandardInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
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
			if !acc.Set(&res.Invoices[missingIdx], value) {
				continue // 数值转换失败：宁可空着
			}
			patches = append(patches, RegexFieldPatch{
				Index: missingIdx, Field: field, Value: value, Source: pattern.source,
			})
			break // 一个字段补一次即止
		}
	}
	return patches
}

// patchConfiguredInvoiceFields 自定义扩展字段的补洞：写进 Fields 动态桶。
//
// 标准字段在这里跳过——它们已经由 patchStandardInvoiceFields 按契约键处理，
// 重复执行只会让同一次提取出现两条补洞记录。
func patchConfiguredInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	if cfg == nil {
		return nil
	}
	var patches []RegexFieldPatch
	seen := make(map[string]bool, len(cfg.Fields))
	for _, f := range cfg.Fields {
		name := strings.TrimSpace(f.Name)
		if name == "" || !f.Enabled || seen[name] {
			continue
		}
		seen[name] = true
		if _, _, isStandard := invoiceAccessorFor(name); isStandard {
			continue
		}
		pattern := strings.TrimSpace(f.Regex)
		if pattern == "" {
			continue
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		missingIdx := -1
		missingCount := 0
		for i := range res.Invoices {
			if customInvoiceFieldValue(&res.Invoices[i], name) == "" {
				missingIdx = i
				missingCount++
			}
		}
		if missingCount != 1 {
			continue
		}
		value, ok := uniqueRegexCapture(text, re)
		if !ok || value == "" {
			continue
		}
		if customFieldUsedElsewhere(res, name, missingIdx, value) {
			continue
		}
		if res.Invoices[missingIdx].Fields == nil {
			res.Invoices[missingIdx].Fields = map[string]any{}
		}
		res.Invoices[missingIdx].Fields[name] = value
		patches = append(patches, RegexFieldPatch{
			Index: missingIdx, Field: name, Value: value, Source: regexPatchSourceConfig,
		})
	}
	return patches
}

// patchInvoiceItemSubFields 明细子项补洞。
//
// 归属铁律：只有「本批恰好一张发票 + 该发票恰好一行明细」时才回填——多行明细里
// 同一个正则会命中多个值，而每一行的归属无法从文本确定，按序塞就是串号。
func patchInvoiceItemSubFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	if cfg == nil || len(res.Invoices) != 1 {
		return nil
	}
	inv := &res.Invoices[0]
	if len(inv.Items) != 1 {
		return nil
	}
	line := &inv.Items[0]
	var patches []RegexFieldPatch
	seen := map[string]bool{}
	for _, f := range cfg.Fields {
		if f.Type != "items" || !f.Enabled {
			continue
		}
		for _, sf := range f.SubFields {
			key := strings.TrimSpace(sf.Key)
			pattern := strings.TrimSpace(sf.Regex)
			if key == "" || pattern == "" || !sf.IsEnabled() || seen[key] {
				continue
			}
			seen[key] = true
			if invoiceItemSubFieldValue(line, key) != "" {
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			value, ok := uniqueRegexCapture(text, re)
			if !ok || value == "" {
				continue
			}
			if !setInvoiceItemSubFieldValue(line, key, value) {
				continue
			}
			patches = append(patches, RegexFieldPatch{
				Index: 0, Field: "items." + key, Value: value, Source: regexPatchSourceConfig,
			})
		}
	}
	return patches
}

// ---- 明细行子项读写 ----

// invoiceItemSubFieldValue 读取一行明细里的子项当前值（含 Extra 自定义键）。
func invoiceItemSubFieldValue(line *InvoiceExtractionLine, key string) string {
	switch key {
	case "name":
		return line.Name
	case "spec":
		return line.Spec
	case "unit":
		return line.Unit
	case "qty":
		return floatFieldText(line.Qty)
	case "price":
		return floatFieldText(line.Price)
	case "amount":
		return floatFieldText(line.Amount)
	case "tax_rate":
		return floatFieldText(line.TaxRate)
	case "tax_amount":
		return floatFieldText(line.TaxAmt)
	}
	if line.Extra == nil {
		return ""
	}
	v, ok := line.Extra[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toStringValue(v))
}

// setInvoiceItemSubFieldValue 写入一行明细的子项；数值键转不成数值则返回 false。
func setInvoiceItemSubFieldValue(line *InvoiceExtractionLine, key, value string) bool {
	switch key {
	case "name":
		line.Name = value
		return true
	case "spec":
		line.Spec = value
		return true
	case "unit":
		line.Unit = value
		return true
	case "qty":
		return setFloatField(&line.Qty, value)
	case "price":
		return setFloatField(&line.Price, value)
	case "amount":
		return setFloatField(&line.Amount, value)
	case "tax_rate":
		return setFloatField(&line.TaxRate, value)
	case "tax_amount":
		return setFloatField(&line.TaxAmt, value)
	}
	if line.Extra == nil {
		line.Extra = map[string]any{}
	}
	line.Extra[key] = value
	return true
}

// ---- 值清洗与工具 ----

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

// normalizePatchedValue 按字段类型归一化（日期统一 YYYY-MM-DD；数值交给 setter；其余去空白）。
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

// parsePatchedNumber 统一清洗货币符与千分位后转 float64。
//
// 覆盖实测票面形态：`¥1,234.56`、`￥1,234.56`、`1,234.56 元`、`13%`（税率）。
// 关键取舍：清洗后仍解析不了的（「壹仟贰佰元整」）一律失败返回 false——
// 写成 0 会被用户当成真实金额，比留空更糟。
func parsePatchedNumber(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	s = strings.NewReplacer(
		"¥", "", "￥", "", "$", "", "＄", "",
		",", "", "，", "",
		"元", "", " ", "", "\u00a0", "", "\u3000", "",
	).Replace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func setFloatField(target **float64, raw string) bool {
	f, ok := parsePatchedNumber(raw)
	if !ok {
		return false
	}
	*target = &f
	return true
}

// floatFieldText 把可空数值字段转成用于"是否已填"判断的字符串。
func floatFieldText(f *float64) string {
	if f == nil {
		return ""
	}
	return strconv.FormatFloat(*f, 'f', -1, 64)
}

// toStringValue 把 JSON 反序列化出的 any 值收敛成字符串（Fields 桶可能是数字/布尔）。
func toStringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return ""
	}
}

// customInvoiceFieldValue 读自定义扩展字段（Fields 动态桶）。
func customInvoiceFieldValue(item *InvoiceExtractionItem, name string) string {
	if item.Fields == nil {
		return ""
	}
	v, ok := item.Fields[name]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toStringValue(v))
}

// customFieldUsedElsewhere 判断该值是否已属于同批其它发票的同名字段。
func customFieldUsedElsewhere(res *InvoiceExtractionResult, name string, skipIdx int, value string) bool {
	for i := range res.Invoices {
		if i == skipIdx {
			continue
		}
		if customInvoiceFieldValue(&res.Invoices[i], name) == value {
			return true
		}
	}
	return false
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
