// 人事三模块（花名册 / 社保 / 公积金）持久化模型，第 45 轮迁移。
//
// 口径：全部 9 张表定义在 handler 包内，由 NewHRHandler 做 best-effort AutoMigrate。
// 源项目用 uint 自增主键 + user_id 做隔离；目标项目统一 string(uuid) 主键 +
// int64 tenant_id（与 types.FleetCategory 同口径）。
// 花名册「30+ 字段」保留为核心列，Excel 未识别的额外列写 custom_data(JSONB)。
// go.mod 未引入 gorm.io/datatypes，故 JSONB 一律用 map[string]any + serializer:json
// （与 types.FleetCategory.Subs 完全相同的写法，PG / sqlite 均可落库）。
package handler

import "time"

// 社保明细维度常量（源项目 models/models.go:12-34 原样移植，用 untyped string 便于直接比较）。
const (
	HRPartPersonal = "personal"
	HRPartUnit     = "unit"

	HRFileTypeNormal     = "normal"
	HRFileTypeAdjustment = "adjustment"

	HRSchemePension        = "pension"
	HRSchemeMedical        = "medical"
	HRSchemeSeriousIllness = "serious_illness"
	HRSchemeUnemployment   = "unemployment"
	HRSchemeInjury         = "injury"
)

// HREmployee 花名册。
type HREmployee struct {
	ID                    string `gorm:"primaryKey" json:"id"`
	TenantID              int64  `gorm:"index;uniqueIndex:uk_hr_employee_tenant_id_number" json:"tenant_id"`
	EmployeeID            string `json:"employee_id"`
	Name                  string `gorm:"index" json:"name"`
	Department            string `json:"department"`
	Position              string `json:"position"`
	Gender                string `json:"gender"`
	HireDate              string `json:"hire_date"`
	Age                   string `json:"age"`
	WorkYears             string `json:"work_years"`
	BirthMonth            string `json:"birth_month"`
	Education             string `json:"education"`
	PoliticalStatus       string `json:"political_status"`
	WorkClothingSize      string `json:"work_clothing_size"`
	SafetyShoeSize        string `json:"safety_shoe_size"`
	HouseholdType         string `json:"household_type"`
	Ethnicity             string `json:"ethnicity"`
	NativePlace           string `json:"native_place"`
	IDAddress             string `json:"id_address"`
	IDNumber              string `gorm:"uniqueIndex:uk_hr_employee_tenant_id_number" json:"id_number"`
	MaritalStatus         string `json:"marital_status"`
	SocialInsurance       string `json:"social_insurance"`
	HasBirth              string `json:"has_birth"`
	Phone                 string `json:"phone"`
	SocialInsuranceNumber string `json:"social_insurance_number"`
	ProvidentFundNumber   string `json:"provident_fund_number"`
	EmergencyContact      string `json:"emergency_contact"`
	EmergencyPhone        string `json:"emergency_phone"`
	CurrentAddress        string `json:"current_address"`
	GraduateSchool        string `json:"graduate_school"`
	Major                 string `json:"major"`
	GraduationTime        string `json:"graduation_time"`
	Email                 string `json:"email"`
	Remarks               string `gorm:"type:text" json:"remarks"`
	Status                string `gorm:"default:active" json:"status"`
	ResignDate            string `json:"resign_date"`
	ResignReasons         string `gorm:"type:text" json:"resign_reasons"`
	// 第 46 轮：办理离职弹窗的证明附件路径与备注说明。
	ResignProofPath string         `json:"resign_proof_path"`
	ResignRemark    string         `gorm:"type:text" json:"resign_remark"`
	CustomData      map[string]any `gorm:"type:jsonb;serializer:json" json:"custom_data"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (HREmployee) TableName() string { return "hr_employees" }

// HRSIPeriod 社保账期。
type HRSIPeriod struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	TenantID         int64          `gorm:"index;uniqueIndex:uk_hr_si_period_tenant_month" json:"tenant_id"`
	YearMonth        string         `gorm:"index;uniqueIndex:uk_hr_si_period_tenant_month" json:"year_month"`
	Status           string         `json:"status"`
	AllowAdjustments bool           `json:"allow_adjustments"`
	SummaryData      map[string]any `gorm:"type:jsonb;serializer:json" json:"summary_data"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (HRSIPeriod) TableName() string { return "hr_si_periods" }

// HRSIRawRecord 五险原始明细（正常文件覆盖 / 补退文件累加）。
type HRSIRawRecord struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	TenantID     int64     `gorm:"index" json:"tenant_id"`
	PeriodID     string    `gorm:"index" json:"period_id"`
	Sequence     int       `json:"sequence"`
	Name         string    `json:"name"`
	IDType       string    `json:"id_type"`
	IDNumber     string    `gorm:"index" json:"id_number"`
	Department   string    `json:"department"`
	PaySalary    float64   `json:"pay_salary"`
	PayBase      float64   `json:"pay_base"`
	RateText     string    `json:"rate_text"`
	AmountDue    float64   `json:"amount_due"`
	AmountAdjust float64   `json:"amount_adjust"`
	PersonCode   string    `json:"person_code"`
	Scheme       string    `gorm:"index" json:"scheme"`
	Part         string    `gorm:"index" json:"part"`
	FileType     string    `gorm:"index" json:"file_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (HRSIRawRecord) TableName() string { return "hr_si_raw_records" }

// HRSICharge 扣款明细（个人 4 险 / 单位 5 险用 part 区分）。
// MedicalMaternity 语义照源：个人=医疗，单位=医疗+大病；SeriousIllness 单列原值。
type HRSICharge struct {
	ID               string    `gorm:"primaryKey" json:"id"`
	TenantID         int64     `gorm:"index" json:"tenant_id"`
	PeriodID         string    `gorm:"index" json:"period_id"`
	Part             string    `gorm:"index" json:"part"`
	Name             string    `json:"name"`
	IDNumber         string    `gorm:"index" json:"id_number"`
	Department       string    `json:"department"`
	Base             float64   `json:"base"`
	Pension          float64   `json:"pension"`
	MedicalMaternity float64   `json:"medical_maternity"`
	SeriousIllness   float64   `json:"serious_illness"`
	Injury           float64   `json:"injury"`
	Unemployment     float64   `json:"unemployment"`
	Subtotal         float64   `json:"subtotal"`
	IsAdjustment     bool      `gorm:"index" json:"is_adjustment"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (HRSICharge) TableName() string { return "hr_si_charges" }

// HRSIChange 增减员。
type HRSIChange struct {
	ID             string         `gorm:"primaryKey" json:"id"`
	TenantID       int64          `gorm:"index" json:"tenant_id"`
	PeriodID       string         `gorm:"index" json:"period_id"`
	ChangeType     string         `gorm:"index" json:"change_type"`
	EmployeeName   string         `json:"employee_name"`
	Department     string         `json:"department"`
	IdentityNumber string         `json:"identity_number"`
	PersonalNumber string         `json:"personal_number"`
	EffectiveDate  string         `json:"effective_date"`
	Reason         string         `json:"reason"`
	TemplateValues map[string]any `gorm:"type:jsonb;serializer:json" json:"template_values"`
	CallbackStatus string         `json:"callback_status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (HRSIChange) TableName() string { return "hr_si_changes" }

// HRPFSettings 单位公积金配置（含默认缴存比例，比例默认 6）。
type HRPFSettings struct {
	ID           string    `gorm:"primaryKey" json:"id"`
	TenantID     int64     `gorm:"index" json:"tenant_id"`
	UnitName     string    `json:"unit_name"`
	UnitAccount  string    `json:"unit_account"`
	DefaultRatio float64   `json:"default_ratio"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (HRPFSettings) TableName() string { return "hr_pf_settings" }

// HRPFRecord 公积金台账。
type HRPFRecord struct {
	ID                string         `gorm:"primaryKey" json:"id"`
	TenantID          int64          `gorm:"index;uniqueIndex:uk_hr_pf_record_tenant_identity" json:"tenant_id"`
	PersonalAccount   string         `json:"personal_account"`
	Name              string         `json:"name"`
	IdentityNumber    string         `gorm:"index;uniqueIndex:uk_hr_pf_record_tenant_identity" json:"identity_number"`
	Department        string         `json:"department"`
	CustomData        map[string]any `gorm:"type:jsonb;serializer:json" json:"custom_data"`
	PersonalBase      float64        `json:"personal_base"`
	ContributionRatio float64        `json:"contribution_ratio"`
	PersonalAmount    float64        `json:"personal_amount"`
	CompanyAmount     float64        `json:"company_amount"`
	TotalAmount       float64        `json:"total_amount"`
	Status            string         `gorm:"default:active" json:"status"`
	Notes             string         `json:"notes"`
	SealedAt          *time.Time     `json:"sealed_at"`
	UnsealedAt        *time.Time     `json:"unsealed_at"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

func (HRPFRecord) TableName() string { return "hr_pf_records" }

// HRPFBill 月度账单。
type HRPFBill struct {
	ID                  string    `gorm:"primaryKey" json:"id"`
	TenantID            int64     `gorm:"index" json:"tenant_id"`
	MonthLabel          string    `gorm:"index" json:"month_label"`
	RecordCount         int       `json:"record_count"`
	BaseAmountTotal     float64   `json:"base_amount_total"`
	BillStatus          string    `json:"bill_status"`
	PersonalAmountTotal float64   `json:"personal_amount_total"`
	CompanyAmountTotal  float64   `json:"company_amount_total"`
	CombinedAmountTotal float64   `json:"combined_amount_total"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (HRPFBill) TableName() string { return "hr_pf_bills" }

// HRPFBillItem 账单明细快照。
type HRPFBillItem struct {
	ID              string    `gorm:"primaryKey" json:"id"`
	TenantID        int64     `gorm:"index" json:"tenant_id"`
	BillID          string    `gorm:"index" json:"bill_id"`
	RecordID        string    `gorm:"index" json:"record_id"`
	PersonalAccount string    `json:"personal_account"`
	Name            string    `json:"name"`
	IdentityNumber  string    `json:"identity_number"`
	Department      string    `json:"department"`
	PersonalBase    float64   `json:"personal_base"`
	PersonalAmount  float64   `json:"personal_amount"`
	CompanyAmount   float64   `json:"company_amount"`
	TotalAmount     float64   `json:"total_amount"`
	CreatedAt       time.Time `json:"created_at"`
}

func (HRPFBillItem) TableName() string { return "hr_pf_bill_items" }
