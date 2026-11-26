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

import { statusTextClassOf } from './hrStatusStyle'
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
  type: 'text' | 'number' | 'date' | 'month' | 'enum' | 'textarea' | 'system' | 'multi'
  /** 编辑抽屉分组：'费用项目' 表示第二分组，缺省为第一分组 */
  subGroup?: string
  options?: string[]
  readonly?: boolean
  width?: number
  /** 整数渲染（床位数 / 可用床位数 / 数量 / 入住天数） */
  integer?: boolean
}

const ROOM_TYPES = ['单人间', '双人间', '夫妻房', '四人间', '六人间', '八人间']
/**
 * 第 72 轮：宿舍状态统一四态。
 *
 * `维修中` 已废弃（存量由 hrMigrateDormState72 清洗为 `空置`）；
 * `空置` 是「人为封存、不对外安排入住」的独立语义，由编辑抽屉的开关控制，
 * 与「空闲（可入住但当前无人）」严格区分。
 */
const ROOM_STATUS = ['空闲', '部分入住', '满员', '空置']
const BEDS = ['1号床', '2号床', '3号床', '4号床', '5号床', '6号床', '7号床', '8号床']
/**
 * 第 70 轮：原 `METERS` 硬编码表名清单已删除。
 *
 * 表计定义权回到宿舍/公租房档案，下拉候选由 `dormMeterOptionsOf()` 从档案
 * （或能耗表计）实时派生 —— 硬编码清单必然与档案漂移。
 */

/**
 * 费用项目（第 70 轮重构：水电气表定义权从「能耗表计设置」移回房源档案）。
 *
 * 宿舍只认「电 + 水」（员工宿舍没有燃气），公租房只认「水 + 气」（不定义电表）。
 * 表计的**名称 / 表号 / 倍率 / 单价**都在房源档案里维护，能耗核算按表号实时引用。
 */
const DORM_FEE_FIELDS: readonly HrDormField[] = [
  { name: '宿舍电表名称', type: 'text', subGroup: '费用项目' },
  { name: '宿舍电表号', type: 'text', subGroup: '费用项目' },
  { name: '电费标准(元/度)', type: 'number', subGroup: '费用项目' },
  { name: '电表倍率', type: 'number', subGroup: '费用项目' },
  { name: '宿舍水表名称', type: 'text', subGroup: '费用项目' },
  { name: '宿舍水表号', type: 'text', subGroup: '费用项目' },
  { name: '水费单价(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '水表倍率', type: 'number', subGroup: '费用项目' },
  { name: '宿舍押金(元)', type: 'number', subGroup: '费用项目' },
]

const PUBLIC_FEE_FIELDS: readonly HrDormField[] = [
  { name: '公租房水表名称', type: 'text', subGroup: '费用项目' },
  { name: '公租房水表号', type: 'text', subGroup: '费用项目' },
  { name: '水费单价(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '污水处理费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '二次供水费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '垃圾处理费(元/套)', type: 'number', subGroup: '费用项目' },
  { name: '水表倍率', type: 'number', subGroup: '费用项目' },
  { name: '公租房气表名称', type: 'text', subGroup: '费用项目' },
  { name: '公租房气表号', type: 'text', subGroup: '费用项目' },
  { name: '气费标准(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '气表倍率', type: 'number', subGroup: '费用项目' },
  { name: '宿舍押金(元)', type: 'number', subGroup: '费用项目' },
  { name: '房屋租金(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '月费用单价(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '季度租金(元/季)', type: 'number', subGroup: '费用项目' },
  { name: '物业费(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '租期起始月份', type: 'month', subGroup: '费用项目' },
  { name: '租期截止月份', type: 'month', subGroup: '费用项目' },
]

/**
 * 住宿记录「费用项目」= 目标房源的收费项（第 70 轮：**可编辑**）。
 *
 * 用户在这条记录上改的值只落到该记录自身（`fee_overrides`），
 * 绝不反写房源档案 —— 房源基准值仍是下一笔入住的初始值。
 * 字段集合按「房源类型」二选一，与房源档案一一对应。
 */
const DORM_STAY_FEE_FIELDS: readonly HrDormField[] = [
  { name: '宿舍押金(元)', type: 'number', subGroup: '费用项目' },
  { name: '电费标准(元/度)', type: 'number', subGroup: '费用项目' },
  { name: '水费单价(元/m³)', type: 'number', subGroup: '费用项目' },
]

const PUBLIC_STAY_FEE_FIELDS: readonly HrDormField[] = [
  { name: '宿舍押金(元)', type: 'number', subGroup: '费用项目' },
  { name: '月费用单价(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '房屋租金(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '季度租金(元/季)', type: 'number', subGroup: '费用项目' },
  { name: '物业费(元/月)', type: 'number', subGroup: '费用项目' },
  { name: '水费单价(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '污水处理费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '二次供水费(元/m³)', type: 'number', subGroup: '费用项目' },
  { name: '垃圾处理费(元/套)', type: 'number', subGroup: '费用项目' },
  { name: '气费标准(元/m³)', type: 'number', subGroup: '费用项目' },
]

/**
 * 住宿记录的费用项（引用房档口径的字段名，全部归「费用项目」组）。
 *
 * 名字与「宿舍档案 / 公租房档案」保持一致（长名带单位），保存时落该条记录的
 * `fee_overrides`，核算优先取记录值、未改回落房档基准。
 */
const STAY_FEE_LINK_FIELDS: readonly HrDormField[] = [
  { name: '电费标准', type: 'number', subGroup: '费用项目' },
  { name: '水费单价', type: 'number', subGroup: '费用项目' },
  { name: '气费标准', type: 'number', subGroup: '费用项目' },
  { name: '房屋租金', type: 'number', subGroup: '费用项目' },
  { name: '月费用单价', type: 'number', subGroup: '费用项目' },
  { name: '季度租金', type: 'number', subGroup: '费用项目' },
  { name: '物业费', type: 'number', subGroup: '费用项目' },
  { name: '污水处理费', type: 'number', subGroup: '费用项目' },
  { name: '二次供水费', type: 'number', subGroup: '费用项目' },
  { name: '垃圾处理费', type: 'number', subGroup: '费用项目' },
]

/** 住宿记录跨房源通用项：分摊比例。 */
const STAY_BASE_FIELDS: readonly HrDormField[] = [
  { name: '分摊比例', type: 'number', subGroup: '费用项目' },
]

/** 五标签字段契约（顺序 = 表头顺序）。 */
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
    // 第 70 轮：成员 = 本房源在住人员剔除舍长（只读派生，默认进表头）
    { name: '成员', type: 'system', readonly: true, width: 140 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 100 },
    { name: '备注', type: 'textarea', width: 140 },
    ...DORM_FEE_FIELDS,
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
    // 第 70 轮：成员 = 本房源在住人员剔除舍长（只读派生，默认进表头）
    { name: '成员', type: 'system', readonly: true, width: 140 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 100 },
    { name: '备注', type: 'textarea', width: 140 },
    ...PUBLIC_FEE_FIELDS,
  ],
  stay: [
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
    { name: '退宿原因', type: 'enum', options: ['离职退宿', '个人外住', '调宿退宿', '工作调动', '其他原因'], width: 110 },
    { name: '住宿状态', type: 'system', options: ['在住', '待入住', '已退宿'], readonly: true, width: 90 },
    /**
     * 第 72 轮：所有**费用类**字段统一进「费用项目」分组。
     *
     * 旧实现把押金/租金/物业/单价混在「住宿记录」基础信息里，用户改个押金要在
     * 一堆身份字段里翻。分组只影响抽屉排版，不影响列与保存链路。
     * 三个「入住时*表度数」字段已彻底删除（六同步：契约/subs/列/筛选/抽屉/模板）。
     */
    { name: '押金金额(元)', type: 'number', subGroup: '费用项目', width: 110 },
    ...STAY_FEE_LINK_FIELDS,
    ...STAY_BASE_FIELDS,
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
    // 第 65 轮：资产档案彻底删除维修 4 字段（报修日期/维修状态/维修费用/维修说明）；
    // 「资产状态」由只读 system 改为可编辑 enum，新增默认「正常」。
    { name: '备注', type: 'textarea', width: 120 },
    { name: '资产状态', type: 'enum', options: ['正常', '损坏', '报废'], width: 90 },
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

/**
 * 第 70 轮：宿舍 12 类标准配套（与 `hrSeedDormAssets70` 同源）。
 *
 * 配套设施下拉 = 这份标准清单 ∪ 资产档案里出现过的名称 ∪ 当前记录已有值，
 * 去重展示。即使迁移还没跑，用户也能选到全部 12 类。
 */
export const HR_DORM_STANDARD_FACILITIES: readonly string[] = [
  '晾衣杆', '毛巾杆', '衣柜', '床柜', '床头柜', '空调遥控器',
  '空调', '电热水器', '床', '床垫', '钥匙', '窗帘',
]

/** 只读派生列。 */
export const HR_DORM_DERIVED_FIELDS: ReadonlySet<string> = new Set([
  // 第 70 轮：`住宿记录ID` 已从字段契约彻底删除（只留内部主键 id 做路由参数）
  '可用床位数', '月费用单价(元/月)', '季度租金(元/季)', '成员',
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

/**
 * 编辑抽屉分组：档案 + 费用项目。
 *
 * 第 70 轮：住宿记录的费用项**随房源类型变化**（宿舍 3 项 / 公租房 10 项），
 * 由调用方把当前表单里的「房源类型」传进来；档案本身固定用各自常量。
 */
export function hrDormDrawerGroups(
  view: HrDormView,
  opts: { roomType?: string } = {},
): Array<{ key: string; label: string; fields: HrDormField[] }> {
  const all = [...(HR_DORM_FIELDS[view] || [])]
  if (view === 'stay') {
    const isPublic = String(opts.roomType || '').trim() === '公租房'
    all.push(...(isPublic ? PUBLIC_STAY_FEE_FIELDS : DORM_STAY_FEE_FIELDS))
  }
  if (view === 'asset' || view === 'fee') {
    return [{ key: view, label: HR_DORM_GROUP_OF[view], fields: all }]
  }
  const main = all.filter((f) => !f.subGroup)
  const fee = all.filter((f) => f.subGroup === '费用项目')
  const out: Array<{ key: string; label: string; fields: HrDormField[] }> = [
    { key: 'main', label: HR_DORM_GROUP_OF[view], fields: main },
  ]
  if (fee.length) out.push({ key: 'fee', label: '费用项目', fields: fee })
  return out
}

/** 第 64 轮：字段契约（供「重置」在 subs 未就绪时兜底）。 */
export function hrDormFieldsOf(view: HrDormView): readonly HrDormField[] {
  return HR_DORM_FIELDS[view] || []
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

/**
 * 第 64 轮：宿舍/住宿/资产状态的纯文字配色类（列表与抽屉同源）。
 *
 * 为什么不再用 Tag：真机上 Tag 的描边被 TDesign 主题渲染成深色方框，
 * 用户反馈「宿舍状态有黑框」。这里统一走文字色，视觉与「参保状态」一致。
 */
/**
 * 宿舍域状态配色（保留旧签名，内部委托到全局唯一映射）。
 *
 * 第 70 轮：状态语义只在 `hrStatusStyle.ts` 维护一份 —— 列表、抽屉、打印共用，
 * 避免「列表满员红、抽屉满员灰」这类漂移。第 3 个参数允许调用方传字段名
 * （资产状态 / 住宿状态 / 房源类型各自有独立值域）。
 */
export function hrDormStatusTextClass(v: unknown, field = '宿舍状态'): string {
  return statusTextClassOf(field, v)
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

/**
 * 扣款明细列（必须自带 value）。
 *
 * 第 65 轮：**删除「员工工号」列**（弹窗里姓名已足够定位，工号挤占金额列宽）；
 * 金额列宽度按「1,214.60 / 2,806.00 / 48,254.78 / 88,147.20」实测给足，
 * 杜绝任何金额被省略号截断。
 */
export const DORM_CHARGE_WIDTH_OVERRIDE: Record<string, number> = {
  姓名: 130, 所属部门: 120, 宿舍号: 110,
  入住天数: 96, 分摊比例: 96,
  '宿舍押金(元)': 118, '公租房租(元)': 118,
  '水费(元)': 110, '电费(元)': 110, '天然气费(元)': 120,
  '社保个人扣款(元)': 140, '公积金个人扣款(元)': 150,
  '其它扣款(元)': 120, '扣款合计(元)': 130,
  '退款(元)': 110, '补贴合计(元)': 120,
  备注: 160,
}
export function hrDormChargeColumns(): HrColumn[] {
  return [
    { colKey: 'row-select', title: '', width: 44 },
    ...HR_DORM_FIELDS.fee.filter((f) => f.name !== '员工工号').map((f) => ({
      colKey: f.name,
      title: f.name,
      width: DORM_CHARGE_WIDTH_OVERRIDE[f.name] || f.width || 110,
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

/** 第 64 轮：费用信息月度账单汇总列（每月 1 行，与社保/公积金月度账单同形）。 */
export const HR_DORM_CHARGE_BILL_FIELDS: readonly HrDormField[] = [
  { name: '账单月份', type: 'text', width: 100 },
  { name: '总人数', type: 'number', integer: true, width: 90 },
  { name: '住宿人数', type: 'number', integer: true, width: 100 },
  { name: '押金合计', type: 'number', width: 110 },
  { name: '房租合计', type: 'number', width: 110 },
  { name: '水费合计', type: 'number', width: 100 },
  { name: '电费合计', type: 'number', width: 100 },
  { name: '气费合计', type: 'number', width: 100 },
  { name: '社保个人合计', type: 'number', width: 130 },
  { name: '公积金个人合计', type: 'number', width: 140 },
  { name: '其它扣款合计', type: 'number', width: 120 },
  { name: '扣款总计', type: 'number', width: 120 },
  { name: '退款合计', type: 'number', width: 110 },
  { name: '补贴合计', type: 'number', width: 110 },
  { name: '账单状态', type: 'system', options: ['已生成', '已作废'], readonly: true, width: 90 },
]

/** 费用信息月度账单列（必须自带 value；合计列走千分位）。 */
export function hrDormChargeBillColumns(): HrColumn[] {
  return [
    { colKey: 'row-select', title: '', width: 44 },
    ...HR_DORM_CHARGE_BILL_FIELDS.map((f) => ({
      colKey: f.name,
      title: f.name,
      width: f.width || 110,
      dataType: f.type === 'number' ? 'number' : undefined,
      ...(f.type === 'system' ? { slot: true } : {}),
      value: (row: any) => {
        const v = row?.[f.name] ?? row?.custom_data?.[f.name]
        if (v === undefined || v === null || v === '') return ''
        if (f.integer) return String(Math.round(Number(v) || 0))
        if (f.type === 'number') return money(v)
        return String(v)
      },
    })),
  ] as unknown as HrColumn[]
}

/** 费用信息月度账单合计行（金额按分累加，与后端口径一致）。 */
export function dormChargeBillTotals(rows: any[]): Record<string, string> {
  const cents = (v: any) => Math.round(Number(String(v ?? '').replace(/,/g, '')) * 100) || 0
  const out: Record<string, string> = { 账单月份: '合计' }
  for (const f of HR_DORM_CHARGE_BILL_FIELDS) {
    if (f.type !== 'number') continue
    const sum = rows.reduce((n, r) => n + cents(r?.[f.name]), 0)
    out[f.name] = f.integer ? String(Math.round(sum / 100)) : money(sum / 100)
  }
  return out
}

/** 五标签列：优先 subs（六同步），缺失时用契约兜底。 */
export function hrDormColumns(view: HrDormView, subs: readonly any[] | null | undefined): HrColumn[] {
  if (view === 'fee') return hrDormChargeBillColumns()
  const group = HR_DORM_GROUP_OF[view]
  const mock = HR_DORM_FIELDS[view].map((f) => ({
    name: f.name,
    label: f.name,
    data_type: f.type === 'number' ? 'number' : f.type === 'date' ? 'date'
      : f.type === 'month' ? 'month' : 'text',
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
