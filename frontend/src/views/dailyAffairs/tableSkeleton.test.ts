/**
 * 列表中台「合计行贴底 + 横向滚动条在最底部」的骨架守护（第 38 轮）。
 *
 * 这套骨架被反复踩过三次（第 31/34/37 轮），每次都是同一类错误：
 * 想让合计行贴底 → 给 `table` 设 `height: 100%` → 浏览器把多余高度**分给数据行**
 * → 唯一那行被拉到整屏高、内容垂直居中（真机：第 1 行悬在表格中间 250px）。
 *
 * 正确做法（本文件锁死）：
 *   `.t-table` / `.t-table__content` flex 列 → `table` 拉满 →
 *   `tbody::after` 弹性占位行吸收剩余高度 → 数据行保持 39~42px、tfoot 贴底、
 *   横向滚动条（属于 `.t-table__content`）天然落在 tfoot 下方。
 */
import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const css = readFileSync(new URL('./styles/business-list-shared.css', import.meta.url), 'utf8')
const pages = ['InvoiceManagement.vue', 'UtilitiesManagement.vue', 'TenantBilling.vue', 'FleetRecordList.vue']

const stripComments = (t: string) => t.replace(/\/\*[\s\S]*?\*\//g, '')

test('表格骨架：flex 列 + tbody 弹性占位行（合计行才能贴底且不拉伸数据行）', () => {
  const code = stripComments(css)
  assert.match(code, /\.doc-list-view \.t-table \{[^}]*display:\s*flex[^}]*flex-direction:\s*column/,
    '.t-table 必须是 flex 列，否则 content 拿不到剩余高度')
  assert.match(code, /\.t-table__content \{[^}]*flex:\s*1 1 auto[^}]*display:\s*flex/,
    '.t-table__content 必须撑满卡片剩余高度')
  assert.match(code, /\.t-table__content > table \{[^}]*flex:\s*1 1 auto/,
    'table 要作为 flex 项拉满 content')
  assert.match(code, /\.t-table__body::after \{[^}]*display:\s*table-row[^}]*height:\s*100%/,
    '必须用 tbody 末尾的弹性占位行吸收多余高度')
})

test('表格骨架：禁止给 table 设 height（会把多余高度分给数据行 → 首行悬空）', () => {
  const code = stripComments(css)
  // 只检查「表格本体」的高度声明；`.t-table`（外层 div）设 height 是允许的
  assert.ok(!/\.t-table__content > table \{[^}]*height:\s*100%/.test(code),
    'table{height:100%} 会把唯一那行撑到整屏高（真机 477px、内容垂直居中）')
  assert.ok(!/\.t-table__body > tr \{[^}]*height:/.test(code),
    '不得给数据行写死高度：textarea 明细行需要 260px')
})

test('空态/加载态只隐藏横向滚动条，不得把表格高度收成 auto', () => {
  const code = stripComments(css)
  const flat = code.slice(code.indexOf('.is-table-flat .t-table {'))
  const block = flat.slice(0, flat.indexOf('}'))
  assert.ok(!/flex:\s*0 0 auto/.test(block), '空态下表格仍要拉满，否则合计行回到半空')
  assert.ok(!/height:\s*auto/.test(block), '空态下不得把高度收成 auto')
})

test('四个列表页都挂 .doc-list-view（骨架样式的作用域锚点）', () => {
  for (const p of pages) {
    const src = readFileSync(new URL('./' + p, import.meta.url), 'utf8')
    assert.match(src, /:class="listViewClass"|class="doc-list-view"/, `${p} 缺少 .doc-list-view 锚点`)
  }
})
