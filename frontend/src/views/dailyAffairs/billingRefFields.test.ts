import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 费用清单第 6 轮真机问题的回归契约（2026-10-28）。
 *
 * 四项问题同源：**跨分类引用字段的「来源身份」丢了一级（分组）**，以及
 * **列可见性存档只做清洗、不做补新**。
 *   ① 新加的引用字段不显示列 —— localStorage 存档里没有它的键；
 *   ② 8 条同名「计费电量」无法区分 —— source 只记 category+field，
 *      候选 key 撞成同一个 `市电账单__qty`；
 *   ③ 引用字段名不可改 —— 模板里写死成只读 span；
 *   ④ 数值单元格没有口径说明。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const readGo = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

// ---- ① 列可见性：新列不能被旧存档挡住 ----

test('列存档必须「补新」：新增的默认列要自动进入可见列', () => {
  const src = read('TenantBilling.vue')
  const fn = src.slice(src.indexOf('function loadStoredKeys'), src.indexOf('function loadStoredKeys') + 1600)
  assert.match(fn, /fresh/, '缺少「新增列」识别')
  assert.match(fn, /!known\.has\(c\.key\)/, '必须能识别「存档里没提过的列」')
  assert.match(fn, /merged/, '必须把新增列并回可见列')
  // 关键反例：只过滤不补新的旧实现
  assert.ok(!/^\s*const valid = arr\.filter\(k => activeColDefs\.value\.some\(c => c\.key === k\)\)\s*$/m.test(fn),
    '不得退回「只过滤存档」的旧实现（新加字段会永远不显示）')
})

test('引用字段列由 subs 派生，且带 source 的列必须带上分组信息', () => {
  const src = read('TenantBilling.vue')
  const block = src.slice(src.indexOf('const extras: ColDef[] = []'), src.indexOf('return extras.length'))
  assert.match(block, /parseFieldSource/)
  assert.match(block, /enabled === false/)
  // 列 key 用 subs.name（数据身份），label 用 subs.label（可被重命名覆盖）
  assert.match(block, /key: String\(sub\.name\)/)
  assert.match(block, /label: String\(sub\.label \|\| sub\.name\)/)
})

// ---- ② 来源必须带分组层级 ----

test('后端 FleetCategorySubSource 必须持久化 group / group_label / label', () => {
  const src = readGo('internal/types/fleet.go')
  const blk = src.slice(src.indexOf('type FleetCategorySubSource struct'), src.indexOf('type FleetCategory struct'))
  assert.match(blk, /Group\s+string\s+`json:"group,omitempty"`/)
  assert.match(blk, /GroupLabel\s+string\s+`json:"group_label,omitempty"`/)
  assert.match(blk, /Label\s+string\s+`json:"label,omitempty"`/)
})

test('前后端 source 映射必须往返保留分组，不得被反序列化丢弃', () => {
  const src = read('fleetCategorySubs.ts')
  // 读入
  assert.match(src, /field\.source\.group = srcGroup/)
  assert.match(src, /field\.source\.groupLabel = gl/)
  // 写出
  assert.match(src, /payload\.source\.group = srcGroup/)
  assert.match(src, /payload\.source\.group_label = gl/)
})

test('候选 key 必须含分组（否则 8 条同名「计费电量」撞成同一个 key）', () => {
  const src = read('SourceFieldPicker.vue')
  assert.match(src, /const value = group \? `\$\{cat\}__\$\{group\}__\$\{field\}` : `\$\{cat\}__\$\{field\}`/)
  // 排除键也要同构，否则加一条会把同名的其它分组的候选一起隐藏
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /return \[s\.category, s\.group, s\.field\]\.filter\(Boolean\)\.join\('__'\)/)
})

test('引用字段名的内部身份也必须含分组（否则第二条被判「已添加」）', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /\['src', src\.category, src\.group, src\.field\]\.filter\(Boolean\)\.join\('_'\)/)
})

test('候选展示必须带二级分组路径，且支持「分组 字段」组合搜索', () => {
  const src = read('SourceFieldPicker.vue')
  assert.match(src, /const path = groupLabel \? `\$\{cat\} · \$\{groupLabel\}` : cat/)
  // 搜索覆盖 分类/分组中文名/分组键/字段名/字段键
  assert.match(src, /haystack: \[cat, groupLabel, group, name, field\]/)
  // 空格分词 AND
  assert.match(src, /const tokens = kw\.split\(\/\\s\+\/\)\.filter\(Boolean\)/)
  assert.match(src, /tokens\.every\(\(t\) => o\.haystack\.includes\(t\)\)/)
})

test('取值必须按分组定位，不得跨组求和', () => {
  const src = read('tenantBillingSources.ts')
  assert.match(src, /function resolveByBillGroup/)
  assert.match(src, /if \(src\.group\) \{/, 'resolveSourceValue 必须先按分组定位')
  // 单价不可相加：按金额÷数量加权
  const fn = src.slice(src.indexOf('function resolveByBillGroup'), src.indexOf('export function fixedBillFeeOf'))
  assert.match(fn, /if \(fieldKey === 'rate'\)/)
  assert.match(fn, /fee \/ qty/)
})

// ---- ③ 引用字段可独立重命名 ----

test('费用清单引用字段的名称必须是可编辑输入框（且保留只读来源徽标）', () => {
  const src = read('FleetCertPanel.vue')
  const blk = src.slice(src.indexOf('fc-name-wrap--ref'), src.indexOf('fc-name-wrap--ref') + 900)
  assert.match(blk, /<t-input/, '名称必须是可编辑输入框，不能是只读 span')
  assert.match(blk, /setFieldEdit\(fd, v\)/, '改名必须走 setFieldEdit（只写 label）')
  assert.match(blk, /fc-ref-badge/, '来源徽标必须保留（数据身份不可改）')
  assert.ok(!/fc-ref-name/.test(blk), '旧的只读名称 span 必须移除')
})

test('改名只写 label，绝不写 name / source（作用域隔离）', () => {
  const src = read('FleetCertPanel.vue')
  const fn = src.slice(src.indexOf('function setFieldEdit'), src.indexOf('function setFieldEdit') + 260)
  assert.match(fn, /fd\.label = String\(v \?\? ''\)/)
  assert.ok(!/fd\.name\s*=/.test(fn), 'setFieldEdit 不得改 name')
  assert.ok(!/source/.test(fn), 'setFieldEdit 不得碰 source')
})

test('来源徽标显示「分类 · 分组」，不再只显示分类', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /const sourcePathOf = \(fd: any\) => sourcePathText/)
  const src2 = read('tenantBillingSources.ts')
  assert.match(src2, /export function sourcePathText/)
  assert.match(src2, /if \(g\) parts\.push\(g\)/)
})

// ---- ④ 单元格悬浮说明 ----

test('单元格口径说明走「独立浮层组件 + document 级委托」，不是每格一个 tooltip', () => {
  const src = read('TenantBilling.vue')
  // 单元格只挂 data-tip，不实例化组件
  assert.match(src, /'data-tip': tip/)
  assert.ok(!/TTooltip|Tooltip as TTooltip/.test(src),
    '不得按单元格实例化 Tooltip：20+ 列 × N 行会互相抢占 hover 事件')
  // 浮层必须是独立组件：放在本页 setup 里每次 hover 都会重渲染整张 t-table，
  // 鼠标下的节点被替换 → 浮层刚出现就被自己关掉（真机表现为极不稳定）
  assert.match(src, /<BillingCellTip \/>/)
  assert.ok(!/const cellTip = ref/.test(src), '浮层状态不得留在列表页（会触发整表重绘）')
  const tip = read('BillingCellTip.vue')
  assert.match(tip, /document\.addEventListener\('mouseover', onOver, true\)/)
  assert.match(tip, /closest\?\.\('\[data-tip\]'\)/)
  // 浮层不吃指针事件，否则挡住 hover 目标把自己关掉
  assert.match(tip, /pointer-events:\s*none/)
})

test('口径说明必须写代入数值（内置列 + 分摊列 + 引用列三类）', () => {
  const tip = read('tenantBillingTips.ts')
  assert.match(tip, /export function buildRowTips/)
  assert.match(tip, /export function tenantItemTip/)
  assert.match(tip, /export function sourceFieldTip/)
  // 内置列：算式必须带实际数值，而不是只有公式
  assert.match(tip, /市电总电量 − 定比电量 − 租户工业分表电量/)
  assert.match(tip, /分摊比例/)
  assert.match(tip, /账单该项金额 × 分摊比例/)
})

test('提示里的数值必须与列表显示同源（由 buildRows 产出，不另算一遍）', () => {
  const src = read('TenantBilling.vue')
  // 业主视角
  assert.match(src, /base\.__tips = buildRowTips\(\{/)
  // 租户视角：分摊项提示
  assert.match(src, /itemTips\[tenantItemColKey\(it\)\] = tenantItemTip\(\{/)
  // 引用字段提示
  assert.match(src, /extraTips\[c\.key\] = sourceFieldTip\(\{/)
})

// ---- 分组归一与分组建模 ----

test('分组名必须做强归一（序号前缀 / 「性」/ 标点）后才能与账单 category 对上', () => {
  const src = read('tenantBillingSources.ts')
  assert.match(src, /const groupNorm = /)
  // 去序号前缀
  assert.match(src, /replace\(\/\^\\s\*\[（\(\]\?\\d\+\[\)）\]\?\[\.、\]\?\\s\*\/g, ''\)/)
  // 去「性」（政府性基金 = 政府基金）
  assert.match(src, /replace\(\/性\/g, ''\)/)
})

test('「电量明细」分组必须从 meter_readings / residential_readings 取值', () => {
  const src = read('tenantBillingSources.ts')
  const fn = src.slice(src.indexOf('function resolveByBillGroup'), src.indexOf('export function fixedBillFeeOf'))
  assert.match(fn, /want\.includes\('电量明细'\)/)
  assert.match(fn, /item\.residential_readings/)
  assert.match(fn, /item\.meter_readings/)
})
