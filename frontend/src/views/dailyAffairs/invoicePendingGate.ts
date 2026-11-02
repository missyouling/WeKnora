// 列表行准入的第二道门禁（第 24 轮任务一）。
//
// 真机事故（图 1）：状态栏还显示「正在解析 2026.9-10油费发票.pdf」，
// 列表第 17~20 行已经出现该文件的 P1~P4（第 20 行还是「待提取 + 0.00」），
// KPI 卡片也提前变了。
//
// 为什么后端已过滤还要前端再挡一次：轮询周期内后端返回的是**上一次快照**，
// 而 pendingFiles 已经先一步反映「这个文件正在解析」。两个数据源之间存在
// 一个轮询周期（3s）的窗口，窗口内列表会短暂展示半成品 —— 用户看到的就是
// 图 1 那一屏。
//
// 判据：pendingFiles 里的文件 ID（= knowledgeId）对应的**所有分页子记录**
// 一律剔除。同一文件的行必须整组进出 —— 不允许 P1/P2 先进、P3/P4 后进。

/**
 * 按「进行中文件 ID 集合」过滤列表行。
 *
 * 零成本短路：集合为空时**返回原数组引用**（不拷贝），避免每次 computed
 * 重算都生成新数组导致下游无谓的 diff。
 *
 * 空 ID 保护：`knowledgeId` 为空的行（占位行/历史数据）**永不剔除** ——
 * 若实现写成 `ids.has(String(r.knowledgeId || ''))`，而 pending 集合里
 * 恰好混入空串，就会把整表清空（这是最容易踩的坑，单测专门锁了）。
 */
export function filterRowsByPendingFiles<T extends { knowledgeId?: string }>(
  rows: T[],
  pendingIds: Set<string>,
): T[] {
  if (!pendingIds.size) return rows
  return rows.filter((r) => {
    const id = String(r?.knowledgeId || '')
    if (!id) return true
    return !pendingIds.has(id)
  })
}
