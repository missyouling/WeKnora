<template>
  <div class="contract-management-container biz-page">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>合同管理</h2>
        <p class="header-subtitle">合同档案自动归档</p>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化合同管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-合同」知识库">
        <template #image><t-icon name="file-copy" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建合同知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="contract-main biz-main">
      <!-- KPI 概览卡（黄金基准 DashboardKpiGroup：轻量聚合自列表数据，不依赖后端统计接口） -->
      <DashboardKpiGroup :loading="false" :cards="overviewCards" :active-type="filterContractType" @card-click="onOverviewCardClick" />
      <!-- 列表视图容器（发票基准 .invoice-list-view 同款：工具栏 + 状态条 + 列表 + 底栏一体） -->
      <div class="biz-list-view">
      <!-- 筛选工具栏（复用中台组件，直接位于列表容器，对齐发票基准：右组按钮由组件内部 margin-left:auto 贴右） -->
      <!-- 筛选工具栏走中台 BusinessFilterBar（与发票同款）：筛选器改为 FilterFieldSpec
           声明，分类切换只由顶部 KPI 卡片承担，工具栏不再出现分类下拉 -->
          <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
            :columns="effectiveColumns" :visible-keys="visibleColKeys" :selected-count="selectedRowKeys.length"
            settings-text="设置" :primary-action="{ text: '上传合同', icon: 'upload' }" hide-batch-bar
            @columns-change="onVisibleColKeysChange" @reset-columns="resetColumns"
            @select-all-columns="selectAllColumns" @filter-change="applyFilter"
            @settings="settingsVisible = true" @primary="uploadVisible = true"
            @clear-selection="clearSelection" />

      <!-- 进行中文件状态横幅（发票基准同款：优先上传实时进度轮播，关闭弹窗后由轮询派生解析/提取进度，多文件滚动展示） -->
      <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

      <!-- 合同列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
        <div class="doc-list-view">
          <t-table
        ref="contractTableRef"
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
        <template #contract_no="{ row }: any">
          <span class="row-contract-no" :title="row.contract_no || row.fileName">{{ row.contract_no }}</span>
        </template>
        <template #contract_name="{ row }: any">
          <span class="row-text" :title="row.contract_name || row.fileName">{{ row.contract_name || row.fileName }}</span>
        </template>
        <template #contract_type="{ row }: any">
          <span class="row-text" :title="row.contract_type">{{ row.contract_type }}</span>
        </template>
        <template #party_a="{ row }: any">
          <span class="row-text" :title="row.party_a">{{ row.party_a }}</span>
        </template>
        <template #party_b="{ row }: any">
          <span class="row-text" :title="row.party_b">{{ row.party_b }}</span>
        </template>
        <template #sign_date="{ row }: any">
          <span class="row-mono">{{ row.sign_date }}</span>
        </template>
        <template #expire_date="{ row }: any">
          <span class="row-mono">{{ row.expire_date }}</span>
        </template>
        <template #amount="{ row }: any">
          <span class="row-mono row-amount">{{ formatAmount(row.amount) }}</span>
        </template>
        <template #tax_rate="{ row }: any">
          <span class="row-mono row-rate">{{ formatRate(row.tax_rate) }}</span>
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
        <template #performance_status="{ row }: any">
          <t-tag v-if="row.performance_status" size="small" :theme="fulfillStatusTheme(row.performance_status)" variant="light-outline">
            {{ row.performance_status }}
          </t-tag>
          <span v-else class="row-muted">--</span>
        </template>
        <template #party_a_tax_no="{ row }: any">
          <span class="row-mono" :title="row.party_a_tax_no">{{ row.party_a_tax_no }}</span>
        </template>
        <template #party_b_tax_no="{ row }: any">
          <span class="row-mono" :title="row.party_b_tax_no">{{ row.party_b_tax_no }}</span>
        </template>
        <template #effective_date="{ row }: any">
          <span class="row-mono">{{ row.effective_date }}</span>
        </template>
        <template #payment_method="{ row }: any">
          <span class="row-text">{{ row.payment_method }}</span>
        </template>
        <template #handler="{ row }: any">
          <span class="row-text">{{ row.handler }}</span>
        </template>
        <template #department="{ row }: any">
          <span class="row-text">{{ row.department }}</span>
        </template>
        <template #fileName="{ row }: any">
          <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
        </template>
      </t-table>
    </div>
      <!-- 底部汇总（列表容器内部底部固定，不随表格滚动；发票基准同款位置与样式） -->
      <div class="doc-summary-bar">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 份</span>
        <span v-if="displaySummary.total" class="doc-summary-item">
          合同金额合计 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTotal) }}</span>
        </span>
      </div>
    </div>
    </div>

    <!-- 自有批量浮条（发票基准同款：hide-batch-bar 后自有 fixed 条） -->
    <div v-if="selectedRowKeys.length" class="doc-batch-bar" role="region">
      <div class="batch-bar-inner">
        <div class="batch-bar-left">
          <span class="batch-bar-count">已选 {{ selectedRowKeys.length }} 项</span>
          <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelectionSafe">
            清除
          </t-button>
        </div>
        <div class="batch-bar-actions">
          <t-popconfirm theme="warning"
            :content="reparseConfirmText"
            :confirm-btn="{ content: '重新解析', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="handleReparse">
            <t-button theme="default" variant="outline" size="small" :loading="reparseBusy" @click.stop>
              <template #icon><t-icon name="refresh" size="14px" /></template>
              重新解析
            </t-button>
          </t-popconfirm>
          <t-popconfirm theme="warning"
            :content="extractConfirmText"
            :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="handleFileExtract">
            <t-button theme="default" variant="outline" size="small" :loading="extractBusy" @click.stop>
              <template #icon><t-icon name="cloud-upload" size="14px" /></template>
              重新提取
            </t-button>
          </t-popconfirm>
          <t-button theme="default" variant="outline" size="small" @click="handleBatchPrint">
            <template #icon><t-icon name="print" size="14px" /></template>
            单页打印
          </t-button>
          <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handleBatchCatalog">
            <template #icon><t-icon name="file-paste" size="14px" /></template>
            列表打印
          </t-button>
          <t-popconfirm theme="warning" v-model:visible="delPopVisible" placement="top"
            :confirm-btn="null" :cancel-btn="null" :popup-props="{ overlayStyle: { width: 'auto' } }">
            <template #content>
              <div class="del-pop-body">
                <div class="del-pop-title">确定删除所选 {{ selectedRowKeys.length }} 个合同记录吗？删除后不可恢复。</div>
                <div class="del-pop-actions">
                  <t-button size="small" theme="danger" @click="confirmFileDelete">删除</t-button>
                  <t-button size="small" variant="text" @click="closeDelPop">取消</t-button>
                </div>
              </div>
            </template>
            <t-button theme="danger" variant="outline" size="small" @click.stop>
              <template #icon><t-icon name="delete" size="14px" /></template>
              删除记录
            </t-button>
          </t-popconfirm>
        </div>
      </div>
    </div>

    </div>

    <!-- 创建知识库向导 -->
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-合同" kb-desc="合同管理固定使用专用知识库，名称不可修改" description-placeholder="用于存放并解析合同文件，自动提取合同字段" default-description="用于存放并解析合同文件，自动提取合同字段" model-tip="提取模型将复用下方「对话模型」，用于解析合同字段。若列表为空，请先在系统设置中添加模型。" @created="onKbCreated" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab，黄金基准 StandardSettingDrawer） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" :scope="'contract'"
      kb-name="日常事务-合同" title="合同设置" @saved="() => reloadColumns(true)" />

    <!-- 上传弹窗（对齐发票/车队：分类选择 + 拖拽/选择/粘贴 + 进度 + 防重检测） -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="contract" title="上传合同"
      :type-options="uploadTypeOptions" @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 历史记录（对齐发票基准：已上传文件抽屉，展示解析/提取状态与合同类型） -->
    <FleetUploadHistoryDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" scope="contract"
      @changed="onUploadHistoryChanged" />


    <!-- 合同详情抽屉（竖向区块，可拖宽，上下滚动） -->
    <!-- 合同详情抽屉：中台 BusinessSplitDetailDrawer（与发票同款单栏结构：
         质量面板 → 两栏动态字段 → 底部折叠源文件预览） -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="currentRow"
      :page-no="detailPageNo" :form="editFormWrapper" :fields="editFieldDefs" :sub-fields="[]"
      :supports-sub-fields="false" :block-title="editForm.contract_type || '合同信息'"
      :quality="detailQuality" :label-of="qualityLabel" :dirty="detailDirty" :saving="saving"
      :storage-key="'weknora-contract-drawer-width'" @save="saveContractDetail" @delete="onDetailDelete"
      @re-extract="onDetailReExtract">
      <!-- 业务特有控件：类型下拉（改类型会联动归档分类，必须走页面自己的 options） -->
      <template #field-contract_type>
        <t-select v-model="editForm.contract_type" :options="contractTypeOptions" clearable filterable allow-create
          placeholder="选择类型" />
      </template>
      <template #field-payment_method>
        <t-select v-model="editForm.payment_method" :options="PAYMENT_METHODS.map(v => ({ value: v, label: v }))"
          clearable filterable allow-create placeholder="选择付款方式" />
      </template>
      <template #field-extract_status>
        <t-tag v-if="statusOf(currentRow || {}).label !== '--'" size="small" :theme="statusOf(currentRow || {}).theme"
          variant="light-outline" class="row-status-tag">{{ statusOf(currentRow || {}).label }}</t-tag>
        <span v-else class="row-muted">--</span>
      </template>
      <!-- 履约状态为系统按到期日期派生的只读字段（履行中/即将到期/已超期）：渲染只读标签，不提供编辑控件 -->
      <template #field-performance_status>
        <t-tag v-if="editForm.performance_status" size="small" :theme="fulfillStatusTheme(editForm.performance_status)"
          variant="light-outline">{{ editForm.performance_status }}</t-tag>
        <span v-else class="row-muted">--</span>
      </template>
      <template #field-tags>
        <div v-if="rowTags(currentRow || {}).length" class="detail-tags-readonly">
          <t-tag v-for="t in rowTags(currentRow || {})" :key="t.id || t.name" size="small" variant="light-outline"
            class="row-tag">{{ t.name }}</t-tag>
        </div>
        <span v-else class="row-muted">—</span>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 合同编号重复二次确认：强制保存前询问（确认后带 force 重试，取消留在编辑态） -->
    <t-dialog v-model:visible="dupConfirmVisible" header="合同编号重复" width="420" @confirm="onDupConfirm">
      <span>系统已存在相同的合同编号「{{ dupNo }}」，是否强制保存？</span>
    </t-dialog>

    <!-- 标签编辑（复用原项目组件） -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理抽屉（复用原项目组件） -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 打印预览弹窗（自定义实现，完全可控；弃用 TDesign dialog） -->
    <teleport to="body">
      <div v-if="printVisible" class="contract-print-mask">
        <div class="contract-print-dialog">
          <div class="contract-print-header">
            <span class="contract-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 份）` }}</span>
            <t-button variant="text" size="small" class="contract-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="contract-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并合同 PDF…'" />
            </div>
          </div>
          <div class="contract-print-footer">
            <t-button variant="outline" size="small" @click="printVisible = false">关闭</t-button>
            <t-button v-if="printMode === 'catalog'" variant="outline" size="small" :disabled="!printUrl"
              @click="downloadCatalogPdf">
              <template #icon><t-icon name="download" size="14px" /></template>
              下载
            </t-button>
            <t-button theme="primary" size="small" :loading="printBusy" :disabled="!printUrl" @click="doPrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
          </div>
        </div>
      </div>
    </teleport>
  </div>
</template>

<!-- 四模块共享列表样式（全局引入，靠 .biz-page 前缀隔离，不影响车队/能耗/账单等页面） -->
<style>
@import './styles/business-list-shared.css';
</style>

<script setup lang="ts">
import {
  ref,
  shallowRef,
  computed,
  watch,
  onMounted,
  onBeforeUnmount,
} from 'vue'
import { listFleetCategories } from '@/api/fleet'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import type { KpiCard } from '@/components/business/DashboardKpiGroup.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import { parseExtractQuality } from './extractQuality'
import { extractFieldLabel } from './extractFieldMeta'
import BusinessFilterBar from './BusinessFilterBar.vue'
import { useBusinessListSync } from './useBusinessListSync'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import FleetUploadDialog from './FleetUploadDialog.vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  batchDeleteKnowledge,
  delKnowledgeDetails,
  updateBusinessMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractBusinessDocument,
  listContractRecords,
  listContractOverview,
  previewKnowledgeFile,
  reparseKnowledge,
  getRecognitionConfig,
} from '@/api/knowledge-base'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import { pdfPageOf } from './pdfPage'
import { CONTRACT_FIELD_ORDER, contractFieldLabel, contractKeyOf } from './contractFieldLabels'
import { buildDetailFieldDefs } from './businessDetailForm'
import { appendListSystemColumns, buildColumnsFromSubs } from './sixWaySync'
import { useBusinessPolling } from '@/composables/useBusinessPolling'
import { isExtractInProgress } from '@/composables/extractConflict'

const KB_NAME = '日常事务-合同'
const PAGE_SIZE = 20

// 履约状态为系统按到期日期自动派生的动态字段（非提取字段，不落库人工值）：
// 已超期（早于今天）/ 即将到期（未来 30 天内）/ 履行中（其余，含未填到期日期）
const FULFILL_STATUSES = ['履行中', '即将到期', '已超期']
const EXPIRY_WARN_DAYS = 30
// 按到期日期派生履约状态（黄金基准：状态列自动计算，不依赖模型提取值）
const calcFulfillStatus = (expire_date?: string): string => {
  if (!expire_date) return ''
  const d = new Date(expire_date)
  if (Number.isNaN(d.getTime())) return ''
  const now = new Date()
  now.setHours(0, 0, 0, 0)
  const diff = Math.round((d.getTime() - now.getTime()) / 86400000)
  if (diff < 0) return '已超期'
  if (diff <= EXPIRY_WARN_DAYS) return '即将到期'
  return '履行中'
}
// 付款方式枚举
const PAYMENT_METHODS = ['一次性', '分期', '按进度']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
// 列定义 = 新契约键严格顺序（11 标准默认显示 + 15 扩展默认不显示），fileName 为系统列置尾；
// 历史字段列（重置前的税率/质保金等）保留在系统列之后，供存量数据在字段筛选器中按需开启，
// 不进入新字段主轴（与后端 contractCatalog() 顺序同源，contractFieldLabels.test.ts 锁定）。
const DEFAULT_CONTRACT_COLUMNS: ColumnDef[] = [
  // ---- 标准 11（默认显示）----
  { key: 'contract_no', label: '合同编号', default: true, w: '1.4fr' },
  { key: 'contract_name', label: '合同名称', default: true, w: '1.6fr' },
  { key: 'contract_type', label: '合同类型', default: true, w: '1fr' },
  { key: 'party_a', label: '甲方', default: true, w: '1.4fr' },
  { key: 'party_b', label: '乙方', default: true, w: '1.4fr' },
  { key: 'sign_date', label: '签订日期', default: true, w: '1.1fr' },
  { key: 'expire_date', label: '到期日期', default: true, w: '1.1fr' },
  { key: 'amount', label: '合同金额', default: true, w: '1.2fr' },
  { key: 'extract_status', label: '状态', default: true, w: '1fr' },
  { key: 'performance_status', label: '履约状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  // ---- 扩展 15（默认不显示，可在字段筛选器开启）----
  { key: 'party_a_contact', label: '甲方经办人', default: false, w: '1fr' },
  { key: 'party_a_tax_no', label: '甲方税号', default: false, w: '1.3fr' },
  { key: 'party_b_contact', label: '乙方经办人', default: false, w: '1fr' },
  { key: 'party_b_tax_no', label: '乙方税号', default: false, w: '1.3fr' },
  { key: 'party_a_address', label: '甲方地址', default: false, w: '1.6fr' },
  { key: 'party_a_phone', label: '甲方电话', default: false, w: '1fr' },
  { key: 'party_a_bank', label: '甲方开户行', default: false, w: '1.4fr' },
  { key: 'party_a_account', label: '甲方账号', default: false, w: '1.3fr' },
  { key: 'party_b_address', label: '乙方地址', default: false, w: '1.6fr' },
  { key: 'party_b_phone', label: '乙方电话', default: false, w: '1fr' },
  { key: 'party_b_bank', label: '乙方开户行', default: false, w: '1.4fr' },
  { key: 'party_b_account', label: '乙方账号', default: false, w: '1.3fr' },
  { key: 'payment_method', label: '付款方式', default: false, w: '1fr' },
  { key: 'sign_location', label: '签订地点', default: false, w: '1.2fr' },
  { key: 'summary', label: '摘要', default: false, w: '1.6fr' },
  // ---- 系统列（置尾）----
  // 历史字段（税率/质保金等）不进入默认列：老环境的分类 subs 自带旧键（reconcile append-only
  // 保留），customColumns 走 subs 仍可显示；新环境的提取结果本就是新契约键。
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
// 目录顺序卫兵：与 contractFieldLabels 的 CONTRACT_FIELD_ORDER 严格一致（单测锁定）
if (DEFAULT_CONTRACT_COLUMNS.slice(0, CONTRACT_FIELD_ORDER.length).map((c) => c.key).join(',') !== CONTRACT_FIELD_ORDER.join(',')) {
  console.warn('[contract] DEFAULT_CONTRACT_COLUMNS 前 26 列与 CONTRACT_FIELD_ORDER 不一致')
}
const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns,
  selectedRowKeys, onSelectChange, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-contract-list-columns',
  defaultColumns: DEFAULT_CONTRACT_COLUMNS,
})

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)

interface KnowledgeItem {
  id: string
  file_name?: string
  file_type?: string
  title?: string
  description?: string
  parse_status?: string
  summary_status?: string
  tags?: any[]
  custom_metadata?: any
  [key: string]: any
}


interface ContractRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'contract' | 'not_contract' | 'unknown' | 'pending'
  contract_no?: string
  contract_name?: string
  contract_type?: string
  sign_date?: string
  effective_date?: string
  expire_date?: string
  sign_location?: string
  party_a?: string
  party_a_tax_no?: string
  party_a_address?: string
  party_a_phone?: string
  party_a_bank?: string
  party_a_account?: string
  party_b?: string
  party_b_tax_no?: string
  party_b_address?: string
  party_b_phone?: string
  party_b_bank?: string
  party_b_account?: string
  amount?: number
  tax_rate?: number
  payment_method?: string
  quality_bond?: number
  liquidated_damages?: number
  subject?: string
  quantity?: number
  unit_price?: number
  performance_period?: string
  handler?: string
  department?: string
  remark?: string
  performance_status?: string
  page?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}


// ---- 列表 ----
const items = ref<KnowledgeItem[]>([])
// shallowRef：几千条合同时消除深层 Proxy 代理开销（行数据只在整体赋值时更新，
// 单行变化必须整体替换数组，见 saveContractDetail 的 map 重建逻辑）
const contractRows = shallowRef<ContractRow[]>([])
// 进行中的文件（解析中/提取中/待提取），在合同级列表顶部以状态行展示
const contractSummary = ref<{ total: number; sumAmount: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
// 请求序号（黄金基准发票同款）：类型/筛选切换会 reset 并作废旧的在途请求，
// 旧筛选响应返回时按序号丢弃，避免轮询/翻页的旧数据污染新筛选结果。
let listSeq = 0
let loadTimer: ReturnType<typeof setTimeout> | null = null
const keyword = ref('')
const filterContractType = ref('')
const filterFulfillStatus = ref('')
// 类型下拉（六同步：以 categories(scope=contract) 为权威源，回退识别规则配置类型）
const contractTypeRaw = computed<Array<{ value: string; label: string }>>(() => {
  const catNames = contractCats.value
    .filter((c: any) => c.enabled !== false)
    .map((c: any) => c.name)
    .filter((n: string) => n && n.trim())
  const base = catNames.length ? catNames : kbTypes.value
  return base.map((v: string) => ({ value: v, label: v }))
})
// 工具栏筛选下拉（六同步：与发票基准一致，只列具体分类、无聚合项，始终选中一项、禁清空）
const contractTypeOptions = computed<Array<{ value: string; label: string }>>(() => contractTypeRaw.value)
const dateRange = ref<Array<string>>([])

// 列显隐

const tableColumns = computed(() => {
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 46 }, { colKey: 'serial-number', title: '', width: 44 }]
  const rows = filteredRows.value.filter(r => r.kind !== 'pending')
  for (const c of visibleColDefs.value) {
    // 列宽：字段配置 width>0 固定（clamp 60~400）；0/未配置按内容自适应（封顶 150），
    // 多行文本列走固定中等宽度（长内容按最长单行估算会失真，见 columnWidth.ts）
    const w = colWidthOf(c.width, c.label, rows.map(r => colValue(r, c.key)), (c as any).dataType)
    // 多行文本列关闭 ellipsis：TDesign 的 ellipsis 会包一层 nowrap + overflow hidden，
    // 换行样式会被直接吃掉，长文本仍旧截断
    cols.push({
      colKey: c.key,
      title: c.label,
      ellipsis: ellipsisOf(c as any),
      className: cellClassOf(c as any),
      width: w,
      // 类型随列下传：tableScrollOf 据此判断是否需要关掉固定行高（多行文本必须逐行测高）
      dataType: (c as any).dataType,
    })
  }
  return cols
})

// 虚拟滚动配置跟随列定义：出现多行文本列（data_type=textarea）必须关掉固定行高，
// 否则 TDesign 仍按 48px 推算滚动位置，折行后的行会错位、长文本被裁在窗口外。
const tableScroll = computed(() => tableScrollOf(tableColumns.value))


// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<ContractRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<ContractRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({})
const saving = ref(false)

// 抽屉宽度（可拖动，localStorage 记忆）
// 打印预览
const printVisible = ref(false)
const historyVisible = ref(false)
const settingsVisible = ref(false)
const printCount = ref(0)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)
const printMode = ref<'print' | 'catalog'>('print')
const catalogBusy = ref(false)


// ---- KB 检测 ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      await loadTags()
      await loadTypeOptions()
      await loadContractCats()
      await cleanNonContractFiles()
      // 列表与概览卡并行加载（对齐发票基准：卡片不等待列表串行完成，消除"先列表后卡片"的滞后感）
      await Promise.all([loadFiles(), loadUploadStats(), loadContractOverview()])
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
    await loadTags()
    await loadTypeOptions()
    await loadContractCats()
    await cleanNonContractFiles()
    await Promise.all([loadFiles(), loadUploadStats(), loadContractOverview()])
    startPolling()
  }
}

// ---- 存量非合同清理：扫描知识库中已标记 not_contract 的文件并自动删除 ----
let cleaningNonContract = false
const cleanNonContractFiles = async () => {
  if (!kbId.value || cleaningNonContract) return
  cleaningNonContract = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_contract' || it.custom_metadata?.extract_status === 'not_contract')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非合同文件（旧数据清理）`)
      loadTypeOptions()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonContract = false }
}

// ---- 合同类型下拉（六同步基准同款）：分类以 categories(scope=contract) 为权威源（含自定义分类），
// 未入库时回退识别规则配置的类型列表；静态枚举已废弃，不再追加写死类型 ----
const kbTypes = ref<string[]>([])
const loadTypeOptions = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    if (Array.isArray(c?.types) && c.types.length) kbTypes.value = c.types.filter(Boolean)
  } catch { /* 保持现状 */ }
  ensureContractType()
}

// ---- 标签 ----
const loadTags = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeTags(kbId.value, { page: 1, page_size: 100 })
    const pageData = (res?.data || {}) as { data?: any[]; total?: number }
    tagList.value = (pageData.data || []).map((tag: any) => ({ ...tag, id: String(tag.id) }))
  } catch { /* 标签加载失败不阻塞 */ }
}

const openTagEdit = (row: ContractRow) => {
  tagTarget.value = row
  tagDialogVisible.value = true
}

// 行内 tags 可能只携带标签名（后端返回字符串数组时被转为 { id: name }），
// 打开弹窗时按 tagList 反查真实 id，保证已选回显与提交 id 正确（对齐发票基准）
const tagTargetTags = computed(() => {
  const tags = tagTarget.value ? rowTags(tagTarget.value) : []
  return tags.map((t: any) => {
    const hit = tagList.value.find((x: any) => String(x.id) === String(t.id) || x.name === t.name || x.name === t)
    return hit ? { id: String(hit.id), name: String(hit.name || t.name || t) }
      : { id: String(t.id ?? t.name ?? t), name: String(t.name ?? t) }
  })
})

const onTagEditConfirm = async (tagIds: string[]) => {
  if (!tagTarget.value) return
  try {
    // 兜底：混合传入的标签名/ID 统一反查真实 tag id（标签名会触发后端 400，对齐发票基准）
    const realIds = tagIds.map(id => {
      const hit = tagList.value.find((t: any) => String(t.id) === id || t.name === id)
      return hit ? String(hit.id) : id
    })
    await updateKnowledgeTagBatch({ updates: { [tagTarget.value.knowledgeId]: realIds } })
    MessagePlugin.success('标签已更新')
    await loadFiles(true)
    loadContractOverview()
  } catch (e: any) {
    // 500/无具体信息：固定文案防止展示后端原始错误；有具体 message（如 400 原因）时保留
    if (e?.status === 500 || !e?.message) MessagePlugin.error('标签更新失败，请稍后重试')
    else MessagePlugin.error(e?.message)
  }
}

const onTagCreated = () => { loadTags() }
const openTagManage = () => { tagManageVisible.value = true }
const onTagManageChanged = () => {
  loadTags()
  loadFiles(true)
}


// ---- 列表加载（合同级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  // 防重检查必须在 ++listSeq 之前：非 reset（翻页/onTick 轮询）若在 reset 请求进行中空转，
  // 绝不能递增请求序号，否则进行中的 reset 请求会被误判为「过期响应」而丢弃，
  // 其 finally 不再清理 listLoading、超时 timer 回调也因 seq 不匹配而失效 → 永久转圈。
  if (!reset && (listLoading.value || loadingMore.value)) return
  const mySeq = ++listSeq
  if (reset) {
    // reset（类型/筛选切换/手动刷新）接管并重置超时计时器
    if (loadTimer) { clearTimeout(loadTimer); loadTimer = null }
    page.value = 1
    contractRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
    loadTimer = setTimeout(() => {
      if (listLoading.value && mySeq === listSeq) {
        listLoading.value = false
        loadingMore.value = false
        MessagePlugin.warning('列表加载超时，请稍后重试')
      }
    }, 15000)
  } else {
    loadingMore.value = true
  }
  try {
    const res: any = await listContractRecords(kbId.value, {
      q: keyword.value || undefined,
      contract_type: filterContractType.value || undefined,
      // 履约状态为前端按到期日期派生的动态字段，后端无存储值可过滤，交由前端筛选
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    // 过期响应丢弃：此期间列表已按新筛选重置，旧结果（如轮询/翻页的旧类型数据）不得混入
    if (mySeq !== listSeq) return
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    contractSummary.value = {
      total,
      sumAmount: Number(res?.sum_amount || 0),
      sumTotal: Number(res?.sum_total || 0),
    }
    const existing = new Set(contractRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapContractRecord).filter(r => !existing.has(r.rowKey))
    contractRows.value = [...contractRows.value, ...fresh]
    hasMore.value = contractRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    if (mySeq !== listSeq) return
    MessagePlugin.error(e?.message || '合同列表加载失败')
  } finally {
    if (loadTimer && mySeq === listSeq) { clearTimeout(loadTimer); loadTimer = null }
    // 仅最新请求可清理 loading 标志；过期响应保持其父请求的进行中状态
    if (mySeq === listSeq) {
      listLoading.value = false
      loadingMore.value = false
    }
  }
}

// 把后端合同级记录（ContractRecord）映射为前端行
const mapContractRecord = (r: any): ContractRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.contract_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'contract',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  contract_no: r.contract_no || '',
  contract_name: r.contract_name || '',
  contract_type: r.contract_type || '',
  sign_date: r.sign_date || '',
  effective_date: r.effective_date || '',
  // 新契约键优先；旧键（expiry_date / sign_place / party_a_name / contract_amount）回退，
  // 兼容 2026-10-21 字段体系重置前落库的数据（后端 UnmarshalJSON 已做同口径兼容，前端双保险）
  expire_date: r.expire_date || r.expiry_date || '',
  sign_location: r.sign_location || r.sign_place || '',
  party_a: r.party_a || r.party_a_name || '',
  party_a_tax_no: r.party_a_tax_no || '',
  party_a_address: r.party_a_address || '',
  party_a_phone: r.party_a_phone || '',
  party_a_bank: r.party_a_bank || '',
  party_a_account: r.party_a_account || '',
  party_b: r.party_b || r.party_b_name || '',
  party_b_tax_no: r.party_b_tax_no || '',
  party_b_address: r.party_b_address || '',
  party_b_phone: r.party_b_phone || '',
  party_b_bank: r.party_b_bank || '',
  party_b_account: r.party_b_account || '',
  amount: numOrUndef(r.amount ?? r.contract_amount),
  tax_rate: numOrUndef(r.tax_rate),
  payment_method: r.payment_method || '',
  quality_bond: numOrUndef(r.quality_bond),
  liquidated_damages: numOrUndef(r.liquidated_damages),
  subject: r.subject || '',
  quantity: numOrUndef(r.quantity),
  unit_price: numOrUndef(r.unit_price),
  performance_period: r.performance_period || '',
  handler: r.handler || '',
  department: r.department || '',
  remark: r.remark || '',
  // 履约状态：后端派生字段优先（performance_status），旧数据/本地兜底按到期日期现算
  performance_status: r.performance_status || calcFulfillStatus(r.expire_date || r.expiry_date),
  page: Number(r.page) || 0,
})


const parseCustomMetadata = (item: KnowledgeItem): ContractRow[] => {
  const meta = item.custom_metadata
  const fileName = item.file_name || item.title || item.id
  const parseStatus = item.parse_status || ''
  const summaryStatus = item.summary_status || ''
  const fileType = item.file_type || ''
  const tags = item.tags || []
  const description = item.description || ''
  const base = { parseStatus, summaryStatus, fileType, tags, description }
  if (!meta || typeof meta !== 'object') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: '', kind: 'unknown', ...base }]
  }
  const kind = meta.kind || 'unknown'
  const extractStatus = meta.extract_status || ''
  const extractError = meta.extract_error || ''
  if (kind === 'not_contract') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_contract', extractError, kind, ...base }]
  }
  const contracts = Array.isArray(meta.contracts) ? meta.contracts : []
  if (contracts.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return contracts.map((ct: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: contracts.length > 1 ? `${fileName}（第${idx + 1}份）` : fileName,
    multiIndex: contracts.length > 1 ? `${idx + 1}/${contracts.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    contract_no: ct.contract_no || '',
    contract_name: ct.contract_name || '',
    contract_type: ct.contract_type || '',
    sign_date: ct.sign_date || '',
    effective_date: ct.effective_date || '',
    // 同 mapContractRecord：新契约键优先，旧键回退（兼容重置前落库数据）
    expire_date: ct.expire_date || ct.expiry_date || '',
    sign_location: ct.sign_location || ct.sign_place || '',
    party_a: ct.party_a || ct.party_a_name || '',
    party_a_tax_no: ct.party_a_tax_no || '',
    party_a_address: ct.party_a_address || '',
    party_a_phone: ct.party_a_phone || '',
    party_a_bank: ct.party_a_bank || '',
    party_a_account: ct.party_a_account || '',
    party_b: ct.party_b || ct.party_b_name || '',
    party_b_tax_no: ct.party_b_tax_no || '',
    party_b_address: ct.party_b_address || '',
    party_b_phone: ct.party_b_phone || '',
    party_b_bank: ct.party_b_bank || '',
    party_b_account: ct.party_b_account || '',
    amount: numOrUndef(ct.amount ?? ct.contract_amount),
    tax_rate: numOrUndef(ct.tax_rate),
    payment_method: ct.payment_method || '',
    quality_bond: numOrUndef(ct.quality_bond),
    liquidated_damages: numOrUndef(ct.liquidated_damages),
    subject: ct.subject || '',
    quantity: numOrUndef(ct.quantity),
    unit_price: numOrUndef(ct.unit_price),
    performance_period: ct.performance_period || '',
    handler: ct.handler || '',
    department: ct.department || '',
    remark: ct.remark || '',
    performance_status: ct.performance_status || calcFulfillStatus(ct.expire_date || ct.expiry_date),
    page: Number(ct.page) || 0,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端合同级接口完成，前端直接使用返回的分页数据；
// 履约状态筛选：行上带后端派生 performance_status（旧数据回退按到期日期现算）
const fulfillFilteredRows = computed(() => {
  if (!filterFulfillStatus.value) return contractRows.value
  return contractRows.value.filter(r =>
    (r.performance_status || calcFulfillStatus(r.expire_date)) === filterFulfillStatus.value)
})
// 在途文件（解析中/提取中/待提取）**只**由页面顶部 BusinessParseStatusBar 与
// 「历史记录 → 正在执行」抽屉展示，严禁在主列表生成占位行（2026-10-22 真机事故：
// 上传/重提时表格里多出 `[↻ 解析中]` 空壳行，把真实合同挤出首屏）。
const filteredRows = computed(() => fulfillFilteredRows.value)

// 扁平筛选态：与上面三个 ref 双向桥接，中台组件只发不可变更新；
// 页面原有筛选语义（AND 复合查询）保持不变。
const filterState = computed<FilterState>({
  get: () => ({
    keyword: keyword.value,
    fulfillStatus: filterFulfillStatus.value,
    dateRange: dateRange.value,
  }),
  set: (v) => {
    keyword.value = String(v?.keyword ?? '')
    filterFulfillStatus.value = String(v?.fulfillStatus ?? '')
    dateRange.value = Array.isArray(v?.dateRange) ? v.dateRange : []
  },
})
// 合同类型筛选统一仅由顶部概览卡片承担，工具栏不再重复下拉
const filterFields = computed<FilterFieldSpec[]>(() => [
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'fulfillStatus', type: 'select', placeholder: '履约状态', icon: 'check-circle', options: fulfillStatusOptions.value },
  { key: 'dateRange', type: 'date-range', placeholder: '签订日期', icon: 'time' },
])
const onVisibleColKeysChange = (keys: string[]) => { visibleColKeys.value = keys }

const fulfillStatusOptions = computed(() => FULFILL_STATUSES.map(v => ({ value: v, label: v })))

// 履约状态标签主题（派生三态：履行中 success / 即将到期 warning / 已超期 danger）
const fulfillStatusTheme = (s: string): 'success' | 'warning' | 'default' | 'danger' => {
  switch (s) {
    case '履行中': return 'success'
    case '即将到期': return 'warning'
    case '已超期': return 'danger'
    default: return 'default'
  }
}


// ---- 多选 ----
const selectedRows = computed(() => {
  const byKey = new Map(contractRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as ContractRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中合同的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  let sumTotal = 0
  for (const r of rows) {
    sumTotal += Number(r.amount) || 0
  }
  return { total: rows.length, sumAmount: sumTotal, sumTotal }
})
const displaySummary = computed(() => {
  if (selectedSummary.value) return selectedSummary.value
  // 履约状态筛选为前端派生过滤：汇总按过滤后的已加载行现算（与列表可见行一致）
  if (filterFulfillStatus.value) {
    const rows = fulfillFilteredRows.value
    let sum = 0
    for (const r of rows) sum += Number(r.amount) || 0
    return { total: rows.length, sumAmount: sum, sumTotal: sum }
  }
  return contractSummary.value
})

const applyFilter = () => { loadFiles(true) }

// ---- 筛选联动闭环（黄金基准发票同款） ----
// 多选清除：先调用 t-table 实例 clearSelected（内部清空并 emit select-change 同步外部），
// 再兜底清空 selectedRowKeys，杜绝 TDesign 受控反向写回旧 key 导致浮条不消失
const contractTableRef = ref()
const clearSelectionSafe = () => {
  try { contractTableRef.value?.clearSelected?.() } catch { /* ignore */ }
  clearSelection()
}
// 搜索态：关键词非空时类型下拉可临时清空（全局搜索优先）
const searchActive = computed(() => !!keyword.value.trim())
// 类型筛选锁定（发票基准同款）：始终保证有一项具体分类被选中（禁清空）；
// 搜索态放行（由 keyword watch 负责临时清空与恢复）；当前选中项合法则保留（含路由/上传回填），
// 否则按分类顺序选中第一个启用分类。
const ensureContractType = () => {
  if (searchActive.value) return
  const valid = new Set(contractTypeOptions.value.map((o) => o.value))
  // 「其它合同」为排除式筛选态（不在分类下拉 options 中，点击其它合同卡进入），分类重载时须放行
  if (filterContractType.value && (valid.has(filterContractType.value) || filterContractType.value === '其它合同')) return
  // 空值 = 「全部」态（点击「全部合同」卡进入，对齐发票「已收录发票」卡）：放行避免被
  // 弹回第一个分类；工具栏下拉本身禁清空，用户无法手动进入此态
  if (filterContractType.value === '') return
  const cats = contractCats.value.filter((c: any) => c.enabled !== false).map((c: any) => c.name)
  filterContractType.value = (cats.length ? cats[0] : contractTypeOptions.value[0]?.value) || ''
}
// 类型切换：清空多选（防跨类型脏数据残留）+ 重建当前分类列 + 立即刷新列表（发票基准同款）
watch(filterContractType, () => {
  clearSelectionSafe()
  reloadColumns()
  applyFilter()
})
// 履约状态筛选（前端派生过滤）：切换时清空多选残留并重载列表
watch(filterFulfillStatus, () => {
  clearSelectionSafe()
  applyFilter()
})
// 关键词防抖 300ms 后刷新；输入时临时清空类型筛选（全局搜索），清空词后恢复原类型。
// 履约状态与签订日期筛选保持不动（任务明确要求）。
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let searchRestoreType = ''
watch(keyword, (v) => {
  if (v && v.trim()) {
    if (searchRestoreType === '' && filterContractType.value) searchRestoreType = filterContractType.value
    if (filterContractType.value) filterContractType.value = ''
  } else {
    if (searchRestoreType) {
      filterContractType.value = searchRestoreType
      searchRestoreType = ''
    }
  }
  if (keywordTimer) clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => loadFiles(true), 300)
})

// ---- KPI 概览卡（黄金基准 DashboardKpiGroup：已收录合同 → 各合同类型卡 → 即将到期 → 历史记录） ----
// 数据源为后端全量概览 contractOverview（不受工具栏类型/日期/搜索筛选影响），
// 仅「历史记录」卡沿用独立的上传文件计数（uploadStats，同属全量口径）。
const overviewCards = computed<KpiCard[]>(() => {
  const st = contractOverview.value || {}
  const m = st.current_month || {}
  const total = Number(st.total || 0)
  const expiring = Number(st.expiring || 0)
  // 分类统计：后端按 contract_type 精确聚合（空归「未分类」）
  const catCount: Record<string, number> = {}
  for (const c of (st.by_category || [])) catCount[String(c.category)] = Number(c.count) || 0
  const cards: KpiCard[] = [
    // 已收录合同：全量总份数（含未分类/其它合同与待补录占位行），点击清空筛选进入全部态（对齐发票）
    { key: 'total', label: '已收录合同', icon: 'file-copy', value: String(total), unit: '份', sub: `本月新增 ${Number(m.count || 0)} 份`, theme: 'brand', action: 'all' },
  ]
  // 类型分布卡：以合同分类列表（categories[scope=contract]）为权威源动态生成，顺序 = 分类数组顺序；
  // enabled=false 的分类不出卡片（六同步：同时不出筛选下拉/上传选项）
  const enabledCats = (contractCats.value || []).filter((c: any) => c.enabled !== false)
  let catSum = 0
  for (const c of enabledCats) {
    // 「其它合同」为系统内置兜底分类：统计口径是排除式桶（total - 各启用分类之和），
    // 跳过该分类卡且不计入 catSum，避免与下方排除式卡重复/口径错乱
    if (c.name === '其它合同') continue
    const cnt = catCount[String(c.name)] || 0
    catSum += cnt
    const typeIcon = c.name === '租赁合同' ? 'file-copy' : 'file-1'
    cards.push({ key: `type-${c.name}`, label: c.name, icon: typeIcon, value: String(cnt), unit: '份', theme: 'neutral', action: 'type', typeValue: c.name })
  }
  // 「其它合同」排除式统计桶（收纳未匹配任何启用分类的记录，含未分类/已停用分类存量），
  // 无条件渲染（计数 0 兜底），避免卡片数量二次跳变
  const otherCount = Math.max(0, total - catSum)
  cards.push({ key: 'type-其它合同', label: '其它合同', icon: 'file-unknown', value: String(otherCount), unit: '份', theme: 'neutral', action: 'type', typeValue: '其它合同' })
  // 即将到期（未来 30 天内，全量口径）：warning 主题色警示
  cards.push({ key: 'expiring', label: '即将到期', icon: 'time', value: String(expiring), unit: '份', theme: 'warning', action: 'expiring' })
  // 历史记录（已上传文件，对齐发票基准 fileCount/failed）：异常文件数 sub 展示，点击打开已上传文件抽屉
  const hs = uploadStats.value
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: String(hs.total), unit: '份', sub: hs.failed ? `${hs.failed} 份异常` : '无异常文件', theme: hs.failed > 0 ? 'warning' : 'neutral', action: 'history' })
  return cards
})
// 卡片点击联动（发票基准同款 action 分流：类型卡切筛选；即将到期卡切履约筛选；历史记录卡开抽屉）
const onOverviewCardClick = (card: KpiCard) => {
  // 「已收录合同」卡：清空类型筛选显示全部记录（对齐发票「已收录发票」卡）
  if (card.action === 'all') {
    filterContractType.value = ''
  } else if (card.action === 'type' && card.typeValue) {
    filterContractType.value = card.typeValue // watch 触发 clearSelectionSafe + applyFilter
  } else if (card.action === 'expiring') {
    filterFulfillStatus.value = '即将到期'
  } else if (card.action === 'history') {
    historyVisible.value = true
  }
}

// 懒加载
const listScrollRef = ref<HTMLElement>()
const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200 && hasMore.value && !listLoading.value && !loadingMore.value) {
    loadFiles()
  }
}

// ---- 上传（对齐发票/车队：FleetUploadDialog 弹窗，支持拖拽/选择/剪贴板粘贴 + 防重检测） ----
const uploadVisible = ref(false)
// 上传弹窗分类选项：contract categories（含自定义分类），复合值 contract__名称
const contractCats = ref<any[]>([])
const uploadTypeOptions = computed(() => {
  const cats = contractCats.value.filter((c: any) => c.enabled !== false)
  if (!cats.length) return [{ label: '服务合同', value: 'contract__服务合同' }]
  return cats.map((c: any) => ({ label: c.name, value: `contract__${c.name}` }))
})
const loadContractCats = async () => {
  try {
    const res: any = await listFleetCategories({ scope: 'contract' })
    const cats = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    contractCats.value = cats
  } catch { contractCats.value = [] }
  // 六同步：分类重载后若当前选中项已失效（禁用/改名/删除），回退第一个启用分类
  ensureContractType()
}
// 全量概览统计（对齐发票基准 InvoiceOverviewStats：后端全量聚合，不随列表筛选变化）。
// KPI 卡片一律使用该数据源，保证切换类型/日期/搜索后卡片统计不受影响。
const contractOverview = ref<any>(null)
// 卡片渲染门控：概览统计 + 上传文件计数两个异步源都就绪后才一次渲染全部卡片，
// 避免数值分批跳变（任一请求失败也置 true，用空数据渲染，不永久转圈）
const contractOverviewLoaded = ref(false)
const uploadStatsLoaded = ref(false)
const loadContractOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listContractOverview(kbId.value)
    contractOverview.value = res?.data || res || null
  } catch {
    contractOverview.value = null
  } finally {
    contractOverviewLoaded.value = true
  }
}
const onUploadDone = async (_typeName?: string) => {
  // 上传完成刷新列表与概览卡（合同记录的 contract_type 为 OCR 提取的业务类型，与上传所选字段分类不强制映射，
  // 不主动切换类型筛选，避免提取结果与分类名不一致时列表被过滤为空）
  await Promise.all([loadFiles(true), loadContractOverview()])
}

// 历史记录（已上传文件）计数：KPI「历史记录」卡数据源（对齐发票基准 fileCount/failed，
// 前端轻量聚合：文件总数 + 解析/提取异常数，异常数按已加载页统计）
const uploadStats = ref<{ total: number; failed: number }>({ total: 0, failed: 0 })
const loadUploadStats = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    const total = Number(res?.total ?? arr.length ?? 0)
    const failed = arr.filter((it: any) =>
      it.parse_status === 'failed' || (it.custom_metadata || {}).extract_status === 'failed'
    ).length
    uploadStats.value = { total, failed }
  } catch { uploadStats.value = { total: 0, failed: 0 } } finally {
    uploadStatsLoaded.value = true
  }
}
// 历史记录抽屉变更（删除/重新解析）后同步刷新列表与卡片计数（并行）
// 上传历史抽屉变化：删除（带 deletedIds）走乐观移除+延迟对账；重新提取等立即刷新
const onUploadHistoryChanged = async (payload?: { deletedIds?: string[] }) => {
  if (payload?.deletedIds?.length) {
    optimisticRemoveContractRows(new Set(payload.deletedIds), new Set())
    schedulePostDeleteRefresh()
    return
  }
  await Promise.all([loadFiles(true), loadUploadStats(), loadContractOverview()])
}

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling, scanOnce,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'contract'),
  onPendingFiles: (files) => {
    // 状态条展示所有「进行中」文件：不仅新上传待解析/待提取的文件，也包括
    // 已产生合同行、正处于「重新解析/重新提取」中的文件（重解/重提选中的记录
    // 必然已有合同行，若按行过滤会把它们排除出状态条，只剩 toast 提示）。
    // 轮询已保证 pend 只含解析中/提取中文件，完成/失败的文件不会进入这里。
    pendingFiles.value = files
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是合同文件，已移至删除历史，可在删除历史中恢复`)
    loadTypeOptions()
  },
  onTick: () => loadFiles(),
})

// 需求：解析/提取完成后自动刷新 KPI 卡片统计。
// pendingFiles 由轮询每 3s 更新：集合变化（有文件解析完成进入合同行 / 提取完成 / 失败）
// 即触发概览统计重载，保证卡片数量与本轮真实状态一致。
let lastPendingKey = ''
watch(pendingFiles, (v) => {
  const key = v.map((f: any) => f.id).sort().join(',')
  if (key !== lastPendingKey) {
    lastPendingKey = key
    if (kbId.value) loadContractOverview()
  }
}, { deep: false })

// 上传任务实时进度（来自上传弹窗 @progress，发票/车队同款）：多文件同时进行时轮播展示
const uploadProgress = ref<{ name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]>([])
const progressIndex = ref(0)
let progressTimer: ReturnType<typeof setInterval> | null = null
function onUploadProgress(items: { name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]) {
  uploadProgress.value = filterLiveProgress(items)
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
// 表格里该文件已「提取完成/失败」的，不再保留旧的「正在提取…N%」进度项
function filterLiveProgress(items: any[]) {
  const doneNames = new Set(
    contractRows.value
      .filter((r) => r.extractStatus === 'success' || r.extractStatus === 'failed')
      .map((r) => r.fileName),
  )
  return items.filter((it: any) => !doneNames.has(it.name))
}
// 轮询刷新列表后：用表格实际 extractStatus 清掉已完成文件的残留进度
watch(contractRows, () => {
  if (!uploadProgress.value.length) return
  uploadProgress.value = filterLiveProgress(uploadProgress.value)
})
function stopProgressTimer() {
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}
onBeforeUnmount(stopProgressTimer)

// 解析/提取实时进度（弹窗关闭后兜底数据源）：
// 后端轮询只返回状态枚举（pending/processing/completed + extract_status），无真实百分比。
// 沿用上传弹窗同款"按耗时估算"口径：parsing 8→92、extracting 30→100；
// 起点 = 文件首次进入进行中集合的时刻（fileStartAt 记忆表）。
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
    // 解析已完成、提取进行中 → 提取阶段；否则视为解析阶段
    if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) {
      return { name, stage: 'extracting' as const, percent: Math.min(100, Math.round(30 + elapsed * 2)) }
    }
    return { name, stage: 'parsing' as const, percent: Math.min(92, Math.round(10 + elapsed * 4)) }
  })
})
// 文件离开进行中集合后清理起点记忆，避免 id 复用导致百分比从旧值续算
watch(pendingFiles, (v) => {
  const alive = new Set(v.map((f: any) => f.id))
  for (const id of Array.from(fileStartAt.keys())) {
    if (!alive.has(id)) fileStartAt.delete(id)
  }
})
// 弹窗打开期间优先展示上传弹窗上报的真实进度（含真实上传百分比）；
// 弹窗关闭后 uploadProgress 停更，自动切换到轮询派生的解析/提取进度，状态条不中断。
const displayProgress = computed(() =>
  uploadProgress.value.length ? uploadProgress.value : liveProgress.value,
)

const { statusOf, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'contract', notLabel: '非合同',
})


// ---- 详情抽屉 ----
const openDetail = async (row: ContractRow) => {
  if (row.kind === 'pending') return
  currentRow.value = row
  currentDetail.value = null
  detailVisible.value = true
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    if (detail && typeof detail === 'object') {
      currentDetail.value = detail
      if (detail.custom_metadata) {
        const parsed = parseCustomMetadata(detail)
        // 合同一份文件一份：优先按合同编号匹配详情（合同不按页匹配）
        const match = parsed.find(p => p.contract_no && p.contract_no === row.contract_no)
          || parsed.find(p => p.contract_name === row.contract_name) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.contract_no ? `合同详情 · ${currentRow.value.contract_no}` : '合同详情'
)

// 提取质量与溯源：详情接口的 custom_metadata（多份合同时按当前合同定位 index）。
// 详情未返回前为空质量 → 面板静默，不抢占视觉。
const detailQuality = computed(() => {
  const meta = (currentDetail.value as any)?.custom_metadata
  const list: any[] = Array.isArray(meta?.contracts) ? meta.contracts : []
  let idx = -1
  if (list.length > 1) {
    idx = list.findIndex((c) => !!currentRow.value?.contract_no && c?.contract_no === currentRow.value?.contract_no)
  }
  return parseExtractQuality(meta, idx >= 0 ? idx : undefined)
})
const qualityLabel = (f: string) => extractFieldLabel('contract', f)

// ---- 字段编辑 + 手动保存（黄金基准发票同款：底部「保存/取消」统一提交） ----
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  editForm.value = {
    contract_no: r.contract_no || '',
    contract_name: r.contract_name || '',
    contract_type: r.contract_type || '',
    sign_date: r.sign_date || '',
    effective_date: r.effective_date || '',
    // editForm 键 = 新契约键（与 editFieldDefs / 落库 contracts[] 同形）
    expire_date: r.expire_date || '',
    sign_location: r.sign_location || '',
    party_a: r.party_a || '',
    party_a_contact: r.party_a_contact || '',
    party_a_tax_no: r.party_a_tax_no || '',
    party_a_address: r.party_a_address || '',
    party_a_phone: r.party_a_phone || '',
    party_a_bank: r.party_a_bank || '',
    party_a_account: r.party_a_account || '',
    party_b: r.party_b || '',
    party_b_contact: r.party_b_contact || '',
    party_b_tax_no: r.party_b_tax_no || '',
    party_b_address: r.party_b_address || '',
    party_b_phone: r.party_b_phone || '',
    party_b_bank: r.party_b_bank || '',
    party_b_account: r.party_b_account || '',
    amount: numToStr(r.amount),
    tax_rate_display: r.tax_rate === null || r.tax_rate === undefined ? '' : String(Number(r.tax_rate) * 100),
    payment_method: r.payment_method || '',
    quality_bond: numToStr(r.quality_bond),
    liquidated_damages: numToStr(r.liquidated_damages),
    subject: r.subject || '',
    quantity: numToStr(r.quantity),
    unit_price: numToStr(r.unit_price),
    performance_period: r.performance_period || '',
    handler: r.handler || '',
    department: r.department || '',
    remark: r.remark || '',
    summary: r.summary || r.description || '',
    // 履约状态为派生只读字段：仅填充供抽屉展示，不参与持久化（updated 不含此键）
    performance_status: r.performance_status || calcFulfillStatus(r.expire_date),
  }
  detailBaseline.value = JSON.parse(JSON.stringify(editForm.value))
}

// 抽屉底部动作：删除（整份文件仅一份合同时级联删除源文件；多份时只移除该条）
const detailPageNo = computed(() => pdfPageOf({ page: currentRow.value?.page, page_no: currentRow.value?.pageNo }))

// 中台 BusinessSplitDetailDrawer 的表单契约是 `{ data, remarkParts, items }`
// （抽屉内部一律读写 `form.data[字段名]`）。合同的 editForm 是**扁平**结构
// （contract_no / sign_date…，与落库 contracts[] 同形），直接传进去会让
// `form.data` 为 undefined → 真机报一堆 Cannot read properties of undefined。
// 这里做零成本包装：data 指向同一个 editForm 对象，抽屉的 v-model 写入
// 直接落在 editForm 上，保存链路一行都不用改。
const editFormWrapper = computed(() => ({
  data: editForm.value,
  remarkParts: {} as Record<string, string>,
  items: [] as any[],
}))

// 脏检查：与填入时基线比对（抽屉「保存」按钮靠它启用）
const detailBaseline = ref<any>(null)
const detailDirty = computed(() => {
  if (!detailBaseline.value) return false
  return JSON.stringify(detailBaseline.value) !== JSON.stringify(editForm.value)
})

// 详情抽屉动态字段：**唯一数据源 = 当前分类的 subs**（六同步铁律）。
// 字段集合/名称/顺序/dataType 全部由 subs 派生：用户在「字段定义」里删除字段、
// 改名（摘要→摘要2）、改类型（text→array）后，抽屉立即跟随。
// 绝不回退静态 DEFAULT_CONTRACT_COLUMNS——那会把已删除的字段复活，并带进
// `fileName` 这类没在字段定义里出现过的系统列（2026-10-21 真机幽灵字段来源）。
const editFieldDefs = computed(() => {
  const t = editForm.value.contract_type
  const cats = (contractCats.value || []).filter((c: any) => c.enabled !== false)
  const hit = cats.find((c: any) => c.name === t) || cats[0]
  const subs = hit?.subs
  return buildDetailFieldDefs(Array.isArray(subs) ? subs : [], {
    // subs.name 可能是新契约键或重置前的旧键（partyAName/expiryDate…）：
    // contractKeyOf 统一归一到契约键，抽屉顺序 100% 跟随 subs（不重排）
    fieldKeyOf: (n: string) => contractKeyOf(n),
    labelOf: (n: string) => contractFieldLabel(n),
    isRemarkPart: () => false,
  })
})

const onDetailDelete = async () => {
  const r = currentRow.value
  if (!r?.knowledgeId) return
  try {
    const res: any = await getKnowledgeDetails(r.knowledgeId)
    const meta = (res?.data || res)?.custom_metadata || {}
    const contracts: any[] = Array.isArray(meta.contracts) ? [...meta.contracts] : []
    const rest = contracts.filter((c: any, i: number) => i !== (Number(r.page) - 1) && c?.contract_no !== r.contract_no)
    if (rest.length === 0) {
      // 该文件只剩这一份合同 → 级联删除底层源文件，杜绝孤儿文件拦截重传
      await delKnowledgeDetails(r.knowledgeId)
    } else {
      await updateBusinessMetadata(kbId.value, r.knowledgeId, { ...meta, kind: 'contract', contracts: rest }, true)
    }
    MessagePlugin.success('已删除')
    detailVisible.value = false
    loadFiles(true)
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

const onDetailReExtract = async () => {
  const r = currentRow.value
  if (!r?.knowledgeId) return
  try {
    await extractBusinessDocument(kbId.value, r.knowledgeId, 'contract', filterContractType.value || undefined)
    MessagePlugin.success('已提交重新提取')
    detailVisible.value = false
    loadFiles(true)
  } catch (e: any) {
    if (!isExtractInProgress(e)) MessagePlugin.error(e?.message || '重新提取失败')
  }
}

const saveContractDetail = async () => {
  await persistContractDetail(false)
}

// 合同编号重复二次确认：后端 409 + DUPLICATE_CONTRACT_NO 时先询问，确认后 force 重试
const dupConfirmVisible = ref(false)
const dupNo = ref('')
const onDupConfirm = async () => {
  dupConfirmVisible.value = false
  await persistContractDetail(true)
}

const persistContractDetail = async (force: boolean) => {
  const now = currentRow.value
  if (!now) return
  saving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const contracts = Array.isArray(meta.contracts) ? [...meta.contracts] : []
    const taxRateVal = toNumber(editForm.value.tax_rate_display)
    const updated: any = {
      contract_no: editForm.value.contract_no || '',
      contract_name: editForm.value.contract_name || '',
      contract_type: editForm.value.contract_type || '',
      sign_date: editForm.value.sign_date || '',
      effective_date: editForm.value.effective_date || '',
      // 落库键 = 新契约键（11 标准 + 15 扩展；后端旧键 alias 读兼容，不写旧键）
      expire_date: editForm.value.expire_date || '',
      sign_location: editForm.value.sign_location || '',
      party_a: editForm.value.party_a || '',
      party_a_contact: editForm.value.party_a_contact || '',
      party_a_tax_no: editForm.value.party_a_tax_no || '',
      party_a_address: editForm.value.party_a_address || '',
      party_a_phone: editForm.value.party_a_phone || '',
      party_a_bank: editForm.value.party_a_bank || '',
      party_a_account: editForm.value.party_a_account || '',
      party_b: editForm.value.party_b || '',
      party_b_contact: editForm.value.party_b_contact || '',
      party_b_tax_no: editForm.value.party_b_tax_no || '',
      party_b_address: editForm.value.party_b_address || '',
      party_b_phone: editForm.value.party_b_phone || '',
      party_b_bank: editForm.value.party_b_bank || '',
      party_b_account: editForm.value.party_b_account || '',
      amount: toNumber(editForm.value.amount),
      // 税率由百分比字符串转回小数（"3" → 0.03）
      tax_rate: taxRateVal === undefined ? undefined : taxRateVal / 100,
      payment_method: editForm.value.payment_method || '',
      quality_bond: toNumber(editForm.value.quality_bond),
      liquidated_damages: toNumber(editForm.value.liquidated_damages),
      subject: editForm.value.subject || '',
      quantity: toNumber(editForm.value.quantity),
      unit_price: toNumber(editForm.value.unit_price),
      performance_period: editForm.value.performance_period || '',
      handler: editForm.value.handler || '',
      department: editForm.value.department || '',
      remark: editForm.value.remark || '',
      summary: editForm.value.summary || '',
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = contracts.findIndex((ct: any, i: number) =>
        i === nowPage - 1 || Number(ct.page) === nowPage || ct.contract_no === now.contract_no)
    }
    if (targetIdx < 0) targetIdx = contracts.findIndex((ct: any) => ct.contract_no === now.contract_no)
    if (targetIdx >= 0) contracts[targetIdx] = { ...contracts[targetIdx], ...updated, page: nowPage || contracts[targetIdx]?.page }
    else contracts.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const hasContractData = contracts.some((c: any) =>
      (c.contract_no || '').trim() || (c.contract_name || '').trim() || (c.contract_type || '').trim() ||
      (c.party_a || '').trim() || (c.party_b || '').trim() || Number(c.amount) > 0)
    const nextStatus = hasContractData ? 'success' : 'manual'
    const nextError = hasContractData ? '' : '人工入库待编辑，请补充字段'
    const newMeta = { ...meta, kind: 'contract', contracts, extract_status: nextStatus, extract_error: nextError }
    // 合同 contracts 为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由（与发票同款）
    await updateBusinessMetadata(kbId.value, now.knowledgeId, newMeta, force)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑后列表立即刷新（shallowRef：整体替换数组触发更新）
    const listIdx = contractRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      const nextRow: any = {
        ...contractRows.value[listIdx],
        extractStatus: nextStatus,
        extractError: nextError,
        contract_no: updated.contract_no,
        contract_name: updated.contract_name,
        contract_type: updated.contract_type,
        sign_date: updated.sign_date,
        effective_date: updated.effective_date,
        expire_date: updated.expire_date,
        sign_location: updated.sign_location,
        party_a: updated.party_a,
        party_b: updated.party_b,
        amount: updated.amount,
        tax_rate: updated.tax_rate,
        payment_method: updated.payment_method,
        summary: updated.summary,
        performance_status: calcFulfillStatus(updated.expire_date),
        handler: updated.handler,
        department: updated.department,
        remark: updated.remark,
      }
      contractRows.value = contractRows.value.map((r: any, i: number) => (i === listIdx ? nextRow : r))
    }
    MessagePlugin.success('保存成功')
    detailVisible.value = false
    // 保存后并行刷新列表与概览卡片（对齐发票基准：合同类型/金额/总数随最新数据同步更新）
    loadFiles(true)
    loadContractOverview()
  } catch (e: any) {
    if (e?.status === 409 && String(e?.message || '').startsWith('DUPLICATE_CONTRACT_NO')) {
      // 合同编号重复：弹二次确认，确认后 force 重试；取消留在编辑态
      dupNo.value = String(e?.message || '').split(':').pop() || ''
      dupConfirmVisible.value = true
      return
    }
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const toNumber = (v: any): number | undefined => {
  if (v === '' || v === null || v === undefined) return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 数字输入辅助：只允许数字与小数点，按实际输入显示（不补零、不保留多余小数）
const numToStr = (v: any): string =>
  v === null || v === undefined || v === '' ? '' : String(v)

// 金额一律不显示货币符号（全系统统一规则，与发票/车队基准一致）
const formatAmount = (v?: number) =>
  v === undefined || v === null ? '' : Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

// 税率以百分比显示且不保留小数（0.03 → 3%）
const formatRate = (v?: number) => {
  if (v === undefined || v === null) return ''
  return `${Math.round(Number(v) * 100)}%`
}

// ---- 浮动工具栏 ----
const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

// ---- 浮动工具栏 ----
// 批量解析/提取的并发上限：后端提取为串行模型调用（内部排队），前端无需无上限并发，
// 3 并发足够掩盖请求排队抖动，又不至于把后端队列一次塞满。
const BATCH_CONCURRENCY = 3

// 轻量并发池：以 limit 上限消费 items，返回与入参同序的 allSettled 结果；
// 单个任务失败不中断整体，语义与 Promise.allSettled 一致（避免引入第三方库）。
async function settleInBatches<T>(items: readonly T[], limit: number, fn: (item: T) => Promise<unknown>): Promise<PromiseSettledResult<unknown>[]> {
  const results: PromiseSettledResult<unknown>[] = new Array(items.length)
  let cursor = 0
  const worker = async () => {
    while (cursor < items.length) {
      const i = cursor++
      results[i] = await Promise.resolve().then(() => fn(items[i])).then(
        (value) => ({ status: 'fulfilled' as const, value }),
        (reason) => ({ status: 'rejected' as const, reason }),
      )
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, () => worker()))
  return results
}

// 批量确认文案：单选显示文件名，多选显示文件数（selectedIds 已按 knowledgeId 去重）
// 注意：多选 N 条记录若同属一个文件，selectedIds.length 为 1，此时取任一选中记录的文件名展示
const reparseConfirmText = computed(() => {
  const ids = selectedIds.value
  if (ids.length === 1) {
    const name = selectedRows.value[0]?.fileName || selectedSingle.value?.fileName || '该文件'
    return `确定重新解析「${name}」的物理文件并重新拆页吗？将覆盖已有解析结果。`
  }
  return `确定重新解析选中的 ${ids.length} 个文件并重新拆页吗？将覆盖已有解析结果。`
})
const extractConfirmText = computed(() => {
  const ids = selectedIds.value
  if (ids.length === 1) {
    const name = selectedRows.value[0]?.fileName || selectedSingle.value?.fileName || '该文件'
    return `确定按最新提取规则重新提取「${name}」的所有合同字段吗？将覆盖已有提取结果。`
  }
  return `确定按最新提取规则重新提取选中的 ${ids.length} 个文件的所有合同字段吗？将覆盖已有提取结果。`
})

// 重新解析：物理文件重新拆页/解析（支持多文件批量，二次确认）
const reparseBusy = ref(false)
const handleReparse = async () => {
  const ids = selectedIds.value
  if (!ids.length) { MessagePlugin.info('请先选中要重新解析的合同'); return }
  reparseBusy.value = true
  // 过程态交给上方状态条实时展示（正在解析「文件名」进度 N%），
  // 这里清空上传弹窗残留进度，确保 displayProgress 走轮询派生的 liveProgress
  uploadProgress.value = []
  let ok = 0
  let fail = 0
  try {
    // selectedIds 已按 knowledgeId 去重：多选 N 条记录若同属一个文件，只对该文件重新解析一次
    const results = await settleInBatches(ids, BATCH_CONCURRENCY, kid => reparseKnowledge(kid))
    results.forEach(r => r.status === 'fulfilled' ? ok++ : fail++)
    if (fail === 0) {
      MessagePlugin.success(`已提交 ${ok} 个文件重新解析`)
    } else {
      MessagePlugin.warning(`${ok} 个文件重新解析已提交，${fail} 个提交失败`)
    }
    // 提交后立即扫描一次：重解走异步队列，parse_status 很快进入 pending，
    // 主动 scanOnce 缩短状态条出现的等待窗口（不必等下一轮 3s 轮询）
    scanOnce()
    loadFiles(true)
    loadContractOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    reparseBusy.value = false
  }
}

// 重新提取：大模型/OCR 按最新提取规则重新提取选中文件全部合同字段（支持多文件批量，二次确认）
const extractBusy = ref(false)
const handleFileExtract = async () => {
  const all = selectedIds.value
  if (!all.length) { MessagePlugin.info('请先选中要重新提取的合同'); return }
  // 跳过已在提取中的文件（轮询自动提取或前一次手动提交仍在途）：
  // 后端 extractMu 按文件防重入，重复提交会 409；前端 in-flight 先挡一轮
  const ids = all.filter(kid => !extractInFlight.value.has(kid))
  const skipped = all.length - ids.length
  if (!ids.length) {
    MessagePlugin.info('所选文件均正在提取中，请稍候')
    return
  }
  ids.forEach(kid => { extractInFlight.value.add(kid); extractFailed.value.delete(kid) })
  extractBusy.value = true
  // 过程态交给上方状态条实时展示（正在提取「文件名」进度 N%）：
  // 后端提取请求在途时已写 extract_status=processing，轮询即可捕获；
  // 清空上传弹窗残留进度，确保 displayProgress 走轮询派生的 liveProgress
  uploadProgress.value = []
  let results: PromiseSettledResult<unknown>[] = []
  try {
    // selectedIds 已按 knowledgeId 去重：多选 N 条记录若同属一个文件，只对该文件重新提取一次。
    results = await settleInBatches(
      ids,
      BATCH_CONCURRENCY,
      kid => extractBusinessDocument(kbId.value, kid, 'contract', filterContractType.value || undefined),
    )
    let ok = 0
    let conflict = 0
    results.forEach((r, i) => {
      if (r.status === 'fulfilled') { ok++; return }
      // 409 冲突：后端 extractMu 判定该文件正被提取（另一标签/请求在途），
      // 属重复提交而非提取失败——不写入 extractFailed，避免状态列误报「提取失败」。
      // request.ts 非 2xx JSON 响应统一带 $httpStatus（非枚举属性）。
      if (isExtractInProgress(r.reason)) { conflict++; return }
      extractFailed.value.set(ids[i], Date.now())
    })
    const fail = results.length - ok - conflict
    if (fail === 0 && conflict === 0) {
      MessagePlugin.success(`已提交 ${ok} 个文件重新提取`)
    } else {
      const parts: string[] = []
      if (ok) parts.push(`${ok} 个已提交`)
      if (conflict) parts.push(`${conflict} 个正在提取中`)
      if (fail) parts.push(`${fail} 个提交失败`)
      if (skipped) parts.push(`${skipped} 个已在提取中，未重复提交`)
      MessagePlugin.warning(parts.join('，'))
    }
    // 提交后立即扫描一次：提取在途文件 extract_status=processing 已被后端写入，
    // 主动 scanOnce 让状态条立即显示「正在提取」，不必等下一轮 3s 轮询
    scanOnce()
    loadFiles(true)
    loadContractOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    // asynq 化后 POST 200 仅代表入队成功，不代表提取完成：
    // 成功提交/409 的文件保留 in-flight，由 useBusinessPolling 轮询在 es 到
    // 终态（success/failed）时统一回收——若此处提前删除，下一轮 3s 轮询
    // 看到 es=processing 且不在 in-flight 会重复提交 → 后端 extractMu 409。
    // 仅真实失败（非 409）删除 in-flight，避免文件因 in-flight 残留永久卡死。
    results.forEach((r, i) => {
      if (r.status === 'rejected' && !isExtractInProgress(r.reason)) {
        extractInFlight.value.delete(ids[i])
      }
    })
    extractBusy.value = false
  }
}

// 打印：合同一个文件一般只有一份合同，打印预览整个文档（含多页），
// 支持跨文件任意组合合并为一个 PDF，单 iframe 预览打印。
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff']
const isImageRow = (r: any, blob: any) => {
  const ext = String(r.fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const handleBatchPrint = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  printBusy.value = true
  printUrl.value = ''
  printLoaded.value = false
  printCount.value = rows.length
  printVisible.value = true
  try {
    const out = await PDFDocument.create()
    const notes: string[] = []
    let mergedPages = 0
    for (const r of rows) {
      const blob: any = await previewKnowledgeFile(r.knowledgeId)
      if (!blob) { notes.push(`${r.contract_no || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式合同：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        // 合同打印整个源文件（一份合同可能多页，如盖章扫描件）
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pageCount = pdf.getPageCount()
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
        if (pageCount > 1) notes.push(`${r.contract_no || r.fileName}（共 ${pageCount} 页）`)
      } catch {
        notes.push(`${r.contract_no || r.fileName}（页抽取失败，已并入整文件）`)
        try {
          const src = await blob.arrayBuffer()
          const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
          const pages = await out.copyPages(pdf, pdf.getPageIndices())
          pages.forEach(p => out.addPage(p))
          mergedPages += pages.length
        } catch { /* 整文件也失败则跳过 */ }
      }
    }
    if (mergedPages === 0) {
      MessagePlugin.error('未获取到可打印的合同页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    if (notes.length) {
      setTimeout(() => MessagePlugin.warning(notes.join('；')), 300)
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
    printVisible.value = false
  } finally {
    printBusy.value = false
  }
}

// 打印：调用预览 iframe 内的 PDF 查看器打印（打印完整合并 PDF，
// 无水印、无裁剪、含全部选中页面）
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement | null
  if (frame?.contentWindow) {
    try { frame.contentWindow.print(); return } catch { /* fallback */ }
  }
  window.print()
}

// 目录生成：把选中记录按「当前列表展示字段」生成表格式清单 PDF（A4 横/纵自适应），
// 复用打印弹窗预览，支持打印与下载
const catalogValueOf = (row: ContractRow, key: string): string => {
  switch (key) {
    case 'contract_no': return row.contract_no || ''
    case 'contract_name': return row.contract_name || ''
    case 'contract_type': return row.contract_type || ''
    case 'party_a': return row.party_a || ''
    case 'party_b': return row.party_b || ''
    case 'sign_date': return row.sign_date || ''
    case 'expire_date': return row.expire_date || ''
    case 'amount': return formatAmount(row.amount)
    case 'tax_rate': return formatRate(row.tax_rate)
    case 'extract_status': { const s = statusOf(row).label; return s === '--' ? '' : s }
    case 'tags': return rowTags(row).map((t: any) => t.name).join('、')
    case 'performance_status': return row.performance_status || ''
    case 'party_a_tax_no': return row.party_a_tax_no || ''
    case 'party_b_tax_no': return row.party_b_tax_no || ''
    case 'effective_date': return row.effective_date || ''
    case 'payment_method': return row.payment_method || ''
    case 'handler': return row.handler || ''
    case 'department': return row.department || ''
    case 'fileName': return row.fileName || ''
    default: return String((row as any)[key] ?? '')
  }
}
const handleBatchCatalog = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = [
      ...visibleColDefs.value.map((c: any) => ({ key: c.key, label: c.label || contractFieldLabel(c.key), value: (r: any) => catalogValueOf(r, c.key) })),
    ]
    const bytes = await generateCatalogPdf({ title: '合同目录', columns: cols, rows })
    printMode.value = 'catalog'
    printCount.value = rows.length
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const downloadCatalogPdf = () => {
  if (!printUrl.value) return
  const a = document.createElement('a')
  a.href = printUrl.value
  a.download = `合同目录-${Date.now()}.pdf`
  a.click()
}

// ---- 删除（黄金基准发票同款：单气泡内嵌二选一，杜绝二次弹窗） ----
const delPopVisible = ref(false)
const closeDelPop = () => { delPopVisible.value = false }
// 选中记录涉及的多页文件数量（同一上传文件含多份合同）
// delMultiCount 已删除：合同不需要按页操作
// 删除编排统一走中台 useBusinessListSync（与发票同款）：乐观移除 + 延迟对账 +
// 同文件多行「先大页码后小页码」级联顺序。本页只注入自己的刷新与删除接口，
// 不再各自维护定时器数组（此前三页各写一份，行为易漂移）。
// 合同不支持按页删除：删除粒度恒为「整份上传文件」。
const listSync = useBusinessListSync(contractRows, {
  reload: (silent: boolean | undefined) => loadFiles(!!silent),
  reloadStats: () => { loadContractOverview(); loadUploadStats() },
  deleteFiles: (ids) => batchDeleteKnowledge(kbId.value, ids),
  onError: (e) => MessagePlugin.error(e?.message || '删除失败'),
})
const optimisticRemoveContractRows = (fileIds: Set<string>, pageKeys: Set<string>) =>
  listSync.removeRows(fileIds, pageKeys)
const schedulePostDeleteRefresh = () => listSync.schedulePostDeleteRefresh()
const doFileDelete = (rows: any[]) => listSync.deleteFiles(rows)
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  clearSelectionSafe()
  // 异步删除：不立即 loadFiles（会把乐观移除的行又拉回来），交由延迟对账权威纠正
  schedulePostDeleteRefresh()
}
// 合同不需要按页操作，直接走文件删除
const confirmFileDelete = async () => {
  closeDelPop()
  try { await doFileDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---- 生命周期 ----
// 六同步：字段/分类配置变更后重建动态列 + 重载分类（KPI 类型卡/工具栏下拉/编辑抽屉类型下拉/
// 上传弹窗分类六处同源）+ 刷新记录（KPI 计数来自列表数据），与发票基准一致
const onCategoriesChanged = () => {
  reloadColumns(true)
  loadContractCats()
  loadFiles(true)
  // 分类配置变更（改名/启停用）影响概览类型分桶，同步刷新卡片统计
  loadContractOverview()
}
// 从后端 categories（scope=contract）重建动态列；无配置时回退内置默认。
// 列按「当前选中分类」的字段配置生成（六同步：列表/字段筛选器/编辑抽屉/打印同源），
// 多分类下切换类型时列表列跟随当前分类（不再假设单一「合同」分类）。
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    // 分类缓存：切换类型时直接用已拉取的分类重建列（省一次请求、切换更快）；
    // 仅首次进入或设置保存/六同步事件（force=true）时重新请求后端
    let cats: any[] = force ? [] : contractCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'contract' })
      cats = res?.data || []
      contractCats.value = cats
    }
    const curCat = cats.find((c: any) => c.name === filterContractType.value && c.enabled !== false)
    const baseCat = curCat || cats[0]
    if (baseCat?.subs?.length) {
      // 六同步：主列表表头 / 字段筛选器 / 打印目录三处共用同一份列定义。
      // label 以 subs.label（用户改名）为权威，静态 DEFAULT_CONTRACT_COLUMNS 只提供
      // 宽度缺省——否则把「摘要」改成「摘要2」后表头仍显示「摘要」。
      customColumns.value = appendListSystemColumns(
        buildColumnsFromSubs(baseCat.subs, {
          labelOf: (n: string) => contractFieldLabel(n),
          defaultColumns: DEFAULT_CONTRACT_COLUMNS as any,
        }),
        DEFAULT_CONTRACT_COLUMNS as any,
        (n: string) => contractFieldLabel(n),
      ) as any
      // 六同步：切换分类或设置保存后，按当前分类默认表头重建字段筛选器勾选。
      // 首轮加载（lastColType 为空）时保留用户既有 localStorage 存档，但若存档缺当前分类
      // 的默认列则补齐当前分类默认列（发票基准同款门控）
      if (!lastColType.value) {
        const defaults = customColumns.value.filter((c: any) => c.default).map((c: any) => c.key)
        if (defaults.some((k: string) => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterContractType.value) {
        resetColumns()
      }
      lastColType.value = filterContractType.value
    }
    // 类型筛选锁定：分类重载后若当前选中项失效（被禁用/改名），回退第一个启用分类
    ensureContractType()
  } catch { /* 后端未配置时回退内置默认 */ }
}
onMounted(() => {
  loadKb()
  loadContractCats()
  reloadColumns()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  listSync.destroy()
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})
</script>

<style scoped lang="less">
.contract-management-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 24px 32px;
  overflow: hidden;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;

  .header-title {
    h2 { font-size: 20px; font-weight: 600; color: var(--td-text-color-primary); margin: 0 0 6px 0; }
    .header-subtitle { font-size: 14px; color: var(--td-text-color-secondary); margin: 0; }
  }
}

.loading-area, .empty-area {
  flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 16px; min-height: 300px;
}

.contract-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

/* ---- 工具栏 #type-extra 插槽内的履约状态/日期控件 ---- */
.doc-filter-field {
  display: flex; align-items: center;
  &--wide { min-width: 260px; }
  .doc-type-select { width: 130px; }
  .doc-date-range { width: 260px; }
}

/* ---- 字段筛选弹层 ---- */
:global(.contract-field-popup) {
  padding: 0 !important;
}
.field-popup-content {
  width: 240px;
  padding: 12px;
  box-sizing: border-box;
  .field-popup-head {
    display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px;
    .field-popup-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
    .field-popup-actions { display: flex; gap: 0; }
  }
  .field-popup-list {
    display: flex; flex-direction: column; gap: 6px; max-height: 320px; overflow-y: auto;
    .field-popup-item { display: flex; align-items: center; }
  }
}

/* ---- 列表 ---- */
@keyframes doc-list-spin { to { transform: rotate(360deg); } }

/* scrollbar-gutter:stable 保证滚动条占位宽度恒定，列与滚动条不错位 */
.doc-list-header, .doc-list-row {
  display: grid;
  align-items: center;
  padding: 0 16px;
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
  .cell { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
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
  &:last-child { border-bottom: 0; }
  &:hover:not(.selected) { background: var(--td-bg-color-secondarycontainer); }
  &.selected { background: var(--td-brand-color-1); }
}

.cell {
  display: flex;
  align-items: center;
  justify-content: center; /* 表头/内容居中 */
  min-width: 0;
  padding: 0 8px;
  text-align: center;
}

.cell-check {
  justify-content: center;
  position: sticky;
  left: 0;
  z-index: 2;
  background: transparent; /* 跟随行/容器背景，避免白色块 */
  padding: 0;
}

.cell-contract_no, .cell-contract_name, .cell-party_a, .cell-party_b, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-contract-no {
  min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font-size: 13px; font-weight: 600; color: var(--td-text-color-primary);
}
.row-page-badge {
  flex-shrink: 0; margin-left: 6px; height: 18px; padding: 0 6px; border-radius: 999px;
  background: var(--td-success-color-light); color: var(--td-success-color); font-size: 11px; line-height: 18px;
  white-space: nowrap;
}
.row-text { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-mono { font-variant-numeric: tabular-nums; font-size: 12px; color: var(--td-text-color-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.row-amount { font-size: 13px; color: var(--td-text-color-primary); font-weight: 500; }
.row-rate { font-size: 12px; color: var(--td-text-color-primary); }
.row-muted { color: var(--td-text-color-disabled); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 提取成功 / 提取失败 / 待提取 三种状态标签必须完全同尺寸（与发票页同一份规则）：
   t-tag 的 small 尺寸在带内联图标（close-circle / loading）时会按内容收缩，
   导致「提取失败」比「提取成功」更小。这里用固定盒模型锁死。 */
.row-status-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 22px;
  line-height: 20px;
  padding: 0 8px;
  border-radius: 4px;
  font-size: 12px;
  box-sizing: border-box;
  white-space: nowrap;
  :deep(.t-icon) { margin-right: 0; font-size: 12px; }
}
.icon-spin { animation: doc-list-spin 0.9s linear infinite; }

.row-tag-chips {
  display: inline-flex; align-items: center; gap: 4px; flex-wrap: nowrap; cursor: pointer;
}
.row-tag-more {
  display: inline-flex; align-items: center; justify-content: center; height: 20px; min-width: 20px; padding: 0 4px;
  border-radius: 999px; border: 1px solid var(--td-component-stroke); color: var(--td-text-color-secondary); font-size: 10px;
}
.row-tag-add {
  font-size: 11px; color: var(--td-text-color-placeholder); border: 1px dashed var(--td-component-stroke);
  border-radius: 999px; padding: 0 6px; height: 20px; display: inline-flex; align-items: center; white-space: nowrap;
  &:hover { border-color: var(--td-brand-color); color: var(--td-brand-color); border-style: solid; }
}

.doc-load-more { display: flex; justify-content: center; padding: 12px 0; }

/* ---- 详情抽屉：竖向区块 + 可拖宽（复用知识库文档抽屉上下滚动样式） ---- */
.contract-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.contract-detail-body {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.detail-block { width: 100%; }
.detail-block-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 12px;
  &::before { content: ''; width: 3px; height: 14px; background: var(--td-brand-color); border-radius: 2px; flex-shrink: 0; }
}
.detail-block-content { width: 100%; }
.detail-preview-collapse { margin-top: 12px; }

// 摘要（复用知识库文档抽屉样式：线框 + 展开/折叠 + 每条字段一行）
.summary_wrapper {
  position: relative;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  &.summary_clickable { cursor: pointer; }
}
.summary_content {
  padding: 12px;
  color: var(--td-text-color-primary);
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
  white-space: pre-wrap;
  &.summary_collapsed { max-height: 4.5em; overflow: hidden; }
}
.summary_fade {
  display: flex;
  justify-content: center;
  padding-bottom: 4px;
  pointer-events: none;
  &:not(.summary_fade_expanded) {
    position: absolute;
    bottom: 0; left: 0; right: 0;
    height: 28px;
    background: linear-gradient(transparent, var(--td-bg-color-container) 80%);
    border-radius: 0 0 6px 6px;
    align-items: flex-end;
  }
}
.summary_fade_icon { color: var(--td-text-color-placeholder); }

.detail-fields { padding-bottom: 16px; }

.field-group { margin-bottom: 18px; }
.field-group-title {
  display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600;
  color: var(--td-text-color-primary); margin-bottom: 12px;
  &::before { content: ''; width: 3px; height: 14px; background: var(--td-brand-color); border-radius: 2px; flex-shrink: 0; }
}
.field-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); column-gap: 20px; row-gap: 12px; }
.field-grid--full { grid-template-columns: 1fr; }
.field-grid :deep(.t-form__item) { margin-bottom: 0; }

.detail-save-hint { display: flex; align-items: center; gap: 6px; margin-top: 16px; font-size: 12px; color: var(--td-text-color-placeholder); }

/* ---- 删除气泡：内嵌二选一（黄金基准发票同款） ---- */
.del-pop-body {
  min-width: 220px;
  .del-pop-title { font-size: 13px; color: var(--td-text-color-primary); line-height: 1.5; margin-bottom: 12px; }
  .del-pop-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; }
}

/* ---- 详情抽屉底部操作：手动保存/取消 ---- */
.detail-footer-actions {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 8px;
}

</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.contract-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: var(--wk-batch-bar-z);
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.contract-print-dialog {
  width: min(900px, calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  height: 82vh;
  max-height: 92vh;
  background: var(--td-bg-color-container);
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.contract-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: var(--td-bg-color-container);
}
.contract-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.contract-print-close {
  flex-shrink: 0;
}
.contract-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.contract-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.contract-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.contract-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}

/* 列内边距收紧、批量浮条、底栏、列表容器：统一走四模块共享样式文件（见文件顶部全局 style 块） */
</style>
