/**
 * 第 64 轮单测：
 *   ① 部门严格按公司（无部门即空数组，禁止回退星达部门）
 *   ② 表计按【用途 + 使用单位】过滤（绝不吃归属单位）
 *   ③ 费用信息月度账单列与合计（金额按分累加）
 *   ④ 宿舍状态纯文字配色
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  departmentsOf, departmentsOfStrict, filterMetersByScopeAndCompany, sameCompany,
  meterKindScope, normalizeCompanyName,
} from './companyScope'
import {
  HR_DORM_CHARGE_BILL_FIELDS, hrDormChargeBillColumns, dormChargeBillTotals,
  hrDormStatusTextClass, hrDormFieldsOf,
} from './hrDorm'

const UNITS = [
  {
    name: '重庆星达铜业有限公司',
    departments: ['人事行政部', '财务部', '机电部', '生产部', '销售部', '仓库', '总经办', '技术质量部'],
  },
  // 持睿在单位设置里未配置任何部门
  { name: '重庆持睿汽车部件有限公司', departments: [] },
]

test('第64轮：所属部门严格按所属公司，持睿无部门时必须为空', () => {
  assert.equal(departmentsOfStrict(UNITS, '重庆星达铜业有限公司').length, 8)
  assert.deepEqual(departmentsOfStrict(UNITS, '重庆持睿汽车部件有限公司'), [])
  assert.deepEqual(departmentsOfStrict(UNITS, ''), [])
  // 简称也能命中同一家公司
  assert.equal(departmentsOfStrict(UNITS, '星达').length, 8)
  // 旧的回退版仍是「全部部门」语义（仅供非宿舍场景），确保两者差异被测试固化
  assert.equal(departmentsOf(UNITS, '重庆持睿汽车部件有限公司').length, 8)
})

test('第64轮：单位名归一与同公司判定', () => {
  const names = UNITS.map((u) => u.name)
  assert.equal(normalizeCompanyName('重庆星达', names), '重庆星达铜业有限公司')
  assert.equal(normalizeCompanyName('持睿汽车', names), '重庆持睿汽车部件有限公司')
  assert.ok(sameCompany('重庆星达', '重庆星达铜业有限公司', names))
  assert.ok(!sameCompany('重庆星达', '重庆持睿汽车部件有限公司', names))
})

const KINDS = [
  { value: 'dorm', label: '宿舍' },
  { value: 'production', label: '工商业' },
  { value: 'custom1', label: '公租房' },
]

const METERS = [
  // 持睿宿舍表：归属单位是星达，使用单位是持睿 —— 本轮必须按使用单位命中持睿
  { alias: '持睿403', meter_kind: 'dorm', use_unit: '重庆持睿汽车部件有限公司', owner_unit: '重庆星达铜业有限公司', enabled: true },
  { alias: '持睿404', meter_kind: 'dorm', use_unit: '重庆持睿汽车部件有限公司', owner_unit: '重庆星达铜业有限公司', enabled: true },
  { alias: '持睿405', meter_kind: 'dorm', use_unit: '重庆持睿汽车部件有限公司', owner_unit: '重庆星达铜业有限公司', enabled: true },
  { alias: '宿舍301', meter_kind: 'dorm', use_unit: '重庆星达', owner_unit: '重庆星达', enabled: true },
  { alias: '宿舍302', meter_kind: 'dorm', use_unit: '重庆星达', owner_unit: '重庆星达', enabled: true },
  { alias: '工商业表', meter_kind: 'production', use_unit: '重庆星达', enabled: true },
  { alias: '两山秀苑19-23-5', meter_kind: 'custom1', use_unit: '重庆星达', enabled: true },
]

test('第64轮：宿舍表计按用途+使用单位过滤，持睿与星达互不串台', () => {
  const names = UNITS.map((u) => u.name)
  const chuirui = filterMetersByScopeAndCompany(METERS, KINDS, 'dorm', '重庆持睿汽车部件有限公司', names)
    .map((m) => m.alias)
  assert.deepEqual(chuirui, ['持睿403', '持睿404', '持睿405'])
  assert.ok(!chuirui.includes('宿舍301'))

  const xinda = filterMetersByScopeAndCompany(METERS, KINDS, 'dorm', '重庆星达铜业有限公司', names)
    .map((m) => m.alias)
  assert.deepEqual(xinda, ['宿舍301', '宿舍302'])
  assert.ok(!xinda.some((v) => String(v).startsWith('持睿')))
  // 工商业表绝不进入宿舍档案
  assert.ok(!xinda.includes('工商业表'))

  // 公租房用途只出公租房表
  const pub = filterMetersByScopeAndCompany(METERS, KINDS, 'public', '重庆星达铜业有限公司', names)
    .map((m) => m.alias)
  assert.deepEqual(pub, ['两山秀苑19-23-5'])
})

test('第64轮：表计缺使用单位时不得被任何公司命中', () => {
  const orphan = [{ alias: '无单位表', meter_kind: 'dorm', owner_unit: '重庆星达铜业有限公司', enabled: true }]
  assert.deepEqual(filterMetersByScopeAndCompany(orphan, KINDS, 'dorm', '重庆星达铜业有限公司', []), [])
})

test('第64轮：用途归类', () => {
  assert.equal(meterKindScope('宿舍'), 'dorm')
  assert.equal(meterKindScope('公租房'), 'public')
  assert.equal(meterKindScope('工商业'), '')
  assert.equal(meterKindScope(''), '')
})

test('第64轮：费用信息月度账单列 = 15 项（含账单月份与账单状态）', () => {
  assert.equal(HR_DORM_CHARGE_BILL_FIELDS.length, 15)
  const cols = hrDormChargeBillColumns() as any[]
  // 首列勾选 + 15 个业务列
  assert.equal(cols.length, 16)
  // 每列必须自带 value，否则整表空白、合计为 0
  for (const c of cols.slice(1)) assert.equal(typeof c.value, 'function', `${c.colKey} 缺少 value`)
  assert.ok(HR_DORM_CHARGE_BILL_FIELDS.some((f) => f.name === '扣款总计'))
  assert.ok(HR_DORM_CHARGE_BILL_FIELDS.some((f) => f.name === '住宿人数'))
})

test('第64轮：月度账单合计按分累加，整数列不带小数', () => {
  const rows = [
    { 总人数: 2, 住宿人数: 1, 押金合计: '100.10', 房租合计: '0.20', 扣款总计: '1,682.47' },
    { 总人数: 3, 住宿人数: 2, 押金合计: '0.20', 房租合计: '1200.00', 扣款总计: '1500.03' },
  ]
  const foot = dormChargeBillTotals(rows)
  assert.equal(foot['账单月份'], '合计')
  assert.equal(foot['总人数'], '5')
  assert.equal(foot['住宿人数'], '3')
  assert.equal(foot['押金合计'], '100.30')
  assert.equal(foot['房租合计'], '1,200.20')
  assert.equal(foot['扣款总计'], '3,182.50')
})

test('第64轮：宿舍状态纯文字配色（无框）', () => {
  assert.equal(hrDormStatusTextClass('空闲'), 'is-ok')
  assert.equal(hrDormStatusTextClass('在住'), 'is-ok')
  assert.equal(hrDormStatusTextClass('部分入住'), 'is-warn')
  assert.equal(hrDormStatusTextClass('满员'), 'is-off')
  assert.equal(hrDormStatusTextClass('已退宿'), 'is-off')
  assert.equal(hrDormStatusTextClass(''), 'is-muted')
})

test('第64轮：宿舍字段契约可按视图取用（重置兜底基准）', () => {
  assert.ok(hrDormFieldsOf('dorm').length > 20)
  assert.ok(hrDormFieldsOf('public').length >= hrDormFieldsOf('dorm').length)
  assert.deepEqual(hrDormFieldsOf('fee').map((f) => f.name)[0], '账单月份')
})
