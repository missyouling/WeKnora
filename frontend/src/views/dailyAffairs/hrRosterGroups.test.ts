import test from 'node:test'
import assert from 'node:assert/strict'
import {
  ROSTER_FIELD_GROUPS, LEAVING_GROUP_KEY, ROSTER_STATUS_OPTIONS,
  allRosterFields, defaultVisibleFields, lockedRosterFields,
  visibleFieldsFor, drawerGroupsFor, groupLabelOf, groupCheckState,
  defaultVisibleKeysFromConfig, enabledFieldKeysFromConfig,
} from './hrRosterGroups'

test('11 大分组，顺序与设计稿一致（第 50 轮：在职信息在前）', () => {
  assert.equal(ROSTER_FIELD_GROUPS.length, 11)
  assert.deepEqual(ROSTER_FIELD_GROUPS.map((g) => g.key), [
    '在职信息', '基本信息', '离职信息', '联系信息', '紧急联系人情况',
    '家庭情况', '教育情况', '工作经历情况', '合同签订情况', '工资社保信息', '个税申报信息',
  ])
})

test('列顺序：离职信息整组排最后（不插在岗位后面）', () => {
  const all = allRosterFields()
  const idxLeaving = all.indexOf('离职日期')
  // 除了离职信息组自己的字段，其余全部排在离职日期之前
  for (const k of ['工号', '姓名', '部门', '岗位', '证件号码', '个人股本(投资)额']) {
    assert.ok(all.indexOf(k) < idxLeaving, `${k} 应排在离职日期之前`)
  }
  // 离职信息组内字段连续且都在末尾
  assert.equal(all.slice(idxLeaving).every((n) => all.indexOf(n) >= idxLeaving), true)
})

test('姓名必选锁定，且是唯一 locked 字段', () => {
  assert.deepEqual(lockedRosterFields(), ['姓名'])
  const name = ROSTER_FIELD_GROUPS[0].fields.find((f) => f.name === '姓名')
  assert.equal(name?.checked, true)
  assert.equal(name?.locked, true)
})

test('全部字段去重', () => {
  const all = allRosterFields()
  assert.equal(new Set(all).size, all.length)
  assert.ok(all.includes('工号') && all.includes('离职证明') && all.includes('个人股本(投资)额'))
})

test('字段定义默认勾选：各组关键字段为 ☑，未勾选字段为 ☐', () => {
  // checked = 字段定义里的默认勾选（控制详情抽屉展示），不是列表默认表头
  const on = new Set(ROSTER_FIELD_GROUPS.flatMap((g) => g.fields.filter((f) => f.checked).map((f) => f.name)))
  // 基本信息
  for (const k of ['姓名', '证件号码', '性别', '年龄', '司龄', '健康证有效期']) assert.ok(on.has(k), k)
  // 生日 / 工龄 已合并下线
  assert.ok(!on.has('生日') && !on.has('工龄') && !on.has('司龄开始日期') && !on.has('司龄(天数)'))
  for (const k of ['曾用名', '英文名', '国家地区', '户籍城市(省)']) assert.ok(!on.has(k), k)
  // 在职信息
  for (const k of ['工号', '部门', '职务', '岗位', '入职日期', '试用期', '转正日期']) assert.ok(on.has(k), k)
  for (const k of ['职级编号', '工作性质', '档案编号', 'OA编码']) assert.ok(!on.has(k), k)
  // 离职信息（全部默认勾选）
  for (const k of ['离职日期', '离职原因', '离职备注', '离职证明']) assert.ok(on.has(k), k)
  // 联系信息
  assert.ok(on.has('手机号码') && on.has('现居住地'))
  assert.ok(!on.has('QQ号') && !on.has('企业微信账号'))
  // 紧急联系人
  assert.ok(on.has('紧急联系人姓名') && on.has('紧急联系人电话'))
  assert.ok(!on.has('紧急联系人关系') && !on.has('紧急联系人地址'))
  // 家庭 / 教育 / 工作经历
  assert.ok(on.has('婚姻状况'))
  assert.ok(!on.has('配偶姓名') && !on.has('子女数'))
  assert.ok(on.has('最高学历') && !on.has('毕业院校'))
  assert.ok(on.has('上家工作单位') && on.has('上家工作职务'))
  // 合同 / 工资社保
  for (const k of ['现合同编号', '现合同类型', '现合同结束日期', '合同状态']) assert.ok(on.has(k), k)
  for (const k of ['工资卡卡号', '工资卡开户行', '个人社保账号', '个人公积金账号']) assert.ok(on.has(k), k)
  assert.ok(!on.has('银行名称') && !on.has('工资卡开户城市'))
  // 个税申报整组默认不勾选
  for (const k of ['是否烈属', '烈属证号', '是否股东/投资者']) assert.ok(!on.has(k), k)
})

test('在职状态只剩 全部 / 在职 / 离职 三项', () => {
  assert.deepEqual(ROSTER_STATUS_OPTIONS.map((o) => o.value), ['全部', '在职', '离职'])
})

test('默认表头 = 在职信息 12 + 基本信息 18（第 50 轮）', () => {
  const vis = defaultVisibleFields()
  const set = new Set(vis)
  // 在职信息组打头，工号第一、姓名第二
  assert.equal(vis[0], '工号')
  assert.equal(vis[1], '姓名')
  for (const k of ['工号', '姓名', '部门', '职务', '岗位', '合同公司', '招聘渠道',
    '员工状态', '入职日期', '试用期', '转正日期', '司龄']) {
    assert.ok(set.has(k), `在职信息缺少 ${k}`)
  }
  for (const k of ['证件类型', '证件号码', '性别', '出生日期', '年龄', '健康证有效期']) {
    assert.ok(set.has(k), `基本信息缺少 ${k}`)
  }
  // 其余分组默认不进表头
  for (const k of ['离职日期', '现合同编号', '工资卡卡号', '是否烈属', '毕业院校', '紧急联系人姓名']) {
    assert.ok(!set.has(k), `${k} 不应默认进表头`)
  }
  assert.equal(vis.length, 30, `默认列数应为 30，实际 ${vis.length}`)
})

test('visibleFieldsFor：在职隐藏离职信息组，离职/全部包含', () => {
  const active = visibleFieldsFor('在职')
  const left = visibleFieldsFor('离职')
  const all = visibleFieldsFor('全部')
  assert.ok(!active.includes('离职日期'))
  assert.ok(!active.includes('离职原因'))
  assert.ok(left.includes('离职日期') && left.includes('离职原因'))
  assert.ok(all.includes('离职日期'))
  // 在职信息组始终在
  assert.ok(active.includes('工号') && active.includes('部门'))
  // 姓名锁定，即使 overrides 为空集合也要出现
  assert.ok(visibleFieldsFor('在职', new Set()).includes('姓名'))
})

test('drawerGroupsFor：空组自动丢弃，在职隐藏离职组', () => {
  const groups = drawerGroupsFor('在职')
  assert.ok(!groups.some((g) => g.key === LEAVING_GROUP_KEY))
  assert.ok(groups.every((g) => g.fields.length > 0), '不应出现空分组')
  const leftGroups = drawerGroupsFor('离职')
  assert.ok(leftGroups.some((g) => g.key === LEAVING_GROUP_KEY))
  // 个税申报信息默认全不勾选 → 在职/离职都应被丢弃
  assert.ok(!leftGroups.some((g) => g.key === '个税申报信息'))
})

test('groupCheckState：全选 / 半选 / 未选', () => {
  const basic = ROSTER_FIELD_GROUPS[0]
  const allSet = new Set(basic.fields.map((f) => f.name))
  assert.equal(groupCheckState(basic, allSet), 'all')
  assert.equal(groupCheckState(basic, new Set()), 'partial') // 姓名 locked 恒勾选
  const tax = ROSTER_FIELD_GROUPS[10]
  assert.equal(groupCheckState(tax, new Set()), 'none')
})

test('groupLabelOf：字段归属分组', () => {
  assert.equal(groupLabelOf('工号'), '在职信息')
  assert.equal(groupLabelOf('离职日期'), '离职信息')
  assert.equal(groupLabelOf('工资卡卡号'), '工资社保信息')
  assert.equal(groupLabelOf('不存在的字段'), '')
})

// ---------------------------------------------------------------------------
// 第 50 轮：六同步契约（后端 subs → 默认表头 / 可选全集）
// ---------------------------------------------------------------------------

test('第50轮：defaultVisibleKeysFromConfig 只取 enabled && is_default', () => {
  const config = [{
    builtin_key: 'hr-roster',
    subs: [
      { name: '工号', enabled: true, is_default: true, group: '在职信息' },
      { name: '姓名', enabled: true, is_default: true, group: '在职信息' },
      { name: '招聘渠道', enabled: true, is_default: false, group: '在职信息' },
      { name: '离职日期', enabled: true, is_default: false, group: '离职信息' },
      { name: '曾用名', enabled: false, is_default: true, group: '基本信息' },
    ],
  }]
  assert.deepEqual(defaultVisibleKeysFromConfig(config), ['工号', '姓名'])
  // 禁用的字段即便 is_default=true 也不能进表头
  assert.ok(!defaultVisibleKeysFromConfig(config).includes('曾用名'))
  // 顺序 = subs 数组顺序（后端已按分组重排）
  assert.equal(defaultVisibleKeysFromConfig(config)[0], '工号')
})

test('第50轮：enabledFieldKeysFromConfig 取全部启用字段（含默认表头关闭的）', () => {
  const config = [{
    builtin_key: 'hr-roster',
    subs: [
      { name: '工号', enabled: true, is_default: true },
      { name: '招聘渠道', enabled: true, is_default: false },
      { name: '曾用名', enabled: false, is_default: true },
    ],
  }]
  assert.deepEqual(enabledFieldKeysFromConfig(config), ['工号', '招聘渠道'])
})

test('第50轮：配置缺失时回落到静态默认，绝不给空表头', () => {
  for (const bad of [null, undefined, [], [{}], [{ subs: null }]]) {
    const keys = defaultVisibleKeysFromConfig(bad as any)
    assert.ok(keys.length >= 28, `配置 ${JSON.stringify(bad)} 时应有兜底列`)
    assert.ok(keys.includes('工号'))
  }
})

test('第50轮：在职态严格无离职字段，离职态末尾追加', () => {
  const active = visibleFieldsFor('在职')
  const left = visibleFieldsFor('离职')
  for (const k of ['离职日期', '离职原因', '离职备注', '离职证明']) {
    assert.ok(!active.includes(k), `在职态不应有 ${k}`)
  }
  // 离职态：默认在职字段原样在前，离职字段全部在末尾
  const firstLeaving = left.findIndex((n) => n === '离职日期')
  assert.ok(firstLeaving > 0)
  for (const k of ['工号', '姓名', '部门', '岗位', '证件号码']) {
    assert.ok(left.indexOf(k) < firstLeaving, `${k} 应在离职字段之前`)
  }
})
