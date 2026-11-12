// 列宽工具：字段配置可显式指定列宽（width>0 固定 px；0/未配置 = 按内容自适应），
// 系统对固定宽度做 min/max 兜底，避免用户误输入导致列表显示异常。

export const MIN_COL_WIDTH = 60
// 上限 600：与设置面板的列宽输入框（60~600）保持一致。
// 此前是 400，用户在界面上填 500 会被静默截成 400 —— 「界面能填、保存无效」。
export const MAX_COL_WIDTH = 600
export const AUTO_COL_CAP = 150 // 内容自适应封顶：防长文本列（购买方等中文长文本）撑爆表格
/**
 * 长号码列（18 位身份证 / 19 位工资卡号 / 20 位发票号）的自适应上限。
 *
 * 第 53 轮：AUTO_COL_CAP=150 会把 18 位身份证（18×8+24=168px）也截到 150，
 * 用户看到的是 `5101111969122542...` ——「字段设置里填 250 也没用」的另一半原因。
 * 命中连续 12 位以上 ASCII 号码串时放宽到 240，保证完整显示且不出现省略号。
 */
export const AUTO_CODE_COL_CAP = 240
// 必须**整段值就是一个纯 ASCII 号码串**：身份证 / 银行卡 / 发票号。
// 不能写 /[A-Za-z0-9]{12,}/ —— 长文本里夹一个流水号也会被放宽到 240，
// 备注列会莫名变宽（textareaField 回归用例）。
const LONG_CODE_RE = /^[A-Za-z0-9]{12,}$/

// 内容自适应估算：max(表头宽度, 当前页数据最长内容)，中文 15px/字、ASCII 8px/字，+24 padding。
// 普通文本封顶 150；含长号码串时封顶 240（见 AUTO_CODE_COL_CAP）。
export function estimateColWidth(label: string, values: Array<string | number | null | undefined>): number {
  let maxW = (String(label || '').length + 2) * 15
  let hasLongCode = false
  for (const v of values) {
    if (v == null || v === '') continue
    const s = String(v)
    if (!hasLongCode && LONG_CODE_RE.test(s)) hasLongCode = true
    let len = 0
    for (const ch of s) len += ch.charCodeAt(0) > 255 ? 15 : 8
    const w = len + 24
    if (w > maxW) maxW = w
  }
  return Math.min(Math.ceil(maxW), hasLongCode ? AUTO_CODE_COL_CAP : AUTO_COL_CAP)
}

// 多行文本列宽度：长内容需要折行空间。内容自适应估算基于「最长单行文本」，
// 对多行文本会给出荒谬宽度（要么 150 封顶导致列过窄、要么按整段长度算出巨宽），
// 因此多行文本列走固定中等宽度；用户仍可在字段定义里显式配 width（优先）。
export const TEXTAREA_COL_WIDTH = 260

// 是否多行文本列（data_type === 'textarea'）。
// 多行文本列：关闭 ellipsis 截断、内容换行完整展示（详情/列表/打印一致）。
export function isTextareaColumn(c: { dataType?: string; key?: string } | null | undefined): boolean {
  return String(c?.dataType || '') === 'textarea'
}

// 统一列宽：width>0 → clamp[min,max] 固定；width<=0（含未配置）→ 内容自适应。
// dataType==='textarea' 时走固定中等宽度：内容自适应对多行文本无意义（见上）。
export function colWidthOf(
  width: number | undefined | null,
  label: string,
  values: Array<string | number | null | undefined>,
  dataType?: string,
): number {
  const w = Number(width) || 0
  if (w > 0) return Math.min(Math.max(w, MIN_COL_WIDTH), MAX_COL_WIDTH)
  if (dataType === 'textarea') return TEXTAREA_COL_WIDTH
  return estimateColWidth(label, values)
}

// 列是否关闭 ellipsis 截断：多行文本列必须关（TDesign 的 ellipsis 会用
// TEllipsis 包一层 + overflow hidden，换行样式直接被吃掉，内容仍被截断）。
export function ellipsisOf(c: { dataType?: string } | null | undefined): boolean {
  return !isTextareaColumn(c)
}

// 多行文本单元格的类名：走共享样式 `.biz-cell--multiline`（pre-wrap 折行、
// 不设 max-height，保证列表与详情/打印同样完整显示）。
export function cellClassOf(c: { dataType?: string } | null | undefined): string | undefined {
  return isTextareaColumn(c) ? 'biz-cell--multiline' : undefined
}

// 表格虚拟滚动配置。
//
// `isFixedRowHeight: true` 时 TDesign 假定每行恒为 rowHeight（48px），滚动位置按
// 「行数 × 48」线性推算，绝不会去测量真实行高（useVirtualScroll 的 handleRowMounted
// 在固定行高下直接 return）。多行文本列一旦折行，行高就不再是 48——固定行高会让
// 后续行错位堆叠/留白，长文本被裁在虚拟窗口之外。
//
// 因此：**列表里出现多行文本列时，必须关掉固定行高**，让 TDesign 逐行测量
// （tr 的 onMounted/onUpdated → row-mounted → handleRowMounted）。
// 没有多行文本列时保持 true：固定行高更稳、滚动更顺，且省掉每行一次几何测量。
export function tableScrollOf(
  cols: ReadonlyArray<{ dataType?: string }>,
  opts?: { rowHeight?: number; bufferSize?: number },
): { type: 'virtual'; rowHeight: number; isFixedRowHeight: boolean; bufferSize: number } {
  return {
    type: 'virtual',
    rowHeight: opts?.rowHeight ?? 48,
    isFixedRowHeight: !cols.some((c) => isTextareaColumn(c)),
    bufferSize: opts?.bufferSize ?? 20,
  }
}
