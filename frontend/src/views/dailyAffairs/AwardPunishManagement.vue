<template>
  <div class="notice-management-container">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>通知公告</h2>
        <p class="header-subtitle">通知公告自动归档</p>
      </div>
      <div class="header-actions">
        <t-button v-if="kbId" theme="primary" @click="uploadVisible = true">
          <template #icon><t-icon name="upload" /></template>
          上传公告
        </t-button>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化通知公告..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-通知公告」知识库">
        <template #image><t-icon name="file-copy" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建通知公告知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="notice-main">
      <!-- 概览 KPI 卡 -->
      <DashboardKpiGroup :loading="overviewLoading" :cards="overviewCards" :active-type="filterApType" @card-click="onOverviewCardClick" class="notice-kpi-group" />
      <!-- 筛选工具栏（复用原项目文档列表样式） -->
      <div class="doc-filter-bar">
        <div class="doc-filter-bar__leading">
          <BusinessListToolbar v-model:keyword="keyword" search-placeholder="搜索全部字段"
            @refresh="applyFilter" :selected-count="selectedRowKeys.length"
            @clear-selection="clearSelection" hide-batch-bar>
            <template #type-extra>
              <div class="doc-filter-field doc-filter-field--wide">
                <t-date-range-picker v-model="dateRange" placeholder="发布日期" class="doc-date-range doc-filter-field__control"
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
                上传公告
              </t-button>
            </template>
          </BusinessListToolbar>
        </div>
      </div>

      <!-- 进行中文件状态横幅（发票基准同款：优先上传实时进度轮播，关闭弹窗后由轮询派生解析/提取进度，多文件滚动展示） -->
      <BusinessParseStatusBar :progress="displayProgress" :pending-files="pendingFiles" />

      <!-- 通知公告列表（t-table，可横向滚动，字段可配置） -->
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
        <template #apNo="{ row }: any">
          <span class="row-notice-no" :title="row.apNo || row.fileName">{{ row.apNo }}</span>
        </template>
        <template #apTitle="{ row }: any">
          <span class="row-text" :title="row.apTitle || row.fileName">{{ row.apTitle || row.fileName }}</span>
        </template>
        <template #apType="{ row }: any">
          <span class="row-text" :title="row.apType">{{ row.apType }}</span>
        </template>
        <template #signDate="{ row }: any">
          <span class="row-mono">{{ row.signDate }}</span>
        </template>
        <template #signer="{ row }: any">
          <span class="row-text" :title="row.signer">{{ row.signer }}</span>
        </template>
        <template #issueDept="{ row }: any">
          <span class="row-text" :title="row.issueDept">{{ row.issueDept }}</span>
        </template>
        <template #summary="{ row }: any">
          <span class="row-text" :title="row.summary">{{ row.summary }}</span>
        </template>
        <template #person="{ row }: any">
          <span class="row-text" :title="row.person">{{ row.person }}</span>
        </template>
        <template #dept="{ row }: any">
          <span class="row-text" :title="row.dept">{{ row.dept }}</span>
        </template>
        <template #position="{ row }: any">
          <span class="row-text" :title="row.position">{{ row.position }}</span>
        </template>
        <template #measure="{ row }: any">
          <span class="row-text" :title="row.measure">{{ row.measure }}</span>
        </template>
        <template #basis="{ row }: any">
          <span class="row-text" :title="row.basis">{{ row.basis }}</span>
        </template>
        <template #effectiveDate="{ row }: any">
          <span class="row-mono">{{ row.effectiveDate }}</span>
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
        <template #fileName="{ row }: any">
          <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
        </template>
      </t-table>
    </div>
        </div>

      <!-- 底部汇总（选中记录时显示选中公告汇总，未选中显示全部；选中时避让底部工具栏） -->
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
                <div class="del-pop-title">确定删除所选 {{ selectedRowKeys.length }} 个公告记录吗？删除后不可恢复。</div>
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
    <BusinessKbWizard v-model:visible="wizardVisible" kb-name="日常事务-通知公告" kb-desc="通知公告管理固定使用专用知识库，名称不可修改" description-placeholder="用于存放并解析通知公告文件，自动提取公告字段" default-description="用于存放并解析通知公告文件，自动提取公告字段" model-tip="提取模型将复用下方「对话模型」，用于解析通知公告文件。若列表为空，请先在系统设置中添加模型。" @created="onKbCreated" />
    <!-- 上传弹窗（对齐发票/车队：分类选择 + 拖拽/选择/粘贴 + 进度 + 防重检测） -->
    <FleetUploadDialog v-model:visible="uploadVisible" :kb-id="kbId || ''" scope="award_punish" title="上传公告"
      :type-options="uploadTypeOptions" @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 设置抽屉（字段定义 + 提取规则双 Tab，黄金基准 StandardSettingDrawer） -->
    <StandardSettingDrawer v-model:visible="settingsVisible" :kb-id="kbId || ''" :scope="'award_punish'"
      kb-name="日常事务-通知公告" title="通知公告设置" @saved="() => reloadColumns(true)" />

    <!-- 历史记录（对齐发票基准：已上传文件抽屉，展示解析/提取状态与公告类型） -->
    <FleetUploadHistoryDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" scope="award_punish"
      @changed="onUploadHistoryChanged" />


    <!-- 通知公告详情抽屉（竖向区块，可拖宽，上下滚动） -->
<SettingDrawer v-model:visible="detailVisible" :title="detailTitle" width="654px" :storage-key="'weknora-notice-drawer-width'" hide-footer destroy-on-close class="notice-detail-drawer">
      <div class="notice-detail-body">
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

        <!-- 公告字段 -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">公告信息</div>
                <div class="field-grid">
                  <t-form-item label="文号" label-width="110px">
                    <t-input v-model="editForm.ap_no" placeholder="无文号时自动生成" />
                  </t-form-item>
                  <t-form-item label="标题" label-width="110px">
                    <t-input v-model="editForm.ap_title" placeholder="" />
                  </t-form-item>
                  <t-form-item label="文件类型" label-width="110px">
                    <t-select v-model="editForm.ap_type" :options="noticeTypeOptions" clearable filterable
                      placeholder="选择类型" />
                  </t-form-item>
                  <t-form-item label="发布单位" label-width="110px">
                    <t-input v-model="editForm.issue_dept" placeholder="" />
                  </t-form-item>
                  <t-form-item label="发布人" label-width="110px">
                    <t-input v-model="editForm.signer" placeholder="" />
                  </t-form-item>
                  <t-form-item label="发布日期" label-width="110px">
                    <t-date-picker v-model="editForm.sign_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                  <t-form-item label="部门" label-width="110px">
                    <t-input v-model="editForm.dept" placeholder="" />
                  </t-form-item>
                  <t-form-item label="当事人" label-width="110px">
                    <t-input v-model="editForm.person" placeholder="" />
                  </t-form-item>
                  <t-form-item label="岗位" label-width="110px">
                    <t-input v-model="editForm.position" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">处理内容</div>
                <div class="field-grid">
                  <t-form-item label="生效日期" label-width="110px">
                    <t-date-picker v-model="editForm.effective_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">处理决定</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="处理决定" label-width="110px">
                    <t-textarea v-model="editForm.measure" :autosize="{ minRows: 2, maxRows: 6 }" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">依据</div>
                <div class="field-grid field-grid--full">
                  <t-form-item label="依据" label-width="110px">
                    <t-textarea v-model="editForm.basis" :autosize="{ minRows: 2, maxRows: 6 }" placeholder="" />
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
                <span>字段修改后点击底部保存{{ autoSaving ? '（保存中...）' : '' }}</span>
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
        <div class="notice-detail-footer">
          <t-button variant="outline" size="small" @click="detailVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="autoSaving" @click="saveEditForm()">保存</t-button>
        </div>
      </div>
    </SettingDrawer>

    <!-- 文号重复二次确认 -->
    <t-dialog v-model:visible="dupConfirmVisible" header="文号重复" width="420" @confirm="onDupConfirm">
      <span>系统已存在相同的文号「{{ dupNo }}」，是否强制保存？</span>
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
      <div v-if="printVisible" class="notice-print-mask">
        <div class="notice-print-dialog">
          <div class="notice-print-header">
            <span class="notice-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 份）` }}</span>
            <t-button variant="text" size="small" class="notice-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="notice-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并公告 PDF…'" />
            </div>
          </div>
          <div class="notice-print-footer">
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
  listAwardPunishRecords,
  listAwardPunishTypes,
  previewKnowledgeFile,
  reparseKnowledge,
  getRecognitionConfig,
  listAwardPunishOverview,
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

const KB_NAME = '日常事务-通知公告'
const ACCEPT_TYPES = ['pdf', 'jpg', 'jpeg', 'png']
const PAGE_SIZE = 20

// 文件类型枚举（编辑/筛选推荐值）
const NOTICE_TYPES = ['通知', '公告', '通报', '决定', '其它通知']

// 字段定义（列显隐设置）
import { useBusinessList, type ColumnDef } from '@/composables/useBusinessList'
import { useDocStatus } from '@/composables/useDocStatus'
const DEFAULT_NOTICE_COLUMNS: ColumnDef[] = [
  { key: 'apNo', label: '文号', default: true, w: '1.4fr' },
  { key: 'apTitle', label: '标题', default: true, w: '1.8fr' },
  { key: 'apType', label: '文件类型', default: true, w: '1fr' },
  { key: 'signDate', label: '发布日期', default: true, w: '1.1fr' },
  { key: 'signer', label: '发布人', default: true, w: '1fr' },
  { key: 'issueDept', label: '发布单位', default: true, w: '1.3fr' },
  { key: 'summary', label: '摘要', default: true, w: '2fr' },
  { key: 'person', label: '当事人', default: false, w: '1fr' },
  { key: 'dept', label: '部门', default: false, w: '1.2fr' },
  { key: 'position', label: '岗位', default: false, w: '1fr' },
  { key: 'measure', label: '处理决定', default: false, w: '1.8fr' },
  { key: 'basis', label: '依据', default: false, w: '1.6fr' },
  { key: 'effectiveDate', label: '生效日期', default: false, w: '1.1fr' },
  { key: 'remark', label: '备注', default: false, w: '1.4fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const {
  customColumns, effectiveColumns,
  visibleColKeys, visibleColDefs,
  resetColumns, selectAllColumns, colVisible,
  selectedRowKeys, onSelectChange, clearSelection,
  colValue,
} = useBusinessList({
  storageKey: 'weknora-notice-list-columns',
  defaultColumns: DEFAULT_NOTICE_COLUMNS,
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

interface NoticeItem {
  ap_no?: string
  ap_title?: string
  ap_type?: string
  issue_dept?: string
  person?: string
  dept?: string
  position?: string
  measure?: string
  basis?: string
  signer?: string
  sign_date?: string
  effective_date?: string
  remark?: string
  summary?: string
  page?: number
}

interface NoticeRow extends Record<string, any> {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  parseStatus: string
  summaryStatus?: string
  extractStatus: string
  kind: 'award_punish' | 'not_award_punish' | 'unknown' | 'pending'
  apNo?: string
  apTitle?: string
  apType?: string
  issueDept?: string
  person?: string
  dept?: string
  position?: string
  measure?: string
  basis?: string
  signer?: string
  signDate?: string
  effectiveDate?: string
  remark?: string
  summary?: string
  page?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}

const items = ref<KnowledgeItem[]>([])
const noticeRows = shallowRef<NoticeRow[]>([])

// KPI 卡：总数、本月新增、按类型分布（后端全量概览 noticeOverview）
const overviewStats = ref<any>(null)
const overviewLoading = ref(true)
const loadNoticeOverview = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listAwardPunishOverview(kbId.value)
    overviewStats.value = res?.data || null
  } catch (e: any) {
    overviewStats.value = null
  } finally {
    overviewLoading.value = false
  }
}
// ---- KPI 概览卡（黄金基准 DashboardKpiGroup：全部公告 → 各文件类型卡 → 其它通知 → 历史记录） ----
const overviewCards = computed<KpiCard[]>(() => {
  const st = overviewStats.value || {}
  const total = Number(st.total || 0)
  const monthCount = Number(st.current_month || 0)
  const parseFailed = Number(st.parse_failed || 0)
  const fileCount = Number(st.file_count || 0)
  // 分类统计：后端按 ap_type 精确聚合（空归「未分类」）
  const catCount: Record<string, number> = {}
  for (const c of (st.by_category || [])) catCount[String(c.category)] = Number(c.count) || 0
  const cards: KpiCard[] = [
    // 全部公告：全量总份数（含未分类/其它通知与待补录占位行），点击清空筛选进入全部态
    { key: 'total', label: '已收录公告', icon: 'file', value: String(total), unit: '份', sub: `本月新增 ${monthCount} 份`, theme: 'brand', action: 'all' },
  ]
  // 类型分布卡：以通知公告分类列表（categories[scope=award_punish]）为权威源动态生成，顺序 = 分类数组顺序
  const enabledCats = (noticeCats.value || []).filter((c: any) => c.enabled !== false)
  let catSum = 0
  for (const c of enabledCats) {
    // 「其它通知」为系统内置兜底分类：排除式桶，跳过该分类卡且不计入 catSum
    if (c.name === '其它通知') continue
    const cnt = catCount[String(c.name)] || 0
    catSum += cnt
    cards.push({ key: `type-${c.name}`, label: c.name, icon: 'file-1', value: String(cnt), unit: '份', theme: 'neutral', action: 'type', typeValue: c.name })
  }
  // 「其它通知」排除式统计桶（收纳未匹配任何启用分类的记录，含未分类/已停用分类存量），无条件渲染
  const otherCount = Math.max(0, total - catSum)
  cards.push({ key: 'type-其它通知', label: '其它通知', icon: 'file-unknown', value: String(otherCount), unit: '份', theme: 'neutral', action: 'type', typeValue: '其它通知' })
  // 历史记录（已上传文件）：异常文件数 sub 展示，点击打开已上传文件抽屉
  cards.push({ key: 'history', label: '历史记录', icon: 'history', value: String(fileCount), unit: '份', sub: parseFailed ? `${parseFailed} 份异常` : '无异常文件', theme: parseFailed > 0 ? 'warning' : 'neutral', action: 'history' })
  return cards
})
// 卡片点击联动
const onOverviewCardClick = (card: KpiCard) => {
  if (card.action === 'all') {
    filterApType.value = ''
  } else if (card.action === 'type' && card.typeValue) {
    filterApType.value = card.typeValue
  } else if (card.action === 'history') {
    historyVisible.value = true
  }
}
const noticeSummary = ref<{ total: number }>({ total: 0 })
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const filterApType = ref('')
const filterFulfillStatus = ref('')
const noticeTypeOptions = ref<Array<{ value: string; label: string }>>([])
const noticeCats = ref<any[]>([])
const uploadTypeOptions = computed(() => {
  const cats = noticeCats.value.filter((c: any) => c.enabled !== false)
  if (!cats.length) return [{ label: '通知', value: 'award_punish__通知' }]
  return cats.map((c: any) => ({ label: c.name, value: `award_punish__${c.name}` }))
})
const onUploadDone = async () => {
  await Promise.all([loadFiles(true), loadNoticeOverview()])
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

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<NoticeRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const previewExpanded = ref<string[]>([])
const currentRow = ref<NoticeRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({})
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
      await loadNoticeTypes()
      await cleanNonNoticeFiles()
      await loadFiles()
      await loadNoticeOverview()
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
    await loadNoticeTypes()
    await cleanNonNoticeFiles()
    await loadFiles()
    await loadNoticeOverview()
    startPolling()
  }
}

// ---- 存量非通知公告清理：扫描知识库中已标记 not_award_punish 的文件并自动删除 ----
let cleaningNonNotice = false
const cleanNonNoticeFiles = async () => {
  if (!kbId.value || cleaningNonNotice) return
  cleaningNonNotice = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_award_punish' || it.custom_metadata?.extract_status === 'not_award_punish')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非通知公告文件（旧数据清理）`)
      loadNoticeTypes()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonNotice = false }
}

// ---- 文件类型下拉：从识别规则配置的分类列表加载（含自定义分类），
// 未配置时兜底用知识库现有类型 + 枚举 ----
const loadNoticeTypes = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    const types: string[] = Array.isArray(c?.types) ? c.types.filter(Boolean) : []
    if (types.length) {
      noticeTypeOptions.value = types.map(t => ({ value: t, label: t }))
      return
    }
  } catch { /* 走兜底 */ }
  try {
    const res: any = await listAwardPunishTypes(kbId.value)
    const list = res?.data || res?.list || []
    const arr = Array.isArray(list) ? list : []
    const dynamic = arr.map((r: any) => ({ value: r.ap_type, label: r.ap_type }))
    // 动态类型在前，未出现过的枚举类型补在后（allow 自定义）
    const seen = new Set(dynamic.map((d: any) => d.value))
    const fixed = NOTICE_TYPES.filter(t => !seen.has(t)).map(t => ({ value: t, label: t }))
    noticeTypeOptions.value = [...dynamic, ...fixed]
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

const openTagEdit = (row: NoticeRow) => {
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


// ---- 列表加载（公告级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    noticeRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) {
    return
  } else {
    loadingMore.value = true
  }
  try {
    // 「其它通知」为排除式筛选：附上启用分类名集合，后端按 ap_type 不属于该集合过滤
    const knownCats = noticeCats.value
      .filter((c: any) => c.enabled !== false)
      .map((c: any) => String(c.name))
      .filter((n: string) => Boolean(n))
    const res: any = await listAwardPunishRecords(kbId.value, {
      q: keyword.value || undefined,
      ap_type: filterApType.value || undefined,
      known_categories: filterApType.value === '其它通知' && knownCats.length > 0
        ? knownCats.join(',') : undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    noticeSummary.value = { total }
    const existing = new Set(noticeRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapNoticeRecord).filter(r => !existing.has(r.rowKey))
    noticeRows.value = [...noticeRows.value, ...fresh]
    hasMore.value = noticeRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '通知公告列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

// 把后端公告级记录（AwardPunishRecord）映射为前端行
const mapNoticeRecord = (r: any): NoticeRow => ({
  rowKey: r.rowKey || `${r.knowledge_id || 'kb'}-${r.page || 0}-${r.ap_no || ''}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || r.fileType || '',
  parseStatus: 'completed',
  summaryStatus: '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: 'award_punish',
  tags: Array.isArray(r.tags) ? r.tags.map((t: any) => (typeof t === 'string' ? { id: t, name: t } : t)) : [],
  description: '',
  apNo: r.ap_no || '',
  apTitle: r.ap_title || '',
  apType: r.ap_type || '',
  issueDept: r.issue_dept || '',
  person: r.person || '',
  dept: r.dept || '',
  position: r.position || '',
  measure: r.measure || '',
  basis: r.basis || '',
  signer: r.signer || '',
  signDate: r.sign_date || '',
  effectiveDate: r.effective_date || '',
  remark: r.remark || '',
  summary: r.summary || '',
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 公告级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

const parseCustomMetadata = (item: KnowledgeItem): NoticeRow[] => {
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
  if (kind === 'not_award_punish') {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus: 'not_award_punish', extractError, kind, ...base }]
  }
  const records = Array.isArray(meta.records) ? meta.records : []
  if (records.length === 0) {
    return [{ rowKey: item.id, knowledgeId: item.id, fileName, extractStatus, extractError, kind, ...base }]
  }
  return records.map((ct: any, idx: number) => ({
    rowKey: `${item.id}-${idx}`,
    knowledgeId: item.id,
    fileName: records.length > 1 ? `${fileName}（第${idx + 1}份）` : fileName,
    multiIndex: records.length > 1 ? `${idx + 1}/${records.length}` : undefined,
    extractStatus,
    extractError,
    kind,
    ...base,
    apNo: ct.ap_no || '',
    apTitle: ct.ap_title || '',
    apType: ct.ap_type || '',
    issueDept: ct.issue_dept || '',
    person: ct.person || '',
    dept: ct.dept || '',
    position: ct.position || '',
    measure: ct.measure || '',
    basis: ct.basis || '',
    signer: ct.signer || '',
    signDate: ct.sign_date || '',
    effectiveDate: ct.effective_date || '',
    remark: ct.remark || '',
    summary: ct.summary || '',
    page: Number(ct.page) || 0,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 顶部合并"进行中文件"状态行
const pendingRows = computed<NoticeRow[]>(() => pendingFiles.value.map((k) => {
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
  } as NoticeRow
}))
const filteredRows = computed(() => [...pendingRows.value, ...noticeRows.value])


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
  const byKey = new Map(noticeRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as NoticeRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中公告汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  return { total: rows.length }
})
const displaySummary = computed(() => selectedSummary.value || noticeSummary.value)

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

// ---- 轮询解析 + 提取（统一 composable） ----
const {
  extractInFlight, extractFailed, pendingFiles,
  start: startPolling, stop: stopPolling, scanOnce,
} = useBusinessPolling({
  kbId,
  extractFn: (kid, fileId) => extractBusinessDocument(kid, fileId, 'award_punish'),
  onPendingFiles: (files) => {
    // 状态条展示所有「进行中」文件（含正处于重新解析/重新提取中的文件）
    pendingFiles.value = files
  },
  onRemoved: (item) => {
    MessagePlugin.info(`「${item.file_name || item.title}」不是通知公告文件，已移至删除历史，可在删除历史中恢复`)
    loadNoticeTypes()
  },
  onTick: () => loadFiles(),
})

// pendingFiles 集合变化即触发概览统计重载
let lastPendingKey = ''
watch(pendingFiles, (v) => {
  const key = v.map((f: any) => f.id).sort().join(',')
  if (key !== lastPendingKey) {
    lastPendingKey = key
    if (kbId.value) loadNoticeOverview()
  }
}, { deep: false })

// 上传任务实时进度（来自上传弹窗 @progress）：多文件同时进行时轮播展示
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
// 表格里该文件已「提取完成/失败」的，不再保留旧进度项
function filterLiveProgress(items: any[]) {
  const doneNames = new Set(
    noticeRows.value
      .filter((r) => r.extractStatus === 'success' || r.extractStatus === 'failed')
      .map((r) => r.fileName),
  )
  return items.filter((it: any) => !doneNames.has(it.name))
}
// 轮询刷新列表后：用表格实际 extractStatus 清掉已完成文件的残留进度
watch(noticeRows, () => {
  if (!uploadProgress.value.length) return
  uploadProgress.value = filterLiveProgress(uploadProgress.value)
})
function stopProgressTimer() {
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}
onBeforeUnmount(stopProgressTimer)

// 解析/提取实时进度（弹窗关闭后兜底数据源），按耗时估算
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
// 文件离开进行中集合后清理起点记忆
watch(pendingFiles, (v) => {
  const alive = new Set(v.map((f: any) => f.id))
  for (const id of Array.from(fileStartAt.keys())) {
    if (!alive.has(id)) fileStartAt.delete(id)
  }
})
// 弹窗打开期间优先展示上传弹窗上报的真实进度；关闭后切换到轮询派生进度
const displayProgress = computed(() =>
  uploadProgress.value.length ? uploadProgress.value : liveProgress.value,
)

const { extractStatusOf, statusOf, summaryState, rowTags } = useDocStatus({
  extractInFlight, extractFailed, currentDetail,
  scope: 'award_punish', notLabel: '非通知公告',
})

// ---- 详情抽屉 ----
const openDetail = async (row: NoticeRow) => {
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
        // 通知公告一份文件一份：优先按文号匹配，再按标题匹配
        const match = parsed.find(p => p.apNo && p.apNo === row.apNo)
          || parsed.find(p => p.apTitle === row.apTitle) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.apNo ? `公告详情 · ${currentRow.value.apNo}` : '公告详情'
)

// 上传历史抽屉（删除/恢复）变更后同步
const onUploadHistoryChanged = async (payload?: { deletedIds?: string[] }) => {
  if (payload?.deletedIds?.length) {
    optimisticRemoveNoticeRows(new Set(payload.deletedIds), new Set())
    schedulePostDeleteRefresh()
    return
  }
  await Promise.all([loadFiles(true), loadNoticeOverview()])
}

// ---- 字段编辑 + 手动保存 ----
const fillEditForm = () => {
  const r = currentRow.value
  if (!r) return
  editForm.value = {
    ap_no: r.apNo || '',
    ap_title: r.apTitle || '',
    ap_type: r.apType || '',
    issue_dept: r.issueDept || '',
    person: r.person || '',
    dept: r.dept || '',
    position: r.position || '',
    measure: r.measure || '',
    basis: r.basis || '',
    signer: r.signer || '',
    sign_date: r.signDate || '',
    effective_date: r.effectiveDate || '',
    remark: r.remark || '',
    summary: r.summary || '',
  }
}

// 文号重复二次确认
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
    const records = Array.isArray(meta.records) ? [...meta.records] : []
    const updated: any = {
      ap_no: editForm.value.ap_no || '',
      ap_title: editForm.value.ap_title || '',
      ap_type: editForm.value.ap_type || '',
      issue_dept: editForm.value.issue_dept || '',
      person: editForm.value.person || '',
      dept: editForm.value.dept || '',
      position: editForm.value.position || '',
      measure: editForm.value.measure || '',
      basis: editForm.value.basis || '',
      signer: editForm.value.signer || '',
      sign_date: editForm.value.sign_date || '',
      effective_date: editForm.value.effective_date || '',
      remark: editForm.value.remark || '',
      summary: editForm.value.summary || '',
    }
    const nowPage = now.page && now.page >= 1 ? now.page : 0
    let targetIdx = -1
    if (nowPage >= 1) {
      targetIdx = records.findIndex((ct: any, i: number) =>
        i === nowPage - 1 || Number(ct.page) === nowPage || ct.ap_no === now.apNo)
    }
    if (targetIdx < 0) targetIdx = records.findIndex((ct: any) => ct.ap_no === now.apNo)
    if (targetIdx >= 0) records[targetIdx] = { ...records[targetIdx], ...updated, page: nowPage || records[targetIdx]?.page }
    else records.push({ ...updated, page: nowPage || 0 })
    // 无任何有效字段 → 保持"待补录"状态，记录不因空数据而从列表消失
    const hasNoticeData = records.some((c: any) =>
      (c.ap_no || '').trim() || (c.ap_title || '').trim() || (c.ap_type || '').trim() ||
      (c.issue_dept || '').trim() || (c.signer || '').trim() || (c.sign_date || '').trim())
    const nextStatus = hasNoticeData ? 'success' : 'manual'
    const nextError = hasNoticeData ? '' : '人工入库待编辑，请补充字段'
    const newMeta = { ...meta, kind: 'award_punish', records, extract_status: nextStatus, extract_error: nextError }
    // records 为数组型元数据，原生 PUT /knowledge/:id 标量校验会 500；走二开通用持久化路由
    try {
      await updateBusinessMetadata(kbId.value, now.knowledgeId, newMeta, force)
    } catch (e: any) {
      if (e?.status === 409 && String(e?.message || '').startsWith('DUPLICATE_AP_NO')) {
        dupNo.value = String(e?.message || '').split(':').pop() || ''
        dupConfirmVisible.value = true
        return
      }
      throw e
    }
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行（shallowRef 铁律：map 后整体替换数组）
    const listIdx = noticeRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      noticeRows.value = noticeRows.value.map((r: any, i: number) =>
        i === listIdx ? {
          ...r,
          extractStatus: nextStatus,
          extractError: nextError,
          apNo: updated.ap_no,
          apTitle: updated.ap_title,
          apType: updated.ap_type,
          issueDept: updated.issue_dept,
          signer: updated.signer,
          signDate: updated.sign_date,
          dept: updated.dept,
          person: updated.person,
        } : r,
      )
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    autoSaving.value = false
  }
}

// ---- 浮动工具栏 ----
const selectedSingle = computed(() => {
  const rows = selectedRows.value
  return rows.length === 1 ? rows[0] : null
})

// 批量重解析/重提取：一个文件一般只有一份公告，选中即对该文件操作
const reparseBusy = ref(false)
const extractBusy = ref(false)
const BATCH_CONCURRENCY = 3

// 轻量并发池：以 limit 上限消费 items，返回与入参同序的 allSettled 结果（避免引入第三方库）
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
    return `确定按最新提取规则重新提取「${name}」的所有公告字段吗？将覆盖已有提取结果。`
  }
  return `确定按最新提取规则重新提取选中的 ${ids.length} 个文件的所有公告字段吗？将覆盖已有提取结果。`
})

// 重新解析：物理文件重新拆页/解析（支持多文件批量，二次确认）
const handleReparse = async () => {
  const ids = selectedIds.value
  if (!ids.length) { MessagePlugin.info('请先选中要重新解析的公告'); return }
  reparseBusy.value = true
  uploadProgress.value = []
  let ok = 0
  let fail = 0
  try {
    const results = await settleInBatches(ids, BATCH_CONCURRENCY, kid => reparseKnowledge(kid))
    results.forEach(r => r.status === 'fulfilled' ? ok++ : fail++)
    if (fail === 0) {
      MessagePlugin.success(`已提交 ${ok} 个文件重新解析`)
    } else {
      MessagePlugin.warning(`${ok} 个文件重新解析已提交，${fail} 个提交失败`)
    }
    scanOnce()
    loadFiles(true)
    loadNoticeOverview()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重新解析失败')
  } finally {
    reparseBusy.value = false
  }
}

// 重新提取：大模型/OCR 按最新提取规则重新提取选中文件全部公告字段（支持多文件批量，二次确认）
const handleFileExtract = async () => {
  const all = selectedIds.value
  if (!all.length) { MessagePlugin.info('请先选中要重新提取的公告'); return }
  const ids = all.filter(kid => !extractInFlight.value.has(kid))
  const skipped = all.length - ids.length
  if (!ids.length) {
    MessagePlugin.info('所选文件均正在提取中，请稍候')
    return
  }
  ids.forEach(kid => { extractInFlight.value.add(kid); extractFailed.value.delete(kid) })
  extractBusy.value = true
  uploadProgress.value = []
  try {
    const results = await settleInBatches(
      ids,
      BATCH_CONCURRENCY,
      kid => extractBusinessDocument(kbId.value, kid, 'award_punish', filterApType.value || undefined),
    )
    let ok = 0
    let conflict = 0
    results.forEach((r, i) => {
      if (r.status === 'fulfilled') { ok++; return }
      // 409 冲突：属重复提交而非提取失败，不写入 extractFailed
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
    scanOnce()
    loadFiles(true)
    loadNoticeOverview()
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

// 打印：公告一个文件一般只有一份，打印预览整个文档，支持跨文件合并
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
      if (!blob) { notes.push(`${r.apNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
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
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
        if (pageCount > 1) notes.push(`${r.apNo || r.fileName}（共 ${pageCount} 页）`)
      } catch {
        notes.push(`${r.apNo || r.fileName}（页抽取失败，已并入整文件）`)
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
      MessagePlugin.error('未获取到可打印的公告页面')
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

// 打印：调用预览 iframe 内的 PDF 查看器打印
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement | null
  if (frame?.contentWindow) {
    try { frame.contentWindow.print(); return } catch { /* fallback */ }
  }
  window.print()
}

// 目录生成：把选中记录按「当前列表展示字段」生成表格式清单 PDF
const catalogValueOf = (row: NoticeRow, key: string): string => {
  switch (key) {
    case 'apNo': return row.apNo || ''
    case 'apTitle': return row.apTitle || ''
    case 'apType': return row.apType || ''
    case 'signDate': return row.signDate || ''
    case 'signer': return row.signer || ''
    case 'issueDept': return row.issueDept || ''
    case 'summary': return row.summary || ''
    case 'person': return row.person || ''
    case 'dept': return row.dept || ''
    case 'position': return row.position || ''
    case 'measure': return row.measure || ''
    case 'basis': return row.basis || ''
    case 'effectiveDate': return row.effectiveDate || ''
    case 'remark': return row.remark || ''
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
    const bytes = await generateCatalogPdf({ title: '通知公告目录', columns: cols, rows })
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
  a.download = `通知公告目录-${Date.now()}.pdf`
  a.click()
}

// 安全清空多选
const clearSelectionSafe = () => {
  clearSelection()
}
// 类型筛选锁定：空值 = 「全部」态（放行）；「其它通知」为排除式筛选态（放行）；
// 当前选中项合法则保留，否则按分类顺序选中第一个启用分类
const ensureApType = () => {
  if (filterApType.value === '其它通知' || filterApType.value === '') return
  const cats = noticeCats.value.filter((c: any) => c.enabled !== false).map((c: any) => c.name)
  if (filterApType.value && cats.includes(filterApType.value)) return
  filterApType.value = cats.length ? cats[0] : ''
}
// 类型切换：清空多选 + 重建当前分类列 + 立即刷新列表
watch(filterApType, () => {
  clearSelectionSafe()
  reloadColumns()
  loadFiles(true)
})

// ---- 删除链路 ----
const delPopVisible = ref(false)
const closeDelPop = () => { delPopVisible.value = false }
// 乐观移除本地行
const optimisticRemoveNoticeRows = (fileIds: Set<string>, pageKeys: Set<string>) => {
  noticeRows.value = noticeRows.value.filter((r) => {
    if (r.knowledgeId && fileIds.has(r.knowledgeId)) return false
    if (r.page && r.page >= 1 && pageKeys.has(`${r.knowledgeId}::${r.page}`)) return false
    return true
  })
}
// 删除走 asynq 异步入队：先乐观移除，再分两次延迟权威对账
let deleteRefreshTimers: ReturnType<typeof setTimeout>[] = []
const schedulePostDeleteRefresh = () => {
  deleteRefreshTimers.forEach(clearTimeout)
  deleteRefreshTimers = []
  loadNoticeOverview()
  const reload = () => { loadFiles(true); loadNoticeOverview() }
  deleteRefreshTimers.push(setTimeout(reload, 800))
  deleteRefreshTimers.push(setTimeout(reload, 2600))
}
// 删除整份上传文件（含其全部公告记录）
const doFileDelete = async (rows: NoticeRow[]) => {
  const ids = new Set(rows.map(r => r.knowledgeId).filter(Boolean))
  if (ids.size) {
    await batchDeleteKnowledge(kbId.value, Array.from(ids))
    optimisticRemoveNoticeRows(ids as Set<string>, new Set())
  }
}
const finishDelete = async () => {
  MessagePlugin.success('删除成功')
  clearSelectionSafe()
  schedulePostDeleteRefresh()
}
// 一份文件一份公告，直接走文件删除
const confirmFileDelete = async () => {
  closeDelPop()
  try { await doFileDelete(selectedRows.value); await finishDelete() } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
}

// ---- 动态列 + 六同步 ----
const onCategoriesChanged = () => {
  reloadColumns(true)
  loadNoticeCats()
  loadFiles(true)
  loadNoticeOverview()
}
const loadNoticeCats = async () => {
  try {
    const res: any = await listFleetCategories({ scope: 'award_punish' })
    const cats = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    noticeCats.value = cats
  } catch { noticeCats.value = [] }
  ensureApType()
}
// 从后端 categories（scope=award_punish）重建动态列；无配置时回退内置默认
const lastColType = ref('')
const reloadColumns = async (force = false) => {
  try {
    let cats: any[] = force ? [] : noticeCats.value
    if (!cats || !cats.length) {
      const res: any = await listFleetCategories({ scope: 'award_punish' })
      cats = res?.data || []
      noticeCats.value = cats
    }
    const curCat = cats.find((c: any) => c.name === filterApType.value && c.enabled !== false)
    const baseCat = curCat || cats[0]
    if (baseCat?.subs?.length) {
      customColumns.value = baseCat.subs
        .filter((s: any) => s.enabled !== false)
        .map((s: any) => {
          const builtin = DEFAULT_NOTICE_COLUMNS.find((b: any) => b.key === s.name)
          return { key: s.name, label: builtin?.label || s.name, default: s.is_default === true, w: builtin?.w || '1fr', width: Number(s.width) || 0 }
        })
      if (!lastColType.value) {
        const defaults = customColumns.value.filter((c: any) => c.default).map((c: any) => c.key)
        if (defaults.some((k: string) => !visibleColKeys.value.includes(k))) resetColumns()
      } else if (force || lastColType.value !== filterApType.value) {
        resetColumns()
      }
      lastColType.value = filterApType.value
    }
    ensureApType()
  } catch { /* 后端未配置时回退内置默认 */ }
}

// ---- 生命周期 ----
onMounted(() => {
  if (typeof route.query.type === 'string' && route.query.type) filterApType.value = route.query.type
  loadKb()
  loadNoticeCats()
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
.notice-management-container {
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

.notice-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

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
:global(.notice-field-popup) {
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
  flex: 0 1 auto;
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
  &.is-batch-visible { margin-bottom: 72px; }
}

.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
}

.cell {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  padding: 0 8px;
  text-align: center;
}

.row-notice-no {
  min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  font-size: 13px; font-weight: 600; color: var(--td-text-color-primary);
}
.row-text { min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-mono { font-variant-numeric: tabular-nums; font-size: 12px; color: var(--td-text-color-secondary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; min-width: 0; }
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

/* ---- 详情抽屉：竖向区块 + 可拖宽 ---- */
.notice-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.notice-detail-body {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.notice-detail-footer {
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

// 摘要（复用知识库文档抽屉样式：线框 + 展开/折叠）
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

/* 列内边距收紧 */
.doc-list-view :deep(.t-table__cell),
.doc-list-view :deep(.t-table__th) {
  padding-left: 8px;
  padding-right: 8px;
}
</style>

<style lang="less">
/* 打印预览弹窗：自定义实现，Teleport 到 body，全局样式 */
.notice-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.notice-print-dialog {
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
.notice-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: var(--td-bg-color-container);
}
.notice-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.notice-print-close {
  flex-shrink: 0;
}
.notice-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: var(--td-bg-color-container);
}
.notice-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.notice-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.notice-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: var(--td-bg-color-container);
  border-top: 1px solid var(--td-component-stroke);
}
</style>
