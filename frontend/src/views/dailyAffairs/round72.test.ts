// 第 72 轮前端单测：源码契约回归（房型联动 / 空置 / 查重 / 公租房级联 / 住宿重构 / 资产字典）
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const DIR = path.dirname(fileURLToPath(import.meta.url))
const read = (rel: string) => fs.readFileSync(path.join(DIR, rel), 'utf8')

test('第72轮：房型切换实时重算床位数/可用床位/状态', () => {
  const src = read('HrManagement.vue')
  // 房型 → 床位数
  assert.match(src, /function onDormRoomTypeChange\(v: any\)/)
  assert.match(src, /const beds = hrDormBedsOfType\(type\)/)
  // 统一重算入口
  assert.match(src, /function recalcDormCapacity\(\)/)
  assert.match(src, /const free = Math\.max\(0, beds - used\)/)
  // 空置优先：可用床位恒 0、状态恒「空置」
  assert.match(src, /if \(vacant\) \{\s*\n\s*d\['可用床位数'\] = '0'\s*\n\s*d\['宿舍状态'\] = '空置'/)
})

test('第72轮：防缩容（前端拦截 + 提示文案）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /床位不能缩容为/)
  assert.match(src, /if \(beds > 0 && used > beds\)/)
  assert.match(src, /const used = dormActiveStayCount\.value/)
})

test('第72轮：宿舍状态四态（去维修中、加空置）', () => {
  const src = read('hrDorm.ts')
  assert.match(src, /const ROOM_STATUS = \['空闲', '部分入住', '满员', '空置'\]/)
  assert.ok(!/维修中/.test(src) || !/const ROOM_STATUS = \[[^\]]*维修中/.test(src))
  const style = read('hrStatusStyle.ts')
  assert.match(style, /空置\|维修中\|停用/, '空置必须映射为灰（off）')
  const hr = read('HrManagement.vue')
  assert.match(hr, /label: '空置', value: '空置'/, '状态筛选器要有空置')
  assert.ok(!/label: '维修中'/.test(hr), '筛选器不应再有维修中')
  // 可住判定排除空置
  assert.match(hr, /status === '满员' \|\| status === '空置' \|\| status === '维修中'/)
})

test('第72轮：空置开关（有在住人员时拦截 + 同行展示）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /function onDormVacantChange\(v: any\)/)
  assert.match(src, /该房源当前仍有在住人员，请先办理退宿后再设为空置/)
  assert.match(src, /data-hr-dorm-vacant/)
  assert.match(src, /template #field-宿舍状态/)
})

test('第72轮：床位号按房型池化并排除已占床位', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const dormBedOptions = computed\(\(\) => \{/)
  assert.match(src, /const total = Math\.max\(1, Math\.min\(8, beds\)\)/)
  assert.match(src, /filter\(\(s: any\) => String\(s\['住宿状态'\] \|\| ''\) !== '已退宿'\)/)
  // 编辑自身保留当前床位
  assert.match(src, /filter\(\(b\) => !used\.has\(b\) \|\| b === cur\)/)
  // 静态 8 床位常量已删除
  assert.ok(!/const DORM_BED_OPTIONS = \[/.test(src), '不得再用静态床位清单')
})

test('第72轮：公租房「租赁单位」参与公司识别，所属部门空态仍为下拉', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /d\['所属公司'\] \|\| d\['租赁单位'\]/, '租赁单位必须等同所属公司')
  assert.match(src, /const isOrgSelect = name === '所属部门'/)
  assert.match(src, /selectLike: true, emptyPlaceholder: '请选择部门'/)
  // 换公司时清空不匹配的部门
  assert.match(src, /if \(!allowed\.includes\(dept\)\) d\['所属部门'\] = ''/)
})

test('第72轮：住宿记录费用项归入「费用项目」且删除 3 个度数字段', () => {
  const src = read('hrDorm.ts')
  assert.match(src, /const STAY_FEE_LINK_FIELDS: readonly HrDormField\[\] = \[/)
  for (const f of ['电费标准', '水费单价', '气费标准', '房屋租金', '季度租金', '物业费', '垃圾处理费', '二次供水费', '污水处理费']) {
    assert.ok(new RegExp(`name: '${f}', type: 'number', subGroup: '费用项目'`).test(src), `费用项 ${f} 必须在费用项目组`)
  }
  assert.ok(!/入住时(水|电|气)表度数/.test(src), '3 个度数字段必须彻底删除')
  assert.ok(!/name: '住宿记录ID'/.test(src), '住宿记录ID 不允许出现在字段契约里')
})

test('第72轮：员工下拉只显示姓名（无工号后缀）', () => {
  const src = read('HrManagement.vue')
  assert.match(src, /const dormEmployeeOptions = computed\(\(\) => dormEmployeeList\.value\.map/)
  assert.ok(!/（\$\{e\.employee_id/.test(src), '不得再拼工号')
})

test('第72轮：资产档案=共用物品字典，配套设施按名称去重', () => {
  const dorm = read('hrDorm.ts')
  assert.match(dorm, /export const HR_DORM_STANDARD_FACILITIES/)
  const hr = read('HrManagement.vue')
  assert.match(hr, /const norm = \(v: any\) => String\(v \?\? ''\)\.replace\(/)
  assert.match(hr, /if \(st && st !== '正常'\) continue/, '配套只取正常状态资产')
})

test('第72轮：设置抽屉底部「取消」真关闭，不再留白壳', () => {
  const drawer = read('../../components/business/StandardSettingDrawer.vue')
  assert.ok(!/^\s*hide-footer\s*$/m.test(drawer), '设置抽屉模板不得再传 hide-footer')
  assert.match(drawer, /@cancel="onFooterCancel"/)
  assert.match(drawer, /@confirm="onFooterConfirm"/)
  assert.match(drawer, /function onFooterCancel\(\) \{[\s\S]{0,200}emit\('update:visible', false\)/)
  assert.match(drawer, /await panelRef\.value\?\.commitPendingEdit\?\.\(\)/)
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /async function commitPendingEdit\(\): Promise<void>/)
  assert.match(panel, /function discardPendingEdit\(\): void/)
})
