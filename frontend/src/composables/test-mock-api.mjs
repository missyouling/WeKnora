// Mock for @/api/knowledge-base - shares mockFiles via global
export async function listKnowledgeFiles(kbId, params) {
  const files = globalThis.__mockFiles || []
  return { data: files }
}
