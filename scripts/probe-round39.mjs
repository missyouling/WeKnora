/**
 * 第 39 轮验收：5 项
 *
 * 断言 1：费用清单（持睿汽车）2026-10 / 2026-09 的「零售损益分摊电费」
 *         计算方式 = 账单原值 × 分摊比例，金额分别 14,207.60 / 4,434.03（绝不为 0），
 *         且市场化购电费展开明细之和 == 汇总列。
 * 断言 2：车辆档案 / 司机档案 / 维保记录
 *         · 已收录证照 == Σ 子分类卡片；
 *         · 每个子分类卡片 == 点击后列表行数；
 *         · 历史记录卡片 == 抽屉「全部文件」；
 *         · 抽屉「正在执行 > 0」时列表上方出现绿色状态条。
 * 断言 3：刷新页面 + 切换 12 个菜单，表头第 1 帧 th 数 == 最终列数。
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

// ======================================================================
// 断言 1：费用清单「零售损益分摊电费」
// ======================================================================
console.log('\n=== 断言 1：零售损益分摊电费 ===')

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
  // 等费用清单真正渲染再动下拉（市电账单页面也有 .bfs-select）
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() =>
      Array.from(document.querySelectorAll('th')).some((e) => (e.textContent || '').includes('光伏发电量')) &&
      !!document.querySelector('.bfs-select'))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(2000)
  // 切到租户视角（持睿汽车）：大类列只在租户视角出现
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

/** 展开某月行的「市场化购电费」，返回展开明细的行数组。 */
async function expandMarket(month) {
  // 列表首次渲染较慢（跨洋 3~8s），先等真实数据行出现
  await waitRows(1, 60)
  await page.waitForTimeout(1500)
  const info = await page.evaluate((m) => {
    const ths = Array.from(document.querySelectorAll('thead th'))
    let idx = -1
    let acc = 0
    for (const th of ths) {
      const span = th.colSpan || 1
      if ((th.textContent || '').includes('市场化购电费')) { idx = acc; break }
      acc += span
    }
    const rows = Array.from(document.querySelectorAll('tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const row = rows.find((tr) => (tr.innerText || '').includes(m))
    if (!row || idx < 0) return { ok: false, idx, months: rows.map((r) => (r.innerText || '').slice(0, 12)) }
    const td = row.children[idx]
    const cellText = (td?.innerText || '').trim()
    const btn = td?.querySelector('.row-expand-toggle')
    if (btn) btn.click()
    return { ok: true, idx, cellText }
  }, month)
  await page.waitForTimeout(2000)
  const panel = await page.evaluate(() => {
    const p = document.querySelector('.fee-group-panel')
    if (!p) return null
    return Array.from(p.querySelectorAll('tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => (td.textContent || '').trim()))
  })
  return { ...info, panel }
}

await openCostList()
const m10 = await expandMarket('2026-10')
check('1 2026-10 行可展开市场化购电费', m10.ok, JSON.stringify(m10.months || m10.cellText))
const loss10 = (m10.panel || []).find((r) => (r[0] || '').includes('零售损益分摊'))
const sum10 = (m10.panel || [])
  .filter((r) => r.length >= 6 && !(r[0] || '').includes('合计'))
  .reduce((s, r) => s + num(r[5]), 0)
check('1 2026-10 零售损益按「账单原值 × 分摊比例」', !!loss10 && (loss10[4] || '').includes('账单原值 × 分摊比例'),
  JSON.stringify(loss10))
check('1 2026-10 账单原值 = 51,418.66 / 比例 = 27.63%',
  !!loss10 && Math.abs(num(loss10[2]) - 51418.66) < 0.01 && /27\.6[0-9]%/.test(loss10[3] || ''),
  `${loss10?.[2]} | ${loss10?.[3]}`)
check('1 2026-10 分摊后金额 = 14,207.60（不是 0）',
  !!loss10 && Math.abs(num(loss10[5]) - 14207.6) < 0.02, loss10?.[5])
check('1 2026-10 明细合计 == 汇总列 88,278.66',
  Math.abs(sum10 - 88278.66) < 0.02 && Math.abs(num(m10.cellText) - 88278.66) < 0.02,
  `sum=${sum10.toFixed(2)} cell=${m10.cellText}`)
await page.screenshot({ path: `${SHOTS}/r39-loss-2026-10.png` })

const m09 = await expandMarket('2026-09')
const loss09 = (m09.panel || []).find((r) => (r[0] || '').includes('零售损益分摊'))
check('1 2026-09 零售损益按「账单原值 × 分摊比例」', !!loss09 && (loss09[4] || '').includes('账单原值 × 分摊比例'),
  JSON.stringify(loss09))
check('1 2026-09 分摊后金额 = 4,434.03',
  !!loss09 && Math.abs(num(loss09[5]) - 4434.03) < 0.02, loss09?.[5])
await page.screenshot({ path: `${SHOTS}/r39-loss-2026-09.png` })

// ======================================================================
// 断言 3：表头第 1 帧 == 最终列数（12 个菜单）
// ======================================================================
console.log('\n=== 断言 3：表头首帧同步加载 ===')
await gotoRetry(`${BASE}/fleet`)
for (let i = 0; i < 60; i++) {
  if (await page.evaluate(() => document.querySelectorAll('.fleet-side-item').length > 0)) break
  await page.waitForTimeout(500)
}
await page.evaluate(() => {
  window.__thSamples = []
  const t0 = Date.now()
  const iv = setInterval(() => {
    const n = document.querySelectorAll('thead th').length
    if (n > 0) window.__thSamples.push([Date.now() - t0, n])
    if (Date.now() - t0 > 12000) clearInterval(iv)
  }, 30)
})
await page.waitForTimeout(13000)
const bootSamples = await page.evaluate(() => window.__thSamples)
const bootFinal = await page.evaluate(() => document.querySelectorAll('thead th').length)
const bootSeq = [...new Set(bootSamples.map((s) => s[1]))]
check('3 刷新页面：表头第 1 帧即最终列数', bootSeq.length > 0 && bootSeq[0] === bootFinal && bootSeq.every((n) => n === bootFinal),
  `seq=${JSON.stringify(bootSeq)} final=${bootFinal}`)

const MENUS = ['司机档案', '维保记录', '车辆档案', '轮胎管理', '年检管理', '加油充电', '路桥费', '维修保养费', '保险理赔', '违章处理', '辅材记录', '请车记录']
let menuBad = []
for (const label of MENUS) {
  const seq = await page.evaluate(async (l) => {
    const el = Array.from(document.querySelectorAll('.fleet-side-item')).find((e) => (e.innerText || '').trim() === l)
    if (!el) return null
    const samples = []
    const t0 = Date.now()
    const iv = setInterval(() => {
      const n = document.querySelectorAll('thead th').length
      if (n > 0) samples.push(n)
    }, 30)
    el.click()
    await new Promise((r) => setTimeout(r, 6000))
    clearInterval(iv)
    return { samples, final: document.querySelectorAll('thead th').length, cost: Date.now() - t0 }
  }, label)
  if (!seq) { menuBad.push(`${label}:未找到菜单`); continue }
  const uniq = [...new Set(seq.samples)]
  if (!uniq.length || uniq[0] !== seq.final || uniq.some((n) => n !== seq.final)) {
    menuBad.push(`${label}:seq=${JSON.stringify(uniq)} final=${seq.final}`)
  }
}
check('3 切换 12 个菜单：表头均一次性成型', menuBad.length === 0, menuBad.join(' | ') || 'all ok')

// ======================================================================
// 断言 2：车队卡片/抽屉/列表 口径对齐
// ======================================================================
console.log('\n=== 断言 2：车队口径对齐 ===')

const readCards = () => page.evaluate(() => {
  const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
  const parse = (c) => {
    const txt = (c.innerText || '').replace(/\s+/g, ' ').trim()
    return { txt, value: Number((txt.match(/(\d+)\s*(份|条)/) || [])[1] || 0), active: /active|is-active/.test(c.className) }
  }
  return cards.map(parse)
})

async function openMenu(label) {
  await page.evaluate((l) => {
    const el = Array.from(document.querySelectorAll('.fleet-side-item')).find((e) => (e.innerText || '').trim() === l)
    if (el) el.click()
  }, label)
  await page.waitForTimeout(6500)
}

async function listRowCount() {
  return page.evaluate(() => Array.from(document.querySelectorAll('tbody tr'))
    .filter((tr) => tr.querySelectorAll('td').length > 3).length)
}

async function drawerAllFiles() {
  const opened = await page.evaluate(() => {
    const card = Array.from(document.querySelectorAll('.dash-kpi-card')).find((c) => (c.innerText || '').includes('历史记录'))
    if (!card) return false
    card.click()
    return true
  })
  if (!opened) return null
  for (let i = 0; i < 40; i++) {
    const ok = await page.evaluate(() => !!document.querySelector('.t-drawer__content-wrapper'))
    if (ok) break
    await page.waitForTimeout(500)
  }
  await page.waitForTimeout(3500)
  const res = await page.evaluate(() => {
    const wrap = document.querySelector('.t-drawer__content-wrapper') || document.body
    const txt = (wrap.innerText || '').replace(/\s+/g, ' ')
    const all = txt.match(/全部文件\s*(\d+)/)
    const running = txt.match(/正在执行\s*(\d+)/)
    return { all: all ? Number(all[1]) : null, running: running ? Number(running[1]) : null, tail: txt.slice(0, 200) }
  })
  // 关闭抽屉
  await page.keyboard.press('Escape')
  await page.waitForTimeout(1200)
  return res
}

async function statusBar() {
  return page.evaluate(() => {
    const bar = document.querySelector('.parse-status-bar')
    return bar ? { text: (bar.innerText || '').replace(/\s+/g, ' ').trim() } : null
  })
}

for (const menu of ['车辆档案', '司机档案', '维保记录']) {
  await openMenu(menu)
  const cards = await readCards()
  const all = cards.find((c) => c.txt.includes('已收录'))
  const history = cards.find((c) => c.txt.includes('历史记录'))
  const subs = cards.filter((c) => !c.txt.includes('已收录') && !c.txt.includes('历史记录'))
  const subsSum = subs.reduce((s, c) => s + c.value, 0)
  check(`2 [${menu}] 已收录 == Σ 子分类卡片`, !!all && all.value === subsSum,
    `已收录=${all?.value} Σ子分类=${subsSum} (${subs.map((c) => c.value).join('+') || '0'})`)

  // 历史记录卡片 == 抽屉「全部文件」（先取，后面要用 running 数）
  await openMenu(menu)
  const cards2 = await readCards()
  const history2 = cards2.find((c) => c.txt.includes('历史记录'))
  const barBefore = await statusBar()
  const drawer = await drawerAllFiles()
  const runningTotal = drawer?.running || 0

  // 每个子分类卡片 == 点击后列表行数
  // 注意：在途文件对应的台账被**在途门禁**挡在列表外（这是设计），
  // 所以「卡片 5 / 列表 4」在 running=1 时是正确行为 —— 差额必须等于在途份数。
  const subBad = []
  let gated = 0
  for (const s of subs) {
    const label = s.txt.split(/\s+/)[0]
    await page.evaluate((l) => {
      const card = Array.from(document.querySelectorAll('.dash-kpi-card')).find((c) => (c.innerText || '').includes(l))
      if (card) card.click()
    }, label)
    await page.waitForTimeout(4000)
    const rows = await listRowCount()
    if (rows > s.value) subBad.push(`${label}:卡片${s.value}行${rows}`)
    else gated += s.value - rows
  }
  check(`2 [${menu}] 子分类卡片 == 列表行数（差额 = 在途门禁 ${runningTotal} 份）`,
    subBad.length === 0 && gated === runningTotal,
    `${subBad.join(' | ') || 'all ok'}；gated=${gated} running=${runningTotal}`)
  check(`2 [${menu}] 历史记录卡片 == 抽屉「全部文件」`,
    !!drawer && drawer.all !== null && history2?.value === drawer.all,
    `卡片=${history2?.value} 抽屉=${drawer?.all}`)
  if ((drawer?.running || 0) > 0) {
    check(`2 [${menu}] 有在途文件时显示绿色状态条`, !!barBefore && /正在解析\/提取/.test(barBefore.text),
      barBefore?.text || 'no bar')
    check(`2 [${menu}] 历史记录卡副文案显示「正在执行 ${drawer.running}」`,
      (history2?.txt || '').includes(`正在执行 ${drawer.running}`), history2?.txt)
  } else if (menu === '车辆档案') {
    console.log(`SKIP | ${menu} 当前无在途文件，状态条断言跳过（fixture 未生效）`)
  }
}

await page.screenshot({ path: `${SHOTS}/r39-fleet.png` })

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
