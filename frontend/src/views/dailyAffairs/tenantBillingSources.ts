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
  mode: 'field' | 'fee_row'
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
}

export const SOURCE_CATEGORIES = ['市电账单', '光伏账单', '电费核算', '水费核算', '气费核算'] as const

/** 从 subs 条目里读出来源描述；老数据没有 source 返回 null（= 内置派生列）。 */
export function parseFieldSource(sub: any): FieldSource | null {
  const raw = sub?.source
  if (!raw || typeof raw !== 'object') return null
  const category = String(raw.category || '').trim()
  const field = String(raw.field || '').trim()
  if (!category || !field) return null
  const src: FieldSource = { category, field, mode: raw.mode === 'fee_row' ? 'fee_row' : 'field' }
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
const aliasNorm = (v: any) => norm(v).replace(/[（）()]/g, '').replace(/因素/g, '因数')

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
}

/** 单价类字段：不能直接求和，按 金额÷用量 加权。 */
const PRICE_FIELDS = new Set(['unit_price', 'price', 'avg_price', 'ind_price', 'default_unit_price'])
/** 取首值的字段（倍率/表号等非可加量）。 */
const FIRST_VALUE_FIELDS = new Set(['rate', 'meter_no', 'meter_type', 'meter_kind', 'install_date', 'reading_date', 'reader'])

function aggregateRows(rows: any[], field: string): number {
  if (!rows.length) return 0
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
export function matchBillFeeItems(ctx: SourceResolveContext, name: string): any[] {
  const items: any[] = ctx.bill?.item?.fee_items || []
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
    // 单价不可相加：按「金额 ÷ 数量」加权，避免直接取首行的分时单价
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

/** 当月市电账单的费用分项求和（mode=fee_row）。 */
function resolveFeeRow(ctx: SourceResolveContext, name: string): number {
  // ① 固定金额分项（基本电费 / 力调电费）**优先**读概况字段。
  //    不能等 fee_items 匹配失败再兜底：账单里可能存在同名的空行
  //    （2026-08 的「功率因数调整」fee=0），会把权威值挡掉。
  const fixed = fixedBillFee(ctx, name)
  if (fixed !== null) return round2(fixed)
  // ② 其余分项按 fee_items 匹配求和
  const hit = matchBillFeeItems(ctx, name)
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
      if (src.group) {
        const grouped = resolveByBillGroup(ctx, src.group, src.field)
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
      return aggregateRows(rows, src.field)
    }
    case '气费核算': {
      const rows = ctx.gasRows.filter((r) => r.month === ctx.month && (!ctx.tenantName || r.use_unit === ctx.tenantName) && r.meter_kind !== 'public')
      return aggregateRows(rows, src.field)
    }
    case '水费核算': {
      const meters = ctx.waterMeters.filter((m: any) =>
        m.enabled !== false && (!ctx.tenantName || m.owner_unit === ctx.tenantName || m.use_unit === ctx.tenantName))
      const rows: any[] = []
      for (const m of meters) {
        const r = ctx.waterReadingMap.get(`${m.id}__${ctx.month}`)
        if (r) rows.push({ ...r, unit_price: r.unit_price ?? r.price ?? m.unit_price })
      }
      return aggregateRows(rows, src.field)
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
export function resolveTenantItemValue(item: any, ctx: SourceResolveContext, ratio: number): number {
  const category = String(item?.source_category || item?.sourceCategory || '市电账单').trim()
  const key = String(item?.source_key || item?.sourceKey || item?.item_name || item?.itemName || '').trim()
  const mode = String(item?.source_mode || item?.sourceMode || 'fee_row').trim()
  const valueMode = String(item?.value_mode || item?.valueMode || 'apportion').trim()
  if (!key) return 0
  if (category === '市电账单' && mode === 'fee_row') {
    const fee = resolveFeeRow(ctx, key)
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
