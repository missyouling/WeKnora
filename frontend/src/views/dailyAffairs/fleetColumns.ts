// 车队记录型菜单的「列 / 筛选器 / 抽屉字段」派生层（第 28 轮六同步）。
//
// 为什么必须独立成文件：12 个菜单的字段来源只有一处（分类 subs），
// 但消费方有四处（列表列、筛选器、抽屉字段、打印列）。若各页各写一段
// `subs.map(...)`，改口径必然漂移；这里把四处的派生规则一次定死。
//
// 与发票/能耗的差异：记录型记录的字段值都在 `record.data[中文字段名]` 里
// （不是行级扁平属性），所以取值器统一走 `data[name]`，车牌号再补一层
// `vehicle_id -> plate_no` 的映射（后端已回填，派生记录与手工记录一致）。

import type { FilterFieldSpec } from './businessFilter'
import type { SummaryColumn } from './listSummary'
import type { CatalogColumn } from './useCatalogPdf'
import { grouped, num2 } from './formatNumber'

/** 后端分类 subs 的最小子集（只取派生需要的键）。 */
export interface FleetSub {
  name: string
  label?: string
  enabled?: boolean
  is_default?: boolean
  data_type?: string
  width?: number
}

/** 一列（列表 / 打印共用）。 */
export interface FleetColumn {
  key: string
  label: string
  dataType: string
  /** 是否默认显示（subs.is_default） */
  default: boolean
  /** 固定宽度（subs.width > 0） */
  width?: number
  /** 取值：默认 data[name]，车牌号走 vehicle 映射 */
  value: (row: any) => string
  /** 原始值（合计行按数值累加用） */
  raw: (row: any) => number
}

/** 不作为筛选器的字段：备注类长文本（无检索价值）、以及纯展示派生列。 */
const NON_FILTERABLE = new Set(['备注', '来源文件', '页码'])

/** 字段名含「日期 / 时间」→ 日期列。 */
const DATE_HINT = /日期|时间/
/** 字段名含「金额 / 费用 / 税额 / 单价 / 数量 / 扣分 / 保费 / 里程 / 比例 / 税率」→ 数值列。 */
const NUMBER_HINT = /金额|费用|税额|单价|数量|扣分|保费|里程|税率|比例|天数|次数|页码|百公里/

/** 由 subs 推导 data_type（subs 已显式给类型时优先，缺省按字段名猜）。 */
export function dataTypeOfSub(sub: FleetSub): string {
  const explicit = String(sub.data_type || '').trim()
  if (explicit) return explicit
  const name = String(sub.name || '')
  if (DATE_HINT.test(name)) return 'date'
  if (NUMBER_HINT.test(name)) return 'number'
  return 'text'
}

/** 车牌号取值：优先 data 里的车牌，其次 vehicle_id 映射。 */
function plateOf(row: any, vehicleMap: Map<string, string>): string {
  const direct = String(row?.data?.['车牌号'] || row?.data?.['车牌号码'] || row?.data?.plate_no || '').trim()
  if (direct) return direct
  const vid = String(row?.vehicle_id || '')
  if (vid && vehicleMap.has(vid)) return vehicleMap.get(vid) || ''
  return ''
}

/** 数值单元格：金额两位、其余按值给。 */
function numText(v: any): string {
  const n = Number(v)
  return Number.isFinite(n) ? num2(n) : ''
}

/**
 * 由分类 subs 派生列定义。
 *
 * `vehicleMap` 是 vehicle_id → plate_no 的映射（列表页一次性加载）。
 * 停用（enabled === false）的字段不产出列 —— 与字段定义面板严格一致。
 */
export function buildFleetColumns(
  subs: readonly FleetSub[] | undefined,
  vehicleMap: Map<string, string>,
): FleetColumn[] {
  const out: FleetColumn[] = []
  const seen = new Set<string>()
  for (const s of Array.isArray(subs) ? subs : []) {
    const name = String(s?.name || '').trim()
    if (!name || seen.has(name)) continue
    if (s.enabled === false) continue
    seen.add(name)
    const dataType = dataTypeOfSub(s)
    const label = String(s.label || name).trim() || name
    const isPlate = name === '车牌号' || name === '车牌号码' || name === 'plate_no'
    const width = Number(s.width)
    const read = (row: any): any => {
      if (isPlate) return plateOf(row, vehicleMap)
      return row?.data?.[name]
    }
    out.push({
      key: name,
      label,
      dataType,
      default: s.is_default === true,
      width: Number.isFinite(width) && width > 0 ? Math.round(width) : undefined,
      raw: (row: any) => {
        const v = read(row)
        if (dataType === 'number') {
          const n = Number(v)
          return Number.isFinite(n) ? n : 0
        }
        return 0
      },
      value: (row: any) => {
        const v = read(row)
        if (v === undefined || v === null || v === '') {
          // 记录型统一空值文案：'—'（与列表其他模块一致，不显示 "undefined"）
          return '—'
        }
        if (dataType === 'date') return String(v).replace('T', ' ').slice(0, 10)
        if (dataType === 'number') {
          // 金额语义：千分位 + 两位小数（列表金额必须带千分位，52,015.90 才读得出来）；
          // 其余数量原值，纯整数不补 .00
          return SUM_AMOUNT_HINT.test(label) ? grouped(v, { fixed: true }) : numText(v)
        }
        return String(v)
      },
    })
  }
  return out
}

/** 标签含「金额 / 税额 / 费用 / 保费 / 单价」→ 按金额格式化（两位小数 + 千分位）。 */
const SUM_AMOUNT_HINT = /金额|税额|费用|保费|单价/

/** 由列定义派生「合计行」用的 SummaryColumn（可累加列才给 fieldType）。 */
export function buildFleetSummaryColumns(cols: readonly FleetColumn[]): SummaryColumn[] {
  return cols.map((c) => ({
    key: c.key,
    label: c.label,
    fieldType: c.dataType === 'number' ? (SUM_AMOUNT_HINT.test(c.label) ? 'amount' : 'number') : 'text',
    value: (row: any) => c.raw(row),
  }))
}

/** 由列定义派生筛选器：车牌号 + 日期区间 + 其余文本字段。 */
export function buildFleetFilterSpecs(cols: readonly FleetColumn[]): FilterFieldSpec[] {
  const out: FilterFieldSpec[] = []
  const hasPlate = cols.some((c) => c.key === '车牌号' || c.key === '车牌号码')
  if (hasPlate) {
    out.push({ key: 'plate', type: 'text', placeholder: '车牌号', clearable: true })
  }
  out.push({ key: 'dateRange', type: 'date-range', placeholder: '记录日期', icon: 'time' })
  // 其余字段作为「字段筛选」：高基数用 text（车牌式自由输入），其余仍用 text
  // （记录型字段取值多为自由文本，下拉候选在前端拿不到全量，用 text 更准）。
  for (const c of cols) {
    if (NON_FILTERABLE.has(c.key)) continue
    if (c.key === '车牌号' || c.key === '车牌号码') continue
    if (c.dataType === 'date' || c.dataType === 'number') continue
    out.push({
      key: `field:${c.key}`,
      type: 'text',
      placeholder: c.label,
      clearable: true,
    })
  }
  return out
}

/** 由列定义派生打印列（与列表列同源）。 */
export function buildFleetCatalogColumns(cols: readonly FleetColumn[]): CatalogColumn[] {
  return cols.map((c) => ({ key: c.key, label: c.label, value: c.value }))
}

/** 把筛选状态拼成后端 `field_filters`（`key:value;key:value`）。 */
export function buildFleetFieldFiltersParam(state: Record<string, any>): string {
  const parts: string[] = []
  for (const [k, v] of Object.entries(state || {})) {
    // 车牌号有专用筛选器（key = `plate`，不是 `field:车牌号`）——它是派生台账
    // 与手工记录共同的归属键，后端按 vehicle_id 精确匹配。漏掉这一支会让
    // 「输车牌 → 列表纹丝不动」（真机 B4）。
    if (k === 'plate') {
      const value = String(v ?? '').trim()
      if (value) parts.push(`车牌号:${value}`)
      continue
    }
    if (!k.startsWith('field:')) continue
    const value = String(v ?? '').trim()
    if (!value) continue
    parts.push(`${k.slice('field:'.length)}:${value}`)
  }
  return parts.join(';')
}
