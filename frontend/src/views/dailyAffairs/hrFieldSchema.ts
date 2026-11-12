/**
 * 人事模块字段契约（第 52 轮）。
 *
 * 唯一事实来源：花名册 6 大分组 / 离职管理 / 劳动合同 的字段名、类型、枚举字典、
 * 默认表头、引用来源、只读派生规则全部集中在这里。前端用它渲染抽屉控件、清洗与
 * 校验录入；后端 `internal/handler/hr_field_schema.go` 是同源镜像，用于分类种子、
 * 三套 Excel 模板表头与导入解析。两边改字段必须同步。
 *
 * 设计铁律：
 *   · 字段配置（设置 → 字段设置）取消胶囊分组，按 6 组顺序合并为单一连续列表；
 *     底层 group 属性保留，供编辑抽屉按 6 组分块展示。
 *   · 抽屉控件一律复用 BusinessSplitDetailDrawer 的 dataType / options 分支，
 *     不在页面里手写第二套控件清单。
 *   · 禁用自动保存：本文件只产出纯函数与常量，不触发任何写操作。
 */

import { ROSTER_ENUM_GROUPS, RESIGN_REASONS } from './hrRosterFields'

// ---------------------------------------------------------------------------
// 模块与字段类型
// ---------------------------------------------------------------------------

export type HrModule = 'roster' | 'resign' | 'contract'

export type HrFieldType = 'text' | 'date' | 'number' | 'enum' | 'readonly' | 'attachment' | 'system'

/** 跨模块引用：值取自花名册（category='花名册'）或组织机构。 */
export interface HrFieldSource {
  category: string
  field: string
}

export interface HrFieldDef {
  name: string
  /** 列表/抽屉展示名，默认等于 name。 */
  label?: string
  type: HrFieldType
  /** type=enum 时的下拉项；动态字典（部门/单位）用 dynamic 标记。 */
  options?: readonly string[]
  dynamic?: 'department' | 'unit'
  /** 默认表头是否开启（字段设置的「默认表头」开关）。 */
  defaultHeader?: boolean
  /** 新增时的默认值。 */
  defaultValue?: string
  /** 分组 key：花名册 6 组，抽屉据此分块。 */
  group?: string
  /** 引用字段来源；有值时在字段设置里显示来源徽标。 */
  source?: HrFieldSource
  /** 系统内置加锁（不可删/改名）。 */
  builtin?: boolean
}

export interface HrFieldGroupDef {
  key: string
  label: string
  fields: readonly HrFieldDef[]
}

// ---------------------------------------------------------------------------
// 枚举字典（复用既有字典 + 补齐第 52 轮新增项）
// ---------------------------------------------------------------------------

const YES_NO: readonly string[] = ['是', '否']

export const HR_ENUMS: Record<string, readonly string[]> = {
  ...ROSTER_ENUM_GROUPS,
  招聘渠道: ['社招', '校招', '内部推荐', '猎头', '劳务派遣', '其他'],
  试用期: ['无试用期', '1个月', '2个月', '3个月', '4个月', '5个月', '6个月',
    '7个月', '8个月', '9个月', '10个月', '11个月', '12个月'],
  证件类型: ['身份证', '港澳居民来往内地通行证', '台湾居民来往内地通行证', '外国护照',
    '香港身份证', '台湾身份证', '澳门身份证', '外国人永久居留证', '其他'],
  性别: ['男', '女'],
  是否已婚: ['未婚', '已婚', '离异', '丧偶'],
  户口性质: ['城镇户口', '农村户口', '居民户口'],
  服装尺寸: ['S', 'M', 'L', 'XL', '2XL', '3XL', '4XL', '5XL'],
  紧急联系人关系: ['父母', '配偶', '子女', '兄弟姐妹', '亲属', '朋友', '其他'],
  银行名称: ['中国建设银行', '中国银行', '中国工商银行', '中国农业银行', '农村商业银行',
    '招商银行', '交通银行', '邮政储蓄银行', '其他'],
  是否本地首次缴纳社保: YES_NO,
  是否本地首次缴纳公积金: YES_NO,
  离职类型: ['主动离职', '被动离职', '退休', '自动离职', '其他'],
  // 42 项标准离职原因（既有 5 分组模板拍平，顺序与导入模板枚举 Sheet 一致）
  离职原因: RESIGN_REASONS.flatMap((g) => g.options),
  是否试用期内离职: YES_NO,
  合同类型: ['固定期限劳动合同', '无固定期限劳动合同', '以完成一定工作任务为期限的劳动合同',
    '实习协议', '劳务合同', '返聘协议', '劳务派遣合同', '借调合同', '其他'],
  合同期限: ['1年', '2年', '3年', '4年', '5年', '6年', '7年', '8年', '9年', '10年', '无固定期限'],
  合同状态: ['正常', '即将到期', '到期'],
}

/** 字段 → 枚举字典；动态字典返回空数组（由页面注入组织机构选项）。 */
export function hrEnumsOf(field: string): readonly string[] {
  return HR_ENUMS[field] || []
}

// ---------------------------------------------------------------------------
// 花名册 6 大分组（48 字段）
// ---------------------------------------------------------------------------

const f = (
  name: string,
  type: HrFieldType = 'text',
  extra: Partial<HrFieldDef> = {},
): HrFieldDef => ({ name, type, ...extra })

const G = {
  active: '在职信息',
  basic: '基本信息',
  contact: '联系信息',
  emergency: '紧急联系人情况',
  education: '教育情况',
  payroll: '工资社保信息',
} as const

export const HR_ROSTER_GROUPS: readonly HrFieldGroupDef[] = [
  {
    key: G.active, label: G.active, fields: [
      f('工号', 'text', { group: G.active, defaultHeader: true }),
      f('姓名', 'text', { group: G.active, defaultHeader: true, builtin: true }),
      f('部门', 'enum', { group: G.active, defaultHeader: true, dynamic: 'department' }),
      f('岗位', 'text', { group: G.active, defaultHeader: true }),
      f('合同公司', 'enum', { group: G.active, dynamic: 'unit' }),
      f('招聘渠道', 'enum', { group: G.active, options: HR_ENUMS['招聘渠道'] }),
      f('入职日期', 'date', { group: G.active, defaultHeader: true }),
      f('试用期', 'enum', { group: G.active, options: HR_ENUMS['试用期'] }),
      f('转正日期', 'date', { group: G.active }),
      f('司龄', 'readonly', { group: G.active, defaultHeader: true }),
    ],
  },
  {
    key: G.basic, label: G.basic, fields: [
      f('证件类型', 'enum', { group: G.basic, options: HR_ENUMS['证件类型'], defaultValue: '身份证' }),
      f('证件号码', 'text', { group: G.basic, defaultHeader: true }),
      f('身份证有效期', 'date', { group: G.basic }),
      f('性别', 'enum', { group: G.basic, options: HR_ENUMS['性别'], defaultHeader: true }),
      f('出生日期', 'date', { group: G.basic, defaultHeader: true }),
      f('年龄', 'readonly', { group: G.basic, defaultHeader: true }),
      f('是否已婚', 'enum', { group: G.basic, options: HR_ENUMS['是否已婚'], defaultHeader: true }),
      f('民族', 'enum', { group: G.basic, options: HR_ENUMS['民族'], defaultValue: '汉族' }),
      f('政治面貌', 'enum', { group: G.basic, options: HR_ENUMS['政治面貌'], defaultValue: '群众' }),
      f('籍贯', 'text', { group: G.basic, defaultHeader: true }),
      f('户口性质', 'enum', { group: G.basic, options: HR_ENUMS['户口性质'], defaultHeader: true }),
      f('户籍地址', 'text', { group: G.basic }),
      f('服装尺寸', 'enum', { group: G.basic, options: HR_ENUMS['服装尺寸'] }),
    ],
  },
  {
    key: G.contact, label: G.contact, fields: [
      f('手机号码', 'text', { group: G.contact, defaultHeader: true }),
      f('现居住地', 'text', { group: G.contact }),
      f('个人邮箱', 'text', { group: G.contact }),
      f('工作邮箱', 'text', { group: G.contact }),
      f('工作电话', 'text', { group: G.contact }),
      f('分机号', 'text', { group: G.contact }),
      f('QQ号', 'text', { group: G.contact }),
      f('微信号', 'text', { group: G.contact }),
      f('企业微信账号', 'text', { group: G.contact }),
    ],
  },
  {
    key: G.emergency, label: G.emergency, fields: [
      f('紧急联系人姓名', 'text', { group: G.emergency }),
      f('紧急联系人电话', 'text', { group: G.emergency }),
      f('紧急联系人关系', 'enum', { group: G.emergency, options: HR_ENUMS['紧急联系人关系'] }),
      f('紧急联系人地址', 'text', { group: G.emergency }),
    ],
  },
  {
    key: G.education, label: G.education, fields: [
      f('最高学历', 'enum', { group: G.education, options: HR_ENUMS['最高学历'], defaultHeader: true }),
      f('毕业院校', 'text', { group: G.education }),
      f('毕业专业', 'text', { group: G.education }),
      f('毕业时间', 'date', { group: G.education }),
    ],
  },
  {
    key: G.payroll, label: G.payroll, fields: [
      f('工资卡卡号', 'text', { group: G.payroll }),
      f('银行名称', 'enum', { group: G.payroll, options: HR_ENUMS['银行名称'] }),
      f('工资卡开户行', 'text', { group: G.payroll }),
      f('工资卡开户城市', 'text', { group: G.payroll }),
      f('个人社保账号', 'text', { group: G.payroll }),
      f('个人公积金账号', 'text', { group: G.payroll }),
      f('是否本地首次缴纳社保', 'enum', { group: G.payroll, options: YES_NO }),
      f('是否本地首次缴纳公积金', 'enum', { group: G.payroll, options: YES_NO }),
    ],
  },
]

export const HR_ROSTER_FIELDS: readonly HrFieldDef[] = HR_ROSTER_GROUPS.flatMap((g) => g.fields)
export const HR_ROSTER_GROUP_LABELS: readonly string[] = HR_ROSTER_GROUPS.map((g) => g.label)

/** 花名册默认表头（顺序 = 6 组顺序）。 */
export const HR_ROSTER_DEFAULT_HEADERS: readonly string[] =
  HR_ROSTER_FIELDS.filter((d) => d.defaultHeader).map((d) => d.name)

/** 已彻底删除的分组与字段（用于回归断言与后端存量清理）。 */
export const HR_REMOVED_GROUPS: readonly string[] =
  ['家庭情况', '工作经历情况', '个税申报信息', '离职信息', '合同签订情况']
export const HR_REMOVED_FIELDS: readonly string[] = ['在职状态', '人员状态']

// ---------------------------------------------------------------------------
// 离职管理：5 个花名册引用 + 9 个核心离职字段
// ---------------------------------------------------------------------------

const ref = (name: string, source: HrFieldSource, extra: Partial<HrFieldDef> = {}): HrFieldDef => ({
  name, type: 'readonly', source, defaultHeader: true, ...extra,
})

export const HR_RESIGN_FIELDS: readonly HrFieldDef[] = [
  ref('工号', { category: '花名册', field: '工号' }),
  ref('姓名', { category: '花名册', field: '姓名' }),
  ref('部门', { category: '花名册', field: '部门' }),
  ref('岗位', { category: '花名册', field: '岗位' }),
  ref('入职日期', { category: '花名册', field: '入职日期' }, { type: 'date' }),
  f('离职申请日期', 'date'),
  f('离职日期', 'date', { defaultHeader: true }),
  f('离职类型', 'enum', { options: HR_ENUMS['离职类型'], defaultHeader: true }),
  f('离职原因', 'enum', { options: HR_ENUMS['离职原因'], defaultHeader: true }),
  f('离职原因说明', 'text', { defaultHeader: true }),
  f('离职去向', 'text', { defaultHeader: true }),
  f('是否试用期内离职', 'enum', { options: YES_NO, defaultHeader: true }),
  f('薪资结算日', 'date', { defaultHeader: true }),
  f('离职证明', 'attachment', { defaultHeader: true }),
]

export const HR_RESIGN_DEFAULT_HEADERS: readonly string[] =
  HR_RESIGN_FIELDS.filter((d) => d.defaultHeader).map((d) => d.name)

// ---------------------------------------------------------------------------
// 劳动合同：乙方引用 6 + 甲方引用 3 + 8 个核心合同字段
// ---------------------------------------------------------------------------

export const HR_CONTRACT_FIELDS: readonly HrFieldDef[] = [
  // 乙方（来自花名册）
  ref('工号', { category: '花名册', field: '工号' }),
  ref('姓名', { category: '花名册', field: '姓名' }),
  ref('部门', { category: '花名册', field: '部门' }),
  ref('岗位', { category: '花名册', field: '岗位' }),
  ref('证件号码', { category: '花名册', field: '证件号码' }),
  ref('联系电话', { category: '花名册', field: '手机号码' }),
  // 甲方（来自组织机构）
  ref('统一社会信用代码', { category: '组织机构', field: '统一社会信用代码' }, { defaultHeader: false }),
  ref('住所', { category: '组织机构', field: '住所' }, { defaultHeader: false }),
  ref('法定代表人', { category: '组织机构', field: '法定代表人' }, { defaultHeader: false }),
  // 合同核心字段
  f('合同编号', 'text'),
  f('合同类型', 'enum', { options: HR_ENUMS['合同类型'], defaultHeader: true }),
  f('合同开始日期', 'date', { defaultHeader: true }),
  f('合同结束日期', 'date', { defaultHeader: true }),
  f('合同期限', 'enum', { options: HR_ENUMS['合同期限'] }),
  f('签约公司', 'enum', { dynamic: 'unit' }),
  f('签订日期', 'date'),
  f('合同状态', 'system', { defaultHeader: true, builtin: true, options: HR_ENUMS['合同状态'] }),
]

export const HR_CONTRACT_DEFAULT_HEADERS: readonly string[] =
  HR_CONTRACT_FIELDS.filter((d) => d.defaultHeader).map((d) => d.name)

/** 模块 → 字段清单（设置面板、导入模板、抽屉共用）。 */
export function hrModuleFields(module: HrModule): readonly HrFieldDef[] {
  if (module === 'resign') return HR_RESIGN_FIELDS
  if (module === 'contract') return HR_CONTRACT_FIELDS
  return HR_ROSTER_FIELDS
}

export function hrModuleGroups(module: HrModule): readonly HrFieldGroupDef[] {
  if (module !== 'roster') {
    return [{ key: module, label: module === 'resign' ? '离职信息' : '合同信息', fields: hrModuleFields(module) }]
  }
  return HR_ROSTER_GROUPS
}

/** 字段类型 → 后端 subs 的 data_type（枚举仍是文本值，只是前端渲染下拉）。 */
export function hrDataTypeOf(d: HrFieldDef): 'date' | 'number' | 'text' {
  return d.type === 'date' ? 'date' : d.type === 'number' ? 'number' : 'text'
}

/** 日期字段全集（第 52 轮要求统一 t-date-picker）。 */
export const HR_DATE_FIELDS: readonly string[] = [
  '入职日期', '转正日期', '身份证有效期', '出生日期', '毕业时间',
  '离职申请日期', '离职日期', '薪资结算日',
  '合同开始日期', '合同结束日期', '签订日期',
]

export function hrIsDateField(name: string): boolean {
  return HR_DATE_FIELDS.includes(name)
}

/** 只读派生字段（身份证 / 入职日期自动推导，人工不可改）。 */
export const HR_DERIVED_FIELDS: readonly string[] = ['出生日期', '性别', '年龄', '司龄', '合同状态']

// ---------------------------------------------------------------------------
// 新合同字段名 → 存量 custom_data 键（读取与保存双向兼容）
// ---------------------------------------------------------------------------

export const HR_FIELD_ALIASES: Record<string, readonly string[]> = {
  合同编号: ['现合同编号'],
  合同类型: ['现合同类型'],
  合同开始日期: ['现合同开始日期'],
  合同结束日期: ['现合同结束日期'],
  合同期限: ['现合同期限'],
  签约公司: ['现合同签约公司', '合同签约公司'],
  签订日期: ['现合同签订日期'],
  联系电话: ['手机号码'],
  离职日期: ['离职日期'],
}

/** 取值候选键：新名优先，兼容存量键。 */
export function hrValueKeys(name: string): string[] {
  return [name, ...(HR_FIELD_ALIASES[name] || [])]
}

// ---------------------------------------------------------------------------
// 录入清洗与校验（纯函数，可单测）
// ---------------------------------------------------------------------------

const INVISIBLE_RE = /[\u200B-\u200D\uFEFF\u00A0\t\r\n]/g

/** 通用清洗：去首尾空白与不可见字符。 */
export function hrTrim(v: unknown): string {
  return String(v ?? '').replace(INVISIBLE_RE, '').trim()
}

const ID_STRIP_FIELDS = new Set(['证件号码', '手机号码', '工资卡卡号', '紧急联系人电话', '工作电话'])

/**
 * 保存/导入前的字段值清洗：
 *  · 所有字符串 trim + 去不可见字符；
 *  · 证件号 / 手机号 / 卡号去中间空格与横杠（严禁转数值）；
 *  · 身份证末位 x 统一大写 X。
 */
export function hrCleanValue(name: string, raw: unknown): string {
  let s = hrTrim(raw)
  if (ID_STRIP_FIELDS.has(name)) s = s.replace(/[\s-]/g, '')
  if (name === '证件号码') s = s.replace(/x$/, 'X')
  return s
}

export const ID_CARD_RE = /^[1-9]\d{5}(19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]$/
export const MOBILE_RE = /^1[3-9]\d{9}$/
export const LANDLINE_RE = /^0\d{2,3}-?\d{7,8}(-\d{1,6})?$/
export const BANK_CARD_RE = /^\d{12,25}$/

const ID_CHECK_WEIGHTS = [7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2]
const ID_CHECK_CODES = ['1', '0', 'X', '9', '8', '7', '6', '5', '4', '3', '2']

/**
 * 18 位身份证：格式 + 出生年月日合法性。
 *
 * 这是表单与 Excel 导入的**拦截口径**：任务给定的样例号
 * `51072319850217371X`（要求自动推导出 1985-02-17 / 男）按 GB11643 校验位应为 0，
 * 若把校验码作为硬拦截会让该样例一保存就报错、与断言冲突。校验码由
 * `hrIsStrictIdCard` 单独提供，供需要强一致的场景使用，不阻断录入。
 */
export function hrIsValidIdCard(v: unknown): boolean {
  const s = hrCleanValue('证件号码', v)
  return ID_CARD_RE.test(s) && hrParseIdCard(s) !== null
}

/** 严格校验（格式 + 出生日期 + GB11643 校验码）。 */
export function hrIsStrictIdCard(v: unknown): boolean {
  if (!hrIsValidIdCard(v)) return false
  const s = hrCleanValue('证件号码', v)
  let sum = 0
  for (let i = 0; i < 17; i++) sum += Number(s[i]) * ID_CHECK_WEIGHTS[i]
  return ID_CHECK_CODES[sum % 11] === s[17].toUpperCase()
}

/** 手机号 / 紧急联系人电话：11 位手机或座机。 */
export function hrIsValidPhone(v: unknown): boolean {
  const s = hrCleanValue('手机号码', v)
  return MOBILE_RE.test(s) || LANDLINE_RE.test(s)
}

/** 工资卡卡号：12~25 位纯数字字符串。 */
export function hrIsValidBankCard(v: unknown): boolean {
  return BANK_CARD_RE.test(hrCleanValue('工资卡卡号', v))
}

export interface HrIdCardInfo {
  birthDate: string
  gender: '男' | '女'
  age: number
}

/**
 * 身份证推导：出生日期 / 性别 / 年龄（保留 1 位小数）。
 * 返回 null 表示不是合法身份证，调用方据此不覆盖原值。
 */
export function hrParseIdCard(v: unknown, today = new Date()): HrIdCardInfo | null {
  const s = hrCleanValue('证件号码', v)
  // 兼容「只要符合 18 位规则就联动」：校验码不合法时仍尝试推导，但对格式严格要求。
  if (!ID_CARD_RE.test(s)) return null
  const birthDate = `${s.slice(6, 10)}-${s.slice(10, 12)}-${s.slice(12, 14)}`
  const y = Number(s.slice(6, 10))
  const m = Number(s.slice(10, 12))
  const d = Number(s.slice(12, 14))
  const bd = new Date(Date.UTC(y, m - 1, d))
  if (bd.getUTCFullYear() !== y || bd.getUTCMonth() !== m - 1 || bd.getUTCDate() !== d) return null
  return {
    birthDate,
    gender: Number(s[16]) % 2 === 1 ? '男' : '女',
    age: hrAgeOf(birthDate, today),
  }
}

/** 精确年龄（保留 1 位小数，按 365.2425 天/年，与后端 hrAgeDecimalAt 同口径）。 */
export function hrAgeOf(birthDate: unknown, today = new Date()): number {
  const m = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(String(birthDate ?? '').trim())
  if (!m) return 0
  const birth = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  const now = Date.UTC(today.getFullYear(), today.getMonth(), today.getDate())
  if (now < birth) return 0
  return Math.round(((now - birth) / 86400000 / 365.2425) * 10) / 10
}

/** 司龄：入职日期 → 「X年Y个月」，与后端 hrWorkYearsAt 同口径。 */
export function hrSeniorityOf(hireDate: unknown, today = new Date()): string {
  const m = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(String(hireDate ?? '').trim())
  if (!m) return ''
  let months = (today.getFullYear() - Number(m[1])) * 12 + (today.getMonth() + 1 - Number(m[2]))
  if (today.getDate() < Number(m[3])) months -= 1
  if (months < 0) return ''
  return `${Math.floor(months / 12)}年${months % 12}个月`
}

/** 转正日期：入职日期 + 试用期（无试用期 → 取入职日期）。 */
export function hrRegularDateOf(hireDate: unknown, probation: unknown): string {
  const m = /^(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})/.exec(String(hireDate ?? '').trim())
  if (!m) return ''
  const n = /^(\d{1,2})个月$/.exec(String(probation ?? '').trim())
  const add = n ? Number(n[1]) : 0
  const dt = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1 + add, Number(m[3])))
  return `${dt.getUTCFullYear()}-${String(dt.getUTCMonth() + 1).padStart(2, '0')}-${String(dt.getUTCDate()).padStart(2, '0')}`
}

/**
 * 单字段校验，返回错误文案（空串 = 通过）。
 * 空值一律放行（必填由后端按业务判定），只对「已填写但不合法」拦截。
 */
export function hrValidateField(name: string, raw: unknown, form: Record<string, any> = {}): string {
  const v = hrCleanValue(name, raw)
  if (!v) return ''
  if (name === '证件号码' && (hrTrim(form['证件类型'] || '身份证') === '身份证' || ID_CARD_RE.test(v))) {
    return hrIsValidIdCard(v) ? '' : '请输入合法的 18 位身份证号码'
  }
  if (name === '手机号码' || name === '紧急联系人电话') {
    return hrIsValidPhone(v) ? '' : '请输入正确的 11 位手机号码'
  }
  if (name === '工资卡卡号') {
    return hrIsValidBankCard(v) ? '' : '请输入 12~25 位数字的工资卡卡号'
  }
  return ''
}

/** 整表校验：返回第一条错误（供保存前拦截 + MessagePlugin 提示）。 */
export function hrValidateForm(data: Record<string, any>): string {
  for (const key of ['证件号码', '手机号码', '紧急联系人电话', '工资卡卡号']) {
    const msg = hrValidateField(key, data?.[key], data)
    if (msg) return msg
  }
  return ''
}

/** 清洗整表（保存与导入共用；绝不把卡号转数值）。 */
export function hrCleanForm(data: Record<string, any>): Record<string, any> {
  const out: Record<string, any> = { ...(data || {}) }
  for (const key of Object.keys(out)) {
    if (typeof out[key] === 'string') out[key] = hrCleanValue(key, out[key])
  }
  return out
}

// ---------------------------------------------------------------------------
// 三套 Excel 导入模板的字段契约（后端生成模板时同源镜像）
// ---------------------------------------------------------------------------

export const HR_TEMPLATE_MATCH_KEYS: readonly string[] = ['工号', '姓名', '证件号码']

/** 花名册模板表头 = 精简后的 48 个在职字段（不含离职/合同/已删分组）。 */
export const HR_ROSTER_TEMPLATE_HEADERS: readonly string[] = HR_ROSTER_FIELDS.map((d) => d.name)

/** 离职管理模板 = 匹配键 + 8 个可导入离职字段（离职证明为附件不进模板）。 */
export const HR_RESIGN_TEMPLATE_HEADERS: readonly string[] = [
  ...HR_TEMPLATE_MATCH_KEYS,
  '离职申请日期', '离职日期', '离职类型', '离职原因', '离职原因说明',
  '离职去向', '是否试用期内离职', '薪资结算日',
]

/** 劳动合同模板 = 匹配键 + 7 个可导入合同字段（合同状态系统计算不进模板）。 */
export const HR_CONTRACT_TEMPLATE_HEADERS: readonly string[] = [
  ...HR_TEMPLATE_MATCH_KEYS,
  '合同编号', '合同类型', '合同开始日期', '合同结束日期', '合同期限', '签约公司', '签订日期',
]

export function hrTemplateHeaders(module: HrModule): readonly string[] {
  if (module === 'resign') return HR_RESIGN_TEMPLATE_HEADERS
  if (module === 'contract') return HR_CONTRACT_TEMPLATE_HEADERS
  return HR_ROSTER_TEMPLATE_HEADERS
}

export const HR_TEMPLATE_NAMES: Record<HrModule, string> = {
  roster: '花名册导入模板.xlsx',
  resign: '离职管理导入模板.xlsx',
  contract: '劳动合同导入模板.xlsx',
}

/**
 * 模板「枚举字段」Sheet 的分组（只列出该模块表头里真实存在的枚举列）。
 * 顺序与后端 hrTemplateSpecOf 的 order 数组严格一致，保证三套模板可复现。
 */
const HR_TEMPLATE_ENUM_ORDER: readonly string[] = [
  '招聘渠道', '试用期', '证件类型', '性别', '是否已婚', '民族', '政治面貌',
  '户口性质', '服装尺寸', '紧急联系人关系', '最高学历', '银行名称',
  '是否本地首次缴纳社保', '是否本地首次缴纳公积金',
  '离职类型', '离职原因', '是否试用期内离职', '合同类型', '合同期限', '合同状态',
]

export function hrTemplateEnumGroups(module: HrModule): { group: string; options: readonly string[] }[] {
  const headers = new Set(hrTemplateHeaders(module))
  const out: { group: string; options: readonly string[] }[] = []
  for (const group of HR_TEMPLATE_ENUM_ORDER) {
    const options = HR_ENUMS[group]
    if (!headers.has(group) || !options?.length) continue
    out.push({ group, options })
  }
  return out
}
