/**
 * 「设置 · 能耗」抽屉的顶层平级标签（第 74 轮）。
 *
 * 设计约束（数据红线）：
 *   重构只发生在视图/交互层，**不动 fleet_categories 的 ID / Key / 归属关系**，
 *   因此这里的标签 → 分类名映射只是一张前端路由表：
 *     · 能耗账单  → 市电账单 / 光伏账单（同一标签内的二级切换）
 *     · 提取规则  → 市电账单 / 光伏账单（AI 提取）
 *     · 电费核算 / 水费核算 / 气费核算 / 费用清单 → 各自分类的字段配置
 *     · 工业表计  → 电/水/气表的表计档案（非分类字段）
 */
export type EnergySettingsTab =
  | 'bill' | 'rules' | 'electric' | 'water' | 'gas' | 'billing' | 'meter'

export interface EnergySettingsTabDef {
  value: EnergySettingsTab
  label: string
}

/**
 * 顶层 7 个平级标签（顺序即视觉顺序，严格 1:1 对应真机断言）。
 * 旧的「字段定义」二级外壳与「表计设置」文案已删除。
 */
export const ENERGY_SETTINGS_TABS: EnergySettingsTabDef[] = [
  { value: 'bill', label: '能耗账单' },
  { value: 'rules', label: '提取规则' },
  { value: 'electric', label: '电费核算' },
  { value: 'water', label: '水费核算' },
  { value: 'gas', label: '气费核算' },
  { value: 'billing', label: '费用清单' },
  { value: 'meter', label: '工业表计' },
]

/** 分类名 → 顶层标签（页面「设置」按钮定位用）。 */
const CATEGORY_TAB: Record<string, EnergySettingsTab> = {
  市电账单: 'bill',
  光伏账单: 'bill',
  电费核算: 'electric',
  水费核算: 'water',
  气费核算: 'gas',
  费用清单: 'billing',
  工业表计: 'meter',
  表计设置: 'meter',
}

/** 能耗账单标签内允许出现（且只允许出现）的两个账单分类。 */
export const ENERGY_BILL_CATEGORIES = ['市电账单', '光伏账单'] as const
export type EnergyBillCategory = typeof ENERGY_BILL_CATEGORIES[number]

/** 7 个标签里承载「字段配置」的那些（其余标签渲染专属面板）。 */
export const ENERGY_FIELD_TAB_CATEGORY: Partial<Record<EnergySettingsTab, string>> = {
  electric: '电费核算',
  water: '水费核算',
  gas: '气费核算',
  billing: '费用清单',
}

/** 分类名落地到顶层标签；未知分类回退「能耗账单」（绝不留空白标签）。 */
export function energyTabOfCategory(category: string): EnergySettingsTab {
  return CATEGORY_TAB[String(category || '').trim()] || 'bill'
}

/** 分类名是否属于「能耗账单」标签里的二级切换项。 */
export function energyBillCategoryOf(category: string): EnergyBillCategory | '' {
  const name = String(category || '').trim()
  return (ENERGY_BILL_CATEGORIES as readonly string[]).includes(name)
    ? (name as EnergyBillCategory)
    : ''
}

/**
 * 该分类的字段是否平铺展示（不按 group 分组）。
 *
 * 光伏账单：需求明确删除「派生字段（不可提取）」与「账单概况」两个胶囊分组外壳，
 * 直接平铺展示全部字段。
 */
export function energyFlattensGroups(category: string): boolean {
  return String(category || '').trim() === '光伏账单'
}

/** 字段分组「默认折叠」只作用于分组多的账单分类（市电账单 8 个费用分组）。 */
export function energyCollapsesGroupsByDefault(category: string): boolean {
  const name = String(category || '').trim()
  return name === '市电账单'
}
