import assert from 'node:assert/strict'
import test from 'node:test'
import {
  EMPTY_QUALITY,
  parseExtractQuality,
  qualityIsEmpty,
  qualityIssueCounts,
  qualityPatchLabels,
} from './extractQuality'

test('历史数据防御：meta 缺失或字段缺失一律返回空质量对象，不抛错', () => {
  for (const input of [undefined, null, {}, { invoices: [] }, { extract_issues: null, patched_by_regex: null }]) {
    const q = parseExtractQuality(input as any)
    assert.deepEqual(q.issues, [])
    assert.deepEqual(q.patches, [])
    assert.deepEqual(q.patchedFields, [])
    assert.equal(qualityIsEmpty(q), true)
  }
  assert.equal(qualityIsEmpty(EMPTY_QUALITY), true)
})

test('脏数据不炸：issues/patches 非数组、条目缺字段、severity 非法时按最宽松口径收敛', () => {
  const q = parseExtractQuality({
    extract_issues: [
      { field: 'invoiceNo', severity: 'ERROR', detail: '缺失', index: 0 },
      { field: 'amount', severity: 'fatal', kind: 'mismatch', index: 1 },
      { field: '', severity: 'warn' },
      'not-an-object',
      null,
    ],
    regex_patches: [{ field: 'invoice_no', value: '123', index: 0 }, null, { value: 'x' }],
    patched_by_regex: ['invoice_no', '', 3],
  })
  // 空 field 与非法条目被丢弃，severity 大小写归一、未知等级降级为 warn
  assert.equal(q.issues.length, 2)
  assert.equal(q.issues[0].severity, 'error')
  assert.equal(q.issues[1].severity, 'warn')
  assert.equal(q.issues[1].kind, 'mismatch')
  // 补洞明细：无 field 的条目丢弃（只有 1 条同时具备 field/value 的合法明细）
  assert.equal(q.patches.length, 1)
  assert.deepEqual(q.patchedFields, ['invoice_no'])
})

test('单条记录视图：按 index 过滤，index<0 的全局问题始终展示', () => {
  const meta = {
    extract_issues: [
      { index: -1, field: 'contractNo', severity: 'error', detail: '全局必填' },
      { index: 0, field: 'signDate', severity: 'warn', detail: '日期格式' },
      { index: 1, field: 'partyAName', severity: 'error', detail: '缺失' },
    ],
    regex_patches: [
      { index: 0, field: 'sign_date', value: '2025-05-13' },
      { index: 1, field: 'party_a_name', value: '某公司' },
    ],
  }
  const second = parseExtractQuality(meta, 1)
  assert.deepEqual(second.issues.map((i) => i.field), ['contractNo', 'partyAName'])
  assert.deepEqual(second.patches.map((p) => p.field), ['party_a_name'])
  // 未传 index（列表/测试弹窗视角）：全量展示
  assert.equal(parseExtractQuality(meta).issues.length, 3)
  assert.equal(parseExtractQuality(meta).patches.length, 2)
})

test('补洞字段名：patched_by_regex 缺失时由 regex_patches 派生，两处共存时以补洞明细为准去重', () => {
  const derived = parseExtractQuality({ regex_patches: [{ index: 0, field: 'invoice_no', value: 'x' }] })
  assert.deepEqual(derived.patchedFields, ['invoice_no'])
  const merged = parseExtractQuality({
    patched_by_regex: ['invoice_no'],
    regex_patches: [{ index: 0, field: 'invoice_no', value: 'x' }],
  })
  assert.deepEqual(merged.patchedFields, ['invoice_no'])
})

test('徽章文案按字段聚合，同一字段多次补洞只出一枚徽章', () => {
  const q = parseExtractQuality({
    patched_by_regex: ['invoiceNo'],
    regex_patches: [
      { index: 0, field: 'invoiceNo', value: '111' },
      { index: 1, field: 'invoiceNo', value: '222' },
    ],
  })
  const labels = qualityPatchLabels(q, (f) => `发票号码(${f})`)
  assert.equal(labels.length, 1)
  assert.equal(labels[0].field, 'invoiceNo')
  assert.equal(labels[0].label, '发票号码(invoiceNo)')
  assert.equal(labels[0].count, 2)
  assert.deepEqual(labels[0].values, ['111', '222'])
})

test('问题计数：error/warn 分开统计，未知等级不计入 error', () => {
  const q = parseExtractQuality({
    extract_issues: [
      { field: 'a', severity: 'error' },
      { field: 'b', severity: 'error' },
      { field: 'c', severity: 'warn' },
    ],
  })
  assert.deepEqual(qualityIssueCounts(q), { error: 2, warn: 1 })
})
