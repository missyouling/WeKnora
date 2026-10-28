<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

withDefaults(defineProps<{
  columns: Array<{ key: string; label: string }>
  visibleKeys: string[]
  hideTrigger?: boolean
}>(), {
  hideTrigger: false,
})
const emit = defineEmits<{
  (e: 'update:visibleKeys', v: string[]): void
  (e: 'reset'): void
  (e: 'selectAll'): void
}>()

const popVisible = ref(false)
/** 弹层几何：x/y 定位 + maxH（可用高度，超出则在列表内部纵向滚动）。 */
const pos = ref({ x: 0, y: 0, maxH: 440 })
const wrapEl = ref<HTMLElement | null>(null)

// 弹层定位：默认右对齐按钮右缘，并对视口左右边界做 clamp，避免窄屏下右侧溢出被截断
const PANEL_WIDTH = 220
/** 列表理想最大高度（字段多时在内部滚动，而不是把弹层顶出屏幕外）。 */
const LIST_MAX = 360
/** 头部 + 内边距的固定高度估算（用于判断下方是否放得下）。 */
const CHROME_H = 84
/** 最小可用高度：头部 + 约两行字段，再小就没有意义了。 */
const MIN_USABLE = CHROME_H + 48
function placePopup() {
  if (!wrapEl.value) return
  const r = wrapEl.value.getBoundingClientRect()
  const vw = window.innerWidth || 0
  const vh = window.innerHeight || 0
  let left = r.right - PANEL_WIDTH
  if (left < 8) left = Math.min(r.left, Math.max(8, vw - PANEL_WIDTH - 8))
  if (vw && left + PANEL_WIDTH > vw - 8) left = vw - PANEL_WIDTH - 8

  // 关键：弹层是 position:fixed，按钮靠近窗口底部时若一律向下展开，
  // 超出的部分会被视口裁掉（用户看到的是「列表只显示 8 条、不能滚动」）。
  // 先算下方可用高度，不够就翻到按钮上方，并把剩余高度交给列表内部滚动。
  const panelH = CHROME_H + LIST_MAX
  const below = vh - (r.bottom + 6) - 12
  const above = r.top - 12
  let top = r.bottom + 6
  let avail = Math.min(panelH, below)
  if (below < 240 && above > below) {
    avail = Math.min(panelH, above)
    top = Math.max(8, r.top - 6 - avail)
  }
  // 硬约束：弹层不允许超出视口下沿。矮窗口下若仍按 160 的下限撑开，
  // 弹层会有一半落在屏幕外，列表既看不全也滚不到（真机复现过）。
  avail = Math.min(avail, Math.max(0, vh - top - 8))
  if (avail < MIN_USABLE) {
    // 上下都放不下：贴顶展开，把整块可用高度都给列表（仍可在内部滚动）
    top = 8
    avail = Math.max(MIN_USABLE, Math.min(panelH, vh - 16))
  }
  pos.value = { x: Math.max(8, left), y: Math.max(8, top), maxH: Math.round(avail) }
}
function toggle() {
  popVisible.value = !popVisible.value
  if (popVisible.value) placePopup()
}
function open() {
  popVisible.value = true
  placePopup()
}
function onDocMouseDown(e: MouseEvent) {
  const t = e.target as Node
  if (wrapEl.value?.contains(t)) return
  const panel = document.querySelector('.business-column-filter')
  if (panel && panel.contains(t)) return
  popVisible.value = false
}
/**
 * 弹层是 position:fixed + 内联 left/top，坐标只在打开那一刻算一次。
 * 窗口尺寸变化或页面滚动后锚点已经移动，旧坐标会让弹层飘到别处（甚至整块跑到视口外，
 * 表现为「弹窗只露一条、里面滚不动」）。这里跟随锚点重算；锚点被滚出视口就关掉。
 */
function onViewportChange() {
  if (!popVisible.value) return
  const r = wrapEl.value?.getBoundingClientRect()
  const vh = window.innerHeight || 0
  if (!r || r.bottom < 0 || r.top > vh) { popVisible.value = false; return }
  placePopup()
}
onMounted(() => {
  document.addEventListener('mousedown', onDocMouseDown, true)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})
onUnmounted(() => {
  document.removeEventListener('mousedown', onDocMouseDown, true)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})

defineExpose({ open })
</script>

<template>
  <div ref="wrapEl" class="business-column-filter-wrap">
    <span v-if="hideTrigger" class="field-filter-anchor"></span>
    <t-button v-else variant="outline" size="small" @click="toggle">
      <template #icon><t-icon name="view-list" size="14px" /></template>
      字段
    </t-button>
    <teleport to="body">
      <div v-if="popVisible" class="business-column-filter"
        :style="{ left: pos.x + 'px', top: pos.y + 'px', maxHeight: pos.maxH + 'px' }">
        <div class="field-popup-content">
          <div class="field-popup-head">
            <span class="field-popup-title">显示字段</span>
            <div class="field-popup-actions">
              <t-button variant="text" size="small" @click="emit('selectAll')">全选</t-button>
              <t-button variant="text" size="small" @click="emit('reset')">重置</t-button>
            </div>
          </div>
          <t-checkbox-group :value="visibleKeys" class="field-popup-list"
            @change="(val: any) => emit('update:visibleKeys', val as string[])">
            <t-checkbox v-for="col in columns" :key="col.key" :value="col.key" class="field-popup-item"
              :title="col.label">
              {{ col.label }}
            </t-checkbox>
          </t-checkbox-group>
        </div>
      </div>
    </teleport>
  </div>
</template>

<style scoped>
.field-filter-anchor { display: inline-block; width: 0; height: 0; overflow: hidden; }
.field-filter-wrap { display: inline-block; }
.field-filter-anchor { display: inline-block; width: 0; height: 0; overflow: hidden; }
.business-column-filter {
  position: fixed;
  z-index: 3000;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  box-shadow: var(--td-shadow-2);
  box-sizing: border-box;
  /* 纵向 flex：高度受 maxHeight（内联，按可用视口算）约束，
     剩余空间全部交给下面的列表去滚动，弹层本身不溢出屏幕。 */
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.field-popup-content {
  /* box-sizing 缺省是 content-box：220 + 左右 padding 24 = 244 会撑出弹窗，
     而内部列表又按 220 排版，两层宽度不一致就把底部横向滚动条顶出来了。 */
  width: 220px;
  box-sizing: border-box;
  padding: 12px;
  display: flex;
  flex-direction: column;
  flex: 1 1 auto;
  min-height: 0;
}
.field-popup-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  flex: none;
  .field-popup-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
  .field-popup-actions { display: flex; gap: 0; }
}
.field-popup-list {
  /* 字段多时（15~30 个）在列表内部纵向滚动，任何一项都不会被裁掉 */
  max-height: 360px;
  min-height: 0;
  flex: 1 1 auto;
  overflow-y: auto;
  /* 长字段名（如「需求响应分摊费（含租赁或购置柴发保生产补贴）」）不换行，
     会把列表撑宽并顶出横向滚动条。这里禁掉横向滚动，改由单项省略号收敛。 */
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
  /* TDesign 的 .t-checkbox-group 自带 flex-wrap: wrap。列方向 + wrap 会让
     超高的字段**换到右侧新开一列**（真机：19 项被拆成 2 列，第二列落在容器外，
     被 overflow-x:hidden 裁掉 → 既不纵向滚动、后面的字段也点不到，
     用户看到的就是「筛选器只显示十几个字段、截断了」）。必须显式 nowrap。 */
  flex-wrap: nowrap;
}
/* 注意：本文件的 <style> 是**纯 CSS**（无 lang="less"），
   嵌套写法 + :deep() 会被编译器拼成非法选择器（真机：样式整块失效）。
   涉及 :deep 的规则一律平铺在顶层。 */
.field-popup-list .field-popup-item {
  display: flex;
  align-items: center;
  /* flex 列容器里子项默认 flex-shrink: 1 —— 高度不够时它们会被**压扁**而不是
     触发滚动（真机：矮窗口下 19 个字段挤成一坨、看着像"只显示了几条"）。
     固定不收缩，超出部分交给列表滚动。 */
  flex: none;
  /* t-checkbox 默认按内容撑开（inline-flex），长字段名会把列表撑宽 →
     底部出现横向滚动条。强制占满列表宽度，溢出一律裁掉。 */
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow: hidden;
}
.field-popup-list :deep(.t-checkbox__label) {
  /* TDesign 的标签 span 默认不截断：撑满剩余宽度并省略，完整文案在 title 里 */
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
