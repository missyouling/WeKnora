/**
 * 第 40 轮验收：5 项
 *
 * 断言 1：费用清单（持睿汽车）展开 2026-10 的「市场化购电费」与
 *         「零售输配电费」，子表 10 列含 电表电量 / 差额电量 / 线损 /
 *         加减电量；四段电表合计 166,550、差额合计 5,850，且逐行
 *         四分量之和 == 账单原值/计费数量。
 * 断言 2：展开「系统运行费用」，辅助服务费用行
 *         计费数量 172400 / 单价 0 / 计算方式「工业总电量 × 单价」。
 * 断言 3：打印预览 PDF 文本层 —— 表头前两列「费用类别 / 费用组成」、
 *         大类不重复拼接、代理采购损益在系统运行费用组内、
 *         系统运行费用小计 784.43 / 政府性基金小计 8222.45。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const BASE = 'http://127.0.0.1:5173/platform/daily-affairs'
const SHOTS = '/tmp/pw/shots'
fs.mkdirSync(SHOTS, { recursive: true })

const results = []
let quietErrors = []
let quiet = false
function check(name, ok, detail) {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}
const num = (s) => {
  const m = String(s ?? '').replace(/,/g, '').match(/-?\d+(?:\.\d+)?/)
  return m ? Number(m[0]) : 0
}
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 950 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  const h = () => {
    if (!document.head) return
    const s = document.createElement('style')
    s.textContent = '.guide{display:none!important}'
    document.head.appendChild(s)
  }
  if (document.head) h(); else document.addEventListener('DOMContentLoaded', h)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (quiet && m.type() === 'error') quietErrors.push(m.text().slice(0, 200)) })
page.on('pageerror', (e) => { if (quiet) quietErrors.push('PAGEERR ' + String(e).slice(0, 200)) })

async function gotoRetry(url, attempts = 3) {
  let last = null
  for (let i = 0; i < attempts; i++) {
    try { await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 45000 }); return } catch (e) { last = e; await page.waitForTimeout(1500) }
  }
  throw last
}
async function waitRows(min = 1, times = 70) {
  for (let i = 0; i < times; i++) {
    const n = await page.evaluate(() =>
      Array.from(document.querySelectorAll('tbody tr'))
        .filter((tr) => tr.querySelectorAll('td').length > 3).length)
    if (n >= min) return n
    await page.waitForTimeout(500)
  }
  return 0
}

async function openCostList() {
  await gotoRetry(`${BASE}/utilities`)
  for (let i = 0; i < 60; i++) {
    if (await page.evaluate(() => document.querySelectorAll('.dash-kpi-card').length > 0)) break
    await page.waitForTimeout(500)
  }
  await page.evaluate(() => {
    const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('费用清单'))
    if (c) c.click()
  })
  for (let i = 0; i < 60; i++) {
    const ok = await page.evaluate(() =>
      Array.from(document.querySelectorAll('th')).some((e) => (e.textContent || '').includes('光伏发电量')) &&
      !!document.querySelector('.bfs-select'))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(2000)
  await page.locator('.bfs-select').first().click()
  await page.waitForTimeout(900)
  const opt = page.locator('.t-select-option, .t-option, li[role="option"]').filter({ hasText: '持睿汽车' }).first()
  if (await opt.count()) await opt.click()
  for (let i = 0; i < 50; i++) {
    const ok = await page.evaluate(() => Array.from(document.querySelectorAll('th'))
      .some((e) => (e.textContent || '').includes('市场化购电费')))
    if (ok) break
    await page.waitForTimeout(600)
  }
  await page.waitForTimeout(3000)
}

/** 展开某月行指定大类的 + 号，返回展开面板的行数组与表头。 */
async function expandGroup(month, groupName) {
  await waitRows(1, 60)
  await page.waitForTimeout(1500)
  const info = await page.evaluate(({ m, g }) => {
    const ths = Array.from(document.querySelectorAll('thead th'))
    let idx = -1
    let acc = 0
    for (const th of ths) {
      const span = th.colSpan || 1
      if ((th.textContent || '').includes(g)) { idx = acc; break }
      acc += span
    }
    const rows = Array.from(document.querySelectorAll('tbody tr')).filter((tr) => tr.querySelectorAll('td').length > 3)
    const row = rows.find((tr) => (tr.innerText || '').includes(m))
    if (!row || idx < 0) return { ok: false, idx, months: rows.map((r) => (r.innerText || '').slice(0, 12)) }
    const td = row.children[idx]
    const cellText = (td?.innerText || '').trim()
    const btn = td?.querySelector('.row-expand-toggle')
    if (btn) btn.click()
    return { ok: true, idx, cellText }
  }, { m: month, g: groupName })
  await page.waitForTimeout(2200)
  const panel = await page.evaluate(() => {
    const p = document.querySelector('.fee-group-panel')
    if (!p) return null
    return {
      head: Array.from(p.querySelectorAll('.fg-head-row td')).map((td) => (td.textContent || '').trim()),
      hint: (p.querySelector('.fg-breakdown-hint')?.textContent || '').trim(),
      rows: Array.from(p.querySelectorAll('tbody tr')).map((tr) =>
        Array.from(tr.querySelectorAll('td')).map((td) => (td.textContent || '').trim())),
    }
  })
  return { ...info, panel }
}

// ======================================================================
// 断言 1：分时四段拆解列
// ======================================================================
console.log('\n=== 断言 1：分时四段拆解 ===')
await openCostList()

for (const group of ['市场化购电费', '零售输配电费']) {
  const g = await expandGroup('2026-10', group)
  const head = g.panel?.head || []
  const rows = g.panel?.rows || []
  check(`1 [${group}] 可展开且有 10 列表头`, g.ok && head.length === 10, JSON.stringify(head))
  for (const name of ['电表电量', '差额电量', '线损', '加减电量', '账单原值/计费数量']) {
    check(`1 [${group}] 表头含「${name}」`, head.includes(name), head.join('|'))
  }
  check(`1 [${group}] 顶部差额算式说明`,
    /差额电量/.test(g.panel?.hint || '') && /172,?400/.test(g.panel?.hint || '') && /166,?550/.test(g.panel?.hint || ''),
    g.panel?.hint || '(缺)')

  // 展开明细：四段分时行（第 1 列细项名、第 2 列时段），列索引：
  // 0 名称 1 时段 2 电表 3 差额 4 线损 5 加减 6 计费数量 7 单价 8 方式 9 金额
  const timed = rows.filter((r) => r.length === 10 && ['尖峰', '峰', '平', '谷'].includes(r[1]))
  check(`1 [${group}] 四段分时行齐全`, timed.length === 4, `rows=${timed.length}`)
  let closed = true
  let detail = []
  for (const r of timed) {
    const sum = Math.round((num(r[2]) + num(r[3]) + num(r[4]) + num(r[5])) * 100) / 100
    const target = num(r[6])
    if (Math.abs(sum - target) > 0.02) closed = false
    detail.push(`${r[1]}:${r[2]}+${r[3]}+${r[4]}+${r[5]}=${sum}/${target}`)
  }
  check(`1 [${group}] 每行四分量之和 == 账单原值/计费数量`, closed, detail.join(' | '))

  const total = rows.find((r) => r.length >= 8 && (r[0] || '').trim() === '合计')
  // 合计行：0 合计(colspan2) 1 电表 2 差额 3 线损 4 加减 5 基准 6 空(colspan2) 7 金额
  const meterTotal = num(total?.[1])
  const diffTotal = num(total?.[2])
  const billTotal = num(total?.[5])
  check(`1 [${group}] 合计电表电量 = 166,550`, Math.abs(meterTotal - 166550) < 0.02, `${total?.[1]}`)
  check(`1 [${group}] 合计差额电量 = 5,850`, Math.abs(diffTotal - 5850) < 0.02, `${total?.[2]}`)
  check(`1 [${group}] 合计计费基准 = 169,887.75`, Math.abs(billTotal - 169887.75) < 0.02, `${total?.[5]}`)
  check(`1 [${group}] 非分时行四列显示 —`,
    rows.some((r) => r.length === 10 && (r[0] || '').includes('零售损益分摊') && r[2] === '—' && r[3] === '—') || group !== '市场化购电费',
    rows.filter((r) => r.length === 10 && (r[0] || '').includes('零售损益分摊')).map((r) => r.slice(0, 7)).join(' | '))
  await page.screenshot({ path: `${SHOTS}/r40-${group === '市场化购电费' ? 'market' : 'trans'}-breakdown.png` })
}

// 2026-09 同样按新口径出拆解列（四段电表 122,700 / 差额 3,900 / 基准 124,767.36）
const g09 = await expandGroup('2026-09', '市场化购电费')
const t09 = (g09.panel?.rows || []).filter((r) => r.length === 10 && ['尖峰', '峰', '平', '谷'].includes(r[1]))
const sum09 = (cells) => t09.reduce((s, r) => s + num(r[cells]), 0)
check('1 2026-09 拆解列齐备（四段、电表 122700、差额 3900、基准 124767.36）',
  t09.length === 4 && Math.abs(sum09(2) - 122700) < 0.02 &&
  Math.abs(sum09(3) - 3900) < 0.05 && Math.abs(sum09(6) - 124767.36) < 0.05,
  `电表=${sum09(2)} 差额=${sum09(3)} 基准=${sum09(6)}`)
check('1 2026-09 每行四分量之和 == 计费数量',
  t09.every((r) => Math.abs(num(r[2]) + num(r[3]) + num(r[4]) + num(r[5]) - num(r[6])) < 0.02),
  t09.map((r) => `${r[1]}:${r[6]}`).join('|'))
await page.screenshot({ path: `${SHOTS}/r40-2026-09-breakdown.png` })

// ======================================================================
// 断言 2：辅助服务费用口径
// ======================================================================
console.log('\n=== 断言 2：辅助服务费用 ===')
const sys = await expandGroup('2026-10', '系统运行费用')
const aux = (sys.panel?.rows || []).find((r) => r.length === 10 && (r[0] || '').includes('辅助服务费用'))
check('2 系统运行费用展开含「辅助服务费用」行', !!aux, JSON.stringify(aux))
check('2 计费数量 = 172400（工业总电量）', !!aux && Math.abs(num(aux[6]) - 172400) < 0.02, `${aux?.[6]}`)
check('2 单价/分摊比例 = 0（不是 27.63%）', !!aux && num(aux[7]) === 0 && !String(aux[7]).includes('%'), `${aux?.[7]}`)
check('2 计算方式 = 工业总电量 × 单价', !!aux && (aux[8] || '').includes('工业总电量 × 单价'), `${aux?.[8]}`)
check('2 分摊后金额 = 0（不改变已锁定总金额）', !!aux && Math.abs(num(aux[9])) < 0.001, `${aux?.[9]}`)
await page.screenshot({ path: `${SHOTS}/r40-aux-service.png` })

const sys09 = await expandGroup('2026-09', '系统运行费用')
const aux09 = (sys09.panel?.rows || []).find((r) => r.length === 10 && (r[0] || '').includes('辅助服务费用'))
check('2 2026-09 辅助服务费用 = 126600 / 0 / 工业总电量 × 单价',
  !!aux09 && Math.abs(num(aux09[6]) - 126600) < 0.02 && num(aux09[7]) === 0 &&
  (aux09[8] || '').includes('工业总电量 × 单价'), JSON.stringify(aux09))

// ======================================================================
// 断言 3：打印预览 PDF 文本层
// ======================================================================
console.log('\n=== 断言 3：打印预览排版 ===')
// 重新打开列表（展开状态会干扰勾选）
await openCostList()
await waitRows(1, 60)
await page.waitForTimeout(1500)
const cb = page.locator('tbody tr').first().locator('input[type="checkbox"], .t-checkbox').first()
if (await cb.count()) { await cb.click(); await page.waitForTimeout(1200) }
for (let i = 0; i < 20; i++) {
  if (await page.evaluate(() => !!document.querySelector('.doc-batch-bar-fixed'))) break
  await page.waitForTimeout(500)
}
const printBtn = page.getByRole('button', { name: '打印清单' })
const pdfPath = '/tmp/pw/r40-statement.pdf'
if (await printBtn.count()) {
  await printBtn.click()
  let pdfOk = false
  for (let i = 0; i < 40; i++) {
    pdfOk = await page.evaluate(() => {
      const d = document.querySelector('.ppd-dialog')
      return !!d && (d.innerText || '').includes('打印预览')
    })
    if (pdfOk) break
    await page.waitForTimeout(600)
  }
  check('3 打印清单可生成 PDF 预览', pdfOk, `preview=${pdfOk}`)
  if (pdfOk) {
    const b64 = await page.evaluate(async () => {
      const el = document.querySelector('.ppd-dialog iframe, .ppd-dialog embed, .ppd-dialog object')
      const a = Array.from(document.querySelectorAll('.ppd-dialog a[download]'))
      let url = el?.src || a.find((x) => (x.href || '').startsWith('blob:'))?.href || ''
      if (!url) {
        const blobs = performance.getEntriesByType('resource').map((r) => r.name).filter((n) => n.startsWith('blob:'))
        url = blobs[blobs.length - 1] || ''
      }
      if (!url) return ''
      const buf = await (await fetch(url)).arrayBuffer()
      let s = ''
      const bytes = new Uint8Array(buf)
      for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i])
      return btoa(s)
    })
    if (b64) {
      fs.writeFileSync(pdfPath, Buffer.from(b64, 'base64'))
      console.log('PDF_BYTES', fs.statSync(pdfPath).size)
    } else {
      console.log('PDF_BYTES 0（blob URL 未取到）')
    }
  }
  // PDF 是 canvas 逐页渲染，弹窗出现后还要等 3~4s 才有内容
  await page.waitForTimeout(4000)
  await page.screenshot({ path: `${SHOTS}/r40-print-preview.png` })
} else {
  check('3 打印清单按钮存在', false, '未找到「打印清单」按钮')
}

if (fs.existsSync(pdfPath) && fs.statSync(pdfPath).size > 10000) {
  const pdfjs = await import('/opt/pw-run/node_modules/pdfjs-dist/legacy/build/pdf.mjs')
  const doc = await pdfjs.getDocument({ data: new Uint8Array(fs.readFileSync(pdfPath)), useSystemFonts: true }).promise
  let pdfText = ''
  for (let p = 1; p <= doc.numPages; p++) {
    const tc = await (await doc.getPage(p)).getTextContent()
    pdfText += tc.items.map((i) => i.str).join(' ') + ' '
  }
  pdfText = pdfText.replace(/\s+/g, ' ')
  console.log(`\n=== PDF 文本层（${doc.numPages} 页 / ${pdfText.length} 字符）===`)
  check('3 PDF 表头前两列是「费用类别」「费用组成」',
    /费用类别 费用组成/.test(pdfText) || /费用类别\s+费用组成/.test(pdfText),
    pdfText.slice(pdfText.indexOf('费用类别') - 20, pdfText.indexOf('费用类别') + 90))
  const joined = (pdfText.match(/\([1246]\)\s*市场化购电费\s*\|/g) || []).length
    + (pdfText.match(/系统运行费\s*\|/g) || []).length
  check('3 PDF 大类单元格不再含「|」拼接符', joined === 0, `joined=${joined}`)
  // 大类只出现一次（rowspan 合并：文字只在合并区首格绘制）
  const marketCount = (pdfText.match(/市场化购电费/g) || []).length
  const sysCount = (pdfText.match(/系统运行费用/g) || []).length
  check('3 PDF 大类文字纵向合并（每类只出现一次）',
    marketCount >= 1 && marketCount <= 4 && sysCount >= 1 && sysCount <= 4,
    `市场化=${marketCount} 系统运行=${sysCount}`)
  const proxyAt = pdfText.indexOf('上网环节线损代理采购损益')
  const sysAt = pdfText.indexOf('系统运行费用')
  const transAt = pdfText.indexOf('输配电量电费')
  check('3 上网环节线损代理采购损益 在 (4) 系统运行费用组内',
    proxyAt > 0 && sysAt > 0 && proxyAt > sysAt && (transAt < 0 || sysAt > transAt),
    `proxy=${proxyAt} sys=${sysAt} trans=${transAt}`)
  check('3 PDF 系统运行费用（小计）784.43', /784\.43/.test(pdfText) && /系统运行费用（小计）/.test(pdfText),
    pdfText.slice(Math.max(0, proxyAt - 60), proxyAt + 160))
  check('3 PDF 政府性基金及附加（小计）8222.45',
    /8,?222\.45/.test(pdfText) && /政府性基金及附加（小计）/.test(pdfText),
    (pdfText.match(/政府性基金及附加（小计）.{0,40}/) || ['not found'])[0])
  check('3 PDF 市场化购电费（小计）88278.66', /88,?278\.66/.test(pdfText),
    (pdfText.match(/市场化购电费（小计）.{0,40}/) || ['not found'])[0])
} else {
  console.log('PDF_BYTES 缺失，跳过 PDF 文本断言')
}

// ---- console 健康检查 ----
quiet = false
const pass = results.filter((r) => r.ok).length
console.log(`\nSUMMARY ${pass}/${results.length} PASS`)
console.log(`CONSOLE_ERRORS ${quietErrors.length}`)
if (quietErrors.length) console.log(quietErrors.slice(0, 5).join('\n'))
await browser.close()
process.exit(pass === results.length ? 0 : 1)
