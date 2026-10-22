package types

import "time"

// 能耗台账（energy scope）的列表契约。
//
// 与合同/制度/通知的列表契约同形态（一文件一记录 + 服务端全字段筛选分页），
// 但**主时间轴是账期（月）而不是日期（日）**：
//   - 合同的筛选轴是「签订日期」date_from/date_to；
//   - 能耗的筛选轴是「账期」billing_period（YYYY-MM）与「抄表日期」read_date。
// 因此过滤参数是月粒度的 period_from/period_to，不是日期区间。
//
// 键名与 internal/application/service/business_field_catalog_energy.go 的
// energyCatalog() 逐项对齐（六同步的字段契约层）。

// EnergyRecord 是能耗台账的一条记录（一个账期的一张汇总单）。
type EnergyRecord struct {
	// —— 标准字段（10）——
	UtilityNo     string `json:"utility_no"`     // 单据编号/户号
	BillingPeriod string `json:"billing_period"` // 账期 YYYY-MM
	UtilityType   string `json:"utility_type"`   // 能耗类型（电费/水费/燃气费/供热费）
	// 读数连续性主线
	PreviousReading *float64 `json:"previous_reading"` // 上期读数
	CurrentReading  *float64 `json:"current_reading"`  // 本期读数
	Multiplier      *float64 `json:"multiplier"`       // 倍率
	UsageAmount     *float64 `json:"usage_amount"`     // 实际用量
	UnitPrice       *float64 `json:"unit_price"`       // 单价
	TotalAmount     *float64 `json:"total_amount"`     // 总金额
	ExtractStatus   string   `json:"extract_status"`   // 提取状态
	// —— 扩展字段 ——
	AccountName   string `json:"account_name"`
	MeterNo       string `json:"meter_no"`
	Address       string `json:"address"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	ReadDate      string `json:"read_date"`
	UsageCategory string `json:"usage_category"`
	VoltageLevel  string `json:"voltage_level"`
	Remark        string `json:"remark"`
	// —— 明细（多表位读数；一文件一档案，不裂变为多条记录）——
	Items []EnergyMeterItem `json:"items"`
	// —— 用量校验告警（前端黄色告警面板的数据源）——
	Issues []EnergyUsageIssue `json:"issues"`
	// UsagePatched 实际用量是否由读数反算自动补齐（前端标注「系统推算」）
	UsagePatched bool `json:"usage_patched"`
	// —— 系统字段 ——
	Fields         map[string]any `json:"fields,omitempty"`
	KnowledgeID    string         `json:"knowledge_id"`
	KnowledgeTitle string         `json:"knowledge_title"`
	FileName       string         `json:"file_name"`
	FileType       string         `json:"file_type"`
	Tags           []string       `json:"tags"`
	ExtractError   string         `json:"extract_error"`
	KBID           string         `json:"knowledge_base_id"`
	CreatedAt      time.Time      `json:"created_at"`
	// ParseStatus 文件级解析状态：状态列徽标的判定输入。
	// 缺了它前端 useDocStatus 会把「extract_status=success 的已归档记录」
	// 判成「待解析」（真机踩过：状态列恒显示「待解析」）。
	ParseStatus string `json:"parse_status"`
}

// EnergyMeterItem 是能耗台账的表位明细行（多计量点的读数）。
type EnergyMeterItem struct {
	MeterNo         string   `json:"meter_no"`
	MeterType       string   `json:"meter_type"`
	PreviousReading *float64 `json:"previous_reading"`
	CurrentReading  *float64 `json:"current_reading"`
	Multiplier      *float64 `json:"multiplier"`
	UsageAmount     *float64 `json:"usage_amount"`
	// ComputedUsage 由读数推算的用量（与 UsageAmount 不一致时前端高亮）
	ComputedUsage *float64 `json:"computed_usage"`
	// UsageMismatch 用量与读数推算值不符（告警）
	UsageMismatch bool   `json:"usage_mismatch"`
	BillingPeriod string `json:"billing_period"`
	Remarks       string `json:"remarks"`
}

// EnergyUsageIssue 是用量校验问题。
type EnergyUsageIssue struct {
	Code     string  `json:"code"`
	Message  string  `json:"message"`
	Expected float64 `json:"expected"`
	Actual   float64 `json:"actual"`
}

// 用量校验问题码（前端按码定制文案与图标）。
const (
	// EnergyIssueUsageMismatch 用量与读数推算值不符
	EnergyIssueUsageMismatch = "usage_mismatch"
	// EnergyIssueUsageMissing 缺读数，无法交叉校验用量
	EnergyIssueUsageMissing = "usage_missing"
	// EnergyIssueReadingMissing 读数与用量全缺
	EnergyIssueReadingMissing = "reading_missing"
	// EnergyIssueReadingRegressed 本期读数小于上期读数（换表/回抄）
	EnergyIssueReadingRegressed = "reading_regressed"
)

// EnergyListFilter 是能耗台账列表查询参数。
type EnergyListFilter struct {
	Keyword       string // 全字段包含搜索
	UtilityType   string // 能耗类型（分类主轴）
	BillingPeriod string // 账期精确匹配 YYYY-MM
	PeriodFrom    string // 账期起 YYYY-MM（含）
	PeriodTo      string // 账期止 YYYY-MM（含）
	ReadDateFrom  string // 抄表日期起 YYYY-MM-DD（含）
	ReadDateTo    string // 抄表日期止 YYYY-MM-DD（含）
	Status        string // extract_status
	// EnabledCategories 当前启用中的能耗分类名集合（scope=energy，服务端权威源）。
	// 与合同/制度同口径：分类归属的「第一命中」判定用它。
	EnabledCategories []string
	Page              int
	PageSize          int
}

// EnergyTypeCount 是能耗类型下拉列表的一个选项。
type EnergyTypeCount struct {
	UtilityType string `json:"utility_type"`
	Count       int    `json:"count"`
}

// EnergyListResult 是能耗台账列表返回值，聚合值基于当前筛选后的可见记录。
type EnergyListResult struct {
	Data        []EnergyRecord `json:"data"`
	Total       int            `json:"total"`
	SumUsage    float64        `json:"sum_usage"`
	SumTotal    float64        `json:"sum_total"`
	SumPrevious float64        `json:"sum_previous"`
	SumCurrent  float64        `json:"sum_current"`
	IssueCount  int            `json:"issue_count"`
	Page        int            `json:"page"`
	PageSize    int            `json:"page_size"`
}

// EnergyOverviewStats 是能耗台账概览统计（纯只读全量聚合，不随列表筛选变化）。
type EnergyOverviewStats struct {
	Total         int                  `json:"total"`           // 全部能耗单份数
	SumUsage      float64              `json:"sum_usage"`       // 实际用量合计
	SumTotal      float64              `json:"sum_total"`       // 总金额合计
	IssueCount    int                  `json:"issue_count"`     // 存在用量告警的记录数
	FileCount     int                  `json:"file_count"`      // 已上传文件数
	ParseFailed   int                  `json:"parse_failed"`    // 解析失败文件数
	ExtractFailed int                  `json:"extract_failed"`  // 提取失败记录数
	ByUtilityType []EnergyTypeStat     `json:"by_utility_type"` // 按能耗类型分组
	ByCategory    []EnergyCategoryStat `json:"by_category"`     // 按用户定义分类分组，供主页类型卡遍历
}

// EnergyTypeStat 是单一能耗类型的聚合统计。
type EnergyTypeStat struct {
	UtilityType string  `json:"utility_type"`
	Count       int     `json:"count"`
	SumUsage    float64 `json:"sum_usage"`
	SumTotal    float64 `json:"sum_total"`
}

// EnergyCategoryStat 是单一用户分类（utility_type 精确值）的聚合统计。
type EnergyCategoryStat struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	SumUsage float64 `json:"sum_usage"`
	SumTotal float64 `json:"sum_total"`
}
