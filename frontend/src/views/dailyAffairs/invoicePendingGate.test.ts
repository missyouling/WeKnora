// 列表行准入的第二道门禁（第 24 轮任务一）。
//
// 真机事故（图 1）：状态栏还显示「正在解析 2026.9-10油费发票.pdf」，
// 列表第 17~20 行已经出现该文件的 P1~P4（第 20 行还是「待提取 + 0.00」），
// KPI 卡片也提前变了。
//
// 后端已按 parse_status/extract_status 过滤，但**前端必须再挡一次**：
// 轮询周期内后端返回的是上一次快照，而 pendingFiles 已经先一步反映
// 「这个文件正在解析」。判据是 pendingFiles 里的文件 ID（= knowledgeId）
// 对应的**所有分页子记录**一律剔除 —— 同一文件的行必须整组进出。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import { filterRowsByPendingFiles } from './invoicePendingGate'

const rows = [
  { rowKey: 'a1', knowledgeId: 'k1', invoiceNo: '111' },
  { rowKey: 'a2', knowledgeId: 'k1', invoiceNo: '112' },
  { rowKey: 'b1', knowledgeId: 'k2', invoiceNo: '221' },
  { rowKey: 'c1', knowledgeId: 'k3', invoiceNo: '331' },
]

test('无进行中文件时原样返回（不得拷贝/改变顺序）', () => {
  const out = filterRowsByPendingFiles(rows, new Set())
  assert.equal(out, rows, '应返回同一个数组引用（零成本短路）')
})

test('进行中文件的**所有**分页子记录一并剔除', () => {
  // k1 有 P1/P2 两条：不能只挡 P1 留 P2，那样用户会看到半份文件
  const out = filterRowsByPendingFiles(rows, new Set(['k1']))
  assert.deepEqual(out.map((r: any) => r.rowKey), ['b1', 'c1'])
})

test('多个进行中文件同时剔除', () => {
  const out = filterRowsByPendingFiles(rows, new Set(['k1', 'k2']))
  assert.deepEqual(out.map((r: any) => r.rowKey), ['c1'])
})

test('不相关文件的行不受影响', () => {
  const out = filterRowsByPendingFiles(rows, new Set(['k9']))
  assert.equal(out.length, 4, '集合里没有的文件不得被剔除')
})

test('knowledgeId 缺失的行保留（避免整表被清空）', () => {
  // 占位行可能没有 knowledgeId；按「空 ID 不匹配任何 pending」处理，
  // 否则一旦 pendingFiles 里混入空串就会把整表清空（真机风险）。
  const withEmpty = [...rows, { rowKey: 'd1', knowledgeId: '', invoiceNo: '441' }]
  const out = filterRowsByPendingFiles(withEmpty, new Set(['k1']))
  assert.deepEqual(out.map((r: any) => r.rowKey), ['b1', 'c1', 'd1'])
})

test('pendingFiles 里混入空串时不得清空整表', () => {
  // 这是最容易踩的坑：如果实现用 `ids.has(String(r.knowledgeId || ''))`，
  // pending 集合里恰好有 '' 就会把所有缺 ID 的行都剔掉。
  const withEmpty = [...rows, { rowKey: 'd1', knowledgeId: '', invoiceNo: '441' }]
  const out = filterRowsByPendingFiles(withEmpty, new Set(['', 'k1']))
  assert.ok(out.length >= 2, '不得把缺 ID 的行全部剔掉')
  assert.deepEqual(out.map((r: any) => r.rowKey), ['b1', 'c1', 'd1'])
})
