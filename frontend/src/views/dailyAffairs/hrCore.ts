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
import { formatCellDateTime } from './formatDateTime'
import {
  ROSTER_TEMPLATE_HEADERS, ROSTER_DERIVED_FIELDS, EXTRA_RESIGN_FIELDS,
  ROSTER_STATUS_COLUMNS, type RosterStatusView,
} from './hrRosterFields'
import { allRosterFields } from './hrRosterGroups'
import {
  HR_RESIGN_FIELDS, HR_CONTRACT_FIELDS, HR_FUND_FIELDS,
  HR_SOCI_RECEIPT_FIELDS, HR_SOCI_NORMAL_FIELDS, HR_SOCI_ABNORMAL_FIELDS, HR_SOCI_BILL_FIELDS,
  hrValueKeys, hrIsDateField,
} from './hrFieldSchema'
import type { HrFieldDef } from './hrFieldSchema'

// ---------------------------------------------------------------------------
// Tab / scope
// ---------------------------------------------------------------------------

export type HrTab = 'hr-roster' | 'hr-resign' | 'hr-contract' | 'hr-social' | 'hr-fund'

export interface HrTabDef {
  key: HrTab
  label: string
  icon: string
  sub: string
}

/**
 * 五个一级 Tab 的唯一配置表（第 52 轮新增「离职管理」）。
 * key 同时作为字段配置 scope（`hrScopeOf` 返回它），KPI 卡的 `typeValue` 也用它。
 * 顺序严格：花名册 → 离职管理 → 劳动合同 → 社保管理 → 公积金管理。
 */
export const HR_TABS: readonly HrTabDef[] = [
  { key: 'hr-roster', label: '花名册', icon: 'usergroup', sub: '' },
  { key: 'hr-resign', label: '离职管理', icon: 'logout', sub: '' },
  { key: 'hr-contract', label: '劳动合同', icon: 'file-paste', sub: '' },
  // tdesign-icons-vue-next 0.4.4 的 manifest 里没有 `shield`（只有 shield-error），
  // 传无效名会让 <use href="#t-icon-shield"> 解析失败 → 图标框空白。secured 已核验有效。
  { key: 'hr-social', label: '社保管理', icon: 'secured', sub: '' },
  { key: 'hr-fund', label: '公积金管理', icon: 'wallet', sub: '' },
]

/** 字段配置 scope：tab 值本身即 scope；未知值兜底到花名册。 */
export function hrScopeOf(tab: string): HrTab {
  const hit = HR_TABS.find((t) => t.key === tab)
  return hit ? hit.key : 'hr-roster'
}

/**
 * 恰好 5 张 KPI 卡（花名册 / 离职管理 / 劳动合同 / 社保管理 / 公积金管理）。
 *
 * 铁律：**绝不产出「已收录记录」「历史记录」之类的汇总卡 / 统计卡** ——
 * 第 44 轮已把车队的两张汇总卡彻底移除，人事页从第一天起就不引入。
 * 五张卡都是 `action:'type'` + `typeValue`，只有它们能高亮（`DashboardKpiGroup.isActive`）。
 */
export function buildHrKpiCards(counts: Record<string, number> = {}) {
  const unsigned = Math.max(0, Number(counts.contract_unsigned || 0) || 0)
  const expiring = Math.max(0, Number(counts.contract_expiring || 0) || 0)
  const expired = Math.max(0, Number(counts.contract_expired || 0) || 0)
  const fundPending = Math.max(0, Number(counts.fund_pending || 0) || 0)
  const fundToSeal = Math.max(0, Number(counts.fund_to_seal || 0) || 0)
  const socialPending = Math.max(0, Number(counts.social_pending || 0) || 0)
  const socialToSeal = Math.max(0, Number(counts.social_to_seal || 0) || 0)
  return HR_TABS.map((t) => {
    // 第 49 轮：劳动合同卡副文案体现异常预警（到期 / 即将到期）
    let sub = t.sub
    let subCls = ''
    if (t.key === 'hr-contract') {
      // 第 55 轮：异常 = 未签订 + 即将到期 + 已到期，副文案只报总数（单一异常带类型后缀）。
      const kinds = [unsigned > 0 ? '未签订' : '', expiring > 0 ? '即将到期' : '',
        expired > 0 ? '到期' : ''].filter(Boolean)
      const total = unsigned + expiring + expired
      if (total > 0) {
        sub = `${total} 份状态异常${kinds.length === 1 ? ` · ${kinds[0]}` : ''}`
        subCls = 'is-warning'
      } else {
        sub = '无状态异常'
      }
    }
    if (t.key === 'hr-fund') {
      // 第 55 轮：全员联动预警（待补录 / 待封存）
      const parts: string[] = []
      if (fundPending > 0) parts.push(`${fundPending} 人待补录`)
      if (fundToSeal > 0) parts.push(`${fundToSeal} 人待封存`)
      if (parts.length) {
        sub = parts.join(' · ')
        subCls = 'is-warning'
      }
    }
    if (t.key === 'hr-social') {
      // 第 56 轮：卡片主数字恒为月度账单条数，底部提醒待补录社保 / 离职待减员
      const parts: string[] = []
      if (socialPending > 0) parts.push(`${socialPending} 人待补录社保`)
      if (socialToSeal > 0) parts.push(`${socialToSeal} 人离职待减员`)
      if (parts.length) {
        sub = parts.join(' · ')
        subCls = 'is-warning'
      }
    }
    return {
      key: t.key,
      label: t.label,
      icon: t.icon,
      value: String(Math.max(0, Number(counts[t.key] || 0) || 0)),
      unit: '条',
      sub,
      subCls,
      cls: '',
      action: 'type',
      typeValue: t.key,
    }
  })
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
  /** 用户显式配置列宽时的最小宽度（= width，禁止被末列拉伸/自适应覆盖）。 */
  minWidth?: number
  dataType: string
  default: boolean
  /** 走具名插槽渲染（状态 Tag / 月份单元格），页面据此不设 cell。 */
  slot?: boolean
  /** 单元格取值（兼容扁平 snake_case 行与 data/custom_data 包裹行）。 */
  value?: (row: any) => string
  /** 合计行是否累加：比例类列（缴存比例）标 false，避免把 5% 加成 60%。 */
  noSum?: boolean
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
  // 第 49 轮：人事列表空值一律留白（不渲染横线 —）
  return v === undefined ? '' : String(v)
}

/** 金额列统一走 formatNumber.grouped（不自己 toFixed）。 */
function money(row: any, keys: readonly string[]): string {
  const v = pickRaw(row, keys)
  return v === undefined
    ? ''
    : grouped(v, { fixed: true, fallback: '', blankWhenEmpty: true })
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
  // 时间/日期列统一走格式化出口：后端 ISO8601 原串（带 T、微秒、+08:00）
  // 绝不能直接上屏。这一层同时覆盖列表 cell、合计行与详情抽屉的灌入值。
  const isDate = opts.dataType === 'date' || /时间|日期|有效期/.test(colKey)
  return {
    colKey,
    title: colKey,
    width: 0,
    dataType: opts.dataType || (isDate ? 'date' : 'text'),
    default: true,
    slot: !!opts.slot || isStatus,
    value: opts.money
      ? (row: any) => money(row, keys)
      : isStatus
        ? (row: any) => normalizeStatus(pick(row, keys))
        : isDate
          ? (row: any) => formatCellDateTime(colKey, pickRaw(row, keys)) || ''
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

// ---------------------------------------------------------------------------
// 劳动合同（第 47 轮）：数据与花名册合同字段同源
// ---------------------------------------------------------------------------

// 第 52 轮：劳动合同列全集改由 `hrContractColumns()` 从字段契约派生
// （见文末「离职管理 / 劳动合同」段），此处不再维护第二份硬编码清单。

// ---------------------------------------------------------------------------
// 社保原始记录 / 汇总记录（第 47 轮双视图）
// ---------------------------------------------------------------------------

/** 原始记录列：一行 = 某人某险种的明细。 */
const SI_RAW_COLS: HrColumn[] = [
  col('账期', ['账期', 'year_month'], { slot: true }),
  col('记录性质', ['记录性质', 'file_type'], { slot: true }),
  col('险种', ['险种', 'scheme']),
  col('缴费类型', ['缴费类型', 'part']),
  col('个人社保编号', ['个人社保编号', 'person_code', 'personCode']),
  col('姓名', ['姓名', 'name']),
  col('证件号码', ['证件号码', 'id_number']),
  col('部门', ['部门', 'department']),
  col('缴费基数', ['缴费基数', 'pay_base'], { dataType: 'number', money: true }),
  col('费率', ['费率', 'rate_text', 'rate']),
  col('单位应缴', ['单位应缴', 'unit_amount'], { dataType: 'number', money: true }),
  col('个人应缴', ['个人应缴', 'personal_amount'], { dataType: 'number', money: true }),
  col('合计金额', ['合计金额', 'amount_due'], { dataType: 'number', money: true }),
  col('补退金额', ['补退金额', 'adjust_amount'], { dataType: 'number', money: true }),
]

/** 汇总记录列：一行 = 一个账期的全员合计。 */
const SI_SUMMARY_COLS: HrColumn[] = [
  col('账期', ['账期', 'year_month'], { slot: true }),
  col('参保人数', ['参保人数', 'headcount', 'record_count'], { dataType: 'number' }),
  col('养老单位', ['养老单位', 'pension_unit'], { dataType: 'number', money: true }),
  col('养老个人', ['养老个人', 'pension_personal'], { dataType: 'number', money: true }),
  col('医疗单位', ['医疗单位', 'medical_unit'], { dataType: 'number', money: true }),
  col('医疗个人', ['医疗个人', 'medical_personal'], { dataType: 'number', money: true }),
  col('大病', ['大病', 'serious_illness'], { dataType: 'number', money: true }),
  col('失业单位', ['失业单位', 'unemploy_unit'], { dataType: 'number', money: true }),
  col('失业个人', ['失业个人', 'unemploy_personal'], { dataType: 'number', money: true }),
  col('工伤单位', ['工伤单位', 'injury_unit'], { dataType: 'number', money: true }),
  col('补退金额', ['补退金额', 'adjust_amount'], { dataType: 'number', money: true }),
  col('单位合计', ['单位合计', 'unit_total'], { dataType: 'number', money: true }),
  col('个人合计', ['个人合计', 'personal_total'], { dataType: 'number', money: true }),
  col('应缴总计', ['应缴总计', 'grand_total'], { dataType: 'number', money: true }),
  col('状态', ['状态', 'status'], { slot: true }),
]

function colsOf(tab: string): HrColumn[] {
  switch (hrScopeOf(tab)) {
    // 第 52 轮：离职管理 / 劳动合同的列全集改由字段契约派生，
    // 与「设置 → 字段设置」的 subs 同源（六同步），不再维护第二份硬编码列。
    case 'hr-resign': return hrResignColumns().filter((c) => c.colKey !== 'row-select')
    case 'hr-contract': return hrContractColumns().filter((c) => c.colKey !== 'row-select')
    case 'hr-social': return SOCIAL_COLS
    case 'hr-fund': return FUND_LEDGER_COLS
    default: return ROSTER_COLS
  }
}

/** 社保双视图列集合（原始 / 汇总，第 47 轮遗留，仅用于旧回归测试）。 */
export function hrSocialColumns(view: 'raw' | 'summary'): HrColumn[] {
  return [SELECT_COL, ...(view === 'raw' ? SI_RAW_COLS : SI_SUMMARY_COLS)]
}

/**
 * 第 56 轮：社保四标签各自的列全集，严格由四胶囊字段契约派生（六同步）。
 *
 * 口径铁律：
 *   · 个人编号 / 身份证号 / 个人社保编号 —— 纯文本，绝不加千分位、绝不进合计行；
 *   · 五险参保状态 / 账单状态 —— 只读彩色 Tag（具名插槽）；
 *   · 缴费比例 —— 文本百分比（如 16%），不参与合计行求和；
 *   · 其余金额列 —— grouped 格式化 + 合计行按分累加。
 */
export function hrSocialColumnsOf(view: string): HrColumn[] {
  const fields = view === 'receipt' ? HR_SOCI_RECEIPT_FIELDS
    : view === 'normal' ? HR_SOCI_NORMAL_FIELDS
      : view === 'abnormal' ? HR_SOCI_ABNORMAL_FIELDS
        : HR_SOCI_BILL_FIELDS
  const cols = columnsOfFields(fields)
  return cols.map((c) => {
    // 人数是整数计数，不能走金额格式化出口变成「75.00」。
    if (c.colKey === '参保人数') {
      return {
        ...c,
        value: (row: any) => {
          const v = pickRaw(row, ['参保人数', 'headcount', 'record_count', '人数'])
          if (v === undefined || v === null || v === '') return ''
          const n = Number(String(v).replace(/[,，\s]/g, ''))
          return Number.isFinite(n) ? String(Math.round(n)) : String(v)
        },
      }
    }
    if (/参保状态|账单状态/.test(c.colKey)) return { ...c, slot: true }
    if (/比例/.test(c.colKey)) return { ...c, noSum: true }
    return c
  })
}

/**
 * 列定义：一律**不带 fixed**、**不带操作列**；第一列是勾选列但不是 fixed。
 * 单条编辑/删除走「点行 → 详情抽屉」，不做右侧冻结操作列（第 31 轮中台口径）。
 */
export function hrColumnDefs(tab: string): HrColumn[] {
  return [SELECT_COL, ...colsOf(tab)]
}

/** 公积金「月度账单」子视图列（第 55 轮：8 个原生字段，严格对齐设置分组）。 */
export function hrBillColumnDefs(): HrColumn[] {
  return [SELECT_COL,
    col('账单周期', ['账单周期', 'month_label', 'month']),
    col('汇缴人数', ['汇缴人数', 'record_count', 'headcount'], { dataType: 'number' }),
    col('缴存基数合计(元)', ['缴存基数合计(元)', 'base_amount_total'], { dataType: 'number', money: true }),
    col('单位缴存额(元)', ['单位缴存额(元)', 'company_amount_total'], { dataType: 'number', money: true }),
    col('个人缴存额(元)', ['个人缴存额(元)', 'personal_amount_total'], { dataType: 'number', money: true }),
    col('月缴存总额(元)', ['月缴存总额(元)', 'combined_amount_total'], { dataType: 'number', money: true }),
    col('账单状态', ['账单状态', 'bill_status', 'status'], { slot: true }),
    col('生成时间', ['生成时间', 'created_at', 'create_time'], { dataType: 'date' }),
  ]
}

// ---------------------------------------------------------------------------
// 花名册 76 字段模板（第 46 轮）与四态动态列
// ---------------------------------------------------------------------------

/**
 * 76 表头 → 后端主列英文键。后端把 76 个字段全量写进 `custom_data`（中文键），
 * `pickRaw` 已能命中；这里额外补主列别名，兼容第 45 轮存量数据（只有主列、无 custom_data）。
 */
const ROSTER_KEY_ALIASES: Record<string, string[]> = {
  '工号': ['employee_id'], '姓名': ['name'], '部门': ['department'],
  '岗位': ['position'], '职务': ['position'], '性别': ['gender'],
  '年龄': ['age'], '司龄': ['work_years', '司龄'], '出生日期': ['birth_date', 'birth_month'],
  '入职日期': ['hire_date'], '离职日期': ['resign_date'],
  '离职原因': ['resign_reasons'], '离职证明': ['resign_proof_path'],
  '离职备注': ['resign_remark'], '手机号码': ['phone'],
  '证件号码': ['id_number'], '证件类型': ['id_type'],
  '员工状态': ['status', 'employ_status'], '在职状态': ['员工状态', 'status', 'employ_status'],
  '个人社保账号': ['social_insurance_number'], '个人公积金账号': ['provident_fund_number'],
  '紧急联系人姓名': ['emergency_contact'], '紧急联系人电话': ['emergency_phone'],
  '最高学历': ['education'], '政治面貌': ['political_status'],
  '籍贯': ['native_place'], '民族': ['ethnicity'],
  '户口性质': ['household_type'], '户籍地址': ['id_address'],
  '个人邮箱': ['email'], '备注': ['remarks'], '档案编号': ['archive_no'],
  // 第 52 轮：劳动合同 / 离职管理新字段名 → 存量 custom_data 键（读取双向兼容）
  '合同编号': ['现合同编号', 'contract_no'],
  '合同类型': ['现合同类型', 'contract_type'],
  '合同开始日期': ['现合同开始日期', 'contract_start_date'],
  '合同结束日期': ['现合同结束日期', 'contract_end_date'],
  '合同期限': ['现合同期限', 'contract_term'],
  '签约公司': ['现合同签约公司', '合同签约公司', 'contract_company'],
  '签订日期': ['现合同签订日期', 'contract_sign_date'],
  '联系电话': ['手机号码', 'phone'],
  '统一社会信用代码': ['credit_code'], '住所': ['address'], '法定代表人': ['legal_person'],
  '离职申请日期': ['resign_apply_date'], '离职类型': ['resign_type'],
  '离职原因说明': ['离职备注', 'resign_remark'], '离职去向': ['resign_destination'],
  '是否试用期内离职': ['probation_resign'], '薪资结算日': ['salary_settlement_date'],
}

/**
 * 号码 / 编码类字段（第 47 轮铁律）：一律纯字符串。
 *
 * 这些值**绝不允许**走数字千分位格式化，也**绝不参与合计行求和** ——
 * 工资卡卡号 6217003760153xxxxx 一旦被 Number() 化，既会被加逗号、
 * 又会因超过 16 位有效数字丢精度（末位变 000），合计行更是会算出
 * 118,179,260,221,922,000 这种荒谬数字。
 */
export const ROSTER_TEXT_CODE_FIELDS: readonly string[] = [
  '工资卡卡号', '个人社保账号', '个人公积金账号', '证件号码', '手机号码', '工号',
  '考勤编号', '档案编号', '现合同编号', 'QQ号', '分机号', '紧急联系人电话',
  '工作电话', '职级编号', '个人账号', '社保编号', '公积金编号', '银行名称',
]

const CODE_FIELD_RE = /卡号|账号|帐号|证件号|手机号|工号|编号|QQ号|分机号|电话/

/** 该列是否为号码/编码类（必须按字符串渲染且不参与合计）。 */
export function isRosterCodeField(name: unknown): boolean {
  const n = String(name ?? '').trim()
  if (!n) return false
  if (ROSTER_TEXT_CODE_FIELDS.includes(n)) return true
  return CODE_FIELD_RE.test(n)
}

/** 单条花名册字段 → 列定义；列宽恒为 0（自适应），无 fixed、无操作列。 */
function rosterColumnOf(name: string): HrColumn {
  const keys = [name, ...(ROSTER_KEY_ALIASES[name] || [])]
  const isStatus = STATUS_COLS.has(name)
  const isDate = /日期|时间|有效期/.test(name)
  // 「离职证明」需要可点击的预览/下载链接，交给页面具名插槽渲染。
  const isProof = name === '离职证明'
  // 号码/编码类优先判字符串，避免被下面的金额规则吞掉（第 47 轮修复）。
  const isCode = isRosterCodeField(name)
  const isMoney = !isCode && /金额|基数/.test(name)
  return {
    colKey: name,
    title: name,
    width: 0,
    dataType: isDate ? 'date' : isMoney ? 'number' : 'text',
    default: true,
    slot: isStatus || isProof,
    value: isStatus
      ? (row: any) => normalizeStatus(pick(row, keys))
      : isDate
        ? (row: any) => formatCellDateTime(name, pickRaw(row, keys)) || ''
        : isCode
          // 原样字符串，不做任何数字处理（含前导零与超长数字都不丢）
          ? (row: any) => {
            const v = pickRaw(row, keys)
            // 第 49 轮：人事列表空值一律留白（不渲染横线 —）
  return v === undefined ? '' : String(v)
          }
          : isMoney
            ? (row: any) => money(row, keys)
            : (row: any) => pick(row, keys),
  }
}

/**
 * 按「在职状态」返回该视图的默认列集合（任务 §3.2 四态动态表头）。
 * 第一列仍是勾选列；用户仍可通过「字段 / 设置」对显隐做微调。
 */
export function hrRosterStatusColumns(view: RosterStatusView): HrColumn[] {
  const key = view === '全部' ? '在职' : view
  return [SELECT_COL, ...ROSTER_STATUS_COLUMNS[key].map(rosterColumnOf)]
}

/** 76 标准表头 + 派生字段 + 离职扩展字段的全集（字段按钮 / 设置抽屉用）。 */
export function hrRosterAllColumns(): HrColumn[] {
  const names = [...ROSTER_TEMPLATE_HEADERS, ...ROSTER_DERIVED_FIELDS, ...EXTRA_RESIGN_FIELDS]
  const seen = new Set<string>()
  const cols: HrColumn[] = [SELECT_COL]
  for (const name of names) {
    if (seen.has(name)) continue
    seen.add(name)
    cols.push(rosterColumnOf(name))
  }
  return cols
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

// ---------------------------------------------------------------------------
// 合同状态三态判定（第 47 轮）
// ---------------------------------------------------------------------------

export type ContractStatus = '正常' | '未签订' | '即将到期' | '到期' | '已解除' | ''

export const CONTRACT_STATUS_OPTIONS = [
  { label: '全部状态', value: '全部状态' },
  { label: '正常', value: '正常' },
  { label: '未签订', value: '未签订' },
  { label: '即将到期', value: '即将到期' },
  { label: '到期', value: '到期' },
  { label: '已解除', value: '已解除' },
] as const

/**
 * 状态主题：正常=success(绿) / 未签订=warning(橙) / 即将到期=warning(橙) /
 * 到期=danger(红) / 已解除=default(灰)。
 */
export function contractStatusTheme(v: unknown): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '').trim()
  if (s === '正常') return 'success'
  if (s === '未签订') return 'warning'
  if (s === '即将到期') return 'warning'
  if (s === '到期') return 'danger'
  return 'default'
}

/**
 * 合同状态判定（与后端 hrContractStatusOf 同口径）：
 *   · 现合同类型含「无固定期限」→ 正常
 *   · 剩余天数 > 30 → 正常；0~30 → 即将到期；< 0 → 到期
 *   · 未填结束日期且非无固定期限 → 未签订
 *
 * 「已解除」依赖员工的在职状态，由后端 hrEmployeeContractStatus 前置判定。
 */
export function contractStatusOf(endDate: unknown, contractType: unknown, today = new Date()): ContractStatus {
  if (String(contractType ?? '').includes('无固定期限')) return '正常'
  const raw = String(endDate ?? '').trim()
  if (!raw) return '未签订'
  const m = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(raw)
  if (!m) return '未签订'
  const end = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate())
  const days = Math.floor((end - now) / 86400000)
  if (days < 0) return '到期'
  if (days <= 30) return '即将到期'
  return '正常'
}

// ---------------------------------------------------------------------------
// 工号自动编码（第 48 轮）—— 与后端 hr_employee_id.go 同口径
// ---------------------------------------------------------------------------

/** 工号结构：[1位职级] + [5位部门码] + [4位序号]。 */
export type HrRank = 'A' | 'B' | 'C'

const RANK_A_KEYWORDS = ['总经理', '副总经理', '总裁', '董事长']
const RANK_B_KEYWORDS = ['主管', '经理', '主任', '部长', '科长', '组长']
const EMPLOYEE_ID_DEPT_RE = /^[A-Za-z](\d{5})\d{4}$/
const EMPLOYEE_ID_TAIL_RE = /(\d{4})$/

/** 按职务判定职级前缀。 */
export function hrRankOfTitle(title: unknown): HrRank {
  const t = String(title ?? '').trim()
  if (RANK_A_KEYWORDS.some((k) => t.includes(k))) return 'A'
  if (RANK_B_KEYWORDS.some((k) => t.includes(k))) return 'B'
  return 'C'
}

/** 从存量工号提取 5 位部门码。 */
export function hrDeptCodeFromEmployeeID(employeeID: unknown): string {
  const m = EMPLOYEE_ID_DEPT_RE.exec(String(employeeID ?? '').trim())
  return m ? m[1] : ''
}

/** 取工号末尾 4 位序号。 */
export function hrSeqFromEmployeeID(employeeID: unknown): number {
  const m = EMPLOYEE_ID_TAIL_RE.exec(String(employeeID ?? '').trim())
  if (!m) return 0
  const n = Number(m[1])
  return Number.isFinite(n) ? n : 0
}

/**
 * 计算下一个工号：扫描同部门码的存量工号取最大序号 +1。
 * 存量工号本身**绝不改写**，这里只产出建议值。
 */
export function hrNextEmployeeID(deptCode: unknown, title: unknown, existing: readonly string[]): string {
  const rank = hrRankOfTitle(title)
  let code = String(deptCode ?? '').trim()
  if (!/^\d{5}$/.test(code)) {
    // 兜底：取存量里出现最多的 5 位码
    const counts = new Map<string, number>()
    for (const id of existing) {
      const c = hrDeptCodeFromEmployeeID(id)
      if (c) counts.set(c, (counts.get(c) || 0) + 1)
    }
    let best = ''
    let bestN = 0
    for (const [c, n] of counts) {
      if (n > bestN || (n === bestN && c < best)) { best = c; bestN = n }
    }
    code = best || '02000'
  }
  let maxSeq = 0
  for (const id of existing) {
    if (hrDeptCodeFromEmployeeID(id) !== code) continue
    const seq = hrSeqFromEmployeeID(id)
    if (seq > maxSeq) maxSeq = seq
  }
  return `${rank}${code}${String(maxSeq + 1).padStart(4, '0')}`
}

// ---------------------------------------------------------------------------
// 11 分组字段全集列定义（第 48 轮）
// ---------------------------------------------------------------------------

/**
 * 按 11 分组契约产出花名册全部列（不区分显隐）。
 * 显隐由页面的 `visibleColKeys`（字段设置保存值）过滤，实现六同步。
 */
export function hrRosterGroupColumns(): HrColumn[] {
  const names = allRosterFields()
  const cols: HrColumn[] = [SELECT_COL]
  for (const name of names) cols.push(rosterColumnOf(name))
  return cols
}

// ---------------------------------------------------------------------------
// 离职管理 / 劳动合同：字段契约派生列（第 52 轮）
// ---------------------------------------------------------------------------

/**
 * 模块字段 → 列定义。引用字段（source）按**来源字段名**取值，
 * 显示名仍用本模块字段名（如「联系电话」取花名册「手机号码」的值）。
 */
function moduleColumnOf(d: HrFieldDef, configuredWidth = 0): HrColumn {
  const name = String(d.label || d.name)
  // 引用字段的取值：来源字段优先，再回退本表字段名。
  // 例：花名册「个人公积金账号」来源是公积金管理的「职工账号」，但后端派生值
  // 写在 custom_data["个人公积金账号"]，只查来源键会整列空白。
  const keys = d.source ? [...hrValueKeys(d.source.field), ...hrValueKeys(d.name)] : hrValueKeys(d.name)
  const isDate = d.type === 'date' || hrIsDateField(d.name)
  const isProof = d.name === '离职证明'
  const isStatusTag = d.name === '合同状态'
  const isCode = isRosterCodeField(d.name)
  const isMoney = !isCode && d.type === 'number'
  const width = Number(configuredWidth) > 0 ? Math.round(Number(configuredWidth)) : 0
  return {
    colKey: name,
    title: name,
    // 第 53 轮：字段设置里配置的列宽必须进入列定义，否则 HrManagement 的
    // colWidthOf(c.width) 永远拿到 0，用户填 250 保存后列表纹丝不动。
    width,
    ...(width > 0 ? { minWidth: width } : {}),
    dataType: isDate ? 'date' : isMoney ? 'number' : 'text',
    default: true,
    slot: isProof || isStatusTag,
    value: isDate
      ? (row: any) => formatCellDateTime(name, pickRaw(row, keys)) || ''
      : isCode
        ? (row: any) => {
          const v = pickRaw(row, keys)
          return v === undefined ? '' : String(v)
        }
        : isMoney
          ? (row: any) => money(row, keys)
          : (row: any) => pick(row, keys),
  }
}

function columnsOfFields(fields: readonly HrFieldDef[]): HrColumn[] {
  const cols: HrColumn[] = [SELECT_COL]
  const seen = new Set<string>(['row-select'])
  for (const d of fields) {
    const name = String(d.label || d.name)
    if (seen.has(name)) continue
    seen.add(name)
    cols.push(moduleColumnOf(d))
  }
  return cols
}

/** 离职管理列全集：5 个花名册引用 + 9 个离职字段。 */
export function hrResignColumns(): HrColumn[] {
  return columnsOfFields(HR_RESIGN_FIELDS)
}

/** 劳动合同列全集：乙方 6 + 甲方 3 + 合同 8。 */
export function hrContractColumns(): HrColumn[] {
  return columnsOfFields(HR_CONTRACT_FIELDS)
}

/**
 * 公积金列全集（第 54 轮）：18 个网厅原生字段。
 *
 * 口径铁律：
 *   · 职工账号 / 联名卡号 / 绑定手机 / 证件号码 —— 纯文本，绝不加千分位、绝不累加；
 *   · 缴存基数 / 月缴存额 / 单位 / 个人 / 账户余额 —— 金额（2 位小数、可累加）；
 *   · 单位缴存比例 / 职工缴存比例 —— 百分比，右侧对齐但不进合计行；
 *   · 账户状态 —— 三色 Tag（正常绿 / 封存橙 / 提取销户灰）。
 */
export function hrFundColumns(): HrColumn[] {
  const cols = columnsOfFields(HR_FUND_FIELDS)
  return cols.map((c) => {
    if (c.colKey === '账户状态') return { ...c, slot: true }
    if (/比例/.test(c.colKey)) return { ...c, noSum: true }
    return c
  })
}

/**
 * 六同步列全集：以「设置 → 字段设置」保存的 subs 为唯一权威。
 *
 * 引用字段（`subs[].source.field`）按来源字段取值，但表头显示本模块字段名，
 * 这正是离职管理 / 劳动合同「搜索引用花名册字段」后能立刻在列表看到数据的关键。
 * subs 为空（接口未就绪）时回退到字段契约派生的静态列，保证首帧不空表。
 */
export function hrColumnsFromSubs(subs: readonly any[] | null | undefined, fallback: HrColumn[]): HrColumn[] {
  const list = Array.isArray(subs) ? subs : []
  if (!list.length) return fallback
  const cols: HrColumn[] = [SELECT_COL]
  const seen = new Set<string>(['row-select'])
  for (const s of list) {
    if (!s || s.enabled === false) continue
    const label = String(s.label || s.name || '').trim()
    if (!label || seen.has(label)) continue
    seen.add(label)
    const sourceField = String(s?.source?.field || '').trim()
    const def: HrFieldDef = {
      name: label,
      type: s.data_type === 'date' ? 'date' : s.data_type === 'number' ? 'number' : 'text',
      ...(sourceField ? { source: { category: String(s?.source?.category || '花名册'), field: sourceField } } : {}),
    }
    // 第 53 轮：subs.width 必须透传给列定义（0 = 自适应，>0 = 用户固定列宽）。
    cols.push(moduleColumnOf(def, Number(s.width) || 0))
  }
  return cols.length > 1 ? cols : fallback
}

/** 模块 → 抽屉只读派生字段（身份证联动 / 系统状态）。 */
export function hrDerivedFieldNames(tab: string): ReadonlySet<string> {
  if (hrScopeOf(tab) === 'hr-roster') {
    return new Set(['年龄', '出生日期', '性别', '司龄'])
  }
  return new Set(['合同状态'])
}
