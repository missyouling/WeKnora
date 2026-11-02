/**
 * 第 18 轮：逐列汇总 / 打印空行裁剪 / 合计应付拆分 / 行点击抽屉下线。
 *
 * 这里的用例全部是**行为级**（纯函数）或源码级（必须接上的关键调用），
 * 目标是把四类口径钉死，防止下次改页面时又各写一套。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  isSummableColumn,
  summaryCellOf,
  summaryLabelOf,
  summaryHeadSpan,
  buildFootRow,
  footSpanOf,
  fmtSumAmount,
  type SummaryColumn,
} from './listSummary'
import { isEmptyDetailRow, pruneEmptyTables } from './useCatalogPdf'
import {
  TENANT_ELEC_TOTAL_FIELD,
  TENANT_WATER_TOTAL_FIELD,
  TENANT_TOTAL_FIELDS,
  LEGACY_TOTAL_FIELDS,
  mergeTenantGroupFields,
  buildTenantGroupFields,
  tenantGroupNeedsSync,
} from './billingFieldGroups'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

// ---- ① 逐列汇总：能加 / 不能加 ----

test('可累加列：电量/电费/水费/气费/金额都求和', () => {
  const cols: SummaryColumn[] = [
    { key: 'total_kwh', label: '本期电量', fieldType: 'number' },
    { key: 'total_amount', label: '本期电费', fieldType: 'amount' },
    { key: 'water_ind_fee', label: '工业水费', fieldType: 'amount' },
    { key: 'gas_fee', label: '气费', fieldType: 'amount' },
    { key: 'elec_total_fee', label: '电费合计', fieldType: 'amount' },
    { key: 'water_total_fee', label: '水费合计', fieldType: 'amount' },
  ]
  for (const c of cols) assert.equal(isSummableColumn(c), true, `${c.label} 必须可累加`)
})

test('不可累加列：单价/均价/比例/倍率/起止度/示数/环比/日期/文本 → 空白', () => {
  const cols: SummaryColumn[] = [
    { key: 'unit_price', label: '单价', fieldType: 'number' },
    { key: 'ind_price', label: '工业均价', fieldType: 'amount' },
    { key: 'avg_price', label: '平均电价', fieldType: 'number' },
    { key: 'power_factor', label: '功率因素', fieldType: 'number' },
    { key: 'ratio', label: '分摊比例', fieldType: 'number' },
    { key: 'meter_ratio', label: '倍率', fieldType: 'text' },
    { key: 'start_reading', label: '起度', fieldType: 'number' },
    { key: 'end_reading', label: '止度', fieldType: 'number' },
    { key: 'prev', label: '上期示数', fieldType: 'number' },
    { key: 'curr', label: '本期示数', fieldType: 'number' },
    { key: 'mom_change', label: '环比', fieldType: 'text' },
    { key: 'bill_period', label: '账单周期', fieldType: 'text' },
    { key: 'remark', label: '备注', fieldType: 'text' },
  ]
  for (const c of cols) {
    assert.equal(isSummableColumn(c), false, `${c.label} 不得累加`)
    // 第 21 轮：不可累加列在合计行里**留空**，不写 `—`
    // （`—` 会被读成「这列没数据」，实际只是「不该相加」）
    assert.equal(summaryCellOf(c, [{ [c.key]: 12 }, { [c.key]: 34 }]), '', `${c.label} 表尾必须留空`)
  }
})

test('金额按「分」累加：0.1 + 0.2 = 0.30（无浮点残留）', () => {
  const col: SummaryColumn = { key: 'amount', label: '金额', fieldType: 'amount' }
  const rows = [{ amount: 0.1 }, { amount: 0.2 }]
  assert.equal(summaryCellOf(col, rows), '0.30')
  // 大额也不出现 53441.77999999996
  const big = [{ amount: 53441.78 }, { amount: 0.0000001 }]
  assert.equal(summaryCellOf(col, big), '53,441.78')
})

test('带千分位/货币符号的字符串也能正确求和', () => {
  const col: SummaryColumn = { key: 'amount', label: '金额', fieldType: 'amount' }
  assert.equal(summaryCellOf(col, [{ amount: '1,234.50' }, { amount: '¥ 765.50' }]), '2,000.00')
})

test('数量列：整数不带小数点，两位小数保留', () => {
  const col: SummaryColumn = { key: 'usage', label: '用电量', fieldType: 'number' }
  assert.equal(summaryCellOf(col, [{ usage: 100 }, { usage: 200 }]), '300')
  assert.equal(summaryCellOf(col, [{ usage: 1.25 }, { usage: 2.5 }]), '3.75')
})

test('首列文案与合并跨度：合计（共 N 条）吃掉复选框+序号+首个标识列', () => {
  assert.equal(summaryLabelOf(16), '合计（共 16 条）')
  // 模拟真实列序：[复选框, 序号, 业务列…] —— 前两列是页面固定插进去的
  const cols: SummaryColumn[] = [
    { key: 'row-select', label: '', fieldType: 'text' },
    { key: 'serial-number', label: '', fieldType: 'text' },
    { key: 'bill_period', label: '账单周期', fieldType: 'text' },
    { key: 'total_kwh', label: '本期电量', fieldType: 'number' },
  ]
  assert.equal(summaryHeadSpan(cols), 3, '首个业务列不可加 → 合并 3 列')
  const numFirst: SummaryColumn[] = [
    { key: 'row-select', label: '', fieldType: 'text' },
    { key: 'serial-number', label: '', fieldType: 'text' },
    { key: 'total_kwh', label: '本期电量', fieldType: 'number' },
  ]
  assert.equal(summaryHeadSpan(numFirst), 2, '首个业务列可加 → 只合并 2 列，不吞掉它的合计')
  const span = footSpanOf(cols)
  assert.deepEqual(span({ colIndex: 0 }), { colspan: 3, rowspan: 1 })
  assert.deepEqual(span({ colIndex: 3 }), { colspan: 1, rowspan: 1 })
})

test('buildFootRow：每个可见列都有单元格值，首列是合计文案', () => {
  const cols: SummaryColumn[] = [
    { key: 'bill_period', label: '账单周期', fieldType: 'text' },
    { key: 'total_kwh', label: '本期电量', fieldType: 'number' },
    { key: 'ind_price', label: '工业均价', fieldType: 'amount' },
  ]
  const rows = [{ bill_period: '2026-09', total_kwh: 596168, ind_price: 0.7071 }]
  const foot = buildFootRow(cols, rows)
  assert.equal(foot['row-select'], '合计（共 1 条）')
  // 不可累加的列留空（第 21 轮），可累加列照常给合计值
  assert.equal(foot.bill_period, '')
  assert.equal(foot.total_kwh, '596,168')
  assert.equal(foot.ind_price, '')
})

test('表尾金额格式：千分位 + 两位小数', () => {
  assert.equal(fmtSumAmount(103018.66), '103,018.66')
  assert.equal(fmtSumAmount(0.5), '0.50')
})

// ---- ② 打印空行裁剪 ----

const FEE_COLS = [
  { label: '费用组成', weight: 3.4 },
  { label: '时段', weight: 1, align: 'center' as const },
  { label: '计费数量', weight: 1.3, align: 'right' as const },
  { label: '单价/标准', weight: 1.2, align: 'right' as const },
  { label: '金额(元)', weight: 1.4, align: 'right' as const },
]
/** 应收汇总这种「结论表」的列：只有金额列右对齐 */
const SUM_COLS = [
  { label: '费用项目', weight: 3 },
  { label: '使用单位', weight: 2 },
  { label: '金额(元)', weight: 1.6, align: 'right' as const },
]

test('空明细行判定：金额 0 且数量 0/— → 隐藏', () => {
  assert.equal(isEmptyDetailRow(FEE_COLS, ['偏差电费', '尖峰', '0', '—', '0.00']), true)
  assert.equal(isEmptyDetailRow(FEE_COLS, ['绿电交易电费（省间）', '平', '—', '—', '0.00']), true)
  assert.equal(isEmptyDetailRow(FEE_COLS, ['中长期超额申报回收费用', '—', '—', '—', '0.00']), true)
})

test('有金额或有数量的行必须保留', () => {
  assert.equal(isEmptyDetailRow(FEE_COLS, ['零售交易电费', '尖峰', '5085.43', '0.436', '2217.25']), false)
  // 只有数量没有金额（如 0 元补贴项）也要保留：它确实发生了电量
  assert.equal(isEmptyDetailRow(FEE_COLS, ['零售交易电费', '峰', '35579.69', '0.436', '0.00']), false)
})

test('负数金额（返还/冲减）绝不能被当成空行', () => {
  assert.equal(isEmptyDetailRow(FEE_COLS, ['上网环节线损代理采购损益', '平', '124701.81', '-0.000971', '-121.09']), false)
})

test('pruneEmptyTables：整表全 0 → 连标题一起隐藏；结论表 alwaysShow 保留', () => {
  const tables = [
    { title: '偏差电费', columns: FEE_COLS, rows: [['偏差电费', '尖峰', '0', '—', '0.00']] },
    { title: '零售交易电费', columns: FEE_COLS, rows: [['零售交易电费', '尖峰', '5085.43', '0.436', '2217.25']] },
    {
      title: '应收费用汇总',
      columns: SUM_COLS,
      rows: [['工商业电费（分摊后）', '持睿汽车', '0.00']],
      alwaysShow: true,
    },
  ]
  const out = pruneEmptyTables(tables)
  assert.deepEqual(out.map((t) => t.title), ['零售交易电费', '应收费用汇总'])
})

test('pruneEmptyTables：同一张表里 0 行被逐行剔除，非 0 行保留', () => {
  const t = {
    title: '市场化购电费',
    columns: FEE_COLS,
    rows: [
      ['偏差电费', '尖峰', '0', '—', '0.00'],
      ['零售交易电费', '尖峰', '5085.43', '0.436', '2217.25'],
      ['绿电交易电费（省外）', '平', '—', '—', '0.00'],
    ],
  }
  const out = pruneEmptyTables([t])
  assert.equal(out.length, 1)
  assert.equal(out[0].rows.length, 1)
  assert.equal(out[0].rows[0][0], '零售交易电费')
})

test('没有金额/数量列的表一律保留（避免误删纯文本清单）', () => {
  const t = {
    title: '备注',
    columns: [{ label: '项目' }, { label: '说明' }],
    rows: [['说明', '本期无异常']],
  }
  assert.equal(pruneEmptyTables([t]).length, 1)
})

test('打印链路接入：generateStatementPdf 排版前裁剪，费用清单/市电/光伏结论表标 alwaysShow', () => {
  const pdf = read('useCatalogPdf.ts')
  assert.match(pdf, /export function pruneEmptyTables/)
  assert.match(pdf, /export function isEmptyDetailRow/)
  assert.match(pdf, /const doc: StatementDoc = \{ \.\.\.source, tables: pruneEmptyTables\(source\.tables\) \}/,
    '必须在排版前裁剪（排版中裁会留下空白页）')
  const billing = read('TenantBilling.vue')
  assert.match(billing, /alwaysShow: true/, '应收汇总必须保留')
  const energy = read('energyBillStatements.ts')
  assert.equal((energy.match(/alwaysShow: true/g) || []).length, 2, '市电「账单合计」与光伏「结算汇总」都必须保留')
})

// ---- ③ 合计应付 → 电费合计 / 水费合计 ----

test('字段常量：新两条收口列存在，旧合并列列入废弃清单', () => {
  assert.equal(TENANT_ELEC_TOTAL_FIELD, 'elec_total_fee')
  assert.equal(TENANT_WATER_TOTAL_FIELD, 'water_total_fee')
  assert.deepEqual([...TENANT_TOTAL_FIELDS], ['elec_total_fee', 'water_total_fee'])
  assert.deepEqual([...LEGACY_TOTAL_FIELDS], ['item_total'])
})

test('buildTenantGroupFields：末尾是电费合计 + 水费合计（顺序固定）', () => {
  const names = buildTenantGroupFields({ id: 't1', name: '持睿汽车' }, []).map((f) => f.name)
  assert.deepEqual(names.slice(-2), [TENANT_ELEC_TOTAL_FIELD, TENANT_WATER_TOTAL_FIELD])
})

test('mergeTenantGroupFields：旧「合计应付」被丢弃且幂等（重复合并不产生重复列）', () => {
  const tenant = { id: 't1', name: '持睿汽车' }
  const legacy = [
    { name: 'month', label: '账单周期', enabled: true, isDefault: true, dataType: 'text' as const, width: 0 },
    { name: 'item_total', label: '合计应付', enabled: true, isDefault: true, dataType: 'number' as const, width: 0 },
  ]
  const once = mergeTenantGroupFields(legacy, tenant, [])
  const twice = mergeTenantGroupFields(once, tenant, [])
  assert.deepEqual(twice.map((f) => f.name), once.map((f) => f.name), '二次合并必须完全幂等')
  assert.equal(once.some((f) => f.name === 'item_total'), false)
  assert.deepEqual(once.slice(-2).map((f) => f.name), [TENANT_ELEC_TOTAL_FIELD, TENANT_WATER_TOTAL_FIELD])
})

test('tenantGroupNeedsSync：存量只有旧合计列 → 必须判定为需要同步', () => {
  const saved = [
    { name: 'month', label: '账单周期', enabled: true, isDefault: true, dataType: 'text' as const, width: 0 },
    { name: 'item_total', label: '合计应付', enabled: true, isDefault: true, dataType: 'number' as const, width: 0 },
  ]
  assert.equal(tenantGroupNeedsSync(saved, []), true)
  // 已经是新两条 → 只要派生字段齐就不再要求同步
  const fresh = mergeTenantGroupFields(saved, { id: 't1', name: '持睿汽车' }, [])
  assert.equal(tenantGroupNeedsSync(fresh, []), false)
})

test('六同步：列表/字段定义/抽屉/打印全部用两条新列，任何一端不得再出现「合计应付」', () => {
  const billing = read('TenantBilling.vue')
  // 列表列定义（COL_DEFS_TENANT）必须有两列，且不再有旧合并列
  assert.match(billing, /key: TENANT_ELEC_TOTAL_FIELD, label: '电费合计'/)
  assert.match(billing, /key: TENANT_WATER_TOTAL_FIELD, label: '水费合计'/)
  assert.ok(!/label: '合计应付'/.test(billing), '不得再有「合计应付」列定义')
  // 取值口径：工业电费 + 宿舍电费 / 工业水费 + 宿舍水费
  assert.match(billing, /const elecTotal = Math\.round\(\(indFee \+ \(dorm\.fee \|\| 0\)\) \* 100\) \/ 100/)
  assert.match(billing, /const waterTotal = Math\.round\(\(\(wInd\.fee \|\| 0\) \+ \(wDorm\.fee \|\| 0\)\) \* 100\) \/ 100/)
  // 浮窗口径文案（与需求原文逐字一致）
  assert.match(billing, /计算方式：工业电费 \(\$\{money2\(indFee\)\}\) \+ 宿舍电费 \(\$\{money2\(dorm\.fee\)\}\)/)
  assert.match(billing, /计算方式：工业水费 \(\$\{money2\(wInd\.fee\)\}\) \+ 宿舍水费 \(\$\{money2\(wDorm\.fee\)\}\)/)
  // 打印目录走可见列（自动带上两条新列）；打印清单的应收汇总按「电+水」分行
  assert.match(billing, /label: isOwnerView\.value \|\| includeGas \? '应收合计' : '电水合计应收'/)
})

// ---- ④ 行点击抽屉下线 ----

test('费用清单：不挂抽屉、不绑行点击、无 openRecord/printRecord 残留', () => {
  const billing = read('TenantBilling.vue')
  assert.ok(!/BusinessSplitDetailDrawer/.test(billing), '不得再挂载 BusinessSplitDetailDrawer')
  assert.ok(!/@row-click=/.test(billing), '不得再绑 @row-click')
  assert.ok(!/const onRowClick/.test(billing), '不得保留 onRowClick')
  assert.ok(!/const openRecord/.test(billing), '不得保留 openRecord')
  assert.ok(!/printRecord/.test(billing), '不得保留抽屉打印入口')
  assert.ok(!/recordVisible|recordRow|recordForm|recordTitle|billingFields/.test(billing),
    '抽屉相关状态必须一并删除')
})

test('费用清单：行光标恢复默认，仅复选框与展开徽标可点', () => {
  const billing = read('TenantBilling.vue')
  assert.match(billing, /\.doc-table :deep\(tbody tr\) \{ cursor: default; \}/)
  assert.match(billing, /\.doc-table :deep\(tbody tr \.t-checkbox\) \{ cursor: pointer; \}/)
  assert.match(billing, /\.doc-table :deep\(tbody tr \.row-expand-toggle\) \{ cursor: pointer; \}/)
})

test('费用清单：保留的交互都还在（勾选 / 展开 / hover 浮窗 / 浮动栏三键）', () => {
  const billing = read('TenantBilling.vue')
  assert.match(billing, /@select-change=/)
  assert.match(billing, /@expand-change=/)
  assert.match(billing, /data-tip/)
  assert.match(billing, />\s*打印清单\s*</)
  assert.match(billing, />\s*打印目录\s*</)
  assert.match(billing, />\s*删除\s*</)
})

// ---- ⑤ 表尾汇总必须真的接到 6 个列表 ----

test('6 个列表都接了 foot-data + 汇总行样式', () => {
  const pages = [
    ['TenantBilling.vue', '费用清单'],
    ['UtilitiesManagement.vue', '市电账单'],
    ['SolarManagement.vue', '光伏账单'],
    ['UtilityMeterTab.vue', '电/水/气核算'],
    ['InvoiceManagement.vue', '发票'],
    ['ContractManagement.vue', '合同'],
  ] as const
  for (const [file, label] of pages) {
    const src = read(file)
    assert.match(src, /:foot-data="footData"/, `${label} 必须接表尾汇总`)
    assert.match(src, /:rowspan-and-colspan-in-footer="footSpan"/, `${label} 必须接首列合并`)
    assert.match(src, /list-summary-footer\.css/, `${label} 必须引入汇总行样式`)
    assert.match(src, /buildFootRow\(/, `${label} 必须用统一汇总实现`)
  }
})
