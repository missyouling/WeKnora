import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const list = readFileSync(new URL('./FleetRecordList.vue', import.meta.url), 'utf8')
// 第 44 轮：两个状态派生列的定义搬到 fleetColumns.buildArchiveColumns（可单测），
// 页面只消费。列宽断言因此改为读纯函数源文件。
const cols = readFileSync(new URL('./fleetColumns.ts', import.meta.url), 'utf8')
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

// ---------------------------------------------------------------------------
// 第 37 轮：车队列表行贴顶 / 证照状态三色 Tag / 内置不可删
// ---------------------------------------------------------------------------

test('表格：禁止任何拉高 table 或推 tbody 的自定义 CSS（首行必须贴表头）', () => {
  const src = list
  const style = src.slice(src.indexOf('<style'), src.lastIndexOf('</style>'))
  // 这三条是「第 1 行悬空 250px」的直接成因
  // 先剥掉注释再判：说明性注释里会提到这些写法（"不能用 flex:1…"），
  // 不剥会把注释误判成真实规则。
  const stripComments = (t: string) => t.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  const flat = (t: string) => t.replace(/\s+/g, ' ').replace(/\s*([{}:])\s*/g, '$1')
  const css = flat(stripComments(style))
  assert.ok(!/\.t-table__content>table\{[^}]*height:100%/.test(css),
    'table{height:100%} 会把多余高度分给行，唯一那行被拉到整屏高')
  assert.ok(!/\.t-table__footer\{[^}]*margin-top:auto/.test(css),
    'tfoot{margin-top:auto} 会把合计顶到底，配合上面就是「行飘在半空」')
  assert.ok(!/\.t-table__content\{[^}]*flex:1/.test(css),
    'content{flex:1} 让滚动容器拉满，是拉高的前提')
  assert.ok(!/tbody::after|spacer/i.test(style), '不得用伪元素占位行顶合计栏')
})

test('证照状态：列表渲染三色 Tag（模板插槽，非 cell 函数）', () => {
  const src = list
  // 第 37 轮：TDesign 1.19 的 cell 回调不注入 h，`_h('t-tag')` 会渲染成空白，
  // 必须走具名插槽 #__cert_status（与发票页同款）。
  const slot = src.slice(src.indexOf('template #__cert_status'), src.indexOf('template #__extract_status'))
  assert.match(slot, /<t-tag/, '证照状态插槽必须渲染 t-tag')
  assert.match(slot, /certThemeOf\(row\)/, '主题必须走 certThemeOf（三态映射）')
  assert.match(slot, /variant="light"/, '浅底深字（light 变体）')
  // 三态映射函数本体
  const fn = src.slice(src.indexOf('const certThemeOf'), src.indexOf('const columnDefs'))
  assert.match(fn, /'success'/, '生效中 → success（绿）')
  assert.match(fn, /'warning'/, '即将到期 → warning（橙）')
  assert.match(fn, /'danger'/, '已过期 → danger（红）')
  // 不得再出现自造的 _h 渲染（剥掉注释再判：说明文字里会提到这种错误写法）
  const codeOnly = src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  assert.ok(!/_h\('t-tag'/.test(codeOnly), 'cell 回调里不能用 _h(\'t-tag\')，TDesign 不注入 h')
})

test('证照状态 / 提取状态列宽 ≥ 100px，表头不被截断', () => {
  const src = cols
  for (const key of ['CERT_STATUS_KEY', 'EXTRACT_STATUS_KEY']) {
    const i = src.indexOf(`key: ${key}`)
    assert.ok(i > 0, `缺少 ${key} 列定义`)
    const seg = src.slice(i, src.indexOf('},', i))
    assert.match(seg, /minWidth:\s*100/, `${key} 必须 minWidth:100`)
  }
})

test('抽屉：证照状态与提取状态都是只读 Tag，不是 input', () => {
  const src = list
  const start = src.indexOf('template #field-__cert_status')
  assert.ok(start > 0, '缺少证照状态抽屉插槽')
  const certSlot = src.slice(start, src.indexOf('template #field-__extract_status'))
  assert.match(certSlot, /<t-tag/, '证照状态插槽必须是 t-tag')
  assert.ok(!/<t-input/.test(certSlot), '证照状态不得用 input（disabled 输入框看起来像漏填）')
  const extSlot = src.slice(src.indexOf('template #field-__extract_status'), src.indexOf('</BusinessSplitDetailDrawer>'))
  assert.match(extSlot, /<t-tag/, '提取状态插槽必须是 t-tag')
  // 两者都必须在 drawerFields 里注册，否则插槽不会渲染
  const fields = src.slice(src.indexOf('const drawerFields'), src.indexOf('const drawerSubFields'))
  assert.match(fields, /CERT_STATUS_KEY/, 'drawerFields 必须含证照状态')
  assert.match(fields, /EXTRACT_STATUS_KEY/, 'drawerFields 必须含提取状态')
})

// ---------------------------------------------------------------------------
// 第 38 轮：取消冻结列 / 合计行贴底骨架 / 驾驶证首屏直出
// ---------------------------------------------------------------------------

test('列表列定义不得再有 fixed（冻结列会遮住后方业务列文字）', () => {
  // 真机图 44/46：勾选列 + 序号列 fixed:'left' → 横向滚动时钉在左侧并画分割线，
  // 许可证号被压成「订字5002…」。
  const cols = list.slice(list.indexOf('const tableColumns'), list.indexOf('const vis = visibleCols.value'))
  // 先剥注释：说明文字里会提到 `fixed: 'left'` 这种历史写法
  const colCode = cols.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  assert.ok(!colCode.includes('fixed'), '列定义不得再有 fixed（左/右固定都会遮住后方列）')
  // 勾选 + 序号是 TDesign 的内置列类型，去掉 fixed 后仍要保留
  assert.match(cols, /colKey: 'row-select'/, '复选框列必须保留')
  assert.match(cols, /colKey: 'serial-number'/, '序号列必须保留')
})

test('切一级菜单时先同步重置子分类焦点，再拉列表（否则首屏 0 条）', () => {
  const bootRaw = list.slice(list.indexOf('async function bootstrap()'), list.indexOf("watch(() => props.recordType"))
  // 剥注释再定位：说明文字里也写了 `reloadList()`，会把「第一次调用」判断带偏
  const boot = bootRaw.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '')
  const resetAt = boot.indexOf('activeType.value = seeded')
  const listAt = boot.indexOf('reloadList()')
  assert.ok(resetAt > 0, 'bootstrap 必须用同步种子重置 activeType')
  assert.ok(listAt > resetAt, '重置必须发生在第一次 reloadList 之前（时序错了就是 0 条）')
  // 服务端分类校正焦点后要补一次列表请求
  assert.match(boot, /if \(want !== activeType\.value\)[\s\S]{0,220}reloadList\(\)/,
    '服务端分类校正后必须重拉列表，不能等用户点刷新')
})

test('子分类过滤兼容英文 key（driver_license / driving_license）', () => {
  assert.match(list, /subCategoryMatches\(r\?\.sub_category/, '必须用别名匹配兜底历史英文 key')
})

test('列显隐跟着菜单/分类走：STORAGE_KEY 变化即重新读该分类的列记忆', () => {
  // 真机：车辆档案 → 司机档案，visibleColKeys 仍是上一个分类的键，
  // 与新的 columnDefs 求交集只剩「有效期起/止/发证机关」→ 表头少 7 列。
  assert.match(list, /watch\(STORAGE_KEY, \(\) => \{ initColumns\(\) \}\)/,
    '必须 watch STORAGE_KEY 重新 initColumns')
})
