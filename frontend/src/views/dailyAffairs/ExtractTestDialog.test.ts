import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const dialog = readFileSync(new URL('./ExtractTestDialog.vue', import.meta.url), 'utf8')
const inputPanel = readFileSync(new URL('./TestInputPanel.vue', import.meta.url), 'utf8')
const resultPanel = readFileSync(new URL('./TestResultPanel.vue', import.meta.url), 'utf8')

test('测试弹窗为 72% 宽度并采用左右 5/12 + 7/12 分栏（TDesign 12 栅格）', () => {
  assert.match(dialog, /:width="'72%'"/)
  assert.match(dialog, /<t-col :span="5" class="col-left">/)
  assert.match(dialog, /<t-col :span="7" class="col-right">/)
})

test('测试弹窗显式挂载到 body，从抽屉容器中独立出来', () => {
  assert.match(dialog, /:attach="'body'"/)
})

test('输入方式：文件提取测试在前、文本提取测试在后', () => {
  const fileIdx = inputPanel.indexOf('value="file" label="文件提取测试"')
  const textIdx = inputPanel.indexOf('value="text" label="文本提取测试"')
  assert.ok(fileIdx !== -1 && textIdx !== -1, '两个标签都存在')
  assert.ok(fileIdx < textIdx, '文件提取测试位于文本提取测试之前')
})

test('左侧面板包含 VLM 识别原文区：识别中/识别完成/无可用原文/文本预览 状态标签，原文只读等宽', () => {
  assert.match(inputPanel, /VLM 识别原文/)
  assert.match(inputPanel, /识别中/)
  assert.match(inputPanel, /识别完成/)
  assert.match(inputPanel, /无可用原文/)
  assert.match(inputPanel, /文本预览/)
  assert.match(inputPanel, /original-area/)
  assert.match(inputPanel, /:model-value="originalText"/)
  assert.match(inputPanel, /font-family: 'JetBrains Mono'/)
})

test('文本提取测试：输入变化 500ms 防抖自动触发，切换标签不触发', () => {
  assert.match(dialog, /watch\(text/)
  assert.match(dialog, /setTimeout\(\(\) => \{/)
  assert.match(dialog, /\}, 500\)/)
  assert.match(dialog, /sourceMode\.value !== 'text'/)
})

test('文件提取测试：选择文件后拉取 VLM 原文但不自动测试（需手动点运行测试）', () => {
  assert.match(dialog, /watch\(knowledgeId, async/)
  assert.match(dialog, /getKnowledgeDetails/)
  assert.match(dialog, /不自动测试/)
  assert.match(dialog, /「运行测试」手动触发/)
  assert.match(dialog, /payload\.knowledge_id = knowledgeId\.value/)
})

test('提取结果按测试方式隔离：文件/文本测试结果互不串扰', () => {
  assert.match(dialog, /resultMap = ref<Record<Mode, any>>/)
  assert.match(dialog, /errorMap = ref<Record<Mode, string>>/)
  assert.match(dialog, /successMap = ref<Record<Mode, boolean>>/)
  assert.match(dialog, /resultMap\.value\[sourceMode\.value\] = data/)
  assert.match(dialog, /const result = computed\(\(\) => resultMap\.value\[sourceMode\.value\]\)/)
})

test('切换标签时弹窗尺寸一致：左侧限高内滚，右侧与左列等高', () => {
  assert.match(inputPanel, /max-height: 540px/)
  assert.match(inputPanel, /overflow-y: auto/)
  assert.match(dialog, /:deep\(\.col-left\),\s*\n\s*:deep\(\.col-right\) \{/)
  assert.match(dialog, /height: 100%;/)
})

test('切回文件标签时若已选文件则重新拉取原文恢复「识别完成」，无文件才显示「待选择」', () => {
  assert.match(dialog, /async function loadOriginal\(kid: string\)/)
  assert.match(dialog, /else if \(knowledgeId\.value\) \{\s*\n\s*void loadOriginal\(knowledgeId\.value\)/)
  assert.match(dialog, /originalStatus\.value = 'none'/)
})

test('底部操作区三按钮：运行测试（loading）、确认规则并保存（成功后显示）、关闭', () => {
  assert.match(dialog, /运行测试/)
  assert.match(dialog, /v-if="success && !testing"/)
  assert.match(dialog, /确认规则并保存/)
  assert.match(dialog, />关闭<\/t-button>/)
})

test('测试期间禁用输入与操作按钮，完成后可保存', () => {
  assert.match(dialog, /:disabled="testing"/)
  assert.match(dialog, /saveRule/)
  assert.match(dialog, /saveExtractConfig/)
})

test('右侧结果面板：初始空态、测试中 loading、成功/失败 alert（简洁文案）', () => {
  assert.match(resultPanel, /t-empty/)
  assert.match(resultPanel, /t-loading/)
  assert.match(resultPanel, /t-alert v-if="error" theme="error"/)
  assert.match(resultPanel, /theme="success" message="提取完成"/)
})

test('字段映射表三列：字段名称 / 提取结果 / 状态，状态三态标签', () => {
  assert.match(resultPanel, /字段名称/)
  assert.match(resultPanel, /提取结果/)
  assert.match(resultPanel, /title: '状态'/)
  assert.match(resultPanel, /已提取/)
  assert.match(resultPanel, /未提取到/)
  assert.match(resultPanel, /格式异常/)
  assert.match(resultPanel, /theme: statusTheme\(row.status\)/)
})

test('字段名称与状态列宽固定，结果列自适应省略，表头不换行', () => {
  assert.match(resultPanel, /colKey: 'name', title: '字段名称', width: 130/)
  assert.match(resultPanel, /title: '状态',\s*\n\s*width: 96/)
  assert.match(resultPanel, /white-space: nowrap/)
  assert.match(resultPanel, /text-overflow: ellipsis/)
})

test('提供原始 JSON 与实际 Prompt 折叠面板，等宽只读展示', () => {
  assert.match(resultPanel, /原始 JSON（模型返回）/)
  assert.match(resultPanel, /实际 Prompt（发送给大模型的完整文本）/)
  assert.match(resultPanel, /mono-area/)
  assert.match(resultPanel, /promptPreview/)
})

test('格式异常按字段数据类型判定：number 非数字、date 非日期、array 非数组', () => {
  assert.match(resultPanel, /t === 'number'/)
  assert.match(resultPanel, /Number\.isNaN\(Number\(v\)\)/)
  assert.ok(resultPanel.includes(String.raw`\d{4}-\d{2}-\d{2}`))
  assert.match(resultPanel, /t === 'array'/)
  assert.match(resultPanel, /!Array\.isArray\(v\)/)
})
