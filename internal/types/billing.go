package types

import "time"

// BillingTenant 费用核算租户（如持睿汽车）：电费按市电账单比例分摊 + 宿舍定额 + 水费按表计。
type BillingTenant struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	TenantID  int64      `gorm:"index" json:"tenant_id"`
	Name      string     `json:"name"`
	Remark    string     `gorm:"type:text" json:"remark"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at"`
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
	MeterType       string     `json:"meter_type"` // star | sub
	Name            string     `json:"name"`
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

// BillingTenantItem 分摊子项开关（市电账单各费用子项是否参与分摊）。
type BillingTenantItem struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	BillingTenantID string    `gorm:"index" json:"billing_tenant_id"`
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
