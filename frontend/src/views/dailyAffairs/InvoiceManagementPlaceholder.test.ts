import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const src = readFileSync(
  fileURLToPath(new URL('./InvoiceManagement.vue', import.meta.url)),
  'utf8',
)

// 铁律一（前端侧）：无 invoices 明细的 Knowledge 必须渲染占位行，而不是被静默过滤。
// 这类文件"知识库里有、列表里看不见"，且会被相同文件哈希挡住重传，是数据不一致的根因。

test('占位行：is_placeholder 必须被识别并保留，不允许静默过滤', () => {
  assert.match(
    src,
    /is_placeholder/,
    'InvoiceManagement.vue 必须识别后端返回的 is_placeholder 标记',
  )
})

test('占位行：不得按"有发票号"过滤掉占位行（历史逻辑只认有明细的行）', () => {
  // 占位行天生没有 invoiceNo，任何以 invoiceNo 为存在前提的过滤都会把
  // 孤儿文件重新变隐身。这里锁定模板不得对占位行做 v-if="row.invoiceNo" 类排除。
  assert.doesNotMatch(
    src,
    /v-if="[^"]*row\.invoiceNo[^"]*"\s*\/?>/,
    '不允许用 row.invoiceNo 作为行的渲染前提——占位行没有发票号',
  )
})

test('占位行：占位原因 placeholder_reason 必须可展示', () => {
  assert.match(src, /placeholder_reason/)
})

test('占位行：必须提供重新提取入口（用户能自救，不必重传）', () => {
  assert.match(src, /extract-business|extractBusinessDocument|reextractInvoice/i)
})

test('占位行：必须提供删除入口（级联删除同一套 handler）', () => {
  assert.match(src, /delete-invoice-page|deleteInvoicePage/)
})

test('日期筛选：前端不得在本地二次按开票日期过滤（后端已兼容通行日期）', () => {
  // 后端 invoiceRecordDateInRange 已做"顶层日期 OR 明细通行日期"命中，
  // 前端若再本地过滤一次会把这部分重新筛掉，造成同一口径两处实现互相打架。
  const forbidden = [
    /localDateFilter/,
    /dateRange\s*&&[^\n]*invoiceDate[^\n]*</,
    /filter\(\s*\(r\)\s*=>[^\n]*InvoiceDate[^\n]*DateFrom/,
  ]
  for (const re of forbidden) {
    assert.doesNotMatch(src, re, `InvoiceManagement.vue 不应本地重复日期过滤：${re}`)
  }
})
