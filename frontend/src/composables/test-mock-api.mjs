// Mock for @/api/knowledge-base - shares mockFiles via global
export async function listKnowledgeFiles(_kbId, _params) {
  const files = globalThis.__mockFiles || []
  return { data: files }
}
