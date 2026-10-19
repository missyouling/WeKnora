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
import '../../registry'
import { getScope } from '../../registry'

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

test('正则置灰判定与必填判定必须分开（required 认全部标准键，regex 只认后端 8 键）', () => {
  // 必填仍走通用校验器 → isStdField 认全部 22 个标准键
  assert.match(panel, /:disabled="!isStdField\(f\.name\)"/)
  // 正则走后端补洞器实际支持集合 → regexPatchable，避免"配了不执行"的静默失效
  assert.match(panel, /:disabled="!regexPatchable\(f\.name\)"/)
  const disabled = (panel.match(/:disabled="!regexPatchable\(f\.name\)"/g) || []).length
  assert.equal(disabled, 2, '行内与展开区两处正则输入都应置灰')
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

// ---------------------------------------------------------------------------
// 2026-10-18 交互重构：图标工具栏 + 直接回填 + 单字段生成 + 内嵌测试
// ---------------------------------------------------------------------------

const panelSrc = readFileSync(new URL('./ExtractRulePanel.vue', import.meta.url), 'utf8')

test('智能生成规则不再走中间预览框，结果直接回填规则表格', () => {
  assert.doesNotMatch(panelSrc, /ai-suggest-panel/, '中间绿色预览框必须移除')
  assert.doesNotMatch(panelSrc, /applyAiSuggestions/, '不再有"勾选后应用"的二次确认')
  assert.match(panelSrc, /async function fillRulesFromAI/, '必须有直接回填实现')
  assert.match(panelSrc, /fields\.value = fields\.value\.map/, '回填必须写回 fields')
})

test('批量「智能生成规则」已下线：无按钮、无函数、无状态', () => {
  // 2026-10-18 终轮：批量按钮与功能一并移除，只保留逐字段生成
  assert.doesNotMatch(panelSrc, /aiBatchTip/)
  assert.doesNotMatch(panelSrc, /智能生成规则（批量）/)
  assert.doesNotMatch(panelSrc, /async function runAiSuggest/)
  assert.doesNotMatch(panelSrc, /aiSuggesting/)
})

test('单字段生成：只传一行且 overwrite=true，并有行级 loading', () => {
  assert.match(panelSrc, /async function generateFieldRule/)
  assert.match(panelSrc, /fillRulesFromAI\(\[\{ name: f\.name \}\], true\)/, '单字段必须 overwrite=true')
  assert.match(panelSrc, /aiFieldBusy = ref\(''\)/, '行级 loading 状态')
  // 行尾 AI 图标已移除，生成入口只在展开区
  assert.doesNotMatch(panelSrc, /class="rtr-ai"/)
  assert.match(panelSrc, /生成该字段规则/, '展开区保留唯一生成入口')
})

test('AI 更新行有徽章与高亮', () => {
  assert.match(panelSrc, /aiFilled = ref<Set<string>>\(new Set\(\)\)/)
  assert.match(panelSrc, /class="rtr-ai-badge">AI</)
  assert.match(panelSrc, /'ai-filled': aiFilled\.has\(f\.name\)/)
  assert.match(panelSrc, /editingName\.value = f\.name/, '单字段生成后展开该行')
})

test('工具栏只剩样票(ticket)与测试(play-circle)两个无边框图标', () => {
  assert.match(panelSrc, /<t-icon name="ticket" \/>/)
  assert.match(panelSrc, /<t-icon name="play-circle" \/>/)
  // 工具栏不再有 AI 批量图标
  const toolbar = panelSrc.slice(panelSrc.indexOf('<div class="extract-toolbar">'), panelSrc.indexOf('<!-- 测试区锚点'))
  assert.doesNotMatch(toolbar, /AiSparkIcon/)
  assert.equal((toolbar.match(/<t-button variant="text"/g) || []).length, 2, '工具栏只应有两个图标按钮')
  assert.match(panelSrc, /shape="square"/, '图标按钮使用方形紧凑形态')
  // 2026-10-18 二轮：整排按钮去描边，hover 只给柔和底色
  assert.doesNotMatch(panelSrc, /<t-button variant="outline" theme="primary" shape="square"/)
  assert.match(panelSrc, /class="tb-icon-btn"/)
})

test('测试规则改为内嵌面板（无模态弹窗）', () => {
  assert.match(panelSrc, /<ExtractTestPanel/)
  assert.doesNotMatch(panelSrc, /ExtractTestDialog/)
  assert.match(panelSrc, /function toggleTest\(\)/)
})

// ---- 2026-10-18 终轮：工具栏图标语义 + 六同步 ----

test('样票图标改 ticket：缓存存在时常驻绿色，hover 平滑变主题绿', () => {
  assert.match(panelSrc, /<t-icon name="ticket" \/>/)
  assert.doesNotMatch(panelSrc, /<t-icon name="refresh" \/>/, '样票不再用 refresh（易被当成刷新按钮）')
  assert.match(panelSrc, /:class="\{ 'is-hot': sampleTextAvailable \}"/)
  assert.match(panelSrc, /&\.is-hot \{\s*color: var\(--td-brand-color\);/, '缓存态常驻绿色')
  assert.match(panelSrc, /transition: color 0\.2s ease/)
  assert.match(panelSrc, /&:hover:not\(:disabled\) \{\s*color: var\(--td-brand-color\)/, 'hover 变主题绿')
})

test('字段中文名优先取分类自定义 label（六同步：改表头后规则表同步）', () => {
  assert.match(panelSrc, /function categoryLabelOf/)
  assert.match(panelSrc, /return categoryLabelOf\(n\) \|\| extractFieldLabel\(props\.scope, n\)/)
  assert.match(panelSrc, /function currentCategory/)
})

test('监听 fleet-categories-changed 广播，无需刷新浏览器即可同步字段增删', () => {
  assert.match(panelSrc, /window\.addEventListener\('fleet-categories-changed', onCategoriesChanged\)/)
  assert.match(panelSrc, /window\.removeEventListener\('fleet-categories-changed', onCategoriesChanged\)/)
  const fn = panelSrc.slice(panelSrc.indexOf('function onCategoriesChanged'), panelSrc.indexOf('function onCategoriesChanged') + 80)
  assert.match(fn, /refresh\(\)/)
})

test('提取规则底稿按三种键形态查找（snake_case 行不再显示「描述 —」）', () => {
  assert.match(panelSrc, /function draftByKeyForms/)
  assert.match(panelSrc, /const d = draftByKeyForms\(draft, a\.name\)/)
  assert.match(panelSrc, /\|\| draftByContains\(draft, a\.name\)/)
})

test('发票公共底稿兜底：发票代码/项目明细等跨分类键也有描述（不再显示「—」）', () => {
  // 底稿已迁入 BusinessScopeRegistry（registry/invoice.ts 的 commonRuleDrafts），
  // 面板只按 scope 取用，不再内嵌 invoice 专属常量。
  const drafts = getScope('invoice')?.commonRuleDrafts || {}
  assert.equal(drafts.invoiceCode?.desc, '发票代码')
  assert.equal(drafts.items?.desc, '项目明细行')
  assert.equal(drafts['车牌号']?.desc, '车牌号码')
  assert.equal(getScope('contract')?.commonRuleDrafts, undefined)
  assert.match(panelSrc, /draftByKeyForms\(commonDrafts\.value, a\.name\)/)
  assert.doesNotMatch(panelSrc, /currentScope\.value === 'invoice'/)
})

test('底稿无命中返回 null（否则空对象真值会短路公共底稿查找）', () => {
  const fn = panelSrc.slice(panelSrc.indexOf('function draftByContains'), panelSrc.indexOf('function draftByContains') + 700)
  assert.match(fn, /return best \|\| null/)
})
