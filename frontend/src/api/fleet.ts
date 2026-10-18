import { get, post, put, del, postUpload } from '@/utils/request'

// ---- 车辆 ----
export function listFleetVehicles() {
  return get('/api/v1/fleet/vehicles')
}
export function createFleetVehicle(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/vehicles', payload)
}
export function updateFleetVehicle(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/vehicles/${id}`, payload)
}
export function deleteFleetVehicle(id: string) {
  return del(`/api/v1/fleet/vehicles/${id}`)
}
export function sortFleetVehicles(ids: string[]) {
  return put('/api/v1/fleet/vehicles/sort', { ids })
}

// ---- 驾驶员 ----
export function listFleetDrivers() {
  return get('/api/v1/fleet/drivers')
}
export function createFleetDriver(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/drivers', payload)
}
export function updateFleetDriver(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/drivers/${id}`, payload)
}
export function deleteFleetDriver(id: string) {
  return del(`/api/v1/fleet/drivers/${id}`)
}
export function sortFleetDrivers(ids: string[]) {
  return put('/api/v1/fleet/drivers/sort', { ids })
}

// ---- 油卡 ----
export function listFleetFuelCards() {
  return get('/api/v1/fleet/fuel-cards')
}
export function createFleetFuelCard(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/fuel-cards', payload)
}
export function updateFleetFuelCard(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/fuel-cards/${id}`, payload)
}
export function deleteFleetFuelCard(id: string) {
  return del(`/api/v1/fleet/fuel-cards/${id}`)
}

// ---- 记录 ----
export function listFleetRecords(params: { type: string; month?: string; vehicle_id?: string; doc_type?: string }) {
  const query = new URLSearchParams()
  query.set('type', params.type)
  if (params.month) query.set('month', params.month)
  if (params.vehicle_id) query.set('vehicle_id', params.vehicle_id)
  if (params.doc_type) query.set('doc_type', params.doc_type)
  return get(`/api/v1/fleet/records?${query.toString()}`)
}
export function createFleetRecord(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/records', payload)
}
export function updateFleetRecord(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/records/${id}`, payload)
}
export function deleteFleetRecord(id: string) {
  return del(`/api/v1/fleet/records/${id}`)
}

// ---- 费用汇总 ----
export function getFleetSummary(params: { month: string; vehicle_id?: string }) {
  const query = new URLSearchParams()
  query.set('month', params.month)
  if (params.vehicle_id) query.set('vehicle_id', params.vehicle_id)
  return get(`/api/v1/fleet/summary?${query.toString()}`)
}

// ---- 档案分类（大项-小项）----
export function listFleetCategories(params: { scope: string }) {
  return get(`/api/v1/fleet/categories?scope=${params.scope}`)
}
export function createFleetCategory(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/categories', payload)
}
export function updateFleetCategory(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/categories/${id}`, payload)
}
export function deleteFleetCategory(id: string) {
  return del(`/api/v1/fleet/categories/${id}`)
}
export function sortFleetCategories(scope: string, ids: string[]) {
  return put('/api/v1/fleet/categories/sort', { scope, ids })
}

// ---- 证照分组 ----
export function listFleetCertGroups(params: { parent_scope: string }) {
  return get(`/api/v1/fleet/cert-groups?parent_scope=${params.parent_scope}`)
}
export function createFleetCertGroup(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/cert-groups', payload)
}
export function deleteFleetCertGroup(id: string) {
  return del(`/api/v1/fleet/cert-groups/${id}`)
}

export function listFleetGroupAliases() {
  return get('/api/v1/fleet/group-alias')
}
export function upsertFleetGroupAlias(payload: Record<string, unknown>) {
  return put('/api/v1/fleet/group-alias', payload)
}

// ---- 供应商 ----
export function listFleetSuppliers() {
  return get('/api/v1/fleet/suppliers')
}
export function createFleetSupplier(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/suppliers', payload)
}
export function updateFleetSupplier(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/suppliers/${id}`, payload)
}
export function deleteFleetSupplier(id: string) {
  return del(`/api/v1/fleet/suppliers/${id}`)
}

// ---- ETC 卡 ----
export function listFleetETCCards() {
  return get('/api/v1/fleet/etc-cards')
}export function createFleetETCCard(payload: Record<string, unknown>) {
  return post('/api/v1/fleet/etc-cards', payload)
}
export function updateFleetETCCard(id: string, payload: Record<string, unknown>) {
  return put(`/api/v1/fleet/etc-cards/${id}`, payload)
}
export function deleteFleetETCCard(id: string) {
  return del(`/api/v1/fleet/etc-cards/${id}`)
}

// ---- 可配置字段提取规则 ----
export interface ExtractFieldConfig {
  name: string
  label?: string
  desc?: string
  type?: string
  rule?: string
  enabled: boolean
  /** 必填：开启后该字段为空会产出 error 级校验告警（不影响落库） */
  required?: boolean
  /** 正则兜底：LLM 漏抽时从原文回填（Go RE2 语法；仅标准契约字段生效） */
  regex?: string
}
export function getExtractConfig(kbId: string, scope: string, certType: string) {
  const q = new URLSearchParams({ scope, cert_type: certType })
  return get(`/api/v1/knowledge-bases/${kbId}/extract-config?${q.toString()}`)
}
export function saveExtractConfig(kbId: string, payload: Record<string, unknown>) {
  return post(`/api/v1/knowledge-bases/${kbId}/extract-config`, payload)
}
export function testExtractConfig(kbId: string, payload: Record<string, unknown>, signal?: AbortSignal) {
  return post(`/api/v1/knowledge-bases/${kbId}/extract-config/test`, payload, { timeout: 120000, signal })
}

// ---- 智能配置（纯建议接口：只返回建议，绝不落库）----

/** 接口 A 返回的标准字段启用建议。 */
export interface AiSuggestStandardField {
  key: string
  label?: string
  data_type?: string
  enable: boolean
  /** 样本值。**必然是样票原文的逐字子串**（后端防幻觉校验，不命中即清空）。 */
  sample?: string
}

/** 接口 A 返回的明细子列建议（挂在 data_type=items 的字段上）。 */
export interface AiSuggestSubField {
  key: string
  label?: string
  data_type?: string
  width?: number
}

/** 接口 A 返回的新增自定义字段建议。 */
export interface AiSuggestCustomField {
  name: string
  data_type?: string
  sample?: string
  /** fields=落动态扩展桶；remark=并入备注片段（前端按此提示用户）。 */
  store?: 'fields' | 'remark'
  remark_part?: boolean
}

/** 被后端丢弃的一条建议及原因（前端要展示，不给静默失败）。 */
export interface AiSuggestSkip {
  name: string
  reason: string
}

export interface AiSuggestFieldsResult {
  cert_type?: string
  /** 解析后的样票纯文本（≤12000 字符），前端缓存后透传给接口 B。 */
  sample_text: string
  sample_text_truncated?: boolean
  standard_fields: AiSuggestStandardField[]
  item_sub_fields: AiSuggestSubField[]
  custom_fields: AiSuggestCustomField[]
  /** 被丢弃的建议（键不在白名单、子列键形态非法、与标准键同名等）。 */
  skipped?: AiSuggestSkip[]
}

/** 接口 B 的正则护栏判定结果；非 accepted 时 regex 必为空串。 */
export type AiRegexStatus =
  | 'accepted'
  | 'rejected_not_standard'
  | 'rejected_compile'
  | 'rejected_not_unique'
  | 'rejected_no_sample'
  | 'rejected_too_broad'
  | 'rejected_empty'

export interface AiSuggestRuleValue {
  name: string
  desc?: string
  rule?: string
  required?: boolean
  regex?: string
  regex_status: AiRegexStatus
  regex_reason?: string
}

export interface AiSuggestRulesResult {
  cert_type?: string
  fields: AiSuggestRuleValue[]
  prompt_template?: string
  /** 本次是否有实质变化（1/0；前端用于提示"是否有可应用的变更"）。 */
  applied_field_count?: number
  /** 是否带了样票：未带时正则会全部按 rejected_no_sample 丢弃。 */
  sample_text_used?: boolean
}

/**
 * 智能字段生成：上传样票（或直传文本），返回字段建议。
 *
 * 用 multipart 而非 JSON：样票可能是扫描 PDF，走 FormData 才能带上二进制。
 * 超时放宽到 180s：内部含解析（含可能的 VLM OCR）+ 一次 LLM 调用。
 */
export function aiSuggestFields(kbId: string, payload: {
  scope: string
  cert_type: string
  file?: File | null
  text?: string
}) {
  const form = new FormData()
  form.append('scope', payload.scope)
  form.append('cert_type', payload.cert_type)
  if (payload.file) form.append('file', payload.file)
  if (payload.text) form.append('text', payload.text)
  return postUpload(`/api/v1/knowledge-bases/${kbId}/extract-config/ai-suggest-fields`, form, undefined, { timeout: 180000 })
}

/**
 * 智能配置规则：按字段清单生成 desc/rule/required/regex + 高级 Prompt 模板。
 * overwrite=false（默认）只填空值字段，保护用户手写内容。
 */
export function aiSuggestRules(kbId: string, payload: {
  scope: string
  cert_type: string
  fields: Array<{ name: string; data_type?: string }>
  sample_text?: string
  overwrite?: boolean
}) {
  return post(`/api/v1/knowledge-bases/${kbId}/extract-config/ai-suggest-rules`, payload, { timeout: 180000 })
}

// ---- 证照提取 ----

/** 触发证照/发票提取。后端只认 cert_type（勿传 doc_type）。超时放宽到 120s。 */
export function extractFleetDocument(kbId: string, knowledgeId: string, payload: { scope: string; cert_type?: string }) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-fleet-document`, payload, { timeout: 120000 })
}

/** 发票提取统一入口（证照上传场景复用）。 */
export function extractBusinessDocument(kbId: string, knowledgeId: string, payload: { scope: string; cert_type?: string }) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-business`, payload, { timeout: 120000 })
}
