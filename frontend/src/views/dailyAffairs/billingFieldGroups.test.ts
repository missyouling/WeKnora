import test from 'node:test'
import assert from 'node:assert/strict'
import {
  COST_LIST_CATEGORY,
  DEFAULT_GROUP_KEY,
  DEFAULT_GROUP_LABEL,
  TENANT_DERIVED_FIELDS,
  TENANT_TOTAL_FIELD,
  TENANT_FEE_GROUPS,
  buildTenantGroupFields,
  collapseFeeGroups,
  costListGroupLabel,
  feeGroupSpecOfCategory,
  feeGroupSpecOfSub,
  itemCategoryOfName,
  mergeFeeGroupFields,
  isTenantGroupKey,
  isTenantItemVisible,
  mergeTenantGroupFields,
  normalizeCostListDefaults,
  normalizeCostListGroup,
  sortTenantItems,
  subsOfGroup,
  tenantGroupKey,
  tenantGroupNeedsSync,
  tenantIdOfGroup,
  tenantItemSubName,
  withCostListGroups,
} from './billingFieldGroups'

const TENANT = { id: 'f5504893-c4aa-4cb0-9649-475ceb757fcc', name: '持睿汽车' }

const items = [
  { item_key: '(1)市场化购电费|零售交易电费', item_name: '零售交易电费', enabled: true, field_enabled: true, sort: 2 },
  { item_key: '基本电费|基本电费', item_name: '基本电费', enabled: true, field_enabled: true, sort: 1 },
  { item_key: '功率因素调整电费|功率因素调整电费', item_name: '功率因素调整电费', enabled: true, field_enabled: true, sort: 3 },
  { item_key: '辅助服务费用|辅助服务费用', item_name: '辅助服务费用', enabled: false, field_enabled: false, sort: 4 },
]

test('分类名常量与设置抽屉里的分类名逐字一致', () => {
  assert.equal(COST_LIST_CATEGORY, '费用清单')
})

test('tenantGroupKey / isTenantGroupKey / tenantIdOfGroup 互逆', () => {
  const k = tenantGroupKey(TENANT.id)
  assert.equal(k, `tenant:${TENANT.id}`)
  assert.equal(isTenantGroupKey(k), true)
  assert.equal(isTenantGroupKey(DEFAULT_GROUP_KEY), false)
  assert.equal(tenantIdOfGroup(k), TENANT.id)
  assert.equal(tenantIdOfGroup(DEFAULT_GROUP_KEY), '')
})

test('空租户 id 回落到默认分组（不产出非法 group 键）', () => {
  assert.equal(tenantGroupKey(''), DEFAULT_GROUP_KEY)
  assert.equal(tenantGroupKey('   '), DEFAULT_GROUP_KEY)
})

test('存量 group=cols 归一到默认分组，其它键原样保留', () => {
  assert.equal(normalizeCostListGroup('cols'), DEFAULT_GROUP_KEY)
  assert.equal(normalizeCostListGroup(''), DEFAULT_GROUP_KEY)
  assert.equal(normalizeCostListGroup(undefined), DEFAULT_GROUP_KEY)
  assert.equal(normalizeCostListGroup('default'), DEFAULT_GROUP_KEY)
  assert.equal(normalizeCostListGroup('tenant:abc'), 'tenant:abc')
  assert.equal(normalizeCostListGroup('market'), 'market')
})

test('分组显示名：默认分组 / 租户名 / 未知键回落键名', () => {
  assert.equal(costListGroupLabel('cols'), DEFAULT_GROUP_LABEL)
  assert.equal(costListGroupLabel('tenant:x', '持睿汽车'), '持睿汽车')
  assert.equal(costListGroupLabel('tenant:x'), 'tenant:x')
})

test('subsOfGroup 按分组过滤，且 cols 与 default 视为同一组', () => {
  const subs = [
    { name: 'a', group: 'cols' },
    { name: 'b', group: 'default' },
    { name: 'c', group: 'tenant:t1' },
    { name: 'd' },
  ]
  assert.deepEqual(subsOfGroup(subs, 'default').map((s) => s.name), ['a', 'b', 'd'])
  assert.deepEqual(subsOfGroup(subs, 'tenant:t1').map((s) => s.name), ['c'])
})

test('分摊子项可见性看 field_enabled（与 enabled=参与分摊 正交）', () => {
  assert.equal(isTenantItemVisible({ field_enabled: undefined }), true)
  assert.equal(isTenantItemVisible({ field_enabled: false }), false)
  assert.equal(isTenantItemVisible({ field_enabled: true, enabled: false }), true)
  assert.equal(isTenantItemVisible(null), false)
})

test('分摊子项字段名与 TenantBilling 列 key 同构', () => {
  assert.equal(tenantItemSubName({ item_key: '基本电费|基本电费' }), 'item_基本电费|基本电费')
  assert.equal(tenantItemSubName({ itemKey: 'k' }), 'item_k')
  assert.equal(tenantItemSubName({}), '')
})

test('sortTenantItems 不改原数组', () => {
  const raw = [{ sort: 2, item_key: 'b' }, { sort: 1, item_key: 'a' }]
  const sorted = sortTenantItems(raw)
  assert.deepEqual(sorted.map((x) => x.item_key), ['a', 'b'])
  assert.deepEqual(raw.map((x) => x.item_key), ['b', 'a'])
})

test('buildTenantGroupFields：派生字段 → 分摊子项 → 合计应付，且只收可见子项', () => {
  const fields = buildTenantGroupFields(TENANT, items)
  const names = fields.map((f) => f.name)
  // 派生字段按固定顺序打头
  assert.deepEqual(names.slice(0, TENANT_DERIVED_FIELDS.length), TENANT_DERIVED_FIELDS.map((d) => d.name))
  // 子项按 sort 排序，被关掉「启用字段」的不进来。
  // (1) 市场化购电费 只有 1 个可见子项 → 仍折叠成大类列（成员留在 subs 里），
  // 所以这里看到的是 `grp_market` 而不是 `item_(1)…|零售交易电费`。
  assert.deepEqual(names.slice(TENANT_DERIVED_FIELDS.length, -1), [
    'item_基本电费|基本电费',
    'grp_market',
    'item_(1)市场化购电费|零售交易电费',
    'item_功率因素调整电费|功率因素调整电费',
  ])
  assert.equal(names[names.length - 1], TENANT_TOTAL_FIELD)
  // 每条都带本租户的 group，否则胶囊条会把它们算进默认分组
  const g = tenantGroupKey(TENANT.id)
  assert.ok(fields.every((f) => f.group === g))
  assert.ok(fields.every((f) => f.groupLabel === '持睿汽车'))
  // 派生字段的中文名（产品口径）
  const byName = new Map(fields.map((f) => [f.name, f]))
  assert.equal(byName.get('month')?.label, '账单周期')
  assert.equal(byName.get('unit')?.label, '使用单位')
  assert.equal(byName.get('ratio')?.label, '分摊比例')
  assert.equal(byName.get('ind_kwh')?.label, '工业电量')
  assert.equal(byName.get('ind_fee')?.label, '工业电费')
  assert.equal(byName.get('ind_price')?.label, '工业均价')
})

test('buildTenantGroupFields：没有子项时只留派生字段 + 合计应付', () => {
  const names = buildTenantGroupFields(TENANT, []).map((f) => f.name)
  assert.equal(names.length, TENANT_DERIVED_FIELDS.length + 1)
  assert.equal(names[names.length - 1], TENANT_TOTAL_FIELD)
})

test('mergeTenantGroupFields：保留用户排序/改名/宽度/开关，不覆盖', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  // 用户把「工业电费」拖到最前、改名、加宽、关掉默认表头
  const indFee = saved.find((f) => f.name === 'ind_fee')!
  indFee.label = '工业电费（元）'
  indFee.width = 180
  indFee.isDefault = false
  indFee.enabled = false
  const reordered = [indFee, ...saved.filter((f) => f.name !== 'ind_fee')]

  const merged = mergeTenantGroupFields(reordered, TENANT, items)
  assert.equal(merged[0].name, 'ind_fee')
  assert.equal(merged[0].label, '工业电费（元）')
  assert.equal(merged[0].width, 180)
  assert.equal(merged[0].isDefault, false)
  assert.equal(merged[0].enabled, false)
  // 顺序未被重排（其它字段保持原相对顺序）
  assert.deepEqual(merged.map((f) => f.name), reordered.map((f) => f.name))
})

test('mergeTenantGroupFields：清单设置新开启的子项追加到合计应付之前', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  const extra = [...items, { item_key: '新增项|新增项', item_name: '新增项', enabled: true, field_enabled: true, sort: 9 }]
  const merged = mergeTenantGroupFields(saved, TENANT, extra)
  const names = merged.map((f) => f.name)
  assert.equal(names[names.length - 1], TENANT_TOTAL_FIELD)
  assert.equal(names[names.length - 2], 'item_新增项|新增项')
})

test('mergeTenantGroupFields：清单设置关掉的子项从分组移除', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  const next = items.map((it) => (it.item_key === '基本电费|基本电费' ? { ...it, field_enabled: false } : it))
  const merged = mergeTenantGroupFields(saved, TENANT, next)
  assert.equal(merged.some((f) => f.name === 'item_基本电费|基本电费'), false)
})

test('mergeTenantGroupFields：老数据缺派生字段时补齐，合计应付只出现一次且在最后', () => {
  const saved = [
    { name: 'month', label: '账单周期', enabled: true, isDefault: true, dataType: 'text', width: 0 },
    { name: 'unit', label: '使用单位', enabled: true, isDefault: true, dataType: 'text', width: 0 },
  ]
  const merged = mergeTenantGroupFields(saved, TENANT, items)
  const names = merged.map((f) => f.name)
  assert.equal(names.filter((n) => n === TENANT_TOTAL_FIELD).length, 1)
  assert.equal(names[names.length - 1], TENANT_TOTAL_FIELD)
  for (const d of TENANT_DERIVED_FIELDS) assert.ok(names.includes(d.name), `缺少 ${d.name}`)
  // 已保存项不被重复追加
  assert.equal(names.filter((n) => n === 'month').length, 1)
})

test('mergeTenantGroupFields：中间位置的合计应付被挪到末尾（收口列）', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  const total = saved.find((f) => f.name === TENANT_TOTAL_FIELD)!
  const moved = [total, ...saved.filter((f) => f.name !== TENANT_TOTAL_FIELD)]
  const merged = mergeTenantGroupFields(moved, TENANT, items)
  assert.equal(merged[merged.length - 1].name, TENANT_TOTAL_FIELD)
  assert.equal(merged.filter((f) => f.name === TENANT_TOTAL_FIELD).length, 1)
})

test('withCostListGroups：存量 cols 归一到默认分组，租户分组自动补出', () => {
  const legacy = [
    { name: 'src_x', label: '账单周期', enabled: true, isDefault: true, dataType: 'text', width: 0, group: 'cols', groupLabel: '费用清单' },
    { name: 'ind_kwh', label: '工业电量', enabled: true, isDefault: true, dataType: 'number', width: 0, group: 'cols', groupLabel: '费用清单' },
  ]
  const out = withCostListGroups(legacy, [TENANT], { [TENANT.id]: items })
  const defaults = out.filter((f) => f.group === DEFAULT_GROUP_KEY)
  assert.deepEqual(defaults.map((f) => f.name), ['src_x', 'ind_kwh'])
  assert.ok(defaults.every((f) => f.groupLabel === DEFAULT_GROUP_LABEL))
  const tenantFields = out.filter((f) => f.group === tenantGroupKey(TENANT.id))
  assert.ok(tenantFields.length >= TENANT_DERIVED_FIELDS.length + 1)
  assert.ok(tenantFields.some((f) => f.name === 'item_基本电费|基本电费'))
  // 默认分组在前，租户分组在后（与胶囊条顺序一致）
  assert.equal(out[0].group, DEFAULT_GROUP_KEY)
  assert.equal(out[out.length - 1].group, tenantGroupKey(TENANT.id))
})

test('withCostListGroups：已保存的租户分组不重复生成', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  const out = withCostListGroups(saved, [TENANT], { [TENANT.id]: items })
  const tenantFields = out.filter((f) => f.group === tenantGroupKey(TENANT.id))
  assert.equal(tenantFields.length, saved.length)
})

test('withCostListGroups：多个租户各出一个分组，互不串字段', () => {
  const t2 = { id: 't2', name: '租户二' }
  const out = withCostListGroups([], [TENANT, t2], {
    [TENANT.id]: items,
    t2: [{ item_key: '基本电费|基本电费', item_name: '基本电费', field_enabled: true, sort: 1 }],
  })
  const g1 = out.filter((f) => f.group === tenantGroupKey(TENANT.id))
  const g2 = out.filter((f) => f.group === tenantGroupKey(t2.id))
  assert.ok(g1.some((f) => f.name === 'item_基本电费|基本电费'))
  assert.ok(g2.some((f) => f.name === 'item_基本电费|基本电费'))
  assert.ok(g2.every((f) => f.groupLabel === '租户二'))
})

test('normalizeCostListDefaults：只归一默认分组，租户分组字段原样保留', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  saved[0].label = '账单周期（自定义）'
  saved[1].width = 200
  const legacy = { name: 'src_x', label: '账单周期', enabled: true, isDefault: true, dataType: 'text', width: 0, group: 'cols', groupLabel: '费用清单' }
  const out = normalizeCostListDefaults([legacy, ...saved])
  // 默认分组被归一
  assert.equal(out[0].group, DEFAULT_GROUP_KEY)
  assert.equal(out[0].groupLabel, DEFAULT_GROUP_LABEL)
  // 租户分组字段一个不少、顺序与改名/宽度都不动
  const tenantOut = out.filter((f) => f.group === tenantGroupKey(TENANT.id))
  assert.equal(tenantOut.length, saved.length)
  assert.equal(tenantOut[0].label, '账单周期（自定义）')
  assert.equal(tenantOut[1].width, 200)
  assert.deepEqual(tenantOut.map((f) => f.name), saved.map((f) => f.name))
})

test('normalizeCostListDefaults：空输入不炸', () => {
  assert.deepEqual(normalizeCostListDefaults(undefined), [])
  assert.deepEqual(normalizeCostListDefaults([]), [])
})

test('withCostListGroups：租户列表为空时只留默认分组（不报错）', () => {
  const out = withCostListGroups([{ name: 'a', enabled: true, isDefault: true, dataType: 'text', width: 0, group: 'cols' }], [], {})
  assert.deepEqual(out.map((f) => f.name), ['a'])
})

test('tenantGroupNeedsSync：集合差异才提示，顺序/文案差异不提示', () => {
  const saved = buildTenantGroupFields(TENANT, items)
  assert.equal(tenantGroupNeedsSync(saved, items), false)
  // 只改顺序
  assert.equal(tenantGroupNeedsSync([...saved].reverse(), items), false)
  // 新开启一项
  assert.equal(
    tenantGroupNeedsSync(saved, [...items, { item_key: 'x|y', item_name: 'y', field_enabled: true, sort: 9 }]),
    true,
  )
  // 关掉一项
  assert.equal(tenantGroupNeedsSync(saved, items.map((it) => ({ ...it, field_enabled: false }))), true)
})

// ---- 第 15 轮：大类汇总列（市场化购电费 / 系统运行费用 / 政府性基金及附加）----

const itemSub = (cat: string, name: string) => ({
  name: `item_${cat}|${name}`,
  label: name,
  enabled: true,
  isDefault: true,
  dataType: 'number',
  width: 0,
  group: 'tenant:t1',
})

test('itemCategoryOfName：从子项字段名反解账单大类', () => {
  assert.equal(itemCategoryOfName('item_(4)系统运行费|煤电容量电费'), '(4)系统运行费')
  assert.equal(itemCategoryOfName('item_total'), '')
  assert.equal(itemCategoryOfName('ind_fee'), '')
})

test('feeGroupSpecOfCategory：只认工商业基金，居民基金不归入', () => {
  assert.equal(feeGroupSpecOfCategory('(1)市场化购电费')?.name, 'grp_market')
  assert.equal(feeGroupSpecOfCategory('(4)系统运行费')?.name, 'grp_system')
  assert.equal(feeGroupSpecOfCategory('(6)政府性基金及附加-工商业')?.name, 'grp_gov')
  // 居民同名基金是另一套账，绝不能折叠进工商业汇总列
  assert.equal(feeGroupSpecOfCategory('(6)政府性基金及附加-居民'), null)
  assert.equal(feeGroupSpecOfCategory('基本电费'), null)
})

test('collapseFeeGroups：三大类成员被折叠，顺序按第一个成员的位置', () => {
  const subs = [
    { name: 'month', label: '账单周期' },
    itemSub('(1)市场化购电费', '零售交易电费'),
    itemSub('(1)市场化购电费', '零售损益分摊电费'),
    { name: 'item_(2)上网环节线损费用|上网环节线损费用', label: '上网环节线损费用' },
    itemSub('(4)系统运行费', '煤电容量电费'),
    itemSub('(4)系统运行费', '燃气机组容量电费'),
    itemSub('(6)政府性基金及附加-工商业', '农网还贷'),
    { name: 'item_total', label: '合计应付' },
  ]
  const rows = collapseFeeGroups(subs)
  assert.deepEqual(rows.map((r) => r.name), [
    'month', 'grp_market', 'item_(2)上网环节线损费用|上网环节线损费用', 'grp_system', 'grp_gov', 'item_total',
  ])
  const market = rows.find((r) => r.name === 'grp_market') as any
  assert.equal(market.kind, 'group')
  assert.deepEqual(market.members.map((m: any) => m.name), [
    'item_(1)市场化购电费|零售交易电费', 'item_(1)市场化购电费|零售损益分摊电费',
  ])
  const gov = rows.find((r) => r.name === 'grp_gov') as any
  assert.equal(gov.spec.category, '(6)政府性基金及附加-工商业')
})

test('collapseFeeGroups：已有大类字段时用它的名称/位置，新成员自动并入末尾', () => {
  const subs = [
    { name: 'month', label: '账单周期' },
    {
      name: 'grp_system', label: '系统运行（自定义）', enabled: true, isDefault: true, dataType: 'number', width: 220,
      source: { category: '市电账单', field: 'grp_system', mode: 'fee_group', group: '(4)系统运行费', members: ['item_(4)系统运行费|煤电容量电费'] },
    },
    itemSub('(4)系统运行费', '煤电容量电费'),
    itemSub('(4)系统运行费', '新增的电费项'),
  ]
  const rows = collapseFeeGroups(subs)
  assert.deepEqual(rows.map((r) => r.name), ['month', 'grp_system'])
  const g = rows[1] as any
  assert.equal(g.label, '系统运行（自定义）')
  assert.deepEqual(g.members.map((m: any) => m.name), [
    'item_(4)系统运行费|煤电容量电费', 'item_(4)系统运行费|新增的电费项',
  ])
})

test('mergeFeeGroupFields：大类插在第一个成员之前，成员留在原位', () => {
  const fields: any[] = [
    { name: 'month', label: '账单周期' },
    itemSub('(4)系统运行费', '煤电容量电费'),
    itemSub('(4)系统运行费', '燃气机组容量电费'),
    { name: 'item_total', label: '合计应付' },
  ]
  const out = mergeFeeGroupFields(fields, 'tenant:t1', '持睿汽车')
  assert.deepEqual(out.map((f) => f.name), [
    'month', 'grp_system', 'item_(4)系统运行费|煤电容量电费', 'item_(4)系统运行费|燃气机组容量电费', 'item_total',
  ])
  const g = out[1]
  assert.equal(g.source?.mode, 'fee_group')
  assert.deepEqual(g.source?.members, ['item_(4)系统运行费|煤电容量电费', 'item_(4)系统运行费|燃气机组容量电费'])
})

test('mergeFeeGroupFields：保留用户改过的大类名称与宽度，只刷新 members', () => {
  const fields: any[] = [
    {
      name: 'grp_gov', label: '基金（改名）', enabled: false, isDefault: false, dataType: 'number', width: 180,
      source: { category: '市电账单', field: 'grp_gov', mode: 'fee_group', group: '(6)政府性基金及附加-工商业', members: [] },
    },
    itemSub('(6)政府性基金及附加-工商业', '农网还贷'),
    itemSub('(6)政府性基金及附加-工商业', '可再生能源附加'),
  ]
  const out = mergeFeeGroupFields(fields, 'tenant:t1', '持睿汽车')
  const g: any = out.find((f) => f.name === 'grp_gov')
  assert.equal(g.label, '基金（改名）')
  assert.equal(g.width, 180)
  assert.equal(g.enabled, false)
  assert.deepEqual(g.source.members, [
    'item_(6)政府性基金及附加-工商业|农网还贷',
    'item_(6)政府性基金及附加-工商业|可再生能源附加',
  ])
})

test('mergeFeeGroupFields：成员全被关掉时撤掉大类列（不留恒 0 空壳）', () => {
  const fields: any[] = [
    { name: 'grp_market', label: '市场化购电费', enabled: true, isDefault: true, dataType: 'number', width: 0,
      source: { category: '市电账单', field: 'grp_market', mode: 'fee_group', group: '(1)市场化购电费', members: [] } },
    { name: 'month', label: '账单周期' },
  ]
  const out = mergeFeeGroupFields(fields, 'tenant:t1', '持睿汽车')
  assert.equal(out.some((f) => f.name === 'grp_market'), false)
})

test('buildTenantGroupFields：租户分组默认带出三个大类汇总列', () => {
  const items = [
    { item_key: '(1)市场化购电费|零售交易电费', item_name: '零售交易电费', field_enabled: true },
    { item_key: '(4)系统运行费|煤电容量电费', item_name: '煤电容量电费', field_enabled: true },
    { item_key: '(6)政府性基金及附加-工商业|农网还贷', item_name: '农网还贷', field_enabled: true },
    { item_key: '(2)上网环节线损费用|上网环节线损费用', item_name: '上网环节线损费用', field_enabled: true },
  ]
  const fields = buildTenantGroupFields({ id: 't1', name: '持睿汽车' }, items)
  const names = fields.map((f) => f.name)
  for (const g of TENANT_FEE_GROUPS) assert.ok(names.includes(g.name), `缺少大类 ${g.name}`)
  // 上网线损不属于三大类，仍然独立成列
  assert.ok(names.includes('item_(2)上网环节线损费用|上网环节线损费用'))
  // 合计应付仍在最后
  assert.equal(names[names.length - 1], TENANT_TOTAL_FIELD)
})
