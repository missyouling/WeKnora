// 提取字段的「元信息」：哪些是系统标准契约键、显示成什么中文名、内置正则预设。
//
// 为什么单独成模块：
//   ExtractRulePanel（规则面板）、ExtractQualityPanel（质量徽章）、六个业务抽屉
//   都要把字段 key 翻成中文、都要判断"这个字段是不是标准契约键"。三处各写一份
//   必然漂移，尤其"自定义扩展中文 key"的判定——它是正则兜底的硬边界。
//
// 与后端的对应关系：
//   后端 `standardDateFields` / `invoiceFieldAccessors` 用 snake_case，前端配置用
//   camelCase（invoiceNo / signDate）。两边靠 canonicalFieldKey 归一后比较，
//   所以这里判断标准键也走 canonical，不做字面前缀匹配。

import { INVOICE_FIELD_LABELS } from './invoiceFieldLabels'
import { CONTRACT_FIELD_LABELS } from './contractFieldLabels'
import { REGULATION_FIELD_LABELS } from './regulationFieldLabels'
import { NOTICE_FIELD_LABELS } from './noticeFieldLabels'

/** 归一化字段 key：小写 + 去掉非字母数字（invoiceNo / invoice_no / InvoiceNO → invoiceno） */
export function canonicalFieldKey(name: string): string {
  return String(name || '')
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
}

// 各业务**结构体内置**的标准契约键（即后端校验器/补洞器认识的字段）。
// 不在这里的中文 key 属于用户自定义扩展字段：只注入 Prompt，不参与后置正则补洞。
const STANDARD_KEYS: Record<string, string[]> = {
  invoice: [
    ...Object.keys(INVOICE_FIELD_LABELS),
    // 后端发票契约里存在但中文映射表未列出的键（补洞/必填会用到）
    'invoice_no',
    'invoice_code',
    'invoice_date',
    'buyer_tax_no',
    'seller_tax_no',
    'buyer_name',
    'seller_name',
    'tax_amount',
  ],
  contract: [...Object.keys(CONTRACT_FIELD_LABELS), 'sign_date', 'effective_date', 'expiry_date'],
  regulation: [...Object.keys(REGULATION_FIELD_LABELS), 'reg_no', 'issue_date'],
  award_punish: [...Object.keys(NOTICE_FIELD_LABELS), 'ap_no', 'issue_date', 'sign_date'],
}

const STANDARD_KEY_SETS: Record<string, Set<string>> = Object.fromEntries(
  Object.entries(STANDARD_KEYS).map(([scope, keys]) => [scope, new Set(keys.map(canonicalFieldKey))]),
)

/** 该 scope 是否已接入字段级校验/补洞（决定 required 与 regex 是否可用） */
export function fieldRulesSupported(scope: string): boolean {
  return !!STANDARD_KEY_SETS[scope]
}

/** 是否标准契约字段（自定义扩展中文 key 返回 false） */
export function isStandardField(scope: string, name: string): boolean {
  const set = STANDARD_KEY_SETS[scope]
  if (!set) return false
  return set.has(canonicalFieldKey(name))
}

// ---- 正则可配白名单（与后端「补洞白名单」严格对齐）----
//
// 这是一处**已知的历史落差**，必须显式收窄：
//   - `isStandardField('invoice', …)` 认22 个发票标准键（用于 required 判定，
//     因为通用校验器 ValidateGenericExtraction 对全部标准键做日期归一与必填告警）；
//   - 但发票**正式提取链路**的后置正则补洞只认 8 个
//     （后端 `internal/application/service/extract_regex_fallback.go` 的
//     `invoiceFieldOrder`，即 invoiceFieldAccessors 的全部键）。
//
// 此前 UI 用同一把尺子判"正则可配"，用户给 totalAmount / taxRate 配了正则，
// 正式提取时根本不执行——静默失效比直接置灰更糟（用户以为配了就生效）。
// 现在把两把尺子拆开：required 走 isStandardField，regex 走 isRegexPatchable。
// 新增可补洞字段时必须同时改后端 invoiceFieldOrder，否则会出现"配了不执行"。

/** 后端发票补洞真正支持的标准键（snake_case 契约键，顺序与后端一致）。 */
export const REGEX_PATCHABLE_KEYS = [
  'invoice_no',
  'invoice_code',
  'invoice_date',
  'buyer_name',
  'buyer_tax_no',
  'seller_name',
  'seller_tax_no',
  'issuer',
]

const REGEX_PATCHABLE_SET = new Set(REGEX_PATCHABLE_KEYS.map(canonicalFieldKey))

/**
 * 该字段能否配置后置正则兜底。
 *
 * 判定口径 = 后端补洞器实际支持集合（当前仅发票 8 个键）。
 * 其余 scope（合同/制度/通知）后端尚未接入字段级补洞，一律 false。
 * 大小写与下划线差异由 canonicalFieldKey 归一（invoiceNo ≡ invoice_no）。
 */
export function isRegexPatchable(scope: string, name: string): boolean {
  if (scope !== 'invoice') return false
  const key = canonicalFieldKey(name)
  if (!key) return false
  return REGEX_PATCHABLE_SET.has(key)
}

/** 正则输入被置灰的原因（Tooltip 文案）。空串代表可配。 */
export function regexDisabledReason(scope: string, name: string): string {
  if (scope !== 'invoice') return '本模块暂未接入后置正则补洞（当前仅发票生效）'
  if (isRegexPatchable(scope, name)) return ''
  if (isStandardField(scope, name)) {
    return '该字段是标准字段，但后端补洞器只支持发票号/发票代码/开票日期/购销方名称/购销方税号/开票人这 8 个字段，配置后正式提取不会执行'
  }
  return '自定义扩展字段仅注入 AI 提取 Prompt，后端后置正则补洞仅对系统标准字段生效'
}

const LABELERS: Record<string, (name: string) => string> = {
  invoice: (n) => INVOICE_FIELD_LABELS[n] || n,
  contract: (n) => CONTRACT_FIELD_LABELS[n] || n,
  regulation: (n) => REGULATION_FIELD_LABELS[n] || n,
  award_punish: (n) => NOTICE_FIELD_LABELS[n] || n,
}

/** 字段中文名（无映射时原样返回；车队等 scope 的字段本来就是中文） */
export function extractFieldLabel(scope: string, name: string): string {
  return (LABELERS[scope] || ((n: string) => n))(name)
}

export interface RegexPreset {
  name: string
  value: string
  /** 适用字段提示，仅用于展示 */
  hint: string
}

/**
 * 常用内置正则（Go RE2 兼容：只用字符类/量词/边界，不使用环视与反向引用）。
 * 这些是"形状足够唯一"的兜底，不要往里加会误命中的宽松表达式。
 */
export const REGEX_PRESETS: RegexPreset[] = [
  {
    name: '20 位全电发票号',
    value: '\\b\\d{20}\\b',
    hint: '全电发票号码，形状唯一',
  },
  {
    name: '18 位统一社会信用代码',
    value: '\\b[0-9A-HJ-NPQRTUWXY]{2}\\d{6}[0-9A-HJ-NPQRTUWXY]{10}\\b',
    hint: '税号 / 信用代码（不含 I O S V Z）',
  },
  {
    name: '标准日期',
    value: '\\d{4}[-/年]\\d{1,2}[-/月]\\d{1,2}日?',
    hint: '2026-09-02 / 2026/9/2 / 2026年9月2日',
  },
  {
    name: '金额（含千分位）',
    value: '\\d{1,3}(?:,\\d{3})*\\.\\d{2}',
    hint: '1,234.56 这类票面金额',
  },
]

/**
 * 轻量正则合法性检查（返回错误文案，空串代表通过）。
 *
 * 用 JS 引擎做**语法预检**：RE2 是 JS 的子集（除命名分组语法），JS 能编译的绝大多数
 * 在 RE2 也合法；反之报错多半是真错。这里只做提示，不阻断保存——RE2 独有语法
 * （如 `(?P<name>…)`）在 JS 会误报，不该因此禁止用户使用。
 */
export function regexSyntaxHint(pattern: string): string {
  const p = String(pattern || '').trim()
  if (!p) return ''
  try {
    new RegExp(p)
    return ''
  } catch {
    return '正则可能不合法（括号/转义未闭合），保存后补洞会跳过该字段'
  }
}
