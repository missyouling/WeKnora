package service

import (
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func sampleFields() []types.ExtractFieldConfig {
	return []types.ExtractFieldConfig{
		{Name: "经营地址", Desc: "公司实际经营地址", Type: "string", Rule: "如果文档中出现公司地址、注册地址，统一归入此字段；未找到则留空", Enabled: true},
		{Name: "发证日期", Desc: "证照核发日期", Type: "date", Rule: "统一为 YYYY-MM-DD", Enabled: true},
		{Name: "经营范围", Desc: "", Type: "array", Rule: "", Enabled: true},
		{Name: "备注", Desc: "", Type: "string", Rule: "", Enabled: false},
	}
}

// 默认模式：必须包含 JSON 骨架、doc_type 约束与 fields 直出约束。
func TestBuildExtractSystemPrompt_DefaultMode(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Scope: types.FleetCategoryScopeVehicle, CertType: "行驶证",
		Fields: sampleFields(), AdvancedEnabled: false, PromptTemplate: "",
	}
	p := BuildExtractSystemPrompt(cfg)
	for _, want := range []string{
		"fleet_vehicle_document",
		`"doc_type": "行驶证"`,
		`"经营地址": "…"`,
		"fields 必须直接输出上面列出的字段名作为键的键值对",
		"doc_type 必须严格等于「行驶证」",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("default prompt missing %q\n---\n%s", want, p)
		}
	}
	// 默认模式允许出现“严禁嵌套 properties”的指令文字，但不得泄漏 JSON Schema 包装结构本身
	if strings.Contains(p, `"properties": {`) || strings.Contains(p, `"type": "object"`) {
		t.Errorf("default prompt must not leak JSON Schema wrapper into the skeleton")
	}
}

// 高级模式：用户模板未声明 JSON 输出时，兜底约束必须被追加。
func TestBuildExtractSystemPrompt_AdvancedModeAddsJSONFallback(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Scope: types.FleetCategoryScopeVehicle, CertType: "行驶证",
		Fields: sampleFields(), AdvancedEnabled: true,
		PromptTemplate: "你是一个证照提取助手。请提取以下字段：{{fields_schema}}",
	}
	p := BuildExtractSystemPrompt(cfg)
	if !strings.Contains(p, "你是一个证照提取助手") {
		t.Errorf("advanced prompt must keep user template text")
	}
	for _, want := range []string{
		"【系统强制输出约束】",
		"只能输出一个合法的 JSON 对象",
		`"kind": "fleet_vehicle_document"`,
		`"doc_type": "行驶证"`,
		"严禁把字段值嵌套进 properties/required/type",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("advanced prompt missing fallback %q\n---\n%s", want, p)
		}
	}
	// 变量插槽替换
	if strings.Contains(p, "{{fields_schema}}") {
		t.Errorf("{{fields_schema}} slot must be replaced")
	}
	if !strings.Contains(p, `"经营地址"`) || !strings.Contains(p, `"发证日期"`) {
		t.Errorf("fields schema must be inlined into advanced prompt")
	}
	// 禁用字段不进入 schema
	if strings.Contains(p, `"备注"`) {
		t.Errorf("disabled field must not appear in fields schema")
	}
}

// 高级模式：用户模板本身已含 JSON 指令，兜底约束不能破坏其语义。
func TestBuildExtractSystemPrompt_AdvancedModeKeepsUserJSON(t *testing.T) {
	cfg := &types.KbExtractConfig{
		Scope: types.FleetCategoryScopeDriver, CertType: "驾驶证",
		Fields: sampleFields(), AdvancedEnabled: true,
		PromptTemplate: "严格按 JSON 输出：{\"doc_type\": \"驾驶证\", \"fields\": {{fields_schema}}}",
	}
	p := BuildExtractSystemPrompt(cfg)
	if !strings.Contains(p, `"kind": "fleet_driver_document"`) {
		t.Errorf("driver scope kind must be fleet_driver_document")
	}
	if !strings.Contains(p, "严格按 JSON 输出") {
		t.Errorf("user JSON instruction must be preserved")
	}
}
