// 第 32 轮真机验收（4 条断言）
//
//  断言 1 加载行位置：切换分类卡时，loading 期间与加载完成后第 1 行 top 始终紧贴
//          thead 底部（误差 < 5px），行高不被拉伸
//  断言 2 滚轮切卡：卡片组上滚轮 → scrollLeft 变化；左右箭头仍可用
//  断言 3 设置抽屉：字段名 200px 紧凑、列宽输入框为正整数；
//          切「提取规则」Tab 时分类下拉从第 1 帧起就是选中分类，不闪「车辆登记证书」
//  断言 4 正则补洞启用：无「本模块暂未接入」提示，必填开关与正则输入框可编辑
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
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
  if (document.head) hide(); else document.addEventListener('DOMContentLoaded', hide)
}, TOKEN)

const page = await ctx.newPage()
page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => consoleErrors.push('PAGEERROR ' + String(e).slice(0, 200)))

await page.goto(`${BASE}/fleet`, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(5000)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

async function clickMenu(label) {
  await page.evaluate((l) => {
    const hit = Array.from(document.querySelectorAll('.fleet-side-item'))
      .find((el) => el.textContent.replace(/\s/g, '') === l)
    if (hit) hit.click()
  }, label)
  const t0 = Date.now()
  while (Date.now() - t0 < 25000) {
    const n = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length)
    if (n > 0) break
    await page.waitForTimeout(400)
  }
  await page.waitForTimeout(1200)
}

async function waitTableReady(timeout = 25000) {
  const t0 = Date.now()
  while (Date.now() - t0 < timeout) {
    const ok = await page.evaluate(() => {
      const loading = !!document.querySelector('.t-loading__overlay, .t-table .t-loading--full')
      const rows = document.querySelectorAll('.doc-table tbody tr').length
      const empty = !!document.querySelector('.biz-empty__desc')
      const foot = !!document.querySelector('.t-table__footer, tfoot')
      return !loading && (rows > 0 || empty || foot)
    })
    if (ok) return true
    await page.waitForTimeout(400)
  }
  return false
}

// ---- 断言 1：加载期间首行位置（浏览器内高频采样，避免 Node 侧漏掉 loading 窗口）----
async function sampleRowDuringCardClick(cardIndex) {
  await page.evaluate(() => {
    window.__rowSamples = []
    if (window.__rowTimer) clearInterval(window.__rowTimer)
    window.__rowTimer = setInterval(() => {
      const t = document.querySelector('.doc-table')
      if (!t) return
      const head = t.querySelector('thead')
      const tr = Array.from(t.querySelectorAll('tbody tr')).find((r) => r.querySelectorAll('td').length > 3)
      if (!head || !tr) return
      const hr = head.getBoundingClientRect()
      const rr = tr.getBoundingClientRect()
      window.__rowSamples.push({
        loading: !!t.querySelector('.t-loading__overlay'),
        gap: Math.round((rr.top - hr.bottom) * 10) / 10,
        rowH: Math.round(rr.height),
      })
    }, 60)
  })
  await page.locator('.dash-kpi-card').nth(cardIndex).click()
  await page.waitForTimeout(2600)
  const samples = await page.evaluate(() => {
    clearInterval(window.__rowTimer)
    window.__rowTimer = null
    return window.__rowSamples || []
  })
  return samples
}

for (const [menu, cardIndex] of [['车辆档案', 2], ['司机档案', 2]]) {
  await clickMenu(menu)
  await waitTableReady()
  const samples = await sampleRowDuringCardClick(cardIndex)
  const loadingSamples = samples.filter((s) => s.loading)
  const allGaps = samples.map((s) => s.gap)
  const bad = samples.filter((s) => s.gap > 5 || s.gap < -2)
  const maxRowH = samples.reduce((m, s) => Math.max(m, s.rowH), 0)
  check(
    `1 ${menu} 切换分类时首行紧贴表头（loading 与完成态一致）`,
    samples.length > 0 && bad.length === 0 && maxRowH <= 80,
    `samples=${samples.length} loadingSamples=${loadingSamples.length} gap=[${Math.min(...allGaps)}..${Math.max(...allGaps)}] maxRowH=${maxRowH} bad=${bad.length}`,
  )
}

// ---- 断言 2：滚轮横向切卡 + 箭头仍可用 ----
await clickMenu('车辆档案')
await waitTableReady()
const wheelInfo = await page.evaluate(async () => {
  const el = document.querySelector('.dash-kpi-group')
  if (!el) return { miss: true }
  el.scrollLeft = 0
  const before = el.scrollLeft
  const overflow = el.scrollWidth > el.clientWidth
  el.dispatchEvent(new WheelEvent('wheel', { deltaY: 240, bubbles: true, cancelable: true }))
  await new Promise((r) => setTimeout(r, 260))
  const after = el.scrollLeft
  return { before, after, overflow, hasRightArrow: !!document.querySelector('.dash-kpi-arrow.is-right') }
})
check(
  '2 卡片组滚轮转横向滚动',
  wheelInfo.overflow && wheelInfo.after > wheelInfo.before,
  `overflow=${wheelInfo.overflow} scrollLeft ${wheelInfo.before}→${wheelInfo.after} rightArrow=${wheelInfo.hasRightArrow}`,
)

const arrowInfo = await (async () => {
  const before = await page.evaluate(() => document.querySelector('.dash-kpi-group')?.scrollLeft ?? -1)
  const arrow = page.locator('.dash-kpi-arrow.is-right')
  const has = await arrow.count()
  if (has) await arrow.first().click()
  await page.waitForTimeout(500)
  const after = await page.evaluate(() => document.querySelector('.dash-kpi-group')?.scrollLeft ?? -1)
  return { before, after, has }
})()
check(
  '2 左右箭头点击滚动仍可用',
  arrowInfo.has > 0 && arrowInfo.after > arrowInfo.before,
  `rightArrow=${arrowInfo.has} scrollLeft ${arrowInfo.before}→${arrowInfo.after}`,
)

// ---- 断言 3 + 4：设置抽屉 ----
async function openSettings() {
  await page.evaluate(() => {
    const btn = Array.from(document.querySelectorAll('.doc-toolbar button'))
      .find((b) => (b.textContent || '').trim() === '设置')
    if (btn) btn.click()
  })
  const t0 = Date.now()
  while (Date.now() - t0 < 12000) {
    const open = await page.evaluate(() => {
      const w = document.querySelector('.standard-settings-drawer .t-drawer__content-wrapper')
      return !!w && w.getBoundingClientRect().width > 0
    })
    if (open) break
    await page.waitForTimeout(300)
  }
  await page.waitForTimeout(900)
}

await openSettings()

// 在分类表里选中「道路运输证」
const picked = await page.evaluate(() => {
  const row = Array.from(document.querySelectorAll('.meter-table-row'))
    .find((r) => (r.querySelector('.mtr-name')?.textContent || '').trim() === '道路运输证')
  if (!row) return ''
  row.click()
  return '道路运输证'
})
await page.waitForTimeout(900)

const layout = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.field-config-list .field-config-row:not(.sub-row)'))
  const nameInputs = rows.map((r) => r.querySelector('.fc-name input')).filter(Boolean)
  const widths = nameInputs.map((i) => Math.round(i.getBoundingClientRect().width))
  const widthVals = rows.map((r) => r.querySelector('.fc-width input')?.value).filter((v) => v !== undefined)
  const headText = Array.from(document.querySelectorAll('.field-colhead span')).map((s) => s.textContent.trim())
  return {
    rows: rows.length,
    nameWidths: widths,
    widthVals,
    headText,
  }
})
const nameOk = layout.nameWidths.length > 0
  && layout.nameWidths.every((w) => w >= 150 && w <= 232)
const widthOk = layout.widthVals.length > 0
  && layout.widthVals.every((v) => Number(v) > 0 && Number(v) <= 600)
check(
  '3 字段配置列宽收敛且列宽默认为正整数',
  nameOk && widthOk && layout.headText.includes('列宽(px)'),
  `rows=${layout.rows} nameW=[${layout.nameWidths.slice(0, 4).join(',')}] widthVals=[${layout.widthVals.slice(0, 4).join(',')}] head=${layout.headText.join('/')}`,
)

// 切到「提取规则」并在浏览器内高频采样下拉值。
// 采样必须在**点击的同一个 tick** 开始：面板挂载时若自己挑第一个分类，
// 只会闪现 1~3 帧，Node 侧 sleep 后再读必然看不到。
const certTexts = await page.evaluate(async () => {
  const out = []
  const read = () => {
    // TDesign Select 的值挂在 input.value 上（innerText 恒为空）
    const el = document.querySelector('.cert-type-select input')
    return (el?.value || '').trim()
  }
  const tab = Array.from(document.querySelectorAll('.t-tabs__nav-item'))
    .find((t) => (t.textContent || '').trim() === '提取规则')
  if (tab) tab.click()
  for (let i = 0; i < 40; i++) {
    out.push(read())
    await new Promise((r) => setTimeout(r, 40))
  }
  return out
})
// 等规则面板真正渲染出来再断言（切 Tab 后要拉一次配置）
for (let i = 0; i < 40; i++) {
  const ok = await page.evaluate(() => document.querySelectorAll('.extract-rule-panel .rtr-name').length > 0)
  if (ok) break
  await page.waitForTimeout(250)
}
const nonEmpty = certTexts.filter(Boolean)
const settled = nonEmpty[nonEmpty.length - 1] || ''
const flashedWrong = nonEmpty.some((t) => t.includes('车辆登记证书'))
check(
  '3 切「提取规则」分类不跳（从第 1 帧起就是选中分类）',
  picked !== '' && settled.includes('道路运输证') && !flashedWrong,
  `picked=${picked} settled="${settled}" firstFrames=${nonEmpty.slice(0, 4).join(' | ') || 'none'}`,
)

const rules = await page.evaluate(() => {
  const body = document.body.innerText
  const bannerGone = !body.includes('本模块暂未接入字段级校验与正则补洞')
  const requiredSw = Array.from(document.querySelectorAll('.rtr-required .t-switch'))
  const regexInputs = Array.from(document.querySelectorAll('.rtr-regex-input input'))
  const anyDisabled = requiredSw.some((s) => s.className.includes('t-is-disabled'))
  const regexDisabled = regexInputs.slice(0, 3).some((i) => i.disabled)
  return {
    bannerGone,
    requiredCount: requiredSw.length,
    regexCount: regexInputs.length,
    anyDisabled,
    regexDisabled,
    firstName: document.querySelector('.rtr-name')?.textContent?.trim() || '',
  }
})
check(
  '4 必填校验与正则兜底已启用（无置灰提示条）',
  rules.bannerGone && rules.requiredCount > 0 && rules.regexCount > 0 && !rules.anyDisabled && !rules.regexDisabled,
  `bannerGone=${rules.bannerGone} required=${rules.requiredCount} regex=${rules.regexCount} disabled=${rules.anyDisabled}/${rules.regexDisabled}`,
)

await page.screenshot({ path: `${SHOTS}/r32-rules.png` })
await page.evaluate(() => {
  const tab = Array.from(document.querySelectorAll('.t-tabs__nav-item'))
    .find((t) => (t.textContent || '').trim() === '字段定义')
  if (tab) tab.click()
})
await page.waitForTimeout(500)
await page.screenshot({ path: `${SHOTS}/r32-fields.png` })

// ---- 汇总 ----
const failed = results.filter((r) => !r.ok)
const realErrors = consoleErrors.filter((e) => !e.includes('favicon'))
console.log(`\nSUMMARY ${results.length - failed.length}/${results.length} PASS`)
if (failed.length) console.log('FAILED: ' + failed.map((f) => f.name).join(' ; '))
console.log(`CONSOLE_ERRORS ${realErrors.length}`)
if (realErrors.length) console.log(realErrors.slice(0, 5).join('\n'))

await browser.close()
process.exit(failed.length ? 1 : 0)
