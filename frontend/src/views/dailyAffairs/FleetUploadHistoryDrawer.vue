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
          <!-- 状态统计筛选小卡片：计数始终是「全量文件」口径，不受搜索/日期影响 -->
          <div class="dh-status-cards">
            <button v-for="c in statusCards" :key="c.key" type="button" class="dh-status-card"
              :class="[`dh-status-card--${c.key}`, { 'is-active': statusFilter === c.key }]"
              :data-status="c.key" :title="c.title" @click="onStatusCardClick(c.key)">
              <span class="dh-status-card__label">{{ c.label }}</span>
              <span class="dh-status-card__num">{{ c.count }}</span>
            </button>
          </div>
          <div class="dh-toolbar">
            <!-- 即输即搜（300ms 防抖）+ 一键清空，不再单独放「搜索」按钮 -->
            <t-input v-model="keywordInput" placeholder="搜索文件名" clearable class="dh-search"
              @clear="onKeywordClear">
              <template #prefix-icon><t-icon name="search" /></template>
            </t-input>
            <!-- 发票类型筛选：默认「全部分类」，选项来自数据里真实出现的分类 -->
            <t-select v-model="typeFilter" class="dh-type-select" :options="typeOptions"
              :popup-props="{ attach: 'body' }" @change="onTypeChange" />
            <!-- 占位符必须用数组形式：单字符串会被当成两端的同一文案，
                 渲染成「上传开始日期 - ... - 上传开始日期 - ...」 -->
            <t-date-range-picker v-model="dateRange" class="dh-date-range" clearable
              value-type="YYYY-MM-DD" :placeholder="['开始日期', '结束日期']"
              @change="onDateRangeChange" />
          </div>
          <div ref="listScrollRef" class="doc-list-scroll" @scroll="onListScroll">
            <div class="doc-list-view">
              <t-table
              :data="rows"
              :columns="columns"
              row-key="id"
              size="small"
              :hover="true"
              max-height="100%"
              class="dh-table"
              :row-class-name="({ row }: any) => activeRow?.id === row.id ? 'is-selected' : ''"
              @row-click="({ row }: any) => (activeRow = row)"
            >
              <template #file="{ row }: any">
                <!-- 双击文件名 → 全屏预览源文件（历史记录里直接大图核对原件） -->
                <div class="dh-file dh-file--clickable" @dblclick="openFullPreview(row)"
                  :title="`${row.title || row.file_name} · ${fmtSize(row.file_size)} · 双击全屏预览源文件`">
                  <span class="dh-file-icon">{{ fileExt(row.file_name) }}</span>
                  <span class="dh-file-name">
                    {{ row.title || row.file_name }}<span class="dh-file-size"> · {{ fmtSize(row.file_size) }}</span>
                  </span>
                </div>
              </template>
              <template #parse="{ row }: any">
                <t-loading v-if="row._reparsing" size="small" text="解析中" />
                <t-tag v-else-if="isParseBusy(row)" size="small" theme="primary" variant="light-outline">
                  <template #icon><t-icon name="loading" spin size="12px" /></template>
                  {{ parseBusyLabel(row) }}
                </t-tag>
                <t-tooltip v-else-if="row.parse_status === 'failed'"
                  :content="formatExtractErrorMessage(parseError(row)) || '解析失败'" placement="top">
                  <t-tag size="small" theme="danger" variant="light-outline">解析失败</t-tag>
                </t-tooltip>
                <t-tag v-else size="small" :theme="parseTheme(row.parse_status)" variant="light-outline">{{ parseLabel(row.parse_status) }}</t-tag>
              </template>
              <template #extract="{ row }: any">
                <t-loading v-if="row._extracting" size="small" text="提取中" />
                <t-tag v-else-if="isExtractBusy(row)" size="small" theme="primary" variant="light-outline">
                  <template #icon><t-icon name="loading" spin size="12px" /></template>
                  提取中
                </t-tag>
                <t-tooltip v-else-if="extractStatus(row) === 'failed'"
                  :content="formatExtractErrorMessage(extractError(row)) || '提取失败'" placement="top">
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

    <!-- 双击文件名 → 全屏源文件预览（历史记录里直接大图核对原件） -->
    <t-dialog v-model:visible="previewVisible" :header="previewTitle" :footer="false" width="92vw"
      placement="center" destroy-on-close class="dh-full-preview" @close="closeFullPreview">
      <div class="dh-full-preview__body">
        <DocumentPreview v-if="previewVisible && previewRow" :knowledge-id="previewRow.id"
          :file-type="previewFileType" :file-name="previewFileName" :active="true" fill-height />
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, h } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { RefreshIcon, CloudUploadIcon, DownloadIcon, DeleteIcon } from 'tdesign-icons-vue-next'
import { listKnowledgeFiles, delKnowledgeDetails, reparseKnowledge } from '@/api/knowledge-base'
import { extractFleetDocument, extractBusinessDocument } from '@/api/fleet'
import { isExtractInProgress } from '@/composables/extractConflict'
import { getScope } from '../../registry'
import DocumentPreview from '@/components/document-preview.vue'
import { formatExtractErrorMessage } from './extractErrorMessage'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope?: string
  filter?: 'all' | 'parse_failed' | 'extract_failed'
  docTypes?: Set<string>
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  // payload.deletedIds：本次异步删除命中的 knowledge id（删除为 asynq 异步入队，
  // 父组件可据此先做乐观移除，再延迟权威对账）；重新提取等场景不带 payload。
  (e: 'changed', payload?: { deletedIds?: string[] }): void
}>()

// 全量快照：抽屉一次拉全（上限 SNAPSHOT_CAP），搜索/日期/状态三种过滤全部在本地做。
// 为什么不做服务端分页：① 状态卡的计数必须是「全量口径」，分页数据算不准；
// ② 即输即搜要求零网络等待；③ 历史记录是低频管理面板，单次拉全可接受。
const SNAPSHOT_CAP = 2000
const DEFAULT_PAGE_SIZE = 20
const allRows = ref<any[]>([])
const loading = ref(false)
// keywordInput 是用户正在输入的值；keyword 是 300ms 防抖后真正参与过滤的值
const keywordInput = ref('')
const keyword = ref('')
const dateRange = ref<string[]>([])
// 发票类型筛选：空串 = 全部分类（默认，不筛掉任何文件）
const typeFilter = ref('')
const statusFilter = ref<'all' | 'running' | 'abnormal'>('all')
const visibleCount = ref(DEFAULT_PAGE_SIZE)
const listScrollRef = ref<HTMLElement>()
const activeRow = ref<any>(null)
let keywordTimer: ReturnType<typeof setTimeout> | null = null

const onKeywordClear = () => {
  keywordInput.value = ''
  if (keywordTimer) clearTimeout(keywordTimer)
  keyword.value = ''
  resetPaging()
}
const onDateRangeChange = () => resetPaging()
const onTypeChange = () => resetPaging()
const onStatusCardClick = (key: 'all' | 'running' | 'abnormal') => {
  statusFilter.value = key
  resetPaging()
}
const resetPaging = () => {
  visibleCount.value = DEFAULT_PAGE_SIZE
}
const columns = [
  { colKey: 'file', title: '文件', ellipsis: true },
  { colKey: 'parse', title: '解析', width: '84px' },
  { colKey: 'extract', title: '提取', width: '84px' },
  { colKey: 'docType', title: getScope(props.scope || '')?.categoryLabel || '证照类型', width: '110px' },
  { colKey: 'createdAt', title: '上传时间', width: '160px' },
  { colKey: 'op', title: '操作', width: '48px' },
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
// 解析进行中判定（含 pending/processing/finalizing），用于渲染与列表同款的
// primary + loading spin 动态标签（列表「解析中」口径一致）
const isParseBusy = (row: any) => {
  const s = row.parse_status || ''
  return s === 'pending' || s === 'processing' || s === 'finalizing'
}
const parseBusyLabel = (row: any) => (row.parse_status === 'finalizing' ? '定稿中' : '解析中')
// 提取进行中判定：文件已完成解析但提取状态仍为 pending/processing
const isExtractBusy = (row: any) => {
  const s = extractStatus(row)
  return s === 'pending' || s === 'processing'
}
const parseTheme = (s?: string) => {
  if (s === 'completed') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'parsing' || s === 'pending' || s === 'processing') return 'primary'
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
  if (s === 'parsing' || s === 'pending' || s === 'processing') return 'primary'
  return 'default'
}

// 发票类型取值：优先上传时写入的 fleet_cert_type，其次 custom_metadata.invoice_type，
// 再次发票提取结果 invoices[0].invoice_type，然后 invoices[0].category（细分分类打标），
// 最后回退 doc_type/顶层字段（补齐历史遗留空白）。
// 注意：「其它票据」是未归类兜底值，不得作为 cert_type 锚点覆盖已识别出的细分分类
// （如通行费发票），故取值时让兜底大类穿透，优先返回第一个有效细分分类。
const FALLBACK_DOC_TYPE = '其它票据'
const invDocType = (r: any) => {
  const m = r.custom_metadata || {}
  const invs = Array.isArray(m.invoices) ? m.invoices : []
  const i0 = (invs.length ? invs[0] : {}) as Record<string, any>
  const candidates = [m.fleet_cert_type, m.invoice_type, i0.invoice_type, i0.category, m.doc_type, r.doc_type]
  for (const v of candidates) {
    if (v && v !== FALLBACK_DOC_TYPE) return v
  }
  // 全部为空或仅剩兜底值时，才回退到「其它票据」（若确实存在）
  if (m.invoice_type === FALLBACK_DOC_TYPE || i0.invoice_type === FALLBACK_DOC_TYPE) return FALLBACK_DOC_TYPE
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

// 状态归类（唯一口径）：解析中/提取中 → running；解析失败/提取失败 → abnormal。
const ROW_PARSE_RUNNING = new Set(['pending', 'processing', 'parsing', 'finalizing'])
const ROW_EXTRACT_RUNNING = new Set(['pending', 'processing', 'parsing'])
const isRowRunning = (r: any) => {
  const ps = String(r.parse_status || '')
  if (ROW_PARSE_RUNNING.has(ps)) return true
  if (ps !== 'completed') return false
  const es = String(extractStatus(r) || '')
  // parse 已完成但「从未提取/待提取/提取中」也算正在执行：
  // 主列表这几行显示「待提取」，历史记录若不计入，两边数量就对不上
  // （真机：主列表 4 行待提取，历史记录「正在执行 0」）。
  return !es || ROW_EXTRACT_RUNNING.has(es)
}
const isRowAbnormal = (r: any) =>
  String(r.parse_status || '') === 'failed' || String(extractStatus(r) || '') === 'failed'

// 文件归属的发票分类：上传即持久化的 invoice_category 优先，历史字段兜底。
const invoiceTypeOf = (r: any): string => {
  const m = r.custom_metadata || {}
  return String(m.invoice_category || m.fleet_cert_type || '').trim()
}

// 发票类型下拉选项：全部分类 + 数据里真实出现的分类（按出现次数降序）。
// 不从分类字典接口取，是因为历史文件可能挂着已停用的旧分类名——只列实际存在的
// 才能保证「选了就一定筛得出数据」。
const typeOptions = computed(() => {
  const counts = new Map<string, number>()
  for (const r of allRows.value) {
    const t = invoiceTypeOf(r)
    if (!t) continue
    counts.set(t, (counts.get(t) || 0) + 1)
  }
  const rest = [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([value, n]) => ({ label: `${value}（${n}）`, value }))
  return [{ label: '全部分类', value: '' }, ...rest]
})

// 状态卡（计数基于全量快照，与搜索/日期条件无关——卡片表达的是「库里有多少」）
const statusCards = computed(() => {
  const all = allRows.value
  return [
    { key: 'all' as const, label: '全部文件', count: all.length, title: '全部已上传文件' },
    { key: 'running' as const, label: '正在执行', count: all.filter(isRowRunning).length, title: '解析中 / 提取中的文件' },
    { key: 'abnormal' as const, label: '异常文件', count: all.filter(isRowAbnormal).length, title: '解析失败 / 提取失败的文件' },
  ]
})

// 关键词 + 上传时间 + 状态 三者 AND 组合过滤（全部本地，即输即生效）
const filteredRows = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  const [from, to] = dateRange.value || []
  return allRows.value.filter((r: any) => {
    if (statusFilter.value === 'running' && !isRowRunning(r)) return false
    if (statusFilter.value === 'abnormal' && !isRowAbnormal(r)) return false
    if (typeFilter.value && invoiceTypeOf(r) !== typeFilter.value) return false
    // 上传时间（created_at）区间：按自然日包含两端
    if (from || to) {
      const day = String(r.created_at || '').slice(0, 10)
      if (!day) return false
      if (from && day < String(from).slice(0, 10)) return false
      if (to && day > String(to).slice(0, 10)) return false
    }
    if (kw) {
      const hay = [r.title, r.file_name, r.doc_type].map((x: string) => String(x || '').toLowerCase())
      if (!hay.some((x: string) => x.includes(kw))) return false
    }
    return true
  })
})
const rows = computed(() => filteredRows.value.slice(0, visibleCount.value))
const hasMore = computed(() => rows.value.length < filteredRows.value.length)

// 拉全量快照（一次拉全，供计数与本地过滤）
const reload = async () => {
  if (!props.kbId) return
  loading.value = true
  try {
    const collected: any[] = []
    let p = 1
    // 逐页拉全，直到拿满 total 或触到上限（防超大知识库拖垮抽屉）
    for (;;) {
      const res: any = await listKnowledgeFiles(props.kbId, { page: p, page_size: 200 })
      const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
      collected.push(...arr)
      const totalCnt = Number(res?.total || 0)
      if (arr.length === 0 || collected.length >= totalCnt || collected.length >= SNAPSHOT_CAP) break
      p++
    }
    let arr = collected.filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
    const dtSet = props.docTypes
    if (dtSet && dtSet.size) arr = arr.filter((r: any) => { const dt = rowDocType(r); return dt ? dtSet.has(dt) : true })
    // 兼容父级传入的固定筛选（保留原有对接约定）
    const f = props.filter || 'all'
    if (f === 'parse_failed') arr = arr.filter((r: any) => r.parse_status === 'failed')
    else if (f === 'extract_failed') arr = arr.filter((r: any) => (r.custom_metadata || {}).extract_status === 'failed')
    allRows.value = arr.map(normalize).sort((a: any, b: any) => String(b.created_at).localeCompare(String(a.created_at)))
    resetPaging()
    activeRow.value = null
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载上传历史失败')
  } finally {
    loading.value = false
  }
}

// 本地分页：滚动到底继续放出下一批（不重新请求）
const loadMore = () => {
  if (!hasMore.value || loading.value) return
  visibleCount.value += DEFAULT_PAGE_SIZE
}

const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 120) loadMore()
}

// 关键词 300ms 防抖：输入停止后才应用过滤（渲染开销大时避免逐字卡顿）
watch(keywordInput, (v) => {
  if (keywordTimer) clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => {
    keyword.value = v
    resetPaging()
  }, 300)
})
onBeforeUnmount(() => { if (keywordTimer) clearTimeout(keywordTimer) })

// ---- 失败原因（展示前统一转译为中文短句，不透出英文栈/裸 JSON） ----
const parseError = (row: any) => row.custom_metadata?.parse_error || row.custom_metadata?.error || ''
const extractError = (row: any) => row.custom_metadata?.extract_error || row.custom_metadata?.error || ''

// ---- 双击文件名：全屏预览源文件 ----
const previewVisible = ref(false)
const previewRow = ref<any>(null)
const previewTitle = computed(() => previewRow.value?.title || previewRow.value?.file_name || '源文件预览')
const previewFileName = computed(() => previewRow.value?.file_name || previewRow.value?.title || '')
const previewFileType = computed(() => {
  const row = previewRow.value
  if (!row) return ''
  if (row.file_type) return row.file_type
  const m = String(row.file_name || '').match(/\.([^.]+)$/)
  return m ? m[1].toLowerCase() : ''
})
// 双击文件名打开全屏预览：历史记录场景下用户只想核对原件，不需要先回列表再点开详情抽屉
function openFullPreview(row: any) {
  if (!row?.id) return
  previewRow.value = row
  previewVisible.value = true
}
function closeFullPreview() {
  previewVisible.value = false
  previewRow.value = null
}

// ---- 重新解析 ----
const reparseRow = async (row: any) => {
  row._reparsing = true
  try {
    await reparseKnowledge(row.id)
    // 后端 reparse 为异步任务：接口 200 只代表任务已提交，此时 parse_status 仍是旧值（如 failed）。
    // ① 乐观置为 pending（解析中），点击后立即反馈；
    // ② 不执行 reload(1)——整表刷新会拉回任务入队前的旧 failed 状态，覆盖乐观值导致"先失败后解析中"闪现；
    //    抽屉轮询（silentSync）随后自动跟踪 pending → processing → completed/failed 真实状态。
    row.parse_status = 'pending'
    row.custom_metadata = { ...(row.custom_metadata || {}), parse_error: '' }
    MessagePlugin.success('已重新解析')
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    row._reparsing = false
  }
}

// ---- 重新提取 ----
const reextractRow = async (row: any) => {
  // 解析失败前置拦截：无正文内容可提取，必须先重新解析
  if (row.parse_status === 'failed') {
    MessagePlugin.warning('该文件解析失败，请先「重新解析」后再提取')
    return
  }
  row._extracting = true
  try {
    // 后端只认 cert_type（历史代码曾误传 doc_type 导致证照类型静默丢失）
    const scope = props.scope || 'vehicle'
    const payload = { scope, cert_type: row.doc_type || undefined }
    if (getScope(scope)?.api.extract === 'business') {
      await extractBusinessDocument(props.kbId, row.id, payload)
    } else {
      await extractFleetDocument(props.kbId, row.id, payload)
    }
    MessagePlugin.success('已重新提取')
    reload()
    emit('changed')
  } catch (e: any) {
    // 409：该文件已有提取任务在途（上传弹窗/列表轮询/另一通道刚提交过），
    // 属重复提交而非提取失败——数据仍由持锁请求完成，按在途处理跟随即可，
    // 否则会把一次正常的并发去重误报成红色「重新提取失败」。
    if (isExtractInProgress(e)) {
      MessagePlugin.info('该文件正在提取中，请稍候刷新查看结果')
      reload()
      return
    }
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
    allRows.value = allRows.value.filter((r) => r.id !== row.id)
    activeRow.value = null
    MessagePlugin.success("已删除")
    emit("changed", { deletedIds: [row.id] })
  } catch (e: any) {
    MessagePlugin.error(e?.message || "删除失败")
  }
}


// ---- 行操作三点菜单 ----

const rowMenuOptions = (_row: any) => {
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

// 需求：解析/提取状态与上传时的真实处理状态实时同步。
// 设计原则：抽屉打开后不做无条件高频整表轮询，避免列表不停闪烁/选中丢失/滚动跳顶。
// 1) 按需启动：仅当存在「活跃任务」（解析/提取进行中，或解析完成提取待落定）才启动轮询；
// 2) 静默增量：轮询时仅对已存在行 patch 状态字段（保持行引用/选中态/滚动位置），
//    仅当发生行增删（id 集合变化）才回退整表刷新；
// 3) 及时收敛：全部任务落定即停轮询；超过 2 分钟无任何状态变化也自动停止，杜绝后台空转。
let pollTimer: ReturnType<typeof setInterval> | null = null
let lastChangeTs = Date.now()
const stopPolling = () => { if (pollTimer) { clearInterval(pollTimer); pollTimer = null } }

// 是否存在需要持续关注的任务（任一命中即保持轮询）
const hasActiveTasks = () => {
  return allRows.value.some((r: any) => {
    const p = r.parse_status || ''
    const e = extractStatus(r)
    if (p === 'pending' || p === 'processing' || p === 'finalizing') return true
    if (e === 'pending' || e === 'processing') return true
    if (p === 'completed' && !e) return true
    return false
  })
}

// 静默增量同步：拉取最新首页数据，仅更新已有行的状态字段；行增删时回退整表刷新
const silentSync = async () => {
  if (!props.kbId) return
  try {
    const res: any = await listKnowledgeFiles(props.kbId, { page: 1, page_size: DEFAULT_PAGE_SIZE })
    let arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    arr = arr.filter((r: any) => !((r.custom_metadata || {}).fleet_history_hidden))
    const dtSet = props.docTypes
    if (dtSet && dtSet.size) arr = arr.filter((r: any) => { const dt = rowDocType(r); return dt ? dtSet.has(dt) : true })
    const f = props.filter || 'all'
    if (f === 'parse_failed') arr = arr.filter((r: any) => r.parse_status === 'failed')
    else if (f === 'extract_failed') arr = arr.filter((r: any) => (r.custom_metadata || {}).extract_status === 'failed')
    // 增删检测：当前首页 id 集合与 fresh 不一致说明发生了真实变化，回退整表刷新
    const currentIds = allRows.value.slice(0, DEFAULT_PAGE_SIZE).map((r: any) => r.id).sort()
    const freshIds = arr.map((r: any) => r.id).sort()
    if (currentIds.length !== freshIds.length || currentIds.some((id, i) => id !== freshIds[i])) {
      await reload()
      return
    }
    // 仅增量 patch 已有行的状态字段（不替换数组、不重置选中/分页/滚动）
    const map = new Map<string, any>(arr.map((r: any) => [r.id, r] as [string, any]))
    let changed = false
    for (const row of allRows.value) {
      const fr = map.get(row.id)
      if (!fr) continue
      if (fr.parse_status !== row.parse_status) { row.parse_status = fr.parse_status; changed = true }
      const fe = (fr.custom_metadata || {}).extract_status || ''
      if (fe !== extractStatus(row)) {
        row.custom_metadata = { ...(row.custom_metadata || {}), extract_status: fe }
        changed = true
      }
      const dt = invDocType(fr)
      if (dt !== row.doc_type) { row.doc_type = dt; changed = true }
    }
    if (changed) lastChangeTs = Date.now()
  } catch { /* 静默轮询失败不打扰用户，下一轮重试 */ }
}

watch(() => props.visible, (v) => {
  stopPolling()
  if (v) {
    reload().then(() => {
      if (!hasActiveTasks()) return
      lastChangeTs = Date.now()
      pollTimer = setInterval(async () => {
        await silentSync()
        // 全部任务落定，或长时间无状态变化（防后台空转），停止轮询
        if (!hasActiveTasks() || Date.now() - lastChangeTs > 120000) stopPolling()
      }, 3000)
    })
  }
}, { immediate: true })
onBeforeUnmount(stopPolling)
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
.dh-search { flex: 1; min-width: 0; }
/* 上传时间区间：与搜索框同高同圆角，宽度固定避免挤压搜索框 */
.dh-date-range { flex: 0 0 260px; width: 260px; }
/* 发票类型下拉：与搜索框/日期框同高同圆角，固定宽度不抢搜索框空间 */
.dh-type-select { flex: 0 0 150px; width: 150px; }
.dh-type-select :deep(.t-input) { border-radius: var(--td-radius-default, 6px); }
.dh-date-range :deep(.t-input) { border-radius: var(--td-radius-default, 6px); }
/* ---- 状态统计筛选小卡片 ---- */
.dh-status-cards { display: flex; gap: 8px; }
.dh-status-card {
  flex: 1 1 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease, box-shadow 0.15s ease;
  text-align: left;
  min-width: 0;
}
.dh-status-card:hover { border-color: var(--td-brand-color-light-active, var(--td-brand-color)); }
.dh-status-card.is-active {
  border-color: var(--td-brand-color);
  background: var(--td-brand-color-light);
  box-shadow: 0 0 0 1px var(--td-brand-color) inset;
}
.dh-status-card__label {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-status-card.is-active .dh-status-card__label { color: var(--td-brand-color); }
.dh-status-card__num {
  font-size: 16px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--td-text-color-primary);
}
.dh-status-card.is-active .dh-status-card__num { color: var(--td-brand-color); }
/* 异常卡用警示色区分：即使未选中也让人一眼看到有失败文件 */
.dh-status-card--abnormal .dh-status-card__num { color: var(--td-error-color); }
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
/* 双击文件名可全屏预览源文件：给出可点击的视觉提示 */
.dh-file--clickable { cursor: pointer; }
.dh-file--clickable:hover .dh-file-name { color: var(--td-brand-color); }
/* 全屏源文件预览：铺满视口，PDF/图片在历史记录里直接大图核对 */
.dh-full-preview :deep(.t-dialog__body) { padding: 0; overflow: hidden; }
.dh-full-preview__body { height: 86vh; width: 100%; overflow: auto; }
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
