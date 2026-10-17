import assert from 'node:assert/strict'
import test from 'node:test'
import {
  EXTRACT_IN_PROGRESS_REASON,
  conflictReasonOf,
  httpStatusOf,
  isExtractInProgress,
} from './extractConflict'
import { readFileSync } from 'node:fs'

/** 复刻 request.ts 注入方式：非枚举属性，spread/JSON.stringify 不可见。 */
function withStatus(payload: Record<string, unknown>, status: number) {
  return Object.defineProperty(payload, '$httpStatus', {
    value: status,
    enumerable: false,
    configurable: true,
    writable: false,
  })
}

test('提取在途 409（新版后端带 details.reason）判为并发冲突', () => {
  const err = withStatus({
    success: false,
    error: {
      code: 1005,
      message: 'extraction already in progress for this document',
      details: { reason: EXTRACT_IN_PROGRESS_REASON, knowledge_id: 'k1', scope: 'invoice' },
    },
  }, 409)
  assert.equal(httpStatusOf(err), 409)
  assert.equal(conflictReasonOf(err), EXTRACT_IN_PROGRESS_REASON)
  assert.equal(isExtractInProgress(err), true)
})

test('旧后端/网关抹掉 details 时按「提取接口 409 只有一种语义」兜底判为在途', () => {
  const err = withStatus({ success: false, error: { code: 1005, message: 'extraction already in progress for this document' } }, 409)
  assert.equal(conflictReasonOf(err), undefined)
  assert.equal(isExtractInProgress(err), true)
})

test('非 409（模型 500 / 参数 400 / 网络错误）不判为并发冲突', () => {
  assert.equal(isExtractInProgress(withStatus({ error: { message: 'boom' } }, 500)), false)
  assert.equal(isExtractInProgress(withStatus({ error: { message: 'bad scope' } }, 400)), false)
  assert.equal(isExtractInProgress({ message: 'Network Error' }), false)
  assert.equal(isExtractInProgress(null), false)
  assert.equal(isExtractInProgress(undefined), false)
})

test('未来若出现非「在途」语义的 409（如单号查重）不误判为在途', () => {
  const err = withStatus({
    error: { code: 1005, message: 'DUPLICATE_INVOICE_NO:已存在相同的发票号码 123', details: { reason: 'duplicate_no' } },
  }, 409)
  assert.equal(isExtractInProgress(err), false)
})

test('各调用点统一走 isExtractInProgress，不再散落 $httpStatus === 409', () => {
  const files = [
    '../views/dailyAffairs/InvoiceManagement.vue',
    '../views/dailyAffairs/ContractManagement.vue',
    '../views/dailyAffairs/RegulationManagement.vue',
    '../views/dailyAffairs/AwardPunishManagement.vue',
    '../views/dailyAffairs/FleetUploadDialog.vue',
    '../views/dailyAffairs/FleetUploadHistoryDrawer.vue',
    '../views/dailyAffairs/FleetRecordList.vue',
  ]
  for (const rel of files) {
    const src = readFileSync(new URL(rel, import.meta.url), 'utf8')
    assert.ok(src.includes('@/composables/extractConflict'), `${rel} 应导入 extractConflict`)
    assert.ok(src.includes('isExtractInProgress('), `${rel} 应调用 isExtractInProgress`)
    assert.doesNotMatch(src, /\$httpStatus\s*[=!]==\s*409/, `${rel} 不应再有裸 409 判定`)
  }
  const polling = readFileSync(new URL('./useBusinessPolling.ts', import.meta.url), 'utf8')
  assert.ok(polling.includes('isExtractInProgress('))
  assert.doesNotMatch(polling, /\$httpStatus\s*[=!]==\s*409/)
})

test('in-flight 防重集合跨页面刷新持久化（刷新后不再重复提交撞 409）', () => {
  const polling = readFileSync(new URL('./useBusinessPolling.ts', import.meta.url), 'utf8')
  // 快照落盘 + 模块初始化回填，缺一则刷新后防重失效
  assert.match(polling, /sessionStorage\.setItem\(FLIGHT_STORAGE_KEY/)
  assert.match(polling, /sessionStorage\.getItem\(FLIGHT_STORAGE_KEY/)
  assert.match(polling, /loadFlightSnapshot\(\)/)
  // 必须有 TTL：否则异常残留会让某个文件永久无法再自动提取
  assert.match(polling, /FLIGHT_TTL_MS/)
  const dialog = readFileSync(new URL('../views/dailyAffairs/FleetUploadDialog.vue', import.meta.url), 'utf8')
  assert.ok(dialog.includes('saveFlightSnapshot'), '上传弹窗提交后应立即落盘快照')
})
