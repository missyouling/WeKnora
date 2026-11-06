import { computed, shallowRef, onBeforeUnmount, type ShallowRef } from 'vue'
import { listKnowledgeFiles } from '@/api/knowledge-base'

/**
 * 第 42 轮：全模块共享的「已上传文件」唯一数据源。
 *
 * 背景：主页面绿色状态条、顶部「历史记录」卡片、右侧「已上传文件」抽屉
 * 原本各自调 listKnowledgeFiles、各自轮询（3s / 4s / 按需 3s），且能耗页
 * 还按当前 Tab 过滤、车队页按当前菜单过滤 —— 于是「抽屉显示正在执行 1，
 * 状态条却不显示」，以及抽屉打开后状态卡在「提取中」不刷新。
 *
 * 这里把文件快照收口成按 scope 共享的响应式数组，并只保留一条全局轮询：
 *   - 任一 scope 绑定后即启动 2.5s 轮询（页面隐藏时跳过）；
 *   - 抽屉打开 / 存在在途文件时保持 2.5s；全部空闲时降到 8s 省请求；
 *   - 每轮返回后同步刷新状态条、抽屉状态标签、顶部「正在执行」计数；
 *   - 在途由「有」变「无」的瞬间广播 settled，各页面据此刷新主列表与 KPI。
 *
 * scope 与业务模块一一对应：energy / invoice / fleet / contract / regulation /
 * award_punish（与 FleetUploadHistoryDrawer 的 scope prop 同口径）。
 */

export interface UploadHistoryFile {
  id: string
  file_name?: string
  title?: string
  parse_status?: string
  created_at?: string
  custom_metadata?: Record<string, any>
  [k: string]: any
}

/** 解析 / 提取终态：这些状态绝不算「正在执行」，也绝不允许自动重提。 */
export const TERMINAL_EXTRACT_STATUSES = new Set([
  'success', 'completed', 'failed', 'manual',
  'not_invoice', 'not_contract', 'not_solar_bill', 'not_utility_bill', 'not_fleet_document',
])

const IN_FLIGHT_PARSE = new Set(['pending', 'parsing', 'processing', 'finalizing', 'queued', 'running', 'extracting'])
const IN_FLIGHT_EXTRACT = new Set(['pending', 'processing', 'extracting', 'queued', 'running'])

/** 统一在途口径：解析中 / 待提取 / 提取中 → true；解析或提取失败 → false。 */
export function isUploadFileRunning(item: any): boolean {
  if (!item) return false
  const ps = String(item.parse_status || '').toLowerCase()
  const es = String(item.custom_metadata?.extract_status ?? item.extract_status ?? '').toLowerCase()
  if (IN_FLIGHT_PARSE.has(ps)) return true
  if (ps === 'failed') return false
  if (ps === 'completed') {
    // 已完成解析但从未提取（待提取）仍算在途；done/failed 等终态不算。
    if (!es) return true
    return IN_FLIGHT_EXTRACT.has(es)
  }
  return IN_FLIGHT_EXTRACT.has(es)
}

export function isTerminalExtractStatus(es: unknown): boolean {
  return TERMINAL_EXTRACT_STATUSES.has(String(es || '').toLowerCase())
}

interface ScopeState {
  getKbId: () => string
  files: ShallowRef<UploadHistoryFile[]>
  bindCount: number
  openCount: number
  lastRunning: number
  lastRefresh: number
  busy: boolean
  settled: Set<() => void>
}

const scopes = new Map<string, ScopeState>()
const SNAPSHOT_CAP = 2000
const ACTIVE_POLL_MS = 2500
const IDLE_POLL_MS = 8000
let timer: ReturnType<typeof setInterval> | null = null

function startTimer() {
  if (timer || typeof window === 'undefined') return
  timer = setInterval(() => {
    if (typeof document !== 'undefined' && document.hidden) return
    for (const [scope, state] of scopes) {
      if (state.bindCount <= 0 || state.busy) continue
      const active = state.openCount > 0 || state.lastRunning > 0
      const gap = active ? ACTIVE_POLL_MS : IDLE_POLL_MS
      if (Date.now() - state.lastRefresh >= gap) void refreshUploadHistory(scope)
    }
  }, ACTIVE_POLL_MS)
}

function stopTimerIfIdle() {
  if (!timer) return
  for (const state of scopes.values()) if (state.bindCount > 0) return
  clearInterval(timer)
  timer = null
}

export function hasUploadHistoryScope(scope: string): boolean {
  const state = scopes.get(scope)
  return !!state && state.files.value.length > 0
}

/** 读取共享快照（其它轮询链路不再各自拉 listKnowledgeFiles）。 */
export function getUploadHistoryFiles(scope: string): UploadHistoryFile[] {
  return scopes.get(scope)?.files.value || []
}

export function runningUploadHistoryFiles(scope: string): UploadHistoryFile[] {
  return getUploadHistoryFiles(scope).filter(isUploadFileRunning)
}

/** 由其它轮询链路（页面/抽屉）把已拿到的快照写回共享源，避免重复请求。 */
export function publishUploadHistoryFiles(scope: string, files: readonly any[]): void {
  const state = scopes.get(scope)
  if (!state || !Array.isArray(files)) return
  state.files.value = files.slice() as UploadHistoryFile[]
  state.lastRefresh = Date.now()
  settleIfNeeded(scope, state)
}

function settleIfNeeded(scope: string, state: ScopeState): void {
  const running = state.files.value.filter(isUploadFileRunning).length
  const prev = state.lastRunning
  state.lastRunning = running
  if (prev > 0 && running === 0) {
    for (const cb of Array.from(state.settled)) {
      try { cb() } catch { /* 单个订阅者失败不影响其它页面 */ }
    }
  }
  void scope
}

/** 拉全量快照并写入共享源（逐页，最多 SNAPSHOT_CAP 条）。 */
export async function refreshUploadHistory(scope: string): Promise<UploadHistoryFile[]> {
  const state = scopes.get(scope)
  if (!state || state.busy) return state?.files.value || []
  const kbId = state.getKbId()
  if (!kbId) return state.files.value
  state.busy = true
  try {
    const collected: any[] = []
    for (let page = 1; ; page++) {
      const res: any = await listKnowledgeFiles(kbId, { page, page_size: 200 })
      const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
      collected.push(...arr)
      const total = Number(res?.total || 0)
      if (arr.length === 0 || collected.length >= total || collected.length >= SNAPSHOT_CAP) break
    }
    state.files.value = collected as UploadHistoryFile[]
    state.lastRefresh = Date.now()
    settleIfNeeded(scope, state)
  } catch {
    // 轮询失败静默降级：保留上一次快照，下一轮重试
  } finally {
    state.busy = false
  }
  return state.files.value
}

/** 页面挂载时绑定模块；多个页面/组件绑定同一 scope 时共享同一份数组。 */
export function bindUploadHistoryScope(scope: string, getKbId: () => string): () => void {
  let state = scopes.get(scope)
  if (!state) {
    state = {
      getKbId,
      files: shallowRef<UploadHistoryFile[]>([]),
      bindCount: 0,
      openCount: 0,
      lastRunning: 0,
      lastRefresh: 0,
      busy: false,
      settled: new Set<() => void>(),
    }
    scopes.set(scope, state)
  }
  state.getKbId = getKbId
  state.bindCount++
  startTimer()
  void refreshUploadHistory(scope)
  return () => {
    state.bindCount--
    if (state.bindCount <= 0 && state.openCount <= 0) {
      scopes.delete(scope)
      stopTimerIfIdle()
    }
  }
}

/** 抽屉打开期间注册（保证 2.5s 高频轮询，关闭后解除）。 */
export function registerUploadHistoryDrawer(scope: string): () => void {
  let state = scopes.get(scope)
  if (!state) {
    // 抽屉可能先于页面就绪：先建一个空 scope，等页面 bind 时补上 kbId。
    state = {
      getKbId: () => '',
      files: shallowRef<UploadHistoryFile[]>([]),
      bindCount: 0,
      openCount: 0,
      lastRunning: 0,
      lastRefresh: 0,
      busy: false,
      settled: new Set<() => void>(),
    }
    scopes.set(scope, state)
    startTimer()
  }
  state.openCount++
  return () => {
    state.openCount--
    if (state.openCount <= 0 && state.bindCount <= 0) {
      scopes.delete(scope)
      stopTimerIfIdle()
    }
  }
}

/** 在途全部落定瞬间回调：主列表 / KPI 卡片在这里刷新一次。 */
export function onUploadHistorySettled(scope: string, cb: () => void): () => void {
  let state = scopes.get(scope)
  if (!state) {
    state = {
      getKbId: () => '', files: shallowRef<UploadHistoryFile[]>([]), bindCount: 0,
      openCount: 0, lastRunning: 0, lastRefresh: 0, busy: false, settled: new Set<() => void>(),
    }
    scopes.set(scope, state)
  }
  state.settled.add(cb)
  return () => state.settled.delete(cb)
}

/** 页面 setup 里一行绑定（自动在卸载时解绑）。 */
export function useUploadHistoryScope(scope: string, getKbId: () => string) {
  const off = bindUploadHistoryScope(scope, getKbId)
  onBeforeUnmount(off)
  return useUploadHistoryFiles(scope)
}

/** 组件内使用：返回共享数组的只读 computed 与派生计数。 */
export function useUploadHistoryFiles(scope: string) {
  const uploadHistoryFiles = computed<UploadHistoryFile[]>(() => scopes.get(scope)?.files.value || [])
  const runningFiles = computed(() => uploadHistoryFiles.value.filter(isUploadFileRunning))
  const runningCount = computed(() => runningFiles.value.length)
  return {
    uploadHistoryFiles,
    runningFiles,
    runningCount,
    refresh: () => refreshUploadHistory(scope),
    onSettled: (cb: () => void) => {
      const off = onUploadHistorySettled(scope, cb)
      onBeforeUnmount(off)
      return off
    },
  }
}
