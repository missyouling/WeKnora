import test from 'node:test'
import assert from 'node:assert/strict'
import {
  cellNum,
  grouped,
  money2,
  num2,
  numOrZero,
  pct,
  rateText,
  round2,
  toNum,
} from './formatNumber'

/**
 * 数字 / 金额 / 比率格式化的唯一出口（第 22 轮 P1-3）。
 *
 * 这组断言的价值不在「函数能不能算」，而在**锁死三件事**：
 *   ① `round2` 必须带 1e-6 浮点补偿 —— 少了它「库区移民基金」就是 788.08 而不是 788.09；
 *   ② 单价 / 比率绝不能套金额格式（0.436 显示成 0.44 就是把单价改了）；
 *   ③ 空值文案按场景分开（`—` / `` / `0`），不能一处 `—` 一处空。
 *
 * 这三条各自都在真机上出过账（第 20/21 轮），且每处都只差 1 分或一格空白。
 */

test('round2：1e-6 浮点补偿必须生效（否则差 1 分）', () => {
  // 126600 × 0.006225 在 IEEE754 里是 788.0849999999999
  assert.equal(round2(126600 * 0.006225), 788.09)
  // 不带补偿会得到 788.08 —— 直接验证原始表达式确实会错
  assert.equal(Math.round(126600 * 0.006225 * 100) / 100, 788.08)
  // 8 位比例乘 44,000 会落在 .675 上（→ 9,343.68），所以后端固定分项
  // 走「原金额 × 工业电量 ÷ 账单电量」的精确除法，而不是 × ratio
  assert.equal(round2(0.21235625 * 44000), 9343.68)
  assert.equal(round2((44000 * 126600) / 596168), 9343.67)
  assert.equal(round2('12.345'), 12.35)
  // 非法值参与运算按 0，不产生 NaN 污染后续累加
  assert.equal(round2(null), 0)
  assert.equal(round2('abc'), 0)
  assert.equal(round2(undefined), 0)
})

test('toNum：非有限值一律 0（供运算，不用于展示）', () => {
  assert.equal(toNum('12.5'), 12.5)
  assert.equal(toNum(''), 0)
  assert.equal(toNum('abc'), 0)
  assert.equal(toNum(Infinity), 0)
})

test('money2：固定两位；非法值显示 —', () => {
  assert.equal(money2(101414.04), '101414.04')
  assert.equal(money2(930), '930.00')
  assert.equal(money2(0), '0.00')
  // null 会被 Number() 折成 0（与收敛前的老实现一致），空值文案交给 cellNum
  assert.equal(money2(null), '0.00')
  assert.equal(money2('abc'), '—')
  assert.equal(money2('1234.5'), '1234.50')
})

test('num2：整数不带小数位，否则两位；非法值显示 —', () => {
  assert.equal(num2(126600), '126600')
  assert.equal(num2(5024.28), '5024.28')
  assert.equal(num2('42462.3'), '42462.30')
  assert.equal(num2(null), '0')
  assert.equal(num2('abc'), '—')
})

test('rateText：最多 6 位、末尾 0 不显示；0 与非法值显示 —', () => {
  assert.equal(rateText(0.436), '0.436')
  assert.equal(rateText(0.2424), '0.2424')
  assert.equal(rateText(0.05757), '0.05757')
  assert.equal(rateText(0.035557), '0.035557')
  // 0 表示「该项不计价」，不是「免费」
  assert.equal(rateText(0), '—')
  assert.equal(rateText(null), '—')
  // 关键：单价绝不能被金额格式化成 0.44
  assert.notEqual(rateText(0.436), '0.44')
})

test('pct：入参是小数，输出两位百分比；0 显示 —', () => {
  assert.equal(pct(0.21235625), '21.24%')
  assert.equal(pct(0.197258), '19.73%')
  assert.equal(pct(0), '—')
  assert.equal(pct(null), '—')
})

test('cellNum：空值不占位（不是 —，也不是 0）', () => {
  assert.equal(cellNum(''), '')
  assert.equal(cellNum(null), '')
  assert.equal(cellNum(undefined), '')
  assert.equal(cellNum('abc'), '')
  assert.equal(cellNum(930), '930')
  assert.equal(cellNum(43316.740000000005), '43316.74')
})

test('numOrZero：空值按 0，整数原样', () => {
  assert.equal(numOrZero(null), '0')
  assert.equal(numOrZero(''), '0')
  assert.equal(numOrZero(930), '930')
  assert.equal(numOrZero(12.345), '12.35')
})

test('grouped：先按分收敛再千分位（抹掉浮点累加残留）', () => {
  assert.equal(grouped(53441.77999999996), '53,441.78')
  assert.equal(grouped(43316.740000000005), '43,316.74')
  assert.equal(grouped(126600), '126,600')
  // 金额固定两位
  assert.equal(grouped(930, { fixed: true }), '930.00')
  // 空值不占位（列表列）
  assert.equal(grouped('', { fallback: '', blankWhenEmpty: true }), '')
  assert.equal(grouped(null, { fallback: '', blankWhenEmpty: true }), '')
  // 默认 0 兜底（合计 / 计数）
  assert.equal(grouped(null), '0')
})

test('同一份数值在三种出口下一致（列表 = 浮窗 = 打印）', () => {
  const v = 101414.0449
  assert.equal(money2(v), '101414.04')
  assert.equal(grouped(v, { fixed: true }), '101,414.04')
  assert.equal(cellNum(v), '101414.04')
  // 比例三处都必须是 21.24%
  assert.equal(pct(0.21235625), '21.24%')
})
