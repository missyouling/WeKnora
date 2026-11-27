/**
 * 第 78 轮单测：
 *  ① 维修结算清单三子表金额钩稽（90 / 189 / 297）与差额告警
 *  ② 维修项目摘要自动生成（N 项 / M 件）
 *  ③ 行驶证「年检日期」→ 有效 / 即将到期 / 已过期 / 空 四态
 *  ④ `YYYY年MM月` → 该月最后一天归一
 *  ⑤ 维保分类收敛 / 附件袋 / 横向抽屉的结构断言
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(HERE, f), 'utf8')
const readGo = (rel: string) => readFileSync(join(HERE, '../../../../', rel), 'utf8')

import {
  EMPTY_LINE_ITEMS,
  lineItemsTotals,
  repairAmountWarnings,
  repairSummaryOf,
  recalcSummaryFromItems,
  type RepairLineItems,
} from './lineItems'
import { certStatusOf, normalizeInspectionMonth } from './fleetColumns'

// ---------- ① 三子表金额钩稽 ----------
// 样单：工时 50/20/20 = 90；零件 160/20/5/4 = 189；其它 18；实际应收 90+189+18 = 297
const SAMPLE: RepairLineItems = {
  labor_items: [
    { item_name: '拔左前轮更换轮胎螺丝', hours: 1, labor_fee: 50, technician: '张', remark: '' },
    { item_name: '更换刹车管', hours: 1, labor_fee: 20, technician: '张', remark: '' },
    { item_name: '更换喷水接头', hours: 1, labor_fee: 20, technician: '李', remark: '' },
  ],
  part_items: [
    { part_name: '轮胎螺丝', unit: '个', qty: 4, unit_price: 40, amount: 160, remark: '' },
    { part_name: '刹车管', unit: '根', qty: 1, unit_price: 20, amount: 20, remark: '' },
    { part_name: '喷水接头', unit: '个', qty: 1, unit_price: 5, amount: 5, remark: '' },
    { part_name: '卡箍', unit: '个', qty: 2, unit_price: 2, amount: 4, remark: '' },
  ],
  extra_items: [{ item_name: '税费', amount: 18 }],
}

test('lineItemsTotals 汇总三子表：工时 90 / 材料 189 / 附加 18', () => {
  const t = lineItemsTotals(SAMPLE)
  assert.equal(t.labor, 90)
  assert.equal(t.part, 189)
  assert.equal(t.extra, 18)
  assert.equal(t.laborCount, 3)
  assert.equal(t.partCount, 4)
})

test('空子表汇总全为 0（不产生 NaN）', () => {
  const t = lineItemsTotals(EMPTY_LINE_ITEMS)
  assert.equal(t.labor, 0)
  assert.equal(t.part, 0)
  assert.equal(t.extra, 0)
  assert.equal(t.laborCount, 0)
  assert.equal(t.partCount, 0)
})

test('金额钩稽：主表 90/189/18/297 与明细一致 → 0 告警', () => {
  const w = repairAmountWarnings(
    { 工时费: 90, 材料费: 189, 管理费: 0, 其它费: 18, 实际应收: 297 },
    SAMPLE,
  )
  assert.deepEqual(w, [])
})

test('金额钩稽：实际应收改成 300 → 报 1 条差额 3.00', () => {
  const w = repairAmountWarnings(
    { 工时费: 90, 材料费: 189, 管理费: 0, 其它费: 18, 实际应收: 300 },
    SAMPLE,
  )
  assert.equal(w.length, 1)
  assert.equal(w[0].field, '实际应收')
  assert.equal(w[0].expected, 297)
  assert.equal(w[0].actual, 300)
  assert.equal(w[0].diff, 3)
})

test('金额钩稽：误差 ≤0.01 视为一致（浮点容差）', () => {
  const w = repairAmountWarnings(
    { 工时费: 90, 材料费: 189, 管理费: 0, 其它费: 18, 实际应收: 297.01 },
    SAMPLE,
  )
  assert.deepEqual(w, [])
})

test('金额钩稽：工时费与明细不符 + 实际应收不符 → 2 条告警', () => {
  // 工时费填 80（明细 90）+ 实际应收填 280（按主表算应为 80+189+0+18=287）
  const w = repairAmountWarnings(
    { 工时费: 80, 材料费: 189, 管理费: 0, 其它费: 18, 实际应收: 280 },
    SAMPLE,
  )
  assert.equal(w.length, 2)
  assert.equal(w[0].field, '工时费')
  assert.equal(w[0].expected, 90)
  assert.equal(w[0].actual, 80)
  assert.equal(w[1].field, '实际应收')
})

test('金额钩稽：零件单行 qty×unit_price ≠ amount 也报出来', () => {
  const bad: RepairLineItems = {
    labor_items: [],
    part_items: [{ part_name: '螺丝', unit: '个', qty: 4, unit_price: 40, amount: 150, remark: '' }],
    extra_items: [],
  }
  const w = repairAmountWarnings({ 工时费: 0, 材料费: 150, 管理费: 0, 其它费: 0, 实际应收: 150 }, bad)
  assert.equal(w.length, 1)
  assert.match(w[0].field, /螺丝/)
  assert.equal(w[0].expected, 160)
  assert.equal(w[0].actual, 150)
})

test('金额钩稽：管理费参与合计（90+189+18+18=315）', () => {
  const w = repairAmountWarnings(
    { 工时费: 90, 材料费: 189, 管理费: 18, 其它费: 18, 实际应收: 315 },
    SAMPLE,
  )
  assert.deepEqual(w, [])
})

// ---------- ② 维修项目摘要 ----------
test('摘要：3 项工时 + 4 件零件 → 「…（3项/4件）」', () => {
  const s = repairSummaryOf(SAMPLE)
  assert.match(s, /拔左前轮更换轮胎螺丝/)
  assert.match(s, /更换刹车管/)
  assert.match(s, /更换喷水接头/)
  assert.match(s, /（3项\/4件）$/)
})

test('摘要：只有零件时也标注件数', () => {
  const s = repairSummaryOf({
    labor_items: [],
    part_items: SAMPLE.part_items,
    extra_items: [],
  })
  assert.match(s, /（0项\/4件）$/)
})

test('摘要：全空返回空串（不产生「（0项/0件）」噪音）', () => {
  assert.equal(repairSummaryOf(EMPTY_LINE_ITEMS), '')
})

test('recalcSummaryFromItems 同时回填工时费/材料费/其它费/实际应收', () => {
  const out = recalcSummaryFromItems(
    { 工时费: 0, 材料费: 0, 管理费: 5, 其它费: 0, 实际应收: 0 },
    SAMPLE,
  )
  assert.equal(out['工时费'], 90)
  assert.equal(out['材料费'], 189)
  assert.equal(out['其它费'], 18)
  assert.equal(out['实际应收'], 90 + 189 + 5 + 18)
  assert.match(String(out['维修项目摘要']), /（3项\/4件）$/)
})

test('recalcSummaryFromItems 不覆盖管理费（用户手填项）', () => {
  const out = recalcSummaryFromItems({ 管理费: 20 }, SAMPLE)
  assert.equal(out['管理费'], 20)
  assert.equal(out['实际应收'], 90 + 189 + 20 + 18)
})

// ---------- ③ 行驶证年检日期 → 证照状态 ----------
test('年检日期为空 → 证照状态空串（严禁渲染 —）', () => {
  const st = certStatusOf({ 年检日期: '' }, '2026-11-27')
  assert.equal(st.label, '')
  assert.equal(st.key, '')
})

test('年检日期已过期 → 已过期', () => {
  const st = certStatusOf({ 年检日期: '2025-01-01' }, '2026-11-27')
  assert.equal(st.label, '已过期')
  assert.equal(st.theme, 'danger')
})

test('年检日期 15 天后 → 即将到期', () => {
  const st = certStatusOf({ 年检日期: '2026-12-12' }, '2026-11-27')
  assert.equal(st.label, '即将到期')
  assert.equal(st.theme, 'warning')
})

test('年检日期 1 年后 → 有效', () => {
  const st = certStatusOf({ 年检日期: '2027-11-27' }, '2026-11-27')
  assert.equal(st.label, '有效')
  assert.equal(st.theme, 'success')
})

test('年检日期恰好今天 → 即将到期（0 天落 0..30 区间）', () => {
  const st = certStatusOf({ 年检日期: '2026-11-27' }, '2026-11-27')
  assert.equal(st.label, '即将到期')
})

test('年检日期第 31 天 → 有效（边界外）', () => {
  const st = certStatusOf({ 年检日期: '2026-12-28' }, '2026-11-27')
  assert.equal(st.label, '有效')
})

// ---------- ④ YYYY年MM月 月末归一 ----------
test('2027年12月 → 2027-12-31（月末最后一天）', () => {
  assert.equal(normalizeInspectionMonth('2027年12月'), '2027-12-31')
})

test('2026年2月 → 2026-02-28（非闰年）', () => {
  assert.equal(normalizeInspectionMonth('2026年2月'), '2026-02-28')
})

test('2028年2月 → 2028-02-29（闰年）', () => {
  assert.equal(normalizeInspectionMonth('2028年2月'), '2028-02-29')
})

test('标准 YYYY-MM-DD 原样返回', () => {
  assert.equal(normalizeInspectionMonth('2027-06-15'), '2027-06-15')
})

test('检验有效期至 2027年9月 → 2027-09-30', () => {
  assert.equal(normalizeInspectionMonth('检验有效期至 2027年9月'), '2027-09-30')
})

test('无法解析返回空串', () => {
  assert.equal(normalizeInspectionMonth('长期'), '')
  assert.equal(normalizeInspectionMonth(''), '')
})

// ---------- ⑤ 结构断言 ----------
test('维保分类收敛为 一般维修 / 二级维护（种子层）', () => {
  const seed = read('fleetCategorySeed.ts')
  const block = seed.slice(seed.indexOf('// ---- maintain ----'), seed.indexOf('// ---- tire ----'))
  assert.match(block, /name: "一般维修"/)
  assert.match(block, /name: "二级维护"/)
  assert.doesNotMatch(block, /name: "合格证"/, '合格证必须并入二级维护')
})

test('行驶证种子含年检日期（date 类型 + 默认表头）', () => {
  const seed = read('fleetCategorySeed.ts')
  assert.match(seed, /f\("年检日期", \{ dataType: "date", def: true \}\)/)
})

test('后端第 78 轮迁移入口与附件表存在', () => {
  const be = readGo('internal/handler/hr_round78.go')
  assert.match(be, /hrReconfigMaintenance78/)
  assert.match(be, /hrAddDrivingLicenseInspection78/)
  const att = readGo('internal/handler/fleet_attachments.go')
  assert.match(att, /fleet_record_attachments/)
  assert.match(att, /gorm:"type:bytea"/)
  // 附件袋铁律：只存不解，绝不触发解析/大模型。
  // 必须剥掉注释再扫 —— 文件头说明里就写着「严禁触发解析」，留着会误报。
  const attCode = att.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
  assert.doesNotMatch(attCode, /ExtractFleetDocument|RunExtractWithRetry|chatModel/, '附件上传严禁触发解析')
})

test('附件四条路由注册（含无后缀预览）', () => {
  const routes = readGo('internal/router/routes_knowledge.go')
  assert.match(routes, /records\/:id\/attachments/)
  assert.match(routes, /attachments\/view/)
  assert.match(routes, /attachments\/:attID/)
})
