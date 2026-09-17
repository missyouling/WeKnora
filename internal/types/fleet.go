package types

import "time"

// 车队管理记录类型
const (
	FleetRecordMaintain   = "maintain"      // 维保记录
	FleetRecordFuel       = "fuel"          // 加油记录
	FleetRecordInsurance  = "insurance"     // 车险记录
	FleetRecordTire       = "tire"          // 轮胎记录
	FleetRecordMaterial   = "material"      // 辅材记录（尿素、篷布等）
	FleetRecordViolation  = "violation"     // 违章记录
	FleetRecordCarRequest = "car-request"   // 请车记录
	FleetRecordToll       = "toll"          // 通行费
)

// FleetVehicle 车辆配置
type FleetVehicle struct {
	ID           string     `gorm:"primaryKey" json:"id"`
	TenantID     int64      `gorm:"index" json:"tenant_id"`
	PlateNo      string     `json:"plate_no"`      // 车牌号
	VehicleType  string     `json:"vehicle_type"`  // 车辆类型：轿车/货车/客车/皮卡/面包车/其它
	BrandModel   string     `json:"brand_model"`   // 品牌型号
	LoadTonnage  float64    `gorm:"numeric(12,2)" json:"load_tonnage"` // 吨位
	SeatCount    int        `json:"seat_count"`    // 座位数
	PurchaseDate string     `json:"purchase_date"` // 购置日期 YYYY-MM-DD
	Department   string     `json:"department"`    // 使用部门
	Manager      string     `json:"manager"`       // 责任人
	SortOrder    int        `json:"sort_order"`    // 拖拽排序，越小越靠前
	Enabled      bool       `json:"enabled"`
	Remark       string     `gorm:"type:text" json:"remark"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetVehicle) TableName() string { return "fleet_vehicles" }

// FleetDriver 驾驶员配置
type FleetDriver struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	TenantID   int64      `gorm:"index" json:"tenant_id"`
	Name       string     `json:"name"`        // 姓名
	LicenseNo  string     `json:"license_no"`  // 驾驶证号
	LicenseType string    `json:"license_type"` // 准驾车型
	Phone      string     `json:"phone"`       // 联系电话
	HireDate   string     `json:"hire_date"`   // 入职日期 YYYY-MM-DD
	SortOrder  int        `json:"sort_order"`
	Enabled    bool       `json:"enabled"`
	Remark     string     `gorm:"type:text" json:"remark"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetDriver) TableName() string { return "fleet_drivers" }

// FleetFuelCard 油卡配置
type FleetFuelCard struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	TenantID   int64      `gorm:"index" json:"tenant_id"`
	CardNo     string     `json:"card_no"`     // 卡号
	VehicleID  string     `json:"vehicle_id"`  // 所属车辆
	DriverID   string     `json:"driver_id"`   // 所属驾驶员
	Station    string     `json:"station"`     // 油站
	FaceValue  float64    `gorm:"numeric(18,2)" json:"face_value"` // 面额
	Balance    float64    `gorm:"numeric(18,2)" json:"balance"`    // 余额
	SortOrder  int        `json:"sort_order"`
	Enabled    bool       `json:"enabled"`
	Remark     string     `gorm:"type:text" json:"remark"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetFuelCard) TableName() string { return "fleet_fuel_cards" }

// FleetRecord 车队通用记录：8 类记录共用一张表，
// 通用列（vehicle/date/month/amount/mileage/remark）+ data(JSONB) 存放类型专属字段。
type FleetRecord struct {
	ID          string         `gorm:"primaryKey" json:"id"`
	TenantID    int64          `gorm:"index" json:"tenant_id"`
	RecordType  string         `gorm:"index" json:"record_type"` // maintain|fuel|insurance|tire|material|violation|car-request|toll
	VehicleID   string         `gorm:"index" json:"vehicle_id"`  // 关联 FleetVehicle
	RecordMonth string         `gorm:"index" json:"record_month"` // YYYY-MM（筛选/汇总用）
	RecordDate  string         `json:"record_date"`               // YYYY-MM-DD
	Amount      float64        `gorm:"numeric(18,2)" json:"amount"` // 金额（通用列，费用清单汇总用）
	Mileage     float64        `gorm:"numeric(18,2)" json:"mileage"` // 里程（通用列）
	Data        map[string]any `gorm:"type:jsonb;serializer:json" json:"data"` // 类型专属字段
	Remark      string         `gorm:"type:text" json:"remark"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   *time.Time     `gorm:"index" json:"deleted_at"`
}

func (FleetRecord) TableName() string { return "fleet_records" }
