<template>
  <div class="utilities-management-container">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>水电气管理</h2>
        <p class="header-subtitle">电费账单自动解析归档；水费、气费按月手动录入，支持多表计自动汇总</p>
      </div>
      <div class="header-actions" v-if="activeTab === 'electricity'">
        <t-button v-if="kbId" theme="primary" @click="triggerUpload">
          <template #icon><t-icon name="upload" /></template>
          上传电费账单
        </t-button>
      </div>
      <input ref="fileInputRef" type="file" multiple accept=".pdf,.jpg,.jpeg,.png" style="display: none"
        @change="onFileInputChange" />
    </div>

    <!-- 加载中 -->
    <div v-if="loading && activeTab === 'electricity'" class="loading-area">
      <t-loading size="large" text="正在初始化电费管理..." />
    </div>

    <!-- KB 不存在：空状态（仅电费 Tab 需要知识库） -->
    <div v-else-if="activeTab === 'electricity' && !kbId" class="empty-area">
      <t-empty description="尚未创建「日常事务-电费」知识库">
        <template #image><t-icon name="dashboard" size="64px" /></template>
      </t-empty>
      <t-button theme="primary" @click="wizardVisible = true">创建电费知识库</t-button>
    </div>

    <!-- 主界面 -->
    <div v-else class="utilities-main">
      <t-tabs v-model="activeTab" class="utilities-tabs" @change="onTabChange">
        <t-tab-panel value="electricity" label="电费">
          <div v-if="kbId" class="electricity-panel">
            <!-- 筛选工具栏 -->
            <div class="doc-filter-bar">
              <div class="doc-filter-bar__leading">
                <div class="doc-filter-field doc-filter-field--search">
                  <t-input v-model="keyword" placeholder="搜索全部字段" clearable class="doc-search doc-filter-field__control"
                    @enter="applyFilter" @clear="applyFilter">
                    <template #prefixIcon><t-icon name="search" size="16px" /></template>
                  </t-input>
                </div>
                <div class="doc-filter-field doc-filter-field--wide">
                  <t-date-range-picker v-model="dateRange" placeholder="账单周期" class="doc-date-range doc-filter-field__control"
                    clearable allow-input @change="applyFilter">
                    <template #prefixIcon><t-icon name="time" size="16px" /></template>
                  </t-date-range-picker>
                </div>
                <t-popup v-model="fieldPopupVisible" trigger="click" placement="bottom-left" :hide-empty-popup="false"
                  overlay-inner-class="utility-field-popup">
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
                      <t-checkbox-group v-model="visibleColKeys" class="field-popup-list">
                        <t-checkbox v-for="col in columnDefs" :key="col.key" :value="col.key" class="field-popup-item">
                          {{ col.label }}
                        </t-checkbox>
                      </t-checkbox-group>
                    </div>
                  </template>
                </t-popup>
                <t-button variant="outline" size="small" @click="loadFiles(true)">
                  <template #icon><t-icon name="refresh" size="14px" /></template>
                </t-button>
                <t-tooltip content="设置" placement="bottom">
                  <t-button variant="outline" size="small" @click="settingsVisible = true">
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

            <!-- 汇总行（选中时统计选中） -->
            <div v-if="summary.total" class="utility-summary-row" :class="{ 'with-toolbar': selectedRowKeys.length }">
              <span>共 {{ summary.total }} 条</span>
              <span>本期电量 {{ fmtKwh(summaryUsage) }} 千瓦时</span>
              <span>本期电费 {{ fmtMoney(summaryAmount) }} 元</span>
              <span v-if="selectedRowKeys.length" class="summary-selected">已选 {{ selectedRowKeys.length }} 条</span>
            </div>

            <!-- 列表 -->
            <div class="doc-list-scroll" ref="listScrollRef" @scroll="onListScroll">
              <div class="doc-list-view">
                <!-- 进行中的文件状态行（解析中/提取中/待提取） -->
                <div v-for="pf in pendingFiles" :key="pf.id" class="doc-pending-row">
                  <t-tag size="small" theme="warning" variant="light-outline" class="row-status-tag">
                    <template #icon><t-icon name="loading" class="icon-spin" /></template>
                    {{ pendingLabel(pf) }}
                  </t-tag>
                  <span class="pending-name">{{ pf.file_name }}</span>
                </div>
                <div class="doc-list-header" :style="gridStyle" role="row">
                  <div class="cell cell-check" role="columnheader" @click.stop>
                    <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
                      :disabled="!selectableRows.length" title="全选" @change="toggleSelectAll" />
                  </div>
                  <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
                    {{ col.label }}
                  </div>
                  <div class="cell cell-extractStatus" role="columnheader">状态</div>
                  <div class="cell cell-tags" role="columnheader">标签</div>
                </div>
                <div class="doc-list-body">
                  <div v-for="row in rows" :key="row.rowKey" class="doc-list-row" :style="gridStyle"
                    :class="{ selected: selectedRowKeys.includes(row.rowKey) }" role="row" @click="openDetail(row)">
                    <div class="cell cell-check" @click.stop>
                      <t-checkbox class="doc-list-check" size="small" :checked="selectedRowKeys.includes(row.rowKey)"
                        :disabled="row.kind === 'pending'" @change="(c: boolean) => toggleRow(row.rowKey, c)" />
                    </div>
                    <template v-for="col in visibleColDefs" :key="col.key">
                      <div class="cell" :class="`cell-${col.key}`">
                        <span v-if="col.fieldType === 'number' || col.fieldType === 'amount'" class="row-mono" :title="cellText(row, col.key)">
                          {{ cellText(row, col.key) }}
                        </span>
                        <span v-else class="row-text" :title="cellText(row, col.key)">{{ cellText(row, col.key) }}</span>
                      </div>
                    </template>
                    <div class="cell cell-extractStatus">
                      <t-tag v-if="statusOf(row).label !== '--'" size="small" :theme="statusOf(row).theme"
                        variant="light-outline" class="row-status-tag">
                        <template v-if="statusOf(row).icon" #icon>
                          <t-icon :name="statusOf(row).icon!" :class="{ 'icon-spin': statusOf(row).spin }" />
                        </template>
                        {{ statusOf(row).label }}
                      </t-tag>
                    </div>
                    <div class="cell cell-tags">
                      <t-tag v-for="(t, ti) in rowTags(row).slice(0, 2)" :key="ti" size="small" variant="light"
                        theme="primary" class="row-tag">
                        {{ t.name || t }}
                      </t-tag>
                      <t-tag v-if="rowTags(row).length > 2" size="small" variant="light" class="row-tag">+{{ rowTags(row).length - 2 }}</t-tag>
                      <t-button v-if="rowTags(row).length" variant="text" size="small" class="row-tag-btn" @click.stop="openTagEdit(row)">
                        <template #icon><t-icon name="edit-1" size="13px" /></template>
                      </t-button>
                    </div>
                  </div>
                  <div v-if="listLoading" class="list-loading">
                    <t-loading size="small" text="加载中..." />
                  </div>
                  <div v-else-if="!rows.length && !pendingFiles.length" class="list-empty">
                    <t-empty description="暂无数据" />
                  </div>
                </div>
              </div>
            </div>

            <!-- 浮动工具栏（选中时显示，自动避让汇总行） -->
            <div v-if="selectedRowKeys.length" class="floating-toolbar">
              <t-button variant="outline" size="small" @click="handleBatchEdit">
                <template #icon><t-icon name="edit-1" size="14px" /></template>
                编辑
              </t-button>
              <t-button variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="handleReExtract">
                <template #icon><t-icon name="refresh" size="14px" /></template>
                重新提取
              </t-button>
              <t-button variant="outline" size="small" @click="handleBatchPrint">
                <template #icon><t-icon name="print" size="14px" /></template>
                打印
              </t-button>
              <t-button variant="outline" size="small" @click="handleBatchDelete">
                <template #icon><t-icon name="delete" size="14px" /></template>
                删除
              </t-button>
            </div>
          </div>
        </t-tab-panel>
        <t-tab-panel value="water" label="水费">
          <UtilityMeterTab category="water" />
        </t-tab-panel>
        <t-tab-panel value="gas" label="气费">
          <UtilityMeterTab category="gas" />
        </t-tab-panel>
      </t-tabs>
    </div>

    <!-- 详情抽屉 -->
    <t-drawer :visible="detailVisible" :header="detailTitle" :size="String(drawerWidth)" :footer="false"
      :close-on-overlay-click="true" destroy-on-close @close="closeDetail">
      <div class="utility-detail-drawer">
        <!-- 摘要 -->
        <section class="detail-block">
          <div class="detail-block-title" @click="summaryExpanded = !summaryExpanded">
            <span>摘要</span>
            <t-icon :name="summaryExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="detail-block-caret" />
          </div>
          <div v-if="summaryExpanded" class="detail-block-content">
            <div class="summary-lines">{{ summaryLines || '暂无摘要' }}</div>
          </div>
        </section>

        <!-- 字段编辑 -->
        <section class="detail-block">
          <div class="detail-block-title">账单字段</div>
          <div class="detail-block-content">
            <div class="field-grid">
              <div v-for="cfg in detailFieldDefs" :key="cfg.field_key" class="field-grid-item">
                <label class="field-label">{{ cfg.label }}</label>
                <t-input v-if="cfg.fieldType === 'text'" :model-value="fieldValue(cfg)" size="small"
                  @update:model-value="(v: string) => setFieldValue(cfg, v)" />
                <t-date-picker v-else-if="cfg.fieldType === 'date'" :model-value="fieldValue(cfg)" size="small"
                  format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable
                  @change="(v: any) => setFieldValue(cfg, v)" />
                <t-input v-else :model-value="fieldValue(cfg)" size="small" type="number"
                  @update:model-value="(v: string) => setFieldValue(cfg, v)" />
              </div>
            </div>
            <div class="auto-save-tip">字段修改后将自动保存{{ autoSaving ? '（保存中...）' : '' }}</div>
          </div>
        </section>

        <!-- 费用明细（可编辑，总账联动，可折叠） -->
        <section class="detail-block">
          <div class="detail-block-title" @click="feeExpanded = !feeExpanded">
            <span>费用明细</span>
            <span class="fee-count">{{ editForm.fee_items?.length || 0 }} 项</span>
            <t-icon :name="feeExpanded ? 'chevron-up' : 'chevron-down'" size="14px" class="detail-block-caret" />
          </div>
          <div v-if="feeExpanded" class="detail-block-content">
            <div class="fee-table">
              <div class="fee-row fee-head">
                <span>类别</span><span>费用组成</span><span>时段</span><span>电量</span><span>标准</span><span>电费</span><span></span>
              </div>
              <div v-for="(it, i) in editForm.fee_items" :key="i" class="fee-row">
                <t-input v-model="it.category" size="small" placeholder="类别" />
                <t-input v-model="it.name" size="small" placeholder="费用组成" />
                <t-input v-model="it.period" size="small" placeholder="时段" />
                <t-input v-model="it.qty" size="small" type="number" />
                <t-input v-model="it.rate" size="small" type="number" />
                <t-input v-model="it.fee" size="small" type="number" />
                <t-button variant="text" size="small" shape="square" @click="removeFeeItem(i)">
                  <template #icon><t-icon name="delete" size="14px" /></template>
                </t-button>
              </div>
            </div>
            <div class="fee-actions">
              <t-button variant="text" size="small" @click="addFeeItem">
                <template #icon><t-icon name="add" size="14px" /></template>
                添加明细
              </t-button>
              <span class="fee-total">合计 {{ fmtMoney(feeTotal) }} 元</span>
            </div>
            <div class="auto-save-tip">明细编辑后自动保存，总账按明细重算</div>
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

    <!-- 标签编辑 -->
    <TagEditDialog v-model:visible="tagDialogVisible" :knowledge-name="tagTargetName" :kb-id="kbId"
      :tag-list="tagList" :selected-tags="tagTargetTags" :can-manage="true" @confirm="onTagEditConfirm"
      @tag-created="onTagCreated" @open-manage="openTagManage" />

    <!-- 标签管理 -->
    <KbTagManageDrawer v-if="kbId" v-model:visible="tagManageVisible" :kb-id="kbId" :is-faq="false"
      @changed="onTagManageChanged" />

    <!-- 删除历史 -->
    <DeletedKnowledgeDrawer v-if="kbId" v-model:visible="historyVisible" :kb-id="kbId" :module-name="'电费'"
      @changed="loadFiles(true)" />

    <!-- 设置（字段配置 + 包含判定） -->
    <UtilitySettingsDrawer v-if="kbId" v-model:visible="settingsVisible" :kb-id="kbId" category="electricity"
      @changed="onSettingsChanged" />

    <!-- 初始化向导 -->
    <UtilitiesKbWizard v-model:visible="wizardVisible" @created="onKbCreated" />

    <!-- 打印预览弹窗 -->
    <teleport to="body">
      <div v-if="printVisible" class="utility-print-mask">
        <div class="utility-print-dialog">
          <div class="utility-print-header">
            <span class="utility-print-title">打印预览（{{ printCount }} 份）</span>
            <t-button variant="text" size="small" class="utility-print-close" @click="closePrint">
              <template #icon><t-icon name="close" size="16px" /></template>
            </t-button>
          </div>
          <div class="utility-print-body">
            <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
            <div v-else class="print-preview-loading">
              <t-loading size="small" text="正在合并账单 PDF…" />
            </div>
          </div>
          <div class="utility-print-footer">
            <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
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
import {
  listKnowledgeBases,
  listKnowledgeFiles,
  uploadKnowledgeFile,
  getKnowledgeDetails,
  updateKnowledgeMetadata,
  listKnowledgeTags,
  updateKnowledgeTagBatch,
  extractUtilityBill,
  listUtilityBillRecords,
  listUtilityFieldConfigs,
  previewKnowledgeFile,
  reparseKnowledge,
} from '@/api/knowledge-base'
import DocumentPreview from '@/components/document-preview.vue'
import TagEditDialog from '@/views/knowledge/components/TagEditDialog.vue'
import KbTagManageDrawer from '@/views/knowledge/components/KbTagManageDrawer.vue'
import UtilitiesKbWizard from './UtilitiesKbWizard.vue'
import UtilityMeterTab from './UtilityMeterTab.vue'
import UtilitySettingsDrawer from './UtilitySettingsDrawer.vue'
import DeletedKnowledgeDrawer from './DeletedKnowledgeDrawer.vue'

const KB_NAME = '日常事务-电费'
const PAGE_SIZE = 30
const COLUMN_STORAGE_KEY = 'weknora-utility-electricity-columns-v1'

interface ColDef { key: string; label: string; fieldType: string; default: boolean; w: string }
const FALLBACK_COLUMNS: ColDef[] = [
  { key: 'bill_period_start', label: '账单周期起', fieldType: 'date', default: true, w: '1.2fr' },
  { key: 'account_no', label: '户号', fieldType: 'text', default: true, w: '1fr' },
  { key: 'account_name', label: '户名', fieldType: 'text', default: true, w: '1.4fr' },
  { key: 'usage_category', label: '用电类别', fieldType: 'text', default: true, w: '1fr' },
  { key: 'total_kwh', label: '本期电量', fieldType: 'number', default: true, w: '0.9fr' },
  { key: 'total_amount', label: '本期电费', fieldType: 'amount', default: true, w: '1fr' },
  { key: 'avg_price', label: '平均电价', fieldType: 'number', default: true, w: '0.9fr' },
  { key: 'power_factor', label: '功率因数', fieldType: 'number', default: true, w: '0.8fr' },
  { key: 'bill_period_end', label: '账单周期止', fieldType: 'date', default: false, w: '1.2fr' },
  { key: 'voltage_level', label: '电压等级', fieldType: 'text', default: false, w: '0.9fr' },
  { key: 'supply_unit', label: '供电服务单位', fieldType: 'text', default: false, w: '1.2fr' },
  { key: 'mom_change', label: '环比', fieldType: 'text', default: false, w: '0.8fr' },
  { key: 'due_date', label: '交费截止', fieldType: 'date', default: false, w: '1.1fr' },
  { key: 'industrial_amount', label: '工商业电费', fieldType: 'amount', default: false, w: '1fr' },
  { key: 'residential_amount', label: '居民电费', fieldType: 'amount', default: false, w: '1fr' },
  { key: 'pf_adjust_amount', label: '功率因数调整电费', fieldType: 'amount', default: false, w: '1.2fr' },
  { key: 'grand_total', label: '合计', fieldType: 'amount', default: false, w: '1fr' },
  { key: 'address', label: '用电地址', fieldType: 'text', default: false, w: '1.6fr' },
  { key: 'market_attr', label: '市场化属性', fieldType: 'text', default: false, w: '1fr' },
  { key: 'print_date', label: '账单打印日期', fieldType: 'date', default: false, w: '1.2fr' },
  { key: 'prev_kwh', label: '上期电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'deep_peak_kwh', label: '尖峰电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'peak_kwh', label: '峰电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'flat_kwh', label: '平电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'valley_kwh', label: '谷电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'reactive_kwh', label: '正向无功电量', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'capacity', label: '容量', fieldType: 'number', default: false, w: '0.8fr' },
  { key: 'capacity_price', label: '容量电价', fieldType: 'number', default: false, w: '0.9fr' },
  { key: 'capacity_fee', label: '输配容量电费', fieldType: 'amount', default: false, w: '1fr' },
  { key: 'demand', label: '需量值', fieldType: 'number', default: false, w: '0.8fr' },
  { key: 'pf_standard', label: '功率因数标准', fieldType: 'number', default: false, w: '1fr' },
  { key: 'adjust_ratio', label: '调整系数', fieldType: 'number', default: false, w: '0.9fr' },
]

const activeTab = ref<'electricity' | 'water' | 'gas'>('electricity')
const kbId = ref('')
const loading = ref(true)
const wizardVisible = ref(false)
const fileInputRef = ref<HTMLInputElement>()

// 字段配置（来自后端 /utilities/field-configs?category=electricity，动态加载）
const columnDefs = ref<ColDef[]>(FALLBACK_COLUMNS)
const visibleColKeys = ref<string[]>(loadStoredColumns())
const fieldPopupVisible = ref(false)
const visibleColDefs = computed(() => columnDefs.value.filter(c => visibleColKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')} 1fr 1.2fr`,
}))

function loadStoredColumns(): string[] {
  try {
    const raw = localStorage.getItem(COLUMN_STORAGE_KEY)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) return arr.filter((k: string) => columnDefs.value.some(c => c.key === k))
    }
  } catch { /* ignore */ }
  return columnDefs.value.filter(c => c.default).map(c => c.key)
}
function selectAllColumns() { visibleColKeys.value = columnDefs.value.map(c => c.key) }
function resetColumns() { visibleColKeys.value = columnDefs.value.filter(c => c.default).map(c => c.key) }
function persistColumns() {
  try { localStorage.setItem(COLUMN_STORAGE_KEY, JSON.stringify(visibleColKeys.value)) } catch { /* ignore */ }
}
watch(visibleColKeys, () => persistColumns(), { deep: true })

const loadFieldConfigs = async () => {
  try {
    const res: any = await listUtilityFieldConfigs('electricity')
    const list = res?.data || res
    if (Array.isArray(list) && list.length) {
      columnDefs.value = list.map((c: any) => ({
        key: c.field_key,
        label: c.label,
        fieldType: c.field_type || 'text',
        default: !!c.default_visible,
        w: colWidth(c.field_key),
      }))
      // 默认列变更后重算可见列（保留用户已存储的偏好，仅当存储为空时使用新默认）
      visibleColKeys.value = loadStoredColumns()
    }
  } catch { /* 字段配置加载失败用内置默认 */ }
}
function colWidth(key: string): string {
  if (['account_name', 'address', 'supply_unit'].includes(key)) return '1.6fr'
  if (['bill_period_start', 'bill_period_end', 'due_date', 'print_date', 'pf_adjust_amount'].includes(key)) return '1.2fr'
  return '1fr'
}

// ---- 列表 ----
interface Row {
  rowKey: string
  knowledgeId: string
  fileName: string
  fileType?: string
  extractStatus: string
  extractError?: string
  kind: string
  item: Record<string, any>
  tags?: any[]
  page?: number
}
const rows = ref<Row[]>([])
const pendingFiles = ref<any[]>([])
const summary = ref<{ total: number; sumKwh: number; sumAmount: number }>({ total: 0, sumKwh: 0, sumAmount: 0 })
const listLoading = ref(false)
const loadingMore = ref(false)
const page = ref(1)
const hasMore = ref(true)
const keyword = ref('')
const dateRange = ref<Array<string>>([])
const selectedRowKeys = ref<string[]>([])
const extractInFlight = ref<Set<string>>(new Set())
const extractFailed = ref<Set<string>>(new Set())
const listScrollRef = ref<HTMLElement>()

const summaryUsage = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.total_kwh) || 0), 0)
})
const summaryAmount = computed(() => {
  const target = selectedRowKeys.value.length ? selectedRows.value : rows.value
  return target.reduce((s, r) => s + (Number(r.item?.total_amount) || 0), 0)
})
const selectedRows = computed(() => rows.value.filter(r => selectedRowKeys.value.includes(r.rowKey)))
const selectableRows = computed(() => rows.value.filter(r => r.kind !== 'pending'))
const isAllSelected = computed(() => selectableRows.value.length > 0 && selectableRows.value.every(r => selectedRowKeys.value.includes(r.rowKey)))
const someSelected = computed(() => selectedRowKeys.value.length > 0 && !isAllSelected.value)

function toggleSelectAll(checked: boolean) {
  selectedRowKeys.value = checked ? selectableRows.value.map(r => r.rowKey) : []
}
function toggleRow(key: string, checked: boolean) {
  if (checked) { if (!selectedRowKeys.value.includes(key)) selectedRowKeys.value.push(key) }
  else selectedRowKeys.value = selectedRowKeys.value.filter(k => k !== key)
}

const mapRow = (r: any): Row => ({
  rowKey: r.row_key || `${r.knowledge_id}-${r.page || 0}`,
  knowledgeId: r.knowledge_id || '',
  fileName: r.file_name || r.knowledge_title || '',
  fileType: r.file_type || '',
  extractStatus: r.extract_status || '',
  extractError: r.extract_error || '',
  kind: r.extract_status === 'manual' ? 'manual' : 'bill',
  item: r.item || {},
  tags: r.tags || [],
  page: r.page,
})

const loadFiles = async (reset = false) => {
  if (!kbId.value) return
  if (reset) {
    page.value = 1
    rows.value = []
    hasMore.value = true
    listLoading.value = true
  } else if (listLoading.value || loadingMore.value) return
  else loadingMore.value = true
  try {
    const res: any = await listUtilityBillRecords(kbId.value, {
      q: keyword.value || undefined,
      date_from: dateRange.value?.[0] || undefined,
      date_to: dateRange.value?.[1] || undefined,
      page: page.value,
      page_size: PAGE_SIZE,
    })
    const data = res?.data || res?.list || []
    const arr = Array.isArray(data) ? data : []
    const total = Number(res?.total || arr.length || 0)
    summary.value = { total, sumKwh: 0, sumAmount: 0 }
    const existing = new Set(rows.value.map(r => r.rowKey))
    const fresh = arr.map(mapRow).filter(r => !existing.has(r.rowKey))
    rows.value = [...rows.value, ...fresh]
    hasMore.value = rows.value.length < total
    if (hasMore.value) page.value += 1
  } catch (e: any) {
    MessagePlugin.error(e?.message || '列表加载失败')
  } finally {
    listLoading.value = false
    loadingMore.value = false
  }
}

const onListScroll = () => {
  const el = listScrollRef.value
  if (!el || !hasMore.value || loadingMore.value) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 80) loadFiles()
}

// ---- 状态行 ----
const STATUS_MAP: Record<string, { label: string; theme: any; icon?: string; spin?: boolean }> = {
  parsing: { label: '解析中', theme: 'warning', icon: 'loading', spin: true },
  extracting: { label: '提取中', theme: 'warning', icon: 'loading', spin: true },
  pending: { label: '待提取', theme: 'default' },
  success: { label: '提取完成', theme: 'success', icon: 'check-circle' },
  manual: { label: '待补录', theme: 'warning', icon: 'edit-1' },
  failed: { label: '提取失败', theme: 'danger', icon: 'error-circle' },
}
const statusOf = (row: Row) => {
  const st = row.extractStatus || ''
  if (STATUS_MAP[st]) return STATUS_MAP[st]
  return { label: '--', theme: 'default' as const }
}
const pendingLabel = (pf: any) => {
  const ps = pf.parse_status || ''
  if (ps === 'parsing' || ps === 'pending') return '解析中'
  const meta = pf.custom_metadata || {}
  if (meta.extract_status === 'extracting' || pf.extract_status === 'extracting') return '提取中'
  return '待提取'
}

// 轮询：解析完成后自动触发提取；提取中动态刷新状态
let pollTimer: ReturnType<typeof setInterval> | null = null
let extractingSet = new Set<string>()
const startPolling = () => {
  stopPolling()
  pollTimer = setInterval(async () => {
    if (!kbId.value) return
    try {
      const res: any = await listKnowledgeFiles(kbId.value, { page: 1, page_size: 100 })
      const data = res?.data || res?.list || []
      const arr = Array.isArray(data) ? data : []
      // 进行中文件（解析中/提取中/待提取）
      pendingFiles.value = arr.filter((it: any) => {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        return ps === 'parsing' || ps === 'pending' ||
          meta.extract_status === 'extracting' || it.extract_status === 'extracting' ||
          (meta.extract_status == null && ps === 'completed' && !meta.kind)
      })
      // 已解析完成但尚未提取的文件自动触发提取
      for (const it of arr) {
        const ps = it.parse_status
        const meta = it.custom_metadata || {}
        if (ps === 'completed' && !meta.kind && !meta.extract_status && !extractingSet.has(it.id)) {
          extractingSet.add(it.id)
          try {
            await extractUtilityBill(kbId.value, it.id)
            loadFiles(true)
          } catch { /* 失败下轮重试 */ } finally {
            extractingSet.delete(it.id)
          }
        }
      }
    } catch { /* 轮询失败静默 */ }
  }, 4000)
}
const stopPolling = () => {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
}

// ---- KB ----
const loadKb = async () => {
  try {
    const res: any = await listKnowledgeBases()
    const list = res?.data || res?.list || []
    const found = Array.isArray(list) ? list.find((kb: any) => kb.name === KB_NAME) : null
    kbId.value = found?.id || ''
    if (kbId.value) {
      loadDrawerWidth()
      await loadFieldConfigs()
      await loadTags()
      await loadFiles(true)
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
    await loadFieldConfigs()
    await loadTags()
    await loadFiles(true)
    startPolling()
  }
}
const onSettingsChanged = () => {
  loadFieldConfigs()
  loadFiles(true)
}

// ---- 上传 ----
const triggerUpload = () => fileInputRef.value?.click()
const onFileInputChange = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  if (!files.length || !kbId.value) return
  for (const f of files) {
    try {
      const res: any = await uploadKnowledgeFile(kbId.value, f)
      const knowledge = res?.data || res
      const kid = knowledge?.id || knowledge?.knowledge_id
      MessagePlugin.success(`已上传 ${f.name}，等待解析`)
      if (kid) {
        // 解析完成后轮询自动提取
        extractingSet.add(kid)
      }
    } catch (e2: any) {
      MessagePlugin.error(`${f.name} 上传失败：${e2?.message || ''}`)
    }
  }
  setTimeout(() => loadFiles(true), 1500)
}

// ---- 标签 ----
const tagList = ref<any[]>([])
const tagDialogVisible = ref(false)
const tagTarget = ref<Row | null>(null)
const tagManageVisible = ref(false)
const tagTargetName = computed(() => tagTarget.value?.fileName || '')
const loadTags = async () => {
  if (!kbId.value) return
  try {
    const res: any = await listKnowledgeTags(kbId.value, { page: 1, page_size: 100 })
    const pageData = (res?.data || {}) as { data?: any[]; total?: number }
    tagList.value = (pageData.data || []).map((tag: any) => ({ ...tag, id: String(tag.id) }))
  } catch { /* ignore */ }
}
const rowTags = (row: Row) => {
  const arr = row.tags || []
  return Array.isArray(arr) ? arr : []
}
const openTagEdit = (row: Row) => {
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
const onTagManageChanged = () => { loadTags(); loadFiles(true) }

// ---- 详情抽屉 ----
const detailVisible = ref(false)
const currentRow = ref<Row | null>(null)
const editForm = ref<Record<string, any>>({})
const detailFieldDefs = computed(() => columnDefs.value)
const summaryExpanded = ref(true)
const feeExpanded = ref(false)
const autoSaving = ref(false)
let autoSaveTimer: ReturnType<typeof setTimeout> | null = null
let autoSaveDirty = false
let editFormSnapshot = ''
let feeItemsSnapshot = ''

const detailTitle = computed(() => currentRow.value?.fileName || '账单详情')

const fieldValue = (cfg: ColDef) => editForm.value?.[cfg.key] ?? ''
const setFieldValue = (cfg: ColDef, v: any) => {
  if (v === undefined || v === null) return
  editForm.value[cfg.key] = v
}

const summaryLines = computed(() => {
  const d = currentRow.value?.item?.remark || ''
  return d ? d.replace(/-(?=[^\s-])/g, '\n-').trim() : ''
})

const openDetail = async (row: Row) => {
  currentRow.value = row
  detailVisible.value = true
  summaryExpanded.value = true
  feeExpanded.value = false
  autoSaveDirty = false
  try {
    const res: any = await getKnowledgeDetails(row.knowledgeId)
    const detail = res?.data || res
    const meta = detail?.custom_metadata || {}
    const records = Array.isArray(meta.records) ? meta.records : []
    const idx = Math.max(0, (row.page || 1) - 1)
    const item = records[idx] || row.item || {}
    editForm.value = { ...item }
    editFormSnapshot = JSON.stringify(editForm.value)
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  } catch {
    editForm.value = { ...row.item }
    editFormSnapshot = JSON.stringify(editForm.value)
    feeItemsSnapshot = JSON.stringify(editForm.value.fee_items || [])
    autoSaveDirty = true
  }
}

const closeDetail = () => {
  detailVisible.value = false
  currentRow.value = null
}

const onFieldEdited = () => { /* 数值字段 change 即进入自动保存流程 */ }

watch(editForm, () => {
  if (!autoSaveDirty || !currentRow.value) return
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
    const idx = Math.max(0, (now.page || 1) - 1)
    const updated: Record<string, any> = {}
    columnDefs.value.forEach(c => {
      const v = editForm.value[c.key]
      if (v !== undefined) updated[c.key] = v
    })
    updated.fee_items = Array.isArray(editForm.value.fee_items) ? editForm.value.fee_items : []
    updated.remark = editForm.value.remark || ''
    if (records[idx]) records[idx] = { ...records[idx], ...updated }
    else records.push({ ...updated })
    // 总账按明细重算：仅当费用明细实际变化时
    const feeChanged = JSON.stringify(updated.fee_items) !== feeItemsSnapshot
    const feeSum = updated.fee_items.reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0)
    const hasItem = updated.fee_items.length > 0
    const nextMeta = { ...meta, kind: 'utility_bill', records, extract_status: 'success', extract_error: '' }
    if (feeChanged && hasItem) {
      nextMeta.records[idx].total_amount = Math.round(feeSum * 100) / 100
      nextMeta.records[idx].grand_total = Math.round(feeSum * 100) / 100
      editForm.value.total_amount = nextMeta.records[idx].total_amount
      editForm.value.grand_total = nextMeta.records[idx].grand_total
      MessagePlugin.info('总账已按明细重算')
    }
    feeItemsSnapshot = JSON.stringify(updated.fee_items)
    await updateKnowledgeMetadata(now.knowledgeId, nextMeta)
    editFormSnapshot = JSON.stringify(editForm.value)
    // 同步列表行
    const listIdx = rows.value.findIndex((r: any) => r.rowKey === now.rowKey)
    if (listIdx >= 0) rows.value[listIdx] = { ...rows.value[listIdx], item: { ...rows.value[listIdx].item, ...updated } }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    autoSaving.value = false
  }
}

// 费用明细操作
const feeTotal = computed(() =>
  (editForm.value.fee_items || []).reduce((s: number, it: any) => s + (Number(it.fee) || 0), 0))
const addFeeItem = () => {
  if (!Array.isArray(editForm.value.fee_items)) editForm.value.fee_items = []
  editForm.value.fee_items.push({ category: '', name: '', period: '', qty: 0, rate: 0, fee: 0 })
}
const removeFeeItem = (i: number) => {
  editForm.value.fee_items.splice(i, 1)
}

// ---- 抽屉宽度拖动 ----
const DRAWER_WIDTH_KEY = 'weknora-utility-drawer-width'
const DRAWER_DEFAULT_WIDTH = 720
const DRAWER_MIN_WIDTH = 560
const drawerWidth = ref(DRAWER_DEFAULT_WIDTH)
const drawerResizing = ref(false)
let drawerResizeStartX = 0
let drawerResizeStartWidth = 0
function loadDrawerWidth() {
  try {
    const v = Number(localStorage.getItem(DRAWER_WIDTH_KEY))
    drawerWidth.value = v >= DRAWER_MIN_WIDTH ? v : DRAWER_DEFAULT_WIDTH
  } catch { /* ignore */ }
}

// ---- 浮动工具栏 ----
const handleBatchEdit = () => {
  const r = selectedRows.value
  if (r.length !== 1) { MessagePlugin.info('请选中单行后编辑，或点击列表中的账单行进入编辑'); return }
  openDetail(r[0])
}
const handleReExtract = async () => {
  const row = selectedRows.value
  if (row.length !== 1) { MessagePlugin.info('重新提取仅支持单选'); return }
  extractInFlight.value.add(row[0].knowledgeId)
  extractFailed.value.delete(row[0].knowledgeId)
  try {
    await reparseKnowledge(row[0].knowledgeId)
    MessagePlugin.success(`已触发「${row[0].fileName}」重新解析与提取`)
    setTimeout(() => loadFiles(true), 1500)
  } catch (e: any) {
    extractFailed.value.add(row[0].knowledgeId)
    MessagePlugin.error(e?.message || '重新提取失败')
  } finally {
    extractInFlight.value.delete(row[0].knowledgeId)
  }
}

// ---- 打印（合并多份账单为一个 PDF，iframe 预览） ----
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'tif', 'tiff']
const isImageRow = (r: any, blob: any) => {
  const ext = String(r.fileType || '').toLowerCase().replace(/^\./, '').split('/').pop() || ''
  if (IMAGE_EXTS.includes(ext)) return true
  const mime = (blob?.type || '').toLowerCase()
  return mime.startsWith('image/')
}
const printVisible = ref(false)
const printBusy = ref(false)
const printUrl = ref('')
const printLoaded = ref(false)
const printCount = ref(0)

const handleBatchPrint = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  printBusy.value = true
  printUrl.value = ''
  printLoaded.value = false
  printCount.value = rowsSel.length
  printVisible.value = true
  try {
    const out = await PDFDocument.create()
    const notes: string[] = []
    let mergedPages = 0
    for (const r of rowsSel) {
      const blob: any = await previewKnowledgeFile(r.knowledgeId)
      if (!blob) { notes.push(`${r.fileName}（获取源文件失败）`); continue }
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
        const pages = await out.copyPages(pdf, pdf.getPageIndices())
        pages.forEach(p => out.addPage(p))
        mergedPages += pages.length
      } catch {
        notes.push(`${r.fileName}（该文件无法合并，可单独打印）`)
      }
    }
    if (mergedPages === 0) {
      MessagePlugin.error('未获取到可打印的账单页面')
      printVisible.value = false
      return
    }
    const bytes = await out.save()
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    if (notes.length) setTimeout(() => MessagePlugin.warning(notes.join('；')), 300)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印预览生成失败')
    printVisible.value = false
  } finally {
    printBusy.value = false
  }
}
const doPrint = () => {
  const frame = document.querySelector('.print-preview-frame') as HTMLIFrameElement | null
  if (frame?.contentWindow) {
    try { frame.contentWindow.print(); return } catch { /* fallback */ }
  }
  window.print()
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 删除 ----
const handleBatchDelete = async () => {
  const rowsSel = selectedRows.value
  if (!rowsSel.length) return
  const confirm = await MessagePlugin.confirm(`确认删除选中的 ${rowsSel.length} 条账单记录？源文件将移入删除历史。`, {
    confirmBtn: '删除', cancelBtn: '取消',
  })
  if (!confirm) return
  try {
    for (const r of rowsSel) {
      // 删除记录 = 删除知识文件（软删进删除历史）
      const { delKnowledgeDetails } = await import('@/api/knowledge-base')
      await delKnowledgeDetails(r.knowledgeId)
    }
    MessagePlugin.success('已删除')
    selectedRowKeys.value = []
    await loadFiles(true)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 筛选 ----
const applyFilter = () => loadFiles(true)

// ---- 格式化 ----
const fmtKwh = (v: any) => {
  const n = Number(v) || 0
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
const cellText = (row: Row, key: string): string => {
  const v = row.item?.[key]
  if (v === null || v === undefined || v === '') return ''
  if (typeof v === 'number') {
    if (key.includes('price') || key === 'avg_price' || key === 'power_factor' || key === 'adjust_ratio') {
      return String(Math.round(v * 10000) / 10000)
    }
    return Number.isInteger(v) ? String(v) : String(Math.round(v * 100) / 100)
  }
  return String(v)
}

// ---- Tab 切换 ----
const onTabChange = () => { /* 子组件自行加载 */ }

onMounted(() => { loadKb() })
onBeforeUnmount(() => { stopPolling() })
</script>

<style lang="less" scoped>
.utilities-management-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 16px 24px;
  box-sizing: border-box;
}

.header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 12px;

  .header-title {
    h2 {
      margin: 0 0 4px;
      font-size: 20px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .header-subtitle {
      margin: 0;
      font-size: 13px;
      color: var(--td-text-color-secondary);
    }
  }

  .header-actions {
    display: flex;
    gap: 8px;
  }
}

.loading-area {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16px;
}

.utilities-main {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.utilities-tabs {
  flex: 1;
  min-height: 0;

  :deep(.t-tabs__content) {
    flex: 1;
    min-height: 0;
  }
}

.electricity-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 8px;
}

.utility-summary-row {
  display: flex;
  gap: 24px;
  align-items: center;
  padding: 8px 16px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);

  &.with-toolbar {
    padding-bottom: 48px;
  }

  .summary-selected {
    color: var(--td-brand-color);
  }
}

.doc-list-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.doc-pending-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 16px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container-hover);

  .pending-name {
    font-size: 13px;
    color: var(--td-text-color-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.list-loading {
  padding: 24px;
  display: flex;
  justify-content: center;
}

.list-empty {
  padding: 40px 0;
}

.floating-toolbar {
  position: absolute;
  right: 24px;
  bottom: 24px;
  display: flex;
  gap: 8px;
  padding: 6px 8px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, .12);
  z-index: 20;
}

.utility-detail-drawer {
  padding: 4px 0 24px;

  .detail-block {
    margin-bottom: 20px;

    .detail-block-title {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      padding: 10px 12px;
      border: 1px solid var(--td-component-stroke);
      border-radius: 6px 6px 0 0;
      background: var(--td-bg-color-container);
      cursor: pointer;

      .detail-block-caret {
        margin-left: auto;
        color: var(--td-text-color-secondary);
      }

      .fee-count {
        font-size: 12px;
        font-weight: 400;
        color: var(--td-text-color-secondary);
      }
    }

    .detail-block-content {
      padding: 12px;
      border: 1px solid var(--td-component-stroke);
      border-top: none;
      border-radius: 0 0 6px 6px;
      background: var(--td-bg-color-container);
    }
  }
}

.field-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 16px;

  .field-grid-item {
    display: flex;
    flex-direction: column;
    gap: 4px;

    .field-label {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}

.auto-save-tip {
  margin-top: 10px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.summary-lines {
  font-size: 13px;
  color: var(--td-text-color-primary);
  line-height: 1.6;
  white-space: pre-line;
  word-break: break-all;
}

.fee-table {
  .fee-row {
    display: grid;
    grid-template-columns: 1.2fr 1.6fr 0.9fr 0.8fr 0.8fr 1fr auto;
    gap: 6px;
    align-items: center;
    margin-bottom: 6px;

    &.fee-head {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      padding: 0 0 4px;
      border-bottom: 1px solid var(--td-component-stroke);
    }
  }
}

.fee-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 8px;

  .fee-total {
    font-size: 13px;
    color: var(--td-text-color-primary);
  }
}

/* 打印弹窗 */
.utility-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, .45);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
}

.utility-print-dialog {
  width: min(100%, 1100px);
  height: min(92vh, 860px);
  background: var(--td-bg-color-container);
  border-radius: 8px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.utility-print-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--td-component-stroke);

  .utility-print-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
}

.utility-print-body {
  flex: 1;
  min-height: 0;
  background: var(--td-bg-color-container);

  .print-preview-frame {
    width: 100%;
    height: 100%;
    border: none;
    display: block;
  }

  .print-preview-loading {
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
  }
}

.utility-print-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--td-component-stroke);
}

.icon-spin {
  animation: t-spin 1s linear infinite;
}

@keyframes t-spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
