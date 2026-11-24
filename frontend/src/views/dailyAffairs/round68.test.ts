/**
 * 第 68 轮单测：
 *   ① 打印方向智能判定（纵向优先）+ 窄列比例上限
 *   ② 抽屉紧凑排版（label 置顶、全行白名单收紧、双列 dense）
 *   ③ 设置抽屉最窄态列宽（数据类型 108px）
 *   ④ 离职人员字段全集（前后端契约）
 *   ⑤ 社保账单快照 / 变更提醒（前端接线）
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import {
  resolvePrintOrientation,
  estimateTotalMinWidthMm,
  PORTRAIT_CONTENT_MM,
} from './useCatalogPdf'
import { fitColumnWidths } from './catalogLayout'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (f: string) => fs.readFileSync(path.join(DIR, f), 'utf8')

const col = (label: string, sample: string) => ({ label, value: () => sample })
const rows = (n: number) => Array.from({ length: n }, () => ({}))

test('第68轮：6/9 列判纵向，15 列判横向', () => {
  const six = [
    col('序号', '12'), col('岗位', '管退操作员'), col('人数', '12'),
    col('个人金额', '3,485.00'), col('单位金额', '3,485.00'), col('合计', '6,970.00'),
  ]
  assert.equal(resolvePrintOrientation(six, rows(28)), 'portrait')
  assert.ok(estimateTotalMinWidthMm(six, rows(1)) <= PORTRAIT_CONTENT_MM)

  const nine = [
    col('序号', '75'), col('姓名', '安洪琦'), col('部门', '技术质量部'), col('基数', '5,000.00'),
    col('养老个人', '400.00'), col('医疗个人', '100.00'), col('大额医疗', '0.00'),
    col('失业个人', '20.00'), col('个人小计', '520.00'),
  ]
  assert.equal(resolvePrintOrientation(nine, rows(75)), 'portrait')

  const fifteen = Array.from({ length: 15 }, (_, i) => col(`列${i + 1}`, '1,234.56'))
  assert.equal(resolvePrintOrientation(fifteen, rows(20)), 'landscape')
})

test('第68轮：列数上限 10 是硬边界（11 列必横向）', () => {
  const ten = Array.from({ length: 10 }, (_, i) => col(`序${i}`, '1'))
  assert.equal(resolvePrintOrientation(ten, rows(1)), 'portrait')
  const eleven = Array.from({ length: 11 }, (_, i) => col(`序${i}`, '1'))
  assert.equal(resolvePrintOrientation(eleven, rows(1)), 'landscape')
})

test('第68轮：窄列上限生效 —— 序号占比 < 12%', () => {
  const headers = ['序号', '岗位', '人数', '个人金额', '单位金额', '合计']
  const cells = [
    ['1', '管退操作员', '12', '3,485.00', '3,485.00', '6,970.00'],
    ['2', '束绞操作员', '11', '4,085.00', '4,085.00', '8,170.00'],
  ]
  const measure = (t: string) => Array.from(t).length * 5
  const avail = 531
  const fit = fitColumnWidths({
    avail, headers, cells, measure,
    maxDesired: headers.map((h) => (/^序号$/.test(h) ? avail * 0.07 : /^人数$/.test(h) ? avail * 0.09 : 0)),
  })
  assert.equal(Math.round(fit.widths.reduce((a, b) => a + b, 0) * 100) / 100, avail)
  assert.ok(fit.widths[0] / avail < 0.12, `序号占比 ${(fit.widths[0] / avail * 100).toFixed(1)}%`)
  assert.ok(fit.widths[1] > fit.widths[0], '弹性列必须比序号宽')
})

test('第68轮：目录引擎不再写死 A4 横向', () => {
  const src = read('useCatalogPdf.ts')
  assert.match(src, /export function resolvePrintOrientation/)
  assert.match(src, /PORTRAIT_CONTENT_MM = 190/)
  assert.match(src, /const pageSize = orientation === 'portrait' \? A4_PORTRAIT : A4_LANDSCAPE/)
  assert.ok(!/const PAGE_W = 841\.89/.test(src), '不得再固定 841.89 横向页宽')
  // 能耗清单专属模板（StatementDoc）保持原样
  assert.match(src, /const PORTRAIT_W = 595\.28/)
})

test('第68轮（第 69 轮已升级为同行优先）：不再按左栏宽度排版', () => {
  const src = read('BusinessSplitDetailDrawer.vue')
  assert.match(src, /label-align="top"/)
  assert.ok(!/label-width="`\$\{labelWidth\}/.test(src), '不得再用左栏 label 宽度')
  assert.match(src, /grid-auto-flow: dense/)
  assert.match(src, /column-gap: 20px/)
  // 第 69 轮把行距从 14 收到 10、并去掉 label 的下外边距（同行布局）
  assert.match(src, /row-gap: 10px/)
  assert.match(src, /font-size: 13px/)
  assert.match(src, /margin: 0 !important;/)
  // 全行白名单收紧：普通单行字段不得再跨整行
  const body = src.slice(src.indexOf('const FULL_WIDTH_FIELDS'), src.indexOf('const fieldClassOf'))
  for (const f of ['工作邮箱', '个人邮箱', '服装尺寸', '户口性质', '手机号码']) {
    assert.ok(!body.includes(`'${f}'`), `${f} 不应整行`)
  }
  for (const f of ['户籍地址', '现居住地', '紧急联系人地址', '配套设施', '备注', '离职原因']) {
    assert.ok(body.includes(`'${f}'`), `${f} 必须整行`)
  }
})

test('第68轮：设置抽屉数据类型列 108px，抽屉最小宽 680', () => {
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /grid-template-columns: 24px minmax\(120px, 1fr\) 108px 58px 60px 68px/)
  assert.match(panel, /&\.has-width \{\s*\n\s*grid-template-columns: 24px minmax\(120px, 1fr\) 108px 58px 56px 60px 68px;/)
  assert.match(panel, /gap: 8px/)
  const drawer = fs.readFileSync(
    path.join(DIR, '..', '..', 'components', 'business', 'StandardSettingDrawer.vue'), 'utf8')
  assert.match(drawer, /min-width: min\(680px, 95vw\)/)
  assert.match(drawer, /width="min\(760px, 95vw\)"/)
})

test('第68轮：离职抽屉由「离职人员」胶囊驱动（全字段配置）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const subs = subsOfTab\('hr-roster'\)\.filter/)
  assert.ok(!/function rosterAllSubs\(\)/.test(src), '旧的全量合并入口必须删除')
  // 静态契约仍是 15 个离职专有字段（新并入的在职字段由 subs 提供，默认关闭）
  const schema = read('hrFieldSchema.ts')
  assert.match(schema, /export const HR_RESIGN_FIELDS/)
})

test('第68轮：社保账单变更提醒接线（导入/保存/删除 + 列表橙标 + 弹窗顶部条）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /function warnSiBillDirty\(/)
  assert.match(src, /bill_dirty_warning/)
  assert.match(src, /warnSiBillDirty\(d\)/)
  assert.match(src, /warnSiBillDirty\(saved\?\.data \|\| saved \|\| \{\}\)/)
  assert.match(src, /data-hr-bill-dirty-flag/)
  assert.match(src, /data-hr-bill-dirty/)
  assert.match(src, /siBillDirty\.value = !!\(d\.bill\?\.source_dirty/)
  // 后端：删除不再重算账单
  const del = fs.readFileSync(
    path.join(DIR, '..', '..', '..', '..', 'internal', 'handler', 'hr_social_delete.go'), 'utf8')
  assert.ok(!/h\.hrReconcileSIBillQuiet\(/.test(del), '删除记录不得再重建账单')
  assert.match(del, /hrSIMarkBillDirty68/)
})

test('第68轮：账单明细弹窗定宽定高（切换标签零跳动）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /width="min\(94vw, 1360px\)"/)
  assert.match(src, /height: min\(82vh, 780px\)/)
  assert.match(src, /class="hr-bill-dialog"/)
  // 表格吃满剩余高度：tfoot 恒贴底
  assert.ok((src.match(/:max-body-height="4000"/g) || []).length >= 5)
})
