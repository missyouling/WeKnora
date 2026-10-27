/**
 * 核算抽屉（电/水/气）字段渲染注册表。
 *
 * 为什么需要这一层：抽屉原先写死 12 个表单项，与「设置 -> 字段定义（subs）」完全脱节 ——
 * 多出 subs 里根本没有的「录入日期」，又漏掉已启用的「抄表方式 / 表号 / 默认单价 /
 * 安装日期 / 用电量」，顺序也不一致（真机截图对比确认）。
 *
 * 现在抽屉只做一件事：按 subs（enabled !== false）的顺序遍历，逐个 key 问这里
 * 「该用哪种控件、是否只读、是否整行」。新增/编辑共用，天然 1:1。
 *
 * 只读字段分两类：
 *   ① 记录上的派生值（usage/amount/rate）—— 由电量×倍率、用量×单价算出；
 *   ② 表计档案属性（meter_no / meter_mode / install_date / default_unit_price）——
 *      它们属于「表计」而不是「这条抄表记录」，在抽屉里改会造成语义混乱，
 *      统一只读展示，要改去「表计设置」。
 */

export type MeterFieldKind =
  | 'month'
  | 'date'
  | 'use-unit'
  | 'meter'
  | 'text'
  | 'number'
  | 'signed'
  | 'readings'
  | 'readonly'
  | 'usage'
  | 'amount'
  | 'textarea'

export interface MeterFieldSpec {
  kind: MeterFieldKind
  /** 整行渲染（栅格跨两列）：备注、分时四段块 */
  fullWidth?: boolean
}

const FIXED: Record<string, MeterFieldSpec> = {
  month: { kind: 'month' },
  reading_date: { kind: 'date' },
  use_unit: { kind: 'use-unit' },
  meter: { kind: 'meter' },
  reader: { kind: 'text' },
  meter_no: { kind: 'readonly' },
  meter_mode: { kind: 'readonly' },
  install_date: { kind: 'readonly' },
  default_unit_price: { kind: 'readonly' },
  start_reading: { kind: 'readings' },
  end_reading: { kind: 'readings' },
  rate: { kind: 'readonly' },
  usage: { kind: 'usage' },
  unit_price: { kind: 'number' },
  subsidy: { kind: 'signed' },
  garbage_fee: { kind: 'number' },
  secondary_water_fee: { kind: 'number' },
  sewage_fee: { kind: 'number' },
  amount: { kind: 'amount' },
  remark: { kind: 'textarea', fullWidth: true },
}

/**
 * 按字段 key 决定渲染方式；未知 key 按 data_type 兜底。
 *
 * 兜底很重要：用户在字段定义里新增自定义字段时，抽屉不该崩，也不该静默不显示 ——
 * 拿不到专用控件就退化成同类型的通用输入框。
 */
export function meterFieldSpec(key: string, dataType?: string): MeterFieldSpec {
  const hit = FIXED[String(key || '').trim()]
  if (hit) return hit
  const dt = String(dataType || 'text').toLowerCase()
  if (dt === 'number' || dt === 'amount') return { kind: 'number' }
  if (dt === 'date') return { kind: 'date' }
  if (dt === 'textarea' || dt === 'longtext') return { kind: 'textarea', fullWidth: true }
  return { kind: 'text' }
}

/**
 * subs 不可用时的兜底字段集（后端未迁移 / 接口失败）。
 *
 * 刻意不含 `record_date`（录入日期）：它从来没在字段定义里出现过，
 * 保留它等于把「字段与配置 1:1」的契约重新破坏掉。
 */
export const METER_FALLBACK_FIELDS: Array<{ name: string; label: string; data_type?: string }> = [
  { name: 'month', label: '月份' },
  { name: 'use_unit', label: '使用单位' },
  { name: 'meter', label: '表计' },
  { name: 'reading_date', label: '抄表日期' },
  { name: 'reader', label: '抄表人' },
  { name: 'meter_mode', label: '抄表方式' },
  { name: 'meter_no', label: '表号' },
  { name: 'start_reading', label: '起度' },
  { name: 'end_reading', label: '止度' },
  { name: 'rate', label: '倍率' },
  { name: 'usage', label: '用量' },
  { name: 'unit_price', label: '单价' },
  { name: 'amount', label: '金额' },
  { name: 'remark', label: '备注' },
]

/** 抄表方式的可读文案（表计档案里存 auto/manual）。 */
export function meterModeLabel(v: any): string {
  const s = String(v || '').trim()
  if (s === 'auto') return '自动抄表'
  if (s === 'manual') return '手动抄表'
  return s ? s : '—'
}

/**
 * 从表计档案取值给「表级只读字段」。
 *
 * 这 4 个字段在列表行上是 mapRow 从 meter 上带出来的，抽屉里必须走同一份来源，
 * 否则列表有值、抽屉空着（光伏账单 meter_* 五项踩过同一个坑）。
 */
export function meterArchiveValue(meter: any, key: string): any {
  const m = meter || {}
  switch (key) {
    case 'meter_no': return m.meter_no ?? ''
    case 'meter_mode': return meterModeLabel(m.meter_mode)
    case 'install_date': return m.install_date ?? ''
    case 'default_unit_price': return m.default_unit_price ?? ''
    default: return ''
  }
}
