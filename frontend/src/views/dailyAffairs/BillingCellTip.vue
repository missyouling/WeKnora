<template>
  <!--
    费用清单单元格「计算方式 / 数据来源」浮层。

    为什么单独成组件（而不是在 TenantBilling 里用一个 ref）：
    浮层状态若放在列表页自己的 setup 里，**每次 hover 改状态都会让整张 t-table
    重新渲染** —— 鼠标下的 DOM 节点被替换，随即触发一串 mouseleave/mouseover，
    浮层刚出现就被自己关掉。真机表现就是「同一列有时弹、有时不弹」，
    实测只有最后一列侥幸正常。
    抽成独立组件后，hover 只重渲染这一个浮层，表格完全不动。

    全表只有一个实例（不是每格一个 tooltip）：20+ 数值列 × N 行会有几百个
    Tooltip 组件互相抢占 hover，事件路径也不可控。
  -->
  <teleport to="body">
    <div v-if="state.visible" class="billing-cell-tip" :style="tipStyle">{{ state.text }}</div>
  </teleport>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

const WIDTH = 420
const OFFSET_X = 14
const OFFSET_Y = 18

const state = ref({ visible: false, text: '', x: 0, y: 0 })

/** 跟随鼠标，并做视口边界收敛（靠右时翻到光标左侧，靠下时翻到上方）。 */
const tipStyle = computed(() => {
  const vw = typeof window !== 'undefined' ? window.innerWidth : 1920
  const vh = typeof window !== 'undefined' ? window.innerHeight : 1080
  const { x, y } = state.value
  const left = x + OFFSET_X + WIDTH > vw ? Math.max(8, x - WIDTH - OFFSET_X) : x + OFFSET_X
  // 估算高度上限（6 行算式约 130px），超出就翻到光标上方
  const estH = 150
  const top = y + OFFSET_Y + estH > vh ? Math.max(8, y - estH - OFFSET_Y) : y + OFFSET_Y
  return { left: `${left}px`, top: `${top}px`, maxWidth: `${WIDTH}px` }
})

function onOver(e: MouseEvent) {
  const target = e.target as HTMLElement | null
  // 以**单元格**为单位解析说明，而不是以挂着 data-tip 的 span 为单位。
  //
  // 真机踩坑：TDesign 的 td 里包了一层 `.t-table__ellipsis` div，鼠标在
  // span 与其父 div 之间抖动（表格 patch、文本省略重排都会触发）时，
  // 事件目标从 span 换成 div，按 `closest('[data-tip]')` 会判成「移到无说明元素」
  // 而立刻收起 —— 表现就是浮层闪一下消失、同一列有时弹有时不弹。
  // 改成先找最近的 td，再在 td 内找 data-tip：同格内的元素切换不会改变结论。
  const cell = target?.closest?.('td') as HTMLElement | null
  const holder = cell
    ? (cell.matches('[data-tip]') ? cell : cell.querySelector('[data-tip]'))
    : (target?.closest?.('[data-tip]') as HTMLElement | null)
  const text = holder?.getAttribute('data-tip') || ''
  if (!text) {
    if (state.value.visible) state.value.visible = false
    return
  }
  state.value = { visible: true, text, x: e.clientX, y: e.clientY }
}

function onScroll() {
  if (state.value.visible) state.value.visible = false
}

onMounted(() => {
  document.addEventListener('mouseover', onOver, true)
  // 滚动（含横向滚动宽表）时浮层位置会失真，直接收起
  window.addEventListener('scroll', onScroll, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('mouseover', onOver, true)
  window.removeEventListener('scroll', onScroll, true)
})
</script>

<style lang="less">
/* 浮层 teleport 到 body，scoped 样式作用不到，必须全局。
   说明常带多行算式：保留换行 + 等宽数字，才能逐项核对代入值。 */
.billing-cell-tip {
  position: fixed;
  z-index: 5000;
  padding: 8px 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-2);
  white-space: pre-line;
  line-height: 1.6;
  font-size: var(--app-text-sm);
  color: var(--td-text-color-primary);
  font-variant-numeric: tabular-nums;
  // 浮层不接收指针事件，否则会挡住 hover 目标、自己把自己关掉
  pointer-events: none;
}
</style>
