// 详情抽屉的纯逻辑层（蓝图 P1-5 的「纯函数先行」）。
//
// 这一层解决的是 InvoiceManagement 里最脆的三段代码：
//   1. 行数据 → 编辑表单的映射（备注片段拆解、明细行子列回填、动态字段取值优先级）；
//   2. 表单 → 后端载荷的组装（税率百分比↔小数、备注片段回写、动态字段入 fields 桶）；
//   3. 保存后的列表局部更新与"是否有有效数据"状态判定。
// 三段原先散在 300 行模板与函数里互相耦合，任一处改口径都会静默错位。
// 全部改成纯函数 + 依赖注入（业务字典/工具函数由调用方传入），可单测、可复用。

import { ITEM_STD_KEYS } from './businessFormat'
import { buildDrawerFieldsFromSubs } from './sixWaySync'

/** 发票级标准契约键（camelCase 表单空间）：其余键视为动态扩展字段 */
export const STD_DATA_KEYS = [
  'invoiceNo', 'invoiceDate', 'invoiceType', 'amount', 'taxRate', 'tax', 'totalAmount',
  'buyerName', 'sellerName', 'issuer', 'remark', 'extractStatus', 'tags',
  'sellerTaxNo', 'buyerTaxNo', 'fileName',
]
const STD_DATA_KEY_SET = new Set(STD_DATA_KEYS)

/** 税额同义键：tax / taxAmount / tax_amount 一律视作同一个「税额」 */
export const TAX_KEYS = new Set(['tax', 'taxAmount', 'tax_amount'])
export function isTaxKey(k: string): boolean {
  return TAX_KEYS.has(String(k || '').trim())
}

/** 详情抽屉表单字段定义（与 BusinessSplitDetailDrawer 的 props.fields 同形） */
export interface DetailFieldMeta {
  name: string
  label: string
  dataType?: string
  /** 备注片段字段名（有值表示数据存 remark 片段里） */
  remarkPart?: string
}

/** 详情表单三个核心字段：永远排在最顶部且完整可见 */
export const CORE_INVOICE_FIELDS = ['invoiceNo', 'invoiceDate', 'invoiceType']

export interface DetailFormShape {
  invoice_no: string
  invoice_type: string
  items: any[]
  remarkParts: Record<string, string>
  data: Record<string, any>
}

export const EMPTY_DETAIL_FORM: DetailFormShape = {
  invoice_no: '', invoice_type: '', items: [], remarkParts: {}, data: {},
}

// ---- 基础清洗 ----

export const numToStr = (v: any): string =>
  v === null || v === undefined || v === '' ? '' : String(v)

export function toNumber(v: any): number | undefined {
  if (v === '' || v === null || v === undefined) return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

/** 数字输入辅助：只允许数字与一个小数点（不补零、不去尾随小数点，按实际输入显示） */
export function sanitizeNum(v: string): string {
  let s = String(v ?? '').replace(/[^\d.]/g, '')
  const firstDot = s.indexOf('.')
  if (firstDot >= 0) s = s.slice(0, firstDot + 1) + s.slice(firstDot + 1).replace(/\./g, '')
  return s
}

/** 标签数组 → 换行文本（表单里按行编辑） */
export function tagsToText(tags: any): string {
  if (Array.isArray(tags)) return tags.map((t: any) => (typeof t === 'string' ? t : t?.name ?? t)).join('\n')
  return String(tags || '')
}

/** 换行文本 → 标签数组（空行/空白自动丢弃） */
export function tagsFromText(text: any): string[] {
  if (Array.isArray(text)) return text
  return String(text || '').split('\n').map((x: string) => x.trim()).filter(Boolean)
}

/**
 * 任意存量值 → 数组控件的 string[]。
 *
 * 为什么需要：用户在「字段定义」把某字段的类型从 text 改成 array 后，历史记录里
 * 该字段的值仍是字符串（`"甲\n乙"` 或 `"甲；乙"`）。数组控件（t-tag-input）只认
 * `string[]`，直接把字符串塞进去会渲染成一个巨型标签。这里按换行 / 分号 / 顿号
 * 拆分，让改类型后的存量数据仍可正常编辑与回显。
 *
 * 不做任何写库副作用：仅用于编辑态初始化。
 */
export function normalizeArrayValue(v: any): string[] {
  if (Array.isArray(v)) {
    return v.map((x) => (x === null || x === undefined ? '' : String(x))).filter((x) => x !== '')
  }
  if (v === null || v === undefined) return []
  const text = String(v)
  if (!text.trim()) return []
  return text
    // 换行 / 分号（中英文）/ 顿号 都视为分隔符
    .split(/[\n;；、]/)
    .map((x) => x.trim())
    .filter(Boolean)
}

/** 数组 → 编辑态文本（每行一条），用于 textarea 兜底与打印导出。 */
export function arrayToText(v: any): string {
  return normalizeArrayValue(v).join('\n')
}

// ---- 行 → 表单 ----

export interface FillDetailContext {
  row: any
  /** 分类列表（含 subs）；按 invoice_type 命中，未命中取第一个 */
  cats: any[]
  /** 列表当前筛选类型，作为分类的最后兜底 */
  filterType?: string
  /** 明细子列定义（用于把 items[0].extra 的自定义子列回填到明细行） */
  itemSubFields?: any[]
  isRemarkPart: (name: string) => boolean
  remarkPartOf: (remark: string, name: string) => string
  fieldKeyOf: (name: string) => string
  /** 标准字段字典（用于判定"是否动态字段"） */
  dict: Record<string, string>
}

/**
 * 备注片段拆解：无标准存储位的字段（收款方式/收款事由/审核…）值存在 remark 的
 * 「字段名：值」片段里，逐字段抽出来单独编辑。
 */
export function collectRemarkParts(
  subs: any[],
  remark: string,
  isRemarkPart: (name: string) => boolean,
  remarkPartOf: (remark: string, name: string) => string,
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const s of subs || []) {
    const name = String(s?.name || '')
    if (!name || !isRemarkPart(name)) continue
    out[name] = remarkPartOf(remark || '', name)
  }
  return out
}

/**
 * 纯明细子项专属键：只可能出现在明细行、绝不可能是票面主表字段的列。
 *
 * 为什么不能再按「items 子列键集合」动态排除：明细子列与主表标准字段**同名共存
 * 是正常设计**——通行费分类的明细子列就有「金额 amount」「税率 tax_rate」
 * 「车牌号 plate_no」，而它们同时是票面主表的标准契约字段。按子列集合动态排除
 * 会把主表「金额」一并过滤掉，导致编辑抽屉缺失金额输入框（2026-10-20 回归：
 * 顶部质量告警提示「金额必填但未提取到」，表单里却根本没有这一项）。
 *
 * 因此排除名单收敛为**与标准契约字段无交集的纯明细键**；标准契约字段
 * （STD_DATA_KEY_SET）拥有最高保留优先级，无论是否与子列同名都一律保留。
 */
export const DETAIL_SUB_ONLY_KEYS = new Set([
  'spec', 'unit', 'quantity', 'qty', 'price', 'unit_price', 'item_name', 'name',
])

/** 是否纯明细子项专属键（明细行独有、不可作为票面主表字段） */
export function isDetailSubOnlyKey(key: string): boolean {
  const k = String(key || '').trim()
  return DETAIL_SUB_ONLY_KEYS.has(k)
}

/**
 * 把分类 `subs` 清洗成详情抽屉的主表单字段定义。解决真机验收暴露的三类脏数据：
 *   1. 明细容器字段（items / dataType=items）混入主表单 → 渲染出「明细见下方表格」占位行；
 *   2. 纯明细子项键（规格型号/单位/数量/单价/项目名称）被错位当成主表扩展字段；
 *   3. 税额同义键（tax / taxAmount / tax_amount）重复出现两个「税额」。
 *
 * 同时把三个核心字段（发票号码/开票日期/发票类型）强制置顶，保证永远完整可见。
 * 纯函数、可单测，与 businessFormat 的税率归一、抽屉的跳过渲染共同锁死布局。
 *
 * 六同步（2026-10-21）：字段集合/名称/顺序/类型一律由 `sixWaySync.buildDrawerFieldsFromSubs`
 * 派生，本函数只在其后做**票面主表专门**的清洗（明细子项、税额去重、核心置顶）。
 * 严禁在此追加 subs 之外的字段（历史 bug：抽屉末尾硬编码 fileName）。
 */
export function buildDetailFieldDefs(
  subs: any[],
  opts: {
    fieldKeyOf: (n: string) => string
    labelOf: (n: string) => string
    isRemarkPart: (n: string) => boolean
  },
): DetailFieldMeta[] {
  // 唯一数据源 = subs（启用项按序）。字段名/中文名/类型全部由 subs 派生：
  // 用户在「字段定义」里改名（摘要→摘要2）、改类型（text→array）、删除后即时生效。
  const derived = buildDrawerFieldsFromSubs(subs as any, {
    fieldKeyOf: opts.fieldKeyOf,
    labelOf: opts.labelOf,
    isRemarkPart: opts.isRemarkPart,
  })

  const out: DetailFieldMeta[] = []
  const seenTax = new Set<string>()
  for (const x of derived) {
    const key = x.name
    const rawName = String(x.rawName || '')
    if (!key) continue
    // 1) 明细容器字段：明细由表格区块承载，不在主表单生成占位行
    if (key === 'items' || x.dataType === 'items') continue
    // 2) 纯明细子项键错位混入主表（规格型号/单位/数量/单价/项目名称…）：跳过。
    //    标准契约字段即使在明细子列里同名出现也一律保留（见 DETAIL_SUB_ONLY_KEYS 注释）。
    if (!STD_DATA_KEY_SET.has(key) && (DETAIL_SUB_ONLY_KEYS.has(key) || DETAIL_SUB_ONLY_KEYS.has(rawName))) continue
    // 3) 税额同义去重：tax / taxAmount / tax_amount 只保留一个「税额」
    if (TAX_KEYS.has(key) || TAX_KEYS.has(rawName)) {
      if (seenTax.has('tax')) continue
      seenTax.add('tax')
    }
    out.push({
      name: key,
      label: x.label,
      dataType: x.dataType,
      remarkPart: x.remarkPart || '',
    })
  }

  // 核心字段强制置顶（按 CORE_INVOICE_FIELDS 顺序），其余保持原序
  const core: DetailFieldMeta[] = []
  const rest: DetailFieldMeta[] = []
  for (const f of out) {
    const idx = CORE_INVOICE_FIELDS.indexOf(f.name)
    if (idx >= 0 && !core.some((c) => c.name === f.name)) core.push(f)
    else rest.push(f)
  }
  core.sort((a, b) => CORE_INVOICE_FIELDS.indexOf(a.name) - CORE_INVOICE_FIELDS.indexOf(b.name))
  return [...core, ...rest]
}

function pickCategory(cats: any[], targetType: string): any {
  const enabled = (cats || []).filter((c: any) => c?.enabled !== false)
  return enabled.find((c: any) => c?.name === targetType) || enabled[0]
}

/**
 * 行数据 → 编辑表单初值。
 *
 * 分类判定顺序（与列表口径一致，勿改）：提取类型 invoiceType > 行级 category
 * （细分分类如「三联收据」）> 列表筛选类型；未命中任何分类时取第一个启用分类，
 * 保证记录永远按一组字段渲染，而不是空白表单。
 */
export function fillDetailForm(ctx: FillDetailContext): DetailFormShape {
  const r = ctx.row || {}
  const targetType = r.invoiceType || r.category || ctx.filterType || ''
  const hitCat = pickCategory(ctx.cats, targetType)
  const subs = Array.isArray(hitCat?.subs) ? hitCat.subs : []
  const remarkParts = collectRemarkParts(subs, r.remark || '', ctx.isRemarkPart, ctx.remarkPartOf)

  // 动态字段（如通行费「车牌号」）：无标准契约键、非备注片段，
  // 数据在行级 fields 扩展桶；取值优先级 fields → 行对象 → 明细行 extra
  const dynamic: Record<string, any> = {}
  const itemExtra: Record<string, any> = Array.isArray(r.items) ? (r.items[0]?.extra || {}) : {}
  for (const s of subs) {
    const rawName = String(s?.name || '')
    if (!rawName || ctx.isRemarkPart(rawName)) continue
    const key = ctx.fieldKeyOf(rawName)
    if (ctx.dict[key] || STD_DATA_KEY_SET.has(key) || TAX_KEYS.has(key)) continue
    dynamic[key] = r.fields?.[key] ?? r.fields?.[rawName] ?? r[key] ?? itemExtra[key] ?? ''
  }

  const items = (Array.isArray(r.items) ? r.items : []).map((it: any) => {
    const row: any = {
      name: it?.name || '',
      unit: it?.unit || '',
      qty: numToStr(it?.qty),
      price: numToStr(it?.price),
      // 历史存量明细常缺 amount：数量为空时单价即金额
      amount: numToStr(it?.amount) || (it?.qty === null || it?.qty === undefined || it?.qty === '' ? numToStr(it?.price) : ''),
      // 税率百分比化显示（0.03 → "3"，已是百分比的 9 不再 ×100）；保存时再转回小数
      tax_rate: it?.tax_rate === null || it?.tax_rate === undefined
        ? ''
        : (() => { const n = Number(it.tax_rate); return n >= 1 ? String(n) : String(n * 100) })(),
      // 税额缺失时回退发票级税额（历史单行明细）
      tax_amount: numToStr(it?.tax_amount) || numToStr(r.tax),
    }
    for (const sf of ctx.itemSubFields || []) {
      if (ITEM_STD_KEYS.has(sf?.key)) continue
      let v: any = it?.[sf.key] ?? it?.extra?.[sf.key] ?? ''
      if ((v === '' || v === null || v === undefined) && r.fields) {
        v = r.fields?.[sf.label] ?? r.fields?.[sf.key] ?? ''
      }
      if ((v === '' || v === null || v === undefined) && r.remark) {
        v = ctx.remarkPartOf(r.remark, sf.label) ?? ''
      }
      if (v === null || v === undefined) v = ''
      row[sf.key] = v
      // name 是标准列不入 extra；其余自定义子列写 extra 桶供后端落库
      if (sf.key !== 'name') {
        row.extra = row.extra || {}
        row.extra[sf.key] = v
      }
    }
    return row
  })

  return {
    invoice_no: r.invoiceNo || '',
    invoice_type: targetType,
    items,
    remarkParts,
    data: {
      invoiceNo: r.invoiceNo || '',
      invoiceDate: r.invoiceDate || '',
      invoiceType: r.invoiceType || '',
      amount: numToStr(r.amount),
      // 税率百分比化显示（与主表单/明细表统一口径；>=1 视为已是百分比点数）
      taxRate: (() => { const n = toNumber(r.taxRate); return n === undefined ? '' : (n >= 1 ? String(n) : String(n * 100)) })(),
      tax: numToStr(r.tax),
      totalAmount: numToStr(r.totalAmount),
      buyerName: r.buyerName || '',
      sellerName: r.sellerName || '',
      issuer: r.issuer || '',
      remark: r.remark || '',
      extractStatus: r.extractStatus || '',
      parseStatus: r.parseStatus || 'completed',
      tags: tagsToText(r.tags),
      sellerTaxNo: r.sellerTaxNo || '',
      buyerTaxNo: r.buyerTaxNo || '',
      fileName: r.fileName || '',
      ...dynamic,
    },
  }
}

// ---- 表单 → 载荷 ----

export interface PayloadContext {
  data: Record<string, any>
  remarkParts?: Record<string, string>
  invoiceType: string
  items?: any[]
  setRemarkPart: (remark: string, part: string, val: string) => string
}

/** 备注片段合回 remark（各片段按原样覆盖写入） */
export function mergeRemarkParts(
  remark: string,
  parts: Record<string, string>,
  setPart: (remark: string, part: string, val: string) => string,
): string {
  let out = remark || ''
  for (const [part, val] of Object.entries(parts || {})) out = setPart(out, part, val)
  return out
}

/** 明细行 → 后端结构：税率百分比转回小数，自定义子列原样写行键（后端自动收进 Extra） */
export function itemToPayload(it: any): any {
  const taxRate = toNumber(it?.tax_rate)
  const base: any = {
    name: it?.name || '',
    unit: it?.unit || '',
    qty: toNumber(it?.qty),
    price: toNumber(it?.price),
    amount: toNumber(it?.amount),
    tax_rate: payloadTaxRate(taxRate),
    tax_amount: toNumber(it?.tax_amount),
  }
  const extraSrc: any = { ...(it?.extra || {}), ...it }
  for (const k of Object.keys(extraSrc)) {
    if (!ITEM_STD_KEYS.has(k) && k !== 'extra') base[k] = extraSrc[k]
  }
  return base
}

/** 税率载荷归一：表单百分比点数（9）转回小数（0.09）；已是小数的 0.09 原样保留 */
function payloadTaxRate(v: number | undefined): number | undefined {
  if (v === undefined) return undefined
  return v >= 1 ? v / 100 : v
}

/** data 中非标准契约键的字段收集进 fields 扩展桶（后端 JSON 透传） */
export function pickDynamicFields(data: Record<string, any>, stdKeys: Iterable<string> = STD_DATA_KEYS): Record<string, any> {
  const std = new Set(stdKeys)
  const out: Record<string, any> = {}
  for (const [k, v] of Object.entries(data || {})) {
    if (std.has(k) || TAX_KEYS.has(k) || k === 'fields') continue
    out[k] = v ?? ''
  }
  return out
}

/** 组装写回后端的发票记录 */
export function buildDetailPayload(ctx: PayloadContext): any {
  const d = ctx.data || {}
  const remark = mergeRemarkParts(d.remark || '', ctx.remarkParts || {}, ctx.setRemarkPart)
  const updated: any = {
    invoice_no: d.invoiceNo || '',
    invoice_date: d.invoiceDate || '',
    invoice_type: ctx.invoiceType || '',
    // 归档分类与发票类型联动：列表筛选/概览卡按 category 精确匹配，不同步会让记录"消失"
    category: ctx.invoiceType || '',
    total_amount: toNumber(d.totalAmount),
    amount: toNumber(d.amount),
    tax: toNumber(d.tax),
    tax_rate: payloadTaxRate(toNumber(d.taxRate)),
    seller_name: d.sellerName || '',
    seller_tax_no: d.sellerTaxNo || '',
    buyer_name: d.buyerName || '',
    buyer_tax_no: d.buyerTaxNo || '',
    issuer: d.issuer || '',
    remark,
    tags: tagsFromText(d.tags),
    items: Array.isArray(ctx.items) ? ctx.items.map(itemToPayload) : [],
  }
  const dynamic = pickDynamicFields(d)
  if (Object.keys(dynamic).length > 0) updated.fields = dynamic
  return updated
}

// ---- 定位 / 级联 / 状态 ----

/**
 * 在多票文件里定位当前发票：先按页码（1 基，且数组下标天然对齐）命中，
 * 再按发票号兜底。页码优先是因为同一文件可能存在重复发票号。
 */
export function findInvoiceIndex(invoices: any[], page?: number, invoiceNo?: string): number {
  const list = Array.isArray(invoices) ? invoices : []
  const p = Number(page)
  // 页码是权威定位键（同文件可能存在重复发票号），先按页码（含下标对齐）命中
  if (Number.isFinite(p) && p >= 1) {
    const idx = list.findIndex((inv: any, i: number) => i === p - 1 || Number(inv?.page) === p)
    if (idx >= 0) return idx
  }
  return list.findIndex((inv: any) => inv?.invoice_no === invoiceNo)
}

/**
 * 同文件级联：发票类型实质性变更时，把该文件下所有发票的类型同步为最新值
 * （列表/概览按 category 精确匹配，不同步会出现"改了类型但其它张还是旧的"）。
 * 原地修改传入数组并返回，便于调用方继续组装整包写回。
 */
export function cascadeInvoiceType(invoices: any[], targetIdx: number, nextType: string): any[] {
  const list = Array.isArray(invoices) ? invoices : []
  if (!nextType || targetIdx < 0 || targetIdx >= list.length) return list
  const prevType = String(list[targetIdx]?.invoice_type || '')
  if (nextType === prevType) return list
  list.forEach((inv: any, i: number) => {
    if (i !== targetIdx) {
      inv.invoice_type = nextType
      inv.category = nextType
    }
  })
  return list
}

/** 是否存在任何有效发票数据（决定 extract_status 是 success 还是"待补录"） */
export function hasInvoiceData(invoices: any[]): boolean {
  return (Array.isArray(invoices) ? invoices : []).some((inv: any) =>
    String(inv?.invoice_no || '').trim() || String(inv?.invoice_date || '').trim() ||
    String(inv?.invoice_type || '').trim() || String(inv?.buyer_name || '').trim() ||
    String(inv?.seller_name || '').trim() ||
    Number(inv?.amount) > 0 || Number(inv?.total_amount) > 0)
}

export function extractStatusOf(has: boolean): { status: string; error: string } {
  return has ? { status: 'success', error: '' } : { status: 'manual', error: '人工入库待编辑，请补充字段' }
}

/** 保存后同步列表行（避免整表刷新前的视觉回滚） */
export function mergeRowAfterSave(row: any, updated: any, status: string, error: string): any {
  return {
    ...(row || {}),
    extractStatus: status,
    extractError: error,
    invoiceNo: updated?.invoice_no,
    invoiceDate: updated?.invoice_date,
    invoiceType: updated?.invoice_type,
    amount: updated?.amount,
    tax: updated?.tax,
    totalAmount: updated?.total_amount,
    buyerName: updated?.buyer_name,
    sellerName: updated?.seller_name,
    issuer: updated?.issuer,
    remark: updated?.remark,
    voidFlag: !!(row?.voidFlag),
    items: updated?.items,
    fields: updated?.fields || row?.fields || {},
  }
}

// ---- 脏检查 ----

const norm = (v: any) => {
  if (v === '' || v === null || v === undefined) return ''
  return typeof v === 'object' ? JSON.stringify(v) : String(v)
}

/** 表单是否被改动（用于启用保存按钮 / 关闭前恢复） */
export function isDetailFormDirty(a: any, b: any): boolean {
  if (!a || !b) return false
  if (norm(a.invoice_no) !== norm(b.invoice_no)) return true
  if (norm(a.invoice_type) !== norm(b.invoice_type)) return true
  const da = a.data || {}
  const db = b.data || {}
  const keys = new Set([...Object.keys(da), ...Object.keys(db)])
  for (const k of keys) if (norm(da[k]) !== norm(db[k])) return true
  const ra = a.remarkParts || {}
  const rb = b.remarkParts || {}
  const rkeys = new Set([...Object.keys(ra), ...Object.keys(rb)])
  for (const k of rkeys) if (norm(ra[k]) !== norm(rb[k])) return true
  const ia = Array.isArray(a.items) ? a.items : []
  const ib = Array.isArray(b.items) ? b.items : []
  if (ia.length !== ib.length) return true
  for (let i = 0; i < ia.length; i += 1) {
    const ka = new Set([...Object.keys(ia[i] || {}), ...Object.keys(ib[i] || {})])
    for (const k of ka) if (norm(ia[i]?.[k]) !== norm(ib[i]?.[k])) return true
  }
  return false
}
