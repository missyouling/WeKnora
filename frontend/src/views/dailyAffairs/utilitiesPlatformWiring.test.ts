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

// ---- 本轮：取消侧边栏 + 7 张卡片 + 汇总栏入框 ----

test('取消左侧边栏：页面为单栏全宽布局', () => {
  assert.doesNotMatch(src, /class="utilities-sidebar"/, '左侧竖向侧边栏必须彻底删除')
  assert.doesNotMatch(src, /class="utilities-layout"/, '侧边栏+内容的双列布局容器必须删除')
  assert.doesNotMatch(src, /utilities-side-item/, '侧边栏菜单项必须删除')
  // 根容器必须是四模块同款骨架，主区与列表容器逐项对齐发票管理
  assert.match(src, /class="utilities-management-container biz-page"/, '根容器必须挂 .biz-page')
  assert.match(src, /class="electricity-panel biz-main"/, '主区必须挂 .biz-main')
  assert.match(src, /class="biz-list-view"/, '列表视图容器必须挂 .biz-list-view')
})

test('顶部 KPI 卡片为 6 业务卡 + 1 历史记录卡，无「已收录账单」', () => {
  // 6 个侧边栏菜单 1:1 转为卡片，末卡历史记录
  const labels = [...src.matchAll(/label: '([^']+)'/g)].map(m => m[1])
  for (const name of ['市电账单', '光伏账单', '电费核算', '水费核算', '气费核算', '费用清单', '历史记录']) {
    assert.ok(labels.includes(name), `KPI 卡片缺少「${name}」`)
  }
  assert.ok(!labels.includes('已收录账单'), '「已收录账单」卡片必须删除')
  // 6 个业务卡用 action=type + typeValue 驱动高亮；历史记录卡不参与高亮
  assert.match(src, /action: 'type'/, '业务卡片必须是 action=type 才能随当前 tab 高亮')
  assert.match(src, /typeValue:/, '业务卡片必须带 typeValue 才能与 activeTab 比对')
  assert.match(src, /:active-type="activeTab"/, 'KPI 组必须接收当前 tab 作为高亮锚点')
})

test('点击前 6 张卡片切换列表视图', () => {
  assert.match(src, /onKpiCardClick\s*=\s*\(|function onKpiCardClick/, '必须有卡片点击处理')
  assert.match(src, /switchTab\(/, '点击业务卡片必须调用 switchTab 切换 tab')
  assert.match(src, /uploadHistoryVisible\.value = true/, '点击历史记录卡片必须打开上传历史抽屉')
})

test('底部汇总栏位于列表白色卡片容器内部', () => {
  // .doc-list-footer-summary 必须出现在 .doc-list-scroll 开标签之后、其闭标签之前
  const open = src.indexOf('<div class="doc-list-scroll"')
  assert.ok(open > 0, '缺少 .doc-list-scroll 滚动容器')
  const summaryIdx = src.indexOf('doc-list-footer-summary')
  assert.ok(summaryIdx > open, '汇总栏必须在 .doc-list-scroll 之后出现（即位于其内部）')
  // 汇总栏之后、模板结尾前必须还有一个 </div> 收口 scroll 容器
  const after = src.slice(summaryIdx)
  assert.match(after, /<\/div>\s*<\/div>/, '汇总栏之后必须收口 .doc-list-scroll')
  // 复用中台 doc-summary-bar 的粘性底栏样式类
  assert.match(src, /class="[^"]*doc-summary-bar/, '汇总栏应复用中台 doc-summary-bar 样式')
})

test('表头吸顶不穿透：sticky-header + 实测遮罩变量', () => {
  assert.match(src, /sticky-header/, 't-table 必须开启 sticky-header，否则表头被滚走')
  // 遮罩本体已收敛到共享样式（business-list-shared.css 的 .doc-list-view::after），
  // 页面只负责把实测值绑到 .doc-list-view 上。
  assert.match(src, /:style="headerFixStyle"/, '必须把实测的滚动条宽/表头高绑到 .doc-list-view')
  assert.match(src, /useTableHeaderFix/, '必须使用统一的表头量测 composable')
})

// ---- 本轮：KPI 卡片栏必须在各 tab 模板之外 ----

test('KPI 卡片栏位于各 tab 模板之外（切卡片不会丢导航）', () => {
  // 曾经把 DashboardKpiGroup 放在 electricity tab 内：切到光伏/核算/清单时整栏消失，
  // 用户失去顶部导航，只能刷新页面回来（真机踩过）。
  const kpi = src.indexOf('<DashboardKpiGroup')
  const tabOpen = src.indexOf('<template v-if="activeTab === \'electricity\'">')
  assert.ok(kpi > 0, '缺少 DashboardKpiGroup')
  assert.ok(tabOpen > 0, '缺少 electricity tab 模板')
  assert.ok(kpi < tabOpen, 'KPI 卡片栏必须在 electricity tab 模板之前（即所有 tab 共享）')
})

test('卡片计数在 kbId 就绪后再拉（否则全部为 0）', () => {
  // 账单/光伏接口以 kbId 为参数，kbId 由 loadKb() 异步解析；
  // 先跑 loadTabCounts 会让 6 张卡片全显示 0（真机踩过）。
  assert.match(src, /loadKb\(\)\.then\(\(\)\s*=>\s*loadTabCounts\(\)\)/, '计数必须等 kbId 解析完成')
  assert.doesNotMatch(src, /loadKb\(\); void loadTabCounts\(\)/, '不得与 loadKb 并行跑计数')
})

// ---- 账单详情页：标题栏精简 + 右侧无底层大框 ----

test('详情页标题栏不得再挂「提取成功」状态标签', () => {
  const head = /<div class="bill-detail-head">([\s\S]*?)<\/div>\s*\n\s*<div class="bill-detail-body">/.exec(src)
  assert.ok(head, '缺少 bill-detail-head 区块')
  assert.doesNotMatch(head![1], /<t-tag/, '标题栏不得再有 t-tag（提取成功徽标）')
  assert.doesNotMatch(head![1], /row-status-tag/, '标题栏不得再挂状态标签类名')
  // 返回按钮与文件名必须保留
  assert.match(head![1], /账单列表/, '返回按钮必须保留')
  assert.match(head![1], /bill-detail-title/, '文件名标题必须保留')
})

test('右侧内容区不得有底层大框（border / 背景 / 内边距）', () => {
  // .bill-content 曾经有 border + #fff 背景 + 16px 内边距 → 4 张指标卡被套在大白框里，
  // 既「框套框」又多吃掉 34px 纵向空间，顶部还与左侧菜单不齐平。
  const css = /\.bill-content\s*\{([\s\S]*?)\n\}/.exec(src)
  assert.ok(css, '缺少 .bill-content 样式')
  const body = css![1]
  assert.doesNotMatch(body, /^\s*border:/m, '.bill-content 不得有边框')
  assert.doesNotMatch(body, /^\s*background:/m, '.bill-content 不得有背景色')
  assert.doesNotMatch(body, /^\s*padding:/m, '.bill-content 不得有内边距')
  // 仍要保留卡片间距与独立滚动兜底
  assert.match(body, /margin-top:\s*12px/, '卡片间距应为 12px')
})
