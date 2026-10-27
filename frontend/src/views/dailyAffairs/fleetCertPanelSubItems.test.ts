import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { fromSubsPayload, toSubsPayload } from './fleetCategorySubs'

/**
 * 「费用组成 / 分时时段」细项配置入口的回归锁定。
 *
 * 背景：`<分组>-rows` 是费用子表的**行标题**集合（零售交易电费、偏差电费、
 * 绿电交易电费(省间)…），`<分组>-periods` 是分时时段（尖/峰/平/谷）。它们不是
 * 主表列，所以不出现在分组胶囊里，而是挂在父字段（费用组成 / 分时时段）下展开。
 *
 * 这组测试锁五条：
 *   ① 细项以 subs 条目存在（group = `<分组>-rows` / `<分组>-periods`）且往返无损；
 *   ② 细项改名只改 label —— name 是 fee_items 的行匹配键，改了就对不上账单；
 *   ③ 展开入口是「点击字段行 + 轻量徽标」，不再是挤占输入框的绿底实心按钮；
 *   ④ 细项行只有 1 个开关（启用），没有「默认表头」——它是行不是列；
 *   ⑤ 细项行是单行 5 列（拖拽手柄/序号/名称/启用/删除），靠拖拽排序。
 */

const src = readFileSync(new URL('./FleetCertPanel.vue', import.meta.url), 'utf8')

test('费用组成/分时时段 存在细项映射与增删能力', () => {
  assert.match(src, /function childGroupOf/, '需要有「字段 → 细项分组」的映射函数')
  // 费用组成 → <分组>-rows；分时时段 → <分组>-periods
  assert.match(src, /n === 'name'\) return `\$\{activeSubGroup\.value\}-rows`/, '费用组成应映射到 -rows 分组')
  assert.match(src, /n === 'period'\) return `\$\{activeSubGroup\.value\}-periods`/, '分时时段应映射到 -periods 分组')
  assert.match(src, /function addChild/, '细项必须可新增')
  assert.match(src, /function removeChild/, '细项必须可删除')
  assert.match(src, /function onChildDragStart/, '细项必须可拖拽排序')
})

test('细项改名只改 label，不动 name（行匹配键）', () => {
  const fn = /function renameChild\(ch: any, v: string\) \{[\s\S]*?\n\}/.exec(src)
  assert.ok(fn, '缺少 renameChild')
  assert.match(fn![0], /ch\.label = v/, '改名必须写 label')
  assert.doesNotMatch(fn![0], /ch\.name\s*=/, '改名绝不能写 name —— 那会让 fee_items 匹配不到行')
})

test('细项分组不进分组胶囊（避免 5 条列字段被 22 条行项淹没）', () => {
  assert.match(src, /!g\.key\.endsWith\('-rows'\) && !g\.key\.endsWith\('-periods'\)/, '胶囊条需同时排除 -rows 与 -periods')
})

test('subs 往返：-rows / -periods 细项带 group 持久化', () => {
  const raw = [
    { name: 'name', label: '费用组成', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market', group_label: '市场化购电费' },
    { name: '偏差电费(尖峰)', label: '偏差电费(尖峰)', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market-rows' },
    { name: '尖', label: '尖', enabled: true, is_default: true, data_type: 'text', width: 0, group: 'market-periods' },
  ]
  const fields = fromSubsPayload(raw)
  assert.equal(fields.length, 3, '同名字段在不同分组下不能被去重掉')

  const back = toSubsPayload(fields)
  assert.equal(back[1]!.group, 'market-rows')
  assert.equal(back[2]!.group, 'market-periods')
  assert.equal(back[1]!.name, '偏差电费(尖峰)')
  assert.equal(back[2]!.name, '尖')
})

// ---- 细项 UI：点击展开 + 抽屉式 + 单开关 + 拖拽 ----

test('展开入口：点击字段行/徽标，不再有挤占输入框的实心按钮', () => {
  // 旧版在 160px 字段名列里塞了实心按钮，输入框被压到 ~80px（真机截图为证）
  assert.doesNotMatch(src, /class="fc-child-toggle"/, '不得再有用按钮承载的细项开关')
  assert.match(src, /@click\.stop="toggleChildGroup\(fd\)"/, '字段行/徽标点击应切换细项展开')
  assert.match(src, /class="fc-name-wrap" :class="\{ 'has-child': !!childGroupOf\(fd\) \}"/, '含细项的字段名区必须打 has-child 标记')
  assert.match(src, /\.fc-child-badge\s*\{/, '必须有轻量徽标样式')
})

test('细项行只保留 1 个开关（启用），不得有「默认表头」开关', () => {
  // 只在细项抽屉这一块里断言：主字段表头本来就有「默认表头」列（对的），
  // 但细项是表格行而不是表头列，「默认表头」对它没有意义。
  const start = src.indexOf('class="child-drawer"')
  assert.ok(start > 0, '缺少细项抽屉区块')
  const drawer = src.slice(start, src.indexOf('</template>', start))
  assert.doesNotMatch(drawer, /ch\.isDefault/, '细项行不得再出现 isDefault（默认表头）开关')
  assert.doesNotMatch(drawer, /默认表头/, '细项行不得再出现「默认表头」字样')
  assert.doesNotMatch(drawer, /默认显示/, '细项行不得再有「默认显示」提示')
  assert.match(drawer, /content="启用 \/ 停用该细项"/, '细项行必须保留启用开关')
  // 每行只允许一个 t-switch
  const rowStart = drawer.indexOf('class="field-config-row child-row"')
  const row = drawer.slice(rowStart, drawer.indexOf('class="field-config-add child-add"', rowStart))
  assert.equal((row.match(/<t-switch/g) || []).length, 1, '细项行只允许 1 个开关')
})

test('细项行是紧凑单行 5 列：拖拽手柄 / 序号 / 名称 / 启用 / 删除', () => {
  assert.match(src, /draggable="true"[^>]*@dragstart="onChildDragStart\(ch\)"/, '细项行必须支持 HTML5 拖拽')
  assert.match(src, /function onChildDragStart/, '必须有细项拖拽开始处理')
  assert.match(src, /function onChildDrop/, '必须有细项放置处理')
  assert.match(src, /\{\{ ci \+ 1 \}\}/, '细项行必须显示自增序号')
  // 删除必须二次确认（popconfirm 包住按钮 + @confirm 才真正删），不得一键直删
  assert.match(src, /@confirm="removeChild\(ch\)"/, '细项行必须有删除按钮（经二次确认触发）')
  assert.match(src, /t-popconfirm[\s\S]{0,600}?removeChild\(ch\)/, '细项删除必须包在 t-popconfirm 里')
  assert.match(src, /biz-del-btn/, '细项删除图标必须走统一删除按钮样式')
  // 操作改为单行按钮后，不再需要 ⋮ 下拉菜单（那是列宽塞不下时的退让）
  assert.doesNotMatch(src, /childMenuOptions/, '不应再有细项下拉菜单')
  assert.doesNotMatch(src, /onChildMenu/, '不应再有细项下拉菜单回调')
})

test('细项容器为内嵌抽屉式（左导引线 + 浅灰底 + 内部滚动）', () => {
  assert.match(src, /\.child-drawer\b/, '细项容器必须有独立的抽屉式类名')
  // 结束花括号前是两个空格的缩进（Less 嵌套在父层里），用 {2} 显式声明
  const css = /\.child-drawer\s*\{([\s\S]*?)\n {2}\}/.exec(src)
  assert.ok(css, '缺少 .child-drawer 样式定义')
  const body = css![1]
  assert.match(body, /border-left:\s*2px solid var\(--td-brand-color\)/, '左侧必须有品牌绿导引线')
  assert.match(body, /background:\s*#f8fafc/, '底色必须是浅灰')
  assert.match(body, /max-height:\s*280px/, '必须有最大高度限制')
  assert.match(body, /overflow-y:\s*auto/, '超出后内部独立滚动')
  // 细项行自身网格（5 列紧凑，不沿用主字段 7 列）
  assert.match(src, /\.field-config-row\.child-row/, '细项行必须有独立网格')
  assert.match(src, /grid-template-columns:\s*24px 28px minmax\(0, 1fr\) 56px 28px/, '细项行网格须为 5 列紧凑布局')
})

test('细项拖拽：按 name 定位，跨行移动后序号重算', () => {
  const fn = /function onChildDrop\(ch: any\) \{[\s\S]*?\n\}/.exec(src)
  assert.ok(fn, '缺少 onChildDrop')
  assert.match(fn![0], /findIndex/, '必须按标识定位而不是裸下标')
  assert.match(fn![0], /splice/, '必须实际移动数组元素')
})
