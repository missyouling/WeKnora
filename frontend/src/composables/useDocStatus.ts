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
  /** 本地轮询失败的 knowledgeId 集合 */
  extractFailed: Ref<Set<string>>
  /** 当前详情抽屉行（用于 summaryState） */
  currentDetail: Ref<{ summary_status?: string } | null>
  /** 业务尾缀，如 contract / invoice / regulation / award_punish */
  scope: 'contract' | 'invoice' | 'regulation' | 'award_punish'
  /** "非XX" 文案，如 "非合同" / "非发票" */
  notLabel: string
}

export function useDocStatus(opts: Options) {
  const extractStatusOf = (row: DocRowLike): string => {
    const ps = row.parseStatus
    if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return 'parsing'
    if (ps === 'failed') return 'parse_failed'
    if (ps === 'completed') {
      if (row.knowledgeId != null && opts.extractInFlight.value.has(String(row.knowledgeId))) return 'processing'
      if (row.knowledgeId != null && opts.extractFailed.value.has(String(row.knowledgeId))) return 'failed'
      const es = row.extractStatus
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
