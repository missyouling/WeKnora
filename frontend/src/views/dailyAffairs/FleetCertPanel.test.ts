import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const panel = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

test('分类变更后通知列表页：dispatch 全局事件 fleet-categories-changed', () => {
  assert.match(panel, /function notifyCategoriesChanged\(\)/)
  assert.match(panel, /dispatchEvent\(new CustomEvent\('fleet-categories-changed'\)\)/)
})

test('所有分类变更成功点均触发通知：创建/编辑保存/启用入库/启用切换/删除/排序', () => {
  const count = (panel.match(/notifyCategoriesChanged\(\)/g) || []).length
  assert.ok(count >= 6, `通知触发点应不少于 6 处，实际 ${count} 处`)
})

test('内置类型启用入库与普通更新启用均触发通知', () => {
  // 内置启用入库
  const enableBlock = panel.split('// ---- 启用/禁用 ----')[1].split('// ---- 删除')[0] || ''
  assert.ok(enableBlock.includes('notifyCategoriesChanged()'), '启用入库/切换应触发通知')
})
