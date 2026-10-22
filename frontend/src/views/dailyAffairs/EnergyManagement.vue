<template>
  <div class="energy-management-container biz-page">
    <!-- 顶部：标题 + 副标题 -->
    <div class="header">
      <div class="header-title">
        <h2>能耗管理</h2>
        <p class="header-subtitle">电费 / 水费 / 燃气费账单归档，按账期台账，用量可反算校验</p>
      </div>
    </div>

    <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
      @change="onFileInputChange" />

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化能耗管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-能耗」知识库">
        <template #image><t-icon name="thunder" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建能耗知识库</t-button>
    </div>

    <!-- 主界面 -->
    <div v-else class="energy-main biz-main">
      <!-- KPI 概览卡：已收录能耗单 → 各能耗类型卡 → 历史记录
           :loading="false" 是硬要求：骨架屏闪烁会让「首卡无阴影加载」验收失败 -->
      <DashboardKpiGroup :loading="false" :cards="overviewCards" :active-type="filterUtilityType"
        @card-click="onOverviewCardClick" />

      <div class="biz-list-view">
        <!-- 筛选工具栏：中台 BusinessFilterBar。
             能耗专属：账期筛选是月粒度（month 类型），不是日粒度 date-range。 -->
        <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
          :columns="effectiveColumns" :visible-keys="visibleColKeys" :selected-count="selectedRowKeys.length"
          settings-text="设置" :primary-action="{ text: '上传能耗单', icon: 'upload' }" hide-batch-bar
          @columns-change="onVisibleColKeysChange" @reset-columns="resetColumns"
          @select-all-columns="selectAllColumns" @filter-change="applyFilter"
          @settings="settingsVisible = true" @primary="uploadVisible = true"
          @clear-selection="clearSelection" />

        <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

        <!-- 能耗台账列表 -->
        <div class="doc-list-scroll">
          <div class="doc-list-view">
            <t-table
              :data="filteredRows"
              :columns="tableColumns"
              row-key="rowKey"
              size="small"
              :hover="true"
              :loading="listLoading"
              max-height="100%"
              sticky-header
              :scroll="tableScroll"
              class="doc-table"
              :selected-row-keys="selectedRowKeys"
              select-on-change
              :row-class-name="({ row }: any) => selectedRowKeys.includes(row.rowKey) ? 'is-selected' : ''"
              @row-click="({ row }: any) => openDetail(row)"
              @select-change="(val: string[]) => onSelectChange(val)"
            >
              <template #utility_no="{ row }: any">
                <span class="row-mono" :title="row.utility_no">{{ row.utility_no || '--' }}</span>
              </template>
              <template #billing_period="{ row }: any">
                <span class="row-mono">{{ row.billing_period || '--' }}</span>
              </template>
              <template #utility_type="{ row }: any">
                <span class="row-text" :title="row.utility_type">{{ row.utility_type || '--' }}</span>
              </template>
              <template #previous_reading="{ row }: any">
                <span class="row-mono">{{ formatNumber(row.previous_reading) }}</span>
              </template>
              <template #current_reading="{ row }: any">
                <span class="row-mono">{{ formatNumber(row.current_reading) }}</span>
              </template>
              <template #multiplier="{ row }: any">
                <span class="row-mono">{{ formatNumber(row.multiplier) }}</span>
              </template>
              <template #usage_amount="{ row }: any">
                <span class="row-mono row-amount" :title="usageTitle(row)">{{ formatNumber(row.usage_amount) }}</span>
                <t-tag v-if="row.usage_patched" size="small" theme="warning" variant="light-outline"
                  class="row-pushed-tag">推算</t-tag>
              </template>
              <template #unit_price="{ row }: any">
                <span class="row-mono row-rate">{{ formatNumber(row.unit_price) }}</span>
              </template>
              <template #total_amount="{ row }: any">
                <span class="row-mono row-amount">{{ formatNumber(row.total_amount) }}</span>
              </template>
              <template #extract_status="{ row }: any">
                <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
                  variant="light-outline" class="row-status-tag">
                  <template v-if="statusOf(row).icon" #icon>
                    <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
                  </template>
                  {{ statusOf(row).label }}
                </t-tag>
                <span v-else class="row-muted">--</span>
              </template>
              <template #tags="{ row }: any">
                <t-tooltip v-if="rowTags(row).length" :content="rowTags(row).map((t: any) => t.name).join('、')" placement="top">
                  <div class="row-tag-chips is-clickable" @click.stop="openTagEdit(row)">
                    <t-tag v-if="rowTags(row).length" size="small" variant="light-outline" class="row-tag">
                      {{ rowTags(row)[0].name }}
                    </t-tag>
                    <span v-if="rowTags(row).length > 1" class="row-tag-more">+{{ rowTags(row).length - 1 }}</span>
                  </div>
                </t-tooltip>
                <span v-else class="row-tag-chips is-clickable" @click.stop="openTagEdit(row)">
                  <span class="row-tag-add">+ 标签</span>
                </span>
              </template>
              <template #remark="{ row }: any">
                <span class="row-text" :title="row.remark">{{ row.remark || '--' }}</span>
              </template>
              <template #fileName="{ row }: any">
                <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
              </template>
            </t-table>

            <div v-if="!listLoading && filteredRows.length === 0" class="doc-list-empty">
              <t-empty description="暂无能耗记录" />
            </div>
          </div>
        </div>

        <!-- 底栏：分页 -->
        <div class="doc-list-footer">
          <span class="footer-count">共 {{ totalRecords }} 份</span>
          <t-pagination
            v-model="currentPage"
            :total="totalRecords"
            :page-size="pageSize"
            :show-jumper="true"
            size="small"
            @change="onPageChange"
          />
        </div>
      </div>
    </div>

    <!-- 创建知识库向导 -->
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-能耗"
      kb-desc="能耗管理固定使用专用知识库，名称不可修改"
      description-placeholder="用于存放并解析电费/水费/燃气费账单，自动提取账期与读数"
      default-description="用于存放并解析电费/水费/燃气费账单，自动提取账期与读数"
      model-tip="提取模型将复用下方「对话模型」，用于解析能耗字段。若列表为空，请先在系统设置中添加模型。"
      @created="onKbCreated" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" scope="energy"
      kb-name="日常事务-能耗" title="能耗设置" @saved="() => reloadColumns(true)" />

    <!-- 上传弹窗 -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="energy" title="上传能耗单"
      :type-options="uploadTypeOptions" @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 历史记录 -->
    <FleetUploadHistoryDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" scope="energy"
      @changed="onUploadHistoryChanged" />

    <!-- 详情抽屉：中台 BusinessSplitDetailDrawer（质量面板 → 动态字段 → 底部折叠源文件预览） -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="currentRow"
      :page-no="detailPageNo" :form="editFormWrapper" :fields="editFieldDefs" :sub-fields="energySubFields"
      :supports-sub-fields="true" item-label="表位明细"
      :block-title="editForm.utility_type || '能耗信息'"
      :quality="detailQuality" :label-of="qualityLabel" :dirty="detailDirty" :saving="saving"
      :storage-key="'weknora-energy-drawer-width'" @save="saveEnergyDetail" @delete="onDetailDelete"
      @re-extract="onDetailReExtract">
      <!-- 能耗类型下拉 -->
      <template #field-utility_type>
        <t-select v-model="editForm.utility_type" :options="utilityTypeOptions" clearable filterable allow-create
          placeholder="选择类型" />
      </template>
      <!-- 读数类字段：数字输入框（读数可能带小数，用 number 控件保留精度） -->
      <template #field-previous_reading>
        <t-input-number v-model="editForm.previous_reading" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="上期读数" />
      </template>
      <template #field-current_reading>
        <t-input-number v-model="editForm.current_reading" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="本期读数" />
      </template>
      <template #field-multiplier>
        <t-input-number v-model="editForm.multiplier" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="倍率" />
      </template>
      <template #field-usage_amount>
        <t-input-number v-model="editForm.usage_amount" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="实际用量" />
      </template>
      <template #field-unit_price>
        <t-input-number v-model="editForm.unit_price" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="单价" />
      </template>
      <template #field-total_amount>
        <t-input-number v-model="editForm.total_amount" class="energy-num-input" :decimal-places="4"
          :controls="false" placeholder="总金额" />
      </template>
      <template #field-extract_status>
        <t-tag v-if="statusOf(currentRow || {}).label !== '--'" size="small" :theme="statusOf(currentRow || {}).theme"
          variant="light-outline" class="row-status-tag">{{ statusOf(currentRow || {}).label }}</t-tag>
        <span v-else class="row-muted">--</span>
      </template>
      <template #field-tags>
        <div v-if="rowTags(currentRow || {}).length" class="detail-tags-readonly">
          <t-tag v-for="t in rowTags(currentRow || {})" :key="t.id || t.name" size="small" variant="light-outline"
            class="row-tag">{{ t.name }}</t-tag>
        </div>
        <span v-else class="row-muted">—</span>
      </template>

      <!-- 能耗告警面板：用量不平 / 环比暴增（阶段三业务优化） -->
      <template #extra>
        <div v-if="energyIssues.length" class="energy-alert-panel">
          <div class="energy-alert-head">
            <t-icon name="warning" size="16px" />
            <span class="energy-alert-title">用量校验提示</span>
          </div>
          <div v-for="(iss, i) in energyIssues" :key="i" class="energy-alert-item">
            <t-icon name="info-circle" size="14px" />
            <span>{{ iss.message }}</span>
          </div>
        </div>
        <div v-if="usageSurgeWarning" class="energy-alert-panel">
          <div class="energy-alert-head">
            <t-icon name="warning" size="16px" />
            <span class="energy-alert-title">用量波动提示</span>
          </div>
          <div class="energy-alert-item">
            <t-icon name="info-circle" size="14px" />
            <span>{{ usageSurgeWarning }}</span>
          </div>
        </div>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 标签编辑 -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理 -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />
  </div>
</template>

<!-- 四模块共享列表样式 -->
<style>
@import './styles/business-list-shared.css';
</style>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

import {
  listKnowledgeBases, getKnowledgeDetails, extractBusinessDocument,
  listEnergyRecords, listEnergyOverview, updateBusinessMetadata,
} from '@/api/knowledge-base'
import { listFleetCategories } from '@/api/fleet'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import type { KpiCard } from '@/components/business/DashboardKpiGroup.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import BusinessFilterBar from './BusinessFilterBar.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import FleetUploadDialog from './FleetUploadDialog.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import { appendListSystemColumns, buildColumnsFromSubs } from './sixWaySync'
import { buildDetailFieldDefs } from './businessDetailForm'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import { parseExtractQuality, EMPTY_QUALITY, type ExtractQuality } from './extractQuality'
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useBusinessPolling } from '@/composables/useBusinessPolling'
import { useDocStatus } from '@/composables/useDocStatus'
import { labelOf as scopeLabelOf } from '@/registry'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'

const KB_NAME = '日常事务-能耗'
const PAGE_SIZE = 20
// 用量环比暴增阈值：超过 30% 触发黄色告警
const USAGE_SURGE_THRESHOLD = 0.3

// 能耗标准列定义：顺序与后端 energyCatalog().Standard 一致（11 标准 + 9 扩展 + 系统列）
const DEFAULT_ENERGY_COLUMNS: ColumnDef[] = [
  { key: 'utility_no', label: '单据编号/户号', default: true, w: '1.3fr' },
  { key: 'billing_period', label: '账期', default: true, w: '0.9fr' },
  { key: 'utility_type', label: '能耗类型', default: true, w: '1fr' },
  { key: 'previous_reading', label: '上期读数', default: true, w: '1.1fr' },
  { key: 'current_reading', label: '本期读数', default: true, w: '1.1fr' },
  { key: 'multiplier', label: '倍率', default: true, w: '0.8fr' },
  { key: 'usage_amount', label: '实际用量', default: true, w: '1.2fr' },
  { key: 'unit_price', label: '单价', default: true, w: '1fr' },
  { key: 'total_amount', label: '总金额', default: true, w: '1.2fr' },
  { key: 'extract_status', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.1fr' },
  // 扩展字段（默认不显示）
  { key: 'account_name', label: '户名', default: false, w: '1.4fr' },
  { key: 'meter_no', label: '表号', default: false, w: '1.2fr' },
  { key: 'address', label: '用能地址', default: false, w: '1.6fr' },
  { key: 'period_start', label: '账期起', default: false, w: '1.1fr' },
  { key: 'period_end', label: '账期止', default: false, w: '1.1fr' },
  { key: 'read_date', label: '抄表日期', default: false, w: '1.1fr' },
  { key: 'usage_category', label: '用能类别', default: false, w: '1.1fr' },
  { key: 'voltage_level', label: '电压等级', default: false, w: '1fr' },
  { key: 'remark', label: '备注', default: false, w: '1.6fr' },
  // 系统列（置尾）
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]

const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns,
  selectedRowKeys, onSelectChange, clearSelection,
} = useBusinessList({
  storageKey: 'weknora-energy-list-columns',
  defaultColumns: DEFAULT_ENERGY_COLUMNS,
})

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const settingsVisible = ref(false)
const uploadVisible = ref(false)
const historyVisible = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)

interface EnergyRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  extractStatus: string
  utility_no?: string
  billing_period?: string
  utility_type?: string
  previous_reading?: number | null
  current_reading?: number | null
  multiplier?: number | null
  usage_amount?: number | null
  unit_price?: number | null
  total_amount?: number | null
  usage_patched?: boolean
  issues?: any[]
  items?: any[]
  tags?: any[]
}

// ---- 列表 ----
const energyRows = shallowRef<EnergyRow[]>([])
const listLoading = ref(false)
const totalRecords = ref(0)
const currentPage = ref(1)
const pageSize = PAGE_SIZE
const energyOverview = ref<any>({})
const energyCats = ref<any[]>([])
const tagList = ref<any[]>([])

const keyword = ref('')
const filterUtilityType = ref('')
const filterBillingPeriod = ref('')

const tableColumns = computed(() => {
  // TDesign 的列标识字段是 **colKey**（不是 key）：用 key 会导致列头渲染但
  // 单元格内容全空（真机踩过：tbody 有 12 个 td 但文本为空、行高 17px）。
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 46 }]
  const rows = filteredRows.value
  for (const c of visibleColDefs.value) {
    cols.push({
      colKey: c.key,
      title: c.label,
      // 列宽：字段配置 width>0 固定（clamp 60~400）；未配置按内容自适应
      width: colWidthOf(c.width, c.label, rows.map((r: any) => r?.[c.key]), (c as any).dataType),
      // 多行文本列关闭 ellipsis：TDesign 的 ellipsis 会包一层 nowrap + overflow hidden，
      // 换行样式会被直接吃掉，长文本仍旧截断
      ellipsis: ellipsisOf(c as any),
      className: cellClassOf(c as any),
      // dataType 必须下传：tableScrollOf 据此判断是否关闭固定行高
      dataType: (c as any).dataType,
    })
  }
  return cols
})

const tableScroll = computed(() => tableScrollOf(tableColumns.value as any))

// 账期筛选：能耗主时间轴是月（YYYY-MM），与合同的日粒度日期区间不同
const filterState = computed<FilterState>({
  get: () => ({
    keyword: keyword.value,
    billingPeriod: filterBillingPeriod.value,
  }),
  set: (v) => {
    keyword.value = String(v?.keyword ?? '')
    filterBillingPeriod.value = String(v?.billingPeriod ?? '')
  },
})

const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'billingPeriod', type: 'month', placeholder: '账期', icon: 'calendar' },
])

const onVisibleColKeysChange = (keys: string[]) => { visibleColKeys.value = keys }

const filteredRows = computed(() => energyRows.value)

const applyFilter = () => { loadRecords() }
const onPageChange = () => { loadRecords() }

// ---- 数值格式化 ----
const formatNumber = (v: any): string => {
  if (v === undefined || v === null || v === '') return '--'
  const n = Number(v)
  if (Number.isNaN(n)) return String(v)
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 4 })
}

// 用量标题：推算值与账单值的对照，便于人工核对
const usageTitle = (row: any): string => {
  if (row?.usage_patched) return '该用量由读数反算推算：实际用量 = (本期读数 − 上期读数) × 倍率'
  return ''
}

// ---- KPI 概览卡 ----
const overviewCards = computed<KpiCard[]>(() => {
  const st = energyOverview.value || {}
  const total = Number(st.total || 0)
  const cards: KpiCard[] = [
    {
      key: 'total', label: '已收录能耗单', icon: 'thunder', value: String(total), unit: '份',
      sub: `用量合计 ${formatNumber(st.sum_usage)}`, theme: 'brand', action: 'all',
    },
  ]
  // 分类卡：以启用分类为权威源动态生成
  for (const c of energyCats.value) {
    if (c.enabled === false) continue
    const name = String(c.name || '')
    if (!name) continue
    const stat = (st.by_category || []).find((x: any) => String(x.category) === name)
    cards.push({
      key: `cat:${name}`,
      label: name,
      icon: 'thunder',
      value: String(Number(stat?.count || 0)),
      unit: '份',
      sub: `金额 ${formatNumber(stat?.sum_total)}`,
      theme: 'neutral',
      action: name,
    })
  }
  cards.push({
    key: 'history', label: '历史记录', icon: 'time', value: String(Number(st.file_count || 0)), unit: '个',
    sub: `异常提示 ${Number(st.issue_count || 0)} 条`,
    theme: 'neutral',
    action: 'history',
  })
  return cards
})

const onOverviewCardClick = (card: any) => {
  if (card.action === 'all') {
    filterUtilityType.value = ''
    filterBillingPeriod.value = ''
  } else if (card.action === 'history') {
    historyVisible.value = true
    return
  } else {
    filterUtilityType.value = String(card.action || '')
  }
  currentPage.value = 1
  applyFilter()
}

// ---- 分类与列（六同步） ----
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    let cats: any[] = force ? [] : energyCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'energy' })
      cats = res?.data || []
      energyCats.value = cats
    }
    const curCat = cats.find((c: any) => c.name === filterUtilityType.value && c.enabled !== false)
    const baseCat = curCat || cats[0]
    if (baseCat?.subs?.length) {
      // 六同步：主列表表头 / 字段筛选器 / 抽屉 / 提取规则同源于 subs
      customColumns.value = appendListSystemColumns(
        buildColumnsFromSubs(baseCat.subs, {
          labelOf: (n: string) => scopeLabelOf('energy', n),
          defaultColumns: DEFAULT_ENERGY_COLUMNS as any,
        }),
        DEFAULT_ENERGY_COLUMNS as any,
        (n: string) => scopeLabelOf('energy', n),
      ) as any
      if (!lastColType.value) {
        const defaults = customColumns.value.filter((c: any) => c.default).map((c: any) => c.key)
        if (defaults.some((k: string) => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterUtilityType.value) {
        resetColumns()
      }
      lastColType.value = filterUtilityType.value
    }
  } catch { /* 后端未配置时回退内置默认 */ }
}

// ---- 数据加载 ----
const loadRecords = async () => {
  if (!kbId.value) return
  listLoading.value = true
  try {
    const res: any = await listEnergyRecords(kbId.value, {
      q: keyword.value || undefined,
      utility_type: filterUtilityType.value || undefined,
      billing_period: filterBillingPeriod.value || undefined,
      page: currentPage.value,
      page_size: pageSize,
    })
    const data = res?.data?.data || res?.data || []
    const total = res?.data?.total ?? res?.total ?? 0
    energyRows.value = (Array.isArray(data) ? data : []).map((r: any, i: number) => ({
      ...r,
      rowKey: `${r.knowledge_id || r.knowledgeId || 'row'}-${i}`,
      knowledgeId: r.knowledge_id || r.knowledgeId || '',
      fileName: r.file_name || r.fileName || '',
      extractStatus: r.extract_status || r.extractStatus || '',
      // 文件级解析状态：状态列徽标的判定输入（缺了会恒显示「待解析」）
      parseStatus: r.parse_status || r.parseStatus || '',
    }))
    totalRecords.value = Number(total) || 0
  } catch (e: any) {
    MessagePlugin.error(e?.message || '能耗记录加载失败')
  } finally {
    listLoading.value = false
  }
}

const loadOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listEnergyOverview(kbId.value)
    energyOverview.value = res?.data || {}
  } catch { /* 概览失败不阻塞列表 */ }
}

const loadCats = async () => {
  try {
    const res: any = await listFleetCategories({ scope: 'energy' })
    energyCats.value = res?.data || []
  } catch { /* ignore */ }
}

const utilityTypeOptions = computed(() =>
  energyCats.value.filter((c: any) => c.enabled !== false).map((c: any) => ({ value: c.name, label: c.name })))
const uploadTypeOptions = computed(() => utilityTypeOptions.value)

// ---- KB ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      await loadCats()
      await reloadColumns()
      await Promise.all([loadRecords(), loadOverview()])
      startPolling()
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '知识库查询失败')
  } finally {
    loading.value = false
  }
}

const onKbCreated = async (kb: any) => {
  kbId.value = kb?.id || ''
  if (kbId.value) {
    await loadCats()
    await reloadColumns(true)
    await Promise.all([loadRecords(), loadOverview()])
    startPolling()
  }
}

// ---- 轮询（统一 composable：start/stop/onTick） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'energy'),
  onPendingFiles: (files) => { pendingFiles.value = files },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是能耗单文件，已移至删除历史，可在删除历史中恢复`)
  },
  onTick: () => { loadRecords(); loadOverview() },
})

// 进行中文件的阶段进度（按耗时估算，与其它业务页同口径）
const fileStartAt = new Map<string, number>()
const liveProgress = computed(() => {
  const now = Date.now()
  return pendingFiles.value.map((f) => {
    const id = f.id
    if (!fileStartAt.has(id)) fileStartAt.set(id, now)
    const start = fileStartAt.get(id)!
    const elapsed = (now - start) / 1000
    const ps = f.parse_status || ''
    const es = f.custom_metadata?.extract_status || ''
    const name = f.file_name || f.title || id
    if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) {
      return { name, stage: 'extracting' as const, percent: Math.min(100, Math.round(30 + elapsed * 2)) }
    }
    return { name, stage: 'parsing' as const, percent: Math.min(92, Math.round(10 + elapsed * 4)) }
  })
})
const displayProgress = computed(() => liveProgress.value)

// ---- 状态 ----
// currentDetail：当前详情抽屉行的详情快照（状态徽标派生需要）
const currentDetail = ref<any>(null)
const { statusOf } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'energy', notLabel: '非能耗单',
})

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<EnergyRow | null>(null)
const detailPageNo = ref(0)
const saving = ref(false)
const editForm = ref<any>({})
// 中台抽屉的 form 契约是 `{ data, remarkParts, items }`：
// 直接把扁平 editForm 传进去会让抽屉读 `form.data.xxx` 报
// "Cannot read properties of undefined"（真机踩过）。这里做零成本包装：
// data 指向同一个 editForm 对象，抽屉的 v-model 写入直接落在 editForm 上，
// 保存链路一行都不用改。
const editFormWrapper = computed(() => ({
  data: editForm.value,
  remarkParts: {} as Record<string, string>,
  // 表位明细：多计量点读数并入 items（一文件一档案，不裂变成多条记录）
  items: Array.isArray(editForm.value?.items) ? editForm.value.items : [],
}))
const detailDirty = ref(false)

const energyIssues = computed<any[]>(() => (currentRow.value?.issues || []) as any[])

// 用量环比暴增告警：与上一账期（同户号）对比
const usageSurgeWarning = computed<string>(() => {
  const cur = currentRow.value
  if (!cur) return ''
  const curUsage = Number(cur.usage_amount)
  const curPeriod = String(cur.billing_period || '')
  if (!Number.isFinite(curUsage) || curUsage <= 0 || !curPeriod) return ''
  // 找同户号的上一账期记录
  const candidates = energyRows.value.filter((r) =>
    r.utility_no === cur.utility_no && r.billing_period && r.billing_period < curPeriod)
  if (!candidates.length) return ''
  const prev = candidates[candidates.length - 1]
  const prevUsage = Number(prev.usage_amount)
  if (!Number.isFinite(prevUsage) || prevUsage <= 0) return ''
  const ratio = (curUsage - prevUsage) / prevUsage
  if (ratio > USAGE_SURGE_THRESHOLD) {
    return `本期实际用量（${formatNumber(curUsage)}）较上一账期（${prev.billing_period}，${formatNumber(prevUsage)}）增长 ${(ratio * 100).toFixed(1)}%，请确认是否为生产扩大或抄表异常`
  }
  return ''
})

const qualityLabel = (f: string) => scopeLabelOf('energy', f)

const energySubFields = computed(() => [
  { key: 'meter_no', label: '表号', dataType: 'text' },
  { key: 'meter_type', label: '示数类型', dataType: 'text' },
  { key: 'previous_reading', label: '上期读数', dataType: 'number' },
  { key: 'current_reading', label: '本期读数', dataType: 'number' },
  { key: 'multiplier', label: '倍率', dataType: 'number' },
  { key: 'usage_amount', label: '实际用量', dataType: 'number' },
  { key: 'remarks', label: '备注', dataType: 'text' },
])

// 抽屉字段：六同步 —— 完全由 subs 派生（改名/改类型/删除即时生效）
const editFieldDefs = computed(() => {
  const cat = energyCats.value.find((c: any) => c.name === filterUtilityType.value && c.enabled !== false)
    || energyCats.value[0]
  if (!cat?.subs?.length) {
    return buildDetailFieldDefs([], {
      fieldKeyOf: (n: string) => scopeLabelOf('energy', n) ? String(n || '').trim() : '',
      labelOf: (n: string) => scopeLabelOf('energy', n),
      isRemarkPart: () => false,
    })
  }
  return buildDetailFieldDefs(cat.subs, {
    fieldKeyOf: (n: string) => String(n || '').trim(),
    labelOf: (n: string) => scopeLabelOf('energy', n),
    isRemarkPart: () => false,
  })
})

// 提取质量与溯源：详情接口的 custom_metadata（多条记录时按当前记录定位 index）。
// 详情未返回前为空质量 → 面板静默，不抢占视觉。
const detailQuality = computed<ExtractQuality>(() => {
  const meta = (currentDetail.value as any)?.custom_metadata
  if (!meta) return EMPTY_QUALITY
  const list: any[] = Array.isArray(meta?.records) ? meta.records : []
  let idx = -1
  if (list.length > 1) {
    idx = list.findIndex((r: any) =>
      !!currentRow.value?.billing_period && r?.billing_period === currentRow.value?.billing_period)
  }
  return parseExtractQuality(meta, idx >= 0 ? idx : undefined)
})

const detailTitle = computed(() => {
  const row = currentRow.value
  if (!row) return '能耗详情'
  return `${row.utility_type || '能耗单'} · ${row.billing_period || ''}`
})

const openDetail = async (row: EnergyRow) => {
  currentRow.value = row
  currentDetail.value = null
  detailPageNo.value = 0
  // 表单初值：数值字段保持 number 类型（读数要精确到小数位）
  editForm.value = {
    utility_no: row.utility_no || '',
    billing_period: row.billing_period || '',
    utility_type: row.utility_type || '',
    previous_reading: toNumOrNull(row.previous_reading),
    current_reading: toNumOrNull(row.current_reading),
    multiplier: toNumOrNull(row.multiplier),
    usage_amount: toNumOrNull(row.usage_amount),
    unit_price: toNumOrNull(row.unit_price),
    total_amount: toNumOrNull(row.total_amount),
    account_name: (row as any).account_name || '',
    meter_no: (row as any).meter_no || '',
    address: (row as any).address || '',
    period_start: (row as any).period_start || '',
    period_end: (row as any).period_end || '',
    read_date: (row as any).read_date || '',
    usage_category: (row as any).usage_category || '',
    voltage_level: (row as any).voltage_level || '',
    remark: (row as any).remark || '',
    // 表位明细：一文件一档案，多表位读数并入 items
    items: Array.isArray((row as any).items) ? [...(row as any).items] : [],
  }
  detailDirty.value = false
  detailVisible.value = true
  // 拉详情：质量面板与溯源信息来自 custom_metadata
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    if (detail && typeof detail === 'object') currentDetail.value = detail
  } catch { /* 详情拉取失败不阻塞编辑 */ }
}

function toNumOrNull(v: any): number | null {
  if (v === undefined || v === null || v === '') return null
  const n = Number(v)
  return Number.isNaN(n) ? null : n
}

const saveEnergyDetail = async () => {
  const row = currentRow.value
  if (!row) return
  saving.value = true
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const records = Array.isArray(meta.records) ? [...meta.records] : []
    if (!records.length) {
      MessagePlugin.warning('未找到可保存的能耗记录')
      return
    }
    const rec = records[0]
    const updated: any = {
      ...rec,
      utility_no: editForm.value.utility_no || '',
      billing_period: editForm.value.billing_period || '',
      utility_type: editForm.value.utility_type || '',
      previous_reading: editForm.value.previous_reading,
      current_reading: editForm.value.current_reading,
      multiplier: editForm.value.multiplier,
      usage_amount: editForm.value.usage_amount,
      unit_price: editForm.value.unit_price,
      total_amount: editForm.value.total_amount,
      account_name: editForm.value.account_name || '',
      meter_no: editForm.value.meter_no || '',
      address: editForm.value.address || '',
      period_start: editForm.value.period_start || '',
      period_end: editForm.value.period_end || '',
      read_date: editForm.value.read_date || '',
      usage_category: editForm.value.usage_category || '',
      voltage_level: editForm.value.voltage_level || '',
      remark: editForm.value.remark || '',
    }
    // 用户手工改过用量 → 取消「系统推算」标记（否则会误导为系统算出的值）
    if (Number(updated.usage_amount) !== Number(row.usage_amount)) {
      updated.usage_patched = false
    }
    records[0] = updated
    await updateBusinessMetadata(kbId.value, row.knowledgeId, { ...meta, kind: 'energy', records })
    MessagePlugin.success('能耗记录已保存')
    detailVisible.value = false
    await Promise.all([loadRecords(), loadOverview()])
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const onDetailDelete = async () => {
  detailVisible.value = false
  await loadRecords()
}

const onDetailReExtract = async () => {
  const row = currentRow.value
  if (!row) return
  try {
    await extractBusinessDocument(kbId.value, row.knowledgeId, 'energy')
    MessagePlugin.success('已提交重新提取')
    detailVisible.value = false
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新提取失败')
  }
}

// ---- 上传 ----
const onUploadProgress = () => { /* 轮询驱动状态条 */ }
const onUploadDone = async () => {
  await Promise.all([loadRecords(), loadOverview()])
}
const onUploadHistoryChanged = async () => {
  await Promise.all([loadRecords(), loadOverview()])
}
const onFileInputChange = (e: Event) => {
  const input = e.target as HTMLInputElement
  input.value = ''
}

// ---- 标签 ----
const tagDialogVisible = ref(false)
const tagManageVisible = ref(false)
const tagTargetName = ref('')
const tagTargetTags = ref<any[]>([])
const tagTargetRow = ref<EnergyRow | null>(null)
const rowTags = (row: any): any[] => (Array.isArray(row?.tags) ? row.tags : [])
const openTagEdit = (row: EnergyRow) => {
  tagTargetRow.value = row
  tagTargetName.value = row.fileName || ''
  tagTargetTags.value = rowTags(row)
  tagDialogVisible.value = true
}
const onTagEditConfirm = async () => {
  tagDialogVisible.value = false
  await loadRecords()
}
const onTagCreated = async () => { await loadRecords() }
const openTagManage = () => { tagManageVisible.value = true }
const onTagManageChanged = async () => { await loadRecords() }

watch(() => filterUtilityType.value, async () => {
  await reloadColumns()
  currentPage.value = 1
  await loadRecords()
})

onMounted(() => {
  loadKb()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})

const onCategoriesChanged = async () => {
  await loadCats()
  await reloadColumns(true)
}
</script>

<style scoped>
.energy-num-input { width: 100%; }
/* 用量「推算」标记：紧跟数值，不换行 */
.row-pushed-tag { margin-left: 4px; }
/* 能耗告警面板：黄色警示条 */
.energy-alert-panel {
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--td-warning-light, #fff7e0);
  border: 1px solid var(--td-warning, #e37318);
  color: var(--td-text-color-primary, #000);
}
.energy-alert-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  color: var(--td-warning, #e37318);
}
.energy-alert-title { font-weight: 600; }
.energy-alert-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  font-size: 13px;
  line-height: 20px;
}
</style>
