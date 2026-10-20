// 通知公告 / 奖惩 字段「英文契约键 → 中文显示名」映射。
//
// 2026-10-21 拆分：此前 NOTICE_FIELD_LABELS 把「公告（文号/标题/发布单位…）」
// 与「奖惩（当事人/岗位/处理决定/依据…）」两套字段混在同一张表里，两个 scope
// 共用字典 → AI 字段建议会把「处理决定」喂给公告、把「文号」喂给奖惩。
// 现按 scope 各持一份、零交集，与后端 ScopeSchema 的标准字段集同源。
export const NOTICE_FIELD_LABELS: Record<string, string> = {
  apNo: '文号',
  apTitle: '标题',
  apType: '公告类型',
  signDate: '发布日期',
  signer: '发布人',
  issueDept: '发布单位',
  summary: '摘要',
  remark: '备注',
  extractStatus: '提取状态',
  tags: '标签',
  fileName: '文件名',
}

// 奖惩管理（award_punish）：与公告零交集。
export const AWARD_PUNISH_FIELD_LABELS: Record<string, string> = {
  person: '当事人',
  dept: '部门',
  position: '岗位',
  measure: '处理决定',
  basis: '依据',
  effectiveDate: '生效日期',
  apTitle: '决定名称',
  signDate: '签发日期',
  remark: '备注',
  extractStatus: '提取状态',
  tags: '标签',
  fileName: '文件名',
}

export function noticeFieldLabel(name: string): string {
  return NOTICE_FIELD_LABELS[name] || name
}

export function awardPunishFieldLabel(name: string): string {
  return AWARD_PUNISH_FIELD_LABELS[name] || name
}
