/**
 * 当前页列表「全字段关键字筛选」唯一实现（第 74 轮）。
 *
 * 业务边界（必须严格遵守）：
 *   · 只针对**当前页面 / 当前激活标签的列表记录**做行级筛选；
 *   · 不是跨模块检索，也不是文档内容全文检索（那是知识库搜索的职责）。
 *
 * 为什么抽成纯函数：
 *   过去每个列表页各写一份 `JSON.stringify(row).includes(kw)` 或只拼几个主列，
 *   于是「所属公司 / 签约公司 / 部门 / 岗位 / 管理员 / 表号 / 自定义字段」这些
 *   派生或在 custom_data 里的值，在不同页面命中结果不一致（真机：搜「重庆星」
 *   在花名册 0 条、在宿舍 3 条）。这里统一成一份：
 *     · 递归遍历记录的全部字段值（原生列、custom_data / extra_data、明细 items）；
 *     · 不区分大小写；
 *     · 支持任意连续子串（搜 `重庆星` / `星达` / `铜业` 都命中
 *       `重庆星达铜业有限公司`）；
 *     · 空格分隔的多个关键字取 AND（搜 `星达 2026-08` 能进一步收敛）。
 */

/** 递归遍历的最大深度：列表行嵌套通常 ≤ 4 层（row → items → sub_fields）。 */
const MAX_DEPTH = 6

/** 这些键的值是"字段元信息/文件流"，不参与业务关键字匹配。 */
const SKIP_KEYS = new Set([
  'id', 'tenant_id', 'kb_id', 'knowledge_id', 'created_at', 'updated_at', 'deleted_at',
  'sort_order', 'page', 'bbox', 'chunk_id', 'file_url', 'preview_url', 'storage_path',
])

/** 单个标量 → 可匹配文本。null/undefined/空串一律忽略；布尔与数字按字面值。 */
function scalarText(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') return String(value)
  if (value instanceof Date) return value.toISOString().slice(0, 10)
  return ''
}

/**
 * 递归收集一条记录里的全部可匹配文本。
 *
 * 数组按元素递归（明细 items / 成员 / 配套设施都是数组），
 * 普通对象按 value 递归（custom_data / extra_data / source 等）。
 * 深度上限防止脏数据里的循环引用把列表拖死。
 */
function collect(value: unknown, out: string[], depth = 0): void {
  if (depth > MAX_DEPTH || value === null || value === undefined) return
  if (Array.isArray(value)) {
    for (const item of value) collect(item, out, depth + 1)
    return
  }
  if (typeof value === 'object') {
    if (value instanceof Date) { out.push(scalarText(value)); return }
    for (const [key, v] of Object.entries(value as Record<string, unknown>)) {
      if (SKIP_KEYS.has(key)) continue
      const text = scalarText(v)
      if (text) { out.push(text); continue }
      collect(v, out, depth + 1)
    }
    return
  }
  const text = scalarText(value)
  if (text) out.push(text)
}

/** 一条记录的完整可匹配文本（小写，空格连接）。 */
export function rowHaystack(row: unknown): string {
  const parts: string[] = []
  collect(row, parts)
  return parts.join(' ').toLowerCase()
}

/** 关键字 → 去空的小写词元。空关键字返回空数组（= 不过滤）。 */
export function keywordTokens(keyword: string): string[] {
  return String(keyword || '')
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
}

/**
 * 行级匹配：每个词元都必须作为连续子串出现在该行的任一字段值里。
 *
 * 命中示例：记录含 `重庆星达铜业有限公司`，搜 `重庆星` / `星达` / `铜业` 均命中。
 */
export function rowMatchesKeyword(row: unknown, keyword: string): boolean {
  const tokens = keywordTokens(keyword)
  if (!tokens.length) return true
  const hay = rowHaystack(row)
  return tokens.every((t) => hay.includes(t))
}

/** 列表级筛选：空关键字原样返回（保持原有顺序与引用）。 */
export function filterRowsByKeyword<T>(rows: T[], keyword: string): T[] {
  const tokens = keywordTokens(keyword)
  if (!tokens.length) return rows
  return rows.filter((row) => rowMatchesKeyword(row, keyword))
}
