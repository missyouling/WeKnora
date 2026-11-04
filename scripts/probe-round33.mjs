// 第 33 轮真机验收：能耗管理（市电账单）4 条断言
//
//  断言 1  2026-10 电量明细：线损/加减/计费电量逐行正确，底部合计 614,578
//  断言 2  输配容（需）量 / 功率因素调整两张子表有数据且数值正确
//  断言 3  账单概况：本期电量/电费卡片、计费数量、用能分析四段
//  断言 4  双列独立：清空 pf_detail 后 电费=0.00、账单电费=-3,394.92、状态=异常
//          （由环境变量 PF_MUTATED=1 触发；正常数据下断言状态为正常）
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
const SHOTS = '/tmp/pw/shots'
const MUTATED = process.env.PF_MUTATED === '1'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
const consoleErrors = []
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}
// 取字符串里**第一个**数字（卡片文本形如「本期电量 623,932千瓦时 ↑+27,764 千瓦时」，
// 直接剥掉非数字会把环比的 27,764 拼进来）
const num = (s) => {
  const m = String(s ?? '').match(/-?[\d,]+(?:\.\d+)?/)
  return m ? Number(m[0].replace(/,/g, '')) : 0
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  const hide = () => {
    if (!document.head) return
    const st = document.createElement('style')
    st.textContent = '.guide{display:none!important}'
    document.head.appendChild(st)
  }
  if (document.head) hide(); else document.addEventListener('DOMContentLoaded', hide)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR ' + String(e).slice(0, 200)))

await page.goto(`${BASE}/utilities`, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(3000)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))
// 等列表出现（17 份账单，首屏约 15s）
for (let i = 0; i < 60; i++) {
  const n = await page.evaluate(() => document.querySelectorAll('.doc-table tbody tr').length)
  if (n > 0) break
  await page.waitForTimeout(1000)
}
const rows = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.doc-table tbody tr')).map((tr) => tr.innerText.replace(/\s+/g, ' ')))
check('列表已加载 17 份市电账单', rows.length >= 17, `rows=${rows.length}`)
check('列表不再出现「解析中/待提取」占位行', !rows.some((r) => /解析中|待提取|提取中/.test(r)),
  `hit=${rows.filter((r) => /解析中|待提取|提取中/.test(r)).slice(0, 2).join(' | ') || 'none'}`)

const idx = rows.findIndex((r) => r.includes('2026-10'))
check('定位 2026-10 账单', idx >= 0, `idx=${idx}`)
await page.locator('.doc-table tbody tr').nth(idx).click()
await page.waitForTimeout(3500)

async function clickMenu(label) {
  await page.evaluate((l) => {
    const el = Array.from(document.querySelectorAll('.bill-nav-item'))
      .find((e) => e.textContent.trim() === l)
    if (el) el.click()
  }, label)
  await page.waitForTimeout(800)
}

// ---- 断言 3：概况（卡片 + 计费数量 + 用能分析）----
const overview = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.os-row')).map((r) => r.innerText.replace(/\s+/g, ' ').trim())
  const metrics = Array.from(document.querySelectorAll('.metric-card')).map((c) => c.innerText.replace(/\s+/g, ' '))
  const ea = Array.from(document.querySelectorAll('.ea-compare-row')).map((r) => r.innerText.replace(/\s+/g, ' ').trim())
  return { rows, metrics, ea }
})
const metricKwh = overview.metrics.find((m) => m.startsWith('本期电量')) || ''
const metricFee = overview.metrics.find((m) => m.startsWith('本期电费')) || ''
check('3 顶部卡片 本期电量 = 623,932', num(metricKwh) === 623932, metricKwh)
check('3 顶部卡片 本期电费 = 481,647.91', Math.abs(num(metricFee) - 481647.91) < 0.01, metricFee)

const rowOf = (label) => overview.rows.find((r) => r.startsWith(label)) || ''
const qtyOf = (label) => {
  const r = rowOf(label)
  const cells = r.split(' ')
  return num(cells[1])
}
check('3 市场化购电费 计费数量 = 614,578', qtyOf('市场化购电费') === 614578, rowOf('市场化购电费'))
check('3 输配电量电费 计费数量 = 614,578', qtyOf('输配电量电费') === 614578, rowOf('输配电量电费'))
check('3 上网环节线损费 计费数量 = 614,578', qtyOf('上网环节线损费') === 614578, rowOf('上网环节线损费'))
check('3 居民目录电费 计费数量 = 9,354', qtyOf('目录电费（居民）') === 9354, rowOf('目录电费（居民）'))
check('3 容需量 计费数量 = 2,000', qtyOf('输配容（需）量电费') === 2000, rowOf('输配容（需）量电费'))

const eaVals = overview.ea.slice(1).map((r) => r.split(' '))
const eaMap = Object.fromEntries(eaVals.map((c) => [c[0], { cur: num(c[1]), pct: num(c[2]) }]))
check('3 用能分析四段电量 = 34,105 / 133,236 / 196,797 / 250,440',
  eaMap['尖']?.cur === 34105 && eaMap['峰']?.cur === 133236 && eaMap['平']?.cur === 196797 && eaMap['谷']?.cur === 250440,
  JSON.stringify(eaMap))
check('3 用能分析四段占比 = 5.55 / 21.68 / 32.02 / 40.75',
  Math.abs((eaMap['尖']?.pct ?? 0) - 5.55) < 0.01 && Math.abs((eaMap['峰']?.pct ?? 0) - 21.68) < 0.01 &&
  Math.abs((eaMap['平']?.pct ?? 0) - 32.02) < 0.01 && Math.abs((eaMap['谷']?.pct ?? 0) - 40.75) < 0.01,
  JSON.stringify(eaMap))

// ---- 断言 4：双列独立与对比状态（正常数据 / 人为清空）----
const pfRow = rowOf('功率因数调整电费')
const pfCells = pfRow.split(' ')
if (MUTATED) {
  check('4 清空力调明细后：电费=0.00 且 账单电费=-3,394.92 且状态=异常',
    Math.abs(num(pfCells[2])) < 0.001 && Math.abs(num(pfCells[3]) + 3394.92) < 0.01 && pfRow.includes('异常'),
    pfRow)
} else {
  check('4 正常数据下力调行对比状态=正常',
    Math.abs(num(pfCells[2]) + 3394.92) < 0.01 && Math.abs(num(pfCells[3]) + 3394.92) < 0.01 && pfRow.includes('正常'),
    pfRow)
  const capCells = rowOf('输配容（需）量电费').split(' ')
  check('4 正常数据下容需量行对比状态=正常',
    num(capCells[1]) === 2000 && Math.abs(num(capCells[2]) - 44000) < 0.01 && capCells.includes('正常'),
    rowOf('输配容（需）量电费'))
}
await page.screenshot({ path: `${SHOTS}/r33-overview.png` })

// ---- 断言 1：电量明细 ----
await clickMenu('电量明细')
const meter = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.meter-row')).map((r) => r.innerText.replace(/\s+/g, ' ').trim()))
const meterRow = (t) => meter.find((r) => r.startsWith(t)) || ''
const cells = (t) => meterRow(t).split(' ')
const byType = {
  deep: cells('正向有功（尖峰）'),
  peak: cells('正向有功（峰）'),
  flat: cells('正向有功（平）'),
  valley: cells('正向有功（谷）'),
}
check('1 尖峰 线损18 / 加减-1403 / 计费34,105',
  num(byType.deep[6]) === 18 && num(byType.deep[7]) === -1403 && num(byType.deep[8]) === 34105, meterRow('正向有功（尖峰）'))
check('1 峰 线损70 / 加减-3274 / 计费133,236',
  num(byType.peak[6]) === 70 && num(byType.peak[7]) === -3274 && num(byType.peak[8]) === 133236, meterRow('正向有功（峰）'))
check('1 平 线损103 / 加减-2866 / 计费196,797',
  num(byType.flat[6]) === 103 && num(byType.flat[7]) === -2866 && num(byType.flat[8]) === 196797, meterRow('正向有功（平）'))
check('1 谷 线损131 / 加减-1871 / 计费250,440',
  num(byType.valley[6]) === 131 && num(byType.valley[7]) === -1871 && num(byType.valley[8]) === 250440, meterRow('正向有功（谷）'))
const totalRow = meter.find((r) => r.startsWith('合计')) || ''
check('1 电量明细合计 = 614,578（不是 1,401,690）', num(totalRow) === 614578, totalRow)
await page.screenshot({ path: `${SHOTS}/r33-meter.png` })

// ---- 断言 2：容需量 / 力调 ----
await clickMenu('输配容（需）量电费')
const capText = await page.evaluate(() => document.querySelector('.bill-content')?.innerText.replace(/\s+/g, ' ') || '')
check('2 容需量：容量2000 / 电价22 / 电费44000',
  /容量 2,000 22 44,000/.test(capText) || (/2,000/.test(capText) && /44,000/.test(capText)),
  capText.slice(0, 200))
await page.screenshot({ path: `${SHOTS}/r33-capacity.png` })

await clickMenu('功率因素调整电费')
const pfText = await page.evaluate(() => document.querySelector('.bill-content')?.innerText.replace(/\s+/g, ' ') || '')
// 人为清空明细的场景下子表就是「暂无数据」，该断言只在正常数据下执行
if (!MUTATED) {
  check('2 力调：0.97 / -0.0075 / -3,394.92',
    pfText.includes('0.97') && pfText.includes('-0.0075') && pfText.includes('-3,394.92'),
    pfText.slice(0, 220))
}
await page.screenshot({ path: `${SHOTS}/r33-pf.png` })

// ---- 2026-09 回归：卡片与概况（另一份完整账单）----
await page.evaluate(() => {
  const btn = Array.from(document.querySelectorAll('.bill-detail-head button'))
    .find((b) => (b.textContent || '').includes('账单列表'))
  if (btn) btn.click()
})
await page.waitForTimeout(2500)
const rows2 = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.doc-table tbody tr')).map((tr) => tr.innerText.replace(/\s+/g, ' ')))
const idx9 = rows2.findIndex((r) => r.includes('2026-09'))
if (idx9 >= 0) {
  await page.locator('.doc-table tbody tr').nth(idx9).click()
  await page.waitForTimeout(3000)
  const m9 = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.metric-card')).map((c) => c.innerText.replace(/\s+/g, ' ')))
  const k9 = m9.find((m) => m.startsWith('本期电量')) || ''
  const f9 = m9.find((m) => m.startsWith('本期电费')) || ''
  check('3 2026-09 本期电量 = 596,168', num(k9) === 596168, k9)
  check('3 2026-09 本期电费 = 433,433.07', Math.abs(num(f9) - 433433.07) < 0.01, f9)
  await page.screenshot({ path: `${SHOTS}/r33-sep.png` })
} else {
  check('3 2026-09 账单可打开', false, 'row not found')
}

const failed = results.filter((r) => !r.ok)
const errs = consoleErrors.filter((e) => !e.includes('favicon'))
console.log(`\nSUMMARY ${results.length - failed.length}/${results.length} PASS`)
if (failed.length) console.log('FAILED: ' + failed.map((f) => f.name).join(' ; '))
console.log(`CONSOLE_ERRORS ${errs.length}`)
if (errs.length) console.log(errs.slice(0, 5).join('\n'))

await browser.close()
process.exit(failed.length ? 1 : 0)
