package types

import "time"

// ContractRecord 是合同级聚合列表中的一行：合同字段 + 所属源文件上下文。
type ContractRecord struct {
	// —— 标准字段（11）：与字段定义顺序一致 ——
	ContractNo   string   `json:"contract_no"`
	ContractName string   `json:"contract_name"`
	ContractType string   `json:"contract_type"`
	PartyA       string   `json:"party_a"`
	PartyB       string   `json:"party_b"`
	SignDate     string   `json:"sign_date"`
	ExpireDate   string   `json:"expire_date"`
	Amount       *float64 `json:"amount"`
	// PerformanceStatus 由到期日期派生（履行中/即将到期/已超期/已终止），
	// 只读展示，不参与编辑与提取。
	PerformanceStatus string `json:"performance_status"`
	// —— 扩展字段（15）——
	PartyAContact string `json:"party_a_contact"`
	PartyATaxNo   string `json:"party_a_tax_no"`
	PartyBContact string `json:"party_b_contact"`
	PartyBTaxNo   string `json:"party_b_tax_no"`
	PartyAAddress string `json:"party_a_address"`
	PartyAPhone   string `json:"party_a_phone"`
	PartyABank    string `json:"party_a_bank"`
	PartyAAccount string `json:"party_a_account"`
	PartyBAddress string `json:"party_b_address"`
	PartyBPhone   string `json:"party_b_phone"`
	PartyBBank    string `json:"party_b_bank"`
	PartyBAccount string `json:"party_b_account"`
	PaymentMethod string `json:"payment_method"`
	SignLocation  string `json:"sign_location"`
	// Summary 摘要：优先复用知识库已生成的 summary（knowledges.description）。
	Summary string `json:"summary"`
	// —— 历史字段：保留输出以兼容老前端/导出，不再出现在字段定义中 ——
	EffectiveDate     string   `json:"effective_date,omitempty"`
	TaxRate           *float64 `json:"tax_rate,omitempty"`
	QualityBond       *float64 `json:"quality_bond,omitempty"`
	LiquidatedDamages *float64 `json:"liquidated_damages,omitempty"`
	Subject           string   `json:"subject,omitempty"`
	Quantity          *float64 `json:"quantity,omitempty"`
	UnitPrice         *float64 `json:"unit_price,omitempty"`
	PerformancePeriod string   `json:"performance_period,omitempty"`
	Handler           string   `json:"handler,omitempty"`
	Department        string   `json:"department,omitempty"`
	Remark            string   `json:"remark,omitempty"`
	Page              int      `json:"page"`
	// Fields 自定义动态字段扩展桶（用户在「字段定义」里新增的字段，如「保证金」）。
	// 由提取项的 Fields 透传，前端动态列与详情抽屉按字段名从这里取值。
	Fields         map[string]any `json:"fields,omitempty"`
	KnowledgeID    string         `json:"knowledge_id"`
	KnowledgeTitle string         `json:"knowledge_title"`
	FileName       string         `json:"file_name"`
	FileType       string         `json:"file_type"`
	Tags           []string       `json:"tags"`
	ExtractStatus  string         `json:"extract_status"`
	ExtractError   string         `json:"extract_error"`
	KBID           string         `json:"knowledge_base_id"`
	CreatedAt      time.Time      `json:"created_at"`
}

// ContractListFilter 是合同级聚合列表查询参数。
type ContractListFilter struct {
	Keyword       string // 全字段包含搜索
	ContractType  string // 合同类型枚举之一
	FulfillStatus string // 履约状态（待签署/执行中/已完成/已到期/已终止）
	Status        string // extract_status
	DateFrom      string // 签订日期起 YYYY-MM-DD（含）
	DateTo        string // 签订日期止 YYYY-MM-DD（含）
	// EnabledCategories 当前启用中的合同分类名集合（scope=contract，服务端权威源）。
	// 分类归属判定用它做「第一命中」：候选 [fleet_cert_type, contract_type]
	// 里第一个命中该集合的值成为记录的 ContractType —— 保证「表格合同类型列
	// 显示什么，它就 100% 属于哪个分类卡片桶」。由服务端从 fleet_categories
	// 实时加载，不再依赖前端传参猜分类（口径漂移源）。
	EnabledCategories []string
	Page              int
	PageSize          int
}

// ContractTypeCount 是合同类型下拉列表的一个选项：类型 + 出现次数。
type ContractTypeCount struct {
	ContractType string `json:"contract_type"`
	Count        int    `json:"count"`
}

// ContractListResult 是聚合列表返回值，聚合值基于当前筛选后的可见记录。
type ContractListResult struct {
	Data      []ContractRecord `json:"data"`
	Total     int              `json:"total"`
	SumAmount float64          `json:"sum_amount"`
	SumTax    float64          `json:"sum_tax"`
	SumTotal  float64          `json:"sum_total"`
	Page      int              `json:"page"`
	PageSize  int              `json:"page_size"`
}

// ContractOverviewStats 是合同台账概览统计（纯只读全量聚合，不随列表筛选变化）：
// 遍历知识库下全部 knowledge → 平铺合同记录（口径与 ListContractRecords 一致），
// 在内存中聚合总份数/金额/本月新增/即将到期/类型分布，兼容 Postgres 与 SQLite 双库。
type ContractOverviewStats struct {
	Total          int                    `json:"total"`            // 全部合同份数（含未分类/其它合同与待补录占位行）
	SumTotal       float64                `json:"sum_total"`        // 合同金额合计（金额字段空按 0）
	CurrentMonth   ContractMonthStat      `json:"current_month"`    // 本月（签订日期）新增看板
	Expiring       int                    `json:"expiring"`         // 即将到期：到期日期在未来 30 天内
	FileCount      int                    `json:"file_count"`       // 已上传文件数（含解析失败文件）
	ParseFailed    int                    `json:"parse_failed"`     // 解析失败文件数
	ExtractFailed  int                    `json:"extract_failed"`   // 提取失败合同数
	ByContractType []ContractTypeStat     `json:"by_contract_type"` // 按合同类型分组（服务合同/租赁合同/其它合同三桶）
	ByCategory     []ContractCategoryStat `json:"by_category"`      // 按用户定义分类（contract_type 精确值）分组，供主页类型卡遍历
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

// ContractCategoryStat 是单一用户分类（contract_type 精确值）的聚合统计；
// contract_type 为空的记录归「未分类」。与发票 InvoiceCategoryStat 对齐，
// 让主页类型卡能精确统计自定义分类（计数不再被硬编码三桶合并）。
type ContractCategoryStat struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	SumTotal float64 `json:"sum_total"`
}
