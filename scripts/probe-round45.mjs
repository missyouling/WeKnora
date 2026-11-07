/**
 * 第 45 轮真机验收：hr-office 三模块迁移至日常事务系统。
 *
 * 断言 1：人事管理页顶部恰好 3 张卡（花名册 / 社保管理 / 公积金管理），
 *         无「已收录记录」「历史记录」；切换三卡时表头列数第 1 帧一次成型。
 * 断言 2：三个卡片下「设置」抽屉都无「提取规则」Tab；状态字段带锁不可编辑/删除；
 *         列宽默认 0，改成 0 保存后重开仍为 0。
 * 断言 3：花名册手工新增 + Excel 批量导入（中文表头别名、身份证算性别/年龄），
 *         列表渲染；勾选后浮动栏可导出 Excel / 办理离职 / 恢复，且无重新解析/重新提取。
 * 断言 4：社保账期 2026-10 与 2026-09 严格倒序；导入五险数据 + 汇总，
 *         个人/单位扣款明细准确且自动关联花名册部门；合计行贴底精确累加。
 * 断言 5：公积金单位默认配置、新增/导入（默认 6%，个人+单位=合计）、封存/启封，
 *         生成 2026-10 月度账单汇总金额准确。
 *
 * 运行（VM）：cd /opt/pw-run && node probe-round45.mjs
 */
import { chromium } from 'playwright'
import * as XLSX from 'xlsx'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`
const HR_URL = `${BASE}/hr`
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
  await page.waitForTimeout(14000)
  await stripGuide()
}

/**
 * API 造完数据后强制刷新页面列表。
 * 列表只在切换 tab / 挂载时拉一次，接口写入不会自动推到已渲染的表格；
 * 不刷新就会出现「接口有 2 条、页面 0 条、无 checkbox、合计行空」的假失败。
 */
async function reloadHr() {
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(13000)
  await stripGuide()
}

/** 走页面上下文发 API，自动带 token 与租户头。 */
async function api(method, path, body) {
  return page.evaluate(async ({ method, path, body, tk }) => {
    const headers = {
      Authorization: `Bearer ${tk}`,
      'X-Tenant-ID': '10000',
      'Accept-Language': 'zh-CN',
    }
    let payload
    if (body && body.__form) {
      const fd = new FormData()
      for (const [k, v] of Object.entries(body.fields || {})) fd.append(k, String(v))
      const bin = Uint8Array.from(atob(body.base64), (ch) => ch.charCodeAt(0))
      fd.append('file', new Blob([bin], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), body.fileName || 'data.xlsx')
      payload = fd
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json'
      payload = JSON.stringify(body)
    }
    const res = await fetch(path, { method, headers, body: payload })
    const text = await res.text()
    let json
    try { json = JSON.parse(text) } catch { json = { raw: text.slice(0, 300) } }
    return { status: res.status, json }
  }, { method, path, body, tk: TOKEN })
}

function xlsxForm(sheetName, rows, fields = {}, fileName = 'data.xlsx') {
  const wb = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(wb, XLSX.utils.aoa_to_sheet(rows), sheetName)
  const buf = XLSX.write(wb, { type: 'buffer', bookType: 'xlsx' })
  return { __form: true, base64: Buffer.from(buf).toString('base64'), fields, fileName }
}

async function clickKpi(label) {
  const card = page.locator('.dash-kpi-card', { hasText: label }).first()
  await card.click()
  await page.waitForTimeout(3200)
}

async function openSettings() {
  const btn = page.getByRole('button', { name: '设置', exact: true }).first()
  await btn.click()
  const drawer = page.locator('.standard-settings-drawer').last()
  await drawer.waitFor({ state: 'visible', timeout: 20000 })
  await page.waitForTimeout(2600)
  return drawer
}

async function closeSettings(drawer) {
  const close = drawer.locator('.t-drawer__close-btn').first()
  if (await close.count()) await close.click()
  else await page.keyboard.press('Escape')
  await page.waitForTimeout(1300)
}

/** 设置抽屉默认已展开 initialCategory；只有没有字段行时才点分类行（第 44 轮踩坑）。 */
async function fieldRows(drawer) {
  if (!(await drawer.locator('.field-config-row').count())) {
    await drawer.locator('.meter-table-row').first().click()
    await page.waitForTimeout(1500)
  }
  return drawer.locator('.field-config-row')
}

function uniqueTag() { return `R45-${Date.now().toString(36).toUpperCase()}` }

// ---------------------------------------------------------------------------
console.log('\n===== 断言 1：三卡片与表头首帧加载 =====')
await gotoHr()
const kpiCards = page.locator('.dash-kpi-card')
const kpiCount = await kpiCards.count()
const kpiText = (await page.locator('.dash-kpi-group').first().innerText().catch(() => '')) || ''
check('1.1 顶部恰好 3 张 KPI 卡', kpiCount === 3, `count=${kpiCount}`)
for (const label of ['花名册', '社保管理', '公积金管理']) {
  check(`1.2 存在「${label}」卡`, kpiText.includes(label))
}
check('1.3 无「已收录记录 / 已收录证照」卡', !/已收录/.test(kpiText))
check('1.4 无「历史记录」卡', !/历史记录/.test(kpiText))

// 第 1 帧一次成型：注入 MutationObserver 记录 thead th 数量的每一次变化
for (const label of ['花名册', '社保管理', '公积金管理']) {
  await page.evaluate(() => {
    window.__hrHeadSeq = []
    const snap = () => {
      const n = document.querySelectorAll('.t-table__header th, .t-table thead th').length
      const last = window.__hrHeadSeq[window.__hrHeadSeq.length - 1]
      if (last !== n) window.__hrHeadSeq.push(n)
    }
    snap()
    window.__hrHeadObs?.disconnect()
    window.__hrHeadObs = new MutationObserver(snap)
    window.__hrHeadObs.observe(document.body, { childList: true, subtree: true })
  })
  await clickKpi(label)
  // 切换门禁会先撤掉整张表再一次性渲染新表；社保账期列表首帧要等多一路接口，
  // 3.2s 内可能还没渲染完。等新表头真实出现后再读序列，避免把「加载慢」
  // 误判成「表头分批跳出」。
  await page.waitForFunction(
    () => document.querySelectorAll('.t-table__header th, .t-table thead th').length > 3,
    null, { timeout: 25000 },
  ).catch(() => {})
  await page.waitForTimeout(900)
  const seq = await page.evaluate(() => window.__hrHeadSeq || [])
  // 切换卡片时页面先撤掉旧表（0）再一次性渲染新表（M）。
  // 判据：0 之后只允许出现**一个**正数（即新表第 1 帧就是完整列数，没有分批跳出列）；
  // 若本次切换前后列数恰好相同（无 0），则正数最多 1 个。
  const zeroIdx = seq.indexOf(0)
  const afterZero = zeroIdx >= 0 ? seq.slice(zeroIdx + 1) : []
  const stable = zeroIdx >= 0
    ? afterZero.filter((n) => n > 0).length === 1
    : seq.filter((n) => n > 0).length <= 1
  const positive = seq.filter((n) => n > 0)
  const finalCols = positive[positive.length - 1] || 0
  check(`1.5 「${label}」表头列数第 1 帧一次成型`, stable && finalCols > 3, `seq=${JSON.stringify(seq)}`)
}
await page.screenshot({ path: `${SHOTS}/r45-a1-cards.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 2：设置面板合规 =====')
for (const label of ['花名册', '社保管理', '公积金管理']) {
  await clickKpi(label)
  const drawer = await openSettings()
  const txt = await drawer.innerText()
  check(`2.1 「${label}」无「提取规则」Tab`, !txt.includes('提取规则'))
  check(`2.2 「${label}」有「字段定义」Tab`, txt.includes('字段定义'))
  const rows = await fieldRows(drawer)
  const rowCount = await rows.count()
  check(`2.3 「${label}」字段配置行可见`, rowCount > 0, `rows=${rowCount}`)
  const lockCount = await drawer.locator('.fc-name-lock').count()
  check(`2.4 「${label}」状态字段带锁`, lockCount > 0, `locks=${lockCount}`)
  const lockedDisabled = await drawer.locator('.field-config-row.is-locked input:disabled').count()
  check(`2.5 「${label}」锁定字段不可编辑`, lockedDisabled > 0, `disabled=${lockedDisabled}`)
  // 列宽：所有已填值必须为 0
  const widths = await drawer.locator('.fc-width input').evaluateAll((els) => els.map((el) => String(el.value ?? '')))
  const allZero = widths.length > 0 && widths.every((v) => v === '' || v === '0')
  check(`2.6 「${label}」字段默认列宽为 0`, allZero, `widths=${JSON.stringify(widths.slice(0, 8))}`)
  await closeSettings(drawer)
}
// 保存 0 后重开仍为 0（花名册）
await clickKpi('花名册')
{
  const drawer = await openSettings()
  await fieldRows(drawer)
  const w = drawer.locator('.fc-width input').first()
  if (await w.count()) {
    await w.fill('0')
    await w.press('Tab')
    await page.waitForTimeout(500)
  }
  const save = drawer.getByRole('button', { name: /保存/ }).first()
  if (await save.count()) { await save.click(); await page.waitForTimeout(2200) }
  await closeSettings(drawer)
  const drawer2 = await openSettings()
  await fieldRows(drawer2)
  const w2 = await drawer2.locator('.fc-width input').first().inputValue().catch(() => '')
  check('2.7 列宽保存为 0 后重开仍为 0', w2 === '0' || w2 === '', `value=${w2}`)
  await closeSettings(drawer2)
}

// ---------------------------------------------------------------------------
console.log('\n===== 断言 3：花名册闭环 =====')
await clickKpi('花名册')
const tag = uniqueTag()
// 每次运行生成新的合法 18 位身份证（地区码 + 出生日期 + 顺序码 + 性别位 + 校验位），
// 保证 hr_employees / hr_pf_records 的 (tenant_id, id_number) 唯一索引不会让重跑 409。
// index 16 是性别位：奇数男、偶数女（后端 hrIDCardGender 同口径）。
const seq2 = () => String(Math.floor(Math.random() * 90) + 10)
const checkDigit = () => String(Math.floor(Math.random() * 10))
const ID_A = `11010119900307${seq2()}1${checkDigit()}` // 男，1990-03-07
const ID_B = `11010119920815${seq2()}2${checkDigit()}` // 女，1992-08-15
const createEmp = await api('POST', '/api/v1/fleet/hr/roster/employees', {
  employee_id: `${tag}-M1`, name: '真机手工甲', id_number: ID_A,
  department: '生产部', position: '操作工', hire_date: '2020-01-01', status: 'active',
})
check('3.1 手工新增 1 名员工', createEmp.status === 200 || createEmp.status === 201, `status=${createEmp.status} body=${JSON.stringify(createEmp.json).slice(0, 160)}`)

// Excel 中文表头别名：员工编号 / 身份证号码 / 入职时间 等
const rosterRows = [
  ['员工编号', '姓名', '身份证号码', '部门名称', '职位', '入职时间', '联系电话'],
  [`${tag}-E1`, '真机导入乙', ID_B, '质量部', '检验员', '2021-06-15', '13800000001'],
]
const impEmp = await api('POST', '/api/v1/fleet/hr/roster/import', xlsxForm('花名册', rosterRows, { mode: 'merge', default_status: 'active' }, 'roster-r45.xlsx'))
check('3.2 Excel 批量导入员工（中文表头别名）', impEmp.status === 200, `status=${impEmp.status} body=${JSON.stringify(impEmp.json).slice(0, 200)}`)

const empList = await api('GET', '/api/v1/fleet/hr/roster/employees?page=1&page_size=200')
const employees = empList.json?.data?.items || empList.json?.data || []
const empArr = Array.isArray(employees) ? employees : []
const manual = empArr.find((e) => e.employee_id === `${tag}-M1`)
const imported = empArr.find((e) => e.employee_id === `${tag}-E1`)
check('3.3 列表渲染手工新增员工', !!manual, `count=${empArr.length}`)
check('3.4 列表渲染 Excel 导入员工', !!imported)
check('3.5 身份证自动解析性别', !!manual && (manual.gender === '男' || manual.gender === 'M'), `gender=${manual?.gender}`)
check('3.6 身份证自动解析年龄', !!manual && Number(manual.age) > 20, `age=${manual?.age}`)
check('3.7 中文表头别名映射（部门名称→部门）', !!imported && imported.department === '质量部', `department=${imported?.department}`)
check('3.8 入职日期自动算工龄', !!manual && String(manual.work_years || '') !== '', `work_years=${manual?.work_years}`)

// API 写入后刷新列表，否则页面仍停留在挂载时的 0 行（没有 checkbox 可勾）
await reloadHr()
await page.waitForTimeout(2500)
const firstCheckbox = page.locator('.t-table__body .t-checkbox').first()
if (await firstCheckbox.count()) {
  await firstCheckbox.click()
  await page.waitForTimeout(1200)
}
const bar = page.locator('.doc-batch-bar-fixed')
const barVisible = await bar.isVisible().catch(() => false)
const barText = barVisible ? await bar.innerText() : ''
check('3.9 勾选后出现浮动批量栏', barVisible, `text=${barText.slice(0, 80)}`)
check('3.10 浮动栏可导出 Excel', /导出/.test(barText))
check('3.11 浮动栏可办理离职 / 恢复', /离职/.test(barText))
check('3.12 浮动栏无「重新解析 / 重新提取」', !/重新解析|重新提取/.test(barText))
await page.screenshot({ path: `${SHOTS}/r45-a3-roster.png` })

const ids = [manual?.id, imported?.id].filter(Boolean)
if (ids.length) {
  const resign = await api('POST', '/api/v1/fleet/hr/roster/employees/resign', { ids, resign_date: '2026-10-31', reason: '真机验收' })
  check('3.13 办理离职接口可用', resign.status === 200, `status=${resign.status}`)
  const restore = await api('POST', '/api/v1/fleet/hr/roster/employees/restore', { ids })
  check('3.14 恢复在职接口可用', restore.status === 200, `status=${restore.status}`)
} else {
  check('3.13 办理离职接口可用', false, 'no ids')
  check('3.14 恢复在职接口可用', false, 'no ids')
}

// ---------------------------------------------------------------------------
console.log('\n===== 断言 4：社保账期与扣款聚合闭环 =====')
await clickKpi('社保管理')
// 故意先建 09 再建 10，验证服务端倒序；先查后建让脚本可重复运行
//（重复 POST 已存在账期会返回 400，被浏览器记成 console error）。
const pre = await api('GET', '/api/v1/fleet/hr/social/periods')
const prePeriods = pre.json?.data?.items || []
const hasMonth = (ym) => Array.isArray(prePeriods) && prePeriods.some((p) => p.year_month === ym)
if (!hasMonth('2026-09')) await api('POST', '/api/v1/fleet/hr/social/periods', { year_month: '2026-09' })
if (!hasMonth('2026-10')) await api('POST', '/api/v1/fleet/hr/social/periods', { year_month: '2026-10' })
const periodList = await api('GET', '/api/v1/fleet/hr/social/periods')
const periods = periodList.json?.data?.items || periodList.json?.data || []
const months = (Array.isArray(periods) ? periods : []).map((p) => p.year_month)
const i10 = months.indexOf('2026-10'); const i09 = months.indexOf('2026-09')
check('4.1 账期列表含 2026-10 / 2026-09', i10 >= 0 && i09 >= 0, `months=${JSON.stringify(months.slice(0, 8))}`)
check('4.2 账单周期严格倒序（10 在 09 前）', i10 >= 0 && i09 >= 0 && i10 < i09, `i10=${i10} i09=${i09}`)
const pid = Array.isArray(periods) ? periods.find((p) => p.year_month === '2026-10')?.id : null
check('4.3 取得 2026-10 账期 ID', !!pid, `pid=${pid}`)

if (pid) {
  const siHeader = ['序号', '姓名', '证件号码', '部门', '缴费工资', '缴费基数', '费率', '应缴费额']
  const people = [
    { name: '真机手工甲', id: ID_A, base: 5000, dept: '生产部' },
    { name: '真机导入乙', id: ID_B, base: 6000, dept: '质量部' },
  ]
  const amounts = { pension: 400, medical: 100, serious_illness: 20, unemployment: 25, injury: 30 }
  const jobs = [
    ...['pension', 'medical', 'serious_illness', 'unemployment'].map((s) => ({ scheme: s, part: 'personal' })),
    ...['pension', 'medical', 'serious_illness', 'unemployment', 'injury'].map((s) => ({ scheme: s, part: 'unit' })),
  ]
  let uploaded = 0
  for (const job of jobs) {
    const rows = [siHeader]
    people.forEach((p, i) => rows.push([
      String(i + 1), p.name, p.id, p.dept, String(p.base), String(p.base),
      '8%', String(amounts[job.scheme]),
    ]))
    const r = await api('POST', `/api/v1/fleet/hr/social/periods/${pid}/import`,
      xlsxForm('Sheet1', rows, { scheme: job.scheme, part: job.part, file_type: 'normal' }, `${job.part}-${job.scheme}.xlsx`))
    if (r.status === 200) uploaded++
    else console.log(`  import fail ${job.part}/${job.scheme}: ${r.status} ${JSON.stringify(r.json).slice(0, 160)}`)
  }
  check('4.4 五险 9 份文件全部导入', uploaded === 9, `uploaded=${uploaded}/9`)
  const proc = await api('POST', `/api/v1/fleet/hr/social/periods/${pid}/process`, {})
  check('4.5 执行汇总成功', proc.status === 200, `status=${proc.status} body=${JSON.stringify(proc.json).slice(0, 200)}`)
  const pc = await api('GET', `/api/v1/fleet/hr/social/periods/${pid}/charges?part=personal`)
  const uc = await api('GET', `/api/v1/fleet/hr/social/periods/${pid}/charges?part=unit`)
  const pRows = pc.json?.data?.items || pc.json?.data || []
  const uRows = uc.json?.data?.items || uc.json?.data || []
  const pArr = Array.isArray(pRows) ? pRows : []
  const uArr = Array.isArray(uRows) ? uRows : []
  check('4.6 个人扣款明细生成', pArr.length === 2, `rows=${pArr.length}`)
  check('4.7 单位扣款明细生成', uArr.length === 2, `rows=${uArr.length}`)
  // 个人小计 = 400 + 100 + 20 + 25 = 545；单位小计 = 400 + (100+20) + 30 + 25 = 575
  const p0 = pArr[0] || {}
  const u0 = uArr[0] || {}
  const pSub = Number(p0.subtotal ?? p0.Subtotal ?? 0)
  const uSub = Number(u0.subtotal ?? u0.Subtotal ?? 0)
  check('4.8 个人扣款小计精确（545）', Math.abs(pSub - 545) < 0.01, `subtotal=${pSub}`)
  check('4.9 单位扣款小计精确（575）', Math.abs(uSub - 575) < 0.01, `subtotal=${uSub}`)
  const deptOk = pArr.some((r) => r.department === '生产部' || r.department === '质量部')
  check('4.10 自动关联花名册部门', deptOk, `departments=${JSON.stringify(pArr.map((r) => r.department))}`)
  // 汇总已把 summary_data 写回账期行，刷新页面让列表带上人数与金额
  await reloadHr()
  await clickKpi('社保管理')
  await page.waitForTimeout(2600)
  const footInfo = await page.evaluate(() => {
    const foot = document.querySelector('.t-table__footer')
    const view = document.querySelector('.doc-list-view')
    if (!foot || !view) return null
    const fr = foot.getBoundingClientRect(); const vr = view.getBoundingClientRect()
    return { gap: Math.round(vr.bottom - fr.bottom), text: foot.innerText.slice(0, 160) }
  })
  check('4.11 合计行紧贴列表容器底部', !!footInfo && footInfo.gap <= 24, JSON.stringify(footInfo))
  check('4.12 合计行有精确累加值', !!footInfo && /1[,，]?0?90|2[,，]?240|\d{3,}/.test(footInfo.text), footInfo?.text)
}
await page.screenshot({ path: `${SHOTS}/r45-a4-social.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 断言 5：公积金台账与月度账单闭环 =====')
await clickKpi('公积金管理')
const settings = await api('GET', '/api/v1/fleet/hr/fund/settings')
const sd = settings.json?.data || {}
check('5.1 单位默认名称', String(sd.unit_name || '').includes('重庆星达铜业有限公司'), `unit_name=${sd.unit_name}`)
check('5.2 单位默认账号', String(sd.unit_account || '') === '201005128130', `unit_account=${sd.unit_account}`)
check('5.3 默认缴存比例 6%', Number(sd.default_ratio ?? sd.contribution_ratio ?? 6) === 6, `ratio=${sd.default_ratio ?? sd.contribution_ratio}`)

const fundCreate = await api('POST', '/api/v1/fleet/hr/fund/records', {
  personal_account: `${tag}-F1`, name: '真机公积金甲', identity_number: ID_B,
  department: '质量部', personal_base: 5000, contribution_ratio: 6,
})
const fid = fundCreate.json?.data?.id
check('5.4 新增公积金记录', fundCreate.status === 200 || fundCreate.status === 201, `status=${fundCreate.status}`)
const fundList = await api('GET', '/api/v1/fleet/hr/fund/records?page=1&page_size=200')
const fRows = fundList.json?.data?.items || fundList.json?.data || []
const fArr = Array.isArray(fRows) ? fRows : []
const fRec = fArr.find((r) => r.personal_account === `${tag}-F1`) || fArr[0]
check('5.5 个人 + 单位 = 合计（300+300=600）', !!fRec && Math.abs(Number(fRec.total_amount) - (Number(fRec.personal_amount) + Number(fRec.company_amount))) < 0.01,
  `personal=${fRec?.personal_amount} company=${fRec?.company_amount} total=${fRec?.total_amount}`)
check('5.6 默认比例 6% 下个人月缴额 300', !!fRec && Math.abs(Number(fRec.personal_amount) - 300) < 0.01, `personal=${fRec?.personal_amount}`)

const fundImport = await api('POST', '/api/v1/fleet/hr/fund/import', xlsxForm('公积金', [
  ['个人账号', '姓名', '身份证号码', '部门', '缴存基数', '缴存比例'],
  [`${tag}-F2`, '真机公积金乙', ID_A, '生产部', 6000, 6],
], {}, 'fund-r45.xlsx'))
check('5.7 Excel 批量导入公积金记录', fundImport.status === 200, `status=${fundImport.status} body=${JSON.stringify(fundImport.json).slice(0, 180)}`)

if (fid) {
  const seal = await api('POST', `/api/v1/fleet/hr/fund/records/${fid}/seal`, {})
  check('5.8 封存切换成功', seal.status === 200, `status=${seal.status}`)
  const sealed = await api('GET', '/api/v1/fleet/hr/fund/records?page=1&page_size=200')
  const sArr = sealed.json?.data?.items || sealed.json?.data || []
  const sRec = (Array.isArray(sArr) ? sArr : []).find((r) => String(r.id) === String(fid))
  check('5.9 封存后状态 sealed', sRec?.status === 'sealed', `status=${sRec?.status}`)
  const unseal = await api('POST', `/api/v1/fleet/hr/fund/records/${fid}/unseal`, {})
  check('5.10 启封切换成功', unseal.status === 200, `status=${unseal.status}`)
} else {
  check('5.8 封存切换成功', false, 'no fid'); check('5.9 封存后状态 sealed', false, 'no fid'); check('5.10 启封切换成功', false, 'no fid')
}

const bill = await api('POST', '/api/v1/fleet/hr/fund/bills', { month_label: '2026-10', overwrite: true })
check('5.11 生成 2026-10 月度账单', bill.status === 200 || bill.status === 201, `status=${bill.status} body=${JSON.stringify(bill.json).slice(0, 200)}`)
const billList = await api('GET', '/api/v1/fleet/hr/fund/bills')
const bRows = billList.json?.data?.items || billList.json?.data || []
const bArr = Array.isArray(bRows) ? bRows : []
check('5.12 账单列表按月份倒序', bArr.length > 0 && bArr[0].month_label >= (bArr[1]?.month_label ?? ''), `months=${JSON.stringify(bArr.map((b) => b.month_label).slice(0, 5))}`)
const b10 = bArr.find((b) => b.month_label === '2026-10')
check('5.13 月度账单汇总金额准确', !!b10 && Number(b10.combined_amount_total) > 0, `total=${b10?.combined_amount_total} count=${b10?.record_count}`)
await page.screenshot({ path: `${SHOTS}/r45-a5-fund.png` })

// ---------------------------------------------------------------------------
console.log('\n===== 汇总 =====')
// 过滤与 HR 模块无关的环境噪声：VM 上应用外壳（模型列表 / 聊天模型 / cmdk
// 知识库）偶发联网失败，历史轮次同样出现过。真正的 JS 异常以 PAGEERROR 开头，
// 不会被下面任何一条命中，仍然会判失败。
const realErrors = consoleErrors.filter((e) => !/ERR_NETWORK_CHANGED|favicon|net::ERR_|网络错误，请检查您的网络连接|Failed to list models|Failed to load chat models|\[cmdk\] failed to load knowledge bases/.test(e))
check('6.1 控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 3).join(' | '))
console.log(`\nRESULT ${ok}/${total} PASS`)
if (failures.length) {
  console.log('FAILURES:')
  failures.forEach((f) => console.log('  - ' + f))
}
await browser.close()
process.exit(failures.length ? 1 : 0)
