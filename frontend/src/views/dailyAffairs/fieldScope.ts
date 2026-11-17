/**
 * 字段设置的「胶囊作用域」纯函数（第 60 轮）。
 *
 * 为什么单独一层：FleetCertPanel 的字段列表既要支持**无胶囊**（花名册早期形态）、
 * 又要支持**多胶囊**（市电账单 12 个分项、人事四胶囊…）。历史实现里
 * `tabGroupKey(f.group) === want || isLockedStatus(f)` 让「加锁状态字段」
 * 无条件穿透所有胶囊：
 *   · 劳动合同胶囊里出现 2 个「合同状态」（劳动合同 + 劳务合同各一个）；
 *   · 社保「正常记录 / 补退记录」里冒出「回盘表」的 5 个五险参保状态与「月度账单」的账单状态。
 * 胶囊隔离因此必须**按胶囊名称精确绑定**，而不是「凡是加锁就放行」。
 *
 * 另一件事：拖拽排序。旧实现用 `name` 找下标，劳动合同与劳务合同同名字段极多
 * （工号 / 姓名 / 部门 / 岗位…），`findIndex` 永远命中第一组 —— 拖劳务合同的行
 * 动的是劳动合同，视觉上「非首胶囊拖不动」。这里统一用 `group + name` 身份键。
 */

import type { EditableField } from './fleetCategorySubs'

/** 任意带 group / name 的字段（EditableField 的超集，便于单测构造）。 */
export type ScopedField = Pick<EditableField, 'name'> & { group?: string; [key: string]: any }

/**
 * 人事胶囊 → 该胶囊**唯一合法**的加锁状态字段（第 60 轮铁律）。
 *
 * 空数组 = 该胶囊 0 个加锁字段（正常记录 / 补退记录必须彻底清空）。
 * 未列出的胶囊名 = 非人事胶囊 / 用户自建胶囊，不参与本表的白名单校验。
 */
export const HR_GROUP_STATUS_FIELDS: Record<string, string[]> = {
  // 劳动合同（第 58 轮双胶囊）
  劳动合同: ['合同状态'],
  劳务合同: ['合同状态'],
  // 社保管理（第 56 轮四胶囊）
  回盘表: ['养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态'],
  正常记录: [],
  补退记录: [],
  月度账单: ['账单状态'],
  // 公积金管理（第 54 轮双胶囊）
  职工明细: ['账户状态'],
  // 花名册双胶囊：第 52 轮已删除「在职状态」，两张表都没有加锁字段
  在职人员: [],
  离职人员: [],
}

/** 胶囊键归一：去空白（费用清单的归一键由调用方先处理）。 */
export function groupKeyOf(group: unknown): string {
  return String(group ?? '').trim()
}

/** 胶囊 → 合法加锁状态字段；返回 null 表示该胶囊不在人事白名单内。 */
export function hrGroupStatusFieldsOf(group: unknown): string[] | null {
  const key = groupKeyOf(group)
  if (!key) return null
  return Object.prototype.hasOwnProperty.call(HR_GROUP_STATUS_FIELDS, key)
    ? HR_GROUP_STATUS_FIELDS[key]
    : null
}

/** 字段身份键：**胶囊 + 字段名**（同名跨胶囊必须区分，否则拖拽会动错组）。 */
export function fieldIdentityOf(field: ScopedField | null | undefined): string {
  const group = groupKeyOf(field?.group)
  const name = String(field?.name ?? '').trim()
  return `${group}\u0000${name}`
}

/**
 * 当前胶囊下应显示的字段。
 *
 * @param fields   扁平化的全部字段（fieldsEditable）
 * @param group    当前胶囊键（与字段 group 同口径）
 * @param options.strictStatus 人事 scope = true：加锁状态字段只认本胶囊白名单；
 *                             车队 / 能耗 = false：保留「文件级状态列在每个胶囊可见」的历史行为
 * @param options.isLocked     是否加锁状态字段（由 statusFields 统一判定，避免这里重复名单）
 */
export function visibleGroupFields<T extends ScopedField>(
  fields: readonly T[],
  group: string,
  options: { strictStatus?: boolean; isLocked?: (field: T) => boolean } = {},
): T[] {
  const want = groupKeyOf(group)
  const strict = !!options.strictStatus
  const isLocked = options.isLocked || (() => false)
  const allowed = strict ? hrGroupStatusFieldsOf(want) : null
  const out: T[] = []
  const seen = new Set<string>()
  for (const field of fields) {
    const inGroup = groupKeyOf(field?.group) === want
    if (!inGroup) {
      // 历史行为：文件级状态列（市电/光伏「状态」）不属于任何费用分组，
      // 任何胶囊下都要可见 —— 仅限非严格模式（人事已改为白名单绑定）。
      if (!strict && isLocked(field)) {
        const name = String(field?.name ?? '').trim()
        if (name && !seen.has(name)) { seen.add(name); out.push(field) }
      }
      continue
    }
    if (strict && isLocked(field)) {
      // 严格模式：本胶囊只允许白名单内的状态字段；正常/补退记录白名单为空 → 一个都不留。
      const name = String(field?.name ?? '').trim()
      if (!allowed || !allowed.includes(name)) continue
    }
    const name = String(field?.name ?? '').trim()
    // 严格模式按字段名去重：历史脏数据里同一胶囊可能落了两条「合同状态」。
    if (strict) {
      if (seen.has(name)) continue
      seen.add(name)
    }
    out.push(field)
  }
  return out
}

/**
 * 在**同一胶囊内**把 from 字段移动到 to 字段的位置（落点在目标之前）。
 *
 * 跨胶囊拖拽一律拒绝：字段顺序是 subs 的全局数组，允许跨组搬动会把字段
 * 直接改到别的胶囊里（用户视角是「字段消失了」）。
 */
export function moveFieldWithinGroup<T extends ScopedField>(
  fields: readonly T[],
  fromKey: string,
  toKey: string,
): { fields: T[]; moved: boolean } {
  if (!fromKey || !toKey || fromKey === toKey) return { fields: fields as T[], moved: false }
  const src = fields.findIndex((f) => fieldIdentityOf(f) === fromKey)
  const dst = fields.findIndex((f) => fieldIdentityOf(f) === toKey)
  if (src < 0 || dst < 0) return { fields: fields as T[], moved: false }
  if (groupKeyOf(fields[src]?.group) !== groupKeyOf(fields[dst]?.group)) {
    return { fields: fields as T[], moved: false }
  }
  const next = [...fields]
  const [moved] = next.splice(src, 1)
  // 删除后重新按身份定位目标：先前的 dst 可能已经左移一位。
  const at = next.findIndex((f) => fieldIdentityOf(f) === toKey)
  const target = at < 0 ? dst : at
  // 与用户直觉一致：把字段拖到目标上 = 占据目标的位置。
  // 向下拖 → 落在目标之后（A 拖到 B，结果是 B、A）；
  // 向上拖 → 落在目标之前。若一律「插到目标之前」，相邻两行互拖会原地不动，
  // 真机上会被读成「拖不动」。
  next.splice(src < dst ? target + 1 : target, 0, moved)
  return { fields: next, moved: true }
}
