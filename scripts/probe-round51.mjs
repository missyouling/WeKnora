/**
 * 第 51 轮真机验收。
 *
 * 断言 1：费用清单（重庆星达）表头在 本期电费 与 光伏发电量 之间紧挨渲染
 *         本期水量 / 本期水费。
 * 断言 2：2026-10 行 本期水量=769、本期水费=4014.18，底部合计行准确累加。
 * 断言 3：设置里关闭 本期水量 默认表头并保存 → 表头立即隐藏；
 *         打开保存（或字段筛选器重置）→ 立即恢复在 本期电费 与 本期水费 之间。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
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
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 } })
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
async function headerTexts() {
  return page.locator('.t-table__header th').evaluateAll((els) =>
    els.map((e) => (e.innerText || '').trim()).filter(Boolean))
}
/**
 * 等六同步真正就绪。
 *
 * 页面首帧会在 loadCategorySubs 完成前回落到静态列（其中 本期电量/本期电费
 * 默认 false），此时读表头会误判成「列丢了」。数据行也是异步 buildRows 产物，
 * 必须一起等，否则 2026-10 行与合计行都还不在 DOM 里。
 */
async function waitForReady(timeoutMs = 45000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const ready = await page.evaluate(() => {
      const h = Array.from(document.querySelectorAll('th')).map((e) => (e.innerText || '').trim())
      const hasCol = h.includes('本期电费') && h.includes('本期水量') && h.includes('本期水费')
      const hasRow = Array.from(document.querySelectorAll('tbody tr'))
        .some((r) => (r.innerText || '').includes('2026-10'))
      return hasCol && hasRow
    })
    if (ready) return true
    await page.waitForTimeout(800)
  }
  return false
}
async function gotoBilling() {
  await page.goto(`${BASE}/utilities`, { waitUntil: 'domcontentloaded', timeout: 60000 })
  await page.waitForTimeout(15000)
  await stripGuide()
  // 顶部 KPI 卡片切到「费用清单」
  for (let i = 0; i < 60; i++) {
    const n = await page.locator('.dash-kpi-card').count()
    if (n > 0) break
    await page.waitForTimeout(500)
  }
  await page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
    const card = cards.find((c) => (c.innerText || '').includes('费用清单'))
    if (card) card.click()
  })
  for (let i = 0; i < 60; i++) {
    const ready = await page.evaluate(() =>
      Array.from(document.querySelectorAll('th')).some((e) =>
        (e.textContent || '').includes('光伏发电量')))
    if (ready) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(3500)
}

/** 读取 2026-10 数据行的单元格（按表头对齐）。 */
async function rowOf(period) {
  return page.evaluate((p) => {
    const table = document.querySelector('table')
    if (!table) return null
    const heads = Array.from(document.querySelectorAll('thead th')).map((e) => (e.innerText || '').trim())
    const tr = Array.from(document.querySelectorAll('tbody tr')).find((r) =>
      (r.innerText || '').includes(p))
    if (!tr) return null
    const cells = Array.from(tr.querySelectorAll('td')).map((e) => (e.innerText || '').trim())
    const out = {}
    heads.forEach((h, i) => { if (h) out[h] = cells[i] ?? '' })
    return out
  }, period)
}
/** 底部合计行（tfoot 或最后一行含「合计」）。 */
async function footRow() {
  return page.evaluate(() => {
    const table = document.querySelector('table')
    if (!table) return null
    const heads = Array.from(document.querySelectorAll('thead th')).map((e) => (e.innerText || '').trim())
    const rows = Array.from(document.querySelectorAll('tfoot tr, tbody tr'))
    const tr = rows.reverse().find((r) => (r.innerText || '').includes('合计'))
    if (!tr) return null
    const cells = Array.from(tr.querySelectorAll('td')).map((e) => (e.innerText || '').trim())
    const out = {}
    heads.forEach((h, i) => { if (h) out[h] = cells[i] ?? '' })
    return out
  })
}
const num = (s) => Number(String(s ?? '').replace(/[,，\s]/g, ''))

async function openSettings() {
  await page.getByRole('button', { name: /^设置$/ }).first().click()
  await page.waitForTimeout(3000)
  // 选中「费用清单」分类行（字段配置按分类组织）
  const cat = page.locator('.fc-category, .fc-cat-row, .category-row').filter({ hasText: '费用清单' }).first()
  if (await cat.count()) { await cat.click(); await page.waitForTimeout(1200) }
  // 切到「默认分组」胶囊
  const pill = page.locator('.fc-group-tabs .fc-group-tab').filter({ hasText: /默认分组/ }).first()
  if (await pill.count()) { await pill.click(); await page.waitForTimeout(1200) }
}
async function fieldRowIndex(re) {
  return page.evaluate((pattern) => {
    const rx = new RegExp(pattern)
    const rows = Array.from(document.querySelectorAll('.standard-settings-drawer .field-config-row'))
    return rows.findIndex((r) => rx.test((r.querySelector('input')?.value || '').trim()))
  }, re)
}
async function readDefaults() {
  return page.evaluate(async (tk) => {
    const res = await fetch('/api/v1/fleet/categories?scope=energy', {
      headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' },
    })
    const j = await res.json()
    const cat = (j?.data || []).find((c) => c.name === '费用清单') || {}
    return (cat.subs || []).filter((s) => (s.group === 'default' || s.group === 'cols' || !s.group)
      && s.enabled !== false && s.is_default === true).map((s) => s.name)
  }, TOKEN)
}

// ---------------------------------------------------------------------------
await gotoBilling()
// 清掉历史列显隐存档，确保断言的是后端 defaultHeader
await page.evaluate(() => {
  Object.keys(localStorage)
    .filter((k) => k.includes('weknora-tenant-billing-cols'))
    .forEach((k) => localStorage.removeItem(k))
})
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(15000)
await stripGuide()
// reload 后回到能耗首页，需要重新点费用清单卡片
await page.evaluate(() => {
  const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
  const card = cards.find((c) => (c.innerText || '').includes('费用清单'))
  if (card) card.click()
})
await page.waitForTimeout(3000)
const firstReady = await waitForReady()
check('0.1 六同步就绪（本期电量/本期电费/本期水量/本期水费 + 2026-10 行）', firstReady)

console.log('\n===== 断言 1：表头顺序 =====')
const heads = await headerTexts()
const iFee = heads.indexOf('本期电费')
const iWaterUsage = heads.indexOf('本期水量')
const iWaterFee = heads.indexOf('本期水费')
const iSolar = heads.indexOf('光伏发电量')
check('1.1 表头含「本期电费」', iFee >= 0, JSON.stringify(heads.slice(0, 10)))
check('1.2 表头含「本期水量」', iWaterUsage >= 0, `idx=${iWaterUsage}`)
check('1.3 表头含「本期水费」', iWaterFee >= 0, `idx=${iWaterFee}`)
check('1.4 本期水量紧跟本期电费', iWaterUsage === iFee + 1, `${iFee} -> ${iWaterUsage}`)
check('1.5 本期水费紧跟本期水量', iWaterFee === iWaterUsage + 1, `${iWaterUsage} -> ${iWaterFee}`)
check('1.6 光伏发电量紧跟本期水费', iSolar === iWaterFee + 1, `${iWaterFee} -> ${iSolar}`)
check('1.7 表头前缀 = 账单周期/使用单位/本期电量/本期电费',
  JSON.stringify(heads.slice(0, 4)) === JSON.stringify(['账单周期', '使用单位', '本期电量', '本期电费']),
  JSON.stringify(heads.slice(0, 6)))
await page.screenshot({ path: `${SHOTS}/r51-a1-headers.png` })

console.log('\n===== 断言 2：2026-10 数值与合计 =====')
const row = await rowOf('2026-10')
check('2.1 找到 2026-10 数据行', !!row, row ? JSON.stringify(row).slice(0, 160) : 'none')
if (row) {
  const wu = num(row['本期水量'])
  const wf = num(row['本期水费'])
  check('2.2 本期水量 = 769', wu === 769, `actual=${row['本期水量']}`)
  check('2.3 本期水费 = 4014.18', Math.abs(wf - 4014.18) < 0.005, `actual=${row['本期水费']}`)
  const ind = num(row['用水量（工业）'])
  check('2.4 用水量（工业）非负（无 -238 异常）', ind >= 0, `actual=${row['用水量（工业）']}`)
}
// 合计行首格是「合计（共 N 条）」跨列，与 thead 的列索引并不严格一一对齐，
// 因此改为「逐行求和 → 断言合计行文本里出现该值」，既验证累加又不依赖列偏移。
const sums = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('thead th')).map((e) => (e.innerText || '').trim())
  const iU = heads.indexOf('本期水量')
  const iF = heads.indexOf('本期水费')
  const rows = Array.from(document.querySelectorAll('tbody tr'))
    .filter((r) => /2026-\d{2}/.test(r.innerText || ''))
  let su = 0, sf = 0
  for (const r of rows) {
    const td = Array.from(r.querySelectorAll('td')).map((e) => (e.innerText || '').trim())
    su += Number((td[iU] || '').replace(/,/g, '')) || 0
    sf += Number((td[iF] || '').replace(/,/g, '')) || 0
  }
  const footTr = Array.from(document.querySelectorAll('tfoot tr, tbody tr, tr'))
    .reverse().find((r) => (r.innerText || '').includes('合计'))
  return {
    rows: rows.length,
    su: Math.round(su * 100) / 100,
    sf: Math.round(sf * 100) / 100,
    footTxt: footTr ? (footTr.innerText || '').replace(/\s+/g, ' ').trim() : '',
  }
})
const fmtNum = (v) => v.toLocaleString('en-US', { minimumFractionDigits: v % 1 ? 2 : 0, maximumFractionDigits: 2 })
check('2.5 合计行对本期水量准确累加',
  sums.su > 0 && sums.footTxt.includes(fmtNum(sums.su)),
  `逐行求和=${sums.su} 合计行=${sums.footTxt.slice(0, 80)}`)
check('2.6 合计行对本期水费准确累加',
  sums.sf > 0 && sums.footTxt.includes(fmtNum(sums.sf)),
  `逐行求和=${sums.sf} 合计行=${sums.footTxt.slice(0, 80)}`)
await page.screenshot({ path: `${SHOTS}/r51-a2-values.png` })

console.log('\n===== 断言 3：六同步动态开关 =====')
const baseline = await readDefaults()
check('3.1 后端默认表头含 current_water_usage', baseline.includes('current_water_usage'),
  JSON.stringify(baseline.slice(0, 8)))

await openSettings()
const idx = await fieldRowIndex('本期水量|current_water_usage')
check('3.2 设置里能定位到「本期水量」字段行', idx >= 0, `idx=${idx}`)
if (idx >= 0) {
  const rowEl = page.locator('.standard-settings-drawer .field-config-row').nth(idx)
  await rowEl.locator('.t-switch').first().click()
  await page.waitForTimeout(500)
  await page.locator('.standard-settings-drawer button:has-text("保存")').first().click()
  await page.waitForTimeout(3500)
  await page.keyboard.press('Escape')
  await page.waitForTimeout(2500)

  const afterOff = await headerTexts()
  check('3.3 关闭默认表头保存后表头立即隐藏「本期水量」',
    !afterOff.includes('本期水量'), JSON.stringify(afterOff.slice(0, 9)))
  check('3.4 关闭后「本期水费」仍在（只影响目标列）', afterOff.includes('本期水费'))

  const defaultsOff = await readDefaults()
  check('3.5 后端已持久化（默认表头不再含 current_water_usage）',
    !defaultsOff.includes('current_water_usage'), JSON.stringify(defaultsOff.slice(0, 8)))

  // 重新打开默认表头并保存
  await openSettings()
  const idx2 = await fieldRowIndex('本期水量|current_water_usage')
  if (idx2 >= 0) {
    await page.locator('.standard-settings-drawer .field-config-row').nth(idx2)
      .locator('.t-switch').first().click()
    await page.waitForTimeout(500)
    await page.locator('.standard-settings-drawer button:has-text("保存")').first().click()
    await page.waitForTimeout(3500)
    await page.keyboard.press('Escape')
    await page.waitForTimeout(2500)
  }
  const back = await headerTexts()
  check('3.6 重新打开后「本期水量」立即恢复', back.includes('本期水量'), JSON.stringify(back.slice(0, 9)))
  check('3.7 恢复后仍夹在本期电费与本期水费之间',
    back.indexOf('本期水量') === back.indexOf('本期电费') + 1 &&
    back.indexOf('本期水量') === back.indexOf('本期水费') - 1,
    JSON.stringify(back.slice(0, 9)))

  // 刷新后仍应保持（后端持久化 + 前端即时同步）
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(15000)
  await stripGuide()
  await page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
    const card = cards.find((c) => (c.innerText || '').includes('费用清单'))
    if (card) card.click()
  })
  await page.waitForTimeout(3000)
  await waitForReady()
  const afterReload = await headerTexts()
  check('3.8 刷新页面后「本期水量」仍在（持久化闭环）', afterReload.includes('本期水量'),
    JSON.stringify(afterReload.slice(0, 9)))
}
await page.screenshot({ path: `${SHOTS}/r51-a3-sixway.png` })

const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|Failed to load resource/.test(e))
check('3.9 控制台无 JS 错误', realErrors.length === 0, JSON.stringify(realErrors.slice(0, 2)))

await browser.close()
console.log(`\n===== RESULT ${ok}/${total} =====`)
if (failures.length) {
  console.log('FAILURES:')
  failures.forEach((f) => console.log('  - ' + f))
}
process.exit(failures.length ? 1 : 0)
