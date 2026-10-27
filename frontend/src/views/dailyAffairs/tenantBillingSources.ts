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
}

export const SOURCE_CATEGORIES = ['市电账单', '光伏账单', '电费核算', '水费核算', '气费核算'] as const

/** 从 subs 条目里读出来源描述；老数据没有 source 返回 null（= 内置派生列）。 */
export function parseFieldSource(sub: any): FieldSource | null {
  const raw = sub?.source
  if (!raw || typeof raw !== 'object') return null
  const category = String(raw.category || '').trim()
  const field = String(raw.field || '').trim()
  if (!category || !field) return null
  return { category, field, mode: raw.mode === 'fee_row' ? 'fee_row' : 'field' }
}

/** 引用字段的展示名：`[来源分类] 字段名`（防同名误选）。 */
export function sourceBadge(src: FieldSource | null, fallback = ''): string {
  if (!src) return ''
  return `[${src.category}] ${fallback || src.field}`
}

const norm = (v: any) => String(v ?? '').trim().toLowerCase().replace(/\s+/g, '')
const num = (v: any) => {
  const n = Number(v)
  return Number.isFinite(n) ? n : 0
}
const round2 = (v: number) => Math.round(v * 100) / 100

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

/** 当月市电账单的费用分项求和（mode=fee_row）。 */
function resolveFeeRow(ctx: SourceResolveContext, name: string): number {
  const items: any[] = ctx.bill?.item?.fee_items || []
  const want = norm(name)
  if (!want) return 0
  let sum = 0
  let hit = false
  for (const it of items) {
    const n = norm(it?.name)
    // 兼容「大类|子项名」写法：取竖线后半段再比对
    const tail = norm(String(it?.name || '').split('|').pop())
    if (n === want || tail === want) {
      sum += num(it?.fee)
      hit = true
    }
  }
  if (hit) return round2(sum)
  // 名称对不上时退化为「前缀包含」匹配（费用分项常带 (1)/(2) 前缀）
  for (const it of items) {
    const n = norm(it?.name)
    const tail = norm(String(it?.name || '').split('|').pop())
    if ((n && (n.includes(want) || want.includes(n))) || (tail && (tail.includes(want) || want.includes(tail)))) {
      sum += num(it?.fee)
    }
  }
  return round2(sum)
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
