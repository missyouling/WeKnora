/**
 * 第 57 / 58 轮回归契约（前端）。
 *
 * 第 57 轮交付了「花名册双标签 + 设置抽屉对齐能耗 + 账单三标签弹窗」，
 * 但当时只有真机断言；本文件把其中**纯函数与源码契约**固化成单测，
 * 防止后续重构（尤其是第 58 轮模板归一化）悄悄改掉口径：
 *
 *   ① 设置抽屉单分类常驻展开（singleCategory）；
 *   ② 加锁字段行严格单行 40px（🔒 与 ⋮ 同格，不得折行）；
 *   ③ 月度账单：一人多条逐行保留，人数按人去重、补缴不计人；
 *   ④ 第 58 轮收口：工具栏按钮顺序 / 弹窗宽度 / Esc / blob 释放 /
 *      导出 loading / 暂停缴费走令牌 / 死抽屉清零。
 */
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { billPeopleCountOf, isAdjustmentRow, siBillPeopleOf, subjectRowSeqOf } from './hrBill'

// 仓库内 .vue 是 CRLF（Windows 侧统一），断言前统一成 LF，避免跨行正则永远不命中。
const read = (name: string) =>
  readFileSync(new URL(`./${name}`, import.meta.url), 'utf8').replace(/\r\n/g, '\n')
const readBiz = (name: string) =>
  readFileSync(new URL(`../../components/business/${name}`, import.meta.url), 'utf8').replace(/\r\n/g, '\n')

// ---------------------------------------------------------------------------
// ① 设置抽屉：单分类常驻展开
// ---------------------------------------------------------------------------

test('singleCategory：FleetCertPanel 声明 prop 且在分类到位后自动展开唯一分类', () => {
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /singleCategory\?: boolean/)
  // 直出模式与单分类模式必须走同一个展开分支，否则设置抽屉首屏是空白
  const hits = panel.match(/directMode\.value \|\| props\.singleCategory/g) || []
  assert.equal(hits.length, 2, 'onMounted 与 watch 两处都要覆盖 singleCategory')
})

test('singleCategory：StandardSettingDrawer 透传，人事页按 true 启用', () => {
  const drawer = readBiz('StandardSettingDrawer.vue')
  assert.match(drawer, /singleCategory\?: boolean/)
  assert.match(drawer, /:single-category="singleCategory"/)
  assert.match(read('HrManagement.vue'), /:single-category="true"/)
})

test('加锁行不换行：🔒 与 ⋮ 同处 .fc-op，行高恒 40px，末列 76px', () => {
  const panel = read('FleetCertPanel.vue')
  // 两者必须在同一个 .fc-op 容器内（此前是两个并列元素，⋮ 掉到第二行）
  const opStart = panel.indexOf('<div class="fc-op">')
  const opEnd = panel.indexOf('</div>', panel.indexOf('</t-popconfirm>', opStart))
  assert.ok(opStart > 0, '.fc-op 容器必须存在')
  const opBlock = panel.slice(opStart, opEnd)
  assert.match(opBlock, /class="fc-used"/, '锁标要在 .fc-op 内')
  assert.match(opBlock, /t-dropdown/, '⋮ 菜单也要在 .fc-op 内')
  // 容器自身禁止折行，且行高与末列宽度锁死
  assert.match(panel, /\.fc-op \{[\s\S]*?flex-wrap: nowrap;[\s\S]*?\}/)
  assert.match(panel,
    /\.field-config-row \{[\s\S]*?grid-template-columns: 24px minmax\(280px, 1fr\) 90px 64px 64px 76px;[\s\S]*?min-height: 40px;/)
})

// ---------------------------------------------------------------------------
// ② 月度账单：一人多行 + 人数去重
// ---------------------------------------------------------------------------

const chenNormal = {
  id: 'si-chen-1', 序号: '1', 姓名: '陈刚', 证件号码: '510232198001011234',
  部门: '库房', 是否补缴: false, 个人小计: '636.58', 单位小计: '1693.24',
}
const chenAdjust = {
  id: 'si-chen-2', 序号: '补缴', 姓名: '陈刚', 证件号码: '510232198001011234',
  部门: '库房', 是否补缴: true, 个人小计: '511.28', 单位小计: '1091.73',
}
const plainRows = Array.from({ length: 74 }, (_, i) => ({
  id: `si-${i}`,
  姓名: `员工${i + 1}`,
  证件号码: `5102321990${String(i).padStart(4, '0')}`,
  是否补缴: false,
}))

test('一人多条：陈刚正常 + 补缴 = 2 行，行序号严格 1:1 不合并', () => {
  const rows = [chenNormal, chenAdjust]
  const seq = subjectRowSeqOf(rows)
  assert.deepEqual(seq, ['1', '2'])
  assert.equal(seq.length, rows.length, '行数必须等于记录数，绝不按人合并')
  assert.equal(isAdjustmentRow(chenAdjust), true)
  assert.equal(isAdjustmentRow(chenNormal), false)
})

test('人数去重：76 行（75 人 + 陈刚补缴）仍算 75 人', () => {
  const rows = [...plainRows, chenNormal, chenAdjust]
  assert.equal(rows.length, 76)
  assert.equal(siBillPeopleOf(rows), 75)
  // 补缴行即使单独存在也不增加人数
  assert.equal(siBillPeopleOf([chenAdjust]), 0)
  // 空值防御
  assert.equal(siBillPeopleOf([]), 0)
  assert.equal(siBillPeopleOf(null), 0)
})

test('公积金人数：同一人两条记录只算 1 人（无补缴概念）', () => {
  const rows = [
    { 姓名: '张三', 证件号码: 'A1' },
    { 姓名: '张三', 证件号码: 'A1' },
    { 姓名: '李四', 证件号码: 'B2' },
  ]
  assert.equal(billPeopleCountOf(rows), 2)
  assert.equal(billPeopleCountOf(undefined), 0)
})

test('HrManagement 明细行必须由 billFiltered 直接 map 出来（不得 groupBy 人）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const billSubjectRows = computed\(\(\) => \{\n {2}const rows = billFiltered\.value/)
  assert.match(src, /return rows\.map\(\(r, i\)/)
  assert.match(src, /const siBillPeople = computed\(\(\) => siBillPeopleOf\(billFiltered\.value\)\)/)
})

// ---------------------------------------------------------------------------
// ③ 第 58 轮：工具栏顺序 / 弹窗宽度 / Esc / blob / loading / 令牌
// ---------------------------------------------------------------------------

test('工具栏顺序：导入与生成账单都在 after-settings，右侧恒为 字段+设置+业务按钮', () => {
  const src = read('HrManagement.vue')
  // 用位置比较，避免被插槽内部嵌套的 </template>（图标插槽）截断
  const afterSettings = src.indexOf('<template #after-settings>')
  const importAt = src.indexOf('data-hr-import')
  const generateAt = src.indexOf('data-hr-generate-bill')
  const barEnd = src.indexOf('</BusinessFilterBar>')
  assert.ok(afterSettings > 0, '必须有 after-settings 插槽')
  assert.ok(afterSettings < importAt && importAt < generateAt && generateAt < barEnd,
    '导入与生成账单都必须排在设置之后、BusinessFilterBar 之内')
  assert.equal(src.includes('<template #right-extra>'), false,
    'right-extra 渲染在设置之前，人事页不得再用它挂业务按钮')
  // BusinessFilterBar 内部顺序：right-extra → 设置 → after-settings → primary
  const bar = read('BusinessFilterBar.vue')
  const trailing = bar.slice(bar.indexOf('<template #right-extra>'))
  assert.ok(trailing.indexOf('<slot name="right-extra" />') < trailing.indexOf('{{ settingsText }}'))
  assert.ok(trailing.indexOf('{{ settingsText }}') < trailing.indexOf('<slot name="after-settings" />'))
})

test('账单明细弹窗宽度 min(1200px, 94vw)，1366 屏不溢出', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /width="min\(1200px, 94vw\)"/)
  assert.equal(/width="1200px"/.test(src), false, '固定 1200px 会在 1366 屏横向溢出')
})

test('暂停缴费改用 --td-text-color-primary 令牌，深色主题可见', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /\.hr-si-status--paused \{ color: var\(--td-text-color-primary, #1d2129\); \}/)
  // 仍保留两色品牌语义（参保缴费绿 / 终止缴费橙）
  assert.match(src, /\.hr-si-status--active \{ color: #00a870; \}/)
  assert.match(src, /\.hr-si-status--ended \{ color: #ed7b2f; \}/)
})

test('导出 Excel 有 loading 态：工具栏与账单弹窗共用一个 exporting', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const exporting = ref\(false\)/)
  const loadings = src.match(/:loading="exporting"/g) || []
  assert.equal(loadings.length, 2, '批量栏与账单弹窗各一处')
  assert.match(src, /if \(exporting\.value\) return/, '重复点击必须短路')
})

test('Esc 关闭：打印预览与附件预览都在 window 捕获阶段监听并解绑', () => {
  for (const [name, src] of [
    ['PrintPreviewDialog.vue', read('PrintPreviewDialog.vue')],
    ['FilePreviewDialog.vue', readBiz('FilePreviewDialog.vue')],
  ] as const) {
    assert.match(src, /window\.addEventListener\('keydown', onKeydown, true\)/, `${name} 缺 Esc 监听`)
    assert.match(src, /window\.removeEventListener\('keydown', onKeydown, true\)/, `${name} 缺解绑`)
    assert.match(src, /e\.stopPropagation\(\)/, `${name} 必须阻止冒泡，避免一次 Esc 关两层`)
    assert.match(src, /e\.key !== 'Escape'/)
  }
})

test('blob URL 释放：关闭预览 / 换文件 / 页面卸载三处都 revoke', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /function releasePreviewUrl\(\)/)
  assert.match(src, /URL\.revokeObjectURL\(previewUrl\)/)
  assert.match(src, /function onPreviewVisibleChange\(v: boolean\)/)
  assert.match(src, /function closeBillPrint\(\)/)
  assert.match(src, /<FilePreviewDialog :visible="previewVisible"/, '关闭要能被父级拦截')
  assert.match(src, /onUnmounted\(\(\) => \{[\s\S]*?releasePreviewUrl\(\)/)
  assert.match(src, /if \(billPrintUrl\.value\) \{ URL\.revokeObjectURL\(billPrintUrl\.value\)/)
})

// ---------------------------------------------------------------------------
// ④ 第 58 轮：死代码 / 模板归一化
// ---------------------------------------------------------------------------

test('第 47 轮两个死抽屉已彻底删除（DOM + 状态 + 列定义）', () => {
  const src = read('HrManagement.vue')
  for (const dead of ['periodDetailVisible', 'periodDetail', 'personalCharges', 'unitCharges',
    'chargeColumns', 'siDetailVisible', 'siDetailItems', 'siDetailColumns',
    'siDetailFoot', 'SI_SCHEME_OPTIONS', 'hr-si-detail', 'hr-charge-group']) {
    assert.equal(src.includes(dead), false, `${dead} 应已删除`)
  }
})

test('BusinessSplitDetailDrawer 模板归一化：只剩一条 normalizedSections 渲染链', () => {
  const src = read('BusinessSplitDetailDrawer.vue')
  assert.match(src, /const normalizedSections = computed</)
  assert.match(src, /<section v-for="sec in normalizedSections"/)
  const loops = src.match(/v-for="f in sec\.fields"/g) || []
  assert.equal(loops.length, 1, '控件分支只允许存在一份')
  assert.equal(src.includes('v-else class="bsd-block"'), false, '旧的无分组模板必须删除')
  // 单组 label 用 blockTitle，保持旧单块视觉
  assert.match(src, /label: props\.blockTitle/)
})
