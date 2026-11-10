// 市电账单列表的列定义与单元格渲染（第 24 轮任务一：UtilitiesManagement 瘦身）。
//
// 从 `UtilitiesManagement.vue` 抽出的**纯逻辑**：列定义常量、列宽收敛、
// 金额列渲染器、费用分项子表的列派生。
//
// 抽离原则：这些函数不碰 Vue 响应式状态（不读 ref/computed），
// 全部是「给数据 → 返回列/文本」的纯函数，因此可以在单测里直接锁死口径，
// 而不必挂载整个页面。页面只负责把 subs（字段配置）与行数据传进来。
//
// **口径一处都不能变**：
//   · FALLBACK_COLUMNS 严格 20 项（与后端 electricityDefaultGroupFields("overview") 逐项一致）；
//   · 列宽优先 subs.width（clamp 60~400），再回落内置 w，最后自适应；
//   · 金额列必须走 fmtAmount 渲染（TDesign 默认会把浮点残留直接上屏）。

import { formatDate, formatDateTime } from './formatDateTime'

/** 列表列定义（页面与「字段配置」同源）。 */
export interface ColDef {
  key: string
  label: string
  fieldType: string
  default: boolean
  w: string
  group?: string
  width?: number
}

/**
 * 内置兜底列：仅在 subs（后端字段配置）为空时整表兜底。
 *
 * 20 个业务字段，与后端 `electricityDefaultGroupFields("overview")` 逐项一致。
 * 上一版这里塞了 7 个费用汇总列（购电电费/线损费用/…）与 capacity_fee，
 * 但它们不在 subs 的 overview 分组里 → 列表列与「字段定义」两套口径打架，
 * 「账单周期」也因为 bill_period_start 混入而出现两列。现在统一按 20 项。
 */
export const FALLBACK_COLUMNS: ColDef[] = [
  { key: 'bill_period', label: '账单周期', fieldType: 'text', default: true, w: '110px' },
  { key: 'account_no', label: '户号', fieldType: 'text', default: true, w: '150px' },
  { key: 'account_name', label: '户名', fieldType: 'text', default: true, w: '170px' },
  { key: 'usage_category', label: '用电类别', fieldType: 'text', default: true, w: '120px' },
  { key: 'voltage_level', label: '电压等级', fieldType: 'text', default: true, w: '110px' },
  { key: 'market_attr', label: '市场化属性', fieldType: 'text', default: true, w: '130px' },
  { key: 'supply_unit', label: '供电服务单位', fieldType: 'text', default: true, w: '150px' },
  { key: 'address', label: '用电地址', fieldType: 'text', default: true, w: '220px' },
  { key: 'meter_no', label: '电能表编号', fieldType: 'text', default: true, w: '160px' },
  { key: 'total_kwh', label: '本期电量', fieldType: 'number', default: true, w: '110px' },
  { key: 'total_amount', label: '本期电费', fieldType: 'amount', default: true, w: '120px' },
  { key: 'prev_kwh', label: '上期电量', fieldType: 'number', default: true, w: '110px' },
  { key: 'mom_change', label: '环比', fieldType: 'text', default: true, w: '90px' },
  { key: 'avg_price', label: '平均电价', fieldType: 'number', default: true, w: '110px' },
  { key: 'power_factor', label: '功率因素', fieldType: 'number', default: true, w: '100px' },
  { key: 'due_date', label: '交费截止', fieldType: 'text', default: true, w: '110px' },
  { key: 'industrial_amount', label: '工商业电费', fieldType: 'amount', default: true, w: '120px' },
  { key: 'residential_amount', label: '居民电费', fieldType: 'amount', default: true, w: '110px' },
  { key: 'pf_adjust_amount', label: '力调电费', fieldType: 'amount', default: true, w: '110px' },
  { key: 'grand_total', label: '合计', fieldType: 'amount', default: true, w: '120px' },
]

/**
 * 列宽：用户在「字段定义」配置的 subs.width 最高优先（>0 精确生效，
 * clamp 60~400 防误输入）；0/未配置才回落内置 w 兜底，再不行自适应。
 *
 * 真机踩过：subs 有 width 但 ColDef 没把字段下传 → 填 260 保存后列纹丝不动。
 */
export function colWidthPx(c: ColDef): number | undefined {
  const cfg = Number(c.width) || 0
  if (cfg > 0) return Math.min(Math.max(cfg, 60), 400)
  const w = String(c.w || '')
  const px = Number(w.replace('px', ''))
  if (w.endsWith('px') && Number.isFinite(px) && px > 0) return px
  return undefined
}

/**
 * 列单元格渲染：金额列统一走 fmtAmount。
 *
 * TDesign 默认单元格会把原始数值直接上屏，浮点累加残留（如 53441.77999999996）
 * 就会原样暴露；这里做渲染期收敛，与聚合期按分累加形成双保险。
 *
 * `fmtAmount` 由调用方注入（页面用的是 `grouped(v, { fallback: '' })`），
 * 这样本模块不必反向依赖 formatNumber，也便于单测替换。
 */
export function makeAmountCell(
  c: ColDef,
  fmtAmount: (v: any) => string,
): ((_h: any, ctx: any) => any) | undefined {
  const label = String(c?.label || c?.key || '')
  const key = String(c?.key || '')
  // 第 46 轮：时间戳列也要有 cell，否则 TDesign 默认把 ISO8601 原串直接上屏。
  const isDate = c.fieldType === 'date' || /时间|日期/.test(label)
    || /^(created_at|updated_at|sealed_at|unsealed_at)$|_at$|_date$|_time$/.test(key)
  if (!isDate && c.fieldType !== 'amount') return undefined
  return (_h: any, { row }: any) => {
    const v = row?.[key]
    // 空值直接给空串：`fmtAmount` 的 fallback 是 ''，但先判一次可以避免
    // 把 `0` 与 `undefined` 交给同一个分支（0 必须上屏，缺值必须留白）。
    if (v === undefined || v === null || v === '') return ''
    if (isDate) return (/日期/.test(label) ? formatDate(v) : formatDateTime(v)) || ''
    return fmtAmount(v)
  }
}

/** 费用菜单默认列（分组字段配置缺省时兜底）。 */
export const DEFAULT_FEE_COLS = [
  { field_key: 'name', label: '费用组成', field_type: 'text' },
  { field_key: 'period', label: '时段', field_type: 'text' },
  { field_key: 'qty', label: '计费电量', field_type: 'number' },
  { field_key: 'rate', label: '计费标准', field_type: 'number' },
  { field_key: 'fee', label: '电费', field_type: 'amount' },
]

/** 字段配置条目（来自 subs 的归一形态）。 */
export interface FeeColCfg {
  field_key: string
  label: string
  field_type: string
  default_visible?: boolean
  group?: string
  sort_order?: number
  deleted_at?: any
}

/**
 * 分组菜单列：按字段配置（group + 默认显示）渲染，设置中增删改后自动同步。
 *
 * 三条兜底顺序不变：无配置 → 默认 5 列；有配置但全被隐藏 → 显示全部（按 sort_order）。
 */
export function feeGroupColsOf(all: FeeColCfg[], group: string): FeeColCfg[] {
  const list = all.filter((c) => c.group === group && c.deleted_at == null)
  if (!list.length) return DEFAULT_FEE_COLS as FeeColCfg[]
  const cols = list
    .filter((c) => c.default_visible)
    .sort((a, b) => (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0))
  if (!cols.length) {
    return list.sort((a, b) => (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0))
  }
  return cols
}

/**
 * 费用分项单元格文本。
 *
 * 数值列走 `fmtRate6`（保留 6 位但不丢尾零前的有效位），金额列走 `fmtMoney`；
 * **`0` 必须上屏**（`v || v === 0` 而不是 `v`），否则「本月这一项为 0」与
 * 「本月没有这一项」在页面上完全一样。
 */
export function feeCellTextOf(
  it: any,
  col: FeeColCfg,
  fmt: { rate6: (v: any) => string; money: (v: any) => string },
): string {
  const v = it?.[col.field_key]
  if (col.field_type === 'number') return v || v === 0 ? fmt.rate6(v) : ''
  if (col.field_type === 'amount') return v || v === 0 ? fmt.money(v) : ''
  if (v === undefined || v === null || v === '') return ''
  return String(v)
}

/** 电量明细倍率：**仅使用提取值**，不推算（推算出来的倍率会把账单算错）。 */
export function displayMultiplierOf(r: any, rate6: (v: any) => string): string {
  if (r?.multiplier !== undefined && r?.multiplier !== null && Number(r.multiplier) !== 0) {
    return rate6(r.multiplier)
  }
  return ''
}
