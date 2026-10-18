import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  REGEX_PATCHABLE_KEYS,
  isRegexPatchable,
  regexDisabledReason,
} from './extractFieldMeta'

// ---------------------------------------------------------------------------
// 边界对齐：前端 isStandardField 认 22 个发票标准键（用于判定 required），
// 但后端正式提取链路的正则补洞只认 invoiceFieldOrder 的 8 个
//（internal/application/service/extract_regex_fallback.go:106-111）。
// 此前 UI 用同一把尺子判"正则可配"，用户给 totalAmount 配了正则却不执行 ——
// 静默失效比置灰更糟。这里把两把尺子拆开。
// ---------------------------------------------------------------------------

test('isRegexPatchable 认后端真正能补洞的 8 个发票键（含 camelCase 形式）', () => {
  const keys = ['invoice_no', 'invoice_code', 'invoice_date', 'buyer_name',
    'buyer_tax_no', 'seller_name', 'seller_tax_no', 'issuer']
  for (const k of keys) assert.equal(isRegexPatchable('invoice', k), true, `${k} 应可配正则`)
  // 前端配置键是 camelCase，必须同样命中
  for (const k of ['invoiceNo', 'invoiceCode', 'invoiceDate', 'buyerName',
    'buyerTaxNo', 'sellerName', 'sellerTaxNo']) {
    assert.equal(isRegexPatchable('invoice', k), true, `${k} 应可配正则`)
  }
  assert.equal(REGEX_PATCHABLE_KEYS.length, 8)
})

test('isRegexPatchable 拒绝「前端认成标准键、后端却不补洞」的字段', () => {
  for (const k of ['totalAmount', 'taxRate', 'tax', 'amount', 'invoiceType',
    'remark', 'fileName', 'tags', 'extractStatus']) {
    assert.equal(isRegexPatchable('invoice', k), false, `${k} 后端不补洞，必须置灰`)
  }
})

test('isRegexPatchable 拒绝中文自定义扩展 key', () => {
  for (const k of ['车牌号', '户号', '通行区间', '收票编号']) {
    assert.equal(isRegexPatchable('invoice', k), false, `${k} 属自定义字段`)
  }
})

test('非发票 scope 一律不可配正则（本轮只对齐了 invoice 的后端补洞白名单）', () => {
  assert.equal(isRegexPatchable('contract', 'contractNo'), false)
  assert.equal(isRegexPatchable('vehicle', '车牌号'), false)
  assert.equal(isRegexPatchable('award_punish', 'apNo'), false)
})

test('未接入字段级校验的 scope 与空字段名不误判', () => {
  assert.equal(isRegexPatchable('unknown_scope', 'invoiceNo'), false)
  assert.equal(isRegexPatchable('invoice', ''), false)
  assert.equal(isRegexPatchable('invoice', '   '), false)
})

test('regexDisabledReason 给出人话原因（消除"配了不生效"的落差）', () => {
  assert.equal(regexDisabledReason('invoice', 'invoiceNo'), '')
  assert.match(regexDisabledReason('invoice', '车牌号'), /自定义/)
  assert.match(regexDisabledReason('invoice', 'totalAmount'), /发票/)
  assert.match(regexDisabledReason('contract', 'contractNo'), /暂未接入/)
  assert.match(regexDisabledReason('unknown_scope', 'x'), /暂未接入/)
})

// ---- 源码级锁定：ExtractRulePanel 的正则 UI 必须改用新判定 ----
const panel = readFileSync(new URL('./ExtractRulePanel.vue', import.meta.url), 'utf8')

test('ExtractRulePanel 的正则输入/预设改用 isRegexPatchable，required 仍用 isStdField', () => {
  assert.match(panel, /isRegexPatchable/)
  // 正则输入框两处（行内 + 展开区）都必须按 isRegexPatchable 置灰
  const regexDisabled = (panel.match(/:disabled="!regexPatchable\(f\.name\)"/g) || []).length
  assert.ok(regexDisabled >= 2, `正则输入置灰处应≥2，实际 ${regexDisabled}`)
  // 必填开关仍走标准键判定（后端校验器认全部 22 个标准键）
  assert.match(panel, /:disabled="!isStdField\(f\.name\)"/)
})

test('自定义扩展字段的 Alert 文案升级为区分「自定义 key」与「后端不补洞」', () => {
  // 旧文案把两类原因混为一谈（"仅对系统标准字段生效"），对 totalAmount 这类
  // 「前端认标准键、后端不补洞」的字段是误导。Alert 现在直接复用 regexDisabledReason。
  assert.match(panel, /:message="regexDisabledReason\(scope, f\.name\)"/)
  assert.doesNotMatch(panel, /自定义扩展字段仅注入 AI 提取 Prompt，后置正则补洞仅对系统标准字段生效/)
})