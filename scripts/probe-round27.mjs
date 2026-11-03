// 第 27 轮真机验收：多页发票明细串页 + 税率错标。
//
// 断言：
//  A. 搜索 26507903331100005079，行内金额 198.09 / 税率 9% / 税额 17.83；
//  B. 打开详情抽屉，项目明细首行为 198.09 / 9% / 17.83，页面内不出现 6.45；
//     （旧实现：抽屉显示 P14 的 6.45 / 3% / 0.19）
//  C. 悬停税率单元格的明细浮窗同样是 198.09 / 9% / 17.83，不出现 6.45；
//  D. 筛 车牌号=渝C81022 + 税率=3%：P2 不在结果里，可见行「合计金额 × 3%」
//     与「合计税额」误差 <= 0.05。
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs/invoices'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}

function num(text) {
  const m = String(text || '').replace(/[,\s¥￥]/g, '').match(/-?\d+(\.\d+)?/)
  return m ? Number(m[0]) : NaN
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  // 新手引导是全屏遮罩，会吞掉所有 pointer 事件（含重载后再次挂载）。
  // 用注入样式永久隐藏，比「点之前删一次」可靠。
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
page.on('console', (m) => { if (m.type() === 'error') console.log('CONSOLE_ERR', m.text().slice(0, 200)) })

await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(3000)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

// 等首屏真实数据行（TDesign 空表也会渲染一行占位，必须数 td>3 的行）
async function waitRows(min = 1, timeout = 90000) {
  const start = Date.now()
  while (Date.now() - start < timeout) {
    const n = await page.evaluate(() =>
      Array.from(document.querySelectorAll('.doc-table tbody tr'))
        .filter((tr) => tr.querySelectorAll('td').length > 3).length)
    if (n >= min) return n
    await page.waitForTimeout(1000)
  }
  return 0
}

const firstRows = await waitRows(1)
check('列表首屏渲染', firstRows > 0, `rows=${firstRows}`)
await page.screenshot({ path: `${SHOTS}/r27-01-list.png` })

// ---- A. 搜索票号 ----
const search = page.locator('.bts-search input, input[placeholder="搜索全部字段"]').first()
await search.click()
await search.fill('26507903331100005079')
await page.waitForTimeout(2500)
const rowsA = await waitRows(1, 30000)
const rowA = await page.evaluate(() => {
  const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
  const idxOf = (name) => head.findIndex((h) => h.replace(/\s/g, '').includes(name))
  const tr = Array.from(document.querySelectorAll('.doc-table tbody tr'))
    .find((r) => r.querySelectorAll('td').length > 3)
  if (!tr) return null
  const cells = Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim())
  const at = (name) => (idxOf(name) >= 0 ? cells[idxOf(name)] || '' : '')
  return {
    no: tr.querySelector('.row-invoice-no')?.textContent?.trim() || '',
    amount: at('金额'),
    rate: at('税率'),
    tax: at('税额'),
    head,
    cells,
  }
})
check('A1 搜索命中单张票', rowsA === 1 && !!rowA, `rows=${rowsA} no=${rowA?.no}`)
check('A2 行金额 198.09', rowA && Math.abs(num(rowA.amount) - 198.09) < 0.005, `amount=${rowA?.amount}`)
check('A3 行税率 9%', rowA && /9\s*%/.test(rowA.rate), `rate=${rowA?.rate}`)
const taxA = rowA ? num(rowA.cells.find((c) => /^1?7\.83$/.test(c.replace(/[¥￥,\s]/g, '')))) : NaN
check('A4 行税额 17.83', rowA && Math.abs(num(rowA.tax) - 17.83) < 0.005, `tax=${rowA?.tax} head=${rowA?.head?.join('/')}`)
await page.screenshot({ path: `${SHOTS}/r27-02-searched.png` })

// ---- B. 详情抽屉的项目明细 ----
const rowSel = page.locator('.doc-table tbody tr').filter({ has: page.locator('.row-invoice-no') }).first()
await rowSel.click()
await page.waitForTimeout(2500)
const drawer = await page.evaluate(() => {
  const panes = Array.from(document.querySelectorAll('.bsd-drawer .t-drawer__content-wrapper'))
    .filter((el) => el.getBoundingClientRect().width > 0)
  if (!panes.length) return null
  const pane = panes[0]
  const table = pane.querySelector('.bsd-items-table')
  const rows = table
    ? Array.from(table.querySelectorAll('tbody tr')).map((r) =>
        Array.from(r.querySelectorAll('td')).map((td) => td.textContent.trim()))
    : []
  const foot = table
    ? Array.from(table.querySelectorAll('tfoot td')).map((td) => td.textContent.trim())
    : []
  return { rows, foot, text: (table ? table.innerText : pane.innerText).slice(0, 4000) }
})
check('B1 详情抽屉打开', !!drawer, drawer ? `rows=${drawer.rows.length}` : 'not open')
if (drawer) {
  const flat = drawer.rows.flat().join(' ')
  check('B2 明细首行金额 198.09', flat.includes('198.09'), `items=${JSON.stringify(drawer.rows)}`)
  check('B3 明细税率 9', /(^|\s)9(\s|$)/.test(flat) || flat.includes('9%'), `items=${JSON.stringify(drawer.rows)}`)
  check('B4 明细税额 17.83', flat.includes('17.83'), `items=${JSON.stringify(drawer.rows)}`)
  check('B5 抽屉不出现串页值 6.45', !drawer.text.includes('6.45'), 'drawer text contains 6.45?')
}
await page.screenshot({ path: `${SHOTS}/r27-03-drawer.png` })

// 关抽屉
// 关抽屉：TDesign 的 mask 会一直截获指针事件，直接重载页面最稳。
await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(2500)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))
await waitRows(1, 60000)
const search2 = page.locator('.bts-search input, input[placeholder="搜索全部字段"]').first()
await search2.click()
await search2.fill('26507903331100005079')
await page.waitForTimeout(2500)
await waitRows(1, 30000)

// ---- C. 悬停浮窗 ----
await page.locator('.row-rate').first().hover()
await page.waitForTimeout(1200)
const pop = await page.evaluate(() => {
  const t = document.querySelector('.biz-sub-table')
  return t ? t.innerText : ''
})
check('C1 浮窗出现', !!pop, pop ? pop.replace(/\n/g, ' / ').slice(0, 120) : 'no popover')
check('C2 浮窗金额 198.09', pop.includes('198.09'), pop.slice(0, 200))
check('C3 浮窗税额 17.83', pop.includes('17.83'), pop.slice(0, 200))
check('C4 浮窗不含 6.45', !pop.includes('6.45'), 'popover contains 6.45?')
await page.screenshot({ path: `${SHOTS}/r27-04-popover.png` })

// ---- D. 车牌 + 3% 过滤 ----
await search.fill('')
await page.keyboard.press('Enter')
await page.waitForTimeout(2000)
await search.click()
await search.fill('渝C81022')
await page.waitForTimeout(2500)
// 税率下拉：TDesign select 输入框 placeholder=税率筛选
await page.locator('.bfs-select').first().click()
await page.waitForTimeout(800)
await page.locator('.t-select__dropdown li, .t-select-option').filter({ hasText: '3%' }).first().click()
await page.waitForTimeout(2500)
await waitRows(1, 30000)
const rowsD = await page.evaluate(() => {
  const trs = Array.from(document.querySelectorAll('.doc-table tbody tr'))
    .filter((tr) => tr.querySelectorAll('td').length > 3)
  const rows = trs.map((tr) => {
    const cells = Array.from(tr.querySelectorAll('td'))
    return {
      no: tr.querySelector('.row-invoice-no')?.textContent?.trim() || '',
      amount: tr.querySelector('.row-amount')?.textContent?.trim() || '',
      rate: tr.querySelector('.row-rate')?.textContent?.trim() || '',
      cells: cells.map((c) => c.textContent.trim()),
    }
  })
  const foot = Array.from(document.querySelectorAll('.doc-table tfoot td, .t-table__footer td'))
    .map((td) => td.textContent.trim())
  const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
  return { rows, foot, head }
})
const stillHasP2 = rowsD.rows.some((r) => r.no.includes('26507903331100005079'))
check('D1 3% 结果不含 P2', !stillHasP2, `rows=${rowsD.rows.length}`)
const ratesD = rowsD.rows.map((r) => r.rate)
check('D2 可见行税率全为 3%', ratesD.every((r) => /3\s*%/.test(r)), ratesD.join(','))
const headIdx = (name) => rowsD.head.findIndex((h) => h.replace(/\s/g, '').includes(name))
const amountIdx = headIdx('金额')
const taxIdx = headIdx('税额')
const sumAmount = rowsD.rows.reduce((s, r) => s + (Number.isFinite(num(r.cells[amountIdx])) ? num(r.cells[amountIdx]) : 0), 0)
const sumTax = rowsD.rows.reduce((s, r) => s + (Number.isFinite(num(r.cells[taxIdx])) ? num(r.cells[taxIdx]) : 0), 0)
const diff = Math.abs(sumAmount * 0.03 - sumTax)
check('D3 可见行金额×3%≈税额（≤0.05）', diff <= 0.05, `sumAmount=${sumAmount.toFixed(2)} sumTax=${sumTax.toFixed(2)} diff=${diff.toFixed(3)}`)
console.log('FOOT', JSON.stringify(rowsD.foot))
await page.screenshot({ path: `${SHOTS}/r27-05-filter3pct.png` })

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY pass=${results.length - failed.length}/${results.length}`)
if (failed.length) console.log('FAILED:', failed.map((f) => f.name).join(' | '))
await browser.close()
process.exit(failed.length ? 1 : 0)
