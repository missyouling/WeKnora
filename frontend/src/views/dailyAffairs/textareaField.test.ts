import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  TEXTAREA_COL_WIDTH,
  cellClassOf,
  colWidthOf,
  ellipsisOf,
  isTextareaColumn,
  tableScrollOf,
} from './columnWidth'
import { buildColumnsFromSubs, buildDrawerFieldsFromSubs, buildRuleFieldsFromSubs } from './sixWaySync'
import { wrapParagraphs } from './useCatalogPdf'

/**
 * 文本字段细分为单行/多行两种模式（2026-10-22）。
 *
 * 真机事故：备注/摘要等长文本字段只有 `text` 一种类型，详情抽屉渲染成单行
 * `<t-input>`，长内容（「业务流水号:2026102215...」）被截断，用户既看不全也录不全。
 * 此前靠硬编码字段名白名单兜底，白名单之外的备注、说明一律单行，用户无从选择。
 *
 * 本文件锁定 textarea 这条链路的四处契约：
 *   ① 类型判定：只有 `data_type === 'textarea'` 才算多行（text 不得误判）；
 *   ② 列表列：多行文本列必须关 ellipsis、走固定中等列宽、挂折行类名；
 *   ③ 虚拟滚动：出现多行文本列必须关掉 isFixedRowHeight（否则行错位）；
 *   ④ 六同步：subs 的 textarea 必须原样传到列表列/抽屉字段/提取规则三处。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

test('isTextareaColumn 只认 textarea', () => {
  assert.equal(isTextareaColumn({ dataType: 'textarea' }), true)
  assert.equal(isTextareaColumn({ dataType: 'text' }), false)
  assert.equal(isTextareaColumn({ dataType: 'number' }), false)
  assert.equal(isTextareaColumn({ dataType: '' }), false)
  assert.equal(isTextareaColumn({}), false)
  assert.equal(isTextareaColumn(null), false)
  assert.equal(isTextareaColumn(undefined), false)
  // 大小写/空格污染（存量 JSON 手改过）不应被当成多行
  assert.equal(isTextareaColumn({ dataType: ' textarea ' }), false, '未 trim 视为非 textarea（后端归一后才是权威值）')
})

test('多行文本列：关 ellipsis、挂折行类名', () => {
  const multi = { key: 'remark', label: '备注', dataType: 'textarea' }
  const single = { key: 'contract_no', label: '合同编号', dataType: 'text' }
  assert.equal(ellipsisOf(multi), false, '多行文本列必须关 ellipsis（否则 nowrap 吃掉折行）')
  assert.equal(ellipsisOf(single), true, '单行文本列保持截断')
  assert.equal(cellClassOf(multi), 'biz-cell--multiline')
  assert.equal(cellClassOf(single), undefined)
})

test('多行文本列宽：未配置宽度时走固定中等宽度，不按内容估算', () => {
  // 一段 200 字的长文本，内容自适应会算出封顶 150，对折行展示过窄
  const longText = '业务流水号:2026102215000001；'.repeat(10)
  const auto = colWidthOf(0, '备注', [longText])
  assert.equal(auto, 150, '普通文本列仍走内容自适应（封顶 150）')
  assert.equal(colWidthOf(0, '备注', [longText], 'textarea'), TEXTAREA_COL_WIDTH)
  // 用户显式配置的宽度优先，且仍受 clamp 保护
  assert.equal(colWidthOf(320, '备注', [longText], 'textarea'), 320)
  assert.equal(colWidthOf(9999, '备注', [longText], 'textarea'), 400, '显式宽度上限 400')
  assert.equal(colWidthOf(10, '备注', [longText], 'textarea'), 60, '显式宽度下限 60')
})

test('虚拟滚动：出现多行文本列必须关掉固定行高', () => {
  const plain = tableScrollOf([{ dataType: 'text' }, { dataType: 'number' }])
  assert.deepEqual(plain, { type: 'virtual', rowHeight: 48, isFixedRowHeight: true, bufferSize: 20 })

  const withMulti = tableScrollOf([{ dataType: 'text' }, { dataType: 'textarea' }])
  assert.equal(withMulti.isFixedRowHeight, false, '多行文本列折行后行高不再是 48，必须逐行测量')
  assert.equal(withMulti.rowHeight, 48, '初始估算值仍是 48（测量前的兜底）')
  assert.equal(withMulti.type, 'virtual')
  assert.equal(withMulti.bufferSize, 20)

  // 空列集不应误判
  assert.equal(tableScrollOf([]).isFixedRowHeight, true)
})

test('六同步：subs 的 textarea 原样派生到列表列 / 抽屉字段 / 提取规则', () => {
  const subs = [
    { name: 'contract_no', label: '', enabled: true, is_default: true, data_type: 'text' },
    { name: 'remark', label: '备注', enabled: true, is_default: false, data_type: 'textarea' },
    { name: 'summary', label: '摘要2', enabled: true, is_default: false, data_type: 'textarea' },
  ]
  const labelOf = (n: string) => ({ contract_no: '合同编号', remark: '备注', summary: '摘要' }[n] || n)

  const cols = buildColumnsFromSubs(subs, { labelOf })
  assert.equal(cols.find((c) => c.key === 'remark')!.dataType, 'textarea')
  assert.equal(cols.find((c) => c.key === 'summary')!.dataType, 'textarea')
  // 用户改名后的表头不能被字典盖回
  assert.equal(cols.find((c) => c.key === 'summary')!.label, '摘要2')
  assert.equal(cols.find((c) => c.key === 'contract_no')!.dataType, 'text')

  const drawerFields = buildDrawerFieldsFromSubs(subs, { labelOf })
  assert.equal(drawerFields.find((f) => f.name === 'remark')!.dataType, 'textarea')

  const ruleFields = buildRuleFieldsFromSubs(subs, { labelOf, savedByKey: new Map() })
  // 提取配置方言：string 是单行文本，textarea 保留（规则面板要显示「多行文本」）
  assert.equal(ruleFields.find((f) => f.name === 'remark')!.type, 'textarea')
  assert.equal(ruleFields.find((f) => f.name === 'contract_no')!.type, 'string')
})

test('抽屉：textarea 分支排在字段名白名单兜底之前', () => {
  const src = read('BusinessSplitDetailDrawer.vue')
  const textareaAt = src.indexOf(`f.dataType === 'textarea'`)
  const legacyAt = src.indexOf('isLongTextField(f)')
  assert.ok(textareaAt > 0, '抽屉必须有 dataType=textarea 分支')
  assert.ok(legacyAt > 0, '字段名白名单兜底必须保留（存量数据未迁移时仍要长文本域）')
  assert.ok(
    textareaAt < legacyAt,
    '用户显式选择的类型必须优先于字段名兜底，否则改回单行文本不生效',
  )
  // 分支控件必须是 t-textarea（不是 t-input）
  const branch = src.slice(textareaAt - 120, textareaAt + 200)
  assert.match(branch, /<t-textarea/)
  assert.match(branch, /autosize/, '多行编辑区必须 autosize，长内容自动撑高')
  // 占满整行：多行字段在双列栅格里并排会过窄
  assert.match(src, /'bsd-field--full':[^\n]*f\.dataType === 'textarea'/)
})

test('字段定义面板：单行文本 / 多行文本两个选项并存', () => {
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /\{ label: '单行文本', value: 'text' \}/)
  assert.match(panel, /\{ label: '多行文本', value: 'textarea' \}/)
  // 主字段与明细子项都要有（子项也会承载长说明）
  const mainAt = panel.indexOf('const dataTypeOptions')
  const subAt = panel.indexOf('const subDataTypeOptions')
  assert.ok(mainAt > 0 && subAt > 0)
  assert.match(panel.slice(mainAt, mainAt + 400), /value: 'textarea'/)
  assert.match(panel.slice(subAt, subAt + 400), /value: 'textarea'/)

  // AI 字段建议面板类型下拉同步（否则 AI 推断出的多行字段落成单行）
  const ai = read('AiFieldSuggestPanel.vue')
  assert.match(ai, /value: 'textarea'/)
})

test('提取规则面板：多行文本与字段定义同口径', () => {
  const src = read('ExtractRulePanel.vue')
  assert.match(src, /\{ label: '多行文本', value: 'textarea' \}/)
  // subs → 提取配置的类型换算必须保留 textarea（归一成 string 会六同步断裂）
  const fnAt = src.indexOf('function subsTypeToExtractType')
  assert.ok(fnAt > 0)
  assert.match(src.slice(fnAt, fnAt + 500), /case 'textarea': return 'textarea'/)
})

test('四个页面的列定义按 dataType 决定 ellipsis / className', () => {
  for (const f of ['InvoiceManagement.vue', 'ContractManagement.vue', 'RegulationManagement.vue', 'AwardPunishManagement.vue']) {
    const src = read(f)
    assert.match(src, /ellipsisOf\(c as any\)/, `${f} 列 ellipsis 必须按列类型判定`)
    assert.match(src, /cellClassOf\(c as any\)/, `${f} 必须给多行文本列挂折行类名`)
    // 列对象必须把 dataType 继续下传：tableScrollOf(tableColumns) 靠它判断要不要关
    // 固定行高。漏传的后果是「列已折行、滚动仍按 48px 推算」→ 行错位（真机踩过）。
    assert.match(src, /dataType: \(c as any\)\.dataType/, `${f} 列对象必须带 dataType（供 tableScrollOf 判定）`)
    // 列宽必须把 dataType 传给 colWidthOf（多行文本走固定中等宽度）。
    // 发票页的 values 是跨行的 map 表达式，因此只断言「colWidthOf(...  (c as any).dataType)」
    // 这一段之间不出现闭合的 `})` 之外的分号。
    const at = src.indexOf('colWidthOf(c.width')
    assert.ok(at > 0, `${f} 必须调用 colWidthOf`)
    const seg = src.slice(at, src.indexOf('\n', src.indexOf('(c as any).dataType', at)))
    assert.match(seg, /\(c as any\)\.dataType\)/, `${f} 列宽必须把 dataType 传进工具`)
    // 虚拟滚动必须走 tableScrollOf(tableColumns)：写死 isFixedRowHeight:true 会让折行失效
    assert.match(src, /:scroll="tableScroll"/, `${f} 表格必须绑定 tableScroll（按列类型动态判定固定行高）`)
    assert.doesNotMatch(src, /isFixedRowHeight: true/, `${f} 不得再写死固定行高`)
  }
})

test('共享样式：多行文本单元格 pre-wrap 折行且不设 max-height', () => {
  const css = read('styles/business-list-shared.css')
  const at = css.indexOf('td.biz-cell--multiline')
  assert.ok(at > 0, '共享样式必须包含 .biz-cell--multiline 规则')
  const block = css.slice(at, css.indexOf('}', css.indexOf('line-height: 20px', at)))
  assert.match(block, /white-space: pre-wrap;/, '必须保留用户录入的换行')
  assert.match(block, /word-break: break-word;/, '长串中文必须按字符折行而不是撑爆列宽')
  assert.match(block, /overflow: visible;/, '不得隐藏溢出内容')
  assert.match(block, /text-overflow: clip;/, '不得用省略号截断')
  // 需求是「列表页完整显示」，因此绝不设 max-height
  const whole = css.slice(at, at + 800)
  assert.doesNotMatch(whole, /max-height/, '多行文本单元格不得限制最大高度（否则内容仍被裁掉）')
})

test('目录 PDF：换行符按行切段，不被 drawText 吞掉', () => {
  const src = read('useCatalogPdf.ts')
  assert.match(src, /export function splitExplicitLines/, '必须有显式换行切分')
  assert.match(src, /replace\(\/\\r\\n\?\/g, '\\n'\)/, '必须统一 \\r\\n / \\r 为 \\n')
  // 折行逻辑必须走 wrapParagraphs（纯函数，行为由下方用例锁定），
  // wrapText 只负责把字体度量喂进去
  const at = src.indexOf('function wrapText')
  const body = src.slice(at, src.indexOf('\nexport async function generateCatalogPdf', at))
  assert.match(body, /wrapParagraphs\(/, 'wrapText 必须委托 wrapParagraphs 折行')
  // 列宽估算同样按最长单行（否则一段长备注会把整列撑到上限）
  assert.match(src, /splitExplicitLines\(c\.value\(r\) \|\| ''\)/)
})

test('目录 PDF 折行行为：显式换行保留、超宽折行、空行与制表符正确', () => {
  // 1 单位/字符的假度量：便于用「字符数」直读列宽
  const measure = (s: string) => [...s].length

  // ① 显式换行必须各占一行：整串逐字折行会把两段黏成一行（原实现的问题）
  assert.deepEqual(wrapParagraphs(measure, '甲方：甲公司\n乙方：乙公司', 100), ['甲方：甲公司', '乙方：乙公司'])
  // ② \r\n / \r / \n 三种换行统一处理
  assert.deepEqual(wrapParagraphs(measure, 'A\r\nB\rC\nD', 100), ['A', 'B', 'C', 'D'])
  // ③ 超宽按宽度折行，且不丢字符
  assert.deepEqual(wrapParagraphs(measure, 'abcdefghij', 4), ['abcd', 'efgh', 'ij'])
  // ④ 段内空行保留（段落间距），否则多段文本会被压成一整块
  assert.deepEqual(wrapParagraphs(measure, 'A\n\nB', 100), ['A', '', 'B'])
  // ⑤ 制表符展开为 4 空格：宽度计算与渲染同一口径
  assert.deepEqual(wrapParagraphs(measure, 'A\tB', 100), ['A    B'])
  // ⑥ 尾部空行裁掉（否则多算一段行高）；全空文本保留一个空行占位
  assert.deepEqual(wrapParagraphs(measure, 'A\n\n', 100), ['A'])
  assert.deepEqual(wrapParagraphs(measure, '', 100), [''])
  // ⑦ 长中文按字符折行（CJK 无空格可断，必须逐字）
  assert.deepEqual(wrapParagraphs(measure, '一二三四五', 2), ['一二', '三四', '五'])
})

test('AI 字段合并：textarea 类型不被降级成 text', () => {
  const src = read('aiFieldMerge.ts')
  assert.match(
    src,
    /\['number', 'date', 'array', 'textarea'\]\.includes\(String\(c\.data_type\)\)/,
    '自定义字段的 textarea 必须原样透传，否则抽屉退回单行输入框',
  )
})
