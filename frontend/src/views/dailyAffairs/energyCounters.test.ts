import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

/**
 * 能耗 4 项收口（本轮）的回归契约：
 *   ① 卡片数字 === 列表底部「共 N 条」（同一份纯函数，禁止两套口径）；
 *   ② 字段宽度 subs.width>0 精确生效（禁止任何「总宽超阈值就整表自适应」的守卫）；
 *   ③ 三张表 / 费用清单的行点击统一走中台 BusinessSplitDetailDrawer；
 *   ④ 六视图工具栏统一 BusinessFilterBar，搜索框「搜索全部字段」。
 */

const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
const utilities = read('./UtilitiesManagement.vue')
const meter = read('./UtilityMeterTab.vue')
const solar = read('./SolarManagement.vue')
const tenant = read('./TenantBilling.vue')
const counters = read('./energyCounters.ts')
const toolbar = read('./BusinessListToolbar.vue')
const bsd = read('./BusinessSplitDetailDrawer.vue')

// ---- ① 计数口径 ----

test('卡片计数必须与列表同源：核算取展平行数、清单取月份并集', () => {
  // 原口径 records.length（3 个月 = 3 条）vs 列表 77 行 → 卡片永远小一个量级
  assert.match(utilities, /countMeterRowsOf/, '核算卡片必须走展平行数')
  assert.match(utilities, /billingRowCountOf/, '费用清单卡片必须走月份并集')
  assert.match(utilities, /countMeterRows\(/, '必须复用 energyCounters 的纯函数')
  assert.match(utilities, /billingRowCount\(/, '必须复用 energyCounters 的纯函数')
  assert.doesNotMatch(utilities, /async function countMeterRecords/, '旧的 records.length 口径必须删除')
  // 历史卡与抽屉同口径
  assert.match(utilities, /historyStats\(/, '历史卡计数必须复用抽屉同款过滤')
})

test('energyCounters 的口径函数是纯函数，可被两侧共用', () => {
  for (const fn of ['countMeterRows', 'billingRowCount', 'historyStats', 'rowFlattenText', 'matchAllTokens']) {
    assert.match(counters, new RegExp(`export function ${fn}`), `缺少 ${fn}`)
  }
  // 展平行数 = 所有 record.items 之和
  assert.match(counters, /n \+= \(rec\?\.items \|\| \[\]\)\.length/, '展平口径必须按 items 累加')
})

test('数据变更后卡片必须重算（保存/删除/生成都要广播）', () => {
  assert.match(counters, /export function notifyEnergyDataChanged/, '缺少变更广播')
  assert.match(utilities, /ENERGY_DATA_CHANGED_EVENT, onEnergyDataChanged/, '页面必须监听变更广播')
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar], ['TenantBilling', tenant]] as const) {
    assert.match(src, /notifyEnergyDataChanged\(\)/, `${name} 落库后必须广播`)
  }
})

// ---- ② 字段宽度 ----

test('禁止任何宽度预算守卫吞掉用户配置', () => {
  for (const [name, src] of [['TenantBilling', tenant], ['UtilityMeterTab', meter], ['SolarManagement', solar]] as const) {
    assert.doesNotMatch(src, /const budget = 1500/, `${name} 不得保留 1500px 预算守卫`)
    assert.doesNotMatch(src, /useFixed/, `${name} 不得按总宽回落自适应`)
  }
})

test('subs.width>0 必须精确生效（clamp 60~400），0 才自适应', () => {
  // 费用清单：用户配置宽度生效（clamp 400 上限），但不得小于表头文字所需宽度
  // —— 否则「上网环节线损费用」会被截成「上网环节线...」（第 16 轮）。
  assert.match(tenant, /width: Math\.min\(400, Math\.max\(cfg > 0 \? cfg : 0, minW\)\)/, '费用清单必须按配置宽度渲染（带表头最小宽保底）')
  assert.match(tenant, /function minColumnWidth\(c: ColDef\): number/, '必须有表头最小宽估算')
  // 三张表/光伏：走 colWidthOf（width>0 固定，dataType textarea 走中等宽，其余自适应）
  assert.match(meter, /colWidthOf\(c\.width/, '核算表列宽必须走 colWidthOf')
  assert.match(solar, /colWidthOf\(c\.width/, '光伏列宽必须走 colWidthOf')
  // 市电账单：subs.width 必须下传到 ColDef，否则填了不生效
  assert.match(utilities, /width: Number\(s\.width\) \|\| 0/, '市电 ColDef 必须携带 subs.width')
  assert.match(utilities, /if \(cfg > 0\) return Math\.min\(Math\.max\(cfg, 60\), 400\)/, '市电列宽必须优先 subs.width')
})

// ---- ③ 抽屉统一 ----

test('费用清单 / 光伏行点击统一 BusinessSplitDetailDrawer', () => {
  assert.match(tenant, /<BusinessSplitDetailDrawer/, '费用清单必须用 BSD 抽屉')
  assert.match(solar, /<BusinessSplitDetailDrawer/, '光伏必须用 BSD 抽屉')
  assert.match(utilities, /<BusinessSplitDetailDrawer/, '市电字段编辑必须用 BSD 抽屉')
  // 行点击必须真的打开抽屉（业主视角曾完全没反应）
  assert.match(tenant, /const onRowClick = \(row: any\) => \{[\s\S]{0,120}void openRecord\(row\)/, '费用清单行点击必须打开抽屉')
  assert.doesNotMatch(tenant, /if \(!isOwnerView\.value\) \{[\s\S]{0,40}records\.value\.find/, '不得只在租户视角打开')
  // 没有源文件的计算行：预览区必须可关
  assert.match(bsd, /showPreview\?: boolean/, 'BSD 需支持关闭源文件预览')
})

test('核算表记录抽屉走统一 SettingDrawer 底座', () => {
  const drawer = read('./MeterRecordDrawer.vue')
  assert.match(drawer, /<SettingDrawer/, '必须复用 SettingDrawer 底座')
  assert.match(drawer, /class="mr-grid"/, '必须是双列网格')
  assert.match(drawer, /t-form-item/, '必须用 t-form-item（控件右缘对齐）')
  assert.match(meter, /<MeterRecordDrawer/, '核算表必须使用该抽屉')
  // 旧自写抽屉必须消失
  assert.doesNotMatch(meter, /class="meter-drawer-body"/, '旧手写抽屉体必须淘汰')
})

// ---- ④ 工具栏统一 ----

test('六视图工具栏统一 BusinessFilterBar + 搜索全部字段', () => {
  for (const [name, src] of [['UtilityMeterTab', meter], ['SolarManagement', solar], ['TenantBilling', tenant]] as const) {
    assert.match(src, /<BusinessFilterBar/, `${name} 必须用 BusinessFilterBar`)
    assert.match(src, /search-placeholder="搜索全部字段"/, `${name} 占位符必须是「搜索全部字段」`)
    // 设置按钮：常规视图走 settings-text；费用清单的主操作需 loading/title，
    // 主按钮与设置按钮一起放 right-extra（BusinessFilterBar 透传该插槽）。
    assert.match(src, /settings-text="设置"|@click="openFieldSettings"/, `${name} 必须有设置按钮`)
    // 旧自写工具栏必须清除
    assert.doesNotMatch(src, /class="doc-filter-bar"/, `${name} 不得保留旧自写工具栏`)
    assert.doesNotMatch(src, /搜索表计\/使用单位/, `${name} 不得保留旧占位文案`)
  }
  // 市电账单本来就是 BusinessFilterBar
  assert.match(utilities, /<BusinessFilterBar/, '市电账单必须用 BusinessFilterBar')
})

test('搜索框 220×32 与发票逐像素一致，且支持全字段检索', () => {
  assert.match(toolbar, /\.doc-search-field \{ width: 220px; flex-shrink: 0; \}/, '搜索框必须 220px')
  assert.match(toolbar, /\.doc-search-field :deep\(\.t-input\) \{ height: 32px; \}/, '搜索框必须 32px 高')
  // 全字段检索：核算/清单都必须用 rowFlattenText + matchAllTokens
  assert.match(meter, /matchAllTokens\(rowFlattenText\(r\), kw\)/, '核算搜索必须匹配全部字段')
  assert.match(tenant, /matchAllTokens\(rowFlattenText\(r\), kw\)/, '清单搜索必须匹配全部字段')
})
