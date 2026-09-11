<template>
  <div class="tenant-billing-container">
    <!-- 筛选工具栏（与电费/合同管理一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-select v-model="activeTenantId" placeholder="租户" filterable class="doc-filter-select doc-filter-field__control"
            :options="tenantOptions" @change="onTenantChange" />
        </div>
        <div class="doc-filter-field">
          <t-date-picker v-model="filters.month" mode="month" placeholder="账单月份" format="YYYY-MM" value-type="YYYY-MM"
            clearable class="doc-date-picker doc-filter-field__control" @change="loadRecords" />
        </div>
        <t-button variant="outline" size="small" @click="loadRecords">
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
                <t-checkbox v-for="col in COLUMN_DEFS" :key="col.key" :value="col.key" class="field-popup-item">
                  {{ col.label }}
                </t-checkbox>
              </t-checkbox-group>
            </div>
          </template>
        </t-popup>
        <t-button variant="outline" size="small" @click="openSettings">
          <template #icon><t-icon name="setting" size="14px" /></template>
          设置
        </t-button>
        <t-button theme="primary" size="small" @click="openReadingDrawer('')">
          <template #icon><t-icon name="add" /></template>
          新增记录
        </t-button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="doc-list-scroll tenant-list-scroll">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <div class="cell cell-check" role="columnheader" @click.stop>
            <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
              :disabled="!displayRows.length" title="全选" @change="toggleSelectAll" />
          </div>
          <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
            {{ col.label }}
          </div>
        </div>
        <div class="doc-list-body">
          <div v-for="row in displayRows" :key="row.id" class="doc-list-row"
            :class="{ 'row-selected': selectedKeys.has(row.id) }" :style="gridStyle" role="row" @click="onRowClick(row)">
            <div class="cell cell-check" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(row.id)" @change="(v: any) => toggleSelect(row, v)" />
            </div>
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`">
              <span v-if="col.key === 'month'" class="row-mono">{{ row.month }}</span>
              <span v-else-if="col.key === 'ratio'" class="row-mono">{{ fmtRatio(row.ratio) }}</span>
              <span v-else-if="col.key === 'total_fee'" class="row-mono strong">{{ fmtMoney(row.total_fee) }}</span>
              <span v-else-if="['total_kwh', 'line_loss'].includes(col.key)" class="row-mono">{{ fmtKwh(row[col.key]) }}</span>
              <span v-else-if="['industrial_fee', 'dorm_fee', 'water_fee', 'bill_total_amount'].includes(col.key)" class="row-mono">{{ fmtMoney(row[col.key]) }}</span>
              <span v-else class="row-text" :title="String(row[col.key] ?? '')">{{ row[col.key] || '' }}</span>
            </div>
          </div>
          <div v-if="!loading && !displayRows.length" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部汇总 -->
    <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
      <span class="doc-summary-item">星达电量 <span class="doc-summary-val">{{ fmtKwh(summaryKwh) }}</span> 千瓦时</span>
      <span class="doc-summary-item">总应付 <span class="doc-summary-val">{{ fmtMoney(summaryFee) }}</span> 元</span>
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
            <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="openEditSelected">
              <template #icon><t-icon name="edit" size="14px" /></template>
              编辑
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>
              打印
            </t-button>
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedKeys.size} 条账单吗？`"
              :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
              @confirm="handleDelete">
              <t-button theme="danger" variant="outline" size="small" @click.stop>
                <template #icon><t-icon name="delete" size="14px" /></template>
                删除
              </t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </transition>

    <!-- ================= 新增记录抽屉（分时读数） ================= -->
    <teleport to="body">
      <div v-if="readingVisible" class="doc-drawer-resize-handle" :style="{ right: readingWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onReadingResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="readingVisible" :visible="true" :header="readingTitle" :size="readingWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="tenant-reading-drawer"
        @close="readingVisible = false" @update:visible="(v: boolean) => (readingVisible = v)">
        <div class="reading-body">
          <div class="rec-field reading-month-field">
            <label>月份 <span class="required">*</span></label>
            <t-date-picker v-model="readingMonth" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份"
              @change="loadReadings" />
          </div>

          <div v-for="group in readingGroups" :key="group.type" class="reading-group">
            <div class="reading-group-title">{{ group.title }}</div>
            <div class="reading-meters">
              <div v-for="m in group.meters" :key="m.id" class="reading-card">
                <div class="reading-card-head">
                  <span>{{ m.name }}</span>
                  <span class="reading-card-rate">倍率 {{ m.rate }}</span>
                </div>
                <div class="reading-grid">
                  <div v-for="p in periods" :key="p.key" class="reading-cell">
                    <div class="reading-cell-label">{{ p.label }}</div>
                    <div class="reading-cell-inputs">
                      <t-input-number :model-value="rdVal(m.id, p.key + '_prev')" size="small" theme="normal" placeholder="起度"
                        @update:model-value="(v: number | string) => setRdVal(m.id, p.key + '_prev', v)" />
                      <t-input-number :model-value="rdVal(m.id, p.key + '_curr')" size="small" theme="normal" placeholder="止度"
                        @update:model-value="(v: number | string) => setRdVal(m.id, p.key + '_curr', v)" />
                    </div>
                  </div>
                </div>
                <div class="reading-card-total">电量 {{ meterKwh(m, readingForm[m.id] || {}) }}</div>
              </div>
              <div v-if="!group.meters.length" class="meter-empty">暂无{{ group.title }}，请先在设置中新增</div>
            </div>
          </div>

          <div v-if="periodTotals" class="reading-summary">
            <span>星达合计 {{ fmtKwh(periodTotals.star) }} 千瓦时</span>
            <span>工业合计 {{ fmtKwh(periodTotals.sub) }} 千瓦时</span>
            <span>线损 {{ fmtKwh(periodTotals.loss) }} 千瓦时</span>
          </div>
          <p class="field-hint">线损 = 星达分表合计 − 工业分表合计，按工业分表分时占比自动分摊</p>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="readingVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="savingReadings" @click="saveReadingsAndGenerate">保存</t-button>
        </div>
      </t-drawer>
    </teleport>

    <!-- ================= 设置抽屉（租户管理 / 电表设置 / 分摊子项） ================= -->
    <teleport to="body">
      <div v-if="settingsVisible" class="doc-drawer-resize-handle" :style="{ right: settingsWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onSettingsResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="settingsVisible" :visible="true" :size="settingsWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="tenant-settings-drawer"
        @close="settingsVisible = false" @update:visible="(v: boolean) => (settingsVisible = v)">
        <template #header>
          <div class="settings-header">
            <span class="settings-header-title">设置</span>
            <div class="settings-header-actions">
              <t-select v-model="activeTenantId" placeholder="租户" filterable class="settings-tenant-select"
                :options="tenantOptions" @change="onTenantChange" />
              <t-button variant="outline" size="small" @click="openCreateTenant">
                <template #icon><t-icon name="add" /></template>新增租户
              </t-button>
            </div>
          </div>
        </template>
        <t-tabs v-model="settingsTab" class="settings-tabs">
          <t-tab-panel value="tenant" label="租户信息">
            <div class="settings-panel">
              <div class="rec-grid">
                <div class="rec-field">
                  <label>租户名称</label>
                  <t-input v-model="tenantForm.name" size="small" />
                </div>
                <div class="rec-field">
                  <label>备注</label>
                  <t-input v-model="tenantForm.remark" size="small" />
                </div>
                <div class="rec-field">
                  <label>宿舍电价（元/度）</label>
                  <t-input-number v-model="settingForm.dorm_price" size="small" :min="0" :step="0.1" theme="column" />
                </div>
                <div class="rec-field">
                  <label>水价（元/吨）</label>
                  <t-input-number v-model="settingForm.water_price" size="small" :min="0" :step="0.01" theme="column" />
                </div>
                <div class="rec-field rec-field--wide">
                  <label>市电账单知识库</label>
                  <t-select v-model="settingForm.bill_kb_id" size="small" clearable filterable placeholder="选择市电账单知识库">
                    <t-option v-for="kb in kbList" :key="kb.id" :value="kb.id" :label="kb.name" />
                  </t-select>
                </div>
              </div>
              <p class="field-hint">表计引用按使用单位自动匹配：宿舍电表与水表取「使用单位 = 租户名称」的表计</p>
              <div class="panel-actions">
                <t-button theme="primary" size="small" :loading="savingBase" @click="saveTenantBase">保存</t-button>
              </div>
            </div>
          </t-tab-panel>
          <t-tab-panel value="meters" label="电表设置">
            <div class="settings-panel">
              <div class="meter-section">
                <div class="meter-section-head">
                  <span class="meter-section-title">星达分表</span>
                  <t-button variant="outline" size="small" @click="openAddMeter('star')">
                    <template #icon><t-icon name="add" /></template>新增
                  </t-button>
                </div>
                <div class="meter-grid">
                  <div v-for="m in starMeters" :key="m.id" class="meter-card" :class="{ disabled: !m.enabled }">
                    <div class="meter-card-head">
                      <span class="meter-card-name">{{ m.name }}</span>
                      <t-switch size="small" :model-value="!!m.enabled" @change="(v: boolean) => toggleMeter(m, v)" />
                    </div>
                    <div class="meter-card-meta">
                      <span>倍率 {{ m.rate }}</span>
                      <span class="meter-card-ops">
                        <t-icon name="edit-1" class="op" @click="openEditMeter(m)" />
                        <t-icon name="delete" class="op danger" @click="removeMeter(m)" />
                      </span>
                    </div>
                  </div>
                  <div v-if="!starMeters.length" class="meter-empty">暂无星达分表</div>
                </div>
              </div>
              <div class="meter-section">
                <div class="meter-section-head">
                  <span class="meter-section-title">工业分表</span>
                  <t-button variant="outline" size="small" @click="openAddMeter('sub')">
                    <template #icon><t-icon name="add" /></template>新增
                  </t-button>
                </div>
                <div class="meter-grid">
                  <div v-for="m in subMeters" :key="m.id" class="meter-card" :class="{ disabled: !m.enabled }">
                    <div class="meter-card-head">
                      <span class="meter-card-name">{{ m.name }}</span>
                      <t-switch size="small" :model-value="!!m.enabled" @change="(v: boolean) => toggleMeter(m, v)" />
                    </div>
                    <div class="meter-card-meta">
                      <span>倍率 {{ m.rate }}</span>
                      <span class="meter-card-ops">
                        <t-icon name="edit-1" class="op" @click="openEditMeter(m)" />
                        <t-icon name="delete" class="op danger" @click="removeMeter(m)" />
                      </span>
                    </div>
                  </div>
                  <div v-if="!subMeters.length" class="meter-empty">暂无工业分表</div>
                </div>
              </div>
            </div>
          </t-tab-panel>
          <t-tab-panel value="items" label="分摊子项">
            <div class="settings-panel">
              <div class="item-list">
                <div class="item-row item-row--head">
                  <span class="item-name">子项名称</span>
                  <span class="item-op">分摊</span>
                </div>
                <div v-for="(it, i) in itemsForm" :key="i" class="item-row">
                  <span class="item-name" :title="it.item_name">{{ it.item_name }}</span>
                  <span class="item-op">
                    <t-switch size="small" :model-value="!!it.enabled" @change="(v: boolean) => toggleItem(it, v)" />
                    <t-icon name="delete" class="op danger" @click="removeItem(it)" />
                  </span>
                </div>
                <div v-if="!itemsForm.length" class="item-empty">暂无子项，生成账单后自动从市电账单引入</div>
              </div>
              <div class="item-actions">
                <t-button variant="outline" size="small" @click="addItemRow">
                  <template #icon><t-icon name="add" /></template>新增
                </t-button>
                <span class="field-hint">关闭的子项不参与分摊；居民/目录类默认关闭</span>
              </div>
            </div>
          </t-tab-panel>
        </t-tabs>
      </t-drawer>
    </teleport>

    <!-- 新增/编辑分时电表弹窗 -->
    <t-dialog :visible="meterVisible" header="分时电表" width="420px" :footer="false" @close="meterVisible = false">
      <div class="form-grid">
        <div class="form-item">
          <label>表名称</label>
          <t-input v-model="meterForm.name" size="small" placeholder="如：星达总表1 / 分表1" />
        </div>
        <div class="form-item">
          <label>倍率</label>
          <t-input-number v-model="meterForm.rate" size="small" :min="1" theme="column" />
        </div>
      </div>
      <div class="dialog-actions">
        <t-button variant="outline" size="small" @click="meterVisible = false">取消</t-button>
        <t-button theme="primary" size="small" :loading="savingMeter" @click="submitMeter">保存</t-button>
      </div>
    </t-dialog>

    <!-- 新增租户弹窗 -->
    <t-dialog v-model:visible="createVisible" header="新增租户" width="420px" :footer="false" @close="createVisible = false">
      <div class="form-grid">
        <div class="form-item">
          <label>租户名称</label>
          <t-input v-model="createForm.name" size="small" placeholder="如：持睿汽车" />
        </div>
        <div class="form-item">
          <label>宿舍电价（元/度）</label>
          <t-input-number v-model="createForm.dorm_price" size="small" :min="0" :step="0.1" theme="column" />
        </div>
        <div class="form-item">
          <label>水价（元/吨）</label>
          <t-input-number v-model="createForm.water_price" size="small" :min="0" :step="0.01" theme="column" />
        </div>
      </div>
      <div class="dialog-actions">
        <t-button variant="outline" size="small" @click="createVisible = false">取消</t-button>
        <t-button theme="primary" size="small" :loading="creating" @click="submitCreate">创建</t-button>
      </div>
    </t-dialog>

    <!-- 账单详情抽屉 -->
    <teleport to="body">
      <div v-if="recordVisible" class="doc-drawer-resize-handle" :style="{ right: recordWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onRecordResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="recordVisible" :visible="true" :header="`${recordDetail?.record?.month || ''} 账单明细`" :size="recordWidth"
        :footer="false" :close-on-overlay-click="true" destroy-on-close class="tenant-record-drawer"
        @close="recordVisible = false" @update:visible="(v: boolean) => (recordVisible = v)">
        <div v-if="recordDetail" class="record-detail">
          <div class="record-overview">
            <div class="ro-item">
              <span class="ro-label">星达电量</span>
              <span class="ro-value row-mono">{{ fmtKwh(recordDetail.record.total_kwh) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">线损</span>
              <span class="ro-value row-mono">{{ fmtKwh(recordDetail.record.line_loss) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">分摊比例</span>
              <span class="ro-value row-mono">{{ fmtRatio(recordDetail.record.ratio) }}</span>
            </div>
            <div class="ro-item">
              <span class="ro-label">市电本期电费</span>
              <span class="ro-value row-mono">{{ fmtMoney(recordDetail.record.bill_total_amount) }}</span>
            </div>
          </div>
          <div class="record-items">
            <div class="record-items-head">
              <span>项目</span>
              <span>时段</span>
              <span>电量</span>
              <span>单价</span>
              <span>费用</span>
            </div>
            <div v-for="(it, i) in recordDetail.items" :key="i" class="record-items-row" :class="{ 'os-neg': Number(it.fee) < 0 }">
              <span>{{ it.name }}</span>
              <span>{{ it.period || '—' }}</span>
              <span class="row-mono">{{ fmtKwh(it.qty) }}</span>
              <span class="row-mono">{{ fmtRate(it.rate) }}</span>
              <span class="row-mono">{{ fmtMoney(it.fee) }}</span>
            </div>
            <div class="record-items-row record-items-total">
              <span>合计</span>
              <span></span>
              <span></span>
              <span></span>
              <span class="row-mono">{{ fmtMoney(recordDetail.record.total_fee) }}</span>
            </div>
          </div>
        </div>
        <template #footer>
          <t-button variant="outline" size="small" @click="recordVisible = false">关闭</t-button>
          <t-button theme="primary" size="small" :loading="printBusy" @click="printRecord">打印</t-button>
        </template>
      </t-drawer>
    </teleport>

    <!-- 打印预览 -->
    <div v-if="printVisible" class="tenant-print-mask">
      <div class="tenant-print-dialog">
        <div class="tenant-print-header">
          <span class="tenant-print-title">{{ printTitle }} 打印预览</span>
          <div class="tenant-print-actions">
            <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
            <t-button theme="primary" size="small" @click="doBrowserPrint">打印</t-button>
          </div>
        </div>
        <div class="tenant-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading"><t-loading size="small" /></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listBillingTenants, createBillingTenant, getBillingTenant, updateBillingTenant,
  listBillingTimeMeters, createBillingTimeMeter, updateBillingTimeMeter, deleteBillingTimeMeter,
  getBillingTimeReading, saveBillingTimeReading,
  saveBillingTenantItems,
  listBillingRecords, generateBillingRecord, getBillingRecord, deleteBillingRecord,
} from '@/api/knowledge-base'
import { listKnowledgeBases } from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

// ---- 列定义 ----
interface ColDef { key: string; label: string; default: boolean; w: string }
const COLUMN_DEFS: ColDef[] = [
  { key: 'month', label: '账单周期', default: true, w: '0.8fr' },
  { key: 'total_kwh', label: '星达电量', default: true, w: '1fr' },
  { key: 'line_loss', label: '线损', default: true, w: '0.9fr' },
  { key: 'ratio', label: '分摊比例', default: true, w: '0.9fr' },
  { key: 'industrial_fee', label: '工业电费', default: true, w: '1fr' },
  { key: 'dorm_fee', label: '宿舍电费', default: true, w: '1fr' },
  { key: 'water_fee', label: '水费', default: true, w: '0.9fr' },
  { key: 'total_fee', label: '总应付', default: true, w: '1.1fr' },
  { key: 'bill_total_amount', label: '市电电费', default: false, w: '1fr' },
  { key: 'remark', label: '备注', default: false, w: '1.5fr' },
]
const STORAGE_KEY = 'weknora-tenant-billing-columns-v1'
const visibleKeys = ref<string[]>(loadStoredKeys())
const visibleColDefs = computed(() => COLUMN_DEFS.filter(c => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))
function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        const valid = arr.filter(k => COLUMN_DEFS.some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return COLUMN_DEFS.filter(c => c.default).map(c => c.key)
}
const fieldPopupVisible = ref(false)
const selectAllColumns = () => { visibleKeys.value = COLUMN_DEFS.map(c => c.key); persistColumns() }
const resetColumns = () => { visibleKeys.value = COLUMN_DEFS.filter(c => c.default).map(c => c.key); persistColumns() }
const persistColumns = () => { try { localStorage.setItem(STORAGE_KEY, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ } }

// ---- 租户与账单 ----
const loading = ref(false)
const tenants = ref<any[]>([])
const activeTenantId = ref('')
const tenantOptions = computed(() => tenants.value.map(t => ({ label: t.name, value: t.id })))
const filters = ref<{ month?: string }>({ month: undefined })
const records = ref<any[]>([])
const displayRows = ref<any[]>([])

const loadTenants = async () => {
  loading.value = true
  try {
    const res: any = await listBillingTenants()
    tenants.value = res.data || []
    if (tenants.value.length && !activeTenantId.value) {
      activeTenantId.value = tenants.value[0].id
    }
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户失败')
  } finally {
    loading.value = false
  }
}

const loadRecords = async () => {
  if (!activeTenantId.value) return
  try {
    const res: any = await listBillingRecords(activeTenantId.value)
    records.value = res.data || []
    let list = records.value
    if (filters.value.month) list = list.filter(r => r.month === filters.value.month)
    displayRows.value = list
    clearSelection()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

const onTenantChange = async () => {
  clearSelection()
  await loadRecords()
}

// ---- 汇总 ----
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => records.value.filter(r => selectedKeys.value.has(r.id)))
const summary = computed(() => ({ total: (selectedKeys.value.size ? selectedRows.value : displayRows.value).length }))
const summaryKwh = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_kwh) || 0), 0) * 100) / 100
})
const summaryFee = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.total_fee) || 0), 0) * 100) / 100
})
const toggleSelect = (row: any, checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) next.add(row.id)
  else next.delete(row.id)
  selectedKeys.value = next
}
const isAllSelected = computed(() => displayRows.value.length > 0 && displayRows.value.every(r => selectedKeys.value.has(r.id)))
const someSelected = computed(() => displayRows.value.some(r => selectedKeys.value.has(r.id)) && !isAllSelected.value)
const toggleSelectAll = (checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) displayRows.value.forEach(r => next.add(r.id))
  else displayRows.value.forEach(r => next.delete(r.id))
  selectedKeys.value = next
}
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.id])
  openRecord(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }
const openEditSelected = () => {
  const r = selectedRows.value[0]
  if (r) openReadingDrawer(r.month)
}

// ---- 浮动工具栏：打印目录 / 删除 ----
const catalogBusy = ref(false)
const handlePrint = async () => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  if (!arr.length) return
  catalogBusy.value = true
  try {
    const columns: CatalogColumn[] = visibleColDefs.value.map(col => ({
      key: col.key,
      label: col.label,
      value: (r: any) => cellText(col.key, r),
    }))
    const bytes = await generateCatalogPdf({
      title: `租户月度账单目录`,
      columns,
      rows: arr,
    })
    showPrint(`${arr[0].month} 账单目录（${arr.length} 条）`, bytes)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印生成失败')
  } finally {
    catalogBusy.value = false
  }
}

const handleDelete = async () => {
  const arr = selectedRows.value
  if (!arr.length) return
  try {
    for (const r of arr) await deleteBillingRecord(r.id)
    MessagePlugin.success('已删除')
    clearSelection()
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 新增记录抽屉（分时读数） ----
const readingVisible = ref(false)
const readingWidth = ref('760px')
const savingReadings = ref(false)
const readingMonth = ref('')
const readingTitle = computed(() => (readingMonth.value ? `${readingMonth.value} 分时读数` : '新增记录'))
const starMeters = ref<any[]>([])
const subMeters = ref<any[]>([])
const readingForm = ref<Record<string, any>>({})
const periods = [
  { key: 'deep', label: '尖' },
  { key: 'peak', label: '峰' },
  { key: 'flat', label: '平' },
  { key: 'valley', label: '谷' },
]
const readingGroups = computed(() => [
  { type: 'star', title: '星达分表', meters: starMeters.value.filter((m: any) => m.enabled) },
  { type: 'sub', title: '工业分表', meters: subMeters.value.filter((m: any) => m.enabled) },
])

const openReadingDrawer = async (month: string) => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  readingMonth.value = month || currentMonth()
  readingVisible.value = true
  await loadMetersForReading()
  await loadReadings()
}

const currentMonth = (): string => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const loadMetersForReading = async () => {
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    const d = res.data
    starMeters.value = (d.meters || []).filter((m: any) => m.meter_type === 'star')
    subMeters.value = (d.meters || []).filter((m: any) => m.meter_type === 'sub')
  } catch { /* ignore */ }
}

const loadReadings = async () => {
  if (!readingMonth.value || !readingVisible.value) return
  const meters = [...starMeters.value, ...subMeters.value].filter((m: any) => m.enabled)
  const form: Record<string, any> = {}
  for (const m of meters) {
    form[m.id] = { deep_prev: 0, deep_curr: 0, peak_prev: 0, peak_curr: 0, flat_prev: 0, flat_curr: 0, valley_prev: 0, valley_curr: 0 }
    try {
      const res: any = await getBillingTimeReading(m.id, readingMonth.value)
      if (res.data) {
        Object.assign(form[m.id], {
          deep_prev: res.data.deep_prev, deep_curr: res.data.deep_curr,
          peak_prev: res.data.peak_prev, peak_curr: res.data.peak_curr,
          flat_prev: res.data.flat_prev, flat_curr: res.data.flat_curr,
          valley_prev: res.data.valley_prev, valley_curr: res.data.valley_curr,
        })
      }
    } catch { /* ignore */ }
  }
  readingForm.value = form
}

const meterKwh = (m: any, f: any): string => {
  const k = (p: string) => Math.max(0, (Number(f[p + '_curr']) || 0) - (Number(f[p + '_prev']) || 0)) * (m.rate || 1)
  const total = periods.reduce((s, p) => s + k(p.key), 0)
  return fmtKwh(total)
}

const rdVal = (id: string, key: string): number => {
  const f = readingForm.value[id]
  return f ? Number(f[key]) || 0 : 0
}
const setRdVal = (id: string, key: string, v: number | string) => {
  if (!readingForm.value[id]) readingForm.value[id] = {}
  readingForm.value[id][key] = Number(v) || 0
}

const periodTotals = computed(() => {
  const sum = (meters: any[]) => {
    const t: Record<string, number> = { deep: 0, peak: 0, flat: 0, valley: 0 }
    meters.forEach((m) => {
      const f = readingForm.value[m.id] || {}
      periods.forEach((p) => {
        t[p.key] += Math.max(0, (Number(f[p.key + '_curr']) || 0) - (Number(f[p.key + '_prev']) || 0)) * (m.rate || 1)
      })
    })
    return t
  }
  const star = sum(starMeters.value.filter((m: any) => m.enabled))
  const sub = sum(subMeters.value.filter((m: any) => m.enabled))
  const st = star.deep + star.peak + star.flat + star.valley
  const sb = sub.deep + sub.peak + sub.flat + sub.valley
  if (st === 0 && sb === 0) return null
  return { star: st, sub: sb, loss: st - sb }
})

const saveReadingsAndGenerate = async () => {
  if (!readingMonth.value) {
    MessagePlugin.warning('请选择月份')
    return
  }
  savingReadings.value = true
  try {
    const meters = [...starMeters.value, ...subMeters.value].filter((m: any) => m.enabled)
    if (!meters.length) {
      MessagePlugin.warning('请先在设置中新增分时电表')
      return
    }
    for (const m of meters) {
      const f = readingForm.value[m.id] || {}
      // 止度不得小于起度
      for (const p of periods) {
        const prev = Number(f[p.key + '_prev']) || 0
        const curr = Number(f[p.key + '_curr']) || 0
        if (curr < prev) {
          MessagePlugin.warning(`${m.name} ${p.label} 时段止度不得小于起度`)
          return
        }
      }
    }
    for (const m of meters) {
      const f = readingForm.value[m.id] || {}
      await saveBillingTimeReading(m.id, { month: readingMonth.value, ...f })
    }
    MessagePlugin.success('读数已保存')
    // 自动生成当月账单
    try {
      await generateBillingRecord(activeTenantId.value, { month: readingMonth.value })
      MessagePlugin.success('账单已生成')
    } catch (e: any) {
      MessagePlugin.warning(e?.message || '账单生成失败，请检查分表读数与市电账单')
    }
    readingVisible.value = false
    await loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingReadings.value = false
  }
}

// ---- 设置抽屉 ----
const settingsVisible = ref(false)
const settingsWidth = ref('680px')
const settingsTab = ref('tenant')
const kbList = ref<any[]>([])
const detail = ref<any>(null)
const tenantForm = ref({ name: '', remark: '' })
const settingForm = ref({ dorm_price: 1, water_price: 5.22, bill_kb_id: '' })
const savingBase = ref(false)
const itemsForm = ref<{ item_key: string; item_name: string; enabled: boolean }[]>([])

const openSettings = async () => {
  if (!activeTenantId.value) {
    MessagePlugin.warning('请先选择租户')
    return
  }
  settingsVisible.value = true
  settingsTab.value = 'tenant'
  await loadDetail()
}

const loadDetail = async () => {
  if (!activeTenantId.value) return
  try {
    const res: any = await getBillingTenant(activeTenantId.value)
    detail.value = res.data
    const d = res.data
    tenantForm.value = { name: d.tenant?.name || '', remark: d.tenant?.remark || '' }
    settingForm.value = {
      dorm_price: d.setting?.dorm_price || 1,
      water_price: d.setting?.water_price || 5.22,
      bill_kb_id: d.setting?.bill_kb_id || '',
    }
    starMeters.value = (d.meters || []).filter((m: any) => m.meter_type === 'star')
    subMeters.value = (d.meters || []).filter((m: any) => m.meter_type === 'sub')
    itemsForm.value = (d.items || []).map((it: any) => ({ item_key: it.item_key, item_name: it.item_name, enabled: it.enabled }))
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载租户详情失败')
  }
}

const saveTenantBase = async () => {
  if (!activeTenantId.value) return
  savingBase.value = true
  try {
    await updateBillingTenant(activeTenantId.value, {
      name: tenantForm.value.name,
      remark: tenantForm.value.remark,
      dorm_price: settingForm.value.dorm_price,
      water_price: settingForm.value.water_price,
      bill_kb_id: settingForm.value.bill_kb_id,
    })
    MessagePlugin.success('已保存')
    await loadTenants()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingBase.value = false
  }
}

// ---- 分摊子项 ----
const toggleItem = async (it: any, v: boolean) => {
  it.enabled = !!v
  await saveItems()
}
const removeItem = async (it: any) => {
  itemsForm.value = itemsForm.value.filter(x => x !== it)
  await saveItems()
}
const addItemRow = () => {
  itemsForm.value.push({ item_key: '', item_name: '自定义子项', enabled: true })
  saveItems()
}
const saveItems = async () => {
  if (!activeTenantId.value) return
  try {
    const items = itemsForm.value
      .filter((it: any) => it.item_name.trim())
      .map((it: any) => ({ item_key: it.item_name.trim(), item_name: it.item_name.trim(), enabled: !!it.enabled }))
    await saveBillingTenantItems(activeTenantId.value, { items })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存子项失败')
  }
}

// ---- 分时电表 CRUD ----
const meterVisible = ref(false)
const savingMeter = ref(false)
const meterForm = ref({ id: '', name: '', rate: 1, meter_type: 'star' })

const openAddMeter = (type: 'star' | 'sub') => {
  meterForm.value = { id: '', name: '', rate: 1, meter_type: type }
  meterVisible.value = true
}
const openEditMeter = (m: any) => {
  meterForm.value = { id: m.id, name: m.name, rate: m.rate, meter_type: m.meter_type }
  meterVisible.value = true
}
const submitMeter = async () => {
  if (!meterForm.value.name.trim()) {
    MessagePlugin.warning('请输入表名称')
    return
  }
  savingMeter.value = true
  try {
    if (meterForm.value.id) {
      await updateBillingTimeMeter(meterForm.value.id, { name: meterForm.value.name, rate: meterForm.value.rate })
    } else {
      await createBillingTimeMeter(activeTenantId.value, {
        meter_type: meterForm.value.meter_type, name: meterForm.value.name, rate: meterForm.value.rate,
      })
    }
    meterVisible.value = false
    MessagePlugin.success('已保存')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}
const toggleMeter = async (m: any, v: boolean) => {
  try {
    await updateBillingTimeMeter(m.id, { enabled: v })
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}
const removeMeter = async (m: any) => {
  try {
    await deleteBillingTimeMeter(m.id)
    MessagePlugin.success('已删除')
    await loadDetail()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 租户创建 ----
const createVisible = ref(false)
const creating = ref(false)
const createForm = ref({ name: '', dorm_price: 1, water_price: 5.22 })

const openCreateTenant = () => {
  createForm.value = { name: '', dorm_price: 1, water_price: 5.22 }
  createVisible.value = true
}
const submitCreate = async () => {
  if (!createForm.value.name.trim()) {
    MessagePlugin.warning('请输入租户名称')
    return
  }
  creating.value = true
  try {
    const res: any = await createBillingTenant({
      name: createForm.value.name.trim(),
      dorm_price: createForm.value.dorm_price,
      water_price: createForm.value.water_price,
    })
    createVisible.value = false
    MessagePlugin.success('已创建')
    activeTenantId.value = res.data.id
    await loadTenants()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建失败')
  } finally {
    creating.value = false
  }
}

// ---- 账单详情与打印 ----
const recordVisible = ref(false)
const recordWidth = ref('760px')
const recordDetail = ref<any>(null)
const printVisible = ref(false)
const printUrl = ref('')
const printBusy = ref(false)
const printTitle = ref('')

const openRecord = async (r: any) => {
  try {
    const res: any = await getBillingRecord(r.id)
    recordDetail.value = res.data
    recordVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载账单失败')
  }
}

const printRecord = async () => {
  if (!recordDetail.value) return
  printBusy.value = true
  try {
    const rec = recordDetail.value.record
    const items = recordDetail.value.items || []
    const columns: CatalogColumn[] = [
      { key: 'name', label: '项目', value: (r: any) => r.name },
      { key: 'period', label: '时段', value: (r: any) => r.period || '—' },
      { key: 'qty', label: '电量', value: (r: any) => fmtKwh(r.qty) },
      { key: 'rate', label: '单价', value: (r: any) => fmtRate(r.rate) },
      { key: 'fee', label: '费用（元）', value: (r: any) => fmtMoney(r.fee) },
    ]
    const totalRow = { name: '合计', period: '', qty: 0, rate: 0, fee: rec.total_fee }
    const bytes = await generateCatalogPdf({
      title: `${rec.month} 租户电费账单`,
      columns,
      rows: [...items, totalRow],
    })
    showPrint(`${rec.month} 租户电费账单`, bytes)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '打印生成失败')
  } finally {
    printBusy.value = false
  }
}

const showPrint = (title: string, bytes: ArrayBuffer) => {
  printTitle.value = title
  if (printUrl.value) URL.revokeObjectURL(printUrl.value)
  printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
  printVisible.value = true
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}
const doBrowserPrint = () => {
  const frame = document.querySelector('.tenant-print-body iframe') as HTMLIFrameElement
  if (frame?.contentWindow) frame.contentWindow.print()
}

// ---- 抽屉拖动调宽 ----
const onResize = (e: MouseEvent, widthRef: { value: string }) => {
  const startX = e.clientX
  const startW = parseFloat(widthRef.value)
  const move = (ev: MouseEvent) => {
    const w = Math.min(1100, Math.max(520, startW + (startX - ev.clientX)))
    widthRef.value = `${w}px`
  }
  const up = () => {
    document.removeEventListener('mousemove', move)
    document.removeEventListener('mouseup', up)
  }
  document.addEventListener('mousemove', move)
  document.addEventListener('mouseup', up)
}
const onReadingResizeStart = (e: MouseEvent) => onResize(e, readingWidth)
const onSettingsResizeStart = (e: MouseEvent) => onResize(e, settingsWidth)
const onRecordResizeStart = (e: MouseEvent) => onResize(e, recordWidth)

// ---- 知识库列表 ----
const loadKbs = async () => {
  try {
    const res: any = await listKnowledgeBases()
    kbList.value = (res.data?.list || res.data || []).map((k: any) => ({ id: k.id, name: k.name }))
  } catch { /* ignore */ }
}

// ---- 格式化 ----
const fmtMoney = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
const fmtKwh = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
const fmtRate = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n) || n === 0) return '—'
  return String(parseFloat(n.toFixed(6)))
}
const fmtRatio = (v: any): string => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return '—'
  return (n * 100).toFixed(2) + '%'
}
const cellText = (key: string, row: any): string => {
  if (key === 'ratio') return fmtRatio(row.ratio)
  if (['total_kwh', 'line_loss'].includes(key)) return fmtKwh(row[key])
  if (['industrial_fee', 'dorm_fee', 'water_fee', 'bill_total_amount', 'total_fee'].includes(key)) return fmtMoney(row[key])
  return row[key] || ''
}

onMounted(() => {
  loadTenants()
  loadKbs()
})
</script>

<style lang="less" scoped>
.tenant-billing-container {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
  padding-top: 4px;
}

/* 筛选工具栏（与电费/合同管理一致） */
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
    .doc-date-picker { width: 140px; }
    .doc-filter-select { width: 160px; }
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

/* 列表（与电费/合同管理一致） */
.doc-list-view { width: 100%; min-width: 100%; box-sizing: border-box; }
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
  &:hover { background: var(--td-bg-color-secondarycontainer); }
  &.row-selected { background: var(--td-brand-color-light); }
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
  &:first-child { padding-left: 0; }
  &:last-child { padding-right: 0; }
}
.cell-check {
  justify-content: center;
  position: sticky;
  left: 0;
  z-index: 2;
  background: transparent;
  padding: 0;
}
.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }
.row-mono,
.row-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-mono { font-family: var(--app-font-family); }
.os-neg { color: var(--td-error-color, #d54941); }
.strong { font-weight: 600; }

.tenant-list-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
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
  .meter-empty-icon { color: var(--td-text-color-placeholder); }
  .meter-empty-text { font-size: 13px; color: var(--td-text-color-placeholder); }
}

/* 底部汇总 */
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

/* 浮动工具栏 */
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
    .batch-bar-count { font-size: 13px; color: var(--td-text-color-primary); }
    .batch-bar-clear { color: var(--td-brand-color); }
  }
  .batch-bar-actions { display: flex; align-items: center; gap: 8px; }
}
.batch-bar-fade-enter-active,
.batch-bar-fade-leave-active { transition: opacity 0.18s ease, transform 0.18s ease; }
.batch-bar-fade-enter-from,
.batch-bar-fade-leave-to { opacity: 0; transform: translateX(-50%) translateY(6px); }

/* 读数抽屉 */
.reading-body { padding: 4px 0 24px; }
.reading-month-field { margin-bottom: 16px; }
.reading-group {
  .reading-group-title { font-size: 13px; font-weight: 600; margin: 4px 0 10px; }
  .reading-meters {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
    margin-bottom: 16px;
    .reading-card {
      border: 1px solid var(--td-component-border);
      border-radius: 8px;
      padding: 12px;
      .reading-card-head {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 10px;
        font-size: 13px;
        font-weight: 500;
        .reading-card-rate { font-size: 12px; color: var(--td-text-color-secondary); font-weight: 400; }
      }
      .reading-grid {
        display: grid;
        grid-template-columns: repeat(2, 1fr);
        gap: 10px;
        .reading-cell {
          .reading-cell-label { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 4px; }
          .reading-cell-inputs { display: flex; gap: 6px; }
        }
      }
      .reading-card-total {
        margin-top: 10px;
        font-size: 12px;
        color: var(--td-text-color-secondary);
        text-align: right;
      }
    }
    .meter-empty {
      grid-column: span 2;
      padding: 24px;
      text-align: center;
      color: var(--td-text-color-placeholder);
      font-size: 12px;
      border: 1px dashed var(--td-component-border);
      border-radius: 8px;
    }
  }
}
.reading-summary {
  display: flex;
  gap: 24px;
  font-size: 13px;
  color: var(--td-text-color-primary);
  padding: 10px 0 0;
}
.field-hint { font-size: 12px; color: var(--td-text-color-secondary); margin-top: 4px; line-height: 1.4; }
.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 设置抽屉 */
.settings-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
  .settings-header-title { font-size: 16px; font-weight: 600; }
  .settings-header-actions { display: flex; align-items: center; gap: 8px; }
  .settings-tenant-select { width: 160px; }
}
.settings-tabs { height: 100%; }
.settings-panel { padding: 4px 0 24px; }
.panel-actions { display: flex; justify-content: flex-end; margin-top: 16px; }
.rec-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px 20px;
}
.rec-field {
  min-width: 0;
  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;
  }
  .required { color: var(--td-error-color); }
}
.rec-field--wide { grid-column: 1 / -1; }

.meter-section {
  margin-bottom: 20px;
  .meter-section-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
    .meter-section-title { font-size: 13px; font-weight: 600; }
  }
}
.meter-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  .meter-card {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    padding: 12px;
    transition: all 0.2s;
    &:hover { border-color: var(--td-brand-color); box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06); }
    &.disabled { opacity: 0.55; }
    .meter-card-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      .meter-card-name { font-size: 13px; font-weight: 500; }
    }
    .meter-card-meta {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-top: 8px;
      font-size: 12px;
      color: var(--td-text-color-secondary);
      .meter-card-ops { display: flex; gap: 8px; }
    }
  }
  .meter-empty {
    grid-column: span 2;
    padding: 24px;
    text-align: center;
    color: var(--td-text-color-placeholder);
    font-size: 12px;
    border: 1px dashed var(--td-component-border);
    border-radius: 8px;
  }
}

.item-list {
  border: 1px solid var(--td-component-border);
  border-radius: 8px;
  overflow: hidden;
  .item-row {
    display: grid;
    grid-template-columns: 1fr 80px;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-bottom: 1px solid var(--td-component-stroke);
    &:last-child { border-bottom: none; }
    &.item-row--head {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      background: var(--td-bg-color-secondarycontainer);
    }
    .item-name { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .item-op { display: flex; align-items: center; justify-content: flex-end; gap: 12px; }
  }
  .item-empty { padding: 24px; text-align: center; color: var(--td-text-color-placeholder); font-size: 12px; }
}
.item-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-top: 12px;
}

.op { cursor: pointer; color: var(--td-text-color-secondary); transition: color 0.2s; &:hover { color: var(--td-brand-color); } &.danger:hover { color: var(--td-error-color); } }
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
  .form-item {
    display: flex;
    flex-direction: column;
    gap: 6px;
    label { font-size: 12px; color: var(--td-text-color-secondary); }
    &.form-item--wide { grid-column: span 2; }
  }
}
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }

/* 详情抽屉 */
.record-detail {
  .record-overview {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    margin-bottom: 16px;
    .ro-item {
      border: 1px solid var(--td-component-border);
      border-radius: 8px;
      padding: 12px;
      display: flex;
      flex-direction: column;
      gap: 6px;
      .ro-label { font-size: 12px; color: var(--td-text-color-secondary); }
      .ro-value { font-size: 15px; font-weight: 600; color: var(--td-text-color-primary); }
    }
  }
  .record-items {
    border: 1px solid var(--td-component-border);
    border-radius: 8px;
    overflow: hidden;
    .record-items-head, .record-items-row {
      display: grid;
      grid-template-columns: 1.6fr 0.7fr 1fr 1fr 1.1fr;
      gap: 8px;
      padding: 9px 12px;
      font-size: 13px;
    }
    .record-items-head {
      background: var(--td-bg-color-secondarycontainer);
      color: var(--td-text-color-secondary);
      font-size: 12px;
    }
    .record-items-row {
      border-bottom: 1px solid var(--td-component-stroke);
      &:last-child { border-bottom: none; }
      &.record-items-total { background: var(--td-brand-color-light); font-weight: 600; }
    }
  }
}

/* 打印预览 */
.tenant-print-mask {
  position: fixed;
  inset: 0;
  z-index: 3000;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  .tenant-print-dialog {
    width: min(960px, 92vw);
    height: min(720px, 88vh);
    background: var(--td-bg-color-container);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    .tenant-print-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 10px 16px;
      border-bottom: 1px solid var(--td-component-stroke);
      .tenant-print-title { font-size: 14px; font-weight: 600; }
      .tenant-print-actions { display: flex; gap: 8px; }
    }
    .tenant-print-body {
      flex: 1;
      min-height: 0;
      .print-preview-frame { width: 100%; height: 100%; border: none; }
      .print-preview-loading { height: 100%; display: flex; align-items: center; justify-content: center; }
    }
  }
}
</style>
