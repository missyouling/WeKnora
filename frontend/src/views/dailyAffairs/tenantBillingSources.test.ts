import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseFieldSource, resolveSourceValue, resolveTenantItemValue } from './tenantBillingSources'

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
  assert.match(certPanel, /fc-ref-badge/, '引用字段行必须显示来源分类徽标')
  assert.match(certPanel, /`src_\$\{payload\.source\.category\}_\$\{payload\.source\.field\}`/, '引用字段 key 由来源固定生成')
  // 选择器候选项带 [来源分类] 前缀
  assert.match(picker, /label: `\[\$\{cat\}\] \$\{sub\.label \|\| sub\.name\}`/, '候选项必须带 [来源分类]')
  // 字段模型要能持久化 source
  const subs = read('./fleetCategorySubs.ts')
  assert.match(subs, /source\?: \{ category: string; field: string; mode\?: 'field' \| 'fee_row' \}/, 'EditableField 要带 source')
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
