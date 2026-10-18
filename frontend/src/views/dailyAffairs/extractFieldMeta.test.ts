import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  INVOICE_STRUCT_KEYS,
  extractFieldLabel,
  isInvoiceStructKey,
  isRegexPatchable,
  regexDisabledReason,
} from './extractFieldMeta'

// ---------------------------------------------------------------------------
// 正则可配范围（2026-10-18 二轮放开）
//
// 旧口径：前端只允许 8 个「后端 invoiceFieldOrder 认的」字段配正则，其余置灰并挂
// 「配置后正式提取不会执行」告警条。用户看到的是"能配的不够用"。
// 新口径：后端补洞已覆盖 全部标准字段 + 自定义扩展字段 + 明细子项，
// 前端因此取消字段名白名单——只保留「本模块是否接入」这一条边界。
// ---------------------------------------------------------------------------

test('标准字段全部可配正则（含此前的数值型/扩展型字段）', () => {
  const keys = [
    'invoice_no', 'invoice_code', 'invoice_date', 'invoice_type',
    'buyer_name', 'buyer_tax_no', 'seller_name', 'seller_tax_no', 'issuer',
    // 此前被置灰的那批：现在都走「清洗后转数值/写扩展桶」回填
    'total_amount', 'amount', 'tax', 'tax_rate', 'remark',
  ]
  for (const k of keys) assert.equal(isRegexPatchable('invoice', k), true, `${k} 应可配正则`)
  // 前端配置键是 camelCase，同样必须命中
  for (const k of ['invoiceNo', 'totalAmount', 'taxRate', 'sellerTaxNo']) {
    assert.equal(isRegexPatchable('invoice', k), true, `${k} 应可配正则`)
  }
})

test('自定义扩展字段（含中文 key）同样可配正则', () => {
  for (const k of ['车牌号', '户号', '通行区间', '收票编号', 'account_no']) {
    assert.equal(isRegexPatchable('invoice', k), true, `${k} 应可配正则（写进扩展字段桶）`)
  }
})

test('未接入字段级校验的 scope 与空字段名不误判', () => {
  assert.equal(isRegexPatchable('contract', 'contractNo'), false)
  assert.equal(isRegexPatchable('vehicle', '车牌号'), false)
  assert.equal(isRegexPatchable('award_punish', 'apNo'), false)
  assert.equal(isRegexPatchable('unknown_scope', 'invoiceNo'), false)
  assert.equal(isRegexPatchable('invoice', ''), false)
  assert.equal(isRegexPatchable('invoice', '   '), false)
})

test('regexDisabledReason 只在「未接入 / 未命名」时给原因', () => {
  assert.equal(regexDisabledReason('invoice', 'invoiceNo'), '')
  assert.equal(regexDisabledReason('invoice', '车牌号'), '')
  assert.equal(regexDisabledReason('invoice', 'totalAmount'), '')
  assert.match(regexDisabledReason('contract', 'contractNo'), /暂未接入/)
  assert.match(regexDisabledReason('unknown_scope', 'x'), /暂未接入/)
  assert.match(regexDisabledReason('invoice', ''), /命名/)
})

test('结构体标准列清单覆盖 22 个发票契约键（决定补洞写标准列还是扩展桶）', () => {
  assert.equal(INVOICE_STRUCT_KEYS.length, 22)
  assert.equal(isInvoiceStructKey('invoiceNo'), true)
  assert.equal(isInvoiceStructKey('total_amount'), true)
  assert.equal(isInvoiceStructKey('车牌号'), false)
})

// ---- 源码级锁定：规则面板不再按字段名置灰、不再挂 8 字段告警 ----

const panel = readFileSync(new URL('./ExtractRulePanel.vue', import.meta.url), 'utf8')

test('正则不再按字段名白名单置灰（置灰只剩「本模块是否接入」一条）', () => {
  // 白名单常量与配套判定必须消失：8 键时代已结束
  assert.doesNotMatch(panel, /REGEX_PATCHABLE_KEYS/)
  assert.doesNotMatch(panel, /regexPatchable[\s\S]{0,80}REGEX_PATCHABLE_SET/)
  // 输入框仍按 regexPatchable 控制，但它对发票所有字段恒为 true（见上面 meta 断言）
  const disabled = (panel.match(/:disabled="!regexPatchable\(f\.name\)"/g) || []).length
  assert.ok(disabled >= 2, `行内 + 展开区都应受同一判定控制，实际 ${disabled}`)
  // 必填开关仍走标准键判定（后端校验器认全部标准键）
  assert.match(panel, /:disabled="!isStdField\(f\.name\)"/)
})

test('删除「只支持 8 个字段」告警条，保留「本模块未接入」提示', () => {
  assert.doesNotMatch(panel, /后端补洞器只支持/)
  assert.doesNotMatch(panel, /配置后正式提取不会执行/)
  assert.doesNotMatch(panel, /:message="regexDisabledReason\(scope, f\.name\)"/)
  assert.match(panel, /本模块暂未接入字段级校验与正则补洞/)
})

test('明细子项可逐项配置提取口径与正则（随 sub_fields 持久化）', () => {
  assert.match(panel, /function isItemsField/)
  assert.match(panel, /class="sub-rule-block"/)
  assert.match(panel, /v-for="sf in f\.sub_fields \|\| \[\]"/)
  assert.match(panel, /v-model="sf\.rule"/)
  assert.match(panel, /v-model="sf\.regex"/)
  // 加载时按 key 合并：结构以证照配置为准，口径以提取配置为准
  assert.match(panel, /function mergeSubFieldRules/)
  assert.match(panel, /sub_fields: mergeSubFieldRules\(a\.subFields, c\?\.sub_fields\)/)
})

test('规则表去掉行尾展开箭头，整行非控件区仍可展开', () => {
  assert.doesNotMatch(panel, /rtr-toggle/)
  assert.doesNotMatch(panel, /chevron-down" size="15px" \/><\/span>\s*\n\s*<\/div>/)
  // 行本身仍是展开热区（控件区各自 @click.stop）
  assert.match(panel, /class="rule-table-row"[^>]*@click="toggleEdit\(f\.name\)"/)
  assert.match(panel, /<span class="rtr-required" @click\.stop>/)
  assert.match(panel, /<span class="rtr-regex" @click\.stop>/)
})

test('高级模式：左侧 13px 文字标签 + 右侧 medium 开关', () => {
  assert.match(panel, /<span class="adv-label">高级模式<\/span>/)
  assert.match(panel, /<t-switch v-model="advancedEnabled" size="medium" class="adv-switch" \/>/)
  // 不再用 t-switch 的 #label 插槽（文字会跟随开关基线漂移）
  assert.doesNotMatch(panel, /<template #label>高级模式<\/template>/)
})

test('工具栏图标按钮无边框（variant="text"）：样票用票据图标，不再有批量 AI 图标', () => {
  const toolbar = panel.slice(panel.indexOf('<div class="extract-toolbar">'), panel.indexOf('<!-- 测试区锚点'))
  assert.doesNotMatch(toolbar, /variant="outline"/)
  assert.equal((toolbar.match(/<t-button variant="text"/g) || []).length, 2, '样票 + 测试两个无边框图标按钮')
  assert.doesNotMatch(toolbar, /name="star"/)
  assert.doesNotMatch(toolbar, /name="delete"/)
  assert.doesNotMatch(toolbar, /AiSparkIcon/, '批量 AI 图标已下线')
  // 2026-10-18 终轮：样票按钮由 refresh 换成 ticket（refresh 容易被当成"刷新数据"）
  assert.match(toolbar, /<t-icon name="ticket" \/>/)
  // hover 给主题绿图标 + 柔和底色（终轮口径：颜色才是主反馈）
  assert.match(panel, /\.tb-icon-btn[\s\S]{0,300}background: var\(--td-brand-color-1\)/)
})

// ---- 2026-10-18 终轮：键名三形态归一（杜绝界面裸英文）----

test('invoice 标签三种形态都能翻成中文（含历史裸英文 invoice_code）', () => {
  for (const k of ['invoiceCode', 'invoice_code']) {
    assert.equal(extractFieldLabel('invoice', k), '发票代码', `${k} 必须翻成「发票代码」`)
  }
  const pairs: Array<[string, string]> = [
    ['invoiceNo', '发票号码'],
    ['invoice_no', '发票号码'],
    ['invoiceDate', '开票日期'],
    ['invoice_date', '开票日期'],
    ['totalAmount', '价税合计'],
    ['total_amount', '价税合计'],
    ['taxRate', '税率'],
    ['tax_rate', '税率'],
    ['taxAmount', '税额'],
    ['tax_amount', '税额'],
    ['buyerName', '购买方'],
    ['buyer_name', '购买方'],
    ['sellerTaxNo', '销售方税号'],
    ['seller_tax_no', '销售方税号'],
    ['extractStatus', '提取状态'],
    ['extract_status', '提取状态'],
  ]
  for (const [key, want] of pairs) {
    assert.equal(extractFieldLabel('invoice', key), want, `${key} 应翻成「${want}」`)
  }
})

test('未收录的扩展键原样返回（不硬凑中文名）', () => {
  assert.equal(extractFieldLabel('invoice', 'account_no'), 'account_no')
  assert.equal(extractFieldLabel('invoice', '户号'), '户号')
})
