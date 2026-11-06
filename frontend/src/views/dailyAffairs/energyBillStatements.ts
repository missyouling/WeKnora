/**
 * 市电账单 / 光伏账单的「打印清单」文档模型。
 *
 * 为什么单独成文件：清单是**纵向文档**（概览 + 分项 + 明细 + 汇总），
 * 与「打印目录」（横向表格，直接由列表列派生）是两种版式。
 * 版式引擎（useCatalogPdf.generateStatementPdf）只认 StatementDoc 结构，
 * 业务侧只负责把账单数据摆进去 —— 这样市电/光伏/费用清单三处共用一套排版，
 * 不会各写一份导致间距、字号、合计行样式各不相同。
 *
 * 取值口径一律「与详情页同源」：分项明细按 `fee_items` 原始顺序分组，
 * 电量明细按 `meter_readings` / `residential_readings` 原样列示，
 * 光伏按 gateway（上网关口 / 发电关口）分组。任何汇总都是明细的累加，
 * 保证「清单合计 = 详情页小计 = 列表金额」三者一致。
 */
import { money2, num2, rateText, round2 } from './formatNumber'
import {
  fixedBillFeeOf,
  matchBillFeeItems,
  resolveTenantItemValue,
  resolveTimedItemFee,
  type SourceResolveContext,
} from './tenantBillingSources'
import type { StatementDoc, StatementTable } from './useCatalogPdf'

// 金额 / 数量 / 单价统一走 formatNumber（与列表、浮窗同一套浮点补偿）
const money = money2
const num = num2
const rate = rateText
const text = (v: any, fallback = '—'): string => {
  const s = String(v ?? '').trim()
  return s || fallback
}
const sum = (arr: any[], key: string) => round2(arr.reduce((s, r) => s + (Number(r?.[key]) || 0), 0))
const periodText = (it: any): string => {
  const s = String(it?.bill_period_start || '')
  const e = String(it?.bill_period_end || '')
  if (s && e) return `${s} ~ ${e}`
  return s || e || '—'
}

/** 账单月份（用于标题与文件名）：优先账期起始月 */
export function billMonthOfItem(item: any): string {
  const s = String(item?.bill_period_start || item?.bill_period_end || '')
  return s ? s.slice(0, 7) : ''
}

const FEE_COLS = [
  { label: '费用组成', weight: 3.6 },
  { label: '时段', weight: 0.9, align: 'center' as const },
  { label: '计费数量', weight: 1.3, align: 'right' as const },
  { label: '单价/标准', weight: 1.2, align: 'right' as const },
  { label: '金额(元)', weight: 1.4, align: 'right' as const },
]

/**
 * 费用分项按「账单原文分类」分组，并保持首次出现顺序。
 *
 * 不能按名称字典序排：账单的分类本身带序号（(1)市场化购电费 → (6)政府性基金），
 * 排序会打乱序号与内容的对应关系，用户对着纸质账单核不上。
 */
function feeTablesOf(items: any[], prefix: string): StatementTable[] {
  const order: string[] = []
  const byCat = new Map<string, any[]>()
  for (const it of items) {
    const cat = String(it?.category || '其他费用').trim() || '其他费用'
    if (!byCat.has(cat)) { byCat.set(cat, []); order.push(cat) }
    byCat.get(cat)!.push(it)
  }
  const tables: StatementTable[] = []
  for (const cat of order) {
    const rows = byCat.get(cat) || []
    const total = sum(rows, 'fee')
    tables.push({
      title: `${prefix}${cat}`,
      columns: FEE_COLS,
      rows: rows.map((it) => [
        text(it?.name, cat),
        text(it?.period, '—'),
        it?.qty || it?.qty === 0 ? num(it.qty) : '—',
        rate(it?.rate),
        money(it?.fee),
      ]),
      total: ['小计', '', '', '', money(total)],
      empty: '该分类本期无明细',
    })
  }
  return tables
}

function meterTableOf(readings: any[], title: string): StatementTable {
  return {
    title,
    columns: [
      { label: '示数类型', weight: 1.6 },
      { label: '上期示数', weight: 1.2, align: 'right' },
      { label: '本期示数', weight: 1.2, align: 'right' },
      { label: '倍率', weight: 0.8, align: 'right' },
      { label: '抄见电量', weight: 1.2, align: 'right' },
      { label: '变损', weight: 0.9, align: 'right' },
      { label: '线损', weight: 0.9, align: 'right' },
      { label: '加减', weight: 0.9, align: 'right' },
      { label: '计费电量', weight: 1.2, align: 'right' },
    ],
    rows: readings.map((r) => [
      text(r?.meter_type),
      num(r?.prev),
      num(r?.curr),
      rate(r?.multiplier),
      num(r?.reading_kwh),
      num(r?.trans_loss),
      num(r?.line_loss),
      num(r?.adjust),
      num(r?.bill_kwh),
    ]),
    total: ['合计', '', '', '', num(sum(readings, 'reading_kwh')), '', '', '', num(sum(readings, 'bill_kwh'))],
    empty: '本期无电量明细',
  }
}

/**
 * 市电账单打印清单。
 *
 * 内容（与详情页分区一一对应）：账单概况 → 本期指标 → 各费用组成细项
 * （按分类逐表，带小计）→ 工商业电量明细 → 居民电量明细 →
 * 基本电费/力调电费 → 合计。
 */
export function buildUtilityBillStatementDoc(rows: any[], title = '市电账单清单'): StatementDoc {
  const items = rows.map((r) => r?.item || r)
  const months = [...new Set(items.map((it) => billMonthOfItem(it)).filter(Boolean))]
  const single = items.length === 1
  const tables: StatementTable[] = []
  let grand = 0

  items.forEach((it, idx) => {
    const prefix = single ? '' : `${billMonthOfItem(it) || `第 ${idx + 1} 份`} · `
    const feeItems: any[] = Array.isArray(it?.fee_items) ? it.fee_items : []
    for (const t of feeTablesOf(feeItems, prefix)) tables.push(t)
    const meters: any[] = Array.isArray(it?.meter_readings) ? it.meter_readings : []
    if (meters.length) tables.push(meterTableOf(meters, `${prefix}电量明细（工商业）`))
    const resMeters: any[] = Array.isArray(it?.residential_readings) ? it.residential_readings : []
    if (resMeters.length) tables.push(meterTableOf(resMeters, `${prefix}电量明细（居民）`))

    // 固定金额分项：账单里不在 fee_items 上（或只有 0 占位行），必须单独列
    const fixedRows: string[][] = []
    if (Number(it?.capacity_fee)) fixedRows.push(['基本电费（输配容/需量）', '—', '—', '—', money(it.capacity_fee)])
    if (Number(it?.pf_adjust_amount)) fixedRows.push(['功率因数调整电费', '—', '—', '—', money(it.pf_adjust_amount)])
    if (fixedRows.length) {
      tables.push({
        title: `${prefix}基本电费与力调电费`,
        columns: FEE_COLS,
        rows: fixedRows,
        total: ['小计', '', '', '', money((Number(it?.capacity_fee) || 0) + (Number(it?.pf_adjust_amount) || 0))],
      })
    }

    const feeTotal = sum(feeItems, 'fee')
    const grandTotal = Number(it?.grand_total ?? it?.total_amount) || Math.round((feeTotal + (Number(it?.capacity_fee) || 0) + (Number(it?.pf_adjust_amount) || 0)) * 100) / 100
    grand += grandTotal
    tables.push({
      title: `${prefix}账单合计`,
      columns: [
        { label: '项目', weight: 2.6 },
        { label: '电量(kWh)', weight: 1.4, align: 'right' },
        { label: '金额(元)', weight: 1.6, align: 'right' },
      ],
      rows: [
        ['本期总电量 / 电费', num(it?.total_kwh), money(it?.total_amount ?? it?.grand_total)],
        ['其中：工商业', '—', money(it?.industrial_amount)],
        ['其中：居民', '—', money(it?.residential_amount)],
        ['基本电费', '—', money(it?.capacity_fee)],
        ['功率因数调整电费', '—', money(it?.pf_adjust_amount)],
      ],
      total: ['合计应付', num(it?.total_kwh), money(grandTotal)],
      // 结论表：本期电费为 0 也要显示（否则「账单合计」整块消失，用户以为漏打）
      alwaysShow: true,
    })
  })

  const first = items[0] || {}
  return {
    title: single ? `${text(first.account_name, '市电账单')} 账单清单` : `${title}（${items.length} 份）`,
    subtitle: `${months.join('、') || '—'} · 户号 ${text(first.account_no)} · ${single ? '单份账单' : `${items.length} 份账单合并`}`,
    overviewTitle: '一、账单概况',
    meta: [
      { label: '户号', value: text(first.account_no) },
      { label: '户名', value: text(first.account_name) },
      { label: '用电类别', value: text(first.usage_category) },
      { label: '电压等级', value: text(first.voltage_level) },
      { label: '市场化属性', value: text(first.market_attr) },
      { label: '供电服务单位', value: text(first.supply_unit) },
      { label: '电能表编号', value: text(first.meter_no) },
      { label: '账单周期', value: periodText(first) },
      { label: '交费截止', value: text(first.due_date) },
      { label: '用电地址', value: text(first.address) },
    ],
    kpis: [
      { label: '本期电量', value: `${num(first.total_kwh)} kWh` },
      { label: '本期电费', value: `${money(first.total_amount ?? first.grand_total)} 元` },
      { label: '平均电价', value: rate(first.avg_price) },
      { label: '功率因数', value: rate(first.power_factor) },
    ],
    tables,
    footerNote: '说明：费用分项按账单原文分类逐项列示；电量明细的「计费电量」= 抄见 + 变损 + 线损 + 加减。'
      + (items.length > 1 ? `本清单合并 ${items.length} 份账单，合计 ${money(grand)} 元。` : ''),
  }
}

/** 关口电量明细（光伏）：按关口分组，每个关口可有多行示数 */
function gatewayMeterTable(gateways: any[], prefix: string): StatementTable {
  const rows: string[][] = []
  for (const g of gateways) {
    const readings: any[] = Array.isArray(g?.readings) ? g.readings : []
    if (!readings.length) {
      rows.push([text(g?.gateway_type), text(g?.meter_no), '—', '—', '—', '—', '—', '—'])
      continue
    }
    for (const r of readings) {
      rows.push([
        text(g?.gateway_type),
        text(g?.meter_no),
        text(r?.meter_type),
        num(r?.prev),
        num(r?.curr),
        rate(r?.multiplier),
        num(r?.reading_kwh),
        num(r?.bill_kwh),
      ])
    }
  }
  return {
    title: `${prefix}关口电量明细`,
    columns: [
      { label: '关口类型', weight: 1.5 },
      { label: '电能表编号', weight: 2.2 },
      { label: '示数类型', weight: 1.3 },
      { label: '上期示数', weight: 1.2, align: 'right' },
      { label: '本期示数', weight: 1.2, align: 'right' },
      { label: '倍率', weight: 0.8, align: 'right' },
      { label: '抄见电量', weight: 1.2, align: 'right' },
      { label: '计费电量', weight: 1.2, align: 'right' },
    ],
    rows,
    empty: '本期无关口电量',
  }
}

/** 费用明细（光伏）：一个关口一张表（上网电费 / 发电补助） */
function gatewayFeeTable(gateways: any[], prefix: string, name: string): StatementTable | null {
  const rows: string[][] = []
  let total = 0
  for (const g of gateways) {
    for (const f of (Array.isArray(g?.fees) ? g.fees : [])) {
      const fee = Number(f?.fee) || 0
      total += fee
      rows.push([text(f?.category), text(f?.period, '—'), num(f?.qty), rate(f?.rate), money(fee)])
    }
  }
  if (!rows.length) return null
  return {
    title: `${prefix}${name}`,
    columns: FEE_COLS,
    rows,
    total: ['小计', '', '', '', money(Math.round(total * 100) / 100)],
  }
}

/**
 * 光伏账单打印清单。
 *
 * 内容：账单概况 → 本期指标 → 关口电量明细 → 上网电费明细 →
 * 发电补助明细 → 结算汇总。
 */
export function buildSolarBillStatementDoc(rows: any[], title = '光伏账单清单'): StatementDoc {
  const items = rows.map((r) => r?.item || r)
  const months = [...new Set(items.map((it) => billMonthOfItem(it)).filter(Boolean))]
  const single = items.length === 1
  const tables: StatementTable[] = []
  let grand = 0

  items.forEach((it, idx) => {
    const prefix = single ? '' : `${billMonthOfItem(it) || `第 ${idx + 1} 份`} · `
    const gateways: any[] = Array.isArray(it?.gateways) ? it.gateways : []
    const grid = gateways.filter((g) => String(g?.gateway_type || '') === '上网关口')
    const subsidy = gateways.filter((g) => String(g?.gateway_type || '') === '发电关口')

    tables.push(gatewayMeterTable(gateways, prefix))
    const gridFee = gatewayFeeTable(grid, prefix, '上网电费明细')
    if (gridFee) tables.push(gridFee)
    const subsidyFee = gatewayFeeTable(subsidy, prefix, '发电补助明细')
    if (subsidyFee) tables.push(subsidyFee)

    const gridTotal = Math.round(grid.reduce((s: number, g: any) =>
      s + (Array.isArray(g?.fees) ? g.fees.reduce((a: number, f: any) => a + (Number(f?.fee) || 0), 0) : 0), 0) * 100) / 100
    const subsidyTotal = Math.round(subsidy.reduce((s: number, g: any) =>
      s + (Array.isArray(g?.fees) ? g.fees.reduce((a: number, f: any) => a + (Number(f?.fee) || 0), 0) : 0), 0) * 100) / 100
    const settle = Number(it?.settlement_amount)
    const total = Number.isFinite(settle) && settle !== 0 ? settle : Math.round((gridTotal + subsidyTotal) * 100) / 100
    grand += total
    tables.push({
      title: `${prefix}结算汇总`,
      columns: [
        { label: '费用项目', weight: 2.6 },
        { label: '电量(kWh)', weight: 1.4, align: 'right' },
        { label: '金额(元)', weight: 1.6, align: 'right' },
      ],
      rows: [
        ['上网电费', num(it?.grid_kwh), money(gridTotal)],
        ['发电补助', num(it?.generation_kwh), money(subsidyTotal)],
      ],
      total: ['结算金额', num(it?.grid_kwh), money(total)],
      alwaysShow: true,
    })
  })

  const first = items[0] || {}
  return {
    title: single ? `${text(first.account_name, '光伏账单')} 账单清单` : `${title}（${items.length} 份）`,
    subtitle: `${months.join('、') || '—'} · 户号 ${text(first.account_no)} · ${single ? '单份账单' : `${items.length} 份账单合并`}`,
    overviewTitle: '一、账单概况',
    meta: [
      { label: '户号', value: text(first.account_no) },
      { label: '户名', value: text(first.account_name) },
      { label: '服务单位', value: text(first.supply_unit) },
      { label: '并网电压', value: text(first.voltage_level) },
      { label: '消纳方式', value: text(first.consumption_mode) },
      { label: '发电方式', value: text(first.generation_mode) },
      { label: '账单周期', value: periodText(first) },
      { label: '纳税人类型', value: text(first.taxpayer_type) },
      { label: '税率', value: text(first.tax_rate) },
      { label: '税额', value: money(first.tax_amount) },
    ],
    kpis: [
      { label: '发电量', value: `${num(first.generation_kwh)} kWh` },
      { label: '上网电量', value: `${num(first.grid_kwh)} kWh` },
      { label: '结算金额', value: `${money(first.settlement_amount)} 元` },
      { label: '年累计上网', value: `${num(first.cumulative_kwh)} kWh` },
    ],
    tables,
    footerNote: '说明：电费 = 电量 × 电价，按关口逐项列示；结算金额 = 上网电费 + 发电补助。'
      + (items.length > 1 ? `本清单合并 ${items.length} 份账单，合计 ${money(grand)} 元。` : ''),
  }
}


// ===========================================================================
// 费用清单（租户）的打印清单构造器（第 22 轮 P1-2 从 TenantBilling.vue 迁入）
// ===========================================================================
//
// 与市电 / 光伏清单同层管理：三者都是「概览 + 分项 + 明细 + 汇总」的纵向文档，
// 版式引擎只认 StatementDoc，业务侧只负责把数据摆进去。此前费用清单的构造器
// 内联在页面里约 400 行，改一处概览口径要在 3000 行的 SFC 里找，而市电/光伏
// 的同类逻辑早已抽到本文件 —— 同一个版式两套维护方式。

// 第 40 轮删掉旧的 `FEE_CATEGORY_ORDER`：它靠「拼接串 includes」排序，
// 把「(4)系统运行费｜上网环节线损代理采购损益」误吸进 (2) 线损组。
// 现在统一由 FEE_GROUPS 的 rank 精确归类（见下方 feeGroupOf）。

const sumBy = (arr: readonly any[], key: string): number =>
  round2(arr.reduce((s, r) => s + (Number(r[key]) || 0), 0))

/**
 * 费用清单打印清单的取数上下文。
 *
 * 构造器是纯函数（不碰 Vue 响应式），所有取数都从这些口子进来 ——
 * 这样它才能被单测直接驱动，也才能同时给「浮动栏打印」与「抽屉底栏打印」复用。
 */
export interface TenantStatementCtx {
  /** 当前视角：业主（全厂）/ 租户（分摊） */
  ownerView: boolean
  /** 某月的市电账单条目 */
  billOf: (month: string) => any
  /** 某月已生成的账单记录（分时核算的 ΔQ 优先取固化值） */
  recordOf: (month: string) => any | undefined
  /** 租户分摊子项（已去重、按 sort） */
  items: readonly any[]
  elecRows: readonly any[]
  waterMeters: readonly any[]
  waterReadingMap: ReadonlyMap<string, any>
  gasRows: readonly any[]
  /** 某行 → 分摊比例 */
  ratioOf: (row: any) => number
  /**
   * 某月某单位的**工业分表抄见合计**（不含宿舍/公共表）。
   *
   * 用于「本期总电量」卡片的「工业 + 差额 + 宿舍」三段式：
   * 差额 = 该租户工业总电量（含星达侧差额分摊）− 工业分表抄见合计。
   * 持睿汽车 2026-10：分表合计 166,550，工业总电量 172,400 → 差额 5,850。
   * 可选：单测/其他调用方不传时退化为「工业 + 宿舍」两段。
   */
  subMeterKwhOf?: (month: string, unit: string) => number
  /** 该单位本次打印是否并入气费（由 gasApplicableOf 判定） */
  includeGas: boolean
  /** 构造分项解析上下文（sourceCtxOf 的等价物） */
  sourceCtxOf: (month: string, bill: any, solarItems: any[], rec: any) => SourceResolveContext
}

/**
 * 「一、账单概览」的指标卡。
 *
 * 业主视角 = 全厂口径（市电账单本期电量/电费）；租户视角**绝不能**继续沿用
 * 全厂数字 —— 真机：持睿汽车的 PDF 概览写着「本期总电量 596168 / 本期电费合计
 * 433433.07」，那是星达整张市电账单，租户拿到的单据上不是自己的数。
 *
 * 租户口径按「工业 + 差额 + 宿舍」三段拆分，三段之和即该租户当期应收：
 *   电量 = 工业分表抄见 + 差额（星达工业总量 − 工业分表抄见）+ 宿舍电表用电量
 *   电费 = 分摊后工业电费 + 宿舍电费
 *   水量 = 分表水表用水量 + 宿舍水表用水量；水费同理。
 *
 * 为什么电量要拆出「差额」：租户的工业电量取的是星达侧工业总量（172,400），
 * 而三个工业分表只抄到 166,550，中间 5,850 是线路损耗/公摊等无法分表计量的部分。
 * 只写「工业 + 宿舍」会让用户算不平（166,550 + 686 ≠ 173,086），
 * 明细一摊开就露馅。
 */
function statementKpis(ctx: TenantStatementCtx, rows: any[], unit = ''): Array<{ label: string; value: string; hint?: string }> {
  const sum = (k: string) => rows.reduce((s: number, r: any) => s + (Number(r[k]) || 0), 0)
  if (ctx.ownerView) {
    return [
      { label: '本期总电量', value: `${num2(sum('total_kwh'))} kWh` },
      { label: '本期电费合计', value: `${money2(sum('total_fee'))} 元` },
      { label: '工商业电费', value: `${money2(sum('ind_fee'))} 元` },
      { label: '应收合计', value: `${money2(sum('ind_fee') + sum('dorm_fee') + sum('res_fee') + sum('water_ind_fee') + sum('water_dorm_fee') + sum('water_fire_fee') + sum('gas_fee'))} 元` },
    ]
  }
  const indKwh = sum('ind_kwh')
  const dormKwh = sum('dorm_kwh')
  const indFee = sum('ind_fee')
  const dormFee = sum('dorm_fee') + sum('res_fee')
  const wIndU = sum('water_ind_usage')
  const wDormU = sum('water_dorm_usage')
  const wIndF = sum('water_ind_fee')
  const wDormF = sum('water_dorm_fee') + sum('water_fire_fee')
  // 工业电量拆成「分表抄见 + 差额」：差额可正可负（分表合计可能超过总量）
  const months = Array.from(new Set(rows.map((r: any) => String(r.month || '')).filter(Boolean)))
  const subMeter = ctx.subMeterKwhOf && months.length
    ? months.reduce((s, m) => s + (Number(ctx.subMeterKwhOf!(m, unit)) || 0), 0)
    : 0
  const diff = round2(indKwh - subMeter)
  const hasSplit = subMeter > 0
  const elecHint = hasSplit
    ? `工业 ${num2(subMeter)} + 差额 ${num2(diff)} + 宿舍 ${num2(dormKwh)}`
    : `工业 ${num2(indKwh)} + 宿舍 ${num2(dormKwh)}`
  return [
    { label: '本期总电量', value: `${num2(indKwh + dormKwh)} 度`, hint: elecHint },
    { label: '本期总电费', value: `${money2(indFee + dormFee)} 元`, hint: `工业 ${money2(indFee)} + 宿舍 ${money2(dormFee)}` },
    { label: '本期用水量', value: `${num2(wIndU + wDormU)} 吨`, hint: `分表 ${num2(wIndU)} + 宿舍 ${num2(wDormU)}` },
    { label: '本期总水费', value: `${money2(wIndF + wDormF)} 元`, hint: `分表 ${money2(wIndF)} + 宿舍 ${money2(wDormF)}` },
  ]
}

/** 租户视角概览里的「电水合计应收」：指标卡只有四张，合计金额写成一行元信息。 */
function statementMeta(ctx: TenantStatementCtx, rows: any[], unit: string, months: string[], ratio: number, includeGas = false) {
  const sum = (k: string) => rows.reduce((s: number, r: any) => s + (Number(r[k]) || 0), 0)
  const power = sum('ind_fee') + sum('dorm_fee') + sum('res_fee')
  const water = sum('water_ind_fee') + sum('water_dorm_fee') + sum('water_fire_fee')
  const gas = includeGas ? sum('gas_fee') : 0
  const tenantTotal = power + water + gas
  return [
    { label: '账单周期', value: months.join('、') },
    { label: '使用单位', value: unit },
    { label: '计费方式', value: ctx.ownerView ? '业主全厂（不分摊）' : '按用电量分摊比例折算' },
    {
      label: '分摊比例',
      // 只显示比例本身。第 37 轮：删除「（仅基本电费、功率因素调整电费按比例）」
      // 这段括号说明 —— 打印页 metas 是两列排布，括号一长就换行挤掉后一行；
      // 适用范围属于口径细节，不进对外单据。
      value: ctx.ownerView ? '—' : (ratio > 0 ? `${(ratio * 100).toFixed(2)}%` : '—'),
    },
    { label: '账单户号', value: String(ctx.billOf(months[0])?.item?.account_no || '—') },
    // 应收合计必须与列表「合计应付」逐分相等：
    //   业主 = 电 + 水 + 气（三块都有列）；
    //   租户 = 工业电费 + 宿舍电费 + 水费（+ 该租户确有气表时并入气费）。
    // 租户没有气表时**绝不能**并入 gas_fee —— 旧实现会把全厂气费加进来，
    // 2026-09 持睿汽车因此多出 1,188.20（那是星达的气费）。
    {
      label: ctx.ownerView || includeGas ? '应收合计' : '电水合计应收',
      value: `${money2(ctx.ownerView ? power + water + sum('gas_fee') : tenantTotal)} 元`,
    },
    { label: '生成时间', value: new Date().toLocaleString('zh-CN', { hour12: false }) },
  ]
}

/**
 * 第 40 轮：打印预览「费用类别 + 费用组成」双列 + 跨行合并。
 *
 * 旧实现每一行第 1 列都拼成「(1)市场化购电费 | 零售交易电费」，
 * 重复啰嗦、列宽被挤到折行；更严重的是排序用拼接串 `includes('上网环节线损')`
 * 匹配，把「(4)系统运行费 | 上网环节线损代理采购损益」吸到了 (2) 线损组里，
 * 与原账单顺序不符。
 *
 * 现在：
 *   · 先按**大类**分组（规范化成简洁名），排序只认大类、绝不认细项名；
 *   · 同一大类的所有明细行与小计行共用 1 个「费用类别」单元格（rowspan）；
 *   · 同名分时项（尖/峰/平/谷 4 行）共用 1 个「费用组成」单元格（rowspan=4）；
 *   · 系统运行费用 / 政府性基金及附加 / 市场化购电费 / 输配电量电费
 *     在大类末尾补小计行。
 */

/** 分项行的中间结构（组装前按大类分组）。 */
interface FeeLine {
  cat: string
  name: string
  period: string
  qty: string
  rate: string
  amount: string
  /** 未取整金额，用于大类小计与整表合计；小计行自身为 0（不重复累加） */
  exact: number
  isSubtotal?: boolean
}

/** 大类规范化 + 排序权重（严格按账单分类，不用细项名做子串匹配）。 */
const FEE_GROUPS: Array<{ match: RegExp; label: string; rank: number; subtotal?: string }> = [
  { match: /市场化购电费/, label: '(1) 市场化购电费', rank: 1, subtotal: '市场化购电费（小计）' },
  { match: /上网环节线损费/, label: '(2) 上网环节线损费', rank: 2 },
  { match: /输配电量电费|输配电费/, label: '(3) 输配电量电费', rank: 3, subtotal: '输配电量电费（小计）' },
  // 系统运行费排在线损之后判，但**分类取的是 cat（大类）**，细项名里的
  // 「上网环节线损代理采购损益」不会参与匹配 —— 这正是旧实现的 bug。
  { match: /系统运行/, label: '(4) 系统运行费用', rank: 4, subtotal: '系统运行费用（小计）' },
  { match: /政府性基金/, label: '(6) 政府性基金及附加', rank: 5, subtotal: '政府性基金及附加（小计）' },
  { match: /居民/, label: '居民电费', rank: 6 },
  { match: /基本电费/, label: '基本电费', rank: 7 },
  { match: /功率因(?:素|数)调整|力调/, label: '功率因素调整电费', rank: 8 },
  { match: /容量|需量/, label: '容（需）量电费', rank: 9 },
]

function feeGroupOf(cat: string) {
  const s = String(cat || '').trim()
  const hit = FEE_GROUPS.find((g) => g.match.test(s))
  return hit || { label: s || '其他费用', rank: 99, subtotal: undefined as string | undefined }
}

/** 细项名去大类前缀：账单里有些 name 自带「（1）市场化购电费.偏差电费」。 */
function cleanFeeName(cat: string, rawName: string): string {
  let s = String(rawName || '').trim()
  if (!s) return '—'
  // 去掉全角/半角竖线拼接符后面重复的大类前缀
  s = s.replace(/^.*[|｜]\s*/, '')
  s = s.replace(/^[（(]\d+[）)]\s*/, '')
  // 只有「大类前缀 + 细项名」这种拼接才剥前缀；name 与 cat 同名时
  // （基本电费 / 功率因素调整电费）整名保留，不能削成「—」。
  if (cat && s.length > cat.length && s.startsWith(cat)) s = s.slice(cat.length).replace(/^[.．:：\s]+/, '')
  return s || '—'
}

/** 把中间行组装成 6 列 + rowSpans 元数据。 */
function assembleFeeRows(lines: FeeLine[]): { rows: string[][]; rowSpans: number[][]; sum: number } {
  const groups = new Map<string, FeeLine[]>()
  for (const l of lines) {
    const g = feeGroupOf(l.cat)
    if (!groups.has(g.label)) groups.set(g.label, [])
    groups.get(g.label)!.push(l)
  }
  const order = Array.from(groups.keys()).sort((a, b) => feeGroupOf(a).rank - feeGroupOf(b).rank)
  const rows: string[][] = []
  const rowSpans: number[][] = []
  let sum = 0
  for (const l of lines) if (!l.isSubtotal) sum += l.exact

  for (const cat of order) {
    const raw = groups.get(cat)!
    const spec = feeGroupOf(cat)
    // 大类小计 = **逐项 round2 后相加**（与列表大类汇总列同源）。
    //
    // 整表合计用的是「未取整原值累加后再 round2」（Excel 口径，见 sum），
    // 两者在部分月份会差 1 分：2026-10 系统运行费用未取整累加 784.42、
    // 逐项取整相加 784.43（列表与需求口径）；2026-09 政府性基金
    // 未取整 6038.06、逐项取整 6038.07。小计必须对得上列表汇总列。
    const detailSum = raw.filter((l) => !l.isSubtotal)
      .reduce((s, l) => s + round2(l.exact), 0)
    // 大类小计：只有该大类确实有多行明细时才有意义（单行大类不必重复出一行）
    const withSubtotal = spec.subtotal && raw.filter((l) => !l.isSubtotal).length >= 2
      ? [...raw, {
        cat, name: spec.subtotal!, period: '—', qty: '—', rate: '—',
        amount: money2(detailSum), exact: 0, isSubtotal: true,
      } as FeeLine]
      : raw
    // 预计算「费用组成」列的跨行合并：连续 4 行同名且每个时段各一行。
    const nameSpans = withSubtotal.map(() => 1)
    for (let i = 0; i + 3 < withSubtotal.length; i++) {
      const head = withSubtotal[i]
      if (head.isSubtotal || !TIMED_LABELS.has(head.period)) continue
      const run = withSubtotal.slice(i, i + 4)
      if (run.some((x) => x.isSubtotal || x.name !== head.name || !TIMED_LABELS.has(x.period))) continue
      if (new Set(run.map((x) => x.period)).size !== 4) continue
      nameSpans[i] = 4
      for (let k = 1; k < 4; k++) nameSpans[i + k] = 0
      i += 3
    }
    withSubtotal.forEach((l, i) => {
      rows.push([i === 0 ? cat : '', l.name, l.period, l.qty, l.rate, l.amount])
      // 第 0 列：组首行 rowspan = 整组行数（含小计行），其余行为 0（被合并）
      const catSpan = i === 0 ? withSubtotal.length : 0
      rowSpans.push([catSpan, nameSpans[i], 1, 1, 1, 1])
    })
  }
  return { rows, rowSpans, sum }
}

const TIMED_LABELS = new Set(['尖峰', '峰', '平', '谷'])

/** 市电账单费用分项 → 细项表（租户视角按比例折算、且只列参与分摊的子项） */
function feeTableOf(ctx: TenantStatementCtx, month: string, unitRatio: number, prefix: string): StatementTable {
  const bill = ctx.billOf(month)
  const items = (bill?.item?.fee_items || []) as any[]
  const lines: FeeLine[] = []
  const push = (cat: string, name: string, period: string, qty: string, rate: string, amount: string, exact: number, isSubtotal = false) => {
    lines.push({ cat, name: cleanFeeName(cat, name), period, qty, rate, amount, exact, isSubtotal })
  }

  if (ctx.ownerView) {
    // 业主口径：账单 fee_items 全量 + 概况里的固定金额分项（基本电费/力调电费）。
    const map = new Map<string, any[]>()
    for (const it of items) {
      const cat = String(it.category || '其他费用')
      if (!map.has(cat)) map.set(cat, [])
      map.get(cat)!.push(it)
    }
    for (const [cat, list] of map) {
      for (const it of list) {
        const exact = (Number(it.fee) || 0) * unitRatio
        const qty = (Number(it.qty) || 0) * unitRatio
        push(cat, String(it.name || '').trim(), String(it.period || '—'),
          qty ? num2(qty) : '—', rateText(it.rate), money2(exact), exact)
      }
    }
  } else {
    // 租户口径：只列该租户「参与分摊」的子项；分时核算的 ΔQ 优先取账单固化值。
    const rec = ctx.recordOf(month)
    const rowCtx = ctx.sourceCtxOf(month, bill, [], rec)
    for (const it of ctx.items) {
      const key = String(it?.source_key || it?.item_name || '').trim()
      if (!key) continue
      const cat = String(it.category || '其他费用')
      // 固定金额分项（基本电费 / 力调电费）按概况字段精确除法折算
      const fixed = fixedBillFeeOf(rowCtx, key)
      if (fixed !== null) {
        const starQty = Number(rowCtx.starTotal) || 0
        const billQty = Number(rowCtx.bill?.item?.total_kwh) || 0
        const exact = starQty > 0 && billQty > 0 ? (fixed * starQty) / billQty : fixed * unitRatio
        const v = round2(exact)
        if (!v) continue
        push(cat, String(it.item_name || key).trim(), '—', '—', rateText(unitRatio), money2(v), exact)
        continue
      }
      // 「零售损益分摊电费」按 **账单原值 × 分摊比例**（第 39 轮口径）。
      // 打印预览此前仍写「工业总电量 × 单价 = 4,501.52」，与列表 4,434.03
      // 分叉；本轮一并对齐，分项合计也随之回到已锁定的 101,346.55。
      if (key.includes('零售损益分摊')) {
        const billRows = matchBillFeeItems(rowCtx, key, cat)
        const raw = round2(billRows.reduce((acc: number, r: any) => acc + (Number(r?.fee) || Number(r?.fee_amount) || 0), 0))
        if (!raw || unitRatio <= 0) continue
        const exact = raw * unitRatio
        push(cat, key, '—', num2(raw), `${(unitRatio * 100).toFixed(2)}%`, money2(exact), exact)
        continue
      }
      // 分时项：与列表同一个核算引擎（逐表逐时段基准电量 × 时段单价）
      const timed = resolveTimedItemFee(rowCtx, key, cat)
      if (timed) {
        const name = String(it.item_name || key).trim()
        let subQty = 0
        let subFee = 0
        for (const p of timed.periods) {
          subQty += p.qty
          subFee += p.fee
          push(cat, name, p.period, num2(p.qty), rateText(p.rate), money2(p.fee), p.fee)
        }
        push(cat, `${name}（小计）`, '四段合计', num2(subQty), '—', money2(subFee), 0, true)
        continue
      }
      const billRows = matchBillFeeItems(rowCtx, key, cat)
      if (billRows.length) {
        // 平段按度计费：计费数量 = 工业总电量，金额 = 电量 × 账单该项单价
        const qty = Number(rowCtx.starTotal) || 0
        const unit = Number(billRows[0]?.rate) || 0
        if (qty > 0 && unit) {
          push(cat, String(billRows[0].name || key).trim(), '—', num2(qty), rateText(unit), money2(qty * unit), qty * unit)
          continue
        }
        // 无单价可用（纯金额返还类）：退回逐行比例折算
        for (const br of billRows) {
          const exact = (Number(br.fee) || 0) * unitRatio
          const qty2 = (Number(br.qty) || 0) * unitRatio
          push(cat, String(br.name || key).trim(), String(br.period || '—'),
            qty2 ? num2(qty2) : '—', rateText(br.rate), money2(exact), exact)
        }
      } else {
        const v = resolveTenantItemValue(it, rowCtx, unitRatio)
        if (!v) continue
        push(cat, String(it.item_name || key).trim(), '—', '—', rateText(unitRatio), money2(v), v)
      }
    }
  }
  const { rows, rowSpans, sum } = assembleFeeRows(lines)
  return {
    title: `${prefix}电费分项细项`,
    columns: [
      { label: '费用类别', weight: 1.7, align: 'center' },
      { label: '费用组成', weight: 2.6 },
      { label: '时段', weight: 0.8, align: 'center' },
      { label: '计费数量', weight: 1.2, align: 'right' },
      { label: '单价/标准', weight: 1.1, align: 'right' },
      { label: '金额(元)', weight: 1.3, align: 'right' },
    ],
    rows,
    rowSpans,
    total: ['合计', '', '', '', '', money2(sum)],
    empty: ctx.ownerView ? '当月无市电账单分项数据' : '当月无参与分摊的费用分项',
  }
}

/** 电费核算抄表明细（分表 / 宿舍表），与主表同口径：用量 × 单价 = 金额 */
function elecTableOf(ctx: TenantStatementCtx, month: string, unit: string, prefix: string): StatementTable {
  const rows = ctx.elecRows.filter((r: any) => r.month === month && String(r.use_unit || '').trim() === unit)
  const body = rows.map((r: any) => [
    r.meter_alias || '—',
    r.meter_kind === 'dorm' ? '宿舍' : '工商业',
    num2(r.start_reading), num2(r.end_reading), num2(r.rate),
    num2(r.usage), rateText(r.unit_price), money2(r.amount),
  ])
  return {
    title: `${prefix}电费核算明细`,
    columns: [
      { label: '电表', weight: 2.2 },
      { label: '用途', weight: 1, align: 'center' },
      { label: '起度', weight: 1.1, align: 'right' },
      { label: '止度', weight: 1.1, align: 'right' },
      { label: '倍率', weight: 0.8, align: 'right' },
      { label: '用电量', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '电费(元)', weight: 1.3, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', '', '', num2(sumBy(rows, 'usage')), '', money2(sumBy(rows, 'amount'))],
    empty: '当月无电费核算记录',
  }
}

/** 水费核算明细（独立区块，绝不与电费合表） */
function waterTableOf(ctx: TenantStatementCtx, month: string, unit: string, prefix: string): StatementTable {
  const meters = (ctx.waterMeters as any[]).filter((m: any) =>
    m.enabled !== false && (m.owner_unit === unit || m.use_unit === unit))
  const rows: any[] = []
  for (const m of meters) {
    const r = ctx.waterReadingMap.get(`${m.id}__${month}`)
    if (r) rows.push({ ...r, alias: m.alias })
  }
  const body = rows.map((r: any) => [
    r.alias || '—',
    num2(r.start_reading), num2(r.end_reading), num2(r.usage),
    rateText(r.unit_price ?? r.price), money2(r.amount),
  ])
  return {
    title: `${prefix}水费核算明细`,
    columns: [
      { label: '水表', weight: 2.6 },
      { label: '起度', weight: 1.2, align: 'right' },
      { label: '止度', weight: 1.2, align: 'right' },
      { label: '用量(吨)', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '水费(元)', weight: 1.4, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', num2(rows.reduce((s, r) => s + (Number(r.usage) || 0), 0)), '', money2(rows.reduce((s, r) => s + (Number(r.amount) || 0), 0))],
    empty: '当月无用水记录',
  }
}

/** 气费核算明细（独立区块；公租房不参与核算） */
function gasTableOf(ctx: TenantStatementCtx, month: string, unit: string, prefix: string): StatementTable {
  const rows = ctx.gasRows.filter((r: any) =>
    r.month === month && String(r.use_unit || '').trim() === unit && r.meter_kind !== 'public')
  const body = rows.map((r: any) => [
    r.meter_alias || '—',
    num2(r.start_reading), num2(r.end_reading), num2(r.usage),
    rateText(r.unit_price), money2(r.amount),
  ])
  return {
    title: `${prefix}气费核算明细`,
    columns: [
      { label: '气表', weight: 2.6 },
      { label: '起度', weight: 1.2, align: 'right' },
      { label: '止度', weight: 1.2, align: 'right' },
      { label: '用量(m³)', weight: 1.2, align: 'right' },
      { label: '单价', weight: 1, align: 'right' },
      { label: '气费(元)', weight: 1.4, align: 'right' },
    ],
    rows: body,
    total: ['合计', '', '', num2(rows.reduce((s, r) => s + (Number(r.usage) || 0), 0)), '', money2(rows.reduce((s, r) => s + (Number(r.amount) || 0), 0))],
    empty: '当月无燃气记录',
  }
}

/**
 * 该单位本次打印是否适用「气费」。
 *
 * 两道条件缺一不可（真机 2026-09 持睿汽车误出气费 1,188.20 的根因）：
 *   ① 该单位确有气表记录（当月、非公租房、使用单位精确匹配）；
 *   ② 当前视角的字段定义里「气费」列是启用的（用户可在设置里停用该字段）。
 * 任一不满足 → 既不出「气费核算明细」表，也不出「应收费用汇总」里的气费行。
 */
export function gasApplicableOf(
  gasRows: readonly any[],
  columnKeys: readonly string[],
  unit: string,
  months: readonly string[],
): boolean {
  const hasMeter = gasRows.some((r: any) =>
    r.meter_kind !== 'public'
    && String(r.use_unit || '').trim() === unit
    && months.includes(String(r.month || '')))
  if (!hasMeter) return false
  return columnKeys.includes('gas_fee')
}

/**
 * 应收汇总：工商业电费 / 居民宿舍电费 / 水费 →（适用时）气费 → 合计。
 *
 * `includeGas` 由 `gasApplicableOf` 决定：没有气表的单位连「气费 0.00」这一行
 * 都不该出现 —— 一行恒为 0 的费用项会让租户以为「有气表但没用气」。
 */
function summaryTableOf(ctx: TenantStatementCtx, rows: any[], unit: string, prefix: string, includeGas: boolean): StatementTable {
  let ind = 0; let dorm = 0; let water = 0; let gas = 0
  for (const r of rows) {
    ind += Number(r.ind_fee) || 0
    dorm += (Number(r.dorm_fee) || 0) + (Number(r.res_fee) || 0)
    water += (Number(r.water_ind_fee) || 0) + (Number(r.water_dorm_fee) || 0) + (Number(r.water_fire_fee) || 0)
    gas += Number(r.gas_fee) || 0
  }
  const total = Math.round((ind + dorm + water + (includeGas ? gas : 0)) * 100) / 100
  const body: string[][] = [
    ['工商业电费（分摊后）', unit, money2(ind)],
    ['居民 / 宿舍电费', unit, money2(dorm)],
    ['水费（分表 + 宿舍）', unit, money2(water)],
  ]
  if (includeGas) body.push(['气费', unit, money2(gas)])
  return {
    title: `${prefix}应收费用汇总`,
    columns: [
      { label: '费用项目', weight: 3 },
      { label: '使用单位', weight: 2 },
      { label: '金额(元)', weight: 1.6, align: 'right' },
    ],
    rows: body,
    total: ['应收合计', unit, money2(total)],
    // 结论表：0 元也必须出现（租户要看到「本期应收多少」）
    alwaysShow: true,
  }
}

export function buildTenantBillingStatementDoc(ctx: TenantStatementCtx, rows: any[], unit: string): StatementDoc {
  const months = rows.map((r: any) => r.month).filter(Boolean)
  const single = rows.length === 1
  const ratioList = rows.map((r: any) => ctx.ratioOf(r)).filter((v) => v > 0)
  const ratio = ratioList.length ? ratioList[0] : 0
  // 气费是否适用：整份清单统一判定（同一单位、同一批月份），
  // 逐行判会让「多月份打印」里某几页有气费行、某几页没有。
  const includeGas = ctx.includeGas
  const tables: StatementTable[] = []
  rows.forEach((row: any) => {
    const month = String(row.month || '')
    const prefix = single ? '' : `${month} · `
    // 租户视角必须按真实分摊比例；ratio=0（当月未生成/未抄表）时按 0 列示，
    // 绝不能兜底成 1 —— 那会把「整张市电账单」当成该租户的份额打印出去。
    const r = ctx.ownerView ? 1 : ctx.ratioOf(row)
    tables.push(feeTableOf(ctx, month, r, prefix))
    tables.push(elecTableOf(ctx, month, unit, prefix))
    tables.push(waterTableOf(ctx, month, unit, prefix))
    if (includeGas) {
      const gas = gasTableOf(ctx, month, unit, prefix)
      if (gas.rows.length) tables.push(gas)
    }
    tables.push(summaryTableOf(ctx, [row], unit, prefix, includeGas))
  })
  return {
    title: `${unit} 费用清单`,
    subtitle: `${months.join('、')} · 使用单位 ${unit} · ${ctx.ownerView ? '业主全厂口径' : '租户分摊口径'}`,
    overviewTitle: '一、账单概览',
    meta: statementMeta(ctx, rows, unit, months, ratio, includeGas),
    kpis: statementKpis(ctx, rows, unit),
    tables,
    footerNote: ctx.ownerView || ratio > 0
      ? '说明：市电分项按账单费用组成逐项列示；租户视角下分项电量与金额均已按当月用电量分摊比例折算；水、电、气核算明细按使用单位直接归属，单独成表。'
      : '说明：当月尚未生成该租户的分摊结果（无市电账单或缺少分时抄表），电费分项暂按 0 列示；水、电、气核算明细按使用单位直接归属。',
  }
}
