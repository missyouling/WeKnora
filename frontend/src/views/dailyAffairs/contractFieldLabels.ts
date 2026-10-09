// 合同字段「英文契约键 → 中文显示名」映射。
// 说明：subs.name（contractNo 等）是提取/存储的 API 契约键，禁止改名；
// 展示层（字段配置、提取规则、详情抽屉）一律经 contractFieldLabel 映射为中文。
export const CONTRACT_FIELD_LABELS: Record<string, string> = {
  contractNo: '合同编号',
  contractName: '合同名称',
  contractType: '合同类型',
  partyAName: '甲方',
  partyBName: '乙方',
  signDate: '签订日期',
  expiryDate: '到期日期',
  contractAmount: '合同金额',
  taxRate: '税率',
  extractStatus: '提取状态',
  tags: '标签',
  fulfillStatus: '履约状态',
  partyATaxNo: '甲方税号',
  partyBTaxNo: '乙方税号',
  effectiveDate: '生效日期',
  paymentMethod: '付款方式',
  handler: '经办人',
  department: '部门',
  fileName: '文件名',
}

export function contractFieldLabel(name: string): string {
  return CONTRACT_FIELD_LABELS[name] || name
}
