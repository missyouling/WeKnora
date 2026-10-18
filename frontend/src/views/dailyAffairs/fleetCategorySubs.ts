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
}

const SUB_KEY_RE = /^[A-Za-z][A-Za-z0-9_]*$/
const MAX_SUB_FIELDS = 64
const MAX_WIDTH = 400

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
    if (!name || seen.has(name)) continue
    seen.add(name)
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