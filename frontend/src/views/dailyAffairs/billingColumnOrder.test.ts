import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { resolveSourceValue } from './tenantBillingSources'

/**
 * 费用清单第 9 轮（2026-10-28 下午）回归契约。
 *
 * ① 列顺序必须严格跟随 subs：旧实现「先铺内置列，再把跨分类引用列追加到末尾」，
 *    用户把「账单周期…」拖到最前面保存后，列表里仍排在十几列之后；
 * ② 三端（设置抽屉 / [☰ 字段] / 表头）必须同名同量同序；
 * ③ [☰ 字段] 弹窗被视口裁掉（只显示前 8 条、不能滚动）；
 * ④ [电费核算] 使用单位 被当数值求和，整列显示 0。
 */

const here = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(here, f), 'utf8')

// ---- ①② 列顺序 ----

test('列派生必须单循环遍历 subs：内置列与引用列按同一顺序产出', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('const activeColDefs = computed'), src.indexOf('const colStorageKey'))
  // 只允许一个主循环（第 15 轮起先按大类折叠 subs，再逐个产出列），
  // 且同时处理 source 与非 source
  assert.match(fn, /for \(const row of collapseFeeGroups\(effective\)\)/)
  assert.match(fn, /const src = parseFieldSource\(sub\)/)
  assert.match(fn, /if \(src\) \{[\s\S]{0,700}?\} else \{/, '引用列与内置列必须在同一循环里分流')
  // 不允许再出现「extras 追加到 base 之后」的旧结构
  assert.ok(!/const extras: ColDef\[\] = \[\]/.test(fn), '不得再有 extras 追加数组')
  assert.ok(!/\[\.\.\.base, \.\.\.extras\]/.test(fn), '不得再把引用列拼到内置列末尾')
  assert.ok(!/restrictToBase/.test(fn))
})

test('表头顺序不得被 localStorage 的勾选顺序锁死', () => {
  const src = read('TenantBilling.vue')
  const m = /const visibleColDefs = computed\(([\s\S]{0,200}?)\)\r?\n/.exec(src)
  assert.ok(m, '缺少 visibleColDefs')
  assert.match(m![1], /activeColDefs\.value\.filter/)
  assert.ok(!/visibleKeys\.value\.map/.test(m![1]), '严禁 visibleKeys.map —— 那会把列顺序钉在历史勾选顺序上')
})

test('重置列 = subs 里「启用 且 默认表头」的字段集合（保持 subs 顺序）', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /const resetColumns = \(\) => \{ visibleKeys\.value = activeColDefs\.value\.filter\(c => c\.default\)\.map\(c => c\.key\); persistColumns\(\) \}/)
})

// ---- ③ 筛选器弹窗可滚动 ----

test('[☰ 字段] 弹窗必须按视口剩余高度收敛，并在列表内部纵向滚动', () => {
  const src = read('BusinessColumnFilter.vue')
  // 定位：算下方可用高度，不够就翻到按钮上方
  assert.match(src, /const below = vh - \(r\.bottom \+ 6\) - 12/)
  assert.match(src, /if \(below < 240 && above > below\)/)
  assert.match(src, /maxH/)
  // 模板把 maxHeight 传给弹层
  assert.match(src, /maxHeight: pos\.maxH \+ 'px'/)
  // 列表纵向滚动 + 不裁切
  assert.match(src, /\.field-popup-list \{[\s\S]{0,240}?max-height: 360px;/)
  assert.match(src, /\.field-popup-list \{[\s\S]{0,240}?overflow-y: auto;/)
  assert.match(src, /\.field-popup-list \{[\s\S]{0,240}?flex: 1 1 auto;/)
  // 列容器里的子项必须 flex:none —— 否则高度不足时它们被压扁而不是滚动
  assert.match(src, /\.field-popup-list \.field-popup-item \{[\s\S]{0,260}?flex: none;/,
    '子项必须禁止收缩，否则矮窗口下会被压扁、看不到滚动')
})

// ---- ④ 文本字段不得被求和 ----

test('[电费核算] 使用单位 必须返回文本，而不是 0', () => {
  const ctx: any = {
    month: '2026-09',
    tenantName: '重庆星达',
    bill: { item: {} },
    solarItems: [],
    elecRows: [
      { month: '2026-09', use_unit: '重庆星达', meter_kind: 'time', usage: 120, unit_price: 0.7 },
      { month: '2026-09', use_unit: '重庆星达', meter_kind: 'dorm', usage: 30, unit_price: 0.7 },
    ],
    gasRows: [],
    waterMeters: [],
    waterReadingMap: new Map(),
  }
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'use_unit', mode: 'field' } as any, ctx), '重庆星达')
  // 数值字段照旧求和
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'usage', mode: 'field' } as any, ctx), 150)
  // 多值文本去重后用「、」连接
  ctx.elecRows.push({ month: '2026-09', use_unit: '重庆星达', meter_kind: 'time', usage: 1, reader: '张三' })
  ctx.elecRows.push({ month: '2026-09', use_unit: '重庆星达', meter_kind: 'time', usage: 1, reader: '李四' })
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'reader', mode: 'field' } as any, ctx), '张三、李四')
})

test('文本判定走「取值形态」而不是字段白名单（未知文本字段也不能算成 0）', () => {
  const src = read('tenantBillingSources.ts')
  const fn = src.slice(src.indexOf('function aggregateRows'), src.indexOf('function matchBillFeeItems'))
  assert.match(fn, /const allNumeric = raw\.length > 0 && raw\.every/)
  assert.match(fn, /if \(!allNumeric && raw\.length\)/)
  assert.match(fn, /new Set\(raw\.map/)
})
