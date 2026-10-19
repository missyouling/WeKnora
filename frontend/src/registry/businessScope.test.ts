import assert from 'node:assert/strict'
import test from 'node:test'
import '../registry'
import { getScope, labelOf, isStandardFieldOf, scopeKeys } from './businessScope'

// BusinessScopeRegistry（蓝图 P1-1）的行为级断言。
// 红线：消灭硬编码的同时，发票的判定口径必须与旧实现逐项一致。

test('十个业务 scope 全部注册', () => {
  const keys = scopeKeys()
  for (const k of ['invoice', 'contract', 'regulation', 'notice', 'award_punish', 'vehicle', 'driver', 'maintain', 'utility', 'solar']) {
    assert.ok(keys.includes(k as never), `${k} 未注册`)
  }
})

test('发票 descriptor 声明三件套', () => {
  const d = getScope('invoice')
  assert.ok(d)
  assert.equal(d.kind, 'invoice')
  assert.equal(d.regexPatchable, true)
  assert.equal(d.supportsSubFields, true)
  assert.equal(d.supportsRemarkParts, true)
  assert.ok(d.standardFields.length >= 22, '补洞面 22 键 + 字典同义键')
  // 补洞面（INVOICE_STRUCT_KEYS）必须全部在标准字段里
  for (const k of ['invoice_no', 'invoice_code', 'invoice_date', 'total_amount', 'tax_rate', 'seller_tax_no']) {
    assert.ok(isStandardFieldOf('invoice', k), `${k} 必须是标准字段`)
    assert.ok(isStandardFieldOf('invoice', k.replace(/_([a-z])/g, (_, c) => c.toUpperCase())), `${k} 的 camel 形态也必须认`)
  }
})

test('document / archive 类 scope 不接入正则补洞', () => {
  for (const s of ['contract', 'regulation', 'notice', 'award_punish', 'vehicle', 'driver', 'maintain', 'utility', 'solar']) {
    assert.equal(getScope(s)?.regexPatchable, false, `${s} 不应接入`)
  }
})

test('labelOf 与旧 lookupLabel 口径一致（camel/snake/原名三种写法都认）', () => {
  assert.equal(labelOf('invoice', 'invoiceNo'), '发票号码')
  assert.equal(labelOf('invoice', 'invoice_no'), '发票号码')
  assert.equal(labelOf('invoice', 'InvoiceNO'), '发票号码')
  // 无映射原样返回
  assert.equal(labelOf('invoice', '车牌号'), '车牌号')
  // 未注册 scope 原样返回
  assert.equal(labelOf('not-a-scope', 'x'), 'x')
})

test('notice 与 award_punish 各自显式注册（消灭隐式混用）', () => {
  const notice = getScope('notice')
  const ap = getScope('award_punish')
  assert.ok(notice && ap)
  // 两者字典同源（历史数据），但 ap 有专属键
  assert.ok(isStandardFieldOf('award_punish', 'ap_no'))
  assert.ok(notice.standardFields.length > 0, 'notice 必须有自己的字段表，不再借道')
})

test('标准字段判定覆盖配置里可能出现的 camelCase 自定义形态', () => {
  assert.ok(isStandardFieldOf('contract', 'contractNo'))
  assert.ok(isStandardFieldOf('contract', 'contract_no'))
  assert.equal(isStandardFieldOf('contract', '合同编号'), false, '中文自定义键不是标准字段')
  assert.equal(isStandardFieldOf('unknown', 'invoice_no'), false)
})
