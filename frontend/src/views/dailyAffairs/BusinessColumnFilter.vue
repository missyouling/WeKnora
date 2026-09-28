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

function toggle() {
  popVisible.value = !popVisible.value
  if (popVisible.value && wrapEl.value) {
    const r = wrapEl.value.getBoundingClientRect()
    pos.value = { x: r.left, y: r.bottom + 6 }
  }
}
function open() {
  popVisible.value = true
  if (wrapEl.value) {
    const r = wrapEl.value.getBoundingClientRect()
    pos.value = { x: r.left, y: r.bottom + 6 }
  }
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
      <div v-show="popVisible" class="business-column-filter" :style="{ left: pos.x + 'px', top: pos.y + 'px' }">
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
            <t-checkbox v-for="col in columns" :key="col.key" :value="col.key" class="field-popup-item">
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
  width: 220px;
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
  display: flex;
  flex-direction: column;
  .field-popup-item { display: flex; align-items: center; }
}
</style>
