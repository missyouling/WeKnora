// 业务列表的通用格式化：金额 / 税率 / 明细单元格。
//
// 为什么单独成模块：发票列表、明细浮层、详情抽屉三处都要把数值渲染成
// "1,234.56" 与 "13%"。各自内联一份必然漂移，而明细浮层抽成中台组件后
// 更不可能去 import 某个页面里的私有函数。
//
// 第 26 轮 O-2：**底层实现统一到 `formatNumber.ts`**，本文件降级为薄封装
// （只做参数映射，不写格式化逻辑）。原因：此前 `formatNumber` / `businessFormat` /
// `ContractManagement.vue` 三处各有一份金额与税率实现，合同页那份连千分位都没有。

import { grouped, ratePct } from './formatNumber'

/**
 * 金额：千分位 + 固定两位小数（1,234.56），空值/非法值 → 空串。
 *
 * **委托 formatNumber 的单一实现**（第 26 轮 O-2）：此前本文件与
 * `ContractManagement.vue`、`formatNumber.ts` 各有一份金额格式化，三处口径
 * 随时可能分叉（合同页那份没有千分位）。现在只保留这一个出口，参数映射固定为
 * 「固定两位 + 空值空串」——这是业务列表的既定展示口径，不要在此处改。
 */
export function formatAmount(v?: number | string | null): string {
  return grouped(v, { fixed: true, fallback: '', blankWhenEmpty: true })
}

/**
 * 税率：归一为百分比字符串，且**绝不重复 ×100**。
 *
 * 实现已移到 `formatNumber.ratePct`（第 26 轮 O-2，单一出口）；
 * 这里只保留对外签名，避免发票/制度/奖惩/浮窗四处 import 改动。
 *
 * 历史 bug：明细表 0.09 被乘两次 100 变成 900%。判别逻辑现在只有一处。
 */
export function formatRate(v?: number | string | null): string {
  return ratePct(v)
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

/**
 * 明细单元格 class：按 dataType 映射（名称宽、数值右对齐、日期中、文本自适应）。
 *
 * 多行文本单独给一个类：明细表默认 `white-space: nowrap`，长说明会被裁在
 * `max-width` 之外且没有省略号提示（看起来像"数据丢了"）。折行类把它改成
 * pre-wrap + break-word，让多行文本在明细表里也完整显示。
 */
export function itemCellClass(sf: any): string {
  if (sf?.key === 'name') return 'item-cell--name'
  if (sf?.dataType === 'number') return 'item-cell--num'
  if (sf?.dataType === 'date') return 'item-cell--date'
  if (sf?.dataType === 'textarea') return 'item-cell--multiline'
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
