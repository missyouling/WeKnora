/**
 * 费用清单动态列的纯函数集合（第 51 轮）。
 *
 * 抽出来的原因：TenantBilling.vue 里的「动态列取值」「动态列合计口径」
 * 「本期水量/水费派生」是六同步的核心规则，必须有可直接运行的单元测试，
 * 不能只靠真机截图验证。Vue 侧只保留装配，不再内联这些判断。
 */

export type DynamicColType = 'text' | 'number' | 'amount'

/**
 * 动态列（设置面板新增/复制、无静态渲染器）的合计口径。
 *
 * 只能按 label 判，不能拼上 key：复制字段的 key 可能是「总电费_副本」，
 * 拼进去会误命中「电费」，把「本期水量」错判成金额列。
 */
export function dynamicColTypeOf(label: unknown): DynamicColType {
  const l = String(label ?? '').trim()
  if (/单价|均价|比例|倍率|系数|税率|编号/.test(l)) return 'text'
  if (/费|金额|税额|价税|结算/.test(l)) return 'amount'
  if (/量|度数/.test(l)) return 'number'
  return 'text'
}

function present(v: any): boolean {
  return v !== '' && v !== null && v !== undefined
}

/**
 * 动态列多级取值：英文 key → 中文 label → row.data[key] → row.data[label]。
 *
 * 用户在设置面板复制字段时只改了 label，name 仍是历史副本名，
 * row 里自然没有这个键；中文 label 与 row.data 兜底保证派生值能上屏。
 */
export function rowValueOf(key: string, row: any, label?: string): any {
  if (!row) return undefined
  if (present(row?.[key])) return row[key]
  const l = String(label ?? '').trim()
  if (l && l !== key && present(row?.[l])) return row[l]
  if (row?.data) {
    if (present(row.data[key])) return row.data[key]
    if (l && present(row.data[l])) return row.data[l]
  }
  // 空值统一收敛成 undefined，调用方只需要判一次
  return undefined
}

export interface WaterAgg {
  usage: number
  fee: number
  price: number
}

/**
 * 本期水量 / 本期水费 = 自来水总表 + 消防总表。
 *
 * 台账里如果只记了抄表读数、没记金额，按「用量 × 总表单价」回算，
 * 否则会出现 769 吨水却算出 0 元水费的荒谬结果。
 */
export function waterPeriodTotals(main: WaterAgg, fire: WaterAgg): { usage: number; fee: number } {
  const usage = round2((Number(main?.usage) || 0) + (Number(fire?.usage) || 0))
  const rawFee = round2((Number(main?.fee) || 0) + (Number(fire?.fee) || 0))
  const price = Number(main?.price || 0) || Number(fire?.price || 0) || 0
  return {
    usage,
    fee: rawFee > 0 ? rawFee : round2(usage * price),
  }
}

function round2(v: number): number {
  return Math.round((v + Number.EPSILON) * 100) / 100
}
