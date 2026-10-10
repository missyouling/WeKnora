package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
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
	InvoiceNo   string                  `json:"invoice_no"`
	InvoiceCode string                  `json:"invoice_code"`
	InvoiceDate string                  `json:"invoice_date"`
	InvoiceType string                  `json:"invoice_type"`
	TotalAmount *float64                `json:"total_amount"`
	Amount      *float64                `json:"amount"`
	Tax         *float64                `json:"tax"`
	TaxRate     *float64                `json:"tax_rate"`
	SellerName  string                  `json:"seller_name"`
	SellerTaxNo string                  `json:"seller_tax_no"`
	SellerAddr  string                  `json:"seller_address"`
	SellerPhone string                  `json:"seller_phone"`
	SellerBank  string                  `json:"seller_bank"`
	SellerAcct  string                  `json:"seller_account"`
	BuyerName   string                  `json:"buyer_name"`
	BuyerTaxNo  string                  `json:"buyer_tax_no"`
	BuyerAddr   string                  `json:"buyer_address"`
	BuyerPhone  string                  `json:"buyer_phone"`
	BuyerBank   string                  `json:"buyer_bank"`
	BuyerAcct   string                  `json:"buyer_account"`
	Issuer      string                  `json:"issuer"`
	Remark      string                  `json:"remark"`
	Category    string                  `json:"category"`
	Items       []InvoiceExtractionLine `json:"items"`
	VoidFlag    bool                    `json:"void_flag"`
	Duplicate   bool                    `json:"duplicate"`
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
	Kind          string                  `json:"kind"` // "invoice" | "not_invoice"
	Invoices      []InvoiceExtractionItem `json:"invoices"`
	ExtractError  string                  `json:"extract_error"`
	FleetCertType string                  `json:"fleet_cert_type,omitempty"` // 上传/重提取时的归档分类打标，供后续重提取选规则兜底
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
6. tax_rate（发票级）使用小数（如 3% 记为 0.03）；无法识别时返回 null。若发票包含多行明细且税率不同，发票级 tax_rate 取金额最大明细行的税率，其余税率在 items 各行分别保留。
7. invoice_date 使用 YYYY-MM-DD 格式；无法识别时返回空字符串。
8. 发票作废或冲红时，void_flag 设为 true。
9. items 为货物或应税劳务明细，无法识别时返回空数组。每行明细必须提取该行对应的税率 tax_rate（小数，如 3% 记为 0.03）；若多行明细税率不同，各明细行分别保留各自税率，不要合并、不要取平均；同一税率档只出现一次的行可各自保留。
10. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。
11. 销售方/购买方字段：seller_name/buyer_name 为名称，seller_tax_no/buyer_tax_no 为统一社会信用代码，seller_address/buyer_address 为注册地址，seller_phone/buyer_phone 为电话，seller_bank/buyer_bank 为开户行，seller_account/buyer_account 为银行账号；无法识别时返回空字符串。
12. issuer 为开票人，remark 为备注/备注栏内容；无法识别时返回空字符串。
13. invoice_type 只从以下枚举中选择：专用发票、普通发票、医疗收据、财政收据、其它票据；含「通行费」或「电子发票」的票据一律归为「普通发票」（通行费电子发票均为增值税普通发票）；无法识别时返回空字符串。
14. category 为发票大分类，只从以下枚举中选择：通行费、办公费、差旅费、餐饮费、通讯费、加油费、住宿费、材料费、设备购置费、服务费、广告费、培训费、会议费、租赁费、其它。根据发票的商品或劳务名称、销方类型判断最匹配的一个大类；无法判定时用「其它」。
15. 如果发票包含车牌号、通行日期等运输信息（如 ETC 通行费发票），将车牌号、通行日期起止以「车牌号：XX；通行日期起：XX；通行日期止：XX」格式写入 remark 字段（与已有备注内容用分号拼接）；无则保持原样。
16. 发票级 total_amount/amount/tax 为价税合计/金额/税额（税额为全部明细税额之和）；无法识别时返回 null。

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

// buildInvoiceRuleSystemPrompt 在默认发票提示词基础上注入沙盒整体规则
// （invoice scope 提取规则：高级模板 + 字段口径）。输出结构契约（kind/invoices）
// 由默认提示词兜底，规则仅用于增强字段识别准确性。
// ============ 规则字段名归一化 ============
//
// 用户可在字段配置中用任意中文名定义字段（如「收据编号」「入账日期」），
// 模型按这些中文名输出 JSON key；而 InvoiceExtractionItem 的 json tag 是
// 固定英文（invoice_no/invoice_date/...），直接 Unmarshal 会把中文 key 全部
// 丢弃，导致"提取结果字段为空"。normalizeRuleFieldKeys 在解析前把模型输出
// 中命中的中文 key 映射回标准英文字段（含 items 明细与收款方式/审核等
// 无标准位的字段并入 remark），确保自定义分类（收据等）也能正常落盘。

// invoiceFieldAliases 中文别名 → 标准英文字段。匹配取"最长的包含命中的别名"，
// 例如字段名「价税合计金额」同时包含「金额」与「价税合计」，取后者 → total_amount。
var invoiceFieldAliases = []struct{ alias, target string }{
	{"收据编号", "invoice_no"},
	{"发票号码", "invoice_no"},
	{"发票号", "invoice_no"},
	{"单据编号", "invoice_no"},
	{"单据号码", "invoice_no"},
	{"流水号", "invoice_no"},
	{"票据编号", "invoice_no"},
	{"票号", "invoice_no"},
	{"号码", "invoice_no"},
	{"编号", "invoice_no"},
	{"发票代码", "invoice_code"},
	{"票据代码", "invoice_code"},
	{"代码", "invoice_code"},
	{"入账日期", "invoice_date"},
	{"开票日期", "invoice_date"},
	{"开票时间", "invoice_date"},
	{"单据日期", "invoice_date"},
	{"日期", "invoice_date"},
	{"发票类型", "invoice_type"},
	{"票据类型", "invoice_type"},
	{"类型", "invoice_type"},
	{"价税合计金额", "total_amount"},
	{"价税合计", "total_amount"},
	{"价税总额", "total_amount"},
	{"总金额", "total_amount"},
	{"总额", "total_amount"},
	{"总价", "total_amount"},
	{"总计", "total_amount"},
	{"合计金额", "total_amount"},
	{"合计", "total_amount"},
	{"不含税金额", "amount"},
	{"金额", "amount"},
	{"小计", "amount"},
	{"合计税额", "tax"},
	{"税额", "tax"},
	{"税金", "tax"},
	{"税率", "tax_rate"},
	{"交款单位", "buyer_name"},
	{"购买方名称", "buyer_name"},
	{"购方名称", "buyer_name"},
	{"买方名称", "buyer_name"},
	{"付款单位", "buyer_name"},
	{"付款方", "buyer_name"},
	{"客户名称", "buyer_name"},
	{"单位名称", "buyer_name"},
	{"购买方税号", "buyer_tax_no"},
	{"购方税号", "buyer_tax_no"},
	{"买方税号", "buyer_tax_no"},
	{"纳税人识别号", "buyer_tax_no"},
	{"销售方名称", "seller_name"},
	{"销方名称", "seller_name"},
	{"收款单位", "seller_name"},
	{"收款方", "seller_name"},
	{"卖方名称", "seller_name"},
	{"开票方名称", "seller_name"},
	{"销售方税号", "seller_tax_no"},
	{"销方税号", "seller_tax_no"},
	{"收款方税号", "seller_tax_no"},
	{"开票人", "issuer"},
	{"收款人", "issuer"},
	{"经办", "issuer"},
	{"收款方式", "remark"},
	{"收款事由", "remark"},
	{"用途", "remark"},
	{"事由", "remark"},
	{"摘要", "remark"},
	{"备注", "remark"},
	{"审核", "remark"},
	{"出纳", "remark"},
	{"财务主管", "remark"},
}

// invoiceLineFieldAliases 明细行字段的中文别名。
var invoiceLineFieldAliases = []struct{ alias, target string }{
	{"货物名称", "name"},
	{"服务名称", "name"},
	{"项目名称", "name"},
	{"名称", "name"},
	{"项目", "name"},
	{"数量", "qty"},
	{"单价", "price"},
	{"价格", "price"},
	{"金额", "price"},
	{"税率", "tax_rate"},
}

// mapAliasKey 在别名表里找"包含命中且别名最长"的目标字段；找不到返回空串。
func mapAliasKey(key string, aliases []struct{ alias, target string }) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	best, bestLen := "", 0
	for _, a := range aliases {
		if strings.Contains(key, a.alias) && len([]rune(a.alias)) > bestLen {
			best, bestLen = a.target, len([]rune(a.alias))
		}
	}
	return best
}

// normalizeRuleFieldKeys 把模型输出的 JSON 里命中规则字段名的中文 key 替换为
// 标准英文字段。只处理顶层 kind/invoices 与 invoices 元素、items 明细元素；
// 无法解析或无需替换时原样返回。
func normalizeRuleFieldKeys(raw string, cfg *types.KbExtractConfig) string {
	if cfg == nil || len(cfg.Fields) == 0 || strings.TrimSpace(raw) == "" {
		return raw
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return raw
	}

	normalizeMap := func(m map[string]interface{}) {
		type kv struct{ k, target string }
		// 全量清洗：模型常把"未找到"写成字符串 "null"，统一置空（不落盘）。
		for k, v := range m {
			if s, ok := v.(string); ok && strings.EqualFold(strings.TrimSpace(s), "null") {
				m[k] = ""
			}
		}
		var moves []kv
		for k := range m {
			if t := mapAliasKey(k, invoiceFieldAliases); t != "" {
				moves = append(moves, kv{k, t})
			}
		}
		for _, mv := range moves {
			v := m[mv.k]
			// 无标准位的字段（收款方式/审核/出纳等）并入 remark，保留数据；
			// 值为空（模型未识别/清洗后为空）时跳过，避免出现"审核：；"。
			if mv.target == "remark" {
				if v == nil {
					delete(m, mv.k)
					continue
				}
				if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
					delete(m, mv.k)
					continue
				}
				line := fmt.Sprintf("%s：%v", mv.k, v)
				if cur, ok := m["remark"].(string); ok && strings.TrimSpace(cur) != "" {
					if !strings.Contains(cur, line) {
						m["remark"] = strings.TrimSpace(cur) + "；" + line
					}
				} else if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					m["remark"] = line
				} else {
					m["remark"] = line
				}
			} else {
				m[mv.target] = v
			}
			delete(m, mv.k)
		}
		// items 明细元素的中文 key 归一化。
		if items, ok := m["items"].([]interface{}); ok {
			for _, it := range items {
				im, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				var lineMoves []kv
				for k := range im {
					if t := mapAliasKey(k, invoiceLineFieldAliases); t != "" {
						lineMoves = append(lineMoves, kv{k, t})
					}
				}
				for _, mv := range lineMoves {
					im[mv.target] = im[mv.k]
					delete(im, mv.k)
				}
			}
		}
	}

	if invoices, ok := root["invoices"].([]interface{}); ok {
		for _, inv := range invoices {
			im, ok := inv.(map[string]interface{})
			if !ok {
				continue
			}
			normalizeMap(im)
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return raw
	}
	return string(out)
}

// parseInvoiceRuleReply 在解析前先做规则字段名归一化，再反序列化为结果结构。
func parseInvoiceRuleReply(raw string, cfg *types.KbExtractConfig) (*InvoiceExtractionResult, error) {
	normalized := normalizeRuleFieldKeys(raw, cfg)
	var parsed InvoiceExtractionResult
	if err := common.ParseLLMJsonResponse(normalized, &parsed); err != nil {
		return nil, fmt.Errorf("parse invoice extraction response: %w", err)
	}
	return &parsed, nil
}

func buildInvoiceRuleSystemPrompt(cfg *types.KbExtractConfig) string {
	if cfg == nil {
		return invoiceExtractionSystemPrompt
	}
	hasTemplate := strings.TrimSpace(cfg.PromptTemplate) != ""
	hasFields := len(cfg.Fields) > 0
	if !hasTemplate && !hasFields {
		return invoiceExtractionSystemPrompt
	}
	var sb strings.Builder
	if hasTemplate {
		r := strings.NewReplacer("{{fields_schema}}", BuildFieldsSchema(cfg.Fields))
		sb.WriteString(strings.TrimSpace(r.Replace(cfg.PromptTemplate)))
		sb.WriteString("\n\n")
	}
	if hasFields {
		sb.WriteString("提取要点（用户配置的字段口径，请按这些要点识别发票内容；输出结构仍严格按下方规则执行）:\n")
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
	sb.WriteString(invoiceExtractionSystemPrompt)
	return sb.String()
}

// ExtractInvoicesFromContentWithRules 与 ExtractInvoicesFromContent 相同，但支持注入
// 沙盒发票整体规则（高级模板与字段口径），规则为空时行为与默认版一致。
func ExtractInvoicesFromContentWithRules(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	systemPrompt := buildInvoiceRuleSystemPrompt(cfg)
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "invoice_extract", ""), []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice fields: %w", err)
	}
	var parsed *InvoiceExtractionResult
	if parsed, err = parseInvoiceRuleReply(result.Content, cfg); err != nil {
		return nil, err
	}
	return parsed, nil
}

// ExtractInvoicesFromContentWithRulesGuided 与 ExtractInvoicesFromContentWithRules
// 相同，但允许注入一段提取引导（guide）。分批提取多发票合集时，模型可能因为
// 单批文本碎片化（模板噪音占比高、发票被跨 chunk 拆开）而把本批误判为
// not_invoice 导致整批发票丢失；guide 用于告知模型"本批是发票合集的一部分、
// 包含多张发票、必须全部提取"，显著降低误判率。
func ExtractInvoicesFromContentWithRulesGuided(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig, guide string) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	systemPrompt := buildInvoiceRuleSystemPrompt(cfg)
	userPrompt := "<document>\n" + content + "\n</document>"
	if strings.TrimSpace(guide) != "" {
		userPrompt = strings.TrimSpace(guide) + "\n\n" + userPrompt
	}
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "invoice_extract", ""), []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice fields: %w", err)
	}
	var parsed *InvoiceExtractionResult
	if parsed, err = parseInvoiceRuleReply(result.Content, cfg); err != nil {
		return nil, err
	}
	return parsed, nil
}

// ExtractInvoicePageFromContentWithRules 单张发票重提取，支持注入沙盒规则。
func ExtractInvoicePageFromContentWithRules(ctx context.Context, model chat.Chat, content string, page int, cfg *types.KbExtractConfig) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	if page < 1 {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "invalid page number"}, nil
	}
	systemPrompt := buildInvoiceRuleSystemPrompt(cfg)
	userPrompt := fmt.Sprintf(
		"以下文档中包含多张发票。请只提取文档中的第 %d 张发票（发票按文档中出现的先后顺序从 1 开始编号）。invoices 数组只包含这一张发票，字段规则不变；若无法确定第 %d 张发票，invoices 返回空数组。\n<document>\n%s\n</document>",
		page, page, strings.TrimSpace(content))
	thinking := false
	result, err := model.Chat(types.WithLLMCallMetadata(ctx, "invoice_extract", ""), []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 8192, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice page: %w", err)
	}
	var parsed *InvoiceExtractionResult
	if parsed, err = parseInvoiceRuleReply(result.Content, cfg); err != nil {
		return nil, err
	}
	return parsed, nil
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
// call as a hard safety net. Batching is driven primarily by the rune limit
// (invoiceExtractMaxContentRunes): a mid-size multi-invoice document stays in
// one or two calls so the model sees the full document context, while very
// long documents (dozens of invoices) still split reliably. Before the rune-
// driven change, a 30-chunk / 18-invoice toll invoice PDF was cut into many
// tiny batches (each seeing only 1-2 invoices) and the model dropped most.
const InvoiceExtractionBatchSize = 64

// BuildInvoiceExtractionBatches assembles the document text into independent
// batches (each carries the document name/summary header), so long
// multi-invoice documents extract reliably across several LLM calls. A batch
// is flushed when its accumulated content exceeds the rune cap (or when the
// chunk-count hard cap is hit), never earlier — this keeps related invoice
// fragments in the same call.
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
	curParts := make([]string, 0, 64)
	curRunes := 0
	flush := func() {
		if len(curParts) == 0 {
			return
		}
		parts := make([]string, 0, len(curParts)+1)
		if headStr != "" {
			parts = append(parts, headStr)
		}
		parts = append(parts, curParts...)
		batches = append(batches, sampleLongContent(strings.Join(parts, "\n\n"), invoiceExtractMaxContentRunes))
		curParts = curParts[:0]
		curRunes = 0
	}
	for _, c := range usable {
		part := strings.TrimSpace(c.Content)
		if part == "" {
			continue
		}
		r := len([]rune(part))
		// 主约束：累积 runes 超限才切批；chunk 数仅作极端兜底。只要单批内容
		// 仍在模型可靠处理范围内就尽量整批，避免发票被拆散导致漏提。
		if (curRunes+r > invoiceExtractMaxContentRunes || len(curParts) >= batchSize) && curRunes > 0 {
			flush()
		}
		curParts = append(curParts, part)
		curRunes += r
	}
	flush()
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

// ============ 规则提取（系统生成的规整电子票据） ============
//
// 通行费电子票据汇总单（PDF）由固定模板渲染，文本层格式完全规整：每张发票
// 由「号码行」（发票号 日期 购买方税号 销售方名称 销售方税号 *项目*通行费
// 车牌 车型 通行日期起 金额 税率 税额）与「金额行」（¥金额 大写 开票人
// ¥价税合计 通行日期止 购买方名称 ¥税额）两行构成。模型在这种高模板噪音
// 长文本中常漏提（实测 18 张只提取 1~8 张），而正则规则可 100% 全量命中，
// 因此提取链路优先走规则、未命中才回退模型。

var (
	// invoiceHeadLineRe 匹配发票「号码行」。
	invoiceHeadLineRe = regexp.MustCompile(
		`^(\d{20})\s+(\d{4})年(\d{1,2})月(\d{1,2})日\s+([0-9A-Z]{15,20})\s+(.+?)([0-9A-Z]{15,20})\s+\*([^*]*)\*通行费\s+(\S+)\s+(\S+)\s+(\d{8})\s+([\d.]+)\s+(\d+(?:\.\d+)?%)\s+([\d.]+)\s*$`)
	// invoiceAmountLineRe 匹配发票「金额行」。
	invoiceAmountLineRe = regexp.MustCompile(
		`^¥([\d.]+)\s+(.+?)\s+(\S+)\s+¥([\d.]+)\s+(\d{8})\s+(.+?)\s+¥([\d.]+)\s*$`)
)

// cleanInvoiceTemplateNoise 逐行清洗电子票据文本中的模板噪音，返回有效行。
// 噪音来源：IDE 本地路径水印（localhost:63342...）、markdown 图片引用、
// 空表单模板标签（购销方信息/项目列/价税合计占位等）。
func cleanInvoiceTemplateNoise(content string) []string {
	noiseLines := map[string]struct{}{
		"开票日期：": {}, "购": {}, "买": {}, "方": {}, "信": {}, "息": {}, "名称：": {},
		"统一社会信用代码/纳税人识别号：": {}, "销": {}, "售": {},
		"项目名称 车牌号 车辆类型 通行日期起 通行日期止 金额 税率/征收率 税额": {},
		"合 计": {}, "价税合计（大写） （小写）": {}, "备": {}, "注": {}, "开票人:": {},
	}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "localhost:63342") || strings.HasPrefix(line, "![") {
			continue
		}
		if _, noise := noiseLines[line]; noise {
			continue
		}
		out = append(out, line)
	}
	return out
}

// parsePercentToFloat 将 "3%" / "9%" 解析为 0.03 / 0.09；解析失败返回 nil。
func parsePercentToFloat(s string) *float64 {
	s = strings.TrimSuffix(strings.TrimSpace(s), "%")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	f = f / 100
	return &f
}

// parseAmountToFloat 解析金额字符串；解析失败返回 nil。
func parseAmountToFloat(s string) *float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return &f
}

// ExtractInvoicesByLineRules 按「号码行+金额行」固定格式从电子票据文本中
// 提取全部发票。返回 (result, true) 表示规则命中（invoices 非空）；返回
// (nil, false) 表示未命中该格式，调用方应回退模型提取。
func ExtractInvoicesByLineRules(content string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	lines := cleanInvoiceTemplateNoise(content)

	type headMatch struct {
		no, dateY, dateM, dateD                                                             string
		buyerTax, sellerName, sellerTax, item, plate, vehType, passStart, amount, rate, tax string
	}
	var heads []headMatch
	for _, ln := range lines {
		m := invoiceHeadLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		heads = append(heads, headMatch{
			no: m[1], dateY: m[2], dateM: m[3], dateD: m[4],
			buyerTax: m[5], sellerName: strings.TrimSpace(m[6]), sellerTax: m[7],
			item: strings.TrimSpace(m[8]), plate: m[9], vehType: m[10],
			passStart: m[11], amount: m[12], rate: m[13], tax: m[14],
		})
	}
	if len(heads) == 0 {
		return nil, false
	}

	type amtMatch struct {
		amount, issuer, total, passEnd, buyer, tax string
	}
	var amts []amtMatch
	for _, ln := range lines {
		m := invoiceAmountLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		amts = append(amts, amtMatch{amount: m[1], issuer: m[3], total: m[4], passEnd: m[5], buyer: strings.TrimSpace(m[6]), tax: m[7]})
	}

	res := &InvoiceExtractionResult{Kind: "invoice"}
	for i, h := range heads {
		inv := InvoiceExtractionItem{
			InvoiceNo:   h.no,
			InvoiceDate: fmt.Sprintf("%s-%s-%s", h.dateY, h.dateM, h.dateD),
			InvoiceType: "普通发票", // 通行费电子发票均为增值税普通发票
			SellerName:  h.sellerName,
			SellerTaxNo: h.sellerTax,
			BuyerTaxNo:  h.buyerTax,
			Category:    "通行费发票", // 与上传分类名一致，供列表「细分分类」筛选精准命中
			Amount:      parseAmountToFloat(h.amount),
			Tax:         parseAmountToFloat(h.tax),
			TaxRate:     parsePercentToFloat(h.rate),
			Remark:      fmt.Sprintf("车牌号：%s；通行日期起：%s", h.plate, h.passStart),
		}
		if it := strings.TrimSpace(h.item); it != "" {
			// 填充 price/tax_rate 供 Normalize 时 dominantItemTaxRate 计算列表税率列
			inv.Items = []InvoiceExtractionLine{{Name: it, Price: inv.Amount, TaxRate: inv.TaxRate}}
		}
		if i < len(amts) {
			a := amts[i]
			inv.Issuer = a.issuer
			inv.BuyerName = a.buyer
			inv.TotalAmount = parseAmountToFloat(a.total)
			if a.amount != "" {
				inv.Amount = parseAmountToFloat(a.amount)
			}
			if a.tax != "" {
				inv.Tax = parseAmountToFloat(a.tax)
			}
			if a.passEnd != "" {
				inv.Remark = fmt.Sprintf("车牌号：%s；通行日期起：%s；通行日期止：%s",
					h.plate, h.passStart, a.passEnd)
			}
		}
		res.Invoices = append(res.Invoices, inv)
	}
	return res, true
}

// ExtractInvoicesByLineRulesFromChunks 将 text/OCR chunks 按页序拼接后执行
// 规则提取。返回 (result, true) 表示规则命中；否则 (nil, false)。
func ExtractInvoicesByLineRulesFromChunks(chunks []*types.Chunk) (*InvoiceExtractionResult, bool) {
	var parts []string
	for _, c := range chunks {
		if c == nil || c.Content == "" {
			continue
		}
		if c.ChunkType != types.ChunkTypeText && c.ChunkType != types.ChunkTypeImageOCR {
			continue
		}
		parts = append(parts, c.Content)
	}
	if len(parts) == 0 {
		return nil, false
	}
	return ExtractInvoicesByLineRules(strings.Join(parts, "\n"))
}
