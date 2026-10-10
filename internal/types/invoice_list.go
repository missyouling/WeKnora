package types

import "time"

// InvoiceRecord 是发票级聚合列表中的一行：发票字段 + 所属源文件上下文。
type InvoiceRecord struct {
	InvoiceNo      string                       `json:"invoice_no"`
	InvoiceCode    string                       `json:"invoice_code"`
	InvoiceDate    string                       `json:"invoice_date"`
	InvoiceType    string                       `json:"invoice_type"`
	TotalAmount    *float64                     `json:"total_amount"`
	Amount         *float64                     `json:"amount"`
	Tax            *float64                     `json:"tax"`
	TaxRate        *float64                     `json:"tax_rate"`
	SellerName     string                       `json:"seller_name"`
	SellerTaxNo    string                       `json:"seller_tax_no"`
	SellerAddress  string                       `json:"seller_address"`
	SellerPhone    string                       `json:"seller_phone"`
	SellerBank     string                       `json:"seller_bank"`
	SellerAccount  string                       `json:"seller_account"`
	BuyerName      string                       `json:"buyer_name"`
	BuyerTaxNo     string                       `json:"buyer_tax_no"`
	BuyerAddress   string                       `json:"buyer_address"`
	BuyerPhone     string                       `json:"buyer_phone"`
	BuyerBank      string                       `json:"buyer_bank"`
	BuyerAccount   string                       `json:"buyer_account"`
	Issuer         string                       `json:"issuer"`
	Remark         string                       `json:"remark"`
	Category       string                       `json:"category"`
	Items          []InvoiceExtractionItemItems `json:"items"`
	VoidFlag       bool                         `json:"void_flag"`
	// Fields 自定义动态字段扩展桶（如通行费发票「车牌号」），透传自 service 层，
	// 前端动态列/详情抽屉按分类 subs.name 读取。
	Fields         map[string]any               `json:"fields,omitempty"`
	Page           int                          `json:"page"`
	KnowledgeID    string                       `json:"knowledge_id"`
	KnowledgeTitle string                       `json:"knowledge_title"`
	FileName       string                       `json:"file_name"`
	FileType       string                       `json:"file_type"`
	Tags           []string                     `json:"tags"`
	ExtractStatus  string                       `json:"extract_status"`
	ExtractError   string                       `json:"extract_error"`
	KBID           string                       `json:"knowledge_base_id"`
	CreatedAt      time.Time                    `json:"created_at"`
}

// InvoiceExtractionItemItems 是发票明细行（与 service 层结构对齐，避免跨包引用）。
type InvoiceExtractionItemItems struct {
	Name    string   `json:"name"`
	Qty     *float64 `json:"qty"`
	Price   *float64 `json:"price"`
	TaxRate *float64 `json:"tax_rate"`
}

// InvoiceListFilter 是发票级聚合列表查询参数。
type InvoiceListFilter struct {
	Keyword     string   // 全字段包含搜索
	InvoiceType string   // 5 枚举之一
	// KnownCategories 启用分类名集合（scope=invoice，前端卡片权威源）。仅当
	// InvoiceType=="其它票据" 时生效：category 不属于该集合（含空）的记录归入
	// 「其它票据」，与概览卡口径一致；为空时回退 invoice_type 排除桶（向后兼容）。
	KnownCategories []string
	TaxRate         *float64 // 税率筛选（小数，如 0.03）
	Status          string   // extract_status
	DateFrom        string   // YYYY-MM-DD（含）
	DateTo          string   // YYYY-MM-DD（含）
	SortBy          string   // 表头排序字段：invoice_no / invoice_date（空=默认创建时间倒序）
	SortOrder       string   // asc / desc（默认 desc）
	Page            int
	PageSize        int
}

// InvoiceTaxRateCount 是税率下拉列表的一个选项：税率 + 出现次数。
type InvoiceTaxRateCount struct {
	TaxRate float64 `json:"tax_rate"`
	Count   int     `json:"count"`
}

// InvoiceListResult 是聚合列表返回值，聚合值基于当前筛选后的可见记录。
type InvoiceListResult struct {
	Data      []InvoiceRecord `json:"data"`
	Total     int             `json:"total"`
	SumAmount float64         `json:"sum_amount"`
	SumTax    float64         `json:"sum_tax"`
	SumTotal  float64         `json:"sum_total"`
	Page      int             `json:"page"`
	PageSize  int             `json:"page_size"`
}

// InvoiceOverviewStats 是发票台账概览统计（纯只读聚合，不涉及任何状态流）。
// 口径与列表完全一致：按发票号去重（保留 created_at 最新）、剔除空提取项。
type InvoiceOverviewStats struct {
	Total         int               `json:"total"`           // 库内有效发票总数（去重后）
	FileCount     int               `json:"file_count"`      // 已上传的发票类文件数（含解析失败文件，不含已删除）
	ParseFailed   int               `json:"parse_failed"`    // 文件级解析失败数
	ExtractFailed int               `json:"extract_failed"`  // 发票级提取失败数
	CurrentMonth  InvoiceMonthStat  `json:"current_month"`   // 本月（开票日期）收录看板
	SumTotal      float64               `json:"sum_total"`       // 库内全部有效发票价税合计总额
	ByInvoiceType []InvoiceTypeStat     `json:"by_invoice_type"` // 按发票类型分组（张数 + 金额）
	ByCategory    []InvoiceCategoryStat `json:"by_category"`     // 按用户定义分类（category 细分）分组
}

// InvoiceMonthStat 是本月发票收录看板。
type InvoiceMonthStat struct {
	Count    int     `json:"count"`
	SumTotal float64 `json:"sum_total"`
}

// InvoiceTypeStat 是单一发票类型的聚合统计。
type InvoiceTypeStat struct {
	InvoiceType string  `json:"invoice_type"`
	Count       int     `json:"count"`
	SumTotal    float64 `json:"sum_total"`
}

// InvoiceCategoryStat 是单一用户分类（category）的聚合统计；category 为空的记录归「未分类」。
type InvoiceCategoryStat struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	SumTotal float64 `json:"sum_total"`
}
