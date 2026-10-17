import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  REGEX_PRESETS,
  canonicalFieldKey,
  extractFieldLabel,
  fieldRulesSupported,
  isStandardField,
  regexSyntaxHint,
} from './extractFieldMeta'

const panel = readFileSync(new URL('./ExtractRulePanel.vue', import.meta.url), 'utf8')

test('规则面板：字段表新增「必填」开关列与「正则兜底」输入列', () => {
  assert.match(panel, /必填/)
  assert.match(panel, /正则兜底/)
  // 必填是开关，直接绑到字段配置的 required
  assert.match(panel, /v-model="f\.required"/)
  // 正则是输入框，绑定 regex
  assert.match(panel, /v-model="f\.regex"/)
  // 表头与行共用同一套列定义（避免两处漂移）
  assert.match(panel, /rule-table-head/)
})

test('正则输入在表格行内不触发展开行（@click.stop）', () => {
  assert.match(panel, /class="rtr-required"/)
  assert.match(panel, /class="rtr-regex"/)
  const regexBlock = panel.slice(panel.indexOf('class="rtr-regex"'))
  assert.match(regexBlock.slice(0, 600), /@click\.stop/)
})

test('内置正则预设可一键填入，且都是 RE2 兼容写法（无环视/反向引用）', () => {
  assert.ok(REGEX_PRESETS.length >= 3)
  const names = REGEX_PRESETS.map((p) => p.name).join('|')
  assert.match(names, /20 位全电发票号/)
  assert.match(names, /18 位统一社会信用代码/)
  assert.match(names, /标准日期/)
  for (const p of REGEX_PRESETS) {
    assert.ok(!p.value.includes('(?<=') && !p.value.includes('(?='), `${p.name} 不得使用环视`)
    assert.ok(!p.value.includes('\\1'), `${p.name} 不得使用反向引用`)
  }
  // 面板里能一键应用（applyRegexPreset 写入字段 regex）
  assert.match(panel, /applyRegexPreset/)
  assert.match(panel, /REGEX_PRESETS/)
})

test('自定义扩展字段：正则输入置灰并给出 Tooltip 说明，标准契约字段可编辑', () => {
  assert.match(panel, /isStdField/)
  assert.match(panel, /自定义扩展字段仅注入 AI 提取 Prompt/)
  // 置灰逻辑：非标准键 / 该模块未接入字段级校验 → disabled
  assert.match(panel, /:disabled="!isStdField\(f\.name\)/)
})

test('标准契约定界：camelCase / snake_case 都算标准键，中文自定义 key 不算', () => {
  assert.equal(isStandardField('invoice', 'invoiceNo'), true)
  assert.equal(isStandardField('invoice', 'invoice_no'), true)
  assert.equal(isStandardField('invoice', 'InvoiceNo'), true)
  assert.equal(isStandardField('invoice', '车牌号'), false)
  assert.equal(isStandardField('contract', 'signDate'), true)
  assert.equal(isStandardField('contract', 'sign_date'), true)
  assert.equal(isStandardField('contract', '补充条款备注'), false)
  // 车队证照未接入通用校验：一律视为不可用，避免"开了不生效"
  assert.equal(fieldRulesSupported('vehicle'), false)
  assert.equal(fieldRulesSupported('invoice'), true)
  assert.equal(canonicalFieldKey('partyA_TaxNo'), 'partyataxno')
})

test('字段中文名解析：发票/合同走映射表，未知 key 原样返回', () => {
  assert.equal(extractFieldLabel('invoice', 'invoiceNo'), '发票号码')
  assert.equal(extractFieldLabel('contract', 'signDate'), '签订日期')
  assert.equal(extractFieldLabel('invoice', '车牌号'), '车牌号')
})

test('正则语法预检：空串与合法正则静默，明显不闭合时给提示（不阻断保存）', () => {
  assert.equal(regexSyntaxHint(''), '')
  assert.equal(regexSyntaxHint('\\b\\d{20}\\b'), '')
  assert.ok(regexSyntaxHint('(\\d{20}').length > 0)
})

test('加载与保存：regex / required 从配置读入并随字段一起提交', () => {
  assert.match(panel, /regex: c\?\.regex \|\| ''/)
  assert.match(panel, /required: c && c\.required !== undefined \? !!c\.required : false/)
  // 保存走 fields 展开提交，不得只挑 name/desc/type/rule 四个键
  assert.match(panel, /const cleanFields = fields\.value\s*\n?\s*\.map\(\(f\) => \(\{ \.\.\.f \}\)\)/)
})

test('表格紧凑：窄屏收缩描述列，正则列保底宽度，避免抽屉内水平挤压', () => {
  const stylePart = panel.slice(panel.indexOf('.rule-table-head,'))
  assert.match(stylePart, /grid-template-columns/)
  assert.match(stylePart, /minmax\(1[0-9]{2}px/)
  assert.match(panel, /eq-narrow/)
})
