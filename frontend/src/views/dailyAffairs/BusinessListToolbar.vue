<script setup lang="ts">
interface Props {
  keyword: string
  searchPlaceholder?: string
  typeOptions?: { label: string; value: string }[]
  typeValue?: string
  typeClearable?: boolean
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
  typeClearable: true,
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
    <div class="doc-toolbar__leading">
      <div class="doc-filter-field doc-search-field">
        <t-input :value="props.keyword" :placeholder="props.searchPlaceholder" clearable
          @update:value="(v: string) => emit('update:keyword', v)">
          <template #prefixIcon><t-icon name="search" size="16px" /></template>
        </t-input>
      </div>
      <div v-if="props.typeOptions.length" class="doc-filter-field">
        <t-select :value="props.typeValue" :options="props.typeOptions" placeholder="类型筛选"
          class="doc-type-select doc-filter-field__control" :clearable="props.typeClearable"
          @update:value="(v: string) => emit('update:typeValue', v || '')">
          <template #prefixIcon><t-icon name="file" size="16px" /></template>
        </t-select>
      </div>
      <slot name="type-extra" />
      <t-button v-if="props.showRefresh" variant="outline" size="small" @click="emit('refresh')">
        <template #icon><t-icon name="refresh" size="14px" /></template>
      </t-button>
    </div>
    <div class="doc-toolbar__trailing">
      <slot name="columns" />
      <slot name="right-extra" />
      <t-button v-if="props.primaryActionText" theme="primary" size="small" @click="emit('primary')">
        <template #icon><t-icon name="add" size="14px" /></template>
        {{ props.primaryActionText }}
      </t-button>
    </div>
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
/* 两端对齐布局：左侧筛选区（搜索/类型/刷新）+ 右侧操作区（字段/设置/上传），下拉随内容自适应 */
.doc-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--td-comp-margin-s);
  flex-wrap: wrap;
  /* 工具栏与下方列表保持间距 */
  margin-bottom: var(--td-comp-margin-l);
}
.doc-toolbar__leading {
  display: flex;
  align-items: center;
  gap: var(--td-comp-margin-s);
  flex-wrap: wrap;
  /* 左侧筛选区占据剩余空间，与右侧操作区保持同一行两端对齐 */
  flex: 1 1 auto;
  min-width: 0;
}
.doc-toolbar__trailing {
  display: flex;
  align-items: center;
  gap: var(--td-comp-margin-s);
  flex-wrap: nowrap;
  flex-shrink: 0;
  margin-left: auto;
}
.doc-filter-field { display: flex; align-items: center; }
/* 搜索框固定宽度且禁止 flex 收缩：避免被容器挤压 */
.doc-search-field { width: 240px; flex-shrink: 0; }
/* 类型下拉随选项内容自适应宽度，禁止收缩避免截断 */
.doc-type-select { width: auto; flex-shrink: 0; min-width: max-content; }
.doc-batch-bar-fixed {
  position: fixed;
  bottom: var(--wk-batch-bar-bottom);
  left: 50%;
  transform: translateX(-50%);
  z-index: var(--wk-batch-bar-z);
  min-width: 420px;
  margin-top: 0;
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
