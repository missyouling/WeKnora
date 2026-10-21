package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/infrastructure/docparser"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// contractExtractMaxContentRunes bounds the document text sent to the
// extraction model so the prompt stays inside a normal context window.
const contractExtractMaxContentRunes = 24000

// ContractExtractionItem mirrors one contract found in a document.
// Numeric fields use pointers so an unrecognized value stays null on the
// wire instead of being coerced to 0.
type ContractExtractionItem struct {
	// —— 标准字段（11）：顺序即字段定义的权威顺序 ——
	ContractNo   string   `json:"contract_no"`
	ContractName string   `json:"contract_name"`
	ContractType string   `json:"contract_type"`
	PartyA       string   `json:"party_a"`
	PartyB       string   `json:"party_b"`
	SignDate     string   `json:"sign_date"`
	ExpireDate   string   `json:"expire_date"`
	Amount       *float64 `json:"amount"`
	// PerformanceStatus 由到期日期派生（履行中/即将到期/已超期/已终止），
	// 模型可留空，列表组装时按 expire_date 计算，前端只读展示。
	PerformanceStatus string `json:"performance_status"`
	// —— 扩展字段（15）——
	PartyAContact string `json:"party_a_contact"`
	PartyATaxNo   string `json:"party_a_tax_no"`
	PartyBContact string `json:"party_b_contact"`
	PartyBTaxNo   string `json:"party_b_tax_no"`
	PartyAAddress string `json:"party_a_address"`
	PartyAPhone   string `json:"party_a_phone"`
	PartyABank    string `json:"party_a_bank"`
	PartyAAccount string `json:"party_a_account"`
	PartyBAddress string `json:"party_b_address"`
	PartyBPhone   string `json:"party_b_phone"`
	PartyBBank    string `json:"party_b_bank"`
	PartyBAccount string `json:"party_b_account"`
	PaymentMethod string `json:"payment_method"`
	SignLocation  string `json:"sign_location"`
	// Summary 摘要：优先复用知识库已生成的摘要（knowledges.description），
	// 模型只在原文有独立摘要段时覆盖。
	Summary string `json:"summary"`
	// —— 历史字段：保留读写以兼容存量数据与旧提示词，不再进入字段定义/列表 ——
	EffectiveDate     string   `json:"effective_date,omitempty"`
	TaxRate           *float64 `json:"tax_rate,omitempty"`
	QualityBond       *float64 `json:"quality_bond,omitempty"`
	LiquidatedDamages *float64 `json:"liquidated_damages,omitempty"`
	Subject           string   `json:"subject,omitempty"`
	Quantity          *float64 `json:"quantity,omitempty"`
	UnitPrice         *float64 `json:"unit_price,omitempty"`
	PerformancePeriod string   `json:"performance_period,omitempty"`
	Handler           string   `json:"handler,omitempty"`
	Department        string   `json:"department,omitempty"`
	Remark            string   `json:"remark,omitempty"`
	// Page is the 1-based page/ordinal of the contract inside its source file.
	// 合同一文件一档案，恒为 1。
	Page int `json:"page"`
}

// contractLegacyFieldAliases 存量字段名 → 新契约字段名。
//
// 2026-10-21 合同字段体系重置（party_a_name → party_a、contract_amount → amount…）
// 前落库的 metadata 仍是旧键。读时统一映射，避免"老数据打开详情全空"，
// 也避免为此写一次不可回滚的数据迁移。
var contractLegacyFieldAliases = map[string]string{
	"party_a_name":    "party_a",
	"party_b_name":    "party_b",
	"expiry_date":     "expire_date",
	"contract_amount": "amount",
	"sign_place":      "sign_location",
	"fulfill_status":  "performance_status",
}

// UnmarshalJSON 兼容旧字段名（见 contractLegacyFieldAliases），新键优先。
func (c *ContractExtractionItem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for legacy, canonical := range contractLegacyFieldAliases {
		if _, ok := raw[canonical]; ok {
			continue
		}
		if v, ok := raw[legacy]; ok {
			raw[canonical] = v
		}
	}
	normalized, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	type alias ContractExtractionItem
	var a alias
	if err := json.Unmarshal(normalized, &a); err != nil {
		return err
	}
	*c = ContractExtractionItem(a)
	return nil
}

// ContractExtractionResult is the strict-JSON shape the model must return.
type ContractExtractionResult struct {
	Kind         string                   `json:"kind"` // "contract" | "not_contract"
	Contracts    []ContractExtractionItem `json:"contracts"`
	ExtractError string                   `json:"extract_error"`
}

// contractExtractionSystemPrompt instructs the model to return strict JSON
// with every contract found in the document.
const contractExtractionSystemPrompt = `你是一个专业的合同信息提取助手。你的任务是从文档文本中识别并提取合同的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 【一文件一档案】一份上传文件在系统中严格对应一条合同记录。即使文档很长、被分段送入，也只输出 contracts 数组中的一个元素；把全文各段的信息合并到这一个对象里（前面段落的甲乙方/金额 + 后面段落的签订日期/盖章信息）。
3. 同一份合同在文档中重复出现（复印件、重复贴页）时只提取一次，不要重复。
4. 如果文档内容与合同无关（如发票、制度、清单、报表等），kind 设为 "not_contract"，contracts 返回空数组。
5. amount（合同金额）使用数字类型，单位为元；无法识别时返回 null。
6. 合同双方：party_a / party_b 为甲方/乙方名称；party_a_contact / party_b_contact 为甲乙方经办人（联系人）；party_a_tax_no / party_b_tax_no 为统一社会信用代码；party_a_address / party_b_address 为地址；party_a_phone / party_b_phone 为电话；party_a_bank / party_b_bank 为开户行；party_a_account / party_b_account 为银行账号。无法识别时返回空字符串。甲方是合同中的"甲方"一方，乙方是"乙方"一方；若只有单方主体信息则填入对应方，另一方留空。
7. 日期字段 sign_date（签订日期）、expire_date（到期日期 / 合同终止日期）使用 YYYY-MM-DD 格式；无法识别时返回空字符串。
8. contract_type 只从以下枚举中选择：采购合同、销售合同、服务合同、租赁合同、技术合同、运输合同、借款合同、保密协议、其它合同。根据合同名称或正文关键字判断：含「采购」→采购合同；含「销售」「售」→销售合同；含「租赁」「租」→租赁合同；含「服务」→服务合同；含「技术」→技术合同；含「运输」「货运」→运输合同；含「借款」「贷款」→借款合同；含「保密」「NDA」→保密协议；都不含时用「其它合同」。
9. payment_method 为付款方式，只从以下枚举中选择：一次性、分期、按进度；无法识别时返回空字符串。
10. sign_location 为签订地点（合同尾部"签订地点/签署地"）；无法识别时返回空字符串。
11. summary 为合同内容摘要：仅当原文自带"摘要/内容摘要"段落时填写，否则返回空字符串（系统会用知识库已生成的文档摘要回填，不要自行编造）。
12. performance_status 由到期日期派生，模型不需要输出（保持空字符串）。
13. contract_no 为合同编号；若文档中确实没有合同编号，返回空字符串（不要编造）。
14. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"contract","contracts":[{"contract_no":"","contract_name":"","contract_type":"","party_a":"","party_b":"","sign_date":"","expire_date":"","amount":0,"performance_status":"","party_a_contact":"","party_a_tax_no":"","party_b_contact":"","party_b_tax_no":"","party_a_address":"","party_a_phone":"","party_a_bank":"","party_a_account":"","party_b_address":"","party_b_phone":"","party_b_bank":"","party_b_account":"","payment_method":"","sign_location":"","summary":""}]}`

// BuildContractExtractionContent assembles the document text sent to the model.
func BuildContractExtractionContent(name, summary string, chunks []*types.Chunk) string {
	parts := make([]string, 0, len(chunks)+2)
	if name = strings.TrimSpace(name); name != "" {
		parts = append(parts, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		parts = append(parts, "Existing summary: "+summary)
	}
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
			parts = append(parts, text)
		}
	}
	return sampleLongContent(strings.Join(parts, "\n\n"), contractExtractMaxContentRunes)
}

// ExtractContractsFromContent calls the configured chat model and parses its
// strict-JSON reply into a ContractExtractionResult.
func ExtractContractsFromContent(ctx context.Context, model chat.Chat, content string) (*ContractExtractionResult, error) {
	return extractContractsWithSystemPrompt(ctx, model, contractExtractionSystemPrompt, content)
}

// extractContractsWithSystemPrompt 是合同提取的唯一实现：走通用执行器
// （流式 → 非流式 → 非流式+schema → 流式重试），并复用跨模型清洗层
// （<think> 块剥离 / 尾逗号 / 千分位与引号金额）。
//
// 此前这里是单次非流式调用、MaxTokens=8192，一次解析失败就直接抛错——合同是长文本，
// 恰恰最容易触发供应商空 content 与输出截断（2026-10 实测）。收编后统一为 4 次机会。
// MaxTokens 收敛为 0：显式传 max_tokens 是部分供应商返回空 content 的诱因。
func extractContractsWithSystemPrompt(
	ctx context.Context,
	model chat.Chat,
	systemPrompt string,
	content string,
) (*ContractExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &ContractExtractionResult{Kind: "not_contract", ExtractError: "no text content to extract"}, nil
	}
	return extractContractsWithUserPrompt(ctx, model, systemPrompt,
		"<document>\n"+content+"\n</document>", DefaultExtractAttempts(nil))
}

// extractContractsWithUserPrompt 是执行器调用的最内层实现：userPrompt 可被调用方改写
// （单份合同重提取会追加"只提取第 N 份"的引导），attempts 可退化为单次尝试。
func extractContractsWithUserPrompt(
	ctx context.Context,
	model chat.Chat,
	systemPrompt string,
	userPrompt string,
	attempts []ExtractAttempt,
) (*ContractExtractionResult, error) {
	thinking := false
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}
	raw, err := RunExtractWithRetry(
		types.WithLLMCallMetadata(ctx, "contract_extract", ""),
		model, messages, baseOpts, attempts,
		func(raw string) bool {
			_, perr := parseContractReply(raw)
			return perr == nil
		},
		func(lastRaw string, lastErr error) {
			logger.Warnf(ctx, "contract extraction unusable after retries: %v (len=%d)", lastErr, len(lastRaw))
		},
	)
	if err != nil {
		return nil, fmt.Errorf("extract contract fields: %w", err)
	}
	parsed, perr := parseContractReply(raw)
	if perr != nil {
		return nil, fmt.Errorf("extract contract fields: %w", perr)
	}
	return parsed, nil
}

// parseContractReply 解析合同提取回复（统一走 ParseLLMJsonResponse 清洗层）。
func parseContractReply(raw string) (*ContractExtractionResult, error) {
	var parsed ContractExtractionResult
	if err := common.ParseLLMJsonResponse(raw, &parsed); err != nil {
		return nil, fmt.Errorf("parse contract extraction response: %w", err)
	}
	return &parsed, nil
}

// ExtractContractPageFromContent asks the model to extract only the Nth contract
// from the document text, keeping single-page re-extraction cheap.
func ExtractContractPageFromContent(ctx context.Context, model chat.Chat, content string, page int) (*ContractExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &ContractExtractionResult{Kind: "not_contract", ExtractError: "no text content to extract"}, nil
	}
	if page < 1 {
		return &ContractExtractionResult{Kind: "not_contract", ExtractError: "invalid page number"}, nil
	}
	userPrompt := fmt.Sprintf(
		"以下文档中包含多份合同。请只提取文档中的第 %d 份合同（合同按文档中出现的先后顺序从 1 开始编号）。contracts 数组只包含这一份合同，字段规则不变；若无法确定第 %d 份合同，contracts 返回空数组。\n<document>\n%s\n</document>",
		page, page, strings.TrimSpace(content))
	// 单份重提取同样走执行器（清洗层 + 思考参数自适应 + 单次硬超时），
	// 但**只尝试一次**：这是用户主动触发的定点修复，4 次尝试会把成本翻四倍。
	res, err := extractContractsWithUserPrompt(ctx, model, contractExtractionSystemPrompt,
		userPrompt, SingleAttemptExtractAttempts())
	if err != nil {
		return nil, fmt.Errorf("extract contract page: %w", err)
	}
	return res, nil
}

// ContractExtractionBatchSize caps how many chunks go into one LLM call.
const ContractExtractionBatchSize = 10

// BuildContractExtractionBatches assembles the document text into independent
// batches so long multi-contract documents extract reliably.
func BuildContractExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = ContractExtractionBatchSize
	}
	// 取数/归一收敛到唯一实现：ChunksForExtraction 负责类型过滤与相邻重复去重，
	// NormalizeHTMLTables 把裸 HTML 表格归一成模型可读的行文本（合同/制度扫描件
	// 常带 <table>，不归一会让模型把整张表读成噪声）；有物理页码时插入 [Page N]
	// 锚点，便于后续按页归位。
	usable := ChunksForExtraction(chunks)
	header := make([]string, 0, 2)
	if name = strings.TrimSpace(name); name != "" {
		header = append(header, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		header = append(header, "Existing summary: "+summary)
	}
	headStr := strings.Join(header, "\n\n")

	var batches []string
	for i := 0; i < len(usable); i += batchSize {
		end := i + batchSize
		if end > len(usable) {
			end = len(usable)
		}
		parts := make([]string, 0, end-i+1)
		if headStr != "" {
			parts = append(parts, headStr)
		}
		for _, c := range usable[i:end] {
			text := strings.TrimSpace(docparser.NormalizeHTMLTables(c.Content))
			if text == "" {
				continue
			}
			if page := ChunkPage(c); page > 0 {
				text = extractPageMarker(page) + "\n" + text
			}
			parts = append(parts, text)
		}
		batches = append(batches, sampleLongContent(strings.Join(parts, "\n\n"), contractExtractMaxContentRunes))
	}
	if len(batches) == 0 {
		batches = []string{""}
	}
	return batches
}

// NormalizeContractExtractionResult cleans up a model reply: it guarantees a
// known kind, drops empty items, normalizes contract type and auto-numbers
// contracts without an explicit number, and assigns the 1-based page ordinal.
//
// autoSeqStart is the highest auto-number already used in the knowledge base
// for the current date (HT-YYYYMMDD-NNN), so generated numbers stay unique
// across files; pass 0 to start from 001 (file-local numbering).
func NormalizeContractExtractionResult(res *ContractExtractionResult, autoSeqStart int) {
	if res == nil {
		return
	}
	if res.Kind != "not_contract" {
		res.Kind = "contract"
	}
	if len(res.Contracts) == 0 {
		return
	}
	// 一文件一档案：多批次结果先按非空优先合并为唯一一条，再进入去重/编号。
	// 否则两批各出一份 → 自动编号生成 -001/-002，列表条数多于上传文件数（真机事故）。
	if merged, ok := MergeExtractionItems(res.Contracts); ok {
		res.Contracts = []ContractExtractionItem{merged}
	}
	dedup := make(map[string]struct{}, len(res.Contracts))
	kept := res.Contracts[:0]
	now := time.Now()
	for i, ct := range res.Contracts {
		key := strings.TrimSpace(ct.ContractNo)
		if key != "" {
			if _, seen := dedup[key]; seen {
				continue // in-file duplicate: keep the first occurrence
			}
			dedup[key] = struct{}{}
		}
		ct.ContractType = NormalizeContractTypeFromName(ct.ContractType)
		// 合同一份文件对应一份合同，不按页展开：Page 恒为 1（区别于发票多页）。
		ct.Page = 1
		// 自动编号：无合同编号时按上传日期自动编号 HT-YYYYMMDD-NNN。
		// NNN 从 autoSeqStart+1 起顺序递增，保证同一天内跨文件不重复。
		if strings.TrimSpace(ct.ContractNo) == "" {
			ct.ContractNo = fmt.Sprintf("HT-%s-%03d", now.Format("20060102"), autoSeqStart+i+1)
		}
		kept = append(kept, ct)
	}
	res.Contracts = kept
}

// MaxAutoContractSeq returns the highest trailing sequence number among
// contracts with an auto-generated number HT-YYYYMMDD-NNN for the current date
// inside the knowledge base. Used to keep auto-numbers unique across files.

// NormalizeContractTypeFromName maps a raw type string onto the fixed enum by
// keyword priority:
//
//	含"采购" → 采购合同
//	含"销售"/"售" → 销售合同
//	含"租赁"/"租" → 租赁合同
//	含"服务" → 服务合同
//	含"技术" → 技术合同
//	含"运输"/"货运" → 运输合同
//	含"借款"/"贷款" → 借款合同
//	含"保密"/"NDA" → 保密协议
//	否则 → 其它合同
//
// A blank input stays blank (model could not recognize a type).
func NormalizeContractTypeFromName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "采购"):
		return "采购合同"
	case strings.Contains(raw, "销售") || strings.Contains(raw, "售"):
		return "销售合同"
	case strings.Contains(raw, "租赁") || strings.Contains(raw, "租"):
		return "租赁合同"
	case strings.Contains(raw, "服务"):
		return "服务合同"
	case strings.Contains(raw, "技术"):
		return "技术合同"
	case strings.Contains(raw, "运输") || strings.Contains(raw, "货运"):
		return "运输合同"
	case strings.Contains(raw, "借款") || strings.Contains(raw, "贷款"):
		return "借款合同"
	case strings.Contains(raw, "保密") || strings.Contains(raw, "NDA") || strings.Contains(raw, "nda"):
		return "保密协议"
	default:
		return "其它合同"
	}
}

// validContractTypes is the fixed contract type enum surfaced in the UI.
var validContractTypes = map[string]struct{}{
	"采购合同": {},
	"销售合同": {},
	"服务合同": {},
	"租赁合同": {},
	"技术合同": {},
	"运输合同": {},
	"借款合同": {},
	"保密协议": {},
	"其它合同": {},
}

// validFulfillStatuses is the business fulfillment status enum.
var validFulfillStatuses = map[string]struct{}{
	"待签署": {},
	"执行中": {},
	"已完成": {},
	"已到期": {},
	"已终止": {},
}

// ContractsAllEmpty reports whether every contract in the result is
// effectively blank. Used to guard against persisting a garbage/truncated
// model reply as a successful extraction.
func ContractsAllEmpty(cts []ContractExtractionItem) bool {
	for _, ct := range cts {
		amt := ct.Amount != nil && *ct.Amount != 0
		if ct.ContractNo != "" || ct.ContractName != "" || ct.ContractType != "" ||
			ct.PartyA != "" || ct.PartyB != "" || amt {
			return false
		}
	}
	return true
}

// ============ 规则提取（用户可配置字段口径） ============

// buildContractRuleSystemPrompt 在默认合同提示词基础上注入用户配置的提取规则
// （字段口径、高级模板）。规则为空时行为与默认版一致。
func buildContractRuleSystemPrompt(cfg *types.KbExtractConfig) string {
	if cfg == nil {
		return contractExtractionSystemPrompt
	}
	hasTemplate := strings.TrimSpace(cfg.PromptTemplate) != ""
	hasFields := len(cfg.Fields) > 0
	if !hasTemplate && !hasFields {
		return contractExtractionSystemPrompt
	}
	var sb strings.Builder
	if hasTemplate {
		r := strings.NewReplacer("{{fields_schema}}", BuildFieldsSchema(cfg.Fields))
		sb.WriteString(strings.TrimSpace(r.Replace(cfg.PromptTemplate)))
		sb.WriteString("\n\n")
	}
	if hasFields {
		sb.WriteString("提取要点（用户配置的字段口径，请按这些要点识别合同内容；输出结构仍严格按下方规则执行）:\n")
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
	sb.WriteString(contractExtractionSystemPrompt)
	return sb.String()
}

// ExtractContractsFromContentWithRules 与 ExtractContractsFromContent 相同，但支持注入
// 用户配置的提取规则（字段口径、高级模板），规则为空时行为与默认版一致。
func ExtractContractsFromContentWithRules(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig) (*ContractExtractionResult, error) {
	return extractContractsWithSystemPrompt(ctx, model, buildContractRuleSystemPrompt(cfg), content)
}
