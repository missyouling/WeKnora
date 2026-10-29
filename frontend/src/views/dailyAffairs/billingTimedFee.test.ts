import test from 'node:test'
import assert from 'node:assert/strict'
import { allocateTimedKwh, billPeriodLossAdjust, periodRatesOf, resolveTimedItemFee, type SourceResolveContext } from './tenantBillingSources'

/**
 * 前端分时电费口径必须与后端 billing_timed.go 逐分一致。
 *
 * 2026-09 持睿汽车：零售交易电费 54,422.79、零售输配电费 18,616.22。
 * 旧实现用「账单整月金额 × 21.24%」，算出 54,369.99 / 16,204.46。
 */
const meters = [
  { name: '持睿工业分表1', usage: { deep: 5200, peak: 15200, flat: 21400, valley: 21800 } },
  { name: '持睿工业分表2', usage: { deep: 0, peak: 15300, flat: 15300, valley: 15300 } },
  { name: '持睿工业分表3', usage: { deep: 0, peak: 4600, flat: 4200, valley: 4400 } },
]
const loss = { deep: 2.68, peak: 12.15, flat: 19.77, valley: 28.83 }
const adjust = { deep: -276.14, peak: -644.12, flat: -552.07, valley: -368.19 }
const RATES: Record<string, [number, number]> = {
  deep: [0.2424, 0.436], peak: [0.2424, 0.436], flat: [0.1515, 0.436], valley: [0.05757, 0.436],
}

test('前端分时分摊：抄见/计费/差额合计与标准核算表一致', () => {
  const alloc = allocateTimedKwh(meters, loss, adjust, 3900)
  assert.equal(alloc.totalUsage, 122700)
  assert.ok(Math.abs(alloc.totalBill - 120922.91) < 0.02, `计费电量合计 ${alloc.totalBill}`)
  assert.equal(alloc.deltaQ, 3900)
  // 逐表明细：损耗/加减只落在第一块表
  const m1 = alloc.rows.slice(0, 4)
  assert.equal(m1[0].billKwh, 4926.54)
  assert.equal(m1[0].diffKwh, 158.89)
  assert.equal(m1[0].baseKwh, 5085.43)
  assert.equal(m1[1].billKwh, 14568.03)
  assert.equal(m1[3].baseKwh, 22152.79)
  const m2 = alloc.rows.slice(4, 8)
  assert.ok(m2.every((r) => r.loss === 0 && r.adjust === 0))
  assert.ok(m2.every((r) => r.billKwh === r.usage))
})

test('前端分时电费：零售交易电费 54422.79 / 零售输配电费 18616.22', () => {
  const alloc = allocateTimedKwh(meters, loss, adjust, 3900)
  let trans = 0
  let purchase = 0
  for (const p of ['deep', 'peak', 'flat', 'valley']) {
    trans = Math.round((trans + Math.round(alloc.periodBaseExact[p] * RATES[p][0] * 100) / 100) * 100) / 100
    purchase = Math.round((purchase + Math.round(alloc.periodBaseExact[p] * RATES[p][1] * 100) / 100) * 100) / 100
  }
  assert.equal(trans, 18616.22)
  assert.equal(purchase, 54422.79)
})

test('billPeriodLossAdjust：把变损+线损合并、按四时段取值', () => {
  const bill = {
    item: {
      meter_readings: [
        { meter_type: '正向有功（尖峰）', trans_loss: 1.5, line_loss: 1.18, adjust: -276.14 },
        { meter_type: '正向有功（峰）', trans_loss: 0, line_loss: 12.15, adjust: -644.12 },
        { meter_type: '正向有功（总）', trans_loss: 9, line_loss: 9, adjust: 9 },
      ],
    },
  }
  const { loss: l, adjust: a } = billPeriodLossAdjust(bill)
  assert.equal(l.deep, 2.68)
  assert.equal(l.peak, 12.15)
  assert.equal(a.deep, -276.14)
  assert.equal(l.flat, undefined, '总表/无功行不得参与分时段折算')
})

function ctxOf(billFees: any[]): SourceResolveContext {
  return {
    month: '2026-09',
    tenantName: '持睿汽车',
    bill: {
      item: {
        meter_readings: [
          { meter_type: '正向有功（尖峰）', trans_loss: 0, line_loss: 2.68, adjust: -276.14 },
          { meter_type: '正向有功（峰）', trans_loss: 0, line_loss: 12.15, adjust: -644.12 },
          { meter_type: '正向有功（平）', trans_loss: 0, line_loss: 19.77, adjust: -552.07 },
          { meter_type: '正向有功（谷）', trans_loss: 0, line_loss: 28.83, adjust: -368.19 },
        ],
        fee_items: billFees,
      },
    },
    solarItems: [],
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    timedMeters: meters,
    deltaQ: 3900,
  }
}

test('resolveTimedItemFee：分时项按基准电量×时段单价，非分时项返回 null', () => {
  const fees = [
    { name: '零售交易电费', period: '尖峰', qty: 1, rate: 0.436, fee: 1 },
    { name: '零售交易电费', period: '峰', qty: 1, rate: 0.436, fee: 1 },
    { name: '零售交易电费', period: '平', qty: 1, rate: 0.436, fee: 1 },
    { name: '零售交易电费', period: '谷', qty: 1, rate: 0.436, fee: 1 },
    { name: '零售输配电费', period: '尖峰', qty: 1, rate: 0.2424, fee: 1 },
    { name: '零售输配电费', period: '峰', qty: 1, rate: 0.2424, fee: 1 },
    { name: '零售输配电费', period: '平', qty: 1, rate: 0.1515, fee: 1 },
    { name: '零售输配电费', period: '谷', qty: 1, rate: 0.05757, fee: 1 },
    { name: '基本电费', period: '', qty: 1, rate: 0, fee: 44000 },
  ]
  const ctx = ctxOf(fees)
  const trade = resolveTimedItemFee(ctx, '零售交易电费')
  assert.ok(trade)
  assert.equal(trade!.value, 54422.79)
  assert.equal(trade!.periods.length, 4)
  assert.equal(trade!.periods[0].period, '尖峰')
  assert.equal(trade!.periods[0].qty, 5085.43)

  const trans = resolveTimedItemFee(ctx, '零售输配电费')
  assert.equal(trans!.value, 18616.22)

  assert.equal(resolveTimedItemFee(ctx, '基本电费'), null, '非分时项必须回落原口径')
  assert.equal(periodRatesOf(ctx, '基本电费').deep, undefined)
})

test('resolveTimedItemFee：没有分时表读数时返回 null（不静默出 0）', () => {
  const ctx = ctxOf([{ name: '零售交易电费', period: '尖峰', qty: 1, rate: 0.436, fee: 1 }])
  ctx.timedMeters = []
  assert.equal(resolveTimedItemFee(ctx, '零售交易电费'), null)
})
