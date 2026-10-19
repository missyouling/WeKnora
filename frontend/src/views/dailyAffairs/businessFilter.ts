// BusinessFilterBar 的声明式筛选协议（蓝图 P1-3）。
//
// 为什么把协议单独成文件：筛选控件的"空值口径"与"状态合并"是纯逻辑，
// 放进 .vue 里既没法单测，又会诱使消费方在模板里写业务分支。
// 页面只负责声明 `FilterFieldSpec[]`，控件长什么样由中台决定。

export type FilterFieldType = 'keyword' | 'select' | 'date-range' | 'number-range'

export interface FilterFieldSpec {
  /** 与 FilterState 的键一一对应 */
  key: string
  type: FilterFieldType
  /** select / date-range 用字符串；number-range 用 [最小占位, 最大占位] */
  placeholder?: string
  placeholders?: [string, string]
  options?: Array<{ label: string; value: any }>
  clearable?: boolean
  /** 前缀图标名（缺省按类型给） */
  icon?: string
  /** 变更立即派发 filter-change；缺省交给父级防抖 */
  immediate?: boolean
}

export type FilterState = Record<string, any>

/** 各类型的"空值"：日期/区间是数组，其余是空串——清空后不能留 null，否则后端收到 "null" */
export const EMPTY_FILTER_VALUE: Record<FilterFieldType, any> = {
  keyword: '',
  select: '',
  'date-range': [],
  'number-range': [],
}

export const DEFAULT_FILTER_ICONS: Record<FilterFieldType, string> = {
  keyword: 'search',
  select: 'file',
  'date-range': 'time',
  'number-range': 'calculator',
}

/**
 * 归一化控件回传值：
 *   - TDesign 清空时给 undefined/null → 落到该类型的空值；
 *   - 数组类型（date-range / number-range）非数组一律转空数组，杜绝后端收到 "[object Object]"。
 */
export function normalizeFilterValue(type: FilterFieldType, v: any): any {
  if (type === 'date-range' || type === 'number-range') {
    return Array.isArray(v) ? v : EMPTY_FILTER_VALUE[type]
  }
  if (v === undefined || v === null) return EMPTY_FILTER_VALUE[type]
  return v
}

/** 合并单个筛选值到状态：返回新对象（配合 v-model 的不可变更新） */
export function patchFilterState(state: FilterState, spec: FilterFieldSpec, v: any): FilterState {
  return { ...(state || {}), [spec.key]: normalizeFilterValue(spec.type, v) }
}

/**
 * 拆分 spec：keyword 走工具栏搜索位，其余按声明顺序渲染在筛选区。
 * 重复的 keyword 只认第一个（多搜索框没有意义）。
 */
export function splitFilterSpecs(
  fields: FilterFieldSpec[],
): { keyword?: FilterFieldSpec; rest: FilterFieldSpec[] } {
  const list = Array.isArray(fields) ? fields : []
  const keyword = list.find((f) => f.type === 'keyword')
  // 按类型排除而非按对象：重复声明的 keyword 一律不落到筛选区
  const rest = list.filter((f) => f.type !== 'keyword')
  return { keyword, rest }
}

/** number-range 的 [min, max] 读取（脏数据回退 [undefined, undefined]） */
export function rangePair(v: any): [any, any] {
  if (Array.isArray(v)) return [v[0], v[1]]
  return [undefined, undefined]
}

/** number-range 写入：任一值为空串/null 归一为 undefined（后端按未设置处理） */
export function writeRange(min: any, max: any): any[] {
  const norm = (x: any) => (x === '' || x === null || x === undefined ? undefined : x)
  return [norm(min), norm(max)]
}
