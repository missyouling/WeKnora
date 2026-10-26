import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

/**
 * 「四模块 + 能耗 6 视图」列表组件统一性的源码级契约。
 *
 * 背景：这几张表各自演化，出现过三类回归：
 *   ① 只有部分页面绑了表头遮罩变量 → 滚动条从容器顶端画到表头里；
 *   ② 子组件（核算/光伏/账单）没 import 共享样式 → `.doc-list-scroll`
 *      退化成 block + overflow:visible，表格不被限高、汇总栏被顶到几千像素；
 *   ③ 工具栏自带浮动批量栏 + 子组件又渲染一个 → 底部两个「已选」叠在一起。
 */

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
const shared = read('./styles/business-list-shared.css')
const composable = read('./useTableHeaderFix.ts')

const pages = {
  Invoice: read('./InvoiceManagement.vue'),
  Contract: read('./ContractManagement.vue'),
  Regulation: read('./RegulationManagement.vue'),
  Notice: read('./AwardPunishManagement.vue'),
  Utilities: read('./UtilitiesManagement.vue'),
  Meter: read('./UtilityMeterTab.vue'),
  Solar: read('./SolarManagement.vue'),
  Tenant: read('./TenantBilling.vue'),
}

test('共享样式提供表头滚动条遮罩与 sticky 表头层级', () => {
  assert.match(shared, /\.doc-list-view::after\s*\{/, '缺少表头遮罩')
  assert.match(shared, /width:\s*var\(--invoice-sbw, 0px\)/, '遮罩宽度必须取实测变量，默认 0')
  assert.match(shared, /height:\s*var\(--invoice-thh, 39px\)/, '遮罩高度必须取实测表头高')
  assert.match(shared, /\.doc-list-view::after\s*\{[\s\S]*?z-index:\s*20/, '遮罩 z-index 必须高于表头')
  assert.match(shared, /\.t-table__header--fixed > tr > th\s*\{[\s\S]*?z-index:\s*3/, 'sticky 表头需置顶')
})

test('8 个列表视图都绑定表头遮罩变量并使用统一量测', () => {
  for (const [name, src] of Object.entries(pages)) {
    assert.match(src, /class="doc-list-view"\s+:style="headerFixStyle"/, `${name} 的 .doc-list-view 未绑定 headerFixStyle`)
    assert.match(src, /useTableHeaderFix/, `${name} 未使用统一量测`)
  }
})

test('量测取真实内容区宽度差，且有界轮询到就有值', () => {
  assert.match(composable, /t-table__content/, '必须优先从真实内容区反推滚动条宽度')
  assert.match(composable, /offsetWidth - clientWidth/, '滚动条宽度 = offsetWidth - clientWidth')
  assert.match(composable, /setInterval/, '数据异步到达后需重测')
  assert.match(composable, /clearInterval/, '轮询必须有界，量到即停')
})

test('核算/光伏/账单子组件必须引入共享列表样式', () => {
  for (const name of ['Meter', 'Solar', 'Tenant'] as const) {
    assert.match(pages[name], /@import '\.\/styles\/business-list-shared\.css'/, `${name} 未引入共享列表样式`)
  }
})

test('8 个视图的列都以 [复选框] → [序号] 开头', () => {
  for (const [name, src] of Object.entries(pages)) {
    assert.match(src, /colKey:\s*'row-select'/, `${name} 复选框列缺少 colKey: 'row-select'`)
    assert.match(src, /colKey:\s*'serial-number'/, `${name} 缺少序号列`)
  }
})

test('核算表不再自带 TDesign 展开列（否则插在复选框左侧）', () => {
  assert.match(pages.Meter, /:expand-icon="false"/, '核算表需关闭自带展开列')
  assert.match(pages.Meter, /function toggleExpand/, '展开改由表计名前三角触发')
  assert.match(pages.Meter, /@click\.stop="toggleExpand\(row\)"/, '三角必须可点')
})

test('浮动批量栏全局唯一：工具栏不得再渲染一条', () => {
  assert.match(pages.Meter, /hide-batch-bar/, '核算工具栏必须关掉自带浮动栏')
})

test('汇总栏位于列表卡片内部（子组件模板顺序）', () => {
  for (const name of ['Meter', 'Solar', 'Tenant'] as const) {
    const src = pages[name]
    const idxT = src.lastIndexOf('</t-table>')
    const idxS = src.indexOf('doc-summary-bar', idxT)
    assert.ok(idxS > 0, `${name} 的汇总栏必须写在 </t-table> 之后（即卡片容器内部）`)
  }
})
