package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// 能耗台账（energy scope）的提取服务。
//
// 与合同/制度同形态：走中台 document 管线（字段字典 + 用户规则 + 后置正则补洞），
// 但在解析后额外跑**读数反算与用量守卫**（ReconcileEnergyReading），
// 把「实际用量 = (本期读数 − 上期读数) × 倍率」这条业务恒等式固化进提取后处理。
//
// 与既有 utility 专用管线的分工：
//   - utility（knowledge_utility_extract.go）：国网电费账单**明细级**口径，
//     专用 Prompt 抽取 30~50 行费用明细 + 峰平谷分时，字段字典为空；
//   - energy（本文件）：**账期台账级**口径，中台字段字典 + 六同步 + 用量守卫。
// 两者并存：前者用于费用核对，后者用于台账与用量治理。

// energyExtractionMaxContentRunes 单次送模型的文档字符上限。
//
// 能耗汇总单通常几页（读数表 + 费用明细），必须整份作为单个批次交给模型，
// 保证账期、读数、金额落在同一条记录内（多表位时也不会裂变）。
const energyExtractionMaxContentRunes = 60000

// EnergyExtractionResult 是模型必须返回的严格 JSON 形态。
type EnergyExtractionResult struct {
	Kind         string           `json:"kind"` // "energy" | "not_energy"
	Records      []map[string]any `json:"records"`
	ExtractError string           `json:"extract_error"`
}

// energyExtractionSystemPrompt 是能耗台账提取的默认 System Prompt。
//
// 字段字典由用户配置注入（buildEnergyRuleSystemPrompt），此处的固定段只讲
// **业务恒等式与口径纪律**（读数连续性、账期月粒度、一文件一档案）。
const energyExtractionSystemPrompt = `你是一位企业能耗台账管理员，需要从水电燃气账单全文中提取账期级结构化字段。

【任务】
阅读 <document> 中的账单文本，输出严格 JSON。一份文件只对应一条能耗台账记录。

【字段与规则】
{{fields_schema}}

【输出】
{"kind": "energy", "records": [ { ...字段... } ]}

【口径纪律（必须遵守）】
1. **账期是月粒度**：统一输出 YYYY-MM（如 2026-09）。原文写「2026年09月」「2026-09」
   「2026.09」都归一为 2026-09。日期类字段（账期起止 / 抄表日期）输出 YYYY-MM-DD。
2. **读数只取主计量表**：账单常有「正向有功（总）/ 尖峰 / 峰 / 平 / 谷」多行示数，
   上期读数、本期读数、倍率都取**总表**那一行，绝不混用分表数值。
3. **数值连续性**：实际用量 = (本期读数 − 上期读数) × 倍率。
   账单已直接给出抄见电量/计费电量时，usage_amount 以**账单数值**为准；
   账单没给时把读数填全，系统会自动反算（倍率缺省为 1）。
4. **一文件一档案**：一份汇总单只输出一条记录。若单据含多个表位/计量点，
   把各表位读数放进 records[0].items 数组（每行一个表位），
   **绝不**把多个表位拆成多条 records —— 那会让一个文件裂变成多条孤立档案。
5. **金额口径**：总金额取本期应付/合计金额（元），去掉千分位与货币符号；
   不要取「上次结余 / 预缴余额 / 欠费金额」。
6. **搜索全文**：账期在封面、读数在「电量明细 / 抄表记录」表、总金额在「费用明细」合计行，
   三者常分处不同页，必须在全文范围内查找。
7. 数字字段输出为**纯数字**（不带单位与千分位）；不确定的字段留空或填 0，严禁编造。
8. 若文档与水电燃气账单无关（发票、合同、制度、奖惩通报、清单等），
   kind 设为 "not_energy"，records 返回空数组。
9. 只返回严格 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。`

// buildEnergyRuleSystemPrompt 注入用户配置的字段口径与高级模板。
//
// 与合同同款策略：用户模板在前、字段要点在中、默认纪律段在后，
// 保证「用户在设置里配的规则」优先级最高，且模型仍受硬性口径纪律约束。
func buildEnergyRuleSystemPrompt(cfg *types.KbExtractConfig) string {
	base := strings.Replace(energyExtractionSystemPrompt, "{{fields_schema}}", "", 1)
	if cfg == nil {
		return base
	}
	hasTemplate := strings.TrimSpace(cfg.PromptTemplate) != ""
	hasFields := len(cfg.Fields) > 0
	if !hasTemplate && !hasFields {
		return base
	}
	var sb strings.Builder
	if hasTemplate {
		// 存量模板可能没有 {{fields_schema}} 插槽：先幂等补齐，
		// 否则用户配的字段规则在提示词里完全缺席（界面上只挂一条告警，提取静默降级）。
		r := strings.NewReplacer("{{fields_schema}}", BuildFieldsSchema(cfg.Fields))
		sb.WriteString(strings.TrimSpace(r.Replace(EnsureFieldsSchemaSlot(cfg.PromptTemplate))))
		sb.WriteString("\n\n")
	}
	if hasFields {
		sb.WriteString("提取要点（用户配置的字段口径，请按这些要点识别能耗单内容；输出结构仍严格按下方规则执行）:\n")
		for _, f := range cfg.Fields {
			n := strings.TrimSpace(f.Name)
			if n == "" || !f.Enabled {
				continue
			}
			parts := make([]string, 0, 2)
			if d := strings.TrimSpace(f.Desc); d != "" {
				parts = append(parts, "描述："+d)
			}
			if r := strings.TrimSpace(f.Rule); r != "" {
				parts = append(parts, "规则："+r)
			}
			line := "· " + n
			if len(parts) > 0 {
				line += "（" + strings.Join(parts, "；") + "）"
			}
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(base)
	return sb.String()
}

// BuildEnergyExtractionBatches 组装文档文本为独立批次。
//
// 能耗汇总单必须整份作为单个批次交给模型，确保账期、读数、金额落在同一条记录内；
// 仅当内容超出上下文上限时才拆分。
func BuildEnergyExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
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
	return splitTextBatches(content, energyExtractionMaxContentRunes+1, energyExtractionMaxContentRunes)
}

// ExtractEnergyFromContentWithRules 从文档内容提取能耗记录（支持用户规则）。
func ExtractEnergyFromContentWithRules(
	ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig,
) (*EnergyExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &EnergyExtractionResult{Kind: "not_energy", ExtractError: "no text content to extract"}, nil
	}
	systemPrompt := buildEnergyRuleSystemPrompt(cfg)
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "<document>\n" + content + "\n</document>"},
	}
	thinking := false
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	raw, err := RunExtractWithRetry(
		types.WithLLMCallMetadata(ctx, "energy_extract", ""),
		model, messages, baseOpts, DefaultExtractAttempts(nil),
		func(raw string) bool {
			_, perr := parseEnergyReply(raw)
			return perr == nil
		},
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "energy extraction unusable after retries: %v (len=%d)", lastErr, len(lastRaw))
		},
	)
	if err != nil {
		return nil, fmt.Errorf("extract energy fields: %w", err)
	}
	parsed, perr := parseEnergyReply(raw)
	if perr != nil {
		return nil, fmt.Errorf("extract energy fields: %w", perr)
	}
	return parsed, nil
}

// parseEnergyReply 解析能耗提取回复（统一走 ParseLLMJsonResponse 清洗层）。
func parseEnergyReply(raw string) (*EnergyExtractionResult, error) {
	var parsed EnergyExtractionResult
	if err := common.ParseLLMJsonResponse(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse energy extraction response: %w", err)
	}
	return &parsed, nil
}

// ReconcileEnergyRecords 对一批能耗记录跑读数反算与用量守卫。
//
// 这是提取后处理的核心收口：
//   - 记录级：主表读数上提后反算实际用量（缺用量则补齐，缺证据绝不臆造）；
//   - 明细级：逐个表位独立反算，偏差行标 UsageMismatch；
//   - 告警统一汇入 Issues，供前端黄色告警面板渲染。
//
// 返回值是**新记录**（不改入参）：落库前必须再跑一次，避免被后续步骤覆盖。
func ReconcileEnergyRecords(records []map[string]any) ([]map[string]any, int) {
	if len(records) == 0 {
		return records, 0
	}
	out := make([]map[string]any, 0, len(records))
	issueTotal := 0
	for _, m := range records {
		rec := energyRecordFromMap(m)
		issueTotal += len(rec.Issues)
		// 写回 map（保持 map 形态，因为落库契约是 map 列表）
		nm := make(map[string]any, len(m)+4)
		for k, v := range m {
			nm[k] = v
		}
		nm["previous_reading"] = energyValueOrNil(rec.PreviousReading)
		nm["current_reading"] = energyValueOrNil(rec.CurrentReading)
		nm["multiplier"] = energyValueOrNil(rec.Multiplier)
		nm["usage_amount"] = energyValueOrNil(rec.UsageAmount)
		nm["unit_price"] = energyValueOrNil(rec.UnitPrice)
		nm["total_amount"] = energyValueOrNil(rec.TotalAmount)
		nm["usage_patched"] = rec.UsagePatched
		if len(rec.Issues) > 0 {
			issues := make([]any, 0, len(rec.Issues))
			for _, iss := range rec.Issues {
				b, _ := json.Marshal(iss)
				var v any
				_ = json.Unmarshal(b, &v)
				issues = append(issues, v)
			}
			nm["issues"] = issues
		}
		// 明细：写回指针化后的读数与推算值
		if len(rec.Items) > 0 {
			items := make([]any, 0, len(rec.Items))
			for _, it := range rec.Items {
				im := map[string]any{
					"meter_no":       it.MeterNo,
					"meter_type":     it.MeterType,
					"billing_period": it.BillingPeriod,
					"remarks":        it.Remarks,
					"usage_mismatch": it.UsageMismatch,
				}
				im["previous_reading"] = energyValueOrNil(it.PreviousReading)
				im["current_reading"] = energyValueOrNil(it.CurrentReading)
				im["multiplier"] = energyValueOrNil(it.Multiplier)
				im["usage_amount"] = energyValueOrNil(it.UsageAmount)
				im["computed_usage"] = energyValueOrNil(it.ComputedUsage)
				items = append(items, im)
			}
			nm["items"] = items
		}
		out = append(out, nm)
	}
	return out, issueTotal
}

// energyValueOrNil 指针转 map 值（nil → nil，保留「空」与「0」的区别）。
func energyValueOrNil(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
