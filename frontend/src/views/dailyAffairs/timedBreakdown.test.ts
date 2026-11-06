import test from 'node:test'
import assert from 'node:assert/strict'
import {
  allocateTimedKwh,
  timedBreakdownOf,
  timedBreakdownTotalsOf,
  TIMED_PERIOD_LABELS,
} from './tenantBillingSources'

// ---------------------------------------------------------------------------
// 第 40 轮：分时四段「基准电量拆解」过程量（与 Go 侧同源）
// ---------------------------------------------------------------------------
//
// 真机图：展开「市场化购电费」只见最终基准电量（7587.43 / 48049.81 /
// 57806.98 / 56443.53），看不到它是怎么由电表电量、差额分摊、线损、加减
// 一步步加出来的。恒等式：
//
//   billing_qty = meter_qty + diff_qty + line_loss_qty + adjust_qty
//
// 2026-10 持睿汽车（库内 billing_record_meters 按 period 聚合）：
//   尖峰 7800 + 170.13 + 4.97 − 387.67 = 7587.43
//   峰   47200 + 1735.12 + 19.34 − 904.65 = 48049.81
//   平   56600 + 1970.43 + 28.46 − 791.91 = 57806.98
//   谷   54950 + 1974.31 + 36.20 − 516.98 = 56443.53
//   合计 166550 + 5850 + 88.97 − 2601.21 = 169887.75

// 库内 billing_record_meters 的逐表逐段抄见电量（2026-10 真实值）：
//   分表1 7800/21800/30800/29000，分表2 0/19200/19200/18750，
//   分表3 0/6200/6600/7200。用真实分表占比，差额分摊才会得出 170.13 等精确值。
const meters202610 = [
  { name: '持睿工业分表1', usage: { deep: 7800, peak: 21800, flat: 30800, valley: 29000 } },
  { name: '持睿工业分表2', usage: { deep: 0, peak: 19200, flat: 19200, valley: 18750 } },
  { name: '持睿工业分表3', usage: { deep: 0, peak: 6200, flat: 6600, valley: 7200 } },
]
const loss202610 = { deep: 4.97, peak: 19.34, flat: 28.46, valley: 36.20 }
const adjust202610 = { deep: -387.67, peak: -904.65, flat: -791.91, valley: -516.98 }

test('timedBreakdownOf：四段拆解与基准电量严格闭合（2026-10）', () => {
  const alloc = allocateTimedKwh(meters202610, loss202610, adjust202610, 5850)
  const want: Record<string, [number, number, number]> = {
    尖峰: [7800, 7587.43, 4.97],
    峰: [47200, 48049.81, 19.34],
    平: [56600, 57806.98, 28.46],
    谷: [54950, 56443.53, 36.2],
  }
  for (const [label, [meter, billing, loss]] of Object.entries(want)) {
    const b = timedBreakdownOf(alloc, label)
    assert.equal(b.meterQty, meter, `${label} 电表电量`)
    assert.equal(b.billingQty, billing, `${label} 基准电量`)
    assert.equal(b.lineLossQty, loss, `${label} 线损`)
    // 恒等式：四分量之和 = 账单原值/计费数量
    const sum = Math.round((b.meterQty + b.diffQty + b.lineLossQty + b.adjustQty) * 100) / 100
    assert.equal(sum, b.billingQty, `${label} 拆解恒等式不成立`)
  }
})

test('timedBreakdownTotalsOf：合计电表 166550 / 差额 5850 / 基准 169887.75', () => {
  const alloc = allocateTimedKwh(meters202610, loss202610, adjust202610, 5850)
  const t = timedBreakdownTotalsOf(alloc)
  assert.equal(t.meterQty, 166550)
  assert.ok(Math.abs(t.diffQty - 5850) <= 0.02, `差额电量合计应≈5850，got ${t.diffQty}`)
  assert.equal(t.billingQty, 169887.75)
  assert.equal(Math.round((t.meterQty + t.diffQty + t.lineLossQty + t.adjustQty) * 100) / 100,
    t.billingQty, '合计恒等式不成立')
})

test('timedBreakdownOf：未知时段返回全 0，不抛错', () => {
  const b = timedBreakdownOf(allocateTimedKwh(meters202610, loss202610, adjust202610, 5850), '未知')
  assert.deepEqual(b, { meterQty: 0, diffQty: 0, lineLossQty: 0, adjustQty: 0, billingQty: 0 })
})

test('时段标签映射：deep/peak/flat/valley → 尖峰/峰/平/谷', () => {
  assert.equal(TIMED_PERIOD_LABELS.deep, '尖峰')
  assert.equal(TIMED_PERIOD_LABELS.peak, '峰')
  assert.equal(TIMED_PERIOD_LABELS.flat, '平')
  assert.equal(TIMED_PERIOD_LABELS.valley, '谷')
})
