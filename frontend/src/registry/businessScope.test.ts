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

test('正则补洞口径：合同/制度/公告/奖惩接入，车队与能耗不接入', () => {
  // 与后端 ScopeSchema.RegexPatchable 同源：已声明标准字段集的 document scope
  // 才允许为标准字段生成/应用 RE2 兜底正则。
  for (const s of ['contract', 'regulation', 'notice', 'award_punish']) {
    assert.equal(getScope(s)?.regexPatchable, true, `${s} 应接入正则补洞`)
  }
  for (const s of ['vehicle', 'driver', 'maintain', 'utility', 'solar']) {
    assert.equal(getScope(s)?.regexPatchable, false, `${s} 不应接入`)
  }
})

test('四个 document scope 的系统内置三件套（分类主轴 + 提取状态 + 标签）', () => {
  const triple: Record<string, string[]> = {
    contract: ['contractType', 'extractStatus', 'tags'],
    regulation: ['regType', 'extractStatus', 'tags'],
    notice: ['apType', 'extractStatus', 'tags'],
    award_punish: ['measure', 'extractStatus', 'tags'],
  }
  for (const [scope, keys] of Object.entries(triple)) {
    const d = getScope(scope)
    assert.ok(d, `${scope} 未注册`)
    for (const k of keys) {
      assert.ok((d.builtinFieldKeys || []).includes(k), `${scope} 缺内置字段 ${k}`)
      assert.ok(isStandardFieldOf(scope, k), `${scope} 的 ${k} 必须是标准字段`)
    }
  }
})

test('分类主轴文案与后端 ScopeSchema.CategoryLabel 一致', () => {
  assert.equal(getScope('contract')?.categoryLabel, '合同类型')
  assert.equal(getScope('regulation')?.categoryLabel, '制度类型')
  assert.equal(getScope('notice')?.categoryLabel, '公告类型')
  assert.equal(getScope('award_punish')?.categoryLabel, '奖惩类型')
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

test('notice 与 award_punish 字段零交集（消灭隐式混用）', () => {
  const notice = getScope('notice')!
  const ap = getScope('award_punish')!
  assert.ok(notice.standardFields.length > 0 && ap.standardFields.length > 0)
  const noticeKeys = new Set(notice.standardFields.map((f) => f.key.toLowerCase()))
  // 公告专属键不得出现在奖惩里，反之亦然
  for (const own of ['apNo', 'issueDept', 'signer']) {
    assert.ok(noticeKeys.has(own.toLowerCase()), `notice 应含 ${own}`)
    assert.equal(isStandardFieldOf('award_punish', own), false, `${own} 不得属于奖惩`)
  }
  for (const own of ['person', 'position', 'basis']) {
    assert.ok(isStandardFieldOf('award_punish', own), `奖惩应含 ${own}`)
    assert.equal(isStandardFieldOf('notice', own), false, `${own} 不得属于公告`)
  }
})

test('标准字段判定覆盖配置里可能出现的 camelCase 自定义形态', () => {
  assert.ok(isStandardFieldOf('contract', 'contractNo'))
  assert.ok(isStandardFieldOf('contract', 'contract_no'))
  assert.equal(isStandardFieldOf('contract', '合同编号'), false, '中文自定义键不是标准字段')
  assert.equal(isStandardFieldOf('unknown', 'invoice_no'), false)
})
