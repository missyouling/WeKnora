import { ref, computed, watch, type Ref } from 'vue'

export interface ColumnDef {
  key: string
  label: string
  default: boolean
  w: string
}

export interface BusinessListOptions {
  /** localStorage 存储 key（如 'weknora-contract-list-columns'） */
  storageKey: string
  /** 前端兜底默认列（后端未配置时回退） */
  defaultColumns: ColumnDef[]
}

/**
 * 日常事务管理页通用列表状态 Hook（Headless）。
 * 收敛：动态列 effectiveColumns、visibleColKeys localStorage 持久化、
 * 多选 selectedRowKeys、colValue 安全取值。
 * 业务行数据 rows、关键字 keyword、加载态 loading 由业务层自己维护。
 */
export function useBusinessList(opts: BusinessListOptions) {
  const { storageKey, defaultColumns } = opts

  // ---- 动态列 ----
  const customColumns = ref<ColumnDef[]>([])
  const effectiveColumns = computed<ColumnDef[]>(() =>
    customColumns.value.length ? customColumns.value : defaultColumns,
  )

  // ---- 可见列（localStorage 持久化） ----
  function loadStored(): string[] {
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        const arr = JSON.parse(raw)
        if (Array.isArray(arr) && arr.length) {
          return arr.filter((k: string) => effectiveColumns.value.some(c => c.key === k))
        }
      }
    } catch { /* ignore */ }
    return effectiveColumns.value.filter(c => c.default).map(c => c.key)
  }
  const visibleColKeys = ref<string[]>(loadStored())
  const visibleColDefs = computed(() =>
    effectiveColumns.value.filter(c => visibleColKeys.value.includes(c.key)),
  )
  function persistColumns() {
    try { localStorage.setItem(storageKey, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
  }
  watch(visibleColKeys, () => persistColumns(), { deep: true })

  function resetColumns() {
    visibleColKeys.value = effectiveColumns.value.filter(c => c.default).map(c => c.key)
  }
  function selectAllColumns() {
    visibleColKeys.value = effectiveColumns.value.map(c => c.key)
  }
  function colVisible(key: string) { return visibleColKeys.value.includes(key) }

  // ---- 多选 ----
  const selectedRowKeys = ref<string[]>([])
  function onSelectChange(val: string[]) { selectedRowKeys.value = val }
  function clearSelection() { selectedRowKeys.value = [] }

  // ---- 安全取值 ----
  function colValue(row: any, key: string): any {
    try {
      if (!row) return '-'
      const v = row[key]
      return v === undefined || v === null || v === '' ? '-' : v
    } catch { return '-' }
  }

  return {
    customColumns, effectiveColumns,
    visibleColKeys, visibleColDefs,
    resetColumns, selectAllColumns, colVisible,
    selectedRowKeys, onSelectChange, clearSelection,
    colValue,
  }
}
