package service

import (
	"strings"
	"sync"

	"github.com/Tencent/WeKnora/internal/types"
)

// ScopeSchema 注册表（蓝图 P2-1）。
//
// 为什么用进程内注册表而不是 DB 配置：标准字段契约是**代码级事实**（结构体
// json tag、补洞正则、明细结构），随二进制发布；运行期可改的部分（必填/正则
// 覆盖）仍在 KbExtractConfig，两份数据在补洞/校验时合并，互不越界。

var (
	scopeSchemaOnce sync.Once
	scopeSchemas    = map[string]types.ScopeSchema{}
	scopeSchemaMu   sync.RWMutex
)

// RegisterScopeSchema 注册（或覆盖）一个 scope 的 Schema。
// 覆盖语义服务于测试与热修复；生产注册在 builtinScopeSchemas 里一次性完成。
func RegisterScopeSchema(s types.ScopeSchema) {
	scopeSchemaMu.Lock()
	defer scopeSchemaMu.Unlock()
	scopeSchemas[s.Scope] = s
}

// GetScopeSchema 取一个 scope 的 Schema。第二个返回值报告是否已注册。
func GetScopeSchema(scope string) (types.ScopeSchema, bool) {
	ensureBuiltinScopeSchemas()
	scopeSchemaMu.RLock()
	defer scopeSchemaMu.RUnlock()
	s, ok := scopeSchemas[scope]
	return s, ok
}

// ScopeSchemaList 返回全部已注册 Schema（供诊断/测试；顺序不保证稳定）。
func ScopeSchemaList() []types.ScopeSchema {
	ensureBuiltinScopeSchemas()
	scopeSchemaMu.RLock()
	defer scopeSchemaMu.RUnlock()
	out := make([]types.ScopeSchema, 0, len(scopeSchemas))
	for _, s := range scopeSchemas {
		out = append(out, s)
	}
	return out
}

// ScopeSchemaStandardKeys 取某 scope 的标准字段键序（= Schema 声明序 =
// 补洞遍历序）。未注册或没有标准字段时返回 nil。
func ScopeSchemaStandardKeys(scope string) []string {
	schema, ok := GetScopeSchema(scope)
	if !ok || len(schema.StandardFields) == 0 {
		return nil
	}
	out := make([]string, 0, len(schema.StandardFields))
	for _, fd := range schema.StandardFields {
		out = append(out, fd.Key)
	}
	return out
}

func ensureBuiltinScopeSchemas() {
	scopeSchemaOnce.Do(func() {
		for _, s := range builtinScopeSchemas() {
			scopeSchemas[s.Scope] = s
		}
	})
}

// builtinScopeSchemas 内置全量注册表。
//
// 发票是唯一「结构体强类型 + 明细数组 + 扩展桶」三件齐全的 scope；document 类
// scope 的提取结果是配置驱动的 map，标准字段集为空（字典为空时 AI 建议走自定义
// 字段，与蓝图 P2-2 预期一致），补洞走通用线的 KbExtractConfig 级别路径。
func builtinScopeSchemas() []types.ScopeSchema {
	return []types.ScopeSchema{
		{
			Scope:          types.FleetCategoryScopeInvoice,
			Kind:           "invoice",
			CategoryLabel:  "发票类型",
			StandardFields: invoiceStandardFieldDefs(),
			DynamicBucket:  "Fields", // InvoiceExtractionItem.Fields（json "fields"）
			ItemBucket:     "items",  // InvoiceExtractionItem.Items
			ItemField:      "Extra",  // InvoiceExtractionLine.Extra（json tag "-"，只能按 Go 字段名反射）
			RegexPatchable: true,
			Deterministic:  []string{"line", "md_table", "flat"},
			RecordFactory:  func() any { return InvoiceExtractionItem{} },
		},
		documentScopeSchema("contract", "合同类型"),
		documentScopeSchema("regulation", "制度名称"),
		documentScopeSchema("notice", "通知标题"),
		documentScopeSchema("award_punish", "通知名称"),
		documentScopeSchema("utility", "账单类型"),
		documentScopeSchema("solar", "账单类型"),
	}
}

// documentScopeSchema document 类 scope 的公共骨架：无标准字段、无明细层级，
// 仍保留扩展桶约定（合同/制度等提取结果的自定义字段语义桶）。
func documentScopeSchema(scope, categoryLabel string) types.ScopeSchema {
	return types.ScopeSchema{
		Scope:          scope,
		Kind:           "document",
		CategoryLabel:  categoryLabel,
		DynamicBucket:  "Fields",
		RegexPatchable: false,
		RecordFactory:  func() any { return map[string]any{} },
	}
}

// invoiceStandardFieldDefs 发票标准字段声明 = **补洞面**。
//
// 字段序、类型、内置正则必须与旧实现的 invoiceFieldOrder / invoiceFieldAccessors /
// builtinInvoiceFieldRegexes 逐项对齐——SchemaRegexPatch 按本顺序遍历，顺序漂移
// 会让补洞记录的顺序变化（日志与 PatchedByRegex 必须可复现）。键名必须是
// InvoiceExtractionItem 的 json tag，注册时反射校验（见 schema 自检测试）。
//
// 注意：category（票据大类）与 items（项目明细）**不在本列表**——它们属于
// 喂模型的字典而非补洞面（详见 invoiceDictionaryExtraFields）。
func invoiceStandardFieldDefs() []types.FieldDef {
	specs := []struct {
		key   string
		label string
		ftype types.FieldType
	}{
		{"invoice_no", "发票号码", types.FieldTypeText},
		{"invoice_code", "发票代码", types.FieldTypeText},
		{"invoice_date", "开票日期", types.FieldTypeDate},
		{"invoice_type", "发票类型", types.FieldTypeText},
		{"buyer_name", "购买方名称", types.FieldTypeText},
		{"buyer_tax_no", "购买方纳税人识别号", types.FieldTypeText},
		{"buyer_address", "购买方地址", types.FieldTypeText},
		{"buyer_phone", "购买方电话", types.FieldTypeText},
		{"buyer_bank", "购买方开户行", types.FieldTypeText},
		{"buyer_account", "购买方账号", types.FieldTypeText},
		{"seller_name", "销售方名称", types.FieldTypeText},
		{"seller_tax_no", "销售方纳税人识别号", types.FieldTypeText},
		{"seller_address", "销售方地址", types.FieldTypeText},
		{"seller_phone", "销售方电话", types.FieldTypeText},
		{"seller_bank", "销售方开户行", types.FieldTypeText},
		{"seller_account", "销售方账号", types.FieldTypeText},
		{"issuer", "开票人", types.FieldTypeText},
		{"remark", "备注", types.FieldTypeText},
		{"total_amount", "价税合计", types.FieldTypeNumber},
		{"amount", "不含税金额", types.FieldTypeNumber},
		{"tax", "税额", types.FieldTypeNumber},
		{"tax_rate", "税率", types.FieldTypeNumber},
	}
	hints := invoiceFieldHints()
	out := make([]types.FieldDef, 0, len(specs))
	for _, s := range specs {
		out = append(out, types.FieldDef{
			Key: s.key, Label: s.label, Type: s.ftype, Hint: hints[s.key],
			Aliases: schemaFieldAliases(s.key),
			Regex:   firstBuiltinFieldRegex(s.key),
		})
	}
	return out
}

// invoiceFieldHints 喂 LLM 的字段语义提示（原 InvoiceStandardFieldDictionary 文案）。
// 只有进字典的 16 个字段有 hint；补洞面独有字段（地址/电话/银行/账号）无 hint。
func invoiceFieldHints() map[string]string {
	return map[string]string{
		"invoice_no":    "明确标注『发票号码/发票号/No.』后的数字串；机器编号、校验码、密码区一律不算",
		"invoice_code":  "发票代码（10~12 位）；全电发票通常无此栏",
		"invoice_date":  "开票日期，统一为 YYYY-MM-DD",
		"invoice_type":  "票面标题所示的票据种类",
		"total_amount":  "价税合计（小数），无货币符号",
		"amount":        "不含税金额（小数）",
		"tax":           "税额（小数）",
		"tax_rate":      "税率（数字，13 表示 13%）",
		"seller_name":   "销售方（开票方）名称",
		"seller_tax_no": "销售方纳税人识别号/统一社会信用代码",
		"buyer_name":    "购买方（受票方）名称",
		"buyer_tax_no":  "购买方纳税人识别号/统一社会信用代码",
		"issuer":        "开票人/收款人/经办人姓名",
		"remark":        "票面备注栏原文",
	}
}

// invoiceFieldAliases 字段中文别名（喂模型的归一提示）。
func schemaFieldAliases(key string) []string {
	switch key {
	case "invoice_no":
		return []string{"票号", "收据编号"}
	}
	return nil
}

// invoiceDictionaryExtraFields 字典专属、不进补洞面的两个条目。
//
// category：旧实现没有它的 accessor（从不补洞），但喂模型字典里有——模型要能
// 把票面大类映射到 category；items：数组字段，补洞逻辑走 ItemBucket 专项，
// 不属于标准字段遍历。
func invoiceDictionaryExtraFields() []types.FieldDef {
	return []types.FieldDef{
		{Key: "category", Label: "票据大类", Type: types.FieldTypeText,
			Hint: "票据大类，如增值税专用发票"},
		{Key: "items", Label: "项目明细", Type: types.FieldTypeArray,
			Hint: "货物/服务明细行", SubFields: invoiceItemSubFieldDefs()},
	}
}

// invoiceItemSubFieldDefs items 明细行的标准子列（喂 Prompt 的 JSON Schema）。
func invoiceItemSubFieldDefs() []types.FieldDef {
	return []types.FieldDef{
		{Key: "name", Label: "项目名称", Type: types.FieldTypeText},
		{Key: "spec", Label: "规格型号", Type: types.FieldTypeText},
		{Key: "unit", Label: "单位", Type: types.FieldTypeText},
		{Key: "qty", Label: "数量", Type: types.FieldTypeNumber},
		{Key: "price", Label: "单价", Type: types.FieldTypeNumber},
		{Key: "amount", Label: "金额", Type: types.FieldTypeNumber},
		{Key: "tax_rate", Label: "税率", Type: types.FieldTypeNumber},
		{Key: "tax_amount", Label: "税额", Type: types.FieldTypeNumber},
	}
}

// firstBuiltinFieldRegex 取某字段的第一条内置正则（FieldDef.Regex 只存一条代表性
// 正则；补洞实现仍读完整 builtinInvoiceFieldRegexes 表，避免 Schema 承载策略细节）。
func firstBuiltinFieldRegex(field string) string {
	list := builtinInvoiceFieldRegexes[field]
	if len(list) == 0 {
		return ""
	}
	return strings.TrimSpace(list[0])
}
