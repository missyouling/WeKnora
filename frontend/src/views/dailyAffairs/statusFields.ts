// 系统内置状态字段的统一口径（第 44 轮）。
//
// 需求：`提取状态` / `证照状态` 这类「系统算出来的状态列」在「字段定义」里必须
// 永远存在、永远启用、字段名与数据类型不可改、不可删除，但允许拖动改列顺序、
// 允许调列宽 / 默认表头。
//
// 为什么需要单独一层：各模块的状态列来源并不相同 ——
//   · 车队档案的「证照状态」不是 subs 字段，而是列表按「有效期止」现算的派生列；
//   · 发票 / 合同 / 制度 / 奖惩的「提取状态」是真 subs 字段（extractStatus）；
//   · 市电 / 光伏账单的「状态」是文件级固定列，不在 subs 里。
// 但在「内置清单 / 锁定判定 / 自动补齐」这三件事上它们完全同构，故收口到这里，
// 避免每个页面各写一份 `name === '提取状态'` 的字符串比较。
//
// 铁律：这里是**唯一**的状态字段名单。新增模块只改 statusFieldSpecsOf，
// 不要在页面里再判断中文名。

/** 一个必须存在于字段定义里的状态字段。 */
export interface StatusFieldSpec {
  /** 字段身份（= subs 的 name；发票/能耗是英文键，车队是中文键）。 */
  name: string
  /** 表头文案。 */
  label: string
  /** 数据类型：状态列一律单行文本。 */
  dataType: string
  /**
   * 只在前端呈现、不写入 subs。
   *
   * 用于「列表状态列不由 subs 驱动」的模块（市电/光伏账单的状态列是文件级固定列）：
   * 落库只会让别的视图按 subs 再生成一条重复列，所以只补展示。
   */
  virtual?: boolean
}

/** 证照状态的各种历史写法（车队 subs 是中文键，个别老库落了英文键）。 */
const CERT_STATUS_NAMES = [
  '证照状态', '证件状态', '驾驶证状态', '保单状态', '证书状态',
  'certStatus', 'cert_status',
]
/** 提取状态：发票/合同/制度用英文契约键，车队用中文。 */
const EXTRACT_STATUS_NAMES = ['提取状态', 'extractStatus', 'extract_status']
/** 账单类「状态 / 对比状态」（市电 / 光伏）。 */
const BILL_STATUS_NAMES = ['状态', '对比状态']

const norm = (v: unknown): string => String(v ?? '').trim().toLowerCase()
const isOneOf = (name: unknown, list: readonly string[]): boolean => {
  const n = norm(name)
  if (!n) return false
  return list.some((x) => norm(x) === n)
}

/** 是否「证照状态」（含证件状态/驾驶证状态/保单状态等历史别名）。 */
export function isCertStatusName(name: unknown): boolean {
  return isOneOf(name, CERT_STATUS_NAMES)
}

/** 是否「提取状态」。 */
export function isExtractStatusName(name: unknown): boolean {
  return isOneOf(name, EXTRACT_STATUS_NAMES)
}

/** 是否「账单状态」（市电 / 光伏）。 */
export function isBillStatusName(name: unknown): boolean {
  return isOneOf(name, BILL_STATUS_NAMES)
}

/** 是否任一系统状态列（列表派生列替换/去重时用）。 */
export function isStatusFieldName(name: unknown): boolean {
  return isCertStatusName(name) || isExtractStatusName(name)
}

/** 车队三个「上传→解析→提取」档案 scope。 */
const FLEET_ARCHIVE_SCOPES = new Set(['vehicle', 'driver', 'maintain'])
/** 账单类分类名（只有这两类有文件级状态列）。 */
const BILL_CATEGORIES = new Set(['市电账单', '光伏账单'])

/** 某 scope（+ 分类）必须内置的状态字段清单。空数组 = 该模块没有状态列。 */
export function statusFieldSpecsOf(scope: string, categoryName?: string): StatusFieldSpec[] {
  const s = String(scope || '').trim()
  if (FLEET_ARCHIVE_SCOPES.has(s)) {
    // 车队档案：证照状态（按有效期算） + 提取状态（按知识库文件算），顺序与列表一致。
    return [
      { name: '证照状态', label: '证照状态', dataType: 'text' },
      { name: '提取状态', label: '提取状态', dataType: 'text' },
    ]
  }
  if (s === 'energy') {
    // 只有市电/光伏账单有文件级状态列；三张核算表与费用清单是纯手工核算，不注入。
    if (!BILL_CATEGORIES.has(String(categoryName || '').trim())) return []
    // 状态列是文件级固定列（不来自 subs）→ 只补展示，落库会与列表里的
    // extractStatus 列重名重复，故标 virtual。
    return [{ name: '提取状态', label: '提取状态', dataType: 'text', virtual: true }]
  }
  if (s === 'invoice' || s === 'contract' || s === 'regulation' || s === 'award_punish') {
    // 提取状态是这些模块的 subs 契约字段（extractStatus），缺失时补回并落库。
    return [{ name: 'extractStatus', label: '提取状态', dataType: 'text' }]
  }
  return []
}

/**
 * 该字段是否被系统锁定（名称/类型/启用/删除四项不可改）。
 *
 * 例外：列宽与「默认表头」始终可调，拖拽排序始终可用 —— 状态列在表头里的位置
 * 是用户正当的排版诉求。
 */
export function isLockedStatusField(scope: string, categoryName: string | undefined, name: unknown): boolean {
  const s = String(scope || '').trim()
  if (isExtractStatusName(name)) return true
  if (FLEET_ARCHIVE_SCOPES.has(s) && isCertStatusName(name)) return true
  if (s === 'energy' && BILL_CATEGORIES.has(String(categoryName || '').trim()) && isBillStatusName(name)) return true
  return false
}

/** 最小字段形态（只约束补齐时要用到的键，避免与 EditableField 循环依赖）。 */
interface StatusCarrier {
  name: string
  enabled?: boolean
  isDefault?: boolean
  dataType?: string
  builtin?: boolean
  virtual?: boolean
}

/**
 * 把缺失的状态字段补到字段列表末尾（就地修改并返回同一数组）。
 *
 * 口径：
 *   · 已存在（含历史别名，如「证件状态」）→ 只强制 enabled=true + builtin=true，
 *     不改名称、不改顺序、不覆盖用户列宽；
 *   · 缺失 → 追加一条 enabled 且默认展示的内置字段；
 *   · virtual 由调用方按 spec 决定是否落库。
 */
export function ensureStatusFields<T extends StatusCarrier>(fields: T[], specs: readonly StatusFieldSpec[]): T[] {
  if (!specs.length) return fields
  for (const spec of specs) {
    const hit = fields.find((f) => {
      const n = String(f?.name ?? '')
      if (n.trim().toLowerCase() === spec.name.toLowerCase()) return true
      // 同一语义的别名视为已存在（「证件状态」不需要再多一条「证照状态」）
      if (spec.name === '证照状态') return isCertStatusName(n)
      if (spec.name === '提取状态' || spec.name === 'extractStatus') return isExtractStatusName(n)
      return false
    })
    if (hit) {
      hit.enabled = true
      hit.builtin = true
      continue
    }
    fields.push({
      name: spec.name,
      enabled: true,
      isDefault: true,
      dataType: spec.dataType || 'text',
      // 列宽 0 = 自适应，与其它字段的默认值一致（输入框显示 0，不是空白）
      width: 0,
      builtin: true,
      ...(spec.virtual ? { virtual: true } : {}),
    } as unknown as T)
  }
  return fields
}
