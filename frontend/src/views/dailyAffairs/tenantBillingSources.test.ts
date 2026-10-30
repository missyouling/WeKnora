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

test('费用清单抽屉：BSD 只读模式，无输入框与保存按钮', () => {
  assert.match(bsd, /readonly\?: boolean/, 'BSD 必须支持 readonly')
  assert.match(bsd, /v-if="readonly" class="bsd-readonly-value"/, '只读模式必须渲染纯文本')
  assert.match(bsd, /v-if="!readonly" theme="primary"/, '只读模式不得出现保存按钮')
  assert.match(bsd, /\{\{ readonly \? '关闭' : '取消' \}\}/, '只读底栏主按钮应是「关闭」')
  assert.match(tenant, /:show-preview="false" readonly/, '费用清单抽屉必须传 readonly')
  assert.doesNotMatch(tenant, /@save="saveRecordDetail"/, '费用清单抽屉不得再挂保存')
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
  assert.match(tenant, /item_total/, '必须有「合计应付」列')
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
  assert.match(tenant, /label: '费用组成'/, '电费细项必须按费用组成列示')
  assert.match(tenant, /function elecTableOf/, '电费核算必须独立成表')
  assert.match(tenant, /function waterTableOf/, '水费核算必须独立成表（水与电分开）')
})

// ---- 需求 9：生成账单 ----

test('生成账单：勾选优先 + 气泡二次确认 + 数据不全禁止生成', () => {
  assert.match(tenant, /const generateTargets = computed/, '必须区分所选/全部')
  assert.match(tenant, /selectedKeys\.value\.size \? selectedRows\.value : displayRows\.value/, '勾选优先，否则全部')
  assert.match(tenant, /generateConfirmText/, '确认文案必须区分所选与全部')
  assert.match(tenant, /<t-popconfirm[\s\S]{0,220}@confirm="handleGenerateAll"/, '主按钮必须包气泡确认')
  assert.match(tenant, /function readinessOf/, '必须做完整性校验')
  assert.match(tenant, /已禁止生成/, '数据不全必须明确提示禁止生成')
  // 校验必须覆盖：市电账单 / 星达分时读数 / 租户电表 / 水 / 气
  for (const kw of ['市电账单', '星达分时读数', '电表读数', '水表读数', '气表读数']) {
    assert.ok(tenant.includes(kw), `校验必须覆盖 ${kw}`)
  }
  assert.match(tenant, /if \(issues\.length\) \{ reportReadiness\(issues\); return \}/, '校验失败必须直接返回，不落库')
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
