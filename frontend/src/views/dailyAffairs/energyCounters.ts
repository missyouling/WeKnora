// 「顶部 KPI 卡片数字 === 列表底部 共 N 条」的统一计数口径。
//
// 真机踩过的坑：卡片读的是**父记录/账期批次**数（utility_meter_records 的
// records.length：3 个月 = 3 条），而列表渲染的是**展平后的抄表明细行**
// （items 总数：77/92/66 条）。两套口径 → 卡片永远比列表小一个量级。
// 本文件把各视图的展平行数收敛成纯函数，父页（卡片）与子视图（列表）
// 共用同一份实现，物理上杜绝口径漂移。
import type { EnergySub } from './energyViewColumns'

/** `YYYY-MM-DD` / `YYYY-MM` / `YYYY/MM` → `YYYY-MM`；取不出月份返回空串。 */
export function monthOf(v: any): string {
  const s = String(v || '')
  const m = s.match(/^(\d{4})[-/.](\d{1,2})/)
  if (m) return `${m[1]}-${m[2].padStart(2, '0')}`
  const t = s.match(/(20\d{2})\s*年\s*(\d{1,2})\s*月/)
  if (t) return `${t[1]}-${t[2].padStart(2, '0')}`
  return ''
}

/**
 * 市电/光伏账单 record → 账单周期月（与 TenantBilling.buildRows 同口径：
 * 先取后端归一的 bill_period，再退回起止日）。
 */
export function billMonthOfItem(it: any): string {
  return monthOf(it?.bill_period)
    || monthOf(it?.bill_period_start)
    || monthOf(it?.bill_period_end)
    || monthOf(it?.bill_month)
    || ''
}

/**
 * 电/水/气核算：record→item 展平行数（列表每行 = 一条抄表明细）。
 * 与 UtilityMeterTab.load() 的 flat 构造一一对应（flat.push 每 item 一行）。
 */
export function countMeterRows(list: any[]): number {
  let n = 0
  for (const rec of Array.isArray(list) ? list : []) n += (rec?.items || []).length
  return n
}

/**
 * 费用清单行数：月份并集，与 TenantBilling.buildRows 的 months 集合同口径。
 *
 * 并集来源：市电账单月 ∪ 光伏账单月 ∪ 已生成账单(rec)月 ∪ 电费行月
 * ∪ 气费行月 ∪ 水表读数月。租户视图额外受「使用单位=当前租户」的行过滤影响，
 * 但月份并集不依赖过滤（每月的行都会渲染，只是数值为 0），故无需区分视角。
 */
export function billingRowCount(input: {
  utilityBills: any[]
  solarBills: any[]
  records: any[]
  elecRows: any[]
  gasRows: any[]
  waterReadings: any[]
}): number {
  const months = new Set<string>()
  for (const r of input.utilityBills || []) { const m = billMonthOfItem(r?.item || r); if (m) months.add(m) }
  for (const r of input.solarBills || []) { const m = billMonthOfItem(r?.item || r); if (m) months.add(m) }
  for (const r of input.records || []) { if (r?.month) months.add(String(r.month)) }
  for (const r of input.elecRows || []) { if (r?.month) months.add(String(r.month)) }
  for (const r of input.gasRows || []) { if (r?.month) months.add(String(r.month)) }
  for (const r of input.waterReadings || []) { if (r?.month) months.add(String(r.month)) }
  return months.size
}

/**
 * 历史抽屉同款过滤：`fleet_history_hidden` 的文件不进「全部文件」。
 * 异常口径 = parse_status=failed 或 custom_metadata.extract_status=failed
 * （与 FleetUploadHistoryDrawer.isRowAbnormal 一致）。
 */
export function historyStats(rows: any[]): { total: number; abnormal: number } {
  const all = (Array.isArray(rows) ? rows : []).filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
  const abnormal = all.filter((r: any) =>
    String(r?.parse_status || '') === 'failed' || String((r?.custom_metadata || {}).extract_status || '') === 'failed').length
  return { total: all.length, abnormal }
}

/** subs 宽度 → TDesign 列宽：>0 精确生效（clamp 60~400 防误输入），0/未配置 = 自适应。 */
export function subWidthOf(sub: EnergySub | undefined | null): number | undefined {
  const w = Number((sub as any)?.width) || 0
  if (w <= 0) return undefined
  return Math.min(Math.max(w, 60), 400)
}

/**
 * 行级「搜索全部字段」文本：把一行里所有可读值拼成一个大串再做包含匹配。
 *
 * 为什么不做逐列判断：列是用户可配置的（字段定义里加列，列表就多列），
 * 逐列写死必漏；"搜索全部字段"的语义就是"我看得到的值都能搜到"。
 * 覆盖：全部标量字段 + 数组标签 + items 明细的标量值。
 */
export function rowFlattenText(row: any): string {
  const parts: string[] = []
  const push = (v: any) => {
    if (v === null || v === undefined) return
    if (typeof v === 'object') return
    parts.push(String(v))
  }
  for (const v of Object.values(row || {})) push(v)
  for (const it of (row?.items || [])) for (const v of Object.values(it || {})) push(v)
  return parts.join(' ').toLowerCase()
}

/** 多关键词（空格分隔）AND 匹配。 */
export function matchAllTokens(haystackLower: string, keyword: string): boolean {
  const tokens = String(keyword || '').trim().toLowerCase().split(/\s+/).filter(Boolean)
  return tokens.every((t) => haystackLower.includes(t))
}

/**
 * 能耗数据变更广播：子视图（核算/清单/光伏）增删改落库后调用，
 * 页面根部的 KPI 卡片计数接住并 refresh。
 *
 * 为什么广播而不是逐层回调：6 个卡片视图分别嵌在三层组件里，
 * 「保存 → 计数刷新」这条链如果靠 props 透传，每加一张表都要改签名。
 */
export const ENERGY_DATA_CHANGED_EVENT = 'energy-data-changed'

export function notifyEnergyDataChanged(): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(ENERGY_DATA_CHANGED_EVENT))
}
