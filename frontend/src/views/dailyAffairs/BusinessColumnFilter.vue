<script setup lang="ts">
import type { ColumnDef } from '@/composables/useBusinessList'

defineProps<{
  columns: ColumnDef[]
  visibleKeys: string[]
}>()
const emit = defineEmits<{
  (e: 'update:visibleKeys', v: string[]): void
  (e: 'reset'): void
  (e: 'selectAll'): void
}>()
</script>

<template>
  <t-popup trigger="click" placement="bottom-left" :hide-empty-popup="false" overlay-inner-class="business-column-filter">
    <t-button variant="outline" size="small">
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
