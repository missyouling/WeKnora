import { ref, onBeforeUnmount, type Ref } from 'vue'
import { listKnowledgeFiles } from '@/api/knowledge-base'

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
  const { kbId, extractFn, onPendingFiles, onRemoved, onTick, intervalMs = 3000, filter } = opts

  const extractInFlight = ref<Set<string>>(sharedExtractInFlight)
  // 失败记录：id -> 首次失败时间戳。5 分钟后允许重试，避免一次网络抖动永久拉黑
  const extractFailed = ref<Map<string, number>>(sharedExtractFailed)
  const RETRY_AFTER_MS = 5 * 60 * 1000
  const pendingFiles = ref<BusinessFileItem[]>([])

  let timer: ReturnType<typeof setInterval> | null = null
  let busy = false

  const scanOnce = async () => {
    if (!kbId.value || busy) return
    busy = true
    try {
      const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
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
          if ((!es || es === 'pending') &&
              !extractInFlight.value.has(item.id) && canRetry) {
            needExtract.push(item)
          }
          if (es && es !== 'pending' && es !== 'processing') extractFailed.value.delete(item.id)
          // 提取失败（前端本地 extractFailed 命中）的文件不进状态条：后端失败已落库
          // failed 或仍在 processing 残留，状态条都不再显示「正在提取」，与状态列一致
          if ((!es || es === 'pending' || es === 'processing') && !extractFailed.value.has(item.id)) pend.push(item)
        }
      }

      pendingFiles.value = pend
      onPendingFiles?.(pend)

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
          conflict = (e as any)?.$httpStatus === 409
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
      // 清理 in-flight 残留：仅当后端 es 已到终态（success/failed）才移除——
      // es=processing 是提取请求自己写的过程态，请求仍在途，删除会触发下轮
      // 重复提交（→409）；终态才说明该文件提取确实结束。
      for (const id of Array.from(extractInFlight.value)) {
        const it = arr.find(k => k.id === id)
        const es = it?.custom_metadata?.extract_status
        if (es && es !== 'pending' && es !== 'processing') extractInFlight.value.delete(id)
      }
    } catch { /* 静默，下轮重试 */ }
    finally { busy = false }
  }

  const tick = async () => {
    if (!kbId.value || document.hidden) return
    await scanOnce()
    if (onTick) await onTick()
  }

  const start = () => {
    if (timer) return
    timer = setInterval(async () => { await tick() }, intervalMs)
  }
  const stop = () => {
    if (timer) { clearInterval(timer); timer = null }
  }

  onBeforeUnmount(stop)

  return { extractInFlight, extractFailed, pendingFiles, start, stop, scanOnce }
}
