/**
 * Shared access-token refresh used by both axios and the chat SSE client.
 *
 * Refresh tokens rotate, so concurrent 401s must queue behind one in-flight
 * /auth/refresh. The SSE handshake runs on raw fetch and cannot use the
 * axios interceptor; both planes call {@link refreshAccessTokenShared}.
 */

const AUTH_STORAGE_KEYS = [
  'weknora_token',
  'weknora_refresh_token',
  'weknora_user',
  'weknora_tenant',
  'weknora_knowledge_bases',
  'weknora_current_kb',
  'weknora_selected_tenant_id',
  'weknora_selected_tenant_name',
  'weknora_memberships',
] as const

let isRefreshing = false
let failedQueue: Array<{
  resolve: (token: string) => void
  reject: (error: unknown) => void
}> = []

export type TokenRefreshResult = {
  success: boolean
  data?: { token: string; refreshToken: string }
  message?: string
}

export type RefreshAccessTokenOptions = {
  refresh?: (refreshToken: string) => Promise<TokenRefreshResult>
  messages?: {
    pleaseRelogin: string
    tokenRefreshFailed: string
  }
}

const defaultMessages = {
  pleaseRelogin: 'Please log in again',
  tokenRefreshFailed: 'Token refresh failed',
}

export class StreamAuthError extends Error {
  constructor(readonly status: number) {
    super(`HTTP ${status}`)
    this.name = 'StreamAuthError'
  }
}

export function isStreamAuthError(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  if (err instanceof StreamAuthError) return true
  return (err as { name?: string }).name === 'StreamAuthError'
}

export function isEmbedPage(): boolean {
  if (typeof window === 'undefined') return false
  return window.location.pathname.startsWith('/embed/')
}

export function redirectToLogin() {
  if (typeof window === 'undefined') return
  if (window.location.pathname === '/login') return
  if (isEmbedPage()) return
  window.location.href = '/login'
}

export function clearAuthStorage() {
  for (const key of AUTH_STORAGE_KEYS) {
    localStorage.removeItem(key)
  }
}

export function forceReloginRedirect() {
  clearAuthStorage()
  redirectToLogin()
}

function processQueue(error: unknown, token: string | null = null) {
  failedQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error)
    } else {
      resolve(token as string)
    }
  })
  failedQueue = []
}

async function defaultRefresh(refreshToken: string): Promise<TokenRefreshResult> {
  const { refreshToken: refreshTokenAPI } = await import('../api/auth/index')
  return refreshTokenAPI(refreshToken)
}

/**
 * 判断刷新失败是否属于「基础设施故障」而非「登录态真的失效」。
 *
 * 关键区别：
 *  - 5xx / 503（auth_backend_unavailable）/ 网络错误 / 超时 → 后端数据库或网关
 *    暂时不可用。此时 refresh token 本身没坏，**绝不能**清凭据跳登录 ——
 *    否则一次连接池抖动就会把所有在线用户踢下线（实测 Supabase session 模式
 *    连接池上限 15 被打满时，50 秒解析轮询期间连踢数次）。
 *  - 401 / 400 才是「这个 refresh token 确实无效」。
 */
function isTransientRefreshFailure(err: unknown): boolean {
  if (!err || typeof err !== 'object') return false
  const e = err as Record<string, any>
  const status = Number(e.$httpStatus ?? e.status ?? e.response?.status ?? 0)
  if (status >= 500) return true
  // 网络层失败（无响应）由 request.ts 统一转成 message，没有状态码
  if (!status) {
    const msg = String(e.message ?? '')
    return /network|timeout|failed to fetch|networkerror/i.test(msg)
  }
  return false
}

/**
 * Refresh the access token, de-duplicated across all callers.
 *
 * Resolves with the new access token. On **auth** failure it has already cleared
 * credentials and redirected to /login. On transient infrastructure failure it
 * rejects without touching the session, so polling can simply retry.
 */
export async function refreshAccessTokenShared(
  options: RefreshAccessTokenOptions = {},
): Promise<string> {
  const messages = { ...defaultMessages, ...options.messages }
  const refresh = options.refresh ?? defaultRefresh

  if (isRefreshing) {
    return new Promise<string>((resolve, reject) => {
      failedQueue.push({ resolve, reject })
    })
  }

  isRefreshing = true
  const storedRefreshToken = localStorage.getItem('weknora_refresh_token')

  if (!storedRefreshToken) {
    clearAuthStorage()
    const noRefreshTokenError = new Error(messages.pleaseRelogin)
    processQueue(noRefreshTokenError, null)
    isRefreshing = false
    redirectToLogin()
    throw noRefreshTokenError
  }

  try {
    const response = await refresh(storedRefreshToken)

    if (!response.success || !response.data?.token) {
      throw new Error(response.message || messages.tokenRefreshFailed)
    }

    const { token, refreshToken: newRefreshToken } = response.data
    localStorage.setItem('weknora_token', token)
    if (newRefreshToken) {
      localStorage.setItem('weknora_refresh_token', newRefreshToken)
    }
    processQueue(null, token)
    return token
  } catch (refreshError) {
    if (isTransientRefreshFailure(refreshError)) {
      // 基础设施抖动：保留登录态，让调用方重试（不清 storage、不跳登录）
      processQueue(refreshError, null)
      throw refreshError
    }
    clearAuthStorage()
    processQueue(refreshError, null)
    redirectToLogin()
    throw refreshError
  } finally {
    isRefreshing = false
  }
}

/**
 * Replay an SSE handshake once after a 401. Refresh failure already
 * redirected; a second 401 after a successful refresh is surfaced to the
 * caller without wiping the newly minted session (axios does the same).
 *
 * Returns undefined when a newer send or an abort superseded the replay.
 */
export async function runStreamWithAuthRetry<T>(options: {
  run: (token: string) => Promise<T>
  initialToken: string
  isEmbed: boolean
  isCurrent: () => boolean
  refreshAccessToken: () => Promise<string>
  reloginMessage: string
}): Promise<T | undefined> {
  try {
    return await options.run(options.initialToken)
  } catch (err) {
    if (!isStreamAuthError(err) || options.isEmbed) throw err

    let refreshedToken: string
    try {
      refreshedToken = await options.refreshAccessToken()
    } catch {
      throw new Error(options.reloginMessage)
    }

    if (!options.isCurrent()) return undefined

    try {
      return await options.run(refreshedToken)
    } catch (retryErr) {
      if (isStreamAuthError(retryErr)) {
        throw new Error(options.reloginMessage)
      }
      throw retryErr
    }
  }
}

/** Reset module locks between unit tests. */
export function resetAuthRefreshStateForTests() {
  isRefreshing = false
  failedQueue = []
}
