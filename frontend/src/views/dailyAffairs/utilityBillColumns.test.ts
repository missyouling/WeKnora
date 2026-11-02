// 市电账单列定义与费用子表列派生的单测（第 24 轮任务一瘦身）。
//
// 这些口径原本写在 3800 行的 .vue 里，只能靠源文本断言「某行代码还在」；
// 抽成纯函数后可以直接喂数据断言结果，比字符串匹配可靠得多。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import {
  DEFAULT_FEE_COLS,
  FALLBACK_COLUMNS,
  colWidthPx,
  displayMultiplierOf,
  feeCellTextOf,
  feeGroupColsOf,
  makeAmountCell,
} from './utilityBillColumns'

const fmt = {
  rate6: (v: any) => String(v),
  money: (v: any) => String(v),
}

test('FALLBACK_COLUMNS：20 列且顺序与后端 overview 字段逐项对齐', () => {
  assert.equal(FALLBACK_COLUMNS.length, 20, '账单概况必须是 20 列')
  assert.deepEqual(
    FALLBACK_COLUMNS.map((c) => c.key),
    [
      'bill_period', 'account_no', 'account_name', 'usage_category', 'voltage_level',
      'market_attr', 'supply_unit', 'address', 'meter_no', 'total_kwh',
      'total_amount', 'prev_kwh', 'mom_change', 'avg_price', 'power_factor',
      'due_date', 'industrial_amount', 'residential_amount', 'pf_adjust_amount', 'grand_total',
    ],
  )
  // 「账单周期」只能出现一列（历史上出现过「账单周期」+「账单周期起」两列）
  assert.equal(FALLBACK_COLUMNS.filter((c) => c.label === '账单周期').length, 1)
})

test('colWidthPx：subs.width 优先并 clamp 60~400', () => {
  assert.equal(colWidthPx({ ...FALLBACK_COLUMNS[0], width: 260 }), 260, '配置宽度精确生效')
  assert.equal(colWidthPx({ ...FALLBACK_COLUMNS[0], width: 10 }), 60, '过窄必须 clamp 到 60')
  assert.equal(colWidthPx({ ...FALLBACK_COLUMNS[0], width: 999 }), 400, '过宽必须 clamp 到 400')
  // width=0 / 未配置 → 回落内置 w
  assert.equal(colWidthPx({ ...FALLBACK_COLUMNS[0], width: 0 }), 110, '未配置时用内置 px 宽')
  // 内置 w 不是 px 时自适应
  assert.equal(colWidthPx({ ...FALLBACK_COLUMNS[0], width: 0, w: '1fr' }), undefined)
})

test('makeAmountCell：仅 amount 列生效，其它类型返回 undefined', () => {
  const amountCol = FALLBACK_COLUMNS.find((c) => c.fieldType === 'amount')!
  const textCol = FALLBACK_COLUMNS.find((c) => c.fieldType === 'text')!
  assert.ok(makeAmountCell(amountCol, fmt.money), 'amount 列必须有渲染器')
  assert.equal(makeAmountCell(textCol, fmt.money), undefined, 'text 列必须走默认渲染')
})

test('makeAmountCell：0 上屏、缺值留白', () => {
  const amountCol = FALLBACK_COLUMNS.find((c) => c.fieldType === 'amount')!
  const cell = makeAmountCell(amountCol, (v: any) => (v === 0 ? '0.00' : `${v}`))!
  assert.equal(cell(null, { row: { [amountCol.key]: 0 } }), '0.00', '0 必须上屏（不能当缺值）')
  assert.equal(cell(null, { row: { [amountCol.key]: undefined } }), '', '缺值必须留白')
  assert.equal(cell(null, { row: {} }), '', '字段不存在必须留白')
})

test('feeGroupColsOf：三级兜底顺序不变', () => {
  // ① 无配置 → 默认 5 列
  assert.deepEqual(feeGroupColsOf([], 'market'), DEFAULT_FEE_COLS)
  // ② 有配置但全被隐藏 → 显示全部（按 sort_order）
  const hidden = [
    { field_key: 'b', label: 'B', field_type: 'text', group: 'market', default_visible: false, sort_order: 2 },
    { field_key: 'a', label: 'A', field_type: 'text', group: 'market', default_visible: false, sort_order: 1 },
  ]
  assert.deepEqual(feeGroupColsOf(hidden, 'market').map((c) => c.field_key), ['a', 'b'])
  // ③ 有默认显示项 → 只取它们，按 sort_order
  const mixed = [
    { field_key: 'c', label: 'C', field_type: 'text', group: 'market', default_visible: false, sort_order: 1 },
    { field_key: 'b', label: 'B', field_type: 'text', group: 'market', default_visible: true, sort_order: 3 },
    { field_key: 'a', label: 'A', field_type: 'text', group: 'market', default_visible: true, sort_order: 2 },
  ]
  assert.deepEqual(feeGroupColsOf(mixed, 'market').map((c) => c.field_key), ['a', 'b'])
  // 已删除（deleted_at 非空）的配置不参与
  const withDeleted = [
    { field_key: 'a', label: 'A', field_type: 'text', group: 'market', default_visible: true, sort_order: 1, deleted_at: '2026-01-01' },
  ]
  assert.deepEqual(feeGroupColsOf(withDeleted, 'market'), DEFAULT_FEE_COLS, '全删后回落到默认列')
})

test('feeGroupColsOf：其它分组的配置不串味', () => {
  const list = [
    { field_key: 'a', label: 'A', field_type: 'text', group: 'market', default_visible: true, sort_order: 1 },
    { field_key: 'b', label: 'B', field_type: 'text', group: 'trans', default_visible: true, sort_order: 1 },
  ]
  assert.deepEqual(feeGroupColsOf(list, 'market').map((c) => c.field_key), ['a'])
  assert.deepEqual(feeGroupColsOf(list, 'trans').map((c) => c.field_key), ['b'])
})

test('feeCellTextOf：数值/金额列 0 上屏，文本列空值留白', () => {
  assert.equal(feeCellTextOf({ fee: 0 }, { field_key: 'fee', label: '电费', field_type: 'amount' }, fmt), '0')
  assert.equal(feeCellTextOf({ qty: 0 }, { field_key: 'qty', label: '电量', field_type: 'number' }, fmt), '0')
  assert.equal(feeCellTextOf({ name: '' }, { field_key: 'name', label: '名称', field_type: 'text' }, fmt), '')
  assert.equal(feeCellTextOf({ name: '零售交易电费' }, { field_key: 'name', label: '名称', field_type: 'text' }, fmt), '零售交易电费')
  // 缺字段 → 留白（不是 'undefined'）
  assert.equal(feeCellTextOf({}, { field_key: 'fee', label: '电费', field_type: 'amount' }, fmt), '')
})

test('displayMultiplierOf：只用提取值，不推算', () => {
  const rate6 = (v: any) => String(v)
  assert.equal(displayMultiplierOf({ multiplier: 1 }, rate6), '1')
  assert.equal(displayMultiplierOf({ multiplier: 0 }, rate6), '', '0 倍率视为无值（不给用户看 0）')
  assert.equal(displayMultiplierOf({ multiplier: null }, rate6), '')
  assert.equal(displayMultiplierOf({}, rate6), '', '没有倍率字段就留空，绝不按示数反算')
})
