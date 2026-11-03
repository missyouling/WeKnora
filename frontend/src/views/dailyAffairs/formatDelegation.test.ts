import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatAmount, formatRate } from './businessFormat'
import { grouped, ratePct } from './formatNumber'

// 第 26 轮 O-2：格式化收敛到 formatNumber 单一出口。
//
// businessFormat 保留原有对外签名（发票/制度/奖惩/浮窗都在用），但实现改为
// 委托 formatNumber —— 否则「合同页自己写一份」这类漂移会反复发生。
//
// 这组测试锁的是**新旧输出完全一致**：迁移不能改变任何已上屏的数值文案。

test('O-2a：formatAmount 必须等于 grouped(fixed+blankWhenEmpty)', () => {
  const cases: any[] = [
    1234.5, 0, -513.35, 51989.49, '50724.26', '', null, undefined,
    'abc', NaN, 53441.77999999996, 43316.740000000005, '1,234.56',
  ]
  for (const v of cases) {
    assert.equal(
      formatAmount(v as any),
      grouped(v, { fixed: true, fallback: '', blankWhenEmpty: true }),
      `formatAmount(${JSON.stringify(v)}) 与 grouped 不一致`,
    )
  }
})

test('O-2b：formatAmount 保留千分位与两位小数（不能退化成 money2）', () => {
  assert.equal(formatAmount(51989.49), '51,989.49', '必须有千分位')
  assert.equal(formatAmount(0), '0.00', '0 不能变成 —')
  assert.equal(formatAmount(''), '', '空值必须是空串（不是 —）')
})

test('O-2c：ratePct 与既有 formatRate 输出完全一致', () => {
  const cases: any[] = [
    0.09, '0.09', 9, '9%', 0.03, 3, '13%', 0, '', null, undefined, 'abc', 0.095,
  ]
  for (const v of cases) {
    assert.equal(ratePct(v), formatRate(v), `ratePct(${JSON.stringify(v)}) 与 formatRate 不一致`)
  }
})

test('O-2d：ratePct 绝不重复 ×100（历史 bug：0.09 → 900%）', () => {
  assert.equal(ratePct(0.09), '9%')
  assert.equal(ratePct('9%'), '9%')
  assert.equal(ratePct(9), '9%')
  assert.equal(ratePct(0.095), '10%', '四舍五入到整数百分比')
})

test('O-2e：businessFormat 不得再自带实现（必须是委托）', async () => {
  const { readFileSync } = await import('node:fs')
  const src = readFileSync(new URL('./businessFormat.ts', import.meta.url), 'utf8')
  assert.match(src, /from '\.\/formatNumber'/, '必须 import formatNumber')
  assert.doesNotMatch(src, /toLocaleString/, '不得再自带千分位实现')
  assert.doesNotMatch(src, /Math\.round\(n >= 1 \? n : n \* 100\)/, '税率逻辑必须移到 formatNumber')
})
