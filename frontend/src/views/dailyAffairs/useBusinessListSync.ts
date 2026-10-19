// useBusinessListSync —— 列表「源文件 ↔ 列表行」同步与删除编排（蓝图 P1-6）。
//
// 抽出来的三件事，都是"看起来简单、写错就静默丢数据"的逻辑：
//   1. 乐观移除：删除接口只代表异步入队，等后端落库再刷新会让行"删了又回来"，
//      必须先在本地按「整文件 / 单页定位键」摘掉对应行；
//   2. 延迟对账：worker 调度有抖动（常规 800ms、兜底 2600ms），两次延迟刷新
//      取权威结果，既不会闪回也不会漏删；
//   3. 级联顺序：同文件多页删除必须**先大页码后小页码**（后端删完会重排页码，
//      从小到大删会把第二张顶到已删的页码上）。
//
// 业务差异全部走回调注入（deletePage / deleteFiles / reload），本文件不认识
// 任何业务 scope：发票传 deleteInvoicePage，未来的合同传自己的删除接口即可。

export interface SyncRow {
  knowledgeId?: string
  /** 业务定位键（发票=票号序号；无页码的历史数据等价整份删除） */
  page?: number
}

export interface SyncErrorContext {
  knowledgeId?: string
  page?: number
}

export interface ListSyncOptions {
  /** 权威对账：重新拉取列表（silent=true 表示不显示 loading 态） */
  reload: (silent?: boolean) => void | Promise<void>
  /** 概览/KPI 刷新（与列表同一节奏；可选） */
  reloadStats?: () => void
  /** 按定位键删除单条；返回体带 deleted_file 表示整份文件已随之删除 */
  deletePage?: (knowledgeId: string, page: number) => Promise<any>
  /** 整份文件批量删除 */
  deleteFiles?: (ids: string[]) => Promise<any>
  /** 单条删除失败回调：交给页面决定文案（中台不写业务话术） */
  onError?: (e: any, ctx: SyncErrorContext) => void
  /** 对账延迟（ms），默认覆盖 worker 调度抖动的两次 */
  refreshDelays?: number[]
}

/** 只要求有 `.value`，便于单测用普通对象构造（无需 vue 运行时） */
export type RowsRef<T> = { value: T[] }

export const DEFAULT_REFRESH_DELAYS = [800, 2600]

export const pageKeyOf = (knowledgeId?: string, page?: number) => `${knowledgeId}::${page}`

export function useBusinessListSync<T extends SyncRow>(rows: RowsRef<T>, opts: ListSyncOptions) {
  const delays = opts.refreshDelays?.length ? opts.refreshDelays : DEFAULT_REFRESH_DELAYS
  let timers: ReturnType<typeof setTimeout>[] = []
  let pending = false

  const clearTimers = () => {
    timers.forEach(clearTimeout)
    timers = []
  }

  /**
   * 乐观移除本地行：先整文件命中，再单页定位键命中。
   * 无 page（历史数据）的行只可能被整文件命中——与"无页码等价删整份"的删除语义一致。
   */
  const removeRows = (fileIds: Set<string>, pageKeys: Set<string>): void => {
    if (!fileIds.size && !pageKeys.size) return
    rows.value = rows.value.filter((r: T) => {
      if (r.knowledgeId && fileIds.has(r.knowledgeId)) return false
      if (r.page && r.page >= 1 && pageKeys.has(pageKeyOf(r.knowledgeId, r.page))) return false
      return true
    })
  }

  /** 删除后延迟对账：立即刷概览 + 两次延迟权威刷新（覆盖 worker 调度抖动） */
  const schedulePostDeleteRefresh = (): void => {
    clearTimers()
    pending = true
    opts.reloadStats?.()
    delays.forEach((d, i) => {
      timers.push(
        setTimeout(async () => {
          await opts.reload(true)
          opts.reloadStats?.()
          if (i === delays.length - 1) pending = false
        }, d),
      )
    })
  }

  /** 删除在途（乐观移除已生效、权威对账未完成） */
  const syncPending = () => pending

  /**
   * 按定位键删除：同文件多页时**先大页码后小页码**；
   * 后端反馈整份已删（deleted_file）时把该文件并入整文件删除集合。
   */
  const deletePages = async (targets: T[]): Promise<void> => {
    const fileDeleteIds = new Set<string>()
    const pageKeys = new Set<string>()
    const pageDeletes: Array<{ knowledgeId: string; page: number }> = []
    for (const r of targets) {
      if (r.page && r.page >= 1 && r.knowledgeId) pageDeletes.push({ knowledgeId: r.knowledgeId, page: r.page })
      else if (r.knowledgeId) fileDeleteIds.add(r.knowledgeId)
    }
    pageDeletes.sort((a, b) => b.page - a.page)
    for (const pd of pageDeletes) {
      try {
        const res: any = await opts.deletePage?.(pd.knowledgeId, pd.page)
        if (res?.deleted_file) fileDeleteIds.add(pd.knowledgeId)
        else pageKeys.add(pageKeyOf(pd.knowledgeId, pd.page))
      } catch (e: any) {
        opts.onError?.(e, { knowledgeId: pd.knowledgeId, page: pd.page })
        throw e
      }
    }
    if (fileDeleteIds.size) await opts.deleteFiles?.(Array.from(fileDeleteIds))
    removeRows(fileDeleteIds, pageKeys)
  }

  /** 整份文件删除（批量） */
  const deleteFiles = async (targets: T[]): Promise<void> => {
    const ids = Array.from(new Set(targets.map((r) => r.knowledgeId).filter((x): x is string => !!x)))
    if (!ids.length) return
    await opts.deleteFiles?.(ids)
    removeRows(new Set(ids), new Set())
  }

  /** 组件卸载必须调用：清理对账定时器，避免刷新已销毁的页面 */
  const destroy = () => {
    clearTimers()
    pending = false
  }

  return { removeRows, schedulePostDeleteRefresh, deletePages, deleteFiles, destroy, syncPending }
}
