import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
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
 * **基准数据不在本文件里**，而是与后端 `internal/handler/billing_timed_test.go`
 * 共读 `testdata/billing-golden-2026-09.json`（第 22 轮 P0-4）。
 *
 * 为什么必须共享：这套口径是业务的唯一验收基准，前后端各自维护一份 fixture 时，
 * 改了一边忘了另一边，单测全绿但线上对不上账（第 20 轮就发生过：前端按新口径
 * 算出 54,398.57，Go 侧断言仍写着 54,422.79）。共享文件后，任何口径改动都必须
 * 让两边同时变绿，改不动就是口径没对齐。
 */
const GOLDEN_PATH = fileURLToPath(
  new URL('../../../../testdata/billing-golden-2026-09.json', import.meta.url),
)
const GOLDEN = JSON.parse(readFileSync(GOLDEN_PATH, 'utf8')) as {
  month: string
  tenantName: string
  inputs: {
    billTotalKwh: number
    starTotalKwh: number
    deltaQ: number
    capacityFee: number
    pfAdjustAmount: number
    dormKwh: number
    dormFee: number
    waterFee: number
    timedMeters: { name: string; usage: Record<string, number> }[]
    meterReadings: { meter_type: string; trans_loss: number; line_loss: number; adjust: number }[]
    feeItems: {
      name: string
      period: string
      qty: number
      rate: number
      fee: number
      category: string
    }[]
    periodRates: Record<string, { trade: number; transmission: number }>
  }
  expected: {
    ratio: number
    lossByPeriod: Record<string, number>
    adjustByPeriod: Record<string, number>
    baseKwh: Record<string, number>
    baseKwhTotal: number
    trade: { periods: Record<string, number>; total: number }
    transmission: { periods: Record<string, number>; total: number }
    plainFees: { name: string; group: string; qty: number; rate: number; fee: number }[]
    summary: Record<string, number>
  }
}

const { inputs, expected } = GOLDEN
/** 分摊比例：以账单电量与工业电量现算（8 位），与后端 round8 同口径。 */
const RATIO = Math.round((inputs.starTotalKwh / inputs.billTotalKwh) * 1e8) / 1e8
const PERIODS = ['deep', 'peak', 'flat', 'valley'] as const
const PERIOD_LABELS: Record<string, string> = { deep: '尖峰', peak: '峰', flat: '平', valley: '谷' }

function ctxOf(): SourceResolveContext {
  return {
    month: GOLDEN.month,
    tenantName: GOLDEN.tenantName,
    bill: {
      item: {
        total_kwh: inputs.billTotalKwh,
        // 基本电费 / 力调电费在账单概况里（fee_items 没有可靠行）
        capacity_fee: inputs.capacityFee,
        pf_adjust_amount: inputs.pfAdjustAmount,
        meter_readings: inputs.meterReadings,
        fee_items: inputs.feeItems,
      },
    },
    solarItems: [],
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    timedMeters: inputs.timedMeters,
    deltaQ: inputs.deltaQ,
    ratio: RATIO,
    starTotal: inputs.starTotalKwh,
  }
}

/** 平段按度计费项：计费数量 126600 × 账单单价。 */
function plainFee(key: string, group: string): number {
  return resolveTenantItemValue(
    {
      category: group,
      source_category: '市电账单',
      source_key: key,
      source_mode: 'fee_row',
      value_mode: 'apportion',
    },
    ctxOf(),
    RATIO,
  )
}

const r2 = (v: number) => Math.round(v * 100) / 100

test('黄金基准文件：ratio 与分时单价齐备（fixture 自检）', () => {
  assert.equal(RATIO, expected.ratio, 'ratio = 126600 / 596168')
  for (const p of PERIODS) {
    assert.ok(inputs.periodRates[p], `缺少 ${p} 时段单价`)
  }
  assert.equal(inputs.timedMeters.length, 3, '必须是 3 块分时表')
})

test('账单损耗/加减电量：乘 ratio 后与标准表四段一致', () => {
  const { loss, adjust } = billPeriodLossAdjust(ctxOf().bill, RATIO)
  for (const p of PERIODS) {
    assert.equal(loss[p], expected.lossByPeriod[p], `${PERIOD_LABELS[p]} 损耗`)
    assert.equal(adjust[p], expected.adjustByPeriod[p], `${PERIOD_LABELS[p]} 加减电量`)
  }
})

test('分时四段计费数量：5024.28 / 35645.33 / 41635.45 / 42462.30', () => {
  const { loss, adjust } = billPeriodLossAdjust(ctxOf().bill, RATIO)
  const alloc = allocateTimedKwh(inputs.timedMeters, loss, adjust, inputs.deltaQ)
  for (const p of PERIODS) {
    assert.equal(alloc.periodBase[p], expected.baseKwh[p], `${PERIOD_LABELS[p]} 基准电量`)
  }
  const total = r2(PERIODS.reduce((s, p) => s + alloc.periodBase[p], 0))
  assert.ok(
    Math.abs(total - expected.baseKwhTotal) < 0.02,
    `四段合计 ${total} 应≈${expected.baseKwhTotal}`,
  )
})

test('零售交易电费 / 零售输配电费：逐段与合计均与标准表一致', () => {
  const trade = resolveTimedItemFee(ctxOf(), '零售交易电费', '(1)市场化购电费')
  assert.ok(trade, '必须识别为分时项')
  const tradeBy = Object.fromEntries(trade!.periods.map((p) => [p.period, p.fee]))
  for (const [label, want] of Object.entries(expected.trade.periods)) {
    assert.equal(tradeBy[label], want, `零售交易电费 ${label}`)
  }
  assert.equal(trade!.value, expected.trade.total)

  const trans = resolveTimedItemFee(ctxOf(), '零售输配电费', '(3)输配电量电费')
  assert.ok(trans)
  const transBy = Object.fromEntries(trans!.periods.map((p) => [p.period, p.fee]))
  for (const [label, want] of Object.entries(expected.transmission.periods)) {
    assert.equal(transBy[label], want, `零售输配电费 ${label}`)
  }
  assert.equal(trans!.value, expected.transmission.total)
})

test('平段按度项：计费数量一律 126600，金额 = 126600 × 账单单价（零售损益除外）', () => {
  for (const row of expected.plainFees) {
    assert.equal(plainFee(row.name, row.group), row.fee, `${row.name} = ${row.qty} × ${row.rate}`)
  }
})

test('纯金额返还类（rate=0 且 qty=0）不计入分摊', () => {
  // 账单里这三条是全厂冲抵项，没有计量口径折算到单租户头上；旧实现硬摊 -7,648.34
  for (const name of ['中长期偏差回收返还（用户）', '中长期超额申报返还', '零售交易发电侧收益返还电费']) {
    const listed = inputs.feeItems.some((f) => f.name.includes(name.slice(0, 6)))
    if (!listed) continue
    const v = plainFee(name, '(1)市场化购电费')
    assert.equal(v, 0, `${name} 必须为 0`)
  }
})

test('汇总：市场化购电费 / 系统运行 / 政府性基金 / 工业电费 / 均价 / 电费合计', () => {
  const s = expected.summary
  const trade = resolveTimedItemFee(ctxOf(), '零售交易电费', '(1)市场化购电费')!.value
  const market = r2(trade + plainFee('零售损益分摊电费', '(1)市场化购电费'))
  assert.equal(market, s.marketPurchaseFee, '市场化购电费 = 交易电费 + 零售损益分摊')

  const system = r2(
    expected.plainFees
      .filter((f) => f.group === '(4)系统运行费')
      .reduce((acc, f) => acc + f.fee, 0),
  )
  assert.equal(system, s.systemOperationFee, '系统运行费用 7 项之和')

  // 政府性基金：用未取整的单价累加再 round（Excel 是 6,038.06，
  // 不是逐项 round 后相加的 6,038.07）
  const fund = r2(
    inputs.starTotalKwh *
      expected.plainFees
        .filter((f) => f.group === '(6)政府性基金及附加-工商业')
        .reduce((acc, f) => acc + f.rate, 0),
  )
  assert.equal(fund, s.govFundFee, '政府性基金及附加')
})

test('工业电费 101346.55 / 均价 0.8005 / 电费合计 102276.55', () => {
  const s = expected.summary
  const ctx = ctxOf()
  const trade = resolveTimedItemFee(ctx, '零售交易电费', '(1)市场化购电费')!.value
  const trans = resolveTimedItemFee(ctx, '零售输配电费', '(3)输配电量电费')!.value
  const marketLoss = plainFee('零售损益分摊电费', '(1)市场化购电费')
  const lineLoss = plainFee('上网环节线损费用', '(2)上网环节线损费用')
  const system = s.systemOperationFee
  const fund = s.govFundFee

  // 基本电费 / 力调电费：**只有这两项**按「账单原金额 × 工业电量 ÷ 账单电量」折算
  const capacity = r2((fixedBillFeeOf(ctx, '基本电费') || 0) * RATIO)
  const pf = r2((fixedBillFeeOf(ctx, '功率因素调整电费') || 0) * RATIO)
  assert.equal(capacity, s.capacityFee, `${inputs.capacityFee} × ${RATIO}`)
  assert.equal(pf, s.pfAdjustFee, `${inputs.pfAdjustAmount} × ${RATIO}`)

  const industrial = r2(
    marketLoss + lineLoss + trans + trade + system + fund + capacity + pf,
  )
  assert.equal(industrial, s.industrialFee, '工业电费 = 6 大项 + 2 项比例项')
  assert.equal(
    Math.round((industrial / inputs.starTotalKwh) * 10000) / 10000,
    s.avgPrice,
    '均价 = 工业电费 ÷ 126600',
  )
  assert.equal(r2(industrial + inputs.dormFee), s.elecTotalFee, '电费合计 = 工业电费 + 宿舍电费')
  assert.equal(r2(s.elecTotalFee + inputs.waterFee), s.grandTotalFee, '本期应付 = 电费合计 + 水费合计')
})
