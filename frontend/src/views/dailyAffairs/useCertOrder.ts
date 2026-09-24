// 证照类型排序：与 FleetCertPanel 设置抽屉拖拽顺序一致（localStorage 持久化）
// 排序键为分组 key（maintain 页即 scope='maintain'），以分类名作排序键。
export function sortCertsByLocalOrder<T extends { name: string; id?: string }>(
  scope: string,
  list: T[],
): T[] {
  try {
    const saved = JSON.parse(localStorage.getItem(`weknora-fleet-cert-order-${scope}`) || '[]')
    if (!Array.isArray(saved) || !saved.length) return list
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
  } catch {
    return list
  }
}
