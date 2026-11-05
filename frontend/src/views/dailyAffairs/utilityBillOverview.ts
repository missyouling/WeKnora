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
// **口径红线**（第 33 轮修订）：
//   · 工商业小计 = 市场化 + 线损 + 输配量 + 系统 + 政府基金(工商业) —— **5 项**；
//     输配容(需)量与功率因数都是小计之外的独立行，由「本期电费」汇总时再加，
//     否则「小计 480,085 + 容需量 44,000」会把容需量算两遍；
//   · 居民小计 = 目录电费 + 政府性基金(居民)；
//   · 「电费」列 **100% 来自明细实算**：某子表没有明细行时必须是 0.00，
//     绝不用顶层提取字段兜底（否则两列同源，差异永远发现不了）；
//   · 「账单电费」列取票面直提值（顶层 capacity_fee / pf_adjust_amount）；
//   · 对比状态 = |电费 − 账单电费| ≤ 0.02；
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

/** 电量保留 1 位小数（票面精度就是 0.001，展示到 0.1 足够）。 */
const roundQty = (v: number) => Math.round(v * 10) / 10

/** 分时档位（顺序即票面顺序）。 */
const TOU_PERIODS = ['尖峰', '峰', '平', '谷']

/**
 * 类别行中的最大计费电量。
 *
 * 适用：上网环节线损 / 系统运行费 / 政府性基金 / 居民两项 —— 这些类别下每一行
 * 的 qty 都是**全量口径**（如每行都是 614,578），相加会翻倍，只能取最大。
 */
const maxQty = (arr: any[]) => {
  if (!arr.length) return 0
  return roundQty(Math.max(...arr.map((it: any) => Number(it.qty) || 0)))
}

/**
 * 分时类别（市场化购电费 / 输配电量电费）的计费电量 = 四段计费电量之和。
 *
 * 真机事故（第 33 轮）：旧实现用「取最大」，概况里显示 250,440（谷段单行），
 * 而正确值是 34,105+133,236+196,797+250,440 = 614,578。
 *
 * 真机事故（第 35 轮，库内 13/17 份历史账单）：这些类别下**同时**存在两种行——
 *   · 分时计费行（`零售交易电费(尖峰/峰/平/谷)`、`零售输配电费(…)`）：qty 是该时段电量；
 *   · 全量口径行（`零售损益分摊电费`、`上网环节线损费用`）：qty 是全账单电量
 *     （2025-10 为 908,222），且 period 也标「平」。
 * 只按时段取最大，平段会被全量口径行顶替 → 四段和变成
 * 62,699+205,510+**908,222**+346,527 = 1,522,958。
 *
 * 正确口径（对库内 17 份账单逐一验证，全部误差为 0）：
 *   ① 只取名称含「零售交易电费 / 零售输配电费」的行（全量口径行名称不同）；
 *   ② **同一时段多行按 qty 去重后相加**：
 *      · 多计量点账单会把**同一个全量值**重复打印在多行上
 *        （2025-10 尖峰两行都是 62,699）→ 去重后只剩一个值，不能翻倍；
 *      · 另一些账单同段是**真分表值**（2025-07 尖峰 57,877 + 13,239 = 71,116）
 *        → 不同值必须相加，取最大会漏掉第二张表。
 *      单计量点账单每段只有一行，两种处理等价。
 */
const touQty = (arr: any[]) => {
  const PERIOD_NAME = /零售交易电费|零售输配电费/
  const hasName = arr.some((it: any) => String(it?.name || '').trim() !== '')
  // 有名称字段时只认分时计费行；老数据没有 name 则退化为全部行（保持兼容）
  const pick = hasName
    ? arr.filter((it: any) => PERIOD_NAME.test(String(it?.name || '')))
    : arr
  let sum = 0
  for (const p of TOU_PERIODS) {
    // 该时段所有非零 qty 的**不同值**集合：重复打印的全量值只算一次，
    // 真分表的不同值相加。
    const vals = new Set<number>()
    for (const it of pick) {
      if (String(it?.period || '').trim() !== p) continue
      const q = Number(it?.qty) || 0
      if (q > 0) vals.add(q)
    }
    for (const v of vals) sum += v
  }
  return roundQty(sum)
}

/**
 * 分时类别电量：优先四段求和；四段都缺（老数据没有时段字段）时退回最大值。
 * 不能反过来 —— 「取最大」正是本轮要修的 Bug。
 */
const touOrMaxQty = (arr: any[]) => {
  const sum = touQty(arr)
  return sum > 0 ? sum : maxQty(arr)
}

/** 各费用子表的输入（由调用方按菜单 key 取好）。 */
export interface OverviewInputs {
  feeItems: any[]
  /** 取某菜单的费用行（页面注入，内部已按行配置过滤）。 */
  rowsOf: (menuKey: string) => any[]
  editForm: Record<string, any>
  overrides: FeeOverrides
  /** 输配容（需）量明细（空 = 该子表无数据 → 电费列必须为 0）。 */
  capacityDetail?: Record<string, any> | null
  /** 功率因素调整明细（空 = 该子表无数据 → 电费列必须为 0）。 */
  pfDetail?: Record<string, any> | null
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

  // 「电费」列 = 纯明细实算：明细缺 → 0（不是用顶层提取值兜底）
  const capDetail = inp.capacityDetail || null
  const capacity = capDetail
    ? roundMoney((Number(capDetail.demand_fee) || 0) + (Number(capDetail.capacity_fee) || 0))
    : 0
  const pf = inp.pfDetail ? roundMoney(Number(inp.pfDetail.adjust_fee) || 0) : 0
  // 「账单电费」列 = 票面直提值（账单概况/小计处读到的原始金额）
  const capacityBill = Number(edit.capacity_fee) || 0
  const pfBill = Number(edit.pf_adjust_amount) || 0

  const marketBill = sumBillFee(rowsOf('industrial-market'))
  const lineBill = sumBillFee(rowsOf('industrial-line'))
  const transBill = sumBillFee(rowsOf('industrial-trans'))
  const sysBill = sumBillFee(rowsOf('industrial-sys'))
  const govIBill = sumBillFee(rowsOf('industrial-gov'))
  const catalogBill = sumBillFee(rowsOf('residential-catalog'))
  const govRBill = sumBillFee(rowsOf('residential-gov'))

  // 小计随子项覆盖联动：子项被手工改过，小计必须跟着变，
  // 否则「明细改了、小计没变」会让用户以为保存失败。
  // 工商业小计只含 5 项：容需量/力调在小计之外（由本期电费汇总时加）
  const industrial = roundMoney(
    ovOf('market', market) + ovOf('line', line) + ovOf('trans', trans) +
    ovOf('sys', sys) + ovOf('govI', govI),
  )
  const residential = roundMoney(ovOf('catalog', catalog) + ovOf('govR', govR))
  const industrialBill = roundMoney(
    ovOf('bill:market', marketBill) + ovOf('bill:line', lineBill) +
    ovOf('bill:trans', transBill) + ovOf('bill:sys', sysBill) +
    ovOf('bill:govI', govIBill),
  )
  const residentialBill = roundMoney(ovOf('bill:catalog', catalogBill) + ovOf('bill:govR', govRBill))

  return [
    {
      key: 'market', label: '市场化购电费',
      ...withOv('market', market), ...withBillOv('market', marketBill),
      qty: touOrMaxQty(rowsOf('industrial-market')), menuKey: 'industrial-market',
    },
    {
      key: 'line', label: '上网环节线损费',
      ...withOv('line', line), ...withBillOv('line', lineBill),
      qty: maxQty(rowsOf('industrial-line')), menuKey: 'industrial-line',
    },
    {
      key: 'trans', label: '输配电量电费',
      ...withOv('trans', trans), ...withBillOv('trans', transBill),
      qty: touOrMaxQty(rowsOf('industrial-trans')), menuKey: 'industrial-trans',
    },
    {
      key: 'sys', label: '系统运行费',
      ...withOv('sys', sys), ...withBillOv('sys', sysBill),
      qty: maxQty(rowsOf('industrial-sys')), menuKey: 'industrial-sys',
    },
    {
      key: 'govI', label: '政府基金及附加（工商业）',
      ...withOv('govI', govI), ...withBillOv('govI', govIBill),
      qty: maxQty(rowsOf('industrial-gov')), menuKey: 'industrial-gov',
    },
    {
      key: 'industrial', label: '工商业电费小计',
      value: industrial, displayValue: industrial, billFee: industrialBill,
      ovBillKey: 'bill:industrial', qty: -1, menuKey: '', total: true,
    },
    {
      key: 'catalog', label: '目录电费（居民）',
      ...withOv('catalog', catalog), ...withBillOv('catalog', catalogBill),
      qty: maxQty(rowsOf('residential-catalog')), menuKey: 'residential-catalog',
    },
    {
      key: 'govR', label: '政府性基金及附加（居民）',
      ...withOv('govR', govR), ...withBillOv('govR', govRBill),
      qty: maxQty(rowsOf('residential-gov')), menuKey: 'residential-gov',
    },
    {
      key: 'residential', label: '居民电费小计',
      value: residential, displayValue: residential, billFee: residentialBill,
      ovBillKey: 'bill:residential', qty: -1, menuKey: '', total: true,
    },
    {
      key: 'capacity', label: '输配容（需）量电费',
      ...withOv('capacity', capacity), ...withBillOv('capacity', capacityBill),
      // 计费数量 = 实际容量（明细优先，空则用顶层提取值）
      qty: Number(capDetail?.capacity) || Number(capDetail?.demand) || Number(edit.capacity) || 0,
      menuKey: 'capacity',
    },
    {
      key: 'pf', label: '功率因数调整电费',
      ...withOv('pf', pf), ...withBillOv('pf', pfBill),
      qty: -1, menuKey: 'pf-adjust',
    },
  ]
}

/**
 * 概览合计（= 工商业小计 + 居民小计 + 输配容（需）量电费 + 功率因数调整电费）。
 *
 * 口径（第 33 轮）：容需量与力调都**不在小计里**，必须在这里单独加；
 * 票据等式：本期电费 = 工商业电费 + 居民电费 + 功率因数调整电费，
 * 其中工商业电费口径含容需量，所以这里等价于「小计(5 项) + 居民 + 容需量 + 力调」。
 * 用「小计 + 独立行」而不是「所有非小计行求和」：后者会把明细行重复计一遍。
 */
export function overviewTotalOf(rows: OverviewRow[]): number {
  const pick = (key: string) => rows.find((r) => r.key === key)?.displayValue || 0
  return roundMoney(pick('industrial') + pick('residential') + pick('capacity') + pick('pf'))
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
 * 概览「本期电费」行的**账单电费**（票面口径合计）。
 *
 * 口径（第 38 轮，真机图 image_9cdbe3）：
 *   本期电费·账单电费 = 工商业账单小计 + 居民账单小计 + 输配容（需）量账单电费
 *                       + 功率因数调整账单电费
 * 也就是**四个汇总行各自的账单电费相加**，与「电费」列（明细实算）用同一组行，
 * 只是取 billFee 侧。这样只要分项对得上，合计必然对得上。
 *
 * 为什么不再直接用 `editForm.grand_total`：
 *   真机 2026-10 的电费列是 481,647.91（分项逐个都对、全绿），
 *   而 grand_total 存的是 446,003.1 —— 提取阶段算错并落库，
 *   底部合计就凭这一个脏字段报「异常」，前面 10 行全绿也救不回来。
 *   分项账单电费（fee_amount / capacity_fee / pf_adjust_amount）是逐行提的，
 *   可靠性远高于单一汇总字段。
 *
 * 兜底：分项一个都没有（老账单没有 fee_items）时返回 0，由调用方回退到顶层字段。
 */
export function overviewBillTotalOf(rows: OverviewRow[]): number {
  const pick = (key: string) => Number(rows.find((r) => r.key === key)?.billFee) || 0
  return roundMoney(pick('industrial') + pick('residential') + pick('capacity') + pick('pf'))
}

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
