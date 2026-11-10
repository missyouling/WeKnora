/**
 * 第 46 轮真机验收。
 *
 * 断言 1：花名册「导入花名册」弹出标准上传弹窗，含「下载导入模板」；
 *         下载 xlsx 并解析，断言 3 个 Sheet 且「导入数据」首行恰好 76 个标准表头。
 * 断言 2：搜索框后有「在职状态」下拉（在职/试用期/退休返聘/离职）；
 *         切「试用期」表头含 试用期/转正日期；切「离职」含 离职日期/离职原因/离职证明/离职备注。
 * 断言 3：三张 KPI 卡图标均非空且 name 有效（社保不再是无效的 shield）。
 * 断言 4：勾选员工 → 办理离职弹窗 4 个表单项齐全；提交后状态变「离职」并在离职视图出现。
 * 断言 5：HR / 车队 / 发票列表与详情不出现 ISO8601 原始串（T + 微秒 + 时区）。
 *
 * 运行（VM）：cd /opt/pw-run && node probe-round46.mjs
 */
import { chromium } from 'playwright'
import * as XLSX from 'xlsx'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`
const HR_URL = `${BASE}/hr`
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

let ok = 0
let total = 0
const failures = []
function check(name, pass, detail = '') {
  total++
  if (pass) ok++
  else failures.push(`${name}${detail ? ' :: ' + detail : ''}`)
  console.log(`${pass ? 'PASS' : 'FAIL'} | ${name}${detail ? ' | ' + detail : ''}`)
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 }, acceptDownloads: true })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
}, TOKEN)
const page = await ctx.newPage()
const consoleErrors = []
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR ' + String(e).slice(0, 200)))

async function stripGuide() {
  await page.evaluate(() => document.querySelectorAll('.guide').forEach((el) => el.remove()))
}
async function gotoHr() {
  await page.goto(HR_URL, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(14000)
  await stripGuide()
}

/** 走页面上下文发 API，自动带 token 与租户头。 */
async function api(method, path, body, asBase64 = false) {
  return page.evaluate(async ({ method, path, body, tk, asBase64 }) => {
    const headers = { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000', 'Accept-Language': 'zh-CN' }
    let payload
    if (body && body.__form) {
      const fd = new FormData()
      for (const [k, v] of Object.entries(body.fields || {})) fd.append(k, String(v))
      const bin = Uint8Array.from(atob(body.base64), (ch) => ch.charCodeAt(0))
      fd.append('file', new Blob([bin], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), body.fileName || 'data.xlsx')
      payload = fd
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
      payload = JSON.stringify(body)
    }
    const res = await fetch(path, { method, headers, body: payload })
    if (asBase64) {
      const buf = await res.arrayBuffer()
      let s = ''
      const u = new Uint8Array(buf)
      for (let i = 0; i < u.length; i += 0x8000) s += String.fromCharCode.apply(null, u.subarray(i, i + 0x8000))
      return { status: res.status, base64: btoa(s) }
    }
    const text = await res.text()
    let json
    try { json = JSON.parse(text) } catch { json = { raw: text.slice(0, 300) } }
    return { status: res.status, json }
  }, { method, path, body, tk: TOKEN, asBase64 })
}

function sheetRows(buf, name) {
  const wb = XLSX.read(buf, { type: 'buffer' })
  return { sheets: wb.SheetNames, rows: XLSX.utils.sheet_to_json(wb.Sheets[name], { header: 1, defval: '', raw: false }) }
}

async function clickKpi(label) {
  await page.locator('.dash-kpi-card', { hasText: label }).first().click()
  await page.waitForTimeout(3200)
}

async function headerTexts() {
  return page.locator('.t-table__header th, .t-table thead th').evaluateAll((els) => els.map((e) => e.innerText.trim()))
}

/** TDesign 下拉是 portal：必须真实 click 选项（DOM click 不触发委托）。 */
async function selectStatus(view) {
  const sel = page.locator('.hr-status-select').first()
  await sel.click()
  await page.waitForTimeout(700)
  const opt = page.locator('.t-select-option, .t-select-option__item', { hasText: new RegExp(`^\\s*${view}\\s*$`) }).first()
  if (await opt.count()) await opt.click()
  else await page.getByText(view, { exact: true }).last().click()
  await page.waitForTimeout(3200)
}

function uniqueTag() { return `R46-${Date.now().toString(36).toUpperCase()}` }
const seq2 = () => String(Math.floor(Math.random() * 90) + 10)
const checkDigit = () => String(Math.floor(Math.random() * 10))
const ID_A = `11010119900307${seq2()}1${checkDigit()}`
const ID_B = `11010119920815${seq2()}2${checkDigit()}`

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：导入弹窗与模板下载 =====')
await gotoHr()
await clickKpi('花名册')
const importBtn = page.getByRole('button', { name: /导入花名册/ }).first()
await importBtn.click()
const importDialog = page.locator('.t-dialog', { hasText: '导入花名册' }).last()
await importDialog.waitFor({ state: 'visible', timeout: 20000 })
const dialogText = await importDialog.innerText()
check('1.1 弹出「导入花名册」上传弹窗', /导入花名册/.test(dialogText))
check('1.2 弹窗含「下载导入模板」按钮', /下载导入模板/.test(dialogText))
check('1.3 弹窗含 3 种导入模式', /合并更新/.test(dialogText) && /仅新增/.test(dialogText) && /仅更新/.test(dialogText))
check('1.4 弹窗含拖拽上传区', (await importDialog.locator('.fu-dropzone, [class*="dropzone"]').count()) > 0)

// 直接打 API 拿模板字节（比浏览器 download 事件更稳），用 xlsx 解析
const tpl = await api('GET', '/api/v1/fleet/hr/roster/template', undefined, true)
check('1.5 模板接口返回 200', tpl.status === 200, `status=${tpl.status}`)
if (tpl.status === 200 && tpl.base64) {
  const buf = Buffer.from(tpl.base64, 'base64')
  let parsed = { sheets: [], rows: [] }
  try { parsed = sheetRows(buf, '导入数据') } catch (e) { console.log('  xlsx parse error', String(e).slice(0, 120)) }
  check('1.6 模板含 导入数据/说明/枚举字段 三个 Sheet',
    ['导入数据', '说明', '枚举字段'].every((s) => parsed.sheets.includes(s)), JSON.stringify(parsed.sheets))
  const headers = (parsed.rows[0] || []).map((x) => String(x || '').trim()).filter(Boolean)
  check('1.7 「导入数据」首行恰好 76 个标准表头', headers.length === 76, `count=${headers.length}`)
  check('1.8 首末表头正确（工号 … 现合同签订日期）', headers[0] === '工号' && headers[75] === '现合同签订日期', `${headers[0]} / ${headers[75]}`)
  check('1.9 含示例行 000001 / 示例员工', JSON.stringify(parsed.rows[1] || []).includes('示例员工'))
  const enums = (() => { try { return sheetRows(buf, '枚举字段').rows.filter((r) => String(r[0] || '').trim()).length } catch { return 0 } })()
  check('1.10 「枚举字段」含 20 组标准下拉', enums === 20, `groups=${enums}`)
  fs.writeFileSync('/tmp/pw/roster-template-r46.xlsx', buf)
}
// 点一次真实下载按钮，验证前端链路不报错
const dlBtn = importDialog.getByRole('button', { name: /下载导入模板/ }).first()
if (await dlBtn.count()) {
  const [download] = await Promise.all([
    page.waitForEvent('download', { timeout: 15000 }).catch(() => null),
    dlBtn.click(),
  ])
  check('1.11 点击「下载导入模板」触发真实下载', !!download, download ? download.suggestedFilename() : 'no download event')
}
await page.screenshot({ path: `${SHOTS}/r46-a1-import-dialog.png` })
// 关闭弹窗（不要真导入，避免污染后续断言）
const closeBtn = importDialog.locator('.t-dialog__close, .t-dialog__close-btn').first()
if (await closeBtn.count()) await closeBtn.click()
else await page.keyboard.press('Escape')
await page.waitForTimeout(1200)

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：在职状态筛选与四态动态表头 =====')
const statusSelect = page.locator('.hr-status-select').first()
check('2.1 搜索框后方存在「在职状态」下拉', await statusSelect.count() > 0)
await statusSelect.click()
await page.waitForTimeout(800)
const optText = await page.locator('.t-select-option, .t-select-option__item').allInnerTexts()
for (const v of ['在职', '试用期', '退休返聘', '离职']) {
  check(`2.2 下拉含「${v}」`, optText.some((t) => t.trim() === v), JSON.stringify(optText.slice(0, 8)))
}
await page.keyboard.press('Escape')
await page.waitForTimeout(600)

let heads = await headerTexts()
check('2.3 默认「在职」视图表头含核心字段', ['工号', '姓名', '部门', '在职状态'].every((h) => heads.includes(h)), JSON.stringify(heads.slice(0, 12)))

await selectStatus('试用期')
heads = await headerTexts()
check('2.4 切「试用期」表头含 试用期 / 转正日期', heads.includes('试用期') && heads.includes('转正日期'), JSON.stringify(heads))
check('2.5 「试用期」视图不含 离职日期 / 离职证明', !heads.includes('离职日期') && !heads.includes('离职证明'))

await selectStatus('离职')
heads = await headerTexts()
for (const h of ['离职日期', '离职原因', '离职证明', '离职备注']) {
  check(`2.6 切「离职」表头含「${h}」`, heads.includes(h), JSON.stringify(heads))
}
check('2.7 「离职」视图隐藏在职期专属列（试用期 / 个人社保账号）', !heads.includes('试用期') && !heads.includes('个人社保账号'))
await page.screenshot({ path: `${SHOTS}/r46-a2-resign-columns.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：三张 KPI 卡图标非空 =====')
const icons = await page.locator('.dash-kpi-card').evaluateAll((cards) => cards.map((c) => {
  const use = c.querySelector('.dash-kpi-card__icon svg use')
  const svg = c.querySelector('.dash-kpi-card__icon svg')
  return { label: c.querySelector('.dash-kpi-card__label')?.textContent?.trim() || '', href: use?.getAttribute('href') || '', hasSvg: !!svg }
}))
check('3.1 三张卡都有 svg 图标节点', icons.length === 3 && icons.every((i) => i.hasSvg), JSON.stringify(icons))
const VALID_ICONS = new Set(['usergroup', 'secured', 'wallet', 'verified', 'shield-error', 'home', 'user', 'user-safety'])
for (const i of icons) {
  const stem = String(i.href).replace('#t-icon-', '')
  check(`3.2 「${i.label}」图标 href 有效（非空白/非 shield）`, VALID_ICONS.has(stem), `href=${i.href}`)
}

// ---------------------------------------------------------------------------
console.log('\n===== 断言 4：办理离职弹窗与状态流转 =====')
const created = await api('POST', '/api/v1/fleet/hr/roster/employees', {
  employee_id: `${uniqueTag()}-R1`, name: '真机离职甲', id_number: ID_A,
  department: '生产部', position: '操作工', hire_date: '2020-01-01', 工作性质: '全职', 员工状态: '正式',
})
const empId = created.json?.data?.id
check('4.1 造 1 名在职员工', created.status === 200 && !!empId, `status=${created.status}`)
// 回到在职视图并刷新
await reloadHr()
await selectStatus('在职')
await page.waitForTimeout(1500)
const cb = page.locator('.t-table__body .t-checkbox').first()
if (await cb.count()) { await cb.click(); await page.waitForTimeout(900) }
const resignBtn = page.getByRole('button', { name: /办理离职/ }).first()
check('4.2 浮动栏「办理离职」按钮可见', await resignBtn.isVisible().catch(() => false))
if (await resignBtn.isVisible().catch(() => false)) await resignBtn.click()
const resignDialog = page.locator('.t-dialog', { hasText: '办理离职' }).last()
await resignDialog.waitFor({ state: 'visible', timeout: 15000 })
const rdText = await resignDialog.innerText()
for (const label of ['离职时间', '离职原因', '离职证明', '备注说明']) {
  check(`4.3 弹窗含「${label}」表单项`, rdText.includes(label))
}
check('4.4 弹窗含确认办理按钮', /确认办理/.test(rdText))
// 填原因（离职时间默认今天）
const reasonSelect = resignDialog.locator('.t-select').first()
await reasonSelect.click()
await page.waitForTimeout(700)
const firstReason = page.locator('.t-select-option, .t-select-option__item').filter({ hasText: '家庭原因' }).first()
if (await firstReason.count()) await firstReason.click()
else await page.getByText('家庭原因', { exact: true }).last().click()
await page.waitForTimeout(700)
const confirmBtn = resignDialog.getByRole('button', { name: /确认办理/ }).first()
await confirmBtn.click()
await page.waitForTimeout(2600)
const after = await api('GET', '/api/v1/fleet/hr/roster/employees?page=1&page_size=200')
const all = after.json?.data?.items || []
const resigned = all.find((e) => String(e.id) === String(empId))
check('4.5 确认后员工状态变为「离职」', String(resigned?.status || '') === '离职', `status=${resigned?.status}`)
check('4.6 离职原因已落库', String(resigned?.resign_reasons || '') === '家庭原因', `reason=${resigned?.resign_reasons}`)
await page.screenshot({ path: `${SHOTS}/r46-a4-resign-dialog.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 5：全局无 ISO8601 原始时间戳 =====')
const ISO_RE = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}|[+-]\d{2}:\d{2}(?=\s|$|<\/)|\.\d{5,}\+/
async function scanPage(url, label) {
  await page.goto(url, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(12000)
  await stripGuide()
  const text = await page.locator('body').innerText()
  const hit = text.match(ISO_RE)?.[0] || ''
  check(`5.x 「${label}」列表无 ISO 原始时间戳`, !hit, hit)
}
await scanPage(HR_URL, '人事管理')
await scanPage(`${BASE}/fleet`, '车队管理')
await scanPage(`${BASE}/invoices`, '发票管理')
// 打开一个详情抽屉再扫一次
await page.goto(HR_URL, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(12000)
await stripGuide()
const firstRow = page.locator('.t-table__body tr').first()
if (await firstRow.count()) { await firstRow.click(); await page.waitForTimeout(1800) }
const drawerText = await page.locator('.t-drawer').last().innerText().catch(() => '')
check('5.x 详情抽屉无 ISO 原始时间戳', !ISO_RE.test(drawerText), (drawerText.match(ISO_RE) || [''])[0])
await page.screenshot({ path: `${SHOTS}/r46-a5-datetime.png` })

// ---------------------------------------------------------------------------
async function reloadHr() {
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(13000)
  await stripGuide()
}

console.log('\n===== 汇总 =====')
const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|favicon|net::ERR_|网络错误，请检查您的网络连接|Failed to list models|Failed to load chat models|\[cmdk\] failed to load knowledge bases/.test(e))
check('6.1 控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 3).join(' | '))
console.log(`\nRESULT ${ok}/${total} PASS`)
if (failures.length) { console.log('FAILURES:'); failures.forEach((f) => console.log('  - ' + f)) }
await browser.close()
process.exit(failures.length ? 1 : 0)
