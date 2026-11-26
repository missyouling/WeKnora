/**
 * 第 74 轮单测：
 *   ① 当前页列表「全字段子串关键字筛选」共享 helper（重庆星 → 重庆星达铜业有限公司、
 *      custom_data / 明细 / 派生字段、表号 E301、多词 AND、空关键字不过滤）；
 *   ② 「设置 · 能耗」7 个平级标签的映射与光伏字段平铺规则。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { filterRowsByKeyword, keywordTokens, rowHaystack, rowMatchesKeyword } from './keywordFilter'
import {
  ENERGY_BILL_CATEGORIES,
  ENERGY_SETTINGS_TABS,
  energyBillCategoryOf,
  energyCollapsesGroupsByDefault,
  energyFlattensGroups,
  energyTabOfCategory,
} from './energySettingsTabs'

// ---------- ① 全字段关键字筛选 ----------

const rosterRow = {
  id: 'e-1',
  工号: 'C02050011',
  姓名: '张三',
  部门: '铜业车间',
  岗位: '大拉操作员',
  签约公司: '重庆星达铜业有限公司',
  custom_data: { 工作地点: '1号楼', 所属公司: '重庆星达铜业有限公司' },
}

test('rowMatchesKeyword：任意连续子串命中同一字段值（重庆星 / 星达 / 铜业）', () => {
  assert.equal(rowMatchesKeyword(rosterRow, '重庆星'), true)
  assert.equal(rowMatchesKeyword(rosterRow, '星达'), true)
  assert.equal(rowMatchesKeyword(rosterRow, '铜业'), true)
  assert.equal(rowMatchesKeyword(rosterRow, '重庆星达铜业有限公司'), true)
  assert.equal(rowMatchesKeyword(rosterRow, '持睿'), false)
})

test('rowMatchesKeyword：custom_data 等嵌套对象里的值也参与匹配', () => {
  assert.equal(rowMatchesKeyword(rosterRow, '工作地点'), false) // 键名不匹配，只有值参与
  assert.equal(rowMatchesKeyword(rosterRow, '1号楼'), true)
})

test('rowMatchesKeyword：不区分大小写，且空格分词取 AND', () => {
  const meterRow = { 表号: 'E301', 表名: '1号楼总表', 使用单位: '重庆星达铜业有限公司' }
  assert.equal(rowMatchesKeyword(meterRow, 'e301'), true)
  assert.equal(rowMatchesKeyword(meterRow, 'E301'), true)
  assert.equal(rowMatchesKeyword(meterRow, '星达 E301'), true)
  assert.equal(rowMatchesKeyword(meterRow, '星达 E999'), false)
})

test('rowMatchesKeyword：数组明细（成员 / 配套设施 / items）逐项参与匹配', () => {
  const dormRow = {
    房屋编号: '301',
    成员: ['李四', '王五'],
    配套设施: [{ 名称: '空调' }, { 名称: '热水器' }],
  }
  assert.equal(rowMatchesKeyword(dormRow, '王五'), true)
  assert.equal(rowMatchesKeyword(dormRow, '热水器'), true)
  assert.equal(rowMatchesKeyword(dormRow, '洗衣机'), false)
})

test('filterRowsByKeyword：空关键字原样返回，非空仅保留命中行', () => {
  const rows = [rosterRow, { id: 'e-2', 姓名: '李四', 签约公司: '重庆持睿汽车配件有限公司' }]
  assert.equal(filterRowsByKeyword(rows, '   ').length, 2)
  const hit = filterRowsByKeyword(rows, '重庆星')
  assert.equal(hit.length, 1)
  assert.equal(hit[0].id, 'e-1')
})

test('keywordTokens / rowHaystack：词元归一与小写化', () => {
  assert.deepEqual(keywordTokens(' 星达   E301 '), ['星达', 'e301'])
  assert.equal(rowHaystack(rosterRow).includes('重庆星达铜业有限公司'), true)
})

// ---------- ② 设置 · 能耗 7 个平级标签 ----------

test('ENERGY_SETTINGS_TABS：顶层严格 7 个平级标签且顺序固定', () => {
  assert.deepEqual(ENERGY_SETTINGS_TABS.map((t) => t.label), [
    '能耗账单', '提取规则', '电费核算', '水费核算', '气费核算', '费用清单', '工业表计',
  ])
  assert.equal(ENERGY_SETTINGS_TABS.some((t) => t.label === '字段定义'), false)
  assert.equal(ENERGY_SETTINGS_TABS.some((t) => t.label === '表计设置'), false)
})

test('energyTabOfCategory：页面「设置」按钮按分类定位到对应平级标签', () => {
  assert.equal(energyTabOfCategory('市电账单'), 'bill')
  assert.equal(energyTabOfCategory('光伏账单'), 'bill')
  assert.equal(energyTabOfCategory('电费核算'), 'electric')
  assert.equal(energyTabOfCategory('水费核算'), 'water')
  assert.equal(energyTabOfCategory('气费核算'), 'gas')
  assert.equal(energyTabOfCategory('费用清单'), 'billing')
  assert.equal(energyTabOfCategory('工业表计'), 'meter')
  // 未知分类绝不落到空白标签
  assert.equal(energyTabOfCategory('不存在的分类'), 'bill')
})

test('energyBillCategoryOf：只有市电/光伏属于「能耗账单」的二级切换', () => {
  assert.deepEqual([...ENERGY_BILL_CATEGORIES], ['市电账单', '光伏账单'])
  assert.equal(energyBillCategoryOf('市电账单'), '市电账单')
  assert.equal(energyBillCategoryOf('光伏账单'), '光伏账单')
  assert.equal(energyBillCategoryOf('费用清单'), '')
})

test('光伏账单平铺、市电账单分组默认折叠', () => {
  assert.equal(energyFlattensGroups('光伏账单'), true)
  assert.equal(energyFlattensGroups('市电账单'), false)
  assert.equal(energyCollapsesGroupsByDefault('市电账单'), true)
  assert.equal(energyCollapsesGroupsByDefault('光伏账单'), false)
})
