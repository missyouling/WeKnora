// 「费用清单」字段定义的多租户分组层（默认分组 + 动态租户分组）。
//
// 背景
// ----
// 费用清单的字段定义（fleet_categories.subs）原先只有一个隐式分组：
// 23 个字段全部写着 group='cols'，于是「字段配置」上方的分组胶囊条只有一格
// 「费用清单」，用户无法为不同租户配置不同的表头。
//
// 现在按 `subs[].group` 分区：
//   · `default`        → 默认分组（业主单位「重庆星达」视角），存量的 23 个字段；
//   · `tenant:<id>`    → 一个租户档案一个分组，字段 = 租户派生字段 + 该租户在
//                        「清单设置 → 分摊子项」里开启的分摊项。
//
// 为什么用 subs.group 而不是新建表：
//   · 六同步（表头 / 筛选器 / 抽屉 / 提取规则）本来就以 subs 为唯一权威源，
//     分组只是给同一份 subs 加一个维度，不需要第二条链路；
//   · 后端 FleetCategorySub 已有 group/group_label 两个字段（市电账单的 12 个
//     费用分项在用），零 schema 变更、零迁移；
//   · 空值即旧行为：其它分类不写 group，六同步派生与改造前逐字节一致。
//
// 本模块只放纯函数，便于单测；组件只做「取数 + 拼装」。

import type { EditableField } from './fleetCategorySubs'

/** 费用清单分类名（与 fleet_categories.name 逐字一致）。 */
export const COST_LIST_CATEGORY = '费用清单'

/** 默认分组（业主单位视角）。 */
export const DEFAULT_GROUP_KEY = 'default'
export const DEFAULT_GROUP_LABEL = '默认分组'

/** 租户分组键前缀。 */
export const TENANT_GROUP_PREFIX = 'tenant:'

/** 租户档案（GET /api/v1/billing/tenants 的最小投影）。 */
export interface BillingTenantLike {
  id?: string
  name?: string
}

/** 「清单设置 → 分摊子项」条目（GET /api/v1/billing/tenants/{id} 的 items[]）。 */
export interface TenantApportionItem {
  item_key?: string
  itemKey?: string
  item_name?: string
  itemName?: string
  source_category?: string
  sourceCategory?: string
  /** 参与分摊（算钱）。 */
  enabled?: boolean
  /** 启用字段（显示）；缺省视为启用。 */
  field_enabled?: boolean
  fieldEnabled?: boolean
  sort?: number
  width?: number
}

/** 租户分组键。 */
export function tenantGroupKey(tenantId: string): string {
  const id = String(tenantId ?? '').trim()
  return id ? `${TENANT_GROUP_PREFIX}${id}` : DEFAULT_GROUP_KEY
}

export function isTenantGroupKey(key: unknown): boolean {
  return String(key ?? '').trim().startsWith(TENANT_GROUP_PREFIX)
}

export function tenantIdOfGroup(key: unknown): string {
  const k = String(key ?? '').trim()
  return isTenantGroupKey(k) ? k.slice(TENANT_GROUP_PREFIX.length) : ''
}

/**
 * 费用清单的 group 归一。
 *
 * 存量 23 个字段写的是 `cols`（当年胶囊条只有一格「费用清单」），
 * 语义上它们就是默认分组；空值同样归到 default，避免出现两格空标签。
 * **只对费用清单调用**：其它分类的 group 有各自的业务含义。
 */
export function normalizeCostListGroup(key: unknown): string {
  const k = String(key ?? '').trim()
  if (!k || k === 'cols' || k === 'default') return DEFAULT_GROUP_KEY
  return k
}

/** 分组显示名：默认分组 / 租户名（拿不到租户名时退回 group 键）。 */
export function costListGroupLabel(key: unknown, tenantName?: string): string {
  const k = normalizeCostListGroup(key)
  if (k === DEFAULT_GROUP_KEY) return DEFAULT_GROUP_LABEL
  return String(tenantName ?? '').trim() || k
}

/** 取某分组下的 subs（同时兼容后端的 snake_case 与编辑态的 camelCase）。 */
export function subsOfGroup<T extends Record<string, any>>(
  subs: readonly T[] | undefined,
  groupKey: string,
): T[] {
  const want = normalizeCostListGroup(groupKey)
  const list = Array.isArray(subs) ? subs : []
  return list.filter((s) => normalizeCostListGroup(s?.group) === want)
}

/** 分摊子项在 subs 里的字段名（与 TenantBilling 的列 key 同构）。 */
export function tenantItemSubName(item: TenantApportionItem | null | undefined): string {
  const key = String(item?.item_key ?? item?.itemKey ?? '').trim()
  return key ? `item_${key}` : ''
}

/** 分摊子项是否要出现在字段列表 / 列表列（看「启用字段」，与「参与分摊」正交）。 */
export function isTenantItemVisible(item: TenantApportionItem | null | undefined): boolean {
  if (!item) return false
  return item.field_enabled !== false && item.fieldEnabled !== false
}

/**
 * 分摊子项的去重签名（大类 + 归一名称）。
 *
 * 库里同一费用项可能存在多条记录（功率因数调整电费 / 功率因素调整电费 /
 * 功率因数调整），item_key 各不相同。不去重会让字段配置多出两行同名同值的
 * 「功率因数调整」，而列表只显示一列 —— 六同步对不上。
 */
function normName(v: unknown): string {
  return String(v ?? '')
    .replace(/[（(]/g, '(')
    .replace(/[）)]/g, ')')
    .replace(/\s+/g, '')
    .replace(/因素/g, '因数')
    .replace(/其它/g, '其他')
    .toLowerCase()
}
export function tenantItemSignature(item: TenantApportionItem | null | undefined): string {
  const key = String(item?.item_key ?? item?.itemKey ?? '').trim()
  const i = key.indexOf('|')
  const cat = normName(i > 0 ? key.slice(0, i) : '')
  const rawName = i > 0 ? key.slice(i + 1) : String(item?.item_name ?? item?.itemName ?? '')
  const n = normName(rawName)
  return `${cat}::${n.includes('功率因数调整') ? '功率因数调整电费' : n}`
}

/**
 * 按签名去重（同一费用项只留一条）。
 *
 * `prefer` 里是「字段定义已经引用到的字段名」：优先保留它们，
 * 否则去重后留下的那条不在 subs 里，列表端会因为「找不到对应子项」而丢列。
 */
export function uniqueTenantItems<T extends TenantApportionItem>(
  items: readonly T[] | undefined,
  prefer?: ReadonlySet<string>,
): T[] {
  const list = Array.isArray(items) ? items : []
  const bySig = new Map<string, T>()
  for (const it of list) {
    const sig = tenantItemSignature(it)
    const cur = bySig.get(sig)
    if (!cur) {
      bySig.set(sig, it)
      continue
    }
    if (prefer && !prefer.has(tenantItemSubName(cur)) && prefer.has(tenantItemSubName(it))) bySig.set(sig, it)
  }
  const keep = new Set(bySig.values())
  return list.filter((it) => keep.has(it))
}

/** 按 sort 升序（与清单设置里的展示顺序一致）。 */
export function sortTenantItems<T extends TenantApportionItem>(items: readonly T[] | undefined): T[] {
  const list = Array.isArray(items) ? [...items] : []
  return list.sort((a, b) => (Number(a?.sort) || 0) - (Number(b?.sort) || 0))
}

/** 合计应付列的字段名（租户视角的金额收口列）。 */
export const TENANT_TOTAL_FIELD = 'item_total'

interface DerivedSpec {
  name: string
  label: string
  dataType: 'text' | 'number'
  isDefault: boolean
}

/**
 * 租户分组的派生字段（由费用清单自身核算逻辑算出，不来自任何分类的提取值）。
 *
 * name 必须与 TenantBilling 的 COL_DEFS_TENANT 键一致 —— 那是列渲染器的身份，
 * 对不上就只是一列永远空白的表头。label 是产品口径的中文名。
 */
export const TENANT_DERIVED_FIELDS: readonly DerivedSpec[] = [
  { name: 'month', label: '账单周期', dataType: 'text', isDefault: true },
  { name: 'unit', label: '使用单位', dataType: 'text', isDefault: true },
  { name: 'ratio', label: '分摊比例', dataType: 'number', isDefault: true },
  { name: 'ind_kwh', label: '工业电量', dataType: 'number', isDefault: true },
  { name: 'ind_fee', label: '工业电费', dataType: 'number', isDefault: true },
  { name: 'ind_price', label: '工业均价', dataType: 'number', isDefault: true },
  { name: 'dorm_kwh', label: '宿舍电量', dataType: 'number', isDefault: true },
  { name: 'dorm_fee', label: '宿舍电费', dataType: 'number', isDefault: true },
  { name: 'water_ind_usage', label: '工业用水量', dataType: 'number', isDefault: true },
  { name: 'water_ind_fee', label: '工业水费', dataType: 'number', isDefault: true },
  { name: 'water_dorm_usage', label: '宿舍水量', dataType: 'number', isDefault: true },
  { name: 'water_dorm_fee', label: '宿舍水费', dataType: 'number', isDefault: true },
  // 气费：租户分组尚未启用（列表没有气表列、打印也不含气），
  // 默认表头关闭 —— 否则字段配置会凭空比表头多两列（六同步违规）。
  { name: 'gas_usage', label: '用气量', dataType: 'number', isDefault: false },
  { name: 'gas_fee', label: '气费', dataType: 'number', isDefault: false },
  { name: 'remark', label: '备注', dataType: 'text', isDefault: false },
]

/** 派生字段名集合（用于判断「这条 subs 是派生字段还是分摊子项」）。 */
export const TENANT_DERIVED_NAMES: readonly string[] = TENANT_DERIVED_FIELDS.map((f) => f.name)

function derivedField(spec: DerivedSpec, group: string, groupLabel: string): EditableField {
  return {
    name: spec.name,
    label: spec.label,
    enabled: true,
    isDefault: spec.isDefault,
    dataType: spec.dataType,
    width: 0,
    group,
    groupLabel,
  }
}

function itemField(item: TenantApportionItem, group: string, groupLabel: string): EditableField {
  const name = tenantItemSubName(item)
  const label = String(item?.item_name ?? item?.itemName ?? name).trim() || name
  return {
    name,
    label,
    enabled: true,
    // 分摊子项默认出列：用户把它配进「分摊子项」就是为了看到这一列
    isDefault: true,
    dataType: 'number',
    width: Number(item?.width) > 0 ? Math.round(Number(item.width)) : 0,
    group,
    groupLabel,
  }
}

function totalField(group: string, groupLabel: string): EditableField {
  return {
    name: TENANT_TOTAL_FIELD,
    label: '合计应付',
    enabled: true,
    isDefault: true,
    dataType: 'number',
    width: 0,
    group,
    groupLabel,
  }
}

/** 从零生成一个租户分组的字段列表（派生字段 → 分摊子项 → 合计应付）。 */
export function buildTenantGroupFields(
  tenant: BillingTenantLike,
  items: readonly TenantApportionItem[] | undefined,
  groupLabel?: string,
): EditableField[] {
  const group = tenantGroupKey(String(tenant?.id ?? ''))
  const label = String(groupLabel ?? tenant?.name ?? '').trim() || group
  const out: EditableField[] = TENANT_DERIVED_FIELDS.map((d) => derivedField(d, group, label))
  for (const it of uniqueTenantItems(sortTenantItems(items))) {
    if (!isTenantItemVisible(it)) continue
    const f = itemField(it, group, label)
    if (f.name === 'item_') continue
    out.push(f)
  }
  out.push(totalField(group, label))
  return mergeFeeGroupFields(out, group, label)
}

/**
 * 合并「已保存的租户分组字段」与「清单设置里的分摊子项」。
 *
 * 规则（与产品口径一致）：
 *   1. 已保存项保留用户的顺序、重命名、宽度、默认表头、启用开关 —— 一个都不覆盖；
 *   2. 派生字段缺失时补在末尾（老数据没有它们，或用户删过）；
 *   3. 清单设置里新开启的分摊子项追加进来（插在「合计应付」之前，保持收口列在最后）；
 *   4. 清单设置里关掉「启用字段」的分摊子项从分组里移除，避免出现永远 0 的列；
 *   5. 「合计应付」始终存在且排在最后。
 */
export function mergeTenantGroupFields(
  saved: readonly EditableField[] | undefined,
  tenant: BillingTenantLike,
  items: readonly TenantApportionItem[] | undefined,
  groupLabel?: string,
): EditableField[] {
  const group = tenantGroupKey(String(tenant?.id ?? ''))
  const label = String(groupLabel ?? tenant?.name ?? '').trim() || group
  const preferNames = new Set(
    (saved || []).map((f) => String(f?.name ?? '').trim()).filter(Boolean),
  )
  const visibleItems = uniqueTenantItems(sortTenantItems(items).filter(isTenantItemVisible), preferNames)
  const visibleNames = new Set(visibleItems.map((it) => tenantItemSubName(it)).filter(Boolean))

  const out: EditableField[] = []
  const seen = new Set<string>()
  for (const f of saved || []) {
    const name = String(f?.name ?? '').trim()
    if (!name || seen.has(name)) continue
    // 分摊子项已被关掉 → 整条移除（否则会留下一列恒为 0 的僵尸字段）
    if (name.startsWith('item_') && name !== TENANT_TOTAL_FIELD && !visibleNames.has(name)) continue
    // 合计应付稍后统一补在末尾，避免它被夹在中间
    if (name === TENANT_TOTAL_FIELD) continue
    seen.add(name)
    out.push({ ...f, group, groupLabel: label })
  }

  for (const spec of TENANT_DERIVED_FIELDS) {
    if (seen.has(spec.name)) continue
    seen.add(spec.name)
    out.push(derivedField(spec, group, label))
  }

  for (const it of visibleItems) {
    const f = itemField(it, group, label)
    if (!f.name || f.name === 'item_' || seen.has(f.name)) continue
    seen.add(f.name)
    out.push(f)
  }

  out.push(totalField(group, label))
  // 大类汇总列：把 (1)/(4)/(6) 的细项收进「市场化购电费 / 系统运行费用 /
  // 政府性基金及附加」三列（成员仍在列表里，只是不再各自出列）。
  return mergeFeeGroupFields(out, group, label)
}

/**
 * 只做「存量默认分组归一」：把 group='cols'/空 改成 default 并把分组名改成「默认分组」，
 * **原样保留租户分组字段**。
 *
 * 为什么需要它：进入编辑态时租户档案还没拉回来（两次接口），此时若直接调
 * withCostListGroups(fields, [], {})，租户分组会被整片丢弃 —— 用户保存过的
 * 顺序/改名/宽度全部丢失，随后 hydration 只能按模板重新合成（真机：改名「工业均价（元）」
 * 与宽 120 在重新打开面板后消失，但库里其实是对的）。
 */
export function normalizeCostListDefaults(fields: readonly EditableField[] | undefined): EditableField[] {
  return (Array.isArray(fields) ? fields : []).map((f) => {
    const g = normalizeCostListGroup(f?.group)
    return g === DEFAULT_GROUP_KEY ? { ...f, group: DEFAULT_GROUP_KEY, groupLabel: DEFAULT_GROUP_LABEL } : { ...f }
  })
}

/**
 * 把「后端 subs 读出来的字段列表」整理成费用清单的完整分组结构。
 *
 * - 存量 group='cols' 归一到 default，并把 group_label 改写成「默认分组」；
 * - 每个租户补出一个 tenant:<id> 分组（已保存项合并、缺失项合成）；
 * - 分组顺序固定为「默认分组 → 各租户（按传入顺序）」，与胶囊条一致。
 */
export function withCostListGroups(
  fields: readonly EditableField[] | undefined,
  tenants: readonly BillingTenantLike[] | undefined,
  itemsByTenant: Record<string, readonly TenantApportionItem[] | undefined> | undefined,
): EditableField[] {
  const list = Array.isArray(fields) ? fields : []
  const out: EditableField[] = []
  const seen = new Set<string>()
  for (const f of list) {
    const g = normalizeCostListGroup(f?.group)
    if (g !== DEFAULT_GROUP_KEY) continue
    const name = String(f?.name ?? '').trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    out.push({ ...f, group: DEFAULT_GROUP_KEY, groupLabel: DEFAULT_GROUP_LABEL })
  }
  for (const t of tenants || []) {
    const id = String(t?.id ?? '').trim()
    if (!id) continue
    const g = tenantGroupKey(id)
    const saved = list.filter((f) => normalizeCostListGroup(f?.group) === g)
    out.push(...mergeTenantGroupFields(saved, t, itemsByTenant?.[id], String(t?.name ?? '')))
  }
  return out
}

/**
 * 租户分组的「清单设置 → 分摊子项」变更后，判断某个已保存的租户分组是否过期。
 *
 * 用于设置抽屉打开时决定要不要提示「分摊子项已变更，请确认字段顺序」。
 * 纯函数：只比字段集合，不比顺序/文案。
 */
export function tenantGroupNeedsSync(
  saved: readonly EditableField[] | undefined,
  items: readonly TenantApportionItem[] | undefined,
): boolean {
  const visible = new Set(
    sortTenantItems(items).filter(isTenantItemVisible).map((it) => tenantItemSubName(it)).filter(Boolean),
  )
  const has = new Set((saved || []).map((f) => String(f?.name ?? '').trim()).filter(Boolean))
  for (const n of visible) if (!has.has(n)) return true
  for (const n of has) if (n.startsWith('item_') && n !== TENANT_TOTAL_FIELD && !visible.has(n)) return true
  for (const spec of TENANT_DERIVED_FIELDS) if (!has.has(spec.name)) return true
  return !has.has(TENANT_TOTAL_FIELD)
}

// ---- 大类汇总列：市场化购电费 / 系统运行费用 / 政府性基金及附加 ----------------
//
// 问题：租户分组把「清单设置 → 分摊子项」里开启的市电分项全部平铺成列，
// 2026-09 的持睿汽车因此有 31 列，其中 (1) 3 列 + (4) 8 列 + (6) 5 列。
// 产品口径：主表头只留三个大类和少量独立列，点单元格的 +/− 在行下方展开细项。
//
// 实现：大类本身也是一条 sub（source.mode='fee_group'），成员仍在 subs 里
// （保留启用/顺序/宽度/默认表头），但**不再各自出列**。
// 这样六同步（表头 / 筛选器 / 字段配置 / 抽屉）仍然只有 subs 一个权威源。

export interface FeeGroupSpec {
  /** subs.name（列 key），稳定不变 */
  name: string
  label: string
  /** 市电账单大类（billing_tenant_items.category 的前缀形态） */
  category: string
  tip: string
  /** 账单大类匹配（账单原文可能带 `费用`/`及附加` 等后缀差异） */
  match: (category: string) => boolean
}

export const TENANT_FEE_GROUPS: readonly FeeGroupSpec[] = [
  {
    name: 'grp_market',
    label: '市场化购电费',
    category: '(1)市场化购电费',
    tip: '(1)市场化购电费 下属细项汇总（零售交易电费 / 零售损益分摊电费 / 偏差电费等）',
    match: (c) => c.startsWith('(1)') && c.includes('市场化购电'),
  },
  {
    name: 'grp_system',
    label: '系统运行费用',
    category: '(4)系统运行费',
    tip: '(4)系统运行费用 下属细项汇总（燃气机组 / 新能源机制差价 / 抽水蓄能 / 煤电容量等）',
    match: (c) => c.startsWith('(4)') && c.includes('系统运行'),
  },
  {
    name: 'grp_gov',
    label: '政府性基金及附加',
    category: '(6)政府性基金及附加-工商业',
    tip: '(6)政府性基金及附加（工商业）下属细项汇总（农网还贷 / 重大水利 / 库区移民 / 可再生能源等）',
    // 只认「工商业」：居民同名基金是另一套（计费电量 8938 度），混进来就重复计费
    match: (c) => c.startsWith('(6)') && c.includes('政府性基金') && c.includes('工商业'),
  },
]

/** 分摊子项名 → 所属账单大类（`item_<大类>|<子项名>`）。 */
export function itemCategoryOfName(name: unknown): string {
  const n = String(name ?? '').trim()
  if (!n.startsWith('item_') || n === TENANT_TOTAL_FIELD) return ''
  const rest = n.slice(5)
  const i = rest.indexOf('|')
  return i > 0 ? rest.slice(0, i) : ''
}

/** 账单大类 → 大类汇总定义（不属于三大类返回 null）。 */
export function feeGroupSpecOfCategory(category: unknown): FeeGroupSpec | null {
  const c = String(category ?? '').trim()
  if (!c) return null
  return TENANT_FEE_GROUPS.find((g) => g.match(c)) || null
}

/** subs 里的「大类汇总字段」→ 定义（普通字段返回 null）。 */
export function feeGroupSpecOfSub(sub: any): FeeGroupSpec | null {
  if (String(sub?.source?.mode || '') !== 'fee_group') return null
  const cat = String(sub?.source?.group || sub?.source?.groupLabel || '').trim()
  const byCat = feeGroupSpecOfCategory(cat)
  if (byCat) return byCat
  const n = String(sub?.name || '').trim()
  return TENANT_FEE_GROUPS.find((g) => g.name === n) || null
}

/** 大类汇总字段（写入 subs）。members 顺序 = 展开后的明细顺序。 */
export function buildFeeGroupField(
  spec: FeeGroupSpec,
  members: readonly string[],
  group: string,
  groupLabel: string,
): EditableField {
  return {
    name: spec.name,
    label: spec.label,
    enabled: true,
    isDefault: true,
    dataType: 'number',
    width: 0,
    group,
    groupLabel,
    source: {
      category: '市电账单',
      field: spec.name,
      mode: 'fee_group',
      group: spec.category,
      label: spec.label,
      members: [...members],
    },
  }
}

/** 折叠后的一行：普通字段 或 大类（含成员）。 */
export type CostListRow<T> =
  | { kind: 'field'; name: string; sub: T }
  | { kind: 'group'; name: string; label: string; spec: FeeGroupSpec; sub: T | null; members: Array<{ name: string; sub: T }> }

/**
 * 把 subs 折叠成「表头行」序列：大类成员被收进大类里，不再单独出列。
 *
 * 两种来源都支持：
 *   · subs 里已有 `mode=fee_group` 的字段 → 用它（保留用户的名称/宽度/显隐/位置）；
 *   · 没有（用户还没保存过）→ 按账单大类**现算**一个大类，位置取第一个成员的位置。
 * 两条路径产出同样的顺序，所以列表表头与字段配置永远一致（六同步）。
 */
export function collapseFeeGroups<T extends { name?: string; label?: string; source?: any }>(
  subs: readonly T[],
): Array<CostListRow<T>> {
  const list = Array.isArray(subs) ? subs : []
  const explicit = new Map<string, { sub: T; index: number }>()
  list.forEach((s, i) => {
    const spec = feeGroupSpecOfSub(s)
    if (spec && !explicit.has(spec.name)) explicit.set(spec.name, { sub: s, index: i })
  })

  // 成员归属：显式 group 的 members 优先；其余按账单大类自动归入
  const memberOf = new Map<string, string>() // sub.name -> group spec name
  const membersOf = new Map<string, string[]>()
  const pushMember = (gname: string, subName: string) => {
    if (!gname || !subName) return
    const arr = membersOf.get(gname) || []
    if (!arr.includes(subName)) arr.push(subName)
    membersOf.set(gname, arr)
    memberOf.set(subName, gname)
  }
  // ① 显式声明
  for (const [gname, { sub }] of explicit) {
    const declared = Array.isArray((sub as any)?.source?.members) ? (sub as any).source.members : []
    for (const m of declared) pushMember(gname, String(m || '').trim())
  }
  // ② 未声明/新开启的子项按大类自动归入（保持 subs 顺序）
  for (const s of list) {
    const name = String(s?.name || '').trim()
    if (!name) continue
    const cat = itemCategoryOfName(name)
    if (!cat) continue
    const spec = feeGroupSpecOfCategory(cat)
    if (!spec) continue
    if (memberOf.has(name)) continue
    pushMember(spec.name, name)
  }
  // ③ 补齐显式 group 的成员（声明的 members 里可能有已被删掉的字段）
  for (const [, arr] of membersOf) {
    for (const n of [...arr]) {
      if (!list.some((s) => String(s?.name || '').trim() === n)) {
        arr.splice(arr.indexOf(n), 1)
      }
    }
  }

  const rows: Array<CostListRow<T>> = []
  const emitted = new Set<string>()
  for (const s of list) {
    const name = String(s?.name || '').trim()
    if (!name) continue
    const spec = feeGroupSpecOfSub(s)
    if (spec) {
      if (emitted.has(spec.name)) continue
      emitted.add(spec.name)
      const members = (membersOf.get(spec.name) || [])
        .map((n) => ({ name: n, sub: list.find((x) => String(x?.name || '').trim() === n) as T }))
        .filter((m) => !!m.sub)
      rows.push({ kind: 'group', name: spec.name, label: String(s?.label || '').trim() || spec.label, spec, sub: s, members })
      continue
    }
    const gname = memberOf.get(name)
    if (!gname) {
      rows.push({ kind: 'field', name, sub: s })
      continue
    }
    if (emitted.has(gname)) continue
    // 合成大类：放在第一个成员的位置
    emitted.add(gname)
    const gspec = TENANT_FEE_GROUPS.find((g) => g.name === gname)
    if (!gspec) {
      rows.push({ kind: 'field', name, sub: s })
      continue
    }
    const members = (membersOf.get(gname) || [])
      .map((n) => ({ name: n, sub: list.find((x) => String(x?.name || '').trim() === n) as T }))
      .filter((m) => !!m.sub)
    rows.push({ kind: 'group', name: gname, label: gspec.label, spec: gspec, sub: null, members })
  }
  return rows
}

/**
 * 在字段列表里补齐大类汇总字段（插在第一个成员之前），成员保持原位置。
 *
 * 只改「有成员的三大类」，其它字段原封不动 —— 用户拖过的顺序、改过的名称、
 * 宽度、显隐全部保留。已存在的大类字段只刷新 members（清单设置新开/关掉子项后同步）。
 */
export function mergeFeeGroupFields(
  fields: readonly EditableField[] | undefined,
  group: string,
  groupLabel: string,
): EditableField[] {
  const list = (Array.isArray(fields) ? fields : []).map((f) => ({ ...f }))
  const existing = new Map<string, number>()
  list.forEach((f, i) => {
    const spec = feeGroupSpecOfSub(f)
    if (spec) existing.set(spec.name, i)
  })
  for (const spec of TENANT_FEE_GROUPS) {
    const members = list
      .map((f) => String(f?.name || '').trim())
      .filter((n) => {
        const cat = itemCategoryOfName(n)
        return !!cat && !!feeGroupSpecOfCategory(cat) && feeGroupSpecOfCategory(cat)!.name === spec.name
      })
    const at = existing.get(spec.name)
    if (!members.length) {
      // 没有成员就撤掉大类列，否则会留下一列永远 0
      if (at !== undefined) list.splice(at, 1)
      continue
    }
    if (at !== undefined) {
      const cur = list[at]
      list[at] = {
        ...cur,
        group,
        groupLabel,
        source: {
          ...(cur.source || { category: '市电账单', field: spec.name }),
          mode: 'fee_group',
          group: spec.category,
          label: cur.source?.label || spec.label,
          members,
        },
      }
      continue
    }
    const firstMember = list.findIndex((f) => members.includes(String(f?.name || '').trim()))
    const field = buildFeeGroupField(spec, members, group, groupLabel)
    list.splice(firstMember < 0 ? list.length : firstMember, 0, field)
  }
  return list
}
