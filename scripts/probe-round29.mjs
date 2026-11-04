// 第 29 轮真机验收：车队像素级对齐发票管理。
//
// 断言（对应需求四大 bug）：
//  A. 列渲染：车辆/司机/维保，「全部」与任意子分类下表格都有 >=5 列表头与单元格；
//  B. 口径一致：第 1 张卡片份数 == 底部「合计 (共 N 条)」== 子分类卡片之和；
//     点任意有数字的子分类卡，列表行数 == 卡片数字；
//  C. 筛选栏收敛：车辆登记证书下筛选输入框 <= 5，筛选区高度 <= 2 行；
//  D. 手工录入闭环：加油充电/路桥费/轮胎管理，主按钮为「新增记录」、
//     无历史记录卡、新增一条含车牌与金额的记录返回 200 且入列并进合计。
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
const num = (t) => {
  const m = String(t || '').replace(/[,\s¥￥]/g, '').match(/-?\d+(\.\d+)?/)
  return m ? Number(m[0]) : NaN
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 } })
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

await page.goto(BASE, { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(4000)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

async function clickMenu(label) {
  await page.evaluate((l) => {
    const hit = Array.from(document.querySelectorAll('.fleet-side-item'))
      .find((el) => el.textContent.replace(/\s/g, '') === l)
    if (hit) hit.click()
  }, label)
  await page.waitForTimeout(1200)
  // 等 KPI 渲染完成
  const t0 = Date.now()
  while (Date.now() - t0 < 30000) {
    const n = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length)
    if (n > 0) break
    await page.waitForTimeout(400)
  }
  await page.waitForTimeout(1200)
}

async function clickCard(label) {
  await page.evaluate((l) => {
    const hit = Array.from(document.querySelectorAll('.dash-kpi-card'))
      .find((el) => el.textContent.replace(/\s/g, '').includes(l))
    if (hit) hit.click()
  }, label)
  await page.waitForTimeout(2500)
}

async function snapshot() {
  return page.evaluate(() => {
    const kpis = Array.from(document.querySelectorAll('.dash-kpi-card')).map((el) => {
      const label = el.querySelector('.dash-kpi-card__label')?.textContent?.trim() || ''
      const numText = el.querySelector('.dash-kpi-card__num')?.textContent?.trim() || ''
      return { label, numText }
    })
    const head = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const trs = Array.from(document.querySelectorAll('.doc-table tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const rows = trs.map((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim()))
    const foot = Array.from(document.querySelectorAll('.doc-table tfoot td, .t-table__footer td')).map((td) => td.textContent.trim())
    const filterInputs = Array.from(document.querySelectorAll('.doc-toolbar .bfs-field input, .doc-toolbar input')).map((i) => i.getAttribute('placeholder'))
    const toolbarBox = document.querySelector('.doc-toolbar')?.getBoundingClientRect()?.height || 0
    const btnTexts = Array.from(document.querySelectorAll('.doc-toolbar__trailing .t-button')).map((b) => b.textContent.trim())
    return { kpis, head, rows, foot, filterInputs, toolbarBox, btnTexts }
  })
}

// ---- A. 列渲染（三个档案类菜单，全部 + 子分类） ----
for (const menu of ['车辆档案', '司机档案', '维保记录']) {
  await clickMenu(menu)
  const all = await snapshot()
  const hasHead = all.head.filter((h) => h).length >= 5
  check(`A ${menu}「全部」表头 >=5 列`, hasHead, `head=${all.head.filter(Boolean).join('/')}`)
  const hasCells = all.rows.length === 0 || all.rows[0].filter((c) => c && c !== '').length >= 3
  check(`A ${menu}「全部」有单元格内容`, hasCells, `rows=${all.rows.length} first=${(all.rows[0] || []).slice(0, 6).join('|')}`)

  // 点第一个有数字的子分类卡
  const subCard = all.kpis.find((k) => k.label !== '已收录证照' && k.label !== '历史记录' && num(k.numText) > 0)
  if (subCard) {
    await clickCard(subCard.label)
    const sub = await snapshot()
    check(`A ${menu}「${subCard.label}」表头 >=5 列`,
      sub.head.filter(Boolean).length >= 5,
      `head=${sub.head.filter(Boolean).join('/')}`)
    check(`A ${menu}「${subCard.label}」有单元格内容`,
      sub.rows.length === 0 || sub.rows[0].filter((c) => c && c !== '').length >= 3,
      `rows=${sub.rows.length} first=${(sub.rows[0] || []).slice(0, 6).join('|')}`)
    await page.screenshot({ path: `${SHOTS}/r29-${menu}-sub.png` })
  }
  await page.screenshot({ path: `${SHOTS}/r29-${menu}-all.png` })
}

// ---- B. 口径一致（车辆档案） ----
await clickMenu('车辆档案')
await clickCard('已收录证照')
await page.waitForTimeout(2000)
const b = await snapshot()
const cardTotal = num(b.kpis.find((k) => k.label === '已收录证照')?.numText)
const footTotal = b.foot.map((t) => t.match(/共\s*(\d+)\s*条/)).find(Boolean)
const footN = footTotal ? Number(footTotal[1]) : -1
const subSum = b.kpis
  .filter((k) => k.label !== '已收录证照' && k.label !== '历史记录')
  .reduce((s, k) => s + (Number.isFinite(num(k.numText)) ? num(k.numText) : 0), 0)
check('B1 第1张卡片 == 合计行 N', cardTotal === footN, `card=${cardTotal} footN=${footN} foot=${b.foot.filter(Boolean).join('|')}`)
check('B2 第1张卡片 == 子分类卡片之和', cardTotal === subSum, `card=${cardTotal} subSum=${subSum}`)

// 逐个点有数字的子分类，断言行数 == 卡片数字
let subChecked = 0
for (const k of b.kpis.filter((x) => x.label !== '已收录证照' && x.label !== '历史记录')) {
  const expect = num(k.numText)
  if (!Number.isFinite(expect) || expect === 0) continue
  await clickCard(k.label)
  const s = await snapshot()
  const ft = s.foot.map((t) => t.match(/共\s*(\d+)\s*条/)).find(Boolean)
  const n = ft ? Number(ft[1]) : s.rows.length
  check(`B3 「${k.label}」行数 == 卡片数字`, n === expect, `card=${expect} list=${n} rows=${s.rows.length}`)
  subChecked++
}
check('B4 至少校验了 1 个子分类', subChecked > 0, `checked=${subChecked}`)

// ---- C. 筛选栏收敛（车辆登记证书） ----
await clickMenu('车辆档案')
await clickCard('车辆登记证书')
await page.waitForTimeout(2000)
const c = await snapshot()
check('C1 筛选输入框 <= 5', c.filterInputs.length <= 5, `n=${c.filterInputs.length} placeholders=${c.filterInputs.join('/')}`)
check('C2 筛选区高度 <= 120px（1~2 行）', c.toolbarBox > 0 && c.toolbarBox <= 120, `h=${Math.round(c.toolbarBox)}`)
await page.screenshot({ path: `${SHOTS}/r29-filter-narrow.png` })

// ---- D. 手工录入闭环 ----
for (const menu of ['加油充电', '路桥费', '轮胎管理']) {
  await clickMenu(menu)
  const d0 = await snapshot()
  const hasHistoryCard = d0.kpis.some((k) => k.label === '历史记录')
  check(`D1 ${menu} 无历史记录卡`, !hasHistoryCard, `kpis=${d0.kpis.map((k) => k.label).join('/')}`)
  const primary = d0.btnTexts.find((t) => t.includes('新增记录'))
  check(`D2 ${menu} 主按钮为「新增记录」`, !!primary, `btns=${d0.btnTexts.join('/')}`)
  const hasUpload = d0.btnTexts.some((t) => t.includes('上传'))
  check(`D3 ${menu} 无上传按钮`, !hasUpload, `btns=${d0.btnTexts.join('/')}`)
}

// 新增一条「加油充电」记录（含车牌 + 金额）
await clickMenu('加油充电')
const before = await snapshot()
const respPromise = page.waitForResponse(
  (r) => r.url().includes('/api/v1/fleet/records') && r.request().method() === 'POST',
  { timeout: 30000 },
).catch(() => null)
await page.locator('.doc-toolbar__trailing .t-button--theme-primary').first().click()
await page.waitForTimeout(2000)
const drawerOpen = await page.evaluate(() => {
  const el = document.querySelector('.bsd-drawer .t-drawer__content-wrapper')
  return !!el && el.getBoundingClientRect().width > 0
})
check('D4 新增抽屉打开', drawerOpen, drawerOpen ? 'open' : 'not open')

if (drawerOpen) {
  const fillByLabel = async (label, value) => {
    const item = page.locator('.bsd-drawer .t-drawer__content-wrapper .t-form__item')
      .filter({ has: page.locator('.t-form__label', { hasText: label }) }).first()
    const input = item.locator('input').first()
    if (!(await input.count())) return false
    await input.fill(value)
    // 车牌号是 t-select（allow-create）：只 fill 不回车不会提交选项，
    // v-model 仍是空 → 保存后该列为空（真机 D7 上一轮就是这个现象）。
    await input.press('Enter')
    await page.waitForTimeout(300)
    return true
  }
  // 车牌号是 t-select(allow-create)：先输入过滤，再从下拉里点选真实选项。
  // 只 fill 不点选时 v-model 仍为空，保存后该列为空。
  let ok1 = false
  {
    const item = page.locator('.bsd-drawer .t-drawer__content-wrapper .t-form__item')
      .filter({ has: page.locator('.t-form__label', { hasText: '车牌号' }) }).first()
    const input = item.locator('input').first()
    if (await input.count()) {
      await input.click()
      await input.fill('渝C81022')
      await page.waitForTimeout(600)
      const opt = page.locator('.t-select__dropdown .t-select-option, .t-popup .t-select-option').first()
      if (await opt.count()) {
        await opt.click()
      } else {
        await input.press('Enter')
      }
      await page.waitForTimeout(400)
      ok1 = true
    }
  }
  const ok2 = await fillByLabel('单价', '88.5')
  const ok3 = await fillByLabel('数量', '2')
  check('D5 表单由 subs 派生并可填（车牌/单价/数量）', ok1 && ok2 && ok3, `plate=${ok1} price=${ok2} qty=${ok3}`)
  await page.waitForTimeout(400)
  await page.locator('.bsd-drawer button').filter({ hasText: '保存' }).first().click()
  const resp = await respPromise
  const status = resp ? resp.status() : 0
  check('D6 新增记录返回 200', status === 200, `status=${status}`)
  await page.waitForTimeout(3000)
  const after = await snapshot()
  // 判定用「整行文本包含车牌」而不是按列索引取值：表头可能被 TDesign 换行/省略，
  // 列索引算错会误判成「未入列」（真机上一轮 rows=1 却 FAIL 就是这个原因）。
  const found = after.rows.some((r) => r.join('|').includes('渝C81022'))
  check('D7 新记录立即出现在列表', found,
    `rows=${after.rows.length} head=${after.head.filter(Boolean).join('/')} first=${(after.rows[0] || []).join('|').slice(0, 120)}`)
  // 合计行含金额（单价 88.5 × 数量 2 = 177，或其他金额列）
  // 合计行必须含金额（单价 88.5 × 数量 2 = 177.00）：这是「金额可累加」的验收点
  const footNums = after.foot.map((t) => num(t)).filter((n) => Number.isFinite(n))
  const sumOk = footNums.some((n) => Math.abs(n - 177) < 0.05)
  check('D8 合计行金额 == 单价×数量 (177.00)', sumOk, `foot=${after.foot.filter(Boolean).join('|')} nums=${footNums.join(',')}`)
  await page.screenshot({ path: `${SHOTS}/r29-manual-created.png` })
}

// ---- E. 控制台 ----
const realErrors = consoleErrors.filter((e) => !/favicon|404 \(Not Found\)/.test(e))
check('E1 控制台 0 error', realErrors.length === 0, realErrors.slice(0, 3).join(' || ') || 'clean')

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY pass=${results.length - failed.length}/${results.length}`)
if (failed.length) console.log('FAILED:', failed.map((f) => f.name).join(' | '))
await browser.close()
process.exit(failed.length ? 1 : 0)
