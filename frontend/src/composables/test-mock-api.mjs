// Mock for @/api/knowledge-base - shares mockFiles via global
//
// __mockFilesError 非空时抛错，用于模拟后端 503（连接池打满）等故障，
// 便于验证前端轮询是否按预期退避而不是疯狂重试。
export async function listKnowledgeFiles(_kbId, _params) {
  if (globalThis.__mockFilesError) {
    throw globalThis.__mockFilesError
  }
  const files = globalThis.__mockFiles || []
  return { data: files }
}
