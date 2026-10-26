import { computed, onBeforeUnmount, onMounted, reactive, type CSSProperties } from 'vue'

/**
 * 表头右侧滚动条轨道遮罩的实测值（四模块 + 能耗统一使用）。
 *
 * 为什么需要实测而不是写死：
 *  - 滚动条宽度随平台/缩放变化（Windows 经典 17px、overlay 模式 0px、
 *    macOS 悬浮 0px），写死会在某些环境下多盖或漏盖表头内容；
 *  - 表头高度随客户端 size 变化（small/normal/large），写死会让遮罩
 *    与表头底边错开一条缝，数据行就从缝里透出来。
 *
 * 用法：
 *   const { headerFixStyle } = useTableHeaderFix()
 *   <div class="doc-list-view" :style="headerFixStyle">
 * 遮罩本体在 styles/business-list-shared.css 的 `.doc-list-view::after`。
 */
export function useTableHeaderFix(selector = '.doc-list-view thead th') {
  const headerFix = reactive({ sbw: 0, thh: 39 })

  const headerFixStyle = computed<CSSProperties>(() => ({
    '--invoice-sbw': `${headerFix.sbw}px`,
    '--invoice-thh': `${headerFix.thh}px`,
  }))

  const measureTableHeaderFix = () => {
    const th = document.querySelector(selector)
    if (th) headerFix.thh = Math.max(30, Math.round(th.getBoundingClientRect().height))

    // 滚动条宽度优先从「真实表格内容区」反推：offsetWidth - clientWidth 就是它被
    // 滚动条吃掉的那几像素。这比造临时 probe 可靠 —— 无头/某些缩放环境下 probe
    // 恒为 0，而内容区能拿到真实值（实测 15~17px）。
    const content = document.querySelector<HTMLElement>('.doc-list-view .t-table__content')
    if (content) {
      const w = content.offsetWidth - content.clientWidth
      if (w > 0) {
        headerFix.sbw = w
        return
      }
    }
    // 兜底：内容区还没渲染（或没有滚动条）时用临时容器探一次
    const probe = document.createElement('div')
    probe.style.cssText = 'width:100px;height:100px;overflow:scroll;position:absolute;top:-9999px;left:-9999px'
    document.body.appendChild(probe)
    const w = probe.offsetWidth - probe.clientWidth
    probe.remove()
    headerFix.sbw = w > 0 ? w : 0
  }

  onMounted(() => {
    measureTableHeaderFix()
    // 数据是异步来的：onMounted 时 .t-table__content 往往还没挂载，
    // 量不到滚动条宽度（表现为表头右侧遮罩缺失、滚动条压住最后一列表头）。
    // 这里做有界轮询，量到就停（最多约 9s），避免无限定时器。
    let tries = 0
    const timer = window.setInterval(() => {
      tries += 1
      measureTableHeaderFix()
      if (headerFix.sbw > 0 || tries >= 30) window.clearInterval(timer)
    }, 300)
    onBeforeUnmount(() => window.clearInterval(timer))
    // 窗口缩放会改变滚动条宽度（如 125% 缩放出现经典滚动条），需要重测
    window.addEventListener('resize', measureTableHeaderFix)
  })
  onBeforeUnmount(() => {
    window.removeEventListener('resize', measureTableHeaderFix)
  })

  return { headerFix, headerFixStyle, measureTableHeaderFix }
}
