package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/common"
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
	ContractNo       string   `json:"contract_no"`
	ContractName     string   `json:"contract_name"`
	ContractType     string   `json:"contract_type"`
	SignDate         string   `json:"sign_date"`
	EffectiveDate    string   `json:"effective_date"`
	ExpiryDate       string   `json:"expiry_date"`
	SignPlace        string   `json:"sign_place"`
	PartyAName       string   `json:"party_a_name"`
	PartyATaxNo      string   `json:"party_a_tax_no"`
	PartyAAddress    string   `json:"party_a_address"`
	PartyAPhone      string   `json:"party_a_phone"`
	PartyABank       string   `json:"party_a_bank"`
	PartyAAccount    string   `json:"party_a_account"`
	PartyBName       string   `json:"party_b_name"`
	PartyBTaxNo      string   `json:"party_b_tax_no"`
	PartyBAddress    string   `json:"party_b_address"`
	PartyBPhone      string   `json:"party_b_phone"`
	PartyBBank       string   `json:"party_b_bank"`
	PartyBAccount    string   `json:"party_b_account"`
	ContractAmount   *float64 `json:"contract_amount"`
	TaxRate          *float64 `json:"tax_rate"`
	PaymentMethod    string   `json:"payment_method"`
	QualityBond      *float64 `json:"quality_bond"`
	LiquidatedDamages *float64 `json:"liquidated_damages"`
	Subject          string   `json:"subject"`
	Quantity         *float64 `json:"quantity"`
	UnitPrice        *float64 `json:"unit_price"`
	PerformancePeriod string  `json:"performance_period"`
	Handler          string   `json:"handler"`
	Department       string   `json:"department"`
	Remark           string   `json:"remark"`
	FulfillStatus    string   `json:"fulfill_status"`
	// Page is the 1-based page/ordinal of the contract inside its source file.
	// It is assigned by the backend (in extraction-result order).
	Page int `json:"page"`
}

// ContractExtractionResult is the strict-JSON shape the model must return.
type ContractExtractionResult struct {
	Kind         string                   `json:"kind"` // "contract" | "not_contract"
	Contracts    []ContractExtractionItem `json:"contracts"`
	ExtractError string                   `json:"extract_error"`
}

// contractExtractionSystemPrompt instructs the model to return strict JSON
// with every contract found in the document.
const contractExtractionSystemPrompt = `你是一个专业的合同信息提取助手。你的任务是从文档文本中识别并提取所有合同的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 一份文档可能包含多份合同（如汇总签署页、多份协议合订本），请将全部合同提取到 contracts 数组中。
3. 同一份合同在文档中重复出现（复印件、重复贴页）时只提取一次，不要重复。
4. 如果文档内容与合同无关（如发票、制度、清单、报表等），kind 设为 "not_contract"，contracts 返回空数组。
5. 金额字段 contract_amount、quality_bond（质保金）、liquidated_damages（违约金）、quantity（数量）、unit_price（单价）使用数字类型，单位为元；无法识别时返回 null。
6. tax_rate 使用小数（如 3% 记为 0.03，13% 记为 0.13）；无法识别时返回 null。
7. 日期字段 sign_date（签订日期）、effective_date（生效日期）、expiry_date（到期日期）使用 YYYY-MM-DD 格式；无法识别时返回空字符串。
8. 合同双方：party_a_name/party_b_name 为甲方/乙方名称，party_a_tax_no/party_b_tax_no 为统一社会信用代码，party_a_address/party_b_address 为地址，party_a_phone/party_b_phone 为电话，party_a_bank/party_b_bank 为开户行，party_a_account/party_b_account 为银行账号；无法识别时返回空字符串。甲方是合同中的"甲方"一方，乙方是"乙方"一方，若只有单方主体信息则填入对应方，另一方留空。
9. contract_type 只从以下枚举中选择：采购合同、销售合同、服务合同、租赁合同、技术合同、运输合同、借款合同、保密协议、其它合同。根据合同名称或正文关键字判断：含「采购」→采购合同；含「销售」「售」→销售合同；含「租赁」「租」→租赁合同；含「服务」→服务合同；含「技术」→技术合同；含「运输」「货运」→运输合同；含「借款」「贷款」→借款合同；含「保密」「NDA」→保密协议；都不含时用「其它合同」。
10. payment_method 为付款方式，只从以下枚举中选择：一次性、分期、按进度；无法识别时返回空字符串。
11. subject 为标的/项目名称，performance_period 为履行期限，handler 为经办人，department 为部门，remark 为备注/特别约定；无法识别时返回空字符串。
12. contract_no 为合同编号；若文档中确实没有合同编号，返回空字符串（不要编造）。
13. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。

输出格式：
{"kind":"contract","contracts":[{"contract_no":"","contract_name":"","contract_type":"","sign_date":"","effective_date":"","expiry_date":"","sign_place":"","party_a_name":"","party_a_tax_no":"","party_a_address":"","party_a_phone":"","party_a_bank":"","party_a_account":"","party_b_name":"","party_b_tax_no":"","party_b_address":"","party_b_phone":"","party_b_bank":"","party_b_account":"","contract_amount":0,"tax_rate":0,"payment_method":"","quality_bond":0,"liquidated_damages":0,"subject":"","quantity":0,"unit_price":0,"performance_period":"","handler":"","department":"","remark":"","fulfill_status":""}]}`

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
	if strings.TrimSpace(content) == "" {
		return &ContractExtractionResult{Kind: "not_contract", ExtractError: "no text content to extract"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "contract_extract", ""), []chat.Message{
		{Role: "system", Content: contractExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract contract fields: %w", err)
	}
	var parsed ContractExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
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
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "contract_extract", ""), []chat.Message{
		{Role: "system", Content: contractExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract contract page: %w", err)
	}
	var parsed ContractExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		return nil, fmt.Errorf("parse contract page response: %w", err)
	}
	return &parsed, nil
}

// ContractExtractionBatchSize caps how many chunks go into one LLM call.
const ContractExtractionBatchSize = 10

// BuildContractExtractionBatches assembles the document text into independent
// batches so long multi-contract documents extract reliably.
func BuildContractExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = ContractExtractionBatchSize
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
		if strings.TrimSpace(chunk.Content) != "" {
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
			parts = append(parts, strings.TrimSpace(c.Content))
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
func (s *knowledgeService) MaxAutoContractSeq(ctx context.Context, kbID string) (int, error) {
	tenantID := ctx.Value(types.TenantIDContextKey).(uint64)
	prefix := "HT-" + time.Now().Format("20060102") + "-"
	max := 0
	page := 1
	const batch = 1000
	for {
		p := &types.Pagination{Page: page, PageSize: batch}
		knowledges, _, err := s.repo.ListPagedKnowledgeByKnowledgeBaseID(ctx, tenantID, kbID, p, types.KnowledgeListFilter{})
		if err != nil {
			return max, err
		}
		if len(knowledges) == 0 {
			break
		}
		for _, k := range knowledges {
			if k == nil || len(k.CustomMetadata) == 0 {
				continue
			}
			var meta contractMetadata
			if err := json.Unmarshal(k.CustomMetadata, &meta); err != nil || meta.Kind != "contract" {
				continue
			}
			for _, ct := range meta.Contracts {
				if !strings.HasPrefix(ct.ContractNo, prefix) {
					continue
				}
				n := strings.TrimPrefix(ct.ContractNo, prefix)
				if v, aerr := strconv.Atoi(n); aerr == nil && v > max {
					max = v
				}
			}
		}
		if len(knowledges) < batch {
			break
		}
		page++
	}
	return max, nil
}

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
		amt := ct.ContractAmount != nil && *ct.ContractAmount != 0
		if ct.ContractNo != "" || ct.ContractName != "" || ct.ContractType != "" ||
			ct.PartyAName != "" || ct.PartyBName != "" || amt {
			return false
		}
	}
	return true
}
