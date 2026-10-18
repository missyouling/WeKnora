import assert from 'node:assert/strict'
import test from 'node:test'
import { mergeAiFieldSuggestions, aiStandardFieldLabel } from './aiFieldMerge'
import type { EditableField } from './fleetCategorySubs'

// 智能字段建议的合并语义三条铁律：
//   1. 勾选的才并入；2. 不改已存在字段（只补明细子列）；3. 同名不重复。

const base: EditableField[] = [
  { name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 },
  { name: 'items', enabled: true, isDefault: false, dataType: 'items', width: 0,
    subFields: [{ key: 'name', label: '项目名称', dataType: 'text', width: 140 }] },
]

test('只并入勾选（enable=true）的标准字段建议', () => {
  const r = mergeAiFieldSuggestions(base, [
    { key: 'buyer_name', enable: true },
    { key: 'tax_rate', enable: false },
    { key: 'seller_tax_no', enable: true },
  ])
  const names = r.fields.map((f) => f.name)
  assert.ok(names.includes('buyer_name'), `实际: ${names.join(',')}`)
  assert.ok(names.includes('seller_tax_no'), `实际: ${names.join(',')}`)
  assert.equal(names.includes('tax_rate'), false, 'enable=false 的建议不得并入')
  assert.equal(r.addedCount, 2)
})

test('已存在的字段不被覆盖（保持用户既有配置）', () => {
  const current: EditableField[] = [
    { name: 'invoiceNo', enabled: false, isDefault: false, dataType: 'text', width: 200 },
  ]
  const r = mergeAiFieldSuggestions(current, [{ key: 'invoiceNo', enable: true }])
  const f = r.fields[0]!
  assert.equal(f.enabled, false, '用户已禁用的字段不应被建议重新启用')
  assert.equal(f.width, 200, '已有列宽不应被建议改掉')
  assert.equal(r.addedCount, 0)
})

test('camelCase 与 snake_case 视为同一字段（不重复追加）', () => {
  const current: EditableField[] = [{ name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 }]
  const r = mergeAiFieldSuggestions(current, [{ key: 'invoice_no', enable: true }])
  assert.equal(r.fields.length, 1)
  assert.equal(r.addedCount, 0)
})

test('明细子列只补给已有的 items 字段，并去重', () => {
  const r = mergeAiFieldSuggestions(
    base,
    [],
    [
      { key: 'name', label: '项目名称' },
      { key: 'account_no', label: '户号' },
    ],
    [],
    ['name', 'account_no'], // 勾选两个（name 已存在应被去重）
  )
  const host = r.fields.find((f) => f.dataType === 'items')!
  assert.equal(host.subFields!.length, 2)
  assert.deepEqual(host.subFields!.map((s) => s.key), ['name', 'account_no'])
  assert.equal(r.addedCount, 1, '只有 account_no 是新增')
})

test('没有 items 字段时，明细子列被跳过并说明原因（不给静默失败）', () => {
  const current: EditableField[] = [{ name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 }]
  const r = mergeAiFieldSuggestions(current, [], [{ key: 'qty', label: '数量' }], [], ['qty'])
  assert.equal(r.addedCount, 0)
  assert.equal(r.skipped.length, 1)
  assert.match(r.skipped[0]!.reason, /明细/)
})

test('自定义字段按中文名去重，且与已有字段冲突时跳过', () => {
  const current: EditableField[] = [
    { name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 },
    { name: '户号', enabled: true, isDefault: false, dataType: 'text', width: 0 },
  ]
  const r = mergeAiFieldSuggestions(
    current, [],
    [],
    [
      { name: '户号', data_type: 'text' },
      { name: '缴费期', data_type: 'text' },
      { name: '缴费期', data_type: 'text' },
    ],
    [],
    ['户号', '缴费期', '缴费期'],
  )
  const names = r.fields.map((f) => f.name)
  assert.equal(names.filter((n) => n === '户号').length, 1)
  assert.equal(names.filter((n) => n === '缴费期').length, 1)
  assert.ok(r.skipped.some((s) => s.name === '户号'))
})

test('不修改入参数组（原列表保持不变）', () => {
  const current: EditableField[] = [
    { name: 'items', enabled: true, isDefault: false, dataType: 'items', width: 0,
      subFields: [{ key: 'name', label: '项目名称', dataType: 'text', width: 0 }] },
  ]
  const snapshot = JSON.stringify(current)
  mergeAiFieldSuggestions(current, [], [{ key: 'qty', label: '数量' }], [], ['qty'])
  assert.equal(JSON.stringify(current), snapshot, '合并必须返回新数组而不是原地修改')
})

test('标准键的中文名解析（snake_case 契约键 → 中文展示名）', () => {
  assert.equal(aiStandardFieldLabel('invoice_no'), '发票号码')
  assert.equal(aiStandardFieldLabel('invoice_date'), '开票日期')
  // 模型给了 label 就用模型的
  assert.equal(aiStandardFieldLabel('invoice_no', '发票号码（20位）'), '发票号码（20位）')
})