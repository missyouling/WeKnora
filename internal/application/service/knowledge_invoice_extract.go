package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// invoiceExtractMaxContentRunes bounds the document text sent to the
// extraction model so the prompt stays inside a normal context window
// even for long PDFs.
const invoiceExtractMaxContentRunes = 24000

// invoiceExtractCallTimeout 单次模型调用的硬超时。
// 2026-10-17 实测：供应商偶发挂起连接（无响应、无错误返回），调用方无限等待，
// 提取任务永久停在 processing（CPU 0%、日志静默无任何 attempt 记录），
// UI 表现为"进度 100% 但永不返回"。有了超时，挂起转为错误 → 走重试/失败路径
// → 状态最终落 failed，UI 不再悬死。
const invoiceExtractCallTimeout = 5 * time.Minute

// withExtractCallTimeout 给单次模型调用包上硬超时。
func withExtractCallTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, invoiceExtractCallTimeout)
}

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
	// Fields 自定义动态字段扩展桶：用户自定义分类里没有标准存储位的字段
	// （如通行费发票的「车牌号」），提取时从模型返回 JSON 的未知 key 收集而来，
	// 随记录持久化并经列表/详情透传给前端动态列渲染。
	Fields map[string]any `json:"fields,omitempty"`
	// Page is the 1-based page/ordinal of the invoice inside its source file.
	// It is assigned by the backend (in extraction-result order) so the
	// floating toolbar can target a single page for re-extraction; legacy
	// records without it keep 0.
	Page int `json:"page"`
}

// InvoiceExtractionLine is one goods/service line inside an invoice.
// 标准字段覆盖通用明细（name/qty/price/tax_rate）；分类特有的明细子列
// （如通行费 plate_no/vehicle_type/travel_from/travel_to，电费 spec/unit/
// amount/tax_amount）通过自定义 JSON 编解码收集进 Extra，随 items 行整体
// 持久化并透传前端动态列渲染。
type InvoiceExtractionLine struct {
	Name    string   `json:"name"`
	Spec    string   `json:"spec,omitempty"` // 规格型号（分类 items 子列键）
	Unit    string   `json:"unit,omitempty"`
	Qty     *float64 `json:"qty,omitempty"`
	Price   *float64 `json:"price,omitempty"`
	Amount  *float64 `json:"amount,omitempty"`
	TaxRate *float64 `json:"tax_rate,omitempty"`
	TaxAmt  *float64 `json:"tax_amount,omitempty"`
	// Extra 分类明细子列（业务语义 key，如 plate_no/vehicle_type/travel_from/
	// travel_to/spec 等）；JSON 序列化时与标准键合并输出，反序列化时收集未知键。
	Extra map[string]any `json:"-"`
}

// UnmarshalJSON 收集 items 行中不在标准键集合内的键（分类动态子列）到 Extra。
func (l *InvoiceExtractionLine) UnmarshalJSON(b []byte) error {
	type lineAlias InvoiceExtractionLine
	var base lineAlias
	if err := json.Unmarshal(b, &base); err != nil {
		return err
	}
	*l = InvoiceExtractionLine(base)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil
	}
	std := map[string]bool{
		"name": true, "spec": true, "unit": true, "qty": true, "price": true,
		"amount": true, "tax_rate": true, "tax_amount": true,
	}
	for k, v := range raw {
		if std[k] {
			continue
		}
		if l.Extra == nil {
			l.Extra = map[string]any{}
		}
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			l.Extra[k] = val
		}
	}
	return nil
}

// MarshalJSON 将标准键与 Extra 动态子列合并输出，保证分类明细子字段随行落库。
func (l InvoiceExtractionLine) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"name": l.Name,
	}
	if l.Spec != "" {
		m["spec"] = l.Spec
	}
	if l.Unit != "" {
		m["unit"] = l.Unit
	}
	if l.Qty != nil {
		m["qty"] = l.Qty
	}
	if l.Price != nil {
		m["price"] = l.Price
	}
	if l.Amount != nil {
		m["amount"] = l.Amount
	}
	if l.TaxRate != nil {
		m["tax_rate"] = l.TaxRate
	}
	if l.TaxAmt != nil {
		m["tax_amount"] = l.TaxAmt
	}
	for k, v := range l.Extra {
		if _, dup := m[k]; !dup {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// InvoiceExtractionResult is the strict-JSON shape the model must return.
type InvoiceExtractionResult struct {
	Kind          string                  `json:"kind"` // "invoice" | "not_invoice"
	Invoices      []InvoiceExtractionItem `json:"invoices"`
	ExtractError  string                  `json:"extract_error"`
	FleetCertType string                  `json:"fleet_cert_type,omitempty"` // 上传/重提取时的归档分类打标，供后续重提取选规则兜底
	// ExtractIssues 字段级校验问题（error/warn）。此前只在服务内部记日志，前端完全
	// 看不到质量问题（"提取成功但号码为空"就是这种）。随结果透出后，前端详情页与
	// 提取测试对话框可直接展示。
	ExtractIssues []ExtractFieldIssue `json:"extract_issues,omitempty"`
	// PatchedByRegex 本次由后置正则补齐的字段名（去重），前端据此标注"值来源=正则"。
	PatchedByRegex []string `json:"patched_by_regex,omitempty"`
	// RegexPatches 补洞明细（字段/取值/来源），用于定位"这个值到底哪来的"。
	RegexPatches []RegexFieldPatch `json:"regex_patches,omitempty"`
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
6. tax_rate（发票级）使用小数（如 3% 记为 0.03）；无法识别时返回 null。若发票包含多行明细且税率不同，发票级 tax_rate 取金额最大明细行（按票面「金额」列判定）的税率，其余税率在 items 各行分别保留。
7. invoice_date 使用 YYYY-MM-DD 格式；无法识别时返回空字符串。
8. 发票作废或冲红时，void_flag 设为 true。
9. items 为货物或应税劳务明细，无法识别时返回空数组。每行明细必须与票面明细表格的行一一对应、逐列对齐「项目名称、规格型号、单位、数量、单价、金额、税率、税额」八列：name 取项目名称，spec 只取票面「规格型号」列、该列为空时必须输出空字符串、严禁用单位列（unit）或其它列值填充，unit 取「单位」列，qty 取数量，price 取单价，amount 取票面「金额」列（不含税额），tax_amount 取「税额」列，tax_rate 取该行税率（小数，如 3% 记为 0.03）。严禁把税额填到金额、把行错位或把票面合计行当作明细行；可用勾稽关系自检：单价×数量≈金额、金额×税率≈税额，不一致时以票面各列原值为准。多行明细税率不同时，各行分别保留各自税率，不要合并、不要取平均。
10. 只返回严格的 JSON，不要包含任何其他文字、解释或 markdown 代码块标记。
11. 销售方/购买方字段：seller_name/buyer_name 为名称，seller_tax_no/buyer_tax_no 为统一社会信用代码，seller_address/buyer_address 为注册地址，seller_phone/buyer_phone 为电话，seller_bank/buyer_bank 为开户行，seller_account/buyer_account 为银行账号；无法识别时返回空字符串。
12. issuer 为开票人，remark 为备注/备注栏内容；无法识别时返回空字符串。
13. invoice_type 只从以下枚举中选择：专用发票、普通发票、医疗收据、财政收据、其它票据；含「通行费」或「电子发票」的票据一律归为「普通发票」（通行费电子发票均为增值税普通发票）；无法识别时返回空字符串。
14. category 为发票大分类，只从以下枚举中选择：通行费、办公费、差旅费、餐饮费、通讯费、加油费、住宿费、材料费、设备购置费、服务费、广告费、培训费、会议费、租赁费、其它。根据发票的商品或劳务名称、销方类型判断最匹配的一个大类；无法判定时用「其它」。
15. 如果发票包含车牌号、通行日期等运输信息（如 ETC 通行费发票），将车牌号、通行日期起止以「车牌号：XX；通行日期起：XX；通行日期止：XX」格式写入 remark 字段（与已有备注内容用分号拼接）；无则保持原样。
16. 发票级 total_amount/amount/tax 为价税合计/金额/税额（税额为全部明细税额之和）；无法识别时返回 null。

输出格式：
{"kind":"invoice","invoices":[{"invoice_no":"","invoice_code":"","invoice_date":"","invoice_type":"","total_amount":0,"amount":0,"tax":0,"tax_rate":0,"seller_name":"","seller_tax_no":"","seller_address":"","seller_phone":"","seller_bank":"","seller_account":"","buyer_name":"","buyer_tax_no":"","buyer_address":"","buyer_phone":"","buyer_bank":"","buyer_account":"","issuer":"","remark":"","category":"","void_flag":false,"duplicate":false,"items":[{"name":"","qty":1,"price":0,"amount":0,"tax_rate":0,"tax_amount":0}]}]}`

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
	callCtx, cancelCall := withExtractCallTimeout(types.WithLLMCallMetadata(ctx, "invoice_extract", ""))
	defer cancelCall()
	result, err := model.Chat(callCtx, []chat.Message{
		{Role: "system", Content: invoiceExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking})
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
	// 提取状态：分类配置里的字段名是 extractStatus，模型常漂移成 parseStatus
	// （实测 435 条真实数据中 20 条如此、extractStatus 反而 0 条），
	// 导致前端按配置键 extractStatus 取值时该列渲染为空。
	{"parseStatus", "extractStatus"},
	{"parse_status", "extractStatus"},
	{"解析状态", "extractStatus"},
	{"提取状态", "extractStatus"},
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

// invoiceStdFieldKeys 是 InvoiceExtractionItem 的全部标准 JSON 契约键。
// normalizeRuleFieldKeys 用它判定模型返回的未知 key（如自定义分类字段「车牌号」），
// 未知 key 收集进 fields 扩展桶，保证动态字段不丢、不误并入 remark。
var invoiceStdFieldKeys = map[string]bool{
	"invoice_no": true, "invoice_code": true, "invoice_date": true, "invoice_type": true,
	"total_amount": true, "amount": true, "tax": true, "tax_rate": true,
	"seller_name": true, "seller_tax_no": true, "seller_address": true, "seller_phone": true,
	"seller_bank": true, "seller_account": true,
	"buyer_name": true, "buyer_tax_no": true, "buyer_address": true, "buyer_phone": true,
	"buyer_bank": true, "buyer_account": true,
	"issuer": true, "remark": true, "category": true, "items": true,
	"void_flag": true, "duplicate": true, "page": true, "kind": true, "fields": true,
}

// normalizeRuleFieldKeys 把模型输出的 JSON 里命中规则字段名的中文 key 替换为
// 标准英文字段。只处理顶层 kind/invoices 与 invoices 元素、items 明细元素；
// 无法解析或无需替换时原样返回。
// cleanInvoiceNullStrings 把模型输出中字符串形式的 "null" 统一置空（递归到
// 对象与数组元素）。模型把"未找到"写成 "null" 是常见坏习惯，若不清洗会以
// 字符串形式落库并在前端展示为 null。解析失败时原样返回，交给上层处理。
func cleanInvoiceNullStrings(raw string) string {
	var root interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return raw
	}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch n := v.(type) {
		case map[string]interface{}:
			for k, val := range n {
				if s, ok := val.(string); ok && strings.EqualFold(strings.TrimSpace(s), "null") {
					n[k] = ""
					continue
				}
				walk(val)
			}
		case []interface{}:
			for _, it := range n {
				walk(it)
			}
		}
	}
	walk(root)
	out, err := json.Marshal(root)
	if err != nil {
		return raw
	}
	return string(out)
}

func normalizeRuleFieldKeys(raw string, cfg *types.KbExtractConfig) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	// 字符串 "null" 清洗是通用健壮性处理（模型常把"未找到"写成 "null"），
	// 与业务字段配置无关，必须无条件执行：否则未配置提取字段的知识库会把
	// "null" 当作真实值落盘，前端列表直接显示 null。
	raw = cleanInvoiceNullStrings(raw)
	if cfg == nil || len(cfg.Fields) == 0 {
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
		// 自定义动态字段收集（如通行费发票「车牌号」）：别名映射后仍不在标准契约键
		// 集合内的 key（含值为空串、非标量）归入 fields 扩展桶，随记录持久化。
		// 与 remark 并入策略不同：动态字段有独立列/编辑位，不能混进备注原文。
		for k, v := range m {
			if k == "fields" || k == "kind" || invoiceStdFieldKeys[k] {
				continue
			}
			fm, ok := m["fields"].(map[string]interface{})
			if !ok || fm == nil {
				fm = map[string]interface{}{}
				m["fields"] = fm
			}
			fm[k] = v
			delete(m, k)
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
		// 动态字段分离指令：关键业务信息（车牌号/通行日期等）常藏在备注中，
		// 必须按字段口径分离到对应的独立输出字段，而不是只汇总在备注里。
		sb.WriteString("分离要求：如果票面包含备注信息，请仔细分析备注内容，并按照上面的字段口径，将隐藏在备注中的关键数据（如车牌号、通行日期等）分离提取到对应的独立字段中输出；不要把这些数据只汇总在备注字段里。\n\n")
		// 明细行子列口径：分类字段配置里 items 字段的 SubFields（单一权威源，
		// 由 handler 从 categories.subs 注入），通行费/电费等分类的明细行因此
		// 按票面真实结构输出，而不是固定 name/qty/price/tax_rate 四键。
		if ins := invoiceItemsSubFieldsInstruction(cfg); ins != "" {
			sb.WriteString(ins + "\n\n")
		}
	}
	// 号码口径（通用防错）：票面上往往并排多个数字串（发票号码/机器编号/校验码/
	// 发票代码/密码区密文），模型很容易取错。实测 16 页通行费票据时，模型把
	// 『机器编号 499098302394』当成了 invoice_no，而真正的『发票号码 64869484』
	// 就在同一页——用户直接反馈"发票号码不正确"。
	sb.WriteString("号码口径（必须严格遵守）：invoice_no 只能取票面明确标注『发票号码』（含『发票号码:』『发票号』『No.』等写法）标签之后的数字串；" +
		"票面上的『机器编号』『校验码』『发票代码』『密码区』密文，以及任何没有明确标签的长数字串，一律不得写入 invoice_no。" +
		"若该张票面确实没有『发票号码』标签，invoice_no 输出空字符串，不要用其它数字顶替。\n\n")
	sb.WriteString(invoiceExtractionSystemPrompt)
	return sb.String()
}

// invoiceItemsSubFieldsInstruction 根据 cfg.Fields 中 items 字段的 SubFields
// 生成明细行提取指令：要求模型 items 每行按分类子列 key 输出（通行费/电费等
// 分类的票面明细结构不同），分类特有字段（车牌号/通行日期等）务必提取到
// 明细行对应键，而不是只写入 remark。无 items 子列口径时返回空串（保持
// 默认 name/qty/price/tax_rate 四键行为）。
func invoiceItemsSubFieldsInstruction(cfg *types.KbExtractConfig) string {
	if cfg == nil {
		return ""
	}
	var keys []string
	var labels []string
	for _, f := range cfg.Fields {
		if !f.Enabled || strings.TrimSpace(f.Name) == "" || f.Type != "items" {
			continue
		}
		for _, sf := range f.SubFields {
			k := strings.TrimSpace(sf.Key)
			if k == "" {
				continue
			}
			keys = append(keys, k)
			lab := strings.TrimSpace(sf.Label)
			if lab == "" {
				lab = k
			}
			labels = append(labels, lab)
		}
	}
	if len(keys) == 0 {
		return ""
	}
	// 举例键一律取自配置的 SubFields.Key，不再写死 plate_no/travel_from/travel_to：
	// 那些是通行费分类的历史硬编码，对其它分类（水电/合同等）是无效甚至误导的键名。
	example := keys[0]
	if len(keys) > 1 {
		example = strings.Join(keys[:min(len(keys), 3)], "/")
	}
	return fmt.Sprintf(
		"额外明细行规则：本分类发票的 items 明细行必须按以下子列键提取（键名严格使用）：%s（含义依次为：%s）。"+
			"每行明细都输出这些键；票面存在但无法识别的键输出 null 或空字符串。若票面明细含分类特有信息"+
			"（如本分类的 %s），必须提取到 items 行对应的键，不要只写入 remark。",
		strings.Join(keys, "/"), strings.Join(labels, "/"), example)
}

// ExtractInvoicesFromContentWithRules 与 ExtractInvoicesFromContent 相同，但支持注入
// 沙盒发票整体规则（高级模板与字段口径），规则为空时行为与默认版一致。
// 走默认 4 次尝试序列（流式 → 非流式 → 非流式+schema → 流式重试）。
func ExtractInvoicesFromContentWithRules(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig) (*InvoiceExtractionResult, error) {
	return extractInvoicesWithRulesAttempts(ctx, model, content, cfg, nil)
}

// ExtractInvoicesWithRulesOnce 与 WithRules 相同，但**只尝试一次**模型调用。
//
// 专供分批提取的回退路径使用：Guided 调用失败时往往已经吃满一次长超时
// （实测 context deadline exceeded），若回退再叠加默认 4 次尝试，单批最坏耗时
// = 5（Guided 超时）+ 4×5（回退重试）= 25 分钟，批次串行后总时长可达 1~2 小时，
// 用户侧表现为"进度 100% 却迟迟不出结果"。回退只求"再试一把"。
func ExtractInvoicesWithRulesOnce(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig) (*InvoiceExtractionResult, error) {
	return extractInvoicesWithRulesAttempts(ctx, model, content, cfg, SingleAttemptExtractAttempts())
}

// extractInvoicesWithRulesAttempts 是上面两个入口的共同实现。
// attempts 为 nil 时使用默认 4 次尝试序列。
func extractInvoicesWithRulesAttempts(ctx context.Context, model chat.Chat, content string, cfg *types.KbExtractConfig, attempts []ExtractAttempt) (*InvoiceExtractionResult, error) {
	if strings.TrimSpace(content) == "" {
		return &InvoiceExtractionResult{Kind: "not_invoice", ExtractError: "no text content to extract"}, nil
	}
	systemPrompt := buildInvoiceRuleSystemPrompt(cfg)
	userPrompt := "<document>\n" + content + "\n</document>"
	thinking := false
	messages := []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	baseOpts := &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking}

	// 由用户配置的字段生成 JSON Schema，作为第 3 次尝试的强约束输出格式。
	// 无字段配置时 schema 为空，该次尝试退化为普通非流式调用（与改动前一致）。
	var schema json.RawMessage
	if cfg != nil && len(cfg.Fields) > 0 {
		if s := BuildFieldsSchema(cfg.Fields); s != "" {
			schema = json.RawMessage(s)
		}
	}

	if attempts == nil {
		attempts = DefaultExtractAttempts(schema)
	}

	// 走通用执行器：流式 → 非流式 → 非流式+schema 强约束 → 流式重试。
	// 之前是单次调用、解析失败即抛错，是"提取不出来"的主要来源；现在最多 4 次机会，
	// 且每步产出都先校验，成功即返回，不浪费后续 token。
	raw, err := RunExtractWithRetry(
		types.WithLLMCallMetadata(ctx, "invoice_extract", ""),
		model, messages, baseOpts, attempts,
		func(raw string) bool {
			_, perr := parseInvoiceRuleReply(raw, cfg)
			return perr == nil
		},
		func(lastRaw string, lastErr error) { logInvoiceReplyUnparsable(ctx, lastRaw, lastErr) },
	)
	if err != nil {
		return nil, fmt.Errorf("extract invoice fields: %w", err)
	}
	parsed, perr := parseInvoiceRuleReply(raw, cfg)
	if perr != nil {
		// 理论上不会走到这里（重试阶段已校验通过），保留兜底防止解析逻辑有副作用差异。
		logInvoiceReplyUnparsable(ctx, raw, perr)
		return nil, perr
	}

	// 质量闭环：确定性校验 → 有 error 级问题就带着问题清单自愈一次 → 择优保留。
	// 关键取舍：自愈结果只有在 error 级问题**严格减少**时才替换，绝不因为"再问了一次"
	// 就丢掉原本可用的数据——空结果比有瑕疵的结果更糟。
	issues := validateInvoiceExtraction(parsed, cfg)
	if countIssues(issues, issueSeverityError) > 0 {
		fixed, fixedIssues, rerr := repairInvoiceExtraction(
			types.WithLLMCallMetadata(ctx, "invoice_extract_repair", ""),
			model, messages, baseOpts, schema, cfg, raw, issues)
		switch {
		case rerr != nil:
			logger.Warnf(ctx, "invoice repair failed: %v", rerr)
		case countIssues(fixedIssues, issueSeverityError) < countIssues(issues, issueSeverityError):
			parsed, issues = fixed, fixedIssues
		default:
			logger.Warnf(ctx, "invoice repair did not reduce errors (%d → %d), keeping original",
				countIssues(issues, issueSeverityError), countIssues(fixedIssues, issueSeverityError))
		}
	}
	// 收尾：后置字段级正则补洞 → 问题清单透出（供前端展示）。
	// 补洞可能消掉 error（如号码原本为空），所以补完再重新校验一次，保证清单准确。
	parsed.ExtractIssues = issues
	if patches := RegexFallbackInvoiceFields(parsed, content, cfg); len(patches) > 0 {
		logger.Infof(ctx, "invoice regex fallback patched %d field(s): %v", len(patches), patchFieldNames(patches))
		parsed.ExtractIssues = validateInvoiceExtraction(parsed, cfg)
	}
	logExtractIssues(ctx, parsed.ExtractIssues)
	return parsed, nil
}

// patchFieldNames 提取补洞记录里的字段名（日志用）。
func patchFieldNames(patches []RegexFieldPatch) []string {
	out := make([]string, 0, len(patches))
	for _, p := range patches {
		out = append(out, p.Field)
	}
	return out
}

// logInvoiceReplyUnparsable 记录模型返回原文诊断信息，用于定位
// "parse invoice extraction response: unexpected end of JSON input" 根因
// （输出截断 / 空内容 / 非 JSON 包装）。
func logInvoiceReplyUnparsable(ctx context.Context, raw string, err error) {
	snippet := raw
	if len(snippet) > 400 {
		snippet = snippet[:400]
	}
	logger.Errorf(ctx, "invoice rule reply unparsable: content_len=%d head=%q err=%v", len(raw), snippet, err)
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
	callCtx, cancelCall := withExtractCallTimeout(types.WithLLMCallMetadata(ctx, "invoice_extract", ""))
	defer cancelCall()
	result, err := model.Chat(callCtx, []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice fields: %w", err)
	}
	var parsed *InvoiceExtractionResult
	if parsed, err = parseInvoiceRuleReply(result.Content, cfg); err != nil {
		logInvoiceReplyUnparsable(ctx, result.Content, err)
		return nil, err
	}
	// 分批路径（Guided）同样做后置正则补洞：本函数只负责单批，作用域天然正确，
	// 不会跨批串号。补洞后再做**本批**校验——问题清单随批结果透出，由 handler
	// 偏移下标后合并成整份结果的问题清单（否则真实的分批链路里永远看不到告警）。
	if patches := RegexFallbackInvoiceFields(parsed, content, cfg); len(patches) > 0 {
		logger.Infof(ctx, "invoice batch regex fallback patched %d field(s): %v", len(patches), patchFieldNames(patches))
	}
	parsed.ExtractIssues = validateInvoiceExtraction(parsed, cfg)
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
	callCtx, cancelCall := withExtractCallTimeout(types.WithLLMCallMetadata(ctx, "invoice_extract", ""))
	defer cancelCall()
	result, err := model.Chat(callCtx, []chat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking})
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
	callCtx, cancelCall := withExtractCallTimeout(types.WithLLMCallMetadata(ctx, "invoice_extract", ""))
	defer cancelCall()
	result, err := model.Chat(callCtx, []chat.Message{
		{Role: "system", Content: invoiceExtractionSystemPrompt},
		{Role: "user", Content: userPrompt},
	}, &chat.ChatOptions{Temperature: 0.1, MaxTokens: 0, Thinking: &thinking})
	if err != nil {
		return nil, fmt.Errorf("extract invoice page: %w", err)
	}
	var parsed InvoiceExtractionResult
	if err := common.ParseLLMJsonResponse(result.Content, &parsed); err != nil {
		return nil, fmt.Errorf("parse invoice page response: %w", err)
	}
	parsed.ExtractIssues = validateInvoiceExtraction(&parsed, nil)
	return &parsed, nil
}

// InvoiceExtractionBatchSize 每批送入模型的 chunk 数上限。
//
// **关键前提：chunk ≠ 页**（2026-10-17 实测）。同一份 16 页通行费 PDF 被解析成
// **49 个 chunk**（约每页 3 个，票据排版碎片化）。因此按"页"估算批次大小必须除以约 3。
//
// 取 8 的依据（多轮实测收敛，约束其实是**输出长度**而非输入长度）：
//   - 太小的历史值 5（误以为 chunk≈页）→ 49 chunks 切成 10 批、每批仅 1.6 页，
//     总时长翻倍且单批内容过少导致漏提（16 张只出 6 张）。
//   - 过大（历史 64）→ 整篇 1 批，单批输出超长，供应商直接返回空 content。
//   - 12 / 16 / 24 chunk 在 items 子列（8 列）开启后均触到 5 分钟单批硬超时
//     ——每张票的输出体积随子列翻倍，瓶颈在输出而非输入。
//   - 8 chunk（约 4~5 张票）可稳定完成；配合并发 4，16 页文档约 3~4 分钟。
//
// 注：扁平/HTML 规则提取器命中时根本不走 LLM，本常量只影响兜底路径。
const InvoiceExtractionBatchSize = 8

// InvoiceExtractionBatch 一批待提取文本，及其在原文档中的位置。
// 记录 chunk 索引区间是为了**结果页码归位**：分批提取必须按批次索引与原始位置
// 严格归位，绝不能用无序并发 append（历史上出现过金额"张冠李戴"的错位事故）。
type InvoiceExtractionBatch struct {
	Content    string // 已拼装头部、可直接送模型的批文本
	StartIndex int    // 本批首个 chunk 的 ChunkIndex（0-based）
	EndIndex   int    // 本批最后一个 chunk 的 ChunkIndex
}

// BuildInvoiceExtractionBatches assembles the document text into independent
// batches (each carries the document name/summary header), so long
// multi-invoice documents extract reliably across several LLM calls. A batch
// is flushed when its accumulated content exceeds the rune cap (or when the
// chunk-count hard cap is hit), never earlier — this keeps related invoice
// fragments in the same call.
func BuildInvoiceExtractionBatches(name, summary string, chunks []*types.Chunk, batchSize int) []InvoiceExtractionBatch {
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
	// 调用方未显式指定时用每批上限（12 个 chunk ≈ 4 页）。
	// 注意 chunk ≠ 页，见 InvoiceExtractionBatchSize 的说明。
	if batchSize <= 0 {
		batchSize = InvoiceExtractionBatchSize
	}
	header := make([]string, 0, 2)
	if name = strings.TrimSpace(name); name != "" {
		header = append(header, "Document name: "+name)
	}
	if summary = strings.TrimSpace(summary); summary != "" {
		header = append(header, "Existing summary: "+summary)
	}
	headStr := strings.Join(header, "\n\n")

	var batches []InvoiceExtractionBatch
	curParts := make([]string, 0, 64)
	curRunes := 0
	curStart, curEnd := -1, -1
	flush := func() {
		if len(curParts) == 0 {
			return
		}
		parts := make([]string, 0, len(curParts)+1)
		if headStr != "" {
			parts = append(parts, headStr)
		}
		parts = append(parts, curParts...)
		batches = append(batches, InvoiceExtractionBatch{
			Content:    sampleLongContent(strings.Join(parts, "\n\n"), invoiceExtractMaxContentRunes),
			StartIndex: curStart,
			EndIndex:   curEnd,
		})
		curParts = curParts[:0]
		curRunes = 0
		curStart, curEnd = -1, -1
	}
	for _, c := range usable {
		part := strings.TrimSpace(c.Content)
		if part == "" {
			continue
		}
		r := len([]rune(part))
		// 主约束：chunk 数（≈页数）达上限即切批，控制单批发票张数与输出长度；
		// rune 上限保留为兜底，防止个别超长 chunk 撑爆单批。
		// 不按"越多越好"整批，是因为单批发票过多会让模型输出超长 → 供应商返回空 content。
		if curRunes > 0 && (len(curParts) >= batchSize || curRunes+r > invoiceExtractMaxContentRunes) {
			flush()
		}
		if curStart < 0 {
			curStart = c.ChunkIndex
		}
		curEnd = c.ChunkIndex
		curParts = append(curParts, part)
		curRunes += r
	}
	flush()
	if len(batches) == 0 {
		batches = []InvoiceExtractionBatch{{Content: "", StartIndex: 0, EndIndex: 0}}
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
		// 规格型号兜底清洗：票面规格列为空时模型常把单位列误填到 spec，
		// 此时 spec 与 unit 文本相同，确定性清空 spec（spec≠unit 时保留）。
		cleanInvoiceItemSpec(inv.Items)
		// 列表税率列展示"金额最大项"的税率（Q2：多档税率在抽屉明细中逐项展示）。
		inv.TaxRate = dominantItemTaxRate(inv.Items)
		page++
		inv.Page = page
		kept = append(kept, inv)
	}
	res.Invoices = kept
}

// cleanInvoiceItemSpec 清理明细行规格型号：规格型号（spec）与单位（unit）
// 文本相同时，几乎必然是模型把单位误填到规格列（票面规格为空），此时清空
// spec；spec 与 unit 不同或其一为空时保持原值。
func cleanInvoiceItemSpec(items []InvoiceExtractionLine) {
	for i := range items {
		spec := strings.TrimSpace(items[i].Spec)
		unit := strings.TrimSpace(items[i].Unit)
		if spec != "" && unit != "" && spec == unit {
			items[i].Spec = ""
		}
	}
}

// dominantItemTaxRate 返回金额最大（price×qty）项目明细的税率；无有效项时返回 nil。
func dominantItemTaxRate(items []InvoiceExtractionLine) *float64 {
	var best *float64
	var bestAmt float64
	for _, it := range items {
		if it.TaxRate == nil {
			continue
		}
		amt := 0.0
		if it.Amount != nil {
			amt = *it.Amount
		} else if it.Price != nil {
			qty := 1.0
			if it.Qty != nil {
				qty = *it.Qty
			}
			amt = *it.Price * qty
		}
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
	// invoiceCompactLineRe 匹配 docreader 新输出的「单行紧凑」格式：号码段与
	// 金额段合并为同一行（旧模板为两行，PDF 文本层按行锚定）。分组布局 =
	// 号码段 1..14（与 invoiceHeadLineRe 一致）+ 金额段 15..21
	// （15=¥金额 16=大写 17=开票人 18=¥价税合计 19=通行日期止 20=购买方名称 21=¥税额）。
	invoiceCompactLineRe = regexp.MustCompile(
		`^(\d{20})\s+(\d{4})年(\d{1,2})月(\d{1,2})日\s+([0-9A-Z]{15,20})\s+(.+?)([0-9A-Z]{15,20})\s+\*([^*]*)\*通行费\s+(\S+)\s+(\S+)\s+(\d{8})\s+([\d.]+)\s+(\d+(?:\.\d+)?%)\s+([\d.]+)\s+¥([\d.]+)\s+(.+?)\s+(\S+)\s+¥([\d.]+)\s+(\d{8})\s+(.+?)\s+¥([\d.]+)\s*$`)
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

// invoiceHeadStartRe 锚定号码行起始（20 位发票号 + 开票日期）。长销售方名称
// 会被 PDF 文本层折断到后续行，因此起始识别不要求整行完整，完整字段由
// ExtractInvoicesByLineRules 跨行合并后再用 invoiceHeadLineRe 解析。
var invoiceHeadStartRe = regexp.MustCompile(`^\d{20}\s+\d{4}年\d{1,2}月\d{1,2}日(?:\s|$)`)

// headJoinMaxLines 限制号码行跨行合并最多向下吞并的行数，防止失控吞掉后续发票。
const headJoinMaxLines = 6

// lineHead / lineAmt 携带物理行号，供「物理窗口 + 金额锚点」配对使用。
type lineHead struct {
	m         []string
	startLine int // 起始行（含）
	endLine   int // 结束行（含，跨行合并后可能大于 startLine）
}

type lineAmt struct {
	m        []string
	line     int
	consumed bool
}

// cjkSpaceRe 匹配两个中文字符之间的空白。PDF 文本层会在词内断行（如长公司名
// 「…公路运营管理一分公司」在「管」后折断），跨行合并时引入的空格对中文词内
// 断点属于伪空格，需要剔除；正常字段间空格两侧不会都是中文。
var cjkSpaceRe = regexp.MustCompile(`(\p{Han})\s+(\p{Han})`)

// joinCJKWords 消除中文字符之间因 PDF 断行产生的伪空格（循环处理连续断点）。
func joinCJKWords(s string) string {
	for {
		next := cjkSpaceRe.ReplaceAllString(s, "$1$2")
		if next == s {
			return s
		}
		s = next
	}
}

// buildInvoiceFromHead 用号码行捕获组构造发票（号码行本身已含金额/税率/税额/
// 车牌/车型/通行日期起/购销方税号），价税合计等金额行字段留待配对后补充。
// 明细行 items 携带完整通行费字段（plate_no/vehicle_type/travel_from/amount/
// tax_rate/tax_amount），供详情抽屉按分类子列自适应回显。
func buildInvoiceFromHead(m []string) InvoiceExtractionItem {
	// 日期补零统一为 YYYY-MM-DD（m[2]=年 m[3]=月 m[4]=日），避免 "2026-1-5" 导致字符串排序错序
	year, _ := strconv.Atoi(m[2])
	month, _ := strconv.Atoi(m[3])
	day, _ := strconv.Atoi(m[4])
	plate := strings.TrimSpace(m[9])
	vehicleType := strings.TrimSpace(m[10])
	passStart := strings.TrimSpace(m[11])
	amount := parseAmountToFloat(m[12])
	taxRate := parsePercentToFloat(m[13])
	tax := parseAmountToFloat(m[14])
	inv := InvoiceExtractionItem{
		InvoiceNo:   m[1],
		InvoiceDate: fmt.Sprintf("%04d-%02d-%02d", year, month, day),
		InvoiceType: "普通发票", // 通行费电子发票均为增值税普通发票
		SellerName:  joinCJKWords(strings.TrimSpace(m[6])),
		SellerTaxNo: m[7],
		BuyerTaxNo:  m[5],
		Category:    "通行费发票", // 与上传分类名一致，供列表「细分分类」筛选精准命中
		Amount:      amount,
		Tax:         tax,
		TaxRate:     taxRate,
		Remark:      fmt.Sprintf("车牌号：%s；通行日期起：%s", plate, passStart),
	}
	// 车牌号写入动态字段扩展桶：行规则路径不经过 normalizeRuleFieldKeys（那是 LLM
	// JSON 输出的清洗路径），这里与 LLM 路径的口径对齐——自定义分类字段（如通行费
	// 发票「车牌号」）以分类 subs 的中文名作为 key 落入 fields 桶，前端列表/编辑
	// 抽屉的动态列才能读到底层数据（否则车牌号只存在于 remark，独立列恒为空）。
	if plate != "" {
		inv.Fields = map[string]any{"车牌号": plate}
	}
	if it := strings.TrimSpace(m[8]); it != "" {
		line := InvoiceExtractionLine{Name: it, Amount: amount, TaxRate: taxRate, TaxAmt: tax}
		extra := map[string]any{}
		if plate != "" {
			extra["plate_no"] = plate
		}
		if vehicleType != "" {
			extra["vehicle_type"] = vehicleType
		}
		if passStart != "" {
			extra["travel_from"] = passStart
		}
		if len(extra) > 0 {
			line.Extra = extra
		}
		inv.Items = []InvoiceExtractionLine{line}
	}
	return inv
}

// fillInvoiceFromAmt 用金额行捕获组补充价税合计/购买方名称/开票人/通行日期止，
// 并以金额行的金额/税额为准（与号码行互相校验）。plate/passStart 来自号码行。
func fillInvoiceFromAmt(inv *InvoiceExtractionItem, m []string, plate, passStart string) {
	inv.Issuer = m[3]
	inv.BuyerName = strings.TrimSpace(m[6])
	inv.TotalAmount = parseAmountToFloat(m[4])
	if m[1] != "" {
		inv.Amount = parseAmountToFloat(m[1])
	}
	if m[7] != "" {
		inv.Tax = parseAmountToFloat(m[7])
	}
	passEnd := strings.TrimSpace(m[5])
	if passEnd != "" {
		inv.Remark = fmt.Sprintf("车牌号：%s；通行日期起：%s；通行日期止：%s", plate, passStart, passEnd)
		// 明细行补通行日期止（号码行构造时已有车牌/车型/通行起）
		if len(inv.Items) > 0 {
			if inv.Items[0].Extra == nil {
				inv.Items[0].Extra = map[string]any{}
			}
			inv.Items[0].Extra["travel_to"] = passEnd
		}
	}
	// 号码行缺失时（孤立金额行兜底路径不调用本函数，直接构造）plate 可能为空；
	// 兜底写入车牌号到 fields 桶，保持与 buildInvoiceFromHead 口径一致。
	if p := strings.TrimSpace(plate); p != "" {
		if inv.Fields == nil {
			inv.Fields = map[string]any{"车牌号": p}
		} else if _, ok := inv.Fields["车牌号"]; !ok {
			inv.Fields["车牌号"] = p
		}
	}
}

// ExtractInvoicesByLineRules 按「号码行+金额行」固定格式从电子票据文本中
// 提取全部发票。返回 (result, true) 表示规则命中（invoices 非空）；返回
// (nil, false) 表示未命中该格式，调用方应回退模型提取。
//
// 解析为流式两遍扫描，每张发票的数据物理隔离，杜绝串行错位：
//  1. 号码行支持跨行合并（PDF 文本层会在长销售方名称处断行，整行正则会漏页）；
//  2. 金额行按六元组去重（分块 overlap 会让同一金额行在相邻 chunk 各出现一次）；
//  3. 配对以「物理窗口内金额相等」为锚点，不再用两个独立数组的下标配对；
//  4. 有金额行无号码行时输出部分发票兜底，保证一张不漏。
func ExtractInvoicesByLineRules(content string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	lines := cleanInvoiceTemplateNoise(content)

	// 单行紧凑格式优先（docreader 新输出：号码段 + 金额段合并为同一行）。
	// 识别特征：行首 20 位发票号 + 开票日期 + 双税号 + *通行费 + 内嵌 ¥金额段。
	// 命中则 100% 确定性命中，无需 LLM 兜底；未命中再回退旧「两行式」逻辑，
	// 保证旧模板文件不受影响。
	var compactInvoices []InvoiceExtractionItem
	compactSeen := make(map[string]struct{})
	for _, ln := range lines {
		m := invoiceCompactLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		inv := buildInvoiceFromHead(m) // m[1..14] 布局与 invoiceHeadLineRe 一致
		// 金额段补充（15=¥金额 17=开票人 18=¥价税合计 19=通行日期止 20=购买方名称 21=¥税额）
		inv.Issuer = m[17]
		inv.BuyerName = strings.TrimSpace(m[20])
		inv.TotalAmount = parseAmountToFloat(m[18])
		if v := strings.TrimSpace(m[15]); v != "" {
			inv.Amount = parseAmountToFloat(v)
		}
		if v := strings.TrimSpace(m[21]); v != "" {
			inv.Tax = parseAmountToFloat(v)
		}
		if end := strings.TrimSpace(m[19]); end != "" {
			inv.Remark = fmt.Sprintf("车牌号：%s；通行日期起：%s；通行日期止：%s", m[9], m[11], end)
			// 明细行补通行日期止（号码段构造时已有车牌/车型/通行起）
			if len(inv.Items) > 0 {
				if inv.Items[0].Extra == nil {
					inv.Items[0].Extra = map[string]any{}
				}
				inv.Items[0].Extra["travel_to"] = end
			}
		}
		// chunk overlap 去重：发票号/车牌/通行起止/金额/税额六元组身份键。
		key := strings.Join([]string{m[1], m[9], m[11], m[19], m[12], m[14]}, "|")
		if _, dup := compactSeen[key]; dup {
			continue
		}
		compactSeen[key] = struct{}{}
		compactInvoices = append(compactInvoices, inv)
	}
	if len(compactInvoices) > 0 {
		return &InvoiceExtractionResult{Kind: "invoice", Invoices: compactInvoices}, true
	}

	var heads []lineHead
	var amts []lineAmt
	amtSeen := make(map[string]struct{})
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if invoiceHeadStartRe.MatchString(ln) {
			// 跨行合并：从起始行向下累积（最多 headJoinMaxLines 行），每合并
			// 一行就尝试完整匹配，成功后消费这些行。
			joined := ln
			m := invoiceHeadLineRe.FindStringSubmatch(joined)
			j := i
			for m == nil && j-i < headJoinMaxLines && j+1 < len(lines) {
				next := lines[j+1]
				// 不跨越下一张发票的号码行或金额行
				if invoiceHeadStartRe.MatchString(next) || invoiceAmountLineRe.MatchString(next) {
					break
				}
				j++
				joined += " " + strings.TrimSpace(next)
				m = invoiceHeadLineRe.FindStringSubmatch(joined)
			}
			if m != nil {
				heads = append(heads, lineHead{m: m, startLine: i, endLine: j})
				i = j
				continue
			}
			// 合并失败：起始行只是巧合的 20 位数字噪音，不消费后续行，继续扫描
		}
		if am := invoiceAmountLineRe.FindStringSubmatch(ln); am != nil {
			// 分块 overlap 去重：重叠产生的金额行完全相同，用金额/开票人/价税
			// 合计/通行日期止/购买方/税额六元组作为身份键。
			key := strings.Join([]string{am[1], am[3], am[4], am[5], am[6], am[7]}, "|")
			if _, dup := amtSeen[key]; !dup {
				amtSeen[key] = struct{}{}
				amts = append(amts, lineAmt{m: am, line: i})
			}
		}
	}
	if len(heads) == 0 {
		return nil, false
	}

	res := &InvoiceExtractionResult{Kind: "invoice"}
	for hi, h := range heads {
		inv := buildInvoiceFromHead(h.m)
		// 物理窗口：本发票号码行结束 ~ 下一张发票号码行起始之间。
		windowEnd := len(lines)
		if hi+1 < len(heads) {
			windowEnd = heads[hi+1].startLine
		}
		var matched *lineAmt
		for ai := range amts {
			a := &amts[ai]
			if a.consumed || a.line < h.endLine || a.line > windowEnd {
				continue
			}
			if a.m[1] == h.m[12] { // 金额锚点：金额行不含税金额 == 号码行金额
				matched = a
				break
			}
		}
		// 窗口内未命中（物理顺序异常的防御）→ 全局按金额锚点兜底
		if matched == nil {
			for ai := range amts {
				a := &amts[ai]
				if !a.consumed && a.m[1] == h.m[12] {
					matched = a
					break
				}
			}
		}
		if matched != nil {
			fillInvoiceFromAmt(&inv, matched.m, h.m[9], h.m[11])
			matched.consumed = true
		}
		res.Invoices = append(res.Invoices, inv)
	}
	// 孤立金额行兜底：号码行缺失/无法识别时，金额行仍代表一张真实发票，
	// 输出部分发票（金额/价税合计/购买方/开票人有值），绝不因号码行失配而漏页。
	for ai := range amts {
		a := &amts[ai]
		if a.consumed {
			continue
		}
		amount := parseAmountToFloat(a.m[1])
		tax := parseAmountToFloat(a.m[7])
		inv := InvoiceExtractionItem{
			InvoiceType: "普通发票",
			Category:    "通行费发票",
			Issuer:      a.m[3],
			BuyerName:   strings.TrimSpace(a.m[6]),
			Amount:      amount,
			TotalAmount: parseAmountToFloat(a.m[4]),
			Tax:         tax,
			Remark:      "通行日期止：" + a.m[5],
		}
		// 明细行补一条通行费占位（金额/税额有值），保证详情抽屉明细区有行可编辑回显。
		if amount != nil {
			inv.Items = []InvoiceExtractionLine{{Name: "通行费", Amount: amount, TaxAmt: tax}}
		}
		res.Invoices = append(res.Invoices, inv)
		a.consumed = true
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

// ============ 规则提取（Markdown 表格模板，invoice2.html 渲染） ============
//
// 新版 docreader 对电子票据汇总单按 HTML 模板（invoice2.html）渲染后，文本层
// 输出为 Markdown 表格（而非旧的纯文本「号码行 + ¥金额行」）：
//
//	发票号码：<20位> 通行费电子发票（普通发票） 开票日期：<日期>
//	购 销 买 名称：<买方> 售 名称：<卖方> 方 方
//	|||<买方税号>|||||<卖方税号>||
//	|项目名称|车牌号|车辆类型|通行日期起||通行日期止|金额|税率/征收率|税额|
//	|<项目>|<车牌>|<车型>|<通行起>||<通行止>|<金额>|<税率>|<税额>|
//
// 该模板下中文名称/大写金额常因 PDF 嵌入字体缺少 ToUnicode 而乱码，但税号、
// 车牌、通行日期、金额、税率、税额均为 ASCII，可 100% 确定性提取。

var (
	// mdTaxRowRe 匹配「税号行」：|||<买方税号>|||||<卖方税号>||
	mdTaxRowRe = regexp.MustCompile(
		`^\|\|\|([0-9A-Z]{15,20})\|\|\|\|\|([0-9A-Z]{15,20})\|\|\s*$`)
	// mdDataRowRe 匹配每张发票的表格「数据行」（表头行因通行起位置非 8 位
	// 数字而天然被排除）：
	// |项目|车牌|车型|通行起||通行止|金额|税率|税额|
	mdDataRowRe = regexp.MustCompile(
		`^\|([^|]*)\|([^|]*)\|([^|]*)\|(\d{8})\|\|(\d{8})\|([\d.]+)\|(\d+(?:\.\d+)?%)\|([\d.]+)\|\s*$`)
	// mdInvoiceNoRe 抓取「发票号码：<20位>」；汇总单通常仅首张渲染号码。
	mdInvoiceNoRe = regexp.MustCompile(`发票号码[：:]\s*(\d{20})`)
	// mdIssuerRe 抓取开票人（仅当不含乱码替换字符时采用）。
	mdIssuerRe = regexp.MustCompile(`开票人\s*[:：]\s*(\S+)`)
)

// mdDateFromCompact 将 8 位紧凑日期（20260902）转为 YYYY-MM-DD；非法返回空。
func mdDateFromCompact(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return ""
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return fmt.Sprintf("%s-%s-%s", s[:4], s[4:6], s[6:8])
}

// mdCleanText 去除 PDF 文本层乱码替换字符并裁剪；若结果含替换字符或为空，
// 返回空串，表示该字段在文本层不可用。
func mdCleanText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsRune(s, '\ufffd') {
		return ""
	}
	return s
}

// ExtractInvoicesByMarkdownTableRules 按 Markdown 表格模板从电子票据文本中
// 提取全部发票。税号行总在其数据行之前（可能跨 chunk：税号行在 chunk 末尾、
// 数据行在下一 chunk 开头），逐行扫描时以「最近一个税号行」配对其后第一个
// 数据行。返回 (result, true) 表示命中（数据行非空）；否则 (nil, false)。
//
// docHint 为来源 knowledge 短标识，用于为缺失发票号生成全局唯一占位，
// 避免 Normalize 按发票号去重时跨文件冲突。
func ExtractInvoicesByMarkdownTableRules(content, docHint string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	// 预收集全部发票号码（按出现顺序），通常仅首张有。
	var headNos []string
	for _, m := range mdInvoiceNoRe.FindAllStringSubmatch(content, -1) {
		headNos = append(headNos, m[1])
	}

	var (
		invoices  []InvoiceExtractionItem
		seen      = make(map[string]struct{})
		buyerTax  string
		sellerTax string
	)
	for _, raw := range strings.Split(content, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" {
			continue
		}
		if tx := mdTaxRowRe.FindStringSubmatch(ln); tx != nil {
			buyerTax = tx[1]
			sellerTax = tx[2]
			continue
		}
		d := mdDataRowRe.FindStringSubmatch(ln)
		if d == nil {
			continue
		}
		plate := mdCleanText(d[2])
		vehicleType := mdCleanText(d[3])
		passStart := d[4]
		passEnd := d[5]
		amount := parseAmountToFloat(d[6])
		taxRate := parsePercentToFloat(d[7])
		tax := parseAmountToFloat(d[8])
		// 身份键去重（防 chunk overlap 重复数据行）。
		ident := strings.Join([]string{plate, passStart, passEnd, d[6], d[8]}, "|")
		if _, dup := seen[ident]; dup {
			continue
		}
		seen[ident] = struct{}{}

		idx := len(invoices) + 1
		// 发票号：取预收集号码的第 idx 个，缺失则生成唯一占位。
		no := ""
		if idx <= len(headNos) {
			no = headNos[idx-1]
		}
		if no == "" {
			no = fmt.Sprintf("TMP-%s-%02d", docHint, idx)
		}
		// 开票日期在该模板下常乱码，用通行日期起兜底。
		date := mdDateFromCompact(passStart)
		// 价税合计 = 金额 + 税额（小写价税合计在文本层乱码）。
		var total *float64
		if amount != nil && tax != nil {
			t := *amount + *tax
			total = &t
		}
		// 开票人：仅在不含乱码时采用。
		issuer := ""
		if im := mdIssuerRe.FindStringSubmatch(ln); im != nil {
			issuer = mdCleanText(im[1])
		}
		remark := fmt.Sprintf("车牌号：%s；通行日期起：%s；通行日期止：%s", plate, passStart, passEnd)

		inv := InvoiceExtractionItem{
			InvoiceNo:   no,
			InvoiceDate: date,
			InvoiceType: "普通发票", // 通行费电子发票均为增值税普通发票
			Category:    "通行费发票",
			BuyerTaxNo:  buyerTax,
			SellerTaxNo: sellerTax,
			Amount:      amount,
			Tax:         tax,
			TaxRate:     taxRate,
			TotalAmount: total,
			Issuer:      issuer,
			Remark:      remark,
		}
		if plate != "" {
			inv.Fields = map[string]any{"车牌号": plate}
		}
		// 明细行携带完整通行费字段（项目名称/车牌号/车辆类型/通行日期起止/金额/税率/税额），
		// 供详情抽屉按分类子列自适应回显；车牌号同时保留 fields 桶兼容列表动态列。
		line := InvoiceExtractionLine{Name: "通行费", Amount: amount, TaxRate: taxRate, TaxAmt: tax}
		extra := map[string]any{}
		if plate != "" {
			extra["plate_no"] = plate
		}
		if vehicleType != "" {
			extra["vehicle_type"] = vehicleType
		}
		if passStart != "" {
			extra["travel_from"] = passStart
		}
		if passEnd != "" {
			extra["travel_to"] = passEnd
		}
		if len(extra) > 0 {
			line.Extra = extra
		}
		inv.Items = []InvoiceExtractionLine{line}
		invoices = append(invoices, inv)
	}

	if len(invoices) == 0 {
		return nil, false
	}
	return &InvoiceExtractionResult{Kind: "invoice", Invoices: invoices}, true
}

// ExtractInvoicesByMarkdownTableRulesFromChunks 将 text/OCR chunks 按页序
// 拼接后执行 Markdown 表格规则提取。
func ExtractInvoicesByMarkdownTableRulesFromChunks(chunks []*types.Chunk, docHint string) (*InvoiceExtractionResult, bool) {
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
	return ExtractInvoicesByMarkdownTableRules(strings.Join(parts, "\n"), docHint)
}

// ---- 扁平格式确定性覆盖层 ----
//
// 背景：LLM 对"票面上并排的多个数字串"判断不可靠——实测把『机器编号
// 499098302394』当成了 invoice_no，而真正的『发票号码 64869484』就在同页。
// 该层不改动 LLM 的语义字段（购销方/地址/开户行等），只用票面明确标签做
// 确定性覆盖：发票号码 + 明细数据行里的车牌号/车辆类型/通行日期起止。

var (
	// flatInvoiceNoLabelRe 只认明确标签，不含『机器编号』『校验码』。
	flatInvoiceNoLabelRe = regexp.MustCompile(`发票号码\s*[:：]\s*(\d{6,20})`)
	// flatItemRowRe 匹配明细数据行（docreader 输出的固定结构）：
	//   *经营租赁*通行费 渝C87567 货车 20251024 20251024 93.32 9% 8.40
	//   └项目名称        └车牌   └车型 └通行起  └通行止 └金额 └税率 └税额
	flatItemRowRe = regexp.MustCompile(
		`(\*[^\s]+\*\S*)\s+([京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼][A-Za-z0-9]{4,8})\s+(\S{1,8})\s+(\d{8})\s+(\d{8})\s+([\d.]+)\s+(\d+(?:\.\d+)?%)\s+([\d.]+)`)
)

// OverlayDeterministicInvoiceFields 用文本中的确定性信息覆盖 LLM 结果的易错字段：
//   - invoice_no：只取『发票号码』标签值，杜绝误取机器编号/校验码
//   - items 子列：车牌号 / 车辆类型 / 通行日期起止（票面数据行结构固定）
//
// 覆盖条件（保守）：文本识别出的张数与结果张数**完全相等**才按序覆盖；否则一律
// 不动——张数不一致时按序对齐必然错位，宁可让用户复核，也不能把 A 票的号码写到
// B 票上（历史上踩过"张冠李戴"）。返回覆盖的张数。
func OverlayDeterministicInvoiceFields(res *InvoiceExtractionResult, chunks []*types.Chunk) int {
	if res == nil || res.Kind == "not_invoice" || len(res.Invoices) == 0 {
		return 0
	}
	var sb strings.Builder
	for _, c := range chunks {
		if c == nil {
			continue
		}
		if c.ChunkType != types.ChunkTypeText &&
			c.ChunkType != types.ChunkTypeImageOCR &&
			c.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		sb.WriteString(c.Content)
		sb.WriteString("\n")
	}
	full := sb.String()

	nos := flatInvoiceNoLabelRe.FindAllStringSubmatch(full, -1)
	if len(nos) != len(res.Invoices) {
		return 0
	}
	rows := flatItemRowRe.FindAllStringSubmatch(full, -1)
	overlaid := 0
	for i := range res.Invoices {
		res.Invoices[i].InvoiceNo = strings.TrimSpace(nos[i][1])
		if i < len(rows) {
			applyFlatItemRow(&res.Invoices[i], rows[i])
		}
		overlaid++
	}
	return overlaid
}

// applyFlatItemRow 把数据行回填到明细行（m 见 flatItemRowRe 的分组）。
func applyFlatItemRow(inv *InvoiceExtractionItem, m []string) {
	if inv == nil || len(m) < 9 {
		return
	}
	if len(inv.Items) == 0 {
		inv.Items = []InvoiceExtractionLine{{}}
	}
	line := &inv.Items[0]
	if line.Extra == nil {
		line.Extra = map[string]any{}
	}
	if v := strings.TrimSpace(m[1]); v != "" {
		line.Name = v
	}
	line.Extra["plate_no"] = strings.TrimSpace(m[2])
	line.Extra["vehicle_type"] = strings.TrimSpace(m[3])
	if v := mdDateFromCompact(m[4]); v != "" {
		line.Extra["travel_from"] = v
	}
	if v := mdDateFromCompact(m[5]); v != "" {
		line.Extra["travel_to"] = v
	}
	if v := parseAmountToFloat(m[6]); v != nil {
		line.Amount = v
	}
	if v := parsePercentToFloat(m[7]); v != nil {
		line.TaxRate = v
	}
	if v := parseAmountToFloat(m[8]); v != nil {
		line.TaxAmt = v
	}
}

// flatSegmentAnchorRe 票段锚点：docreader 对每张票都会输出「发票代码: X」，
// 以此为界把一个 chunk 里的多张票切成独立段，避免跨票串号。
var flatSegmentAnchorRe = regexp.MustCompile(`发票代码\s*[:：]\s*(\d{10,12})`)

// flatDateLineRe 抓取「开票日期: 2025年11月04日」。
var flatDateLineRe = regexp.MustCompile(`开票日期\s*[:：]\s*(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日`)

// flatTitleRe 抓取票种标题（docreader 用 markdown 高亮包裹，如
// `# <u>重庆增值税电子普通发票</u>`）。
var flatTitleRe = regexp.MustCompile(`<u>\s*([^<]+?)\s*</u>`)

// ExtractInvoicesByFlatLayout 从 docreader 票据文本中**确定性**解析全部发票，
// 不依赖大模型。返回 (result, true) 表示命中该格式。
//
// 为什么需要它：LLM 在"每张票 8 个子列 × 多张"的输出量下极不稳定（实测 16 页
// 票据即使缩到 8 chunk/批仍会触发 5 分钟超时），且会把『机器编号』当成发票号。
// 该格式的票面结构固定（号码/代码/日期独立成行、明细行字段顺序固定），
// 正则可以 100% 命中，毫秒级完成。
//
// 命中判据：至少解析出 1 张且**每张都有明确发票号码**；号码缺失的段直接跳过
// （宁可漏也不猜——票面上并排的机器编号/校验码极易被误当号码）。
func ExtractInvoicesByFlatLayout(content string) (*InvoiceExtractionResult, bool) {
	if strings.TrimSpace(content) == "" {
		return nil, false
	}
	locs := flatSegmentAnchorRe.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		return nil, false
	}
	invs := make([]InvoiceExtractionItem, 0, len(locs))
	for i, loc := range locs {
		start := loc[0]
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		seg := content[start:end]
		inv, ok := parseFlatInvoiceSegment(seg)
		if !ok {
			continue
		}
		inv.Page = len(invs) + 1
		invs = append(invs, inv)
	}
	if len(invs) == 0 {
		return nil, false
	}
	return &InvoiceExtractionResult{Kind: "invoice", Invoices: invs}, true
}

// parseFlatInvoiceSegment 解析单张票段。返回 false 表示该段缺少发票号码（不可靠）。
func parseFlatInvoiceSegment(seg string) (InvoiceExtractionItem, bool) {
	var inv InvoiceExtractionItem
	if m := flatInvoiceNoLabelRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.InvoiceNo = strings.TrimSpace(m[1])
	}
	if inv.InvoiceNo == "" {
		return inv, false
	}
	if m := flatSegmentAnchorRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.InvoiceCode = strings.TrimSpace(m[1])
	}
	if m := flatDateLineRe.FindStringSubmatch(seg); len(m) > 3 {
		inv.InvoiceDate = mdDateFromCompact(fmt.Sprintf("%s%02d%02d",
			m[1], atoiOrZero(m[2]), atoiOrZero(m[3])))
	}
	if m := flatTitleRe.FindStringSubmatch(seg); len(m) > 1 {
		inv.InvoiceType = NormalizeInvoiceTypeFromTicket(strings.TrimSpace(m[1]))
	}
	// 明细行：一张通行费票通常只有一个通行区间。优先纯文本布局（docreader），
	// 其次 HTML 表格（OCR 引擎 paddleocr_vl_cloud 的输出格式）。
	if m := flatItemRowRe.FindStringSubmatch(seg); len(m) > 8 {
		applyFlatItemRow(&inv, m)
	} else if vals := htmlTableDataRow(seg); len(vals) > 0 {
		applyHTMLItemRow(&inv, vals)
	}
	if len(inv.Items) > 0 {
		line := inv.Items[0]
		if line.Amount != nil {
			inv.Amount = line.Amount
		}
		if line.TaxAmt != nil {
			inv.Tax = line.TaxAmt
		}
		if inv.Amount != nil && inv.Tax != nil {
			total := *inv.Amount + *inv.Tax
			inv.TotalAmount = &total
		}
	}
	// 价税合计以票面「(小写) ¥101.72」为准（比按明细相加更可靠，能覆盖汇总口径）。
	if m := flatHTMLTotalRe.FindStringSubmatch(seg); len(m) > 1 {
		if v := parseAmountToFloat(strings.ReplaceAll(m[1], ",", "")); v != nil {
			inv.TotalAmount = v
		}
	}
	// 购销方 / 开票人（HTML 单元格内的固定标签）
	applyFlatParties(&inv, seg)
	return inv, true
}

// atoiOrZero 解析小整数（年月日），非法返回 0。
func atoiOrZero(s string) int {
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ExtractInvoicesByFlatLayoutFromChunks 拼接文本类 chunk 后按扁平布局规则解析。
// chunk 边界可能切开单张票，但按顺序拼接后内容连续，按「发票代码」切段即可复原。
func ExtractInvoicesByFlatLayoutFromChunks(chunks []*types.Chunk) (*InvoiceExtractionResult, bool) {
	var sb strings.Builder
	for _, c := range chunks {
		if c == nil {
			continue
		}
		if c.ChunkType != types.ChunkTypeText &&
			c.ChunkType != types.ChunkTypeImageOCR &&
			c.ChunkType != types.ChunkTypeImageCaption {
			continue
		}
		sb.WriteString(c.Content)
		sb.WriteString("\n")
	}
	return ExtractInvoicesByFlatLayout(sb.String())
}

// ---- HTML 表格行解析（OCR 解析引擎的输出格式） ----
//
// paddleocr_vl_cloud 输出的票面是标准 HTML 表格：
//
//	<tr><td>…通行费</td><td>渝C87567</td><td>货车</td><td>20251024</td><td>20251024</td>
//	    <td>93.32</td><td>9%</td><td>8.40</td></tr>
//
// 与纯文本布局（docreader）字段顺序一致，但分隔符不同，需要单独解析。
var (
	flatHTMLTrRe = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	flatHTMLTdRe = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	// flatHTMLTotalRe 抓价税合计小写金额：<td colspan=7>…(小写) ¥101.72</td>
	flatHTMLTotalRe = regexp.MustCompile(`(?is)小写\D{0,4}([\d,]+(?:\.\d+)?)`)
)

// stripHTMLTags 去掉内联标签与实体，保留可见文本。
func stripHTMLTags(s string) string {
	s = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(s, "")
	s = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&", "&quot;", `"`, "&nbsp;", " ").Replace(s)
	return strings.TrimSpace(s)
}

// plateLike 判断是否像车牌（省份简称开头的 6~8 位）。
func plateLike(s string) bool {
	const prov = "京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼"
	r := []rune(s)
	if len(r) < 6 || len(r) > 9 {
		return false
	}
	return strings.ContainsRune(prov, r[0])
}

// isCompact8Date 判断 8 位紧凑日期。
func isCompact8Date(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// htmlTableDataRow 从票段中找出明细数据行（项目/车牌/车型/通行起/通行止/金额/税率/税额）。
func htmlTableDataRow(seg string) []string {
	for _, tr := range flatHTMLTrRe.FindAllStringSubmatch(seg, -1) {
		cells := flatHTMLTdRe.FindAllStringSubmatch(tr[1], -1)
		if len(cells) < 8 {
			continue
		}
		vals := make([]string, 0, len(cells))
		for _, c := range cells {
			vals = append(vals, stripHTMLTags(c[1]))
		}
		// 数据行特征：第 2 列像车牌、第 4/5 列是 8 位日期。
		if plateLike(vals[1]) && isCompact8Date(vals[3]) && isCompact8Date(vals[4]) {
			return vals
		}
	}
	return nil
}

// applyHTMLItemRow 把 HTML 表格数据行回填到明细行。
// vals = [项目, 车牌, 车型, 通行起, 通行止, 金额, 税率, 税额]
func applyHTMLItemRow(inv *InvoiceExtractionItem, vals []string) {
	if inv == nil || len(vals) < 8 {
		return
	}
	if len(inv.Items) == 0 {
		inv.Items = []InvoiceExtractionLine{{}}
	}
	line := &inv.Items[0]
	if line.Extra == nil {
		line.Extra = map[string]any{}
	}
	// 项目名里 OCR 会带入 ^ {{*}} 之类的标记，清洗为「*经营租赁*通行费」。
	name := strings.NewReplacer("^", "", "{{*}}", "*", " $", "", "$", "").Replace(vals[0])
	line.Name = strings.TrimSpace(name)
	line.Extra["plate_no"] = vals[1]
	line.Extra["vehicle_type"] = vals[2]
	if v := mdDateFromCompact(vals[3]); v != "" {
		line.Extra["travel_from"] = v
	}
	if v := mdDateFromCompact(vals[4]); v != "" {
		line.Extra["travel_to"] = v
	}
	if v := parseAmountToFloat(strings.TrimSpace(vals[5])); v != nil {
		line.Amount = v
	}
	if v := parsePercentToFloat(strings.TrimSpace(vals[6])); v != nil {
		line.TaxRate = v
	}
	if v := parseAmountToFloat(strings.TrimSpace(vals[7])); v != nil {
		line.TaxAmt = v
	}
}

// 购销方单元格：<td>购买方</td><td colspan=3>名 称：X\n纳税人识别号：Y\n地 址、电 话：Z\n开户行及账号：W</td>
var (
	flatPartyCellRe = regexp.MustCompile(`(?is)<td>\s*(购买方|销售方)\s*</td>\s*<td[^>]*>(.*?)</td>`)
	flatNameLikeRe  = regexp.MustCompile(`名\s*称\s*[:：]\s*([^\n\\]+)`)
	flatTaxNoLikeRe = regexp.MustCompile(`纳税人识别号\s*[:：]\s*([0-9A-Z]{15,20})`)
	flatAddrPhoneRe = regexp.MustCompile(`地\s*址、\s*电\s*话\s*[:：]\s*([^\n\\]+)`)
	flatBankAcctRe  = regexp.MustCompile(`开户行及账号\s*[:：]\s*([^\n\\]+)`)
	flatIssuerCell  = regexp.MustCompile(`开票人\s*[:：]\s*([^\s<\\]+)`)
	flatPhoneTailRe = regexp.MustCompile(`\s*((?:\d{3,4}-\d{7,8}(?:-\d{1,6})?)|(?:1\d{10}))\s*$`)
	flatAcctTailRe  = regexp.MustCompile(`\s*(\d{10,25})\s*$`)
)

// firstSub 返回第一个捕获组（无匹配返回空串）。
func firstSub(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// splitTrailing 把「正文 + 尾部电话/账号」拆开（地址电话、开户行账号共用）。
func splitTrailing(s string, tail *regexp.Regexp) (head, tailVal string) {
	s = strings.TrimSpace(s)
	if m := tail.FindStringSubmatch(s); len(m) > 1 {
		return strings.TrimSpace(strings.TrimSuffix(s, m[0])), m[1]
	}
	return s, ""
}

// applyFlatParties 回填购买方/销售方/开票人。OCR 与 docreader 的 HTML 表格里，
// 购销方信息集中在单元格内按「名 称 / 纳税人识别号 / 地 址、电 话 / 开户行及账号」
// 四行排列，可用固定标签确定性解析。
func applyFlatParties(inv *InvoiceExtractionItem, seg string) {
	if inv == nil {
		return
	}
	for _, m := range flatPartyCellRe.FindAllStringSubmatch(seg, -1) {
		body := m[2]
		name := firstSub(flatNameLikeRe, body)
		taxNo := firstSub(flatTaxNoLikeRe, body)
		addr, phone := splitTrailing(firstSub(flatAddrPhoneRe, body), flatPhoneTailRe)
		bank, account := splitTrailing(firstSub(flatBankAcctRe, body), flatAcctTailRe)
		if strings.Contains(m[1], "购买") {
			inv.BuyerName, inv.BuyerTaxNo = name, taxNo
			inv.BuyerAddr, inv.BuyerPhone = addr, phone
			inv.BuyerBank, inv.BuyerAcct = bank, account
		} else {
			inv.SellerName, inv.SellerTaxNo = name, taxNo
			inv.SellerAddr, inv.SellerPhone = addr, phone
			inv.SellerBank, inv.SellerAcct = bank, account
		}
	}
	if inv.Issuer == "" {
		inv.Issuer = firstSub(flatIssuerCell, seg)
	}
}
