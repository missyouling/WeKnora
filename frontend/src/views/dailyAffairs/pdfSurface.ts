// PDF 绘制的共享核心（第 24 轮任务一：双引擎收敛）。
//
// 背景：`useCatalogPdf.ts` 里并存两套页面排版 ——
//   · `generateCatalogPdf`（9 个页面用的「目录」，A4 横向，纯表格流）；
//   · `generateStatementPdf`（3 个能耗页用的「清单」，A4 纵向，概览 + 分节表格）。
// 两者各自实现了一遍**字体加载、页眉页脚、分页判定、单元格绘制、换行度量**，
// 改一处版式要同时改两边，且已经出现漂移（目录页脚写「第 N 页」、
// 清单续页写紧凑页眉，字号常量各定义一份）。
//
// 本文件把「与版面方向无关」的那部分收敛成一份：
//   · createPdfSurface —— 字体加载 + 文档创建 + 页面游标 + 分页 + 页脚；
//   · drawWrappedCell  —— 单元格多行文本绘制（含垂直居中与不拆数字串）；
//   · fitHeaderFontSize—— 表头字号按可用宽度逐档收敛。
//
// **对外 API 完全不变**：`generateCatalogPdf` / `generateStatementPdf` /
// `loadCatalogFont` / `columnWidths` 等签名一个没动，9 个页面的调用点零修改。
// 两套引擎各自的**版面常量与版式差异**（横向 vs 纵向、有无概览卡片、
// 页脚文案不同）全部留在各自函数里 —— 收敛的是机制，不是版式。
import { PDFDocument, PDFFont, PDFPage, rgb, type RGB } from 'pdf-lib'
import fontkit from '@pdf-lib/fontkit'
import { wrapNoBreakNumbers } from './catalogLayout'

/** 字体缓存：整站只下载一次（并发调用共享同一 Promise）。 */
let fontPromise: Promise<ArrayBuffer> | null = null

/** 加载内嵌中文字体（霞鹜文楷子集，OFL）。失败时清缓存以便重试。 */
export function loadSharedFont(url = '/fonts/LXGWWenKai-Regular.subset.ttf'): Promise<ArrayBuffer> {
  if (!fontPromise) {
    fontPromise = fetch(url)
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

/** 页面尺寸与页边距（两套引擎共用同一份纸张描述）。 */
export interface PageGeometry {
  width: number
  height: number
  margin: number
}

/**
 * 分页游标：统一「还能画多少 → 不够就翻页」的判定与页眉页脚钩子。
 *
 * 两套引擎的翻页逻辑此前各写一遍，差异只在「续页画什么」——
 * 目录续页重画表头行，清单续页画紧凑页眉 + 分隔线。这里把差异做成回调，
 * 机制（阈值判定、页码自增、游标重置）只留一份。
 */
export interface PdfSurface {
  doc: PDFDocument
  font: PDFFont
  geom: PageGeometry
  /** 当前页（翻页后自动更新）。 */
  page: PDFPage
  /** 当前基线 y（翻页后自动重置到页顶）。 */
  y: number
  /** 当前逻辑页码（1 基，翻页时自增）。 */
  pageNo: number
  /** 可用内容宽度 = 纸宽 − 左右页边距。 */
  avail: number
  /** 翻到下一页（会调用 onNewPage 钩子做续页页眉）。 */
  newPage: () => void
  /** 剩余空间不足以放下 need 高度时自动翻页。 */
  ensure: (need: number) => void
  /** 在页面底部画页脚（翻页前调用，避免每页页脚互相覆盖）。 */
  drawFooter: () => void
}

/** createPdfSurface 的参数。 */
export interface PdfSurfaceOptions {
  geom: PageGeometry
  /** 首屏 y 起点（默认 纸高 − 页边距）。 */
  startY?: number
  /** 续页时画什么（目录重画表头、清单画紧凑页眉）。 */
  onNewPage?: (surface: PdfSurface) => void
  /** 页脚文案（每页底部）。 */
  footerText?: (pageNo: number) => string
  /** 底部安全边距（低于它就该翻页，默认 22）。 */
  bottomReserve?: number
  /** 页脚字号与颜色（默认 8pt / 灰）。 */
  footerSize?: number
  footerColor?: RGB
  /**
   * 已建好的文档（仅供测试注入）。
   *
   * 生产路径不要传：正常流程由工厂自己 PDFDocument.create()。
   * 加这个口子是因为「翻页后游标/页引用是否同步」这条契约最容易回归，
   * 而它不依赖真实字体 —— 没有注入点就只能靠真机肉眼验证。
   */
  doc?: PDFDocument
  /** 已嵌入的字体（仅供测试注入，与 doc 配套使用）。 */
  font?: PDFFont
}

/**
 * 创建绘制面板：加载字体、创建文档、初始化首页游标。
 *
 * 注意 `ensure` 在翻页时会**先画页脚再翻页** —— 旧实现把页脚画在翻页时，
 * 导致「最后一页没页脚」（循环结束后不会再触发 ensure）。这里保持同一顺序，
 * 由调用方在收尾时补一次 drawFooter。
 */
export async function createPdfSurface(opts: PdfSurfaceOptions): Promise<PdfSurface> {
  // doc/font 只在测试里注入；生产路径走「加载字体 → 建文档 → 嵌入字体」。
  let doc: PDFDocument
  let font: PDFFont
  if (opts.doc && opts.font) {
    doc = opts.doc
    font = opts.font
  } else {
    const fontBytes = await loadSharedFont()
    doc = await PDFDocument.create()
    doc.registerFontkit(fontkit)
    font = await doc.embedFont(fontBytes)
  }
  const { geom } = opts
  const avail = geom.width - geom.margin * 2
  const bottomReserve = opts.bottomReserve ?? 22
  const footerSize = opts.footerSize ?? 8
  const footerColor = opts.footerColor ?? rgb(0.45, 0.45, 0.45)

  let page = doc.addPage([geom.width, geom.height])
  let pageNo = 1
  let y = opts.startY ?? geom.height - geom.margin

  const surface: PdfSurface = {
    doc,
    font,
    geom,
    page,
    y,
    pageNo,
    avail,
    newPage: () => {},
    ensure: () => {},
    drawFooter: () => {},
  }

  // y 是原始数值，通过 getter/setter 与 surface 同步（对象属性无法被闭包捕获变更）
  const sync = () => { surface.page = page; surface.y = y; surface.pageNo = pageNo }

  surface.drawFooter = () => {
    if (!opts.footerText) return
    page.drawText(opts.footerText(pageNo), {
      x: geom.margin, y: geom.margin - 8, size: footerSize, color: footerColor, font,
    })
  }

  surface.newPage = () => {
    page = doc.addPage([geom.width, geom.height])
    pageNo++
    y = geom.height - geom.margin
    sync()
    opts.onNewPage?.(surface)
  }

  surface.ensure = (need: number) => {
    if (y - need < geom.margin + bottomReserve) {
      surface.drawFooter()
      surface.newPage()
    }
  }

  sync()
  return surface
}

/**
 * 间距归一：把「页面游标 y」写成表面对象，供后续绘制读取。
 *
 * 每段绘制后调用一次，避免调用方忘了同步（历史两套引擎都出现过
 * 「画完忘了写回 y → 下一段叠字」）。
 */
export function syncCursor(surface: PdfSurface, y: number): void {
  surface.y = y
}

/**
 * 表头字号收敛：按可用宽度逐档缩小（0.5pt 步进），返回字号与折行结果。
 *
 * 抽出来的理由：两套引擎都要「长表头必须完整显示且不压到邻列」，
 * 而各自的收敛循环写得一模一样（目录用 headerSize，清单用 S_HEADER_SIZE）。
 */
export function fitHeaderFontSize(
  label: string,
  availW: number,
  startSize: number,
  measure: (text: string, size: number) => number,
): { size: number; lines: string[] } {
  let size = startSize
  while (size > 6 && measure(label, size) > availW) size -= 0.5
  const lines = wrapNoBreakNumbers((s) => measure(s, size), label, availW)
  return { size, lines }
}

export interface WrappedCellOptions {
  lines: string[]
  /** 单元格左边界 x。 */
  x: number
  /** 单元格顶边 y。 */
  top: number
  /** 单元格高。 */
  height: number
  /** 字号。 */
  size: number
  lineHeight: number
  padX: number
  font: PDFFont
  color: RGB
  align?: 'left' | 'right'
  /** 单元格宽度（align=right 时用于右对齐计算）。 */
  width?: number
  /**
   * 基线微调（pt）。默认 0。
   *
   * 清单引擎历史上手工加了 2.5pt：它的行高由 `lines * S_LINE_H + 8` 决定，
   * 上下留白不对称（下多上少），按纯数学居中会把文字整体偏上一点点。
   * 目录引擎用的是 `maxLines * lineHeight + padX * 2`（上下对称），无需补偿。
   * 收敛时把这个差异显式化，避免「统一公式」顺手把清单的版式改掉。
   */
  baselineShift?: number
}

/**
 * 单元格多行文本绘制：垂直居中 + 可选右对齐。
 *
 * 两套引擎的单元格绘制此前各写一遍（目录用 startY + totalH 表达式，
 * 清单用另一套），对齐算法只要有一处算错就会出现「数字列贴左边」这类
 * 一眼可见的版式事故。收敛成一份后由单测锁死居中公式。
 */
export function drawWrappedCell(page: PDFPage, opts: WrappedCellOptions): void {
  const {
    lines, x, top, height, size, lineHeight, padX, font, color, align = 'left', baselineShift = 0,
  } = opts
  if (!lines.length) return
  const totalH = lines.length * lineHeight
  const startY = top - height + (height - totalH) / 2
  lines.forEach((line, li) => {
    const baseline = startY + totalH - (li + 1) * lineHeight + baselineShift
    let drawX = x + padX
    if (align === 'right' && opts.width) {
      drawX = x + opts.width - padX - font.widthOfTextAtSize(line, size)
    }
    page.drawText(line, { x: drawX, y: baseline, size, font, color })
  })
}
