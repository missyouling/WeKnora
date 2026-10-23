import assert from 'node:assert/strict'
import test from 'node:test'
import { normalizeBillingPeriod } from './billingPeriod'

/**
 * 账单周期归一化（前端展示口径）。
 *
 * 与后端 service.NormalizeBillingPeriod 的用例保持同表：任何一边改了规则，
 * 另一边必须同步，否则「列表显示 2026-08、筛选按 2026-08-01 ~」这类错位就会出现。
 */
const cases: Array<[string, string, string]> = [
  ['中文范围', '2026年8月1日~2026年8月31日', '2026-08'],
  ['中文范围带空格', '2026年8月1日 - 2026年8月31日', '2026-08'],
  ['ISO 范围', '2026-08-01~2026-08-31', '2026-08'],
  ['ISO 范围至', '2026-08-01至2026-08-31', '2026-08'],
  ['点分范围', '2026.08.01-2026.08.31', '2026-08'],
  ['斜杠范围', '2026/08/01 - 2026/08/31', '2026-08'],
  ['单日 ISO', '2026-09-01', '2026-09'],
  ['中文年月', '2026年9月', '2026-09'],
  ['紧凑六位', '202608', '2026-08'],
  ['已归一', '2026-08', '2026-08'],
  ['单位数月补零', '2026-8-1', '2026-08'],
  ['带前缀', '账单周期：2026年8月1日~2026年8月31日', '2026-08'],
  ['空串', '', ''],
  ['无法解析', '本月', ''],
  ['月份越界', '2026年13月', ''],
]

for (const [name, input, want] of cases) {
  test(`normalizeBillingPeriod ${name}`, () => {
    assert.equal(normalizeBillingPeriod(input), want, `${input} → ${want}`)
  })
}

test('normalizeBillingPeriod 幂等', () => {
  for (const input of ['2026-08', '2026年8月1日~2026年8月31日', '202608']) {
    const once = normalizeBillingPeriod(input)
    assert.equal(normalizeBillingPeriod(once), once)
  }
})
