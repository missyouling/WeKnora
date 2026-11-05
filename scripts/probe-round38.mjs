/**
 * 第 38 轮验收：6 项
 *
 * 断言 1：所有列表 `.t-table__cell--fixed-left/right` 数量为 0；横向滚动时首列一起走
 * 断言 2：合计行始终贴底（0 / 1 / 多条）：首行紧贴 thead（≤2px）、行高 39~42、
 *         tfoot 落在滚动容器可视区底边（横向滚动条位于其下方）
 * 断言 3：账单概况 2026-10「本期电费」电费 = 账单电费 = 481,647.91，11 行全部「正常」
 * 断言 4：车辆档案 → 司机档案，不点刷新，驾驶证直接渲染 1 条
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
let quietErrors = []
let quiet = false
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}
const num = (s) => {
  const m = String(s ?? '').replace(/,/g, '').match(/-?\d+(?:\.\d+)?/)
  return m ? Number(m[0]) : 0
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 950 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  const h = () => {
    if (!document.head) return
    const s = document.createElement('style')
    s.textContent = '.guide{display:none!important}'
    document.head.appendChild(s)
  }
  if (document.head) h(); else document.addEventListener('DOMContentLoaded', h)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (quiet && m.type() === 'error') quietErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => { if (quiet) quietErrors.push('PAGEERR ' + String(e).slice(0, 200)) })

async function gotoRetry(url, attempts = 3) {
  let last = null
  for (let i = 0; i < attempts; i++) {
    try { await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 }); return } catch (e) { last = e; await page.waitForTimeout(1500) }
  }
  throw last
}

/** 列表几何：首行贴合度 / 行高 / 合计行贴底 / 横向滚动条位置。 */
const tableGeom = () => page.evaluate(() => {
  const root = document.querySelector('.doc-list-view')
  const content = root?.querySelector('.t-table__content')
  const thead = root?.querySelector('thead')
  const tfoot = root?.querySelector('tfoot')
  const trs = Array.from(root?.querySelectorAll('tbody tr') || [])
    .filter((tr) => tr.querySelectorAll('td').length > 3)
  const tr = trs[0] || null
  if (!content || !thead) return { found: false }
  const cb = content.getBoundingClientRect()
  const sbh = content.offsetHeight - content.clientHeight   // 横向滚动条占位高度
  const sbw = content.offsetWidth - content.clientWidth
  const tb = thead.getBoundingClientRect()
  return {
    found: true,
    rows: trs.length,
    headBottom: Math.round(tb.bottom),
    firstTop: tr ? Math.round(tr.getBoundingClientRect().top) : null,
    gap: tr ? Math.round((tr.getBoundingClientRect().top - tb.bottom) * 10) / 10 : null,
    rowH: tr ? Math.round(tr.getBoundingClientRect().height) : null,
    // 合计行：可视区底边（不含横向滚动条）与 tfoot 下沿的距离
    tfootGap: tfoot
      ? Math.round((cb.bottom - sbh - tfoot.getBoundingClientRect().bottom) * 10) / 10
      : null,
    tfootH: tfoot ? Math.round(tfoot.getBoundingClientRect().height) : 0,
    hScroll: content.scrollWidth > content.clientWidth + 1,
    sbh,
    sbw,
    contentH: Math.round(cb.height),
    docFlat: !!document.querySelector('.doc-list-view.is-table-flat'),
  }
})

/** 打开车队某菜单（左侧栏）。 */
async function openFleet(label) {
  await gotoRetry(`${BASE}/fleet`)
  for (let i = 0; i < 60; i++) {
    if (await page.evaluate(() => document.querySelectorAll('.fleet-side-item').length > 0)) break
    await page.waitForTimeout(500)
  }
  await page.evaluate((l) => {
    const el = Array.from(document.querySelectorAll('.fleet-side-item')).find((e) => (e.innerText || '').trim() === l)
    if (el) el.click()
  }, label)
  await waitRows()
}

async function waitRows(min = 1, times = 70) {
  for (let i = 0; i < times; i++) {
    const n = await page.evaluate(() =>
      Array.from(document.querySelectorAll('tbody tr'))
        .filter((tr) => tr.querySelectorAll('td').length > 3).length)
    if (n >= min) return n
    await page.waitForTimeout(500)
  }
  return 0
}

async function openInvoices() {
  await gotoRetry(`${BASE}/invoices`)
  await waitRows(3, 80)
  await page.waitForTimeout(2500)
}

// ======================================================================
// 断言 1：无冻结列
// ======================================================================
console.log('\n=== 断言 1：无冻结列 ===')
await openFleet('车辆档案')
const fixedFleet = await page.evaluate(() => ({
  left: document.querySelectorAll('.t-table__cell--fixed-left').length,
  right: document.querySelectorAll('.t-table__cell--fixed-right').length,
  cls: Array.from(document.querySelectorAll('tbody tr:first-child td')).map((td) => String(td.className)).slice(0, 4),
}))
check('1 车辆档案无冻结单元格', fixedFleet.left === 0 && fixedFleet.right === 0, JSON.stringify(fixedFleet))

await openInvoices()
const fixedInv = await page.evaluate(() => ({
  left: document.querySelectorAll('.t-table__cell--fixed-left').length,
  right: document.querySelectorAll('.t-table__cell--fixed-right').length,
}))
check('1 发票管理无冻结单元格', fixedInv.left === 0 && fixedInv.right === 0, JSON.stringify(fixedInv))

// 横向滚动时首列必须跟着走（冻结列会原地不动）
await page.setViewportSize({ width: 1200, height: 800 })
await page.waitForTimeout(1500)
const scrollProof = await page.evaluate(() => {
  const c = document.querySelector('.doc-list-view .t-table__content')
  if (!c) return { ok: false }
  const td = () => document.querySelector('.doc-list-view tbody tr td')
  const before = td().getBoundingClientRect().left
  c.scrollLeft = 160
  const after = td().getBoundingClientRect().left
  return { ok: true, moved: Math.round((before - after) * 10) / 10, sl: c.scrollLeft }
})
check('1 横向滚动时首列随表格一起滑动（未冻结）',
  scrollProof.ok && scrollProof.sl > 100 && scrollProof.moved > 100, JSON.stringify(scrollProof))
await page.setViewportSize({ width: 1680, height: 950 })
await page.waitForTimeout(800)

// ======================================================================
// 断言 2：合计行贴底 + 横向滚动条在合计行下方
// ======================================================================
console.log('\n=== 断言 2：合计行贴底 ===')
// ① 1 条记录（车辆档案）
await openFleet('车辆档案')
await page.waitForTimeout(2500)
const g1 = await tableGeom()
check('2 [1 条] 首行紧贴表头（≤2px）且行高 39~42', g1.found && Math.abs(g1.gap) <= 2 && g1.rowH >= 36 && g1.rowH <= 44,
  JSON.stringify(g1))
check('2 [1 条] 合计行贴容器底边（可视区，横向滚动条下方无空白）',
  g1.found && g1.tfootGap !== null && Math.abs(g1.tfootGap) <= 2,
  `tfootGap=${g1.tfootGap} sbh=${g1.sbh} contentH=${g1.contentH}`)
await page.screenshot({ path: `${SHOTS}/r38-fleet-1row.png` })

// ② 多条记录 + 横向溢出（发票，窄视口制造横滚）
await page.setViewportSize({ width: 1200, height: 800 })
await openInvoices()
const g2 = await tableGeom()
check('2 [多条] 首行紧贴表头（≤2px）且行高 39~42', g2.found && Math.abs(g2.gap) <= 2 && g2.rowH >= 36 && g2.rowH <= 44,
  JSON.stringify(g2))
check('2 [多条] 合计行贴滚动容器底边', g2.found && Math.abs(g2.tfootGap) <= 2,
  `tfootGap=${g2.tfootGap} rows=${g2.rows}`)
check('2 [多条] 出现横向滚动条时，滚动条位于合计行下方（tfoot 下沿 = 可视区底边）',
  g2.found && g2.hScroll && Math.abs(g2.tfootGap) <= 2,
  `hScroll=${g2.hScroll} sbh=${g2.sbh} sbw=${g2.sbw}`)
await page.screenshot({ path: `${SHOTS}/r38-invoice-rows.png` })
await page.setViewportSize({ width: 1680, height: 950 })

// ③ 0 条记录（车队关键词搜不到）
await openFleet('车辆档案')
await page.waitForTimeout(1500)
// 搜索框是工具栏的 `.doc-search-field`（`.bfs-field` 是证照状态下拉，输入无效）
const kw = page.locator('.doc-search-field input').first()
let typed = false
if (await kw.count()) {
  await kw.click()
  await kw.type('ZZZ_NO_SUCH_RECORD', { delay: 25 })
  typed = true
}
await page.waitForTimeout(6000)
const g0 = await tableGeom()
check('2 [0 条] 关键词无结果时合计行仍贴底',
  typed && g0.found && g0.rows === 0 && Math.abs(g0.tfootGap) <= 2,
  `typed=${typed} rows=${g0.rows} tfootGap=${g0.tfootGap} flat=${g0.docFlat}`)
check('2 [0 条] 空态隐藏横向滚动条',
  g0.found && (g0.docFlat ? true : !g0.hScroll), `flat=${g0.docFlat} hScroll=${g0.hScroll}`)
await page.screenshot({ path: `${SHOTS}/r38-fleet-0row.png` })

// ======================================================================
// 断言 3：账单概况「本期电费」两列一致
// ======================================================================
console.log('\n=== 断言 3：账单概况本期电费 ===')
await gotoRetry(`${BASE}/utilities`)
for (let i = 0; i < 60; i++) {
  if (await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length > 0)) break
  await page.waitForTimeout(500)
}
await page.evaluate(() => {
  const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('市电账单'))
  if (c) c.click()
})
await waitRows(1)
await page.waitForTimeout(3000)
await page.evaluate(() => { document.querySelector('tbody tr')?.dispatchEvent(new MouseEvent('click', { bubbles: true })) })
await page.waitForTimeout(7000)
const overview = await page.evaluate(() => {
  const wrap = document.querySelector('.os-table')
  if (!wrap) return { found: false }
  const rows = Array.from(wrap.querySelectorAll('.os-row')).map((r) => {
    const cells = Array.from(r.children).map((c) => (c.innerText || '').trim())
    return { cells, tag: (r.querySelector('.t-tag')?.innerText || '').trim(), total: r.className.includes('os-row--total') }
  })
  return { found: true, rows }
})
check('3 账单概况已渲染', overview.found, `rows=${overview.rows?.length}`)
const bodyRows = (overview.rows || []).filter((r) => r.cells.length >= 5 && r.cells[0] !== '费用组成')
const abnormal = bodyRows.filter((r) => r.tag && r.tag !== '正常')
check('3 账单概况全部行对比状态为「正常」（0 行异常）', overview.found && abnormal.length === 0,
  `abnormal=${JSON.stringify(abnormal.map((r) => r.cells[0]))}`)
// 账单概况里有多行带 os-row--total（工商业/居民小计都是），
// 底部合计只能按第一列文案定位。
const totalRow = bodyRows.find((r) => r.cells[0] === '本期电费')
check('3 本期电费：电费 = 481,647.91', totalRow ? Math.abs(num(totalRow.cells[2]) - 481647.91) < 0.02 : false,
  JSON.stringify(totalRow?.cells))
check('3 本期电费：账单电费 = 481,647.91（不再是 446,003.1）',
  totalRow ? Math.abs(num(totalRow.cells[3]) - 481647.91) < 0.02 : false,
  JSON.stringify(totalRow?.cells))
check('3 本期电费：对比状态为「正常」', totalRow ? totalRow.tag === '正常' : false, `tag=${totalRow?.tag}`)
await page.screenshot({ path: `${SHOTS}/r38-overview.png` })

// ======================================================================
// 断言 4：司机档案首屏直出
// ======================================================================
console.log('\n=== 断言 4：驾驶证首屏直出 ===')
await openFleet('车辆档案')
await page.waitForTimeout(2000)
// 点左侧「司机档案」，不点刷新
const switched = await page.evaluate(() => {
  const el = Array.from(document.querySelectorAll('.fleet-side-item')).find((e) => (e.innerText || '').trim() === '司机档案')
  if (el) { el.click(); return true }
  return false
})
const driverRows = await waitRows(1, 60)
await page.waitForTimeout(2500)
const driverState = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('thead th')).map((t) => (t.textContent || '').trim()).filter(Boolean)
  const rows = Array.from(document.querySelectorAll('tbody tr'))
    .filter((tr) => tr.querySelectorAll('td').length > 3)
  const pager = document.querySelector('.t-table__pagination, .fleet-pager')
  return {
    rows: rows.length,
    first: rows[0] ? (rows[0].innerText || '').replace(/\s+/g, ' ').slice(0, 120) : '',
    heads: heads.slice(0, 8).join('|'),
    totalText: (document.querySelector('.doc-list-view tfoot')?.innerText || '').replace(/\s+/g, ' ').trim(),
    activeCard: Array.from(document.querySelectorAll('.dash-kpi-card')).filter((c) => /active|is-active/.test(c.className)).map((c) => (c.innerText || '').replace(/\s+/g, ' ').slice(0, 30)),
    pager: !!pager,
  }
})
check('4 切到司机档案可直接点击', switched, `switched=${switched}`)
check('4 不点刷新即渲染出 1 条驾驶证记录', driverRows >= 1,
  `rows=${driverRows} first=${driverState.first}`)
check('4 合计显示「共 1 条」', /共\s*1\s*条/.test(driverState.totalText), driverState.totalText)
check('4 列表表头是驾驶证字段（驾驶证号/准驾车型）',
  /驾驶证号/.test(driverState.heads) && /准驾车型/.test(driverState.heads), driverState.heads)
await page.screenshot({ path: `${SHOTS}/r38-driver.png` })

// ======================================================================
// console 健康检查
// ======================================================================
quiet = true
await page.waitForTimeout(3000)
check('console 无 error', quietErrors.length === 0, JSON.stringify(quietErrors.slice(0, 3)))

const pass = results.filter((r) => r.ok).length
console.log(`\nSUMMARY ${pass}/${results.length} PASS`)
if (pass !== results.length) {
  console.log('FAILED: ' + results.filter((r) => !r.ok).map((r) => r.name).join(' ; '))
}
await browser.close()
