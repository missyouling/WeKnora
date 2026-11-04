// 市电账单「详情视图」的纯逻辑（第 24 轮任务一：UtilitiesManagement 瘦身）。
//
// 详情视图是分层菜单：概览 → 工商业电费（6 个子表）/ 居民电费（3 个子表）
// → 输配容需量 / 功率因素调整。每个子表「显示哪些行」由字段配置（subs 的
// `*-rows` 分组）决定，行内金额再按「名称 + 时段」精确匹配。
//
// 这块逻辑从 3800 行的 .vue 里抽出来，是因为它全是「数据进 → 行出」的纯函数：
// 菜单结构、行匹配、行排序、合计——不读任何响应式状态。
// 抽离后单测可以直接喂 editForm 快照，不必挂载页面。
//
// **口径红线**（都是真机事故换来的，改动必须同步改单测）：
//   · 同名多行优先取「有金额的那行」：合并单元格解析出的全 0 影子行若排在前，
//     `find` 会拿到 0，账单概况就少算这笔钱（真机少 31520.93 的偏差回收返还）。
//   · 分时子项按 `名称(时段)` 一一对应，`尖峰/峰/平/谷` 之外不拆。
//   · 容需量与功率因素各自独立成表，不进费用子表。
//   · 合计一律按「分」整数累加再除（`Math.round(sum*100)/100`），避免 0.1+0.2 的浮点尾巴。

/** 详情左侧菜单项。`group: true` 表示可折叠分组，children 是子表。 */
export interface BillMenu {
  key: string
  label: string
  group?: boolean
  children?: { key: string; label: string }[]
}

/** 分层菜单结构（顺序即左侧导航顺序，不要随意调换）。 */
export const BILL_MENUS: BillMenu[] = [
  { key: 'overview', label: '账单概况' },
  {
    key: 'industrial',
    label: '工商业电费',
    group: true,
    children: [
      { key: 'industrial-meter', label: '电量明细' },
      { key: 'industrial-market', label: '市场化购电费' },
      { key: 'industrial-line', label: '上网环节线损费' },
      { key: 'industrial-trans', label: '输配电量电费' },
      { key: 'industrial-sys', label: '系统运行费' },
      { key: 'industrial-gov', label: '政府基金及附加' },
    ],
  },
  {
    key: 'residential',
    label: '居民电费',
    group: true,
    children: [
      { key: 'residential-meter', label: '电量明细' },
      { key: 'residential-catalog', label: '目录电费' },
      { key: 'residential-gov', label: '政府性基金及附加' },
    ],
  },
  { key: 'capacity', label: '输配容（需）量电费' },
  { key: 'pf-adjust', label: '功率因素调整电费' },
]

/**
 * 费用菜单 → 行归属判定。
 *
 * `industrial-gov` / `residential-gov` 两者都命中「政府性基金」，靠**电量数量级**区分：
 * 工商业表里那一项的电量是十万级（总表读数），居民表里是万级以下。
 * 这个判据看着糙，但它是唯一能区分同名条目的稳定特征 —— 两表都叫「政府性基金及附加」。
 */
export const FEE_MENU_MAP: Record<string, (it: any) => boolean> = {
  'industrial-market': (it) => (it.category || '').includes('市场化购电'),
  'industrial-line': (it) => (it.category || '').includes('上网环节线损'),
  'industrial-trans': (it) => (it.category || '').includes('输配电量'),
  'industrial-sys': (it) => (it.category || '').includes('系统运行'),
  'industrial-gov': (it) =>
    (it.category || '').includes('政府性基金') &&
    Number(it.qty || 0) >= 100000 &&
    !(it.name || '').includes('功率因数'),
  'residential-catalog': (it) => (it.category || '').includes('目录电费'),
  'residential-gov': (it) =>
    (it.category || '').includes('政府性基金') &&
    Number(it.qty || 0) < 100000 &&
    !(it.name || '').includes('功率因数'),
  'pf-adjust': (it) => (it.category || '').includes('功率因数') || (it.name || '').includes('功率因数'),
}

/** 是否为「费用子表」菜单（功率因素调整是独立明细列表，不算）。 */
export const feeMenuOf = (key: string) => key in FEE_MENU_MAP && key !== 'pf-adjust'

/** 费用菜单 → 行配置分组（行字段配置驱动各费用表格的行）。 */
export const ROW_GROUP_OF_MENU: Record<string, string> = {
  'industrial-market': 'market-rows',
  'industrial-trans': 'trans-rows',
  'industrial-sys': 'sys-rows',
  'industrial-gov': 'gov-industrial-rows',
  'residential-catalog': 'catalog-rows',
  'residential-gov': 'gov-residential-rows',
}

/**
 * 分时时段。**仅括号内为这些时段时**，配置 key 才拆分为「名称 + 时段」
 * （如 `零售交易电费(尖峰)`）。其它括号内容（如「(含税)」）不拆。
 */
export const ROW_PERIODS = ['尖峰', '峰', '平', '谷']

/** 全角括号归一：配置与提取结果的括号写法经常不一致。 */
export const normRowName = (s: string) => (s || '').trim().replace(/[（(]/g, '(').replace(/[）)]/g, ')')

/** 把 `名称(时段)` 拆成两段；时段不在白名单时整体当名称。 */
export function parseRowKey(k: string): { name: string; period: string } {
  const m = /^(.+?)\(([^)]+)\)$/.exec(k || '')
  if (m && ROW_PERIODS.includes(m[2])) return { name: m[1], period: m[2] }
  return { name: k || '', period: '' }
}

/** 行配置条目（subs 里 `*-rows` 分组的形态）。 */
export interface RowCfg {
  field_key: string
  label: string
  /** 所属分组（subs.group），rowConfigsOf 按它过滤。 */
  group?: string
  default_visible?: boolean
  sort_order?: number
  deleted_at?: any
}

/** 取某分组的行配置（按 sort_order 升序，已删除的剔除）。 */
export function rowConfigsOf(all: RowCfg[], group: string): RowCfg[] {
  const list = all.filter((c) => c.group === group && c.deleted_at == null)
  return [...list].sort((a, b) => (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0))
}

/**
 * 费用子表的行：按行配置过滤 + 排序 + 改名。
 *
 * 无行配置时**原样返回提取结果**（不猜、不排序），这样新字段上线前不会因为
 * 缺配置而整表空掉。有配置时：未配置的行隐藏，配置里点名的行按配置顺序出场。
 *
 * 同名多行时优先取「有金额或电量」的那行 —— 合并单元格解析出的全 0 影子行
 * 若排在前，直接 `find` 会拿到 0，账单概况就少算这笔钱。
 */
export function feeRowsOf(
  feeItems: any[],
  key: string,
  rowCfgs: RowCfg[],
): any[] {
  const filter = FEE_MENU_MAP[key]
  if (!filter) return []
  const list = (feeItems || []).filter(filter)
  const rg = ROW_GROUP_OF_MENU[key]
  const cfgs = (rg ? rowCfgs : []).filter((c) => c.default_visible !== false)
  if (!cfgs.length) return list
  return cfgs
    .map((cfg) => {
      const want = parseRowKey(cfg.field_key)
      const hits = list.filter((it: any) => {
        const n = normRowName(it.name)
        const p = (it.period || '').trim()
        if (want.period) return n === normRowName(want.name) && p === want.period
        return n === normRowName(want.name)
      })
      const hit =
        hits.find((it: any) => (Number(it.fee) || 0) !== 0 || (Number(it.qty) || 0) !== 0) || hits[0]
      if (!hit) return null
      return { ...hit, name: cfg.label || cfg.field_key }
    })
    .filter(Boolean) as any[]
}

/** 金额按「分」累加后还原（避免浮点尾巴上屏）。 */
export const roundMoney = (n: number) => Math.round(n * 100) / 100

/** 某费用子表的小计（空表给 0，不是 undefined）。 */
export function feeSubtotalOf(feeItems: any[], key: string, rowCfgs: RowCfg[]): number {
  return roundMoney(
    feeRowsOf(feeItems, key, rowCfgs).reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0),
  )
}

/** 菜单 key → 中文标签（找不到时原样返回 key，便于排障时一眼看出是哪个 key 漏配）。 */
export function menuLabelOf(key: string): string {
  for (const m of BILL_MENUS) {
    if (m.children) {
      const c = m.children.find((x) => x.key === key)
      if (c) return c.label
    }
  }
  return key
}

/** 容需量字段（编辑抽屉）。 */
export const CAPACITY_FIELDS = [
  { key: 'demand', label: '需量值（kW）', type: 'number' },
  { key: 'demand_price', label: '需量电价（元/kW）', type: 'number' },
  { key: 'demand_fee', label: '输配需量电费（元）', type: 'number' },
  { key: 'kwh_per_kva', label: '月每千伏安用电量（kWh/kVA）', type: 'number' },
  { key: 'discount_demand_fee', label: '折扣需量电费（元）', type: 'number' },
  { key: 'capacity', label: '容量（kVA）', type: 'number' },
  { key: 'capacity_price', label: '容量电价（元/kVA）', type: 'number' },
  { key: 'capacity_fee', label: '输配容量电费（元）', type: 'number' },
]

/** 功率因数字段（编辑抽屉）。 */
export const PF_FIELDS = [
  { key: 'project', label: '项目', type: 'text' },
  { key: 'power_factor', label: '功率因素实际值', type: 'number' },
  { key: 'pf_standard', label: '功率因素标准', type: 'number' },
  { key: 'adjust_ratio', label: '调整系数', type: 'number' },
  { key: 'pf_active_kwh', label: '参与调整有功电量（kWh）', type: 'number' },
  { key: 'pf_reactive_kwh', label: '参与调整无功电量（kvarh）', type: 'number' },
  { key: 'pf_fee_base', label: '参与调整电费金额（元）', type: 'number' },
  { key: 'adjust_fee', label: '功率因素调整电费（元）', type: 'number' },
]

/**
 * 电量明细的固定顺序。
 *
 * 无行配置时按这个顺序排；配置里出现的其它 meter_type 排到末尾（99）。
 * 正向无功（总）在最后 —— 它不参与计费，但要在表里露出来供对数。
 */
export const META_ORDER = [
  '正向有功（总）',
  '正向有功（尖峰）',
  '正向有功（峰）',
  '正向有功（平）',
  '正向有功（谷）',
  '正向无功（总）',
]

/**
 * 电量明细行匹配：`meter_type` / `project` 与配置的 field_key/label 双向包含。
 *
 * 用「双向包含」而不是精确等值，是因为提取结果里常带后缀（如「正向有功（总）」
 * 与「正向有功(总)」全半角混用），精确等值会整表匹配不上。
 */
export function matchRow(row: any, cfg: { field_key: string; label?: string }): boolean {
  const t = row?.meter_type || row?.project || ''
  if (!t) return false
  return (
    t === cfg.field_key ||
    t === cfg.label ||
    t.includes(cfg.field_key) ||
    cfg.field_key.includes(t)
  )
}

/**
 * 电量明细行（9 列）：示数类型 / 上期 / 本期 / 倍率 / 抄见 / 变损 / 线损 / 加减 / 计费电量。
 *
 * 有配置 → 按配置逐行输出（匹配不到的行给**空串占位**而不是 0：
 * 「这一行没有数据」与「这一行是 0 度」在页面上必须能分辨）；
 * 无配置 → 按 META_ORDER 排序后原样输出，或从顶层字段兜底（旧数据只有计费电量）。
 */
/** 分时档位（票面顺序）。 */
const TOU_KEYS = ['尖峰', '峰', '平', '谷'] as const

/** 示数类型 → 档位（非分时行返回空串）。 */
export function meterPeriodOfType(meterType: string): string {
  const m = /正向(?:有功|无功)[（(](总|尖峰|峰|平|谷)[)）]/.exec(String(meterType || ''))
  return m ? m[1] : ''
}

/**
 * 电量明细的合计：**只累加四段计费电量**。
 *
 * 票面上「正向有功（总）」的计费电量通常是 0（原单不给），而无功总行即使有值
 * 也不属于电费口径。旧实现把 6 行全加起来，2026-10 得到 1,401,690
 * （总 623,610 + 四段 614,578 + 无功 154,410 + …），与账单 614,578 差一倍多。
 */
export function meterTotalOf(rows: any[]): number {
  let sum = 0
  for (const r of Array.isArray(rows) ? rows : []) {
    if (!TOU_KEYS.includes(meterPeriodOfType(r?.meter_type) as any)) continue
    sum += Number(r?.bill_kwh) || 0
  }
  return Math.round(sum * 100) / 100
}

/**
 * 工商业四段「计费电量」：优先取电量明细，缺失时回落到顶层字段。
 *
 * 用能分析的占比与尖/峰/平/谷电量都必须绑这个口径。
 * 旧实现直接读顶层 deep_peak_kwh 等字段，而提取把它们抄成了
 * 「抄见电量」甚至「尖峰复制到峰段」，占比因此全错。
 */
export function touBillKwhOf(editForm: Record<string, any>): { deep: number; peak: number; flat: number; valley: number } {
  const form = editForm || {}
  const fromMeter: Record<string, number> = {}
  for (const r of Array.isArray(form.meter_readings) ? form.meter_readings : []) {
    const p = meterPeriodOfType(r?.meter_type)
    if (p) fromMeter[p] = Number(r?.bill_kwh) || 0
  }
  const pick = (period: string, fallback: any) => {
    const v = fromMeter[period]
    if (v !== undefined && v > 0) return v
    return Number(fallback) || 0
  }
  return {
    deep: pick('尖峰', form.deep_peak_kwh),
    peak: pick('峰', form.peak_kwh),
    flat: pick('平', form.flat_kwh),
    valley: pick('谷', form.valley_kwh),
  }
}

export function buildMeterRows(editForm: Record<string, any>, cfgs: RowCfg[]): any[] {
  let list: any[]
  const mr = editForm?.meter_readings
  if (Array.isArray(mr) && mr.length) {
    list = mr
  } else {
    // 旧数据兜底：仅计费电量有值
    const e = editForm || {}
    const row = (meter_type: string, bill_kwh: number) => ({
      meter_type,
      prev: 0,
      curr: 0,
      multiplier: 1,
      reading_kwh: 0,
      trans_loss: 0,
      line_loss: 0,
      adjust: 0,
      bill_kwh: Number(bill_kwh) || 0,
    })
    list = [
      row('正向有功（总）', e.total_kwh),
      row('正向有功（尖峰）', e.deep_peak_kwh),
      row('正向有功（峰）', e.peak_kwh),
      row('正向有功（平）', e.flat_kwh),
      row('正向有功（谷）', e.valley_kwh),
      row('正向无功（总）', e.reactive_kwh),
    ]
  }
  if (!cfgs.length) {
    return [...list].sort((a, b) => {
      const ia = META_ORDER.indexOf(a.meter_type)
      const ib = META_ORDER.indexOf(b.meter_type)
      return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib)
    })
  }
  return cfgs.map((cfg) => {
    const hit = list.find((m) => matchRow(m, cfg))
    if (hit) return { ...hit, meter_type: cfg.label }
    return {
      meter_type: cfg.label,
      prev: '',
      curr: '',
      multiplier: '',
      reading_kwh: '',
      trans_loss: '',
      line_loss: '',
      adjust: '',
      bill_kwh: '',
    }
  })
}

/**
 * 居民电量明细（与工商业 9 列一致）。
 *
 * 行 = 定比 0.015，计费电量 = 本期电量 × 定比 + 加减。
 * `bill_kwh_manual` 为真时用人工值 —— 定比是电网给的经验值，
 * 实际有差异时财务会手改，自动值不能覆盖人工值。
 */
export function buildResidentMeterRows(editForm: Record<string, any>, cfgs: RowCfg[]): any[] {
  const totalKwh = Number(editForm?.total_kwh) || 0
  const list = editForm?.residential_readings
  const arr = Array.isArray(list) && list.length ? list : []
  const build = (cfgLabel: string, hit: any) => {
    const ratio = Number(hit?.ratio) || 0.015
    const adjust = Number(hit?.adjust) || 0
    const autoBill = totalKwh > 0 ? Math.round((totalKwh * ratio + adjust) * 100) / 100 : 0
    const bill = hit?.bill_kwh_manual ? Number(hit.bill_kwh) || 0 : autoBill
    return {
      meter_type: cfgLabel,
      prev: '',
      curr: '',
      multiplier: '',
      reading_kwh: '',
      trans_loss: '',
      line_loss: '',
      adjust,
      bill_kwh: bill,
    }
  }
  // 无配置：只输出一行「定比0.015」（居民只有一个定比行，不是每户一行）
  if (!cfgs.length) {
    return [build('定比0.015', arr[0] || { ratio: 0.015, adjust: 0, bill_kwh: 0 })]
  }
  // 有配置：**按索引配对**（第 i 个配置对应第 i 条读数），配对不上时回落第 0 条。
  // 不用 matchRow 是因为居民行的读数里没有 meter_type，只有 ratio/adjust，
  // 配置的 label 与读数没有可匹配的公共键。
  return cfgs.map((cfg, ci) => {
    const hit = arr[ci] || arr[0] || null
    return build(cfg.label || cfg.field_key, hit)
  })
}

/**
 * 上期分时数据：按账单周期取「最近上一期」。
 *
 * `allRecords` 是列表里全部记录（每项形如 `{ item: {...} }`）。
 * 判据用字符串比较 `bill_period_start`：它是 `YYYY-MM-DD` 定长格式，
 * 字典序等价于时间序，比 Date 解析稳（不会踩时区坑）。
 */
export function pickPrevEnergy(allRecords: any[], curStart: string) {
  const prevs = (allRecords || [])
    .map((r) => r?.item || {})
    .filter((m: any) => m.bill_period_start && String(m.bill_period_start) < (curStart || ''))
    .sort((a: any, b: any) =>
      String(b.bill_period_start).localeCompare(String(a.bill_period_start)),
    )
  const p = prevs[0] || {}
  const exists = !!p.bill_period_start
  const numOr0 = (v: any) => (v === null || v === undefined || v === '' ? 0 : Number(v) || 0)
  return {
    exists,
    period: exists ? String(p.bill_period_start).slice(0, 7) : '',
    deep: Number(p.deep_peak_kwh) || 0,
    peak: Number(p.peak_kwh) || 0,
    flat: Number(p.flat_kwh) || 0,
    valley: Number(p.valley_kwh) || 0,
    total: Number(p.total_kwh) || 0,
    fee: Number(p.total_amount) || Number(p.grand_total) || 0,
    power_factor: numOr0(p.power_factor),
    avg_price: numOr0(p.avg_price),
  }
}

/** 分时占比对比行（本期 vs 上期）。 */
export interface EnergyCompareRow {
  key: string
  label: string
  cur: number
  curPct: number
  prev: number
  prevPct: number
  delta: number
}

/**
 * 分时占比对比表。
 *
 * 占比公式：本期占比 = 时段电量 ÷ 本期总电量 × 100%，保留两位。
 * **平段取余量**（100 − 尖 − 峰 − 谷），保证四舍五入后四段合计恰好 100% ——
 * 直接各算各的会出现「尖 12.35 + 峰 30.11 + 平 44.32 + 谷 13.23 = 100.01」这种
 * 财务一眼就要问的表。余量允许为 0，但不允许为负（异常数据时 clamp 到 0）。
 */
export function buildEnergyCompareRows(
  editForm: Record<string, any>,
  prev: ReturnType<typeof pickPrevEnergy>,
): EnergyCompareRow[] {
  const tou = touBillKwhOf(editForm)
  const cur = [
    { key: 'deep', label: '尖', v: tou.deep },
    { key: 'peak', label: '峰', v: tou.peak },
    { key: 'flat', label: '平', v: tou.flat },
    { key: 'valley', label: '谷', v: tou.valley },
  ]
  // 占比分母 = 工商业四段计费电量之和，不是账单总电量（总电量含居民 9,354，
  // 会把工商业四段压到合计 98.5%，用户看到「占比加起来不到 100%」）。
  const curSum = cur.reduce((s, c) => s + c.v, 0)
  const curTotal = curSum > 0 ? curSum : (Number(editForm?.total_kwh) || 0)
  const prevSum = [prev.deep, prev.peak, prev.flat, prev.valley].reduce((s, v) => s + (Number(v) || 0), 0)
  const prevTotal = prevSum > 0 ? prevSum : prev.total
  const pct = (v: number, total: number) => (total ? Math.round((v / total) * 10000) / 100 : 0)
  const prevQty: Record<string, number> = {
    deep: prev.deep,
    peak: prev.peak,
    flat: prev.flat,
    valley: prev.valley,
  }
  const curPcts = cur.map((c) => ({ key: c.key, pct: pct(c.v, curTotal) }))
  const prevPcts = cur.map((c) => ({ key: c.key, pct: prevTotal ? pct(prevQty[c.key] || 0, prevTotal) : 0 }))
  const sumExcl = (arr: { key: string; pct: number }[], excl: string) =>
    arr.filter((a) => a.key !== excl).reduce((s, a) => s + a.pct, 0)
  const curFlat = curTotal ? Math.max(0, roundMoney(100 - sumExcl(curPcts, 'flat'))) : 0
  const prevFlat = prevTotal ? Math.max(0, roundMoney(100 - sumExcl(prevPcts, 'flat'))) : 0
  const curPctOf = (key: string) =>
    key === 'flat' ? curFlat : curPcts.find((p) => p.key === key)?.pct || 0
  const prevPctOf = (key: string) =>
    key === 'flat' ? prevFlat : prevPcts.find((p) => p.key === key)?.pct || 0
  return cur.map((c) => {
    const pv = prevQty[c.key] || 0
    const curPct = curPctOf(c.key)
    const prevPct = prevPctOf(c.key)
    return {
      key: c.key,
      label: c.label,
      cur: c.v,
      curPct,
      prev: pv,
      prevPct,
      delta: prevTotal ? Math.round((curPct - prevPct) * 10) / 10 : 0,
    }
  })
}
