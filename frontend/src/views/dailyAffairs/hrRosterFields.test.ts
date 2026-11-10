import test from 'node:test'
import assert from 'node:assert/strict'
import {
  ROSTER_TEMPLATE_HEADERS, ROSTER_FIELD_GROUPS, ROSTER_ENUM_GROUPS,
  RESIGN_REASONS, ROSTER_STATUS_OPTIONS, ROSTER_STATUS_COLUMNS,
  ROSTER_DERIVED_FIELDS, EXTRA_RESIGN_FIELDS,
  normalizeRosterStatus, isRosterDateField,
} from './hrRosterFields'

test('76 个标准表头，顺序与模板一致（首 工号 / 末 现合同签订日期）', () => {
  assert.equal(ROSTER_TEMPLATE_HEADERS.length, 76)
  assert.equal(ROSTER_TEMPLATE_HEADERS[0], '工号')
  assert.equal(ROSTER_TEMPLATE_HEADERS[1], '姓名')
  assert.equal(ROSTER_TEMPLATE_HEADERS[75], '现合同签订日期')
  assert.equal(new Set(ROSTER_TEMPLATE_HEADERS).size, 76)
  for (const h of ['部门', '员工状态', '证件号码', '试用期', '离职原因', '个人社保账号', '现合同类型']) {
    assert.ok(ROSTER_TEMPLATE_HEADERS.includes(h), `缺少表头 ${h}`)
  }
})

test('说明 Sheet 11 大分组，字段无重复且都在 76 表头内', () => {
  assert.equal(ROSTER_FIELD_GROUPS.length, 11)
  const seen = new Set<string>()
  for (const g of ROSTER_FIELD_GROUPS) {
    assert.ok(g.fields.length > 0, `${g.group} 不应为空`)
    for (const f of g.fields) {
      assert.ok(ROSTER_TEMPLATE_HEADERS.includes(f), `${g.group}/${f} 不在 76 表头内`)
      assert.ok(!seen.has(f), `${f} 重复分组`)
      seen.add(f)
    }
  }
  assert.equal(seen.size, 76, '11 个分组应恰好覆盖 76 表头')
})

test('枚举字段 20 组，离职原因恰 42 项', () => {
  assert.equal(Object.keys(ROSTER_ENUM_GROUPS).length, 20)
  assert.equal(ROSTER_ENUM_GROUPS['离职原因'].length, 42)
  assert.ok(ROSTER_ENUM_GROUPS['工作性质'].includes('退休返聘'))
  assert.equal(RESIGN_REASONS.reduce((n, g) => n + g.options.length, 0), 42)
  assert.deepEqual(RESIGN_REASONS.map((g) => g.group),
    ['主动离职', '被动离职', '退休', '非正常离职', '自动离职'])
  assert.ok(RESIGN_REASONS[0].options.includes('家庭原因'))
  assert.ok(RESIGN_REASONS[4].options.includes('连续旷工'))
})

test('在职状态下拉含 4 态 + 全部', () => {
  assert.deepEqual(ROSTER_STATUS_OPTIONS.map((o) => o.value),
    ['全部', '在职', '试用期', '退休返聘', '离职'])
})

test('四态动态列：试用期含 试用期/转正日期，离职含 4 个离职专属列', () => {
  const trial = ROSTER_STATUS_COLUMNS['试用期']
  assert.ok(trial.includes('试用期'))
  assert.ok(trial.includes('转正日期'))
  assert.ok(!trial.includes('离职日期'))
  const left = ROSTER_STATUS_COLUMNS['离职']
  for (const h of ['离职日期', '离职原因', '离职证明', '离职备注']) {
    assert.ok(left.includes(h), `离职视图缺少 ${h}`)
  }
  assert.ok(!left.includes('试用期'))
  assert.ok(!left.includes('个人社保账号'))
  const active = ROSTER_STATUS_COLUMNS['在职']
  assert.ok(active.includes('个人社保账号'))
  assert.ok(active.includes('工龄'))
  const reemp = ROSTER_STATUS_COLUMNS['退休返聘']
  assert.ok(reemp.includes('现合同类型'))
  assert.ok(reemp.includes('紧急联系人电话'))
})

test('四态列字段都在 76 表头 / 派生 / 离职扩展名单内', () => {
  const valid = new Set([...ROSTER_TEMPLATE_HEADERS, ...ROSTER_DERIVED_FIELDS, ...EXTRA_RESIGN_FIELDS])
  for (const [view, cols] of Object.entries(ROSTER_STATUS_COLUMNS)) {
    const bad = cols.filter((c) => !valid.has(c))
    assert.deepEqual(bad, [], `${view} 有非法列 ${bad.join(',')}`)
  }
})

test('normalizeRosterStatus：模板值 / 旧值 / 工作性质优先', () => {
  assert.equal(normalizeRosterStatus('正式'), '在职')
  assert.equal(normalizeRosterStatus('active'), '在职')
  assert.equal(normalizeRosterStatus('试用'), '试用期')
  assert.equal(normalizeRosterStatus('已离职'), '离职')
  assert.equal(normalizeRosterStatus('待离职'), '离职')
  assert.equal(normalizeRosterStatus('resigned'), '离职')
  assert.equal(normalizeRosterStatus('正式', '退休返聘'), '退休返聘')
  assert.equal(normalizeRosterStatus(undefined), '全部')
  assert.equal(normalizeRosterStatus('未知状态'), '全部')
})

test('isRosterDateField 覆盖日期 / 时间 / 有效期', () => {
  assert.ok(isRosterDateField('入职日期'))
  assert.ok(isRosterDateField('现合同签订日期'))
  assert.ok(isRosterDateField('教育开始时间'))
  assert.ok(isRosterDateField('身份证有效期'))
  assert.ok(!isRosterDateField('工号'))
})
