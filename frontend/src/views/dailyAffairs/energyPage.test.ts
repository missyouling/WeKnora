import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

// 能耗页（EnergyManagement.vue）的中台接入契约断言。
//
// 验收口径（用户要求）：BusinessFilterBar 正确挂载、六同步规则端到端无断链。
// 这里做**源码契约**断言（组件挂载、插槽、筛选类型、告警面板），
// 行为级断言由后端单测与 VM Playwright 真机承担。

const read = (f: string) => readFileSync(new URL(`./${f}`, import.meta.url), 'utf8')
const energySrc = read('EnergyManagement.vue')

test('能耗页接入中台 KPI 卡片，且明确关闭骨架屏加载', () => {
  assert.match(energySrc, /<DashboardKpiGroup/, '必须接入 DashboardKpiGroup')
  assert.match(energySrc, /:loading="false"/, 'KPI 卡片必须 :loading="false"（杜绝骨架屏闪烁）')
  assert.match(energySrc, /已收录能耗单/, '首卡必须是「已收录能耗单」')
})

test('能耗页接入中台筛选工具栏 BusinessFilterBar', () => {
  assert.match(energySrc, /<BusinessFilterBar/, '必须接入 BusinessFilterBar')
  assert.match(energySrc, /v-model="filterState"/, '必须用 filterState 双向绑定')
  assert.match(energySrc, /:fields="filterFields"/, '筛选器必须由 FilterFieldSpec 声明')
  // 旧的独立工具栏不得残留
  assert.doesNotMatch(energySrc, /<BusinessListToolbar/, '不得残留旧版 BusinessListToolbar')
})

test('账期筛选是月粒度（month 类型），不是日粒度 date-range', () => {
  assert.match(energySrc, /type: 'month'/, '账期筛选必须声明 month 类型')
  assert.match(energySrc, /key: 'billingPeriod'/, '账期筛选键必须是 billingPeriod')
  assert.doesNotMatch(energySrc, /type: 'date-range'/, '能耗主时间轴是账期，不得用 date-range')
})

test('账期筛选类型已在中台协议里注册', () => {
  const filterSrc = read('businessFilter.ts')
  assert.match(filterSrc, /'month'/, "FilterFieldType 必须含 'month'")
  assert.match(filterSrc, /month: ''/, 'month 的空值必须是空串（后端不能收到 null）')
  assert.match(filterSrc, /month: 'calendar'/, 'month 必须有默认图标')
})

test('中台筛选器渲染月粒度选择器', () => {
  const barSrc = read('BusinessFilterBar.vue')
  assert.match(barSrc, /f\.type === 'month'/, '必须渲染 month 控件')
  assert.match(barSrc, /mode="month"/, '必须用 month 模式选择器')
  assert.match(barSrc, /format="YYYY-MM"/, '值格式必须是 YYYY-MM')
})

test('能耗页接入中台详情抽屉与设置/历史抽屉', () => {
  assert.match(energySrc, /<BusinessSplitDetailDrawer/, '必须接入 BusinessSplitDetailDrawer')
  assert.match(energySrc, /<StandardSettingDrawer/, '必须接入 StandardSettingDrawer')
  assert.match(energySrc, /<FleetUploadHistoryDrawer/, '必须接入 FleetUploadHistoryDrawer')
  assert.match(energySrc, /scope="energy"/, '设置/历史抽屉的 scope 必须是 energy')
})

test('读数类字段在抽屉里渲染为数字输入框', () => {
  for (const f of ['previous_reading', 'current_reading', 'multiplier', 'usage_amount', 'unit_price', 'total_amount']) {
    assert.match(energySrc, new RegExp(`#field-${f}`), `${f} 必须有专属控件插槽`)
    assert.match(energySrc, new RegExp(`#field-${f}[\\s\\S]{0,200}t-input-number`), `${f} 必须是数字输入框`)
  }
})

test('抽屉 #extra 插槽渲染能耗告警面板', () => {
  assert.match(energySrc, /<template #extra>/, '必须使用 #extra 插槽')
  assert.match(energySrc, /energy-alert-panel/, '必须有告警面板')
  assert.match(energySrc, /用量校验提示/, '必须有用量校验告警标题')
  assert.match(energySrc, /用量波动提示/, '必须有环比暴增告警标题')
})

test('六同步：列表列与抽屉字段都由 subs 派生', () => {
  assert.match(energySrc, /buildColumnsFromSubs\(baseCat\.subs/, '列表列必须由 subs 派生')
  assert.match(energySrc, /buildDetailFieldDefs\(cat\.subs/, '抽屉字段必须由 subs 派生')
  assert.match(energySrc, /appendListSystemColumns/, '必须追加列表系统列')
  // 改名同步：label 以 subs.label 为权威
  assert.match(energySrc, /scopeLabelOf\('energy', n\)/, '标签必须走 registry（改名后实时生效）')
})

test('六同步：设置保存后强制重建列', () => {
  assert.match(energySrc, /@saved="\(\) => reloadColumns\(true\)"/, '设置保存后必须 force 重建列')
  assert.match(energySrc, /reloadColumns\(true\)/, '必须支持 force 重建')
})

test('列表列必须下传 dataType（虚拟滚动动态行高的前提）', () => {
  assert.match(energySrc, /dataType: \(c as any\)\.dataType/, '列对象必须下传 dataType')
  assert.match(energySrc, /:scroll="tableScroll"/, '必须接 tableScroll')
})

test('后端能耗接口与前端 API 对齐', () => {
  const apiSrc = readFileSync(new URL('../../api/knowledge-base/index.ts', import.meta.url), 'utf8')
  assert.match(apiSrc, /export function listEnergyRecords/, '缺少 listEnergyRecords')
  assert.match(apiSrc, /energy-records\?/, '列表路径必须是 /energy-records')
  assert.match(apiSrc, /export function listEnergyOverview/, '缺少 listEnergyOverview')
  assert.match(apiSrc, /energy-records\/overview-stats/, '概览路径不正确')
  assert.match(apiSrc, /period_from/, '必须支持账期区间参数')
})

test('用量环比暴增阈值为 30%', () => {
  assert.match(energySrc, /USAGE_SURGE_THRESHOLD = 0\.3/, '阈值必须是 30%')
})

test('列表列必须用 colKey（TDesign 的列标识字段）', () => {
  // 用 key 会导致「表头渲染但单元格内容全空」：真机踩过，tbody 有 12 个 td
  // 但文本全空、行高 17px，列表看着像没数据。
  assert.match(energySrc, /colKey: c\.key/, '列必须用 colKey')
  assert.doesNotMatch(energySrc, /\n\s*key: c\.key,/, '不得用 key 作为列标识')
  assert.match(energySrc, /colKey: 'row-select'/, '必须有多选列')
})

test('抽屉 form 必须是中台契约结构 {data, remarkParts, items}', () => {
  // 直接传扁平 editForm 会让抽屉读 form.data.xxx 报
  // "Cannot read properties of undefined"（真机踩过，一屏 Vue 错误）。
  assert.match(energySrc, /data: editForm\.value/, '必须包一层 data')
  assert.match(energySrc, /remarkParts: \{\}/, '必须给 remarkParts')
  assert.match(energySrc, /items: Array\.isArray/, '必须给 items（表位明细）')
})

test('列宽计算必须传当前行数据（自适应宽度）', () => {
  assert.match(energySrc, /rows\.map\(\(r: any\) => r\?\.\[c\.key\]\)/, '列宽必须按当前行取值')
})
