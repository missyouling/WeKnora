import assert from 'node:assert/strict'
import test from 'node:test'
import { mergeAiFieldSuggestions, standardFieldName, aiStandardFieldLabel } from './aiFieldMerge'
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
  // 2026-10-18 终轮：建议键统一归一为 camelCase 落库名（与分类 subs 写法一致）
  assert.ok(names.includes('buyerName'), `实际: ${names.join(',')}`)
  assert.ok(names.includes('sellerTaxNo'), `实际: ${names.join(',')}`)
  assert.equal(names.includes('taxRate'), false, 'enable=false 的建议不得并入')
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

// 2026-10-18 需求变更：原行为是"没有 items 字段就把子列丢掉并提示无处挂载"，
// 用户反馈这等于让人先去改字段类型才能保存。现改为自动补建「项目明细」父字段。
test('没有 items 字段时自动补建父字段（不再丢弃子列）', () => {
  const current: EditableField[] = [{ name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 }]
  const r = mergeAiFieldSuggestions(current, [], [{ key: 'qty', label: '数量' }], [], ['qty'])
  assert.equal(r.createdItemsHost, true)
  const host = r.fields.find((f) => f.dataType === 'items')!
  assert.deepEqual(host.subFields?.map((s) => s.key), ['qty'])
  assert.equal(r.skipped.length, 0)
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
// ---------------------------------------------------------------------------
// 2026-10-18 重构：三分组四列 + 扩展字段 key + 自动补建明细父字段
// ---------------------------------------------------------------------------

test('扩展字段按 key 身份去重：改中文名不会重复追加同一字段', () => {
  const current: EditableField[] = []
  const r1 = mergeAiFieldSuggestions(current, [], [], [{ key: 'bank_account', name: '开户账号' }], [], ['bank_account'])
  assert.equal(r1.addedCount, 1)
  const pushed = r1.fields[0]!
  assert.equal(pushed.name, 'bank_account', '身份用后端 key')
  assert.equal(pushed.label, '开户账号', '中文名存 label，不占 name')

  // 第二次换了个中文名，但 key 相同：不应再追加
  const r2 = mergeAiFieldSuggestions(r1.fields, [], [], [{ key: 'bank_account', name: '银行账号' }], [], ['bank_account'])
  assert.equal(r2.addedCount, 0)
  assert.equal(r2.fields.length, 1)
})

test('扩展字段中文名与已有字段 label 冲突时跳过并给原因', () => {
  const current: EditableField[] = [
    { name: 'custom_a', label: '开户账号', enabled: true, isDefault: false, dataType: 'text', width: 0 },
  ]
  const r = mergeAiFieldSuggestions(current, [], [], [{ key: 'bank_account', name: '开户账号' }], [], ['bank_account'])
  assert.equal(r.addedCount, 0)
  assert.equal(r.skipped.length, 1)
  assert.match(r.skipped[0]!.reason, /同名/)
})

test('勾选明细子列但分类没有 items 字段：自动补建「项目明细」父字段', () => {
  const current: EditableField[] = [
    { name: 'invoice_no', enabled: true, isDefault: true, dataType: 'text', width: 0 },
  ]
  const r = mergeAiFieldSuggestions(
    current,
    [], [{ key: 'plate_no', label: '车牌号', data_type: 'text' }], [],
    ['plate_no'], [],
  )
  assert.equal(r.createdItemsHost, true, '应自动创建 items 父字段')
  const host = r.fields.find((f) => f.dataType === 'items')
  assert.ok(host, 'items 父字段必须存在')
  assert.equal(host!.name, 'items')
  assert.equal(host!.label, '项目明细')
  assert.equal(host!.enabled, true)
  assert.deepEqual(host!.subFields?.map((s) => s.key), ['plate_no'])
  // 不再出现"无处挂载"的跳过原因
  assert.equal(r.skipped.filter((s) => /无处挂载/.test(s.reason)).length, 0)
})

test('标准字段的中文名可改：写入 label，契约键 name 不变', () => {
  const r = mergeAiFieldSuggestions(
    [],
    [{ key: 'invoice_date', label: '开票日', data_type: 'date', enable: true }],
  )
  // 2026-10-18 终轮：契约键归一到分类写法（invoiceDate），但绝不被中文名替换
  assert.equal(r.fields[0]!.name, 'invoiceDate', '契约键应归一为 camelCase')
  assert.equal(r.fields[0]!.label, '开票日')
  assert.equal(r.fields[0]!.dataType, 'date')
})

test('扩展字段支持 array 数据类型（后端已放宽），非法类型回落 text', () => {
  const r = mergeAiFieldSuggestions(
    [], [], [],
    [
      { key: 'attach_list', name: '附件清单', data_type: 'array' },
      { key: 'weird', name: '怪类型', data_type: 'bogus' },
    ],
    [], ['attach_list', 'weird'],
  )
  assert.equal(r.fields.find((f) => f.name === 'attach_list')!.dataType, 'array')
  assert.equal(r.fields.find((f) => f.name === 'weird')!.dataType, 'text')
})

// ---- 2026-10-18 终轮：标准建议键归一到分类既有写法 ----

test('standardFieldName 把 snake_case 契约键归一为 camelCase 字段名', () => {
  assert.equal(standardFieldName('invoice_no'), 'invoiceNo')
  assert.equal(standardFieldName('invoice_code'), 'invoiceCode')
  assert.equal(standardFieldName('total_amount'), 'totalAmount')
  // 已经是 camelCase / 字典里没有的键：原样保留
  assert.equal(standardFieldName('invoiceNo'), 'invoiceNo')
  assert.equal(standardFieldName('account_no'), 'account_no')
})

test('合入标准字段时用 camelCase 命名，且与既有字段 canonical 去重', () => {
  const current: EditableField[] = [
    { name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 },
  ]
  const merged = mergeAiFieldSuggestions(
    current,
    [
      { key: 'invoice_no', label: '发票号码', data_type: 'text', enable: true },
      { key: 'invoice_code', label: '发票代码', data_type: 'text', enable: true },
    ],
    [],
    [],
  )
  const names = merged.fields.map((f) => f.name)
  assert.ok(!names.includes('invoice_no'), '不得新增 snake_case 重复行')
  assert.ok(names.includes('invoiceCode'), '缺失的标准字段应以 camelCase 追加')
  assert.equal(merged.addedCount, 1)
})
