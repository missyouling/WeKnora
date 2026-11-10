import test from 'node:test'
import assert from 'node:assert/strict'
import {
  ROSTER_FIELD_GROUPS, LEAVING_GROUP_KEY, ROSTER_STATUS_OPTIONS,
  allRosterFields, defaultVisibleFields, lockedRosterFields,
  visibleFieldsFor, drawerGroupsFor, groupLabelOf, groupCheckState,
} from './hrRosterGroups'

test('11 大分组，顺序与设计稿一致', () => {
  assert.equal(ROSTER_FIELD_GROUPS.length, 11)
  assert.deepEqual(ROSTER_FIELD_GROUPS.map((g) => g.key), [
    '基本信息', '在职信息', '离职信息', '联系信息', '紧急联系人情况',
    '家庭情况', '教育情况', '工作经历情况', '合同签订情况', '工资社保信息', '个税申报信息',
  ])
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

test('列表默认表头只有基本信息组 + 核心锚点（第 49 轮）', () => {
  const vis = new Set(defaultVisibleFields())
  for (const k of ['工号', '姓名', '部门', '岗位']) assert.ok(vis.has(k), k)
  for (const k of ['证件号码', '性别', '出生日期', '年龄', '司龄']) assert.ok(vis.has(k), k)
  // 其余分组默认不进表头
  for (const k of ['离职日期', '现合同编号', '工资卡卡号', '是否烈属', '毕业院校', '紧急联系人姓名']) {
    assert.ok(!vis.has(k), `${k} 不应默认进表头`)
  }
  assert.ok(vis.size <= 25, `默认列数应 <= 25，实际 ${vis.size}`)
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
