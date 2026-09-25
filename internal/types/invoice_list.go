package types

import "time"

// InvoiceRecord 是发票级聚合列表中的一行：发票字段 + 所属源文件上下文。
type InvoiceRecord struct {
	InvoiceNo      string    `json:"invoice_no"`
	InvoiceCode    string    `json:"invoice_code"`
	InvoiceDate    string    `json:"invoice_date"`
	InvoiceType    string    `json:"invoice_type"`
	TotalAmount    *float64  `json:"total_amount"`
	Amount         *float64  `json:"amount"`
	Tax            *float64  `json:"tax"`
	TaxRate        *float64  `json:"tax_rate"`
	SellerName     string    `json:"seller_name"`
	SellerTaxNo    string    `json:"seller_tax_no"`
	SellerAddress  string    `json:"seller_address"`
	SellerPhone    string    `json:"seller_phone"`
	SellerBank     string    `json:"seller_bank"`
	SellerAccount  string    `json:"seller_account"`
	BuyerName      string    `json:"buyer_name"`
	BuyerTaxNo     string    `json:"buyer_tax_no"`
	BuyerAddress   string    `json:"buyer_address"`
	BuyerPhone     string    `json:"buyer_phone"`
	BuyerBank      string    `json:"buyer_bank"`
	BuyerAccount   string    `json:"buyer_account"`
	Issuer         string    `json:"issuer"`
	Remark         string    `json:"remark"`
	Category       string    `json:"category"`
	Items          []InvoiceExtractionItemItems `json:"items"`
	VoidFlag       bool      `json:"void_flag"`
	Page           int       `json:"page"`
	KnowledgeID    string    `json:"knowledge_id"`
	KnowledgeTitle string    `json:"knowledge_title"`
	FileName       string    `json:"file_name"`
	FileType       string    `json:"file_type"`
	Tags           []string  `json:"tags"`
	ExtractStatus  string    `json:"extract_status"`
	ExtractError   string    `json:"extract_error"`
	KBID           string    `json:"knowledge_base_id"`
	CreatedAt      time.Time `json:"created_at"`
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
	Keyword     string  // 全字段包含搜索
	InvoiceType string  // 5 枚举之一
	TaxRate     *float64 // 税率筛选（小数，如 0.03）
	Status      string  // extract_status
	DateFrom    string  // YYYY-MM-DD（含）
	DateTo      string  // YYYY-MM-DD（含）
	Page        int
	PageSize    int
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
