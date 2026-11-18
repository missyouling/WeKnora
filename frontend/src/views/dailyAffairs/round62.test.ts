/**
 * 第 62 轮前端回归：
 *   · 宿舍 5 胶囊字段契约（双分组 / 费用项目 / 公租房租期）；
 *   · 房型 → 床位数（整数，禁 4.00）；
 *   · 字段显示名与数据身份分离（改名不丢数据）；
 *   · 抽屉标题 / 子表删除路由 / 导入三模式 / 设置保存自动关闭。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  HR_DORM_FIELDS, HR_DORM_INTEGER_FIELDS, HR_DORM_GROUP_OF, hrDormBedsOfType,
  hrDormDrawerGroups, hrDormRepairTheme, hrDormChargeColumns,
} from './hrDorm'
import { hrColumnsFromSubs } from './hrCore'

const readLocal = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

test('字段契约：五胶囊列数与第 62 轮需求一致', () => {
  assert.equal(HR_DORM_FIELDS.dorm.length, 26)
  assert.equal(HR_DORM_FIELDS.public.length, 30)
  assert.equal(HR_DORM_FIELDS.stay.length, 27)
  assert.equal(HR_DORM_FIELDS.asset.length, 16)
  assert.equal(HR_DORM_FIELDS.fee.length, 20)
  assert.equal(HR_DORM_GROUP_OF.fee, '费用信息')
})

test('字段契约：公租房含租期与自动派生，宿舍含房间面积', () => {
  const pub = HR_DORM_FIELDS.public.map((f) => f.name)
  for (const n of ['房屋编号', '房屋地址', '租赁单位', '建筑面积(㎡)',
    '月费用单价(元/月)', '季度租金(元/季)', '租期起始月份', '租期截止月份',
    '电费标准(元/度)', '水费单价(元/m³)', '垃圾处理费(元/套)', '二次供水费(元/m³)',
    '污水处理费(元/m³)', '气费标准(元/m³)']) {
    assert.ok(pub.includes(n), `公租房缺字段 ${n}`)
  }
  const dorm = HR_DORM_FIELDS.dorm.map((f) => f.name)
  assert.ok(dorm.includes('房间面积(㎡)'))
  assert.ok(dorm.includes('所属公司'))
  assert.ok(dorm.includes('所属部门'))
  assert.equal(HR_DORM_FIELDS.dorm.find((f) => f.name === '房型')?.options?.join(','),
    '单人间,双人间,夫妻房,四人间,六人间,八人间')
  assert.equal(HR_DORM_FIELDS.asset.find((f) => f.name === '资产价值(元)')?.type, 'number')
})

test('房型 → 床位数（只读整数），夫妻房自动不限性别', () => {
  assert.equal(hrDormBedsOfType('单人间'), 1)
  assert.equal(hrDormBedsOfType('双人间'), 2)
  assert.equal(hrDormBedsOfType('夫妻房'), 2)
  assert.equal(hrDormBedsOfType('四人间'), 4)
  assert.equal(hrDormBedsOfType('六人间'), 6)
  assert.equal(hrDormBedsOfType('八人间'), 8)
  assert.equal(hrDormBedsOfType('异形'), 1)
  for (const n of ['床位数', '可用床位数', '数量', '入住天数']) {
    assert.ok(HR_DORM_INTEGER_FIELDS.has(n), `${n} 必须是整数列`)
  }
  const beds = HR_DORM_FIELDS.dorm.find((f) => f.name === '床位数')
  assert.equal(beds?.readonly, true, '床位数只读')
})

test('编辑抽屉：宿舍/公租房/住宿双分组，资产与费用单分组', () => {
  for (const v of ['dorm', 'public', 'stay'] as const) {
    const groups = hrDormDrawerGroups(v)
    assert.equal(groups.length, 2, `${v} 必须两组`)
    assert.equal(groups[0].label, HR_DORM_GROUP_OF[v])
    assert.equal(groups[1].label, '费用项目')
    assert.ok(groups[0].fields.every((f) => !f.subGroup))
    assert.ok(groups[1].fields.every((f) => f.subGroup === '费用项目'))
  }
  assert.equal(hrDormDrawerGroups('asset').length, 1)
  assert.equal(hrDormDrawerGroups('fee').length, 1)
  // 配套设施必须是多选控件（multi）
  assert.equal(HR_DORM_FIELDS.dorm.find((f) => f.name === '配套设施')?.type, 'multi')
})

test('整数列渲染：床位数/可用床位数/数量绝不出现 4.00', () => {
  const subs = [
    { name: '床位数', enabled: true, data_type: 'number' },
    { name: '可用床位数', enabled: true, data_type: 'number' },
    { name: '数量', enabled: true, data_type: 'number' },
  ]
  const cols = hrColumnsFromSubs(subs, []) as any[]
  const row = { 床位数: 4, 可用床位数: '2', 数量: 12.0 }
  for (const key of ['床位数', '可用床位数', '数量']) {
    const col = cols.find((c) => c.colKey === key)
    assert.ok(col, `${key} 列必须存在`)
    const out = String(col.value(row))
    assert.equal(out, String(Math.round(Number(row[key as keyof typeof row]))))
    assert.equal(out.includes('.'), false, `${key} 不得带小数`)
  }
})

test('改名不丢数据：label 变更后仍按原始 name 取值', () => {
  const subs = [{ name: '建筑面积(㎡)', label: '建筑面积', enabled: true, data_type: 'number' }]
  const cols = hrColumnsFromSubs(subs, []) as any[]
  const col = cols.find((c) => c.colKey === '建筑面积')
  assert.ok(col)
  assert.equal(Number(col.value({ '建筑面积(㎡)': 49.28 })), 49.28)
})

test('维修状态三色（无维修灰 / 待维修橙 / 已修复绿）', () => {
  assert.equal(hrDormRepairTheme('无维修'), 'default')
  assert.equal(hrDormRepairTheme('待维修'), 'warning')
  assert.equal(hrDormRepairTheme('维修中'), 'warning')
  assert.equal(hrDormRepairTheme('已修复'), 'success')
})

test('费用信息列：19 列全带 value，金额两位小数、天数为整数', () => {
  const cols = hrDormChargeColumns() as any[]
  const dataCols = cols.filter((c) => c.colKey !== 'row-select')
  assert.equal(dataCols.length, 20)
  assert.ok(dataCols.every((c) => typeof c.value === 'function'))
  const row = { 入住天数: 31, '扣款合计(元)': 1304.122, 分摊比例: 1 }
  assert.equal(dataCols.find((c) => c.colKey === '入住天数').value(row), '31')
  assert.equal(dataCols.find((c) => c.colKey === '扣款合计(元)').value(row), '1,304.12')
})

test('人事页：五标签删除路由 / 导入三模式 / 设置保存自动关闭', () => {
  const src = readLocal('HrManagement.vue')
  // 子表专属删除，严禁误调账单接口
  assert.match(src, /DORM_DELETE_ROUTE[\s\S]{0,220}dorm: 'rooms'/)
  assert.match(src, /stay: 'stays'/)
  assert.match(src, /asset: 'assets'/)
  assert.match(src, /removeDormTyped\(DORM_DELETE_ROUTE\[dormSubView\.value\], row\.id\)/)
  // 导入三模式与花名册一致
  assert.match(src, /const DORM_IMPORT_MODES = \[[\s\S]{0,160}合并更新[\s\S]{0,80}仅新增[\s\S]{0,80}仅更新/)
  assert.match(src, /:modes="DORM_IMPORT_MODES" default-mode="merge"/)
  // 设置抽屉保存后立即关闭
  assert.match(src, /async function onSettingsSaved\(\) \{\r?\n\s*\/\/[^\n]*\r?\n\s*settingsVisible\.value = false/)
  // 抽屉标题精准
  assert.match(src, /新增宿舍/)
  assert.match(src, /公租房详情/)
  assert.match(src, /安排入住/)
  assert.match(src, /新增资产/)
  // 退宿原因下拉 + 资产状态筛选
  assert.match(src, /CHECKOUT_REASONS/)
  assert.match(src, /ASSET_STATUS_FILTER_OPTIONS/)
  assert.match(src, /DORM_STATUS_FILTER_OPTIONS/)
})
