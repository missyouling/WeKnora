/**
 * 花名册分组字段矩阵（第 52 轮：11 组 → 精简 6 组）。
 *
 * 唯一数据源已上移到 `hrFieldSchema.ts` 的 `HR_ROSTER_GROUPS`：
 * 字段配置面板、列表表头、`[☰ 字段]` 筛选器、编辑抽屉、导出/打印清单、
 * 三套 Excel 模板全部从同一份契约派生 —— 改一处即六同步。
 *
 * 本文件保留历史导出 API（页面 / 单测仍在用），只做一层薄适配：
 *   · 家庭情况 / 工作经历情况 / 个税申报信息 三组已彻底删除；
 *   · 离职信息 → 独立「离职管理」模块；合同签订情况 → 独立「劳动合同」模块；
 *   · 在职状态 / 人员状态字段删除；
 *   · 字段配置取消胶囊分组，但 group 属性保留供编辑抽屉按 6 组分块。
 */

import {
  HR_ROSTER_GROUPS, HR_ROSTER_FIELDS, HR_ROSTER_DEFAULT_HEADERS,
} from './hrFieldSchema'

/** 字段定义（与历史契约同形：`checked` = 默认表头，`locked` = 系统内置）。 */
export interface RosterFieldDef {
  name: string
  checked: boolean
  locked?: boolean
}

export interface RosterFieldGroupDef {
  key: string
  label: string
  fields: RosterFieldDef[]
}

/** 离职信息分组 key（历史常量；离职字段已剥离到独立模块，这里保留避免旧代码报错）。 */
export const LEAVING_GROUP_KEY = '离职信息'

/** 默认表头取自「在职信息 + 基本信息」两组即可覆盖契约里 defaultHeader 的字段。 */
export const DEFAULT_HEADER_GROUPS: readonly string[] = ['在职信息', '基本信息']

export type RosterStatusFilter = '全部' | '在职' | '离职'

/** 花名册列表固定只查询在职人员，本常量仅保留给历史调用与单测，页面不再渲染。 */
export const ROSTER_STATUS_OPTIONS: readonly { label: string; value: RosterStatusFilter }[] = [
  { label: '全部', value: '全部' },
  { label: '在职', value: '在职' },
  { label: '离职', value: '离职' },
]

/**
 * 6 大分组（顺序即字段配置的单一连续列表顺序，也是编辑抽屉的分组顺序）。
 * `checked` 由 `defaultHeader` 映射；`locked` 由 `builtin` 映射。
 */
export const ROSTER_FIELD_GROUPS: readonly RosterFieldGroupDef[] = HR_ROSTER_GROUPS.map((g) => ({
  key: g.key,
  label: g.label,
  fields: g.fields.map((d) => ({
    name: d.name,
    checked: !!d.defaultHeader,
    ...(d.builtin ? { locked: true } : {}),
  })),
}))

/** 列表列顺序：花名册已无离职组，直接按 6 组自然顺序。 */
export function columnGroupOrder(): RosterFieldGroupDef[] {
  return [...ROSTER_FIELD_GROUPS]
}

/** 全部字段名（按列顺序，已去重）。 */
export function allRosterFields(): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const g of columnGroupOrder()) {
    for (const fd of g.fields) {
      if (seen.has(fd.name)) continue
      seen.add(fd.name)
      out.push(fd.name)
    }
  }
  return out
}

/** 核心锚点字段：任何视图都必须出现（定位行用）。 */
export const ROSTER_ANCHOR_FIELDS: readonly string[] = ['工号', '姓名', '部门', '岗位']

/** 静态兜底：默认表头 = 契约里 defaultHeader 的字段（顺序严格 6 组顺序）。 */
export function defaultVisibleFields(): string[] {
  return [...HR_ROSTER_DEFAULT_HEADERS]
}

/**
 * 从后端分类配置派生「默认表头集合」。
 * 契约：`enabled !== false && is_default === true`，顺序 = subs 数组顺序。
 */
export function defaultVisibleKeysFromConfig(categories: readonly any[] | null | undefined): string[] {
  const list = Array.isArray(categories) ? categories : []
  const cat = list.find((c: any) => String(c?.builtin_key ?? '') === 'hr-roster') || list[0]
  const subs = Array.isArray(cat?.subs) ? cat.subs : []
  const out: string[] = []
  for (const s of subs) {
    if (!s || s.enabled === false) continue
    if (s.is_default !== true) continue
    const name = String(s.name ?? '').trim()
    if (name) out.push(name)
  }
  return out.length ? out : defaultVisibleFields()
}

/** 从后端分类配置派生「可选全集」（`[☰ 字段]` 里能勾的字段）。 */
export function enabledFieldKeysFromConfig(categories: readonly any[] | null | undefined): string[] {
  const list = Array.isArray(categories) ? categories : []
  const cat = list.find((c: any) => String(c?.builtin_key ?? '') === 'hr-roster') || list[0]
  const subs = Array.isArray(cat?.subs) ? cat.subs : []
  const out: string[] = []
  for (const s of subs) {
    if (!s || s.enabled === false) continue
    const name = String(s.name ?? '').trim()
    if (name) out.push(name)
  }
  return out.length ? out : allRosterFields()
}

/** 必选字段（不可取消勾选）。 */
export function lockedRosterFields(): string[] {
  return HR_ROSTER_FIELDS.filter((d) => d.builtin).map((d) => d.name)
}

/** 按筛选态返回可见字段（花名册固定在职，不再有离职专属列）。 */
export function visibleFieldsFor(
  _status: RosterStatusFilter,
  overrides?: ReadonlySet<string> | null,
): string[] {
  const base = new Set(defaultVisibleFields())
  const out: string[] = []
  for (const g of columnGroupOrder()) {
    for (const fd of g.fields) {
      const visible = overrides ? (fd.locked || overrides.has(fd.name)) : base.has(fd.name)
      if (visible) out.push(fd.name)
    }
  }
  return out
}

/**
 * 抽屉分组：严格按「设置 → 字段设置」里启用的字段切分 6 组，空组自动丢弃。
 *
 * 第 52 轮修复：旧实现会把 `fd.checked` 也算作可见，导致用户关掉的字段
 * 仍在抽屉里出现（六同步断裂）。现在 `overrides`（后端 enabled 集合）是唯一判据。
 */
export function drawerGroupsFor(
  _status: RosterStatusFilter,
  overrides?: ReadonlySet<string> | null,
): { key: string; label: string; fields: string[] }[] {
  const enabled = overrides && overrides.size > 0 ? overrides : null
  const out: { key: string; label: string; fields: string[] }[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    const fields: string[] = []
    for (const fd of g.fields) {
      // 配置未加载时抽屉要展示全部 6 组字段（后端 subs 默认 enabled=true），
      // 不能只显示 is_default=true 的 15 个默认表头字段 —— 否则紧急联系人 /
      // 工资社保两组整组消失，抽屉分组标题不全。
      if (fd.locked || (enabled ? enabled.has(fd.name) : true)) fields.push(fd.name)
    }
    if (fields.length) out.push({ key: g.key, label: g.label, fields })
  }
  return out
}

/** 字段 → 所属分组 label（抽屉/表头 tooltip 用）。 */
export function groupLabelOf(field: string): string {
  for (const g of ROSTER_FIELD_GROUPS) {
    if (g.fields.some((fd) => fd.name === field)) return g.label
  }
  return ''
}

/** 分组全选/半选状态。 */
export function groupCheckState(
  group: RosterFieldGroupDef,
  visible: ReadonlySet<string>,
): 'all' | 'none' | 'partial' {
  const total = group.fields.length
  const on = group.fields.filter((fd) => fd.locked || visible.has(fd.name)).length
  if (on === 0) return 'none'
  if (on === total) return 'all'
  return 'partial'
}
