// 人事管理（第 45 轮）纯函数单测。
//
// 覆盖设计文档 §5 的前端 5 组用例：
//   1. 三卡片恰好 3 张且无「已收录」/「历史记录」；
//   2. 字段种子 width 全 0、状态字段 builtin；
//   3. 月份倒序纯函数；
//   4. 列定义无 fixed、无操作列；
//   5. 0 行 footRow 除首列全空。
// 另加 hrScopeOf / hrRowKey / KPI 高亮契约的回归断言。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  HR_TABS,
  HR_STATUS_FIELDS,
  buildHrKpiCards,
  hrScopeOf,
  hrColumnDefs,
  hrBillColumnDefs,
  hrSummaryColumns,
  sortMonthsDesc,
  isHrMonth,
  hrFieldSeeds,
  hrFootRowOf,
  hrRowKey,
  hrRosterStatusColumns,
  hrRosterAllColumns,
} from './hrCore'
import { isLockedStatusField, statusFieldSpecsOf, isHrStatusName } from './statusFields'

const ALL_TABS = ['hr-roster', 'hr-social', 'hr-fund'] as const

test('KPI：恰好 3 张卡，且绝不出现「已收录记录」/「历史记录」汇总卡', () => {
  const cards = buildHrKpiCards({ 'hr-roster': 12, 'hr-social': 9, 'hr-fund': 6 })
  assert.equal(cards.length, 3)
  assert.deepEqual(cards.map((c) => c.label), ['花名册', '社保管理', '公积金管理'])
  // 卡里任何字段都不许带上被移除的两张汇总卡文案
  const text = JSON.stringify(cards)
  assert.equal(text.includes('已收录'), false)
  assert.equal(text.includes('历史记录'), false)
  // 空 counts 也仍是 3 张、数值 0
  assert.equal(buildHrKpiCards().length, 3)
  assert.deepEqual(buildHrKpiCards().map((c) => c.value), ['0', '0', '0'])
})

test('KPI：三张卡都是 action=type + typeValue，才能被 DashboardKpiGroup 高亮', () => {
  const cards = buildHrKpiCards()
  for (const c of cards) {
    assert.equal(c.action, 'type')
    assert.ok(c.typeValue, `${c.key} 必须有 typeValue`)
    assert.notEqual((c as { static?: boolean }).static, true)
  }
  assert.deepEqual(cards.map((c) => c.typeValue), [...ALL_TABS])
  assert.equal(HR_TABS.length, 3)
})

test('hrScopeOf：三个 HR scope 恒等映射，未知值兜底花名册', () => {
  assert.equal(hrScopeOf('hr-roster'), 'hr-roster')
  assert.equal(hrScopeOf('hr-social'), 'hr-social')
  assert.equal(hrScopeOf('hr-fund'), 'hr-fund')
  assert.equal(hrScopeOf('unknown'), 'hr-roster')
  assert.equal(hrScopeOf(''), 'hr-roster')
})

test('列定义：一律无 fixed、无操作列，第一列是勾选列（也不是 fixed）', () => {
  const allCols = [
    ...ALL_TABS.flatMap((t) => hrColumnDefs(t)),
    ...hrBillColumnDefs(),
  ]
  assert.ok(allCols.length > 3)
  for (const c of allCols) {
    assert.equal('fixed' in c, false, `${c.colKey} 不得带 fixed`)
    assert.doesNotMatch(c.colKey, /操作|action|operation|handle/i, `${c.colKey} 不得是操作列`)
    assert.notEqual((c as any).type, 'operation')
  }
  for (const t of ALL_TABS) {
    const first = hrColumnDefs(t)[0]
    assert.equal(first.colKey, 'row-select', `${t} 第一列必须是勾选列`)
    assert.equal(first.type, 'multiple')
    assert.equal('fixed' in first, false)
  }
  // 业务列都不带 fixed，且不含 row-select 之外的系统列
  for (const t of ALL_TABS) {
    assert.ok(hrColumnDefs(t).length > 1, `${t} 必须有业务列`)
  }
})

test('sortMonthsDesc：严格 YYYY-MM 倒序', () => {
  assert.deepEqual(sortMonthsDesc(['2026-09', '2026-10'], ''), ['2026-10', '2026-09'])
  const rows = [
    { year_month: '2026-09', n: 1 },
    { year_month: '2026-10', n: 2 },
    { year_month: '2025-12', n: 3 },
  ]
  assert.deepEqual(sortMonthsDesc(rows, 'year_month').map((r) => r.year_month), [
    '2026-10', '2026-09', '2025-12',
  ])
})

test('sortMonthsDesc：过滤非法值且稳定（同月保持原顺序）', () => {
  const rows = [
    { m: '2026-10', tag: 'a' },
    { m: 'bad', tag: 'x' },
    { m: '2026-10', tag: 'b' },
    { m: '2026-09', tag: 'c' },
    { m: '2026-13', tag: 'y' },
  ]
  const out = sortMonthsDesc(rows, 'm')
  assert.deepEqual(out.map((r) => r.tag), ['a', 'b', 'c'])
  assert.equal(isHrMonth('2026-10'), true)
  assert.equal(isHrMonth('2026-13'), false)
  assert.equal(isHrMonth('bad'), false)
})

test('字段种子：width 全 0，四个状态字段 builtin + enabled', () => {
  for (const scope of ALL_TABS) {
    const seeds = hrFieldSeeds(scope)
    assert.ok(seeds.length > 0)
    for (const s of seeds) {
      assert.equal(s.width, 0, `${scope}/${s.name} 列宽必须是 0`)
      assert.equal(s.enabled, true)
    }
  }
  // 四个状态名（在职状态/账期状态/参保状态/缴存状态）都被 builtin 覆盖
  const all = ALL_TABS.flatMap((s) => hrFieldSeeds(s))
  const statusSeeds = all.filter((s) => HR_STATUS_FIELDS.includes(s.name))
  assert.ok(statusSeeds.length >= 4)
  for (const s of statusSeeds) {
    assert.equal(s.builtin, true, `${s.name} 必须 builtin`)
    assert.equal(s.enabled, true, `${s.name} 必须 enabled`)
  }
  for (const name of HR_STATUS_FIELDS) {
    assert.ok(statusSeeds.some((s) => s.name === name), `${name} 必须出现在字段种子里`)
  }
})

test('hrFootRowOf：0 行时除首列「合计（共 0 条）」外全部为空串', () => {
  for (const t of ALL_TABS) {
    const foot = hrFootRowOf(t, [])
    assert.equal(foot['row-select'], '合计（共 0 条）')
    for (const c of hrSummaryColumns(t)) {
      assert.equal(foot[c.key], '', `${t}/${c.key} 0 行时必须留空`)
    }
    // 除首列文案与内部 id 之外，不允许出现任何 0 / 0.00 / —
    for (const [k, v] of Object.entries(foot)) {
      if (k === 'row-select' || k === 'id') continue
      assert.equal(v, '', `${t}/${k} 不该有值：${v}`)
    }
  }
})

test('hrFootRowOf：有行时首列文案随条数变化', () => {
  const rows = [{ 工号: 'A001', 姓名: '张三', 在职状态: '在职' }]
  const foot = hrFootRowOf('hr-roster', rows)
  assert.equal(foot['row-select'], '合计（共 1 条）')
})

test('状态字段在 HR scope 下被锁定，且既有 scope 行为不被破坏', () => {
  for (const [scope, name] of [
    ['hr-roster', '在职状态'],
    ['hr-social', '账期状态'],
    ['hr-social', '参保状态'],
    ['hr-fund', '缴存状态'],
  ]) {
    assert.equal(isLockedStatusField(scope, '', name), true, `${scope}/${name} 必须被锁定`)
  }
  assert.equal(isHrStatusName('在职状态'), true)
  assert.equal(isHrStatusName('状态'), false)
  // HR scope 的普通业务字段不锁
  assert.equal(isLockedStatusField('hr-roster', '', '姓名'), false)
  // 既有 scope 契约不变
  assert.equal(isLockedStatusField('tire', '轮胎管理', '状态'), false)
  assert.equal(isLockedStatusField('vehicle', '道路运输证', '证照状态'), true)
  // 三个 HR scope 各自注入对应状态字段
  assert.deepEqual(statusFieldSpecsOf('hr-roster').map((s) => s.name), ['在职状态'])
  assert.deepEqual(statusFieldSpecsOf('hr-social').map((s) => s.name), ['账期状态', '参保状态'])
  assert.deepEqual(statusFieldSpecsOf('hr-fund').map((s) => s.name), ['缴存状态'])
})

test('hrRowKey：id 优先并回退业务唯一键', () => {
  assert.equal(hrRowKey({ id: 'e1' }), 'e1')
  assert.equal(hrRowKey({ employee_id: 'E002' }), 'E002')
  assert.equal(hrRowKey({ year_month: '2026-10' }), '2026-10')
  assert.equal(hrRowKey(null), '')
})

// ---------------------------------------------------------------------------
// 第 46 轮
// ---------------------------------------------------------------------------

test('第46轮：社保卡片图标是有效名 secured，不再是无效的 shield', () => {
  const social = HR_TABS.find((t) => t.key === 'hr-social')
  assert.equal(social?.icon, 'secured')
  assert.notEqual(social?.icon, 'shield')
  assert.equal(HR_TABS.find((t) => t.key === 'hr-roster')?.icon, 'usergroup')
  assert.equal(HR_TABS.find((t) => t.key === 'hr-fund')?.icon, 'wallet')
})

test('第46轮：列工厂对时间/日期列做格式化，ISO 原串不上屏', () => {
  const iso = '2026-11-07T20:12:33.37663+08:00'
  const social = hrColumnDefs('hr-social')
  const created = social.find((c) => c.colKey === '创建时间')
  assert.ok(created)
  const out = created!.value?.({ created_at: iso }) ?? ''
  assert.equal(out, '2026-11-07 20:12:33')
  assert.ok(!out.includes('T') && !out.includes('+08:00') && !out.includes('.37663'))

  const roster = hrColumnDefs('hr-roster')
  const hire = roster.find((c) => c.colKey === '入职日期')
  assert.equal(hire?.value?.({ hire_date: '2018-07-01 00:00:00' }), '2018-07-01')
})

test('第46轮：四态动态列——试用期/离职各自专属字段', () => {
  const trial = hrRosterStatusColumns('试用期').map((c) => c.colKey)
  assert.ok(trial.includes('试用期') && trial.includes('转正日期'))
  assert.ok(!trial.includes('离职日期'))
  const left = hrRosterStatusColumns('离职').map((c) => c.colKey)
  for (const h of ['离职日期', '离职原因', '离职证明', '离职备注']) {
    assert.ok(left.includes(h), `离职视图缺少 ${h}`)
  }
  assert.ok(!left.includes('试用期') && !left.includes('个人社保账号'))
  for (const view of ['在职', '试用期', '退休返聘', '离职'] as const) {
    const cols = hrRosterStatusColumns(view)
    assert.equal(cols[0].colKey, 'row-select')
    assert.ok(cols.every((c) => !('fixed' in c)))
    // 勾选列固定 44px（中台统一），业务字段列宽一律 0 = 自适应。
    assert.ok(cols.slice(1).every((c) => c.width === 0))
  }
})

test('第46轮：字段全集覆盖 76 表头 + 派生 + 离职扩展，且无重复 colKey', () => {
  const cols = hrRosterAllColumns()
  const keys = cols.map((c) => c.colKey)
  assert.equal(new Set(keys).size, keys.length)
  assert.ok(keys.length >= 78)
  assert.ok(keys.includes('工号') && keys.includes('现合同签订日期'))
  assert.ok(keys.includes('离职证明') && keys.includes('离职备注') && keys.includes('在职状态'))
})
