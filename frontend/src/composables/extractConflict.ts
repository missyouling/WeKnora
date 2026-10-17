/**
 * 提取接口 409 冲突的统一语义判别。
 *
 * 后端 BusinessExtractHandler.extractMu 按 knowledgeID 防重入：同一文档已有
 * 提取任务在途时，重复投递会返回 409（details.reason = extract_in_progress）。
 * 这是**并发重复提交**，不是提取失败——数据仍由持锁的那个请求完成，前端应当
 * 转为「等待终态跟随」，而不是把状态列打成「提取失败」或弹红色报错。
 *
 * 单独成文件（不依赖 vue / api）是为了能被 `tsx --test` 直接单测：
 * useBusinessPolling.ts 依赖 vue，测试环境跑不起来。
 */

/** 后端在 409 details 中标记的「已有任务在途」原因值。 */
export const EXTRACT_IN_PROGRESS_REASON = 'extract_in_progress'

/** 从各种错误形状中取 HTTP 状态码（request.ts 注入的是非枚举属性 $httpStatus）。 */
export function httpStatusOf(err: unknown): number {
  const e = err as Record<string, any> | null | undefined
  if (!e || typeof e !== 'object') return 0
  const raw =
    e.$httpStatus ??
    e.status ??
    e.response?.status ??
    e.error?.status ??
    e.data?.status
  const n = Number(raw)
  return Number.isFinite(n) ? n : 0
}

/** 从各种错误形状中取后端 details.reason（可能为 undefined）。 */
export function conflictReasonOf(err: unknown): string | undefined {
  const e = err as Record<string, any> | null | undefined
  if (!e || typeof e !== 'object') return undefined
  const d =
    e.details ??
    e.error?.details ??
    e.data?.error?.details ??
    e.data?.details
  const reason = d?.reason
  return typeof reason === 'string' && reason ? reason : undefined
}

/**
 * 判断一个提取请求错误是否为「同一文档已有任务在途」的并发冲突。
 *
 * 兼容两种后端形态：
 * - 新版：409 + details.reason = extract_in_progress → 精确命中；
 * - 旧版/网关改写响应体：details 丢失 → 提取接口上的 409 只有「在途」这一种
 *   语义（单号查重 409 发生在元数据保存接口，不在提取链路），故兜底判为在途。
 */
export function isExtractInProgress(err: unknown): boolean {
  if (httpStatusOf(err) !== 409) return false
  const reason = conflictReasonOf(err)
  return !reason || reason === EXTRACT_IN_PROGRESS_REASON
}
