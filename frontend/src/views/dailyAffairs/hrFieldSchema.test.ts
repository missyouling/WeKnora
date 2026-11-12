/**
 * 第 52 轮：字段类型 / 枚举下拉 / 身份证自动推导 / 数据清洗校验 / 三套导入模板契约。
 *
 * 全部是纯函数，直接 tsx --test 运行，不依赖 Vue / DOM / 网络。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  HR_ENUMS, hrEnumsOf, HR_DATE_FIELDS, hrIsDateField, hrDataTypeOf,
  HR_ROSTER_FIELDS, HR_ROSTER_GROUPS, HR_RESIGN_FIELDS, HR_CONTRACT_FIELDS,
  hrModuleGroups, hrTemplateHeaders, hrTemplateEnumGroups, HR_TEMPLATE_NAMES,
  HR_TEMPLATE_MATCH_KEYS,
  ID_CARD_RE, hrIsValidIdCard, hrIsStrictIdCard, hrParseIdCard, hrAgeOf, hrSeniorityOf,
  hrRegularDateOf, hrCleanValue, hrCleanForm, hrValidateField, hrValidateForm,
  hrIsValidPhone, hrIsValidBankCard, hrValueKeys,
} from './hrFieldSchema'

// ---------------------------------------------------------------------------
// 字段类型：日期一律 date，内置枚举一律有 options
// ---------------------------------------------------------------------------

test('11 个日期字段全部登记，且 dataType 为 date', () => {
  assert.equal(HR_DATE_FIELDS.length, 11)
  for (const name of ['入职日期', '转正日期', '身份证有效期', '出生日期', '毕业时间',
    '离职申请日期', '离职日期', '薪资结算日', '合同开始日期', '合同结束日期', '签订日期']) {
    assert.ok(hrIsDateField(name), `${name} 必须是日期字段`)
  }
  const all = [...HR_ROSTER_FIELDS, ...HR_RESIGN_FIELDS, ...HR_CONTRACT_FIELDS]
  for (const d of all) {
    if (hrIsDateField(d.name)) assert.equal(hrDataTypeOf(d), 'date', `${d.name} 应渲染 t-date-picker`)
  }
})

test('内置枚举字段全部有下拉选项（t-select filterable）', () => {
  for (const d of [...HR_ROSTER_FIELDS, ...HR_RESIGN_FIELDS, ...HR_CONTRACT_FIELDS]) {
    if (d.type === 'enum' && !d.dynamic) {
      assert.ok((d.options || []).length > 0, `枚举字段 ${d.name} 缺少 options`)
    }
  }
  assert.equal(HR_ENUMS['离职原因'].length, 42, '离职原因应为 42 项标准模板')
  assert.equal(HR_ENUMS['民族'].length >= 56, true)
  assert.equal(HR_ENUMS['试用期'].length, 13)
  assert.equal(HR_ENUMS['合同期限'].length, 11)
  assert.deepEqual(hrEnumsOf('性别'), ['男', '女'])
  assert.deepEqual(hrEnumsOf('不存在的字段'), [])
})

test('动态枚举：部门 / 合同公司 / 签约公司标记 dynamic', () => {
  const all = [...HR_ROSTER_FIELDS, ...HR_CONTRACT_FIELDS]
  assert.equal(all.find((d) => d.name === '部门')?.dynamic, 'department')
  assert.equal(all.find((d) => d.name === '合同公司')?.dynamic, 'unit')
  assert.equal(all.find((d) => d.name === '签约公司')?.dynamic, 'unit')
})

test('花名册编辑抽屉按 6 大分组，离职/合同各按单组', () => {
  assert.deepEqual(hrModuleGroups('roster').map((g) => g.key), [
    '在职信息', '基本信息', '联系信息', '紧急联系人情况', '教育情况', '工资社保信息',
  ])
  assert.equal(hrModuleGroups('resign').length, 1)
  assert.equal(hrModuleGroups('contract').length, 1)
  for (const mod of ['roster', 'resign', 'contract'] as const) {
    const fields = hrModuleGroups(mod).flatMap((g) => g.fields)
    assert.equal(new Set(fields.map((d) => d.name)).size, fields.length, `${mod} 字段名必须去重`)
  }
})

// ---------------------------------------------------------------------------
// 身份证自动推导（真机断言用 51072319850217371X）
// ---------------------------------------------------------------------------

test('合法身份证 51072319850217371X → 出生日期 1985-02-17 / 男 / 年龄 1 位小数', () => {
  assert.equal(ID_CARD_RE.test('51072319850217371X'), true)
  const info = hrParseIdCard('51072319850217371X', new Date('2026-11-12T00:00:00Z'))
  assert.ok(info)
  assert.equal(info.birthDate, '1985-02-17')
  assert.equal(info.gender, '男')
  assert.equal(info.age, 41.7)
})

test('身份证清洗：去空格 / 横杠 / 末位小写 x 转大写 X', () => {
  assert.equal(hrCleanValue('证件号码', ' 5107 2319 8502 1737 1x '), '51072319850217371X')
  assert.equal(hrCleanValue('手机号码', '138-0013-8000'), '13800138000')
  assert.equal(hrCleanValue('工资卡卡号', '6217 0037 6015 3123 456'), '6217003760153123456')
  assert.equal(hrCleanValue('姓名', '  张\u200B三  '), '张三')
})

test('身份证校验：格式 / 出生年月日合法性；校验码单独提供严格口径', () => {
  assert.equal(hrIsValidIdCard('51072319850217371X'), true)
  assert.equal(hrIsValidIdCard('51072319850217371x'), true, '小写 x 清洗后应合法')
  assert.equal(hrIsValidIdCard('51072319850217371'), false, '17 位不合法')
  assert.equal(hrIsValidIdCard('51072319851317371X'), false, '月份 13 不合法')
  assert.equal(hrIsValidIdCard('51072319850230371X'), false, '2 月 30 日不合法')
  // 任务给定的样例号校验位按 GB11643 应为 0，严格口径识别出来但不阻断录入
  assert.equal(hrIsStrictIdCard('51072319850217371X'), false)
  assert.equal(hrIsStrictIdCard('510723198502173710'), true)
})

test('手机号 / 座机 / 工资卡校验', () => {
  assert.equal(hrIsValidPhone('13800138000'), true)
  assert.equal(hrIsValidPhone('0755-2342343'), true)
  assert.equal(hrIsValidPhone('12345'), false)
  assert.equal(hrIsValidBankCard('6217003760153123456'), true)
  assert.equal(hrIsValidBankCard('6217-0037'), false)
  assert.equal(hrIsValidBankCard('6217abc'), false)
})

test('年龄 / 司龄 / 转正日期按日期精确计算', () => {
  const today = new Date('2026-11-12T00:00:00Z')
  assert.equal(hrAgeOf('1985-02-17', today), 41.7)
  assert.equal(hrSeniorityOf('2018-07-01', today), '8年4个月')
  assert.equal(hrRegularDateOf('2018-07-01', '3个月'), '2018-10-01')
  assert.equal(hrRegularDateOf('2018-07-01', '无试用期'), '2018-07-01')
  assert.equal(hrRegularDateOf('', '3个月'), '')
})

test('表单校验：非法证件号 / 手机号 / 卡号给出清晰错误提示', () => {
  assert.equal(hrValidateField('证件号码', '51072319851317371X', { 证件类型: '身份证' }),
    '请输入合法的 18 位身份证号码')
  assert.equal(hrValidateField('证件号码', '51072319850217371X', { 证件类型: '身份证' }), '')
  assert.equal(hrValidateField('手机号码', '12345'), '请输入正确的 11 位手机号码')
  assert.equal(hrValidateField('紧急联系人电话', '0755-2342343'), '')
  assert.equal(hrValidateField('工资卡卡号', '6217abc'), '请输入 12~25 位数字的工资卡卡号')
  assert.equal(hrValidateForm({ 证件号码: '51072319850217371X', 手机号码: '13800138000' }), '')
  assert.match(hrValidateForm({ 手机号码: '123' }), /11 位手机号码/)
})

test('整表清洗：字符串 trim + 号码去空格横杠，卡号绝不转数值', () => {
  const out = hrCleanForm({
    姓名: '  张三 ', 证件号码: '5107 2319 8502 1737 1x',
    手机号码: '138-0013-8000', 工资卡卡号: '6217 0037', 年龄: 41.7,
  })
  assert.equal(out['姓名'], '张三')
  assert.equal(out['证件号码'], '51072319850217371X')
  assert.equal(out['手机号码'], '13800138000')
  assert.equal(out['工资卡卡号'], '62170037')
  assert.equal(typeof out['工资卡卡号'], 'string')
  assert.equal(out['年龄'], 41.7)
})

// ---------------------------------------------------------------------------
// 三套 Excel 导入模板
// ---------------------------------------------------------------------------

test('三套模板文件名各不相同', () => {
  assert.equal(new Set(Object.values(HR_TEMPLATE_NAMES)).size, 3)
  assert.equal(HR_TEMPLATE_NAMES.roster, '花名册导入模板.xlsx')
  assert.equal(HR_TEMPLATE_NAMES.resign, '离职管理导入模板.xlsx')
  assert.equal(HR_TEMPLATE_NAMES.contract, '劳动合同导入模板.xlsx')
})

test('花名册模板表头 = 精简后 48 字段，不含删除/剥离字段', () => {
  const headers = [...hrTemplateHeaders('roster')]
  assert.equal(headers.length, 48)
  assert.deepEqual(headers, HR_ROSTER_FIELDS.map((d) => d.name))
  for (const removed of ['在职状态', '人员状态', '婚姻状况', '配偶姓名', '上家工作单位',
    '是否烈属', '现合同编号', '合同状态', '离职日期', '离职证明']) {
    assert.ok(!headers.includes(removed), `${removed} 不应出现在花名册模板`)
  }
})

test('离职管理模板 = 3 匹配键 + 8 离职字段（附件不进模板）', () => {
  const headers = [...hrTemplateHeaders('resign')]
  assert.deepEqual(headers.slice(0, 3), [...HR_TEMPLATE_MATCH_KEYS])
  assert.equal(headers.length, 11)
  assert.ok(!headers.includes('离职证明'))
  for (const k of ['离职申请日期', '离职日期', '离职类型', '离职原因',
    '离职原因说明', '离职去向', '是否试用期内离职', '薪资结算日']) {
    assert.ok(headers.includes(k), `离职模板缺少 ${k}`)
  }
})

test('劳动合同模板 = 3 匹配键 + 7 合同字段（合同状态系统计算不进模板）', () => {
  const headers = [...hrTemplateHeaders('contract')]
  assert.deepEqual(headers.slice(0, 3), [...HR_TEMPLATE_MATCH_KEYS])
  assert.equal(headers.length, 10)
  assert.ok(!headers.includes('合同状态'))
  for (const k of ['合同编号', '合同类型', '合同开始日期', '合同结束日期',
    '合同期限', '签约公司', '签订日期']) {
    assert.ok(headers.includes(k), `合同模板缺少 ${k}`)
  }
})

test('模板枚举 Sheet 只包含该模块真实存在的枚举列', () => {
  const roster = hrTemplateEnumGroups('roster').map((g) => g.group)
  assert.ok(roster.includes('试用期') && roster.includes('最高学历') && roster.includes('银行名称'))
  assert.ok(!roster.includes('离职类型') && !roster.includes('合同类型'))
  const resign = hrTemplateEnumGroups('resign').map((g) => g.group)
  assert.deepEqual(resign, ['离职类型', '离职原因', '是否试用期内离职'])
  const contract = hrTemplateEnumGroups('contract').map((g) => g.group)
  assert.deepEqual(contract, ['合同类型', '合同期限'])
})

test('新合同字段名与存量 custom_data 键双向兼容', () => {
  assert.ok(hrValueKeys('合同结束日期').includes('现合同结束日期'))
  assert.ok(hrValueKeys('签订日期').includes('现合同签订日期'))
  assert.ok(hrValueKeys('联系电话').includes('手机号码'))
})

test('三套模板表头互不重复字段集合且各自无重复列', () => {
  for (const mod of ['roster', 'resign', 'contract'] as const) {
    const h = hrTemplateHeaders(mod)
    assert.equal(new Set(h).size, h.length, `${mod} 模板表头有重复列`)
  }
})
