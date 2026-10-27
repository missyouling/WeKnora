import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  METER_FALLBACK_FIELDS,
  meterArchiveValue,
  meterFieldSpec,
  meterModeLabel,
} from './meterFieldRenderers'

const here = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(here, f), 'utf8')

const meter = read('./UtilityMeterTab.vue')
const drawer = read('./MeterRecordDrawer.vue')
const solar = read('./SolarManagement.vue')
const utilities = read('./UtilitiesManagement.vue')
const polling = read('../../composables/useBusinessPolling.ts')

test('核算抽屉：分时表在起度位置整行渲染四段块，止度不再单独出现', () => {
  assert.equal(meterFieldSpec('start_reading').kind, 'readings')
  assert.equal(meterFieldSpec('end_reading').kind, 'readings')
  assert.match(drawer, /v-for="p in timePeriods"/, '分时电表必须渲染尖峰平谷四段')
  assert.match(drawer, /f\.key === 'end_reading'/, '分时态必须跳过止度单项')
})

test('核算抽屉：派生/档案字段只读，不给出可编辑输入框', () => {
  for (const key of ['usage', 'amount', 'rate']) {
    const kind = meterFieldSpec(key).kind
    assert.ok(kind === 'usage' || kind === 'amount' || kind === 'readonly', `${key} 必须是只读派生字段`)
  }
  // 表计档案属性：改它们要去「表计设置」，抽屉里只读展示
  for (const key of ['meter_no', 'meter_mode', 'install_date', 'default_unit_price']) {
    assert.equal(meterFieldSpec(key).kind, 'readonly', `${key} 必须是只读档案字段`)
  }
  assert.equal(meterArchiveValue({ meter_no: 'M-01' }, 'meter_no'), 'M-01')
  assert.equal(meterModeLabel('auto'), '自动抄表')
  assert.equal(meterModeLabel('manual'), '手动抄表')
  assert.equal(meterModeLabel(''), '—')
})

test('核算抽屉：未知字段按 data_type 兜底，不静默丢字段', () => {
  assert.equal(meterFieldSpec('custom_amount', 'number').kind, 'number')
  assert.equal(meterFieldSpec('custom_date', 'date').kind, 'date')
  assert.equal(meterFieldSpec('custom_note', 'textarea').kind, 'textarea')
  assert.equal(meterFieldSpec('custom_text').kind, 'text')
})

test('核算抽屉兜底字段集不得包含「录入日期」（它从来不在字段定义里）', () => {
  assert.ok(!METER_FALLBACK_FIELDS.some((f) => f.name === 'record_date'), '兜底集不得含 record_date')
  // 只校验模板不渲染；注释里保留历史说明（为什么删掉它）不算问题
  assert.doesNotMatch(drawer, /label="录入日期"/, '抽屉不得再渲染录入日期')
})

test('核算抽屉：字段渲染严格来自 subs，编辑与新增共用同一实现', () => {
  assert.match(drawer, /props\.subs \|\| \[\]/, '抽屉必须读 subs')
  assert.match(drawer, /filter\(\(s: any\) => s && s\.enabled !== false/, '必须过滤未启用字段')
  assert.match(meter, /:subs="categorySubs"/, '父级必须把字段定义传进抽屉')
  assert.match(meter, /:meter="currentMeter"/, '父级必须传当前表计档案')
  // 编辑与新增共用同一抽屉组件（openCreate/openEdit 都只改 form）
  assert.match(meter, /const openCreate = \(\) => \{/)
  assert.match(meter, /const openEdit = \(row: any\) => \{/)
})

test('核算抽屉底部只有取消/保存：删除本行必须移除', () => {
  assert.doesNotMatch(meter, />删除本行</, '核算页不得再有「删除本行」按钮')
  assert.doesNotMatch(meter, /#footer-left/, '抽屉不得再透出 footer-left 插槽')
  assert.match(drawer, /@click="emit\('close'\)">取消</, '必须有取消')
  assert.match(drawer, /@click="emit\('save'\)">保存</, '必须有保存')
})

test('7 处模块初始化 Loading 统一为 t-loading size=small（无文字）', () => {
  for (const f of [
    'AwardPunishManagement.vue',
    'ContractManagement.vue',
    'EnergyManagement.vue',
    'InvoiceManagement.vue',
    'RegulationManagement.vue',
    'SolarManagement.vue',
    'UtilitiesManagement.vue',
  ]) {
    const src = read(`./${f}`)
    assert.doesNotMatch(src, /size="large"/, `${f} 不得再有大号 loading`)
    assert.doesNotMatch(src, /正在初始化/, `${f} 不得再有加载文字`)
    assert.match(src, /<t-loading size="small" \/>/, `${f} 必须是标准小圈`)
  }
})

test('市电/光伏上传走中台 FleetUploadDialog，原生 input 必须消失', () => {
  for (const [name, src, scope, cert] of [
    ['UtilitiesManagement', utilities, 'utility', '市电账单'],
    ['SolarManagement', solar, 'solar', '光伏账单'],
  ] as const) {
    assert.match(src, /<FleetUploadDialog/, `${name} 必须使用中台上传弹窗`)
    assert.match(src, new RegExp(`scope="${scope}"`), `${name} 弹窗 scope 必须是 ${scope}`)
    assert.match(src, /extract-mode="energy-bill"/, `${name} 必须走账单专用提取通道`)
    assert.ok(src.includes(`default-type="${cert}"`), `${name} 必须默认选中 ${cert}`)
    assert.doesNotMatch(src, /<input ref="fileInputRef"/, `${name} 不得再用原生文件选择`)
  }
})

test('市电/光伏轮询双向隔离：任一光伏痕迹即让路，反之亦然', () => {
  assert.match(utilities, /function isElectricityBillFile/, '市电必须有分类判定')
  assert.match(utilities, /activeTab\.value === 'electricity' && isElectricityBillFile\(item\)/, '市电轮询必须按 Tab 收口')
  assert.match(solar, /function isSolarBillFile/, '光伏必须有分类判定')
  // 双向排除：任一光伏痕迹不让市电认领；任一电费痕迹不让光伏认领
  assert.match(utilities, /meta\.fleet_cert_type === '光伏账单'\) return false/)
  assert.match(utilities, /\/光伏\/\.test\(name\)\) return false/)
  assert.match(solar, /meta\.fleet_cert_type === '市电账单'\) return false/)
})

test('提取全流程结束自动刷新列表与卡片（onSettled / 在途清零）', () => {
  assert.match(polling, /onSettled\?:/, '轮询必须暴露 onSettled')
  assert.match(polling, /prevPendingCount > 0 && visiblePend\.length === 0/, '必须识别「由有变无」')
  assert.match(utilities, /onSettled: async \(\) => \{ await loadFiles\(true\)/, '市电必须接住 onSettled')
  assert.match(solar, /if \(prevPending > 0 && pend\.length === 0\)/, '光伏必须识别整批完成')
  assert.match(solar, /prevPending > 0 && pend\.length === 0\) \{[\s\S]{0,120}notifyEnergyDataChanged\(\)/, '光伏完成后必须刷新列表与卡片')
})
