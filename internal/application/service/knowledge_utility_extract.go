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

// utilityBillExtractMaxContentRunes bounds the document text sent to the
// extraction model. A single State Grid bill (few pages) must fit in one batch
// so all fee rows land in the same record.
const utilityBillExtractMaxContentRunes = 60000

// UtilityBillExtractionResult is the strict-JSON shape the model must return.
type UtilityBillExtractionResult struct {
	Kind         string                         `json:"kind"` // "utility_bill" | "not_utility_bill"
	Records      []types.UtilityBillExtractionItem `json:"records"`
	ExtractError string                         `json:"extract_error"`
}

// utilityBillExtractionSystemPrompt instructs the model to extract all four
// field groups from a State Grid electricity bill: overview, metering
// breakdown, capacity/demand, and full fee-item detail.
const utilityBillExtractionSystemPrompt = `你是一个专业的电费账单信息提取助手。你的任务是从国网电费账单文本中识别并提取账单关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段；无法识别时返回空字符串或 0。
2. 如果文档内容与电费账单无关（如发票、合同、制度、奖惩通报、清单等），kind 设为 "not_utility_bill"，records 返回空数组。
3. 一份账单只输出一条记录（含多个计量点/多块电表的也合并为一条记录，计量信息并入对应字段）。
4. 概况字段：bill_period_start 为账单周期开始日期（YYYY-MM-DD）；bill_period_end 为账单周期结束日期；account_no 为户号；account_name 为户名；address 为用电地址；usage_category 为用电类别；voltage_level 为电压等级；supply_unit 为供电服务单位；market_attr 为市场化属性；print_date 为账单打印日期；total_kwh 为本期电量（千瓦时）；total_amount 为本期电费（元）；prev_kwh 为上期电量（无则 0）；mom_change 为环比百分比（如"-15.18%"）；avg_price 为平均电价（元/千瓦时）；power_factor 为功率因数；due_date 为交费截止日期；industrial_amount 为工商业电费；residential_amount 为居民电费；pf_adjust_amount 为功率因数调整电费（负数保留负号）；grand_total 为合计电费。
5. 电量明细：deep_peak_kwh 为尖峰电量；peak_kwh 为峰电量；flat_kwh 为平电量；valley_kwh 为谷电量；reactive_kwh 为正向无功电量（千瓦时/千乏时）。
5.1 电量明细表 meter_readings：账单"电量明细"表格中每一行（示数类型：正向有功（总）、正向有功（尖峰）、正向有功（峰）、正向有功（平）、正向有功（谷）、正向无功（总）等）都要提取，逐行列出，共 6 行（如表中缺失某行则跳过该行）。每行字段：meter_type 为示数类型（原文）；prev 为上期示数；curr 为本期示数；multiplier 为倍率；reading_kwh 为抄见电量（千瓦时）；trans_loss 为变损（千瓦时）；line_loss 为线损（千瓦时）；adjust 为加减（千瓦时，正负照抄）；bill_kwh 为计费电量（千瓦时）。数值无法识别时返回 0。
6. 容需量：capacity 为容量（千伏安）；capacity_price 为容量电价；capacity_fee 为输配容量电费；demand 为需量值；pf_standard 为功率因数标准；adjust_ratio 为调整系数。
7. 费用明细 fee_items：账单"电费明细"表格（费用类别/费用组成/分时时段/计费电量/计费标准/电费）中每一行"费用组成"都必须提取为一条，逐行列出，不得合并、不得省略、不得只取部分、不得截断。注意：文本中"费用组成"行之间可能夹着用能分析等无关文字，也要把表格行按原顺序全部找出来；类别小计行（如"(4)系统运行费 31633.19"后面跟着"其中："子行的）不要单独输出，改输出其下每个子行（如"抽水蓄能容量电费 3666.82"）；0 金额行（如"辅助服务费用 0"）也要输出；返还/损益类行（如"零售交易发电侧收益返还电费""中长期超额申报费用返还（用户）""中长期偏差回收返还（用户）""零售损益分摊电费""上网环节线损代理采购损益"）即使金额为负也必须逐条输出，不得遗漏。该账单费用明细行数通常在 30~50 行，如果输出少于 15 条，说明有遗漏，请补全。category 为该行所属费用类别：尽量使用"(1)市场化购电费""(2)上网环节线损费用""(3)输配电量电费""(4)系统运行费""(5)目录电费""(6)政府性基金及附加""功率因数调整电费"；政府性基金及附加必须区分工商业与居民：若该行 qty 对应的计费电量与工商业分时电量（尖峰+峰+平+谷之和，本账单约 54.7 万千瓦时）一致则为"(6)政府性基金及附加-工商业"，若与居民目录电量（本账单 8330 千瓦时）一致则为"(6)政府性基金及附加-居民"；name 为该行费用组成名称；period 为分时时段（尖峰/峰/平/谷，无时段填""）；qty 为该行计费电量；rate 为该行计费标准/单价；fee 为该行电费金额（负数保留负号，返还/偏差/调整类金额照抄原值）；fee_amount 为该行电费在账单上的标称金额（账单原值，与 fee 相同即照抄；fee 与账单原值不同时以账单标称为准）。fee_items 按账单出现顺序输出。
8. remark 为备注（如账单备注、附加说明）。
9. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"utility_bill","records":[{"bill_period_start":"","bill_period_end":"","account_no":"","account_name":"","address":"","usage_category":"","voltage_level":"","supply_unit":"","market_attr":"","print_date":"","total_kwh":0,"total_amount":0,"prev_kwh":0,"mom_change":"","avg_price":0,"power_factor":0,"due_date":"","industrial_amount":0,"residential_amount":0,"pf_adjust_amount":0,"grand_total":0,"deep_peak_kwh":0,"peak_kwh":0,"flat_kwh":0,"valley_kwh":0,"reactive_kwh":0,"capacity":0,"capacity_price":0,"capacity_fee":0,"demand":0,"pf_standard":0,"adjust_ratio":0,"meter_readings":[{"meter_type":"","prev":0,"curr":0,"multiplier":0,"reading_kwh":0,"trans_loss":0,"line_loss":0,"adjust":0,"bill_kwh":0}],"fee_items":[{"category":"","name":"","period":"","qty":0,"rate":0,"fee":0,"fee_amount":0}],"remark":""}]}

示例（概况+费用明细多行）：
{"kind":"utility_bill","records":[{"bill_period_start":"2026-08-01","bill_period_end":"2026-08-31","account_no":"5001654065328","account_name":"重庆星达铜业有限公司","address":"重庆市璧山区青杠街道","usage_category":"大工业用电","voltage_level":"交流10kV","supply_unit":"璧城供电服务中心","market_attr":"市场化零售客户","print_date":"2026-09-04","total_kwh":555617,"total_amount":393459.72,"prev_kwh":0,"mom_change":"-15.18%","avg_price":0.70815,"power_factor":0.97,"due_date":"2026-09-09","industrial_amount":391579.91,"residential_amount":4414.9,"pf_adjust_amount":-2535.09,"grand_total":393459.72,"deep_peak_kwh":14177,"peak_kwh":92683,"flat_kwh":160816,"valley_kwh":279611,"reactive_kwh":140820,"capacity":2000,"capacity_price":22,"capacity_fee":44000,"demand":0,"pf_standard":0.9,"adjust_ratio":-0.0075,"meter_readings":[{"meter_type":"正向有功（总）","prev":2863740,"curr":3419357,"multiplier":1,"reading_kwh":555617,"trans_loss":0,"line_loss":0,"adjust":0,"bill_kwh":555617},{"meter_type":"正向有功（尖峰）","prev":2863740,"curr":2877917,"multiplier":1,"reading_kwh":14177,"trans_loss":0,"line_loss":0,"adjust":0,"bill_kwh":14177}],"fee_items":[{"category":"(1)市场化购电费","name":"零售交易电费","period":"尖峰","qty":14177,"rate":0.436,"fee":6181.17,"fee_amount":6181.17},{"category":"(1)市场化购电费","name":"零售交易电费","period":"峰","qty":92683,"rate":0.436,"fee":40409.79},{"category":"(1)市场化购电费","name":"零售交易电费","period":"平","qty":160816,"rate":0.436,"fee":70115.78},{"category":"(1)市场化购电费","name":"零售交易电费","period":"谷","qty":279611,"rate":0.436,"fee":121910.4},{"category":"(1)市场化购电费","name":"零售交易发电侧收益返还电费","period":"平","qty":0,"rate":0,"fee":-4940.37},{"category":"(1)市场化购电费","name":"中长期超额申报费用返还（用户）","period":"平","qty":0,"rate":0,"fee":-524.7},{"category":"(1)市场化购电费","name":"中长期偏差回收返还（用户）","period":"平","qty":0,"rate":0,"fee":-39397.7},{"category":"(1)市场化购电费","name":"零售损益分摊电费","period":"平","qty":547287,"rate":0.035786,"fee":19585.21},{"category":"(2)上网环节线损费用","name":"上网环节线损费用","period":"平","qty":547287,"rate":0.017274,"fee":9453.84},{"category":"(3)输配电量电费","name":"零售输配电费","period":"尖峰","qty":14177,"rate":0.29088,"fee":4123.81},{"category":"(3)输配电量电费","name":"零售输配电费","period":"峰","qty":92683,"rate":0.2424,"fee":22466.36},{"category":"(3)输配电量电费","name":"零售输配电费","period":"平","qty":160816,"rate":0.1515,"fee":24363.62},{"category":"(3)输配电量电费","name":"零售输配电费","period":"谷","qty":279611,"rate":0.05757,"fee":16097.21},{"category":"(4)系统运行费","name":"燃气机组容量电费","period":"平","qty":547287,"rate":0.015522,"fee":8494.99},{"category":"(4)系统运行费","name":"新能源机制差价分摊电费","period":"平","qty":547287,"rate":0.006505,"fee":3560.1},{"category":"(4)系统运行费","name":"电价交叉补贴新增损益","period":"平","qty":547287,"rate":0.005454,"fee":2984.9},{"category":"(4)系统运行费","name":"上网环节线损代理采购损益","period":"平","qty":547287,"rate":-0.001436,"fee":-785.9},{"category":"(4)系统运行费","name":"其他系统运行费用","period":"平","qty":547287,"rate":0.00375,"fee":2052.33},{"category":"(4)系统运行费","name":"抽水蓄能容量电费","period":"平","qty":547287,"rate":0.0067,"fee":3666.82},{"category":"(4)系统运行费","name":"辅助服务费用","period":"平","qty":547287,"rate":0,"fee":0},{"category":"(4)系统运行费","name":"煤电容量电费","period":"平","qty":547287,"rate":0.021305,"fee":11659.95},{"category":"(5)目录电费","name":"基础电费","period":"平","qty":8330,"rate":0.500306,"fee":4167.55},{"category":"(6)政府性基金及附加-居民","name":"农网还贷","period":"平","qty":8330,"rate":0.02,"fee":166.6},{"category":"(6)政府性基金及附加-居民","name":"可再生能源附加","period":"平","qty":8330,"rate":0.001,"fee":8.33},{"category":"(6)政府性基金及附加-居民","name":"国家重大水利工程建设基金","period":"平","qty":8330,"rate":0.001969,"fee":16.4},{"category":"(6)政府性基金及附加-居民","name":"库区移民基金","period":"平","qty":8330,"rate":0.006225,"fee":51.85},{"category":"(6)政府性基金及附加-居民","name":"小型水库移民后期扶持资金（地方）","period":"平","qty":8330,"rate":0.0005,"fee":4.17},{"category":"功率因数调整电费","name":"功率因数调整电费","period":"","qty":555617,"rate":0,"fee":-2535.09}],"remark":""}]}`

// utilityBillExtractionOutputSchema 是传给模型强制 json_object 模式的输出骨架。
const utilityBillExtractionOutputSchema = `{"type":"object","properties":{"kind":{"type":"string"},"records":{"type":"array","items":{"type":"object","properties":{"bill_period_start":{"type":"string"},"bill_period_end":{"type":"string"},"account_no":{"type":"string"},"account_name":{"type":"string"},"address":{"type":"string"},"usage_category":{"type":"string"},"voltage_level":{"type":"string"},"supply_unit":{"type":"string"},"market_attr":{"type":"string"},"print_date":{"type":"string"},"total_kwh":{"type":"number"},"total_amount":{"type":"number"},"prev_kwh":{"type":"number"},"mom_change":{"type":"string"},"avg_price":{"type":"number"},"power_factor":{"type":"number"},"due_date":{"type":"string"},"industrial_amount":{"type":"number"},"residential_amount":{"type":"number"},"pf_adjust_amount":{"type":"number"},"grand_total":{"type":"number"},"deep_peak_kwh":{"type":"number"},"peak_kwh":{"type":"number"},"flat_kwh":{"type":"number"},"valley_kwh":{"type":"number"},"reactive_kwh":{"type":"number"},"meter_readings":{"type":"array","items":{"type":"object","properties":{"meter_type":{"type":"string"},"prev":{"type":"number"},"curr":{"type":"number"},"multiplier":{"type":"number"},"reading_kwh":{"type":"number"},"trans_loss":{"type":"number"},"line_loss":{"type":"number"},"adjust":{"type":"number"},"bill_kwh":{"type":"number"}},"required":["meter_type","prev","curr","multiplier","reading_kwh","trans_loss","line_loss","adjust","bill_kwh"]}},"capacity":{"type":"number"},"capacity_price":{"type":"number"},"capacity_fee":{"type":"number"},"demand":{"type":"number"},"pf_standard":{"type":"number"},"adjust_ratio":{"type":"number"},"fee_items":{"type":"array","items":{"type":"object","properties":{"category":{"type":"string"},"name":{"type":"string"},"period":{"type":"string"},"qty":{"type":"number"},"rate":{"type":"number"},"fee":{"type":"number"},"fee_amount":{"type":"number"}},"required":["category","name","period","qty","rate","fee","fee_amount"]}},"remark":{"type":"string"}},"required":["bill_period_start","bill_period_end","account_no","account_name","address","usage_category","voltage_level","supply_unit","market_attr","print_date","total_kwh","total_amount","prev_kwh","mom_change","avg_price","power_factor","due_date","industrial_amount","residential_amount","pf_adjust_amount","grand_total","deep_peak_kwh","peak_kwh","flat_kwh","valley_kwh","reactive_kwh","meter_readings","capacity","capacity_price","capacity_fee","demand","pf_standard","adjust_ratio","fee_items","remark"]}},"required":["kind","records"]}}`

// BuildUtilityBillExtractionBatches assembles the document text into
// independent batches so long bills extract reliably.
func BuildUtilityBillExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
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
	// 电费账单必须整份作为单个批次交给模型，确保四组字段与全部费用明细
	// 落在同一条记录内；仅当内容超出上下文上限时才拆分。
	parts := splitTextBatches(content, utilityBillExtractMaxContentRunes+1, utilityBillExtractMaxContentRunes)
	return parts
}

// splitTextBatches splits joined text into batchSize-sized batches, each
// capped at maxRunes.
func splitTextBatches(content string, batchSize, maxRunes int) []string {
	if batchSize <= 0 {
		batchSize = 1
	}
	segs := strings.Split(content, "\n\n")
	var batches []string
	var cur []string
	curLen := 0
	flush := func() {
		if len(cur) == 0 {
			return
		}
		batches = append(batches, strings.Join(cur, "\n\n"))
		cur = nil
		curLen = 0
	}
	for _, s := range segs {
		if len(s) > maxRunes {
			// 超长片段单独成批并截断
			flush()
			runes := []rune(s)
			if len(runes) > maxRunes {
				runes = runes[:maxRunes]
			}
			batches = append(batches, string(runes))
			continue
		}
		if len(cur) >= batchSize || curLen+len(s) > maxRunes {
			flush()
		}
		cur = append(cur, s)
		curLen += len(s)
	}
	flush()
	if len(batches) == 0 && strings.TrimSpace(content) != "" {
		batches = []string{content}
	}
	return batches
}

// ExtractUtilityBillsFromContent calls the configured chat model and parses
// its strict-JSON reply into a UtilityBillExtractionResult.
func ExtractUtilityBillsFromContent(ctx context.Context, model chat.Chat, content string) (*UtilityBillExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &UtilityBillExtractionResult{Kind: "not_utility_bill"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	opts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 16384, Thinking: &thinking}
	// 强制 json_object 模式：模型必须输出合法 JSON，显著降低长输出中的格式错误
	opts.Format = json.RawMessage(utilityBillExtractionOutputSchema)
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "utility_bill_extract", ""), []chat.Message{
		{Role: "system", Content: utilityBillExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, opts)
	if err != nil {
		return nil, fmt.Errorf("extract utility bill fields: %w", err)
	}
	var parsed UtilityBillExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		// 容错：模型偶发输出 Markdown 修饰（反引号/代码围栏），剥离后重试
		cleaned := strings.NewReplacer("```json", "", "```", "", "`", "'").Replace(result.Content)
		if cleaned != result.Content {
			if err2 := common.ParseLLMJsonResponse(cleaned, &parsed); err2 == nil {
				return &parsed, nil
			}
		}
		logger.Warnf(ctx, "utility bill extraction raw response (first 20000 chars): %s", truncateForLog(result.Content, 20000))
		return nil, fmt.Errorf("parse utility bill extraction response: %w", err)
	}
	return &parsed, nil
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// UtilityBillBatchesHaveText reports whether any extraction batch carries
// usable text (scanned document guard, same as invoice/award-punish).
func UtilityBillBatchesHaveText(batches []string) bool {
	for _, b := range batches {
		if strings.TrimSpace(b) != "" {
			return true
		}
	}
	return false
}

// NormalizeUtilityBillExtractionResult normalizes one record per bill and
// derives grand total consistency fields.
func NormalizeUtilityBillExtractionResult(extracted *UtilityBillExtractionResult) {
	if extracted == nil {
		return
	}
	if len(extracted.Records) > 1 {
		first := extracted.Records[0]
		extracted.Records = []types.UtilityBillExtractionItem{first}
	}
	for i := range extracted.Records {
		r := &extracted.Records[i]
		if r.GrandTotal == 0 && r.TotalAmount != 0 {
			r.GrandTotal = r.TotalAmount
		}
		if r.TotalAmount == 0 && r.GrandTotal != 0 {
			r.TotalAmount = r.GrandTotal
		}
		// 清洗 fee_items：只保留标准六字段，丢弃模型编造的空行
		clean := make([]types.UtilityBillFeeItem, 0, len(r.FeeItems))
		for _, f := range r.FeeItems {
			it := types.UtilityBillFeeItem{
				Category: strings.TrimSpace(f.Category),
				Name:     strings.TrimSpace(f.Name),
				Period:   strings.TrimSpace(f.Period),
				Qty:      f.Qty,
				Rate:     f.Rate,
				Fee:      f.Fee,
				    BillFee:  f.BillFee,
				}
			if it.Name == "" && it.Fee == 0 && it.Qty == 0 && it.Category == "" {
				continue
			}
			clean = append(clean, it)
		}
		r.FeeItems = clean
	}
}

// utilityBillMonthOf extracts "MM" from a YYYY-MM-DD bill period start.
func utilityBillMonthOf(periodStart string) string {
	if len(periodStart) < 7 {
		return ""
	}
	return strings.TrimPrefix(periodStart[5:7], "0")
}

// tariffRuleContainsMonth reports whether the rule's months list (e.g. "7,8")
// contains the given month "8".
func tariffRuleContainsMonth(months, month string) bool {
	for _, m := range strings.Split(months, ",") {
		if strings.TrimSpace(m) == month {
			return true
		}
	}
	return false
}

// RecalculateUtilityBillFees 重算电费金额。核心原则：提取只填充基础要素
// （计费电量、计费标准），各子项电费一律计算得出，确保数据准确：
//  1. 零售交易电费（分时计价）：按账单月份匹配分时电价规则，fee = 时段电量 × 规则单价；
//  2. 其它「电量 × 标准」类行：fee = round(qty × rate, 2)；
//  3. 特殊项（返还/偏差/功率因数调整等 qty=0 或 rate=0 的）保持提取值，仅参与汇总。
func RecalculateUtilityBillFees(item *types.UtilityBillExtractionItem, rules []types.UtilityTariffRule) {
	if item == nil {
		return
	}
	month := utilityBillMonthOf(item.BillPeriodStart)
	var rule *types.UtilityTariffRule
	if month != "" {
		for i := range rules {
			if rules[i].Months != "" && tariffRuleContainsMonth(rules[i].Months, month) {
				rule = &rules[i]
				break
			}
		}
	}
	if rule == nil {
		for i := range rules {
			if rules[i].IsDefault {
				rule = &rules[i]
				break
			}
		}
	}
	for i := range item.FeeItems {
		f := &item.FeeItems[i]
		// 分时零售交易：优先用分时电价规则单价
		if rule != nil && strings.Contains(f.Name, "零售交易电费") && f.Period != "" {
			var rate float64
			switch f.Period {
			case "尖峰":
				rate = rule.DeepPeakRate
			case "峰":
				rate = rule.PeakRate
			case "平":
				rate = rule.FlatRate
			case "谷":
				rate = rule.ValleyRate
			}
			if rate > 0 {
				f.Rate = rate
				f.Fee = math.Round(f.Qty*rate*100) / 100
				continue
			}
		}
		// 一般乘法行
		if f.Rate != 0 && f.Qty != 0 {
			f.Fee = math.Round(f.Qty*f.Rate*100) / 100
		}
		// 其余（返还/偏差/功率因数等）保持提取值
	}
	// 容（需）量电费：容量 × 容量电价（需量计费时按需量）
	if item.Demand > 0 && item.CapacityPrice > 0 {
		item.CapacityFee = math.Round(item.Demand*item.CapacityPrice*100) / 100
	} else if item.Capacity > 0 && item.CapacityPrice > 0 {
		item.CapacityFee = math.Round(item.Capacity*item.CapacityPrice*100) / 100
	}
}
