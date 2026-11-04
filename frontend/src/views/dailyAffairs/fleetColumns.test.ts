import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildFleetColumns,
  buildFleetSummaryColumns,
  buildFleetFilterSpecs,
  buildFleetFieldFiltersParam,
  filterableOf,
  buildFleetSummaryViewColumns,
  amountFromForm,
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

test('buildFleetFilterSpecs：车牌号在最前、日期区间第二、仅 filterable 文本字段在后', () => {
  // 第 29 轮：筛选栏从「subs 全量平铺」收敛为「只取 filterable」。
  // 「费用类型」不在旧数据白名单里、且未显式标 filterable → 不进筛选栏；
  // 显式标了 filterable 的才进。这正是修复「29 个输入框占满 4 行」的口径。
  const cols = buildFleetColumns(
    [
      { name: '车牌号', data_type: 'text' },
      { name: '费用类型', data_type: 'text' },
      { name: '票据号码', data_type: 'text', filterable: true },
      { name: '含税金额', data_type: 'number' },
      { name: '备注', data_type: 'textarea' },
    ],
    plateMap,
  )
  const specs = buildFleetFilterSpecs(cols)
  assert.equal(specs[0].key, 'plate')
  assert.equal(specs[1].key, 'dateRange')
  assert.deepEqual(specs.slice(2).map((s) => s.key), ['field:票据号码'])
  // 未标 filterable 的文本字段、备注（长文本）与数值列都不做筛选器
  assert.ok(!specs.some((s) => s.key === 'field:费用类型'))
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

test('filterableOf：显式 filterable 优先，旧数据走白名单，备注永不可筛', () => {
  // 显式 false 覆盖白名单（用户手动关闭）
  assert.equal(filterableOf({ name: '车牌号', filterable: false }), false)
  // 显式 true 覆盖白名单（用户手动开启详情字段）
  assert.equal(filterableOf({ name: '经营地址', filterable: true }), true)
  // 旧数据（无 filterable 键）走白名单
  assert.equal(filterableOf({ name: '车牌号' }), true)
  assert.equal(filterableOf({ name: '证件编号' }), true)
  // 非白名单详情字段默认不可筛 —— 这是筛选栏收敛的关键
  assert.equal(filterableOf({ name: '经营范围' }), false)
  assert.equal(filterableOf({ name: '安装里程' }), false)
  // 备注类长文本永不可筛
  assert.equal(filterableOf({ name: '备注', filterable: true }), false)
})

test('buildFleetFilterSpecs：只产出 filterable 字段，且字段筛选最多 3 个', () => {
  const mk = (name: string, filterable: boolean, dataType = 'text') => ({
    key: name, label: name, dataType, default: true, filterable,
    value: () => '', raw: () => 0,
  })
  const cols = [
    mk('车牌号', true),
    mk('费用类型', true),
    mk('票据号码', true),
    mk('入口站/起点', true),
    mk('出口站/终点', true),
    mk('经营范围', false),
  ]
  const specs = buildFleetFilterSpecs(cols)
  // 车牌号 + 日期区间 + 最多 3 个字段筛选
  assert.equal(specs[0].key, 'plate')
  assert.equal(specs[1].key, 'dateRange')
  const fieldSpecs = specs.filter((s) => s.key.startsWith('field:'))
  assert.equal(fieldSpecs.length, 3)
  assert.deepEqual(fieldSpecs.map((s) => s.key), ['field:费用类型', 'field:票据号码', 'field:入口站/起点'])
})

test('buildFleetSummaryViewColumns：汇总视图列含证照类型/车牌号/编号/日期/状态', () => {
  const cols = buildFleetSummaryViewColumns(new Map([['v1', '渝C81022']]))
  assert.deepEqual(cols.map((c) => c.label), ['证照类型', '车牌号', '持有人/关联对象', '证件编号', '有效期/日期', '状态'])
  // 子分类列取 sub_category，缺失时退回 doc_type
  assert.equal(cols[0].value({ sub_category: '行驶证' }), '行驶证')
  assert.equal(cols[0].value({ doc_type: '道路运输证' }), '道路运输证')
  assert.equal(cols[0].value({}), '—')
  // 车牌号走 data 优先，其次 vehicle_id 映射
  assert.equal(cols[1].value({ data: { 车牌号: '渝A12345' } }), '渝A12345')
  assert.equal(cols[1].value({ vehicle_id: 'v1' }), '渝C81022')
  // 证件编号走候选链（许可证号 / 档案编号 / 保单号 都能取到）
  assert.equal(cols[3].value({ data: { 许可证号: 'XK-001' } }), 'XK-001')
  assert.equal(cols[3].value({ data: { 档案编号: 'DA-001' } }), 'DA-001')
  assert.equal(cols[3].value({ data: { 保单号: 'P-001' } }), 'P-001')
  // 全部缺失时统一 —
  assert.equal(cols[3].value({ data: {} }), '—')
})

test('amountFromForm 语义：单价×数量优先于单价，避免把「2升×88.5」记成 88.5', () => {
  // 这段逻辑曾因「单价」被写进金额候选链而算错（真机 D8：期望 177 实得 88.5），
  // 因此固化为单测：单价必须在「单价 × 数量」分支里单独处理。
  const cases: Array<[Record<string, any>, number]> = [
    // 显式金额字段优先
    [{ '金额': 120 }, 120],
    [{ '含税金额': 55.89 }, 55.89],
    [{ '总费用': 300 }, 300],
    // 只有单价 + 数量 → 相乘
    [{ '单价': 88.5, '数量': 2 }, 177],
    [{ '单价（元）': 12.5, '数量': 4 }, 50],
    // 只有单价（无数量）→ 退回单价
    [{ '单价': 88.5 }, 88.5],
    // 都没有 → 0
    [{}, 0],
    // 金额字段为 0/空 时继续往下一个候选找
    [{ '金额': 0, '单价': 10, '数量': 3 }, 30],
  ]
  for (const [input, want] of cases) {
    assert.equal(amountFromForm(input), want, `input=${JSON.stringify(input)}`)
  }
})
