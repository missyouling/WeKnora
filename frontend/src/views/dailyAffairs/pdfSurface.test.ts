// PDF 绘制共享核心的单测（第 24 轮任务一）。
//
// 这两个引擎（目录 / 清单）共用 createPdfSurface 之后，最容易回归的是
// **「翻页后游标与页引用是否同步」** —— 旧实现各自把 page / y 写成局部变量，
// 翻页后局部变量失效，后续绘制静默落在已完成的页面上（没有任何报错）。
// 这里用最小的假 PDFDocument 锁死这个契约，不依赖真实字体加载。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import { createPdfSurface, fitHeaderFontSize, loadSharedFont } from './pdfSurface'

interface FakePage {
  ops: string[]
  drawText: (s: string, o: any) => void
  drawLine: (o: any) => void
  drawRectangle: (o: any) => void
}

function fakeDoc() {
  const pages: FakePage[] = []
  return {
    pages,
    addPage: () => {
      const p: FakePage = {
        ops: [],
        drawText: (s: string) => p.ops.push(`text:${s}`),
        drawLine: () => p.ops.push('line'),
        drawRectangle: () => p.ops.push('rect'),
      }
      pages.push(p)
      return p
    },
    getPages: () => pages,
    save: async () => new Uint8Array(),
  }
}

/** 取假文档的页数组（真 PDFPage 上没有 ops，这里只读测试注入的假页）。 */
function fakePages(doc: any): FakePage[] {
  return (doc.getPages() as any) as FakePage[]
}

function fakeFont() {
  return {
    widthOfTextAtSize: (s: string, size: number) => s.length * size * 0.55,
  }
}

/** 直接构造面板（绕过真实字体 fetch）。 */
async function makeSurface(over: Record<string, any> = {}) {
  const doc = fakeDoc()
  return createPdfSurface({
    geom: { width: 595, height: 842, margin: 40 },
    doc, font: fakeFont(), ...over,
  } as any)
}

test('createPdfSurface：首页 y 落在页顶，avail = 纸宽减双页边距', async () => {
  // 这里必须走真实工厂路径才能验证 avail 计算，所以只断言「不依赖字体」的部分：
  // avail 由 geom 推导，属于纯算术。
  const s = await makeSurface()
  assert.equal(s.avail, 595 - 40 * 2)
})

test('翻页后 surface.page 指向新页（不能是快照）', async () => {
  const s = await makeSurface({ startY: 100 })
  const first = s.page
  s.newPage()
  assert.notEqual(s.page, first, '翻页后 page 必须换成新页对象')
  assert.equal(s.pageNo, 2)
  assert.equal(s.y, 842 - 40, '翻页后游标必须回到页顶')
})

test('ensure：空间不足才翻页，空间足够不翻页', async () => {
  const s = await makeSurface({ startY: 100 })
  s.ensure(10) // 100 - 10 = 90 > 40 + 22，不翻
  assert.equal(s.pageNo, 1)
  s.ensure(100) // 100 - 100 = 0 < 62，翻页
  assert.equal(s.pageNo, 2)
})

test('ensure 翻页前先画页脚（否则上一页没有页脚）', async () => {
  const s = await makeSurface({ startY: 80, footerText: (n: number) => `p${n}` })
  const before = fakePages(s.doc)[0].ops.length
  s.ensure(100)
  // 首页被翻走前，页脚必须已经画上去
  assert.ok(fakePages(s.doc)[0].ops.length > before, '上一页必须已经画过页脚')
})

test('drawFooter 可重复调用且写到当前页', async () => {
  const s = await makeSurface({ startY: 100, footerText: (n: number) => `p${n}` })
  s.drawFooter()
  assert.ok(fakePages(s.doc)[0].ops.some((o: string) => o === 'text:p1'))
  s.newPage()
  s.drawFooter()
  assert.ok(fakePages(s.doc)[1].ops.some((o: string) => o === 'text:p2'), '新页页脚页码必须是 2')
})

test('onNewPage 钩子在翻页后执行，能改写续页游标', async () => {
  const seen: number[] = []
  const s = await makeSurface({
    startY: 100,
    onNewPage: (sf: any) => {
      seen.push(sf.pageNo)
      sf.y = 842 - 40 - 50 // 模拟「续页要留 50pt 页眉」
    },
  })
  s.newPage()
  assert.deepEqual(seen, [2])
  assert.equal(s.y, 842 - 40 - 50)
})

test('fitHeaderFontSize：超宽表头逐档缩小且不丢字', async () => {
  const measure = (t: string, sz: number) => t.length * sz * 0.55
  // 10 个字 × 10pt × 0.55 = 55pt < 60pt 放得下；要真正超宽必须把宽度压到 30pt
  const r = fitHeaderFontSize('非常非常长的表头名称', 30, 10, measure)
  assert.ok(r.size < 10, '放不下时必须缩字号')
  assert.equal(r.lines.join(''), '非常非常长的表头名称', '缩字号后不得丢字')
  // 也不能无限缩：下限 6pt，否则表头小到无法阅读
  const tiny = fitHeaderFontSize('非常非常长的表头名称', 5, 10, measure)
  assert.ok(tiny.size >= 6, '字号下限是 6pt')
})

test('fitHeaderFontSize：放得下时保持原字号', async () => {
  const measure = (t: string, sz: number) => t.length * sz * 0.55
  const r = fitHeaderFontSize('短', 200, 10, measure)
  assert.equal(r.size, 10)
  assert.deepEqual(r.lines, ['短'])
})

test('loadSharedFont 缓存同一 Promise（并发不重复下载）', async () => {
  // 只验证「两次调用拿到同一个 Promise 实例」这一缓存契约，
  // 不去真的发请求（未 await，失败也不影响断言）。
  const a = loadSharedFont()
  const b = loadSharedFont()
  assert.equal(a, b, '字体必须整站只下载一次')
  a.catch(() => {})
})
