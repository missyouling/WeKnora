// 目录 PDF 生成工具：把选中记录的字段清单渲染为可打印/可下载的 PDF。
// 表格式清单，固定 A4 横向；列宽按内容权重自适应（单列上限约页宽 30%），
// 超长内容自动换行完整显示（不做省略号截断），行高随行数自适应。
// 内嵌开源中文字体（霞鹜文楷，OFL）。
import { PDFDocument, PDFFont, rgb } from 'pdf-lib'
import fontkit from '@pdf-lib/fontkit'

export interface CatalogColumn {
  key: string
  label: string
  value: (row: any) => string
}

let fontPromise: Promise<ArrayBuffer> | null = null
export function loadCatalogFont(): Promise<ArrayBuffer> {
  if (!fontPromise) {
    fontPromise = fetch('/fonts/LXGWWenKai-Regular.subset.ttf')
      .then((r) => {
        if (!r.ok) throw new Error('中文字体加载失败')
        return r.arrayBuffer()
      })
      .catch((e) => {
        fontPromise = null
        throw e
      })
  }
  return fontPromise
}

// 固定 A4 横向（方向由生成时定死，用户打印时可在系统打印对话框内用缩放适配）
const PAGE_W = 841.89
const PAGE_H = 595.28
const MARGIN = 32
const TITLE_SIZE = 15
const META_SIZE = 9
const HEADER_SIZE = 8.5
const CELL_SIZE = 8.5
const LINE_HEIGHT = 12.5
const HEADER_HEIGHT = 26
const CELL_PAD = 6
const MIN_COL_W = 40
// 单列上限：约页宽 30%，超过则内容换行完整显示
const MAX_COL_W = Math.floor((PAGE_W - MARGIN * 2) * 0.3)
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

  const fontBytes = await loadCatalogFont()
  const doc = await PDFDocument.create()
  doc.registerFontkit(fontkit)
  const font = await doc.embedFont(fontBytes)

  const avail = PAGE_W - MARGIN * 2

  // 列宽：按 表头/内容 最大宽度估算（内容权重），clamp 到 [MIN_COL_W, MAX_COL_W]
  // 多行文本取「最长单行」而非整段宽度：否则一段带换行的备注会把列撑到 MAX_COL_W，
  // 其余列被压到 MIN_COL_W，整张目录表比例失衡。
  const colMax = columns.map((c) => {
    let max = font.widthOfTextAtSize(c.label, HEADER_SIZE)
    for (const r of rows) {
      for (const line of splitExplicitLines(c.value(r) || '')) {
        const w = font.widthOfTextAtSize(line, CELL_SIZE)
        if (w > max) max = w
      }
    }
    return Math.min(MAX_COL_W, Math.max(MIN_COL_W, max + CELL_PAD * 2))
  })
  // 归一化：未超出则按比例撑满；超出则按比例压缩（保底 MIN_COL_W）
  let widths: number[]
  const rawSum = colMax.reduce((a, b) => a + b, 0)
  if (rawSum <= avail) {
    const extra = avail - rawSum
    widths = colMax.map((w) => Math.min(MAX_COL_W, w + (w / rawSum) * extra))
    // 撑满后仍有富余则平均补到最后一列，避免表格不贴底
    const sum = widths.reduce((a, b) => a + b, 0)
    if (sum < avail) widths[widths.length - 1] += avail - sum
  } else {
    const scale = avail / rawSum
    widths = colMax.map((w) => Math.max(MIN_COL_W, w * scale))
    const sum = widths.reduce((a, b) => a + b, 0)
    if (sum > avail) {
      const floored = widths.filter((w) => w > MIN_COL_W).length
      const extraNeed = sum - avail
      const shrink = floored > 0 ? extraNeed / floored : 1
      widths = widths.map((w) => (w > MIN_COL_W ? Math.max(MIN_COL_W, w - shrink) : w))
    }
  }

  const now = new Date()
  const dateStr = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())} ${pad2(now.getHours())}:${pad2(now.getMinutes())}`

  // 预计算每行的换行行数与行高
  const rowLines = rows.map((row) =>
    columns.map((c, i) => wrapText(font, c.value(row) || '', widths[i] - CELL_PAD * 2, CELL_SIZE))
  )
  const rowHeights = rowLines.map((linesArr) => {
    const maxLines = Math.max(...linesArr.map((ls) => ls.length))
    return Math.max(ROW_MIN_HEIGHT, maxLines * LINE_HEIGHT + CELL_PAD)
  })
  // 表头单行自适应：优先单行完整显示，列宽不足时缩小字号（下限 6），仍不足才换行兜底
  const headerFonts = columns.map((c, i) => {
    const availW = widths[i] - CELL_PAD * 2
    let size = HEADER_SIZE
    while (size > 6 && font.widthOfTextAtSize(c.label, size) > availW) size -= 0.5
    const lines = wrapText(font, c.label, availW, size)
    return { size, lines }
  })
  const headerMaxLines = Math.max(...headerFonts.map((h) => h.lines.length))

  let page = doc.addPage([PAGE_W, PAGE_H])
  let y = PAGE_H - MARGIN - TITLE_SIZE - 4

  // 标题基线；副标题必须落到标题下沿之外，否则 15pt 标题与 9pt 元信息会叠字
  page.drawText(title, { x: MARGIN, y, size: TITLE_SIZE, font, color: COLOR_TITLE })
  y -= TITLE_SIZE + 6
  // 元信息（共 N 条 / 生成时间）
  const meta = `共 ${rows.length} 条 · 生成时间 ${dateStr}`
  page.drawText(meta, { x: MARGIN, y, size: META_SIZE, font, color: COLOR_MUTED })
  y -= META_SIZE + 14

  let pageNo = 1
  const drawFooter = () => {
    page.drawText(`${title} · 第 ${pageNo} 页`, { x: MARGIN, y: MARGIN - 8, size: 8, font, color: COLOR_MUTED })
  }

  const drawHeaderRow = () => {
    let x = MARGIN
    const h = Math.max(HEADER_HEIGHT, headerMaxLines * LINE_HEIGHT + CELL_PAD)
    page.drawRectangle({ x: MARGIN, y: y - h, width: avail, height: h, color: COLOR_HEADER_BG })
    page.drawRectangle({ x: MARGIN, y: y - h, width: avail, height: h, borderColor: COLOR_BORDER, borderWidth: 0.5 })
    columns.forEach((c, i) => {
      const w = widths[i]
      const { size, lines } = headerFonts[i]
      const totalH = lines.length * LINE_HEIGHT
      const startY = y - h + (h - totalH) / 2
      lines.forEach((line, li) => {
        page.drawText(line, { x: x + CELL_PAD, y: startY + totalH - (li + 1) * LINE_HEIGHT, size, font, color: COLOR_TEXT })
      })
      x += w
    })
    y -= h
  }

  const ensureSpace = (needed: number) => {
    if (y - needed < MARGIN + 22) {
      drawFooter()
      page = doc.addPage([PAGE_W, PAGE_H])
      pageNo++
      y = PAGE_H - MARGIN - HEADER_HEIGHT
      drawHeaderRow()
    }
  }

  drawHeaderRow()

  rows.forEach((row, idx) => {
    const linesArr = rowLines[idx]
    const rh = rowHeights[idx]
    ensureSpace(rh)
    const rowTop = y
    // 斑马纹
    if (idx % 2 === 1) {
      page.drawRectangle({ x: MARGIN, y: y - rh, width: avail, height: rh, color: COLOR_ZEBRA_BG })
    }
    // 边框
    page.drawRectangle({ x: MARGIN, y: y - rh, width: avail, height: rh, borderColor: COLOR_BORDER, borderWidth: 0.5 })
    let x = MARGIN
    columns.forEach((c, i) => {
      const w = widths[i]
      const lines = linesArr[i]
      const totalH = lines.length * LINE_HEIGHT
      const startY = rowTop - rh + (rh - totalH) / 2
      lines.forEach((line, li) => {
        page.drawText(line, {
          x: x + CELL_PAD,
          y: startY + totalH - (li + 1) * LINE_HEIGHT,
          size: CELL_SIZE,
          font,
          color: c.value(row) ? COLOR_TEXT : COLOR_MUTED,
        })
      })
      x += w
    })
    y -= rh
  })

  drawFooter()

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
  /** 合计行（整行加粗、灰底、顶部深色线） */
  total?: string[]
  /** 无数据时的占位文案 */
  empty?: string
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

  const fontBytes = await loadCatalogFont()
  const pdf = await PDFDocument.create()
  pdf.registerFontkit(fontkit)
  const font = await pdf.embedFont(fontBytes)

  const avail = PORTRAIT_W - S_MARGIN * 2
  const now = new Date()
  const dateStr = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())} ${pad2(now.getHours())}:${pad2(now.getMinutes())}`

  let page = pdf.addPage([PORTRAIT_W, PORTRAIT_H])
  let y = PORTRAIT_H - S_MARGIN

  const text = (
    s: string,
    x: number,
    baseline: number,
    size: number,
    color = C_INK,
  ) => {
    if (!s) return
    page.drawText(s, { x, y: baseline, size, font, color })
  }
  const hline = (fromX: number, toX: number, yy: number, color = C_RULE, thickness = 0.6) => {
    page.drawLine({ start: { x: fromX, y: yy }, end: { x: toX, y: yy }, color, thickness })
  }
  const vline = (x: number, top: number, bottom: number, color = C_RULE, thickness = 0.5) => {
    page.drawLine({ start: { x, y: top }, end: { x, y: bottom }, color, thickness })
  }

  /** 续页：留一个紧凑的页眉，避免翻页后不知道在看哪张表 */
  const newPage = () => {
    page = pdf.addPage([PORTRAIT_W, PORTRAIT_H])
    y = PORTRAIT_H - S_MARGIN
    text(source.title, S_MARGIN, y - 9, 9, C_MUTED)
    if (source.subtitle) {
      const w = font.widthOfTextAtSize(source.subtitle, 9)
      text(source.subtitle, S_MARGIN + avail - w, y - 9, 9, C_MUTED)
    }
    y -= 16
    hline(S_MARGIN, S_MARGIN + avail, y)
    y -= 16
  }
  const ensure = (need: number) => {
    if (y - need < S_MARGIN + 30) newPage()
  }

  const drawSectionTitle = (title: string) => {
    ensure(S_SECTION_SIZE + 22)
    page.drawRectangle({ x: S_MARGIN, y: y - S_SECTION_SIZE - 1, width: 3, height: S_SECTION_SIZE + 2, color: C_BRAND })
    text(title, S_MARGIN + 9, y - S_SECTION_SIZE, S_SECTION_SIZE, C_INK)
    y -= S_SECTION_SIZE + 10
  }

  // ---- 页眉：主标题（居中）+ 副标题 + 分隔线 ----
  const titleW = font.widthOfTextAtSize(source.title, S_TITLE_SIZE)
  text(source.title, S_MARGIN + (avail - titleW) / 2, y - S_TITLE_SIZE, S_TITLE_SIZE, C_BRAND)
  y -= S_TITLE_SIZE + 10
  if (source.subtitle) {
    const subW = font.widthOfTextAtSize(source.subtitle, S_SUBTITLE_SIZE)
    text(source.subtitle, S_MARGIN + (avail - subW) / 2, y - S_SUBTITLE_SIZE, S_SUBTITLE_SIZE, C_MUTED)
    y -= S_SUBTITLE_SIZE + 10
  }
  page.drawRectangle({ x: S_MARGIN, y: y - 2, width: avail, height: 2, color: C_BRAND })
  y -= 16

  // ---- 概览：基本信息两列网格 ----
  if (source.overviewTitle) drawSectionTitle(source.overviewTitle)
  if (source.meta?.length) {
    const half = avail / 2
    const perRow = 2
    for (let i = 0; i < source.meta.length; i += perRow) {
      ensure(S_META_SIZE + 12)
      const row = source.meta.slice(i, i + perRow)
      row.forEach((m, idx) => {
        const label = `${m.label}`
        const labelW = font.widthOfTextAtSize(`${label}：`, S_META_SIZE)
        const x = S_MARGIN + idx * half
        text(`${label}：`, x, y - S_META_SIZE, S_META_SIZE, C_MUTED)
        text(m.value || '—', x + labelW, y - S_META_SIZE, S_META_SIZE, C_INK)
      })
      y -= S_META_SIZE + 11
    }
    y -= 4
  }

  // ---- 概览指标卡：一行最多 4 张 ----
  if (source.kpis?.length) {
    const perRow = Math.min(4, source.kpis.length)
    const gap = 8
    const cardW = (avail - gap * (perRow - 1)) / perRow
    const cardH = 42
    for (let i = 0; i < source.kpis.length; i += perRow) {
      ensure(cardH + 10)
      const row = source.kpis.slice(i, i + perRow)
      row.forEach((k, idx) => {
        const x = S_MARGIN + idx * (cardW + gap)
        page.drawRectangle({
          x, y: y - cardH, width: cardW, height: cardH,
          color: rgb(0.985, 0.99, 0.988), borderColor: C_KPI_BORDER, borderWidth: 0.6,
        })
        text(k.label, x + 8, y - 13, 8, C_MUTED)
        const vSize = 11.5
        const vw = font.widthOfTextAtSize(k.value, vSize)
        text(k.value, x + cardW - 8 - vw, y - cardH + 12, vSize, C_INK)
        if (k.hint) text(k.hint, x + 8, y - cardH + 12, 7.5, C_MUTED)
      })
      y -= cardH + 10
    }
  }

  // ---- 表格区 ----
  for (const table of source.tables) {
    drawSectionTitle(table.title)

    const widths = columnWidths(table.columns, avail)
    const xs: number[] = []
    let acc = S_MARGIN
    for (const w of widths) { xs.push(acc); acc += w }

    const drawHead = () => {
      page.drawRectangle({ x: S_MARGIN, y: y - S_HEADER_H, width: avail, height: S_HEADER_H, color: C_HEAD_BG })
      hline(S_MARGIN, S_MARGIN + avail, y, C_RULE, 0.8)
      hline(S_MARGIN, S_MARGIN + avail, y - S_HEADER_H, C_RULE, 0.8)
      table.columns.forEach((c, i) => {
        const label = c.label
        const tw = font.widthOfTextAtSize(label, S_TH_SIZE)
        const cx = alignRight(c.align)
          ? xs[i] + widths[i] - S_CELL_PAD - tw
          : c.align === 'center'
            ? xs[i] + (widths[i] - tw) / 2
            : xs[i] + S_CELL_PAD
        text(label, cx, y - S_HEADER_H + 7, S_TH_SIZE, C_INK)
        if (i > 0) vline(xs[i], y, y - S_HEADER_H, C_RULE, 0.5)
      })
      y -= S_HEADER_H
    }

    ensure(S_HEADER_H + S_ROW_MIN * 2)
    drawHead()

    const drawRow = (cells: string[], opts: { total?: boolean; zebra?: boolean } = {}) => {
      const wrapped = table.columns.map((c, i) => wrapText(font, String(cells[i] ?? ''), widths[i] - S_CELL_PAD * 2, S_TD_SIZE))
      const lines = Math.max(...wrapped.map((w) => w.length))
      const h = Math.max(S_ROW_MIN, lines * S_LINE_H + 8)
      if (y - h < S_MARGIN + 24) {
        newPage()
        // 翻页后重画表头（列宽不变）
        drawHead()
      }
      if (opts.total) {
        page.drawRectangle({ x: S_MARGIN, y: y - h, width: avail, height: h, color: C_TOTAL_BG })
        hline(S_MARGIN, S_MARGIN + avail, y, C_RULE, 0.9)
      } else if (opts.zebra) {
        page.drawRectangle({ x: S_MARGIN, y: y - h, width: avail, height: h, color: C_ZEBRA })
      }
      table.columns.forEach((c, i) => {
        const cellLines = wrapped[i]
        const totalH = cellLines.length * S_LINE_H
        const startBaseline = y - h + (h - totalH) / 2 + totalH - S_LINE_H + 2.5
        cellLines.forEach((line, li) => {
          const lw = font.widthOfTextAtSize(line, S_TD_SIZE)
          const lx = alignRight(c.align)
            ? xs[i] + widths[i] - S_CELL_PAD - lw
            : c.align === 'center'
              ? xs[i] + (widths[i] - lw) / 2
              : xs[i] + S_CELL_PAD
          text(line, lx, startBaseline - li * S_LINE_H, S_TD_SIZE, C_INK)
        })
        if (i > 0) vline(xs[i], y, y - h, C_RULE, 0.4)
      })
      if (!opts.total) hline(S_MARGIN, S_MARGIN + avail, y - h, C_RULE, 0.4)
      y -= h
    }

    if (!table.rows.length) {
      ensure(S_ROW_MIN)
      text(table.empty || '本期无数据', S_MARGIN + S_CELL_PAD, y - 13, S_TD_SIZE, C_MUTED)
      hline(S_MARGIN, S_MARGIN + avail, y - S_ROW_MIN, C_RULE, 0.4)
      y -= S_ROW_MIN
    } else {
      table.rows.forEach((row, idx) => drawRow(row, { zebra: idx % 2 === 1 }))
    }
    if (table.total?.length && table.rows.length) drawRow(table.total, { total: true })
    y -= 10
  }

  // ---- 页脚备注 ----
  if (source.footerNote) {
    ensure(30)
    const lines = wrapText(font, source.footerNote, avail, 8)
    lines.forEach((line) => {
      text(line, S_MARGIN, y - 9, 8, C_MUTED)
      y -= 12
    })
  }

  // ---- 统一页脚：标题 · 生成时间 · 第 N / 共 M 页 ----
  const pages = pdf.getPages()
  pages.forEach((p, i) => {
    const label = `${source.title} · 生成于 ${dateStr}`
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

  return pdf.save()
}
