// 列宽工具：字段配置可显式指定列宽（width>0 固定 px；0/未配置 = 按内容自适应），
// 系统对固定宽度做 min/max 兜底，避免用户误输入导致列表显示异常。

export const MIN_COL_WIDTH = 60
export const MAX_COL_WIDTH = 400
export const AUTO_COL_CAP = 150 // 内容自适应封顶：防长文本列（发票号码/购买方等）撑爆表格

// 内容自适应估算：max(表头宽度, 当前页数据最长内容)，中文 15px/字、ASCII 8px/字，+24 padding，封顶 150
export function estimateColWidth(label: string, values: Array<string | number | null | undefined>): number {
  let maxW = (String(label || '').length + 2) * 15
  for (const v of values) {
    if (v == null || v === '') continue
    const s = String(v)
    let len = 0
    for (const ch of s) len += ch.charCodeAt(0) > 255 ? 15 : 8
    const w = len + 24
    if (w > maxW) maxW = w
  }
  return Math.min(Math.ceil(maxW), AUTO_COL_CAP)
}

// 统一列宽：width>0 → clamp[min,max] 固定；width<=0（含未配置）→ 内容自适应
export function colWidthOf(
  width: number | undefined | null,
  label: string,
  values: Array<string | number | null | undefined>,
): number {
  const w = Number(width) || 0
  if (w > 0) return Math.min(Math.max(w, MIN_COL_WIDTH), MAX_COL_WIDTH)
  return estimateColWidth(label, values)
}
