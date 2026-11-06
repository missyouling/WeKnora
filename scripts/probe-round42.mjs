/**
 * 第 42 轮真机验收（Playwright + PDF 文本层）。
 *
 * 断言 1：访问站点根路径 / 与 /platform → 自动落地「日常事务」，知识库菜单仍在。
 * 断言 2：持睿汽车 2026-10 费用清单打印预览 = 2 页，且不含「应收费用汇总」；
 *         第 1 页含电费分项细项全表，第 2 页含电费/水费核算明细。
 * 断言 3：光伏账单 2026-10 的发电量/上网电量/结算金额非 0，费用清单同步；
 *         连续刷新 2 次不发起任何提取 POST，历史抽屉「正在执行」恒为 0。
 * 断言 4：绿色状态条、顶部「正在执行」计数、右侧历史抽屉绑定同一数据源，
 *         抽屉打开后 2.5s 轮询自动刷新状态标签。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
const consoleErrors = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
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
  if (document.head) h()
  else document.addEventListener('DOMContentLoaded', h)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERR ' + String(e).slice(0, 200)))

let extractPosts = 0
page.on('request', (r) => {
  if (r.method() === 'POST' && /\/extract-(solar-bill|utility-bill|business|invoice|energy|fleet-document)/.test(r.url())) {
    extractPosts++
  }
})

async function gotoRetry(url, attempts = 3) {
  let last = null
  for (let i = 0; i < attempts; i++) {
    try { await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 }); return } catch (e) { last = e; await page.waitForTimeout(1500) }
  }
  throw last
}
async function waitFor(fn, times = 60, gap = 500) {
  for (let i = 0; i < times; i++) {
    if (await page.evaluate(fn)) return true
    await page.waitForTimeout(gap)
  }
  return false
}
const bodyText = () => page.evaluate(() => document.body.innerText || '')

// ---------------------------------------------------------------------------
// 断言 1：默认首页
// ---------------------------------------------------------------------------
for (const path of ['/', '/platform']) {
  await gotoRetry(`${ORIGIN}${path}`)
  // 首屏要拉 KB / 分类 / 统计，真机实测 12~20s 才完成路由重定向与菜单渲染
  await waitFor(() => /daily-affairs/.test(location.href), 60, 500)
  const url = page.url()
  const ok = /daily-affairs/.test(url) && !/\/knowledge-bases($|\?)/.test(url)
  check(`断言1 根路径 ${path} 落地日常事务`, ok, `url=${url}`)
  if (path === '/') {
    await waitFor(() => (document.body.innerText || '').includes('知识库'), 30, 500)
    const menuOk = (await bodyText()).includes('知识库')
    check('断言1 知识库菜单入口仍保留', menuOk, `知识库=${menuOk}`)
  }
}

// ---------------------------------------------------------------------------
// 断言 2：打印预览 2 页 / 无应收汇总
// ---------------------------------------------------------------------------
async function openCostList() {
  await gotoRetry(`${BASE}/utilities`)
  await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0, 60)
  await page.evaluate(() => {
    const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('费用清单'))
    if (c) c.click()
  })
  await waitFor(() => Array.from(document.querySelectorAll('th')).some((e) => (e.textContent || '').includes('光伏发电量')), 60)
  await page.waitForTimeout(1500)
  const sel = page.locator('.bfs-select').first()
  if (await sel.count()) {
    await sel.click({ timeout: 8000 })
    await page.waitForTimeout(900)
    // TDesign 下拉是 portal：必须真实 click，DOM click 不触发委托
    const opt = page.getByText('持睿汽车', { exact: true }).last()
    if (await opt.count()) await opt.click({ timeout: 8000 })
  }
  await page.waitForTimeout(2500)
  await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3), 60)
  await page.waitForTimeout(1500)
}

async function makeStatementPdf(month, outPath) {
  await openCostList()
  const picked = await page.evaluate((m) => {
    const trs = Array.from(document.querySelectorAll('tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const tr = trs.find((t) => (t.innerText || '').includes(m))
    if (!tr) return false
    const cb = tr.querySelector('input[type="checkbox"], .t-checkbox')
    if (cb) cb.click()
    return !!cb
  }, month)
  if (!picked) return 0
  await page.waitForTimeout(1200)
  await waitFor(() => !!document.querySelector('.doc-batch-bar-fixed'), 20)
  const printBtn = page.getByRole('button', { name: '打印清单' })
  if (!(await printBtn.count())) return 0
  await printBtn.click()
  const ok = await waitFor(() => {
    const d = document.querySelector('.ppd-dialog')
    return !!d && (d.innerText || '').includes('打印预览')
  }, 40, 600)
  if (!ok) return 0
  await page.waitForTimeout(3500)
  const b64 = await page.evaluate(async () => {
    const el = document.querySelector('.ppd-dialog iframe, .ppd-dialog embed, .ppd-dialog object')
    const a = Array.from(document.querySelectorAll('.ppd-dialog a[download]'))
    let url = el?.src || a.find((x) => (x.href || '').startsWith('blob:'))?.href || ''
    if (!url) {
      const blobs = performance.getEntriesByType('resource').map((r) => r.name).filter((n) => n.startsWith('blob:'))
      url = blobs[blobs.length - 1] || ''
    }
    if (!url) return ''
    const buf = await (await fetch(url)).arrayBuffer()
    let s = ''
    const bytes = new Uint8Array(buf)
    for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
    return btoa(s)
  })
  if (!b64) return 0
  fs.writeFileSync(outPath, Buffer.from(b64, 'base64'))
  return fs.statSync(outPath).size
}

async function pdfPages(path) {
  const pdfjs = await import('/opt/pw-run/node_modules/pdfjs-dist/legacy/build/pdf.mjs')
  const doc = await pdfjs.getDocument({ data: new Uint8Array(fs.readFileSync(path)), useSystemFonts: true }).promise
  const pages = []
  for (let p = 1; p <= doc.numPages; p++) {
    const tc = await (await doc.getPage(p)).getTextContent()
    const items = tc.items.map((i) => ({ s: i.str, x: i.transform[4], y: i.transform[5] }))
    items.sort((a, b) => b.y - a.y || a.x - b.x)
    let lastY = null
    let line = []
    let out = ''
    for (const it of items) {
      if (lastY !== null && Math.abs(it.y - lastY) > 2) { out += line.join(' ') + '\n'; line = [] }
      line.push(it.s)
      lastY = it.y
    }
    out += line.join(' ') + '\n'
    pages.push(out)
  }
  return { pages, numPages: doc.numPages }
}

const pdfPath = '/tmp/pw/r42-statement-2026-10.pdf'
const pdfSize = await makeStatementPdf('2026-10', pdfPath)
if (pdfSize > 0) {
  const { pages, numPages } = await pdfPages(pdfPath)
  const all = pages.join('\n')
  check('断言2 打印对象为持睿汽车租户视角', all.includes('持睿汽车') && !all.includes('业主全厂口径'),
    `持睿=${all.includes('持睿汽车')} 业主口径=${all.includes('业主全厂口径')}`)
  check('断言2 打印预览总页数为 2', numPages === 2, `numPages=${numPages} size=${pdfSize}`)
  check('断言2 PDF 不含「应收费用汇总」', !all.includes('应收费用汇总'), `命中=${all.includes('应收费用汇总')}`)
  check('断言2 第 1 页含电费分项细项全表',
    /费用类别/.test(pages[0] || '') && /合计/.test(pages[0] || '') && /电费分项细项/.test(pages[0] || ''),
    `第1页前120字=${(pages[0] || '').slice(0, 120).replace(/\n/g, ' ')}`)
  const p2 = pages.slice(1).join('\n')
  check('断言2 第 2 页含电费 + 水费核算明细',
    /电费核算明细/.test(p2) && /水费核算明细/.test(p2),
    `电费=${/电费核算明细/.test(p2)} 水费=${/水费核算明细/.test(p2)}`)
  fs.writeFileSync('/tmp/pw/r42-pages.txt', pages.map((p, i) => `=== P${i + 1} ===\n${p}`).join('\n'))
} else {
  check('断言2 打印预览生成 PDF', false, `pdfSize=${pdfSize}`)
}

// ---------------------------------------------------------------------------
// 断言 3：光伏字段完整 / 费用清单联动 / 刷新不重提
// ---------------------------------------------------------------------------
await gotoRetry(`${BASE}/utilities`)
await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0, 60)
await page.evaluate(() => {
  const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => /光伏/.test(e.innerText || ''))
  if (c) c.click()
})
await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3), 60)
await page.waitForTimeout(2500)
const solarText = await bodyText()
const numRe = (v) => new RegExp(v.replace(/(\d)(?=(\d{3})(\.|$))/g, '$1,?'))
const hasGen = numRe('66026').test(solarText)
const hasGrid = numRe('2970').test(solarText)
const hasAmt = numRe('1125').test(solarText) && /1125[,.]?14|1,125\.14/.test(solarText)
check('断言3 光伏账单 2026-10 三大字段非空非零', /2026-10/.test(solarText) && hasGen && hasGrid && hasAmt,
  `发电量=${hasGen} 上网=${hasGrid} 金额=${hasAmt} 片段=${solarText.slice(-350).replace(/\n/g, ' ')}`)

await page.evaluate(() => {
  const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('费用清单'))
  if (c) c.click()
})
await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3), 60)
await page.waitForTimeout(2500)
const costText = await bodyText()
const cGen = numRe('66026').test(costText)
const cGrid = numRe('2970').test(costText)
const cAmt = numRe('1125').test(costText) && /1125[,.]?14|1,125\.14/.test(costText)
check('断言3 费用清单 2026-10 光伏三列同步真实数值', cGen && cGrid && cAmt,
  `发电量=${cGen} 上网=${cGrid} 金额=${cAmt} 片段=${costText.slice(-300).replace(/\n/g, ' ')}`)

extractPosts = 0
for (let i = 0; i < 2; i++) {
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(4000)
}
check('断言3 连续刷新 2 次不发起任何提取 POST', extractPosts === 0, `extractPosts=${extractPosts}`)

// ---------------------------------------------------------------------------
// 断言 4：状态条 / 正在执行计数 / 抽屉同源
// ---------------------------------------------------------------------------
await gotoRetry(`${BASE}/utilities`)
await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0, 60)
const historyBtn = page.getByRole('button', { name: /历史记录|已上传文件/ }).first()
let drawerOpened = false
if (await historyBtn.count()) {
  await historyBtn.click()
  drawerOpened = await waitFor(() => !!document.querySelector('.t-drawer'), 25, 400)
}
if (!drawerOpened) {
  // 兜底：直接点带「历史记录」文本的元素
  await page.evaluate(() => {
    const el = Array.from(document.querySelectorAll('button, div, span')).find((e) =>
      /历史记录/.test(e.textContent || '') && (e.textContent || '').length < 20)
    if (el) el.click()
  })
  drawerOpened = await waitFor(() => !!document.querySelector('.t-drawer'), 25, 400)
}
check('断言4 已上传文件抽屉可打开', drawerOpened, `opened=${drawerOpened}`)
if (drawerOpened) {
  await page.waitForTimeout(3500)
  const snap = await page.evaluate(() => {
    const drawer = document.querySelector('.t-drawer')
    const txt = drawer ? drawer.innerText : ''
    const m = txt.match(/正在执行\s*(\d+)/)
    const running = m ? Number(m[1]) : 0
    const bar = document.querySelector('.parse-status-bar')
    return { running, barVisible: !!bar && getComputedStyle(bar).display !== 'none', txt: txt.slice(0, 200) }
  })
  check('断言4 抽屉「正在执行」与状态条同源（无在途时均为 0 / 不显示）',
    snap.running === 0 && !snap.barVisible,
    `running=${snap.running} barVisible=${snap.barVisible}`)
  // 共享源持续轮询：等一个 2.5s 周期后再读一次，状态不应卡在「提取中」
  await page.waitForTimeout(3200)
  const stuck = await page.evaluate(() => {
    const d = document.querySelector('.t-drawer')
    return d ? /提取中/.test(d.innerText || '') : false
  })
  check('断言4 抽屉打开期间状态自动刷新（不卡在「提取中」）', !stuck, `stillRunning=${stuck}`)
}

check('断言4 console error 为 0', consoleErrors.length === 0, `errors=${consoleErrors.length} ${consoleErrors.slice(0, 2).join(' | ')}`)

await page.screenshot({ path: `${SHOTS}/r42-final.png`, fullPage: false })
await browser.close()

const failed = results.filter((r) => !r.ok)
console.log(`\n==== R42 SUMMARY: ${results.length - failed.length}/${results.length} PASS ====`)
if (failed.length) {
  for (const f of failed) console.log(`FAILED: ${f.name} | ${f.detail}`)
  process.exit(1)
}
