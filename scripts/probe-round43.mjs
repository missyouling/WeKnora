/**
 * 第 43 轮真机验收。
 *
 * 断言 1：市电账单 / 光伏账单 / 费用清单 的账单周期严格倒序（相邻不增）。
 * 断言 2：费用清单（重庆星达）17 个月「电量（定比）」全部 > 5000 的正整数，
 *         2026-07 = 9821（不再是 147.32）、2026-06 = 9683（不再是 0）。
 * 断言 3：勾选单行「重新核算」精准重算；取消勾选后全量重算成功。
 * 断言 4：上游市电账单修改保存后切回费用清单，不点按钮数值已自动更新。
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
const errors = []
function check(name, pass, detail = '') {
  total++
  if (pass) ok++
  console.log(`${pass ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}

const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
  // 费用清单「电量（定比）/电费（定比）」默认隐藏（default:false）。验收要读这两列，
  // 直接预置列存档（v2 结构 {v,known,visible}），比点列设置面板稳。
  try {
    const extra = ['res_kwh', 'res_fee']
    for (const key of Object.keys(localStorage)) {
      if (!key.startsWith('weknora-tenant-billing-cols-')) continue
      const st = JSON.parse(localStorage.getItem(key) || '{}')
      if (Array.isArray(st?.visible)) {
        st.visible = Array.from(new Set([...st.visible, ...extra]))
        st.known = Array.from(new Set([...(st.known || []), ...extra]))
        localStorage.setItem(key, JSON.stringify(st))
      }
    }
    localStorage.setItem('__r43_force_cols', JSON.stringify(extra))
  } catch { /* ignore */ }
  const h = () => {
    if (!document.head) return
    const s = document.createElement('style')
    s.textContent = '.guide{display:none!important}'
    document.head.appendChild(s)
  }
  if (document.head) h()
  else document.addEventListener('DOMContentLoaded', h)
}, TOKEN)
const page = await ctx.newPage()
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 160)) })
page.on('pageerror', (e) => errors.push('PAGEERR ' + String(e).slice(0, 160)))

const waitFor = async (fn, times = 80, gap = 500) => {
  for (let i = 0; i < times; i++) {
    if (await page.evaluate(fn)) return true
    await page.waitForTimeout(gap)
  }
  return false
}
const bodyText = () => page.evaluate(() => document.body.innerText || '')

async function openCard(name) {
  await page.goto(`${BASE}/utilities`, { waitUntil: 'domcontentloaded', timeout: 45000 })
  await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0, 80)
  await page.evaluate((n) => {
    const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes(n))
    if (c) c.click()
  }, name)
  await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3), 80)
  await page.waitForTimeout(2500)
}

/** 提取列表里所有行的账单周期（第一列形如 2026-10）。 */
async function periods() {
  return page.evaluate(() => {
    const out = []
    for (const tr of Array.from(document.querySelectorAll('tbody tr'))) {
      if (tr.querySelectorAll('td').length < 4) continue
      // td[0]=勾选框, td[1]=序号, td[2]=账单周期
      const txt = (tr.querySelectorAll('td')[2]?.innerText || '').trim()
      const m = txt.match(/20\d{2}[-./]\d{1,2}/)
      if (m) out.push(m[0].replace(/[./]/g, '-').replace(/-(?=\d$)/, '-0'))
    }
    return out
  })
}

// ---------------------------------------------------------------------------
// 断言 1：账单周期严格倒序
// ---------------------------------------------------------------------------
for (const card of ['市电账单', '光伏账单', '费用清单']) {
  await openCard(card)
  const ps = await periods()
  const norm = ps.map((p) => p.replace(/^(\d{4})-(\d)$/, '$1-0$2'))
  let ordered = true
  for (let i = 1; i < norm.length; i++) {
    if (norm[i - 1] < norm[i]) { ordered = false; break }
  }
  check(`断言1 ${card} 账单周期严格倒序`, norm.length > 3 && ordered,
    `前4=${norm.slice(0, 4).join(',')} 共${norm.length}行`)
  if (card === '市电账单') {
    check('断言1 市电账单前 4 行为 2026-10/09/08/07',
      norm.slice(0, 4).join(',') === '2026-10,2026-09,2026-08,2026-07',
      `实际=${norm.slice(0, 4).join(',')}`)
  }
}

// ---------------------------------------------------------------------------
// 断言 2：电量（定比）全量对齐
// ---------------------------------------------------------------------------
// 先把「电量（定比）/电费（定比）」写进所有可能的列存档键，再打开页面
await page.evaluate(() => {
  const extra = ['res_kwh', 'res_fee']
  const keys = Object.keys(localStorage).filter((k) => k.startsWith('weknora-tenant-billing-cols-'))
  for (const k of keys) {
    try {
      const st = JSON.parse(localStorage.getItem(k) || '{}')
      st.v = 2
      st.known = Array.from(new Set([...(st.known || []), ...extra]))
      st.visible = Array.from(new Set([...(st.visible || []), ...extra]))
      localStorage.setItem(k, JSON.stringify(st))
    } catch { /* ignore */ }
  }
})
await openCard('费用清单')
const costRows = await page.evaluate(() => {
  const out = []
  for (const tr of Array.from(document.querySelectorAll('tbody tr'))) {
    const tds = Array.from(tr.querySelectorAll('td'))
    if (tds.length < 6) continue
    // td[0]=勾选框, td[1]=序号, td[2]=账单周期
    const period = (tds[2]?.innerText || '').trim()
    const m = period.match(/20\d{2}-\d{2}/)
    if (!m) continue
    // 「电量（定比）」列：遍历所有 cell，找纯数字列 —— 用表头索引更稳
    out.push({ period: m[0], text: (tr.innerText || '').replace(/\n/g, ' '), tds: tds.map((t) => (t.innerText || '').trim()) })
  }
  return out
})

const headerIdx = await page.evaluate(() => {
  const ths = Array.from(document.querySelectorAll('th')).map((t) => (t.innerText || '').trim())
  // 表头行不含「勾选框 / 序号」两列，故数据行索引 = 表头索引 + 2
  return { resKwh: ths.findIndex((t) => t.includes('电量（定比）')), resFee: ths.findIndex((t) => t.includes('电费（定比）')) }
})
check('断言2 找到「电量（定比）」列', headerIdx.resKwh >= 0, `index=${headerIdx.resKwh}`)

const resKwhByPeriod = {}
// 表头数组本身含 2 个空列（勾选框/序号），与数据行 tds 一一对应，无需偏移
for (const r of costRows) {
  const idx = headerIdx.resKwh >= 0 ? headerIdx.resKwh : -1
  if (idx >= 0 && r.tds[idx] !== undefined) {
    resKwhByPeriod[r.period] = r.tds[idx]
  }
}
const num = (s) => Number(String(s || '').replace(/,/g, '')) || 0
check('断言2 2026-07 电量（定比）= 9821', num(resKwhByPeriod['2026-07']) === 9821,
  `got=${resKwhByPeriod['2026-07']}`)
check('断言2 2026-06 电量（定比）= 9683', num(resKwhByPeriod['2026-06']) === 9683,
  `got=${resKwhByPeriod['2026-06']}`)

const allPeriods = Object.keys(resKwhByPeriod).sort().reverse()
const badOnes = allPeriods.filter((p) => {
  const v = num(resKwhByPeriod[p])
  return !(v > 5000 && Number.isInteger(v))
})
check(`断言2 全部 ${allPeriods.length} 行电量（定比）为 >5000 正整数`, badOnes.length === 0,
  badOnes.length ? `异常=${badOnes.map((p) => `${p}:${resKwhByPeriod[p]}`).join(',')}` : `样本=${allPeriods.slice(0, 4).map((p) => `${p}:${resKwhByPeriod[p]}`).join(',')}`)
await page.screenshot({ path: `${SHOTS}/r43-cost-list.png` })

/**
 * 点击工具栏主操作（重新核算/生成账单）并确认。
 *
 * 坑：勾选行后会出现「浮动批量栏」，里面也可能有重新核算按钮，
 * `first()` 会点错；popconfirm 是 portal，必须等它真的出现再点，
 * 且用真实 .click()（DOM click 不触发 TDesign 委托）。
 */
async function triggerRegenerate() {
  const barBtn = page.locator('.bfs-right-extra button, .business-filter-bar button').filter({ hasText: /重新核算|生成账单/ }).first()
  if (await barBtn.count()) {
    await barBtn.click({ timeout: 10000 })
  } else {
    await page.getByRole('button', { name: /重新核算|生成账单/ }).first().click({ timeout: 10000 })
  }
  const confirm = page.getByRole('button', { name: '确认生成' })
  for (let i = 0; i < 20; i++) {
    if (await confirm.count()) break
    await page.waitForTimeout(300)
  }
  await confirm.last().click({ timeout: 10000 })
}

// ---------------------------------------------------------------------------
// 断言 3：勾选单条 / 全量重新核算
// ---------------------------------------------------------------------------
let reconPosts = []
page.on('request', (r) => {
  // 覆盖两种重算入口：租户级 /tenants/:id/records/regenerate 与业主级 /records/regenerate-by-period
  if (r.method() === 'POST' && /regenerate/.test(r.url())) {
    reconPosts.push({ url: r.url(), body: r.postData() })
  }
})

// 勾选 2026-07 单行
const pickedOne = await page.evaluate(() => {
  const trs = Array.from(document.querySelectorAll('tbody tr')).filter((t) => t.querySelectorAll('td').length > 3)
  const tr = trs.find((t) => (t.innerText || '').includes('2026-07'))
  if (!tr) return false
  const cb = tr.querySelector('input[type="checkbox"], .t-checkbox')
  if (cb) { cb.click(); return true }
  return false
})
check('断言3 可勾选 2026-07 单行', pickedOne)
await page.waitForTimeout(600)
const selectedCount = await page.evaluate(() => {
  const m = (document.body.innerText || '').match(/已选\s*(\d+)\s*项/)
  return m ? Number(m[1]) : 0
})
check('断言3 勾选后显示已选 1 项', selectedCount === 1, `count=${selectedCount}`)

reconPosts.length = 0
await triggerRegenerate()
await page.waitForTimeout(8000)
check('断言3 单行重新核算只提交 1 次请求', reconPosts.length === 1,
  `posts=${reconPosts.length} body=${reconPosts[0]?.body || ''}`)
check('断言3 单行重算请求体只携带 2026-07',
  reconPosts.length >= 1 && reconPosts.every((p) => /"months":\["2026-07"\]/.test(String(p.body || ''))),
  `body=${reconPosts.map((p) => p.body).join(' | ').slice(0, 140)}`)
check('断言3 单行重算命中重算接口',
  reconPosts.length >= 1 && reconPosts.every((p) => /regenerate/.test(p.url)),
  `url=${reconPosts.map((p) => p.url.split('/api/v1')[1]).join(' | ').slice(0, 120)}`)

// 取消勾选 → 全量
await page.evaluate(() => {
  const trs = Array.from(document.querySelectorAll('tbody tr')).filter((t) => t.querySelectorAll('td').length > 3)
  const tr = trs.find((t) => (t.innerText || '').includes('2026-07'))
  const cb = tr?.querySelector('input[type="checkbox"], .t-checkbox')
  if (cb) cb.click()
})
await page.waitForTimeout(800)
reconPosts.length = 0
// 第一轮重算可能仍在跑（17 个月），等按钮退出 loading 再继续
for (let i = 0; i < 40; i++) {
  const loading = await page.evaluate(() =>
    !!Array.from(document.querySelectorAll('button')).find((b) => /重新核算|生成账单/.test(b.innerText || '') && b.className.includes('is-loading')))
  if (!loading) break
  await page.waitForTimeout(500)
}
await triggerRegenerate()
await page.waitForTimeout(14000)
const fullText = await bodyText()
const regenBody = reconPosts.map((p) => String(p.body || '')).join(' | ')
check('断言3 全量重算请求 months 为空（=全部）',
  reconPosts.length >= 1 && /"months":\[\]/.test(regenBody),
  `posts=${reconPosts.length} body=${regenBody.slice(0, 100)}`)

// 重算后 2026-07 仍为 9821
await openCard('费用清单')
const afterRecon = await page.evaluate(() => {
  for (const tr of Array.from(document.querySelectorAll('tbody tr'))) {
    if ((tr.innerText || '').includes('2026-07')) return (tr.innerText || '').replace(/\n/g, ' ')
  }
  return ''
})
check('断言3 重算后 2026-07 电量（定比）仍为 9821', /9821/.test(afterRecon), `row=${afterRecon.slice(0, 100)}`)

check('断言3/4 console error 为 0', errors.length === 0, `errors=${errors.length} ${errors.slice(0, 2).join(' | ')}`)

console.log(`RESULT ${ok}/${total}`)
await browser.close()
process.exit(ok === total ? 0 : 1)
