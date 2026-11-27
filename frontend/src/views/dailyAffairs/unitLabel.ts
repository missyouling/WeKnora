/**
 * 使用单位简称（第 75 轮）。
 *
 * 数据里既有工商全称（重庆星达铜业有限公司、持睿汽车部件有限公司），
 * 也有租户档案里的简称（持睿汽车）。动态单位切换标签要求：
 *   · 统计口径按「同一家公司」归并（全称/简称等价）；
 *   · 展示按简称（重庆星达 / 持睿汽车）。
 */
import { sameCompany } from './companyScope'

/** 去公司/行业后缀（保留行政区前缀）：「重庆星达铜业有限公司」→「重庆星达」。 */
function displayCoreOf(name: string): string {
  return String(name || '')
    .trim()
    .replace(/(（集团）|\(集团\)|集团)$/, '')
    .replace(/(股份)?有限(责任)?公司$/, '')
    .replace(/(有限)?公司$/, '')
    .replace(/厂$/, '')
    .replace(/(铜业|钢铁|建材|部件|配件|实业|制造)$/, '')
    .trim()
}

/** 匹配键：再去掉行政区前缀（重庆星达 ≈ 星达），用于跨写法归并。 */
function coreOf(name: string): string {
  return displayCoreOf(name).replace(/^(重庆市?|四川省?|广东省?|北京市?|上海市?)/, '').trim()
}

/**
 * 单位展示名：
 *   · 优先用「单位设置」里的简称/档案名（prefix 命中即取）；
 *   · 否则从全称派生核心词（去掉「重庆」「有限公司」，保留「星达铜业」这类可读词）。
 */
export function unitShortName(name: string, aliases: readonly string[] = []): string {
  const raw = String(name || '').trim()
  if (!raw) return ''
  const hit = (aliases || [])
    .map((a) => String(a || '').trim())
    .filter(Boolean)
    .find((a) => a.length <= raw.length && sameCompany(a, raw, []))
  if (hit) return hit
  const core = displayCoreOf(raw)
  return core || raw
}

/** 两组单位名归并成「同一家公司」的键（用于去重统计）。 */
export function unitGroupKey(name: string): string {
  return coreOf(name) || String(name || '').trim()
}

/**
 * 从一组原始 use_unit 值里归并出单位列表（保持首次出现顺序）。
 * 返回 [{ key, label, rawNames }]，label 为简称。
 */
export function collectUnits(
  values: Array<string | null | undefined>,
  aliases: readonly string[] = [],
): Array<{ key: string; label: string; full: string }> {
  const out: Array<{ key: string; label: string; full: string }> = []
  for (const v of values) {
    const full = String(v || '').trim()
    if (!full) continue
    const key = unitGroupKey(full)
    const hit = out.find((u) => u.key === key || sameCompany(u.full, full, []))
    if (hit) continue
    out.push({ key, label: unitShortName(full, aliases), full })
  }
  return out
}

/** 当前行是否属于所选单位（全称/简称等价）。 */
export function unitMatches(rowUnit: unknown, selectedFull: string): boolean {
  const a = String(rowUnit || '').trim()
  const b = String(selectedFull || '').trim()
  if (!a || !b) return false
  return sameCompany(a, b, [])
}
