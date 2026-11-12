/**
 * 花名册 6 大分组契约 + 六同步回归（第 52 轮）。
 *
 * 第 52 轮：11 组 → 6 组；家庭情况 / 工作经历情况 / 个税申报信息 彻底删除；
 * 离职信息 / 合同签订情况 剥离为独立模块；在职状态 / 人员状态 删除。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  ROSTER_FIELD_GROUPS, ROSTER_STATUS_OPTIONS,
  allRosterFields, defaultVisibleFields, lockedRosterFields,
  visibleFieldsFor, drawerGroupsFor, groupLabelOf, groupCheckState,
  defaultVisibleKeysFromConfig, enabledFieldKeysFromConfig,
} from './hrRosterGroups'
import {
  HR_ROSTER_GROUPS, HR_ROSTER_FIELDS, HR_ROSTER_DEFAULT_HEADERS,
  HR_REMOVED_GROUPS, HR_REMOVED_FIELDS,
  HR_RESIGN_FIELDS, HR_CONTRACT_FIELDS,
} from './hrFieldSchema'

const GROUP_KEYS = ['在职信息', '基本信息', '联系信息', '紧急联系人情况', '教育情况', '工资社保信息']

test('第52轮：恰好 6 大分组，顺序严格为 在职信息→基本信息→联系信息→紧急联系人情况→教育情况→工资社保信息', () => {
  assert.equal(ROSTER_FIELD_GROUPS.length, 6)
  assert.deepEqual(ROSTER_FIELD_GROUPS.map((g) => g.key), GROUP_KEYS)
  assert.deepEqual(HR_ROSTER_GROUPS.map((g) => g.label), GROUP_KEYS)
})

test('第52轮：3 个废弃分组与在职状态字段彻底删除', () => {
  const all = allRosterFields()
  for (const g of HR_REMOVED_GROUPS) {
    assert.ok(!GROUP_KEYS.includes(g), `不应再有分组 ${g}`)
  }
  for (const f of [...HR_REMOVED_FIELDS,
    '婚姻状况', '配偶姓名', '上家工作单位', '是否烈属', '个人股本(投资)额',
    '现合同编号', '合同状态', '员工状态', '健康证有效期', '是否已育']) {
    assert.ok(!all.includes(f), `废弃字段 ${f} 仍在花名册字段集里`)
  }
  // 离职 / 合同字段已剥离到独立模块，花名册不得包含
  for (const f of ['离职日期', '离职类型', '离职证明', '合同类型', '合同结束日期']) {
    assert.ok(!all.includes(f), `已剥离字段 ${f} 仍在花名册`)
  }
})

test('第52轮：6 组字段数与顺序（10 / 13 / 9 / 4 / 4 / 8 = 48）', () => {
  assert.deepEqual(ROSTER_FIELD_GROUPS.map((g) => g.fields.length), [10, 13, 9, 4, 4, 8])
  assert.equal(HR_ROSTER_FIELDS.length, 48)
  const all = allRosterFields()
  assert.equal(all.length, 48)
  assert.equal(new Set(all).size, all.length, '字段必须去重')
  assert.equal(all[0], '工号')
  assert.equal(all[1], '姓名')
  // group 属性保留（供编辑抽屉按 6 组分块）
  for (const g of HR_ROSTER_GROUPS) {
    assert.ok(g.fields.every((d) => d.group === g.key), `${g.key} 内字段 group 必须一致`)
  }
})

test('第52轮：姓名必选锁定，且是唯一 locked 字段', () => {
  assert.deepEqual(lockedRosterFields(), ['姓名'])
  const name = ROSTER_FIELD_GROUPS[0].fields.find((f) => f.name === '姓名')
  assert.equal(name?.checked, true)
  assert.equal(name?.locked, true)
})

test('第52轮：默认表头 = 契约里 defaultHeader 的字段，工号打头', () => {
  const vis = defaultVisibleFields()
  assert.deepEqual(vis, [...HR_ROSTER_DEFAULT_HEADERS])
  assert.equal(vis[0], '工号')
  assert.equal(vis[1], '姓名')
  for (const k of ['部门', '岗位', '入职日期', '司龄', '证件号码', '性别', '出生日期',
    '年龄', '是否已婚', '籍贯', '户口性质', '手机号码', '最高学历']) {
    assert.ok(vis.includes(k), `默认表头缺少 ${k}`)
  }
  for (const k of ['合同公司', '招聘渠道', '试用期', '转正日期', '证件类型', '民族',
    '政治面貌', '服装尺寸', '工资卡卡号', '银行名称', '毕业院校']) {
    assert.ok(!vis.includes(k), `${k} 默认表头应为关`)
  }
})

test('在职状态筛选选项保留 3 态，但花名册列表固定只查在职', () => {
  assert.deepEqual(ROSTER_STATUS_OPTIONS.map((o) => o.value), ['全部', '在职', '离职'])
})

test('visibleFieldsFor：三个视图都只产出精简后的 48 字段（无离职分组）', () => {
  for (const status of ['在职', '离职', '全部'] as const) {
    const vis = visibleFieldsFor(status)
    assert.ok(!vis.includes('离职日期'))
    assert.ok(!vis.includes('现合同编号'))
    assert.ok(vis.includes('工号') && vis.includes('姓名'))
  }
  // 姓名锁定，即使 overrides 为空集合也要出现
  assert.ok(visibleFieldsFor('在职', new Set()).includes('姓名'))
})

test('drawerGroupsFor：严格按启用字段切分 6 组，空组自动丢弃', () => {
  const groups = drawerGroupsFor('在职')
  assert.deepEqual(groups.map((g) => g.key), GROUP_KEYS)
  assert.ok(groups.every((g) => g.fields.length > 0), '不应出现空分组')
  // 六同步：只启用「工号 / 姓名」时，其余 5 组都必须从抽屉消失
  const partial = drawerGroupsFor('在职', new Set(['工号', '姓名']))
  assert.deepEqual(partial.map((g) => g.key), ['在职信息'])
  assert.deepEqual(partial[0].fields, ['工号', '姓名'])
})

test('groupCheckState：全选 / 半选 / 未选', () => {
  const active = ROSTER_FIELD_GROUPS[0]
  const allSet = new Set(active.fields.map((f) => f.name))
  assert.equal(groupCheckState(active, allSet), 'all')
  assert.equal(groupCheckState(active, new Set()), 'partial') // 姓名 locked 恒勾选
  const education = ROSTER_FIELD_GROUPS[4]
  assert.equal(groupCheckState(education, new Set()), 'none')
})

test('groupLabelOf：字段归属 6 大分组', () => {
  assert.equal(groupLabelOf('工号'), '在职信息')
  assert.equal(groupLabelOf('证件号码'), '基本信息')
  assert.equal(groupLabelOf('手机号码'), '联系信息')
  assert.equal(groupLabelOf('紧急联系人姓名'), '紧急联系人情况')
  assert.equal(groupLabelOf('最高学历'), '教育情况')
  assert.equal(groupLabelOf('工资卡卡号'), '工资社保信息')
  assert.equal(groupLabelOf('离职日期'), '')
})

// ---------------------------------------------------------------------------
// 离职管理 / 劳动合同：剥离后的独立字段契约
// ---------------------------------------------------------------------------

test('第52轮：离职管理 = 5 花名册引用 + 9 离职字段，引用带来源徽标', () => {
  assert.equal(HR_RESIGN_FIELDS.length, 14)
  const refs = HR_RESIGN_FIELDS.filter((d) => d.source)
  assert.equal(refs.length, 5)
  assert.ok(refs.every((d) => d.source?.category === '花名册'))
  assert.deepEqual(refs.map((d) => d.name), ['工号', '姓名', '部门', '岗位', '入职日期'])
  for (const k of ['离职申请日期', '离职日期', '离职类型', '离职原因', '离职原因说明',
    '离职去向', '是否试用期内离职', '薪资结算日', '离职证明']) {
    assert.ok(HR_RESIGN_FIELDS.some((d) => d.name === k), `离职管理缺少 ${k}`)
  }
  assert.equal(HR_RESIGN_FIELDS.find((d) => d.name === '离职证明')?.type, 'attachment')
})

test('第52轮：劳动合同 = 乙方 6 + 甲方 3 + 合同 8', () => {
  assert.equal(HR_CONTRACT_FIELDS.length, 17)
  const partyB = HR_CONTRACT_FIELDS.filter((d) => d.source?.category === '花名册')
  const partyA = HR_CONTRACT_FIELDS.filter((d) => d.source?.category === '组织机构')
  assert.equal(partyB.length, 6)
  assert.equal(partyA.length, 3)
  assert.deepEqual(partyA.map((d) => d.name), ['统一社会信用代码', '住所', '法定代表人'])
  const status = HR_CONTRACT_FIELDS.find((d) => d.name === '合同状态')
  assert.equal(status?.type, 'system')
  assert.equal(status?.builtin, true)
  for (const k of ['合同编号', '合同类型', '合同开始日期', '合同结束日期',
    '合同期限', '签约公司', '签订日期']) {
    assert.ok(HR_CONTRACT_FIELDS.some((d) => d.name === k), `劳动合同缺少 ${k}`)
  }
})

// ---------------------------------------------------------------------------
// 六同步契约（后端 subs → 默认表头 / 可选全集）
// ---------------------------------------------------------------------------

test('defaultVisibleKeysFromConfig 只取 enabled && is_default', () => {
  const config = [{
    builtin_key: 'hr-roster',
    subs: [
      { name: '工号', enabled: true, is_default: true, group: '在职信息' },
      { name: '姓名', enabled: true, is_default: true, group: '在职信息' },
      { name: '招聘渠道', enabled: true, is_default: false, group: '在职信息' },
      { name: '户籍地址', enabled: false, is_default: true, group: '基本信息' },
    ],
  }]
  assert.deepEqual(defaultVisibleKeysFromConfig(config), ['工号', '姓名'])
  assert.equal(defaultVisibleKeysFromConfig(config)[0], '工号')
})

test('enabledFieldKeysFromConfig 取全部启用字段（含默认表头关闭的）', () => {
  const config = [{
    builtin_key: 'hr-roster',
    subs: [
      { name: '工号', enabled: true, is_default: true },
      { name: '招聘渠道', enabled: true, is_default: false },
      { name: '户籍地址', enabled: false, is_default: true },
    ],
  }]
  assert.deepEqual(enabledFieldKeysFromConfig(config), ['工号', '招聘渠道'])
})

test('配置缺失时回落到静态默认，绝不给空表头', () => {
  for (const bad of [null, undefined, [], [{}], [{ subs: null }]]) {
    const keys = defaultVisibleKeysFromConfig(bad as any)
    assert.ok(keys.length >= 10, `配置 ${JSON.stringify(bad)} 时应有兜底列`)
    assert.ok(keys.includes('工号'))
  }
})
