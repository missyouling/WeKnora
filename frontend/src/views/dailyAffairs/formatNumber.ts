/**
 * 能耗模块的数字 / 金额 / 比率格式化**唯一出口**（第 22 轮 P1-3）。
 *
 * ## 为什么要收口
 *
 * 收口前同一套格式化逻辑散落在 7 个文件里，各有各的浮点补偿与空值文案：
 *
 * | 文件 | 局部实现 |
 * |---|---|
 * | `TenantBilling.vue` | `money2` / `num2` / `round2Num` / `rateText` / `fmtMoney` / `fmtKwh` |
 * | `UtilitiesManagement.vue` | `fmtKwh` / `fmtMoney` / `fmtAmount` |
 * | `SolarManagement.vue` | `fmtKwh` / `fmtMoney` |
 * | `UtilityMeterTab.vue` | `fmtNum` / `fmtMoney` |
 * | `tenantBillingSources.ts` | `num` / `round2` |
 * | `tenantBillingTips.ts` | `n2` / `pct` |
 * | `energyBillStatements.ts` | `money` / `num` / `rate` |
 *
 * 代价是实打实的：`round2` 是否带 `1e-6` 直接决定 1 分钱 —— 第 20 轮「库区移民
 * 基金 788.08 vs 788.09」就是漏了补偿；第 21 轮又在 TenantBilling 里补了一次。
 * 同一份 Excel 对账，前端各处各算一套，迟早出现「列表 101,414.04、PDF 101,414.05」。
 *
 * ## 命名约定（按**空值文案**区分，不按数据类型）
 *
 *   · `money2` / `num2` / `rateText` / `pct`  → 空值显示 `—`（表格列、打印稿）
 *   · `cellNum`                               → 空值显示 ``（列表单元格，不占位）
 *   · `numOrZero`                             → 空值显示 `0`（合计、计数）
 *   · `grouped`                               → 千分位，选项决定空值文案
 *
 * 单价 / 比率一律走 `rateText`：**不能**用金额格式化（会把 0.436 显示成 0.44）。
 */

/** 浮点补偿量。1e-6 远小于「分」的量级，只用于抵消 IEEE754 表示误差。 */
const EPSILON = 1e-6

/**
 * 两位小数（四舍五入）。
 *
 * `+1e-6` 不是可选项：126600 × 0.006225 在 IEEE754 里是 788.0849999999999，
 * 直接 `Math.round(v * 100)` 得 788.08，而账单 / Excel 的预期是 788.09。
 */
export function round2(v: any): number {
  const n = Number(v)
  return Number.isFinite(n) ? Math.round(n * 100 + EPSILON) / 100 : 0
}

/** 数值收敛：非有限值一律按 0（用于参与后续运算，不用于展示）。 */
export function toNum(v: any): number {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

/** 金额：固定两位小数；非有限值显示 `—`。 */
export function money2(v: any): string {
  const n = Number(v)
  return Number.isFinite(n) ? round2(n).toFixed(2) : '—'
}

/** 数量（电量 / 水量 / 气量）：整数不带小数位，否则两位；非有限值显示 `—`。 */
export function num2(v: any): string {
  const n = Number(v)
  if (!Number.isFinite(n)) return '—'
  const r = round2(n)
  return Number.isInteger(r) ? String(r) : r.toFixed(2)
}

/**
 * 单价 / 比率文本：最多 6 位小数、末尾 0 不显示（0.436 / 0.2424 / 0.05757）。
 *
 * 0 与非法值显示 `—`：单价为 0 在账单里表示「该项不计价」，不是「免费」。
 */
export function rateText(v: any): string {
  const n = Number(v)
  if (!Number.isFinite(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}

/** 百分比：入参是小数（0.21235625 → `21.24%`）；0 与非法值显示 `—`。 */
export function pct(v: any): string {
  const n = Number(v)
  return Number.isFinite(n) && n !== 0 ? `${(n * 100).toFixed(2)}%` : '—'
}

/**
 * 列表单元格数值：空值显示空串（不占位、不显示 `—`）。
 *
 * 与 `money2` 的区别就是空值文案 —— 列表里一片 `—` 会让「本行没有这项」和
 * 「这项为 0」看起来一样（第 21 轮合计行同类问题的翻版）。
 */
export function cellNum(v: any): string {
  if (v === '' || v == null) return ''
  const n = Number(v)
  if (!Number.isFinite(n)) return ''
  const r = round2(n)
  return Number.isInteger(r) ? String(r) : r.toFixed(2)
}

/** 计数 / 合计兜底：空值按 0，整数原样，否则两位小数。 */
export function numOrZero(v: any): string {
  return String(round2(toNum(v)))
}

export interface GroupedOptions {
  /** 固定两位小数（金额场景）。默认 `false`：整数不带小数位。 */
  fixed?: boolean
  /** 空值 / 非法值的文案，默认 `'0'`。 */
  fallback?: string
  /** `''` / `null` / `undefined` 是否也算空值（返回 fallback）。默认 `false`。 */
  blankWhenEmpty?: boolean
}

/**
 * 千分位数值（列表列展示用）。
 *
 * 账单金额动辄六位数，不分组很难核对；同时必须**先按分收敛再格式化**，
 * 否则 53441.77999999996 会显示成 53,441.78（看着对）而 43316.740000000005
 * 显示成 43,316.74（也对）—— 但累加后与合计对不上。
 */
export function grouped(v: any, opts: GroupedOptions = {}): string {
  const { fixed = false, fallback = '0', blankWhenEmpty = false } = opts
  if (blankWhenEmpty && (v === '' || v == null)) return fallback
  const n = Number(v)
  if (!Number.isFinite(n)) return fallback
  const r = round2(n)
  return r.toLocaleString('zh-CN', fixed
    ? { minimumFractionDigits: 2, maximumFractionDigits: 2 }
    : { maximumFractionDigits: 2 })
}
