/**
 * 「打印目录」的表格排版计算（纯函数，不依赖 pdf-lib）。
 *
 * 为什么单独抽出来：打印目录的列数是**用户字段定义**决定的，20+ 列是常态
 * （费用清单 22 列、市电账单 20 列）。旧实现固定 8.5pt 字号 + 6px 内边距，
 * 每列保底 40pt：20 列 × 40 = 800 > A4 横向内容区 777.89 ——
 * 表格右边界直接越过纸张，「用气量」被裁掉一半、「燃气费」整列消失；
 * 同时保底 40pt 扣掉左右内边距只剩 28pt，`596168` 这种 6 位数放不下，
 * 被拆成 `59616` + `8` 两行。
 *
 * 这里的口径（产品要求）：
 *   ① 所有列宽之和**严格等于**纸张内容区宽度，任何列数都不越界；
 *   ② 列数越多，字号与左右内边距阶梯缩小（12 列以上进入 6~7.5pt）；
 *   ③ 数值单元格单行完整显示（不允许在数字中间断行），表头允许换行但不丢字。
 *
 * 因为要「按真实字体度量算宽度」，这里不写死字符宽度，而是由调用方传入
 * `measure(text, size)`（真实实现是 `font.widthOfTextAtSize`），
 * 纯函数只负责策略 —— 这样也能在单测里用假度量验证边界行为。
 */

export interface CatalogMetrics {
  /** 表头字号 */
  headerSize: number
  /** 单元格字号 */
  cellSize: number
  /** 单元格左右内边距（单侧） */
  padX: number
  /** 行高（多行文本按行数累加） */
  lineHeight: number
}

/** 字号下限：再小就不可读了，此时宁可让表头多换几行 */
export const MIN_FONT_SIZE = 6
export const MAX_FONT_SIZE = 8.5
export const MAX_PAD_X = 6
export const MIN_PAD_X = 2

/**
 * 按列数阶梯选择字号与内边距。
 *
 * 分档依据是「A4 横向 777.89pt 内容区 / 平均列宽」：8 列以内每列还有 90pt+，
 * 可以用常规 8.5pt；到 12 列只剩 65pt，必须缩到 8pt/5px；16 列 48pt、20 列 39pt，
 * 继续缩到 7pt/3px 与 6.5pt/2.5px，才能让 6 位数字（约 22pt）单行放下。
 */
export function metricsForColumnCount(n: number): CatalogMetrics {
  if (n <= 8) return { headerSize: 8.5, cellSize: 8.5, padX: 6, lineHeight: 12.5 }
  if (n <= 11) return { headerSize: 8, cellSize: 8, padX: 5, lineHeight: 11.8 }
  if (n <= 14) return { headerSize: 7.5, cellSize: 7.5, padX: 4, lineHeight: 11.2 }
  if (n <= 17) return { headerSize: 7, cellSize: 7, padX: 3, lineHeight: 10.6 }
  if (n <= 21) return { headerSize: 6.5, cellSize: 6.5, padX: 2.5, lineHeight: 10 }
  return { headerSize: 6, cellSize: 6, padX: 2, lineHeight: 9.4 }
}

/** 数字/金额类 token：这类字符之间**绝不允许**换行（否则 596168 会被拆开）。 */
const NUMERIC_TOKEN = /^[0-9.,%+\-/:×*]+$/
/** 把文本切成「不可断 token」序列：连续数字串算一个 token，其余逐字。 */
export function tokenizeForWrap(text: string): string[] {
  const out: string[] = []
  let buf = ''
  for (const ch of String(text ?? '')) {
    if (NUMERIC_TOKEN.test(ch)) {
      buf += ch
      continue
    }
    if (buf) { out.push(buf); buf = '' }
    out.push(ch)
  }
  if (buf) out.push(buf)
  return out
}

/**
 * 按宽度折行，**数字串不拆**。
 *
 * 与 `wrapParagraphs` 的差别只有一处：后者逐字判断，`596168` 会被切成
 * `59616` / `8`；这里以 token 为单位，数字串整体换行。
 * 单个 token 本身就超宽时（列宽被压到极窄）仍然单独成行 —— 宁可轻微溢出，
 * 也不能把金额拆成两行让用户读错。
 */
export function wrapNoBreakNumbers(
  measure: (s: string) => number,
  text: string,
  maxWidth: number,
): string[] {
  if (!text) return ['']
  const lines: string[] = []
  for (const para of String(text).replace(/\r\n?/g, '\n').split('\n')) {
    if (para === '') { lines.push(''); continue }
    let cur = ''
    let curW = 0
    for (const tok of tokenizeForWrap(para)) {
      const w = measure(tok)
      if (cur && curW + w > maxWidth) {
        lines.push(cur)
        cur = tok
        curW = w
      } else {
        cur += tok
        curW += w
      }
    }
    if (cur) lines.push(cur)
  }
  while (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  return lines.length ? lines : ['']
}

export interface FitInput {
  /** 纸张内容区宽度（pageWidth - margin * 2） */
  avail: number
  /** 每列表头文本 */
  headers: string[]
  /** 每列每行内容文本（已转成字符串，不含换行外的格式） */
  cells: string[][]
  /** 真实字体度量 */
  measure: (text: string, size: number) => number
  /** 起始排版参数（默认按列数阶梯选） */
  metrics?: CatalogMetrics
}

export interface FitResult {
  widths: number[]
  metrics: CatalogMetrics
  /** 各列允许的文本宽度（widths[i] - 2*padX） */
  textWidths: number[]
}

/**
 * 计算列宽：`sum(widths) === avail` 恒成立。
 *
 * 步骤：
 *   ① 先按列数取阶梯参数；
 *   ② 求每列「不可再窄」的下限 minW：内容最长不可断 token + 左右内边距
 *      （表头可以换行，所以下限只取「一个汉字宽 + 内边距」）；
 *   ③ 若 Σ minW 仍超过 avail，就继续缩字号/内边距（直到 MIN_FONT_SIZE），
 *      极端情况下按比例压缩下限（此时才允许轻微换行，但绝不允许越界）；
 *   ④ 剩余空间按「期望宽度 − 下限」的比例分配，最后一列吸收取整误差。
 */
export function fitColumnWidths(input: FitInput): FitResult {
  const { avail, headers, cells, measure } = input
  const n = headers.length
  if (n === 0) return { widths: [], metrics: metricsForColumnCount(0), textWidths: [] }

  let metrics = input.metrics || metricsForColumnCount(n)

  const minWidthOf = (i: number, m: CatalogMetrics) => {
    let longestToken = 0
    for (const row of cells) {
      for (const tok of tokenizeForWrap(row?.[i] ?? '')) {
        const w = measure(tok, m.cellSize)
        if (w > longestToken) longestToken = w
      }
    }
    // 表头只需容纳一个汉字（可换行），但要留出内边距
    const oneChar = measure(String(headers[i] ?? ' ').slice(0, 1) || ' ', m.headerSize)
    return Math.max(longestToken, oneChar) + m.padX * 2
  }

  const desiredOf = (i: number, m: CatalogMetrics) => {
    let maxCell = 0
    for (const row of cells) {
      for (const line of String(row?.[i] ?? '').split('\n')) {
        const w = measure(line, m.cellSize)
        if (w > maxCell) maxCell = w
      }
    }
    const head = measure(String(headers[i] ?? ''), m.headerSize)
    return Math.max(maxCell, head) + m.padX * 2
  }

  let mins: number[] = []
  let desired: number[] = []
  // ③ 逐步缩排：字号每档 0.5pt、内边距同步收敛，直到下限之和塞得进纸张
  for (let guard = 0; guard < 24; guard++) {
    mins = Array.from({ length: n }, (_, i) => minWidthOf(i, metrics))
    const sumMin = mins.reduce((a, b) => a + b, 0)
    if (sumMin <= avail) break
    if (metrics.cellSize > MIN_FONT_SIZE) {
      const next = Math.max(MIN_FONT_SIZE, metrics.cellSize - 0.5)
      const scale = next / metrics.cellSize
      metrics = {
        headerSize: Math.max(MIN_FONT_SIZE, Math.round(metrics.headerSize * scale * 10) / 10),
        cellSize: next,
        padX: Math.max(MIN_PAD_X, Math.round(metrics.padX - 0.5)),
        lineHeight: Math.max(8, Math.round(metrics.lineHeight * 0.96 * 10) / 10),
      }
      continue
    }
    // 字号已到下限仍塞不进（30+ 列）：按比例压缩下限，保证不越界
    const scale = avail / sumMin
    mins = mins.map((w) => w * scale)
    break
  }

  desired = Array.from({ length: n }, (_, i) => desiredOf(i, metrics))
  const sumMin = mins.reduce((a, b) => a + b, 0)
  const widths = mins.slice()

  const extra = avail - sumMin
  if (extra > 0) {
    const weights = desired.map((d, i) => Math.max(0, d - mins[i]))
    const sumW = weights.reduce((a, b) => a + b, 0)
    if (sumW > 0) {
      for (let i = 0; i < n; i++) widths[i] += (extra * weights[i]) / sumW
    } else {
      for (let i = 0; i < n; i++) widths[i] += extra / n
    }
  }

  // 取整误差全部压到最后一列：表格右边界必须与纸张内容区右边界重合
  const rounded = widths.map((w) => Math.floor(w * 100) / 100)
  const sum = rounded.reduce((a, b) => a + b, 0)
  rounded[n - 1] = Math.round((rounded[n - 1] + (avail - sum)) * 100) / 100
  // 兜底：浮点误差导致末列被削成负数时，退化为等分
  if (rounded[n - 1] <= 0) {
    const even = Math.round((avail / n) * 100) / 100
    for (let i = 0; i < n; i++) rounded[i] = even
    rounded[n - 1] = Math.round((avail - even * (n - 1)) * 100) / 100
  }

  return {
    widths: rounded,
    metrics,
    textWidths: rounded.map((w) => Math.max(1, w - metrics.padX * 2)),
  }
}
