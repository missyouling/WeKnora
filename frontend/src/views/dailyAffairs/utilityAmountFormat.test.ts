import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

/**
 * 市电账单（utilities）金额列的浮点脏值守卫。
 *
 * 真机踩过：列表「输配电费」列显示 53441.77999999996、「系统运行费」列显示
 * 43316.740000000005。根因是 mapRow 里 fee_items 逐项累加用的是裸浮点，
 * 而该列由 TDesign 默认单元格渲染（不经任何格式化），脏值原样上屏。
 *
 * 两道防线都必须存在：
 *   1) 聚合期按「分」整数累加 → 从源头不产生误差；
 *   2) 渲染期 amount 列走 fmtAmount → 万一将来又有人加新聚合点漏了收敛，
 *      也不会把 17 位小数甩到用户脸上。
 */
const src = readFileSync(new URL('./UtilitiesManagement.vue', import.meta.url), 'utf8')

/** 取源码里某函数的函数体（兼容 `const NAME = ...` 与 `function NAME(...)` 两种写法） */
function bodyOf(name: string): string {
  let start = src.indexOf(`const ${name} = `)
  if (start < 0) start = src.indexOf(`function ${name}(`)
  assert.ok(start >= 0, `未找到 ${name}`)
  const rest = src.slice(start + 1)
  const nextTop = rest.search(/\nconst |\nfunction |\n\/\*\*|\n\/\/ ----/)
  return nextTop > 0 ? rest.slice(0, nextTop) : rest
}

test('费用分类聚合必须按分整数累加，不得裸浮点相加', () => {
  const body = bodyOf('mapRow')
  // 必须有按分取整的累加入口
  assert.match(body, /Math\.round\(\s*fee \* 100\s*\)/, '聚合时必须把元换算成分再取整')
  assert.match(body, /\/ 100/, '聚合结束必须折回元')
  // 不得残留裸浮点累加写法（feeAgg.xxx = (feeAgg.xxx || 0) + fee）
  assert.doesNotMatch(
    body,
    /feeAgg\.\w+ = \(feeAgg\.\w+ \|\| 0\) \+ fee/,
    '不得用裸浮点累加金额（会产生 53441.77999999996 这类脏值）',
  )
})

test('amount 列必须挂单元格渲染，不能交给 TDesign 默认渲染', () => {
  const body = bodyOf('tableColumns')
  assert.match(body, /cell: cellOf\(c\)/, 'amount 列必须显式指定 cell 渲染器')
})

test('fmtAmount 必须先按分收敛再千分位', () => {
  const body = bodyOf('fmtAmount')
  assert.match(body, /Math\.round\(n \* 100\) \/ 100/, 'fmtAmount 必须先收敛到分')
  assert.match(body, /toLocaleString\('zh-CN'/, 'fmtAmount 必须用 zh-CN 千分位')
  assert.match(body, /Number\.isFinite/, 'fmtAmount 必须挡掉 NaN/Infinity')
})

test('cellOf 仅对 amount 列生效，其它类型保持默认渲染', () => {
  const body = bodyOf('cellOf')
  assert.match(body, /c\.fieldType !== 'amount'/, 'cellOf 必须按 fieldType 分流')
  assert.match(body, /return undefined/, '非 amount 列必须返回 undefined 走默认渲染')
  assert.match(body, /fmtAmount\(v\)/, 'amount 列必须走 fmtAmount')
})

test('浮点残留在收敛后不留痕', () => {
  // 直接以真机脏值为样板验证收敛口径（与 fmtAmount 同算法）
  const fmt = (v: number) => (Math.round(v * 100) / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
  assert.equal(fmt(53441.77999999996), '53,441.78')
  assert.equal(fmt(43316.740000000005), '43,316.74')
  assert.equal(fmt(393633.02999999997), '393,633.03')
  // 边界：负数与整元
  assert.equal(fmt(-4538.480000000001), '-4,538.48')
  assert.equal(fmt(44000), '44,000')
})
