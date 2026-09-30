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
          <div ref="listScrollRef" class="doc-list-scroll" @scroll="onListScroll">
            <div class="doc-list-view">
              <t-table
              :data="rows"
              :columns="columns"
              row-key="id"
              size="small"
              :hover="true"
              :loading="loading"
              max-height="100%"
              class="dh-table"
              :row-class-name="({ row }: any) => activeRow?.id === row.id ? 'is-selected' : ''"
              @row-click="({ row }: any) => (activeRow = row)"
            >
              <template #file="{ row }: any">
                <div class="dh-file">
                  <span class="dh-file-icon">{{ fileExt(row.file_name) }}</span>
                  <span class="dh-file-name" :title="`${row.title || row.file_name} · ${fmtSize(row.file_size)}`">
                    {{ row.title || row.file_name }}<span class="dh-file-size"> · {{ fmtSize(row.file_size) }}</span>
                  </span>
                </div>
              </template>
              <template #parse="{ row }: any">
                <t-tooltip v-if="row.parse_status === 'failed'" :content="parseError(row)" placement="top">
                  <t-tag size="small" theme="danger" variant="light-outline">解析失败</t-tag>
                </t-tooltip>
                <t-tag v-else size="small" :theme="parseTheme(row.parse_status)" variant="light-outline">{{ parseLabel(row.parse_status) }}</t-tag>
              </template>
              <template #extract="{ row }: any">
                <t-tooltip v-if="extractStatus(row) === 'failed'" :content="extractError(row)" placement="top">
                  <t-tag size="small" theme="danger" variant="light-outline">提取失败</t-tag>
                </t-tooltip>
                <t-tag v-else size="small" :theme="extractTheme(row)" variant="light-outline">{{ extractLabel(row) }}</t-tag>
              </template>
              <template #docType="{ row }: any">
                <span v-if="row.doc_type" class="row-mono">{{ row.doc_type }}</span>
                <span v-else class="row-muted">—</span>
              </template>
              <template #createdAt="{ row }: any">
                <span class="row-mono">{{ fmtTime(row.created_at) }}</span>
              </template>
              <template #op="{ row }: any">
                <t-popconfirm theme="warning" :visible="delPopRow?.id === row.id" placement="left"
                  :content="'确定从知识库删除该文件吗？已解析记录将一并清除。'"
                  :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
                  @confirm="onPopRemove" @cancel="delPopRow = null"
                  @visible-change="(v: boolean) => { if (!v) delPopRow = null }">
                  <t-dropdown :options="rowMenuOptions(row)" placement="bottom-right" min-column-width="120px"
                    @click="(ctx: any) => onRowMenu(row, ctx.value)">
                    <t-button variant="text" size="small" shape="square" class="row-more-btn">
                      <template #icon><t-icon name="more" size="16px" /></template>
                    </t-button>
                  </t-dropdown>
                </t-popconfirm>
              </template>
            </t-table>
          </div>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, h } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { RefreshIcon, CloudUploadIcon, DownloadIcon, DeleteIcon } from 'tdesign-icons-vue-next'
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
const columns = [
  { colKey: 'file', title: '文件', ellipsis: true },
  { colKey: 'parse', title: '解析', width: '100px' },
  { colKey: 'extract', title: '提取', width: '100px' },
  { colKey: 'docType', title: props.scope === 'invoice' ? '发票类型' : '证照类型', width: '120px' },
  { colKey: 'createdAt', title: '上传时间', width: '160px' },
  { colKey: 'op', title: '操作', width: '60px' },
]

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
// 提取状态文案与解析状态同口径：success→已完成 / failed→提取失败 / 处理中→提取中 / 其他→待提取
const extractLabel = (row: any) => {
  const s = extractStatus(row)
  if (s === 'success') return '已完成'
  if (s === 'failed') return '提取失败'
  if (s === 'parsing' || s === 'pending' || s === 'processing') return '提取中'
  return '待提取'
}
const extractTheme = (row: any) => {
  const s = extractStatus(row)
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'parsing' || s === 'pending' || s === 'processing') return 'warning'
  return 'default'
}

// 发票类型取值：优先上传时写入的 fleet_cert_type，其次 custom_metadata.invoice_type，
// 再次发票提取结果 invoices[0].invoice_type，最后回退 doc_type/顶层字段（补齐历史遗留空白）。
const invDocType = (r: any) => {
  const m = r.custom_metadata || {}
  if (m.fleet_cert_type) return m.fleet_cert_type
  if (m.invoice_type) return m.invoice_type
  const invs = Array.isArray(m.invoices) ? m.invoices : []
  if (invs.length && invs[0]?.invoice_type) return invs[0].invoice_type
  return m.doc_type || r.doc_type || ''
}

const normalize = (r: any) => ({
  id: r.id,
  title: r.title || '',
  file_name: r.file_name || r.name || '',
  file_size: r.file_size || 0,
  parse_status: r.parse_status || '',
  created_at: r.created_at || '',
  custom_metadata: r.custom_metadata || {},
  doc_type: invDocType(r),
})

const rowDocType = (r: any) => invDocType(r)

const reload = async (p = 1) => {
  if (!props.kbId) return
  loading.value = true
  try {
    const res: any = await listKnowledgeFiles(props.kbId, { page: p, page_size: pageSize.value, keyword: keyword.value || undefined })
    let arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    // 已删除（隐藏）的历史条目不再显示
    arr = arr.filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
    const dtSet = props.docTypes
    if (dtSet && dtSet.size) arr = arr.filter((r: any) => { const dt = rowDocType(r); return dt ? dtSet.has(dt) : true })
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
    if (dtSet2 && dtSet2.size) arr = arr.filter((r: any) => { const dt = rowDocType(r); return dt ? dtSet2.has(dt) : true })
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
    const url = props.scope === 'invoice'
      ? `/api/v1/knowledge-bases/${props.kbId}/knowledge/${row.id}/extract-business`
      : `/api/v1/knowledge-bases/${props.kbId}/knowledge/${row.id}/extract-fleet-document`
    const res = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + (localStorage.getItem('weknora_token') || '') },
      body: props.scope === 'invoice'
        ? JSON.stringify({ scope: 'invoice' })
        : JSON.stringify({ scope: props.scope || 'vehicle', doc_type: row.doc_type || undefined }),
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

const delPopRow = ref<any>(null)
const onPopRemove = async () => {
  const row = delPopRow.value
  delPopRow.value = null
  if (row) await removeOne(row)
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

// ---- 行操作三点菜单 ----
const IMG_EXT = ['jpg', 'jpeg', 'png', 'bmp', 'webp', 'gif', 'tif', 'tiff']
const isImg = (name: string) => IMG_EXT.includes(fileExt(name).toLowerCase())

const rowMenuOptions = (row: any) => {
  const opts: any[] = [
    { content: '重新解析', value: 'reparse', prefixIcon: () => h(RefreshIcon, { size: '14px' }) },
    { content: '重新提取', value: 'reextract', prefixIcon: () => h(CloudUploadIcon, { size: '14px' }) },
    { content: '下载源文件', value: 'download', prefixIcon: () => h(DownloadIcon, { size: '14px' }) },
  ]
  opts.push({ content: '删除', value: 'delete', theme: 'error', prefixIcon: () => h(DeleteIcon, { size: '14px' }) })
  return opts
}

const authHeaders = () => ({ Authorization: 'Bearer ' + (localStorage.getItem('weknora_token') || '') })

const fetchBlob = async (row: any) => {
  const res = await fetch(`/api/v1/knowledge/${row.id}/download`, { headers: authHeaders() })
  if (!res.ok) throw new Error('文件获取失败')
  return res.blob()
}

const downloadSource = async (row: any) => {
  try {
    const blob = await fetchBlob(row)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = row.file_name || 'source'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '下载失败')
  }
}

const onRowMenu = (row: any, value: string) => {
  if (value === 'reparse') reparseRow(row)
  else if (value === 'reextract') reextractRow(row)
  else if (value === 'download') downloadSource(row)
  else if (value === 'delete') delPopRow.value = row
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
.row-muted { color: var(--td-text-color-disabled); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  font-size: 10px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  letter-spacing: 0.5px;
}
.dh-file-name {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--td-text-color-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-file-size {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}
</style>
