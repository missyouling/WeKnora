package service

import (
	"context"
	"strings"
	"testing"
)

// Phase C：六个业务模块统一收编到通用执行器（extract_executor.go）后的行为测试。
// 命名以 TestExtract 开头，确保被 Daily-Affairs CI 白名单正则覆盖。
//
// 这里锁死的是「不再有裸调」这件事本身：
//   - 首次返回空/垃圾内容时必须换策略重试，而不是直接失败；
//   - MaxTokens 恒为 0（显式传值会让部分供应商在长输出时返回空 content）；
//   - 思考类模型的 <think> 块与千分位/货币符号金额能被清洗层吃掉。

func TestExtractContractUsesExecutorRetry(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"抱歉，我无法处理该文档", `{"kind":"contract","contracts":[{"contract_no":"HT-2026-001","contract_name":"设备采购合同"}]}`},
	}
	res, err := ExtractContractsFromContent(context.Background(), m, "甲方：A 公司\n乙方：B 公司")
	if err != nil {
		t.Fatalf("第 3 次可解析时应成功: %v", err)
	}
	if len(res.Contracts) != 1 || res.Contracts[0].ContractNo != "HT-2026-001" {
		t.Fatalf("解析结果异常: %+v", res.Contracts)
	}
	if len(m.calls) != 3 {
		t.Errorf("应重试到第 3 次，实际 %d 次: %v", len(m.calls), m.calls)
	}
	for i, mt := range m.maxTokens {
		if mt != 0 {
			t.Errorf("第 %d 次调用 MaxTokens 必须为 0，实际 %d", i+1, mt)
		}
	}
}

func TestExtractRegulationUsesExecutorRetry(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{`<think>先看页眉编号</think>{"kind":"regulation","regulations":[{"reg_no":"AD-032","reg_name":"员工转岗管理制度"}]}`},
		nonStreamOut: []string{},
	}
	res, err := ExtractRegulationsFromContent(context.Background(), m, "编号：AD-032 员工转岗管理制度")
	if err != nil {
		t.Fatalf("应成功: %v", err)
	}
	if len(res.Regulations) != 1 || res.Regulations[0].RegNo != "AD-032" {
		t.Fatalf("思考块应被剥离后正常解析，实际 %+v", res.Regulations)
	}
	if len(m.calls) != 1 {
		t.Errorf("首次成功应只调用 1 次，实际 %d: %v", len(m.calls), m.calls)
	}
}

func TestExtractAwardPunishUsesExecutorRetry(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"not json", `{"kind":"award_punish","records":[{"ap_no":"星达行政字〔2026〕11号","ap_type":"决定"}]}`},
	}
	res, err := ExtractAwardPunishesFromContent(context.Background(), m, "关于对汪艳违规行为的处罚通报", nil)
	if err != nil {
		t.Fatalf("第 3 次可解析时应成功: %v", err)
	}
	if len(res.Records) != 1 || res.Records[0].ApNo == "" {
		t.Fatalf("解析结果异常: %+v", res.Records)
	}
	for i, mt := range m.maxTokens {
		if mt != 0 {
			t.Errorf("第 %d 次调用 MaxTokens 必须为 0，实际 %d", i+1, mt)
		}
	}
}

func TestExtractSolarBillUsesExecutorRetry(t *testing.T) {
	m := &fakeRetryChat{
		streamOut: []string{"", ""},
		nonStreamOut: []string{"抱歉", `{"kind":"solar_bill","records":[{"account_no":"5000303216586",
			"bill_period_start":"2026-08-01","bill_period_end":"2026-08-31","grid_kwh":27480,
			"gateways":[{"gateway_type":"上网关口","meter_no":"5030001221300052554525"}]}]}`},
	}
	res, err := ExtractSolarBillsFromContent(context.Background(), m, "国网光伏发电账单")
	if err != nil {
		t.Fatalf("第 3 次可解析时应成功: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("应解析出 1 条记录，实际 %+v", res.Records)
	}
	if got := res.Records[0].AccountNo; got != "5000303216586" {
		t.Errorf("account_no = %q", got)
	}
	if len(res.Records[0].Gateways) != 1 {
		t.Errorf("关口明细丢失: %+v", res.Records[0].Gateways)
	}
	if len(m.calls) != 3 {
		t.Errorf("应重试到第 3 次（含 schema 强约束），实际 %d: %v", len(m.calls), m.calls)
	}
	if m.formats[2] == "" {
		t.Error("第 3 次应带输出 schema")
	}
	for i, mt := range m.maxTokens {
		if mt != 0 {
			t.Errorf("第 %d 次调用 MaxTokens 必须为 0，实际 %d", i+1, mt)
		}
	}
}

// 电费账单的经典失败形态：模型把千分位电量与「¥」金额原样加引号输出，
// 标准 json.Unmarshal 直接失败——旧实现重试 4 次也救不回来（每次都是同样的垃圾）。
// 清洗层必须先转成裸数字，重试才有意义。
func TestExtractUtilityBillHandlesThinkAndThousandsSeparators(t *testing.T) {
	raw := `<think>核对费用明细行数</think>{"kind":"utility_bill","records":[{` +
		`"account_no":"5001654065328","total_kwh":"555,617","total_amount":"¥393,459.72",` +
		`"bill_period_start":"2026-08-01","bill_period_end":"2026-08-31",` +
		`"fee_items":[{"category":"(1)市场化购电费","name":"零售交易电费","qty":"547,287","fee":"19,585.21"}]}]}`
	m := &fakeRetryChat{
		streamOut:    []string{""},
		nonStreamOut: []string{"抱歉", raw},
	}
	res, err := ExtractUtilityBillsFromContent(context.Background(), m, "国网电费账单")
	if err != nil {
		t.Fatalf("含 think 块与千分位金额时应能解析: %v", err)
	}
	if len(res.Records) != 1 {
		t.Fatalf("应解析出 1 条记录，实际 %+v", res.Records)
	}
	r := res.Records[0]
	if r.TotalKwh != 555617 {
		t.Errorf("total_kwh 应转成 555617，实际 %v", r.TotalKwh)
	}
	if !floatEquals(r.TotalAmount, 393459.72) {
		t.Errorf("total_amount 应转成 393459.72，实际 %v", r.TotalAmount)
	}
	if len(r.FeeItems) != 1 || !floatEquals(r.FeeItems[0].Fee, 19585.21) {
		t.Errorf("费用明细金额异常: %+v", r.FeeItems)
	}
	if len(m.calls) != 3 {
		t.Errorf("应重试到第 3 次，实际 %d: %v", len(m.calls), m.calls)
	}
}

// 全部尝试都失败时必须返回错误（而不是把空结果当成功落库）。
func TestExtractUtilityBillFailsAfterAllAttempts(t *testing.T) {
	m := &fakeRetryChat{
		streamOut:    []string{"", ""},
		nonStreamOut: []string{"垃圾", "还是垃圾"},
	}
	_, err := ExtractUtilityBillsFromContent(context.Background(), m, "国网电费账单")
	if err == nil {
		t.Fatal("全部尝试失败时应返回错误")
	}
	if !strings.Contains(err.Error(), "utility bill") {
		t.Errorf("错误应带业务前缀便于定位，实际 %v", err)
	}
	if len(m.calls) != 4 {
		t.Errorf("应跑满 4 次尝试，实际 %d", len(m.calls))
	}
}

// 车队证照提取保留自己的策略顺序（json_object 优先），但同样必须走执行器：
// 第 1 次返回非 JSON 时应换流式再试，而不是直接失败。
func TestExtractFleetDocumentKeepsJsonObjectFirstThenRetries(t *testing.T) {
	raw := `<think>识别行驶证</think>{"kind":"fleet_vehicle_document","doc_type":"行驶证",` +
		`"vehicle_no":"渝A12345","fields":{"vin":"LSVAM4185E2100001"}}`
	m := &fakeRetryChat{
		nonStreamOut: []string{"抱歉，输出失败"},
		streamOut:    []string{raw},
	}
	res, err := ExtractFleetDocumentFromContent(context.Background(), m, "行驶证正本", "vehicle", "行驶证", nil, "")
	if err != nil {
		t.Fatalf("第 2 次（流式）可解析时应成功: %v", err)
	}
	if res.Kind != "fleet_vehicle_document" || res.VehicleNo != "渝A12345" {
		t.Fatalf("解析结果异常: %+v", res)
	}
	if len(m.calls) != 2 || m.calls[0] != "nonstream" || m.calls[1] != "stream" {
		t.Fatalf("策略顺序应保持 json_object → 流式，实际 %v", m.calls)
	}
	if !strings.Contains(m.formats[0], "json_object") {
		t.Errorf("第 1 次应带 json_object 约束，实际 Format=%q", m.formats[0])
	}
}
