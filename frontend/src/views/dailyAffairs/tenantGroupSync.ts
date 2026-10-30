/**
 * 「清单设置 · 分摊子项」与「字段定义 · 费用清单 · 租户分组」的双向同步。
 *
 * 两处编辑的是同一份业务事实，但存在两个物理落点：
 *   ① `billing_tenant_items`（租户分摊子项表：参与分摊 / 启用字段 / 宽度 / 来源）；
 *   ② `fleet_categories.subs`（费用清单分类里 `group=tenant:<id>` 的字段列表）。
 * 旧实现只写 ①，于是「分摊子项里开了某个项、字段定义里看不到」；
 * 反过来在字段定义里改了宽度也不会回到 ① —— 两边长期分叉。
 *
 * 本模块只做**纯计算**（把一侧的状态折算成另一侧应有的状态），
 * 读写接口由调用方注入，方便单测与避免循环依赖。
 */

export interface SyncItem {
  item_key?: string
  itemKey?: string
  item_name?: string
  itemName?: string
  category?: string
  enabled?: boolean
  field_enabled?: boolean
  fieldEnabled?: boolean
  width?: number
  source_category?: string
  source_key?: string
  source_mode?: string
  value_mode?: string
  sort?: number
}

/**
 * 字段的最小形状。刻意把 `enabled` / `isDefault` / `dataType` / `width` 声明为**必填**，
 * 与 `EditableField`（字段定义面板的编辑态）结构兼容 —— 这样本模块产出的数组
 * 可以直接喂给 `mergeTenantGroupFields` / `toSubsPayload`，不需要再做一次类型转换。
 */
export interface SyncField {
  name: string
  label?: string
  enabled: boolean
  isDefault: boolean
  dataType: string
  width: number
  group?: string
  groupLabel?: string
  source?: any
  builtin?: boolean
  subFields?: any[]
}

/** 子项 → 字段名（与 TenantBilling / billingFieldGroups 的 `item_<item_key>` 一致）。 */
export const ITEM_FIELD_PREFIX = 'item_'

export function itemFieldNameOf(it: SyncItem): string {
  const key = String(it?.item_key ?? it?.itemKey ?? '').trim()
  return key ? `${ITEM_FIELD_PREFIX}${key}` : ''
}

/**
 * 从「分摊子项」推算出租户分组应有的 item_* 字段。
 *
 * 只处理**启用字段**的子项（`field_enabled !== false`）：关掉「启用字段」意味着
 * 它不该作为列出现，此时字段定义里也必须没有它 —— 这正是旧实现缺的那一步。
 * 已存在字段的用户配置（顺序、改名、宽度、默认表头、显隐）由调用方保留，
 * 这里只给出「应该存在哪些字段、各自的兜底属性」。
 */
export function itemsToFields(items: readonly SyncItem[], group: string, groupLabel: string): SyncField[] {
  const out: SyncField[] = []
  const seen = new Set<string>()
  for (const it of items || []) {
    if (it?.field_enabled === false || it?.fieldEnabled === false) continue
    const name = itemFieldNameOf(it)
    if (!name || seen.has(name)) continue
    seen.add(name)
    const label = String(it?.item_name ?? it?.itemName ?? '').trim()
      || String(it?.item_key ?? it?.itemKey ?? '').trim()
    const width = Number(it?.width) || 0
    out.push({
      name,
      label,
      enabled: true,
      isDefault: true,
      dataType: 'number',
      width: width > 0 ? Math.round(width) : 0,
      group,
      groupLabel,
    })
  }
  return out
}

/**
 * 合并：保留已保存字段的用户配置，只做「增 / 删 / 属性兜底」。
 *
 * 规则（与 mergeTenantGroupFields 互补，不重复造轮子）：
 *   1. 已保存字段按原顺序保留，用户的 label / width / isDefault / enabled 一律不覆盖；
 *   2. 子项侧新开启的字段追加到末尾（位置由上层 merge 再收敛到收口列之前）；
 *   3. 子项侧已关掉「启用字段」的 item_* 字段整条移除；
 *   4. 非 item_* 字段（派生字段 / 收口列）原样保留，本模块不碰。
 */
export function mergeItemsIntoFields(
  saved: readonly SyncField[],
  items: readonly SyncItem[],
  group: string,
  groupLabel: string,
): { fields: SyncField[]; changed: boolean } {
  const desired = new Map<string, SyncField>()
  for (const f of itemsToFields(items, group, groupLabel)) desired.set(f.name, f)

  const out: SyncField[] = []
  const seen = new Set<string>()
  let changed = false
  for (const f of saved || []) {
    const name = String(f?.name ?? '').trim()
    if (!name || seen.has(name)) continue
    // item_* 字段：子项侧已关掉「启用字段」→ 移除
    if (name.startsWith(ITEM_FIELD_PREFIX) && !desired.has(name)) {
      changed = true
      continue
    }
    seen.add(name)
    // 兜底补齐必填项：从后端 subs 读回来的字段可能缺 dataType/width
    out.push({
      ...f,
      enabled: f?.enabled !== false,
      isDefault: f?.isDefault === true,
      dataType: String(f?.dataType || 'text'),
      width: Number(f?.width) || 0,
    })
  }
  // 子项侧新开启、字段定义里还没有 → 追加
  for (const [name, f] of desired) {
    if (seen.has(name)) continue
    seen.add(name)
    changed = true
    out.push(f)
  }
  return { fields: out, changed }
}

/**
 * 反向：从租户分组字段推算分摊子项该有的 `field_enabled` / `width`。
 *
 * 用于「在字段定义里改宽度 / 关掉启用字段 → 保存后回写 billing_tenant_items」。
 * 只回写这两个属性：参与分摊（enabled）由「清单设置」独占，字段定义侧不改它
 * （字段定义里没有「参与分摊」开关，硬写会把手动设置的分摊口径冲掉）。
 */
export function applyFieldsToItems(
  items: readonly SyncItem[],
  fields: readonly SyncField[],
): { items: SyncItem[]; changed: boolean } {
  const byName = new Map<string, SyncField>()
  for (const f of fields || []) {
    const name = String(f?.name ?? '').trim()
    if (name.startsWith(ITEM_FIELD_PREFIX)) byName.set(name, f)
  }
  let changed = false
  const out: SyncItem[] = []
  for (const it of items || []) {
    const name = itemFieldNameOf(it)
    const f = byName.get(name)
    if (!f) {
      // 字段定义里已经没有这个 item_* → 该子项不启用字段（仍保留参与分摊开关）
      if (it?.field_enabled !== false) changed = true
      out.push({ ...it, field_enabled: false })
      continue
    }
    const nextWidth = Math.max(0, Math.min(400, Math.round(Number(f.width) || 0)))
    const nextEnabled = f.enabled !== false
    if ((it?.field_enabled !== false) !== nextEnabled || (Number(it?.width) || 0) !== nextWidth) changed = true
    out.push({ ...it, field_enabled: nextEnabled, width: nextWidth })
  }
  return { items: out, changed }
}

/**
 * 胶囊/分组标题上的「字段数」：只数**顶层可见行**。
 *
 * 旧口径把折叠在大类里的成员也算进总数（持睿汽车显示 38，实际表头只有 22 列），
 * 用户按这个数字核对表头永远对不上。顶层行 = 大类汇总列 1 行 + 独立字段 1 行，
 * 大类里的成员不计。
 */
export function topLevelFieldCount(fields: readonly SyncField[]): number {
  const names = new Set<string>()
  const groupMembers = new Set<string>()
  for (const f of fields || []) {
    const name = String(f?.name ?? '').trim()
    if (!name) continue
    if (String(f?.source?.mode || '') === 'fee_group') {
      names.add(name)
      for (const m of (f.source?.members || [])) groupMembers.add(String(m))
      continue
    }
    names.add(name)
  }
  // 被某个大类声明为成员的字段不再单独计一行
  let count = 0
  for (const n of names) if (!groupMembers.has(n)) count += 1
  return count
}

// ---- 「派生字段 / 分摊子项」视觉分组 ------------------------------------------
//
// 需求背景：租户分组里 `使用单位 / 工业电量 / 宿舍水费`（内置派生）与
// `市场化购电费 / 上网环节线损费用 / 零售输配电费`（来自分摊子项）混排在一起，
// 且子项没有徽标，用户无法判断哪一列是"算出来的"、哪一列是"从账单分项摊过来的"。

/** 字段来源类型：派生（内置计算） / 分摊子项（来自清单设置） / 汇总大类（分摊子项折叠）。 */
export type FieldOrigin = 'derived' | 'item' | 'feeGroup'

/**
 * 判定某个租户分组字段的来源类型。
 *
 * 判定顺序（先具体后笼统）：
 *   ① `source.mode === 'fee_group'` → 分摊汇总大类（市场化购电费 / 系统运行费用 /
 *      政府性基金及附加），标签写「分摊汇总 · 细项 N」；
 *   ② 名字以 `item_` 开头 → 分摊子项（清单设置里逐项配的来源）；
 *   ③ 其余（month/unit/ratio/ind_fee/elec_total_fee/remark…）→ 派生字段。
 * 注意不能按「有没有 source」判：派生字段也可能挂 source（跨分类引用），
 * 但它们仍由费用清单自身算出来，语义上属于派生。
 */
export function fieldOriginOf(field: { name?: string; source?: any }): FieldOrigin {
  const mode = String(field?.source?.mode || '').trim()
  if (mode === 'fee_group') return 'feeGroup'
  const name = String(field?.name || '').trim()
  if (name.startsWith(ITEM_FIELD_PREFIX)) return 'item'
  return 'derived'
}

/** 徽标文案：分摊大类带细项数量，子项不带。 */
export function originBadgeOf(field: { name?: string; source?: any }): { text: string; kind: FieldOrigin } {
  const kind = fieldOriginOf(field)
  if (kind === 'feeGroup') {
    const n = Array.isArray(field?.source?.members) ? field.source.members.length : 0
    return { text: n ? `分摊汇总 · 细项 ${n}` : '分摊汇总', kind }
  }
  if (kind === 'item') return { text: '分摊子项', kind }
  return { text: '派生字段', kind }
}

/**
 * 把字段行切成两个区块（派生字段在前、分摊子项在后）。
 *
 * 顺序在区块内保持原相对顺序 —— 用户拖拽过的排列不能因为分组而被打乱。
 * `全部` 视图仍按原顺序平铺（只加徽标，不重排），避免改变既有操作习惯。
 */
export function splitFieldsByOrigin<T extends { name?: string; source?: any }>(
  fields: readonly T[],
): { derived: T[]; items: T[] } {
  const derived: T[] = []
  const items: T[] = []
  for (const f of fields || []) {
    if (fieldOriginOf(f) === 'derived') derived.push(f)
    else items.push(f)
  }
  return { derived, items }
}
