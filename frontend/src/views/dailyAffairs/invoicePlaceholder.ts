// 文件级占位行的统一构造（第 26 轮 O-5）。
//
// 发票列表有三种「占位行」——该源文件没有任何可展示的发票明细：
//   ① meta 缺失（老数据 / 刚上传还没解析完）→ kind=unknown、状态空
//   ② 系统判定非发票                      → kind=not_invoice、状态强制 not_invoice
//   ③ 是发票但一张都没提出来                → kind=invoice、状态取 meta.extract_status
//
// 三处此前各写一遍 `{ rowKey, knowledgeId, fileName, extractStatus, extractError,
// kind, ...base }`，字段顺序与默认值随时可能漂移（尤其 ② 的「强制 not_invoice」
// 一旦漏掉，非发票文件会显示成「提取失败」并进入失败统计）。

export interface FilePlaceholderInput {
  /** knowledge id */
  id: string
  fileName: string
  /** 文件级公共字段（parseStatus / summaryStatus / fileType / tags / description…） */
  base: Record<string, any>
  /** 缺省 unknown */
  kind?: string
  extractStatus?: string
  extractError?: string
}

// 返回 any 而不是 Record<string, any>：调用方会直接塞进 `InvoiceRow[]`，
// 而 InvoiceRow 的必填项由 base（parseStatus 等）补齐，类型系统看不到这层约定。
// 用 any 让调用点保持干净；真正的字段完整性由 invoicePlaceholder.test.ts 的
// O-5d 断言（base 必须完整透传）来守。
export function makeFilePlaceholder(input: FilePlaceholderInput): any {
  const { id, fileName, base, kind, extractStatus, extractError } = input
  // 非发票是**结论**而非状态：无论 meta 里残留什么状态（历史上真踩过
  // extract_status=failed 残留在 not_invoice 文件上），都必须落 not_invoice。
  const finalStatus = kind === 'not_invoice' ? 'not_invoice' : (extractStatus || '')
  return {
    rowKey: id,
    knowledgeId: id,
    fileName,
    extractStatus: finalStatus,
    extractError: extractError || '',
    kind: kind || 'unknown',
    ...base,
  }
}
