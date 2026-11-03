// 第 28 轮真机验收：车队 12 菜单三段式巡检 + 派生台账 + 合计行 + 新增记录。
//
// 断言：
//  A. 12 个菜单逐个点击，右侧均渲染三段式中台布局（KPI 卡片 + 筛选栏 + 表格）；
//  B. 「路桥费」「加油充电」展示发票派生的真实记录，可按车牌筛选，合计行准确；
//  C. 勾选 2 行 → 合计行文案变「合计（已选 2 条）」；
//  D. 新增一条「轮胎管理」记录 → 200（不再 400 请选择车辆），并立即出现在列表；
//  E. 全程控制台 0 error。
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs/fleet'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const MENUS = [
  ['vehicle-archive', '车辆档案'],
  ['driver-archive', '司机档案'],
  ['maintain-archive', '维保记录'],
  ['tire', '轮胎管理'],
  ['inspection', '年检管理'],
  ['fuel-charge', '加油充电'],
  ['road-toll', '路桥费'],
  ['repair-cost', '维修保养费'],
  ['insurance-claim', '保险理赔'],
  ['violation', '违章处理'],
  ['material', '辅材记录'],
  ['car-request', '请车记录'],
]

const results = []
const consoleErrors = []
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}
const num = (t) => {
  const m = String(t || '').replace(/[,\s¥￥]/g, '').match(/-?\d+(\.\d+)?/)
  return m ? Number(m[0]) : NaN
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  const hide = () => {
    if (!document.head) return
    const st = document.createElement('style')
    st.textContent = '.guide{display:none!important}'
    document.head.appendChild(st)
  }
  if (document.head) hide()
  else document.addEventListener('DOMContentLoaded', hide)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 300)) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR ' + String(e).slice(0, 300)))

await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(3500)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

async function waitRows(min = 1, timeout = 45000) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    const n = await page.evaluate(() =>
      Array.from(document.querySelectorAll('.doc-table tbody tr'))
        .filter((tr) => tr.querySelectorAll('td').length > 3).length)
    if (n >= min) return n
    await page.waitForTimeout(600)
  }
  return 0
}

// ---- A. 12 菜单巡检 ----
for (const [key, label] of MENUS) {
  await page.evaluate((k) => {
    const items = Array.from(document.querySelectorAll('.fleet-side-item'))
    const hit = items.find((el) => el.textContent.replace(/\s/g, '') === k)
    if (hit) hit.click()
  }, label)
  // 等 KPI 卡片真正渲染（骨架屏期间 .dash-kpi-card 数为 0，会误判成布局缺失）
  const start = Date.now()
  while (Date.now() - start < 30000) {
    const n = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length)
    if (n > 0) break
    await page.waitForTimeout(500)
  }
  await page.waitForTimeout(1200)
  const snap = await page.evaluate(() => {
    const kpi = document.querySelectorAll('.dash-kpi-card').length
    const toolbar = !!document.querySelector('.doc-toolbar')
    const table = !!document.querySelector('.doc-table')
    const foot = document.querySelectorAll('.doc-table tfoot td, .t-table__footer td').length
    const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const rows = Array.from(document.querySelectorAll('.doc-table tbody tr'))
      .filter((tr) => tr.querySelectorAll('td').length > 3)
      .map((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim()))
    const empty = document.querySelector('.biz-empty, .t-table__empty, .t-empty')?.textContent?.trim() || ''
    return { kpi, toolbar, table, foot, head, rows, empty }
  })
  const layoutOK = snap.kpi > 0 && snap.toolbar && snap.table
  check(`A ${label} 三段式布局`, layoutOK,
    `kpi=${snap.kpi} toolbar=${snap.toolbar} table=${snap.table} foot=${snap.foot} rows=${snap.rows.length} head=${snap.head.filter(Boolean).slice(0, 6).join('/')}`)
  await page.screenshot({ path: `${SHOTS}/r28-menu-${key}.png` })
}

// ---- B. 路桥费：派生记录 + 车牌筛选 + 合计行 ----
await page.evaluate(() => {
  const items = Array.from(document.querySelectorAll('.fleet-side-item'))
  const hit = items.find((el) => el.textContent.replace(/\s/g, '') === '路桥费')
  if (hit) hit.click()
})
await page.waitForTimeout(3500)
const roadRows = await waitRows(1, 45000)
const roadSnap = await page.evaluate(() => {
  const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
  const trs = Array.from(document.querySelectorAll('.doc-table tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
  const rows = trs.map((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim()))
  const foot = Array.from(document.querySelectorAll('.doc-table tfoot td, .t-table__footer td')).map((td) => td.textContent.trim())
  const pager = document.querySelector('.t-pagination__total')?.textContent?.trim() || ''
  return { head, rows, foot, pager }
})
check('B1 路桥费有派生记录', roadRows > 0, `rows=${roadRows} pager=${roadSnap.pager}`)
const amtIdx = roadSnap.head.findIndex((h) => h.replace(/\s/g, '').includes('含税金额'))
const taxIdx = roadSnap.head.findIndex((h) => h.replace(/\s/g, '').includes('税额'))
const sumAmount = roadSnap.rows.reduce((s, r) => s + (Number.isFinite(num(r[amtIdx])) ? num(r[amtIdx]) : 0), 0)
const sumTax = roadSnap.rows.reduce((s, r) => s + (Number.isFinite(num(r[taxIdx])) ? num(r[taxIdx]) : 0), 0)
const footAmount = num(roadSnap.foot.find((t) => Number.isFinite(num(t)) && Math.abs(num(t) - sumAmount) < 1))
check('B2 合计行金额 == 本页行金额之和', Number.isFinite(footAmount) && Math.abs(footAmount - sumAmount) < 0.05,
  `foot=${roadSnap.foot.filter(Boolean).join('|')} sumAmount=${sumAmount.toFixed(2)} sumTax=${sumTax.toFixed(2)}`)
check('B3 合计行含「共 N 条」文案', roadSnap.foot.some((t) => /合计（共 \d+ 条）/.test(t)),
  roadSnap.foot.filter(Boolean).join('|'))

// 车牌筛选
const plateInput = page.locator('.bfs-text input, input[placeholder="车牌号"]').first()
if (await plateInput.count()) {
  await plateInput.click()
  await plateInput.fill('渝C81022')
  await plateInput.press('Enter')
  // 轮询等待：筛选走 300ms 防抖 + 后端查询 + 表格重渲染，固定 sleep 会拿到
  // 旧数据（未筛选的 20 行）而误报失败。以「合计行的总数变小」为收敛判据。
  const readFiltered = () => page.evaluate(() => {
    const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const trs = Array.from(document.querySelectorAll('.doc-table tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const pi = head.findIndex((h) => h.replace(/\s/g, '').includes('车牌号'))
    const pager = document.querySelector('.t-pagination__total')?.textContent || ''
    const m = pager.match(/([\d,]+)\s*条/)
    return {
      total: m ? Number(m[1].replace(/,/g, '')) : -1,
      plates: trs.map((tr) => Array.from(tr.querySelectorAll('td'))[pi]?.textContent.trim() || ''),
    }
  })
  let filtered = { total: -1, plates: [] }
  const t0 = Date.now()
  while (Date.now() - t0 < 30000) {
    filtered = await readFiltered()
    // 收敛：总数已变小 且 当前页全部是目标车牌
    if (filtered.total > 0 && filtered.total < 1528 &&
      filtered.plates.length > 0 && filtered.plates.every((p) => p.includes('渝C81022'))) break
    await page.waitForTimeout(700)
  }
  check('B4 车牌筛选生效（全部渝C81022）',
    filtered.plates.length > 0 && filtered.plates.every((p) => p.includes('渝C81022')) && filtered.total < 1528,
    `total=${filtered.total} n=${filtered.plates.length} plates=${filtered.plates.slice(0, 5).join(',')}`)
  await page.screenshot({ path: `${SHOTS}/r28-roadtoll-filtered.png` })
  // 清空筛选
  await plateInput.fill('')
  await plateInput.press('Enter')
  await page.waitForTimeout(2500)
} else {
  check('B4 车牌筛选生效（全部渝C81022）', false, '车牌号筛选器未渲染')
}

// ---- C. 勾选 2 行 → 合计（已选 2 条） ----
await page.waitForTimeout(1500)
const boxes = page.locator('.doc-table tbody tr .t-checkbox')
const boxCount = await boxes.count()
if (boxCount >= 2) {
  await boxes.nth(0).click()
  await boxes.nth(1).click()
  await page.waitForTimeout(1500)
  const foot = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.doc-table tfoot td, .t-table__footer td')).map((td) => td.textContent.trim()))
  check('C1 勾选后合计行变「已选 2 条」', foot.some((t) => /合计（已选 2 条）/.test(t)), foot.filter(Boolean).join('|'))
  const selAmount = await page.evaluate(() => {
    const trs = Array.from(document.querySelectorAll('.doc-table tbody tr.is-selected'))
    const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const ai = head.findIndex((h) => h.replace(/\s/g, '').includes('含税金额'))
    return trs.map((tr) => Array.from(tr.querySelectorAll('td'))[ai]?.textContent.trim() || '')
  })
  check('C2 勾选行金额可读', selAmount.length === 2, selAmount.join('|'))
  await page.screenshot({ path: `${SHOTS}/r28-selected-2.png` })
  // 清除勾选
  await page.evaluate(() => {
    const clear = Array.from(document.querySelectorAll('.doc-toolbar button, .batch-bar-clear'))
      .find((b) => b.textContent.trim() === '清除')
    if (clear) clear.click()
  })
  await page.waitForTimeout(1200)
} else {
  check('C1 勾选后合计行变「已选 2 条」', false, `可勾选行不足 2（${boxCount}）`)
}

// ---- D. 新增轮胎记录 → 200 ----
await page.evaluate(() => {
  const items = Array.from(document.querySelectorAll('.fleet-side-item'))
  const hit = items.find((el) => el.textContent.replace(/\s/g, '') === '轮胎管理')
  if (hit) hit.click()
})
await page.waitForTimeout(3000)
const primaryBtn = page.locator('.doc-toolbar__trailing .t-button--theme-primary').first()
const respPromise = page.waitForResponse(
  (r) => r.url().includes('/api/v1/fleet/records') && r.request().method() === 'POST',
  { timeout: 30000 },
).catch(() => null)
await primaryBtn.click()
await page.waitForTimeout(2000)
const drawerOpen = await page.evaluate(() => {
  const el = document.querySelector('.bsd-drawer .t-drawer__content-wrapper')
  return !!el && el.getBoundingClientRect().width > 0
})
check('D1 新增抽屉打开', drawerOpen, drawerOpen ? 'open' : 'not open')

if (drawerOpen) {
  // 填「轮胎编号」与「车牌号」
  const inputs = await page.evaluate(() => {
    const pane = document.querySelector('.bsd-drawer .t-drawer__content-wrapper')
    return Array.from(pane.querySelectorAll('.t-form__item')).map((it) => it.querySelector('.t-form__label')?.textContent?.trim() || '')
  })
  const fillByLabel = async (label, value) => {
    const item = page.locator('.bsd-drawer .t-drawer__content-wrapper .t-form__item')
      .filter({ has: page.locator('.t-form__label', { hasText: label }) }).first()
    const input = item.locator('input').first()
    if (await input.count()) { await input.fill(value); return true }
    return false
  }
  const ok1 = await fillByLabel('轮胎编号', 'LT-R28-001')
  const ok2 = await fillByLabel('车牌号', '渝C81022')
  check('D2 表单字段由 subs 派生并可填', ok1 && ok2, `labels=${inputs.slice(0, 8).join('/')}`)
  await page.waitForTimeout(500)
  const saveBtn = page.locator('.bsd-drawer button').filter({ hasText: '保存' }).first()
  await saveBtn.click()
  const resp = await respPromise
  const status = resp ? resp.status() : 0
  check('D3 新增记录返回 200（不再 400 请选择车辆）', status === 200, `status=${status}`)
  await page.waitForTimeout(3000)
  const after = await page.evaluate(() => {
    const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const ti = head.findIndex((h) => h.replace(/\s/g, '').includes('轮胎编号'))
    return Array.from(document.querySelectorAll('.doc-table tbody tr'))
      .filter((tr) => tr.querySelectorAll('td').length > 3)
      .map((tr) => Array.from(tr.querySelectorAll('td'))[ti]?.textContent.trim() || '')
  })
  check('D4 新记录立即出现在列表', after.some((t) => t.includes('LT-R28-001')), after.slice(0, 6).join(','))
  await page.screenshot({ path: `${SHOTS}/r28-tire-created.png` })
}

// ---- E. 控制台 0 error ----
const realErrors = consoleErrors.filter((e) => !/favicon|404 \(Not Found\)/.test(e))
check('E1 控制台 0 error', realErrors.length === 0, realErrors.slice(0, 3).join(' || ') || 'clean')

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY pass=${results.length - failed.length}/${results.length}`)
if (failed.length) console.log('FAILED:', failed.map((f) => f.name).join(' | '))
await browser.close()
process.exit(failed.length ? 1 : 0)
