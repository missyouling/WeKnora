/**
 * 车队保险单：交强险 / 商业险 / 险种类别 三个配置标签的纯函数层（第 76 轮）。
 *
 * 数据形态：仍然只有一个 `保险单` 分类（记录、列表、提取全部按它走），
 * 字段用 subs.group 携带所属标签：
 *   · `交强险/基本信息`、`交强险/被保险机动车`、`交强险/责任限额`
 *   · `商业险/基本信息`、`商业险/被保险机动车`、`商业险/投保险别`
 *   · `险种类别`
 *
 * 为什么不拆成三个分类：`fleet_records.sub_category / doc_type` 都是分类名，
 * 拆分类会让存量保单在列表里「消失」；而同一分类下字段名必须唯一
 * （列表列 / 编辑抽屉都按 name 取用），所以商业险字段用 `商业险_` 前缀、
 * 投保险别用 `投保险别_` 前缀做身份隔离，展示名（label）仍是业务原名。
 */

export const INSURANCE_CATEGORY = '保险单'
export const INSURANCE_TABS = ['交强险', '商业险', '险种类别'] as const
export type InsuranceTab = typeof INSURANCE_TABS[number]

export const INSURANCE_TYPE_GROUP = '险种类别'
export const INSURANCE_COVERAGE_GROUP = '商业险/投保险别'
export const INSURANCE_COVERAGE_PREFIX = '投保险别_'

/** 字段所属标签（group 的第一段；险种类别组没有斜杠）。 */
export function insuranceTabOfGroup(group: unknown): string {
  const g = String(group ?? '').trim()
  if (!g) return ''
  const i = g.indexOf('/')
  return i < 0 ? g : g.slice(0, i)
}

/** 标签内的分组显示名（去掉「交强险/」前缀）。 */
export function insuranceGroupLabel(group: unknown): string {
  const g = String(group ?? '').trim()
  const i = g.indexOf('/')
  return i < 0 ? g : g.slice(i + 1)
}

/** 该字段是否属于当前配置标签。 */
export function insuranceFieldInTab(field: { group?: unknown } | null | undefined, tab: string): boolean {
  const want = String(tab || '').trim()
  if (!want) return true
  return insuranceTabOfGroup(field?.group) === want
}

interface EditableLike {
  name: string
  label?: string
  enabled: boolean
  isDefault: boolean
  dataType: string
  width: number
  group?: string
  groupLabel?: string
  [k: string]: any
}

/**
 * 把「险种类别」同步到「商业险 / 投保险别」。
 *
 * 规则（严格对齐需求）：
 *   · 险种类别新增 → 投保险别新增同名一项（amount，默认列）；
 *   · 险种类别改名 → 投保险别跟随改 label（name 以类型名为身份，改名视为新类型）；
 *   · 险种类别停用 → 投保险别保留但同样停用（历史数据仍可显示）；
 *   · 险种类别调序 → 投保险别按同一顺序重排；
 *   · 险种类别删除 → 投保险别移除对应项（历史记录里的值仍留在 data 里）。
 *
 * 已存在的投保险别项保留用户改过的宽度 / 类型 / 默认表头，只同步顺序与启停。
 */
export function syncInsuranceCoverageFields<T extends EditableLike>(fields: T[]): T[] {
  const types = fields.filter((f) => String(f.group ?? '').trim() === INSURANCE_TYPE_GROUP)
  if (!types.length) return fields
  const byType = new Map<string, T>()
  for (const f of fields) {
    const g = String(f.group ?? '').trim()
    if (g !== INSURANCE_COVERAGE_GROUP) continue
    const raw = String(f.name || '')
    const key = raw.startsWith(INSURANCE_COVERAGE_PREFIX) ? raw.slice(INSURANCE_COVERAGE_PREFIX.length) : raw
    byType.set(key, f)
  }
  const desired: T[] = types.map((t) => {
    const key = String(t.name || '').trim()
    const hit = byType.get(key)
    if (hit) {
      // 启停跟随险种类别；宽度/类型/顺序保留用户配置
      return { ...hit, label: String(t.label || key), enabled: t.enabled !== false }
    }
    return {
      name: `${INSURANCE_COVERAGE_PREFIX}${key}`,
      label: String(t.label || key),
      enabled: t.enabled !== false,
      isDefault: true,
      dataType: 'amount',
      width: 0,
      group: INSURANCE_COVERAGE_GROUP,
      groupLabel: '商业险 · 投保险别',
    } as unknown as T
  })
  const out: T[] = []
  let inserted = false
  for (const f of fields) {
    if (String(f.group ?? '').trim() !== INSURANCE_COVERAGE_GROUP) {
      out.push(f)
      continue
    }
    if (!inserted) {
      out.push(...desired)
      inserted = true
    }
  }
  if (!inserted) out.push(...desired)
  return out
}

/** 该分类是否需要渲染 3 个保险配置标签。 */
export function isInsuranceCategory(name: unknown): boolean {
  return String(name ?? '').trim() === INSURANCE_CATEGORY
}
