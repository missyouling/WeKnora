import { ref, onBeforeUnmount, type Ref } from 'vue'
import { listKnowledgeFiles } from '@/api/knowledge-base'
import { isExtractInProgress } from './extractConflict'

export interface BusinessFileItem {
  id: string
  file_name?: string
  title?: string
  parse_status?: string
  custom_metadata?: Record<string, any>
  [k: string]: any
}

/**
 * 模块级共享提取防重状态：
 * 发票/车队/合同/能耗等页面各自的 useBusinessPolling 实例，以及上传弹窗
 * （FleetUploadDialog）的提取队列，全部读写同一份 in-flight/failed。
 * 由此消除"上传弹窗提取、列表轮询提取、手动重新提取"多通道对同一文件
 * 并发提交 → 后端 BusinessExtractHandler.extractMu 防重入 409 冲突。
 */
export const sharedExtractInFlight = new Set<string>()
export const sharedExtractFailed = new Map<string, number>()

// ---- 跨刷新防重 ----
// 模块级 in-flight 是纯内存状态，页面刷新 / Vite HMR 全量 reload 后会清空。
// 此刻后端 extract_status 往往还停在 processing（终态要等模型跑完才落），
// 下一轮 3s 轮询就会认为「没人在途」再提交一次，撞后端 extractMu 的 409。
// 把集合快照进 sessionStorage，模块初始化时回填，即可跨刷新续上防重。
const FLIGHT_STORAGE_KEY = 'weknora-extract-inflight'
// 兜底 TTL：提取最长 30 分钟硬上限，超时快照按失效处理，避免异常残留
// 让某个文件再也无法自动提取。终态到达时仍由 scanOnce 正常回收。
const FLIGHT_TTL_MS = 30 * 60 * 1000

function loadFlightSnapshot(): void {
  try {
    const raw = sessionStorage.getItem(FLIGHT_STORAGE_KEY)
    if (!raw) return
    const snap = JSON.parse(raw) as Record<string, number>
    const now = Date.now()
    for (const [id, at] of Object.entries(snap || {})) {
      if (typeof at === 'number' && now - at < FLIGHT_TTL_MS) sharedExtractInFlight.add(id)
    }
  } catch { /* sessionStorage 不可用（SSR/无痕模式）时静默降级为纯内存防重 */ }
}

/** 把当前 in-flight 集合写入 sessionStorage，供下次加载回填。 */
export function saveFlightSnapshot(): void {
  try {
    const now = Date.now()
    const snap: Record<string, number> = {}
    for (const id of sharedExtractInFlight) snap[id] = now
    sessionStorage.setItem(FLIGHT_STORAGE_KEY, JSON.stringify(snap))
  } catch { /* ignore */ }
}

loadFlightSnapshot()

/**
 * 轮询失败退避上限。后端 503（例如上游数据库连接池打满）时若仍按 3s 死循环
 * 重试，只会把已经过载的后端压得更死，形成「轮询风暴 → 连接池耗尽 → 更多
 * 503」的正反馈。改为指数退避，给后端留出恢复窗口。
 */
export const POLLING_MAX_BACKOFF_MS = 60_000

/**
 * 轮询间隔单次请求的硬超时。
 *
 * 为什么必须有：轮询循环是 `await tick()` 后 setTimeout 排下一轮，只要有一次
 * 请求悬住（后端在跑提取、连接池打满、代理 keep-alive 卡死），`busy` 永远是 true，
 * 整个轮询就冻结——表现是**状态条一直转、永不消失**（真机复现过：账单已提取完成、
 * 库里已是 success，横幅还挂在页面上）。
 * 超时后本轮按失败计（触发指数退避），下一轮重新发请求，状态条即可跟上真实状态。
 */
export const POLLING_REQUEST_TIMEOUT_MS = 15_000

/** 给 promise 套一个硬超时；超时按失败抛出，由调用方计入退避。 */
export function withTimeout<T>(p: Promise<T>, ms = POLLING_REQUEST_TIMEOUT_MS): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('polling request timeout')), ms)
    p.then(
      (v) => { clearTimeout(timer); resolve(v) },
      (e) => { clearTimeout(timer); reject(e) },
    )
  })
}

/** 计算下一轮轮询延迟：无失败用基础间隔，失败按 2^n 退避并封顶。 */
export function pollingBackoffDelay(baseMs: number, failures: number): number {
  if (!Number.isFinite(failures) || failures <= 0) return baseMs
  const factor = Math.pow(2, Math.min(failures, 5))
  return Math.min(baseMs * factor, POLLING_MAX_BACKOFF_MS)
}

export interface UseBusinessPollingOptions {
  /** 当前知识库 ID（ref，可异步就绪） */
  kbId: Ref<string>
  /** 提取接口：每个业务模块不同（extractContract / extractInvoice / ...） */
  extractFn: (kbId: string, fileId: string) => Promise<any>
  /** 每轮把"仍在解析/提取中"的文件回调给业务层（用于顶部状态行） */
  onPendingFiles?: (files: BusinessFileItem[]) => void
  /** 后端判定非本业务并已自动删除时回调（合同模块用于提示+刷新类型下拉） */
  onRemoved?: (item: BusinessFileItem) => void
  /** 每轮扫描完额外的业务行刷新回调（合同刷新聚合行、发票刷新文件行等） */
  onTick?: () => Promise<void> | void
  /**
   * 在途文件由「有」变「无」的回调（整批上传/解析/提取全部结束）。
   *
   * 为什么必须有：账单页此前只靠 `onTick` 每 3s 拉一次列表，而光伏页干脆
   * 只在自己触发提取时刷新 —— 用户在别处（上传弹窗）完成提取后，列表永远
   * 停在旧数据上，得手动刷新（真机：解析提取都成功，列表里没有新账单）。
   * 这里给出唯一的「全流程结束」信号，各业务页接上即可自动刷新列表与卡片计数。
   */
  onSettled?: () => Promise<void> | void
  /** 轮询间隔，默认 3s */
  intervalMs?: number
  /** 文件过滤器：只处理匹配的文件（能耗模块按 bill_kind 区分电费/光伏） */
  filter?: (item: BusinessFileItem) => boolean
}

/**
 * 日常事务业务模块通用轮询：
 * 1) 拉取知识库文件列表；
 * 2) 识别 parse_status=completed 且未提取的文件，调用 extractFn；
 * 3) extractInFlight / extractFailed 防重；
 * 4) 把 pending 文件回调给业务层渲染状态行。
 */
export function useBusinessPolling(opts: UseBusinessPollingOptions) {
  const { kbId, extractFn, onPendingFiles, onRemoved, onTick, onSettled, intervalMs = 3000, filter } = opts

  const extractInFlight = ref<Set<string>>(sharedExtractInFlight)
  // 失败记录：id -> 首次失败时间戳。5 分钟后允许重试，避免一次网络抖动永久拉黑
  const extractFailed = ref<Map<string, number>>(sharedExtractFailed)
  const RETRY_AFTER_MS = 5 * 60 * 1000
  const pendingFiles = ref<BusinessFileItem[]>([])

  let timer: ReturnType<typeof setTimeout> | null = null
  let busy = false
  // 僵尸在途兜底：id -> 首次进入「进行中」的时间戳。
  // 后端任务若在重启/超时中被丢弃，extract_status 会永远停在 processing，
  // 状态条就一直显示「正在提取…」不消失（真机曾挂到 11 分 3 秒）。
  const PENDING_STALE_MS = 15 * 60 * 1000
  const pendingSince = new Map<string, number>()
  /** 上一轮在途数量：用于识别「由有变无」的整批完成时刻 */
  let lastPendingCount = 0
  // 整轮（扫描 + onTick 刷新列表）在途锁。此前只有 scanOnce 有 busy 护栏，
  // onTick（发票页 loadFiles）没有——列表接口一旦超过 intervalMs，每 3s 就会
  // 新起一轮 onTick，请求层层叠加堆积，最终压垮后端连接池。
  let tickBusy = false
  let running = false
  /** 连续失败轮数，驱动指数退避；任一轮成功即归零。 */
  const consecutiveFailures = ref(0)

  const scanOnce = async (): Promise<boolean> => {
    if (!kbId.value || busy) return true
    busy = true
    let ok = true
    try {
      const res: any = await withTimeout(listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 }))
      const data = res?.data || res?.list || []
      const arr: BusinessFileItem[] = Array.isArray(data) ? data : []
      const needExtract: BusinessFileItem[] = []
      const pend: BusinessFileItem[] = []

      for (const item of arr) {
        if (filter && !filter(item)) continue
        const ps = item.parse_status
        if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') {
          pend.push(item)
          continue
        }
        if (ps === 'completed') {
          const es = item.custom_metadata?.extract_status
          const failedAt = extractFailed.value.get(item.id)
          const canRetry = !failedAt || Date.now() - failedAt > RETRY_AFTER_MS
          // 只对「从未提取（空）或已入队待处理（pending）」提交；
          // es=processing 是后端 worker 正在提取的过程态（extractMu 已持锁），
          // 前端再次提交必然 409——processing 只进下方 pend 状态条展示，等终态。
          // 只对「从未提取」（extract_status 为空）的文件自动投递。
          //
          // 旧逻辑把 es='pending'（已入队、后端 worker 会处理）也算作待提取，
          // 而 in-flight 集合是模块级内存状态、刷新页面即重置——于是每次刷新都会
          // 对 pending 文件重新提交一次，用户观感是"刷新就重新提取、永远提取不完"。
          // pending 的实际含义是"已入队"，前端不应替后端重投。
          if (!es && !extractInFlight.value.has(item.id) && canRetry) {
            needExtract.push(item)
          }
          if (es && es !== 'pending' && es !== 'processing') extractFailed.value.delete(item.id)
          // 提取失败（前端本地 extractFailed 命中）的文件不进状态条：后端失败已落库
          // failed 或仍在 processing 残留，状态条都不再显示「正在提取」，与状态列一致
          if ((!es || es === 'pending' || es === 'processing') && !extractFailed.value.has(item.id)) pend.push(item)
        }
      }

      // 僵尸在途清理：超过 15 分钟仍在 pending/processing 的文件，判定后端任务已丢失
      // （重启/超时/任务被丢弃），强制移出状态条并记一次本地失败，避免永久转圈。
      const nowTs = Date.now()
      const liveIds = new Set(pend.map((f) => f.id))
      for (const id of Array.from(pendingSince.keys())) {
        if (!liveIds.has(id)) pendingSince.delete(id)
      }
      const staleIds: string[] = []
      for (const f of pend) {
        const seenAt = pendingSince.get(f.id)
        if (seenAt == null) {
          pendingSince.set(f.id, nowTs)
          continue
        }
        if (nowTs - seenAt > PENDING_STALE_MS) staleIds.push(f.id)
      }
      let visiblePend = pend
      if (staleIds.length) {
        const staleSet = new Set(staleIds)
        visiblePend = pend.filter((f) => !staleSet.has(f.id))
        for (const id of staleIds) {
          pendingSince.delete(id)
          if (!extractFailed.value.has(id)) extractFailed.value.set(id, nowTs)
        }
      }

      const prevPendingCount = lastPendingCount
      lastPendingCount = visiblePend.length
      pendingFiles.value = visiblePend
      onPendingFiles?.(visiblePend)
      // 整批完成（上一轮还有在途、这一轮清零）：通知业务页刷新列表与卡片计数。
      // 放在 onTick 之前，保证「列表里能立刻看到刚提取完的文件」。
      if (prevPendingCount > 0 && visiblePend.length === 0 && onSettled) {
        try {
          await onSettled()
        } catch { /* 刷新失败不影响轮询节奏，下轮 onTick 仍会兜住 */ }
      }

      for (const item of needExtract.slice(0, 5)) {
        extractInFlight.value.add(item.id)
        let conflict = false
        let succeeded = false
        try {
          const r: any = await extractFn(kbId.value, item.id)
          succeeded = true
          if (r?.data?.removed) onRemoved?.(item)
        } catch (e: any) {
          // 409：后端 extractMu 判定该文件正被其他请求提取（如手动重新提取持锁、
          // 另一标签在途），属并发冲突而非提取失败——不写 extractFailed（否则状态列
          // 误报「提取失败」），并保留 in-flight 让下轮扫描不再重复提交打 409。
          conflict = isExtractInProgress(e)
          if (!conflict && !extractFailed.value.has(item.id)) {
            extractFailed.value.set(item.id, Date.now())
          }
        } finally {
          // asynq 化后 POST 200 仅代表入队成功，不代表提取完成：
          // 成功入队/409 都保留 in-flight，由下方「终态清残留」在 es 到
          // success/failed 时统一回收——若此处提前删除，下一轮扫描看到
          // es=processing 且不在 in-flight 会重复提交 → 后端 extractMu 409。
          // 仅真实失败（非 409 且未成功）删除 in-flight，允许下轮按
          // extractFailed 5 分钟退避重试；否则该文件会因 in-flight 残留
          // 永久无法再提取。
          if (!succeeded && !conflict) extractInFlight.value.delete(item.id)
        }
      }
      // 清理 in-flight 残留（三种情况都清）：
      //   1. es 已到终态（success/failed/not_x/manual）→ 提取确实结束；
      //   2. es 仍是 pending/processing → 后端在跑，**保留**（删了会重复提交撞 409）；
      //   3. es 为空 → 后端连"已入队"都没记录，说明任务丢了（worker 重启/队列丢失）。
      //      旧实现只在终态清理，这种文件会永久留在 in-flight：状态条一直转圈、
      //      而且因为 in-flight 存在，前端再也不会重新提交它（僵尸在途）。
      //      重复提交的风险由后端 extractMu 兜住（409 → conflict，保持 in-flight）。
      for (const id of Array.from(extractInFlight.value)) {
        const it = arr.find(k => k.id === id)
        const es = it?.custom_metadata?.extract_status
        if (!es || (es !== 'pending' && es !== 'processing')) extractInFlight.value.delete(id)
      }
    } catch {
      // 不抛给外层：本轮失败只用于驱动退避，下轮继续（旧行为是纯静默，
      // 会把 503 风暴藏起来且永不降频）。
      ok = false
    } finally {
      // 每轮落盘：刷新后模块级 in-flight 能被回填，避免重复提交撞 409
      saveFlightSnapshot()
      busy = false
    }
    return ok
  }

  /**
   * 一轮完整轮询：扫描文件 + 业务侧刷新（onTick）。
   * 返回本轮是否整体成功，供调度器决定退避。
   */
  const tick = async (): Promise<boolean> => {
    // document 在 SSR / Node 单测环境不存在，必须显式判空再取 hidden。
    if (!kbId.value || (typeof document !== 'undefined' && document.hidden)) return true
    // 上一轮尚未结束：直接跳过本轮，绝不叠加并发请求。
    if (tickBusy) return true
    tickBusy = true
    try {
      const scanOk = await scanOnce()
      let refreshOk = true
      if (onTick) {
        try {
          await onTick()
        } catch {
          refreshOk = false
        }
      }
      const ok = scanOk && refreshOk
      // 退避计数内聚在 tick 里：只要一轮真实执行过，就按结果记成功/失败，
      // 调度器只负责读取，避免"跳过轮次"被误计为失败。
      consecutiveFailures.value = ok
        ? 0
        : Math.min(consecutiveFailures.value + 1, 6)
      return ok
    } finally {
      tickBusy = false
    }
  }

  // 当前生效的轮询间隔：空闲时业务层可调到 15s，一旦有新任务立刻回到 3s。
  // 用 ref 而非闭包常量，才能在不重启定时器的情况下改变下一轮延迟。
  const activeInterval = ref(intervalMs)
  let loopFn: (() => Promise<void>) | null = null

  const start = () => {
    if (running) return
    running = true
    const loop = async () => {
      await tick()
      if (!running) return
      timer = setTimeout(loop, pollingBackoffDelay(activeInterval.value, consecutiveFailures.value))
    }
    loopFn = loop
    timer = setTimeout(loop, activeInterval.value)
  }
  const stop = () => {
    running = false
    loopFn = null
    if (timer) { clearTimeout(timer); timer = null }
  }

  /**
   * 调整轮询间隔（空闲降频 / 有任务回升）。
   *
   * 语义：只影响**下一轮**的排期。为避免"刚设了 15s 又要等 15s 才响应新任务"，
   * 在运行中且当前挂起的等待比新间隔更长时，会立即重排一次。
   */
  const setPollingInterval = (ms: number) => {
    const next = Math.max(1000, Math.floor(ms))
    if (next === activeInterval.value) return
    activeInterval.value = next
    if (!running || !timer || !loopFn) return
    clearTimeout(timer)
    timer = setTimeout(loopFn, pollingBackoffDelay(next, consecutiveFailures.value))
  }

  onBeforeUnmount(stop)

  return {
    extractInFlight, extractFailed, pendingFiles, consecutiveFailures, activeInterval,
    start, stop, scanOnce, tick, setPollingInterval,
  }
}
