import { test } from 'node:test'
import assert from 'node:assert/strict'
import { FLEET_CATEGORY_SEEDS, fleetCategorySeedsOf } from './fleetCategorySeed'
import { buildFleetColumns } from './fleetColumns'
import { resolveDefaultCategoryName } from './fleetColumns'

/**
 * 前端同步种子的守护测试（第 34 轮）。
 *
 * 这份数据存在的唯一目的：让首屏第 1 帧就有完整卡片组与完整表头。
 * 因此测试必须锁住「每个菜单都能算出 ≥2 张卡」「每个菜单都能派生 ≥6 列」，
 * 一旦有人改坏某个 scope 的种子，首屏又会退回「2 张卡 + 2 列残缺表头」。
 */

const MENU_SCOPES = ['vehicle', 'driver', 'maintain', 'tire', 'inspection', 'fleet-cost'] as const

test('每个车队 scope 都有同步种子分类，且字段非空', () => {
  for (const scope of MENU_SCOPES) {
    const cats = fleetCategorySeedsOf(scope)
    assert.ok(cats.length > 0, `${scope} 必须有默认分类，否则首屏卡片与表头都是空的`)
    for (const c of cats) {
      assert.ok(c.name.trim().length > 0, `${scope} 分类名不能为空`)
      assert.ok(c.subs.length > 0, `${scope}/${c.name} 必须有字段`)
      for (const s of c.subs) {
        assert.ok(s.name.trim().length > 0, `${scope}/${c.name} 字段名不能为空`)
        assert.ok(s.data_type.length > 0, `${scope}/${c.name}/${s.name} 必须有 data_type`)
      }
    }
  }
})

test('有有效期的归档分类必须含「有效期」类字段（证照状态派生依赖它）', () => {
  // 与 certStatusOf 的候选链同口径（含「强制报废日期」——行驶证唯一的合法性边界）。
  const VALIDITY_FIELD = /有效期|保险止期|质保期止|截止|到期|报废/
  // 例外说明：这两类本身**没有**时间边界，证照状态显示 `—` 是正确行为，
  // 不能为了「让状态有值」而凭空加字段。
  //   · 一般维修 = 维修工单（一次性事件，不是证照）
  //   · 车辆登记证书 = 长期有效的产权凭证（票面只有发证日期）
  //   · 合格证 = 出厂合格证（票面只有进厂/出厂日期，质保期由用户另填）
  const NO_VALIDITY = new Set(['一般维修', '车辆登记证书', '合格证'])
  for (const scope of ['vehicle', 'driver', 'maintain'] as const) {
    for (const c of fleetCategorySeedsOf(scope)) {
      if (NO_VALIDITY.has(c.name)) continue
      const has = c.subs.some((s) => VALIDITY_FIELD.test(s.name))
      assert.ok(has, `${scope}/${c.name} 缺少有效期类字段，证照状态永远算不出来`)
    }
  }
})

test('每个菜单都能派生 >= 6 个业务列（表头绝不残缺）', () => {
  // 真机图 1/2/3：表头只剩「证照状态 | 提取状态」两列或只有复选框。
  // 根因是 activeSubs 为空（分类异步）。种子同步后必须能立刻算出完整列。
  for (const scope of MENU_SCOPES) {
    const cats = fleetCategorySeedsOf(scope)
    const first = resolveDefaultCategoryName(cats)
    assert.ok(first, `${scope} 必须能解析出默认分类（否则默认焦点失效）`)
    const cat = cats.find((c) => c.name === first)!
    const cols = buildFleetColumns(cat.subs, new Map())
    assert.ok(cols.length >= 6, `${scope}/${first} 只派生出 ${cols.length} 列，表头会残缺`)
  }
})

test('每个分类的默认列 >= 4（加上复选框/序号/系统列后表头 >= 6）', () => {
  // 需求硬指标：12 个子菜单的表头列数必须 >= 6。
  // 表头 = 复选框 + 序号 + 默认业务列 (+ 归档类的证照状态/提取状态)，
  // 因此默认业务列至少要 4 个；低于 4 个的分类（辅材记录曾只有 5 列、
  // 请车记录只有 4 列）在真机上看起来就是「表头残缺」。
  const MIN_DEFAULT = 4
  for (const scope of MENU_SCOPES) {
    for (const c of fleetCategorySeedsOf(scope)) {
      const dft = c.subs.filter((s) => s.is_default).length
      assert.ok(dft >= MIN_DEFAULT,
        `${scope}/${c.name} 只有 ${dft} 个默认列（需 >= ${MIN_DEFAULT}），表头会不足 6 列`)
    }
  }
})

test('9 个手工录入菜单的默认分类都有「车牌号」（归属键）', () => {
  // 记录型分类首个字段必须是车牌号：它既是车辆归属键，也是筛选器来源。
  const RECORD_SCOPES: Record<string, string[]> = {
    tire: ['轮胎管理'],
    inspection: ['年检管理'],
    'fleet-cost': ['加油充电', '路桥费', '维修保养费', '保险理赔', '违章处理', '辅材记录', '请车记录'],
  }
  for (const [scope, names] of Object.entries(RECORD_SCOPES)) {
    const cats = fleetCategorySeedsOf(scope)
    for (const name of names) {
      const c = cats.find((x) => x.name === name)
      assert.ok(c, `${scope}/${name} 种子缺失`)
      assert.ok(
        c!.subs.some((s) => s.name === '车牌号'),
        `${scope}/${name} 缺少车牌号字段`,
      )
    }
  }
})

test('fleetCategorySeedsOf 返回浅拷贝：调用方改动不会污染常量', () => {
  const a = fleetCategorySeedsOf('tire')
  a[0].subs[0].name = '被改坏'
  const b = fleetCategorySeedsOf('tire')
  assert.notEqual(b[0].subs[0].name, '被改坏', '种子常量被调用方污染了')
  assert.equal(FLEET_CATEGORY_SEEDS.tire[0].subs[0].name, b[0].subs[0].name)
})

test('未知 scope 返回空数组（不抛错）', () => {
  assert.deepEqual(fleetCategorySeedsOf('unknown-scope'), [])
})
