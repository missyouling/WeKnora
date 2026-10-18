// 智能字段建议 → 编辑态字段列表的合并逻辑（纯函数，可单测）。
//
// 单独成模块的原因：这个合并要同时处理三套命名空间
//（后端契约键 snake_case / 前端字段名 camelCase / 用户中文自定义名），
// 任何一处判错都会让用户看到"AI 建议的字段没并进来"或"把标准字段拆成了扩展字段"。
// 把它从 .vue 组件里抽出来，才能对命名映射做真正的回归测试。

import type { AiSuggestCustomField, AiSuggestStandardField, AiSuggestSubField } from '@/api/fleet'
import { canonicalFieldKey } from './extractFieldMeta'
import { invoiceFieldLabel } from './invoiceFieldLabels'
import type { EditableField, EditableSubField } from './fleetCategorySubs'

/** 标准契约键 → 前端展示名（snake_case 契约键经 invoiceFieldLabels 转中文）。 */
function labelOfStandardKey(key: string): string {
  // invoiceFieldLabels 的表是 camelCase 键，这里把契约键也试一次
  const camel = key.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase())
  return invoiceFieldLabel(camel) || invoiceFieldLabel(key) || key
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
}

/**
 * 把智能字段建议并入当前字段列表。
 *
 * 语义（三条铁律）：
 *  1. **勾选的才并入**：`standard` 只处理 `enable=true` 的条目；
 *     明细子列与自定义字段由调用方用勾选集合（`pickedSubKeys` / `pickedCustomNames`）控制。
 *  2. **不改已存在的字段**：已在列表里的字段名一律保留现状
 *     （改 data_type / enabled 会破坏用户既有配置），只在它是 items 且
 *     建议里带了子列时**补充缺失的子列**。
 *  3. **同名不重复**：标准键按 canonical 归一去重；自定义字段名与
 *     已存在字段 / 标准键冲突时跳过并给出原因。
 */
export function mergeAiFieldSuggestions(
  current: readonly EditableField[],
  standard: readonly AiSuggestStandardField[] = [],
  subFields: readonly AiSuggestSubField[] = [],
  custom: readonly AiSuggestCustomField[] = [],
  pickedSubKeys: readonly string[] = [],
  pickedCustomNames: readonly string[] = [],
): AiFieldMergeResult {
  const fields = current.map<EditableField>((f) => ({
    ...f,
    subFields: f.subFields?.map((s) => ({ ...s })),
  }))
  const skipped: AiFieldMergeResult['skipped'] = []
  let addedCount = 0

  const nameSet = new Set<string>()
  const canonSet = new Set<string>()
  for (const f of fields) {
    nameSet.add(f.name)
    canonSet.add(canonicalFieldKey(f.name))
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
    const name = key
    fields.push({
      name,
      enabled: true,
      // 新识别出的字段不默认勾选为"默认表头"：用户还没确认它是否常用
      isDefault: false,
      dataType: s.data_type || dataTypeOfStandardKey(key),
      width: 0,
    })
    nameSet.add(name)
    canonSet.add(canon)
    addedCount++
  }

  // ② 明细子列：只补给"已存在的 items 字段"；找不到 items 字段就跳过并说明，
  //    否则用户勾了半天发现没生效。
  const pickedSubs = new Set(pickedSubKeys.map((k) => String(k).trim()).filter(Boolean))
  if (pickedSubs.size) {
    const host = fields.find((f) => f.dataType === 'items')
    if (!host) {
      for (const k of pickedSubs) {
        skipped.push({ name: k, reason: '当前分类没有「明细」类型字段，明细子列无处挂载' })
      }
    } else {
      host.subFields = host.subFields || []
      const subKeys = new Set(host.subFields.map((s) => s.key))
      for (const s of subFields) {
        const key = String(s?.key || '').trim()
        if (!key || !pickedSubs.has(key)) continue
        if (subKeys.has(key)) continue
        const normalized: EditableSubField = {
          key,
          label: String(s.label || key).trim() || key,
          dataType: s.data_type || 'text',
          width: Number(s.width) > 0 ? Number(s.width) : 0,
        }
        host.subFields.push(normalized)
        subKeys.add(key)
        addedCount++
      }
    }
  }

  // ③ 自定义字段：中文名去重，且不与已有字段/标准契约键冲突
  const pickedCustom = new Set(pickedCustomNames.map((n) => String(n).trim()).filter(Boolean))
  for (const c of custom) {
    const name = String(c?.name || '').trim()
    if (!name || !pickedCustom.has(name)) continue
    const canon = canonicalFieldKey(name)
    if (nameSet.has(name) || (canon && canonSet.has(canon))) {
      skipped.push({ name, reason: '已存在同名字段' })
      continue
    }
    fields.push({
      name,
      enabled: true,
      isDefault: false,
      dataType: c.data_type === 'number' || c.data_type === 'date' ? c.data_type : 'text',
      width: 0,
    })
    nameSet.add(name)
    if (canon) canonSet.add(canon)
    addedCount++
  }

  return { fields, addedCount, skipped }
}

/** 供预览展示用：标准建议的中文名（前后端展示口径统一在这里收敛）。 */
export function aiStandardFieldLabel(key: string, fallback?: string): string {
  const f = String(fallback || '').trim()
  if (f) return f
  return labelOfStandardKey(String(key || '').trim())
}