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
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar]] as const) {
    assert.match(src, /applySubsToColumns\(/, `${name} 未接入 subs 派生`)
    assert.match(src, /loadEnergyCategorySubs\(/, `${name} 未拉取分类字段定义`)
    assert.match(src, /fleet-categories-changed/, `${name} 未订阅设置变更事件`)
  }
  // 费用清单：内置列与跨分类引用列必须按**同一个 subs 主循环**产出（不许先内置后追加）。
  // 窗口放到 1200 字符：主循环里在 parseFieldSource 之前还有「合计应付 / 分摊子项」
  // 两个分支（多租户分组引入），但仍是同一个 for-of 循环、同一次下标遍历。
  // 主循环里多了一段「大类折叠」分支（市场化购电费 / 系统运行费用 / 政府性基金及附加），
  // 距 parseFieldSource 约 2KB，窗口给到 4000 字符。
  assert.match(tenant, /for \(const row of collapseFeeGroups\(effective\)\)[\s\S]{0,4000}?parseFieldSource\(sub\)/,
    'TenantBilling 未按 subs 顺序派生列')
  assert.match(tenant, /loadEnergyCategorySubs\(/, 'TenantBilling 未拉取分类字段定义')
  assert.match(tenant, /fleet-categories-changed/, 'TenantBilling 未订阅设置变更事件')
  // 分类名必须与设置抽屉逐字一致
  assert.match(meter, /electricity: '电费核算'/)
  assert.match(meter, /water: '水费核算'/)
  assert.match(meter, /gas: '气费核算'/)
  assert.match(tenant, /const CATEGORY_NAME = '费用清单'/)
  assert.match(solar, /const CATEGORY_NAME = '光伏账单'/)
})

test('表头宽度下传（改宽度要真的生效）', () => {
  // 宽度唯一权威源 = subs.width：>0 固定（clamp 60~400），0 才自适应。
  // 旧「总和 ≤1500 才生效」的预算守卫已删除——它会把用户填的宽度整体作废。
  assert.match(meter, /width: colWidthOf\(c\.width/, '核算表必须按 subs.width 渲染')
  assert.match(solar, /width: colWidthOf\(c\.width/, '光伏必须按 subs.width 渲染')
  assert.match(tenant, /width: Math\.min\(400, Math\.max\(cfg > 0 \? cfg : 0, minW\)\)/, '费用清单必须按 subs.width 渲染（带表头最小宽保底）')
  assert.doesNotMatch(tenant, /const budget = 1500/, '预算守卫必须删除')
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

test('光伏 / 核算 / 费用清单 工具栏都有「设置」入口，指向统一设置抽屉', () => {
  // 真机踩过两次：
  //   ① 这三张表的「设置」原来打开各自的模块配置（光伏基本户 / 表计档案 / 租户档案），
  //      用户点「设置」永远看不到「字段定义」；
  //   ② 改成广播事件又踩了第二个坑——抽屉挂在页面 v-if 分支里，切卡即卸载，
  //      点「设置」毫无反应。现在收敛为「子视图 emit → 页面根部常驻抽屉」。
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar], ['TenantBilling', tenant]] as const) {
    assert.match(src, /function openFieldSettings\(\)/, `${name} 缺少打开字段设置的入口`)
    assert.match(src, /emit\('open-settings'\)/, `${name} 应向父级请求统一设置抽屉`)
  }
  // 工具栏严格 3 个按钮：模块配置不再是独立按钮（只允许出现在抽屉 Tab 里）
  assert.doesNotMatch(meter, />\s*表计配置\s*</, '核算表工具栏不应再有独立的表计配置按钮')
  assert.doesNotMatch(solar, />\s*光伏设置\s*</, '光伏工具栏不应再有独立的光伏设置按钮')
  assert.doesNotMatch(tenant, />\s*租户设置\s*</, '费用清单工具栏不应再有独立的租户设置按钮')
  // 页面根部接住 emit，并定位到当前卡片的分类
  assert.match(utilities, /@open-settings="openChildSettings\('/, '页面必须接住子视图的设置请求')
  assert.match(utilities, /settingsCategoryOverride/, '需要按请求的分类定位抽屉')
  // 专属设置以抽屉 Tab 的形式内嵌，而不是另开弹窗
  assert.match(utilities, /:extra-tab-label="extraTabLabel"/, '页面必须把专属设置 Tab 交给抽屉')
  assert.match(settingDrawer, /name="extra-tab"/, '设置抽屉必须支持业务专属 Tab 插槽')
  assert.match(settingDrawer, /:lazy-load="false"/, '专属 Tab 必须随抽屉一起渲染，否则切 Tab 才挂载会空白')
})

test('分类编辑区块必须渲染在分类表之后（不能在行间展开把后续分类顶出视口）', () => {
  // 市电账单 176 个字段，编辑区块夹在行间时会把后面 5 个分类顶到 1400px 之外，
  // 用户看到的就是「分类表只有市电账单 1 行」。
  assert.doesNotMatch(panel, /v-if="editingId === item\.id"/, '编辑区块不得夹在分类行之间')
  assert.match(panel, /v-if="editingId && editingItem"/, '编辑区块应作为分类表之后的一段单独渲染')
  assert.match(panel, /const editingItem = computed/, '需要按 editingId 反查当前分类')
})

test('工具栏严格 3 个按钮：字段 → 设置 → 主操作', () => {
  // 光伏主按钮改名（现由 BusinessFilterBar 的 primaryAction 渲染）
  assert.match(solar, /text: '上传账单'/, '光伏主按钮必须叫「上传账单」')
  assert.doesNotMatch(solar, /上传光伏账单/, '光伏主按钮不得再叫「上传光伏账单」')
  // 三张表 / 费用清单的列显隐统一走中台 BusinessFilterBar（内部即 BusinessColumnFilter）
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar], ['TenantBilling', tenant]] as const) {
    assert.match(src, /<BusinessFilterBar/, `${name} 必须走中台 BusinessFilterBar`)
    assert.match(src, /emit\('open-settings'\)/, `${name} 的设置入口必须走父级统一抽屉`)
  }
})

test('市电/光伏都必须挂解析提取状态条，且上传与重新提取后立即刷新在途', () => {
  // 真机两处踩过：
  //   ① 光伏页压根没挂 BusinessParseStatusBar —— 解析提取几分钟里页面零反馈；
  //   ② 市电页挂了，但只用 3s 定时器刷新在途，点完「重新提取」状态条迟滞，
  //      用户以为没反应（大账单要跑 1~6 分钟）。
  assert.match(utilities, /<BusinessParseStatusBar/, '市电账单必须挂状态条')
  assert.match(solar, /<BusinessParseStatusBar/, '光伏账单必须挂状态条')
  // 在途数据源必须来自知识库原始文件列表，且暴露「立即刷一轮」的入口
  assert.match(utilities, /scanOnce/, '市电需要能立即扫一轮在途文件')
  assert.match(solar, /const pollOnce = async/, '光伏需要可立即执行的扫描函数')
  assert.match(solar, /const refreshPending = /, '光伏需要暴露 refreshPending')
  // 上传 / 重新提取后各调用一次
  const solarRefreshCalls = (solar.match(/refreshPending\(\)/g) || []).length
  assert.ok(solarRefreshCalls >= 3, `光伏上传与重提后都应立即刷新（当前 ${solarRefreshCalls} 处）`)
  const utilityScanCalls = (utilities.match(/void scanOnce\(\)/g) || []).length
  assert.ok(utilityScanCalls >= 2, `市电上传与重提后都应立即刷新（当前 ${utilityScanCalls} 处）`)
})

test('光伏编辑抽屉：五項示数/电量必须从 gateways 上提，保存时双向写回', () => {
  // 真机：列表已有 16.91 / 18.75 / 3000 / 5520 / 5520，抽屉里五个输入框全空。
  // 根因是抽屉直接铺了原始 item（五个值在 gateways[].readings[0] 里）。
  assert.match(solar, /function detailFormOf/, '抽屉表单初始化必须做电量明细上提')
  assert.match(solar, /detailForm\.value = detailFormOf\(row\.item\)/, 'openDetail 必须走上提后的表单')
  for (const k of ['meter_prev', 'meter_curr', 'meter_ratio', 'meter_reading', 'meter_bill']) {
    assert.ok(solar.includes(`lift('${k}'`), `抽屉必须上提 ${k}`)
  }
  assert.match(solar, /function syncSolarMeterReading/, '保存时必须把五项同步回 gateways')
  assert.match(solar, /gw\.readings\[0\] = \{ \.\.\.gw\.readings\[0\], \.\.\.meter \}/, '上网关口首行要写回同一份值')
})
