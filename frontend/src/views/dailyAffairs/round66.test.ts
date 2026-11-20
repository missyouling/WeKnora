/**
 * 第 66 轮单测：
 *   ① 费用清单拆成「电费（2 页）+ 水费（1 页）」两份文档，电水数据互不串场
 *   ② 公积金弹窗 4 标签（部门汇总 / 按岗位汇总），打印导出走同一取数口
 *   ③ 合同「无固定期限」字段选项与抽屉禁用联动
 *   ④ 社保导入切片覆盖（前端不再整月覆盖）
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'

const DIR = path.dirname(new URL(import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'))
const read = (f: string) => fs.readFileSync(path.join(DIR, f), 'utf8')

test('第66轮：费用清单拆成电费/水费两份文档', () => {
  const src = read('energyBillStatements.ts')
  // 两个独立构造器
  assert.match(src, /export function buildElecBillingStatementDoc/)
  assert.match(src, /export function buildWaterBillingStatementDoc/)
  // 标题必须是「电费费用清单」「水费费用清单」
  assert.match(src, /`\$\{unit\} 电费费用清单`/)
  assert.match(src, /`\$\{unit\} 水费费用清单`/)
  // 概览标题分离
  assert.match(src, /overviewTitle: '一、电费账单概览'/)
  assert.match(src, /overviewTitle: '一、水费账单概览'/)
})

test('第66轮：电费 KPI 不含水费，水费 KPI 不含电费', () => {
  const src = read('energyBillStatements.ts')
  const elecStart = src.indexOf('function elecKpisOf')
  const waterStart = src.indexOf('function waterKpisOf')
  // 去掉 waterKpisOf 上方的文档注释（注释里会提到「水费专属」）
  const elecBody = src.slice(elecStart, src.lastIndexOf('/**', waterStart))
  // 去掉电费 meta 上方的注释块（它会提到电费）
  const waterEnd = src.lastIndexOf('/**', src.indexOf('function elecMetaOf'))
  const waterBody = src.slice(waterStart, waterEnd)

  assert.ok(!/water_/.test(elecBody), '电费 KPI 不得引用 water_* 字段')
  assert.ok(!/水费|用水量|吨/.test(elecBody), '电费 KPI 不得出现水费文案')
  // 注意 water_ind_fee 里也含 ind_fee 子串，必须按「电费字段名」精确判
  assert.ok(!/sum\('ind_fee'\)|sum\('dorm_fee'\)|total_kwh|度/.test(waterBody), '水费 KPI 不得出现电费字段')
  assert.match(waterBody, /water_ind_usage/)
  assert.match(waterBody, /water_dorm_usage/)
})

test('第66轮：电费/水费概览元信息各自独立', () => {
  const src = read('energyBillStatements.ts')
  assert.match(src, /label: '电费账单户号'/)
  assert.match(src, /label: '本期电费应收合计'/)
  assert.match(src, /label: '本期水费应收合计'/)
  // 新的电费概览不得再出现「电水合计应收」（遗留的旧构造器不在调用链上）
  const elecMeta = src.slice(src.indexOf('function elecMetaOf'), src.indexOf('function waterMetaOf'))
  assert.ok(!/电水合计应收/.test(elecMeta), '电费概览不得再有「电水合计应收」')
})

test('第66轮：TenantBilling 用 pdf-lib 合并两份 PDF 成 3 页', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /buildElecBillingStatementDoc/)
  assert.match(src, /buildWaterBillingStatementDoc/)
  assert.match(src, /mergeStatementPdfs66/)
  assert.match(src, /await import\('pdf-lib'\)/)
  // 先电后水（页序 1-2 → 3）
  assert.match(src, /mergeStatementPdfs66\(\[elec, water\]\)/)
})

test('第66轮：公积金弹窗 4 标签（部门汇总 + 按岗位汇总）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /value="summary">部门汇总</)
  assert.match(src, /value="post">按岗位汇总</)
  assert.ok(!/汇总数据/.test(src), '旧文案「汇总数据」必须消失')
  // 打印/导出共用一处取数（含 post 分支）
  assert.match(src, /function billTabSource/)
  assert.match(src, /tab === 'post'/)
  assert.match(src, /label: '按岗位汇总'/)
  // 岗位汇总的金额口径与部门汇总同源
  const postIdx = src.indexOf('const billPostRows')
  const body = src.slice(postIdx, postIdx + 1400)
  assert.match(body, /siCents\(r\.个人缴存\)/)
  assert.match(body, /siCents\(r\.单位缴存\)/)
  assert.match(body, /isAdjustmentRow\(r\)/)
})

test('第66轮：岗位下拉/搜索与「共 X 个岗位」', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /data-hr-bill-post-search/)
  assert.match(src, /data-hr-bill-post-select/)
  assert.match(src, /共 \{\{ billPostRows\.length \}\} 个岗位/)
  // 未匹配到岗位的人归入「未设置岗位」
  assert.match(src, /未设置岗位/)
})

test('第66轮：合同期限含「无固定期限」且抽屉禁用结束日期', () => {
  const fields = read('hrRosterFields.ts')
  assert.match(fields, /"现合同期限": \[\s*[\s\S]{0,120}"无固定期限"/)

  const src = read('HrManagement.vue')
  assert.match(src, /contractOpenEnded/)
  // 两个字段名都要禁用（合同模块 / 花名册模块）
  // 两个字段名各有一个插槽，禁用条件都绑 contractOpenEnded
  assert.match(src, /#field-合同结束日期[\s\S]{0,400}disabled="contractOpenEnded"/)
  assert.match(src, /#field-现合同结束日期[\s\S]{0,400}disabled="contractOpenEnded"/)
  // 变为无固定期限时清空
  // watch 内部会清空两个可能字段名（合同结束日期 / 现合同结束日期）
  assert.match(src, /watch\(contractOpenEnded[\s\S]{0,600}合同结束日期'\] = ''/)
})

test('第66轮：社保导入带切片覆盖参数与冲突确认', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /checkSocialImport/)
  assert.match(src, /overwrite_scope', 'slice'/)
  assert.match(src, /__cancel__/)
  assert.match(src, /siConflictVisible/)
  // 确认弹窗必须点明「仅覆盖当前文件对应的险种-缴纳方」
  assert.match(src, /仅覆盖当前文件对应的/)
  const api = read('hrApi.ts')
  assert.match(api, /social\/records\/import-check/)
})
