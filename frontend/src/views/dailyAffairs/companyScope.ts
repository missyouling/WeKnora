/**
 * 单位名称归一与表计筛选（第 63 轮）。
 *
 * 背景：
 *   · 表计设置里的「使用单位 / 归属单位」历史上是自由文本，存量数据存在简称
 *     （如「重庆星达」「持睿汽车」），而单位设置里是工商全称；
 *   · 宿舍 / 公租房档案的「关联电表 / 关联水表」必须按【用途 + 使用单位】
 *     双重过滤，否则会把工商业表甚至别家公司的表挂到宿舍上。
 *
 * 这里全部做成纯函数，前后端同一套口径（后端 Go 侧同名实现见 hr_round63.go），
 * 便于单测覆盖简称/全称互通、用途归类与过滤结果。
 */

export interface OrgUnitLike {
  name?: string
  departments?: Array<{ name?: string } | string>
}

/** 常见前后缀：比较核心词时忽略它们（「重庆星达」≈「重庆星达铜业有限公司」）。 */
const COMPANY_PREFIX = /^(重庆市?|四川省?|广东省?)/
const COMPANY_SUFFIX = /((股份)?有限(责任)?公司|公司|厂|集团)$/

function coreName(v: string): string {
  return String(v || '')
    .trim()
    .replace(COMPANY_PREFIX, '')
    .replace(COMPANY_SUFFIX, '')
    .replace(/\s+/g, '')
}

/** 把任意写法（简称 / 全称 / 空）归一为「单位设置」里的标准名称。 */
export function normalizeCompanyName(value: unknown, names: readonly string[]): string {
  const raw = String(value ?? '').trim()
  if (!raw) return ''
  const list = (names || []).map((n) => String(n ?? '').trim()).filter(Boolean)
  if (!list.length) return raw
  const exact = list.find((n) => n === raw)
  if (exact) return exact
  // 互相包含（取最长 = 全称）
  const contained = list.filter((n) => n.includes(raw) || raw.includes(n))
  if (contained.length) return contained.sort((a, b) => b.length - a.length)[0]
  // 核心词互通：「重庆星达」↔「重庆星达铜业有限公司」
  const core = coreName(raw)
  if (core) {
    const byCore = list.filter((n) => {
      const nc = coreName(n)
      return nc && (nc.includes(core) || core.includes(nc))
    })
    if (byCore.length) return byCore.sort((a, b) => b.length - a.length)[0]
  }
  return raw
}

/** 两个单位名是否指同一家公司（简称 / 全称双向等价）。 */
export function sameCompany(a: unknown, b: unknown, names: readonly string[]): boolean {
  const x = normalizeCompanyName(a, names)
  const y = normalizeCompanyName(b, names)
  if (!x || !y) return false
  if (x === y) return true
  if (x.includes(y) || y.includes(x)) return true
  const cx = coreName(x)
  const cy = coreName(y)
  return !!cx && !!cy && (cx.includes(cy) || cy.includes(cx))
}

/** 单位设置 → 公司名列表（保持后端 sort_order 顺序）。 */
export function companyNamesOf(units: readonly OrgUnitLike[] | null | undefined): string[] {
  return (units || []).map((u) => String(u?.name ?? '').trim()).filter(Boolean)
}

/** 单位设置 → 某公司下的部门名列表；该公司无部门时回退全部部门（兼容标准部门列表）。 */
export function departmentsOf(
  units: readonly OrgUnitLike[] | null | undefined,
  company: string,
): string[] {
  const list = units || []
  const all: string[] = []
  const seen = new Set<string>()
  const push = (v: string) => {
    const s = String(v ?? '').trim()
    if (!s || seen.has(s)) return
    seen.add(s)
    all.push(s)
  }
  let matched: string[] = []
  for (const u of list) {
    const depts = (u?.departments || []).map((d) => (typeof d === 'string' ? d : String(d?.name ?? '')))
    depts.forEach(push)
    if (company && sameCompany(u?.name, company, companyNamesOf(list))) {
      matched = depts.filter(Boolean)
    }
  }
  return matched.length ? [...new Set(matched)] : all
}

/**
 * 第 64 轮：严格版部门取值 —— 该公司没配置部门就返回空数组。
 *
 * 为什么不再回退全部部门：宿舍/公租房档案的「所属部门」必须与「所属公司」同源，
 * 选「重庆持睿汽车部件有限公司」却还能挑到星达的人事行政部，属于跨公司串台。
 */
export function departmentsOfStrict(
  units: readonly OrgUnitLike[] | null | undefined,
  company: string,
): string[] {
  const list = units || []
  const names = companyNamesOf(list)
  if (!company) return []
  for (const u of rnUnits(list)) {
    if (!sameCompany(u?.name, company, names)) continue
    return (u?.departments || [])
      .map((d) => (typeof d === 'string' ? d : String(d?.name ?? '')))
      .map((v) => String(v ?? '').trim())
      .filter(Boolean)
  }
  return []
}

/** 内部：把 units 归一为数组（容忍 null / 非数组）。 */
function rnUnits(units: readonly OrgUnitLike[] | null | undefined): OrgUnitLike[] {
  return Array.isArray(units) ? [...units] : []
}

export type MeterScope = 'dorm' | 'public' | ''

/** 用途标签 → 归属场景：宿舍 / 公租房 / 其它（工商业等）。 */
export function meterKindScope(label: unknown): MeterScope {
  const s = String(label ?? '').trim()
  if (!s) return ''
  if (s.includes('宿舍')) return 'dorm'
  if (s.includes('公租房')) return 'public'
  return ''
}

export interface MeterLike {
  meter_kind?: string
  /** 第 64 轮：过滤一律比「使用单位」，绝不比「归属单位」（owner_unit）。 */
  use_unit?: string
  usage_unit?: string
  owner_unit?: string
  enabled?: boolean
  alias?: string
  name?: string
}

export interface KindLike {
  value?: string
  label?: string
}

/**
 * 按【用途 + 使用单位】过滤可关联表计。
 *
 * @param meters 表计列表（能耗管理 utility_meters）
 * @param kinds  用途列表（内置 dorm=宿舍 / production=工商业 + 自定义公租房）
 * @param scope  当前档案类型：宿舍 / 公租房
 * @param company 当前抽屉所选「所属公司」
 */
export function filterMetersByScopeAndCompany(
  meters: readonly MeterLike[] | null | undefined,
  kinds: readonly KindLike[] | null | undefined,
  scope: MeterScope,
  company: string,
  names: readonly string[] = [],
): MeterLike[] {
  if (!scope) return []
  let kindList = (kinds || []).filter((k) => meterKindScope(k?.label) === scope)
  if (!kindList.length) {
    // 用途接口未就绪 / 未落库时的兜底：内置宿舍 = dorm
    if (scope === 'dorm') kindList = [{ value: 'dorm', label: '宿舍' }]
    // 第 65 轮：public 没有专属用途时返回空，由调用方回退到 dorm（公租房共用宿舍表）
  }
  const wanted = new Set(kindList.map((k) => String(k?.value ?? '').trim()).filter(Boolean))
  if (!wanted.size) return []
  return (meters || []).filter((m) => {
    if (!m || m.enabled === false) return false
    if (!wanted.has(String(m.meter_kind ?? '').trim())) return false
    if (!company) return true
    // 第 64 轮：只认使用单位。表计的「归属单位」是资产归属（持睿403 的归属单位是星达），
    // 错比 owner_unit 会把持睿宿舍表挂到星达档案上，反之亦然。
    const useUnit = String(m.use_unit ?? m.usage_unit ?? '').trim()
    if (!useUnit) return false
    return sameCompany(useUnit, company, names)
  })
}

/** 表计展示名：优先 alias，其次 name。 */
export function meterLabelOf(m: MeterLike | null | undefined): string {
  return String(m?.alias ?? m?.name ?? '').trim()
}
