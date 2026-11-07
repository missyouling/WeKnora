/**
 * 人事管理 API（第 45 轮）。前缀 `/api/v1/fleet/hr`。
 *
 * 全部经 `@/utils/request` 的 get/post/put/del/postUpload，继承 fl 组的
 * apiKey / RBAC / 租户 header / 401 刷新，不在本文件重复造请求实例。
 * 导入类接口走 multipart（postUpload，勿手动设 Content-Type，会丢 boundary），
 * 导出类接口走 getDown（整包 Blob，保留 token）。
 */

import { get, post, put, del, postUpload, getDown } from '@/utils/request'

const BASE = '/api/v1/fleet/hr'

/** 拼查询串（只写非空值，避免 `?q=undefined`）。 */
function qs(params: Record<string, any> = {}): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    p.set(k, Array.isArray(v) ? v.join(',') : String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

// ---------------------------------------------------------------------------
// 花名册 /roster
// ---------------------------------------------------------------------------

export function listRosterEmployees(params: {
  q?: string
  status?: string
  page?: number
  page_size?: number
} = {}): Promise<any> {
  return get(`${BASE}/roster/employees${qs(params)}`) as Promise<any>
}

export function createRosterEmployee(payload: Record<string, any>): Promise<any> {
  return post(`${BASE}/roster/employees`, payload) as Promise<any>
}

export function updateRosterEmployee(id: string | number, payload: Record<string, any>): Promise<any> {
  return put(`${BASE}/roster/employees/${encodeURIComponent(String(id))}`, payload) as Promise<any>
}

export function removeRosterEmployee(id: string | number): Promise<any> {
  return del(`${BASE}/roster/employees/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function batchDeleteRosterEmployees(ids: Array<string | number>): Promise<any> {
  return post(`${BASE}/roster/employees/batch-delete`, { ids }) as Promise<any>
}

export function resignRosterEmployees(
  ids: Array<string | number>,
  resign_date?: string,
  reason?: string,
): Promise<any> {
  return post(`${BASE}/roster/employees/resign`, { ids, resign_date, reason }) as Promise<any>
}

export function restoreRosterEmployees(ids: Array<string | number>): Promise<any> {
  return post(`${BASE}/roster/employees/restore`, { ids }) as Promise<any>
}

/** 花名册导入：multipart(file, mode, default_status, force_transition) → 409 {conflicts}。 */
export function importEmployees(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/roster/import`, formData) as Promise<any>
}

export function exportEmployees(ids?: Array<string | number>): Promise<any> {
  return getDown(`${BASE}/roster/export${qs({ ids })}`) as Promise<any>
}

export function getRosterStats(): Promise<any> {
  return get(`${BASE}/roster/stats`) as Promise<any>
}

// ---------------------------------------------------------------------------
// 社保 /social/periods
// ---------------------------------------------------------------------------

/** 账期列表：后端 ORDER BY year_month DESC，前端再用 sortMonthsDesc 兜底。 */
export function listSocialPeriods(): Promise<any> {
  return get(`${BASE}/social/periods`) as Promise<any>
}

export function createSocialPeriod(payload: Record<string, any>): Promise<any> {
  return post(`${BASE}/social/periods`, payload) as Promise<any>
}

export function removeSocialPeriod(id: string | number): Promise<any> {
  return del(`${BASE}/social/periods/${encodeURIComponent(String(id))}`) as Promise<any>
}

/** 五险导入：multipart(file, scheme, part, file_type)。 */
export function importSi(periodId: string | number, formData: FormData): Promise<any> {
  return postUpload(`${BASE}/social/periods/${encodeURIComponent(String(periodId))}/import`, formData) as Promise<any>
}

/** 执行汇总：聚合出个人 / 单位 charges。 */
export function processSocialPeriod(id: string | number): Promise<any> {
  return post(`${BASE}/social/periods/${encodeURIComponent(String(id))}/process`, {}) as Promise<any>
}

export function getSocialCharges(id: string | number, part?: string): Promise<any> {
  return get(`${BASE}/social/periods/${encodeURIComponent(String(id))}/charges${qs({ part })}`) as Promise<any>
}

export function exportSocialCharges(id: string | number, part?: string): Promise<any> {
  return getDown(`${BASE}/social/periods/${encodeURIComponent(String(id))}/export${qs({ part })}`) as Promise<any>
}

// 变更单 CRUD
export function listSocialChanges(id: string | number): Promise<any> {
  return get(`${BASE}/social/periods/${encodeURIComponent(String(id))}/changes`) as Promise<any>
}

export function createSocialChange(id: string | number, payload: Record<string, any>): Promise<any> {
  return post(`${BASE}/social/periods/${encodeURIComponent(String(id))}/changes`, payload) as Promise<any>
}

export function updateSocialChange(
  id: string | number,
  changeId: string | number,
  payload: Record<string, any>,
): Promise<any> {
  return put(
    `${BASE}/social/periods/${encodeURIComponent(String(id))}/changes/${encodeURIComponent(String(changeId))}`,
    payload,
  ) as Promise<any>
}

export function removeSocialChange(id: string | number, changeId: string | number): Promise<any> {
  return del(
    `${BASE}/social/periods/${encodeURIComponent(String(id))}/changes/${encodeURIComponent(String(changeId))}`,
  ) as Promise<any>
}

// ---------------------------------------------------------------------------
// 公积金 /fund
// ---------------------------------------------------------------------------

/** 单位默认配置：重庆星达铜业有限公司 / 201005128130 / 6。 */
export function getFundSettings(): Promise<any> {
  return get(`${BASE}/fund/settings`) as Promise<any>
}

export function updateFundSettings(payload: Record<string, any>): Promise<any> {
  return put(`${BASE}/fund/settings`, payload) as Promise<any>
}

export function listFundRecords(): Promise<any> {
  return get(`${BASE}/fund/records`) as Promise<any>
}

export function createFundRecord(payload: Record<string, any>): Promise<any> {
  return post(`${BASE}/fund/records`, payload) as Promise<any>
}

export function updateFundRecord(id: string | number, payload: Record<string, any>): Promise<any> {
  return put(`${BASE}/fund/records/${encodeURIComponent(String(id))}`, payload) as Promise<any>
}

export function removeFundRecord(id: string | number): Promise<any> {
  return del(`${BASE}/fund/records/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function sealFundRecord(id: string | number): Promise<any> {
  return post(`${BASE}/fund/records/${encodeURIComponent(String(id))}/seal`, {}) as Promise<any>
}

export function unsealFundRecord(id: string | number): Promise<any> {
  return post(`${BASE}/fund/records/${encodeURIComponent(String(id))}/unseal`, {}) as Promise<any>
}

export function importFund(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/fund/import`, formData) as Promise<any>
}

export function exportFund(ids?: Array<string | number>): Promise<any> {
  return getDown(`${BASE}/fund/export${qs({ ids })}`) as Promise<any>
}

/** 月度账单：后端 ORDER BY month_label DESC，前端再倒序兜底。 */
export function listFundBills(): Promise<any> {
  return get(`${BASE}/fund/bills`) as Promise<any>
}

export function createFundBill(payload: Record<string, any>): Promise<any> {
  return post(`${BASE}/fund/bills`, payload) as Promise<any>
}

export function getFundBill(id: string | number): Promise<any> {
  return get(`${BASE}/fund/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function removeFundBill(id: string | number): Promise<any> {
  return del(`${BASE}/fund/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}
