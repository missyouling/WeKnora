import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  CONTRACT_FIELD_LABELS,
  CONTRACT_FIELD_ORDER,
  CONTRACT_KEY_ALIASES,
  contractFieldLabel,
  contractKeyOf,
} from './contractFieldLabels'

// ---------------------------------------------------------------------------
// 合同字段体系重置（2026-10-21）：11 标准 + 15 扩展，严格顺序与后端
// business_field_catalog.go 的 contractCatalog() 逐项一致。
// 旧键（partyAName / expiryDate / contract_amount…）只做展示兼容与身份归一，
// 不再进入字段主轴。
// ---------------------------------------------------------------------------

test('合同字段顺序 = 11 标准 + 15 扩展，严格锁定', () => {
  assert.deepEqual(CONTRACT_FIELD_ORDER, [
    // 标准11
    'contract_no', 'contract_name', 'contract_type', 'party_a', 'party_b',
    'sign_date', 'expire_date', 'amount', 'extract_status', 'performance_status', 'tags',
    // 扩展 15
    'party_a_contact', 'party_a_tax_no', 'party_b_contact', 'party_b_tax_no',
    'party_a_address', 'party_a_phone', 'party_a_bank', 'party_a_account',
    'party_b_address', 'party_b_phone', 'party_b_bank', 'party_b_account',
    'payment_method', 'sign_location', 'summary',
  ])
  // 每个键都有中文名
  for (const k of CONTRACT_FIELD_ORDER) {
    assert.ok(CONTRACT_FIELD_LABELS[k], `${k} 缺中文名`)
  }
})

test('contractKeyOf 旧键归一到新契约键', () => {
  const pairs: Array<[string, string]> = [
    ['partyAName', 'party_a'], ['party_a_name', 'party_a'],
    ['partyBName', 'party_b'], ['party_b_name', 'party_b'],
    ['expiryDate', 'expire_date'], ['expiry_date', 'expire_date'],
    ['contractAmount', 'amount'], ['contract_amount', 'amount'],
    ['signPlace', 'sign_location'], ['sign_place', 'sign_location'],
    ['fulfillStatus', 'performance_status'], ['fulfill_status', 'performance_status'],
    // camelCase 直通 snake_case
    ['contractNo', 'contract_no'], ['partyATaxNo', 'party_a_tax_no'],
    ['paymentMethod', 'payment_method'], ['contract_no', 'contract_no'],
  ]
  for (const [from, to] of pairs) {
    assert.equal(contractKeyOf(from), to, `${from} 应归一为 ${to}`)
  }
  // 别名表的值必须落在字段顺序表内（旧键只指向存在的新键）
  for (const [legacy, canonical] of Object.entries(CONTRACT_KEY_ALIASES)) {
    assert.ok(CONTRACT_FIELD_ORDER.includes(canonical), `${legacy} → ${canonical} 不是合法契约键`)
  }
})

test('契约键与旧键都能翻成中文（列表/抽屉/质量面板统一口径）', () => {
  assert.equal(contractFieldLabel('party_a'), '甲方')
  assert.equal(contractFieldLabel('partyAName'), '甲方')
  assert.equal(contractFieldLabel('expire_date'), '到期日期')
  assert.equal(contractFieldLabel('expiryDate'), '到期日期')
  assert.equal(contractFieldLabel('amount'), '合同金额')
  assert.equal(contractFieldLabel('contractAmount'), '合同金额')
  assert.equal(contractFieldLabel('performance_status'), '履约状态')
  assert.equal(contractFieldLabel('fulfillStatus'), '履约状态')
  assert.equal(contractFieldLabel('summary'), '摘要')
  // 未收录键原样返回
  assert.equal(contractFieldLabel('not_a_field'), 'not_a_field')
})

// ---- 源码级锁定：列表列定义顺序与契约键一致 ----

const page = readFileSync(new URL('./ContractManagement.vue', import.meta.url), 'utf8')

test('DEFAULT_CONTRACT_COLUMNS 按契约键顺序生成（fileName 为系统列置尾）', () => {
  const block = page.slice(
    page.indexOf('const DEFAULT_CONTRACT_COLUMNS'),
    page.indexOf('const {', page.indexOf('const DEFAULT_CONTRACT_COLUMNS')),
  )
  const keys = [...block.matchAll(/key:\s*'([a-z_]+)'/g)].map((m) => m[1]!)
  const fileIdx = keys.indexOf('file_name')
  const business = fileIdx >= 0 ? keys.slice(0, fileIdx) : keys
  assert.deepEqual(business, CONTRACT_FIELD_ORDER, '列定义必须按 11 标准 + 15 扩展严格排序')
  if (fileIdx >= 0) assert.equal(fileIdx, keys.length - 1, 'fileName 只能作为系统列置尾')
})

test('行映射读新契约键（带旧键回退），不再只认 party_a_name 旧键', () => {
  assert.match(page, /r\.party_a\s*\|\|\s*r\.party_a_name/, 'mapContractRecord 必须新旧键兼容')
  assert.match(page, /r\.expire_date\s*\|\|\s*r\.expiry_date/)
  assert.match(page, /r\.amount\s*\?\?\s*r\.contract_amount/)
  assert.doesNotMatch(page, /partyAName:\s*r\.party_a_name\s*\|\|\s*''/,
    '旧的只认旧键映射必须废除')
})

test('履约状态在抽屉里是只读标签（履行中/即将到期/已超期），不是编辑控件', () => {
  assert.match(page, /<template #field-performance_status>/)
  const slot = page.slice(page.indexOf('<template #field-performance_status>'))
  assert.match(slot.slice(0, 600), /t-tag/, '履约状态渲染为 t-tag')
  assert.doesNotMatch(slot.slice(0, 600), /t-input|t-select|t-date-picker/,
    '履约状态不允许出现编辑控件')
})

test('editForm / 保存链路使用契约键（party_a / expire_date / amount / sign_location）', () => {
  assert.match(page, /party_a:\s*/)
  assert.match(page, /expire_date:\s*/)
  assert.match(page, /amount:\s*/)
  assert.match(page, /sign_location:\s*/)
  assert.doesNotMatch(page, /party_a_name:\s*editForm/, '落库键必须切到 party_a')
  assert.doesNotMatch(page, /expiry_date:\s*editForm/)
  assert.doesNotMatch(page, /contract_amount:\s*editForm/)
  assert.doesNotMatch(page, /sign_place:\s*editForm/)
})
