import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 费用清单第 5 轮真机问题的回归契约（2026-10-27 傍晚）。
 *
 * 这一轮三个问题都属「两套口径各写一半」或「一个开关兼两职」：
 *   ① 打印预览完全滚不动 —— `.ppd-viewer` 的 display 覆盖掉 viewer 的 flex，
 *      滚动体失去约束被撑到 6000px，clientHeight === scrollHeight；
 *   ② 基本电费 / 力调电费列恒为 0 —— 账单把这两项存在**概况字段**
 *      （capacity_fee / pf_adjust_amount）而不是 fee_items 行，前端只按名称
 *      在 fee_items 里找 → 一行都匹配不到；后端生成账单时读概况字段是对的，
 *      于是「记录里有值、列表里是 0」；
 *   ③ 打印清单漏过滤未参与分摊的子项 —— 旧实现直接铺账单 fee_items 全量，
 *      关掉开关的偏差电费等仍以 0.00 行打印出来。
 *   ④ 分摊子项缺「宽度(px)」「启用字段」两个维度。
 *
 * 这些都不会被类型检查发现，只能靠源码契约钉住。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const readGo = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

// ---- ① 打印预览必须能滚：display 不能被覆盖成 block ----

test('打印预览的 viewer 容器不得被覆盖成 display:block（会让滚动体失去 flex 父级）', () => {
  const src = read('PrintPreviewDialog.vue')
  const css = /\.ppd-viewer\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(css, '缺少 .ppd-viewer 样式定义')
  const body = css![1]
  assert.ok(!/display:\s*block/.test(body), '.ppd-viewer 不得为 display:block，滚动体将失去 flex 高度约束')
  assert.match(body, /display:\s*flex/)
  assert.match(body, /flex-direction:\s*column/)
  assert.match(body, /min-height:\s*0/)
})

test('打印预览 body 必须是可压缩的弹性区（父级定高才滚得动）', () => {
  const src = read('PrintPreviewDialog.vue')
  const css = /\.ppd-body\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(css, '缺少 .ppd-body 样式定义')
  const body = css![1]
  assert.match(body, /flex:\s*1/)
  assert.match(body, /min-height:\s*0/)
  // dialog 自身必须有确定高度，否则 flex:1 没有参照
  const dlg = /\.ppd-dialog\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(dlg && /height:\s*min\(/.test(dlg[1]), '弹窗必须有确定高度（min(...)）')
  assert.match(dlg![1], /flex-direction:\s*column/)
  // 操作按钮必须在独立 footer 里，不悬浮遮挡正文
  assert.match(src, /<footer class="ppd-footer">/)
  assert.ok(!/position:\s*fixed[^}]*ppd-actions/.test(src), '操作按钮不得悬浮覆盖正文')
})

test('PdfSourceViewer 的滚动体仍是 flex:1 + overflow:auto（外部不得改写）', () => {
  const src = readFileSync(new URL('../../components/source-preview/PdfSourceViewer.vue', import.meta.url), 'utf8')
  const css = /\.pdf-source-viewer__scroller\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(css, '缺少滚动体样式')
  assert.match(css![1], /flex:\s*1/)
  assert.match(css![1], /min-height:\s*0/)
  assert.match(css![1], /overflow:\s*auto/)
  // 根节点必须声明成 flex 列，滚动体才拿得到高度
  const root = /\.pdf-source-viewer\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(root && /flex-direction:\s*column/.test(root[1]), 'viewer 根节点必须是 flex 列')
})

// ---- ② 固定金额分项（基本电费 / 力调电费）必须走概况字段兜底 ----

test('固定金额分项必须有概况字段兜底（fee_items 里没有它们）', () => {
  const src = read('tenantBillingSources.ts')
  assert.match(src, /function fixedBillFee/)
  assert.match(src, /capacity_fee/)
  assert.match(src, /pf_adjust_amount/)
  // 匹配不到 fee_items 时必须落到概况字段，而不是直接 0
  const fn = src.slice(src.indexOf('function resolveFeeRow'), src.indexOf('function resolveFeeRow') + 900)
  assert.match(fn, /fixedBillFee\(ctx, name\)/)
})

test('「因素」与「因数」、「括号」必须归一（账单与配置常年对不上）', () => {
  const src = read('tenantBillingSources.ts')
  assert.match(src, /const aliasNorm =/)
  assert.match(src, /主要因素|因素\/g/)
  assert.match(src, /（[（）()]）|\[（）()\]|\(\)\)\[\]|（）\(\)/)
})

test('别名不得收录裸「容量电费」（会误伤燃气机组/煤电容量电费）', () => {
  const src = read('tenantBillingSources.ts')
  const capList = /const CAPACITY_ALIASES = \[([^\]]*)\]/.exec(src)
  assert.ok(capList, '缺少基本电费别名清单')
  const aliases = capList![1].split(',').map((s) => s.trim().replace(/^'|'$/g, ''))
  // 裸「容量电费」正是系统运行费下「燃气机组容量电费 / 煤电容量电费」的后缀，
  // 收录它会把这些正常分项全部改写成容量电费总额。
  assert.ok(!aliases.includes('容量电费'), `别名清单不得含裸「容量电费」：${aliases.join('/')}`)
  assert.ok(aliases.includes('基本电费'), '必须认「基本电费」')
})

test('费用分项匹配只有一处实现（列表与打印清单共用）', () => {
  const sources = read('tenantBillingSources.ts')
  assert.match(sources, /export function matchBillFeeItems/)
  const billing = read('TenantBilling.vue')
  assert.match(billing, /matchBillFeeItems/)
  // 打印细项表不得再自己写一套 name 匹配循环
  const feeTable = billing.slice(billing.indexOf('function feeTableOf'), billing.indexOf('function elecTableOf'))
  assert.ok(!/\.includes\(want\)/.test(feeTable), '打印细项表不得自建第二套名称匹配')
})

test('固定金额分项优先读概况字段（防止全 0 的同名占位行挡住权威值）', () => {
  const src = read('tenantBillingSources.ts')
  const fn = src.slice(src.indexOf('function resolveFeeRow'), src.indexOf('function resolveFeeRow') + 700)
  // 必须先判 fixedBillFee，再走 fee_items 匹配
  const iFixed = fn.indexOf('fixedBillFee(ctx, name)')
  const iMatch = fn.indexOf('matchBillFeeItems(ctx, name, group)')
  assert.ok(iFixed >= 0 && iMatch >= 0, '缺少两档判定')
  assert.ok(iFixed < iMatch, '固定金额分项必须优先：2026-08 存在 fee=0 的「功率因数调整」占位行会挡住 pf_adjust_amount')
})

// ---- ③ 租户打印清单只打印「参与分摊」的子项 ----

test('租户打印清单按「参与分摊」过滤，未开启的项不得打印 0.00 行', () => {
  const src = read('TenantBilling.vue')
  const feeTable = src.slice(src.indexOf('function feeTableOf'), src.indexOf('function elecTableOf'))
  assert.match(feeTable, /tenantApportionItems\.value/, '打印细项必须只遍历参与分摊的子项')
  // 业主视角仍走账单全量（全厂口径不分摊），两者必须分支
  assert.match(feeTable, /isOwnerView\.value/)
})

// ---- ④ 启用字段 / 宽度 三维正交 ----

test('「参与分摊」与「启用字段」是两个独立维度，不得互相过滤', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /field_enabled !== false/, '列表列必须按 field_enabled 过滤')
  assert.match(src, /tenantApportionItems/, '打印/合计必须按参与分摊过滤')
  // 只截取 tenantEnabledItems 这一个定义（到右括号为止）：
  // 紧随其后的 tenantApportionItems 本来就该用 enabled 过滤，不能算违规。
  const start = src.indexOf('const tenantEnabledItems = computed(')
  const end = src.indexOf(')', src.indexOf('field_enabled !== false', start))
  const block = src.slice(start, end + 1)
  assert.ok(!/it\.enabled !== false/.test(block), '列显隐不得用 enabled（那是分摊开关）')
  assert.match(block, /it\.field_enabled !== false/)
})

test('分摊子项面板必须是 6 列：名称/来源/参与分摊/宽度/启用字段/操作', () => {
  const src = read('TenantSettingsPanel.vue')
  const head = /item-row--head[\s\S]{0,700}?<\/div>/.exec(src)
  assert.ok(head, '缺少子项表头')
  const h = head![0]
  for (const label of ['子项名称', '来源', '参与分摊', '宽度(px)', '启用字段', '操作']) {
    assert.ok(h.includes(label), `表头缺少「${label}」`)
  }
  assert.match(src, /placeholder="0"[^>]*class="item-width-input"|class="item-width-input"[^>]*placeholder="0"/,
    '宽度必须是数字输入框')
  assert.match(src, /toggleFieldEnabled/)
  // 保存 payload 三个维度都要带上
  assert.match(src, /field_enabled:\s*it\.field_enabled !== false/)
  assert.match(src, /width:\s*Math\.max\(0, Math\.min\(400, Number\(it\.width\) \|\| 0\)\)/)
})

test('列宽 CSS 由子项 width 驱动（0=自适应）', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /width:\s*Number\(it\.width\) \|\| undefined/)
  // 第 16 轮：用户配置只允许更大，不允许小于表头文字所需宽度（避免表头被截断）
  assert.match(src, /width: Math\.min\(400, Math\.max\(cfg > 0 \? cfg : 0, minW\)\)/)
})

// ---- 后端：field_enabled 列与回填 ----

test('后端新增 field_enabled 列并在升级时回填为 true（避免列一次性全隐藏）', () => {
  const typ = readGo('internal/types/billing.go')
  assert.match(typ, /FieldEnabled\s+bool\s+`json:"field_enabled"`/)
  const h = readGo('internal/handler/billing.go')
  assert.match(h, /"FieldEnabled"/, 'EnsureBillingSchema 必须补该列')
  assert.match(h, /SET field_enabled = true WHERE field_enabled IS NOT TRUE/)
})

test('保存接口的 field_enabled 用指针接收且缺省为 true（旧前端不带该字段时不能隐藏列）', () => {
  const h = readGo('internal/handler/billing.go')
  assert.match(h, /FieldEnabled\s+\*bool\s+`json:"field_enabled"`/)
  // 指针判空后缺省为 true，再同一个事务里落库
  assert.match(h, /fieldEnabled := true[\s\S]{0,900}?FieldEnabled:\s+fieldEnabled/)
})
