<template>
  <div class="dh-wrap">
    <div v-if="visible" class="dh-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="dh-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" header="删除历史" :size="`${drawerWidth}px`" :footer="false"
      class="deleted-history-drawer" @close="onClose" @update:visible="(v: boolean) => (v || onClose())">
      <div class="dh-body">
        <!-- 上半：列表（懒加载 + 垂直滚动） -->
        <div class="dh-list-region">
          <div class="dh-toolbar">
            <t-input v-model="keyword" placeholder="搜索文件名 / 标题" clearable class="dh-search"
              @enter="reload(1)" @clear="reload(1)">
              <template #prefix-icon><t-icon name="search" /></template>
            </t-input>
            <t-button theme="default" variant="outline" @click="reload(1)">
              <template #icon><t-icon name="search" size="14px" /></template>
              搜索
            </t-button>
          </div>
          <div class="dh-hint">
            系统自动删除（判定非{{ moduleName || '该类文档' }}）的记录，源文件已保留，可重新入库或永久删除。
          </div>
          <div ref="listScrollRef" class="doc-list-scroll" @scroll="onListScroll">
            <div class="doc-list-view">
              <div class="doc-list-header" :style="gridStyle" role="row">
                <div class="cell cell-file" role="columnheader">文件</div>
                <div class="cell" role="columnheader">删除原因</div>
                <div class="cell" role="columnheader">删除时间</div>
                <div class="cell" role="columnheader">次数</div>
                <div class="cell cell-op" role="columnheader">操作</div>
              </div>
              <div class="doc-list-body">
                <div v-for="row in rows" :key="row.id" class="doc-list-row" :style="gridStyle"
                  :class="{ selected: activeRow?.id === row.id }" @click="activeRow = row">
                  <div class="cell cell-file">
                    <div class="dh-file">
                      <span class="dh-file-icon">{{ fileExt(row.fileName) }}</span>
                      <div class="dh-file-meta">
                        <div class="dh-file-name" :title="row.title || row.fileName">{{ row.title || row.fileName }}</div>
                        <div class="dh-file-sub">{{ row.fileName }} · {{ fmtSize(row.fileSize) }}</div>
                      </div>
                    </div>
                  </div>
                  <div class="cell">
                    <t-tag size="small" theme="warning" variant="light-outline">{{ row.reason || '自动删除' }}</t-tag>
                  </div>
                  <div class="cell"><span class="row-mono">{{ row.deletedAt }}</span></div>
                  <div class="cell">
                    <span v-if="(row.deleteCount || 1) > 1" class="dh-count-badge">{{ row.deleteCount }} 次</span>
                    <span v-else class="row-muted">1 次</span>
                  </div>
                  <div class="cell cell-op" @click.stop>
                    <t-dropdown :min-column-width="120" @click="(d: any) => onMenuClick(d, row)">
                      <t-button variant="text" size="small" shape="square">
                        <template #icon><t-icon name="more" size="16px" /></template>
                      </t-button>
                      <template #dropdown>
                        <t-dropdown-menu>
                          <t-dropdown-item value="restore">
                            <span class="dh-menu-item"><t-icon name="rollback" size="14px" class="dh-menu-icon dh-menu-icon--restore" />重新入库</span>
                          </t-dropdown-item>
                          <t-dropdown-item value="purge">
                            <span class="dh-menu-item"><t-icon name="delete" size="14px" class="dh-menu-icon dh-menu-icon--purge" />永久删除</span>
                          </t-dropdown-item>
                        </t-dropdown-menu>
                      </template>
                    </t-dropdown>
                  </div>
                </div>
                <div v-if="loading" class="dh-list-loading"><t-loading size="small" text="加载中..." /></div>
                <div v-if="!loading && !rows.length" class="dh-list-empty"><t-empty title="暂无删除历史" /></div>
              </div>
            </div>
          </div>
        </div>

        <!-- 下半：源文件预览（点击行加载，固定约 50% 高度） -->
        <div class="dh-preview-region">
          <div class="dh-preview-head">
            <span class="dh-preview-title">{{ previewTitle }}</span>
            <t-button variant="outline" size="small" :disabled="!activeRow" :loading="downloadLoading"
              @click="downloadPreview">
              <template #icon><t-icon name="download" size="14px" /></template>
              下载源文件
            </t-button>
          </div>
          <div class="dh-preview-file">
            <div v-if="!activeRow" class="dh-preview-empty">
              <t-empty description="点击上方记录查看源文件预览" />
            </div>
            <div v-else-if="previewLoading" class="dh-preview-loading"><t-loading size="small" text="加载预览..." /></div>
            <template v-else>
              <iframe v-if="previewKind === 'pdf'" :src="previewUrl" class="dh-preview-frame" />
              <img v-else-if="previewKind === 'image'" :src="previewUrl" class="dh-preview-img" />
              <div v-else class="dh-preview-text">
                <t-empty description="该类型文件暂不支持内嵌预览，可下载查看" />
              </div>
            </template>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { listDeletedKnowledge, restoreDeletedKnowledge, purgeDeletedKnowledge, previewDeletedKnowledgeFile } from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  moduleName?: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'changed'): void
  (e: 'restored', knowledgeId: string): void
}>()

const rows = ref<any[]>([])
const total = ref(0)
const loading = ref(false)
const loadingMore = ref(false)
const keyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const hasMore = ref(true)
const listScrollRef = ref<HTMLElement>()

// 列表网格列宽（复刻合同管理列表样式）
const gridStyle = computed(() => ({
  gridTemplateColumns: 'minmax(0, 30%) minmax(0, 22%) minmax(0, 20%) minmax(0, 12%) minmax(0, 16%)',
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

const reload = async (p = 1) => {
  if (!props.kbId) return
  loading.value = true
  try {
    const res: any = await listDeletedKnowledge(props.kbId, { page: p, page_size: pageSize.value, q: keyword.value || undefined })
    rows.value = (res?.data || []).map((r: any) => ({ ...r, fileName: r.file_name, fileSize: r.file_size, deletedAt: fmtTime(r.deleted_at), createdAt: fmtTime(r.created_at), deleteCount: r.delete_count }))
    total.value = res?.total || 0
    page.value = res?.page || p
    hasMore.value = rows.value.length < total.value
    // 列表重载后不自动加载预览，等待用户点击记录后再加载
    activeRow.value = null
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载删除历史失败')
  } finally {
    loading.value = false
  }
}

const loadMore = async () => {
  if (!hasMore.value || loading.value || loadingMore.value) return
  loadingMore.value = true
  try {
    const next = page.value + 1
    const res: any = await listDeletedKnowledge(props.kbId, { page: next, page_size: pageSize.value, q: keyword.value || undefined })
    const more = (res?.data || []).map((r: any) => ({ ...r, fileName: r.file_name, fileSize: r.file_size, deletedAt: fmtTime(r.deleted_at), createdAt: fmtTime(r.created_at), deleteCount: r.delete_count }))
    rows.value.push(...more)
    page.value = next
    hasMore.value = rows.value.length < (res?.total || 0)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载更多失败')
  } finally {
    loadingMore.value = false
  }
}

const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 120) loadMore()
}

// ---- 源文件预览（点击行自动加载） ----
const activeRow = ref<any>(null)
const previewUrl = ref('')
const previewKind = ref<'pdf' | 'image' | 'other'>('other')
const previewLoading = ref(false)
const downloadLoading = ref(false)
const previewTitle = computed(() => activeRow.value ? (activeRow.value.title || activeRow.value.fileName || '源文件预览') : '源文件预览')

const loadPreview = async (row: any) => {
  previewLoading.value = true
  if (previewUrl.value) URL.revokeObjectURL(previewUrl.value)
  previewUrl.value = ''
  previewKind.value = 'other'
  try {
    const blob: any = await previewDeletedKnowledgeFile(props.kbId, row.id)
    if (!blob) throw new Error('获取源文件失败')
    const type = String(blob?.type || '').toLowerCase()
    const ext = String(row.fileName || '').toLowerCase().match(/\.([^.]+)$/)?.[1] || ''
    previewKind.value = type.includes('pdf') || ext === 'pdf' ? 'pdf'
      : (type.startsWith('image/') || ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff'].includes(ext)) ? 'image' : 'other'
    previewUrl.value = URL.createObjectURL(blob)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '预览失败')
  } finally {
    previewLoading.value = false
  }
}

watch(activeRow, (row) => {
  if (row) loadPreview(row)
})

const downloadPreview = async () => {
  const row = activeRow.value
  if (!row) return
  downloadLoading.value = true
  try {
    const blob: any = await previewDeletedKnowledgeFile(props.kbId, row.id)
    if (!blob) throw new Error('获取源文件失败')
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = row.fileName || row.title || 'source'
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 2000)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '下载失败')
  } finally {
    downloadLoading.value = false
  }
}

// ---- 操作（三点菜单） ----
const onMenuClick = (data: any, row: any) => {
  // t-dropdown 的 @click 第一个参数即菜单项 value（字符串）
  const v = typeof data === 'string' ? data : data?.value
  if (v === 'restore') confirmRestore(row)
  else if (v === 'purge') confirmPurge(row)
}

const confirmRestore = (row: any) => {
  let dlg: any = null
  dlg = DialogPlugin.confirm({
    header: '重新入库',
    body: `恢复「${row.fileName}」？恢复后将在列表中显示，可在编辑抽屉中补录字段。`,
    confirmBtn: { content: '重新入库', theme: 'primary' },
    cancelBtn: '取消',
    onConfirm: async () => {
      try {
        await restoreDeletedKnowledge(props.kbId, row.id)
        MessagePlugin.success(`「${row.fileName}」已重新入库，请在列表中编辑补录字段`)
        emit('changed')
        emit('restored', row.id)
        onClose()
      } catch (e: any) {
        MessagePlugin.error(e?.message || '重新入库失败')
      } finally {
        dlg?.destroy()
      }
    },
  })
}

const confirmPurge = (row: any) => {
  let dlg: any = null
  dlg = DialogPlugin.confirm({
    header: '永久删除',
    body: `永久删除「${row.fileName}」？源文件将一并删除，不可恢复。`,
    confirmBtn: { content: '永久删除', theme: 'danger' },
    cancelBtn: '取消',
    onConfirm: async () => {
      try {
        await purgeDeletedKnowledge(props.kbId, row.id)
        MessagePlugin.success('已永久删除')
        if (activeRow.value?.id === row.id) activeRow.value = null
        reload(1)
      } catch (e: any) {
        MessagePlugin.error(e?.message || '永久删除失败')
      } finally {
        dlg?.destroy()
      }
    },
  })
}

// ---- 抽屉宽度拖动（复用发票抽屉方案） ----
const DRAWER_MIN_WIDTH = 720
const DRAWER_MAX_WIDTH = 1200
const DRAWER_WIDTH_KEY = 'weknora-deleted-history-drawer-width'
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
/* 上半：列表 */
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
/* ---- 列表（复刻合同管理列表样式：grid 自绘 + 懒加载滚动） ---- */
.doc-list-scroll {
  flex: 0 1 auto; /* 高度随内容自适应：1 条就包 1 条，多条向下扩展 */
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
  justify-content: center; /* 表头/内容居中 */
  min-width: 0;
  padding: 0 8px;
  text-align: center;
}
.cell-file {
  justify-content: flex-start;
  text-align: left;
}
.cell-op { justify-content: flex-end; }
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
.dh-count-badge { color: var(--td-warning-color, #e37318); font-weight: 600; }
.dh-count-once { color: var(--td-text-color-secondary, #888); }
.dh-menu-item { display: inline-flex; align-items: center; gap: 6px; }
.dh-menu-icon { flex-shrink: 0; }
.dh-menu-icon--restore { color: var(--td-brand-color, #0052d9); }
.dh-menu-icon--purge { color: var(--td-error-color, #d54941); }

/* 下半：预览 */
.dh-preview-region {
  flex: 0 0 50%;
  min-height: 200px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 10px;
}
.dh-preview-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.dh-preview-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary, #333);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-preview-file {
  flex: 1;
  min-height: 0;
  border: 1px solid var(--td-component-border, #e7e7e7);
  border-radius: 8px;
  background: var(--td-bg-color-container, #fff);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
.dh-preview-empty, .dh-preview-loading, .dh-preview-text {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.dh-preview-frame { flex: 1; width: 100%; border: none; }
.dh-preview-img { flex: 1; width: 100%; object-fit: contain; }
</style>
