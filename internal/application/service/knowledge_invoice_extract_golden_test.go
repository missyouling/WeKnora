package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// 本文件是提取器的 golden 回归集（解析层）。
//
// 目的：提取器精度依赖「模型输出 → 结构化结果」这条解析链。模型输出形态会漂移
// （代码围栏、别名键、"null" 字符串、非发票文档、动态字段等），这些漂移是历史上
// 提取失败的主要来源。这里把各种形态固化成样本，任何改动解析/归一化逻辑的行为
// 都会被立刻发现。
//
// 这一层是**确定性的**（不调用模型），因此可以在 CI 里跑。
// 端到端精度（真实文档 → 字段）依赖模型，另以手工/定期脚本验证。

func TestGoldenParseInvoiceRuleReply(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		cfg   *types.KbExtractConfig
		check func(t *testing.T, got *InvoiceExtractionResult)
	}{
		{
			name: "标准 JSON",
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"12345678","invoice_date":"2026-01-15","total_amount":100.5}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if got.Kind != "invoice" {
					t.Fatalf("kind = %q, want invoice", got.Kind)
				}
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1", len(got.Invoices))
				}
				inv := got.Invoices[0]
				if inv.InvoiceNo != "12345678" {
					t.Errorf("invoice_no = %q, want 12345678", inv.InvoiceNo)
				}
				if inv.InvoiceDate != "2026-01-15" {
					t.Errorf("invoice_date = %q, want 2026-01-15", inv.InvoiceDate)
				}
				if inv.TotalAmount == nil || *inv.TotalAmount != 100.5 {
					t.Errorf("total_amount = %v, want 100.5", inv.TotalAmount)
				}
			},
		},
		{
			name: "Markdown 代码围栏包裹",
			raw:  "```json\n" + `{"kind":"invoice","invoices":[{"invoice_no":"87654321"}]}` + "\n```",
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1 (围栏必须被剥离)", len(got.Invoices))
				}
				if got.Invoices[0].InvoiceNo != "87654321" {
					t.Errorf("invoice_no = %q, want 87654321", got.Invoices[0].InvoiceNo)
				}
			},
		},
		{
			name: `字符串 "null" 应清洗为空`,
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"null","remark":"null"}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 1 {
					t.Fatalf("invoices = %d, want 1", len(got.Invoices))
				}
				if got.Invoices[0].InvoiceNo != "" {
					t.Errorf(`invoice_no = %q, want "" (字符串 null 必须清洗为空)`, got.Invoices[0].InvoiceNo)
				}
			},
		},
		{
			name: "非发票文档",
			raw:  `{"kind":"not_invoice","invoices":[]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if got.Kind != "not_invoice" {
					t.Errorf("kind = %q, want not_invoice", got.Kind)
				}
				if len(got.Invoices) != 0 {
					t.Errorf("invoices = %d, want 0", len(got.Invoices))
				}
			},
		},
		{
			name: "多张发票",
			raw:  `{"kind":"invoice","invoices":[{"invoice_no":"A1"},{"invoice_no":"A2"},{"invoice_no":"A3"}]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 3 {
					t.Fatalf("invoices = %d, want 3", len(got.Invoices))
				}
				want := []string{"A1", "A2", "A3"}
				for i, w := range want {
					if got.Invoices[i].InvoiceNo != w {
						t.Errorf("invoices[%d].invoice_no = %q, want %q", i, got.Invoices[i].InvoiceNo, w)
					}
				}
			},
		},
		{
			name: "空发票列表",
			raw:  `{"kind":"invoice","invoices":[]}`,
			cfg:  nil,
			check: func(t *testing.T, got *InvoiceExtractionResult) {
				if len(got.Invoices) != 0 {
					t.Errorf("invoices = %d, want 0", len(got.Invoices))
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseInvoiceRuleReply(tc.raw, tc.cfg)
			if err != nil {
				t.Fatalf("parseInvoiceRuleReply 解析失败: %v\n原始输出: %s", err, tc.raw)
			}
			if got == nil {
				t.Fatal("解析结果为 nil")
			}
			tc.check(t, got)
		})
	}
}

// TestGoldenBuildInvoiceRuleSystemPrompt 锁定提示词拼装行为：
// 提示词一变，模型输出就变，因此它必须像代码一样被回归保护。
func TestGoldenBuildInvoiceRuleSystemPrompt(t *testing.T) {
	cfg := &types.KbExtractConfig{
		CertType: "通行费",
		Fields: []types.ExtractFieldConfig{
			{Name: "plate_no", Desc: "车牌号", Rule: "备注中以「车牌」开头的号码", Enabled: true},
			{Name: "travel_date", Desc: "通行日期", Enabled: true},
			{Name: "disabled_field", Enabled: false},
		},
	}
	p := buildInvoiceRuleSystemPrompt(cfg)

	// 启用字段必须出现在提示词里
	for _, want := range []string{"plate_no", "travel_date", "车牌号", "通行日期"} {
		if !strings.Contains(p, want) {
			t.Errorf("提示词缺少 %q", want)
		}
	}
	// 禁用字段不得出现
	if strings.Contains(p, "disabled_field") {
		t.Error("已禁用字段不应出现在提示词里")
	}
	// 分离要求必须存在（车牌号常藏在备注里）
	if !strings.Contains(p, "分离要求") {
		t.Error("缺少「分离要求」指令，备注中的关键数据不会被分离到独立字段")
	}
}

// TestGoldenBuildFieldsSchema 锁定 JSON Schema 生成：字段名/类型/必填。
func TestGoldenBuildFieldsSchema(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Fields: []types.ExtractFieldConfig{
			{Name: "name", Type: "string", Enabled: true},
			{Name: "amount", Type: "number", Enabled: true},
			{Name: "tags", Type: "array", Enabled: true},
			{Name: "date", Type: "date", Enabled: true},
			{Name: "off", Type: "string", Enabled: false},
		},
	}
	s := BuildFieldsSchema(cfg.Fields)
	for _, want := range []string{`"name"`, `"amount"`, `"tags"`, `"date"`, `"number"`, `"array"`, `"format": "date"`} {
		if !strings.Contains(s, want) {
			t.Errorf("schema 缺少 %q", want)
		}
	}
	if strings.Contains(s, `"off"`) {
		t.Error("禁用字段不应出现在 schema 中")
	}
}

// 注：本包已有 contains([]string, string) 辅助函数（knowledge_span_tracker.go），
// 这里统一用标准库 strings.Contains，避免重名冲突。
