import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const dialogSrc = readFileSync(new URL('./FleetUploadDialog.vue', import.meta.url), 'utf8')
const requestSrc = readFileSync(new URL('../../utils/request.ts', import.meta.url), 'utf8')

test('批量上传削峰：上传通道限并发，提取通道严格单飞', () => {
  // 上传：不得再一次性 forEach 全量并发（20 份同时上传 → 同时提取 → 429/1302）
  assert.doesNotMatch(dialogSrc, /queued\.forEach\(\(t\) => \{ void runTask\(t\) \}\)/)
  assert.match(dialogSrc, /const UPLOAD_CONCURRENCY = 2/)
  assert.match(dialogSrc, /const lanes = Math\.min\(UPLOAD_CONCURRENCY, queued\.length\)/)
  // 提取：单飞闸门（extractWorkerRunning）必须保留
  assert.match(dialogSrc, /function drainExtractQueue\(\)/)
  assert.match(dialogSrc, /if \(extractWorkerRunning\) return/)
})

test('重复文件 409 静默：request 层标注 silent，调用方按业务分支处理', () => {
  assert.match(requestSrc, /const ERR_CONFLICT = 409;/)
  assert.match(requestSrc, /conflictCode === 'duplicate_file'/)
  assert.match(requestSrc, /silent: true/)
  // 调用方已有重复文件自愈分支（不再把 409 当故障抛红色异常）
  assert.match(dialogSrc, /handleDuplicateUpload\(t, recovery, existingId\)/)
})
