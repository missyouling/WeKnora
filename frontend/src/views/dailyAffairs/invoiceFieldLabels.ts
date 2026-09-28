// 发票字段「英文契约键 → 中文显示名」映射。
// 说明：subs.name（invoiceNo 等）是提取/存储的 API 契约键，禁止改名；
// 展示层（字段配置、提取规则、详情抽屉）一律经 invoiceFieldLabel 映射为中文。
export const INVOICE_FIELD_LABELS: Record<string, string> = {
  invoiceNo: '发票号码',
  invoiceCode: '发票代码',
  invoiceType: '发票类型',
  invoiceDate: '开票日期',
  buyerName: '购买方',
  sellerName: '销售方',
  buyerTaxNo: '购买方税号',
  sellerTaxNo: '销售方税号',
  amount: '金额',
  tax: '税额',
  taxAmount: '税额',
  taxRate: '税率',
  totalAmount: '价税合计',
  issuer: '开票人',
  remark: '备注',
  extractStatus: '提取状态',
  tags: '标签',
  fileName: '文件名',
  pdfUrl: '源文件',
  sourceFile: '源文件',
  invoice_category: '发票类型',
}

export function invoiceFieldLabel(name: string): string {
  return INVOICE_FIELD_LABELS[name] || name
}
