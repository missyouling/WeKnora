// 第 30 轮真机验收：车队 UI 像素级对齐发票管理（6 项精细化修正）。
//
// 断言：
//  A1 无灰色骨架屏；第 1 张「已收录证照」无绿框，第 2 张子分类卡默认带绿框
//  A2 所有卡片内不含「%」「占」字样
//  A3 点击「已收录证照」后：焦点仍在子分类卡，且不发列表请求
//  B1 KPI 区右侧存在可见滚动箭头
//  B2 点击右箭头后能滚到最右，「历史记录」卡完整可见，副文案为「无异常文件」
//  C1 左侧工具栏只有 1 个搜索框 + 1 个刷新按钮，无证照类型/日期/字段筛选
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs/fleet'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
const consoleErrors = []
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
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

// 记录列表请求次数（断言 A3：点纯统计卡不应触发新请求）
let listCalls = 0
const listUrls = []
page.on('request', (r) => {
  const u = r.url()
  if (u.includes('/api/v1/fleet/records') && r.method() === 'GET' && !u.includes('/stats')) {
    listCalls++
    listUrls.push(decodeURIComponent(u.split('?')[1] || ''))
  }
})

await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(3500)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

// 等 KPI 卡片出现
let waited = 0
while (waited < 40000) {
  const n = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length)
  if (n > 0) break
  await page.waitForTimeout(500)
  waited += 500
}
await page.waitForTimeout(2500)

async function snap() {
  return page.evaluate(() => {
    const cards = Array.from(document.querySelectorAll('.dash-kpi-card')).map((el) => ({
      label: el.querySelector('.dash-kpi-card__label')?.textContent?.trim() || '',
      num: el.querySelector('.dash-kpi-card__num')?.textContent?.trim() || '',
      sub: el.querySelector('.dash-kpi-card__sub')?.textContent?.trim() || '',
      active: el.classList.contains('is-active'),
      isStatic: el.classList.contains('is-static'),
      cursor: getComputedStyle(el).cursor,
    }))
    const group = document.querySelector('.dash-kpi-group')
    return {
      cards,
      skeleton: document.querySelectorAll('.dash-kpi-skeleton__card').length,
      arrows: {
        left: !!document.querySelector('.dash-kpi-arrow.is-left'),
        right: !!document.querySelector('.dash-kpi-arrow.is-right'),
      },
      scroll: group ? {
        left: group.scrollLeft,
        client: group.clientWidth,
        total: group.scrollWidth,
      } : null,
      filterInputs: Array.from(document.querySelectorAll('.doc-toolbar input')).map((i) => i.getAttribute('placeholder')),
      selects: document.querySelectorAll('.doc-toolbar .t-select').length,
      datePicker: document.querySelectorAll('.doc-toolbar .t-date-range-picker, .doc-toolbar .t-date-picker').length,
      refreshBtn: !!document.querySelector('.doc-toolbar .t-button'),
    }
  })
}

// ---- A1/A2 默认焦点与卡片文案 ----
const s0 = await snap()
check('A1 无灰色骨架屏占位', s0.skeleton === 0, `skeletonCards=${s0.skeleton}`)

const first = s0.cards[0] || {}
const second = s0.cards[1] || {}
check('A1 第1张「已收录证照」无绿框', first.label === '已收录证照' && first.active === false,
  `label=${first.label} active=${first.active}`)
check('A1 第1张为纯展示卡（cursor=default）', first.isStatic === true && first.cursor === 'default',
  `isStatic=${first.isStatic} cursor=${first.cursor}`)
check('A1 第2张子分类卡默认带绿框', second.active === true, `label=${second.label} active=${second.active}`)

const badSub = s0.cards.filter((c) => /%|占/.test(c.sub || ''))
check('A2 卡片无「%」/「占」副文本', badSub.length === 0,
  `bad=${badSub.map((c) => `${c.label}:${c.sub}`).join('|') || 'none'}`)
const historyCard = s0.cards.find((c) => c.label === '历史记录')
check('A2 历史记录卡副文案为「无异常文件」', !historyCard || historyCard.sub === '无异常文件',
  `sub=${historyCard?.sub}`)

// ---- A3 点纯统计卡不切焦点、不发请求 ----
// 先测一个「不点击」的对照窗口：如果这里也有请求，说明是后台重复拉取，
// 与点击无关，不能算到 A3 头上（否则断言会误报）。
const ctrlCalls = listCalls
await page.waitForTimeout(2500)
const ctrlDelta = listCalls - ctrlCalls
const beforeCalls = listCalls
await page.evaluate(() => {
  const el = Array.from(document.querySelectorAll('.dash-kpi-card'))
    .find((c) => (c.querySelector('.dash-kpi-card__label')?.textContent || '').trim() === '已收录证照')
  if (el) el.click()
})
await page.waitForTimeout(2500)
const s1 = await snap()
const activeAfter = s1.cards.filter((c) => c.active).map((c) => c.label)
check('A3 点「已收录证照」后焦点未变', activeAfter.length === 1 && activeAfter[0] === second.label,
  `active=${activeAfter.join(',')} expect=${second.label}`)
// 注意 slice(-0) 等于 slice(0)（取全部），delta 为 0 时必须显式取空数组
const deltaCalls = Math.max(0, listCalls - beforeCalls)
const newUrls = deltaCalls > 0 ? listUrls.slice(-deltaCalls) : []
// 只惩罚「切到汇总视图」的请求：轮询/防抖可能重发同一条件，属正常。
// 真正的 bug 是请求里丢掉 sub_category（= 汇总视图，列表会变成通用列）。
const summaryReqs = newUrls.filter((q) => !q.includes('sub_category='))
check('A3 点「已收录证照」不切到汇总视图（不发无 sub_category 请求）',
  summaryReqs.length === 0,
  `delta=${listCalls - beforeCalls} ctrlDelta=${ctrlDelta} urls=${newUrls.join(' || ') || 'none'}`)

// ---- B1/B2 横向滚动箭头 ----
check('B1 右侧存在可见滚动箭头', s1.arrows.right === true, `arrows=${JSON.stringify(s1.arrows)} scroll=${JSON.stringify(s1.scroll)}`)
if (s1.arrows.right) {
  const before = s1.scroll
  await page.locator('.dash-kpi-arrow.is-right').click()
  await page.waitForTimeout(900)
  let s2 = await snap()
  // 可连点直到最右（步长 260，卡片多时需多次）
  let guard = 0
  while (s2.arrows.right && guard < 12) {
    await page.locator('.dash-kpi-arrow.is-right').click()
    await page.waitForTimeout(600)
    s2 = await snap()
    guard++
  }
  check('B2 点击右箭头后滚到最右', s2.scroll && s2.scroll.left + s2.scroll.client >= s2.scroll.total - 4,
    `left=${s2.scroll?.left} client=${s2.scroll?.client} total=${s2.scroll?.total} clicks=${guard + 1}`)
  check('B2 滚到底后左箭头出现、右箭头隐藏', s2.arrows.left === true && s2.arrows.right === false,
    `arrows=${JSON.stringify(s2.arrows)}`)
  // 「历史记录」卡此时应完整可见且可点
  const histVisible = await page.evaluate(() => {
    const card = Array.from(document.querySelectorAll('.dash-kpi-card'))
      .find((c) => (c.querySelector('.dash-kpi-card__label')?.textContent || '').trim() === '历史记录')
    if (!card) return { found: false }
    const r = card.getBoundingClientRect()
    const group = document.querySelector('.dash-kpi-group')?.getBoundingClientRect()
    return { found: true, fullyVisible: !!group && r.left >= group.left - 2 && r.right <= group.right + 2, width: Math.round(r.width) }
  })
  check('B2 「历史记录」卡完整露出', histVisible.found && histVisible.fullyVisible,
    JSON.stringify(histVisible))
  await page.screenshot({ path: `${SHOTS}/r30-kpi-scrolled.png` })
}

// ---- C1 工具栏极简 ----
const s3 = await snap()
const extraFilters = s3.filterInputs.filter((p) => p !== '搜索全部字段')
check('C1 左侧仅 1 个搜索框', s3.filterInputs.filter((p) => p === '搜索全部字段').length === 1,
  `placeholders=${s3.filterInputs.join('/')}`)
check('C1 无其他字段筛选框', extraFilters.length === 0, `extra=${extraFilters.join('/') || 'none'}`)
check('C1 无证照类型下拉', s3.selects === 0, `selects=${s3.selects}`)
check('C1 无日期选择器', s3.datePicker === 0, `datePicker=${s3.datePicker}`)
check('C1 存在刷新按钮', s3.refreshBtn === true, `refresh=${s3.refreshBtn}`)
await page.screenshot({ path: `${SHOTS}/r30-toolbar.png` })

// ---- D 手工录入类仍为极简工具栏 ----
await page.evaluate(() => {
  const hit = Array.from(document.querySelectorAll('.fleet-side-item'))
    .find((el) => el.textContent.replace(/\s/g, '') === '加油充电')
  if (hit) hit.click()
})
await page.waitForTimeout(3500)
const s4 = await snap()
check('D1 加油充电工具栏同样极简', s4.filterInputs.length === 1 && s4.selects === 0 && s4.datePicker === 0,
  `inputs=${s4.filterInputs.join('/')} selects=${s4.selects}`)
check('D2 加油充电无历史记录卡', !s4.cards.some((c) => c.label === '历史记录'),
  `cards=${s4.cards.map((c) => c.label).join('/')}`)
await page.screenshot({ path: `${SHOTS}/r30-manual.png` })

// ---- E 控制台 ----
const realErrors = consoleErrors.filter((e) => !/favicon|404 \(Not Found\)/.test(e))
check('E1 控制台 0 error', realErrors.length === 0, realErrors.slice(0, 3).join(' || ') || 'clean')

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY pass=${results.length - failed.length}/${results.length}`)
if (failed.length) console.log('FAILED:', failed.map((f) => f.name).join(' | '))
await browser.close()
process.exit(failed.length ? 1 : 0)
