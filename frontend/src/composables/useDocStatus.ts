import { computed, type Ref } from 'vue'

export interface DocStatusInfo {
  label: string
  theme: 'default' | 'primary' | 'success' | 'warning' | 'danger'
  icon?: string
  spin?: boolean
}

export interface DocRowLike {
  parseStatus?: string
  extractStatus?: string
  knowledgeId?: string | number
  tags?: Array<{ id?: string | number; name?: string }>
}

interface Options {
  /** 本地轮询中的 knowledgeId 集合（来自 useBusinessPolling） */
  extractInFlight: Ref<Set<string>>
  /** 本地轮询失败的 knowledgeId 集合（id -> 首次失败时间戳，支持冷却后重试） */
  extractFailed: Ref<Map<string, number>>
  /** 当前详情抽屉行（用于 summaryState） */
  currentDetail: Ref<{ summary_status?: string } | null>
  /** 业务尾缀，如 contract / invoice / regulation / award_punish / energy */
  scope: 'contract' | 'invoice' | 'regulation' | 'award_punish' | 'energy'
  /** "非XX" 文案，如 "非合同" / "非发票" */
  notLabel: string
  /**
   * 是否以「后端已落库的 success」为最高优先级（发票页面启用）。
   *
   * 背景：文件级提取状态在重提期间会在 pending/processing/failed 之间波动，
   * 而单张票的字段早已完整落库。开启后，只要记录本身 extract_status=success
   * 就恒显示「提取成功」，不再被打回「提取中/待提取」（真机第 7~10 行状态跳变）。
   */
  preferRecordedSuccess?: boolean
}

export function useDocStatus(opts: Options) {
  const extractStatusOf = (row: DocRowLike): string => {
    const ps = row.parseStatus
    if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return 'parsing'
    if (ps === 'failed') return 'parse_failed'
    if (ps === 'completed') {
      // 行级 success 优先于文件级在途状态（见 preferRecordedSuccess 注释）
      if (opts.preferRecordedSuccess && row.extractStatus === 'success') return 'success'
      if (row.knowledgeId != null && opts.extractInFlight.value.has(String(row.knowledgeId))) return 'processing'
      const es = row.extractStatus
      // 前端本地失败记录（extractFailed）仅在「后端尚无终态」时覆盖展示：
      // 后端已明确 success/failed/not_x/manual 时以落库状态为权威，避免
      // 轮询取数范围（前 100 条）外的文件失败记录永不清理、误报「提取失败」。
      if (row.knowledgeId != null && opts.extractFailed.value.has(String(row.knowledgeId)) &&
          (!es || es === 'pending' || es === 'processing')) return 'failed'
      if (!es || es === 'pending' || es === 'processing') return 'pending'
      if (es === 'success') return 'success'
      if (es === 'failed') return 'failed'
      if (es === `not_${opts.scope}`) return 'not_x'
      if (es === 'manual') return 'manual'
    }
    return ''
  }

  const statusOf = (row: DocRowLike): DocStatusInfo => {
    const s = extractStatusOf(row)
    switch (s) {
      case 'parsing': return { label: '解析中', theme: 'primary', icon: 'loading', spin: true }
      case 'parse_failed': return { label: '解析失败', theme: 'danger', icon: 'close-circle' }
      case 'pending': return { label: '待提取', theme: 'primary', icon: 'loading', spin: true }
      case 'processing': return { label: '提取中', theme: 'primary', icon: 'loading', spin: true }
      case 'success': return { label: '提取成功', theme: 'success' }
      case 'failed': return { label: '提取失败', theme: 'danger', icon: 'close-circle' }
      case 'not_x': return { label: opts.notLabel, theme: 'default' }
      case 'manual': return { label: '待补录', theme: 'warning', icon: 'edit-1' }
      default: return { label: '待解析', theme: 'default' }
    }
  }

  const summaryState = computed<DocStatusInfo | null>(() => {
    const ss = opts.currentDetail.value?.summary_status
    if (!ss) return null
    if (ss === 'pending' || ss === 'processing') return { label: '摘要生成中', theme: 'primary', icon: 'loading', spin: true }
    if (ss === 'completed') return { label: '摘要已生成', theme: 'success' }
    if (ss === 'failed') return { label: '摘要生成失败', theme: 'danger' }
    return null
  })

  const rowTags = (row: DocRowLike): any[] => {
    const arr = row.tags || []
    return Array.isArray(arr) ? arr : []
  }

  return { extractStatusOf, statusOf, summaryState, rowTags }
}
