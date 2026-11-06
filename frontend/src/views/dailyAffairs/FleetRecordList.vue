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

      <!-- ③ 列表：与发票管理逐字同构（.doc-list-view + :max-height="100%" + sticky-header） -->
      <div class="doc-list-scroll">
        <!-- 表头就绪前只渲染占位块：宁可空着，也不能先用「复选框 + 状态列」
             渲染一帧再蹦业务列（需求 4：第 1 帧必须一次性出现全部列）。 -->
        <div v-if="!headerReady" class="doc-list-view">
          <div class="fleet-empty-placeholder" />
        </div>
        <div v-else :class="listViewClass" :style="headerFixStyle">
          <t-table :data="pagedRows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="tableLoading" :max-height="tableMaxHeight"
            :foot-data="tableFootData"
            :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header
            class="doc-table fleet-table"
            :row-class-name="({ row }: any) => selectedKeys.has(rowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <!-- 证照状态：**三色只读 Tag**（第 37 轮，需求图 5）
                 颜色与需求给的值一一对应，TDesign light 变体即浅底深字：
                   生效中 → success（#e8f8f0 / #00a870）
                   即将到期 → warning（#fff3e6 / #ed7b2f）
                   已过期 → danger（#fdecee / #e34d59）
                 空态（没填有效期）显示「—」，不强行造状态。 -->
            <template #__cert_status="{ row }: any">
              <t-tag v-if="certStatusOf(row?.data, todayStr).label"
                :theme="certThemeOf(row)" variant="light" size="small">
                {{ certStatusOf(row?.data, todayStr).label }}
              </t-tag>
              <span v-else>—</span>
            </template>
            <!-- 提取状态：同为派生值，绿底成功 Tag -->
            <template #__extract_status="{ row }: any">
              <t-tag :theme="extractStatusOf(row).theme === 'success' ? 'success' : 'default'"
                variant="light" size="small">{{ extractStatusOf(row).label }}</t-tag>
            </template>
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
      <!-- 证照状态：系统派生值，**只读彩色 Tag**（第 37 轮，需求 4.2）
           视觉与「提取状态」完全一致：浅底 + 深字 + 同尺寸，只是颜色按三态变。
           旧实现是 disabled 输入框 —— 灰底灰字长得像「可编辑但被锁住」，
           与旁边同为派生值的提取状态（绿 Tag）风格断裂，用户以为这里漏填了。
           用插槽而不是改共享抽屉组件，是为了不动中台契约（发票/能耗的字段
           渲染分支保持原样）。 -->
      <template #field-__cert_status>
        <div class="bsd-readonly-tag">
          <t-tag v-if="detailCertStatus && detailCertStatus !== '—'"
            :theme="detailCertTheme" variant="light" size="small">{{ detailCertStatus }}</t-tag>
          <span v-else class="bsd-readonly-empty">—</span>
        </div>
      </template>
      <!-- 提取状态：同为派生值，抽屉里也是只读 Tag（与列表同款） -->
      <template #field-__extract_status>
        <div class="bsd-readonly-tag">
          <t-tag :theme="detailExtractTheme" variant="light" size="small">{{ detailExtractLabel }}</t-tag>
        </div>
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
  listKnowledgeBases, createKnowledgeBase, getFleetOverviewStats,
  reparseKnowledge, delKnowledgeDetails,
} from '@/api/knowledge-base'
import {
  listFleetVehicles, listFleetRecords, createFleetRecord, updateFleetRecord, deleteFleetRecord,
  listFleetCategories, getFleetRecordStats, extractFleetDocument,
} from '@/api/fleet'
import { generateCatalogPdf } from './useCatalogPdf'
import { useTableFlatState, flatClassOf } from './useTableFlatState'

import { isExtractInProgress } from '@/composables/extractConflict'
import {
  useUploadHistoryScope, runningUploadHistoryFiles,
} from '@/composables/useUploadHistoryFiles'
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
import { useTableHeaderFix } from './useTableHeaderFix'
import { buildFootRow, footSpanOf, summaryScopeRows } from './listSummary'
import './styles/list-summary-footer.css'
import { filterRowsByPendingFiles } from './invoicePendingGate'
import { fleetHistoryFilesOf } from './fleetHistoryStats'
import {
  buildFleetColumns, buildFleetSummaryColumns, buildFleetSummaryViewColumns,
  amountFromForm, dateFromForm, resolveDefaultCategoryName, buildFleetKpiCards, fleetEmptyState,
  certStatusOf, certStatusOptions, extractStatusOf, subCategoryMatches,
  type FleetColumn,
} from './fleetColumns'
import { fleetCategorySeedsOf } from './fleetCategorySeed'
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
// 第 42 轮：全模块共享上传历史数据源。状态条 / 历史卡 / 右侧抽屉读同一份数组，
// 由统一 2.5s 轮询驱动；页面不再各自拉文件列表、不再各自定时提取。
const uploadHistory = useUploadHistoryScope('fleet', () => kbId.value)
// 在途全部落定的瞬间刷新主列表与 KPI 卡片（无需用户关开抽屉/刷新网页）
uploadHistory.onSettled(() => { reloadList(); reloadStats() })
const loading = ref(false)
// 最近一次**成功渲染**的列表数据源 key（菜单+分类+筛选+分页）。
// 用于判断 reloadList 是「换数据源」（需要清空旧行）还是「同源刷新」（不能清空）。
const lastListKey = ref('')
// 别名兜底只做一次的分类键（`recordType|分类名`），避免空分类反复多打请求。
const aliasTried = new Set<string>()
const cardsReady = ref(false)
const vehicles = ref<any[]>([])
const categories = ref<Record<string, any[]>>({})
const vehicleMap = computed(() => {
  const m = new Map<string, string>()
  for (const v of vehicles.value) if (v?.id) m.set(v.id, v.plate_no || '')
  return m
})
const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.plate_no })))

// 当前菜单的分类列表（六同步数据源）。
//
// **第 1 帧就必须完整**（第 34 轮）：分类原先只能异步拉取，首屏 `menuCategories`
// 是空数组 → KPI 卡片只剩「已收录 / 历史记录」两张、`activeSubs` 为空 →
// 表头只剩复选框与两个系统列，接口回来后才「蹦」出其余卡片与业务列
// （真机：先 2 张卡停顿一下、表头先只有 2 列）。
//
// 现在：服务端已返回 → 用服务端数据（权威，用户在设置里改的字段/顺序/启停都生效）；
// 还没返回 → 用前端同步种子（与后端 seed 同源）占位。两种形态结构完全一致，
// 下游派生逻辑（卡片 / 表头 / 抽屉字段）无需区分。
const menuCategories = computed<any[]>(() => {
  const remote = categories.value[meta.value.scope]
  const arr = Array.isArray(remote) && remote.length ? remote : fleetCategorySeedsOf(meta.value.scope)
  return arr.filter((c: any) => c && c.enabled !== false)
})

// 当前激活分类：档案类空 = 总览（文件列表）；记录型锁定菜单分类。
//
// **初值必须同步算出来**（第 34 轮）：早先初始化为空串、要等 onMounted 的
// bootstrap 异步走完才赋值，于是首帧 `activeSubs` 为空 → 表头只剩复选框与
// 两个系统保留列（真机图 1/2/3 的残缺表头）。现在用同步种子直接算出默认焦点：
// 档案类 = 第一个启用子分类；记录型 = 该菜单唯一分类。接口返回后 bootstrap
// 会用服务端分类再校正一次（用户可能停用了某个分类）。
const activeType = ref(
  isArchive.value
    ? (() => {
      const first = resolveDefaultCategoryName(fleetCategorySeedsOf(meta.value.scope))
      return first ? 'cat-' + first : ''
    })()
    : 'cat-' + (meta.value.singleCategory || ''),
)
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
  // 「历史记录」必须等于抽屉「全部文件」，副文案跟随在途/异常
  historyCount: historyFileCount.value,
  running: runningCount.value,
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
const initialFilterState = (): Record<string, any> => ({ keyword: '', certStatus: '', dateRange: [] })
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

/** 证照状态的三色主题：生效中绿 / 即将到期橙 / 已过期红（需求图 5 的色值）。 */
const certThemeOf = (row: any): 'success' | 'warning' | 'danger' => {
  const st = certStatusOf(row?.data, todayStr)
  return st.theme === 'success' ? 'success' : st.theme === 'warning' ? 'warning' : 'danger'
}

// 系统保留列名：这些字段在 subs 里是**历史遗留的可编辑文本列**（旧种子把它们
// 当普通字段录入），现在统一由系统按有效期派生，必须从业务列里剔除，
// 否则真机上会同时出现「驾驶证状态」与「证照状态」两列（需求图 15）。
const SYSTEM_STATUS_FIELD_NAMES = new Set(['证照状态', '证件状态', '驾驶证状态', '保单状态', '证书状态'])

const columnDefs = computed<FleetColumn[]>(() => {
  const base = isSummaryView.value
    ? buildFleetSummaryViewColumns(vehicleMap.value)
    : buildFleetColumns(activeSubs.value, vehicleMap.value)
  if (!isArchive.value) return base
  return [
    // 归档类：剔除 subs 里的历史状态文本列（系统统一派生「证照状态」）
    ...base.filter((c) => !SYSTEM_STATUS_FIELD_NAMES.has(c.key)),
    {
      key: CERT_STATUS_KEY,
      label: '证照状态',
      dataType: 'text',
      default: true,
      value: (row: any) => certStatusOf(row?.data, todayStr).label || '—',
      raw: () => 0,
      // 需求图 5：表头曾被截断成「证照状」—— 三色 Tag 比纯文本更宽，
      // 必须给足 100px（Tag 四周 padding + 4 个汉字）。
      minWidth: 100,
    },
    {
      key: EXTRACT_STATUS_KEY,
      label: '提取状态',
      dataType: 'text',
      default: true,
      value: (row: any) => extractStatusOf(row).label,
      raw: () => 0,
      minWidth: 100,
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
  // 第 38 轮（需求图 44/46）：**取消所有冻结列**。
  //
  // 原来勾选列与序号列是 `fixed: 'left'`，横向滚动时它们被钉在左侧并画一条
  // 分割竖线，把后面的业务列文字压掉半截（真机：许可证号只剩「订字5002…」）。
  // 现在全部列（含勾选/序号/状态）一起随横向滚动条滑动，表格是一个整体。
  const cols: any[] = [
    { colKey: 'row-select', type: 'multiple', width: 44 },
    { colKey: 'serial-number', title: '', width: 56 },
  ]
  const vis = visibleCols.value
  vis.forEach((c, i) => {
    const values = rows.value.slice(0, 30).map((r) => c.value(r))
    // minWidth 是**列的下限**（证照状态/提取状态需要 100px，否则表头会被
    // TDesign 的 ellipsis 截成「证照状」）。有下限时按它兜住估计值。
    const minW = Number((c as any).minWidth) || 0
    const w = Math.max(colWidthOf(c.width, c.label, values, c.dataType), minW)
    const base: any = {
      colKey: c.key,
      title: c.label,
      ellipsis: ellipsisOf(c),
      // 两列派生状态走**模板插槽**（#__cert_status / #__extract_status）。
      //
      // 不能用 `cell: (_h, params) => _h('t-tag', …)`：TDesign 1.19 的 cell
      // 回调不注入 h，`_h` 恒为 undefined → 两列渲染成空白（真机实测
      // 单元格 text 为空、无 .t-tag）。发票页用的是具名插槽，这里对齐它。
      cell: c.key === CERT_STATUS_KEY || c.key === EXTRACT_STATUS_KEY
        ? undefined
        : (_h: any, params: any) => c.value(params?.row),
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
  // 维保记录：额外提供维修日期区间（按时间段查是该菜单最高频的操作）。
  // 车牌号筛选已删除（第 34 轮）：车牌已在「搜索全部字段」里可搜，
  // 单独一个车牌输入框既重复又占位（需求图 7 明确划掉）。
  if (props.recordType === 'maintain-archive') {
    out.push({ key: 'dateRange', type: 'date-range', placeholder: '维修日期', icon: 'time' })
  }
  return out
})

function persistColumns() {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
function resetColumns() {
  const defs = columnDefs.value.filter((c) => c.default).map((c) => c.key)
  if (defs.length) {
    visibleColKeys.value = defs
  } else {
    // 该分类一个默认列都没标（如「一般维修」7 个字段 is_default 全 false）：
    // 不能把所有列都摊开（会把 35 字段的分类撑成横向滚动条），
    // 退化为「前 8 个业务列」，保证表头有实质内容且不至于过宽。
    // 系统保留列永远追加在末尾，由 columnDefs 顺序保证。
    visibleColKeys.value = columnDefs.value.slice(0, 8).map((c) => c.key)
  }
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
      // 记忆值的接受条件（第 34 轮收紧）：
      //   ① 记忆里至少有一个业务列 —— 只含 __cert_status / __extract_status 的
      //      记忆是上一轮字段集变化留下的残骸；
      //   ② **所有**默认列都还在 —— 旧版字段集留下的记忆会缺列
      //      （真机：辅材记录只剩 5 列、请车记录只剩 4 列，因为旧版种子默认列更少），
      //      `defs.every` 对这种「部分缺失」恒成立，于是脏记忆被一直沿用，
      //      表现为「设置里有字段、列表里没有表头」。
      //   ③ defs 为空（该分类一个默认列都没标）时，退化为只要求 ①。
      const hasBusiness = matched.some((k) => k !== CERT_STATUS_KEY && k !== EXTRACT_STATUS_KEY)
      const defsOk = defs.length === 0 ? true : defs.every((k) => matched.includes(k))
      // 额外兜底：业务列数量不能少于默认列数量（挡住「旧记忆比现定义少」的残骸）
      const businessCount = matched.filter((k) => k !== CERT_STATUS_KEY && k !== EXTRACT_STATUS_KEY).length
      const countOk = defs.length === 0 || businessCount >= defs.length
      if (matched.length && hasBusiness && defsOk && countOk) {
        visibleColKeys.value = matched
        return
      }
    } catch { /* ignore */ }
  }
  resetColumns()
}

/**
 * 表头就绪：有业务字段才渲染表格（第 39 轮，需求 4）。
 *
 * 真机现象：进入车队先出现复选框 + 两个状态列，随后业务列「依次蹦出来」。
 * 根因是异步分类没回来前 `activeSubs` 为空，`columnDefs` 也跟着空，
 * TDesign 就先用「只剩内置列」的表头渲染了一帧。
 *
 * 现在：没有业务字段就**不渲染表头**（显示占位块），有就一次性全出 ——
 * 标准分类由同步种子兜底（第 1 帧即完整），自定义分类等分类接口回来一次成型。
 */
const headerReady = computed(() => columnDefs.value.length > 0)

// setup 阶段先算一次列显隐：`columnDefs` 已由同步种子得出，
// 不先算的话首帧 `visibleCols` 为空 → 表头只有复选框与序号（真机图 1/2/3）。
// bootstrap 里分类到位后会再调一次 initColumns 校正。
initColumns()

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

// 表格 loading 只由「列表请求在途」决定。
//
// 旧实现是 `loading || !cardsReady`，而 cardsReady 要等卡片统计接口返回 ——
// 于是切换菜单/分类时表格会先渲染出一段「既非 loading 也无数据」的空窗期：
// 表头不完整（分类还没到）、tbody 里先出现一个孤立序号 1 的空壳行。
// 现在卡片与表头都有同步种子，第 1 帧即完整；loading 只跟列表请求走。
const tableLoading = computed(() => loading.value)
// 未就绪时合计行不渲染，避免「合计 (共 0 条)」误导
const tableFootData = computed(() => (tableLoading.value ? [] : footData.value))

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
  // 清空旧行**只在切换数据源时**做（菜单/分类/筛选条件变化），
  // 不能无条件清空：同一数据源的分页/轮询刷新也走这里，
  // 无条件清空会与在途请求形成竞态 —— 后发的清空把先到的结果抹掉，
  // 真机表现为「接口返回 1 条、页面 0 行、合计（共 0 条）」。
  //
  // 判据：当前请求的 key 与上次成功渲染的 key 不同才清空。
  const reqKey = [
    recordType.value,
    isArchive.value ? activeCategoryName.value : meta.value.singleCategory,
    JSON.stringify(filterState.value || {}),
    page.value,
    pageSize.value,
  ].join('|')
  const changed = reqKey !== lastListKey.value
  if (changed) {
    rows.value = []
    total.value = 0
    selectedKeys.value = new Set()
  }
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
      date_from: df || undefined,
      date_to: dt || undefined,
      page: statusFilter ? 1 : page.value,
      page_size: statusFilter ? 200 : pageSize.value,
    })
    const data = res?.data || {}
    let items: any[] = Array.isArray(data.items) ? data.items : Array.isArray(data) ? data : []
    let totalNum = Number(data.total) || items.length
    // 子分类别名兜底（第 38 轮，需求 5.2）：库里历史记录可能把分类存成英文 key
    // （driver_license / driving_license），而后端是 `sub_category = ?` 精确匹配
    // → 中文名查不到它们，页面显示「暂无数据」。
    // 命中 0 条时按该分类的全部别名重查一次（先不带分类、前端过滤），
    // 结果为空也是合法结论，所以每个「菜单+分类」只试一次，避免每次翻页多打一次请求。
    const aliasKey = `${recordType.value}|${activeCategoryName.value}`
    if (isArchive.value && items.length === 0 && activeCategoryName.value && !aliasTried.has(aliasKey)) {
      aliasTried.add(aliasKey)
      const fallback: any = await listFleetRecords({
        type: recordType.value,
        q: String(state.keyword || '').trim() || undefined,
        date_from: df || undefined,
        date_to: dt || undefined,
        page: 1,
        page_size: 200,
      })
      const fdata = fallback?.data || {}
      const all: any[] = Array.isArray(fdata.items) ? fdata.items : Array.isArray(fdata) ? fdata : []
      const hit = all.filter((r) => subCategoryMatches(r?.sub_category, activeCategoryName.value))
      if (hit.length) {
        items = hit
        totalNum = hit.length
      }
    }
    rows.value = items
    total.value = totalNum
    // 记住本次已渲染的数据源 key：后续同 key 的刷新不再清空（避免竞态抹掉结果）
    lastListKey.value = reqKey
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
// 车队知识库的**全量文件快照**（第 39 轮）。
//
// 它是三处数字的唯一来源：
//   · 「历史记录」卡片的份数  == 抽屉「全部文件」；
//   · 状态条「正在解析/提取 N 个文件」；
//   · 在途门禁（在途文件对应的记录不进列表）。
// 旧实现只取第一页 100 条、还按文件元数据里的 scope 过滤，历史文件缺 scope
// 时被整批丢掉 —— 于是抽屉里 13 份、卡片 2 份、状态条永远不出现。
const fleetFiles = computed<any[]>(() => uploadHistory.uploadHistoryFiles.value as any[])

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

const drawerFields = computed(() => {
  const base = (activeSubs.value || [])
    .filter((s: any) => s && s.enabled !== false)
    // 归档类：剔除历史状态文本列（列表已由系统派生的「证照状态」取代）
    .filter((s: any) => !(isArchive.value && SYSTEM_STATUS_FIELD_NAMES.has(String(s.name))))
    .map((s: any) => ({ name: String(s.name), label: String(s.label || s.name), dataType: String(s.data_type || 'text') }))
  if (!isArchive.value) return base
  // 两个系统派生值都以**只读 Tag** 展示（模板 #field-__cert_status / #field-__extract_status）。
  // 不放进 editForm.data：避免脏检查把派生值当用户输入提交给后端。
  // 提取状态也必须进抽屉：它是判断「这条记录到底提没提出来」的唯一锚点，
  // 列表里能看、进详情反而看不到，用户会以为数据丢了。
  return [
    ...base,
    { name: CERT_STATUS_KEY, label: '证照状态', dataType: 'text' },
    { name: EXTRACT_STATUS_KEY, label: '提取状态', dataType: 'text' },
  ]
})
const drawerSubFields = computed(() => [])

// 抽屉里「证照状态」的展示值：按当前编辑中的有效期止实时联动
// （用户改了有效期止，状态要立刻跟着变，不能只在保存后才更新）。
const detailCertStatus = computed(() => certStatusOf(editForm.value?.data, todayStr).label || '—')
/**
 * 抽屉里证照状态 Tag 的主题与文案。
 *
 * 跟随 `editForm` 实时算：用户改「有效期止」时 Tag 必须立刻变色 ——
 * 这正是需求「随有效期止日期的修改自动重算更新 Tag 状态」的要求。
 */
const detailCertTheme = computed(() => {
  const st = certStatusOf(editForm.value?.data, todayStr)
  return st.theme === 'success' ? 'success' : st.theme === 'warning' ? 'warning' : 'danger'
})
/** 抽屉里的提取状态（与列表同一个 extractStatusOf，颜色语义一致）。 */
const detailExtractLabel = computed(() => extractStatusOf(detailRow.value).label)
const detailExtractTheme = computed(() =>
  extractStatusOf(detailRow.value).theme === 'success' ? 'success' : 'default')

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
/**
 * 本菜单的上传文件（抽屉「全部文件」同源）。
 *
 * 只按**证照类型**归属菜单，不再要求文件元数据里带 scope ——
 * 历史文件（早期版本上传）没有 scope 字段，按 scope 过滤会让它们从卡片与
 * 状态条里整批消失（真机：抽屉 13 份、卡片 2 份、状态条不出现）。
 */
const historyFileRows = computed(() => fleetHistoryFilesOf(fleetFiles.value, currentDocTypes.value))
/** 「历史记录」卡片数字 = 抽屉「全部文件」。 */
const historyFileCount = computed(() => historyFileRows.value.length)
/** 正在解析/提取的文件数（状态条/卡片副文案）。 */
// 全域感知：无论当前焦点在哪张卡片，只要有 1 份文件在途就显示绿色状态条。
const runningCount = computed(() => uploadHistory.runningCount.value)
/** 在途文件（门禁 + 状态条）——与抽屉同源，不按当前菜单过滤。 */
const pendingFiles = computed(() => runningUploadHistoryFiles('fleet'))
const uploadProgress = ref<{ name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]>([])
const progressIndex = ref(0)
let progressTimer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null

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

async function extractFile(kid: string, scope: string, certType?: string) {
  const payload: any = { cert_type: certType || undefined }
  if (isArchive.value) payload.scope = scope
  else payload.record_type = recordType.value
  await extractFleetDocument(kbId.value, kid, payload)
}

async function loadPending() {
  if (!kbId.value || !isArchive.value) return
  // 第 42 轮：页面轮询不再自动补提。提取只由上传弹窗与「重新提取」按钮显式触发，
  // 刷新页面 / 切换 Tab 绝不 POST 提取接口（根除无限重提死循环）。
  const nowRunning = uploadHistory.runningCount.value
  if (lastRunningCount > 0 && nowRunning === 0) {
    reloadList()
    reloadStats()
  }
  lastRunningCount = nowRunning
}
let lastRunningCount = 0

function startPolling() {
  stopPolling()
  // 统一轮询由 useUploadHistoryFiles 负责；这里只保留上传进度轮播定时器。
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
  // 切换一级菜单时**先把子分类焦点落到本菜单第 1 个分类**，再发列表请求（第 38 轮）。
  //
  // 真机事故（图 47）：车辆档案 → 司机档案，顶部「驾驶证 1 份」已亮，列表却
  // 「暂无数据 / 合计 0 条」，必须点刷新。根因是竞态：`activeType` 只在 setup
  // 时算过一次，切菜单后仍是 `cat-行驶证`；而 bootstrap 里 `reloadList()` 与
  // `reloadCategories()` 并行发出 → 列表带着上一个菜单的分类名去查，必然 0 条；
  // 分类接口回来后虽然把 activeType 改成了「驾驶证」，却没有再拉一次列表。
  //
  // 现在：① 用同步种子（与后端 seed 同源）立即重置焦点，第一帧的请求就带对分类；
  //       ② 服务端分类到位后若校正了焦点，再补一次列表请求。
  if (isArchive.value) {
    const seeded = resolveDefaultCategoryName(fleetCategorySeedsOf(meta.value.scope))
    activeType.value = seeded ? 'cat-' + seeded : ''
  }
  // 四路并行（第 34 轮）：vehicles / kb / 分类 / (统计 + 列表)。
  //
  // 串行是首屏慢的根因：`vehicles+kb → 分类 → stats → list` 四次往返，
  // 跨洋链路每次 3~10s，实测首屏要 16s 才出数据（截图里一直是「0 份 + 暂无数据」）。
  const catsP = reloadCategories()
  const dataP = Promise.all([reloadStats(), reloadList()])
  await Promise.all([loadVehicles(), ensureKb()])
  await catsP
  // 分类到位后用服务端数据校正默认焦点（种子里的第一个分类可能被用户停用/改名）
  if (isArchive.value) {
    const first = resolveDefaultCategoryName(menuCategories.value)
    const want = first ? 'cat-' + first : ''
    if (want !== activeType.value) {
      activeType.value = want
      initColumns()
      // 焦点变了 → 之前那次列表请求查的是别的分类，必须按新分类重拉，
      // 否则「卡片有数、列表为空」会一直停在那里等用户点刷新。
      await reloadList()
    }
  }
  // 列显隐：首帧（同步种子）已经算过列，**不要再算一遍** (第 39 轮)。
  // 服务端配置与种子若有差异（老库少一个默认列），重算会让表头在一帧后少一列
  // （真机 13 → 12 的跳动）。只有在完全没有业务列时（自定义分类，种子不认识）
  // 才在分类到位后补算一次 —— 那一次是**一次性成型**，不会分批。
  if (!visibleColKeys.value.length) initColumns()
  await dataP
  // 文件快照：卡片「历史记录」与在途状态条的数字来源。
  // 首屏必须拉到，否则卡片先显示 0 份、几秒后跳到 13 份。
  void loadPending()
  // 列表已就绪，之后 filterState 的任何变化都是用户输入 → 交给防抖请求
  filterWatchReady = true
  // 注：**不做**发票派生同步（第 29 轮明确取消跨库同步）。
  // 加油充电 / 路桥费是独立手工台账，数据由用户录入。
  startPolling()
}

watch(() => props.recordType, () => { void bootstrap() })

// 列显隐必须跟着「菜单 + 分类」走（第 38 轮）。
//
// 每个子分类有独立的列记忆键（STORAGE_KEY），切菜单/切卡片后如果不重新读一遍，
// `visibleColKeys` 还是上一个分类的键 —— 与当前 columnDefs 求交集后只剩**两套字段
// 的重叠部分**（真机：车辆档案 → 司机档案，驾驶证只剩「有效期起/止/发证机关」3 列，
// 用户以为数据没加载出来）。
watch(STORAGE_KEY, () => { initColumns() })

// 表头右侧滚动条轨道遮罩（与发票管理共用同一 hook 与同一 CSS）
const { headerFixStyle } = useTableHeaderFix()

// 表格高度策略：与发票管理逐字一致 —— `:max-height="'100%'"`。
//
// 为什么不再自创占位行（第 34 轮，已删除上一轮的像素占位方案）：
//   TDesign 的 tfoot 是 `position: sticky; bottom: 0`，只要滚动容器有高度上限
//   （max-height:100% + sticky-header），合计行天然贴在容器底边；数据少时
//   tbody 自动撑满剩余空间，数据行高度恒为内容高（不会被拉伸）。
//   上一轮用 JS 实测剩余空间 + `tbody::after` 像素占位，是自己造了一套
//   「表格高度计算」，直接导致：loading 期占位未算出 → 首行被拉高居中、
//   伪行 border 与 tfoot border 叠成双横线、满页数据多撑一屏空白。
//   全部删除，回归中台标准实现。
const tableMaxHeight = computed(() => '100%')

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

// 列表空态/加载态禁止横向滚动条（第 37 轮）：loading 或 0 行时挂 is-table-flat，
// 样式在 styles/business-list-shared.css 统一收口。
const { isTableFlat } = useTableFlatState({ loading: loading, rows: rows })
const listViewClass = computed(() => flatClassOf(isTableFlat.value))
</script>

<style lang="less" scoped>
/* 中台列表样式（第 31 轮补）：
   发票 / 合同 / 制度 / 奖惩 / 能耗 / 计量 / 账单 8 个页面都 import 了这份 CSS，
   唯独车队页漏了 —— 结果 .doc-list-scroll 没有 flex 与卡片边框、.t-table 没有
   height:100%，合计行只能贴着内容悬在半空（真机图 3/4/5）。
   这里补上后高度链路才完整：.biz-page(100%) → .biz-list-view(flex:1) →
   .doc-list-scroll(flex:1 + overflow:hidden) → .t-table(height:100%)。 */

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

/* 表格：**完全复刻 InvoiceManagement.vue**，不写任何自定义高度规则。
 *
 * 第 37 轮真机事故（图 5）：只有 1 条记录时，第 1 行离表头近 250px，
 * 悬在表格半空中。根因是下面这段（已删除）：
 *   .t-table{display:flex} / .t-table__content{flex:1} / table{flex:1;height:100%}
 * `table{height:100%}` 在 CSS 表格布局里是「最小高度」语义 —— 容器拉满后，
 * 多出来的高度会被**分配给行**，唯一那一行就被撑到整屏高、内容垂直居中。
 * 而 `tfoot{margin-top:auto}` 又把合计行顶到底，看起来就像数据行飘在半空。
 *
 * 发票页零自定义 CSS 就能正确贴顶：`.doc-list-view`（中台 flex 容器）+
 * `:max-height="100%"` + `sticky-header`。TDesign 的 tfoot 是 `position:sticky;
 * bottom:0`，数据满屏时自然贴底；不满屏时紧跟最后一行 —— 这才是中台标准行为，
 * 也是需求「thead 与 tbody 首行 0px 紧贴」的前提。
 *
 * 铁律：**永远不要为「顶合计栏」去改 table / tbody / tr 的高度或布局**。 */

/* 证照状态三色 Tag（第 37 轮）：与「提取状态」同一套 Tag 视觉，靠颜色区分三态。
 * 颜色值居中台 design token，不在模板里写死 style。 */
.cell-cert-active :deep(.t-tag) { --td-tag-light-primary-color: #00a870; }
.fleet-pager { display: flex; justify-content: flex-end; padding: 8px 0 4px; }
/* 抽屉里的只读派生值（证照状态 / 提取状态）：与普通字段左对齐、垂直居中，
   不出现输入框的边框感 —— 让用户一眼看出「这是算出来的，不是填的」。
   插槽内容渲染在父组件作用域（FleetRecordList），所以样式必须写在这里，
   不能落到 BusinessSplitDetailDrawer 的 scoped 块里。 */
.bsd-readonly-tag {
  display: flex;
  align-items: center;
  min-height: 24px;
}
.bsd-readonly-empty {
  color: var(--td-text-color-placeholder);
}
</style>

<style>
/* 共享列表样式：必须在**非 scoped** 块里导入。
 * 放在 <style scoped> 里时 Vue 会给每条规则追加 [data-v-xxx]，
 * 而 .t-table__content / .t-table__body 是 TDesign 子组件内部元素、
 * 不带当前组件的 data-v 属性 → 选择器永不匹配（横向滚动条收不住）。
 * 发票页一直是对的（它就在非 scoped 块里），其余三页对齐它。 */
@import './styles/business-list-shared.css';
</style>
