import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  EMPTY_FILTER_VALUE,
  DEFAULT_FILTER_ICONS,
  normalizeFilterValue,
  patchFilterState,
  splitFilterSpecs,
  rangePair,
  writeRange,
  type FilterFieldSpec,
} from './businessFilter'

const barSrc = readFileSync(new URL('./BusinessFilterBar.vue', import.meta.url), 'utf8')
const pageSrc = readFileSync(new URL('./InvoiceManagement.vue', import.meta.url), 'utf8')

const spec = (key: string, type: FilterFieldSpec['type']): FilterFieldSpec => ({ key, type })

test('空值口径：日期/区间是数组，其余是空串（清空不能留 null）', () => {
  assert.deepEqual(EMPTY_FILTER_VALUE['date-range'], [])
  assert.deepEqual(EMPTY_FILTER_VALUE['number-range'], [])
  assert.equal(EMPTY_FILTER_VALUE.keyword, '')
  assert.equal(EMPTY_FILTER_VALUE.select, '')
})

test('归一化：TDesign 清空的 undefined/null 落回该类型空值', () => {
  assert.equal(normalizeFilterValue('select', undefined), '')
  assert.equal(normalizeFilterValue('select', null), '')
  assert.equal(normalizeFilterValue('keyword', undefined), '')
  // 脏数据（对象/字符串）不能污染数组型控件
  assert.deepEqual(normalizeFilterValue('date-range', '2026-01-01'), [])
  assert.deepEqual(normalizeFilterValue('date-range', { a: 1 }), [])
  assert.deepEqual(normalizeFilterValue('date-range', ['2026-01-01', '2026-01-31']), ['2026-01-01', '2026-01-31'])
})

test('状态合并是不可变更新（原对象不被改写）', () => {
  const before = { keyword: 'abc', taxRate: '' }
  const next = patchFilterState(before, spec('taxRate', 'select'), '0.03')
  assert.equal(next.taxRate, '0.03')
  assert.equal(next.keyword, 'abc', '其余筛选条件保留')
  assert.equal(before.taxRate, '', '原状态不被改写')
})

test('spec 拆分：keyword 进搜索位，其余按声明顺序保留', () => {
  const { keyword, rest } = splitFilterSpecs([
    spec('keyword', 'keyword'),
    spec('invoiceType', 'select'),
    spec('taxRate', 'select'),
    spec('dateRange', 'date-range'),
  ])
  assert.equal(keyword?.key, 'keyword')
  assert.deepEqual(rest.map((f) => f.key), ['invoiceType', 'taxRate', 'dateRange'])
  // 重复 keyword 只认第一个
  const dup = splitFilterSpecs([spec('a', 'keyword'), spec('b', 'keyword'), spec('c', 'select')])
  assert.equal(dup.keyword?.key, 'a')
  assert.deepEqual(dup.rest.map((f) => f.key), ['c'])
  // 空/未传不炸
  assert.deepEqual(splitFilterSpecs(undefined as never).rest, [])
})

test('数值区间读写：空串/null 归一为 undefined', () => {
  assert.deepEqual(rangePair(['1', '9']), ['1', '9'])
  assert.deepEqual(rangePair(undefined), [undefined, undefined])
  assert.deepEqual(rangePair('x'), [undefined, undefined])
  assert.deepEqual(writeRange('', '9'), [undefined, '9'])
  assert.deepEqual(writeRange(1, null), [1, undefined])
  assert.deepEqual(writeRange('0', '0'), ['0', '0'], '0 是有效值，不能被当空值抹掉')
})

test('四类控件各有默认图标（缺省不裸奔）', () => {
  for (const t of ['keyword', 'select', 'date-range', 'number-range'] as const) {
    assert.ok(DEFAULT_FILTER_ICONS[t], `${t} 应有默认图标`)
  }
})

test('组件只按 spec 渲染，不认识任何业务 scope', () => {
  assert.doesNotMatch(barSrc, /scope\s*===/)
  assert.doesNotMatch(barSrc, /'invoice'/)
  // 四类控件齐全
  assert.match(barSrc, /f\.type === 'select'/)
  assert.match(barSrc, /f\.type === 'date-range'/)
  assert.match(barSrc, /f\.type === 'number-range'/)
  // 列显隐、设置、主操作、批量条全部走 props/插槽
  assert.match(barSrc, /BusinessColumnFilter/)
  assert.match(barSrc, /@click="emit\('settings'\)"/)
  assert.match(barSrc, /@click="emit\('primary'\)"/)
  assert.match(barSrc, /slot name="batch-actions"/)
})

test('发票页已切换为 spec 声明（3 个筛选器不再手写控件）', () => {
  assert.match(pageSrc, /<BusinessFilterBar/)
  assert.match(pageSrc, /:fields="filterFields"/)
  assert.match(pageSrc, /v-model="filterState"/)
  // 不再内联日期区间/税率下拉
  assert.doesNotMatch(pageSrc, /t-date-range-picker/)
  assert.doesNotMatch(pageSrc, /<BusinessListToolbar/)
  // 三个筛选器齐全：关键词/税率/日期（发票类型筛选已移至顶部概览卡片）
  assert.match(pageSrc, /key: 'keyword', type: 'keyword'/)
  assert.match(pageSrc, /key: 'taxRate', type: 'select'/)
  assert.match(pageSrc, /key: 'dateRange', type: 'date-range'/)
  // 发票类型下拉已从工具栏移除（仅由顶部 KPI 卡片承担）
  assert.doesNotMatch(pageSrc, /key: 'invoiceType', type: 'select'/)
  // 300ms 防抖与 AND 复合查询语义保留
  assert.match(pageSrc, /watch\(filterState, \(\) => \{\s*scheduleFilter\(\)/)
  assert.match(pageSrc, /}, 300\)/)
})
