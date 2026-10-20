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

// ---- 2026-10-20 升级：状态统计卡 / 即输即搜 / 上传时间区间 ----

test('历史记录：顶部有状态统计筛选小卡片（全部 / 正在执行 / 异常）', () => {
  assert.match(src, /class="dh-status-cards"/)
  assert.match(src, /v-for="c in statusCards"/)
  assert.match(src, /class="dh-status-card"/)
  // 选中态高亮 + 点击切换
  assert.match(src, /:class="\[`dh-status-card--\$\{c\.key\}`.*is-active.*\]"/)
  assert.match(src, /@click="onStatusCardClick\(c\.key\)"/)
})

test('历史记录：状态卡按全量快照计数，正在执行/异常各自归类', () => {
  // 计数来源必须是全量快照（不是当前页）
  assert.match(src, /const statusCards = computed/)
  assert.match(src, /all\.filter\(isRowRunning\)/)
  assert.match(src, /all\.filter\(isRowAbnormal\)/)
  // 执行中口径：解析/提取 的 pending|processing（含 parsing/finalizing）
  assert.match(src, /ROW_PARSE_RUNNING = new Set\(\[[^\]]*'pending'[^\]]*'processing'[^\]]*\]\)/)
  // 异常口径：parse_status=failed 或 extract_status=failed
  assert.match(src, /String\(r\.parse_status \|\| ''\) === 'failed'/)
})

test('历史记录：工具栏无独立「搜索」按钮（即输即搜 + 防抖 + clearable）', () => {
  // 旧按钮已被移除
  assert.equal(/<t-button[^>]*>\s*<template #icon><t-icon name="search"/.test(src), false,
    '不应再渲染独立的「搜索」按钮')
  // 输入框保留 clearable 与搜索图标
  assert.match(src, /<t-input v-model="keywordInput"[^>]*clearable/)
  // 300ms 防抖
  assert.match(src, /setTimeout\(\(\) => \{[\s\S]{0,120}\}, 300\)/)
})

test('历史记录：工具栏含上传时间范围选择器', () => {
  assert.match(src, /<t-date-range-picker/)
  assert.match(src, /v-model="dateRange"/)
  // 2026-10-20 变更：占位符改为数组形式（单字符串会被两端重复渲染）
  assert.match(src, /:placeholder="\['开始日期', '结束日期'\]"/)
  assert.match(src, /clearable/)
})

test('历史记录：搜索 / 时间 / 状态三者 AND 组合过滤', () => {
  assert.match(src, /const filteredRows = computed/)
  assert.match(src, /statusFilter\.value === 'running' && !isRowRunning\(r\)/)
  assert.match(src, /statusFilter\.value === 'abnormal' && !isRowAbnormal\(r\)/)
  // 上传时间按 created_at 自然日比较
  assert.match(src, /String\(r\.created_at \|\| ''\)\.slice\(0, 10\)/)
  // 文件名搜索命中 title/file_name/doc_type
  assert.match(src, /\[r\.title, r\.file_name, r\.doc_type\]/)
})

// ---- 2026-10-20 第二批：发票类型下拉 / 日期占位符 / pending 计入正在执行 ----

test('历史记录：工具栏有「发票类型」下拉框，默认全部分类', () => {
  // 下拉框位于搜索框与日期区间之间
  assert.match(src, /<t-select v-model="typeFilter"[^>]*class="dh-type-select"/)
  // 默认值必须是空串（= 全部分类），不能默认筛掉任何文件
  assert.match(src, /const typeFilter = ref\(''\)/)
  assert.match(src, /label: '全部分类', value: ''/)
  // 选项来自数据里真实出现的分类，且排除空值
  assert.match(src, /const typeOptions = computed/)
  assert.match(src, /if \(!t\) continue/)
})

test('历史记录：日期区间占位符用数组形式，避免「开始 - 结束」重复拼接', () => {
  assert.match(src, /:placeholder="\['开始日期', '结束日期'\]"/)
  // 旧的单字符串占位符必须移除
  assert.equal(/placeholder="上传开始日期 - 上传结束日期"/.test(src), false)
})

test('历史记录：pending（待提取）必须计入「正在执行」卡片并可筛出', () => {
  // parse 已完成 + extract_status 为空（主列表显示「待提取」）也算正在执行
  assert.match(src, /const es = String\(extractStatus\(r\) \|\| ''\)/)
  assert.match(src, /return !es \|\| ROW_EXTRACT_RUNNING\.has\(es\)/)
  // pending 明确在集合里
  assert.match(src, /ROW_EXTRACT_RUNNING = new Set\(\['pending', 'processing', 'parsing'\]\)/)
})

test('历史记录：发票类型与关键词/日期/状态构成 AND 组合过滤', () => {
  assert.match(src, /if \(typeFilter\.value && invoiceTypeOf\(r\) !== typeFilter\.value\) return false/)
})
