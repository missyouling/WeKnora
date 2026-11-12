import test from 'node:test'
import assert from 'node:assert/strict'
import {
  dynamicColTypeOf, rowValueOf, waterPeriodTotals,
  type WaterAgg,
} from './energyDynamicColumns'

test('第51轮：动态列按 label 判合计口径，副本 key 不干扰', () => {
  assert.equal(dynamicColTypeOf('本期水量'), 'number')
  assert.equal(dynamicColTypeOf('本期水费'), 'amount')
  assert.equal(dynamicColTypeOf('结算金额'), 'amount')
  assert.equal(dynamicColTypeOf('均价（工业）'), 'text')
  assert.equal(dynamicColTypeOf('分摊比例'), 'text')
  assert.equal(dynamicColTypeOf('备注'), 'text')
  // 关键回归：key 叫「总电费_副本」但 label 是「本期水量」，仍应判 number
  assert.equal(dynamicColTypeOf('本期水量'), 'number')
})

test('第51轮：rowValueOf 多级兜底（英文 key / 中文 label / row.data）', () => {
  const byKey = rowValueOf('current_water_usage', { current_water_usage: 769 })
  assert.equal(byKey, 769)

  // 复制字段：name 是副本名，值存在中文 label 键里
  const byLabel = rowValueOf('总电费_副本', { '本期水量': 769 }, '本期水量')
  assert.equal(byLabel, 769)

  // 值在 row.data 里
  const byData = rowValueOf('total_water_fee', { data: { total_water_fee: 4014.18 } })
  assert.equal(byData, 4014.18)
  const byDataLabel = rowValueOf('总用水量_副本', { data: { '本期水费': 4014.18 } }, '本期水费')
  assert.equal(byDataLabel, 4014.18)

  // 0 是有效值，不能被吞掉
  assert.equal(rowValueOf('fire', { fire: 0 }), 0)
  // 空值返回 undefined
  assert.equal(rowValueOf('x', { x: '' }), undefined)
  assert.equal(rowValueOf('x', null), undefined)
})

test('第51轮：waterPeriodTotals 2026-10 精确对齐 769 / 4014.18', () => {
  const main: WaterAgg = { usage: 769, fee: 4014.18, price: 5.22 }
  const fire: WaterAgg = { usage: 0, fee: 0, price: 5.22 }
  assert.deepEqual(waterPeriodTotals(main, fire), { usage: 769, fee: 4014.18 })
})

test('第51轮：台账未记金额时按用量 × 单价回算', () => {
  const main: WaterAgg = { usage: 769, fee: 0, price: 5.22 }
  const fire: WaterAgg = { usage: 0, fee: 0, price: 5.22 }
  assert.deepEqual(waterPeriodTotals(main, fire), { usage: 769, fee: 4014.18 })
})

test('第51轮：两表都有量与金额时累加并 round2', () => {
  const main: WaterAgg = { usage: 100.005, fee: 522.026, price: 5.22 }
  const fire: WaterAgg = { usage: 10, fee: 52.2, price: 5.22 }
  const out = waterPeriodTotals(main, fire)
  assert.equal(out.usage, 110.01)
  assert.equal(out.fee, 574.23)
})

test('第51轮：全 0 不产生 NaN', () => {
  const zero: WaterAgg = { usage: 0, fee: 0, price: 0 }
  assert.deepEqual(waterPeriodTotals(zero, zero), { usage: 0, fee: 0 })
  assert.deepEqual(
    waterPeriodTotals(undefined as any, null as any),
    { usage: 0, fee: 0 },
  )
})
