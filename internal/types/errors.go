package types

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrTokenStoreUnavailable 标记「鉴权数据存储不可用」——与「token 确实无效」
// 是两类完全不同的故障：
//   - 前者是基础设施故障（DB 连接池打满/网络断），重试即可恢复，**绝不能**
//     让前端清空登录态；
//   - 后者才应该返回 401 并引导用户重新登录。
//
// 实测事故：Supabase session 模式连接池上限 15 被打满，鉴权 SQL 报
// EMAXCONNSESSION，`ValidateToken` 把它当错误返回、中间件一律映射成 401，
// 前端随即清 token 跳登录页——用户在一次 50 秒解析轮询里被踢下线数次。
var ErrTokenStoreUnavailable = errors.New("token store unavailable")

// StorageQuotaExceededError represents the storage quota exceeded error
type StorageQuotaExceededError struct {
	Message string
}

// Error implements the error interface
func (e *StorageQuotaExceededError) Error() string {
	return e.Message
}

// NewStorageQuotaExceededError creates a storage quota exceeded error
func NewStorageQuotaExceededError() *StorageQuotaExceededError {
	return &StorageQuotaExceededError{
		Message: "Storage quota exceeded",
	}
}

// DuplicateKnowledgeError duplicate knowledge error, contains the existing knowledge object
type DuplicateKnowledgeError struct {
	Message   string
	Knowledge *Knowledge
	// Recovery 上传去重冲突的「可恢复性摘要」。上传弹窗据此决定：
	//   - CanReextract=true  → 不报错拦截，自动改分类 + 重新提取，数据直接回列表；
	//   - Normal=true        → 文件已有正常票据，提示当前分类/开票日期，给手动重新提取入口。
	// 没有该摘要时前端按最保守的「提示已存在」处理，绝不自动发起提取。
	Recovery *DuplicateRecovery `json:"recovery,omitempty"`
}

func (e *DuplicateKnowledgeError) Error() string {
	return e.Message
}

// DuplicateRecovery 描述重复文件当前的提取状态，供上传弹窗判断能否自动自愈。
type DuplicateRecovery struct {
	// CanReextract 该文件当前没有可用票据（未提取 / 提取失败 / 被判定非本业务），
	// 可以安全地重新提取。
	CanReextract bool `json:"can_reextract"`
	// Normal 已有正常发票数据（重提取会覆盖，需用户确认场景）。
	Normal bool `json:"normal"`
	// Kind 归档分类标识（invoice / not_invoice / 空）
	Kind string `json:"kind,omitempty"`
	// Category 当前归档分类名（如「通行费发票」），空表示未分类。
	Category string `json:"category,omitempty"`
	// ExtractStatus 提取状态（success / failed / pending / 空）
	ExtractStatus string `json:"extract_status,omitempty"`
	// ExtractError 提取失败原因
	ExtractError string `json:"extract_error,omitempty"`
	// InvoiceCount 已提取出的发票张数
	InvoiceCount int `json:"invoice_count"`
	// InvoiceDate 首张票的开票日期（YYYY-MM-DD），用于提示"文件当前所在分类与开票日期"
	InvoiceDate string `json:"invoice_date,omitempty"`
	// InvoiceNo 首张票的发票号
	InvoiceNo string `json:"invoice_no,omitempty"`
	// Reason 拒绝自动重提取的原因（可展示）
	Reason string `json:"reason,omitempty"`
}

// SummarizeDuplicateRecovery 从已存在的 knowledge 记录推导「能否自动重提取」。
//
// 判定原则（保守优先）：
//   - 解析失败 / 删除中 / 非票据 / metadata 损坏 → 不自动重提取，给出原因；
//   - 已提取出有效票据 → Normal（提示分类与开票日期，交用户确认）；
//   - 其余（从未提取、提取失败、解析出 0 张）→ CanReextract，上传弹窗可自动
//     更新分类并重新提取，让数据直接回到列表，而不是把用户挡在冲突错误外。
//
// 返回 nil 表示无法判断（knowledge 为 nil），前端据此退回最保守的提示。
func SummarizeDuplicateRecovery(k *Knowledge) *DuplicateRecovery {
	if k == nil {
		return nil
	}
	r := &DuplicateRecovery{Kind: ""}
	var meta map[string]interface{}
	if len(k.CustomMetadata) > 0 {
		if err := json.Unmarshal([]byte(k.CustomMetadata), &meta); err != nil || meta == nil {
			// metadata 损坏：不能判为可自动重提取（可能反复失败），只能提示重传
			r.Reason = "文件记录异常，请重新上传"
			return r
		}
	} else {
		meta = map[string]interface{}{}
	}
	invoices, _ := meta["invoices"].([]interface{})
	r.InvoiceCount = len(invoices)

	switch k.ParseStatus {
	case "deleting":
		r.Reason = "文件正在删除中，请稍后重试"
		return r
	case "failed":
		r.Reason = "文件解析失败，请重新上传"
		return r
	}

	switch str(meta["kind"]) {
	case "not_invoice":
		r.Kind = "not_invoice"
		r.Reason = "该文件此前被判定为非票据，如需作为发票入库请先在列表中修改类型"
		return r
	case "invoice":
		r.Kind = "invoice"
	}

	r.ExtractStatus = str(meta["extract_status"])
	r.ExtractError = str(meta["extract_error"])
	if ct := str(meta["fleet_cert_type"]); ct != "" {
		r.Category = ct
	} else if len(invoices) > 0 {
		r.Category = str(invoices[0].(map[string]interface{})["category"])
	}
	for _, e := range invoices {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if r.InvoiceNo == "" {
			r.InvoiceNo = str(m["invoice_no"])
			r.InvoiceDate = str(m["invoice_date"])
		}
	}

	// 已有有效票据 → 只提示不自动覆盖
	if r.InvoiceCount > 0 && r.ExtractStatus != "failed" {
		r.Normal = true
		r.Reason = ""
		return r
	}
	// 其余：可自动重提取
	r.CanReextract = true
	return r
}

func str(v interface{}) string {
	s, _ := v.(string)
	return s
}

// NewDuplicateFileError creates a duplicate file error
func NewDuplicateFileError(knowledge *Knowledge) *DuplicateKnowledgeError {
	return &DuplicateKnowledgeError{
		Message:   fmt.Sprintf("File already exists: %s", knowledge.FileName),
		Knowledge: knowledge,
		Recovery:  SummarizeDuplicateRecovery(knowledge),
	}
}

// NewDuplicateURLError creates a duplicate URL error
func NewDuplicateURLError(knowledge *Knowledge) *DuplicateKnowledgeError {
	return &DuplicateKnowledgeError{
		Message:   fmt.Sprintf("URL already exists: %s", knowledge.Source),
		Knowledge: knowledge,
	}
}
