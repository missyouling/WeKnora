// 物理页码解析（第一批次 P2-5 的前端消费端）。
//
// 背景：InvoiceExtractionItem 有两个页码字段，语义完全不同——
//   - page    票号序号（1..N 连续），由 NormalizeInvoiceExtractionResult 自增，
//             同时是删除接口的定位键（delete-invoice-page?page=N），绝不能改语义；
//   - page_no PDF 物理页，来自 Chunk.SourceLocators（MinerU/PaddleOCR/DocReader
//             等报告位置的引擎），无位置信息的引擎回退为序号。
// 预览跳转与批量打印要的是物理页；老数据（重构前提取）没有 page_no，回退序号。
//
// 为什么抽成纯函数：InvoiceManagement.vue 有 3000 行，跳页判据散在徽章、
// DocumentPreview、批量打印三处，曾因口径不一出现“徽章显示第 10 页、预览跳到
// 第 2 页”。收敛到一个函数 + 单测锁死。

export interface PdfPageRow {
  page?: number | null
  page_no?: number | null
  pageNo?: number | null
}

/** 取用于 PDF 定位的页码：物理页优先，缺失回退序号，都没有返回 0。 */
export function pdfPageOf(row: PdfPageRow | null | undefined): number {
  if (!row) return 0
  const physical = Number(row.page_no ?? row.pageNo ?? 0)
  if (physical > 0) return physical
  return Number(row.page ?? 0)
}
