/**
 * 第 19 轮：禁用自动保存 / 后端 500 修复 / 双向同步 / 派生-分摊分组展示。
 *
 * 这些断言分两类：
 *   · 行为级（纯函数 tenantGroupSync）—— 同步与分组口径；
 *   · 源码级 —— 「不得再有自动保存」「必须接上同步」「后端三层保护」。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  itemFieldNameOf,
  itemsToFields,
  mergeItemsIntoFields,
  applyFieldsToItems,
  fieldOriginOf,
  originBadgeOf,
  splitFieldsByOrigin,
  topLevelFieldCount,
  ITEM_FIELD_PREFIX,
  type SyncField,
  type SyncItem,
} from './tenantGroupSync'
import {
  TENANT_DERIVED_FIELDS,
  TENANT_ELEC_TOTAL_FIELD,
  TENANT_WATER_TOTAL_FIELD,
  mergeTenantGroupFields,
  isTenantGroupMember,
} from './billingFieldGroups'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
const readGo = (rel: string) => readFileSync(new URL(`../../../../${rel}`, import.meta.url), 'utf8')

// ---- ① 禁用自动保存 ----

test('租户设置面板：开关 / 宽度 / 导入 / 增删都不得自动保存', () => {
  const src = read('TenantSettingsPanel.vue')
  // 宽度输入框不得再绑 blur / enter 保存
  assert.ok(!/@blur="saveItems/.test(src), '宽度输入框不得在失焦时保存')
  assert.ok(!/@enter="saveItems/.test(src), '宽度输入框不得在回车时保存')
  // toggle 函数体内不得出现 saveItems()（先剥掉注释：注释里会引用旧实现写法）
  const strip = (t: string) => t.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
  const toggleItem = strip(src.slice(src.indexOf('const toggleItem'), src.indexOf('const toggleFieldEnabled')))
  assert.ok(!/saveItems\(/.test(toggleItem), '参与分摊开关不得自动保存')
  const toggleField = strip(src.slice(src.indexOf('const toggleFieldEnabled'), src.indexOf('const importUtilityFields')))
  assert.ok(!/saveItems\(/.test(toggleField), '启用字段开关不得自动保存')
  // 导入市电账单字段后不得自动保存
  const importer = strip(src.slice(src.indexOf('const importUtilityFields'), src.indexOf('const addSourceItem')))
  assert.ok(!/await saveItems\(/.test(importer), '导入市电账单字段不得自动保存')
  // 保存按钮仍在，且是唯一落库入口
  assert.match(src, /@click="saveItems\(true\)"/, '必须保留手动「保存」按钮')
  assert.match(src, /saveBillingTenantItems/, '保存走 PUT items')
})

test('字段定义面板：拖拽排序与分类状态开关不得自动保存', () => {
  const src = read('FleetCertPanel.vue')
  const onDrop = src.slice(src.indexOf('async function onDrop'), src.indexOf('function onFieldDragStart'))
  assert.ok(!/await sortFleetCategories/.test(onDrop), '拖拽排序不得自动调 sortFleetCategories')
  const toggle = src.slice(src.indexOf('async function toggleEnabled'), src.indexOf('// ---- 删除（仅自定义'))
  assert.ok(!/await updateFleetCategory/.test(toggle), '分类启用开关不得自动保存')
  // 字段保存仍必须走手动「保存」（commitEdit）
  assert.match(src, /@click="commitEdit"/, '必须保留手动「保存」按钮')
})

test('表计设置面板：拖拽排序与启用开关不得自动保存', () => {
  const src = read('MeterSettingsPanel.vue')
  const sortFn = src.slice(src.indexOf('const persistMeterSort'), src.indexOf('watch([activeMeterGroup'))
  assert.ok(!/await sortUtilityMeters/.test(sortFn), '拖拽排序不得自动调 sortUtilityMeters')
  assert.match(sortFn, /pendingSortIds/, '拖拽结果必须暂存待保存时提交')
  const toggle = src.slice(src.indexOf('const toggleEnabled'), src.indexOf('const deleteMeter'))
  assert.ok(!/await updateUtilityMeter/.test(toggle), '表计启用开关不得自动保存')
  // 保存时把顺序一并提交
  const saveMeter = src.slice(src.indexOf('const saveMeter'), src.indexOf('const toggleEnabled'))
  assert.match(saveMeter, /sortUtilityMeters\(props\.category, pendingSortIds\.value\)/,
    '保存时必须提交暂存的排序')
})

test('提取规则面板：只有「保存」按钮触发写库（无 switch/blur 自动保存）', () => {
  const src = read('ExtractRulePanel.vue')
  assert.equal((src.match(/await saveExtractConfig\(/g) || []).length, 1, '写库只能有一处（save 函数内）')
  assert.match(src, /@click="save">保存<\/t-button>/, '必须保留手动保存按钮')
})

// ---- ② 后端 500 修复 ----

test('后端 SaveBillingTenantItems：串行 + 归一化去重 + ON CONFLICT 三层保护', () => {
  const go = readGo('internal/handler/billing.go')
  const fn = go.slice(go.indexOf('func (h *BillingHandler) SaveBillingTenantItems'),
    go.indexOf('// SaveBillingTenantRefs godoc'))
  assert.match(fn, /unlock := billingItemsLock\(id\)/, '必须按租户加锁串行')
  assert.match(fn, /defer unlock\(\)/, '锁必须 defer 释放')
  assert.match(fn, /normalizeBillingItemKey\(raw\)/, '入库前必须归一化 item_key')
  assert.match(fn, /if key == "" \|\| seen\[key\]/, '必须按归一化键去重')
  assert.match(fn, /clause\.OnConflict\{/, '必须有 ON CONFLICT 兜底')
  assert.match(fn, /Columns: \[\]clause\.Column\{\{Name: "billing_tenant_id"\}, \{Name: "item_key"\}\}/,
    '冲突目标必须是唯一键 (billing_tenant_id, item_key)')
  assert.match(fn, /h\.db\.WithContext\(ctx\)\.Transaction\(/, '删除与插入必须在同一事务')
  // 幂等与同义字归一
  const norm = go.slice(go.indexOf('func normalizeBillingItemKey'), go.indexOf('// SaveBillingTenantItems godoc'))
  assert.match(norm, /功率因素/, '必须归一「功率因素 → 功率因数」')
})

test('后端不再把并发唯一键冲突暴露成 500：错误路径只保留真正失败的日志', () => {
  const go = readGo('internal/handler/billing.go')
  // 成功响应必须带 200 + count（便于前端/验收确认真的写入了）
  assert.ok(go.includes('gin.H{"count": len(rows)}'), '成功响应必须带写入条数（便于验收确认真的落库）')
})

// ---- ③ 双向同步 ----

const TENANT = 'f5504893-c4aa-4cb0-9649-475ceb757fcc'
const GROUP = `tenant:${TENANT}`

const mkItem = (over: Partial<SyncItem> = {}): SyncItem => ({
  item_key: '基本电费|基本电费',
  item_name: '基本电费',
  category: '基本电费',
  enabled: true,
  field_enabled: true,
  width: 0,
  ...over,
})

test('itemFieldNameOf：与 TenantBilling 的列 key 严格一致（item_<item_key>）', () => {
  assert.equal(itemFieldNameOf({ item_key: '基本电费|基本电费' }), 'item_基本电费|基本电费')
  assert.equal(itemFieldNameOf({ itemKey: 'a|b' }), 'item_a|b')
  assert.equal(itemFieldNameOf({}), '')
})

test('itemsToFields：只收「启用字段」的子项，其余（参与分摊关掉但字段开着）照收', () => {
  const items = [
    mkItem({ item_key: 'k1', item_name: '甲', field_enabled: true }),
    mkItem({ item_key: 'k2', item_name: '乙', field_enabled: false }),
    // 参与分摊关掉、但启用字段开着 → 仍是列（与「参与分摊」正交）
    mkItem({ item_key: 'k3', item_name: '丙', enabled: false, field_enabled: true }),
  ]
  const names = itemsToFields(items, GROUP, '持睿汽车').map((f) => f.name)
  assert.deepEqual(names, ['item_k1', 'item_k3'])
})

test('mergeItemsIntoFields：保留用户配置，只增删', () => {
  const saved: SyncField[] = [
    { name: 'month', label: '账单周期', enabled: true, isDefault: true, dataType: 'text', width: 0, group: GROUP },
    { name: 'item_k1', label: '甲（自定义名）', enabled: true, isDefault: false, dataType: 'number', width: 180, group: GROUP },
    { name: 'item_k2', label: '乙', enabled: true, isDefault: true, dataType: 'number', width: 0, group: GROUP },
  ]
  const items = [
    mkItem({ item_key: 'k1', item_name: '甲' }),
    mkItem({ item_key: 'k3', item_name: '丙' }),
  ]
  const { fields, changed } = mergeItemsIntoFields(saved, items, GROUP, '持睿汽车')
  assert.equal(changed, true)
  const names = fields.map((f) => f.name)
  // k2 在子项侧已关掉「启用字段」→ 移除；k3 新增；month 原样保留
  assert.deepEqual(names, ['month', 'item_k1', 'item_k3'])
  // 用户配置一个都不覆盖
  const k1 = fields.find((f) => f.name === 'item_k1')!
  assert.equal(k1.label, '甲（自定义名）')
  assert.equal(k1.width, 180)
  assert.equal(k1.isDefault, false)
})

test('mergeItemsIntoFields：已同步时不产生变更（幂等）', () => {
  const items = [mkItem({ item_key: 'k1', item_name: '甲' })]
  const once = mergeItemsIntoFields([], items, GROUP, '持睿汽车').fields
  const twice = mergeItemsIntoFields(once, items, GROUP, '持睿汽车')
  assert.equal(twice.changed, false)
  assert.deepEqual(twice.fields.map((f) => f.name), once.map((f) => f.name))
})

test('applyFieldsToItems：字段定义改宽度/关启用 → 回写子项（不动「参与分摊」）', () => {
  const items = [
    mkItem({ item_key: 'k1', enabled: true, field_enabled: true, width: 0 }),
    mkItem({ item_key: 'k2', enabled: false, field_enabled: true, width: 0 }),
  ]
  const fields: SyncField[] = [
    { name: 'item_k1', enabled: true, isDefault: true, dataType: 'number', width: 180 },
    { name: 'item_k2', enabled: false, isDefault: true, dataType: 'number', width: 0 },
  ]
  const { items: next, changed } = applyFieldsToItems(items, fields)
  assert.equal(changed, true)
  assert.equal(next[0].width, 180, '宽度必须回写')
  assert.equal(next[0].field_enabled, true)
  assert.equal(next[1].field_enabled, false, '字段定义关掉启用 → 子项也关')
  // 「参与分摊」绝不被字段定义冲掉
  assert.equal(next[0].enabled, true)
  assert.equal(next[1].enabled, false)
})

test('applyFieldsToItems：字段定义里没有的 item_* → 子项置为不启用字段', () => {
  const items = [mkItem({ item_key: 'gone', field_enabled: true })]
  const { items: next, changed } = applyFieldsToItems(items, [])
  assert.equal(changed, true)
  assert.equal(next[0].field_enabled, false)
  assert.equal(next[0].enabled, true, '参与分摊保持不变')
})

test('双向同步接线：分摊子项保存后同步 subs；字段定义保存后回写 items', () => {
  const panel = read('TenantSettingsPanel.vue')
  assert.match(panel, /syncItemsToCostList/, '必须实现「子项 → 字段定义」同步')
  const saveItems = panel.slice(panel.indexOf('const saveItems'), panel.indexOf('onMounted('))
  assert.match(saveItems, /await saveBillingTenantItems/, '先存子项')
  assert.match(saveItems, /await syncItemsToCostList\(items\)/, '再同步到字段定义')
  assert.match(panel, /mergeItemsIntoFields\(/, '同步必须走统一的合并口径')

  const cert = read('FleetCertPanel.vue')
  assert.match(cert, /async function syncFieldsToTenantItems/, '必须实现「字段定义 → 子项」回写')
  assert.match(cert, /applyFieldsToItems\(items, groupFields\)/, '回写必须走统一口径')
  assert.match(cert, /await saveBillingTenantItems\(tenantId, \{ items: next \}\)/, '回写走 PUT items')
  const commit = cert.slice(cert.indexOf('async function commitEdit'), cert.indexOf('async function syncFieldsToTenantItems'))
  assert.match(commit, /void syncFieldsToTenantItems\(item, fieldsEditable\.value\)/, '保存后必须触发回写')
})

// ---- ④ 派生 / 分摊子项分组展示 ----

test('fieldOriginOf：fee_group → 分摊汇总；item_* → 分摊子项；其余 → 派生字段', () => {
  assert.equal(fieldOriginOf({ name: 'grp_market', source: { mode: 'fee_group', members: ['a', 'b'] } }), 'feeGroup')
  assert.equal(fieldOriginOf({ name: 'item_基本电费|基本电费' }), 'item')
  assert.equal(fieldOriginOf({ name: 'month' }), 'derived')
  assert.equal(fieldOriginOf({ name: 'elec_total_fee' }), 'derived')
  // 派生字段也可能挂 source（跨分类引用），仍属派生
  assert.equal(fieldOriginOf({ name: 'ind_fee', source: { category: '电费核算', field: 'amount', mode: 'field' } }), 'derived')
})

test('originBadgeOf：分摊汇总带细项数量，子项与派生各用自己的文案', () => {
  assert.deepEqual(originBadgeOf({ name: 'grp_market', source: { mode: 'fee_group', members: ['a', 'b', 'c'] } }),
    { text: '分摊汇总 · 细项 3', kind: 'feeGroup' })
  assert.deepEqual(originBadgeOf({ name: 'grp_market', source: { mode: 'fee_group' } }),
    { text: '分摊汇总', kind: 'feeGroup' })
  assert.deepEqual(originBadgeOf({ name: 'item_x|y' }), { text: '分摊子项', kind: 'item' })
  assert.deepEqual(originBadgeOf({ name: 'month' }), { text: '派生字段', kind: 'derived' })
})

test('splitFieldsByOrigin：派生在前、子项在后，区块内保持原相对顺序', () => {
  const fields = [
    { name: 'month' },
    { name: 'item_a|a' },
    { name: 'ind_fee' },
    { name: 'grp_market', source: { mode: 'fee_group' } },
    { name: 'elec_total_fee' },
  ]
  const { derived, items } = splitFieldsByOrigin(fields)
  assert.deepEqual(derived.map((f) => f.name), ['month', 'ind_fee', 'elec_total_fee'])
  assert.deepEqual(items.map((f) => f.name), ['item_a|a', 'grp_market'])
})

test('topLevelFieldCount：折叠在大类里的成员不重复计入', () => {
  const fields: any[] = [
    { name: 'month' },
    { name: 'ind_fee' },
    { name: 'item_独立项|独立项' },
    { name: 'grp_market', source: { mode: 'fee_group', members: ['item_a|a', 'item_b|b'] } },
    { name: 'item_a|a' },
    { name: 'item_b|b' },
  ]
  assert.equal(topLevelFieldCount(fields), 4, 'month + ind_fee + 独立项 + 大类 = 4')
})

test('字段定义面板：租户分组插「派生字段 / 分摊子项」区块标题', () => {
  const src = read('FleetCertPanel.vue')
  assert.match(src, /const fieldRows = computed/, '必须有行序列（标题 + 字段行）')
  assert.match(src, /isTenantGroupActive/, '必须只对租户分组生效')
  assert.match(src, /startsWith\('tenant:'\)/, '租户分组键必须以 tenant: 开头')
  assert.match(src, /title: '派生字段'/, '必须有「派生字段」区块标题')
  assert.match(src, /title: '分摊子项'/, '必须有「分摊子项」区块标题')
  assert.match(src, /fc-origin-block--/, '必须有区块样式类')
})

/**
 * `.field-config-row` 必须与 `.field-colhead` 严格同构（第 20 轮真机回归）。
 *
 * 现象：在 7 列 Grid 里多插一个独立 `<span class="fc-origin-badge">` 子节点，
 * 后面所有列整体右移一格 —— 字段名输入框被挤进 90px 的类型列、
 * ⋮ 掉到第二行左下角（用户截图）。来源改由上方区块标题表达。
 */
test('字段行严格 7 列：不得再有独立来源徽标节点', () => {
  const src = read('FleetCertPanel.vue')
  assert.ok(!/fc-origin-badge/.test(src), '独立来源徽标节点必须移除（会破坏 Grid 列数）')
  // 行首必须是拖拽手柄，紧接着就是字段名/类型/宽度/默认表头/启用/操作
  const rowStart = src.slice(src.indexOf('class="field-config-row"'), src.indexOf('class="field-config-row"') + 700)
  assert.match(rowStart, /^class="field-config-row"[\s\S]{0,400}?<span class="fc-drag"/, '行首必须是拖拽手柄')
  assert.ok(!/fc-origin-badge/.test(rowStart), '字段行内不得出现徽标')
})

test('胶囊字段数按顶层行重算（不再把折叠成员计入）', () => {
  const src = read('FleetCertPanel.vue')
  const tabs = src.slice(src.indexOf('const subGroupTabs'), src.indexOf('const fieldsVisible'))
  assert.match(tabs, /collapseFeeGroups\(groupFields as any\[\]\)\.length/,
    '胶囊计数必须用 collapseFeeGroups 的顶层行数')
})

// ---- 跨分组污染守卫（第 19 轮真机暴露）--------------------------------------
//
// 真机现象：字段定义「持睿汽车」42 行里有 16 行是**业主视角（默认分组）**的列
// （总电量 / 总电费 / 光伏发电量 / 上网电量 / 结算金额 / 本期电量 / 居民电量 /
// 居民电费 / src_市电账单_*），于是「账单周期」「使用单位」「光伏发电量」
// 在同一列表里各出现两遍。根因：mergeTenantGroupFields 会给留下的字段重打 group，
// 早期版本把整份 subs 传进来，这些外来字段就被永久钉成了租户分组字段。

test('isTenantGroupMember：只认派生字段 / 分摊子项 / 大类汇总', () => {
  for (const n of ['month', 'unit', 'ratio', 'ind_kwh', 'ind_fee', 'ind_price',
    'dorm_kwh', 'dorm_fee', 'water_ind_usage', 'water_ind_fee',
    'water_dorm_usage', 'water_dorm_fee', 'gas_usage', 'gas_fee', 'remark',
    'elec_total_fee', 'water_total_fee', 'grp_market', 'grp_system', 'grp_gov']) {
    assert.equal(isTenantGroupMember(n), true, `${n} 必须算租户分组成员`)
  }
  for (const n of ['item_(4)系统运行费|煤电容量电费', 'item_基本电费|基本电费']) {
    assert.equal(isTenantGroupMember(n), true, `${n} 分摊子项必须算租户分组成员`)
  }
  // 默认分组的列 + 跨分类引用列：一律不得进租户分组
  for (const n of ['total_kwh', 'total_fee', 'solar_gen', 'solar_grid', 'solar_amount',
    'res_kwh', 'res_fee', 'water_fire_usage', 'water_fire_fee',
    'src_市电账单_overview_total_kwh', 'src_市电账单_overview_residential_amount',
    'src_光伏账单_overview_generation_kwh', 'src_电费核算_cols_use_unit']) {
    assert.equal(isTenantGroupMember(n), false, `${n} 属于默认分组，不得进租户分组`)
  }
})

test('mergeTenantGroupFields：混进默认分组的列必须被剔除（真机 42 → 26 行）', () => {
  // 复刻真机被污染的入库数据：租户分组里混着默认分组的 16 列
  const polluted: any[] = [
    ...TENANT_DERIVED_FIELDS.map((f) => ({
      name: f.name, label: f.label, enabled: true, isDefault: f.isDefault,
      dataType: f.dataType, width: 0, group: 'tenant:t1', groupLabel: '持睿汽车',
    })),
    // 污染项：默认分组的内置列 + 跨分类引用列
    ...[
      ['total_kwh', '总电量'], ['total_fee', '总电费'],
      ['solar_gen', '光伏发电量'], ['solar_grid', '上网电量'], ['solar_amount', '结算金额'],
      ['res_kwh', '电量（定比）'], ['res_fee', '电费（定比）'],
      ['water_fire_usage', '用水量（消防）'], ['water_fire_fee', '水费（消防）'],
      ['src_市电账单_overview_total_kwh', '本期电量'],
      ['src_市电账单_overview_total_amount', '本期电费'],
      ['src_市电账单_catalog_qty', '居民电量'],
      ['src_市电账单_overview_residential_amount', '居民电费'],
      ['src_市电账单_overview_bill_period', '账单周期'],
      ['src_电费核算_cols_use_unit', '使用单位'],
      ['src_光伏账单_overview_settlement_amount', '结算金额'],
    ].map(([name, label]) => ({
      name, label, enabled: true, isDefault: true, dataType: 'number', width: 0,
      group: 'tenant:t1', groupLabel: '持睿汽车',
    })),
  ]
  const merged = mergeTenantGroupFields(polluted, { id: 't1', name: '持睿汽车' }, [], '持睿汽车')
  const names = merged.map((f) => f.name)
  // 污染项一个都不能留
  for (const bad of ['total_kwh', 'total_fee', 'solar_gen', 'solar_grid', 'solar_amount',
    'res_kwh', 'res_fee', 'water_fire_usage', 'water_fire_fee',
    'src_市电账单_overview_total_kwh', 'src_市电账单_overview_total_amount',
    'src_市电账单_catalog_qty', 'src_市电账单_overview_residential_amount',
    'src_市电账单_overview_bill_period', 'src_电费核算_cols_use_unit',
    'src_光伏账单_overview_settlement_amount']) {
    assert.ok(!names.includes(bad), `${bad} 必须被剔除（属于默认分组）`)
  }
  // 同一列表里不得有重名（真机「账单周期」「使用单位」各出现两遍）
  assert.equal(new Set(names).size, names.length, '字段名不得重复')
  // 派生字段 + 两条收口列都在
  for (const f of TENANT_DERIVED_FIELDS) {
    assert.ok(names.includes(f.name), `${f.label} 必须保留`)
  }
  assert.deepEqual(names.slice(-2), [TENANT_ELEC_TOTAL_FIELD, TENANT_WATER_TOTAL_FIELD])
})
