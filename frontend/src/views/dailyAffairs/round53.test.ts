import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  AUTO_CODE_COL_CAP, AUTO_COL_CAP, colWidthOf, estimateColWidth,
} from './columnWidth'
import { hrColumnsFromSubs } from './hrCore'

/**
 * 第 53 轮回归：列宽生效、长号码自适应、跨模块引用字段取值、
 * 工业均价 tooltip 口径、五列表 subs 消费。
 */
const read = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')

// ---------------------------------------------------------------------------
// 1. 列宽：用户配置 > 0 必须生效；0 才自适应
// ---------------------------------------------------------------------------

test('18 位身份证自适应宽度突破 150 上限（不再出现省略号）', () => {
  const w = estimateColWidth('证件号码', ['510111196912254212'])
  // 18 位 ASCII 号码 = 18×8 + 24 padding = 168px，必须完整放下
  assert.equal(w, 168, `18 位身份证应为 168px，实际 ${w}`)
  assert.ok(w > AUTO_COL_CAP, '必须突破普通文本 150 的上限')
  assert.ok(w <= AUTO_CODE_COL_CAP, '不得超过长号码上限 240')
  // 19 位工资卡号：19×8+24 = 176
  assert.equal(estimateColWidth('工资卡卡号', ['6217003760153123456']), 176)
})

test('普通长文本仍封顶 150（长号码放宽不能误伤备注列）', () => {
  const longText = '业务流水号:2026102215000001；'.repeat(10)
  assert.equal(estimateColWidth('备注', [longText]), AUTO_COL_CAP)
  assert.equal(colWidthOf(0, '备注', [longText], 'textarea'), 260)
})

test('显式列宽 250 原样生效，且不受自适应上限影响', () => {
  assert.equal(colWidthOf(250, '证件号码', ['510111196912254212']), 250)
  assert.equal(colWidthOf(250, '证件号码', ['510111196912254212'], 'text'), 250)
})

test('hrColumnsFromSubs 把 subs.width 透传成 width + minWidth', () => {
  const cols = hrColumnsFromSubs([
    { name: '证件号码', enabled: true, data_type: 'text', width: 250 },
    { name: '姓名', enabled: true, data_type: 'text', width: 0 },
  ], [])
  const id = cols.find((c) => c.colKey === '证件号码')
  const name = cols.find((c) => c.colKey === '姓名')
  assert.equal(id?.width, 250, '配置宽度必须进入列定义')
  assert.equal(id?.minWidth, 250, '同时钉住 minWidth，防末列拉伸')
  assert.equal(name?.width, 0, '未配置宽度 = 自适应')
  assert.equal(name?.minWidth, undefined)
})

// ---------------------------------------------------------------------------
// 2. 跨模块引用字段：按 source.field 取值 + 状态列走三色插槽
// ---------------------------------------------------------------------------

test('引用「劳动合同 · 合同状态」到花名册：列键=显示名，取值=来源字段，走 Tag 插槽', () => {
  const cols = hrColumnsFromSubs([
    {
      name: 'src_劳动合同_合同状态',
      label: '合同状态',
      enabled: true,
      is_default: true,
      data_type: 'text',
      source: { category: '劳动合同', field: '合同状态' },
    },
  ], [])
  const col = cols.find((c) => c.colKey === '合同状态')
  assert.ok(col, '引用字段必须产出列')
  assert.equal(col?.slot, true, '合同状态必须走具名插槽渲染三色 Tag')
  assert.equal(col?.value?.({ custom_data: { 合同状态: '即将到期' } }), '即将到期')
  assert.equal(col?.value?.({ 合同状态: '到期' }), '到期')
})

test('引用「社保管理 · 个人社保编号」到花名册：按来源字段取值', () => {
  const cols = hrColumnsFromSubs([
    {
      name: 'src_社保管理_个人社保编号',
      label: '个人社保编号',
      enabled: true,
      data_type: 'text',
      source: { category: '社保管理', field: '个人社保编号' },
    },
  ], [])
  const col = cols.find((c) => c.colKey === '个人社保编号')
  assert.equal(col?.value?.({ custom_data: { 个人社保编号: 'JM000123' } }), 'JM000123')
})

// ---------------------------------------------------------------------------
// 3. 工业均价 tooltip：分母必须是工业电量
// ---------------------------------------------------------------------------

test('工业均价 tooltip 展示 工业电费 ÷ 工业电量，不再出现宿舍电量分母', () => {
  const src = read('./tenantBillingTips.ts')
  const idx = src.indexOf('计算方式：工业电费 ÷ 工业电量')
  assert.ok(idx > 0, 'tooltip 必须改成新口径')
  const block = src.slice(idx, idx + 260)
  assert.match(block, /f\.indFee/, '分子必须是工业电费')
  assert.match(block, /f\.indKwh/, '分母必须是工业电量')
  assert.ok(!/tenantDorm/.test(block), '不得再出现宿舍电量/宿舍电费')
})

test('TenantBilling 业主视角均价公式分子分母同源，且先算电费再算均价', () => {
  const src = read('./TenantBilling.vue')
  const idx = src.indexOf('工业均价 = round4(工业电费 ÷ 工业电量)')
  assert.ok(idx > 0)
  const block = src.slice(idx, idx + 420)
  assert.match(block, /indPrice = indKwh > 0 \? Math\.round\(\(indFee \/ indKwh\) \* 10000\) \/ 10000/)
  assert.ok(!/tenantDormKwh/.test(block))
  assert.ok(!/tenantDormFee/.test(block))
  // indFee 必须定义在 indPrice 之前（否则引用未初始化变量）
  const feeIdx = src.indexOf('const indFee = Math.round(indKwh * priceExact * 100) / 100')
  assert.ok(feeIdx > 0 && feeIdx < idx, 'indFee 必须先于 indPrice 定义')
})

// ---------------------------------------------------------------------------
// 4. 持睿汽车列：useUnit 必须回写 activeTenantId + 空列集回落
// ---------------------------------------------------------------------------

test('切使用单位时同步 activeTenantId（否则列分组/子项/tenantName 全错）', () => {
  const src = read('./TenantBilling.vue')
  const idx = src.indexOf('const onUseUnitChange = () => {')
  assert.ok(idx > 0)
  const block = src.slice(idx, idx + 700)
  assert.match(block, /activeTenantId\.value = picked\?\.id \|\| ''/, '必须按 useUnit 解析租户 id')
  assert.match(block, /t\.name \|\| ''\)\.trim\(\) === String\(useUnit\.value/, '按名称匹配')
})

test('列派生为空时回落静态列，绝不允许只剩两条收口列', () => {
  const src = read('./TenantBilling.vue')
  assert.match(src, /if \(out\.length <= TENANT_TOTAL_FIELDS\.length\)/, '必须有空列集兜底')
  const idx = src.indexOf('if (out.length <= TENANT_TOTAL_FIELDS.length)')
  const block = src.slice(idx, idx + 700)
  assert.match(block, /return \[\.\.\.base, \.\.\.items\]/, '回落静态列 + 当前子项')
})
