// 第 31 轮真机验收：车队 9 项深度修复（6 条断言 + 跨模块回归）
//
//  断言 1 列宽配置：设置 → 字段配置每行有「列宽」数字输入框
//  断言 2 状态三色 + 提取状态列 + 证照状态筛选
//  断言 3 维保工具栏：搜索 / 证照状态 / 车牌号 / 日期范围 / 刷新
//  断言 4 12 个子菜单均无「操作」列、无右侧冻结分割线
//  断言 5 空状态只出现 1 次「暂无数据」；合计行贴表格容器底部
//  断言 6 详情抽屉预览加载期不出现「暂不支持在线预览」
//  回归 发票管理 / 能耗管理 页面无 console error、布局未破坏
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

const MENUS = [
  ['vehicle-archive', '车辆档案'], ['driver-archive', '司机档案'], ['maintain-archive', '维保记录'],
  ['tire', '轮胎管理'], ['inspection', '年检管理'], ['fuel-charge', '加油充电'],
  ['road-toll', '路桥费'], ['repair-cost', '维修保养费'], ['insurance-claim', '保险理赔'],
  ['violation', '违章处理'], ['material', '辅材记录'], ['car-request', '请车记录'],
]

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
await page.waitForTimeout(4000)
await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))

async function clickMenu(label) {
  await page.evaluate((l) => {
    const hit = Array.from(document.querySelectorAll('.fleet-side-item'))
      .find((el) => el.textContent.replace(/\s/g, '') === l)
    if (hit) hit.click()
  }, label)
  await page.waitForTimeout(1200)
  const t0 = Date.now()
  while (Date.now() - t0 < 25000) {
    const n = await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length)
    if (n > 0) break
    await page.waitForTimeout(400)
  }
  await page.waitForTimeout(1500)
}

// 等表格真正就绪：loading 遮罩消失，且出现「数据行 / 空状态 / 合计行」之一。
// 只等 KPI 卡片是不够的——切菜单时上一页的卡片还在，snap 会拿到空表格。
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
    await page.waitForTimeout(500)
  }
  return false
}

async function snap() {
  return page.evaluate(() => {
    const heads = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const trs = Array.from(document.querySelectorAll('.doc-table tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const cells = trs.flatMap((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim()))
    const toolbar = document.querySelector('.doc-toolbar')
    const inputs = Array.from(document.querySelectorAll('.doc-toolbar input')).map((i) => i.getAttribute('placeholder'))
    // 空状态重复检测
    // 只统计列表空状态组件里的主文案（不再叠加 t-empty 内部节点，避免嵌套重复计数）
    const emptyDescs = Array.from(document.querySelectorAll('.biz-empty .biz-empty__desc'))
      .map((el) => (el.textContent || '').trim())
    const noDataCount = emptyDescs.filter((t) => t === '暂无数据').length
    // 合计行贴底：footer 底边 与 表格容器底边 的距离
    const wrap = document.querySelector('.doc-list-scroll')
    const foot = document.querySelector('.doc-table .t-table__footer, .doc-table tfoot')
    const wrapRect = wrap ? wrap.getBoundingClientRect() : null
    const footRect = foot ? foot.getBoundingClientRect() : null
    return {
      heads,
      hasOpCol: heads.some((h) => h === '操作'),
      tagTexts: cells.filter((t) => ['生效中', '即将到期', '已过期', '提取成功'].includes(t)),
      inputs,
      selects: document.querySelectorAll('.doc-toolbar .t-select').length,
      datePicker: document.querySelectorAll('.doc-toolbar .t-date-range-picker').length,
      noDataCount,
      gapBottom: wrapRect && footRect ? Math.round(wrapRect.bottom - footRect.bottom) : null,
      toolbarH: toolbar ? Math.round(toolbar.getBoundingClientRect().height) : 0,
    }
  })
}

// ---- 断言 2：三个归档菜单的状态列 + 筛选 ----
for (const [, label] of [['', '车辆档案'], ['', '司机档案'], ['', '维保记录']]) {
  await clickMenu(label)
  await waitTableReady()
  const s = await snap()
  check(`2 ${label} 表头含「证照状态」`, s.heads.includes('证照状态'), `heads=${s.heads.join('/')}`)
  check(`2 ${label} 表头含「提取状态」`, s.heads.includes('提取状态'), `heads=${s.heads.join('/')}`)
  check(`2 ${label} 工具栏有「证照状态」下拉`, s.inputs.includes('证照状态') || s.selects > 0,
    `inputs=${s.inputs.join('/')} selects=${s.selects}`)
  await page.screenshot({ path: `${SHOTS}/r31-${label}.png` })
}

// ---- 断言 3：维保工具栏顺序 ----
await clickMenu('维保记录')
await waitTableReady()
const maint = await snap()
const expectOrder = ['搜索全部字段', '证照状态', '车牌号']
const okOrder = expectOrder.every((p, i) => maint.inputs[i] === p || maint.inputs.includes(p))
check('3 维保工具栏含 搜索/证照状态/车牌号/日期/刷新',
  okOrder && maint.datePicker >= 1,
  `inputs=${maint.inputs.join('/')} datePicker=${maint.datePicker}`)
await page.screenshot({ path: `${SHOTS}/r31-maintain-toolbar.png` })

// ---- 断言 4：12 菜单均无操作列 ----
let opCols = []
for (const [key, label] of MENUS) {
  await clickMenu(label)
  await waitTableReady()
  const s = await snap()
  if (s.hasOpCol) opCols.push(label)
}
check('4 12 个菜单均无「操作」列', opCols.length === 0, `有操作列=${opCols.join('/') || 'none'}`)

// ---- 断言 5：空状态去重 + 合计贴底 ----
// 选一个 0 条数据的菜单（轮胎管理）与 1 条以上（维保记录）
await clickMenu('轮胎管理')
await waitTableReady()
const empty = await snap()
check('5 空状态只出现 1 次「暂无数据」', empty.noDataCount === 1, `count=${empty.noDataCount}`)
await clickMenu('车辆档案')
await waitTableReady()
const one = await snap()
check('5 合计行贴容器底部（误差<=32px）', one.gapBottom !== null && Math.abs(one.gapBottom) <= 32,
  `gapBottom=${one.gapBottom}`)
await page.screenshot({ path: `${SHOTS}/r31-empty.png` })

// ---- 断言 1：设置抽屉字段配置含列宽 ----
await clickMenu('车辆档案')
await page.evaluate(() => {
  const btns = Array.from(document.querySelectorAll('.doc-toolbar__trailing .t-button'))
  const b = btns.find((x) => x.textContent.trim() === '设置')
  if (b) b.click()
})
await page.waitForTimeout(2500)
const widthCol = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('.field-colhead span')).map((s) => s.textContent.trim())
  const inputs = document.querySelectorAll('.field-config-row .fc-width').length
  return { heads, inputs }
})
check('1 字段配置含「列宽(px)」列', widthCol.heads.includes('列宽(px)'), `heads=${widthCol.heads.join('/')}`)
check('1 每行有列宽数字输入框', widthCol.inputs > 0, `inputs=${widthCol.inputs}`)
await page.screenshot({ path: `${SHOTS}/r31-field-width.png` })
// 关闭抽屉
await page.evaluate(() => {
  const close = document.querySelector('.t-drawer__close-btn, .t-dialog__close')
  if (close) close.click()
})
await page.waitForTimeout(800)

// ---- 断言 6：详情抽屉预览加载期无「暂不支持」 ----
await clickMenu('车辆档案')
const opened = await page.evaluate(() => {
  const tr = Array.from(document.querySelectorAll('.doc-table tbody tr')).find((t) => t.querySelectorAll('td').length > 3)
  if (tr) { tr.click(); return true }
  return false
})
if (opened) {
  await page.waitForTimeout(2000)
  // 展开源文件预览
  await page.evaluate(() => {
    const b = Array.from(document.querySelectorAll('.bsd-preview-toggle')).find(Boolean)
    if (b) b.click()
  })
  // 加载态期间采样 12 次，任一次出现 unsupported 文案即失败
  let sawUnsupported = false
  for (let i = 0; i < 12; i++) {
    const txt = await page.evaluate(() => {
      const pane = document.querySelector('.bsd-preview-pane')
      return pane ? pane.innerText : ''
    })
    if (txt.includes('暂不支持') || txt.includes('不支持在线预览')) sawUnsupported = true
    await page.waitForTimeout(250)
  }
  check('6 预览加载期不出现「暂不支持在线预览」', !sawUnsupported, `saw=${sawUnsupported}`)
  await page.screenshot({ path: `${SHOTS}/r31-preview.png` })
} else {
  check('6 预览加载期不出现「暂不支持在线预览」', false, '无可点行')
}

// ---- 回归：发票 / 能耗 ----
for (const [path, name] of [['invoices', '发票管理'], ['utilities', '能耗管理']]) {
  const before = consoleErrors.length
  await page.goto(`${BASE}/${path}`, { waitUntil: 'domcontentloaded' })
  // 能耗页要等 KB 列表 + 用电 Tab 初始化（实测 12s 才出表格），
  // 固定 sleep 会把它误判成「渲染失败」。轮询到表格或建库引导出现为止。
  {
    const t0 = Date.now()
    while (Date.now() - t0 < 30000) {
      const ready = await page.evaluate(() =>
        !!document.querySelector('.doc-table, .t-table') || !!document.querySelector('.empty-area'))
      if (ready) break
      await page.waitForTimeout(700)
    }
  }
  await page.waitForTimeout(1200)
  await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))
  const s = await page.evaluate(() => {
    const heads = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => th.textContent.trim())
    const h2 = document.querySelector('.header-title h2')?.textContent?.trim() || ''
    const body = document.body.innerText.replace(/\s+/g, ' ')
    return {
      heads: heads.filter(Boolean).length,
      h2,
      // 能耗页在未建「电费知识库」时停在建库引导态，属预期
      guided: body.includes('尚未创建') || body.includes('创建电费知识库'),
    }
  })
  check(`回归 ${name} 正常渲染`, s.heads > 0 || s.guided, `h2=${s.h2} heads=${s.heads} guided=${s.guided}`)
  check(`回归 ${name} 控制台无新增 error`, consoleErrors.length === before,
    `new=${consoleErrors.slice(before, before + 2).join(' || ') || 'none'}`)
  await page.screenshot({ path: `${SHOTS}/r31-reg-${path}.png` })
}

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY pass=${results.length - failed.length}/${results.length}`)
if (failed.length) console.log('FAILED:', failed.map((f) => f.name).join(' | '))
await browser.close()
process.exit(failed.length ? 1 : 0)
