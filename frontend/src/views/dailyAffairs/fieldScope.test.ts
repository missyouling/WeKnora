/**
 * 第 60 轮：字段设置的「胶囊作用域」纯函数回归。
 *
 * 背景（图 2 / 图 3）：`fieldsVisible` 里 `tabGroupKey(f.group) === want || isLockedStatus(f)`
 * 让「加锁状态字段」无条件穿透所有胶囊 —— 劳动合同胶囊里出现 2 个合同状态
 * （自己和劳务合同的都算命中），社保正常记录里冒出回盘表的 5 个五险参保状态
 * 与月度账单的账单状态。
 *
 * 另一条：拖拽按 `name` 找下标。劳动合同与劳务合同同名字段（工号/姓名/部门…）
 * 极多，`findIndex` 永远命中第一组，用户拖劳务合同的行，动的却是劳动合同，
 * 表现为「非首胶囊无法拖拽」。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  fieldIdentityOf,
  groupKeyOf,
  hrGroupStatusFieldsOf,
  moveFieldWithinGroup,
  visibleGroupFields,
  HR_GROUP_STATUS_FIELDS,
} from './fieldScope'

const SI_STATUS = ['养老参保状态', '失业参保状态', '医疗参保状态', '工伤参保状态', '生育参保状态']

/** 构造一组带 group 的最小字段。 */
const f = (group: string, name: string, extra: Record<string, any> = {}) => ({ group, name, ...extra })
const names = (rows: any[]) => rows.map((r) => String(r.name))

// ---------------------------------------------------------------------------
// ① 胶囊 → 合法加锁状态字段白名单
// ---------------------------------------------------------------------------

test('白名单：劳动合同 / 劳务合同各有且仅有 1 个合同状态', () => {
  assert.deepEqual(hrGroupStatusFieldsOf('劳动合同'), ['合同状态'])
  assert.deepEqual(hrGroupStatusFieldsOf('劳务合同'), ['合同状态'])
})

test('白名单：社保四胶囊严格隔离', () => {
  assert.deepEqual(hrGroupStatusFieldsOf('回盘表'), SI_STATUS)
  assert.deepEqual(hrGroupStatusFieldsOf('正常记录'), [])
  assert.deepEqual(hrGroupStatusFieldsOf('补退记录'), [])
  assert.deepEqual(hrGroupStatusFieldsOf('月度账单'), ['账单状态'])
})

test('白名单：公积金两胶囊各自唯一', () => {
  assert.deepEqual(hrGroupStatusFieldsOf('职工明细'), ['账户状态'])
  assert.deepEqual(hrGroupStatusFieldsOf('月度账单'), ['账单状态'])
})

test('白名单：非人事胶囊返回 null（不参与严格隔离）', () => {
  assert.equal(hrGroupStatusFieldsOf('电费核算'), null)
  assert.equal(hrGroupStatusFieldsOf(''), null)
  assert.equal(HR_GROUP_STATUS_FIELDS['正常记录'].length, 0)
})

// ---------------------------------------------------------------------------
// ② 严格胶囊过滤（人事）
// ---------------------------------------------------------------------------

test('劳动合同胶囊：跨胶囊的合同状态被挡掉，恰好 1 个', () => {
  const fields = [
    ...['工号', '姓名', '部门'].map((n) => f('劳动合同', n)),
    f('劳动合同', '合同状态'),
    ...['工号', '姓名', '部门'].map((n) => f('劳务合同', n)),
    f('劳务合同', '合同状态'),
  ]
  const shown = visibleGroupFields(fields, '劳动合同', { strictStatus: true })
  assert.deepEqual(names(shown), ['工号', '姓名', '部门', '合同状态'])
})

test('劳务合同胶囊：同名业务字段全部可见，合同状态恰好 1 个', () => {
  const fields = [
    ...['工号', '姓名', '部门'].map((n) => f('劳动合同', n)),
    f('劳动合同', '合同状态'),
    ...['工号', '姓名', '部门'].map((n) => f('劳务合同', n)),
    f('劳务合同', '合同状态'),
  ]
  const shown = visibleGroupFields(fields, '劳务合同', { strictStatus: true })
  assert.deepEqual(names(shown), ['工号', '姓名', '部门', '合同状态'])
})

test('社保正常记录 / 补退记录：0 个加锁字段（五险 + 账单状态全挡掉）', () => {
  const fields = [
    ...SI_STATUS.map((n) => f('回盘表', n, { builtin: true })),
    ...Array.from({ length: 16 }, (_, i) => f('正常记录', `正常字段${i + 1}`)),
    ...Array.from({ length: 16 }, (_, i) => f('补退记录', `补退字段${i + 1}`)),
    f('月度账单', '账单状态', { builtin: true }),
  ]
  for (const g of ['正常记录', '补退记录']) {
    const shown = visibleGroupFields(fields, g, { strictStatus: true })
    assert.equal(shown.length, 16)
    for (const n of SI_STATUS) assert.ok(!names(shown).includes(n), `${g} 不得出现 ${n}`)
    assert.ok(!names(shown).includes('账单状态'), `${g} 不得出现账单状态`)
  }
})

test('社保月度账单：只剩自己的账单状态，五险一个都不许出现', () => {
  const fields = [
    ...SI_STATUS.map((n) => f('回盘表', n, { builtin: true })),
    ...Array.from({ length: 15 }, (_, i) => f('月度账单', `账单字段${i + 1}`)),
    f('月度账单', '账单状态', { builtin: true }),
  ]
  const shown = visibleGroupFields(fields, '月度账单', { strictStatus: true })
  assert.equal(shown.length, 16)
  assert.equal(names(shown).filter((n) => n === '账单状态').length, 1)
  for (const n of SI_STATUS) assert.ok(!names(shown).includes(n))
})

test('严格模式下同一胶囊内重复的加锁字段按名称去重', () => {
  const fields = [f('劳动合同', '姓名'), f('劳动合同', '合同状态'), f('劳动合同', '合同状态')]
  const shown = visibleGroupFields(fields, '劳动合同', { strictStatus: true })
  assert.deepEqual(names(shown), ['姓名', '合同状态'])
})

test('非严格模式保留历史行为：文件级加锁状态在每个胶囊都可见', () => {
  const fields = [
    f('概况', '户号', { builtin: true }),
    f('分项', '电费'),
    f('概况', '状态', { builtin: true }),
  ]
  const shown = visibleGroupFields(fields, '分项', {
    strictStatus: false,
    isLocked: (x: any) => String(x.name) === '状态',
  })
  assert.deepEqual(names(shown), ['电费', '状态'])
})

// ---------------------------------------------------------------------------
// ③ 拖拽排序（按 group + name 身份，跨胶囊同名安全）
// ---------------------------------------------------------------------------

test('身份键：同名不同胶囊必须区分', () => {
  assert.notEqual(fieldIdentityOf(f('劳动合同', '姓名')), fieldIdentityOf(f('劳务合同', '姓名')))
  assert.equal(fieldIdentityOf(f('劳动合同', ' 姓名 ')), fieldIdentityOf(f('劳动合同', '姓名')))
  assert.equal(groupKeyOf(' 劳务合同 '), '劳务合同')
})

test('劳务合同首两字段交换：只动劳务合同，劳动合同一字不动', () => {
  const fields = [
    f('劳动合同', '工号'), f('劳动合同', '姓名'), f('劳动合同', '部门'),
    f('劳务合同', '工号'), f('劳务合同', '姓名'), f('劳务合同', '部门'),
  ]
  const { fields: next, moved } = moveFieldWithinGroup(
    fields,
    fieldIdentityOf(f('劳务合同', '工号')),
    fieldIdentityOf(f('劳务合同', '姓名')),
  )
  assert.equal(moved, true)
  assert.deepEqual(next.map((x: any) => `${x.group}:${x.name}`), [
    '劳动合同:工号', '劳动合同:姓名', '劳动合同:部门',
    '劳务合同:姓名', '劳务合同:工号', '劳务合同:部门',
  ])
})

test('向下拖拽（第一个拖到第三个）落点在目标之前，顺序确定', () => {
  const fields = [
    f('劳务合同', 'A'), f('劳务合同', 'B'), f('劳务合同', 'C'), f('劳务合同', 'D'),
  ]
  const { fields: next } = moveFieldWithinGroup(fields, fieldIdentityOf(f('劳务合同', 'A')), fieldIdentityOf(f('劳务合同', 'C')))
  assert.deepEqual(names(next), ['B', 'C', 'A', 'D'])
})

test('跨胶囊拖拽被拒绝（不得把字段挪到别的胶囊）', () => {
  const fields = [f('劳动合同', '工号'), f('劳务合同', '姓名')]
  const { fields: next, moved } = moveFieldWithinGroup(
    fields,
    fieldIdentityOf(f('劳动合同', '工号')),
    fieldIdentityOf(f('劳务合同', '姓名')),
  )
  assert.equal(moved, false)
  assert.deepEqual(next, fields)
})

test('拖到自己身上 / 未知字段：静默无操作', () => {
  const fields = [f('劳务合同', 'A'), f('劳务合同', 'B')]
  assert.equal(moveFieldWithinGroup(fields, fieldIdentityOf(f('劳务合同', 'A')), fieldIdentityOf(f('劳务合同', 'A'))).moved, false)
  assert.equal(moveFieldWithinGroup(fields, 'nope', fieldIdentityOf(f('劳务合同', 'B'))).moved, false)
})
