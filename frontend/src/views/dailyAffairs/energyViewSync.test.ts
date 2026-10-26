import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { applySubsToColumns, hasUsableSubs, type EnergySub } from './energyViewColumns'

/**
 * 能耗 6 个卡片视图的「列表 ← 字段定义」六同步 + 本轮 8 项缺陷的回归契约。
 *
 * 这里锁的是几类"看着像后端问题、其实是前端契约错位"的坑：
 *   ① TDesign 的 rowKey 只吃字符串字段名——传函数 → 每行 key 都是 undefined
 *      → 勾选一行等于勾选全表；
 *   ② 列表列硬编码在组件里、设置抽屉读 subs——两套数据源，改名/改宽度/改顺序全不生效；
 *   ③ 解析失败的文件被当成"在途行"混进主列表，渲染出一条空壳；
 *   ④ 光伏账单的账单周期用起止日范围拼串，列宽必被截断；
 *   ⑤ 子组件工具栏还留着旧「删除历史」按钮 / 旧搜索文案。
 */

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')

const meter = read('./UtilityMeterTab.vue')
const tenant = read('./TenantBilling.vue')
const solar = read('./SolarManagement.vue')
const utilities = read('./UtilitiesManagement.vue')
const panel = read('./FleetCertPanel.vue')
const rulePanel = read('./ExtractRulePanel.vue')
const history = read('./FleetUploadHistoryDrawer.vue')
const settingDrawer = read('../../components/business/StandardSettingDrawer.vue')

// ---- applySubsToColumns：纯函数契约 ----

interface TestCol { key: string; label: string; default: boolean; w: string; width?: number }
const base: TestCol[] = [
  { key: 'month', label: '月份', default: true, w: '1fr' },
  { key: 'meter', label: '表计', default: true, w: '1fr' },
  { key: 'usage', label: '用量', default: true, w: '1fr' },
]

test('applySubsToColumns：顺序 / 改名 / 宽度 / 默认显示全部以 subs 为准', () => {
  const subs: EnergySub[] = [
    { name: 'usage', label: '用电量', is_default: true, width: 120 },
    { name: 'meter', label: '电表', is_default: true, width: 150 },
    { name: 'month', label: '月份', is_default: false },
  ]
  const cols = applySubsToColumns(base, subs)
  assert.deepEqual(cols.map((c) => c.key), ['usage', 'meter', 'month'], '顺序必须跟 subs 走')
  assert.equal(cols[0].label, '用电量')
  assert.equal(cols[0].width, 120)
  assert.equal(cols[2].default, false, '取消「默认表头」后列表必须收起该列')
  // 静态表里的渲染器信息要保留（w 用于无宽度时的兜底）
  assert.equal(cols[0].w, '1fr')
})

test('applySubsToColumns：停用的字段不产出列，subs 里没有的字段也不产出', () => {
  const subs: EnergySub[] = [
    { name: 'month', label: '月份', is_default: true },
    { name: 'meter', label: '电表', enabled: false, is_default: true },
  ]
  const cols = applySubsToColumns(base, subs)
  assert.deepEqual(cols.map((c) => c.key), ['month'], '停用要真的消失；usage 不在 subs 里也不得复活')
})

test('applySubsToColumns：subs 为空时回退静态列（不把表变成空表）', () => {
  assert.equal(hasUsableSubs([]), false)
  assert.equal(hasUsableSubs(undefined), false)
  const cols = applySubsToColumns(base, [])
  assert.deepEqual(cols.map((c) => c.key), ['month', 'meter', 'usage'])
})

test('applySubsToColumns：restrictToBase 用于双视角列表（并集不能变成多出的空列）', () => {
  const subs: EnergySub[] = [
    { name: 'month', label: '账单周期', is_default: true },
    { name: 'ratio', label: '分摊比例', is_default: true },
  ]
  const owner = applySubsToColumns(base, subs, undefined, { restrictToBase: true })
  assert.deepEqual(owner.map((c) => c.key), ['month'], '业主视角没有 ratio，不得凭空多出一列')
  const full = applySubsToColumns(base, subs, (key) => ({ key, label: key, default: false, w: '1fr' }))
  assert.deepEqual(full.map((c) => c.key), ['month', 'ratio'], '单视角表要把用户新增字段渲染出来')
})

test('applySubsToColumns：用户新增字段走 makeExtra 生成通用列', () => {
  const cols = applySubsToColumns(base, [
    { name: 'month', label: '月份', is_default: true },
    { name: 'custom_a', label: '自定义列', is_default: true, width: 90 },
  ], (key) => ({ key, label: key, default: false, w: '1fr' }))
  assert.equal(cols.length, 2)
  assert.equal(cols[1].label, '自定义列')
  assert.equal(cols[1].width, 90)
})

// ---- ① 行主键：勾选一行不能变全选 ----

test('核算表 row-key 必须是字符串字段名（传函数会让全表共用 undefined）', () => {
  assert.match(meter, /row-key="rowKey"/, 'row-key 必须是字符串 rowKey')
  assert.doesNotMatch(meter, /:row-key="\(row/, 'TDesign 1.19 的 rowKey 不接受函数')
  assert.match(meter, /rowKey: `\$\{rec\.id \|\| rec\.month \|\| 'rec'\}__/, '扁平行必须落一个非空 rowKey')
})

test('费用清单行主键必须带使用单位（同一月份多单位不能撞键）', () => {
  assert.match(tenant, /id: `row-\$\{useUnit\.value \|\| 'unit'\}__\$\{month\}`/, '主键需含单位')
})

// ---- ② 六同步：三张表都以 subs 为列权威源 ----

test('核算 / 费用清单 / 光伏 三张表的列定义都由 subs 派生', () => {
  for (const [name, src] of [['UtilityMeterTab', meter], ['TenantBilling', tenant], ['SolarManagement', solar]] as const) {
    assert.match(src, /applySubsToColumns\(/, `${name} 未接入 subs 派生`)
    assert.match(src, /loadEnergyCategorySubs\(/, `${name} 未拉取分类字段定义`)
    assert.match(src, /fleet-categories-changed/, `${name} 未订阅设置变更事件`)
  }
  // 分类名必须与设置抽屉逐字一致
  assert.match(meter, /electricity: '电费核算'/)
  assert.match(meter, /water: '水费核算'/)
  assert.match(meter, /gas: '气费核算'/)
  assert.match(tenant, /const CATEGORY_NAME = '费用清单'/)
  assert.match(solar, /const CATEGORY_NAME = '光伏账单'/)
})

test('表头宽度下传（改宽度要真的生效）', () => {
  assert.match(meter, /width: c\.width \|\| undefined/)
  assert.match(solar, /width: c\.width \|\| undefined/)
  // 费用清单有 24 列：宽度总和超过预算时整表回退自适应，避免横向滚动条
  assert.match(tenant, /const useFixed = wantWidth > 0 && wantWidth <= budget/)
  assert.match(tenant, /width: useFixed \? \(Number\(\(c as any\)\.width\) \|\| undefined\) : undefined/)
})

// ---- ③ 解析失败不进主列表 ----

test('市电 / 光伏 主列表都过滤解析失败行', () => {
  assert.match(utilities, /FAILED_ROW_STATUS = new Set\(\['failed', 'parse_failed'\]\)/)
  assert.match(utilities, /\.filter\(\(pf: any\) => !FAILED_ROW_STATUS\.has\(pendingLabel\(pf\)\)\)/)
  assert.match(solar, /FAILED_ROW_STATUS = new Set\(\['failed', 'parse_failed'\]\)/)
  assert.match(solar, /\.filter\(\(pf: any\) => !FAILED_ROW_STATUS\.has\(pendingLabel\(pf\)\)\)/)
})

// ---- ④ 光伏账单周期只显示 YYYY-MM ----

test('光伏账单周期用归一化后的 bill_period，不再拼起止日范围', () => {
  assert.doesNotMatch(solar, /bill_period: `\$\{it\.bill_period_start/, '不得再用「起 ~ 止」拼串')
  assert.match(solar, /bill_period: it\.bill_period \|\| monthOfDate\(it\.bill_period_start\)/)
})

// ---- ⑤ 工具栏清理 & 标签列样式 ----

test('光伏工具栏已删掉旧「删除历史」按钮与抽屉，搜索改为全字段', () => {
  assert.doesNotMatch(solar, /historyVisible/, '旧历史抽屉状态应已删除')
  assert.doesNotMatch(solar, /DeletedKnowledgeDrawer/, '旧历史抽屉组件应已删除')
  assert.doesNotMatch(solar, /content="删除历史"/, '旧历史图标按钮应已删除（按钮文案是 tooltip 的 content）')
  assert.doesNotMatch(solar, /@click="historyVisible = true"/, '旧历史按钮的点击入口应已删除')
  assert.match(solar, /placeholder="搜索全部字段"/, '搜索框占位符必须与发票一致')
})

test('光伏标签列与发票管理同构（.row-tag-add 胶囊）', () => {
  assert.match(solar, /<span class="row-tag-add">\+ 标签<\/span>/, '+ 标签 必须是胶囊而不是裸文字')
  // 市电账单本来就是这个结构，锁住防止回退
  assert.match(utilities, /<span class="row-tag-add">\+ 标签<\/span>/)
})

// ---- ⑥ 设置抽屉：核算分类不显示提取规则 ----

test('设置抽屉按分类白名单隐藏「提取规则」Tab', () => {
  assert.match(settingDrawer, /ruleCategories\?: string\[\]/)
  assert.match(settingDrawer, /<t-tab-panel v-if="showRulesTab" value="rules"/, '规则 Tab 必须可隐藏')
  assert.match(settingDrawer, /@category-change="onCategoryChange"/, '需要跟随字段定义里选中的分类')
  assert.match(panel, /emit\('category-change'/, 'FleetCertPanel 必须广播当前分类')
  // 能耗只放行市电/光伏
  assert.match(utilities, /const ENERGY_RULE_CATEGORIES = \['市电账单', '光伏账单'\]/)
  assert.match(utilities, /:rule-categories="ENERGY_RULE_CATEGORIES"/)
  assert.match(rulePanel, /ENERGY_EXTRACTABLE_CATEGORIES = \['市电账单', '光伏账单'\]/)
})

// ---- ⑦ 表头右侧：白底 + 遮罩同色 ----

test('能耗列表表头底色统一为白（否则右侧遮罩会露出白块）', () => {
  assert.match(utilities, /\.biz-page \.doc-list-view \.t-table th,/)
  assert.match(utilities, /background-color: var\(--td-bg-color-container\);/)
  assert.match(utilities, /border-bottom-color: var\(--td-component-border\)/)
})

// ---- ⑧ 历史记录抽屉：能耗分类名 ----

test('历史记录抽屉把 kind/bill_kind 映射成账单分类', () => {
  assert.match(history, /utility_bill: '市电账单'/)
  assert.match(history, /solar_bill: '光伏账单'/)
  assert.match(history, /electricity: '市电账单'/)
  assert.match(history, /energyCategoryOf\(/)
  assert.match(history, /props\.scope === 'energy' \? '账单类型'/, '能耗这一列叫账单类型')
  // 筛选与表格列必须同源
  assert.match(history, /return energyCategoryOf\(m\)/)
  assert.match(history, /doc_type: invDocType\(r\) \|\| energyCategoryOf\(/)
})

// ---- ⑨ 设置入口：每个卡片都能打开「字段定义 / 提取规则」 ----

test('光伏 / 核算 / 费用清单 工具栏都有「设置」入口，指向中台设置抽屉', () => {
  // 真机踩过：这三张表的工具栏「设置」原来打开的是各自的模块配置
  //（光伏基本户信息 / 表计档案 / 租户档案），所以用户点「设置」永远看不到
  //「字段定义」，以为这几类没有字段可配。
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar], ['TenantBilling', tenant]] as const) {
    assert.match(src, /function openFieldSettings\(\)/, `${name} 缺少打开字段设置的入口`)
    assert.match(src, /requestEnergySettings\(/, `${name} 未广播打开设置事件`)
  }
  // 模块配置按钮必须改名，避免和「设置」混淆
  assert.match(meter, /表计配置/, '核算表的表计档案入口应改名')
  assert.match(solar, /光伏设置/, '光伏基本户信息入口应改名')
  assert.match(tenant, /租户设置/, '租户档案入口应改名')
  // 页面接住事件并定位分类
  assert.match(utilities, /ENERGY_OPEN_SETTINGS_EVENT/, '页面必须监听打开设置事件')
  assert.match(utilities, /settingsCategoryOverride/, '需要按请求的分类定位抽屉')
})
