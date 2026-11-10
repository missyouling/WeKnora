/**
 * 第 50 轮真机验收。
 *
 * 断言 1：六同步与后端持久化 —— 关掉「招聘渠道」默认表头并保存后表头立即少一列；
 *         刷新页面后仍不显示（证明已落 PostgreSQL）；[☰ 字段] 临时勾选后点「重置」
 *         精准回到后端 defaultHeader 集合（不是全选、不是前端静态数组）。
 * 断言 2：在职信息胶囊在前；表头 工号→姓名→部门→职务→岗位 在 证件号码 之前；
 *         在职态无任何离职字段；切「离职」后离职字段追加在末尾。
 * 断言 3：部门下拉与组织机构 100% 同源（无孤立「质量部」）；单位行可点击折叠/展开；
 *         单位与部门操作是删除图标 + 二次确认；部门改名保存后存量员工级联更新。
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
// 埋点：记录每次字段配置保存的请求体规模，定位「保存把默认表头写坏」的时序
const putLog = []
page.on('request', (r) => {
  if (r.method() !== 'PUT' || !/\/fleet\/categories\//.test(r.url())) return
  try {
    const body = JSON.parse(r.postData() || '{}')
    const subs = body.subs || []
    putLog.push({ total: subs.length, def: subs.filter((x) => x.is_default).length })
  } catch { putLog.push({ err: true }) }
})

async function stripGuide() {
  await page.evaluate(() => document.querySelectorAll('.guide').forEach((el) => el.remove()))
}
async function gotoHr(waitMs = 15000) {
  await page.goto(HR_URL, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(waitMs)
  await stripGuide()
}
async function headerTexts() {
  return page.locator('.t-table__header th').evaluateAll((els) =>
    els.map((e) => e.innerText.trim()).filter(Boolean))
}
async function openSettings(tabLabel = '字段设置') {
  await page.getByRole('button', { name: /^设置$/ }).first().click()
  await page.waitForTimeout(2600)
  if (tabLabel) {
    await page.locator('.standard-settings-drawer .t-tabs__nav-item')
      .filter({ hasText: tabLabel }).first().click()
    await page.waitForTimeout(1200)
    if (tabLabel === '组织机构') {
      // 面板随 Tab 懒挂载：必须等 .org-units 真正出现，否则读到 0 行部门
      await page.locator('.standard-settings-drawer .org-units')
        .waitFor({ state: 'visible', timeout: 20000 }).catch(() => {})
      await page.waitForTimeout(800)
    }
  }
}
async function closeSettings() {
  await page.keyboard.press('Escape')
  await page.waitForTimeout(1600)
}
/** 在字段配置里找到某字段行（按字段名输入框的当前值匹配）。 */
async function fieldRowIndex(name) {
  return page.evaluate((n) => {
    const rows = Array.from(document.querySelectorAll('.standard-settings-drawer .field-config-row'))
    return rows.findIndex((r) => {
      const inp = r.querySelector('input')
      return (inp?.value || '').trim() === n
    })
  }, name)
}
async function getOrgUnits() {
  return page.evaluate(async (tk) => {
    const res = await fetch('/api/v1/fleet/hr/org/units', {
      headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' },
    })
    const j = await res.json()
    return j?.data || {}
  }, TOKEN)
}
async function countByDept(dept) {
  return page.evaluate(async ({ tk, d }) => {
    const url = `/api/v1/fleet/hr/roster/employees?department=${encodeURIComponent(d)}&page_size=1`
    const res = await fetch(url, { headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' } })
    const j = await res.json()
    return Number(j?.data?.total || 0)
  }, { tk: TOKEN, d: dept })
}

// 清掉历史列显隐，确保断言的是后端默认表头
await gotoHr()
await page.evaluate(() => {
  Object.keys(localStorage).filter((k) => k.includes('weknora-hr-') && k.includes('-cols-'))
    .forEach((k) => localStorage.removeItem(k))
})
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(15000)
await stripGuide()

/** 读后端 defaultHeader 集合（唯一事实源）。 */
async function readDefaults() {
  return page.evaluate(async (tk) => {
    const res = await fetch('/api/v1/fleet/categories?scope=hr-roster', {
      headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' },
    })
    const j = await res.json()
    const subs = ((j?.data || [])[0] || {}).subs || []
    // 排除系统注入的状态字段：它 is_default 由 statusFields.ts 维护，不是设计稿字段
    return subs.filter((x) => x.is_default && x.enabled !== false)
      .map((x) => x.name).filter((n) => n !== '在职状态')
  }, TOKEN)
}

// 起始归一化：保证「招聘渠道」处于开启状态，否则本脚本每跑一次都会翻转一次语义
let baselineDef = await readDefaults()
if (!baselineDef.includes('招聘渠道')) {
  await openSettings('字段设置')
  const i0 = await fieldRowIndex('招聘渠道')
  if (i0 >= 0) {
    await page.locator('.standard-settings-drawer .field-config-row').nth(i0)
      .locator('.t-switch').first().click()
    await page.waitForTimeout(400)
    await page.locator('.standard-settings-drawer button:has-text("保存")').first().click()
    await page.waitForTimeout(3200)
  }
  await closeSettings()
  baselineDef = await readDefaults()
}
const BASE_COUNT = baselineDef.length
const baseHeads = await headerTexts()

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：六同步闭环 + 后端持久化 =====')
await openSettings('字段设置')

const pills = await page.evaluate(() => {
  const d = document.querySelector('.standard-settings-drawer')
  return Array.from(d?.querySelectorAll('.fc-group-tabs .fc-group-tab') || [])
    .filter((e) => e.querySelector('.fc-group-tab__count'))
    .map((e) => (e.textContent || '').replace(/\s+/g, ' ').trim())
})
check('1.1 胶囊第 1 个是「在职信息」', /^在职信息/.test(pills[0] || ''), JSON.stringify(pills.slice(0, 3)))
check('1.2 胶囊第 2 个是「基本信息」', /^基本信息/.test(pills[1] || ''), JSON.stringify(pills.slice(0, 3)))
check('1.3 胶囊共 11 个分组', pills.length === 11, String(pills.length))

const idxChannel = await fieldRowIndex('招聘渠道')
check('1.4 字段配置里能定位到「招聘渠道」', idxChannel >= 0, `idx=${idxChannel}`)
await page.screenshot({ path: `${SHOTS}/r50-a1-fields.png` })

// 关掉「招聘渠道」的默认表头（第 1 个开关）
const channelRow = page.locator('.standard-settings-drawer .field-config-row').nth(idxChannel)
await channelRow.locator('.t-switch').first().click()
await page.waitForTimeout(400)
const switchOff = await channelRow.locator('.t-switch').first().evaluate((el) =>
  el.classList.contains('t-is-checked'))
check('1.5 「招聘渠道」默认表头已切换为关闭', switchOff === false, String(switchOff))

await page.locator('.standard-settings-drawer button:has-text("保存")').first().click()
await page.waitForTimeout(3200)
await closeSettings()

const afterSave = await headerTexts()
check('1.6 保存后表头立即移除「招聘渠道」', !afterSave.includes('招聘渠道'), JSON.stringify(afterSave.slice(0, 6)))
// 列数用「表头基线」而不是 subs 基线：在职状态是系统状态字段，is_default=true 但不出列
check('1.7 保存后列数 = 表头基线 - 1', afterSave.length === baseHeads.length - 1,
  `${afterSave.length} vs ${baseHeads.length}`)

// 刷新页面：列显隐必须由后端 defaultHeader 恢复（不再多出招聘渠道）
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(15000)
await stripGuide()
const afterReload = await headerTexts()
check('1.8 刷新页面后「招聘渠道」仍不显示（后端已持久化）',
  !afterReload.includes('招聘渠道') && afterReload.length === baseHeads.length - 1,
  `${afterReload.length} vs ${baseHeads.length}`)

// [☰ 字段] 临时勾选「曾用名」→ 重置 → 必须回到后端 defaultHeader
await page.getByRole('button', { name: /^字段$/ }).first().click()
await page.waitForTimeout(1200)
const popVisible = await page.locator('.business-column-filter').isVisible().catch(() => false)
check('1.9 [☰ 字段] 弹层可打开', popVisible)
const usedName = page.locator('.business-column-filter .field-popup-item', { hasText: '曾用名' }).first()
const usedCount = await usedName.count()
if (usedCount > 0) {
  await usedName.click()
  await page.waitForTimeout(600)
}
await page.waitForTimeout(400)
await page.locator('.business-column-filter .field-popup-actions button:has-text("重置")').first().click()
await page.waitForTimeout(1600)
await page.keyboard.press('Escape')
await page.waitForTimeout(1200)

const afterReset = await headerTexts()
check('1.10 重置后不含「曾用名」（不回到全选）', !afterReset.includes('曾用名'), JSON.stringify(afterReset.slice(-5)))
check('1.11 重置后不含「招聘渠道」（严格等于后端 defaultHeader）',
  !afterReset.includes('招聘渠道'), JSON.stringify(afterReset.slice(0, 6)))
check('1.12 重置后列数 = 表头基线 - 1', afterReset.length === baseHeads.length - 1, String(afterReset.length))

// 恢复：把「招聘渠道」重新打开，避免污染用户默认配置
await openSettings('字段设置')
const idx2 = await fieldRowIndex('招聘渠道')
if (idx2 >= 0) {
  const row2 = page.locator('.standard-settings-drawer .field-config-row').nth(idx2)
  await row2.locator('.t-switch').first().click()
  await page.waitForTimeout(300)
  await page.locator('.standard-settings-drawer button:has-text("保存")').first().click()
  await page.waitForTimeout(3000)
}
await closeSettings()
const restored = await headerTexts()
check('1.13 恢复后「招聘渠道」重新出现', restored.includes('招聘渠道'), String(restored.length))

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：在职信息在前 + 离职字段按状态附加 =====')
const heads2 = await headerTexts()
const posOf = (k) => heads2.indexOf(k)
check('2.1 第 1 列是「工号」', heads2[0] === '工号', heads2[0])
check('2.2 第 2 列是「姓名」', heads2[1] === '姓名', heads2[1])
check('2.3 前 5 列 = 工号/姓名/部门/职务/岗位',
  JSON.stringify(heads2.slice(0, 5)) === JSON.stringify(['工号', '姓名', '部门', '职务', '岗位']),
  JSON.stringify(heads2.slice(0, 5)))
check('2.4 「岗位」排在「证件号码」之前', posOf('岗位') < posOf('证件号码'),
  `岗位@${posOf('岗位')} 证件号码@${posOf('证件号码')}`)
check('2.5 「司龄」排在「证件号码」之前', posOf('司龄') < posOf('证件号码'),
  `司龄@${posOf('司龄')}`)
const leavingFields = ['离职日期', '离职原因', '离职备注', '离职证明', '离职去向']
check('2.6 在职态表头不含任何离职字段',
  leavingFields.every((k) => !heads2.includes(k)),
  JSON.stringify(heads2.filter((h) => leavingFields.includes(h))))
await page.screenshot({ path: `${SHOTS}/r50-a2-active.png` })

// 切到「离职」
const statusSel = page.locator('.hr-status-select').nth(1)
await statusSel.click()
await page.waitForTimeout(900)
await page.locator('.t-select__dropdown .t-select-option', { hasText: '离职' }).first().click()
await page.waitForTimeout(4200)
const heads3 = await headerTexts()
check('2.7 离职态追加展示「离职日期」', heads3.includes('离职日期'), JSON.stringify(heads3.slice(-6)))
check('2.8 离职态追加展示「离职原因」', heads3.includes('离职原因'))
check('2.9 离职字段排在所有在职字段之后',
  heads3.indexOf('离职日期') > heads3.indexOf('证件号码'),
  `离职日期@${heads3.indexOf('离职日期')} 证件号码@${heads3.indexOf('证件号码')}`)
check('2.10 离职态仍以「工号」打头', heads3[0] === '工号', heads3[0])
await page.screenshot({ path: `${SHOTS}/r50-a2-resigned.png` })

// 切回在职
await page.locator('.hr-status-select').nth(1).click()
await page.waitForTimeout(900)
await page.locator('.t-select__dropdown .t-select-option', { hasText: '在职' }).first().click()
await page.waitForTimeout(4000)
const heads4 = await headerTexts()
check('2.11 切回在职后离职字段消失', !heads4.includes('离职日期'), JSON.stringify(heads4.slice(-4)))

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：组织机构折叠 / 删除图标 / 部门同源与级联改名 =====')
// 起始归一化：上一次运行若在改名断言中途失败，部门会留在「仓储物流部」。
// 先复原成「仓库」，保证脚本可重复执行（幂等）。
async function normalizeDeptName(from, to) {
  await page.evaluate(async ({ tk, from, to }) => {
    const bh = { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' }
    const res = await fetch('/api/v1/fleet/hr/org/units', { headers: bh })
    const d = (await res.json())?.data || {}
    const units = d.units || []
    if (!units.some((u) => (u.departments || []).some((x) => x.name === from))) return
    const payload = {
      units: units.map((u) => ({
        id: u.id, name: u.name, code: u.code,
        credit_code: u.credit_code || '', address: u.address || '', legal_person: u.legal_person || '',
        departments: (u.departments || []).map((x) => ({
          id: x.id, name: x.name === from ? to : x.name, code: x.code,
        })),
      })),
    }
    await fetch('/api/v1/fleet/hr/org/units', {
      method: 'PUT',
      headers: { ...bh, 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
  }, { tk: TOKEN, from, to })
  await page.waitForTimeout(2500)
}
await normalizeDeptName('仓储物流部', '仓库')
await page.reload({ waitUntil: 'domcontentloaded' })
await page.waitForTimeout(14000)
await stripGuide()

const units = await getOrgUnits()
const flatDepts = []
for (const u of units.units || []) {
  for (const d of u.departments || []) flatDepts.push(String(d.name || '').trim())
}
check('3.1 组织机构部门不含孤立的「质量部」', !flatDepts.includes('质量部'), JSON.stringify(flatDepts))
check('3.2 组织机构含「技术质量部」', flatDepts.includes('技术质量部'), JSON.stringify(flatDepts))

// 部门下拉选项
await page.locator('.hr-status-select').first().click()
await page.waitForTimeout(1000)
// 只取当前**可见**的那个弹层：两个下拉（部门 / 在职状态）的 dropdown 都在 DOM 里，
// 不筛可见性会把「全部/在职/离职」也算进部门选项。
const selectOpts = await page.locator('.t-select__dropdown:visible').last()
  .locator('.t-select-option').evaluateAll((els) =>
    els.map((e) => (e.innerText || '').trim()).filter(Boolean))
await page.keyboard.press('Escape')
await page.waitForTimeout(800)
const optsNoAll = selectOpts.filter((x) => x !== '全部部门')
check('3.3 部门下拉数量与组织机构一致', optsNoAll.length === flatDepts.length,
  `下拉 ${optsNoAll.length} vs 机构 ${flatDepts.length}`)
check('3.4 部门下拉名称与组织机构完全一致',
  JSON.stringify([...optsNoAll].sort()) === JSON.stringify([...flatDepts].sort()),
  JSON.stringify(optsNoAll))

// 组织机构 Tab
await openSettings('组织机构')
const orgRoot = page.locator('.standard-settings-drawer .org-units')
check('3.5 组织机构面板已渲染', await orgRoot.isVisible().catch(() => false))

// 删除图标（不是文字「删除」）
const unitText = await orgRoot.locator('.org-unit').first().innerText()
check('3.6 单位行不再显示文字「删除」', !unitText.includes('删除'), unitText.replace(/\s+/g, ' ').slice(0, 60))
const unitDelHasIcon = await orgRoot.locator('.org-unit .org-unit__ops svg').count()
check('3.7 单位删除是图标按钮', unitDelHasIcon > 0, `svg=${unitDelHasIcon}`)

// 二次确认：点删除图标弹出带单位名的确认
await orgRoot.locator('.org-unit .org-unit__ops button').first().click()
await page.waitForTimeout(1200)
const popText = await page.locator('.t-popconfirm').first().innerText().catch(() => '')
check('3.8 删除单位弹二次确认且含单位名',
  /确定删除单位/.test(popText) && /重庆|公司/.test(popText), popText.replace(/\s+/g, ' ').slice(0, 60))
// 取消，不能真删
await page.locator('.t-popconfirm button:has-text("取消")').first().click().catch(() => {})
await page.waitForTimeout(800)

// 折叠 / 展开
const formBefore = await orgRoot.locator('.org-form').isVisible().catch(() => false)
check('3.9 单位默认展开（可见单位信息表单）', formBefore === true)
await orgRoot.locator('.org-unit').first().click()
await page.waitForTimeout(900)
const formAfterCollapse = await orgRoot.locator('.org-form').count()
check('3.10 再次点击单位行可折叠收起编辑区', formAfterCollapse === 0, `count=${formAfterCollapse}`)
await orgRoot.locator('.org-unit').first().click()
await page.waitForTimeout(900)
check('3.11 再点一次可重新展开', (await orgRoot.locator('.org-form').count()) === 1)

// 部门删除图标 + 确认文案
const deptDelText = await orgRoot.locator('.org-dept').first().innerText()
check('3.12 部门行不再显示文字「删除」', !deptDelText.includes('删除'), deptDelText.replace(/\s+/g, ' ').slice(0, 50))
check('3.13 部门删除是图标按钮',
  (await orgRoot.locator('.org-dept .t-button svg').count()) > 0)
await orgRoot.locator('.org-dept').first().locator('.t-button').first().click()
await page.waitForTimeout(1000)
const deptPop = await page.locator('.t-popconfirm').first().innerText().catch(() => '')
check('3.14 删除部门二次确认含部门名', /确定删除部门/.test(deptPop), deptPop.replace(/\s+/g, ' ').slice(0, 50))
await page.locator('.t-popconfirm button:has-text("取消")').first().click().catch(() => {})
await page.waitForTimeout(800)
await page.screenshot({ path: `${SHOTS}/r50-a3-org.png` })

// 级联改名：仓库 → 仓储物流部
const beforeRename = await countByDept('仓库')
const deptRowIdx = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.standard-settings-drawer .org-dept'))
  return rows.findIndex((r) => (r.querySelector('input')?.value || '').trim() === '仓库')
})
check('3.15 组织机构里能定位到「仓库」', deptRowIdx >= 0, `idx=${deptRowIdx} 员工数=${beforeRename}`)

if (deptRowIdx >= 0 && beforeRename > 0) {
  const deptRow = orgRoot.locator('.org-dept').nth(deptRowIdx)
  await deptRow.locator('input').first().fill('仓储物流部')
  await page.waitForTimeout(400)
  await orgRoot.locator('.org-units__footer button:has-text("保存")').first().click()
  await page.waitForTimeout(4200)

  const afterRenameNew = await countByDept('仓储物流部')
  const afterRenameOld = await countByDept('仓库')
  check('3.16 改名后员工级联到「仓储物流部」', afterRenameNew > 0, `count=${afterRenameNew}`)
  check('3.17 改名后原「仓库」已无员工', afterRenameOld === 0, `count=${afterRenameOld}`)
  check('3.18 级联覆盖原有全部员工', afterRenameNew === beforeRename,
    `${afterRenameNew} vs ${beforeRename}`)

  // 列表部门列同步（前端刷新后）
  await closeSettings()
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(15000)
  await stripGuide()
  const deptCol = await headerTexts().then((hs) => hs.indexOf('部门'))
  const deptCells = await page.locator('.t-table__body td').evaluateAll((els) =>
    els.map((e) => (e.innerText || '').trim()))
  check('3.19 列表部门列出现「仓储物流部」', deptCells.includes('仓储物流部'),
    `deptCol=${deptCol}`)

  // 改回「仓库」
  await openSettings('组织机构')
  const backIdx = await page.evaluate(() => {
    const rows = Array.from(document.querySelectorAll('.standard-settings-drawer .org-dept'))
    return rows.findIndex((r) => (r.querySelector('input')?.value || '').trim() === '仓储物流部')
  })
  check('3.20 能定位到临时改名后的部门', backIdx >= 0, `idx=${backIdx}`)
  if (backIdx >= 0) {
    await page.locator('.standard-settings-drawer .org-dept').nth(backIdx)
      .locator('input').first().fill('仓库')
    await page.waitForTimeout(400)
    await page.locator('.standard-settings-drawer .org-units__footer button:has-text("保存")').first().click()
    await page.waitForTimeout(4200)
  }
  await closeSettings()
  const restoredDept = await countByDept('仓库')
  check('3.21 改名已复原为「仓库」', restoredDept === beforeRename, `${restoredDept} vs ${beforeRename}`)
} else {
  check('3.16 改名级联（前置数据不足，跳过）', false, `before=${beforeRename}`)
}

const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|Failed to load resource/.test(e))
check('3.22 控制台无 JS 错误', realErrors.length === 0, JSON.stringify(realErrors.slice(0, 2)))

// 后端口径复核：所有保存动作结束后，defaultHeader 必须仍是 30（招聘渠道已恢复）
const finalCfg = await page.evaluate(async (tk) => {
  const res = await fetch('/api/v1/fleet/categories?scope=hr-roster', {
    headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' },
  })
  const j = await res.json()
  const cat = (j?.data || [])[0] || {}
  const subs = cat.subs || []
  return {
    def: subs.filter((x) => x.is_default && x.enabled !== false)
      .map((x) => x.name).filter((n) => n !== '在职状态'),
    grouped: subs.filter((x) => (x.group || '').trim()).length,
    total: subs.length,
  }
}, TOKEN)
check('3.23 后端 defaultHeader 恢复为基线', finalCfg.def.length === BASE_COUNT,
  `count=${finalCfg.def.length} vs 基线 ${BASE_COUNT}`)
check('3.24 后端字段仍全部带分组', finalCfg.grouped >= finalCfg.total - 1,
  `grouped=${finalCfg.grouped}/${finalCfg.total}`)
console.log('PUT_LOG', JSON.stringify(putLog))

await page.screenshot({ path: `${SHOTS}/r50-final.png` })
await browser.close()

console.log(`\n===== RESULT ${ok}/${total} =====`)
if (failures.length) {
  console.log('FAILURES:')
  failures.forEach((f) => console.log('  - ' + f))
}
process.exit(failures.length ? 1 : 0)
