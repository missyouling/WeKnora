package types

import "time"

// AwardPunishExtractionItem mirrors one notice document's extracted fields.
// 通知公告模块（原奖惩）一份文件一份公告，内部 scope 仍为 award_punish。
type AwardPunishExtractionItem struct {
	ApNo          string `json:"ap_no"`           // 文号（如 星达行政字〔2026〕11号 / JC-20260907-001）
	ApTitle       string `json:"ap_title"`        // 标题（如 关于加强安全生产管理的通知）
	ApType        string `json:"ap_type"`         // 文件类型（通知/公告/通报/决定/其它通知）
	Person        string `json:"person"`          // 当事人（通报/决定类文件涉及人员）
	Dept          string `json:"dept"`            // 部门（当事人所属部门）
	IssueDept     string `json:"issue_dept"`      // 发布单位（文件落款单位）
	Position      string `json:"position"`        // 岗位
	Measure       string `json:"measure"`         // 处理决定（书面警告/经济罚款/嘉奖等）
	Basis         string `json:"basis"`           // 依据（制度/法规依据原文）
	Signer        string `json:"signer"`          // 发布人
	SignDate      string `json:"sign_date"`       // 发布日期 YYYY-MM-DD
	EffectiveDate string `json:"effective_date"`  // 生效日期 YYYY-MM-DD
	Scope         string `json:"scope"`           // 受文对象 / 适用范围
	Contact       string `json:"contact"`         // 联系人
	ContactPhone  string `json:"contact_phone"`   // 联系电话
	Urgency       string `json:"urgency"`         // 紧急程度 / 重要程度
	ValidUntil    string `json:"valid_until"`     // 有效期限
	Attachments   string `json:"attachments"`     // 附件名称
	Remark        string `json:"remark"`          // 备注
	Summary       string `json:"summary"`         // 摘要（正文概述）
	// Page is always 1 for notices (one file = one notice).
	Page int `json:"page"`
}

// AwardPunishRecord is one row of the notice management list.
type AwardPunishRecord struct {
	RowKey        string   `json:"row_key"`
	ApNo          string   `json:"ap_no"`
	ApTitle       string   `json:"ap_title"`
	ApType        string   `json:"ap_type"`
	Person        string   `json:"person"`
	Dept          string   `json:"dept"`
	IssueDept     string   `json:"issue_dept"`
	Position      string   `json:"position"`
	Measure       string   `json:"measure"`
	Basis         string   `json:"basis"`
	Signer        string   `json:"signer"`
	SignDate      string   `json:"sign_date"`
	EffectiveDate string   `json:"effective_date"`
	Scope         string   `json:"scope"`
	Contact       string   `json:"contact"`
	ContactPhone  string   `json:"contact_phone"`
	Urgency       string   `json:"urgency"`
	ValidUntil    string   `json:"valid_until"`
	Attachments   string   `json:"attachments"`
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

// AwardPunishListFilter is the server-side filter for the notice list.
type AwardPunishListFilter struct {
	Keyword         string
	ApType          string
	KnownCategories []string // 启用分类名集合，供「其它通知」排除式筛选使用
	Status          string
	DateFrom        string
	DateTo          string
	Page            int
	PageSize        int
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

// AwardPunishOverviewStats 是通知公告台账概览统计（纯只读全量聚合，不随列表筛选变化）：
// 遍历知识库下全部 knowledge → 平铺公告记录（口径与 ListAwardPunishRecords 一致），
// 在内存中聚合总条数/本月新增/分类分布，兼容 Postgres 与 SQLite 双库。
type AwardPunishOverviewStats struct {
	Total        int                    `json:"total"`         // 全部公告条数
	CurrentMonth int                    `json:"current_month"` // 本月（发布日期）新增数
	FileCount    int                    `json:"file_count"`    // 已上传文件数（含解析失败文件）
	ParseFailed  int                    `json:"parse_failed"`  // 解析失败文件数
	ByApType     []AwardPunishTypeStat  `json:"by_ap_type"`    // 按文件类型分组（固定/兼容口径）
	ByCategory   []NoticeCategoryStat   `json:"by_category"`   // 按用户定义分类（ap_type 精确值）分组，供主页类型卡遍历
}

// AwardPunishTypeStat 是单一文件类型的聚合统计。
type AwardPunishTypeStat struct {
	ApType string `json:"ap_type"`
	Count  int    `json:"count"`
}

// NoticeCategoryStat 是单一用户分类（ap_type 精确值）的聚合统计；
// ap_type 为空的记录归「未分类」。与发票/合同/制度 CategoryStat 对齐。
type NoticeCategoryStat struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}
