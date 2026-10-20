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
  assert.match(src, /placeholder="上传开始日期 - 上传结束日期"/)
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
