// 全量 17 份市电账单的对比状态核对（第 33 轮断言 4 的后半段）。
// 逐份打开详情 → 读账单概况每一行的对比状态 → 有任何「异常」即失败。
import { chromium } from 'playwright'
import fs from 'node:fs'

const TOKEN = fs.readFileSync('/tmp/pw/token.txt', 'utf8').trim()
const browser = await chromium.launch({ args: ['--no-sandbox'] })
const ctx = await browser.newContext({ viewport: { width: 1600, height: 1000 } })
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
await page.goto('http://127.0.0.1:5173/platform/daily-affairs/utilities', { waitUntil: 'domcontentloaded' })
await page.waitForTimeout(3000)
for (let i = 0; i < 60; i++) {
  const n = await page.evaluate(() => document.querySelectorAll('.doc-table tbody tr').length)
  if (n >= 17) break
  await page.waitForTimeout(1000)
}
// 行的第一列是序号、第二列才是账单周期；用 YYYY-MM 做定位键（唯一且稳定）
const names = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.doc-table tbody tr'))
    .map((tr) => (tr.innerText.match(/\b\d{4}-\d{2}\b/) || [''])[0])
    .filter(Boolean))
console.log('bills', names.length)

const bad = []
for (const name of names) {
  // 返回列表后表格会重建：按文件名定位，等它出现再点（索引会漂移）
  let clicked = false
  for (let k = 0; k < 20; k++) {
    const row = page.locator('.doc-table tbody tr').filter({ hasText: name }).first()
    if (await row.count()) {
      await row.click()
      clicked = true
      break
    }
    await page.waitForTimeout(500)
  }
  if (!clicked) { bad.push({ name, open: 'row not found' }); continue }
  await page.waitForTimeout(1000)
  const states = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.os-row')).map((r) => r.innerText.replace(/\s+/g, ' ').trim()))
  const abnormal = states.filter((s) => s.includes('异常'))
  if (abnormal.length) bad.push({ name, abnormal })
  const totalRow = states.find((s) => s.startsWith('本期电费'))
  if (!totalRow || !totalRow.includes('正常')) bad.push({ name, totalRow: totalRow || 'none' })
  await page.evaluate(() => {
    const btn = Array.from(document.querySelectorAll('.bill-detail-head button'))
      .find((b) => (b.textContent || '').includes('账单列表'))
    if (btn) btn.click()
  })
  await page.waitForTimeout(900)
}
console.log('ABNORMAL', JSON.stringify(bad, null, 1))
console.log(bad.length ? 'FAIL' : `PASS | ${names.length} 份账单对比状态全部正常`)
await browser.close()
process.exit(bad.length ? 1 : 0)
