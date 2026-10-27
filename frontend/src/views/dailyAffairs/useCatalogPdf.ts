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

  // 标题
  page.drawText(title, { x: MARGIN, y, size: TITLE_SIZE, font, color: COLOR_TITLE })
  y -= 6
  // 元信息（共 N 条 / 生成时间）
  const meta = `共 ${rows.length} 条 · 生成时间 ${dateStr}`
  page.drawText(meta, { x: MARGIN, y: y - 10, size: META_SIZE, font, color: COLOR_MUTED })
  y -= 10 + META_SIZE + 10

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
// 清单 = 每个所选记录一页纵向结算单（基本信息 + 逐项分摊明细 + 合计），
// 适合直接发给租户。两者共用同一套中文字体与折行逻辑，样式天然一致。
// ---------------------------------------------------------------------------

const PORTRAIT_W = 595.28
const PORTRAIT_H = 841.89

export interface StatementItem {
  label: string
  /** 来源分类（市电账单 / 电费核算 …），打印时单独一列便于对账 */
  category?: string
  value: string
}

export interface StatementRecord {
  /** 标题（如「2026-09 费用清单 · 持睿汽车」） */
  title: string
  /** 基本信息行（月份 / 使用单位 / 分摊比例 …） */
  meta: Array<{ label: string; value: string }>
  items: StatementItem[]
  /** 合计应付 */
  total: string
}

export async function generateStatementPdf(opts: {
  title: string
  records: StatementRecord[]
}): Promise<Uint8Array> {
  const { title, records } = opts
  if (!records.length) throw new Error('无可用数据生成清单')

  const fontBytes = await loadCatalogFont()
  const doc = await PDFDocument.create()
  doc.registerFontkit(fontkit)
  const font = await doc.embedFont(fontBytes)

  const avail = PORTRAIT_W - MARGIN * 2
  const colW = [avail - 90 - 110, 90, 110]
  const now = new Date()
  const dateStr = `${now.getFullYear()}-${pad2(now.getMonth() + 1)}-${pad2(now.getDate())} ${pad2(now.getHours())}:${pad2(now.getMinutes())}`

  let page = doc.addPage([PORTRAIT_W, PORTRAIT_H])
  let y = PORTRAIT_H - MARGIN - TITLE_SIZE - 4
  let pageNo = 1
  const drawFooter = () => {
    page.drawText(`${title} · 第 ${pageNo} 页 · 生成时间 ${dateStr}`, { x: MARGIN, y: MARGIN - 8, size: 7.5, font, color: COLOR_MUTED })
  }
  const newPage = () => {
    drawFooter()
    page = doc.addPage([PORTRAIT_W, PORTRAIT_H])
    pageNo++
    y = PORTRAIT_H - MARGIN - 16
  }
  const ensure = (need: number) => { if (y - need < MARGIN + 24) newPage() }

  page.drawText(title, { x: MARGIN, y, size: TITLE_SIZE, font, color: COLOR_TITLE })
  y -= 6

  for (const rec of records) {
    ensure(70)
    // 记录标题
    page.drawText(rec.title, { x: MARGIN, y, size: 12, font, color: COLOR_TEXT })
    y -= 14
    // 基本信息两列
    for (const m of rec.meta) {
      page.drawText(`${m.label}：${m.value}`, { x: MARGIN + 2, y, size: META_SIZE, font, color: COLOR_MUTED })
      y -= 12
    }
    y -= 4
    // 明细表头
    ensure(HEADER_HEIGHT + 24)
    const drawRowBg = (h: number, bg?: any) => {
      page.drawRectangle({ x: MARGIN, y: y - h, width: avail, height: h, color: bg ?? rgb(1, 1, 1) })
      page.drawRectangle({ x: MARGIN, y: y - h, width: avail, height: h, borderColor: COLOR_BORDER, borderWidth: 0.5 })
    }
    let x = MARGIN
    drawRowBg(HEADER_HEIGHT, COLOR_HEADER_BG)
    for (const [i, label] of ['项目', '来源分类', '金额(元)'].entries()) {
      page.drawText(label, { x: x + CELL_PAD, y: y - HEADER_HEIGHT + 9, size: HEADER_SIZE, font, color: COLOR_TEXT })
      x += colW[i]
    }
    y -= HEADER_HEIGHT

    for (const [idx, it] of rec.items.entries()) {
      const nameLines = wrapText(font, it.label, colW[0] - CELL_PAD * 2, CELL_SIZE)
      const h = Math.max(ROW_MIN_HEIGHT, nameLines.length * LINE_HEIGHT + CELL_PAD)
      ensure(h)
      const top = y
      if (idx % 2 === 1) drawRowBg(h, COLOR_ZEBRA_BG)
      else drawRowBg(h)
      let cx = MARGIN
      const totalH = nameLines.length * LINE_HEIGHT
      nameLines.forEach((line, li) => {
        page.drawText(line, {
          x: cx + CELL_PAD,
          y: top - h + (h - totalH) / 2 + totalH - (li + 1) * LINE_HEIGHT,
          size: CELL_SIZE, font, color: COLOR_TEXT,
        })
      })
      cx += colW[0]
      page.drawText(it.category || '—', { x: cx + CELL_PAD, y: top - h + 7, size: CELL_SIZE, font, color: COLOR_MUTED })
      cx += colW[1]
      page.drawText(it.value, { x: cx + CELL_PAD, y: top - h + 7, size: CELL_SIZE, font, color: COLOR_TEXT })
      y -= h
    }

    // 合计
    ensure(ROW_MIN_HEIGHT + 8)
    drawRowBg(ROW_MIN_HEIGHT, COLOR_HEADER_BG)
    page.drawText('合计应付', { x: MARGIN + CELL_PAD, y: y - ROW_MIN_HEIGHT + 7, size: CELL_SIZE + 0.5, font, color: COLOR_TEXT })
    const totalW = font.widthOfTextAtSize(rec.total, CELL_SIZE + 0.5)
    page.drawText(rec.total, {
      x: MARGIN + avail - CELL_PAD - totalW, y: y - ROW_MIN_HEIGHT + 7,
      size: CELL_SIZE + 0.5, font, color: COLOR_TEXT,
    })
    y -= ROW_MIN_HEIGHT + 18
  }

  drawFooter()
  return doc.save()
}
