import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import {
  buildTenantBillingStatementDoc,
  gasApplicableOf,
  type TenantStatementCtx,
} from './energyBillStatements'
import type { SourceResolveContext } from './tenantBillingSources'

/**
 * 费用清单打印清单（结算单）的构造契约（第 22 轮 P1-2）。
 *
 * 构造器原本内联在 `TenantBilling.vue` 里约 435 行，改一处概览口径要在 3000 行的
 * SFC 里找，而且完全无法单测 —— 只能靠「打印一次、肉眼比对」。迁到
 * `energyBillStatements.ts` 后它是纯函数，可以用同一份 Excel 黄金基准直接断言
 * 「PDF 上的合计 = 列表的合计 = 标准表」。
 *
 * 这里只覆盖**数值口径**与**区块取舍**（气费该不该出、分时项怎么分行）；
 * 版式（字号/间距/分页）由 `useCatalogPdf` 的渲染单测与真机验收负责。
 */

const GOLDEN = JSON.parse(
  readFileSync(
    fileURLToPath(new URL('../../../../testdata/billing-golden-2026-09.json', import.meta.url)),
    'utf8',
  ),
) as any

const RATIO = Math.round((GOLDEN.inputs.starTotalKwh / GOLDEN.inputs.billTotalKwh) * 1e8) / 1e8

/** 账单对象（与 loadUtilityBillByMonth 读出来的形态一致）。 */
const BILL = {
  item: {
    total_kwh: GOLDEN.inputs.billTotalKwh,
    capacity_fee: GOLDEN.inputs.capacityFee,
    pf_adjust_amount: GOLDEN.inputs.pfAdjustAmount,
    fee_items: GOLDEN.inputs.feeItems,
    meter_readings: GOLDEN.inputs.meterReadings,
  },
}

/** 参与分摊的子项（与清单设置里的 item_key 同构）。 */
const ITEMS = [
  ['(1)市场化购电费', '零售交易电费'],
  ['(1)市场化购电费', '零售损益分摊电费'],
  ['(3)输配电量电费', '零售输配电费'],
  ['(2)上网环节线损费用', '上网环节线损费用'],
  ['(4)系统运行费', '燃气机组容量电费'],
  ['(4)系统运行费', '新能源机制差价分摊电费'],
  ['(4)系统运行费', '电价交叉补贴新增损益'],
  ['(4)系统运行费', '上网环节线损代理采购损益'],
  ['(4)系统运行费', '其他系统运行费用'],
  ['(4)系统运行费', '抽水蓄能容量电费'],
  ['(4)系统运行费', '煤电容量电费'],
  ['(6)政府性基金及附加-工商业', '国家重大水利工程建设基金'],
  ['(6)政府性基金及附加-工商业', '可再生能源附加'],
  ['(6)政府性基金及附加-工商业', '库区移民基金'],
  ['(6)政府性基金及附加-工商业', '农网还贷'],
  ['(6)政府性基金及附加-工商业', '小型水库移民后期扶持资金(地方)'],
  ['基本电费', '基本电费'],
  ['功率因素调整电费', '功率因素调整电费'],
].map(([category, item_name]) => ({
  item_name,
  category,
  source_key: item_name,
  source_category: '市电账单',
  source_mode: 'fee_row',
  enabled: true,
}))

function sourceCtxOf(): SourceResolveContext {
  return {
    month: GOLDEN.month,
    tenantName: GOLDEN.tenantName,
    bill: BILL,
    solarItems: [],
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    timedMeters: GOLDEN.inputs.timedMeters,
    deltaQ: GOLDEN.inputs.deltaQ,
    ratio: RATIO,
    starTotal: GOLDEN.inputs.starTotalKwh,
  }
}

/** 一行费用清单（列表行的最小形态）。 */
const ROW = {
  id: 'r1',
  month: GOLDEN.month,
  unit: GOLDEN.tenantName,
  total_kwh: GOLDEN.inputs.starTotalKwh,
  ind_kwh: GOLDEN.inputs.starTotalKwh,
  ind_fee: GOLDEN.expected.summary.industrialFee,
  total_fee: GOLDEN.expected.summary.grandTotalFee,
  dorm_kwh: GOLDEN.inputs.dormKwh,
  dorm_fee: GOLDEN.inputs.dormFee,
  res_fee: 0,
  water_ind_fee: GOLDEN.inputs.waterFee,
  water_dorm_fee: 0,
  water_fire_fee: 0,
  gas_fee: 0,
}

function ctxOf(overrides: Partial<TenantStatementCtx> = {}): TenantStatementCtx {
  return {
    ownerView: false,
    billOf: (month: string) => (month === GOLDEN.month ? BILL : undefined),
    recordOf: (month: string) =>
      month === GOLDEN.month ? { month, line_loss: GOLDEN.inputs.deltaQ, ratio: RATIO } : undefined,
    items: ITEMS,
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    ratioOf: () => RATIO,
    includeGas: false,
    sourceCtxOf: () => sourceCtxOf(),
    ...overrides,
  }
}

const findTable = (doc: any, keyword: string) =>
  doc.tables.find((t: any) => String(t.title).includes(keyword))

test('打印清单：电费分项细项合计 = 工业电费 101346.55（与列表 / Excel 同源）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  assert.ok(fee, '必须有「电费分项细项」区块')
  assert.equal(fee.total[5], '101346.55', 'PDF 分项合计必须等于工业电费')
})

test('打印清单：分时项逐段出行（第 41 轮删除独立小计行）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const rows: string[][] = fee.rows

  // 第 40 轮：6 列结构（费用类别 | 费用组成 | 时段 | 计费数量 | 单价/标准 | 金额）
  const trade = rows.filter((r) => r[1].includes('零售交易电费') && !r[1].includes('小计'))
  assert.equal(trade.length, 4, '零售交易电费必须拆成尖/峰/平/谷四行')
  // 四段金额与标准表逐分一致
  assert.deepEqual(trade.map((r) => r[5]), ['2190.59', '15541.36', '18153.06', '18513.56'])
  // 第 41 轮：四段小计行已删除，小计金额内嵌到「费用类别」列
  assert.ok(!rows.some((r) => r[1].includes('零售交易电费（小计）')),
    '不得再渲染「零售交易电费（小计）」独立行')
})

test('打印清单：零售损益按 账单原值 × 分摊比例', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const market = fee.rows.find((r: string[]) => r[1].includes('零售损益分摊电费'))
  assert.ok(market, '必须有零售损益分摊电费')
  assert.equal(market![3], '20880.13', '计费数量是账单原值（第 39 轮按比例分摊口径）')
  assert.equal(market![4], '21.24%')
  assert.equal(market![5], '4434.03')
})

test('打印清单：基本电费 / 力调电费按精确除法折算，单独成行', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const base = fee.rows.find((r: string[]) => r[1] === '基本电费')
  const pf = fee.rows.find((r: string[]) => r[1].includes('功率因素调整电费'))
  assert.ok(base && pf, '基本电费与力调电费都必须出账')
  assert.equal(base![5], '9343.67')
  assert.equal(pf![5], '-598.43')
})

test('打印清单：概览指标按租户口径拆「工业 + 宿舍」', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const kpi = (label: string) => (doc.kpis || []).find((k: any) => k.label === label)
  assert.equal(kpi('本期总电量')?.value, '127530 度', '工业 126600 + 宿舍 930')
  assert.equal(kpi('本期总电费')?.value, '102276.55 元')
  // 构成说明必须写清两段
  assert.match(String(kpi('本期总电费')?.hint), /工业 101346\.55 \+ 宿舍 930\.00/)
})

test('打印清单：业主视角走全厂口径，不出租户两段拆分', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf({ ownerView: true }), [ROW], GOLDEN.tenantName)
  const kpi = (label: string) => (doc.kpis || []).find((k: any) => k.label === label)
  assert.equal(kpi('本期电费合计')?.value, `${GOLDEN.expected.summary.grandTotalFee} 元`)
  // 业主视角的电量卡是全厂 kWh（租户视角是「度」并带构成说明）
  assert.equal(kpi('本期总电量')?.value, `${GOLDEN.inputs.starTotalKwh} kWh`)
  assert.equal(kpi('本期总电量')?.hint, undefined, '业主视角不得出「工业 + 宿舍」构成说明')
})

test('打印清单：无气表单位不出气费明细表；应收汇总下线且为 2 页结构', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  assert.equal(findTable(doc, '气费核算明细'), undefined, '没有气表不得出气费表')
  assert.equal(findTable(doc, '应收费用汇总'), undefined, '第 42 轮：应收汇总章节必须删除')
  assert.equal((doc as any).footerNote, undefined, '底部说明文字必须删除')
  // 单月固定 3 张表：分项细项（第 1 页，末尾强制分页）+ 电费/水费核算明细（第 2 页）
  assert.equal(doc.tables.length, 3, '文档只保留分项细项 / 电费核算明细 / 水费核算明细 3 张表')
  assert.equal(doc.tables[0].pageBreakAfter, true, '分项表后必须强制分页，整表落在第 1 页')
  assert.ok(doc.tables[1].title.includes('电费核算明细'), '第 2 页从电费核算明细开始')
  assert.ok(doc.tables[2].title.includes('水费核算明细'), '第 2 页含水费核算明细')
})

test('打印清单：水费单独成表，不与电费合表', () => {
  const ctx = ctxOf({
    waterMeters: [{ id: 'w1', alias: '持睿总水表', enabled: true, owner_unit: GOLDEN.tenantName }],
    waterReadingMap: new Map([
      [`w1__${GOLDEN.month}`, { start_reading: 100, end_reading: 280, usage: 180, unit_price: 5.22, amount: 939.6 }],
    ]),
  })
  const doc = buildTenantBillingStatementDoc(ctx, [ROW], GOLDEN.tenantName)
  const water = findTable(doc, '水费核算明细')
  assert.ok(water, '必须有独立的水费核算明细表')
  assert.equal(water.rows.length, 1)
  assert.equal(water.total[5], '939.60')
})

test('gasApplicableOf：有气表且气费列启用才并入气费', () => {
  const gasRows = [{ month: GOLDEN.month, use_unit: GOLDEN.tenantName, meter_kind: 'normal', usage: 100 }]
  assert.equal(gasApplicableOf(gasRows, ['gas_fee'], GOLDEN.tenantName, [GOLDEN.month]), true)
  // 列被用户停用 → 不并
  assert.equal(gasApplicableOf(gasRows, ['month', 'ind_fee'], GOLDEN.tenantName, [GOLDEN.month]), false)
  // 公租房不参与核算
  assert.equal(
    gasApplicableOf([{ ...gasRows[0], meter_kind: 'public' }], ['gas_fee'], GOLDEN.tenantName, [GOLDEN.month]),
    false,
  )
  // 没有该单位的气表
  assert.equal(gasApplicableOf(gasRows, ['gas_fee'], '别的单位', [GOLDEN.month]), false)
})


// ---------------------------------------------------------------------------
// 第 40 轮：打印预览「费用类别 + 费用组成」双列 + rowspan 跨行合并
// ---------------------------------------------------------------------------
//
// 旧版每行第一列拼成「(1)市场化购电费 | 零售交易电费」，重复啰嗦且折行；
// 更严重的是排序用拼接串 includes('上网环节线损') 匹配，把
// 「(4)系统运行费 | 上网环节线损代理采购损益」吸到了 (2) 线损组里。

test('打印预览：电费分项细项为 6 列，前两列是「费用类别」「费用组成」', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  assert.deepEqual(fee.columns.map((c: any) => c.label),
    ['费用类别', '费用组成', '时段', '计费数量', '单价/标准', '金额(元)'])
})

test('打印预览：第 1 列只有简洁大类，绝不再出现「| / ｜」拼接符', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  for (const r of fee.rows as string[][]) {
    assert.ok(!r[0].includes('|') && !r[0].includes('｜'), `费用类别含拼接符：${r[0]}`)
    assert.ok(!r[1].includes('|') && !r[1].includes('｜'), `费用组成含拼接符：${r[1]}`)
  }
  assert.ok((fee.rows as string[][]).some((r) => r[0].includes('市场化购电费')))
  assert.ok((fee.rows as string[][]).some((r) => r[0].includes('系统运行费用')))
})

test('打印预览：费用类别列用 rowspan 合并，同名分时项也 rowspan=4', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  assert.ok(Array.isArray(fee.rowSpans), 'StatementTable 必须带 rowSpans 元数据')
  const spans: number[][] = fee.rowSpans
  assert.equal(spans.length, fee.rows.length, 'rowSpans 与 rows 必须等长')
  // 第 0 列至少有一个 >1 的合并（系统运行费 8 行）
  const catSpans = spans.map((r) => r[0]).filter((v) => v > 1)
  assert.ok(catSpans.length >= 4, `至少 4 个大类应跨行合并，got ${catSpans.length}`)
  // 同名分时项（零售交易电费 4 行）第 1 列合并为 4
  const rows = fee.rows as string[][]
  const idx = rows.findIndex((r) => r[1] === '零售交易电费')
  assert.ok(idx >= 0, '应有零售交易电费分时行')
  assert.equal(spans[idx][1], 4, '零售交易电费四行的「费用组成」必须 rowspan=4')
})

test('打印预览：上网环节线损代理采购损益 严格归在 (4) 系统运行费用组内', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const rows: string[][] = fee.rows
  const spans: number[][] = fee.rowSpans
  const idx = rows.findIndex((r) => r[1] === '上网环节线损代理采购损益')
  assert.ok(idx >= 0, '代理采购损益必须在分项细项里')
  // 向上找它所属的大类合并起点
  let group = idx
  while (group > 0 && spans[group][0] === 0) group -= 1
  assert.match(rows[group][0], /系统运行费用/, `代理采购损益被放进了「${rows[group][0]}」组`)
  // 并且它必须排在输配电量电费之后（原缺陷：夹在 (2) 与 (3) 之间）
  const transIdx = rows.findIndex((r) => r[0].includes('输配电量电费'))
  assert.ok(transIdx >= 0 && transIdx < idx, '系统运行费用组必须排在输配电量电费之后')
})

// ---------------------------------------------------------------------------
// 第 41 轮：6 条独立小计行全部删除，大类小计内嵌到「费用类别」列（\n 双行）
// ---------------------------------------------------------------------------

const DELETED_SUBTOTAL_ROWS = [
  '零售交易电费（小计）', '市场化购电费（小计）', '零售输配电费（小计）',
  '输配电量电费（小计）', '系统运行费用（小计）', '政府性基金及附加（小计）',
]

test('打印预览：6 条独立小计行全部删除（只剩整表最底部合计行）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const rows: string[][] = fee.rows
  for (const name of DELETED_SUBTOTAL_ROWS) {
    assert.ok(!rows.some((r) => r[1] === name || r[1].includes(name)),
      `不得再渲染独立小计行「${name}」`)
  }
  // 基线 30 行 − 6 条小计 = 24 行明细；合计行是 total，不在 rows 里
  assert.equal(rows.length, 24, '分项明细行必须由 30 缩减为 24')
})

test('打印预览：大类小计以 \n 双行内嵌到「费用类别」合并格', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const rows: string[][] = fee.rows
  const want: Array<[RegExp, string]> = [
    [/市场化购电费/, '58832.60'],
    [/输配电量电费/, '18610.64'],
    [/系统运行费用/, '6949.58'],
    [/政府性基金及附加/, '6038.07'],
  ]
  for (const [catRe, amount] of want) {
    const cell = rows.find((r) => catRe.test(r[0]))?.[0] || ''
    assert.ok(cell.includes('\n'), `「${catRe}」第 1 列必须含换行，got ${JSON.stringify(cell)}`)
    assert.match(cell, new RegExp(`\\n（小计 ${amount.replace('.', '\\.')}）`),
      `「${catRe}」小计金额应为 ${amount}`)
  }
  // 单行类别不重复附加小计（右侧金额列已直接显示）
  for (const name of ['(2) 上网环节线损费', '基本电费', '功率因素调整电费']) {
    const cell = rows.find((r) => r[0].includes(name))?.[0] || ''
    assert.ok(!cell.includes('\n'), `单行类别「${name}」不应附加小计，got ${JSON.stringify(cell)}`)
  }
})

test('打印预览：内嵌小计不改变整表合计（仍是工业电费 101346.55）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  assert.equal(fee.total[5], '101346.55')
  // 分项表在第 1 页完成后强制分页，第 2 页从「电费核算明细」开始
  assert.equal(fee.pageBreakAfter, true, '分项表后必须强制分页')
})
