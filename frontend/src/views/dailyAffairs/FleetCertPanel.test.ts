import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  fromSubsPayload,
  toSubsPayload,
  normalizeSubFields,
  subsPayloadFromNames,
  subFieldKeyForLabel,
  type EditableField,
} from './fleetCategorySubs'

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

test('系统内置字段与内置子项禁止删除，仅自建扩展字段可删', () => {
  assert.match(panel, /function isBuiltinField/)
  assert.match(panel, /function isBuiltinSubField/)
  assert.match(panel, /系统内置字段，不可删除/)
  assert.match(panel, /系统内置子项，不可删除/)
  // 内置行渲染锁图标而不是删除入口
  assert.match(panel, /<template v-if="isBuiltinField\(fd\)">[\s\S]{0,200}lock-on/)
  assert.match(panel, /<template v-if="isBuiltinSubField\(sf\)">[\s\S]{0,200}lock-on/)
  // 存量分类没带 builtin 标记时，发票标准键仍按内置处理
  assert.match(panel, /INVOICE_FIELD_LABELS, fd\.name/)
  assert.match(panel, /STANDARD_SUB_KEYS\.has\(sf\.key\)/)
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
