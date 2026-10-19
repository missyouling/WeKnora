// 业务分类字典（`listFleetCategories`）的进程内缓存。
//
// 为什么需要：设置抽屉（FleetCertPanel / ExtractRulePanel）每次打开都会为每个
// scope 各发一次 `listFleetCategories`，而发票页切换类型时也会再发一次；
// 分类是低频变更的配置数据，重复请求纯属浪费。更关键的是：Supabase 跨区
// pooler 下单次往返约 350ms，一次抽屉打开能叠加出 4~5 次往返。
//
// 策略：
//   - TTL 30s，同 scope 命中直接返回；
//   - 并发去重：同 scope 的第二次调用复用同一个 Promise（不是再发一次）；
//   - 任意写操作后由调用方 `invalidate(scope)`，或全局广播
//     `fleet-categories-changed` 自动失效，保证"改了立刻看到"。
import { ref } from 'vue'
import { listFleetCategories } from '@/api/fleet'

const TTL_MS = 30_000

interface CacheEntry {
  at: number
  data: any[]
}

const cache = new Map<string, CacheEntry>()
const inflight = new Map<string, Promise<any[]>>()
/** 缓存代际：失效时自增，供需要感知变化的调用方 watch。 */
const generation = ref(0)

function normalize(res: any): any[] {
  if (Array.isArray(res)) return res
  if (Array.isArray(res?.data)) return res.data
  return []
}

async function load(scope: string): Promise<any[]> {
  const res: any = await listFleetCategories({ scope })
  const data = normalize(res)
  cache.set(scope, { at: Date.now(), data })
  return data
}

/**
 * 取某 scope 的分类列表。
 * @param scope 业务 scope（invoice / contract / regulation / award_punish / vehicle…）
 * @param force 跳过缓存强制刷新（保存后需要立即可见时用）
 */
async function list(scope: string, force = false): Promise<any[]> {
  if (!force) {
    const hit = cache.get(scope)
    if (hit && Date.now() - hit.at < TTL_MS) return hit.data
    const pending = inflight.get(scope)
    if (pending) return pending
  }
  const p = load(scope)
  inflight.set(scope, p)
  try {
    return await p
  } finally {
    inflight.delete(scope)
  }
}

/** 失效缓存：不传 scope 清空全部。 */
function invalidate(scope?: string): void {
  if (scope) cache.delete(scope)
  else cache.clear()
  generation.value++
}

// 任何地方改了分类 → 全局事件广播 → 缓存立即失效，
// 下一次读取必然回源，避免"设置里改了、列表还是旧的"。
if (typeof window !== 'undefined') {
  window.addEventListener('fleet-categories-changed', () => invalidate())
}

export function useExtractCatalog() {
  return { list, invalidate, generation }
}
