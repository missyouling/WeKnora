package types

import (
	"time"
)

// UtilityFieldConfig 水电气字段配置：电费/水费/气费三个分类各自的字段集，
// 列表列、字段选择器、抽屉表单均由该配置驱动（字段可增删改、自定义）。
type UtilityFieldConfig struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	TenantID       int64      `gorm:"index" json:"tenant_id"`
	Category       string     `gorm:"index" json:"category"` // electricity | water | gas
	FieldKey       string     `json:"field_key"`
	Label          string     `json:"label"`
	FieldType      string     `json:"field_type"` // text | number | amount | date
	DefaultVisible bool       `json:"default_visible"`
	SortOrder      int        `json:"sort_order"`
	IsCustom       bool       `json:"is_custom"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"deleted_at"`
}

func (UtilityFieldConfig) TableName() string { return "utility_field_configs" }

// UtilityMeterRecord 水/气月度记录：同类型同月份唯一，含若干表计子行。
type UtilityMeterRecord struct {
	ID          string             `gorm:"primaryKey" json:"id"`
	TenantID    int64              `gorm:"index" json:"tenant_id"`
	Category    string             `gorm:"index" json:"category"` // water | gas
	Month       string             `gorm:"index" json:"month"`    // 2026-08
	MeterCount  int                `json:"meter_count"`
	TotalUsage  float64            `gorm:"numeric(18,2)" json:"total_usage"`
	TotalAmount float64            `gorm:"numeric(18,2)" json:"total_amount"`
	Remark      string             `gorm:"type:text" json:"remark"`
	Items       []UtilityMeterItem `gorm:"-" json:"items"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	DeletedAt   *time.Time         `gorm:"index" json:"deleted_at"`
}

func (UtilityMeterRecord) TableName() string { return "utility_meter_records" }

// UtilityMeterItem 表计子行：期初/期末读数、单价，用量与金额由后端自动计算。
type UtilityMeterItem struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	RecordID     string    `gorm:"index" json:"record_id"`
	MeterName    string    `json:"meter_name"`
	StartReading float64   `gorm:"numeric(18,2)" json:"start_reading"`
	EndReading   float64   `gorm:"numeric(18,2)" json:"end_reading"`
	UnitPrice    float64   `gorm:"numeric(18,2)" json:"unit_price"`
	Usage        float64   `gorm:"numeric(18,2)" json:"usage"`
	Amount       float64   `gorm:"numeric(18,2)" json:"amount"`
	Remark       string    `gorm:"type:text" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (UtilityMeterItem) TableName() string { return "utility_meter_items" }

// UtilityBillExtractionItem 电费账单提取字段（国网标准账单，四组全量）。
type UtilityBillExtractionItem struct {
	// 概况
	BillPeriodStart  string  `json:"bill_period_start"`  // 账单周期起 2026-08-01
	BillPeriodEnd    string  `json:"bill_period_end"`    // 账单周期止 2026-08-31
	AccountNo        string  `json:"account_no"`         // 户号
	AccountName      string  `json:"account_name"`       // 户名
	Address          string  `json:"address"`            // 用电地址
	UsageCategory    string  `json:"usage_category"`     // 用电类别 大工业用电
	VoltageLevel     string  `json:"voltage_level"`      // 电压等级 交流10kV
	SupplyUnit       string  `json:"supply_unit"`        // 供电服务单位
	MarketAttr       string  `json:"market_attr"`        // 市场化属性
	PrintDate        string  `json:"print_date"`         // 账单打印日期
	TotalKwh         float64 `json:"total_kwh"`          // 本期电量 千瓦时
	TotalAmount      float64 `json:"total_amount"`       // 本期电费 元
	PrevKwh          float64 `json:"prev_kwh"`           // 上期电量
	MomChange        string  `json:"mom_change"`         // 环比 -15.18%
	AvgPrice         float64 `json:"avg_price"`          // 平均电价 元/千瓦时
	PowerFactor      float64 `json:"power_factor"`       // 功率因数
	DueDate          string  `json:"due_date"`           // 交费截止日期
	IndustrialAmount float64 `json:"industrial_amount"`  // 工商业电费
	ResidentialAmount float64 `json:"residential_amount"` // 居民电费
	PfAdjustAmount   float64 `json:"pf_adjust_amount"`   // 功率因数调整电费
	GrandTotal       float64 `json:"grand_total"`        // 合计
	// 电量明细
	DeepPeakKwh float64 `json:"deep_peak_kwh"` // 尖峰电量
	PeakKwh     float64 `json:"peak_kwh"`      // 峰电量
	FlatKwh     float64 `json:"flat_kwh"`      // 平电量
	ValleyKwh   float64 `json:"valley_kwh"`    // 谷电量
	ReactiveKwh float64 `json:"reactive_kwh"`  // 正向无功电量
	// 容需量
	Capacity      float64 `json:"capacity"`       // 容量 kVA
	CapacityPrice float64 `json:"capacity_price"` // 容量电价
	CapacityFee   float64 `json:"capacity_fee"`   // 输配容量电费
	Demand        float64 `json:"demand"`         // 需量值
	PfStandard    float64 `json:"pf_standard"`    // 功率因数标准
	AdjustRatio   float64 `json:"adjust_ratio"`   // 调整系数
	// 费用明细（可编辑，总账联动）
	FeeItems []UtilityBillFeeItem `json:"fee_items"`
	Remark   string               `json:"remark"`
}

// UtilityBillFeeItem 费用明细行。
type UtilityBillFeeItem struct {
	Category  string  `json:"category"`   // 费用类别 (1)市场化购电费
	Name      string  `json:"name"`       // 费用组成 零售交易电费
	Period    string  `json:"period"`     // 分时时段 尖峰/峰/平/谷
	Qty       float64 `json:"qty"`        // 计费电量
	Rate      float64 `json:"rate"`       // 计费标准
	Fee       float64 `json:"fee"`        // 电费
	FeeAmount float64 `json:"fee_amount"` // 电费（别名，兼容）
}

// UtilityBillRecord 电费账单列表行（聚合 KB custom_metadata 生成）。
type UtilityBillRecord struct {
	RowKey         string                      `json:"row_key"`
	KnowledgeID    string                      `json:"knowledge_id"`
	KnowledgeTitle string                      `json:"knowledge_title"`
	FileName       string                      `json:"file_name"`
	FileType       string                      `json:"file_type"`
	Tags           []string                    `json:"tags"`
	ExtractStatus  string                      `json:"extract_status"`
	ExtractError   string                      `json:"extract_error"`
	KBID           string                      `json:"kb_id"`
	CreatedAt      time.Time                   `json:"created_at"`
	Item           UtilityBillExtractionItem   `json:"item"`
}

// UtilityBillListFilter 电费账单列表筛选。
type UtilityBillListFilter struct {
	Keyword  string `json:"keyword" form:"q"`
	DateFrom string `json:"date_from" form:"date_from"`
	DateTo   string `json:"date_to" form:"date_to"`
	Status   string `json:"status" form:"status"`
	Page     int    `json:"page" form:"page"`
	PageSize int    `json:"page_size" form:"page_size"`
}

// UtilityBillListResult 电费账单列表分页结果。
type UtilityBillListResult struct {
	Data     []UtilityBillRecord `json:"data"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}

// UtilityTariffRule 分时电价规则：按月份配置尖峰平谷单价，
// 零售交易电费 = 时段电量 × 对应时段单价（不同月份可不同计价方式）。
type UtilityTariffRule struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	TenantID     int64      `gorm:"index" json:"tenant_id"`
	Category     string     `gorm:"index" json:"category"` // electricity | water | gas
	Name         string     `json:"name"`                  // 规则名称，如「7-8月尖峰分时」
	Months       string     `json:"months"`                // 适用月份，逗号分隔 "7,8"；空串表示不限定
	DeepPeakRate float64    `gorm:"numeric(18,4)" json:"deep_peak_rate"`
	PeakRate     float64    `gorm:"numeric(18,4)" json:"peak_rate"`
	FlatRate     float64    `gorm:"numeric(18,4)" json:"flat_rate"`
	ValleyRate   float64    `gorm:"numeric(18,4)" json:"valley_rate"`
	IsDefault    bool       `gorm:"column:is_default" json:"is_default"` // 无匹配月份时兜底
	SortOrder    int        `json:"sort_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `gorm:"index" json:"deleted_at"`
}

func (UtilityTariffRule) TableName() string { return "utility_tariff_rules" }

// UtilityBasicInfo 电费基本户信息：按租户+分类各一份（全局配置），
// 账单概览-基础信息直接读取此处，不再依赖每次解析提取。
type UtilityBasicInfo struct {
	TenantID      int64     `gorm:"primaryKey" json:"tenant_id"`
	Category      string    `gorm:"primaryKey" json:"category"` // electricity
	AccountNo     string    `json:"account_no"`                 // 户号
	AccountName   string    `json:"account_name"`               // 户名
	UsageCategory string    `json:"usage_category"`             // 用电类别
	VoltageLevel  string    `json:"voltage_level"`              // 电压等级
	MarketAttr    string    `json:"market_attr"`                // 市场化属性
	SupplyUnit    string    `json:"supply_unit"`                // 供电服务单位
	Address       string    `json:"address"`                    // 用电地址
	UpdatedAt     time.Time `json:"updated_at"`
}

func (UtilityBasicInfo) TableName() string { return "utility_basic_info" }
