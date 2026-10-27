<template>
  <!--
    跨分类字段选择器（仅「费用清单」用）。

    费用清单是综合汇总列表，自身没有独立存储：每个字段都来自市电账单 / 光伏账单 /
    电费核算 / 水费核算 / 气费核算。所以这里**不提供自由输入**，只允许检索并引用
    其它分类已存在的字段；候选项一律带 `[来源分类]` 前缀，避免同名误选
    （「电费」在四个分类里都存在，只写字段名根本分不清）。

    为什么不用 t-select：它需要「有值」的受控 model 才能稳定触发 change，而本组件是
    「选完即加入、自身不留值」的一次性选择器 —— 受控空值时 change 在某些交互路径下
    不触发（真机复现：选中候选后毫无反应）。改成输入框 + 自绘候选面板，行为确定。
  -->
  <div ref="rootRef" class="source-field-picker">
    <t-input :model-value="keyword" :placeholder="placeholder" clearable
      @update:model-value="onInput" @focus="open = true" @keydown.enter.prevent="pickFirst">
      <template #prefixIcon><t-icon name="search" size="16px" /></template>
    </t-input>
    <div v-if="open" class="sfp-panel">
      <div v-if="loading" class="sfp-empty">正在加载字段…</div>
      <div v-else-if="!filtered.length" class="sfp-empty">没有匹配的字段</div>
      <button v-for="(o, i) in filtered" :key="o.value" type="button" class="sfp-option"
        :class="{ 'is-active': i === activeIndex }" :title="o.label" @click="pick(o)">
        <span class="sfp-cat">{{ o.category }}</span>
        <span class="sfp-name">{{ o.name }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { loadEnergyCategorySubs, type EnergySub } from './energyViewColumns'
import { SOURCE_CATEGORIES } from './tenantBillingSources'

const props = defineProps<{
  /** 已添加的字段 key（`分类__字段`），用于过滤候选项 */
  exclude?: string[]
  /** 只在这些分类里检索（不传 = 全部五个来源分类） */
  onlyCategories?: string[]
}>()
const emit = defineEmits<{
  (e: 'select', payload: { key: string; label: string; dataType: string; source: { category: string; field: string; mode: 'field' } }): void
}>()

interface Option {
  value: string
  label: string
  category: string
  name: string
  dataType: string
  field: string
}

const loading = ref(false)
const keyword = ref('')
const open = ref(false)
const activeIndex = ref(0)
const rootRef = ref<HTMLElement | null>(null)
const placeholder = '搜索并添加其它分类的字段（如 [水费核算] 水费）'
const byCategory = ref<Record<string, EnergySub[]>>({})

const allOptions = computed<Option[]>(() => {
  const out: Option[] = []
  const excluded = new Set((props.exclude || []).map(String))
  const allow = props.onlyCategories && props.onlyCategories.length ? new Set(props.onlyCategories) : null
  for (const cat of SOURCE_CATEGORIES) {
    if (allow && !allow.has(cat)) continue
    for (const sub of byCategory.value[cat] || []) {
      if (sub.enabled === false) continue
      const key = `${cat}__${sub.name}`
      if (excluded.has(key)) continue
      out.push({
        value: key,
        // 候选项固定 `[来源分类] 字段名`：同名不同源的字段必须一眼可分
        label: `[${cat}] ${sub.label || sub.name}`,
        category: cat,
        name: String(sub.label || sub.name),
        dataType: String(sub.data_type || 'text'),
        field: String(sub.name),
      })
    }
  }
  return out
})

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return allOptions.value
  return allOptions.value.filter((o) => o.label.toLowerCase().includes(kw))
})

watch(filtered, () => { activeIndex.value = 0 })

function onInput(v: string) {
  keyword.value = String(v ?? '')
  open.value = true
}

function pick(o: Option) {
  const parts = o.value.split('__')
  const category = parts[0]
  emit('select', {
    key: o.value,
    label: o.name,
    dataType: o.dataType,
    source: { category, field: parts.slice(1).join('__') || o.field, mode: 'field' },
  })
  // 一次性选择：选完清空，不保留选中态（避免「以为已选、其实没加」）
  keyword.value = ''
  open.value = false
}

function pickFirst() {
  const o = filtered.value[activeIndex.value] || filtered.value[0]
  if (o) pick(o)
}

/** 点击外部收起面板（输入框自身与其面板除外）。 */
function onDocClick(e: MouseEvent) {
  if (!open.value) return
  const el = rootRef.value
  if (el && !el.contains(e.target as Node)) open.value = false
}

onMounted(async () => {
  document.addEventListener('mousedown', onDocClick)
  loading.value = true
  try {
    const cats = props.onlyCategories && props.onlyCategories.length ? props.onlyCategories : SOURCE_CATEGORIES
    const entries = await Promise.all(
      cats.map(async (cat) => {
        try {
          return [cat, await loadEnergyCategorySubs(cat)] as const
        } catch {
          return [cat, [] as EnergySub[]] as const
        }
      }),
    )
    const map: Record<string, EnergySub[]> = {}
    for (const [cat, subs] of entries) map[cat] = subs || []
    byCategory.value = map
  } finally {
    loading.value = false
  }
})

onBeforeUnmount(() => { document.removeEventListener('mousedown', onDocClick) })
</script>

<style lang="less" scoped>
.source-field-picker {
  position: relative;
  width: 100%;
}
.sfp-panel {
  position: absolute;
  z-index: 3000;
  left: 0;
  right: 0;
  top: calc(100% + 4px);
  max-height: 240px;
  overflow-y: auto;
  padding: 4px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-2);
}
.sfp-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 4px;
  background: transparent;
  cursor: pointer;
  text-align: left;
  font-size: 12px;
  color: var(--td-text-color-primary);

  &:hover,
  &.is-active { background: var(--td-bg-color-secondarycontainer); }
}
.sfp-cat {
  flex: none;
  max-width: 84px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  padding: 0 4px;
  border-radius: 2px;
  font-size: 11px;
  line-height: 16px;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}
.sfp-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.sfp-empty {
  padding: 10px 0;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
</style>
