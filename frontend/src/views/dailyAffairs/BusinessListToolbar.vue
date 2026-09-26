<script setup lang="ts">
interface Props {
  keyword: string
  searchPlaceholder?: string
  typeOptions?: { label: string; value: string }[]
  typeValue?: string
  primaryActionText?: string
  showPrint?: boolean
  showRefresh?: boolean
  selectedCount?: number
  hideBatchBar?: boolean
}
const props = withDefaults(defineProps<Props>(), {
  searchPlaceholder: '请输入关键词',
  typeOptions: () => [],
  typeValue: '',
  primaryActionText: '',
  showPrint: true,
  showRefresh: true,
  selectedCount: 0,
  hideBatchBar: false,
})
const emit = defineEmits<{
  (e: 'update:keyword', v: string): void
  (e: 'update:typeValue', v: string): void
  (e: 'primary'): void
  (e: 'print'): void
  (e: 'refresh'): void
  (e: 'clear-selection'): void
}>()
</script>

<template>
  <div class="doc-toolbar">
    <div class="doc-filter-field">
      <t-input :value="props.keyword" :placeholder="props.searchPlaceholder" clearable
        @update:value="(v: string) => emit('update:keyword', v)">
        <template #prefixIcon><t-icon name="search" size="16px" /></template>
      </t-input>
    </div>
    <div v-if="props.typeOptions.length" class="doc-filter-field">
      <t-select :value="props.typeValue" :options="props.typeOptions" placeholder="类型筛选"
        class="doc-type-select doc-filter-field__control" clearable
        @update:value="(v: string) => emit('update:typeValue', v || '')">
        <template #prefixIcon><t-icon name="file" size="16px" /></template>
      </t-select>
    </div>
    <slot name="type-extra" />
    <slot name="columns" />
    <t-button v-if="props.showRefresh" variant="outline" size="small" @click="emit('refresh')">
      <template #icon><t-icon name="refresh" size="14px" /></template>
    </t-button>
    <slot name="right-extra" />
    <t-button v-if="props.primaryActionText" theme="primary" size="small" @click="emit('primary')">
      <template #icon><t-icon name="add" size="14px" /></template>
      {{ props.primaryActionText }}
    </t-button>
  </div>

  <transition name="batch-bar-fade">
    <div v-if="!props.hideBatchBar && props.selectedCount > 0" class="doc-batch-bar-fixed" role="region">
      <div class="batch-bar-inner">
        <div class="batch-bar-left">
          <span class="batch-bar-count">已选 {{ props.selectedCount }} 项</span>
          <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="emit('clear-selection')">
            清除
          </t-button>
        </div>
        <div class="batch-bar-actions">
          <slot name="batch-actions" />
        </div>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.doc-toolbar {
  display: flex;
  align-items: center;
  gap: var(--td-comp-margin-s);
  flex-wrap: wrap;
}
.doc-filter-field { display: flex; align-items: center; }
.doc-type-select { width: 160px; }
.doc-batch-bar-fixed {
  position: sticky;
  bottom: 12px;
  margin-top: 12px;
  z-index: 10;
}
.batch-bar-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  padding: 8px 12px;
  box-shadow: var(--td-shadow-2);
}
.batch-bar-left { display: flex; align-items: center; gap: 8px; }
.batch-bar-count { font-size: 13px; color: var(--td-text-color-primary); }
.batch-bar-actions { display: flex; gap: 8px; }
.batch-bar-fade-enter-active, .batch-bar-fade-leave-active { transition: opacity 0.15s; }
.batch-bar-fade-enter-from, .batch-bar-fade-leave-to { opacity: 0; }
</style>
