package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是蓝图（docs/plans/2026-10-19-invoice-module-review-and-platformization.md）
// 附录 B 的落地：ScopeSchema 驱动的统一补洞，必须与发票原有补洞实现**逐项等价**。
//
// 等价口径（刻意聚焦补洞层，原因见蓝图 §3.2）：
//   - 补洞明细 RegexFieldPatch 列表逐项相等（field/index/value/source、顺序稳定）；
//   - 补洞对发票结构体的**突变结果**逐字段相等（含 Fields 桶与 items[].Extra）；
//   - 两边跑同一 validateInvoiceExtraction 后的 required-missing 问题集相等。
//
// 发票专属业务问题（号码/代码二选一、明细合计交叉校验）由 validateInvoiceExtraction
// 保留产出，不属于通用补洞线的等价范围。

// scopeEquivalenceFixture 一组「合成结果 + 原文 + 配置」。
// 用合成结果而非 MinerU 原文：补洞的输入是"模型已产出但缺了某字段"的结构体，
// 直接构造可精确控制 4 类补洞场景与 3 类放弃边界，比真实 fixture 更覆盖。
type scopeEquivalenceFixture struct {
	name    string
	text    string
	invoice []InvoiceExtractionItem
	cfg     *types.KbExtractConfig
}

func scopeEquivalenceFixtures() []scopeEquivalenceFixture {
	return []scopeEquivalenceFixture{
		{
			// 1) 标准文本字段：恰好一张缺 seller_tax_no → builtin 正则补。
			name: "std_text_builtin",
			text: "销售方名称：示例高速公路有限公司\n销售方纳税人识别号：91500000696569675Y\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "A1", SellerName: "示例高速公路有限公司", SellerTaxNo: ""},
				{InvoiceNo: "A2", SellerName: "示例高速公路有限公司", SellerTaxNo: "91500000696569675Y"},
			},
			cfg: nil,
		},
		{
			// 2) 标准数值字段：total_amount 空 → 用户配置优先、builtin 兜底都没有
			//    数值内置正则 → 必须都不补（放弃路径也要等价）。
			name: "std_number_no_pattern",
			text: "价税合计 ¥73.28\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "B1", TotalAmount: nil},
			},
			cfg: nil,
		},
		{
			// 3) 标准数值字段：total_amount 空 → 用户配置正则补（config 来源）。
			name: "std_number_config_regex",
			text: "价税合计（小写） ¥88.50\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "C1", TotalAmount: nil},
			},
			cfg: &types.KbExtractConfig{
				Fields: []types.ExtractFieldConfig{
					{Name: "totalAmount", Type: "number", Enabled: true,
						Regex: `价税合计（小写）\s*¥([\d.]+)`},
				},
			},
		},
		{
			// 4) 扩展字段：plateNo 无标准位 → cfg 正则应补进 Fields 桶。
			name: "custom_field_bucket",
			text: "车牌号渝C12345\n通行日期起20250505\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "D1", Fields: map[string]any{"plate_no": ""}},
			},
			cfg: &types.KbExtractConfig{
				Fields: []types.ExtractFieldConfig{
					{Name: "plateNo", Type: "text", Enabled: true,
						Regex: `车牌号\s*([渝川京沪A-Z]\d{5,6})`},
				},
			},
		},
		{
			// 5) 明细子项：单票单行缺 plate_no（Extra 空）→ cfg 正则应补进
			//    items[0].Extra（字段名为 items.plate_no）。
			name: "item_subfield_extra",
			text: "车牌号渝C87567\n类型货车\n",
			invoice: []InvoiceExtractionItem{
				{
					InvoiceNo: "E1",
					Items:     []InvoiceExtractionLine{{Name: "经营租赁代收通行费", Amount: fptr(93.32)}},
				},
			},
			cfg: &types.KbExtractConfig{
				Fields: []types.ExtractFieldConfig{
					{Name: "items", Type: "items", Enabled: true, SubFields: []types.FleetItemSubField{
						{Key: "plate_no", DataType: "text",
							Regex: `车牌号\s*([渝川京沪A-Z]\d{5,6})`},
					}},
				},
			},
		},
		{
			// 6) 放弃边界：两张都缺 seller_tax_no → 无法归属，双跑都不补。
			name: "guard_multi_missing",
			text: "销售方纳税人识别号：91500000696569675Y\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "F1", SellerTaxNo: ""},
				{InvoiceNo: "F2", SellerTaxNo: ""},
			},
			cfg: nil,
		},
		{
			// 7) 放弃边界：待补值已被同批另一张占用 → 不补（防串号）。
			name: "guard_value_used_elsewhere",
			text: "销售方纳税人识别号：91500000696569675Y\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "G1", SellerTaxNo: "91500000696569675Y"},
				{InvoiceNo: "G2", SellerTaxNo: ""},
			},
			cfg: nil,
		},
		{
			// 8) 放弃边界：正文里同一正则有多个不同取值 → uniqueRegexCapture 拒绝。
			name: "guard_not_unique",
			text: "车牌号渝C11111\n车牌号渝C22222\n",
			invoice: []InvoiceExtractionItem{
				{InvoiceNo: "H1", Fields: map[string]any{"plate_no": ""}},
			},
			cfg: &types.KbExtractConfig{
				Fields: []types.ExtractFieldConfig{
					{Name: "plateNo", Type: "text", Enabled: true,
						Regex: `车牌号\s*([渝川京沪A-Z]\d{5,6})`},
				},
			},
		},
	}
}

// fptr 浮点指针字面量（测试专用）。
func fptr(f float64) *float64 { return &f }

// runLegacyInvoicePatch 复刻旧路径：validateInvoiceExtraction →
// legacyRegexFallbackInvoiceFields（重构前实现的原样副本，见
// legacy_invoice_patch_test.go）→ 再 validate（FinalizeInvoiceRuleResult 的三步，
// 去掉日志）。
func runLegacyInvoicePatch(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig) ([]RegexFieldPatch, []ExtractFieldIssue) {
	issues := validateInvoiceExtraction(res, cfg)
	patches := legacyRegexFallbackInvoiceFields(res, text, cfg)
	if len(patches) > 0 {
		issues = validateInvoiceExtraction(res, cfg)
	}
	return patches, issues
}

// runSchemaInvoicePatch 新路径：同一批发票走 Schema 驱动的统一补洞。
// 结构体突变通过 NewGenericRecords 的指针视图直接落在 newRes.Invoices 上。
func runSchemaInvoicePatch(res *InvoiceExtractionResult, text string, cfg *types.KbExtractConfig, schema types.ScopeSchema) ([]RegexFieldPatch, []ExtractFieldIssue) {
	records := NewGenericRecords(res.Invoices)
	patches := SchemaRegexPatch(records, text, cfg, schema)
	// 与 FinalizeInvoiceRuleResult 同步：补洞后重跑业务校验，问题集以最新状态为准。
	issues := validateInvoiceExtraction(res, cfg)
	return patches, issues
}

// requiredIssuesOf 只保留与补洞线相关的 required-missing 问题（两边共有的口径）。
// 业务规则问题（号码/代码/金额交叉校验）在旧校验器里产出，是发票专属层，不参与通用线等价。
func requiredIssuesOf(issues []ExtractFieldIssue) []ExtractFieldIssue {
	out := make([]ExtractFieldIssue, 0, len(issues))
	for _, is := range issues {
		if is.Kind == "missing" && is.Detail == "用户配置为必填，但未提取到该字段" {
			out = append(out, is)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normPatches 归一化空切片口径：旧实现返回 make([]…, 0)，新实现无补洞时返回
// nil，两者在"没有任何补洞"上语义等价（调用方都按 len() 判定）。
func normPatches(ps []RegexFieldPatch) []RegexFieldPatch {
	if len(ps) == 0 {
		return nil
	}
	return ps
}

// TestScopeSchemaEquivalence_Invoice 蓝图附录 B 落地：发票补洞新旧实现双跑等价。
//
// 这是 P2-4「删除发票专用补洞实现」的唯一放行条件：本测试绿了，
// extract_regex_fallback.go 里约 560 行结构体专用代码才允许被 Schema 实现替换。
func TestScopeSchemaEquivalence_Invoice(t *testing.T) {
	schema, ok := GetScopeSchema(types.FleetCategoryScopeInvoice)
	require.True(t, ok, "invoice ScopeSchema 必须已注册")

	for _, tc := range scopeEquivalenceFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			oldRes := &InvoiceExtractionResult{Kind: "invoice", Invoices: append([]InvoiceExtractionItem(nil), tc.invoice...)}
			newRes := &InvoiceExtractionResult{Kind: "invoice", Invoices: append([]InvoiceExtractionItem(nil), tc.invoice...)}

			oldPatches, oldIssues := runLegacyInvoicePatch(oldRes, tc.text, tc.cfg)
			newPatches, newIssues := runSchemaInvoicePatch(newRes, tc.text, tc.cfg, schema)

			// 1) 补洞明细逐项相等（含顺序：字段遍历顺序必须可复现）
			assert.Equal(t, normPatches(oldPatches), normPatches(newPatches), "补洞明细必须逐项等价")
			// 2) 结构体突变逐字段相等（文本字段、数值字段、Fields 桶、items[].Extra）
			assert.Equal(t, oldRes.Invoices, newRes.Invoices, "补洞后的发票结构体必须一致")
			// 3) required-missing 问题集相等
			assert.Equal(t, requiredIssuesOf(oldIssues), requiredIssuesOf(newIssues), "必填缺失问题必须一致")
		})
	}
}

// TestScopeSchemaEquivalence_MultiInvoiceOrderStable 锁定补洞遍历顺序稳定
// （map 迭代无序会让同一输入产出不同顺序的补洞记录）。
func TestScopeSchemaEquivalence_MultiInvoiceOrderStable(t *testing.T) {
	schema, ok := GetScopeSchema(types.FleetCategoryScopeInvoice)
	require.True(t, ok)

	text := "发票代码: 050002400112\n开票日期: 2025年02月08日\n销售方纳税人识别号：91500000696569675Y\n"
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "invoiceCode", Type: "text", Enabled: true, Required: true},
			{Name: "invoiceDate", Type: "date", Enabled: true, Required: true},
		},
	}
	invoice := []InvoiceExtractionItem{{InvoiceNo: "Z1"}}

	var first []RegexFieldPatch
	for i := 0; i < 20; i++ {
		res := &InvoiceExtractionResult{Kind: "invoice", Invoices: append([]InvoiceExtractionItem(nil), invoice...)}
		records := NewGenericRecords(res.Invoices)
		patches := SchemaRegexPatch(records, text, cfg, schema)
		if i == 0 {
			first = normPatches(patches)
			continue
		}
		assert.Equal(t, first, normPatches(patches), "第 %d 次运行的补洞顺序/内容发生变化", i)
	}
	require.NotEmpty(t, first, "该 fixture 至少应补到发票代码/开票日期")
}
