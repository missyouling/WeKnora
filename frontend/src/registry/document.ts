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
import { NOTICE_FIELD_LABELS, AWARD_PUNISH_FIELD_LABELS } from '../views/dailyAffairs/noticeFieldLabels'

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
  builtinFieldKeys: string[],
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
    // 与后端 ScopeSchema.RegexPatchable=true 对齐（合同/制度/公告/奖惩已声明
    // 标准字段集）：允许为标准字段生成/应用 RE2 兜底正则。
    regexPatchable: true,
    listRows: 'documents',
    standardFields: fieldsOf(dict, extraKeys),
    // 系统内置三件套（不可删除）：分类主轴 + 提取状态 + 标签
    builtinFieldKeys,
    api: { extract: 'business' },
  })
}

registerDocument(
  'contract', '合同类型', '合同', '合同', '合同类型',
  CONTRACT_FIELD_LABELS,
  ['sign_date', 'effective_date', 'expiry_date'],
  ['contractType', 'extractStatus', 'tags'],
)
registerDocument(
  'regulation', '制度类型', '制度', '制度', '制度类型',
  REGULATION_FIELD_LABELS,
  ['reg_no', 'issue_date'],
  ['regType', 'extractStatus', 'tags'],
)
// 通知公告 与 奖惩：统一 scope、同一份字段目录（2026-10-21，与后端 noticeCatalog() 同源：
// BusinessFieldCatalogOf("notice") 归一到 award_punish 一份目录）。
// 内置三件套（分类主轴 apType + 提取状态 + 标签）也统一，不再让 measure 当奖惩主轴。
registerDocument(
  'notice', '公告类型', '公告', '公告', '公告类型',
  NOTICE_FIELD_LABELS,
  ['ap_no', 'sign_date'],
  ['apType', 'extractStatus', 'tags'],
)
registerDocument(
  'award_punish', '奖惩类型', '奖惩', '奖惩决定', '奖惩类型',
  AWARD_PUNISH_FIELD_LABELS,
  ['ap_no', 'sign_date'],
  ['apType', 'extractStatus', 'tags'],
)
