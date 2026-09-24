// 证照类型排序：与 FleetCertPanel 设置抽屉一致
// 1) 先按 BUILTIN 内置顺序（builtin_key 匹配）
// 2) 再追加 categories 里未 matched 的自定义分类（后端顺序）
// 3) 若 localStorage 有拖拽顺序则优先用拖拽顺序
export function sortCertsByLocalOrder<T extends { name: string; id?: string; builtin_key?: string }>(
  scope: string,
  list: T[],
  builtinOrder: string[] = [],
): T[] {
  // 1) localStorage 拖拽顺序
  try {
    const saved = JSON.parse(localStorage.getItem(`weknora-fleet-cert-order-${scope}`) || '[]')
    if (Array.isArray(saved) && saved.length) {
      const byName = new Map(list.map((m) => [m.name, m]))
      const byId = new Map(list.map((m) => [String(m.id), m]))
      const ordered: T[] = []
      const pushed = new Set<string>()
      for (const key of saved) {
        let item = byName.get(key)
        if (!item && typeof key === 'string' && key.startsWith('builtin-')) {
          item = byName.get(key.slice('builtin-'.length))
        }
        if (!item && byId.has(key)) item = byId.get(String(key))
        if (item && !pushed.has(item.name)) {
          ordered.push(item)
          pushed.add(item.name)
        }
      }
      list.forEach((m) => { if (!pushed.has(m.name)) ordered.push(m) })
      return ordered
    }
  } catch { /* ignore */ }

  // 2) 无拖拽顺序：按内置顺序（builtin_key 匹配）排，自定义追加末尾
  const byBuiltin = new Map<string, T>()
  list.forEach((c) => { if (c.builtin_key) byBuiltin.set(c.builtin_key, c) })
  const ordered: T[] = []
  const pushed = new Set<string>()
  builtinOrder.forEach((bKey) => {
    const c = byBuiltin.get(bKey)
    if (c && !pushed.has(c.name)) { ordered.push(c); pushed.add(c.name) }
  })
  list.forEach((c) => { if (!pushed.has(c.name)) ordered.push(c) })
  return ordered
}
