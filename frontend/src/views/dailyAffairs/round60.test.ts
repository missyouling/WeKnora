/**
 * 第 60 轮源码级回归：
 *   · 离职证明覆盖确认改为 t-popconfirm（同步唤起文件选择框，不再走模态框）；
 *   · 字段设置胶囊隔离（人事按白名单严格过滤，拖拽按 group+name 身份）；
 *   · 社保月度账单只手动生成，且生成前先检查正常记录。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const strip = (t: string) => t.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')

test('离职证明：已上传时用 t-popconfirm 气泡确认，模态框已删除', () => {
  const src = read('HrManagement.vue')
  assert.ok(/<t-popconfirm[\s\S]{0,400}原来的图片将会被覆盖，是否继续？[\s\S]{0,400}@confirm="pickProof"/.test(src),
    '覆盖确认必须是 t-popconfirm 且 @confirm 直接调 pickProof')
  assert.ok(!/data-hr-proof-confirm/.test(src), '第 58 轮的 alert 模态框必须彻底删除')
  assert.ok(!/proofOverwriteVisible/.test(src), '模态框开关状态必须一并删除')
})

test('离职证明：pickProof 只同步 click，不包 setTimeout / await', () => {
  const src = read('HrManagement.vue')
  const fn = strip(src.slice(src.indexOf('function pickProof'), src.indexOf('async function onProofFileChange')))
  assert.match(fn, /proofInputRef\.value\?\.click\(\)/, '必须直接 click 隐藏 input')
  assert.ok(!/setTimeout|await /.test(fn), '文件选择框必须同步唤起，否则 user activation 丢失')
})

test('离职证明：上传成功后回填草稿 + 列表行并提示「离职证明已更新」', () => {
  const src = read('HrManagement.vue')
  const fn = src.slice(src.indexOf('async function onProofFileChange'), src.indexOf('function applyProofPathToRow'))
  assert.match(fn, /editForm\.value\.data\['离职证明'\] = path/)
  assert.match(fn, /applyProofPathToRow\(detailRow\.value, path\)/)
  assert.match(fn, /MessagePlugin\.success\('离职证明已更新'\)/)
})

test('字段设置：人事胶囊严格按白名单过滤，不再让加锁字段穿透', () => {
  const src = read('FleetCertPanel.vue')
  const fn = strip(src.slice(src.indexOf('const fieldsVisible = computed'), src.indexOf('const isTenantGroupActive')))
  assert.match(fn, /visibleGroupFields\(/, '必须走 fieldScope 的胶囊过滤')
  assert.ok(!/tabGroupKey\(f\.group\) === want \|\| isLockedStatus\(f\)/.test(fn),
    '旧写法让所有加锁字段跨胶囊穿透，必须删除')
})

test('字段设置：拖拽按 group+name 身份重排（同名跨胶囊不串组）', () => {
  const src = read('FleetCertPanel.vue')
  const fn = src.slice(src.indexOf('function onFieldDragStart'), src.indexOf('async function load()'))
  assert.match(fn, /fieldIdentityOf\(fd\)/)
  assert.match(fn, /moveFieldWithinGroup\(/)
  assert.ok(!/findIndex\(\(f: any\) => f\.name === fromName\)/.test(fn), '按下标 / 裸 name 定位的旧实现必须移除')
})

test('生成账单：先查正常记录，0 条时 Toast 拦截且不发写请求', () => {
  const src = read('HrManagement.vue')
  const fn = strip(src.slice(src.indexOf('async function confirmGenerateBill'), src.indexOf('async function doGenerateBill')))
  assert.match(fn, /precheckSocialBill\(month\)/)
  assert.match(fn, /normal_record_count/)
  assert.match(fn, /该月份暂无正常社保记录，请先上传社保系统导出的记录数据/)
  assert.match(fn, /月度账单已存在，重新生成将覆盖原有账单数据，是否继续？/)
  const guard = fn.indexOf('normal_record_count')
  const create = fn.indexOf('doGenerateBill(false)')
  assert.ok(guard > 0 && guard < create, '前置校验必须在生成之前')
})

test('生成账单：账单标签页只有手动入口，导入链路不得自动生成', () => {
  const src = read('HrManagement.vue')
  const importFn = strip(src.slice(src.indexOf('async function runSocialImport'), src.indexOf('async function onSocialImported')))
  assert.ok(!/createSocialBill|doGenerateBill/.test(importFn), '导入后不得自动生成账单')
  assert.match(src, /data-hr-generate-bill/, '必须保留手动「生成账单」按钮')
})
