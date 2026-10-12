import assert from 'node:assert/strict'
import test from 'node:test'

// 测试 document-preview.vue 中 loadPreview 的 in-flight guard 逻辑
// 场景：openDetail 先设 currentRow，getKnowledgeDetails 返回后又设一次，
// 若第二次发生在第一次的 getKnowledgePreviewUrl 等待期间，
// 必须只发起一次网络请求，而不是两次（两次会拿到不同的签名 URL 导致闪烁）。

test('in-flight guard: concurrent loadPreview for same key makes only one request', async () => {
  let requestCount = 0
  let loadedForId = ''
  let loadingForKey = ''
  let loadingPromise: Promise<void> | null = null

  const getPreviewSourceKey = () => 'knowledge:file-123'

  async function doLoadPreview(sourceKey: string): Promise<void> {
    requestCount++
    // 模拟 getKnowledgePreviewUrl 的网络延迟
    await new Promise(resolve => setTimeout(resolve, 50))
    loadedForId = sourceKey
  }

  async function loadPreview() {
    const sourceKey = getPreviewSourceKey()
    if (!sourceKey) return
    if (loadedForId === sourceKey) return
    if (loadingForKey === sourceKey && loadingPromise) {
      await loadingPromise
      return
    }
    loadingForKey = sourceKey
    loadingPromise = doLoadPreview(sourceKey)
    try {
      await loadingPromise
    } finally {
      if (loadingForKey === sourceKey) {
        loadingForKey = ''
        loadingPromise = null
      }
    }
  }

  // 模拟：两次 loadPreview 并发调用（第二次在第一次的网络等待期间触发）
  await Promise.all([loadPreview(), loadPreview()])

  assert.equal(requestCount, 1, '只应发起一次网络请求（in-flight guard 生效）')
  assert.equal(loadedForId, 'knowledge:file-123', '加载应正常完成')
  assert.equal(loadingForKey, '', '加载完成后应清理 in-flight 状态')
})

test('in-flight guard: sequential loads for same key use cache (no re-request)', async () => {
  let requestCount = 0
  let loadedForId = ''

  const getPreviewSourceKey = () => 'knowledge:file-456'

  async function doLoadPreview(sourceKey: string): Promise<void> {
    requestCount++
    await new Promise(resolve => setTimeout(resolve, 10))
    loadedForId = sourceKey
  }

  async function loadPreview() {
    const sourceKey = getPreviewSourceKey()
    if (!sourceKey) return
    if (loadedForId === sourceKey) return
    await doLoadPreview(sourceKey)
  }

  await loadPreview()
  await loadPreview() // 第二次应命中 loadedForId 缓存，不重新请求

  assert.equal(requestCount, 1, '第二次应命中缓存，不重新请求')
})

test('in-flight guard: different keys load independently', async () => {
  let requestCount = 0
  let loadedForId = ''
  let loadingForKey = ''
  let loadingPromise: Promise<void> | null = null
  let currentKey = 'knowledge:file-a'

  const getPreviewSourceKey = () => currentKey

  async function doLoadPreview(sourceKey: string): Promise<void> {
    requestCount++
    await new Promise(resolve => setTimeout(resolve, 20))
    loadedForId = sourceKey
  }

  async function loadPreview() {
    const sourceKey = getPreviewSourceKey()
    if (!sourceKey) return
    if (loadedForId === sourceKey) return
    if (loadingForKey === sourceKey && loadingPromise) {
      await loadingPromise
      return
    }
    loadingForKey = sourceKey
    loadingPromise = doLoadPreview(sourceKey)
    try {
      await loadingPromise
    } finally {
      if (loadingForKey === sourceKey) {
        loadingForKey = ''
        loadingPromise = null
      }
    }
  }

  // 先加载 file-a
  await loadPreview()
  assert.equal(requestCount, 1)

  // 切换到 file-b，应重新加载（不同的 key 不受 guard 阻挡）
  currentKey = 'knowledge:file-b'
  await loadPreview()
  assert.equal(requestCount, 2, '不同 key 应独立加载')
})
