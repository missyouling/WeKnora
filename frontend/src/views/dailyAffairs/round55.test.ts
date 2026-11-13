/**
 * 第 55 轮纯函数回归：KPI 精简文案 / 双胶囊契约 / 状态加锁 / 月度账单列。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  buildHrKpiCards, CONTRACT_STATUS_OPTIONS, hrBillColumnDefs, hrFundColumns,
} from './hrCore'
import {
  HR_FUND_FIELDS, HR_FUND_BILL_FIELDS, HR_FUND_GROUPS,
  HR_FUND_BILL_GROUP, HR_FUND_DETAIL_GROUP, HR_FUND_RATIOS,
  HR_FUND_DEFAULT_HEADERS,
} from './hrFieldSchema'
import { statusFieldSpecsOf, isHrStatusName, isLockedStatusField } from './statusFields'

test('合同卡片异常文案精简为「X 份状态异常」，单一异常带类型后缀', () => {
  const cards = buildHrKpiCards({
    contract_unsigned: 2, contract_expiring: 1, contract_expired: 1,
  })
  const c = cards.find((x) => x.key === 'hr-contract')!
  assert.equal(c!.sub, '4 份状态异常')
  assert.equal(c!.subCls, 'is-warning')

  const single = buildHrKpiCards({ contract_unsigned: 2 }).find((x) => x.key === 'hr-contract')!
  assert.equal(single!.sub, '2 份状态异常 · 未签订')

  const none = buildHrKpiCards({}).find((x) => x.key === 'hr-contract')!
  assert.equal(none!.sub, '无状态异常')
})

test('公积金卡片主数字=账单条数，副文案「X 人待补录 · Y 人待封存」', () => {
  const cards = buildHrKpiCards({ 'hr-fund': 3, fund_pending: 6, fund_to_seal: 3 })
  const c = cards.find((x) => x.key === 'hr-fund')!
  assert.equal(c!.value, '3')
  assert.equal(c!.sub, '6 人待补录 · 3 人待封存')
  assert.equal(c!.subCls, 'is-warning')
  const only = buildHrKpiCards({ 'hr-fund': 1, fund_pending: 2 }).find((x) => x.key === 'hr-fund')!
  assert.equal(only!.sub, '2 人待补录')
})

test('合同状态下拉首项统一为「全部状态」', () => {
  assert.equal(CONTRACT_STATUS_OPTIONS[0].value, '全部状态')
  assert.deepEqual(CONTRACT_STATUS_OPTIONS.map((o) => o.value),
    ['全部状态', '正常', '未签订', '即将到期', '到期', '已解除'])
})

test('公积金双胶囊：月度账单 8 + 职工明细 18，顺序账单在前', () => {
  assert.equal(HR_FUND_BILL_FIELDS.length, 8)
  assert.equal(HR_FUND_FIELDS.length, 18)
  assert.equal(HR_FUND_GROUPS.length, 2)
  assert.deepEqual(HR_FUND_GROUPS.map((g) => g.key), [HR_FUND_BILL_GROUP, HR_FUND_DETAIL_GROUP])
  assert.ok(HR_FUND_BILL_FIELDS.every((d) => d.group === HR_FUND_BILL_GROUP))
  assert.ok(HR_FUND_FIELDS.every((d) => d.group === HR_FUND_DETAIL_GROUP))
})

test('六同步：4 个「是否」默认表头关闭，比例字段带 5~12 下拉选项', () => {
  const closed = ['是否公贷', '是否冻结', '是否可在线提取公贷', '是否可在线提取商贷']
  for (const name of closed) {
    const d = HR_FUND_FIELDS.find((x) => x.name === name)!
    assert.ok(d, name)
    assert.equal(d!.defaultHeader, undefined, `${name} 默认表头必须关`)
    assert.ok(!HR_FUND_DEFAULT_HEADERS.includes(name))
  }
  assert.equal(HR_FUND_DEFAULT_HEADERS.length, 14)
  assert.deepEqual(HR_FUND_RATIOS, ['5', '6', '7', '8', '9', '10', '11', '12'])
  for (const name of ['单位缴存比例(%)', '职工缴存比例(%)', '账户状态', '联名卡银行']) {
    const d = HR_FUND_FIELDS.find((x) => x.name === name)!
    assert.ok(d!.options?.length, `${name} 必须是 t-select 下拉`)
  }
})

test('状态字段全局加锁，且公积金不再自动注入「缴存状态」', () => {
  assert.equal(isHrStatusName('合同状态'), true)
  assert.equal(isHrStatusName('账户状态'), true)
  assert.equal(isHrStatusName('账单状态'), true)
  assert.equal(isLockedStatusField('hr-contract', '', '合同状态'), true)
  assert.equal(isLockedStatusField('hr-fund', '', '账户状态'), true)
  assert.equal(isLockedStatusField('hr-fund', '', '账单状态'), true)
  assert.deepEqual(statusFieldSpecsOf('hr-fund').map((s) => s.name), [])
})

test('月度账单 8 列无 fixed、无操作列；职工明细 18 列', () => {
  const bills = hrBillColumnDefs()
  assert.equal(bills.length, 9) // 含勾选列
  assert.equal(bills[0].colKey, 'row-select')
  assert.ok(bills.every((c) => !(c as any).fixed))
  assert.deepEqual(bills.slice(1).map((c) => c.colKey), [
    '账单周期', '汇缴人数', '缴存基数合计(元)', '单位缴存额(元)',
    '个人缴存额(元)', '月缴存总额(元)', '账单状态', '生成时间',
  ])
  const detail = hrFundColumns()
  assert.equal(detail.length, 19)
  // 比例列不进合计行
  assert.equal(detail.find((c) => c.colKey === '单位缴存比例(%)')!.noSum, true)
  assert.equal(detail.find((c) => c.colKey === '职工缴存比例(%)')!.noSum, true)
})
