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
		ExpireDate:    "20270702",
	}}
	records := NewGenericRecords(contracts)
	issues, patches := ValidateGenericExtraction(records, "", nil)

	if got := contracts[0].SignDate; got != "2026-07-02" {
		t.Errorf("sign_date 应归一为 2026-07-02，实际 %q", got)
	}
	if got := contracts[0].EffectiveDate; got != "2026-07-03" {
		t.Errorf("effective_date 应归一为 2026-07-03，实际 %q", got)
	}
	if got := contracts[0].ExpireDate; got != "2027-07-02" {
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

// 行为变更（2026-10-22）：用户自定义字段不再被整体跳过。
//
// 旧行为「强类型结果里没有这个 json 字段 → 跳过避免误报」正是真机事故的根因：
// 用户在「字段定义」里加了「保证金」并配了规则，模型没提到时系统既不报错、
// 也不正则补洞，列表与抽屉里那个字段永远为空，用户完全看不出问题在哪。
//
// 新契约：记录带 Fields 动态桶（合同/制度/公告/发票提取项都有）时，自定义字段
// 是**合法落脚点** —— 必填缺失要报出来，配了正则要能补进去。
func TestExtractGenericSkipsRequiredForUnknownField(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "车牌号", Enabled: true, Required: true},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-1"}}
	issues, _ := ValidateGenericExtraction(NewGenericRecords(contracts), "渝A12345", cfg)
	if len(issues) != 1 {
		t.Fatalf("自定义必填字段缺失应报 1 条 error，实际 %d: %+v", len(issues), issues)
	}
	if issues[0].Field != "车牌号" || issues[0].Kind != "missing" {
		t.Errorf("必填缺失问题异常: %+v", issues[0])
	}
}

// TestExtractGenericCustomFieldRegexPatchLandsInBucket 锁定补洞落点：
// 用户给自定义字段配了正则时，值必须写进 Fields 动态桶（真机事故的直接修复）。
func TestExtractGenericCustomFieldRegexPatchLandsInBucket(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "车牌号", Enabled: true, Required: true, Regex: `车牌号[:：]?\s*(\S+)`},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-1"}}
	_, patches := ValidateGenericExtraction(NewGenericRecords(contracts), "车牌号：渝A12345", cfg)
	if len(patches) != 1 {
		t.Fatalf("应补 1 处洞，实际 %d: %+v", len(patches), patches)
	}
	if contracts[0].Fields["车牌号"] != "渝A12345" {
		t.Errorf("补洞值必须落进 Fields 动态桶，实际 %+v", contracts[0].Fields)
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

// ---- Phase D：前端配置用的是 camelCase（invoiceNo / signDate），后端契约是 snake_case。
// 两边必须靠 canonical 归一才能对上，否则用户在界面上打开"必填/正则"后完全不生效。

func TestExtractGenericMatchesCamelCaseConfigKeys(t *testing.T) {
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "contractName", Enabled: true, Required: true, Type: "string",
			Regex: `合同名称[:：]\s*([^\s]+)`},
		{Name: "signDate", Enabled: true, Type: "date"},
	}}
	contracts := []ContractExtractionItem{{ContractNo: "HT-2026-009", SignDate: "2026年7月2日"}}
	text := "合同名称： 设备采购合同\n"

	issues, patches := ValidateGenericExtraction(NewGenericRecords(contracts), text, cfg)

	if got := contracts[0].ContractName; got != "设备采购合同" {
		t.Fatalf("camelCase 配置键应能补洞到 snake_case 字段，实际 %q", got)
	}
	if got := contracts[0].SignDate; got != "2026-07-02" {
		t.Errorf("camelCase 声明 Type=date 也应归一化，实际 %q", got)
	}
	if len(patches) != 1 || patches[0].Field != "contractName" {
		t.Fatalf("补洞明细应保留用户配置的键名: %+v", patches)
	}
	for _, it := range issues {
		if it.Severity == issueSeverityError {
			t.Errorf("已补洞的必填字段不应再报 error: %+v", it)
		}
	}
}

func TestExtractGenericFlatMapRecordValidatesAndPatches(t *testing.T) {
	// 提取测试接口走的是"扁平 map"沙箱结果，不是强类型结构体；同一套校验必须能用。
	fields := map[string]any{
		"invoiceNo":   "",
		"invoiceDate": "2026年7月2日",
		"amount":      0,
	}
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "invoiceNo", Enabled: true, Required: true, Regex: `发票号码[:：]?\s*(\d{20})`},
		{Name: "buyerName", Enabled: true, Required: true},
		{Name: "invoiceDate", Enabled: true, Type: "date"},
	}}
	text := "发票号码：26507000000266963455\n"

	records := []GenericExtractRecord{NewGenericRecordFromMap(0, fields)}
	issues, patches := ValidateGenericExtraction(records, text, cfg)

	if got := fields["invoiceNo"]; got != "26507000000266963455" {
		t.Fatalf("map 记录应被补洞回写，实际 %v", got)
	}
	if got := fields["invoiceDate"]; got != "2026-07-02" {
		t.Errorf("map 记录日期应归一化，实际 %v", got)
	}
	if len(patches) != 1 || patches[0].Field != "invoiceNo" {
		t.Fatalf("补洞明细异常: %+v", patches)
	}
	var missing []string
	for _, it := range issues {
		if it.Severity == issueSeverityError {
			missing = append(missing, it.Field)
		}
	}
	if len(missing) != 1 || missing[0] != "buyerName" {
		t.Fatalf("只应为 buyerName 报 error，实际 %v (issues=%+v)", missing, issues)
	}
}

// ---- Phase D 收尾：模型整键漏提取时，map 视图的必填字段也应能靠正则**建键**回填，
// 与强类型结构体"字段一定存在、只是零值"同口径；正则也补不到才报 missing。

func TestExtractGenericFlatMapRecordRegexCreatesAbsentKey(t *testing.T) {
	fields := map[string]any{
		"amount": 0,
	}
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "invoiceNo", Enabled: true, Required: true, Regex: `发票号码[:：]?\s*(\d{20})`},
	}}
	text := "发票号码：26507000000266963455\n"

	records := []GenericExtractRecord{NewGenericRecordFromMap(0, fields)}
	issues, patches := ValidateGenericExtraction(records, text, cfg)

	if got := fields["invoiceNo"]; got != "26507000000266963455" {
		t.Fatalf("缺键的必填字段应被正则建键回填，实际 %v", got)
	}
	if len(patches) != 1 || patches[0].Field != "invoiceNo" {
		t.Fatalf("补洞明细异常: %+v", patches)
	}
	for _, it := range issues {
		if it.Severity == issueSeverityError {
			t.Errorf("补洞成功的字段不应再报 error: %+v", it)
		}
	}
}

// ---- Phase D：发票侧同样要认 camelCase 配置键（规则面板保存的就是 invoiceNo/buyerName）。

func TestExtractInvoiceConfigCamelCaseRegexPatchesMissingNo(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind:     "invoice",
		Invoices: []InvoiceExtractionItem{{TotalAmount: f64(88.5)}},
	}
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "invoiceNo", Enabled: true, Required: true, Regex: `发票号码[:：]\s*(\d{20})`},
	}}
	text := "重庆增值税电子普通发票\n发票号码: 26507904700100036197\n"

	issues := FinalizeInvoiceRuleResult(context.Background(), res, text, cfg)

	if got := res.Invoices[0].InvoiceNo; got != "26507900100036197" && got != "26507904700100036197" {
		t.Fatalf("camelCase 配置的正则应参与补洞，实际 %q", got)
	}
	if len(res.PatchedByRegex) == 0 {
		t.Fatalf("补洞来源应透出（patched_by_regex）")
	}
	for _, it := range issues {
		if it.Field == "invoiceNo" && it.Severity == issueSeverityError {
			t.Errorf("已补洞的必填字段不应再报 error: %+v", it)
		}
	}
}

func TestExtractInvoiceRequiredCamelCaseReportsError(t *testing.T) {
	res := &InvoiceExtractionResult{
		Kind: "invoice",
		Invoices: []InvoiceExtractionItem{{
			InvoiceNo:   "26507904700100036197",
			InvoiceDate: "2026-07-02",
			TotalAmount: f64(101.72),
		}},
	}
	cfg := &types.KbExtractConfig{Fields: []types.ExtractFieldConfig{
		{Name: "buyerName", Enabled: true, Required: true},
	}}
	issues := FinalizeInvoiceRuleResult(context.Background(), res, "", cfg)

	found := false
	for _, it := range issues {
		if it.Field == "buyerName" && it.Severity == issueSeverityError {
			found = true
		}
	}
	if !found {
		t.Fatalf("camelCase 必填缺失应产出 error 级 issue，实际 %+v", issues)
	}
}
