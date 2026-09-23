<template>
  <div class="dh-wrap">
    <div v-if="visible" class="dh-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="dh-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" header="已上传文件" :size="`${drawerWidth}px`" :footer="false"
      :close-btn="true" class="fleet-history-drawer" @close="onClose" @update:visible="(v: boolean) => (v || onClose())">
      <div class="dh-body">
        <!-- 上传历史列表（解析 / 提取状态，支持删除历史条目） -->
        <div class="dh-list-region">
          <div class="dh-toolbar">
            <t-input v-model="keyword" placeholder="搜索文件名" clearable class="dh-search"
              @enter="reload(1)" @clear="reload(1)">
              <template #prefix-icon><t-icon name="search" /></template>
            </t-input>
            <t-button theme="default" variant="outline" @click="reload(1)">
              <template #icon><t-icon name="search" size="14px" /></template>
              搜索
            </t-button>
          </div>
          <div class="dh-hint">
            展示该档案库全部上传文件的解析与提取状态。
          </div>
          <div ref="listScrollRef" class="doc-list-scroll" @scroll="onListScroll">
            <div class="doc-list-view">
              <div class="doc-list-header" :style="gridStyle" role="row">
                <div class="cell cell-file" role="columnheader">文件</div>
                <div class="cell" role="columnheader">解析</div>
                <div class="cell" role="columnheader">提取</div>
                <div class="cell" role="columnheader">证照类型</div>
                <div class="cell" role="columnheader">上传时间</div>
                <div class="cell cell-del" role="columnheader">操作</div>
              </div>
              <div class="doc-list-body">
                <div v-for="row in rows" :key="row.id" class="doc-list-row" :style="gridStyle"
                  :class="{ selected: activeRow?.id === row.id }" @click="activeRow = row">
                  <div class="cell cell-file">
                    <div class="dh-file">
                      <span class="dh-file-icon">{{ fileExt(row.file_name) }}</span>
                      <div class="dh-file-meta">
                        <div class="dh-file-name" :title="row.title || row.file_name">{{ row.title || row.file_name }}</div>
                        <div class="dh-file-sub">{{ row.file_name }} · {{ fmtSize(row.file_size) }}</div>
                      </div>
                    </div>
                  </div>
                  <div class="cell">
                    <t-tooltip v-if="row.parse_status === 'failed'" :content="parseError(row)" placement="top">
                      <t-tag size="small" theme="danger" variant="light-outline">解析失败</t-tag>
                    </t-tooltip>
                    <t-tag v-else size="small" :theme="parseTheme(row.parse_status)" variant="light-outline">{{ parseLabel(row.parse_status) }}</t-tag>
                  </div>
                  <div class="cell">
                    <t-tooltip v-if="extractStatus(row) === 'failed'" :content="extractError(row)" placement="top">
                      <t-tag size="small" theme="danger" variant="light-outline">提取失败</t-tag>
                    </t-tooltip>
                    <t-tag v-else-if="extractStatus(row) === 'success'" size="small" theme="success" variant="light-outline">提取完成</t-tag>
                    <t-tag v-else-if="row.parse_status === 'completed'" size="small" theme="warning" variant="light-outline">待提取</t-tag>
                    <span v-else class="row-muted">—</span>
                  </div>
                  <div class="cell">
                    <span v-if="row.doc_type" class="row-mono">{{ row.doc_type }}</span>
                    <span v-else class="row-muted">—</span>
                  </div>
                  <div class="cell"><span class="row-mono">{{ fmtTime(row.created_at) }}</span></div>
                  <div class="cell cell-del">
                    <t-tooltip content="重新解析" placement="top">
                      <t-button variant="text" size="small" shape="square" @click="reparseRow(row)">
                        <template #icon><t-icon name="refresh" size="14px" /></template>
                      </t-button>
                    </t-tooltip>
                    <t-tooltip content="重新提取" placement="top">
                      <t-button variant="text" size="small" shape="square" @click="reextractRow(row)">
                        <template #icon><t-icon name="scan" size="14px" /></template>
                      </t-button>
                    </t-tooltip>
                    <t-popconfirm theme="danger" content="确定从知识库删除该文件吗？已解析记录将一并清除。"
                      confirm-btn="删除" cancel-btn="取消" placement="top" @confirm="removeOne(row)">
                      <t-button variant="text" size="small" shape="square" theme="danger">
                        <template #icon><t-icon name="delete" size="14px" /></template>
                      </t-button>
                    </t-popconfirm>
                  </div>
                </div>
                <div v-if="loading" class="dh-list-loading"><t-loading size="small" text="加载中..." /></div>
                <div v-if="!loading && !rows.length" class="dh-list-empty"><t-empty title="暂无上传历史" /></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { listKnowledgeFiles, delKnowledgeDetails, reparseKnowledge } from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope?: string
  filter?: 'all' | 'parse_failed' | 'extract_failed'
  docTypes?: Set<string>
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'changed'): void
}>()

const rows = ref<any[]>([])
const total = ref(0)
const loading = ref(false)
const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const hasMore = ref(true)
const listScrollRef = ref<HTMLElement>()
const activeRow = ref<any>(null)

// 列表网格列宽（复刻合同管理历史抽屉样式，末尾操作列）
const gridStyle = computed(() => ({
  gridTemplateColumns: 'minmax(0, 28%) minmax(0, 11%) minmax(0, 11%) minmax(0, 15%) minmax(0, 19%) 96px',
}))

const fileExt = (name: string) => {
  const m = String(name || '').match(/\.([^.]+)$/)
  return m ? m[1].toUpperCase() : 'FILE'
}
const fmtSize = (v: number) => {
  if (!v && v !== 0) return '--'
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`
  return `${(v / 1024 / 1024).toFixed(2)} MB`
}
const fmtTime = (v?: string) => (v ? v.replace('T', ' ').slice(0, 19) : '--')

const parseLabel = (s?: string) => {
  if (s === 'completed') return '已完成'
  if (s === 'parsing' || s === 'pending' || s === 'processing') return '解析中'
  if (s === 'failed') return '解析失败'
  return '待解析'
}
const parseTheme = (s?: string) => {
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'parsing' || s === 'pending' || s === 'processing') return 'warning'
  return 'default'
}
const extractStatus = (row: any) => {
  const m = row.custom_metadata || {}
  return m.extract_status || ''
}

const normalize = (r: any) => ({
  id: r.id,
  title: r.title || '',
  file_name: r.file_name || r.name || '',
  file_size: r.file_size || 0,
  parse_status: r.parse_status || '',
  created_at: r.created_at || '',
  custom_metadata: r.custom_metadata || {},
  doc_type: (r.custom_metadata || {}).doc_type || '',
})

const reload = async (p = 1) => {
  if (!props.kbId) return
  loading.value = true
  try {
    const res: any = await listKnowledgeFiles(props.kbId, { page: p, page_size: pageSize.value, keyword: keyword.value || undefined })
    let arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    // 已删除（隐藏）的历史条目不再显示
    arr = arr.filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
    const dtSet = props.docTypes
    if (dtSet && dtSet.size) arr = arr.filter((r: any) => { const dt = (r.custom_metadata || {}).doc_type; return dt ? dtSet.has(dt) : true })
    // 按概览卡片传入的状态筛选
    const f = props.filter || 'all'
    if (f === 'parse_failed') arr = arr.filter((r: any) => r.parse_status === 'failed')
    else if (f === 'extract_failed') arr = arr.filter((r: any) => (r.custom_metadata || {}).extract_status === 'failed')
    arr = arr.map(normalize).sort((a: any, b: any) => String(b.created_at).localeCompare(String(a.created_at)))
    rows.value = arr
    total.value = res?.total || arr.length
    page.value = res?.page || p
    hasMore.value = rows.value.length < total.value
    activeRow.value = null
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载上传历史失败')
  } finally {
    loading.value = false
  }
}

const loadMore = async () => {
  if (!hasMore.value || loading.value || !listScrollRef.value) return
  loading.value = true
  try {
    const next = page.value + 1
    const res: any = await listKnowledgeFiles(props.kbId, { page: next, page_size: pageSize.value, keyword: keyword.value || undefined })
    let arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    arr = arr.filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
    const dtSet2 = props.docTypes
    if (dtSet2 && dtSet2.size) arr = arr.filter((r: any) => { const dt = (r.custom_metadata || {}).doc_type; return dt ? dtSet2.has(dt) : true })
    const more = arr.map(normalize).sort((a: any, b: any) => String(b.created_at).localeCompare(String(a.created_at)))
    rows.value.push(...more)
    page.value = next
    hasMore.value = rows.value.length < (res?.total || 0)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载更多失败')
  } finally {
    loading.value = false
  }
}

const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 120) loadMore()
}

// ---- 失败原因 ----
const parseError = (row: any) => row.custom_metadata?.parse_error || row.custom_metadata?.error || ''
const extractError = (row: any) => row.custom_metadata?.extract_error || row.custom_metadata?.error || ''

// ---- 重新解析 ----
const reparseRow = async (row: any) => {
  row._reparsing = true
  try {
    await reparseKnowledge(row.id)
    MessagePlugin.success('已重新解析')
    reload(1)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    row._reparsing = false
  }
}

// ---- 重新提取 ----
const reextractRow = async (row: any) => {
  row._extracting = true
  try {
    const res = await fetch(`/api/v1/knowledge-bases/${props.kbId}/knowledge/${row.id}/extract-fleet-document`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + (localStorage.getItem('weknora_token') || '') },
      body: JSON.stringify({ scope: props.scope || 'vehicle', doc_type: row.doc_type || undefined }),
    })
    if (!res.ok) throw new Error('提取请求失败')
    MessagePlugin.success('已重新提取')
    reload(1)
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    row._extracting = false
  }
}

const confirmRemove = (row: any) => {
  const dlg = DialogPlugin.confirm({
    header: '删除文件',
    body: '确定从知识库删除该文件吗？已解析记录将一并清除。',
    confirmBtn: { content: '删除', theme: 'danger' },
    cancelBtn: '取消',
    onConfirm: async () => { dlg.destroy(); await removeOne(row) },
    onClose: () => dlg.destroy(),
  })
}

// ---- 删除：复用知识库文档删除逻辑（真删除，从知识库移除） ----
const removeOne = async (row: any) => {
  try {
    await delKnowledgeDetails(row.id)
    rows.value = rows.value.filter((r) => r.id !== row.id)
    total.value = Math.max(0, total.value - 1)
    MessagePlugin.success("已删除")
    emit("changed")
  } catch (e: any) {
    MessagePlugin.error(e?.message || "删除失败")
  }
}

const clearAll = async () => {
  try {
    let all: any[] = []
    let p = 1
    for (;;) {
      const res: any = await listKnowledgeFiles(props.kbId, { page: p, page_size: 100 })
      const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
      all.push(...arr)
      if (arr.length < 100) break
      p += 1
    }
    for (const row of all) await delKnowledgeDetails(row.id)
    rows.value = []
    total.value = 0
    hasMore.value = false
    MessagePlugin.success(`已删除 ${all.length} 个文件`)
    emit("changed")
  } catch (e: any) {
    MessagePlugin.error(e?.message || "删除失败")
  }
}

// ---- 抽屉宽度拖动（复用合同管理历史抽屉方案） ----
const DRAWER_MIN_WIDTH = 720
const DRAWER_MAX_WIDTH = 1200
const DRAWER_WIDTH_KEY = 'weknora-fleet-history-drawer-width'
const drawerWidth = ref(860)
let resizeStartX = 0
let resizeStartWidth = 0

function drawerMaxWidth() { return Math.min(DRAWER_MAX_WIDTH, Math.max(DRAWER_MIN_WIDTH, Math.floor(window.innerWidth * 0.95))) }
function clampDrawerWidth(w: number) { return Math.max(DRAWER_MIN_WIDTH, Math.min(drawerMaxWidth(), w)) }
function loadDrawerWidth() {
  try {
    const raw = localStorage.getItem(DRAWER_WIDTH_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isNaN(parsed)) drawerWidth.value = clampDrawerWidth(parsed)
  } catch { /* ignore */ }
}
function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartWidth = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}
function onResizeMove(e: MouseEvent) {
  const delta = resizeStartX - e.clientX
  drawerWidth.value = clampDrawerWidth(resizeStartWidth + delta)
}
function onResizeEnd() {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
}
loadDrawerWidth()
onBeforeUnmount(() => {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
})

const onClose = () => emit('update:visible', false)

watch(() => props.visible, (v) => {
  if (v) reload(1)
}, { immediate: true })
</script>

<style scoped>
.dh-wrap { position: relative; }
.dh-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2001;
  cursor: col-resize;
  display: flex;
  align-items: center;
  justify-content: center;
}
.dh-resize-line {
  width: 2px;
  height: 40px;
  border-radius: 1px;
  background: var(--td-brand-color);
  opacity: 0;
  transition: opacity 0.15s ease, height 0.15s ease;
}
.dh-resize-handle:hover .dh-resize-line { opacity: 1; height: 80px; }

.dh-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  height: 100%;
  overflow: hidden;
}
/* 列表 */
.dh-list-region {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.dh-toolbar { display: flex; gap: 8px; align-items: center; }
.dh-search { flex: 1; }
.dh-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary, #666);
}
/* ---- 列表（复刻合同管理历史抽屉样式：grid 自绘 + 懒加载滚动） ---- */
.doc-list-scroll {
  flex: 1 1 auto;
  min-height: 0;
  max-height: 100%;
  min-width: 0;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
}
.doc-list-header, .doc-list-row {
  display: grid;
  align-items: center;
  padding: 0 12px;
  min-width: 100%;
  box-sizing: border-box;
}
.doc-list-header {
  position: sticky;
  top: 0;
  z-index: 5;
  height: 40px;
  font-size: 12px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-bottom: 1px solid var(--td-component-stroke);
}
.doc-list-body { display: flex; flex-direction: column; }
.doc-list-row {
  position: relative;
  min-height: 52px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background-color 0.2s ease;
}
.doc-list-row:last-child { border-bottom: 0; }
.doc-list-row:hover:not(.selected) { background: var(--td-bg-color-secondarycontainer); }
.doc-list-row.selected { background: var(--td-brand-color-1); }
.cell {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  padding: 0 8px;
  text-align: center;
}
.cell-file {
  justify-content: flex-start;
  text-align: left;
}
.cell-del { padding: 0; }
.row-mono {
  font-variant-numeric: tabular-nums;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
}
.row-muted { color: var(--td-text-color-disabled, #bbb); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.dh-list-loading, .dh-list-empty {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 28px 0;
}
.dh-file { display: flex; align-items: center; gap: 8px; min-width: 0; }
.dh-file-icon {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 6px;
  background: var(--td-brand-color-light, #e8f3ff);
  color: var(--td-brand-color, #0052d9);
  font-size: 10px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  letter-spacing: 0.5px;
}
.dh-file-meta { min-width: 0; }
.dh-file-name {
  font-size: 13px;
  color: var(--td-text-color-primary, #333);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-file-sub {
  font-size: 12px;
  color: var(--td-text-color-secondary, #888);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
