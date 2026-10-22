<template>
  <div class="invoice-management-container biz-page">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>发票管理</h2>
        <p class="header-subtitle">发票档案自动归档</p>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化发票管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-发票」知识库">
        <template #image><t-icon name="file" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建发票知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="invoice-main biz-main">
      <!-- 概览紧凑卡（中台 DashboardKpiGroup 组件，type-card 范式） -->
      <div class="overview-group">
        <DashboardKpiGroup :loading="!cardsReady" :cards="overviewCards" :active-type="filterInvoiceType" @card-click="onOverviewCardClick" />
      </div>

      <!-- 发票列表 -->
      <div class="invoice-list-view biz-list-view">
      <!-- 筛选工具栏（对齐车队标准中台组件） -->
          <!-- 筛选工具栏走中台 BusinessFilterBar：4 个筛选器改为 FilterFieldSpec 声明 -->
          <BusinessFilterBar v-model="filterState" :fields="filterFields" search-placeholder="搜索全部字段"
            :columns="effectiveColumns" :visible-keys="visibleColKeys" :selected-count="selectedRowKeys.length"
            settings-text="设置" :primary-action="{ text: '上传发票', icon: 'upload' }" hide-batch-bar
            @columns-change="onVisibleColKeysChange" @reset-columns="resetColumns"
            @select-all-columns="selectAllColumns" @filter-change="applyFilter"
            @settings="invoiceSettingsVisible = true" @primary="uploadVisible = true"
            @clear-selection="clearSelection" />

      <!-- 进行中文件状态横幅（证照档案同款：优先上传实时进度轮播，关闭弹窗后由轮询派生解析/提取进度，多文件滚动展示） -->
      <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

      <!-- 发票列表（自绘 grid，可横向滚动，字段可配置；虚拟滚动：>100 行自动仅渲染视口内 DOM） -->
      <div class="doc-list-scroll" ref="listScrollRef">
        <div class="doc-list-view" :style="{ '--invoice-sbw': headerFix.sbw + 'px', '--invoice-thh': headerFix.thh + 'px' }">
          <t-table
        ref="invoiceTableRef"
        :data="filteredRows"
        :columns="tableColumns"
        row-key="rowKey"
        size="small"
        :hover="true"
        :loading="listLoading"
        :max-height="tableMaxHeight"
        sticky-header
        class="doc-table"
        :selected-row-keys="selectedRowKeys"
        :sort="sortState"
        :row-class-name="({ row }: any) => selectedRowKeys.includes(row.rowKey) ? 'is-selected' : ''"
        :scroll="tableScroll"
        @scroll="onListScroll"
        @row-click="({ row, e }: any) => onRowClick(row, e)"
        @select-change="onTableSelectChange"
        @sort-change="onSortChange"
      >
        <template #invoiceNo="{ row }: any">
          <span v-if="row.isPlaceholder" class="row-placeholder-badge" :title="row.placeholderReason">
            <t-tag size="small" theme="warning" variant="light-outline">{{ row.placeholderReason || '未提取出票据' }}</t-tag>
          </span>
          <span v-else class="row-invoice-no" :title="row.invoiceNo || row.fileName">{{ row.invoiceNo || row.fileName }}</span>
          <span v-if="pageBadgeOf(row)" class="row-page-badge">{{ pageBadgeOf(row) }}</span>
        </template>
        <template #invoiceDate="{ row }: any">
          <span class="row-mono">{{ row.invoiceDate }}</span>
        </template>
        <template #invoiceType="{ row }: any">
          <span class="row-text" :title="row.invoiceType">{{ row.invoiceType }}</span>
        </template>
        <template #amount="{ row }: any">
          <span class="row-mono row-amount">{{ formatAmount(row.amount) }}</span>
        </template>
        <template #taxRate="{ row }: any">
          <!-- 明细浮层走中台组件 BusinessSubItemsPopover（宽度自适应与取值兜底同源码搬入） -->
          <BusinessSubItemsPopover :items="row.items" :sub-fields="enabledSubFieldsOfRow(row)"
            empty-text="暂无项目明细">
            <span class="row-mono row-rate">{{ rateTextOf(row) || '-' }}</span>
          </BusinessSubItemsPopover>
        </template>
        <template #tax="{ row }: any">
          <span class="row-mono">{{ formatAmount(row.tax) }}</span>
        </template>
        <template #totalAmount="{ row }: any">
          <span class="row-mono row-amount">{{ formatAmount(row.totalAmount) }}</span>
        </template>
        <template #buyerName="{ row }: any">
          <span class="row-text" :title="row.buyerName">{{ row.buyerName }}</span>
        </template>
        <template #sellerName="{ row }: any">
          <span class="row-text" :title="row.sellerName">{{ row.sellerName }}</span>
        </template>
        <template #issuer="{ row }: any">
          <span class="row-text">{{ row.issuer }}</span>
        </template>
        <template #remark="{ row }: any">
          <span class="row-text" :title="row.remark">{{ row.remark }}</span>
        </template>
        <template #extractStatus="{ row }: any">
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
        <template #sellerTaxNo="{ row }: any">
          <span class="row-mono" :title="row.sellerTaxNo">{{ row.sellerTaxNo }}</span>
        </template>
        <template #buyerTaxNo="{ row }: any">
          <span class="row-mono" :title="row.buyerTaxNo">{{ row.buyerTaxNo }}</span>
        </template>
        <template #items="{ row }: any">
          <t-tooltip v-if="(row.items || []).length" :content="itemsTextOf(row)" placement="top">
            <div class="row-items">
              <div v-for="(it, i) in (row.items || []).slice(0, 1)" :key="i" class="row-items-line">
                <span class="row-items-name">{{ it.name || '—' }}</span>
                <template v-if="it.qty !== undefined && it.qty !== null && it.qty !== ''"><span class="row-items-qty">×{{ it.qty }}</span></template>
                <template v-if="it.price !== undefined && it.price !== null && it.price !== ''"><span class="row-items-price">{{ formatAmount(it.price) }}</span></template>
              </div>
              <div v-if="(row.items || []).length > 3" class="row-items-more">等 {{ (row.items || []).length - 3 }} 条</div>
            </div>
          </t-tooltip>
          <span v-else class="row-muted">—</span>
        </template>
        <template #fileName="{ row }: any">
          <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
        </template>
      </t-table>
    </div>
      <!-- 底部汇总（列表容器内部底部固定，不随表格滚动；选中时避让底部浮动工具栏） -->
      <div class="doc-summary-bar">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 条</span>
        <span v-if="displaySummary.total && summaryShowAmount" class="doc-summary-item">
          金额 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumAmount) }}</span>
        </span>
        <span v-if="displaySummary.total && summaryShowTax" class="doc-summary-item">
          税额 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTax) }}</span>
        </span>
        <span v-if="displaySummary.total && summaryShowTotal" class="doc-summary-item">
          价税合计 <span class="doc-summary-val">{{ formatAmount(displaySummary.sumTotal) }}</span>
        </span>
      </div>
      <!-- 底部悬浮工具条：fixed 脱离文档流，出现/消失不占物理空间；选中行时显示 -->
        <div v-if="selectedRowKeys.length" class="doc-batch-bar" role="region">
          <div class="batch-bar-inner">
            <div class="batch-bar-left">
              <span class="batch-bar-count">已选 {{ selectedRowKeys.length }} 项</span>
              <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelectionSafe">
                清除
              </t-button>
            </div>
            <div class="batch-bar-actions">
              <!-- 重新解析：物理文件重新拆页/解析（支持多文件批量，二次确认） -->
              <t-popconfirm theme="warning"
                :content="reparseConfirmText"
                :confirm-btn="{ content: '重新解析', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleReparse">
                <t-button theme="default" variant="outline" size="small" :loading="reparseBusy" :disabled="!selectedIds.length" @click.stop>
                  <template #icon><t-icon name="refresh" size="14px" /></template>
                  重新解析
                </t-button>
              </t-popconfirm>
              <!-- 重新提取：大模型/OCR 按最新提取规则重新提取全部字段（支持多文件批量，二次确认） -->
              <t-popconfirm theme="warning"
                :content="extractConfirmText"
                :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleFileExtract">
                <t-button theme="default" variant="outline" size="small" :loading="extractBusy" :disabled="!selectedIds.length" @click.stop>
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
              <!-- 删除确认：单气泡内嵌二选一（多页文件时「删除此页/删除文件」，否则普通确认），杜绝二次弹窗 -->
              <t-popconfirm theme="warning" v-model:visible="delPopVisible" placement="top"
                :confirm-btn="null" :cancel-btn="null" :popup-props="{ overlayStyle: { width: 'auto' } }">
                <template #content>
                  <div class="del-pop-body">
                    <div v-if="delMultiCount > 0" class="del-pop-title">
                      选中的发票涉及 {{ delMultiCount }} 份多页文件，请选择删除范围：
                    </div>
                    <div v-else class="del-pop-title">确定删除所选 {{ selectedRowKeys.length }} 条发票记录吗？删除后不可恢复。</div>
                    <div class="del-pop-actions">
                      <template v-if="delMultiCount > 0">
                        <t-button size="small" theme="danger" @click="confirmPageDelete">删除此页</t-button>
                        <t-button size="small" theme="danger" variant="outline" @click="confirmFileDelete">删除文件</t-button>
                        <t-button size="small" variant="text" @click="closeDelPop">取消</t-button>
                      </template>
                      <template v-else>
                        <t-button size="small" theme="danger" @click="confirmPageDelete">删除</t-button>
                        <t-button size="small" variant="text" @click="closeDelPop">取消</t-button>
                      </template>
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

    </div>
    </div>
    <!-- 创建知识库向导 -->
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-发票" kb-desc="发票管理固定使用专用知识库，名称不可修改" description-placeholder="用于存放并解析电子发票，自动提取发票字段" default-description="用于存放并解析电子发票，自动提取发票字段" model-tip="提取模型将复用下方「对话模型」，用于解析发票字段。若列表为空，请先在系统设置中添加模型。" @created="onKbCreated" />

    <!-- 已上传文件（复用车队：全部上传文件的解析/提取状态；待复核卡点击打开） -->
    <FleetUploadHistoryDrawer v-model:visible="uploadHistoryVisible" :kb-id="kbId || ''" scope="invoice"
      @changed="onUploadHistoryChanged" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab） -->
    <StandardSettingDrawer v-model:visible="invoiceSettingsVisible" :kb-id="kbId || ''" @saved="() => reloadColumns(true)" />

    <!-- 上传弹窗（对齐车队：分类选择 + 拖拽/选择/粘贴 + 进度） -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="invoice"
      :type-options="uploadTypeOptions" :default-type="filterInvoiceType || ''" @done="onUploadDone"
      @progress="onUploadProgress" />

    <!-- 发票详情抽屉：中台 BusinessSplitDetailDrawer（左预览 / 右表单 + 质量溯源） -->
    <BusinessSplitDetailDrawer v-model:visible="detailVisible" :title="detailTitle" :row="currentRow"
      :page-no="pdfPageOf(currentRow)" :form="editForm" :fields="editFieldDefs" :sub-fields="itemSubFields"
      supports-sub-fields item-label="项目明细" :block-title="editForm.invoice_type || '发票信息'"
      :quality="detailQuality" :label-of="qualityLabel" :dirty="detailDirty" :saving="autoSaving"
      :storage-key="'weknora-invoice-drawer-width'" @save="saveEditForm" @delete="onDetailDelete"
      @re-extract="onDetailReExtract">
      <!-- 业务特有控件：类型下拉（改类型会联动归档分类，必须走页面自己的 options） -->
      <template #field-invoiceType>
        <t-select v-model="editForm.invoice_type" :options="invoiceTypeOptions" clearable allow-create
          filterable placeholder="选择或输入类型" />
      </template>
      <template #field-extractStatus>
        <t-tag v-if="statusOf(editForm.data).label !== '--'" size="small" :theme="statusOf(editForm.data).theme"
          variant="light-outline" class="row-status-tag">
          <template v-if="statusOf(editForm.data).icon" #icon>
            <t-icon :name="statusOf(editForm.data).icon!" :class="{ 'icon-spin': statusOf(editForm.data).spin }" />
          </template>
          {{ statusOf(editForm.data).label }}
        </t-tag>
        <span v-else class="row-muted">--</span>
      </template>
      <template #field-tags>
        <div v-if="String(editForm.data.tags || '').trim()" class="detail-tags-readonly">
          <t-tag v-for="t in String(editForm.data.tags).split('\n').map((x: string) => x.trim()).filter(Boolean)" :key="t"
            size="small" variant="light-outline" class="row-tag">
            {{ t }}
          </t-tag>
        </div>
        <span v-else class="row-muted">—</span>
      </template>
      <!-- 税率：与主表单/明细表统一为百分比展示（输入 9 即 9%），加 % 单位提示避免割裂感 -->
      <template #field-taxRate>
        <t-input :value="editForm.data.taxRate" placeholder="如 9 表示 9%"
          @input="(v: string) => (editForm.data.taxRate = sanitizeNum(v))">
          <template #suffix>%</template>
        </t-input>
      </template>
    </BusinessSplitDetailDrawer>

    <!-- 单号重复二次确认：强制保存前询问（确认后带 force 重试，取消留在编辑态） -->
    <t-dialog v-model:visible="dupConfirmVisible" header="发票号码重复" width="420" @confirm="onDupConfirm">
      <span>系统已存在相同的发票号码「{{ dupNo }}」，是否强制保存？</span>
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
      <div v-if="printVisible" class="invoice-print-mask">
        <div class="invoice-print-dialog">
          <div class="invoice-print-header">
            <span class="invoice-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 张）` }}</span>
            <t-button variant="text" size="small" class="invoice-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="invoice-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并发票 PDF…'" />
            </div>
          </div>
          <div class="invoice-print-footer">
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
  reactive,
  computed,
  watch,
  onMounted,
  onBeforeUnmount,
} from 'vue'
import { useRoute } from 'vue-router'
import { listFleetCategories } from '@/api/fleet'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { colWidthOf, ellipsisOf, cellClassOf, tableScrollOf } from './columnWidth'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  batchDeleteKnowledge,
  updateBusinessMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractBusinessDocument,
  reparseKnowledge,
  deleteInvoicePage,
  listInvoiceRecords,
  listInvoiceTaxRates,
  getInvoiceOverviewStats,
  previewKnowledgeFile,
  getRecognitionConfig,
} from '@/api/knowledge-base'
import { pdfPageOf } from './pdfPage'
import BusinessSplitDetailDrawer from './BusinessSplitDetailDrawer.vue'
import {
  buildDetailPayload, buildDetailFieldDefs, cascadeInvoiceType, extractStatusOf, fillDetailForm, findInvoiceIndex,
  hasInvoiceData, isDetailFormDirty, mergeRowAfterSave, sanitizeNum,
} from './businessDetailForm'
import BusinessSubItemsPopover from './BusinessSubItemsPopover.vue'
import { formatAmount, formatRate } from './businessFormat'
import { useBusinessListSync } from './useBusinessListSync'
import BusinessFilterBar from './BusinessFilterBar.vue'
import type { FilterFieldSpec, FilterState } from './businessFilter'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import { parseExtractQuality } from './extractQuality'
import { extractFieldLabel } from './extractFieldMeta'
import { useBusinessPolling } from '@/composables/useBusinessPolling'
import { isExtractInProgress } from '@/composables/extractConflict'

const KB_NAME = '日常事务-发票'
const PAGE_SIZE = 20

// 发票类型枚举（编辑/筛选推荐值，allow-create 兼容提取值）
const INVOICE_TYPES = ['专用发票', '普通发票', '医疗收据', '财政收据', '其它票据']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
import { INVOICE_FIELD_LABELS, invoiceFieldKeyOf, invoiceFieldLabel, isRemarkPartField, remarkPartOf, setRemarkPart, toCamelKey, toSnakeKey } from './invoiceFieldLabels'
const DEFAULT_INVOICE_COLUMNS: ColumnDef[] = [
  { key: 'invoiceNo', label: '发票号码', default: true, w: '1.5fr' },
  { key: 'invoiceDate', label: '开票日期', default: true, w: '1.1fr' },
  { key: 'invoiceType', label: '发票类型', default: true, w: '1fr' },
  { key: 'amount', label: '金额', default: true, w: '1.1fr' },
  { key: 'taxRate', label: '税率', default: true, w: '0.7fr' },
  { key: 'tax', label: '税额', default: true, w: '1fr' },
  { key: 'totalAmount', label: '价税合计', default: true, w: '1.2fr' },
  { key: 'buyerName', label: '购买方', default: true, w: '1.5fr' },
  { key: 'sellerName', label: '销售方', default: true, w: '1.5fr' },
  { key: 'issuer', label: '开票人', default: true, w: '0.9fr' },
  { key: 'remark', label: '备注', default: false, w: '1.3fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'sellerTaxNo', label: '销售方税号', default: false, w: '1.3fr' },
  { key: 'buyerTaxNo', label: '购买方税号', default: false, w: '1.3fr' },
  { key: 'items', label: '项目明细', default: true, w: '1.4fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns, 
  selectedRowKeys, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-invoice-list-columns',
  defaultColumns: DEFAULT_INVOICE_COLUMNS,
})

const invoiceTableRef = ref<any>(null)

const route = useRoute()

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

interface InvoiceItem {
  invoice_no?: string
  invoice_code?: string
  invoice_date?: string
  invoice_type?: string
  total_amount?: number
  amount?: number
  tax?: number
  tax_rate?: number
  seller_name?: string
  seller_tax_no?: string
  seller_address?: string
  seller_phone?: string
  seller_bank?: string
  seller_account?: string
  buyer_name?: string
  buyer_tax_no?: string
  buyer_address?: string
  buyer_phone?: string
  buyer_bank?: string
  buyer_account?: string
  issuer?: string
  remark?: string
  items?: Array<{ name?: string; spec?: string; unit?: string; qty?: number; price?: number; amount?: number; tax_rate?: number; tax_amount?: number }>
  void_flag?: boolean
  duplicate?: boolean
  page?: number
}

interface InvoiceRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'invoice' | 'not_invoice' | 'unknown' | 'pending'
  // 自定义动态字段扩展桶（如通行费发票「车牌号」）：后端 fields JSON 透传，
  // 列表动态列/详情抽屉按分类 subs.name 读取（colValue 已做回退）
  fields?: Record<string, any>
  invoiceNo?: string
  // page 是票号序号（删除接口定位键，保持 1..N 连续语义）。
  page?: number
  // pageNo 是 PDF 物理页（SourceLocators 解析），预览跳转/批量打印认它，
  // 缺失时由 pdfPageOf 回退 page。
  pageNo?: number
  invoiceDate?: string
  invoiceType?: string
  totalAmount?: number
  amount?: number
  tax?: number
  sellerName?: string
  sellerTaxNo?: string
  sellerAddress?: string
  sellerPhone?: string
  sellerBank?: string
  sellerAccount?: string
  buyerName?: string
  buyerTaxNo?: string
  buyerAddress?: string
  buyerPhone?: string
  buyerBank?: string
  buyerAccount?: string
  issuer?: string
  remark?: string
  items?: InvoiceItem['items']
  /** 发票所属分类（细分分类，如「三联收据」）；用于详情抽屉按分类字段渲染 */
  category?: string
  voidFlag?: boolean
  duplicate?: boolean
  taxRate?: number
  multiIndex?: string
  tags?: any[]
  description?: string
  // 文件级占位行：该源文件没有任何可展示的发票明细（提取失败 / 未提取出票据 /
  // 删空待清理），但文件仍在知识库中。占位行是"知识库 ↔ 列表 100% 对齐"的兜底，
  // 严禁被静默过滤——否则用户既看不见它，又被相同文件哈希挡住重传。
  isPlaceholder?: boolean
  placeholderReason?: string
}


// ---- 列表 ----
const items = ref<KnowledgeItem[]>([])
// shallowRef：几千条发票行时消除深层 Proxy 代理开销（行数据只在整体赋值时更新，
// 单行变化必须整体替换数组，见 saveInvoiceDetail 的 map 重建逻辑）
const invoiceRows = shallowRef<InvoiceRow[]>([])
// 进行中的文件（解析中/提取中/待提取），在发票级列表顶部以状态行展示
const invoiceSummary = ref<{ total: number; sumAmount: number; sumTax: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTax: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const filterInvoiceType = ref('')

// ---- 台账概览看板（对标车辆档案：档案质量 / 当月看板 / 类型分布） ----
const overviewStats = ref<any>(null)
const overviewLoading = ref(false)
// 卡片渲染门控：概览统计 + 分类列表两个异步源都就绪后才一次渲染全部卡片，
// 避免「先 4 张再追加其它票据」的二次跳变（任一请求失败也置 true，用空数据渲染，不永久转圈）
// 卡片渲染门控：分类列表就绪即可先渲染全部卡片（名称/图标/摘要），
// 计数由 overviewStats 提供——未到达时 computed 自然兜底为 0，数据到后自动填充，
// 消除「先空白骨架几秒再整组出现」的加载断层（overview 请求失败也置 loaded，不永久转圈）
const overviewLoaded = ref(false)
const catsLoaded = ref(false)
const cardsReady = computed(() => catsLoaded.value)
const loadInvoiceOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getInvoiceOverviewStats(kbId.value)
    overviewStats.value = res?.data || null
  } catch (_e: any) {
    overviewStats.value = null
  } finally {
    overviewLoading.value = false
    overviewLoaded.value = true
  }
}
const overviewCards = computed(() => {
  const st = overviewStats.value || {}
  const m = st.current_month || {}
  const total = Number(st.total || 0)
  const parseFailed = Number(st.parse_failed || 0)
  const extractFailed = Number(st.extract_failed || 0)
  // 文件级提取失败数：批量上传触发限流（429/1302）时，失败文件一张票都提不出来，
  // 按「发票记录」计数恒为 0 —— 必须用文件级口径才能提示用户去「历史记录」处理。
  const extractFailedFiles = Number(st.extract_failed_files || 0)
  const failed = extractFailedFiles + parseFailed + extractFailed
  const fileCount = Number(st.file_count || 0)
  // 分类统计：后端按 category 精确聚合（category 为空归「未分类」）
  const catCount: Record<string, number> = {}
  for (const c of (st.by_category || [])) catCount[String(c.category)] = Number(c.count) || 0
  // 分类卡：以分类列表（categories[scope=invoice]）为权威源动态生成，
  // 顺序 = 分类数组顺序；隐藏（enabled=false）的分类不出卡片（六同步：同时不出筛选下拉/上传选项）
  const enabledCats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const cards: Array<{ key: string; label: string; icon: string; value: string; unit: string; sub: string; cls: string; action: string; typeValue?: string; numCls?: string }> = [
    { key: 'total', label: '已收录发票', icon: 'file', value: `${total}`, unit: '张', sub: `本月新增 ${Number(m.count || 0)} 张`, cls: '', action: 'all' },
  ]
  let catSum = 0
  for (const c of enabledCats) {
    // 「其它票据」为系统内置兜底分类（seed invoice-misc，SortOrder=99 沉底）：
    // 其统计口径是「排除式桶」（total - 各启用分类之和），不是独立计数卡。
    // 因此跳过该分类卡且不计入 catSum，避免与下方排除式「其它票据」卡重复/口径错乱。
    if (c.name === '其它票据') continue
    const cnt = catCount[String(c.name)] || 0
    catSum += cnt
    const typeIcon = c.name === '专用发票' ? 'file-copy' : c.name === '普通发票' ? 'file-1' : 'file'
    cards.push({ key: `type-${c.name}`, label: c.name, icon: typeIcon, value: `${cnt}`, unit: '张', sub: '', cls: 'is-type', action: 'type', typeValue: c.name })
  }
  // 「其它票据」为排除式统计桶：不在任何启用分类中的记录（含未分类/已停用分类存量），
  // 无条件渲染该卡（计数 0 兜底，overview 数据到达后自动回填），保证五张卡片
  // 一次性渲染，不再出现「先 4 张再追加其它票据」的二次跳变；点击筛选未分类记录
  const otherCount = Math.max(0, total - catSum)
  cards.push({ key: 'type-其它票据', label: '其它票据', icon: 'file-unknown', value: `${otherCount}`, unit: '张', sub: '', cls: 'is-type', action: 'type', typeValue: '其它票据' })
  // 历史记录卡：显示已上传文件数（含解析失败文件）；点击打开已上传文件抽屉。
  // 副标题按失败类型细分：提取失败优先（用户可直接在历史记录里重提），
  // 其次解析失败，全健康时才显示「无异常文件」。
  const historySub = extractFailedFiles > 0
    ? `${extractFailedFiles} 份提取失败`
    : parseFailed > 0
      ? `${parseFailed} 份解析失败`
      : '无异常文件'
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: `${fileCount}`, unit: '份', sub: historySub, cls: failed > 0 ? 'is-warn' : '', action: 'history' })
  return cards
})
const onOverviewCardClick = (card: any) => {
  // 「已收录发票」卡：清空类型筛选显示全部记录（与其它票据排除态一样为显式卡片动作）
  if (card.action === 'all') {
    filterInvoiceType.value = ''
  } else if (card.action === 'type' && card.typeValue) {
    // 类型卡点击：联动列表类型筛选（watch(filterInvoiceType) 自动刷新）
    filterInvoiceType.value = card.typeValue
  } else if (card.action === 'history') {
    // 待复核卡：打开已上传文件抽屉（全部文件，含解析/提取失败与非发票保留件）
    uploadHistoryVisible.value = true
  }
}

// 工具栏「更多操作」：字段配置 / 提取规则设置 / 删除历史
const taxRateFilter = ref<number | string>('')
const taxRateOptions = ref<Array<{ value: string | number; label: string }>>([])
const dateRange = ref<Array<string>>([])

// ---- BusinessFilterBar（P1-3）：4 个筛选器改为 FilterFieldSpec 声明 ----
// 扁平筛选态与上面四个 ref 双向桥接，组件只发不可变更新；
// 页面原有的筛选语义（AND 复合查询、300ms 防抖）保持不变。
const filterState = computed<FilterState>({
  get: () => ({
    keyword: keyword.value,
    invoiceType: filterInvoiceType.value,
    taxRate: taxRateFilter.value,
    dateRange: dateRange.value,
  }),
  set: (v) => {
    keyword.value = String(v?.keyword ?? '')
    filterInvoiceType.value = String(v?.invoiceType ?? '')
    taxRateFilter.value = v?.taxRate ?? ''
    dateRange.value = Array.isArray(v?.dateRange) ? v.dateRange : []
  },
})
const filterFields = computed<FilterFieldSpec[]>(() => [
  // 发票类型筛选统一仅由顶部概览卡片（DashboardKpiGroup）承担，工具栏不再重复下拉
  { key: 'keyword', type: 'keyword', placeholder: '搜索全部字段' },
  { key: 'taxRate', type: 'select', placeholder: '税率筛选', icon: 'percent', options: taxRateOptions.value },
  { key: 'dateRange', type: 'date-range', placeholder: '开票日期', icon: 'time' },
])
const onVisibleColKeysChange = (keys: string[]) => { visibleColKeys.value = keys }

// 列表列取值：通用 colValue 只看 row[key] 与 row.fields[key]，而分类子列
// （车牌号/车辆类型/通行日期起止等 plate_no/vehicle_type/travel_from/travel_to）
// 实际存在**明细行** items[0][key] 里——新数据不再冗余写 fields 扩展桶，
// 导致「车牌号」等列长期渲染为空。此处补一层 items[0] 回退，只作用于发票列表。
const invoiceColValue = (row: any, key: string): any => {
  const v = colValue(row, key)
  if (v !== '-') return v
  // 键名归一兜底：列 key 可能来自分类 subs 的 snake_case（invoice_code），
  // 而行数据是 camelCase（invoiceCode）。不兜这一下，列会整列为空。
  for (const alt of [toCamelKey(key), toSnakeKey(key)]) {
    if (alt === key) continue
    const av = colValue(row, alt)
    if (av !== '-') return av
  }
  const it = Array.isArray(row?.items) ? row.items[0] : null
  if (!it || typeof it !== 'object') return '-'
  // 明细子列在 API 里是 items[0].extra[key]（如 plate_no/vehicle_type/travel_from），
  // 兼容平铺在 items[0][key] 的历史结构。
  const iv = it[key] !== undefined ? it[key] : it.extra?.[key]
  return iv === undefined || iv === null || iv === '' ? '-' : iv
}

const tableColumns = computed(() => {
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 44 }]
  const vis = visibleColDefs.value
  const rows = filteredRows.value.filter(r => r.kind !== 'pending')
  vis.forEach((c, i) => {
    // 列宽：字段配置 width>0 固定（clamp 60~400）；0/未配置按内容自适应（封顶 150）。
    // 自适应估算必须与单元格实际渲染文本一致：状态列渲染 statusOf 中文标签（非原始英文值），
    // 否则空/英文状态值会把列宽压到表头宽度导致中文 tag 截断。
    const rp = c.remarkPart
    const w = colWidthOf(c.width, c.label, rows.map(r => {
      if (rp) return remarkPartOf(r.remark || '', rp)
      if (c.key === 'items') return itemsTextOf(r)
      if (c.key === 'taxRate') return rateTextOf(r)
      if (c.key === 'extractStatus') { const s = statusOf(r).label; return s === '--' ? '' : s }
      return invoiceColValue(r, c.key)
    }), (c as any).dataType)
    // 多行文本列（data_type=textarea，如备注/说明）关闭 ellipsis 并换行完整展示；
    // 其余列保持单行截断 + 悬浮全文，避免长文本把行高撑乱。
    // dataType 随列下传：tableScrollOf 据此判断是否要关掉固定行高（多行文本必须逐行测高）。
    const colBase = {
      colKey: c.key,
      title: c.label,
      ellipsis: ellipsisOf(c as any),
      className: cellClassOf(c as any),
      dataType: (c as any).dataType,
    }
    const base: any = i === vis.length - 1
      ? { ...colBase, minWidth: w }
      : { ...colBase, width: w }
    // 无标准存储位字段（收款方式/收款事由/审核…）：数据存于 remark「字段名：值」片段，按片段渲染
    // TDesign cell 签名为 (h, { row })，第一参是 createElement，row 在第二参 props 中
    if (rp) {
      base.cell = (_h: any, { row }: any) => remarkPartOf(row?.remark || '', rp) || '-'
    } else if (['amount', 'tax', 'totalAmount'].includes(c.key)) {
      // 金额类字段统一格式化（千分位 + 两位小数），消除自定义分类 colKey 不匹配
      // #amount 插槽导致的显示差异；所有分类风格一致。
      base.cell = (_h: any, { row }: any) => {
        const v = invoiceColValue(row, c.key)
        return v === '-' ? '-' : formatAmount(Number(v))
      }
    } else if (!INVOICE_FIELD_LABELS[c.key]) {
      // 自定义动态字段列（如通行费发票「车牌号」）：数据存于 row.fields 扩展桶，
      // TDesign 默认只读 row[colKey] 不查 fields，导致这类列长期渲染为空。
      // 内置契约键列（invoiceNo/sellerTaxNo/items…）走 #colKey 具名插槽，不在此列。
      base.cell = (_h: any, { row }: any) => invoiceColValue(row, c.key)
    }
    cols.push(base)
  })
  return cols
})

// 虚拟滚动配置跟随列定义：出现多行文本列（data_type=textarea）必须关掉固定行高，
// 否则 TDesign 仍按 48px 推算滚动位置，折行后的行会错位、长文本被裁在窗口外。
const tableScroll = computed(() => tableScrollOf(tableColumns.value))

// 字段显隐变化即持久化：t-checkbox-group 的 @change 在部分勾选交互下不触发，
// 用 watch 兜底，确保取消列（如备注）后硬刷新不恢复默认。

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<InvoiceRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<InvoiceRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)

// 质量与溯源（Phase D）：与发票列表同源的 custom_metadata，多张票时按当前张过滤。
// 详情未返回前是空质量 → 面板静默，不会闪一下再消失。
const detailQuality = computed(() =>
  parseExtractQuality(
    (currentDetail.value as any)?.custom_metadata,
    indexOfMulti(currentRow.value?.multiIndex),
  ),
)
const qualityLabel = (f: string) => extractFieldLabel('invoice', f)

// "2/5" → 1（0 基）；单张/缺省 → undefined（全量展示，不做过滤）
function indexOfMulti(multi?: string): number | undefined {
  const n = Number(String(multi || '').split('/')[0])
  return Number.isFinite(n) && n > 0 ? n - 1 : undefined
}
const editForm = ref<Record<string, any>>({ invoice_no: '', invoice_type: '', items: [], data: {}, remarkParts: {} })
const autoSaving = ref(false)

// 详情抽屉动态字段：按当前发票类型匹配分类 subs（启用字段优先），无配置时回退内置列
// 字段清洗走中台 buildDetailFieldDefs：税额同义去重（tax/taxAmount/tax_amount 只留一个）、
// 剔除 items 容器字段与错位混入主表的明细子项（spec/unit/qty/price/name）、核心字段置顶。
// 字段 name 一律归一到英文契约键（自定义中文字段名经 invoiceFieldKeyOf 映射）。
const editFieldDefs = computed(() => {
  const t = editForm.value.invoice_type
  const cats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const hit = cats.find((c: any) => c.name === t) || cats[0]
  const subs = hit?.subs
  if (Array.isArray(subs) && subs.length) {
    return buildDetailFieldDefs(subs, {
      fieldKeyOf: (n: string) => invoiceFieldKeyOf(n),
      labelOf: (n: string) => INVOICE_FIELD_LABELS[n] || invoiceFieldLabel(n),
      isRemarkPart: (n: string) => isRemarkPartField(n),
    })
  }
  // 无分类配置回退内置列：剔除 items（明细由表格区块承载，不生成占位行）
  return DEFAULT_INVOICE_COLUMNS
    .filter((c: any) => c.key !== 'items')
    .map((c: any) => ({
      name: c.key,
      label: c.label,
      dataType: c.key === 'invoiceDate' ? 'date' : ['amount', 'tax', 'totalAmount', 'taxRate'].includes(c.key) ? 'number' : 'text',
      remarkPart: '',
    }))
})

// 抽屉宽度（可拖动，localStorage 记忆）
// 打印预览
const printVisible = ref(false)
const uploadHistoryVisible = ref(false)
const invoiceSettingsVisible = ref(false)
const printCount = ref(0)
const printBusy = ref(false)
const printUrl = ref('')
// 释放旧 Blob URL，避免重复生成/关闭弹窗时泄漏内存
function revokePrintUrl() {
  if (printUrl.value) {
    URL.revokeObjectURL(printUrl.value)
    printUrl.value = ''
  }
}
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
      await loadTaxRates()
      await loadTypeOptions()
      await cleanNonInvoiceFiles()
      await loadFiles()
      loadInvoiceOverview()
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
    await loadTaxRates()
    await loadTypeOptions()
    await cleanNonInvoiceFiles()
    await loadFiles()
    startPolling()
  }
}

// ---- 存量非发票清理：扫描知识库中已标记 not_invoice 的文件并自动删除 ----
let cleaningNonInvoice = false
const cleanNonInvoiceFiles = async () => {
  if (!kbId.value || cleaningNonInvoice) return
  cleaningNonInvoice = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_invoice' || it.custom_metadata?.extract_status === 'not_invoice')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非发票文件（旧数据清理）`)
      loadTaxRates()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonInvoice = false }
}

// ---- 税率下拉：加载该知识库下所有发票出现过的去重税率 ----
const loadTaxRates = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listInvoiceTaxRates(kbId.value)
    const list = res?.data || res?.list || []
    const arr = Array.isArray(list) ? list : []
    taxRateOptions.value = arr.map((r: any) => ({ value: Number(r.tax_rate), label: formatRate(r.tax_rate) }))
  } catch { /* 税率加载失败不阻塞 */ }
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


const openTagEdit = (row: InvoiceRow) => {
  tagTarget.value = row
  tagDialogVisible.value = true
}

// 行内 tags 可能只携带标签名（后端返回字符串数组时被转为 { id: name }），
// 打开弹窗时按 tagList 反查真实 id，保证已选回显与提交 id 正确
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
    // 兜底：混合传入的标签名/ID 统一反查真实 tag id（标签名会触发后端 400）
    const realIds = tagIds.map(id => {
      const hit = tagList.value.find((t: any) => String(t.id) === id || t.name === id)
      return hit ? String(hit.id) : id
    })
    await updateKnowledgeTagBatch({ updates: { [tagTarget.value.knowledgeId]: realIds } })
    MessagePlugin.success('标签已更新')
    await loadFiles(true)
    loadInvoiceOverview()
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
  loadInvoiceOverview()
}

// ---- 列表加载（发票级聚合列表，懒加载分页） ----
// listSeq 请求序号守卫：类型/筛选切换时递增，丢弃过期响应，防止旧筛选
// （轮询/翻页）in-flight 请求晚到被 append 进新筛选结果造成行混入。
let listSeq = 0
// 加载超时兜底：切换类型/筛选后若列表请求长时间未返回（代理/后端瞬慢或挂起），
// 15s 后强制结束 loading 并提示，避免表格永久转圈；新请求接管时旧 timer 作废。
let loadTimer: ReturnType<typeof setTimeout> | null = null
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  // 防重检查必须在 ++listSeq 之前：非 reset（翻页/onTick 轮询）若在 reset 请求进行中空转，
  // 绝不能递增请求序号，否则进行中的 reset 请求会被误判为「过期响应」而丢弃，
  // 其 finally 不再清理 listLoading、超时 timer 回调也因 seq 不匹配而失效 → 永久转圈。
  if (!reset && (listLoading.value || loadingMore.value)) return
  const mySeq = ++listSeq
  if (reset) {
    // reset（类型/筛选切换/手动刷新）接管并重置超时计时器；
    // 新 reset 会作废旧 timer，非 reset 空转不碰 timer。
    if (loadTimer) { clearTimeout(loadTimer); loadTimer = null }
    page.value = 1
    invoiceRows.value = []
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
    // 「其它票据」为排除式筛选：附上启用分类名集合，后端按 category 不属于该集合过滤，
    // 与概览卡其它票据口径一致（否则会命中 invoice_type 非普通/专用的全部记录）。
    // 「其它票据」本身已是系统内置实体分类（seed invoice-misc），但统计/筛选口径保持
    // 排除式桶（category=其它票据 的记录属于「已分类」，不属于本桶）——因此从
    // knownCats 剔除它，后端同样做了防御剔除，双保险保证 category=其它票据 不被排除。
    const knownCats = invoiceCats.value
      .filter((c: any) => c.enabled !== false)
      .map((c: any) => String(c.name))
      .filter((n: string) => Boolean(n) && n !== '其它票据')
    const res: any = await listInvoiceRecords(kbId.value, {
      q: keyword.value || undefined,
      invoice_type: filterInvoiceType.value || undefined,
      known_categories: filterInvoiceType.value === '其它票据' && knownCats.length > 0
        ? knownCats.join(',') : undefined,
      tax_rate: taxRateFilter.value === '' || taxRateFilter.value === null || taxRateFilter.value === undefined
        ? undefined : Number(taxRateFilter.value),
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      sort_by: sortBy.value || undefined,
      sort_order: sortDirection.value || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    // 过期响应丢弃：此期间列表已按新筛选重置，旧结果不得混入
    if (mySeq !== listSeq) return
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    invoiceSummary.value = {
      total,
      sumAmount: Number(res?.sum_amount || 0),
      sumTax: Number(res?.sum_tax || 0),
      sumTotal: Number(res?.sum_total || 0),
    }
    // 行合并：已加载的行（同 rowKey）用最新响应替换，新行追加。
    // 修复回填/重新提取后 fields 更新不生效——旧写法 `filter(!existing.has)` 会把
    // rowKey 相同的更新数据全部丢弃，导致列表永远展示旧行（如车牌号列长期为空）。
    const byKey = new Map(invoiceRows.value.map((r: any) => [r.rowKey, r]))
    for (const r of arr.map(mapInvoiceRecord)) byKey.set(r.rowKey, r)
    invoiceRows.value = [...byKey.values()]
    hasMore.value = invoiceRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    if (mySeq !== listSeq) return
    MessagePlugin.error(e?.message || '发票列表加载失败')
  } finally {
    if (loadTimer && mySeq === listSeq) { clearTimeout(loadTimer); loadTimer = null }
    // 仅最新请求可清理 loading 标志；过期响应保持其父请求的进行中状态
    if (mySeq === listSeq) {
      listLoading.value = false
      loadingMore.value = false
    }
  }
}

// 把后端发票级记录（InvoiceRecord）映射为前端行
const mapInvoiceRecord = (r: any): InvoiceRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.invoice_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'invoice',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  // 文件级占位行透传：后端对"无任何发票明细但文件仍在库中"的记录置位，
  // 前端绝不静默过滤（知识库 ↔ 列表 100% 对齐铁律）。
  isPlaceholder: !!r.is_placeholder,
  placeholderReason: r.placeholder_reason || '',
  invoiceNo: r.invoice_no || '',
  invoiceDate: r.invoice_date || '',
  invoiceType: r.invoice_type || '',
  totalAmount: numOrUndef(r.total_amount),
  amount: numOrUndef(r.amount),
  tax: numOrUndef(r.tax),
  taxRate: numOrUndef(r.tax_rate),
  sellerName: r.seller_name || '',
  sellerTaxNo: r.seller_tax_no || '',
  sellerAddress: r.seller_address || '',
  sellerPhone: r.seller_phone || '',
  sellerBank: r.seller_bank || '',
  sellerAccount: r.seller_account || '',
  buyerName: r.buyer_name || '',
  buyerTaxNo: r.buyer_tax_no || '',
  buyerAddress: r.buyer_address || '',
  buyerPhone: r.buyer_phone || '',
  buyerBank: r.buyer_bank || '',
  buyerAccount: r.buyer_account || '',
  issuer: r.issuer || '',
  remark: r.remark || '',
  category: r.category || r.invoice_category || '',
  items: Array.isArray(r.items) ? r.items : [],
  voidFlag: !!r.void_flag,
  duplicate: !!r.duplicate,
  fields: r.fields && typeof r.fields === 'object' ? r.fields : {},
  page: Number(r.page) || 0,
  pageNo: Number(r.page_no ?? r.pageNo) || 0,
})


const parseCustomMetadata = (item: KnowledgeItem): InvoiceRow[] => {
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
  if (kind === 'not_invoice') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_invoice', extractError, kind, ...base }]
  }
  const invoices = Array.isArray(meta.invoices) ? meta.invoices : []
  if (invoices.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return invoices.map((inv: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: invoices.length > 1 ? `${fileName}（第${idx + 1}张）` : fileName,
    multiIndex: invoices.length > 1 ? `${idx + 1}/${invoices.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    invoiceNo: inv.invoice_no || '',
    invoiceDate: inv.invoice_date || '',
    invoiceType: inv.invoice_type || '',
    totalAmount: numOrUndef(inv.total_amount),
    amount: numOrUndef(inv.amount),
    tax: numOrUndef(inv.tax),
    taxRate: numOrUndef(inv.tax_rate),
    page: Number(inv.page) || 0,
    sellerName: inv.seller_name || '',
    sellerTaxNo: inv.seller_tax_no || '',
    sellerAddress: inv.seller_address || '',
    sellerPhone: inv.seller_phone || '',
    sellerBank: inv.seller_bank || '',
    sellerAccount: inv.seller_account || '',
    buyerName: inv.buyer_name || '',
    buyerTaxNo: inv.buyer_tax_no || '',
    buyerAddress: inv.buyer_address || '',
    buyerPhone: inv.buyer_phone || '',
    buyerBank: inv.buyer_bank || '',
    buyerAccount: inv.buyer_account || '',
    issuer: inv.issuer || '',
    remark: inv.remark || '',
    category: inv.category || '',
    items: Array.isArray(inv.items) ? inv.items : [],
    voidFlag: !!inv.void_flag,
    duplicate: !!inv.duplicate,
    fields: inv.fields && typeof inv.fields === 'object' ? inv.fields : {},
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端发票级接口完成，前端直接使用返回的分页数据。
// 「进行中文件」（解析中/提取中/待提取）不再以伪行占用列表，改为列表上方的独立
// 状态条（BusinessParseStatusBar）滚动展示；列表只渲染已产生发票行的记录。
const filteredRows = computed(() => invoiceRows.value)

// 表格高度约束：数据加载后由空串切到 '100%'，驱动 TDesign useFixed 重算
// isFixedHeader（scrollHeight > clientHeight）→ 表头/表体分离，滚动条仅在表体
// 滚动条固定：无论记录多少，表格始终撑满容器高度 → 垂直滚动条轨道恒显于底部（统计摘要栏上方）
const tableMaxHeight = computed(() => '100%')

// 同一上传文件内的多张发票：按文件分组计数，用于生成绿色页码标签
const fileInvoiceCounts = computed(() => {
  const map = new Map<string, number>()
  for (const r of invoiceRows.value) {
    if (!r.knowledgeId) continue
    map.set(r.knowledgeId, (map.get(r.knowledgeId) || 0) + 1)
  }
  return map
})
// 仅同一文件含多张发票时显示：有真实页码显示 P{n}，否则显示"第n张/共m张"
const pageBadgeOf = (row: InvoiceRow): string => {
  if (!row.knowledgeId) return ''
  const count = fileInvoiceCounts.value.get(row.knowledgeId) || 0
  // 物理页码优先（与预览/打印同一判据 pdfPageOf）；没有物理页的老数据
  // （page_no 缺失）回落序号。同页两票显示同一 P 号是正确的。
  const physical = pdfPageOf(row)
  // 物理页码 ≥2 时**无条件**展示：用户按票号搜索时列表只剩这一行，
  // 若按「同文件多张才显示」判断，页码就会消失，而此刻恰恰最需要它去翻 PDF。
  if (physical >= 2) return `P${physical}`
  if (count <= 1) return ''
  if (physical >= 1) return `P${physical}`
  const same = invoiceRows.value.filter(r => r.knowledgeId === row.knowledgeId)
  const idx = same.findIndex(r => r.rowKey === row.rowKey)
  return idx >= 0 ? `${idx + 1}/${count}` : ''
}

const invoiceTypeOptions = computed(() => {
  // 六同步：类型下拉以 categories(scope=invoice) 为权威源（设置页改名/排序/启停后自动同步），
  // 未入库时回退识别规则配置 -> 内置枚举；「其它票据」为排除式统计桶，始终追加在末尾
  const catNames = invoiceCats.value
    .filter((c: any) => c.enabled !== false)
    .map((c: any) => c.name)
    .filter((n: string) => n && n !== '其它票据')
  const base = catNames.length
    ? catNames
    : kbTypes.value.length
      ? kbTypes.value
      : INVOICE_TYPES.filter((n) => n !== '其它票据')
  const opts = base.map((v: string) => ({ value: v, label: v }))
  // 「其它票据」是系统内置实体分类（seed invoice-misc）：详情抽屉改类型/新建记录时
  // 也必须可选，否则用户无法把杂散票据归入该分类（仅靠排除式统计桶兜底）。
  if (!opts.some((o) => o.value === '其它票据')) opts.push({ value: '其它票据', label: '其它票据' })
  return opts
})

// ---- 类型分类列表：从识别规则配置加载（支持自定义分类增删改查） ----
const kbTypes = ref<string[]>([])
const loadTypeOptions = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    if (Array.isArray(c?.types) && c.types.length) kbTypes.value = c.types.filter(Boolean)
  } catch { /* 保持现状 */ }
}
// 重新拉取后端发票动态列配置（字段定义保存后同步刷新列表列）
// 列表列以「普通发票」分类字段为底稿（切换任意类型后列表样式统一），
// 但列宽/默认表头/remarkPart 片段跟随当前选中分类（六同步：当前分类的 subs 优先）。
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    // 分类缓存：切换类型时直接用已拉取的分类重建列（省一次请求、切换更快）；
    // 仅首次进入或设置保存/六同步事件（force=true）时重新请求后端
    let cats: any[] = force ? [] : invoiceCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'invoice' })
      cats = res?.data || []
      invoiceCats.value = cats
    }
    // 列表列按「当前选中分类」的字段配置生成（六同步：列表/筛选器/编辑抽屉同源）。
    // 自定义分类（如三联收据）显示自己配置的中文字段；未配置字段的分类回退普通发票字段。
    const curCat = cats.find((c: any) => c.name === filterInvoiceType.value && c.enabled !== false)
    const baseCat = curCat || cats.find((c: any) => c.name === '普通发票') || cats[0]
    if (baseCat?.subs?.length) {
      customColumns.value = baseCat.subs
        .filter((s: any) => s.enabled !== false)
        .map((s: any) => {
          const builtin = DEFAULT_INVOICE_COLUMNS.find((b: any) => b.key === s.name)
          const width = Number(s.width) || 0
          // 默认表头：分类保存的 is_default 为准（关闭后重置不再恢复默认勾选）
          const isDefault = s.is_default === true
          // 无标准存储位字段（收款方式/收款事由/审核…）：数据在 remark 片段，列表列按片段渲染
          const remarkPart = isRemarkPartField(String(s.name)) ? String(s.name) : ''
          return {
            key: remarkPart ? String(s.name) : invoiceFieldKeyOf(String(s.name)),
            label: String(s.label || '').trim() || invoiceFieldLabel(String(s.name)),
            default: isDefault,
            w: builtin?.w || '1fr',
            width,
            remarkPart,
            // 类型以 subs.data_type 为权威（六同步）：列表据此决定是否折行完整展示，
            // 抽屉/提取规则读的是同一个字段，三处不会漂移
            dataType: String(s.data_type ?? s.dataType ?? '').trim() || 'text',
          }
        })
      // 六同步：切换分类或设置保存后，按当前分类默认表头重建字段筛选器勾选。
      // 首轮加载（lastColType 为空）时保留用户既有 localStorage 存档，但若存档缺当前分类
      // 的默认列（如 URL 直带「三联收据」刷新，存档仍是普通发票列集），则补齐当前分类默认列
      if (!lastColType.value) {
        const defaults = customColumns.value.filter(c => c.default).map(c => c.key)
        if (defaults.some(k => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterInvoiceType.value) {
        resetColumns()
      }
      lastColType.value = filterInvoiceType.value
    }
    // 类型筛选锁定（Q4）：分类重载后若当前选中项失效（被禁用/改名），回退第一个启用的分类
    ensureInvoiceType()
  } catch { /* 后端未配置时回退内置默认 */ } finally {
    catsLoaded.value = true
  }
}

// 类型筛选锁定：始终保证有一项分类被选中（禁清空）。
// 优先保留当前合法选中项（含路由 query 回填），否则按分类顺序选中第一个启用分类。
// 搜索（关键词）不再清空类型筛选：keyword、税率、日期与类型为 AND 复合查询。
const ensureInvoiceType = () => {
  const valid = new Set(invoiceTypeOptions.value.map((o) => o.value))
  // 「其它票据」为排除式筛选态（不在分类下拉 options 中，点击其它票据卡进入），分类重载时须放行
  if (filterInvoiceType.value && (valid.has(filterInvoiceType.value) || filterInvoiceType.value === '其它票据')) return
  // 空值 = 「全部」态（点击已收录发票卡进入）：与搜索态一样是显式卡片动作，
  // 放行以避免被弹回第一个分类；工具栏下拉本身禁清空，用户无法手动进入此态
  if (filterInvoiceType.value === '') return
  const cats = invoiceCats.value.filter((c: any) => c.enabled !== false).map((c: any) => c.name)
  filterInvoiceType.value = (cats.length ? cats[0] : invoiceTypeOptions.value[0]?.value) || ''
}


// ---- 多选 ----
const selectableRows = computed(() => filteredRows.value.filter(r => r.kind !== 'pending'))
// select-change 守卫：只接受与当前可选项 rowKey 匹配的 key，杜绝受控模式幽灵写回导致浮条残留
const onTableSelectChange = (val: string[]) => {
  const valid = new Set(selectableRows.value.map(r => r.rowKey))
  selectedRowKeys.value = val.filter(k => valid.has(k))
}
const selectedRows = computed(() => {
  const byKey = new Map(invoiceRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as InvoiceRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中发票的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  let sumAmount = 0
  let sumTax = 0
  let sumTotal = 0
  for (const r of rows) {
    sumAmount += Number(r.amount) || 0
    sumTax += Number(r.tax) || 0
    sumTotal += Number(r.totalAmount) || 0
  }
  return { total: rows.length, sumAmount, sumTax, sumTotal }
})
const displaySummary = computed(() => selectedSummary.value || invoiceSummary.value)

// 底部统计条按当前分类字段动态显示：无 tax/totalAmount 字段的分类（如三联收据）不显示冗余统计项。
// subs.name 为中文自定义名（金额/税额…），经 invoiceFieldKeyOf 归一为契约键后再比对。
const currentCatSubs = computed(() => {
  const t = filterInvoiceType.value
  const cats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const hit = cats.find((c: any) => c.name === t) || cats[0]
  return Array.isArray(hit?.subs) ? hit.subs : []
})
const summaryShowAmount = computed(() =>
  currentCatSubs.value.some((s: any) => invoiceFieldKeyOf(String(s.name)) === 'amount' && s.enabled !== false))
const summaryShowTax = computed(() =>
  currentCatSubs.value.some((s: any) => invoiceFieldKeyOf(String(s.name)) === 'tax' && s.enabled !== false))
const summaryShowTotal = computed(() =>
  currentCatSubs.value.some((s: any) => invoiceFieldKeyOf(String(s.name)) === 'totalAmount' && s.enabled !== false))

const applyFilter = () => { loadFiles(true); loadInvoiceOverview() }

// ---- 筛选合并（300ms） ----
// 问题：类型/税率/关键词三个 watch 各自触发 applyFilter，而 applyFilter 同时打
// /invoices + /invoices/overview-stats 两个接口 —— 一次操作就能连发 3~4 个请求。
// 统一收敛到一个 300ms 合并窗口：窗口内的多次变更只发一次。
let filterTimer: ReturnType<typeof setTimeout> | null = null
const scheduleFilter = () => {
  if (filterTimer) clearTimeout(filterTimer)
  filterTimer = setTimeout(() => {
    filterTimer = null
    applyFilter()
  }, 300)
}

// ---- 表头排序（服务端排序）：sort_by=invoice_no|invoice_date ----
const sortBy = ref('')
const sortDirection = ref('desc')
const sortState = computed(() => sortBy.value ? [{ sortBy: sortBy.value, desc: sortDirection.value === 'desc' }] : [])
const onSortChange = (ctx: any) => {
  // TDesign 返回列 colKey（invoiceNo/invoiceDate），映射为后端 sort_by 参数（invoice_no/invoice_date）
  const by = ctx?.sortBy || ''
  const dir = ctx?.direction
  const map: Record<string, string> = { invoiceNo: 'invoice_no', invoiceDate: 'invoice_date' }
  if (!by || !map[by]) {
    sortBy.value = ''
    sortDirection.value = 'desc'
  } else {
    sortBy.value = map[by]
    sortDirection.value = dir === 'asc' ? 'asc' : 'desc'
  }
  loadFiles(true)
}
// 筛选联动：类型下拉变更即时刷新（修复中台 Toolbar 只发 update 不触发刷新的空转）；
// 同时重载列表列（列宽跟随当前分类配置生效）。
// 状态隔离：切换类型时清空多选，防止浮条「已选 N 项」残留上一类型的脏数据。
watch(filterInvoiceType, () => {
  // 本地副作用（清多选、重建列）立即执行；网络请求走下方 filterState 的合并窗口。
  clearSelectionSafe()
  reloadColumns()
})
// 税率筛选联动：变更时清空多选（税率不影响列定义，无需 reloadColumns）
watch(taxRateFilter, () => {
  clearSelectionSafe()
})
// 关键词、税率、日期、类型为 AND 复合查询：输入关键词时不再清空类型筛选，
// 四个条件同时发送后端取交集。四个筛选统一走 300ms 合并窗口（连续操作只发一次 /invoices）。
watch(filterState, () => {
  scheduleFilter()
})

// 懒加载（触底翻页）：滚动发生在 t-table 内部内容容器（.t-table__content，外层 overflow:hidden），
// 事件经 t-table @scroll 转发（params: { e: WheelEvent }），取 e.target 判触底。
const listScrollRef = ref<HTMLElement>()
const onListScroll = ({ e }: any) => {
  const el = e?.target as HTMLElement
  if (!el) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200 && hasMore.value && !listLoading.value && !loadingMore.value) {
    loadFiles()
  }
}

// ---- 上传（对齐车队：FleetUploadDialog 弹窗，支持拖拽/选择/剪贴板粘贴） ----
const uploadVisible = ref(false)
// 上传弹窗分类选项：categories['invoice'] 分类列表（含新增自定义类型），复合值 scope__name
const invoiceCats = ref<any[]>([])
const uploadTypeOptions = computed(() => {
  const cats = invoiceCats.value.filter((c: any) => c.enabled !== false)
  if (!cats.length) return [{ label: '发票', value: 'invoice__发票' }]
  return cats.map((c: any) => ({ label: c.name, value: `invoice__${c.name}` }))
})
const onUploadDone = async (typeName?: string) => {
  // 上传完成联动：携带本次上传的票据类型 → 列表切换为该细分分类（watch 触发 applyFilter）
  if (typeName && typeName !== filterInvoiceType.value) filterInvoiceType.value = typeName
  await loadFiles(true)
  loadInvoiceOverview()
}
// 上传历史抽屉内容变化：
// - 删除（带 deletedIds）：异步删除，先乐观移除再延迟权威对账（见 schedulePostDeleteRefresh）；
// - 重新提取等（无 payload）：custom_metadata 已同步写回，立即刷新。
const onUploadHistoryChanged = (payload?: { deletedIds?: string[] }) => {
  if (payload?.deletedIds?.length) {
    optimisticRemoveInvoiceRows(new Set(payload.deletedIds), new Set())
    schedulePostDeleteRefresh()
    return
  }
  loadFiles(true)
  loadInvoiceOverview()
}

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling, scanOnce, setPollingInterval,
} = useBusinessPolling({
  kbId,
  // 传入当前细分分类：自动提取（轮询）此前不带 cert_type，后端只能靠票面推断，
  // 推断结果不在启用列表时就降级成「其它票据」，覆盖了用户上传时选择的分类。
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'invoice', filterInvoiceType.value || undefined),
  onPendingFiles: (files) => {
    // 状态条展示所有「进行中」文件：不仅新上传待解析/待提取的文件，也包括
    // 已产生发票行、正处于「重新解析/重新提取」中的文件（重解/重提选中的记录
    // 必然已有发票行，若按行过滤会把它们排除出状态条，只剩 toast 提示）。
    // 轮询已保证 pend 只含解析中/提取中文件，完成/失败的文件不会进入这里。
    pendingFiles.value = files
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是电子发票，已保留待人工复核`)
    loadTaxRates()
  },
  onTick: () => loadFiles(),
})

// 上传任务实时进度（来自上传弹窗 @progress，车队同款）：多文件同时进行时轮播展示
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
    invoiceRows.value
      .filter((r) => r.extractStatus === 'success' || r.extractStatus === 'failed')
      .map((r) => r.fileName),
  )
  return items.filter((it: any) => !doneNames.has(it.name))
}
// 轮询刷新列表后：用表格实际 extractStatus 清掉已完成文件的残留进度
watch(invoiceRows, () => {
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
      // 提取阶段**不显示百分比**：后端只返回状态枚举（pending/processing），没有真实
      // 进度数据；此前的 30 + elapsed*2 估算在 35 秒后就封顶 100%，用户看到的是
      // "进度 100% 却迟迟不出结果"（实为假进度，2026-10-17 用户反馈）。
      // 这里给 percent=null，组件只展示阶段与文件名，不再误导。
      return { name, stage: 'extracting' as const, percent: null, elapsedSec: Math.round(elapsed) }
    }
    // 解析阶段同样没有真实百分比（后端只给状态枚举），统一展示"已用时长"，
    // 不用 10+elapsed*4 这类估算——用户已明确要求"算不准就换一种方式反映进度"。
    return { name, stage: 'parsing' as const, percent: null, elapsedSec: Math.round(elapsed) }
  })
})
// pendingFiles 的唯一订阅点（原先有两个重复 watch，每轮轮询各跑一次）：
//  1) 集合变化 → 重载 KPI（解析/提取完成后卡片数字要跟着动）；
//  2) 清理已离开集合的进度起点（避免 id 复用导致百分比从旧值续算）；
//  3) 空闲降频：没有在途文件时把轮询从 3s 退到 15s，有新任务立刻回到 3s。
//     这一步直接削减了空转时 80% 的 /invoices 请求。
const IDLE_POLL_MS = 15000
const ACTIVE_POLL_MS = 3000
let lastPendingKey = ''
watch(pendingFiles, (v) => {
  const key = v.map((f: any) => f.id).sort().join(',')
  if (key !== lastPendingKey) {
    lastPendingKey = key
    if (kbId.value) loadInvoiceOverview()
  }
  const alive = new Set(v.map((f: any) => f.id))
  for (const id of Array.from(fileStartAt.keys())) {
    if (!alive.has(id)) fileStartAt.delete(id)
  }
  setPollingInterval(v.length > 0 ? ACTIVE_POLL_MS : IDLE_POLL_MS)
})
// 弹窗打开期间优先展示上传弹窗上报的真实进度（含真实上传百分比）；
// 弹窗关闭后 uploadProgress 停更，自动切换到轮询派生的解析/提取进度，状态条不中断。
const displayProgress = computed(() =>
  uploadProgress.value.length ? uploadProgress.value : liveProgress.value,
)

const { statusOf, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'invoice', notLabel: '非发票',
  // 发票行级状态以后端归一结果为准：单张票核心字段完整就是「提取成功」，
  // 不因同文件重提（文件级 pending/processing/failed）被打回中间态。
  preferRecordedSuccess: true,
})

// ---- 详情抽屉 ----
// 行点击打开详情；点击多选 checkbox 区域不触发（避免勾选即弹窗）
const onRowClick = (row: InvoiceRow, e?: MouseEvent) => {
  const t = e?.target as HTMLElement | null
  if (t?.closest('.t-table__cell-check') || t?.closest('.t-checkbox') || t?.closest('input[type="checkbox"]')) return
  openDetail(row)
}
const openDetail = async (row: InvoiceRow) => {
  if (row.kind === 'pending') return
  currentRow.value = row
  currentDetail.value = null
  // 先填充表单再打开抽屉：editFieldDefs 依赖 editForm.invoice_type，
  // 若先 detailVisible=true 再 fillEditForm，抽屉打开瞬间会渲染上一分类（或空）的字段，
  // 造成「先显示发票字段再跳转收据字段」的闪烁。行级 invoiceType/category 已就绪，可立即定字段。
  fillEditForm()
  detailVisible.value = true
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    if (detail && typeof detail === 'object') {
      currentDetail.value = detail
      if (detail.custom_metadata) {
        const parsed = parseCustomMetadata(detail)
        const match = parsed.find(p => row.page && p.page === row.page)
          || parsed.find(p => p.invoiceNo === row.invoiceNo) || parsed[0]
        if (match) {
          // items 优先用列表行：列表接口返回的明细已带 extra（plate_no/vehicle_type/
          // travel_from/travel_to 等分类子列），而详情接口的 items 结构可能不含 extra。
          // 直接 {...row, ...match} 会让 match 的空 items 覆盖掉列表值，表现就是
          // 抽屉先显示车牌号、1 秒后（详情返回重填时）又消失。
          const keepItems = Array.isArray(row.items) && row.items.length ? row.items : (match.items || [])
          currentRow.value = { ...row, ...match, items: keepItems }
          // 详情数据到达后重填一次（发票类型/字段值以落库数据为准）
          fillEditForm()
        }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
}

const detailTitle = computed(() =>
  currentRow.value?.invoiceNo ? `发票详情 · ${currentRow.value.invoiceNo}` : '发票详情'
)

// ---- 字段编辑 + 手动保存 ----
// 明细子列口径（单一权威源 = 分类字段配置中 items 字段的 sub_fields，设置抽屉可编辑）。
// 通行费发票 → 车牌号/车辆类型/通行日期起止；电费/普通等 → 规格型号/单位/数量/单价；
// 分类未配置子列时回退标准 7 列（项目名称/单位/数量/单价/金额/税率/税额），
// 保证非明细分类与旧数据仍可编辑。
const fallbackItemSubFields = () => [
  { key: 'name', label: '项目名称', dataType: 'text' },
  { key: 'spec', label: '规格型号', dataType: 'text' },
  { key: 'unit', label: '单位', dataType: 'text' },
  { key: 'qty', label: '数量', dataType: 'number' },
  { key: 'price', label: '单价', dataType: 'number' },
  { key: 'amount', label: '金额', dataType: 'number' },
  { key: 'tax_rate', label: '税率/征收率', dataType: 'number' },
  { key: 'tax_amount', label: '税额', dataType: 'number' },
]
// 按行计算明细子列（列表行 hover 回显用；itemSubFields 复用同一口径，仅绑定当前编辑行）
const subFieldsOfRow = (row: InvoiceRow | null) => {
  if (!row) return fallbackItemSubFields()
  const targetType = row.invoiceType || row.category || filterInvoiceType.value || ''
  const cats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const hit = cats.find((c: any) => c.name === targetType) || cats[0]
  const itemsF = (hit?.subs || []).find((s: any) => s.name === 'items')
  const sfs: any[] = itemsF?.sub_fields || []
  return sfs.length ? sfs : fallbackItemSubFields()
}

/**
 * 明细浮层专用：只保留启用的子项列。
 *
 * 与 subFieldsOfRow 分开的原因：详情抽屉的明细编辑表需要"一定有几列可填"，
 * 所以它保留 fallback；而浮层是只读展示，用户关掉的列必须真的消失。
 */
const enabledSubFieldsOfRow = (row: InvoiceRow | null) => {
  const all = subFieldsOfRow(row)
  const sfs: any[] = row ? ((((invoiceCats.value || []).filter((c: any) => c.enabled !== false)
    .find((c: any) => c.name === (row.invoiceType || row.category || filterInvoiceType.value || ''))?.subs || [])
    .find((x: any) => x.name === 'items')?.sub_fields) || []) : []
  // 分类没配子列（走 fallback）时全部保留；配了子列就按 enabled 过滤
  if (!sfs.length) return all
  return all.filter((sf: any) => {
    const hit = sfs.find((x: any) => String(x.key || '') === String(sf.key || ''))
    return !hit || hit.enabled !== false
  })
}

const itemSubFields = computed(() => subFieldsOfRow(currentRow.value))
// 写入明细行子列值：标准键写行对象；分类子列写 extra 桶（后端 UnmarshalJSON 自动收集）

// 行数据 → 编辑表单：口径全部在 businessDetailForm.ts（纯函数 + 单测锁定），
// 本页只注入发票自己的字典与工具函数（分类/备注片段/契约键归一）。
const detailBaseline = ref<any>(null)
const detailDirty = ref(false)
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  editForm.value = fillDetailForm({
    row: r,
    cats: invoiceCats.value || [],
    filterType: filterInvoiceType.value || '',
    itemSubFields: itemSubFields.value,
    isRemarkPart: (n: string) => isRemarkPartField(n),
    remarkPartOf: (remark: string, n: string) => remarkPartOf(remark, n),
    fieldKeyOf: (n: string) => invoiceFieldKeyOf(n),
    dict: INVOICE_FIELD_LABELS,
  })
  // 脏检查基线：保存按钮按"是否真改动"启用，避免打开抽屉就允许空保存
  detailBaseline.value = JSON.parse(JSON.stringify(editForm.value))
  detailDirty.value = false
}
watch(editForm, () => {
  detailDirty.value = isDetailFormDirty(detailBaseline.value, editForm.value)
}, { deep: true })

// 明细行联动计算（保留两位小数）：
//   金额 = 数量 × 单价；税额 = 金额 × 税率(%)。
// 手动改金额/税率时只联动税额，避免覆盖用户手填的金额。

const saveEditForm = async () => {
  await persistEditForm(false)
}

// 单号重复二次确认：后端 409 + DUPLICATE_INVOICE_NO 时先询问，确认后 force 重试
const dupConfirmVisible = ref(false)
const dupNo = ref('')
const onDupConfirm = async () => {
  dupConfirmVisible.value = false
  await persistEditForm(true)
}

// 源文件预览：默认折叠、展开才挂载，由 BusinessSplitDetailDrawer 负责
const persistEditForm = async (force: boolean) => {
  const now = currentRow.value
  if (!now) return
  autoSaving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const invoices = Array.isArray(meta.invoices) ? [...meta.invoices] : []
    // 表单 → 载荷：税率百分比↔小数、备注片段回写、动态字段入 fields 桶全部走纯函数
    const updated = buildDetailPayload({
      data: editForm.value.data || {},
      remarkParts: editForm.value.remarkParts || {},
      invoiceType: editForm.value.invoice_type || '',
      items: editForm.value.items,
      setRemarkPart,
    })
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    const targetIdx = findInvoiceIndex(invoices, nowPage, now.invoiceNo)
    if (targetIdx >= 0) {
      // 同文件级联：发票类型实质性变更时同步该文件下所有发票（概览卡按 category 精确匹配）
      cascadeInvoiceType(invoices, targetIdx, updated.invoice_type)
      invoices[targetIdx] = { ...invoices[targetIdx], ...updated, page: nowPage || invoices[targetIdx]?.page }
    }
    else invoices.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const { status: nextStatus, error: nextError } = extractStatusOf(hasInvoiceData(invoices))
    // 文件级归档分类同步为最新发票类型：列表平铺以文件级 fleet_cert_type 为优先，
    // 不同步会导致改类型后仍按旧分类展示
    const newMeta = {
      ...meta,
      kind: 'invoice',
      invoices,
      fleet_cert_type: editForm.value.invoice_type || '',
      extract_status: nextStatus,
      extract_error: nextError,
    }
    await updateBusinessMetadata(kbId.value, now.knowledgeId, newMeta, force)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑后列表立即刷新（shallowRef：整体替换数组触发更新）
    const listIdx = invoiceRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      invoiceRows.value = invoiceRows.value.map((r: any, i: number) =>
        (i === listIdx ? mergeRowAfterSave(r, updated, nextStatus, nextError) : r))
    }
    // 同文件级联后，同步列表中同一文件的其他行类型展示（loadFiles 随后全量刷新兜底）
    if (updated.invoice_type && now.knowledgeId) {
      const kid = now.knowledgeId
      invoiceRows.value = invoiceRows.value.map((r: any) =>
        r.knowledgeId === kid && String(r.invoiceType || '') !== updated.invoice_type
          ? { ...r, invoiceType: updated.invoice_type } : r)
    }
    // 手动保存：成功后关闭详情抽屉（对齐车队行内保存）
    detailVisible.value = false
    // 记录修改后自动刷新：列表汇总与概览卡片随最新数据同步
    loadFiles(true)
    loadInvoiceOverview()
  } catch (e: any) {
    if (e?.status === 409 && String(e?.message || '').startsWith('DUPLICATE_INVOICE_NO')) {
      // 单号重复：弹二次确认，确认后 force 重试；取消留在编辑态
      dupNo.value = String(e?.message || '').split(':').pop() || ''
      dupConfirmVisible.value = true
      return
    }
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    autoSaving.value = false
  }
}

// 抽屉底部动作：删除（按页/整份，走中台 listSync 的级联编排）与重新提取
const onDetailDelete = async () => {
  const r = currentRow.value
  if (!r) return
  try {
    await doPageDelete([r])
    await finishDelete()
    detailVisible.value = false
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}
const onDetailReExtract = async () => {
  const r = currentRow.value
  if (!r?.knowledgeId) return
  try {
    await extractBusinessDocument(kbId.value, r.knowledgeId, 'invoice', filterInvoiceType.value || undefined)
    MessagePlugin.success('已提交重新提取')
    detailVisible.value = false
    loadFiles(true)
  } catch (e: any) {
    if (!isExtractInProgress(e)) MessagePlugin.error(e?.message || '重新提取失败')
  }
}

// formatAmount / formatRate 已收入 businessFormat.ts（明细浮层中台组件共用同一口径）
// 发票内所有不同税率档（用于 tooltip 展示全部档位；列表展示金额最大项税率）
const rateVariants = (row: InvoiceRow): number[] => {
  const set = new Set<number>()
  for (const it of row.items || []) {
    const r = Number(it?.tax_rate)
    if (Number.isFinite(r) && r > 0) set.add(Math.round(r * 10000) / 10000)
  }
  if (set.size === 0) {
    const t = Number(row.taxRate)
    if (Number.isFinite(t) && t > 0) set.add(Math.round(t * 10000) / 10000)
  }
  return [...set]
}

// 税率列紧凑文本：多档税率以顿号拼接（13%、9%、6%），单档直接显示
const rateTextOf = (row: InvoiceRow): string => {
  const vs = rateVariants(row)
  return vs.length ? vs.map(formatRate).join('、') : formatRate(row.taxRate)
}

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

const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

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
    return `确定按最新提取规则重新提取「${name}」的所有发票字段吗？将覆盖已有提取结果。`
  }
  return `确定按最新提取规则重新提取选中的 ${ids.length} 个文件的所有发票字段吗？将覆盖已有提取结果。`
})

// 重新解析：物理文件重新拆页/解析（支持多文件批量，二次确认）
const reparseBusy = ref(false)
const handleReparse = async () => {
  const ids = selectedIds.value
  if (!ids.length) { MessagePlugin.info('请先选中要重新解析的发票'); return }
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
    loadInvoiceOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    reparseBusy.value = false
  }
}

// 重新提取：大模型/OCR 按最新提取规则重新提取选中文件全部发票字段（支持多文件批量，二次确认）
const extractBusy = ref(false)
const handleFileExtract = async () => {
  const all = selectedIds.value
  if (!all.length) { MessagePlugin.info('请先选中要重新提取的发票'); return }
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
    // 携带当前细分分类（用户正在该分类视图下操作），锚定同文件多页发票的类型，
    // 避免历史文件缺 fleet_cert_type 打标时被错误降级为「其它票据」。
    // selectedIds 已按 knowledgeId 去重：多选 N 条记录若同属一个文件，只对该文件重新提取一次。
    results = await settleInBatches(
      ids,
      BATCH_CONCURRENCY,
      kid => extractBusinessDocument(kbId.value, kid, 'invoice', filterInvoiceType.value || undefined),
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
    loadInvoiceOverview()
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

// 打印：把所有选中发票的页面合并为一个 PDF，单 iframe 预览打印
// （跨文件、同文件任意组合均支持；图片发票自动转 PDF 页；无页码记录回退整文件页）
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
  revokePrintUrl()
  printLoaded.value = false
  printCount.value = rows.length
  printVisible.value = true
  try {
    const out = await PDFDocument.create()
    const notes: string[] = []
    let mergedPages = 0
    for (const r of rows) {
      const blob: any = await previewKnowledgeFile(r.knowledgeId)
      if (!blob) { notes.push(`${r.invoiceNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式发票：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pageCount = pdf.getPageCount()
        // 物理页码优先：page_no（SourceLocators）才是 PDF 里的真实页，
        // page 是票号序号（同页两票/同票跨页时两者不等）。
        const pdfPage = pdfPageOf(r)
        if (pdfPage >= 1) {
          if (pdfPage <= pageCount) {
            const [pg] = await out.copyPages(pdf, [pdfPage - 1])
            out.addPage(pg)
            mergedPages++
          } else {
            notes.push(`${r.invoiceNo || r.fileName}（页码 ${pdfPage} 超出文件 ${pageCount} 页，已并入整文件）`)
            const pages = await out.copyPages(pdf, pdf.getPageIndices())
            pages.forEach(p => out.addPage(p))
            mergedPages += pages.length
          }
        } else {
          notes.push(`${r.invoiceNo || r.fileName}（无页码，已并入整文件）`)
          const pages = await out.copyPages(pdf, pdf.getPageIndices())
          pages.forEach(p => out.addPage(p))
          mergedPages += pages.length
        }
      } catch {
        notes.push(`${r.invoiceNo || r.fileName}（页抽取失败，已并入整文件）`)
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
      MessagePlugin.error('未获取到可打印的发票页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    revokePrintUrl()
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
// 抽屉「提取状态」只读中文化（与列表 statusOf 中文口径一致）

// 项目明细 → 多行文本（列表列宽 / tooltip / 打印共用）
const itemsTextOf = (row: InvoiceRow): string =>
  (row.items || []).map((it: any) => {
    const q = it.qty !== undefined && it.qty !== null && it.qty !== '' ? ` ×${it.qty}` : ''
    const p = it.price !== undefined && it.price !== null && it.price !== '' ? ` ${formatAmount(it.price)}` : ''
    return `${it.name || '—'}${q}${p}`
  }).join('\n')

// 字段配置开启「项目明细」后，抽屉才渲染明细编辑面板

const catalogValueOf = (row: InvoiceRow, key: string): string => {
  switch (key) {
    case 'invoiceNo': return row.invoiceNo || ''
    case 'invoiceDate': return row.invoiceDate || ''
    case 'invoiceType': return row.invoiceType || ''
    case 'amount': return formatAmount(row.amount)
    case 'taxRate': return rateVariants(row).length ? rateVariants(row).map(formatRate).join('、') : formatRate(row.taxRate)
    case 'tax': return formatAmount(row.tax)
    case 'totalAmount': return formatAmount(row.totalAmount)
    case 'buyerName': return row.buyerName || ''
    case 'sellerName': return row.sellerName || ''
    case 'issuer': return row.issuer || ''
    case 'remark': return row.remark || ''
    case 'extractStatus': { const s = statusOf(row).label; return s === '--' ? '' : s }
    case 'tags': return rowTags(row).map((t: any) => t.name).join('、')
    case 'sellerTaxNo': return row.sellerTaxNo || ''
    case 'buyerTaxNo': return row.buyerTaxNo || ''
    case 'items': return itemsTextOf(row)
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
      ...visibleColDefs.value.map((c: any) => ({ key: c.key, label: c.label, value: (r: any) => catalogValueOf(r, c.key) })),
    ]
    const bytes = await generateCatalogPdf({ title: '发票目录', columns: cols, rows })
    printMode.value = 'catalog'
    printCount.value = rows.length
    revokePrintUrl()
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
  a.download = `发票目录-${Date.now()}.pdf`
  a.click()
}

// 删除记录：有页码的行按页删除该发票（同文件其它发票保留）；无页码（历史数据）删整份文件
// 批量删除二选一：选中记录若来自多页文件集合，先让用户选择「仅删选中发票」或「整份删除该文件」；
// 仅删选中：按页删除（无页码的行等价整份删除该文件）；整份删除：对选中行涉及的文件整体软删并从上传历史移除。
// 删除编排走中台 useBusinessListSync：乐观移除 + 延迟对账 + 级联顺序（先大页码）
// 全部由它负责，本页只注入发票自己的删除接口与文案。
const listSync = useBusinessListSync(invoiceRows, {
  reload: (silent) => loadFiles(!!silent),
  reloadStats: () => loadInvoiceOverview(),
  deletePage: (kid, page) => deleteInvoicePage(kbId.value, kid, page),
  deleteFiles: (ids) => batchDeleteKnowledge(kbId.value, ids),
  onError: (e, ctx) => MessagePlugin.error(e?.message || (ctx.page ? `删除发票（${ctx.page}）失败` : '删除失败')),
})
const optimisticRemoveInvoiceRows = (fileIds: Set<string>, pageKeys: Set<string>) =>
  listSync.removeRows(fileIds, pageKeys)
const schedulePostDeleteRefresh = () => listSync.schedulePostDeleteRefresh()
const doPageDelete = (rows: InvoiceRow[]) => listSync.deletePages(rows)
const doFileDelete = (rows: InvoiceRow[]) => listSync.deleteFiles(rows)
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  selectedRowKeys.value = []
  // 异步删除：不立即 loadFiles（会把乐观移除的行又拉回来），交由延迟对账权威纠正
  schedulePostDeleteRefresh()
}
// 删除确认：气泡内二选一（多页文件「删除此页/删除文件」，否则普通确认），杜绝二次弹窗
const delPopVisible = ref(false)
const closeDelPop = () => { delPopVisible.value = false }
// 选中记录涉及的多页文件数量（同一上传文件含多张发票）
const delMultiCount = computed(() => {
  const set = new Set<string>()
  for (const r of selectedRows.value) {
    if (!r.knowledgeId) continue
    const count = fileInvoiceCounts.value.get(r.knowledgeId) || 0
    if (count > 1) set.add(r.knowledgeId)
  }
  return set.size
})
const confirmPageDelete = async () => {
  closeDelPop()
  try { await doPageDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}
const confirmFileDelete = async () => {
  closeDelPop()
  try { await doFileDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// 多选清除：先调用 t-table 实例 clearSelected（内部清空并 emit select-change 同步外部），
// 再兜底清空 selectedRowKeys，杜绝 TDesign 受控反向写回旧 key 导致浮条不消失
const clearSelectionSafe = () => {
  try { invoiceTableRef.value?.clearSelected?.() } catch { /* ignore */ }
  selectedRowKeys.value = []
}

// 六同步：字段/分类配置变更后重建动态列与概览统计（类型卡/下拉由 invoiceCats computed 自动派生）
const onCategoriesChanged = () => {
  reloadColumns(true)
  loadInvoiceOverview()
}

// ---- 表头右侧滚动条轨道遮罩（Q5）：宽度=真实滚动条宽度、高度=表头实测高度，
// 背景与表头 th 同令牌（container）→ 主题切换同步变色，无色差残留 ----
const headerFix = reactive({ sbw: 17, thh: 39 })
const measureTableHeaderFix = () => {
  const th = document.querySelector('.doc-list-view thead th')
  if (th) headerFix.thh = Math.max(30, Math.round(th.getBoundingClientRect().height))
  const d = document.createElement('div')
  d.style.cssText = 'width:100px;height:100px;overflow:scroll;position:absolute;top:-9999px;left:-9999px'
  document.body.appendChild(d)
  const w = d.offsetWidth - d.clientWidth
  d.remove()
  if (w > 0) headerFix.sbw = w
}

// ---- 生命周期 ----
onMounted(() => {
  // 路由 query 回填：type / failed 直达状态
  if (typeof route.query.type === 'string' && route.query.type) filterInvoiceType.value = route.query.type
  if (route.query.failed === '1') uploadHistoryVisible.value = true
  loadKb()
  reloadColumns()
  measureTableHeaderFix()
  // 六同步：设置页字段/分类保存后全局事件 -> 重建动态列/类型卡/下拉
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  listSync.destroy()
  revokePrintUrl()
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})

// 打印弹窗关闭时释放 Blob URL
watch(printVisible, (v) => {
  if (!v) revokePrintUrl()
})
</script>

<style scoped lang="less">
.invoice-management-container {
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

.invoice-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

/* ---- 概览紧凑卡（由中台组件 DashboardKpiGroup 渲染，样式内置组件） ---- */
.overview-group {
  margin-bottom: var(--td-comp-margin-m);
}

/* ---- 发票列表视图 ---- */
.invoice-list-view {
  display: flex; flex-direction: column; gap: 0;
  flex: 1; min-height: 0;
}
/* ---- 筛选工具栏 ---- */
/* 筛选控件样式随组件走：见 BusinessFilterBar.vue（.bfs-select / .bfs-date-range） */
/* 明细浮层样式随组件走：见 BusinessSubItemsPopover.vue（.biz-sub-popover） */


/* ---- 字段筛选弹层 ---- */
:global(.invoice-field-popup) {
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

/* 底部悬浮工具条 / 列内边距：统一走四模块共享样式文件（.biz-page 前缀） */

/* scrollbar-gutter:stable 保证滚动条占位宽度恒定，列与滚动条不错位 */
/* 表头右侧滚动条轨道遮罩：滚动条属于滚动容器 UI 层，绘制层级高于 sticky 表头，
   无法靠表头背景覆盖。在滚动容器父层(.doc-list-view)挂绝对定位色块，
   宽度=真实滚动条宽度(--invoice-sbw，JS 实测)、高度=表头实测高度(--invoice-thh)，
   盖住表头高内的轨道段 → 垂直滚动条仅在表体区域可见；
   底边框与 th 底边框同色同位 → 表头横线在滚动条区单线闭合（无缝隙、无双线）。
   背景与 th 同令牌(container)，深色主题下两者同步变化，不产生主题色差 */
.doc-list-view::after {
  content: '';
  position: absolute;
  top: 0;
  right: 0;
  z-index: 20;
  width: var(--invoice-sbw, 17px);
  height: var(--invoice-thh, 39px);
  box-sizing: border-box;
  pointer-events: none;
  background: var(--td-bg-color-container);
  border-bottom: 1px solid var(--td-component-stroke);
}

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

.cell-invoiceNo, .cell-buyerName, .cell-sellerName, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.row-invoice-no {
  min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font-size: 13px; font-weight: 600; color: var(--td-text-color-primary);
}
/* 文件级占位行徽章：源文件仍在库但未提取出票据，提示用户可重新提取/删除 */
.row-placeholder-badge {
  flex-shrink: 0; min-width: 0;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.row-page-badge {
  flex-shrink: 0; margin-left: 6px; height: 18px; padding: 0 6px; border-radius: 4px;
  background: var(--td-success-color-1, #e8f8f2); color: var(--td-success-color, #00a870);
  border: 1px solid var(--td-success-color-3, #94e3c5);
  font-size: 11px; font-weight: 600; line-height: 16px; box-sizing: border-box;
  white-space: nowrap;
}
.row-text { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-mono { font-variant-numeric: tabular-nums; font-size: 12px; color: var(--td-text-color-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
.row-amount { font-size: 13px; color: var(--td-text-color-primary); font-weight: 500; }
.row-muted { color: var(--td-text-color-disabled); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
/* 状态标签（.row-status-tag）与 .icon-spin 统一走四模块共享样式文件 */

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
.doc-load-end {
  text-align: center; padding: 10px 0; font-size: 12px; color: var(--td-text-color-placeholder);
}

/* ---- 详情抽屉：竖向区块 + 可拖宽（复用知识库文档抽屉上下滚动样式） ---- */
.invoice-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.invoice-detail-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

// 详情抽屉底部取消/保存（置于源文件预览下方，无分割线）
.invoice-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 4px 0 0;
  margin-top: 0;
}

// 标题页码标签：与列表行发票号码后的 P1/P2 标签一致（浅绿底深绿字）
.invoice-detail-drawer .setting-drawer__title { display: flex; align-items: center; }
.invoice-detail-drawer .setting-drawer__title .row-page-badge {
  display: inline-flex; align-items: center;
  font-size: inherit;
  height: 22px; line-height: 22px; padding: 0 10px; margin-left: 8px;
}

// 开票日期选择器与其它字段输入框对齐（同宽撑满 grid 单元格）
.invoice-detail-drawer .field-grid :deep(.t-date-picker) { width: 100%; }
.row-items { display: flex; flex-direction: column; gap: 2px; max-width: 100%; overflow: hidden; }
.row-items-line { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: flex; gap: 4px; align-items: baseline; color: var(--td-text-color-primary); }
.row-items-name { max-width: 120px; overflow: hidden; text-overflow: ellipsis; }
.row-items-qty, .row-items-price { color: var(--td-text-color-secondary); font-family: var(--td-font-family-mono, monospace); }
.row-items-more { color: var(--td-text-color-disabled); font-size: var(--td-font-size-body-small); }
.detail-tags-readonly { display: flex; flex-wrap: wrap; gap: 4px; }
.detail-preview-collapse { margin-top: 0; }
.detail-readonly {
  display: inline-flex;
  align-items: center;
  height: 32px;
  color: var(--td-text-color-primary);
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



.detail-fields { padding-bottom: 0; }

.field-group { margin-bottom: 4px; }
.field-group-title {
  display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 600;
  color: var(--td-text-color-primary); margin-bottom: 12px;
  &::before { content: ''; width: 3px; height: 14px; background: var(--td-brand-color); border-radius: 2px; flex-shrink: 0; }
}
.field-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); column-gap: 20px; row-gap: 12px; }
.field-grid--full { grid-template-columns: 1fr; }
.field-grid__full { grid-column: 1 / -1; }
.field-grid :deep(.t-form__item) { margin-bottom: 0; }

.items-section { border-top: 1px solid var(--td-component-stroke); padding-top: 14px; margin-top: 4px;
  .items-header { display: flex; align-items: center; justify-content: flex-end; margin-bottom: 10px; }
}
// 明细行按分类 sub_fields 动态渲染（通行费/电费列不同），flex 布局 + 固定列宽类，
// 表头与数据行共用同一组列宽类保证强对齐；列数多时横向滚动而非挤压
.items-table { overflow-x: auto; }
.items-row { display: flex; gap: 8px; align-items: center; margin-bottom: 8px;
  &--head { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
}
.item-cell { min-width: 0; }
.item-cell--name { width: 140px; flex-shrink: 0; }
.item-cell--text { width: 100px; flex-shrink: 0; }
.item-cell--num { width: 90px; flex-shrink: 0; }
.item-cell--date { width: 118px; flex-shrink: 0; }
.item-op { width: 44px; flex-shrink: 0; display: flex; justify-content: center; }
.items-empty { font-size: 13px; color: var(--td-text-color-placeholder); padding: 12px 0; }
.detail-save-hint { display: flex; align-items: center; gap: 6px; margin-top: 16px; font-size: 12px; color: var(--td-text-color-placeholder); }

/* ---- 打印预览（合并单个 PDF，单 iframe 全高预览；白色背景、页脚固定） ---- */
.invoice-print-dialog :deep(.t-dialog) {
  display: flex;
  flex-direction: column;
  height: 82vh !important;
  max-height: 92vh !important;
  background: var(--td-bg-color-container);
}
.invoice-print-dialog :deep(.t-dialog__wrap) {
  align-items: center;
}
.invoice-print-dialog :deep(.t-dialog__header) { color: var(--td-text-color-primary); }
.invoice-print-dialog :deep(.t-dialog__body) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 8px 24px 0;
  background: var(--td-bg-color-container);
}
.invoice-print-dialog :deep(.t-dialog__footer) {
  padding: 12px 24px;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
.print-preview-area {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  .print-preview-frame {
    width: 100%;
    flex: 1;
    min-height: 0;
    border: 1px solid var(--td-component-stroke);
    border-radius: 6px;
    background: var(--td-bg-color-container);
  }
  .print-preview-loading {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
    font-size: 14px;
  }
}
.print-preview-footer { display: flex; justify-content: center; gap: 8px; }
</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.invoice-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.invoice-print-dialog {
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
.invoice-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: var(--td-bg-color-container);
}
.invoice-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.invoice-print-close {
  flex-shrink: 0;
}
.invoice-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.invoice-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.invoice-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.invoice-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}


.del-pop-body {
  min-width: 200px;
  .del-pop-title { font-size: 13px; color: var(--td-text-color-primary); line-height: 1.5; margin-bottom: 12px; }
  .del-pop-actions { display: flex; justify-content: flex-end; align-items: center; gap: 8px; }
}
</style>
