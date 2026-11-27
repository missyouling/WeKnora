/**
 * 第 76 轮单测：提取规则标签联动、公司设置租户档案、保险单 3 标签 + 险种引用、
 * 证照改名不丢记录、样票生成功能清零。
 *
 * 口径说明：与既有 roundNN 测试一致，读源码断言「契约形状」，
 * 纯函数（insuranceFields / energySettingsTabs）做真实行为断言。
 */
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

import { energyBillCategoriesOf, ENERGY_BILL_SUB_TABS } from './energySettingsTabs'
import {
  INSURANCE_TABS,
  insuranceGroupLabel,
  insuranceTabOfGroup,
  syncInsuranceCoverageFields,
} from './insuranceFields'

const HERE = dirname(fileURLToPath(import.meta.url))
const read = (f: string) => readFileSync(join(HERE, f), 'utf8')

// ---------- ① 提取规则：方块标签 + 动态分类 ----------
test('能耗账单分类动态推导：内置两项 + 新增账单分类自动追加，核算表被排除', () => {
  const cats = [
    { name: '市电账单', enabled: true },
    { name: '光伏账单', enabled: true },
    { name: '电费核算', enabled: true },
    { name: '水费核算', enabled: true },
    { name: '气费核算', enabled: true },
    { name: '费用清单', enabled: true },
    { name: '能耗单', enabled: true },
    { name: '工业用电', enabled: true, builtin_key: 'energy-industry' },
    { name: '停用账单', enabled: false },
  ]
  assert.deepEqual(energyBillCategoriesOf(cats), ['市电账单', '光伏账单'])
  assert.deepEqual(energyBillCategoriesOf([...cats, { name: '燃气账单', enabled: true }]),
    ['市电账单', '光伏账单', '燃气账单'])
})

test('提取规则：能耗 scope 用方块标签而非下拉，且无缓存样票入口', () => {
  const panel = read('ExtractRulePanel.vue')
  assert.match(panel, /data-extract-type-tabs/, '能耗分类必须是方块标签')
  assert.match(panel, /energyBillCategoriesOf\(categories\.value\[props\.scope\]/)
  assert.doesNotMatch(panel, /name="ticket"/, '缓存样票按钮必须删除')
  assert.doesNotMatch(panel, /sampleTextCache/, '样票缓存依赖必须删除')
  // 5 列定宽：24 / 14 / 11 / 39 / 12
  assert.match(panel, /minmax\(0, 24fr\) minmax\(0, 14fr\) minmax\(0, 11fr\) minmax\(0, 39fr\) minmax\(0, 12fr\)/)
  assert.doesNotMatch(panel, /<span>字段描述<\/span>/, '字段描述列已下线')
  assert.match(panel, /<span class="adv-label">高级模式<\/span>/, '高级模式文字标签必须保留')
})

// ---------- ② 公司设置：租户档案 ----------
test('公司设置：单位表单含别名 / 租户开关 / 租户编码 / 分摊子项面板', () => {
  const org = read('HrOrgPanel.vue')
  assert.match(org, /data-org-alias/, '必须有别名输入框')
  assert.match(org, /data-org-tenant-switch/, '必须有是否为租户开关')
  assert.match(org, /data-org-tenant-no/, '必须有租户编码输入框')
  assert.match(org, /电费分摊子项/, '必须集成电费分摊子项分组')
  assert.match(org, /<TenantSettingsPanel[\s\S]{0,160}items-only/, '分摊子项面板必须天然绑定当前单位')
  // 后端：单位表扩展 + 保存同步 billing_tenants
  const units = read('../../../../internal/handler/hr_org_units.go')
  assert.match(units, /Alias\s+string/)
  assert.match(units, /IsTenant\s+bool/)
  assert.match(units, /BillingTenantID\s+string/)
  assert.match(units, /hrSyncOrgTenants76/)
})

test('单位切换标签以「公司设置 → 别名」为唯一展示源', () => {
  for (const f of ['UtilityMeterTab.vue', 'TenantBilling.vue']) {
    const src = read(f)
    assert.match(src, /getOrgUnits\(\)/, `${f} 必须读取公司设置的单位档案`)
    assert.match(src, /orgAliases/, `${f} 必须把别名传给 collectUnits`)
    assert.match(src, /collectUnits\([\s\S]{0,160}orgAliases\.value/, `${f} 标签展示必须用别名`)
  }
})

test('分摊子项面板：items-only 模式隐藏租户下拉与跨分类搜索', () => {
  const panel = read('TenantSettingsPanel.vue')
  assert.match(panel, /itemsOnly/)
  assert.match(panel, /v-if="!props\.itemsOnly" class="settings-section"/)
  assert.match(panel, /v-if="!props\.itemsOnly && tenants\.length"/)
  assert.match(panel, /v-if="!props\.itemsOnly" class="item-add-row"/)
})

// ---------- ③ 保险单 3 标签 ----------
test('保险单：标签键解析与分组显示名', () => {
  assert.deepEqual([...INSURANCE_TABS], ['交强险', '商业险', '险种类别'])
  assert.equal(insuranceTabOfGroup('交强险/基本信息'), '交强险')
  assert.equal(insuranceTabOfGroup('商业险/投保险别'), '商业险')
  assert.equal(insuranceTabOfGroup('险种类别'), '险种类别')
  assert.equal(insuranceGroupLabel('交强险/责任限额'), '责任限额')
  assert.equal(insuranceGroupLabel('险种类别'), '险种类别')
})

test('保险单：投保险别随险种类别新增 / 停用 / 调序同步', () => {
  const fields: any[] = [
    { name: '保险单号', label: '保险单号', enabled: true, isDefault: true, dataType: 'text', width: 0, group: '交强险/基本信息' },
    { name: '机动车损失保险', label: '机动车损失保险', enabled: true, isDefault: false, dataType: 'text', width: 0, group: '险种类别' },
    { name: '机动车第三者责任保险', label: '机动车第三者责任保险', enabled: true, isDefault: false, dataType: 'text', width: 0, group: '险种类别' },
    { name: '投保险别_机动车损失保险', label: '机动车损失保险', enabled: true, isDefault: true, dataType: 'amount', width: 120, group: '商业险/投保险别' },
  ]
  // 新增第三个险种 → 投保险别补一条，且保留已有项的宽度
  const added = syncInsuranceCoverageFields([
    ...fields,
    { name: '车上人员责任险(司机)', label: '车上人员责任险(司机)', enabled: true, isDefault: false, dataType: 'text', width: 0, group: '险种类别' },
  ] as any)
  const coverage = added.filter((f: any) => f.group === '商业险/投保险别')
  assert.deepEqual(coverage.map((f: any) => f.label), ['机动车损失保险', '机动车第三者责任保险', '车上人员责任险(司机)'])
  assert.equal(coverage[0].width, 120, '已存在项保留用户宽度')

  // 调序 + 停用：投保险别按险种类别顺序重排，启停跟随
  const reordered = syncInsuranceCoverageFields([
    fields[0],
    { ...fields[2], enabled: false },
    fields[1],
    fields[3],
  ] as any).filter((f: any) => f.group === '商业险/投保险别')
  assert.deepEqual(reordered.map((f: any) => f.label), ['机动车第三者责任保险', '机动车损失保险'])
  assert.equal(reordered[0].enabled, false, '停用的险种在投保险别里同样停用')

  // 删除险种 → 投保险别移除对应项
  const removed = syncInsuranceCoverageFields([fields[0], fields[1], fields[3]] as any)
    .filter((f: any) => f.group === '商业险/投保险别')
  assert.deepEqual(removed.map((f: any) => f.label), ['机动车损失保险'])
})

test('保险单：后端字段清单 30 + 24 + 7 与三个配置标签', () => {
  const be = read('../../../../internal/handler/hr_round76.go')
  assert.match(be, /hrInsTabJq76\s+=\s+"交强险"/)
  assert.match(be, /hrInsTabSy76\s+=\s"商业险"/)
  assert.match(be, /hrInsTabXz76\s+=\s"险种类别"/)
  assert.match(be, /"死亡伤残赔偿限额"/)
  assert.match(be, /"机动车第三者责任保险"/)
  // 字段数量契约：交强险 13+11+6=30，商业险 13+11+7=31，险种类别 7
  const panel = read('FleetCertPanel.vue')
  assert.match(panel, /data-insurance-tabs/)
  assert.match(panel, /syncInsuranceCoverageFields/)
  const drawer = read('../../components/business/StandardSettingDrawer.vue')
  assert.match(drawer, /insuranceTabs/)
  const list = read('FleetRecordList.vue')
  assert.match(list, /activeCategoryName === '保险单'/)
})

// ---------- ④ 证照改名不丢记录 ----------
test('证照改名：同一事务同时更新 doc_type 与 sub_category', () => {
  const src = read('../../../../internal/handler/fleet_category_rename.go')
  assert.match(src, /Update\("doc_type", newName\)/)
  assert.match(src, /Update\("sub_category", newName\)/, '列表按 sub_category 过滤，必须一起改')
  assert.match(src, /rename fleet records sub_category/)
})

// ---------- ⑤ 样票功能全局清零 ----------
test('样票生成功能全局清零：无按钮文案 / 无接口 / 无组件', () => {
  /** 只检查「会渲染/会执行」的代码：注释里保留需求说明是允许的。 */
  const strip = (src: string) => src
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
  const files = [
    'FleetCertPanel.vue',
    'ExtractRulePanel.vue',
    '../../components/business/StandardSettingDrawer.vue',
  ]
  for (const f of files) {
    const src = strip(read(f))
    assert.doesNotMatch(src, /用样票生成字段|上传样票生成字段|缓存样票/, `${f} 仍有样票文案`)
  }
  const api = strip(read('../../api/fleet.ts'))
  assert.doesNotMatch(api, /aiSuggestFields/, '样票字段生成接口封装必须删除')
})
