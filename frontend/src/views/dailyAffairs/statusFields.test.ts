// 系统内置状态字段（第 44 轮）的单元测试。
//
// 覆盖三件事，任何一件回退都会让真机断言 3 直接失败：
//   ① 状态字段在字段定义里必须「自动出现」（缺失时补齐）；
//   ② 名称/类型/启用/删除锁死（不是靠页面里散落的字符串判断）；
//   ③ 补齐只补缺失，已存在的别名不重复造列、用户列宽与顺序不被覆盖。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  isCertStatusName,
  isExtractStatusName,
  isBillStatusName,
  isStatusFieldName,
  isLockedStatusField,
  statusFieldSpecsOf,
  ensureStatusFields,
  type StatusFieldSpec,
} from './statusFields'

test('isCertStatusName：覆盖全部历史别名与英文键', () => {
  for (const n of ['证照状态', '证件状态', '驾驶证状态', '保单状态', '证书状态', 'certStatus', 'cert_status']) {
    assert.equal(isCertStatusName(n), true, `${n} 应识别为证照状态`)
  }
  for (const n of ['提取状态', '备注', '状态', '', null, undefined]) {
    assert.equal(isCertStatusName(n), false, `${String(n)} 不应识别为证照状态`)
  }
})

test('isExtractStatusName / isBillStatusName / isStatusFieldName', () => {
  assert.equal(isExtractStatusName('提取状态'), true)
  assert.equal(isExtractStatusName('extractStatus'), true)
  assert.equal(isExtractStatusName('extract_status'), true)
  assert.equal(isExtractStatusName('证照状态'), false)

  assert.equal(isBillStatusName('状态'), true)
  assert.equal(isBillStatusName('对比状态'), true)
  assert.equal(isBillStatusName('提取状态'), false)

  assert.equal(isStatusFieldName('证照状态'), true)
  assert.equal(isStatusFieldName('extractStatus'), true)
  // 账单「状态」不算列表派生列（它是文件级固定列），不能被列表过滤掉
  assert.equal(isStatusFieldName('状态'), false)
})

test('statusFieldSpecsOf：档案类两个、发票/合同一个、账单类只有市电/光伏', () => {
  for (const scope of ['vehicle', 'driver', 'maintain']) {
    const specs = statusFieldSpecsOf(scope)
    assert.deepEqual(specs.map((s) => s.name), ['证照状态', '提取状态'], `${scope} 必须两个状态列`)
  }
  assert.deepEqual(statusFieldSpecsOf('invoice').map((s) => s.name), ['extractStatus'])
  assert.deepEqual(statusFieldSpecsOf('contract').map((s) => s.name), ['extractStatus'])

  // 市电/光伏：只补展示（virtual），落库会产生重复列
  const bill = statusFieldSpecsOf('energy', '市电账单')
  assert.equal(bill.length, 1)
  assert.equal(bill[0].name, '提取状态')
  assert.equal(bill[0].virtual, true)
  assert.equal(statusFieldSpecsOf('energy', '光伏账单')[0].virtual, true)
  // 三张核算表 / 费用清单是纯手工核算，不注入
  assert.deepEqual(statusFieldSpecsOf('energy', '电费核算'), [])
  assert.deepEqual(statusFieldSpecsOf('energy', '费用清单'), [])
  // 轮胎/年检/成本没有状态列
  assert.deepEqual(statusFieldSpecsOf('tire'), [])
  assert.deepEqual(statusFieldSpecsOf('inspection'), [])
  assert.deepEqual(statusFieldSpecsOf('fleet-cost'), [])
})

test('isLockedStatusField：只锁身份，不把普通业务字段一起锁死', () => {
  // 档案类：证照状态 + 提取状态都锁
  assert.equal(isLockedStatusField('vehicle', '道路运输证', '证照状态'), true)
  assert.equal(isLockedStatusField('vehicle', '道路运输证', '证件状态'), true)
  assert.equal(isLockedStatusField('vehicle', '道路运输证', '提取状态'), true)
  // 普通字段不锁
  assert.equal(isLockedStatusField('vehicle', '道路运输证', '许可证号'), false)

  // 发票：只锁提取状态（发票类型虽有 builtin 标记，但不在本函数口径内）
  assert.equal(isLockedStatusField('invoice', '专用发票', 'extractStatus'), true)
  assert.equal(isLockedStatusField('invoice', '专用发票', 'invoiceType'), false)

  // 账单类：状态 / 对比状态（仅市电/光伏）
  assert.equal(isLockedStatusField('energy', '市电账单', '状态'), true)
  assert.equal(isLockedStatusField('energy', '光伏账单', '对比状态'), true)
  assert.equal(isLockedStatusField('energy', '电费核算', '状态'), false)

  // 手工台账不锁任何字段
  assert.equal(isLockedStatusField('tire', '轮胎管理', '状态'), false)
})

test('ensureStatusFields：缺失补齐为启用+内置，已存在别名不重复', () => {
  const specs: StatusFieldSpec[] = statusFieldSpecsOf('vehicle')
  const fields: any[] = [
    { name: '许可证号', enabled: true, isDefault: true, dataType: 'text', width: 0 },
    { name: '证件状态', enabled: false, isDefault: false, dataType: 'text', width: 180 },
  ]
  ensureStatusFields(fields, specs)
  // 「证件状态」被识别为证照状态别名 → 不新增「证照状态」，但强制启用+内置
  assert.equal(fields.filter((f) => isCertStatusName(f.name)).length, 1)
  assert.equal(fields[1].enabled, true)
  assert.equal(fields[1].builtin, true)
  // 用户列宽不被覆盖
  assert.equal(fields[1].width, 180)
  // 提取状态缺失 → 追加在末尾
  const appended = fields.find((f) => f.name === '提取状态')
  assert.ok(appended, '提取状态必须被补齐')
  assert.equal(appended.enabled, true)
  assert.equal(appended.isDefault, true)
  assert.equal(appended.builtin, true)
  assert.equal(appended.width, 0) // 列宽默认 0（自适应），输入框显示 0 而不是空白
})

test('ensureStatusFields：virtual 字段只做展示（落库由 toSubsPayload 过滤）', () => {
  const fields: any[] = [{ name: 'bill_period', enabled: true, isDefault: true, dataType: 'text', width: 0 }]
  ensureStatusFields(fields, statusFieldSpecsOf('energy', '市电账单'))
  const st = fields.find((f) => f.name === '提取状态')
  assert.ok(st)
  assert.equal(st.virtual, true)
})

test('ensureStatusFields：不传 specs 时原样返回（不猜、不加列）', () => {
  const fields: any[] = [{ name: '车牌号', enabled: true, isDefault: true, dataType: 'text', width: 0 }]
  const out = ensureStatusFields(fields, [])
  assert.equal(out, fields)
  assert.equal(out.length, 1)
})
