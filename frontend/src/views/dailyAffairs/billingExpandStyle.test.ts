import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

/**
 * 第 16 轮回归锁定：
 *   ① 默认分组（重庆星达）绝不掺入租户分摊列；
 *   ② 分时细项在展开区必须逐段列出尖峰/峰/平/谷；
 *   ③ 行内展开徽标只有一份样式源，与「电费核算」完全一致；
 *   ④ 列宽必须按表头文字保底，不允许被截成「上网环节线...」。
 */
const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
const billing = read('./TenantBilling.vue')
const meter = read('./UtilityMeterTab.vue')
const toggleCss = read('./styles/row-expand-toggle.css')

test('默认分组不掺租户列：itemByKey 与 extra 都按视角切断', () => {
  // 租户子项来自 billing_tenant_items（上一家租户的残留），
  // 切回重庆星达时 activeTenantId 不会清空 → 必须按视角显式隔离。
  assert.match(billing, /const ownerView = isOwnerView\.value/,
    '必须显式取一次视角，避免各处重复判断走岔')
  assert.match(billing, /const itemByKey = ownerView\s*\n?\s*\? new Map<string, any>\(\)/,
    '业主视角不得建立分摊子项索引（否则会追加租户列）')
  assert.match(billing, /for \(const it of ownerView \? \[\] : tenantItemsUnique\.value\)/,
    '业主视角不得把租户子项补进表头')
  // 切回业主时清空子项，避免残留
  assert.match(billing, /if \(!id \|\| isOwnerView\.value\) \{ tenantItems\.value = \[\]; syncColumnKeys\(\); return \}/,
    '切回业主必须清空 tenantItems')
})

test('分时细项在展开区逐段列出（尖峰/峰/平/谷）', () => {
  assert.match(billing, /const itemPeriods = new Map/, '必须缓存分时四段明细')
  assert.match(billing, /itemPeriods\.set\(ckey, timed\.periods\)/, '分时项要把四段存下来')
  assert.match(billing, /for \(const pd of periods\)/, '展开区要逐段出行，而不是一行合计')
  assert.match(billing, /period: pd\.period/, '每行要带时段名')
  assert.match(billing, /qty: Math\.round\(pd\.qty \* 100\) \/ 100/, '每行要带基准电量')
  assert.match(billing, /price: pd\.rate/, '每行要带时段单价')
})

test('展开徽标：唯一 CSS 源，且与电费核算完全同款', () => {
  // 18×18 正圆、无边框、收起浅灰 / 展开品牌绿
  assert.match(toggleCss, /width:\s*18px/)
  assert.match(toggleCss, /height:\s*18px/)
  assert.match(toggleCss, /border-radius:\s*50%/)
  assert.match(toggleCss, /border:\s*none/)
  assert.match(toggleCss, /\.row-expand-toggle\.is-open/)
  // 两个页面都从这里取，不再各写一套
  assert.match(billing, /import '\.\/styles\/row-expand-toggle\.css'/)
  assert.match(meter, /import '\.\/styles\/row-expand-toggle\.css'/)
  // 旧的方形绿框按钮类必须彻底消失
  assert.ok(!/\.fee-group-toggle\s*\{/.test(billing), '费用清单不得再有自己的方形按钮样式')
  assert.ok(!/^\s*\.row-expand-toggle\s*\{/m.test(meter), '电费核算不得再本地重复定义徽标样式')
  // 单元格渲染器用的是共享类名
  assert.match(billing, /class: \['row-expand-toggle', open \? 'is-open' : ''\]/)
})

test('展开区结构对齐电费核算：无标题横幅、表头行 + 明细行 + 合计行', () => {
  assert.ok(!/fgp-head/.test(billing), '不得再有「大类 + 小计 + N 个细项之和」标题横幅')
  assert.ok(!/fgp-title/.test(billing), '不得再有标题文案节点')
  assert.match(billing, /class="fee-group-table"/)
  assert.match(billing, /class="fg-head-row"/)
  assert.match(billing, /class="fg-total-row"/)
  // 与电费核算同款的单元格 padding / 行高 / 分隔线
  assert.match(billing, /padding: 6px var\(--td-comp-paddingLR-s\)/)
  assert.match(billing, /tr \+ tr \.fg-cell \{ border-top: 1px solid var\(--td-component-stroke\); \}/)
})

test('列宽按表头文字保底，且不吞用户配置的大宽度', () => {
  assert.match(billing, /function minColumnWidth\(c: ColDef\): number/, '必须有列宽估算')
  assert.match(billing, /label\.length \* 15 \+ 34/, '表头文字按 15px/字 + padding 估算')
  assert.match(billing, /c\.feeGroup \? 26 : 0/, '大类列要额外留出 18px 徽标的位置')
  assert.match(billing, /width: Math\.min\(400, Math\.max\(cfg > 0 \? cfg : 0, minW\)\)/,
    '用户配置只允许更大，不允许小于表头所需宽度')
})
