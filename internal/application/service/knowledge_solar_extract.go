package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// solarBillExtractMaxContentRunes bounds the document text sent to the
// extraction model. A single State Grid solar bill (few pages) must fit in
// one batch so all gateway rows land in the same record.
const solarBillExtractMaxContentRunes = 60000

// SolarBillExtractionResult is the strict-JSON shape the model must return.
type SolarBillExtractionResult struct {
	Kind         string                          `json:"kind"` // "solar_bill" | "not_solar_bill"
	Records      []types.SolarBillExtractionItem `json:"records"`
	ExtractError string                          `json:"extract_error"`
}

// solarBillExtractionSystemPrompt instructs the model to extract all fields
// from a State Grid photovoltaic (solar) generation bill.
const solarBillExtractionSystemPrompt = `你是一个专业的光伏发电账单信息提取助手。你的任务是从国网光伏发电账单（电费账单-光伏）文本中识别并提取账单关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段；无法识别时返回空字符串或 0。
2. 如果文档内容与光伏发电账单无关（如普通电费账单、发票、合同、制度、奖惩通报、清单等），kind 设为 "not_solar_bill"，records 返回空数组。
3. 一份账单只输出一条记录，所有关口（上网关口、发电关口）明细合并进该记录 gateways 数组，按账单出现顺序排列。
4. 基础信息：bill_period_start 为账单周期开始日期（YYYY-MM-DD）；bill_period_end 为账单周期结束日期；account_no 为发电户号；account_name 为户名；supply_unit 为服务单位；address 为发电地址；taxpayer_type 为纳税人类型（如"一般纳税人"）；consumption_mode 为消纳方式（如"自发自用余电上网"）；voltage_level 为并网电压等级（如"交流380V"）；generation_mode 为发电方式（如"光伏发电"）。
5. 概览：generation_kwh 为发电量（千瓦时）；grid_kwh 为上网电量（千瓦时）；settlement_amount 为结算金额（元）；mom_change 为本期上网电量较上期环比（如"+99.13%"）；cumulative_kwh 为年累计上网电量（千瓦时）；tax_rate 为税率（如"13%"）；tax_amount 为税额（元）。
6. gateways 关口明细：账单中每个"本期电量明细"块对应一个关口。gateway_type 为该关口类型（账单中含"上网关口"字样则为"上网关口"，含"发电关口"字样则为"发电关口"）；meter_no 为电能表编号（括号内 5030001221300052554525 形式）；project_name 为关口所属项目名称（如"1.4MW屋顶分布式光伏发电项目"，无则空）。
6.1 readings 电量明细：该关口"本期电量明细"表中每一行（示数类型通常为"反向有功（总）"）提取为一条：meter_type 为示数类型（原文）；prev 为上期示数；curr 为本期示数；multiplier 为倍率；reading_kwh 为抄见电量；bill_kwh 为计费电量。数值无法识别时返回 0。
6.2 fees 电费明细：该关口"本期电费明细"表中每一行提取为一条：category 为类别（如"分布式上网电费""分布式电源发电补助"）；qty 为电量（千瓦时）；rate 为电价（元/千瓦时，无则 0）；fee 为电费（元，= qty×rate，由你计算）。0 金额行也要输出。
7. remark 为备注（如账单备注、补贴说明）。
8. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"solar_bill","records":[{"bill_period_start":"","bill_period_end":"","account_no":"","account_name":"","supply_unit":"","address":"","taxpayer_type":"","consumption_mode":"","voltage_level":"","generation_mode":"","generation_kwh":0,"grid_kwh":0,"settlement_amount":0,"mom_change":"","cumulative_kwh":0,"tax_rate":"","tax_amount":0,"gateways":[{"gateway_type":"","meter_no":"","project_name":"","readings":[{"meter_type":"","prev":0,"curr":0,"multiplier":0,"reading_kwh":0,"bill_kwh":0}],"fees":[{"category":"","qty":0,"rate":0,"fee":0}]}],"remark":""}]}

示例：
{"kind":"solar_bill","records":[{"bill_period_start":"2026-08-01","bill_period_end":"2026-08-31","account_no":"5000303216586","account_name":"重庆星达铜业有限公司","supply_unit":"璧城供电服务中心","address":"重庆市璧山区青杠街道莲花社区居民委员会106号","taxpayer_type":"一般纳税人","consumption_mode":"自发自用余电上网","voltage_level":"交流380V","generation_mode":"光伏发电","generation_kwh":194489,"grid_kwh":27480,"settlement_amount":8409.37,"mom_change":"+99.13%","cumulative_kwh":97440,"tax_rate":"13%","tax_amount":967.45,"gateways":[{"gateway_type":"上网关口","meter_no":"5030001221300052554525","project_name":"1.4MW屋顶分布式光伏发电项目","readings":[{"meter_type":"反向有功（总）","prev":23.35,"curr":32.51,"multiplier":3000,"reading_kwh":27480,"bill_kwh":27480}],"fees":[{"category":"分布式上网电费","qty":27480,"rate":0.306018,"fee":8409.37}]},{"gateway_type":"发电关口","meter_no":"5030001221300051525236","project_name":"","readings":[{"meter_type":"反向有功（总）","prev":2094.96,"curr":2501.55,"multiplier":100,"reading_kwh":40659,"bill_kwh":0}],"fees":[{"category":"分布式电源发电补助","qty":40659,"rate":0,"fee":0}]}],"remark":"1.本期账单电费含税金额:8409.37,税率为:13%,税额为:967.45。"}]}`

// solarBillExtractionOutputSchema 是传给模型强制 json_object 模式的输出骨架。
const solarBillExtractionOutputSchema = `{"type":"object","properties":{"kind":{"type":"string"},"records":{"type":"array","items":{"type":"object","properties":{"bill_period_start":{"type":"string"},"bill_period_end":{"type":"string"},"account_no":{"type":"string"},"account_name":{"type":"string"},"supply_unit":{"type":"string"},"address":{"type":"string"},"taxpayer_type":{"type":"string"},"consumption_mode":{"type":"string"},"voltage_level":{"type":"string"},"generation_mode":{"type":"string"},"generation_kwh":{"type":"number"},"grid_kwh":{"type":"number"},"settlement_amount":{"type":"number"},"mom_change":{"type":"string"},"cumulative_kwh":{"type":"number"},"tax_rate":{"type":"string"},"tax_amount":{"type":"number"},"gateways":{"type":"array","items":{"type":"object","properties":{"gateway_type":{"type":"string"},"meter_no":{"type":"string"},"project_name":{"type":"string"},"readings":{"type":"array","items":{"type":"object","properties":{"meter_type":{"type":"string"},"prev":{"type":"number"},"curr":{"type":"number"},"multiplier":{"type":"number"},"reading_kwh":{"type":"number"},"bill_kwh":{"type":"number"}},"required":["meter_type","prev","curr","multiplier","reading_kwh","bill_kwh"]}},"fees":{"type":"array","items":{"type":"object","properties":{"category":{"type":"string"},"qty":{"type":"number"},"rate":{"type":"number"},"fee":{"type":"number"}},"required":["category","qty","rate","fee"]}}},"required":["gateway_type","meter_no","project_name","readings","fees"]}},"remark":{"type":"string"}},"required":["bill_period_start","bill_period_end","account_no","account_name","supply_unit","address","taxpayer_type","consumption_mode","voltage_level","generation_mode","generation_kwh","grid_kwh","settlement_amount","mom_change","cumulative_kwh","tax_rate","tax_amount","gateways","remark"]}},"required":["kind","records"]}}`

// BuildSolarBillExtractionBatches assembles the document text into batches.
func BuildSolarBillExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = AwardPunishExtractionBatchSize
	}
	usable := make([]*types.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.ChunkType != types.ChunkTypeText &&
			chunk.ChunkType != types.ChunkTypeImageOCR &&
			chunk.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		if text := strings.TrimSpace(chunk.Content); text != "" {
			usable = append(usable, chunk)
		}
	}
	header := make([]string, 0, 2)
	if name = strings.TrimSpace(name); name != "" {
		header = append(header, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		header = append(header, "Existing summary: "+summary)
	}
	joined := strings.Join(header, "\n")
	if joined != "" {
		joined += "\n\n"
	}
	content := joined + strings.Join(func() []string {
		out := make([]string, 0, len(usable))
		for _, c := range usable {
			out = append(out, c.Content)
		}
		return out
	}(), "\n\n")
	parts := splitTextBatches(content, solarBillExtractMaxContentRunes+1, solarBillExtractMaxContentRunes)
	return parts
}

// SolarBillBatchesHaveText reports whether any extraction batch carries usable text.
func SolarBillBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// tryParseSolarBillJSON 尝试把模型原始输出解析为提取结果；成功返回 true。
func tryParseSolarBillJSON(raw string, parsed *SolarBillExtractionResult) bool {
	cleaned := strings.NewReplacer("```json", "", "```", "", "`", "'").Replace(raw)
	if err := common.ParseLLMJsonResponse(cleaned, parsed); err == nil {
		return true
	}
	return false
}

// ExtractSolarBillsFromContent calls the configured chat model and parses
// its strict-JSON reply into a SolarBillExtractionResult.
// 与电费提取一致：智谱等供应商对长输入+大输出偶发返回空 content，
// 采用"流式优先、多次重试"策略（无 Format 流式 → 无 Format 非流式 → json_object 非流式 → 无 Format 流式），
// 且不传 max_tokens（智谱 glm-5.3-flash 显式传 max_tokens 时对超长输出返回空）。
func ExtractSolarBillsFromContent(ctx context.Context, model chat.Chat, content string) (*SolarBillExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &SolarBillExtractionResult{Kind: "not_solar_bill"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	llmCtx := types.WithLLMCallMetadata(ctx, "solar_bill_extract", "")
	messages := []chat.Message{
		{Role: "system", Content: solarBillExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}
	mkOpts := func() *chat.ChatOptions {
		return &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	}
	var lastErr error
	var lastRaw string
	attempts := []struct {
		label string
		run   func() (string, error)
	}{
		{"stream-free", func() (string, error) {
			return chatStreamCollect(llmCtx, model, messages, mkOpts())
		}},
		{"nonstream-free", func() (string, error) {
			resp, err := model.Chat(llmCtx, messages, mkOpts())
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		}},
		{"nonstream-jsonobject", func() (string, error) {
			opts := mkOpts()
			opts.Format = json.RawMessage(solarBillExtractionOutputSchema)
			resp, err := model.Chat(llmCtx, messages, opts)
			if err != nil {
				return "", err
			}
			return resp.Content, nil
		}},
		{"stream-free-2", func() (string, error) {
			return chatStreamCollect(llmCtx, model, messages, mkOpts())
		}},
	}
	for i, a := range attempts {
		raw, err := a.run()
		if err != nil {
			lastErr = err
			logger.Warnf(ctx, "solar bill extraction attempt %d (%s) error: %v", i+1, a.label, err)
			continue
		}
		var parsed SolarBillExtractionResult
		if tryParseSolarBillJSON(raw, &parsed) {
			return &parsed, nil
		}
		lastRaw = raw
		lastErr = fmt.Errorf("unparseable response (len=%d)", len(raw))
		logger.Warnf(ctx, "solar bill extraction attempt %d (%s) unparseable (len=%d)", i+1, a.label, len(raw))
	}
	logger.Warnf(ctx, "solar bill extraction raw response (first 20000 chars): %s", truncateForLog(lastRaw, 20000))
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable response")
	}
	return nil, fmt.Errorf("parse solar bill extraction response: %w", lastErr)
}

// NormalizeSolarBillExtractionResult normalizes one record per bill and
// recalculates each gateway fee as qty × rate.
func NormalizeSolarBillExtractionResult(extracted *SolarBillExtractionResult) {
	if extracted == nil {
		return
	}
	if len(extracted.Records) > 1 {
		extracted.Records = []types.SolarBillExtractionItem{extracted.Records[0]}
	}
	for i := range extracted.Records {
		r := &extracted.Records[i]
		// 结算金额兜底：无值时按上网关口电费合计
		if r.SettlementAmount == 0 {
			for _, g := range r.Gateways {
				if g.GatewayType == "上网关口" {
					var sum float64
					for _, f := range g.Fees {
						sum += f.Fee
					}
					r.SettlementAmount = math.Round(sum*100) / 100
				}
			}
		}
		// 清理空关口
		clean := make([]types.SolarGateway, 0, len(r.Gateways))
		for _, g := range r.Gateways {
			gt := strings.TrimSpace(g.GatewayType)
			if gt == "" && strings.TrimSpace(g.MeterNo) == "" && len(g.Readings) == 0 && len(g.Fees) == 0 {
				continue
			}
			g.GatewayType = gt
			// 电费 = 电量 × 电价（统一重算）
			for j := range g.Fees {
				f := &g.Fees[j]
				if f.Rate != 0 && f.Qty != 0 {
					f.Fee = math.Round(f.Qty*f.Rate*100) / 100
				}
			}
			clean = append(clean, g)
		}
		r.Gateways = clean
	}
}
