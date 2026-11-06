// 车队记录型菜单的「列 / 筛选器 / 抽屉字段」派生层（第 28 轮六同步）。
//
// 为什么必须独立成文件：12 个菜单的字段来源只有一处（分类 subs），
// 但消费方有四处（列表列、筛选器、抽屉字段、打印列）。若各页各写一段
// `subs.map(...)`，改口径必然漂移；这里把四处的派生规则一次定死。
//
// 与发票/能耗的差异：记录型记录的字段值都在 `record.data[中文字段名]` 里
// （不是行级扁平属性），所以取值器统一走 `data[name]`，车牌号再补一层
// `vehicle_id -> plate_no` 的映射（后端已回填，派生记录与手工记录一致）。

import type { FilterFieldSpec } from './businessFilter'
import type { SummaryColumn } from './listSummary'
import type { CatalogColumn } from './useCatalogPdf'
import { grouped, num2 } from './formatNumber'
import { isCertStatusName, isExtractStatusName, isStatusFieldName } from './statusFields'

/** 后端分类 subs 的最小子集（只取派生需要的键）。 */
export interface FleetSub {
  name: string
  label?: string
  enabled?: boolean
  is_default?: boolean
  data_type?: string
  width?: number
  /** 是否出现在筛选栏（后端 subs.filterable）。缺省按字段名白名单兜底。 */
  filterable?: boolean
}

/** 一列（列表 / 打印共用）。 */
export interface FleetColumn {
  key: string
  label: string
  dataType: string
  /** 是否默认显示（subs.is_default） */
  default: boolean
  /** 固定宽度（subs.width > 0） */
  width?: number
  /**
   * 列宽下限（仅派生列使用，如三色 Tag 状态列需要 100px）。
   * 用户配置了固定宽度时以 width 为准，否则按内容估算后再用 minWidth 兜住。
   */
  minWidth?: number
  /** 是否出现在筛选栏（由 subs.filterable 派生） */
  filterable?: boolean
  /** 取值：默认 data[name]，车牌号走 vehicle 映射 */
  value: (row: any) => string
  /** 原始值（合计行按数值累加用） */
  raw: (row: any) => number
}

/** 不作为筛选器的字段：备注类长文本（无检索价值）、以及纯展示派生列。 */
const NON_FILTERABLE = new Set(['备注', '来源文件', '页码'])

/**
 * 旧数据的筛选兜底白名单：subs 里没有 filterable 键（改造前写入的分类）时，
 * 只让这几个高频归属/编号字段可筛。**默认收窄**是关键 —— 之前把 subs 全量
 * 塞进筛选栏，一个分类 29 个字段平铺占满 4 行（真机实测）。
 */
const FILTERABLE_FALLBACK = new Set([
  '车牌号', '车牌号码', 'plate_no',
  '证照类型', '子分类', '分类', 'doc_type',
  '证件编号', '单号', '编号', '工单号', '保单号', '理赔单号', '违章单号', '轮胎编号',
  '状态', '办理状态', '处理状态', '理赔状态', '当前状态', '付款状态', '提醒状态',
  '持有人', '司机姓名', '申请人', '经办人',
])

/**
 * 字段是否出现在筛选栏。
 *
 * 三级判定：显式 filterable（true/false）> 白名单兜底 > 默认不可筛。
 * 与 is_default（列表默认显示）完全独立 —— 一个字段可以默认显示但不可筛。
 */
export function filterableOf(sub: FleetSub): boolean {
  const name = String(sub?.name || '')
  if (NON_FILTERABLE.has(name)) return false
  if (typeof sub?.filterable === 'boolean') return sub.filterable
  return FILTERABLE_FALLBACK.has(name)
}

/** 字段名含「日期 / 时间」→ 日期列。 */
const DATE_HINT = /日期|时间/
/**
 * 字段名含这些词 → 数值列。
 *
 * 注意「费用」不能单独当判据：「费用类型」「费用说明」是文本字段，
 * 被误判成 number 后既不能筛选、合计行也会试图累加字符串。
 * 因此只认「费用」后接金额语义（费用金额/费用合计），或明确的金额词。
 */
const NUMBER_HINT = /金额|税额|单价|数量|扣分|保费|里程|税率|比例|天数|次数|页码|百公里|^费用$|费用金额|费用合计|总费用/

/** 由 subs 推导 data_type（subs 已显式给类型时优先，缺省按字段名猜）。 */
export function dataTypeOfSub(sub: FleetSub): string {
  const explicit = String(sub.data_type || '').trim()
  if (explicit) return explicit
  const name = String(sub.name || '')
  if (DATE_HINT.test(name)) return 'date'
  if (NUMBER_HINT.test(name)) return 'number'
  return 'text'
}

/** 车牌号取值：优先 data 里的车牌，其次 vehicle_id 映射。 */
function plateOf(row: any, vehicleMap: Map<string, string>): string {
  const direct = String(row?.data?.['车牌号'] || row?.data?.['车牌号码'] || row?.data?.plate_no || '').trim()
  if (direct) return direct
  const vid = String(row?.vehicle_id || '')
  if (vid && vehicleMap.has(vid)) return vehicleMap.get(vid) || ''
  return ''
}

/** 数值单元格：金额两位、其余按值给。 */
function numText(v: any): string {
  const n = Number(v)
  return Number.isFinite(n) ? num2(n) : ''
}

/**
 * 由分类 subs 派生列定义。
 *
 * `vehicleMap` 是 vehicle_id → plate_no 的映射（列表页一次性加载）。
 * 停用（enabled === false）的字段不产出列 —— 与字段定义面板严格一致。
 */
export function buildFleetColumns(
  subs: readonly FleetSub[] | undefined,
  vehicleMap: Map<string, string>,
): FleetColumn[] {
  const out: FleetColumn[] = []
  const seen = new Set<string>()
  for (const s of Array.isArray(subs) ? subs : []) {
    const name = String(s?.name || '').trim()
    if (!name || seen.has(name)) continue
    if (s.enabled === false) continue
    seen.add(name)
    const dataType = dataTypeOfSub(s)
    const label = String(s.label || name).trim() || name
    const isPlate = name === '车牌号' || name === '车牌号码' || name === 'plate_no'
    const width = Number(s.width)
    const read = (row: any): any => {
      if (isPlate) return plateOf(row, vehicleMap)
      return row?.data?.[name]
    }
    out.push({
      key: name,
      label,
      dataType,
      filterable: filterableOf(s),
      default: s.is_default === true,
      width: Number.isFinite(width) && width > 0 ? Math.round(width) : undefined,
      raw: (row: any) => {
        const v = read(row)
        if (dataType === 'number') {
          const n = Number(v)
          return Number.isFinite(n) ? n : 0
        }
        return 0
      },
      value: (row: any) => {
        const v = read(row)
        if (v === undefined || v === null || v === '') {
          // 记录型统一空值文案：'—'（与列表其他模块一致，不显示 "undefined"）
          return '—'
        }
        if (dataType === 'date') return String(v).replace('T', ' ').slice(0, 10)
        if (dataType === 'number') {
          // 金额语义：千分位 + 两位小数（列表金额必须带千分位，52,015.90 才读得出来）；
          // 其余数量原值，纯整数不补 .00
          return SUM_AMOUNT_HINT.test(label) ? grouped(v, { fixed: true }) : numText(v)
        }
        return String(v)
      },
    })
  }
  return out
}

/** 标签含「金额 / 税额 / 费用 / 保费 / 单价」→ 按金额格式化（两位小数 + 千分位）。 */
const SUM_AMOUNT_HINT = /金额|税额|费用|保费|单价/

/** 由列定义派生「合计行」用的 SummaryColumn（可累加列才给 fieldType）。 */
export function buildFleetSummaryColumns(cols: readonly FleetColumn[]): SummaryColumn[] {
  return cols.map((c) => ({
    key: c.key,
    label: c.label,
    fieldType: c.dataType === 'number' ? (SUM_AMOUNT_HINT.test(c.label) ? 'amount' : 'number') : 'text',
    value: (row: any) => c.raw(row),
  }))
}

/**
 * 由列定义派生筛选器。
 *
 * 布局对齐发票管理：搜索框（BusinessFilterBar 内置）+ 车牌号 + 日期区间 +
 * **仅 filterable 的字段**。之前是全量 subs 平铺，29 个输入框占满 4 行。
 * 上限再兜一层：最多 3 个字段筛选器，防止用户把 20 个字段都开启后再次爆行。
 */
export function buildFleetFilterSpecs(cols: readonly FleetColumn[]): FilterFieldSpec[] {
  const out: FilterFieldSpec[] = []
  const hasPlate = cols.some((c) => c.key === '车牌号' || c.key === '车牌号码')
  if (hasPlate) {
    out.push({ key: 'plate', type: 'text', placeholder: '车牌号', clearable: true })
  }
  out.push({ key: 'dateRange', type: 'date-range', placeholder: '记录日期', icon: 'time' })
  // 其余字段作为「字段筛选」：高基数用 text（车牌式自由输入），其余仍用 text
  // （记录型字段取值多为自由文本，下拉候选在前端拿不到全量，用 text 更准）。
  let fieldCount = 0
  for (const c of cols) {
    if (fieldCount >= 3) break
    if (!c.filterable) continue
    if (c.key === '车牌号' || c.key === '车牌号码') continue
    if (c.dataType === 'date' || c.dataType === 'number') continue
    out.push({
      key: `field:${c.key}`,
      type: 'text',
      placeholder: c.label,
      clearable: true,
    })
    fieldCount++
  }
  return out
}

/** 由列定义派生打印列（与列表列同源）。 */
export function buildFleetCatalogColumns(cols: readonly FleetColumn[]): CatalogColumn[] {
  return cols.map((c) => ({ key: c.key, label: c.label, value: c.value }))
}

/** 把筛选状态拼成后端 `field_filters`（`key:value;key:value`）。 */
export function buildFleetFieldFiltersParam(state: Record<string, any>): string {
  const parts: string[] = []
  for (const [k, v] of Object.entries(state || {})) {
    // 车牌号有专用筛选器（key = `plate`，不是 `field:车牌号`）——它是派生台账
    // 与手工记录共同的归属键，后端按 vehicle_id 精确匹配。漏掉这一支会让
    // 「输车牌 → 列表纹丝不动」（真机 B4）。
    if (k === 'plate') {
      const value = String(v ?? '').trim()
      if (value) parts.push(`车牌号:${value}`)
      continue
    }
    if (!k.startsWith('field:')) continue
    const value = String(v ?? '').trim()
    if (!value) continue
    parts.push(`${k.slice('field:'.length)}:${value}`)
  }
  return parts.join(';')
}

/**
 * 汇总视图列（档案类菜单点「全部 / 已收录证照」卡片时使用）。
 *
 * 为什么需要单独一套：汇总视图下没有「当前分类」，各子分类字段差异很大
 * （行驶证有「档案编号」，许可证有「许可证号」，保险单有「保单号」）。
 * 若直接拼所有子分类的 subs，表头会变成 40+ 列且一半是空单元格。
 *
 * 口径：只保留**跨子分类通用的核心语义列** —— 子分类、车牌号、持有人/关联对象、
 * 证件编号、日期、状态、更新时间。取值按「字段名候选链」逐个试，
 * 命中即用，保证同一列在不同子分类下都能取到值。
 */
interface SummaryField {
  key: string
  label: string
  /** 字段名候选链：按顺序试，命中第一个非空值 */
  candidates: string[]
  dataType: string
}

const SUMMARY_FIELDS: SummaryField[] = [
  { key: 'sub_category', label: '证照类型', candidates: ['__sub_category'], dataType: 'text' },
  { key: 'plate_no', label: '车牌号', candidates: ['车牌号', '车牌号码', 'plate_no'], dataType: 'text' },
  { key: 'holder', label: '持有人/关联对象', candidates: ['持有人', '司机姓名', '车主', '业户名称', '单位名称', '姓名'], dataType: 'text' },
  { key: 'doc_no', label: '证件编号', candidates: ['证件编号', '许可证号', '证号', '档案编号', '保单号', '工单号', '编号', '单号'], dataType: 'text' },
  { key: 'valid_date', label: '有效期/日期', candidates: ['有效期', '有效期至', '发证日期', '初次领证日期', '出险日期', '维修日期', '日期'], dataType: 'text' },
  { key: 'status', label: '状态', candidates: ['状态', '办理状态', '处理状态', '理赔状态', '当前状态', '是否有效'], dataType: 'text' },
]

/** 汇总视图列（不含操作列，操作列由页面固定追加）。 */
export function buildFleetSummaryViewColumns(vehicleMap: Map<string, string>): FleetColumn[] {
  return SUMMARY_FIELDS.map((f) => ({
    key: f.key,
    label: f.label,
    dataType: f.dataType,
    default: true,
    value: (row: any) => {
      if (f.key === 'sub_category') {
        const v = String(row?.sub_category || row?.doc_type || '').trim()
        return v || '—'
      }
      for (const name of f.candidates) {
        if (name === '车牌号' || name === '车牌号码' || name === 'plate_no') {
          const p = plateOf(row, vehicleMap)
          if (p) return p
          continue
        }
        const v = row?.data?.[name]
        if (v !== undefined && v !== null && String(v).trim() !== '') {
          return String(v).trim()
        }
      }
      return '—'
    },
    raw: () => 0,
  }))
}

// ---------------------------------------------------------------------------
// 保存时的「通用列」推导（金额 / 日期）
//
// 记录型表单的字段名由 subs 决定（金额、含税金额、总费用、单价…），而列表的
// 金额列与合计行、费用汇总、日期筛选都读**通用列**（record.amount / record_date）。
// 两处口径必须在这里统一，且必须可测 —— 这两段都出过 bug：
//   · 金额把「单价」当总额，2 升 × 88.5 元记成 88.5（真机 D8 期望 177）；
//   · 日期候选链漏字段，导致日期筛选与月份汇总永远查不到新记录。
// ---------------------------------------------------------------------------

/**
 * 金额字段候选链（顺序即优先级）。
 *
 * 注意「单价/单价（元）」**不在**这里：单价是单位价格，直接当金额会把
 * 「数量 × 单价」的总价算小。单价走下面的乘法分支单独处理。
 */
const AMOUNT_FIELD_PRIORITY = ['金额', '含税金额', '总费用', '定损金额', '费用', '工时费', '材料费', '缴款金额']

/** 从表单数据推导通用金额列（0 表示没有可用金额）。 */
export function amountFromForm(data: Record<string, any>): number {
  for (const key of AMOUNT_FIELD_PRIORITY) {
    const v = Number(data?.[key])
    if (Number.isFinite(v) && v > 0) return v
  }
  // 单价 × 数量：加油/辅材这类没有「金额」字段的分类。
  // 数量缺失时退回单价本身（只记单价的场景）。
  const price = Number(data?.['单价'] ?? data?.['单价（元）'])
  const qty = Number(data?.['数量'])
  if (Number.isFinite(price) && price > 0) {
    const n = Number.isFinite(qty) && qty > 0 ? price * qty : price
    return Math.round(n * 100) / 100
  }
  return 0
}

/** 日期字段候选链（顺序即优先级）：取第一个非空日期。 */
const DATE_FIELD_PRIORITY = [
  '通行起始日期', '出险日期', '维修日期', '安装日期', '送检日期', '上次年检日期',
  '开始时间', '处理日期', '购置日期', '赔付日期', '缴款日期', '结案日期',
]

/** 从表单数据推导通用日期列（YYYY-MM-DD，空串表示没有可用日期）。 */
export function dateFromForm(data: Record<string, any>): string {
  for (const key of DATE_FIELD_PRIORITY) {
    const v = String(data?.[key] ?? '').trim()
    if (v) return v.slice(0, 10)
  }
  return ''
}

// ---------------------------------------------------------------------------
// KPI 卡片 / 默认焦点 / 空状态（第 30 轮：像素级对齐发票管理）
// ---------------------------------------------------------------------------

/** 一张 KPI 卡（结构对齐 DashboardKpiGroup 的 KpiCard，额外支持 static 纯展示卡）。 */
export interface FleetKpiCard {
  key: string
  label: string
  icon: string
  value: string
  unit?: string
  sub?: string
  theme?: 'brand' | 'success' | 'warning' | 'neutral'
  action: 'stat' | 'type' | 'history'
  typeValue?: string
  /** 纯展示卡：不响应点击、无 hover 指针、永不高亮 */
  static?: boolean
}

/**
 * 默认激活的分类名：取第 1 个启用分类。
 *
 * 为什么不再默认选中「已收录证照」汇总卡：各子分类字段集完全不同（许可证 10 个
 * 字段 vs 登记证书 35 个），汇总视图只能拼通用列且一半单元格是「—」。默认落到
 * 第一个具体分类，用户一进来就看到真实表头与数据。
 */
export function resolveDefaultCategoryName(cats: readonly any[] | undefined): string {
  for (const c of Array.isArray(cats) ? cats : []) {
    if (!c || c.enabled === false) continue
    const name = String(c.name || '').trim()
    if (name) return name
  }
  return ''
}

/**
 * 子分类别名（第 38 轮，需求 5.2）。
 *
 * 同一分类在库里可能以**英文 key** 落库（历史导入、外部接口写入、早期种子），
 * 而界面按中文名过滤 `sub_category` 时是精确匹配 —— 这些记录会静默消失
 * （真机：司机档案卡片显示「驾驶证 1 份」，列表却是「暂无数据」）。
 *
 * 这里给出「中文名 ↔ 英文 key」的规范映射；改名/新增分类时同步补一行即可。
 */
const SUB_CATEGORY_ALIASES: Record<string, readonly string[]> = {
  行驶证: ['vehicle_license', 'driving_license'],
  道路运输证: ['road_transport_certificate', 'transport_certificate'],
  道路运输经营许可证: ['road_transport_business_license', 'operation_license'],
  驾驶证: ['driver_license', 'driving_license'],
  从业资格证: ['qualification_certificate', 'employment_certificate'],
  维修工单: ['maintenance_order', 'repair_order'],
}

/** 某分类的全部可接受写法（中文名→英文 key，也支持反向传英文 key）。 */
export function subCategoryAliasesOf(name: string): string[] {
  const key = String(name || '').trim()
  if (!key) return []
  const out = new Set<string>()
  const direct = SUB_CATEGORY_ALIASES[key]
  if (direct) direct.forEach((a) => out.add(a))
  for (const [cn, list] of Object.entries(SUB_CATEGORY_ALIASES)) {
    if (!list.includes(key)) continue
    out.add(cn)
    list.forEach((a) => out.add(a))
  }
  out.delete(key)
  return [...out]
}

/** 记录的 sub_category 是否属于该分类（中文名或任一英文别名）。 */
export function subCategoryMatches(recordSub: string, categoryName: string): boolean {
  const r = String(recordSub || '').trim()
  const c = String(categoryName || '').trim()
  if (!r || !c) return false
  return r === c || subCategoryAliasesOf(c).includes(r)
}

/**
 * 构造车队 KPI 卡片（纯函数，便于单测）。
 *
 * 第 44 轮（需求图 1）：**删掉第 1 张「已收录证照 / 已收录记录」汇总卡**。
 * 这张卡只有总数，点不动、也没有筛选语义（与发票「已收录发票 = 全部」不同：
 * 车队列表本来就没有「全部」视图，默认焦点永远落在第一个具体分类上），
 * 三个菜单里它纯粹占掉一个卡片位，把真正的分类卡挤到右边。
 *
 * 现在：
 *   · 卡片组从第 1 个分类卡开始（如 车辆档案：道路运输经营许可证 / 道路运输证 / …）；
 *   · 档案类最右侧保留「历史记录」卡（上传历史抽屉的入口）；
 *   · 手工录入类（轮胎/年检/成本 7 项）没有上传链路 → 只有分类卡，没有历史卡。
 *
 * 文案口径对齐发票管理：子分类卡只给标题 + 数量，不显示占比；
 * 历史记录卡副文案 `无异常文件` / `N 份异常` / `正在执行 N`。
 */
export function buildFleetKpiCards(args: {
  isArchive: boolean
  /** 该菜单的记录总数。第 44 轮起不再产出汇总卡，保留参数仅为调用方兼容。 */
  total?: number
  categories: readonly any[]
  subCounts: Record<string, number>
  subAmounts: Record<string, number>
  singleCategory?: string
  parseFailed?: number
  extractFailed?: number
  /**
   * 「历史记录」卡片的数字 = 抽屉「全部文件」的数量（第 39 轮）。
   *
   * 旧实现复用 `total`（fleet_records 行数），于是卡片写 2 份、抽屉写 13 份 ——
   * 一个数两个口径，用户点开就发现对不上。两个数字现在由同一个文件集合算出来
   * （fleetHistoryStats.fleetHistoryFilesOf），卡片只是显示。
   */
  historyCount?: number
  /** 正在解析/提取的文件数：>0 时副文案改为「正在执行 N」。 */
  running?: number
}): FleetKpiCard[] {
  const { isArchive, categories, subCounts, singleCategory } = args
  const unit = isArchive ? '份' : '条'
  const cards: FleetKpiCard[] = []
  for (const c of Array.isArray(categories) ? categories : []) {
    const name = String(c?.name || '').trim()
    if (!name) continue
    if (singleCategory && name !== singleCategory) continue
    cards.push({
      key: `cat-${name}`,
      label: name,
      icon: 'file',
      value: String(Number(subCounts?.[name] || 0)),
      unit,
      sub: '',
      action: 'type',
      typeValue: `cat-${name}`,
    })
  }
  if (isArchive) {
    const failed = Number(args.parseFailed || 0) + Number(args.extractFailed || 0)
    const running = Number(args.running || 0)
    // 历史记录 = 已上传文件数（与抽屉「全部文件」同一集合），不是台账行数
    const historyCount = args.historyCount === undefined
      ? Number(args.total || 0)
      : Number(args.historyCount) || 0
    // 副文案优先级：在途 > 异常 > 正常（与抽屉顶部三个计数同序）
    const sub = running > 0 ? `正在执行 ${running}` : failed > 0 ? `${failed} 份异常` : '无异常文件'
    cards.push({
      key: 'history',
      label: '历史记录',
      icon: 'history',
      value: String(historyCount),
      unit: '份',
      sub,
      action: 'history',
    })
  }
  return cards
}

/**
 * 表格空状态文案（主文案 + 副文案）。
 *
 * 旧版按分类拼接「「车辆登记证书」暂无记录，可上传文件自动提取」，又长又重复；
 * 统一为「暂无数据」+ 一句操作指引，与发票管理的极简口径一致。
 */
export function fleetEmptyState(isArchive: boolean): { description: string; hint: string } {
  return {
    description: '暂无数据',
    hint: isArchive ? '上传文件后自动提取归档' : '点击右上角新增记录',
  }
}

// ---------------------------------------------------------------------------
// 证照状态三态（第 31 轮）
//
// 口径（需求原文）：剩余天数 > 30 → 生效中（绿）；0 ≤ 剩余 ≤ 30 → 即将到期（橙）；
// 剩余 < 0 → 已过期（红）；长期/永久/长期有效 → 生效中。
//
// 必须是**派生值**而不是可编辑字段：用户手填「已过期」永远不会随日期变化，
// 一年后整张表都是错的。抽屉里该字段只读展示，随「有效期止」实时联动。
// ---------------------------------------------------------------------------

/** 截止日期字段候选链（不同证照类型叫法不同）。 */
export const CERT_STATUS_FIELDS = [
  '有效期止', '有效期至', '保险止期', '检验有效期止', '质保期止',
  '截止日期', '到期日期', '有效期结束', '保险止期至',
  // 行驶证没有「有效期止」：它的合法性边界就是强制报废日期（票面唯一的时间锚点）。
  // 漏掉它会让整张行驶证列表的「证照状态」恒为 —。
  '强制报废日期', '报废日期',
] as const

/** 长期有效的常见写法（不受日期约束，直接算生效中）。 */
const LONG_TERM_HINTS = ['长期', '永久', '无固定期限', '长期有效', ' indefinite']

export interface CertStatus {
  /** '' 表示无可用日期（不渲染标签） */
  key: '' | 'active' | 'expiring' | 'expired'
  label: string
  theme: '' | 'success' | 'warning' | 'danger'
}

const EMPTY_STATUS: CertStatus = { key: '', label: '', theme: '' }

/** 解析为 UTC 日期零点；无法解析返回 null。 */
function parseDateOnly(raw: unknown): Date | null {
  const s = String(raw ?? '').trim()
  if (!s) return null
  // 统一分隔符：2027/01/01 → 2027-01-01；带时间戳只取日期段
  const norm = s.replace(/\//g, '-').replace(/[T\s].*$/, '')
  const m = norm.match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/)
  if (!m) return null
  const y = Number(m[1])
  const mo = Number(m[2])
  const d = Number(m[3])
  if (mo < 1 || mo > 12 || d < 1 || d > 31) return null
  const dt = new Date(Date.UTC(y, mo - 1, d))
  // 反向校验，挡住 2027-02-31 这类溢出日期
  if (dt.getUTCFullYear() !== y || dt.getUTCMonth() !== mo - 1 || dt.getUTCDate() !== d) return null
  return dt
}

function isLongTerm(raw: unknown): boolean {
  const s = String(raw ?? '').trim()
  if (!s) return false
  const lower = s.toLowerCase()
  return LONG_TERM_HINTS.some((h) => lower.includes(h.trim()))
}

/**
 * 由记录 data 推导证照状态。
 *
 * 多字段同时存在时取**最早**的截止日期：一份证照既有长期有效期又有已过期保险，
 * 真实风险是保险过期，不能因为长期字段就显示生效中。
 */
export function certStatusOf(data: Record<string, any> | undefined, today: string): CertStatus {
  const d = data || {}
  const todayDate = parseDateOnly(today)
  if (!todayDate) return EMPTY_STATUS

  let earliest: Date | null = null
  let sawLongTerm = false
  for (const key of CERT_STATUS_FIELDS) {
    const raw = d[key]
    if (raw === undefined || raw === null || String(raw).trim() === '') continue
    if (isLongTerm(raw)) { sawLongTerm = true; continue }
    const dt = parseDateOnly(raw)
    if (!dt) continue
    if (!earliest || dt.getTime() < earliest.getTime()) earliest = dt
  }

  if (earliest) {
    const days = Math.round((earliest.getTime() - todayDate.getTime()) / 86400000)
    if (days < 0) return { key: 'expired', label: '已过期', theme: 'danger' }
    if (days <= 30) return { key: 'expiring', label: '即将到期', theme: 'warning' }
    return { key: 'active', label: '生效中', theme: 'success' }
  }
  if (sawLongTerm) return { key: 'active', label: '生效中', theme: 'success' }
  return EMPTY_STATUS
}

/** 证照状态 → 筛选下拉选项（与列表 Tag 文案同源）。 */
export function certStatusOptions(): Array<{ label: string; value: string }> {
  return [
    { label: '生效中', value: 'active' },
    { label: '即将到期', value: 'expiring' },
    { label: '已过期', value: 'expired' },
  ]
}

/**
 * 字段列宽归一：0/空 = 自适应（沿用 columnWidth.ts 兜底），其余夹到 60~600。
 *
 * 与后端 subs[].width 同一语义；越界值（用户误输 9999）在这里夹住，
 * 避免单列把整张表撑爆导致横向滚动条永远存在。
 */
export function normalizeFieldWidth(v: unknown): number {
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) return 0
  return Math.min(600, Math.max(60, Math.round(n)))
}

/** 证照状态派生列的 key（列表用模板插槽 #__cert_status 渲染三色 Tag）。 */
export const CERT_STATUS_KEY = '__cert_status'
/** 提取状态派生列的 key（模板插槽 #__extract_status）。 */
export const EXTRACT_STATUS_KEY = '__extract_status'

/** 证照状态派生列（三色 Tag 走模板插槽）。 */
export function certStatusColumn(today: string): FleetColumn {
  return {
    key: CERT_STATUS_KEY,
    label: '证照状态',
    dataType: 'text',
    default: true,
    value: (row: any) => certStatusOf(row?.data, today).label || '—',
    raw: () => 0,
    // 需求图 5：表头曾被截断成「证照状」—— 三色 Tag 比纯文本更宽，
    // 必须给足 100px（Tag 四周 padding + 4 个汉字）。
    minWidth: 100,
  }
}

/** 提取状态派生列（模板插槽 #__extract_status）。 */
export function extractStatusColumn(): FleetColumn {
  return {
    key: EXTRACT_STATUS_KEY,
    label: '提取状态',
    dataType: 'text',
    default: true,
    value: (row: any) => extractStatusOf(row).label,
    raw: () => 0,
    minWidth: 100,
  }
}

/**
 * 档案类列表的列定义：业务列（subs 派生） + 两个系统状态列。
 *
 * 第 44 轮：状态字段在「字段定义」里可拖动排序（名称/类型/删除锁死，顺序不锁），
 * 所以派生列的位置必须**跟着 subs 走** —— 用户把「提取状态」拖到第 2 位，
 * 表头第 2 列就该是它。旧实现是「过滤掉 + 永远追加在末尾」，拖了等于没拖。
 *
 * 同一语义可能存在多个历史别名（证照状态 / 证件状态 / 保单状态 / 驾驶证状态），
 * 只取第一次出现的位置，避免同一列渲染两遍；subs 里完全没有时追加在末尾，
 * 保证任何分类都看得到状态列。
 */
export function buildArchiveColumns(
  subs: readonly any[],
  vehicleMap: Map<string, string>,
  today: string,
): FleetColumn[] {
  const all = buildFleetColumns(subs, vehicleMap)
  const out: FleetColumn[] = []
  const used = new Set<string>()
  let certAdded = false
  let extractAdded = false
  const pushCert = (): void => { if (!certAdded) { out.push(certStatusColumn(today)); certAdded = true } }
  const pushExtract = (): void => { if (!extractAdded) { out.push(extractStatusColumn()); extractAdded = true } }
  const byKey = new Map(all.map((c) => [c.key, c]))
  for (const s of Array.isArray(subs) ? subs : []) {
    const name = String(s?.name || '')
    if (isCertStatusName(name)) { pushCert(); continue }
    if (isExtractStatusName(name)) { pushExtract(); continue }
    const col = byKey.get(name)
    if (col && !used.has(name)) { used.add(name); out.push(col) }
  }
  // subs 顺序之外合成的列（buildFleetColumns 兜底产出的）按原顺序补在末尾
  for (const c of all) {
    if (used.has(c.key) || isStatusFieldName(c.key)) continue
    used.add(c.key)
    out.push(c)
  }
  if (!certAdded) pushCert()
  if (!extractAdded) pushExtract()
  return out
}

/** 提取状态（归档类）：记录能落库即代表提取成功。 */
export function extractStatusOf(row: any): { label: string; theme: 'success' | 'default' } {
  const hasData = !!(row && row.data && Object.keys(row.data).length > 0)
  const hasKid = !!fleetKnowledgeIdOf(row)
  return hasData || hasKid
    ? { label: '提取成功', theme: 'success' }
    : { label: '未提取', theme: 'default' }
}

/**
 * 台账记录 → 源文件（knowledge）ID（第 44 轮）。
 *
 * 档案类的列表读 `fleet_records`（台账），而「重新解析 / 重新提取」要的是
 * **知识库文件 ID**。两个 ID 不同、字段也不同名，历史上有过四种写法：
 *   · `doc_knowledge_id`：提取落库时写入的正式关联列（现网全部有值）；
 *   · `knowledge_id` / `source_knowledge_id`：外部导入与早期接口的写法；
 *   · `data._knowledge_id` / `data['源文件ID']`：第 39 轮回填时写进 JSONB 的副本。
 *
 * 旧实现只认 `row.__file`（文件行专用标记），台账行永远没有它 →
 * 勾选后「重新解析/重新提取」恒为禁用（真机图 2）。这里按候选链统一取，
 * 并过滤空白串（空 ID 打接口只会 500）。
 */
export function fleetKnowledgeIdOf(row: any): string {
  if (!row || typeof row !== 'object') return ''
  const candidates = [
    row.doc_knowledge_id,
    row.docKnowledgeId,
    row.knowledge_id,
    row.source_knowledge_id,
    row.knowledgeId,
    row.data?._knowledge_id,
    row.data?.knowledge_id,
    row.data?.['源文件ID'],
  ]
  for (const c of candidates) {
    const v = String(c ?? '').trim()
    if (v) return v
  }
  return ''
}
