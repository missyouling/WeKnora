// 发票 scope 注册（蓝图 P1-1 / §2.3.2 的 registry/invoice.ts 落地）。
//
// 标准字段 = INVOICE_FIELD_LABELS（含 camel/snake 同义键）+ 补洞面 22 键，
// canonical 去重后成为前端唯一的「标准契约字段」真源。原
// extractFieldMeta.ts 的 STANDARD_KEYS.invoice / INVOICE_STRUCT_KEYS 从这里派生。
import { registerScope } from './businessScope'
import { INVOICE_FIELD_LABELS, INVOICE_REMARK_PART_FIELDS } from '../views/dailyAffairs/invoiceFieldLabels'

/** 发票结构体内置字段键（snake_case）：正则命中后写标准列，其余写扩展字段桶。 */
export const INVOICE_STRUCT_KEYS = [
  'invoice_no',
  'invoice_code',
  'invoice_date',
  'invoice_type',
  'buyer_name',
  'buyer_tax_no',
  'buyer_address',
  'buyer_phone',
  'buyer_bank',
  'buyer_account',
  'seller_name',
  'seller_tax_no',
  'seller_address',
  'seller_phone',
  'seller_bank',
  'seller_account',
  'issuer',
  'remark',
  'total_amount',
  'amount',
  'tax',
  'tax_rate',
]

// canonical 去重：字典里 camel/snake 同义键只留一条声明，避免重复注册。
function dedupeByCanonical(keys: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const k of keys) {
    const c = k.toLowerCase().replace(/[^a-z0-9]/g, '')
    if (!c || seen.has(c)) continue
    seen.add(c)
    out.push(k)
  }
  return out
}

const invoiceKeys = dedupeByCanonical([
  ...Object.keys(INVOICE_FIELD_LABELS),
  'extract_status',
  'extract_error',
  'tags',
  ...INVOICE_STRUCT_KEYS,
])

/** 备注片段字段（无标准存储位，数据拆回 remark）：发票独有能力。 */
const remarkParts = new Set(INVOICE_REMARK_PART_FIELDS)

function dataTypeOf(key: string): 'text' | 'number' | 'date' | 'array' {
  const k = key.toLowerCase()
  if (k === 'invoice_date') return 'date'
  if (['total_amount', 'amount', 'tax', 'tax_rate', 'tax_amount'].includes(k)) return 'number'
  if (k === 'items' || k === 'tags') return 'array'
  return 'text'
}

/**
 * 发票公共规则底稿：分类底稿里没有的契约键统一在这里兜底。
 *
 * 为什么单独一张表：invoice 下有 5 个内置分类，各自抄一份"发票代码/项目明细"
 * 只会漂移；而这几个键是跨分类通用的。键写成 camelCase 或中文，查找时经
 * draftByKeyForms 兼容 snake_case（invoice_code）写法。
 */
const INVOICE_COMMON_RULES: Record<string, { desc: string; rule: string }> = {
  invoiceCode: {
    desc: '发票代码',
    rule: '提取“发票代码”栏（10~12 位纯数字）；全电发票/数电票通常无此栏，留空',
  },
  items: {
    desc: '项目明细行',
    rule: '逐行提取货物或服务明细（项目名称/规格型号/单位/数量/单价/金额/税率/税额）；原文无明细行则返回空数组',
  },
  车牌号: {
    desc: '车牌号码',
    rule: '提取“车牌号 / 车牌号码”栏，格式如“渝A12345”；无则留空',
  },
  车辆类型: {
    desc: '车辆类型',
    rule: '提取“车型 / 车辆类型”栏，如“一类车”“客车”；无则留空',
  },
}

registerScope({
  key: 'invoice',
  kind: 'invoice',
  categoryLabel: '发票类型',
  categoryNoun: '分类',
  namePlaceholder: '请输入分类名称',
  emptyText: '暂无发票类型',
  uploadNoun: '发票',
  uploadTypeLabel: '发票类型',
  uploadTypeTip: '选择发票类型，上传后按所选类型归档归类',
  addLabel: '新增分类',
  itemLabel: '项目明细',
  supportsSubFields: true,
  supportsRemarkParts: true,
  regexPatchable: true,
  listRows: 'invoices',
  standardFields: invoiceKeys.map((k) => ({
    key: k,
    label: INVOICE_FIELD_LABELS[k] || k,
    dataType: dataTypeOf(k),
    remarkPart: remarkParts.has(k),
  })),
  api: { extract: 'business' },
  // 列表渲染、筛选器与六同步的硬依赖：少一列整条链路静默失效，故不可删除
  builtinFieldKeys: ['invoiceType', 'extractStatus', 'tags'],
  commonRuleDrafts: INVOICE_COMMON_RULES,
})
