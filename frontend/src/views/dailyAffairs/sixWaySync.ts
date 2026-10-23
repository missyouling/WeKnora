// 「六同步」的唯一派生层：把 `fleet_categories.subs` 派生成六处消费形态。
//
// 为什么必须收口：
//   2026-10-21 真机实测发现同一份 subs 被六个地方各自解释，五处走偏——
//     · 主列表表头用 `builtin?.label || s.name`，静态字典把用户改名盖回旧名；
//     · 字段筛选器吃表头的产物，跟着一起错；
//     · 编辑抽屉在 subs 为空时回退静态 DEFAULT_*_COLUMNS，引入 subs 之外的
//       幽灵字段（`fileName`），删掉的字段又活了过来；
//     · 提取规则让 `extract_config.type` 压过 `subs.data_type`，改类型不生效；
//     · 后端 seed 里硬编码 `fileName` 并在启动 reconcile 时把用户删掉的字段补回。
//
// 铁律（本文件存在的意义）：
//   **`fleet_categories.subs` 是字段集合 / 名称 / 顺序 / 类型的唯一权威源。**
//   任何静态表（DEFAULT_*_COLUMNS、ScopeDescriptor.standardFields、字段目录）
//   只能提供「缺省值」，绝不允许覆盖用户在字段定义里保存过的值。
//
// 派生规则：
//   · 集合：`enabled !== false` 的 subs，顺序即 subs 顺序（拖拽排序直接生效）；
//   · 名称：`subs.label`（用户改名）优先，其次字典 `labelOf(name)`，最后才是 name；
//   · 类型：`subs.data_type` 优先，缺省才回退静态表/字典的 dataType；
//   · 幽灵字段：subs 里没有的键一律不产出（不补、不并、不追加）。

/** subs 条目的最小形态（后端 `FleetCategorySub` 的 JSON 形态）。 */
export interface SubEntry {
  name: string
  /** 用户自定义中文表头；空 = 按内置字典显示。 */
  label?: string
  enabled?: boolean
  is_default?: boolean
  data_type?: string
  dataType?: string
  width?: number
  builtin?: boolean
  /**
   * 字段所属业务分组（仅能耗类 scope 使用；空 = 主分组/未分组）。
   *
   * 市电账单一个分类下挂 12 个费用分项（账单概况/市场化购电费/上网环节线损费/…），
   * 共 79+ 字段。若不分分组，主列表表头会被子表字段淹没、字段定义表会变成 80 行。
   *
   * 兼容铁律：**空值即旧行为**。发票/合同/制度/公告不写这个键，
   * 六同步派生结果与改造前逐字节一致。
   */
  group?: string
  /** 分组中文名（仅展示用；空则按 group 键的字典值显示）。 */
  group_label?: string
}

/** 列表列定义（与 `useBusinessList.ColumnDef` 兼容并多带 dataType）。 */
export interface SyncedColumn {
  key: string
  label: string
  default: boolean
  w: string
  width?: number
  dataType: string
}

/** 抽屉/提取规则共用的字段元信息。 */
export interface SyncedField {
  name: string
  label: string
  dataType: string
  /** 备注片段字段名（仅发票口径使用）。 */
  remarkPart?: string
  /** 该字段在 subs 中的原始名（未经 fieldKeyOf 归一），供 remark 片段等处回查。 */
  rawName?: string
}

export interface DeriveOptions {
  /** 字段字典（静态表）→ 中文名。只用作 subs.label 为空时的缺省值。 */
  labelOf: (name: string) => string
  /** 字段身份归一（合同旧键 → 契约键）。返回空串表示该字段不可用，直接跳过。 */
  fieldKeyOf?: (name: string) => string
  /** 静态列定义（只提供宽度等缺省值，绝不用其 label 覆盖 subs.label）。 */
  defaultColumns?: Array<{ key: string; label?: string; dataType?: string; w?: string; width?: number }>
  /**
   * 按 `subs.group` 过滤（细分表消费方专用）。
   *
   * 缺省（undefined）= **不过滤**，即改造前的行为——发票/合同等无 group 的
   * scope 全部字段照常产出，零行为变化。
   *
   * 传入具体键：只产出该分组的字段（12 个费用分项子表各取各的）。
   * 传入数组：产出命中任一键的字段（主列表 `['', 'overview']` 取概况+未分组）。
   */
  groupFilter?: string | readonly string[]
}

/** subs 条目的业务分组键归一（空串代表主分组/未分组）。 */
export function groupOfSub(s: SubEntry): string {
  return String(s.group ?? '').trim()
}

/** 该 subs 条目是否命中分组过滤条件。 */
function matchGroup(s: SubEntry, filter: DeriveOptions['groupFilter']): boolean {
  if (filter === undefined) return true
  const g = groupOfSub(s)
  if (typeof filter === 'string') return g === filter
  return filter.includes(g)
}

/**
 * 列表系统列键：**不是提取字段**，不参与 subs / 编辑抽屉 / 提取规则。
 *
 * `fileName` 是文件属性（源文件名），模型不需要也不应该提取它。但它此前被
 * seed 进了 `fleet_categories.subs`，于是一路渗进「字段定义」面板、编辑抽屉
 * （末尾冒出「文件名称」输入框）、提取规则表——用户在字段定义里找不到它的来历，
 * 看到的就是一个幽灵字段（2026-10-21 真机验收）。
 *
 * 现在的口径：
 *   · subs / 抽屉 / 提取规则 → 一律不含系统列；
 *   · 列表 → 由 `appendListSystemColumns` 追加，保留「显示源文件名」的列表能力。
 */
export const LIST_SYSTEM_KEYS: readonly string[] = ['fileName']

/** 是否列表系统列（不是业务提取字段）。 */
export function isListSystemKey(key: string): boolean {
  return LIST_SYSTEM_KEYS.includes(String(key || '').trim())
}

/**
 * 给 subs 派生列追加列表系统列（置尾）。
 *
 * 系统列与业务字段分开管理的原因：系统列的显隐不受「字段定义」管辖（用户删不掉它，
 * 后端 seed 会补回来），但它必须能被字段筛选器勾选显示，所以仍要进列定义。
 * 键与 `defaultColumns` 命中时取其宽度与中文名，保持与旧列表视觉一致。
 */
export function appendListSystemColumns(
  cols: SyncedColumn[],
  defaultColumns?: Array<{ key: string; label?: string; dataType?: string; w?: string; width?: number }>,
  labelOf?: (n: string) => string,
): SyncedColumn[] {
  const out = [...cols]
  const taken = new Set(out.map((c) => c.key))
  for (const key of LIST_SYSTEM_KEYS) {
    if (taken.has(key)) continue
    const def = (defaultColumns || []).find((d) => String(d.key) === key)
    out.push({
      key,
      label: def?.label || labelOf?.(key) || key,
      // 系统列默认不显示：字段筛选器里可自行勾选，避免挤占业务列宽度
      default: false,
      w: def?.w || '1.4fr',
      dataType: def?.dataType || 'text',
    })
  }
  return out
}

/** subs 的类型取值归一（后端 snake_case 与前端 camelCase 都收）。 */
function dataTypeOf(s: SubEntry): string {
  return String(s.data_type ?? s.dataType ?? '').trim() || 'text'
}

/** 该 subs 条目是否启用（缺省视为启用，兼容没有 enabled 键的存量 JSON）。 */
function isEnabled(s: SubEntry): boolean {
  return s.enabled !== false
}

/**
 * 字段显示名：**subs.label 优先**。
 *
 * 这是「改名同步」的总闸门。此前各处写 `dictLabel || s.name` 或
 * `builtin?.label || s.name`，用户把「摘要」改成「摘要2」后会被静态字典盖回
 * 「摘要」，表头/筛选器/抽屉/规则面板五处同时显示旧名。
 */
export function columnLabelOf(s: SubEntry, labelOf: (n: string) => string): string {
  const custom = String(s.label ?? '').trim()
  if (custom) return custom
  const dict = String(labelOf(s.name) ?? '').trim()
  return dict || String(s.name || '').trim()
}

/**
 * 由 subs 派生列表列（同步点③字段筛选器 / 同步点④主列表表头同源）。
 *
 * 集合、顺序、名称、类型全部来自 subs；静态列只提供宽度与 dataType 缺省。
 * **只产出业务字段**：列表系统列（`fileName`）由调用方再用
 * `appendListSystemColumns` 追加——保持本函数「subs 里有什么就产出什么」的纯洁性，
 * 否则系统列会顺着列定义回流到字段筛选器以外的消费方。
 */
export function buildColumnsFromSubs(subs: readonly SubEntry[] | undefined, opts: DeriveOptions): SyncedColumn[] {
  const list = Array.isArray(subs) ? subs : []
  const keyOf = opts.fieldKeyOf || ((n: string) => String(n || '').trim())
  const byDefaultKey = new Map<string, NonNullable<DeriveOptions['defaultColumns']>[number]>()
  for (const d of opts.defaultColumns || []) byDefaultKey.set(String(d.key), d)

  const out: SyncedColumn[] = []
  const seen = new Set<string>()
  for (const s of list) {
    if (!s || typeof s !== 'object') continue
    const raw = String(s.name || '').trim()
    if (!raw) continue
    if (!isEnabled(s)) continue
    // 分组过滤：缺省不过滤（发票/合同等无 group 的 scope 行为不变）
    if (!matchGroup(s, opts.groupFilter)) continue
    const key = keyOf(raw)
    if (!key || seen.has(key)) continue
    // 系统列不走 subs 派生（见 LIST_SYSTEM_KEYS）；即便存量 subs 里还留着也不产出
    if (isListSystemKey(key) || isListSystemKey(raw)) continue
    seen.add(key)
    const def = byDefaultKey.get(key) || byDefaultKey.get(raw)
    const width = Number(s.width)
    out.push({
      key,
      label: columnLabelOf(s, opts.labelOf),
      default: s.is_default === true,
      w: def?.w || '1fr',
      width: Number.isFinite(width) && width > 0 ? Math.round(width) : undefined,
      dataType: dataTypeOf(s) || def?.dataType || 'text',
    })
  }
  return out
}

/**
 * 由 subs 派生抽屉字段（同步点⑤）。
 *
 * **唯一数据源铁律**：subs 为空就是空——绝不回退静态列。
 * 回退静态列的直接后果是 subs 里已删除的字段在抽屉里复活，且带进
 * `fileName` 这类系统列（用户从未在字段定义里见过它）。
 */
export function buildDrawerFieldsFromSubs(
  subs: readonly SubEntry[] | undefined,
  opts: DeriveOptions & { isRemarkPart?: (n: string) => boolean },
): SyncedField[] {
  const list = Array.isArray(subs) ? subs : []
  const keyOf = opts.fieldKeyOf || ((n: string) => String(n || '').trim())
  const out: SyncedField[] = []
  const seen = new Set<string>()
  for (const s of list) {
    if (!s || typeof s !== 'object') continue
    const raw = String(s.name || '').trim()
    if (!raw) continue
    if (!isEnabled(s)) continue
    // 分组过滤：缺省不过滤（发票/合同等无 group 的 scope 行为不变）
    if (!matchGroup(s, opts.groupFilter)) continue
    const key = keyOf(raw)
    if (!key || seen.has(key)) continue
    seen.add(key)
    const field: SyncedField = {
      name: key,
      label: columnLabelOf(s, opts.labelOf),
      dataType: dataTypeOf(s),
      rawName: raw,
    }
    if (opts.isRemarkPart?.(raw)) field.remarkPart = raw
    out.push(field)
  }
  return out
}

/** 提取规则面板的字段行（与 ExtractFieldConfig 兼容的最小子集）。 */
export interface SyncedRuleField {
  name: string
  label: string
  /** subs 的原始 data_type（text/number/date/array/items），六处一致性比对的键。 */
  dataType: string
  /** 提取配置方言下的类型（text↔string），后端 ExtractFieldConfig.type 用。 */
  type: string
  enabled: boolean
  desc: string
  rule: string
  regex: string
  required: boolean
}

export interface RuleDeriveOptions extends DeriveOptions {
  /** 已保存的提取配置（按 name 索引）：只提供 desc/rule/regex/required 等口径。 */
  savedByKey?: Map<string, { desc?: string; rule?: string; regex?: string; required?: boolean; enabled?: boolean; type?: string }>
}

/**
 * 由 subs 派生提取规则字段（同步点②）。
 *
 * 三处权威分工（与后端 `injectInvoiceItemSubFields` 同口径）：
 *   · 名称 / 顺序 / 启用 / 类型 → **subs**（字段定义）
 *   · desc / rule / regex / required → **extract_config**（用户在规则面板里配的口径）
 *
 * 此前实现让 `extract_config.type` 优先，用户把「摘要」改成 array 后规则面板
 * 仍显示「字符串」；改名后 `cfgMap.get(新名字)` 精确 miss，用户的 desc/rule 也一并丢失。
 * 现在按 subs 的 name 建索引，同时在 `subs.label` 命中时回退到旧配置（见 savedAlias）。
 */
export function buildRuleFieldsFromSubs(
  subs: readonly SubEntry[] | undefined,
  opts: RuleDeriveOptions,
): SyncedRuleField[] {
  const fields = buildDrawerFieldsFromSubs(subs, opts)
  const saved = opts.savedByKey || new Map()
  const out: SyncedRuleField[] = []
  for (const f of fields) {
    const hit = saved.get(f.name) || saved.get(String(f.rawName || '')) || saved.get(f.label)
    out.push({
      name: f.name,
      label: f.label,
      dataType: f.dataType,
      // 类型以 subs.data_type 为权威：它才是「字段定义」里用户选的那个值
      type: fieldTypeOfSub(f.dataType),
      enabled: true,
      desc: String(hit?.desc || ''),
      rule: String(hit?.rule || ''),
      regex: String(hit?.regex || ''),
      required: hit?.required === true,
    })
  }
  return out
}

/** subs 的 data_type → 提取配置的 type（text↔string 的方言归一）。 */
export function fieldTypeOfSub(dataType: string): string {
  switch (String(dataType || '').trim()) {
    case 'number': return 'number'
    case 'date': return 'date'
    case 'array': return 'array'
    case 'items': return 'items'
    // textarea 原样保留（与后端 extractTypeOfSubDataType / invoiceRuleTypeFor 同口径）：
    // 归一成 string 会让「字段定义」显示多行文本、「提取规则」显示单行文本，六同步断裂。
    case 'textarea': return 'textarea'
    default: return 'string'
  }
}

/** 一个业务分组（供「字段定义 / 提取规则」的分组胶囊条渲染）。 */
export interface SubGroup {
  /** 分组键；空串代表主分组/未分组。 */
  key: string
  /** 展示名：`group_label` 优先，其次字典，最后回退键名。 */
  label: string
  /** 该分组下的字段数（含禁用字段，便于用户看到全貌）。 */
  count: number
}

/**
 * 枚举 subs 里的业务分组（保持首次出现顺序）。
 *
 * 返回空数组 = 该分类没有分组维度（发票/合同/制度/公告等），
 * 调用方据此**不渲染**分组胶囊条，视觉与改造前完全一致。
 */
export function listSubGroups(
  subs: readonly SubEntry[] | undefined,
  labelOf?: (key: string) => string,
): SubGroup[] {
  const list = Array.isArray(subs) ? subs : []
  const out: SubGroup[] = []
  const index = new Map<string, number>()
  // 已显式声明过 group_label 的键：只有它们才值得保留，避免回退出的键名被误当成显式值
  const explicit = new Set<string>()
  const labelFor = (key: string) => (labelOf?.(key) || '').trim() || key
  for (const s of list) {
    if (!s || typeof s !== 'object') continue
    if (!String(s.name || '').trim()) continue
    const key = groupOfSub(s)
    const own = String(s.group_label ?? '').trim()
    const at = index.get(key)
    if (at !== undefined) {
      out[at].count += 1
      // 首个条目没写 group_label、后续条目写了 → 用后续的补上（真实场景：迁移顺序不定）
      if (own && !explicit.has(key)) {
        explicit.add(key)
        out[at].label = own
      }
      continue
    }
    index.set(key, out.length)
    if (own) explicit.add(key)
    out.push({ key, label: own || labelFor(key), count: 1 })
  }
  return out
}

/** 该 subs 是否带分组维度（决定是否渲染分组胶囊条）。 */
export function hasSubGroups(subs: readonly SubEntry[] | undefined): boolean {
  const groups = listSubGroups(subs)
  // 只有一个「空分组」= 没有分组维度
  return groups.length > 1 || (groups.length === 1 && groups[0].key !== '')
}
