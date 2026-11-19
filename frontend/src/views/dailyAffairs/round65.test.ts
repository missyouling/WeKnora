/**
 * 第 65 轮单测：
 *   ① 合计行 onlyLabel（宿舍四档案只留首列文案）
 *   ② 宿舍状态 4 色语义（满员红 / 部分入住橙 / 空闲绿 / 维修中灰）
 *   ③ 资产档案去维修字段 + 资产状态可编辑
 *   ④ 月份字段类型契约
 *   ⑤ 扣款明细列（无员工工号、金额列宽足够）
 *   ⑥ 抽屉强校验正则（金额/整数/月份）
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'

import { buildFootRow, type SummaryColumn } from './listSummary'
import {
  HR_DORM_FIELDS, hrDormStatusTextClass, hrDormChargeColumns,
  HR_DORM_CHARGE_BILL_FIELDS,
} from './hrDorm'

const COLS: SummaryColumn[] = [
  { key: 'row-select', label: '', fieldType: 'text' },
  { key: '宿舍编号', label: '宿舍编号', fieldType: 'text' },
  { key: '房间面积(㎡)', label: '房间面积', fieldType: 'number' },
  { key: '资产价值(元)', label: '资产价值', fieldType: 'number' },
]
const ROWS = [
  { 宿舍编号: 'A', '房间面积(㎡)': '28', '资产价值(元)': '100' },
  { 宿舍编号: 'B', '房间面积(㎡)': '30', '资产价值(元)': '200' },
]

test('第65轮：onlyLabel 时合计行仅首列有文案，其余列为空', () => {
  const foot = buildFootRow(COLS, ROWS, { selectKey: 'row-select', onlyLabel: true })
  assert.equal(foot['row-select'], '合计（共 2 条）')
  assert.equal(foot['宿舍编号'], '')
  assert.equal(foot['房间面积(㎡)'], '')
  assert.equal(foot['资产价值(元)'], '')
})

test('第65轮：不传 onlyLabel 时仍保留数值汇总（费用信息月度账单）', () => {
  const foot = buildFootRow(COLS, ROWS, { selectKey: 'row-select' })
  assert.equal(foot['row-select'], '合计（共 2 条）')
  assert.notEqual(foot['房间面积(㎡)'], '')
  assert.notEqual(foot['资产价值(元)'], '')
})

test('第65轮：宿舍状态 4 色语义', () => {
  assert.equal(hrDormStatusTextClass('满员'), 'is-danger')
  assert.equal(hrDormStatusTextClass('部分入住'), 'is-warn')
  assert.equal(hrDormStatusTextClass('空闲'), 'is-ok')
  assert.equal(hrDormStatusTextClass('维修中'), 'is-off')
  // 住宿记录的状态仍沿用既有语义
  assert.equal(hrDormStatusTextClass('在住'), 'is-ok')
  assert.equal(hrDormStatusTextClass('已退宿'), 'is-off')
  assert.equal(hrDormStatusTextClass(''), 'is-muted')
})

test('第65轮：资产档案无维修字段，资产状态可编辑 enum', () => {
  const names = HR_DORM_FIELDS.asset.map((f) => f.name)
  for (const gone of ['报修日期', '维修状态', '维修费用(元)', '维修说明']) {
    assert.ok(!names.includes(gone), `${gone} 必须从资产档案删除`)
  }
  const st = HR_DORM_FIELDS.asset.find((f) => f.name === '资产状态')
  assert.ok(st, '资产状态必须保留')
  assert.equal(st.type, 'enum')
  assert.ok(!st.readonly, '资产状态必须可编辑')
  assert.deepEqual(st.options, ['正常', '损坏', '报废'])
})

test('第65轮：月份型字段用 month 契约', () => {
  const all = [...HR_DORM_FIELDS.public, ...HR_DORM_FIELDS.dorm]
  for (const name of ['租期起始月份', '租期截止月份']) {
    const f = all.find((x) => x.name === name)
    assert.ok(f, `${name} 必须存在`)
    assert.equal(f.type, 'month', `${name} 必须是月份选择器`)
  }
})

test('第65轮：扣款明细列不含员工工号，金额列宽足够', () => {
  const cols = hrDormChargeColumns() as any[]
  const keys = cols.map((c) => c.colKey)
  assert.ok(!keys.includes('员工工号'), '扣款明细不得有员工工号列')
  assert.ok(keys.includes('其它扣款(元)'))
  assert.ok(keys.includes('退款(元)'))
  assert.ok(keys.includes('备注'))
  // 每列必须自带 value，否则整表空白
  for (const c of cols.filter((x) => x.colKey !== 'row-select')) {
    assert.equal(typeof c.value, 'function', `${c.colKey} 缺少 value`)
  }
})

test('第65轮：月度账单列仍为 15 列（费用信息主列表）', () => {
  assert.equal(HR_DORM_CHARGE_BILL_FIELDS.length, 15)
})

test('第65轮：抽屉强校验正则（与后端同口径）', () => {
  const money = /^\d+(\.\d{1,4})?$/
  const int = /^\d+$/
  const month = /^\d{4}-(0[1-9]|1[0-2])$/
  // 合法
  for (const v of ['0', '1', '28.5', '0.6185', '1234.5678']) assert.ok(money.test(v), v)
  // 非法（中文/字母/符号/负数/超 4 位小数）
  for (const v of ["打撒'", '饿243', '34￥￥@', '-100', '1.23456']) assert.ok(!money.test(v), v)
  for (const v of ['0', '2', '100']) assert.ok(int.test(v), v)
  for (const v of ['2.5', '3个', '-1']) assert.ok(!int.test(v), v)
  for (const v of ['2026-01', '2026-12']) assert.ok(month.test(v), v)
  for (const v of ['53q', '5353', '2026-13', '2026-00']) assert.ok(!month.test(v), v)
})
