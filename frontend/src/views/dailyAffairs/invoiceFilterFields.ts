// 分类专属字段筛选器派生（第 26 轮 O-3，补齐六同步第 ② 环）。
//
// 背景：字段定义 / 列表列 / 悬停明细浮窗 / 编辑抽屉 / 打印都由分类 subs 派生，
// 只有筛选器写死成「关键词 / 税率 / 开票日期」三项。通行费发票最需要按车牌号筛
// （运营会问「渝A75R1C 这个月跑了几趟」），此前只能把车牌丢进关键词框 ——
// 那会误命中备注、明细名称里出现同一串的记录。
//
// 设计取舍：
//   - **从列定义派生**（`customColumns`）而不是从原始 subs 派生：列已经是
//     「subs + 内置列宽/键归一」的产物，与列表/抽屉/浮窗同源，天然不会漂移。
//   - 用 `text` 而不是 `select`：车牌号候选值成百上千，而前端只有当前页数据，
//     下拉必然「选不到想选的值」。自由输入 + 后端结构化包含匹配才正确。
//   - 白名单控制哪些字段可作为筛选器：不是所有列都值得筛（金额/税率已有专门控件），
//     且白名单能防止未来新增列自动撑爆工具栏。

import type { FilterFieldSpec } from './businessFilter'

/**
 * 允许作为筛选器的字段（键可写列 key 或中文名，两种都认）。
 *
 * 只放「高基数 + 运营高频查询」的维度：
 *   - 车牌号：按车统计通行费
 *   - 车辆类型：客车/货车分账
 */
export const FILTERABLE_FIELD_LABELS: Record<string, string> = {
  plate_no: '车牌号',
  车牌号: '车牌号',
  vehicle_type: '车辆类型',
  车辆类型: '车辆类型',
}

/** 动态筛选器的 key 前缀：与固定项（keyword/taxRate/dateRange）区分。 */
export const FIELD_FILTER_PREFIX = 'field:'

/** 该字段是否可作为筛选器；是则返回展示名，否则返回空串。 */
export function filterableLabelOf(key: string): string {
  return FILTERABLE_FIELD_LABELS[String(key || '').trim()] || ''
}

/**
 * 由当前分类的列定义派生动态筛选器 spec。
 *
 * 顺序在 `keyword` 之后、`taxRate` 之前（视觉：搜索｜分类字段｜税率｜日期）。
 * 无匹配列时返回空数组 —— 普通发票等没有专属字段的分类，工具栏保持原样。
 */
export function buildInvoiceFieldFilterSpecs(
  columns: Array<{ key: string; label?: string }> | null | undefined,
): FilterFieldSpec[] {
  const out: FilterFieldSpec[] = []
  const seen = new Set<string>()
  for (const c of Array.isArray(columns) ? columns : []) {
    const label = filterableLabelOf(c?.key)
    if (!label || seen.has(label)) continue
    seen.add(label)
    out.push({
      key: `${FIELD_FILTER_PREFIX}${c.key}`,
      type: 'text',
      placeholder: label,
      clearable: true,
    })
  }
  return out
}

/**
 * 从扁平筛选态里抽出 `field:*` 键，拼成后端 `field_filters` 参数。
 *
 * 格式 `key:value;key:value`（用 `;` 分隔，因为值里可能含逗号如「重庆市,璧山区」）。
 * 空值跳过 —— 清空筛选器后不能传 `车牌号:`，那会被后端当作「筛不到任何记录」。
 */
export function buildFieldFiltersParam(state: Record<string, any>): string {
  const parts: string[] = []
  for (const [k, v] of Object.entries(state || {})) {
    if (!k.startsWith(FIELD_FILTER_PREFIX)) continue
    const key = k.slice(FIELD_FILTER_PREFIX.length)
    const val = String(v ?? '').trim()
    if (!key || !val) continue
    parts.push(`${key}:${val}`)
  }
  return parts.join(';')
}

/**
 * 把 `field:*` 键抽成独立对象（页面用它把动态筛选值存进 ref）。
 *
 * 反向：`splitFieldFilterState` 用于 filterState 的 get。
 */
export function pickFieldFilterValues(state: Record<string, any>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(state || {})) {
    if (!k.startsWith(FIELD_FILTER_PREFIX)) continue
    out[k] = String(v ?? '')
  }
  return out
}
