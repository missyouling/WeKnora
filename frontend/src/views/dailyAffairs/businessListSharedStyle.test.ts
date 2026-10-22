import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

/**
 * 四模块（发票 / 合同 / 制度 / 通知公告）像素级一致性契约。
 *
 * 背景（2026-10-22）：四个页面的列表骨架本是各自拷贝，长期漂移后同一处样式
 * 出现四份不同值（border-radius 9px vs var(--td-radius-large)、overflow hidden
 * vs auto、底栏 sticky vs static、padding 10px 16px vs 0 2px），工具栏也被
 * 各自包裹导致右组按钮贴不到容器右缘。制度/公告还额外在标题栏放了一个
 * 「上传」按钮，与工具栏的主按钮重复，页面上出现 2 个上传入口。
 *
 * 本文件锁定三条铁律：
 *   ① 列表骨架 / 工具栏 / 表格 / 底栏样式只有一份来源（business-list-shared.css），
 *      四个页面必须都引用它；
 *   ② 每个页面的工具栏只能有一个上传入口（标题栏不得再放上传/设置/字段）；
 *   ③ 工具栏单行不折行，右组按钮 margin-left:auto 贴齐容器右缘。
 */

const PAGE_FILES = [
  'InvoiceManagement.vue',
  'ContractManagement.vue',
  'RegulationManagement.vue',
  'AwardPunishManagement.vue',
]

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

test('四个页面共用同一份列表样式文件', () => {
  for (const f of PAGE_FILES) {
    const src = read(f)
    assert.match(
      src,
      /@import '\.\/styles\/business-list-shared\.css';/,
      `${f} 必须引用四模块共享列表样式`,
    )
    // 共享样式必须挂在 .biz-page 下，避免 .doc-list-scroll 等通用类泄漏到车队/能耗页面
    assert.match(src, /class="[^"]*\bbiz-page\b/, `${f} 根容器必须带 .biz-page`)
    assert.match(src, /class="[^"]*\bbiz-main\b/, `${f} 主区必须带 .biz-main`)
    assert.match(src, /class="[^"]*\bbiz-list-view\b/, `${f} 列表视图必须带 .biz-list-view`)
  }
})

test('共享样式文件覆盖列表骨架 / 工具栏 / 表格 / 底栏', () => {
  const css = read('styles/business-list-shared.css')
  for (const sel of [
    '.biz-page .doc-list-scroll',
    '.biz-page .doc-list-view',
    '.biz-page .doc-summary-bar',
    '.biz-page .doc-batch-bar',
    '.biz-page .doc-toolbar',
    '.biz-page .doc-list-view .t-table__cell',
    '.biz-page .row-status-tag',
    '.biz-page .row-tag-chips',
    '.biz-page .doc-list-check .t-checkbox__label',
  ]) {
    assert.ok(css.includes(sel), `共享样式缺少规则：${sel}`)
  }
  // 全局样式里不能出现 :deep()（非 scoped 块里它不会被编译，规则会整条失效）
  assert.equal(css.includes(':deep('), false, '共享样式不得使用 :deep()')
  // 列表容器必须禁止自身滚动，滚动交给 t-table 内部（否则表头会跟着滚走）
  const scroll = css.slice(css.indexOf('.biz-page .doc-list-scroll {'), css.indexOf('.biz-page .doc-list-view {'))
  assert.match(scroll, /overflow: hidden;/)
  assert.match(scroll, /border-radius: var\(--td-radius-large\);/)
  // 底栏必须是 sticky 贴列表底部
  const summary = css.slice(css.indexOf('.biz-page .doc-summary-bar {'), css.indexOf('.biz-page .doc-batch-bar {'))
  assert.match(summary, /position: sticky;/)
  assert.match(summary, /padding: 10px 16px;/)
})

test('工具栏：单行不折行 + 右组贴右 + 左组控件 32px 等高', () => {
  const css = read('styles/business-list-shared.css')
  const toolbar = css.slice(css.indexOf('.biz-page .doc-toolbar {'), css.indexOf('.biz-page .doc-toolbar__leading'))
  assert.match(toolbar, /flex-wrap: nowrap;/)
  assert.match(toolbar, /justify-content: space-between;/)
  const trailing = css.slice(css.indexOf('.biz-page .doc-toolbar__trailing'), css.indexOf('.biz-page .doc-toolbar__leading)'))
  assert.match(trailing, /margin-left: auto;/)
  assert.match(trailing, /flex-shrink: 0;/)
  assert.match(
    css,
    /\.biz-page \.doc-toolbar__leading > \* \{[^}]*height: 32px;/,
    '左组控件必须统一 32px 高，才能与右组按钮同一水平中心线',
  )
})

test('上传按钮唯一：标题栏不得再放上传/设置/字段入口', () => {
  for (const f of PAGE_FILES) {
    const src = read(f)
    // 截取 <div class="header"> ... </div> 区块（标题栏）
    const start = src.indexOf('<div class="header">')
    assert.ok(start > 0, `${f} 缺少 header 区块`)
    const header = src.slice(start, src.indexOf('</div>\n    </div>', start) + 10)
    assert.doesNotMatch(header, /上传/, `${f} 标题栏不得出现上传按钮（会与工具栏主按钮重复）`)
    assert.doesNotMatch(header, /t-button/, `${f} 标题栏不应有任何操作按钮`)
  }
  // 页面模板里的 primary-action 只有一处（工具栏）
  for (const f of PAGE_FILES) {
    const src = read(f)
    const hits = src.match(/primary-action=/g) || []
    assert.equal(hits.length, 1, `${f} 只能有一处 primary-action（上传入口）`)
  }
})

test('四个页面列表表格参数一致（sticky-header + 虚拟滚动 48px）', () => {
  for (const f of PAGE_FILES) {
    const src = read(f)
    assert.match(src, /class="doc-table"/, `${f} 表格必须使用 .doc-table`)
    assert.match(src, /sticky-header/, `${f} 表头必须 sticky`)
    assert.match(
      src,
      /:scroll="\{ type: 'virtual', rowHeight: 48, isFixedRowHeight: true, bufferSize: 20 \}"/,
      `${f} 虚拟滚动行高必须与基准页一致（48px）`,
    )
  }
})

test('底栏位于列表边框容器内部（与基准页同层级）', () => {
  for (const f of PAGE_FILES) {
    const src = read(f)
    const docScroll = src.indexOf('class="doc-list-scroll"')
    const summary = src.indexOf('class="doc-summary-bar"')
    const viewEnd = src.indexOf('class="doc-batch-bar"')
    assert.ok(docScroll > 0 && summary > docScroll, `${f} 底栏必须写在 .doc-list-scroll 之内`)
    assert.ok(viewEnd === -1 || viewEnd > summary, `${f} 底栏必须早于批量浮条`)
  }
})

test('三模块不再各自定义列表骨架样式（避免再次漂移）', () => {
  for (const f of PAGE_FILES) {
    const src = read(f)
    // 页面 scoped 样式里不应再出现这些选择器（统一由共享样式提供）
    for (const sel of ['.doc-list-scroll {', '.doc-summary-bar {', '.doc-batch-bar {']) {
      assert.equal(
        src.includes(sel),
        false,
        `${f} 仍自定义 ${sel}，会与共享样式漂移`,
      )
    }
  }
})
