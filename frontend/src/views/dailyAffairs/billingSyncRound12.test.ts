import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { resolveSourceValue } from './tenantBillingSources'

/**
 * 第 8 轮（2026-10-28 下午）真机问题回归契约。
 *
 * ① 保存字段配置把分类 enabled 写成 false（整行变灰、卡片/类型下拉里消失）；
 * ② 新增字段没同步到 [☰ 字段]（且弹窗底部有横向滚动条）；
 * ③ 目录电费（居民）·计费电量 全空 —— source.group 存的是分组键 `catalog`，
 *    账单 fee_items.category 是中文 `(5)目录电费`，键撞中文永远不匹配；
 * ④ 字段名输入框被 160px 固定列 + 120px 徽标挤成「账...」；
 * ⑤ 保存后重拉全量业务数据导致列表刷新慢。
 */

const here = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(here, f), 'utf8')
const readGo = (rel: string) => readFileSync(join(here, '../../../../', rel), 'utf8')

// ---- ① 保存不得误关分类 ----

test('后端 PUT 不得把缺省的 enabled 写成 Go 零值 false', () => {
  const go = readGo('internal/handler/fleet.go')
  const fn = go.slice(go.indexOf('func (h *FleetHandler) UpdateFleetCategory'), go.indexOf('func (h *FleetHandler) SortFleetCategories'))
  // updates 初始化里不允许预置 enabled（预置了就是 false 覆盖库中原值）
  const init = fn.slice(fn.indexOf('updates := map[string]interface{}{'), fn.indexOf('var reqRaw'))
  assert.ok(!/["']enabled["']\s*:/.test(init),
    'updates 初始值不得包含 enabled：请求体不带该字段时会把分类静默关停')
  assert.match(fn, /if err := json\.Unmarshal\(bodyBytes, &reqRaw\); err == nil && reqRaw\.Enabled != nil \{\s*\n\s*updates\["enabled"\] = \*reqRaw\.Enabled/)
})

test('前端保存必须显式回传当前 enabled，且本地回写保留原状态', () => {
  const src = read('FleetCertPanel.vue')
  const fn = src.slice(src.indexOf('async function commitEdit'), src.indexOf('// ---- 字段生成'))
  assert.match(fn, /const enabled = item\.enabled !== false/, 'undefined 视为开启')
  assert.match(fn, /updateFleetCategory\(item\.id, \{ name, subs, enabled \}\)/)
  assert.match(fn, /list\[idx\] = \{ \.\.\.list\[idx\], name, subs, enabled \}/,
    '本地回写必须保留 enabled，否则保存后整行立即变灰')
})

// ---- ② 保存后同步：事件直接携带 subs，不再回查接口 ----

test('分类变更事件必须携带刚保存的 subs', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /function notifyCategoriesChanged\(detail\?:/, '必须支持 detail 参数')
  assert.match(src, /new CustomEvent\('fleet-categories-changed', \{ detail: detail \|\| \{\} \}\)/)
  assert.match(src, /notifyCategoriesChanged\(\{ name, subs \}\)/, '保存成功后要把 subs 广播出去')
})

test('四个列表组件都要消费事件里的 subs（不再为改列配置发 GET）', () => {
  const files = ['TenantBilling.vue', 'SolarManagement.vue', 'UtilityMeterTab.vue']
  for (const f of files) {
    const src = read(f)
    assert.match(src, /detail\?\.subs/, `${f} 必须从事件 detail 取 subs`)
    assert.match(src, /(applySubs|categorySubs\.value = detail!?\.subs)/, `${f} 必须把 subs 落到本地`)
  }
  // 市电账单列表：只重建列定义，绝不重拉账单
  const um = read('UtilitiesManagement.vue')
  assert.match(um, /window\.addEventListener\('fleet-categories-changed', onCategoriesChanged\)/)
  const fn = um.slice(um.indexOf('function onCategoriesChanged()'), um.indexOf('onMounted('))
  assert.match(fn, /loadFieldConfigs\(\)/)
  assert.ok(!/loadFiles\(/.test(fn), '字段变更不得触发账单列表重拉')
})

// ---- ③ 分组键 ↔ 账单中文分类 ----

test('分组取值必须支持「键 → 中文分类」别名（catalog → (5)目录电费）', () => {
  const ctx: any = {
    month: '2026-09', tenantName: '持睿汽车',
    bill: {
      item: {
        fee_items: [
          { category: '(5)目录电费', name: '基础电费', period: '平', qty: 8938, rate: 0.500306, fee: 4471.73 },
          { category: '(1)市场化购电费', name: '偏差电费', period: '平', qty: 0, rate: 0, fee: 0 },
          { category: '(1)市场化购电费', name: '零售交易电费', period: '平', qty: 587230, rate: 0.09, fee: 52850.7 },
        ],
        meter_readings: [{ meter_type: '正向有功（总）', bill_kwh: 596168 }],
        residential_readings: [{ project: '定比0.015', kwh: 596168, ratio: 0.015, bill_kwh: 8938 }],
      },
    },
    solarItems: [], elecRows: [], gasRows: [], waterMeters: [], waterReadingMap: new Map(),
  }
  // 旧数据只有键：catalog 必须靠别名命中「(5)目录电费」
  const legacy = resolveSourceValue({ category: '市电账单', field: 'qty', mode: 'field', group: 'catalog' } as any, ctx)
  assert.equal(legacy, 8938, 'catalog·qty 必须取到目录电费的计费电量 8938')
  // 新数据带 label：直接命中
  const withLabel = resolveSourceValue({ category: '市电账单', field: 'qty', mode: 'field', group: 'catalog', groupLabel: '目录电费（居民）' } as any, ctx)
  assert.equal(withLabel, 8938)
  // 电费同样要能取
  const fee = resolveSourceValue({ category: '市电账单', field: 'fee', mode: 'field', group: 'catalog' } as any, ctx)
  assert.equal(fee, 4471.73)
  // 市场化的计费电量不能把别的分组算进来
  const market = resolveSourceValue({ category: '市电账单', field: 'qty', mode: 'field', group: 'market' } as any, ctx)
  assert.equal(market, 587230)
  // 「电量明细（居民）」走 residential_readings 的 bill_kwh
  const meter = resolveSourceValue({ category: '市电账单', field: 'bill_kwh', mode: 'field', group: 'meter', groupLabel: '电量明细（居民）' } as any, ctx)
  assert.equal(meter, 8938)
  // 计费标准：同价分段必须回传账单原值（fee÷qty 会漂成 0.500305，核对不上）
  const rate = resolveSourceValue({ category: '市电账单', field: 'rate', mode: 'field', group: 'catalog' } as any, ctx)
  assert.equal(rate, 0.500306, '同价分段取原值，不做金额反算')
  // 真分时差价（各段单价不同）才加权
  ctx.bill.item.fee_items.push(
    { category: '(7)分时测试', name: '峰', period: '峰', qty: 100, rate: 1, fee: 100 },
    { category: '(7)分时测试', name: '谷', period: '谷', qty: 300, rate: 0.5, fee: 150 },
  )
  const mixed = resolveSourceValue({ category: '市电账单', field: 'rate', mode: 'field', group: '', groupLabel: '(7)分时测试' } as any, ctx)
  assert.equal(mixed, 0.625, '(100+150) ÷ (100+300) = 0.625')
})

test('别名表必须覆盖市电账单全部业务分组', () => {
  const src = read('tenantBillingSources.ts')
  for (const key of ['market', 'line', 'trans', 'sys', 'catalog', 'gov-industrial', 'gov-residential', 'capacity', 'pf']) {
    assert.match(src, new RegExp(`['"]?${key}['"]?:\\s*\\[`), `别名表缺分组 ${key}`)
  }
})

// ---- ④ 字段名编辑框宽度 ----

test('字段名所在列必须弹性加宽，且输入框保底 140px', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /grid-template-columns: 24px minmax\(280px, 1fr\) 90px 64px 64px 48px/)
  assert.match(src, /grid-template-columns: 24px minmax\(280px, 1fr\) 90px 70px 64px 64px 48px/)
  assert.ok(!/24px 160px 90px/.test(src), '160px 固定列会把字段名压成「账...」')
  assert.match(src, /\.fc-name--ref \{ flex: 1 1 auto; min-width: 140px; \}/)
  assert.match(src, /max-width: 130px;/, '来源徽标要有上限，给输入框留出 140px')
})

// ---- ⑤ 隐藏副作用：筛选器弹窗不得有横向滚动条 ----

test('[☰ 字段] 弹窗必须收敛长字段名，不产生横向滚动', () => {
  const src = read('BusinessColumnFilter.vue')
  assert.match(src, /\.field-popup-content \{[\s\S]{0,200}?box-sizing: border-box;/)
  assert.match(src, /overflow-x: hidden;/)
  assert.match(src, /text-overflow: ellipsis;/)
})

test('BusinessColumnFilter 的 <style> 是纯 CSS：不得出现 // 行注释', () => {
  // 真机踩过：在无 lang="less" 的 <style> 里写 `// 注释`，postcss 直接 500，
  // 整个页面白屏（Failed to fetch dynamically imported module）。修一次要重开浏览器。
  const src = read('BusinessColumnFilter.vue')
  const css = src.slice(src.indexOf('<style'), src.indexOf('</style>'))
  assert.ok(!/^\s*\/\//m.test(css), '纯 CSS 块里不得使用 // 注释')
})

test('BusinessColumnFilter 不得把 :deep 写在嵌套规则里（纯 CSS 会被拼成非法选择器）', () => {
  // 真机踩过：` .field-popup-item { :deep(.x) { ... } }` 编译成
  // `field-popup-item { &[data-v-x] { ... } }` —— 整块样式失效，横向滚动条照旧。
  const src = read('BusinessColumnFilter.vue')
  const css = src.slice(src.indexOf('<style'), src.indexOf('</style>'))
  assert.ok(!/^\s+:deep\(/m.test(css), ':deep 必须平铺在顶层')
  assert.match(css, /^\.field-popup-list :deep\(\.t-checkbox__label\)/m)
})
