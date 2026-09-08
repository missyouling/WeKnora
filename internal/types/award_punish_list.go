package types

import "time"

// AwardPunishExtractionItem mirrors one award/punish record extracted from a
// notice document. A single notice may carry several persons (one item each),
// exactly like invoices carry several pages.
type AwardPunishExtractionItem struct {
	ApNo          string `json:"ap_no"`           // 文号（如 星达行政字〔2026〕11号 / JC-20260907-001）
	ApTitle       string `json:"ap_title"`        // 标题（如 关于对汪艳、王郑红违规行为的处罚通报）
	ApType        string `json:"ap_type"`         // 奖惩类型（处罚/奖励/通报/其它奖惩）
	Person        string `json:"person"`          // 当事人
	Dept          string `json:"dept"`            // 部门
	Position      string `json:"position"`        // 岗位
	Measure       string `json:"measure"`         // 措施（书面警告/经济罚款/嘉奖等）
	Basis         string `json:"basis"`           // 依据（制度依据原文）
	Signer        string `json:"signer"`          // 签发人
	SignDate      string `json:"sign_date"`       // 签发日期 YYYY-MM-DD
	EffectiveDate string `json:"effective_date"`  // 生效日期 YYYY-MM-DD
	Remark        string `json:"remark"`          // 备注
	Summary       string `json:"summary"`         // 摘要（正文概述）
	// Page is the item ordinal inside the file (1-based), reused for display
	// ordering. It does not correspond to a PDF page.
	Page int `json:"page"`
}

// AwardPunishRecord is one row of the award/punish management list.
type AwardPunishRecord struct {
	RowKey        string   `json:"row_key"`
	ApNo          string   `json:"ap_no"`
	ApTitle       string   `json:"ap_title"`
	ApType        string   `json:"ap_type"`
	Person        string   `json:"person"`
	Dept          string   `json:"dept"`
	Position      string   `json:"position"`
	Measure       string   `json:"measure"`
	Basis         string   `json:"basis"`
	Signer        string   `json:"signer"`
	SignDate      string   `json:"sign_date"`
	EffectiveDate string   `json:"effective_date"`
	Remark        string   `json:"remark"`
	Summary       string   `json:"summary"`
	Page          int      `json:"page"`
	KnowledgeID   string   `json:"knowledge_id"`
	KnowledgeTitle string  `json:"knowledge_title"`
	FileName      string   `json:"file_name"`
	FileType      string   `json:"file_type"`
	Tags          []string `json:"tags"`
	ExtractStatus string   `json:"extract_status"`
	ExtractError  string   `json:"extract_error"`
	KBID          string   `json:"kb_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// AwardPunishListFilter is the server-side filter for the award/punish list.
type AwardPunishListFilter struct {
	Keyword  string
	ApType   string
	Status   string
	DateFrom string
	DateTo   string
	Page     int
	PageSize int
}

// AwardPunishListResult is the paginated award/punish list with totals.
type AwardPunishListResult struct {
	Data     []AwardPunishRecord `json:"data"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// AwardPunishTypeCount is a distinct award/punish type seen in the KB (with count).
type AwardPunishTypeCount struct {
	ApType string `json:"ap_type"`
	Count  int    `json:"count"`
}
