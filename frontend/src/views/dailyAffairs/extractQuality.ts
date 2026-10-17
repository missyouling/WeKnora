// 提取质量与溯源的前端解析层（纯函数，零 Vue 依赖）。
//
// 数据来源：
//   - 详情/列表接口的 `custom_metadata`（六个业务模块同键名）
//   - 提取测试接口的返回体（Phase D 起同样带 extract_issues / patched_by_regex）
//
// 为什么单独成模块：
//   六个业务抽屉 + 测试弹窗都要同一套口径；这段逻辑是"历史数据防御"的主战场
//   （存量文档的 custom_metadata 里根本没有这几个键），放进组件里就没法单测，
//   只能靠字符串断言糊过去。
//
// 防御原则：任何字段缺失/类型错乱都收敛成空数组，绝不抛错、绝不返回 undefined。

/** 校验问题等级：error = 必填缺失；warn = 格式/交叉核对偏差 */
export type ExtractIssueSeverity = 'error' | 'warn'

export interface ExtractIssue {
  /** 结果数组下标；< 0 表示与具体记录无关的全局问题 */
  index: number
  /** 字段 key（与提取配置/结果同口径） */
  field: string
  /** missing | format | mismatch */
  kind: string
  detail: string
  severity: ExtractIssueSeverity
}

export interface RegexPatch {
  index: number
  field: string
  value: string
  /** config（用户配置正则） | builtin（内置高置信度正则） */
  source: string
}

export interface ExtractQuality {
  issues: ExtractIssue[]
  patches: RegexPatch[]
  /** 发生过补洞的字段名（按出现顺序去重），用于抽屉/测试弹窗的蓝色徽章 */
  patchedFields: string[]
}

export const EMPTY_QUALITY: ExtractQuality = { issues: [], patches: [], patchedFields: [] }

const isObject = (v: unknown): v is Record<string, any> =>
  !!v && typeof v === 'object' && !Array.isArray(v)

const asArray = (v: unknown): any[] => (Array.isArray(v) ? v : [])

const asString = (v: unknown): string => (typeof v === 'string' ? v.trim() : '')

function asIndex(v: unknown): number {
  const n = Number(v)
  return Number.isFinite(n) ? Math.trunc(n) : -1
}

function normalizeSeverity(v: unknown): ExtractIssueSeverity {
  return String(v ?? '').trim().toLowerCase() === 'error' ? 'error' : 'warn'
}

/**
 * 从 custom_metadata（或测试返回体）解析质量信息。
 *
 * @param meta 任意来源对象；缺失/非对象时返回空质量
 * @param index 可选：只看某一条记录（结果数组下标）。
 *              传了就只保留 `issue.index === index` 与本条无关的全局问题（index < 0）；
 *              不传则全量返回（列表/测试弹窗视角）。
 */
export function parseExtractQuality(meta: unknown, index?: number): ExtractQuality {
  if (!isObject(meta)) return { ...EMPTY_QUALITY }

  const scoped = typeof index === 'number' && Number.isFinite(index)
  const keep = (i: number) => !scoped || i === index || i < 0

  const issues: ExtractIssue[] = asArray(meta.extract_issues)
    .filter(isObject)
    .map((raw) => ({
      index: asIndex(raw.index),
      field: asString(raw.field),
      kind: asString(raw.kind) || 'missing',
      detail: asString(raw.detail),
      severity: normalizeSeverity(raw.severity),
    }))
    // 没有字段名的条目无法定位，直接丢弃（后端不会产出，但历史/脏数据可能出现）
    .filter((it) => !!it.field && keep(it.index))

  const patches: RegexPatch[] = asArray(meta.regex_patches)
    .filter(isObject)
    .map((raw) => ({
      index: asIndex(raw.index),
      field: asString(raw.field),
      value: asString(raw.value),
      source: asString(raw.source) || 'config',
    }))
    .filter((p) => !!p.field && keep(p.index))

  // patched_by_regex 是"整份文档"维度的字段名清单（后端按字段去重），
  // regex_patches 是逐条明细。两者并存时以清单顺序为主、明细补充漏项。
  const patchedFields: string[] = []
  const pushField = (f: string) => {
    if (f && !patchedFields.includes(f)) patchedFields.push(f)
  }
  if (!scoped) asArray(meta.patched_by_regex).forEach((f) => pushField(asString(f)))
  patches.forEach((p) => pushField(p.field))

  return { issues, patches, patchedFields }
}

export function qualityIsEmpty(q: ExtractQuality | null | undefined): boolean {
  return !q || (q.issues.length === 0 && q.patches.length === 0)
}

export function qualityIssueCounts(q: ExtractQuality | null | undefined): { error: number; warn: number } {
  const out = { error: 0, warn: 0 }
  for (const it of q?.issues || []) {
    if (it.severity === 'error') out.error += 1
    else out.warn += 1
  }
  return out
}

export interface QualityPatchLabel {
  field: string
  label: string
  count: number
  values: string[]
}

/**
 * 按字段聚合补洞徽章：同一字段补了 N 条只出一枚徽章，hover 里列 N 个值。
 * labelOf 由各业务注入（发票/合同等有自己的中文字段名映射）。
 */
export function qualityPatchLabels(
  q: ExtractQuality | null | undefined,
  labelOf: (field: string) => string,
): QualityPatchLabel[] {
  const out: QualityPatchLabel[] = []
  for (const field of q?.patchedFields || []) {
    const hits = (q?.patches || []).filter((p) => p.field === field)
    out.push({
      field,
      label: labelOf(field),
      count: hits.length || 1,
      values: hits.map((p) => p.value).filter(Boolean),
    })
  }
  return out
}

/** 问题定位文案：`#2 invoiceNo` / `invoiceNo`（全局问题不显示下标） */
export function issueLocation(it: ExtractIssue): string {
  return it.index >= 0 ? `#${it.index + 1} ${it.field}` : it.field
}
