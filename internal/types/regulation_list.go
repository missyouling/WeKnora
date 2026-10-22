package types

import (
	"encoding/json"
	"time"
)

// RegulationExtractionItem mirrors one regulation document's extracted fields.
type RegulationExtractionItem struct {
	RegNo           string `json:"reg_no"`          // 制度编号（如 AD-032）
	RegName         string `json:"reg_name"`        // 制度名称（如 员工转岗管理制度）
	RegType         string `json:"reg_type"`        // 制度类型（人事管理/财务管理/...）
	Dept            string `json:"dept"`            // 编制部门
	Version         string `json:"version"`         // 版本
	IssueDate       string `json:"issue_date"`      // 编制日期 YYYY-MM-DD
	PageCount       string `json:"page_count"`      // 页数（如 3 / 共3页）
	ModifyCount     string `json:"modify_count"`    // 修改次数
	Scope           string `json:"scope"`           // 适用范围
	EffectiveDate   string `json:"effective_date"`  // 生效日期 YYYY-MM-DD
	Confidentiality string `json:"confidentiality"` // 密级
	Remark          string `json:"remark"`          // 备注
	CompiledBy      string `json:"compiled_by"`     // 编制（编制人）
	ReviewedBy      string `json:"reviewed_by"`     // 审核（审核人）
	ApprovedBy      string `json:"approved_by"`     // 批准（批准人）
	Summary         string `json:"summary"`         // 摘要
	Attachments     string `json:"attachments"`     // 附件清单
	// Page is always 1 for regulations (one file = one regulation).
	Page int `json:"page"`
	// Fields 自定义动态字段扩展桶：用户在「字段定义」里新增的字段（如「保证金」）
	// 不在强类型字段上，UnmarshalJSON 把顶层未知键与显式 fields 子对象统一收进这里，
	// 否则 json.Unmarshal 会静默丢弃它们（2026-10-22 真机事故）。
	Fields map[string]any `json:"fields,omitempty"`
}

// UnmarshalJSON 收集用户自定义字段进 Fields（标准字段仍走强类型解码）。
func (r *RegulationExtractionItem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	type alias RegulationExtractionItem
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = RegulationExtractionItem(a)
	r.Fields = CollectExtractionCustomFields(raw, StandardJSONKeys(RegulationExtractionItem{}), a.Fields)
	return nil
}

// RegulationRecord is one row of the regulation management list.
type RegulationRecord struct {
	RegNo           string         `json:"reg_no"`
	RegName         string         `json:"reg_name"`
	RegType         string         `json:"reg_type"`
	Dept            string         `json:"dept"`
	Version         string         `json:"version"`
	IssueDate       string         `json:"issue_date"`
	PageCount       string         `json:"page_count"`
	ModifyCount     string         `json:"modify_count"`
	Scope           string         `json:"scope"`
	EffectiveDate   string         `json:"effective_date"`
	Confidentiality string         `json:"confidentiality"`
	Remark          string         `json:"remark"`
	CompiledBy      string         `json:"compiled_by"`
	ReviewedBy      string         `json:"reviewed_by"`
	ApprovedBy      string         `json:"approved_by"`
	Summary         string         `json:"summary"`
	Attachments     string         `json:"attachments"`
	Fields          map[string]any `json:"fields,omitempty"`
	KnowledgeID     string         `json:"knowledge_id"`
	KnowledgeTitle  string         `json:"knowledge_title"`
	FileName        string         `json:"file_name"`
	FileType        string         `json:"file_type"`
	Tags            []string       `json:"tags"`
	ExtractStatus   string         `json:"extract_status"`
	ExtractError    string         `json:"extract_error"`
	KBID            string         `json:"kb_id"`
	CreatedAt       time.Time      `json:"created_at"`
}

// RegulationListFilter is the server-side filter for the regulation list.
type RegulationListFilter struct {
	Keyword string
	RegType string
	// KnownCategories 启用分类名集合（服务端权威源）：既供「其它制度」排除式
	// 筛选使用，也是分类归属「第一命中」的判定集合（见 resolveBusinessCategory）。
	KnownCategories []string
	// EnabledCategories 与 KnownCategories 同源；显式声明以便读取侧无需
	// 区分两个字段的语义（分类归属统一读它）。
	EnabledCategories []string
	Status            string
	DateFrom          string
	DateTo            string
	Page              int
	PageSize          int
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

// RegulationOverviewStats 是制度台账概览统计（纯只读全量聚合，不随列表筛选变化）：
// 遍历知识库下全部 knowledge → 平铺制度记录（口径与 ListRegulationRecords 一致），
// 在内存中聚合总份数/本月新增/类型分布，兼容 Postgres 与 SQLite 双库。
type RegulationOverviewStats struct {
	Total        int                      `json:"total"`         // 全部制度份数
	CurrentMonth int                      `json:"current_month"` // 本月（编制日期）新增数
	FileCount    int                      `json:"file_count"`    // 已上传文件数（含解析失败文件）
	ParseFailed  int                      `json:"parse_failed"`  // 解析失败文件数
	ByRegType    []RegulationTypeStat     `json:"by_reg_type"`   // 按制度类型分组（固定/兼容口径）
	ByCategory   []RegulationCategoryStat `json:"by_category"`   // 按用户定义分类（reg_type 精确值）分组，供主页类型卡遍历
}

// RegulationTypeStat 是单一制度类型的聚合统计。
type RegulationTypeStat struct {
	RegType string `json:"reg_type"`
	Count   int    `json:"count"`
}

// RegulationCategoryStat 是单一用户分类（reg_type 精确值）的聚合统计；
// reg_type 为空的记录归「未分类」。与发票/合同 CategoryStat 对齐，
// 让主页类型卡能精确统计自定义分类（计数不再被硬编码桶合并）。
type RegulationCategoryStat struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}
