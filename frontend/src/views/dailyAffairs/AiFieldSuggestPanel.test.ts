import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const panel = readFileSync(new URL('./AiFieldSuggestPanel.vue', import.meta.url), 'utf8')
const cert = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

// 2026-10-18 需求 1+2：字段生成去弹窗（内嵌）、三分组四列、自动补建明细父字段。

test('字段生成是内嵌面板，不再有模态对话框与遮罩', () => {
  assert.doesNotMatch(panel, /<t-dialog/)
  assert.doesNotMatch(panel, /:close-on-overlay-click/)
  assert.match(panel, /<section v-if="visible" class="ai-field-panel"/)
})

test('内嵌面板锚点在「发票类型」下方、「字段配置」上方', () => {
  const formGridEnd = cert.indexOf('<AiFieldSuggestPanel')
  const fieldConfig = cert.indexOf('<div class="field-config">')
  const nameInput = cert.indexOf('v-model="editForm.name"')
  assert.ok(nameInput !== -1 && formGridEnd !== -1 && fieldConfig !== -1)
  assert.ok(nameInput < formGridEnd, 'AI 面板在分类名输入之后')
  assert.ok(formGridEnd < fieldConfig, 'AI 面板在字段配置之前')
})

test('编辑面板左下角不再有「智能生成字段」按钮（入口已移到分类行 ⋮ 菜单）', () => {
  assert.doesNotMatch(cert, /btn-ai-field/, '旧的按钮样式/类名必须消失')
  assert.doesNotMatch(cert, /@click="openAiFieldSuggest"/)
  assert.match(cert, /content: '智能生成字段', value: 'ai'/, '菜单项必须存在')
  assert.match(cert, /if \(value === 'ai'\) openAiPanel\(item\)/, '菜单必须能打开内嵌面板')
})

test('新建分类后自动展开编辑面板并打开 AI 生成区', () => {
  const idx = cert.indexOf('const created = res.data || res')
  assert.ok(idx !== -1)
  const after = cert.slice(idx, idx + 900)
  assert.match(after, /openAiPanel\(createdItem\)/, '新建成功后应展开 AI 面板')
})

test('识别结果固定三分组，且使用统一的五列网格', () => {
  assert.match(panel, /title: '标准字段'/)
  assert.match(panel, /title: '扩展字段'/)
  assert.match(panel, /title: '明细字段'/)
  // 五列：勾选 / 编码 / 名称 / 类型 / 样本
  assert.match(panel, /grid-template-columns: 22px minmax\(110px, 0\.9fr\) minmax\(110px, 1fr\) 96px minmax\(80px, 1fr\)/)
  // 三个分组固定展示（某组为空时给空态，不整块消失）
  assert.match(panel, /本样票未识别到该组字段/)
})

test('字段编码只读展示；字段名称与数据类型可编辑', () => {
  assert.match(panel, /<span class="afp-key"[^>]*>\{\{ r\.key \}\}<\/span>/, '编码必须是纯文本（不可编辑）')
  assert.match(panel, /<t-input v-model="r\.label"/, '中文名可改')
  assert.match(panel, /<t-select v-model="r\.data_type"/, '类型可改')
  assert.match(panel, /label: '数组', value: 'array'/, '类型选项含 array')
})

test('勾选明细子列且无 items 字段时提示自动补建，而不是"无法保存"', () => {
  assert.match(panel, /当前分类没有「明细」字段，保存时会自动创建「项目明细」父字段/)
  assert.doesNotMatch(panel, /子列将无法保存/)
})

// 2026-10-18 实测补齐：内嵌面板高 1444px（含 276px textarea + 924px 结果表），
// 把下方「字段配置」和「保存」顶出视口；内嵌反而比弹窗更难用。两处收口：
test('识别成功后折叠输入区，避免 textarea 与上传区长期占高', () => {
  assert.match(panel, /const inputCollapsed = ref\(false\)/)
  assert.match(panel, /<template v-if="!inputCollapsed">/, '输入区必须可折叠')
  assert.match(panel, /if \(n\) inputCollapsed\.value = true/, '识别成功应自动折叠')
  assert.match(panel, /重新选择样票/)
  assert.match(panel, /inputCollapsed\.value = false/, 'resetState 必须复位折叠态')
})

test('结果区限高内滚，字段多时仍能看到下方字段配置', () => {
  assert.match(panel, /<div class="afp-scroll">/)
  assert.match(panel, /max-height: min\(46vh, 420px\)/)
  assert.match(panel, /overflow-y: auto/)
})
