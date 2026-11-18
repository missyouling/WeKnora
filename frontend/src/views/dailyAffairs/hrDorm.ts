/**
 * 宿舍管理五标签契约（第 62 轮 v2）。
 *
 * 与后端 `internal/handler/hr_dorm_schema.go` 的 hrDormFieldDefs 同源：
 *
 *   宿舍档案 25 / 公租房档案 29 / 住宿记录 27 / 资产档案 16 / 费用信息 20
 *
 * 「费用项目」是编辑抽屉的第二分组（subGroup），不是独立胶囊 ——
 * 设置抽屉保持 5 个胶囊，与页面 5 个标签严格 1:1。
 */

import { hrColumnsFromSubs } from './hrCore'
import type { HrColumn } from './hrCore'

export type HrDormView = 'dorm' | 'public' | 'stay' | 'asset' | 'fee'

export interface HrDormViewDef {
  key: HrDormView
  label: string
  module: 'dorm' | 'public' | 'stay' | 'asset' | 'fee'
}

export const HR_DORM_VIEWS: readonly HrDormViewDef[] = [
  { key: 'dorm', label: '宿舍档案', module: 'dorm' },
  { key: 'public', label: '公租房档案', module: 'public' },
  { key: 'stay', label: '住宿记录', module: 'stay' },
  { key: 'asset', label: '资产档案', module: 'asset' },
  { key: 'fee', label: '费用信息', module: 'fee' },
]

export const HR_DORM_GROUP_OF: Record<HrDormView, string> = {
  dorm: '宿舍档案',
  public: '公租房档案',
  stay: '住宿记录',
  asset: '资产档案',
  fee: '费用信息',
}

export function hrDormModuleOf(view: HrDormView): string {
  return HR_DORM_VIEWS.find((v) => v.key === view)?.module || 'dorm'
}

export interface HrDormField {
  name: string
  type: 'text' | 'number' | 'date' | 'enum' | 'textarea' | 'system' | 'multi'
  /** 编辑抽屉分组：'费用项目' 表示第二分组，缺省为第一分组 */
  subGroup?: string
  options?: string[]
  readonly?: boolean
  width?: number
  /** 整数渲染（床位数 / 可用床位数 / 数量 / 入住天数） */
  integer?: boolean
}

const ROOM_TYPES = ['单人间', '双人间', '夫妻房', '四人间', '六人间', '八人间']
const ROOM_STATUS = ['空闲', '部分入住', '满员', '维修中']
const BEDS = ['1号床', '2号床', '3号床', '4号床', '5号床', '6号床', '7号床', '8号床']
const METERS = [
  '宿舍总表', '宿舍301', '宿舍302', '宿舍303', '宿舍304', '宿舍305', '宿舍306',
  '宿舍307', '宿舍308', '宿舍309', '宿舍310', '宿舍311', '宿舍312', '宿舍313',
  '宿舍314', '宿舍315', '宿舍316', '宿舍317', '宿舍318', '宿舍319', '宿舍320',
  '宿舍417', '宿舍419',
  '两山秀苑19-23-5', '两山秀苑19-23-6', '两山秀苑19-23-7', '两山秀苑19-23-8',
  '两山秀苑19-23-9', '两山秀苑19-23-10', '两山秀苑19-23-11', '两山秀苑19-23-12',
  '两山秀苑19-23-14',
  '两山景苑5-2-6', '两山景苑5-2-7', '两山景苑5-2-8', '两山景苑5-21-15',
  '两山景苑5-21-16', '两山景苑5-24-3', '两山景苑5-24-6', '两山景苑5-24-7',
  '两山景苑5-24-8', '两山景苑5-24-9',
]

/** 费用项目分组（宿舍 / 公租房 / 住宿记录三张档案的第二分组）。 */
const FEE_FIELDS: readonly HrDormField[] = [
  { name: '关联电表', type: 'enum', options: METERS, subGroup: '费用项目' },
  { name: '关联水表', type: 'enum', options: METERS, subGroup: '费用项目' },
  { name: '关联气表', type: 'enum', options: METERS, subGroup: '费用项目' },
  { name: '宿舍押金(元)', type: 'number', subGroup: '费用项目' },
  { name: '电费标准(元/度)', type: 'number', subGroup: '费用项目' },
  { name: '水费单价(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '垃圾处理费(元/套)', type: 'number', subGroup: '费用项目' },
  { name: '二次供水费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '污水处理费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '气费标准(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '房屋租金(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '物业费(元/月)', type: 'number', subGroup: '费用项目' },
]

const PUBLIC_ONLY_PRICING: readonly HrDormField[] = [
  { name: '月费用单价(元/月)', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '季度租金(元/季)', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '租期起始月份', type: 'text', subGroup: '费用项目' },
  { name: '租期截止月份', type: 'text', subGroup: '费用项目' },
]

const STAY_FEE_FIELDS: readonly HrDormField[] = [
  { name: '入住时水表度数', type: 'number', subGroup: '费用项目' },
  { name: '入住时电表度数', type: 'number', subGroup: '费用项目' },
  { name: '入住时气表度数', type: 'number', subGroup: '费用项目' },
  { name: '分摊比例', type: 'number', subGroup: '费用项目' },
  { name: '房屋租金', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '物业费', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '季度租金', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '电费标准', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '水费单价', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '垃圾处理费', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '二次供水费', type: 'number', readonly: true, subGroup: '费用项目' },
  { name: '气费标准', type: 'number', readonly: true, subGroup: '费用项目' },
]

/** 五胶囊字段契约（顺序 = 表头顺序）。 */
export const HR_DORM_FIELDS: Record<HrDormView, readonly HrDormField[]> = {
  dorm: [
    { name: '宿舍编号', type: 'text', width: 110 },
    { name: '宿舍楼/区域', type: 'text', width: 110 },
    { name: '房间号', type: 'text', width: 80 },
    { name: '房型', type: 'enum', options: ROOM_TYPES, width: 90 },
    { name: '床位数', type: 'number', readonly: true, integer: true, width: 80 },
    { name: '可用床位数', type: 'number', readonly: true, integer: true, width: 100 },
    { name: '性别限制', type: 'enum', options: ['男', '女', '不限'], width: 90 },
    { name: '房间面积(㎡)', type: 'number', width: 100 },
    { name: '配套设施', type: 'multi', width: 170 },
    { name: '所属公司', type: 'enum', width: 190 },
    { name: '所属部门', type: 'enum', width: 110 },
    { name: '管理员/舍长', type: 'text', width: 100 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 100 },
    { name: '备注', type: 'textarea', width: 140 },
    ...FEE_FIELDS,
  ],
  public: [
    { name: '房屋编号', type: 'text', width: 110 },
    { name: '房屋地址', type: 'text', width: 190 },
    { name: '租赁单位', type: 'enum', options: ['重庆星达铜业有限公司'], width: 170 },
    { name: '所属公司', type: 'enum', width: 190 },
    { name: '所属部门', type: 'enum', width: 110 },
    { name: '建筑面积(㎡)', type: 'number', width: 100 },
    { name: '房型', type: 'enum', options: ROOM_TYPES, width: 90 },
    { name: '床位数', type: 'number', readonly: true, integer: true, width: 80 },
    { name: '可用床位数', type: 'number', readonly: true, integer: true, width: 100 },
    { name: '性别限制', type: 'enum', options: ['男', '女', '不限'], width: 90 },
    { name: '配套设施', type: 'multi', width: 160 },
    { name: '管理员/舍长', type: 'text', width: 100 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 100 },
    { name: '备注', type: 'textarea', width: 140 },
    ...FEE_FIELDS,
    ...PUBLIC_ONLY_PRICING,
  ],
  stay: [
    { name: '住宿记录ID', type: 'text', readonly: true, width: 110 },
    { name: '员工工号', type: 'text', readonly: true, width: 110 },
    { name: '员工姓名', type: 'text', width: 90 },
    { name: '所属部门', type: 'text', readonly: true, width: 110 },
    { name: '联系电话', type: 'text', readonly: true, width: 120 },
    { name: '紧急联系人及电话', type: 'text', readonly: true, width: 150 },
    { name: '房源类型', type: 'enum', options: ['公司宿舍', '公租房'], width: 100 },
    { name: '宿舍编号/房屋地址', type: 'text', width: 190 },
    { name: '床位号', type: 'enum', options: BEDS, width: 90 },
    { name: '入住类型', type: 'enum', options: ['新入住', '调宿', '续住'], width: 90 },
    { name: '入住日期', type: 'date', width: 110 },
    { name: '退宿日期', type: 'date', width: 110 },
    { name: '押金金额(元)', type: 'number', width: 110 },
    { name: '退宿原因', type: 'enum', options: ['离职退宿', '个人外住', '调宿退宿', '工作调动', '其他原因'], width: 110 },
    { name: '住宿状态', type: 'system', options: ['在住', '待入住', '已退宿'], readonly: true, width: 90 },
    ...STAY_FEE_FIELDS,
  ],
  asset: [
    { name: '资产编号', type: 'text', width: 120 },
    { name: '资产名称', type: 'text', width: 110 },
    { name: '资产分类', type: 'enum', options: ['家具', '家电', '办公', '其他'], width: 90 },
    { name: '规格型号', type: 'text', width: 110 },
    { name: '所属宿舍', type: 'text', width: 170 },
    { name: '数量', type: 'number', integer: true, width: 70 },
    { name: '计量单位', type: 'enum', options: ['张', '个', '台', '把', '套'], width: 80 },
    { name: '资产价值(元)', type: 'number', width: 110 },
    { name: '购置日期', type: 'date', width: 110 },
    { name: '责任人', type: 'text', width: 90 },
    { name: '报修日期', type: 'date', width: 110 },
    { name: '维修状态', type: 'enum', options: ['无维修', '待维修', '维修中', '已修复'], width: 100 },
    { name: '维修费用(元)', type: 'number', width: 110 },
    { name: '维修说明', type: 'textarea', width: 150 },
    { name: '备注', type: 'textarea', width: 120 },
    { name: '资产状态', type: 'system', options: ['正常', '损坏', '报废'], readonly: true, width: 90 },
  ],
  fee: [
    { name: '账单月份', type: 'text', width: 100 },
    { name: '员工工号', type: 'text', width: 110 },
    { name: '姓名', type: 'text', width: 90 },
    { name: '所属部门', type: 'text', width: 110 },
    { name: '宿舍号', type: 'text', width: 170 },
    { name: '入住天数', type: 'number', integer: true, width: 90 },
    { name: '分摊比例', type: 'number', width: 90 },
    { name: '宿舍押金(元)', type: 'number', width: 110 },
    { name: '公租房租(元)', type: 'number', width: 110 },
    { name: '水费(元)', type: 'number', width: 90 },
    { name: '电费(元)', type: 'number', width: 90 },
    { name: '天然气费(元)', type: 'number', width: 100 },
    { name: '社保个人扣款(元)', type: 'number', width: 130 },
    { name: '公积金个人扣款(元)', type: 'number', width: 140 },
    { name: '其它扣款(元)', type: 'number', width: 110 },
    { name: '扣款合计(元)', type: 'number', width: 120 },
    { name: '退款(元)', type: 'number', width: 90 },
    { name: '补贴合计(元)', type: 'number', width: 110 },
    { name: '备注', type: 'textarea', width: 120 },
    { name: '账单状态', type: 'system', options: ['已生成', '已作废'], readonly: true, width: 90 },
  ],
}

/** 只读派生列。 */
export const HR_DORM_DERIVED_FIELDS: ReadonlySet<string> = new Set([
  '可用床位数', '月费用单价(元/月)', '季度租金(元/季)', '住宿记录ID',
  '宿舍状态', '住宿状态', '资产状态', '账单状态', '入住天数', '扣款合计(元)', '补贴合计(元)',
])

/** 整数列（严禁 4.00 / 2.00）。 */
export const HR_DORM_INTEGER_FIELDS: ReadonlySet<string> = new Set([
  '床位数', '可用床位数', '数量', '入住天数',
])

/** 房型 → 床位数（与后端 hrDormBedsOfType 同源）。 */
export function hrDormBedsOfType(roomType: string): number {
  switch (String(roomType || '').trim()) {
    case '单人间': return 1
    case '双人间':
    case '夫妻房': return 2
    case '四人间': return 4
    case '六人间': return 6
    case '八人间': return 8
    default: return 1
  }
}

/** 编辑抽屉分组：档案 + 费用项目。 */
export function hrDormDrawerGroups(view: HrDormView): Array<{ key: string; label: string; fields: HrDormField[] }> {
  const all = HR_DORM_FIELDS[view] || []
  if (view === 'asset' || view === 'fee') {
    return [{ key: view, label: HR_DORM_GROUP_OF[view], fields: [...all] }]
  }
  const main = all.filter((f) => !f.subGroup)
  const fee = all.filter((f) => f.subGroup === '费用项目')
  const out: Array<{ key: string; label: string; fields: HrDormField[] }> = [
    { key: 'main', label: HR_DORM_GROUP_OF[view], fields: main },
  ]
  if (fee.length) out.push({ key: 'fee', label: '费用项目', fields: fee })
  return out
}

/** 状态 Tag 主题。 */
/** 维修状态三色（无维修=灰、待维修=橙、维修中=蓝、已修复=绿）。 */
export function hrDormRepairTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const t = String(v || '').trim()
  if (t === '已修复' || t === '无维修') return t === '无维修' ? 'default' : 'success'
  if (t === '待维修' || t === '维修中') return 'warning'
  return 'default'
}

/** 状态列：维修状态不是加锁字段但要走 Tag 渲染（列表插槽）。 */
export function hrDormStatusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '').trim()
  if (/空闲|在住|正常|已生成|已修复|无维修/.test(s)) return 'success'
  if (/部分入住|待入住|待维修|维修中|损坏|待/.test(s)) return 'warning'
  if (/满员|已退宿|报废|作废/.test(s)) return 'danger'
  return 'default'
}

export function hrDormRoomTypeTheme(v: string): 'success' | 'warning' | 'default' {
  const s = String(v ?? '').trim()
  if (s === '公租房') return 'warning'
  if (s === '公司宿舍') return 'success'
  return 'default'
}

/** 公租房合计（19 套口径）。 */
export function dormPublicTotals(rows: any[]): Record<string, string> {
  const num = (v: any) => {
    const n = Number(String(v ?? '').replace(/,/g, ''))
    return Number.isFinite(n) ? n : 0
  }
  const round2 = (n: number) => Math.round(n * 100) / 100
  const rent = rows.reduce((n, r) => n + num(r['房屋租金(元/月)']), 0)
  const prop = rows.reduce((n, r) => n + num(r['物业费(元/月)']), 0)
  const area = rows.reduce((n, r) => n + num(r['建筑面积(㎡)']), 0)
  // 季度租金：档案有值用档案，缺省按 (租金+物业)×3 派生（与后端 Normalize 同口径）
  const quarter = rows.reduce((n, r) => {
    const q = num(r['季度租金(元/季)'])
    if (q > 0) return n + q
    return n + (num(r['房屋租金(元/月)']) + num(r['物业费(元/月)'])) * 3
  }, 0)
  return {
    租金: rent.toFixed(0),
    物业: prop.toFixed(0),
    建筑面积: area.toFixed(2),
    季度租金: round2(quarter).toFixed(0),
  }
}

const money = (v: any) => {
  const n = Number(String(v ?? '').replace(/,/g, ''))
  return Number.isFinite(n)
    ? n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
    : ''
}

/** 费用信息列（19 列，必须自带 value）。 */
export function hrDormChargeColumns(): HrColumn[] {
  return [
    { colKey: 'row-select', title: '', width: 44 },
    ...HR_DORM_FIELDS.fee.map((f) => ({
      colKey: f.name,
      title: f.name,
      width: f.width || 110,
      dataType: f.type === 'number' ? 'number' : undefined,
      ...(f.type === 'system' ? { slot: true } : {}),
      value: (row: any) => {
        const v = row?.[f.name] ?? row?.custom_data?.[f.name]
        if (v === undefined || v === null || v === '') return ''
        if (HR_DORM_INTEGER_FIELDS.has(f.name)) return String(Math.round(Number(v) || 0))
        if (f.type === 'number') return money(v)
        return String(v)
      },
    })),
  ] as unknown as HrColumn[]
}

/** 五标签列：优先 subs（六同步），缺失时用契约兜底。 */
export function hrDormColumns(view: HrDormView, subs: readonly any[] | null | undefined): HrColumn[] {
  if (view === 'fee') return hrDormChargeColumns()
  const group = HR_DORM_GROUP_OF[view]
  const mock = HR_DORM_FIELDS[view].map((f) => ({
    name: f.name,
    label: f.name,
    data_type: f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text',
    enabled: true,
    is_default: true,
    width: f.width || 0,
    group,
  }))
  const scoped = (Array.isArray(subs) ? subs : []).filter(
    (s) => s && String(s.group || '').trim() === group && s.enabled !== false,
  )
  return hrColumnsFromSubs(scoped.length ? scoped : mock, hrColumnsFromSubs(mock, []))
}
