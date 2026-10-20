import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const src = readFileSync(new URL('./FleetUploadHistoryDrawer.vue', import.meta.url), 'utf8')

test('历史记录：双击文件名打开全屏源文件预览', () => {
  // 双击事件绑定在文件名区域
  assert.match(src, /@dblclick="openFullPreview\(row\)"/)
  // 全屏预览对话框：复用中台 document-preview，宽 92vw
  assert.match(src, /class="dh-full-preview"/)
  assert.match(src, /width="92vw"/)
  assert.match(src, /<DocumentPreview/)
  assert.match(src, /:knowledge-id="previewRow\.id"/)
  // 关闭时销毁，避免 PDF 渲染器常驻占用内存
  assert.match(src, /destroy-on-close/)
})

test('历史记录：文件名区域可点击提示（cursor: pointer + 双击文案）', () => {
  assert.match(src, /\.dh-file--clickable \{ cursor: pointer; \}/)
  assert.match(src, /双击全屏预览源文件/)
})

test('历史记录：提取/解析失败 Tooltip 走中文转译，不透出裸错误', () => {
  assert.match(src, /formatExtractErrorMessage\(extractError\(row\)\)/)
  assert.match(src, /formatExtractErrorMessage\(parseError\(row\)\)/)
})

test('历史记录：全屏预览仅在打开时挂载重型预览组件', () => {
  assert.match(src, /v-if="previewVisible && previewRow"/)
  // 关闭后清理行引用，避免残留旧文件
  assert.match(src, /function closeFullPreview\(\)/)
})
