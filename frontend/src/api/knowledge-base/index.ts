import { get, post, put, del, postUpload, getDown } from "../../utils/request";
import type { KnowledgeProcessOverrides } from '@/types/knowledgeProcess';
import type { AuditLog, AuditOutcome, ListAuditLogResponse } from '@/api/tenant/audit-log';

export type KnowledgeBaseActivity = AuditLog;

export interface ListKnowledgeBaseActivityParams {
  after_id?: number;
  limit?: number;
  action?: string;
  outcome?: AuditOutcome;
  actor?: string;
}

export async function listKnowledgeBaseActivity(
  id: string,
  params: ListKnowledgeBaseActivityParams = {},
): Promise<ListAuditLogResponse> {
  const query = new URLSearchParams();
  if (params.after_id) query.set('after_id', String(params.after_id));
  if (params.limit) query.set('limit', String(params.limit));
  if (params.action) query.set('action', params.action);
  if (params.outcome) query.set('outcome', params.outcome);
  if (params.actor) query.set('actor', params.actor);
  const qs = query.toString();
  return (await get(`/api/v1/knowledge-bases/${id}/activity${qs ? `?${qs}` : ''}`)) as unknown as ListAuditLogResponse;
}

// 知识库管理 API（列表、创建、获取、更新、删除、复制）
export function listKnowledgeBases(params?: {
  agent_id?: string;
  agent_source_tenant_id?: string;
  /**
   * Optional creator filter. Server-side semantics:
   *   - "mine"   → only KBs whose creator_id matches the caller
   *   - "others" → only KBs created by someone else in this tenant
   *   - omitted/"all" → no filter
   * KBs predating the RBAC backfill (creator_id="") never match
   * mine/others — they fall out of both views by design.
   */
  creator?: 'all' | 'mine' | 'others';
}) {
  const query = new URLSearchParams();
  if (params?.agent_id) query.set('agent_id', params.agent_id);
  if (params?.agent_source_tenant_id) query.set('agent_source_tenant_id', params.agent_source_tenant_id);
  if (params?.creator && params.creator !== 'all') query.set('creator', params.creator);
  const qs = query.toString();
  return get(qs ? `/api/v1/knowledge-bases?${qs}` : '/api/v1/knowledge-bases');
}

// Read-only vector-store binding metadata enriched onto every KB
// response (list, create, get, update, pin). Source carries where the
// binding points; status reports whether that target is currently
// reachable by the server.
//
//   - source 'env'    → KB uses the tenant's env-configured store
//                       (RETRIEVE_DRIVER). vector_store_id is null and
//                       vector_store_name is the localized "System
//                       default" label; vector_store_engine_type still
//                       reports the underlying engine (e.g. "postgres").
//   - source 'user'   → KB is bound to a tenant-owned VectorStore.
//                       vector_store_id / name / engine_type are real.
//   - source 'shared' → KB belongs to a different tenant and is
//                       readable via cross-organization sharing. The
//                       server strips vector_store_id and engine_type
//                       to avoid leaking the owner tenant's store
//                       inventory; only this source marker arrives.
//   - status 'unavailable' → the binding cannot be reached right now
//                       (deleted row, registry miss, transient infra
//                       failure). Operators recover via the global
//                       Vector Stores settings page.
export type VectorStoreSource = 'env' | 'user' | 'shared' | 'unavailable';
export type VectorStoreStatus = 'available' | 'unavailable';

export interface KnowledgeBaseStoreView {
  vector_store_id?: string | null;
  vector_store_name?: string;
  vector_store_engine_type?: string;
  vector_store_source?: VectorStoreSource;
  vector_store_status?: VectorStoreStatus;
}

export function createKnowledgeBase(data: {
  name: string;
  description?: string;
  type?: 'document' | 'faq';
  chunking_config?: any;
  embedding_model_id?: string;
  summary_model_id?: string;
  auto_tag_config?: { enabled: boolean; model_id?: string; max_tags?: number; skip_if_tagged?: boolean };
  // Opt-in binding to a specific tenant-owned VectorStore. Omit (or
  // send undefined / empty string) to fall back to the env-configured
  // store. Immutable after creation — UpdateKnowledgeBase intentionally
  // does not accept this field.
  vector_store_id?: string;
  // Concrete tenant-owned storage instance. When omitted, the tenant default
  // backend is bound by the server at creation time.
  storage_backend_id?: string;
  vlm_config?: {
    enabled: boolean;
    model_id?: string;
    description_language?: string;
    custom_instructions?: string;
  };
  storage_provider_config?: { provider: string };
  storage_config?: any; // legacy, kept for backward compat (dual-write)
  asr_config?: {
    enabled: boolean;
    model_id?: string;
    language?: string;
  };
  extract_config?: any;
  faq_config?: { index_mode: string; question_index_mode?: string };
  wiki_config?: {
    synthesis_model_id?: string;
    max_pages_per_ingest?: number;
    extraction_granularity?: 'focused' | 'standard' | 'exhaustive';
    content_instructions?: string;
    extraction_instructions?: string;
  };
  indexing_strategy?: {
    vector_enabled: boolean;
    keyword_enabled: boolean;
    wiki_enabled: boolean;
    graph_enabled: boolean;
  };
}) {
  return post(`/api/v1/knowledge-bases`, data);
}

export function getKnowledgeBaseById(id: string, options?: { agent_id?: string; agent_source_tenant_id?: string }) {
  const query = new URLSearchParams();
  if (options?.agent_id) query.set('agent_id', options.agent_id);
  if (options?.agent_source_tenant_id) query.set('agent_source_tenant_id', options.agent_source_tenant_id);
  const qs = query.toString();
  return get(qs ? `/api/v1/knowledge-bases/${id}?${qs}` : `/api/v1/knowledge-bases/${id}`);
}

export function updateKnowledgeBase(id: string, data: {
  name: string;
  description?: string;
  config?: {
    chunking_config?: any;
    image_processing_config?: any;
    faq_config?: any;
    wiki_config?: {
      synthesis_model_id?: string;
      max_pages_per_ingest?: number;
      extraction_granularity?: 'focused' | 'standard' | 'exhaustive';
      content_instructions?: string;
      extraction_instructions?: string;
    };
    auto_tag_config?: { enabled: boolean; model_id?: string; max_tags?: number; skip_if_tagged?: boolean };
    indexing_strategy?: {
      vector_enabled: boolean;
      keyword_enabled: boolean;
      wiki_enabled: boolean;
      graph_enabled: boolean;
    };
  }
}) {
  return put(`/api/v1/knowledge-bases/${id}`, data);
}

export function rebuildKBIndex(kbId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/rebuild-index`, {});
}

export function deleteKnowledgeBase(id: string) {
  return del(`/api/v1/knowledge-bases/${id}`);
}

export function copyKnowledgeBase(data: { source_id: string; target_id?: string }) {
  return post(`/api/v1/knowledge-bases/copy`, data);
}

export function duplicateKnowledgeBase(id: string) {
  return post(`/api/v1/knowledge-bases/${id}/duplicate`);
}

// 获取可移动目标知识库列表（同类型、同Embedding模型）
export function listMoveTargets(sourceKbId: string) {
  return get(`/api/v1/knowledge-bases/${sourceKbId}/move-targets`);
}

// 移动知识到其他知识库
export function moveKnowledge(data: {
  knowledge_ids: string[];
  source_kb_id: string;
  target_kb_id: string;
  mode: 'reuse_vectors' | 'reparse';
}) {
  return post('/api/v1/knowledge/move', data);
}

// 获取知识移动进度
export function getKnowledgeMoveProgress(taskId: string) {
  return get(`/api/v1/knowledge/move/progress/${taskId}`);
}

export function togglePinKnowledgeBase(id: string) {
  return put(`/api/v1/knowledge-bases/${id}/pin`);
}

// 知识文件 API（基于具体知识库）
// data.tag_ids: 可选，指定知识所属的多个标签 ID
export function uploadKnowledgeFile(
  kbId: string,
  data: {
    file: File
    tag_ids?: string[]
    fileName?: string
    process_config?: KnowledgeProcessOverrides | string
    [key: string]: any
  } = { file: new File([], '') },
  onProgress?: (progressEvent: any) => void,
) {
  const formData = new FormData();
  Object.keys(data).forEach(key => {
    const value = data[key];
    if (value === undefined) return;
    if (key === 'tag_ids' && Array.isArray(value)) {
      formData.append(key, value.join(','));
    } else if (key === 'process_config' && value && typeof value !== 'string') {
      formData.append(key, JSON.stringify(value));
    } else {
      formData.append(key, value);
    }
  });
  return postUpload(`/api/v1/knowledge-bases/${kbId}/knowledge/file`, formData, onProgress);
}

// 从URL创建知识
// data.tag_ids: 可选，指定知识所属的多个标签 ID
export function createKnowledgeFromURL(
  kbId: string,
  data: { url: string; enable_multimodel?: boolean; tag_ids?: string[]; process_config?: KnowledgeProcessOverrides },
) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/url`, data);
}

// 手工创建知识
// data.tag_ids: 可选，指定知识所属的标签 ID
export function createManualKnowledge(
  kbId: string,
  data: {
    title: string
    content: string
    status: string
    tag_ids?: string[]
    process_config?: KnowledgeProcessOverrides
  },
) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/manual`, data);
}

export function listKnowledgeFiles(
  kbId: string,
  params: {
    page: number;
    page_size: number;
    tag_ids?: string;
    keyword?: string;
    file_type?: string;
    parse_status?: string;
    source?: string;
    start_time?: string;
    end_time?: string;
    /**
     * Folder to browse. An empty string means the knowledge base root, so the
     * parameter is only sent when it is defined — leaving it out lists every
     * folder (the flat view).
     */
    folder_path?: string;
    /** Include documents stored in sub-folders of folder_path. */
    folder_recursive?: boolean;
  },
) {
  const query = new URLSearchParams();
  query.append('page', String(params.page));
  query.append('page_size', String(params.page_size));
  if (params.tag_ids) query.append('tag_ids', params.tag_ids);
  if (params.keyword) query.append('keyword', params.keyword);
  if (params.file_type) query.append('file_type', params.file_type);
  if (params.parse_status) query.append('parse_status', params.parse_status);
  if (params.source) query.append('source', params.source);
  if (params.start_time) query.append('start_time', params.start_time);
  if (params.end_time) query.append('end_time', params.end_time);
  if (params.folder_path !== undefined) {
    query.append('folder_path', params.folder_path);
    if (params.folder_recursive) query.append('folder_recursive', 'true');
  }
  const qs = query.toString();
  return get(`/api/v1/knowledge-bases/${kbId}/knowledge?${qs}`);
}

/** One node of the knowledge base folder tree. */
export interface KnowledgeFolderNode {
  /** Canonical folder path, e.g. "docs/spec". */
  path: string;
  /** Last segment of the path, used as the row label. */
  name: string;
  /** Documents stored directly in this folder. */
  document_count: number;
  /** Documents in this folder plus every descendant folder. */
  total_count: number;
  children?: KnowledgeFolderNode[];
}

export interface KnowledgeFolderTree {
  /** Documents that are not part of any uploaded folder. */
  root_document_count: number;
  /** Documents in the whole knowledge base. */
  total_document_count: number;
  folders: KnowledgeFolderNode[];
}

export function listKnowledgeFolders(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/knowledge/folders`);
}

/**
 * Re-file documents under `folderPath` ('' = knowledge base top level). Folders
 * are derived from the stored paths, so a path that does not exist yet is
 * created by this call. Only the grouping changes; documents are not re-parsed.
 */
export function moveKnowledgeToFolder(kbId: string, ids: string[], folderPath: string) {
  return post('/api/v1/knowledge/folder', {
    kb_id: kbId,
    knowledge_ids: ids,
    folder_path: folderPath,
  });
}

/** Rename or move a folder together with everything below it. */
export function renameKnowledgeFolder(kbId: string, from: string, to: string) {
  return put(`/api/v1/knowledge-bases/${kbId}/knowledge/folders`, { from, to });
}

export function getKnowledgeDetails(id: string, options?: { agent_id?: string; agent_source_tenant_id?: string }) {
  const query = new URLSearchParams();
  if (options?.agent_id) query.set('agent_id', options.agent_id);
  if (options?.agent_source_tenant_id) query.set('agent_source_tenant_id', options.agent_source_tenant_id);
  const qs = query.toString();
  return get(qs ? `/api/v1/knowledge/${id}?${qs}` : `/api/v1/knowledge/${id}`);
}

export function updateManualKnowledge(
  id: string,
  data: { title: string; content: string; status: string; process_config?: KnowledgeProcessOverrides },
) {
  return put(`/api/v1/knowledge/manual/${id}`, data);
}

export function reparseKnowledge(id: string, data?: { process_config?: KnowledgeProcessOverrides }) {
  return post(`/api/v1/knowledge/${id}/reparse`, data);
}

export function cancelKnowledgeParse(id: string) {
  return post(`/api/v1/knowledge/${id}/cancel-parse`);
}

export function getKnowledgeSpans(id: string, attempt?: number) {
  const qs = attempt ? `?attempt=${attempt}` : '';
  return get(`/api/v1/knowledge/${id}/spans${qs}`);
}

export function delKnowledgeDetails(id: string) {
  return del(`/api/v1/knowledge/${id}`);
}

// 批量删除（同一知识库内）。后端会校验所有 id 隶属于 kb_id 且具有编辑权限。
export function batchDeleteKnowledge(kbId: string, ids: string[]) {
  return post(`/api/v1/knowledge/batch-delete`, { kb_id: kbId, ids });
}

export function downKnowledgeDetails(id: string) {
  return getDown(`/api/v1/knowledge/${id}/download`);
}

export function previewKnowledgeFile(id: string) {
  return getDown(`/api/v1/knowledge/${id}/preview`);
}

/** 删除历史预览：读取被系统自动删除（软删）但仍保留源文件的记录。
 * 必须走 KB 作用域路由（URL 携带 KB id）——/knowledge/:id/preview 的知识解析
 * 中间件对软删记录（deleted_at IS NOT NULL）必然 404。 */
export function previewDeletedKnowledgeFile(kbId: string, id: string) {
  return getDown(`/api/v1/knowledge-bases/${kbId}/knowledge/${id}/preview-deleted`);
}

/** @param idsQueryString - query string with ids (e.g. ids=xxx&ids=yyy) */
export function batchQueryKnowledge(idsQueryString: string, kbId?: string, agentId?: string, agentSourceTenantId?: string) {
  let qs = idsQueryString;
  if (kbId) qs += `&kb_id=${encodeURIComponent(kbId)}`;
  if (agentId) qs += `&agent_id=${encodeURIComponent(agentId)}`;
  if (agentSourceTenantId) qs += `&agent_source_tenant_id=${encodeURIComponent(agentSourceTenantId)}`;
  return get(`/api/v1/knowledge/batch?${qs}`);
}

export const KNOWLEDGE_CHUNK_PAGE_SIZE = 25;

export function getKnowledgeDetailsCon(id: string, page: number) {
  return get(`/api/v1/chunks/${id}?page=${page}&page_size=${KNOWLEDGE_CHUNK_PAGE_SIZE}`);
}

export interface ChunkEditPayload {
  content?: string;
  is_enabled?: boolean;
  expected_revision?: number;
}

export function updateDocumentChunk(knowledgeId: string, chunkId: string, data: ChunkEditPayload) {
  return put(`/api/v1/chunks/${knowledgeId}/${chunkId}`, data);
}

export function listChunkRevisions(knowledgeId: string, chunkId: string) {
  return get(`/api/v1/chunks/${knowledgeId}/${chunkId}/revisions`);
}

export function revertDocumentChunk(knowledgeId: string, chunkId: string, revision: number, expectedRevision: number) {
  return post(`/api/v1/chunks/${knowledgeId}/${chunkId}/revert`, {
    revision,
    expected_revision: expectedRevision,
  });
}

export function updateKnowledgeMetadata(knowledgeId: string, customMetadata: Record<string, unknown>) {
  return put(`/api/v1/knowledge/${knowledgeId}`, { custom_metadata: customMetadata });
}

/**
 * 触发发票字段提取：后端读取已解析文本并调用提取模型（复用知识库的
 * summary_model_id），将结果写入 custom_metadata。
 * 幂等：对同一 knowledge 重复调用会覆盖写，返回当前提取结果。
 */
export function extractInvoice(kbId: string, knowledgeId: string) {
  // 长超时：多张发票（如 18/22 张）提取需 LLM 生成超长 JSON，30s 默认超时会
  // 导致前端取消请求 → 后端 context canceled → 提取失败。
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-invoice`, {}, { timeout: 600000 });
}

/**
 * 发票级聚合列表：服务端按发票号去重、全字段搜索、类型/税率/开票日期筛选、
 * 排序、分页并返回金额聚合（sum_amount/sum_tax/sum_total）。
 */
export function listInvoiceRecords(kbId: string, params: {
  q?: string;
  invoice_type?: string;
  tax_rate?: number;
  date_from?: string;
  date_to?: string;
  page?: number;
  page_size?: number;
} = {}) {
  const q = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') q.append(k, String(v));
  });
  return get(`/api/v1/knowledge-bases/${kbId}/invoices?${q.toString()}`);
}

/**
 * 发票税率列表：返回该知识库下所有发票出现过的去重税率（含多档税率明细），
 * 用于税率筛选下拉框自动加载。
 */
export function listInvoiceTaxRates(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/invoice-tax-rates`);
}

/**
 * 按页重新提取发票：只重新提取该知识文档中第 page 张发票（发票按文档内
 * 出现顺序编号），仅替换该张发票数据，不影响同文件其它发票。
 */
export function extractInvoicePage(kbId: string, knowledgeId: string, page: number) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-invoice-page`, { page }, { timeout: 600000 });
}

/**
 * 按页删除发票记录：从该文档的发票数组中移除第 page 张；若文档仅剩该张发票，
 * 则整份文档一并删除（后端返回 deleted_file=true）。
 */
export function deleteInvoicePage(kbId: string, knowledgeId: string, page: number) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/delete-invoice-page`, { page }, { timeout: 120000 });
}

/**
 * 触发合同字段提取：后端读取已解析文本并调用提取模型（复用知识库的
 * summary_model_id），将结果写入 custom_metadata。
 * 幂等：对同一 knowledge 重复调用会覆盖写，返回当前提取结果。
 */
export function extractContract(kbId: string, knowledgeId: string) {
  // 长超时：多份合同文档提取需 LLM 生成超长 JSON，30s 默认超时会
  // 导致前端取消请求 → 后端 context canceled → 提取失败。
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-contract`, {}, { timeout: 600000 });
}

/**
 * 合同级聚合列表：服务端全字段搜索、类型/履约状态/签订日期筛选、
 * 排序、分页并返回金额聚合（sum_amount/sum_total）。
 */
export function listContractRecords(kbId: string, params: {
  q?: string;
  contract_type?: string;
  fulfill_status?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  page_size?: number;
} = {}) {
  const q = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') q.append(k, String(v));
  });
  return get(`/api/v1/knowledge-bases/${kbId}/contracts?${q.toString()}`);
}

/**
 * 合同类型列表：返回该知识库下所有合同出现过的去重合同类型（含数量），
 * 用于合同类型筛选下拉框自动加载。
 */
export function listContractTypes(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/contract-types`);
}

/**
 * 触发制度字段提取：后端读取已解析文本并调用提取模型（复用知识库的
 * summary_model_id），将结果写入 custom_metadata。
 */
export function extractRegulation(kbId: string, knowledgeId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-regulation`, {}, { timeout: 600000 });
}

/**
 * 制度级聚合列表：服务端全字段搜索、类型/编制日期筛选、排序与分页。
 */
export function listRegulationRecords(kbId: string, params: {
  q?: string;
  reg_type?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  page_size?: number;
} = {}) {
  const q = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') q.append(k, String(v));
  });
  return get(`/api/v1/knowledge-bases/${kbId}/regulations?${q.toString()}`);
}

/**
 * 制度类型列表：返回该知识库下所有制度出现过的去重制度类型（含数量），
 * 用于制度类型筛选下拉框自动加载。
 */
export function listRegulationTypes(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/regulation-types`);
}

/**
 * 奖惩字段提取：复用知识库配置的摘要模型（summary_model_id），按当事人
 * 拆分多条奖惩记录，将结果写入 custom_metadata。
 */
export function extractAwardPunish(kbId: string, knowledgeId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-award-punish`, {}, { timeout: 600000 });
}

/**
 * 奖惩级聚合列表：服务端全字段搜索、奖惩类型/状态/签发日期筛选、排序与分页
 * （一个当事人一条记录，同一文件多个当事人共享文号）。
 */
export function listAwardPunishRecords(kbId: string, params: {
  q?: string;
  ap_type?: string;
  status?: string;
  date_from?: string;
  date_to?: string;
  page?: number;
  page_size?: number;
} = {}) {
  const q = new URLSearchParams();
  Object.entries(params).forEach(([k, v]) => {
    if (v !== undefined && v !== null && v !== '') q.append(k, String(v));
  });
  return get(`/api/v1/knowledge-bases/${kbId}/award-punish-records?${q.toString()}`);
}

/**
 * 奖惩类型列表：返回该知识库下所有奖惩出现过的去重奖惩类型（含数量），
 * 用于奖惩类型筛选下拉框自动加载。
 */
export function listAwardPunishTypes(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/award-punish-types`);
}

/**
 * 识别规则配置（发票/合同管理页"识别规则"设置面板）：
 * 包含判定规则（模型判非但规则命中 → 认定为该类型）与类型归类规则。
 */
export function getRecognitionConfig(kbId: string) {
  return get(`/api/v1/knowledge-bases/${kbId}/recognition-config`);
}

export function saveRecognitionConfig(kbId: string, cfg: Record<string, unknown>) {
  return put(`/api/v1/knowledge-bases/${kbId}/recognition-config`, cfg);
}

/** 用当前包含判定规则重新评估删除历史：命中的自动恢复为待补录记录。 */
export function reassessRecognition(kbId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/recognition/reassess`, {});
}

/**
 * 按页重新提取合同：只重新提取该知识文档中第 page 份合同（合同按文档内
 * 出现顺序编号），仅替换该份合同数据，不影响同文件其它合同。
 */
export function extractContractPage(kbId: string, knowledgeId: string, page: number) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/extract-contract-page`, { page }, { timeout: 600000 });
}

/**
 * 按页删除合同记录：从该文档的合同数组中移除第 page 份；若文档仅剩该份合同，
 * 则整份文档一并删除（后端返回 deleted_file=true）。
 */
export function deleteContractPage(kbId: string, knowledgeId: string, page: number) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/${knowledgeId}/delete-contract-page`, { page }, { timeout: 120000 });
}

// ---- 删除历史（非合同/非发票自动删除记录的查看/恢复/永久删除）----

/** 删除历史列表：知识库内被系统自动删除（判定非合同/非发票）且保留源文件的记录 */
export function listDeletedKnowledge(kbId: string, params: { page?: number; page_size?: number; q?: string } = {}) {
  return get(`/api/v1/knowledge-bases/${kbId}/knowledge/deleted-knowledge`, params);
}

/** 恢复自动删除的记录：恢复回知识库并重新解析提取（再次判定非该类文档时不再自动删除，防循环） */
export function restoreDeletedKnowledge(kbId: string, knowledgeId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/deleted-knowledge/${knowledgeId}/restore`, {}, { timeout: 120000 });
}

/** 永久删除历史记录：DB 硬删 + 物理源文件删除，不可恢复 */
export function purgeDeletedKnowledge(kbId: string, knowledgeId: string) {
  return post(`/api/v1/knowledge-bases/${kbId}/knowledge/deleted-knowledge/${knowledgeId}/purge`, {}, { timeout: 120000 });
}

export function updateKnowledgeSummary(knowledgeId: string, description: string) {
  return put(`/api/v1/knowledge/${knowledgeId}`, { description });
}

export function regenerateKnowledgeSummary(knowledgeId: string) {
  return post(`/api/v1/knowledge/${knowledgeId}/regenerate-summary`, {});
}

// Get chunk by chunk_id only (new endpoint - to be added to backend)
export function getChunkByIdOnly(chunkId: string) {
  return get(`/api/v1/chunks/by-id/${chunkId}`);
}

// Delete a single generated question from a chunk by question ID
export function deleteGeneratedQuestion(chunkId: string, questionId: string) {
  return del(`/api/v1/chunks/by-id/${chunkId}/questions`, { question_id: questionId });
}

export function upsertGeneratedQuestion(chunkId: string, question: string, questionId?: string) {
  return put(`/api/v1/chunks/by-id/${chunkId}/questions`, {
    question_id: questionId || '',
    question,
  });
}

export function regenerateGeneratedQuestions(chunkId: string) {
  return post(`/api/v1/chunks/by-id/${chunkId}/questions/regenerate`, {});
}

export function listKnowledgeTags(
  kbId: string,
  params?: { page?: number; page_size?: number; keyword?: string },
) {
  const query = buildQuery(params);
  return get(`/api/v1/knowledge-bases/${kbId}/tags${query}`);
}

export function createKnowledgeBaseTag(
  kbId: string,
  data: { name: string; color?: string; sort_order?: number },
) {
  return post(`/api/v1/knowledge-bases/${kbId}/tags`, data);
}

export function updateKnowledgeBaseTag(
  kbId: string,
  tagId: string,
  data: { name?: string; color?: string; sort_order?: number },
) {
  return put(`/api/v1/knowledge-bases/${kbId}/tags/${tagId}`, data);
}

export function deleteKnowledgeBaseTag(kbId: string, tagSeqId: number, params?: { force?: boolean }) {
  const forceQuery = params?.force ? '?force=true' : '';
  return del(`/api/v1/knowledge-bases/${kbId}/tags/${tagSeqId}${forceQuery}`);
}

export function updateKnowledgeTagBatch(data: { updates: Record<string, string[]> }) {
  return put(`/api/v1/knowledge/tags`, data);
}

export function updateFAQEntryTagBatch(kbId: string, data: { updates: Record<number, number | null> }) {
  return put(`/api/v1/knowledge-bases/${kbId}/faq/entries/tags`, data);
}

const buildQuery = (params?: Record<string, any>) => {
  if (!params) return '';
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') return;
    query.append(key, String(value));
  });
  const queryString = query.toString();
  return queryString ? `?${queryString}` : '';
};

export function listFAQEntries(
  kbId: string,
  params?: { page?: number; page_size?: number; tag_id?: number; tag_ids?: string; keyword?: string },
) {
  const query = buildQuery(params);
  return get(`/api/v1/knowledge-bases/${kbId}/faq/entries${query}`);
}

export function upsertFAQEntries(kbId: string, data: { entries: any[]; mode: 'append' | 'replace' }) {
  return post(`/api/v1/knowledge-bases/${kbId}/faq/entries`, data);
}

export function createFAQEntry(kbId: string, data: any) {
  return post(`/api/v1/knowledge-bases/${kbId}/faq/entry`, data);
}

export function updateFAQEntry(kbId: string, entryId: number, data: any) {
  return put(`/api/v1/knowledge-bases/${kbId}/faq/entries/${entryId}`, data);
}

// Unified batch update API - supports is_enabled, is_recommended, tag_id
// Supports two modes:
// 1. By entry ID: use by_id field
// 2. By Tag: use by_tag field to apply the same update to all entries under a tag
export interface FAQEntryFieldsUpdate {
  is_enabled?: boolean
  is_recommended?: boolean
  tag_id?: number | null
}

export interface FAQEntryFieldsBatchRequest {
  by_id?: Record<number, FAQEntryFieldsUpdate>
  by_tag?: Record<number, FAQEntryFieldsUpdate>
  exclude_ids?: number[]
}

export function updateFAQEntryFieldsBatch(kbId: string, data: FAQEntryFieldsBatchRequest) {
  return put(`/api/v1/knowledge-bases/${kbId}/faq/entries/fields`, data);
}

export function deleteFAQEntries(kbId: string, ids: number[]) {
  return del(`/api/v1/knowledge-bases/${kbId}/faq/entries`, { ids });
}

export function searchFAQEntries(
  kbId: string,
  data: {
    query_text: string
    vector_threshold?: number
    match_count?: number
  }
) {
  return post(`/api/v1/knowledge-bases/${kbId}/faq/search`, data);
}

// Export FAQ entries as CSV or JSON file
export async function exportFAQEntries(kbId: string, format: 'csv' | 'json' = 'csv'): Promise<Blob> {
  const suffix = format === 'json' ? '?format=json' : ''
  const response = await getDown(`/api/v1/knowledge-bases/${kbId}/faq/entries/export${suffix}`)
  return response as unknown as Blob
}

// FAQ Import Progress API
export interface FAQBlockedEntry {
  index: number
  standard_question: string
  reason: string
}

export interface FAQSuccessEntry {
  index: number
  seq_id: number
  tag_id?: number
  tag_name?: string
  standard_question: string
}

export interface FAQImportProgress {
  task_id: string
  kb_id: string
  knowledge_id: string
  status: 'pending' | 'processing' | 'completed' | 'failed'
  progress: number
  total: number
  processed: number
  blocked: number
  blocked_entries?: FAQBlockedEntry[]
  success_entries?: FAQSuccessEntry[]
  message: string
  error: string
  created_at: number
  updated_at: number
}

export function getFAQImportProgress(taskId: string) {
  return get(`/api/v1/faq/import/progress/${taskId}`);
}

export function updateFAQImportResultDisplayStatus(knowledgeBaseId: string, displayStatus: 'open' | 'close') {
  return put(`/api/v1/knowledge-bases/${knowledgeBaseId}/faq/import/last-result/display`, {
    display_status: displayStatus
  });
}

export function searchKnowledge(
  keyword: string,
  offset = 0,
  limit = 20,
  fileTypes?: string[],
  options?: { agent_id?: string; agent_source_tenant_id?: string; recent?: boolean }
) {
  const query = new URLSearchParams();
  if (keyword) {
    query.set('keyword', keyword);
  }
  query.set('offset', String(offset));
  query.set('limit', String(limit));
  if (fileTypes && fileTypes.length > 0) {
    query.set('file_types', fileTypes.join(','));
  }
  if (options?.agent_id) query.set('agent_id', options.agent_id);
  if (options?.agent_source_tenant_id) query.set('agent_source_tenant_id', options.agent_source_tenant_id);
  if (options?.recent) query.set('recent', 'true');
  return get(`/api/v1/knowledge/search?${query.toString()}`);
}

export function knowledgeSemanticSearch(data: {
  query: string;
  knowledge_base_ids?: string[];
  knowledge_ids?: string[];
}) {
  return post('/api/v1/knowledge-search', data);
}

export function batchReparseKnowledge(kbId: string, ids: string[], processConfig?: KnowledgeProcessOverrides) {
  return post(`/api/v1/knowledge/batch-reparse`, {
    kb_id: kbId,
    ids,
    process_config: processConfig,
  });
}
