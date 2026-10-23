import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { fromSubsPayload, toSubsPayload } from './fleetCategorySubs'

/**
 * 「费用组成 / 分时时段」细项配置入口的回归锁定。
 *
 * 背景（本轮修复）：上一版把 `<分组>-rows` 直接从分组胶囊里过滤掉了，
 * 结果点进「市场化购电费」只剩 5 个列字段（费用组成/分时时段/计费电量/计费标准/电费），
 * 用户完全没有入口去配置「费用组成」下的 22 条行细项（零售交易电费、偏差电费、
 * 绿电交易电费(省间)…）与「分时时段」下的 尖/峰/平/谷。
 *
 * 这组测试锁三条：
 *   ① 细项仍以 subs 条目存在（group = `<分组>-rows` / `<分组>-periods`）；
 *   ② 面板里有对应的展开入口与映射规则（源码级契约）；
 *   ③ 细项的 label 改名不会动 name（name 是 fee_items 的行匹配键）。
 */

const src = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

test('费用组成/分时时段 存在细项展开入口', () => {
  assert.match(src, /fc-child-toggle/, '字段行必须有细项展开按钮')
  assert.match(src, /function childGroupOf/, '需要有「字段 → 细项分组」的映射函数')
  // 费用组成 → <分组>-rows；分时时段 → <分组>-periods
  assert.match(src, /n === 'name'\) return `\$\{activeSubGroup\.value\}-rows`/, '费用组成应映射到 -rows 分组')
  assert.match(src, /n === 'period'\) return `\$\{activeSubGroup\.value\}-periods`/, '分时时段应映射到 -periods 分组')
  assert.match(src, /function addChild/, '细项必须可新增')
  assert.match(src, /function removeChild/, '细项必须可删除')
  assert.match(src, /function moveChild/, '细项必须可排序')
})

test('细项改名只改 label，不动 name（行匹配键）', () => {
  const fn = /function renameChild\(ch: any, v: string\) \{[\s\S]*?\n\}/.exec(src)
  assert.ok(fn, '缺少 renameChild')
  assert.match(fn![0], /ch\.label = v/, '改名必须写 label')
  assert.doesNotMatch(fn![0], /ch\.name\s*=/, '改名绝不能写 name —— 那会让 fee_items 匹配不到行')
})

test('细项分组不进分组胶囊（避免 5 条列字段被 22 条行项淹没）', () => {
  assert.match(src, /!g\.key\.endsWith\('-rows'\) && !g\.key\.endsWith\('-periods'\)/, '胶囊条需同时排除 -rows 与 -periods')
})

test('subs 往返：-rows / -periods 细项带 group 持久化', () => {
  const raw = [
    { name: 'name', label: '费用组成', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market', group_label: '市场化购电费' },
    { name: '偏差电费(尖峰)', label: '偏差电费(尖峰)', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market-rows' },
    { name: '尖', label: '尖', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market-periods' },
  ]
  const fields = fromSubsPayload(raw)
  assert.equal(fields.length, 3, '同名字段在不同分组下不能被去重掉')

  const back = toSubsPayload(fields)
  assert.equal(back[1]!.group, 'market-rows')
  assert.equal(back[2]!.group, 'market-periods')
  // 往返无损：细项命名与分组都要保留
  assert.equal(back[1]!.name, '偏差电费(尖峰)')
  assert.equal(back[2]!.name, '尖')
})
