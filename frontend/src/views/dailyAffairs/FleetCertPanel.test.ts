import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  fromSubsPayload,
  toSubsPayload,
  normalizeSubFields,
  subsPayloadFromNames,
  subFieldKeyForLabel,
  findDuplicateSubField,
  copyFieldUniqueName,
  type EditableField,
  type EditableSubField,
} from './fleetCategorySubs'
import '../../registry'
import { getScope, isBuiltinFieldOf } from '../../registry'

// ---------------------------------------------------------------------------
// 回归锁定：FleetCertPanel 曾经在三处把 subs 的sub_fields 整体丢掉
//（toggleEdit 读、commitEdit 写、duplicateItem 复制），导致用户在「字段定义」
// Tab 点一次保存，明细子列就被抹成空（要等下次进程重启 seed 才补回）。
// 这组测试锁死「读—改—写」整条链路必须无损搬运 sub_fields。
// ---------------------------------------------------------------------------

test('fromSubsPayload 完整搬运 sub_fields（不被丢弃）', () => {
  const raw = [
    { name: 'items', enabled: true, is_default: true, data_type: 'items', width: 0,
      sub_fields: [
        { key: 'plate_no', label: '车牌号', data_type: 'text', width: 120 },
        { key: 'amount', label: '金额', data_type: 'number', width: 100 },
      ] },
  ]
  const out = fromSubsPayload(raw)
  assert.equal(out.length, 1)
  const items = out[0]!
  assert.equal(items.name, 'items')
  assert.equal(items.dataType, 'items')
  assert.equal(items.subFields?.length, 2)
  assert.deepEqual(items.subFields![0], { key: 'plate_no', label: '车牌号', dataType: 'text', width: 120 })
})

test('toSubsPayload 把 sub_fields 原样写回后端 snake_case 契约', () => {
  const fields: EditableField[] = [
    { name: 'items', enabled: true, isDefault: true, dataType: 'items', width: 0,
      subFields: [{ key: 'plate_no', label: '车牌号', dataType: 'text', width: 120 }] },
  ]
  const out = toSubsPayload(fields)
  assert.equal(out.length, 1)
  assert.equal(out[0].data_type, 'items')
  assert.ok(Array.isArray((out[0] as any).sub_fields))
  assert.deepEqual((out[0] as any).sub_fields, [
    { key: 'plate_no', label: '车牌号', data_type: 'text', width: 120 },
  ])
})

test('往返一致性：fromSubsPayload → toSubsPayload 不丢失任何字段与子列', () => {
  const raw = [
    { name: 'invoiceNo', enabled: true, is_default: true, data_type: 'text', width: 160 },
    { name: 'items', enabled: true, is_default: false, data_type: 'items', width: 0,
      sub_fields: [
        { key: 'name', label: '项目名称', data_type: 'text', width: 140 },
        { key: 'qty', label: '数量', data_type: 'number', width: 80 },
      ] },
  ]
  const round = toSubsPayload(fromSubsPayload(raw))
  assert.deepEqual(round, raw, '往返必须与原始 subs 完全一致（这是 P0 修复的核心断言）')
})

test('非 items 字段不携带 sub_fields（保持与后端 seed 形态一致）', () => {
  const out = toSubsPayload([
    { name: 'invoiceNo', enabled: true, isDefault: true, dataType: 'text', width: 0 },
  ])
  assert.equal('sub_fields' in (out[0] as any), false)
})

test('sub_fields 缺失/脏数据被清洗为合法子列，不抛异常', () => {
  const out = normalizeSubFields([
    { key: '', label: '空键' },
    { key: 'ok_key', label: '合法', data_type: 'text', width: 0 },
    { key: 'ok_key', label: '重复键' },
    null as any,
  ])
  assert.equal(out.length, 2)
  assert.deepEqual(out.map((s) => s.key), ['ok_key', 'ok_key'])
})

test('subsPayloadFromNames（内置底稿兜底）产出可被 fromSubsPayload 消费的结构', () => {
  const subs = subsPayloadFromNames(['发票号码', '开票日期'])
  const fields = fromSubsPayload(subs)
  assert.deepEqual(fields.map((f) => f.name), ['发票号码', '开票日期'])
  assert.deepEqual(fields.map((f) => f.dataType), ['text', 'text'])
})

// ---- 源码级锁定：三处读写必须走统一纯函数，杜绝第三处漂移 ----
const panel = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

test('FleetCertPanel 三处 subs 读写均已收敛到 toSubsPayload/fromSubsPayload', () => {
  // 旧实现的内联映射（会丢 sub_fields）必须消失
  assert.doesNotMatch(panel, /is_default:\s*!!f\.isDefault[^\n]*data_type:\s*f\.dataType/)
  assert.doesNotMatch(panel, /is_default:\s*s\.is_default\s*===\s*true[^\n]*data_type:\s*s\.data_type/)
  assert.match(panel, /import\s*\{[^}]*toSubsPayload[^}]*\}\s*from\s*'\.\/fleetCategorySubs'/)
  assert.match(panel, /fromSubsPayload/)
  // toggleEdit（读）、commitEdit（写）、duplicateItem（复制）三处都调用了纯函数
  const usages = (panel.match(/toSubsPayload\(/g) || []).length
  const reads = (panel.match(/fromSubsPayload\(/g) || []).length
  assert.ok(usages >= 2, `写/复制路径应至少 2 处调用 toSubsPayload，实际 ${usages}`)
  assert.ok(reads >= 2, `读路径应至少 2 处调用 fromSubsPayload，实际 ${reads}`)
})

test('明细子项内联展开在 items 字段行正下方，不再有底部独立区块', () => {
  // 旧结构：字段表底部的 .sub-fields-block（要滚到最下面才能配子项）
  assert.doesNotMatch(panel, /sub-fields-block/)
  assert.doesNotMatch(panel, /sub-fields-head/)
  // 新结构：点字段名在该行正下方展开，子行复用主行网格
  assert.match(panel, /class="fc-name fc-name-toggle"/)
  assert.match(panel, /@click="toggleSubFields\(fd\)"/)
  assert.match(panel, /class="field-config-row sub-row"/)
  assert.match(panel, /@enter="addSubField\(fd\)"/)
  assert.match(panel, /removeSubField\(fd, sf\)/)
})

test('子项行隐藏 key 输入：只留名称/类型/启用/列宽', () => {
  const subRow = panel.slice(panel.indexOf('class="field-config-row sub-row"'),
    panel.indexOf('class="field-config-add sub-inline-add"'))
  assert.doesNotMatch(subRow, /v-model="sf\.key"/, '子项 key 不允许手改')
  assert.match(subRow, /v-model="sf\.label"/)
  assert.match(subRow, /v-model="sf\.dataType"/)
  assert.match(subRow, /v-model="sf\.width"/)
  assert.match(subRow, /subFieldEnabled\(sf\)/)
})

test('新建子项由系统生成合法 key（中文名映射 / 递增 sub_N）', () => {
  const fields: EditableField[] = [
    { name: 'items', enabled: true, isDefault: true, dataType: 'items', width: 0, subFields: [] },
  ]
  // 中文名命中文档化别名表
  assert.equal(subFieldKeyForLabel('规格型号', []), 'spec')
  assert.equal(subFieldKeyForLabel('车牌号', []), 'plate_no')
  // 未命中且非英文 → 递增
  assert.equal(subFieldKeyForLabel('批次号', []), 'sub_1')
  assert.equal(subFieldKeyForLabel('批次号', ['sub_1', 'sub_2']), 'sub_3')
  // 已是合法英文键直接用
  assert.equal(subFieldKeyForLabel('account_no', []), 'account_no')
  // 别名被占用时退化为递增，绝不重名
  assert.equal(subFieldKeyForLabel('规格型号', ['spec']), 'sub_1')
  void fields
})

test('系统内置字段只有 3 个（发票类型/提取状态/标签），其余一律可删', () => {
  // 内置键声明在 BusinessScopeRegistry（registry/invoice.ts 的 builtinFieldKeys），
  // 面板按 scope 查询不再有 scope 硬编码分支。
  assert.deepEqual(getScope('invoice')?.builtinFieldKeys, ['invoiceType', 'extractStatus', 'tags'])
  assert.equal(getScope('vehicle')?.builtinFieldKeys, undefined)
  assert.equal(isBuiltinFieldOf('invoice', 'invoiceType'), true)
  assert.equal(isBuiltinFieldOf('invoice', 'invoiceNo'), false)
  assert.equal(isBuiltinFieldOf('vehicle', 'invoiceType'), false)
  assert.match(panel, /<template v-if="isBuiltinField\(fd\)">[\s\S]{0,200}lock-on/)
  assert.match(panel, /系统内置字段，不可删除/)
  // 2026-10-18 终轮：明细子项全部放开（含标准子项）——锁定逻辑已整体移除
  assert.doesNotMatch(panel, /系统内置子项，不可删除/)
  assert.match(panel, /class="field-config-row sub-row"/)
  // 不再用「标准键字典」兜底把 invoiceNo/amount 也锁死
  assert.doesNotMatch(panel, /INVOICE_FIELD_LABELS, fd\.name/)
})

test('明细子项删除走冒泡二次确认；items 行无箭头、无默认表头开关', () => {
  // 冒泡确认
  const subBlock = panel.slice(panel.indexOf('class="field-config-row sub-row"'))
  assert.match(subBlock.slice(0, 2200), /<t-popconfirm[\s\S]{0,300}removeSubField\(fd, sf\)/)
  assert.match(subBlock.slice(0, 2200), /确认删除子项/)
  // 无展开箭头（点击字段名本体展开/收起）
  assert.doesNotMatch(panel, /fc-caret/, 'items 行不再渲染箭头图标')
  assert.match(panel, /class="fc-name fc-name-toggle"/)
  // items 行的「默认表头」列改成占位，保持网格对齐
  assert.match(panel, /<span v-if="fd\.dataType === 'items'" class="sub-blank"><\/span>/)
})

test('子项 contract 扩展字段（enabled/builtin/desc/rule/regex）严格往返', () => {
  const raw = [
    { name: 'items', enabled: true, is_default: true, data_type: 'items', width: 0,
      sub_fields: [
        { key: 'spec', label: '规格型号', data_type: 'text', width: 120, builtin: true,
          rule: '取型号列', regex: '规格型号\\s*[:：]\\s*([A-Za-z0-9]+)' },
        { key: 'batch_no', label: '批次号', data_type: 'text', width: 0, enabled: false, desc: '生产批次' },
      ] },
  ]
  const fields = fromSubsPayload(raw)
  assert.equal(fields[0]!.subFields![0]!.builtin, true)
  assert.equal(fields[0]!.subFields![0]!.rule, '取型号列')
  assert.equal(fields[0]!.subFields![1]!.enabled, false)
  assert.equal(fields[0]!.subFields![1]!.desc, '生产批次')
  assert.deepEqual(toSubsPayload(fields), raw, '扩展字段必须零丢失往返')
})

test('默认值不下发：enabled=true / builtin=false 不污染存量结构', () => {
  const out = toSubsPayload([
    { name: 'items', enabled: true, isDefault: false, dataType: 'items', width: 0,
      subFields: [{ key: 'name', label: '项目名称', dataType: 'text', width: 0, enabled: true }] },
  ])
  assert.deepEqual((out[0] as any).sub_fields, [
    { key: 'name', label: '项目名称', data_type: 'text', width: 0 },
  ])
})
test('label（自定义中文表头）严格往返：name 身份不变，空 label 不下发', () => {
  const raw = [
    { name: 'invoice_date', label: '开票日', enabled: true, is_default: true, data_type: 'date', width: 0 },
    { name: 'invoice_no', enabled: true, is_default: false, data_type: 'text', width: 0 },
  ]
  const fields = fromSubsPayload(raw)
  assert.equal(fields[0]!.name, 'invoice_date')
  assert.equal(fields[0]!.label, '开票日')
  assert.equal(fields[1]!.label, undefined, '没有自定义表头时不产生 label')

  const back = toSubsPayload(fields)
  assert.equal(back[0]!.label, '开票日')
  assert.equal('label' in back[1]!, false, '空 label 必须省略，保持旧数据结构')
  assert.deepEqual(toSubsPayload(fromSubsPayload(back)), back, '往返必须严格互逆')
})

test('label 与 name 相同时不重复下发（语义等价去重）', () => {
  const out = toSubsPayload([
    { name: 'items', label: 'items', enabled: true, isDefault: false, dataType: 'items', width: 0 },
  ])
  assert.equal('label' in out[0]!, false)
})

// ---- 2026-10-18 终轮：明细子项去重 + 删除修复 ----

test('子项去重：同义键（税额 tax/tax_amount、单价 price/unit_price）视为同一项', () => {
  const subs: EditableSubField[] = [
    { key: 'tax_amount', label: '税额', dataType: 'number', width: 0 },
    { key: 'price', label: '单价', dataType: 'number', width: 0 },
  ]
  assert.ok(findDuplicateSubField(subs, { key: 'tax', label: '税额' }), 'tax 应判为 tax_amount 的同义项')
  assert.ok(findDuplicateSubField(subs, { key: 'unit_price', label: '单价(元)' }), 'unit_price 应判为 price 的同义项')
  assert.equal(findDuplicateSubField(subs, { key: 'plate_no', label: '车牌号' }), null, '不同语义不应误判')
})

test('子项去重：同列头也算重复（浮层里两列同名无法区分）', () => {
  const subs: EditableSubField[] = [{ key: 'batch_no', label: '备注', dataType: 'text', width: 0 }]
  const dup = findDuplicateSubField(subs, { key: 'memo', label: ' 备注 ' })
  assert.ok(dup)
  assert.equal(dup!.key, 'batch_no')
})

test('子项删除不再调用已删除的 isBuiltinSubField，并按 key 兜底移除', () => {
  const fn = panel.slice(panel.indexOf('function removeSubField'), panel.indexOf('function removeSubField') + 700)
  assert.doesNotMatch(fn, /isBuiltinSubField/, '上一轮删掉的函数不能再被调用（否则运行时报错→删不掉）')
  assert.match(fn, /s !== sf/, '按引用移除')
  assert.match(fn, /String\(s\?\.key \|\| ''\) !== key/, '按 key 兜底移除')
})

test('新增子项先查同义/同列头再落库', () => {
  const fn = panel.slice(panel.indexOf('function addSubField'), panel.indexOf('function addSubField') + 800)
  assert.match(fn, /findDuplicateSubField\(subs, \{ key, label \}\)/)
  assert.match(fn, /已存在/)
})

// ---- 2026-10-21：复制字段中文命名 + 可编辑 + 自动聚焦 ----

test('复制字段命名是「{中文名}_副本」递增，不再产出 tags_copy 英文键', () => {
  assert.equal(copyFieldUniqueName('标签', []), '标签_副本')
  assert.equal(copyFieldUniqueName('标签', ['标签_副本']), '标签_副本2')
  assert.equal(copyFieldUniqueName('标签', ['标签_副本', '标签_副本2']), '标签_副本3')
  // 空基名兜底为「字段」，绝不产出「_副本」这种裸前缀
  assert.equal(copyFieldUniqueName('', []), '字段_副本')
  // taken 同时收 name 与 label：表头撞名也算占用
  assert.equal(copyFieldUniqueName('备注', ['备注_副本']), '备注_副本2')
  // 复制「副本本身」时先剥尾部 _副本N，序号递增而不是层层嵌套
  //（否则真机上出现 甲方税号_副本_副本）
  assert.equal(copyFieldUniqueName('标签_副本', ['标签_副本']), '标签_副本2')
  assert.equal(copyFieldUniqueName('标签_副本2', ['标签_副本', '标签_副本2']), '标签_副本3')
})

test('duplicateField 用 copyFieldUniqueName 且不再拼 _copy 英文后缀', () => {
  const fn = panel.slice(panel.indexOf('function duplicateField'), panel.indexOf('function addField'))
  assert.match(fn, /copyFieldUniqueName\(/, '复制字段命名必须走中文副本命名')
  assert.doesNotMatch(fn, /_copy/, '禁止再生成 tags_copy 这类英文身份键')
  assert.match(fn, /已复制字段/)
})

test('字段名可编辑（t-input 绑定显示名），不再有只读 static 展示', () => {
  // 所有非 items 字段名都是输入框；显示值走 fieldEditOf（label 优先，契约键回退字典中文名）
  assert.match(panel, /:model-value="fieldEditOf\(fd\)"/, '输入框显示值必须走 fieldEditOf')
  assert.match(panel, /@update:model-value="[^"]*setFieldEdit/, '改名走 setFieldEdit（契约键只改表头）')
  assert.doesNotMatch(panel, /fc-name-static/, '字段名只读 static 展示已废除')
})

test('复制后自动聚焦新字段名输入框', () => {
  const fn = panel.slice(panel.indexOf('function duplicateField'), panel.indexOf('function addField'))
  assert.match(fn, /copyFieldUniqueName\(/)
  assert.match(fn, /nextTick/, '复制后需等待渲染再聚焦')
  assert.match(fn, /focus\(\)/, '复制后必须 focus 新字段名输入框')
  // 输入框注册 ref，供 duplicateField 聚焦
  assert.match(panel, /setFieldInputRef\(/)
})

// 大类汇总字段（mode=fee_group）的 members 必须无损往返：
// 它是「哪些细项属于这一列」的唯一依据，丢了就退化成「按大类名自动归并」，
// 用户在字段配置里关掉的细项会被重新算进汇总列（金额变大）。
test('费用清单大类汇总字段（fee_group + members）严格往返', () => {
  const raw = [
    {
      name: 'grp_system', label: '系统运行费用', enabled: true, is_default: true,
      data_type: 'number', width: 0, group: 'tenant:t1', group_label: '持睿汽车',
      source: {
        category: '市电账单', field: 'grp_system', mode: 'fee_group',
        group: '(4)系统运行费', group_label: '(4)系统运行费', label: '系统运行费用',
        members: ['item_(4)系统运行费|煤电容量电费', 'item_(4)系统运行费|燃气机组容量电费'],
      },
    },
  ]
  const round = toSubsPayload(fromSubsPayload(raw))
  assert.deepEqual(round, raw, 'fee_group 字段往返必须逐字段一致（含 members）')
  // 反向：members 不能在解析时被丢掉
  const parsed = fromSubsPayload(raw)[0]
  assert.equal(parsed.source?.mode, 'fee_group')
  assert.deepEqual(parsed.source?.members, raw[0].source.members)
})
