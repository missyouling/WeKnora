// 证照类型排序：以后端 categories 数组顺序为唯一权威
// （后端 ListFleetCategories 已按 sort_order ASC 排序，用户拖动后通过
// sortFleetCategories 持久化 sort_order）。
//
// 历史上这里曾优先读 localStorage 拖拽顺序、无 localStorage 时按内置
// BUILTIN 顺序重排，导致设置抽屉（直接渲染 categories）与工具栏下拉、
// 提取规则下拉、上传弹窗下拉的类型顺序不一致。现已废弃这两套本地排序，
// 统一信任后端返回顺序。
//
// 内置未入库分类（categories 里缺失，理论上不应出现）按内置顺序追加末尾兜底。
export function sortCertsByLocalOrder<T extends { name: string; id?: string; builtin_key?: string }>(
  _scope: string,
  list: T[],
  builtinOrder: string[] = [],
): T[] {
  const ordered: T[] = []
  const pushed = new Set<string>()
  // 1) categories 后端顺序（权威）
  list.forEach((c) => {
    if (c && c.name && !pushed.has(c.name)) {
      ordered.push(c)
      pushed.add(c.name)
    }
  })
  // 2) 内置未入库兜底：按内置顺序追加
  const byBuiltin = new Map<string, T>()
  list.forEach((c) => { if (c.builtin_key) byBuiltin.set(c.builtin_key, c) })
  builtinOrder.forEach((bKey) => {
    const c = byBuiltin.get(bKey)
    if (c && !pushed.has(c.name)) {
      ordered.push(c)
      pushed.add(c.name)
    }
  })
  return ordered
}
