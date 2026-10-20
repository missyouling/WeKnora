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

test('两栏表单输入控件宽度统一：开票日期与税率/价税合计右边缘对齐', () => {
  // 栅格里所有输入类控件必须强制 100% 宽 + 允许收缩 + border-box，
  // 否则 t-date-picker 的固有宽度会撑破列宽，右边缘比下方输入框更靠右。
  const gridBlock = src.slice(src.indexOf('.bsd-fields {'), src.indexOf('.bsd-fields :deep(.bsd-field--full)'))
  for (const sel of ['.t-date-picker', '.t-select', '.t-input-number', '.t-input', '.t-textarea']) {
    assert.ok(gridBlock.includes(sel), `栅格宽度约束缺少 ${sel}`)
  }
  assert.match(gridBlock, /width: 100%;/)
  assert.match(gridBlock, /min-width: 0;/)
  assert.match(gridBlock, /box-sizing: border-box;/)
})

test('源预览上分割线与抽屉底栏分割线等宽（负 margin 拉通内容区全宽）', () => {
  const block = src.slice(src.indexOf('.bsd-preview-block {'), src.indexOf('.bsd-preview-toggle {'))
  // 用等量负 margin 抵消 .bsd-body 的左右内边距，使 border-top 贯穿全宽
  assert.match(block, /margin-left: calc\(-1 \* var\(--td-comp-margin-m\)\)/)
  assert.match(block, /margin-right: calc\(-1 \* var\(--td-comp-margin-m\)\)/)
  // 内容必须用等量 padding 推回原位，否则整块文字会贴边
  assert.match(block, /padding: var\(--td-comp-margin-s\) var\(--td-comp-margin-m\) 0/)
  assert.match(block, /border-top: 1px solid var\(--td-component-stroke\)/)
})
