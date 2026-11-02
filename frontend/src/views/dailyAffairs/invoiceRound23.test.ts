import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

/**
 * 第 23 轮：发票分类与「未完成记录不进主列表」的契约锁定。
 *
 * 这四项都不是「算法对不对」，而是「同一条数据在几个地方被解释成不同结果」：
 *   ① 解析中的文件以占位行进入主列表（用户看到「发票号码: 提取中」的假记录）；
 *   ② 分类有两个键（invoice_category / fleet_cert_type），写入只更新一个 → 漂移；
 *   ③ 抽屉表单被详情异步重填覆盖 → 手动输入「输不进去」；
 *   ④ 多页汇总单缺 HTML 表格规则 → 全落到 LLM，金额/税率/税额/开票人全空。
 *
 * 这里只断言**结构性契约**（哪个函数必须存在、必须被谁调用），
 * 数值断言在 Go 侧（TestExtractInvoicesByHTMLTableRules*）。
 */

const read = (f: string) => readFileSync(new URL(`./${f}`, import.meta.url), 'utf8')
const readGo = (p: string) => readFileSync(new URL(`../../../../${p}`, import.meta.url), 'utf8')

// ---- 契约 1：未完成提取的记录不进主列表 / 不进 KPI ----

test('后端：列表与概览共用同一道「提取未完成」守卫', () => {
  const gate = readGo('internal/application/service/extraction_gate.go')
  // 白名单而不是黑名单：新增中间态时黑名单会漏（真机 pending 被当完成态）
  assert.match(gate, /extractionCompletedStatuses = map\[string\]struct\{\}/, '必须有完成态白名单')
  for (const s of ['"success"', '"manual"', '"failed"']) {
    assert.ok(gate.includes(s), `完成态白名单必须含 ${s}`)
  }
  assert.match(gate, /func extractionNotStarted\(/, '必须有判定函数')
  // 只有「还没跑完 + 一条子记录都没有」才拦：存量数据不能因为状态字段缺失整体隐身
  assert.match(gate, /func hasNoExtractedItems\(/, '必须有子记录数判定')

  // 第 24 轮升级：判据从「未完成 + 无子记录」升级为 extractionShouldHide，
  // 增加两条**不看子记录**的拦截（解析未完成 / 提取明确在途）。
  // 原因：图 1 里状态栏还显示「正在解析 2026.9-10油费发票.pdf」，
  // 列表第 17~20 行已经出现该文件的 P1~P4（第 20 行「待提取 + 0.00」），
  // 旧判据一旦有半成品子记录就不再成立，直接放行。
  assert.match(gate, /func extractionShouldHide\(/, '必须有统一准入判据')
  assert.match(gate, /func extractionParseIncomplete\(/, '必须判解析是否完成')
  assert.match(gate, /func extractionInFlight\(/, '必须判提取是否明确在途')

  for (const f of [
    'knowledge_invoice_list.go',
    'knowledge_contract_list.go',
    'knowledge_regulation_list.go',
    'knowledge_award_punish_list.go',
    'knowledge_energy_list.go',
    'knowledge_utility_bill_list.go',
  ]) {
    const src = readGo(`internal/application/service/${f}`)
    // 必须用统一判据（传 parse_status + extract_status + 子记录数）
    assert.match(src, /extractionShouldHide\(/, `${f} 必须用统一准入判据`)
    // 反向断言：不得再退回旧的「只看未完成 + 子记录数」判据
    // （那会在有半成品子记录时放行，正是图 1 的事故形态）
    assert.doesNotMatch(
      src,
      /extractionNotStarted\(meta\.ExtractStatus\) && hasNoExtractedItems\(/,
      `${f} 不得退回旧判据（有半成品子记录时会放行）`,
    )
  }
  // 发票概览（KPI 卡）必须与列表同口径，否则出现「卡片 3 张、点进去只有 1 行」
  const inv = readGo('internal/application/service/knowledge_invoice_list.go')
  const overview = inv.slice(inv.indexOf('func (s *BusinessExtractService) InvoiceOverviewStats'))
  assert.match(overview, /extractionShouldHide\(/, '发票概览统计必须与列表同口径')
})

// ---- 契约 2/3：分类必须是「上传选择」，且读写同键 ----

test('前端：保存时两个分类键一起写（避免只更新一个导致漂移）', () => {
  const src = read('InvoiceManagement.vue')
  assert.match(src, /invoice_category: nextType/, '必须写 invoice_category（读取侧的优先键）')
  assert.match(src, /fleet_cert_type: nextType/, '必须同时写 fleet_cert_type（历史兜底键）')
})

test('后端：UpdateInvoiceMetadata 写库前把两个分类键归一', () => {
  const src = readGo('internal/handler/knowledge_business.go')
  assert.match(src, /func normalizeInvoiceCategoryKeys\(/, '必须有分类双键归一函数')
  assert.match(src, /normalizeInvoiceCategoryKeys\(meta\)/, 'UpdateInvoiceMetadata 必须调用它')
  // 归一函数的语义：权威值取 invoice_category，两键同步
  const fn = src.slice(src.indexOf('func normalizeInvoiceCategoryKeys'))
  assert.match(fn, /meta\["invoice_category"\] = authoritative/, '归一后必须回写 invoice_category')
  assert.match(fn, /meta\["fleet_cert_type"\] = authoritative/, '归一后必须回写 fleet_cert_type')
  // 两个键都没出现时不得落库（用户只是在改金额，不能顺手改分类）
  assert.match(fn, /if !newOK && !oldOK \{\s*\n\s*return ""/, '未提及分类时不得落库')
})

test('后端：不存在按票面关键词自动改写分类的路径（分类只能来自上传选择）', () => {
  const lock = readGo('internal/application/service/knowledge_invoice_category_lock.go')
  assert.match(lock, /func LockInvoiceCategory\(/, '分类锁定函数必须存在')
  assert.match(lock, /res\.Invoices\[i\]\.InvoiceType = certType/, '锁定必须覆盖票面推断值')
  assert.match(lock, /res\.Invoices\[i\]\.Category = certType/, '锁定必须覆盖 category')
  // 分类唯一权威链：请求 cert > invoice_category > fleet_cert_type（历史）
  assert.match(lock, /func resolveInvoiceCertType\(/, '必须有分类来源优先级链')
})

// ---- 契约 4：抽屉手动录入不得被重填覆盖 ----

test('前端：详情异步返回的重填不得覆盖用户已输入的值', () => {
  const src = read('InvoiceManagement.vue')
  assert.match(src, /const fillEditForm = \(force = true\)/, 'fillEditForm 必须支持 force 开关')
  assert.match(src, /fillEditForm\(true\)/, '打开抽屉必须强制重填（否则显示上一分类字段）')
  assert.match(src, /fillEditForm\(false\)/, '详情返回后的补填必须非强制')
  // 非强制分支必须只补空字段
  const fn = src.slice(src.indexOf('const fillEditForm = (force = true)'))
  assert.match(fn, /if \(!force && detailDirty\.value\)/, '脏状态下必须走「只补空」分支')
  assert.match(fn, /cv === '' \|\| cv === null \|\| cv === undefined/, '只补空值字段')
})

test('后端：多页 HTML 表格汇总单必须有确定性规则（不再全落 LLM）', () => {
  const rules = readGo('internal/application/service/knowledge_invoice_rules.go')
  assert.match(rules, /ExtractInvoicesByHTMLTableRulesFromChunks/, '规则链必须包含 HTML 表格规则')
  assert.match(rules, /InvoiceRuleKindHTMLTable/, '必须登记规则类型（供日志诊断）')

  const html = readGo('internal/application/service/knowledge_invoice_html_rules.go')
  // 图 3 的四个空白字段必须有确定性来源
  assert.match(html, /func ExtractInvoicesByHTMLTableRules\(/, '必须有 HTML 表格规则入口')
  assert.match(html, /inv\.Amount = head\.Amount/, '金额必须来自数据行')
  assert.match(html, /inv\.TaxRate = head\.TaxRate/, '税率必须来自数据行')
  assert.match(html, /inv\.Tax = head\.TaxAmount/, '税额必须来自数据行')
  assert.match(html, /inv\.TotalAmount = &total/, '价税合计必须由 金额+税额 反算兜底')
  assert.match(html, /htmlPartyNear\(seg, "购买方"\)/, '购销方必须按位置就近配对')
  // 绝不跨段配对：段落内无数据行时不产出该票
  assert.match(html, /if !ok \{\s*\n\s*continue/, '段落无数据行必须跳过（不拿邻居的金额）')
})

// ---- 契约 5：不改写发票类型枚举（票面类型 vs 归档分类的边界）----

test('后端：票面类型归一保持固定枚举，不被分类名污染', () => {
  const src = readGo('internal/application/service/knowledge_invoice_extract.go')
  const fn = src.slice(src.indexOf('func NormalizeInvoiceTypeFromTicket'),
    src.indexOf('func NormalizeInvoiceTypeFromTicket') + 900)
  assert.match(fn, /return "普通发票"/, '含通行费 → 普通发票（票面类型）')
  assert.match(fn, /return "专用发票"/, '含专用 → 专用发票')
  // 归档分类（通行费发票/能耗发票）不属于票面类型枚举，不得在此返回
  assert.ok(!fn.includes('"通行费发票"'), '票面类型枚举不得混入归档分类名')
  assert.ok(!fn.includes('"能耗发票"'), '票面类型枚举不得混入归档分类名')
})
