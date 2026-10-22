// 智能字段建议 → 编辑态字段列表的合并逻辑（纯函数，可单测）。
//
// 单独成模块的原因：这个合并要同时处理三套命名空间
//（后端契约键 snake_case / 前端字段名 camelCase / 用户中文自定义名），
// 任何一处判错都会让用户看到"AI 建议的字段没并进来"或"把标准字段拆成了扩展字段"。
// 把它从 .vue 组件里抽出来，才能对命名映射做真正的回归测试。

import type { AiSuggestCustomField, AiSuggestStandardField, AiSuggestSubField } from '@/api/fleet'
import { canonicalFieldKey } from './extractFieldMeta'
import { INVOICE_FIELD_LABELS, invoiceFieldLabel, toCamelKey } from './invoiceFieldLabels'
import { findDuplicateSubField, type EditableField, type EditableSubField } from './fleetCategorySubs'

/** 标准契约键 → 前端展示名（snake_case 契约键经 invoiceFieldLabels 转中文）。 */
function labelOfStandardKey(key: string): string {
  return invoiceFieldLabel(key) || key
}

/**
 * 标准建议键 → 落库字段名（**键名归一化的唯一入口**）。
 *
 * 后端字典与契约是 snake_case（invoice_no），而业务分类的 subs 历史用 camelCase
 * （invoiceNo）。直接拿建议键当字段名，会让同一个分类里同时出现 invoiceNo 与
 * invoice_code 两种写法——界面就多一行裸英文。
 * 所以：能在字典里查到 camelCase 形态的一律归一到 camelCase。
 */
export function standardFieldName(key: string): string {
  const k = String(key || '').trim()
  if (!k) return ''
  const camel = toCamelKey(k)
  return INVOICE_FIELD_LABELS[camel] ? camel : k
}

/** 字段名 → snake_case（用作 dataType 口径查表的键） */
function toSnakeKeyOf(name: string): string {
  return String(name || '').replace(/([a-z0-9])([A-Z])/g, '$1_$2').toLowerCase()
}

/** 标准契约键 → 数据类型（与后端 InvoiceStandardFieldDictionary 口径一致）。 */
function dataTypeOfStandardKey(key: string): string {
  switch (key) {
    case 'invoice_date':
      return 'date'
    case 'total_amount':
    case 'amount':
    case 'tax':
    case 'tax_rate':
      return 'number'
    case 'items':
      return 'items'
    default:
      return 'text'
  }
}

export interface AiFieldMergeResult {
  /** 合并后的完整字段列表（可直接写入 fieldsEditable）。 */
  fields: EditableField[]
  /** 实际被并入的条目数（用于给用户一句"已并入 N 项"）。 */
  addedCount: number
  /** 被跳过的条目与原因（前端以 warning 展示，避免"我明明勾了却没进去"）。 */
  skipped: Array<{ name: string; reason: string }>
  /** 是否自动补建了「项目明细」父字段（勾了子列但当前分类没有 items 字段）。 */
  createdItemsHost?: boolean
}

/**
 * 扩展字段的合并身份：优先用后端分配的英文 key，没有 key 才退回中文名。
 *
 * 为什么不能用中文名当身份：中文名会被用户随时改（它只是表头）。
 * key 是只读的 snake_case 身份，改中文名不会让同一字段被重复追加。
 */
export function customFieldIdentity(c: AiSuggestCustomField): string {
  return String(c?.key || '').trim() || String(c?.name || '').trim()
}

/** 「项目明细」父字段的兜底定义（勾了子列但没有 items 宿主时自动补建）。 */
export const AUTO_ITEMS_HOST: EditableField = {
  name: 'items',
  label: '项目明细',
  enabled: true,
  isDefault: false,
  dataType: 'items',
  width: 0,
  subFields: [],
}

/**
 * 把智能字段建议并入当前字段列表。
 *
 * 语义（四条铁律）：
 *  1. **勾选的才并入**：`standard` 只处理 `enable=true` 的条目；
 *     明细子列与扩展字段由调用方用勾选集合（`pickedSubKeys` / `pickedCustomIds`）控制。
 *  2. **不改已存在的字段**：已在列表里的字段一律保留现状（name 身份不变，
 *     data_type / enabled 不被覆盖），只在它是 items 且建议里带了子列时
 *     **补充缺失的子列**。
 *  3. **同名不重复**：标准键按 canonical 归一去重；扩展字段按 key 身份去重，
 *     中文显示名同名也算重复（避免两个"开户银行"）。
 *  4. **勾了子列就必须有宿主**：当前分类没有 dataType==='items' 的字段时，
 *     自动补建「项目明细」父字段，不再把子列丢弃并报"无处挂载"。
 */
export function mergeAiFieldSuggestions(
  current: readonly EditableField[],
  standard: readonly AiSuggestStandardField[] = [],
  subFields: readonly AiSuggestSubField[] = [],
  custom: readonly AiSuggestCustomField[] = [],
  pickedSubKeys: readonly string[] = [],
  pickedCustomIds: readonly string[] = [],
): AiFieldMergeResult {
  const fields = current.map<EditableField>((f) => ({
    ...f,
    subFields: f.subFields?.map((s) => ({ ...s })),
  }))
  const skipped: AiFieldMergeResult['skipped'] = []
  let addedCount = 0

  const nameSet = new Set<string>()
  const canonSet = new Set<string>()
  const labelSet = new Set<string>()
  for (const f of fields) {
    nameSet.add(f.name)
    canonSet.add(canonicalFieldKey(f.name))
    if (f.label) labelSet.add(f.label.trim())
  }

  // ① 标准字段：勾选启用且尚不存在 → 追加（用契约键作为字段名，与后端落库契约同源）
  for (const s of standard) {
    if (!s?.enable) continue
    const key = String(s.key || '').trim()
    if (!key) continue
    const canon = canonicalFieldKey(key)
    if (!canon) {
      skipped.push({ name: key, reason: '字段键无法识别' })
      continue
    }
    if (nameSet.has(key) || canonSet.has(canon)) continue
    const name = standardFieldName(key)
    if (nameSet.has(name)) continue
    const label = String(s.label || '').trim()
    fields.push({
      name,
      // 用户/AI 给的中文表头：与契约键分离保存，改表头不动身份
      ...(label && label !== name ? { label } : {}),
      enabled: true,
      // 新识别出的字段不默认勾选为"默认表头"：用户还没确认它是否常用
      isDefault: false,
      dataType: s.data_type || dataTypeOfStandardKey(toSnakeKeyOf(name)),
      width: 0,
    })
    nameSet.add(name)
    canonSet.add(canon)
    if (label) labelSet.add(label)
    addedCount++
  }

  // ② 明细子列：补给 items 字段；没有宿主就自动补建「项目明细」父字段。
  const pickedSubs = new Set(pickedSubKeys.map((k) => String(k).trim()).filter(Boolean))
  let createdItemsHost = false
  if (pickedSubs.size) {
    let host = fields.find((f) => f.dataType === 'items')
    if (!host) {
      host = { ...AUTO_ITEMS_HOST, subFields: [] }
      fields.push(host)
      nameSet.add(host.name)
      canonSet.add(canonicalFieldKey(host.name))
      addedCount++
      createdItemsHost = true
    }
    host.subFields = host.subFields || []
    for (const s of subFields) {
      const key = String(s?.key || '').trim()
      if (!key || !pickedSubs.has(key)) continue
      const normalized: EditableSubField = {
        key,
        label: String(s.label || key).trim() || key,
        dataType: s.data_type || 'text',
        width: Number(s.width) > 0 ? Number(s.width) : 0,
      }
      // 去重口径与后端一致：同义键（税额 tax / tax_amount、单价 price / unit_price）
      // 与同列头都算重复，否则明细浮层会出现两列同名「税额」。
      const dup = findDuplicateSubField(host.subFields, { key, label: normalized.label })
      if (dup) {
        skipped.push({ name: normalized.label, reason: `与已有子项「${dup.label || dup.key}」同义` })
        continue
      }
      host.subFields.push(normalized)
      addedCount++
    }
  }

  // ③ 扩展字段：按 key 身份去重，中文名（label）也参与查重
  const pickedCustom = new Set(
    pickedCustomIds.map((id) => String(id).trim()).filter(Boolean),
  )
  for (const c of custom) {
    const identity = customFieldIdentity(c)
    const label = String(c?.name || '').trim()
    if (!identity) continue
    // 兼容调用方传中文名（旧口径）或传 key（新口径）
    if (!pickedCustom.has(identity) && !pickedCustom.has(label)) continue
    const canon = canonicalFieldKey(identity)
    if (nameSet.has(identity) || (canon && canonSet.has(canon)) || (label && labelSet.has(label))) {
      skipped.push({ name: label || identity, reason: '已存在同名字段' })
      continue
    }
    fields.push({
      name: identity,
      ...(label && label !== identity ? { label } : {}),
      enabled: true,
      isDefault: false,
      // textarea 必须原样透传：它是「多行文本」这一独立类型（不是 text 的别名），
      // 落成 text 会让抽屉退回单行输入框、长文本重新被截断
      dataType: ['number', 'date', 'array', 'textarea'].includes(String(c.data_type))
        ? String(c.data_type)
        : 'text',
      width: 0,
    })
    nameSet.add(identity)
    if (canon) canonSet.add(canon)
    if (label) labelSet.add(label)
    addedCount++
  }

  return { fields, addedCount, skipped, createdItemsHost }
}

/** 供预览展示用：标准建议的中文名（前后端展示口径统一在这里收敛）。 */
export function aiStandardFieldLabel(key: string, fallback?: string): string {
  const f = String(fallback || '').trim()
  if (f) return f
  return labelOfStandardKey(String(key || '').trim())
}