/**
 * 第 63 轮前端回归：
 *   · 单位名称简称/全称归一（重庆星达 ↔ 重庆星达铜业有限公司）；
 *   · 表计按【用途 + 使用单位】双重过滤（宿舍 / 公租房）；
 *   · 花名册签约公司改名与列配置兼容；
 *   · 离职抽屉全分组、账单弹窗无横向滚动条、抽屉 Label 不截断。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  normalizeCompanyName, sameCompany, companyNamesOf, departmentsOf,
  meterKindScope, filterMetersByScopeAndCompany,
} from './companyScope'
import { HR_DORM_FIELDS, hrDormBedsOfType, hrDormDrawerGroups } from './hrDorm'
import { HR_ROSTER_FIELDS } from './hrFieldSchema'

const readLocal = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const read = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

const UNITS = [
  { name: '重庆星达铜业有限公司', departments: [{ name: '生产部' }, { name: '财务部' }] },
  { name: '重庆持睿汽车部件有限公司', departments: [{ name: '装配车间' }] },
]
const NAMES = companyNamesOf(UNITS)

test('单位名称归一：简称 / 全称 / 空值', () => {
  assert.equal(normalizeCompanyName('重庆星达', NAMES), '重庆星达铜业有限公司')
  assert.equal(normalizeCompanyName('星达', NAMES), '重庆星达铜业有限公司')
  assert.equal(normalizeCompanyName('持睿汽车', NAMES), '重庆持睿汽车部件有限公司')
  assert.equal(normalizeCompanyName('重庆星达铜业有限公司', NAMES), '重庆星达铜业有限公司')
  assert.equal(normalizeCompanyName('', NAMES), '')
  assert.equal(normalizeCompanyName('未知公司', NAMES), '未知公司')
})

test('sameCompany：双向等价', () => {
  assert.equal(sameCompany('重庆星达', '重庆星达铜业有限公司', NAMES), true)
  assert.equal(sameCompany('重庆星达铜业有限公司', '重庆星达', NAMES), true)
  assert.equal(sameCompany('重庆星达', '重庆持睿汽车部件有限公司', NAMES), false)
  assert.equal(sameCompany('', '重庆星达', NAMES), false)
})

test('departmentsOf：按公司取部门，公司无部门时回退全部部门', () => {
  assert.deepEqual(departmentsOf(UNITS, '重庆星达铜业有限公司'), ['生产部', '财务部'])
  assert.deepEqual(departmentsOf(UNITS, '重庆星达'), ['生产部', '财务部'])
  assert.deepEqual(departmentsOf(UNITS, '重庆持睿汽车部件有限公司'), ['装配车间'])
  assert.deepEqual(departmentsOf([
    { name: 'A公司' }, { name: 'B公司', departments: [{ name: 'B1' }] },
  ], 'A公司'), ['B1'])
})

test('用途归类：宿舍 / 公租房 / 工商业', () => {
  assert.equal(meterKindScope('宿舍'), 'dorm')
  assert.equal(meterKindScope('员工宿舍'), 'dorm')
  assert.equal(meterKindScope('公租房'), 'public')
  assert.equal(meterKindScope('工商业'), '')
  assert.equal(meterKindScope(''), '')
})

const KINDS = [
  { value: 'dorm', label: '宿舍' },
  { value: 'production', label: '工商业' },
  { value: 'custom1', label: '公租房' },
]
const METERS = [
  { alias: '宿舍301电表', meter_kind: 'dorm', use_unit: '重庆星达' },
  { alias: '宿舍302电表', meter_kind: 'dorm', use_unit: '重庆星达铜业有限公司' },
  { alias: '持睿宿舍电表', meter_kind: 'dorm', use_unit: '持睿汽车' },
  { alias: '工商业电表', meter_kind: 'production', use_unit: '重庆星达' },
  { alias: '公租房电表', meter_kind: 'custom1', use_unit: '重庆星达' },
  { alias: '停用电表', meter_kind: 'dorm', use_unit: '重庆星达', enabled: false },
]

test('表计过滤（宿舍）：仅 用途=宿舍 且 使用单位=本公司', () => {
  const got = filterMetersByScopeAndCompany(METERS, KINDS, 'dorm', '重庆星达铜业有限公司', NAMES)
  assert.deepEqual(got.map((m) => m.alias), ['宿舍301电表', '宿舍302电表'])
})

test('表计过滤（公租房）：仅 用途=公租房 且 使用单位=本公司', () => {
  const got = filterMetersByScopeAndCompany(METERS, KINDS, 'public', '重庆星达', NAMES)
  assert.deepEqual(got.map((m) => m.alias), ['公租房电表'])
})

test('表计过滤：未选公司时不按单位过滤，但仍排除工商业与停用表', () => {
  const got = filterMetersByScopeAndCompany(METERS, KINDS, 'dorm', '', NAMES)
  assert.deepEqual(got.map((m) => m.alias), ['宿舍301电表', '宿舍302电表', '持睿宿舍电表'])
})

test('花名册：合同公司已改名签约公司，字段仍在在职信息组', () => {
  const names = HR_ROSTER_FIELDS.map((d) => d.name)
  assert.ok(names.includes('签约公司'))
  assert.equal(names.includes('合同公司'), false)
  assert.equal(HR_ROSTER_FIELDS.find((d) => d.name === '签约公司')?.dynamic, 'unit')
})

test('宿舍字段：所属公司/所属部门拆分，双人间派生 2 床', () => {
  const dorm = HR_DORM_FIELDS.dorm.map((f) => f.name)
  assert.ok(dorm.includes('所属公司'))
  assert.ok(dorm.includes('所属部门'))
  assert.equal(dorm.includes('所属公司/部门'), false)
  assert.equal(hrDormBedsOfType('双人间'), 2)
  const groups = hrDormDrawerGroups('dorm')
  assert.equal(groups.length, 2)
  assert.ok(groups[0].fields.some((f) => f.name === '所属公司'))
})

test('离职抽屉：渲染花名册全部启用分组 + 离职信息', () => {
  const src = readLocal('HrManagement.vue')
  // 第 68 轮：离职抽屉由「离职人员」胶囊驱动（在职字段已并入该胶囊，默认关闭）
  assert.match(src, /const subs = subsOfTab\('hr-roster'\)\.filter/)
  assert.match(src, /resignNames\.has\(name\) \? '离职信息'/)
  assert.match(src, /HR_ROSTER_GROUPS\.flatMap\(\(g\) => g\.fields\)/)
})

test('账单弹窗：1360px + 三段表百分比列宽（无横向滚动条）', () => {
  const src = readLocal('HrManagement.vue')
  assert.match(src, /width="min\(94vw, 1360px\)"/)
  const table = readLocal('../../components/business/DetailScrollTable.vue')
  assert.match(table, /width: '100%'/)
  assert.match(table, /function pctOf/)
  assert.ok(!/minWidth: '100%'/.test(table), 'minWidth:100% 会让列宽之和超容器时溢出')
})

test('抽屉：紧凑同行排版 + 长字段名按需换行（第 69 轮）', () => {
  const src = readLocal('BusinessSplitDetailDrawer.vue')
  // 第 69 轮：短标签同行（label 84px 右对齐），长标签单独换行；
  // label-align="top" 仅作 TDesign 语义锚点，布局由容器内 CSS 接管。
  assert.match(src, /label-align="top"/)
  assert.ok(!/label-width="`\$\{labelWidth\}px`"/.test(src))
  assert.match(src, /const LONG_LABEL_MIN = 7/)
  assert.match(src, /'bsd-field--long-label': isLongLabel\(f\)/)
  // 长字段白名单化（保留真正需要整行的地址/说明类）
  assert.match(src, /const FULL_WIDTH_FIELDS = new Set\(\[[\s\S]{0,400}'配套设施'/)
  assert.match(src, /t-form__label[\s\S]{0,200}font-size: 13px/)
  // 第 64 轮：状态字段由 Tag 改为纯带色文字，但仍必须无边框
  assert.match(src, /\.bsd-status-text \{[\s\S]{0,200}border: 0 !important/)
})

test('表计设置：归属/使用单位改下拉并做名称归一', () => {
  const src = readLocal('MeterSettingsPanel.vue')
  assert.equal((src.match(/<t-select v-model="meterForm\.owner_unit"/g) || []).length, 2)
  assert.equal((src.match(/<t-select v-model="meterForm\.use_unit"/g) || []).length, 2)
  assert.match(src, /normalizeCompanyName\(meterForm\.value\.owner_unit/)
  assert.match(src, /normalizeCompanyName\(meterForm\.value\.use_unit/)
})

test('列表状态 Tag 无黑框（border: 0）', () => {
  const src = readLocal('HrManagement.vue')
  assert.match(src, /\.hr-table :deep\(\.t-tag\) \{[\s\S]{0,60}border: 0 !important/)
})
