// 第 73 轮前端单测：源码契约回归
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const DIR = path.dirname(fileURLToPath(import.meta.url))
const read = (rel: string) => fs.readFileSync(path.join(DIR, rel), 'utf8')

test('第73轮：住宿记录工具栏=住宿状态筛选且默认在住', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const DORM_STAY_STATUS_OPTIONS = \[/)
  assert.match(src, /const dormStayStatusFilter = ref\('在住'\)/)
  assert.match(src, /v-model="dormStayStatusFilter"/)
  // 旧的房源类型筛选已从住宿记录移除
  assert.ok(!/v-model="dormRoomTypeFilter"/.test(src), '住宿记录不应再有房源类型筛选')
  assert.match(src, /String\(r\.住宿状态 \|\| '在住'\)\.trim\(\) === dormStayStatusFilter\.value/)
})

test('第73轮：房源类型列改普通文字（无彩色胶囊）', () => {
  const src = read('HrManagement.vue')
  const seg = src.slice(src.indexOf("template #房源类型"), src.indexOf("template #房源类型") + 400)
  assert.ok(!/t-tag/.test(seg), '房源类型不得再用 t-tag')
  assert.match(seg, /<span v-else>\{\{ colValue\(row, '房源类型'\) \}\}<\/span>/)
})

test('第73轮：安排在住房源候选用具体房号（宿舍编号 / 房屋编号）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const pubNo = String\(r\['房屋编号'\] \|\| ''\)\.trim\(\)/)
  assert.match(src, /label: `\$\{pubNo\}（\$\{addr \|\| '公租房'\}，可用 \$\{free\}）`/)
  assert.match(src, /value: `\$\{addr\}\$\{pubNo\}`/)
  // 安排入住函数内部不再以「房屋地址」当 value（会导致多行重复的「两山景苑」）
  const allocSeg = src.slice(src.indexOf('const dormAvailableRoomOptions'), src.indexOf('async function loadDormHelpers'))
  assert.ok(!/value: r\['房屋地址'\] \|\| r\['宿舍编号'\]/.test(allocSeg))
})

test('第73轮：退宿日期/原因仅在「已退宿」时可编辑', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const isStayResigned = computed/)
  assert.match(src, /=== '已退宿'/)
  assert.match(src, /<template #field-退宿日期>[\s\S]{0,300}:disabled="!isStayResigned"/)
  assert.match(src, /<template #field-退宿原因>[\s\S]{0,300}:disabled="!isStayResigned"/)
})

test('第73轮：资产档案点行弹「资产分配明细」，编辑迁到单选浮动栏', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /data-hr-asset-alloc-dialog/)
  assert.match(src, /const ASSET_ALLOC_COLUMNS = \[/)
  for (const col of ['宿舍编号', '管理员/舍长', '所属部门', '房源类型', '资产名称', '数量', '计量单位', '资产价值']) {
    assert.ok(new RegExp(`'${col}'`).test(src), `资产分配明细缺列 ${col}`)
  }
  assert.match(src, /if \(activeTab\.value === 'hr-dorm' && dormSubView\.value === 'asset' && row\) \{\s*\n\s*openAssetAlloc\(row\)/)
  assert.match(src, /const assetSingleSelected = computed/)
  assert.match(src, /v-if="assetSingleSelected"[\s\S]{0,160}data-hr-asset-edit/)
})

test('第73轮：费用弹窗标签顺序与默认激活（扣款明细最后且默认）', () => {
  const src = read('HrManagement.vue')
  const order = ['value="dorm"', 'value="public"', 'value="social"', 'value="fund"', 'value="personal"']
    .map((k) => src.indexOf(k, src.indexOf('data-hr-dorm-bill-tabs')))
  assert.ok(order.every((v, i) => v > 0 && (i === 0 || v > order[i - 1])), `标签顺序错误：${order}`)
  assert.match(src, /const dormBillTab = ref<[^>]*>\('personal'\)/)
})

test('第73轮：费用表头去单位后缀', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const trimBillTitle = \(t: any\) => String\(t \?\? ''\)/)
  assert.match(src, /title: trimBillTitle\(c\.title\)/)
  for (const bad of ["电费(元)", "水费(元)", "水电合计(元)", "天然气费(元)", "缴费基数(元)"]) {
    assert.ok(!src.includes(`title: '${bad}'`), `表头仍带单位：${bad}`)
  }
})

test('第73轮：设置抽屉自动定位当前子标签（社保/公积金）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /if \(activeTab\.value === 'hr-social'\) \{[\s\S]{0,220}receipt: '回盘表'/)
  assert.match(src, /abnormal: '补退记录'/)
  assert.match(src, /return fundSubView\.value === 'bills' \? '月度账单' : '职工明细'/)
})

test('第73轮：工具栏筛选默认不可清空 + 统一尺寸', () => {
  const bar = read('BusinessFilterBar.vue')
  assert.match(bar, /:clearable="f\.clearable === true" class="bfs-select"/)
  assert.match(bar, /\.bfs-field--keyword \{ width: 220px; \}/)
  assert.match(bar, /\.bfs-select \{ width: 140px; flex-shrink: 0; \}/)
  assert.match(bar, /\.bfs-month \{ width: 140px; flex-shrink: 0; \}/)
})

test('第73轮：能耗核算默认最近有数据月份；工业表计极简', () => {
  const src = read('UtilitiesManagement.vue')
  assert.match(src, /async function ensureLatestMonth\(\)/)
  assert.match(src, /if \(months\.length\) monthFilter\.value = String\(months\[0\]\)/)
  // 第 74 轮：工业表计从「专属 Tab 文案映射」升级为顶层平级标签
  assert.match(src, /const settingsTab = computed<EnergySettingsTab>/)
  const meter = read('MeterSettingsPanel.vue')
  assert.ok(!/kindManageVisible = true/.test(meter), '工业表计不得再有用途管理入口')
  assert.ok(!/:header="'用途管理/.test(meter), '用途管理弹窗必须删除')
  assert.ok(!/搜索别名 \/ 表号/.test(meter), '工业表计不得再有搜索框')
  assert.ok(!/<t-tabs/.test(meter), '工业表计不得再有子标签栏（t-tabs）')
})
