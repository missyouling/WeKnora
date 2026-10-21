// 通知公告 / 奖惩 字段「契约键 → 中文显示名」映射。
//
// 2026-10-21 统一 scope：notice 与 award_punish **共用同一份字段目录**，与后端
// internal/application/service/business_field_catalog.go 的 noticeCatalog()
// （BusinessFieldCatalogOf("notice") 归一到 award_punish）严格同源：标准 9 + 扩展 10。
// 此前拆成两份零交集字典（公告一套 / 奖惩一套）的版本作废：统一后 AI 字段建议、
// 提取规则、抽屉表单三处都按真实样文（通报类同样要当事人/处理决定/依据）喂同一目录。
//
// 键用 snake契约键（camelCase 形态经 registry 的 canonicalFieldKey 归一也能查）。
// 历史键（老数据/老配置可能仍是 apNo/apTitle 或 dept/position）保留读兼容。
export const NOTICE_FIELD_LABELS: Record<string, string> = {
  // ---- 标准 9 ----
  ap_no: '公告编号',
  ap_title: '公告标题',
  ap_type: '公告类型',
  issue_dept: '发文部门',
  sign_date: '发布日期',
  effective_date: '生效日期',
  scope: '受文对象',
  extract_status: '提取状态',
  tags: '标签',
  // ---- 扩展 10 ----
  signer: '签发人',
  contact: '联系人',
  contact_phone: '联系电话',
  urgency: '紧急程度',
  valid_until: '有效期限',
  attachments: '附件名称',
  person: '当事人',
  measure: '处理决定',
  basis: '依据',
  summary: '摘要',
  // ---- 历史键（读兼容标签，不进目录主轴）----
  apNo: '公告编号',
  apTitle: '公告标题',
  apType: '公告类型',
  issueDept: '发文部门',
  signDate: '发布日期',
  effectiveDate: '生效日期',
  contactPhone: '联系电话',
  validUntil: '有效期限',
  dept: '部门',
  position: '岗位',
  remark: '备注',
}

// 奖惩与公告统一目录（同一份引用：改一处即两处同步，杜绝再漂移）。
export const AWARD_PUNISH_FIELD_LABELS: Record<string, string> = NOTICE_FIELD_LABELS

export function noticeFieldLabel(name: string): string {
  return NOTICE_FIELD_LABELS[name] || name
}

export function awardPunishFieldLabel(name: string): string {
  return AWARD_PUNISH_FIELD_LABELS[name] || name
}
