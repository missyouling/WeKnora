/**
 * 第 41 轮验收：打印预览删除 6 条独立小计行、大类小计内嵌「费用类别」列、
 * 紧凑排版让电费分项细项整表（含合计行）完整落在第 1 页。
 *
 * 断言 1：PDF 文本中 6 条独立小计行全部消失；
 *         （小计 88278.66 / 25493.68 / 784.43 / 8222.45）内嵌正确。
 * 断言 2：Page 1 同时含表头、5 个大类、基本电费、力调、合计；
 *         Page 2 不再有分项细项残余，从电费核算明细开始。
 * 断言 3：第 1 / 2 页高清截图视觉核验。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'
import { createCanvas } from '/opt/pw-run/node_modules/@napi-rs/canvas/index.js'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
const quietErrors = []
let quiet = false
function check(name, ok, detail) {
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

async function openCostList() {
  await gotoRetry(`${BASE}/utilities`)
  for (let i = 0; i < 60; i++) {
    if (await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length > 0)) break
    await page.waitForTimeout(500)
  }
  await page.evaluate(() => {
    const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('费用清单'))
    if (c) c.click()
  })
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() =>
      Array.from(document.querySelectorAll('th')).some((e) => (e.textContent || '').includes('光伏发电量')) &&
      !!document.querySelector('.bfs-select'))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(2000)
  await page.locator('.bfs-select').first().click()
  await page.waitForTimeout(900)
  const opt = page.locator('.t-select-option, .t-option, li[role="option"]').filter({ hasText: '持睿汽车' }).first()
  if (await opt.count()) await opt.click()
  for (let i = 0; i < 50; i++) {
    const ok = await page.evaluate(() => Array.from(document.querySelectorAll('th'))
      .some((e) => (e.textContent || '').includes('市场化购电费')))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(3000)
}

/** 勾选指定月份行 → 打印清单 → 取回 PDF 字节。 */
async function makeStatementPdf(month, outPath) {
  await openCostList()
  await waitRows(1, 60)
  await page.waitForTimeout(1500)
  // 找到目标月份行，勾它的复选框（不是第一行）
  const picked = await page.evaluate((m) => {
    const trs = Array.from(document.querySelectorAll('tbody tr'))
      .filter((tr) => tr.querySelectorAll('td').length > 3)
    const tr = trs.find((t) => (t.innerText || '').includes(m))
    if (!tr) return false
    const cb = tr.querySelector('input[type="checkbox"], .t-checkbox')
    if (cb) cb.click()
    return !!cb
  }, month)
  if (!picked) return 0
  await page.waitForTimeout(1200)
  for (let i = 0; i < 20; i++) {
    if (await page.evaluate(() => !!document.querySelector('.doc-batch-bar-fixed'))) break
    await page.waitForTimeout(500)
  }
  const printBtn = page.getByRole('button', { name: '打印清单' })
  if (!(await printBtn.count())) return 0
  await printBtn.click()
  let pdfOk = false
  for (let i = 0; i < 40; i++) {
    pdfOk = await page.evaluate(() => {
      const d = document.querySelector('.ppd-dialog')
      return !!d && (d.innerText || '').includes('打印预览')
    })
    if (pdfOk) break
    await page.waitForTimeout(600)
  }
  if (!pdfOk) return 0
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

/** 逐页提取 PDF 文本（按 Y 聚成行，避免字符乱序）。 */
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
  return pages
}

/** 断言 3：把 PDF 第 1 / 2 页渲染成真正的整页高清 PNG（2× 缩放）。
 *  打印预览弹窗是 canvas 逐页绘制 + 滚动容器，直接 page.screenshot 只能截到
 *  视口里的上半页；用 pdfjs + @napi-rs/canvas 离屏渲染才能核验整页排版。 */
async function renderPages(path, month) {
  const pdfjs = await import('/opt/pw-run/node_modules/pdfjs-dist/legacy/build/pdf.mjs')
  const doc = await pdfjs.getDocument({ data: new Uint8Array(fs.readFileSync(path)), useSystemFonts: true }).promise
  const made = []
  for (let p = 1; p <= Math.min(2, doc.numPages); p++) {
    const pg = await doc.getPage(p)
    const vp = pg.getViewport({ scale: 2 })
    const canvas = createCanvas(Math.ceil(vp.width), Math.ceil(vp.height))
    const c = canvas.getContext('2d')
    c.fillStyle = '#ffffff'
    c.fillRect(0, 0, canvas.width, canvas.height)
    await pg.render({ canvasContext: c, viewport: vp, canvas }).promise
    const out = `${SHOTS}/r41-${month}-p${p}.png`
    fs.writeFileSync(out, canvas.toBuffer('image/png'))
    made.push({ out, w: canvas.width, h: canvas.height })
  }
  return made
}

// ======================================================================
// 断言 1：无独立小计行 & 内嵌小计正确（2026-10 / 2026-09）
// ======================================================================
const DELETED = [
  '零售交易电费（小计）', '市场化购电费（小计）', '零售输配电费（小计）',
  '输配电量电费（小计）', '系统运行费用（小计）', '政府性基金及附加（小计）',
]
const EMBEDDED = {
  '2026-10': ['88278.66', '25493.68', '784.43', '8222.45'],
  '2026-09': ['58832.60', '18610.64', '6949.58', '6038.07'],
}
const TOTALS = { '2026-10': '136521.10', '2026-09': '101346.55' }

for (const month of ['2026-10', '2026-09']) {
  console.log(`\n=== 断言 1/2：${month} ===`)
  const pdfPath = `/tmp/pw/r41-${month}.pdf`
  const size = await makeStatementPdf(month, pdfPath)
  check(`1/2 [${month}] 打印清单生成 PDF`, size > 10000, `bytes=${size}`)
  if (size <= 10000) continue
  const pages = await pdfPages(pdfPath)
  const all = pages.join('\n').replace(/\s+/g, ' ')
  console.log(`PDF_PAGES ${month} = ${pages.length}`)

  for (const name of DELETED) {
    check(`1 [${month}] 不存在独立小计行「${name}」`, !all.includes(name), `found=${all.includes(name)}`)
  }
  const p1 = pages[0].replace(/\s+/g, ' ')
  for (const amount of EMBEDDED[month]) {
    check(`1 [${month}] Page1 费用类别列内嵌（小计 ${amount}）`,
      p1.includes(`（小计 ${amount}）`), (p1.match(new RegExp(`（小计 ${amount.replace('.', '\\.')}）.{0,20}`)) || ['not found'])[0])
  }

  // ---- 断言 2：Page 1 完整容纳分项表 ----
  for (const keyword of [
    '电费分项细项', '费用类别', '费用组成',
    '(1) 市场化购电费', '(2) 上网环节线损费', '(3) 输配电量电费',
    '(4) 系统运行费用', '(6) 政府性基金及附加',
    '基本电费', '功率因', TOTALS[month],
  ]) {
    check(`2 [${month}] Page1 含「${keyword}」`, p1.includes(keyword),
      p1.includes(keyword) ? 'ok' : (p1.match(new RegExp(keyword.slice(0, 6) + '.{0,40}')) || ['not found'])[0])
  }
  // Page 2 不得有分项表残余，且从电费核算明细开始。
  // 注意「费用组成」四个字也出现在第 3 页的页脚说明「按账单费用组成逐项列示」，
  // 那是文档备注不是表格行 —— 判据必须用分项表专属的表头/细项，不能用泛词。
  const rest = pages.slice(1).join('\n').replace(/\s+/g, ' ')
  for (const name of ['零售交易电费', '零售输配电费', '市场化购电费', '政府性基金及附加',
    '电费分项细项', '费用类别', '单价/标准', '（小计 ']) {
    check(`2 [${month}] Page2+ 无分项表残余「${name}」`, !rest.includes(name), `found=${rest.includes(name)}`)
  }
  check(`2 [${month}] Page2 从「电费核算明细」开始`, /电费核算明细/.test(pages[1] || ''),
    (pages[1] || '').slice(0, 160).replace(/\s+/g, ' '))

  // 断言 3：离屏渲染第 1 / 2 页高清整页图（弹窗截图只做过程留档）
  const shots = await renderPages(pdfPath, month)
  check(`3 [${month}] 第 1 / 2 页整页高清图已生成`,
    shots.length === 2 && shots.every((x) => x.w >= 1100 && x.h >= 1600),
    shots.map((x) => `${x.out.split('/').pop()} ${x.w}x${x.h}`).join(' | '))
  if (month === '2026-10') {
    await page.waitForTimeout(3500)
    await page.screenshot({ path: `${SHOTS}/r41-dialog.png` })
  }
}

quiet = false
const pass = results.filter((r) => r.ok).length
console.log(`\nSUMMARY ${pass}/${results.length} PASS`)
console.log(`CONSOLE_ERRORS ${quietErrors.length}`)
if (quietErrors.length) console.log(quietErrors.slice(0, 5).join('\n'))
await browser.close()
process.exit(pass === results.length ? 0 : 1)
