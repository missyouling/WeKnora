import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { useBusinessListSync, pageKeyOf, DEFAULT_REFRESH_DELAYS, type SyncRow } from './useBusinessListSync'

const src = readFileSync(new URL('./useBusinessListSync.ts', import.meta.url), 'utf8')

interface Row extends SyncRow { name: string }

const mkRows = (): { value: Row[] } => ({
  value: [
    { knowledgeId: 'f1', page: 1, name: 'f1-1' },
    { knowledgeId: 'f1', page: 2, name: 'f1-2' },
    { knowledgeId: 'f1', page: 3, name: 'f1-3' },
    { knowledgeId: 'f2', page: 1, name: 'f2-1' },
    { knowledgeId: 'f3', name: 'f3-nopage' },
  ],
})

const noop = () => {}

test('乐观移除：整文件命中优先，单页按 knowledgeId::page 命中', () => {
  const rows = mkRows()
  const sync = useBusinessListSync(rows, { reload: noop })
  sync.removeRows(new Set(['f2']), new Set([pageKeyOf('f1', 2)]))
  assert.deepEqual(rows.value.map((r) => r.name), ['f1-1', 'f1-3', 'f3-nopage'])
})

test('无 page 的历史行只可能被整文件命中（不会误删）', () => {
  const rows = mkRows()
  const sync = useBusinessListSync(rows, { reload: noop })
  sync.removeRows(new Set(), new Set([pageKeyOf('f3', 1)]))
  assert.equal(rows.value.length, 5, 'f3 没有 page，单页键不应命中它')
  sync.removeRows(new Set(['f3']), new Set())
  assert.equal(rows.value.length, 4)
})

test('空集合不触发任何变更（避免无谓重渲染）', () => {
  const rows = mkRows()
  const before = rows.value
  const sync = useBusinessListSync(rows, { reload: noop })
  sync.removeRows(new Set(), new Set())
  assert.equal(rows.value, before, '引用未变')
})

test('deletePages：同文件多页先删大页码（后端删后重排页码）', async () => {
  const rows = mkRows()
  const calls: string[] = []
  const sync = useBusinessListSync(rows, {
    reload: noop,
    deletePage: async (kid, page) => { calls.push(`${kid}#${page}`); return {} },
  })
  await sync.deletePages(rows.value.filter((r) => r.knowledgeId === 'f1'))
  assert.deepEqual(calls, ['f1#3', 'f1#2', 'f1#1'])
  assert.deepEqual(rows.value.map((r) => r.name), ['f2-1', 'f3-nopage'])
})

test('deletePages：后端返回 deleted_file 时整文件并入删除，随后整行消失', async () => {
  const rows = mkRows()
  const files: string[][] = []
  const sync = useBusinessListSync(rows, {
    reload: noop,
    deletePage: async () => ({ deleted_file: true }),
    deleteFiles: async (ids) => { files.push(ids) },
  })
  await sync.deletePages(rows.value.filter((r) => r.knowledgeId === 'f1'))
  assert.deepEqual(files, [['f1']])
  assert.deepEqual(rows.value.map((r) => r.name), ['f2-1', 'f3-nopage'])
})

test('deletePages：无页码的行走整文件删除', async () => {
  const rows = mkRows()
  const files: string[][] = []
  const sync = useBusinessListSync(rows, {
    reload: noop,
    deleteFiles: async (ids) => { files.push(ids) },
  })
  await sync.deletePages(rows.value.filter((r) => r.knowledgeId === 'f3'))
  assert.deepEqual(files, [['f3']])
  assert.equal(rows.value.length, 4)
})

test('deletePages 失败：回调带上下文且不吞掉异常（页面据此提示并中止）', async () => {
  const rows = mkRows()
  const errs: any[] = []
  const sync = useBusinessListSync(rows, {
    reload: noop,
    deletePage: async () => { throw new Error('boom') },
    onError: (e, ctx) => errs.push({ msg: e.message, ...ctx }),
  })
  await assert.rejects(() => sync.deletePages(rows.value.filter((r) => r.knowledgeId === 'f1')))
  assert.equal(errs.length, 1)
  assert.equal(errs[0].page, 3, '第一个删的是最大页码')
  assert.equal(errs[0].knowledgeId, 'f1')
  // 失败不落地乐观移除
  assert.equal(rows.value.length, 5)
})

test('deleteFiles：去重后整批删除并移除该文件所有行', async () => {
  const rows = mkRows()
  const files: string[][] = []
  const sync = useBusinessListSync(rows, {
    reload: noop,
    deleteFiles: async (ids) => { files.push(ids) },
  })
  await sync.deleteFiles(rows.value.filter((r) => r.knowledgeId === 'f1'))
  assert.deepEqual(files, [['f1']], '同文件多行只删一次')
  assert.deepEqual(rows.value.map((r) => r.name), ['f2-1', 'f3-nopage'])
})

test('延迟对账：两次刷新（800/2600），pending 到最后一次才落', async () => {
  const times: number[] = []
  const sync = useBusinessListSync(mkRows(), {
    reload: async (silent) => { times.push(Date.now()); assert.equal(silent, true) },
    refreshDelays: [10, 20],
  })
  assert.equal(sync.syncPending(), false)
  sync.schedulePostDeleteRefresh()
  assert.equal(sync.syncPending(), true)
  await new Promise((r) => setTimeout(r, 60))
  assert.equal(times.length, 2, '两次权威对账')
  assert.equal(sync.syncPending(), false)
})

test('重复调度会清掉上一轮定时器（不会叠加刷新风暴）', async () => {
  let n = 0
  const sync = useBusinessListSync(mkRows(), { reload: async () => { n += 1 }, refreshDelays: [10, 20] })
  sync.schedulePostDeleteRefresh()
  sync.schedulePostDeleteRefresh()
  sync.schedulePostDeleteRefresh()
  await new Promise((r) => setTimeout(r, 60))
  assert.equal(n, 2, '三次调度只应留下最后一轮的两刷')
})

test('destroy 清理定时器：卸载后不再刷新已销毁的页面', async () => {
  let n = 0
  const sync = useBusinessListSync(mkRows(), { reload: async () => { n += 1 }, refreshDelays: [10, 20] })
  sync.schedulePostDeleteRefresh()
  sync.destroy()
  await new Promise((r) => setTimeout(r, 60))
  assert.equal(n, 0)
  assert.equal(sync.syncPending(), false)
})

test('默认对账延迟与注释一致（800/2600）', () => {
  assert.deepEqual(DEFAULT_REFRESH_DELAYS, [800, 2600])
  assert.match(src, /worker 调度/)
})

test('中台不认识任何业务 scope（全部走回调注入）', () => {
  assert.doesNotMatch(src, /scope\s*===/)
  // 不直接 import 任何业务 API，也不直接调用发票删除接口
  assert.doesNotMatch(src, /from '@\/api/)
  assert.doesNotMatch(src, /deleteInvoicePage\(/)
  // 对外只有回调注入点
  assert.match(src, /deletePage\?:/)
  assert.match(src, /deleteFiles\?:/)
})

test('发票页已切换使用 listSync（删除编排不再内联）', () => {
  const page = readFileSync(new URL('./InvoiceManagement.vue', import.meta.url), 'utf8')
  assert.match(page, /useBusinessListSync\(invoiceRows/)
  assert.match(page, /listSync\.destroy\(\)/)
  assert.doesNotMatch(page, /deleteRefreshTimers/)
})
