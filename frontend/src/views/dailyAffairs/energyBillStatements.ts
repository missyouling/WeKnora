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
import type { StatementDoc, StatementTable } from './useCatalogPdf'

const money = (v: any): string => {
  const n = Number(v)
  return Number.isFinite(n) ? (Math.round(n * 100) / 100).toFixed(2) : '—'
}
const num = (v: any): string => {
  const n = Number(v)
  if (!Number.isFinite(n)) return '—'
  const r = Math.round(n * 100) / 100
  return Number.isInteger(r) ? String(r) : r.toFixed(2)
}
const rate = (v: any): string => {
  const n = Number(v)
  if (!Number.isFinite(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}
const text = (v: any, fallback = '—'): string => {
  const s = String(v ?? '').trim()
  return s || fallback
}
const sum = (arr: any[], key: string) => Math.round(arr.reduce((s, r) => s + (Number(r?.[key]) || 0), 0) * 100) / 100
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
