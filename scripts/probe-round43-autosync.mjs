/**
 * 第 43 轮断言 4（API 直改版）：上游市电账单被修改后，费用清单自动同步。
 *
 * 为什么走 API 而不是点抽屉输入框：市电账单详情抽屉的字段是「只读渲染 + 明细
 * 子表编辑」，没有可直接改「本期电量」的输入框；而产品要求验证的是
 * **保存上游数据 → 费用清单自动更新** 这条链路，用 PUT metadata 改 record
 * 等价于用户保存，且可精确还原。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
const BASE = `${ORIGIN}/platform/daily-affairs`

let ok = 0
let total = 0
const check = (name, pass, detail = '') => {
  total++
  if (pass) ok++
  console.log(`${pass ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}

// ---- 后端直连：拿到 kbId 与 2026-08 的市电账单 knowledge ----
const KB_API = `${ORIGIN}/api/v1/knowledge-bases?page=1&page_size=50`
const req = async (path, init = {}) => {
  const res = await fetch(`${ORIGIN}${path}`, {
    ...init,
    headers: { Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json', ...(init.headers || {}) },
  })
  return { status: res.status, body: await res.text() }
}

const kbs = JSON.parse((await req('/api/v1/knowledge-bases?page=1&page_size=100')).body)
const kbList = kbs?.data || kbs?.list || []
// 按「有市电账单记录」挑知识库：名字带能耗的库里其实是空演示数据
let kbId = ''
for (const k of kbList) {
  const r = await req(`/api/v1/knowledge-bases/${k.id}/utility-bill-records?page=1&page_size=200`)
  try {
    if ((JSON.parse(r.body)?.total || 0) > 0) { kbId = k.id; break }
  } catch { /* ignore */ }
}
check('断言4 取到含市电账单的知识库', !!kbId, `kb=${kbId}`)

// 拉全部文件，找到 2026-08 市电账单
const files = JSON.parse((await req(`/api/v1/knowledge-bases/${kbId}/utility-bill-records?page=1&page_size=200`)).body)
const fileList = files?.data || files?.list || []
let target = null
for (const f of fileList) {
  const rec = f?.item || {}
  if (rec?.bill_period === '2026-08') { target = { file: { id: f.knowledge_id }, rec }; break }
}
check('断言4 找到 2026-08 市电账单', !!target, `file=${target?.file?.id} period=${target?.rec?.bill_period}`)

const beforeKwh = Number(target?.rec?.total_kwh) || 0
check('断言4 基线本期电量', beforeKwh > 0, `total_kwh=${beforeKwh}`)

// ---- 浏览器：先读费用清单的用电量（工业）----
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1680, height: 1000 } })
await ctx.addInitScript((tk) => {
  localStorage.setItem('weknora_token', tk)
  localStorage.setItem('weknora_selected_tenant_id', '10000')
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
const errors = []
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 140)) })

const waitFor = async (fn, times = 80, gap = 500) => {
  for (let i = 0; i < times; i++) {
    if (await page.evaluate(fn)) return true
    await page.waitForTimeout(gap)
  }
  return false
}
async function openCostList() {
  await page.goto(`${BASE}/utilities`, { waitUntil: 'domcontentloaded', timeout: 45000 })
  await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0)
  await page.evaluate(() => {
    const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => (e.innerText || '').includes('费用清单'))
    if (c) c.click()
  })
  await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3))
  await page.waitForTimeout(3000)
}
async function indKwhOf(month) {
  return page.evaluate((m) => {
    const ths = Array.from(document.querySelectorAll('thead th')).map((t) => (t.innerText || '').trim())
    const idx = ths.findIndex((t) => t === '用电量（工业）')
    for (const tr of Array.from(document.querySelectorAll('tbody tr'))) {
      const tds = Array.from(tr.querySelectorAll('td'))
      if (tds.length < 4) continue
      if (!(tds[2]?.innerText || '').includes(m)) continue
      return idx >= 0 ? (tds[idx]?.innerText || '').trim() : ''
    }
    return ''
  }, month)
}

await openCostList()
const before = await indKwhOf('2026-08')
check('断言4 费用清单基线', Number(String(before).replace(/,/g, '')) > 0, `before=${before}`)

// ---- 改上游：本期电量 +1000（保存元数据）----
const patchedRec = { ...target.rec, total_kwh: beforeKwh + 1000 }
// 走二开 invoice-metadata 路由（数组型元数据专用，绕过原生标量校验）
const patchBody = JSON.stringify({
  custom_metadata: { records: [patchedRec], kind: 'utility_bill', extract_status: 'success' },
})
const patchRes = await req(`/api/v1/knowledge-bases/${kbId}/knowledge/${target.file.id}/invoice-metadata`, {
  method: 'PUT', body: patchBody,
})
check('断言4 上游本期电量保存成功', patchRes.status === 200, `status=${patchRes.status} body=${String(patchRes.body).slice(0, 100)}`)

// ---- 切回费用清单（不点重新核算），断言自动更新 ----
await openCostList()
await page.waitForTimeout(4000)
const after = await indKwhOf('2026-08')
const nBefore = Number(String(before).replace(/,/g, ''))
const nAfter = Number(String(after).replace(/,/g, ''))
check('断言4 费用清单自动同步（未点重新核算）', Math.abs(nAfter - nBefore) >= 100,
  `before=${nBefore} after=${nAfter} delta=${nAfter - nBefore}`)

// ---- 还原 ----
const restoreBody = JSON.stringify({
  custom_metadata: { records: [target.rec], kind: 'utility_bill', extract_status: 'success' },
})
const restoreRes = await req(`/api/v1/knowledge-bases/${kbId}/knowledge/${target.file.id}/invoice-metadata`, {
  method: 'PUT', body: restoreBody,
})
check('断言4 已还原上游数据', restoreRes.status === 200, `status=${restoreRes.status}`)
await openCostList()
const restored = await indKwhOf('2026-08')
check('断言4 还原后费用清单回到基线', Number(String(restored).replace(/,/g, '')) === nBefore,
  `restored=${restored} want=${nBefore}`)

check('断言4 console error 为 0', errors.length === 0, `errors=${errors.length} ${errors.slice(0, 2).join(' | ')}`)
console.log(`RESULT ${ok}/${total}`)
await browser.close()
process.exit(ok === total ? 0 : 1)
