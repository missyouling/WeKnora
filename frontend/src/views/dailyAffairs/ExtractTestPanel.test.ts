import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const panel = readFileSync(new URL('./ExtractTestPanel.vue', import.meta.url), 'utf8')
const inputPanel = readFileSync(new URL('./TestInputPanel.vue', import.meta.url), 'utf8')

// 2026-10-18 需求：测试规则去弹窗化——改为页面内嵌可折叠面板，
// 复用 TestInputPanel / TestResultPanel，做到"边改规则边在同屏测试"。

test('测试面板是页面内嵌 section，不再是 t-dialog 模态框', () => {
  assert.doesNotMatch(panel, /<t-dialog/)
  assert.doesNotMatch(panel, /:attach="'body'"/)
  assert.match(panel, /<section v-if="visible" class="extract-test-panel"/)
})

test('复用输入/结果子面板，保持左右 5/12 + 7/12 分栏', () => {
  assert.match(panel, /<TestInputPanel/)
  assert.match(panel, /<TestResultPanel/)
  assert.match(panel, /<t-col :span="5" class="col-left">/)
  assert.match(panel, /<t-col :span="7" class="col-right">/)
})

test('文本测试 500ms 防抖自动触发；文件测试只拉原文不自动测', () => {
  assert.match(panel, /watch\(text/)
  assert.match(panel, /\}, 500\)/)
  assert.match(panel, /watch\(knowledgeId, async/)
  assert.match(panel, /getKnowledgeDetails/)
  assert.match(panel, /payload\.knowledge_id = knowledgeId\.value/)
})

test('两种测试方式的结果互相隔离（切换标签不串扰）', () => {
  assert.match(panel, /resultMap = ref<Record<Mode, any>>/)
  assert.match(panel, /errorMap = ref<Record<Mode, string>>/)
  assert.match(panel, /successMap = ref<Record<Mode, boolean>>/)
  assert.match(panel, /resultMap\.value\[sourceMode\.value\] = data/)
})

test('收起不打断进行中的测试/保存（避免丢请求）', () => {
  assert.match(panel, /function onClose\(\) \{\r?\n {2}if \(testing\.value \|\| saving\.value\) return/)
})

test('VLM 原文区仍在输入面板中（原有契约不回退）', () => {
  assert.match(inputPanel, /VLM 识别原文/)
  assert.match(inputPanel, /original-area/)
})

// ---- 2026-10-18 终轮：测试面板交互收口 ----

const etp = readFileSync(new URL('./ExtractTestPanel.vue', import.meta.url), 'utf8')
const tip = readFileSync(new URL('./TestInputPanel.vue', import.meta.url), 'utf8')
const trp = readFileSync(new URL('./TestResultPanel.vue', import.meta.url), 'utf8')

test('面板内不再有「收起」「关闭」按钮（开关由工具栏图标控制）', () => {
  assert.doesNotMatch(etp, />收起</)
  assert.doesNotMatch(etp, />关闭</)
  assert.match(etp, /工具栏的「测试规则」图标按钮控制/)
})

test('测试反馈统一走 Toast，不再挂会自己消失的内嵌提示条', () => {
  assert.doesNotMatch(trp, /t-alert/)
  assert.doesNotMatch(trp, /alertVisible/)
  assert.match(etp, /MessagePlugin\.success\(`提取完成/)
  assert.match(etp, /MessagePlugin\.error\(/)
})

test('文本提取测试 Tab 不渲染「VLM 识别原文」预览框', () => {
  assert.match(tip, /<div v-if="sourceMode === 'file'" class="original-block">/)
})

test('字段映射第一列显示中文名；列表限高内滚', () => {
  assert.match(trp, /return \{ name: f\.label \|\| labelOf\(f\.name\), valueText, status \}/)
  assert.match(trp, /class="map-scroll"/)
  assert.match(trp, /\.map-scroll \{\s*max-height: 300px;\s*overflow-y: auto;/)
})

test('左右两栏顶部对齐并等高', () => {
  assert.match(etp, /:deep\(\.t-row\) \{ align-items: stretch; \}/)
})
