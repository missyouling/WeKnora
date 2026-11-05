/**
 * 第 36 轮真数据回归：费用清单打印「电费核算明细」起止度 + 本期总电量三段式。
 *
 * 数据取自库内 2026-10 / 2026-09 的真实 `utility_meter_items`：
 *   · 持睿工业分表 1~3 是分时表（meter_type=time），回填后起止度由四段求和得出；
 *   · 电费核算明细表直接读 elecRows 的 start_reading / end_reading。
 *
 * 断言 1：工业分表起止度 > 0 且 (止度 − 起度) × 倍率 == 用电量。
 * 断言 2：本期总电量卡片 hint = `工业 X + 差额 Y + 宿舍 Z`，三段之和 == 大字值。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { buildTenantBillingStatementDoc } from './energyBillStatements'

/**
 * 取 KPI 卡片的防空守卫（第 38 轮，修 CI type-check TS18048）。
 *
 * `StatementDoc.kpis` 在类型上是可选字段（有的单据确实没有概览卡），
 * 直接 `doc.kpis.find(...)` 在 `vue-tsc --build` 下会报 'doc.kpis' is possibly
 * 'undefined'（CI 图 48 的 90,17 / 109,15 两处）。这里统一走一个守卫，
 * 顺带兜住 `doc` 本身为空的极端情况。
 */
const kpisOf = (doc: any): any[] => (Array.isArray(doc?.kpis) ? doc.kpis : [])

// 库内 2026-10 真实读数（回填后 start/end_reading 已落库）
const ELEC_2026_10 = [
  { month: '2026-10', use_unit: '持睿汽车', meter_alias: '持睿工业分表1', meter_kind: 'industrial',
    meter_type: 'time', rate: 200, start_reading: 22188, end_reading: 22635, usage: 89400, unit_price: 0.436, amount: 38978.4 },
  { month: '2026-10', use_unit: '持睿汽车', meter_alias: '持睿工业分表2', meter_kind: 'industrial',
    meter_type: 'time', rate: 150, start_reading: 3382, end_reading: 3763, usage: 57150, unit_price: 0.436, amount: 24917.4 },
  { month: '2026-10', use_unit: '持睿汽车', meter_alias: '持睿工业分表3', meter_kind: 'industrial',
    meter_type: 'time', rate: 200, start_reading: 2104, end_reading: 2204, usage: 20000, unit_price: 0.436, amount: 8720 },
  { month: '2026-10', use_unit: '重庆星达', meter_alias: '宿舍301', meter_kind: 'dorm',
    meter_type: 'normal', rate: 1, start_reading: 8293, end_reading: 8357, usage: 64, unit_price: 0.5, amount: 32 },
]
// 库内 2026-09 真实读数
const ELEC_2026_09 = [
  { month: '2026-09', use_unit: '持睿汽车', meter_alias: '持睿工业分表1', meter_kind: 'industrial',
    meter_type: 'time', rate: 200, start_reading: 21870, end_reading: 22188, usage: 63600, unit_price: 0.436, amount: 27729.6 },
  { month: '2026-09', use_unit: '持睿汽车', meter_alias: '持睿工业分表2', meter_kind: 'industrial',
    meter_type: 'time', rate: 150, start_reading: 3076, end_reading: 3382, usage: 45900, unit_price: 0.436, amount: 20012.4 },
  { month: '2026-09', use_unit: '持睿汽车', meter_alias: '持睿工业分表3', meter_kind: 'industrial',
    meter_type: 'time', rate: 200, start_reading: 2038, end_reading: 2104, usage: 13200, unit_price: 0.436, amount: 5755.2 },
]

/** 构造最小可用的 ctx（只驱动概览卡片与电费核算明细表）。 */
function makeCtx(elecRows: any[], subMeterKwh: number): any {
  return {
    ownerView: false,
    billOf: () => ({ item: { total_kwh: 623932, grand_total: 481647.91 } }),
    recordOf: () => undefined,
    items: [],
    elecRows,
    waterMeters: [],
    waterReadingMap: new Map(),
    gasRows: [],
    ratioOf: () => 0.2123,
    subMeterKwhOf: (_m: string, unit: string) => unit === '持睿汽车' ? subMeterKwh : 0,
    includeGas: false,
    sourceCtxOf: () => ({}) as any,
  }
}

/** 从 doc.tables 找「电费核算明细」表。 */
function elecTableOf(doc: any): any {
  return doc.tables.find((t: any) => String(t.title || '').includes('电费核算明细'))
}

for (const [month, elec, subMeter, indKwh, dormKwh, expInd, expDiff, expDorm, expTotal] of [
  ['2026-10', ELEC_2026_10, 166550, 172400, 686, 166550, 5850, 686, 173086],
  ['2026-09', ELEC_2026_09, 122700, 126600, 930, 122700, 3900, 930, 127530],
] as const) {
  test(`${month} 打印清单：工业分表起止度真实且 (止−起)×倍率=用电量`, () => {
    const doc = buildTenantBillingStatementDoc(makeCtx(elec, subMeter), [
      { month, unit: '持睿汽车', ind_kwh: indKwh, dorm_kwh: dormKwh,
        ind_fee: 100, dorm_fee: 10, res_fee: 0,
        water_ind_usage: 0, water_dorm_usage: 0, water_ind_fee: 0, water_dorm_fee: 0, water_fire_fee: 0, gas_fee: 0 },
    ], '持睿汽车')
    const t = elecTableOf(doc)
    assert.ok(t, '必须有电费核算明细表')
    const industrial = (t.rows as string[][]).filter((r) => String(r[0] || '').includes('工业分表'))
    assert.equal(industrial.length, 3, `工业分表应有 3 行，got ${industrial.length}`)
    for (const r of industrial) {
      const start = Number(String(r[2]).replace(/,/g, ''))
      const end = Number(String(r[3]).replace(/,/g, ''))
      const rate = Number(String(r[4]).replace(/,/g, ''))
      const usage = Number(String(r[5]).replace(/,/g, ''))
      assert.ok(start > 0 && end > 0, `${r[0]} 起止度不能为 0：起=${r[2]} 止=${r[3]}`)
      assert.ok(Math.abs((end - start) * rate - usage) <= 1,
        `${r[0]} (止−起)×倍率 != 用电量：(${end}−${start})×${rate} != ${usage}`)
    }
  })

  test(`${month} 打印清单：本期总电量副文本 = 工业 ${expInd} + 差额 ${expDiff} + 宿舍 ${expDorm}`, () => {
    const doc = buildTenantBillingStatementDoc(makeCtx(elec, subMeter), [
      { month, unit: '持睿汽车', ind_kwh: indKwh, dorm_kwh: dormKwh,
        ind_fee: 100, dorm_fee: 10, res_fee: 0,
        water_ind_usage: 0, water_dorm_usage: 0, water_ind_fee: 0, water_dorm_fee: 0, water_fire_fee: 0, gas_fee: 0 },
    ], '持睿汽车')
    const kpi = kpisOf(doc).find((k: any) => k.label === '本期总电量')
    assert.ok(kpi, '必须有本期总电量卡片')
    const norm = (s: string) => String(s || '').replace(/,/g, '').replace(/\s+/g, ' ').trim()
    assert.equal(norm(kpi!.hint || ''), norm(`工业 ${expInd} + 差额 ${expDiff} + 宿舍 ${expDorm}`),
      `hint 应为三段式，got "${kpi!.hint}"`)
    assert.equal(expInd + expDiff + expDorm, expTotal, '三段之和必须等于本期总电量')
    const big = Number(String(kpi!.value).replace(/[^\d.]/g, ''))
    assert.equal(big, expTotal, `大字值应为 ${expTotal}，got ${kpi!.value}`)
  })
}

test('subMeterKwhOf 缺失时退化为「工业 + 宿舍」两段（向后兼容）', () => {
  const ctx = makeCtx(ELEC_2026_10, 0)
  delete ctx.subMeterKwhOf
  const doc = buildTenantBillingStatementDoc(ctx, [
    { month: '2026-10', unit: '持睿汽车', ind_kwh: 172400, dorm_kwh: 686,
      ind_fee: 100, dorm_fee: 10, res_fee: 0,
      water_ind_usage: 0, water_dorm_usage: 0, water_ind_fee: 0, water_dorm_fee: 0, water_fire_fee: 0, gas_fee: 0 },
  ], '持睿汽车')
  const kpi = kpisOf(doc).find((k: any) => k.label === '本期总电量')
  assert.match(String(kpi!.hint || ''), /工业.*宿舍/, '退化路径仍要有构成说明')
  assert.ok(!String(kpi!.hint || '').includes('差额'), '无分表数据时不应出现差额')
})
