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

test('打印清单：电费分项细项合计 = 工业电费 101414.04（与列表 / Excel 同源）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  assert.ok(fee, '必须有「电费分项细项」区块')
  assert.equal(fee.total[4], '101414.04', 'PDF 分项合计必须等于工业电费')
})

test('打印清单：分时项逐段出行 + 小计行（不再按账单行 × 比例）', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const rows: string[][] = fee.rows

  // 表内名称形如「大类｜子项」，用 includes 匹配
  const trade = rows.filter((r) => r[0].includes('零售交易电费') && !r[0].includes('小计'))
  assert.equal(trade.length, 4, '零售交易电费必须拆成尖/峰/平/谷四行')
  // 四段金额与标准表逐分一致
  assert.deepEqual(trade.map((r) => r[4]), ['2190.59', '15541.36', '18153.06', '18513.56'])
  // 四段之后必须有小计行（否则「PDF 与列表一致」只能靠人脑加总）
  const subtotal = rows.find((r) => r[0].includes('零售交易电费（小计）'))
  assert.ok(subtotal, '分时项必须有（小计）行')
  assert.equal(subtotal![4], '54398.57')
  assert.equal(subtotal![2], '124767.36')
})

test('打印清单：平段按度项计费数量 = 工业总电量 126600', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const market = fee.rows.find((r: string[]) => r[0].includes('零售损益分摊电费'))
  assert.ok(market, '必须有零售损益分摊电费')
  assert.equal(market![2], '126600', '计费数量必须是工业总电量')
  assert.equal(market![3], '0.035557')
  assert.equal(market![4], '4501.52')
})

test('打印清单：基本电费 / 力调电费按精确除法折算，单独成行', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  const fee = findTable(doc, '电费分项细项')
  const base = fee.rows.find((r: string[]) => r[0] === '基本电费')
  const pf = fee.rows.find((r: string[]) => r[0].includes('功率因素调整电费'))
  assert.ok(base && pf, '基本电费与力调电费都必须出账')
  assert.equal(base![4], '9343.67')
  assert.equal(pf![4], '-598.43')
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

test('打印清单：无气表单位不出「气费核算明细」表，也不出应收汇总的气费行', () => {
  const doc = buildTenantBillingStatementDoc(ctxOf(), [ROW], GOLDEN.tenantName)
  assert.equal(findTable(doc, '气费核算明细'), undefined, '没有气表不得出气费表')
  const summary = findTable(doc, '应收费用汇总')
  assert.ok(summary)
  assert.ok(!summary.rows.some((r: string[]) => r[0] === '气费'), '应收汇总不得出现气费行')
  // 应收合计 = 工商业电费 + 宿舍电费 + 水费（不是「电费合计」，电费合计不含水费）
  assert.equal(summary.total[2], GOLDEN.expected.summary.grandTotalFee.toFixed(2))
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
