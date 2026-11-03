import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  FIELD_FILTER_PREFIX,
  buildFieldFiltersParam,
  buildInvoiceFieldFilterSpecs,
  filterableLabelOf,
  pickFieldFilterValues,
} from './invoiceFilterFields'

// 第 26 轮 O-3：筛选器从分类列定义派生（六同步第 ② 环）。
//
// 真机场景：通行费发票分类的 subs 里有「车牌号」列，但筛选栏之前只有
// 「关键词 / 税率 / 开票日期」，用户只能把车牌丢进关键词框（会误命中备注）。

test('O-3a：车牌号列必须派生出 text 筛选器', () => {
  const specs = buildInvoiceFieldFilterSpecs([
    { key: 'plate_no', label: '车牌号' },
    { key: 'invoiceNo', label: '发票号码' },
  ])
  assert.equal(specs.length, 1, '只有白名单字段生成筛选器')
  assert.equal(specs[0].type, 'text', '车牌号必须是自由输入（候选值太多，下拉选不到）')
  assert.equal(specs[0].key, `${FIELD_FILTER_PREFIX}plate_no`)
  assert.equal(specs[0].placeholder, '车牌号')
})

test('O-3b：无白名单字段时不生成任何筛选器（工具栏保持原样）', () => {
  const specs = buildInvoiceFieldFilterSpecs([
    { key: 'invoiceNo', label: '发票号码' },
    { key: 'amount', label: '金额' },
  ])
  assert.equal(specs.length, 0)
})

test('O-3c：中文键同样识别，且同名只生成一个', () => {
  const specs = buildInvoiceFieldFilterSpecs([
    { key: '车牌号', label: '车牌号' },
    { key: 'plate_no', label: '车牌号' },
  ])
  assert.equal(specs.length, 1, '中英文键指向同一字段，不能出现两个输入框')
  assert.equal(specs[0].placeholder, '车牌号')
})

test('O-3d：null / 非数组输入不崩', () => {
  assert.deepEqual(buildInvoiceFieldFilterSpecs(null), [])
  assert.deepEqual(buildInvoiceFieldFilterSpecs(undefined), [])
  assert.deepEqual(buildInvoiceFieldFilterSpecs([] as any), [])
})

test('O-3e：field_filters 参数用分号拼接，空值跳过', () => {
  assert.equal(
    buildFieldFiltersParam({ keyword: 'x', [`${FIELD_FILTER_PREFIX}plate_no`]: '渝A75R1C' }),
    'plate_no:渝A75R1C',
  )
  assert.equal(
    buildFieldFiltersParam({
      [`${FIELD_FILTER_PREFIX}plate_no`]: '渝A75R1C',
      [`${FIELD_FILTER_PREFIX}vehicle_type`]: '客车',
    }),
    'plate_no:渝A75R1C;vehicle_type:客车',
  )
  // 清空后不能传 `plate_no:`（后端会当成筛不到任何记录）
  assert.equal(buildFieldFiltersParam({ [`${FIELD_FILTER_PREFIX}plate_no`]: '' }), '')
  assert.equal(buildFieldFiltersParam({ [`${FIELD_FILTER_PREFIX}plate_no`]: '   ' }), '')
  assert.equal(buildFieldFiltersParam({}), '')
})

test('O-3f：pickFieldFilterValues 只抽 field:* 键', () => {
  const got = pickFieldFilterValues({
    keyword: 'kw',
    taxRate: 0.09,
    [`${FIELD_FILTER_PREFIX}plate_no`]: '渝A75R1C',
  })
  assert.deepEqual(got, { [`${FIELD_FILTER_PREFIX}plate_no`]: '渝A75R1C' })
})

test('O-3g：filterableLabelOf 对未知字段返回空串', () => {
  assert.equal(filterableLabelOf('plate_no'), '车牌号')
  assert.equal(filterableLabelOf('unknown_field'), '')
  assert.equal(filterableLabelOf(''), '')
})
