<script setup lang="ts">
import { ref } from 'vue'
import type { ColumnDef } from '@/composables/useBusinessList'

withDefaults(defineProps<{
  columns: ColumnDef[]
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
defineExpose({
  open: () => { popVisible.value = true },
})
</script>

<template>
  <t-popup trigger="click" placement="bottom-left" :hide-empty-popup="false" overlay-inner-class="business-column-filter"
    :visible="popVisible" @visible-change="(v: boolean) => (popVisible = v)">
    <span v-if="hideTrigger" class="field-filter-anchor"></span>
    <t-button v-else variant="outline" size="small" @click="popVisible = true">
      <template #icon><t-icon name="view-list" size="14px" /></template>
      字段
    </t-button>
    <template #content>
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
    </template>
  </t-popup>
</template>

<style scoped>
.field-filter-anchor { display: inline-block; width: 0; height: 0; overflow: hidden; }
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
