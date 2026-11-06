import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseFieldSource, resolveSourceValue, resolveTenantItemValue, matchBillFeeItems, billRawAmountOf } from './tenantBillingSources'

const here = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(here, f), 'utf8')

const tenant = read('./TenantBilling.vue')
const panel = read('./TenantSettingsPanel.vue')
const certPanel = read('./FleetCertPanel.vue')
const picker = read('./SourceFieldPicker.vue')
const bsd = read('./BusinessSplitDetailDrawer.vue')
const pdf = read('./useCatalogPdf.ts')

function ctx(overrides: Record<string, any> = {}) {
  return {
    month: '2026-09',
    tenantName: '持睿汽车',
    bill: {
      item: {
        total_kwh: 596168,
        grand_total: 433433.07,
        fee_items: [
          { category: '(1)市场化购电费', name: '峰', fee: 48928.36 },
          { category: '(1)市场化购电费', name: '平', fee: 79877.38 },
          { category: '(5)输配电量电费', name: '电量电费', fee: 76307.97 },
        ],
      },
    },
    solarItems: [],
    elecRows: [],
    waterMeters: [],
    waterReadingMap: new Map<string, any>(),
    gasRows: [],
    ...overrides,
  }
}

test('引用字段：市电账单费用分项按月求和；概况字段直读', () => {
  const ctx1 = ctx()
  assert.equal(
    resolveSourceValue({ category: '市电账单', field: '峰', mode: 'fee_row' }, ctx1),
    48928.36,
  )
  assert.equal(
    resolveSourceValue({ category: '市电账单', field: 'total_kwh', mode: 'field' }, ctx1),
    596168,
  )
  // 账单电费别名：grand_total → total_amount
  assert.equal(
    resolveSourceValue({ category: '市电账单', field: 'total_amount', mode: 'field' }, ctx1),
    433433.07,
  )
})

test('引用字段：电/水/气核算按「月份 + 使用单位」求和，单价取加权', () => {
  const rows = [
    { month: '2026-09', use_unit: '持睿汽车', usage: 100, amount: 100, unit_price: 1 },
    { month: '2026-09', use_unit: '持睿汽车', usage: 300, amount: 600, unit_price: 2 },
    { month: '2026-09', use_unit: '重庆星达', usage: 999, amount: 999, unit_price: 1 },
    { month: '2026-08', use_unit: '持睿汽车', usage: 888, amount: 888, unit_price: 1 },
  ]
  const c = ctx({ elecRows: rows })
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'usage', mode: 'field' }, c), 400)
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'amount', mode: 'field' }, c), 700)
  // 加权单价 = 700 / 400
  assert.equal(resolveSourceValue({ category: '电费核算', field: 'unit_price', mode: 'field' }, c), 1.75)
  // 气费同口径，公租房剔除
  const gas = ctx({
    gasRows: [
      { month: '2026-09', use_unit: '持睿汽车', meter_kind: 'dorm', usage: 12, amount: 36 },
      { month: '2026-09', use_unit: '持睿汽车', meter_kind: 'public', usage: 50, amount: 150 },
    ],
  })
  assert.equal(resolveSourceValue({ category: '气费核算', field: 'usage', mode: 'field' }, gas), 12)
})

test('引用字段：光伏账单按当月求和（无使用单位维度）', () => {
  const c = ctx({ solarItems: [{ item: { generation_kwh: 126350 } }, { item: { generation_kwh: 1000 } }] })
  assert.equal(resolveSourceValue({ category: '光伏账单', field: 'generation_kwh', mode: 'field' }, c), 127350)
})

test('分摊子项：市电分项按比例折算，其它分类直接求和', () => {
  const c = ctx({ elecRows: [{ month: '2026-09', use_unit: '持睿汽车', usage: 100, amount: 500, unit_price: 5 }] })
  const apportion = { source_category: '市电账单', source_key: '峰', source_mode: 'fee_row', value_mode: 'apportion', item_name: '峰' }
  assert.equal(resolveTenantItemValue(apportion, c, 0.5), 24464.18)
  const direct = { source_category: '电费核算', source_key: 'amount', source_mode: 'field', value_mode: 'direct_sum', item_name: '电费' }
  assert.equal(resolveTenantItemValue(direct, c, 0.5), 500)
})

test('parseFieldSource：缺字段/缺分类视为非引用（内置派生列）', () => {
  assert.equal(parseFieldSource({ name: 'month' }), null)
  assert.equal(parseFieldSource({ name: 'x', source: { category: '市电账单' } }), null)
  assert.deepEqual(
    parseFieldSource({ name: 'x', source: { category: '水费核算', field: 'amount' } }),
    { category: '水费核算', field: 'amount', mode: 'field' },
  )
  assert.deepEqual(
    parseFieldSource({ name: 'x', source: { category: '市电账单', field: '峰', mode: 'fee_row' } }),
    { category: '市电账单', field: '峰', mode: 'fee_row' },
  )
})

// ---- 需求 6：字段定义只允许跨分类检索 ----

test('费用清单字段定义：引用模式禁止自由新建，改用带来源徽标的选择器', () => {
  assert.match(certPanel, /const isRefCategory = computed/, '必须有引用模式判定')
  assert.match(certPanel, /=== '费用清单'/, '判定条件必须是费用清单')
  assert.match(certPanel, /<SourceFieldPicker/, '引用模式必须用跨分类选择器')
  assert.match(certPanel, /v-if="isRefCategory"/, '引用模式要分支渲染')
  assert.match(certPanel, /fc-ref-badge/, '引用字段行必须显示来源徽标')
  // 引用字段 key 由来源固定生成，且必须含**来源分组**：
  // 市电账单 8 个分组都有 qty（「计费电量」7 条），只用 分类_字段 会撞名，
  // 第二条起被判「已添加」而加不进来。
  assert.match(certPanel, /\['src', src\.category, src\.group, src\.field\]\.filter\(Boolean\)\.join\('_'\)/,
    '引用字段 key 必须含来源分组')
  // 选择器候选项带 [分类 · 分组] 路径前缀（只带分类无法区分同名项）
  assert.match(picker, /const path = groupLabel \? `\$\{cat\} · \$\{groupLabel\}` : cat/, '候选项必须拼多级路径')
  assert.match(picker, /label: `\[\$\{path\}\] \$\{name\}`/, '候选项必须带 [分类 · 分组] 前缀')
  // 字段模型要能持久化 source（含分组层级）
  const subs = read('./fleetCategorySubs.ts')
  assert.match(subs, /source\?: FieldSourceRef/, 'EditableField 要带 source')
  assert.match(subs, /group\?: string/, 'source 必须能带分组')
  assert.match(subs, /payload\.source = \{/, '保存时必须回写 source')
})

// ---- 需求 7：只读抽屉 ----

test('费用清单抽屉已下线（BSD 仍支持只读模式供其它模块用）', () => {
  assert.match(bsd, /readonly\?: boolean/, 'BSD 必须支持 readonly')
  assert.match(bsd, /v-if="readonly" class="bsd-readonly-value"/, '只读模式必须渲染纯文本')
  assert.match(bsd, /v-if="!readonly" theme="primary"/, '只读模式不得出现保存按钮')
  assert.match(bsd, /\{\{ readonly \? '关闭' : '取消' \}\}/, '只读底栏主按钮应是「关闭」')
  // 第 18 轮：费用清单不再挂载抽屉（点行不开弹窗），只保留打印两个入口。
  assert.ok(!/BusinessSplitDetailDrawer/.test(tenant), '费用清单不得再挂载详情抽屉')
  assert.ok(!/:show-preview="false" readonly/.test(tenant), '不得再传只读抽屉参数')
})

// ---- 需求 8：分摊子项 + 双打印 ----

test('分摊子项：一键导入市电账单字段 + 开关 + 跨分类检索 + 保存', () => {
  assert.match(panel, /导入市电账单字段/, '必须有一键导入入口')
  assert.match(panel, /const importUtilityFields = async/, '导入要实现')
  assert.match(panel, /listUtilityBillRecords/, '导入要读已提取的市电账单')
  assert.match(panel, /source_mode: 'fee_row'/, '导入的分项来源必须是市电费用分项')
  assert.match(panel, /<t-switch size="small"/, '每项必须有开关')
  assert.match(panel, /<SourceFieldPicker[\s\S]{0,160}:only-categories="nonUtilityCategories"/, '检索只允许其它分类')
  assert.match(panel, /value_mode: 'direct_sum'/, '其它分类字段按归属直接求和')
  assert.match(panel, /saveBillingTenantItems/, '必须持久化')
})

test('费用清单：选择租户后按分摊子项动态加载列与数据', () => {
  assert.match(tenant, /const tenantEnabledItems = computed/, '必须按启用的分摊子项派生列')
  assert.match(tenant, /async function loadTenantItems\(\)/, '必须加载该租户子项')
  assert.match(tenant, /getBillingTenant\(id\)/, '子项来自租户详情')
  assert.match(tenant, /resolveTenantItemValue\(it, ctx, ratio\)/, '列值必须走同一份口径')
  // 第 18 轮：合并列「合计应付」拆成「电费合计 / 水费合计」两条收口列
  assert.match(tenant, /TENANT_ELEC_TOTAL_FIELD/, '必须有「电费合计」列')
  assert.match(tenant, /TENANT_WATER_TOTAL_FIELD/, '必须有「水费合计」列')
  assert.ok(!/base\.item_total/.test(tenant), '不得再写旧的合并列 item_total')
})

test('浮动栏双打印：打印清单（结算单）与打印目录（表格）', () => {
  assert.match(tenant, />\s*打印清单\s*</, '必须有打印清单按钮')
  assert.match(tenant, />\s*打印目录\s*</, '必须有打印目录按钮')
  assert.match(tenant, /handlePrintStatement/, '打印清单要独立实现')
  assert.match(tenant, /handlePrintCatalog/, '打印目录要独立实现')
  assert.match(pdf, /export async function generateStatementPdf/, '必须有结算单 PDF 生成器')
  // 第 3 轮重构：结算单不再是「项目 | 来源分类 | 金额」三列，而是
  // 市电账单同款「概览 + 分项细项 + 水电气分区」；来源分类列随之取消。
  assert.match(pdf, /export interface StatementTable/, '结算单必须有分区表格模型')
  // 第 22 轮 P1-2：构造器迁到 energyBillStatements.ts（与市电/光伏清单同层）
  const statements = read('energyBillStatements.ts')
  assert.match(statements, /label: '费用组成'/, '电费细项必须按费用组成列示')
  assert.match(statements, /function elecTableOf/, '电费核算必须独立成表')
  assert.match(statements, /function waterTableOf/, '水费核算必须独立成表（水与电分开）')
})

// ---- 需求 9：生成账单 ----
// 第 17 轮口径变更：**弃用「数据不全禁止生成」门禁**。
// 旧断言（必须 readinessOf / 已禁止生成）已随产品口径作废：只要有数据就允许核算，
// 缺项按 0 参与计算。这里反向断言，防止有人把门禁加回来。

test('生成账单：勾选优先 + 气泡二次确认，且不得再有「数据不全禁止生成」门禁', () => {
  assert.match(tenant, /const generateTargets = computed/, '必须区分所选/全部')
  assert.match(tenant, /selectedKeys\.value\.size \? selectedRows\.value : displayRows\.value/, '勾选优先，否则全部')
  assert.match(tenant, /generateConfirmText/, '确认文案必须区分所选与全部')
  assert.match(tenant, /<t-popconfirm[\s\S]{0,220}@confirm="handleGenerateAll"/, '主按钮必须包气泡确认')
  assert.ok(!/function readinessOf/.test(tenant), '不得再有生成前完整性门禁')
  assert.ok(!/已禁止生成/.test(tenant), '不得再出现「已禁止生成」提示')
  assert.ok(!/reportReadiness/.test(tenant), '不得再调用门禁上报')
  // 主流程必须直奔生成，中间不得插入 return 阻断
  const fn = tenant.slice(tenant.indexOf('const handleGenerateAll'), tenant.indexOf('// 数据齐全时默认自动生成'))
  assert.ok(!/if \(issues\.length\)/.test(fn), '生成流程不得被校验中断')
  // 第 43 轮：改走批量重算接口（后端仍逐月 prepareBillingInputs → persistRecord 落库，
  // 且支持勾选单条/多条精准重算）。断言新的契约而不是旧的单月循环。
  assert.match(fn, /regenerateBillingRecords\(activeTenantId\.value/, '租户视角必须调批量重算接口')
  assert.match(fn, /picked \? months : \[\]/, '勾选 → 传所选月份；未勾选 → 传空数组（后端取全部）')
  assert.match(fn, /regenerateBillingByPeriod\(picked \? months : \[\]\)/, '业主视角必须走跨租户按周期重算')
  assert.match(fn, /已重新核算选中的/, '勾选场景提示必须写「选中的 k 条」')
  assert.match(fn, /已完成全部/, '全量场景提示必须写「全部 N 条」')
})

// ---- 第 13 轮：按「名称」精确匹配（同义词归一），绝不按数组下标 ----

test('费用分项匹配：名称同义词（其它/其他、因素/因数、全半角括号）都能命中', () => {
  const c = ctx({
    bill: {
      item: {
        total_kwh: 596168,
        // 力调电费的权威值在概况字段（fee_items 里常常没有行 / 只有 0 占位行）
        pf_adjust_amount: -2818.07,
        fee_items: [
          { category: '(4)系统运行费', name: '其他系统运行费用', fee: 2646.06 },
          { category: '(4)系统运行费', name: '上网环节线损代理采购损益', fee: -570.2 },
          { category: '(1)市场化购电费', name: '绿电交易电费（省间）', fee: 12.34 },
        ],
      },
    },
  })
  // 账单写「其他」，配置写「其它」
  assert.equal(resolveSourceValue({ category: '市电账单', field: '其它系统运行费用', mode: 'fee_row' }, c), 2646.06)
  // 账单写「因数」，配置写「因素」
  assert.equal(resolveSourceValue({ category: '市电账单', field: '功率因素调整电费', mode: 'fee_row' }, c), -2818.07)
  // 全半角括号
  assert.equal(resolveSourceValue({ category: '市电账单', field: '绿电交易电费(省间)', mode: 'fee_row' }, c), 12.34)
})

test('费用分项匹配：同一分组内按名称取值，不受数组顺序影响', () => {
  // 故意把账单行序打乱：取值必须只认名称，不能靠下标
  const rows = [
    { category: '(4)系统运行费', name: '其他系统运行费用', fee: 2646.06, rate: 0.004506 },
    { category: '(4)系统运行费', name: '新能源机制差价分摊电费', fee: 4431.24, rate: 0.007546 },
    { category: '(4)系统运行费', name: '电价交叉补贴新增损益', fee: -3250.91, rate: -0.005536 },
    { category: '(4)系统运行费', name: '上网环节线损代理采购损益', fee: -570.2, rate: -0.000971 },
  ]
  const c = ctx({ bill: { item: { total_kwh: 596168, fee_items: rows } } })
  const expect: Array<[string, number]> = [
    ['燃气机组容量电费', 0],
    ['新能源机制差价分摊电费', 4431.24],
    ['电价交叉补贴新增损益', -3250.91],
    ['上网环节线损代理采购损益', -570.2],
    ['其他系统运行费用', 2646.06],
  ]
  for (const [name, fee] of expect) {
    assert.equal(resolveSourceValue({ category: '市电账单', field: name, mode: 'fee_row' }, c), fee, name)
  }
})

// ---- 第 14 轮：政府性基金「工商业 / 居民」分组隔离 ----

test('分摊子项按分组取值：工商业基金不得混入居民同名行', () => {
  const ratio = 0.212356
  const c = ctx({
    bill: {
      item: {
        total_kwh: 596168,
        fee_items: [
          { category: '(6)政府性基金及附加-工商业', name: '农网还贷', qty: 587230, rate: 0.02, fee: 11744.6 },
          { category: '(6)政府性基金及附加-居民', name: '农网还贷', qty: 8938, rate: 0.02, fee: 178.76 },
          { category: '(6)政府性基金及附加-工商业', name: '国家重大水利工程建设基金', qty: 587230, rate: 0.001969, fee: 1156.26 },
          { category: '(6)政府性基金及附加-居民', name: '国家重大水利工程建设基金', qty: 8938, rate: 0.001969, fee: 16.4 },
        ],
      },
    },
  })
  const grp = '(6)政府性基金及附加-工商业'
  // 分组内只有 1 行；不带分组时才会把两行加在一起（旧 bug 的形态）
  assert.equal(matchBillFeeItems(c, '农网还贷', grp).length, 1)
  assert.equal(matchBillFeeItems(c, '农网还贷').length, 2)
  assert.equal(billRawAmountOf(c, '农网还贷', grp), 11744.6)
  assert.equal(billRawAmountOf(c, '农网还贷'), 11923.36)

  const item = {
    category: grp, item_name: '农网还贷', source_key: '农网还贷',
    source_category: '市电账单', source_mode: 'fee_row', value_mode: 'apportion',
  }
  assert.ok(Math.abs(resolveTenantItemValue(item, c, ratio) - 11744.6 * ratio) < 0.005,
    `分摊后应为 11744.6 × ${ratio}`)

  const water = { ...item, item_name: '国家重大水利工程建设基金', source_key: '国家重大水利工程建设基金' }
  assert.ok(Math.abs(resolveTenantItemValue(water, c, ratio) - 1156.26 * ratio) < 0.005)
})

test('六同步：列表列按归一名称去重，重复力调子项不得追加成额外列', () => {
  const billing = read('TenantBilling.vue')
  const groups = read('billingFieldGroups.ts')
  // 第 15 轮起去重规则收敛到 billingFieldGroups（列表与字段配置共用同一份签名）：
  // 列表取值用 uniqueTenantItems，字段配置用 uniqueTenantItems(build/merge)。
  assert.match(billing, /uniqueTenantItems\(tenantEnabledItems\.value, inSubs\)/,
    '列表必须用共享签名去重子项（否则力调被算两次）')
  assert.match(groups, /export function tenantItemSignature/, '缺少子项签名归一')
  assert.match(groups, /n\.includes\('功率因数调整'\) \? '功率因数调整电费' : n/,
    '力调电费的三种写法必须折叠成同一签名')
  assert.match(groups, /export function uniqueTenantItems/, '缺少共享去重函数')
  // 字段配置也必须去重：否则配置里两行「功率因数调整」而表头只有一列
  assert.match(groups, /uniqueTenantItems\(sortTenantItems\(items\)\.filter\(isTenantItemVisible\), preferNames\)/,
    'mergeTenantGroupFields 必须按签名去重（优先保留 subs 已引用的那条）')
})

// ---------------------------------------------------------------------------
// 第 39 轮：「零售损益分摊电费」统一按 账单原值 × 分摊比例
// ---------------------------------------------------------------------------
//
// 真机图 4（2026-10）：账单上该项是**纯金额行**（qty=0 / rate=0 / fee=51,418.66），
// 旧实现按「工业总电量 × 单价」找不到单价 → 落进「纯金额返还」判定被跳过 → 金额 0。
// 2026-09 则是按度行（qty=587,230 / rate=0.035557），旧口径算出 4,501.52。
// 现在两种形态统一：round2(账单原值 × ratio)。

test('零售损益分摊：2026-10 纯金额行按 原值 × 比例 = 14,207.60（不是 0）', () => {
  const c = ctx({
    month: '2026-10',
    bill: {
      item: {
        total_kwh: 623932,
        fee_items: [
          { category: '(1)市场化购电费', name: '零售损益分摊电费', period: '平', qty: 0, rate: 0, fee: 51418.66 },
        ],
      },
    },
    starTotal: 172400,
    ratio: 0.27631216,
  })
  const item = { source_category: '市电账单', source_key: '零售损益分摊电费', category: '(1)市场化购电费' }
  assert.equal(resolveTenantItemValue(item, c, 0.27631216), 14207.6)
})

test('零售损益分摊：2026-09 按度行同样按 原值 × 比例 = 4,434.03（旧口径 4,501.52）', () => {
  const c = ctx({
    bill: {
      item: {
        total_kwh: 596168,
        fee_items: [
          { category: '(1)市场化购电费', name: '零售损益分摊电费', period: '平', qty: 587230, rate: 0.035557, fee: 20880.13 },
          { category: '(2)上网环节线损费用', name: '上网环节线损费用', period: '平', qty: 587230, rate: 0.017144, fee: 10067.47 },
        ],
      },
    },
    starTotal: 126600,
    ratio: 0.21235625,
  })
  const item = { source_category: '市电账单', source_key: '零售损益分摊电费', category: '(1)市场化购电费' }
  const got = resolveTenantItemValue(item, c, 0.21235625)
  assert.equal(got, 4434.03)
  // 其它平段按度项仍是「工业总电量 × 单价」——不能被这条特例带偏
  const other = { source_category: '市电账单', source_key: '上网环节线损费用', category: '(2)上网环节线损费用' }
  assert.ok(Math.abs(resolveTenantItemValue(other, c, 0.21235625) - 126600 * 0.017144) < 0.02)
})
