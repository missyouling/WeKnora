import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 能耗管理第 3 轮真机问题的回归契约（2026-10-27）。
 *
 * 五个真机现象各自的根因都很具体，重犯的代价又很高（用户逐张截图指出），
 * 因此这里用源码断言把它们钉死：
 *   ① 工具栏筛选不生效 —— BusinessFilterBar 非 immediate 控件只发
 *      `update:modelValue`，列表若不监听筛选态就永远不会重算；
 *   ② 分时表新增记录不带上期止度 —— 只索引 end_reading 填不到四段起度；
 *   ③ 分时明细可同时展开多行 —— 展开态用了 Set；
 *   ④ 电表名称被前置箭头挤得不对齐 —— 展开符必须在名称右侧；
 *   ⑤ 费用清单打印混表 —— 水费与电费必须各自成表，且抽屉只到「备注」。
 */

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

test('核算列表：筛选态变化必须触发本地重算（不依赖 filter-change 事件）', () => {
  const src = read('UtilityMeterTab.vue')
  assert.match(src, /watch\(filters, \(\) => applyFilters\(\), \{ deep: true \}\)/, '筛选项变化必须即时重算列表')
  assert.match(src, /if \(filters\.value\.useUnit\) list = list\.filter/, '使用单位必须参与过滤')
  assert.match(src, /if \(filters\.value\.month\) list = list\.filter/, '月份必须参与过滤')
})

test('核算列表：分时明细是手风琴（单一 expandedKey），展开符在名称右侧', () => {
  const src = read('UtilityMeterTab.vue')
  assert.match(src, /const expandedKey = ref<string \| null>\(null\)/)
  assert.doesNotMatch(src, /expandedKeys/, '旧的多行展开 Set 必须彻底移除')
  assert.match(src, /expandedKey === row\.key \? '−' : '\+'/, '展开符必须是 +/− 而不是三角')
  // 电表名称必须在展开符之前（先文字后徽标），否则名称首字会被徽标顶开
  const meterCell = src.slice(src.indexOf('#meter='), src.indexOf('#start_reading='))
  assert.ok(
    meterCell.indexOf('row-label') < meterCell.indexOf('row-expand-toggle'),
    '电表名称文字必须排在展开符之前',
  )
})

test('核算列表：分时明细按主表实测列宽渲染，时段电量不带单位', () => {
  const src = read('UtilityMeterTab.vue')
  assert.match(src, /measureTableColumns/)
  assert.match(src, /<col v-for="\(c, i\) in expandCols"/, '子表必须复用主表列宽')
  assert.match(src, /case 'usage': return fmtNum\(p\.usage\)/, '时段电量只显示数值')
  assert.match(src, /const DETAIL_HEAD[\s\S]*meter: '时段'[\s\S]*start_reading: '起度'[\s\S]*end_reading: '止度'[\s\S]*rate: '倍率'[\s\S]*usage: '时段电量'/)
})

test('新增记录：上期止度按「最近历史周期」回填四段起度', () => {
  const src = read('UtilityMeterTab.vue')
  assert.match(src, /function prevSnapshotOf/, '必须有最近历史周期的回溯查找')
  assert.match(src, /deepPrev = prev\?\.deep_curr/, '分时表尖段起度取上期止度')
  assert.match(src, /valleyPrev = prev\?\.valley_curr/)
  assert.match(src, /form\.value\.startReading = prev\?\.end_reading \?\? 0/, '普通表起度取上期止度')
})

test('分时电表单价必须是禁用态，且不参与金额计算', () => {
  const drawer = read('MeterRecordDrawer.vue')
  assert.match(drawer, /unitPriceLocked/)
  assert.match(drawer, /:disabled="unitPriceLocked\(f\.key\)"/)
  const tab = read('UtilityMeterTab.vue')
  assert.match(tab, /if \(isTimeMeter\.value\) return 0/, '分时表不按单一标准单价算金额')
  assert.match(tab, /unit_price: isTimeMeter\.value \? 0 :/)
})

test('费用清单抽屉：只渲染字段，不得追加电量/费用/水费明细块', () => {
  const src = read('TenantBilling.vue')
  assert.doesNotMatch(src, /<template #extra>/, '抽屉不得再追加明细插槽')
  assert.doesNotMatch(src, /bsd-block-title">电量明细/)
  assert.match(src, /readonly/, '详情抽屉必须只读')
})

test('费用清单打印：概览 + 分项细项 + 水电气分表 + 汇总', () => {
  const src = read('TenantBilling.vue')
  assert.match(src, /generateStatementPdf\(\{\s*doc:/, '必须走结构化文档模型')
  assert.match(src, /function feeTableOf/)
  assert.match(src, /function elecTableOf/)
  assert.match(src, /function waterTableOf/)
  assert.match(src, /function summaryTableOf/)
  assert.match(src, /overviewTitle: '一、账单概览'/)
  // 水费与电费必须是两张表（函数级别分离），不能合成一张
  assert.ok(src.indexOf('function elecTableOf') < src.indexOf('function waterTableOf'))
})

test('清单 PDF 引擎：标题不重叠、支持概览指标与分区表格', () => {
  const src = read('useCatalogPdf.ts')
  assert.match(src, /overviewTitle\?: string/)
  assert.match(src, /export interface StatementTable/)
  // 页标题与内容之间必须有明确下推：旧版 y -= 6 导致两个标题叠印
  assert.match(src, /y -= S_TITLE_SIZE \+ 10/)
  assert.doesNotMatch(src, /y -= 6\b/)
  assert.match(src, /const pages = pdf\.getPages\(\)/, '页脚页码需要总页数')
})

test('市电/光伏：月份与关键词都在本地过滤，汇总按可见行统计', () => {
  for (const f of ['UtilitiesManagement.vue', 'SolarManagement.vue']) {
    const src = read(f)
    assert.match(src, /if \(month\)/, `${f} 月份必须在本地过滤`)
    assert.match(src, /const summaryCount = computed/, `${f} 底部条数必须与可见行同口径`)
  }
  assert.match(read('SolarManagement.vue'), /billMonthOfItem\(r\) === month/)
})

test('四个能耗视图统一使用同一个打印预览组件', () => {
  for (const f of ['TenantBilling.vue', 'UtilityMeterTab.vue', 'UtilitiesManagement.vue', 'SolarManagement.vue']) {
    assert.match(read(f), /PrintPreviewDialog/, `${f} 必须复用统一打印预览壳子`)
  }
})
