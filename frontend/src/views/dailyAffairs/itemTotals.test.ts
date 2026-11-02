// 明细合计的单测（第 24 轮任务三）。
//
// 锁三件事：
//  1. 只有金额/税额参与合计（数量/单价绝不累加）；
//  2. 负数折扣行正确参与（按「分」整数累加，不留浮点尾巴）；
//  3. 非合计列留空串（不是 0）—— 写 0 会被误读成「合计值为 0」。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import { hasSumColumn, isSumColumn, numOfItem, sumTextOfColumn, totalRowTexts } from './itemTotals'

const COLS = [
  { key: 'name', label: '项目名称' },
  { key: 'spec', label: '规格型号' },
  { key: 'unit', label: '单位' },
  { key: 'qty', label: '数量' },
  { key: 'price', label: '单价' },
  { key: 'amount', label: '金额' },
  { key: 'tax_rate', label: '税率' },
  { key: 'tax_amount', label: '税额' },
]

// 真机 P1 的 6 行明细（3 正 + 3 折扣）
const P1_ITEMS = [
  { name: '*柴油*0号', spec: '0#柴油', unit: '升', qty: 7085.54, price: 7.15884181022, amount: 50724.26, tax_rate: 0.13, tax_amount: 6594.15 },
  { name: '*柴油*0号', spec: '', unit: '', qty: null, price: null, amount: -513.35, tax_rate: 0.13, tax_amount: -66.73 },
  { name: '*汽油*92号', spec: '92#汽油', unit: '升', qty: 115.67, price: 7.63197026022, amount: 882.79, tax_rate: 0.13, tax_amount: 114.76 },
  { name: '*汽油*92号', spec: '', unit: '', qty: null, price: null, amount: -8.05, tax_rate: 0.13, tax_amount: -1.05 },
  { name: '*汽油*95号', spec: '95#汽油', unit: '升', qty: 117.67, price: 7.76960992606, amount: 914.25, tax_rate: 0.13, tax_amount: 118.85 },
  { name: '*汽油*95号', spec: '', unit: '', qty: null, price: null, amount: -10.41, tax_rate: 0.13, tax_amount: -1.35 },
]

test('isSumColumn：只有金额与税额两列参与合计', () => {
  assert.equal(isSumColumn({ key: 'amount', label: '金额' }), true)
  assert.equal(isSumColumn({ key: 'tax_amount', label: '税额' }), true)
  // 数量/单价也是数值，但合计没有业务意义 —— 绝不能累加
  assert.equal(isSumColumn({ key: 'qty', label: '数量' }), false)
  assert.equal(isSumColumn({ key: 'price', label: '单价' }), false)
  assert.equal(isSumColumn({ key: 'name', label: '项目名称' }), false)
  assert.equal(isSumColumn({ key: 'spec', label: '规格型号' }), false)
  assert.equal(isSumColumn({ key: 'tax_rate', label: '税率' }), false)
})

test('isSumColumn：中文标签兜底（历史配置 dataType 不统一）', () => {
  // 分类字段配置里金额列可能标成 number，只能靠标签认
  assert.equal(isSumColumn({ key: 'some_custom_key', label: '金额' }), true)
  assert.equal(isSumColumn({ key: 'another_key', label: '税额' }), true)
  assert.equal(isSumColumn({ key: 'x', label: '价税合计' }), true)
})

test('sumTextOfColumn：P1 金额合计 = 51,989.49', () => {
  // 50724.26 - 513.35 + 882.79 - 8.05 + 914.25 - 10.41
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'amount', label: '金额' }), '51,989.49')
})

test('sumTextOfColumn：P1 税额合计 = 6,758.63', () => {
  // 6594.15 - 66.73 + 114.76 - 1.05 + 118.85 - 1.35
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'tax_amount', label: '税额' }), '6,758.63')
})

test('sumTextOfColumn：非合计列给空串（不是 0）', () => {
  // 写 '0' 会被误读成「这一列的合计是 0」，空串才是「不适用」
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'spec', label: '规格型号' }), '')
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'unit', label: '单位' }), '')
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'qty', label: '数量' }), '')
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'price', label: '单价' }), '')
  assert.equal(sumTextOfColumn(P1_ITEMS, { key: 'tax_rate', label: '税率' }), '')
})

test('sumTextOfColumn：按分整数累加，不留浮点尾巴', () => {
  // 裸浮点相加 0.1+0.2 会得到 0.30000000000000004
  const items = [{ amount: 0.1 }, { amount: 0.2 }]
  assert.equal(sumTextOfColumn(items, { key: 'amount', label: '金额' }), '0.30')
  // 真机脏值样板：多项相加后必须干净
  const dirty = [{ amount: 53441.77999999996 }, { amount: 0.00000000004 }]
  assert.equal(sumTextOfColumn(dirty, { key: 'amount', label: '金额' }), '53,441.78')
})

test('sumTextOfColumn：空明细给 0.00（不是空串）', () => {
  assert.equal(sumTextOfColumn([], { key: 'amount', label: '金额' }), '0.00')
})

test('sumTextOfColumn：非数值/缺失当 0 参与（不产生 NaN）', () => {
  const items = [{ amount: '100.5' }, { amount: null }, { amount: '' }, {}]
  assert.equal(sumTextOfColumn(items, { key: 'amount', label: '金额' }), '100.50')
})

test('numOfItem：标准键优先，回退 extra', () => {
  assert.equal(numOfItem({ amount: 12.5 }, { key: 'amount' }), 12.5)
  assert.equal(numOfItem({ extra: { amount: 8.25 } }, { key: 'amount' }), 8.25)
  assert.equal(numOfItem({}, { key: 'amount' }), 0)
})

test('hasSumColumn：有金额/税额列才渲染合计行', () => {
  assert.equal(hasSumColumn(COLS), true)
  // 只有名称/规格的表不该多出一条空合计行
  assert.equal(hasSumColumn([{ key: 'name', label: '名称' }, { key: 'spec', label: '规格' }]), false)
  assert.equal(hasSumColumn([]), false)
})

test('totalRowTexts：首列固定「合计」，其余按列判定', () => {
  const texts = totalRowTexts(P1_ITEMS, COLS)
  assert.equal(texts.length, COLS.length, '单元格数必须与列数一致')
  assert.equal(texts[0], '合计')
  assert.equal(texts[COLS.findIndex((c) => c.key === 'amount')], '51,989.49')
  assert.equal(texts[COLS.findIndex((c) => c.key === 'tax_amount')], '6,758.63')
  // 不可累加列留空
  for (const k of ['spec', 'unit', 'qty', 'price', 'tax_rate']) {
    assert.equal(texts[COLS.findIndex((c) => c.key === k)], '', `${k} 必须留空`)
  }
})

test('totalRowTexts：单列时首列既是「合计」也可能承载金额', () => {
  // 只有一列金额时，首列被「合计」占用 → 金额值会丢失。
  // 这是已知边界：明细表至少 3 列（名称 + 金额 + 税额），实际不会触发。
  const texts = totalRowTexts(P1_ITEMS, [{ key: 'amount', label: '金额' }])
  assert.equal(texts.length, 1)
  assert.equal(texts[0], '合计')
})
