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

/** 打印分项的分类排序基准（账单原文顺序：市场化 → 线损 → 输配 → 系统运行 → 基金）。 */
const FEE_CATEGORY_ORDER = ['市场化购电费', '上网环节线损', '输配电', '系统运行费', '政府性基金及附加', '居民', '基本电费', '功率因素调整电费']

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
 * 租户口径按「工业 + 宿舍」两段拆分，两段之和即该租户当期应收：
 *   电量 = 分表总电量（含差额分摊）+ 宿舍电表用电量
 *   电费 = 分摊后工业电费 + 宿舍电费
 *   水量 = 分表水表用水量 + 宿舍水表用水量；水费同理。
 */
function statementKpis(ctx: TenantStatementCtx, rows: any[]): Array<{ label: string; value: string; hint?: string }> {
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
  return [
    { label: '本期总电量', value: `${num2(indKwh + dormKwh)} 度`, hint: `工业 ${num2(indKwh)} + 宿舍 ${num2(dormKwh)}` },
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
      // 需求原文要求带适用范围：这个比例**只**作用于基本电费与力调电费，
      // 其余分项按分时电量或工业总电量 × 单价算，不乘它。
      value: ctx.ownerView ? '—' : (ratio > 0 ? `${(ratio * 100).toFixed(2)}%（仅基本电费、功率因素调整电费按比例）` : '—'),
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

/** 市电账单费用分项 → 细项表（租户视角按比例折算、且只列参与分摊的子项） */
function feeTableOf(ctx: TenantStatementCtx, month: string, unitRatio: number, prefix: string): StatementTable {
  const bill = ctx.billOf(month)
  const items = (bill?.item?.fee_items || []) as any[]
  const rows: string[][] = []
  let sum = 0
  const label = (cat: string, rawName: string) =>
    // 提取出来的 name 通常已带「（1）市场化购电费.偏差电费」这类前缀，
    // 再拼一次分类会变成「（1）市场化购电费｜（1）市场化购电费.偏差电费」。
    (rawName && cat && !rawName.includes(cat) ? `${cat}｜${rawName}` : (rawName || cat))

  if (ctx.ownerView) {
    // 业主口径：账单 fee_items 全量 + 概况里的固定金额分项（基本电费/力调电费），
    // 后者不是 fee_items 的行，必须单独补，否则全厂口径也会漏掉这两笔大额费用。
    const map = new Map<string, any[]>()
    for (const it of items) {
      const cat = String(it.category || '其他费用')
      if (!map.has(cat)) map.set(cat, [])
      map.get(cat)!.push(it)
    }
    const order = Array.from(map.keys()).sort((a, b) => {
      const ia = FEE_CATEGORY_ORDER.findIndex((c) => a.includes(c))
      const ib = FEE_CATEGORY_ORDER.findIndex((c) => b.includes(c))
      return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
    })
    for (const cat of order) {
      for (const it of map.get(cat)!) {
        const exact = (Number(it.fee) || 0) * unitRatio
        const qty = (Number(it.qty) || 0) * unitRatio
        sum += exact
        rows.push([label(cat, String(it.name || '').trim()), it.period || '—', qty ? num2(qty) : '—', rateText(it.rate), money2(exact)])
      }
    }
  } else {
    // 租户口径：**只列该租户「参与分摊」的子项**。
    // 未参与分摊的子项（用户在分摊子项里关掉的偏差电费、绿电交易电费等）
    // 一律不打印 —— 旧实现直接铺账单 fee_items 全量，于是关掉开关的项
    // 依然以 4 行 0.00 的形式出现在租户清单上（真机问题）。
    // 必须带上已生成账单：分时核算的差额电量 ΔQ 优先取账单固化值
    // （rec.line_loss），缺失时才按「星达分表 − 分表合计」现算。
    // 不传 rec 会让打印口径与列表口径在个别月份分叉。
    const rec = ctx.recordOf(month)
    // 单行解析上下文（分时项 / 固定金额项 / 平段按度项的计算都靠它）
    const rowCtx = ctx.sourceCtxOf(month, bill, [], rec)
    for (const it of ctx.items) {
      const key = String(it?.source_key || it?.item_name || '').trim()
      if (!key) continue
      const cat = String(it.category || '其他费用')
      // 固定金额分项（基本电费 / 力调电费）优先按概况字段出账：
      // 账单里它们可能没有 fee_items 行（2026-09），也可能是一条全 0 的占位行
      // （2026-08），按行打印会打出 0.00 的假明细。
      const fixed = fixedBillFeeOf(rowCtx, key)
      if (fixed !== null) {
        // 按「原金额 × 工业电量 ÷ 账单电量」折算，**不要** × 8 位 ratio：
        // 44,000 × 0.21235625 = 9,343.675 → 9,343.68，而标准表是 9,343.67
        // （精确除法 44,000 × 126,600 ÷ 596,168 = 9,343.6749）。
        const starQty = Number(rowCtx.starTotal) || 0
        const billQty = Number(rowCtx.bill?.item?.total_kwh) || 0
        const exact = starQty > 0 && billQty > 0
          ? (fixed * starQty) / billQty
          : fixed * unitRatio
        const v = round2(exact)
        if (!v) continue
        // 累加**未取整**值：Excel 的合计是「各格原值相加后再四舍五入」，
        // 逐项 round 再相加会让合计比列表的工业电费多 1 分。
        sum += exact
        rows.push([label(cat, String(it.item_name || key).trim()), '—', '—', rateText(unitRatio), money2(v)])
        continue
      }
      // 分时项（零售交易电费 / 零售输配电费…）：与列表**同一个**核算引擎
      // （resolveTimedItemFee → 逐表逐时段基准电量 × 时段单价）。
      // 旧实现对分时项也走「账单分时行 × 比例」，于是 PDF 打出的是
      // 5228.63 / 2279.68 这类旧值，与列表的 5085.43 / 2217.25 对不上。
      const timed = resolveTimedItemFee(rowCtx, key, String(it.category || ''))
      if (timed) {
        const name = label(cat, String(it.item_name || key).trim())
        let subQty = 0
        let subFee = 0
        for (const p of timed.periods) {
          sum += p.fee
          subQty += p.qty
          subFee += p.fee
          rows.push([name, p.period, num2(p.qty), rateText(p.rate), money2(p.fee)])
        }
        // 逐段之后补一行小计：分时项有 4 行明细，没有小计的话
        // 「PDF 的 54422.79 与列表的 54422.79 一致」只能靠人脑加总验算。
        rows.push([`${name}（小计）`, '四段合计', num2(subQty), '—', money2(subFee)])
        continue
      }
      const billRows = matchBillFeeItems(rowCtx, key, String(it.category || ''))
      if (billRows.length) {
        // 平段按度计费（零售损益分摊 / 上网环节线损 / 系统运行费 / 政府性基金）：
        //   **计费数量 = 工业总电量（126,600），金额 = 电量 × 账单该项单价**
        //
        // 旧实现在这里逐行写「账单金额 × 比例、账单数量 × 比例」，
        // 打印出来是「124,701.81 度 / 4,434.02 元」，而 Excel 是
        // 「126,600 度 / 4,501.52 元」—— 账单按度项的计费电量是 587,230
        // （全厂 596,168 − 居民 8,938），比例法系统性少算。
        const qty = Number(rowCtx.starTotal) || 0
        const unit = Number(billRows[0]?.rate) || 0
        if (qty > 0 && unit) {
          // 累加未取整值（同固定金额分项）：政府性基金 5 项逐项 round 再相加
          // 会得 6,038.07，而标准表与工业电费用的是 6,038.06。
          sum += qty * unit
          rows.push([label(cat, String(billRows[0].name || key).trim()), '—', num2(qty), rateText(unit), money2(qty * unit)])
          continue
        }
        // 无单价可用（纯金额返还类）：退回逐行比例折算
        for (const br of billRows) {
          const exact = (Number(br.fee) || 0) * unitRatio
          const qty2 = (Number(br.qty) || 0) * unitRatio
          sum += exact
          rows.push([label(cat, String(br.name || key).trim()), br.period || '—', qty2 ? num2(qty2) : '—', rateText(br.rate), money2(exact)])
        }
      } else {
        // 账单里查无此行：按比例折算概况字段，避免整行丢失
        const v = resolveTenantItemValue(it, rowCtx, unitRatio)
        if (!v) continue
        sum += v
        rows.push([label(cat, String(it.item_name || key).trim()), '—', '—', rateText(unitRatio), money2(v)])
      }
    }
    rows.sort((a, b) => {
      const ia = FEE_CATEGORY_ORDER.findIndex((c) => a[0].includes(c))
      const ib = FEE_CATEGORY_ORDER.findIndex((c) => b[0].includes(c))
      return (ia === -1 ? 999 : ia) - (ib === -1 ? 999 : ib)
    })
  }
  return {
    title: `${prefix}电费分项细项`,
    columns: [
      { label: '费用组成', weight: 3.4 },
      { label: '时段', weight: 1, align: 'center' },
      { label: '计费数量', weight: 1.3, align: 'right' },
      { label: '单价/标准', weight: 1.2, align: 'right' },
      { label: '金额(元)', weight: 1.4, align: 'right' },
    ],
    rows,
    total: ['合计', '', '', '', money2(sum)],
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
    kpis: statementKpis(ctx, rows),
    tables,
    footerNote: ctx.ownerView || ratio > 0
      ? '说明：市电分项按账单费用组成逐项列示；租户视角下分项电量与金额均已按当月用电量分摊比例折算；水、电、气核算明细按使用单位直接归属，单独成表。'
      : '说明：当月尚未生成该租户的分摊结果（无市电账单或缺少分时抄表），电费分项暂按 0 列示；水、电、气核算明细按使用单位直接归属。',
  }
}
