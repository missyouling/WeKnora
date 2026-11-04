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
function makeInputs(
  byMenu: Record<string, any[]>,
  editForm: Record<string, any> = {},
  overrides: Record<string, number> = {},
  details: { capacityDetail?: any; pfDetail?: any } = {},
) {
  return {
    feeItems: Object.values(byMenu).flat(),
    rowsOf: (k: string) => byMenu[k] || [],
    editForm,
    overrides,
    capacityDetail: details.capacityDetail ?? null,
    pfDetail: details.pfDetail ?? null,
  }
}

/** 分时费用行（列表时按尖/峰/平/谷各一段）。 */
const touRows = (qtys: number[], fee = 10) =>
  qtys.map((q, i) => ({ ...row(fee, q), period: ['尖峰', '峰', '平', '谷'][i] }))

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

test('工商业小计 = 5 项工商业细项（容需量单独成行，不进小计）', () => {
  // 第 33 轮口径：容需量与功率因素都在小计之外，由「本期电费」汇总时再加。
  // 若把它们塞进小计，页面上会出现「小计 480,085 + 容需量 44,000」的重复计。
  const rows = buildOverviewRows(makeInputs(
    {
      'industrial-market': [row(100)],
      'industrial-line': [row(20)],
      'industrial-trans': [row(30)],
      'industrial-sys': [row(40)],
      'industrial-gov': [row(50)],
    },
    { capacity_fee: 60 },
    {},
    { capacityDetail: { capacity: 2000, capacity_price: 22, capacity_fee: 60 } },
  ))
  const industrial = rows.find((r) => r.key === 'industrial')!
  assert.equal(industrial.displayValue, 240, '100+20+30+40+50 = 240（不含容需量 60）')
  assert.equal(rows.find((r) => r.key === 'capacity')!.displayValue, 60)
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
  const rows = buildOverviewRows(makeInputs({}, { pf_adjust_amount: 123 }, {}, { pfDetail: { adjust_fee: 123 } }))
  assert.equal(rows.find((r) => r.key === 'pf')!.displayValue, 123)
  assert.equal(rows.find((r) => r.key === 'industrial')!.displayValue, 0, '力调电费不进工商业小计')
  assert.equal(rows.find((r) => r.key === 'residential')!.displayValue, 0, '力调电费不进居民小计')
})

test('概览合计 = 工商业小计 + 居民小计 + 输配容需量 + 功率因数调整电费', () => {
  const rows = buildOverviewRows(makeInputs(
    { 'industrial-market': [row(100)], 'residential-catalog': [row(50)] },
    { capacity_fee: 40, pf_adjust_amount: 30 },
    {},
    { capacityDetail: { capacity: 2000, capacity_fee: 40 }, pfDetail: { adjust_fee: 30 } },
  ))
  assert.equal(overviewTotalOf(rows), 220, '100 + 50 + 40 + 30 = 220')
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

test('市场化/输配电量 = 四段计费电量之和（不是谷段单行）', () => {
  // 第 33 轮真机 Bug：概况显示 250,440（谷段），正确是 34,105+133,236+196,797+250,440。
  const rows = buildOverviewRows(makeInputs({
    'industrial-market': touRows([34105, 133236, 196797, 250440]),
    'industrial-trans': touRows([34105, 133236, 196797, 250440]),
  }))
  assert.equal(rows[0].qty, 614578, '四段求和 = 614,578')
  assert.equal(rows[2].qty, 614578)
})

test('线损/系统/政府基金取单行总电量（各行都是全量口径，不能相加）', () => {
  const rows = buildOverviewRows(makeInputs({
    'industrial-line': [{ ...row(10, 614578), period: '平' }],
    'industrial-sys': [{ ...row(10, 614578), period: '平' }, { ...row(5, 614578), period: '平' }],
    'industrial-gov': [{ ...row(1, 614578), period: '平' }],
  }))
  assert.equal(rows[1].qty, 614578)
  assert.equal(rows[3].qty, 614578, '多行同值取最大，不累加成 1,229,156')
  assert.equal(rows[4].qty, 614578)
})

test('居民两项取居民计费电量（9,354）', () => {
  const rows = buildOverviewRows(makeInputs({
    'residential-catalog': [{ ...row(4679.86, 9354), period: '平' }],
    'residential-gov': [{ ...row(277.76, 9354), period: '平' }],
  }))
  assert.equal(rows[6].qty, 9354)
  assert.equal(rows[7].qty, 9354)
})

test('容需量行的计费数量 = 容量（2,000），无明细时 0', () => {
  const withDetail = buildOverviewRows(makeInputs({}, { capacity: 2000 }, {}, {
    capacityDetail: { capacity: 2000, capacity_price: 22, capacity_fee: 44000 },
  }))
  assert.equal(withDetail.find((r) => r.key === 'capacity')!.qty, 2000)
  const noDetail = buildOverviewRows(makeInputs({}, { capacity: 2000 }))
  assert.equal(noDetail.find((r) => r.key === 'capacity')!.qty, 2000, '顶层容量仍可用于展示数量')
})

test('类别电量保留 1 位小数', () => {
  const rows = buildOverviewRows(makeInputs({ 'industrial-market': [{ ...row(10, 123.456), period: '尖峰' }] }))
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

// ---------------------------------------------------------------------------
// 第 33 轮：双列独立计算（电费 = 纯明细实算；账单电费 = 票面直提）
// ---------------------------------------------------------------------------

test('明细为空时：电费列必须为 0，账单电费仍显示票面值，对比状态判异常', () => {
  // 真机事故：功率因素明细数组被清空，旧实现用顶层 pf_adjust_amount 兜底，
  // 于是「电费」列也显示 -3,394.92 —— 两列同源，异常永远发现不了。
  const rows = buildOverviewRows(makeInputs(
    {},
    { capacity_fee: 44000, pf_adjust_amount: -3394.92, capacity: 2000 },
    {},
    { capacityDetail: null, pfDetail: null },
  ))
  const pf = rows.find((r) => r.key === 'pf')!
  assert.equal(pf.displayValue, 0, '无明细 → 电费列 0.00')
  assert.equal(pf.billFee, -3394.92, '账单电费取票面提取值')
  assert.equal(feeCloseOf(pf), false, '0 vs -3394.92 → 异常')

  const cap = rows.find((r) => r.key === 'capacity')!
  assert.equal(cap.displayValue, 0, '无容量明细 → 电费列 0.00')
  assert.equal(cap.billFee, 44000)
  assert.equal(feeCloseOf(cap), false)
})

test('明细完整时：电费列由明细实算，与账单电费一致判正常', () => {
  const rows = buildOverviewRows(makeInputs(
    {},
    { capacity_fee: 44000, pf_adjust_amount: -3394.92 },
    {},
    {
      capacityDetail: { demand_fee: 0, capacity: 2000, capacity_price: 22, capacity_fee: 44000 },
      pfDetail: { adjust_fee: -3394.92 },
    },
  ))
  const cap = rows.find((r) => r.key === 'capacity')!
  assert.equal(cap.displayValue, 44000)
  assert.equal(feeCloseOf(cap), true)
  const pf = rows.find((r) => r.key === 'pf')!
  assert.equal(pf.displayValue, -3394.92)
  assert.equal(feeCloseOf(pf), true)
  // 本期电费 = 0 + 0 + 44000 - 3394.92
  assert.equal(overviewTotalOf(rows), 40605.08)
})
