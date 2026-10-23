// 「分类字段」与后端 `fleet_categories.subs` 的**唯一双向映射层**。
//
// 为什么抽出来：
//   FleetCertPanel 里有三处需要搬运 subs——进入编辑态时「读」、保存时「写」、
//   复制分类时「复制」。此前每处各写一份内联 map，三份都只搬了
//   name/enabled/is_default/data_type/width，**把 subs 内嵌的 sub_fields
//   整体丢了**：用户在「字段定义」Tab 点一次保存，该分类的明细子列就被抹成空，
//   要等下次进程重启 seed（fleet.go seedInvoiceItemSubFields）才补回来。
//   智能字段生成（ai-suggest-fields）识别出的明细子列正是走这条链路，
//   不先修它，新功能产出会被用户的下一次保存抹掉。
//
// 契约对齐（internal/types/fleet.go FleetCategorySub / FleetItemSubField）：
//   前端编辑态 camelCase  ⇄  后端 JSONB snake_case
//   SubFields 带 omitempty → 非 items 字段不写 sub_fields，保持与 seed 形态一致。

import { INVOICE_FIELD_KEY_ALIASES } from './invoiceFieldLabels'

/** 明细子列（前端编辑态形态）。 */
export interface EditableSubField {
  key: string
  label: string
  dataType: string
  width: number
  /** 缺省（undefined）= 启用。只在显式关闭时下发 false，保持旧数据结构不被污染。 */
  enabled?: boolean
  /** 系统内置子列：禁止删除（可改文案/类型/宽度）。 */
  builtin?: boolean
  /** 提取口径（给模型的自然语言说明）。 */
  desc?: string
  /** 提取提示词里的子项规则（与 desc 同源，历史命名）。 */
  rule?: string
  /** 该子项的正则兜底（单票单明细行时回填）。 */
  regex?: string
}

/** 分类字段（前端编辑态形态）。 */
export interface EditableField {
  /** 字段身份：标准字段是契约键（invoice_no），扩展字段是后端分配的 snake_case 键。 */
  name: string
  /** 自定义中文表头；空=按内置字典显示。改它不影响 name 身份。 */
  label?: string
  enabled: boolean
  isDefault: boolean
  dataType: string
  width: number
  /** 系统内置字段：禁止删除。 */
  builtin?: boolean
  /** 仅 dataType === 'items' 时有值；随 subs 一并持久化，是明细子列的权威源。 */
  subFields?: EditableSubField[]
  /**
   * 业务分组（仅能耗类 scope 使用；空 = 主分组/未分组）。
   *
   * 市电账单一个分类下挂 12 个费用分项（账单概况/市场化购电费/…），共 79+ 字段。
   * 带上它才能让「字段定义」按分组切换、「提取规则」按分组过滤，
   * 主列表也不至于被子表字段淹没。
   */
  group?: string
  /** 分组中文名（展示用；空则按 group 键的字典值显示）。 */
  groupLabel?: string
}

/** 后端 subs 条目（读入形态；字段均为 snake_case）。 */
export interface SubPayload {
  name: string
  label?: string
  enabled: boolean
  is_default: boolean
  data_type: string
  width: number
  builtin?: boolean
  sub_fields?: Array<{
    key: string
    label: string
    data_type: string
    width: number
    enabled?: boolean
    builtin?: boolean
    desc?: string
    rule?: string
    regex?: string
  }>
  /** 业务分组键（仅能耗类 scope；空/缺省 = 无分组，与改造前数据结构一致）。 */
  group?: string
  /** 分组中文名。 */
  group_label?: string
}

const SUB_KEY_RE = /^[A-Za-z][A-Za-z0-9_]*$/
const MAX_SUB_FIELDS = 64
const MAX_WIDTH = 400

// ---- 明细子项去重（同义键 / 同列头）----
//
// 与后端 handler/fleet_item_subs.go 的同义分组保持一份口径：
// AI 建议与历史 seed 会同时产出 tax_amount(税额) 与 tax(税额)、price 与 unit_price，
// 界面上就会出现两列同名「税额」，用户既分不清也删不干净。

/** 同义键分组：同组内只保留一个，第一个元素是该语义的标准键。 */
const SUB_FIELD_ALIAS_GROUPS: readonly string[][] = [
  ['name', 'item_name', 'project_name', 'goods_name', 'product_name'],
  ['spec', 'model', 'specification', 'type_spec'],
  ['unit', 'uom', 'measure_unit'],
  ['qty', 'quantity', 'num', 'count'],
  ['price', 'unit_price', 'deal_price'],
  ['amount', 'total', 'money', 'subtotal'],
  ['tax_rate', 'rate', 'taxrate', 'tax_percent'],
  ['tax_amount', 'tax', 'taxamt', 'tax_money'],
]

/** 归一键：小写 + 去非字母数字（保留中文，避免自定义中文键互相误判）。 */
export function canonicalSubFieldKey(key: string): string {
  return String(key || '')
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fa5]/g, '')
}

/** 该键所属的同义组（返回组首=标准键）；无分组返回空串。 */
export function subFieldAliasGroup(key: string): string {
  const cn = canonicalSubFieldKey(key)
  if (!cn) return ''
  for (const g of SUB_FIELD_ALIAS_GROUPS) {
    if (g.some((k) => canonicalSubFieldKey(k) === cn)) return g[0]
  }
  return ''
}

/** 列头归一：去空白、统一中英文括号。 */
export function normalizeSubFieldLabel(label: string): string {
  return String(label || '')
    .replace(/[（]/g, '(')
    .replace(/[）]/g, ')')
    .replace(/\s+/g, '')
}

/**
 * 判断新子项是否与已有子项「同义或同列头」。
 * 返回命中的已有子项（无冲突返回 null），调用方据此提示用户。
 */
export function findDuplicateSubField(
  list: readonly EditableSubField[],
  candidate: { key?: string; label: string },
): EditableSubField | null {
  const key = canonicalSubFieldKey(candidate.key || '')
  const group = subFieldAliasGroup(candidate.key || '')
  const label = normalizeSubFieldLabel(candidate.label || candidate.key || '')
  for (const sf of list) {
    if (key && canonicalSubFieldKey(sf.key) === key) return sf
    if (group && subFieldAliasGroup(sf.key) === group) return sf
    if (label && normalizeSubFieldLabel(sf.label) === label) return sf
  }
  return null
}

/** 子列是否启用（缺省视为启用，兼容存量 JSON 里没有 enabled 的情况）。 */
export function subFieldEnabled(sf: EditableSubField): boolean {
  return sf.enabled !== false
}

/**
 * 系统内置明细子项键：与后端 invoiceItemSubFieldsStandard 的 8 个键一致。
 * 这些键是提取/列表渲染的硬编码口径（items[].name/spec/… 或 extra[plate_no]），
 * 删掉会让对应列永远空白，因此前端不允许删除。
 */
export const STANDARD_SUB_FIELD_KEYS: readonly string[] = [
  'name', 'spec', 'unit', 'qty', 'price', 'amount', 'tax_rate', 'tax_amount',
]

/**
 * 由中文子项名生成合法的子列键（用户不再手填 key）。
 *
 * 生成顺序：
 *   1. 命中文档化的中文→契约键表（规格型号→spec、车牌号→plate_no…）且未被占用 → 直接用；
 *   2. 输入本身已是合法英文键且未被占用 → 直接用（用户直接敲英文也行）；
 *   3. 否则递增 `sub_N`，保证 `^[A-Za-z][A-Za-z0-9_]*$` 且不重名。
 */
export function subFieldKeyForLabel(label: string, existing: readonly string[]): string {
  const clean = String(label ?? '').trim()
  const used = new Set(existing.map((k) => String(k).trim()))
  const alias = INVOICE_FIELD_KEY_ALIASES[clean]
  if (alias && SUB_KEY_RE.test(alias) && !used.has(alias)) return alias
  if (SUB_KEY_RE.test(clean) && !used.has(clean)) return clean
  let n = 1
  while (used.has(`sub_${n}`)) n += 1
  return `sub_${n}`
}

/**
 * 清洗明细子列：丢弃空键、非法键与超量数据，数字宽度做安全兜底。
 * 不抛异常——脏数据来自历史库/seed，最坏结果是少几个子列，而不是整个面板打不开。
 *
 * 默认值不下发到编辑态对象上（enabled=true / builtin=false / 空口径），
 * 这样 `fromSubsPayload → toSubsPayload` 对存量数据保持字节级互逆。
 */
export function normalizeSubFields(raw: unknown): EditableSubField[] {
  if (!Array.isArray(raw)) return []
  const out: EditableSubField[] = []
  for (const it of raw) {
    if (!it || typeof it !== 'object') continue
    const r = it as Record<string, unknown>
    const key = String(r.key ?? '').trim()
    if (!key || !SUB_KEY_RE.test(key)) continue
    const width = Number(r.width)
    const sf: EditableSubField = {
      key,
      label: String(r.label ?? key).trim() || key,
      dataType: String(r.data_type ?? r.dataType ?? 'text').trim() || 'text',
      width: Number.isFinite(width) && width > 0 ? Math.min(Math.round(width), MAX_WIDTH) : 0,
    }
    if (r.enabled === false) sf.enabled = false
    if (r.builtin === true) sf.builtin = true
    const desc = String(r.desc ?? '').trim()
    if (desc) sf.desc = desc
    const rule = String(r.rule ?? '').trim()
    if (rule) sf.rule = rule
    const regex = String(r.regex ?? '').trim()
    if (regex) sf.regex = regex
    out.push(sf)
    if (out.length >= MAX_SUB_FIELDS) break
  }
  return out
}

/**
 * subs（后端形态）→ 编辑态字段列表。
 * 容忍三种脏输入：字符串名（旧 seed 形态）、缺字段、重复名。
 */
export function fromSubsPayload(raw: unknown): EditableField[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: EditableField[] = []
  for (const s of raw) {
    if (s === null || s === undefined) continue
    const r = typeof s === 'object' ? (s as Record<string, unknown>) : { name: String(s) }
    const name = String(r.name ?? '').trim()
    if (!name) continue
    // 去重身份 = 分组 + 字段名。
    //
    // 不能只用 name：市电账单的 12 个费用分项里，market/line/trans/sys/gov-industrial/
    // catalog/gov-residential 这 7 组共用同一批列名（name/period/qty/rate/fee，
    // 它们本就是子表的列），只按 name 去重会把 7 个分组压成 1 个，
    // 表现为「分组胶囊条只剩 5 个」。带分组维度后各组独立。
    const gkey = String((r as Record<string, unknown>).group ?? '').trim()
    const dedupeKey = gkey ? `${gkey}::${name}` : name
    if (seen.has(dedupeKey)) continue
    seen.add(dedupeKey)
    const dataType = String(r.data_type ?? r.dataType ?? 'text').trim() || 'text'
    const width = Number(r.width)
    const subFields = normalizeSubFields(r.sub_fields)
    const label = String(r.label ?? '').trim()
    const field: EditableField = {
      name,
      enabled: r.enabled !== false,
      isDefault: r.is_default === true || r.isDefault === true,
      dataType,
      width: Number.isFinite(width) && width > 0 ? Math.min(Math.round(width), MAX_WIDTH) : 0,
    }
    // label 与 name 分离：name 是契约身份（改名会让历史数据错位），label 只是表头文案。
    if (label && label !== name) field.label = label
    if (r.builtin === true) field.builtin = true
    // 业务分组：只在有值时才写（空值不下发，保持旧数据结构形态）
    const group = String(r.group ?? '').trim()
    if (group) {
      field.group = group
      const gl = String(r.group_label ?? r.groupLabel ?? '').trim()
      if (gl) field.groupLabel = gl
    }
    // sub_fields 只在 items 字段上有意义；其他字段携带它属于脏数据，丢弃。
    if (dataType === 'items' && subFields.length) field.subFields = subFields
    out.push(field)
  }
  return out
}

/**
 * 编辑态字段列表 → subs（后端形态）。空名条目直接剔除。
 *
 * 与 fromSubsPayload 严格互逆：`toSubsPayload(fromSubsPayload(x)) === x`
 * （对合法后端数据成立，见 FleetCertPanel.test.ts 的往返断言）。
 */
export function toSubsPayload(fields: readonly EditableField[]): SubPayload[] {
  const out: SubPayload[] = []
  for (const f of fields) {
    const name = String(f.name ?? '').trim()
    if (!name) continue
    const dataType = f.dataType || 'text'
    const width = Number(f.width)
    const payload: SubPayload = {
      name,
      enabled: !!f.enabled,
      is_default: !!f.isDefault,
      data_type: dataType,
      width: Number.isFinite(width) && width > 0 ? Math.min(Math.round(width), MAX_WIDTH) : 0,
    }
    // 与 fromSubsPayload 互逆：label 为空或等于 name 时不下发（保持旧数据结构）
    const label = String(f.label ?? '').trim()
    if (label && label !== name) payload.label = label
    if (f.builtin) payload.builtin = true
    // 业务分组：仅在有值时下发，无 group 的旧分类序列化结果与改造前一致
    const group = String(f.group ?? '').trim()
    if (group) {
      payload.group = group
      const gl = String(f.groupLabel ?? '').trim()
      if (gl) payload.group_label = gl
    }
    const subs = normalizeSubFields(f.subFields)
    // omitempty 语义：非 items 或无子列时不下发 sub_fields，保持与后端 seed 一致。
    if (dataType === 'items' && subs.length) {
      payload.sub_fields = subs.map((s) => {
        const out: NonNullable<SubPayload['sub_fields']>[number] = {
          key: s.key,
          label: s.label,
          data_type: s.dataType,
          width: s.width,
        }
        // 默认值一律省略，保证存量 subs 往返后结构不变。
        if (s.enabled === false) out.enabled = false
        if (s.builtin) out.builtin = true
        if (s.desc) out.desc = s.desc
        if (s.rule) out.rule = s.rule
        if (s.regex) out.regex = s.regex
        return out
      })
    }
    out.push(payload)
  }
  return out
}

/**
 * 复制字段的唯一命名：`{中文名}_副本`，被占用时递增 `_副本2`、`_副本3`…
 *
 * 之前复制字段用英文 `{name}_copy` 生成身份键，字段配置/列表里直接露出
 * `tags_copy` 这类英文，与「新增字段用中文名」的口径不一致。
 * 现在复制与新增同口径：中文名即字段名（taken 需同时收进 name 与 label，防表头撞名）。
 */
export function copyFieldUniqueName(baseLabel: string, taken: Iterable<string>): string {
  const used = new Set<string>()
  for (const t of taken) used.add(String(t ?? ''))
  // 复制「副本」再复制时先剥掉尾部 `_副本`/`_副本N`，否则会长出
  // `甲方税号_副本_副本` 这种层层嵌套的名字；剥掉后才轮到序号递增（甲方税号_副本2）。
  const base = String(baseLabel ?? '').trim().replace(/_副本\d*$/, '').trim() || '字段'
  let n = 1
  let nm = `${base}_副本`
  while (used.has(nm)) { n += 1; nm = `${base}_副本${n}` }
  return nm
}

/**
 * 内置底稿（只有字段名，没有 enabled/width 等元信息）→ subs 形态。
 * 用于 FleetCertPanel「内置未入库分类」首次编辑时的兜底底稿，
 * 产出结构与 fromSubsPayload 消费的形态严格一致。
 */
export function subsPayloadFromNames(names: readonly string[]): SubPayload[] {
  return names
    .map((n) => String(n || '').trim())
    .filter(Boolean)
    .map((name) => ({
      name,
      enabled: true,
      is_default: false,
      data_type: 'text',
      width: 0,
    }))
}