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
  isRosterCodeField,
  hrSocialColumns,
  contractStatusOf,
  contractStatusTheme,
  CONTRACT_STATUS_OPTIONS,
  hrRankOfTitle,
  hrDeptCodeFromEmployeeID,
  hrSeqFromEmployeeID,
  hrNextEmployeeID,
} from './hrCore'
import { buildFootRow, type SummaryColumn } from './listSummary'
import { isLockedStatusField, statusFieldSpecsOf, isHrStatusName } from './statusFields'

const ALL_TABS = ['hr-roster', 'hr-resign', 'hr-contract', 'hr-social', 'hr-fund'] as const

test('KPI：第 52 轮恰好 5 张卡，顺序为 花名册→离职管理→劳动合同→社保管理→公积金管理', () => {
  const cards = buildHrKpiCards({
    'hr-roster': 12, 'hr-resign': 3, 'hr-contract': 5, 'hr-social': 9, 'hr-fund': 6,
  })
  assert.equal(cards.length, 5)
  assert.deepEqual(cards.map((c) => c.label),
    ['花名册', '离职管理', '劳动合同', '社保管理', '公积金管理'])
  // 卡里任何字段都不许带上被移除的两张汇总卡文案
  const text = JSON.stringify(cards)
  assert.equal(text.includes('已收录'), false)
  assert.equal(text.includes('历史记录'), false)
  // 空 counts 也仍是 5 张、数值 0
  assert.equal(buildHrKpiCards().length, 5)
  assert.deepEqual(buildHrKpiCards().map((c) => c.value), ['0', '0', '0', '0', '0'])
})

test('KPI：五张卡都是 action=type + typeValue，才能被 DashboardKpiGroup 高亮', () => {
  const cards = buildHrKpiCards()
  for (const c of cards) {
    assert.equal(c.action, 'type')
    assert.ok(c.typeValue, `${c.key} 必须有 typeValue`)
    assert.notEqual((c as { static?: boolean }).static, true)
  }
  assert.deepEqual(cards.map((c) => c.typeValue), [...ALL_TABS])
  assert.equal(HR_TABS.length, 5)
})

test('hrScopeOf：五个 HR scope 恒等映射，未知值兜底花名册', () => {
  assert.equal(hrScopeOf('hr-roster'), 'hr-roster')
  assert.equal(hrScopeOf('hr-resign'), 'hr-resign')
  assert.equal(hrScopeOf('hr-social'), 'hr-social')
  assert.equal(hrScopeOf('hr-contract'), 'hr-contract')
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
  // 第 52 轮：花名册不再注入「在职状态」；公积金口径不变
  assert.deepEqual(statusFieldSpecsOf('hr-roster').map((s) => s.name), [])
  // 第 56 轮：社保四胶囊契约内置五险参保状态 + 账单状态，旧的两个幽灵状态列不再注入
  assert.deepEqual(statusFieldSpecsOf('hr-social').map((s) => s.name), [])
  // 第 54 轮：公积金状态列已内建为「账户状态」，不再额外注入「缴存状态」
  assert.deepEqual(statusFieldSpecsOf('hr-fund').map((s) => s.name), [])
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

// ---------------------------------------------------------------------------
// 第 47 轮
// ---------------------------------------------------------------------------

test('第47轮：卡号/账号/证件号一律字符串，绝不千分位、绝不参与合计', () => {
  for (const name of ['工资卡卡号', '个人社保账号', '个人公积金账号', '证件号码', '手机号码', '工号',
    '考勤编号', '档案编号', '现合同编号', 'QQ号', '分机号', '紧急联系人电话']) {
    assert.equal(isRosterCodeField(name), true, `${name} 应判为号码类`)
    const cols = hrRosterAllColumns()
    const hit = cols.find((c) => c.colKey === name)
    if (!hit) continue
    // dataType 不能是 number（否则会被合计行求和）
    assert.notEqual(hit.dataType, 'number', `${name} 不能是 number 类型`)
    const long = '6217003760153123456'
    const out = hit.value?.({ [name]: long, custom_data: { [name]: long } }) ?? ''
    assert.equal(out, long, `${name} 必须原样输出`)
    assert.ok(!out.includes(','), `${name} 不能有千分位`)
  }
})

test('第47轮：合计行对号码列留空，不对卡号求和', () => {
  // 用字段全集（含工资卡卡号）构造合计列，等价于用户在「字段」里勾上该列的场景
  const allCols = hrRosterAllColumns().filter((c) => c.colKey !== 'row-select')
  const cols: SummaryColumn[] = allCols.map((c) => ({
    key: c.colKey,
    label: c.title,
    fieldType: c.dataType === 'number' ? 'number' : 'text',
    value: (row: any) => c.value?.(row) ?? '',
  }))
  const card = cols.find((c) => c.key === '工资卡卡号')
  assert.ok(card, '合计列应包含工资卡卡号（但不可加）')
  const row = buildFootRow(cols, [
    { 工资卡卡号: '6217003760153123456' },
    { 工资卡卡号: '6217003760153999888' },
  ], { selectKey: 'row-select' })
  assert.equal(String(row['工资卡卡号'] ?? ''), '', '合计行卡号必须为空串')
  assert.ok(!JSON.stringify(row).includes('118179'), '绝不能出现求和后的荒谬数字')
})

test('第54轮：合同状态五态判定（未签订 / 已解除 补齐）', () => {
  const today = new Date('2026-11-10T00:00:00Z')
  // 无固定期限恒正常
  assert.equal(contractStatusOf('', '无固定期限劳动合同', today), '正常')
  assert.equal(contractStatusOf('2020-01-01', '无固定期限劳动合同', today), '正常')
  // 剩余 > 30 天
  assert.equal(contractStatusOf('2027-06-30', '固定期限劳动合同', today), '正常')
  // 0~30 天
  assert.equal(contractStatusOf('2026-11-20', '固定期限劳动合同', today), '即将到期')
  assert.equal(contractStatusOf('2026-11-10', '固定期限劳动合同', today), '即将到期')
  // 已过期
  assert.equal(contractStatusOf('2026-10-01', '固定期限劳动合同', today), '到期')
  // 未填且非无固定期限 → 未签订
  assert.equal(contractStatusOf('', '固定期限劳动合同', today), '未签订')
  // 五态主题
  assert.equal(contractStatusTheme('正常'), 'success')
  assert.equal(contractStatusTheme('未签订'), 'warning')
  assert.equal(contractStatusTheme('即将到期'), 'warning')
  assert.equal(contractStatusTheme('到期'), 'danger')
  assert.equal(contractStatusTheme('已解除'), 'default')
  assert.equal(contractStatusTheme(''), 'default')
  // 下拉 6 项（全部 + 5 态）
  assert.deepEqual(CONTRACT_STATUS_OPTIONS.map((o) => o.value),
    ['全部状态', '正常', '未签订', '即将到期', '到期', '已解除'])
})

test('第47轮：社保原始/汇总双视图列集合', () => {
  const raw = hrSocialColumns('raw').map((c) => c.colKey)
  for (const h of ['账期', '记录性质', '险种', '个人社保编号', '单位应缴', '个人应缴', '补退金额']) {
    assert.ok(raw.includes(h), `原始记录缺少 ${h}`)
  }
  const sum = hrSocialColumns('summary').map((c) => c.colKey)
  for (const h of ['账期', '参保人数', '养老单位', '医疗个人', '大病', '工伤单位', '应缴总计', '状态']) {
    assert.ok(sum.includes(h), `汇总记录缺少 ${h}`)
  }
  // 两套列都不带 fixed、无操作列
  for (const cols of [hrSocialColumns('raw'), hrSocialColumns('summary')]) {
    assert.ok(cols.every((c) => !('fixed' in c)))
    assert.ok(!cols.some((c) => c.colKey === 'operations'))
  }
})

test('第52轮：劳动合同列 = 乙方 6 + 甲方 3 + 合同 8，含三色合同状态', () => {
  const cols = hrColumnDefs('hr-contract').map((c) => c.colKey)
  for (const h of ['工号', '姓名', '部门', '岗位', '证件号码', '联系电话',
    '统一社会信用代码', '住所', '法定代表人',
    '合同编号', '合同类型', '合同开始日期', '合同结束日期', '合同期限',
    '签约公司', '签订日期', '合同状态']) {
    assert.ok(cols.includes(h), `劳动合同缺少 ${h}`)
  }
  // 已剥离的旧「现合同…」字段名不再出现在劳动合同列表
  for (const h of ['现合同编号', '现合同类型', '现合同结束日期', '职务']) {
    assert.ok(!cols.includes(h), `劳动合同不应再有旧字段 ${h}`)
  }
  assert.ok(cols.every((c) => c !== 'row-select' || cols[0] === 'row-select'))
})

// ---------------------------------------------------------------------------
// 第 48 轮：工号自动编码
// ---------------------------------------------------------------------------

test('第48轮：职级前缀判定（A 高管 / B 主管级 / C 普通）', () => {
  for (const t of ['总经理', '副总经理', '董事长']) assert.equal(hrRankOfTitle(t), 'A', t)
  for (const t of ['主管', '行政部主管', '经理', '主任', '部长', '科长', '组长']) assert.equal(hrRankOfTitle(t), 'B', t)
  for (const t of ['普工', '操作员', '文员', '驾驶员', '检验员', '过磅员', '保安', '保洁', '']) {
    assert.equal(hrRankOfTitle(t), 'C', t)
  }
})

test('第48轮：从存量工号提取部门码与序号', () => {
  assert.equal(hrDeptCodeFromEmployeeID('C020300005'), '02030')
  assert.equal(hrDeptCodeFromEmployeeID('A020100001'), '02010')
  assert.equal(hrDeptCodeFromEmployeeID('C020610012'), '02061')
  // C010300019 也是 10 位合法结构（部门码 01030，历史遗留段）
  assert.equal(hrDeptCodeFromEmployeeID('C010300019'), '01030')
  // 非 10 位结构返回空
  assert.equal(hrDeptCodeFromEmployeeID('E001'), '')
  assert.equal(hrDeptCodeFromEmployeeID(''), '')
  assert.equal(hrSeqFromEmployeeID('C020300005'), 5)
  assert.equal(hrSeqFromEmployeeID('C020500135'), 135)
})

test('第48轮：工号自动递增（销售部 02030 最大 0005 → 0006）', () => {
  const existing = ['C020300001', 'C020300002', 'C020300005', 'C020300006', 'C020300008', 'A020100001']
  assert.equal(hrNextEmployeeID('02030', '普工', existing), 'C020300009')
  assert.equal(hrNextEmployeeID('02030', '主管', existing), 'B020300009')
  assert.equal(hrNextEmployeeID('02030', '总经理', existing), 'A020300009')
  // 空部门 → 无存量 → 0001
  assert.equal(hrNextEmployeeID('02080', '普工', existing), 'C020800001')
})

test('第48轮：部门码缺省时从存量反推', () => {
  const existing = ['C020500135', 'C020500136', 'C020500137']
  assert.equal(hrNextEmployeeID('', '普工', existing), 'C020500138')
})
