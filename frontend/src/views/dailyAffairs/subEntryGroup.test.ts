import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildColumnsFromSubs,
  buildDrawerFieldsFromSubs,
  buildRuleFieldsFromSubs,
  listSubGroups,
  hasSubGroups,
  groupOfSub,
  type SubEntry,
} from './sixWaySync'

/**
 * P0：`SubEntry.group` 切片。
 *
 * 核心验收口径是「空 group 即旧行为」——发票/合同/制度/公告四个已中台化模块
 * 的 subs 里没有 group 键，它们的列表列、抽屉字段、提取规则行必须与改造前
 * 逐项一致。任何在这条路径上引入的行为漂移都是回归。
 */

const labelOf = (n: string) => ({ a: '甲', b: '乙', c: '丙' } as Record<string, string>)[n] || ''

// ---- 兼容性：无 group 的旧数据行为逐项不变 ----

test('无 group 的 subs：列派生与改造前完全一致（集合/顺序/名称/类型）', () => {
  const subs: SubEntry[] = [
    { name: 'a', label: '甲改', data_type: 'text', is_default: true },
    { name: 'b', data_type: 'number' },
    { name: 'c', enabled: false },
  ]
  const cols = buildColumnsFromSubs(subs, { labelOf })
  assert.deepEqual(cols.map(c => c.key), ['a', 'b'], '禁用字段不产出')
  assert.deepEqual(cols.map(c => c.label), ['甲改', '乙'], 'subs.label 优先，其次字典')
  assert.deepEqual(cols.map(c => c.dataType), ['text', 'number'])
})

test('无 group 的 subs：抽屉字段与规则字段不受影响', () => {
  const subs: SubEntry[] = [{ name: 'a' }, { name: 'b', data_type: 'number' }]
  const drawer = buildDrawerFieldsFromSubs(subs, { labelOf })
  assert.deepEqual(drawer.map(f => f.name), ['a', 'b'])
  const rules = buildRuleFieldsFromSubs(subs, {
    labelOf,
    savedByKey: new Map([['a', { desc: '描述A', rule: '规则A', regex: '^x' }]]),
  })
  assert.equal(rules[0].desc, '描述A')
  assert.equal(rules[0].type, 'string')
  assert.equal(rules[1].type, 'number')
})

test('groupFilter 缺省时绝不过滤——这是兼容铁律', () => {
  const subs: SubEntry[] = [
    { name: 'a', group: 'g1' },
    { name: 'b', group: 'g2' },
    { name: 'c' },
  ]
  // 不传 groupFilter：三个字段全出（旧调用方从不传这个键）
  assert.deepEqual(buildColumnsFromSubs(subs, { labelOf }).map(c => c.key), ['a', 'b', 'c'])
  // 传 undefined 等价于不传
  assert.deepEqual(
    buildColumnsFromSubs(subs, { labelOf, groupFilter: undefined }).map(c => c.key),
    ['a', 'b', 'c'],
  )
})

// ---- 分组过滤：12 个费用分项各取各的 ----

test('groupFilter 传字符串：只产出该分组字段', () => {
  const subs: SubEntry[] = [
    { name: 'account_no', group: 'overview' },
    { name: 'total_kwh', group: 'overview' },
    { name: 'market_a', group: 'market' },
    { name: 'line_a', group: 'line' },
  ]
  assert.deepEqual(
    buildColumnsFromSubs(subs, { labelOf, groupFilter: 'overview' }).map(c => c.key),
    ['account_no', 'total_kwh'],
  )
  assert.deepEqual(
    buildColumnsFromSubs(subs, { labelOf, groupFilter: 'market' }).map(c => c.key),
    ['market_a'],
  )
  assert.deepEqual(buildColumnsFromSubs(subs, { labelOf, groupFilter: 'pf' }), [], '空分组产出空列')
})

test('groupFilter 传数组：命中任一键即产出（主列表取概况+未分组）', () => {
  const subs: SubEntry[] = [
    { name: 'account_no', group: 'overview' },
    { name: 'legacy_no_group' },
    { name: 'market_a', group: 'market' },
  ]
  assert.deepEqual(
    buildColumnsFromSubs(subs, { labelOf, groupFilter: ['', 'overview'] }).map(c => c.key),
    ['account_no', 'legacy_no_group'],
    '空串代表未分组，与 overview 一起构成主表口径',
  )
})

test('分组过滤同样作用于抽屉字段与规则字段（六同步不能只在表头生效）', () => {
  const subs: SubEntry[] = [
    { name: 'account_no', group: 'overview' },
    { name: 'market_a', group: 'market', data_type: 'number' },
  ]
  assert.deepEqual(
    buildDrawerFieldsFromSubs(subs, { labelOf, groupFilter: 'market' }).map(f => f.name),
    ['market_a'],
  )
  const rules = buildRuleFieldsFromSubs(subs, {
    labelOf,
    groupFilter: 'market',
    savedByKey: new Map([['market_a', { rule: '取市场化购电费行合计' }]]),
  })
  assert.deepEqual(rules.map(r => r.name), ['market_a'])
  assert.equal(rules[0].rule, '取市场化购电费行合计')
  assert.equal(rules[0].type, 'number')
})

// ---- 分组枚举（胶囊条数据源）----

test('listSubGroups：按首次出现排序，count 累加，label 后到补前空', () => {
  const subs: SubEntry[] = [
    { name: 'a', group: 'overview', group_label: '账单概况' },
    { name: 'b', group: 'market' },
    { name: 'c', group: 'overview' },
    { name: 'd', group: 'market', group_label: '市场化购电费' },
  ]
  const groups = listSubGroups(subs)
  assert.deepEqual(groups.map(g => g.key), ['overview', 'market'])
  assert.deepEqual(groups.map(g => g.count), [2, 2])
  assert.equal(groups[0].label, '账单概况')
  // market 首条没写 group_label，由第二条补上
  assert.equal(groups[1].label, '市场化购电费')
})

test('listSubGroups：无 group 的旧数据只产出一个空键分组', () => {
  const groups = listSubGroups([{ name: 'a' }, { name: 'b' }])
  assert.equal(groups.length, 1)
  assert.equal(groups[0].key, '')
  assert.equal(groups[0].count, 2)
})

test('hasSubGroups：无 group 判 false（据此不渲染胶囊条）', () => {
  assert.equal(hasSubGroups([{ name: 'a' }, { name: 'b' }]), false, '发票/合同等旧 subs')
  assert.equal(hasSubGroups([{ name: 'a', group: 'overview' }]), true, '单个具名分组也算有分组维度')
  assert.equal(hasSubGroups([{ name: 'a', group: 'overview' }, { name: 'b' }]), true)
  assert.equal(hasSubGroups([]), false)
  assert.equal(hasSubGroups(undefined), false)
})

test('groupOfSub：空值/空白/缺键统一归一为空串', () => {
  assert.equal(groupOfSub({ name: 'a' }), '')
  assert.equal(groupOfSub({ name: 'a', group: '' }), '')
  assert.equal(groupOfSub({ name: 'a', group: '   ' }), '')
  assert.equal(groupOfSub({ name: 'a', group: ' market ' }), 'market')
})
