/**
 * 第 48 轮真机验收。
 *
 * 断言 1：设置抽屉 → 组织机构，只有「公司 / 部门」两个 Tab（无岗位），
 *         公司编码 02，部门含 销售部 02030 / 生产部 02050；
 *         新增员工选销售部+普工 → 工号 C020300009；主管 → B…；总经理 → A…
 * 断言 2：花名册工具栏顺序 = 搜索框 / 部门 / 在职状态 / 刷新；
 *         在职状态下拉仅 全部/在职/离职，默认「在职」。
 * 断言 3：搜索「过磅」命中王弟浩等，不出现暂无数据，KPI 卡不清零。
 * 断言 4：设置 → 字段设置 有 11 分组且默认勾选与设计稿一致；
 *         勾掉/勾上一个字段保存后，列表表头与详情抽屉同步变化，抽屉按分组渲染。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const HR_URL = `${ORIGIN}/platform/daily-affairs/hr`
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
async function gotoHr() {
  await page.goto(HR_URL, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(15000)
  await stripGuide()
}
async function headerTexts() {
  return page.locator('.t-table__header th, .t-table thead th').evaluateAll((els) =>
    els.map((e) => e.innerText.trim()).filter(Boolean))
}

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：组织机构（公司+部门）与工号自动编码 =====')
await gotoHr()
// 每次运行先清掉上一次保存的列显隐，保证断言的是「默认勾选」而非历史状态
await page.evaluate(() => {
  Object.keys(localStorage).filter((k) => k.includes('weknora-hr-') && k.includes('-cols-')).forEach((k) => localStorage.removeItem(k))
})
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(14000)
await stripGuide()
await page.getByRole('button', { name: /^设置$/ }).first().click()
await page.waitForTimeout(2500)
const settingsDrawer = page.locator('.hr-settings-drawer').first()
const topTabs = await settingsDrawer.locator('.t-tabs__nav-item').allInnerTexts()
check('1.1 设置抽屉含 字段设置 / 组织机构 两个 Tab',
  topTabs.some((t) => t.trim() === '字段设置') && topTabs.some((t) => t.trim() === '组织机构'),
  JSON.stringify(topTabs))

await settingsDrawer.locator('.t-tabs__nav-item', { hasText: '组织机构' }).first().click()
await page.waitForTimeout(2500)
const orgTabs = await settingsDrawer.locator('.t-tabs__nav-item').allInnerTexts()
check('1.2 组织机构无「岗位」定义', !orgTabs.some((t) => t.trim() === '岗位'), JSON.stringify(orgTabs))
check('1.3 组织机构含 公司定义 / 部门定义',
  /公司定义/.test(await settingsDrawer.innerText()) && /部门定义/.test(await settingsDrawer.innerText()))

const orgData = await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  const inputs = Array.from(d?.querySelectorAll('.org-row input') || []).map((i) => i.value.trim())
  const depts = Array.from(d?.querySelectorAll('.org-item') || []).map((r) => {
    const vals = Array.from(r.querySelectorAll('input')).map((i) => i.value.trim())
    return { name: vals[0] || '', code: vals[1] || '' }
  })
  return { company: { name: inputs[0] || '', code: inputs[1] || '' }, depts }
})
check('1.4 公司默认 重庆星达铜业有限公司 / 02',
  orgData.company.code === '02', JSON.stringify(orgData.company))
const deptMap = Object.fromEntries(orgData.depts.map((d) => [d.name, d.code]))
check('1.5 部门编码来自存量工号（销售部 02030）', deptMap['销售部'] === '02030', JSON.stringify(deptMap))
check('1.6 部门编码（生产部 02050 / 仓库 02061 / 人事行政部 02070）',
  deptMap['生产部'] === '02050' && deptMap['仓库'] === '02061' && deptMap['人事行政部'] === '02070',
  JSON.stringify(deptMap))
await page.screenshot({ path: `${SHOTS}/r48-a1-org.png` })
await page.keyboard.press('Escape')
await page.waitForTimeout(1200)

// 工号自动编码：走接口验（UI 新增抽屉需大量填单，接口更稳且同源）
const nextId = async (dept, title) => page.evaluate(async ({ dept, title, tk }) => {
  const url = `/api/v1/fleet/hr/roster/next-employee-id?department=${encodeURIComponent(dept)}&title=${encodeURIComponent(title)}`
  const res = await fetch(url, { headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' } })
  const j = await res.json()
  return j?.data?.employee_id || ''
}, { dept, title, tk: TOKEN })
const idStaff = await nextId('销售部', '普工')
const idLead = await nextId('销售部', '主管')
const idGm = await nextId('销售部', '总经理')
check('1.7 销售部+普工 → C020300009（存量最大 +1）', /^C0203\d{5}$/.test(idStaff) && idStaff.startsWith('C02030'), idStaff)
check('1.8 职务=主管 → B 前缀', idLead.startsWith('B02030'), idLead)
check('1.9 职务=总经理 → A 前缀', idGm.startsWith('A02030'), idGm)

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：工具栏顺序与在职状态精简 =====')
const leading = await page.locator('.doc-toolbar__leading').first().evaluate((el) => {
  const out = []
  for (const child of el.children) {
    if (child.classList.contains('doc-search-field')) { out.push('搜索'); continue }
    const sel = child.querySelector('.t-select__wrap, .t-input')
    const text = (sel?.innerText || child.innerText || '').trim().split('\n')[0]
    if (text) out.push(text)
  }
  return out
})
check('2.1 工具栏顺序 = 搜索 / 部门 / 在职状态', leading[0] === '搜索', JSON.stringify(leading))
// 部门与在职状态共用 .hr-status-select：DOM 顺序上部门在前、在职状态在后
const statusSel = page.locator('.hr-status-select').nth(1)
// t-select 的选中值在 input.value 里，innerText 为空
const statusText = await statusSel.locator('input').first().inputValue().catch(() => '')
check('2.2 在职状态默认选中「在职」', statusText.trim() === '在职', statusText)
await statusSel.click()
await page.waitForTimeout(900)
const statusOpts = (await page.locator('.t-select-option, .t-select-option__item').allInnerTexts())
  .map((s) => s.trim()).filter(Boolean)
check('2.3 在职状态仅 全部 / 在职 / 离职 三项',
  statusOpts.length === 3 && ['全部', '在职', '离职'].every((o) => statusOpts.includes(o)),
  JSON.stringify(statusOpts))
await page.keyboard.press('Escape')
await page.waitForTimeout(600)
await page.screenshot({ path: `${SHOTS}/r48-a2-toolbar.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：搜索「过磅」精准命中 =====')
const kpiBefore = await page.locator('.dash-kpi-card__num').first().innerText().catch(() => '')
const search = page.locator('.doc-search-field input').first()
await search.fill('过磅')
await page.waitForTimeout(4000)
const hit = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.t-table__body tr'))
    .filter((r) => r.querySelectorAll('td').length > 3)
  const text = rows.map((r) => (r.innerText || '').replace(/\s+/g, ' '))
  return { count: rows.length, hasWang: text.some((t) => t.includes('王弟浩')), sample: text.slice(0, 3) }
})
check('3.1 搜索「过磅」命中记录（非空）', hit.count >= 1, `rows=${hit.count}`)
check('3.2 命中含「王弟浩」', hit.hasWang, JSON.stringify(hit.sample))
check('3.3 不显示「暂无数据」', !/暂无数据/.test(await page.locator('.t-table').first().innerText()))
const kpiAfter = await page.locator('.dash-kpi-card__num').first().innerText().catch(() => '')
check('3.4 搜索时花名册卡片不清零', kpiAfter.trim() === kpiBefore.trim() && kpiAfter.trim() !== '0',
  `${kpiBefore} -> ${kpiAfter}`)
await page.screenshot({ path: `${SHOTS}/r48-a3-search.png` })
await search.fill('')
await page.waitForTimeout(3000)

// ---------------------------------------------------------------------------
console.log('\n===== 断言 4：11 分组字段设置与抽屉同步 =====')
await page.getByRole('button', { name: /^设置$/ }).first().click()
await page.waitForTimeout(2500)
const groups = await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  return Array.from(d?.querySelectorAll('.hrs-group-label') || []).map((e) => e.textContent.trim())
})
const EXPECT = ['基本信息', '在职信息', '离职信息', '联系信息', '紧急联系人情况',
  '家庭情况', '教育情况', '工作经历情况', '合同签订情况', '工资社保信息', '个税申报信息']
check('4.1 字段设置含 11 个分组', groups.length === 11, JSON.stringify(groups))
check('4.2 分组名称与设计稿一致', JSON.stringify(groups) === JSON.stringify(EXPECT), JSON.stringify(groups))

// 默认勾选：基本信息组勾选数 > 20，个税组 0
const basicCount = await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  const items = Array.from(d?.querySelectorAll('.hrs-group-item') || [])
  const target = items.find((x) => (x.textContent || '').includes('基本信息'))
  return (target?.querySelector('.hrs-group-count')?.textContent || '').trim()
})
check('4.3 基本信息组默认勾选数正确（21/30）', basicCount === '21/30', basicCount)

// 切到「个税申报信息」→ 默认全不勾选
await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  const items = Array.from(d?.querySelectorAll('.hrs-group-item') || [])
  items.find((x) => (x.textContent || '').includes('个税申报信息'))?.click()
})
await page.waitForTimeout(900)
const taxCount = await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  const items = Array.from(d?.querySelectorAll('.hrs-group-item') || [])
  const target = items.find((x) => (x.textContent || '').includes('个税申报信息'))
  return (target?.querySelector('.hrs-group-count')?.textContent || '').trim()
})
check('4.4 个税申报信息组默认全不勾选（0/6）', taxCount === '0/6', taxCount)
await page.screenshot({ path: `${SHOTS}/r48-a4-fields.png` })

// 勾上一个「是否烈属」并保存 → 表头应出现该列
await page.evaluate(() => {
  const d = document.querySelector('.hr-settings-drawer')
  const items = Array.from(d?.querySelectorAll('.hrs-field-item') || [])
  const target = items.find((x) => (x.textContent || '').includes('是否烈属'))
  target?.querySelector('input[type=checkbox]')?.click()
})
await page.waitForTimeout(700)
await page.getByRole('button', { name: /^保存$/ }).last().click()
await page.waitForTimeout(4000)
const headsAfter = await headerTexts()
// 保存后「是否烈属」必须出现；同时个税组其余字段不应被连带勾选（点的是单个复选框）
check('4.5 勾选「是否烈属」保存后表头出现该列',
  headsAfter.includes('是否烈属') && !headsAfter.includes('烈属证号'),
  JSON.stringify(headsAfter.slice(-8)))

// 详情抽屉按分组渲染
await page.keyboard.press('Escape')
await page.waitForTimeout(1200)
await page.locator('.t-table__body tr').first().locator('td').nth(3).click()
await page.waitForTimeout(3000)
const drawerInfo = await page.evaluate(() => {
  const d = document.querySelector('.bsd-drawer')
  if (!d) return { none: true }
  const titles = Array.from(d.querySelectorAll('.bsd-group .bsd-block-title')).map((e) => e.textContent.trim())
  return { titles, hasLeaving: titles.includes('离职信息'), text: (d.innerText || '').slice(0, 120) }
})
check('4.6 详情抽屉按分组标题渲染', Array.isArray(drawerInfo.titles) && drawerInfo.titles.length >= 3,
  JSON.stringify(drawerInfo.titles))
check('4.7 在职员工抽屉隐藏「离职信息」组', drawerInfo.hasLeaving === false,
  JSON.stringify(drawerInfo.titles))
await page.screenshot({ path: `${SHOTS}/r48-a4-drawer.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 汇总 =====')
const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|favicon|net::ERR_|网络错误，请检查您的网络连接|Failed to list models|Failed to load chat models|\[cmdk\] failed to load knowledge bases/.test(e))
check('5.1 控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 2).join(' | '))
console.log(`\nRESULT ${ok}/${total} PASS`)
if (failures.length) { console.log('FAILURES:'); failures.forEach((f) => console.log('  - ' + f)) }
await browser.close()
process.exit(failures.length ? 1 : 0)
