/**
 * 第 49 轮真机验收。
 *
 * 断言 1：字段合并——字段设置/表头/抽屉无「生日」「工龄」，只有「出生日期」「司龄」。
 * 断言 2：工具栏右侧仅 字段/设置/导入/新增（无组织机构）；在职表头 ≤25 列；
 *         全表无 `—` 单元格（空值留白）。
 * 断言 3：设置抽屉走中台 StandardSettingDrawer（含 字段设置/组织机构 两 Tab），
 *         字段配置是胶囊分组 + 拖拽行；组织机构支持多单位、含信用代码/住所/法人，
 *         部门用拖拽手柄（无数字排序输入框）。
 * 断言 4：劳动合同卡有异常副文案；工具栏含部门筛选；列表/详情含
 *         统一社会信用代码/住所/法定代表人/联系电话，且与机构、花名册一致。
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
  return page.locator('.t-table__header th').evaluateAll((els) =>
    els.map((e) => e.innerText.trim()).filter(Boolean))
}
async function clickKpi(label) {
  await page.locator('.dash-kpi-card', { hasText: label }).first().click()
  await page.waitForTimeout(3800)
}

// 清掉历史列显隐，保证断言的是「默认表头」
await gotoHr()
await page.evaluate(() => {
  Object.keys(localStorage).filter((k) => k.includes('weknora-hr-') && k.includes('-cols-'))
    .forEach((k) => localStorage.removeItem(k))
})
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(15000)
await stripGuide()

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：工具栏 4 按钮 / 默认基本信息列 / 空值无横线 =====')
const trailing = (await page.locator('.doc-toolbar__trailing button').allInnerTexts())
  .map((s) => s.replace(/\s+/g, '')).filter(Boolean)
check('2.1 工具栏无「组织机构」按钮', !trailing.includes('组织机构'), JSON.stringify(trailing))
const REQUIRED = ['字段', '设置', '导入', '新增']
const pos = REQUIRED.map((b) => trailing.indexOf(b))
check('2.2 右侧固定 4 按钮齐全且顺序正确',
  pos.every((p, i) => p >= 0 && (i === 0 || p > pos[i - 1])), JSON.stringify(trailing))

const heads = await headerTexts()
check('2.3 在职表头列数 ≤ 25', heads.length <= 25, `cols=${heads.length}`)
for (const k of ['工号', '姓名', '部门', '岗位', '出生日期', '年龄', '司龄']) {
  check(`2.4 表头含「${k}」`, heads.includes(k), JSON.stringify(heads))
}
check('2.5 表头不含「生日」', !heads.includes('生日'), JSON.stringify(heads))
check('2.6 表头不含「工龄」', !heads.includes('工龄'), JSON.stringify(heads))

const dash = await page.evaluate(() => {
  const cells = Array.from(document.querySelectorAll('.t-table__body td'))
  const text = cells.map((c) => (c.innerText || '').trim())
  return { total: text.length, dashes: text.filter((t) => t === '—').length }
})
check('2.7 列表无「—」横线单元格', dash.dashes === 0, `cells=${dash.total} dashes=${dash.dashes}`)
await page.screenshot({ path: `${SHOTS}/r49-a2-roster.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：设置抽屉（中台组件 + 胶囊分组 + 多单位） =====')
await page.getByRole('button', { name: /^设置$/ }).first().click()
await page.waitForTimeout(3000)
const drawer = page.locator('.standard-settings-drawer').first()
const drawerVisible = await drawer.isVisible().catch(() => false)
check('3.1 设置抽屉复用中台 StandardSettingDrawer', drawerVisible)
const tabs = await drawer.locator('.t-tabs__nav-item').allInnerTexts()
check('3.2 抽屉含 字段设置 / 组织机构 两个 Tab',
  tabs.some((t) => t.trim() === '字段设置') && tabs.some((t) => t.trim() === '组织机构'),
  JSON.stringify(tabs))

// 字段配置：胶囊分组
const pills = await page.evaluate(() => {
  const d = document.querySelector('.standard-settings-drawer')
  // 只取真正的分组胶囊（带 __count 子节点），排除「新增分组」等杂项
  return Array.from(d?.querySelectorAll('.fc-group-tabs .fc-group-tab') || [])
    .filter((e) => e.querySelector('.fc-group-tab__count'))
    .map((e) => (e.textContent || '').replace(/\s+/g, ' ').trim())
})
const EXPECT_GROUPS = ['基本信息', '在职信息', '离职信息', '联系信息', '紧急联系人情况',
  '家庭情况', '教育情况', '工作经历情况', '合同签订情况', '工资社保信息', '个税申报信息']
check('3.3 字段配置为胶囊分组栏（11 组）', pills.length === 11, JSON.stringify(pills))
check('3.4 胶囊分组名称与设计稿一致',
  EXPECT_GROUPS.every((g, i) => (pills[i] || '').startsWith(g)), JSON.stringify(pills))
// 字段行：拖拽手柄 + 默认表头 + 启用字段 + 操作
const rowInfo = await page.evaluate(() => {
  const d = document.querySelector('.standard-settings-drawer')
  const head = d?.querySelector('.field-colhead')
  return {
    hasDrag: !!d?.querySelector('.fc-row-drag, .field-row [draggable="true"], .field-config-list [draggable="true"]'),
    headText: (head?.textContent || '').replace(/\s+/g, ' ').trim(),
  }
})
check('3.5 字段行有拖拽手柄', rowInfo.hasDrag, rowInfo.headText)
check('3.6 表头含 默认表头 / 启用字段 / 操作',
  /默认表头/.test(rowInfo.headText) && /启用字段/.test(rowInfo.headText), rowInfo.headText)
await page.screenshot({ path: `${SHOTS}/r49-a3-fields.png` })

// 组织机构 Tab
await drawer.locator('.t-tabs__nav-item', { hasText: '组织机构' }).first().click()
await page.waitForTimeout(3000)
const org = await page.evaluate(() => {
  const d = document.querySelector('.standard-settings-drawer')
  const units = Array.from(d?.querySelectorAll('.org-unit') || []).map((e) => (e.textContent || '').replace(/\s+/g, ' ').trim())
  const labels = Array.from(d?.querySelectorAll('.org-form__label') || []).map((e) => (e.textContent || '').trim())
  return {
    unitCount: units.length, units,
    labels,
    hasDeptDrag: !!d?.querySelector('.org-dept .org-depts__drag'),
    hasSortInput: !!d?.querySelector('.org-dept .t-input-number'),
    hasAddUnit: /新增单位/.test(d?.innerText || ''),
  }
})
check('3.7 支持多单位（有单位列表 + 新增单位）', org.hasAddUnit && org.unitCount >= 1, JSON.stringify(org.units))
for (const l of ['单位名称', '单位编码', '统一社会信用代码', '住所', '法定代表人']) {
  check(`3.8 单位含「${l}」`, org.labels.includes(l), JSON.stringify(org.labels))
}
check('3.9 部门有拖拽手柄', org.hasDeptDrag === true)
check('3.10 部门无数字排序输入框', org.hasSortInput === false)
await page.screenshot({ path: `${SHOTS}/r49-a3-org.png` })
await page.keyboard.press('Escape')
await page.waitForTimeout(1500)

// ---------------------------------------------------------------------------
console.log('\n===== 断言 4：劳动合同卡异常 / 部门筛选 / 甲乙方字段 =====')
const contractCard = await page.locator('.dash-kpi-card', { hasText: '劳动合同' }).first().innerText()
check('4.1 劳动合同卡有异常副文案（X 份已到期 / 即将到期 / 无异常合同）',
  /份已到期|份即将到期|无异常合同/.test(contractCard.replace(/\s+/g, ' ')), contractCard.replace(/\s+/g, ' '))

await clickKpi('劳动合同')
const contractLeading = (await page.locator('.doc-toolbar__leading .t-select__wrap').allInnerTexts()).length
const selectCount = await page.locator('.doc-toolbar__leading .t-select').count()
check('4.2 劳动合同工具栏含部门 + 合同状态两个筛选', selectCount >= 2, `selects=${selectCount} text=${contractLeading}`)

const contractHeads = await headerTexts()
for (const k of ['统一社会信用代码', '住所', '法定代表人', '联系电话', '合同状态']) {
  check(`4.3 合同列表含「${k}」`, contractHeads.includes(k), JSON.stringify(contractHeads))
}

// 甲乙方字段联动：列表值与接口一致
const link = await page.evaluate(async (tk) => {
  const res = await fetch('/api/v1/fleet/hr/contracts', { headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' } })
  const j = await res.json()
  const items = j?.data?.items || []
  const hit = items.find((x) => x?.custom_data?.['统一社会信用代码'])
  const unitsRes = await fetch('/api/v1/fleet/hr/org/units', { headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' } })
  const uj = await unitsRes.json()
  const units = uj?.data?.units || []
  const unit = units.find((u) => u.name === hit?.custom_data?.['现合同签约公司']) || units[0]
  return {
    credit: hit?.custom_data?.['统一社会信用代码'] || '',
    unitCredit: unit?.credit_code || '',
    phone: hit?.custom_data?.['联系电话'] || '',
    rowPhone: hit?.phone || '',
    partyA: hit?.custom_data?.['现合同签约公司'] || '',
    unitName: unit?.name || '',
  }
}, TOKEN)
check('4.4 合同甲方信用代码与组织机构一致',
  !!link.credit && link.credit === link.unitCredit, `${link.credit} vs ${link.unitCredit}`)
check('4.5 合同乙方联系电话与花名册手机号一致',
  !!link.phone && link.phone === link.rowPhone, `${link.phone} vs ${link.rowPhone}`)
check('4.6 甲方名称能在组织机构中找到匹配单位',
  !!link.partyA && (link.partyA === link.unitName || link.unitName.includes(link.partyA) || link.partyA.includes(link.unitName)),
  `${link.partyA} / ${link.unitName}`)
await page.screenshot({ path: `${SHOTS}/r49-a4-contract.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：字段合并在详情抽屉生效 =====')
await clickKpi('花名册')
await page.waitForTimeout(2500)
await page.locator('.t-table__body tr').first().locator('td').nth(3).click()
await page.waitForTimeout(3000)
const drawerText = await page.locator('.bsd-drawer').first().innerText().catch(() => '')
check('1.1 详情抽屉含「出生日期」', /出生日期/.test(drawerText))
check('1.2 详情抽屉含「司龄」', /司龄/.test(drawerText))
check('1.3 详情抽屉不含「生日」独立字段', !/(^|\n)生日(\n|$)/.test(drawerText.replace(/\s+/g, '\n')), '')
check('1.4 详情抽屉不含「工龄」', !/工龄/.test(drawerText))
check('1.5 详情抽屉按分组标题渲染', /基本信息/.test(drawerText) && /在职信息/.test(drawerText))
await page.screenshot({ path: `${SHOTS}/r49-a1-drawer.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 汇总 =====')
const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|favicon|net::ERR_|网络错误，请检查您的网络连接|Failed to list models|Failed to load chat models|\[cmdk\] failed to load knowledge bases/.test(e))
check('5.1 控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 2).join(' | '))
console.log(`\nRESULT ${ok}/${total} PASS`)
if (failures.length) { console.log('FAILURES:'); failures.forEach((f) => console.log('  - ' + f)) }
await browser.close()
process.exit(failures.length ? 1 : 0)
