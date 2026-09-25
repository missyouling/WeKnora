<template>
  <div class="awardPunish-management-container">
    <!-- 顶部：标题 + 上传按钮（唯一上传入口） -->
    <div class="header">
      <div class="header-title">
        <h2>奖惩管理</h2>
        <p class="header-subtitle">上传奖惩文件，自动识别文号、类型、当事人与措施等字段并归档，支持查询、编辑、打印、下载与删除</p>
      </div>
      <div class="header-actions">
        <t-button v-if="kbId" theme="primary" @click="triggerUpload">
          <template #icon><t-icon name="upload" /></template>
          上传奖惩
        </t-button>
      </div>
      <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
        @change="onFileInputChange" />
    </div>

    <!-- 加载中 -->
    <div v-if="loading && !kbId" class="loading-area">
      <t-loading size="large" text="正在初始化奖惩管理..." />
    </div>

    <!-- KB 不存在：空状态 -->
    <div v-else-if="!kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-奖惩」知识库">
        <template #image><t-icon name="file-copy" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建奖惩知识库</t-button>
    </div>

    <!-- KB 存在：主界面 -->
    <div v-else class="awardPunish-main">
      <!-- 筛选工具栏（复用原项目文档列表样式） -->
      <div class="doc-filter-bar">
        <div class="doc-filter-bar__leading">
          <div class="doc-filter-field doc-filter-field--search">
            <t-input v-model="keyword" placeholder="搜索全部字段" clearable class="doc-search doc-filter-field__control"
              @enter="onKeywordChange" @clear="onKeywordChange">
              <template #prefixIcon><t-icon name="search" size="16px" /></template>
            </t-input>
          </div>
          <div class="doc-filter-field">
            <t-select v-model="filterApType" :options="apTypeOptions" placeholder="奖惩类型"
              class="doc-type-select doc-filter-field__control" clearable @change="applyFilter">
              <template #prefixIcon><t-icon name="file" size="16px" /></template>
            </t-select>
          </div>
          <div class="doc-filter-field doc-filter-field--wide">
            <t-date-range-picker v-model="dateRange" placeholder="签发日期" class="doc-date-range doc-filter-field__control"
              clearable allow-input @change="applyFilter">
              <template #prefixIcon><t-icon name="time" size="16px" /></template>
            </t-date-range-picker>
          </div>
          <!-- 字段筛选（列显隐设置） -->
          <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
            overlay-inner-class="awardPunish-field-popup">
            <t-button variant="outline" size="small">
              <template #icon><t-icon name="view-list" size="14px" /></template>
              字段
            </t-button>
            <template #content>
              <div class="field-popup-content">
                <div class="field-popup-head">
                  <span class="field-popup-title">显示字段</span>
                  <div class="field-popup-actions">
                    <t-button variant="text" size="small" @click="selectAllColumns">全选</t-button>
                    <t-button variant="text" size="small" @click="resetColumns">重置</t-button>
                  </div>
                </div>
                <t-checkbox-group v-model="visibleColKeys" class="field-popup-list" @change="persistColumns">
                  <t-checkbox v-for="col in columnDefs" :key="col.key" :value="col.key" class="field-popup-item">
                    {{ col.label }}
                  </t-checkbox>
                </t-checkbox-group>
              </div>
            </template>
          </t-popup>
          <t-button variant="outline" size="small" @click="applyFilter">
            <template #icon><t-icon name="refresh" size="14px" /></template>
          </t-button>
          <t-tooltip content="设置" placement="bottom">
            <t-button variant="outline" size="small" @click="recognitionVisible = true">
              <template #icon><t-icon name="setting" size="14px" /></template>
            </t-button>
          </t-tooltip>
          <t-tooltip content="删除历史" placement="bottom">
            <t-button variant="outline" size="small" @click="historyVisible = true">
              <template #icon><t-icon name="history" size="14px" /></template>
            </t-button>
          </t-tooltip>
        </div>
      </div>

      <!-- 奖惩列表（自绘 grid，可横向滚动，字段可配置） -->
      <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
        <div class="doc-list-view">
          <div class="doc-list-header" :style="gridStyle" role="row">
            <div class="cell cell-check" role="columnheader" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
                :disabled="!selectableRows.length" title="全选" @change="toggleSelectAll" />
            </div>
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
              {{ col.label }}
            </div>
          </div>
          <div class="doc-list-body">
            <div v-for="row in filteredRows" :key="row.rowKey" class="doc-list-row" :style="gridStyle"
              :class="{ selected: selectedRowKeys.includes(row.rowKey) }" role="row" @click="openDetail(row)">
              <div class="cell cell-check" @click.stop>
                <t-checkbox class="doc-list-check" size="small" :checked="selectedRowKeys.includes(row.rowKey)"
                  :disabled="row.kind === 'pending'" @change="(c: boolean) => toggleRow(row.rowKey, c)" />
              </div>
                            <!-- 数据列（与表头同源 v-for，顺序自动对齐） -->
              <template v-for="col in visibleColDefs" :key="col.key">
                <div v-if="col.key === 'apNo'" class="cell cell-apNo">
                  <span class="row-awardPunish-no" :title="row.apNo || row.fileName">{{ row.apNo }}</span>
                </div>
                <div v-else-if="col.key === 'apTitle'" class="cell cell-apTitle">
                  <span class="row-text" :title="row.apTitle || row.fileName">{{ row.apTitle || row.fileName }}</span>
                </div>
                <div v-else-if="col.key === 'apType'" class="cell cell-apType">
                  <span class="row-text" :title="row.apType">{{ row.apType }}</span>
                </div>
                <div v-else-if="col.key === 'person'" class="cell cell-person">
                  <span class="row-text" :title="row.person">{{ row.person }}</span>
                </div>
                <div v-else-if="col.key === 'dept'" class="cell cell-dept">
                  <span class="row-text" :title="row.dept">{{ row.dept }}</span>
                </div>
                <div v-else-if="col.key === 'position'" class="cell cell-position">
                  <span class="row-text">{{ row.position }}</span>
                </div>
                <div v-else-if="col.key === 'signDate'" class="cell cell-signDate">
                  <span class="row-mono">{{ row.signDate }}</span>
                </div>
                <div v-else-if="col.key === 'signer'" class="cell cell-signer">
                  <span class="row-text">{{ row.signer }}</span>
                </div>
                <div v-else-if="col.key === 'measure'" class="cell cell-measure">
                  <span class="row-text">{{ row.measure }}</span>
                </div>
                <div v-else-if="col.key === 'summary'" class="cell cell-summary">
                  <span class="row-text" :title="row.summary">{{ row.summary }}</span>
                </div>
                <div v-else-if="col.key === 'extractStatus'" class="cell cell-extractStatus">
                  <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
                    variant="light-outline" class="row-status-tag">
                    <template v-if="statusOf(row).icon" #icon>
                      <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
                    </template>
                    {{ statusOf(row).label }}
                  </t-tag>
                  <span v-else class="row-muted">--</span>
                </div>
                <div v-else-if="col.key === 'tags'" class="cell cell-tags" @click.stop>
                  <t-tooltip v-if="rowTags(row).length" :content="rowTags(row).map((t: any) => t.name).join('、')"
                    placement="top">
                    <div class="row-tag-chips is-clickable" @click="openTagEdit(row)">
                      <t-tag v-if="rowTags(row).length" size="small" variant="light-outline" class="row-tag">
                        {{ rowTags(row)[0].name }}
                      </t-tag>
                      <span v-if="rowTags(row).length > 1" class="row-tag-more">+{{ rowTags(row).length - 1 }}</span>
                    </div>
                  </t-tooltip>
                  <span v-else class="row-tag-chips is-clickable" @click="openTagEdit(row)">
                    <span class="row-tag-add">+ 标签</span>
                  </span>
                </div>
                <div v-else-if="col.key === 'basis'" class="cell cell-basis">
                  <span class="row-text" :title="row.basis">{{ row.basis }}</span>
                </div>
                <div v-else-if="col.key === 'effectiveDate'" class="cell cell-effectiveDate">
                  <span class="row-mono">{{ row.effectiveDate }}</span>
                </div>
                <div v-else-if="col.key === 'remark'" class="cell cell-remark">
                  <span class="row-text" :title="row.remark">{{ row.remark }}</span>
                </div>
                <div v-else-if="col.key === 'fileName'" class="cell cell-fileName">
                  <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
                </div>
              </template>
              <!-- 文件名 -->
              <div v-if="colVisible('fileName')" class="cell cell-fileName">
                <span class="row-text" :title="row.fileName">{{ row.fileName }}</span>
              </div>
            </div>
            <div v-if="!filteredRows.length && !listLoading" class="doc-empty-state">
              <t-empty description="暂无奖惩，请点击右上角「上传奖惩」" />
            </div>
            <div v-if="loadingMore" class="doc-load-more">
              <t-loading size="small" text="加载中..." />
            </div>
          </div>
        </div>
      </div>

      <!-- 底部汇总（选中记录时显示选中奖惩汇总，未选中显示全部；选中时避让底部工具栏） -->
      <div class="doc-summary-bar" :class="{ 'is-batch-visible': selectedRowKeys.length }">
        <span class="doc-summary-count">共 {{ displaySummary.total }} 份</span>
      </div>

      <!-- 底部浮动工具栏（选中行时显示；打印弹窗打开时隐藏，避免浮于弹窗之上） -->
      <transition name="batch-bar-fade">
        <div v-if="selectedRowKeys.length && !printVisible" class="doc-batch-bar-fixed" role="region">
          <div class="batch-bar-inner">
            <div class="batch-bar-left">
              <span class="batch-bar-count">已选 {{ selectedRowKeys.length }} 项</span>
              <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
                清除
              </t-button>
            </div>
            <div class="batch-bar-actions">
              <t-popconfirm theme="warning"
                :content="`确定重新解析并提取「${selectedSingle?.fileName || '该文件'}」吗？将覆盖已有提取结果。`"
                :confirm-btn="{ content: '重新提取', theme: 'warning' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleReExtract">
                <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click.stop>
                  <template #icon><t-icon name="refresh" size="14px" /></template>
                  重新提取
                </t-button>
              </t-popconfirm>
              <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handleBatchEdit">
                <template #icon><t-icon name="edit" size="14px" /></template>
                编辑数据
              </t-button>
              <t-button theme="default" variant="outline" size="small" @click="handleBatchPrint">
                <template #icon><t-icon name="print" size="14px" /></template>
                打印
              </t-button>
              <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handleBatchCatalog">
                <template #icon><t-icon name="file-paste" size="14px" /></template>
                目录
              </t-button>
              <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedRowKeys.length} 个奖惩记录吗？删除后不可恢复。`"
                :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="handleBatchDelete">
                <t-button theme="danger" variant="outline" size="small" @click.stop>
                  <template #icon><t-icon name="delete" size="14px" /></template>
                  删除记录
                </t-button>
              </t-popconfirm>
            </div>
          </div>
        </div>
      </transition>
    </div>

    <!-- 创建知识库向导 -->
    <AwardPunishKbWizard v-model:visible="wizardVisible" @created="onKbCreated" />

    <!-- 删除历史（自动删除的非奖惩记录） -->
    <DeletedKnowledgeDrawer v-model:visible="historyVisible" :kb-id="kbId || ''" module-name="奖惩"
      @changed="loadFiles(true)" @restored="onRestored" />

    <!-- 识别规则（包含判定 + 类型归类） -->
    <RecognitionRulesDrawer v-model:visible="recognitionVisible" :kb-id="kbId || ''" module-name="奖惩"
      @changed="onRecognitionChanged" />

    <!-- 奖惩详情抽屉（竖向区块，可拖宽，上下滚动） -->
    <div v-if="detailVisible" class="doc-drawer-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
      <div class="doc-drawer-resize-line" />
    </div>
    <t-drawer v-model:visible="detailVisible" :header="detailTitle" :size="`${drawerWidth}px`" :footer="false"
      destroy-on-close class="awardPunish-detail-drawer">
      <div class="awardPunish-detail-body">
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

        <!-- 奖惩字段 -->
        <section class="detail-block">
          <div class="detail-block-content">
            <div class="detail-fields">
              <div class="field-group">
                <div class="field-group-title">奖惩信息</div>
                <div class="field-grid">
                  <t-form-item label="文号" label-width="110px">
                    <t-input v-model="editForm.ap_no" placeholder="无编号时自动生成" />
                  </t-form-item>
                  <t-form-item label="标题" label-width="110px">
                    <t-input v-model="editForm.ap_title" placeholder="" />
                  </t-form-item>
                  <t-form-item label="奖惩类型" label-width="110px">
                    <t-select v-model="editForm.ap_type" :options="apTypeOptions" clearable filterable
                      placeholder="选择类型" style="width: 100%" />
                  </t-form-item>
                  <t-form-item label="部门" label-width="110px">
                    <t-input v-model="editForm.dept" placeholder="" />
                  </t-form-item>
                  <t-form-item label="岗位" label-width="110px">
                    <t-input v-model="editForm.position" placeholder="" />
                  </t-form-item>
                  <t-form-item label="签发日期" label-width="110px">
                    <t-date-picker v-model="editForm.sign_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input style="width: 100%" />
                  </t-form-item>
                  <t-form-item label="措施" label-width="110px">
                    <t-select :value="measureArray" multiple :options="measureOptions" filterable clearable
                      placeholder="选择措施" style="width: 100%" @change="(v: string[]) => editForm.measure = (v || []).join(',')" />
                  </t-form-item>
                  <t-form-item label="当事人" label-width="110px">
                    <t-input v-model="editForm.person" placeholder="" />
                  </t-form-item>
                </div>
              </div>

              <div class="field-group">
                <div class="field-group-title">奖惩内容</div>
                <div class="field-grid">
                  <t-form-item label="依据" label-width="110px">
                    <t-input v-model="editForm.basis" placeholder="" />
                  </t-form-item>
                  <t-form-item label="生效日期" label-width="110px">
                    <t-date-picker v-model="editForm.effective_date" value-type="YYYY-MM-DD" format="YYYY-MM-DD" clearable
                      allow-input style="width: 100%" />
                  </t-form-item>
                  <t-form-item label="签发人" label-width="110px">
                    <t-input v-model="editForm.signer" placeholder="" />
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

        <!-- 源文件预览 -->
        <section class="detail-block">
          <div class="detail-block-title">源文件预览</div>
          <div class="detail-block-content">
            <DocumentPreview v-if="currentRow" :knowledge-id="currentRow.knowledgeId"
              :file-type="currentRow?.fileType || ''" :file-name="currentRow?.fileName || ''" :active="true" />
          </div>
        </section>
      </div>
    </t-drawer>

    <!-- 标签编辑（复用原项目组件） -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理抽屉（复用原项目组件） -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 打印预览弹窗（自定义实现，完全可控；弃用 TDesign dialog） -->
    <teleport to="body">
      <div v-if="printVisible" class="awardPunish-print-mask">
        <div class="awardPunish-print-dialog">
          <div class="awardPunish-print-header">
            <span class="awardPunish-print-title">{{ printMode === 'catalog' ? `目录预览（${printCount} 条）` : `打印预览（${printCount} 份）` }}</span>
            <t-button variant="text" size="small" class="awardPunish-print-close" @click="printVisible = false">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="awardPunish-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" :text="printMode === 'catalog' ? '正在生成目录…' : '正在合并奖惩 PDF…'" />
            </div>
          </div>
          <div class="awardPunish-print-footer">
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
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { PDFDocument } from 'pdf-lib'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  uploadKnowledgeFile,
  getKnowledgeDetails,
  delKnowledgeDetails,
  batchDeleteKnowledge,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractAwardPunish,
  listAwardPunishRecords,
  listAwardPunishTypes,
  previewKnowledgeFile,
  reparseKnowledge,
  listDeletedKnowledge,
  restoreDeletedKnowledge,
  purgeDeletedKnowledge,
  getRecognitionConfig,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import AwardPunishKbWizard from './AwardPunishKbWizard.vue'
import DeletedKnowledgeDrawer from './DeletedKnowledgeDrawer.vue'
import RecognitionRulesDrawer from './RecognitionRulesDrawer.vue'

const KB_NAME = '日常事务-奖惩'
const ACCEPT_TYPES = ['pdf', 'jpg', 'jpeg', 'png']
const PAGE_SIZE = 20

// 奖惩类型枚举（编辑/筛选推荐值）
const AWARD_PUNISH_TYPES = ['处罚', '奖励', '通报', '其它奖惩']

// 字段定义（列显隐设置）
interface ColumnDef {
  key: string
  label: string
  default: boolean
  w: string
}
const COLUMN_DEFS: ColumnDef[] = [
  { key: 'apNo', label: '文号', default: true, w: '1.4fr' },
  { key: 'apTitle', label: '标题', default: true, w: '1.8fr' },
  { key: 'apType', label: '奖惩类型', default: true, w: '1fr' },
  { key: 'person', label: '当事人', default: true, w: '1fr' },
  { key: 'dept', label: '部门', default: false, w: '1.3fr' },
  { key: 'position', label: '岗位', default: false, w: '0.7fr' },
  { key: 'signDate', label: '签发日期', default: true, w: '1.1fr' },
  { key: 'signer', label: '签发人', default: true, w: '0.8fr' },
  { key: 'measure', label: '措施', default: true, w: '1.2fr' },
  { key: 'summary', label: '摘要', default: true, w: '2fr' },
  { key: 'extractStatus', label: '状态', default: true, w: '1fr' },
  { key: 'tags', label: '标签', default: true, w: '1.2fr' },
  { key: 'basis', label: '依据', default: false, w: '1.6fr' },
  { key: 'effectiveDate', label: '生效日期', default: false, w: '1.1fr' },
  { key: 'remark', label: '备注', default: false, w: '1.4fr' },
  { key: 'fileName', label: '文件名', default: false, w: '1.4fr' },
]
const COLUMN_STORAGE_KEY = 'weknora-awardPunish-list-columns-v4'

const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const fileInputRef = ref<HTMLInputElement>()

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

interface AwardPunishItem {
  ap_no?: string
  ap_title?: string
  ap_type?: string
  dept?: string
  position?: string
  sign_date?: string
  measure?: string
  basis?: string
  effective_date?: string
  signer?: string
  remark?: string
  person?: string
  reviewed_by?: string
  approved_by?: string
  summary?: string
  page?: number
}

interface AwardPunishRow extends Record<string, any> {
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
  dept?: string
  position?: string
  signDate?: string
  measure?: string
  basis?: string
  effectiveDate?: string
  signer?: string
  remark?: string
  person?: string
  summary?: string
  page?: number
  multiIndex?: string
  tags?: any[]
  description?: string
}

interface StatusInfo {
  label: string
  theme: 'success' | 'warning' | 'danger' | 'primary' | 'default'
  icon?: string
  spin?: boolean
}

// ---- 列表 ----
const items = ref<KnowledgeItem[]>([])
const awardPunishRows = ref<AwardPunishRow[]>([])
// 进行中的文件（解析中/提取中/待提取），在奖惩级列表顶部以状态行展示
const pendingFiles = ref<KnowledgeItem[]>([])
const awardPunishSummary = ref<{ total: number; sumAmount: number; sumTotal: number }>({
  total: 0, sumAmount: 0, sumTotal: 0,
})
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const filterApType = ref('')
const filterFulfillStatus = ref('')
const apTypeOptions = ref<Array<{ value: string; label: string }>>([])
const measureList = ref<{ name: string; type: string }[]>([])

// 措施多选：editForm.measure 以逗号分隔字符串存储，多选组件用数组适配
const measureArray = computed(() =>
  String(editForm.value.measure || '').split(/[,，]/).map(v => v.trim()).filter(Boolean)
)
const measureOptions = computed(() => {
  const t = String(editForm.value.ap_type || '').trim()
  const opts = measureList.value
    .filter(m => !m.type || !t || m.type === t)
    .map(m => ({ label: m.name, value: m.name }))
  const names = new Set(opts.map(o => o.value))
  measureArray.value.forEach(v => { if (!names.has(v)) opts.push({ label: v, value: v }) })
  return opts
})
const loadMeasures = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    const arr = Array.isArray(c?.measures) ? c.measures : []
    measureList.value = arr.filter((m: any) => m?.name).map((m: any) => ({ name: m.name, type: m.type || '' }))
  } catch { /* 措施加载失败不阻塞 */ }
}
const dateRange = ref<Array<string>>([])
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const extractFailed = ref<Set<string>>(new Set())

// 列显隐
const visibleColKeys = ref<string[]>(loadStoredColumns())
const fieldPopupVisible = ref(false)
const columnDefs = COLUMN_DEFS
const visibleColDefs = computed(() => COLUMN_DEFS.filter(c => visibleColKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))

function loadStoredColumns(): string[] {
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) return arr.filter((k: string) => COLUMN_DEFS.some(c => c.key === k))
    }
  } catch { /* ignore */ }
  return COLUMN_DEFS.filter(c => c.default).map(c => c.key)
}
function colVisible(key: string) { return visibleColKeys.value.includes(key) }
function selectAllColumns() { visibleColKeys.value = COLUMN_DEFS.map(c => c.key) }
function resetColumns() { visibleColKeys.value = COLUMN_DEFS.filter(c => c.default).map(c => c.key) }
function persistColumns() {
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
// 字段显隐变化即持久化：t-checkbox-group 的 @change 在部分勾选交互下不触发，
// 用 watch 兜底，确保取消列（如备注）后硬刷新不恢复默认。
watch(visibleColKeys, () => persistColumns(), { deep: true })

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<AwardPunishRow | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<AwardPunishRow | null>(null)
const currentDetail = ref<KnowledgeItem | null>(null)
const editForm = ref<Record<string, any>>({})
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null
let autoSaveDirty = false
let editFormSnapshot = ''
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
const DRAWER_WIDTH_KEY = 'weknora-awardPunish-drawer-width'
const DRAWER_DEFAULT_WIDTH = 700
const DRAWER_MIN_WIDTH = 520
const drawerWidth = ref(DRAWER_DEFAULT_WIDTH)
const drawerResizing = ref(false)
let drawerResizeStartX = 0
let drawerResizeStartWidth = 0

// 打印预览
const printVisible = ref(false)
const historyVisible = ref(false)
const recognitionVisible = ref(false)
const printCount = ref(0)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)
const printMode = ref<'print' | 'catalog'>('print')
const catalogBusy = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null

// ---- KB 检测 ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      loadDrawerWidth()
      await loadTags()
      await loadApTypes()
      await loadMeasures()
      await cleanNonAwardPunishFiles()
      await loadFiles()
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
    loadDrawerWidth()
    await loadTags()
    await loadApTypes()
    await loadMeasures()
    await cleanNonAwardPunishFiles()
    await loadFiles()
    startPolling()
  }
}

// ---- 存量非奖惩清理：扫描知识库中已标记 not_awardPunish 的文件并自动删除 ----
let cleaningNonAwardPunish = false
const cleanNonAwardPunishFiles = async () => {
  if (!kbId.value || cleaningNonAwardPunish) return
  cleaningNonAwardPunish = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const bad = arr.filter((it: any) =>
      it.custom_metadata?.kind === 'not_award_punish' || it.custom_metadata?.extract_status === 'not_award_punish')
    if (bad.length) {
      await batchDeleteKnowledge(kbId.value, bad.map((b: any) => b.id))
      MessagePlugin.warning(`已移除 ${bad.length} 个非奖惩文件（旧数据清理）`)
      loadApTypes()
      loadMeasures()
    }
  } catch { /* 清理失败静默，下轮重试 */ }
  finally { cleaningNonAwardPunish = false }
}

// ---- 奖惩类型下拉：从识别规则配置的分类列表加载（含自定义分类），
// 未配置时兜底用知识库现有奖惩类型 + 枚举 ----
const loadApTypes = async () => {
  if (!kbId.value) return
  try {
    const res: any = await getRecognitionConfig(kbId.value)
    const c = res?.data || res
    const types: string[] = Array.isArray(c?.types) ? c.types.filter(Boolean) : []
    if (types.length) {
      apTypeOptions.value = types.map(t => ({ value: t, label: t }))
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
    const fixed = AWARD_PUNISH_TYPES.filter(t => !seen.has(t)).map(t => ({ value: t, label: t }))
    apTypeOptions.value = [...dynamic, ...fixed]
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

const rowTags = (row: AwardPunishRow) => {
  const arr = row.tags || []
  return Array.isArray(arr) ? arr : []
}

const openTagEdit = (row: AwardPunishRow) => {
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

// 识别规则保存后：刷新奖惩类型选项（含新增分类）+ 重新加载列表
const onRecognitionChanged = () => {
  loadApTypes()
  loadMeasures()
  loadFiles(true)
}

// ---- 列表加载（奖惩级聚合列表，懒加载分页） ----
const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    awardPunishRows.value = []
    items.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) {
    return
  } else {
    loadingMore.value = true
  }
  try {
    const res: any = await listAwardPunishRecords(kbId.value, {
      q: keyword.value || undefined,
      ap_type: filterApType.value || undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    awardPunishSummary.value = { total, sumAmount: 0, sumTotal: 0 }
    const existing = new Set(awardPunishRows.value.map(r => r.rowKey))
    const fresh = arr.map(mapAwardPunishRecord).filter(r => !existing.has(r.rowKey))
    awardPunishRows.value = [...awardPunishRows.value, ...fresh]
    hasMore.value = awardPunishRows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '奖惩列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

// 把后端奖惩级记录（AwardPunishRecord）映射为前端行
const mapAwardPunishRecord = (r: any): AwardPunishRow => ({
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
  dept: r.dept || '',
  position: r.position || '',
  signDate: r.sign_date || '',
  measure: r.measure || '',
  basis: r.basis || '',
  effectiveDate: r.effective_date || '',
  signer: r.signer || '',
  remark: r.remark || '',
  person: r.person || '',
  summary: r.summary || '',
  page: Number(r.page) || 0,
})

const rebuildRows = () => {
  // 奖惩级列表数据已由后端去重/过滤/分页返回，无需前端重建
}

const parseCustomMetadata = (item: KnowledgeItem): AwardPunishRow[] => {
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
    dept: ct.dept || '',
    position: ct.position || '',
    signDate: ct.sign_date || '',
    measure: ct.measure || '',
    basis: ct.basis || '',
    effectiveDate: ct.effective_date || '',
    signer: ct.signer || '',
    remark: ct.remark || '',
    person: ct.person || '',
    summary: ct.summary || '',
    page: Number(ct.page) || 0,
  }))
}

const numOrUndef = (v: any): number | undefined => {
  if (v === null || v === undefined || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

// 过滤/搜索/去重已由后端奖惩级接口完成，前端直接使用返回的分页数据；
// 顶部合并"进行中文件"状态行（解析中/提取中/待提取），提取完成即消失
const pendingRows = computed<AwardPunishRow[]>(() => pendingFiles.value.map((k) => {
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
  } as AwardPunishRow
}))
const filteredRows = computed(() => [...pendingRows.value, ...awardPunishRows.value])

// ---- 状态列（复用原项目"绿色 loading 动态"样式） ----
const extractStatusOf = (row: AwardPunishRow): string => {
  const ps = row.parseStatus
  if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return 'parsing'
  if (ps === 'failed') return 'parse_failed'
  if (ps === 'completed') {
    if (extractInFlight.value.has(row.knowledgeId)) return 'processing'
    if (extractFailed.value.has(row.knowledgeId)) return 'failed'
    const es = row.extractStatus
    if (!es || es === 'pending' || es === 'processing') return 'pending'
    if (es === 'success') return 'success'
    if (es === 'failed') return 'failed'
    if (es === 'not_award_punish') return 'not_award_punish'
    if (es === 'manual') return 'manual'
  }
  return ''
}

const statusOf = (row: AwardPunishRow): StatusInfo => {
  const s = extractStatusOf(row)
  switch (s) {
    case 'parsing': return { label: '解析中', theme: 'primary', icon: 'loading', spin: true }
    case 'parse_failed': return { label: '解析失败', theme: 'danger', icon: 'close-circle' }
    case 'pending': return { label: '待提取', theme: 'primary', icon: 'loading', spin: true }
    case 'processing': return { label: '提取中', theme: 'primary', icon: 'loading', spin: true }
    case 'success': return { label: '提取成功', theme: 'success' }
    case 'failed': return { label: '提取失败', theme: 'danger', icon: 'close-circle' }
    case 'not_award_punish': return { label: '非奖惩', theme: 'default' }
    case 'manual': return { label: '待补录', theme: 'warning', icon: 'edit-1' }
    default: return { label: '待解析', theme: 'default' }
  }
}

const summaryState = computed<StatusInfo | null>(() => {
  const ss = currentDetail.value?.summary_status
  if (!ss) return null
  if (ss === 'pending' || ss === 'processing') return { label: '摘要生成中', theme: 'primary', icon: 'loading', spin: true }
  if (ss === 'completed') return { label: '摘要已生成', theme: 'success' }
  if (ss === 'failed') return { label: '摘要生成失败', theme: 'danger' }
  return null
})

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
const clearSelection = () => { selectedRowKeys.value = [] }
const selectedRows = computed(() => {
  const byKey = new Map(awardPunishRows.value.map(r => [r.rowKey, r]))
  return selectedRowKeys.value.map(k => byKey.get(k)).filter(Boolean) as AwardPunishRow[]
})
const selectedIds = computed(() => Array.from(new Set(selectedRows.value.map(r => r.knowledgeId))))

// 底部汇总：选中记录时显示选中奖惩的汇总，未选中时显示全部
const selectedSummary = computed(() => {
  const rows = selectedRows.value
  if (!rows.length) return null
  return { total: rows.length, sumAmount: 0, sumTotal: 0 }
})
const displaySummary = computed(() => selectedSummary.value || awardPunishSummary.value)

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

// ---- 上传 ----
const triggerUpload = () => { fileInputRef.value?.click() }
const onFileInputChange = (e: Event) => {
  const input = e.target as HTMLInputElement
  if (input.files?.length) handleUploadFiles(Array.from(input.files))
  input.value = ''
}

const handleUploadFiles = async (files: File[]) => {
  const valid = files.filter(f => {
    const ext = (f.name.split('.').pop() || '').toLowerCase()
    return ACCEPT_TYPES.includes(ext)
  })
  const invalidCount = files.length - valid.length
  if (invalidCount) MessagePlugin.warning(`已忽略 ${invalidCount} 个不支持的文件（仅支持 PDF/JPG/PNG）`)
  if (!valid.length) return
  try {
    for (const file of valid) await uploadKnowledgeFile(kbId.value, { file })
    MessagePlugin.success(`已上传 ${valid.length} 个奖惩文件，正在解析...`)
    await loadFiles(true)
    ensurePolling()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '上传失败')
  }
}

// ---- 轮询解析 + 提取 ----
const startPolling = () => ensurePolling()
const ensurePolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(async () => { await pollTick() }, 3000)
}

// 轮询刷新：
// 1) probeExtract 用 knowledge 级列表探测"已解析完成但尚未提取"的新上传文件并触发提取；
// 2) refreshAwardPunishRows 刷新奖惩级聚合列表，把提取完成的奖惩行实时合并进列表。
let refreshBusy = false
const probeExtract = async () => {
  if (!kbId.value || refreshBusy) return
  refreshBusy = true
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const data = res?.data || res?.list || []
    const arr: KnowledgeItem[] = Array.isArray(data) ? data : []
    const needExtract: KnowledgeItem[] = []
    const pend: KnowledgeItem[] = []
    for (const item of arr) {
      const ps = item.parse_status
      if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') {
        pend.push(item)
        continue
      }
      if (ps === 'completed') {
        const es = item.custom_metadata?.extract_status
        if ((!es || es === 'pending' || es === 'processing') &&
            !extractInFlight.value.has(item.id) && !extractFailed.value.has(item.id)) {
          needExtract.push(item)
        }
        if (es && es !== 'pending' && es !== 'processing') extractFailed.value.delete(item.id)
        if (!es || es === 'pending' || es === 'processing') pend.push(item)
      }
    }
    // 已有奖惩行（提取完成）的文件不再显示进行中状态行
    const withRows = new Set(awardPunishRows.value.map(r => r.knowledgeId))
    pendingFiles.value = pend.filter(k => !withRows.has(k.id))
    for (const item of needExtract.slice(0, 5)) {
      extractInFlight.value.add(item.id)
      try {
        const r: any = await extractAwardPunish(kbId.value, item.id)
        // 后端判定非奖惩并已自动删除该文件 → 提示并刷新类型
        if (r?.data?.removed) {
          MessagePlugin.info(`「${item.file_name || item.title}」不是奖惩文件，已移至删除历史，可在删除历史中恢复`)
          loadApTypes()
      loadMeasures()
        }
      } catch {
        extractFailed.value.add(item.id)
      }
    }
    for (const id of Array.from(extractInFlight.value)) {
      const it = arr.find(k => k.id === id)
      if (it?.custom_metadata?.extract_status) extractInFlight.value.delete(id)
    }
  } catch { /* 轮询探测失败静默，下轮重试 */ }
  finally { refreshBusy = false }
}

// 刷新奖惩级列表（合并更新已加载行，提取完成的新数据实时出现）
let awardPunishRefreshBusy = false
const refreshAwardPunishRows = async () => {
  if (!kbId.value || awardPunishRefreshBusy) return
  awardPunishRefreshBusy = true
  try {
    const res: any = await listAwardPunishRecords(kbId.value, {
      q: keyword.value || undefined,
      ap_type: filterApType.value || undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: 1,
      page_size: Math.max(awardPunishRows.value.length, PAGE_SIZE),
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    awardPunishSummary.value = { total, sumAmount: 0, sumTotal: 0 }
    const byKey = new Map(awardPunishRows.value.map(r => [r.rowKey, r]))
    const fresh: AwardPunishRow[] = []
    for (const r of arr) {
      const row = mapAwardPunishRecord(r)
      const old = byKey.get(row.rowKey)
      if (old) {
        // 已存在行：合并最新字段（提取完成后从空变有值）
        const idx = awardPunishRows.value.findIndex(x => x.rowKey === row.rowKey)
        if (idx >= 0) awardPunishRows.value[idx] = { ...old, ...row }
      } else {
        fresh.push(row)
      }
    }
    if (fresh.length) awardPunishRows.value = [...fresh, ...awardPunishRows.value]
  } catch { /* 奖惩列表刷新失败静默 */ }
  finally { awardPunishRefreshBusy = false }
}

const pollTick = async () => {
  if (!kbId.value || document.hidden) return
  await probeExtract()
  await refreshAwardPunishRows()
}
const stopPolling = () => {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- 抽屉宽度拖动（复刻原项目） ----
function drawerMaxWidth() { return Math.min(1600, Math.max(DRAWER_MIN_WIDTH, Math.floor(window.innerWidth * 0.95))) }
function clampDrawerWidth(w: number) { return Math.max(DRAWER_MIN_WIDTH, Math.min(drawerMaxWidth(), w)) }
function loadDrawerWidth() {
  try {
    const raw = localStorage.getItem(DRAWER_WIDTH_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isNaN(parsed)) drawerWidth.value = clampDrawerWidth(parsed)
  } catch { /* ignore */ }
}
function onDrawerResizeStart(e: MouseEvent) {
  drawerResizing.value = true
  drawerResizeStartX = e.clientX
  drawerResizeStartWidth = drawerWidth.value
  document.addEventListener('mousemove', onDrawerResizeMove)
  document.addEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}
function onDrawerResizeMove(e: MouseEvent) {
  const delta = drawerResizeStartX - e.clientX
  drawerWidth.value = clampDrawerWidth(drawerResizeStartWidth + delta)
}
function onDrawerResizeEnd() {
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  drawerResizing.value = false
  try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
}

// ---- 详情抽屉 ----
const openDetail = async (row: AwardPunishRow) => {
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
        // 奖惩一份文件一份：优先按文号匹配详情（奖惩不按页匹配）
        const match = parsed.find(p => p.apNo && p.apNo === row.apNo)
          || parsed.find(p => p.apTitle === row.apTitle) || parsed[0]
        if (match) currentRow.value = { ...row, ...match }
      }
    }
  } catch { /* 详情刷新失败不影响查看 */ }
  fillEditForm()
}

const detailTitle = computed(() =>
  currentRow.value?.apNo ? `奖惩详情 · ${currentRow.value.apNo}` : '奖惩详情'
)

// ---- 重新入库（恢复）后自动打开编辑抽屉 ----
const onRestored = async (knowledgeId: string) => {
  await loadFiles(true)
  const row = awardPunishRows.value.find((r: any) => r.knowledgeId === knowledgeId)
  if (row) {
    openDetail(row)
  } else {
    // 列表可能因分页/懒加载未包含该行，用最小占位行打开编辑抽屉补录字段
    openDetail({
      rowKey: `manual-${knowledgeId}`,
      knowledgeId,
      fileName: '',
      parseStatus: 'completed',
      extractStatus: 'manual',
      kind: 'award_punish',
      tags: [],
      description: '',
      page: 0,
    } as AwardPunishRow)
  }
}

// ---- 字段编辑 + 自动保存 ----
const fillEditForm = () => {
  const r = currentRow.value
  autoSaveDirty = false
  if (!r) return
  editForm.value = {
    ap_no: r.apNo || '',
    ap_title: r.apTitle || '',
    ap_type: r.apType || '',
    dept: r.dept || '',
    position: r.position || '',
    sign_date: r.signDate || '',
      measure: r.measure || '',
    basis: r.basis || '',
    effective_date: r.effectiveDate || '',
    signer: r.signer || '',
    remark: r.remark || '',
    person: r.person || '',
    summary: r.summary || '',
  }
  editFormSnapshot = JSON.stringify(editForm.value)
  autoSaveDirty = true
}

watch(editForm, () => {
  if (!autoSaveDirty || !currentRow.value) return
  // 打开抽屉未做任何编辑（表单值与初始快照一致）时不触发自动保存，
  // 避免"打开即保存空字段"把待补录记录从列表挤掉。
  if (JSON.stringify(editForm.value) === editFormSnapshot) return
  if (autoSaveTimer) clearTimeout(autoSaveTimer)
  autoSaveTimer = setTimeout(() => { saveEditForm() }, 1200)
}, { deep: true })

const saveEditForm = async () => {
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
      dept: editForm.value.dept || '',
      position: editForm.value.position || '',
      sign_date: editForm.value.sign_date || '',
        measure: editForm.value.measure || '',
      basis: editForm.value.basis || '',
      effective_date: editForm.value.effective_date || '',
      signer: editForm.value.signer || '',
      remark: editForm.value.remark || '',
      person: editForm.value.person || '',
      reviewed_by: editForm.value.reviewed_by || '',
      approved_by: editForm.value.approved_by || '',
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
    const hasAwardPunishData = records.some((c: any) =>
      (c.ap_no || '').trim() || (c.ap_title || '').trim() || (c.ap_type || '').trim() ||
      (c.dept || '').trim() || (c.position || '').trim() || (c.sign_date || '').trim())
    const nextStatus = hasAwardPunishData ? 'success' : 'manual'
    const nextError = hasAwardPunishData ? '' : '人工入库待编辑，请补充字段'
    const newMeta = { ...meta, kind: 'award_punish', records, extract_status: nextStatus, extract_error: nextError }
    await updateKnowledgeMetadata(now.knowledgeId, newMeta)
    if (currentRow.value) currentRow.value = { ...currentRow.value, ...updated, extractStatus: nextStatus }
    // 同步列表行，保证编辑后列表立即刷新
    const listIdx = awardPunishRows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) {
      awardPunishRows.value[listIdx] = {
        ...awardPunishRows.value[listIdx],
        extractStatus: nextStatus,
        extractError: nextError,
        apNo: updated.ap_no,
        apTitle: updated.ap_title,
        apType: updated.ap_type,
        dept: updated.dept,
        position: updated.position,
        signDate: updated.sign_date,
        measure: updated.measure,
        basis: updated.basis,
        effectiveDate: updated.effective_date,
        signer: updated.signer,
        remark: updated.remark,
      }
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

// 重新提取：重新解析选中行对应源文件，解析完成后自动触发奖惩字段提取
// （奖惩一个文件一般只有一份奖惩，无需按页提取；打印也整文档预览）。
const handleReExtract = async () => {
  const row = selectedSingle.value
  if (!row) { MessagePlugin.info('重新提取仅支持单选，请选中一份奖惩'); return }
  extractInFlight.value.add(row.knowledgeId)
  extractFailed.value.delete(row.knowledgeId)
  try {
    await reparseKnowledge(row.knowledgeId)
    MessagePlugin.success(`已触发「${row.fileName}」重新解析与提取`)
    setTimeout(() => { loadFiles(true) }, 1500)
  } catch (e: any) {
    extractFailed.value.add(row.knowledgeId)
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(row.knowledgeId)
  }
}

const handleBatchEdit = () => {
  const rows = selectedRows.value
  if (rows.length !== 1) {
    MessagePlugin.info('请选中单行后编辑，或点击列表中的奖惩行进入编辑')
    return
  }
  openDetail(rows[0])
}

// 打印：奖惩一个文件一般只有一份奖惩，打印预览整个文档（含多页），
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
      if (!blob) { notes.push(`${r.apNo || r.fileName}（获取源文件失败）`); continue }
      try {
        const src = await blob.arrayBuffer()
        // 图片格式奖惩：转为 PDF 页合并（支持跨文件/同文件任意组合）
        if (isImageRow(r, blob)) {
          const ext = String(r.fileType || '').toLowerCase()
          const isPng = ext.includes('png') || (blob?.type || '').toLowerCase().includes('png')
          const img = isPng ? await out.embedPng(src) : await out.embedJpg(src)
          const page = out.addPage([img.width, img.height])
          page.drawImage(img, { x: 0, y: 0, width: img.width, height: img.height })
          mergedPages++
          continue
        }
        // 奖惩直接打印整个源文件（一份奖惩可能多页，如盖章扫描件），不做页抽取
        const pdf = await PDFDocument.load(src, { ignoreEncryption: true })
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
        if (pdf.getPageCount() > 1) notes.push(`${r.apNo || r.fileName}（共 ${pdf.getPageCount()} 页）`)
      } catch {
        notes.push(`${r.apNo || r.fileName}（该文件无法合并，可单独打印）`)
      }
    }
    if (mergedPages === 0) {
      MessagePlugin.error('未获取到可打印的奖惩页面')
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
const catalogValueOf = (row: AwardPunishRow, key: string): string => {
  switch (key) {
    case 'apNo': return row.apNo || ''
    case 'apTitle': return row.apTitle || ''
    case 'apType': return row.apType || ''
    case 'position': return row.position || ''
    case 'signDate': return row.signDate || ''
    case 'dept': return row.dept || ''
    case 'measure': return row.measure || ''
    case 'basis': return row.basis || ''
    case 'effectiveDate': return row.effectiveDate || ''
    case 'signer': return row.signer || ''
    case 'remark': return row.remark || ''
    case 'person': return row.person || ''
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
    const bytes = await generateCatalogPdf({ title: '奖惩目录', columns: cols, rows })
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
  a.download = `奖惩目录-${Date.now()}.pdf`
  a.click()
}

// 删除记录：奖惩一份文件一份奖惩，直接删除整份源文件
const handleBatchDelete = async () => {
  const rows = selectedRows.value
  if (!rows.length) return
  try {
    const fileDeleteIds = new Set<string>()
    for (const r of rows) {
      fileDeleteIds.add(r.knowledgeId)
    }
    if (fileDeleteIds.size) {
      await batchDeleteKnowledge(kbId.value, Array.from(fileDeleteIds))
    }
    MessagePlugin.success('删除成功')
    selectedRowKeys.value = []
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 生命周期 ----
onMounted(() => { loadKb() })
onBeforeUnmount(() => {
  stopPolling()
  if (autoSaveTimer) clearTimeout(autoSaveTimer)
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
})
</script>

<style basisd lang="less">
.awardPunish-management-container {
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

.awardPunish-main { display: flex; flex-direction: column; gap: 12px; flex: 1; min-height: 0; }

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
:global(.awardPunish-field-popup) {
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

.cell-apNo, .cell-apTitle, .cell-dept, .cell-basis, .cell-remark, .cell-fileName {
  justify-content: flex-start;
  text-align: left;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-awardPunish-no {
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
.row-muted { color: var(--td-text-color-disabled, #bbb); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.row-status-tag :deep(.t-icon) { margin-right: 2px; }
.icon-spin { animation: doc-list-spin 0.9s linear infinite; }

.row-tag-chips {
  display: inline-flex; align-items: center; gap: 4px; flex-wrap: nowrap; cursor: pointer;
  .row-tag { max-width: 110px; :deep(.t-tag__text) { max-width: 100px; overflow: hidden; text-overflow: ellipsis; display: inline-block; } }
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

.doc-empty-state { padding: 40px 0; display: flex; justify-content: center; }
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
.awardPunish-detail-drawer :deep(.t-drawer__body) {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0 24px 24px;
}

.awardPunish-detail-body {
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
.field-grid { display: grid; grid-template-columns: 1fr 1fr; column-gap: 20px; row-gap: 12px; }
.field-grid--full { grid-template-columns: 1fr; }
.field-grid :deep(.t-form__item) { margin-bottom: 0; }

.detail-save-hint { display: flex; align-items: center; gap: 6px; margin-top: 16px; font-size: 12px; color: var(--td-text-color-placeholder); }

/* 抽屉 resize 手柄（复刻原项目） */
.doc-drawer-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2001;
  cursor: col-resize;
  display: flex;
  align-items: center;
  justify-content: center;
  .doc-drawer-resize-line {
    width: 2px;
    height: 40px;
    border-radius: 1px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity 0.15s ease, height 0.15s ease;
  }
  &:hover .doc-drawer-resize-line { opacity: 1; height: 80px; }
}
</style>

<style lang="less">
/* 打印预览弹窗：自定义实现（弃用 TDesign dialog），Teleport 到 body，全局样式 */
.awardPunish-print-mask {
  position: fixed;
  top: 0; left: 0; right: 0; bottom: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}
.awardPunish-print-dialog {
  width: min(900px, calc(100vw - 32px));
  max-width: calc(100vw - 32px);
  height: 82vh;
  max-height: 92vh;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 8px 40px rgba(0, 0, 0, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.awardPunish-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 12px 24px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex-shrink: 0;
  background: #fff;
}
.awardPunish-print-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.awardPunish-print-close {
  flex-shrink: 0;
}
.awardPunish-print-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: #fff;
}
.awardPunish-print-body .print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
.awardPunish-print-body .print-preview-loading {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--td-text-color-placeholder);
  font-size: 14px;
}
.awardPunish-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 24px;
  flex-shrink: 0;
  background: #fff;
  border-top: 1px solid var(--td-component-stroke);
}
</style>
