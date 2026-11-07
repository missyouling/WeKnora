<!--
  人事管理（第 45 轮）：花名册 / 社保管理 / 公积金管理 单页三卡片。

  骨架 100% 照抄 FleetRecordList.vue（日常事务中台）：
    ① 顶部 DashboardKpiGroup（恰好 3 张 type 卡，无任何汇总 / 历史卡）；
    ② 中部 BusinessFilterBar（#batch-actions 浮动条 + #right-extra 业务按钮）；
    ③ 下部 doc-list-scroll > doc-list-view > t-table（勾选感知合计行）+ 各类抽屉。

  表头首帧一次成型：切换 tab / 二级视图时先把 headerReady 置 false 撤掉表格，
  await Promise.all([字段配置, 列表数据]) 后一次性置 true —— 绝不先渲染
  「只有复选框」的半套列（第 39 轮门禁）。
-->
<template>
  <div class="biz-page hr-page" :data-hr-tab="activeTab">
    <!-- ① 顶部 KPI：3 张卡，包裹 div 带 data-hr-kpi-group 便于真机断言 -->
    <div class="overview-group" data-hr-kpi-group>
      <DashboardKpiGroup :loading="false" :skeleton="false" :cards="kpiCards" :active-type="activeTab"
        @card-click="onCardClick" />
    </div>

    <div class="biz-list-view">
      <!-- 公积金二级切换：缴存台账 / 月度账单（账单严格倒序） -->
      <div v-if="activeTab === 'hr-fund'" class="hr-subview-bar">
        <t-radio-group v-model="fundSubView" size="small" @change="onFundSubViewChange">
          <t-radio-button value="ledger">缴存台账</t-radio-button>
          <t-radio-button value="bills">月度账单</t-radio-button>
        </t-radio-group>
        <span v-if="fundSettings" class="hr-unit-config">
          单位：{{ fundSettings.unit_name || '—' }}
          · 账号 {{ fundSettings.unit_account || '—' }}
          · 默认比例 {{ fundSettings.default_ratio ?? 6 }}%
        </span>
      </div>

      <!-- ② 筛选工具栏 -->
      <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
        :columns="columnDefsForFilter" :visible-keys="visibleColKeys" :selected-count="selectedKeys.size"
        settings-text="设置" :primary-action="primaryAction" @columns-change="onColumnsChange"
        @reset-columns="resetColumns" @select-all-columns="selectAllColumns" @filter-change="applyFilter"
        @settings="settingsVisible = true" @primary="onPrimaryAction" @clear-selection="clearSelection">
        <template #batch-actions>
          <!-- display:contents 让按钮仍是 .batch-bar-actions 的直接 flex 项，不额外产生布局层 -->
          <div class="hr-batch-actions" data-hr-batch-bar>
            <t-button theme="default" variant="outline" size="small" @click="handleExportExcel">
              <template #icon><t-icon name="download" size="14px" /></template>
              导出 Excel
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="printBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印清单
            </t-button>
            <!-- 公积金：封存 / 启封 -->
            <template v-if="activeTab === 'hr-fund' && fundSubView === 'ledger'">
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleSeal(true)">
                <template #icon><t-icon name="lock-on" size="14px" /></template>
                封存
              </t-button>
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleSeal(false)">
                <template #icon><t-icon name="lock-off" size="14px" /></template>
                启封
              </t-button>
            </template>
            <!-- 花名册：办理离职 / 恢复在职（批量） -->
            <template v-if="activeTab === 'hr-roster'">
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleResign">
                <template #icon><t-icon name="logout" size="14px" /></template>
                办理离职
              </t-button>
              <t-button theme="default" variant="outline" size="small" :disabled="!selectedRowKeys.length"
                @click="handleRestore">
                <template #icon><t-icon name="rollback" size="14px" /></template>
                恢复在职
              </t-button>
            </template>
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedRowKeys.length} 条记录吗？删除后不可恢复。`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleBatchDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                批量删除
              </t-button>
            </t-popconfirm>
          </div>
        </template>

        <!-- 业务专属按钮：#right-extra（不进批量浮条，常驻工具栏右侧） -->
        <template #right-extra>
          <t-button v-if="activeTab === 'hr-roster'" theme="default" variant="outline" size="small"
            @click="triggerImport('roster')">
            <template #icon><t-icon name="upload" size="14px" /></template>
            导入花名册
          </t-button>
          <template v-if="activeTab === 'hr-social'">
            <t-button theme="default" variant="outline" size="small" @click="triggerImport('si')">
              <template #icon><t-icon name="upload" size="14px" /></template>
              导入五险
            </t-button>
            <t-button theme="default" variant="outline" size="small" @click="handleProcess">
              <template #icon><t-icon name="calculation" size="14px" /></template>
              执行汇总
            </t-button>
            <t-button theme="default" variant="outline" size="small" @click="handleExportExcel">
              <template #icon><t-icon name="download" size="14px" /></template>
              导出扣款明细
            </t-button>
          </template>
          <t-button v-if="activeTab === 'hr-fund'" theme="default" variant="outline" size="small"
            @click="handleGenerateBill">
            <template #icon><t-icon name="file-add" size="14px" /></template>
            生成月度账单
          </t-button>
        </template>
      </BusinessFilterBar>

      <!-- ③ 列表 -->
      <div class="doc-list-scroll" :data-hr-row-count="rows.length">
        <!-- 表头就绪前只渲染占位块：绝不先渲染半套列 -->
        <div v-if="!headerReady" class="doc-list-view">
          <div class="hr-empty-placeholder" />
        </div>
        <div v-else :class="listViewClass" :style="headerFixStyle">
          <t-table :data="rows" :columns="tableColumns" row-key="id" size="small"
            :hover="true" :loading="tableLoading" :max-height="'100%'"
            :foot-data="tableFootData" :rowspan-and-colspan-in-footer="footSpan"
            :selected-row-keys="selectedRowKeys" :scroll="tableScroll" sticky-header
            class="doc-table hr-table"
            :row-class-name="({ row }: any) => selectedKeys.has(hrRowKey(row)) ? 'is-selected' : ''"
            @row-click="({ row }: any) => openDetail(row)"
            @select-change="onTableSelectChange">
            <!-- 4 个系统状态列：**只读彩色 Tag**（具名插槽 #<colKey>）。
                 TDesign 1.19 的 cell 回调不注入 h，用 cell+h(Tag) 会渲染成空白；
                 同一个插槽还会被复用于表头，此时必须输出纯文本列标题，
                 否则表头会变成「—」或一枚彩色 Tag。 -->
            <template #在职状态="{ row }: any">
              <span v-if="isHeaderContext(row)">在职状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '在职状态'))" variant="light" size="small">
                {{ colValue(row, '在职状态') }}
              </t-tag>
            </template>
            <template #账期状态="{ row }: any">
              <span v-if="isHeaderContext(row)">账期状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '账期状态'))" variant="light" size="small">
                {{ colValue(row, '账期状态') }}
              </t-tag>
            </template>
            <template #参保状态="{ row }: any">
              <span v-if="isHeaderContext(row)">参保状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '参保状态'))" variant="light" size="small">
                {{ colValue(row, '参保状态') }}
              </t-tag>
            </template>
            <template #缴存状态="{ row }: any">
              <span v-if="isHeaderContext(row)">缴存状态</span>
              <t-tag v-else :theme="statusTheme(colValue(row, '缴存状态'))" variant="light" size="small">
                {{ colValue(row, '缴存状态') }}
              </t-tag>
            </template>
            <!-- 账期 / 账单月份：合法 YYYY-MM 的单元格挂 data-hr-month-cell 便于真机断言倒序 -->
            <template #账期="{ row }: any">
              <span v-if="isHeaderContext(row)">账期</span>
              <span v-else-if="isHrMonth(colValue(row, '账期'))" :data-hr-month-cell="colValue(row, '账期')">
                {{ colValue(row, '账期') }}
              </span>
              <span v-else>{{ colValue(row, '账期') }}</span>
            </template>
            <template #月份="{ row }: any">
              <span v-if="isHeaderContext(row)">月份</span>
              <span v-else-if="isHrMonth(colValue(row, '月份'))" :data-hr-month-cell="colValue(row, '月份')">
                {{ colValue(row, '月份') }}
              </span>
              <span v-else>{{ colValue(row, '月份') }}</span>
            </template>
            <template #empty>
              <BusinessEmptyState v-if="!tableLoading" compact :icon="emptyIcon"
                :description="emptyText" :hint="emptyHint" />
              <div v-else class="hr-empty-placeholder" />
            </template>
          </t-table>
        </div>
      </div>
    </div>

    <!-- 详情抽屉：深拷贝草稿 + 脏检查只标脏不自动保存，必须点「保存」才调 API -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="detailRow"
      :page-no="1" :form="editForm" :fields="drawerFields" :sub-fields="[]"
      :supports-sub-fields="false" :dirty="detailDirty" :saving="saving"
      storage-key="weknora-hr-detail-width" :show-preview="false" :block-title="detailTitle"
      @save="saveDetail" @delete="onDetailDelete" />

    <!-- 设置抽屉：纯手工模块，显式关掉「提取规则」Tab -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="''" :scope="hrScopeOf(activeTab)"
      :initial-category="initialCategory" :title="settingsTitle"
      storage-key="weknora-hr-settings-drawer-width" :show-rules="false" @saved="onSettingsSaved" />

    <!-- 社保账期：个人 / 单位扣款明细（点行打开） -->
    <t-drawer v-if="periodDetailVisible" :visible="true"
      :header="`账期 ${periodDetail?.year_month || ''} 扣款明细`" size="720px" :footer="false"
      @close="periodDetailVisible = false">
      <div class="hr-charge-group">
        <div class="hr-charge-title">个人扣款明细</div>
        <t-table :data="personalCharges" :columns="chargeColumns" row-key="id" size="small" bordered
          :empty="'暂无个人扣款明细'" />
      </div>
      <div class="hr-charge-group">
        <div class="hr-charge-title">单位扣款明细</div>
        <t-table :data="unitCharges" :columns="chargeColumns" row-key="id" size="small" bordered
          :empty="'暂无单位扣款明细'" />
      </div>
    </t-drawer>

    <!-- 打印预览（统一中台组件） -->
    <PrintPreviewDialog v-model:visible="printVisible" :title="printTitle" :url="printUrl" :data="printData"
      :download-name="`${printTitle}.pdf`" @close="closePrint" />

    <!-- 通用文件选择（花名册 / 五险 / 公积金导入共用） -->
    <input ref="fileInputRef" type="file" accept=".xlsx,.xls,.csv" class="hr-file-input" @change="onFileChange" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessEmptyState from './BusinessEmptyState.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import PrintPreviewDialog from './PrintPreviewDialog.vue'
import { listFleetCategories } from '@/api/fleet'
import { useTableFlatState, flatClassOf } from './useTableFlatState'
import { useTableHeaderFix } from './useTableHeaderFix'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import { buildFootRow, footSpanOf, summaryScopeRows, type SummaryColumn } from './listSummary'
import './styles/list-summary-footer.css'
import { generateCatalogPdf } from './useCatalogPdf'
import { grouped } from './formatNumber'
import {
  HR_TABS, buildHrKpiCards, hrScopeOf, hrColumnDefs, hrBillColumnDefs,
  hrSummaryColumns, hrBillSummaryColumns, sortMonthsDesc, isHrMonth,
  hrFieldSeeds, hrRowKey,
} from './hrCore'
import type { HrTab, HrColumn } from './hrCore'
import * as hrApi from './hrApi'
import type { FilterFieldSpec } from './businessFilter'

// ---------------------------------------------------------------------------
// 基础状态
// ---------------------------------------------------------------------------
const activeTab = ref<HrTab>('hr-roster')
const fundSubView = ref<'ledger' | 'bills'>('ledger')
/** 表头门禁：false 时不渲染 t-table，保证第 1 帧列数一次成型。 */
const headerReady = ref(false)
const loading = ref(false)
const rows = ref<any[]>([])
const selectedKeys = ref<Set<string>>(new Set())
const selectedRowKeys = computed(() => Array.from(selectedKeys.value))
const filterState = ref<Record<string, any>>({ keyword: '' })
const kpiCounts = ref<Record<string, number>>({})
const fundSettings = ref<any>(null)
const fieldConfig = ref<Record<string, any[]>>({})

const tabLabel = computed(() => HR_TABS.find((t) => t.key === activeTab.value)?.label || '人事管理')
const settingsTitle = computed(() => `${tabLabel.value}设置`)
const initialCategory = computed(() => tabLabel.value)
const detailTitle = computed(() => `${tabLabel.value}详情`)
const printTitle = computed(() => `${tabLabel.value}清单`)

const primaryAction = computed(() => {
  if (activeTab.value === 'hr-social') return { text: '新增账期', icon: 'add' }
  if (activeTab.value === 'hr-fund') return { text: '新增缴存记录', icon: 'add' }
  return { text: '新增员工', icon: 'add' }
})

const kpiCards = computed(() => buildHrKpiCards(kpiCounts.value))

// ---------------------------------------------------------------------------
// 列定义 / 列显隐
// ---------------------------------------------------------------------------
const currentColumns = computed<HrColumn[]>(() =>
  activeTab.value === 'hr-fund' && fundSubView.value === 'bills'
    ? hrBillColumnDefs()
    : hrColumnDefs(activeTab.value),
)
const businessColumns = computed(() => currentColumns.value.filter((c) => c.colKey !== 'row-select'))
const columnDefsForFilter = computed(() => businessColumns.value.map((c) => ({ key: c.colKey, label: c.title })))

const STORAGE_KEY = computed(() => `weknora-hr-${activeTab.value}-${fundSubView.value}-cols-v1`)
const visibleColKeys = ref<string[]>([])
const visibleCols = computed(() => businessColumns.value.filter((c) => visibleColKeys.value.includes(c.colKey)))

function persistColumns() {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
function resetColumns() {
  visibleColKeys.value = businessColumns.value.map((c) => c.colKey)
  persistColumns()
}
function selectAllColumns() { resetColumns() }
function onColumnsChange(keys: string[]) { visibleColKeys.value = keys; persistColumns() }
function initColumns() {
  const allKeys = businessColumns.value.map((c) => c.colKey)
  const saved = localStorage.getItem(STORAGE_KEY.value)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      const matched = allKeys.filter((k) => Array.isArray(arr) && arr.includes(k))
      if (matched.length) { visibleColKeys.value = matched; return }
    } catch { /* ignore */ }
  }
  resetColumns()
}
watch(STORAGE_KEY, () => { initColumns() })

const tableColumns = computed(() => {
  const cols: any[] = []
  const vis = currentColumns.value.filter(
    (c) => c.colKey === 'row-select' || visibleColKeys.value.includes(c.colKey),
  )
  vis.forEach((c, i) => {
    // 勾选列：不带 fixed（第 38 轮起全站取消冻结列）
    if (c.colKey === 'row-select') {
      cols.push({ colKey: 'row-select', type: 'multiple', width: 44 })
      return
    }
    const values = rows.value.slice(0, 30).map((r) => c.value?.(r))
    const w = Math.max(colWidthOf(c.width, c.title, values, c.dataType), Number((c as any).minWidth) || 0)
    const base: any = {
      colKey: c.colKey,
      title: c.title,
      ellipsis: ellipsisOf(c),
      // 状态 / 月份列走具名插槽；其余用 cell 返回字符串（TDesign 1.19 不注入 h）
      cell: c.slot ? undefined : (_h: any, params: any) => c.value?.(params?.row) ?? '',
      dataType: c.dataType,
    }
    if (i === vis.length - 1) base.minWidth = w
    else base.width = w
    const cls = cellClassOf(c)
    if (cls) base.className = cls
    cols.push(base)
  })
  return cols
})
const tableScroll = computed(() => tableScrollOf(visibleCols.value))

// ---------------------------------------------------------------------------
// 合计行（勾选感知；loading 时不渲染）
// ---------------------------------------------------------------------------
const summaryCols = computed<SummaryColumn[]>(() => {
  const cols = activeTab.value === 'hr-fund' && fundSubView.value === 'bills'
    ? hrBillSummaryColumns()
    : hrSummaryColumns(activeTab.value)
  return cols.filter((c) => visibleColKeys.value.includes(c.key))
})
const footSpan = computed(() => footSpanOf(summaryCols.value))
const summaryScope = computed(() => summaryScopeRows(rows.value, selectedKeys.value, hrRowKey))
const footData = computed(() => [buildFootRow(summaryCols.value, summaryScope.value.rows, {
  selectKey: 'row-select',
  selectedCount: summaryScope.value.selectedCount,
})])
const tableLoading = computed(() => loading.value)
const tableFootData = computed(() => (tableLoading.value ? [] : footData.value))

const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
])

// ---------------------------------------------------------------------------
// 展示辅助
// ---------------------------------------------------------------------------
/**
 * TDesign 1.19 会把 `#<colKey>` 具名插槽**复用于表头渲染**，此时 row 为空，
 * 若照常走取值函数，表头就会显示成占位符「—」（真机截图实测）。
 * 表头上下文一律返回列标题，数据行才取真实值。
 */
function isHeaderContext(row: any): boolean {
  return !row || (typeof row === 'object' && Object.keys(row).length === 0)
}
function colValue(row: any, colKey: string): string {
  const c = currentColumns.value.find((x) => x.colKey === colKey)
  if (!c) return '—'
  if (isHeaderContext(row)) return c.title
  return c.value?.(row) ?? '—'
}
function statusTheme(v: string): 'success' | 'warning' | 'danger' | 'default' {
  const s = String(v ?? '')
  if (/在职|正常|已缴|生效|已汇总|已完成|正常缴存|active/.test(s)) return 'success'
  if (/离职|封存|停缴|暂停|过期|异常|作废/.test(s)) return 'danger'
  if (/待|进行|处理中|未|pending/.test(s)) return 'warning'
  return 'default'
}

const emptyIcon = 'search-error'
const emptyText = computed(() => '暂无数据')
const emptyHint = computed(() =>
  activeTab.value === 'hr-social'
    ? '点击「新增账期」创建账期后，可导入五险数据并执行汇总'
    : '点击右上角「设置」可调整字段，或先导入数据',
)

// ---------------------------------------------------------------------------
// 数据加载 + headerReady 门禁
// ---------------------------------------------------------------------------
function extractItems(res: any): any[] {
  const d = res?.data
  if (Array.isArray(d)) return d
  if (Array.isArray(d?.items)) return d.items
  if (Array.isArray(d?.list)) return d.list
  if (Array.isArray(res)) return res
  return []
}

async function loadFieldConfig(tab: HrTab) {
  try {
    const res: any = await listFleetCategories({ scope: hrScopeOf(tab) })
    fieldConfig.value = { ...fieldConfig.value, [tab]: Array.isArray(res?.data) ? res.data : [] }
  } catch {
    // 分类接口未就绪时用同步种子兜底，绝不让门禁卡住表头
    fieldConfig.value = { ...fieldConfig.value, [tab]: hrFieldSeeds(tab) as any }
  }
}

async function loadList(tab: HrTab) {
  loading.value = true
  try {
    let items: any[] = []
    if (tab === 'hr-roster') {
      const res: any = await hrApi.listRosterEmployees({
        q: String(filterState.value?.keyword || '').trim() || undefined,
      })
      items = extractItems(res)
    } else if (tab === 'hr-social') {
      // 后端 ORDER BY year_month DESC + 前端二次稳定倒序兜底
      items = sortMonthsDesc(extractItems(await hrApi.listSocialPeriods()), 'year_month')
    } else if (fundSubView.value === 'bills') {
      items = sortMonthsDesc(extractItems(await hrApi.listFundBills()), 'month_label')
    } else {
      items = extractItems(await hrApi.listFundRecords())
    }
    // TDesign 的 row-key="id" 是列名，要求每行都有 id 字段。后端三个模块主键都是
    // string id；万一某视图缺 id，用 hrRowKey(row) 补一个，保证 TDesign 的选中键、
    // selectedKeys 与合计行的 hrRowKey 三者口径统一（都等于该行 id）。
    rows.value = items.map((r: any) => ({ ...(r || {}), id: r?.id ?? hrRowKey(r) }))
    kpiCounts.value = { ...kpiCounts.value, [tab]: items.length }
  } catch (e: any) {
    rows.value = []
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadFundSettings() {
  try {
    const res: any = await hrApi.getFundSettings()
    fundSettings.value = res?.data || res || null
  } catch { fundSettings.value = null }
}

async function reloadStats() {
  try {
    const res: any = await hrApi.getRosterStats()
    const d = res?.data || {}
    kpiCounts.value = {
      'hr-roster': Number(d.total ?? d.roster ?? d.employees ?? kpiCounts.value['hr-roster'] ?? 0) || 0,
      'hr-social': Number(d.social ?? d.periods ?? kpiCounts.value['hr-social'] ?? 0) || 0,
      'hr-fund': Number(d.fund ?? d.records ?? kpiCounts.value['hr-fund'] ?? 0) || 0,
    }
  } catch { /* 统计失败不阻塞列表 */ }
}

/**
 * 切换一级 Tab：**先撤表头 → 并行等字段配置与列表 → 一次性置 true**。
 * 这是「表头列数第 1 帧一次成型」的硬保证。
 */
async function switchTab(tab: HrTab) {
  headerReady.value = false
  activeTab.value = tab
  fundSubView.value = 'ledger'
  selectedKeys.value = new Set()
  filterState.value = { keyword: '' }
  rows.value = []
  initColumns()
  if (tab === 'hr-fund') void loadFundSettings()
  await Promise.all([loadFieldConfig(tab), loadList(tab)])
  headerReady.value = true
}

function onCardClick(card: any) {
  if (card.action !== 'type' || !card.typeValue) return
  if (activeTab.value === card.typeValue) return
  void switchTab(card.typeValue as HrTab)
}

/** 公积金二级切换同样走门禁，避免列集换了一半先渲染一帧。 */
async function onFundSubViewChange() {
  headerReady.value = false
  selectedKeys.value = new Set()
  rows.value = []
  initColumns()
  await Promise.all([loadFieldConfig(activeTab.value), loadList(activeTab.value)])
  headerReady.value = true
}

let filterTimer: ReturnType<typeof setTimeout> | null = null
let filterWatchReady = false
watch(filterState, () => {
  if (!filterWatchReady) return
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => { filterTimer = null; void loadList(activeTab.value) }, 300)
}, { deep: true })
function applyFilter() { void loadList(activeTab.value) }

function onTableSelectChange(val: any) {
  const arr = Array.isArray(val) ? val : []
  selectedKeys.value = new Set(arr.map(String))
}
function clearSelection() { selectedKeys.value = new Set() }
const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(hrRowKey(r))))

const { isTableFlat } = useTableFlatState({ loading, rows })
const listViewClass = computed(() => flatClassOf(isTableFlat.value))
const { headerFixStyle } = useTableHeaderFix()

// ---------------------------------------------------------------------------
// 详情抽屉（深拷贝草稿 + 手存）
// ---------------------------------------------------------------------------
const detailVisible = ref(false)
const detailRow = ref<any>(null)
const detailDirty = ref(false)
const saving = ref(false)
const editForm = ref<any>({ data: {}, remarkParts: {}, items: [] })
const editBaseline = ref('')
const drawerFields = computed(() => businessColumns.value.map((c) => ({
  name: c.colKey, label: c.title, dataType: c.dataType || 'text',
})))

function openDetail(row: any) {
  // 社保账期点行 → 打开个人 / 单位扣款明细抽屉（不进通用编辑抽屉）
  if (activeTab.value === 'hr-social') { void openPeriodDetail(row); return }
  detailRow.value = row
  const data: Record<string, any> = {}
  for (const c of businessColumns.value) {
    const v = c.value?.(row) ?? ''
    data[c.colKey] = v === '—' ? '' : v
  }
  editForm.value = { data, remarkParts: {}, items: [] }
  editBaseline.value = JSON.stringify(editForm.value)
  detailDirty.value = false
  detailVisible.value = true
}

// 脏检查只标脏，**不自动保存**（第 19 轮）
watch(editForm, () => {
  detailDirty.value = JSON.stringify(editForm.value) !== editBaseline.value
}, { deep: true })

async function saveDetail() {
  const row = detailRow.value
  saving.value = true
  try {
    const payload: Record<string, any> = { ...(row || {}), ...(editForm.value.data || {}) }
    const id = row?.id
    if (activeTab.value === 'hr-roster') {
      if (id) await hrApi.updateRosterEmployee(id, payload)
      else await hrApi.createRosterEmployee(payload)
    } else if (fundSubView.value === 'bills') {
      MessagePlugin.info('月度账单由系统生成，不支持手工编辑')
    } else {
      if (id) await hrApi.updateFundRecord(id, payload)
      else await hrApi.createFundRecord(payload)
    }
    MessagePlugin.success('已保存')
    detailVisible.value = false
    await loadList(activeTab.value)
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
    if (activeTab.value === 'hr-roster') await hrApi.removeRosterEmployee(row.id)
    else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
    else await hrApi.removeFundRecord(row.id)
    MessagePlugin.success('已删除')
    detailVisible.value = false
    await loadList(activeTab.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// 社保账期：个人 / 单位扣款明细
const periodDetailVisible = ref(false)
const periodDetail = ref<any>(null)
const personalCharges = ref<any[]>([])
const unitCharges = ref<any[]>([])
function chargeVal(row: any, keys: string[]): string {
  for (const k of keys) {
    const v = row?.[k]
    if (v !== undefined && v !== null && v !== '') return String(v)
  }
  return '—'
}
// 列键对齐后端 HRSICharge：name / id_number / department / base / subtotal
const chargeColumns = [
  { colKey: 'name', title: '姓名', cell: (_h: any, { row }: any) => chargeVal(row, ['name', '姓名', 'employee_name']) },
  { colKey: 'id_number', title: '证件号', cell: (_h: any, { row }: any) => chargeVal(row, ['id_number', 'identity_number', '证件号', 'person_code']) },
  { colKey: 'department', title: '部门', cell: (_h: any, { row }: any) => chargeVal(row, ['department', '部门']) },
  {
    colKey: 'base', title: '缴费基数',
    cell: (_h: any, { row }: any) => grouped(chargeVal(row, ['base', '缴费基数', 'base_amount', 'personal_base']), { fixed: true, fallback: '—', blankWhenEmpty: true }),
  },
  {
    colKey: 'subtotal', title: '小计',
    cell: (_h: any, { row }: any) => grouped(chargeVal(row, ['subtotal', 'amount', '小计', '金额']), { fixed: true, fallback: '—', blankWhenEmpty: true }),
  },
]
async function openPeriodDetail(row: any) {
  periodDetail.value = row
  personalCharges.value = []
  unitCharges.value = []
  periodDetailVisible.value = true
  if (!row?.id) return
  try {
    const [p, u] = await Promise.all([
      hrApi.getSocialCharges(row.id, 'personal'),
      hrApi.getSocialCharges(row.id, 'unit'),
    ])
    personalCharges.value = extractItems(p)
    unitCharges.value = extractItems(u)
  } catch { /* 明细失败保留空表 */ }
}

// ---------------------------------------------------------------------------
// 设置 / 新增 / 打印
// ---------------------------------------------------------------------------
const settingsVisible = ref(false)
function onSettingsSaved() { void loadFieldConfig(activeTab.value) }

function onPrimaryAction() {
  if (activeTab.value === 'hr-social') { void handleAddPeriod(); return }
  openDetail(null)
}
async function handleAddPeriod() {
  const d = new Date()
  const year_month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    await hrApi.createSocialPeriod({ year_month })
    MessagePlugin.success('已新增账期')
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '新增失败') }
}

const printVisible = ref(false)
const printData = ref<Uint8Array | null>(null)
const printUrl = ref('')
const printBusy = ref(false)
async function handlePrint() {
  const list = selectedRows.value.length ? selectedRows.value : rows.value
  if (!list.length) { MessagePlugin.warning('当前无数据可打印'); return }
  printBusy.value = true
  try {
    const cols = visibleCols.value.map((c) => ({ key: c.colKey, label: c.title, value: (r: any) => c.value?.(r) ?? '' }))
    printData.value = await generateCatalogPdf({ title: printTitle.value, columns: cols, rows: list })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([printData.value as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '清单生成失败')
  } finally {
    printBusy.value = false
  }
}
function closePrint() {
  printVisible.value = false
  printData.value = null
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---------------------------------------------------------------------------
// 批量操作（导出 / 封存启封 / 离职恢复 / 删除）
// ---------------------------------------------------------------------------
function downloadBlob(blob: any, name: string) {
  const b = blob instanceof Blob
    ? blob
    : new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  const url = URL.createObjectURL(b)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}
async function handleExportExcel() {
  try {
    let blob: any
    if (activeTab.value === 'hr-roster') {
      blob = await hrApi.exportEmployees(selectedRowKeys.value)
    } else if (activeTab.value === 'hr-social') {
      const id = selectedRowKeys.value[0] || rows.value[0]?.id
      if (!id) { MessagePlugin.warning('请先选择账期'); return }
      blob = await hrApi.exportSocialCharges(id)
    } else {
      blob = await hrApi.exportFund(selectedRowKeys.value)
    }
    downloadBlob(blob, `${printTitle.value}.xlsx`)
    MessagePlugin.success('已导出')
  } catch (e: any) { MessagePlugin.error(e?.message || '导出失败') }
}

async function handleSeal(seal: boolean) {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    for (const id of ids) {
      if (seal) await hrApi.sealFundRecord(id)
      else await hrApi.unsealFundRecord(id)
    }
    MessagePlugin.success(seal ? '已封存' : '已启封')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

async function handleResign() {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择员工'); return }
  try {
    await hrApi.resignRosterEmployees(ids)
    MessagePlugin.success('已办理离职')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}
async function handleRestore() {
  const ids = selectedRowKeys.value
  if (!ids.length) { MessagePlugin.warning('请先选择员工'); return }
  try {
    await hrApi.restoreRosterEmployees(ids)
    MessagePlugin.success('已恢复在职')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '操作失败') }
}

async function handleBatchDelete() {
  const list = selectedRows.value
  if (!list.length) { MessagePlugin.warning('请先选择记录'); return }
  try {
    if (activeTab.value === 'hr-roster') {
      await hrApi.batchDeleteRosterEmployees(list.map((r) => r.id).filter(Boolean))
    } else {
      for (const row of list) {
        if (!row?.id) continue
        if (activeTab.value === 'hr-social') await hrApi.removeSocialPeriod(row.id)
        else if (fundSubView.value === 'bills') await hrApi.removeFundBill(row.id)
        else await hrApi.removeFundRecord(row.id)
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---------------------------------------------------------------------------
// 社保业务按钮 / 公积金账单 / 文件导入
// ---------------------------------------------------------------------------
async function handleProcess() {
  const id = selectedRowKeys.value[0] || rows.value[0]?.id
  if (!id) { MessagePlugin.warning('请先选择账期'); return }
  try {
    await hrApi.processSocialPeriod(id)
    MessagePlugin.success('已执行汇总')
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '汇总失败') }
}

async function handleGenerateBill() {
  const d = new Date()
  const month_label = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
  try {
    await hrApi.createFundBill({ month_label })
    MessagePlugin.success('已生成月度账单')
    fundSubView.value = 'bills'
    await onFundSubViewChange()
  } catch (e: any) { MessagePlugin.error(e?.message || '生成失败') }
}

const fileInputRef = ref<HTMLInputElement | null>(null)
let importKind: 'roster' | 'si' | 'fund' = 'roster'
function triggerImport(kind: 'roster' | 'si' | 'fund') {
  if (kind === 'si' && !rows.value.length) { MessagePlugin.warning('请先新增账期'); return }
  importKind = kind
  fileInputRef.value?.click()
}
/**
 * 五险文件按文件名推断 scheme/part（源项目 `parseAdjustmentFileName` 同款口径）。
 * 例：「2026-10_个人_养老.xlsx」「单位工伤.xlsx」。大额医疗必须先于医疗判定，
 * 否则「大额医疗」会被吸到 medical。
 */
function siMetaOf(fileName: string): { scheme: string; part: string } | null {
  const s = fileName.replace(/\.(xlsx|xls|csv)$/i, '')
  const part = /单位/.test(s) ? 'unit' : /个人/.test(s) ? 'personal' : ''
  let scheme = ''
  if (/大病|大额/.test(s)) scheme = 'serious_illness'
  else if (/养老/.test(s)) scheme = 'pension'
  else if (/医疗/.test(s)) scheme = 'medical'
  else if (/失业/.test(s)) scheme = 'unemployment'
  else if (/工伤/.test(s)) scheme = 'injury'
  return scheme && part ? { scheme, part } : null
}

async function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  try {
    if (importKind === 'roster') {
      fd.append('mode', 'merge')
      fd.append('default_status', 'active')
      await hrApi.importEmployees(fd)
    } else if (importKind === 'si') {
      const meta = siMetaOf(file.name)
      if (!meta) {
        MessagePlugin.warning('文件名需含「个人/单位」与「养老/医疗/大额/失业/工伤」，如「个人_养老.xlsx」')
        return
      }
      fd.append('scheme', meta.scheme)
      fd.append('part', meta.part)
      fd.append('file_type', 'normal')
      const id = selectedRowKeys.value[0] || rows.value[0]?.id
      await hrApi.importSi(id, fd)
    } else await hrApi.importFund(fd)
    MessagePlugin.success('导入成功')
    await loadList(activeTab.value)
  } catch (e: any) { MessagePlugin.error(e?.message || '导入失败') }
}

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------
onMounted(async () => {
  initColumns()
  await switchTab('hr-roster')
  await reloadStats()
  filterWatchReady = true
})
</script>

<style lang="less" scoped>
/* 中台共享列表样式统一在文末的**非 scoped** 块 @import：
   scoped 会给每条规则加 [data-v-xxx]，而 TDesign 内部渲染的
   .t-table__content / tbody / tfoot 不带该属性，选择器永远命中不到。
   这里只放页面自用样式。 */
.hr-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 0;
  flex: 1;
}
.overview-group { flex: 0 0 auto; }
.biz-list-view { flex: 1; min-height: 0; display: flex; flex-direction: column; gap: 8px; }

.hr-subview-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 2px 0;
  flex: 0 0 auto;
}
.hr-unit-config {
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
/* 批量按钮外层保留可见盒（真机 data-hr-batch-bar 断言可读性），
   内部按钮随浮条换行；不设固定宽度，避免撑破浮动条。 */
.hr-batch-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.hr-empty-placeholder { flex: 1; min-height: 0; }
.hr-file-input { display: none; }
.hr-charge-group { margin-bottom: 20px; }
.hr-charge-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 8px;
}
</style>

<style>
/* 共享列表样式必须在**非 scoped** 块再引入一次：TDesign 内部的
   .t-table__content / .t-table__body / tfoot 等元素拿不到当前组件的 data-v，
   scoped 版本的选择器命中不到它们（发票 / 车队页同款做法）。 */
@import './styles/business-list-shared.css';
</style>
