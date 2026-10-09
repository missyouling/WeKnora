package types

import "time"

// ContractRecord 是合同级聚合列表中的一行：合同字段 + 所属源文件上下文。
type ContractRecord struct {
	ContractNo     string    `json:"contract_no"`
	ContractName   string    `json:"contract_name"`
	ContractType   string    `json:"contract_type"`
	SignDate       string    `json:"sign_date"`
	EffectiveDate  string    `json:"effective_date"`
	ExpiryDate     string    `json:"expiry_date"`
	SignPlace      string    `json:"sign_place"`
	PartyAName     string    `json:"party_a_name"`
	PartyATaxNo    string    `json:"party_a_tax_no"`
	PartyAAddress  string    `json:"party_a_address"`
	PartyAPhone    string    `json:"party_a_phone"`
	PartyABank     string    `json:"party_a_bank"`
	PartyAAccount  string    `json:"party_a_account"`
	PartyBName     string    `json:"party_b_name"`
	PartyBTaxNo    string    `json:"party_b_tax_no"`
	PartyBAddress  string    `json:"party_b_address"`
	PartyBPhone    string    `json:"party_b_phone"`
	PartyBBank     string    `json:"party_b_bank"`
	PartyBAccount  string    `json:"party_b_account"`
	ContractAmount *float64  `json:"contract_amount"`
	TaxRate        *float64  `json:"tax_rate"`
	PaymentMethod  string    `json:"payment_method"`
	QualityBond    *float64  `json:"quality_bond"`
	LiquidatedDamages *float64 `json:"liquidated_damages"`
	Subject        string    `json:"subject"`
	Quantity       *float64  `json:"quantity"`
	UnitPrice      *float64  `json:"unit_price"`
	PerformancePeriod string `json:"performance_period"`
	Handler        string    `json:"handler"`
	Department     string    `json:"department"`
	Remark         string    `json:"remark"`
	FulfillStatus  string    `json:"fulfill_status"`
	Page           int       `json:"page"`
	KnowledgeID    string    `json:"knowledge_id"`
	KnowledgeTitle string    `json:"knowledge_title"`
	FileName       string    `json:"file_name"`
	FileType       string    `json:"file_type"`
	Tags           []string  `json:"tags"`
	ExtractStatus  string    `json:"extract_status"`
	ExtractError   string    `json:"extract_error"`
	KBID           string    `json:"knowledge_base_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// ContractListFilter 是合同级聚合列表查询参数。
type ContractListFilter struct {
	Keyword      string // 全字段包含搜索
	ContractType string // 合同类型枚举之一
	FulfillStatus string // 履约状态（待签署/执行中/已完成/已到期/已终止）
	Status       string // extract_status
	DateFrom     string // 签订日期起 YYYY-MM-DD（含）
	DateTo       string // 签订日期止 YYYY-MM-DD（含）
	Page         int
	PageSize     int
}

// ContractTypeCount 是合同类型下拉列表的一个选项：类型 + 出现次数。
type ContractTypeCount struct {
	ContractType string `json:"contract_type"`
	Count        int    `json:"count"`
}

// ContractListResult 是聚合列表返回值，聚合值基于当前筛选后的可见记录。
type ContractListResult struct {
	Data         []ContractRecord `json:"data"`
	Total        int              `json:"total"`
	SumAmount    float64          `json:"sum_amount"`
	SumTax       float64          `json:"sum_tax"`
	SumTotal     float64          `json:"sum_total"`
	Page         int              `json:"page"`
	PageSize     int              `json:"page_size"`
}

// ContractOverviewStats 是合同台账概览统计（纯只读全量聚合，不随列表筛选变化）：
// 遍历知识库下全部 knowledge → 平铺合同记录（口径与 ListContractRecords 一致），
// 在内存中聚合总份数/金额/本月新增/即将到期/类型分布，兼容 Postgres 与 SQLite 双库。
type ContractOverviewStats struct {
	Total          int                `json:"total"`            // 全部合同份数（含未分类/其它合同与待补录占位行）
	SumTotal       float64            `json:"sum_total"`        // 合同金额合计（金额字段空按 0）
	CurrentMonth   ContractMonthStat  `json:"current_month"`    // 本月（签订日期）新增看板
	Expiring       int                `json:"expiring"`         // 即将到期：到期日期在未来 30 天内
	FileCount      int                `json:"file_count"`       // 已上传文件数（含解析失败文件）
	ParseFailed    int                `json:"parse_failed"`     // 解析失败文件数
	ExtractFailed  int                `json:"extract_failed"`   // 提取失败合同数
	ByContractType []ContractTypeStat `json:"by_contract_type"` // 按合同类型分组（服务合同/租赁合同/其它合同三桶）
}

// ContractMonthStat 是本月（签订日期）合同新增看板。
type ContractMonthStat struct {
	Count    int     `json:"count"`
	SumTotal float64 `json:"sum_total"`
}

// ContractTypeStat 是单一合同类型的聚合统计。
type ContractTypeStat struct {
	ContractType string  `json:"contract_type"`
	Count        int     `json:"count"`
	SumTotal     float64 `json:"sum_total"`
}
