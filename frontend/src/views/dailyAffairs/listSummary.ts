/**
 * 列表底部「逐列表尾汇总」的统一实现（中台口径）。
 *
 * 为什么抽成纯函数：
 *   费用清单 / 市电账单 / 光伏账单 / 电水气核算 / 发票 / 合同 六个列表都要
 *   「每列一个合计」，而它们的列定义、字段类型、金额格式化各不相同。
 *   如果每页各写一段 reduce，必然出现「这页把单价也加了、那页把起止度加了」
 *   的口径漂移。这里把「哪些列能加、怎么加、怎么格式化、表尾怎么合并首列」
 *   一次性定死，页面只负责把自己的列定义与行数据喂进来。
 *
 * 三条硬规则（产品口径）：
 *   ① 首列/标识列显示 `合计（共 N 条）`，并横向合并掉复选框 / 序号 / 首个标识列；
 *   ② 只有**可累加的数量与金额**才求和：单价、均价、比例、倍率、功率因数、
 *      起度、止度、上期/本期示数、环比、税率、文本/日期列一律显示 `—`；
 *   ③ 金额按「分」累加再除回（避免 0.1+0.2 这类浮点残留），显示与单元格同格式。
 */

/** 页面传入的列定义（只需 key/label/类型，其余字段忽略）。 */
export interface SummaryColumn {
  key: string
  label: string
  /** TDesign 列类型：amount / number / text / date / array… */
  fieldType?: string
  /** 显式覆盖自动判定（true=累加，false=不累加） */
  summable?: boolean
  /** 取值函数（缺省读 row[key]） */
  value?: (row: any) => any
}

/**
 * 绝不能相加的键：它们是**点位值/比率**，相加毫无意义。
 *
 * 注意不要误伤：「抄见电量（reading_kwh / meter_reading）」「计费电量（bill_kwh）」
 * 是电量，必须能加；只有「上期/本期示数」这类**表盘读数**不能加。
 */
const NON_SUMMABLE_KEYS = new Set([
  // 单价 / 均价
  'unit_price', 'price', 'avg_price', 'ind_price', 'default_unit_price',
  // 比率 / 倍率 / 系数
  'ratio', 'power_factor', 'pf', 'meter_ratio', 'rate', 'tax_rate', 'multiplier',
  'adjust_ratio', 'discount', 'discount_rate',
  // 表盘读数（起止度 / 上期本期示数）
  'prev', 'curr', 'meter_prev', 'meter_curr', 'start_reading', 'end_reading',
  'prev_reading', 'curr_reading', 'reading_date',
  // 环比 / 同比
  'mom_change', 'yoy_change',
])

/** 中文表头兜底：用户可以把字段名改成任意中文，键对不上时靠文案识别。 */
const NON_SUMMABLE_LABEL = /(单价|均价|平均电价|比例|倍率|功率因|起度|止度|示数|环比|同比|税率|折扣|系数|编号|户号|表号)/

/** 可累加的列类型（其余类型一律视为不可加）。 */
const SUMMABLE_TYPES = new Set(['number', 'amount', 'money', 'currency'])

/** 该列是否参与求和。 */
export function isSummableColumn(col: SummaryColumn): boolean {
  if (typeof col.summable === 'boolean') return col.summable
  const type = String(col.fieldType || '').toLowerCase()
  if (!SUMMABLE_TYPES.has(type)) return false
  const key = String(col.key || '')
  if (NON_SUMMABLE_KEYS.has(key)) return false
  return !NON_SUMMABLE_LABEL.test(String(col.label || ''))
}

const toNum = (v: any): number => {
  if (v === null || v === undefined || v === '') return 0
  if (typeof v === 'number') return Number.isFinite(v) ? v : 0
  // 带千分位/货币符号的字符串也要能算（打印/接口偶有 '1,234.50'）
  const n = Number(String(v).replace(/[,，¥￥\s]/g, ''))
  return Number.isFinite(n) ? n : 0
}

/** 金额：按「分」整数累加，避免浮点残留（0.1+0.2=0.30000000000000004）。 */
function sumAmount(rows: any[], pick: (r: any) => any): number {
  let cents = 0
  for (const r of rows) cents += Math.round(toNum(pick(r)) * 100)
  return cents / 100
}

function sumNumber(rows: any[], pick: (r: any) => any): number {
  let total = 0
  for (const r of rows) total += toNum(pick(r))
  return Math.round(total * 100) / 100
}

/** 金额展示：千分位 + 两位小数（与列表单元格一致）。 */
export function fmtSumAmount(v: number): string {
  return (Math.round(v * 100) / 100).toLocaleString('zh-CN', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })
}

/** 数量展示：千分位 + 最多两位小数，整数不带小数点。 */
export function fmtSumNumber(v: number): string {
  const n = Math.round(v * 100) / 100
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}

/** 表尾单元格文本：不可加列 / 无数据 → `—`。 */
export function summaryCellOf(col: SummaryColumn, rows: any[]): string {
  if (!isSummableColumn(col)) return '—'
  const pick = col.value || ((r: any) => r?.[col.key])
  if (col.fieldType === 'amount' || col.fieldType === 'money' || col.fieldType === 'currency') {
    const total = sumAmount(rows, pick)
    return total === 0 && !rows.some((r) => toNum(pick(r)) !== 0) ? '—' : fmtSumAmount(total)
  }
  const total = sumNumber(rows, pick)
  return total === 0 && !rows.some((r) => toNum(pick(r)) !== 0) ? '—' : fmtSumNumber(total)
}

/** 首列表尾文案。 */
export function summaryLabelOf(count: number): string {
  return `合计（共 ${count} 条）`
}

/**
 * 首列需要横向合并几列。
 *
 * 复选框（46px）与序号（56px）都放不下「合计（共 16 条）」，所以至少合并 2 列；
 * 再往后合并一个**不可累加**的标识列（账单周期 / 名称 / 编号）刚好腾出位置。
 * 若第一个业务列本身可累加（少见），则只合并 2 列，绝不吞掉它的合计值。
 */
export function summaryHeadSpan(columns: SummaryColumn[]): number {
  if (columns.length <= 2) return columns.length
  const first = columns[2]
  return first && !isSummableColumn(first) ? 3 : 2
}

/** 表尾合并策略（喂给 `t-table` 的 `rowspanAndColspanInFooter`）。 */
export function footSpanOf(columns: SummaryColumn[]) {
  const head = summaryHeadSpan(columns)
  return ({ colIndex }: { colIndex: number }) =>
    colIndex === 0 ? { colspan: head, rowspan: 1 } : { colspan: 1, rowspan: 1 }
}

/**
 * 组装 TDesign `foot-data` 行。
 *
 * `selectKey` 是复选框列的 colKey（默认 `row-select`）：文案写在这列上，
 * 再靠 colspan 吃掉序号与首个标识列，视觉上就是「合计（共 N 条）」占住首列。
 */
export function buildFootRow(
  columns: SummaryColumn[],
  rows: any[],
  opts: { selectKey?: string; rowKey?: string } = {},
): Record<string, string> {
  const selectKey = opts.selectKey || 'row-select'
  const row: Record<string, string> = { id: '__summary__' }
  if (opts.rowKey) row[opts.rowKey] = '__summary__'
  row[selectKey] = summaryLabelOf(rows.length)
  for (const col of columns) {
    row[col.key] = summaryCellOf(col, rows)
  }
  return row
}
