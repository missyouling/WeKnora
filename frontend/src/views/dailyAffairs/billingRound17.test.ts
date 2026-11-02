/**
 * 第 17 轮实测反馈四项修复的回归锁。
 *
 *   ① 租户打印清单不得混入「别人的气费」（2026-09 持睿汽车多出 1,188.20）；
 *   ② 市电/光伏浮动工具栏统一为 5 按钮，删除文案全模块统一；
 *   ③ 弃用「数据不全禁止生成」门禁（前后端一起放开）；
 *   ④ 打印目录超多列不越界、数字不单字断行。
 *
 * 其中 ④ 的算法行为由 catalogLayout.test.ts 覆盖，这里只锁「有没有接上」。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const readGo = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

// ---- ① 气费按使用单位隔离 ----

test('气费汇总：必须按使用单位过滤（旧实现只按月 → 把全厂气费加在一起）', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('const gasOf ='), src.indexOf('const base: Record<string, any>'))
  assert.match(fn, /gasOf = \(unit: string\)/, 'gasOf 必须接收使用单位')
  assert.match(fn, /String\(r\.use_unit \|\| ''\)\.trim\(\) === unit/, '必须按 use_unit 精确匹配')
  assert.ok(!/gasOf\(\)/.test(src), '不得再有无参调用（等于不过滤）')
  // 两个视角各按自己的单位取数
  assert.match(src, /gasOf\(OWNER_UNIT\)/, '业主视角按星达取气表')
  assert.match(src, /gasOf\(tenantName\)/, '租户视角按当前租户取气表')
})

test('打印清单：无气表/未启用气费时，既不出气费明细表也不出「气费」汇总行', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /function gasApplicableOf\(/, '必须有「气费是否适用」判定')
  const fn = src.slice(src.indexOf('function gasApplicableOf'), src.indexOf('function summaryTableOf'))
  assert.match(fn, /hasMeter/, '必须检查该单位确有气表记录')
  assert.match(fn, /activeColDefs\.value\.some\(\(c\) => c\.key === 'gas_fee'\)/, '必须检查气费字段是否启用')

  const sum = src.slice(src.indexOf('function summaryTableOf'), src.indexOf('打印清单的字节缓存'))
  assert.match(sum, /includeGas: boolean/, '应收汇总必须接收 includeGas')
  assert.match(sum, /if \(includeGas\) body\.push\(\['气费'/, '气费行必须条件渲染')
  assert.ok(!/rows: \[[\s\S]*?\['气费', unit, money2\(gas\)\][\s\S]*?\],\n\s*total:/.test(sum),
    '气费行不得写死在 rows 字面量里')

  const build = src.slice(src.indexOf('function buildStatementDoc'), src.indexOf('const statementPrewarm'))
  assert.match(build, /const includeGas = gasApplicableOf\(unit, months\)/, '整份清单统一判定气费适用性')
  assert.match(build, /if \(includeGas\) \{/, '气费明细表必须受 includeGas 控制')
  assert.match(build, /summaryTableOf\(\[row\], unit, prefix, includeGas\)/, '汇总表必须传 includeGas')
})

test('概览元信息：无气表的租户不得把 gas_fee 并进应收合计', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('function statementMeta'), src.indexOf('function feeTableOf'))
  assert.match(fn, /includeGas = false/, '元信息必须接收气费适用标记')
  assert.match(fn, /const gas = includeGas \? sum\('gas_fee'\) : 0/, '不适用时气费按 0 计')
  assert.match(fn, /tenantTotal = power \+ water \+ gas/)
})

// ---- ② 浮动工具栏 ----

const BUTTON_ORDER = ['重新解析', '重新提取', '打印清单', '打印目录', '删除']

function assertToolbar(src: string, label: string) {
  const start = src.indexOf('#batch-actions') >= 0 ? src.indexOf('#batch-actions') : src.indexOf('batch-bar-actions')
  assert.ok(start > 0, `${label} 找不到浮动栏插槽`)
  // 浮动栏整段：插槽起点往后 5000 字符足够覆盖 5 个按钮
  const body = src.slice(start, start + 5000)
  const positions = BUTTON_ORDER.map((b) => {
    // 只认「独占一行的按钮文本节点」，避开 popconfirm 文案里的同名词
    const m = new RegExp(`\\n\\s*${b}\\s*\\n`).exec(body)
    assert.ok(m, `${label} 浮动栏缺少按钮「${b}」`)
    return m!.index
  })
  assert.deepEqual([...positions].sort((a, b) => a - b), positions,
    `${label} 按钮顺序必须是 ${BUTTON_ORDER.join(' → ')}`)
  // 只看「独占一行的按钮文本节点」：注释里解释「已删除编辑数据」不算违规
  assert.ok(!/\n\s*编辑数据\s*\n/.test(body), `${label} 不得再有「编辑数据」按钮`)
  assert.ok(!/\n\s*删除记录\s*\n/.test(body), `${label} 删除按钮文案必须统一为「删除」`)
}

test('市电账单浮动栏：5 个按钮、顺序正确、无编辑数据', () => {
  assertToolbar(read('UtilitiesManagement.vue'), '市电账单')
})

test('光伏账单浮动栏：5 个按钮、顺序正确、无编辑数据', () => {
  assertToolbar(read('SolarManagement.vue'), '光伏账单')
})

test('重新解析与重新提取分工明确：解析走 reparse，提取走专用接口', () => {
  const util = read('UtilitiesManagement.vue')
  const reparse = util.slice(util.indexOf('const handleReparse'), util.indexOf('const handleReExtract'))
  assert.match(reparse, /await reparseKnowledge\(r\.knowledgeId\)/, '重新解析必须调 reparseKnowledge')
  assert.ok(!/extractUtilityBill/.test(reparse), '重新解析不得只调提取')
  const reextract = util.slice(util.indexOf('const handleReExtract'), util.indexOf('// ---- 打印'))
  assert.match(reextract, /await extractUtilityBill\(kbId\.value, r\.knowledgeId\)/, '重新提取必须调 extract-utility-bill')

  const solar = read('SolarManagement.vue')
  const sReparse = solar.slice(solar.indexOf('const handleReparse'), solar.indexOf('const handleReExtract'))
  assert.match(sReparse, /await reparseKnowledge\(row\.knowledgeId\)/)
  const sReextract = solar.slice(solar.indexOf('const handleReExtract'), solar.indexOf('const handleBatchDelete'))
  assert.match(sReextract, /await extractSolarBill\(kbId\.value, row\.knowledgeId\)/)
})

test('打印清单/目录：分别走结算单与目录生成器，不再合并源 PDF', () => {
  for (const [file, builder] of [
    ['UtilitiesManagement.vue', 'buildUtilityBillStatementDoc'],
    ['SolarManagement.vue', 'buildSolarBillStatementDoc'],
  ] as const) {
    const src = read(file)
    assert.match(src, /generateStatementPdf\(\{ doc: [a-zA-Z]+\(rowsSel\) \}\)/, `${file} 打印清单必须走 generateStatementPdf`)
    assert.match(src, new RegExp(`${builder}\\(rowsSel\\)`), `${file} 必须复用 ${builder}`)
    assert.match(src, /generateCatalogPdf\(\{/, `${file} 打印目录必须走 generateCatalogPdf`)
    assert.match(src, /visibleColDefs\.value\.map\(\(c\) => \(\{/, `${file} 目录列必须来自已启用表头`)
    assert.ok(!/previewKnowledgeFile\(r\.knowledgeId\)/.test(src), `${file} 不得再合并源 PDF`)
  }
})

test('删除按钮文案全模块统一（无「删除记录」残留）', () => {
  const dir = new URL('.', import.meta.url)
  const files = readdirSync(dir).filter((f) => f.endsWith('.vue'))
  const offenders: string[] = []
  for (const f of files) {
    const src = read(f)
    // 注释里提到旧文案不算违规，只看标签文本行
    if (/\n\s*删除记录\s*\n/.test(src)) offenders.push(f)
  }
  assert.deepEqual(offenders, [], `仍有页面用「删除记录」：${offenders.join('、')}`)
})

// ---- ③ 门禁移除 ----

test('前端：生成账单不再有「数据不全禁止生成」门禁', () => {
  const src = read('TenantBilling.vue')
  assert.ok(!/function readinessOf/.test(src), '不得保留 readinessOf')
  assert.ok(!/已禁止生成/.test(src), '不得保留禁止生成提示')
  const fn = src.slice(src.indexOf('const handleGenerateAll'), src.indexOf('// 数据齐全时默认自动生成'))
  assert.ok(!/issues/.test(fn), '生成流程不得被校验中断')
})

test('后端：缺读数/缺账单一律按 0 计算，不再返回 400 阻断生成', () => {
  // 第 22 轮 P0-3：生成流程拆成 prepareBillingInputs / allocateTimed /
  // buildFeeItems / persistRecord，判定逻辑落在 billing_generate.go。
  const go = readGo('internal/handler/billing.go')
  const gen = readGo('internal/handler/billing_generate.go')
  const whole = `${go}\n${gen}`
  // 注释里会引用旧文案，这里只断言「不再以错误形式返回」
  assert.ok(!/NewBadRequestError\("请先录入该月星达分表读数"\)/.test(whole), '不得因缺分时读数阻断')
  assert.ok(!/NewBadRequestError\("星达分表电量小于工业分表合计/.test(whole), '负差额不得阻断')
  assert.ok(!/NewBadRequestError\("市电账单本期电量为空/.test(whole), '不得因账单电量为空阻断')
  assert.match(gen, /if in\.LineLoss < 0 \{/, '负差额必须收敛为 0 而不是报错')
  assert.match(gen, /billItem = &types\.UtilityBillExtractionItem\{\}/, '缺账单必须退化为零值账单')
  // ratio 未赋值时保持结构体零值 0：只在两端电量都 > 0 时才计算
  assert.match(
    gen,
    /if billItem\.TotalKwh > 0 && in\.StarTotal > 0 \{[\s\S]{0,160}?in\.Ratio = round8/,
    'ratio 缺数据时为 0',
  )
})

// ---- ④ 打印目录 ----

test('目录 PDF 接入 catalogLayout：总宽锁死内容区、数字不拆行', () => {
  const src = read('useCatalogPdf.ts')
  assert.match(src, /import \{ fitColumnWidths, wrapNoBreakNumbers/, '必须复用 catalogLayout')
  assert.match(src, /fitColumnWidths\(\{/, '列宽必须由 fitColumnWidths 统一计算')
  assert.match(src, /const widths = fit\.widths/, '渲染必须使用 fit 结果')
  assert.match(src, /wrapNoBreakNumbers\(\(s\) => font\.widthOfTextAtSize\(s, cellSize\)/, '单元格折行必须禁拆数字')
  // 旧实现的固定字号/保底列宽必须消失，否则会再次越界
  assert.ok(!/MIN_COL_W/.test(src), '不得再有 40pt 保底列宽（20 列必然越界）')
  assert.ok(!/MAX_COL_W/.test(src), '不得再有 30% 单列上限（会挤压数字列）')
})
