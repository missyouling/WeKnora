// 能耗 6 个卡片视图的「列表列 ← 字段定义（subs）」派生层。
//
// 为什么必须收口到一处：
//   `电费核算 / 水费核算 / 气费核算 / 费用清单 / 光伏账单` 五张表的列原先各自
//   硬编码在组件里（COLUMN_DEFS / COL_DEFS_OWNER / FALLBACK_COLUMNS），
//   而设置抽屉读的是 `fleet_categories.subs`——两套数据源，用户在「字段定义」里
//   改名、改宽度、拖顺序、开关默认显示，列表纹丝不动（真机：改了「电表」还是
//   显示「表计」）。这里把 subs 变成列定义的唯一权威源，组件只提供
//   「渲染器 + 缺省宽度」。
//
// 铁律：
//   · 集合 / 顺序 / 名称 / 显隐 / 宽度 一律以 subs 为准；
//   · 组件静态表只提供 subs 里没有的缺省值（宽度比例、格式化类型、tooltip）；
//   · subs 里被「停用」的字段不产出列；subs 里没有的字段不产出列。
/** subs 条目的最小形态（后端 `FleetCategorySub` 的 JSON 形态）。 */
export interface EnergySub {
  name: string
  label?: string
  enabled?: boolean
  is_default?: boolean
  data_type?: string
  width?: number
  group?: string
  group_label?: string
}

/** 列表列定义的最小形态（各组件自带更丰富的扩展字段）。 */
export interface EnergyViewCol {
  key: string
  label: string
  default: boolean
  width?: number
}

/** 一次取回 energy scope 下所有分类（含 subs）。 */
export async function loadEnergyCategories(): Promise<any[]> {
  // 动态 import：本文件同时被单元测试直接 import（纯函数部分），
  // 静态依赖 `@/api/fleet` 会让 node:test 的 tsx 解析别名失败。
  const { listFleetCategories } = await import('@/api/fleet')
  const res: any = await listFleetCategories({ scope: 'energy' })
  const list = Array.isArray(res) ? res : res?.data
  return Array.isArray(list) ? list : []
}

/** 取某个分类的 subs；分类不存在或未配置字段时返回空数组。 */
export async function loadEnergyCategorySubs(categoryName: string): Promise<EnergySub[]> {
  const want = String(categoryName || '').trim()
  if (!want) return []
  const list = await loadEnergyCategories()
  const cat = list.find((c: any) => String(c?.name || '').trim() === want && !c?.deleted_at)
  const subs = Array.isArray(cat?.subs) ? cat!.subs : []
  return subs.filter((s: any) => s && String(s.name || '').trim())
}

/** subs 是否真的能派生列（空数组 = 该分类还没配字段，调用方回退静态列）。 */
export function hasUsableSubs(subs: readonly EnergySub[] | undefined): boolean {
  return Array.isArray(subs) && subs.some((s) => String(s?.name || '').trim() && s.enabled !== false)
}

export interface ApplySubsOptions {
  /**
   * 只保留静态表里已有的键。
   *
   * 费用清单有「业主视角 / 租户视角」两套列，subs 是并集；不过滤的话，
   * 业主视角会多出只在租户视角有意义的「分摊比例」空列。
   * 核算 / 光伏是单视角，用户新增的字段必须能出现在表头，因此不开启。
   */
  restrictToBase?: boolean
}

/**
 * 把 subs 落到组件自己的列定义结构上。
 *
 * @param base       组件的静态列定义（提供 w / tip / fieldType / only 等缺省值）
 * @param subs       该分类当前保存的字段定义
 * @param makeExtra  静态表里没有的字段 → 组件列对象（用户新增字段）
 */
export function applySubsToColumns<T extends EnergyViewCol>(
  base: readonly T[],
  subs: readonly EnergySub[] | undefined,
  makeExtra?: (key: string, sub: EnergySub) => T,
  opts: ApplySubsOptions = {},
): T[] {
  if (!hasUsableSubs(subs)) return [...base]
  const byKey = new Map(base.map((c) => [String(c.key), c]))
  const out: T[] = []
  const seen = new Set<string>()
  for (const s of subs || []) {
    const key = String(s?.name || '').trim()
    if (!key || seen.has(key)) continue
    if (s.enabled === false) continue
    const hit = byKey.get(key)
    if (!hit && opts.restrictToBase) continue
    seen.add(key)
    const label = String(s.label || '').trim()
    const width = Number(s.width) > 0 ? Math.round(Number(s.width)) : 0
    if (hit) {
      out.push({
        ...hit,
        label: label || hit.label,
        // is_default 以 subs 为权威：用户在「默认表头」里取消勾选后列表必须收起该列
        default: s.is_default === true,
        width: width || hit.width,
      })
    } else if (makeExtra) {
      out.push({ ...makeExtra(key, s), label: label || key, default: s.is_default === true, width })
    }
  }
  return out
}

/**
 * 「打开能耗设置抽屉」的跨组件事件。
 *
 * 为什么用 window 事件而不是 props 逐层传：6 个卡片视图分别渲染在
 * UtilitiesManagement / SolarManagement / UtilityMeterTab / TenantBilling 里，
 * 层级最深的有三层。设置抽屉只挂在页面根部一份（单一实例，避免多开），
 * 子组件只需要喊一声"我要改这个分类的字段"。
 */
export const ENERGY_OPEN_SETTINGS_EVENT = 'energy-open-settings'

/** 请求打开能耗设置抽屉（字段定义 / 提取规则），并定位到指定分类。 */
export function requestEnergySettings(category: string): void {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(ENERGY_OPEN_SETTINGS_EVENT, { detail: { category: String(category || '') } }))
}

