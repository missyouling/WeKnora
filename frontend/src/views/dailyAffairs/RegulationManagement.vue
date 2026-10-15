<template>
  <div class="regulation-management-container">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>制度管理</h2>
        <p class="header-subtitle">制度档案自动归档</p>
      </div>
      <div class="header-actions">
        <t-button v-if="kbId" theme="primary" @click="uploadVisible = true">
          <template #icon><t-icon name="upload" /></template>
          上传制度
        </t-button>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化制度管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-制度」知识库">
        <template #image><t-icon name="file-copy" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建制度知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="regulation-main">
      <!-- 概览 KPI 卡 -->
      <DashboardKpiGroup :loading="overviewLoading" :cards="overviewCards" :active-type="filterRegType" @card-click="onOverviewCardClick" class="regulation-kpi-group" />
      <!-- 筛选工具栏（复用原项目文档列表样式） -->
      <div class="doc-filter-bar">
        <div class="doc-filter-bar__leading">
          <BusinessListToolbar v-model:keyword="keyword" search-placeholder="搜索全部字段"
            @refresh="applyFilter" :selected-count="selectedRowKeys.length"
            @clear-selection="clearSelection" hide-batch-bar>
            <template #type-extra>
              <div class="doc-filter-field doc-filter-field--wide">
                <t-date-range-picker v-model="dateRange" placeholder="编制日期" class="doc-date-range doc-filter-field__control"
                  clearable allow-input @change="applyFilter">
                  <template #prefixIcon><t-icon name="time" size="16px" /></template>
                </t-date-range-picker>
              </div>
            </template>
            <template #columns>
              <BusinessColumnFilter :columns="effectiveColumns" v-model:visibleKeys="visibleColKeys" @reset="resetColumns" @select-all="selectAllColumns" />
            </template>
            <template #right-extra>
              <t-button variant="outline" size="small" @click="settingsVisible = true">
                <template #icon><t-icon name="setting" size="14px" /></template>
                设置
              </t-button>
              <t-button theme="primary" size="small" @click="uploadVisible = true">
                <template #icon><t-icon name="upload" size="14px" /></template>
                上传制度
              </t-button>
            </template>
          </BusinessListToolbar>
        </div>
      </div>

      <!-- 进行中文件状态横幅（发票基准同款：优先上传实时进度轮播，关闭弹窗后由轮询派生解析/提取进度，多文件滚动展示） -->
      <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

      <!-- 制度列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
        <div class="doc-list-view">
          <t-table
        :data="filteredRows"
        :columns="tableColumns"
        row-key="rowKey"
        size="small"
        :hover="true"
        :loading="listLoading"
        max-height="100%"
        class="doc-table"
        :selected-row-keys="selectedRowKeys"
        select-on-change
        :row-class-name="({ row }: any) => selectedRowKeys.includes(row.rowKey) ? 'is-selected' : ''"
        @row-click="({ row }: any) => openDetail(row)"
        @select-change="(val: string[]) => (selectedRowKeys = val)"
      >
        <template #regNo="{ row }: any">
          <span class="row-regulation-no" :title="row.regNo || row.fileName">{{ row.regNo }}</span>
        </template>
        <template #regName="{ row }: any">
          <span class="row-text" :title="row.regName || row.fileName">{{ row.regName || row.fileName }}</span>
        </template>
        <template #regType="{ row }: any">
          <span class="row-text" :title="row.regType">{{ row.regType }}</span>
        </template>
        <template #version="{ row }: any">
          <span class="row-text">{{ row.version }}</span>
        </template>
        <template #pageCount="{ row }: any">
          <span class="row-text">{{ row.pageCount }}</span>
        </template>
        <template #issueDate="{ row }: any">
          <span class="row-mono">{{ row.issueDate }}</span>
        </template>
        <template #effectiveDate="{ row }: any">
          <span class="row-mono">{{ row.effectiveDate }}</span>
        </template>
        <template #scope="{ row }: any">
          <span class="row-text" :title="row.scope">{{ row.scope }}</span>
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
        <template #dept="{ row }: any">
          <span class="row-text" :title="row.dept">{{ row.dept }}</span>
        </template>
        <template #modifyCount="{ row }: any">
          <span class="row-text">{{ row.modifyCount }}</span>
        </template>
        <template #compiledBy="{ row }: any">
          <span class="row-text" :title="row.compiledBy">{{ row.compiledBy }}</span>
        </template>
        <template #reviewedBy="{ row }: any">
          <span class="row-text" :title="row.reviewedBy">{{ row.reviewedBy }}</span>
        </template>
        <template #approvedBy="{ row }: any">
          <span class="row-text" :title="row.approvedBy">{{ row.approvedBy }}</span>
        </template>
        <template #summary="{ row }: any">
          <span class="row-text" :title="row.summary">{{ row.summary }}</span>
        </template>
        <template #confidentiality="{ row }: any">
          <span class="row-text">{{ row.confidentiality }}</span>
        </template>
        <template #remark="{ row }: any">
          <span class="row-text" :title="row.remark">{{ row.remark }}</span>
        </template>
        <template #fileName="{ row }: any">
          <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
        </template>
      </t-table>
    </div>
        </div>

      <!-- 底部汇总（选中记录时显示选中制度汇总，未选中显示全部；选中时避让底部工具栏） -->
      <div class="doc-summary-bar" :class="{ 'is-batch-visible': selectedRowKeys.length }">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 份</span>
      </div>

    </div>

    <!-- 自有批量浮条（发票基准同款） -->
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
                <div class="del-pop-title">确定删除所选 {{ selectedRowKeys.length }} 个制度记录吗？删除后不可恢复。</div>
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

    <!-- 创建知识库向导 -->
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-制度" kb-desc="制度管理固定使用专用知识库，名称不可修改" description-placeholder="用于存放并解析制度文件，自动提取制度字段" default-description="用于存放并解析制度文件，自动提取制度字段" model-tip="提取模型将复用下方「对话模型」，用于解析制度文件。若列表为空，请先在系统设置中添加模型。" @created="onKbCreated" />
    <!-- 上传弹窗（对齐发票/车队：分类选择 + 拖拽/选择/粘贴 + 进度 + 防重检测） -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="regulation" title="上传制度"
      :type-options="uploadTypeOptions" @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab，黄金基准 StandardSettingDrawer） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" :scope="'regulation'"
      kb-name="日常事务-制度" title="制度设置" @saved="() => reloadColumns(true)" />

    <!-- 历史记录（对齐发票基准：已上传文件抽屉，展示解析/提取状态与制度类型） -->
    <FleetUploadHistoryDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" scope="regulation"
      @changed="onUploadHistoryChanged" />


    <!-- 制度详情抽屉（竖向区块，可拖宽，上下滚动） -->
<SettingDrawer v-model:visible="detailVisible" :title="detailTitle" width="654px" :storage-key="'weknora-regulation-drawer-width'" hide-footer destroy-on-close class="regulation-detail-drawer">
      <div class="regulation-detail-body">
        <!-- 摘要 -->
        <section class="detail-block">
          <div class="detail-block-title">摘要</div>
          <div class="detail-block-content">
            <template v-if="currentDetail?.description">
              <div v-if="summaryState" class="summary-status">
                <t-tag :theme="summaryState.theme" variant="light" size="small">
                  <template v-if="summaryState.icon" #icon>
                    <t-icon :name="summaryState.icon" :class="{ 'icon-spin': summaryState.spin }" />
                  </template>
                  {{ summaryState.label }}
                </t-tag>
              </div>
              <!-- 摘要：复用知识库文档抽屉样式（线框 + 展开/折叠 + 每条字段一行） -->
              <div class="summary_wrapper" :class="{ 'summary_clickable': summaryOverflow || summaryExpanded }"
                @click="(summaryOverflow || summaryExpanded) && (summaryExpanded = !summaryExpanded)">
                <div ref="summaryRef" :class="['summary_content', { 'summary_collapsed': !summaryExpanded }]">{{
                  summaryLines
                }}</div>
                <div v-if="(summaryOverflow && !summaryExpanded) || summaryExpanded" class="summary_fade"
                  :class="{ 'summary_fade_expanded': summaryExpanded }">
                  <t-icon :name="summaryExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="summary_fade_icon" />
                </div>
              </div>
            </template>
            <t-empty v-else description="暂无摘要" />
          </div>
        </section>

        <!-- 制度字段 -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">制度信息</div>
                <div class="field-grid">
                  <t-form-item label="制度编号" label-width="110px">
                    <t-input v-model="editForm.reg_no" placeholder="无编号时自动生成" />
                  </t-form-item>
                  <t-form-item label="制度名称" label-width="110px">
                    <t-input v-model="editForm.reg_name" placeholder="" />
                  </t-form-item>
                  <t-form-item label="制度类型" label-width="110px">
                    <t-select v-model="editForm.reg_type" :options="regTypeOptions" clearable filterable
                      placeholder="选择类型" />
                  </t-form-item>
                  <t-form-item label="编制部门" label-width="110px">
                    <t-input v-model="editForm.dept" placeholder="" />
                  </t-form-item>
                  <t-form-item label="版本" label-width="110px">
                    <t-input v-model="editForm.version" placeholder="" />
                  </t-form-item>
                  <t-form-item label="编制日期" label-width="110px">
                    <t-date-picker v-model="editForm.issue_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="页数" label-width="110px">
                    <t-input v-model="editForm.page_count" placeholder="" />
                  </t-form-item>
                  <t-form-item label="修改次数" label-width="110px">
                    <t-input v-model="editForm.modify_count" placeholder="" />
                  </t-form-item>
                  <t-form-item label="编制" label-width="110px">
                    <t-input v-model="editForm.compiled_by" placeholder="" />
                  </t-form-item>
                  <t-form-item label="审核" label-width="110px">
                    <t-input v-model="editForm.reviewed_by" placeholder="" />
                  </t-form-item>
                  <t-form-item label="批准" label-width="110px">
                    <t-input v-model="editForm.approved_by" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">制度内容</div>
                <div class="field-grid">
                  <t-form-item label="适用范围" label-width="110px">
                    <t-input v-model="editForm.scope" placeholder="" />
                  </t-form-item>
                  <t-form-item label="生效日期" label-width="110px">
                    <t-date-picker v-model="editForm.effective_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="密级" label-width="110px">
                    <t-select v-model="editForm.confidentiality" :options="CONFIDENTIALITY_OPTIONS" clearable
                      placeholder="选择密级" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">摘要</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="摘要" label-width="110px">
                    <t-textarea v-model="editForm.summary" :autosize="{ minRows: 2, maxRows: 6 }" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">备注</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="备注" label-width="110px">
                    <t-textarea v-model="editForm.remark" :autosize="{ minRows: 2, maxRows: 5 }" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="detail-save-hint">
                <t-icon name="check-circle" size="14px" />
                <span>字段修改后将自动保存{{ autoSaving ? '（保存中...）' : '' }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 源文件预览：默认折叠；折叠时不挂载 DocumentPreview -->
        <t-collapse v-model="previewExpanded" class="detail-preview-collapse">
          <t-collapse-panel value="preview" header="源文件预览">
            <div class="detail-block-content">
              <DocumentPreview v-if="currentRow && previewExpanded.length" :knowledge-id="currentRow.knowledgeId"
                :file-type="currentRow?.fileType || ''" :file-name="currentRow?.fileName || ''" :active="true" />
            </div>
          </t-collapse-panel>
        </t-collapse>
        <!-- 手动保存：四模块统一为手动保存（2026-10-12 决策），替代原来的 1200ms 自动保存 -->
        <div class="regulation-detail-footer">
          <t-button variant="outline" size="small" @click="detailVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="autoSaving" @click="saveEditForm()">保存</t-button>
        </div>
      </div>
    </SettingDrawer>

    <!-- 制度编号重复二次确认 -->
    <t-dialog v-model:visible="dupConfirmVisible" header="制度编号重复" width="420" @confirm="onDupConfirm">
      <span>系统已存在相同的制度编号「{{ dupNo }}」，是否强制保存？</span>
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
      <div v-if="printVisible" class="regulation-print-mask">
        <div class="regulation-print-dialog">
          <div class="regulation-print-header">
            <span class="regulation-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 份）` }}</span>
            <t-button variant="text" size="small" class="regulation-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="regulation-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并制度 PDF…'" />
            </div>
          </div>
          <div class="regulation-print-footer">
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
import { ref, shallowRef, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { listFleetCategories } from '@/api/fleet'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import { colWidthOf } from './columnWidth'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  getKnowledgeDetails,
  batchDeleteKnowledge,
  updateBusinessMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractBusinessDocument,
  listRegulationRecords,
  listRegulationTypes,
  previewKnowledgeFile,
  reparseKnowledge,
  getRecognitionConfig,
  listRegulationOverview,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import StandardSettingDrawer from '@/components/business/StandardSettingDrawer.vue'
import DashboardKpiGroup from '@/components/business/DashboardKpiGroup.vue'
import type { KpiCard } from '@/components/business/DashboardKpiGroup.vue'
import BusinessKbWizard from './BusinessKbWizard.vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import BusinessColumnFilter from './BusinessColumnFilter.vue'
import FleetUploadDialog from './FleetUploadDialog.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'
import BusinessListToolbar from './BusinessListToolbar.vue'
import BusinessParseStatusBar from '@/components/business/BusinessParseStatusBar.vue'
import { useBusinessPolling } from '@/composables/useBusinessPolling'

const KB_NAME = '日常事务-制度'
const ACCEPT_TYPES = ['pdf', 'jpg', 'jpeg', 'png']
const PAGE_SIZE = 20

// 制度类型枚举（编辑/筛选推荐值）
const REGULATION_TYPES = ['人事管理', '财务管理', '生产管理', '行政管理', '安全管理', '其它制度']
// 密级枚举
const CONFIDENTIALITY_OPTIONS = ['公开', '内部', '机密', '绝密']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
const DEFAULT_REGULATION_COLUMNS: ColumnDef[] = [
  { key: 'regNo', label: '制度编号', default: true, w: '1.4fr' },
  { key: 'regName', label: '制度名称', default: true, w: '1.8fr' },
  { key: 'regType', label: '制度类型', default: true, w: '1fr' },
  { key: 'version', label: '版本', default: true, w: '0.7fr' },
  { key: 'pageCount', label: '页数', default: true, w: '0.7fr' },
  { key: 'issueDate', label: '编制日期', default: true, w: '1.1fr' },
  { key: 'effectiveDate', label: '生效日期', default: true, w: '1.1fr' },
  { key: 'scope', label: '适用范围', default: true, w: '1.6fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'dept', label: '编制部门', default: false, w: '1.3fr' },
  { key: 'modifyCount', label: '修改次数', default: false, w: '0.8fr' },
  { key: 'compiledBy', label: '编制', default: false, w: '1fr' },
  { key: 'reviewedBy', label: '审核', default: false, w: '1fr' },
  { key: 'approvedBy', label: '批准', default: false, w: '1fr' },
  { key: 'summary', label: '摘要', default: false, w: '2fr' },
  { key: 'confidentiality', label: '密级', default: false, w: '0.8fr' },
  { key: 'remark', label: '备注', default: false, w: '1.4fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns, colVisible,
  selectedRowKeys, onSelectChange, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-regulation-list-columns',
  defaultColumns: DEFAULT_REGULATION_COLUMNS,
})

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

interface RegulationItem {
  reg_no?: string
  reg_name?: string
  reg_type?: string
  dept?: string
  version?: string
  issue_date?: string
  page_count?: string
  modify_count?: string
  scope?: string
  effective_date?: string
  confidentiality?: string
  remark?: string
  compiled_by?: string
  reviewed_by?: string
  approved_by?: string
  summary?: string
  page?: number
}

interface RegulationRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'regulation' | 'not_regulation' | 'unknown' | 'pending'
  regNo?: string
  regName?: string
  regType?: string
  dept?: string
  version?: string
  issueDate?: string
  pageCount?: string
  modifyCount?: string
  scope?: string
  effectiveDate?: string
  confidentiality?: string
  remark?: string
  compiledBy?: string
  reviewedBy?: string
  approvedBy?: string
  summary?: string
  page?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}

const items = ref<KnowledgeItem[]>([])
const regulationRows = shallowRef<RegulationRow[]>([])

// KPI 卡：总数、本月新增、按类型分布（前端基于已加载数据计算）
const overviewStats = ref<any>(null)
const overviewLoading = ref(true)
const loadRegulationOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listRegulationOverview(kbId.value)
    overviewStats.value = res?.data || null
  } catch (e: any) {
    overviewStats.value = null
  } finally {
    overviewLoading.value = false
  }
}
// ---- KPI 概览卡（黄金基准 DashboardKpiGroup：全部制度 → 各制度类型卡 → 其它制度 → 历史记录） ----
// 数据源为后端全量概览 regulationOverview（不受工具栏类型/日期/搜索筛选影响）。
const overviewCards = computed<KpiCard[]>(() => {
  const st = overviewStats.value || {}
  const total = Number(st.total || 0)
  const monthCount = Number(st.current_month || 0)
  const parseFailed = Number(st.parse_failed || 0)
  const fileCount = Number(st.file_count || 0)
  // 分类统计：后端按 reg_type 精确聚合（空归「未分类」）
  const catCount: Record<string, number> = {}
  for (const c of (st.by_category || [])) catCount[String(c.category)] = Number(c.count) || 0
  const cards: KpiCard[] = [
    // 全部制度：全量总份数（含未分类/其它制度与待补录占位行），点击清空筛选进入全部态（对齐发票）
    { key: 'total', label: '已收录制度', icon: 'file', value: String(total), unit: '份', sub: `本月新增 ${monthCount} 份`, theme: 'brand', action: 'all' },
  ]
  // 类型分布卡：以制度分类列表（categories[scope=regulation]）为权威源动态生成，顺序 = 分类数组顺序；
  // enabled=false 的分类不出卡片（六同步：同时不出筛选下拉/上传选项）
  const enabledCats = (regCats.value || []).filter((c: any) => c.enabled !== false)
  let catSum = 0
  for (const c of enabledCats) {
    // 「其它制度」为系统内置兜底分类：统计口径是排除式桶（total - 各启用分类之和），
    // 跳过该分类卡且不计入 catSum，避免与下方排除式卡重复/口径错乱
    if (c.name === '其它制度') continue
    const cnt = catCount[String(c.name)] || 0
    catSum += cnt
    cards.push({ key: `type-${c.name}`, label: c.name, icon: 'file-1', value: String(cnt), unit: '份', theme: 'neutral', action: 'type', typeValue: c.name })
  }
  // 「其它制度」排除式统计桶（收纳未匹配任何启用分类的记录，含未分类/已停用分类存量），
  // 无条件渲染（计数 0 兜底），避免卡片数量二次跳变
  const otherCount = Math.max(0, total - catSum)
  cards.push({ key: 'type-其它制度', label: '其它制度', icon: 'file-unknown', value: String(otherCount), unit: '份', theme: 'neutral', action: 'type', typeValue: '其它制度' })
  // 历史记录（已上传文件，对齐发票基准 fileCount/parseFailed）：异常文件数 sub 展示，点击打开已上传文件抽屉
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: String(fileCount), unit: '份', sub: parseFailed ? `${parseFailed} 份异常` : '无异常文件', theme: parseFailed > 0 ? 'warning' : 'neutral', action: 'history' })
  return cards
})
// 卡片点击联动（发票基准同款 action 分流：类型卡切筛选；全部卡清空；历史记录卡开抽屉）
const onOverviewCardClick = (card: KpiCard) => {
  if (card.action === 'all') {
    filterRegType.value = ''
  } else if (card.action === 'type' && card.typeValue) {
    filterRegType.value = card.typeValue
  } else if (card.action === 'history') {
    historyVisible.value = true
  }
}
// 进行中的文件（解析中/提取中/待提取），在制度级列表顶部以状态行展示
const regulationSummary = ref<{ total: number; sumAmount: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const filterRegType = ref('')
const filterFulfillStatus = ref('')
const regTypeOptions = ref<Array<{ value: string; label: string }>>([])
const regCats = ref<any[]>([])
const uploadTypeOptions = computed(() => {
  const cats = regCats.value.filter((c: any) => c.enabled !== false)
  if (!cats.length) return [{ label: '公司制度', value: 'regulation__公司制度' }]
  return cats.map((c: any) => ({ label: c.name, value: `regulation__${c.name}` }))
})
const onUploadDone = async () => {
  await Promise.all([loadFiles(true), loadRegulationOverview()])
}
const dateRange = ref<Array<string>>([])

// 列显隐
const tableColumns = computed(() => {
  const cols: any[] = [{ colKey: 'row-select', type: 'multiple', width: 46 },
    { colKey: 'serial-number', title: '', width: 44 }]
  const rows = filteredRows.value.filter(r => r.kind !== 'pending')
  for (const c of visibleColDefs.value) {
    // 列宽：字段配置 width>0 固定（clamp 60~400）；0/未配置按内容自适应（封顶 150）
    const w = colWidthOf(c.width, c.label, rows.map(r => colValue(r, c.key)))
    cols.push({ colKey: c.key, title: c.label, ellipsis: true, width: w })
  }
  return cols
})

// 字段显隐变化即持久化：t-checkbox-group 的 @change 在部分勾选交互下不触发，
// 用 watch 兜底，确保取消列（如备注）后硬刷新不恢复默认。

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<RegulationRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const previewExpanded = ref<string[]>([])
const currentRow = ref<RegulationRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({})
// 手动保存模式：已删除 autoSaveTimer/autoSaveDirty/editFormSnapshot（原自动保存用）
const autoSaving = ref(false)

// 摘要（复用知识库文档抽屉样式：线框 + 展开/折叠 + 每条字段一行）
const summaryExpanded = ref(false)
const summaryRef = ref<HTMLElement>()
const summaryOverflow = ref(false)
const summaryLines = computed(() => {
  const d = currentDetail.value?.description || ''
  if (!d) return ''
  return d.replace(/-(?=[^\s-])/g, '\n-').trim()
})
const checkSummaryOverflow = () => {
  const el = summaryRef.value
  if (!el) { summaryOverflow.value = false; return }
  summaryOverflow.value = el.scrollHeight > el.clientHeight + 1
}
watch(summaryRef, () => checkSummaryOverflow())
watch(() => currentDetail.value?.description, () => {
  summaryExpanded.value = false
  nextTick(() => checkSummaryOverflow())
})

// 抽屉宽度（可拖动，localStorage 记忆）
const DRAWER_WIDTH_KEY = 'weknora-regulation-drawer-width'
// 打印预览
const printVisible = ref(false)
const historyVisible = ref(false)
const settingsVisible = ref(false)
const uploadVisible = ref(false)
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
      await loadRegTypes()
      await cleanNonRegulationFiles()
      await loadFiles()
      await loadRegulationOverview()
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
    await loadRegTypes()
    await cleanNonRegulationFiles()
    await loadFiles()
    await loadRegulationOverview()
    startPolling()
  }
}

// ---- 存量非制度清理：扫描知识库中已标记 not_regulation 的文件并自动删除 ----
let cleaningNonRegulation = false
const cleanNonRegulationFiles = async () => {
  if (!kbId.value || cleaningNonRegulation) return
  cleaningNonRegulation = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_regulation' || it.custom_metadata?.extract_status === 'not_regulation')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非制度文件（旧数据清理）`)
      loadRegTypes()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonRegulation = false }
}

// ---- 制度类型下拉：从识别规则配置的分类列表加载（含自定义分类），
// 未配置时兜底用知识库现有制度类型 + 枚举 ----
const loadRegTypes = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    const types: string[] = Array.isArray(c?.types) ? c.types.filter(Boolean) : []
    if (types.length) {
      regTypeOptions.value = types.map(t => ({ value: t, label: t }))
      return
    }
  } catch { /* 走兜底 */ }
  try {
    const res: any = await listRegulationTypes(kbId.value)
    const list = res?.data || res?.list || []
    const arr = Array.isArray(list) ? list : []
    const dynamic = arr.map((r: any) => ({ value: r.reg_type, label: r.reg_type }))
    // 动态类型在前，未出现过的枚举类型补在后（allow 自定义）
    const seen = new Set(dynamic.map((d: any) => d.value))
    const fixed = REGULATION_TYPES.filter(t => !seen.has(t)).map(t => ({ value: t, label: t }))
    regTypeOptions.value = [...dynamic, ...fixed]
  } catch { /* 类型加载失败不阻塞 */ }
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

const openTagEdit = (row: RegulationRow) => {
  tagTarget.value = row
  tagDialogVisible.value = true
}

const tagTargetTags = computed(() => (tagTarget.value ? rowTags(tagTarget.value) : []))

const onTagEditConfirm = async (tagIds: string[]) => {
  if (!tagTarget.value) return
  try {
    await updateKnowledgeTagBatch({ updates: { [tagTarget.value.knowledgeId]: tagIds } })
    MessagePlugin.success('标签已更新')
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '标签更新失败')
  }
}

const onTagCreated = () => { loadTags() }
const openTagManage = () => { tagManageVisible.value = true }
const onTagManageChanged = () => {
  loadTags()
  loadFiles(true)
}


// ---- 列表加载（制度级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    regulationRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) {
    return
  } else {
    loadingMore.value = true
  }
  try {
    // 「其它制度」为排除式筛选：附上启用分类名集合，后端按 reg_type 不属于该集合过滤，
    // 与概览卡其它制度口径一致。「其它制度」本身作为归属目标不从集合剔除（后端防御）。
    const knownCats = regCats.value
      .filter((c: any) => c.enabled !== false)
      .map((c: any) => String(c.name))
      .filter((n: string) => Boolean(n))
    const res: any = await listRegulationRecords(kbId.value, {
      q: keyword.value || undefined,
      reg_type: filterRegType.value || undefined,
      known_categories: filterRegType.value === '其它制度' && knownCats.length > 0
        ? knownCats.join(',') : undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    regulationSummary.value = { total, sumAmount: 0, sumTotal: 0 }
    const existing = new Set(regulationRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapRegulationRecord).filter(r => !existing.has(r.rowKey))
    regulationRows.value = [...regulationRows.value, ...fresh]
    hasMore.value = regulationRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '制度列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

// 把后端制度级记录（RegulationRecord）映射为前端行
const mapRegulationRecord = (r: any): RegulationRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.reg_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'regulation',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  regNo: r.reg_no || '',
  regName: r.reg_name || '',
  regType: r.reg_type || '',
  dept: r.dept || '',
  version: r.version || '',
  issueDate: r.issue_date || '',
  pageCount: r.page_count || '',
  modifyCount: r.modify_count || '',
  scope: r.scope || '',
  effectiveDate: r.effective_date || '',
  confidentiality: r.confidentiality || '',
  remark: r.remark || '',
  compiledBy: r.compiled_by || '',
  reviewedBy: r.reviewed_by || '',
  approvedBy: r.approved_by || '',
  summary: r.summary || '',
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 制度级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

const parseCustomMetadata = (item: KnowledgeItem): RegulationRow[] => {
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
  if (kind === 'not_regulation') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_regulation', extractError, kind, ...base }]
  }
  const regulations = Array.isArray(meta.regulations) ? meta.regulations : []
  if (regulations.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return regulations.map((ct: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: regulations.length > 1 ? `${fileName}（第${idx + 1}份）` : fileName,
    multiIndex: regulations.length > 1 ? `${idx + 1}/${regulations.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    regNo: ct.reg_no || '',
    regName: ct.reg_name || '',
    regType: ct.reg_type || '',
    dept: ct.dept || '',
    version: ct.version || '',
    issueDate: ct.issue_date || '',
    pageCount: ct.page_count || '',
    modifyCount: ct.modify_count || '',
    scope: ct.scope || '',
    effectiveDate: ct.effective_date || '',
    confidentiality: ct.confidentiality || '',
    remark: ct.remark || '',
    compiledBy: ct.compiled_by || '',
    reviewedBy: ct.reviewed_by || '',
    approvedBy: ct.approved_by || '',
    summary: ct.summary || '',
    page: Number(ct.page) || 0,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端制度级接口完成，前端直接使用返回的分页数据；
// 顶部合并"进行中文件"状态行（解析中/提取中/待提取），提取完成即消失
const pendingRows = computed<RegulationRow[]>(() => pendingFiles.value.map((k) => {
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
  } as RegulationRow
}))
const filteredRows = computed(() => [...pendingRows.value, ...regulationRows.value])


// ---- 多选 ----
const selectableRows = computed(() => filteredRows.value.filter(r => r.kind !== 'pending'))
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
  const byKey = new Map(regulationRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as RegulationRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中制度的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  return { total: rows.length, sumAmount: 0, sumTotal: 0 }
})
const displaySummary = computed(() => selectedSummary.value || regulationSummary.value)

const applyFilter = () => { loadFiles(true) }
const onKeywordChange = () => { loadFiles(true) }

// 懒加载
const listScrollRef = ref<HTMLElement>()
const onListScroll = (e: Event) => {
  const el = e.target as HTMLElement
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 200 && hasMore.value && !listLoading.value && !loadingMore.value) {
    loadFiles()
  }
}

// 统一上传弹窗 FleetUploadDialog 是唯一上传入口（旧的原生 file input 已移除）。

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling, scanOnce,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'regulation'),
  onPendingFiles: (files) => {
    // 状态条展示所有「进行中」文件：不仅新上传待解析/待提取的文件，也包括
    // 已产生制度行、正处于「重新解析/重新提取」中的文件（重解/重提选中的记录
    // 必然已有制度行，若按行过滤会把它们排除出状态条，只剩 toast 提示）。
    // 轮询已保证 pend 只含解析中/提取中文件，完成/失败的文件不会进入这里。
    pendingFiles.value = files
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是制度文件，已移至删除历史，可在删除历史中恢复`)
    loadRegTypes()
  },
  onTick: () => loadFiles(),
})

// 需求：解析/提取完成后自动刷新 KPI 卡片统计。
// pendingFiles 由轮询每 3s 更新：集合变化（有文件解析完成进入制度行 / 提取完成 / 失败）
// 即触发概览统计重载，保证卡片数量与本轮真实状态一致。
let lastPendingKey = ''
watch(pendingFiles, (v) => {
  const key = v.map((f: any) => f.id).sort().join(',')
  if (key !== lastPendingKey) {
    lastPendingKey = key
    if (kbId.value) loadRegulationOverview()
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
    regulationRows.value
      .filter((r) => r.extractStatus === 'success' || r.extractStatus === 'failed')
      .map((r) => r.fileName),
  )
  return items.filter((it: any) => !doneNames.has(it.name))
}
// 轮询刷新列表后：用表格实际 extractStatus 清掉已完成文件的残留进度
watch(regulationRows, () => {
  if (!uploadProgress.value.length) return
  uploadProgress.value = filterLiveProgress(uploadProgress.value)
})
function stopProgressTimer() {
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}
onBeforeUnmount(stopProgressTimer)

// 解析/提取实时进度（弹窗关闭后兜底数据源）：
// 后端轮询只返回状态枚举（pending/processing/completed + extract_status），无真实百分比。
// 沿用上传弹窗同款"按耗时估算"口径：parsing 10→92、extracting 30→100；
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

const { extractStatusOf, statusOf, summaryState, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'regulation', notLabel: '非制度',
})

// ---- 详情抽屉 ----
const openDetail = async (row: RegulationRow) => {
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
        // 制度一份文件一份：优先按制度编号匹配详情（制度不按页匹配）
        const match = parsed.find(p => p.regNo && p.regNo === row.regNo)
          || parsed.find(p => p.regName === row.regName) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.regNo ? `制度详情 · ${currentRow.value.regNo}` : '制度详情'
)

// 上传历史抽屉（删除/恢复）变更后同步：删除走乐观移除 + 延迟对账；其余全量刷新
const onUploadHistoryChanged = async (payload?: { deletedIds?: string[] }) => {
  if (payload?.deletedIds?.length) {
    optimisticRemoveRegulationRows(new Set(payload.deletedIds), new Set())
    schedulePostDeleteRefresh()
    return
  }
  await Promise.all([loadFiles(true), loadRegulationOverview()])
}

// ---- 字段编辑 + 自动保存 ----
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  editForm.value = {
    reg_no: r.regNo || '',
    reg_name: r.regName || '',
    reg_type: r.regType || '',
    dept: r.dept || '',
    version: r.version || '',
    issue_date: r.issueDate || '',
    page_count: r.pageCount || '',
    modify_count: r.modifyCount || '',
    scope: r.scope || '',
    effective_date: r.effectiveDate || '',
    confidentiality: r.confidentiality || '',
    remark: r.remark || '',
    compiled_by: r.compiledBy || '',
    reviewed_by: r.reviewedBy || '',
    approved_by: r.approvedBy || '',
    summary: r.summary || '',
  }
}

// 手动保存模式（2026-10-12 决策：四模块统一手动保存）：删除原来的 1200ms 自动保存 watcher，
// 用户点「保存」按钮时才调用 saveEditForm。

// 制度编号重复二次确认：后端 409 + DUPLICATE_REGULATION_NO 时先询问，确认后 force 重试
const dupConfirmVisible = ref(false)
const dupNo = ref('')
const onDupConfirm = async () => {
  dupConfirmVisible.value = false
  await saveEditForm(true)
}

const saveEditForm = async (force = false) => {
  const now = currentRow.value
  if (!now) return
  autoSaving.value = true
  try {
    const res: any = await getKnowledgeDetails(now.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const regulations = Array.isArray(meta.regulations) ? [...meta.regulations] : []
    const updated: any = {
      reg_no: editForm.value.reg_no || '',
      reg_name: editForm.value.reg_name || '',
      reg_type: editForm.value.reg_type || '',
      dept: editForm.value.dept || '',
      version: editForm.value.version || '',
      issue_date: editForm.value.issue_date || '',
      page_count: editForm.value.page_count || '',
      modify_count: editForm.value.modify_count || '',
      scope: editForm.value.scope || '',
      effective_date: editForm.value.effective_date || '',
      confidentiality: editForm.value.confidentiality || '',
      remark: editForm.value.remark || '',
      compiled_by: editForm.value.compiled_by || '',
      reviewed_by: editForm.value.reviewed_by || '',
      approved_by: editForm.value.approved_by || '',
      summary: editForm.value.summary || '',
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = regulations.findIndex((ct: any, i: number) =>
        i === nowPage - 1 || Number(ct.page) === nowPage || ct.reg_no === now.regNo)
    }
    if (targetIdx < 0) targetIdx = regulations.findIndex((ct: any) => ct.reg_no === now.regNo)
    if (targetIdx >= 0) regulations[targetIdx] = { ...regulations[targetIdx], ...updated, page: nowPage || regulations[targetIdx]?.page }
    else regulations.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const hasRegulationData = regulations.some((c: any) =>
      (c.reg_no || '').trim() || (c.reg_name || '').trim() || (c.reg_type || '').trim() ||
      (c.dept || '').trim() || (c.version || '').trim() || (c.issue_date || '').trim())
    const nextStatus = hasRegulationData ? 'success' : 'manual'
    const nextError = hasRegulationData ? '' : '人工入库待编辑，请补充字段'
    const newMeta = { ...meta, kind: 'regulation', regulations, extract_status: nextStatus, extract_error: nextError }
    // regulations 为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由
    // force=true 表示用户已确认编号重复，后端跳过查重强制保存
    try {
      await updateBusinessMetadata(kbId.value, now.knowledgeId, newMeta, force)
    } catch (e: any) {
      if (e?.status === 409 && String(e?.message || '').startsWith('DUPLICATE_REGULATION_NO')) {
        dupNo.value = String(e?.message || '').split(':').pop() || ''
        dupConfirmVisible.value = true
        return
      }
      throw e
    }
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑后列表立即刷新（shallowRef 铁律：不能按下标赋值，
    // 必须 map 后整体替换数组，否则不触发更新）
    const listIdx = regulationRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      regulationRows.value = regulationRows.value.map((r: any, i: number) =>
        i === listIdx ? {
          ...r,
          extractStatus: nextStatus,
          extractError: nextError,
          regNo: updated.reg_no,
          regName: updated.reg_name,
          regType: updated.reg_type,
          dept: updated.dept,
          version: updated.version,
          issueDate: updated.issue_date,
          pageCount: updated.page_count,
          modifyCount: updated.modify_count,
          scope: updated.scope,
          effectiveDate: updated.effective_date,
          confidentiality: updated.confidentiality,
          remark: updated.remark,
        } : r,
      )
    }
  } catch (e: any) {
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
  v === undefined || v === null ? '' : `¥ ${Number(v).toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`

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

// 批量重解析/重提取：制度一个文件一般只有一份制度，选中即对该文件操作（无需按页）。
const reparseBusy = ref(false)
const extractBusy = ref(false)
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
    return `确定按最新提取规则重新提取「${name}」的所有制度字段吗？将覆盖已有提取结果。`
  }
  return `确定按最新提取规则重新提取选中的 ${ids.length} 个文件的所有制度字段吗？将覆盖已有提取结果。`
})

// 重新解析：物理文件重新拆页/解析（支持多文件批量，二次确认）
const handleReparse = async () => {
  const ids = selectedIds.value
  if (!ids.length) { MessagePlugin.info('请先选中要重新解析的制度'); return }
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
    loadRegulationOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    reparseBusy.value = false
  }
}

// 重新提取：大模型/OCR 按最新提取规则重新提取选中文件全部制度字段（支持多文件批量，二次确认）
const handleFileExtract = async () => {
  const all = selectedIds.value
  if (!all.length) { MessagePlugin.info('请先选中要重新提取的制度'); return }
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
  try {
    // selectedIds 已按 knowledgeId 去重：多选 N 条记录若同属一个文件，只对该文件重新提取一次。
    const results = await settleInBatches(
      ids,
      BATCH_CONCURRENCY,
      kid => extractBusinessDocument(kbId.value, kid, 'regulation', filterRegType.value || undefined),
    )
    let ok = 0
    let conflict = 0
    results.forEach((r, i) => {
      if (r.status === 'fulfilled') { ok++; return }
      // 409 冲突：后端 extractMu 判定该文件正被提取（另一标签/请求在途），
      // 属重复提交而非提取失败——不写入 extractFailed，避免状态列误报「提取失败」。
      // request.ts 非 2xx JSON 响应统一带 $httpStatus（非枚举属性）。
      if ((r.reason as any)?.$httpStatus === 409) { conflict++; return }
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
    loadRegulationOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    // asynq 化后 POST 200 仅代表入队成功，不代表提取完成：
    // 成功提交/409 的文件保留 in-flight，由 useBusinessPolling 轮询在 es 到
    // 终态（success/failed）时统一回收——若此处提前删除，下一轮 3s 轮询
    // 看到 es=processing 且不在 in-flight 会重复提交 → 后端 extractMu 409。
    // 仅真实失败（非 409）删除 in-flight，避免文件因 in-flight 残留永久卡死。
    results.forEach((r, i) => {
      if (r.status === 'rejected' && (r.reason as any)?.$httpStatus !== 409) {
        extractInFlight.value.delete(ids[i])
      }
    })
    extractBusy.value = false
  }
}

// 打印：制度一个文件一般只有一份制度，打印预览整个文档（含多页），
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
      if (!blob) { notes.push(`${r.regNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式制度：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        // 制度打印整个源文件（一份制度可能多页，如盖章扫描件）
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pageCount = pdf.getPageCount()
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
        if (pageCount > 1) notes.push(`${r.regNo || r.fileName}（共 ${pageCount} 页）`)
      } catch {
        notes.push(`${r.regNo || r.fileName}（页抽取失败，已并入整文件）`)
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
      MessagePlugin.error('未获取到可打印的制度页面')
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
const catalogValueOf = (row: RegulationRow, key: string): string => {
  switch (key) {
    case 'regNo': return row.regNo || ''
    case 'regName': return row.regName || ''
    case 'regType': return row.regType || ''
    case 'version': return row.version || ''
    case 'issueDate': return row.issueDate || ''
    case 'dept': return row.dept || ''
    case 'pageCount': return row.pageCount || ''
    case 'modifyCount': return row.modifyCount || ''
    case 'scope': return row.scope || ''
    case 'effectiveDate': return row.effectiveDate || ''
    case 'confidentiality': return row.confidentiality || ''
    case 'remark': return row.remark || ''
    case 'compiledBy': return row.compiledBy || ''
    case 'reviewedBy': return row.reviewedBy || ''
    case 'approvedBy': return row.approvedBy || ''
    case 'summary': return row.summary || ''
    case 'extractStatus': { const s = statusOf(row).label; return s === '--' ? '' : s }
    case 'tags': return rowTags(row).map((t: any) => t.name).join('、')
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
    const bytes = await generateCatalogPdf({ title: '制度目录', columns: cols, rows })
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
  a.download = `制度目录-${Date.now()}.pdf`
  a.click()
}

// 安全清空多选（表格无 ref 时仅重置 selectedRowKeys，与发票基准同口径）
const clearSelectionSafe = () => {
  clearSelection()
}
// 类型筛选锁定（发票基准同款）：当前选中项合法则保留（含路由/上传回填），
// 否则按分类顺序选中第一个启用分类。
// 「其它制度」为排除式筛选态（点击其它制度卡进入），分类重载时须放行；
// 空值 = 「全部」态（点击「已收录制度」卡进入，对齐发票），放行避免被弹回。
const ensureRegType = () => {
  if (filterRegType.value === '其它制度' || filterRegType.value === '') return
  const cats = regCats.value.filter((c: any) => c.enabled !== false).map((c: any) => c.name)
  if (filterRegType.value && cats.includes(filterRegType.value)) return
  filterRegType.value = cats.length ? cats[0] : ''
}
// 类型切换：清空多选（防跨类型脏数据残留）+ 重建当前分类列 + 立即刷新列表（发票基准同款）
watch(filterRegType, () => {
  clearSelectionSafe()
  reloadColumns()
  loadFiles(true)
})

// ---- 删除链路 ----
const delPopVisible = ref(false)
const closeDelPop = () => { delPopVisible.value = false }
// 乐观移除本地行：fileIds 整文件移除；pageKeys（`knowledgeId::page`）仅移除单页制度（制度一般用不到）
const optimisticRemoveRegulationRows = (fileIds: Set<string>, pageKeys: Set<string>) => {
  regulationRows.value = regulationRows.value.filter((r) => {
    if (r.knowledgeId && fileIds.has(r.knowledgeId)) return false
    if (r.page && r.page >= 1 && pageKeys.has(`${r.knowledgeId}::${r.page}`)) return false
    return true
  })
}
// 删除走 asynq 异步入队：200 仅代表入队，worker 启动后才置 parse_status=deleting
// （列表据此排除）。立即刷新会读到旧数据，故先乐观移除，再分两次延迟权威对账。
let deleteRefreshTimers: ReturnType<typeof setTimeout>[] = []
const schedulePostDeleteRefresh = () => {
  deleteRefreshTimers.forEach(clearTimeout)
  deleteRefreshTimers = []
  loadRegulationOverview()
  const reload = () => { loadFiles(true); loadRegulationOverview() }
  deleteRefreshTimers.push(setTimeout(reload, 800))
  deleteRefreshTimers.push(setTimeout(reload, 2600))
}
// 删除整份上传文件（含其全部制度记录，从知识库完全删除）
const doFileDelete = async (rows: RegulationRow[]) => {
  const ids = new Set(rows.map(r => r.knowledgeId).filter(Boolean))
  if (ids.size) {
    await batchDeleteKnowledge(kbId.value, Array.from(ids))
    optimisticRemoveRegulationRows(ids as Set<string>, new Set())
  }
}
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  clearSelectionSafe()
  // 异步删除：不立即 loadFiles（会把乐观移除的行又拉回来），交由延迟对账权威纠正
  schedulePostDeleteRefresh()
}
// 制度一份文件一份制度，直接走文件删除
const confirmFileDelete = async () => {
  closeDelPop()
  try { await doFileDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---- 动态列 + 六同步 ----
// 六同步：字段/分类配置变更后重建动态列 + 重载分类（KPI 类型卡/上传弹窗分类同源）+ 刷新记录
const onCategoriesChanged = () => {
  reloadColumns(true)
  loadRegCats()
  loadFiles(true)
  // 分类配置变更（改名/启停用）影响概览类型分桶，同步刷新卡片统计
  loadRegulationOverview()
}
const loadRegCats = async () => {
  try {
    const res: any = await listFleetCategories({ scope: 'regulation' })
    const cats = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    regCats.value = cats
  } catch { regCats.value = [] }
  // 六同步：分类重载后若当前选中项已失效（禁用/改名/删除），回退第一个启用分类
  ensureRegType()
}
// 从后端 categories（scope=regulation）重建动态列；无配置时回退内置默认。
// 列按「当前选中分类」的字段配置生成（六同步：列表/字段筛选器/编辑抽屉/打印同源）。
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    // 分类缓存：切换类型时直接用已拉取的分类重建列（省一次请求、切换更快）；
    // 仅首次进入或设置保存/六同步事件（force=true）时重新请求后端
    let cats: any[] = force ? [] : regCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'regulation' })
      cats = res?.data || []
      regCats.value = cats
    }
    const curCat = cats.find((c: any) => c.name === filterRegType.value && c.enabled !== false)
    const baseCat = curCat || cats[0]
    if (baseCat?.subs?.length) {
      customColumns.value = baseCat.subs
        .filter((s: any) => s.enabled !== false)
        .map((s: any) => {
          const builtin = DEFAULT_REGULATION_COLUMNS.find((b: any) => b.key === s.name)
          return { key: s.name, label: builtin?.label || s.name, default: s.is_default === true, w: builtin?.w || '1fr', width: Number(s.width) || 0 }
        })
      // 六同步：切换分类或设置保存后，按当前分类默认表头重建字段筛选器勾选。
      // 首轮加载（lastColType 为空）时保留用户既有 localStorage 存档，但若存档缺当前分类
      // 的默认列则补齐当前分类默认列（发票基准同款门控）
      if (!lastColType.value) {
        const defaults = customColumns.value.filter((c: any) => c.default).map((c: any) => c.key)
        if (defaults.some((k: string) => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterRegType.value) {
        resetColumns()
      }
      lastColType.value = filterRegType.value
    }
    // 分类重载后若当前选中项失效（被禁用/改名），回退第一个启用分类
    ensureRegType()
  } catch { /* 后端未配置时回退内置默认 */ }
}

// ---- 生命周期 ----
onMounted(() => {
  if (typeof route.query.type === 'string' && route.query.type) filterRegType.value = route.query.type
  loadKb()
  loadRegCats()
  reloadColumns()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  stopPolling()
  stopProgressTimer()
  deleteRefreshTimers.forEach(clearTimeout)
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
})
</script>

<style scoped lang="less">
.regulation-management-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 24px 32px;
  overflow-y: auto;
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

.regulation-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

/* ---- 筛选工具栏 ---- */
.doc-filter-bar {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  &__leading { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; flex: 1; }
  .doc-filter-field {
    display: flex; align-items: center;
    &--search { min-width: 220px; }
    &--wide { min-width: 260px; }
    .doc-search { width: 220px; }
    .doc-type-select { width: 130px; }
    .doc-date-range { width: 260px; }
  }
}

/* ---- 字段筛选弹层 ---- */
:global(.regulation-field-popup) {
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
  flex: 0 1 auto; /* 高度随内容自适应：1 条就包 1 条，多条向下扩展 */
  max-height: 100%;
  min-width: 0;
  overflow-y: auto;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

/* ---- 底部汇总 ---- */
.doc-summary-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 0 2px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .doc-summary-count { font-weight: 600; color: var(--td-text-color-primary); }
  .doc-summary-item {
    display: inline-flex; align-items: baseline; gap: 6px;
    .doc-summary-val { font-variant-numeric: tabular-nums; color: var(--td-text-color-primary); font-weight: 600; }
  }
  &.is-batch-visible { margin-bottom: 72px; } /* 选中时给底部浮动工具栏让位 */
}

.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
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

.cell-regNo, .cell-regName, .cell-dept, .cell-scope, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-regulation-no {
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

/* ---- 底部浮动工具栏 ---- */
.doc-batch-bar-fixed {
  position: fixed; bottom: 24px; left: 50%; transform: translateX(-50%); z-index: 50;
  width: 100%; max-width: 700px; padding: 0 4px; box-sizing: border-box;
}
.batch-bar-inner {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  padding: 8px 12px; background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke); border-radius: 8px; box-shadow: 0 6px 16px rgba(0, 0, 0, 0.12);
}
.batch-bar-left { display: flex; align-items: center; gap: 4px; min-width: 0; flex: 1; }
.batch-bar-count { font-size: 13px; font-weight: 500; color: var(--td-text-color-secondary); white-space: nowrap; }
.batch-bar-clear { flex-shrink: 0; padding: 0 6px !important; height: 28px !important; font-size: 12px; color: var(--td-text-color-secondary) !important; &:hover { color: var(--td-brand-color) !important; } }
.batch-bar-actions { flex-shrink: 0; display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-end; gap: 8px; }
.batch-bar-fade-enter-active, .batch-bar-fade-leave-active { transition: transform 0.2s ease, opacity 0.2s ease; }
.batch-bar-fade-enter-from, .batch-bar-fade-leave-to { opacity: 0; transform: translate(-50%, 6px); }

/* ---- 详情抽屉：竖向区块 + 可拖宽（复用知识库文档抽屉上下滚动样式） ---- */
.regulation-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.regulation-detail-body {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

// 详情抽屉底部取消/保存（对齐发票 invoice-detail-footer）
.regulation-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid var(--td-border-level-1-color);
  margin-top: 8px;
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

</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.regulation-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.regulation-print-dialog {
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
.regulation-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: var(--td-bg-color-container);
}
.regulation-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.regulation-print-close {
  flex-shrink: 0;
}
.regulation-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.regulation-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.regulation-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.regulation-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}

/* 自有批量浮条（发票基准同款） */
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
</style>
