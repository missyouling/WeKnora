// 市电账单详情视图纯逻辑的单测（第 24 轮任务一瘦身）。
//
// 这组断言锁的是**真机事故换来的口径**，不是实现细节：
//   · 同名多行优先取有金额的那行（否则账单概况少算钱）；
//   · 分时子项按 `名称(时段)` 一一对应；
//   · 居民行按索引配对，无配置只输出「定比0.015」一行；
//   · 分时占比「平段取余量」，四段合计恰好 100%。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import {
  BILL_MENUS,
  CAPACITY_FIELDS,
  FEE_MENU_MAP,
  META_ORDER,
  PF_FIELDS,
  ROW_GROUP_OF_MENU,
  buildEnergyCompareRows,
  buildMeterRows,
  meterTotalOf,
  touBillKwhOf,
  buildResidentMeterRows,
  feeMenuOf,
  feeRowsOf,
  feeSubtotalOf,
  matchRow,
  menuLabelOf,
  normRowName,
  parseRowKey,
  pickPrevEnergy,
  rowConfigsOf,
  // 第 37 轮：居民电量明细编辑
  isResidentFeeRow,
  recalcResidentBillKwh,
  residentBaseElecOf,
  syncResidentFeeQty,
} from './utilityBillDetail'

// ---- 菜单结构 ----

test('BILL_MENUS：概览 + 两个分组 + 两张独立表', () => {
  assert.equal(BILL_MENUS[0].key, 'overview', '第一个菜单必须是账单概况')
  const groups = BILL_MENUS.filter((m) => m.group)
  assert.deepEqual(groups.map((m) => m.key), ['industrial', 'residential'])
  // 工商业 6 个子表 / 居民 3 个子表（缺一个用户就找不到那张表）
  assert.equal(groups[0].children!.length, 6)
  assert.equal(groups[1].children!.length, 3)
  // 容需量与功率因素不进分组（各自独立成表）
  assert.ok(BILL_MENUS.some((m) => m.key === 'capacity' && !m.group))
  assert.ok(BILL_MENUS.some((m) => m.key === 'pf-adjust' && !m.group))
})

test('menuLabelOf：能取到中文标签，未知 key 原样返回', () => {
  assert.equal(menuLabelOf('industrial-market'), '市场化购电费')
  assert.equal(menuLabelOf('residential-gov'), '政府性基金及附加')
  assert.equal(menuLabelOf('unknown-key'), 'unknown-key', '未知 key 原样返回便于排障')
})

test('feeMenuOf：功率因素调整是独立明细列表，不算费用子表', () => {
  assert.equal(feeMenuOf('industrial-market'), true)
  assert.equal(feeMenuOf('pf-adjust'), false, 'pf-adjust 不进费用子表渲染分支')
  assert.equal(feeMenuOf('overview'), false)
})

// ---- 费用归属判定 ----

test('FEE_MENU_MAP：工商业与居民的「政府性基金」按电量数量级区分', () => {
  const bigQty = { category: '政府性基金及附加', qty: 126600 }
  const smallQty = { category: '政府性基金及附加', qty: 5000 }
  assert.equal(FEE_MENU_MAP['industrial-gov'](bigQty), true)
  assert.equal(FEE_MENU_MAP['residential-gov'](bigQty), false)
  assert.equal(FEE_MENU_MAP['industrial-gov'](smallQty), false)
  assert.equal(FEE_MENU_MAP['residential-gov'](smallQty), true)
})

test('FEE_MENU_MAP：功率因数条目不得混进政府性基金表', () => {
  // 功率因数调整电费在票面上也常归到「政府性基金」类别下，
  // 不排除的话它会在两张 gov 表里重复出现（金额被算两遍）。
  const pf = { category: '政府性基金及附加', name: '功率因数调整电费', qty: 126600 }
  assert.equal(FEE_MENU_MAP['industrial-gov'](pf), false)
  assert.equal(FEE_MENU_MAP['residential-gov'](pf), false)
  assert.equal(FEE_MENU_MAP['pf-adjust'](pf), true)
})

// ---- 行配置与匹配 ----

test('parseRowKey：只拆半角括号 + 白名单时段，其它一律当整体名称', () => {
  assert.deepEqual(parseRowKey('零售交易电费(尖峰)'), { name: '零售交易电费', period: '尖峰' })
  // 「(含税)」不是时段 → 整体当名称，否则会和「零售交易电费」匹配不上
  assert.deepEqual(parseRowKey('零售交易电费(含税)'), { name: '零售交易电费(含税)', period: '' })
  assert.deepEqual(parseRowKey(''), { name: '', period: '' })
  // **已知边界**：全角括号不拆（正则只认 `(`）。配置里若写全角括号，
  // 该行会退化为「整体名称匹配」，与同名但带时段的行对不上。
  // 这是既有行为，不是本次重构引入的；要改必须同时改配置录入端的归一。
  assert.deepEqual(parseRowKey('零售交易电费（峰）'), { name: '零售交易电费（峰）', period: '' })
})

test('normRowName：全角括号归一', () => {
  assert.equal(normRowName('正向有功（总）'), '正向有功(总)')
  assert.equal(normRowName('  正向有功(总)  '), '正向有功(总)')
})

test('matchRow：精确或双向包含；不做全半角归一', () => {
  const cfg = { field_key: '正向有功(总)', label: '总电量' }
  assert.equal(matchRow({ meter_type: '正向有功(总)' }, cfg), true, '精确命中')
  assert.equal(matchRow({ project: '正向有功(总)' }, cfg), true, 'project 字段也参与匹配')
  assert.equal(matchRow({ meter_type: '总电量' }, cfg), true, 'label 命中')
  // 双向包含：提取结果带后缀（如「正向有功(总)合计」）也能匹配上
  assert.equal(matchRow({ meter_type: '正向有功(总)合计' }, cfg), true)
  assert.equal(matchRow({ meter_type: '正向有功(尖峰)' }, cfg), false)
  assert.equal(matchRow({}, cfg), false, '没有类型字段时不匹配')
  // **已知边界**：全角括号不归一 → 全角写法匹配不上。
  // 归一的职责在配置录入端（subs.field_key 统一半角），这里保持零分配实现。
  assert.equal(matchRow({ meter_type: '正向有功（总）' }, cfg), false, '全角括号不归一是既有行为')
})

test('rowConfigsOf：按 sort_order 升序且剔除已删除', () => {
  const all = [
    { field_key: 'b', label: 'B', group: 'market-rows', sort_order: 2 },
    { field_key: 'a', label: 'A', group: 'market-rows', sort_order: 1 },
    { field_key: 'c', label: 'C', group: 'market-rows', sort_order: 3, deleted_at: 'x' },
    { field_key: 'd', label: 'D', group: 'trans-rows', sort_order: 1 },
  ]
  assert.deepEqual(rowConfigsOf(all, 'market-rows').map((c) => c.field_key), ['a', 'b'])
})

// ---- 费用行筛选（核心口径）----

test('feeRowsOf：无行配置时原样返回提取结果', () => {
  const items = [
    { name: '零售交易电费', category: '市场化购电', fee: 100 },
    { name: '偏差电费', category: '市场化购电', fee: 5 },
  ]
  assert.deepEqual(feeRowsOf(items, 'industrial-market', []), items)
})

test('feeRowsOf：有配置时按配置过滤 + 排序 + 改名', () => {
  const items = [
    { name: '零售交易电费', category: '市场化购电', fee: 100 },
    { name: '偏差电费', category: '市场化购电', fee: 5 },
    { name: '未配置项', category: '市场化购电', fee: 999 },
  ]
  const cfgs = [
    { field_key: '偏差电费', label: '偏差调整', default_visible: true, sort_order: 1 },
    { field_key: '零售交易电费', label: '零售交易', default_visible: true, sort_order: 2 },
  ]
  const rows = feeRowsOf(items, 'industrial-market', cfgs)
  assert.deepEqual(rows.map((r: any) => r.name), ['偏差调整', '零售交易'], '按配置顺序出场并改名')
  assert.equal(rows.length, 2, '未配置的行必须隐藏')
})

test('feeRowsOf：同名多行优先取有金额的那行（真机少算 31520.93 的根因）', () => {
  const items = [
    // 合并单元格解析出的「全 0 影子行」排在前
    { name: '偏差回收返还', category: '市场化购电', fee: 0, qty: 0 },
    { name: '偏差回收返还', category: '市场化购电', fee: -31520.93, qty: 0 },
  ]
  const cfgs = [{ field_key: '偏差回收返还', label: '偏差回收返还', default_visible: true }]
  const rows = feeRowsOf(items, 'industrial-market', cfgs)
  assert.equal(rows.length, 1)
  assert.equal(rows[0].fee, -31520.93, '必须取有金额的那行，不能取排在前面的 0 影子行')
})

test('feeRowsOf：分时子项按 名称(时段) 精确配对', () => {
  const items = [
    { name: '零售交易电费', period: '尖峰', category: '市场化购电', fee: 1 },
    { name: '零售交易电费', period: '峰', category: '市场化购电', fee: 2 },
    { name: '零售交易电费', period: '平', category: '市场化购电', fee: 3 },
    { name: '零售交易电费', period: '谷', category: '市场化购电', fee: 4 },
  ]
  const cfgs = [
    { field_key: '零售交易电费(谷)', label: '零售-谷', default_visible: true, sort_order: 1 },
    { field_key: '零售交易电费(尖峰)', label: '零售-尖', default_visible: true, sort_order: 2 },
  ]
  const rows = feeRowsOf(items, 'industrial-market', cfgs)
  assert.deepEqual(rows.map((r: any) => [r.name, r.fee]), [['零售-谷', 4], ['零售-尖', 1]])
})

test('feeRowsOf：配置里隐藏的行不出现', () => {
  const items = [{ name: '偏差电费', category: '市场化购电', fee: 5 }]
  const cfgs = [{ field_key: '偏差电费', label: '偏差', default_visible: false }]
  // 全部隐藏 → 走「显示全部」兜底（否则整表空掉）
  assert.equal(feeRowsOf(items, 'industrial-market', cfgs).length, 1)
})

test('feeRowsOf：未知菜单 key 返回空数组', () => {
  assert.deepEqual(feeRowsOf([{ name: 'x', fee: 1 }], 'overview', []), [])
})

test('feeSubtotalOf：按分累加，不留浮点尾巴', () => {
  const items = [
    { name: 'a', category: '市场化购电', fee: 0.1 },
    { name: 'b', category: '市场化购电', fee: 0.2 },
  ]
  const cfgs = [
    { field_key: 'a', label: 'a', default_visible: true, sort_order: 1 },
    { field_key: 'b', label: 'b', default_visible: true, sort_order: 2 },
  ]
  assert.equal(feeSubtotalOf(items, 'industrial-market', cfgs), 0.3, '0.1+0.2 必须等于 0.3')
  assert.equal(feeSubtotalOf([], 'industrial-market', []), 0, '空表小计是 0 不是 undefined')
})

test('ROW_GROUP_OF_MENU：有行配置的表必须能查到分组；无配置的原样展示', () => {
  // 6 张费用子表里有 6 个分组（industrial-line 无行配置 —— 上网环节线损费
  // 只有一行，不需要按行配置增减；feeRowsOf 会原样返回提取结果）。
  // 键是**菜单 key**（不是分组名）：菜单 → 分组，再由分组取行配置
  assert.deepEqual(Object.keys(ROW_GROUP_OF_MENU).sort(), [
    'industrial-gov', 'industrial-market', 'industrial-sys', 'industrial-trans',
    'residential-catalog', 'residential-gov',
  ])
  assert.equal(ROW_GROUP_OF_MENU['industrial-market'], 'market-rows')
  // 没有分组的表也不能崩：feeRowsOf 无 cfgs → 原样返回
  const items = [{ name: '上网环节线损费', category: '上网环节线损', fee: 12 }]
  assert.deepEqual(feeRowsOf(items, 'industrial-line', []), items)
})

// ---- 电量明细 ----

test('buildMeterRows：无配置时按 META_ORDER 排序', () => {
  const form = {
    meter_readings: [
      { meter_type: '正向无功（总）', bill_kwh: 0 },
      { meter_type: '正向有功（总）', bill_kwh: 100 },
      { meter_type: '正向有功（峰）', bill_kwh: 30 },
    ],
  }
  assert.deepEqual(buildMeterRows(form, []).map((r) => r.meter_type), [
    '正向有功（总）', '正向有功（峰）', '正向无功（总）',
  ])
})

test('buildMeterRows：旧数据无 meter_readings 时从顶层字段兜底', () => {
  const rows = buildMeterRows({ total_kwh: 100, peak_kwh: 30 }, [])
  assert.equal(rows.length, 6, '兜底也要出 6 行（含正向无功）')
  assert.equal(rows[0].bill_kwh, 100)
})

test('buildMeterRows：有配置时匹配不到的行给空串占位（不是 0）', () => {
  const cfgs = [
    { field_key: '正向有功(总)', label: '总', default_visible: true, sort_order: 1 },
    { field_key: '不存在的行', label: '缺', default_visible: true, sort_order: 2 },
  ]
  const rows = buildMeterRows({ meter_readings: [{ meter_type: '正向有功（总）', bill_kwh: 100 }] }, cfgs)
  assert.equal(rows[1].bill_kwh, '', '缺数据必须是空串 —— 「没有」与「0 度」要能分辨')
})

test('META_ORDER：正向无功在最后（不参与计费但要露出来对数）', () => {
  assert.equal(META_ORDER[META_ORDER.length - 1], '正向无功（总）')
})

// ---- 居民电量明细 ----

test('buildResidentMeterRows：无配置只输出一行「定比0.015」', () => {
  const rows = buildResidentMeterRows({ total_kwh: 1000, residential_readings: [] }, [])
  assert.equal(rows.length, 1, '居民只有一个定比行，不是每户一行')
  assert.equal(rows[0].meter_type, '定比0.015')
  assert.equal(rows[0].bill_kwh, 15, '1000 × 0.015 = 15')
})

test('buildResidentMeterRows：定比 + 加减，按分取整', () => {
  const rows = buildResidentMeterRows(
    { total_kwh: 1000, residential_readings: [{ ratio: 0.02, adjust: 3.335 }] },
    [],
  )
  assert.equal(rows[0].bill_kwh, 23.34, '1000×0.02 + 3.335 = 23.335 → 23.34')
})

test('buildResidentMeterRows：列表计费电量恒等于账上值（基准 + 加减）', () => {
  // 第 37 轮口径变更：旧实现 `bill_kwh_manual ? bill_kwh : total*ratio+adjust`
  // 会让 2026-10 显示 9,358.98（存的是基准值）而费用行是 9,354，两个数对不上。
  // 现在一律按「基准 + 加减」算 —— 有费用行时基准由费用行反推，
  // 因此列表显示恒等于账上计费电量。
  const rows = buildResidentMeterRows(
    { total_kwh: 1000, residential_readings: [{ ratio: 0.015, adjust: -5, bill_kwh_manual: true, bill_kwh: 99 }] },
    [],
  )
  // 无费用行 → 基准 = total*ratio = 15 → 计费 = 15 + (-5) = 10
  assert.equal(rows[0].bill_kwh, 10, '必须按 基准 + 加减 计算，不能直接用历史 bill_kwh')

  // 有费用行时以账上 qty 为准（反推基准后再加回落，值不变）
  const withFee = buildResidentMeterRows(
    {
      total_kwh: 623932,
      residential_readings: [{ ratio: 0.015, adjust: -4.98, bill_kwh: 9358.98, bill_kwh_manual: true }],
      fee_items: [{ category: '(5)目录电费', qty: 9354, rate: 0.500306, fee: 4679.86 }],
    },
    [],
  )
  assert.equal(withFee[0].bill_kwh, 9354, '显示值必须与账上计费电量一致')
})

test('buildResidentMeterRows：有配置按索引配对', () => {
  const cfgs = [
    { field_key: 'a', label: '一号表', default_visible: true },
    { field_key: 'b', label: '二号表', default_visible: true },
  ]
  const rows = buildResidentMeterRows(
    { total_kwh: 1000, residential_readings: [{ ratio: 0.015 }, { ratio: 0.02 }] },
    cfgs,
  )
  assert.deepEqual(rows.map((r) => r.meter_type), ['一号表', '二号表'])
  assert.equal(rows[0].bill_kwh, 15)
  assert.equal(rows[1].bill_kwh, 20)
})

// ---- 上期数据与分时占比 ----

test('pickPrevEnergy：取严格早于本期的最近一期', () => {
  const records = [
    { item: { bill_period_start: '2026-09-01', total_kwh: 900 } },
    { item: { bill_period_start: '2026-08-01', total_kwh: 800 } },
    { item: { bill_period_start: '2026-10-01', total_kwh: 1000 } },
  ]
  const p = pickPrevEnergy(records, '2026-10-01')
  assert.equal(p.exists, true)
  assert.equal(p.period, '2026-09', '取最近的上一期')
  assert.equal(p.total, 900)
})

test('pickPrevEnergy：没有上一期时 exists=false 且数值归零', () => {
  const p = pickPrevEnergy([{ item: { bill_period_start: '2026-10-01', total_kwh: 1000 } }], '2026-10-01')
  assert.equal(p.exists, false)
  assert.equal(p.total, 0)
  assert.equal(p.fee, 0)
})

test('pickPrevEnergy：power_factor / avg_price 空值给 0（不是 NaN）', () => {
  const p = pickPrevEnergy(
    [{ item: { bill_period_start: '2026-09-01', power_factor: null, avg_price: '' } }],
    '2026-10-01',
  )
  assert.equal(p.power_factor, 0)
  assert.equal(p.avg_price, 0)
})

test('buildEnergyCompareRows：平段取余量，四段合计恰好 100%', () => {
  const form = { total_kwh: 1000, deep_peak_kwh: 123, peak_kwh: 301, flat_kwh: 445, valley_kwh: 131 }
  const prev = pickPrevEnergy([], '2026-10-01')
  const rows = buildEnergyCompareRows(form, prev)
  const sum = rows.reduce((s, r) => s + r.curPct, 0)
  assert.ok(Math.abs(sum - 100) < 1e-9, `四段占比合计必须恰好 100%，实际 ${sum}`)
  assert.equal(rows.find((r) => r.key === 'flat')!.curPct, 100 - 12.3 - 30.1 - 13.1)
})

test('buildEnergyCompareRows：无上期时 delta 为 0 且 prevPct 全 0', () => {
  const form = { total_kwh: 1000, deep_peak_kwh: 100, peak_kwh: 200, flat_kwh: 500, valley_kwh: 200 }
  const rows = buildEnergyCompareRows(form, pickPrevEnergy([], ''))
  assert.ok(rows.every((r) => r.delta === 0))
  assert.ok(rows.every((r) => r.prevPct === 0))
})

test('buildEnergyCompareRows：总电量为 0 时占比全 0，不产生 NaN', () => {
  const rows = buildEnergyCompareRows({ total_kwh: 0 }, pickPrevEnergy([], ''))
  assert.ok(rows.every((r) => r.curPct === 0), '除零必须收敛成 0')
  assert.ok(rows.every((r) => Number.isFinite(r.curPct)))
})

// ---- 编辑抽屉字段 ----

test('CAPACITY_FIELDS / PF_FIELDS：字段数固定且都是 number', () => {
  assert.equal(CAPACITY_FIELDS.length, 8)
  assert.equal(PF_FIELDS.length, 8)
  // 容需量表只有「项目」是文本，其余全是数值 —— 数值字段错标 text 会导致
  // 抽屉里出现可输入任意字符的框，保存后后端解析失败。
  assert.equal(CAPACITY_FIELDS.filter((f) => f.type === 'text').length, 0)
  assert.equal(PF_FIELDS.filter((f) => f.type === 'text').length, 1)
  assert.equal(PF_FIELDS[0].key, 'project')
})

// ---------------------------------------------------------------------------
// 第 33 轮：电量明细合计 / 四段计费电量 / 用能分析占比分母
// ---------------------------------------------------------------------------

test('meterTotalOf：只累加四段计费电量，总表与无功不参与', () => {
  const rows = [
    { meter_type: '正向有功（总）', bill_kwh: 0 },
    { meter_type: '正向有功（尖峰）', bill_kwh: 34105 },
    { meter_type: '正向有功（峰）', bill_kwh: 133236 },
    { meter_type: '正向有功（平）', bill_kwh: 196797 },
    { meter_type: '正向有功（谷）', bill_kwh: 250440 },
    // 即便历史数据里总表/无功带值，也不得累加（否则 1,401,690）
    { meter_type: '正向无功（总）', bill_kwh: 154410 },
  ]
  assert.equal(meterTotalOf(rows), 614578)
  assert.equal(meterTotalOf([]), 0)
})

test('touBillKwhOf：优先取电量明细的计费电量，缺失才回落顶层字段', () => {
  const form = {
    deep_peak_kwh: 35490, peak_kwh: 35490, flat_kwh: 199560, valley_kwh: 250440,
    meter_readings: [
      { meter_type: '正向有功（尖峰）', bill_kwh: 34105 },
      { meter_type: '正向有功（峰）', bill_kwh: 133236 },
      { meter_type: '正向有功（平）', bill_kwh: 196797 },
      { meter_type: '正向有功（谷）', bill_kwh: 250440 },
    ],
  }
  const tou = touBillKwhOf(form)
  assert.deepEqual(tou, { deep: 34105, peak: 133236, flat: 196797, valley: 250440 })
  // 没有 meter_readings 的老数据回落顶层字段，不返回 0
  assert.deepEqual(touBillKwhOf({ deep_peak_kwh: 1, peak_kwh: 2, flat_kwh: 3, valley_kwh: 4 }),
    { deep: 1, peak: 2, flat: 3, valley: 4 })
})

test('buildEnergyCompareRows：占比分母是工商业四段之和（不是账单总电量）', () => {
  const form = {
    total_kwh: 623932,
    meter_readings: [
      { meter_type: '正向有功（尖峰）', bill_kwh: 34105 },
      { meter_type: '正向有功（峰）', bill_kwh: 133236 },
      { meter_type: '正向有功（平）', bill_kwh: 196797 },
      { meter_type: '正向有功（谷）', bill_kwh: 250440 },
    ],
  }
  const rows = buildEnergyCompareRows(form, pickPrevEnergy([], '2026-10-01'))
  assert.deepEqual(rows.map((r) => r.cur), [34105, 133236, 196797, 250440])
  // 票面给的占比：5.55% / 21.68% / 32.02% / 40.75%
  assert.equal(rows[0].curPct, 5.55)
  assert.equal(rows[1].curPct, 21.68)
  assert.equal(rows[2].curPct, 32.02)
  assert.equal(rows[3].curPct, 40.75)
})

// ---------------------------------------------------------------------------
// 第 37 轮：居民电量明细编辑（基准电量 / 实时联动 / 级联居民电费）
// ---------------------------------------------------------------------------
//
// 数据取自库内 2026-10 真实账单：
//   total_kwh=623,932，定比 0.015，加减 -4.98，计费电量 9,354
//   居民目录电费 9,354 × 0.500306 = 4,679.86
//   政府性基金及附加-居民 5 项，qty 均 9,354

const residentBill202610 = {
  total_kwh: 623932,
  residential_readings: [{ kwh: 9354, ratio: 0, adjust: -4.98, bill_kwh: 9358.98, bill_kwh_manual: true }],
  fee_items: [
    { category: '(5)目录电费', name: '基础电费', period: '平', qty: 9354, rate: 0.500306, fee: 4679.86 },
    { category: '(6)政府性基金及附加-居民', name: '农网还贷', period: '平', qty: 9354, rate: 0.02, fee: 187.08 },
    { category: '(6)政府性基金及附加-居民', name: '可再生能源附加', period: '平', qty: 9354, rate: 0.001, fee: 9.35 },
    { category: '(6)政府性基金及附加-居民', name: '国家重大水利工程建设基金', period: '平', qty: 9354, rate: 0.001969, fee: 18.42 },
    { category: '(6)政府性基金及附加-居民', name: '库区移民基金', period: '平', qty: 9354, rate: 0.006225, fee: 58.23 },
    { category: '(6)政府性基金及附加-居民', name: '小型水库移民后期扶持资金(地方)', period: '平', qty: 9354, rate: 0.0005, fee: 4.68 },
    // 工商业同名基金：绝不能被级联波及
    { category: '(6)政府性基金及附加-工商业', name: '农网还贷', period: '平', qty: 614578, rate: 0.02, fee: 12291.56 },
  ],
}

test('居民基准电量 = 费用行计费电量 − 加减（2026-10 得 9358.98）', () => {
  const base = residentBaseElecOf(residentBill202610, 0)
  assert.equal(base, 9358.98, `基准电量应为 9,358.98，got ${base}`)
})

test('居民基准电量：无费用行时退化为 本期电量 × 定比', () => {
  const base = residentBaseElecOf({ total_kwh: 623932, residential_readings: [{ ratio: 0.015, adjust: 0 }] }, 0)
  assert.equal(base, Math.round(623932 * 0.015 * 100) / 100, '应等于 623932 × 0.015 = 9358.98')
})

test('输入加减 -4.98 → 计费电量实时算成 9354', () => {
  const base = residentBaseElecOf(residentBill202610, 0)
  assert.equal(recalcResidentBillKwh(base, -4.98), 9354)
  assert.equal(recalcResidentBillKwh(base, 0), 9358.98, '加减归零应回到基准')
  assert.equal(recalcResidentBillKwh(base, -8.98), 9350, '支持任意加减')
})

test('级联居民电费：目录电费 + 5 项基金 qty 全部同步，金额按 qty×rate 重算', () => {
  const { items, changed } = syncResidentFeeQty(residentBill202610.fee_items, 9354)
  assert.equal(changed, false, '同一电量重复同步不应标记为变化（幂等）')

  const { items: next, changed: c2 } = syncResidentFeeQty(residentBill202610.fee_items, 9000)
  assert.equal(c2, true, '电量变化必须报告 changed')
  const res = next.filter((it: any) => it.category.includes('居民') || it.category.includes('目录电费'))
  assert.equal(res.length, 6, '居民侧应有 6 行（目录 1 + 基金 5）')
  for (const it of res) {
    assert.equal(it.qty, 9000, `${it.name} 计费电量未同步`)
    assert.equal(it.fee, Math.round(9000 * it.rate * 100) / 100, `${it.name} 金额未按 qty×rate 重算`)
    assert.equal(it.fee_amount, it.fee, 'fee_amount 必须同步，否则双列校验误报异常')
  }
  // 工商业同名基金不能被改
  const ind = next.find((it: any) => it.category.includes('工商业'))
  assert.equal(ind.qty, 614578, '工商业基金电量被误改')
  assert.equal(ind.fee, 12291.56, '工商业基金金额被误改')
})

test('isResidentFeeRow：只认目录电费与政府性基金居民', () => {
  assert.equal(isResidentFeeRow({ category: '(5)目录电费' }), true)
  assert.equal(isResidentFeeRow({ category: '(6)政府性基金及附加-居民' }), true)
  assert.equal(isResidentFeeRow({ category: '(6)政府性基金及附加-工商业' }), false)
  assert.equal(isResidentFeeRow({ category: '(1)市场化购电费' }), false)
})

// ---------------------------------------------------------------------------
// 第 37 轮回归：保存时「计费电量 = 基准 + 加减」，第二个参数必须是加减
// ---------------------------------------------------------------------------
//
// 真机事故：保存路径误把 `bill_kwh`（本身已是「基准 + 加减」的结果）
// 当成 adjust 传进 recalcResidentBillKwh，于是算成 基准 + 计费电量：
//   9,358.98 + 9,354 = 18,712.98（翻倍，且级联把居民电费全部带偏）。
test('保存口径：计费电量 = 基准 + 加减，不得用「基准 + 计费电量」', () => {
  const base = residentBaseElecOf(residentBill202610, 0)   // 9358.98
  // 正确：传加减
  assert.equal(recalcResidentBillKwh(base, -4.98), 9354)
  // 错误写法（传计费电量）会得出翻倍值 —— 这正是要防的回归
  assert.equal(recalcResidentBillKwh(base, 9354), 18712.98)
  assert.notEqual(recalcResidentBillKwh(base, 9354), 9354, '若两者相等，本测试就失去守护意义')
})
