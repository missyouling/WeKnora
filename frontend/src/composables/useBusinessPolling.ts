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
}

/**
 * 日常事务业务模块通用轮询：
 * 1) 拉取知识库文件列表；
 * 2) 识别 parse_status=completed 且未提取的文件，调用 extractFn；
 * 3) extractInFlight / extractFailed 防重；
 * 4) 把 pending 文件回调给业务层渲染状态行。
 */
export function useBusinessPolling(opts: UseBusinessPollingOptions) {
  const { kbId, extractFn, onPendingFiles, onRemoved, onTick, intervalMs = 3000 } = opts

  const extractInFlight = ref<Set<string>>(new Set())
  const extractFailed = ref<Set<string>>(new Set())
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
        const ps = item.parse_status
        if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') {
          pend.push(item)
          continue
        }
        if (ps === 'completed') {
          const es = item.custom_metadata?.extract_status
          if ((!es || es === 'pending' || es === 'processing') &&
              !extractInFlight.value.has(item.id) && !extractFailed.value.has(item.id)) {
            needExtract.push(item)
          }
          if (es && es !== 'pending' && es !== 'processing') extractFailed.value.delete(item.id)
          if (!es || es === 'pending' || es === 'processing') pend.push(item)
        }
      }

      pendingFiles.value = pend
      onPendingFiles?.(pend)

      for (const item of needExtract.slice(0, 5)) {
        extractInFlight.value.add(item.id)
        try {
          const r: any = await extractFn(kbId.value, item.id)
          if (r?.data?.removed) onRemoved?.(item)
        } catch {
          extractFailed.value.add(item.id)
        }
      }
      for (const id of Array.from(extractInFlight.value)) {
        const it = arr.find(k => k.id === id)
        if (it?.custom_metadata?.extract_status) extractInFlight.value.delete(id)
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
