import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const drawer = readFileSync(new URL('./UtilitySettingsDrawer.vue', import.meta.url), 'utf8')
// 只看 template 部分，避免把 <style>/<script> 里的字符串算进来
const template = drawer.slice(0, drawer.indexOf('<script setup'))

test('能耗设置收敛为单层抽屉：模板里只有一个 t-drawer（不再抽屉套抽屉）', () => {
  const count = (template.match(/<t-drawer/g) || []).length
  assert.equal(count, 1, `期望只有 1 个 t-drawer，实际 ${count} 个`)
  assert.ok(!template.includes('字段分组管理抽屉'), '第二层抽屉应已移除')
})

test('字段分组编辑改为同一抽屉内切换视图（含返回入口）', () => {
  assert.match(template, /v-if="!groupView"/)
  assert.match(template, /v-else/)
  assert.match(template, /closeGroupEdit/)
  assert.match(template, /返回/)
  // 不再依赖控制第二层抽屉可见性的布尔量
  assert.ok(!/groupEditVisible/.test(drawer), 'groupEditVisible 应被 groupView 取代')
})

test('保存动作保留：分组内保存后回到设置主视图，主视图保存仍写包含判定', () => {
  assert.match(drawer, /saveGroupFields/)
  assert.match(drawer, /persistAll/)
  assert.match(drawer, /const saveGroupEdit = async/)
  assert.match(drawer, /groupView\.value = false/)
})

test('列表字段计数与分组卡片交互保持原状（未被重写）', () => {
  assert.match(drawer, /groupFieldCount/)
  assert.match(drawer, /openGroupEdit/)
  assert.match(drawer, /ROW_GROUP_MAP/)
})
