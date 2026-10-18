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

/** 明细子列（前端编辑态形态）。 */
export interface EditableSubField {
  key: string
  label: string
  dataType: string
  width: number
}

/** 分类字段（前端编辑态形态）。 */
export interface EditableField {
  name: string
  enabled: boolean
  isDefault: boolean
  dataType: string
  width: number
  /** 仅 dataType === 'items' 时有值；随 subs 一并持久化，是明细子列的权威源。 */
  subFields?: EditableSubField[]
}

/** 后端 subs 条目（读入形态；字段均为 snake_case）。 */
export interface SubPayload {
  name: string
  enabled: boolean
  is_default: boolean
  data_type: string
  width: number
  sub_fields?: Array<{
    key: string
    label: string
    data_type: string
    width: number
  }>
}

const SUB_KEY_RE = /^[A-Za-z][A-Za-z0-9_]*$/
const MAX_SUB_FIELDS = 64
const MAX_WIDTH = 400

/**
 * 清洗明细子列：丢弃空键、非法键与超量数据，数字宽度做安全兜底。
 * 不抛异常——脏数据来自历史库/seed，最坏结果是少几个子列，而不是整个面板打不开。
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
    out.push({
      key,
      label: String(r.label ?? key).trim() || key,
      dataType: String(r.data_type ?? r.dataType ?? 'text').trim() || 'text',
      width: Number.isFinite(width) && width > 0 ? Math.min(Math.round(width), MAX_WIDTH) : 0,
    })
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
    const field: EditableField = {
      name,
      enabled: r.enabled !== false,
      isDefault: r.is_default === true || r.isDefault === true,
      dataType,
      width: Number.isFinite(width) && width > 0 ? Math.min(Math.round(width), MAX_WIDTH) : 0,
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
    const subs = normalizeSubFields(f.subFields)
    // omitempty 语义：非 items 或无子列时不下发 sub_fields，保持与后端 seed 一致。
    if (dataType === 'items' && subs.length) {
      payload.sub_fields = subs.map((s) => ({
        key: s.key,
        label: s.label,
        data_type: s.dataType,
        width: s.width,
      }))
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