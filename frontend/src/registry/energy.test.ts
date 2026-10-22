import assert from 'node:assert/strict'
import test from 'node:test'
import '../registry'
import { getScope, labelOf, isStandardFieldOf, isBuiltinFieldOf, scopeKeys } from './businessScope'

// 能耗（energy）scope 的前端契约断言。
//
// 与后端 internal/application/service/energy_scope_test.go 同口径：
// 两处字段集必须逐项一致，否则「设置里配的字段」与「提取器读的字段」会脱钩。

test('energy scope 已注册', () => {
  assert.ok(scopeKeys().includes('energy' as never), 'energy 未注册')
  const d = getScope('energy')
  assert.ok(d)
  assert.equal(d.kind, 'document')
  assert.equal(d.categoryLabel, '能耗类型')
  assert.equal(d.listRows, 'documents')
  assert.deepEqual(d.api, { extract: 'business' })
})

test('能耗产品清单的 10 个系统级标准字段齐全', () => {
  const d = getScope('energy')
  assert.ok(d)
  for (const k of [
    'utility_no', 'billing_period', 'utility_type',
    'previous_reading', 'current_reading', 'multiplier',
    'usage_amount', 'unit_price', 'total_amount', 'extract_status',
  ]) {
    assert.ok(isStandardFieldOf('energy', k), `${k} 必须是标准字段`)
  }
})

test('能耗读数类字段是 number 类型（抽屉渲染数字输入框）', () => {
  const d = getScope('energy')
  assert.ok(d)
  const byKey = new Map(d.standardFields.map((f) => [f.key, f]))
  for (const k of [
    'previous_reading', 'current_reading', 'multiplier',
    'usage_amount', 'unit_price', 'total_amount',
  ]) {
    assert.equal(byKey.get(k)?.dataType, 'number', `${k} 必须是 number`)
  }
  assert.equal(byKey.get('billing_period')?.dataType, 'text', '账期是月粒度文本，不是 date')
})

test('能耗字段中文名可查（labelOf 命中标准字段）', () => {
  assert.equal(labelOf('energy', 'previous_reading'), '上期读数')
  assert.equal(labelOf('energy', 'current_reading'), '本期读数')
  assert.equal(labelOf('energy', 'usage_amount'), '实际用量')
  assert.equal(labelOf('energy', 'utility_no'), '单据编号/户号')
  assert.equal(labelOf('energy', 'billing_period'), '账期')
})

test('能耗支持明细子表格（多表位读数并入 items，一文件一档案）', () => {
  const d = getScope('energy')
  assert.ok(d)
  assert.equal(d.supportsSubFields, true, '能耗汇总单含多表位，需要明细子表格')
  assert.equal(d.itemLabel, '表位明细')
})

test('能耗接入正则补洞（与后端 ScopeSchema.RegexPatchable 同源）', () => {
  const d = getScope('energy')
  assert.ok(d)
  assert.equal(d.regexPatchable, true, 'energy 有标准字段集，应接入正则补洞')
})

test('能耗系统内置三件套不可删', () => {
  assert.equal(isBuiltinFieldOf('energy', 'utilityType'), true, '分类主轴不可删')
  assert.equal(isBuiltinFieldOf('energy', 'extractStatus'), true, '提取状态不可删')
  assert.equal(isBuiltinFieldOf('energy', 'tags'), true, '标签不可删')
  assert.equal(isBuiltinFieldOf('energy', 'usage_amount'), false, '用量可删')
})

test('能耗规则面板内置账期与读数正则预设', () => {
  const d = getScope('energy')
  assert.ok(d)
  const names = (d.regexPresets || []).map((p) => p.name)
  for (const want of ['账期(YYYY-MM)', '上期读数', '本期读数', '倍率', '实际用量']) {
    assert.ok(names.includes(want), `缺正则预设 ${want}`)
  }
  // 账期正则必须能匹配 2026-09 / 2026年9月
  const preset = (d.regexPresets || []).find((p) => p.name === '账期(YYYY-MM)')
  assert.ok(preset)
  const re = new RegExp(preset.value)
  assert.ok(re.test('账期：2026-09'), '账期正则必须匹配 2026-09')
  assert.ok(re.test('账单周期 2026年9月'), '账期正则必须匹配 2026年9月')
})

test('能耗与 utility/solar 是三个独立 scope（口径不同，不得混用）', () => {
  assert.equal(getScope('utility')?.categoryLabel, '账单类型')
  assert.equal(getScope('solar')?.categoryLabel, '账单类型')
  assert.equal(getScope('energy')?.categoryLabel, '能耗类型')
  // utility/solar 仍是账单明细级口径：不接正则补洞、无字段字典
  assert.equal(getScope('utility')?.regexPatchable, false)
  assert.equal(getScope('solar')?.regexPatchable, false)
})
