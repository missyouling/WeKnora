// 概览指标卡与图表配置的单测（第 24 轮任务一瘦身）。
//
// 这组断言锁的是**展示口径**：箭头方向、颜色档、空值处理、除零边界。
// 这些量此前写在 40 多个 computed 里，只能靠源文本断言「某行还在」；
// 抽成纯函数后可以逐项断言输出。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import { buildEnergyChartOption, ENERGY_CHART_HEIGHT } from './utilityEnergyChart'
import {
  avgPriceCard,
  buildBillMetrics,
  compareExtremes,
  metricDueText,
  metricFee,
  metricKwh,
  metricPeriodText,
  momCard,
  pfCard,
} from './utilityBillMetrics'
import { buildEnergyCompareRows, pickPrevEnergy } from './utilityBillDetail'

const f = {
  kwh: (v: any) => `${Math.round(Number(v) * 100) / 100}`,
  money: (v: any) => `${Math.round(Number(v) * 100) / 100}`,
}

const noPrev = pickPrevEnergy([], '')
const withPrev = pickPrevEnergy(
  [
    {
      item: {
        bill_period_start: '2026-09-01',
        total_kwh: 800,
        total_amount: 500,
        power_factor: 0.9,
        avg_price: 0.625,
        deep_peak_kwh: 100,
        peak_kwh: 200,
        flat_kwh: 400,
        valley_kwh: 100,
      },
    },
  ],
  '2026-10-01',
)

// ---- 基础文本 ----

test('metricPeriodText：取 start，缺失回落 end，都没有给 —', () => {
  assert.equal(metricPeriodText({ bill_period_start: '2026-10-01' }), '2026-10')
  assert.equal(metricPeriodText({ bill_period_end: '2026-10-31' }), '2026-10')
  assert.equal(metricPeriodText({}), '—', '空值必须给 — 而不是空串（否则卡片塌陷）')
})

test('metricDueText：取 due_date，缺失回落 pay_deadline', () => {
  assert.equal(metricDueText({ due_date: '2026-10-25' }), '2026-10-25')
  assert.equal(metricDueText({ pay_deadline: '2026-10-28' }), '2026-10-28')
  assert.equal(metricDueText({}), '—')
})

// ---- 本期电量 ----

test('metricKwh：无上期时 delta=0 且提示「暂无上期数据」', () => {
  const v = metricKwh({ total_kwh: 1000 }, noPrev, f)
  assert.equal(v.text, '1000')
  assert.equal(v.delta.delta, 0)
  assert.equal(v.delta.sub, '持平')
  assert.equal(v.delta.tip, '暂无上期数据')
})

test('metricKwh：上涨给 ↑ + up 色档，副文案带 + 号', () => {
  const v = metricKwh({ total_kwh: 1000 }, withPrev, f)
  assert.equal(v.delta.delta, 200)
  assert.equal(v.delta.arrow, '↑')
  assert.equal(v.delta.tone, 'up')
  assert.equal(v.delta.cls, 'ea-delta--up')
  assert.equal(v.delta.sub, '+200 千瓦时')
})

test('metricKwh：下降给 ↓ + down 色档，副文案带 - 号', () => {
  const v = metricKwh({ total_kwh: 600 }, withPrev, f)
  assert.equal(v.delta.delta, -200)
  assert.equal(v.delta.arrow, '↓')
  assert.equal(v.delta.cls, 'ea-delta--down')
  assert.equal(v.delta.sub, '-200 千瓦时')
})

test('metricKwh：持平给 — 箭头 + flat 色档', () => {
  const v = metricKwh({ total_kwh: 800 }, withPrev, f)
  assert.equal(v.delta.arrow, '—', '概览卡的持平用破折号（与用能分析卡不同）')
  assert.equal(v.delta.cls, 'ea-delta--flat')
  assert.equal(v.delta.sub, '持平')
})

test('metricKwh：悬浮说明必须带公式，便于对数', () => {
  const v = metricKwh({ total_kwh: 1000 }, withPrev, f)
  assert.match(v.delta.tip, /环比 = \(本期 − 上期\) ÷ 上期 = 25\.00%/)
})

// ---- 本期电费 ----

test('metricFee：以传入的 curFee 为准（不自己重算）', () => {
  const v = metricFee(700, withPrev, f)
  assert.equal(v.text, '700')
  assert.equal(v.delta.delta, 200)
  assert.equal(v.delta.sub, '+200 元')
})

test('metricFee：上期为 0 时环比百分比给 0（不除零）', () => {
  const zeroPrev = { ...noPrev, exists: true, fee: 0 }
  const v = metricFee(100, zeroPrev, f)
  assert.match(v.delta.tip, /0\.00%/, '上期电费为 0 时百分比收敛成 0，不得出现 NaN')
})

// ---- 用能分析环比卡（口径与概览卡不同）----

test('momCard：任一侧为 0 时显示 —（除零无意义）', () => {
  assert.equal(momCard({ total_kwh: 0 }, withPrev, f).text, '—')
  assert.equal(momCard({ total_kwh: 1000 }, noPrev, f).text, '—', '无上期时也显示 —')
})

test('momCard：有上期时显示百分比（保留 1 位）', () => {
  const v = momCard({ total_kwh: 1000 }, withPrev, f)
  assert.equal(v.text, '+25%', '1000 vs 800 → +25.0% → 去掉尾零显示 +25%')
})

test('momCard：下跌时百分比带负号', () => {
  const v = momCard({ total_kwh: 600 }, withPrev, f)
  assert.equal(v.text, '-25%')
})

test('momCard：持平箭头给空串（不是破折号）', () => {
  const v = momCard({ total_kwh: 800 }, withPrev, f)
  assert.equal(v.delta.arrow, '', '用能分析卡持平不画箭头，避免与「无数据」混淆')
})

// ---- 功率因数 ----

test('pfCard：0 是合法读数，不得当空值显示 —', () => {
  // 关键边界：用 `!v` 判空会把 0 显示成 —（功率因数确实可以是 0）
  const v = pfCard({ power_factor: 0 }, { ...withPrev, power_factor: 0 }, )
  assert.equal(v.text, '0', 'power_factor=0 必须显示 0')
})

test('pfCard：空值给 —', () => {
  assert.equal(pfCard({ power_factor: null }, withPrev).text, '—')
  assert.equal(pfCard({ power_factor: '' }, withPrev).text, '—')
  assert.equal(pfCard({}, withPrev).text, '—')
})

test('pfCard：差值取 3 位小数（业务精度）', () => {
  const v = pfCard({ power_factor: 0.955 }, withPrev)
  assert.equal(v.delta.delta, 0.055)
  assert.equal(v.delta.sub, '0.06', '副文案取 2 位（0.055 → 0.06）')
})

test('pfCard：两侧都为 0 才算无数据', () => {
  const v = pfCard({ power_factor: 0 }, { ...noPrev, power_factor: 0 })
  assert.equal(v.delta.tip, '暂无上期数据')
  assert.equal(v.delta.delta, 0)
})

// ---- 平均电价 ----

test('avgPriceCard：有 avg_price 直接用', () => {
  assert.equal(avgPriceCard({ avg_price: 0.6153, total_kwh: 1000 }, 700, noPrev).text, '0.6153')
})

test('avgPriceCard：无 avg_price 时用 电费÷电量 反算 4 位', () => {
  assert.equal(avgPriceCard({ total_kwh: 1000 }, 615.3, noPrev).text, '0.6153')
})

test('avgPriceCard：电量为 0 时给 —（不除零）', () => {
  assert.equal(avgPriceCard({ total_kwh: 0 }, 700, noPrev).text, '—')
})

test('avgPriceCard：差值取 4 位小数', () => {
  const v = avgPriceCard({ avg_price: 0.6255, total_kwh: 1000 }, 700, withPrev)
  assert.equal(v.delta.delta, 0.0005)
  assert.equal(v.delta.sub, '0.0005 元/千瓦时')
})

// ---- 汇总 ----

test('buildBillMetrics：一次给齐 5 张卡 + 周期/截止', () => {
  const m = buildBillMetrics(
    { total_kwh: 1000, bill_period_start: '2026-10-01', due_date: '2026-10-25' },
    700,
    withPrev,
    f,
  )
  assert.equal(m.period, '2026-10')
  assert.equal(m.due, '2026-10-25')
  assert.equal(m.kwh.text, '1000')
  assert.equal(m.fee.text, '700')
  assert.equal(m.mom.text, '+25%')
  assert.ok(m.pf.text)
  assert.ok(m.avgPrice.text)
})

// ---- 图表配置 ----

const colors = {
  text: '#111', axis: '#222', grid: '#333',
  brand: '#07c05f', brandWeak: '#aae', brandDeep: '#046',
}

const chartRows = buildEnergyCompareRows(
  { total_kwh: 1000, deep_peak_kwh: 100, peak_kwh: 200, flat_kwh: 500, valley_kwh: 200 },
  withPrev,
)

test('buildEnergyChartOption：配色全部来自传入的主题色', () => {
  const opt: any = buildEnergyChartOption(chartRows, true, colors)
  assert.equal(opt.legend.textStyle.color, colors.text)
  assert.equal(opt.xAxis.axisLine.lineStyle.color, colors.axis)
  assert.equal(opt.yAxis.splitLine.lineStyle.color, colors.grid)
  assert.equal(opt.series[0].itemStyle.color, colors.brand)
  assert.equal(opt.series[1].itemStyle.color, colors.brandWeak)
})

test('buildEnergyChartOption：无上期时第二组给 null（不是 0）', () => {
  const opt: any = buildEnergyChartOption(chartRows, false, colors)
  assert.deepEqual(opt.series[1].data, [null, null, null, null], '画成 0 柱会被误读成上期 0%')
  assert.equal(opt.series[1].label.show, false, '无上期时不显示柱内标签')
})

test('buildEnergyChartOption：横轴是尖/峰/平/谷四段', () => {
  const opt: any = buildEnergyChartOption(chartRows, true, colors)
  assert.deepEqual(opt.xAxis.data, ['尖', '峰', '平', '谷'])
  assert.equal(opt.series[0].data.length, 4)
})

test('buildEnergyChartOption：纵轴上限留 15% 余量并取整到 10 的倍数', () => {
  const opt: any = buildEnergyChartOption(chartRows, true, colors)
  // 本期占比最大 50% → 50 * 1.15 = 57.5 → 向上取整到 60
  assert.equal(opt.yAxis.max({ max: 50 }), 60)
  assert.equal(opt.yAxis.max({ max: 12 }), 20, '12*1.15=13.8 → 20')
})

test('buildEnergyChartOption：上期柱标签用 inside（柱矮时不压边）', () => {
  const opt: any = buildEnergyChartOption(chartRows, true, colors)
  assert.equal(opt.series[1].label.position, 'inside')
  assert.equal(opt.series[0].label.position, 'top')
})

test('ENERGY_CHART_HEIGHT：与模板里 .ea-echart 高度一致', () => {
  // 模板用 160px；容器高度改了小图会被压扁，这里的常量是唯一出口
  assert.equal(ENERGY_CHART_HEIGHT, 160)
})

// ---- 极值 ----

test('compareExtremes：空数组给 0/0（不产生 Infinity）', () => {
  assert.deepEqual(compareExtremes([]), { maxDelta: 0, minDelta: 0 })
})

test('compareExtremes：取最大/最小差值', () => {
  const rows = [
    { key: 'a', label: 'A', cur: 1, curPct: 1, prev: 1, prevPct: 0, delta: 5 },
    { key: 'b', label: 'B', cur: 1, curPct: 1, prev: 1, prevPct: 0, delta: -3 },
    { key: 'c', label: 'C', cur: 1, curPct: 1, prev: 1, prevPct: 0, delta: 0 },
  ] as any
  assert.deepEqual(compareExtremes(rows), { maxDelta: 5, minDelta: -3 })
})
