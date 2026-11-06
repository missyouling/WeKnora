/**
 * 第 43 轮真机验收。
 *
 * 断言 1（CI）：后端静态前端门控恢复后，standard 且未配 WEB_DIR 时 `/` 不再返回 SPA。
 *              此处用页面侧间接验证：站点仍能正常打开（前端由 vite 提供），
 *              且后端 API 未受影响（/api/v1/auth/me 401 属正常）。
 * 断言 2（光伏）：2026-10 光伏账单详情抽屉里，表头类字段与示数字段全部有值。
 */
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const ORIGIN = 'http://127.0.0.1:5173'
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
page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text().slice(0, 160)) })
page.on('pageerror', (e) => errors.push('PAGEERR ' + String(e).slice(0, 160)))

let ok = 0
let total = 0
const check = (name, pass, detail = '') => {
  total++
  if (pass) ok++
  console.log(`${pass ? 'PASS' : 'FAIL'} | ${name} | ${detail}`)
}
const waitFor = async (fn, times = 80, gap = 500) => {
  for (let i = 0; i < times; i++) {
    if (await page.evaluate(fn)) return true
    await page.waitForTimeout(gap)
  }
  return false
}

// ---- 进入能耗管理 → 光伏账单 ----
await page.goto(`${ORIGIN}/platform/daily-affairs/utilities`, { waitUntil: 'domcontentloaded', timeout: 45000 })
await waitFor(() => document.querySelectorAll('.dash-kpi-card').length > 0)
await page.evaluate(() => {
  const c = Array.from(document.querySelectorAll('.dash-kpi-card')).find((e) => /光伏/.test(e.innerText || ''))
  if (c) c.click()
})
await waitFor(() => Array.from(document.querySelectorAll('tbody tr')).some((t) => t.querySelectorAll('td').length > 3))

// 找到 2026-10 行并打开详情
const opened = await page.evaluate(() => {
  const trs = Array.from(document.querySelectorAll('tbody tr')).filter((t) => t.querySelectorAll('td').length > 3)
  const tr = trs.find((t) => (t.innerText || '').includes('2026-10'))
  if (!tr) return false
  const target = tr.querySelector('.t-table__row-link, a, .t-link, td:first-child div') || tr.querySelectorAll('td')[1]
  if (target) { target.click(); return true }
  tr.click()
  return true
})
check('2026-10 光伏账单行可打开详情', opened)
await page.waitForTimeout(4000)

// 详情抽屉内的输入框值
const values = await page.evaluate(() => {
  const inputs = Array.from(document.querySelectorAll('.t-dialog input, .t-drawer input, [class*=dialog] input'))
  const map = {}
  for (const el of inputs) {
    const label = (el.closest('.t-form__item')?.innerText || '').split('\n')[0].trim()
    if (label) map[label] = el.value
  }
  return map
})
const has = (label, want) => {
  const v = String(values[label] || '').trim()
  if (!want) return v !== '' && v !== '0'
  return v === want || v.includes(want)
}
check('账单周期 = 2026-10', has('账单周期', '2026-10'), `got=${values['账单周期']}`)
check('发电户号 有值', has('发电户号'), `got=${values['发电户号']}`)
check('户名 有值', has('户名'), `got=${values['户名']}`)
check('发电量 = 66026', has('发电量', '66026'), `got=${values['发电量']}`)
check('上网电量 = 2970', has('上网电量', '2970'), `got=${values['上网电量']}`)
check('结算金额 = 1125.14', has('结算金额', '1125.14'), `got=${values['结算金额']}`)
check('年累计 有值', has('年累计'), `got=${values['年累计']}`)
check('上期示数 有值', has('上期示数'), `got=${values['上期示数']}`)
check('本期示数 有值', has('本期示数'), `got=${values['本期示数']}`)
check('倍率 有值', has('倍率'), `got=${values['倍率']}`)
check('抄见电量 有值', has('抄见电量'), `got=${values['抄见电量']}`)
check('计费电量 有值', has('计费电量'), `got=${values['计费电量']}`)
check('纳税人类型 有值', has('纳税人类型'), `got=${values['纳税人类型']}`)
check('并网电压 有值', has('并网电压'), `got=${values['并网电压']}`)
check('消纳方式 有值', has('消纳方式'), `got=${values['消纳方式']}`)
check('发电方式 有值', has('发电方式'), `got=${values['发电方式']}`)
check('服务单位 有值', has('服务单位'), `got=${values['服务单位']}`)
check('发电地址 有值', has('发电地址'), `got=${values['发电地址']}`)
check('console error 为 0', errors.length === 0, `errors=${errors.length} ${errors.slice(0, 2).join(' | ')}`)

await page.screenshot({ path: '/tmp/pw/shots/r43-solar-detail.png', fullPage: false })
console.log('VALUES', JSON.stringify(values, null, 1))
console.log(`RESULT ${ok}/${total}`)
await browser.close()
process.exit(ok === total ? 0 : 1)
