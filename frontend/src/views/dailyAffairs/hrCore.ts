/**
 * 人事管理（第 45 轮）纯函数核心层。
 *
 * 这里只放「不依赖 Vue / 组件 / 网络」的纯函数，便于 `tsx --test` 直接单测：
 *   · Tab / KPI / scope 映射；
 *   · 列定义与合计列（TDesign 1.19 契约：colKey、无 fixed、无操作列）；
 *   · YYYY-MM 严格倒序；
 *   · 字段种子（列宽全 0、状态字段 builtin）；
 *   · 表尾合计行、行键。
 *
 * 页面（HrManagement.vue）只负责把数据喂进来，不在模板里散落字符串判断。
 */

import { buildFootRow, type SummaryColumn } from './listSummary'
import { grouped } from './formatNumber'

// ---------------------------------------------------------------------------
// Tab / scope
// ---------------------------------------------------------------------------

export type HrTab = 'hr-roster' | 'hr-social' | 'hr-fund'

export interface HrTabDef {
  key: HrTab
  label: string
  icon: string
  sub: string
}

/**
 * 三个一级 Tab 的唯一配置表。
 * key 同时作为字段配置 scope（`hrScopeOf` 返回它），KPI 卡的 `typeValue` 也用它。
 */
export const HR_TABS: readonly HrTabDef[] = [
  { key: 'hr-roster', label: '花名册', icon: 'usergroup', sub: '在职员工档案统一管理' },
  { key: 'hr-social', label: '社保管理', icon: 'shield', sub: '五险账期与扣款明细' },
  { key: 'hr-fund', label: '公积金管理', icon: 'wallet', sub: '缴存台账与月度账单' },
]

/** 字段配置 scope：tab 值本身即 scope；未知值兜底到花名册。 */
export function hrScopeOf(tab: string): HrTab {
  const hit = HR_TABS.find((t) => t.key === tab)
  return hit ? hit.key : 'hr-roster'
}

/**
 * 恰好 3 张 KPI 卡（花名册 / 社保管理 / 公积金管理）。
 *
 * 铁律：**绝不产出「已收录记录」「历史记录」之类的汇总卡 / 统计卡** ——
 * 第 44 轮已把车队的两张汇总卡彻底移除，人事页从第一天起就不引入。
 * 三张卡都是 `action:'type'` + `typeValue`，只有它们能高亮（`DashboardKpiGroup.isActive`）。
 */
export function buildHrKpiCards(counts: Record<string, number> = {}) {
  return HR_TABS.map((t) => ({
    key: t.key,
    label: t.label,
    icon: t.icon,
    value: String(Math.max(0, Number(counts[t.key] || 0) || 0)),
    unit: '条',
    sub: t.sub,
    cls: '',
    action: 'type',
    typeValue: t.key,
  }))
}

// ---------------------------------------------------------------------------
// 列定义
// ---------------------------------------------------------------------------

export interface HrColumn {
  /** TDesign 1.19 列标识（中文表头名即 colKey，与中台其它列表一致）。 */
  colKey: string
  title: string
  /** 勾选列专用：type='multiple'。 */
  type?: string
  width: number
  dataType: string
  default: boolean
  /** 走具名插槽渲染（状态 Tag / 月份单元格），页面据此不设 cell。 */
  slot?: boolean
  /** 单元格取值（兼容扁平 snake_case 行与 data/custom_data 包裹行）。 */
  value?: (row: any) => string
}

const STATUS_COLS = new Set(['在职状态', '账期状态', '参保状态', '缴存状态'])
export const HR_STATUS_FIELDS: readonly string[] = [
  '在职状态', '账期状态', '参保状态', '缴存状态',
]

/**
 * 从行里按候选键取值；空值显示 `—`。
 * 覆盖后端的三种包裹层：data（旧台账）、custom_data（花名册/公积金扩展列）、
 * summary_data（社保账期聚合结果，JSONB）。
 */
function pickRaw(row: any, keys: readonly string[]): any {
  for (const k of keys) {
    const v = row?.[k] ?? row?.data?.[k] ?? row?.custom_data?.[k] ?? row?.summary_data?.[k]
    if (v !== undefined && v !== null && v !== '') return v
  }
  return undefined
}
function pick(row: any, keys: readonly string[]): string {
  const v = pickRaw(row, keys)
  return v === undefined ? '—' : String(v)
}

/** 金额列统一走 formatNumber.grouped（不自己 toFixed）。 */
function money(row: any, keys: readonly string[]): string {
  const v = pickRaw(row, keys)
  return v === undefined
    ? '—'
    : grouped(v, { fixed: true, fallback: '—', blankWhenEmpty: true })
}

type ColOpts = { dataType?: string; slot?: boolean; money?: boolean }

/** 后端状态是英文枚举（active/sealed/resigned…），列表按中文列头展示中文。 */
const STATUS_TEXT: Record<string, string> = {
  active: '在职', normal: '正常', sealed: '封存', resigned: '离职',
  inactive: '停缴', pending: '待处理', processing: '处理中', done: '已完成',
  suspended: '暂停', adjusted: '已补退',
  // 社保账期（HRSIPeriod.Status）：新建默认 pending，汇总后 processed
  processed: '已汇总', draft: '草稿',
}
function normalizeStatus(v: string): string {
  const s = String(v ?? '').trim()
  return STATUS_TEXT[s.toLowerCase()] || s
}

function col(colKey: string, keys: readonly string[], opts: ColOpts = {}): HrColumn {
  const isStatus = STATUS_COLS.has(colKey)
  return {
    colKey,
    title: colKey,
    width: 0,
    dataType: opts.dataType || 'text',
    default: true,
    slot: !!opts.slot || isStatus,
    value: opts.money
      ? (row: any) => money(row, keys)
      : isStatus
        ? (row: any) => normalizeStatus(pick(row, keys))
        : (row: any) => pick(row, keys),
  }
}

/** 第一列永远是勾选列，且**不带 fixed**（第 38 轮起全站取消冻结列）。 */
const SELECT_COL: HrColumn = {
  colKey: 'row-select',
  title: '',
  type: 'multiple',
  width: 44,
  dataType: 'text',
  default: true,
}

/** 花名册列（hr_employees）。 */
const ROSTER_COLS: HrColumn[] = [
  col('工号', ['工号', 'employee_id', 'employee_no', 'staff_no']),
  col('姓名', ['姓名', 'name', 'employee_name']),
  col('性别', ['性别', 'gender']),
  col('部门', ['部门', 'department', 'dept']),
  col('岗位', ['岗位', 'position', 'job', 'job_title']),
  col('入职日期', ['入职日期', 'hire_date', 'entry_date', 'onboard_date'], { dataType: 'date' }),
  col('在职状态', ['在职状态', 'status', 'employ_status', 'employment_status'], { slot: true }),
  col('联系电话', ['联系电话', 'phone', 'mobile', 'tel']),
]

/** 社保账期列（hr_si_periods）；账期为 YYYY-MM，前端二次倒序兜底。 */
const SOCIAL_COLS: HrColumn[] = [
  col('账期', ['账期', 'year_month', 'month', 'period'], { slot: true }),
  col('账期状态', ['账期状态', 'status', 'period_status'], { slot: true }),
  col('参保人数', ['参保人数', 'headcount', 'employee_count', 'insured_count', 'record_count'], { dataType: 'number' }),
  col('个人扣款合计', ['个人扣款合计', 'personal_total', 'personal_sum', 'personal_amount_total', 'personal_subtotal'], { dataType: 'number', money: true }),
  col('单位扣款合计', ['单位扣款合计', 'unit_total', 'unit_sum', 'company_total', 'company_amount_total', 'unit_subtotal'], { dataType: 'number', money: true }),
  col('参保状态', ['参保状态', 'insure_status', 'insured_status'], { slot: true }),
  col('创建时间', ['创建时间', 'created_at', 'create_time']),
]

/** 公积金缴存台账列（hr_pf_records）。 */
const FUND_LEDGER_COLS: HrColumn[] = [
  // 台账是「人员缴存配置」，没有月份字段（month_label 属于月度账单视图）；
  // 这里放月份列会让整列恒为「—」，故不设置。
  // 后端台账主键是 personal_account / identity_number，不是 employee_id
  col('工号', ['工号', 'employee_id', 'employee_no', 'personal_account', 'identity_number']),
  col('姓名', ['姓名', 'name', 'employee_name']),
  col('缴存基数', ['缴存基数', 'base_amount', 'base', 'salary_base', 'personal_base'], { dataType: 'number', money: true }),
  // 后端只有一个 contribution_ratio（比例 6%），个人/单位两列共用它做兜底
  col('个人比例', ['个人比例', 'personal_rate', 'personal_ratio', 'contribution_ratio']),
  col('单位比例', ['单位比例', 'company_rate', 'company_ratio', 'unit_rate', 'contribution_ratio']),
  col('个人金额', ['个人金额', 'personal_amount', 'personal'], { dataType: 'number', money: true }),
  col('单位金额', ['单位金额', 'company_amount', 'company', 'unit_amount'], { dataType: 'number', money: true }),
  col('缴存状态', ['缴存状态', 'status', 'fund_status'], { slot: true }),
]

/** 公积金月度账单列（hr_pf_bills）；月份严格倒序。 */
const FUND_BILL_COLS: HrColumn[] = [
  col('月份', ['月份', 'month_label', 'month'], { slot: true }),
  col('缴存人数', ['缴存人数', 'headcount', 'employee_count', 'record_count'], { dataType: 'number' }),
  col('个人合计', ['个人合计', 'personal_total', 'personal_sum', 'personal_amount_total'], { dataType: 'number', money: true }),
  col('单位合计', ['单位合计', 'company_total', 'company_sum', 'company_amount_total'], { dataType: 'number', money: true }),
  col('合计金额', ['合计金额', 'total', 'total_amount', 'amount', 'combined_amount_total'], { dataType: 'number', money: true }),
  col('生成时间', ['生成时间', 'created_at', 'create_time']),
]

function colsOf(tab: string): HrColumn[] {
  switch (hrScopeOf(tab)) {
    case 'hr-social': return SOCIAL_COLS
    case 'hr-fund': return FUND_LEDGER_COLS
    default: return ROSTER_COLS
  }
}

/**
 * 列定义：一律**不带 fixed**、**不带操作列**；第一列是勾选列但不是 fixed。
 * 单条编辑/删除走「点行 → 详情抽屉」，不做右侧冻结操作列（第 31 轮中台口径）。
 */
export function hrColumnDefs(tab: string): HrColumn[] {
  return [SELECT_COL, ...colsOf(tab)]
}

/** 公积金「月度账单」子视图列（二级切换用）。 */
export function hrBillColumnDefs(): HrColumn[] {
  return [SELECT_COL, ...FUND_BILL_COLS]
}

/** 合计列：与列定义同源，可累加列才给 number/amount 类型。 */
function summaryOf(cols: readonly HrColumn[]): SummaryColumn[] {
  return cols
    .filter((c) => c.colKey !== 'row-select')
    .map((c) => ({
      key: c.colKey,
      label: c.title,
      fieldType: c.dataType === 'number' ? 'number' : 'text',
      value: (row: any) => c.value?.(row) ?? '',
    }))
}

export function hrSummaryColumns(tab: string): SummaryColumn[] {
  return summaryOf(hrColumnDefs(tab))
}

export function hrBillSummaryColumns(): SummaryColumn[] {
  return summaryOf(hrBillColumnDefs())
}

// ---------------------------------------------------------------------------
// 月份倒序
// ---------------------------------------------------------------------------

const MONTH_RE = /^\d{4}-(0[1-9]|1[0-2])$/

/**
 * 严格 `YYYY-MM` 倒序（字符串比较即可）。
 *
 * · 过滤非法值：非 `YYYY-MM` 或月份越界的行不参与结果（脏数据不该混进时间轴）；
 * · 稳定：同为一个月的多行保持输入原顺序（用原始下标做次级键）；
 * · 兼容 `string[]`（此时忽略 key，元素本身即月份）。
 */
export function sortMonthsDesc<T>(rows: readonly T[], key: string): T[] {
  const src = Array.isArray(rows) ? rows : []
  const valid: Array<{ row: T; month: string; i: number }> = []
  src.forEach((row, i) => {
    const raw = typeof row === 'string'
      ? row
      : ((row as any)?.[key] ?? (row as any)?.data?.[key] ?? (row as any)?.custom_data?.[key])
    const m = String(raw ?? '').trim()
    if (MONTH_RE.test(m)) valid.push({ row, month: m, i })
  })
  valid.sort((a, b) => (a.month < b.month ? 1 : a.month > b.month ? -1 : a.i - b.i))
  return valid.map((x) => x.row)
}

/** 是否合法 `YYYY-MM`（页面据此决定是否给单元格挂 data-hr-month-cell）。 */
export function isHrMonth(v: unknown): boolean {
  return MONTH_RE.test(String(v ?? '').trim())
}

// ---------------------------------------------------------------------------
// 字段种子 / 合计行 / 行键
// ---------------------------------------------------------------------------

export interface HrFieldSeed {
  name: string
  label: string
  enabled: boolean
  is_default: boolean
  data_type: string
  width: number
  builtin?: boolean
}

/**
 * 字段配置种子（与后端 `fleet_categories.subs` 同形）。
 * 列宽一律 0（按内容自适应，第 44 轮口径）；四个状态字段 builtin + enabled。
 */
export function hrFieldSeeds(scope: string): HrFieldSeed[] {
  return colsOf(scope).map((c) => {
    const isStatus = STATUS_COLS.has(c.colKey)
    return {
      name: c.colKey,
      label: c.title,
      enabled: true,
      is_default: c.default,
      data_type: c.dataType,
      width: 0,
      ...(isStatus ? { builtin: true } : {}),
    }
  })
}

/**
 * 表尾合计行（复用中台 buildFootRow）。
 * 0 行时：首列 `合计（共 0 条）`，其余全部空串（不写 0 / 0.00 / —）。
 */
export function hrFootRowOf(tab: string, rows: readonly any[]): Record<string, string> {
  return buildFootRow(hrSummaryColumns(hrScopeOf(tab)), (rows || []) as any[], {
    selectKey: 'row-select',
  })
}

/** 行键：id 优先，依次回退业务唯一键。 */
export function hrRowKey(row: any): string {
  return String(
    row?.id ?? row?._id ?? row?.employee_id ?? row?.year_month ?? row?.month_label ?? '',
  )
}
