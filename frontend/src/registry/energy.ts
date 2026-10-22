// 能耗（energy）scope 注册：账期级台账。
//
// 与既有 utility/solar 的分工（重要，不要混淆）：
//   - utility / solar：账单**明细级**口径（一条账单 30~50 行费用明细 + 峰平谷分时），
//     走后端专用提取管线，字段字典为空，登记在 registry/utility.ts；
//   - energy：账期**台账级**口径（一条记录 = 一个账期的一张汇总单），
//     核心是「上期读数 / 本期读数 / 倍率 / 实际用量」这条数值连续性主线，
//     接入中台字段字典 + 六同步 + 中台组件。
//
// 键名与后端 internal/application/service/business_field_catalog_energy.go 的
// energyCatalog() 逐项对齐（snake_case），两处漂移会导致正则补洞静默失效。
import { registerScope, type FieldMeta } from './businessScope'

// 能耗标准字段：顺序即界面顺序，与后端 energyCatalog().Standard 逐项一致。
const ENERGY_STANDARD_FIELDS: FieldMeta[] = [
  { key: 'utility_no', label: '单据编号/户号', dataType: 'text' },
  { key: 'billing_period', label: '账期', dataType: 'text' },
  { key: 'utility_type', label: '能耗类型', dataType: 'text' },
  { key: 'previous_reading', label: '上期读数', dataType: 'number' },
  { key: 'current_reading', label: '本期读数', dataType: 'number' },
  { key: 'multiplier', label: '倍率', dataType: 'number' },
  { key: 'usage_amount', label: '实际用量', dataType: 'number' },
  { key: 'unit_price', label: '单价', dataType: 'number' },
  { key: 'total_amount', label: '总金额', dataType: 'number' },
  { key: 'extract_status', label: '提取状态', dataType: 'text' },
  { key: 'tags', label: '标签', dataType: 'array' },
]

// 能耗扩展字段：台账补充信息（户名 / 表号 / 地址 / 账期起止 / 抄表日期 …）。
const ENERGY_EXTENDED_FIELDS: FieldMeta[] = [
  { key: 'account_name', label: '户名', dataType: 'text' },
  { key: 'meter_no', label: '表号', dataType: 'text' },
  { key: 'address', label: '用能地址', dataType: 'text' },
  { key: 'period_start', label: '账期起', dataType: 'date' },
  { key: 'period_end', label: '账期止', dataType: 'date' },
  { key: 'read_date', label: '抄表日期', dataType: 'date' },
  { key: 'usage_category', label: '用能类别', dataType: 'text' },
  { key: 'voltage_level', label: '电压等级', dataType: 'text' },
  { key: 'remark', label: '备注', dataType: 'textarea' },
]

registerScope({
  key: 'energy',
  kind: 'document',
  categoryLabel: '能耗类型',
  categoryNoun: '能耗单',
  namePlaceholder: '请输入能耗类型名称',
  emptyText: '暂无能耗类型',
  uploadNoun: '能耗单',
  uploadTypeLabel: '能耗类型',
  uploadTypeTip: '选择能耗类型（电费/水费/燃气费/供热费），上传后按所选类型归类提取',
  itemLabel: '表位明细',
  // 能耗支持明细子表格：一份汇总单含多个表位读数时并入 items（一文件一档案）
  supportsSubFields: true,
  supportsRemarkParts: false,
  // 与后端 ScopeSchema.RegexPatchable=true 对齐：能耗有标准字段集，
  // 允许为读数/账期等字段生成并应用 RE2 兜底正则。
  regexPatchable: true,
  listRows: 'documents',
  standardFields: [...ENERGY_STANDARD_FIELDS, ...ENERGY_EXTENDED_FIELDS],
  // 系统内置不可删：分类主轴 + 提取状态 + 标签
  builtinFieldKeys: ['utilityType', 'extractStatus', 'tags'],
  aliases: {
    户号: 'utility_no',
    单据编号: 'utility_no',
    账期: 'billing_period',
    能耗类型: 'utility_type',
    上期读数: 'previous_reading',
    本期读数: 'current_reading',
    倍率: 'multiplier',
    实际用量: 'usage_amount',
    用量: 'usage_amount',
    单价: 'unit_price',
    总金额: 'total_amount',
  },
  // 规则面板内置正则预设：能耗高频字段的兜底锚点
  regexPresets: [
    {
      name: '账期(YYYY-MM)',
      value: '(?:账\\s*期|账单周期|抄表周期|费用所属月份)\\s*[:：]?\\s*(20\\d{2}\\s*[-年/.]\\s*(?:0?[1-9]|1[0-2])\\s*月?)',
      hint: '从「账期 2026-09」「2026年09月」等写法提取账期',
    },
    {
      name: '上期读数',
      value: '(?:上期(?:示数|读数|累计|抄见)|期初(?:示数|读数|累计))\\s*[:：]?\\s*([0-9][0-9,，]*(?:\\.[0-9]{1,4})?)',
      hint: '提取上期累计示数',
    },
    {
      name: '本期读数',
      value: '(?:本期(?:示数|读数|累计|抄见)|期末(?:示数|读数|累计))\\s*[:：]?\\s*([0-9][0-9,，]*(?:\\.[0-9]{1,4})?)',
      hint: '提取本期累计示数',
    },
    {
      name: '倍率',
      value: '(?:倍\\s*率|变\\s*比|互感器倍率)\\s*[:：]?\\s*([0-9][0-9,，]*(?:\\.[0-9]{1,4})?)',
      hint: '提取计量倍率（CT 变比）',
    },
    {
      name: '实际用量',
      value: '(?:实际用量|本期用量|用\\s*量(?:合计|总计)?|抄见电量|计费电量)\\s*[:：]?\\s*([0-9][0-9,，]*(?:\\.[0-9]{1,4})?)',
      hint: '提取本期用量（可与读数差值交叉校验）',
    },
  ],
  api: { extract: 'business' },
})
