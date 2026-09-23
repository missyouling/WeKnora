package types

import "time"

// 车队管理记录类型
const (
	// 档案类（上传解析入知识库）
	FleetRecordVehicleArchive = "vehicle-archive" // 车辆档案（登记证书/行驶证/营运证/道路运输证/保险单/驾驶证/从业资格证）
	FleetRecordDriverArchive  = "driver-archive"  // 司机档案（驾驶证/从业资格证）
	FleetRecordMaintainArchive = "maintain-archive" // 维保管理（维修合同/维修工单/二级维护/保险单）
	// 手动/成本类
	FleetRecordTire         = "tire"           // 轮胎管理
	FleetRecordInspection   = "inspection"     // 年检管理
	FleetRecordFuelCharge   = "fuel-charge"    // 加油充电
	FleetRecordRoadToll     = "road-toll"      // 路桥费
	FleetRecordRepairCost   = "repair-cost"    // 维修保养费
	FleetRecordInsuranceClaim = "insurance-claim" // 保险理赔
	FleetRecordViolation    = "violation"      // 违章处理
	FleetRecordMaterial     = "material"       // 辅材记录（尿素、篷布等）
	FleetRecordCarRequest   = "car-request"    // 请车记录
	// 历史类型（v1 保留，新 UI 不再展示：maintain/fuel/insurance/toll）
	FleetRecordMaintain  = "maintain"  // 历史维保记录
	FleetRecordFuel      = "fuel"      // 历史加油记录
	FleetRecordInsurance = "insurance" // 历史车险记录
	FleetRecordToll      = "toll"      // 历史通行费
)

// 档案分类 scope
const (
	FleetCategoryScopeVehicle  = "vehicle"
	FleetCategoryScopeDriver   = "driver"
	FleetCategoryScopeMaintain = "maintain"
)

// FleetVehicle 车辆配置（历史保留）
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

// FleetDriver 驾驶员配置（历史保留）
type FleetDriver struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	TenantID    int64      `gorm:"index" json:"tenant_id"`
	Name        string     `json:"name"`         // 姓名
	LicenseNo   string     `json:"license_no"`   // 驾驶证号
	LicenseType string     `json:"license_type"` // 准驾车型
	Phone       string     `json:"phone"`        // 联系电话
	HireDate    string     `json:"hire_date"`    // 入职日期 YYYY-MM-DD
	SortOrder   int        `json:"sort_order"`
	Enabled     bool       `json:"enabled"`
	Remark      string     `gorm:"type:text" json:"remark"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetDriver) TableName() string { return "fleet_drivers" }

// FleetFuelCard 油卡配置
type FleetFuelCard struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	TenantID   int64      `gorm:"index" json:"tenant_id"`
	Alias      string     `json:"alias"`       // 油卡别名
	CardNo     string     `json:"card_no"`     // 卡号
	CardType   string     `json:"card_type"`   // 卡类型：主卡/副卡/子卡/绑定卡
	Brand      string     `json:"brand"`       // 油卡品牌：中石化/中石油/壳牌/民营等
	VehicleID  string     `json:"vehicle_id"`  // 绑定车辆
	DriverID   string     `json:"driver_id"`   // 绑定司机
	CardStatus string     `json:"card_status"` // 卡状态：正常/挂失/冻结/注销/过期
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

// FleetRecord 车队通用记录：各类型共用一张表，
// 通用列（vehicle/date/month/amount/mileage/remark）+ data(JSONB) 存放类型专属字段；档案类使用 doc_type/file_name/doc_knowledge_id。
type FleetRecord struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	TenantID       int64          `gorm:"index" json:"tenant_id"`
	RecordType     string         `gorm:"index" json:"record_type"` // 见常量
	VehicleID      string         `gorm:"index" json:"vehicle_id"`  // 关联车辆（车牌号来源：档案解析或手动配置）
	RecordMonth    string         `gorm:"index" json:"record_month"` // YYYY-MM（筛选/汇总用）
	RecordDate     string         `json:"record_date"`               // YYYY-MM-DD
	Amount         float64        `gorm:"numeric(18,2)" json:"amount"` // 金额（通用列，费用清单汇总用）
	Mileage        float64        `gorm:"numeric(18,2)" json:"mileage"` // 里程（通用列）
	Data           map[string]any `gorm:"type:jsonb;serializer:json" json:"data"` // 类型专属字段
	DocType        string         `gorm:"index" json:"doc_type"`    // 证照类型（档案类）
	FileName       string         `json:"file_name"`                // 源文件名（档案类）
	DocKnowledgeID string         `json:"doc_knowledge_id"`         // 关联知识库文件 ID（档案类，用于源文件打印）
	Remark         string         `gorm:"type:text" json:"remark"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      *time.Time     `gorm:"index" json:"deleted_at"`
}

func (FleetRecord) TableName() string { return "fleet_records" }

// FleetCategorySub 档案分类小项（大项=分类行，小项=解析提取的字段，可启/禁用）
type FleetCategorySub struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"is_default"` // 字段筛选器「重置」时默认勾选
	DataType  string `json:"data_type"`  // text | number | date | array（影响提取骨架与编辑控件）
}

// FleetCategory 档案分类配置：scope=vehicle|driver|maintain
type FleetCategory struct {
	ID        string             `gorm:"primaryKey" json:"id"`
	TenantID  int64              `gorm:"index" json:"tenant_id"`
	Scope     string             `gorm:"index" json:"scope"`
	GroupID   string             `gorm:"index" json:"group_id"` // 自定义证照分组 id（fleet_cert_groups.id）；内置分组为空
	Name      string             `json:"name"`                  // 大项：如“行驶证”“驾驶证”
	Subs      []FleetCategorySub `gorm:"type:jsonb;serializer:json" json:"subs"`
	SortOrder int                `json:"sort_order"`
	Enabled   bool               `json:"enabled"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	DeletedAt *time.Time         `gorm:"index" json:"deleted_at"`
}

func (FleetCategory) TableName() string { return "fleet_categories" }

// FleetCertGroup 证照自定义分组（公司证照/司机证照内置，用户可新增自定义分组）
type FleetCertGroup struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	TenantID    int64      `gorm:"index" json:"tenant_id"`
	ParentScope string     `gorm:"index" json:"parent_scope"` // 归属：vehicle/driver/maintain
	Name        string     `json:"name"`
	SortOrder   int        `json:"sort_order"`
	Builtin     bool       `json:"builtin"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetCertGroup) TableName() string { return "fleet_cert_groups" }

// FleetSupplier 供应商
type FleetSupplier struct {
	ID                string     `gorm:"primaryKey" json:"id"`
	TenantID          int64      `gorm:"index" json:"tenant_id"`
	Name              string     `json:"name"`               // 供应商名称
	SupplierType      string     `json:"supplier_type"`      // 类型：维修厂/配件商/轮胎商/油品商/保险公司/年检代办/洗车/救援/租赁等
	Qualification     string     `json:"qualification"`      // 资质等级：一类维修/二类维修
	CreditCode        string     `json:"credit_code"`        // 统一社会信用代码
	LegalPerson       string     `json:"legal_person"`       // 法定代表人
	Contact           string     `json:"contact"`            // 联系人
	Phone             string     `json:"phone"`              // 联系电话
	Address           string     `json:"address"`            // 联系地址
	CooperationStatus string     `json:"cooperation_status"` // 合作状态：潜在/合作中/暂停/终止
	CoopStartDate     string     `json:"coop_start_date"`    // 合作开始日期
	SettleMethod      string     `json:"settle_method"`      // 结算方式：月结/现结/季度结
	TaxRate           string     `gorm:"type:varchar(20)" json:"tax_rate"` // 税率（如 13% 或 0.13）
	InvoiceType       string     `json:"invoice_type"`       // 发票类型：增值税专票/普票
	Status            string     `json:"status"`             // 供应商状态：正常/停用/黑名单
	Enabled           bool       `json:"enabled"`
	SortOrder         int        `json:"sort_order"`
	Remark            string     `gorm:"type:text" json:"remark"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetSupplier) TableName() string { return "fleet_suppliers" }

// FleetETCCard ETC 卡
type FleetETCCard struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	TenantID   int64      `gorm:"index" json:"tenant_id"`
	Alias      string     `json:"alias"`       // 别名
	CardNo     string     `json:"card_no"`     // ETC卡号
	CardType   string     `json:"card_type"`   // 卡类型
	Issuer     string     `json:"issuer"`      // 发卡方
	Bank       string     `json:"bank"`        // 开户行
	OpenDate   string     `json:"open_date"`   // 开户日期
	ExpireDate string     `json:"expire_date"` // 有效期
	VehicleID  string     `json:"vehicle_id"`  // 车牌号
	CardStatus string     `json:"card_status"` // 卡状态
	SortOrder  int        `json:"sort_order"`
	Enabled    bool       `json:"enabled"`
	Remark     string     `gorm:"type:text" json:"remark"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `gorm:"index" json:"deleted_at"`
}

func (FleetETCCard) TableName() string { return "fleet_etc_cards" }
