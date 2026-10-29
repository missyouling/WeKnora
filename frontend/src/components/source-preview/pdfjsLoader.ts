// Bundled as a worker chunk with a .js name, so any static server hands it
// out with a JavaScript MIME type (module workers refuse anything else).
import pdfWorkerUrl from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?worker&url'

export type PdfJs = typeof import('pdfjs-dist/legacy/build/pdf.mjs')

let loading: Promise<PdfJs> | null = null

/**
 * Load the library once, but let each document own its worker. Reusing a port
 * while the previous loading task is being destroyed can break rapid navigation.
 */
export function loadPdfJs(): Promise<PdfJs> {
  if (!loading) {
    loading = import('pdfjs-dist/legacy/build/pdf.mjs').then((lib) => {
      lib.GlobalWorkerOptions.workerSrc = pdfWorkerUrl
      return lib
    })
    loading.catch(() => {
      loading = null
    })
  }
  return loading
}

type PdfWorker = InstanceType<PdfJs['PDFWorker']>

let workerPromise: Promise<PdfWorker> | null = null

/**
 * 复用一个 PDFWorker（而不是每个文档新建一个）。
 *
 * 为什么能复用：把 worker 传给 `getDocument` 时，pdf.js **不会**把它记成
 * `task._worker`（只有它自己创建的那种才会），所以 `task.destroy()` 只销毁
 * 该文档的 transport，worker 线程本身仍在。这样第二次打开打印预览、
 * 或换一条记录再打印，都省掉 worker 启动（真机 300~500ms）。
 *
 * 为什么进程内只有一个：worker 起一个线程 + 一份 js 运行时，按文档起会让
 * 快速连点（预览 / 关闭 / 再预览）时堆一串线程，浏览器直接卡死。
 */
export function getPdfWorker(): Promise<PdfWorker> {
  if (!workerPromise) {
    workerPromise = loadPdfJs().then((lib) => new lib.PDFWorker())
    workerPromise.catch(() => {
      workerPromise = null
    })
  }
  return workerPromise
}

/** 预热：库 + worker 都提前就绪，点击「打印」时才不用等它们。 */
export function warmupPdfJs(): void {
  void loadPdfJs().catch(() => {})
  void getPdfWorker().catch(() => {})
}
