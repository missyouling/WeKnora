<template>
  <!--
    跨分类字段选择器（仅「费用清单」用）。

    费用清单是综合汇总列表，自身没有独立存储：每个字段都来自市电账单 / 光伏账单 /
    电费核算 / 水费核算 / 气费核算。所以这里**不提供自由输入**，只允许检索并引用
    其它分类已存在的字段。

    候选项必须带**多级路径**：`[分类 · 分组] 字段名`。
    市电账单有 8 个分组共用同一批列名（「计费电量」在 7 个分组里各有一条），
    只写 `[市电账单] 计费电量` 会出现 8 条完全一样的选项，用户根本不知道
    自己引用的是「市场化购电费」还是「输配电量电费」——取值结果天差地别。

    为什么不用 t-select：它需要「有值」的受控 model 才能稳定触发 change，而本组件是
    「选完即加入、自身不留值」的一次性选择器 —— 受控空值时 change 在某些交互路径下
    不触发（真机复现：选中候选后毫无反应）。改成输入框 + 自绘候选面板，行为确定。
  -->
  <div ref="rootRef" class="source-field-picker">
    <!-- 不能写 @keydown.enter：TDesign 的 t-input 把 keydown 的第一个参数当「输入值字符串」传，
         Vue 的 .enter 修饰符会执行 `'key' in "总电量"` 直接抛 TypeError。用原生 @enter。 -->
    <t-input :model-value="keyword" :placeholder="placeholder" clearable
      @update:model-value="onInput" @focus="open = true" @enter="pickFirst">
      <template #prefixIcon><t-icon name="search" size="16px" /></template>
    </t-input>
    <div v-if="open" class="sfp-panel">
      <div v-if="loading" class="sfp-empty">正在加载字段…</div>
      <div v-else-if="!filtered.length" class="sfp-empty">没有匹配的字段</div>
      <button v-for="(o, i) in filtered" :key="o.value" type="button" class="sfp-option"
        :class="{ 'is-active': i === activeIndex }" :title="o.label" @click="pick(o)">
        <!-- 一级：分类 -->
        <span class="sfp-cat">{{ o.category }}</span>
        <!-- 二级：分组（同名候选的唯一区分点） -->
        <span v-if="o.groupLabel" class="sfp-group">· {{ o.groupLabel }}</span>
        <span class="sfp-name">{{ o.name }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { loadEnergyCategorySubs, type EnergySub } from './energyViewColumns'
import { SOURCE_CATEGORIES, type FieldSource } from './tenantBillingSources'

const props = defineProps<{
  /** 已添加的字段 key（`分类__分组__字段`），用于过滤候选项 */
  exclude?: string[]
  /** 只在这些分类里检索（不传 = 全部五个来源分类） */
  onlyCategories?: string[]
}>()
const emit = defineEmits<{
  (e: 'select', payload: { key: string; label: string; dataType: string; source: FieldSource }): void
}>()

interface Option {
  /** 全局唯一键：分类__分组__字段（不含分组会撞名，见文件头注释） */
  value: string
  /** 展示文本（也用于搜索）：`[分类 · 分组] 字段名` */
  label: string
  /** 搜索用小写串：分类+分组+分组键+字段名+原始键，多关键词按空格分片全命中 */
  haystack: string
  category: string
  group?: string
  groupLabel?: string
  name: string
  dataType: string
  field: string
}

const loading = ref(false)
const keyword = ref('')
const open = ref(false)
const activeIndex = ref(0)
const rootRef = ref<HTMLElement | null>(null)
const placeholder = '搜索并添加其它分类的字段（支持「分组 字段」组合，如「市场化 计费电量」）'
const byCategory = ref<Record<string, EnergySub[]>>({})

/** 分组键 → 中文名（同一分组在不同 subs 条目上可能只有一处带 group_label）。 */
function labelOfGroup(subs: EnergySub[]): Map<string, string> {
  const m = new Map<string, string>()
  for (const s of subs) {
    const k = String(s?.group || '').trim()
    if (!k) continue
    const gl = String(s?.group_label || '').trim()
    if (gl && !m.has(k)) m.set(k, gl)
  }
  return m
}

const allOptions = computed<Option[]>(() => {
  const out: Option[] = []
  const excluded = new Set((props.exclude || []).map(String))
  const allow = props.onlyCategories && props.onlyCategories.length ? new Set(props.onlyCategories) : null
  for (const cat of SOURCE_CATEGORIES) {
    if (allow && !allow.has(cat)) continue
    const subs = byCategory.value[cat] || []
    const groupLabels = labelOfGroup(subs)
    for (const sub of subs) {
      if (sub.enabled === false) continue
      const group = String(sub.group || '').trim()
      const groupLabel = String(sub.group_label || groupLabels.get(group) || '').trim()
      const field = String(sub.name)
      // key 必须含分组：市电账单 8 个分组都有 qty/rate/fee，
      // 只用 分类__字段 会让 7 条「计费电量」算出同一个 key，
      // 既被 exclude 一并过滤掉（加一条其余全消失），也无法区分。
      const value = group ? `${cat}__${group}__${field}` : `${cat}__${field}`
      if (excluded.has(value)) continue
      const name = String(sub.label || sub.name)
      const path = groupLabel ? `${cat} · ${groupLabel}` : cat
      out.push({
        value,
        label: `[${path}] ${name}`,
        // 搜索覆盖四层：分类 / 分组中文名 / 分组键 / 字段中文名 / 字段键
        haystack: [cat, groupLabel, group, name, field].filter(Boolean).join(' ').toLowerCase(),
        category: cat,
        group: group || undefined,
        groupLabel: groupLabel || undefined,
        name,
        dataType: String(sub.data_type || 'text'),
        field,
      })
    }
  }
  return out
})

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return allOptions.value
  // 空格分词 = AND：输入「市场化 计费电量」时分组名与字段名都要命中
  const tokens = kw.split(/\s+/).filter(Boolean)
  return allOptions.value.filter((o) => tokens.every((t) => o.haystack.includes(t)))
})

watch(filtered, () => { activeIndex.value = 0 })

function onInput(v: string) {
  keyword.value = String(v ?? '')
  open.value = true
}

function pick(o: Option) {
  const source: FieldSource = { category: o.category, field: o.field, mode: 'field' }
  if (o.group) {
    source.group = o.group
    if (o.groupLabel) source.groupLabel = o.groupLabel
  }
  // 原始字段名：费用清单里可能被用户改名，保留它才能回溯「数据取自哪个字段」
  source.label = o.name
  emit('select', { key: o.value, label: o.name, dataType: o.dataType, source })
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
  max-height: 280px;
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
  gap: 6px;
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
/* 分组层级：与分类徽标同一行、弱化一档，让「同名 8 条」一眼可分 */
.sfp-group {
  flex: none;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  line-height: 16px;
  color: var(--td-text-color-secondary);
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
