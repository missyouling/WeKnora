import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { formatExtractErrorMessage } from './extractErrorMessage'

const RAW_429 =
  'invoice extraction failed: extract invoice fields: create chat completion: API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制，请您控制请求频率"}}'

test('429/1302 转译为限流提示', () => {
  assert.equal(formatExtractErrorMessage(RAW_429), '请求频率超限（已触发大模型限流保护），请稍后重试')
  assert.equal(
    formatExtractErrorMessage('API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制"}}'),
    '请求频率超限（已触发大模型限流保护），请稍后重试',
  )
  assert.equal(formatExtractErrorMessage('rate limit exceeded'), '请求频率超限（已触发大模型限流保护），请稍后重试')
  assert.equal(formatExtractErrorMessage('您的账户已达到速率限制'), '请求频率超限（已触发大模型限流保护），请稍后重试')
})

test('超时转译为提取超时', () => {
  assert.equal(formatExtractErrorMessage('context deadline exceeded'), '提取超时，文档较大或模型响应缓慢')
  assert.equal(formatExtractErrorMessage('i/o timeout'), '提取超时，文档较大或模型响应缓慢')
})

test('无有效文本转译为源文件提示', () => {
  assert.equal(formatExtractErrorMessage('no text content to extract'), '未识别到有效票面文本，请检查源文件是否清晰')
  assert.equal(formatExtractErrorMessage('empty content'), '未识别到有效票面文本，请检查源文件是否清晰')
})

test('剥离英文技术栈前缀露出中文原因', () => {
  assert.equal(formatExtractErrorMessage('invoice extraction failed: extract invoice fields: 票面缺少发票号码'), '票面缺少发票号码')
  assert.equal(formatExtractErrorMessage('票面金额与明细不符'), '票面金额与明细不符')
})

test('空值返回空串', () => {
  assert.equal(formatExtractErrorMessage(''), '')
  assert.equal(formatExtractErrorMessage('   '), '')
  assert.equal(formatExtractErrorMessage(null), '')
  assert.equal(formatExtractErrorMessage(undefined), '')
})

test('转译结果幂等且不含裸 JSON / 英文栈残留', () => {
  const once = formatExtractErrorMessage(RAW_429)
  assert.equal(formatExtractErrorMessage(once), once)
  for (const leak of ['{', '}', 'invoice extraction failed', 'create chat completion', 'status 429']) {
    assert.equal(once.includes(leak), false, `转译结果不应残留 ${leak}: ${once}`)
  }
})

test('历史记录抽屉与发票列表必须使用统一转译函数（禁止裸错误透出）', () => {
  const src = readFileSync(new URL('./FleetUploadHistoryDrawer.vue', import.meta.url), 'utf8')
  assert.ok(src.includes('formatExtractErrorMessage'), 'FleetUploadHistoryDrawer 必须使用 formatExtractErrorMessage')
  // 提取状态 Tooltip 不得再直接把 extract_error 原样塞进去
  assert.ok(
    /:content="formatExtractErrorMessage\(extractError\(row\)\)"/.test(src) ||
      /formatExtractErrorMessage\(extractError\(row\)\)/.test(src),
    '提取失败 Tooltip 必须经 formatExtractErrorMessage 转译',
  )
})
