/**
 * 第 17 轮：打印目录「超多列」排版。
 *
 * 真机故障（重庆星达 20+ 列）：表格右边界越过 A4 横向纸张，
 * 「用气量」被裁掉一半、「燃气费」整列丢失；`596168` 被拆成 `59616` + `8`。
 *
 * 这里用**假字体度量**（中文 1em、ASCII 0.5em）验证纯函数行为：
 *   · 总宽严格等于内容区宽度（任何列数都不越界）；
 *   · 列数越多字号/内边距越小；
 *   · 数字串绝不在中间断行；
 *   · 长数字列拿到足够宽度单行显示。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  fitColumnWidths,
  metricsForColumnCount,
  tokenizeForWrap,
  wrapNoBreakNumbers,
  MIN_FONT_SIZE,
} from './catalogLayout'

/** A4 横向内容区：841.89 - 32*2 */
const AVAIL = 841.89 - 64

/** 假度量：中文/全角 1em，ASCII 0.5em。 */
const measure = (s: string, size: number) => {
  let w = 0
  for (const ch of String(s)) w += ch.charCodeAt(0) > 127 ? size : size * 0.5
  return w
}

const HEADERS_20 = [
  '账单周期', '使用单位', '分摊比例', '工业电量', '工业电费', '工业均价',
  '宿舍电量', '宿舍电费', '工业用水量', '工业水费', '宿舍水量', '宿舍水费',
  '用气量', '气费', '市场化购电费', '系统运行费用', '政府性基金及附加',
  '合计应付', '备注', '燃气费',
]

test('20 列：总宽严格等于纸张内容区，最右列不越界', () => {
  const cells = Array.from({ length: 16 }, (_, r) =>
    HEADERS_20.map((_, i) => (i % 3 === 0 ? String(596168 + r) : i % 3 === 1 ? (433433.07 + r).toFixed(2) : '重庆星达')),
  )
  const fit = fitColumnWidths({ avail: AVAIL, headers: HEADERS_20, cells, measure })
  const sum = fit.widths.reduce((a, b) => a + b, 0)
  assert.equal(fit.widths.length, 20)
  assert.ok(Math.abs(sum - AVAIL) < 0.05, `总宽必须等于内容区：${sum} vs ${AVAIL}`)
  assert.ok(sum <= AVAIL + 0.05, '绝不允许超出纸张内容区')
  assert.ok(fit.widths.every((w) => w > 0), '不得出现非正列宽')
})

test('22 列：仍然不越界（旧实现 22×40=880 > 777.89）', () => {
  const headers = [...HEADERS_20, '消防水费', '上年同期']
  const cells = Array.from({ length: 8 }, () => headers.map(() => '123456.78'))
  const fit = fitColumnWidths({ avail: AVAIL, headers, cells, measure })
  const sum = fit.widths.reduce((a, b) => a + b, 0)
  assert.ok(sum <= AVAIL + 0.05, `22 列不得越界：${sum}`)
  assert.ok(Math.abs(sum - AVAIL) < 0.05)
})

test('30 列：极端列数也锁死在内容区内', () => {
  const headers = Array.from({ length: 30 }, (_, i) => `字段${i + 1}`)
  const cells = Array.from({ length: 4 }, () => headers.map((_, i) => (i % 2 ? '98765.43' : '2026-09')))
  const fit = fitColumnWidths({ avail: AVAIL, headers, cells, measure })
  const sum = fit.widths.reduce((a, b) => a + b, 0)
  assert.ok(sum <= AVAIL + 0.05, `30 列不得越界：${sum}`)
  assert.ok(fit.metrics.cellSize >= MIN_FONT_SIZE, '字号不得低于下限')
})

test('列数越多字号与内边距越小（12 列以上进入 6~7.5pt）', () => {
  const small = metricsForColumnCount(6)
  const mid = metricsForColumnCount(13)
  const big = metricsForColumnCount(20)
  const huge = metricsForColumnCount(26)
  assert.ok(small.cellSize > mid.cellSize, '13 列必须比 6 列小')
  assert.ok(mid.cellSize > big.cellSize, '20 列必须比 13 列小')
  assert.ok(big.cellSize >= huge.cellSize)
  assert.ok(mid.cellSize <= 7.5 && mid.cellSize >= 6, `13 列字号应落在 6~7.5pt：${mid.cellSize}`)
  assert.ok(big.padX <= 3, `20 列左右内边距应 ≤3pt：${big.padX}`)
})

test('长数字列拿到足够宽度：596168 / 433433.07 单行完整显示', () => {
  const headers = ['账单周期', '本期电量', '本期电费', '户名']
  const cells = [['2026-09', '596168', '433433.07', '重庆星达铜业有限公司']]
  const fit = fitColumnWidths({ avail: AVAIL, headers, cells, measure })
  const idxQty = 1
  const idxFee = 2
  const needQty = measure('596168', fit.metrics.cellSize)
  const needFee = measure('433433.07', fit.metrics.cellSize)
  assert.ok(fit.textWidths[idxQty] >= needQty, `电量列宽不足：${fit.textWidths[idxQty]} < ${needQty}`)
  assert.ok(fit.textWidths[idxFee] >= needFee, `电费列宽不足：${fit.textWidths[idxFee]} < ${needFee}`)
  // 折行后必须仍是一行
  assert.equal(wrapNoBreakNumbers((s) => measure(s, fit.metrics.cellSize), '596168', fit.textWidths[idxQty]).length, 1)
  assert.equal(wrapNoBreakNumbers((s) => measure(s, fit.metrics.cellSize), '433433.07', fit.textWidths[idxFee]).length, 1)
})

test('数字串切分：596168 / 433,433.07 / 21.24% 各自是一个不可断 token', () => {
  assert.deepEqual(tokenizeForWrap('596168'), ['596168'])
  assert.deepEqual(tokenizeForWrap('433,433.07'), ['433,433.07'])
  assert.deepEqual(tokenizeForWrap('21.24%'), ['21.24%'])
  assert.deepEqual(tokenizeForWrap('电量596168度'), ['电', '量', '596168', '度'])
})

test('数字串绝不在中间断行（旧故障：59616 + 8）', () => {
  // 故意给一个只够 5 位数的宽度
  const narrow = measure('59616', 8.5)
  const lines = wrapNoBreakNumbers((s) => measure(s, 8.5), '596168', narrow)
  assert.deepEqual(lines, ['596168'], `不得把 596168 拆行：${JSON.stringify(lines)}`)
  assert.ok(!lines.some((l) => /^\d{1,5}$/.test(l)), '不得出现单字断行')
})

test('表头允许换行但不丢字（拼回去必须等于原文）', () => {
  const fit = fitColumnWidths({
    avail: AVAIL,
    headers: HEADERS_20,
    cells: [HEADERS_20.map(() => '1')],
    measure,
  })
  HEADERS_20.forEach((label, i) => {
    const lines = wrapNoBreakNumbers((s) => measure(s, fit.metrics.headerSize), label, fit.textWidths[i])
    assert.equal(lines.join(''), label, `表头「${label}」换行后丢字：${lines.join('|')}`)
  })
})
