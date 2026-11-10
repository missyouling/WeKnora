/**
 * 列表 / 详情抽屉的时间戳统一格式化出口（第 46 轮）。
 *
 * 问题：后端返回 ISO8601 原串 `2026-11-07T20:12:33.37663+08:00`，
 * 直接 `String(v)` 上屏会露出 T、微秒小数和时区偏移。
 *
 * 铁律：**优先按字面截取**，不 `new Date()` 解析带时区的串 —— 后者会按
 * 浏览器本地时区偏移，同一条数据在 UTC+8 / UTC-5 会显示成不同日期。
 * 只有字面无法匹配时才退到 Date 的 UTC 分量做兜底。
 */

const pad = (n: number): string => String(n).padStart(2, '0')

/** `2026-11-07T20:12:33.37663+08:00` / `2018-07-01 00:00:00` / `2026/7/9 9:5` 通用前缀。 */
const LITERAL = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})(?:[T\s]+(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?)?/

function parts(v: unknown): { y: number; m: number; d: number; hh: number; mm: number; ss: number; hasTime: boolean } | null {
  if (v === null || v === undefined || v === '') return null
  if (v instanceof Date) {
    if (Number.isNaN(v.getTime())) return null
    return { y: v.getUTCFullYear(), m: v.getUTCMonth() + 1, d: v.getUTCDate(), hh: v.getUTCHours(), mm: v.getUTCMinutes(), ss: v.getUTCSeconds(), hasTime: true }
  }
  const s = String(v).trim()
  if (!s) return null
  const m = LITERAL.exec(s)
  if (m) {
    const y = Number(m[1]); const mo = Number(m[2]); const d = Number(m[3])
    if (mo < 1 || mo > 12 || d < 1 || d > 31) return null
    return {
      y, m: mo, d,
      hh: m[4] === undefined ? 0 : Number(m[4]),
      mm: m[5] === undefined ? 0 : Number(m[5]),
      ss: m[6] === undefined ? 0 : Number(m[6]),
      hasTime: m[4] !== undefined,
    }
  }
  // 兜底：JS 可解析的其它形态（如 `Mon Nov 07 2026 ...`），用 UTC 分量避免时区漂移。
  const dt = new Date(s)
  if (Number.isNaN(dt.getTime())) return null
  return {
    y: dt.getUTCFullYear(), m: dt.getUTCMonth() + 1, d: dt.getUTCDate(),
    hh: dt.getUTCHours(), mm: dt.getUTCMinutes(), ss: dt.getUTCSeconds(), hasTime: true,
  }
}

/** ISO / 日期时间 → `YYYY-MM-DD HH:mm:ss`；空值或无法解析一律返回空串（绝不回显原串）。 */
export function formatDateTime(v: unknown): string {
  const p = parts(v)
  if (!p) return ''
  return `${p.y}-${pad(p.m)}-${pad(p.d)} ${pad(p.hh)}:${pad(p.mm)}:${pad(p.ss)}`
}

/** ISO / 日期时间 / 日期 → `YYYY-MM-DD`；空值或无法解析一律返回空串。 */
export function formatDate(v: unknown): string {
  const p = parts(v)
  if (!p) return ''
  return `${p.y}-${pad(p.m)}-${pad(p.d)}`
}

/**
 * 按列名分派的单元格格式化：含「时间」输出到秒，含「日期」只输出年月日，
 * 其余原样返回（调用方负责 `—` 占位）。
 */
export function formatCellDateTime(colKey: string, v: unknown): string {
  const key = String(colKey || '')
  if (/时间/.test(key)) return formatDateTime(v)
  if (/日期/.test(key)) return formatDate(v)
  return v === null || v === undefined ? '' : String(v)
}
