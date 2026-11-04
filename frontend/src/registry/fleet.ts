// 车队档案类 scope 注册：车辆 / 驾驶员 / 维保（蓝图 P1-1）。
//
// 档案类是配置驱动（无提取规则面板的 scope 概念），注册它们是为了让
// FleetCertPanel 的分组标签/空态/占位文案统一走 registry，消灭五层三元。
import { registerScope } from './businessScope'

const fleetScopes: Array<{
  key: 'vehicle' | 'driver' | 'maintain' | 'tire' | 'inspection' | 'fleet-cost'
  categoryLabel: string
  categoryNoun: string
  emptyText: string
  namePlaceholder: string
  uploadNoun: string
  uploadTypeLabel?: string
  /** 是否接入字段级必填校验 + 后置正则补洞（仅三个上传提取类档案） */
  fieldRules?: boolean
}> = [
  {
    key: 'vehicle',
    categoryLabel: '证照名称',
    categoryNoun: '证照类型',
    emptyText: '暂无证照类型',
    namePlaceholder: '请输入证照名称',
    uploadNoun: '证照',
    // 车辆档案：字段来自分类 subs（中文键），提取结果就是 map[string]any，
    // 后端 ValidateGenericExtraction 用扁平 map 视图直接做必填校验与正则补洞。
    fieldRules: true,
  },
  {
    key: 'driver',
    categoryLabel: '证照名称',
    categoryNoun: '证照类型',
    emptyText: '暂无证照类型',
    namePlaceholder: '请输入证照名称',
    uploadNoun: '证照',
    fieldRules: true,
  },
  {
    key: 'maintain',
    categoryLabel: '维保项名称',
    categoryNoun: '维保类型',
    emptyText: '暂无维保类型',
    namePlaceholder: '请输入维保项名称',
    uploadNoun: '证照',
    fieldRules: true,
  },
  // 记录型菜单（第 28 轮）：轮胎/年检单分类，成本管理 7 分类。
  // 这些是台账字段而非证照，文案统一用「分类 / 字段」，上传名词用「票据」。
  {
    key: 'tire',
    categoryLabel: '分类名称',
    categoryNoun: '分类',
    emptyText: '暂无分类',
    namePlaceholder: '请输入分类名称',
    uploadNoun: '票据',
    uploadTypeLabel: '分类',
  },
  {
    key: 'inspection',
    categoryLabel: '分类名称',
    categoryNoun: '分类',
    emptyText: '暂无分类',
    namePlaceholder: '请输入分类名称',
    uploadNoun: '票据',
    uploadTypeLabel: '分类',
  },
  {
    key: 'fleet-cost',
    categoryLabel: '成本项名称',
    categoryNoun: '成本项',
    emptyText: '暂无成本项',
    namePlaceholder: '请输入成本项名称',
    uploadNoun: '票据',
    uploadTypeLabel: '成本项',
  },
]

for (const s of fleetScopes) {
  registerScope({
    key: s.key,
    kind: 'archive',
    categoryLabel: s.categoryLabel,
    categoryNoun: s.categoryNoun,
    namePlaceholder: s.namePlaceholder,
    emptyText: s.emptyText,
    uploadNoun: s.uploadNoun,
    uploadTypeLabel: s.uploadTypeLabel || s.categoryNoun,
    uploadTypeTip: `必须选择${s.categoryNoun}，上传后按所选类型归类提取`,
    itemLabel: '档案字段',
    supportsSubFields: false,
    supportsRemarkParts: false,
    // 记录型菜单（轮胎/年检/成本）是纯手工录入，没有上传提取链路 → 不开放正则
    regexPatchable: s.fieldRules === true,
    listRows: 'records',
    standardFields: [],
    api: { extract: 'fleet' },
  })
}
