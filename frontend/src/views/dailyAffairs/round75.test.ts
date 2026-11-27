/**
 * 第 75 轮单测：
 *   ① 单位全称/简称归一（动态单位标签 + 费用清单归零根因）
 *   ② 单位标签 ≤1 隐藏 / ≥2 渲染（纯函数 + 源码契约）
 *   ③ 能耗账单三子标签（市电账单 | 光伏账单 | 基本户）
 *   ④ 列宽 0 必须原样生效（不再回落到静态固定宽）
 *   ⑤ 工业表计三介质隔离（前端按 category 重置 + 载荷校验）
 *   ⑥ 费用清单多租户块标签 + 「—」占位清除
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { collectUnits, unitMatches, unitShortName } from './unitLabel'
import { ENERGY_BILL_SUB_TABS } from './energySettingsTabs'
import { applySubsToColumns } from './energyViewColumns'

const read = (p: string) => readFileSync(new URL(`./${p}`, import.meta.url), 'utf8')

// ---------- ① 单位归一 ----------
test('单位简称：全称 → 核心词，档案简称优先', () => {
  assert.equal(unitShortName('重庆星达铜业有限公司'), '重庆星达')
  assert.equal(unitShortName('持睿汽车部件有限公司'), '持睿汽车')
  assert.equal(unitShortName('持睿汽车部件有限公司', ['持睿汽车']), '持睿汽车')
})

test('单位等价匹配：全称/简称双向命中', () => {
  assert.equal(unitMatches('持睿汽车部件有限公司', '持睿汽车'), true)
  assert.equal(unitMatches('重庆星达铜业有限公司', '重庆星达'), true)
  assert.equal(unitMatches('重庆星达铜业有限公司', '持睿汽车'), false)
  assert.equal(unitMatches('', '持睿汽车'), false)
})

test('collectUnits：同一家公司只出一个标签，保持首次出现顺序', () => {
  const list = collectUnits([
    '重庆星达铜业有限公司',
    '持睿汽车部件有限公司',
    '重庆星达',
    '',
  ])
  assert.equal(list.length, 2)
  assert.equal(list[0].full, '重庆星达铜业有限公司')
  assert.equal(list[1].full, '持睿汽车部件有限公司')
  assert.equal(list[1].label, '持睿汽车')
})

// ---------- ② 单位标签显隐 ----------
test('单位标签：≤1 个单位隐藏、≥2 个渲染（页面源码契约）', () => {
  for (const file of ['UtilityMeterTab.vue', 'TenantBilling.vue']) {
    const src = read(file)
    assert.match(src, /showUnitTags/, `${file} 必须有动态显隐开关`)
    assert.match(src, /unitTags\.value\.length >= 2/, `${file} 只有 ≥2 个单位才渲染标签行`)
    assert.match(src, /data-unit-switch/, `${file} 标签行需要稳定的 data 属性`)
    assert.doesNotMatch(src, /key: 'useUnit', type: 'select'/, `${file} 不得再保留使用单位下拉`)
  }
})

// ---------- ③ 能耗账单三子标签 ----------
test('能耗账单子标签严格为 市电账单 | 光伏账单 | 基本户', () => {
  assert.deepEqual([...ENERGY_BILL_SUB_TABS], ['市电账单', '光伏账单', '基本户'])
  const drawer = read('EnergySettingDrawer.vue')
  assert.match(drawer, /ENERGY_BILL_SUB_TABS/, '子标签必须来自唯一定义')
  assert.match(drawer, /billTab !== '基本户'/, '基本户不得再渲染字段面板')
  assert.match(drawer, /<SolarSettingsPanel[\s\S]{0,80}embedded/, '基本户表单必须内嵌')
})

// ---------- ④ 列宽 0 ----------
test('列宽 0：subs.width=0 必须原样生效，不再回落静态宽', () => {
  const base = [{ key: 'use_unit', label: '使用单位', default: true, w: '1.2fr', width: 140 }]
  const out = applySubsToColumns(base, [
    { name: 'use_unit', label: '使用单位', enabled: true, is_default: true, data_type: 'text', width: 0 },
  ] as any)
  assert.equal(out.length, 1)
  assert.equal(out[0].width, 0, '0 是「自适应」的权威值')
  const fixed = applySubsToColumns(base, [
    { name: 'use_unit', label: '使用单位', enabled: true, is_default: true, data_type: 'text', width: 180 },
  ] as any)
  assert.equal(fixed[0].width, 180)
})

// ---------- ⑤ 工业表计隔离 ----------
test('工业表计：切换介质必须重置草稿并按介质加载', () => {
  const src = read('MeterSettingsPanel.vue')
  assert.match(src, /watch\(\(\) => props\.category/, '必须有 category 变化监听')
  assert.match(src, /dirtyEnabled\.value = new Set\(\)/, '切介质要清空待提交集合')
  assert.match(src, /meters\.value = \[\]/, '切介质要先清空旧介质列表')
  assert.match(src, /String\(m\.category \|\| props\.category\) !== String\(props\.category\)/, '保存时必须二次校验介质')
})

// ---------- ⑥ 费用清单块标签与空值 ----------
test('费用清单设置：租户信息 / 默认分组 / 租户分组 三类型块标签', () => {
  const drawer = read('EnergySettingDrawer.vue')
  assert.match(drawer, /data-cost-tabs/)
  assert.match(drawer, /<t-radio-button value="tenant">租户信息<\/t-radio-button>/)
  assert.match(drawer, /<t-radio-button value="default">默认分组<\/t-radio-button>/)
  assert.match(drawer, /v-for="t in costTenants"/)
  assert.match(drawer, /tenant:\$\{t\.id\}/)
  const tenantPanel = read('TenantSettingsPanel.vue')
  assert.doesNotMatch(tenantPanel, /<t-tabs/, '租户信息与分摊子项必须合并为同一视图')
  assert.match(tenantPanel, /data-tenant-section="info"/)
  assert.match(tenantPanel, /data-tenant-section="items"/)
})

test('费用清单：不再渲染「—」占位，空值为空白', () => {
  const src = read('TenantBilling.vue')
  assert.equal(src.includes("'—'"), false, '列表中不得出现破折号占位')
})
