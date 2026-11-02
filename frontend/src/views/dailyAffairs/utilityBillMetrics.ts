// 市电账单「概览页指标卡 + 用能分析」的派生计算（第 24 轮任务一：瘦身）。
//
// 抽出的是**纯计算**：给定 editForm 快照与上期数据，返回每张卡的
// 主值 / 环比箭头 / 环比颜色档 / 环比文案 / 悬浮说明。
//
// 这些量此前散落在页面里写成 40 多个 computed，每个都重复一遍
// 「if (!prev) return 0」「delta > 0 ? '↑' : ...」的样板。
// 收敛成「输入 → 指标结构」之后：
//   · 箭头/颜色档的映射只有一份（不会出现「电量涨用红色、电费涨用绿色」这种不一致）；
//   · 文案模板集中在一处，改口径只改一行；
//   · 单测可以直接断言边界（无上期、除零、持平）。

import type { EnergyCompareRow } from './utilityBillDetail'

/** 环比方向 → 展示形态（箭头 + 颜色档）。 */
export type DeltaTone = 'up' | 'down' | 'flat'

export interface DeltaView {
  /** 环比差值（已按各指标精度取整）。 */
  delta: number
  /** 箭头：涨 ↑ / 跌 ↓ / 持平 —。 */
  arrow: string
  /** 颜色档 class（涨红跌绿，符合国内账单习惯）。 */
  tone: DeltaTone
  /** 颜色档对应的样式类名。 */
  cls: string
  /** 环比副文案（如「+1,200 千瓦时」「持平」）。 */
  sub: string
  /** 悬浮说明（含公式，便于用户对数）。 */
  tip: string
}

export interface MetricCardView {
  /** 主值文本（空值统一给 `—`，不给空串 —— 空串会让卡片高度塌陷）。 */
  text: string
  delta: DeltaView
}

/** 数值格式化依赖（由页面注入，避免本模块反向依赖 formatNumber）。 */
export interface MetricFormatters {
  kwh: (v: any) => string
  money: (v: any) => string
}

/** 上期数据（pickPrevEnergy 的返回形态）。 */
export interface PrevEnergyLike {
  exists: boolean
  period: string
  total: number
  fee: number
  power_factor: number
  avg_price: number
}

const toneOf = (delta: number): DeltaTone => (delta > 0 ? 'up' : delta < 0 ? 'down' : 'flat')
const clsOf = (t: DeltaTone) => `ea-delta--${t}`
const arrowOf = (delta: number, flatAs: '—' | '' = '—') =>
  delta > 0 ? '↑' : delta < 0 ? '↓' : flatAs

/**
 * 空值判定：`null` / `undefined` / `''` 都算空。
 *
 * **不能用 `!v`**：`power_factor = 0` 是合法值（功率因数真的可以是 0），
 * 用 `!v` 会把它当空值显示成 `—`。
 */
const isBlank = (v: any) => v === null || v === undefined || v === ''

/** 账单周期：取 `bill_period_start`，缺失时回落 `bill_period_end`，都没有给 `—`。 */
export function metricPeriodText(form: Record<string, any>): string {
  const v = form?.bill_period_start || form?.bill_period_end
  return v ? String(v).slice(0, 7) : '—'
}

/** 交费截止：取 `due_date`，缺失时回落 `pay_deadline`。 */
export function metricDueText(form: Record<string, any>): string {
  const v = form?.due_date || form?.pay_deadline
  return v ? String(v).slice(0, 10) : '—'
}

/**
 * 本期电量卡。
 *
 * 差值保留原始精度（不取整），展示时才格式化 —— 取整后再相减会让
 * 「1234.56 → 1234.6」这类小差异看起来是 0，卡片显示「持平」而实际有变化。
 */
export function metricKwh(form: Record<string, any>, prev: PrevEnergyLike, f: MetricFormatters): MetricCardView {
  const cur = Number(form?.total_kwh) || 0
  const delta = prev.exists ? cur - prev.total : 0
  const sub = delta === 0 ? '持平' : `${delta > 0 ? '+' : '-'}${f.kwh(Math.abs(delta))} 千瓦时`
  const pct = prev.total ? ((cur - prev.total) / prev.total) * 100 : 0
  const tip = prev.exists
    ? `本期电量 ${f.kwh(cur)} 千瓦时，上期 ${f.kwh(prev.total)} 千瓦时\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
    : '暂无上期数据'
  return {
    text: f.kwh(cur),
    delta: { delta, arrow: arrowOf(delta), tone: toneOf(delta), cls: clsOf(toneOf(delta)), sub, tip },
  }
}

/**
 * 本期电费卡。
 *
 * `cur` 由调用方传入（页面上它等于「概览各子项合计」，不是票面 grand_total）——
 * 这是第 22 轮定的口径：以子项合计为准，票面合计只作对数参考。
 */
export function metricFee(cur: number, prev: PrevEnergyLike, f: MetricFormatters): MetricCardView {
  const delta = prev.exists ? cur - prev.fee : 0
  const sub = delta === 0 ? '持平' : `${delta > 0 ? '+' : '-'}${f.money(Math.abs(delta))} 元`
  const pct = prev.fee ? ((cur - prev.fee) / prev.fee) * 100 : 0
  const tip = prev.exists
    ? `本期电费 ${f.money(cur)} 元，上期 ${f.money(prev.fee)} 元\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
    : '暂无上期数据'
  return {
    text: f.money(cur),
    delta: { delta, arrow: arrowOf(delta), tone: toneOf(delta), cls: clsOf(toneOf(delta)), sub, tip },
  }
}

/**
 * 用能分析的「环比」卡（与本期电量卡口径不同：**任一侧为 0 就显示 `—`**）。
 *
 * 为什么和 metricKwh 不一样：这里是「用能分析」的对比卡，上期或本期为 0 时
 * 环比百分比在数学上无意义（除零），显示 `—` 比显示 0% 更诚实；
 * 而概览的「本期电量」卡即使上期为 0 也要显示本期绝对值。
 */
export function momCard(form: Record<string, any>, prev: PrevEnergyLike, f: MetricFormatters): MetricCardView {
  const cur = Number(form?.total_kwh) || 0
  const prevTotal = prev.exists ? prev.total : 0
  if (!prevTotal || !cur) {
    return {
      text: '—',
      delta: { delta: 0, arrow: '', tone: 'flat', cls: clsOf('flat'), sub: '持平', tip: '暂无上期数据' },
    }
  }
  const pct = Math.round(((cur - prevTotal) / prevTotal) * 1000) / 10
  const delta = cur - prevTotal
  const tip = `本期电量 ${f.kwh(cur)} 千瓦时，上期 ${f.kwh(prevTotal)} 千瓦时\n环比 = (本期 − 上期) ÷ 上期 = ${pct.toFixed(2)}%`
  return {
    text: `${pct > 0 ? '+' : ''}${pct}%`,
    delta: {
      delta,
      arrow: arrowOf(delta, ''),
      tone: toneOf(delta),
      cls: clsOf(toneOf(delta)),
      sub: delta === 0 ? '持平' : `${f.kwh(Math.abs(delta))} 千瓦时`,
      tip,
    },
  }
}

/**
 * 功率因数卡。
 *
 * 差值取到 3 位小数（功率因数的业务精度就是 3 位：0.950 / 0.955）。
 * `cur` 与 `prev` **都为 0** 才算无数据（单侧为 0 是合法读数，要显示差值）。
 */
export function pfCard(form: Record<string, any>, prev: PrevEnergyLike): MetricCardView {
  const raw = form?.power_factor
  const cur = Number(raw) || 0
  const prevVal = Number(prev.power_factor) || 0
  const noData = !cur && !prevVal
  const delta = noData ? 0 : Math.round((cur - prevVal) * 1000) / 1000
  const tip = noData
    ? '暂无上期数据'
    : `本期功率因数 ${cur}，上期 ${prevVal}\n变化 = ${delta > 0 ? '+' : ''}${delta.toFixed(2)}`
  return {
    text: isBlank(raw) ? '—' : String(raw),
    delta: {
      delta,
      arrow: arrowOf(delta, ''),
      tone: toneOf(delta),
      cls: clsOf(toneOf(delta)),
      sub: delta === 0 ? '持平' : `${Math.abs(delta).toFixed(2)}`,
      tip,
    },
  }
}

/**
 * 平均电价卡。
 *
 * 有 `avg_price` 就直接用（票面/提取值权威）；没有则用「本期电费 ÷ 本期电量」
 * 反算到 4 位小数 —— 电价的业务精度是 4 位（元/千瓦时，如 0.6153）。
 * 本期电量为 0 时给 `—`（不能除零）。
 */
export function avgPriceCard(
  form: Record<string, any>,
  curFee: number,
  prev: PrevEnergyLike,
): MetricCardView {
  const raw = form?.avg_price
  let text: string
  if (!isBlank(raw)) {
    text = String(Number(raw))
  } else {
    const kwh = Number(form?.total_kwh) || 0
    text = kwh ? String(Math.round((curFee / kwh) * 10000) / 10000) : '—'
  }
  const cur = Number(text) || 0
  const prevVal = prev.avg_price
  const noData = !cur && !prevVal
  const delta = noData ? 0 : Math.round((cur - prevVal) * 10000) / 10000
  const pct = prevVal ? ((cur - prevVal) / prevVal) * 100 : 0
  const tip = noData
    ? '暂无上期数据'
    : `平均电价 = 本期电费 ÷ 本期电量 = ${cur} 元/千瓦时\n上期 ${prevVal} 元/千瓦时，变化 ${delta > 0 ? '+' : ''}${delta.toFixed(4)}（${pct.toFixed(2)}%）`
  return {
    text,
    delta: {
      delta,
      arrow: arrowOf(delta, ''),
      tone: toneOf(delta),
      cls: clsOf(toneOf(delta)),
      sub: delta === 0 ? '持平' : `${Math.abs(delta).toFixed(4)} 元/千瓦时`,
      tip,
    },
  }
}

/** 概览 5 张卡 + 用能分析 1 张卡的汇总结构（页面直接展开用）。 */
export interface BillMetrics {
  period: string
  due: string
  kwh: MetricCardView
  fee: MetricCardView
  mom: MetricCardView
  pf: MetricCardView
  avgPrice: MetricCardView
}

/**
 * 一次算出全部指标卡。
 *
 * `curFee` 由调用方给（= 概览子项合计），不在本模块内重算 —— 避免与
 * `overviewTotal` 的口径分叉（两处算法一旦不一致，卡片和明细表就对不上）。
 */
export function buildBillMetrics(
  form: Record<string, any>,
  curFee: number,
  prev: PrevEnergyLike,
  f: MetricFormatters,
): BillMetrics {
  return {
    period: metricPeriodText(form),
    due: metricDueText(form),
    kwh: metricKwh(form, prev, f),
    fee: metricFee(curFee, prev, f),
    mom: momCard(form, prev, f),
    pf: pfCard(form, prev),
    avgPrice: avgPriceCard(form, curFee, prev),
  }
}

/** 分时对比行的「本期/上期差值」极值（用能分析表里标红用）。 */
export function compareExtremes(rows: EnergyCompareRow[]): { maxDelta: number; minDelta: number } {
  if (!rows.length) return { maxDelta: 0, minDelta: 0 }
  const ds = rows.map((r) => r.delta)
  return { maxDelta: Math.max(...ds), minDelta: Math.min(...ds) }
}
