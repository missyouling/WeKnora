/**
 * 第 66 轮：把 TDesign 的 `tfoot`（合计行）搬出纵向滚动容器。
 *
 * 问题：`.t-table__content` 是 `overflow-y: auto` 的滚动容器，而 thead / tbody /
 * tfoot 全在它内部 —— 垂直滚动条从左到右贯穿整块内容，视觉上直接压在
 * 「合计（共 N 条）」那一行右侧，真机上像一条把合计行切开的黑线。
 *
 * 为什么不能只用 CSS：滚动条画在滚动容器的 padding box 右缘，只要 tfoot 还在
 * 容器内，它就一定处在滚动条的覆盖范围内。唯一解法是把 tfoot 移到容器外。
 *
 * 做法：
 *   1. 在 `.doc-list-view`（position: relative）末尾建一个 `.biz-foot-host`；
 *   2. 把**真实的** tfoot 节点移进去，并克隆同一份 `<colgroup>`（列宽同源，
 *      三段列线仍像素级对齐）；
 *   3. 给 `.t-table__content` 加 `margin-bottom = 合计行高度`，让纵向滚动条
 *      止于合计行上沿；
 *   4. 横向滚动时同步 `scrollLeft`（表头/数据体/合计行一起平移）。
 *
 * 为什么移动真节点而不是克隆：Vue 的 vdom 持有节点引用，移动父节点后 diff
 * 仍能就地 patch 文本与属性 —— 合计行的数字随勾选实时变化，不会僵死。
 * 若 TDesign 因列变化重建了 tfoot（旧节点被移除），观察器会把新节点再搬一次。
 */

const HOST_CLS = 'biz-foot-host'
/** 原件隐藏标记：合计行已由 host 里的克隆体呈现 */
const HIDDEN_CLS = 'biz-foot-hidden'
const TABLE_CLS = 'biz-foot-table'

interface HoistState {
  view: HTMLElement
  content: HTMLElement
  host: HTMLElement
  hostTable: HTMLTableElement
  /** 最近一次同步的 scrollLeft，避免无意义写入 */
  lastLeft: number
  /** 最近一次克隆的内容签名（tfoot innerHTML + colgroup） */
  lastSig: string
  ro: ResizeObserver | null
  mo: MutationObserver | null
  onScroll: () => void
}

const states = new WeakMap<HTMLElement, HoistState>()

function ensureHost(view: HTMLElement): { host: HTMLElement; hostTable: HTMLTableElement } {
  let host = view.querySelector<HTMLElement>(`:scope > .${HOST_CLS}`)
  if (!host) {
    host = document.createElement('div')
    host.className = HOST_CLS
    view.appendChild(host)
  }
  let hostTable = host.querySelector<HTMLTableElement>(`:scope > table.${TABLE_CLS}`)
  if (!hostTable) {
    hostTable = document.createElement('table')
    hostTable.className = TABLE_CLS
    host.appendChild(hostTable)
  }
  return { host, hostTable }
}

/**
 * 单次同步：把 `content` 里的 tfoot **克隆**进 host，并把原件隐藏。
 *
 * 为什么克隆而不是移动真节点：移动后 Vue 的 diff 会认为该节点已不在原位，
 * 下一次数据刷新直接把它移除 —— 真机上表现为「合计行整体消失」。
 * 原件留在表格里继续被 Vue patch（只是 display:none），克隆体每次按内容签名
 * 增量刷新，视觉与交互都不受影响。
 */
function hoistOnce(state: HoistState): void {
  const { view, content, hostTable } = state
  const table = content.querySelector<HTMLTableElement>('table')
  if (!table) return
  const tfoot = table.querySelector<HTMLTableElement>('tfoot')
  const colgroup = table.querySelector<HTMLTableElement>('colgroup')

  // 没有合计行（加载中 / 空表 / 该页无汇总）：清掉高度补偿，让表格吃满
  if (!tfoot) {
    view.style.setProperty('--biz-foot-h', '0px')
    if (hostTable.childElementCount) hostTable.replaceChildren()
    state.lastSig = ''
    return
  }
  // 内联 display:none —— TDesign 的 `.t-table__footer--fixed` 自带 sticky 定位，
  // 只靠类名可能被更高优先级的主题规则覆盖，真机出现过「两份合计行」。
  tfoot.classList.add(HIDDEN_CLS)
  if (tfoot.style.display !== 'none') tfoot.style.display = 'none'

  // 内容签名：只有合计行真的变了才重建克隆（避免自触发死循环）
  const sig = `${tfoot.innerHTML}|${colgroup?.innerHTML || ''}`
  if (sig !== state.lastSig) {
    state.lastSig = sig
    const frag = document.createDocumentFragment()
    if (colgroup) frag.appendChild(colgroup.cloneNode(true))
    const clone = tfoot.cloneNode(true) as HTMLTableElement
    clone.classList.remove(HIDDEN_CLS)
    clone.removeAttribute('style')
    frag.appendChild(clone)
    hostTable.replaceChildren(frag)
  }

  // 高度补偿：滚动条止于合计行上沿。
  // 注意**必须量克隆体**：原件已被 display:none，量它恒为 0（footer 会消失）。
  const cloneFoot = hostTable.querySelector('tfoot')
  const h = Math.max(0, Math.round(cloneFoot?.getBoundingClientRect().height || 0))
  if (view.style.getPropertyValue('--biz-foot-h') !== `${h}px`) {
    view.style.setProperty('--biz-foot-h', `${h}px`)
  }
  // 与主表同宽（百分比列宽需要同一分母）
  const w = Math.round(table.getBoundingClientRect().width)
  if (w > 0 && hostTable.style.width !== `${w}px`) hostTable.style.width = `${w}px`
  syncLeft(state)
}

/** 横向滚动同步：数据体横向滚动时，合计行跟着平移。 */
function syncLeft(state: HoistState): void {
  const left = state.content.scrollLeft
  if (left === state.lastLeft) return
  state.lastLeft = left
  const inner = state.hostTable
  if (inner) inner.style.transform = left ? `translateX(${-left}px)` : ''
}

/**
 * 安装监听：页面挂载后调用一次（幂等）。
 *
 * `viewRef` 指向 `.doc-list-view`（注意：不是 `.doc-list-scroll`，后者是外框）。
 */
export function installListFootHoist(view: HTMLElement | null | undefined): void {
  if (!view || states.has(view)) return
  const content = view.querySelector<HTMLElement>('.t-table__content')
  if (!content) return
  const { host, hostTable } = ensureHost(view)

  const state: HoistState = {
    view, content, host, hostTable, lastLeft: -1, lastSig: '',
    ro: null, mo: null, onScroll: () => syncLeft(state),
  }
  states.set(view, state)

  hoistOnce(state)

  content.addEventListener('scroll', state.onScroll, { passive: true })

  if (typeof ResizeObserver !== 'undefined') {
    state.ro = new ResizeObserver(() => hoistOnce(state))
    state.ro.observe(content)
    state.ro.observe(view)
  }
  if (typeof MutationObserver !== 'undefined') {
    // tbody / tfoot 被 TDesign 重建时（列变化、数据刷新）重新搬一次
    state.mo = new MutationObserver(() => hoistOnce(state))
    state.mo.observe(content, { childList: true, subtree: true })
  }
  // 首次布局尚未稳定（字体/列宽异步计算），下一帧再校一次
  requestAnimationFrame(() => hoistOnce(state))
}

/** 卸载监听（页面卸载时调用）。 */
export function disposeListFootHoist(view: HTMLElement | null | undefined): void {
  if (!view) return
  const state = states.get(view)
  if (!state) return
  state.content.removeEventListener('scroll', state.onScroll)
  state.ro?.disconnect()
  state.mo?.disconnect()
  state.hostTable.replaceChildren()
  state.view.style.setProperty('--biz-foot-h', '0px')
  states.delete(view)
}

/**
 * 全站安装：扫描当前文档里所有 `.doc-list-view` 并逐个接管。
 *
 * 各业务页只需在 onMounted 调一次 —— 页面切换时 Vue 会重建列表节点，
 * 这里用一个全局 MutationObserver 把新出现的节点也接上，避免每个页面
 * 各自维护 ref + 生命周期。
 */
/**
 * 注入结构样式（幂等）。
 *
 * 为什么不写进 `business-list-shared.css`：该文件由各页在 `<style>` 块里
 * `@import`，而 Vite 开发态对 SFC 内的 CSS `@import` 不会内联 —— 开发环境
 * 里那一整套 `.biz-page .doc-list-scroll` 规则**根本没有生效**（实测
 * computedStyle 显示 `flex: 0 1 auto / overflow: visible`），列表因此被
 * 外层 `overflow: hidden` 裁掉且无法滚动。这里用一次 style 注入保证
 * 开发 / 生产两种构建下结构一致，且不与既有规则冲突（数值同源）。
 */
const STYLE_ID = 'biz-list-foot-hoist-style'
const STRUCT_CSS = `
.biz-page .doc-list-scroll {
  flex: 1 1 auto; min-height: 0; min-width: 0;
  overflow: hidden; display: flex; flex-direction: column;
}
.biz-page .doc-list-view {
  position: relative; flex: 1; min-height: 0;
  display: flex; flex-direction: column;
  --biz-foot-h: 0px;
}
.biz-page .doc-list-view .t-table {
  flex: 1 1 auto; min-height: 0; height: 100%; max-height: 100%;
  display: flex; flex-direction: column;
}
.biz-page .doc-list-view .t-table__content {
  flex: 1 1 auto; min-height: 0; overflow-y: auto;
  margin-bottom: var(--biz-foot-h);
}
.biz-page .doc-list-view .t-table__content > table { flex: 1 1 auto; }
.biz-page .doc-list-view .t-table__content tfoot.biz-foot-hidden { display: none !important; }
.biz-page .doc-list-view .biz-foot-host {
  position: absolute; left: 0; right: 0; bottom: 0;
  height: var(--biz-foot-h); overflow: hidden; z-index: 2;
  background: var(--td-bg-color-container);
}
.biz-page .doc-list-view .biz-foot-host > table.biz-foot-table {
  border-collapse: collapse; table-layout: fixed; min-width: 100%;
  font-size: var(--td-font-size-body-small); will-change: transform;
}
.biz-page .doc-list-view .biz-foot-host > table.biz-foot-table td {
  box-sizing: border-box; padding: 0 8px; height: 40px;
  border-top: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-primary); font-weight: 600;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
`

function injectStyle(): void {
  if (typeof document === 'undefined' || document.getElementById(STYLE_ID)) return
  const el = document.createElement('style')
  el.id = STYLE_ID
  el.textContent = STRUCT_CSS
  document.head.appendChild(el)
}

let globalObserver: MutationObserver | null = null
const watched = new WeakSet<HTMLElement>()

function sweep(): void {
  if (typeof document === 'undefined') return
  for (const el of Array.from(document.querySelectorAll<HTMLElement>('.doc-list-view'))) {
    if (watched.has(el)) continue
    // 只接管带 t-table 的中台列表（弹窗里的三段表不在此列）
    if (!el.querySelector('.t-table__content')) continue
    watched.add(el)
    installListFootHoist(el)
  }
}

export function installListFootHoistAll(): void {
  if (typeof document === 'undefined') return
  injectStyle()
  sweep()
  if (globalObserver || typeof MutationObserver === 'undefined') return
  globalObserver = new MutationObserver(() => sweep())
  globalObserver.observe(document.body, { childList: true, subtree: true })
  // 字体/列宽异步就绪后补一次
  requestAnimationFrame(() => sweep())
  setTimeout(() => sweep(), 600)
}
