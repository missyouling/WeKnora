/**
 * 维修结算清单「分项与细项」结构化子表（第 78 轮）。
 *
 * 业务背景：一般维修的结算清单（维修结算单）票面本身是**一张三段的表**——
 * 上部工时项目、中部零件材料、下部附加费用。旧实现把它整段塞进一个
 * `维修明细` 文本字段，导致：
 *   · 工时费/材料费只能靠模型「目测」汇总，与票面明细对不上时无从发现；
 *   · 用户想在系统里改一行零件数量，只能改整段文本。
 *
 * 现在 `line_items` 以 JSONB 持久化到 `fleet_records.data`，本文件是它的
 * **唯一纯函数层**：类型、汇总、钩稽验算、摘要生成。
 *
 * 金额一律按「元」的 number 处理（不是分）：票面本身就是两位小数的元，
 * 内部再换成分会把「0.5 小时工时」这类值也拖进整数域，得不偿失。
 * 比较统一走 `AMOUNT_EPS` 容差，避免 0.1+0.2 之类的浮点尾差误报。
 */

/** 金额比较容差：差额 ≤ 0.01 视为一致（票面最小单位是分）。 */
export const AMOUNT_EPS = 0.01

/** 维修项目明细一行（工时）。 */
export interface RepairLaborItem {
  item_name: string
  hours: number
  labor_fee: number
  technician: string
  remark: string
}

/** 零件材料明细一行。`amount` 允许与 qty×unit_price 不等（票面折扣/让价），
 *  但不等时钩稽会报出来，由用户确认。 */
export interface RepairPartItem {
  part_name: string
  unit: string
  qty: number
  unit_price: number
  amount: number
  remark: string
}

/** 附加费用明细一行。 */
export interface RepairExtraItem {
  item_name: string
  amount: number
}

export interface RepairLineItems {
  labor_items: RepairLaborItem[]
  part_items: RepairPartItem[]
  extra_items: RepairExtraItem[]
}

export const EMPTY_LINE_ITEMS: RepairLineItems = {
  labor_items: [],
  part_items: [],
  extra_items: [],
}

/** 主表参与钩稽的金额字段（顺序即提示顺序）。 */
export const REPAIR_AMOUNT_FIELDS = ['工时费', '材料费', '管理费', '其它费', '实际应收'] as const

const num = (v: any): number => {
  if (v === null || v === undefined || v === '') return 0
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}

/** 四舍五入到分，消掉浮点尾差（189.00000000000003 → 189）。 */
const round2 = (n: number): number => Math.round((n + Number.EPSILON) * 100) / 100

/** 把任意来源（后端 JSONB / 表单）归一成合法结构，缺失数组补空。 */
export function normalizeLineItems(raw: any): RepairLineItems {
  const src = raw && typeof raw === 'object' ? raw : {}
  const arr = (v: any): any[] => (Array.isArray(v) ? v : [])
  return {
    labor_items: arr(src.labor_items).map((it: any) => ({
      item_name: String(it?.item_name ?? ''),
      hours: num(it?.hours),
      labor_fee: num(it?.labor_fee),
      technician: String(it?.technician ?? ''),
      remark: String(it?.remark ?? ''),
    })),
    part_items: arr(src.part_items).map((it: any) => ({
      part_name: String(it?.part_name ?? ''),
      unit: String(it?.unit ?? ''),
      qty: num(it?.qty),
      unit_price: num(it?.unit_price),
      amount: num(it?.amount),
      remark: String(it?.remark ?? ''),
    })),
    extra_items: arr(src.extra_items).map((it: any) => ({
      item_name: String(it?.item_name ?? ''),
      amount: num(it?.amount),
    })),
  }
}

export interface LineItemsTotals {
  labor: number
  part: number
  extra: number
  /** 工时项目条数（摘要里的「N 项」） */
  laborCount: number
  /** 零件材料条目数（摘要里的「M 件」）——是**行数**，不是 qty 之和：
   *  票面摘要的「4 件」指 4 种零件，同一个螺丝买 4 个仍算 1 件。 */
  partCount: number
}

/** 三子表汇总。 */
export function lineItemsTotals(items: RepairLineItems | undefined | null): LineItemsTotals {
  const it = items || EMPTY_LINE_ITEMS
  const labor = round2(it.labor_items.reduce((a, x) => a + num(x.labor_fee), 0))
  const part = round2(it.part_items.reduce((a, x) => a + num(x.amount), 0))
  const extra = round2(it.extra_items.reduce((a, x) => a + num(x.amount), 0))
  return {
    labor,
    part,
    extra,
    laborCount: it.labor_items.length,
    partCount: it.part_items.length,
  }
}

/** 一条钩稽差异。 */
export interface RepairAmountWarning {
  /** 出问题的字段名（零件单行差异用「零件：<名称>」） */
  field: string
  /** 按明细应有的值 */
  expected: number
  /** 主表当前值 */
  actual: number
  /** actual - expected（正=多收，负=少收） */
  diff: number
  message: string
}

const money = (n: number): string => n.toFixed(2)

/**
 * 三道数学钩稽（第 78 轮需求 2.②）：
 *   1. Σ labor_fee            == 工时费
 *   2. Σ part_items.amount    == 材料费（且单行 qty × unit_price == amount）
 *   3. 工时费+材料费+管理费+其它费 == 实际应收
 *
 * 只在**明细非空**时校验第 1、2 条：手工录入、没有明细的老记录
 * （票面明细没录进系统）不该被红字追着改。
 * 第 3 条同样要求至少存在一条明细，否则「实际应收」是纯手填值。
 *
 * 返回空数组 = 全部一致。
 */
export function repairAmountWarnings(
  data: Record<string, any> | undefined,
  items: RepairLineItems | undefined | null,
): RepairAmountWarning[] {
  const d = data || {}
  const it = items || EMPTY_LINE_ITEMS
  const t = lineItemsTotals(it)
  const out: RepairAmountWarning[] = []
  const hasAny = it.labor_items.length > 0 || it.part_items.length > 0 || it.extra_items.length > 0
  if (!hasAny) return out

  const push = (field: string, expected: number, actual: number, message: string) => {
    if (Math.abs(actual - expected) <= AMOUNT_EPS) return
    out.push({ field, expected, actual, diff: round2(actual - expected), message })
  }

  if (it.labor_items.length > 0) {
    push('工时费', t.labor, num(d['工时费']),
      `工时费 ${money(num(d['工时费']))} ≠ 明细合计 ${money(t.labor)}`)
  }

  // 单行先自校验：数量 × 单价 与金额不符，说明这一行本身录错了。
  for (const p of it.part_items) {
    const calc = round2(num(p.qty) * num(p.unit_price))
    const amt = round2(num(p.amount))
    if (Math.abs(calc - amt) > AMOUNT_EPS) {
      out.push({
        field: `零件：${p.part_name || '未命名'}`,
        expected: calc,
        actual: amt,
        diff: round2(amt - calc),
        message: `${p.part_name || '未命名'} 数量×单价=${money(calc)}，金额填的是 ${money(amt)}`,
      })
    }
  }
  if (it.part_items.length > 0) {
    push('材料费', t.part, num(d['材料费']),
      `材料费 ${money(num(d['材料费']))} ≠ 明细合计 ${money(t.part)}`)
  }

  const expectedTotal = round2(t.labor + t.part + num(d['管理费']) + num(d['其它费']))
  push('实际应收', expectedTotal, num(d['实际应收']),
    `实际应收 ${money(num(d['实际应收']))} ≠ 工时+材料+管理+其它 ${money(expectedTotal)}`)

  return out
}

/** 摘要里最多列出的项目名条数（再多用户也读不完，票面摘要本身也是概括）。 */
const SUMMARY_MAX_NAMES = 6

/**
 * 自动生成主表 `维修项目摘要`（第 78 轮需求 2.②）。
 *
 * 形如：`拔左前轮更换轮胎螺丝、更换刹车管、更换喷水接头（3项/4件）`
 *   · 项目名优先取工时项目，工时为空时退零件名称；
 *   · 括号内是「工时条数 项 / 零件件数 件」；
 *   · 全空返回空串 —— 绝不产出「（0项/0件）」这种噪音。
 */
export function repairSummaryOf(items: RepairLineItems | undefined | null): string {
  const it = items || EMPTY_LINE_ITEMS
  const t = lineItemsTotals(it)
  if (!it.labor_items.length && !it.part_items.length) {
    // 只有附加费用时不生成摘要（摘要描述的是「修了什么」，税费不算修车内容）
    return ''
  }
  const names = [
    ...it.labor_items.map((x) => String(x.item_name || '').trim()),
    ...it.part_items.map((x) => String(x.part_name || '').trim()),
  ].filter(Boolean)
  const uniq: string[] = []
  for (const n of names) if (!uniq.includes(n)) uniq.push(n)
  const shown = uniq.slice(0, SUMMARY_MAX_NAMES)
  const head = shown.join('、') + (uniq.length > SUMMARY_MAX_NAMES ? ' 等' : '')
  return `${head}（${t.laborCount}项/${t.partCount}件）`
}

/**
 * 「按明细重算汇总」：把三子表汇总写回主表金额字段。
 *
 * 只覆盖**由明细推导**的四个字段（工时费 / 材料费 / 其它费 / 实际应收）与摘要；
 * 管理费是用户手填项（票面固定管理费率或一口价），绝不被覆盖。
 * 返回新的 data 对象，不改入参（Vue 里避免就地改 props）。
 */
export function recalcSummaryFromItems(
  data: Record<string, any> | undefined,
  items: RepairLineItems | undefined | null,
): Record<string, any> {
  const d = { ...(data || {}) }
  const it = items || EMPTY_LINE_ITEMS
  const t = lineItemsTotals(it)
  d['工时费'] = t.labor
  d['材料费'] = t.part
  d['其它费'] = t.extra
  d['实际应收'] = round2(t.labor + t.part + num(d['管理费']) + t.extra)
  const summary = repairSummaryOf(it)
  if (summary) d['维修项目摘要'] = summary
  return d
}
