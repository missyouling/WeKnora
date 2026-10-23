import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

/**
 * 能耗列表页中台化契约（本轮 7 项修复的结构锁定）。
 *
 * 真机踩过的坑（上一轮）：
 *   ① 只换了设置抽屉，没接 KPI / 状态条 / 历史抽屉；
 *   ② 工具栏仍是手写插槽，按钮顺序成「字段 → 上传(白底) → 设置 → 历史图标」；
 *   ③ 表格无序号列、两个「账单周期」列、账单周期显示 2026-09-0...；
 *   ④ 搜索只匹配文件名。
 * 这组测试把「正确形态」写死，避免下次改工具栏时又回退。
 */

const src = readFileSync(new URL('./UtilitiesManagement.vue', import.meta.url), 'utf8')

test('接入中台四件套：KPI / 状态条 / 筛选栏 / 历史抽屉', () => {
  assert.match(src, /import DashboardKpiGroup from '@\/components\/business\/DashboardKpiGroup\.vue'/)
  assert.match(src, /import BusinessParseStatusBar from '@\/components\/business\/BusinessParseStatusBar\.vue'/)
  assert.match(src, /import BusinessFilterBar from '\.\/BusinessFilterBar\.vue'/)
  assert.match(src, /import FleetUploadHistoryDrawer from '\.\/FleetUploadHistoryDrawer\.vue'/)
  assert.match(src, /<DashboardKpiGroup[^>]*:cards="overviewCards"/, 'KPI 卡必须挂载')
  assert.match(src, /<BusinessParseStatusBar[^>]*:pending-files="pendingFiles"/, '解析/提取状态条必须接轮询队列')
  assert.match(src, /<BusinessFilterBar/, '工具栏必须走中台筛选栏')
  assert.match(src, /<FleetUploadHistoryDrawer/, '历史抽屉必须复用中台组件')
  // 旧的自写历史抽屉必须下线
  assert.doesNotMatch(src, /DeletedKnowledgeDrawer/, '旧的删除历史抽屉必须移除')
})

test('工具栏按钮顺序与主按钮样式（字段 → 设置 → 上传账单 primary）', () => {
  // 设置与主操作都由 BusinessFilterBar 的 props 驱动：设置在前，primary-action 在后
  assert.match(src, /settings-text="设置"/, '必须有设置按钮')
  assert.match(src, /:primary-action="\{ text: '上传账单', icon: 'upload' \}"/, '上传账单必须是 primary-action')
  // 旧版工具栏里那个历史图标按钮必须彻底消失
  assert.doesNotMatch(src, /t-icon name="history"/, '工具栏不得再有历史图标按钮')
  assert.doesNotMatch(src, /historyVisible/, '旧的历史抽屉开关必须移除')
})

test('搜索为全字段穿透（占位符 + 检索范围）', () => {
  assert.match(src, /search-placeholder="搜索全部字段"/, '占位符必须是「搜索全部字段」')
  assert.match(src, /function rowSearchText/, '必须有行级全字段检索文本')
  for (const key of ['file_name', 'bill_period', 'fee_items', 'tags']) {
    // 覆盖：文件名 / 账单周期 / 费用明细 / 标签
    assert.ok(
      src.includes(key) || src.includes(key.replace('file_name', 'fileName')),
      `检索范围必须覆盖 ${key}`,
    )
  }
})

test('账单周期唯一且为 YYYY-MM', () => {
  // 默认列里只能有一个「账单周期」，且键是派生的 bill_period
  const billingPeriodLabels = (src.match(/label: '账单周期'/g) || []).length
  assert.equal(billingPeriodLabels, 1, '「账单周期」只能出现一列')
  assert.doesNotMatch(src, /label: '账单周期起'/, '不得再有「账单周期起」列')
  assert.doesNotMatch(src, /`\$\{it\.bill_period_start \|\| ''\} ~ \$\{it\.bill_period_end \|\| ''\}`/, '不得再拼接起止范围')
  assert.match(src, /normalizeBillingPeriod\(it\.bill_period\)/, '列表必须用归一化后的账单月份')
})

test('账单概况 20 字段 + 序号列 + 共享表格样式', () => {
  const fallback = /const FALLBACK_COLUMNS: ColDef\[\] = \[([\s\S]*?)\n\]/.exec(src)
  assert.ok(fallback, '缺少默认列定义')
  const keys = [...fallback![1].matchAll(/key: '([^']+)'/g)].map(m => m[1])
  assert.equal(keys.length, 20, `账单概况应为 20 列，实际 ${keys.length}`)
  assert.equal(keys[0], 'bill_period', '第一列必须是账单周期')
  assert.deepEqual(keys, [
    'bill_period', 'account_no', 'account_name', 'usage_category', 'voltage_level',
    'market_attr', 'supply_unit', 'address', 'meter_no', 'total_kwh',
    'total_amount', 'prev_kwh', 'mom_change', 'avg_price', 'power_factor',
    'due_date', 'industrial_amount', 'residential_amount', 'pf_adjust_amount', 'grand_total',
  ])
  // 序号列 + 共享样式
  assert.match(src, /colKey: 'serial-number'/, '必须有序号列')
  assert.match(src, /colKey: 'row-select'/, '复选框列必须用 TDesign 的 row-select（key 写错会导致单元格空白）')
  assert.match(src, /@import '\.\/styles\/business-list-shared\.css';/, '必须引用四模块共享列表样式')
  for (const cls of ['biz-page', 'biz-main', 'biz-list-view']) {
    assert.ok(src.includes(cls), `根/主区/列表容器必须带 .${cls}`)
  }
})
