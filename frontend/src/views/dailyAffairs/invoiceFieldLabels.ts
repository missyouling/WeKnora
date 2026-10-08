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
  items: '项目明细',
}

export function invoiceFieldLabel(name: string): string {
  return INVOICE_FIELD_LABELS[name] || name
}

// ---- 自定义分类中文字段名 → 标准英文契约键 ----
// 用户可在分类配置里用任意中文名定义字段（如三联收据的「收据编号」）；
// 提取与存储仍走标准英文键（invoiceNo 等），展示层经此映射完成双向对齐。
export const INVOICE_FIELD_KEY_ALIASES: Record<string, string> = {
  收据编号: 'invoiceNo', 单据编号: 'invoiceNo', 发票号码: 'invoiceNo', 发票号: 'invoiceNo',
  发票代码: 'invoiceCode', 票据代码: 'invoiceCode',
  入账日期: 'invoiceDate', 开票日期: 'invoiceDate', 开票时间: 'invoiceDate', 单据日期: 'invoiceDate',
  发票类型: 'invoiceType', 票据类型: 'invoiceType',
  价税合计: 'totalAmount', 价税合计金额: 'totalAmount', 价税总额: 'totalAmount',
  总金额: 'totalAmount', 总额: 'totalAmount', 合计金额: 'totalAmount',
  金额: 'amount', 不含税金额: 'amount', 小计: 'amount',
  税额: 'tax', 合计税额: 'tax', 税金: 'tax',
  税率: 'taxRate',
  购买方: 'buyerName', 购买方名称: 'buyerName', 购方名称: 'buyerName', 买方名称: 'buyerName',
  交款单位: 'buyerName', 付款单位: 'buyerName', 付款方: 'buyerName', 客户名称: 'buyerName',
  购买方税号: 'buyerTaxNo', 购方税号: 'buyerTaxNo', 买方税号: 'buyerTaxNo', 纳税人识别号: 'buyerTaxNo',
  销售方: 'sellerName', 销售方名称: 'sellerName', 销方名称: 'sellerName', 收款单位: 'sellerName',
  收款方: 'sellerName', 卖方名称: 'sellerName', 开票方名称: 'sellerName',
  销售方税号: 'sellerTaxNo', 销方税号: 'sellerTaxNo', 收款方税号: 'sellerTaxNo',
  开票人: 'issuer', 收款人: 'issuer', 经办: 'issuer', 经办人: 'issuer',
  备注: 'remark',
  项目明细: 'items', 明细: 'items',
  提取状态: 'extractStatus', 状态: 'extractStatus',
  标签: 'tags',
  文件名: 'fileName', 文件名称: 'fileName',
}

/** 无标准存储位的字段：提取时并入 remark 的「字段名：值」片段（收款方式/收款事由/审核等） */
export const INVOICE_REMARK_PART_FIELDS = ['收款方式', '收款事由', '审核', '审核人', '用途', '摘要', '出纳', '财务主管']

/** 中文自定义字段名 → 标准英文契约键（未命中返回原名） */
export function invoiceFieldKeyOf(name: string): string {
  return INVOICE_FIELD_KEY_ALIASES[name] || name
}

/** 是否无标准存储位字段（数据在 remark 片段中） */
export function isRemarkPartField(name: string): boolean {
  return INVOICE_REMARK_PART_FIELDS.includes(name)
}

/** 从 remark「字段名：值；字段名：值…」中提取指定片段的取值 */
export function remarkPartOf(remark: string, part: string): string {
  if (!remark) return ''
  const m = String(remark).match(new RegExp(`(?:^|；)${part}：([^；]*)`))
  return m ? (m[1] || '').trim() : ''
}

/** 更新 remark 中的指定片段（存在则替换，不存在则追加） */
export function setRemarkPart(remark: string, part: string, val: string): string {
  const v = String(val ?? '').trim()
  const seg = `${part}：${v}`
  if (!remark) return v ? seg : ''
  const re = new RegExp(`(?:^|；)${part}：[^；]*`)
  if (re.test(remark)) return remark.replace(re, seg)
  const parts = String(remark).split('；').filter(Boolean)
  if (v) parts.push(seg)
  return parts.join('；')
}
