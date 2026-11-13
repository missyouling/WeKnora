/**
 * 第 56 轮纯函数回归：社保四标签 ↔ 四胶囊契约 / 状态锁 / KPI 联动提醒 / 列定义。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildHrKpiCards, hrSocialColumnsOf } from './hrCore'
import {
  HR_SOCI_GROUPS, HR_SOCI_VIEW_GROUPS, hrSocialGroupOf,
  HR_SOCI_RECEIPT_GROUP, HR_SOCI_NORMAL_GROUP, HR_SOCI_ABNORMAL_GROUP, HR_SOCI_BILL_GROUP,
  HR_SOCI_RECEIPT_FIELDS, HR_SOCI_NORMAL_FIELDS, HR_SOCI_ABNORMAL_FIELDS, HR_SOCI_BILL_FIELDS,
  HR_SOCI_DEFAULT_HEADERS, hrModuleGroups, hrModuleFields, hrTemplateHeaders,
} from './hrFieldSchema'
import { statusFieldSpecsOf, isHrStatusName, isLockedStatusField } from './statusFields'

test('四胶囊分组顺序 = 页面四标签顺序（回盘表 / 正常记录 / 异常记录 / 月度账单）', () => {
  assert.deepEqual(HR_SOCI_VIEW_GROUPS, ['回盘表', '正常记录', '异常记录', '月度账单'])
  assert.deepEqual(HR_SOCI_GROUPS.map((g) => g.key), HR_SOCI_VIEW_GROUPS)
  assert.deepEqual(HR_SOCI_GROUPS.map((g) => g.label), HR_SOCI_VIEW_GROUPS)
  assert.deepEqual(hrModuleGroups('social').map((g) => g.key), HR_SOCI_VIEW_GROUPS)
  // 四组严格独立：正常/异常字段名相同但分属两个胶囊
  assert.deepEqual(HR_SOCI_NORMAL_FIELDS.map((f) => f.name), HR_SOCI_ABNORMAL_FIELDS.map((f) => f.name))
  assert.equal(HR_SOCI_NORMAL_FIELDS[0].group, HR_SOCI_NORMAL_GROUP)
  assert.equal(HR_SOCI_ABNORMAL_FIELDS[0].group, HR_SOCI_ABNORMAL_GROUP)
})

test('四胶囊字段数：回盘表 28（27 业务 + 部门引用）/ 正常 16 / 异常 16 / 账单 16', () => {
  assert.equal(HR_SOCI_RECEIPT_FIELDS.length, 28)
  assert.equal(HR_SOCI_NORMAL_FIELDS.length, 16)
  assert.equal(HR_SOCI_ABNORMAL_FIELDS.length, 16)
  assert.equal(HR_SOCI_BILL_FIELDS.length, 16)
  assert.equal(hrModuleFields('social').length, 76)
  // 部门是跨表引用，其余 27 个是回盘表原生字段
  assert.equal(HR_SOCI_RECEIPT_FIELDS.filter((f) => !!f.source).length, 1)
  assert.equal(hrTemplateHeaders('social').length, 27)
  assert.equal(hrTemplateHeaders('social').includes('部门'), false)
})

test('回盘表默认表头：个人编号 / 姓名 / 身份证号 / 部门 / 五险参保状态开，证件种类 / 民族关', () => {
  const d = HR_SOCI_DEFAULT_HEADERS[HR_SOCI_RECEIPT_GROUP]
  for (const name of ['个人编号', '姓名', '身份证号', '部门', '性别', '户口性质',
    '养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态',
    '养老缴费基数', '个人身份']) {
    assert.ok(d.includes(name), `${name} 应默认开启`)
  }
  for (const name of ['证件种类', '民族', '出生日期', '工作日期', '备注', '工资']) {
    assert.equal(d.includes(name), false, `${name} 应默认关闭`)
  }
  // 五险参保状态一律系统内置加锁
  for (const name of ['养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态']) {
    const f = HR_SOCI_RECEIPT_FIELDS.find((x) => x.name === name)!
    assert.equal(f.type, 'system')
    assert.equal(f.builtin, true)
    assert.deepEqual(f.options, ['参保缴费', '暂停缴费', '终止缴费'])
  }
})

test('月度账单 16 字段含 12 个金额列 + 账单状态 + 生成时间', () => {
  const d = HR_SOCI_DEFAULT_HEADERS[HR_SOCI_BILL_GROUP]
  for (const name of ['账单周期', '参保人数', '缴费基数合计', '单位养老', '单位医疗+生育',
    '单位工伤', '单位失业', '社保单位金额', '个人养老', '个人医疗', '个人大额医疗',
    '个人失业', '社保个人金额', '应缴总计', '账单状态', '生成时间']) {
    assert.ok(d.includes(name), `${name} 应默认开启`)
  }
  assert.equal(d.length, 16)
})

test('社保状态字段加锁，不再注入第 45 轮的两个幽灵状态列', () => {
  for (const name of ['养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态']) {
    assert.equal(isHrStatusName(name), true)
    assert.equal(isLockedStatusField('hr-social', '社保管理', name), true)
  }
  assert.equal(isHrStatusName('个人编号'), false)
  assert.deepEqual(statusFieldSpecsOf('hr-social'), [])
})

test('子视图 → 胶囊分组映射唯一（六同步）', () => {
  assert.equal(hrSocialGroupOf('receipt'), HR_SOCI_RECEIPT_GROUP)
  assert.equal(hrSocialGroupOf('normal'), HR_SOCI_NORMAL_GROUP)
  assert.equal(hrSocialGroupOf('abnormal'), HR_SOCI_ABNORMAL_GROUP)
  assert.equal(hrSocialGroupOf('bills'), HR_SOCI_BILL_GROUP)
  assert.equal(hrSocialGroupOf('unknown'), HR_SOCI_BILL_GROUP)
})

test('四标签列全集：无冻结列、无操作列，状态列走具名插槽，比例列不进合计', () => {
  for (const [view, n] of [['receipt', 29], ['normal', 17], ['abnormal', 17], ['bills', 17]] as const) {
    const cols = hrSocialColumnsOf(view)
    assert.equal(cols.length, n, `${view} 列数（含勾选列）`)
    assert.equal(cols[0].colKey, 'row-select')
    for (const c of cols) {
      assert.equal((c as any).fixed, undefined, `${view}/${c.colKey} 禁止冻结列`)
    }
  }
  const receipt = hrSocialColumnsOf('receipt')
  for (const name of ['养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态']) {
    assert.equal(receipt.find((c) => c.colKey === name)!.slot, true)
  }
  assert.equal(hrSocialColumnsOf('bills').find((c) => c.colKey === '账单状态')!.slot, true)
  assert.equal(hrSocialColumnsOf('normal').find((c) => c.colKey === '单位缴费比例')!.noSum, true)
  // 号码类列必须按 text 渲染，绝不参与合计行累加
  assert.equal(receipt.find((c) => c.colKey === '个人编号')!.dataType, 'text')
  assert.equal(receipt.find((c) => c.colKey === '身份证号')!.dataType, 'text')
})

test('社保卡片主数字=月度账单条数，副文案「X 人待补录社保 · Y 人离职待减员」', () => {
  const cards = buildHrKpiCards({
    'hr-social': 1, social_pending: 6, social_to_seal: 3,
  })
  const c = cards.find((x) => x.key === 'hr-social')!
  assert.equal(c!.value, '1')
  assert.equal(c!.unit, '条')
  assert.equal(c!.sub, '6 人待补录社保 · 3 人离职待减员')
  assert.equal(c!.subCls, 'is-warning')
  const only = buildHrKpiCards({ 'hr-social': 2, social_to_seal: 2 }).find((x) => x.key === 'hr-social')!
  assert.equal(only!.sub, '2 人离职待减员')
  const none = buildHrKpiCards({ 'hr-social': 0 }).find((x) => x.key === 'hr-social')!
  assert.equal(none!.sub, '')
})
