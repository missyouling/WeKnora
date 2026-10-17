package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

// Phase C：通用（非发票）质量闭环的测试。
// 命名以 TestExtract 开头，确保被 Daily-Affairs CI 白名单正则
// (-run "TestBusiness|TestExtract|TestDailyAffairs|TestGolden") 覆盖到。
//
// 覆盖三件事：
//  1. 标准日期字段整串归一化（合同签订/到期日、账单所属期…）；
//  2. 用户配置 Enabled && Required 的字段：先用 field.Regex 从原文补洞，
//     补不到才记 error；
//  3. 安全边界：多条记录时禁止正则补洞（无法判定匹配值属于哪条）。

func TestExtractGenericNormalizesStandardDateFields(t *testing.T) {
	contracts := []ContractExtractionItem{{
		ContractNo:    "HT-2026-001",
		SignDate:      "2026年7月2日",
		EffectiveDate: "2026/7/3",
		ExpiryDate:    "20270702",
	}}
	records := NewGenericRecords(contracts)
	issues, patches := ValidateGenericExtraction(records, "", nil)

	if got := contracts[0].SignDate; got != "2026-07-02" {
		t.Errorf("sign_date 应归一为 2026-07-02，实际 %q", got)
	}
	if got := contracts[0].EffectiveDate; got != "2026-07-03" {
		t.Errorf("effective_date 应归一为 2026-07-03，实际 %q", got)
	}
	if got := contracts[0].ExpiryDate; got != "2027-07-02" {
		t.Errorf("expiry_date 应归一为 2027-07-02，实际 %q", got)
	}
	if len(issues) != 0 {
		t.Errorf("内置日期字段归一成功时不应产生问题: %+v", issues)
	}
	if len(patches) != 0 {
		t.Errorf("无必填配置时不应补洞: %+v", patches)
	}
}

func TestExtractGenericKeepsUnparseableDateUntouched(t *testing.T) {
	// 区间/自然语言原文不能被截成半截日期（这是通用校验与发票宽松版的差别）。
	utility := []types.UtilityBillExtractionItem{{
		BillPeriodStart: "2026-08-01至2026-08-31",
	}}
	records := NewGenericRecords(utility)
	issues, _ := ValidateGenericExtraction(records, "", nil)
	if got := utility[0].BillPeriodStart; got != "2026-08-01至2026-08-31" {
		t.Errorf("区间值不应被改写，实际 %q", got)
	}
	if len(issues) != 0 {
		t.Errorf("内置日期字段无法归一时应静默（不制造噪音）: %+v", issues)
	}
}

func TestExtractGenericWarnsWhenConfiguredDateUnparseable(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "sign_date", Type: "date", Enabled: true},
	}}
	contracts := []ContractExtractionItem{{SignDate: "本月内签署"}}
	issues, _ := ValidateGenericExtraction(NewGenericRecords(contracts), "", cfg)
	if len(issues) != 1 || issues[0].Field != "sign_date" || issues[0].Kind != "format" {
		t.Fatalf("用户声明的 date 字段无法归一应记 format 告警，实际 %+v", issues)
	}
	if issues[0].Severity != issueSeverityWarn {
		t.Errorf("格式问题应为 warn，实际 %s", issues[0].Severity)
	}
}

func TestExtractGenericRequiredFieldPatchedByConfiguredRegex(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "contract_name", Enabled: true, Required: true,
			Regex: `合同名称[:：]\s*([^\s]+)`},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-2026-001"}}
	text := "合同名称： 设备采购合同\n合同编号：HT-2026-001\n"

	issues, patches := ValidateGenericExtraction(NewGenericRecords(contracts), text, cfg)

	if got := contracts[0].ContractName; got != "设备采购合同" {
		t.Fatalf("必填字段应由配置正则补洞，实际 %q", got)
	}
	if len(patches) != 1 || patches[0].Field != "contract_name" || patches[0].Source != regexPatchSourceConfig {
		t.Fatalf("补洞记录异常: %+v", patches)
	}
	for _, it := range issues {
		if it.Field == "contract_name" && it.Severity == issueSeverityError {
			t.Errorf("已补洞的必填字段不应再报 error: %+v", it)
		}
	}
	if names := PatchedFieldNames(patches); len(names) != 1 || names[0] != "contract_name" {
		t.Errorf("PatchedFieldNames = %v", names)
	}
}

func TestExtractGenericRequiredMissingProducesErrorIssue(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "reg_name", Enabled: true, Required: true, Regex: `制度名称[:：]\s*(\S+)`},
	}}
	regs := []types.RegulationExtractionItem{{RegNo: "AD-032"}}
	issues, patches := ValidateGenericExtraction(NewGenericRecords(regs), "本文只有正文，没有标签", cfg)

	if len(patches) != 0 {
		t.Errorf("正则不命中时不应补洞: %+v", patches)
	}
	if len(issues) != 1 || issues[0].Field != "reg_name" || issues[0].Severity != issueSeverityError {
		t.Fatalf("必填缺失应记 1 条 error，实际 %+v", issues)
	}
}

func TestExtractGenericSkipsRequiredForUnknownField(t *testing.T) {
	// 强类型结果装不下用户自造的 key（如中文名"车牌号"），绝不能因此误报缺失。
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "车牌号", Enabled: true, Required: true},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-1"}}
	issues, _ := ValidateGenericExtraction(NewGenericRecords(contracts), "渝A12345", cfg)
	if len(issues) != 0 {
		t.Errorf("结构体不存在的字段不应报缺失: %+v", issues)
	}
}

func TestExtractGenericSkipsRegexPatchForMultipleRecords(t *testing.T) {
	// 安全边界：两条记录都缺同一字段时，匹配值到底属于谁无从判断 → 放弃补洞。
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "contract_name", Enabled: true, Required: true, Regex: `合同名称[:：]\s*(\S+)`},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-1"}, {ContractNo: "HT-2"}}
	issues, patches := ValidateGenericExtraction(
		NewGenericRecords(contracts), "合同名称：设备采购合同", cfg)

	if len(patches) != 0 {
		t.Fatalf("多记录时不得正则补洞: %+v", patches)
	}
	for i, ct := range contracts {
		if ct.ContractName != "" {
			t.Errorf("第 %d 条不应被回填: %q", i+1, ct.ContractName)
		}
	}
	if len(issues) != 2 {
		t.Errorf("两条记录都应各报 1 条必填 error，实际 %d: %+v", len(issues), issues)
	}
	for _, it := range issues {
		if it.Severity != issueSeverityError || it.Kind != "missing" {
			t.Errorf("必填缺失问题异常: %+v", it)
		}
	}
}

func TestExtractGenericDoesNotOverwriteExistingValue(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "contract_name", Enabled: true, Required: true, Regex: `合同名称[:：]\s*(\S+)`},
	}}
	contracts := []ContractExtractionItem{{ContractName: "模型给出的名称"}}
	_, patches := ValidateGenericExtraction(NewGenericRecords(contracts), "合同名称：别的名字", cfg)
	if len(patches) != 0 {
		t.Errorf("已有值不得覆盖: %+v", patches)
	}
	if contracts[0].ContractName != "模型给出的名称" {
		t.Errorf("值被改写: %q", contracts[0].ContractName)
	}
}

func TestExtractGenericPatchedDateIsNormalized(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "sign_date", Type: "date", Enabled: true, Required: true,
			Regex: `签订日期[:：]\s*(\d{4}年\d{1,2}月\d{1,2}日)`},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-1"}}
	_, patches := ValidateGenericExtraction(NewGenericRecords(contracts), "签订日期：2026年7月2日", cfg)
	if got := contracts[0].SignDate; got != "2026-07-02" {
		t.Fatalf("补洞进来的日期也必须归一化，实际 %q", got)
	}
	if len(patches) != 1 {
		t.Errorf("补洞记录异常: %+v", patches)
	}
}

// ---- 发票前置规则路径的质量收尾 ----

func TestExtractInvoiceRulePathNormalizesDateAndReportsIssues(t *testing.T) {
	// 规则提取出的结果此前完全没有质量元数据：日期保持票面原样、issues 恒为空。
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo:   "26507904700100036197",
			InvoiceDate: "2026年07月02日",
			TotalAmount: f64(101.72),
		}},
	}
	issues := FinalizeInvoiceRuleResult(context.Background(), res, "", nil)

	if got := res.Invoices[0].InvoiceDate; got != "2026-07-02" {
		t.Fatalf("规则路径日期应归一为 2026-07-02，实际 %q", got)
	}
	if len(res.ExtractIssues) != len(issues) {
		t.Errorf("issues 应写回结果: %d vs %d", len(res.ExtractIssues), len(issues))
	}
	for _, it := range issues {
		if it.Severity == issueSeverityError {
			t.Errorf("号码/金额齐全时不应有 error 级问题: %+v", it)
		}
	}
}

func TestExtractInvoiceRulePathPatchesMissingInvoiceNo(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{TotalAmount: f64(88.5)}},
	}
	text := "重庆增值税电子普通发票\n发票号码: 26507904700100036197\n"
	issues := FinalizeInvoiceRuleResult(context.Background(), res, text, nil)

	if got := res.Invoices[0].InvoiceNo; got != "26507904700100036197" {
		t.Fatalf("规则路径同样应做正则补洞，实际 %q", got)
	}
	if len(res.PatchedByRegex) == 0 {
		t.Error("应记录补洞字段供前端展示来源")
	}
	for _, it := range issues {
		if it.Field == "invoice_no" && it.Severity == issueSeverityError {
			t.Errorf("已补洞的发票号不应再报 error: %+v", it)
		}
	}
}

func TestNormalizeDateValue(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"2026-09-02", "2026-09-02", true},
		{"2026/9/2", "2026-09-02", true},
		{"2026.09.02", "2026-09-02", true},
		{"2026年9月2日", "2026-09-02", true},
		{"20260902", "2026-09-02", true},
		{"2026-08-01至2026-08-31", "", false},
		{"自发布之日起生效", "", false},
		{"2026-13-02", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeDateValue(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("NormalizeDateValue(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
