import assert from 'node:assert/strict'
import test from 'node:test'
import { register } from 'node:module'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Type declaration for test mock
declare global {
  var __mockFiles: any[] | undefined
  var __mockFilesError: any
}

// Mock @/api/knowledge-base via a custom ESM loader
// Files are shared via globalThis.__mockFiles
register(pathToFileURL(join(__dirname, 'test-loader.mjs')).href)

// Import after registering the loader
const { useBusinessPolling, pollingBackoffDelay, POLLING_MAX_BACKOFF_MS } = await import('./useBusinessPolling.ts')
const { ref } = await import('vue')

test('failed extraction does not deadlock: extractInFlight cleared, retry allowed after cooldown', async (t) => {
  // Setup: one file with parse completed, no extract_status
  globalThis.__mockFiles = [{
    id: 'file-1',
    file_name: 'test.pdf',
    parse_status: 'completed',
    custom_metadata: {},
  }]

  let extractCalls = 0
  const kbId = ref('kb-1')
  
  const { extractInFlight, extractFailed, scanOnce } = useBusinessPolling({
    kbId,
    extractFn: async (kid, fileId) => {
      extractCalls++
      if (extractCalls === 1) {
        throw new Error('simulated network timeout')
      }
      return { data: { success: true } }
    },
  })

  // First scan: extraction fails
  await scanOnce()
  
  // After fix: extractInFlight must be empty (no deadlock)
  assert.equal(extractInFlight.value.size, 0,
    'extractInFlight should be empty after failed extraction (deadlock fix)')
  assert.equal(extractFailed.value.has('file-1'), true,
    'extractFailed should track the failure for backoff')
  assert.equal(extractCalls, 1, 'extractFn should have been called once')

  // Second scan (immediately): should NOT retry due to 5-min cooldown
  await scanOnce()
  assert.equal(extractCalls, 1, 'should not retry immediately (5-min cooldown)')

  // Simulate 6 minutes passing
  extractFailed.value.set('file-1', Date.now() - 6 * 60 * 1000)
  await scanOnce()
  assert.equal(extractCalls, 2, 'should retry after cooldown expires')
  assert.equal(extractInFlight.value.size, 0,
    'extractInFlight should be empty after successful extraction')
})

test('successful extraction clears extractInFlight', async () => {
  globalThis.__mockFiles = [{
    id: 'file-2',
    file_name: 'test2.pdf',
    parse_status: 'completed',
    custom_metadata: {},
  }]

  const kbId = ref('kb-1')
  const { extractInFlight, scanOnce } = useBusinessPolling({
    kbId,
    extractFn: async () => ({ data: { success: true } }),
  })

  await scanOnce()
  assert.equal(extractInFlight.value.size, 0,
    'extractInFlight should be empty after successful extraction')
})

// ---------------------------------------------------------------------------
// 503 退避 + 单轮不叠加
//
// 事故：发票页按车牌查询时两条接口同时 503。日志确认真因是上游 Supabase
// session pooler 被打满（pool_size 15），但前端 3s 固定间隔 + onTick 无护栏
// 会把失败放大成"轮询风暴"，让后端更难恢复。
// ---------------------------------------------------------------------------

test('pollingBackoffDelay：无失败用基础间隔，失败按 2^n 退避并封顶', () => {
  assert.equal(pollingBackoffDelay(3000, 0), 3000)
  assert.equal(pollingBackoffDelay(3000, 1), 6000)
  assert.equal(pollingBackoffDelay(3000, 2), 12000)
  assert.equal(pollingBackoffDelay(3000, 3), 24000)
  assert.equal(pollingBackoffDelay(3000, 4), 48000)
  assert.equal(pollingBackoffDelay(3000, 5), POLLING_MAX_BACKOFF_MS)
  assert.equal(pollingBackoffDelay(3000, 99), POLLING_MAX_BACKOFF_MS,
    '连续失败不得无限增长，必须封顶')
  assert.equal(pollingBackoffDelay(3000, -1), 3000, '异常值按无失败处理')
})

test('tick 在途时再次调用必须直接跳过，绝不并发叠加请求', async () => {
  globalThis.__mockFiles = []
  delete globalThis.__mockFilesError

  let onTickCalls = 0
  let release: (() => void) | null = null
  const gate = new Promise<void>((r) => { release = r })

  const { tick } = useBusinessPolling({
    kbId: ref('kb-1'),
    extractFn: async () => ({ data: {} }),
    onTick: async () => {
      onTickCalls++
      await gate // 模拟"列表接口比轮询间隔还慢"
    },
  })

  const first = tick()
  // 第一轮还卡在 onTick 里，此时再来两轮
  const second = await tick()
  const third = await tick()
  assert.equal(second, true, '在途期间的重入应视为安全跳过')
  assert.equal(third, true, '在途期间的重入应视为安全跳过')

  release!()
  await first
  assert.equal(onTickCalls, 1, '同一时间只允许一轮 onTick 在飞')
})

test('扫描失败（模拟 503）累计连续失败次数，成功后归零', async () => {
  globalThis.__mockFiles = []
  const { tick, consecutiveFailures } = useBusinessPolling({
    kbId: ref('kb-1'),
    extractFn: async () => ({ data: {} }),
  })

  globalThis.__mockFilesError = { status: 503, message: 'Service Unavailable' }
  assert.equal(await tick(), false, '503 应被判定为失败轮')
  assert.equal(consecutiveFailures.value, 1)
  assert.equal(await tick(), false)
  assert.equal(consecutiveFailures.value, 2)

  delete globalThis.__mockFilesError
  assert.equal(await tick(), true, '恢复后本轮成功')
  assert.equal(consecutiveFailures.value, 0, '成功必须把退避计数归零')
})
