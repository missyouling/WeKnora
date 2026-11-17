/**
 * 宿舍管理五标签契约（第 61 轮）。
 *
 * 与后端 `internal/handler/hr_dorm_schema.go` 的 hrDormFieldDefs 同源：
 *
 *   宿舍档案 15 / 公租房档案 17 / 住宿记录 15 / 资产档案 10 / 费用信息 9
 *
 * 页面标签顺序 = 设置抽屉胶囊顺序 = 本文件 HR_DORM_VIEWS 顺序（六同步）。
 */

import { hrColumnsFromSubs } from './hrCore'
import type { HrColumn } from './hrCore'

export type HrDormView = 'dorm' | 'public' | 'stay' | 'asset' | 'fee'

export interface HrDormViewDef {
  key: HrDormView
  /** 页面标签文案 = 设置抽屉胶囊名 = 后端 subs.group */
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

/** 字段契约（类型 / 枚举 / 只读），设置抽屉与编辑抽屉共用。 */
export interface HrDormField {
  name: string
  type: 'text' | 'number' | 'date' | 'enum' | 'textarea' | 'system'
  options?: string[]
  readonly?: boolean
  width?: number
}

const ROOM_TYPES = ['单人间', '双人间', '四人间', '六人间', '八人间']
const ROOM_STATUS = ['空闲', '部分入住', '满员', '维修中']
const BEDS = ['1号床', '2号床', '3号床', '4号床', '5号床', '6号床', '7号床', '8号床']
const METERS = [
  '宿舍总表', '宿舍302', '宿舍304',
  '持睿403', '持睿404', '持睿405', '持睿406', '持睿407', '持睿408',
  '持睿409', '持睿410', '持睿411', '持睿412', '持睿413', '持睿414',
  '持睿415', '持睿416', '持睿417', '持睿418',
  '两山景苑5栋2-6', '两山景苑5栋2-7', '两山景苑5栋2-8',
  '两山景苑5栋21-15', '两山景苑5栋21-16',
  '两山景苑5栋24-3', '两山景苑5栋24-6', '两山景苑5栋24-7',
  '两山景苑5栋24-8', '两山景苑5栋24-9',
]

/** 五胶囊字段契约（顺序 = 表头顺序）。 */
export const HR_DORM_FIELDS: Record<HrDormView, readonly HrDormField[]> = {
  dorm: [
    { name: '宿舍编号', type: 'text', readonly: true, width: 120 },
    { name: '宿舍楼/区域', type: 'text', width: 120 },
    { name: '房间号', type: 'text', width: 90 },
    { name: '房型', type: 'enum', options: ROOM_TYPES, width: 100 },
    { name: '床位数', type: 'number', width: 90 },
    { name: '可用床位数', type: 'number', readonly: true, width: 110 },
    { name: '性别限制', type: 'enum', options: ['男', '女', '不限'], width: 100 },
    { name: '房间面积(㎡)', type: 'number', width: 110 },
    { name: '配套设施', type: 'text', width: 180 },
    { name: '关联电表', type: 'enum', options: METERS, width: 130 },
    { name: '关联水表', type: 'enum', options: METERS, width: 130 },
    { name: '所属公司/部门', type: 'text', width: 160 },
    { name: '管理员/舍长', type: 'text', width: 110 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 110 },
    { name: '备注', type: 'textarea', width: 160 },
  ],
  public: [
    { name: '房屋编号', type: 'text', readonly: true, width: 110 },
    { name: '房屋地址', type: 'text', width: 220 },
    { name: '租赁单位', type: 'enum', options: ['重庆星达铜业有限公司'], width: 180 },
    { name: '建筑面积(㎡)', type: 'number', width: 110 },
    { name: '房型', type: 'enum', options: ROOM_TYPES, width: 100 },
    { name: '床位数', type: 'number', width: 90 },
    { name: '可用床位数', type: 'number', readonly: true, width: 110 },
    { name: '租金(元/月)', type: 'number', width: 110 },
    { name: '物业(元/月)', type: 'number', width: 110 },
    { name: '首月费用(元)', type: 'number', readonly: true, width: 120 },
    { name: '季度(元/季)', type: 'number', readonly: true, width: 120 },
    { name: '关联电表', type: 'enum', options: METERS, width: 130 },
    { name: '关联水表', type: 'enum', options: METERS, width: 130 },
    { name: '关联气表', type: 'enum', options: METERS, width: 130 },
    { name: '管理员/舍长', type: 'text', width: 110 },
    { name: '宿舍状态', type: 'system', options: ROOM_STATUS, readonly: true, width: 110 },
    { name: '备注', type: 'textarea', width: 160 },
  ],
  stay: [
    { name: '住宿记录ID', type: 'text', readonly: true, width: 120 },
    { name: '员工工号', type: 'text', readonly: true, width: 120 },
    { name: '员工姓名', type: 'text', width: 100 },
    { name: '所属部门', type: 'text', readonly: true, width: 120 },
    { name: '联系电话', type: 'text', readonly: true, width: 130 },
    { name: '紧急联系人及电话', type: 'text', readonly: true, width: 160 },
    { name: '房源类型', type: 'enum', options: ['厂区宿舍', '公租房'], width: 110 },
    { name: '宿舍编号/房屋地址', type: 'text', width: 220 },
    { name: '床位号', type: 'enum', options: BEDS, width: 100 },
    { name: '入住类型', type: 'enum', options: ['新入住', '调宿', '续住'], width: 100 },
    { name: '入住日期', type: 'date', width: 120 },
    { name: '退宿日期', type: 'date', width: 120 },
    { name: '押金金额(元)', type: 'number', width: 120 },
    { name: '退宿原因', type: 'textarea', width: 160 },
    { name: '住宿状态', type: 'system', options: ['在住', '待入住', '已退宿'], readonly: true, width: 100 },
  ],
  asset: [
    { name: '资产编号', type: 'text', readonly: true, width: 120 },
    { name: '资产名称/类型', type: 'enum', options: ['床', '衣柜', '空调', '热水器', '书桌', '椅子', '洗衣机', '其他'], width: 130 },
    { name: '所属宿舍', type: 'text', width: 200 },
    { name: '数量', type: 'number', width: 80 },
    { name: '责任人', type: 'text', width: 110 },
    { name: '报修日期', type: 'date', width: 120 },
    { name: '维修状态', type: 'enum', options: ['无维修', '待维修', '维修中', '已修复'], width: 110 },
    { name: '维修费用(元)', type: 'number', width: 120 },
    { name: '维修说明', type: 'textarea', width: 180 },
    { name: '资产状态', type: 'system', options: ['正常', '损坏', '报废'], readonly: true, width: 100 },
  ],
  fee: [
    { name: '账单周期', type: 'text', width: 110 },
    { name: '房源数', type: 'number', width: 90 },
    { name: '入住人数', type: 'number', width: 100 },
    { name: '房租合计', type: 'number', width: 120 },
    { name: '物业合计', type: 'number', width: 120 },
    { name: '水电费合计', type: 'number', width: 130 },
    { name: '费用总计', type: 'number', width: 130 },
    { name: '账单状态', type: 'system', options: ['已生成', '已作废'], readonly: true, width: 100 },
    { name: '生成时间', type: 'date', width: 170 },
  ],
}

/** 只读派生列（不可编辑；保存时后端重算）。 */
export const HR_DORM_DERIVED_FIELDS: ReadonlySet<string> = new Set([
  '可用床位数', '首月费用(元)', '季度(元/季)', '在住人数', '住宿记录ID', '宿舍状态', '住宿状态', '资产状态', '账单状态',
])

/** 宿舍状态 / 住宿状态 / 资产状态 / 账单状态 的 Tag 主题。 */
export function hrDormStatusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '').trim()
  if (/空闲|在住|正常|已生成|已修复|无维修/.test(s)) return 'success'
  if (/部分入住|待入住|待维修|维修中|损坏|待/.test(s)) return 'warning'
  if (/满员|已退宿|报废|作废/.test(s)) return 'danger'
  return 'default'
}

/** 房源类型 Tag 主题。 */
export function hrDormRoomTypeTheme(v: string): 'success' | 'warning' | 'default' {
  const s = String(v ?? '').trim()
  if (s === '公租房') return 'warning'
  if (s === '厂区宿舍') return 'success'
  return 'default'
}

/** 公租房合计（图 7：首月 3401 / 面积 485.55 / 租金 2915 / 物业 486 / 季度 10203）。 */
export function dormPublicTotals(rows: any[]): Record<string, string> {
  const num = (v: any) => {
    const n = Number(String(v ?? '').replace(/,/g, ''))
    return Number.isFinite(n) ? n : 0
  }
  const rent = rows.reduce((n, r) => n + num(r['租金(元/月)']), 0)
  const prop = rows.reduce((n, r) => n + num(r['物业(元/月)']), 0)
  const area = rows.reduce((n, r) => n + num(r['建筑面积(㎡)']), 0)
  const round2 = (n: number) => Math.round(n * 100) / 100
  const first = round2(rent + prop)
  return {
    首月费用: first.toFixed(0),
    建筑面积: area.toFixed(2),
    租金: rent.toFixed(0),
    物业: prop.toFixed(0),
    季度: round2(first * 3).toFixed(0),
  }
}

/**
 * 费用信息列表列。
 *
 * 必须自带 `value`：HrManagement 的 tableColumns 只认 `c.value?.(row)`，
 * 缺省会渲染成空白单元格（真机表现为「有行但全空、合计行全 0」）。
 */
export function hrDormBillColumns(): HrColumn[] {
  const money = (v: any) => {
    const n = Number(String(v ?? '').replace(/,/g, ''))
    return Number.isFinite(n)
      ? n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
      : ''
  }
  return [
    { colKey: 'row-select', title: '', width: 44 },
    ...HR_DORM_FIELDS.fee.map((f) => ({
      colKey: f.name,
      title: f.name,
      width: f.width || 120,
      dataType: f.type === 'number' ? 'number' : undefined,
      ...(f.type === 'system' ? { slot: true } : {}),
      value: (row: any) => {
        const v = row?.[f.name] ?? row?.custom_data?.[f.name]
        if (v === undefined || v === null) return ''
        return f.type === 'number' ? money(v) : String(v)
      },
    })),
  ] as unknown as HrColumn[]
}

/** 当前标签的列集合：优先 subs（六同步），缺失时用字段契约兜底。 */
export function hrDormColumns(view: HrDormView, subs: readonly any[] | null | undefined): HrColumn[] {
  if (view === 'fee') return hrDormBillColumns()
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
  // 只取当前胶囊：其他胶囊的同名字段（床位数/宿舍状态）不得混进来。
  return hrColumnsFromSubs(scoped.length ? scoped : mock, hrColumnsFromSubs(mock, []))
}
