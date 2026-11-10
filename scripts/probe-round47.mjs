/**
 * 第 47 轮真机验收。
 *
 * 断言 1：顶部依次 4 张卡片（花名册 / 劳动合同 / 社保管理 / 公积金管理）；
 *         点「劳动合同」展示合同列表与三色「合同状态」Tag。
 * 断言 2：花名册全量加载（行数=卡片数，不再 20 条）、工号递增、无工号排末尾、
 *         工资卡卡号无千分位且合计行为空、年龄 1 位小数、
 *         工具栏右侧 4 按钮顺序 = 字段 / 设置 / 导入 / 新增。
 * 断言 3：员工详情抽屉标题为「员工详情 · xxx」（无 P1 徽标），年龄/工龄只读，
 *         合同状态为 Tag；组织机构弹窗已自动生成公司/部门/岗位与编码。
 * 断言 4：真实社保目录 9 个正常文件导入 → 原始记录可见 → 生成汇总 →
 *         汇总视图出现 2026-10 → 点行打开费用清单抽屉，搜索/部门/险种筛选可用；
 *         花名册个人社保账号已回填。
 *
 * 运行（VM）：cd /opt/pw-run && node probe-round47.mjs
 */
import { chromium } from 'playwright'
import fs from 'node:fs'
import path from 'node:path'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`
const HR_URL = `${BASE}/hr`
const SHOTS = '/tmp/pw/shots'
const SI_DIR = '/mnt/si-2026-10'
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
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 }, acceptDownloads: true })
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
async function clickKpi(label) {
  await page.locator('.dash-kpi-card', { hasText: label }).first().click()
  await page.waitForTimeout(4000)
}
async function headerTexts() {
  return page.locator('.t-table__header th, .t-table thead th').evaluateAll((els) => els.map((e) => e.innerText.trim()))
}
async function api(method, path, body) {
  return page.evaluate(async ({ method, path, body, tk }) => {
    const headers = { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' }
    let payload
    if (body !== undefined) { headers['Content-Type'] = 'application/json'; payload = JSON.stringify(body) }
    const res = await fetch(path, { method, headers, body: payload })
    const text = await res.text()
    let json
    try { json = JSON.parse(text) } catch { json = { raw: text.slice(0, 300) } }
    return { status: res.status, json }
  }, { method, path, body, tk: TOKEN })
}

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：4 卡片与劳动合同视图 =====')
await gotoHr()
const kpiText = (await page.locator('.dash-kpi-group').first().innerText().catch(() => '')) || ''
const kpiCount = await page.locator('.dash-kpi-card').count()
check('1.1 顶部恰好 4 张 KPI 卡', kpiCount === 4, `count=${kpiCount}`)
const labels = await page.locator('.dash-kpi-card__label').allInnerTexts()
check('1.2 卡片顺序 = 花名册/劳动合同/社保管理/公积金管理',
  JSON.stringify(labels.map((s) => s.trim())) === JSON.stringify(['花名册', '劳动合同', '社保管理', '公积金管理']),
  JSON.stringify(labels))
check('1.3 卡片无冗余副标题', !/在职员工档案统一管理|五险账期与扣款明细|缴存台账与月度账单/.test(kpiText))

await clickKpi('劳动合同')
let heads = await headerTexts()
for (const h of ['工号', '姓名', '部门', '合同签约公司', '现合同编号', '现合同类型',
  '现合同开始日期', '现合同结束日期', '现合同期限', '现合同签订日期', '合同状态']) {
  check(`1.4 劳动合同表头含「${h}」`, heads.includes(h), JSON.stringify(heads))
}
// 三色 Tag：正常 / 即将到期 / 到期 至少出现一种，且 theme 正确
const tagInfo = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.t-table__body tr'))
  const out = []
  for (const r of rows) {
    const tag = r.querySelector('.t-tag')
    if (!tag) continue
    const text = (tag.textContent || '').trim()
    if (!['正常', '即将到期', '到期'].includes(text)) continue
    out.push({ text, cls: tag.className })
  }
  return out
})
check('1.5 合同状态列渲染三色 Tag', tagInfo.length > 0, JSON.stringify(tagInfo.slice(0, 3)))
const themeOk = tagInfo.every((t) => {
  if (t.text === '正常') return /success/.test(t.cls)
  if (t.text === '即将到期') return /warning/.test(t.cls)
  if (t.text === '到期') return /danger/.test(t.cls)
  return true
})
check('1.6 Tag 主题与状态匹配（正常绿/即将到期橙/到期红）', themeOk, JSON.stringify(tagInfo.slice(0, 3)))
await page.screenshot({ path: `${SHOTS}/r47-a1-contract.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：全量加载 / 工号递增 / 卡号不汇总 / 年龄小数 =====')
await clickKpi('花名册')
await page.waitForTimeout(2500)
const rosterInfo = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  const heads = Array.from(document.querySelectorAll('.t-table__header th')).map((t) => (t.textContent || '').trim())
  const idx = (name) => heads.indexOf(name)
  const cell = (r, name) => {
    const i = idx(name)
    return i < 0 ? null : (r.querySelectorAll('td')[i]?.innerText || '').trim()
  }
  return {
    rowCount: rows.length,
    empNos: rows.map((r) => cell(r, '工号')),
    cards: Array.from(document.querySelectorAll('.dash-kpi-card')).map((c) => ({
      label: (c.querySelector('.dash-kpi-card__label')?.textContent || '').trim(),
      num: (c.querySelector('.dash-kpi-card__num')?.textContent || '').trim(),
    })),
    footText: (document.querySelector('.t-table__footer')?.innerText || '').replace(/\s+/g, ' ').trim(),
    heads,
  }
})
const rosterCard = rosterInfo.cards.find((c) => c.label === '花名册')
const cardNum = Number((rosterCard?.num || '0').replace(/\D/g, '')) || 0
check('2.1 花名册卡片数与列表行数一致（不再截断 20 条）',
  rosterInfo.rowCount === cardNum && rosterInfo.rowCount > 20,
  `rows=${rosterInfo.rowCount} card=${cardNum}`)
check('2.2 合计行显示全部条数', rosterInfo.footText.includes(String(rosterInfo.rowCount)), rosterInfo.footText.slice(0, 80))

// 工号递增（有工号的行自然升序，空工号在末尾）
const empNos = rosterInfo.empNos
const nonEmpty = empNos.filter((v) => v && v !== '—')
const naturalKey = (s) => s.replace(/\d+/g, (d) => String(d).padStart(12, '0'))
const sorted = [...nonEmpty].sort((a, b) => naturalKey(a) < naturalKey(b) ? -1 : naturalKey(a) > naturalKey(b) ? 1 : 0)
check('2.3 工号按自然升序排列', JSON.stringify(nonEmpty) === JSON.stringify(sorted),
  `first=${nonEmpty.slice(0, 5).join(',')}`)
const firstEmptyIdx = empNos.findIndex((v) => !v || v === '—')
const lastNonEmptyIdx = empNos.map((v) => !!(v && v !== '—')).lastIndexOf(true)
check('2.4 无工号记录排在末尾', firstEmptyIdx === -1 || firstEmptyIdx > lastNonEmptyIdx,
  `firstEmpty=${firstEmptyIdx} lastNonEmpty=${lastNonEmptyIdx}`)

// 工资卡卡号：无千分位、合计行为空
const cardCheck = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('.t-table__header th')).map((t) => (t.textContent || '').trim())
  const i = heads.indexOf('工资卡卡号')
  if (i < 0) return { hasCol: false }
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  const values = rows.map((r) => (r.querySelectorAll('td')[i]?.innerText || '').trim()).filter((v) => v && v !== '—')
  const foot = document.querySelector('.t-table__footer')
  const footCells = foot ? Array.from(foot.querySelectorAll('td')).map((td) => (td.innerText || '').trim()) : []
  return { hasCol: true, sample: values.slice(0, 3), withComma: values.filter((v) => v.includes(',')).length, footCell: footCells[i] ?? null }
})
if (cardCheck.hasCol) {
  check('2.5 工资卡卡号无千分位逗号', cardCheck.withComma === 0, JSON.stringify(cardCheck.sample))
  check('2.6 工资卡卡号合计行为空串', !cardCheck.footCell, `foot=${JSON.stringify(cardCheck.footCell)}`)
} else {
  check('2.5 工资卡卡号无千分位逗号', false, '列未显示，需在字段里勾选')
  check('2.6 工资卡卡号合计行为空串', false, '列未显示')
}

// 年龄 1 位小数
const ageInfo = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('.t-table__header th')).map((t) => (t.textContent || '').trim())
  const i = heads.indexOf('年龄')
  if (i < 0) return { hasCol: false, values: [] }
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  return { hasCol: true, values: rows.map((r) => (r.querySelectorAll('td')[i]?.innerText || '').trim()).filter((v) => v && v !== '—') }
})
if (ageInfo.hasCol && ageInfo.values.length) {
  const decimal = ageInfo.values.filter((v) => /^\d+\.\d$/.test(v)).length
  check('2.7 年龄保留 1 位小数', decimal === ageInfo.values.length, `sample=${ageInfo.values.slice(0, 5).join(',')}`)
} else {
  check('2.7 年龄保留 1 位小数', false, '年龄列未显示')
}

// 工具栏 4 按钮顺序
const toolbarButtons = await page.locator('.doc-toolbar__trailing button').allInnerTexts()
const cleaned = toolbarButtons.map((s) => s.replace(/\s+/g, '')).filter(Boolean)
// 固定 4 按钮是「字段 / 设置 / 导入 / 新增」，其余业务按钮（组织机构/导出）可插在字段之后
const REQUIRED = ['字段', '设置', '导入', '新增']
const positions = REQUIRED.map((b) => cleaned.indexOf(b))
const ordered = positions.every((p, i) => p >= 0 && (i === 0 || p > positions[i - 1]))
check('2.8 右侧固定 4 按钮齐全且相对顺序 = 字段/设置/导入/新增',
  ordered, JSON.stringify(cleaned))
await page.screenshot({ path: `${SHOTS}/r47-a2-roster.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：员工详情抽屉与组织机构 =====')
const firstRow = page.locator('.t-table__body tr').first()
await firstRow.click()
await page.waitForTimeout(2500)
const drawer = page.locator('.t-drawer').last()
const drawerText = await drawer.innerText().catch(() => '')
const drawerHeader = await page.evaluate(() => {
  const d = document.querySelector('.bsd-drawer') || document.querySelector('.t-drawer')
  const h = d?.querySelector('.t-drawer__header') || d?.querySelector('.t-drawer__header-content')
  return (h?.innerText || '').trim()
})
check('3.1 抽屉标题为「员工详情」', /员工详情/.test(drawerHeader), drawerHeader.slice(0, 60))
check('3.2 抽屉无 [P1] 徽标', !/\[?P1\]?/.test(drawerText), (drawerText.match(/P\d+/) || [''])[0])
check('3.3 抽屉内无重复二级标题「花名册详情」', !/花名册详情/.test(drawerText))
// 年龄 / 工龄只读
const readonlyInfo = await page.evaluate(() => {
  const labels = Array.from(document.querySelectorAll('.t-drawer .t-form__label, .t-drawer label'))
  const find = (text) => labels.find((l) => (l.textContent || '').trim().startsWith(text))
  const state = (text) => {
    const l = find(text)
    if (!l) return null
    const item = l.closest('.t-form__item') || l.parentElement
    const input = item?.querySelector('input, textarea')
    return input ? { disabled: input.disabled, readonly: input.readOnly } : { missing: true }
  }
  return { age: state('年龄'), workYears: state('工龄') }
})
const ageRo = readonlyInfo.age && (readonlyInfo.age.disabled || readonlyInfo.age.readonly)
const workRo = readonlyInfo.workYears && (readonlyInfo.workYears.disabled || readonlyInfo.workYears.readonly)
check('3.4 年龄只读', !!ageRo, JSON.stringify(readonlyInfo.age))
check('3.5 工龄只读', !!workRo, JSON.stringify(readonlyInfo.workYears))
await page.keyboard.press('Escape')
await page.waitForTimeout(1200)

// 组织机构
const orgBtn = page.getByRole('button', { name: /组织机构/ }).first()
await orgBtn.click()
const orgDlg = page.locator('.t-dialog', { hasText: '组织机构' }).last()
await orgDlg.waitFor({ state: 'visible', timeout: 15000 })
await page.waitForTimeout(2000)
const orgTabs = await orgDlg.locator('.t-tabs__nav-item').allInnerTexts()
check('3.6 组织机构含 公司/部门/岗位 三个 Tab',
  ['公司', '部门', '岗位'].every((t) => orgTabs.some((x) => x.trim() === t)), JSON.stringify(orgTabs))
await orgDlg.locator('.t-tabs__nav-item', { hasText: '部门' }).first().click()
await page.waitForTimeout(2000)
// 部门名与编码都在 t-input 的 value 里（不进 innerText），必须读 value
const deptData = await page.evaluate(() => {
  const d = Array.from(document.querySelectorAll('.t-dialog')).find((x) => /组织机构/.test(x.innerText || ''))
  const rows = Array.from(d?.querySelectorAll('.org-item') || [])
  return rows.map((r) => {
    const inputs = Array.from(r.querySelectorAll('input')).map((i) => i.value.trim())
    return { name: inputs[0] || '', code: inputs[1] || '' }
  })
})
const deptNames = deptData.map((x) => x.name).filter(Boolean)
check('3.7 部门已自动生成（含生产部/人事行政部/仓库）',
  ['生产部', '人事行政部', '仓库'].filter((d) => deptNames.includes(d)).length >= 2,
  JSON.stringify(deptNames.slice(0, 8)))
check('3.8 部门带默认编码（D01…）',
  deptData.length > 0 && deptData.every((x) => /^D\d{2,}$/.test(x.code)),
  JSON.stringify(deptData.slice(0, 3)))
await orgDlg.locator('.t-tabs__nav-item', { hasText: '岗位' }).first().click()
await page.waitForTimeout(2000)
const posData = await page.evaluate(() => {
  const d = Array.from(document.querySelectorAll('.t-dialog')).find((x) => /组织机构/.test(x.innerText || ''))
  const rows = Array.from(d?.querySelectorAll('.org-item') || [])
  return rows.map((r) => {
    const inputs = Array.from(r.querySelectorAll('input')).map((i) => i.value.trim())
    return { name: inputs[0] || '', code: inputs[1] || '' }
  })
})
check('3.9 岗位已自动生成且带编码（P01…）',
  posData.length > 0 && posData.filter((x) => /^P\d{2,}$/.test(x.code)).length === posData.length,
  JSON.stringify(posData.slice(0, 3)))
await page.screenshot({ path: `${SHOTS}/r47-a3-org.png` })
await page.keyboard.press('Escape')
await page.waitForTimeout(1000)

// ---------------------------------------------------------------------------
console.log('\n===== 断言 4：真实社保导入与原始/汇总双视图 =====')
// 通过页面上下文上传 9 个真实文件（用 fetch + FormData 读本地文件不方便，改用后端直传）
const siFiles = fs.existsSync(SI_DIR) ? fs.readdirSync(SI_DIR).filter((f) => /\.xlsx$/i.test(f)) : []
check('4.1 找到真实社保文件（9 个正常社保）', siFiles.length === 9, `files=${siFiles.length}`)

// 用 Playwright 的文件选择器走真实 UI 导入
await clickKpi('社保管理')
await page.waitForTimeout(2000)
const importBtn = page.getByRole('button', { name: /^导入$/ }).first()
const [chooser] = await Promise.all([
  page.waitForEvent('filechooser', { timeout: 15000 }).catch(() => null),
  importBtn.click(),
])
if (chooser) {
  await chooser.setFiles(siFiles.map((f) => path.join(SI_DIR, f)))
  await page.waitForTimeout(12000)
}
const rawCount = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  return rows.length
})
// 表格首屏只渲染可见行（虚拟滚动），用接口总数判断是否真的导入成功
const rawTotal = await page.evaluate(async (tk) => {
  const res = await fetch('/api/v1/fleet/hr/social/raw', { headers: { Authorization: `Bearer ${tk}`, 'X-Tenant-ID': '10000' } })
  const j = await res.json()
  return Number(j?.data?.total || 0)
}, TOKEN)
check('4.2 原始记录已导入并展示（接口总数 9 险种 × 77 人）',
  rawTotal >= 600 && rawCount >= 10, `apiTotal=${rawTotal} rows=${rawCount}`)
const rawHeads = await headerTexts()
for (const h of ['账期', '记录性质', '险种', '个人社保编号', '单位应缴', '个人应缴']) {
  check(`4.3 原始记录表头含「${h}」`, rawHeads.includes(h), JSON.stringify(rawHeads))
}
await page.screenshot({ path: `${SHOTS}/r47-a4-raw.png` })

// 勾选若干行 → 生成汇总
const boxes = page.locator('.t-table__body .t-checkbox')
const boxCount = await boxes.count()
if (boxCount > 0) {
  for (let i = 0; i < Math.min(5, boxCount); i++) await boxes.nth(i).click()
  await page.waitForTimeout(1200)
}
const genBtn = page.getByRole('button', { name: /生成汇总/ }).first()
check('4.4 批量栏提供「生成汇总」', await genBtn.isVisible().catch(() => false))
if (await genBtn.isVisible().catch(() => false)) {
  await genBtn.click()
  await page.waitForTimeout(8000)
}
// 切到汇总记录
const sumHeads = await headerTexts()
check('4.5 已切到汇总记录视图（含参保人数/应缴总计）',
  sumHeads.includes('参保人数') && sumHeads.includes('应缴总计'), JSON.stringify(sumHeads))
const sumInfo = await page.evaluate(() => {
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  const heads = Array.from(document.querySelectorAll('.t-table__header th')).map((t) => (t.textContent || '').trim())
  const i = heads.indexOf('账期')
  return { count: rows.length, months: rows.map((r) => (r.querySelectorAll('td')[i]?.innerText || '').trim()) }
})
check('4.6 汇总记录出现 2026-10', sumInfo.months.includes('2026-10'), JSON.stringify(sumInfo.months.slice(0, 5)))
check('4.7 每月仅 1 条汇总记录', new Set(sumInfo.months).size === sumInfo.count, `count=${sumInfo.count}`)
await page.screenshot({ path: `${SHOTS}/r47-a4-summary.png` })

// 点汇总行 → 费用清单抽屉
await page.locator('.t-table__body tr').first().click()
await page.waitForTimeout(5000)
const siDrawer = page.locator('.hr-si-detail-drawer').last()
const siDrawerVisible = await siDrawer.isVisible().catch(() => false)
check('4.8 点汇总行打开费用清单抽屉', siDrawerVisible)
if (siDrawerVisible) {
  const filterCount = await siDrawer.locator('.hr-si-filters .t-input, .hr-si-filters .t-select').count()
  check('4.9 抽屉含搜索/部门/险种三个筛选', filterCount >= 3, `controls=${filterCount}`)
  const detailRows = await siDrawer.locator('.t-table__body tr').evaluateAll((rows) =>
    rows.filter((r) => r.querySelectorAll('td').length > 3).length)
  check('4.10 抽屉展示全员各险种明细', detailRows > 10, `rows=${detailRows}`)
  // 搜索筛选
  const searchInput = siDrawer.locator('.hr-si-search input').first()
  if (await searchInput.count()) {
    const firstName = await siDrawer.locator('.t-table__body tr td').nth(1).innerText().catch(() => '')
    if (firstName) {
      await searchInput.fill(firstName.trim())
      await page.waitForTimeout(1200)
      const after = await siDrawer.locator('.t-table__body tr').evaluateAll((rows) =>
        rows.filter((r) => r.querySelectorAll('td').length > 3).length)
      check('4.11 抽屉搜索生效', after > 0 && after <= detailRows, `${detailRows} -> ${after}`)
      await searchInput.fill('')
      await page.waitForTimeout(800)
    }
  }
  // 险种筛选
  const schemeSel = siDrawer.locator('.hr-si-scheme').first()
  if (await schemeSel.count()) {
    await schemeSel.click()
    await page.waitForTimeout(700)
    const opt = page.locator('.t-select-option', { hasText: '养老' }).first()
    if (await opt.count()) {
      await opt.click()
      await page.waitForTimeout(1200)
      const after = await siDrawer.locator('.t-table__body tr').evaluateAll((rows) =>
        rows.filter((r) => r.querySelectorAll('td').length > 3).length)
      check('4.12 抽屉险种筛选生效', after > 0 && after <= detailRows, `${detailRows} -> ${after}`)
    } else {
      check('4.12 抽屉险种筛选生效', false, 'no option')
    }
  }
  await page.screenshot({ path: `${SHOTS}/r47-a4-detail.png` })
}
await page.keyboard.press('Escape')
await page.waitForTimeout(1000)

// 花名册个人社保账号回填
await clickKpi('花名册')
await page.waitForTimeout(3000)
const siBackfill = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('.t-table__header th')).map((t) => (t.textContent || '').trim())
  const i = heads.indexOf('个人社保账号')
  if (i < 0) return { hasCol: false }
  const rows = Array.from(document.querySelectorAll('.t-table__body tr')).filter((r) => r.querySelectorAll('td').length > 3)
  const values = rows.map((r) => (r.querySelectorAll('td')[i]?.innerText || '').trim()).filter((v) => v && v !== '—')
  return { hasCol: true, count: values.length, sample: values.slice(0, 3) }
})
if (siBackfill.hasCol) {
  check('4.13 花名册个人社保账号已自动回填', siBackfill.count > 0, JSON.stringify(siBackfill.sample))
} else {
  check('4.13 花名册个人社保账号已自动回填', false, '列未显示')
}

// ---------------------------------------------------------------------------
console.log('\n===== 汇总 =====')
const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|favicon|net::ERR_|网络错误，请检查您的网络连接|Failed to list models|Failed to load chat models|\[cmdk\] failed to load knowledge bases/.test(e))
check('5.1 控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 3).join(' | '))
console.log(`\nRESULT ${ok}/${total} PASS`)
if (failures.length) { console.log('FAILURES:'); failures.forEach((f) => console.log('  - ' + f)) }
await browser.close()
process.exit(failures.length ? 1 : 0)
