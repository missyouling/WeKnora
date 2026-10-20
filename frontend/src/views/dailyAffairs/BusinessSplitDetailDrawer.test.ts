import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const src = readFileSync(new URL('./BusinessSplitDetailDrawer.vue', import.meta.url), 'utf8')
const pageSrc = readFileSync(new URL('./InvoiceManagement.vue', import.meta.url), 'utf8')

test('单栏抽屉：源预览位于页面底部，默认折叠不挂载 DocumentPreview', () => {
  // 单栏垂直流容器
  assert.match(src, /class="bsd-body"/)
  assert.match(src, /flex-direction: column/)
  // 不再有左右分屏的右栏
  assert.doesNotMatch(src, /bsd-main/)
  assert.doesNotMatch(src, /bsd-preview"/)
  assert.doesNotMatch(src, /bsd-preview-rail/)
  // 预览容器永远在最底部
  assert.match(src, /class="bsd-preview-block"/)
  // 折叠态（默认）不挂载重型资源：预览面板与 DocumentPreview 都由 previewExpanded 门控
  assert.match(src, /const previewExpanded = ref\(false\)/)
  assert.match(src, /v-if="previewExpanded" class="bsd-preview-pane"/)
  assert.match(src, /<DocumentPreview v-if="row"/)
})

test('单栏抽屉：不再随预览加宽（无 1180px 分屏逻辑）', () => {
  assert.doesNotMatch(src, /expandedWidth/)
  assert.doesNotMatch(src, /1180px/)
  assert.match(src, /const drawerWidth = computed\(\(\) => props\.width\)/)
  // 表单栅格自适应列数（窄抽屉落单列）
  assert.match(src, /repeat\(auto-fit, minmax\(200px, 1fr\)\)/)
})

test('单栏抽屉：质量溯源 / 动态字段 / 明细 / 扩展插槽齐全', () => {
  assert.match(src, /<ExtractQualityPanel v-if="quality"/)
  // 主表单只渲染 formFields（已剔除 items 占位行）
  assert.match(src, /v-for="f in formFields"/)
  assert.match(src, /:name="`field-\$\{f\.name\}`"/)
  assert.match(src, /v-if="supportsSubFields && itemRows\.length"/)
  assert.match(src, /slot name="extra"/)
  assert.match(src, /slot name="footer-left"/)
})

test('单栏抽屉：items 类型字段不渲染占位行（明细由表格承载）', () => {
  // formFields 必须过滤掉 items 容器字段
  assert.match(src, /f\.dataType !== 'items' && f\.name !== 'items' && f\.label !== '项目明细'/)
})

test('单栏抽屉：底部动作绑定脏检查与 loading', () => {
  assert.match(src, /:loading="saving"/)
  assert.match(src, /:disabled="dirty === false"/)
  assert.match(src, /@confirm="emit\('delete'\)"/)
  assert.match(src, /emit\('re-extract'\)/)
})

test('单栏抽屉不认识任何业务 scope（字段控件由 dataType + 具名插槽决定）', () => {
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
