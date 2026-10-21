// 合同字段「契约键 → 中文显示名」单一权威源。
//
// 2026-10-21 字段体系重置：合同字段目录重构为 **11 标准 + 15 扩展（严格顺序）**，
// 与后端 internal/application/service/business_field_catalog.go 的 contractCatalog()
// 逐项同源（标准 11 → 扩展 15）。旧键（partyAName / expiryDate / contractAmount /
// signPlace / fulfillStatus…）不再进入字段主轴，只做**存量数据/存量配置的读兼容**：
//   - 行映射：mapContractRecord / parseCustomMetadata 读新键，旧键回退；
//   - 归一：contractKeyOf() 把任意旧形态归一到新契约键（editFieldDefs 用）。
// 历史字段（税率/质保金/违约金…）仍在 ContractRecord JSON（legacy omitempty），
// 字典保留其标签，方便用户自建分类 subs 显示中文；但不属于新字段体系。

/** 新契约键严格顺序 = 后端 contractCatalog().AllFields()。 */
export const CONTRACT_FIELD_ORDER: readonly string[] = [
  // 标准 11
  'contract_no', 'contract_name', 'contract_type', 'party_a', 'party_b',
  'sign_date', 'expire_date', 'amount', 'extract_status', 'performance_status', 'tags',
  // 扩展 15
  'party_a_contact', 'party_a_tax_no', 'party_b_contact', 'party_b_tax_no',
  'party_a_address', 'party_a_phone', 'party_a_bank', 'party_a_account',
  'party_b_address', 'party_b_phone', 'party_b_bank', 'party_b_account',
  'payment_method', 'sign_location', 'summary',
]

/** 契约键 → 中文名。顺序即目录顺序（旧键/历史字段追加在后，不影响主轴顺序）。 */
export const CONTRACT_FIELD_LABELS: Record<string, string> = {
  // ---- 新契约体系（11 标准 + 15 扩展，严格顺序）----
  contract_no: '合同编号',
  contract_name: '合同名称',
  contract_type: '合同类型',
  party_a: '甲方',
  party_b: '乙方',
  sign_date: '签订日期',
  expire_date: '到期日期',
  amount: '合同金额',
  extract_status: '提取状态',
  performance_status: '履约状态',
  tags: '标签',
  party_a_contact: '甲方经办人',
  party_a_tax_no: '甲方税号',
  party_b_contact: '乙方经办人',
  party_b_tax_no: '乙方税号',
  party_a_address: '甲方地址',
  party_a_phone: '甲方电话',
  party_a_bank: '甲方开户行',
  party_a_account: '甲方账号',
  party_b_address: '乙方地址',
  party_b_phone: '乙方电话',
  party_b_bank: '乙方开户行',
  party_b_account: '乙方账号',
  payment_method: '付款方式',
  sign_location: '签订地点',
  summary: '摘要',
  // ---- 旧键（存量配置/数据的读兼容标签：camelCase 与 snake_case 两形态都收）----
  contractNo: '合同编号',
  contractName: '合同名称',
  partyAName: '甲方', party_a_name: '甲方',
  partyBName: '乙方', party_b_name: '乙方',
  expiryDate: '到期日期', expiry_date: '到期日期',
  contractAmount: '合同金额', contract_amount: '合同金额',
  signPlace: '签订地点', sign_place: '签订地点',
  fulfillStatus: '履约状态', fulfill_status: '履约状态',
  // ---- 历史字段（ContractRecord legacy omitempty：仍可读、可经用户 subs 显示）----
  effective_date: '生效日期',
  effectiveDate: '生效日期',
  tax_rate: '税率',
  taxRate: '税率',
  quality_bond: '质保金',
  qualityBond: '质保金',
  liquidated_damages: '违约金',
  liquidatedDamages: '违约金',
  subject: '合同标的',
  quantity: '数量',
  unit_price: '单价',
  unitPrice: '单价',
  performance_period: '履行期限',
  performancePeriod: '履行期限',
  handler: '经办人',
  department: '部门',
  remark: '备注',
}

/**
 * 旧键 → 新契约键（与后端 contractLegacyFieldAliases 同口径）。
 * 约束：值必须全部落在 CONTRACT_FIELD_ORDER 内（单测锁定，
 * 防别名指向不存在的新键造成"归一后查无此字段"）。
 */
export const CONTRACT_KEY_ALIASES: Record<string, string> = {
  partyAName: 'party_a', party_a_name: 'party_a',
  partyBName: 'party_b', party_b_name: 'party_b',
  expiryDate: 'expire_date', expiry_date: 'expire_date',
  contractAmount: 'amount', contract_amount: 'amount',
  signPlace: 'sign_location', sign_place: 'sign_location',
  fulfillStatus: 'performance_status', fulfill_status: 'performance_status',
}

/** camelCase / 旧 snake 键 → 契约键（snake_case）。 */
function snakeize(name: string): string {
  return String(name || '')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1_$2')
    .toLowerCase()
}

/**
 * 归一任意合同字段键到新契约键。
 * - 命中别名表 → 新契约键（partyAName → party_a）；
 * - 其余按 camelCase → snake_case（contractNo → contract_no，partyATaxNo → party_a_tax_no）；
 * - 已是新键或中文自定义键 → 原样透传。
 */
export function contractKeyOf(name: string): string {
  const raw = String(name || '').trim()
  if (!raw) return raw
  return CONTRACT_KEY_ALIASES[raw] || snakeize(raw)
}

/** 字段中文名（旧键也能翻，未收录原样返回）。 */
export function contractFieldLabel(name: string): string {
  const key = contractKeyOf(name)
  return CONTRACT_FIELD_LABELS[key] || CONTRACT_FIELD_LABELS[name] || name
}
