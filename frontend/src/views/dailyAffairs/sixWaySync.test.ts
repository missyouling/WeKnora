import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  buildDetailFieldDefs,
  normalizeArrayValue,
  arrayToText,
} from './businessDetailForm.ts'
import {
  buildColumnsFromSubs,
  buildRuleFieldsFromSubs,
  buildDrawerFieldsFromSubs,
  columnLabelOf,
} from './sixWaySync.ts'

/**
 * 六同步（six-way sync）回归。
 *
 * 背景：2026-10-21 真机实测暴露四处断裂——
 *   ① 编辑抽屉末尾硬编码塞了未在字段定义中出现的 fileName；
 *   ② 字段改名（摘要 → 摘要2）后表头/筛选器/抽屉仍是旧名；
 *   ③ 字段类型改 array 后抽屉仍是文本框；
 *   ④ 删除字段后抽屉/列表仍有残留。
 *
 * 根因是「同一份 subs 被六处各自解释」：列表列取静态 DEFAULT_*_COLUMNS 的 label
 * 覆盖 subs.label、抽屉在 subs 缺失时回落静态列、规则面板让 extract_config.type
 * 压过 subs.data_type、后端 seed 里硬编码 fileName 并在重启时把删除的字段补回来。
 *
 * 本文件锁定一条铁律：**fleet_categories.subs 是唯一权威源**，六处的
 * 字段集合 / 名称 / 顺序 / dataType 必须 100% 由它派生。
 */

// ---- 测试用分类：模拟用户在「字段定义」里改过名的合同分类 ----

/** 基线：改名前的原始 subs（后端 seed 形态）。 */
function baseSubs() {
  return [
    { name: 'contract_no', label: '', enabled: true, is_default: true, data_type: 'text', width: 0 },
    { name: 'contract_name', label: '', enabled: true, is_default: true, data_type: 'text', width: 0 },
    { name: 'party_a', label: '', enabled: true, is_default: true, data_type: 'text', width: 0 },
    { name: 'summary', label: '', enabled: true, is_default: false, data_type: 'text', width: 0 },
  ]
}

/** 场景 A：把「摘要」改名为「摘要2」，并把类型改为数组。 */
function renamedArraySubs() {
  return baseSubs().map((s) =>
    s.name === 'summary' ? { ...s, label: '摘要2', data_type: 'array' } : s,
  )
}

/** 场景 B：在此基础上删掉「甲方」、禁用「合同名称」。 */
function trimmedSubs() {
  return renamedArraySubs()
    .filter((s) => s.name !== 'party_a')
    .map((s) => (s.name === 'contract_name' ? { ...s, enabled: false } : s))
}

const LABELS: Record<string, string> = {
  contract_no: '合同编号',
  contract_name: '合同名称',
  party_a: '甲方',
  summary: '摘要',
}

test('同步点①：字段定义自身——用户自定义 label 必须盖过静态字典', () => {
  const cols = buildColumnsFromSubs(renamedArraySubs(), { labelOf: (n) => LABELS[n] || n })
  const summary = cols.find((c) => c.key === 'summary')
  assert.ok(summary, '摘要2 必须出现在列定义中')
  assert.equal(summary.label, '摘要2', '用户改名后不得被静态字典「摘要」盖回')
  // 未改名的字段仍走字典
  assert.equal(cols.find((c) => c.key === 'contract_no')?.label, '合同编号')
})

test('同步点①：改类型同步到列定义（列表按 array 格式化）', () => {
  const cols = buildColumnsFromSubs(renamedArraySubs(), { labelOf: (n) => LABELS[n] || n })
  assert.equal(cols.find((c) => c.key === 'summary')?.dataType, 'array')
  assert.equal(cols.find((c) => c.key === 'contract_no')?.dataType, 'text')
})

test('同步点①：删除与禁用字段必须消失，且不得出现 fileName 幽灵字段', () => {
  const cols = buildColumnsFromSubs(trimmedSubs(), { labelOf: (n) => LABELS[n] || n })
  const keys = cols.map((c) => c.key)
  assert.deepEqual(keys, ['contract_no', 'summary'], `列集合必须严格等于启用字段，实际 ${keys.join(',')}`)
  assert.ok(!keys.includes('party_a'), '删除的字段不得残留在列表表头')
  assert.ok(!keys.includes('contract_name'), '禁用的字段不得留在列表表头')
  // 幽灵字段：subs 里没有 fileName 就绝不能出现在任何一处
  assert.ok(!keys.includes('fileName'), 'fileName 未在 subs 中，不得硬编码追加')
})

test('同步点③：字段筛选器与表头同源（同集合、同名称、同顺序）', () => {
  const cols = buildColumnsFromSubs(trimmedSubs(), { labelOf: (n) => LABELS[n] || n })
  const filterColumns = cols.map((c) => ({ key: c.key, label: c.label }))
  assert.deepEqual(
    filterColumns.map((c) => c.label),
    cols.map((c) => c.label),
    '筛选器 label 必须与表头逐项一致',
  )
  assert.equal(filterColumns.find((c) => c.key === 'summary')?.label, '摘要2')
})

test('同步点⑤：抽屉字段——改名为摘要2 且类型变 array', () => {
  const fields = buildDrawerFieldsFromSubs(renamedArraySubs(), {
    fieldKeyOf: (n) => n,
    labelOf: (n) => LABELS[n] || n,
    isRemarkPart: () => false,
  })
  const summary = fields.find((f) => f.name === 'summary')
  assert.ok(summary)
  assert.equal(summary.label, '摘要2', '抽屉标签必须是用户改名后的值')
  assert.equal(summary.dataType, 'array', '抽屉必须按 subs 的 data_type 切到数组控件')
})

test('同步点⑤：抽屉字段 = 启用字段集合，无 fileName 幽灵字段', () => {
  const fields = buildDrawerFieldsFromSubs(trimmedSubs(), {
    fieldKeyOf: (n) => n,
    labelOf: (n) => LABELS[n] || n,
    isRemarkPart: () => false,
  })
  const names = fields.map((f) => f.name)
  assert.deepEqual(names, ['contract_no', 'summary'])
  assert.ok(!names.includes('fileName'), '抽屉不得追加 subs 之外的 fileName 字段')
})

test('同步点②：提取规则字段与字段定义逐项一致（名称/顺序/类型/启用）', () => {
  const fields = buildRuleFieldsFromSubs(renamedArraySubs(), {
    labelOf: (n) => LABELS[n] || n,
    savedByKey: new Map([
      // 旧配置：字段仍叫「摘要」、类型 text、desc/rule 有用户配置
      ['summary', { desc: '合同要点', rule: '提取正文摘要', type: 'text', enabled: false, regex: '\\d+' }],
    ]),
  })
  const summary = fields.find((f) => f.name === 'summary')
  assert.ok(summary)
  assert.equal(summary.label, '摘要2', '规则面板字段名必须跟随 subs 改名')
  assert.equal(summary.type, 'array', 'subs.data_type 优先于 extract_config 里的旧 type')
  assert.equal(summary.enabled, true, '启用状态以 subs 为准')
  // 改名/改类型不得丢失用户已配的口径
  assert.equal(summary.desc, '合同要点', '描述不得因改名而丢失')
  assert.equal(summary.rule, '提取正文摘要', '规则不得因改名而丢失')
  assert.equal(summary.regex, '\\d+', '正则兜底不得因改名而丢失')
})

test('同步点②：字段定义里删掉的字段同步从提取规则移除', () => {
  const fields = buildRuleFieldsFromSubs(trimmedSubs(), {
    labelOf: (n) => LABELS[n] || n,
    savedByKey: new Map([
      ['party_a', { desc: '甲方名称', rule: '', type: 'text', enabled: true }],
      ['summary', { desc: '合同要点', rule: '', type: 'text', enabled: true }],
    ]),
  })
  assert.deepEqual(fields.map((f) => f.name), ['contract_no', 'summary'])
  assert.ok(!fields.some((f) => f.name === 'party_a'), '已删字段不得残留在提取规则')
})

test('五处一致性总断言：集合/名称/顺序/dataType 100% 一致', () => {
  const subs = renamedArraySubs()
  const opts = { labelOf: (n: string) => LABELS[n] || n }

  const cols = buildColumnsFromSubs(subs, opts)
  const filter = cols.map((c) => ({ key: c.key, label: c.label }))
  const drawer = buildDrawerFieldsFromSubs(subs, { ...opts, fieldKeyOf: (n) => n, isRemarkPart: () => false })
  const rules = buildRuleFieldsFromSubs(subs, opts)

  const shape = (list: { key?: string; name?: string; label: string; dataType?: string }[]) =>
    list.map((x) => `${x.key ?? x.name}|${x.label}|${x.dataType}`)

  assert.deepEqual(shape(drawer as any), shape(cols), '抽屉与表头必须逐项同形')
  assert.deepEqual(shape(rules as any), shape(cols), '提取规则与表头必须逐项同形')
  assert.deepEqual(filter.map((c) => `${c.key}|${c.label}`), shape(cols).map((s) => s.split('|').slice(0, 2).join('|')))
})

// ---- 数组类型的编辑控件契约 ----

test('数组字段：字符串存量值按换行/分号拆成 string[]，保存后可回显', () => {
  assert.deepEqual(normalizeArrayValue('甲\n乙；丙;丁'), ['甲', '乙', '丙', '丁'])
  assert.deepEqual(normalizeArrayValue(['甲', '乙']), ['甲', '乙'])
  assert.deepEqual(normalizeArrayValue(''), [])
  assert.deepEqual(normalizeArrayValue(null), [])
  // 数字/对象等非字符串也要能安全转成标签文本
  assert.deepEqual(normalizeArrayValue([1, 2]), ['1', '2'])
})

test('数组字段：数组回写成编辑态文本（每行一条）', () => {
  assert.equal(arrayToText(['甲', '乙']), '甲\n乙')
  assert.equal(arrayToText('甲'), '甲')
  assert.equal(arrayToText(null), '')
})

test('抽屉：array 类型渲染数组控件（t-tag-input），不再是纯 textarea', () => {
  const src = readFileSync(new URL('./BusinessSplitDetailDrawer.vue', import.meta.url), 'utf8')
  assert.match(src, /<t-tag-input/, 'dataType=array 必须渲染 t-tag-input 数组控件')
  // 数组控件必须做字符串存量兼容
  assert.match(src, /normalizeArrayValue/)
})

test('抽屉：四种 dataType 都有对应控件（text/number/date/array）', () => {
  const src = readFileSync(new URL('./BusinessSplitDetailDrawer.vue', import.meta.url), 'utf8')
  assert.match(src, /f\.dataType === 'date'/)
  assert.match(src, /f\.dataType === 'number'/)
  assert.match(src, /f\.dataType === 'array'/)
  assert.match(src, /<t-input v-else/)
})

// ---- 源码契约：六处不得再用静态表覆盖 subs ----

test('列表页：reloadColumns 的 label 以 subs.label 优先（三个模块同口径）', () => {
  for (const name of ['ContractManagement.vue', 'RegulationManagement.vue', 'AwardPunishManagement.vue']) {
    const src = readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
    assert.match(
      src,
      /buildColumnsFromSubs/,
      `${name} 必须统一走 buildColumnsFromSubs（禁止再写 builtin?.label || s.name）`,
    )
    assert.doesNotMatch(
      src,
      /label:\s*builtin\?\.label \|\| s\.name/,
      `${name} 仍在用静态字典 label 盖住用户改名`,
    )
  }
})

test('抽屉字段：三模块无 subs 时不回退静态列（唯一数据源铁律）', () => {
  for (const name of ['ContractManagement.vue', 'RegulationManagement.vue', 'AwardPunishManagement.vue']) {
    const src = readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')
    const block = src.slice(src.indexOf('buildDetailFieldDefs'), src.indexOf('buildDetailFieldDefs') + 1400)
    assert.doesNotMatch(
      block,
      /DEFAULT_(CONTRACT|REGULATION|NOTICE)_COLUMNS\s*\n?\s*\.filter/,
      `${name} 的抽屉字段不得回退静态列（会引入 subs 之外的幽灵字段）`,
    )
  }
})

test('字段定义面板：fileName 不得由前端硬编码追加', () => {
  const src = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')
  assert.doesNotMatch(src, /fileName/, 'FleetCertPanel 不得出现硬编码 fileName')
})

// ===========================================================================
// 费用清单：默认分组 / 租户分组的字段集合一致性（第 22 轮 P0-1）
// ===========================================================================
//
// 第 19 轮真机事故：租户分组混进 16 个业主视角的列，表头 18 列、字段定义 42 行、
// 浮窗 40 个 key —— 当时**单测全绿，六端已经不一致**，只能靠人眼看截图。
//
// 费用清单的字段集合比发票/合同多一层折叠（市电三大类），所以「同量」不能只比
// subs 条数，必须比 `collapseFeeGroups` 折叠后的行序列 —— 那是表头/筛选器/浮窗/
// 打印目录共同经过的唯一一步。

import {
  buildTenantGroupFields,
  collapseFeeGroups,
  DEFAULT_GROUP_KEY,
  isTenantGroupMember,
  subsOfGroup,
  TENANT_DERIVED_FIELDS,
  TENANT_DERIVED_NAMES,
  TENANT_ELEC_TOTAL_FIELD,
  TENANT_FEE_GROUPS,
  TENANT_TOTAL_FIELDS,
  TENANT_WATER_TOTAL_FIELD,
  tenantGroupKey,
} from './billingFieldGroups'

/** 默认分组（业主视角）24 列 —— 与库 `fleet_categories.subs(group='default')` 一致。 */
const DEFAULT_SUBS = ([
  ['month', '账单周期', 'text', true],
  ['unit', '使用单位', 'text', true],
  ['total_kwh', '总电量', 'number', true],
  ['total_fee', '总电费', 'number', true],
  ['ind_kwh', '用电量（工业）', 'number', true],
  ['ind_fee', '电费（工业）', 'number', true],
  ['ind_price', '均价（工业）', 'number', true],
  ['res_kwh', '电量（定比）', 'number', true],
  ['res_fee', '电费（定比）', 'number', true],
  ['dorm_kwh', '电量（宿舍）', 'number', true],
  ['dorm_fee', '电费（宿舍）', 'number', true],
  ['water_ind_usage', '用水量（工业）', 'number', true],
  ['water_ind_fee', '水费（工业）', 'number', true],
  ['water_dorm_usage', '用水量（宿舍）', 'number', true],
  ['water_dorm_fee', '水费（宿舍）', 'number', true],
  ['water_fire_usage', '用水量（消防）', 'number', true],
  ['water_fire_fee', '水费（消防）', 'number', true],
  ['gas_usage', '用气量', 'number', true],
  ['gas_fee', '气费', 'number', true],
  ['solar_gen', '光伏发电量', 'number', true],
  ['solar_grid', '上网电量', 'number', true],
  ['solar_amount', '结算金额', 'number', true],
  ['ratio', '分摊比例', 'number', true],
  ['remark', '备注', 'text', false],
] as Array<[string, string, string, boolean]>).map(([name, label, dataType, isDefault]) => ({
  name,
  label,
  dataType,
  isDefault,
  enabled: true,
  width: 0,
  group: DEFAULT_GROUP_KEY,
  groupLabel: '默认分组',
}))

/** 持睿汽车的分摊子项（7 条，覆盖 4 个账单大类）。 */
const TENANT_ITEMS = [
  ['(1)市场化购电费|零售交易电费', '(1)市场化购电费', '零售交易电费'],
  ['(1)市场化购电费|零售损益分摊电费', '(1)市场化购电费', '零售损益分摊电费'],
  ['(3)输配电量电费|零售输配电费', '(3)输配电量电费', '零售输配电费'],
  ['(4)系统运行费|煤电容量电费', '(4)系统运行费', '煤电容量电费'],
  ['(4)系统运行费|抽水蓄能容量电费', '(4)系统运行费', '抽水蓄能容量电费'],
  ['(6)政府性基金及附加-工商业|农网还贷', '(6)政府性基金及附加-工商业', '农网还贷'],
  ['基本电费|基本电费', '基本电费', '基本电费'],
].map(([item_key, category, item_name], i) => ({
  item_key,
  item_name,
  category,
  enabled: true,
  field_enabled: true,
  width: 0,
  sort: i + 1,
}))

const TENANT_LIKE = { id: 't-chengrui', name: '持睿汽车' }

/** 五端共同经过的那一步：subs → 表头行序列。 */
function rowsOf(subs: ReadonlyArray<Record<string, any>>) {
  return collapseFeeGroups(subs.filter((s) => s.enabled !== false))
}

test('费用清单·默认分组：24 列平铺，字段配置与表头同量同序', () => {
  const enabled = DEFAULT_SUBS.filter((s) => s.enabled !== false)
  const rows = rowsOf(DEFAULT_SUBS)
  // 默认分组不含市电分摊子项 → 不产生大类折叠
  assert.equal(rows.filter((r) => r.kind === 'group').length, 0, '默认分组不得出现大类折叠行')
  assert.deepEqual(
    rows.map((r) => r.name),
    enabled.map((s) => s.name),
    '默认分组：字段配置启用项与列表表头必须同名同序',
  )
})

test('费用清单·默认分组：零重名，且不得含租户专属列', () => {
  const names = rowsOf(DEFAULT_SUBS).map((r) => r.name)
  assert.equal(new Set(names).size, names.length, '默认分组字段名不得重复')
  for (const n of TENANT_TOTAL_FIELDS) {
    assert.ok(!names.includes(n), `默认分组不得含收口列 ${n}`)
  }
  assert.ok(!names.some((n) => n.startsWith('item_')), '默认分组不得含分摊子项列')
})

test('费用清单·默认分组：备注默认不出列（否则表头比字段配置多一列）', () => {
  const remark = DEFAULT_SUBS.find((s) => s.name === 'remark')!
  assert.equal(remark.isDefault, false, '「备注」默认表头必须关闭')
  assert.equal(remark.enabled, true, '「备注」字段本身必须启用')
})

test('费用清单·租户分组：大类折叠后，字段配置与表头严格同量', () => {
  const fields = buildTenantGroupFields(TENANT_LIKE, TENANT_ITEMS)
  const enabled = fields.filter((f) => f.enabled !== false)
  const rows = collapseFeeGroups(enabled)

  const groups = rows.filter((r) => r.kind === 'group') as any[]
  assert.equal(groups.length, TENANT_FEE_GROUPS.length, '大类折叠行数必须等于大类定义数')
  assert.deepEqual(
    groups.map((g) => g.name).sort(),
    TENANT_FEE_GROUPS.map((g) => g.name).sort(),
    '大类名字必须与 TENANT_FEE_GROUPS 一一对应',
  )

  // 被折叠的成员不得再出现在顶层（否则同一笔费用出两列）
  const topNames = rows.map((r) => r.name)
  for (const g of groups) {
    assert.ok(g.members.length > 0, `大类 ${g.name} 必须有成员`)
    for (const m of g.members) {
      assert.ok(!topNames.includes(m.name), `成员 ${m.name} 不得在顶层重复出列`)
    }
  }

  const collapsed = groups.reduce((n, g) => n + g.members.length, 0)
  assert.equal(
    rows.length,
    enabled.length - collapsed,
    `表头 ${rows.length} 行 ≠ 字段配置 ${enabled.length} 项 − 折叠 ${collapsed} 项`,
  )
})

test('费用清单·租户分组：派生字段齐全、收口列恒在最后、零重名', () => {
  const fields = buildTenantGroupFields(TENANT_LIKE, TENANT_ITEMS)
  assert.equal(fields[0].group, tenantGroupKey(TENANT_LIKE.id), '字段必须打上租户分组键')

  const names = collapseFeeGroups(fields.filter((f) => f.enabled !== false)).map((r) => r.name)
  for (const spec of TENANT_DERIVED_FIELDS) {
    assert.ok(names.includes(spec.name), `派生字段 ${spec.name}（${spec.label}）必须出列`)
  }
  assert.deepEqual(names.slice(-2), [TENANT_ELEC_TOTAL_FIELD, TENANT_WATER_TOTAL_FIELD])
  assert.equal(new Set(names).size, names.length, '租户分组表头不得重名')
})

test('费用清单·租户分组：不得混入业主视角（默认分组）的列', () => {
  const ownerOnly = DEFAULT_SUBS.map((s) => s.name).filter((n) => !TENANT_DERIVED_NAMES.includes(n))
  const tenantNames = collapseFeeGroups(
    buildTenantGroupFields(TENANT_LIKE, TENANT_ITEMS).filter((f) => f.enabled !== false),
  ).map((r) => r.name)

  const leaked = ownerOnly.filter((n) => tenantNames.includes(n))
  assert.deepEqual(leaked, [], `租户分组混入了业主视角列：${leaked.join(' / ')}`)

  for (const n of ['total_kwh', 'solar_gen', 'res_kwh', 'water_fire_fee', 'src_市电账单_overview_total_kwh']) {
    assert.equal(isTenantGroupMember(n), false, `${n} 不属于租户分组`)
  }
})

test('费用清单·租户分组：关掉「启用字段」的子项不出列，大类仍保留', () => {
  const items = TENANT_ITEMS.map((it) =>
    it.item_key === '(4)系统运行费|煤电容量电费' ? { ...it, field_enabled: false } : it,
  )
  const rows = collapseFeeGroups(
    buildTenantGroupFields(TENANT_LIKE, items).filter((f) => f.enabled !== false),
  )
  const names = rows.map((r) => r.name)
  assert.ok(!names.includes('item_(4)系统运行费|煤电容量电费'), '已关闭的子项不得出列')

  const system = rows.find((r) => r.name === 'grp_system') as any
  assert.ok(system, '系统运行费用大类必须仍出列')
  assert.deepEqual(
    system.members.map((m: any) => m.name),
    ['item_(4)系统运行费|抽水蓄能容量电费'],
    '大类成员必须只剩仍启用的那个（字段名带 item_ 前缀）',
  )
})

test('费用清单·五端同源：都从 collapseFeeGroups(subsOfGroup(...)) 派生', () => {
  const src = readFileSync(new URL('./TenantBilling.vue', import.meta.url), 'utf8')

  // ① 字段定义启用项：只取当前分组那一份 subs
  assert.match(src, /const subs = subsOfGroup\(allSubs, groupKey\)/, '字段定义必须按分组取 subs')
  // ② [☰ 字段] 筛选器：与表头同一个列集（组件内部按 visibleKeys 过滤）
  assert.match(src, /:columns="activeColDefs"/, '[☰ 字段] 筛选器必须与表头同源')
  // ③ 列表表头：colKey 逐条来自 visibleColDefs
  assert.match(
    src,
    /for \(const c of visibleColDefs\.value\)[\s\S]{0,240}?colKey: c\.key/,
    '表头 colKey 必须逐条来自 visibleColDefs',
  )
  // ④ 行浮窗 keys：与列 key 同一套 tenantItemColKey 规则
  assert.match(src, /itemTips\[tenantItemColKey\(it\)\]/, '浮窗 key 必须与列 key 同规则')
  // ⑤ 打印目录列：映射 visibleColDefs，不得另写表头数组
  assert.match(src, /visibleColDefs\.value\.map\(col => \(\{/, '打印目录必须映射 visibleColDefs')

  // 大类折叠是五端同量的前提
  assert.match(
    src,
    /for \(const row of collapseFeeGroups\(effective\)\)/,
    'activeColDefs 必须在 collapseFeeGroups 的结果上遍历',
  )
})

test('费用清单·字段定义页与列表页共用 subsOfGroup / collapseFeeGroups', () => {
  const panel = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')
  assert.match(panel, /collapseFeeGroups/, '字段定义页必须用同一套大类折叠')
  assert.match(panel, /normalizeCostListGroup/, '字段定义页必须按分组键取 subs（tenant:<id> 归一）')
})
