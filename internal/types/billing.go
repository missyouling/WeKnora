package types

import "time"

// BillingTenant 费用核算租户（如持睿汽车）：电费按市电账单比例分摊 + 宿舍定额 + 水费按表计。
type BillingTenant struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	TenantID       int64      `gorm:"index" json:"tenant_id"`
	Name           string     `json:"name"`
	TenantNo       string     `json:"tenant_no"`       // 租户编号
	AllocationMode string     `json:"allocation_mode"` // 分摊方式(默认按比例分摊)
	LeaseStart     *string    `gorm:"type:date" json:"lease_start"` // 租赁日期
	LeaseYears     int        `json:"lease_years"`     // 租赁年限
	Contact        string     `json:"contact"`         // 单位联系人
	Phone          string     `json:"phone"`           // 联系电话
	Remark         string     `gorm:"type:text" json:"remark"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `gorm:"index" json:"deleted_at"`
}

func (BillingTenant) TableName() string { return "billing_tenants" }

// BillingTenantSetting 租户核算参数。
type BillingTenantSetting struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	BillingTenantID string    `gorm:"index" json:"billing_tenant_id"`
	DormPrice       float64   `gorm:"numeric(18,4)" json:"dorm_price"` // 宿舍电价 元/度，默认 1
	WaterPrice      float64   `gorm:"numeric(18,4)" json:"water_price"` // 水价 元/吨，默认 5.22
	BillKBID        string    `json:"bill_kb_id"`                       // 市电账单知识库 ID
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (BillingTenantSetting) TableName() string { return "billing_tenant_settings" }

// BillingTimeMeter 分时电表（星达分表 star / 持睿工业分表 sub），按尖峰平谷四时段录入。
type BillingTimeMeter struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	BillingTenantID string     `gorm:"index" json:"billing_tenant_id"`
	MeterType       string     `json:"meter_type"` // 兼容旧数据 star | sub,新逻辑按归属单位分组
	Name            string     `json:"name"`       // 别名
	MeterNo         string     `json:"meter_no"`   // 表号
	MeterKind       string     `json:"meter_kind"` // time | normal 分时/普通
	OwnerUnit       string     `json:"owner_unit"` // 归属单位(动态分组)
	UseUnit         string     `json:"use_unit"`   // 使用单位
	Manager         string     `json:"manager"`    // 管理人员
	Contact         string     `json:"contact"`    // 联系方式
	MeterMode       string     `json:"meter_mode"` // auto | manual 抄表方式
	InstallDate     *string    `gorm:"type:date" json:"install_date"`
	Remark          string     `gorm:"type:text" json:"remark"`
	Rate            float64    `gorm:"numeric(12,4)" json:"rate"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `gorm:"index" json:"deleted_at"`
}

func (BillingTimeMeter) TableName() string { return "billing_time_meters" }

// BillingTimeReading 分时电表月度读数（尖峰平谷四时段起止度）。
type BillingTimeReading struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	MeterID    string    `gorm:"index" json:"meter_id"`
	Month      string    `json:"month"` // YYYY-MM
	DeepPrev   float64   `gorm:"numeric(18,2)" json:"deep_prev"`
	DeepCurr   float64   `gorm:"numeric(18,2)" json:"deep_curr"`
	PeakPrev   float64   `gorm:"numeric(18,2)" json:"peak_prev"`
	PeakCurr   float64   `gorm:"numeric(18,2)" json:"peak_curr"`
	FlatPrev   float64   `gorm:"numeric(18,2)" json:"flat_prev"`
	FlatCurr   float64   `gorm:"numeric(18,2)" json:"flat_curr"`
	ValleyPrev float64   `gorm:"numeric(18,2)" json:"valley_prev"`
	ValleyCurr float64   `gorm:"numeric(18,2)" json:"valley_curr"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (BillingTimeReading) TableName() string { return "billing_time_meter_readings" }

// BillingWaterMeter 水表(总表/工业/宿舍/消防),按归属单位动态分组,默认单价用于水费计算。
type BillingWaterMeter struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	BillingTenantID string     `gorm:"index" json:"billing_tenant_id"`
	Name            string     `json:"name"`       // 别名
	MeterNo         string     `json:"meter_no"`   // 表号
	MeterKind       string     `json:"meter_kind"` // total | sub | fire 总表/分表/消防
	OwnerUnit       string     `json:"owner_unit"` // 归属单位(动态分组)
	UseUnit         string     `json:"use_unit"`   // 使用单位
	Manager         string     `json:"manager"`    // 管理人员
	Contact         string     `json:"contact"`    // 联系方式
	MeterMode       string     `json:"meter_mode"` // auto | manual 抄表方式
	InstallDate     *string    `gorm:"type:date" json:"install_date"`
	Remark          string     `gorm:"type:text" json:"remark"`
	Rate            float64    `gorm:"numeric(12,4)" json:"rate"`
	Price           float64    `gorm:"numeric(18,4)" json:"price"` // 默认单价 元/吨
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `gorm:"index" json:"deleted_at"`
}

func (BillingWaterMeter) TableName() string { return "billing_water_meters" }

// BillingWaterReading 水表月度读数(单起止 + 单价)。
type BillingWaterReading struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	MeterID   string    `gorm:"index" json:"meter_id"`
	Month     string    `json:"month"` // YYYY-MM
	Prev      float64   `gorm:"numeric(18,2)" json:"prev"`
	Curr      float64   `gorm:"numeric(18,2)" json:"curr"`
	Price     float64   `gorm:"numeric(18,4)" json:"price"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (BillingWaterReading) TableName() string { return "billing_water_meter_readings" }

// BillingTenantItem 分摊子项开关（市电账单各费用子项是否参与分摊，按子项名 name 粒度）。
type BillingTenantItem struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	BillingTenantID string    `gorm:"index" json:"billing_tenant_id"`
	Category        string    `json:"category"` // 大类（如 (1)市场化购电费），用于 UI 分组
	ItemKey         string    `json:"item_key"`
	ItemName        string    `json:"item_name"`
	Enabled         bool      `json:"enabled"`
	Sort            int       `json:"sort"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (BillingTenantItem) TableName() string { return "billing_tenant_items" }

// BillingTenantMeterRef 租户引用表计（宿舍电表 category=electricity / 水表 category=water，引用 utility_meters）。
type BillingTenantMeterRef struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	BillingTenantID string    `gorm:"index" json:"billing_tenant_id"`
	Category        string    `json:"category"` // electricity | water
	MeterID         string    `json:"meter_id"`
	MeterName       string    `json:"meter_name"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (BillingTenantMeterRef) TableName() string { return "billing_tenant_meter_refs" }

// BillingRecord 月度账单（生成时固化金额）。
type BillingRecord struct {
	ID              string     `gorm:"primaryKey" json:"id"`
	BillingTenantID string     `gorm:"index" json:"billing_tenant_id"`
	Month           string     `json:"month"`
	Status          string     `json:"status"` // generated | adjusted
	TotalKwh        float64    `gorm:"numeric(18,2)" json:"total_kwh"`        // 星达分表总电量
	LineLoss        float64    `gorm:"numeric(18,2)" json:"line_loss"`        // 线损
	Ratio           float64    `gorm:"numeric(18,6)" json:"ratio"`            // 分摊比例
	BillTotalAmount float64    `gorm:"numeric(18,2)" json:"bill_total_amount"` // 市电本期电费
	DormKwh         float64    `gorm:"numeric(18,2)" json:"dorm_kwh"`         // 宿舍度数
	DormFee         float64    `gorm:"numeric(18,2)" json:"dorm_fee"`         // 宿舍电费
	WaterUsage      float64    `gorm:"numeric(18,2)" json:"water_usage"`      // 水用量
	WaterFee        float64    `gorm:"numeric(18,2)" json:"water_fee"`        // 水费
	IndustrialFee   float64    `gorm:"numeric(18,2)" json:"industrial_fee"`   // 厂区电费
	TotalFee        float64    `gorm:"numeric(18,2)" json:"total_fee"`        // 总应付
	Remark          string     `gorm:"type:text" json:"remark"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `gorm:"index" json:"deleted_at"`
}

func (BillingRecord) TableName() string { return "billing_records" }

// BillingRecordItem 账单子行（固化明细）。
type BillingRecordItem struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	RecordID  string    `gorm:"index" json:"record_id"`
	Kind      string    `json:"kind"` // fee | base | pf | dorm | water
	Name      string    `json:"name"`
	Period    string    `json:"period"` // 尖峰/峰/平/谷
	Qty       float64   `gorm:"numeric(18,2)" json:"qty"`
	Rate      float64   `gorm:"numeric(18,6)" json:"rate"`
	Fee       float64   `gorm:"numeric(18,2)" json:"fee"`
	Sort      int       `json:"sort"`
	CreatedAt time.Time `json:"created_at"`
}

func (BillingRecordItem) TableName() string { return "billing_record_items" }
