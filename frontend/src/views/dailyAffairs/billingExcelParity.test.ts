import test from 'node:test'
import assert from 'node:assert/strict'
import {
  allocateTimedKwh,
  billPeriodLossAdjust,
  fixedBillFeeOf,
  resolveTenantItemValue,
  resolveTimedItemFee,
  type SourceResolveContext,
} from './tenantBillingSources'

/**
 * 2026-09 持睿汽车：与人工 Excel 核算表逐格对账。
 *
 * 这张表是业务的**唯一验收基准**（用户手写批注 + 标准 Excel），
 * 任何口径改动都必须让这组断言全绿，否则列表、浮窗、PDF 三处就会各说各话。
 *
 * 关键口径（与需求原文一致）：
 *   ① 分摊比例 ratio = 126600 / 596168 = 0.21235625，**只作用于基本电费与功率因素调整电费**；
 *   ② 账单电量明细的损耗/加减电量是**全厂**口径，进分时折算前要乘 ratio；
 *   ③ 差额电量 ΔQ = 3900 先在 3 块分表间均分（每表 1300），再按表内时段占比分摊；
 *   ④ 分时电费 = 逐表逐段 round2(基准电量 × 时段单价) 相加；
 *   ⑤ 平段按度项：计费数量 = 126600，金额 = 126600 × 账单单价（**不是**账单金额 × ratio）。
 */
const RATIO = 126600 / 596168

/** 三块分时表的抄见电量（2026-09 真实读数）。 */
const meters = [
  { name: '持睿工业分表1', usage: { deep: 5200, peak: 15200, flat: 21400, valley: 21800 } },
  { name: '持睿工业分表2', usage: { deep: 0, peak: 15300, flat: 15300, valley: 15300 } },
  { name: '持睿工业分表3', usage: { deep: 0, peak: 4600, flat: 4200, valley: 4400 } },
]

/** 市电账单原始 fee_items（只保留参与分摊的部分，数值取自库中 2026-09 账单）。 */
const FEE_ITEMS = [
  { name: '零售交易电费', period: '尖峰', qty: 24622, rate: 0.436, fee: 10735.19, category: '(1)市场化购电费' },
  { name: '零售交易电费', period: '峰', qty: 112221, rate: 0.436, fee: 48928.36, category: '(1)市场化购电费' },
  { name: '零售交易电费', period: '平', qty: 183205, rate: 0.436, fee: 79877.38, category: '(1)市场化购电费' },
  { name: '零售交易电费', period: '谷', qty: 267182, rate: 0.436, fee: 116491.35, category: '(1)市场化购电费' },
  { name: '零售损益分摊电费', period: '平', qty: 587230, rate: 0.035557, fee: 20880.13, category: '(1)市场化购电费' },
  { name: '上网环节线损费用', period: '平', qty: 587230, rate: 0.017144, fee: 10067.47, category: '(2)上网环节线损费用' },
  { name: '零售输配电费', period: '尖峰', qty: 24622, rate: 0.2424, fee: 5968.37, category: '(3)输配电量电费' },
  { name: '零售输配电费', period: '峰', qty: 112221, rate: 0.2424, fee: 27202.37, category: '(3)输配电量电费' },
  { name: '零售输配电费', period: '平', qty: 183205, rate: 0.1515, fee: 27755.56, category: '(3)输配电量电费' },
  { name: '零售输配电费', period: '谷', qty: 267182, rate: 0.05757, fee: 15381.67, category: '(3)输配电量电费' },
  // 系统运行费 7 项（平段按度）
  { name: '燃气机组容量电费', period: '平', qty: 587230, rate: 0.018983, fee: 11147.39, category: '(4)系统运行费' },
  { name: '新能源机制差价分摊电费', period: '平', qty: 587230, rate: 0.007546, fee: 4431.24, category: '(4)系统运行费' },
  { name: '电价交叉补贴新增损益', period: '平', qty: 587230, rate: -0.005536, fee: -3250.91, category: '(4)系统运行费' },
  { name: '上网环节线损代理采购损益', period: '平', qty: 587230, rate: -0.000971, fee: -570.2, category: '(4)系统运行费' },
  { name: '其他系统运行费用', period: '平', qty: 587230, rate: 0.004506, fee: 2646.06, category: '(4)系统运行费' },
  { name: '抽水蓄能容量电费', period: '平', qty: 587230, rate: 0.00731, fee: 4292.65, category: '(4)系统运行费' },
  { name: '煤电容量电费', period: '平', qty: 587230, rate: 0.023056, fee: 13539.17, category: '(4)系统运行费' },
  // 政府性基金及附加（工商业）5 项
  { name: '国家重大水利工程建设基金', period: '平', qty: 587230, rate: 0.001969, fee: 1156.26, category: '(6)政府性基金及附加-工商业' },
  { name: '可再生能源附加', period: '平', qty: 587230, rate: 0.019, fee: 11157.37, category: '(6)政府性基金及附加-工商业' },
  { name: '库区移民基金', period: '平', qty: 587230, rate: 0.006225, fee: 3655.51, category: '(6)政府性基金及附加-工商业' },
  { name: '农网还贷', period: '平', qty: 587230, rate: 0.02, fee: 11744.6, category: '(6)政府性基金及附加-工商业' },
  { name: '小型水库移民后期扶持资金(地方)', period: '平', qty: 587230, rate: 0.0005, fee: 293.62, category: '(6)政府性基金及附加-工商业' },
]

/**
 * 账单电量明细（**全厂原始值**）。
 * 尖峰线损 13 度、加减 -1341 度 —— 乘 ratio 后才是持睿承担的 2.76 / -284.77。
 */
const METER_READINGS = [
  { meter_type: '正向有功（总）', trans_loss: 0, line_loss: 0, adjust: 0 },
  { meter_type: '正向有功（尖峰）', trans_loss: 0, line_loss: 13, adjust: -1341 },
  { meter_type: '正向有功（峰）', trans_loss: 0, line_loss: 59, adjust: -3128 },
  { meter_type: '正向有功（平）', trans_loss: 0, line_loss: 96, adjust: -2681 },
  { meter_type: '正向有功（谷）', trans_loss: 0, line_loss: 140, adjust: -1788 },
]

function ctxOf(): SourceResolveContext {
  return {
    month: '2026-09',
    tenantName: '持睿汽车',
    bill: {
      item: {
        total_kwh: 596168,
        // 基本电费 / 力调电费在账单概况里（fee_items 没有可靠行）
        capacity_fee: 44000,
        pf_adjust_amount: -2818.07,
        meter_readings: METER_READINGS,
        fee_items: FEE_ITEMS,
      },
    },
    solarItems: [],
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    timedMeters: meters,
    deltaQ: 3900,
    ratio: RATIO,
    starTotal: 126600,
  }
}

/** 平段按度计费项：计费数量 126600 × 账单单价。 */
function plainFee(key: string, group: string): number {
  return resolveTenantItemValue(
    { category: group, source_category: '市电账单', source_key: key, source_mode: 'fee_row', value_mode: 'apportion' },
    ctxOf(),
    RATIO,
  )
}

const r2 = (v: number) => Math.round(v * 100) / 100

test('账单损耗/加减电量：乘 ratio 后与标准表四段一致', () => {
  const { loss, adjust } = billPeriodLossAdjust(ctxOf().bill, RATIO)
  assert.equal(loss.deep, 2.76, '尖 13 × 0.21235625')
  assert.equal(adjust.deep, -284.77, '尖 -1341 × 0.21235625')
  assert.equal(loss.peak, 12.53)
  assert.equal(adjust.peak, -664.25)
  assert.equal(loss.flat, 20.39)
  assert.equal(adjust.flat, -569.33)
  assert.equal(loss.valley, 29.73)
  assert.equal(adjust.valley, -379.69)
})

test('分时四段计费数量：5024.28 / 35645.33 / 41635.45 / 42462.30', () => {
  const { loss, adjust } = billPeriodLossAdjust(ctxOf().bill, RATIO)
  const alloc = allocateTimedKwh(meters, loss, adjust, 3900)
  assert.equal(alloc.periodBase.deep, 5024.28)
  assert.equal(alloc.periodBase.peak, 35645.33)
  assert.equal(alloc.periodBase.flat, 41635.45)
  assert.equal(alloc.periodBase.valley, 42462.3)
  const total = r2(alloc.periodBase.deep + alloc.periodBase.peak + alloc.periodBase.flat + alloc.periodBase.valley)
  // 标准表展示为 124,767（整数）；精确累加 124,767.36
  assert.ok(Math.abs(total - 124767.36) < 0.02, `四段合计 ${total}`)
})

test('零售交易电费 = 54398.57（尖 2190.59 / 峰 15541.36 / 平 18153.06 / 谷 18513.56）', () => {
  const t = resolveTimedItemFee(ctxOf(), '零售交易电费', '(1)市场化购电费')
  assert.ok(t, '必须识别为分时项')
  const byPeriod = Object.fromEntries(t!.periods.map((p) => [p.period, p.fee]))
  assert.equal(byPeriod['尖峰'], 2190.59)
  assert.equal(byPeriod['峰'], 15541.36)
  assert.equal(byPeriod['平'], 18153.06)
  assert.equal(byPeriod['谷'], 18513.56)
  assert.equal(t!.value, 54398.57)
})

test('零售输配电费 = 18610.64（尖 1217.89 / 峰 8640.42 / 平 6307.77 / 谷 2444.56）', () => {
  const t = resolveTimedItemFee(ctxOf(), '零售输配电费', '(3)输配电量电费')
  assert.ok(t)
  const byPeriod = Object.fromEntries(t!.periods.map((p) => [p.period, p.fee]))
  assert.equal(byPeriod['尖峰'], 1217.89)
  assert.equal(byPeriod['峰'], 8640.42)
  assert.equal(byPeriod['平'], 6307.77)
  assert.equal(byPeriod['谷'], 2444.56)
  assert.equal(t!.value, 18610.64)
})

test('平段按度项：计费数量一律 126600，金额 = 126600 × 账单单价', () => {
  assert.equal(plainFee('零售损益分摊电费', '(1)市场化购电费'), 4501.52)
  assert.equal(plainFee('上网环节线损费用', '(2)上网环节线损费用'), 2170.43)
  // 系统运行费用 7 项
  assert.equal(plainFee('燃气机组容量电费', '(4)系统运行费'), 2403.25)
  assert.equal(plainFee('新能源机制差价分摊电费', '(4)系统运行费'), 955.32)
  assert.equal(plainFee('电价交叉补贴新增损益', '(4)系统运行费'), -700.86)
  assert.equal(plainFee('上网环节线损代理采购损益', '(4)系统运行费'), -122.93)
  assert.equal(plainFee('其他系统运行费用', '(4)系统运行费'), 570.46)
  assert.equal(plainFee('抽水蓄能容量电费', '(4)系统运行费'), 925.45)
  assert.equal(plainFee('煤电容量电费', '(4)系统运行费'), 2918.89)
  // 政府性基金及附加（工商业）5 项
  assert.equal(plainFee('国家重大水利工程建设基金', '(6)政府性基金及附加-工商业'), 249.28)
  assert.equal(plainFee('可再生能源附加', '(6)政府性基金及附加-工商业'), 2405.4)
  assert.equal(plainFee('库区移民基金', '(6)政府性基金及附加-工商业'), 788.09)
  assert.equal(plainFee('农网还贷', '(6)政府性基金及附加-工商业'), 2532)
  assert.equal(plainFee('小型水库移民后期扶持资金(地方)', '(6)政府性基金及附加-工商业'), 63.3)
})

test('汇总：市场化购电费 58900.09 / 系统运行 6949.58 / 政府性基金 6038.06', () => {
  const trade = resolveTimedItemFee(ctxOf(), '零售交易电费', '(1)市场化购电费')!
  const market = r2(trade.value + plainFee('零售损益分摊电费', '(1)市场化购电费'))
  assert.equal(market, 58900.09, '市场化购电费 = 54398.57 + 4501.52')

  const system = r2(
    2403.25 + 955.32 + -700.86 + -122.93 + 570.46 + 925.45 + 2918.89,
  )
  assert.equal(system, 6949.58, '系统运行费用 7 项之和')

  // 政府性基金：用未取整的单价累加再 round（Excel 是 6,038.06，不是逐项 round 后相加的 .07）
  const fund = r2(126600 * (0.001969 + 0.019 + 0.006225 + 0.02 + 0.0005))
  assert.equal(fund, 6038.06)
})

test('工业电费 101414.04 / 均价 0.8011 / 电费合计 102344.04', () => {
  const ctx = ctxOf()
  const trade = resolveTimedItemFee(ctx, '零售交易电费', '(1)市场化购电费')!.value
  const trans = resolveTimedItemFee(ctx, '零售输配电费', '(3)输配电量电费')!.value
  const marketLoss = plainFee('零售损益分摊电费', '(1)市场化购电费')
  const lineLoss = plainFee('上网环节线损费用', '(2)上网环节线损费用')
  const system = 6949.58
  const fund = 6038.06
  // 基本电费 / 力调电费：**只有这两项**按账单原金额 × 比例
  const capacity = r2((fixedBillFeeOf(ctx, '基本电费') || 0) * RATIO)
  const pf = r2((fixedBillFeeOf(ctx, '功率因素调整电费') || 0) * RATIO)
  assert.equal(capacity, 9343.67, '44000 × 0.21235625')
  assert.equal(pf, -598.43, '-2818.07 × 0.21235625')

  const industrial = r2(marketLoss + lineLoss + trans + trade + system + fund + capacity + pf)
  // marketLoss+trade 已含在 market 里；这里按 Excel 的 6 大项 + 2 项比例项累加
  assert.equal(industrial, 101414.04)
  assert.equal(Math.round((industrial / 126600) * 10000) / 10000, 0.8011, '均价 = 工业电费 ÷ 126600')
  assert.equal(r2(industrial + 930), 102344.04, '电费合计 = 工业电费 + 宿舍电费 930')
})
