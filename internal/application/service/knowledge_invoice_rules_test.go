package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildInvoiceRuleSystemPromptDefault(t *testing.T) {
	got := buildInvoiceRuleSystemPrompt(nil)
	if got != invoiceExtractionSystemPrompt {
		t.Errorf("nil cfg should return default prompt")
	}
	empty := &types.KbExtractConfig{}
	if got := buildInvoiceRuleSystemPrompt(empty); got != invoiceExtractionSystemPrompt {
		t.Errorf("empty cfg should return default prompt")
	}
}

func TestBuildInvoiceRuleSystemPromptFields(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Scope:  types.FleetCategoryScopeInvoice,
		Fields: []types.ExtractFieldConfig{
			{Name: "sellerName", Desc: "销方名称", Rule: "取发票右上角销售方全称", Enabled: true},
			{Name: "totalAmount", Desc: "价税合计", Enabled: true},
			{Name: "disabledField", Enabled: false},
		},
	}
	got := buildInvoiceRuleSystemPrompt(cfg)
	for _, want := range []string{"sellerName", "销方名称", "价税合计", "invoices", "kind"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(got, "disabledField") {
		t.Errorf("disabled field must not appear in prompt")
	}
	// 输出结构契约必须保留
	if !strings.Contains(got, "invoices 数组") && !strings.Contains(got, "invoices") {
		t.Errorf("invoice output contract must be preserved")
	}
}

func TestBuildInvoiceRuleSystemPromptTemplate(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Scope:          types.FleetCategoryScopeInvoice,
		PromptTemplate: "请重点核对发票号码与金额：{{fields_schema}}",
		Fields: []types.ExtractFieldConfig{
			{Name: "invoiceNo", Enabled: true},
		},
	}
	got := buildInvoiceRuleSystemPrompt(cfg)
	if !strings.Contains(got, "请重点核对发票号码与金额") {
		t.Errorf("template body missing")
	}
	if strings.Contains(got, "{{fields_schema}}") {
		t.Errorf("fields_schema slot must be replaced")
	}
	if !strings.Contains(got, "invoiceNo") {
		t.Errorf("schema fields missing after replacement")
	}
	if !strings.Contains(got, "发票") {
		t.Errorf("default invoice prompt should still be appended")
	}
}
