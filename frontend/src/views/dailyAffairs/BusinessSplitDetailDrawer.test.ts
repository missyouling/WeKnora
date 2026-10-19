import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const src = readFileSync(new URL('./BusinessSplitDetailDrawer.vue', import.meta.url), 'utf8')
const pageSrc = readFileSync(new URL('./InvoiceManagement.vue', import.meta.url), 'utf8')

test('分屏抽屉：左预览右表单，预览折叠时不挂载 DocumentPreview', () => {
  // 预览用 v-if 门控（折叠=不渲染，避免 PDF/图片渲染器初始化）
  assert.match(src, /<div v-if="previewOpen" class="bsd-preview-pane">/)
  assert.match(src, /<DocumentPreview v-if="row"/)
  // 折叠态留竖排引导条，用户仍能找到展开入口
  assert.match(src, /bsd-preview-rail/)
})

test('分屏抽屉：展开预览时自动加宽，否则右栏被压成省略号', () => {
  assert.match(src, /const drawerWidth = computed\(\(\) => \(previewOpen\.value \? props\.expandedWidth : props\.width\)\)/)
  assert.match(src, /expandedWidth: '1180px'/)
  // 表单栅格自适应列数（窄抽屉落单列）
  assert.match(src, /repeat\(auto-fit, minmax\(200px, 1fr\)\)/)
  assert.match(src, /min-width: 340px/)
})

test('分屏抽屉：质量溯源 / 动态字段 / 明细 / 扩展插槽齐全', () => {
  assert.match(src, /<ExtractQualityPanel v-if="quality"/)
  assert.match(src, /v-for="f in fields"/)
  assert.match(src, /:name="`field-\$\{f\.name\}`"/)
  assert.match(src, /v-if="supportsSubFields && itemRows\.length"/)
  assert.match(src, /slot name="extra"/)
  assert.match(src, /slot name="footer-left"/)
})

test('分屏抽屉：底部动作绑定脏检查与 loading', () => {
  assert.match(src, /:loading="saving"/)
  assert.match(src, /:disabled="dirty === false"/)
  assert.match(src, /@confirm="emit\('delete'\)"/)
  assert.match(src, /emit\('re-extract'\)/)
})

test('分屏抽屉不认识任何业务 scope（字段控件由 dataType + 具名插槽决定）', () => {
  assert.doesNotMatch(src, /scope\s*===/)
  assert.doesNotMatch(src, /'invoice'/)
  // 三种 dataType 的默认控件必须齐
  assert.match(src, /f\.dataType === 'date'/)
  assert.match(src, /f\.dataType === 'number'/)
  assert.match(src, /f\.dataType === 'array'/)
})

test('发票页已切换：内联抽屉模板与重复表单逻辑移出', () => {
  assert.match(pageSrc, /<BusinessSplitDetailDrawer/)
  assert.match(pageSrc, /#field-invoiceType/)
  assert.match(pageSrc, /#field-extractStatus/)
  assert.match(pageSrc, /#field-tags/)
  // 旧的折叠预览/内联字段网格不再存在
  assert.doesNotMatch(pageSrc, /t-collapse-panel value="preview"/)
  assert.doesNotMatch(pageSrc, /class="field-grid"/)
  // 保存链路走纯函数
  assert.match(pageSrc, /buildDetailPayload\(\{/)
  assert.match(pageSrc, /findInvoiceIndex\(invoices, nowPage, now\.invoiceNo\)/)
  assert.match(pageSrc, /extractStatusOf\(hasInvoiceData\(invoices\)\)/)
})
