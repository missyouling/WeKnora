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
const pos = ref({ x: 0, y: 0 })
const wrapEl = ref<HTMLElement | null>(null)

// 弹层定位：默认右对齐按钮右缘，并对视口左右边界做 clamp，避免窄屏下右侧溢出被截断
const PANEL_WIDTH = 220
function placePopup() {
  if (!wrapEl.value) return
  const r = wrapEl.value.getBoundingClientRect()
  const vw = window.innerWidth || 0
  let left = r.right - PANEL_WIDTH
  if (left < 8) left = Math.min(r.left, Math.max(8, vw - PANEL_WIDTH - 8))
  if (vw && left + PANEL_WIDTH > vw - 8) left = vw - PANEL_WIDTH - 8
  pos.value = { x: Math.max(8, left), y: r.bottom + 6 }
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
onMounted(() => document.addEventListener('mousedown', onDocMouseDown, true))
onUnmounted(() => document.removeEventListener('mousedown', onDocMouseDown, true))

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
      <div v-if="popVisible" class="business-column-filter" :style="{ left: pos.x + 'px', top: pos.y + 'px' }">
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
}
.field-popup-content {
  /* box-sizing 缺省是 content-box：220 + 左右 padding 24 = 244 会撑出弹窗，
     而内部列表又按 220 排版，两层宽度不一致就把底部横向滚动条顶出来了。 */
  width: 220px;
  box-sizing: border-box;
  padding: 12px;
}
.field-popup-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
  .field-popup-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
  .field-popup-actions { display: flex; gap: 0; }
}
.field-popup-list {
  max-height: 320px;
  overflow-y: auto;
  /* 长字段名（如「需求响应分摊费（含租赁或购置柴发保生产补贴）」）不换行，
     会把列表撑宽并顶出横向滚动条。这里禁掉横向滚动，改由单项省略号收敛。 */
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
}
/* 注意：本文件的 <style> 是**纯 CSS**（无 lang="less"），
   嵌套写法 + :deep() 会被编译器拼成非法选择器（真机：样式整块失效）。
   涉及 :deep 的规则一律平铺在顶层。 */
.field-popup-list .field-popup-item {
  display: flex;
  align-items: center;
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
