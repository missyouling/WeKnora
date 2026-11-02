// 市电账单「账单概况」表的行构造与合计（第 24 轮任务一：瘦身）。
//
// 这张表是整份账单的骨架：11 行（7 个费用子项 + 2 个小计 + 容需量 + 功率因数），
// 每行同时有「计算值」「账单提取值」「电量」三列，且计算值可被人工覆盖。
//
// 抽出的理由：
//   · 它是**纯计算**（给定 fee_items + 覆盖表 + editForm，返回行数组）；
//   · 小计的组成项（哪些行进工商业小计、哪些进居民小计）是财务口径，
//     必须在一处写清楚，散在 computed 里最容易出现「改了明细没改小计」；
//   · 覆盖键（`market` / `bill:market`）的命名规则必须与保存路径一致。
//
// **口径红线**：
//   · 工商业小计 = 市场化 + 线损 + 输配量 + 系统 + 政府基金(工商业) + 输配容(需)量；
//   · 居民小计 = 目录电费 + 政府性基金(居民)；
//   · 功率因数调整电费**不进任何小计**（它是罚/奖，单独一行）；
//   · 所有合计按「分」整数累加（`Math.round(sum*100)/100`），不留浮点尾巴。

/** 账单概况行（含计算值、账单值、覆盖键）。 */
export interface OverviewRow {
  /** 行标识（也是覆盖表的键前缀）。 */
  key: string
  label: string
  /** 计算值（子项按「分」累加得出）。 */
  value: number
  /** 实际展示值（有手动覆盖时用覆盖值）。 */
  displayValue: number
  /** 账单提取值（票面/明细里读到的）。 */
  billFee: number | number | null
  /** 覆盖键（`bill:<key>`），与保存路径一致。 */
  ovBillKey: string
  /** 电量（`-1` 表示该行不适用，模板按「—」显示）。 */
  qty: number
  /** 点击行跳转的详情菜单 key（空串表示不可跳）。 */
  menuKey: string
  /** 是否为小计行（小计行加粗、不可编辑）。 */
  total?: boolean
}

/** 覆盖表：`key → 金额`，以及 `bill:<key> → 金额`。 */
export type FeeOverrides = Record<string, number>

/** 某费用子表的「按分累加」合计。 */
export const roundMoney = (n: number) => Math.round(n * 100) / 100

const sumFee = (arr: any[]) => roundMoney(arr.reduce((s, it) => s + (Number(it.fee) || 0), 0))
const sumBillFee = (arr: any[]) => roundMoney(arr.reduce((s, it) => s + (Number(it.fee_amount) || 0), 0))

/**
 * 类别计费数量：**取该类别行中的最大计费电量**，不是求和。
 *
 * 为什么不是求和：分时子项（尖/峰/平/谷）的 qty 是「该时段电量」，
 * 相加会得到双倍总电量；类别口径要用整表电量（即各时段里的最大值）。
 * 保留 1 位小数（电量精度）。
 */
const sumQty = (arr: any[]) => {
  if (!arr.length) return 0
  return Math.round(Math.max(...arr.map((it: any) => Number(it.qty) || 0)) * 10) / 10
}

/** 各费用子表的输入（由调用方按菜单 key 取好）。 */
export interface OverviewInputs {
  feeItems: any[]
  /** 取某菜单的费用行（页面注入，内部已按行配置过滤）。 */
  rowsOf: (menuKey: string) => any[]
  editForm: Record<string, any>
  overrides: FeeOverrides
}

/**
 * 构造账单概况的 11 行。
 *
 * 行的顺序即页面上从上到下的顺序，**不可随意调换**：
 * 工商业 5 项 → 工商业小计 → 居民 2 项 → 居民小计 → 容需量 → 功率因数。
 */
export function buildOverviewRows(inp: OverviewInputs): OverviewRow[] {
  const { rowsOf, editForm: edit, overrides } = inp
  // 覆盖优先：`ov !== undefined` 而不是 `ov || value` —— 覆盖成 0 是合法操作
  //（把算错的一项手工改成 0），用 `||` 会让 0 被当成「没覆盖」。
  const ovOf = (key: string, value: number) => {
    const ov = overrides[key]
    return ov !== undefined ? ov : value
  }
  const withOv = (key: string, value: number) => ({
    value,
    displayValue: ovOf(key, value),
  })
  const withBillOv = (key: string, value: number) => {
    const ov = overrides[`bill:${key}`]
    return { billFee: ov !== undefined ? ov : value, ovBillKey: `bill:${key}` }
  }

  const market = sumFee(rowsOf('industrial-market'))
  const line = sumFee(rowsOf('industrial-line'))
  const trans = sumFee(rowsOf('industrial-trans'))
  const sys = sumFee(rowsOf('industrial-sys'))
  const govI = sumFee(rowsOf('industrial-gov'))
  const catalog = sumFee(rowsOf('residential-catalog'))
  const govR = sumFee(rowsOf('residential-gov'))
  const capacity = Number(edit.capacity_fee) || 0
  const pf = Number(edit.pf_adjust_amount) || 0

  const marketBill = sumBillFee(rowsOf('industrial-market'))
  const lineBill = sumBillFee(rowsOf('industrial-line'))
  const transBill = sumBillFee(rowsOf('industrial-trans'))
  const sysBill = sumBillFee(rowsOf('industrial-sys'))
  const govIBill = sumBillFee(rowsOf('industrial-gov'))
  const catalogBill = sumBillFee(rowsOf('residential-catalog'))
  const govRBill = sumBillFee(rowsOf('residential-gov'))

  // 小计随子项覆盖联动：子项被手工改过，小计必须跟着变，
  // 否则「明细改了、小计没变」会让用户以为保存失败。
  const industrial = roundMoney(
    ovOf('market', market) + ovOf('line', line) + ovOf('trans', trans) +
    ovOf('sys', sys) + ovOf('govI', govI) + ovOf('capacity', capacity),
  )
  const residential = roundMoney(ovOf('catalog', catalog) + ovOf('govR', govR))
  const industrialBill = roundMoney(
    ovOf('bill:market', marketBill) + ovOf('bill:line', lineBill) +
    ovOf('bill:trans', transBill) + ovOf('bill:sys', sysBill) +
    ovOf('bill:govI', govIBill) + ovOf('bill:capacity', capacity),
  )
  const residentialBill = roundMoney(ovOf('bill:catalog', catalogBill) + ovOf('bill:govR', govRBill))

  return [
    {
      key: 'market', label: '市场化购电费',
      ...withOv('market', market), ...withBillOv('market', marketBill),
      qty: sumQty(rowsOf('industrial-market')), menuKey: 'industrial-market',
    },
    {
      key: 'line', label: '上网环节线损费',
      ...withOv('line', line), ...withBillOv('line', lineBill),
      qty: sumQty(rowsOf('industrial-line')), menuKey: 'industrial-line',
    },
    {
      key: 'trans', label: '输配电量电费',
      ...withOv('trans', trans), ...withBillOv('trans', transBill),
      qty: sumQty(rowsOf('industrial-trans')), menuKey: 'industrial-trans',
    },
    {
      key: 'sys', label: '系统运行费',
      ...withOv('sys', sys), ...withBillOv('sys', sysBill),
      qty: sumQty(rowsOf('industrial-sys')), menuKey: 'industrial-sys',
    },
    {
      key: 'govI', label: '政府基金及附加（工商业）',
      ...withOv('govI', govI), ...withBillOv('govI', govIBill),
      qty: sumQty(rowsOf('industrial-gov')), menuKey: 'industrial-gov',
    },
    {
      key: 'industrial', label: '工商业电费小计',
      value: industrial, displayValue: industrial, billFee: industrialBill,
      ovBillKey: 'bill:industrial', qty: -1, menuKey: '', total: true,
    },
    {
      key: 'catalog', label: '目录电费（居民）',
      ...withOv('catalog', catalog), ...withBillOv('catalog', catalogBill),
      qty: sumQty(rowsOf('residential-catalog')), menuKey: 'residential-catalog',
    },
    {
      key: 'govR', label: '政府性基金及附加（居民）',
      ...withOv('govR', govR), ...withBillOv('govR', govRBill),
      qty: sumQty(rowsOf('residential-gov')), menuKey: 'residential-gov',
    },
    {
      key: 'residential', label: '居民电费小计',
      value: residential, displayValue: residential, billFee: residentialBill,
      ovBillKey: 'bill:residential', qty: -1, menuKey: '', total: true,
    },
    {
      key: 'capacity', label: '输配容（需）量电费',
      ...withOv('capacity', capacity), ...withBillOv('capacity', capacity),
      qty: Number(edit.capacity) || 0, menuKey: 'capacity',
    },
    {
      key: 'pf', label: '功率因数调整电费',
      ...withOv('pf', pf), ...withBillOv('pf', pf),
      qty: -1, menuKey: 'pf-adjust',
    },
  ]
}

/**
 * 概览合计（= 工商业小计 + 居民小计 + 功率因数调整电费）。
 *
 * 注意组成：容量电费已计入工商业小计，**功率因数调整电费单独加进来**
 * （它不是任何小计的成员，但属于本期电费的组成部分）。
 * 用「两个小计 + pf」而不是「所有非小计行求和」：后者会把明细行
 * 重复计一遍（小计本来就是明细之和）。
 */
export function overviewTotalOf(rows: OverviewRow[]): number {
  const pick = (key: string) => rows.find((r) => r.key === key)?.displayValue || 0
  return roundMoney(pick('industrial') + pick('residential') + pick('pf'))
}

/** 电量列展示：`-1` 与缺失都给 `—`，其余按数值展示。 */
export function qtyLabelOf(row: OverviewRow, fmtKwh: (v: any) => string): string {
  const q = row.qty
  if (q === -1 || q === null || q === undefined) return '—'
  return fmtKwh(q)
}

/** 计算值与账单值的差额（容差判定用）。 */
export const feeDiffOf = (row: OverviewRow) =>
  roundMoney((row.displayValue || 0) - (Number(row.billFee) || 0))

/** 容差：±0.02 元（四舍五入误差）判为「对上」。 */
export const FEE_TOLERANCE = 0.02

/** 该行的计算值与账单值是否「对上」。 */
export const feeCloseOf = (row: OverviewRow) => Math.abs(feeDiffOf(row)) <= FEE_TOLERANCE

/**
 * 账单合计校验：把「计算合计」与「账单合计」对齐。
 *
 * 用于账单概况底部的「合计」行 —— 用户看到的是两个数 + 一个是否相符的标记。
 */
export function billTotalCheck(
  rows: OverviewRow[],
  billTotal: number,
): { calc: number; bill: number; diff: number; ok: boolean } {
  const calc = overviewTotalOf(rows)
  const bill = roundMoney(Number(billTotal) || 0)
  const diff = roundMoney(calc - bill)
  return { calc, bill, diff, ok: Math.abs(diff) <= FEE_TOLERANCE }
}
