import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildFleetColumns,
  buildFleetSummaryColumns,
  buildFleetFilterSpecs,
  buildFleetFieldFiltersParam,
  dataTypeOfSub,
} from './fleetColumns'

const plateMap = new Map<string, string>([['v1', '渝C81022']])

test('buildFleetColumns：停用字段不出列、按 subs 顺序产出', () => {
  const cols = buildFleetColumns(
    [
      { name: '车牌号', is_default: true, data_type: 'text' },
      { name: '费用类型', is_default: true, data_type: 'text' },
      { name: '税额', is_default: false, data_type: 'number' },
      { name: '备注', is_default: false, data_type: 'textarea', enabled: false },
    ],
    plateMap,
  )
  assert.deepEqual(cols.map((c) => c.key), ['车牌号', '费用类型', '税额'])
  assert.equal(cols[0].label, '车牌号')
  assert.equal(cols[2].default, false)
})

test('buildFleetColumns：车牌号走 vehicle_id 映射，缺映射时回退 data', () => {
  const cols = buildFleetColumns([{ name: '车牌号', data_type: 'text' }], plateMap)
  const plate = cols[0]
  assert.equal(plate.value({ vehicle_id: 'v1' }), '渝C81022')
  assert.equal(plate.value({ vehicle_id: 'v9', data: { 车牌号: '渝A11111' } }), '渝A11111')
  assert.equal(plate.value({}), '—')
})

test('buildFleetColumns：金额两位小数 + 千分位，数量整数不补 .00', () => {
  const cols = buildFleetColumns(
    [
      { name: '含税金额', data_type: 'number' },
      { name: '数量', data_type: 'number' },
    ],
    plateMap,
  )
  assert.equal(cols[0].value({ data: { 含税金额: 52015.9 } }), '52,015.90')
  assert.equal(cols[1].value({ data: { 数量: 1000 } }), '1000')
  assert.equal(cols[1].value({ data: { 数量: 3.5 } }), '3.50')
})

test('buildFleetColumns：空值统一 —，日期裁到 10 位', () => {
  const cols = buildFleetColumns(
    [
      { name: '通行起始日期', data_type: 'date' },
      { name: '入口站/起点', data_type: 'text' },
    ],
    plateMap,
  )
  assert.equal(cols[0].value({ data: { 通行起始日期: '2026-10-13T00:00:00' } }), '2026-10-13')
  assert.equal(cols[1].value({}), '—')
})

test('dataTypeOfSub：显式类型优先，缺省按字段名推断', () => {
  assert.equal(dataTypeOfSub({ name: '税额', data_type: 'text' }), 'text')
  assert.equal(dataTypeOfSub({ name: '出险日期' }), 'date')
  assert.equal(dataTypeOfSub({ name: '含税金额' }), 'number')
  assert.equal(dataTypeOfSub({ name: '理赔状态' }), 'text')
})

test('buildFleetSummaryColumns：金额/数量列可累加，文本列留空', () => {
  const cols = buildFleetColumns(
    [
      { name: '含税金额', data_type: 'number' },
      { name: '费用类型', data_type: 'text' },
    ],
    plateMap,
  )
  const sums = buildFleetSummaryColumns(cols)
  assert.equal(sums[0].fieldType, 'amount')
  assert.equal(sums[1].fieldType, 'text')
  // 取值器走 raw，不返回格式化字符串（否则合计行会拿到 '52,015.90' 无法相加）
  assert.equal(sums[0].value!({ data: { 含税金额: 52015.9 } }), 52015.9)
  assert.equal(sums[1].value!({ data: { 费用类型: 'ETC' } }), 0)
})

test('buildFleetFilterSpecs：车牌号在最前、日期区间第二、其余文本字段在后', () => {
  const cols = buildFleetColumns(
    [
      { name: '车牌号', data_type: 'text' },
      { name: '费用类型', data_type: 'text' },
      { name: '含税金额', data_type: 'number' },
      { name: '备注', data_type: 'textarea' },
    ],
    plateMap,
  )
  const specs = buildFleetFilterSpecs(cols)
  assert.equal(specs[0].key, 'plate')
  assert.equal(specs[1].key, 'dateRange')
  assert.deepEqual(specs.slice(2).map((s) => s.key), ['field:费用类型'])
  // 备注（长文本）与数值列不做筛选器
  assert.ok(!specs.some((s) => s.key === 'field:备注'))
  assert.ok(!specs.some((s) => s.key === 'field:含税金额'))
})

test('buildFleetFilterSpecs：无车牌号字段时不产出 plate 筛选器', () => {
  const cols = buildFleetColumns([{ name: '轮胎编号', data_type: 'text' }], plateMap)
  const specs = buildFleetFilterSpecs(cols)
  assert.equal(specs[0].key, 'dateRange')
  assert.ok(!specs.some((s) => s.key === 'plate'))
})

test('buildFleetFieldFiltersParam：车牌号走专用 key，其余只取 field: 前缀，空值跳过', () => {
  const param = buildFleetFieldFiltersParam({
    plate: '渝C81022',
    dateRange: ['2026-10-01', '2026-10-31'],
    'field:费用类型': 'ETC通行费',
    'field:入口站/起点': '  ',
  })
  // plate 是专用筛选器 key（不是 `field:车牌号`），漏掉会让「输车牌列表不动」
  assert.equal(param, '车牌号:渝C81022;费用类型:ETC通行费')
  assert.equal(buildFleetFieldFiltersParam({}), '')
  assert.equal(buildFleetFieldFiltersParam({ plate: '  ' }), '')
})
