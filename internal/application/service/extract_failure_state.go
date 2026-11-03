package service

import (
	"encoding/json"
	"strings"
)

// 提取失败时的状态降级守门（第 25 轮问题 1）。
//
// 背景事故：`mergeFailedStateMeta` 在任何失败（模型 429 / panic / 超时）时
// 无条件把 `extract_status` 写成 `failed`。但库里往往**已经有上一次成功提取
// 的发票行**（重试失败不该抹掉既有成果）。于是出现三种互相矛盾的表现：
//
//	列表（记录级状态，核心字段完整即 success） → 绿色「提取成功」
//	详情抽屉（文档级 extract_status）           → 红色「提取失败」
//	历史文件抽屉（文档级 extract_status）       → 「异常文件 74」
//
// 真机样本 3354297f（2026.08月气费发票.pdf）：extract_error 是
// 「请求频率超限」，invoices 里躺着 1 张完整发票 —— 数据是对的，只有状态错。
//
// 本文件提供判定：**只要库里已有可用记录，失败就只记错误、不降级状态**。
// 这样「重试失败」不会让已经进主列表的记录突然变成失败态。

// HasUsableInvoiceRecords 判断 custom_metadata 里是否已有可用的发票行。
//
// 判定口径与列表的记录级状态保持一致：发票号非空 **或** 金额/价税合计非零，
// 即视为「这张票是真实提取出来的」，不是占位残片。
//
// 只看发票号会漏掉一类真实场景：少数票 OCR 把号码吃掉但金额齐全，用户在
// 列表里能看到金额并手动补号码 —— 那种记录也不能因为一次重试失败就标失败。
func HasUsableInvoiceRecords(meta []byte) bool {
	if len(meta) == 0 {
		return false
	}
	var m struct {
		Invoices []struct {
			InvoiceNo   string   `json:"invoice_no"`
			Amount      *float64 `json:"amount"`
			TotalAmount *float64 `json:"total_amount"`
		} `json:"invoices"`
	}
	if err := json.Unmarshal(meta, &m); err != nil {
		return false
	}
	for _, inv := range m.Invoices {
		if strings.TrimSpace(inv.InvoiceNo) != "" {
			return true
		}
		if inv.Amount != nil && *inv.Amount != 0 {
			return true
		}
		if inv.TotalAmount != nil && *inv.TotalAmount != 0 {
			return true
		}
	}
	return false
}

// SyncDocumentIssuer 把票级开票人回填到文档级 issuer / drawer。
//
// 三处必须同源：invoices[i].issuer、invoices[i].drawer、custom_metadata.issuer/drawer。
// 详情抽屉在未选中具体发票行时读文档级字段；只写票级会导致抽屉顶部空白，
// 而列表（读票级）有值 —— 即「同一张票两处不一致」（第 25 轮问题 2）。
//
// 取**首个非空**票的开票人：多页文件的开票人可能不同（不同开票机），
// 文档级只作为摘要展示，逐票仍以票级为准。
func SyncDocumentIssuer(meta *InvoiceCustomMetadata) {
	if meta == nil || len(meta.Invoices) == 0 {
		return
	}
	for _, inv := range meta.Invoices {
		name := strings.TrimSpace(inv.Issuer)
		if name == "" {
			name = strings.TrimSpace(inv.Drawer)
		}
		if name == "" {
			continue
		}
		meta.Issuer = name
		meta.Drawer = name
		return
	}
}

// SuppressInvoiceAnchorWarning 判断「锚点数多于已提取票数」这个差异是否应当静默。
//
// 背景（第 25 轮问题 1 真机样本 3354297f）：
//
//	源文件是**单页**气费发票，正文只有 1 个「发票号码：」，但备注里还有
//	`银行账号:50001183600050002131`（20 位）。早期锚点算法把这段数字也算成
//	票号，于是「原文共 2 张锚点，仅提取到 1 张」的橙色警告一直挂在详情页上。
//	而该文件唯一的发票号码/日期/金额/税额/价税合计/购销方全部齐全。
//
// 两条抑制条件：
//
//  1. 单页（anchors <= 1）且已提取到至少 1 张 —— 物理上一页最多一张电子发票，
//     差异只能来自误计；
//  2. 已提取的票**全部**字段完整 —— 数据本身没问题，提示只会误导用户去
//     「复核源文件」（而源文件是对的）。
//
// 反向保护：多页文件 + 存在字段残缺的票时，仍然要报 —— 那才是真漏提。
func SuppressInvoiceAnchorWarning(anchors int, invoices []InvoiceExtractionItem) bool {
	if len(invoices) == 0 {
		return false // 一张都没提到，必须报
	}
	// 只提到 1 张票时**一定抑制**：一张电子发票在物理上只占一页，
	// 「原文还有一张」这个结论只可能来自锚点误计（真机 3354297f 就是
	// 单页气费票的备注里有 20 位银行账号，被早期算法当成第二张票）。
	// 注意不能用「锚点 <= 1」判断，因为误计后锚点已经是 2。
	if anchors <= 1 || len(invoices) == 1 {
		return true
	}
	for _, inv := range invoices {
		if !invoiceItemCoreComplete(inv) {
			return false // 有残票 → 可能是真漏提，保留告警
		}
	}
	return true
}
