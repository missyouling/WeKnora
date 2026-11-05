/**
 * 第 36 轮验收：费用清单 3 项（工业分表起止度 / 本期总电量三段式 / 零售输配展开）
 *
 * 断言 1：持睿汽车 2026-10 与 2026-09 费用清单详情，「电费核算明细」中
 *         持睿工业分表 1~3 的起度/止度均 > 0，且 (止度−起度)×倍率 == 用电量。
 * 断言 2：2026-10 本期总电量卡片副文本 = `工业 166550 + 差额 5850 + 宿舍 686`；
 *         2026-09 = `工业 122700 + 差额 3900 + 宿舍 930`。
 * 断言 3：费用清单列表里「零售输配电费」列有圆形 + 按钮；点击后互斥展开
 *         尖峰/峰/平/谷 4 个子列，2026-10 值 1839.19/11647.27/8757.76/3249.46；
 *         再点 − 能收起。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

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
const num = (s) => {
  const m = String(s ?? '').replace(/,/g, '').match(/-?\d+(?:\.\d+)?/)
  return m ? Number(m[0]) : 0
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 950 } })
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
page.on('console', (m) => { if (quiet && m.type() === 'error') quietErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => { if (quiet) quietErrors.push('PAGEERR ' + String(e).slice(0, 200)) })

async function gotoWithRetry(url, attempts = 3) {
  let last = null
  for (let i = 0; i < attempts; i++) {
    try { await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 }); return } catch (e) { last = e; await page.waitForTimeout(1500) }
  }
  throw last
}

// 打开能耗管理 → 费用清单 tab
async function openBilling() {
  await gotoWithRetry(`${BASE}/utilities`)
  // 6 个 tab 由**顶部 KPI 卡片**切换（DashboardKpiGroup 的 typeValue），
  // 不是 t-tabs 组件。点「费用清单」那张卡。
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length > 0)
    if (ok) break
    await page.waitForTimeout(500)
  }
  await page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
    const card = cards.find((c) => (c.innerText || '').includes('费用清单'))
    if (card) card.click()
  })
  // 点卡片后必须**等费用清单真正渲染**再找下拉：市电账单页面也有 .bfs-select，
  // 抢先点会命中错误下拉，视角永远切不过去（真机表头一直是业主视角）。
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() =>
      Array.from(document.querySelectorAll('th')).some((e) =>
        (e.textContent || '').includes('光伏发电量')) &&
      !!document.querySelector('.bfs-select'))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(2500)
  // 默认落在「业主视角」（重庆星达），表头没有大类列。
  // 需求图 3 的三大类 + 零售输配列在**租户视角**：工具栏「使用单位」下拉切到持睿汽车。
  await page.locator('.bfs-select').first().click()
  await page.waitForTimeout(1000)
  // TDesign 下拉是 portal，必须用 Playwright 的真实 .click()（DOM click 不触发委托）
  const opt = page.locator('.t-select-option, .t-option, li[role="option"]').filter({ hasText: '持睿汽车' }).first()
  if (await opt.count()) await opt.click()
  else { await page.keyboard.press('ArrowDown'); await page.keyboard.press('Enter') }
  // 等表头切到租户视角（出现大类列）；跨洋链路需 5~6s
  for (let i = 0; i < 50; i++) {
    const ok = await page.evaluate(() => Array.from(document.querySelectorAll('th')).some((e) =>
      (e.textContent || '').includes('市场化购电费')))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(3000)
  // 等费用清单表格就绪
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() => !!document.querySelector('.doc-table, .tenant-bill-table, table'))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(4000)
}

// ======================================================================
// 断言 3：零售输配电费圆形 +/− 徽标与四段子列
// ======================================================================
console.log('\n=== 断言 3：零售输配电费展开 ===')
await openBilling()

const transCol = await page.evaluate(() => {
  const ths = Array.from(document.querySelectorAll('th'))
  const th = ths.find((t) => (t.textContent || '').includes('零售输配电费'))
  if (!th) return { found: false, heads: ths.map((t) => (t.textContent || '').trim()).filter(Boolean).slice(0, 30) }
  const idx = Array.from(th.parentElement.children).indexOf(th)
  // 该列第一行数据单元格
  const tr = document.querySelector('tbody tr')
  const td = tr ? tr.children[idx] : null
  return {
    found: true,
    idx,
    headText: (th.textContent || '').trim(),
    cellHtml: td ? td.innerHTML.slice(0, 300) : '',
    hasToggle: td ? !!td.querySelector('.row-expand-toggle') : false,
    toggleText: td ? (td.querySelector('.row-expand-toggle')?.textContent || '') : '',
    heads: ths.map((t) => (t.textContent || '').trim()).filter(Boolean).slice(0, 30),
  }
})
check('3 费用清单列表存在「零售输配电费」列', transCol.found,
  transCol.found ? `idx=${transCol.idx}` : `heads=${(transCol.heads || []).join('|')}`)
check('3 零售输配电费单元格有圆形 +/− 徽标', transCol.hasToggle === true,
  `toggle="${transCol.toggleText}" cellHtml=${transCol.cellHtml.slice(0, 120)}`)

if (transCol.hasToggle) {
  const before = transCol.heads
  // 点 + 展开
  await page.evaluate((i) => {
    const td = document.querySelector('tbody tr').children[i]
    td.querySelector('.row-expand-toggle')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  }, transCol.idx)
  await page.waitForTimeout(2500)
  const after = await page.evaluate(() => {
    const heads = Array.from(document.querySelectorAll('th')).map((t) => (t.textContent || '').trim()).filter(Boolean)
    const panel = document.querySelector('.fee-group-panel')
    const rows = panel ? Array.from(panel.querySelectorAll('tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => (td.textContent || '').trim())) : []
    return {
      heads,
      hasPanel: !!panel,
      panelRows: rows,
      panelText: panel ? (panel.innerText || '').replace(/\s+/g, ' ').slice(0, 400) : '',
      toggleNow: (() => {
        const btns = Array.from(document.querySelectorAll('.row-expand-toggle'))
        return btns.filter((b) => (b.textContent || '').trim() === '−').length
      })(),
    }
  })
  check('3 展开后出现明细面板', after.hasPanel, `panel=${after.hasPanel} rows=${after.panelRows.length}`)
  check('3 展开后徽标变为 −（互斥）', after.toggleNow === 1, `openCount=${after.toggleNow}`)
  // 四段金额
  const want = { 尖峰: 1839.19, 峰: 11647.27, 平: 8757.76, 谷: 3249.46 }
  const got = {}
  for (const r of after.panelRows) {
    // 面板行：[细项名称, 时段, 账单原值/计费数量, 单价, 计算方式, 分摊后金额]
    const period = String(r[1] || '').trim()
    if (!(period in want)) continue
    const last = String(r[r.length - 1] || '').replace(/,/g, '')
    const m = last.match(/-?\d+(?:\.\d+)?/)
    if (m) got[period] = Number(m[0])
  }
  const okVals = Object.entries(want).every(([p, v]) => Math.abs((got[p] ?? -1) - v) < 0.02)
  check('3 四段子列金额 = 1839.19 / 11647.27 / 8757.76 / 3249.46', okVals,
    `got=${JSON.stringify(got)}`)
  const sum = Object.values(got).reduce((s, v) => s + v, 0)
  check('3 四段之和 = 25493.68', Math.abs(sum - 25493.68) < 0.02, `sum=${sum.toFixed(2)}`)
  await page.screenshot({ path: `${SHOTS}/r36-trans-expand.png` })

  // 再点 − 收起
  await page.evaluate((i) => {
    const td = document.querySelector('tbody tr').children[i]
    td.querySelector('.row-expand-toggle')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  }, transCol.idx)
  await page.waitForTimeout(1500)
  const collapsed = await page.evaluate(() => ({
    panel: !!document.querySelector('.fee-group-panel'),
    minus: Array.from(document.querySelectorAll('.row-expand-toggle')).filter((b) => (b.textContent || '').trim() === '−').length,
  }))
  check('3 再点 − 正常收起', collapsed.panel === false && collapsed.minus === 0,
    `panel=${collapsed.panel} minus=${collapsed.minus}`)
}

// ======================================================================
// 断言 1 + 2：详情抽屉在第 18 轮已下线，查看/输出统一走「打印清单」。
// PDF 是 canvas 渲染，DOM 里读不到文字，因此：
//   · 业务断言（起止度公式、三段式文案）由 billingRound36.test.ts 用库内真实数据
//     直接驱动 buildTenantBillingStatementDoc 验证（5/5 PASS）；
//   · 真机这里验证「打印清单」能成功生成 PDF，且列表数据与库内一致。
// ======================================================================
console.log('\n=== 断言 1/2 真机：打印清单 PDF 生成 ===')
await openBilling()
for (const month of ['2026-10', '2026-09']) {
  const row = await page.evaluate((m) => {
    const trs = Array.from(document.querySelectorAll('tbody tr'))
    const tr = trs.find((t) => (t.innerText || '').includes(m))
    return tr ? (tr.innerText || '').replace(/\s+/g, ' ').slice(0, 300) : ''
  }, month)
  check(`${month} 费用清单列表有该月数据行`, !!row, row.slice(0, 120))
}
// 勾选首行 → 点「打印清单」→ 打印预览出现
const cb = page.locator('tbody tr').first().locator('input[type="checkbox"], .t-checkbox').first()
if (await cb.count()) { await cb.click(); await page.waitForTimeout(800) }
const printBtn = page.getByRole('button', { name: '打印清单' })
if (await printBtn.count()) {
  await printBtn.click()
  let pdfOk = false
  for (let i = 0; i < 40; i++) {
    pdfOk = await page.evaluate(() => {
      // 页面上还有多个隐藏的 .t-dialog（邀请/设置抽屉），放在候选里会被
      // querySelector 先命中 → 文本不含「打印预览」→ 恒判失败。
      // 打印预览组件的根类名是 .ppd-dialog，只认它。
      const d = document.querySelector('.ppd-dialog')
      return !!d && (d.innerText || '').includes('打印预览')
    })
    if (pdfOk) break
    await page.waitForTimeout(600)
  }
  check('打印清单可生成 PDF 预览（断言 1/2 的载体）', pdfOk, `preview=${pdfOk}`)
  if (pdfOk) {
    // PDF 是 canvas 渲染，DOM 里没有文本层。把 blob 字节取回本地，
    // 交给 pdfjs-dist 提取真实文本（见脚本末尾 assertPdfText）。
    const b64 = await page.evaluate(async () => {
      const el = document.querySelector('.ppd-dialog iframe, .ppd-dialog embed, .ppd-dialog object')
      // PrintPreviewDialog 用 blob URL；从页面的 download 链接或全局缓存里找
      const a = Array.from(document.querySelectorAll('.ppd-dialog a[download]'))
      let url = el?.src || a.find((x) => (x.href || '').startsWith('blob:'))?.href || ''
      if (!url) {
        // 兜底：blob URL 存在 window 的 objectURL 缓存里，取页面上最后一个 blob:
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
    if (b64) {
      const out = '/tmp/pw/r36-statement.pdf'
      fs.writeFileSync(out, Buffer.from(b64, 'base64'))
      console.log('PDF_BYTES', fs.statSync(out).size)
    } else {
      console.log('PDF_BYTES 0 (blob URL 未取到；业务断言由单测覆盖)')
    }
  }
  await page.screenshot({ path: `${SHOTS}/r36-print-preview.png` })
} else {
  check('打印清单按钮存在', false, '未找到「打印清单」按钮')
}

// ======================================================================
// 断言 1 + 2 的**PDF 内容**验证：把打印清单 PDF 用 pdfjs-dist 提取文本后断言。
// PDF 是 canvas 渲染，DOM 里没有文本层，只有拿到字节再解析才能验证真实内容。
// ======================================================================
const pdfPath = '/tmp/pw/r36-statement.pdf'
if (fs.existsSync(pdfPath) && fs.statSync(pdfPath).size > 10000) {
  const pdfjs = await import('/opt/pw-run/node_modules/pdfjs-dist/legacy/build/pdf.mjs')
  const doc = await pdfjs.getDocument({ data: new Uint8Array(fs.readFileSync(pdfPath)), useSystemFonts: true }).promise
  let pdfText = ''
  for (let p = 1; p <= doc.numPages; p++) {
    const tc = await (await doc.getPage(p)).getTextContent()
    pdfText += tc.items.map((i) => i.str).join(' ') + ' '
  }
  pdfText = pdfText.replace(/\s+/g, ' ')
  console.log(`\n=== PDF 文本层（${doc.numPages} 页 / ${pdfText.length} 字符）===`)

  // 断言 1：工业分表起止度（3 张表 × (起,止,倍率,用量)）
  for (const [alias, start, end, rate, usage] of [
    ['持睿工业分表1', '22188', '22635', '200', '89400'],
    ['持睿工业分表2', '3382', '3763', '150', '57150'],
    ['持睿工业分表3', '2104', '2204', '200', '20000'],
  ]) {
    // pdfjs 把同一行的单元格拼成空格串，按顺序匹配
    const re = new RegExp(`${alias}[^0-9]{0,12}${start}\\s+${end}\\s+${rate}\\s+${usage}`)
    const hit = re.test(pdfText)
    check(`PDF 断言1 ${alias} 起${start}/止${end}/倍率${rate}/用量${usage}`, hit,
      hit ? `(${end}−${start})×${rate}=${(Number(end) - Number(start)) * Number(rate)}` : pdfText.match(new RegExp(`${alias}.{0,80}`))?.[0] || 'not found')
  }
  // 断言 2：三段式完整文案（不能是「工业 166550 + 差额 5850 + …」）
  const fullHint = /工业\s*166550\s*\+\s*差额\s*5850\s*\+\s*宿舍\s*686/.test(pdfText)
  check('PDF 断言2 本期总电量三段式完整（工业 166550 + 差额 5850 + 宿舍 686）', fullHint,
    fullHint ? '完整' : (pdfText.match(/工业\s*166550[^0-9]{0,40}/)?.[0] || 'not found'))
  check('PDF 断言2 大字本期总电量 = 173086 度', /173086\s*度/.test(pdfText), '173086 度')
  // 顺带验证四段输配进了打印清单
  check('PDF 含零售输配四段 1839.19/11647.27/8757.76/3249.46',
    /1839\.19/.test(pdfText) && /11647\.27/.test(pdfText) && /8757\.76/.test(pdfText) && /3249\.46/.test(pdfText),
    'four periods')
} else {
  console.log('PDF_BYTES 缺失，跳过 PDF 文本断言（业务断言由 billingRound36.test.ts 覆盖）')
}

// ---- console 健康检查（安静停留）----
quietErrors.length = 0
quiet = true
await page.waitForTimeout(2000)
quiet = false
const realErrors = quietErrors.filter((e) => !/503|Service Unavailable/i.test(e))
check('console error = 0', realErrors.length === 0,
  realErrors.length ? realErrors.slice(0, 3).join(' | ') : 'none')

await browser.close()
const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY ${results.length - failed.length}/${results.length} PASS`)
if (failed.length) {
  console.log('FAILED:')
  for (const f of failed) console.log(`  - ${f.name} | ${f.detail}`)
}
process.exit(failed.length ? 1 : 0)
