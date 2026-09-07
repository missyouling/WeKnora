package types

import "time"

// RegulationExtractionItem mirrors one regulation document's extracted fields.
type RegulationExtractionItem struct {
	RegNo          string `json:"reg_no"`           // 制度编号（如 AD-032）
	RegName        string `json:"reg_name"`         // 制度名称（如 员工转岗管理制度）
	RegType        string `json:"reg_type"`         // 制度类型（人事管理/财务管理/...）
	Dept           string `json:"dept"`             // 编制部门
	Version        string `json:"version"`          // 版本
	IssueDate      string `json:"issue_date"`       // 编制日期 YYYY-MM-DD
	PageCount      string `json:"page_count"`       // 页数（如 3 / 共3页）
	ModifyCount    string `json:"modify_count"`     // 修改次数
	Scope          string `json:"scope"`            // 适用范围
	EffectiveDate  string `json:"effective_date"`   // 生效日期 YYYY-MM-DD
	Confidentiality string `json:"confidentiality"` // 密级
	Remark         string `json:"remark"`           // 备注
	// Page is always 1 for regulations (one file = one regulation).
	Page int `json:"page"`
}

// RegulationRecord is one row of the regulation management list.
type RegulationRecord struct {
	RegNo          string   `json:"reg_no"`
	RegName        string   `json:"reg_name"`
	RegType        string   `json:"reg_type"`
	Dept           string   `json:"dept"`
	Version        string   `json:"version"`
	IssueDate      string   `json:"issue_date"`
	PageCount      string   `json:"page_count"`
	ModifyCount    string   `json:"modify_count"`
	Scope          string   `json:"scope"`
	EffectiveDate  string   `json:"effective_date"`
	Confidentiality string  `json:"confidentiality"`
	Remark         string   `json:"remark"`
	KnowledgeID    string   `json:"knowledge_id"`
	KnowledgeTitle string   `json:"knowledge_title"`
	FileName       string   `json:"file_name"`
	FileType       string   `json:"file_type"`
	Tags           []string `json:"tags"`
	ExtractStatus  string   `json:"extract_status"`
	ExtractError   string   `json:"extract_error"`
	KBID           string   `json:"kb_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// RegulationListFilter is the server-side filter for the regulation list.
type RegulationListFilter struct {
	Keyword  string
	RegType  string
	Status   string
	DateFrom string
	DateTo   string
	Page     int
	PageSize int
}

// RegulationListResult is the paginated regulation list with totals.
type RegulationListResult struct {
	Data     []RegulationRecord `json:"data"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

// RegulationTypeCount is a distinct regulation type seen in the KB (with count).
type RegulationTypeCount struct {
	RegType string `json:"reg_type"`
	Count   int    `json:"count"`
}
