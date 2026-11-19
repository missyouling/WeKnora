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
              <!-- 第 65 轮：行内编辑（opt-in）。失焦 / 回车提交，值未变不发请求。 -->
              <input v-if="isEditable(c)" class="detail-cell-input"
                :data-cell-input="`${c.colKey}`"
                :value="draftOf(row, c)"
                :inputmode="c.colKey === '备注' ? 'text' : 'decimal'"
                @input="(e: any) => onCellInput(row, c, e.target.value)"
                @focus="(e: any) => onCellFocus(row, c, e.target)"
                @blur="(e: any) => onCellCommit(row, c, e.target.value)"
                @keyup.enter="(e: any) => e.target.blur()" />
              <template v-else>{{ cellText(row, c) }}</template>
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
  /**
   * 第 65 轮：行内编辑（opt-in）。只有 `editableCols` 命中的列会渲染输入框，
   * 提交时 emit `cell-commit`（row / colKey / value），由父级决定调哪个接口。
   */
  editableCols?: string[]
  /** 行内编辑的语义标识（父级用来区分不同表格的提交来源）。 */
  editableKey?: string
}>(), {
  foot: () => [],
  rowKey: 'id',
  maxBodyHeight: 380,
  emptyText: '暂无数据',
  name: 'detail',
  editableCols: () => [],
  editableKey: '',
})

const emit = defineEmits<{
  (e: 'cell-commit', payload: { key: string; row: Record<string, any>; colKey: string; value: string }): void
}>()

/** 行内编辑草稿（按 `${rowKey}|${colKey}` 存，避免直接改 props.rows）。 */
const drafts = ref<Record<string, string>>({})
const editableSet = computed(() => new Set(props.editableCols || []))
function isEditable(c: { colKey: string }): boolean {
  return editableSet.value.has(c.colKey)
}
function draftKeyOf(row: Record<string, any>, colKey: string): string {
  return `${row?.[props.rowKey] ?? ''}|${colKey}`
}
function draftOf(row: Record<string, any>, c: { colKey: string }): string {
  const k = draftKeyOf(row, c.colKey)
  if (k in drafts.value) return drafts.value[k]
  const v = row?.[c.colKey]
  return v === null || v === undefined ? '' : String(v)
}
function onCellFocus(row: Record<string, any>, c: { colKey: string }, el: HTMLInputElement) {
  drafts.value[draftKeyOf(row, c.colKey)] = el.value
}
/** 数字列只允许数字与小数点（负数、字母、符号一律拦掉）。 */
function onCellInput(row: Record<string, any>, c: { colKey: string }, raw: string) {
  const isText = c.colKey === '备注'
  const clean = isText ? raw : raw.replace(/[^\d.]/g, '').replace(/(\..*)\./g, '$1')
  drafts.value[draftKeyOf(row, c.colKey)] = clean
}
function onCellCommit(row: Record<string, any>, c: { colKey: string }, raw: string) {
  const k = draftKeyOf(row, c.colKey)
  const next = String(raw ?? '')
  const prev = row?.[c.colKey] === null || row?.[c.colKey] === undefined ? '' : String(row[c.colKey])
  delete drafts.value[k]
  // 值未变不发请求（避免用户只是点一下也触发保存）
  if (next === prev) return
  emit('cell-commit', { key: props.editableKey, row, colKey: c.colKey, value: next })
}

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
  /* 第 65 轮：弹窗内必须吃满剩余高度，否则数据区 max-height 生效但外层先被裁 */
  flex: 1 1 auto;
  height: 100%;
  width: 100%;
  /* 第 64 轮：容器自身永不出现横向滚动条（三段表已按百分比收口） */
  overflow-x: hidden;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  overflow: hidden;
  /* 第 64 轮：边框计入宽度 —— 否则 width:100% + 1px 边框 = 容器外溢 2px，
     真机上表现为「弹窗内 2px 横向滚动」（sw 1318 / cw 1316）。 */
  box-sizing: border-box;
}
/* 表头与合计行绝不滚动；第 64 轮起同步预留滚动条槽位，
   与数据区的 scrollbar-gutter 同宽，三段列竖线才能像素级对齐 */
.detail-table-head,
.detail-table-foot {
  flex: 0 0 auto;
  overflow: hidden !important;
  overflow-x: hidden !important;
  /* 第 64 轮：这里**不能**加 scrollbar-gutter —— 表头/合计行本身不滚动，
     预留槽位会让它们的可用宽度比数据区少一个滚动条宽，列竖线错位。
     滚动条宽度已由 JS 实测后通过 paddingRight 精确补偿（sideStyle）。 */
  background: var(--td-bg-color-secondarycontainer);
}
.detail-table-body {
  flex: 1 1 auto;
  min-height: 0;
  /* 第 65 轮：显式 auto（原值已是 auto，这里加 !important 防被父级 hidden 覆盖），
     并保证内容超出时一定出现可见垂直滚动条 */
  overflow-y: auto !important;
  /* 第 64 轮：横向溢出必须被吞掉（外层容器也设 hidden），否则底部出现横滚轨道 */
  overflow-x: hidden !important;
  /* 滚动条常驻槽位；真实宽度仍以 JS 实测值补 padding（Firefox/Safari 宽度不同） */
  scrollbar-gutter: stable;
  background: var(--td-bg-color-container);
}
/* 第 64 轮：表格恒等于容器宽度，任何视口都不产生横向滚动 */
.detail-table table {
  border-collapse: collapse;
  table-layout: fixed;
  width: 100%;
  min-width: 0;
  font-size: var(--td-font-size-body-small);
}
.detail-table th,
.detail-table td {
  box-sizing: border-box;
  /* 第 64 轮：垂直 padding 保持 0 —— 表头/合计行靠 height 定高，
     加 padding 会让三段的行高不一致，列宽实测出现 1px+ 偏差（61.2 对齐断言）。 */
  padding: 0 8px;
  height: 36px;
  line-height: 20px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--td-component-stroke);
  border-bottom: 1px solid var(--td-component-stroke);
}
/* 第 65 轮：行内编辑输入框 —— 无边框融入单元格，聚焦时高亮 */
.detail-cell-input {
  width: 100%;
  height: 28px;
  padding: 0 4px;
  border: 1px solid transparent;
  border-radius: var(--td-radius-small);
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: inherit;
  outline: none;

  &:hover { border-color: var(--td-component-stroke); }
  &:focus {
    border-color: var(--td-brand-color);
    background: var(--td-bg-color-container);
  }
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
