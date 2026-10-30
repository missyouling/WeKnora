/**
 * 费用清单「跨分类字段引用」的取值层。
 *
 * 业务定位：费用清单是**综合汇总列表**，它自己的字段大多没有独立存储 ——
 * 要么是市电账单的费用分项，要么是光伏/电/水/气核算的月度汇总。
 * 所以「字段定义」里给费用清单加字段，本质是**引用别的分类已有字段**，
 * 并用这里定义的规则把值算出来。
 *
 * 三条取值口径（与产品确认一致）：
 *   ① 市电账单 · 费用分项（fee_row）：按费用组成名称匹配当月账单的 fee_items 求和；
 *   ② 市电账单 · 概况字段（field）：直接读账单概况；
 *   ③ 核算类（电/水/气）· 普通字段：按「月份 + 使用单位 = 当前租户」过滤后求和；
 *      单价类取「金额 ÷ 用量」的加权均值，倍率等取首值。
 *   ④ 光伏账单（全厂公共，无使用单位）：当月账单同字段求和。
 *
 * 为什么在前端算而不新增后端聚合接口：这些数据集（市电账单、光伏账单、
 * 电/气抄表明细、水表明细）本来就已经为列表加载过了，再让后端重复聚合一遍
 * 只会引入第二套口径 —— 而口径漂移正是这个模块历史上最多的 bug 来源。
 */

export interface FieldSource {
  /** 来源分类（市电账单 / 光伏账单 / 电费核算 / 水费核算 / 气费核算） */
  category: string
  /** 来源字段 key；mode=fee_row 时是费用组成名称 */
  field: string
  /**
   * fee_group = 「大类汇总列」：它不对应任何真实字段，而是把 `members`
   * 里那几条市电分项折叠成一个可展开的大类（市场化购电费 / 系统运行费用 /
   * 政府性基金及附加）。值为成员分摊后金额之和。
   */
  mode: 'field' | 'fee_row' | 'fee_group'
  /**
   * 来源字段在原分类里所属的分组（市电账单的 12 个费用分项）。
   *
   * 必须带上：市电账单 8 个分组共用同一批列名（`qty`「计费电量」在 7 个分组里
   * 各有一条），只按 field 名称取值会跨组求和、金额被放大数倍；
   * 候选下拉里也会出现 8 条无法分辨的同名项。
   */
  group?: string
  groupLabel?: string
  /** 来源字段的原始名称（费用清单里可能已改名，回溯/悬浮要显示它）。 */
  label?: string
  /** 大类汇总列的成员字段名（仅 mode=fee_group；与 subs 里的 name 一致）。 */
  members?: string[]
}

export const SOURCE_CATEGORIES = ['市电账单', '光伏账单', '电费核算', '水费核算', '气费核算'] as const

/** 从 subs 条目里读出来源描述；老数据没有 source 返回 null（= 内置派生列）。 */
export function parseFieldSource(sub: any): FieldSource | null {
  const raw = sub?.source
  if (!raw || typeof raw !== 'object') return null
  const category = String(raw.category || '').trim()
  const field = String(raw.field || '').trim()
  if (!category || !field) return null
  const rawMode = String(raw.mode || '').trim()
  const mode: FieldSource['mode'] = rawMode === 'fee_row' ? 'fee_row' : rawMode === 'fee_group' ? 'fee_group' : 'field'
  const src: FieldSource = { category, field, mode }
  if (mode === 'fee_group') {
    const members = Array.isArray(raw.members) ? raw.members.map((m: any) => String(m || '').trim()).filter(Boolean) : []
    if (members.length) src.members = members
  }
  const group = String(raw.group || raw.groupLabel || '').trim()
  if (group) {
    src.group = group
    const gl = String(raw.group_label || raw.groupLabel || '').trim()
    if (gl) src.groupLabel = gl
  }
  const label = String(raw.label || '').trim()
  if (label) src.label = label
  return src
}

/**
 * 来源的完整路径文本：`分类 · 分组`（有分组时）。
 * 候选下拉与悬浮提示共用同一份拼法，避免两处文案漂移。
 */
export function sourcePathText(src: FieldSource | null, fallback = ''): string {
  if (!src) return fallback
  const parts = [src.category]
  const g = src.groupLabel || src.group
  if (g) parts.push(g)
  return parts.join(' · ')
}

/** 引用字段的展示名：`[来源分类 · 分组] 字段名`（防同名误选）。 */
export function sourceBadge(src: FieldSource | null, fallback = ''): string {
  if (!src) return ''
  return `[${sourcePathText(src)}] ${fallback || src.field}`
}

const norm = (v: any) => String(v ?? '').trim().toLowerCase().replace(/\s+/g, '')
const num = (v: any) => {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}
const round2 = (v: number) => Math.round(v * 100) / 100

/**
 * 名称归一（用于费用分项匹配）：在 `norm` 之上再去掉括号、统一同义词。
 *
 * 账单原文与用户配置的名称常年对不上，两个真实踩过的例子：
 *   · 「功率因**数**调整电费」（账单）vs「功率因**素**调整电费」（配置）；
 *   · 「输配容（需）量电费」（账单细项名）vs「基本电费」（分摊子项名）。
 */
// 同义词归一：账单原文与用户配置常年写法不同。已收的真实差异：
//   ① 括号全半角；
//   ② 「功率因**数**调整电费」vs「功率因**素**调整电费」；
//   ③ 「其**它**系统运行费用」vs「其**他**系统运行费用」（国网两种写法都出现过）。
// 归一后仍按「名称」精确匹配，不按位置下标 —— 下标匹配在合并单元格被拆成
// 多列时必然错位（2026-09 (4)系统运行费 张冠李戴的根因）。
const aliasNorm = (v: any) => norm(v)
  .replace(/[（）()]/g, '')
  .replace(/因素/g, '因数')
  .replace(/其它/g, '其他')

/**
 * 账单里「固定金额、不挂在 fee_items 行上」的分项 → 概况字段。
 *
 * 为什么必须单独处理：`基本电费`（输配容量/需量电费）与 `功率因数调整电费`
 * 在账单里**不是可靠的 fee_items 行** —— 2026-09 的 43 行里完全没有它们
 * （权威值在顶层 `capacity_fee=44000` / `pf_adjust_amount=-2818.07`）；
 * 2026-08 又反过来，有一条 `name:'功率因数调整', fee:0}` 的**空占位行**。
 * 所以只按名称匹配 fee_items 会出现两种错法：要么恒为 0，要么被 0 行钉死。
 * 唯一可靠来源是概况字段，因此**优先**读它（见 resolveFeeRow）。
 *
 * 返回 `null` = 该名称不属于这两类，调用方继续走 fee_items 口径。
 */
const fixedBillFee = (ctx: SourceResolveContext, name: string): number | null => {
  const key = aliasNorm(name)
  if (!key) return null
  const item = ctx.bill?.item || {}
  // 别名刻意收紧：只认「基本电费」与输配容/需量口径。
  // 不能收录裸「容量电费」——「燃气机组容量电费」「煤电容量电费」等是
  // 系统运行费下的正常 fee_items 行，收进来会把它们全部改写成容量电费。
  const CAPACITY_ALIASES = ['基本电费', '输配容需量电费', '输配容需量', '输配需量电费', '输配容量电费']
  const PF_ALIASES = ['因数调整电费', '力调电费', '功率因数调整']
  if (CAPACITY_ALIASES.some((a) => key.includes(a))) {
    return num(item.capacity_fee ?? item.basic_fee ?? item.demand_fee)
  }
  if (PF_ALIASES.some((a) => key.includes(a))) {
    return num(item.pf_adjust_amount ?? item.pf_adj_fee ?? item.power_factor_fee)
  }
  return null
}

export interface SourceResolveContext {
  month: string
  /** 当前选中的使用单位（租户视角 = 租户名；业主视角 = 重庆星达） */
  tenantName: string
  /** 当月市电账单行（{ item }），没有则 null */
  bill: any
  /** 当月光伏账单行数组 */
  solarItems: any[]
  /** 电费核算明细行（已展平：month / use_unit / meter_kind / usage / amount / unit_price / meter_type） */
  elecRows: any[]
  /** 水表档案与读数索引（`meterId__month` → reading） */
  waterMeters: any[]
  waterReadingMap: Map<string, any>
  /** 气费核算明细行 */
  gasRows: any[]
  /**
   * 该租户当月分时电表的逐段抄见电量（meter_type=time 的表）。
   * 分时电费（零售交易电费 / 零售输配电费等）必须按「逐表逐时段基准电量 × 时段单价」
   * 计算，光有整月合计金额是算不出来的。
   */
  timedMeters?: Array<{ name: string; usage: Record<string, number> }>
  /** 该月差额电量 ΔQ = 星达分表总电量 − 各分表抄见合计（取自已生成账单 line_loss）。 */
  deltaQ?: number
}

/** 单价类字段：不能直接求和，按 金额÷用量 加权。 */
const PRICE_FIELDS = new Set(['unit_price', 'price', 'avg_price', 'ind_price', 'default_unit_price'])
/** 取首值的字段（倍率/表号等非可加量）。 */
const FIRST_VALUE_FIELDS = new Set(['rate', 'meter_no', 'meter_type', 'meter_kind', 'install_date', 'reading_date', 'reader'])
/**
 * 「使用单位」类字段：值恒等于当前核算单位。
 *
 * 它出现在费用清单的默认分组里（引用自电费核算），而**某个月没有该单位的抄表
 * 记录**时明细行是空的 —— 旧实现 `if (!rows.length) return 0` 于是整列显示 0
 * （真机：持睿汽车视角下 2026-06 ~ 2025-08 十行「使用单位」全是 0）。
 * 使用单位是行的身份，不是聚合值，没有明细也必须显示单位名。
 */
const UNIT_FIELDS = new Set(['use_unit', 'unit'])

function aggregateRows(rows: any[], field: string, fallbackUnit = ''): number | string {
  if (!rows.length) {
    if (UNIT_FIELDS.has(field)) return String(fallbackUnit || '').trim()
    return 0
  }
  // 文本字段（使用单位 / 抄表人 / 表计名称 / 抄表方式…）**不能求和**。
  //
  // 旧实现一律走 `sum(num(r[field]))`，`num('重庆星达')` = 0 —— 于是
  // 「[电费核算] 使用单位」整列显示 0（真机问题 3）。这里先判定列的取值形态：
  // 只要不是「全部能当数字」，就按文本处理：去重后用「、」连接（单值即原样）。
  const raw = rows
    .map((r) => r?.[field])
    .filter((v) => v !== null && v !== undefined && String(v).trim() !== '')
  const allNumeric = raw.length > 0 && raw.every((v) => !Number.isNaN(Number(v)))
  if (!allNumeric && raw.length) {
    const uniq = [...new Set(raw.map((v) => String(v).trim()))]
    return uniq.join('、')
  }
  if (PRICE_FIELDS.has(field)) {
    const qty = rows.reduce((s, r) => s + (num(r.usage) || num(r.qty)), 0)
    const amount = rows.reduce((s, r) => s + (num(r.amount) || num(r.fee) || num(r.usage) * num(r.unit_price)), 0)
    if (qty > 0 && amount > 0) return Math.round((amount / qty) * 10000) / 10000
    const prices = rows.map((r) => num(r[field])).filter((v) => v > 0)
    return prices.length ? prices[0] : 0
  }
  if (FIRST_VALUE_FIELDS.has(field)) return num(rows[0]?.[field])
  return round2(rows.reduce((s, r) => s + num(r[field]), 0))
}

/**
 * 在当月账单 `fee_items` 里找出与该名称对应的行（可能多行，如分时 4 段）。
 *
 * **唯一匹配口径**：列表列取值与打印清单细项都走这里。分散写两套匹配规则
 * 是这个模块历史上最多的 bug 来源（「列表有值、打印没有」这类漂移）。
 *
 * 三档递进，命中即返回，不再往下走：
 *   ① 精确同名（含「大类|子项名」取竖线后半段）；
 *   ② 前缀包含（费用分项常带 `(1)` 前缀或后缀差异）；
 *   ③ 别名归一（括号、因数字/因素字）。
 * 都不命中返回 `[]`。调用方按 `fixedBillFeeOf` 决定是否改走概况字段。
 */
/**
 * 账单「费用类别」归一（用于**按分组**圈定匹配范围）。
 *
 * 为什么必须有它：`(6)政府性基金及附加` 下 5 个基金子项在「工商业」与「居民」
 * 两个分组里**同名同价**（农网还贷 0.02、国家重大水利 0.001969…），只是计费电量
 * 一个 58.7 万、一个 8,938。旧实现只按名称匹配，于是把两组同名行一起求和：
 * 农网还贷 11,744.60（工商业）+ 178.76（居民）= 11,923.36（真机错值）。
 * 分组名本身就是账单原文里的 `(6)…-工商业` / `(6)…-居民`，属于强约束。
 */
const catNorm = (v: any) => norm(v).replace(/[（）()]/g, '').replace(/因素/g, '因数')

/** 账单行是否属于该分组（group 空 = 不约束）。 */
function rowInGroup(rowCategory: unknown, group?: string | null): boolean {
  const want = catNorm(group)
  if (!want) return true
  const got = catNorm(rowCategory)
  if (!got) return false
  return got === want || got.includes(want) || want.includes(got)
}

export function matchBillFeeItems(ctx: SourceResolveContext, name: string, group?: string | null): any[] {
  const all: any[] = ctx.bill?.item?.fee_items || []
  // 先按分组圈定，再做名称匹配：跨分组同名（工商业/居民政府基金）绝不允许混加
  const items = group ? all.filter((it) => rowInGroup(it?.category, group)) : all
  const want = norm(name)
  if (!want) return []
  const parts = (it: any) => [norm(it?.name), norm(String(it?.name || '').split('|').pop())]
  const exact = (it: any) => parts(it).some((p) => p === want)
  const loose = (it: any) => parts(it).some((p) => p && (p.includes(want) || want.includes(p)))
  const wantAlias = aliasNorm(name)
  const aliasParts = (it: any) => [aliasNorm(it?.name), aliasNorm(String(it?.name || '').split('|').pop())]
  const alias = (it: any) => aliasParts(it).some((p) => p === wantAlias || (p && (p.includes(wantAlias) || wantAlias.includes(p))))
  for (const test of [exact, loose, alias]) {
    const hit = items.filter(test)
    if (hit.length) return hit
  }
  return []
}

/**
 * 分组的强归一：去序号前缀、去「性」、去标点，只留中英文数字。
 *
 * 分组名来自「字段定义」的分组胶囊条，账单 `fee_items.category` 来自 PDF 原文，
 * 同一个分组在两处的写法常年不同。真机取证的真实差异：
 *   分组「政府基金及附加（工商业）」 vs 账单「(6)政府性基金及附加-工商业」
 *   分组「目录电费（居民）」        vs 账单「(5)目录电费」
 *   分组「功率因素调整」            vs 账单「功率因数调整电费」
 * 只做「包含」判断会全部落空（于是分组取值退化成 0）。这里统一掉这三类噪声：
 *   ① 去掉 `(1)` / `1.` 这类序号前缀；
 *   ② 去掉「性」（政府性基金 = 政府基金）；
 *   ③ 去掉括号、连字符、空格。
 */
const groupNorm = (v: any) => String(v ?? '')
  .toLowerCase()
  .replace(/^\s*[（(]?\d+[)）]?[.、]?\s*/g, '')
  .replace(/性/g, '')
  .replace(/[\s（）()\-—_·・,，.。/\\|]+/g, '')

/**
 * 分组**键** → 账单 `fee_items.category` 的中文别名。
 *
 * 为什么需要：`source.group` 存的是字段定义里的分组键（`catalog` / `market` /
 * `gov-industrial`…），而账单 `fee_items.category` 是 PDF 原文（`(5)目录电费`、
 * `(1)市场化购电费`、`(6)政府性基金及附加-工商业`）。拿键去和中文分类比，
 * 一条都匹配不上 → 分组取值恒为 null → 列全空（真机：目录电费（居民）·计费电量）。
 *
 * 取值顺序是「group_label → group → 本别名表」：旧数据可能只存了键，
 * 新数据两者都有；label 是人工可读名，匹配成功率最高。
 */
const BILL_GROUP_KEY_ALIASES: Record<string, readonly string[]> = {
  overview: ['账单概况'],
  market: ['市场化购电费'],
  line: ['上网环节线损费用', '上网环节线损费'],
  trans: ['输配电量电费', '输配电费', '零售输配电费'],
  sys: ['系统运行费'],
  catalog: ['目录电费', '目录电费（居民）'],
  'gov-industrial': ['政府性基金及附加-工商业', '政府基金及附加（工商业）'],
  'gov-residential': ['政府性基金及附加-居民', '政府基金及附加（居民）'],
  capacity: ['输配容（需）量电费', '基本电费', '输配容需量电费'],
  pf: ['功率因数调整电费', '功率因素调整电费', '力调电费'],
  meter: ['电量明细（工商业）', '电量明细（居民）', '电量明细'],
  'meter-rows': ['电量明细（工商业）', '电量明细（居民）'],
}

/** 按分组取值的候选名（label 优先，其次键，最后别名表）。 */
function billGroupCandidates(group?: string, groupLabel?: string): string[] {
  const out: string[] = []
  const push = (v?: string) => {
    const s = String(v || '').trim()
    if (s && !out.includes(s)) out.push(s)
  }
  push(groupLabel)
  push(group)
  for (const alias of BILL_GROUP_KEY_ALIASES[String(group || '').trim()] || []) push(alias)
  return out
}

/**
 * 按「分组 + 字段」定位市电账单里的费用分项（带 group 的引用字段用它）。
 *
 * 市电账单 8 个分组共用 `qty` / `rate` / `fee` 等列名，「计费电量」在 7 个分组里
 * 各有一条：不带分组只能全表求和（金额被放大数倍），带了才能精确到那一组。
 *
 * 匹配优先级：强归一全等 → 双向包含（容忍「上网环节线损费」vs「…费用」这类后缀差）。
 * 都不中返回 `null`，调用方退回「概况字段 / 全分类同名」的老口径
 * —— 「账单概况」这类分组本来就不在 fee_items 里，必须让它落到概况字段。
 */
function resolveByBillGroup(ctx: SourceResolveContext, group: string, field: string): number | null {
  const want = groupNorm(group)
  if (!want) return null
  const item = ctx.bill?.item || {}
  const fieldKey = String(field || '').trim()

  // 「电量明细（工商业/居民）」不是 fee_items 的行，而是 meter_readings /
  // residential_readings 数组：市电账单的示数表按示数类型分行，字段就是列名。
  // 不单独处理的话，这组字段（计费电量/抄见电量/上期示数…）会永远取到 0。
  if (want.includes('电量明细')) {
    const isResident = want.includes('居民')
    const rows: any[] = isResident
      ? (Array.isArray(item.residential_readings) ? item.residential_readings : [])
      : (Array.isArray(item.meter_readings) ? item.meter_readings : [])
    if (!rows.length) return null
    // 居民示数表用 bill_kwh/kwh/ratio，工商业用 bill_kwh/reading_kwh/curr…
    if (fieldKey === 'bill_kwh') return round2(rows.reduce((s, r) => s + num(r?.bill_kwh), 0))
    if (fieldKey === 'reading_kwh') return round2(rows.reduce((s, r) => s + num(r?.reading_kwh ?? r?.kwh), 0))
    if (fieldKey === 'qty' || fieldKey === 'kwh') return round2(rows.reduce((s, r) => s + num(r?.kwh ?? r?.reading_kwh), 0))
    if (fieldKey === 'curr') return round2(rows.reduce((s, r) => s + num(r?.curr), 0))
    if (fieldKey === 'prev') return round2(rows.reduce((s, r) => s + num(r?.prev), 0))
    if (fieldKey === 'adjust') return round2(rows.reduce((s, r) => s + num(r?.adjust), 0))
    const first = rows[0]?.[fieldKey]
    return first === undefined ? null : round2(num(first))
  }

  const items: any[] = item.fee_items || []
  if (!items.length) return null
  const catOf = (it: any) => groupNorm(it?.category)
  let inGroup = items.filter((it) => catOf(it) === want)
  if (!inGroup.length) {
    inGroup = items.filter((it) => {
      const c = catOf(it)
      return !!c && (c.includes(want) || want.includes(c))
    })
  }
  if (!inGroup.length) return null
  if (fieldKey === 'fee' || fieldKey === 'fee_amount') {
    return round2(inGroup.reduce((s, it) => s + num(it?.fee ?? it?.fee_amount), 0))
  }
  if (fieldKey === 'qty') return round2(inGroup.reduce((s, it) => s + num(it?.qty), 0))
  if (fieldKey === 'rate') {
    // 单价不可相加：同价分段直接用账单原值（`fee ÷ qty` 会因金额已四舍五入
    // 产生 0.500305 vs 0.500306 这类末位漂移，用户对着账单核不上）；
    // 真正的分时差价（尖/峰/平/谷各不同）才用「金额 ÷ 数量」加权。
    const rates = inGroup.map((it) => num(it?.rate)).filter((v) => v > 0)
    const uniq = new Set(rates.map((v) => v.toFixed(6)))
    if (uniq.size === 1) return rates[0]
    const qty = inGroup.reduce((s, it) => s + num(it?.qty), 0)
    const fee = inGroup.reduce((s, it) => s + num(it?.fee), 0)
    if (qty > 0) return Math.round((fee / qty) * 1000000) / 1000000
    return num(inGroup[0]?.rate)
  }
  // 其余键（name/period 等）取首值：它们不是可加量
  const first = inGroup[0]?.[fieldKey]
  return first === undefined ? null : round2(num(first))
}

/**
 * 固定金额分项（基本电费 / 力调电费）的权威值；不属于这两类返回 `null`。
 *
 * 打印清单用它决定走「概况字段 × 比例」还是「账单行 × 比例」：这两项在账单里
 * 没有可靠的 fee_items 行（2026-09 完全没有；2026-08 是一条全 0 的占位行），
 * 按行打印会打出 0.00 的假明细。
 */
export function fixedBillFeeOf(ctx: SourceResolveContext, name: string): number | null {
  return fixedBillFee(ctx, name)
}

/**
 * 账单里该分项的**原始金额**（未乘分摊比例）。
 *
 * 浮窗文案与打印清单都要写「原始金额 × 比例 = 本行金额」，此前浮窗调用点
 * 只把「固定金额分项」的概况值传了进去，其余分项一律 `undefined` → `Number()
 * || 0` → 文案变成 `= 0 × 21.24% = 4,434.02 元`（真机截图，逻辑自相矛盾）。
 * 统一从这里取，保证「账单原始金额」只有一处口径。
 */
export function billRawAmountOf(ctx: SourceResolveContext, name: string, group?: string | null): number {
  return resolveFeeRow(ctx, name, group)
}

/** 当月市电账单的费用分项求和（mode=fee_row）。 */
function resolveFeeRow(ctx: SourceResolveContext, name: string, group?: string | null): number {
  // ① 固定金额分项（基本电费 / 力调电费）**优先**读概况字段。
  //    不能等 fee_items 匹配失败再兜底：账单里可能存在同名的空行
  //    （2026-08 的「功率因数调整」fee=0），会把权威值挡掉。
  const fixed = fixedBillFee(ctx, name)
  if (fixed !== null) return round2(fixed)
  // ② 其余分项按「分组 + 名称」匹配求和
  const hit = matchBillFeeItems(ctx, name, group)
  return hit.length ? round2(hit.reduce((s, it) => s + num(it?.fee), 0)) : 0
}

/**
 * 解析一个引用字段在某月某单位下的值。
 *
 * 值型字段返回数字（保留两位）；文本型（备注/抄表人等）返回字符串。
 * 找不到数据一律返回 0 / ''，让列表显示 `—` 而不是 NaN。
 */
export function resolveSourceValue(src: FieldSource, ctx: SourceResolveContext): number | string {
  switch (src.category) {
    case '市电账单': {
      if (src.mode === 'fee_row') return resolveFeeRow(ctx, src.field)
      // 带分组 = 该分组的费用分项列（qty/rate/fee…）：
      // 必须按分组定位，否则「计费电量」会把 7 个分组的数量全加起来。
      // 候选顺序 label → 键 → 键别名：旧数据可能只存了 `catalog` 这类键，
      // 直接拿键去撞中文分类必然落空（真机：目录电费（居民）·计费电量 全空）。
      for (const g of billGroupCandidates(src.group, src.groupLabel)) {
        const grouped = resolveByBillGroup(ctx, g, src.field)
        if (grouped !== null) return grouped
      }
      const v = ctx.bill?.item?.[src.field]
      if (v === undefined || v === null || v === '') {
        // 概况别名：账单电费在不同管线里叫 grand_total / total_amount
        if (src.field === 'total_amount') return num(ctx.bill?.item?.grand_total ?? ctx.bill?.item?.total_amount)
        return 0
      }
      return typeof v === 'number' ? round2(v) : v
    }
    case '光伏账单': {
      if (!ctx.solarItems.length) return 0
      if (FIRST_VALUE_FIELDS.has(src.field)) return num(ctx.solarItems[0]?.item?.[src.field])
      return round2(ctx.solarItems.reduce((s, r) => s + num(r?.item?.[src.field]), 0))
    }
    case '电费核算': {
      const rows = ctx.elecRows.filter((r) => r.month === ctx.month && (!ctx.tenantName || r.use_unit === ctx.tenantName))
      return aggregateRows(rows, src.field, ctx.tenantName)
    }
    case '气费核算': {
      const rows = ctx.gasRows.filter((r) => r.month === ctx.month && (!ctx.tenantName || r.use_unit === ctx.tenantName) && r.meter_kind !== 'public')
      return aggregateRows(rows, src.field, ctx.tenantName)
    }
    case '水费核算': {
      const meters = ctx.waterMeters.filter((m: any) =>
        m.enabled !== false && (!ctx.tenantName || m.owner_unit === ctx.tenantName || m.use_unit === ctx.tenantName))
      const rows: any[] = []
      for (const m of meters) {
        const r = ctx.waterReadingMap.get(`${m.id}__${ctx.month}`)
        if (r) rows.push({ ...r, unit_price: r.unit_price ?? r.price ?? m.unit_price })
      }
      return aggregateRows(rows, src.field, ctx.tenantName)
    }
    default:
      return 0
  }
}

/**
 * 分摊子项在某个租户某个月的值。
 *
 * 口径（产品已确认）：
 *   · 市电账单费用分项 + value_mode=apportion → 费用 × 该月分摊比例（沿用生成账单口径）；
 *   · 其它分类字段 + value_mode=direct_sum → 直接取「当月 + 使用单位=该租户」的合计，
 *     不再乘比例（水/气/电核算天然按户归属，再乘比例会重复折算）。
 */
// ---- 分时电费：逐表逐时段基准电量 × 时段单价 ----
//
// 与后端 internal/handler/billing_timed.go **同一口径**（改一处必须改另一处）：
//   计费电量 c = 抄见 + 变损 + 线损 + 加减（账单电量明细，整体落在第一块表）
//   差额分摊 d = ΔQ × c / C总
//   基准电量 b = c + d，分时电费 = Σ b × 时段单价
//
// 为什么不能用「账单整月金额 × 分摊比例」：2026-09 持睿汽车按比例算得 54,369.99，
// 标准核算表是 54,422.79（差 52.8 元）。比例法把「尖峰高价电」和「谷段低价电」
// 按同一个比例摊，本质上丢掉了分时结构。
export const TIMED_PERIODS = ['deep', 'peak', 'flat', 'valley'] as const
export const TIMED_PERIOD_LABELS: Record<string, string> = { deep: '尖峰', peak: '峰', flat: '平', valley: '谷' }

export interface TimedMeterRow {
  period: string
  usage: number
  loss: number
  adjust: number
  billKwh: number
  diffKwh: number
  baseKwh: number
}
export interface TimedAllocation {
  totalUsage: number
  totalBill: number
  deltaQ: number
  /** 各时段基准电量合计（两位小数，用于展示）。 */
  periodBase: Record<string, number>
  /** 各时段基准电量合计（未取整）：分时电费必须用它乘单价，否则四段累加差 1 分。 */
  periodBaseExact: Record<string, number>
  rows: TimedMeterRow[]
}

/** 从市电账单「电量明细」取四段变损+线损 / 加减电量（按 period 键）。 */
export function billPeriodLossAdjust(bill: any): { loss: Record<string, number>; adjust: Record<string, number> } {
  const loss: Record<string, number> = {}
  const adjust: Record<string, number> = {}
  const readings: any[] = bill?.item?.meter_readings || []
  for (const mr of readings) {
    const t = String(mr?.meter_type || '')
    const p = t.includes('尖') ? 'deep' : t.includes('峰') ? 'peak' : t.includes('平') ? 'flat' : t.includes('谷') ? 'valley' : ''
    if (!p) continue
    loss[p] = round2((Number(mr?.trans_loss) || 0) + (Number(mr?.line_loss) || 0))
    adjust[p] = round2(Number(mr?.adjust) || 0)
  }
  return { loss, adjust }
}

/** 分时电量分摊（纯函数，与 Go 侧 allocateTimedKwh 一一对应）。 */
export function allocateTimedKwh(
  meters: Array<{ name: string; usage: Record<string, number> }>,
  loss: Record<string, number>,
  adjust: Record<string, number>,
  deltaQ: number,
): TimedAllocation {
  const out: TimedAllocation = {
    totalUsage: 0, totalBill: 0, deltaQ: round2(Number(deltaQ) || 0),
    periodBase: {}, periodBaseExact: {}, rows: [],
  }
  const rows: TimedMeterRow[] = []
  meters.forEach((m, i) => {
    for (const p of TIMED_PERIODS) {
      const u = round2(Number(m.usage?.[p]) || 0)
      out.totalUsage = round2(out.totalUsage + u)
      // 损耗/加减整体落在第一块表：账单电量明细是按表 1 列示的
      const l = i === 0 ? round2(Number(loss?.[p]) || 0) : 0
      const a = i === 0 ? round2(Number(adjust?.[p]) || 0) : 0
      const c = round2(u + l + a)
      out.totalBill = round2(out.totalBill + c)
      rows.push({ period: p, usage: u, loss: l, adjust: a, billKwh: c, diffKwh: 0, baseKwh: 0 })
    }
  })
  for (const r of rows) {
    const d = out.totalBill > 0 ? (out.deltaQ * r.billKwh) / out.totalBill : 0
    r.diffKwh = round2(d)
    r.baseKwh = round2(r.billKwh + d)
    out.periodBase[r.period] = round2((out.periodBase[r.period] || 0) + r.baseKwh)
    out.periodBaseExact[r.period] = (out.periodBaseExact[r.period] || 0) + r.billKwh + d
  }
  out.rows = rows
  return out
}

/** 账单里该子项的分时段单价（尖峰/峰/平/谷）。 */
export function periodRatesOf(ctx: SourceResolveContext, name: string, group?: string | null): Record<string, number> {
  const rates: Record<string, number> = {}
  for (const r of matchBillFeeItems(ctx, name, group)) {
    const label = String(r?.period || '')
    const p = label.includes('尖') ? 'deep' : label.includes('峰') ? 'peak' : label.includes('平') ? 'flat' : label.includes('谷') ? 'valley' : ''
    if (!p) continue
    const rate = Number(r?.rate) > 0 ? Number(r.rate) : (Number(r?.fee) || 0) / (Number(r?.qty) || 1)
    if (rate) rates[p] = rate
  }
  return rates
}

/** 分时子项是否该走「基准电量 × 时段单价」口径（账单里确有分时段行）。 */
export function isTimedFeeItem(ctx: SourceResolveContext, name: string): boolean {
  const rates = periodRatesOf(ctx, name)
  return !!(rates.deep || rates.peak || rates.valley)
}

/**
 * 分时子项金额 + 明细（列表取值与浮窗同源）。
 * 返回 null 表示该子项不是分时项，调用方走原有「账单金额 × 比例」口径。
 */
export function resolveTimedItemFee(
  ctx: SourceResolveContext,
  name: string,
  group?: string | null,
): { value: number; allocation: TimedAllocation; rates: Record<string, number>; periods: Array<{ period: string; qty: number; rate: number; fee: number }> } | null {
  const rates = periodRatesOf(ctx, name, group)
  const isTimed = !!(rates.deep || rates.peak || rates.valley)
  if (!isTimed) return null
  const meters = ctx.timedMeters || []
  if (!meters.length) return null
  const { loss, adjust } = billPeriodLossAdjust(ctx.bill)
  const alloc = allocateTimedKwh(meters, loss, adjust, Number(ctx.deltaQ) || 0)
  const periods = TIMED_PERIODS
    .filter((p) => rates[p])
    .map((p) => ({
      period: TIMED_PERIOD_LABELS[p],
      qty: alloc.periodBase[p] || 0,
      rate: rates[p],
      fee: round2((alloc.periodBaseExact[p] || 0) * rates[p]),
    }))
  const value = round2(periods.reduce((s, p) => s + p.fee, 0))
  return { value, allocation: alloc, rates, periods }
}

export function resolveTenantItemValue(item: any, ctx: SourceResolveContext, ratio: number): number {
  const category = String(item?.source_category || item?.sourceCategory || '市电账单').trim()
  const key = String(item?.source_key || item?.sourceKey || item?.item_name || item?.itemName || '').trim()
  const mode = String(item?.source_mode || item?.sourceMode || 'fee_row').trim()
  const valueMode = String(item?.value_mode || item?.valueMode || 'apportion').trim()
  if (!key) return 0
  if (category === '市电账单' && mode === 'fee_row') {
    // 子项自带的大类（如 `(6)政府性基金及附加-工商业`）就是匹配范围：
    // 少了它，工商业与居民的同名基金会被一起累加（农网还贷 11744.6+178.76）。
    const fee = resolveFeeRow(ctx, key, item?.category || item?.source_group || '')
    return valueMode === 'direct_sum' ? fee : round2(fee * (Number(ratio) || 0))
  }
  const v = resolveSourceValue({ category, field: key, mode: mode === 'fee_row' ? 'fee_row' : 'field' }, ctx)
  return typeof v === 'number' ? v : 0
}

/** 分摊子项的来源展示：`[来源分类] 名称`。 */
export function tenantItemBadge(item: any): string {
  const category = String(item?.source_category || item?.sourceCategory || '').trim()
  const name = String(item?.item_name || item?.itemName || item?.source_key || '').trim()
  return category ? `[${category}] ${name}` : name
}

/** 引用字段的计算口径（用于字段定义面板的提示文案）。 */
export function sourceRuleText(src: FieldSource): string {
  switch (src.category) {
    case '市电账单':
      return src.mode === 'fee_row' ? '当月账单费用分项求和' : '当月账单概况字段'
    case '光伏账单':
      return '当月光伏账单求和（全厂公共）'
    case '电费核算':
    case '水费核算':
    case '气费核算':
      return '当月 + 使用单位=当前单位 求和'
    default:
      return ''
  }
}
