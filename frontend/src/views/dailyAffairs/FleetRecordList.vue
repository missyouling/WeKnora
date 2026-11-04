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
      <!-- skeleton=false：车队页不用灰色骨架占位（数量未返回前直接显示 0），
           表格区域单独用 loading 态；卡片横向溢出时由组件内置左右箭头滚动。 -->
      <DashboardKpiGroup :loading="!cardsReady" :skeleton="false" :cards="kpiCards" :active-type="activeType"
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
      <div class="doc-list-scroll" ref="tableBoxRef">
        <div class="doc-list-view">
          <t-table :data="pagedRows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="tableLoading" :foot-data="tableFootData"
            :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header
            :class="['doc-table', 'fleet-table', { 'fleet-table--sparse': sparseTable }]"
            :row-class-name="({ row }: any) => selectedKeys.has(rowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <template #empty>
              <!-- 首屏未就绪时不显示「暂无数据」：否则骨架屏期间会先闪一下空态 -->
              <BusinessEmptyState v-if="!tableLoading" compact :icon="emptyIcon"
                :description="emptyText" :hint="emptyHint" />
              <div v-else class="fleet-empty-placeholder" />
            </template>
          </t-table>
        </div>
      </div>

      <!-- 分页：派生台账 1500+ 条，必须分页（懒加载会与合计行冲突） -->
      <div v-if="total > pageSize && !activeCertStatus" class="fleet-pager">
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
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listKnowledgeBases, createKnowledgeBase, listKnowledgeFiles, getFleetOverviewStats,
  reparseKnowledge, delKnowledgeDetails,
} from '@/api/knowledge-base'
import {
  listFleetVehicles, listFleetRecords, createFleetRecord, updateFleetRecord, deleteFleetRecord,
  listFleetCategories, getFleetRecordStats, extractFleetDocument,
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
  buildFleetColumns, buildFleetSummaryColumns, buildFleetSummaryViewColumns,
  amountFromForm, dateFromForm, resolveDefaultCategoryName, buildFleetKpiCards, fleetEmptyState,
  certStatusOf, certStatusOptions, extractStatusOf,
  type FleetColumn,
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
  // 9 个手工录入类菜单：upload=false —— 不展示上传弹窗、历史记录卡与
  // 批量「重新解析/重新提取」（第 29 轮明确：纯手工录入，不做 OCR 提取）。
  tire: { label: '轮胎管理', scope: 'tire', archive: false, upload: false, primaryText: '新增记录', singleCategory: '轮胎管理' },
  inspection: { label: '年检管理', scope: 'inspection', archive: false, upload: false, primaryText: '新增记录', singleCategory: '年检管理' },
  'fuel-charge': { label: '加油充电', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '加油充电' },
  'road-toll': { label: '路桥费', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '路桥费' },
  'repair-cost': { label: '维修保养费', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '维修保养费' },
  'insurance-claim': { label: '保险理赔', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '保险理赔' },
  violation: { label: '违章处理', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '违章处理' },
  material: { label: '辅材记录', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '辅材记录' },
  'car-request': { label: '请车记录', scope: 'fleet-cost', archive: false, upload: false, primaryText: '新增记录', singleCategory: '请车记录' },
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
const recordSubStats = ref<Record<string, number>>({})
const recordSubAmount = ref<Record<string, number>>({})

// KPI 卡片数字的唯一来源：`/fleet/records/stats`（与列表同表同条件）。
//
// 第 29 轮修的两处口径错位：
//   ① 旧实现档案类卡片读 `knowledges` 的 fleet-overview-stats（整个知识库的文件数，
//      不含菜单隔离）→ 车辆档案与司机档案都显示 14 份，互相串数；
//   ② 卡片读文件表、列表读记录表 → 卡片 2 份、点开 1 条甚至 0 条。
// 现在两者都读 fleet_records：卡片 = BySubCategory[分类名]，列表 =
// records?record_type=..&sub_category=..，条件逐字相同。
//
// 第 30 轮：卡片结构/文案抽到 fleetColumns.buildFleetKpiCards（纯函数、有单测），
// 页面只负责喂数据。第 1 张「已收录」改为纯统计卡（action=stat，不可点击）。
const kpiCards = computed(() => buildFleetKpiCards({
  isArchive: isArchive.value,
  total: Number(recordStats.value[recordType.value] || 0),
  categories: menuCategories.value,
  subCounts: recordSubStats.value,
  subAmounts: recordSubAmount.value,
  singleCategory: meta.value.singleCategory || '',
  parseFailed: Number(overviewStats.value?.parse_failed || 0),
  extractFailed: Number(overviewStats.value?.extract_failed || 0),
}))

function onKpiCardClick(card: any) {
  if (card.action === 'history') {
    historyFilter.value = 'all'
    historyVisible.value = true
    return
  }
  // 「已收录…」是纯统计卡（action=stat），组件层已吞掉点击事件；
  // 这里再兜一层：任何非 type 卡都不切换列表、不发请求。
  if (card.action !== 'type' || !card.typeValue) return
  if (activeType.value === card.typeValue) return
  activeType.value = card.typeValue
  page.value = 1
  initColumns()
  reloadList()
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
// 只保留 keyword：日期区间与证照类型筛选已按需求移除，不再初始化空值
// （TDesign 的 date-range-picker 已不渲染，无需再防 undefined 的 console.error）。
// dateRange 必须是数组：TDesign date-range-picker 收到 undefined 会每帧
// console.error（真机「控制台 0 error」硬断言会失败）。
const initialFilterState = (): Record<string, any> => ({ keyword: '', certStatus: '', plate: '', dateRange: [] })
const filterState = ref<Record<string, any>>(initialFilterState())
const selectedKeys = ref<Set<string>>(new Set())
const selectedRowKeys = computed(() => Array.from(selectedKeys.value))
const rowKey = (r: any) => String(r?.id || '')

const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(rowKey(r))))
const selectedFileRows = computed(() => selectedRows.value.filter((r) => r.__file))

// 汇总视图：档案类菜单点「全部 / 已收录证照」卡片时（activeType 为空）。
// 此时没有「当前分类」，用跨子分类通用的核心语义列；选中具体分类后
// 严格由该分类 subs 派生。手工录入类永远有单一分类，不会进汇总视图。
const isSummaryView = computed(() => isArchive.value && !activeType.value)

// 归档类固定追加的两列（不进 subs：它们是派生值，用户不可编辑）
const CERT_STATUS_KEY = '__cert_status'
const EXTRACT_STATUS_KEY = '__extract_status'
// 当天日期（会话内固定）：证照状态按它算剩余天数。用本地日期而非 toISOString，
// 否则东八区 08:00 前会被算成昨天，把「今天到期」误判成「已过期」。
const todayStr = (() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
})()

const columnDefs = computed<FleetColumn[]>(() => {
  const base = isSummaryView.value
    ? buildFleetSummaryViewColumns(vehicleMap.value)
    : buildFleetColumns(activeSubs.value, vehicleMap.value)
  if (!isArchive.value) return base
  return [
    ...base,
    {
      key: CERT_STATUS_KEY,
      label: '证照状态',
      dataType: 'text',
      default: true,
      value: (row: any) => certStatusOf(row?.data, todayStr).label || '—',
      raw: () => 0,
    },
    {
      key: EXTRACT_STATUS_KEY,
      label: '提取状态',
      dataType: 'text',
      default: true,
      value: (row: any) => extractStatusOf(row).label,
      raw: () => 0,
    },
  ]
})
const columnDefsForFilter = computed(() => columnDefs.value.map((c) => ({ key: c.key, label: c.label })))

const visibleColKeys = ref<string[]>([])
// 列显隐按「菜单 + 子分类」分别记忆：同一菜单下不同子分类的字段集完全不同
// （许可证 10 个字段 vs 登记证书 35 个），共用一套会让切分类后列全空。
const STORAGE_KEY = computed(() => {
  const sub = isSummaryView.value ? '__summary__' : (activeCategoryName.value || meta.value.singleCategory || '__default__')
  return `weknora-fleet-${meta.value.scope}-${props.recordType}-${sub}-cols-v5`
})

const visibleCols = computed(() => columnDefs.value.filter((c) => visibleColKeys.value.includes(c.key)))

const tableColumns = computed(() => {
  // 与发票管理同款固定列：勾选 + 序号，二者都左固定，横向滚动时保持可见。
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 44, fixed: 'left' },
    { colKey: 'serial-number', title: '', width: 56, fixed: 'left' },
  ]
  const vis = visibleCols.value
  vis.forEach((c, i) => {
    const values = rows.value.slice(0, 30).map((r) => c.value(r))
    const w = colWidthOf(c.width, c.label, values, c.dataType)
    const base: any = {
      colKey: c.key,
      title: c.label,
      ellipsis: ellipsisOf(c),
      cell: (_h: any, params: any) => {
        const row = params?.row
        // 证照状态：三态三色 Tag（发票管理「提取成功」同色系）
        if (c.key === CERT_STATUS_KEY) {
          const st = certStatusOf(row?.data, todayStr)
          if (!st.key) return '—'
          return _h('t-tag', {
            theme: st.theme, variant: 'light', size: 'small',
          }, () => st.label)
        }
        // 提取状态：绿底成功 Tag
        if (c.key === EXTRACT_STATUS_KEY) {
          const st = extractStatusOf(row)
          return _h('t-tag', {
            theme: st.theme === 'success' ? 'success' : 'default', variant: 'light', size: 'small',
          }, () => st.label)
        }
        return c.value(row)
      },
      dataType: c.dataType,
    }
    // 最后一列给 minWidth（随视口伸展），其余给固定 width —— 与发票页一致
    if (i === vis.length - 1) base.minWidth = w
    else base.width = w
    const cls = cellClassOf(c)
    if (cls) base.className = cls
    cols.push(base)
  })
  return cols
})

const tableScroll = computed(() => tableScrollOf(visibleCols.value))

// 合计行的列定义（与 visibleCols 同源，可累加列才给 fieldType）
const summaryCols = computed(() => buildFleetSummaryColumns(visibleCols.value))
const footSpan = computed(() => footSpanOf(summaryCols.value))

// 筛选器：总览态按证照类型；其余由 subs 派生
// 左侧工具栏：**只有**「搜索全部字段」+ 刷新按钮（第 30 轮统一口径）。
//
// 为什么必须保留 keyword spec 而不是直接传空数组：BusinessFilterBar 的
// keywordSpec 来自 fields 里 type=keyword 的那条，传空数组会让
// keywordValue 恒为空、patch() 直接 no-op —— 搜索框变成打字没反应的死控件。
// 其余筛选（证照类型下拉 / 车牌 / 日期区间 / 子分类字段）全部移除。
const filterFields = computed<FilterFieldSpec[]>(() => {
  const out: FilterFieldSpec[] = [
    { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  ]
  if (!isArchive.value) return out
  // 证照状态：三个归档菜单都有（生效中 / 即将到期 / 已过期）
  out.push({
    key: 'certStatus', type: 'select', placeholder: '证照状态',
    options: certStatusOptions(), clearable: true,
  })
  // 维保记录：额外提供车牌号 + 维修日期区间（该菜单按车辆与时间段查得最频繁）
  if (props.recordType === 'maintain-archive') {
    out.push({ key: 'plate', type: 'text', placeholder: '车牌号', clearable: true })
    out.push({ key: 'dateRange', type: 'date-range', placeholder: '维修日期', icon: 'time' })
  }
  return out
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

// 表格数据必须过门禁：解析中/提取中的文件绝不进列表、不参与合计。
// 证照状态筛选（派生值，只能前端过滤；筛选期间已把 page_size 拉到 200）
const activeCertStatus = computed(() => String(filterState.value?.certStatus || '').trim())
const statusFilteredRows = computed(() => {
  const want = activeCertStatus.value
  if (!want) return gatedRows.value
  return gatedRows.value.filter((r) => certStatusOf(r?.data, todayStr).key === want)
})
const pagedRows = computed(() => statusFilteredRows.value)

// 合计行：勾选感知（选中时只汇总选中行；非累加列留空）。
// 数据源必须是 gatedRows（不是 rows）—— 在途文件既不入列，也不入合计。
const summaryScope = computed(() => summaryScopeRows(statusFilteredRows.value, selectedKeys.value, rowKey))
const footData = computed(() => [buildFootRow(summaryCols.value, summaryScope.value.rows, {
  selectKey: 'row-select', selectedCount: summaryScope.value.selectedCount,
})])

// 首屏门禁：卡片与列表首次请求都完成前，表格保持 loading —— 禁止在骨架屏
// 期间闪现「暂无数据」和「合计 (共 0 条)」（第 29 轮 bug 4）。
const tableLoading = computed(() => loading.value || !cardsReady.value)
// 未就绪时合计行不渲染，避免「合计 (共 0 条)」误导
const tableFootData = computed(() => (tableLoading.value ? [] : footData.value))

// 稀疏态逻辑见文件末尾（必须放在所有 ref/computed 之后，否则 watch 注册时会
// 立即求值 pagedRows → gatedRows → pendingFiles 而触发 TDZ）。





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
// bootstrap 里会 `filterState.value = initialFilterState()` 重置筛选，这会触发本
// 监听 → 300ms 后再发一次与 bootstrap 重复的列表请求（每个菜单切一次多一请求）。
// 用就绪门闩挡掉「程序性重置」，只响应用户真实输入。
let filterWatchReady = false
watch(filterState, () => { if (filterWatchReady) scheduleFilter() }, { deep: true })

async function reloadList() {
  loading.value = true
  try {
    const state = filterState.value || {}
    // 12 个菜单统一读 fleet_records（第 29 轮）。
    // 档案类不再在「全部」视图切到 knowledges 文件列表 —— 那条路径的数据源
    // 与 KPI 卡片不同表，必然出现「卡片 N 份、列表 M 条」的口径错位。
    // 现在「全部」= 该 record_type 的全部记录，子分类 = sub_category 精确过滤，
    // 两者与 /records/stats 的 where 条件逐字一致。
    const [df, dt] = Array.isArray(state.dateRange) ? state.dateRange : []
    // 证照状态是**派生值**（按有效期止与今天算），数据库里没有这一列，
    // 无法用 SQL 精确过滤（跨 PG/sqlite 的 JSON 日期运算不可移植）。
    // 因此状态筛选时把当前分类的记录一次拉大页（上限 200）再做前端过滤，
    // 并在筛选期间隐藏分页器 —— 避免「合计 N 条」与分页总数互相打脸。
    const statusFilter = String(state.certStatus || '').trim()
    const res: any = await listFleetRecords({
      type: recordType.value,
      sub_category: isArchive.value ? activeCategoryName.value : undefined,
      q: String(state.keyword || '').trim() || undefined,
      // 维保记录：车牌号走 field_filters（后端按 vehicle_id/车牌精确匹配）
      field_filters: String(state.plate || '').trim()
        ? `车牌号:${String(state.plate).trim()}`
        : undefined,
      date_from: df || undefined,
      date_to: dt || undefined,
      page: statusFilter ? 1 : page.value,
      page_size: statusFilter ? 200 : pageSize.value,
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
    const d = res?.data || {}
    recordStats.value = (d.by_type || {}) as Record<string, number>
    recordSubStats.value = (d.by_sub_category || {}) as Record<string, number>
    recordSubAmount.value = (d.sub_category_amount || {}) as Record<string, number>
  } catch { /* 统计失败不阻塞列表 */ }
  // 历史记录抽屉的异常文件统计仍走知识库总览（卡片数字不再依赖它）
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
// 档案类的文件列表已不再参与列表渲染（第 29 轮：列表统一读 fleet_records，
// 与 KPI 卡片同源）。原 ensureFileRows / fileRows 已删除，避免「两套数据源」
// 再次导致「卡片 N 份、列表 M 条」的口径错位。历史记录抽屉仍读知识库文件表。
// rawFileRows 保留：loadPending（在途门禁）需要原始文件列表判断解析/提取状态。
const rawFileRows = ref<any[]>([])

// docTypeOptions 已随「证照类型」下拉一并移除（第 30 轮工具栏极简）
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

// 从表单里挑出「金额语义」字段，写入通用 amount 列（费用清单汇总与合计行都读它）。
// 表单字段名由 subs 决定（含税金额/总费用/工时费/单价…），优先级从高到低。
// 金额/日期推导已抽到 fleetColumns.ts（纯函数，便于单测）：
// 这两个函数都出过口径 bug（金额把单价当总额、日期取不到），必须可测。

async function saveDetail() {
  const row = detailRow.value
  saving.value = true
  try {
    const data = { ...(editForm.value.data || {}) }
    const amount = Number(row?.amount) || amountFromForm(data)
    // 「金额」列必须与通用 amount 一致：列表金额列与底部合计行都读 data['金额']，
    // 只写 amount 列会出现「行内金额显示 —、合计 0.00」的自相矛盾。
    // 仅当该分类确实有「金额」字段且用户没手填时才回填（不覆盖用户输入）。
    const hasAmountField = drawerFields.value.some((f) => f.name === '金额')
    if (hasAmountField && amount > 0 && String(data['金额'] ?? '').trim() === '') {
      data['金额'] = amount
    }
    const payload: any = {
      record_type: row?.record_type || recordType.value,
      vehicle_id: row?.vehicle_id || '',
      record_date: row?.record_date || dateFromForm(data),
      record_month: row?.record_month || '',
      amount,
      remark: row?.remark || '',
      // 子分类必须显式写入（第 29 轮）：列表按 sub_category 过滤，
      // 缺这一列新建的手工记录会「保存成功但列表查不到」。
      sub_category: row?.sub_category || activeCategoryName.value || meta.value.singleCategory || '',
      data,
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

// 行内操作列已移除（第 31 轮：对齐发票管理，没有右侧冻结列）。
// 单条删除走「点行 → 详情抽屉 → 删除」，多条走批量浮条 handleDelete。

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
// 空状态文案统一为「暂无数据」+ 一句操作指引（见 fleetColumns.fleetEmptyState）
const emptyState = computed(() => fleetEmptyState(isArchive.value))
const emptyText = computed(() => emptyState.value.description)
const emptyHint = computed(() => emptyState.value.hint)

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------
async function bootstrap() {
  cardsReady.value = false
  selectedKeys.value = new Set()
  page.value = 1
  filterWatchReady = false
  filterState.value = initialFilterState()
  await Promise.all([loadVehicles(), ensureKb()])
  await reloadCategories()
  // 默认焦点（第 30 轮）：档案类直接选中第 1 个启用子分类，而不是「已收录」汇总卡。
  // 汇总视图下各分类字段集不同，只能拼通用列且大量「—」；默认落到具体分类后
  // 用户一进来就看到真实表头与数据。无任何启用分类时退化为汇总视图（空串）。
  //
  // 必须在 reloadCategories() **之后**算：分类是异步拉的，提前算只能拿到空数组，
  // 结果永远是汇总视图（默认焦点失效）。
  activeType.value = isArchive.value
    ? (() => {
      const first = resolveDefaultCategoryName(menuCategories.value)
      return first ? 'cat-' + first : ''
    })()
    : 'cat-' + (meta.value.singleCategory || '')
  initColumns()
  await reloadStats()
  await reloadList()
  // 列表已就绪，之后 filterState 的任何变化都是用户输入 → 交给防抖请求
  filterWatchReady = true
  // 注：**不做**发票派生同步（第 29 轮明确取消跨库同步）。
  // 加油充电 / 路桥费是独立手工台账，数据由用户录入。
  startPolling()
}

watch(() => props.recordType, () => { void bootstrap() })

// 稀疏态：仅当数据行未填满容器时才启用「伪填充行」把合计行顶到底部。
// 必须实测：直接无条件给 tbody 加 height:100% 的伪行，在数据满页时
// 会额外撑出一整屏空白（表格高度 100% × 伪行 100%）。
const sparseTable = ref(false)
const tableBoxRef = ref<HTMLElement | null>(null)
function measureSparse() {
  const box = tableBoxRef.value?.querySelector('.t-table__content') as HTMLElement | null
  const body = tableBoxRef.value?.querySelector('.t-table__body') as HTMLElement | null
  if (!box || !body) { sparseTable.value = false; return }
  // 预留合计行自身高度（约 40px），否则临界情况会误判成稀疏
  sparseTable.value = body.scrollHeight + 40 < box.clientHeight
}
// 位置很关键：watch 的数组 source 在**注册时**就会读取一次（触发 computed 求值），
// pagedRows → gatedRows → pendingFiles 里 pendingFiles 定义在文件后段，
// 因此这段必须放在所有依赖之后，否则 setup 直接 TDZ 白屏。
watch([pagedRows, tableLoading], () => { nextTick(measureSparse) })

onMounted(() => {
  void bootstrap()
  nextTick(measureSparse)
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
/* 中台列表样式（第 31 轮补）：
   发票 / 合同 / 制度 / 奖惩 / 能耗 / 计量 / 账单 8 个页面都 import 了这份 CSS，
   唯独车队页漏了 —— 结果 .doc-list-scroll 没有 flex 与卡片边框、.t-table 没有
   height:100%，合计行只能贴着内容悬在半空（真机图 3/4/5）。
   这里补上后高度链路才完整：.biz-page(100%) → .biz-list-view(flex:1) →
   .doc-list-scroll(flex:1 + overflow:hidden) → .t-table(height:100%)。 */
@import './styles/business-list-shared.css';

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
/* 合计行贴底（第 31 轮）：
   TDsign 把 tfoot 渲染在滚动容器内部（.t-table__content，position:sticky），
   容器高度只有「表头 + 数据行」，所以只有 1 行数据时合计条悬在半空。
   这里把滚动容器改成 flex column、让合计行用 margin-top:auto 顶到底部，
   数据多时仍随内容滚动（sticky 保留），数据少时贴容器底。 */
.fleet-page :deep(.t-table) {
  display: flex;
  flex-direction: column;
}
.fleet-page :deep(.t-table__content) {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
/* 表格撑满滚动容器 + 伪填充行。
   tfoot 是 <table> 的子元素，只能随表格下沉；直接给 table 设 height:100%
   会把**数据行本身**拉高（实测 1 行被拉到 477px，行内会出现诡异空档）。
   正确做法是让 tbody 末尾长出一条空的 table-row 吃掉剩余高度，
   数据行保持 39px 原高，合计行自然贴到容器底。 */
.fleet-page :deep(.t-table__content > table) {
  flex: 1;
}
.fleet-page :deep(.fleet-table--sparse .t-table__content > table) {
  height: 100%;
}
.fleet-page :deep(.fleet-table--sparse .t-table__body)::after {
  content: '';
  display: table-row;
  height: 100%;
}
.fleet-page :deep(.t-table__footer) {
  margin-top: auto;
}

/* 注意：这里**不要**覆盖 .doc-list-scroll。
   中台 business-list-shared.css 已把它定义成 flex 容器 + overflow:hidden + 卡片边框，
   表格靠 .t-table{height:100%} 撑满、合计行才能贴容器底。
   早期这里写过 `overflow-y:auto`，结果表格退化成内容高度 → 合计行悬在半空，
   而且卡片的圆角/边框也一起丢了（真机图 3/4/5 的红框就是这么来的）。 */
.fleet-pager { display: flex; justify-content: flex-end; padding: 8px 0 4px; }
</style>
