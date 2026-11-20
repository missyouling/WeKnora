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
  department?: string
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

/**
 * 办理离职（第 46 轮扩展）：前 3 个参数保持第 45 轮兼容，
 * 新增离职证明路径与备注说明，后端会同步写 custom_data.离职证明 / 离职备注。
 */
export function resignRosterEmployees(
  ids: Array<string | number>,
  resign_date?: string,
  reason?: string,
  proof_path?: string,
  remark?: string,
): Promise<any> {
  const body: Record<string, any> = { ids }
  if (resign_date) body.resign_date = resign_date
  if (reason) body.reason = reason
  if (proof_path) body.proof_path = proof_path
  if (remark) body.remark = remark
  return post(`${BASE}/roster/employees/resign`, body) as Promise<any>
}

export function restoreRosterEmployees(ids: Array<string | number>): Promise<any> {
  return post(`${BASE}/roster/employees/restore`, { ids }) as Promise<any>
}

/** 花名册导入：multipart(file, mode, default_status, force_transition) → 409 {conflicts}。 */
export function importEmployees(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/roster/import`, formData) as Promise<any>
}

/** 第 46 轮：下载《导入花名册模板.xlsx》（后端 excelize 生成 3 个 Sheet）。 */
export function getRosterTemplate(): Promise<any> {
  return getDown(`${BASE}/roster/template`) as Promise<any>
}

/**
 * 第 52 轮：三套独立 Excel 导入模板（花名册 / 离职管理 / 劳动合同）。
 * 每个模板都含「导入数据 / 说明 / 枚举字段」3 个 Sheet，枚举列配 Excel 数据验证。
 */
export type HrImportModule = 'roster' | 'resign' | 'contract' | 'social' | 'fund'

export function getHrTemplate(module: HrImportModule): Promise<any> {
  return getDown(`${BASE}/roster/template${qs({ module })}`) as Promise<any>
}

/** 按模块导入：花名册走 /roster/import，离职/合同走各自的专用解析器。 */
export function importHrModule(module: HrImportModule, formData: FormData): Promise<any> {
  const path = module === 'resign' ? `${BASE}/resign/import`
    : module === 'contract' ? `${BASE}/contract/import`
      // 第 54 轮：公积金走网厅解析器（支持 .xls / .xlsx，自动跳标题与验证码行）
      : module === 'fund' ? `${BASE}/fund/import`
        // 第 56 轮：社保回盘表走专用解析器（27 列，自动跳首行空行）
        : module === 'social' ? `${BASE}/social/receipts/import`
          : `${BASE}/roster/import`
  return postUpload(path, formData) as Promise<any>
}

/** 第 46 轮：上传离职证明，返回 {id, name, path}。 */
export function uploadResignProof(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/roster/resign-proof`, formData) as Promise<any>
}

/**
 * 第 46 轮：在线预览 / 下载离职证明（走 blob，保留 token）。
 * 第 61 轮：改走**无后缀**路由 `resign-proof-view?id=...` —— 生产 Nginx 的
 * 静态图片正则（\.png|\.jpg|\.pdf$）会把带扩展名的 API 路径当静态资源拦掉（404）。
 */
export function getResignProof(id: string): Promise<any> {
  return getDown(`${BASE}/roster/resign-proof-view${qs({ id })}`) as Promise<any>
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

/** 公积金台账：status 支持中文（正常 / 封存 / 提取销户），空 = 全部。 */
export function listFundRecords(status?: string): Promise<any> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : ''
  return get(`${BASE}/fund/records${qs}`) as Promise<any>
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

// ---------------------------------------------------------------------------
// 第 47 轮：社保双视图 / 劳动合同 / 组织机构
// ---------------------------------------------------------------------------

/** 社保原始记录（一行 = 某人某险种明细）。 */
export function listSocialRaw(params: { q?: string; year_month?: string; scheme?: string } = {}): Promise<any> {
  return get(`${BASE}/social/raw${qs(params)}`) as Promise<any>
}

/** 多文件自适应批量导入社保（正常 + 异常一起传）。 */
export function importSocialBatch(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/social/import`, formData) as Promise<any>
}

/** 勾选原始记录后按账期生成月度汇总（每月 1 条）。 */
export function generateSocialSummary(payload: { year_month?: string; ids?: Array<string | number> }): Promise<any> {
  return post(`${BASE}/social/summary`, payload) as Promise<any>
}

/** 月度费用清单抽屉数据（全员各险种明细 + 部门筛选项）。 */
export function getSocialPeriodDetail(id: string | number): Promise<any> {
  return get(`${BASE}/social/periods/${encodeURIComponent(String(id))}/detail`) as Promise<any>
}

/** 劳动合同视图（数据与花名册合同字段同源）。 */
export function listContracts(params: { q?: string; contract_status?: string; department?: string } = {}): Promise<any> {
  return get(`${BASE}/contracts${qs(params)}`) as Promise<any>
}

/** 组织机构（公司 / 部门 / 岗位）。 */
export function getOrganization(): Promise<any> {
  return get(`${BASE}/org`) as Promise<any>
}

export function saveOrganization(payload: Record<string, any>): Promise<any> {
  return put(`${BASE}/org`, payload) as Promise<any>
}

/** 第 48 轮：新增员工时的工号自动编码建议（A/B/C + 5 位部门码 + 4 位序号）。 */
export function nextEmployeeId(params: { department?: string; title?: string } = {}): Promise<any> {
  return get(`${BASE}/roster/next-employee-id${qs(params)}`) as Promise<any>
}

// ---------------------------------------------------------------------------
// 第 56 轮：社保四标签列表 / 账单 / 导入
// ---------------------------------------------------------------------------

export function listSocialReceipts(params: {
  q?: string; department?: string; month?: string; pension_status?: string
} = {}): Promise<any> {
  return get(`${BASE}/social/receipts${qs(params)}`) as Promise<any>
}

export function importSocialReceipts(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/social/receipts/import`, formData) as Promise<any>
}

/**
 * 第 66 轮：社保导入冲突预检。
 *
 * 后端按 `(bill_month, record_type, scheme, payer_side)` 切片统计：
 * 该月同类型下已有数据的切片会出现在 conflicts 里，前端据此弹确认框。
 */
export function checkSocialImport(form: FormData): Promise<any> {
  return postUpload(`${BASE}/social/records/import-check`, form) as Promise<any>
}

/** 第 65 轮：社保正常/补退记录批量删除（单次请求 + 单条汇总提示）。 */
export function batchDeleteSocialRecords(ids: string[]): Promise<any> {
  return post(`${BASE}/social/records/batch-delete`, { ids }) as Promise<any>
}

/** 第 65 轮：社保记录单条删除。 */
export function removeSocialRecord(id: string): Promise<any> {
  return del(`${BASE}/social/records/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function listSocialRecords(params: {
  record_type?: 'normal' | 'abnormal'; q?: string; department?: string; scheme?: string; month?: string
} = {}): Promise<any> {
  return get(`${BASE}/social/records${qs(params)}`) as Promise<any>
}

export function importSocialRecords(formData: FormData): Promise<any> {
  return postUpload(`${BASE}/social/records/import`, formData) as Promise<any>
}

/** 第 58 轮：正常 / 异常记录抽屉保存（金额与比例受控更新，合计由后端重算）。 */
export function updateSocialRecord(id: string, payload: Record<string, any>): Promise<any> {
  return put(`${BASE}/social/records/${encodeURIComponent(String(id))}`, payload) as Promise<any>
}

export function listSocialBills(): Promise<any> {
  return get(`${BASE}/social/bills`) as Promise<any>
}

export function createSocialBill(payload: { month_label: string; overwrite?: boolean }): Promise<any> {
  return post(`${BASE}/social/bills`, payload) as Promise<any>
}

/**
 * 第 60 轮：生成社保月度账单的前置校验（只读）。
 *
 * 返回 { normal_record_count, adjust_record_count, bill_exists }：
 * 正常记录为 0 时前端必须先 Toast 拦截（后端同样会拒绝），
 * bill_exists 为 true 时走覆盖二次确认。
 */
export function precheckSocialBill(month: string): Promise<any> {
  return get(`${BASE}/social/bills/precheck${qs({ month })}`) as Promise<any>
}

export function getSocialBill(id: string | number): Promise<any> {
  return get(`${BASE}/social/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function removeSocialBill(id: string | number): Promise<any> {
  return del(`${BASE}/social/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}

/** 第 49 轮：多单位组织机构（单位可无下属部门）。 */
export function getOrgUnits(): Promise<any> {
  return get(`${BASE}/org/units`) as Promise<any>
}

export function saveOrgUnits(payload: { units: any[] }): Promise<any> {
  return put(`${BASE}/org/units`, payload) as Promise<any>
}

// ---------------------------------------------------------------------------
// 第 61 轮：宿舍管理（五标签）
// ---------------------------------------------------------------------------

/** 宿舍记录：module = dorm / public / stay / asset。 */
export function listDormRecords(params: { module: string; q?: string }): Promise<any> {
  return get(`${BASE}/dorm/records${qs(params)}`) as Promise<any>
}

export function createDormRecord(module: string, data: Record<string, any>): Promise<any> {
  return post(`${BASE}/dorm/records`, { module, data }) as Promise<any>
}

export function updateDormRecord(id: string | number, data: Record<string, any>): Promise<any> {
  return put(`${BASE}/dorm/records/${encodeURIComponent(String(id))}`, { data }) as Promise<any>
}

export function removeDormRecord(id: string | number): Promise<any> {
  return del(`${BASE}/dorm/records/${encodeURIComponent(String(id))}`) as Promise<any>
}

/** 第 62 轮：子表专属删除（rooms / publics / stays / assets），严禁误调账单接口。 */
export function removeDormTyped(
  module: 'rooms' | 'publics' | 'stays' | 'assets',
  id: string | number,
): Promise<any> {
  return del(`${BASE}/dorm/${module}/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function importDormRecords(module: string, formData: FormData, mode = 'merge'): Promise<any> {
  return postUpload(
    `${BASE}/dorm/records/import?module=${encodeURIComponent(module)}&mode=${encodeURIComponent(mode)}`,
    formData,
  ) as Promise<any>
}

export function getDormTemplate(module: string): Promise<any> {
  return getDown(`${BASE}/dorm/template?module=${encodeURIComponent(module)}`) as Promise<any>
}

export function getDormStats(): Promise<any> {
  return get(`${BASE}/dorm/stats`) as Promise<any>
}

/** 住宿办理：调宿 / 退宿（批量）。 */
export function transferDormStays(payload: { ids: Array<string | number>; room: string; bed?: string }): Promise<any> {
  return post(`${BASE}/dorm/stays/transfer`, payload) as Promise<any>
}

export function checkoutDormStays(payload: { ids: Array<string | number>; date?: string; reason?: string }): Promise<any> {
  return post(`${BASE}/dorm/stays/checkout`, payload) as Promise<any>
}

/** 第 64 轮：恢复住宿（退宿可逆，后端重新校验床位与性别）。 */
export function restoreDormStay(id: string): Promise<any> {
  return post(`${BASE}/dorm/stays/${encodeURIComponent(String(id))}/restore`, {}) as Promise<any>
}

export function restoreDormStays(payload: { ids: Array<string | number> }): Promise<any> {
  return post(`${BASE}/dorm/stays/restore`, payload) as Promise<any>
}

/** 第 64 轮：费用信息月度账单汇总（每月 1 行）。 */
export function listDormChargeBills(): Promise<any> {
  return get(`${BASE}/dorm/charge-bills`) as Promise<any>
}

export function listDormBills(): Promise<any> {
  return get(`${BASE}/dorm/bills`) as Promise<any>
}

export function createDormBill(payload: { month_label: string; overwrite?: boolean }): Promise<any> {
  return post(`${BASE}/dorm/bills`, payload) as Promise<any>
}

export function getDormBill(id: string | number): Promise<any> {
  return get(`${BASE}/dorm/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}

export function removeDormBill(id: string | number): Promise<any> {
  return del(`${BASE}/dorm/bills/${encodeURIComponent(String(id))}`) as Promise<any>
}

// ---------------------------------------------------------------------------
// 第 62 轮：宿舍费用明细（全员月度扣款，跨模块聚合）
// ---------------------------------------------------------------------------

export function listDormCharges(params: {
  month?: string; department?: string; room_type?: string; q?: string
} = {}): Promise<any> {
  return get(`${BASE}/dorm/charges${qs(params)}`) as Promise<any>
}

export function generateDormCharges(payload: { month: string; overwrite?: boolean }): Promise<any> {
  return post(`${BASE}/dorm/charges/generate`, payload) as Promise<any>
}

export function updateDormCharge(id: string | number, payload: { other?: number; refund?: number; remark?: string }): Promise<any> {
  return put(`${BASE}/dorm/charges/${encodeURIComponent(String(id))}`, payload) as Promise<any>
}

export function removeDormCharge(id: string | number): Promise<any> {
  return del(`${BASE}/dorm/charges/${encodeURIComponent(String(id))}`) as Promise<any>
}
