package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// invoiceExtractMaxContentRunes bounds the document text sent to the
// extraction model so the prompt stays inside a normal context window
// even for long PDFs.
const invoiceExtractMaxContentRunes = 24000

// InvoiceExtractionItem mirrors one invoice found in a document.
// Numeric fields use pointers so an unrecognized value stays null on the
// wire instead of being coerced to 0.
type InvoiceExtractionItem struct {
	InvoiceNo    string                  `json:"invoice_no"`
	InvoiceCode  string                  `json:"invoice_code"`
	InvoiceDate  string                  `json:"invoice_date"`
	InvoiceType  string                  `json:"invoice_type"`
	TotalAmount  *float64                `json:"total_amount"`
	Amount       *float64                `json:"amount"`
	Tax          *float64                `json:"tax"`
	TaxRate      *float64                `json:"tax_rate"`
	SellerName   string                  `json:"seller_name"`
	SellerTaxNo  string                  `json:"seller_tax_no"`
	SellerAddr   string                  `json:"seller_address"`
	SellerPhone  string                  `json:"seller_phone"`
	SellerBank   string                  `json:"seller_bank"`
	SellerAcct   string                  `json:"seller_account"`
	BuyerName    string                  `json:"buyer_name"`
	BuyerTaxNo   string                  `json:"buyer_tax_no"`
	BuyerAddr    string                  `json:"buyer_address"`
	BuyerPhone   string                  `json:"buyer_phone"`
	BuyerBank    string                  `json:"buyer_bank"`
	BuyerAcct    string                  `json:"buyer_account"`
	Issuer       string                  `json:"issuer"`
	Remark       string                  `json:"remark"`
	Category     string                  `json:"category"`
	Items        []InvoiceExtractionLine `json:"items"`
	VoidFlag     bool                    `json:"void_flag"`
	Duplicate    bool                    `json:"duplicate"`
	// Page is the 1-based page/ordinal of the invoice inside its source file.
	// It is assigned by the backend (in extraction-result order) so the
	// floating toolbar can target a single page for re-extraction; legacy
	// records without it keep 0.
	Page int `json:"page"`
}

// InvoiceExtractionLine is one goods/service line inside an invoice.
type InvoiceExtractionLine struct {
	Name    string   `json:"name"`
	Qty     *float64 `json:"qty"`
	Price   *float64 `json:"price"`
	TaxRate *float64 `json:"tax_rate"`
}

// InvoiceExtractionResult is the strict-JSON shape the model must return.
type InvoiceExtractionResult struct {
	Kind         string                  `json:"kind"` // "invoice" | "not_invoice"
	Invoices     []InvoiceExtractionItem `json:"invoices"`
	ExtractError string                  `json:"extract_error"`
}

// invoiceExtractionSystemPrompt instructs the model to return strict JSON
// with every invoice found in the document. The output schema is written
// inline so the model reproduces the keys verbatim; a separate schema pass
// would risk drifting from what the service expects.
const invoiceExtractionSystemPrompt = `你是一个专业的发票信息提取助手。你的任务是从文档文本中识别并提取所有发票的关键字段信息。

规则：
1. 只提取文本中真实存在的信息，不要编造任何字段。
2. 一份文档可能包含多张发票，请将全部发票提取到 invoices 数组中。
3. 同一张发票在文档中重复出现（复印件、多次贴票）时只提取一次，不要重复。
4. 如果文档内容与发票无关（如合同、协议、制度、清单等），kind 设为 "not_invoice"，invoices 返回空数组。
5. 金额字段 total_amount、amount、tax 使用数字类型，单位为元；无法识别时返回 null。
6. tax_rate 使用小数（如 3% 记为 0.03）；无法识别时返回 null。
7. invoice_date 使用 YYYY-MM-DD 格式；无法识别时返回空字符串。
8. 发票作废或冲红时，void_flag 设为 true。
9. items 为货物或应税劳务明细，无法识别时返回空数组。
10. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。
11. 销售方/购买方字段：seller_name/buyer_name 为名称，seller_tax_no/buyer_tax_no 为统一社会信用代码，seller_address/buyer_address 为注册地址，seller_phone/buyer_phone 为电话，seller_bank/buyer_bank 为开户行，seller_account/buyer_account 为银行账号；无法识别时返回空字符串。
12. issuer 为开票人，remark 为备注/备注栏内容；无法识别时返回空字符串。
13. invoice_type 只从以下枚举中选择：专用发票、普通发票、医疗收据、财政收据、其它票据；含「通行费」或「电子发票」的票据一律归为「普通发票」（通行费电子发票均为增值税普通发票）；无法识别时返回空字符串。
14. category 为发票大分类，只从以下枚举中选择：通行费、办公费、差旅费、餐饮费、通讯费、加油费、住宿费、材料费、设备购置费、服务费、广告费、培训费、会议费、租赁费、其它。根据发票的商品或劳务名称、销方类型判断最匹配的一个大类；无法判定时用「其它」。
15. 如果发票包含车牌号、通行日期等运输信息（如 ETC 通行费发票），将车牌号、通行日期起止以「车牌号：XX；通行日期起：XX；通行日期止：XX」格式写入 remark 字段（与已有备注内容用分号拼接）；无则保持原样。

输出格式：
{"kind":"invoice","invoices":[{"invoice_no":"","invoice_code":"","invoice_date":"","invoice_type":"","total_amount":0,"amount":0,"tax":0,"tax_rate":0,"seller_name":"","seller_tax_no":"","seller_address":"","seller_phone":"","seller_bank":"","seller_account":"","buyer_name":"","buyer_tax_no":"","buyer_address":"","buyer_phone":"","buyer_bank":"","buyer_account":"","issuer":"","remark":"","category":"","void_flag":false,"duplicate":false,"items":[{"name":"","qty":1,"price":0,"tax_rate":0}]}]}`

// BuildInvoiceExtractionContent assembles the document text sent to the
// model, mirroring the auto-tag content builder: document name + summary
// first, then text/OCR chunks in reading order, bounded to a rune cap.
func BuildInvoiceExtractionContent(name, summary string, chunks []*types.Chunk) string {
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
	return sampleLongContent(strings.Join(parts, "\n\n"), invoiceExtractMaxContentRunes)
}

// ExtractInvoicesFromContent calls the configured chat model and parses its
// strict-JSON reply into an InvoiceExtractionResult. Callers decide how to
// persist the result.
func ExtractInvoicesFromContent(ctx context.Context, model chat.Chat, content string) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "invoice_extract", ""), []chat.Message{
		{Role: "system", Content: invoiceExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice fields: %w", err)
	}
	var parsed InvoiceExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		return nil, fmt.Errorf("parse invoice extraction response: %w", err)
	}
	return &parsed, nil
}

// ExtractInvoicePageFromContent asks the model to extract only the Nth invoice
// from the document text, keeping single-page re-extraction cheap (short
// output, only that invoice is replaced by the caller).
func ExtractInvoicePageFromContent(ctx context.Context, model chat.Chat, content string, page int) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	if page < 1 {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "invalid page number"}, nil
	}
	userPrompt := fmt.Sprintf(
		"以下文档中包含多张发票。请只提取文档中的第 %d 张发票（发票按文档中出现的先后顺序从 1 开始编号）。invoices 数组只包含这一张发票，字段规则不变；若无法确定第 %d 张发票，invoices 返回空数组。\n<document>\n%s\n</document>",
		page, page, strings.TrimSpace(content))
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "invoice_extract", ""), []chat.Message{
		{Role: "system", Content: invoiceExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice page: %w", err)
	}
	var parsed InvoiceExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		return nil, fmt.Errorf("parse invoice page response: %w", err)
	}
	return &parsed, nil
}

// InvoiceExtractionBatchSize caps how many text/OCR chunks go into one LLM
// call. Very long documents (dozens of invoices) can exceed a chat model's
// reliable single-call output, so extraction splits into batches and merges.
const InvoiceExtractionBatchSize = 10

// BuildInvoiceExtractionBatches assembles the document text into independent
// batches (each carries the document name/summary header), so long
// multi-invoice documents extract reliably across several LLM calls.
func BuildInvoiceExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []string {
	if batchSize <= 0 {
		batchSize = InvoiceExtractionBatchSize
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
		batches = append(batches, sampleLongContent(strings.Join(parts, "\n\n"), invoiceExtractMaxContentRunes))
	}
	if len(batches) == 0 {
		batches = []string{""}
	}
	return batches
}

// NormalizeInvoiceExtractionResult cleans up a model reply: it guarantees a
// known kind, drops empty items, normalizes the in-file dedup guarantee and
// assigns the 1-based page ordinal to every kept invoice.
func NormalizeInvoiceExtractionResult(res *InvoiceExtractionResult) {
	if res == nil {
		return
	}
	if res.Kind != "not_invoice" {
		res.Kind = "invoice"
	}
	if len(res.Invoices) == 0 {
		return
	}
	dedup := make(map[string]struct{}, len(res.Invoices))
	kept := res.Invoices[:0]
	page := 0
	for _, inv := range res.Invoices {
		key := inv.InvoiceNo
		if key != "" {
			if _, seen := dedup[key]; seen {
				continue // in-file duplicate: keep the first occurrence
			}
			dedup[key] = struct{}{}
		}
		// Invoice type is resolved by keyword priority (see
		// NormalizeInvoiceTypeFromTicket); empty stays empty (unrecognized).
		inv.InvoiceType = NormalizeInvoiceTypeFromTicket(inv.InvoiceType)
		// 列表税率列展示"金额最大项"的税率（Q2：多档税率在抽屉明细中逐项展示）。
		inv.TaxRate = dominantItemTaxRate(inv.Items)
		page++
		inv.Page = page
		kept = append(kept, inv)
	}
	res.Invoices = kept
}

// dominantItemTaxRate 返回金额最大（price×qty）项目明细的税率；无有效项时返回 nil。
func dominantItemTaxRate(items []InvoiceExtractionLine) *float64 {
	var best *float64
	var bestAmt float64
	for _, it := range items {
		if it.Price == nil || it.TaxRate == nil {
			continue
		}
		qty := 1.0
		if it.Qty != nil {
			qty = *it.Qty
		}
		amt := *it.Price * qty
		if amt > bestAmt {
			bestAmt = amt
			best = it.TaxRate
		}
	}
	return best
}

// NormalizeInvoiceTypeFromTicket maps a raw ticket/type string onto the fixed
// enum by keyword priority:
//
//	含"通行费" → 普通发票（通行费电子发票均为增值税普通发票）
//	含"专用" → 专用发票
//	含"医疗" → 医疗收据
//	含"财政" → 财政收据
//	含"普通" → 普通发票（增值税普通发票 / 电子发票（普通发票）等）
//	否则     → 其它票据
//
// A blank input stays blank (model could not recognize a type).
func NormalizeInvoiceTypeFromTicket(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "通行费"):
		return "普通发票"
	case strings.Contains(raw, "专用"):
		return "专用发票"
	case strings.Contains(raw, "医疗"):
		return "医疗收据"
	case strings.Contains(raw, "财政"):
		return "财政收据"
	case strings.Contains(raw, "普通"):
		return "普通发票"
	default:
		return "其它票据"
	}
}

// validInvoiceTypes is the fixed invoice type enum surfaced in the UI.
var validInvoiceTypes = map[string]struct{}{
	"专用发票": {},
	"普通发票": {},
	"医疗收据": {},
	"财政收据": {},
	"其它票据": {},
}

// InvoicesAllEmpty reports whether every invoice in the result is effectively
// blank (no identifier, date, type, amounts or parties). Used to guard against
// persisting a garbage/truncated model reply as a successful extraction.
func InvoicesAllEmpty(invs []InvoiceExtractionItem) bool {
	for _, inv := range invs {
		amt := inv.Amount != nil && *inv.Amount != 0
		total := inv.TotalAmount != nil && *inv.TotalAmount != 0
		if inv.InvoiceNo != "" || inv.InvoiceDate != "" || inv.InvoiceType != "" ||
			inv.SellerName != "" || inv.BuyerName != "" || amt || total {
			return false
		}
	}
	return true
}
