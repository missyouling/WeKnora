/**
 * 费用清单「数据来源 / 计算方式」悬浮提示的文案层。
 *
 * 为什么单独成文件：提示里必须写清**实际参与计算的数值**（不是千篇一律的公式），
 * 而这些数值只有在 `buildRows` 里才算得出来。所以由 `buildRows` 在算完一行后
 * 调 `buildRowTips(row, facts)` 一次性产出 `{ 列key: 说明文本 }`，
 * 单元格渲染只负责把它塞进 tooltip。
 *
 * 分三类口径（与列表取值的三类来源一一对应）：
 *   ① 内置派生列：写完整的加减乘除算式 + 代入数值；
 *   ② 租户分摊列：写「账单金额 × 分摊比例」或「分时电量 × 单价，逐段求和」；
 *   ③ 跨分类引用列：写来源路径 `[分类 · 分组] 字段` 与取数范围。
 */

const n2 = (v: any): string => {
  const n = Number(v)
  if (!Number.isFinite(n)) return '0'
  // 千分位 + 最多两位小数：账单金额动辄六位数，不分组很难核对
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const pct = (v: any): string => {
  const n = Number(v)
  return Number.isFinite(n) && n !== 0 ? `${(n * 100).toFixed(2)}%` : '—'
}

/** 一行在计算过程中真实用到的中间值。 */
export interface RowFacts {
  /** 当前视角（决定同一列名的公式口径） */
  ownerView: boolean
  useUnit: string
  tenantName: string
  month: string
  /** 市电账单概况 */
  totalKwh: number
  totalFee: number
  /** 定比（居民）电量/电费 */
  resKwh: number
  resFee: number
  /** 宿舍电量/电费 */
  dormKwh: number
  dormFee: number
  /** 已生成账单：租户分表总电量 / 工业电费 / 分摊比例 */
  recKwh: number
  recFee: number
  ratio: number
  /** 工业口径中间值 */
  indKwh: number
  indFee: number
  indPrice: number
  /** 均价分母用到的各分量（业主视角才有意义） */
  tenantDormKwh: number
  tenantDormFee: number
  /** 水量：总表 / 工业 / 宿舍 / 消防 */
  waterTotalUsage: number
  waterTotalPrice: number
  waterIndUsage: number
  waterIndFee: number
  waterDormUsage: number
  waterDormFee: number
  waterFireUsage: number
  waterFireFee: number
  /** 业主视角：参与减法的两个租户水量分量（写成算式要用真实值） */
  waterTenantDormUsage: number
  waterTenantIndUsage: number
  /** 气 */
  gasUsage: number
  gasFee: number
  /** 气表条数（用于「各表求和」文案） */
  gasCount: number
}

/** 内置派生列的说明（只对存在的列产出）。 */
function builtinTips(f: RowFacts): Record<string, string> {
  const t: Record<string, string> = {}
  if (f.ownerView) {
    t.total_kwh = `数据来源：市电账单「本期电量」= ${n2(f.totalKwh)} 千瓦时`
    t.total_fee = `数据来源：市电账单「合计电费」= ${n2(f.totalFee)} 元`
    t.res_kwh = `数据来源：市电账单 · 电量明细（居民）主行「计费电量」= ${n2(f.resKwh)}`
    t.res_fee = `数据来源：市电账单「居民电费」（目录电费 + 政府性基金及附加）= ${n2(f.resFee)} 元`
    t.ind_kwh = [
      '计算方式：市电总电量 − 定比电量 − 租户工业分表电量',
      `= ${n2(f.totalKwh)} − ${n2(f.resKwh)} − ${n2(f.recKwh)} = ${n2(f.indKwh)}`,
    ].join('\n')
    const priceDenom = f.totalKwh - f.tenantDormKwh - f.recKwh
    t.ind_price = [
      '计算方式：工商业总电费 ÷ 工商业总电量',
      `= (${n2(f.totalFee)} − ${n2(f.tenantDormFee)} − ${n2(f.recFee)}) ÷ ${n2(priceDenom)}`,
      `= ${Number(f.indPrice) || 0} 元/kWh`,
    ].join('\n')
    t.ind_fee = `计算方式：用电量（工业）× 均价（工业）\n= ${n2(f.indKwh)} × ${Number(f.indPrice) || 0} = ${n2(f.indFee)} 元`
    t.dorm_kwh = `数据来源：电费核算「用途=宿舍」电表当月用电量汇总 = ${n2(f.dormKwh)} 千瓦时`
    t.dorm_fee = `计算方式：各宿舍表「用电量 × 单价」求和 = ${n2(f.dormFee)} 元`
    t.water_ind_usage = [
      '计算方式：自来水总表 − 星达宿舍 − 租户宿舍 − 租户工业',
      `= ${n2(f.waterTotalUsage)} − ${n2(f.waterDormUsage)} − ${n2(f.waterTenantDormUsage)} − ${n2(f.waterTenantIndUsage)}`,
      `= ${n2(f.waterIndUsage)} 吨`,
    ].join('\n')
    t.water_ind_fee = `计算方式：工业用水量 × 自来水总表单价\n= ${n2(f.waterIndUsage)} × ${n2(f.waterTotalPrice)} = ${n2(f.waterIndFee)} 元`
    t.water_dorm_usage = `数据来源：水表（用途=宿舍、归属星达）当月用量汇总 = ${n2(f.waterDormUsage)} 吨`
    t.water_dorm_fee = `数据来源：水表（用途=宿舍、归属星达）当月水费汇总 = ${n2(f.waterDormFee)} 元`
    t.water_fire_usage = `数据来源：水表（类型=消防）当月用量汇总 = ${n2(f.waterFireUsage)} 吨`
    t.water_fire_fee = `数据来源：水表（类型=消防）当月水费汇总 = ${n2(f.waterFireFee)} 元`
  } else {
    t.total_kwh = `数据来源：市电账单「本期电量」= ${n2(f.totalKwh)} 千瓦时（全厂口径）`
    t.total_fee = `数据来源：市电账单「合计电费」= ${n2(f.totalFee)} 元（全厂口径）`
    t.ratio = [
      `计算方式：${f.tenantName}分表电量 ÷ 市电账单本期电量`,
      `= ${n2(f.recKwh)} ÷ ${n2(f.totalKwh)} = ${pct(f.ratio)}`,
    ].join('\n')
    t.ind_kwh = `数据来源：已生成账单「${f.tenantName}分表总电量」= ${n2(f.recKwh)} 千瓦时`
    t.ind_fee = `数据来源：已生成账单「工业电费」（各分摊项按比例折算后求和）= ${n2(f.recFee)} 元`
    t.ind_price = `计算方式：工业电费 ÷ 工业用电量\n= ${n2(f.recFee)} ÷ ${n2(f.recKwh)} = ${Number(f.indPrice) || 0} 元/kWh`
    t.dorm_kwh = `数据来源：电费核算「用途=宿舍、使用单位=${f.tenantName}」用电量汇总 = ${n2(f.dormKwh)} 千瓦时`
    t.dorm_fee = `计算方式：宿舍各表「用电量 × 单价」求和 = ${n2(f.dormFee)} 元`
    t.water_ind_usage = `数据来源：水表（归属/使用单位=${f.tenantName}、非宿舍非公租）当月用量 = ${n2(f.waterIndUsage)} 吨`
    t.water_ind_fee = `计算方式：各工业水表当月水费求和 = ${n2(f.waterIndFee)} 元`
    t.water_dorm_usage = `数据来源：水表（宿舍、${f.tenantName}）当月用量 = ${n2(f.waterDormUsage)} 吨`
    t.water_dorm_fee = `数据来源：水表（宿舍、${f.tenantName}）当月水费 = ${n2(f.waterDormFee)} 元`
  }
  if (f.gasCount) {
    t.gas_usage = `计算方式：${f.gasCount} 条气表记录（剔除公租房）用量求和 = ${n2(f.gasUsage)} m³`
    t.gas_fee = `计算方式：${f.gasCount} 条气表记录「用量 × 单价」求和 = ${n2(f.gasFee)} 元`
  }
  return t
}

/**
 * 生成整行的悬浮提示。
 *
 * @param facts 该行的中间值
 * @param extras 非内置列（跨分类引用列 / 租户分摊子项）的说明，由调用方按各自口径产出
 */
export function buildRowTips(facts: RowFacts, extras: Record<string, string> = {}): Record<string, string> {
  return { ...builtinTips(facts), ...extras }
}

/** 租户分摊子项的提示：分时项写逐段算式，固定项写「金额 × 比例」。 */
export function tenantItemTip(opts: {
  itemName: string
  sourceCategory?: string
  /** 原账单该项金额（分时项为合计金额） */
  billAmount?: number
  ratio: number
  /** 分时段明细：[{ period, qty, rate, fee }] */
  periods?: Array<{ period: string; qty: number; rate: number; fee: number }>
  /** 该行最终值 */
  value: number
  /** 是否参与分摊 */
  participating: boolean
}): string {
  const { itemName, sourceCategory, billAmount, ratio, periods, value, participating } = opts
  const head = sourceCategory ? `来源：${sourceCategory}` : '来源：市电账单费用分项'
  if (!participating) {
    return `${head}\n「${itemName}」未开启「参与分摊」，本行金额记 0（不计入应付合计）`
  }
  if (periods && periods.length) {
    const lines = periods
      .filter((p) => p.qty || p.fee)
      .map((p) => `  ${p.period || '平'}：${n2(p.qty)} × ${p.rate} = ${n2(p.fee)}`)
    const sum = periods.reduce((s, p) => s + (Number(p.fee) || 0), 0)
    return [
      `${head}`,
      `计算方式：各时段「分摊电量 × 单价」后求和，再按分摊比例折算`,
      ...lines,
      `  小计 = ${n2(sum)}`,
      `合计 = ${n2(sum)} × ${pct(ratio)} = ${n2(value)} 元`,
    ].join('\n')
  }
  const base = Number(billAmount) || 0
  return [
    head,
    `计算方式：账单该项金额 × 分摊比例`,
    `= ${n2(base)} × ${pct(ratio)} = ${n2(value)} 元`,
  ].join('\n')
}

/** 跨分类引用字段的提示。 */
export function sourceFieldTip(opts: {
  category: string
  groupLabel?: string
  /** 原字段名（可能已在费用清单改名，提示里显示原名才能回溯） */
  fieldLabel: string
  month: string
  value: number | string
  /** 多行汇总时给出明细条数 */
  rowCount?: number
}): string {
  const { category, groupLabel, fieldLabel, month, value, rowCount } = opts
  const path = groupLabel ? `${category} · ${groupLabel}` : category
  const shown = typeof value === 'number' ? n2(value) : String(value ?? '—')
  if (rowCount && rowCount > 1) {
    return `数据来源：[${path}] ${fieldLabel}（${month}）\n当月 ${rowCount} 条记录汇总 = ${shown}`
  }
  return `数据来源：[${path}] ${fieldLabel}（${month}）= ${shown}`
}
