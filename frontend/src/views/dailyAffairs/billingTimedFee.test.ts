import test from 'node:test'
import assert from 'node:assert/strict'
import { allocateTimedKwh, billPeriodLossAdjust, periodRatesOf, resolveTimedItemFee, type SourceResolveContext } from './tenantBillingSources'

/**
 * 前端分时电费口径必须与后端 billing_timed.go、人工 Excel 核算表逐分一致。
 *
 * 2026-09 持睿汽车（标准核算表）：零售交易电费 54,398.57、零售输配电费 18,610.64。
 * 四段计费电量 5,024.28 / 35,645.33 / 41,635.45 / 42,462.30，合计 124,767.36。
 *
 * 三次口径演进（每次都是「对不上账」倒逼出来的）：
 *   ① 账单整月金额 × 21.24%   → 54,369.99（丢了分时结构）
 *   ② 差额按全部计费电量占比   → 54,422.79（表1 占比虚高，尖峰多 61 度）
 *   ③ 差额每表均分 + 表内占比  → 54,398.57（对齐 Excel）
 */
const meters = [
  { name: '持睿工业分表1', usage: { deep: 5200, peak: 15200, flat: 21400, valley: 21800 } },
  { name: '持睿工业分表2', usage: { deep: 0, peak: 15300, flat: 15300, valley: 15300 } },
  { name: '持睿工业分表3', usage: { deep: 0, peak: 4600, flat: 4200, valley: 4400 } },
]
// 账单电量明细是**全厂**口径，乘 ratio=126600/596168=0.21235625 后才是持睿的量。
// 13×0.21235625=2.76、-1341×0.21235625=-284.77（尖峰），其余三段同理。
const loss = { deep: 2.76, peak: 12.53, flat: 20.39, valley: 29.73 }
const adjust = { deep: -284.77, peak: -664.25, flat: -569.33, valley: -379.69 }
const RATES: Record<string, [number, number]> = {
  deep: [0.2424, 0.436], peak: [0.2424, 0.436], flat: [0.1515, 0.436], valley: [0.05757, 0.436],
}

test('前端分时分摊：抄见/计费/差额合计与标准核算表一致', () => {
  const alloc = allocateTimedKwh(meters, loss, adjust, 3900)
  assert.equal(alloc.totalUsage, 122700)
  assert.equal(alloc.deltaQ, 3900)
  // 逐表明细：损耗/加减只落在第一块表
  const m1 = alloc.rows.slice(0, 4)
  assert.equal(m1[0].billKwh, 4917.99, '尖峰计费电量 = 5200 + 2.76 - 284.77')
  assert.equal(m1[0].diffKwh, 106.29, '表1 尖峰差额 = 1300 × 5200/63600')
  assert.equal(m1[0].baseKwh, 5024.28)
  assert.equal(m1[1].billKwh, 14548.28)
  assert.equal(m1[1].diffKwh, 310.69)
  assert.equal(m1[2].baseKwh, 21288.48)
  assert.equal(m1[3].baseKwh, 21895.64)
  const m2 = alloc.rows.slice(4, 8)
  assert.ok(m2.every((r) => r.loss === 0 && r.adjust === 0))
  assert.ok(m2.every((r) => r.billKwh === r.usage))
  // 表2/表3：差额按表内占比均分（每表 1300 度）
  assert.equal(m2[0].diffKwh, 0, '表2 无尖峰用电 → 不分摊')
  assert.equal(m2[1].diffKwh, 433.33)
  const m3 = alloc.rows.slice(8, 12)
  assert.equal(m3[1].diffKwh, 453.03, '表3 峰 = 1300 × 4600/13200')
  assert.equal(m3[2].diffKwh, 413.64)
  // 四段基准电量合计 = 标准表 124,767
  const total = ['deep', 'peak', 'flat', 'valley'].reduce((s2, p) => s2 + (alloc.periodBase[p] || 0), 0)
  assert.ok(Math.abs(total - 124767.36) < 0.05, `四段合计 ${total}`)
})

test('前端分时电费：零售交易电费 54398.57 / 零售输配电费 18610.64', () => {
  // 逐表逐段 round2 后相加（Excel 是每格单独乘、合计再加）
  const perPeriod = (idx: number) => {
    const alloc = allocateTimedKwh(meters, loss, adjust, 3900)
    const out: Record<string, number> = {}
    for (const p of ['deep', 'peak', 'flat', 'valley']) {
      let v = 0
      for (let mi = 0; mi < alloc.rows.length; mi += 4) {
        const row = alloc.rows.slice(mi, mi + 4).find((r) => r.period === p)
        if (row) v = Math.round((v + Math.round(row.baseExact * RATES[p][idx] * 100) / 100) * 100) / 100
      }
      out[p] = v
    }
    return out
  }
  const trans = perPeriod(0)
  assert.equal(trans.deep, 1217.89)
  assert.equal(trans.peak, 8640.42)
  assert.equal(trans.flat, 6307.77)
  assert.equal(trans.valley, 2444.56)
  const sumTrans = Math.round((trans.deep + trans.peak + trans.flat + trans.valley) * 100) / 100
  assert.equal(sumTrans, 18610.64)

  const purchase = perPeriod(1)
  assert.equal(purchase.deep, 2190.59)
  assert.equal(purchase.peak, 15541.36)
  assert.equal(purchase.flat, 18153.06)
  assert.equal(purchase.valley, 18513.56)
  const sumPurchase = Math.round((purchase.deep + purchase.peak + purchase.flat + purchase.valley) * 100) / 100
  assert.equal(sumPurchase, 54398.57)
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
          // 账单原始值（全厂口径）：函数内部会乘 ctx.ratio 折成持睿的量
          { meter_type: '正向有功（尖峰）', trans_loss: 0, line_loss: 13, adjust: -1341 },
          { meter_type: '正向有功（峰）', trans_loss: 0, line_loss: 59, adjust: -3128 },
          { meter_type: '正向有功（平）', trans_loss: 0, line_loss: 96, adjust: -2681 },
          { meter_type: '正向有功（谷）', trans_loss: 0, line_loss: 140, adjust: -1788 },
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
    // 分时折算损耗要用它（账单明细是全厂口径）
    ratio: 126600 / 596168,
    starTotal: 126600,
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
  assert.equal(trade!.value, 54398.57)
  assert.equal(trade!.periods.length, 4)
  assert.equal(trade!.periods[0].period, '尖峰')
  assert.equal(trade!.periods[0].qty, 5024.28)

  const trans = resolveTimedItemFee(ctx, '零售输配电费')
  assert.equal(trans!.value, 18610.64)

  assert.equal(resolveTimedItemFee(ctx, '基本电费'), null, '非分时项必须回落原口径')
  assert.equal(periodRatesOf(ctx, '基本电费').deep, undefined)
})

test('resolveTimedItemFee：没有分时表读数时返回 null（不静默出 0）', () => {
  const ctx = ctxOf([{ name: '零售交易电费', period: '尖峰', qty: 1, rate: 0.436, fee: 1 }])
  ctx.timedMeters = []
  assert.equal(resolveTimedItemFee(ctx, '零售交易电费'), null)
})
