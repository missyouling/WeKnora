/**
 * 花名册 11 大分组字段矩阵（第 48 轮）。
 *
 * 唯一数据源：字段设置面板、列表表头、`[☰ 字段]` 筛选器、编辑抽屉、
 * 导出/打印清单全部从这里派生 —— 改一处即六同步。
 *
 * `checked` 是**默认显示**状态（对应设计稿的 ☑ / ☐），用户在设置里改动后
 * 存 localStorage，不回写本文件。
 */

export interface RosterFieldDef {
  /** 字段名（同时是 t-table 的 colKey） */
  name: string
  /** 默认是否勾选显示 */
  checked: boolean
  /** 系统字段：不可取消勾选（姓名必选） */
  locked?: boolean
}

export interface RosterFieldGroupDef {
  key: string
  label: string
  fields: RosterFieldDef[]
}

/** 离职信息分组的 key —— 在职视图要隐藏整组。 */
export const LEAVING_GROUP_KEY = '离职信息'

/** 三个筛选态（第 48 轮精简为 全部 / 在职 / 离职）。 */
export type RosterStatusFilter = '全部' | '在职' | '离职'

export const ROSTER_STATUS_OPTIONS: readonly { label: string; value: RosterStatusFilter }[] = [
  { label: '全部', value: '全部' },
  { label: '在职', value: '在职' },
  { label: '离职', value: '离职' },
]

const f = (name: string, checked: boolean, locked = false): RosterFieldDef => ({ name, checked, locked })

/** 11 大分组，顺序与默认勾选严格按设计稿。 */
export const ROSTER_FIELD_GROUPS: readonly RosterFieldGroupDef[] = [
  {
    key: '基本信息',
    label: '基本信息',
    fields: [
      f('姓名', true, true),
      f('证件类型', true), f('证件号码', true), f('身份证有效期', true),
      f('性别', true), f('生日', true), f('出生日期', true), f('年龄', true),
      f('是否已婚', true), f('是否已育', true), f('民族', true), f('政治面貌', true),
      f('籍贯', true), f('户口性质', true), f('户籍地址', true), f('参加工作时间', true),
      f('工龄', true), f('血型', true), f('服装尺寸', true), f('健康状况', true),
      f('健康证有效期', true),
      f('曾用名', false), f('英文名', false), f('是否农历生日', false), f('国家地区', false),
      f('籍贯(省)', false), f('籍贯(市)', false), f('户籍城市', false),
      f('户籍城市(省)', false), f('户籍城市(市)', false),
    ],
  },
  {
    key: '在职信息',
    label: '在职信息',
    fields: [
      f('工号', true), f('部门', true), f('职务', true), f('岗位', true), f('合同公司', true),
      f('招聘渠道', true), f('员工状态', true), f('入职日期', true), f('试用期', true),
      f('转正日期', true), f('司龄开始日期', true), f('司龄', true),
      f('多级部门名称', false), f('岗位类别', false), f('汇报上级', false), f('职等', false),
      f('职级编号', false), f('职级名称', false), f('工作城市', false), f('工作地点', false),
      f('考勤编号', false), f('工时制度', false), f('工作性质', false), f('司龄(天数)', false),
      f('档案编号', false), f('档案备注', false), f('OA编码', false),
    ],
  },
  {
    key: LEAVING_GROUP_KEY,
    label: '离职信息',
    fields: [
      f('离职日期', true), f('离职申请日期', true), f('薪资结算日', true), f('离职类型', true),
      f('离职原因', true), f('离职原因说明', true), f('离职备注', true), f('离职去向', true),
      f('是否试用期内离职', true), f('离职证明', true),
    ],
  },
  {
    key: '联系信息',
    label: '联系信息',
    fields: [
      f('手机号码', true), f('现居住地', true),
      f('个人邮箱', false), f('工作邮箱', false), f('工作电话', false), f('分机号', false),
      f('QQ号', false), f('微信号', false), f('企业微信账号', false),
    ],
  },
  {
    key: '紧急联系人情况',
    label: '紧急联系人情况',
    fields: [
      f('紧急联系人姓名', true), f('紧急联系人电话', true),
      f('紧急联系人关系', false), f('紧急联系人地址', false),
    ],
  },
  {
    key: '家庭情况',
    label: '家庭情况',
    fields: [
      f('婚姻状况', true),
      f('结婚时间', false), f('子女数', false), f('配偶姓名', false),
      f('配偶户籍城市', false), f('配偶文化程度', false),
    ],
  },
  {
    key: '教育情况',
    label: '教育情况',
    fields: [
      f('最高学历', true),
      f('毕业院校', false), f('毕业专业', false), f('教育开始时间', false),
      f('毕业时间', false), f('学习形式', false), f('学位类型', false),
    ],
  },
  {
    key: '工作经历情况',
    label: '工作经历情况',
    fields: [f('上家工作单位', true), f('上家工作职务', true)],
  },
  {
    key: '合同签订情况',
    label: '合同签订情况',
    fields: [
      f('现合同编号', true), f('现合同类型', true), f('现合同开始日期', true),
      f('现合同结束日期', true), f('现合同期限', true), f('现合同签约公司', true),
      f('现合同签订日期', true), f('合同签订次数', true), f('合同状态', true),
    ],
  },
  {
    key: '工资社保信息',
    label: '工资社保信息',
    fields: [
      f('工资卡卡号', true), f('工资卡开户行', true), f('个人社保账号', true), f('个人公积金账号', true),
      f('工资卡开户城市', false), f('银行名称', false),
      f('是否本地首次缴纳社保', false), f('是否本地首次缴纳公积金', false),
    ],
  },
  {
    key: '个税申报信息',
    label: '个税申报信息',
    fields: [
      f('是否烈属', false), f('烈属证号', false), f('是否孤老', false), f('是否特定行业', false),
      f('是否股东/投资者', false), f('个人股本(投资)额', false),
    ],
  },
]

/** 全部字段名（按分组顺序，已去重）。 */
export function allRosterFields(): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    for (const fd of g.fields) {
      if (seen.has(fd.name)) continue
      seen.add(fd.name)
      out.push(fd.name)
    }
  }
  return out
}

/** 默认可见字段（按分组顺序）。 */
export function defaultVisibleFields(): string[] {
  const out: string[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    for (const fd of g.fields) {
      if (fd.checked) out.push(fd.name)
    }
  }
  return out
}

/** 必选字段（不可取消勾选）。 */
export function lockedRosterFields(): string[] {
  const out: string[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    for (const fd of g.fields) {
      if (fd.locked) out.push(fd.name)
    }
  }
  return out
}

/**
 * 按筛选态返回可见字段：
 *   · 在职：排除「离职信息」整组
 *   · 离职 / 全部：包含离职信息
 * `overrides` 是用户在字段设置里保存的显隐（未提供的字段回落到默认值）。
 */
export function visibleFieldsFor(
  status: RosterStatusFilter,
  overrides?: ReadonlySet<string> | null,
): string[] {
  const hideLeaving = status === '在职'
  const out: string[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    if (hideLeaving && g.key === LEAVING_GROUP_KEY) continue
    for (const fd of g.fields) {
      const visible = overrides ? (fd.locked || overrides.has(fd.name)) : fd.checked
      if (visible) out.push(fd.name)
    }
  }
  return out
}

/** 抽屉分组：按 11 组切分已启用字段，空组自动丢弃；在职隐藏离职信息。 */
export function drawerGroupsFor(
  status: RosterStatusFilter,
  overrides?: ReadonlySet<string> | null,
): { key: string; label: string; fields: string[] }[] {
  const hideLeaving = status === '在职'
  const out: { key: string; label: string; fields: string[] }[] = []
  for (const g of ROSTER_FIELD_GROUPS) {
    if (hideLeaving && g.key === LEAVING_GROUP_KEY) continue
    const fields: string[] = []
    for (const fd of g.fields) {
      const visible = overrides ? (fd.locked || overrides.has(fd.name)) : fd.checked
      if (visible) fields.push(fd.name)
    }
    if (fields.length) out.push({ key: g.key, label: g.label, fields })
  }
  return out
}

/** 字段 → 所属分组 label（抽屉/表头 tooltip 用）。 */
export function groupLabelOf(field: string): string {
  for (const g of ROSTER_FIELD_GROUPS) {
    if (g.fields.some((fd) => fd.name === field)) return g.label
  }
  return ''
}

/** 分组全选/半选状态（字段设置面板左侧复选框）。 */
export function groupCheckState(
  group: RosterFieldGroupDef,
  visible: ReadonlySet<string>,
): 'all' | 'none' | 'partial' {
  const total = group.fields.length
  const on = group.fields.filter((fd) => fd.locked || visible.has(fd.name)).length
  if (on === 0) return 'none'
  if (on === total) return 'all'
  return 'partial'
}
