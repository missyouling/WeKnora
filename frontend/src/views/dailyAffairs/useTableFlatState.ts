/**
 * 列表「空 / 加载中」态的横向滚动条收口（第 37 轮）。
 *
 * 现象（真机图 3）：列表在 loading 或 0 条数据时，表头下方半空中出现一条
 * 横向滚动条 —— 数据行还没渲染，但列总宽度已超过容器，浏览器把滚动条轨道画了出来。
 *
 * 规则：**只有「加载完成 && 有数据」时才允许横向滚动**。
 * 页面把 `flat` 绑到 `.doc-list-view` 的 class 上，样式在
 * `styles/business-list-shared.css` 里统一收口（四模块共用一份规则）。
 *
 * 为什么做成 composable 而不是各自写 computed：判据必须一致，
 * 否则某个页漏判（比如把 loading 写成 listLoading）就只修好一半模块。
 */
import { computed, type Ref } from 'vue'

export interface TableFlatInput {
  /** 是否正在加载首屏数据 */
  loading?: Ref<boolean> | boolean
  /** 当前数据行数（或行数组本身） */
  rows?: Ref<number> | Ref<readonly any[]> | number | readonly any[]
}

const toNum = (v: TableFlatInput['rows']): number => {
  if (v === undefined || v === null) return 0
  const raw = typeof v === 'object' && 'value' in (v as any) ? (v as any).value : v
  if (Array.isArray(raw)) return raw.length
  return Number(raw) || 0
}

const toBool = (v: TableFlatInput['loading']): boolean => {
  if (v === undefined || v === null) return false
  const raw = typeof v === 'object' && 'value' in (v as any) ? (v as any).value : v
  return !!raw
}

/**
 * 返回 `{ isTableFlat }`；为真时给 `.doc-list-view` 挂 `is-table-flat`。
 *
 * 判定：`loading === true` **或** 行数为 0 → flat（隐藏横向滚动条）。
 */
export function useTableFlatState(input: TableFlatInput) {
  const isTableFlat = computed(() => {
    // loading 传 ref 时要读到 .value；直接传布尔也支持（部分页面是 props）
    const busy = input.loading
    const loadingNow = busy && typeof busy === 'object' && 'value' in (busy as any)
      ? !!(busy as any).value
      : toBool(busy)
    if (loadingNow) return true
    return toNum(input.rows) === 0
  })
  return { isTableFlat }
}

/** 拼接 class：`doc-list-view` + 可选的 flat 类。模板里直接用。 */
export const flatClassOf = (isFlat: boolean): string =>
  isFlat ? 'doc-list-view is-table-flat' : 'doc-list-view'
