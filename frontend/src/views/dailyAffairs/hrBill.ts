/**
 * 月度账单明细的行/人数口径（第 57 轮定义，第 58 轮抽成纯函数以便回归）。
 *
 * 两条铁律：
 *   ① **一人多条记录必须逐行独立展示**（陈刚：正常缴费行 636.58 / 1693.24
 *      与补缴异常行 511.28 / 1091.73 是两条记录，合并成一行就等于篡改金额明细）；
 *   ② **人数按人去重，且补缴行不计人**（社保恒 75 人，列表 76 行）。
 *
 * 抽出来的原因：这两个数字是打印 / 导出 / 合计行 / 卡片提醒的公共输入，
 * 留在组件 computed 里只能靠真机断言，无法在单测里锁口径。
 */

/** 是否补缴行（社保账单明细字段 `是否补缴`，兼容后端 is_adjustment）。 */
export function isAdjustmentRow(row: any): boolean {
  if (!row) return false
  const v = row['是否补缴'] ?? row.is_adjustment ?? row.isAdjustment
  return v === true || v === 'true' || v === 1 || v === '1' || v === '是'
}

/**
 * 社保账单人数：按证件号码（回退姓名）去重，**补缴行不计数**。
 *
 * 例：75 名正常员工 + 陈刚补缴行 → 行数 76，人数仍是 75。
 */
export function siBillPeopleOf(rows: readonly any[] | null | undefined): number {
  if (!rows || !rows.length) return 0
  const keys = new Set<string>()
  for (const r of rows) {
    if (isAdjustmentRow(r)) continue
    const key = String(r?.['证件号码'] ?? r?.id_number ?? r?.['姓名'] ?? r?.name ?? '').trim()
    if (key) keys.add(key)
  }
  return keys.size
}

/** 公积金账单人数：按证件号码（回退姓名）去重，没有补缴概念。 */
export function billPeopleCountOf(rows: readonly any[] | null | undefined): number {
  if (!rows || !rows.length) return 0
  const keys = new Set<string>()
  for (const r of rows) {
    const key = String(r?.['证件号码'] ?? r?.identity_number ?? r?.['姓名'] ?? r?.name ?? '').trim()
    if (key) keys.add(key)
  }
  return keys.size
}

/**
 * 个人 / 单位明细的行序号：**严格 1:1**，不按人合并、不去重。
 *
 * 返回与入参等长的序号数组（'1'、'2'…），调用方据此渲染明细表；
 * 单独抽出来是为了让「同人多行」这条契约可以被单测直接锁住。
 */
export function subjectRowSeqOf(rows: readonly any[] | null | undefined): string[] {
  if (!rows || !rows.length) return []
  return rows.map((_, i) => String(i + 1))
}

/** 汇总分组（部门 / 岗位同构）：人数 + 个人 / 单位金额（分）。 */
export interface BillGroupCents {
  key: string
  people: number
  personalCents: number
  unitCents: number
}

/**
 * 第 67 轮：部门汇总 / 岗位汇总的**同一套**分累加口径。
 *
 * 两个视图必须逐分相等（2026-11 公积金：74 人 / 22,939.00 / 22,939.00 / 45,878.00），
 * 唯一差别是分组键（部门 → 岗位），所以聚合逻辑抽成纯函数，页面只负责投影成表格行。
 * 排序：人数降序 → 合计金额降序 → 键名（中文）升序。
 */
export function groupBillCents(
  rows: readonly any[] | null | undefined,
  opts: {
    keyOf: (row: any) => string
    personalCentsOf: (row: any) => number
    unitCentsOf: (row: any) => number
    /** 是否计入人数（默认全部计入；社保补缴行传 false）。 */
    countPerson?: (row: any) => boolean
  },
): BillGroupCents[] {
  if (!rows || !rows.length) return []
  const map = new Map<string, BillGroupCents>()
  for (const r of rows) {
    const key = String(opts.keyOf(r) ?? '').trim() || '未分配'
    const e = map.get(key) || { key, people: 0, personalCents: 0, unitCents: 0 }
    if (!opts.countPerson || opts.countPerson(r)) e.people++
    e.personalCents += opts.personalCentsOf(r) || 0
    e.unitCents += opts.unitCentsOf(r) || 0
    map.set(key, e)
  }
  return [...map.values()].sort((a, b) =>
    b.people - a.people
    || (b.personalCents + b.unitCents) - (a.personalCents + a.unitCents)
    || a.key.localeCompare(b.key, 'zh-Hans-CN'))
}
