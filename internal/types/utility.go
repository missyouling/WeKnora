package types

import (
	"time"
)

// UtilityFieldConfig 水电气字段配置：电费/水费/气费三个分类各自的字段集，
// 列表列、字段选择器、抽屉表单均由该配置驱动（字段可增删改、自定义）。
// Group 用于电费按费用分组管理（market/line/trans/sys/gov-industrial/catalog/gov-residential/capacity/pf/meter/resident-meter/overview），
// 每组字段对应一个费用菜单的列定义，设置后自动同步到对应菜单。
type UtilityFieldConfig struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	TenantID       int64      `gorm:"index" json:"tenant_id"`
	Category       string     `gorm:"index" json:"category"` // electricity | water | gas
	Group          string     `gorm:"index" json:"group"`    // 电费分组 key，水气为空串
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
// MeterID 关联 UtilityMeter（水表/气表配置），别名/倍率取自配置。
type UtilityMeterItem struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	RecordID     string    `gorm:"index" json:"record_id"`
	MeterID      string    `gorm:"index" json:"meter_id"`
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

// UtilityMeter 水表/气表配置：别名、表号、倍率、默认单价等基本参数。
// Enabled=false 时列表水表筛选中隐藏，已引用该表的记录不受影响。
type UtilityMeter struct {
	ID               string     `gorm:"primaryKey" json:"id"`
	TenantID         int64      `gorm:"index" json:"tenant_id"`
	Category         string     `gorm:"index" json:"category"` // water | gas
	Alias            string     `json:"alias"`
	MeterNo          string     `json:"meter_no"`
	Rate             float64    `gorm:"numeric(12,4)" json:"rate"`
	DefaultUnitPrice float64    `gorm:"numeric(18,4)" json:"default_unit_price"`
	UseUnit          string     `json:"use_unit"`
	Manager          string     `json:"manager"`
	Contact          string     `json:"contact"`
	MeterMode        string     `json:"meter_mode"` // auto | manual
	InstallDate      string     `json:"install_date"`
	Remark           string     `gorm:"type:text" json:"remark"`
	Enabled          bool       `json:"enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `gorm:"index" json:"deleted_at"`
}

func (UtilityMeter) TableName() string { return "utility_meters" }

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
	// 电量明细表（账单原表逐行：示数类型/上期/本期/倍率/抄见/变损/线损/加减/计费电量）
	MeterReadings []UtilityMeterReading `json:"meter_readings"`
	// 容需量
	Capacity      float64 `json:"capacity"`       // 容量 kVA
	CapacityPrice float64 `json:"capacity_price"` // 容量电价
	CapacityFee   float64 `json:"capacity_fee"`   // 输配容量电费
	Demand        float64 `json:"demand"`         // 需量值
	PfStandard    float64 `json:"pf_standard"`    // 功率因数标准
	AdjustRatio   float64 `json:"adjust_ratio"`   // 调整系数
	// 明细（账单原表）：居民电量明细 / 输配容（需）量 / 功率因素调整
	ResidentialReadings []UtilityResidentialReading `json:"residential_readings"` // 居民电量明细（项目/本期电量/比例/加减/计费电量）
	CapacityDetail      *UtilityCapacityDetail       `json:"capacity_detail"`     // 输配容（需）量电费明细
	PfDetail            *UtilityPfAdjustDetail       `json:"pf_detail"`           // 功率因素调整电费明细
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
	Fee       float64 `json:"fee"`        // 电费（按 电量×标准/分时规则 计算值）
	BillFee   float64 `json:"fee_amount"` // 账单标称电费（账单原值，提取保留，不被重算覆盖）
}

// UtilityMeterReading 电量明细行（与账单「电量明细」表一致）：
// 示数类型/上期示数/本期示数/倍率/抄见电量/变损/线损/加减/计费电量。
type UtilityMeterReading struct {
	MeterType  string  `json:"meter_type"`  // 示数类型 正向有功（总）/正向有功（尖峰）/正向有功（峰）/正向有功（平）/正向有功（谷）/正向无功（总）
	Prev       float64 `json:"prev"`        // 上期示数
	Curr       float64 `json:"curr"`        // 本期示数
	Multiplier float64 `json:"multiplier"`  // 倍率
	ReadingKwh float64 `json:"reading_kwh"` // 抄见电量
	TransLoss  float64 `json:"trans_loss"`  // 变损
	LineLoss   float64 `json:"line_loss"`   // 线损
	Adjust     float64 `json:"adjust"`      // 加减
	BillKwh    float64 `json:"bill_kwh"`    // 计费电量
}

// UtilityResidentialReading 居民电量明细行：项目/本期电量/比例/加减/计费电量，
// 计费电量 = 本期电量 × 比例 + 加减。
type UtilityResidentialReading struct {
	Project string  `json:"project"`  // 项目 居民目录电量等
	Kwh     float64 `json:"kwh"`      // 本期电量
	Ratio   float64 `json:"ratio"`    // 比例
	Adjust  float64 `json:"adjust"`   // 加减
	BillKwh float64 `json:"bill_kwh"` // 计费电量（= Kwh×Ratio+Adjust）
}

// UtilityCapacityDetail 输配容（需）量电费明细（账单原表，单行）。
type UtilityCapacityDetail struct {
	Demand       float64 `json:"demand"`              // 需量值 kW
	DemandPrice  float64 `json:"demand_price"`        // 需量电价 元/kW
	DemandFee    float64 `json:"demand_fee"`          // 输配需量电费（= 需量值×需量电价）
	KwhPerKva    float64 `json:"kwh_per_kva"`         // 月每千伏安用电量 kWh/kVA
	DiscountFee  float64 `json:"discount_demand_fee"` // 折扣需量电费
	Capacity     float64 `json:"capacity"`            // 容量 kVA
	CapacityPrice float64 `json:"capacity_price"`     // 容量电价 元/kVA
	CapacityFee  float64 `json:"capacity_fee"`        // 输配容量电费（= 容量×容量电价）
}

// UtilityPfAdjustDetail 功率因素调整电费明细（账单原表，单行）。
type UtilityPfAdjustDetail struct {
	Project     string  `json:"project"`      // 项目 功率因数调整电费
	PowerFactor float64 `json:"power_factor"` // 功率因素实际值
	Standard    float64 `json:"pf_standard"`  // 功率因素标准
	AdjustRatio float64 `json:"adjust_ratio"` // 调整系数
	ActiveKwh   float64 `json:"pf_active_kwh"`   // 参与调整有功电量
	ReactiveKwh float64 `json:"pf_reactive_kwh"` // 参与调整无功电量
	FeeBase     float64 `json:"pf_fee_base"`     // 参与调整电费金额
	AdjustFee   float64 `json:"adjust_fee"`      // 功率因素调整电费
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

// SolarBillRecord 光伏账单列表行（聚合 KB custom_metadata 生成；与电费共库，kind=solar_bill 区分）。
type SolarBillRecord struct {
	RowKey         string                    `json:"row_key"`
	KnowledgeID    string                    `json:"knowledge_id"`
	KnowledgeTitle string                    `json:"knowledge_title"`
	FileName       string                    `json:"file_name"`
	FileType       string                    `json:"file_type"`
	Tags           []string                  `json:"tags"`
	ExtractStatus  string                    `json:"extract_status"`
	ExtractError   string                    `json:"extract_error"`
	KBID           string                    `json:"kb_id"`
	CreatedAt      time.Time                 `json:"created_at"`
	Item           SolarBillExtractionItem   `json:"item"`
}

// SolarBillExtractionItem 光伏发电账单提取字段（国网光伏账单：基础信息/发电量/上网关口/发电关口明细）。
type SolarBillExtractionItem struct {
	// 基础信息
	BillPeriodStart string `json:"bill_period_start"` // 账单周期起 2026-08-01
	BillPeriodEnd   string `json:"bill_period_end"`   // 账单周期止 2026-08-31
	AccountNo       string `json:"account_no"`        // 发电户号
	AccountName     string `json:"account_name"`      // 户名
	SupplyUnit      string `json:"supply_unit"`       // 服务单位
	Address         string `json:"address"`           // 发电地址
	TaxpayerType    string `json:"taxpayer_type"`     // 纳税人类型
	ConsumptionMode string `json:"consumption_mode"`  // 消纳方式 自发自用余电上网
	VoltageLevel    string `json:"voltage_level"`     // 并网电压等级 交流380V
	GenerationMode  string `json:"generation_mode"`   // 发电方式 光伏发电
	// 概览
	GenerationKwh   float64 `json:"generation_kwh"`   // 发电量 千瓦时
	GridKwh         float64 `json:"grid_kwh"`         // 上网电量 千瓦时
	SettlementAmount float64 `json:"settlement_amount"` // 结算金额 元
	MomChange       string  `json:"mom_change"`       // 本期上网电量环比 +99.13%
	CumulativeKwh   float64 `json:"cumulative_kwh"`   // 年累计上网电量 千瓦时
	TaxRate         string  `json:"tax_rate"`         // 税率 13%
	TaxAmount       float64 `json:"tax_amount"`       // 税额 元
	// 关口明细（1 个上网关口 + N 个发电关口）
	Gateways []SolarGateway `json:"gateways"`
	Remark   string         `json:"remark"`
}

// SolarGateway 光伏账单关口明细（上网关口 / 发电关口）。
type SolarGateway struct {
	GatewayType string          `json:"gateway_type"` // 上网关口 | 发电关口
	MeterNo     string          `json:"meter_no"`     // 电能表编号
	ProjectName string          `json:"project_name"` // 项目名称（如 1.4MW 屋顶分布式光伏发电项目）
	Readings    []SolarReading  `json:"readings"`     // 电量明细（示数类型/上期/本期/倍率/抄见/计费）
	Fees        []SolarFeeItem  `json:"fees"`         // 电费明细（类别/电量/电价/电费）
}

// SolarReading 光伏电量明细行（与账单「本期电量明细」表一致）。
type SolarReading struct {
	MeterType  string  `json:"meter_type"`  // 示数类型 反向有功（总）
	Prev       float64 `json:"prev"`        // 上期示数
	Curr       float64 `json:"curr"`        // 本期示数
	Multiplier float64 `json:"multiplier"`  // 倍率
	ReadingKwh float64 `json:"reading_kwh"` // 抄见电量
	BillKwh    float64 `json:"bill_kwh"`    // 计费电量
}

// SolarFeeItem 光伏电费明细行（与账单「本期电费明细」表一致）。
type SolarFeeItem struct {
	Category string  `json:"category"` // 类别 分布式上网电费 | 分布式电源发电补助
	Qty      float64 `json:"qty"`      // 电量
	Rate     float64 `json:"rate"`     // 电价
	Fee      float64 `json:"fee"`      // 电费（= 电量×电价）
}

// SolarBillListResult 光伏账单列表分页结果。
type SolarBillListResult struct {
	Data     []SolarBillRecord `json:"data"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// UtilityBillListFilter 电费/光伏账单列表筛选。
type UtilityBillListFilter struct {
	Keyword  string `json:"keyword" form:"q"`
	DateFrom string `json:"date_from" form:"date_from"`
	DateTo   string `json:"date_to" form:"date_to"`
	Status   string `json:"status" form:"status"`
	Kind     string `json:"kind" form:"kind"` // utility_bill | solar_bill，默认 utility_bill
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

// UtilityBasicAccount 电费基本户（多户）：每户一条，可自定义名称，
// 账单解析后按户号自动匹配；倍率用于工商业电量明细倍率引用，电能表编号仅档案展示。
type UtilityBasicAccount struct {
	ID            string     `gorm:"primaryKey" json:"id"`
	TenantID      int64      `gorm:"index" json:"tenant_id"`
	Category      string     `gorm:"index" json:"category"` // electricity
	Name          string     `json:"name"`                  // 自定义名称（卡片标题）
	AccountNo     string     `json:"account_no"`            // 户号
	AccountName   string     `json:"account_name"`          // 户名
	UsageCategory string     `json:"usage_category"`        // 用电类别
	VoltageLevel  string     `json:"voltage_level"`         // 电压等级
	MarketAttr    string     `json:"market_attr"`           // 市场化属性
	SupplyUnit    string     `json:"supply_unit"`           // 供电服务单位
	Address       string     `json:"address"`               // 用电地址
	MeterNo       string     `json:"meter_no"`              // 电能表编号
	Ratio         float64    `gorm:"numeric(18,4)" json:"ratio"` // 倍率
	IsDefault     bool       `gorm:"column:is_default" json:"is_default"` // 默认户（兜底）
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     *time.Time `gorm:"index" json:"deleted_at"`
}

func (UtilityBasicAccount) TableName() string { return "utility_basic_accounts" }
