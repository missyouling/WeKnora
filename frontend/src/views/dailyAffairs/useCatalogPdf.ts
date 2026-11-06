// 目录 PDF 生成工具：把选中记录的字段清单渲染为可打印/可下载的 PDF。
// 表格式清单，固定 A4 横向；列宽按内容权重自适应（单列上限约页宽 30%），
// 超长内容自动换行完整显示（不做省略号截断），行高随行数自适应。
// 内嵌开源中文字体（霞鹜文楷，OFL）。
import { PDFFont, rgb } from 'pdf-lib'
import { fitColumnWidths, wrapNoBreakNumbers, type CatalogMetrics } from './catalogLayout'
import {
  createPdfSurface,
  drawWrappedCell,
  fitHeaderFontSize,
  loadSharedFont,
  type PdfSurface,
} from './pdfSurface'

export interface CatalogColumn {
  key: string
  label: string
  value: (row: any) => string
}

// 字体加载收敛到 pdfSurface（两套引擎共用同一份缓存与错误处理）。
// 对外保留 loadCatalogFont 名字：9 个页面的 import 不动。
export const loadCatalogFont = loadSharedFont

// 固定 A4 横向（方向由生成时定死，用户打印时可在系统打印对话框内用缩放适配）
const PAGE_W = 841.89
const PAGE_H = 595.28
const MARGIN = 32
const TITLE_SIZE = 15
const META_SIZE = 9
// 字号 / 左右内边距 / 列宽全部由 catalogLayout 按列数动态给：
// 20+ 列时必须缩到 6~7.5pt 且总宽严格等于内容区，否则最右列会被纸张裁掉。
const HEADER_HEIGHT = 26
const ROW_MIN_HEIGHT = 22

const COLOR_BORDER = rgb(0.82, 0.82, 0.82)
const COLOR_HEADER_BG = rgb(0.94, 0.94, 0.94)
const COLOR_ZEBRA_BG = rgb(0.98, 0.98, 0.98)
const COLOR_TEXT = rgb(0.16, 0.16, 0.16)
const COLOR_MUTED = rgb(0.45, 0.45, 0.45)
const COLOR_TITLE = rgb(0.1, 0.35, 0.2)

function pad2(n: number) { return n < 10 ? `0${n}` : String(n) }

// 按显式换行符切段（\r\n / \r / \n 统一），供列宽估算与折行共用同一口径。
export function splitExplicitLines(text: string): string[] {
  return String(text || '').replace(/\r\n?/g, '\n').split('\n')
}

// 按宽度换行，返回多行文本（完整显示，不截断）。`measure` 是单串宽度度量
// （真实场景为 font.widthOfTextAtSize），抽出纯函数以便行为级单测。
//
// 必须先按显式换行符切段：多行文本字段（remark/summary/说明…）的值里带 `\n`，
// pdf-lib 的 drawText **不解释** `\n`（会被当成缺失字形丢弃或画出豆腐块），
// 直接逐字折行会把「第一行第二行」黏成一行，用户录入的段落结构丢失。
// 制表符同理先展开成空格，避免宽度计算与实际渲染不一致。
export function wrapParagraphs(
  measure: (s: string) => number,
  text: string,
  maxWidth: number,
): string[] {
  if (!text) return ['']
  const lines: string[] = []
  for (const para of splitExplicitLines(text).map((p) => p.replace(/\t/g, '    '))) {
    if (para === '') {
      // 空行保留为真正的空行（段落间距），否则多段文本会被压成一整块
      lines.push('')
      continue
    }
    let cur = ''
    let curW = 0
    for (const ch of para) {
      const w = measure(ch)
      if (cur && curW + w > maxWidth) {
        lines.push(cur)
        cur = ch
        curW = w
      } else {
        cur += ch
        curW += w
      }
    }
    if (cur) lines.push(cur)
  }
  // 尾部空行没有渲染意义（会被行高算成额外留白），裁掉；全空时保留一个空行占位
  while (lines.length > 1 && lines[lines.length - 1] === '') lines.pop()
  return lines.length ? lines : ['']
}

// 字体度量版折行（调用方只需关心 fontSize）
function wrapText(font: PDFFont, text: string, maxWidth: number, size: number): string[] {
  return wrapParagraphs((s) => font.widthOfTextAtSize(s, size), text, maxWidth)
}

export async function generateCatalogPdf(opts: {
  title: string
  columns: CatalogColumn[]
  rows: any[]
}): Promise<Uint8Array> {
  const { title, columns, rows } = opts
  if (!columns.length || !rows.length) throw new Error('无可用数据生成目录')

  // 页面面板（字体 / 分页 / 页脚 / 游标）统一由 pdfSurface 提供。
  // 两套引擎此前各写一遍「加载字体 → 建文档 → 游标 → 翻页 → 页脚」，
  // 收敛后差异只剩「续页画什么」——目录重画表头行（横向表格跨页后不看表头无法读数）。
  const surface = await createPdfSurface({
    geom: { width: PAGE_W, height: PAGE_H, margin: MARGIN },
    startY: PAGE_H - MARGIN - TITLE_SIZE - 4,
    footerText: (n) => `${title} · 第 ${n} 页`,
    onNewPage: (s) => {
      s.y = PAGE_H - MARGIN - HEADER_HEIGHT
      drawHeaderRow()
    },
  })
  const { doc, font, avail } = surface
  // **不能解构 page**：它是快照，surface.newPage() 之后局部变量仍指向旧页，
  // 继续往上画会全部落在已完成的页面上（且没有任何报错）。用访问器每次取当前页。
  const curPage = () => surface.page
  // 列宽：交给 catalogLayout 统一收敛。
  //
  // 旧实现「每列保底 40pt + 按比例压缩」在 20 列时会算出 800pt > 777.89pt，
  // 表格右边界越过纸张（「燃气费」整列消失）；且 40pt 扣掉 12pt 内边距只剩 28pt，
  // 6 位数字放不下被拆行。现在：总宽严格 = avail，列多则自动缩字号/内边距，
  // 数字串绝不拆行。
  const cellTexts: string[][] = rows.map((row) => columns.map((c) => String(c.value(row) ?? '')))
  const fit = fitColumnWidths({
    avail,
    headers: columns.map((c) => c.label),
    cells: cellTexts,
    measure: (text, size) => font.widthOfTextAtSize(text, size),
  })
  const widths = fit.widths
  const metrics: CatalogMetrics = fit.metrics
  const { padX, lineHeight, cellSize, headerSize } = metrics
  const textWidths = fit.textWidths

  const now = new Date()
  const dateStr = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())} ${pad2(now.getHours())}:${pad2(now.getMinutes())}`

  // 预计算每行的换行行数与行高（数字串不拆）
  const rowLines = cellTexts.map((cells) =>
    cells.map((text, i) => wrapNoBreakNumbers((s) => font.widthOfTextAtSize(s, cellSize), text, textWidths[i]))
  )
  const rowHeights = rowLines.map((linesArr) => {
    const maxLines = Math.max(...linesArr.map((ls) => ls.length))
    return Math.max(ROW_MIN_HEIGHT, maxLines * lineHeight + padX * 2)
  })
  // 表头：允许优雅换行（中文按字断），但绝不丢字。
  // 字号已在 fitColumnWidths 里随列数收敛；这里只在个别超长表头上再压一档
  // （收敛逻辑抽到 pdfSurface.fitHeaderFontSize，与清单引擎共用）。
  const headerFonts = columns.map((c, i) =>
    fitHeaderFontSize(c.label, textWidths[i], headerSize, (t, sz) => font.widthOfTextAtSize(t, sz))
  )
  const headerMaxLines = Math.max(...headerFonts.map((h) => h.lines.length))

  // 标题基线；副标题必须落到标题下沿之外，否则 15pt 标题与 9pt 元信息会叠字
  curPage().drawText(title, { x: MARGIN, y: surface.y, size: TITLE_SIZE, font, color: COLOR_TITLE })
  surface.y -= TITLE_SIZE + 6
  // 元信息（共 N 条 / 生成时间）
  const meta = `共 ${rows.length} 条 · 生成时间 ${dateStr}`
  curPage().drawText(meta, { x: MARGIN, y: surface.y, size: META_SIZE, font, color: COLOR_MUTED })
  surface.y -= META_SIZE + 14

  const drawHeaderRow = () => {
    let x = MARGIN
    const h = Math.max(HEADER_HEIGHT, headerMaxLines * lineHeight + padX * 2)
    const top = surface.y
    curPage().drawRectangle({ x: MARGIN, y: top - h, width: avail, height: h, color: COLOR_HEADER_BG })
    curPage().drawRectangle({ x: MARGIN, y: top - h, width: avail, height: h, borderColor: COLOR_BORDER, borderWidth: 0.5 })
    columns.forEach((c, i) => {
      const w = widths[i]
      const { size, lines } = headerFonts[i]
      // 单元格绘制复用共享实现（垂直居中 + 多行基线，见 pdfSurface.drawWrappedCell）
      drawWrappedCell(curPage(), {
        lines, x, top, height: h, size, lineHeight, padX, font, color: COLOR_TEXT,
      })
      // 列分隔线：20+ 列时没有竖线几乎无法对齐读数
      if (i > 0) {
        curPage().drawLine({
          start: { x, y: top }, end: { x, y: top - h },
          color: COLOR_BORDER, thickness: 0.4,
        })
      }
      x += w
    })
    surface.y -= h
  }

  drawHeaderRow()

  rows.forEach((row, idx) => {
    const linesArr = rowLines[idx]
    const rh = rowHeights[idx]
    surface.ensure(rh)
    const rowTop = surface.y
    // 斑马纹
    if (idx % 2 === 1) {
      curPage().drawRectangle({ x: MARGIN, y: rowTop - rh, width: avail, height: rh, color: COLOR_ZEBRA_BG })
    }
    // 边框
    curPage().drawRectangle({ x: MARGIN, y: rowTop - rh, width: avail, height: rh, borderColor: COLOR_BORDER, borderWidth: 0.5 })
    let x = MARGIN
    columns.forEach((c, i) => {
      const w = widths[i]
      // 空值用浅灰：一眼能分辨「这一格没数据」与「有值但很短」
      drawWrappedCell(curPage(), {
        lines: linesArr[i], x, top: rowTop, height: rh, size: cellSize, lineHeight, padX, font,
        color: c.value(row) ? COLOR_TEXT : COLOR_MUTED,
      })
      if (i > 0) {
        curPage().drawLine({
          start: { x, y: rowTop }, end: { x, y: rowTop - rh },
          color: COLOR_BORDER, thickness: 0.4,
        })
      }
      x += w
    })
    surface.y -= rh
  })

  // 收尾页脚：ensure 只在翻页前画页脚，最后一页要在这里补
  surface.drawFooter()

  return doc.save()
}

// ---------------------------------------------------------------------------
// 打印清单（结算单）：与「打印目录」并列的另一种版式。
//
// 目录 = 列表表头 + 行的横向表格（一屏看全，适合对账）；
// 清单 = 参照市电账单详情页的「概览 + 细项」结构：账单概览 → 电费分项细项
//        → 电费核算 / 水费核算 / 气费核算（**水与电必须分表**）→ 应收合计。
//
// 为什么是「通用文档模型」而不是写死一段版式：
//   业主视角与租户视角（按分摊比例折算）共用同一套排版，只是数值不同；
//   水/电/气三个附加区块的列也不同。把「标题 + 概览 + 若干表格」抽象成
//   StatementDoc，排版引擎只认结构，业务侧只负责算数。
// ---------------------------------------------------------------------------

const PORTRAIT_W = 595.28
const PORTRAIT_H = 841.89
const S_MARGIN = 40
const S_TITLE_SIZE = 17
const S_SUBTITLE_SIZE = 9.5
const S_META_SIZE = 8.8
const S_SECTION_SIZE = 10.5
const S_TH_SIZE = 8.4
const S_TD_SIZE = 8.4
const S_LINE_H = 11
const S_CELL_PAD = 5
const S_ROW_MIN = 19
const S_HEADER_H = 22
// 第 41 轮：带 rowSpans 的「电费分项细项」用紧凑行高，让 24 行明细 +
// 合计行完整落在第 1 页；其余明细表保持原行高，版式不受影响。
const S_COMPACT_ROW_MIN = 18
const S_COMPACT_LINE_H = 10.5

const C_RULE = rgb(0.85, 0.87, 0.86)
const C_HEAD_BG = rgb(0.945, 0.955, 0.95)
const C_ZEBRA = rgb(0.975, 0.98, 0.977)
const C_TOTAL_BG = rgb(0.93, 0.95, 0.94)
const C_INK = rgb(0.13, 0.15, 0.14)
const C_MUTED = rgb(0.44, 0.48, 0.46)
const C_BRAND = rgb(0.03, 0.55, 0.30)
const C_KPI_BORDER = rgb(0.86, 0.9, 0.88)

export interface StatementMeta { label: string; value: string }
export interface StatementKpi { label: string; value: string; hint?: string }
export interface StatementColumn {
  label: string
  align?: 'left' | 'right' | 'center'
  /** 相对列宽权重（缺省 1） */
  weight?: number
}
export interface StatementTable {
  title: string
  columns: StatementColumn[]
  rows: string[][]
  /**
   * 单元格纵向合并（第 40 轮）：与 rows 等长，每格给 rowspan 数。
   *   > 1 → 从本行起向下合并 n 行（文字只在首格绘制、垂直居中）
   *   = 0 → 被上方合并区覆盖（不绘制文字，内部不画横线）
   *   = 1 或缺省 → 普通单元格
   *
   * 打印清单的「费用类别 / 费用组成」用它与市电账单原件排版对齐。
   */
  rowSpans?: number[][]
  /**
   * 画完本表后强制翻页（第 41 轮）。
   *
   * 「电费分项细项」必须整表落在第 1 页，且第 2 页顶部从「电费核算明细」
   * 章节标题开始。删掉 6 条小计行 + 紧凑化后表格本身就放得下，这里再加一道
   * 显式分页，避免后续表格蹭到第 1 页剩余空白、把续页标题挤到页尾。
   */
  pageBreakAfter?: boolean
  /** 合计行（整行加粗、灰底、顶部深色线） */
  total?: string[]
  /** 无数据时的占位文案 */
  empty?: string
  /**
   * 即使所有明细行都是 0 也保留（应收汇总 / 账单合计 / 结算汇总这类**结论表**）。
   *
   * 结论表的作用是「告诉租户这一期总共多少钱」，0 元也必须显示；
   * 而明细表（费用分项 / 电量明细 / 水电气核算）0 行毫无意义，必须裁掉。
   */
  alwaysShow?: boolean
}
export interface StatementDoc {
  /** 主标题，如「重庆星达 费用清单」 */
  title: string
  /** 副标题，如「2026-08 · 使用单位 重庆星达」 */
  subtitle?: string
  /** 概览区标题（缺省不显示）；有值即渲染成「一、账单概览」样式的分区标题 */
  overviewTitle?: string
  meta?: StatementMeta[]
  kpis?: StatementKpi[]
  tables: StatementTable[]
  footerNote?: string
}

/**
 * 第 41 轮：拆分「费用类别」合并单元格的双行文案。
 *
 * 业务侧写成 `大类\n（小计 XXX.XX）`，渲染时第 1 行用标准字号/主色、
 * 第 2 行用小 0.8pt 的次要色，并整体在 rowspan 合并区内垂直居中。
 * 没有 `\n` 时 `sub` 为空，走原来的单行绘制路径（其它表零影响）。
 */
export function splitCategoryLabel(raw: string): { main: string; sub: string } {
  const parts = String(raw ?? '').split('\n')
  return { main: parts[0] || '', sub: parts.slice(1).join(' ').trim() }
}

/**
 * 明细行「有实际数据」判定。
 *
 * 口径（产品要求）：金额为 0/空/`0.00` **且** 数量为 0/空/`—` 的行没有发生过业务，
 * 打印时一律隐藏 —— 一张 2026-09 账单里 `偏差电费`、`绿电交易电费（省间/省内）`、
 * `绿电环境价值电费（省外）`、`中长期超额申报回收费用` 全 0，却各占 4 行，
 * 把单月清单撑到 4 页，真正有数的分项反被挤到后页。
 *
 * 判定顺序：
 *   ① 显式 `__skip` 标记（业务侧已判定「无数据」）→ 隐藏；
 *   ② 找出「金额列 / 数量列」：按列标签（金额(元) / 电费 / 水费 / 气费 / 计费数量 / 用量…）；
 *   ③ 金额列与数量列**全部为 0 或占位** → 隐藏；任一非 0 → 保留。
 * 没有金额/数量列的表（如纯文本清单）一律保留，避免误删。
 */
const AMOUNT_COL = /金额|电费|水费|气费|结算|税额|价税|\(元\)|（元）/
const QTY_COL = /数量|用量|电量|水量|气量|kwh|吨|m³|立方/
/** 「空值」占位：破折号、全角破折号、空串、纯 0 的各种写法。 */
const isBlankOrZero = (v: unknown): boolean => {
  const s = String(v ?? '').trim()
  if (!s || s === '—' || s === '-' || s === '–' || s === '——') return true
  const n = Number(s.replace(/[,，¥￥\s]/g, ''))
  if (!Number.isFinite(n)) return false
  return n === 0
}

/**
 * 该行是否没有任何实际发生的数据（应隐藏）。
 *
 * 数值列的识别顺序（越靠前越可靠）：
 *   ① 右对齐列（本项目的所有金额/数量列都是 `align: 'right'`，而名称列都是左对齐）；
 *   ② 没有右对齐列时，才按列标签匹配「金额/电费/水费/计费数量/用量…」。
 *
 * **不能只按标签匹配**：`费用组成` 里含「费用」二字，会被当成金额列，
 * 于是「偏差电费 | 尖峰 | 0 | — | 0.00」这一行因为第 1 格是文字而判为「有数据」，
 * 空行裁剪整体失效（真机 2026-09 清单仍是 4 页的直接原因）。
 */
export function isEmptyDetailRow(columns: StatementColumn[], cells: string[]): boolean {
  if ((cells as any)?.__skip) return true
  let probes = columns.map((c, i) => (c.align === 'right' ? i : -1)).filter((i) => i >= 0)
  if (!probes.length) {
    probes = columns
      .map((c, i) => (AMOUNT_COL.test(String(c.label || '')) || QTY_COL.test(String(c.label || '')) ? i : -1))
      .filter((i) => i >= 0)
  }
  if (!probes.length) return false
  return probes.every((i) => isBlankOrZero(cells[i]))
}

/**
 * 过滤掉空明细行，并**同步隐藏过滤后为空**的整张表。
 *
 * 返回新的 tables 数组；`total` 行不需要重算（它本来就是各行金额的累加，
 * 被删的都是 0 行，合计不变）。
 */
export function pruneEmptyTables(tables: StatementTable[]): StatementTable[] {
  const out: StatementTable[] = []
  for (const t of tables) {
    if (t.alwaysShow) { out.push(t); continue }
    // 本来就没有行（该月无水费 / 无某分项 / 无关口电量）→ 整表连同标题一起隐藏。
    // 保留的话会打出一行「当月无用水记录」占掉版面，与「紧凑只展示真实数据」相悖。
    if (!t.rows.length) continue
    // rowSpans 与 rows 是平行数组：裁剪行时必须同步裁剪，否则跨行合并会错位。
    const keep = t.rows.map((row) => !isEmptyDetailRow(t.columns, row))
    const kept = t.rows.filter((_, i) => keep[i])
    if (!kept.length) continue
    if (kept.length === t.rows.length) { out.push(t); continue }
    const spans = t.rowSpans?.filter((_, i) => keep[i])
    out.push({ ...t, rows: kept, ...(spans ? { rowSpans: spans } : {}) })
  }
  return out
}

const alignRight = (align: string | undefined) => align === 'right'

/** 列宽：按权重分摊可用宽度，最小 34pt 防止数字列被压成竖排 */
function columnWidths(columns: StatementColumn[], avail: number): number[] {
  const weights = columns.map((c) => Math.max(0.15, Number(c.weight) || 1))
  const sum = weights.reduce((a, b) => a + b, 0)
  const widths = weights.map((w) => (avail * w) / sum)
  const floor = 34
  let deficit = 0
  widths.forEach((w, i) => {
    if (w < floor) { deficit += floor - w; widths[i] = floor }
  })
  if (deficit > 0) {
    const flexible = widths.map((w, i) => (w > floor ? i : -1)).filter((i) => i >= 0)
    const totalFlex = flexible.reduce((s, i) => s + widths[i], 0)
    // 全是窄列时无处可让，保持 floor 即可（宁可总宽略超，也不把数字列压成竖排）
    if (totalFlex > 0) for (const i of flexible) widths[i] -= (deficit * widths[i]) / totalFlex
  }
  return widths
}

export async function generateStatementPdf(opts: { doc: StatementDoc }): Promise<Uint8Array> {
  const { doc: source } = opts
  if (!source?.tables?.length) throw new Error('无可用数据生成清单')
  // 空明细行 + 空表在**进入排版前**统一裁剪：
  // 排在排版里做的话，被删行的行高已经被 ensure/翻页消费掉，PDF 仍会多出空白页。
  const doc: StatementDoc = { ...source, tables: pruneEmptyTables(source.tables) }
  if (!doc.tables.length) throw new Error('本期没有任何发生数据的明细')

  // 页面面板（字体 / 分页 / 页脚 / 游标）统一由 pdfSurface 提供 —— 与目录引擎同一份实现。
  // 清单是 A4 纵向 + 概览区，续页只画一条紧凑页眉（不重画表头：纵向表格每张表
  // 自己带表头，重画会与当前表的列宽冲突）。
  const surface = await createPdfSurface({
    geom: { width: PORTRAIT_W, height: PORTRAIT_H, margin: S_MARGIN },
    startY: PORTRAIT_H - S_MARGIN,
    bottomReserve: 30,
    onNewPage: (s) => {
      text(s, doc.title, S_MARGIN, s.y - 9, 9, C_MUTED)
      if (doc.subtitle) {
        const w = s.font.widthOfTextAtSize(doc.subtitle, 9)
        text(s, doc.subtitle, S_MARGIN + s.avail - w, s.y - 9, 9, C_MUTED)
      }
      s.y -= 16
      hline(s, S_MARGIN, S_MARGIN + s.avail, s.y)
      s.y -= 16
    },
  })
  const { font, avail } = surface
  // **不能解构 page**：翻页后局部变量仍指向旧页（见目录引擎同处注释）。
  const curPage = () => surface.page

  const now = new Date()
  const dateStr = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())} ${pad2(now.getHours())}:${pad2(now.getMinutes())}`

  // 三个绘制原语全部显式接收 surface：它们要跟着翻页走，
  // 写成闭包捕获局部 page 变量正是旧实现的隐患来源。
  const text = (s: PdfSurface, str: string, x: number, baseline: number, size: number, color = C_INK) => {
    if (!str) return
    s.page.drawText(str, { x, y: baseline, size, font, color })
  }
  const hline = (s: PdfSurface, fromX: number, toX: number, yy: number, color = C_RULE, thickness = 0.6) => {
    s.page.drawLine({ start: { x: fromX, y: yy }, end: { x: toX, y: yy }, color, thickness })
  }
  const vline = (s: PdfSurface, x: number, top: number, bottom: number, color = C_RULE, thickness = 0.5) => {
    s.page.drawLine({ start: { x, y: top }, end: { x, y: bottom }, color, thickness })
  }

  const drawSectionTitle = (title: string) => {
    surface.ensure(S_SECTION_SIZE + 22)
    curPage().drawRectangle({
      x: S_MARGIN, y: surface.y - S_SECTION_SIZE - 1,
      width: 3, height: S_SECTION_SIZE + 2, color: C_BRAND,
    })
    text(surface, title, S_MARGIN + 9, surface.y - S_SECTION_SIZE, S_SECTION_SIZE, C_INK)
    surface.y -= S_SECTION_SIZE + 10
  }

  // ---- 页眉：主标题（居中）+ 副标题 + 分隔线 ----
  // 第 41 轮紧凑化：标题/概览/KPI 合计省 ~20pt，把「电费分项细项」
  // 连表头带合计行完整留在第 1 页。数值都是留白，字号与内容一律不动。
  const titleW = font.widthOfTextAtSize(doc.title, S_TITLE_SIZE)
  text(surface, doc.title, S_MARGIN + (avail - titleW) / 2, surface.y - S_TITLE_SIZE, S_TITLE_SIZE, C_BRAND)
  surface.y -= S_TITLE_SIZE + 6            // 原 +10 → 省 4
  if (doc.subtitle) {
    const subW = font.widthOfTextAtSize(doc.subtitle, S_SUBTITLE_SIZE)
    text(surface, doc.subtitle, S_MARGIN + (avail - subW) / 2, surface.y - S_SUBTITLE_SIZE, S_SUBTITLE_SIZE, C_MUTED)
    surface.y -= S_SUBTITLE_SIZE + 6       // 原 +10 → 省 4
  }
  curPage().drawRectangle({ x: S_MARGIN, y: surface.y - 2, width: avail, height: 2, color: C_BRAND })
  surface.y -= 13                          // 原 16 → 省 3

  // ---- 概览：基本信息两列网格 ----
  if (doc.overviewTitle) drawSectionTitle(doc.overviewTitle)
  if (doc.meta?.length) {
    const half = avail / 2
    const perRow = 2
    for (let i = 0; i < doc.meta.length; i += perRow) {
      surface.ensure(S_META_SIZE + 12)
      const row = doc.meta.slice(i, i + perRow)
      row.forEach((m, idx) => {
        const label = `${m.label}`
        const labelW = font.widthOfTextAtSize(`${label}：`, S_META_SIZE)
        const x = S_MARGIN + idx * half
        text(surface, `${label}：`, x, surface.y - S_META_SIZE, S_META_SIZE, C_MUTED)
        text(surface, m.value || '—', x + labelW, surface.y - S_META_SIZE, S_META_SIZE, C_INK)
      })
      surface.y -= S_META_SIZE + 8         // 原 +11 → 省 3 × 4 行 = 12
    }
    surface.y -= 2                         // 原 4 → 省 2
  }

  // ---- 概览指标卡：一行最多 4 张 ----
  //
  // 卡片可以带一行 `hint`（如「工业 126600 + 宿舍 930」）。有 hint 时卡片加高到 54，
  // 三行文本自上而下是 标题(8pt) / 构成(7pt) / 数值(11.5pt)，互不重叠：
  // 标题基线 y-12、构成基线 y-23、数值基线 y-cardH+11。
  if (doc.kpis?.length) {
    const perRow = Math.min(4, doc.kpis.length)
    const gap = 8
    const cardW = (avail - gap * (perRow - 1)) / perRow
    const hasHint = doc.kpis.some((k) => k.hint)
    const cardH = hasHint ? 50 : 40          // 原 54 / 42 → 省 4 / 2
    for (let i = 0; i < doc.kpis.length; i += perRow) {
      surface.ensure(cardH + 10)
      const row = doc.kpis.slice(i, i + perRow)
      row.forEach((k, idx) => {
        const x = S_MARGIN + idx * (cardW + gap)
        curPage().drawRectangle({
          x, y: surface.y - cardH, width: cardW, height: cardH,
          color: rgb(0.985, 0.99, 0.988), borderColor: C_KPI_BORDER, borderWidth: 0.6,
        })
        text(surface, k.label, x + 8, surface.y - 12, 8, C_MUTED)
        if (k.hint) {
          // 构成说明**完整展示**，不截断。
          //
          // 第 36 轮：三段式「工业 166550 + 差额 5850 + 宿舍 686」比原来的
          // 两段式长，7pt 下宽约 127pt、超过卡片可用宽 109pt，旧实现按字符
          // 截断成「工业 166550 + 差额 5850 + …」—— 用户看不到「宿舍 686」，
          // 三项加起来也没法核对是否等于下方大字值。
          // 改为**自适应字号**：7 → 6.5 → 6 → 5.5，逐级缩到能放下为止。
          // 5.5pt 仍放不下（极端长文案）才退回截断，保证不压到右侧数值。
          const maxW = cardW - 16
          let hint = k.hint
          let hintSize = 7
          for (const s of [7, 6.5, 6, 5.5]) {
            hintSize = s
            if (font.widthOfTextAtSize(hint, s) <= maxW) break
          }
          if (font.widthOfTextAtSize(hint, hintSize) > maxW) {
            while (hint.length > 1 && font.widthOfTextAtSize(hint, hintSize) > maxW) hint = hint.slice(0, -1)
            hint = `${hint.slice(0, -1)}…`
          }
          text(surface, hint, x + 8, surface.y - 23, hintSize, C_MUTED)
        }
        const vSize = 11.5
        const vw = font.widthOfTextAtSize(k.value, vSize)
        text(surface, k.value, x + cardW - 8 - vw, surface.y - cardH + 11, vSize, C_INK)
      })
      surface.y -= cardH + 7               // 原 +10 → 省 3
    }
  }

  // ---- 表格区 ----
  for (let tableIdx = 0; tableIdx < doc.tables.length; tableIdx++) {
    const table = doc.tables[tableIdx]
    drawSectionTitle(table.title)

    // 带 rowSpans 的表（电费分项细项）用紧凑行高：24 行明细 + 表头 + 合计
    // 完整落在第 1 页。其它明细表沿用原行高，版式不受影响。
    const compact = !!table.rowSpans?.length
    const rowMin = compact ? S_COMPACT_ROW_MIN : S_ROW_MIN
    const lineH = compact ? S_COMPACT_LINE_H : S_LINE_H
    const headerH = compact ? 20 : S_HEADER_H

    const widths = columnWidths(table.columns, avail)
    const xs: number[] = []
    let acc = S_MARGIN
    for (const w of widths) { xs.push(acc); acc += w }

    const drawHead = () => {
      curPage().drawRectangle({
        x: S_MARGIN, y: surface.y - headerH, width: avail, height: headerH, color: C_HEAD_BG,
      })
      hline(surface, S_MARGIN, S_MARGIN + avail, surface.y, C_RULE, 0.8)
      hline(surface, S_MARGIN, S_MARGIN + avail, surface.y - headerH, C_RULE, 0.8)
      table.columns.forEach((c, i) => {
        const label = c.label
        const tw = font.widthOfTextAtSize(label, S_TH_SIZE)
        const cx = alignRight(c.align)
          ? xs[i] + widths[i] - S_CELL_PAD - tw
          : c.align === 'center'
            ? xs[i] + (widths[i] - tw) / 2
            : xs[i] + S_CELL_PAD
        text(surface, label, cx, surface.y - headerH + 7, S_TH_SIZE, C_INK)
        if (i > 0) vline(surface, xs[i], surface.y, surface.y - headerH, C_RULE, 0.5)
      })
      surface.y -= headerH
    }

    surface.ensure(headerH + rowMin * 2)
    drawHead()

    // 第 40 轮：预计算行高，供跨行合并单元格算「合并总高」做垂直居中。
    // 第 41 轮：含 `\n` 的「费用类别」双行文案放在 rowspan 合并区里居中绘制，
    // 因此**测高时按单行计**——否则每个大类首行都会被撑成 2 行高，24 行
    // 反而更高，第一页依旧放不下。
    const measureRow = (cells: string[]) => {
      const wrapped = table.columns.map((c, i) =>
        wrapText(font, String(cells[i] ?? ''), widths[i] - S_CELL_PAD * 2, S_TD_SIZE))
      const lines = Math.max(...wrapped.map((w) => w.length))
      return { wrapped, h: Math.max(rowMin, lines * lineH + 8) }
    }
    const measured = table.rows.map((row, ri) => {
      const wrapped = table.columns.map((c, i) =>
        wrapText(font, String(row[i] ?? ''), widths[i] - S_CELL_PAD * 2, S_TD_SIZE))
      // 合并起始格的文本高度由整个 rowspan 区域承担，不参与本行行高计算。
      const lines = Math.max(1, ...wrapped
        .map((w, i) => ((table.rowSpans?.[ri]?.[i] ?? 1) > 1 ? 1 : w.length)))
      return { wrapped, h: Math.max(rowMin, lines * lineH + 8) }
    })
    const rowHeights = measured.map((m) => m.h)
    const spansOf = (r: number): number[] | undefined => table.rowSpans?.[r]
    /** 该行某列的有效合并行数（裁剪/跨页保护后至少 1）。 */
    const spanOf = (r: number, c: number): number => {
      const v = spansOf(r)?.[c] ?? 1
      // 被上方合并区覆盖 → 0；起始行若超出剩余行数则收缩到可用范围
      return v === 0 ? 0 : Math.max(1, Math.min(v, table.rows.length - r))
    }

    const drawRow = (cells: string[], r: number, rowOpts: { total?: boolean; zebra?: boolean } = {}) => {
      const { wrapped, h } = rowOpts.total
        ? measureRow(cells)
        : (measured[r] || measureRow(cells))
      if (surface.y - h < S_MARGIN + 24) {
        surface.newPage()
        // 翻页后重画表头（列宽不变）
        drawHead()
      }
      const bg = rowOpts.total ? C_TOTAL_BG : (rowOpts.zebra ? C_ZEBRA : null)
      const spanRow = rowOpts.total ? undefined : spansOf(r)
      // 合并区总高：只在**同一页放得下**时才做真正的跨行合并；
      // 放不下就退化为单行（避免把文字画到下一页去）。
      const mergedHeights = table.columns.map((_, c) => {
        const n = spanOf(r, c)
        if (n <= 1) return h
        let total = 0
        for (let k = 0; k < n; k++) total += rowHeights[r + k] ?? h
        return total
      })
      const canMerge = (c: number) => spanOf(r, c) > 1 && surface.y - mergedHeights[c] >= S_MARGIN + 24

      // 背景按列段绘制：被上方合并区覆盖的列跳过（否则会盖掉合并单元格文字）。
      if (bg) {
        table.columns.forEach((_c, i) => {
          if (!rowOpts.total && spanRow?.[i] === 0) return
          const hh = canMerge(i) ? mergedHeights[i] : h
          curPage().drawRectangle({ x: xs[i], y: surface.y - hh, width: widths[i], height: hh, color: bg })
        })
      }
      if (rowOpts.total) hline(surface, S_MARGIN, S_MARGIN + avail, surface.y, C_RULE, 0.9)

      table.columns.forEach((c, i) => {
        // 被合并覆盖的单元格不绘制文字（文字只在合并区首格画一次）
        if (!rowOpts.total && spanRow?.[i] === 0) {
          if (i > 0) vline(surface, xs[i], surface.y, surface.y - h, C_RULE, 0.4)
          return
        }
        const raw = String(cells[i] ?? '')
        const cellH = canMerge(i) ? mergedHeights[i] : h
        const centered = !alignRight(c.align) && c.align === 'center'
        // 第 41 轮：大类合并格的「名称 + 小计」双行文本。
        // 主行用标准字号/主色，小计行小 0.8pt + 次要色，两行整体垂直居中。
        const two = !rowOpts.total && raw.includes('\n') ? splitCategoryLabel(raw) : null
        if (two?.sub) {
          const mainSize = S_TD_SIZE
          const subSize = S_TD_SIZE - 0.8
          const totalH = 2 * lineH
          const startY = surface.y - cellH + (cellH - totalH) / 2
          const drawLine = (s: string, size: number, baseline: number, color: typeof C_INK) => {
            const tw = font.widthOfTextAtSize(s, size)
            const cx = centered ? xs[i] + (widths[i] - tw) / 2 : xs[i] + S_CELL_PAD
            text(surface, s, cx, baseline, size, color)
          }
          drawLine(two.main, mainSize, startY + lineH + 2.5, C_INK)
          drawLine(two.sub, subSize, startY + 2.5, C_MUTED)
          if (i > 0) vline(surface, xs[i], surface.y, surface.y - h, C_RULE, 0.4)
          return
        }
        // 普通单元格：文本列左对齐、时段列居中、数量/单价/金额右对齐。
        // drawWrappedCell 只认 left/right，center 用「起始 x 偏移」实现。
        const lineWidths = wrapped[i].map((l) => font.widthOfTextAtSize(l, S_TD_SIZE))
        const centerShift = centered
          ? (widths[i] - Math.max(0, ...lineWidths)) / 2 - S_CELL_PAD
          : 0
        drawWrappedCell(curPage(), {
          lines: wrapped[i], x: xs[i] + centerShift, top: surface.y,
          height: cellH,
          size: S_TD_SIZE,
          lineHeight: lineH, padX: S_CELL_PAD, font, color: C_INK,
          align: alignRight(c.align) ? 'right' : 'left',
          width: widths[i] - centerShift,
          baselineShift: 2.5,
        })
        if (i > 0) vline(surface, xs[i], surface.y, surface.y - h, C_RULE, 0.4)
      })
      // 横线按列段绘制：合并区内部的边界不画线，视觉上就是一格跨 N 行。
      if (!rowOpts.total) {
        const next = spansOf(r + 1)
        table.columns.forEach((_c, i) => {
          if (next?.[i] === 0) return
          hline(surface, xs[i], xs[i] + widths[i], surface.y - h, C_RULE, 0.4)
        })
      }
      surface.y -= h
    }

    if (!table.rows.length) {
      surface.ensure(S_ROW_MIN)
      text(surface, table.empty || '本期无数据', S_MARGIN + S_CELL_PAD, surface.y - 13, S_TD_SIZE, C_MUTED)
      hline(surface, S_MARGIN, S_MARGIN + avail, surface.y - S_ROW_MIN, C_RULE, 0.4)
      surface.y -= S_ROW_MIN
    } else {
      table.rows.forEach((row, idx) => drawRow(row, idx, { zebra: idx % 2 === 1 }))
    }
    if (table.total?.length && table.rows.length) drawRow(table.total, table.rows.length, { total: true })
    surface.y -= 10
    // 第 41 轮：分项细项表画完后强制翻页，第 2 页顶部从「电费核算明细」
    // 开始。只在后面还有表时翻，避免末尾多出一张空白页。
    if (table.pageBreakAfter && tableIdx < doc.tables.length - 1) {
      surface.drawFooter()
      surface.newPage()
    }
  }

  // ---- 页脚备注 ----
  if (doc.footerNote) {
    surface.ensure(30)
    const lines = wrapText(font, doc.footerNote, avail, 8)
    lines.forEach((line) => {
      text(surface, line, S_MARGIN, surface.y - 9, 8, C_MUTED)
      surface.y -= 12
    })
  }

  // ---- 统一页脚：标题 · 生成时间 · 第 N / 共 M 页 ----
  // 清单页脚要写「共 M 页」，只有全部页生成完才知道 M，所以不能走 surface 的
  // 逐页页脚钩子（那是目录引擎的「第 N 页」形态），在这里统一补一遍。
  const pages = surface.doc.getPages()
  pages.forEach((p, i) => {
    const label = `${doc.title} · 生成于 ${dateStr}`
    p.drawLine({
      start: { x: S_MARGIN, y: S_MARGIN - 6 },
      end: { x: PORTRAIT_W - S_MARGIN, y: S_MARGIN - 6 },
      color: C_RULE, thickness: 0.5,
    })
    p.drawText(label, { x: S_MARGIN, y: S_MARGIN - 17, size: 7.2, font, color: C_MUTED })
    const pn = `第 ${i + 1} / ${pages.length} 页`
    const pw = font.widthOfTextAtSize(pn, 7.2)
    p.drawText(pn, { x: PORTRAIT_W - S_MARGIN - pw, y: S_MARGIN - 17, size: 7.2, font, color: C_MUTED })
  })

  return surface.doc.save()
}
