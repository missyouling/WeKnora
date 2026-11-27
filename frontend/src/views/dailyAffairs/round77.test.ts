/**
 * 第 77 轮单测：序号列下线、表计类型三源、宿舍保存放通、抽屉统一保存、
 * 全局去必填、公司设置精简、车队保险单三标签与行驶证隔离。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const HERE = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(HERE, f), 'utf8')
const readGo = (rel: string) => readFileSync(join(HERE, '../../../../', rel), 'utf8')

// ---------- ① 能耗序号列下线 ----------
test('能耗 4 个列表不再构建自增序号列（复选框后直接业务字段）', () => {
  for (const f of ['UtilitiesManagement.vue', 'UtilityMeterTab.vue', 'SolarManagement.vue', 'TenantBilling.vue']) {
    const src = read(f)
    assert.doesNotMatch(src, /colKey: 'serial-number'/, `${f} 仍有序号列`)
    assert.match(src, /colKey: 'row-select'/, `${f} 复选框列保留`)
  }
})

// ---------- ② 表计类型三源 ----------
test('表计类型：工业表计改文本输入 + 宿舍/公租房字段与默认值', () => {
  const meter = read('MeterSettingsPanel.vue')
  assert.match(meter, /<label>表计类型<\/label>/)
  assert.match(meter, /data-meter-kind-input/)
  assert.doesNotMatch(meter, /<label>用途<\/label>/, '用途 Label 必须改为表计类型')
  assert.match(meter, /const defaultKind = '工商业'/)
  const hr = read('HrManagement.vue')
  assert.match(hr, /data\['表计类型'\] = data\['表计类型'\] \|\| '宿舍'/)
  assert.match(hr, /data\['表计类型'\] = data\['表计类型'\] \|\| '公租房'/)
  const be = readGo('internal/handler/hr_round77.go')
  assert.match(be, /hrMeterTypeField77 = "表计类型"/)
  assert.match(be, /hrSeedMeterType77/)
})

test('核算筛选：表计类型选项由当前行 + 宿舍/公租房动态汇总，首项全部表计类型', () => {
  const src = read('UtilityMeterTab.vue')
  assert.match(src, /const kindFilterOptions = computed/)
  assert.match(src, /meterKindLabelOf\(r\?\.meter_kind\)/)
  assert.match(src, /archiveKinds/)
  assert.match(src, /label: '全部表计类型'/)
  assert.doesNotMatch(src, /meterKinds\.value\.map\(k => \(\{ label: k\.label, value: k\.value \}\)\)/, '不得再写死静态枚举')
})

// ---------- ③④ 宿舍保存放通 ----------
test('宿舍修改放通：更新路径不再调用表计查重，新增仍拦截', () => {
  const src = readGo('internal/handler/hr_dorm.go')
  const hits = (src.match(/hrDormValidateMeters72/g) || []).length
  assert.equal(hits, 1, '只允许新增路径调用表计查重')
  // 更新分支必须保留空置/缩容校验（不是把整段删掉）
  assert.match(src, /hrDormValidateRoomMeta72/)
})

test('查重池只含工商业表计，排除历史宿舍/公租房残留', () => {
  const src = readGo('internal/handler/hr_round72.go')
  assert.match(src, /meter_kind IN \?/)
  assert.match(src, /"工商业", "production"/)
})

// ---------- ⑤ 抽屉统一保存 ----------
test('抽屉统一保存：内嵌表单不再出第二套保存/取消', () => {
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /drawerManaged\?: boolean/)
  assert.match(panel, /!props\.lockCategoryShell && !props\.drawerManaged/)
  const meter = read('MeterSettingsPanel.vue')
  assert.match(meter, /props\.embedded[\s\S]{0,200}收起/, '工业表计内嵌只留「收起」')
  assert.match(meter, /function foldMeterForm/)
  const std = read('../../components/business/StandardSettingDrawer.vue')
  assert.match(std, /drawer-managed/)
  assert.match(std, /rulesVisited/)
  const energy = read('EnergySettingDrawer.vue')
  assert.match(energy, /const jobs: Array<\{ tab: EnergySettingsTab; run: \(\) => Promise<void> \}>/)
  assert.match(energy, /job\.tab/, '失败时切到失败标签')
})

// ---------- ⑥ 去必填 ----------
test('提取规则：4 列定宽、无必填列；后端不再因 required 阻断', () => {
  const panel = read('ExtractRulePanel.vue')
  assert.doesNotMatch(panel, /col-required|rtr-required|v-model="f\.required"/)
  assert.match(panel, /minmax\(0, 32fr\) minmax\(0, 16fr\) minmax\(0, 40fr\) minmax\(0, 12fr\)/)
  assert.match(panel, /<span class="adv-label">高级模式<\/span>/)
  const beRaw = readGo('internal/application/service/extract_generic_validate.go')
  const be = beRaw.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '')
  assert.doesNotMatch(be, /f\.Required/, '必填阻断逻辑必须删除')
  assert.match(be, /genericRegexPatch/, '正则兜底能力保留')
})

// ---------- ⑦ 公司设置 ----------
test('公司设置：菜单改公司管理、删冗余标题、分摊子项默认折叠手风琴', () => {
  const modal = read('../../components/business/CompanySettingsModal.vue')
  assert.match(modal, /公司管理/)
  assert.doesNotMatch(modal, /单位与部门/)
  const org = read('HrOrgPanel.vue')
  assert.doesNotMatch(org, /电费分摊子项/, '冗余标题栏必须删除')
  const panel = read('TenantSettingsPanel.vue')
  assert.match(panel, /expandedItemGroup\.value = ''/, '默认全部折叠')
})

// ---------- ⑧ 车队 ----------
test('车辆档案设置：横向证照标签 + 保险单子标签仅归属保险单', () => {
  const std = read('../../components/business/StandardSettingDrawer.vue')
  assert.match(std, /flatCertTabs/)
  assert.match(std, /data-cert-flat-tabs/)
  assert.match(std, /certTab === '保险单'/)
  const list = read('FleetRecordList.vue')
  assert.match(list, /:flat-cert-tabs="isArchive && meta\.scope === 'vehicle'"/)
  assert.match(list, /data-insurance-list-tabs/)
  assert.match(list, /insuranceKindsOf/)
  const cols = read('fleetColumns.ts')
  assert.doesNotMatch(cols, /'—'/, '列表空值必须空白')
})
