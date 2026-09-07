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

// 按字符宽度换行，返回多行文本（完整显示，不截断）
function wrapText(font: PDFFont, text: string, maxWidth: number, size: number): string[] {
  if (!text) return ['']
  const lines: string[] = []
  let cur = ''
  let curW = 0
  for (const ch of text) {
    const w = font.widthOfTextAtSize(ch, size)
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
  return lines.length ? lines : ['']
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
  const colMax = columns.map((c) => {
    let max = font.widthOfTextAtSize(c.label, HEADER_SIZE)
    for (const r of rows) {
      const w = font.widthOfTextAtSize(c.value(r) || '', CELL_SIZE)
      if (w > max) max = w
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
  const headerLines = columns.map((c, i) => wrapText(font, c.label, widths[i] - CELL_PAD * 2, HEADER_SIZE))
  const headerMaxLines = Math.max(...headerLines.map((ls) => ls.length))

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
      const lines = headerLines[i]
      const totalH = lines.length * LINE_HEIGHT
      const startY = y - h + (h - totalH) / 2
      lines.forEach((line, li) => {
        page.drawText(line, { x: x + CELL_PAD, y: startY + totalH - (li + 1) * LINE_HEIGHT, size: HEADER_SIZE, font, color: COLOR_TEXT })
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
