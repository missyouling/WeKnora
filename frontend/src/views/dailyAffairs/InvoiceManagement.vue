<template>
  <div class="invoice-management-container">
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
    <div v-else class="invoice-main">
      <!-- 概览紧凑卡（中台 DashboardKpiGroup 组件，type-card 范式） -->
      <div class="overview-group">
        <DashboardKpiGroup :cards="overviewCards" @card-click="onOverviewCardClick" />
      </div>

      <!-- 发票列表 -->
      <div class="invoice-list-view">
      <!-- 筛选工具栏（对齐车队标准中台组件） -->
          <BusinessListToolbar v-model:keyword="keyword" search-placeholder="搜索全部字段"
            @refresh="applyFilter" :selected-count="selectedRowKeys.length"
            @clear-selection="clearSelection" hide-batch-bar>
            <template #type-extra>
              <div class="doc-filter-field">
                <t-select v-model="taxRateFilter" :options="taxRateOptions" placeholder="税率筛选" clearable
                  class="doc-tax-select doc-filter-field__control">
                  <template #prefixIcon><t-icon name="percent" size="16px" /></template>
                </t-select>
              </div>
              <t-date-range-picker v-model="dateRange" placeholder="开票日期" clearable allow-input @change="applyFilter"
                class="invoice-date-range">
                <template #prefixIcon><t-icon name="time" size="16px" /></template>
              </t-date-range-picker>
            </template>
            <template #columns>
              <BusinessColumnFilter :columns="effectiveColumns" v-model:visibleKeys="visibleColKeys" @reset="resetColumns" @select-all="selectAllColumns" />
            </template>
            <template #right-extra>
              <t-button variant="outline" size="small" @click="invoiceSettingsVisible = true">
                <template #icon><t-icon name="setting" size="14px" /></template>
                设置
              </t-button>
              <t-button theme="primary" size="small" @click="uploadVisible = true">
                <template #icon><t-icon name="upload" size="14px" /></template>
                上传发票
              </t-button>
            </template>
          </BusinessListToolbar>

      <!-- 发票列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
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
        @row-click="({ row, e }: any) => onRowClick(row, e)"
        @select-change="onTableSelectChange"
        @sort-change="onSortChange"
      >
        <template #invoiceNo="{ row }: any">
          <span class="row-invoice-no" :title="row.invoiceNo || row.fileName">{{ row.invoiceNo || row.fileName }}</span>
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
          <div v-if="rateVariants(row).length > 1" class="row-rate-tags">
            <t-tag v-for="r in rateVariants(row)" :key="r" size="small" variant="outline" theme="default"
              class="row-rate-tag">{{ formatRate(r) }}</t-tag>
          </div>
          <span v-else class="row-mono row-rate">{{ formatRate(row.taxRate) || '-' }}</span>
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
              <div v-for="(it, i) in (row.items || []).slice(0, 3)" :key="i" class="row-items-line">
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
              <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handlePageExtract">
                <template #icon><t-icon name="refresh" size="14px" /></template>
                页面提取
              </t-button>
              <t-popconfirm theme="warning"
                :content="`确定重新解析并提取「${selectedSingle?.fileName || '该文件'}」的所有页面发票吗？将覆盖已有提取结果。`"
                :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleFileExtract">
                <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                  <template #icon><t-icon name="refresh" size="14px" /></template>
                  单文件提取
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
      :type-options="uploadTypeOptions" :default-type="filterInvoiceType || ''" @done="onUploadDone" />

    <!-- 发票详情抽屉（三 tab，可拖宽，竖向滚动） -->
<SettingDrawer v-model:visible="detailVisible" :title="detailTitle" width="654px" :storage-key="'weknora-invoice-drawer-width'" hide-footer destroy-on-close class="invoice-detail-drawer">
      <div class="invoice-detail-body">
        <!-- 发票字段（按分类配置动态渲染，字段来自设置-字段定义 subs） -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">{{ editForm.invoice_type || '发票信息' }}</div>
                <div class="field-grid">
                  <template v-for="f in editFieldDefs" :key="f.name">
                    <t-form-item v-if="f.name === 'invoiceType'" label="发票类型" label-width="110px">
                      <t-select v-model="editForm.invoice_type" :options="invoiceTypeOptions" clearable allow-create
                        filterable placeholder="选择或输入类型" />
                    </t-form-item>
                    <t-form-item v-else-if="f.name === 'extractStatus'" label="提取状态" label-width="110px">
                      <span class="detail-readonly">{{ invoiceStatusText(editForm.data.extractStatus) }}</span>
                    </t-form-item>
                    <t-form-item v-else-if="f.name === 'tags'" label="标签" label-width="110px">
                      <div v-if="String(editForm.data.tags || '').trim()" class="detail-tags-readonly">
                        <t-tag v-for="t in String(editForm.data.tags).split('\n').map((x: string) => x.trim()).filter(Boolean)" :key="t"
                          size="small" variant="light-outline" class="row-tag">
                          {{ t }}
                        </t-tag>
                      </div>
                      <span v-else class="row-muted">—</span>
                    </t-form-item>
                    <t-form-item v-else-if="f.remarkPart" :label="f.label" label-width="110px" :class="{ 'field-grid__full': f.dataType === 'items' || f.dataType === 'array' }">
                      <t-textarea v-if="f.dataType === 'items' || f.dataType === 'array'" v-model="editForm.remarkParts[f.remarkPart]"
                        :autosize="{ minRows: 2, maxRows: 5 }" placeholder="每行一条" />
                      <t-input v-else v-model="editForm.remarkParts[f.remarkPart]" placeholder="" />
                    </t-form-item>
                    <t-form-item v-else-if="f.name !== 'items'" :label="f.label" label-width="110px" :class="{ 'field-grid__full': f.dataType === 'array' }">
                      <t-date-picker v-if="f.dataType === 'date'" v-model="editForm.data[f.name]" value-type="YYYY-MM-DD"
                        format="YYYY-MM-DD" clearable allow-input />
                      <t-input v-else-if="f.dataType === 'number'" v-model="editForm.data[f.name]"
                        @input="(v: string) => (editForm.data[f.name] = sanitizeNum(v))" />
                      <t-textarea v-else-if="f.dataType === 'array'" v-model="editForm.data[f.name]"
                        :autosize="{ minRows: 2, maxRows: 5 }" placeholder="每行一条" />
                      <t-input v-else v-model="editForm.data[f.name]" />
                    </t-form-item>
                  </template>
                </div>
              </div>
              <!-- 明细 items（发票特有业务；字段配置开启 items 时显示） -->
              <div v-if="hasItemsField" class="items-section">
                <div class="items-header">
                  <t-button size="small" variant="outline" @click="addItemRow">
                    <template #icon><t-icon name="add" size="14px" /></template>
                    添加明细
                  </t-button>
                </div>
                <div v-if="(editForm.items || []).length" class="items-table">
                  <div class="items-row items-row--head">
                    <span class="item-cell item-name">项目名称</span>
                    <span class="item-cell item-num">数量</span>
                    <span class="item-cell item-num">单价</span>
                    <span class="item-cell item-num">税率</span>
                    <span class="item-cell item-op"></span>
                  </div>
                  <div v-for="(it, idx) in editForm.items || []" :key="idx" class="items-row">
                    <t-input v-model="it.name" size="small" class="item-cell item-name" placeholder="" />
                    <t-input :value="numToStr(it.qty)" size="small" class="item-cell item-num" placeholder=""
                      @input="(v: string) => (it.qty = parseNum(v))" />
                    <t-input :value="numToStr(it.price)" size="small" class="item-cell item-num" placeholder=""
                      @input="(v: string) => (it.price = parseNum(v))" />
                    <t-input :value="it.tax_rate" size="small" class="item-cell item-num" placeholder="" suffix="%"
                      @input="(v: string) => (it.tax_rate = sanitizeNum(v))" />
                    <div class="item-cell item-op">
                      <t-popconfirm theme="warning" :content="`确定删除明细「${it.name || '未命名'}」吗？删除后需保存才生效。`"
                        :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                        @confirm="removeItemRow(Number(idx))">
                        <t-button variant="text" theme="danger" size="small" @click.stop>
                          <t-icon name="delete" />
                        </t-button>
                      </t-popconfirm>
                    </div>
                  </div>
                </div>
                <div v-else class="items-empty">暂无明细</div>
              </div>


            </div>
          </div>
        </section>

        <!-- 保存/取消前置到表单底部（源文件预览上方），无需滚过 PDF 预览即可保存 -->
        <div class="invoice-detail-footer">
          <t-button variant="outline" size="small" @click="detailVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="autoSaving" @click="saveEditForm">保存</t-button>
        </div>

        <!-- 源文件预览：默认折叠；折叠时不挂载 DocumentPreview，避免 PDF/图片渲染器初始化 -->
        <t-collapse v-model="previewExpanded" class="detail-preview-collapse">
          <t-collapse-panel value="preview" header="源文件预览">
            <div class="detail-block-content">
              <DocumentPreview v-if="currentRow && previewExpanded.length" :knowledge-id="currentRow.knowledgeId"
                :file-type="currentRow?.fileType || ''" :file-name="currentRow?.fileName || ''" :active="true" />
            </div>
          </t-collapse-panel>
        </t-collapse>
      </div>
    </SettingDrawer>

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

<script setup lang="ts">
import { ref, shallowRef, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listFleetCategories } from '@/api/fleet'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { colWidthOf } from './columnWidth'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  delKnowledgeDetails,
  batchDeleteKnowledge,
  updateInvoiceMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractBusinessDocument,
  extractInvoicePage,
  deleteInvoicePage,
  listInvoiceRecords,
  listInvoiceTaxRates,
  getInvoiceOverviewStats,
  previewKnowledgeFile,
  getRecognitionConfig,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import BusinessColumnFilter from './BusinessColumnFilter.vue'
import BusinessListToolbar from './BusinessListToolbar.vue'
import { useBusinessPolling } from '@/composables/useBusinessPolling'

const KB_NAME = '日常事务-发票'
const PAGE_SIZE = 20

// 发票类型枚举（编辑/筛选推荐值，allow-create 兼容提取值）
const INVOICE_TYPES = ['专用发票', '普通发票', '医疗收据', '财政收据', '其它票据']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
import { INVOICE_FIELD_LABELS, invoiceFieldKeyOf, isRemarkPartField, remarkPartOf, setRemarkPart } from './invoiceFieldLabels'
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
  resetColumns, selectAllColumns, colVisible,
  selectedRowKeys, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-invoice-list-columns',
  defaultColumns: DEFAULT_INVOICE_COLUMNS,
})

const invoiceTableRef = ref<any>(null)

const route = useRoute()
const router = useRouter()

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
  items?: Array<{ name?: string; qty?: number; price?: number; tax_rate?: number }>
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
  invoiceNo?: string
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
  page?: number
  taxRate?: number
  multiIndex?: string
  tags?: any[]
  description?: string
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
const loadInvoiceOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getInvoiceOverviewStats(kbId.value)
    overviewStats.value = res?.data || null
  } catch (e: any) {
    overviewStats.value = null
  } finally {
    overviewLoading.value = false
  }
}
const overviewCards = computed(() => {
  const st = overviewStats.value || {}
  const m = st.current_month || {}
  const total = Number(st.total || 0)
  const parseFailed = Number(st.parse_failed || 0)
  const extractFailed = Number(st.extract_failed || 0)
  const failed = parseFailed + extractFailed
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
  // 仅当确实存在这类记录时（数量 > 0）才显示卡片；点击筛选未分类记录，可编辑后转移分类
  const otherCount = Math.max(0, total - catSum)
  if (otherCount > 0) {
    cards.push({ key: 'type-其它票据', label: '其它票据', icon: 'file-unknown', value: `${otherCount}`, unit: '张', sub: '', cls: 'is-type', action: 'type', typeValue: '其它票据' })
  }
  // 历史记录卡：显示已上传文件数（含解析失败文件）；点击打开已上传文件抽屉
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: `${fileCount}`, unit: '份', sub: failed ? `${failed} 份异常` : '无异常文件', cls: failed > 0 ? 'is-warn' : '', action: 'history' })
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
      if (c.key === 'extractStatus') { const s = statusOf(r).label; return s === '--' ? '' : s }
      return colValue(r, c.key)
    }))
    const base: any = i === vis.length - 1
      ? { colKey: c.key, title: c.label, ellipsis: true, minWidth: w }
      : { colKey: c.key, title: c.label, ellipsis: true, width: w }
    // 无标准存储位字段（收款方式/收款事由/审核…）：数据存于 remark「字段名：值」片段，按片段渲染
    // TDesign cell 签名为 (h, { row })，第一参是 createElement，row 在第二参 props 中
    if (rp) {
      base.cell = (_h: any, { row }: any) => remarkPartOf(row?.remark || '', rp) || '-'
    } else if (['amount', 'tax', 'totalAmount'].includes(c.key)) {
      // 金额类字段统一格式化（千分位 + 两位小数），消除自定义分类 colKey 不匹配
      // #amount 插槽导致的显示差异；所有分类风格一致。
      base.cell = (_h: any, { row }: any) => {
        const v = colValue(row, c.key)
        return v === '-' ? '-' : formatAmount(Number(v))
      }
    } else if (c.key === 'taxRate') {
      base.cell = (_h: any, { row }: any) => {
        const v = colValue(row, c.key)
        return v === '-' ? '-' : formatRate(Number(v))
      }
    }
    cols.push(base)
  })
  return cols
})

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
const editForm = ref<Record<string, any>>({ invoice_no: '', invoice_type: '', items: [], data: {}, remarkParts: {} })
const autoSaving = ref(false)

// 详情抽屉动态字段：按当前发票类型匹配分类 subs（启用字段优先），无配置时回退内置列
// 字段 name 一律归一到英文契约键（自定义中文字段名经 invoiceFieldKeyOf 映射），
// 保证表单读写与保存写库都落在标准字段上；无标准存储位的字段（remark 片段）单独标记 remarkPart
const editFieldDefs = computed(() => {
  const t = editForm.value.invoice_type
  const cats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const hit = cats.find((c: any) => c.name === t) || cats[0]
  const subs = hit?.subs
  if (Array.isArray(subs) && subs.length) {
    return subs
      .filter((x: any) => x.enabled !== false)
      .map((x: any) => {
        const remarkPart = isRemarkPartField(String(x.name)) ? String(x.name) : ''
        return {
          name: remarkPart ? String(x.name) : invoiceFieldKeyOf(String(x.name)),
          label: INVOICE_FIELD_LABELS[String(x.name)] || String(x.name),
          dataType: x.data_type || 'text',
          remarkPart,
        }
      })
  }
  return DEFAULT_INVOICE_COLUMNS.map((c) => ({
    name: c.key,
    label: c.label,
    dataType: c.key === 'invoiceDate' ? 'date' : ['amount', 'tax', 'totalAmount', 'taxRate'].includes(c.key) ? 'number' : 'text',
    remarkPart: '',
  }))
})

// 抽屉宽度（可拖动，localStorage 记忆）
const DRAWER_WIDTH_KEY = 'weknora-invoice-drawer-width'
// 打印预览
const printVisible = ref(false)
const uploadHistoryVisible = ref(false)
const invoiceSettingsVisible = ref(false)
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
    const existing = new Set(invoiceRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapInvoiceRecord).filter(r => !existing.has(r.rowKey))
    invoiceRows.value = [...invoiceRows.value, ...fresh]
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
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 发票级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

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
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端发票级接口完成，前端直接使用返回的分页数据；
// 顶部合并"进行中文件"状态行（解析中/提取中/待提取），提取完成即消失
const pendingRows = computed<InvoiceRow[]>(() => pendingFiles.value.map((k) => {
  const ps = k.parse_status || ''
  const es = k.custom_metadata?.extract_status || ''
  let extractStatus = ''
  if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') extractStatus = 'parsing'
  else if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) extractStatus = 'pending'
  return {
    rowKey: `pending-${k.id}`,
    knowledgeId: k.id,
    fileName: k.file_name || k.title || k.id,
    parseStatus: ps,
    extractStatus,
    extractError: k.custom_metadata?.extract_error || '',
    kind: 'pending',
    tags: Array.isArray(k.tags) ? k.tags : [],
  } as InvoiceRow
}))
const filteredRows = computed(() => [...pendingRows.value, ...invoiceRows.value])

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
  if (count <= 1) return ''
  if (row.page && row.page >= 1) return `P${row.page}`
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
  return base.map((v: string) => ({ value: v, label: v }))
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
            label: INVOICE_FIELD_LABELS[String(s.name)] || String(s.name),
            default: isDefault,
            w: builtin?.w || '1fr',
            width,
            remarkPart,
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
  } catch { /* 后端未配置时回退内置默认 */ }
}

// 搜索态：关键词非空时类型下拉可临时清空（全局搜索优先，见 ⑦）
const searchActive = computed(() => !!keyword.value.trim())
// 类型筛选锁定：始终保证有一项分类被选中（禁清空）；搜索态放行（由 keyword watch 负责恢复）。
// 优先保留当前合法选中项（含路由 query 回填），否则按分类顺序选中第一个启用分类。
const ensureInvoiceType = () => {
  if (searchActive.value) return
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
const isAllSelected = computed(() =>
  selectableRows.value.length > 0 && selectableRows.value.every(r => selectedRowKeys.value.includes(r.rowKey))
)
const someSelected = computed(() => {
  const sel = selectableRows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)).length
  return sel > 0 && sel < selectableRows.value.length
})
const toggleSelectAll = (checked: boolean) => {
  selectedRowKeys.value = checked ? selectableRows.value.map(r => r.rowKey) : []
}
const toggleRow = (rowKey: string, checked: boolean) => {
  if (checked) {
    if (!selectedRowKeys.value.includes(rowKey)) selectedRowKeys.value = [...selectedRowKeys.value, rowKey]
  } else {
    selectedRowKeys.value = selectedRowKeys.value.filter(k => k !== rowKey)
  }
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
const onKeywordChange = () => { loadFiles(true) }

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
  clearSelectionSafe()
  reloadColumns()
  applyFilter()
})
// 税率筛选联动：变更时清空多选并刷新（税率不影响列定义，无需 reloadColumns）
watch(taxRateFilter, () => {
  clearSelectionSafe()
  applyFilter()
})
// 关键词防抖 300ms 后刷新
// ⑦ 全局搜索：输入关键词时临时清空类型筛选（下拉与请求参数均不带筛选），
// 清空搜索词后恢复原选中类型并重新锁定。
let keywordTimer: ReturnType<typeof setTimeout> | null = null
let searchRestoreType = ''
watch(keyword, (v) => {
  if (v && v.trim()) {
    if (searchRestoreType === '' && filterInvoiceType.value) searchRestoreType = filterInvoiceType.value
    if (filterInvoiceType.value) filterInvoiceType.value = ''
  } else {
    if (searchRestoreType) {
      filterInvoiceType.value = searchRestoreType
      searchRestoreType = ''
    } else {
      ensureInvoiceType()
    }
  }
  if (keywordTimer) clearTimeout(keywordTimer)
  keywordTimer = setTimeout(() => loadFiles(true), 300)
})

// 懒加载
const listScrollRef = ref<HTMLElement>()
const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
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
// 上传历史抽屉内容变化（删除/重新提取等）：列表与概览卡片一并刷新
const onUploadHistoryChanged = () => { loadFiles(true); loadInvoiceOverview() }

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'invoice'),
  onPendingFiles: (files) => {
    const withRows = new Set(invoiceRows.value.map(r => r.knowledgeId))
    pendingFiles.value = files.filter(k => !withRows.has(k.id))
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是电子发票，已保留待人工复核`)
    loadTaxRates()
  },
  onTick: () => loadFiles(),
})

const { extractStatusOf, statusOf, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'invoice', notLabel: '非发票',
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
          currentRow.value = { ...row, ...match }
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
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  const tags = Array.isArray(r.tags)
    ? r.tags.map((t: any) => (typeof t === 'string' ? t : t?.name ?? t)).join('\n')
    : (r.tags || '')
  // 分类优先：行级 category（细分分类如「三联收据」）> 提取类型 > 当前列表筛选类型，
  // 保证自定义分类记录按自己配置的字段渲染，而非回退到普通发票
  const targetType = r.invoiceType || r.category || filterInvoiceType.value || ''
  const cats = (invoiceCats.value || []).filter((c: any) => c.enabled !== false)
  const hitCat = cats.find((c: any) => c.name === targetType) || cats[0]
  // 无标准存储位字段（收款方式/收款事由/审核…）：从 remark「字段名：值」片段提取各字段值
  const rps: Record<string, string> = {}
  for (const s of hitCat?.subs || []) {
    if (isRemarkPartField(String(s.name))) rps[String(s.name)] = remarkPartOf(r.remark || '', String(s.name))
  }
  editForm.value = {
    invoice_no: r.invoiceNo || '',
    invoice_type: targetType,
    items: Array.isArray(r.items) ? r.items.map(it => ({
      name: it.name || '',
      qty: numToStr(it.qty),
      price: numToStr(it.price),
      // 税率以百分比显示（0.03 → "3"），保存时再转回小数
      tax_rate: it.tax_rate === null || it.tax_rate === undefined ? '' : String(Number(it.tax_rate) * 100),
    })) : [],
    remarkParts: rps,
    data: {
      invoiceNo: r.invoiceNo || '',
      invoiceDate: r.invoiceDate || '',
      invoiceType: r.invoiceType || '',
      amount: numToStr(r.amount),
      taxRate: numToStr(r.taxRate),
      tax: numToStr(r.tax),
      totalAmount: numToStr(r.totalAmount),
      buyerName: r.buyerName || '',
      sellerName: r.sellerName || '',
      issuer: r.issuer || '',
      remark: r.remark || '',
      extractStatus: r.extractStatus || '',
      tags,
      sellerTaxNo: r.sellerTaxNo || '',
      buyerTaxNo: r.buyerTaxNo || '',
      fileName: r.fileName || '',
    },
  }
}

const addItemRow = () => {
  editForm.value.items = [...(editForm.value.items || []), { name: '', qty: '', price: '', tax_rate: '' }]
}
const removeItemRow = (idx: number) => {
  editForm.value.items = (editForm.value.items || []).filter((_: any, i: number) => i !== idx)
}

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

// 源文件预览折叠面板（默认折叠，展开时才挂载 DocumentPreview）
const previewExpanded = ref<string[]>([])

const persistEditForm = async (force: boolean) => {
  const now = currentRow.value
  if (!now) return
  autoSaving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const invoices = Array.isArray(meta.invoices) ? [...meta.invoices] : []
    const d = editForm.value.data || {}
    // 无标准存储位字段（收款方式/收款事由/审核…）：各片段合并回 remark 后再整体写入
    const rps = (editForm.value.remarkParts || {}) as Record<string, string>
    for (const [part, val] of Object.entries(rps)) d.remark = setRemarkPart(d.remark, part, val)
    const updated: any = {
      invoice_no: d.invoiceNo || '',
      invoice_date: d.invoiceDate || '',
      invoice_type: editForm.value.invoice_type || '',
      // 归档分类与发票类型联动：列表筛选/概览卡按 category 精确匹配（本轮统一口径），
      // 保存时必须同步，否则改类型后记录会从对应分类视图消失
      category: editForm.value.invoice_type || '',
      total_amount: toNumber(d.totalAmount),
      amount: toNumber(d.amount),
      tax: toNumber(d.tax),
      tax_rate: toNumber(d.taxRate),
      seller_name: d.sellerName || '',
      seller_tax_no: d.sellerTaxNo || '',
      buyer_name: d.buyerName || '',
      buyer_tax_no: d.buyerTaxNo || '',
      issuer: d.issuer || '',
      remark: d.remark || '',
      tags: Array.isArray(d.tags) ? d.tags : (d.tags ? String(d.tags).split('\n').map((x: string) => x.trim()).filter(Boolean) : []),
      items: Array.isArray(editForm.value.items)
        ? editForm.value.items.map((it: any) => {
            const taxRate = toNumber(it.tax_rate)
            return {
              name: it.name || '',
              qty: toNumber(it.qty),
              price: toNumber(it.price),
              // 税率由百分比字符串转回小数（"3" → 0.03）
              tax_rate: taxRate === undefined ? undefined : taxRate / 100,
            }
          })
        : [],
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = invoices.findIndex((inv: any, i: number) =>
        i === nowPage - 1 || Number(inv.page) === nowPage || inv.invoice_no === now.invoiceNo)
    }
    if (targetIdx < 0) targetIdx = invoices.findIndex((inv: any) => inv.invoice_no === now.invoiceNo)
    if (targetIdx >= 0) {
      const prevType = String((invoices[targetIdx] as any)?.invoice_type || '')
      const nextType = String(updated.invoice_type || '')
      // 同文件级联：发票类型实质性变更时，将该文件（同一 knowledge 的 invoices 数组）下
      // 所有发票的类型同步为最新值（对齐标签行级同步逻辑；保存整包写回天然原子）。
      // category 与 invoice_type 联动更新，保证概览卡（category 精确）与列表口径一致。
      if (nextType && nextType !== prevType) {
        invoices.forEach((inv: any, i: number) => {
          if (i !== targetIdx) {
            inv.invoice_type = nextType
            inv.category = nextType
          }
        })
      }
      invoices[targetIdx] = { ...invoices[targetIdx], ...updated, page: nowPage || invoices[targetIdx]?.page }
    }
    else invoices.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const hasInvoiceData = invoices.some((inv: any) =>
      (inv.invoice_no || '').trim() || (inv.invoice_date || '').trim() || (inv.invoice_type || '').trim() ||
      (inv.buyer_name || '').trim() || (inv.seller_name || '').trim() ||
      Number(inv.amount) > 0 || Number(inv.total_amount) > 0)
    const nextStatus = hasInvoiceData ? 'success' : 'manual'
    const nextError = hasInvoiceData ? '' : '人工入库待编辑，请补充字段'
    // 文件级归档分类同步为最新发票类型：列表平铺以文件级 fleet_cert_type 为优先
    // （见后端 ListInvoiceRecords cat 取值），不同步会导致改类型后仍按旧分类展示
    const newMeta = {
      ...meta,
      kind: 'invoice',
      invoices,
      fleet_cert_type: editForm.value.invoice_type || '',
      extract_status: nextStatus,
      extract_error: nextError,
    }
    await updateInvoiceMetadata(kbId.value, now.knowledgeId, newMeta, force)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑（如备注）后列表立即刷新（shallowRef：整体替换数组触发更新）
    const listIdx = invoiceRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      const nextRow: any = {
        ...invoiceRows.value[listIdx],
        extractStatus: nextStatus,
        extractError: nextError,
        invoiceNo: updated.invoice_no,
        invoiceDate: updated.invoice_date,
        invoiceType: updated.invoice_type,
        amount: updated.amount,
        tax: updated.tax,
        totalAmount: updated.total_amount,
        buyerName: updated.buyer_name,
        sellerName: updated.seller_name,
        issuer: updated.issuer,
        remark: updated.remark,
        voidFlag: !!invoiceRows.value[listIdx].voidFlag,
        items: updated.items,
      }
      invoiceRows.value = invoiceRows.value.map((r: any, i: number) => (i === listIdx ? nextRow : r))
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
    // 记录修改后自动刷新：列表汇总与概览卡片（金额/类型/历史记录数）随最新数据同步
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

const toNumber = (v: any): number | undefined => {
  if (v === '' || v === null || v === undefined) return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 数字输入辅助：只允许数字与小数点，按实际输入显示（不补零、不保留多余小数）
const sanitizeNum = (v: string): string => {
  let s = (v ?? '').replace(/[^\d.]/g, '')
  const firstDot = s.indexOf('.')
  if (firstDot >= 0) s = s.slice(0, firstDot + 1) + s.slice(firstDot + 1).replace(/\./g, '')
  return s
}
const parseNum = (v: string): number | null => {
  const s = sanitizeNum(v)
  if (s === '' || s === '.') return null
  const n = Number(s)
  return Number.isFinite(n) ? n : null
}
const numToStr = (v: any): string =>
  v === null || v === undefined || v === '' ? '' : String(v)

const formatAmount = (v?: number) =>
  v === undefined || v === null ? '' : Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })

// 税率以百分比显示且不保留小数（0.03 → 3%）
const formatRate = (v?: number) => {
  if (v === undefined || v === null) return ''
  return `${Math.round(Number(v) * 100)}%`
}

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

// ---- 浮动工具栏 ----
const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

// 页面提取：只重新提取选中行所在页（单选）
const handlePageExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('页面提取仅支持单选，请选中一张发票'); return }
  if (!row.page || row.page < 1) {
    MessagePlugin.warning('该发票暂无页码信息，请先执行「单文件提取」')
    return
  }
  extractInFlight.value.add(row.knowledgeId)
  try {
    await extractInvoicePage(kbId.value, row.knowledgeId, row.page)
    MessagePlugin.success(`已重新提取第 ${row.page} 张发票`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '页面提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
  }
}

// 单文件提取：重新解析并提取选中行对应源文件的全部页面（单选，二次确认）
const handleFileExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('单文件提取仅支持单选，请选中一张发票'); return }
  extractInFlight.value.add(row.knowledgeId)
  extractFailed.value.delete(row.knowledgeId)
  try {
    await extractBusinessDocument(kbId.value, row.knowledgeId, 'invoice')
    MessagePlugin.success(`已触发「${row.fileName}」全量重新提取`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    extractFailed.value.add(row.knowledgeId)
    MessagePlugin.error(e?.message || '单文件提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
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
        if (r.page && r.page >= 1) {
          if (r.page <= pageCount) {
            const [pg] = await out.copyPages(pdf, [r.page - 1])
            out.addPage(pg)
            mergedPages++
          } else {
            notes.push(`${r.invoiceNo || r.fileName}（页码 ${r.page} 超出文件 ${pageCount} 页，已并入整文件）`)
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
const invoiceStatusText = (s: string): string =>
  ({ success: '提取成功', parse_failed: '解析失败', extract_failed: '提取失败', not_invoice: '非发票文件', manual: '待补录' } as Record<string, string>)[s] || s || '—'

// 项目明细 → 多行文本（列表列宽 / tooltip / 打印共用）
const itemsTextOf = (row: InvoiceRow): string =>
  (row.items || []).map((it: any) => {
    const q = it.qty !== undefined && it.qty !== null && it.qty !== '' ? ` ×${it.qty}` : ''
    const p = it.price !== undefined && it.price !== null && it.price !== '' ? ` ${formatAmount(it.price)}` : ''
    return `${it.name || '—'}${q}${p}`
  }).join('\n')

// 字段配置开启「项目明细」后，抽屉才渲染明细编辑面板
const hasItemsField = computed(() => editFieldDefs.value.some((f: any) => f.name === 'items'))

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
  a.download = `发票目录-${Date.now()}.pdf`
  a.click()
}

// 删除记录：有页码的行按页删除该发票（同文件其它发票保留）；无页码（历史数据）删整份文件
// 批量删除二选一：选中记录若来自多页文件集合，先让用户选择「仅删选中发票」或「整份删除该文件」；
// 仅删选中：按页删除（无页码的行等价整份删除该文件）；整份删除：对选中行涉及的文件整体软删并从上传历史移除。
const doPageDelete = async (rows: InvoiceRow[]) => {
  const fileDeleteIds = new Set<string>()
  const pageDeleteIds: Array<{ knowledgeId: string; page: number }> = []
  for (const r of rows) {
    if (r.page && r.page >= 1) pageDeleteIds.push({ knowledgeId: r.knowledgeId, page: r.page })
    else fileDeleteIds.add(r.knowledgeId)
  }
  // 同文件多页删除时，先删大页码再删小页码（后端删后重排页码）
  pageDeleteIds.sort((a, b) => b.page - a.page)
  for (const pd of pageDeleteIds) {
    try {
      const res: any = await deleteInvoicePage(kbId.value, pd.knowledgeId, pd.page)
      if (res?.deleted_file) fileDeleteIds.add(pd.knowledgeId)
    } catch (e: any) {
      MessagePlugin.error(e?.message || `删除发票（${pd.page}）失败`)
      return
    }
  }
  if (fileDeleteIds.size) {
    await batchDeleteKnowledge(kbId.value, Array.from(fileDeleteIds))
  }
}
const doFileDelete = async (rows: InvoiceRow[]) => {
  const fileIds = Array.from(new Set(rows.map((r) => r.knowledgeId).filter((x): x is string => !!x)))
  if (fileIds.length) await batchDeleteKnowledge(kbId.value, fileIds)
}
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  selectedRowKeys.value = []
  await loadFiles(true)
  loadInvoiceOverview()
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
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
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
.invoice-date-range { width: 240px; flex-shrink: 0; }
/* 税率下拉随选项内容自适应宽度，禁止收缩避免截断（对齐原类型下拉样式） */
.doc-tax-select { width: auto; flex-shrink: 0; min-width: max-content; }
/* 多税率发票：多个税率档位并排标签展示 */
.row-rate-tags { display: inline-flex; align-items: center; flex-wrap: wrap; gap: 4px; }
.row-rate-tag { font-family: var(--td-font-family); }


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

.doc-list-scroll {
  /* P4：列表容器只撑可视区高度，滚动必须由 t-table 内部承担（max-height 100% + 表头 sticky）；
     容器自身不滚动，否则表头会被滚走、底部摘要 sticky 定位错乱（回归修复：overflow 由 auto 恢复为 hidden） */
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-large);
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

/* ---- 底部汇总 ---- */
.doc-summary-bar {
  position: sticky;
  bottom: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 10px 16px;
  font-size: 13px;
  background: var(--td-bg-color-container);
  box-shadow: 0 -2px 8px rgba(0, 0, 0, 0.06);
  color: var(--td-text-color-secondary);
  .doc-summary-count { font-weight: 600; color: var(--td-text-color-primary); }
  .doc-summary-item {
    display: inline-flex; align-items: baseline; gap: 6px;
    .doc-summary-val { font-variant-numeric: tabular-nums; color: var(--td-text-color-primary); font-weight: 600; }
  }
}

/* 底部悬浮工具条：fixed 脱离文档流（悬浮于视口底部），
   出现/消失不占据物理空间 → 页面不会上下跳动、底部无冗余留白 */
.doc-batch-bar {
  position: fixed;
  bottom: var(--wk-batch-bar-bottom);
  left: 50%;
  transform: translateX(-50%);
  z-index: var(--wk-batch-bar-z);
  width: max-content;
  max-width: calc(100vw - 48px);
  box-sizing: border-box;
}
.batch-bar-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px 24px;
  padding: 8px 16px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  box-shadow: var(--td-shadow-2);
}
.batch-bar-left {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 4px;
  min-width: 0;
  flex: 0 0 auto;
}
.batch-bar-count {
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  white-space: nowrap;
}
.batch-bar-clear {
  flex-shrink: 0;
  padding: 0 6px !important;
  height: 28px !important;
  font-size: 12px;
  color: var(--td-text-color-secondary) !important;
  &:hover { color: var(--td-text-color-primary) !important; }
}
.batch-bar-actions {
  flex: 0 0 auto;
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
.batch-bar-actions > * { flex-shrink: 0; }

/* 列内边距收紧：减少列间视觉空隙（表头与单元格同步） */
.doc-list-view :deep(.t-table__cell),
.doc-list-view :deep(.t-table__th) {
  padding-left: 8px;
  padding-right: 8px;
}

/* 排序图标紧凑对齐：TDesign bordered 表格默认对排序列标题两端对齐(space-between)，
   导致图标被推到单元格最右侧。强制左对齐并让图标紧跟文本（非排序列表头不受影响） */
.doc-list-view :deep(.t-table th .t-table__cell--title) {
  justify-content: flex-start !important;
}
.doc-list-view :deep(.t-table th .t-table__cell--title .t-table__filter-icon-wrap) {
  margin-left: 4px;
}

.doc-list-view {
  position: relative;
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 表格高度受限于容器，滚动由 .t-table__content 内部承担（max-height 100% + sticky 表头） */
.doc-list-view :deep(.t-table) {
  flex: 1;
  min-height: 0;
  height: 100%;
  max-height: 100%;
}

/* scrollbar-gutter:stable 保证滚动条占位宽度恒定，列与滚动条不错位 */
.doc-list-view :deep(.t-table__content) {
  scrollbar-gutter: stable;
}

/* 表头 th 背景与内容区统一为 container 令牌（非 bordered 表格默认白底），
   与下方表头右侧遮罩同令牌 → 主题切换同步变色，永不产生色差 */
.doc-list-view :deep(.t-table__header--fixed > tr > th) {
  background-color: var(--td-bg-color-container);
}

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

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-invoice-no {
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
.row-muted { color: var(--td-text-color-disabled); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-status-tag :deep(.t-icon) { margin-right: 2px; }
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
  gap: 28px;
}

// 详情抽屉底部取消/保存（对齐车队 meter-drawer-footer 行内保存）
.invoice-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 0 0;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 4px;
}
.row-items { display: flex; flex-direction: column; gap: 2px; max-width: 100%; overflow: hidden; }
.row-items-line { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: flex; gap: 4px; align-items: baseline; color: var(--td-text-color-primary); }
.row-items-name { max-width: 120px; overflow: hidden; text-overflow: ellipsis; }
.row-items-qty, .row-items-price { color: var(--td-text-color-secondary); font-family: var(--td-font-family-mono, monospace); }
.row-items-more { color: var(--td-text-color-disabled); font-size: var(--td-font-size-body-small); }
.detail-tags-readonly { display: flex; flex-wrap: wrap; gap: 4px; }
.detail-preview-collapse { margin-top: 12px; }
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



.detail-fields { padding-bottom: 16px; }

.field-group { margin-bottom: 18px; }
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
.items-row { display: grid; grid-template-columns: 2fr 1fr 1fr 1fr 44px; gap: 8px; align-items: center; margin-bottom: 8px;
  &--head { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
}
.item-cell { min-width: 0; }
.item-op { display: flex; justify-content: center; }
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
