/**
 * 提取错误信息中文化（与后端 service.FormatExtractErrorMessage 同规则，必须保持同步）。
 *
 * 背景：模型侧报错原样透出时会是「英文调用栈 + 裸 JSON」，例如
 *   invoice extraction failed: extract invoice fields: create chat completion:
 *   API request failed with status 429: {"error":{"code":"1302","message":"您的账户已达到速率限制"}}
 * 直接塞进 Tooltip 既看不懂也容易让用户误以为数据损坏。
 *
 * 设计约束：
 *   - 幂等：已经是中文的结果再次传入不会被破坏（前后端都会调用）。
 *   - 只做展示层转译，不吞掉信息：匹配不到分类时剥离英文技术栈前缀后返回核心原因。
 */

const CJK_RE = /[\u4e00-\u9fa5]/

/** 命中速率限制（429 / 厂商码 1302 / 速率限制 / 请求频率）。 */
function isRateLimit(lower: string): boolean {
  return (
    lower.includes('429') ||
    lower.includes('1302') ||
    lower.includes('rate limit') ||
    lower.includes('rate_limit') ||
    lower.includes('ratelimit') ||
    lower.includes('rate-limit') ||
    lower.includes('too many requests') ||
    lower.includes('速率限制') ||
    lower.includes('请求频率') ||
    lower.includes('限流')
  )
}

function isTimeout(lower: string): boolean {
  return (
    lower.includes('timeout') ||
    lower.includes('deadline exceeded') ||
    lower.includes('context deadline') ||
    lower.includes('i/o timeout') ||
    lower.includes('超时')
  )
}

function isNoText(lower: string): boolean {
  return (
    lower.includes('no_text') ||
    lower.includes('no text content') ||
    lower.includes('empty content') ||
    lower.includes('empty extraction') ||
    lower.includes('未识别到')
  )
}

/** 从 `{"error":{"code":"1302","message":"..."}}` 中取出 message；取不到则原样返回。 */
export function extractJsonMessage(raw: string): string {
  const s = raw.trim()
  if (!s.includes('{')) return s
  const start = s.indexOf('{')
  let depth = 0
  let end = -1
  for (let i = start; i < s.length; i++) {
    if (s[i] === '{') depth++
    else if (s[i] === '}') {
      depth--
      if (depth === 0) {
        end = i + 1
        break
      }
    }
  }
  if (end <= 0) return s
  try {
    const parsed = JSON.parse(s.slice(start, end)) as Record<string, any>
    const nested = parsed?.error?.message
    if (typeof nested === 'string' && nested.trim()) return nested.trim()
    if (typeof parsed?.message === 'string' && parsed.message.trim()) return parsed.message.trim()
  } catch {
    /* 非 JSON：原样返回 */
  }
  return s
}

/** 按 ":" 分段剥离英文技术栈前缀，露出含中文的核心原因。 */
export function stripExtractTechPrefix(raw: string): string {
  const parts = raw.split(':')
  for (let i = 0; i < parts.length; i++) {
    const seg = parts[i].trim()
    if (!seg) continue
    if (CJK_RE.test(seg)) return parts.slice(i).join('：').trim()
  }
  // 全英文：保留最后一段（信息量最大的尾部）
  for (let i = parts.length - 1; i >= 0; i--) {
    if (parts[i].trim()) return parts[i].trim()
  }
  return ''
}

/** 统一的中文错误转译。 */
export function formatExtractErrorMessage(raw?: string | null): string {
  const msg = extractJsonMessage(String(raw ?? '').trim())
  if (!msg) return ''
  const lower = msg.toLowerCase()
  if (isRateLimit(lower)) return '请求频率超限（已触发大模型限流保护），请稍后重试'
  if (isTimeout(lower)) return '提取超时，文档较大或模型响应缓慢'
  if (isNoText(lower)) return '未识别到有效票面文本，请检查源文件是否清晰'
  return stripExtractTechPrefix(msg) || msg
}
