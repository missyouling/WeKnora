/**
 * 第 12 轮：打印清单三处口径 + PDF 预览卡死。
 *
 * 这一轮把「PDF 概览卡片」「PDF 分项细项」「鼠标浮窗」统一到与列表同一套算法，
 * 所以测试分两类：
 *   · 行为级：浮窗文案（纯函数）不得出现 `0 × 比例 = 非零金额`；
 *   · 源码级：打印链路必须调用同一个核算函数、概览卡片必须按租户拆两段、
 *     PDF 预览必须有加载序号锁（这几项都在 .vue 里，没有纯函数可测）。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { tenantItemTip, tenantTimedTip } from './tenantBillingTips'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

// ---- ① 浮窗：绝不出现 `= 0 × 21.24% = 4,434.02`

test('浮窗：原账单金额缺失时不得拼出「0 × 比例 = 非零金额」', () => {
  const tip = tenantItemTip({
    itemName: '零售损益分摊电费',
    sourceCategory: '市电账单',
    billAmount: 0,
    ratio: 0.212356,
    value: 4434.02,
    participating: true,
  })
  assert.ok(!/= 0 ×/.test(tip), `不得出现「= 0 ×」：${tip}`)
  assert.ok(!/0 × 21/.test(tip), `不得出现「0 × 21」：${tip}`)
  assert.match(tip, /4,434\.02/)
})

test('浮窗：原账单金额为 undefined 时同样不编造乘式', () => {
  const tip = tenantItemTip({
    itemName: '上网环节线损费',
    ratio: 0.212356,
    value: 2137.89,
    participating: true,
  })
  assert.ok(!/= 0 ×/.test(tip), tip)
  assert.match(tip, /2,137\.89/)
})

test('浮窗：金额齐全时写真实算式（原账单金额 × 分摊比例 = 本行金额）', () => {
  const tip = tenantItemTip({
    itemName: '零售损益分摊电费',
    sourceCategory: '市电账单',
    billAmount: 20880.14,
    ratio: 0.212356,
    value: 4434.02,
    participating: true,
  })
  assert.match(tip, /20,880\.14 × 21\.24% = 4,434\.02 元/)
})

test('浮窗：direct_sum 子项直接计入，不乘比例', () => {
  const tip = tenantItemTip({
    itemName: '偏差电费',
    billAmount: 1234.5,
    ratio: 0.2,
    value: 1234.5,
    participating: true,
    directSum: true,
  })
  assert.match(tip, /直接计入/)
  assert.ok(!/× 20\.00%/.test(tip), tip)
})

test('浮窗：分时子项只写「基准电量 × 时段单价」，不乘比例', () => {
  const tip = tenantTimedTip({
    itemName: '零售交易电费',
    deltaQ: 3900,
    totalUsage: 122700,
    totalBill: 120922.91,
    periods: [{ period: '尖峰', qty: 5085.43, rate: 0.436, fee: 2217.25 }],
    value: 2217.25,
  })
  assert.ok(!/21\.24%/.test(tip), tip)
  assert.match(tip, /5,085\.43 度 × 0\.436 = 2,217\.25 元/)
  assert.match(tip, /不乘分摊比例/)
})

test('浮窗：未参与分摊的项明确写「本行金额记 0」', () => {
  const tip = tenantItemTip({
    itemName: '偏差电费', ratio: 0.2, value: 0, participating: false,
  })
  assert.match(tip, /未开启「参与分摊」/)
})

// ---- ② 打印链路必须复用列表的核算引擎

test('打印细项表：分时子项走 resolveTimedItemFee（不再按账单行 × 比例）', () => {
  // 第 22 轮 P1-2：构造器已从 TenantBilling.vue 迁到 energyBillStatements.ts
  const statements = read('energyBillStatements.ts')
  const feeTable = statements.slice(statements.indexOf('function feeTableOf'), statements.indexOf('function elecTableOf'))
  // 第 40 轮：cat 已提前归一成局部变量（6 列重构），语义不变
  assert.match(feeTable, /resolveTimedItemFee\(rowCtx, key, cat\)/,
    '打印细项必须复用分时核算引擎，且按分组圈定匹配范围')
  // 分时分支必须逐段出行（时段 / 基准电量 / 单价 / 金额）
  assert.match(feeTable, /for \(const p of timed\.periods\)/)
  assert.match(feeTable, /num2\(p\.qty\)/, '计费数量必须是基准电量')
})

test('打印细项表：带上已生成账单，ΔQ 与列表同源', () => {
  const statements = read('energyBillStatements.ts')
  const feeTable = statements.slice(statements.indexOf('function feeTableOf'), statements.indexOf('function elecTableOf'))
  assert.match(feeTable, /ctx\.sourceCtxOf\(month, bill, \[\], rec\)/, '打印必须传 rec，否则差额电量口径分叉')
  // 已生成账单必须由调用方按月份传进来（不再从组件 records 里捞）
  assert.match(feeTable, /ctx\.recordOf\(month\)/, 'ΔQ 的固化值必须按月份取')
})

test('列表与打印共用「账单原始金额」唯一实现', () => {
  const sources = read('tenantBillingSources.ts')
  assert.match(sources, /export function billRawAmountOf/)
  const billing = read('TenantBilling.vue')
  assert.match(billing, /const billAmount = participating && key \? billRawAmountOf\(ctx, key, grp\) : 0/,
    '浮窗账单原始金额必须真的去账单里取（旧实现传 undefined → 0 × 比例），且按分组圈定')
  // 不允许再退回「只传固定金额分项」的老写法
  assert.ok(!/billAmount: fixed !== null \? fixed : undefined/.test(billing))
})

// ---- ③ 概览卡片必须按租户拆「工业 + 宿舍」

test('概览卡片：租户视角按工业/宿舍两段拆，业主视角保持全厂口径', () => {
  const statements = read('energyBillStatements.ts')
  const fn = statements.slice(statements.indexOf('function statementKpis'), statements.indexOf('function statementMeta'))
  assert.match(fn, /ctx\.ownerView/, '必须区分业主/租户两套口径')
  for (const k of ['ind_kwh', 'dorm_kwh', 'ind_fee', 'dorm_fee', 'water_ind_usage', 'water_dorm_usage', 'water_ind_fee', 'water_dorm_fee']) {
    assert.ok(fn.includes(`'${k}'`), `概览卡片缺少 ${k}`)
  }
  assert.match(fn, /hint:/, '卡片必须给出构成说明（工业 x + 宿舍 y）')
  // 租户视角不得再出现「全厂本期电费合计」
  const tenantPart = fn.slice(fn.indexOf('const indKwh'))
  assert.ok(!/total_fee/.test(tenantPart), '租户卡片不得用全厂 total_fee')
})

test('概览卡片：打印清单只有浮动栏一条入口，共用 buildStatementDoc', () => {
  const billing = read('TenantBilling.vue')
  const statements = read('energyBillStatements.ts')
  // 卡片/元信息只在构造器里拼一次；页面只负责把上下文传进去
  assert.equal((statements.match(/kpis: statementKpis\(/g) || []).length, 1, '卡片构造只应有一处')
  assert.equal((statements.match(/meta: statementMeta\(/g) || []).length, 1, '元信息构造只应有一处')
  assert.equal((billing.match(/kpis: statementKpis\(/g) || []).length, 0, '页面不得再自带一份卡片构造')
  const statement = billing.slice(billing.indexOf('const handlePrintStatement'), billing.indexOf('const handlePrintCatalog'))
  assert.match(statement, /statementBytesOf\(arr, unit, \(\) => buildStatementDoc\(arr, unit\)\)/)
  // 第 18 轮：详情抽屉已下线（行点击不再弹抽屉），抽屉里的「打印」入口随之移除，
  // 打印清单的唯一入口是底部浮动栏。这里反向断言，防止抽屉被悄悄加回来。
  assert.ok(!/printRecord/.test(billing), '抽屉底栏打印入口应随抽屉一起下线')
  assert.ok(!/BusinessSplitDetailDrawer/.test(billing), '费用清单不得再挂载详情抽屉')
})

test('概览元信息：租户视角展示「电水合计应收」', () => {
  const statements = read('energyBillStatements.ts')
  const fn = statements.slice(statements.indexOf('function statementMeta'), statements.indexOf('function feeTableOf'))
  assert.match(fn, /电水合计应收/)
})

test('PDF 卡片渲染：有 hint 时加高卡片，标题/构成/数值三行不重叠', () => {
  const pdf = read('useCatalogPdf.ts')
  // 第 41 轮紧凑化：54/42 → 50/40，仍要求有 hint 的卡片更高（三行文字不重叠）
  assert.match(pdf, /const cardH = hasHint \? 50 : 40/)
  assert.match(pdf, /if \(k\.hint\) \{/)
})

// ---- ④ PDF 预览：加载序号锁，绝不卡在转圈

test('PdfSourceViewer 用递增序号锁，过期的那次不写 loading/pageCount', () => {
  const src = readFileSync(new URL('../../components/source-preview/PdfSourceViewer.vue', import.meta.url), 'utf8')
  assert.match(src, /let loadSeq = 0/)
  assert.match(src, /const seq = \+\+loadSeq/)
  assert.match(src, /if \(seq === loadSeq\) loading\.value = false/)
  // 过期任务不得污染状态
  assert.match(src, /if \(disposed \|\| seq !== loadSeq\) return/)
  // src 变化必须重新加载
  assert.match(src, /watch\(\(\) => props\.data, \(\) => \{ void load\(\) \}\)/)
  // 首屏主动渲染，避免 IntersectionObserver 在弹窗刚挂载时判定不可见
  assert.match(src, /void renderPage\(1\)/)
})

test('打印弹窗给 PdfSourceViewer 绑唯一 key，复用弹窗时不沿用旧文档', () => {
  const src = read('PrintPreviewDialog.vue')
  assert.match(src, /:key="viewerSeq"/)
  assert.match(src, /viewerSeq\.value\+\+/)
})

test('PDF 文本层用 getTextContent（可重渲染），不用一次性 streamTextContent', () => {
  const src = readFileSync(new URL('../../components/source-preview/PdfSourceViewer.vue', import.meta.url), 'utf8')
  assert.match(src, /await page\.getTextContent\(\)/)
  assert.ok(!/page\.streamTextContent\(/.test(src),
    'streamTextContent 是一次性流，同页重渲染后文本层会变空（可看不可选）')
})

test('复用单个 PDFWorker：第二次打开预览不再重启线程', () => {
  const loader = readFileSync(new URL('../../components/source-preview/pdfjsLoader.ts', import.meta.url), 'utf8')
  assert.match(loader, /export function getPdfWorker/)
  assert.match(loader, /if \(!workerPromise\)/)
  assert.match(loader, /export function warmupPdfJs/)
  const viewer = readFileSync(new URL('../../components/source-preview/PdfSourceViewer.vue', import.meta.url), 'utf8')
  assert.match(viewer, /getDocument\(worker \? \{ data: new Uint8Array\(data\.slice\(0\)\), worker \}/,
    'worker 必须显式传入，才能跨文档复用')
})

// ---------------------------------------------------------------------------
// 第 36 轮：三段式 hint 必须完整展示，不能按字符截断
// ---------------------------------------------------------------------------
//
// 「工业 166550 + 差额 5850 + 宿舍 686」比两段式长，7pt 下超出卡片宽度。
// 旧实现 `while (宽 > maxW) hint = hint.slice(0,-1)` 把它截成
// 「工业 166550 + 差额 5850 + …」—— 沙发那一段和宿舍数都看不见，
// 三项之和也没法与下方大字值核对（需求明确要求三数相加严格等于大字值）。

test('PDF 概览卡片：hint 用自适应字号完整展示，禁止固定 7pt 按字符截断', () => {
  const pdf = read('useCatalogPdf.ts')
  const seg = pdf.slice(pdf.indexOf('if (k.hint) {'), pdf.indexOf('const vSize = 11.5'))
  assert.match(seg, /6\.5|6, 5\.5|自适应字号|完整展示/, '必须有降字号逻辑（而非只按字符切）')
  // 不能再出现「固定 7pt 边切边比」的旧写法
  assert.ok(!/font\.widthOfTextAtSize\(hint, 7\) > maxW\) hint = hint\.slice/.test(seg),
    '固定 7pt 按字符截断会把三段式截成「…」')
  assert.match(seg, /hintSize/, '绘制时必须用自适应后的字号，不能恒为 7')
})

// ---------------------------------------------------------------------------
// 第 37 轮：打印清单的「分摊比例」只显示百分比，不带括号适用范围说明
// ---------------------------------------------------------------------------
//
// 括号说明「（仅基本电费、功率因素调整电费按比例）」属于口径细节，
// 打印页 metas 是两列排布，文案一长就换行挤掉后一行。
test('打印清单：分摊比例只显示百分比，不再附带括号说明', () => {
  const src = read('energyBillStatements.ts')
  assert.ok(!/仅基本电费、功率因素调整电费按比例/.test(
    src.split('\n').filter((l) => !l.trim().startsWith('//')).join('\n')),
    '「仅基本电费…」不得出现在输出文案里（注释除外）')
  assert.match(src, /\(ratio \* 100\)\.toFixed\(2\)\}%`/, '必须仍输出百分比')
})
