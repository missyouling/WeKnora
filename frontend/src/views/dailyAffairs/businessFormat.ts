// 业务列表的通用格式化：金额 / 税率 / 明细单元格。
//
// 为什么单独成模块：发票列表、明细浮层、详情抽屉三处都要把数值渲染成
// "1,234.56" 与 "13%"。各自内联一份必然漂移，而明细浮层抽成中台组件后
// 更不可能去 import 某个页面里的私有函数。

/** 金额：千分位 + 固定两位小数（1,234.56） */
export function formatAmount(v?: number | string | null): string {
  if (v === undefined || v === null || v === '') return ''
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/** 税率：小数转百分比且不保留小数（0.03 → 3%） */
export function formatRate(v?: number | string | null): string {
  if (v === undefined || v === null || v === '') return ''
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  return `${Math.round(n * 100)}%`
}

/**
 * 明细行的标准列键：这些键在明细行对象上有**独立的存储位**，
 * 其余自定义子列（车牌号/通行日期…）落在 `extra` 桶里。
 */
export const ITEM_STD_KEYS = new Set([
  'name', 'spec', 'unit', 'qty', 'price', 'amount', 'tax_rate', 'tax_amount',
])

/** 读取明细行子列值：标准键直接取行属性；自定义子列从 extra 桶兜底 */
export function itemValOf(it: any, sf: any): any {
  const key = String(sf?.key ?? '')
  if (ITEM_STD_KEYS.has(key)) return it?.[key] ?? ''
  return it?.[key] ?? it?.extra?.[key] ?? ''
}

/** 明细行单元格格式化：税率转百分比、数值千分位、空值统一显示 '-' */
export function formatItemCell(it: any, sf: any): string {
  const v = itemValOf(it, sf)
  if (v === undefined || v === null || v === '') return '-'
  if (sf?.key === 'tax_rate') return formatRate(v)
  if (sf?.dataType === 'number') return formatAmount(v)
  return String(v)
}

/** 明细单元格 class：按 dataType 映射（名称宽、数值右对齐、日期中、文本自适应） */
export function itemCellClass(sf: any): string {
  if (sf?.key === 'name') return 'item-cell--name'
  if (sf?.dataType === 'number') return 'item-cell--num'
  if (sf?.dataType === 'date') return 'item-cell--date'
  return 'item-cell--text'
}

/** 明细浮层宽度的默认约束 */
export const ITEM_POP_MIN = 280
export const ITEM_POP_MAX = 820
/** 未配置列宽的子列按此宽度估算 */
export const ITEM_POP_DEFAULT_COL = 96

/**
 * 明细浮层宽度 = 启用子列列宽之和（0/缺省按 96px 估）+ 20px 内距，夹在 [280, 820]。
 *
 * 为什么不再写死 680px：通行费发票只有 6 列、普通发票有 8 列，
 * 固定宽度要么留大片空白要么把列挤成省略号。
 */
export function subItemPopoverWidth(
  cols: any[],
  min = ITEM_POP_MIN,
  max = ITEM_POP_MAX,
): number {
  if (!cols?.length) return min
  const sum = cols.reduce(
    (acc: number, sf: any) => acc + (Number(sf?.width) > 0 ? Number(sf.width) : ITEM_POP_DEFAULT_COL),
    0,
  )
  return Math.min(max, Math.max(min, sum + 20))
}

/** 只保留启用子列（`enabled !== false`；缺省视为启用） */
export function enabledSubFields(cols: any[]): any[] {
  return (cols || []).filter((sf: any) => sf?.enabled !== false)
}
