import { test } from 'node:test'
import assert from 'node:assert/strict'
import { makeFilePlaceholder } from './invoicePlaceholder'

// 第 26 轮 O-5：文件级占位行统一构造。
//
// 发票列表有三种「文件级占位行」（该记录没有可展示的发票明细）：
//   ① meta 缺失（老数据 / 上传中）→ kind=unknown，状态空
//   ② 系统判定非发票           → kind=not_invoice，状态强制 not_invoice
//   ③ 是发票但一张都没提出来     → kind=invoice，状态取 meta.extract_status
//
// 三处此前各写一遍 {...base} 展开，字段顺序与默认值容易漂移。

const base = {
  parseStatus: 'completed',
  summaryStatus: '',
  fileType: 'pdf',
  tags: [],
  description: '',
}

test('O-5a：meta 缺失 → kind=unknown、状态为空', () => {
  const row = makeFilePlaceholder({ id: 'k1', fileName: 'a.pdf', base })
  assert.equal(row.kind, 'unknown')
  assert.equal(row.extractStatus, '')
  assert.equal(row.extractError, '')
  assert.equal(row.rowKey, 'k1')
  assert.equal(row.knowledgeId, 'k1')
})

test('O-5b：非发票 → 状态强制 not_invoice（不受 meta 影响）', () => {
  const row = makeFilePlaceholder({
    id: 'k2', fileName: 'b.pdf', base, kind: 'not_invoice', extractStatus: 'failed', extractError: '不是发票',
  })
  assert.equal(row.kind, 'not_invoice')
  assert.equal(row.extractStatus, 'not_invoice', '非发票必须落 not_invoice，不能透传 failed')
  assert.equal(row.extractError, '不是发票')
})

test('O-5c：发票但零明细 → 透传 meta 状态', () => {
  const row = makeFilePlaceholder({
    id: 'k3', fileName: 'c.pdf', base, kind: 'invoice', extractStatus: 'failed', extractError: '限流',
  })
  assert.equal(row.kind, 'invoice')
  assert.equal(row.extractStatus, 'failed')
  assert.equal(row.extractError, '限流')
})

test('O-5d：base 字段必须完整透传（parseStatus 等驱动状态徽标）', () => {
  const row = makeFilePlaceholder({ id: 'k4', fileName: 'd.pdf', base, kind: 'invoice' })
  assert.equal(row.parseStatus, 'completed')
  assert.equal(row.fileType, 'pdf')
  assert.deepEqual(row.tags, [])
})

test('O-5e：rowKey 与 knowledgeId 同源（表格选中/删除按 knowledgeId 聚合）', () => {
  const row = makeFilePlaceholder({ id: 'abc', fileName: 'x.pdf', base })
  assert.equal(row.rowKey, 'abc')
  assert.equal(row.knowledgeId, 'abc')
})
