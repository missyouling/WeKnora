<template>
  <div class="utility-meter-tab">
    <!-- 解析状态提示（证照型）：优先展示上传任务的实时进度（多文件轮播），无活动任务时回退列表轮询兜底 -->
    <div v-if="uploadProgress.length || pendingFiles.length" class="archive-pending-bar">
      <t-loading size="small" />
      <template v-if="uploadProgress.length">
        <span>{{ stageLabel(uploadProgress[progressIndex].stage) }}「{{ uploadProgress[progressIndex].name }}」进度 {{ uploadProgress[progressIndex].percent }}%</span>
        <span v-if="uploadProgress.length > 1" class="archive-pending-count">{{ progressIndex + 1 }}/{{ uploadProgress.length }}</span>
      </template>
      <span v-else>正在解析 {{ pendingFiles.length }} 个文件，解析完成后自动提取字段...</span>
    </div>

    <!-- 筛选工具栏（与电费核算内容页一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-input v-model="searchText" :placeholder="isArchive ? '搜索车牌号 / 源文件 / 字段值' : '搜索车牌号 / 单号 / 字段值'"
            clearable class="doc-filter-field__control" style="width: 200px">
            <template #prefix-icon><t-icon name="search" size="14px" /></template>
          </t-input>
        </div>
        <div class="doc-filter-field">
          <t-date-picker v-if="!isArchive" v-model="filters.month" mode="month" placeholder="月份" format="YYYY-MM"
            value-type="YYYY-MM" clearable class="doc-date-picker doc-filter-field__control" @change="loadRecords" />
          <t-select v-else v-model="filters.docType" :options="docTypeOptions" clearable placeholder="全部证照类型"
            class="doc-filter-select doc-filter-field__control" style="width: 180px" @change="onDocTypeChange" />
        </div>
        <t-button variant="outline" size="small" @click="refreshAll">
          <template #icon><t-icon name="refresh" size="14px" /></template>
        </t-button>
      </div>
      <div class="doc-filter-bar__trailing">
        <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
          overlay-inner-class="meter-field-popup">
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
              <t-checkbox-group v-model="visibleKeys" class="field-popup-list" @change="persistColumns">
                <t-checkbox v-for="col in columnDefs" :key="col.key" :value="col.key" class="field-popup-item">
                  {{ col.label }}
                </t-checkbox>
              </t-checkbox-group>
            </div>
          </template>
        </t-popup>
        <t-button variant="outline" size="small" @click="emit('openSettings')">
          <template #icon><t-icon name="setting" size="14px" /></template>
          设置
        </t-button>
        <t-button v-if="isArchive" theme="primary" size="small" :disabled="!kbId" @click="uploadVisible = true">
          <template #icon><t-icon name="upload" size="14px" /></template>
          上传{{ group.fileLabel }}
        </t-button>
        <t-button v-else-if="isBilling" theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
          <template #icon><t-icon name="print" size="14px" /></template>
          打印
        </t-button>
        <t-button v-else theme="primary" size="small" @click="openDrawer(null)">
          <template #icon><t-icon name="add" size="14px" /></template>
          新增记录
        </t-button>
      </div>
    </div>

    <!-- 上传弹窗（证照型） -->
    <FleetUploadDialog v-if="isArchive" v-model:visible="uploadVisible" :kb-id="kbId" :scope="group.scope"
      :type-options="uploadTypeOptions" :default-type="isArchive && filters.docType ? filters.docType : ''"
      @progress="onUploadProgress" @done="onUploadDone" />

    <!-- 上传历史抽屉（证照型） -->
    <FleetUploadHistoryDrawer v-if="isArchive" v-model:visible="historyVisible" :kb-id="kbId" :scope="group.scope" :filter="historyFilter"
      @reload="loadRecords" />

    <!-- 无筛选时：统计概览卡片（文件级生命周期），点击打开历史记录 -->
    <div v-if="isArchive && !filters.docType" class="archive-overview">
      <div class="overview-group">
        <div class="overview-group__title">文件状态</div>
        <div class="overview-group__cards overview-group__cards--status">
          <div v-for="card in overviewStatusCards" :key="card.key" class="stat-card" :class="card.cls"
            @click="onOverviewCardClick(card)">
            <div class="stat-card__icon"><t-icon :name="card.icon" size="20px" /></div>
            <div class="stat-card__num">{{ card.num }}</div>
            <div class="stat-card__label">{{ card.label }}</div>
          </div>
        </div>
      </div>
      <div v-if="overviewVehicleTypeCards.length || overviewDriverTypeCards.length" class="overview-group overview-group--panel">
        <div v-if="overviewVehicleTypeCards.length" class="overview-subgroup">
          <div class="overview-group__title">公司证照</div>
          <div class="overview-group__cards overview-group__cards--type">
            <div v-for="card in overviewVehicleTypeCards" :key="card.key" class="type-card"
              @click="onOverviewCardClick(card)">
              <div class="type-card__icon"><t-icon :name="card.icon" size="22px" /></div>
              <div class="type-card__body">
                <div class="type-card__label">{{ card.label }}</div>
                <div class="type-card__num">{{ card.num }} <span>份</span></div>
              </div>
            </div>
          </div>
        </div>
        <div v-if="overviewDriverTypeCards.length" class="overview-subgroup">
          <div class="overview-group__title">司机证照</div>
          <div class="overview-group__cards overview-group__cards--type">
            <div v-for="card in overviewDriverTypeCards" :key="card.key" class="type-card"
              @click="onOverviewCardClick(card)">
              <div class="type-card__icon"><t-icon :name="card.icon" size="22px" /></div>
              <div class="type-card__body">
                <div class="type-card__label">{{ card.label }}</div>
                <div class="type-card__num">{{ card.num }} <span>份</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 列表 -->

    <!-- 列表 -->
    <div v-else class="doc-list-scroll meter-list-scroll">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <div class="cell cell-check" role="columnheader" @click.stop>
            <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
              @change="toggleAll" />
          </div>
          <template v-for="col in visibleColDefs" :key="col.key">
            <div class="cell cell-body" role="columnheader">
              <span class="col-tip">{{ col.label }}</span>
            </div>
          </template>
        </div>
        <div v-if="!loading && !displayRows.length" class="meter-empty">
          <t-icon :name="isArchive ? 'file-copy' : 'search-error'" size="40px" class="meter-empty-icon" />
          <span class="meter-empty-text">{{ emptyText }}</span>
          <t-button v-if="isArchive" theme="primary" variant="outline" size="small" class="meter-empty-action"
            :disabled="!kbId" @click="uploadVisible = true">
            <template #icon><t-icon name="upload" size="14px" /></template>
            上传{{ group.fileLabel }}
          </t-button>
        </div>
        <template v-for="row in displayRows" :key="rowKey(row)">
          <div class="doc-list-row" :style="gridStyle" role="row" :class="{ 'row-selected': selectedKeys.has(rowKey(row)) }"
            @click="openDrawer(row)">
            <div class="cell cell-check" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(rowKey(row))" @change="toggleSelect(row)" />
            </div>
            <template v-for="col in visibleColDefs" :key="col.key">
              <div class="cell cell-body" :title="cellTitle(col, row)">
                <span v-if="isFileRow(row) && col.key === 'upload_status'" class="cell-status">
                  <t-tag size="small" :theme="uploadTagTheme(row.upload_status)" variant="light-outline">{{ col.value(row) }}</t-tag>
                </span>
                <span v-else-if="isFileRow(row) && col.key === 'file_type'" class="cell-status">
                  <t-tag v-if="row.file_type" size="small" theme="default" variant="light-outline">{{ row.file_type }}</t-tag>
                  <span v-else>—</span>
                </span>
                <span v-else-if="isFileRow(row) && col.key === 'parse_status'" class="cell-status">
                  <t-tag size="small" :theme="parseTagTheme(row.parse_status)" variant="light-outline">{{ col.value(row) }}</t-tag>
                  <t-button v-if="row.parse_status === 'failed'" variant="text" size="small" class="cell-retry"
                    @click.stop="retryFile(row, 'parse')">
                    <template #icon><t-icon name="refresh" size="13px" /></template>重试
                  </t-button>
                </span>
                <span v-else-if="isFileRow(row) && col.key === 'extract_status'" class="cell-status">
                  <t-tag size="small" :theme="extractTagTheme(row)" variant="light-outline">{{ col.value(row) }}</t-tag>
                </span>
                <span v-else-if="isFileRow(row) && col.key === 'tags'" class="cell-status cell-tags">
                  <template v-if="(row.tags || []).length">
                    <div class="row-tag-chips is-clickable" @click.stop="openTagEdit(row)">
                      <t-tag v-for="t in (row.tags || []).slice(0, 3)" :key="t.id" size="small" variant="light-outline"
                        class="row-tag">{{ t.name }}</t-tag>
                      <span v-if="(row.tags || []).length > 3" class="row-tag-overflow"
                        :title="(row.tags || []).map((t: any) => t.name).join('、')">+{{ (row.tags || []).length - 3 }}</span>
                    </div>
                  </template>
                  <span v-else class="row-tag-add" @click.stop="openTagEdit(row)">+ 标签</span>
                </span>
                <span v-else-if="!isFileRow(row) && isStatusField((col.key || '').replace('data.',''))" class="cell-status">
                  <t-tag size="small" :theme="certStatusTheme(calcCertStatus(row.data))" variant="light-outline">{{ calcCertStatus(row.data) }}</t-tag>
                </span>
                <span v-else>{{ col.value(row) }}</span>
              </div>
            </template>
          </div>
        </template>
      </div>
    </div>

    <!-- 底部汇总 -->
    <!-- 底部汇总 -->
    <div v-if="displayRows.length && !(isArchive && !filters.docType)" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <span v-if="typeMeta.hasAmount && !isArchive" class="doc-summary-item">总金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
      <span v-if="isBilling" class="doc-summary-item">总费用 <span class="doc-summary-val">{{ fmtMoney(totalAll) }}</span> 元</span>
    </div>

    <!-- 底部浮动工具栏 -->
    <transition name="batch-bar-fade">
      <div v-if="selectedKeys.size && !printVisible" class="doc-batch-bar-fixed" role="region">
        <div class="batch-bar-inner">
          <div class="batch-bar-left">
            <span class="batch-bar-count">已选 {{ selectedKeys.size }} 项</span>
            <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">清除</t-button>
          </div>
          <div class="batch-bar-actions">
            <template v-if="isArchive && selectedRows.some((r: any) => r.__file)">
              <t-button theme="default" variant="outline" size="small" :loading="batchBusy" @click="batchRetry('parse')">
                <template #icon><t-icon name="refresh" size="14px" /></template>重新解析
              </t-button>
              <t-button theme="default" variant="outline" size="small" :loading="batchBusy" @click="batchRetry('extract')">
                <template #icon><t-icon name="file-search" size="14px" /></template>重新提取
              </t-button>
            </template>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>{{ isArchive ? '目录打印' : '打印' }}
            </t-button>
            <t-popconfirm v-if="!isBilling" theme="warning"
              :content="archiveDeleteHint"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>删除
              </t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </transition>

    <!-- 新增/编辑抽屉 -->
    <teleport to="body">
      <div v-if="drawerVisible" class="doc-drawer-resize-handle" :style="{ right: drawerWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="drawerVisible" :visible="true" :header="drawerTitle" :size="drawerWidth" :footer="false"
        :close-btn="true" :close-on-overlay-click="true" :esc-close="true" destroy-on-close class="meter-record-drawer"
        @close="drawerVisible = false" @update:visible="(v: boolean) => (v || (drawerVisible = false))">
        <div class="meter-drawer-body">
          <div class="rec-grid">
            <!-- 证照型：文件编辑（全部证照类型列表） -->
            <template v-if="isArchive && editMode === 'file'">
              <div class="rec-field">
                <label>文件别名</label>
                <t-input v-model="form.file_name" placeholder="别名可任意修改" />
              </div>
              <div class="rec-field">
                <label>证照类型</label>
                <t-select v-model="form.doc_type" :options="uploadTypeOptions" filterable clearable
                  placeholder="选择证照类型（保存后按所选类型重新提取）" />
              </div>
              <div class="rec-field">
                <label>源文件</label>
                <div class="readonly-val">{{ form.source_name || '—' }}</div>
              </div>
              <div class="rec-field">
                <label>文件类型</label>
                <div class="readonly-val">{{ form.file_type || '—' }}</div>
              </div>
              <div v-if="form.fail_reason" class="rec-field rec-field--wide">
                <label>失败原因</label>
                <div class="readonly-val read-only-fail">{{ form.fail_reason }}</div>
              </div>
              <div class="rec-field rec-field--wide">
                <label>源文件预览</label>
                <DocumentPreview v-if="form.doc_knowledge_id" :knowledge-id="form.doc_knowledge_id"
                  :file-type="form.file_type || ''" :file-name="form.file_name || ''" :active="true" />
                <div v-else class="readonly-val">无关联源文件</div>
              </div>
            </template>
            <!-- 证照型：已提取记录（字段编辑 + 底部源文件预览） -->
            <template v-else-if="isArchive">
              <template v-for="key in archiveEditKeys" :key="key">
                <div class="rec-field" :class="{ 'rec-field--wide': key === '备注' }">
                  <label>{{ String(key) }}</label>
                  <t-select v-if="isStatusField(key)" v-model="form.data[key]"
                    :options="CERT_STATUS_OPTIONS.map((v) => ({ label: v, value: v }))" clearable placeholder="选择状态"
                    :popup-props="{ overlayClassName: 'cert-status-pop' }" />
                  <t-textarea v-else-if="key === '备注'" v-model="form.data[key]" :autosize="{ minRows: 3, maxRows: 6 }"
                    :maxlength="500" placeholder="可修改" />
                  <t-input v-else v-model="form.data[key]" placeholder="可修改" />
                </div>
              </template>
              <div class="rec-field rec-field--wide">
                <label>源文件预览</label>
                <DocumentPreview v-if="form.doc_knowledge_id" :knowledge-id="form.doc_knowledge_id"
                  :file-type="form.file_type || ''" :file-name="form.file_name || ''" :active="true" />
                <div v-else class="readonly-val">无关联源文件</div>
              </div>
            </template>
            <!-- 记录型：车辆/日期/月份 + 类型字段 -->
            <template v-else>
              <div class="rec-field">
                <label>车辆 <span class="required">*</span></label>
                <t-select v-model="form.vehicle_id" :options="vehicleOptions" filterable :placeholder="'选择车辆'" />
              </div>
              <div class="rec-field">
                <label>记录日期</label>
                <t-date-picker v-model="form.record_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable placeholder="选择日期" @change="onDateChange" />
              </div>
              <div class="rec-field">
                <label>月份</label>
                <t-date-picker v-model="form.record_month" mode="month" format="YYYY-MM" value-type="YYYY-MM" clearable placeholder="自动按日期取月" />
              </div>
              <template v-for="f in formFields" :key="f.key">
                <div class="rec-field" :class="{ 'rec-field--wide': f.wide }">
                  <label>{{ f.label }}{{ f.required ? ' *' : '' }}</label>
                  <t-select v-if="f.type === 'select'" v-model="form[f.key]" :options="f.options || []" filterable clearable :placeholder="f.placeholder || '选填'" />
                  <t-input v-else-if="f.type === 'textarea'" v-model="form[f.key]" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" :placeholder="f.placeholder || '选填'" />
                  <t-date-picker v-else-if="f.type === 'date'" v-model="form[f.key]" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable :placeholder="f.placeholder || '选填'" />
                  <t-input v-else v-model.number="form[f.key]" :type="f.type === 'number' ? 'number' : 'text'" :placeholder="f.placeholder || '选填'" />
                </div>
              </template>
            </template>
            <div v-if="!isArchive" class="rec-field rec-field--wide">
              <label>备注</label>
              <t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" />
            </div>
          </div>
        </div>
        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="drawerVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="saving" @click="saveRecord">保存</t-button>
        </div>
      </t-drawer>
    </teleport>

    <!-- 打印预览弹窗 -->
    <div v-if="printVisible" class="meter-print-mask">
      <div class="meter-print-dialog">
        <div class="meter-print-header">
          <span class="meter-print-title">{{ printTitle }}预览（{{ printCount }} {{ isBilling ? '辆车' : isArchive ? '份' : '条' }}）</span>
          <t-button variant="text" size="small" class="meter-print-close" @click="closePrint">
            <template #icon><t-icon name="close" size="16px" /></template>
          </t-button>
        </div>
        <div class="meter-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true" />
          <div v-else class="print-preview-loading"><t-loading size="small" text="正在生成目录..." /></div>
        </div>
        <div class="meter-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>

    <!-- 标签编辑（复用知识库文档标签功能，双向同步） -->
    <TagEditDialog v-model:visible="tagEditVisible" :knowledge-name="tagEditTarget?.alias || ''" :kb-id="kbId || ''"
      :tag-list="tagList" :selected-tags="tagEditTarget?.tags || []" :can-manage="true"
      @confirm="onTagConfirm" @open-manage="tagManageVisible = true" @tag-created="loadTagList" />
    <KbTagManageDrawer v-if="tagManageVisible" :visible="tagManageVisible" :kb-id="kbId || ''"
      @update:visible="tagManageVisible = $event" @changed="loadTagList" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listKnowledgeBases, createKnowledgeBase, listKnowledgeFiles,
  updateKnowledgeMetadata, updateKnowledgeInfo, reparseKnowledge, delKnowledgeDetails,
  listKnowledgeTags, updateKnowledgeTagBatch,
} from '@/api/knowledge-base'
import { listFleetVehicles, listFleetDrivers, listFleetFuelCards, listFleetRecords, createFleetRecord, updateFleetRecord, deleteFleetRecord, listFleetCategories, getFleetSummary } from '@/api/fleet'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '../knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '../knowledge/components/KbTagManageDrawer.vue'
import { useChatResourcesStore } from '@/stores/chatResources'
import { selectInitialModelId } from '@/utils/modelDefaults'
import FleetUploadDialog from './FleetUploadDialog.vue'
import FleetUploadHistoryDrawer from './FleetUploadHistoryDrawer.vue'

const props = defineProps<{ recordType: string }>()
const emit = defineEmits<{ (e: 'openSettings'): void }>()

const KB_NAME = '车队管理'
const chatResources = useChatResourcesStore()

// ---------------------------------------------------------------------------
// 类型判定
// ---------------------------------------------------------------------------
const ARCHIVE_TYPES = ['vehicle-archive', 'driver-archive', 'maintain-archive']
const isArchive = computed(() => ARCHIVE_TYPES.includes(props.recordType))
const isBilling = computed(() => props.recordType === 'billing')

const typeMeta = computed(() => TYPES[props.recordType] || TYPES.tire)
const groupLabel = computed(() => (isArchive.value ? group.value.label : typeMeta.value.label))

// 证照分组：内容页不分 t-tab，每个档案页单一分组（公司证照/司机证照/维保文件）
const ARCHIVE_GROUPS: Record<string, { key: string; label: string; fileLabel: string; scope: string; recordType: string }[]> = {
  'vehicle-archive': [
    { key: 'vehicle', label: '公司证照', fileLabel: '证照', scope: 'vehicle', recordType: 'vehicle-archive' },
  ],
  'driver-archive': [
    { key: 'driver', label: '司机证照', fileLabel: '司机证照', scope: 'driver', recordType: 'driver-archive' },
  ],
  'maintain-archive': [
    { key: 'maintain', label: '维保文件', fileLabel: '维保文件', scope: 'maintain', recordType: 'maintain-archive' },
  ],
}
const archiveGroups = computed(() => ARCHIVE_GROUPS[props.recordType] || [])
const activeGroup = ref('')
const group = computed(() => archiveGroups.value.find((g) => g.key === activeGroup.value) || archiveGroups.value[0])

// ---------------------------------------------------------------------------
// 字段定义（记录型）
// ---------------------------------------------------------------------------
interface FieldDef {
  key: string
  label: string
  type?: 'text' | 'number' | 'select' | 'date' | 'textarea'
  required?: boolean
  wide?: boolean
  options?: { label: string; value: string }[]
  placeholder?: string
}
interface ColDef { key: string; label: string; def: boolean; tip?: string | ((r: any) => string); value: (r: any) => any; fixedWidth?: number }

const TYPES: Record<string, { label: string; hasAmount: boolean; fields: FieldDef[] }> = {
  tire: {
    label: '轮胎管理', hasAmount: true,
    fields: [
      { key: 'tire_no', label: '轮胎编号', placeholder: '如：LT-001' },
      { key: 'brand', label: '品牌', placeholder: '选填' },
      { key: 'spec', label: '规格型号', placeholder: '如：12R22.5' },
      { key: 'tire_type', label: '轮胎类型', type: 'select', options: [['全钢', '全钢'], ['半钢', '半钢'], ['斜交', '斜交'], ['子午线', '子午线']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'apply_pos', label: '适用轮位', type: 'select', options: [['导向轮', '导向轮'], ['驱动轮', '驱动轮'], ['挂车轮', '挂车轮'], ['备胎', '备胎']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'install_pos', label: '安装轮位', type: 'select', options: [['左前', '左前'], ['右前', '右前'], ['左后内', '左后内'], ['左后外', '左后外'], ['右后内', '右后内'], ['右后外', '右后外']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'install_date', label: '安装日期', type: 'date' },
      { key: 'install_mileage', label: '安装里程', type: 'number' },
      { key: 'status', label: '当前状态', type: 'select', options: [['在库', '在库'], ['在用', '在用'], ['待修', '待修'], ['翻新中', '翻新中'], ['已报废', '已报废'], ['已调拨', '已调拨']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'purchase_date', label: '购置日期', type: 'date' },
      { key: 'purchase_price', label: '购置单价', type: 'number' },
      { key: 'current_mileage', label: '当前里程', type: 'number' },
      { key: 'supplier', label: '供应商', placeholder: '选填' },
      { key: 'pay_status', label: '付款状态', type: 'select', options: [['待支付', '待支付'], ['已请款', '已请款'], ['已完成', '已完成']].map(([v, l]) => ({ value: v, label: l })) },
    ],
  },
  inspection: {
    label: '年检管理', hasAmount: true,
    fields: [
      { key: 'inspection_type', label: '年检类型', type: 'select', required: true, options: [['上线检测', '上线检测'], ['免检申领', '免检申领'], ['营运证年审', '营运证年审'], ['综合性能检测', '综合性能检测']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'cycle', label: '检验周期', type: 'select', options: [['1年1检', '1年1检'], ['2年1检', '2年1检'], ['半年1检', '半年1检']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'last_date', label: '上次年检日期', type: 'date' },
      { key: 'next_date', label: '下次年检日期', type: 'date' },
      { key: 'remind_days', label: '到期提醒天数', type: 'number' },
      { key: 'remind_status', label: '提醒状态', type: 'select', options: [['未提醒', '未提醒'], ['已提醒', '已提醒'], ['已办理', '已办理'], ['已逾期', '已逾期']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'overdue_days', label: '逾期天数', type: 'number' },
      { key: 'process_status', label: '办理状态', type: 'select', options: [['待办理', '待办理'], ['办理中', '办理中'], ['已办理', '已办理'], ['已取消', '已取消']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'send_date', label: '送检日期', type: 'date' },
      { key: 'sender', label: '送检人', placeholder: '选填' },
      { key: 'station', label: '检测站', placeholder: '选填' },
      { key: 'result', label: '检测结果', type: 'select', options: [['合格', '合格'], ['不合格', '不合格'], ['复检', '复检']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'valid_until', label: '检验有效期', type: 'date' },
      { key: 'total_fee', label: '总费用', type: 'number' },
      { key: 'invoice_no', label: '发票号', placeholder: '选填' },
    ],
  },
  'fuel-charge': {
    label: '加油充电', hasAmount: true,
    fields: [
      { key: 'bill_no', label: '记录单号', placeholder: '自动生成' },
      { key: 'trade_type', label: '交易类型', type: 'select', options: [['加油', '加油'], ['充电', '充电'], ['加气', '加气']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'consume_type', label: '消费类型', type: 'select', options: [['加油', '加油'], ['充电', '充电'], ['加气', '加气'], ['便利店', '便利店']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'card_id', label: '卡号', type: 'select', options: [] },
      { key: 'oil_type', label: '油品/电价类型', type: 'select', options: [['92#', '92#'], ['95#', '95#'], ['0#柴油', '0#柴油'], ['LNG', 'LNG'], ['充电', '充电']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'qty', label: '数量', type: 'number' },
      { key: 'unit_price', label: '单价', type: 'number' },
      { key: 'consumption', label: '百公里油耗/电耗', type: 'number' },
      { key: 'pay_method', label: '支付方式', type: 'select', options: [['油卡', '油卡'], ['现金', '现金'], ['扫码', '扫码'], ['月结', '月结']].map(([v, l]) => ({ value: v, label: l })) },
    ],
  },
  'road-toll': {
    label: '路桥费', hasAmount: true,
    fields: [
      { key: 'bill_no', label: '记录单号', placeholder: '自动生成' },
      { key: 'fee_type', label: '费用类型', type: 'select', options: [['ETC通行费', 'ETC通行费'], ['人工过路费', '人工过路费'], ['停车费', '停车费'], ['过桥费', '过桥费'], ['渡口费', '渡口费']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'start_date', label: '通行起始日期', type: 'date' },
      { key: 'end_date', label: '通行截止日期', type: 'date' },
      { key: 'entry', label: '入口站/起点', placeholder: '选填' },
      { key: 'exit', label: '出口站/终点', placeholder: '选填' },
      { key: 'park_name', label: '停车场名称', placeholder: '选填' },
      { key: 'tax_rate', label: '税率', type: 'number' },
      { key: 'tax_amount', label: '税额', type: 'number' },
      { key: 'tax_total', label: '含税金额', type: 'number' },
      { key: 'park_fee', label: '停车费', type: 'number' },
      { key: 'pay_method', label: '支付方式', type: 'select', options: [['ETC', 'ETC'], ['现金', '现金'], ['扫码', '扫码'], ['月结', '月结'], ['报销', '报销']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'ticket_no', label: '票据号码', placeholder: '选填' },
    ],
  },
  'repair-cost': {
    label: '维修保养费', hasAmount: true,
    fields: [
      { key: 'project', label: '维修项目', required: true, placeholder: '如：更换机油' },
      { key: 'repair_type', label: '维修类别', type: 'select', options: [['小修', '小修'], ['大修', '大修'], ['保养', '保养'], ['二级维护', '二级维护']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'supplier', label: '维修厂/供应商', placeholder: '选填' },
      { key: 'work_no', label: '工单号', placeholder: '选填' },
      { key: 'labor_fee', label: '工时费', type: 'number' },
      { key: 'material_fee', label: '材料费', type: 'number' },
      { key: 'invoice_no', label: '发票号', placeholder: '选填' },
    ],
  },
  'insurance-claim': {
    label: '保险理赔', hasAmount: true,
    fields: [
      { key: 'claim_no', label: '理赔单号', placeholder: '自动生成' },
      { key: 'policy_no', label: '关联保单号', placeholder: '选填' },
      { key: 'company', label: '保险公司', placeholder: '选填' },
      { key: 'insurance_type', label: '险种', placeholder: '如：车损险' },
      { key: 'accident_date', label: '出险日期', type: 'date' },
      { key: 'report_date', label: '报案日期', type: 'date' },
      { key: 'driver_name', label: '司机姓名', placeholder: '选填' },
      { key: 'accident_type', label: '事故类型', placeholder: '如：追尾' },
      { key: 'location', label: '事故地点', placeholder: '选填' },
      { key: 'liability', label: '事故责任', placeholder: '如：全责' },
      { key: 'assess_amount', label: '定损金额', type: 'number' },
      { key: 'pay_date', label: '赔付日期', type: 'date' },
      { key: 'pay_method', label: '赔付方式', placeholder: '选填' },
      { key: 'status', label: '理赔状态', type: 'select', options: [['待处理', '待处理'], ['处理中', '处理中'], ['已赔付', '已赔付'], ['已结案', '已结案'], ['已拒赔', '已拒赔']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'deductible', label: '免赔金额', type: 'number' },
      { key: 'self_pay', label: '自付金额', type: 'number' },
      { key: 'close_date', label: '结案日期', type: 'date' },
    ],
  },
  violation: {
    label: '违章处理', hasAmount: true,
    fields: [
      { key: 'violation_no', label: '违章单号', placeholder: '自动生成' },
      { key: 'driver_name', label: '司机姓名', placeholder: '选填' },
      { key: 'violation_type', label: '违章类型', type: 'select', options: [['超速', '超速'], ['闯红灯', '闯红灯'], ['违停', '违停'], ['超载', '超载'], ['疲劳驾驶', '疲劳驾驶'], ['不系安全带', '不系安全带'], ['遮挡号牌', '遮挡号牌']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'behavior', label: '违章行为', placeholder: '选填' },
      { key: 'location', label: '违章地点', placeholder: '选填' },
      { key: 'code', label: '违章代码', placeholder: '如：1039' },
      { key: 'points', label: '扣分', type: 'number' },
      { key: 'decision_no', label: '处罚决定书号', placeholder: '选填' },
      { key: 'status', label: '处理状态', type: 'select', options: [['待处理', '待处理'], ['处理中', '处理中'], ['已处理', '已处理'], ['已缴款', '已缴款'], ['已申诉', '已申诉'], ['已撤销', '已撤销']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'handler', label: '处理人', placeholder: '选填' },
      { key: 'handle_date', label: '处理日期', type: 'date' },
      { key: 'pay_date', label: '缴款日期', type: 'date' },
      { key: 'pay_amount', label: '缴款金额', type: 'number' },
      { key: 'pay_method', label: '缴款方式', placeholder: '选填' },
      { key: 'paid', label: '是否已缴款', type: 'select', options: [['是', '是'], ['否', '否']].map(([v, l]) => ({ value: v, label: l })) },
      { key: 'responsible', label: '责任人', placeholder: '选填' },
      { key: 'bearer', label: '承担方', placeholder: '选填' },
    ],
  },
  material: {
    label: '辅材记录', hasAmount: true,
    fields: [
      { key: 'material_type', label: '类型', type: 'select', required: true,
        options: [{ label: '尿素', value: '尿素' }, { label: '篷布', value: '篷布' }, { label: '其它', value: '其它' }] },
      { key: 'quantity', label: '数量', type: 'number' },
      { key: 'unit_price', label: '单价（元）', type: 'number' },
    ],
  },
  'car-request': {
    label: '请车记录', hasAmount: false,
    fields: [
      { key: 'applicant', label: '申请人', placeholder: '选填' },
      { key: 'reason', label: '事由', placeholder: '选填', wide: true },
      { key: 'start_time', label: '开始时间', type: 'date' },
      { key: 'end_time', label: '结束时间', type: 'date' },
      { key: 'approver', label: '审批人', placeholder: '选填' },
    ],
  },
}

// ---------------------------------------------------------------------------
// 列定义
// ---------------------------------------------------------------------------
const fmtMoney = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
const fmtNum = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
const fmtVal = (v: any) => (v === null || v === undefined || v === '') ? '—' : String(v)

function vehiclePlate(id: string) { return vehicles.value.find((v: any) => v.id === id)?.plate_no || '—' }

// 证照内置类型字段（base=默认显示，detail=详细字段）
const BUILTIN_CERTS: Record<string, { name: string; scope: string; base: string[]; detail: string[] }> = {
  '车辆登记证书': {
    name: '车辆登记证书', scope: 'vehicle',
    base: ['证书编号', '车牌号', 'VIN码', '发证机关', '发证日期', '证书状态', '存放位置', '是否随车'],
    detail: ['证书编号', '车牌号', '发证机关', '发证日期', '证书状态', '存放位置', 'VIN', '发动机号', '品牌型号', '使用性质', '注册日期', '报废日期', '车主信息', '产权归属', '过户记录', '备注'],
  },
  '行驶证': {
    name: '行驶证', scope: 'vehicle',
    base: ['编号', '车牌号', 'VIN码/车架号', '发动机号', '品牌型号', '车辆类型', '使用性质', '注册日期', '发证日期', '发证机关', '有效期', '状态'],
    detail: [],
  },
  '道路运输经营许可证': {
    name: '道路运输经营许可证', scope: 'vehicle',
    base: ['许可证号', '业户名称', '经营地址', '经营范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态'],
    detail: [],
  },
  '道路运输证': {
    name: '道路运输证', scope: 'vehicle',
    base: ['道路运输证号', '车牌号', '经营许可证号', '车辆类型', '吨（座）位', '经营范围', '发证日期', '有效期止', '发证机关', '审验有效期至', '技术评定等级', '评定日期', '证照状态'],
    detail: ['业户名称', '经营地址', '车辆尺寸', '上次审验日期', '下次审验日期', '审验状态', '备注'],
  },
  '保险单': {
    name: '保险单', scope: 'vehicle',
    base: ['车牌号码', '保险类型', '车架号', '保险起期', '保险止期', '交强险保单号', '商业险保单号', '交强险保费', '商业险保费', '出单机构', '保单状态'],
    detail: ['保单号', '保险公司', '车辆ID', 'VIN码', '被保险人名称', '险种名称', '保额', '保费', '总保费', '起保日期', '终保日期', '缴费状态', '发票号', '到期提醒天数', '是否续保'],
  },
  '驾驶证': {
    name: '驾驶证', scope: 'driver',
    base: ['驾驶证号', '司机姓名', '准驾车型', '初次领证日期', '有效期起', '有效期止', '发证机关', '驾驶证状态', '到期提醒天数', '提醒状态'],
    detail: [],
  },
  '从业资格证': {
    name: '从业资格证', scope: 'driver',
    base: ['从业资格证号', '司机姓名', '从业资格类别', '准运范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态', '到期提醒天数', '提醒状态', '审验状态'],
    detail: [],
  },
}

const RECORD_COLS: Record<string, ColDef[]> = {
  tire: [
    { key: 'install_date', label: '安装日期', def: true, tip: '轮胎安装日期', value: (r) => r.data?.install_date || '—' },
    { key: 'vehicle', label: '安装车辆', def: true, tip: '轮胎安装的车辆', value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'tire_no', label: '轮胎编号', def: true, value: (r) => r.data?.tire_no || '—' },
    { key: 'brand', label: '品牌', def: true, value: (r) => r.data?.brand || '—' },
    { key: 'spec', label: '规格型号', def: true, value: (r) => r.data?.spec || '—' },
    { key: 'tire_type', label: '轮胎类型', def: true, value: (r) => r.data?.tire_type || '—' },
    { key: 'install_pos', label: '安装轮位', def: false, value: (r) => r.data?.install_pos || '—' },
    { key: 'status', label: '当前状态', def: true, value: (r) => r.data?.status || '—' },
    { key: 'amount', label: '购置单价(元)', def: false, tip: '购置单价', value: (r) => fmtMoney(r.amount) },
    { key: 'supplier', label: '供应商', def: false, value: (r) => r.data?.supplier || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  inspection: [
    { key: 'next_date', label: '下次年检日期', def: true, value: (r) => r.data?.next_date || '—' },
    { key: 'vehicle', label: '车牌号', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'inspection_type', label: '年检类型', def: true, value: (r) => r.data?.inspection_type || '—' },
    { key: 'cycle', label: '检验周期', def: true, value: (r) => r.data?.cycle || '—' },
    { key: 'last_date', label: '上次年检日期', def: false, value: (r) => r.data?.last_date || '—' },
    { key: 'remind_status', label: '提醒状态', def: true, value: (r) => r.data?.remind_status || '—' },
    { key: 'process_status', label: '办理状态', def: true, value: (r) => r.data?.process_status || '—' },
    { key: 'result', label: '检测结果', def: true, value: (r) => r.data?.result || '—' },
    { key: 'amount', label: '检测费(元)', def: false, value: (r) => fmtMoney(r.amount) },
    { key: 'total_fee', label: '总费用(元)', def: false, value: (r) => fmtMoney(r.data?.total_fee) },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  'fuel-charge': [
    { key: 'record_date', label: '交易日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车牌号', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'bill_no', label: '记录单号', def: true, value: (r) => r.data?.bill_no || '—' },
    { key: 'trade_type', label: '交易类型', def: true, value: (r) => r.data?.trade_type || '—' },
    { key: 'oil_type', label: '油品/电价类型', def: true, value: (r) => r.data?.oil_type || '—' },
    { key: 'qty', label: '数量', def: true, value: (r) => fmtNum(r.data?.qty) },
    { key: 'unit_price', label: '单价', def: true, value: (r) => fmtNum(r.data?.unit_price) },
    { key: 'amount', label: '消费金额(元)', def: true, tip: '消费金额', value: (r) => fmtMoney(r.amount) },
    { key: 'mileage', label: '消费时里程', def: false, value: (r) => fmtNum(r.mileage) },
    { key: 'consumption', label: '百公里油耗/电耗', def: false, value: (r) => fmtNum(r.data?.consumption) },
    { key: 'pay_method', label: '支付方式', def: false, value: (r) => r.data?.pay_method || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  'road-toll': [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车牌号码', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'bill_no', label: '记录单号', def: true, value: (r) => r.data?.bill_no || '—' },
    { key: 'fee_type', label: '费用类型', def: true, value: (r) => r.data?.fee_type || '—' },
    { key: 'entry', label: '入口站/起点', def: false, value: (r) => r.data?.entry || '—' },
    { key: 'exit', label: '出口站/终点', def: false, value: (r) => r.data?.exit || '—' },
    { key: 'tax_amount', label: '税额', def: false, value: (r) => fmtNum(r.data?.tax_amount) },
    { key: 'tax_total', label: '含税金额', def: false, value: (r) => fmtNum(r.data?.tax_total) },
    { key: 'amount', label: '总金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'pay_method', label: '支付方式', def: true, value: (r) => r.data?.pay_method || '—' },
    { key: 'ticket_no', label: '票据号码', def: false, value: (r) => r.data?.ticket_no || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  'repair-cost': [
    { key: 'record_date', label: '维修日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车牌号', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'project', label: '维修项目', def: true, value: (r) => r.data?.project || '—' },
    { key: 'repair_type', label: '维修类别', def: true, value: (r) => r.data?.repair_type || '—' },
    { key: 'supplier', label: '维修厂', def: true, value: (r) => r.data?.supplier || '—' },
    { key: 'work_no', label: '工单号', def: false, value: (r) => r.data?.work_no || '—' },
    { key: 'labor_fee', label: '工时费', def: false, value: (r) => fmtMoney(r.data?.labor_fee) },
    { key: 'material_fee', label: '材料费', def: false, value: (r) => fmtMoney(r.data?.material_fee) },
    { key: 'amount', label: '总费用(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'mileage', label: '里程(km)', def: false, value: (r) => fmtNum(r.mileage) },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  'insurance-claim': [
    { key: 'record_date', label: '出险日期', def: true, value: (r) => r.data?.accident_date || r.record_date || '—' },
    { key: 'vehicle', label: '车牌号', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'claim_no', label: '理赔单号', def: true, value: (r) => r.data?.claim_no || '—' },
    { key: 'company', label: '保险公司', def: true, value: (r) => r.data?.company || '—' },
    { key: 'insurance_type', label: '险种', def: true, value: (r) => r.data?.insurance_type || '—' },
    { key: 'accident_type', label: '事故类型', def: true, value: (r) => r.data?.accident_type || '—' },
    { key: 'assess_amount', label: '定损金额', def: false, value: (r) => fmtMoney(r.data?.assess_amount) },
    { key: 'amount', label: '理赔金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'status', label: '理赔状态', def: true, value: (r) => r.data?.status || '—' },
    { key: 'pay_date', label: '赔付日期', def: false, value: (r) => r.data?.pay_date || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  violation: [
    { key: 'record_date', label: '违章日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车牌号', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'violation_no', label: '违章单号', def: true, value: (r) => r.data?.violation_no || '—' },
    { key: 'driver_name', label: '司机姓名', def: true, value: (r) => r.data?.driver_name || '—' },
    { key: 'violation_type', label: '违章类型', def: true, value: (r) => r.data?.violation_type || '—' },
    { key: 'location', label: '违章地点', def: true, value: (r) => r.data?.location || '—' },
    { key: 'points', label: '扣分', def: true, value: (r) => r.data?.points ?? '—' },
    { key: 'amount', label: '罚款金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'status', label: '处理状态', def: true, value: (r) => r.data?.status || '—' },
    { key: 'pay_date', label: '缴款日期', def: false, value: (r) => r.data?.pay_date || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  material: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'material_type', label: '类型', def: true, value: (r) => r.data?.material_type || '—' },
    { key: 'quantity', label: '数量', def: true, value: (r) => fmtNum(r.data?.quantity) },
    { key: 'unit_price', label: '单价(元)', def: false, value: (r) => fmtNum(r.data?.unit_price) },
    { key: 'amount', label: '金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  'car-request': [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'applicant', label: '申请人', def: true, value: (r) => r.data?.applicant || '—' },
    { key: 'reason', label: '事由', def: true, value: (r) => r.data?.reason || '—' },
    { key: 'start_time', label: '开始时间', def: true, value: (r) => r.data?.start_time || '—' },
    { key: 'end_time', label: '结束时间', def: false, value: (r) => r.data?.end_time || '—' },
    { key: 'approver', label: '审批人', def: false, value: (r) => r.data?.approver || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
}

const BILLING_COLS: ColDef[] = [
  { key: 'plate_no', label: '车牌号', def: true, value: (r: any) => r.plate_no || '—' },
  { key: 'vehicle_type', label: '车辆类型', def: false, value: (r: any) => r.vehicle_type || '—' },
  { key: 'fuel_amount', label: '加油充电(元)', def: true, value: (r: any) => fmtMoney(r.fuel_amount) },
  { key: 'maintain_amount', label: '维修保养(元)', def: true, value: (r: any) => fmtMoney(r.maintain_amount) },
  { key: 'insurance_amount', label: '保险理赔(元)', def: true, value: (r: any) => fmtMoney(r.insurance_amount) },
  { key: 'tire_amount', label: '轮胎(元)', def: false, value: (r: any) => fmtMoney(r.tire_amount) },
  { key: 'material_amount', label: '辅材(元)', def: false, value: (r: any) => fmtMoney(r.material_amount) },
  { key: 'violation_amount', label: '违章罚款(元)', def: true, value: (r: any) => fmtMoney(r.violation_amount) },
  { key: 'toll_amount', label: '路桥费(元)', def: true, value: (r: any) => fmtMoney(r.toll_amount) },
  { key: 'total_amount', label: '合计(元)', def: true, tip: '各项费用合计', value: (r: any) => fmtMoney(r.total_amount) },
  { key: 'record_count', label: '记录数', def: false, value: (r: any) => r.record_count ?? '—' },
]

// 证照型固定列
const ARCHIVE_FIXED: ColDef[] = [
  { key: 'doc_type', label: '证照类型', def: true, tip: '证照类型', value: (r: any) => r.doc_type || '—' },
  { key: 'file_name', label: '源文件', def: true, tip: '来源文件', value: (r: any) => r.file_name || '—' },
  { key: 'vehicle', label: '车牌号', def: true, tip: '关联车辆', value: (r: any) => vehiclePlate(r.vehicle_id) },
]

// 证件状态：单选选项 + 按有效期自动计算（各证照类型统一，不依赖模型提取）
const CERT_STATUS_OPTIONS = ['有效', '即将到期', '已过期']
// 状态字段：不同证照类型命名不同，统一按有效期自动判定
const STATUS_FIELD_KEYS = ['证件状态', '状态', '驾驶证状态', '从业资格证状态', '保单状态']
// 有效期字段：优先"有效期止"，其次行驶证的"有效期"、"有效期至"
const EXPIRE_DATE_KEYS = ['审验有效期至', '有效期止', '有效期至', '有效期', '检验有效期至', '检验有效期', '到期日期', '下次审验日期', '检验期', '保险止期']
function isStatusField(k?: string): boolean { return !!k && STATUS_FIELD_KEYS.includes(k) }
function parseCertDate(v?: string): Date | null {
  if (!v) return null
  const s = String(v).trim()
  // 在文本任意位置找日期（值常带中文前缀，如"检验有效期至2023年7月"）
  let m = s.match(/(\d{4})\s*[-/年.]\s*(\d{1,2})\s*[-/月.]\s*(\d{1,2})/)
  if (m) return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  // 仅年月，如"2023年7月"
  m = s.match(/(\d{4})\s*[-/年.]\s*(\d{1,2})\s*月?/)
  if (m) return new Date(Number(m[1]), Number(m[2]) - 1, 1)
  return null
}
function getExpireDate(data: any): Date | null {
  for (const k of EXPIRE_DATE_KEYS) { const d = parseCertDate(data?.[k]); if (d) return d }
  return null
}
function calcCertStatus(data: any): string {
  const end = getExpireDate(data)
  if (!end) return data?.['证件状态'] || data?.['状态'] || data?.['驾驶证状态'] || '有效'
  const today = new Date(); today.setHours(0, 0, 0, 0)
  const endT = new Date(end); endT.setHours(0, 0, 0, 0)
  if (endT.getTime() < today.getTime()) return '已过期'
  const days = Math.ceil((endT.getTime() - today.getTime()) / 86400000)
  if (days <= 30) return '即将到期'
  return '有效'
}
// 证件状态颜色：有效=绿、即将到期=橙、已过期=红，与上传/解析状态标签样式统一
function certStatusTheme(v?: string): string {
  if (v === '已过期') return 'danger'
  if (v === '即将到期') return 'warning'
  if (v === '有效') return 'success'
  return 'default'
}

// 全部证照类型（文件维度）列：文件别名 / 证照类型 / 源文件 / 文件类型 / 上传状态 / 解析状态 / 提取状态 / 说明 / 上传时间 / 标签
const FILE_UPLOAD_LABEL: Record<string, string> = { completed: '已完成', failed: '上传失败', pending: '上传中' }
const FILE_PARSE_LABEL: Record<string, string> = { completed: '解析完成', parsing: '解析中', pending: '解析中', processing: '解析中', failed: '解析失败' }
const FILE_EXTRACT_LABEL: Record<string, string> = { success: '提取完成', failed: '提取失败' }
function fmtFileTime(v?: string) { return v ? String(v).replace('T', ' ').slice(0, 19) : '—' }

const ALL_FILE_COLS: ColDef[] = [
  { key: 'alias', label: '文件别名', def: true, value: (r: any) => r.alias || r.file_name || '—' },
  { key: 'doc_type', label: '证照类型', def: true, value: (r: any) => r.doc_type || '—' },
  { key: 'file_name', label: '源文件', def: true, value: (r: any) => r.file_name || '—' },
  { key: 'file_type', label: '文件类型', def: true, value: (r: any) => r.file_type || '—' },
  { key: 'upload_status', label: '上传状态', def: true, value: (r: any) => FILE_UPLOAD_LABEL[r.upload_status] || FILE_UPLOAD_LABEL.completed },
  {
    key: 'parse_status', label: '解析状态', def: true,
    value: (r: any) => FILE_PARSE_LABEL[r.parse_status] || '待解析',
    tip: (r: any) => (r.parse_status === 'failed' ? (r.parse_fail_reason || '解析失败') : ''),
  },
  {
    key: 'extract_status', label: '提取状态', def: true,
    value: (r: any) => FILE_EXTRACT_LABEL[r.extract_status] || (r.parse_status === 'completed' ? '待提取' : '—'),
    tip: (r: any) => (r.extract_status === 'failed' ? (r.extract_error || '提取失败') : ''),
  },
  {
    key: 'fail_reason', label: '说明', def: true,
    value: (r: any) => (r.parse_status === 'failed' ? (r.parse_fail_reason || '解析失败') : r.extract_status === 'failed' ? (r.extract_error || '提取失败') : r.upload_error ? r.upload_error : '—'),
  },
  { key: 'created_at', label: '上传时间', def: true, value: (r: any) => fmtFileTime(r.created_at) },
  { key: 'tags', label: '标签', def: true, value: (r: any) => (r.tags || []).map((t: any) => t.name).join('、') || '—' },
]

// ---------------------------------------------------------------------------
// 数据
// ---------------------------------------------------------------------------
const vehicles = ref<any[]>([])
const drivers = ref<any[]>([])
const cards = ref<any[]>([])
const categories = ref<Record<string, any[]>>({ vehicle: [], driver: [], maintain: [] })
const rows = ref<any[]>([])
const fileRows = ref<any[]>([])
const loading = ref(false)
const filters = reactive({ month: '', vehicle_id: '' as any, docType: '' })

const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.id })))
const searchText = ref('')

const displayRows = computed(() => {
  if (isBilling.value) return rows.value
  const base = isArchive.value && !filters.docType ? fileRows.value : rows.value
  const q = searchText.value.trim().toLowerCase()
  if (!q) return base
  return base.filter((r: any) => {
    const hay = [vehiclePlate(r.vehicle_id), r.doc_type || '', r.file_name || '', r.data?.bill_no || '', r.data?.violation_no || '', ...Object.values(r.data || {}).map(String)].join(' ').toLowerCase()
    return hay.includes(q)
  })
})

// 文件行（全部证照类型列表）标记与状态样式
const isFileRow = (r: any) => isArchive.value && !filters.docType && r?.__file === true
function cellTitle(col: ColDef, row: any) {
  return typeof col.tip === 'function' ? String(col.tip(row) ?? '') : (col.tip || String(col.value(row) ?? ''))
}
const parseTagTheme = (s?: string) => (s === 'completed' ? 'success' : s === 'failed' ? 'danger' : s === 'parsing' || s === 'pending' || s === 'processing' ? 'warning' : 'default')
const uploadTagTheme = (s?: string) => (s === 'completed' ? 'success' : s === 'failed' ? 'danger' : s === 'pending' ? 'warning' : 'default')
const extractTagTheme = (r: any) => (r.extract_status === 'success' ? 'success' : r.extract_status === 'failed' ? 'danger' : r.parse_status === 'completed' ? 'warning' : 'default')

// 无筛选时的统计概览卡片：文件级生命周期聚合 + 各证照类型快捷入口
const historyFilter = ref<"all" | "parse_failed" | "extract_failed">("all")
const overviewCards = computed(() => {
  const all = fileRows.value || []
  const cards: any[] = [
    { key: "total", label: "已上传文件", num: all.length, icon: "file", cls: "", action: "history-all" },
    { key: "parseFailed", label: "解析失败", num: all.filter((r: any) => r.parse_status === "failed").length, icon: "close-circle", cls: "", action: "history-parse_failed" },
    { key: "extractFailed", label: "提取失败", num: all.filter((r: any) => (r.extract_status || "") === "failed").length, icon: "close-circle", cls: "", action: "history-extract_failed" },
  ]
  // 各证照类型卡片（按文件数聚合），点击直接跳列表
  const byType = new Map<string, number>()
  all.forEach((r: any) => { if (r.doc_type) byType.set(r.doc_type, (byType.get(r.doc_type) || 0) + 1) })
  byType.forEach((num, name) => {
    const def = (BUILTIN_CERTS as any)[name]
    cards.push({ key: "type-" + name, label: name, num, icon: "file-copy", cls: "", action: "type", value: name, scope: def?.scope || 'vehicle' })
  })
  return cards
})
const overviewStatusCards = computed(() => overviewCards.value.filter((c: any) => !String(c.key).startsWith('type-')))
const overviewVehicleTypeCards = computed(() => overviewCards.value.filter((c: any) => String(c.key).startsWith('type-') && c.scope === 'vehicle'))
const overviewDriverTypeCards = computed(() => overviewCards.value.filter((c: any) => String(c.key).startsWith('type-') && c.scope === 'driver'))
function onOverviewCardClick(card: any) {
  if (card.action === "history-all") { historyFilter.value = "all"; historyVisible.value = true }
  else if (card.action === "history-parse_failed") { historyFilter.value = "parse_failed"; historyVisible.value = true }
  else if (card.action === "history-extract_failed") { historyFilter.value = "extract_failed"; historyVisible.value = true }
  else if (card.action === "type") { filters.docType = card.value; historyVisible.value = false; initColumns(); loadRecords() }
}

const docTypeOptions = computed(() => {
  if (!isArchive.value) return []
  const set = new Map<string, { label: string; value: string }>()
  // 仅加载已解析提取且存在数据的证照类型（来自知识库文件 meta + 记录）
  ;(fileRows.value || []).forEach((f: any) => { if (f.doc_type) set.set(f.doc_type, { label: f.doc_type, value: f.doc_type }) })
  ;(rows.value || []).forEach((r: any) => { if (r.doc_type) set.set(r.doc_type, { label: r.doc_type, value: r.doc_type }) })
  return [...set.values()]
})

const emptyText = computed(() => {
  if (isBilling.value) return '暂无数据，请选择月份'
  if (isArchive.value) return '暂无证照，上传后自动解析提取字段'
  return `暂无${groupLabel.value}`
})

async function loadBase() {
  try {
    const [vr, dr, cr] = await Promise.all([
      listFleetVehicles(),
      listFleetDrivers().catch(() => ({ data: [] })),
      listFleetFuelCards().catch(() => ({ data: [] })),
    ])
    vehicles.value = vr.data || []
    drivers.value = dr.data || []
    cards.value = cr.data || []
    const f = TYPES['fuel-charge']?.fields.find((x: any) => x.key === 'card_id')
    if (f) f.options = cards.value.map((c: any) => ({ label: c.card_no || c.alias, value: c.id }))
    if (isArchive.value) {
      await reloadCategories()
    }
  } catch { /* ignore */ }
}

// 只重载证照分类（上传弹窗类型选项、证照类型筛选框依赖 categories）
async function reloadCategories() {
  if (!isArchive.value) return
  try {
    const [crv, crd, crm] = await Promise.all([
      listFleetCategories({ scope: 'vehicle' }),
      listFleetCategories({ scope: 'driver' }),
      listFleetCategories({ scope: 'maintain' }),
    ])
    categories.value = { vehicle: crv.data || [], driver: crd.data || [], maintain: crm.data || [] }
  } catch { /* ignore */ }
}
// 设置抽屉（证照配置/提取规则）变更分类后同步刷新
function onCategoriesChanged() { void reloadCategories() }

async function loadRecords() {
  loading.value = true
  try {
    if (isBilling.value) {
      if (!filters.month) { rows.value = []; loading.value = false; return }
      const res = await getFleetSummary({ month: filters.month, vehicle_id: filters.vehicle_id || undefined })
      rows.value = res.data || []
    } else if (isArchive.value) {
      // 证照型：始终刷新文件列表（供“全部证照类型”视图与证照类型选项）；筛选类型时再加载对应记录
      rows.value = []
      if (kbId.value) {
        const fres: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
        let arr = Array.isArray(fres?.data) ? fres.data : Array.isArray(fres?.list) ? fres.list : []
        const scope = group.value.scope
        fileRows.value = arr
          .filter((it: any) => {
            const meta2 = it.custom_metadata || {}
            // 该 kb 下所有文件都属于当前 scope，仅排除历史抽屉中已隐藏的条目
            return !meta2.fleet_history_hidden
          })
          .map((it: any) => {
            const meta2 = it.custom_metadata || {}
            return {
              __file: true,
              id: it.id,
              meta: meta2,
              title: it.title || '',
              // 别名 = 标题（可改）；源文件 = 上传时的原始文件名（不可改）
              alias: it.title || it.file_name || it.name || '',
              file_name: it.file_name || it.name || it.title || '',
              file_type: it.file_type || '',
              upload_status: 'completed',
              upload_error: it.error_message || '',
              tags: Array.isArray(it.tags) ? it.tags : [],
              parse_status: it.parse_status || '',
              parse_fail_reason: it.parse_fail_reason || it.reason || (it.parse_status === 'failed' ? '解析失败' : ''),
              created_at: it.created_at || '',
              // 用户手动选择的类型（fleet_cert_type）优先，模型自动判定仅作兜底
              doc_type: meta2.fleet_cert_type || meta2.doc_type || '',
              extract_status: meta2.extract_status || '',
              extract_error: meta2.extract_error || '',
            }
          })
      }
      if (filters.docType) {
        const res = await listFleetRecords({
          type: group.value.recordType || props.recordType,
          doc_type: filters.docType,
        })
        rows.value = res.data || []
      }
    } else {
      const res = await listFleetRecords({
        type: group.value.recordType || props.recordType,
        month: filters.month || undefined,
        vehicle_id: filters.vehicle_id || undefined,
        doc_type: filters.docType || undefined,
      })
      rows.value = res.data || []
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function refreshAll() {
  Promise.all([loadBase(), ensureKb()]).then(() => { loadRecords(); loadPending() })
}
function onDocTypeChange() { initColumns(); loadRecords() }
function onDateChange() {
  if (form.record_date && !form.record_month) {
    form.record_month = String(form.record_date).slice(0, 7)
  }
}

watch(() => props.recordType, () => {
  filters.month = ''
  filters.vehicle_id = '' as any
  filters.docType = ''
  searchText.value = ''
  activeGroup.value = archiveGroups.value[0]?.key || ''
  initColumns()
  loadRecords()
})

onMounted(async () => {
  activeGroup.value = archiveGroups.value[0]?.key || ''
  await Promise.all([loadBase(), ensureKb()])
  initColumns()
  loadRecords()
  startPolling()
  window.addEventListener('fleet-categories-changed', onCategoriesChanged)
})
onBeforeUnmount(() => {
  window.removeEventListener('fleet-categories-changed', onCategoriesChanged)
  stopPolling()
  stopProgressTimer()
})

// ---------------------------------------------------------------------------
// 列定义与字段选择器
// ---------------------------------------------------------------------------
const archiveTypeFields = computed(() => {
  const t = filters.docType
  const builtin = t ? BUILTIN_CERTS[t] : null
  // 用户分类配置优先（与设置页保持同步）；内置定义兜底
  const cat = (categories.value[group.value.scope] || []).find((c: any) => c.name === t)
  const enabledSet = new Set(
    (cat?.subs || [])
      .filter((s: any) => s.enabled !== false && s.name && !String(s.name).includes('其它文档中出现的字段'))
      .map((s: any) => s.name)
  )
  if (builtin) {
    // 以内置定义的 base/detail 为准；base 字段始终显示（不受用户启用状态影响），detail 字段根据用户启用状态决定
    const base = [...builtin.base]
    const detail = (builtin.detail || []).filter((k: string) => enabledSet.size === 0 || enabledSet.has(k))
    // 用户新增的字段（内置定义没有的）归入 detail
    const known = new Set([...builtin.base, ...(builtin.detail || [])])
    const extra = [...enabledSet].filter((k: string) => !known.has(k))
    return { base, detail: [...detail, ...extra] }
  }
  if (cat?.subs?.length) {
    const subs = [...enabledSet]
    if (subs.length) return { base: subs, detail: [] }
  }
  return { base: [], detail: [] }
})

// 编辑抽屉字段渲染顺序：按当前类型配置字段顺序（备注固定排证件状态后），多余字段追加末尾
const archiveEditKeys = computed(() => {
  const f = archiveTypeFields.value
  const ordered = [...(f.base || []), ...(f.detail || [])]
  // 严格按配置字段渲染：未提取到的也显示空输入框；模型额外输出的字段不显示（与列表字段保持一致）
  return [...ordered]
})

const columnDefs = computed<ColDef[]>(() => {
  if (isBilling.value) return BILLING_COLS
  if (isArchive.value) {
    if (!filters.docType) return ALL_FILE_COLS
    const f = archiveTypeFields.value
    const keys = [...f.base]
    f.detail.forEach((k) => { if (!keys.includes(k)) keys.push(k) })
    const dyn = keys.map((k) => ({
      key: `data.${k}`, label: k, def: f.base.includes(k) && k !== '备注', tip: `提取自源文件：${k}`,
      value: (r: any) => {
        // 证件状态：各证照类型统一按有效期自动计算，不依赖模型提取值
        if (isStatusField(k)) return calcCertStatus(r.data)
        return fmtVal(r.data?.[k])
      },
    }))
    // 具体证照类型视图：仅展示配置字段，不含固定列（证照类型/源文件/车牌号），与设置页字段同步
    return dyn
  }
  return RECORD_COLS[props.recordType] || []
})

const STORAGE_KEY = computed(() => {
  const scopePart = isArchive.value ? group.value.key : props.recordType
  const mode = isArchive.value && !filters.docType ? 'all-v3' : (filters.docType || 'all-v3')
  return `weknora-fleet-${scopePart}-${mode}-cols-v3`
})
const visibleKeys = ref<string[]>([])
const visibleColDefs = computed(() => columnDefs.value.filter((c) => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => {
  const n = visibleColDefs.value.length
  // 列宽按内容自适应：长内容列（表头/数据）分配更多宽度，短列收缩；总和 100%，无横向滚动条
  // CJK 按 14px/字、ASCII 按 8.5px/字估算；超长内容仍以省略号兜底（title 悬浮查看全文）
  const weights = visibleColDefs.value.map((col) => {
    let maxStr = col.label
    for (const r of displayRows.value) {
      let t = ''
      try { t = String(col.value(r) ?? '') } catch { t = '' }
      if (t.length > maxStr.length) maxStr = t
    }
    let px = 0
    for (const ch of maxStr) px += /[\u3000-\u9fff\uff00-\uffef]/.test(ch) ? 14 : 8.5
    px += 24
    return Math.max(1, Math.min(14, Math.round(px / 45)))
  })
  return {
    gridTemplateColumns: `44px ${weights.map((w) => `${w}fr`).join(' ')}`,
    minWidth: '100%',
  }
})
const fieldPopupVisible = ref(false)

function persistColumns() { try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ } }
function resetColumns() { visibleKeys.value = columnDefs.value.filter((c) => c.def).map((c) => c.key); persistColumns() }
function selectAllColumns() { visibleKeys.value = columnDefs.value.map((c) => c.key); persistColumns() }
function initColumns() {
  const saved = localStorage.getItem(STORAGE_KEY.value)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      if (Array.isArray(arr) && arr.length) {
        const matched = columnDefs.value.map((c) => c.key).filter((k) => arr.includes(k))
        // 存档必须完整包含全部默认列才采用（列定义升级后旧存档自动重置为新默认，避免缺列）
        const defs = columnDefs.value.filter((c) => c.def).map((c) => c.key)
        if (defs.length && defs.every((k) => matched.includes(k))) {
          visibleKeys.value = matched
          return
        }
      }
    } catch { /* ignore */ }
  }
  resetColumns()
}

// ---------------------------------------------------------------------------
// 选择与汇总
// ---------------------------------------------------------------------------
const selectedKeys = ref<Set<string>>(new Set())
const rowKey = (r: any) => (r?.__file ? `f-${r.id}` : (r.key || r.id))
const isAllSelected = computed(() => displayRows.value.length > 0 && displayRows.value.every((r) => selectedKeys.value.has(rowKey(r))))
const someSelected = computed(() => displayRows.value.some((r) => selectedKeys.value.has(rowKey(r))))
const selectedRows = computed(() => displayRows.value.filter((r) => selectedKeys.value.has(rowKey(r))))
function toggleSelect(row: any) {
  const k = rowKey(row)
  if (selectedKeys.value.has(k)) selectedKeys.value.delete(k)
  else selectedKeys.value.add(k)
  selectedKeys.value = new Set(selectedKeys.value)
}
function toggleAll(v: boolean) { selectedKeys.value = v ? new Set(displayRows.value.map((r) => rowKey(r))) : new Set() }
function clearSelection() { selectedKeys.value = new Set() }
const archiveDeleteHint = computed(() => {
  const n = selectedKeys.value.size
  if (isArchive.value) {
    const hasFile = selectedRows.value.some((r: any) => r.__file)
    if (hasFile) return `确定删除所选 ${n} 个文件吗？将同时删除对应的提取记录。`
    return `确定删除所选 ${n} 份证照记录吗？源文件保留在知识库。`
  }
  return `确定删除所选 ${n} 条记录吗？`
})
const summaryAmount = computed(() => displayRows.value.reduce((s, r) => s + (Number(r.amount) || 0), 0))
const totalAll = computed(() => displayRows.value.reduce((s, r) => s + (Number(r.total_amount) || 0), 0))

// ---------------------------------------------------------------------------
// 新增/编辑
// ---------------------------------------------------------------------------
const drawerVisible = ref(false)
const saving = ref(false)
const editMode = ref<'file' | 'record' | ''>('')
const form = reactive<any>({})
const drawerTitle = computed(() => {
  if (isArchive.value) {
    if (editMode.value === 'file') return '编辑证照'
    return (form.id ? '编辑' : '新增') + group.value.label
  }
  return (form.id ? '编辑' : '新增') + typeMeta.value.label
})
const formFields = computed(() => typeMeta.value.fields || [])

function openDrawer(row: any) {
  Object.keys(form).forEach((k) => delete form[k])
  if (row) {
    if (isArchive.value) {
      if (row.__file) {
        // 文件编辑：文件名（别名）+ 证照类型（手动匹配） + 失败原因
        editMode.value = 'file'
        form.id = ''
        form.doc_knowledge_id = row.id
        form.file_name = row.alias || row.title || row.file_name || ''
        form.source_name = row.file_name || ''
        form.file_type = row.file_type || ''
        form.doc_type = row.doc_type || ''
        form.meta = JSON.parse(JSON.stringify(row.meta || {}))
        form.fail_reason = row.extract_error || (row.parse_status === 'failed' ? (row.parse_fail_reason || '解析失败') : '')
      } else {
        editMode.value = 'record'
        form.doc_type = row.doc_type || ''
        form.file_name = row.file_name || ''
        form.file_type = row.file_type || ''
        form.doc_knowledge_id = row.doc_knowledge_id || ''
        form.vehicle_id = row.vehicle_id || ''
        form.remark = row.remark || ''
        form.data = JSON.parse(JSON.stringify(row.data || {}))
        form.id = row.id
        // 旧记录可能没有 doc_knowledge_id：按车牌号在已加载的知识文件列表里反查源文件，恢复预览
        if (!form.doc_knowledge_id) {
          const plate = String(form.data['车牌号'] || '').trim()
          // 文件名常省略省份字母，如“渝C81022”对应“81022行驶证.pdf”；先完整车牌，再去省份前缀取尾段
          const tail = plate.replace(/^[京津沪渝冀晋蒙辽吉黑苏浙皖闽赣鲁豫鄂湘粤桂琼川贵云藏陕甘青宁新][A-Z]/, '')
          const hit = (fileRows.value || []).find((f: any) => {
            const fn = String(f.file_name || f.title || '')
            return (plate && fn.includes(plate)) || (tail && tail.length >= 4 && fn.includes(tail))
          })
          if (hit) {
            form.doc_knowledge_id = hit.id
            if (!form.file_name) form.file_name = hit.file_name || hit.title || ''
            if (!form.file_type) form.file_type = hit.file_type || ''
          }
        }
        // 证件状态：各证照类型统一按有效期自动计算（可手动修改）
        const autoStatus = calcCertStatus(form.data)
        Object.keys(form.data || {}).forEach((k) => { if (isStatusField(k)) form.data[k] = autoStatus })
      }
    } else {
      editMode.value = ''
      form.vehicle_id = row.vehicle_id || ''
      form.record_date = row.record_date || ''
      form.record_month = row.record_month || ''
      form.remark = row.remark || ''
      Object.keys(row.data || {}).forEach((k) => { form[k] = row.data[k] })
      form.id = row.id
    }
  } else {
    editMode.value = isArchive.value ? 'record' : ''
    form.vehicle_id = ''
    form.record_date = ''
    form.record_month = ''
    form.remark = ''
    form.data = {}
    form.id = ''
    if (isArchive.value) {
      form.doc_type = filters.docType || ''; form.file_name = ''; form.file_type = ''; form.doc_knowledge_id = ''; form.meta = {}; form.fail_reason = ''
    }
  }
  drawerVisible.value = true
}

// 编辑抽屉内：有效期起/止变化时，证件状态自动重新计算（随 form.data 替换重新收集依赖）
watch(
  () => {
    if (!drawerVisible.value || !isArchive.value) return undefined
    return EXPIRE_DATE_KEYS.map((k) => form.data?.[k])
  },
  () => {
    if (form.data && drawerVisible.value && isArchive.value) {
      const next = calcCertStatus(form.data)
      Object.keys(form.data).forEach((k) => { if (isStatusField(k) && form.data[k] !== next) form.data[k] = next })
    }
  },
)

async function saveRecord() {
  if (isArchive.value) {
    if (editMode.value === 'file') return saveFileRecord()
    if (!form.doc_type) { MessagePlugin.warning('请选择证照类型'); return }
    const payload: any = {
      record_type: group.value.recordType,
      vehicle_id: form.vehicle_id || '',
      doc_type: form.doc_type,
      file_name: form.file_name,
      doc_knowledge_id: form.doc_knowledge_id,
      remark: form.remark,
      data: form.data || {},
    }
    saving.value = true
    try {
      if (form.id) await updateFleetRecord(form.id, payload)
      else await createFleetRecord(payload)
      MessagePlugin.success('已保存')
      drawerVisible.value = false
      loadRecords()
    } catch (e: any) {
      MessagePlugin.error(e?.message || '保存失败')
    } finally { saving.value = false }
    return
  }
  if (!form.vehicle_id) { MessagePlugin.warning('请选择车辆'); return }
  const data: any = {}
  formFields.value.forEach((f: any) => {
    if (form[f.key] !== undefined && form[f.key] !== '' && form[f.key] !== null) data[f.key] = form[f.key]
  })
  const payload: any = {
    record_type: props.recordType,
    vehicle_id: form.vehicle_id,
    record_date: form.record_date || '',
    record_month: form.record_month || '',
    remark: form.remark || '',
    data,
  }
  saving.value = true
  try {
    if (form.id) await updateFleetRecord(form.id, payload)
    else await createFleetRecord(payload)
    MessagePlugin.success('已保存')
    drawerVisible.value = false
    loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally { saving.value = false }
}

// 文件编辑保存：更新文件名 + 证照类型（打标），选择类型后重新提取归类
// 乐观优先：点击保存立即关闭抽屉并更新列表，后台完成落库（PUT 约需 1~2s）
async function saveFileRecord() {
  const kid = form.doc_knowledge_id
  if (!kid) { MessagePlugin.warning('缺少文件标识'); return }
  const title = String(form.file_name || '').trim()
  if (!title) { MessagePlugin.warning('请输入文件名'); return }
  const scope = group.value.scope
  const prev = fileRows.value.find((f: any) => f.id === kid)
  const prevTitle = prev?.title || ''
  // 乐观更新 UI（仅改别名，源文件保持上传时原名）
  drawerVisible.value = false
  if (prev) { prev.title = title; prev.alias = title }
  saving.value = true
  try {
    const meta = { ...(form.meta || {}), fleet_scope: scope }
    if (form.doc_type) meta.fleet_cert_type = form.doc_type
    if (form.doc_type) meta.doc_type = form.doc_type
    delete meta.extract_status
    delete meta.extract_error
    await updateKnowledgeInfo(kid, { title, custom_metadata: meta })
    if (form.doc_type) {
      // 手动匹配/变更类型：触发重新提取（幂等覆盖）。提取为同步耗时操作，后台执行不阻塞保存
      const row = fileRows.value.find((f: any) => f.id === kid)
      if (row && row.parse_status === 'completed') {
        extractFile(kid, scope, form.doc_type).catch((e: any) => {
          MessagePlugin.error(e?.message || '提取失败')
          loadPending()
        })
      } else {
        MessagePlugin.info('文件尚未解析完成，将自动提取')
        try {
          const pv = pendingScopeOf(kid)
          pendingScopes.value[kid] = pv.t === form.doc_type ? pv : { s: scope, t: form.doc_type }
          savePendingScopes()
        } catch { /* ignore */ }
      }
    }
    MessagePlugin.success('已保存')
    loadRecords()
    loadPending()
  } catch (e: any) {
    // 保存失败回滚乐观值
    if (prev) { prev.title = prevTitle; prev.alias = prevTitle }
    MessagePlugin.error(e?.message || '保存失败')
    loadRecords()
  } finally { saving.value = false }
}

async function handleDelete() {
  try {
    for (const row of selectedRows.value) {
      if (row.__file) {
        // 删除知识库源文件及其关联提取记录
        await delKnowledgeDetails(row.id)
        try {
          const res: any = await listFleetRecords({ type: group.value.recordType || props.recordType })
          const rel = (res.data || []).filter((r: any) => r.doc_knowledge_id === row.id)
          for (const r of rel) await deleteFleetRecord(r.id)
        } catch { /* 记录清理失败不阻塞文件删除 */ }
      } else {
        await deleteFleetRecord(row.id)
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    loadRecords()
    loadPending()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 标签：复用知识库文档标签功能 ----
const tagEditVisible = ref(false)
const tagManageVisible = ref(false)
const tagEditTarget = ref<any>(null)
const tagList = ref<any[]>([])

async function loadTagList() {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeTags(kbId.value, { page: 1, page_size: 200 })
    tagList.value = res?.data?.data || res?.data || []
  } catch { /* 标签加载失败不阻塞 */ }
}

function onFileAction(action: any, row: any) {
  const act = typeof action === 'string' ? action : action?.value
  if (!row?.id) return
  if (act === 'tags') { openTagEdit(row); return }
  if (act === 'delete') { handleDelete() }
}

// 悬浮工具栏：批量重新解析 / 重新提取（仅文件行）
const batchBusy = ref(false)
async function batchRetry(kind: 'parse' | 'extract') {
  const files = selectedRows.value.filter((r: any) => r.__file && r.id)
  if (!files.length) return
  if (batchBusy.value) return
  batchBusy.value = true
  let ok = 0, fail = 0
  for (const f of files) {
    try {
      await retryFile(f, kind)
      ok++
    } catch { fail++ }
  }
  batchBusy.value = false
  MessagePlugin.success(`已发起 ${ok} 个文件${kind === 'parse' ? '重新解析' : '重新提取'}${fail ? `，${fail} 个失败` : ''}`)
  clearSelection()
  loadPending()
}

function openTagEdit(row: any) {
  tagEditTarget.value = row
  if (!tagList.value.length) loadTagList()
  tagEditVisible.value = true
}

async function onTagConfirm(tagIds: string[]) {
  const row = tagEditTarget.value
  if (!row?.id) return
  try {
    await updateKnowledgeTagBatch({ updates: { [row.id]: tagIds } })
    MessagePlugin.success('标签已保存')
    row.tags = tagList.value.filter((t: any) => tagIds.includes(t.id))
    tagEditVisible.value = false
    loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '标签保存失败')
  }
}

// 失败文件重试：解析失败 → 重新解析；提取失败 → 重新提取
const retryingId = ref('')
async function retryFile(row: any, kind: 'parse' | 'extract') {
  if (retryingId.value) return
  retryingId.value = row.id
  try {
    if (kind === 'parse') {
      await reparseKnowledge(row.id)
      MessagePlugin.success('已重新解析，等待解析完成后自动提取')
    } else {
      const pv = pendingScopeOf(row.id)
      await extractFile(row.id, group.value.scope, row.doc_type || pv.t)
      MessagePlugin.success('已重新提取')
    }
    loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '重试失败')
  } finally {
    retryingId.value = ''
  }
}

// ---------------------------------------------------------------------------
// 打印
// ---------------------------------------------------------------------------
const printVisible = ref(false)
const printUrl = ref('')
const printCount = ref(0)
const catalogBusy = ref(false)
const printLoaded = ref(false)
const printTitle = computed(() => (isBilling.value ? `车辆费用清单（${filters.month}）` : `${groupLabel.value}目录`))

async function handlePrint() {
  const rowsToPrint = selectedRows.value.length ? selectedRows.value : displayRows.value
  if (!rowsToPrint.length) { MessagePlugin.warning('当前无数据可打印'); return }
  catalogBusy.value = true
  try {
    // 目录打印：严格按当前列表显示的字段（visibleColDefs）生成，含 data.* 字段，不截断
    const cols: CatalogColumn[] = visibleColDefs.value
      .filter((c: any) => !(isArchive.value && (c.key === 'file_name' || c.key === 'source')))
      .map((c: any) => ({ key: c.key, label: c.label, value: c.value }))
    const bytes = await generateCatalogPdf({
      title: printTitle.value,
      columns: cols,
      rows: rowsToPrint,
    })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printCount.value = rowsToPrint.length
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
  } finally {
    catalogBusy.value = false
  }
}
function closePrint() {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}
function handleSourcePrint() {
  const row = selectedRows.value[0]
  if (!row) return
  openDrawer(row)
}

// ---------------------------------------------------------------------------
// 上传与解析（证照型）
// ---------------------------------------------------------------------------
const kbId = ref('')
const pendingFiles = ref<any[]>([])
const uploadVisible = ref(false)
const historyVisible = ref(false)

// 上传任务实时进度（来自上传弹窗 emit）：多文件同时进行时轮播展示
const uploadProgress = ref<{ name: string; stage: 'uploading' | 'parsing' | 'extracting'; percent: number }[]>([])
const progressIndex = ref(0)
let progressTimer: ReturnType<typeof setInterval> | null = null
const STAGE_LABEL: Record<string, string> = { uploading: '正在上传', parsing: '正在解析', extracting: '正在提取' }
function stageLabel(stage: string) { return STAGE_LABEL[stage] || stage }
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
// 表格里该文件已"提取完成/失败"的，不再保留旧的"正在提取…30%"进度项
function filterLiveProgress(items) {
  const doneNames = new Set(
    (fileRows.value || [])
      .filter((r) => r.extract_status === 'success' || r.extract_status === 'failed')
      .map((r) => r.file_name || r.alias || r.name),
  )
  return items.filter((it) => !doneNames.has(it.name))
}
// 轮询刷新列表后：用表格实际 extract_status 清掉已完成文件的残留进度
watch(fileRows, () => {
  if (!uploadProgress.value.length) return
  uploadProgress.value = filterLiveProgress(uploadProgress.value)
})

function stopProgressTimer() {
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}

// 上传弹窗证照类型选项（当前档案组内置类型 + 已入库分类）
const uploadTypeOptions = computed(() => {
  if (!isArchive.value) return []
  const scope = group.value.scope
  const set = new Map<string, { label: string; value: string }>()
  Object.values(BUILTIN_CERTS).filter((b) => b.scope === scope).forEach((b) => set.set(b.name, { label: b.name, value: b.name }))
  ;(categories.value[scope] || []).filter((c: any) => c.enabled).forEach((c: any) => set.set(c.name, { label: c.name, value: c.name }))
  return [...set.values()]
})

function onUploadDone() {
  loadRecords()
  loadBase()
  loadPending()
}

// 上传记忆：解析流程会重置 custom_metadata（fleet_scope 丢失），
// 这里把"上传过的文件 → 档案 scope（+可选证照类型）"记到会话级存储，轮询时据此补打标并提取。
// 兼容旧格式：值可以是 scope 字符串，或 { s: scope, t: certType }
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
      description: '存放并解析车辆/司机证照与维保文件，自动提取字段',
      type: 'document',
      chunking_config: { chunk_size: 512, chunk_overlap: 80, separators: ['\n\n', '\n', '。', '；', '，', ';', '、'], enable_parent_child: true, strategy: '', token_limit: 0, languages: [], table_metadata_instructions: '' },
      embedding_model_id: emb,
      summary_model_id: llm,
      indexing_strategy: { vector_enabled: true, keyword_enabled: true, wiki_enabled: false, graph_enabled: false },
    }
    const cres: any = await createKnowledgeBase(payload)
    const kb = cres?.data || cres
    kbId.value = kb?.id || ''
    if (kbId.value) MessagePlugin.success('车队管理知识库已创建')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '知识库初始化失败')
  }
}

let pollTimer: ReturnType<typeof setInterval> | null = null
let extractingSet = new Set<string>()

async function extractFile(kid: string, scope: string, certType?: string) {
  const token = localStorage.getItem('weknora_token')
  const res = await fetch(`/api/v1/knowledge-bases/${kbId.value}/knowledge/${kid}/extract-fleet-document`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
    body: JSON.stringify(certType ? { scope, cert_type: certType } : { scope }),
  })
  const j = await res.json()
  if (!res.ok) throw new Error(j?.message || '提取失败')
}

async function loadPending() {
  if (!kbId.value || !isArchive.value) return
  // 弹窗组件可能已更新 sessionStorage 上传记忆（含证照类型），轮询时重读保持同步
  pendingScopes.value = loadPendingScopes()
  try {
    const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
    const arr = Array.isArray(res?.data) ? res.data : Array.isArray(res?.list) ? res.list : []
    const scope = group.value.scope
    const mine = arr.filter((it: any) => {
      const meta2 = it.custom_metadata || {}
      if (meta2.fleet_history_hidden) return false
      const pv = pendingScopeOf(it.id)
      // 解析流程会重置 custom_metadata，fleet_scope 可能丢失：用上传记忆兜底
      return meta2.fleet_scope === scope || meta2.scope === scope || pv.s === scope
    })
    pendingFiles.value = mine.filter((it: any) => {
      const ps = it.parse_status
      const meta2 = it.custom_metadata || {}
      if (meta2.extract_status === 'success' || meta2.extract_status === 'failed') return false
      // 仅真正进行中的文件计入「正在解析」提示条；解析失败的文件不再误报为进行中
      return ps === 'parsing' || ps === 'pending' || ps === 'processing'
    })
    for (const it of mine) {
      const ps = it.parse_status
      const meta2 = it.custom_metadata || {}
      if (ps === 'completed' && !meta2.extract_status && !extractingSet.has(it.id)) {
        extractingSet.add(it.id)
        try {
          // 解析已完成，custom_metadata 不会再被重置：此时补打标才安全
          const pv = pendingScopeOf(it.id)
          if (meta2.fleet_scope !== scope) {
            await updateKnowledgeMetadata(it.id, { ...meta2, fleet_scope: scope })
          }
          // 类型优先级：上传记忆 → 文件已打标类型（用户所选，严禁被模型改写）
          await extractFile(it.id, scope, pv.t || meta2.fleet_cert_type || '')
          delete pendingScopes.value[it.id]
          savePendingScopes()
          loadRecords()
          loadBase()
        } catch { /* 失败下轮重试 */ } finally {
          extractingSet.delete(it.id)
        }
      }
    }
  } catch { /* ignore */ }
}
function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => { loadPending() }, 4000)
}
function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---------------------------------------------------------------------------
// 抽屉宽度
// ---------------------------------------------------------------------------
const DRAWER_KEY = 'weknora-fleet-drawer-width'
const drawerWidth = ref(`${parseInt(localStorage.getItem(DRAWER_KEY) || '', 10) || 640}px`)
let resizing = false
let startX = 0
let startW = 0
function onDrawerResizeStart(e: MouseEvent) {
  resizing = true
  startX = e.clientX
  startW = parseInt(drawerWidth.value, 10)
  document.addEventListener('mousemove', onDrawerResizeMove)
  document.addEventListener('mouseup', onDrawerResizeEnd)
}
function onDrawerResizeMove(e: MouseEvent) {
  if (!resizing) return
  const w = Math.min(1200, Math.max(560, startW + (startX - e.clientX)))
  drawerWidth.value = `${w}px`
}
function onDrawerResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  localStorage.setItem(DRAWER_KEY, drawerWidth.value)
}
</script>

<style lang="less" scoped>
.utility-meter-tab {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding-top: 4px;
}

/* 筛选工具栏（与电费核算内容页一致） */
.doc-filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;

  &__leading {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    flex: 1;
  }

  &__trailing {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .doc-filter-field {
    display: flex;
    align-items: center;

    .doc-date-picker {
      width: 140px;
    }

    .doc-filter-select {
      width: 160px;
    }
  }
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

/* 列表组件样式（与电费核算内容页一致，scoped 自包含） */
.doc-list-view {
  width: 100%;
  min-width: 100%;
  box-sizing: border-box;
}

.doc-list-header,
.doc-list-row {
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

  .cell {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .col-tip {
    cursor: default;
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    vertical-align: middle;
  }
}

.doc-list-row {
  position: relative;
  min-height: 52px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  border-bottom: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background-color 0.2s ease;

  &:last-child {
    border-bottom: 0;
  }

  &:hover {
    background: var(--td-bg-color-secondarycontainer);
  }

  &.row-selected {
    background: var(--td-brand-color-light);
  }
}

.cell {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
  padding: 0 8px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &:first-child {
    padding-left: 0;
  }

  &:last-child {
    padding-right: 0;
  }
}

.cell-check {
  justify-content: center;
  padding: 0;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.cert-status-radio :deep(.t-radio-button) {
  margin-right: 8px;
  border: 1px solid var(--td-component-border) !important;
  border-radius: 4px !important;
  padding: 0 14px;
  height: 32px;
  line-height: 30px;
  background: var(--td-bg-color-container);
}
.cert-status-radio :deep(.t-radio-button.t-is-checked) {
  border-color: var(--td-brand-color) !important;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

.meter-list-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  background: var(--td-bg-color-container);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
}

.meter-empty {
  padding: 40px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);

  .meter-empty-icon {
    color: var(--td-text-color-placeholder);
  }

  .meter-empty-text {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}

/* 底部汇总（与电费核算一致） */
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

/* 抽屉 resize 手柄（发票管理同款） */
.doc-drawer-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 8px;
  z-index: 2200;
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

/* 浮动工具栏（与电费核算一致） */
.doc-batch-bar-fixed {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: 24px;
  z-index: 3000;

  .batch-bar-inner {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 8px 16px;
    border-radius: 10px;
    background: var(--td-bg-color-container);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
    border: 1px solid var(--td-component-stroke);
  }

  .batch-bar-left {
    display: flex;
    align-items: center;
    gap: 8px;

    .batch-bar-count {
      font-size: 13px;
      color: var(--td-text-color-primary);
    }

    .batch-bar-clear {
      color: var(--td-brand-color);
    }
  }

  .batch-bar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
}

.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to {
  opacity: 0;
  transform: translateX(-50%) translateY(6px);
}

/* 编辑抽屉 */
.meter-drawer-body {
  padding: 4px 0 24px;
}

/* 新增/编辑记录：两列紧凑布局 */
.rec-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 20px;
}
.rec-field {
  min-width: 0;

  :deep(.t-input__wrap),
  :deep(.t-select__wrap),
  :deep(.t-date-picker) {
    width: 100%;
    min-width: 0;
  }

  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;

    .required {
      color: var(--td-error-color);
    }
  }
}
.rec-field--wide {
  grid-column: 1 / -1;
}
/* 证件状态下拉面板：最小宽度与输入框对齐，避免菜单项溢出选择框 */
:global(.cert-status-pop) {
  min-width: 180px;
}
.readonly-val {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 13px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-component);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-default);
  padding: 0 10px;
}

.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 证照解析状态提示 */
.archive-pending-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  margin-bottom: 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  border-radius: 8px;
  font-size: 13px;

  .archive-pending-count {
    margin-left: auto;
    font-size: 12px;
    opacity: 0.75;
  }
}

/* 打印弹窗 */
.meter-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3100;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
}

.meter-print-dialog {
  width: min(960px, 92vw);
  max-width: 100%;
  height: min(720px, 88vh);
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
}

.meter-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  flex: 0 0 auto;

  .meter-print-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.meter-print-body {
  flex: 1;
  min-height: 0;
  background: var(--td-bg-color-container);
  overflow: auto;
}

.print-preview-frame {
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}

.print-preview-loading {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.meter-print-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--td-component-stroke);
  flex: 0 0 auto;
}

/* 引导式空态 */
.meter-empty-action {
  margin-top: 4px;
}

/* 文件行状态列 */
.cell-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  overflow: hidden;
}

.cell-retry {
  flex: none;
  color: var(--td-brand-color);
}

/* 标签列：与知识库-文档管理列表标签样式一致（light-outline + 溢出+N + 空态） */
.cell-tags {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px;
}
.row-tag {
  max-width: 100%;
}
.row-tag :deep(.t-tag__text) {
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 120px;
  display: inline-block;
}
.row-tag-chips {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-wrap: nowrap;
  max-width: 100%;
}
.row-tag-chips.is-clickable { cursor: pointer; }
.row-tag-overflow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 20px;
  min-width: 20px;
  padding: 0 4px;
  border-radius: 999px;
  border: 1px solid var(--td-component-stroke);
  color: var(--td-text-color-secondary);
  font-size: 12px;
  flex: none;
}
.row-tag-add {
  flex: none;
  color: var(--td-brand-color);
  font-size: 12px;
  cursor: pointer;
}

/* 操作列（三个点下拉） */
.cell-actions {
  display: inline-flex;
  align-items: center;
}
.cell-action-danger {
  color: var(--td-error-color);
}

/* 文件删除二次确认 */
.fleet-delete-confirm p { margin: 6px 0; font-size: 13px; line-height: 1.7; color: var(--td-text-color-primary); }
.fleet-delete-confirm p:last-child { color: var(--td-text-color-secondary); }

/* 失败原因只读框 */
.read-only-fail {
  color: var(--td-error-color);
  background: var(--td-error-color-1);
  border-color: var(--td-error-color-2);
}
.archive-overview { display: flex; flex-direction: column; gap: 20px; padding: 24px; }
.overview-group { display: flex; flex-direction: column; gap: 12px; }
.overview-group__title {
  font-size: 12px; font-weight: 500; color: var(--td-text-color-secondary);
  padding-left: 8px; border-left: 2px solid var(--td-brand-color); line-height: 1;
}
.overview-group__cards { display: grid; grid-template-columns: repeat(3, 240px); gap: 14px; }
.overview-group--panel { background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke); border-radius: var(--td-radius-medium); padding: 20px; }
.overview-subgroup { display: flex; flex-direction: column; gap: 12px; }
.overview-subgroup + .overview-subgroup { margin-top: 8px; }
/* 状态卡片:紧凑横向,图标圆形背景 */
.stat-card {
  display: flex; align-items: center; gap: 12px; padding: 14px 18px;
  background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke);
  border-radius: 10px; cursor: pointer; transition: all .18s ease;
}
.stat-card:hover { border-color: var(--td-brand-color); box-shadow: 0 4px 12px rgba(0,82,217,.1); transform: translateY(-1px); }
.stat-card.is-warn { border-color: #e37318; background: #fff7ed; }
.stat-card.is-err { border-color: #d54941; background: #fef2f2; }
.stat-card__icon {
  flex-shrink: 0; width: 36px; height: 36px; border-radius: 8px;
  display: flex; align-items: center; justify-content: center;
  background: var(--td-brand-color-1, #e8f3ff); color: var(--td-brand-color);
}
.stat-card.is-warn .stat-card__icon { background: #fff1e0; color: #e37318; }
.stat-card.is-err .stat-card__icon { background: #fde8e8; color: #d54941; }
.stat-card__num { font-size: 22px; font-weight: 600; line-height: 1.1; color: var(--td-text-color-primary); }
.stat-card__label { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 2px; }
/* 类型卡片:稍大,浅色背景,左图标右文字 */
.type-card {
  display: flex; align-items: center; gap: 14px; padding: 20px 22px;
  background: var(--td-bg-color-secondarycontainer, #f7f8fa); border: 1px solid transparent;
  border-radius: 10px; cursor: pointer; transition: all .18s ease;
}
.type-card:hover { background: var(--td-brand-color-1, #e8f3ff); border-color: var(--td-brand-color); transform: translateY(-1px); }
.type-card__icon {
  flex-shrink: 0; width: 40px; height: 40px; border-radius: 10px;
  display: flex; align-items: center; justify-content: center;
  background: var(--td-bg-color-container); color: var(--td-brand-color);
  box-shadow: 0 1px 3px rgba(0,0,0,.06);
}
.type-card__body { flex: 1; min-width: 0; }
.type-card__label { font-size: 14px; font-weight: 500; color: var(--td-text-color-primary); margin-bottom: 4px; }
.type-card__num { font-size: 20px; font-weight: 600; color: var(--td-brand-color); }
.type-card__num span { font-size: 12px; font-weight: 400; color: var(--td-text-color-secondary); margin-left: 2px; }
@media (max-width: 768px) {
  .overview-group__cards { grid-template-columns: 1fr; }
}
</style>
