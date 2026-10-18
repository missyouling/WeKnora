import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  SAMPLE_TEXT_STORAGE_KEY,
  SAMPLE_TEXT_TTL_MS,
  cacheSampleText,
  dropSampleText,
  hasSampleText,
  readSampleText,
} from './sampleTextCache'

// 样票文本必须能跨 Tab 传递（字段定义 → 提取规则），且不得跨浏览器会话留存：
// 票面原文含企业名称/税号，属敏感信息，因此用 sessionStorage 而非 localStorage。

/** 最小 sessionStorage 替身（Node 测试环境无 DOM）。 */
function withSessionStorage<T>(fn: (store: Map<string, string>) => T): T {
  const store = new Map<string, string>()
  const prev = (globalThis as any).sessionStorage
  const fake = {
    getItem: (k: string) => (store.has(k) ? store.get(k)! : null),
    setItem: (k: string, v: string) => void store.set(k, String(v)),
    removeItem: (k: string) => void store.delete(k),
    clear: () => store.clear(),
  }
  ;(globalThis as any).sessionStorage = fake
  try {
    return fn(store)
  } finally {
    ;(globalThis as any).sessionStorage = prev
  }
}

test('样票文本按 cert_type 分键存取', () => {
  withSessionStorage(() => {
    cacheSampleText('水电发票', '发票号码 24312000000012345678')
    cacheSampleText('通行费发票', '车牌号 渝C87567')
    assert.equal(readSampleText('水电发票'), '发票号码 24312000000012345678')
    assert.equal(readSampleText('通行费发票'), '车牌号 渝C87567')
    // 不同分类不得串票
    assert.equal(readSampleText('其它票据'), '')
  })
})

test('空值不写缓存（避免把"没识别到票面"当成有样票）', () => {
  withSessionStorage((store) => {
    cacheSampleText('水电发票', '')
    cacheSampleText('', 'some text')
    assert.equal(store.has(SAMPLE_TEXT_STORAGE_KEY), false)
    assert.equal(hasSampleText('水电发票'), false)
  })
})

test('超过 30 分钟 TTL 的缓存视为失效', () => {
  withSessionStorage(() => {
    cacheSampleText('水电发票', 'x')
    // 手工把时间戳拨到 TTL 之外
    const raw = JSON.parse(sessionStorage.getItem(SAMPLE_TEXT_STORAGE_KEY)!)
    raw['水电发票'].at = Date.now() - SAMPLE_TEXT_TTL_MS - 1000
    sessionStorage.setItem(SAMPLE_TEXT_STORAGE_KEY, JSON.stringify(raw))
    assert.equal(readSampleText('水电发票'), '', '过期缓存不得返回')
    assert.equal(hasSampleText('水电发票'), false)
  })
})

test('dropSampleText 清除指定分类，不影响其他分类', () => {
  withSessionStorage(() => {
    cacheSampleText('水电发票', 'a')
    cacheSampleText('通行费发票', 'b')
    dropSampleText('水电发票')
    assert.equal(readSampleText('水电发票'), '')
    assert.equal(readSampleText('通行费发票'), 'b')
  })
})

test('缓存内容损坏时静默降级为空，不抛异常', () => {
  withSessionStorage((store) => {
    store.set(SAMPLE_TEXT_STORAGE_KEY, '{not json')
    assert.equal(readSampleText('水电发票'), '')
    cacheSampleText('水电发票', 'ok')
    assert.equal(readSampleText('水电发票'), 'ok')
  })
})

// ---- 源码级锁定：跨 Tab 串联链路不得断 ----

test('StandardSettingDrawer 暴露 goRules 并能切到提取规则 Tab', () => {
  const src = readFileSync(
    new URL('../../components/business/StandardSettingDrawer.vue', import.meta.url), 'utf8')
  assert.match(src, /function goRules\(/)
  assert.match(src, /defineExpose\(\{[^}]*goRules/)
  // 字段定义 Tab 保存后经事件冒泡触发跳转
  assert.match(src, /@ai-guide|onAiGuide/)
})

test('ExtractRulePanel 暴露 focusCertType 与 runAiSuggest，且工具栏有智能生成规则按钮', () => {
  const src = readFileSync(new URL('./ExtractRulePanel.vue', import.meta.url), 'utf8')
  assert.match(src, /function focusCertType\(/)
  assert.match(src, /function runAiSuggest\(/)
  assert.match(src, /defineExpose\(\{[^}]*focusCertType/)
  assert.match(src, /智能生成规则/)
})

// 2026-10-18 改造：字段生成已从弹窗改为编辑面板内嵌，测试名同步（断言不变）
test('FleetCertPanel 内嵌智能字段生成面板，并在保存后询问是否跳转', () => {
  const src = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')
  assert.match(src, /AiFieldSuggestPanel/)
  assert.match(src, /智能生成字段/)
  // 保存成功后必须提示跳转（这是需求里"保存 → 询问 → 跳转"的一环）
  assert.match(src, /DialogPlugin\.confirm|ai-guide/)
})