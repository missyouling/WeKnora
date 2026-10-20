import assert from 'node:assert/strict'
import test from 'node:test'
import {
  STD_DATA_KEYS,
  EMPTY_DETAIL_FORM,
  numToStr,
  toNumber,
  sanitizeNum,
  tagsToText,
  tagsFromText,
  collectRemarkParts,
  fillDetailForm,
  mergeRemarkParts,
  itemToPayload,
  pickDynamicFields,
  buildDetailPayload,
  buildDetailFieldDefs,
  isTaxKey,
  findInvoiceIndex,
  cascadeInvoiceType,
  hasInvoiceData,
  extractStatusOf,
  mergeRowAfterSave,
  isDetailFormDirty,
  type FillDetailContext,
} from './businessDetailForm'

// 注入的业务字典/工具（与 invoiceFieldLabels 的真实行为同形）
const DICT: Record<string, string> = {
  invoiceNo: '发票号码', invoiceDate: '开票日期', invoiceType: '发票类型',
  amount: '金额', taxRate: '税率', tax: '税额', totalAmount: '价税合计',
  buyerName: '购买方', sellerName: '销售方', issuer: '开票人', remark: '备注',
}
const REMARK_PARTS = new Set(['收款方式', '审核'])
const ctx = (over: Partial<FillDetailContext> = {}): FillDetailContext => ({
  row: {},
  cats: [],
  itemSubFields: [],
  isRemarkPart: (n: string) => REMARK_PARTS.has(n),
  remarkPartOf: (_r: string, n: string) => (n === '收款方式' ? '转账' : ''),
  fieldKeyOf: (n: string) => n,
  dict: DICT,
  ...over,
})

test('基础清洗：空值与脏数据都不炸', () => {
  assert.equal(numToStr(undefined), '')
  assert.equal(numToStr(0), '0', '0 是有效值')
  assert.equal(toNumber(''), undefined)
  assert.equal(toNumber('abc'), undefined)
  assert.equal(toNumber('12.5'), 12.5)
  assert.equal(toNumber(0), 0)
  assert.equal(sanitizeNum('1.2.3'), '1.23', '只保留第一个小数点')
  assert.equal(sanitizeNum('12a3'), '123')
})

test('标签：数组↔换行文本互逆，空行丢弃', () => {
  assert.equal(tagsToText(['a', 'b']), 'a\nb')
  assert.equal(tagsToText([{ name: 'x' }, 'y']), 'x\ny')
  assert.equal(tagsToText(undefined), '')
  assert.deepEqual(tagsFromText(' a \n\nb '), ['a', 'b'])
  assert.deepEqual(tagsFromText(['a']), ['a'], '数组原样返回')
  assert.deepEqual(tagsFromText(''), [])
})

test('备注片段拆解：只收 remarkPart 字段，其余不进 remarkParts', () => {
  const parts = collectRemarkParts(
    [{ name: '收款方式' }, { name: '审核' }, { name: 'invoiceNo' }],
    '收款方式：转账\n审核：张三',
    (n) => REMARK_PARTS.has(n),
    (_r, n) => (n === '收款方式' ? '转账' : '张三'),
  )
  assert.deepEqual(parts, { 收款方式: '转账', 审核: '张三' })
  assert.deepEqual(collectRemarkParts([], 'x', () => true, () => ''), {})
  assert.deepEqual(collectRemarkParts(undefined as never, 'x', () => true, () => ''), {})
})

test('fillDetailForm：分类判定顺序 = 提取类型 > 行 category > 列表筛选类型', () => {
  const cats = [{ name: '通行费发票', enabled: true, subs: [{ name: '车牌号' }] }, { name: '普通发票', enabled: true, subs: [] }]
  // 提取类型优先于行级 category（与列表口径一致）
  const form = fillDetailForm(ctx({
    row: { invoiceType: '普通发票', category: '通行费发票', invoiceNo: 'NO1', amount: 10 },
    cats,
    filterType: '通行费发票',
  }))
  assert.equal(form.invoice_type, '普通发票')
  // 无提取类型时取行级 category
  assert.equal(fillDetailForm(ctx({ row: { category: '通行费发票' }, cats })).invoice_type, '通行费发票')
  // 都没有时回落列表筛选类型
  assert.equal(fillDetailForm(ctx({ row: {}, cats, filterType: '普通发票' })).invoice_type, '普通发票')
  // 未命中任何分类：字段按第一个启用分类渲染，但发票类型本身保留原值
  // （下拉是 allow-create，历史脏类型不能被静默改写成"看起来合法"的分类）
  const f2 = fillDetailForm(ctx({ row: { invoiceType: '不存在的分类' }, cats }))
  assert.equal(f2.invoice_type, '不存在的分类')
  // 停用分类不参与命中
  const f3 = fillDetailForm(ctx({ row: { invoiceType: '停用' }, cats: [{ name: '停用', enabled: false, subs: [{ name: '停用字段' }] }, { name: '在用的', enabled: true, subs: [{ name: '在用字段' }] }] }))
  assert.equal(f3.invoice_type, '停用', '类型文本保留原值')
  // 停用分类不参与命中：字段按「在用的」渲染（动态字段来自它的 subs）
  assert.equal(f3.data['在用字段'], '')
  assert.equal('停用字段' in f3.data, false)
})

test('fillDetailForm：动态字段取值优先级 fields → 行对象 → 明细行 extra', () => {
  const cats = [{ name: '通行费发票', subs: [{ name: '车牌号' }] }]
  const base = ctx({ cats, row: { invoiceType: '通行费发票' } })
  assert.equal(fillDetailForm({ ...base, row: { ...base.row, fields: { 车牌号: '渝A1' } } }).data['车牌号'], '渝A1')
  assert.equal(fillDetailForm({ ...base, row: { ...base.row, 车牌号: '渝B2' } }).data['车牌号'], '渝B2')
  assert.equal(
    fillDetailForm({ ...base, row: { ...base.row, items: [{ name: '通行费', extra: { 车牌号: '渝C3' } }] } }).data['车牌号'],
    '渝C3',
  )
  // 标准契约键不进动态桶
  assert.equal(fillDetailForm({ ...base, row: { ...base.row, invoiceNo: 'X' } }).data['invoiceNo'], 'X')
})

test('fillDetailForm：明细行税率百分比化、缺失金额/税额回退', () => {
  const form = fillDetailForm(ctx({
    row: {
      tax: 7.03,
      items: [
        { name: '通行费', price: 234.25, tax_rate: 0.03, tax_amount: 7.03 },
        { name: '无金额', price: 10 },
      ],
    },
  }))
  assert.equal(form.items[0].tax_rate, '3', '0.03 → "3"')
  assert.equal(form.items[1].amount, '10', '数量为空时单价即金额')
  assert.equal(form.items[1].tax_amount, '7.03', '税额缺失回退发票级税额')
  assert.equal(form.items[0].tax_rate, '3')
  // 明细子列回填：extra → fields → remark 片段
  const f2 = fillDetailForm(ctx({
    itemSubFields: [{ key: 'plate_no', label: '车牌号' }],
    row: { remark: '车牌号：渝D4', items: [{ name: 'x' }] },
    remarkPartOf: (_r: string, n: string) => (n === '车牌号' ? '渝D4' : ''),
  }))
  assert.equal(f2.items[0].plate_no, '渝D4')
  assert.equal(f2.items[0].extra.plate_no, '渝D4', '自定义子列同步写 extra 桶')
})

test('备注片段回写：逐片段合并，不丢其它内容', () => {
  const setPart = (remark: string, part: string, val: string) => `${remark}\n${part}：${val}`.trim()
  assert.equal(mergeRemarkParts('原备注', { 收款方式: '转账' }, setPart), '原备注\n收款方式：转账')
  assert.equal(mergeRemarkParts('原备注', {}, setPart), '原备注')
})

test('itemToPayload：税率百分比转回小数，自定义子列原样写行键', () => {
  const p = itemToPayload({ name: '通行费', tax_rate: '3', amount: '234.25', plate_no: '渝A1', extra: { travel_from: '20250505' } })
  assert.equal(p.tax_rate, 0.03, '"3" → 0.03')
  assert.equal(p.amount, 234.25)
  assert.equal(p.plate_no, '渝A1')
  assert.equal(p.travel_from, '20250505', 'extra 桶展开为行键供后端收集')
  assert.equal(p.extra, undefined, 'extra 本身不落库')
  // 空税率不产生 0
  assert.equal(itemToPayload({ tax_rate: '' }).tax_rate, undefined)
})

test('pickDynamicFields：标准键不入桶，0/空串保留', () => {
  const dyn = pickDynamicFields({ invoiceNo: 'X', 车牌号: '渝A1', 户号: '' })
  assert.deepEqual(dyn, { 车牌号: '渝A1', 户号: '' })
  assert.equal('invoiceNo' in dyn, false)
  assert.equal(pickDynamicFields({ invoiceNo: 'X' }).fields ?? Object.keys(pickDynamicFields({})).length, 0)
  // 自定义标准键集合
  assert.deepEqual(pickDynamicFields({ a: 1 }, ['a']), {})
  assert.ok(STD_DATA_KEYS.includes('invoiceNo'))
})

test('buildDetailPayload：category 与 invoice_type 联动 + tags 归一 + 动态桶', () => {
  const payload = buildDetailPayload({
    data: { invoiceNo: 'NO1', invoiceDate: '2026-01-01', amount: '10.5', tags: ' a \nb ', 车牌号: '渝A1', remark: 'r' },
    remarkParts: { 收款方式: '转账' },
    invoiceType: '通行费发票',
    items: [{ name: 'x', tax_rate: '3' }],
    setRemarkPart: (r, p, v) => `${r}\n${p}：${v}`,
  })
  assert.equal(payload.invoice_type, '通行费发票')
  assert.equal(payload.category, '通行费发票', '不同步会让记录从分类视图消失')
  assert.equal(payload.total_amount, undefined, '未填的字段不写 0')
  assert.equal(payload.amount, 10.5)
  assert.deepEqual(payload.tags, ['a', 'b'])
  assert.equal(payload.remark, 'r\n收款方式：转账')
  assert.deepEqual(payload.fields, { 车牌号: '渝A1' })
  assert.equal(payload.items[0].tax_rate, 0.03)
})

test('findInvoiceIndex：页码优先，发票号兜底，找不到 -1', () => {
  const invoices = [{ invoice_no: 'A', page: 1 }, { invoice_no: 'B', page: 2 }, { invoice_no: 'C', page: 3 }]
  assert.equal(findInvoiceIndex(invoices, 2, 'A'), 1, '页码优先于发票号')
  assert.equal(findInvoiceIndex(invoices, 0, 'C'), 2, '无页码时按发票号')
  assert.equal(findInvoiceIndex(invoices, 9, 'X'), -1)
  assert.equal(findInvoiceIndex([], 1, 'A'), -1)
  assert.equal(findInvoiceIndex(undefined as never, 1, 'A'), -1)
})

test('cascadeInvoiceType：类型实质变更时同文件其它发票一起同步', () => {
  const invoices: any[] = [{ invoice_type: '旧', category: '旧' }, { invoice_type: '旧', category: '旧' }]
  cascadeInvoiceType(invoices, 0, '新')
  assert.deepEqual(invoices.map((i) => i.invoice_type), ['旧', '新'], '目标行由后续 merge 覆盖')
  assert.deepEqual(invoices.map((i) => i.category), ['旧', '新'])
  // 类型未变 → 不动
  const same: any[] = [{ invoice_type: 'X' }, { invoice_type: 'X' }]
  cascadeInvoiceType(same, 0, 'X')
  assert.deepEqual(same.map((i) => i.invoice_type), ['X', 'X'])
  // 越界/空类型安全
  assert.deepEqual(cascadeInvoiceType([{ invoice_type: 'a' }], 9, 'b'), [{ invoice_type: 'a' }])
  assert.deepEqual(cascadeInvoiceType([{ invoice_type: 'a' }], 0, ''), [{ invoice_type: 'a' }])
})

test('hasInvoiceData / extractStatusOf：空数据保持"待补录"不消失', () => {
  assert.equal(hasInvoiceData([{ invoice_no: 'A' }]), true)
  assert.equal(hasInvoiceData([{ amount: 0, total_amount: 0 }]), false)
  assert.equal(hasInvoiceData([]), false)
  assert.equal(hasInvoiceData([{ seller_name: '销售方' }]), true)
  assert.deepEqual(extractStatusOf(true), { status: 'success', error: '' })
  assert.deepEqual(extractStatusOf(false), { status: 'manual', error: '人工入库待编辑，请补充字段' })
})

test('mergeRowAfterSave：列表行局部同步，未提供的字段保持原值', () => {
  const row = { rowKey: 'k', voidFlag: true, items: [], fields: { a: 1 } }
  const next = mergeRowAfterSave(row, { invoice_no: 'N', invoice_type: 'T', amount: 1 }, 'success', '')
  assert.equal(next.invoiceNo, 'N')
  assert.equal(next.invoiceType, 'T')
  assert.equal(next.extractStatus, 'success')
  assert.equal(next.voidFlag, true)
  assert.deepEqual(next.fields, { a: 1 }, '载荷无 fields 时保留行上原有扩展桶')
})

test('isDetailFormDirty：能识别 data / remarkParts / items / 类型的改动', () => {
  const a = { invoice_no: 'N', invoice_type: 'T', data: { amount: '1' }, remarkParts: { p: 'v' }, items: [{ name: 'x' }] }
  const clone = () => JSON.parse(JSON.stringify(a))
  assert.equal(isDetailFormDirty(a, clone()), false)
  const b = clone(); b.data.amount = '2'
  assert.equal(isDetailFormDirty(a, b), true)
  const c = clone(); c.remarkParts.p = 'v2'
  assert.equal(isDetailFormDirty(a, c), true)
  const d = clone(); d.items[0].name = 'y'
  assert.equal(isDetailFormDirty(a, d), true)
  const e = clone(); e.invoice_type = 'T2'
  assert.equal(isDetailFormDirty(a, e), true)
  // 空值口径：'' 与 undefined 视同相等，避免打开抽屉就"脏"
  const f = clone(); f.data.amount = undefined
  assert.equal(isDetailFormDirty({ ...a, data: { amount: '' } }, f), false)
  // 缺表单/缺基线不炸
  assert.equal(isDetailFormDirty(null, a), false)
  assert.equal(isDetailFormDirty(a, undefined), false)
  assert.equal(isDetailFormDirty(EMPTY_DETAIL_FORM, EMPTY_DETAIL_FORM), false)
})

// ---- buildDetailFieldDefs：字段清洗（税额去重 / items 跳过 / 核心置顶） ----

// 模拟发票字段映射（与 invoiceFieldLabels 同形，仅覆盖被测键）
const FK: Record<string, string> = {
  税额: 'tax', 项目明细: 'items', 规格型号: 'spec', 单位: 'unit', 数量: 'qty', 单价: 'price', 项目名称: 'name',
}
const LBL: Record<string, string> = { tax: '税额', tax_amount: '税额', taxAmount: '税额', items: '项目明细' }
const fk = (n: string) => FK[n] || n
const lbl = (n: string) => LBL[n] || n
const isRp = () => false

test('buildDetailFieldDefs：税额同义键（tax/taxAmount/tax_amount）只保留一个「税额」', () => {
  const subs = [
    { name: 'invoiceNo' },
    { name: 'tax', label: '税额' },
    { name: 'tax_amount', label: '税额' },
    { name: 'taxAmount', label: '税额' },
  ]
  const defs = buildDetailFieldDefs(subs, { fieldKeyOf: fk, labelOf: lbl, isRemarkPart: isRp })
  const taxDefs = defs.filter((d) => d.name === 'tax' || d.name === 'tax_amount' || d.name === 'taxAmount')
  assert.equal(taxDefs.length, 1, '只保留一个税额')
  assert.equal(taxDefs[0].label, '税额')
})

test('buildDetailFieldDefs：明细容器 items 不生成主表单占位行', () => {
  const subs = [
    { name: 'invoiceNo' },
    { name: 'items', data_type: 'items', sub_fields: [{ name: 'spec' }, { name: 'unit' }] },
  ]
  const defs = buildDetailFieldDefs(subs, { fieldKeyOf: fk, labelOf: lbl, isRemarkPart: isRp })
  assert.equal(defs.some((d) => d.name === 'items' || d.dataType === 'items'), false)
})

test('buildDetailFieldDefs：明细子项键（spec/unit/qty/price/name）错位混入主表被跳过', () => {
  const subs = [
    { name: 'invoiceNo' },
    { name: 'items', data_type: 'items', sub_fields: [{ name: 'spec' }, { name: 'unit' }, { name: 'qty' }, { name: 'price' }, { name: 'name' }] },
    { name: 'spec' }, { name: 'unit' }, { name: 'qty' }, { name: 'price' }, { name: 'name' },
  ]
  const defs = buildDetailFieldDefs(subs, { fieldKeyOf: fk, labelOf: lbl, isRemarkPart: isRp })
  for (const k of ['spec', 'unit', 'qty', 'price', 'name']) {
    assert.equal(defs.some((d) => d.name === k), false, `${k} 不应出现在主表单`)
  }
})

test('buildDetailFieldDefs：核心字段（发票号码/开票日期/发票类型）强制置顶', () => {
  const subs = [
    { name: 'amount' }, { name: 'tax' }, { name: 'invoiceDate' }, { name: 'invoiceNo' }, { name: 'invoiceType' },
  ]
  const defs = buildDetailFieldDefs(subs, { fieldKeyOf: fk, labelOf: lbl, isRemarkPart: isRp })
  assert.deepEqual(defs.slice(0, 3).map((d) => d.name), ['invoiceNo', 'invoiceDate', 'invoiceType'])
})

// 回归（2026-10-20）：通行费发票分类的 items 子列含「金额 amount」「税率 tax_rate」
// 「车牌号 plate_no」，而 buildDetailFieldDefs 原先按「items 子列键集合」动态排除，
// 把主表标准契约字段 amount 一并过滤 → 编辑抽屉缺失「金额」输入框，
// 顶部质量告警还提示「金额：用户配置为必填，但未提取到该字段」。
// 契约：标准契约字段拥有最高保留优先级，明细子项排除仅限纯明细专属键。
test('buildDetailFieldDefs：items 子列含 amount/tax_rate/plate_no 时，主表标准契约字段必须保留', () => {
  const fkRich: Record<string, string> = {
    ...FK,
    金额: 'amount', 不含税金额: 'amount', 税率: 'taxRate', 税额: 'tax', 价税合计: 'totalAmount',
    车牌号: 'plate_no', 车辆类型: 'vehicle_type', 通行日期起: 'travel_from', 通行日期止: 'travel_to',
  }
  const fkRichFn = (n: string) => fkRich[n] || n
  const subs = [
    { name: 'invoiceNo' }, { name: 'invoiceDate' }, { name: 'invoiceType' },
    { name: 'amount', label: '金额' },
    { name: 'taxRate', label: '税率' },
    { name: 'tax', label: '税额' },
    { name: 'totalAmount', label: '价税合计' },
    { name: 'plate_no', label: '车牌号' },
    // 通行费分类的 items 明细子列（与 invoiceFieldKeyOf 的真实映射对齐）
    { name: 'items', data_type: 'items', sub_fields: [
      { name: '项目名称' }, { name: '车牌号' }, { name: '车辆类型' },
      { name: '通行日期起' }, { name: '通行日期止' },
      { name: '金额' }, { name: '税率/征收率' }, { name: '税额' },
    ] },
  ]
  const defs = buildDetailFieldDefs(subs, { fieldKeyOf: fkRichFn, labelOf: lbl, isRemarkPart: isRp })

  // 标准契约字段一个都不能少（金额回归重点）
  for (const k of ['invoiceNo', 'invoiceDate', 'invoiceType', 'amount', 'taxRate', 'tax', 'totalAmount', 'plate_no']) {
    assert.ok(defs.some((d) => d.name === k), `标准契约字段 ${k} 必须保留在主表单`)
  }
  assert.equal(defs.find((d) => d.name === 'amount')?.label, '金额', '金额标签应为「金额」')

  // 明细容器仍不占位；纯明细专属键仍被排除
  assert.equal(defs.some((d) => d.name === 'items' || d.dataType === 'items'), false, 'items 不得占位')
  for (const k of ['spec', 'unit', 'qty', 'price', 'name', 'vehicle_type', 'travel_from', 'travel_to']) {
    assert.equal(defs.some((d) => d.name === k), false, `明细专属键 ${k} 不应出现在主表单`)
  }
})

test('isTaxKey：税额同义键判定', () => {
  assert.equal(isTaxKey('tax'), true)
  assert.equal(isTaxKey('taxAmount'), true)
  assert.equal(isTaxKey('tax_amount'), true)
  assert.equal(isTaxKey('amount'), false)
})

test('fillDetailForm：税额同义键（tax_amount）不进动态桶，只保留 tax', () => {
  const cats = [{ name: '普通发票', subs: [{ name: 'tax' }, { name: 'tax_amount' }] }]
  const form = fillDetailForm(ctx({ row: { invoiceType: '普通发票', tax: 5, tax_amount: 6 }, cats }))
  assert.equal(form.data.tax, '5')
  assert.equal('tax_amount' in form.data, false, '税额同义键不重复进动态桶')
})

test('fillDetailForm / itemToPayload：税率百分比↔小数双向归一（9 与 0.09 等价）', () => {
  // 明细税率 0.09（小数）→ 显示 "9"，保存回 0.09
  const form = fillDetailForm(ctx({ row: { items: [{ name: 'x', tax_rate: 0.09 }] } }))
  assert.equal(form.items[0].tax_rate, '9')
  assert.equal(itemToPayload(form.items[0]).tax_rate, 0.09)
  // 已是百分比点数 9 → 不再 ×100，保存回 0.09
  const form2 = fillDetailForm(ctx({ row: { items: [{ name: 'x', tax_rate: 9 }] } }))
  assert.equal(form2.items[0].tax_rate, '9')
  assert.equal(itemToPayload(form2.items[0]).tax_rate, 0.09)
  // 主表单税率同上
  const form3 = fillDetailForm(ctx({ row: { taxRate: 0.09 } }))
  assert.equal(form3.data.taxRate, '9')
  assert.equal(buildDetailPayload({ data: form3.data, invoiceType: '', setRemarkPart: (r) => r }).tax_rate, 0.09)
})
