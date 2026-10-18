// 样票文本的会话级缓存：让「智能字段生成」（字段定义 Tab）产出的样票文本
// 能跨组件传到「智能配置规则」（提取规则 Tab）触发正则唯一命中校验。
//
// 为什么必须缓存而不是直接透传：
//   两个 Tab 在 StandardSettingDrawer 里是**兄弟组件**，字段定义 Tab 的
//   AiFieldSuggestDialog 是它的子组件，dialog 关掉后文本就没了；
//   而提取规则 Tab 的正则护栏**必须**拿到同一份样票原文才能验证唯一命中。
//
// 存储形态对齐既有的 weknora-extract-inflight（sessionStorage + TTL）：
//   - sessionStorage 而非 localStorage：样票原文可能含企业名称、税号等敏感信息，
//     不该跨浏览器会话留存；
//   - 30 分钟 TTL：覆盖"生成字段 → 保存 → 切Tab → 生成规则"这段人工操作时间，
//     同时避免半天的 tab 里留着一张旧发票的原文。

const KEY = 'weknora-invoice-sample-text'
const TTL_MS = 30 * 60 * 1000

interface Entry {
  certType: string
  text: string
  at: number
}

function readStore(): Record<string, Entry> {
  try {
    const raw = sessionStorage.getItem(KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, Entry>) : {}
  } catch {
    return {}
  }
}

function writeStore(store: Record<string, Entry>) {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(store))
  } catch {
    /* 隐私模式/配额满：缓存失败只影响"一键带样票"，不影响主流程 */
  }
}

function isFresh(e: Entry | undefined): e is Entry {
  return !!e && typeof e.text === 'string' && !!e.text && Date.now() - (e.at || 0) < TTL_MS
}

/** 缓存某分类的样票文本（智能字段生成成功后调用）。 */
export function cacheSampleText(certType: string, text: string): void {
  const key = String(certType || '').trim()
  const body = String(text || '')
  if (!key || !body) return
  const store = readStore()
  store[key] = { certType: key, text: body, at: Date.now() }
  writeStore(store)
}

/** 读取某分类的样票文本（过期或不存在返回空串）。 */
export function readSampleText(certType: string): string {
  const key = String(certType || '').trim()
  if (!key) return ''
  const e = readStore()[key]
  return isFresh(e) ? e.text : ''
}

/** 丢弃某分类的缓存（用户保存后或主动清除时调用，避免陈旧样票误导正则）。 */
export function dropSampleText(certType: string): void {
  const key = String(certType || '').trim()
  if (!key) return
  const store = readStore()
  if (!(key in store)) return
  delete store[key]
  writeStore(store)
}

/** 是否存在可用的样票文本（用于决定按钮是否提示"将带上样票校验正则"）。 */
export function hasSampleText(certType: string): boolean {
  return !!readSampleText(certType)
}

/** 存储 key 与 TTL 暴露给测试断言，避免悄悄改口径。 */
export const SAMPLE_TEXT_STORAGE_KEY = KEY
export const SAMPLE_TEXT_TTL_MS = TTL_MS