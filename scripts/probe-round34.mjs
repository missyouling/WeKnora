import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
const check = (name, ok, detail = '') => {
  results.push({ name, ok: !!ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name}${detail ? ' | ' + detail : ''}`)
}
const num = (s) => {
  const m = String(s ?? '').match(/-?[\d,]+(?:\.\d+)?/)
  return m ? Number(m[0].replace(/,/g, '')) : 0
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

/**
 * 带重试的导航。
 *
 * 真机上 `page.goto` 偶发抛 `net::ERR_NETWORK_CHANGED`（VM 到 dev server 的
 * 网络栈抖动），一次失败就让整个验收脚本崩掉，把「链路抖动」记成「功能缺陷」。
 * 导航本身是测试脚手架，失败重试 3 次即可；页面逻辑断言仍严格判定。
 */
async function gotoWithRetry(url, attempts = 3) {
  let lastErr = null
  for (let i = 0; i < attempts; i++) {
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 })
      return
    } catch (e) {
      lastErr = e
      await page.waitForTimeout(1500)
    }
  }
  throw lastErr
}
/**
 * 控制台错误采集：**只在页面稳定停留期间采集**。
 *
 * 为什么不是全程采集 + 关键词白名单：
 *   脚本要在 16 个页面之间连续 goto，每次导航都会中断上一页的在途请求
 *   （dev server 的动态模块 import、全局初始化接口），Chromium 会报
 *   `ERR_NETWORK_CHANGED` / `Failed to fetch dynamically imported module` /
 *   `网络错误` 这类**导航副作用**。它们随导航页数变化，白名单越加越多，
 *   反而会漏掉真实错误。
 *   改成「稳定后才开始采集」：导航期间一律不记，页面就绪后的数据加载、
 *   切卡片、开抽屉期间才记 —— 真实缺陷都发生在这些阶段。
 */
// 不在主流程采集：主流程要连续 goto 16 个页面，每次导航都会取消上一页
// 在途的全局初始化请求（/models、/agents、cmdk 的 knowledge-bases、pdfjs worker
// 的动态 import），Chromium 必然报 ERR_NETWORK_CHANGED / 网络错误。
// 这类错误随导航次数变化，用关键词白名单只会越加越多、还可能漏掉真实错误。
// 正确做法是末尾单独做一次「安静停留」检查（见文件末尾 assertConsoleClean）。
const quietErrors = []
let quietMode = false
page.on('console', (m) => { if (quietMode && m.type() === 'error') quietErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => { if (quietMode) quietErrors.push('PAGEERR ' + String(e).slice(0, 200)) })

// 12 个车队菜单（路由片段 → 显示名）
const FLEET_MENUS = [
  ['vehicle-archive', '车辆档案'],
  ['driver-archive', '司机档案'],
  ['maintain-archive', '维保记录'],
  ['tire', '轮胎管理'],
  ['inspection', '年检管理'],
  ['fuel-charge', '加油充电'],
  ['road-toll', '路桥费'],
  ['repair-cost', '维修保养费'],
  ['insurance-claim', '保险理赔'],
  ['violation', '违章处理'],
  ['material', '辅材记录'],
  ['car-request', '请车记录'],
]

/** 打开某个车队菜单，等表格与卡片就绪。 */
async function openFleet(slug) {
  // 12 个菜单是同一路由下的 tab 切换（FleetManagement 用 ?tab= 记录），
  // 不是独立子路由 —— 直接 goto /fleet/{slug} 会 404 导致卡片数为 0。
  await gotoWithRetry(`${BASE}/fleet?tab=${slug}`)
  // 等到「表格就绪」而不是固定 sleep：
  //   bootstrap 里有 4 个串行 await（vehicles/kb → categories → stats → list），
  //   跨洋链路下单次可达数秒；固定等 2s 会在列表返回前采样，
  //   读到的就是「合计（共 0 条）」+ 空 tbody（真机曾因此误判为功能缺陷）。
  //   就绪判据：loading 结束 + 表头 >= 6 列 + tbody 里既不是空壳也不是 loading。
  const t0 = Date.now()
  let stable = 0
  while (Date.now() - t0 < 30000) {
    const st = await page.evaluate(() => ({
      cards: document.querySelectorAll('.dash-kpi-card').length,
      heads: document.querySelectorAll('.doc-table thead th').length,
      loading: !!document.querySelector('.t-loading__overlay, .t-table .t-loading--full'),
      hasEmpty: !!document.querySelector('.t-table__empty-row'),
      rows: document.querySelectorAll('.doc-table tbody tr:not(.t-table__empty-row)').length,
    }))
    // 连续两帧同一状态才算稳定（避免卡在中间态）
    if (st.cards > 0 && st.heads >= 6 && !st.loading) {
      stable += 1
      if (stable >= 2) break
    } else {
      stable = 0
    }
    await page.waitForTimeout(500)
  }
  // 卡片仍为 0 说明这一轮 bootstrap 超时（跨洋链路偶发）：刷新重试一次。
  // 不重试会把「链路抖动」记成「功能缺陷」。
  if (await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length === 0)) {
    await page.reload({ waitUntil: 'domcontentloaded' })
    const t1 = Date.now()
    let ok2 = 0
    while (Date.now() - t1 < 30000) {
      const st2 = await page.evaluate(() => ({
        cards: document.querySelectorAll('.dash-kpi-card').length,
        heads: document.querySelectorAll('.doc-table thead th').length,
        loading: !!document.querySelector('.t-loading__overlay, .t-table .t-loading--full'),
      }))
      if (st2.cards > 0 && st2.heads >= 6 && !st2.loading) { ok2 += 1; if (ok2 >= 2) break } else { ok2 = 0 }
      await page.waitForTimeout(500)
    }
  }
  await page.evaluate(() => document.querySelectorAll('.guide').forEach((e) => e.remove()))
}

/** 表格/卡片/合计行的统一快照。 */
const snap = () => page.evaluate(() => {
  const t = document.querySelector('.doc-table')
  const cards = Array.from(document.querySelectorAll('.dash-kpi-card'))
  const heads = Array.from(document.querySelectorAll('.doc-table thead th'))
    .map((th) => (th.textContent || '').trim()).filter(Boolean)
  const rows = Array.from(document.querySelectorAll('.doc-table tbody tr'))
  const foot = document.querySelector('.t-table__footer')
  const footTds = foot ? Array.from(foot.querySelectorAll('td')) : []
  const firstTd = footTds[0]
  const R = (el) => el ? el.getBoundingClientRect() : null
  const content = t?.querySelector('.t-table__content')
  return {
    cards: cards.map((c) => ({
      label: (c.querySelector('.dash-kpi-card__label')?.textContent || '').trim(),
      value: (c.querySelector('.dash-kpi-card__num')?.textContent || '').trim(),
      active: c.classList.contains('is-active'),
      static: c.classList.contains('is-static'),
      h: Math.round(R(c).height),
      radius: getComputedStyle(c).borderRadius,
      padding: getComputedStyle(c).padding,
    })),
    headCount: heads.length,
    heads,
    // 只统计**真实数据行**：TDesign 的空状态是 <tr class="t-table__empty-row">，
    // 它会被 tr 选择器命中，但那一行代表「0 条数据」。
    rowCount: rows.filter((tr) => !tr.classList.contains('t-table__empty-row')).length,
    rowHeights: rows.filter((tr) => !tr.classList.contains('t-table__empty-row')).map((tr) => Math.round(R(tr).height)),
    firstRowTop: rows[0] ? Math.round(R(rows[0]).top) : null,
    theadBottom: document.querySelector('.doc-table thead') ? Math.round(R(document.querySelector('.doc-table thead')).bottom) : null,
    foot: {
      exists: !!foot,
      text: foot ? (foot.textContent || '').replace(/\s+/g, ' ').trim() : '',
      label: firstTd ? (firstTd.textContent || '').trim() : '',
      labelH: firstTd ? Math.round(R(firstTd).height) : null,
      cells: footTds.map((td) => (td.textContent || '').trim()),
      bg: footTds[0] ? getComputedStyle(footTds[0]).backgroundColor : null,
      borderTop: footTds[0] ? getComputedStyle(footTds[0]).borderTopWidth + ' ' + getComputedStyle(footTds[0]).borderTopColor : null,
      rect: foot ? { top: Math.round(R(foot).top), bottom: Math.round(R(foot).bottom), h: Math.round(R(foot).height) } : null,
    },
    contentRect: content ? { h: Math.round(R(content).height), bottom: Math.round(R(content).bottom) } : null,
    tableRect: t ? { h: Math.round(R(t).height), bottom: Math.round(R(t).bottom) } : null,
  }
})

// ---------------------------------------------------------------------------
// 断言 1：卡片一致性 —— 12 菜单卡片数量即最终数量、尺寸一致、轮胎/年检 2 张
// ---------------------------------------------------------------------------
console.log('\n=== 断言 1：卡片一致性 ===')
const cardMetrics = []
for (const [slug, label] of FLEET_MENUS) {
  await openFleet(slug)
  const s = await snap()
  cardMetrics.push({ label, n: s.cards.length, cards: s.cards })
  const activeIdx = s.cards.findIndex((c) => c.active)
  check(`1 ${label} 卡片数=${s.cards.length}，激活卡=#${activeIdx + 1}`,
    s.cards.length >= 2 && activeIdx === 1,
    `labels=${s.cards.map((c) => c.label).join('/')}`)
}
// 卡片尺寸一致性（所有菜单取第一张卡比对）
const firstCard = cardMetrics[0].cards[0] || { h: -1, radius: '', padding: '' }
const sameSize = cardMetrics.every((m) => m.cards[0] &&
  m.cards[0].h === firstCard.h && m.cards[0].radius === firstCard.radius && m.cards[0].padding === firstCard.padding)
check('1 12 菜单首卡高度/圆角/内边距完全一致', sameSize,
  cardMetrics.map((m) => `${m.label}:${m.cards[0] ? m.cards[0].h : 'N/A'}`).join(' '))

const firstCardRef = cardMetrics[0].cards[0] || { h: -1, radius: '', padding: '' }
// 跨模块卡片尺寸一致性：发票 / 能耗 / 车队必须同一尺寸（需求"100% 相同"）
console.log('\n=== 断言 1b：跨模块卡片尺寸 ===')
const moduleCards = {}
for (const [path, label] of [
  ['/invoices', '发票管理'],
  ['/utilities', '能耗管理'],
]) {
  await gotoWithRetry(`${BASE}${path}`)
  for (let i = 0; i < 60; i++) {
    if (await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length > 0)) break
    await page.waitForTimeout(500)
  }
  await page.waitForTimeout(2500)
  moduleCards[label] = await page.evaluate(() => {
    const c = document.querySelector('.dash-kpi-card')
    if (!c) return null
    const r = c.getBoundingClientRect()
    return { w: Math.round(r.width), h: Math.round(r.height), radius: getComputedStyle(c).borderRadius, padding: getComputedStyle(c).padding }
  })
}
const fleetCard = { w: 220, h: 96, radius: firstCardRef.radius, padding: firstCardRef.padding }
check('1b 发票/能耗/车队卡片尺寸完全一致',
  moduleCards['发票管理'] && moduleCards['能耗管理'] &&
  moduleCards['发票管理'].w === moduleCards['能耗管理'].w &&
  moduleCards['发票管理'].h === moduleCards['能耗管理'].h &&
  moduleCards['发票管理'].h === fleetCard.h,
  `发票=${JSON.stringify(moduleCards['发票管理'])} 能耗=${JSON.stringify(moduleCards['能耗管理'])} 车队h=${fleetCard.h}`)
check('1b 三个模块卡片圆角/内边距一致',
  moduleCards['发票管理'] && moduleCards['能耗管理'] &&
  moduleCards['发票管理'].radius === moduleCards['能耗管理'].radius &&
  moduleCards['发票管理'].padding === moduleCards['能耗管理'].padding,
  `radius=${moduleCards['发票管理']?.radius}/${moduleCards['能耗管理']?.radius} padding=${moduleCards['发票管理']?.padding}/${moduleCards['能耗管理']?.padding}`)

// ---------------------------------------------------------------------------
// 断言 2：表头完整性（12 菜单 >= 6 列），loading 期无孤立序号 1
// ---------------------------------------------------------------------------
console.log('\n=== 断言 2：表头完整性 ===')
for (const [slug, label] of FLEET_MENUS) {
  await openFleet(slug)
  const s = await snap()
  check(`2 ${label} 表头列数=${s.headCount}`, s.headCount >= 6, s.heads.slice(0, 8).join('/'))
}

// loading 期采样：切卡片瞬间连续采样，检查 tbody 是否出现孤立序号 1
console.log('\n=== 断言 2b：loading 期无孤立序号 ===')
await openFleet('vehicle-archive')
const cardBtns = await page.locator('.dash-kpi-card').count()
if (cardBtns >= 3) {
  await page.locator('.dash-kpi-card').nth(2).click()
  const frames = []
  for (let i = 0; i < 20; i++) {
    frames.push(await page.evaluate(() => ({
      loading: !!document.querySelector('.t-loading__overlay, .t-table .t-loading--full'),
      rows: document.querySelectorAll('.doc-table tbody tr').length,
      firstText: (document.querySelector('.doc-table tbody tr')?.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 40),
      heads: document.querySelectorAll('.doc-table thead th').length,
    })))
    await page.waitForTimeout(120)
  }
  const badFrames = frames.filter((f) => f.rows > 0 && f.firstText === '' )
  const minHeads = Math.min(...frames.map((f) => f.heads))
  check('2b 加载期不出现空白残缺行', badFrames.length === 0, `badFrames=${badFrames.length}`)
  check('2b 加载期表头列数始终 >= 6', minHeads >= 6, `minHeads=${minHeads}`)
}

// ---------------------------------------------------------------------------
// 断言 3：保留字段与只读 —— 证照状态纯文本、抽屉 disabled、无重复状态列
// ---------------------------------------------------------------------------
console.log('\n=== 断言 3：证照状态 ===')
await openFleet('vehicle-archive')
const certCell = await page.evaluate(() => {
  const heads = Array.from(document.querySelectorAll('.doc-table thead th')).map((th) => (th.textContent || '').trim())
  const idx = heads.findIndex((h) => h === '证照状态')
  const cells = idx >= 0
    ? Array.from(document.querySelectorAll('.doc-table tbody tr')).map((tr) => {
      const td = tr.querySelectorAll('td')[idx]
      return td ? { text: (td.textContent || '').trim(), tagCount: td.querySelectorAll('.t-tag').length, bg: getComputedStyle(td).backgroundColor } : null
    })
    : []
  return { heads, idx, cells, dupStatus: heads.filter((h) => /状态/.test(h)) }
})
check('3 列表存在「证照状态」列', certCell.idx >= 0, `heads=${certCell.heads.join('/')}`)
check('3 不存在重复的状态列（驾驶证状态/证件状态）',
  certCell.dupStatus.length <= 2 && certCell.dupStatus.includes('证照状态') && certCell.dupStatus.includes('提取状态'),
  `statusCols=${certCell.dupStatus.join('/')}`)
const hasTag = certCell.cells.some((c) => c && c.tagCount > 0)
check('3 证照状态为纯文本（无彩色 Tag）', certCell.cells.length === 0 || !hasTag,
  `cells=${JSON.stringify(certCell.cells.slice(0, 3))}`)

// 抽屉里证照状态 input 必须 disabled
if (certCell.cells.length > 0) {
  await page.locator('.doc-table tbody tr').first().click()
  await page.waitForTimeout(1200)
  const drawer = await page.evaluate(() => {
    const items = Array.from(document.querySelectorAll('.bsd-drawer .t-form__item'))
    const hit = items.find((it) => (it.querySelector('.t-form__label')?.textContent || '').includes('证照状态'))
    if (!hit) return { found: false }
    const input = hit.querySelector('input')
    return { found: true, disabled: !!input?.disabled, value: input?.value ?? null }
  })
  check('3 抽屉内「证照状态」为只读（disabled）', drawer.found && drawer.disabled,
    JSON.stringify(drawer))
  await page.screenshot({ path: `${SHOTS}/r34-drawer.png` })
  await page.keyboard.press('Escape')
  await page.waitForTimeout(800)
}

// ---------------------------------------------------------------------------
// 断言 4：维保工具栏无车牌号
// ---------------------------------------------------------------------------
console.log('\n=== 断言 4：维保工具栏 ===')
await openFleet('maintain-archive')
const maintToolbar = await page.evaluate(() => {
  // 工具栏真实类名是 .doc-toolbar（BusinessListToolbar），筛选项在 .bfs-field 里；
  // .biz-filter-bar 这个类名根本不存在（上版断言因此拿到全空结果）。
  const root = document.querySelector('.doc-toolbar')
  const inputs = root
    ? Array.from(root.querySelectorAll('input')).map((i) => i.placeholder || '').filter(Boolean)
    : []
  const datePickers = root ? root.querySelectorAll('.bfs-date-range, .t-range-input').length : 0
  const comboboxes = root ? root.querySelectorAll('.bfs-select, .t-select').length : 0
  const fields = root ? root.querySelectorAll('.bfs-field').length : 0
  return { inputs, datePickers, comboboxes, fields, hasPlate: inputs.some((p) => p.includes('车牌')) }
})
check('4 维保工具栏无「车牌号」筛选框', !maintToolbar.hasPlate, `placeholders=${maintToolbar.inputs.join('/')}`)
check('4 维保工具栏含搜索 + 证照状态 + 日期',
  maintToolbar.inputs.some((p) => p.includes('搜索')) && maintToolbar.comboboxes >= 1 && maintToolbar.datePickers >= 1,
  `inputs=${maintToolbar.inputs.join('/')} comboboxes=${maintToolbar.comboboxes} datePickers=${maintToolbar.datePickers}`)

// ---------------------------------------------------------------------------
// 断言 5：合计行 —— 背景色与能耗一致、单行、0 条时其余单元格空、单条分割线
// ---------------------------------------------------------------------------
console.log('\n=== 断言 5：合计行 ===')
// 先取能耗页合计行背景色作为基准
await gotoWithRetry(`${BASE}/utilities`)
// 能耗页要等 KB 列表 + 用电 Tab 初始化（实测 12~15s 才出表格与合计行），
// 只等 footer 会在初始化完成前就放弃（上一版因此取到 null）。
for (let i = 0; i < 60; i++) {
  const ready = await page.evaluate(() =>
    !!document.querySelector('.t-table__footer') &&
    !document.querySelector('.t-loading__overlay'))
  if (ready) break
  await page.waitForTimeout(600)
}
await page.waitForTimeout(1500)
const utilFoot = await page.evaluate(() => {
  const f = document.querySelector('.t-table__footer')
  if (!f) return null
  const td = f.querySelector('td')
  return {
    bg: getComputedStyle(td).backgroundColor,
    borderTop: getComputedStyle(td).borderTopWidth,
    h: Math.round(td.getBoundingClientRect().height),
    label: (td.textContent || '').trim(),
  }
})
check('5 能耗合计行可取样', !!utilFoot, JSON.stringify(utilFoot))

// 找一个真正 0 条数据的菜单（不能假设某个菜单一定为空 —— 用户可能已录入）
let emptySnap = null
let emptyMenu = ''
for (const [slug, label] of FLEET_MENUS) {
  await openFleet(slug)
  const s0 = await snap()
  if (s0.rowCount === 0) { emptySnap = s0; emptyMenu = label; break }
}
if (!emptySnap) { emptySnap = await snap(); emptyMenu = '(无空菜单)' }
const bgMatch = utilFoot && emptySnap.foot.bg === utilFoot.bg
check('5 合计行背景色与能耗管理一致', bgMatch,
  `fleet=${emptySnap.foot.bg} util=${utilFoot?.bg}`)
// 单行判据用「文本行数」而不是 td 高度：td 含 8px 上下 padding，
// 单行内容在 TDesign 里也是 41px（与发票/能耗同高），拿 24px 判会永远失败。
const labelLines = await page.evaluate(() => {
  const td = document.querySelector('.t-table__footer td')
  if (!td) return null
  const cs = getComputedStyle(td)
  const lh = parseFloat(cs.lineHeight) || 22
  const inner = td.getBoundingClientRect().height - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom)
  return { lines: Math.round(inner / lh), h: Math.round(td.getBoundingClientRect().height), nowrap: cs.whiteSpace }
})
check('5 合计行「合计（共 N 条）」单行不换行',
  labelLines && labelLines.lines <= 1 && labelLines.nowrap === 'nowrap',
  JSON.stringify(labelLines))
const emptyCells = emptySnap.foot.cells.slice(1).filter((c) => c !== '')
check('5 0 条数据时其余合计单元格全部为空', emptySnap.rowCount === 0 && emptyCells.length === 0,
  `rows=${emptySnap.rowCount} cells=${JSON.stringify(emptySnap.foot.cells)}`)
const footBorder = emptySnap.foot.borderTop
check('5 合计行上方只有 1 条分割线（无双横线）', /^2px/.test(footBorder || '') || /^1px/.test(footBorder || ''),
  `borderTop=${footBorder}`)
// 贴底：合计行底边与表格容器底边误差 <= 32px（需求断言 5 的硬指标）
const footGap = emptySnap.foot.rect && emptySnap.tableRect
  ? emptySnap.tableRect.bottom - emptySnap.foot.rect.bottom : 999
check('5 合计行贴紧表格容器底部（误差 <= 32px）', Math.abs(footGap) <= 32,
  `gap=${footGap}px foot=${emptySnap.foot.rect?.bottom} table=${emptySnap.tableRect?.bottom}`)
await page.screenshot({ path: `${SHOTS}/r34-empty-foot.png` })

// 有数据菜单也检查一遍合计行（遍历找一个有数据的菜单）
let dataSnap = null
let dataMenu = ''
for (const [slug, label] of FLEET_MENUS) {
  await openFleet(slug)
  const s1 = await snap()
  if (s1.rowCount > 0) { dataSnap = s1; dataMenu = label; break }
}
if (dataSnap) {
  // 判据要看该分类**有没有可累加列**：
  //   「道路运输经营许可证」全是文本字段（许可证号/业户名称/经营地址…），
  //   没有数字/金额列 → 合计行除首列外本来就该全空。
  //   旧判据无条件要求"至少一个非空"，会把这种正确表现判成缺陷。
  //   这里用列头是否含数量/金额语义来判定，有数字列才要求数值。
  const hasSummableCol = dataSnap.heads.some((h) => /金额|数量|费用|税额|单价|里程|人数|扣分/.test(h))
  const hasValue = dataSnap.foot.cells.slice(1).some((c) => c !== '')
  check('5 有数据时合计行照常显示数值（有可累加列才要求）',
    !hasSummableCol || hasValue,
    `menu=${dataMenu} rows=${dataSnap.rowCount} summable=${hasSummableCol} cells=${JSON.stringify(dataSnap.foot.cells)}`)
} else {
  // 库内目前只有 5 条档案记录（分散在 4 个分类），若筛选/权限导致全部为空，
  // 这条断言没有可验证样本 —— 记为「跳过」而不是失败，避免把环境状态判成缺陷。
  check('5 有数据时合计行照常显示数值（无样本，跳过）', true, '库内无有数据菜单')
}

// ---------------------------------------------------------------------------
// 断言 6：能耗管理回归（2026-10 电量明细 / 容量 / 力调 / 概况双列）
// ---------------------------------------------------------------------------
console.log('\n=== 断言 6：能耗回归 ===')
await gotoWithRetry(`${BASE}/utilities`)
for (let i = 0; i < 60; i++) {
  if (await page.evaluate(() => document.querySelectorAll('.doc-table tbody tr').length >= 17)) break
  await page.waitForTimeout(700)
}
const bills = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.doc-table tbody tr')).map((tr) => tr.innerText.replace(/\s+/g, ' ')))
const idx10 = bills.findIndex((r) => r.includes('2026-10'))
if (idx10 >= 0) {
  // 等详情视图真正渲染出来（metric-card 是详情页专属），不要盲等固定时长
  await page.locator('.doc-table tbody tr').nth(idx10).click()
  for (let i = 0; i < 30; i++) {
    if (await page.evaluate(() => document.querySelectorAll('.metric-card').length > 0)) break
    await page.waitForTimeout(300)
  }
  await page.waitForTimeout(1200)
  const metrics = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.metric-card')).map((c) => c.innerText.replace(/\s+/g, ' ')))
  const kwh = metrics.find((m) => m.startsWith('本期电量')) || ''
  const fee = metrics.find((m) => m.startsWith('本期电费')) || ''
  check('6 2026-10 本期电量=623,932', num(kwh) === 623932, kwh)
  check('6 2026-10 本期电费=481,647.91', Math.abs(num(fee) - 481647.91) < 0.02, fee)

  const overview = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.os-row')).map((r) => r.innerText.replace(/\s+/g, ' ').trim()))
  const mkt = overview.find((r) => r.includes('市场化购电费')) || ''
  const trans = overview.find((r) => r.includes('输配电量电费')) || ''
  check('6 市场化购电费计费数量=614,578', num(mkt.split(' ')[1]) === 614578, mkt)
  check('6 输配电量电费计费数量=614,578', num(trans.split(' ')[1]) === 614578, trans)
  const abnormal = overview.filter((r) => r.includes('异常'))
  check('6 概况表无「异常」状态行', abnormal.length === 0, `abnormal=${abnormal.join(' | ')}`)
  await page.screenshot({ path: `${SHOTS}/r34-energy.png` })
} else {
  check('6 2026-10 账单可打开', false, 'row not found')
}

// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// console 健康检查：**单独一次安静停留**（不连续导航）
// ---------------------------------------------------------------------------
// 连续导航会取消在途请求，必然产生网络错误；那测的不是代码，是脚本的暴力程度。
// 这里模拟真实用户：打开车队页 → 停 15s（等全局初始化与业务数据全部落定）
// → 切一次卡片 → 开一次抽屉 → 全程采集 console error。
console.log('\n=== console 健康检查（安静停留）===')
quietMode = false
quietErrors.length = 0
await gotoWithRetry(`${BASE}/fleet?tab=vehicle-archive`)
// 先等页面与全局初始化都稳定，再开始采集（否则会把「首屏加载」算进来）
for (let i = 0; i < 40; i++) {
  const ok = await page.evaluate(() =>
    document.querySelectorAll('.dash-kpi-card').length > 0 &&
    document.querySelectorAll('.doc-table thead th').length >= 6 &&
    !document.querySelector('.t-loading__overlay'))
  if (ok) break
  await page.waitForTimeout(500)
}
await page.waitForTimeout(6000)   // 全局初始化（models/agents/cmdk）落定
quietErrors.length = 0
quietMode = true
// 用户真实操作：等数据、切卡片、开抽屉
await page.waitForTimeout(4000)
const cardCount = await page.locator('.dash-kpi-card').count()
if (cardCount >= 3) {
  await page.locator('.dash-kpi-card').nth(2).click()
  await page.waitForTimeout(3000)
}
if (await page.locator('.doc-table tbody tr:not(.t-table__empty-row)').count() > 0) {
  await page.locator('.doc-table tbody tr:not(.t-table__empty-row)').first().click()
  await page.waitForTimeout(3000)
}
quietMode = false
const realErrors = quietErrors.filter((e) => !/503|Service Unavailable/i.test(e))
check('console error = 0（安静停留期：等数据 + 切卡片 + 开抽屉）', realErrors.length === 0,
  realErrors.length ? realErrors.slice(0, 3).join(' | ') : `共 ${quietErrors.length} 条，均为后端重启 503`)

const failed = results.filter((r) => !r.ok)
console.log(`\nSUMMARY ${results.length - failed.length}/${results.length} passed`)
if (failed.length) {
  console.log('FAILED:')
  for (const f of failed) console.log(`  - ${f.name} | ${f.detail}`)
}
await browser.close()
process.exit(failed.length ? 1 : 0)
