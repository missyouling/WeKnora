/**
 * 车队「上传文件」口径（第 39 轮）。
 *
 * 真机事故：
 *   · 车辆档案「历史记录」卡片 2 份，抽屉「全部文件 13」；
 *   · 抽屉里 3 个文件在解析/提取，列表上方却没有绿色状态条；
 *   · 维保记录「已收录 1 份」，但「一般维修 0 + 合格证 0」。
 * 前两个的根因就是三处各写一套判定。这里把判定收敛成一个纯函数并锁死。
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import {
  fleetDocTypeOf, fleetHistoryFilesOf, isFleetFileRunning, fleetRunningCountOf, fleetFailedCountOf,
} from './fleetHistoryStats'
import { buildFleetKpiCards } from './fleetColumns'

const file = (docType: string, parse: string, extract = '', extra: Record<string, any> = {}) => ({
  id: `${docType}-${parse}-${extract}`,
  parse_status: parse,
  custom_metadata: { doc_type: docType, extract_status: extract, ...extra },
})

test('fleetDocTypeOf：doc_type 优先，历史文件回退 fleet_cert_type', () => {
  assert.equal(fleetDocTypeOf({ custom_metadata: { doc_type: '行驶证' } }), '行驶证')
  assert.equal(fleetDocTypeOf({ custom_metadata: { fleet_cert_type: '保险单' } }), '保险单')
  assert.equal(fleetDocTypeOf({}), '')
})

test('fleetHistoryFilesOf：与抽屉「全部文件」同源（剔除隐藏 + 按证照类型归属菜单）', () => {
  const all = [
    file('行驶证', 'completed', 'success'),
    file('保险单', 'completed', 'success'),
    file('驾驶证', 'completed', 'success'),
    file('行驶证', 'completed', 'success', { fleet_history_hidden: true }),
  ]
  const vehicle = fleetHistoryFilesOf(all, new Set(['行驶证', '保险单']))
  assert.equal(vehicle.length, 2, '隐藏文件剔除、驾驶证不属于车辆档案')
  // 类型缺失的历史文件不能被丢掉（旧实现按 scope 过滤时整批消失）
  const legacy = { id: 'x', parse_status: 'completed', custom_metadata: { extract_status: 'success' } }
  assert.equal(fleetHistoryFilesOf([...all.slice(0, 2), legacy], new Set(['行驶证', '保险单'])).length, 3)
})

test('isFleetFileRunning：解析/提取任一阶段未终态都算在途', () => {
  assert.equal(isFleetFileRunning(file('行驶证', 'pending')), true)
  assert.equal(isFleetFileRunning(file('行驶证', 'parsing')), true)
  assert.equal(isFleetFileRunning(file('行驶证', 'processing')), true)
  assert.equal(isFleetFileRunning(file('行驶证', 'completed', 'processing')), true)
  // 解析完成但从未提取 → 待提取，也算在途（否则状态条不出现）
  assert.equal(isFleetFileRunning(file('行驶证', 'completed', '')), true)
  assert.equal(isFleetFileRunning(file('行驶证', 'completed', 'success')), false)
  assert.equal(isFleetFileRunning(file('行驶证', 'completed', 'failed')), false)
  assert.equal(isFleetFileRunning(file('行驶证', 'failed')), false)
})

test('fleetRunningCountOf / fleetFailedCountOf：状态条与卡片副文案的计数', () => {
  const all = [
    file('行驶证', 'pending'),
    file('行驶证', 'completed', 'processing'),
    file('行驶证', 'completed', 'success'),
    file('行驶证', 'completed', 'failed'),
  ]
  assert.equal(fleetRunningCountOf(all), 2)
  assert.equal(fleetFailedCountOf(all), 1)
})

test('KPI：历史记录卡片数字 = 抽屉全部文件（不再用台账行数）', () => {
  const cards = buildFleetKpiCards({
    isArchive: true,
    total: 2,                       // fleet_records 行数（旧卡片误用这个）
    categories: [{ name: '行驶证' }, { name: '保险单' }],
    subCounts: { 行驶证: 1, 保险单: 1 },
    subAmounts: {},
    historyCount: 13,               // 抽屉「全部文件」
    running: 3,
  })
  const history = cards.find((c) => c.key === 'history')!
  assert.equal(history.value, '13', '必须等于抽屉的全部文件数')
  assert.equal(history.sub, '正在执行 3', '有在途时副文案显示进度')
  // 无在途、有异常 → N 份异常；都正常 → 无异常文件
  const ok = buildFleetKpiCards({
    isArchive: true, total: 2, categories: [], subCounts: {}, subAmounts: {}, historyCount: 13,
  })
  assert.equal(ok.find((c) => c.key === 'history')!.sub, '无异常文件')
  const bad = buildFleetKpiCards({
    isArchive: true, total: 2, categories: [], subCounts: {}, subAmounts: {}, historyCount: 13, parseFailed: 2,
  })
  assert.equal(bad.find((c) => c.key === 'history')!.sub, '2 份异常')
})
