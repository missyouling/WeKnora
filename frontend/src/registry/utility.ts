// 账单类 scope 注册：电费 / 光伏（蓝图 P1-1）。
//
// 与 document 同形态（一份账单一条记录），单独文件是为了让「再加一种账单」
// 有明确的落点，不必翻 document.ts。
import { registerScope } from './businessScope'

for (const [key, categoryLabel] of [
  ['utility', '账单类型'],
  ['solar', '账单类型'],
] as const) {
  registerScope({
    key,
    kind: 'document',
    categoryLabel,
    categoryNoun: '账单类型',
    namePlaceholder: '请输入账单类型名称',
    emptyText: `暂无${categoryLabel}`,
    uploadNoun: '账单',
    uploadTypeLabel: categoryLabel,
    itemLabel: '账单明细',
    supportsSubFields: false,
    supportsRemarkParts: false,
    regexPatchable: false,
    listRows: 'documents',
    standardFields: [],
    api: { extract: 'business' },
  })
}
