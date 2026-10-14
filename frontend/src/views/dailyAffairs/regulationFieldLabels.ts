// 制度字段「英文契约键 → 中文显示名」映射。
// 说明：subs.name（regNo 等）是提取/存储的 API 契约键，禁止改名；
// 展示层（字段配置、提取规则、详情抽屉）一律经 regulationFieldLabel 映射为中文。
export const REGULATION_FIELD_LABELS: Record<string, string> = {
  regNo: '制度编号',
  regName: '制度名称',
  regType: '制度类型',
  version: '版本',
  pageCount: '页数',
  issueDate: '编制日期',
  effectiveDate: '生效日期',
  scope: '适用范围',
  dept: '编制部门',
  modifyCount: '修改次数',
  compiledBy: '编制',
  reviewedBy: '审核',
  approvedBy: '批准',
  summary: '摘要',
  confidentiality: '密级',
  remark: '备注',
  extractStatus: '提取状态',
  tags: '标签',
  fileName: '文件名',
}

export function regulationFieldLabel(name: string): string {
  return REGULATION_FIELD_LABELS[name] || name
}
