// 通知公告字段「英文契约键 → 中文显示名」映射。
// 说明：subs.name（apNo 等）是提取/存储的 API 契约键，禁止改名；
// 展示层（字段配置、提取规则、详情抽屉）一律经 noticeFieldLabel 映射为中文。
export const NOTICE_FIELD_LABELS: Record<string, string> = {
  apNo: '文号',
  apTitle: '标题',
  apType: '文件类型',
  signDate: '发布日期',
  signer: '发布人',
  issueDept: '发布单位',
  summary: '摘要',
  person: '当事人',
  dept: '部门',
  position: '岗位',
  measure: '处理决定',
  basis: '依据',
  effectiveDate: '生效日期',
  remark: '备注',
  extractStatus: '提取状态',
  tags: '标签',
  fileName: '文件名',
}

export function noticeFieldLabel(name: string): string {
  return NOTICE_FIELD_LABELS[name] || name
}
