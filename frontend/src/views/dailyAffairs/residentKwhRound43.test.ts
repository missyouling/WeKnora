import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 第 43 轮：费用清单「电量（定比）」的唯一权威来源是市电账单票面分项
 * `(5)目录电费` 的计费数量，不是 residential_readings[].bill_kwh。
 *
 * 真机事故：2026-07 显示 147.32（= 9821 × 0.015，被乘了两次定比）、
 * 2026-06/2026-04/2025-10 显示 0、2026-05/03/02/2025-12/11 显示乘过一次的值。
 * 而票面 (5)目录电费 qty 在全部 17 个月都正确。
 */
const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
const src = read('./TenantBilling.vue')

test('residentInfoOf 优先读票面 (5)目录电费 的计费数量', () => {
  const fn = src.slice(src.indexOf('const residentInfoOf ='), src.indexOf('// 水表读数按 (meterId, month) 索引'))
  assert.ok(fn.length > 200, '必须能定位到 residentInfoOf 实现')

  // 必须先取 catalogKwh，并且过滤掉工商业目录电费
  assert.match(fn, /catalogKwh/, '必须从 fee_items 汇总目录电费')
  assert.match(fn, /includes\('目录电费'\)/, '必须识别目录电费分类')
  assert.match(fn, /!c\.includes\('工商业'\)/, '必须排除工商业目录电费')

  // 优先级：catalogKwh 在前，bill_kwh 只作兜底
  const catalogIdx = fn.indexOf('Math.round(catalogKwh')
  const billIdx = fn.indexOf("main?.bill_kwh")
  assert.ok(catalogIdx >= 0, '必须以 catalogKwh 初始化 kwh')
  assert.ok(billIdx > catalogIdx, 'bill_kwh 只能作为兜底，不能优先')
})

test('不得把已算好的居民计费电量再乘一次 0.015', () => {
  const fn = src.slice(src.indexOf('const residentInfoOf ='), src.indexOf('// 水表读数按 (meterId, month) 索引'))
  // 反算分支只允许出现在 kwh 仍为 0 时（base × ratio），且必须受 `if (!kwh)` 保护
  const guarded = /if \(!kwh\) \{\s*const base = Number\(main\?\.kwh\)/.test(fn)
  assert.ok(guarded, 'kwh×ratio 反算必须在 !kwh 分支内，不能对已有值再乘')
})

test('用电量（工业）核减用的是修正后的定比电量', () => {
  const idx = src.indexOf('星达工业用电量 = 总电量 - 星达居民电量')
  assert.ok(idx > 0, '必须存在工业电量核减公式')
  const line = src.slice(idx, idx + 220)
  assert.match(line, /totalKwh - res\.kwh - \(Number\(rec\?\.total_kwh\)/, '必须用 res.kwh 核减')
  assert.match(line, /Math\.round\(.*100\) \/ 100/, '必须按分取整')
})

test('均价（工业）按 round4(工业电费 ÷ 工业电量)', () => {
  const idx = src.indexOf('工业均价 = round4(工业电费 ÷ 工业电量)')
  assert.ok(idx > 0, '必须存在新的均价口径注释')
  const line = src.slice(idx, idx + 600)
  // 第 53 轮：分子分母必须同源于本行两列，不得再用宿舍电量/宿舍电费做扣减
  assert.match(line, /indPrice = indKwh > 0 \? Math\.round\(\(indFee \/ indKwh\) \* 10000\) \/ 10000/,
    '均价 = round4(工业电费 ÷ 工业电量)')
  assert.ok(!/tenantDormKwh/.test(line), '分母不得再用持睿宿舍电量（会产生 450,846）')
  assert.ok(!/tenantDormFee/.test(line), '分子不得再用持睿宿舍电费')
})
