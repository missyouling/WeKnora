// document 类业务 scope 注册：合同 / 制度 / 通知公告 / 奖惩（蓝图 P1-1）。
//
// 四个业务共用「一份文件一条记录」的 document 形态，差异只在分类轴名称与字段
// 字典。特别注意 notice 与 award_punish 的关系：奖惩模块展示的字段就是通知
// 公告字典（历史事实，库中数据如此），但两个 scope 在此**显式各自注册**——
// 消除 extractFieldMeta.ts 里「award_punish 借 NOTICE_FIELD_LABELS、LABELERS
// 没有 notice」的隐式耦合（蓝图 §1.1.5 坏味道：标签混用）。
import { registerScope, type FieldMeta } from './businessScope'
import { CONTRACT_FIELD_LABELS } from '../views/dailyAffairs/contractFieldLabels'
import { REGULATION_FIELD_LABELS } from '../views/dailyAffairs/regulationFieldLabels'
import { NOTICE_FIELD_LABELS } from '../views/dailyAffairs/noticeFieldLabels'

function fieldsOf(dict: Record<string, string>, extra: string[] = []): FieldMeta[] {
  const keys = [...new Set([...Object.keys(dict), ...extra])]
  return keys.map((k) => {
    const lower = k.toLowerCase()
    const dataType: FieldMeta['dataType'] =
      lower.endsWith('_date') || lower.endsWith('date') ? 'date' : 'text'
    return { key: k, label: dict[k] || k, dataType }
  })
}

function registerDocument(
  key: 'contract' | 'regulation' | 'notice' | 'award_punish',
  categoryLabel: string,
  categoryNoun: string,
  uploadNoun: string,
  uploadTypeLabel: string,
  dict: Record<string, string>,
  extraKeys: string[],
): void {
  registerScope({
    key,
    kind: 'document',
    categoryLabel,
    categoryNoun,
    namePlaceholder: `请输入${categoryNoun}名称`,
    emptyText: `暂无${categoryLabel}`,
    uploadNoun,
    uploadTypeLabel,
    uploadTypeTip: `选择${uploadTypeLabel}，上传后按所选类型归类提取`,
    itemLabel: '条目明细',
    supportsSubFields: false,
    supportsRemarkParts: false,
    // 与后端 ScopeSchema.RegexPatchable=false 对齐：document 类无标准字段
    // 补洞面，required 级补洞走通用线（genericRegexPatch），不在此设限。
    regexPatchable: false,
    listRows: 'documents',
    standardFields: fieldsOf(dict, extraKeys),
    api: { extract: 'business' },
  })
}

registerDocument(
  'contract', '合同类型', '合同', '合同', '合同类型',
  CONTRACT_FIELD_LABELS,
  ['sign_date', 'effective_date', 'expiry_date'],
)
registerDocument(
  'regulation', '制度名称', '制度', '制度', '制度类型',
  REGULATION_FIELD_LABELS,
  ['reg_no', 'issue_date'],
)
registerDocument(
  'notice', '通知标题', '通知', '通知', '通知类型',
  NOTICE_FIELD_LABELS,
  ['notice_no', 'issue_date'],
)
// 奖惩：字段字典与 notice 同源（历史数据如此），extra 补奖惩专属键。
// 上传弹窗沿用「公告」口径（奖惩模块对外即通知公告）。
registerDocument(
  'award_punish', '通知名称', '通知', '公告', '公告类型',
  NOTICE_FIELD_LABELS,
  ['ap_no', 'issue_date', 'sign_date'],
)
