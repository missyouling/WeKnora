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
export function listFleetRecords(params: { type: string; month?: string; vehicle_id?: string }) {
  const query = new URLSearchParams()
  query.set('type', params.type)
  if (params.month) query.set('month', params.month)
  if (params.vehicle_id) query.set('vehicle_id', params.vehicle_id)
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
