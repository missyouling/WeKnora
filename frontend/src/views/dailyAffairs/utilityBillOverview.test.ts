// 账单概况行构造与合计的单测（第 24 轮任务一瘦身）。
//
// 这张表是财务对数的骨架，口径错一处就是「明细对了、小计错了」，
// 而且用户很难发现。因此这里把每个组成项都显式锁住。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import {
  FEE_TOLERANCE,
  billTotalCheck,
  buildOverviewRows,
  feeCloseOf,
  feeDiffOf,
  overviewTotalOf,
  qtyLabelOf,
} from './utilityBillOverview'

/** 造一份「只有金额与电量」的费用行。 */
const row = (fee: number, qty = 0, fee_amount?: number) => ({ fee, qty, fee_amount })

/** 按菜单 key 分组的输入。 */
function makeInputs(byMenu: Record<string, any[]>, editForm: Record<string, any> = {}, overrides: Record<string, number> = {}) {
  return {
    feeItems: Object.values(byMenu).flat(),
    rowsOf: (k: string) => byMenu[k] || [],
    editForm,
    overrides,
  }
}

test('buildOverviewRows：11 行且顺序固定', () => {
  const rows = buildOverviewRows(makeInputs({}))
  assert.deepEqual(rows.map((r) => r.key), [
    'market', 'line', 'trans', 'sys', 'govI', 'industrial',
    'catalog', 'govR', 'residential', 'capacity', 'pf',
  ])
  // 两个小计行必须带 total 标记（模板靠它加粗、禁编辑）
  assert.deepEqual(rows.filter((r) => r.total).map((r) => r.key), ['industrial', 'residential'])
})

test('buildOverviewRows：金额按分累加，不留浮点尾巴', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(0.1), row(0.2)] }))
  assert.equal(rows[0].displayValue, 0.3, '0.1 + 0.2 必须等于 0.3')
})

test('工商业小计 = 市场化 + 线损 + 输配量 + 系统 + 政府基金 + 容需量', () => {
  const rows = buildOverviewRows(makeInputs(
    {
      'industrial-market': [row(100)],
      'industrial-line': [row(20)],
      'industrial-trans': [row(30)],
      'industrial-sys': [row(40)],
      'industrial-gov': [row(50)],
    },
    { capacity_fee: 60 },
  ))
  const industrial = rows.find((r) => r.key === 'industrial')!
  assert.equal(industrial.displayValue, 300, '100+20+30+40+50+60 = 300')
})

test('居民小计 = 目录电费 + 政府性基金（居民）', () => {
  const rows = buildOverviewRows(makeInputs({
    'residential-catalog': [row(70)],
    'residential-gov': [row(10)],
  }))
  const residential = rows.find((r) => r.key === 'residential')!
  assert.equal(residential.displayValue, 80)
})

test('功率因数调整电费不进任何小计', () => {
  const rows = buildOverviewRows(makeInputs({}, { pf_adjust_amount: 123 }))
  assert.equal(rows.find((r) => r.key === 'pf')!.displayValue, 123)
  assert.equal(rows.find((r) => r.key === 'industrial')!.displayValue, 0, '力调电费不进工商业小计')
  assert.equal(rows.find((r) => r.key === 'residential')!.displayValue, 0, '力调电费不进居民小计')
})

test('概览合计 = 工商业小计 + 居民小计 + 功率因数调整电费', () => {
  const rows = buildOverviewRows(makeInputs(
    { 'industrial-market': [row(100)], 'residential-catalog': [row(50)] },
    { pf_adjust_amount: 30 },
  ))
  assert.equal(overviewTotalOf(rows), 180, '100 + 50 + 30 = 180')
})

test('概览合计不得把明细行重复计一遍', () => {
  // 若用「所有非小计行求和」，明细 100 会被算两次（一次自身、一次小计）
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100)] }))
  assert.equal(overviewTotalOf(rows), 100, '明细与小计只能计一次')
})

test('手动覆盖：子项覆盖后小计随之联动', () => {
  const rows = buildOverviewRows(makeInputs(
    { 'industrial-market': [row(100)], 'industrial-line': [row(20)] },
    {},
    { market: 150 }, // 把市场化从 100 手工改成 150
  ))
  assert.equal(rows[0].displayValue, 150, '子项展示覆盖值')
  assert.equal(rows[0].value, 100, '原始计算值保留（用于对比）')
  assert.equal(rows.find((r) => r.key === 'industrial')!.displayValue, 170, '150 + 20 = 170，小计必须跟着变')
})

test('覆盖成 0 是合法操作（不能用 || 判空）', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100)] }, {}, { market: 0 }))
  assert.equal(rows[0].displayValue, 0, '覆盖成 0 必须生效（把算错的一项手工清零）')
  assert.equal(rows.find((r) => r.key === 'industrial')!.displayValue, 0)
})

test('账单值也可覆盖（bill: 前缀），与计算值独立', () => {
  const rows = buildOverviewRows(makeInputs(
    { 'industrial-market': [row(100, 0, 100)] },
    {},
    { 'bill:market': 105 },
  ))
  assert.equal(rows[0].billFee, 105, '账单列用覆盖值')
  assert.equal(rows[0].displayValue, 100, '计算列不受影响')
  assert.equal(rows[0].ovBillKey, 'bill:market')
})

test('类别电量取最大值而不是求和（分时子项相加会翻倍）', () => {
  const rows = buildOverviewRows(makeInputs({
    'industrial-market': [row(10, 100), row(20, 300), row(30, 200)],
  }))
  assert.equal(rows[0].qty, 300, '尖/峰/平/谷各段电量取最大 = 整表电量')
})

test('类别电量保留 1 位小数', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(10, 123.456)] }))
  assert.equal(rows[0].qty, 123.5)
})

test('无费用行时电量为 0，不产生 -Infinity', () => {
  const rows = buildOverviewRows(makeInputs({}))
  assert.equal(rows[0].qty, 0, '空数组取 Max 会得到 -Infinity，必须提前返回 0')
})

test('qtyLabelOf：-1 给 —（该行不适用电量）', () => {
  const rows = buildOverviewRows(makeInputs({}))
  const pf = rows.find((r) => r.key === 'pf')!
  assert.equal(pf.qty, -1)
  assert.equal(qtyLabelOf(pf, (v) => `${v}`), '—', '功率因数行没有电量，必须显示 —')
  assert.equal(qtyLabelOf(rows[0], (v) => `${v}`), '0')
})

test('feeDiffOf / feeCloseOf：±0.02 元内判为对上', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100, 0, 100.02)] }))
  assert.equal(feeDiffOf(rows[0]), -0.02)
  assert.equal(feeCloseOf(rows[0]), true, '0.02 是四舍五入误差，算对上')
})

test('feeCloseOf：差 0.03 判为不符', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100, 0, 100.03)] }))
  assert.equal(feeCloseOf(rows[0]), false)
  assert.equal(FEE_TOLERANCE, 0.02)
})

test('billTotalCheck：计算合计与账单合计对比', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100)] }))
  const r = billTotalCheck(rows, 100)
  assert.deepEqual(r, { calc: 100, bill: 100, diff: 0, ok: true })
})

test('billTotalCheck：账单合计为 0 时也要给出明确结论', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [row(100)] }))
  const r = billTotalCheck(rows, 0)
  assert.equal(r.bill, 0)
  assert.equal(r.diff, 100)
  assert.equal(r.ok, false, '账单合计缺失时不能默认判「对上」')
})
