import assert from 'node:assert/strict'
import test from 'node:test'
import { register } from 'node:module'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, join } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

// Type declaration for test mock
declare global {
  // eslint-disable-next-line no-var
  var __mockFiles: any[] | undefined
}

// Mock @/api/knowledge-base via a custom ESM loader
// Files are shared via globalThis.__mockFiles
register(pathToFileURL(join(__dirname, 'test-loader.mjs')).href)

// Import after registering the loader
const { useBusinessPolling } = await import('./useBusinessPolling.ts')
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
