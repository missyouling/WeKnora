/**
 * 第 44 轮真机验收。
 *
 * 断言 1：车队 12 个菜单顶部不再有「已收录证照 / 已收录记录」卡；
 *         三个档案菜单最右侧有「历史记录」卡，手工录入类没有。
 * 断言 2：车辆档案 → 道路运输证 勾选第 1 行，「重新解析 / 重新提取」可点（disabled=false）。
 * 断言 3：车辆档案 / 发票管理 / 市电账单 的字段定义里，状态字段锁定
 *         （名称 input disabled、类型 select disabled、启用开关锁定、无删除、有拖拽手柄）。
 * 断言 4：轮胎管理 / 车辆档案 设置里把列宽改成 0 保存，刷新后仍是 0；新增字段默认 0。
 * 断言 5：轮胎管理 / 年检管理 / 加油充电 设置抽屉无「提取规则」Tab；车辆档案有。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`
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

async function goto(path) {
  await page.goto(`${BASE}${path}`, { waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(14000)
  await stripGuide()
}

async function clickFleetMenu(label) {
  const item = page.locator('.fleet-side-item', { hasText: new RegExp(`^\\s*${label}\\s*$`) }).first()
  await item.click()
  await page.waitForTimeout(3500)
}

async function openSettings() {
  const btn = page.getByRole('button', { name: '设置', exact: true }).first()
  await btn.click()
  const drawer = page.locator('.standard-settings-drawer').last()
  await drawer.waitFor({ state: 'visible', timeout: 20000 })
  await page.waitForTimeout(2500)
  return drawer
}

async function closeSettings(drawer) {
  const close = drawer.locator('.t-drawer__close-btn').first()
  if (await close.count()) await close.click()
  else await page.keyboard.press('Escape')
  await page.waitForTimeout(1200)
}

/** 字段配置行（等抽屉里出现，必要时点分类行展开）。 */
async function fieldRows(drawer) {
  if (!(await drawer.locator('.field-config-row').count())) {
    await drawer.locator('.meter-table-row').first().click()
    await page.waitForTimeout(1500)
  }
  return drawer.locator('.field-config-row')
}

/** 读一行字段配置的锁定状态。 */
async function readRow(row) {
  return row.evaluate((e) => {
    const nameInput = e.querySelector('.fc-name input')
    const typeInput = e.querySelector('.fc-type input')
    const widthInput = e.querySelector('.fc-width input')
    const switches = [...e.querySelectorAll('.t-switch')]
    return {
      name: (nameInput?.value || '').trim(),
      nameDisabled: !!nameInput?.disabled,
      typeDisabled: !!typeInput?.disabled,
      width: widthInput?.value ?? '',
      hasDrag: !!e.querySelector('.fc-drag'),
      hasLockIcon: !!e.querySelector('.fc-name-lock'),
      hasDelete: !!e.querySelector('.biz-del-btn'),
      hasLockCell: !!e.querySelector('.fc-used'),
      switches: switches.map((s) => s.className.includes('t-is-checked') ? 'on' : 'off'),
      switchesDisabled: switches.map((s) => s.className.includes('t-is-disabled')),
    }
  })
}

async function findRowByName(drawer, names) {
  const rows = await fieldRows(drawer)
  const count = await rows.count()
  for (let i = 0; i < count; i++) {
    const info = await readRow(rows.nth(i))
    if (names.includes(info.name)) return { row: rows.nth(i), info }
  }
  return null
}

// ---------------------------------------------------------------------------
// 断言 1：12 个菜单不再有「已收录」卡
// ---------------------------------------------------------------------------
await goto('/fleet')
const ARCHIVE_MENUS = ['车辆档案', '司机档案', '维保记录']
const MANUAL_MENUS = ['轮胎管理', '年检管理', '加油充电', '路桥费', '维修保养费', '保险理赔', '违章处理', '辅材记录', '请车记录']
for (const menu of [...ARCHIVE_MENUS, ...MANUAL_MENUS]) {
  await clickFleetMenu(menu)
  const labels = await page.$$eval('.dash-kpi-card', (els) => els.map((e) => (e.innerText || '').split('\n')[0].trim()))
  const hasCollected = labels.some((l) => /已收录/.test(l))
  check(`断言1 ${menu} 无「已收录」卡`, labels.length > 0 && !hasCollected, `cards=${labels.join('/')}`)
  if (ARCHIVE_MENUS.includes(menu)) {
    check(`断言1 ${menu} 最右侧有「历史记录」卡`, labels[labels.length - 1] === '历史记录', `last=${labels[labels.length - 1]}`)
    check(`断言1 ${menu} 首张为子分类卡（默认焦点）`, !/历史记录/.test(labels[0] || '') && labels.length > 1, `first=${labels[0]}`)
  } else {
    check(`断言1 ${menu} 无「历史记录」卡`, !labels.includes('历史记录'), `cards=${labels.join('/')}`)
  }
}

// ---------------------------------------------------------------------------
// 断言 2：勾选记录后「重新解析 / 重新提取」可用
// ---------------------------------------------------------------------------
await clickFleetMenu('车辆档案')
await page.locator('.dash-kpi-card', { hasText: '道路运输证' }).first().click()
await page.waitForTimeout(4000)
const firstRow = page.locator('.t-table tbody tr').first()
await firstRow.waitFor({ timeout: 30000 })
await firstRow.locator('.t-checkbox').first().click()
await page.waitForTimeout(1200)
const bar = page.locator('.doc-batch-bar-fixed')
check('断言2 浮动批量栏出现', (await bar.count()) === 1)
const retryParse = bar.getByRole('button', { name: '重新解析' }).first()
const retryExtract = bar.getByRole('button', { name: '重新提取' }).first()
check('断言2 重新解析可点', (await retryParse.count()) === 1 && !(await retryParse.isDisabled()))
check('断言2 重新提取可点', (await retryExtract.count()) === 1 && !(await retryExtract.isDisabled()))
// 真点一次「重新解析」：必须弹出二次确认（不确认，避免真重跑模型）
await retryParse.click()
await page.waitForTimeout(800)
const confirmVisible = await page.locator('.t-popconfirm').filter({ hasText: '确定重新解析所选' }).count()
check('断言2 点击重新解析弹出确认框（证明按钮真的可触发）', confirmVisible > 0)
await page.keyboard.press('Escape')
await page.waitForTimeout(500)
await page.screenshot({ path: `${SHOTS}/r44-batch-bar.png` })

// 手工台账：勾选后不应出现重新解析/重新提取
await clickFleetMenu('轮胎管理')
const tireRow = page.locator('.t-table tbody tr').first()
if (await tireRow.count()) {
  await tireRow.locator('.t-checkbox').first().click()
  await page.waitForTimeout(1000)
  const manualBar = page.locator('.doc-batch-bar-fixed')
  if (await manualBar.count()) {
    check('断言2 手工台账无「重新解析」', (await manualBar.getByRole('button', { name: '重新解析' }).count()) === 0)
    check('断言2 手工台账无「重新提取」', (await manualBar.getByRole('button', { name: '重新提取' }).count()) === 0)
    check('断言2 手工台账保留「打印目录 / 删除」',
      (await manualBar.getByRole('button', { name: '打印目录' }).count()) === 1 &&
      (await manualBar.getByRole('button', { name: '删除' }).count()) === 1)
  }
}

// ---------------------------------------------------------------------------
// 断言 3：状态字段全局加锁
// ---------------------------------------------------------------------------
async function assertStatusLock(where, names) {
  const drawer = await openSettings()
  const found = await findRowByName(drawer, names)
  if (!found) {
    check(`断言3 ${where} 存在 ${names.join('/')} 行`, false, '未找到状态字段')
    await closeSettings(drawer)
    return
  }
  const { info } = found
  check(`断言3 ${where} ${info.name} 名称输入锁定`, info.nameDisabled === true)
  check(`断言3 ${where} ${info.name} 数据类型锁定`, info.typeDisabled === true)
  check(`断言3 ${where} ${info.name} 有锁形图标`, info.hasLockIcon === true)
  check(`断言3 ${where} ${info.name} 不可删除（锁图标占位）`, info.hasLockCell === true && info.hasDelete === false)
  check(`断言3 ${where} ${info.name} 拖拽手柄可用`, info.hasDrag === true)
  check(`断言3 ${where} ${info.name} 启用字段锁定且开启`,
    info.switches[1] === 'on' && info.switchesDisabled[1] === true,
    `switches=${info.switches.join(',')} disabled=${info.switchesDisabled.join(',')}`)
  await page.screenshot({ path: `${SHOTS}/r44-lock-${where.replace(/[^\w\u4e00-\u9fa5]/g, '')}.png` })
  await closeSettings(drawer)
}

await clickFleetMenu('车辆档案')
await assertStatusLock('车辆档案', ['证照状态', '证件状态', '保单状态', '驾驶证状态'])

await goto('/invoices')
await page.waitForTimeout(3000)
await assertStatusLock('发票管理', ['提取状态', 'extractStatus'])

await goto('/utilities')
await page.waitForTimeout(3000)
await assertStatusLock('市电账单', ['提取状态', 'extractStatus'])

// ---------------------------------------------------------------------------
// 断言 4：列宽 0 保存持久化 & 新增默认为 0
// ---------------------------------------------------------------------------
async function widthPersistence(where) {
  await goto('/fleet')
  if (where !== '车辆档案') await clickFleetMenu(where)
  let drawer = await openSettings()
  let rows = await fieldRows(drawer)
  const count = await rows.count()
  // 先把第 1 行改成 240 并保存，证明「非 0 也能原样落库」
  const firstWidth = rows.first().locator('.fc-width input')
  await firstWidth.fill('240')
  await firstWidth.press('Enter')
  await drawer.getByRole('button', { name: '保存', exact: true }).first().click()
  await page.waitForTimeout(3000)
  rows = await fieldRows(drawer)
  const afterSave = await rows.first().locator('.fc-width input').inputValue()
  check(`断言4 ${where} 自定义列宽 240 保存成功`, afterSave === '240', `实际=${afterSave}`)

  // 全部改成 0 并保存
  const n = await rows.count()
  for (let i = 0; i < n; i++) {
    const input = rows.nth(i).locator('.fc-width input')
    if (!(await input.count())) continue
    await input.fill('0')
    await input.press('Enter')
  }
  await drawer.getByRole('button', { name: '保存', exact: true }).first().click()
  await page.waitForTimeout(3500)
  await closeSettings(drawer)

  // 刷新页面后重新打开
  await page.reload({ waitUntil: 'domcontentloaded' })
  await page.waitForTimeout(14000)
  await stripGuide()
  if (where !== '车辆档案') await clickFleetMenu(where)
  drawer = await openSettings()
  rows = await fieldRows(drawer)
  const n2 = await rows.count()
  const widths = []
  for (let i = 0; i < n2; i++) {
    const v = await rows.nth(i).locator('.fc-width input').inputValue().catch(() => '')
    widths.push(v)
  }
  const nonZero = widths.filter((w) => w !== '0' && w !== '')
  check(`断言4 ${where} 刷新后列宽严格为 0`, nonZero.length === 0, `异常值=${nonZero.join(',')} 共${n2}行`)

  // 新增字段默认列宽 0（不保存，取消退出，避免污染配置）
  const addInput = drawer.locator('.field-config-add .fc-add-input input').first()
  await addInput.fill('验收测试字段')
  await drawer.getByRole('button', { name: '添加', exact: true }).first().click()
  await page.waitForTimeout(1200)
  const added = await findRowByName(drawer, ['验收测试字段'])
  check(`断言4 ${where} 新增字段存在`, !!added)
  if (added) {
    check(`断言4 ${where} 新增字段初始列宽为 0`, added.info.width === '0', `实际=${added.info.width}`)
  }
  await drawer.getByRole('button', { name: '取消', exact: true }).first().click()
  await page.waitForTimeout(800)
  await page.screenshot({ path: `${SHOTS}/r44-width-${where}.png` })
  return drawer
}

let drawerAfter = await widthPersistence('轮胎管理')
await closeSettings(drawerAfter)
drawerAfter = await widthPersistence('车辆档案')
await closeSettings(drawerAfter)

// ---------------------------------------------------------------------------
// 断言 5：纯手工模块无「提取规则」Tab
// ---------------------------------------------------------------------------
async function tabsOf(menu) {
  await goto('/fleet')
  await clickFleetMenu(menu)
  const drawer = await openSettings()
  const tabs = await drawer.locator('.t-tabs__nav-item').allInnerTexts()
  const text = tabs.map((t) => t.trim()).filter(Boolean)
  await closeSettings(drawer)
  return text
}

for (const menu of ['轮胎管理', '年检管理', '加油充电']) {
  const tabs = await tabsOf(menu)
  check(`断言5 ${menu} 无「提取规则」Tab`, !tabs.includes('提取规则'), `tabs=${tabs.join('/')}`)
  check(`断言5 ${menu} 有「字段定义」Tab`, tabs.includes('字段定义'), `tabs=${tabs.join('/')}`)
}
for (const menu of ['车辆档案', '维保记录']) {
  const tabs = await tabsOf(menu)
  check(`断言5 ${menu} 有「提取规则」Tab`, tabs.includes('提取规则'), `tabs=${tabs.join('/')}`)
}

// 控制台错误（忽略网络层的偶发 401/资源 404）
const realErrors = consoleErrors.filter((e) => !/Failed to load resource|net::ERR|ResizeObserver/.test(e))
check('控制台无 JS 错误', realErrors.length === 0, realErrors.slice(0, 3).join(' || '))

await browser.close()
console.log(`\n===== 第 44 轮：${ok}/${total} PASS =====`)
if (failures.length) {
  console.log('FAILURES:')
  failures.forEach((f) => console.log(' - ' + f))
}
process.exit(ok === total ? 0 : 1)
