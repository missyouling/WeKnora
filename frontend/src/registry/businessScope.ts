// BusinessScopeRegistry —— 业务 scope 的单一权威描述表（蓝图 P1-1）。
//
// 解决的问题：frontend 曾有 16 处 `scope === 'invoice'` 硬编码分支（分布在
// extractFieldMeta / FleetCertPanel / FleetUploadDialog / FleetUploadHistoryDrawer），
// 加一个业务要改 4 个文件；`notice` 与 `award_punish` 还因共用一个字典而互相污染
// 标签。现在每个 scope 的元信息（分类标签/空态文案/明细支持/正则可配/标准字段/
// 别名/列表形态）只在本 registry 声明一次，消费方一律 getScope()/labelOf()。
//
// 与后端 ScopeSchema（internal/types/extract_schema.go）的对应：本表是前端的
// 「同一份契约」镜像，键名经 canonicalFieldKey 归一后与后端 json tag 对齐。

export type ScopeKey =
  | 'invoice'
  | 'contract'
  | 'regulation'
  | 'notice'
  | 'award_punish'
  | 'vehicle'
  | 'driver'
  | 'maintain'
  | 'utility'
  | 'solar'

export type FieldDataType = 'text' | 'number' | 'date' | 'array'

export interface FieldMeta {
  /** 契约键（snake_case 或 camelCase，与后端 json tag 同源） */
  key: string
  label: string
  dataType: FieldDataType
  hint?: string
  aliases?: string[]
  /** 无标准存储位、数据在 remark 片段里的字段（发票专属能力） */
  remarkPart?: boolean
  subFields?: FieldMeta[]
}

export interface RegexPresetDef {
  name: string
  value: string
  hint?: string
}

export interface ScopeDescriptor {
  key: ScopeKey
  kind: 'invoice' | 'document' | 'archive'
  /** 分类轴名称：「发票类型 / 合同类型 / 制度名称」 */
  categoryLabel: string
  /** 分类个体的名词：「分类 / 维保类型 / 证照类型 / 合同」——用于「该X已存在」等提示 */
  categoryNoun: string
  /** 新增分类弹窗的输入占位 */
  namePlaceholder: string
  /** 分类列表空态文案 */
  emptyText: string
  /** 上传弹窗名词：「发票 / 合同 / 制度 / 公告 / 证照」——拼成「上传X」 */
  uploadNoun: string
  /** 上传弹窗的类型选择列标签（多数等于 categoryLabel，制度为「制度类型」） */
  uploadTypeLabel: string
  /** 上传弹窗的类型选择提示语；不填则按「必须选择${uploadTypeLabel}…」兜底 */
  uploadTypeTip?: string
  /** 列表末尾「新增」按钮文案；不填则用 `新增${分组名}` */
  addLabel?: string
  /** 明细子项浮层标题 */
  itemLabel: string
  /** 是否展示明细子项配置 */
  supportsSubFields: boolean
  /** 是否启用「备注片段当字段存」能力 */
  supportsRemarkParts: boolean
  /** 是否接入后置正则补洞（后端 Schema 的 RegexPatchable 镜像） */
  regexPatchable: boolean
  /** 标准契约字段（唯一真源；canonical 归一后参与 isStandardField 判定） */
  standardFields: FieldMeta[]
  /** 字段中文别名（中文 → 契约键），喂 AI 与规则面板 */
  aliases?: Record<string, string>
  /** 规则面板内置正则预设 */
  regexPresets?: RegexPresetDef[]
  /** 列表行形态：驱动列表接口选择与占位行策略 */
  listRows: 'invoices' | 'documents' | 'records'
  /** 提取接口通道：business=业务文档提取，fleet=车队档案提取 */
  api: { extract: 'business' | 'fleet' }
  /** 系统内置字段键（小写比对）：列表/筛选/同步的硬依赖，不可删除 */
  builtinFieldKeys?: string[]
  /** 跨分类公共规则底稿（规则面板；分类底稿 miss 时兜底） */
  commonRuleDrafts?: Record<string, { desc: string; rule: string }>
}

const registry = new Map<ScopeKey, ScopeDescriptor>()

/** 注册（或覆盖）一个 scope 的描述。测试与热修复用覆盖语义。 */
export function registerScope(d: ScopeDescriptor): void {
  registry.set(d.key, d)
}

/** 取 scope 描述；未注册返回 undefined（调用方按"无该业务"处理）。 */
export function getScope(scope: string): ScopeDescriptor | undefined {
  return registry.get(scope as ScopeKey)
}

/** 全部已注册 scope 的键（注册顺序）。 */
export function scopeKeys(): ScopeKey[] {
  return [...registry.keys()]
}

/** 归一化字段 key：小写 + 去掉非字母数字（invoiceNo / invoice_no → invoiceno）。 */
export function canonicalFieldKey(name: string): string {
  return String(name || '')
    .toLowerCase()
    .replace(/[^a-z0-9]/g, '')
}

/** 字段中文名：标准字段查 descriptor，未命中原样返回。 */
export function labelOf(scope: string, field: string): string {
  const d = getScope(scope)
  if (!d) return String(field ?? '')
  const want = canonicalFieldKey(field)
  for (const f of d.standardFields) {
    if (canonicalFieldKey(f.key) === want) return f.label
  }
  for (const [cn, key] of Object.entries(d.aliases || {})) {
    if (cn === field || canonicalFieldKey(key) === want) return cn
  }
  return String(field ?? '')
}

/** 是否标准契约字段（未注册 scope / 自定义中文 key 返回 false）。 */
export function isStandardFieldOf(scope: string, name: string): boolean {
  const d = getScope(scope)
  if (!d) return false
  const want = canonicalFieldKey(name)
  if (!want) return false
  return d.standardFields.some((f) => canonicalFieldKey(f.key) === want)
}

/**
 * 该 scope 是否有字段字典：有字典的 scope 字段名只读（展示中文名），
 * 无字典的（车队档案 / 账单）字段名由用户自由输入。
 */
export function hasFieldDict(scope: string): boolean {
  return (getScope(scope)?.standardFields.length ?? 0) > 0
}

/** 是否系统内置字段（不可删除）。未声明 builtinFieldKeys 的 scope 一律返回 false。 */
export function isBuiltinFieldOf(scope: string, name: string): boolean {
  const keys = getScope(scope)?.builtinFieldKeys
  if (!keys?.length) return false
  const want = String(name || '').toLowerCase()
  return keys.some((k) => k.toLowerCase() === want)
}
