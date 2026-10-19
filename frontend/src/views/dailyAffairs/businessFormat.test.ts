import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  formatAmount,
  formatRate,
  itemValOf,
  formatItemCell,
  itemCellClass,
  subItemPopoverWidth,
  enabledSubFields,
  ITEM_STD_KEYS,
} from './businessFormat'

const popoverSrc = readFileSync(new URL('./BusinessSubItemsPopover.vue', import.meta.url), 'utf8')

test('金额千分位两位小数；空值返回空串', () => {
  assert.equal(formatAmount(1234.5), '1,234.50')
  assert.equal(formatAmount(0), '0.00')
  assert.equal(formatAmount(undefined), '')
  assert.equal(formatAmount(null), '')
  assert.equal(formatAmount(''), '')
  // 脏数据不炸
  assert.equal(formatAmount('abc' as never), '')
})

test('税率小数转百分比（0.03 → 3%）', () => {
  assert.equal(formatRate(0.03), '3%')
  assert.equal(formatRate(0.13), '13%')
  assert.equal(formatRate(undefined), '')
})

test('明细取值：标准键取行属性，自定义子列回退 extra 桶', () => {
  const row = { name: '通行费', extra: { plate_no: '渝C87567' } }
  assert.equal(itemValOf(row, { key: 'name' }), '通行费')
  assert.equal(itemValOf(row, { key: 'plate_no' }), '渝C87567')
  // 自定义键也可能被提到行顶层（历史数据），优先取顶层
  assert.equal(itemValOf({ plate_no: '渝A1' , extra: { plate_no: '渝B2' } }, { key: 'plate_no' }), '渝A1')
  assert.equal(itemValOf({}, { key: 'name' }), '')
})

test('明细单元格格式化：税率百分比 / 数值千分位 / 空值 -', () => {
  const row = { name: '通行费', amount: 234.25, tax_rate: 0.03 }
  assert.equal(formatItemCell(row, { key: 'name' }), '通行费')
  assert.equal(formatItemCell(row, { key: 'amount', dataType: 'number' }), '234.25')
  assert.equal(formatItemCell(row, { key: 'tax_rate', dataType: 'number' }), '3%')
  assert.equal(formatItemCell(row, { key: 'spec' }), '-')
})

test('单元格 class 按 key/dataType 映射', () => {
  assert.equal(itemCellClass({ key: 'name' }), 'item-cell--name')
  assert.equal(itemCellClass({ key: 'amount', dataType: 'number' }), 'item-cell--num')
  assert.equal(itemCellClass({ key: 'travel_from', dataType: 'date' }), 'item-cell--date')
  assert.equal(itemCellClass({ key: 'spec', dataType: 'text' }), 'item-cell--text')
})

test('浮层宽度 = 启用列宽之和 + 20，夹在 [280, 820]', () => {
  // 6 列 × 96 + 20 = 596
  assert.equal(subItemPopoverWidth(Array.from({ length: 6 }, () => ({ key: 'k' }))), 596)
  // 无列回退最小宽
  assert.equal(subItemPopoverWidth([]), 280)
  assert.equal(subItemPopoverWidth(undefined as never), 280)
  // 自定义列宽生效：200 + 200 + 20 = 420
  assert.equal(subItemPopoverWidth([{ width: 200 }, { width: 200 }]), 420)
  // 列宽之和不足下限时按下限给（100 + 120 + 20 = 240 → 280）
  assert.equal(subItemPopoverWidth([{ width: 100 }, { width: 120 }]), 280)
  // 上限：40 列远超 820
  assert.equal(subItemPopoverWidth(Array.from({ length: 40 }, () => ({ key: 'k' }))), 820)
  // 自定义边界
  assert.equal(subItemPopoverWidth([{ width: 500 }], 300, 400), 400)
})

test('只渲染 enabled !== false 的子列（缺省视为启用）', () => {
  const cols = [{ key: 'a' }, { key: 'b', enabled: true }, { key: 'c', enabled: false }]
  assert.deepEqual(enabledSubFields(cols).map((c: any) => c.key), ['a', 'b'])
  assert.deepEqual(enabledSubFields(undefined as never), [])
})

test('标准键清单与后端明细结构一致（8 个独立存储位）', () => {
  for (const k of ['name', 'spec', 'unit', 'qty', 'price', 'amount', 'tax_rate', 'tax_amount']) {
    assert.ok(ITEM_STD_KEYS.has(k), `${k} 必须是标准键`)
  }
  assert.equal(ITEM_STD_KEYS.has('plate_no'), false)
})

test('BusinessSubItemsPopover 只吃 props，不认识任何业务 scope', () => {
  // 中台化的判定：组件源码里不得出现具体业务分支
  assert.doesNotMatch(popoverSrc, /scope\s*===/)
  assert.doesNotMatch(popoverSrc, /'invoice'/)
  assert.match(popoverSrc, /subItemPopoverWidth\(/)
  assert.match(popoverSrc, /enabledSubFields\(/)
  assert.match(popoverSrc, /:overlay-class-name="overlayClassName"/)
  // 空态必须给，不能整块 v-if 掉
  assert.match(popoverSrc, /biz-sub-empty/)
})

test('发票列表页已切换使用中台浮层（不再内联 tooltip 表格）', () => {
  const src = readFileSync(new URL('./InvoiceManagement.vue', import.meta.url), 'utf8')
  assert.match(src, /<BusinessSubItemsPopover/)
  assert.doesNotMatch(src, /rate-detail-table/)
  assert.doesNotMatch(src, /itemsPopoverWidth\(/)
})
