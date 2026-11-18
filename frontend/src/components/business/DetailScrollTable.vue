<!--
  DetailScrollTable —— 明细弹窗专用「三段分离」表格（第 61 轮）。

  为什么不用 t-table：TDesign 的 sticky-header + max-height 仍把 thead / tbody / tfoot
  放在**同一个**滚动容器里，垂直滚动条会从表头顶部一路贯穿到底部合计行（图 1 红圈）。
  这里改成三个独立容器：

      .detail-table-head   固定表头，overflow: hidden
      .detail-table-body   唯一可滚动区，overflow-y: auto; max-height: 380px
      .detail-table-foot   固定合计行，overflow: hidden

  三段共用同一份 <colgroup>（列宽同源、table-layout: fixed），
  出现垂直滚动条时给表头与合计行补相同的 padding-right（= 实测滚动条宽度），
  保证每一条纵向分割线像素级对齐。
-->
<template>
  <div class="detail-table" :data-detail-scroll-table="name">
    <div class="detail-table-head" :style="sideStyle">
      <table :style="tableStyle">
        <colgroup>
          <col v-for="c in cols" :key="`h-${c.colKey}`" :style="{ width: pctOf(c) }" />
        </colgroup>
        <thead>
          <tr>
            <th v-for="c in cols" :key="`th-${c.colKey}`" :style="{ textAlign: c.align || 'left' }">
              {{ c.title }}
            </th>
          </tr>
        </thead>
      </table>
    </div>

    <div ref="bodyRef" class="detail-table-body" :style="bodyStyle" @scroll="onScroll">
      <table :style="tableStyle">
        <colgroup>
          <col v-for="c in cols" :key="`b-${c.colKey}`" :style="{ width: pctOf(c) }" />
        </colgroup>
        <tbody>
          <tr v-for="(row, i) in rows" :key="row[rowKey] ?? i">
            <td v-for="c in cols" :key="`td-${c.colKey}`" :style="{ textAlign: c.align || 'left' }"
              :title="cellText(row, c)">
              {{ cellText(row, c) }}
            </td>
          </tr>
          <tr v-if="!rows.length">
            <td :colspan="cols.length" class="detail-table-empty">{{ emptyText }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div v-if="footRows.length" class="detail-table-foot" :style="sideStyle">
      <table :style="tableStyle">
        <colgroup>
          <col v-for="c in cols" :key="`f-${c.colKey}`" :style="{ width: pctOf(c) }" />
        </colgroup>
        <tfoot>
          <tr v-for="(row, i) in footRows" :key="`foot-${i}`">
            <td v-for="c in cols" :key="`tf-${c.colKey}`" :style="{ textAlign: c.align || 'left' }"
              :title="cellText(row, c)">
              {{ cellText(row, c) }}
            </td>
          </tr>
        </tfoot>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  /** 列定义：colKey + title + 固定列宽（px，必须给全，三段 colgroup 才能对齐）。 */
  columns: Array<{ colKey: string; title: string; width?: number; align?: 'left' | 'right' | 'center' }>
  rows: Array<Record<string, any>>
  /** 合计行（单行对象数组，键与列 colKey 一致；空数组则不渲染 tfoot）。 */
  foot?: Array<Record<string, any>>
  rowKey?: string
  /** 数据区最大高度：滚动条严格止于合计行上边框。 */
  maxBodyHeight?: number
  emptyText?: string
  name?: string
}>(), {
  foot: () => [],
  rowKey: 'id',
  maxBodyHeight: 380,
  emptyText: '暂无数据',
  name: 'detail',
})

const DEFAULT_WIDTH = 120
const bodyRef = ref<HTMLElement | null>(null)
/** 实测滚动条宽度：0（无滚动）时表头/合计行右侧不留白。 */
const scrollbarWidth = ref(0)
let observer: ResizeObserver | null = null

const cols = computed(() => props.columns.map((c) => ({ ...c, width: Number(c.width) || DEFAULT_WIDTH })))
const totalWidth = computed(() => cols.value.reduce((n, c) => n + c.width, 0))
/**
 * 第 63 轮：列宽改「百分比同源」。
 *
 * 旧实现按 px 写 colgroup + `minWidth:100%`：当列宽之和大于弹窗内容区时，
 * 表头/合计行被 overflow:hidden 裁掉，数据区出现横向滚动条（社保/公积金
 * 账单明细弹窗真机复现）。改成占比后表格恒等于容器宽度，三段仍共用同一份
 * 百分比，列分割线像素级对齐，且任何视口都不会出横向滚动条。
 */
const tableStyle = computed(() => ({
  width: '100%',
  tableLayout: 'fixed' as const,
}))
function pctOf(c: { width: number }): string {
  const total = totalWidth.value || 1
  return `${(c.width / total) * 100}%`
}
const bodyStyle = computed(() => ({ maxHeight: `${props.maxBodyHeight}px` }))
/** 表头 / 合计行用 padding 让出滚动条宽度（不用 margin，避免表格错位）。 */
const sideStyle = computed(() => ({ paddingRight: scrollbarWidth.value ? `${scrollbarWidth.value}px` : '0' }))
const footRows = computed(() => props.foot || [])

function cellText(row: Record<string, any>, c: { colKey: string }): string {
  const v = row?.[c.colKey]
  if (v === null || v === undefined) return ''
  return String(v)
}

/** 只有数据区出现垂直滚动时才补宽；无滚动条时归零。 */
function syncScrollbar() {
  const el = bodyRef.value
  if (!el) return
  const overflow = el.scrollHeight > el.clientHeight + 1
  const w = overflow ? Math.max(0, el.offsetWidth - el.clientWidth) : 0
  if (w !== scrollbarWidth.value) scrollbarWidth.value = w
}

function onScroll() { /* 表头固定，无需联动；保留钩子便于真机断言滚动事件 */ }

onMounted(() => {
  nextTick(syncScrollbar)
  if (typeof ResizeObserver !== 'undefined' && bodyRef.value) {
    observer = new ResizeObserver(() => syncScrollbar())
    observer.observe(bodyRef.value)
  }
})
onBeforeUnmount(() => { observer?.disconnect(); observer = null })
watch(() => [props.rows.length, props.columns.length, props.maxBodyHeight], () => nextTick(syncScrollbar))
</script>

<style scoped>
.detail-table {
  display: flex;
  flex-direction: column;
  min-height: 0;
  width: 100%;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  overflow: hidden;
}
/* 表头与合计行绝不滚动 */
.detail-table-head,
.detail-table-foot {
  flex: 0 0 auto;
  overflow: hidden !important;
  background: var(--td-bg-color-secondarycontainer);
}
.detail-table-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  /* 滚动条常驻槽位；真实宽度仍以 JS 实测值补 padding（Firefox/Safari 宽度不同） */
  scrollbar-gutter: stable;
  background: var(--td-bg-color-container);
}
.detail-table table {
  border-collapse: collapse;
  font-size: var(--td-font-size-body-small);
}
.detail-table th,
.detail-table td {
  box-sizing: border-box;
  padding: 0 8px;
  height: 36px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--td-component-stroke);
  border-bottom: 1px solid var(--td-component-stroke);
}
.detail-table th:first-child,
.detail-table td:first-child { border-left: 1px solid var(--td-component-stroke); }
.detail-table th {
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  font-weight: 500;
}
.detail-table-foot td {
  background: var(--td-bg-color-secondarycontainer);
  font-weight: 600;
  border-bottom: none;
}
.detail-table-empty {
  height: 120px;
  text-align: center;
  color: var(--td-text-color-placeholder);
}
</style>
