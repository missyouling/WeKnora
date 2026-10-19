<!--
  BusinessFilterBar —— 声明式筛选工具栏中台组件（蓝图 P1-3）。

  由 BusinessListToolbar 升级而来：布局/批量浮条/列显隐沿用既有实现（视觉零变化），
  筛选控件改为 `FilterFieldSpec[]` 声明——页面只写"我要哪几个筛选器、各自什么类型"，
  不再手写 t-select / t-date-range-picker。

  契约：
    - `modelValue` 是扁平的 FilterState（键 = spec.key），组件只做不可变更新；
    - keyword 类型的 spec 渲染在搜索位，其余按声明顺序渲染在筛选区；
    - 组件内严禁出现任何业务 scope 分支判断：文案全部来自 spec 的 placeholder/options。

  用法：
    <BusinessFilterBar v-model="filterState" :fields="filterFields"
      :columns="columns" :visible-keys="visibleKeys" settings-text="设置"
      :primary-action="{ text: '上传发票', icon: 'upload' }" @settings="..." @primary="..." />
-->
<template>
  <BusinessListToolbar :keyword="keywordValue" :search-placeholder="searchPlaceholder"
    :selected-count="selectedCount" :hide-batch-bar="hideBatchBar" :show-refresh="showRefresh"
    @update:keyword="v => patch(keywordSpec, v)" @refresh="emit('filter-change')"
    @clear-selection="emit('clear-selection')">
    <template #type-extra>
      <template v-for="f in restSpecs" :key="f.key">
        <!-- 下拉筛选（类型/税率…）：宽度随选项自适应，禁止收缩避免截断 -->
        <div v-if="f.type === 'select'" class="bfs-field">
          <t-select :value="state[f.key]" :options="f.options || []" :placeholder="f.placeholder || '筛选'"
            :clearable="f.clearable !== false" class="bfs-select"
            @update:value="(v: any) => patch(f, v)">
            <template #prefixIcon><t-icon :name="f.icon || DEFAULT_FILTER_ICONS.select" size="16px" /></template>
          </t-select>
        </div>

        <!-- 日期区间 -->
        <t-date-range-picker v-else-if="f.type === 'date-range'" :value="state[f.key]"
          :placeholder="f.placeholder || '日期区间'" :clearable="f.clearable !== false" allow-input
          class="bfs-date-range" @change="(v: any) => patch(f, v)">
          <template #prefixIcon><t-icon :name="f.icon || DEFAULT_FILTER_ICONS['date-range']" size="16px" /></template>
        </t-date-range-picker>

        <!-- 数值区间：最小/最大两个输入 -->
        <div v-else-if="f.type === 'number-range'" class="bfs-field bfs-range">
          <t-input :value="rangePair(state[f.key])[0]" :placeholder="f.placeholders?.[0] || '最小'"
            class="bfs-num" clearable @change="(v: any) => patchRange(f, v, 0)" />
          <span class="bfs-range-sep">-</span>
          <t-input :value="rangePair(state[f.key])[1]" :placeholder="f.placeholders?.[1] || '最大'"
            class="bfs-num" clearable @change="(v: any) => patchRange(f, v, 1)" />
        </div>
      </template>
      <slot name="type-extra" />
    </template>

    <template #columns>
      <slot name="columns">
        <BusinessColumnFilter v-if="columns.length" :columns="columns" :visible-keys="visibleKeys"
          @update:visible-keys="v => emit('columns-change', v)" @reset="emit('reset-columns')"
          @select-all="emit('select-all-columns')" />
      </slot>
    </template>

    <template #right-extra>
      <slot name="right-extra" />
      <t-button v-if="settingsText" variant="outline" size="small" @click="emit('settings')">
        <template #icon><t-icon name="setting" size="14px" /></template>
        {{ settingsText }}
      </t-button>
      <t-button v-if="primaryAction" theme="primary" size="small" @click="emit('primary')">
        <template #icon><t-icon :name="primaryAction.icon || 'add'" size="14px" /></template>
        {{ primaryAction.text }}
      </t-button>
    </template>

    <template #batch-actions>
      <slot name="batch-actions" />
    </template>
  </BusinessListToolbar>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import BusinessListToolbar from './BusinessListToolbar.vue'
import BusinessColumnFilter from './BusinessColumnFilter.vue'
import {
  DEFAULT_FILTER_ICONS,
  patchFilterState,
  rangePair,
  splitFilterSpecs,
  writeRange,
  type FilterFieldSpec,
  type FilterState,
} from './businessFilter'

const props = withDefaults(defineProps<{
  modelValue: FilterState
  fields?: FilterFieldSpec[]
  searchPlaceholder?: string
  /** 列定义（传了才渲染列显隐） */
  columns?: Array<{ key: string; label: string }>
  visibleKeys?: string[]
  selectedCount?: number
  hideBatchBar?: boolean
  showRefresh?: boolean
  /** 有值才渲染设置按钮 */
  settingsText?: string
  /** 主操作按钮（上传/新建…） */
  primaryAction?: { text: string; icon?: string }
}>(), {
  fields: () => [],
  searchPlaceholder: '请输入关键词',
  columns: () => [],
  visibleKeys: () => [],
  selectedCount: 0,
  hideBatchBar: false,
  showRefresh: true,
  settingsText: '',
  primaryAction: undefined,
})

const emit = defineEmits<{
  (e: 'update:modelValue', v: FilterState): void
  /** 显式刷新（点刷新按钮 / spec.immediate 的控件变更） */
  (e: 'filter-change'): void
  (e: 'columns-change', keys: string[]): void
  (e: 'reset-columns'): void
  (e: 'select-all-columns'): void
  (e: 'settings'): void
  (e: 'primary'): void
  (e: 'clear-selection'): void
}>()

const state = computed<FilterState>(() => props.modelValue || {})
// spec 列表由父级动态声明（分类下拉随数据变化），必须整体响应式，不能 setup 时解构
const split = computed(() => splitFilterSpecs(props.fields))
const keywordSpec = computed(() => split.value.keyword)
const restSpecs = computed(() => split.value.rest)
const keywordValue = computed(() => (keywordSpec.value ? state.value[keywordSpec.value.key] ?? '' : ''))

function patch(spec: FilterFieldSpec | undefined, v: any) {
  if (!spec) return
  emit('update:modelValue', patchFilterState(state.value, spec, v))
  if (spec.immediate) emit('filter-change')
}

function patchRange(spec: FilterFieldSpec, v: any, idx: 0 | 1) {
  const [min, max] = rangePair(state.value[spec.key])
  const next = writeRange(idx === 0 ? v : min, idx === 1 ? v : max)
  emit('update:modelValue', patchFilterState(state.value, spec, next))
  if (spec.immediate) emit('filter-change')
}
</script>

<style scoped>
/* 只补筛选控件自身的样式；工具栏布局沿用 BusinessListToolbar */
.bfs-field { display: flex; align-items: center; gap: 4px; }
.bfs-select { width: auto; flex-shrink: 0; min-width: max-content; }
.bfs-date-range { width: 240px; flex-shrink: 0; }
.bfs-num { width: 96px; flex-shrink: 0; }
.bfs-range-sep { color: var(--td-text-color-placeholder); }
</style>
