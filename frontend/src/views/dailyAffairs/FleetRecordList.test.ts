import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const list = readFileSync(new URL('./FleetRecordList.vue', import.meta.url), 'utf8')
const bar = readFileSync(new URL('../../components/business/BusinessParseStatusBar.vue', import.meta.url), 'utf8')

// 第 28 轮：三段式重构后，进度横幅改由中台 BusinessParseStatusBar 承载，
// 列表页只负责把 uploadProgress / pendingFiles 喂进去。

test('解析/提取进度横幅走中台组件（不再自绘 .archive-pending-bar）', () => {
  assert.match(list, /<BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" \/>/)
  assert.doesNotMatch(list, /archive-pending-bar/)
})

test('displayProgress 只喂「当前轮播位」一条（中台组件负责多文件轮播）', () => {
  assert.match(list, /const displayProgress = computed/)
  assert.match(list, /uploadProgress\.value\[progressIndex\.value\] \|\| uploadProgress\.value\[0\]/)
  assert.match(list, /elapsedSec: 0/)
})

test('上传进度轮播：3 秒切换，单任务停止', () => {
  assert.match(list, /function onUploadProgress/)
  assert.match(list, /setInterval\(\(\) => \{/)
  assert.match(list, /\}, 3000\)/)
  assert.match(list, /progressIndex\.value = \(progressIndex\.value \+ 1\) % uploadProgress\.value\.length/)
  assert.match(list, /if \(uploadProgress\.value\.length <= 1\)/)
  assert.match(list, /clearInterval\(progressTimer\)/)
})

test('页面卸载时清理轮播与轮询定时器', () => {
  assert.match(list, /stopPolling\(\)/)
  assert.match(list, /if \(progressTimer\) \{ clearInterval\(progressTimer\); progressTimer = null \}/)
  assert.match(list, /onBeforeUnmount\(\(\) => \{/)
})

test('设置抽屉变更分类后同步刷新：监听全局事件，卸载时移除', () => {
  assert.match(list, /addEventListener\('fleet-categories-changed', onCatChanged\)/)
  assert.match(list, /removeEventListener\('fleet-categories-changed', onCatChanged\)/)
  assert.match(list, /async function reloadCategories\(\)/)
  assert.match(list, /function onCatChanged\(\)/)
})

// ---- 第 28 轮零重复造轮子硬断言（Phase 5 静态检查的镜像） ----
test('不出现任何自绘 KPI / 浮条 / 合计条 / 打印弹窗 / 旧格式化函数', () => {
  for (const bad of [
    'stat-card', 'type-card', 'meter-print-mask', 'doc-summary-bar',
    'doc-batch-bar-fixed', 'archive-overview', 'fmtMoney', 'fmtNum', 'fmtVal',
    'RECORD_COLS', 'BILLING_COLS', 'isBilling',
  ]) {
    assert.equal(list.includes(bad), false, `不得再出现 ${bad}`)
  }
  // 第 34 轮口径变更：**必须**用与发票管理同构的 `:max-height="'100%'"`。
  //
  // 旧断言（第 28 轮）写的是「不得设 max-height，内层滚动会让懒加载失效」，
  // 前提是「表格靠滚动到底触发懒加载」。但车队页早已改为**分页**
  // （`total > pageSize` 时出分页器），不存在滚动触发加载；
  // 而合计行贴底恰恰需要容器限高 —— TDesign 的 tfoot 是
  // `position: sticky; bottom: 0`，只有滚动容器有高度上限时它才贴底。
  // 发票 / 能耗的标准实现就是 `.doc-list-view` + `max-height:100%` +
  // `sticky-header` 三件套，车队必须逐字对齐，不再自创占位行。
  assert.match(list, /:max-height="tableMaxHeight"/, '表格必须用与发票同构的 max-height')
  assert.match(list, /const tableMaxHeight = computed\(\(\) => '100%'\)/)
  // 自创的像素占位行已彻底移除（它会造成首行拉高、双横线、满页多撑一屏）
  assert.equal(list.includes('--fleet-spacer-h'), false, '不得再出现自创的 --fleet-spacer-h 占位')
  assert.equal(list.includes('measureSpacer'), false, '不得再出现 measureSpacer 自算表格高度')
  assert.equal(list.includes('fleet-table--sparse'), false, '不得再出现 sparse 表格分支')
})

test('三段式中台组件全部接入', () => {
  for (const comp of [
    '<DashboardKpiGroup', '<BusinessParseStatusBar', '<BusinessFilterBar',
    '<BusinessSplitDetailDrawer', '<StandardSettingDrawer',
    '<PrintPreviewDialog', '<BusinessEmptyState',
  ]) {
    assert.ok(list.includes(comp), `缺少中台组件 ${comp}`)
  }
})

test('批量操作只走 #batch-actions 插槽（页面不自写浮条）', () => {
  assert.match(list, /<template #batch-actions>/)
  for (const act of ['重新解析', '重新提取', '打印目录', '删除']) {
    assert.ok(list.includes(act), `批量操作缺少 ${act}`)
  }
})

test('门禁：pending 文件的行不进列表', () => {
  assert.match(list, /filterRowsByPendingFiles/)
  assert.match(list, /const pendingIds = computed/)
})

test('合计行勾选感知（走 listSummary，非累加列留空）', () => {
  assert.match(list, /buildFootRow/)
  assert.match(list, /summaryScopeRows/)
  assert.match(list, /footSpanOf/)
})

test('十二个菜单全部登记到分类 scope（无硬编码字段）', () => {
  for (const key of [
    'vehicle-archive', 'driver-archive', 'maintain-archive', 'tire', 'inspection',
    'fuel-charge', 'road-toll', 'repair-cost', 'insurance-claim', 'violation',
    'material', 'car-request',
  ]) {
    // 含连字符的 key 在对象字面量里带引号（'vehicle-archive':），两种形态都认
    assert.ok(list.includes(`'${key}':`) || list.includes(`${key}:`), `菜单 ${key} 未登记`)
  }
  // 列/抽屉字段全部由 subs 派生
  assert.match(list, /buildFleetColumns\(activeSubs\.value/)
  assert.match(list, /drawerFields/)
  // 第 30 轮：左侧工具栏只留「搜索全部字段」+ 刷新，不再派生任何字段筛选器
  // （证照类型下拉、车牌、日期区间、子分类字段筛选全部移除）。
  assert.match(list, /type: 'keyword'/)
  assert.ok(!list.includes('buildFleetFilterSpecs('), '工具栏不应再派生字段筛选器')
})

test('中台进度横幅本身仍保留 3 秒轮播实现', () => {
  assert.match(bar, /setInterval\(\(\) => \{/)
  assert.match(bar, /\}, 3000\)/)
})
