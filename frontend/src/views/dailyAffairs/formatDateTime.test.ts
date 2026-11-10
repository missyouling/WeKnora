import test from 'node:test'
import assert from 'node:assert/strict'
import { formatDate, formatDateTime, formatCellDateTime } from './formatDateTime'

const ISO = '2026-11-07T20:12:33.37663+08:00'

test('formatDateTime：ISO8601 带微秒与时区 → YYYY-MM-DD HH:mm:ss', () => {
  assert.equal(formatDateTime(ISO), '2026-11-07 20:12:33')
})

test('formatDateTime：空格分隔的时间戳同样归一', () => {
  assert.equal(formatDateTime('2018-07-01 00:00:00'), '2018-07-01 00:00:00')
})

test('formatDate：日期时间戳 → YYYY-MM-DD', () => {
  assert.equal(formatDate('2018-07-01 00:00:00'), '2018-07-01')
  assert.equal(formatDate(ISO), '2026-11-07')
})

test('formatDate：单位数月日补零、斜杠格式', () => {
  assert.equal(formatDate('2026/7/9'), '2026-07-09')
  assert.equal(formatDate('2026-7-9 09:05:00'), '2026-07-09')
})

test('空值与非法值返回空串，绝不回显原串', () => {
  for (const v of [null, undefined, '', '   ', 'not-a-date']) {
    assert.equal(formatDateTime(v), '')
    assert.equal(formatDate(v), '')
  }
  assert.equal(formatDateTime('？？？'), '')
})

test('任何输出都不含 T / 微秒小数 / +08:00', () => {
  for (const out of [formatDateTime(ISO), formatDate(ISO), formatCellDateTime('创建时间', ISO), formatCellDateTime('入职日期', ISO)]) {
    assert.ok(!/\d{4}-\d{2}-\d{2}T/.test(out), out)
    assert.ok(!/\.\d{5,}/.test(out), out)
    assert.ok(!/[+-]\d{2}:\d{2}/.test(out), out)
  }
})

test('formatCellDateTime：按列名分派', () => {
  assert.equal(formatCellDateTime('创建时间', ISO), '2026-11-07 20:12:33')
  assert.equal(formatCellDateTime('生成时间', ISO), '2026-11-07 20:12:33')
  assert.equal(formatCellDateTime('封存时间', ISO), '2026-11-07 20:12:33')
  assert.equal(formatCellDateTime('入职日期', '2018-07-01 00:00:00'), '2018-07-01')
  assert.equal(formatCellDateTime('离职日期', ISO), '2026-11-07')
  assert.equal(formatCellDateTime('普通列', 'abc'), 'abc')
  assert.equal(formatCellDateTime('创建时间', undefined), '')
})
