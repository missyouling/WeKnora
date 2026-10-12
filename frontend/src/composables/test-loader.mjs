// ESM loader to mock @/api/knowledge-base for useBusinessPolling tests
export async function resolve(specifier, context, nextResolve) {
  if (specifier === '@/api/knowledge-base') {
    return {
      url: new URL('./test-mock-api.mjs', import.meta.url).href,
      shortCircuit: true,
    }
  }
  return nextResolve(specifier, context)
}
