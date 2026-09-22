import { get, post, put, del } from '@/utils/request'

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
}
export function getExtractConfig(kbId: string, scope: string, certType: string) {
  const q = new URLSearchParams({ scope, cert_type: certType })
  return get(`/api/v1/knowledge-bases/${kbId}/extract-config?${q.toString()}`)
}
export function saveExtractConfig(kbId: string, payload: Record<string, unknown>) {
  return post(`/api/v1/knowledge-bases/${kbId}/extract-config`, payload)
}
export function testExtractConfig(kbId: string, payload: Record<string, unknown>) {
  return post(`/api/v1/knowledge-bases/${kbId}/extract-config/test`, payload)
}
