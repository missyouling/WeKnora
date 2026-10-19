// 车队档案类 scope 注册：车辆 / 驾驶员 / 维保（蓝图 P1-1）。
//
// 档案类是配置驱动（无提取规则面板的 scope 概念），注册它们是为了让
// FleetCertPanel 的分组标签/空态/占位文案统一走 registry，消灭五层三元。
import { registerScope } from './businessScope'

const fleetScopes: Array<{
  key: 'vehicle' | 'driver' | 'maintain'
  categoryLabel: string
  categoryNoun: string
  emptyText: string
  namePlaceholder: string
  uploadNoun: string
}> = [
  {
    key: 'vehicle',
    categoryLabel: '证照名称',
    categoryNoun: '证照类型',
    emptyText: '暂无证照类型',
    namePlaceholder: '请输入证照名称',
    uploadNoun: '证照',
  },
  {
    key: 'driver',
    categoryLabel: '证照名称',
    categoryNoun: '证照类型',
    emptyText: '暂无证照类型',
    namePlaceholder: '请输入证照名称',
    uploadNoun: '证照',
  },
  {
    key: 'maintain',
    categoryLabel: '维保项名称',
    categoryNoun: '维保类型',
    emptyText: '暂无维保类型',
    namePlaceholder: '请输入维保项名称',
    uploadNoun: '证照',
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
    uploadTypeLabel: s.categoryNoun,
    uploadTypeTip: `必须选择${s.categoryNoun}，上传后按所选类型归类提取`,
    itemLabel: '档案字段',
    supportsSubFields: false,
    supportsRemarkParts: false,
    regexPatchable: false,
    listRows: 'records',
    standardFields: [],
    api: { extract: 'fleet' },
  })
}
