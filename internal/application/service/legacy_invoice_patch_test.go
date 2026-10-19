package service

// 本文件是 Schema 补洞重构（蓝图 P2-4）的**等价性锚点**：把重构前的发票结构体
// 专用补洞实现按原样保留在测试构建里（符号加 legacy 前缀），供
// extract_schema_equivalence_test.go 做永久双跑对照。
//
// 为什么不允许删：extract_regex_fallback.go 的执行体已换成 Schema 驱动的
// SchemaRegexPatch，一旦未来改动破坏"逐项等价"（含放弃路径），只有与旧实现
// 双跑才能发现——单元断言只能覆盖想到的用例，双跑覆盖全部行为。
//
// 来源：git show HEAD（重构前）的 extract_regex_fallback.go，仅做符号重命名，
// 逻辑零改动。共享工具（uniqueRegexCapture / parsePatchedNumber /
// floatFieldText / toStringValue / stringInSlice / fieldRegexPattern 类型 /
// builtinInvoiceFieldRegexes）仍在生产文件中，此处直接复用。

import (
	"regexp"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

type legacyInvoiceFieldAccessor struct {
	Get func(*InvoiceExtractionItem) string
	Set func(*InvoiceExtractionItem, string) bool
}

// legacyTextSetter 文本字段的统一 setter（永远成功）。
func legacyTextSetter(target func(*InvoiceExtractionItem) *string) func(*InvoiceExtractionItem, string) bool {
	return func(i *InvoiceExtractionItem, v string) bool {
		*target(i) = v
		return true
	}
}

// legacyMoneySetter 数值字段的统一 setter：清洗货币符/千分位 → ParseFloat，失败即放弃。
func legacyMoneySetter(target func(*InvoiceExtractionItem) **float64) func(*InvoiceExtractionItem, string) bool {
	return func(i *InvoiceExtractionItem, v string) bool {
		f, ok := parsePatchedNumber(v)
		if !ok {
			return false
		}
		*target(i) = &f
		return true
	}
}

var legacyInvoiceFieldAccessors = map[string]legacyInvoiceFieldAccessor{
	// ---- 文本标准字段 ----
	"invoice_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceNo },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceNo }),
	},
	"invoice_code": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceCode },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceCode }),
	},
	"invoice_date": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceDate },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceDate }),
	},
	"invoice_type": {
		Get: func(i *InvoiceExtractionItem) string { return i.InvoiceType },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.InvoiceType }),
	},
	"buyer_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerName },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerName }),
	},
	"buyer_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerTaxNo },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerTaxNo }),
	},
	"buyer_address": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerAddr },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerAddr }),
	},
	"buyer_phone": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerPhone },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerPhone }),
	},
	"buyer_bank": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerBank },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerBank }),
	},
	"buyer_account": {
		Get: func(i *InvoiceExtractionItem) string { return i.BuyerAcct },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.BuyerAcct }),
	},
	"seller_name": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerName },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerName }),
	},
	"seller_tax_no": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerTaxNo },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerTaxNo }),
	},
	"seller_address": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerAddr },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerAddr }),
	},
	"seller_phone": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerPhone },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerPhone }),
	},
	"seller_bank": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerBank },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerBank }),
	},
	"seller_account": {
		Get: func(i *InvoiceExtractionItem) string { return i.SellerAcct },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.SellerAcct }),
	},
	"issuer": {
		Get: func(i *InvoiceExtractionItem) string { return i.Issuer },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.Issuer }),
	},
	"remark": {
		Get: func(i *InvoiceExtractionItem) string { return i.Remark },
		Set: legacyTextSetter(func(i *InvoiceExtractionItem) *string { return &i.Remark }),
	},
	// ---- 数值标准字段（清洗后转 float64）----
	"total_amount": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.TotalAmount) },
		Set: legacyMoneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.TotalAmount }),
	},
	"amount": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.Amount) },
		Set: legacyMoneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.Amount }),
	},
	"tax": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.Tax) },
		Set: legacyMoneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.Tax }),
	},
	"tax_rate": {
		Get: func(i *InvoiceExtractionItem) string { return floatFieldText(i.TaxRate) },
		Set: legacyMoneySetter(func(i *InvoiceExtractionItem) **float64 { return &i.TaxRate }),
	},
}

// legacyInvoiceFieldOrder 补洞字段的固定遍历顺序：让日志与 PatchedByRegex 稳定可复现
// （map 迭代无序会让同一份输入产出不同顺序的补洞记录）。
// 文本字段在前、数值字段在后：金额类正则误命中代价更高，让确定性更强的字段先落位。
var legacyInvoiceFieldOrder = []string{
	"invoice_no", "invoice_code", "invoice_date", "invoice_type",
	"buyer_name", "buyer_tax_no", "buyer_address", "buyer_phone", "buyer_bank", "buyer_account",
	"seller_name", "seller_tax_no", "seller_address", "seller_phone", "seller_bank", "seller_account",
	"issuer", "remark",
	"total_amount", "amount", "tax", "tax_rate",
}

// legacyInvoiceAccessorFor 按**用户配置里可能出现的任意键名**解析补洞读写器。
//
// 前端保存的是 camelCase（invoiceNo / buyerTaxNo），这里的内置字典是 snake_case。
// 不做 canonical 归一，用户在规则面板填的正则永远匹配不到字段。返回的 key 是契约键
// （snake_case），用于日志与补洞明细；issue 里仍用用户配置的原名，便于界面回显。
func legacyInvoiceAccessorFor(field string) (string, legacyInvoiceFieldAccessor, bool) {
	if acc, ok := legacyInvoiceFieldAccessors[field]; ok {
		return field, acc, true
	}
	want := canonicalFieldKey(field)
	if want == "" {
		return "", legacyInvoiceFieldAccessor{}, false
	}
	// 按固定顺序遍历，保证同一份输入的结果可复现
	for _, key := range legacyInvoiceFieldOrder {
		if canonicalFieldKey(key) == want {
			return key, legacyInvoiceFieldAccessors[key], true
		}
	}
	return "", legacyInvoiceFieldAccessor{}, false
}

// legacyRegexFallbackInvoiceFields 对发票结果执行后置正则补洞，返回本次补洞明细。
//
// cfg 为用户提取配置（可为 nil）：其中 ExtractFieldConfig.Regex 若非空，优先于内置
// 正则使用（用户口径 > 系统默认）。补洞结果同时写入 res.PatchedByRegex（字段名去重）

func legacyRegexFallbackInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	patches := make([]RegexFieldPatch, 0, 4)
	patches = append(patches, legacyPatchStandardInvoiceFields(res, text, cfg)...)
	patches = append(patches, legacyPatchConfiguredInvoiceFields(res, text, cfg)...)
	patches = append(patches, legacyPatchInvoiceItemSubFields(res, text, cfg)...)
	for _, p := range patches {
		if !stringInSlice(res.PatchedByRegex, p.Field) {
			res.PatchedByRegex = append(res.PatchedByRegex, p.Field)
		}
	}
	res.RegexPatches = append(res.RegexPatches, patches...)
	return patches
}

func legacyPatchStandardInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
	var patches []RegexFieldPatch
	for _, field := range legacyInvoiceFieldOrder {
		acc, ok := legacyInvoiceFieldAccessors[field]
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
		for _, pattern := range legacyFieldRegexPatterns(field, cfg) {
			re, err := regexp.Compile(pattern.re)
			if err != nil {
				continue // 用户配置的正则非法：跳过，不影响其它字段
			}
			value, ok := uniqueRegexCapture(text, re)
			if !ok {
				continue
			}
			value = legacyNormalizePatchedValue(field, value)
			if value == "" {
				continue
			}
			// 该值已属于同批另一张票 → 放弃（防重复/错位）。
			if legacyValueUsedElsewhere(res, acc.Get, missingIdx, value) {
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

// legacyPatchConfiguredInvoiceFields 自定义扩展字段的补洞：写进 Fields 动态桶。
//
// 标准字段在这里跳过——它们已经由 legacyPatchStandardInvoiceFields 按契约键处理，
// 重复执行只会让同一次提取出现两条补洞记录。
func legacyPatchConfiguredInvoiceFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
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
		if _, _, isStandard := legacyInvoiceAccessorFor(name); isStandard {
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
			if legacyCustomInvoiceFieldValue(&res.Invoices[i], name) == "" {
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
		if legacyCustomFieldUsedElsewhere(res, name, missingIdx, value) {
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

// legacyPatchInvoiceItemSubFields 明细子项补洞。
//
// 归属铁律：只有「本批恰好一张发票 + 该发票恰好一行明细」时才回填——多行明细里
// 同一个正则会命中多个值，而每一行的归属无法从文本确定，按序塞就是串号。
func legacyPatchInvoiceItemSubFields(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) []RegexFieldPatch {
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
			if legacyInvoiceItemSubFieldValue(line, key) != "" {
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
			if !legacySetInvoiceItemSubFieldValue(line, key, value) {
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

// legacyInvoiceItemSubFieldValue 读取一行明细里的子项当前值（含 Extra 自定义键）。
func legacyInvoiceItemSubFieldValue(line *InvoiceExtractionLine, key string) string {
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

// legacySetInvoiceItemSubFieldValue 写入一行明细的子项；数值键转不成数值则返回 false。
func legacySetInvoiceItemSubFieldValue(line *InvoiceExtractionLine, key, value string) bool {
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
		return legacySetFloatField(&line.Qty, value)
	case "price":
		return legacySetFloatField(&line.Price, value)
	case "amount":
		return legacySetFloatField(&line.Amount, value)
	case "tax_rate":
		return legacySetFloatField(&line.TaxRate, value)
	case "tax_amount":
		return legacySetFloatField(&line.TaxAmt, value)
	}
	if line.Extra == nil {
		line.Extra = map[string]any{}
	}
	line.Extra[key] = value
	return true
}

// ---- 值清洗与工具 ----

// fieldRegexPattern 一条候选正则及其来源。
// legacyFieldRegexPatterns 返回该字段的候选正则：用户配置优先，其次内置字典。
func legacyFieldRegexPatterns(field string, cfg *types.KbExtractConfig) []fieldRegexPattern {
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

func legacyNormalizePatchedValue(field, value string) string {
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

func legacySetFloatField(target **float64, raw string) bool {
	f, ok := parsePatchedNumber(raw)
	if !ok {
		return false
	}
	*target = &f
	return true
}

func legacyCustomInvoiceFieldValue(item *InvoiceExtractionItem, name string) string {
	if item.Fields == nil {
		return ""
	}
	v, ok := item.Fields[name]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(toStringValue(v))
}

// legacyCustomFieldUsedElsewhere 判断该值是否已属于同批其它发票的同名字段。
func legacyCustomFieldUsedElsewhere(res *InvoiceExtractionResult, name string, skipIdx int, value string) bool {
	for i := range res.Invoices {
		if i == skipIdx {
			continue
		}
		if legacyCustomInvoiceFieldValue(&res.Invoices[i], name) == value {
			return true
		}
	}
	return false
}

// legacyValueUsedElsewhere 判断该值是否已被同批其它发票占用。
func legacyValueUsedElsewhere(
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
