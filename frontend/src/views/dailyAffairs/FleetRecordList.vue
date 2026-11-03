<!--
  车队记录列表（第 28 轮三段式重构）。

  布局严格对齐发票管理 / 能耗管理：
    ① 顶部 DashboardKpiGroup（分类卡 + 历史记录卡）
    ② 中部 BusinessParseStatusBar + BusinessFilterBar（含 #batch-actions）
    ③ 下部 t-table（勾选感知合计行）+ BusinessSplitDetailDrawer

  数据源：分类 subs（六同步唯一真源）。轮胎/年检/成本 7 项由 tire/inspection/
  fleet-cost scope 的分类驱动，档案类（车辆/司机/维保）由 vehicle/driver/maintain 驱动。

  门禁：解析中/提取中的文件绝不进列表、不参与合计、不计入 KPI。
-->
<template>
  <div class="biz-page fleet-page">
    <!-- ① 顶部 KPI 卡片组：分类卡（或档案证照卡）+ 历史记录卡 -->
    <div class="overview-group">
      <DashboardKpiGroup :loading="!cardsReady || !kpiCards.length" :cards="kpiCards" :active-type="activeType"
        @card-click="onKpiCardClick" />
    </div>

    <div class="biz-list-view">
      <!-- 解析/提取进度横幅（与发票同款） -->
      <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

      <!-- ② 筛选工具栏 -->
      <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
        :columns="columnDefsForFilter" :visible-keys="visibleColKeys" :selected-count="selectedKeys.size"
        settings-text="设置" :primary-action="primaryAction" @columns-change="onColumnsChange"
        @reset-columns="resetColumns" @select-all-columns="selectAllColumns" @filter-change="applyFilter"
        @settings="settingsVisible = true" @primary="onPrimaryAction" @clear-selection="clearSelection">
        <template #batch-actions>
          <!-- 重新解析 / 重新提取：仅档案类（记录型派生台账无源文件可重解析） -->
          <t-popconfirm v-if="isArchive" theme="warning"
            :content="`确定重新解析所选 ${selectedFileRows.length} 个文件吗？解析完成后自动提取。`"
            :confirm-btn="{ content: '重新解析', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="batchRetry('parse')">
            <t-button theme="default" variant="outline" size="small" :loading="batchBusy"
              :disabled="!selectedFileRows.length" @click.stop>
              <template #icon><t-icon name="refresh" size="14px" /></template>
              重新解析
            </t-button>
          </t-popconfirm>
          <t-popconfirm v-if="isArchive" theme="warning"
            :content="`确定重新提取所选 ${selectedFileRows.length} 个文件吗？将覆盖已有提取结果。`"
            :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="batchRetry('extract')">
            <t-button theme="default" variant="outline" size="small" :loading="batchBusy"
              :disabled="!selectedFileRows.length" @click.stop>
              <template #icon><t-icon name="cloud-upload" size="14px" /></template>
              重新提取
            </t-button>
          </t-popconfirm>
          <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handleCatalogPrint">
            <template #icon><t-icon name="file-paste" size="14px" /></template>
            打印目录
          </t-button>
          <t-popconfirm theme="warning" :content="deleteHint"
            :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="handleDelete">
            <t-button theme="danger" variant="outline" size="small" @click.stop>
              <template #icon><t-icon name="delete" size="14px" /></template>
              删除
            </t-button>
          </t-popconfirm>
        </template>
      </BusinessFilterBar>

      <!-- ③ 列表 -->
      <div class="doc-list-scroll">
        <div class="doc-list-view">
          <t-table :data="pagedRows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="loading" :foot-data="footData" :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header class="doc-table fleet-table"
            :row-class-name="({ row }: any) => selectedKeys.has(rowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <template #empty>
              <BusinessEmptyState compact :icon="emptyIcon" :description="emptyText" />
            </template>
          </t-table>
        </div>
      </div>

      <!-- 分页：派生台账 1500+ 条，必须分页（懒加载会与合计行冲突） -->
      <div v-if="total > pageSize" class="fleet-pager">
        <t-pagination :current="page" :page-size="pageSize" :total="total" size="small"
          :page-size-options="[20, 50, 100]" @current-change="onPageChange" @page-size-change="onPageSizeChange" />
      </div>
    </div>

    <!-- 上传弹窗（档案类 + 记录型上传提取） -->
    <FleetUploadDialog v-if="canUpload" v-model:visible="uploadVisible" :kb-id="kbId" :scope="uploadScope"
      :type-options="uploadTypeOptions" :default-type="uploadDefaultType" :extract-record-type="recordType"
      @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 历史记录抽屉 -->
    <FleetUploadHistoryDrawer v-if="canUpload" v-model:visible="historyVisible" :kb-id="kbId" :scope="uploadScope"
      :filter="historyFilter" :doc-types="currentDocTypes" @changed="onHistoryChanged" />

    <!-- 设置抽屉（字段定义 + 提取规则 + 基础配置扩展页签） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId" :scope="settingsScope"
      :initial-category="activeCategoryName" :title="`${menuLabel}设置`" storage-key="weknora-fleet-settings-width"
      :extra-tab-label="extraTabLabel" @saved="onSettingsSaved">
      <template #extra-tab>
        <OilCardPanel v-if="extraTabKind === 'oilcard'" />
        <EtcPanel v-else-if="extraTabKind === 'etc'" />
      </template>
    </StandardSettingDrawer>

    <!-- 详情抽屉：左 PDF 预览 + 右 subs 驱动表单 -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="detailRow"
      :page-no="detailPageNo" :form="editForm" :fields="drawerFields" :sub-fields="drawerSubFields"
      :supports-sub-fields="false" :dirty="detailDirty" :saving="saving" :storage-key="'weknora-fleet-detail-width'"
      :show-preview="!!detailKnowledgeId" :block-title="menuLabel" @save="saveDetail" @delete="onDetailDelete">
      <template #field-车牌号>
        <t-select v-model="editForm.data['车牌号']" :options="vehicleOptions" filterable clearable allow-create
          placeholder="选择或输入车牌号" />
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 打印预览（统一中台组件） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="printTitle" :url="printUrl" :data="printData"
      :download-name="`${menuLabel}目录.pdf`" @close="closePrint" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listKnowledgeBases, createKnowledgeBase, listKnowledgeFiles, getFleetOverviewStats,
  reparseKnowledge, delKnowledgeDetails,
} from '@/api/knowledge-base'
import {
  listFleetVehicles, listFleetRecords, createFleetRecord, updateFleetRecord, deleteFleetRecord,
  listFleetCategories, getFleetRecordStats, syncFleetInvoices, extractFleetDocument,
} from '@/api/fleet'
import { generateCatalogPdf } from './useCatalogPdf'
import { isExtractInProgress } from '@/composables/extractConflict'
import { selectInitialModelId } from '@/utils/modelDefaults'
import { useChatResourcesStore } from '@/stores/chatResources'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessEmptyState from './BusinessEmptyState.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import OilCardPanel from './FleetOilCardPanel.vue'
import EtcPanel from './FleetEtcPanel.vue'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import { buildFootRow, footSpanOf, summaryScopeRows } from './listSummary'
import { filterRowsByPendingFiles } from './invoicePendingGate'
import {
  buildFleetColumns, buildFleetSummaryColumns, buildFleetFilterSpecs,
  buildFleetFieldFiltersParam, type FleetColumn,
} from './fleetColumns'
import type { FilterFieldSpec } from './businessFilter'

const props = defineProps<{ recordType: string }>()
const chatResources = useChatResourcesStore()

// ---------------------------------------------------------------------------
// 菜单 → 分类 scope 映射（12 个菜单的唯一配置表）
// ---------------------------------------------------------------------------
interface MenuMeta {
  label: string
  /** 分类 scope（六同步数据源） */
  scope: string
  /** 是否档案类（有源文件、走上传→解析→提取） */
  archive: boolean
  /** 是否支持上传 */
  upload: boolean
  /** 主操作按钮文案 */
  primaryText: string
  /** 单分类菜单的固定分类名 */
  singleCategory?: string
}

const MENUS: Record<string, MenuMeta> = {
  'vehicle-archive': { label: '车辆档案', scope: 'vehicle', archive: true, upload: true, primaryText: '上传证照' },
  'driver-archive': { label: '司机档案', scope: 'driver', archive: true, upload: true, primaryText: '上传证照' },
  'maintain-archive': { label: '维保记录', scope: 'maintain', archive: true, upload: true, primaryText: '上传文件' },
  tire: { label: '轮胎管理', scope: 'tire', archive: false, upload: true, primaryText: '新增记录', singleCategory: '轮胎管理' },
  inspection: { label: '年检管理', scope: 'inspection', archive: false, upload: true, primaryText: '新增记录', singleCategory: '年检管理' },
  'fuel-charge': { label: '加油充电', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '加油充电' },
  'road-toll': { label: '路桥费', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '路桥费' },
  'repair-cost': { label: '维修保养费', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '维修保养费' },
  'insurance-claim': { label: '保险理赔', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '保险理赔' },
  violation: { label: '违章处理', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '违章处理' },
  material: { label: '辅材记录', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '辅材记录' },
  'car-request': { label: '请车记录', scope: 'fleet-cost', archive: false, upload: true, primaryText: '新增记录', singleCategory: '请车记录' },
}

const meta = computed<MenuMeta>(() => MENUS[props.recordType] || MENUS.tire)
const menuLabel = computed(() => meta.value.label)
const isArchive = computed(() => meta.value.archive)
const canUpload = computed(() => meta.value.upload)
const primaryAction = computed(() => ({ text: meta.value.primaryText, icon: isArchive.value ? 'upload' : 'add' }))
const recordType = computed(() => props.recordType)

// ---------------------------------------------------------------------------
// 基础数据
// ---------------------------------------------------------------------------
const KB_NAME = '日常事务-车队'
const kbId = ref('')
const loading = ref(false)
const cardsReady = ref(false)
// 发票派生同步每次会话只自动跑一次（全库扫描代价高，见 bootstrap 注释）。
let invoiceSynced = false
const vehicles = ref<any[]>([])
const categories = ref<Record<string, any[]>>({})
const vehicleMap = computed(() => {
  const m = new Map<string, string>()
  for (const v of vehicles.value) if (v?.id) m.set(v.id, v.plate_no || '')
  return m
})
const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.plate_no })))

// 当前菜单的分类列表（六同步数据源）
const menuCategories = computed<any[]>(() => {
  const arr = categories.value[meta.value.scope] || []
  return arr.filter((c: any) => c && c.enabled !== false)
})

// 当前激活分类：档案类空 = 总览（文件列表）；记录型锁定菜单分类
const activeType = ref('')
const activeCategoryName = computed(() => {
  if (!isArchive.value) return meta.value.singleCategory || ''
  return activeType.value ? activeType.value.replace(/^cat-/, '') : ''
})
const activeSubs = computed<any[]>(() => {
  if (!isArchive.value) {
    const hit = menuCategories.value.find((c: any) => c.name === meta.value.singleCategory)
    return hit?.subs || menuCategories.value[0]?.subs || []
  }
  if (!activeType.value) return []
  const hit = menuCategories.value.find((c: any) => c.name === activeCategoryName.value)
  return hit?.subs || []
})

// ---------------------------------------------------------------------------
// KPI 卡片
// ---------------------------------------------------------------------------
const overviewStats = ref<any>(null)
const recordStats = ref<Record<string, number>>({})
const kpiCards = computed(() => {
  const cards: any[] = []
  if (isArchive.value) {
    const items = ((overviewStats.value?.by_doc_type || []) as any[])
    const totalN = items.reduce((s, t) => s + (t.count || 0), 0)
    const failN = items.reduce((s, t) => s + (t.parse_failed || 0) + (t.extract_failed || 0), 0)
    cards.push({
      key: 'all', label: '已收录证照', icon: 'file-copy', value: String(totalN), unit: '份',
      sub: failN ? `${failN} 份异常` : '无异常文件', action: 'all', theme: 'brand',
    })
    for (const c of menuCategories.value) {
      const hit = items.find((t: any) => t.doc_type === c.name || (c.builtin_key && t.doc_type === c.builtin_key))
      const bad = hit ? (hit.parse_failed || 0) + (hit.extract_failed || 0) : 0
      cards.push({
        key: 'cat-' + c.name, label: c.name, icon: 'file', value: String(hit?.count || 0), unit: '份',
        sub: bad ? `${bad} 份异常` : '无异常文件',
        action: 'type', typeValue: 'cat-' + c.name,
      })
    }
    cards.push({
      key: 'history', label: '历史记录', icon: 'history', value: String(totalN), unit: '份',
      sub: failN ? `${failN} 份异常文件` : '无异常文件', action: 'history',
    })
  } else {
    const n = recordStats.value[recordType.value] || 0
    cards.push({
      key: 'all', label: '已收录记录', icon: 'file-copy', value: String(n), unit: '条',
      sub: '全部有效记录', action: 'all', theme: 'brand',
    })
    for (const c of menuCategories.value) {
      if (meta.value.singleCategory && c.name !== meta.value.singleCategory) continue
      cards.push({
        key: 'cat-' + c.name, label: c.name, icon: 'file', value: String(n), unit: '条',
        sub: '当前菜单', action: 'type', typeValue: 'cat-' + c.name,
      })
    }
    cards.push({
      key: 'history', label: '历史记录', icon: 'history', value: String(n), unit: '条',
      sub: '源文件 / 派生来源', action: 'history',
    })
  }
  return cards
})

function onKpiCardClick(card: any) {
  if (card.action === 'history') {
    historyFilter.value = 'all'
    historyVisible.value = true
    return
  }
  if (card.action === 'all') {
    activeType.value = ''
    page.value = 1
    reloadList()
    return
  }
  if (card.action === 'type' && card.typeValue) {
    activeType.value = card.typeValue
    page.value = 1
    reloadList()
  }
}

// ---------------------------------------------------------------------------
// 列表
// ---------------------------------------------------------------------------
const rows = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
// 初始态必须带 dateRange: []：TDesign 的 date-range-picker 收到 undefined 会
// 每次渲染打 console.error「typeof value: undefined must be Array!」，
// 真机验收的「控制台 0 error」硬断言会直接失败（发票页同样如此初始化）。
const initialFilterState = (): Record<string, any> => ({ keyword: '', docType: '', dateRange: [] })
const filterState = ref<Record<string, any>>(initialFilterState())
const selectedKeys = ref<Set<string>>(new Set())
const selectedRowKeys = computed(() => Array.from(selectedKeys.value))
const rowKey = (r: any) => String(r?.id || '')

const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(rowKey(r))))
const selectedFileRows = computed(() => selectedRows.value.filter((r) => r.__file))

const columnDefs = computed<FleetColumn[]>(() => buildFleetColumns(activeSubs.value, vehicleMap.value))
const columnDefsForFilter = computed(() => columnDefs.value.map((c) => ({ key: c.key, label: c.label })))

const visibleColKeys = ref<string[]>([])
const STORAGE_KEY = computed(() => `weknora-fleet-${meta.value.scope}-${props.recordType}-cols-v4`)

const visibleCols = computed(() => columnDefs.value.filter((c) => visibleColKeys.value.includes(c.key)))

const tableColumns = computed(() => {
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 44 }]
  for (const c of visibleCols.value) {
    const values = rows.value.slice(0, 30).map((r) => c.value(r))
    const base: any = {
      colKey: c.key,
      title: c.label,
      width: colWidthOf(c.width, c.label, values, c.dataType),
      ellipsis: ellipsisOf(c),
      cell: (_h: any, params: any) => c.value(params?.row),
    }
    const cls = cellClassOf(c)
    if (cls) base.className = cls
    cols.push(base)
  }
  return cols
})

const tableScroll = computed(() => tableScrollOf(visibleCols.value))

// 合计行：勾选感知（选中时只汇总选中行；非累加列留空）
const summaryCols = computed(() => buildFleetSummaryColumns(visibleCols.value))
const summaryScope = computed(() => summaryScopeRows(rows.value, selectedKeys.value, rowKey))
const footData = computed(() => [buildFootRow(summaryCols.value, summaryScope.value.rows, {
  selectKey: 'row-select', selectedCount: summaryScope.value.selectedCount,
})])
const footSpan = computed(() => footSpanOf(summaryCols.value))

const pagedRows = computed(() => rows.value)

// 筛选器：总览态按证照类型；其余由 subs 派生
const filterFields = computed<FilterFieldSpec[]>(() => {
  if (isArchive.value && !activeType.value) {
    return [{ key: 'docType', type: 'select', placeholder: '证照类型', options: docTypeOptions.value }]
  }
  return buildFleetFilterSpecs(columnDefs.value)
})

function persistColumns() {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
function resetColumns() {
  const defs = columnDefs.value.filter((c) => c.default).map((c) => c.key)
  visibleColKeys.value = defs.length ? defs : columnDefs.value.map((c) => c.key)
  persistColumns()
}
function selectAllColumns() { visibleColKeys.value = columnDefs.value.map((c) => c.key); persistColumns() }
function onColumnsChange(keys: string[]) { visibleColKeys.value = keys; persistColumns() }

function initColumns() {
  const saved = localStorage.getItem(STORAGE_KEY.value)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      const allKeys = columnDefs.value.map((c) => c.key)
      const defs = columnDefs.value.filter((c) => c.default).map((c) => c.key)
      const matched = allKeys.filter((k) => Array.isArray(arr) && arr.includes(k))
      if (matched.length && defs.every((k) => matched.includes(k))) {
        visibleColKeys.value = matched
        return
      }
    } catch { /* ignore */ }
  }
  resetColumns()
}

function onTableSelectChange(val: any) {
  const arr = Array.isArray(val) ? val : []
  selectedKeys.value = new Set(arr.map(String))
}
function clearSelection() { selectedKeys.value = new Set() }

// 门禁：pending 文件对应的记录整组剔除
const pendingIds = computed(() => new Set(pendingFiles.value.map((f: any) => String(f.id))))
const gatedRows = computed(() => {
  if (!isArchive.value) return rows.value
  return filterRowsByPendingFiles(
    rows.value.map((r) => ({ ...r, knowledgeId: r.doc_knowledge_id || r.data?.['源文件ID'] || '' })),
    pendingIds.value,
  )
})

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------
// 筛选变更合并窗口（与发票页同口径）：关键词/车牌等文本筛选由用户逐字输入，
// 每次按键都打后端会让 1500+ 条的派生台账频繁抖动；300ms 合并成一次请求。
let filterTimer: ReturnType<typeof setTimeout> | null = null
function scheduleFilter() {
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    filterTimer = null
    applyFilter()
  }, 300)
}
function applyFilter() { page.value = 1; reloadList() }

// BusinessFilterBar 只负责「不可变更新 + 可选 immediate」，防抖归父级。
// 缺这个 watch 时文本筛选（车牌号/费用类型…）必须点刷新才生效，
// 用户输完回车看起来「没反应」（真机 B4 因此拿到 20 行混车牌）。
watch(filterState, () => { scheduleFilter() }, { deep: true })

async function reloadList() {
  loading.value = true
  try {
    if (isArchive.value && !activeType.value) {
      await ensureFileRows(true)
      rows.value = fileRows.value
      total.value = fileRows.value.length
      return
    }
    const state = filterState.value || {}
    const [df, dt] = Array.isArray(state.dateRange) ? state.dateRange : []
    const res: any = await listFleetRecords({
      type: recordType.value,
      doc_type: isArchive.value ? activeCategoryName.value : undefined,
      q: String(state.keyword || '').trim() || undefined,
      field_filters: buildFleetFieldFiltersParam(state) || undefined,
      date_from: df || undefined,
      date_to: dt || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    const data = res?.data || {}
    rows.value = Array.isArray(data.items) ? data.items : Array.isArray(data) ? data : []
    total.value = Number(data.total) || rows.value.length
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function reloadStats() {
  try {
    const res: any = await getFleetRecordStats({ types: recordType.value })
    recordStats.value = (res?.data?.by_type || {}) as Record<string, number>
  } catch { /* 统计失败不阻塞列表 */ }
  if (isArchive.value && kbId.value) {
    try {
      const res: any = await getFleetOverviewStats(kbId.value)
      overviewStats.value = res?.data || res || null
    } catch { /* ignore */ }
  }
  cardsReady.value = true
}

function onPageChange(p: any) {
  page.value = Number(p?.current ?? p) || 1
  reloadList()
}
function onPageSizeChange(size: any) {
  pageSize.value = Number(size) || 20
  page.value = 1
  reloadList()
}

// ---------------------------------------------------------------------------
// 档案类：文件列表与分类
// ---------------------------------------------------------------------------
const fileRows = ref<any[]>([])
const rawFileRows = ref<any[]>([])

async function ensureFileRows(force = false) {
  if (!kbId.value) return
  if (!force && fileRows.value.length) return
  const fres: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
  const arr = Array.isArray(fres?.data) ? fres.data : Array.isArray(fres?.list) ? fres.list : []
  rawFileRows.value = arr
  fileRows.value = arr
    .filter((it: any) => !(it.custom_metadata || {}).fleet_history_hidden)
    .map((it: any) => {
      const m = it.custom_metadata || {}
      return {
        __file: true,
        id: it.id,
        knowledgeId: it.id,
        title: it.title || '',
        文件别名: it.title || it.file_name || it.name || '',
        证照类型: m.fleet_cert_type || m.doc_type || '',
        源文件: it.file_name || it.name || '',
        文件类型: it.file_type || '',
        解析状态: it.parse_status || '',
        提取状态: m.extract_status || '',
        说明: m.extract_error || '',
        上传时间: it.created_at || '',
        doc_type: m.fleet_cert_type || m.doc_type || '',
        parse_status: it.parse_status || '',
        extract_status: m.extract_status || '',
        created_at: it.created_at || '',
        tags: Array.isArray(it.tags) ? it.tags : [],
      }
    })
}

const docTypeOptions = computed(() => menuCategories.value.map((c: any) => ({ label: c.name, value: c.name })))
const currentDocTypes = computed(() => new Set(menuCategories.value.map((c: any) => c.name)))

async function reloadCategories() {
  try {
    const res: any = await listFleetCategories({ scope: meta.value.scope })
    categories.value = { ...categories.value, [meta.value.scope]: res?.data || [] }
  } catch { /* ignore */ }
}

// ---------------------------------------------------------------------------
// 详情抽屉
// ---------------------------------------------------------------------------
const detailVisible = ref(false)
const detailRow = ref<any>(null)
const detailDirty = ref(false)
const saving = ref(false)
const editForm = ref<any>({ data: {}, remarkParts: {}, items: [] })
const editBaseline = ref('')

const detailKnowledgeId = computed(() =>
  detailRow.value?.doc_knowledge_id || detailRow.value?.data?.['源文件ID'] || '',
)
const detailPageNo = computed(() => Number(detailRow.value?.data?.['页码']) || 1)
const detailTitle = computed(() => `${menuLabel.value}详情${detailRow.value?.file_name ? ' · ' + detailRow.value.file_name : ''}`)

const drawerFields = computed(() =>
  (activeSubs.value || [])
    .filter((s: any) => s && s.enabled !== false)
    .map((s: any) => ({ name: String(s.name), label: String(s.label || s.name), dataType: String(s.data_type || 'text') })),
)
const drawerSubFields = computed(() => [])

function openDetail(row: any) {
  detailRow.value = row
  editForm.value = { data: { ...(row?.data || {}) }, remarkParts: {}, items: [] }
  editBaseline.value = JSON.stringify(editForm.value)
  detailDirty.value = false
  detailVisible.value = true
}

watch(editForm, () => {
  detailDirty.value = JSON.stringify(editForm.value) !== editBaseline.value
}, { deep: true })

async function saveDetail() {
  const row = detailRow.value
  saving.value = true
  try {
    const payload: any = {
      record_type: row?.record_type || recordType.value,
      vehicle_id: row?.vehicle_id || '',
      record_date: row?.record_date || '',
      record_month: row?.record_month || '',
      amount: Number(row?.amount) || 0,
      remark: row?.remark || '',
      data: { ...(editForm.value.data || {}) },
    }
    if (row?.id) {
      await updateFleetRecord(row.id, payload)
    } else {
      await createFleetRecord(payload)
    }
    MessagePlugin.success('已保存')
    detailVisible.value = false
    await reloadList()
    await reloadStats()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function onDetailDelete() {
  const row = detailRow.value
  if (!row?.id) { detailVisible.value = false; return }
  try {
    if (row.__file) await delKnowledgeDetails(row.id)
    else await deleteFleetRecord(row.id)
    MessagePlugin.success('已删除')
    detailVisible.value = false
    await reloadList()
    await reloadStats()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---------------------------------------------------------------------------
// 新增 / 上传 / 设置
// ---------------------------------------------------------------------------
const uploadVisible = ref(false)
const historyVisible = ref(false)
const historyFilter = ref<'all' | 'parse_failed' | 'extract_failed'>('all')
const settingsVisible = ref(false)

const uploadScope = computed(() => meta.value.scope)
// 设置抽屉只接受已注册的 scope（tire/inspection/fleet-cost 由 StandardSettingDrawer 扩展）
const settingsScope = computed<'vehicle' | 'driver' | 'maintain' | 'tire' | 'inspection' | 'fleet-cost'>(() =>
  meta.value.scope as 'vehicle' | 'driver' | 'maintain' | 'tire' | 'inspection' | 'fleet-cost',
)
const uploadDefaultType = computed(() => (isArchive.value ? activeCategoryName.value : meta.value.singleCategory || ''))
const uploadTypeOptions = computed(() =>
  isArchive.value ? menuCategories.value.map((c: any) => ({ label: c.name, value: c.name })) : [],
)

function onPrimaryAction() {
  if (isArchive.value) { uploadVisible.value = true; return }
  openDetail(null)
}

function onUploadDone() { reloadList(); reloadStats(); loadPending() }
function onHistoryChanged() { reloadList(); reloadStats() }
function onSettingsSaved() { reloadCategories().then(() => { initColumns(); reloadList() }) }

const extraTabLabel = computed(() =>
  props.recordType === 'fuel-charge' ? '油卡配置' : props.recordType === 'road-toll' ? 'ETC 配置' : '',
)
const extraTabKind = computed(() =>
  props.recordType === 'fuel-charge' ? 'oilcard' : props.recordType === 'road-toll' ? 'etc' : '',
)

// ---------------------------------------------------------------------------
// 批量操作
// ---------------------------------------------------------------------------
const batchBusy = ref(false)
async function batchRetry(kind: 'parse' | 'extract') {
  const files = selectedFileRows.value
  if (!files.length || batchBusy.value) return
  batchBusy.value = true
  let ok = 0, fail = 0
  for (const f of files) {
    try {
      if (kind === 'parse') await reparseKnowledge(f.id)
      else await extractFile(f.id, uploadScope.value, f.doc_type || '')
      ok++
    } catch (e: any) {
      if (isExtractInProgress(e)) { ok++; continue }
      fail++
    }
  }
  batchBusy.value = false
  MessagePlugin.success(`已发起 ${ok} 个文件${kind === 'parse' ? '重新解析' : '重新提取'}${fail ? `，${fail} 个失败` : ''}`)
  clearSelection()
  loadPending()
  reloadList()
}

const deleteHint = computed(() =>
  isArchive.value
    ? `确定删除所选 ${selectedRows.value.length} 项吗？文件将从知识库移除。`
    : `确定删除所选 ${selectedRows.value.length} 条记录吗？删除后不可恢复。`,
)

async function handleDelete() {
  try {
    for (const row of selectedRows.value) {
      if (row.__file) await delKnowledgeDetails(row.id)
      else await deleteFleetRecord(row.id)
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await reloadList()
    await reloadStats()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---------------------------------------------------------------------------
// 打印
// ---------------------------------------------------------------------------
const printVisible = ref(false)
const printData = ref<Uint8Array | null>(null)
const printUrl = ref('')
const catalogBusy = ref(false)
const printTitle = computed(() => `${menuLabel.value}目录`)

async function handleCatalogPrint() {
  const list = selectedRows.value.length ? selectedRows.value : gatedRows.value
  if (!list.length) { MessagePlugin.warning('当前无数据可打印'); return }
  catalogBusy.value = true
  try {
    const cols = visibleCols.value.map((c) => ({ key: c.key, label: c.label, value: c.value }))
    printData.value = await generateCatalogPdf({ title: printTitle.value, columns: cols, rows: list })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([printData.value as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
function closePrint() {
  printVisible.value = false
  printData.value = null
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---------------------------------------------------------------------------
// 上传进度 / 轮询（档案类）
// ---------------------------------------------------------------------------
const pendingFiles = ref<any[]>([])
const uploadProgress = ref<{ name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]>([])
const progressIndex = ref(0)
let progressTimer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
const extractingSet = new Set<string>()

const displayProgress = computed(() => {
  if (uploadProgress.value.length) {
    const cur = uploadProgress.value[progressIndex.value] || uploadProgress.value[0]
    return [{ name: cur.name, stage: cur.stage, percent: cur.percent, elapsedSec: 0 }]
  }
  return []
})

function onUploadProgress(items: { name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]) {
  uploadProgress.value = items || []
  if (uploadProgress.value.length <= 1) {
    progressIndex.value = 0
    if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
    return
  }
  if (progressTimer) return
  progressTimer = setInterval(() => {
    if (uploadProgress.value.length <= 1) {
      if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
      return
    }
    progressIndex.value = (progressIndex.value + 1) % uploadProgress.value.length
  }, 3000)
}

const pendingScopes = ref<Record<string, string | { s: string; t?: string }>>(loadPendingScopes())
function loadPendingScopes(): Record<string, string | { s: string; t?: string }> {
  try { return JSON.parse(sessionStorage.getItem('weknora-fleet-pending-scopes') || '{}') || {} } catch { return {} }
}
function savePendingScopes() {
  try { sessionStorage.setItem('weknora-fleet-pending-scopes', JSON.stringify(pendingScopes.value)) } catch { /* ignore */ }
}
function pendingScopeOf(kid: string): { s: string; t?: string } {
  const v = pendingScopes.value[kid]
  if (typeof v === 'string') return { s: v }
  return v || { s: '' }
}

async function extractFile(kid: string, scope: string, certType?: string) {
  const payload: any = { cert_type: certType || undefined }
  if (isArchive.value) payload.scope = scope
  else payload.record_type = recordType.value
  await extractFleetDocument(kbId.value, kid, payload)
}

async function loadPending() {
  if (!kbId.value || !isArchive.value) return
  pendingScopes.value = loadPendingScopes()
  try {
    let arr = rawFileRows.value
    if (!arr.length) {
      const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
      arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
      rawFileRows.value = arr
    }
    const scope = uploadScope.value
    const mine = arr.filter((it: any) => {
      const m = it.custom_metadata || {}
      if (m.fleet_history_hidden) return false
      const pv = pendingScopeOf(it.id)
      return m.fleet_scope === scope || m.scope === scope || pv.s === scope
    })
    pendingFiles.value = mine.filter((it: any) => {
      const ps = it.parse_status
      const m = it.custom_metadata || {}
      if (m.extract_status === 'success' || m.extract_status === 'failed') return false
      return ps === 'parsing' || ps === 'pending' || ps === 'processing'
    })
    for (const it of mine) {
      const ps = it.parse_status
      const m = it.custom_metadata || {}
      if (ps === 'completed' && !m.extract_status && !extractingSet.has(it.id)) {
        extractingSet.add(it.id)
        try {
          const pv = pendingScopeOf(it.id)
          await extractFile(it.id, scope, pv.t || m.fleet_cert_type || '')
          delete pendingScopes.value[it.id]
          savePendingScopes()
          reloadList()
        } catch { /* 下轮重试 */ } finally {
          extractingSet.delete(it.id)
        }
      }
    }
  } catch { /* ignore */ }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => { void loadPending() }, 4000)
}
function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}

// ---------------------------------------------------------------------------
// 知识库初始化
// ---------------------------------------------------------------------------
async function ensureKb() {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    if (found?.id) { kbId.value = found.id; return }
    await chatResources.ensureModels()
    const allModels = chatResources.allModels || []
    const llm = selectInitialModelId(allModels, 'KnowledgeQA') || ''
    const emb = selectInitialModelId(allModels, 'Embedding') || ''
    if (!llm || !emb) {
      MessagePlugin.warning('未找到可用模型，请在设置中选择提取/向量模型后重试')
      return
    }
    const payload: any = {
      name: KB_NAME,
      description: '存放并解析车辆/司机证照、维保与成本票据，自动提取字段',
      type: 'document',
      chunking_config: {
        chunk_size: 512, chunk_overlap: 80,
        separators: ['\n\n', '\n', '。', '；', '，', ';', '、'],
        enable_parent_child: true, strategy: '', token_limit: 0, languages: [], table_metadata_instructions: '',
      },
      embedding_model_id: emb,
      summary_model_id: llm,
      indexing_strategy: { vector_enabled: true, keyword_enabled: true, wiki_enabled: false, graph_enabled: false },
    }
    const cres: any = await createKnowledgeBase(payload)
    const kb = cres?.data || cres
    kbId.value = kb?.id || ''
    if (kbId.value) MessagePlugin.success(`${KB_NAME}知识库已创建`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '知识库初始化失败')
  }
}

async function loadVehicles() {
  try {
    const res: any = await listFleetVehicles()
    vehicles.value = res?.data || []
  } catch { /* ignore */ }
}

// ---------------------------------------------------------------------------
// 空态
// ---------------------------------------------------------------------------
const emptyIcon = computed(() => (isArchive.value ? 'file-copy' : 'search-error'))
const emptyText = computed(() => {
  if (isArchive.value && !activeType.value) return '暂无证照，上传后自动解析提取字段'
  if (isArchive.value) return `「${activeCategoryName.value}」暂无记录，可上传文件自动提取`
  return `暂无${menuLabel.value}记录，点击右上角「${primaryAction.value.text}」录入`
})

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------
async function bootstrap() {
  cardsReady.value = false
  selectedKeys.value = new Set()
  page.value = 1
  filterState.value = initialFilterState()
  activeType.value = isArchive.value ? '' : 'cat-' + (meta.value.singleCategory || '')
  await Promise.all([loadVehicles(), ensureKb()])
  await reloadCategories()
  initColumns()
  await reloadStats()
  await reloadList()
  // 发票 → 成本台账派生同步：**全库扫描**（1500+ 张票，几十秒），不能每次切菜单都跑。
  // 后端启动时已跑过一次；这里只在本次会话首次进入成本菜单时补一次
  // （用户在发票侧刚补录完、后端还没重启的场景）。手动入口在工具栏「同步发票」。
  if (!isArchive.value && ['fuel-charge', 'road-toll'].includes(props.recordType) && !invoiceSynced) {
    invoiceSynced = true
    syncFleetInvoices().then(() => { reloadList(); reloadStats() }).catch(() => { /* 忽略 */ })
  }
  startPolling()
}

watch(() => props.recordType, () => { void bootstrap() })

onMounted(() => {
  void bootstrap()
  window.addEventListener('fleet-categories-changed', onCatChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  if (filterTimer) { clearTimeout(filterTimer); filterTimer = null }
  window.removeEventListener('fleet-categories-changed', onCatChanged)
})
function onCatChanged() {
  reloadCategories().then(() => { initColumns(); reloadList() })
}
</script>

<style lang="less" scoped>
/* 三段式纵向排布：卡片组 / 工具栏 / 列表各组件自带样式，这里只保证间距与滚动 */
.fleet-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  flex: 1;
}
.overview-group { flex: 0 0 auto; }
.biz-list-view { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 8px; }
.doc-list-scroll { flex: 1; min-height: 0; overflow-y: auto; }
.fleet-pager { display: flex; justify-content: flex-end; padding: 8px 0 4px; }
</style>
