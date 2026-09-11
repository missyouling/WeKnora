<template>
  <div class="utility-meter-tab">
    <!-- 筛选工具栏（与电费/合同管理一致） -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-date-picker v-model="filters.month" mode="month" placeholder="月份" format="YYYY-MM" value-type="YYYY-MM" clearable
            class="doc-date-picker doc-filter-field__control" @change="load" />
        </div>
        <div class="doc-filter-field">
          <t-select v-model="filters.meterId" :placeholder="meterLabel" clearable filterable class="doc-filter-select doc-filter-field__control"
            :options="meterFilterOptions" @change="load" />
        </div>
        <t-button variant="outline" size="small" @click="load">
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
        <t-button theme="primary" size="small" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          新增记录
        </t-button>
      </div>
    </div>

    <!-- 列表（自绘 grid，数据少时只包裹记录行，超出滚动） -->
    <div class="doc-list-scroll meter-list-scroll" ref="listScrollRef">
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
          <div v-for="row in displayRows" :key="row.item_id || row.key" class="doc-list-row"
            :class="{ 'row-selected': selectedKeys.has(row.key) }" :style="gridStyle" role="row" @click="onRowClick(row)">
            <div class="cell cell-check" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(row.key)" @change="(v: any) => toggleSelect(row, v)" />
            </div>
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`">
              <span v-if="col.key === 'month'" class="row-mono">{{ row.month }}</span>
              <span v-else-if="col.key === 'meter'" class="row-text" :title="row.meter_alias">{{ row.meter_alias }}</span>
              <span v-else-if="col.key === 'start_reading'" class="row-mono">{{ fmtNum(row.start_reading) }}</span>
              <span v-else-if="col.key === 'end_reading'" class="row-mono">{{ fmtNum(row.end_reading) }}</span>
              <span v-else-if="col.key === 'rate'" class="row-mono">{{ fmtNum(row.rate) }}</span>
              <span v-else-if="col.key === 'usage'" class="row-mono">{{ fmtNum(row.usage) }}</span>
              <span v-else-if="col.key === 'unit_price'" class="row-mono">{{ fmtNum(row.unit_price) }}</span>
              <span v-else-if="col.key === 'subsidy'" class="row-mono" :class="{ 'os-neg': Number(row.subsidy) < 0 }">{{ fmtMoney(row.subsidy) }}</span>
              <span v-else-if="col.key === 'amount'" class="row-mono">{{ fmtMoney(row.amount) }}</span>
              <span v-else-if="col.key === 'reading_date'" class="row-mono">{{ row.reading_date || '' }}</span>
              <span v-else-if="col.key === 'reader'" class="row-text" :title="String(row.reader ?? '')">{{ row.reader || '' }}</span>
              <span v-else-if="col.key === 'meter_no'" class="row-mono" :title="String(row.meter_no ?? '')">{{ row.meter_no || '' }}</span>
              <span v-else-if="col.key === 'use_unit'" class="row-text" :title="String(row.use_unit ?? '')">{{ row.use_unit || '' }}</span>
              <span v-else-if="col.key === 'default_unit_price'" class="row-mono">{{ fmtNum(row.default_unit_price) }}</span>
              <span v-else-if="col.key === 'meter_mode'" class="row-text">{{ row.meter_mode === 'auto' ? '自动抄表' : row.meter_mode === 'manual' ? '手动抄表' : '' }}</span>
              <span v-else-if="col.key === 'install_date'" class="row-mono">{{ row.install_date || '' }}</span>
              <span v-else class="row-text" :title="String(row.remark ?? '')">{{ row.remark }}</span>
            </div>
          </div>
          <div v-if="!loading && !displayRows.length" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部汇总（列表容器外固定显示；选中时按选中统计并避让浮动工具栏，样式与发票管理一致） -->
    <div v-if="summary.total" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : summary.total }} 条</span>
      <span class="doc-summary-item">总用量 <span class="doc-summary-val">{{ fmtNum(summaryUsage) }}</span> {{ unitLabel }}</span>
      <span class="doc-summary-item">总金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
    </div>

    <!-- 底部浮动工具栏（选中行时显示；打印弹窗打开时隐藏） -->
    <transition name="batch-bar-fade">
      <div v-if="selectedKeys.size && !printVisible" class="doc-batch-bar-fixed" role="region">
        <div class="batch-bar-inner">
          <div class="batch-bar-left">
            <span class="batch-bar-count">已选 {{ selectedKeys.size }} 项</span>
            <t-button variant="text" theme="default" size="small" class="batch-bar-clear" @click="clearSelection">
              清除
            </t-button>
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
            <t-popconfirm theme="warning"
              :content="`确定删除所选 ${selectedKeys.size} 条记录吗？`"
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

    <!-- 编辑/新增抽屉 -->
    <teleport to="body">
      <div v-if="drawerVisible" class="doc-drawer-resize-handle" :style="{ right: drawerWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="drawerVisible" :visible="true" :header="drawerTitle" :size="drawerWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="meter-record-drawer"
        @close="onDrawerClose" @update:visible="(v: boolean) => (drawerVisible = v)">
        <div class="meter-drawer-body">
          <div class="rec-grid">
            <div class="rec-field">
              <label>月份 <span class="required">*</span></label>
              <t-date-picker v-model="form.month" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份" />
              <p v-if="duplicateWarning" class="field-error-text">该月该{{ meterLabel }}已有记录，可直接编辑</p>
            </div>
            <div class="rec-field">
              <label>{{ meterLabel }} <span class="required">*</span></label>
              <t-select v-model="form.meterId" :placeholder="'选择' + meterLabel" :options="meterEditOptions" filterable @change="onMeterChange" />
              <p v-if="duplicateWarning" class="field-error-text">该月该{{ meterLabel }}已有记录，可直接编辑</p>
            </div>

            <div class="rec-field">
              <label>抄表日期</label>
              <t-date-picker v-model="form.readingDate" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选择日期" clearable />
            </div>
            <div class="rec-field">
              <label>抄表人</label>
              <t-input v-model="form.reader" placeholder="默认取表计管理人员" />
            </div>

            <div class="rec-field">
              <label>录入日期</label>
              <div class="readonly-val">{{ form.recordDate || today }}</div>
            </div>
            <div class="rec-field">
              <label>倍率</label>
              <div class="readonly-val">{{ fmtNum(formRate) }}×</div>
            </div>

            <div class="rec-field" :class="{ 'field-invalid': readingInvalid }">
              <label>起度 <span class="required">*</span></label>
              <t-input v-model.number="form.startReading" type="number" placeholder="起度" :status="readingInvalid ? 'error' : ''" />
              <p v-if="readingInvalid" class="field-error-text">止度不得小于起度</p>
            </div>
            <div class="rec-field" :class="{ 'field-invalid': readingInvalid }">
              <label>止度 <span class="required">*</span></label>
              <t-input v-model.number="form.endReading" type="number" placeholder="止度" :status="readingInvalid ? 'error' : ''" />
              <p v-if="readingInvalid" class="field-error-text">止度不得小于起度</p>
            </div>

            <div class="rec-field" :class="{ 'field-invalid': unitPriceDiff }">
              <label>单价 <span class="required">*</span></label>
              <t-input v-model.number="form.unitPrice" type="number" placeholder="单价" :status="unitPriceDiff ? 'error' : ''" />
              <p v-if="unitPriceDiff" class="field-error-text">与配置默认单价 {{ fmtNum(currentMeter?.default_unit_price) }} 不同</p>
            </div>
            <div class="rec-field">
              <label>补差</label>
              <t-input v-model.number="form.subsidy" type="number" placeholder="补差金额，可为负" />
            </div>
            <div class="rec-field">
              <label>{{ categoryLabel }}（自动计算）</label>
              <div class="calc-val-lg">{{ fmtMoney(formAmount) }} 元</div>
              <p class="field-hint">{{ usageLabel }} {{ fmtNum(formUsage) }} {{ unitLabel }}</p>
            </div>

            <div class="rec-field rec-field--wide">
              <label>备注</label>
              <t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" />
            </div>
          </div>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="drawerVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="saving" @click="save">保存</t-button>
        </div>
      </t-drawer>
    </teleport>

    <!-- 设置抽屉 -->
    <teleport to="body">
      <div v-if="settingsVisible" class="doc-drawer-resize-handle" :style="{ right: settingsWidth }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onSettingsResizeStart">
        <div class="doc-drawer-resize-line" />
      </div>
    </teleport>
    <teleport to="body">
      <t-drawer v-if="settingsVisible" :visible="true" :header="meterLabel + '配置'" :size="settingsWidth" :footer="false"
        :close-on-overlay-click="true" destroy-on-close class="meter-settings-drawer"
        @close="settingsVisible = false" @update:visible="(v: boolean) => (settingsVisible = v)">
        <div class="meter-settings-body">
          <div class="settings-head">
            <span class="settings-desc">{{ meterLabel }}配置：开启的显示在录入选择中，关闭后不影响已有记录</span>
            <t-button theme="primary" size="small" @click="openMeterForm(null)">
              <template #icon><t-icon name="add" /></template>
              新增{{ meterLabel }}
            </t-button>
          </div>

          <div v-if="meters.length" class="meter-search">
            <t-input v-model="meterSearch" placeholder="搜索别名 / 表号" clearable>
              <template #prefix-icon><t-icon name="search" size="14px" /></template>
            </t-input>
          </div>

          <div v-if="!meters.length && !meterFormVisible" class="meter-empty">
            <t-icon name="setting" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无{{ meterLabel }}，点击新增{{ meterLabel }}开始配置</span>
          </div>

          <div class="meter-grid" :style="{ gridTemplateColumns: meterGridCols }">
            <!-- 新增表计：表单展开在网格顶部 -->
            <div v-if="meterFormVisible && !meterForm.id" class="meter-form">
              <div class="meter-form-title">新增{{ meterLabel }}</div>
              <div class="form-grid">
                <div class="form-item">
                  <label>别名 <span class="required">*</span></label>
                  <t-input ref="meterAliasInput" v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
                </div>
                <div class="form-item">
                  <label>表号</label>
                  <t-input v-model="meterForm.meter_no" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>倍率</label>
                  <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                </div>
                <div class="form-item">
                  <label>默认单价</label>
                  <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
                </div>
                <div class="form-item">
                  <label>使用单位</label>
                  <t-input v-model="meterForm.use_unit" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>管理人员</label>
                  <t-input v-model="meterForm.manager" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>联系方式</label>
                  <t-input v-model="meterForm.contact" placeholder="选填" />
                </div>
                <div class="form-item">
                  <label>抄表方式</label>
                  <t-radio-group v-model="meterForm.meter_mode">
                    <t-radio-button value="auto">自动抄表</t-radio-button>
                    <t-radio-button value="manual">手动抄表</t-radio-button>
                  </t-radio-group>
                </div>
                <div class="form-item">
                  <label>安装日期</label>
                  <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                </div>
                <div class="form-item">
                  <label>启用</label>
                  <t-switch v-model="meterForm.enabled" />
                </div>
              </div>
              <div class="form-item form-item--full">
                <label>备注</label>
                <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
              </div>
              <div class="meter-form-actions">
                <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
              </div>
            </div>

            <template v-for="m in filteredMeters" :key="m.id">
              <div class="meter-card">
                <div class="meter-card-head">
                  <span class="meter-card-name">{{ m.alias }}</span>
                  <t-switch :model-value="!!m.enabled" size="small" @change="(v: any) => toggleEnabled(m, v)" />
                  <span class="meter-card-actions">
                    <t-button variant="text" size="small" @click="openMeterForm(m)">
                      <template #icon><t-icon name="edit" size="15px" /></template>
                    </t-button>
                    <t-popconfirm theme="warning" :content="`确定删除${meterLabel}「${m.alias}」吗？`"
                      :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                      @confirm="deleteMeter(m)">
                      <t-button variant="text" size="small" @click.stop>
                        <template #icon><t-icon name="delete" size="15px" /></template>
                      </t-button>
                    </t-popconfirm>
                  </span>
                </div>
                <div class="meter-card-grid">
                  <div class="meter-card-item"><span class="k">表号</span><span class="v">{{ m.meter_no || '—' }}</span></div>
                  <div class="meter-card-item"><span class="k">倍率</span><span class="v">{{ fmtNum(m.rate) }}</span></div>
                  <div class="meter-card-item"><span class="k">单价</span><span class="v">{{ fmtNum(m.default_unit_price) }} 元/{{ unitLabel }}</span></div>
                </div>
              </div>
              <!-- 编辑表计：表单展开在当前卡片下方 -->
              <div v-if="meterFormVisible && meterForm.id === m.id" class="meter-form">
                <div class="meter-form-title">编辑{{ meterLabel }}</div>
                <div class="form-grid">
                  <div class="form-item">
                    <label>别名 <span class="required">*</span></label>
                    <t-input v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
                  </div>
                  <div class="form-item">
                    <label>表号</label>
                    <t-input v-model="meterForm.meter_no" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>倍率</label>
                    <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
                  </div>
                  <div class="form-item">
                    <label>默认单价</label>
                    <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
                  </div>
                  <div class="form-item">
                    <label>使用单位</label>
                    <t-input v-model="meterForm.use_unit" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>管理人员</label>
                    <t-input v-model="meterForm.manager" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>联系方式</label>
                    <t-input v-model="meterForm.contact" placeholder="选填" />
                  </div>
                  <div class="form-item">
                    <label>抄表方式</label>
                    <t-radio-group v-model="meterForm.meter_mode">
                      <t-radio-button value="auto">自动抄表</t-radio-button>
                      <t-radio-button value="manual">手动抄表</t-radio-button>
                    </t-radio-group>
                  </div>
                  <div class="form-item">
                    <label>安装日期</label>
                    <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
                  </div>
                  <div class="form-item">
                    <label>启用</label>
                    <t-switch v-model="meterForm.enabled" />
                  </div>
                </div>
                <div class="form-item form-item--full">
                  <label>备注</label>
                  <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
                </div>
                <div class="meter-form-actions">
                  <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
                  <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
                </div>
              </div>
            </template>
          </div>
        </div>
      </t-drawer>
    </teleport>

    <!-- 打印预览弹窗 -->
    <div v-if="printVisible" class="meter-print-mask">
      <div class="meter-print-dialog">
        <div class="meter-print-header">
          <span class="meter-print-title">{{ categoryLabel }}目录预览（{{ printCount }} 条）</span>
          <t-button variant="text" size="small" class="meter-print-close" @click="closePrint">
            <t-icon name="close" size="16px" />
          </t-button>
        </div>
        <div class="meter-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true"></iframe>
          <div v-else class="print-preview-loading">
            <t-loading size="small" :text="'正在生成目录…'" />
          </div>
        </div>
        <div class="meter-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeterRecords,
  createUtilityMeterRecord,
  updateUtilityMeterRecord,
  deleteUtilityMeterRecord,
  listUtilityMeters,
  createUtilityMeter,
  updateUtilityMeter,
  deleteUtilityMeter,
} from '@/api/knowledge-base'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

const props = defineProps<{
  category: 'water' | 'gas' | 'electricity'
}>()

const categoryLabel = computed(() => (props.category === 'water' ? '水费' : props.category === 'electricity' ? '电费' : '气费'))
const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))
const usageLabel = computed(() => (props.category === 'water' ? '用水量' : props.category === 'electricity' ? '用电量' : '用气量'))

// ---- 列定义 ----
interface ColDef { key: string; label: string; default: boolean; w: string }
const COLUMN_DEFS: ColDef[] = [
  { key: 'month', label: '月份', default: true, w: '1fr' },
  { key: 'meter', label: meterLabel.value, default: true, w: '1.2fr' },
  { key: 'start_reading', label: '起度', default: true, w: '0.9fr' },
  { key: 'end_reading', label: '止度', default: true, w: '0.9fr' },
  { key: 'rate', label: '倍率', default: true, w: '0.7fr' },
  { key: 'usage', label: usageLabel.value, default: true, w: '1fr' },
  { key: 'unit_price', label: '单价', default: true, w: '0.9fr' },
  { key: 'subsidy', label: '补差', default: true, w: '0.8fr' },
  { key: 'amount', label: categoryLabel.value, default: true, w: '1fr' },
  { key: 'remark', label: '备注', default: true, w: '2fr' },
  // 详细字段：抄表信息与表计档案参数
  { key: 'reading_date', label: '抄表日期', default: false, w: '1fr' },
  { key: 'reader', label: '抄表人', default: false, w: '1fr' },
  { key: 'meter_no', label: '表号', default: false, w: '1fr' },
  { key: 'use_unit', label: '使用单位', default: false, w: '1.2fr' },
  { key: 'default_unit_price', label: '默认单价', default: false, w: '0.9fr' },
  { key: 'meter_mode', label: '抄表方式', default: false, w: '0.9fr' },
  { key: 'install_date', label: '安装日期', default: false, w: '1.1fr' },
]
const STORAGE_KEY = computed(() => `weknora-utility-meter-${props.category}-columns-v2`)
const visibleKeys = ref<string[]>(loadStoredKeys())
const visibleColDefs = computed(() => COLUMN_DEFS.filter(c => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `44px ${visibleColDefs.value.map(c => c.w).join(' ')}`,
}))

function loadStoredKeys(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) {
        // 仅保留本组件认识的字段，避免跨页面污染
        const valid = arr.filter(k => COLUMN_DEFS.some(c => c.key === k))
        if (valid.length) return valid
      }
    }
  } catch { /* ignore */ }
  return COLUMN_DEFS.filter(c => c.default).map(c => c.key)
}

const fieldPopupVisible = ref(false)
const selectAllColumns = () => {
  visibleKeys.value = COLUMN_DEFS.map(c => c.key)
  persistColumns()
}
const resetColumns = () => {
  visibleKeys.value = COLUMN_DEFS.filter(c => c.default).map(c => c.key)
  persistColumns()
}
const persistColumns = () => {
  try { localStorage.setItem(STORAGE_KEY.value, JSON.stringify(visibleKeys.value)) } catch { /* ignore */ }
}

// ---- 数据 ----
const meters = ref<any[]>([])
const rows = ref<any[]>([]) // 展开后的扁平行
const displayRows = ref<any[]>([])
const loading = ref(true)
const filters = ref<{ month?: string; meterId?: string }>({ month: undefined, meterId: undefined })

const meterFilterOptions = computed(() => meters.value.filter(m => m.enabled).map(m => ({ label: m.alias, value: m.id })))
const meterSearch = ref('')
const filteredMeters = computed(() => {
  const kw = meterSearch.value.trim().toLowerCase()
  if (!kw) return meters.value
  return meters.value.filter((m: any) =>
    (m.alias || '').toLowerCase().includes(kw) || (m.meter_no || '').toLowerCase().includes(kw))
})

const load = async () => {
  loading.value = true
  try {
    const [recRes, meterRes]: any[] = await Promise.all([
      listUtilityMeterRecords({ category: props.category }),
      listUtilityMeters({ category: props.category }),
    ])
    const recData = recRes?.data || {}
    const records = recData?.records || recRes?.records || []
    const mets = meterRes?.data || recData?.meters || meterRes || []
    meters.value = Array.isArray(mets) ? mets : []
    const meterMap = new Map(meters.value.map((m: any) => [m.id, m]))
    const flat: any[] = []
    for (const rec of records) {
      for (const it of (rec.items || [])) {
        const meter = meterMap.get(it.meter_id)
        flat.push({
          record_id: rec.id,
          item_id: it.id,
          key: `${rec.id}__${it.id}`,
          month: rec.month,
          record_date: rec.record_date || '',
          reading_date: it.reading_date || '',
          reader: it.reader || '',
          meter_id: it.meter_id,
          meter_alias: meter?.alias || it.meter_name || '未配置',
          meter_no: meter?.meter_no || '',
          use_unit: meter?.use_unit || '',
          default_unit_price: meter?.default_unit_price ?? '',
          meter_mode: meter?.meter_mode || '',
          install_date: meter?.install_date || '',
          start_reading: it.start_reading,
          end_reading: it.end_reading,
          rate: meter?.rate ?? it.rate ?? '',
          usage: it.usage,
          unit_price: it.unit_price,
          subsidy: it.subsidy,
          amount: it.amount,
          remark: it.remark || '',
        })
      }
    }
    rows.value = flat
    // 构建 表计→月份→止度 索引（新增记录时上月止度自动填充起度）
    const idx = new Map<string, Map<string, number>>()
    for (const r of flat) {
      if (!r.meter_id) continue
      let m = idx.get(r.meter_id)
      if (!m) { m = new Map(); idx.set(r.meter_id, m) }
      m.set(r.month, Number(r.end_reading) || 0)
    }
    meterMonthEnds.value = idx
    applyFilters()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

const applyFilters = () => {
  let list = rows.value
  if (filters.value.month) list = list.filter(r => r.month === filters.value.month)
  if (filters.value.meterId) list = list.filter(r => r.meter_id === filters.value.meterId)
  displayRows.value = list
}

// ---- 汇总 ----
const summary = computed(() => {
  const base = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return { total: base.length }
})
const selectedKeys = ref<Set<string>>(new Set())
const selectedRows = computed(() => rows.value.filter(r => selectedKeys.value.has(r.key)))
const summaryUsage = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.usage) || 0), 0) * 100) / 100
})
const summaryAmount = computed(() => {
  const arr = selectedKeys.value.size ? selectedRows.value : displayRows.value
  return Math.round(arr.reduce((s, r) => s + (Number(r.amount) || 0), 0) * 100) / 100
})

const toggleSelect = (row: any, checked: any) => {
  const key = row.key
  const next = new Set(selectedKeys.value)
  if (checked) next.add(key)
  else next.delete(key)
  selectedKeys.value = next
}
const isAllSelected = computed(() => displayRows.value.length > 0 && displayRows.value.every(r => selectedKeys.value.has(r.key)))
const someSelected = computed(() => displayRows.value.some(r => selectedKeys.value.has(r.key)) && !isAllSelected.value)
const toggleSelectAll = (checked: any) => {
  const next = new Set(selectedKeys.value)
  if (checked) displayRows.value.forEach(r => next.add(r.key))
  else displayRows.value.forEach(r => next.delete(r.key))
  selectedKeys.value = next
}
// 单击行：单选该行并打开编辑抽屉（与发票管理一致）
const onRowClick = (row: any) => {
  selectedKeys.value = new Set([row.key])
  openEdit(row)
}
const clearSelection = () => { selectedKeys.value = new Set() }

// ---- 编辑抽屉 ----
const drawerVisible = ref(false)
const saving = ref(false)
const editingRecordId = ref('')
const editingItemId = ref('')
const editingOriginalMonth = ref('') // 编辑行原月份（改月时仅迁移该行）
const lastRecordId = ref('')
const recordItems = ref<any[]>([]) // 当前编辑 record 的原始 items（编辑时保留其它行）
const form = ref<{ month: string; meterId: string; readingDate: string; reader: string; recordDate: string; startReading: number; endReading: number; unitPrice: number; subsidy: number; remark: string }>({
  month: '', meterId: '', readingDate: '', reader: '', recordDate: '', startReading: 0, endReading: 0, unitPrice: 0, subsidy: 0, remark: '',
})
const today = (() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
})()
const drawerTitle = computed(() => (editingItemId.value ? '编辑' + categoryLabel.value + '记录' : '新增' + categoryLabel.value + '记录'))

const meterEditOptions = computed(() => {
  const opts = meters.value.filter(m => m.enabled).map((m: any) => ({ label: m.alias, value: m.id }))
  // 编辑时若当前水表已停用，保留在选项中
  if (form.value.meterId && !meters.value.some((m: any) => m.id === form.value.meterId && m.enabled)) {
    const cur = meters.value.find((m: any) => m.id === form.value.meterId)
    if (cur) opts.push({ label: cur.alias, value: cur.id })
  }
  return opts
})

const currentMeter = computed(() => meters.value.find((m: any) => m.id === form.value.meterId))
const formRate = computed(() => Number(currentMeter.value?.rate) > 0 ? Number(currentMeter.value?.rate) : 1)
const formUsage = computed(() => Math.round((Number(form.value.endReading) - Number(form.value.startReading)) * formRate.value * 100) / 100)
const formAmount = computed(() => Math.round((formUsage.value * Number(form.value.unitPrice) + (Number(form.value.subsidy) || 0)) * 100) / 100)

// ---- 录入校验：差异标红提醒不拦截，止度<起度（起度>止度）标红且保存拦截；起度=止度视为当月无用量，合法 ----
const readingInvalid = computed(() => {
  const s = Number(form.value.startReading) || 0
  const e = Number(form.value.endReading) || 0
  return s > e
})
const duplicateWarning = computed(() => {
  if (editingItemId.value) return false
  if (!form.value.month || !form.value.meterId) return false
  return meterMonthEnds.value.get(form.value.meterId)?.has(form.value.month) || false
})
const unitPriceDiff = computed(() => {
  const up = Number(form.value.unitPrice)
  const def = Number(currentMeter.value?.default_unit_price)
  return !!form.value.meterId && up > 0 && def > 0 && up !== def
})
// 设置抽屉表计卡片网格列数（随抽屉宽度自适应）
const meterGridCols = computed(() => {
  const w = parseInt(settingsWidth.value) || 680
  if (w >= 980) return 'repeat(3, minmax(0, 1fr))'
  if (w >= 660) return 'repeat(2, minmax(0, 1fr))'
  return 'repeat(1, minmax(0, 1fr))'
})

const onMeterChange = () => {
  if (currentMeter.value) {
    form.value.unitPrice = Number(currentMeter.value.default_unit_price) || 0
    if (!form.value.reader) form.value.reader = currentMeter.value.manager || ''
  }
}

const openCreate = () => {
  editingRecordId.value = ''
  editingItemId.value = ''
  editingOriginalMonth.value = ''
  lastRecordId.value = ''
  recordItems.value = []
  form.value = {
    month: '', meterId: '', readingDate: today, reader: '', recordDate: today,
    startReading: 0, endReading: 0, unitPrice: 0, subsidy: 0, remark: '',
  }
  drawerVisible.value = true
}

// ---- 新增记录：上月止度自动填充本月起度 ----
const meterMonthEnds = ref<Map<string, Map<string, number>>>(new Map())
const prevMonthOf = (month: string) => {
  const [y, m] = month.split('-').map(Number)
  if (!y || !m) return ''
  if (m === 1) return `${y - 1}-12`
  return `${y}-${String(m - 1).padStart(2, '0')}`
}
// 连续录入时跳过自动填充（起度/单价由保存逻辑显式写入）
const skipAutoFill = ref(false)
watch([() => form.value.month, () => form.value.meterId], () => {
  if (editingItemId.value) return // 编辑模式不填充
  if (skipAutoFill.value) { skipAutoFill.value = false; return } // 连续录入：已显式设置
  if (!form.value.month || !form.value.meterId) return
  const ends = meterMonthEnds.value.get(form.value.meterId)
  const prevEnd = ends?.get(prevMonthOf(form.value.month))
  if (prevEnd) {
    form.value.startReading = prevEnd
  } else {
    form.value.startReading = 0
  }
})

const openEditSelected = () => {
  const row = selectedRows.value[0]
  if (!row) return
  openEdit(row)
}

const openEdit = (row: any) => {
  editingRecordId.value = row.record_id
  editingItemId.value = row.item_id
  editingOriginalMonth.value = row.month
  const rec = rowsOfRecord(row.record_id)
  recordItems.value = rec.map((r: any) => ({
    item_id: r.item_id,
    meter_id: r.meter_id,
    reading_date: r.reading_date || '',
    reader: r.reader || '',
    start_reading: Number(r.start_reading) || 0,
    end_reading: Number(r.end_reading) || 0,
    unit_price: Number(r.unit_price) || 0,
    subsidy: Number(r.subsidy) || 0,
    remark: r.remark || '',
  }))
  form.value = {
    month: row.month,
    meterId: row.meter_id || '',
    readingDate: row.reading_date || today,
    reader: row.reader || '',
    recordDate: row.record_date || today,
    startReading: Number(row.start_reading) || 0,
    endReading: Number(row.end_reading) || 0,
    unitPrice: Number(row.unit_price) || 0,
    subsidy: Number(row.subsidy) || 0,
    remark: row.remark || '',
  }
  drawerVisible.value = true
}

function rowsOfRecord(recordId: string) {
  return rows.value.filter(r => r.record_id === recordId)
}

const save = async () => {
  if (!form.value.month) {
    MessagePlugin.warning('请选择月份')
    return
  }
  if (!form.value.meterId) {
    MessagePlugin.warning(`请选择${meterLabel}`)
    return
  }
  if (readingInvalid.value) {
    MessagePlugin.error('止度不得小于起度')
    return
  }
  saving.value = true
  try {
    const editedItem = {
      meter_id: form.value.meterId,
      reading_date: form.value.readingDate || '',
      reader: form.value.reader || '',
      start_reading: Number(form.value.startReading) || 0,
      end_reading: Number(form.value.endReading) || 0,
      unit_price: Number(form.value.unitPrice) || 0,
      subsidy: Number(form.value.subsidy) || 0,
      remark: form.value.remark || '',
    }
    const isNew = !editingItemId.value
    if (editingItemId.value) {
      // 单行改月份：仅迁移该行到目标月，其它行保留原月（不整月搬家）
      const monthChanged = form.value.month !== editingOriginalMonth.value
      if (monthChanged && recordItems.value.length > 1) {
        const others = recordItems.value.filter((it: any) => it.item_id !== editingItemId.value)
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: editingOriginalMonth.value,
          record_date: form.value.recordDate || today,
          remark: '',
          items: others.map((it: any) => ({
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: Number(it.start_reading) || 0,
            end_reading: Number(it.end_reading) || 0,
            unit_price: Number(it.unit_price) || 0,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          })),
        })
        await createUtilityMeterRecord({
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items: [editedItem],
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      } else {
        // 更新：整 record 全量提交，替换编辑行、保留其它行
        const items = recordItems.value.map((it: any) => {
          if (it.item_id === editingItemId.value) return editedItem
          return {
            meter_id: it.meter_id,
            reading_date: it.reading_date || '',
            reader: it.reader || '',
            start_reading: it.start_reading,
            end_reading: it.end_reading,
            unit_price: it.unit_price,
            subsidy: Number(it.subsidy) || 0,
            remark: it.remark || '',
          }
        })
        await updateUtilityMeterRecord(editingRecordId.value, {
          category: props.category,
          month: form.value.month,
          record_date: form.value.recordDate || today,
          remark: '',
          items,
        })
        MessagePlugin.success('已保存')
        lastRecordId.value = ''
      }
    } else if (lastRecordId.value) {
      // 连续录入后续条：同月 record 追加 item（同月唯一，后端要求 update）
      const existing = rowsOfRecord(lastRecordId.value)
      const items = [
        ...existing.map((r: any) => ({
          meter_id: r.meter_id,
          reading_date: r.reading_date || '',
          reader: r.reader || '',
          start_reading: Number(r.start_reading) || 0,
          end_reading: Number(r.end_reading) || 0,
          unit_price: Number(r.unit_price) || 0,
          subsidy: Number(r.subsidy) || 0,
          remark: r.remark || '',
        })),
        editedItem,
      ]
      await updateUtilityMeterRecord(lastRecordId.value, {
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items,
      })
      MessagePlugin.success('已新增')
    } else {
      const res: any = await createUtilityMeterRecord({
        category: props.category,
        month: form.value.month,
        record_date: form.value.recordDate || today,
        remark: '',
        items: [editedItem],
      })
      const rec = res?.data?.data || res?.data || res
      lastRecordId.value = rec?.id || ''
      MessagePlugin.success('已新增')
    }
    const nextMeter = (() => {
      if (!isNew) return undefined
      const enabled = meters.value.filter((m: any) => m.enabled)
      const idx = enabled.findIndex((m: any) => m.id === form.value.meterId)
      return idx >= 0 ? enabled[idx + 1] : undefined
    })()
    if (nextMeter) {
      // 连续录入：抽屉不关闭，按启用表顺序切到下一张，月份复用、抄表日期=第一条、
      // 起度=下一张表自身上月止度（无则空）、单价=上一条、抄表人=下张表管理人员
      skipAutoFill.value = true
      form.value.meterId = nextMeter.id
      form.value.reader = nextMeter.manager || ''
      const nextEnds = meterMonthEnds.value.get(nextMeter.id)
      const prevEnd = nextEnds?.get(prevMonthOf(form.value.month))
      form.value.startReading = prevEnd ? Number(prevEnd) : 0
      form.value.endReading = 0
      form.value.unitPrice = Number(form.value.unitPrice) || 0
      form.value.remark = ''
      editingRecordId.value = ''
      editingItemId.value = ''
      recordItems.value = []
    } else {
      drawerVisible.value = false
      clearSelection()
      lastRecordId.value = ''
    }
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const onDrawerClose = () => { drawerVisible.value = false }

// ---- 删除 ----
const handleDelete = async () => {
  const targets = selectedRows.value
  if (!targets.length) return
  // 按 record 分组：单 item record 直接删 record；多 item record 重提交去掉目标行
  const groups = new Map<string, any[]>()
  for (const r of targets) {
    const arr = groups.get(r.record_id) || []
    arr.push(r)
    groups.set(r.record_id, arr)
  }
  try {
    for (const [recordId, itemRows] of groups) {
      const all = rowsOfRecord(recordId)
      const targetItemIds = new Set(itemRows.map(r => r.item_id))
      const remaining = all.filter(r => !targetItemIds.has(r.item_id))
      if (!remaining.length) {
        await deleteUtilityMeterRecord(recordId)
      } else {
        await updateUtilityMeterRecord(recordId, {
          category: props.category,
          month: remaining[0].month,
          record_date: remaining[0].record_date || '',
          remark: '',
          items: remaining.map((r: any) => ({
            meter_id: r.meter_id,
            reading_date: r.reading_date || '',
            reader: r.reader || '',
            start_reading: Number(r.start_reading) || 0,
            end_reading: Number(r.end_reading) || 0,
            unit_price: Number(r.unit_price) || 0,
            remark: r.remark || '',
          })),
        })
      }
    }
    MessagePlugin.success('已删除')
    clearSelection()
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 打印 ----
const printVisible = ref(false)
const printBusy = ref(false)
const catalogBusy = ref(false)
const printLoaded = ref(false)
const printUrl = ref('')
const printCount = ref(0)

const catalogValueOf = (r: any, key: string): string => {
  switch (key) {
    case 'month': return r.month
    case 'meter': return r.meter_alias
    case 'start_reading': return fmtNum(r.start_reading)
    case 'end_reading': return fmtNum(r.end_reading)
    case 'rate': return fmtNum(r.rate)
    case 'usage': return `${fmtNum(r.usage)} ${unitLabel.value}`
    case 'unit_price': return fmtNum(r.unit_price)
    case 'subsidy': return r.subsidy === '' || r.subsidy == null ? '' : fmtMoney(r.subsidy)
    case 'amount': return `${fmtMoney(r.amount)} 元`
    case 'reading_date': return r.reading_date || ''
    case 'reader': return r.reader || ''
    case 'meter_no': return r.meter_no || ''
    case 'use_unit': return r.use_unit || ''
    case 'default_unit_price': return r.default_unit_price === '' || r.default_unit_price == null ? '' : fmtNum(r.default_unit_price)
    case 'meter_mode': return r.meter_mode === 'auto' ? '自动抄表' : r.meter_mode === 'manual' ? '手动抄表' : (r.meter_mode || '')
    case 'install_date': return r.install_date || ''
    case 'remark': return r.remark || ''
    default: return ''
  }
}

const handlePrint = async () => {
  const rowsToPrint = selectedRows.value
  if (!rowsToPrint.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = visibleColDefs.value.map((c: any) => ({
      key: c.key,
      label: c.label,
      value: (r: any) => catalogValueOf(r, c.key),
    }))
    const bytes = await generateCatalogPdf({ title: `${categoryLabel.value}目录`, columns: cols, rows: rowsToPrint })
    printCount.value = rowsToPrint.length
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes as unknown as BlobPart], { type: 'application/pdf' }))
    printLoaded.value = false
    printVisible.value = true
  } catch (e: any) {
    MessagePlugin.error(e?.message || '目录生成失败')
  } finally {
    catalogBusy.value = false
  }
}
const closePrint = () => {
  printVisible.value = false
  if (printUrl.value) { URL.revokeObjectURL(printUrl.value); printUrl.value = '' }
}

// ---- 水表配置设置抽屉 ----
const settingsVisible = ref(false)
const savingMeter = ref(false)
const meterFormVisible = ref(false)
const meterForm = ref<any>({})
const meterAliasInput = ref()
const settingsWidth = ref<string>(loadSettingsWidth())
function loadSettingsWidth(): string {
  try {
    const v = parseInt(localStorage.getItem('weknora-utility-meter-settings-width') || '')
    if (!Number.isNaN(v)) return clampWidth(v, 520) + 'px'
  } catch { /* ignore */ }
  return '680px'
}

const openSettings = async () => {
  settingsVisible.value = true
  meterFormVisible.value = false
  if (!meters.value.length) {
    try {
      const res: any = await listUtilityMeters({ category: props.category })
      const mets = res?.data || res || []
      meters.value = Array.isArray(mets) ? mets : []
    } catch { /* ignore */ }
  }
}

const openMeterForm = (m: any) => {
  meterForm.value = m ? {
    id: m.id,
    alias: m.alias || '',
    meter_no: m.meter_no || '',
    rate: Number(m.rate) > 0 ? m.rate : 1,
    default_unit_price: Number(m.default_unit_price) || 0,
    use_unit: m.use_unit || '',
    manager: m.manager || '',
    contact: m.contact || '',
    meter_mode: m.meter_mode || 'manual',
    install_date: m.install_date || '',
    remark: m.remark || '',
    enabled: m.enabled !== false,
  } : {
    id: '', alias: '', meter_no: nextMeterNo(), rate: 1, default_unit_price: 0,
    use_unit: '', manager: '', contact: '', meter_mode: 'manual',
    install_date: '', remark: '', enabled: true,
  }
  meterFormVisible.value = true
  nextTick(() => { if (!m) (meterAliasInput.value as any)?.focus?.() })
}

const saveMeter = async () => {
  if (!meterForm.value.alias?.trim()) {
    MessagePlugin.warning('请填写别名')
    return
  }
  savingMeter.value = true
  try {
    const payload = {
      category: props.category,
      alias: meterForm.value.alias.trim(),
      meter_no: meterForm.value.meter_no || '',
      rate: Number(meterForm.value.rate) > 0 ? Number(meterForm.value.rate) : 1,
      default_unit_price: Number(meterForm.value.default_unit_price) || 0,
      use_unit: meterForm.value.use_unit || '',
      manager: meterForm.value.manager || '',
      contact: meterForm.value.contact || '',
      meter_mode: meterForm.value.meter_mode || 'manual',
      install_date: meterForm.value.install_date || '',
      remark: meterForm.value.remark || '',
      enabled: meterForm.value.enabled !== false,
    }
    const isEdit = !!meterForm.value.id
    if (isEdit) {
      await updateUtilityMeter(meterForm.value.id, payload)
      MessagePlugin.success('已保存')
    } else {
      await createUtilityMeter(payload)
      MessagePlugin.success('已新增')
    }
    await loadMetersOnly()
    if (isEdit) {
      // 编辑保存后关闭
      meterFormVisible.value = false
    } else {
      // 连续新增：保持表单展开，表号自动递增，清空其余字段并聚焦别名
      meterForm.value = {
        id: '',
        alias: '',
        meter_no: nextMeterNo(),
        rate: '',
        default_unit_price: '',
        use_unit: '',
        manager: '',
        contact: '',
        meter_mode: 'manual',
        install_date: '',
        remark: '',
        enabled: true,
      }
      meterFormVisible.value = true
      nextTick(() => { (meterAliasInput.value as any)?.focus?.() })
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}
const meterNoPrefix = () => (props.category === 'water' ? 'W' : props.category === 'electricity' ? 'E' : 'G')
const nextMeterNo = () => {
  let max = 0
  for (const m of meters.value) {
    const mm = String(m.meter_no || '').match(/^[A-Za-z]*(\d+)$/)
    if (mm) max = Math.max(max, parseInt(mm[1], 10))
  }
  return meterNoPrefix() + String(max + 1).padStart(3, '0')
}

const toggleEnabled = async (m: any, v: any) => {
  try {
    await updateUtilityMeter(m.id, { ...m, enabled: !!v })
    m.enabled = !!v
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}

const deleteMeter = async (m: any) => {
  try {
    await deleteUtilityMeter(m.id)
    MessagePlugin.success('已删除')
    meters.value = meters.value.filter(x => x.id !== m.id)
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

const loadMetersOnly = async () => {
  try {
    const res: any = await listUtilityMeters({ category: props.category })
    const mets = res?.data || res || []
    meters.value = Array.isArray(mets) ? mets : []
  } catch { /* ignore */ }
}

// ---- 抽屉宽度拖动 ----
const drawerWidth = ref<string>(loadDrawerWidth())
function loadDrawerWidth(): string {
  try {
    const v = parseInt(localStorage.getItem(`weknora-utility-${props.category}-drawer-width`) || '')
    if (!Number.isNaN(v)) return clampWidth(v, 480) + 'px'
  } catch { /* ignore */ }
  return '640px'
}
let dragging = false
let startX = 0
let startW = 640
let draggingS = false
let startXS = 0
let startWS = 680
const clampWidth = (w: number, min: number) => Math.min(Math.floor(window.innerWidth * 0.95), Math.max(min, w))

// 记录抽屉宽度拖动（发票管理同款：独立手柄 + document 监听）
const onDrawerResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  dragging = true
  startX = e.clientX
  startW = parseInt(drawerWidth.value) || 640
  document.addEventListener('mousemove', onDrawerResizeMove)
  document.addEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = 'col-resize'
}
const onDrawerResizeMove = (e: MouseEvent) => {
  if (!dragging) return
  drawerWidth.value = clampWidth(startW + (startX - e.clientX), 480) + 'px'
}
const onDrawerResizeEnd = () => {
  if (!dragging) return
  dragging = false
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.body.style.cursor = ''
  try { localStorage.setItem(`weknora-utility-${props.category}-drawer-width`, drawerWidth.value) } catch { /* ignore */ }
}

// 设置抽屉宽度拖动
const onSettingsResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  draggingS = true
  startXS = e.clientX
  startWS = parseInt(settingsWidth.value) || 680
  document.addEventListener('mousemove', onSettingsResizeMove)
  document.addEventListener('mouseup', onSettingsResizeEnd)
  document.body.style.cursor = 'col-resize'
}
const onSettingsResizeMove = (e: MouseEvent) => {
  if (!draggingS) return
  settingsWidth.value = clampWidth(startWS + (startXS - e.clientX), 520) + 'px'
}
const onSettingsResizeEnd = () => {
  if (!draggingS) return
  draggingS = false
  document.removeEventListener('mousemove', onSettingsResizeMove)
  document.removeEventListener('mouseup', onSettingsResizeEnd)
  document.body.style.cursor = ''
  try { localStorage.setItem('weknora-utility-meter-settings-width', settingsWidth.value) } catch { /* ignore */ }
}

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}

onMounted(() => { load() })
onBeforeUnmount(() => {
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  document.removeEventListener('mousemove', onSettingsResizeMove)
  document.removeEventListener('mouseup', onSettingsResizeEnd)
})
</script>

<style lang="less" scoped>
.utility-meter-tab {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  padding-top: 4px;
}

/* 筛选工具栏（与合同管理一致） */
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

/* 列表组件样式（与合同/发票/知识库列表保持一致，scoped 自包含） */
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
}

.doc-list-body {
  display: flex;
  flex-direction: column;
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
  position: sticky;
  left: 0;
  z-index: 2;
  background: transparent;
  padding: 0;
}

.doc-list-check :deep(.t-checkbox__label) { display: none !important; width: 0 !important; min-width: 0 !important; margin: 0 !important; padding: 0 !important; }
.doc-list-check :deep(.t-checkbox__input-wrapper) { margin: 0; }

.row-mono,
.row-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-mono {
  font-family: var(--app-font-family);
}

.os-neg { color: var(--td-error-color, #d54941); }

.meter-list-scroll {
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

  .meter-empty-icon {
    color: var(--td-text-color-placeholder);
  }

  .meter-empty-text {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
  }
}

/* 底部汇总（与电费/光伏一致） */
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

/* 抽屉 resize 手柄（发票管理同款，teleport 到 body 避免被抽屉遮罩遮挡） */
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

/* 浮动工具栏（与电费/合同一致） */
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

  label {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 6px;
  }
  .reading-row { display: flex; align-items: center; gap: 8px; }
  .reading-input { flex: 1; }
  .reading-sep { color: var(--td-text-color-placeholder); flex: 0 0 auto; }
  .field-error-text {
    font-size: 12px;
    color: var(--td-error-color);
    margin-top: 4px;
    line-height: 1.4;
  }
}
.rec-field--wide {
  grid-column: 1 / -1;
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
.calc-val-lg {
  min-height: 30px;
  display: flex;
  align-items: center;
  font-size: 14px;
  font-weight: 600;
  color: var(--td-brand-color);
  font-variant-numeric: tabular-nums;
}
.field-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 4px;
  line-height: 1.4;
}
.calc-row {
  display: flex;
  align-items: center;
  gap: 16px;
  min-height: 30px;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  .calc-rate { color: var(--td-brand-color); }
  .calc-val { color: var(--td-text-color-primary); font-weight: 600; font-variant-numeric: tabular-nums; }
}

.setting-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 14px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child {
    border-bottom: none;
  }
}

.setting-info {
  flex: 0 0 30%;
  max-width: 30%;
  padding-right: 16px;

  label {
    font-size: 14px;
    font-weight: 500;
    color: var(--td-text-color-primary);
    display: block;
    margin-bottom: 4px;

    .required {
      color: var(--td-error-color);
    }
  }

  .desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.5;
  }
}

.setting-control {
  flex: 1;
  min-width: 0;
}

.reading-row {
  display: flex;
  align-items: center;
  gap: 10px;

  .reading-input {
    flex: 1;
  }

  .reading-sep {
    color: var(--td-text-color-placeholder);
  }
}

.calc-row {
  display: flex;
  gap: 24px;
  padding: 10px 12px;
  border-radius: 6px;
  background: var(--td-bg-color-container-hover);
  font-size: 13px;
  color: var(--td-text-color-primary);

  .calc-val {
    font-size: 15px;
    color: var(--td-brand-color);
    font-family: var(--app-font-family);
  }
}

.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
}

/* 水表配置抽屉 */
.meter-grid {
  display: grid;
  gap: 12px;
  align-items: start;
}
.meter-search {
  margin-bottom: 12px;
}
.meter-settings-body {
  padding: 4px 0 24px;
}

.settings-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;

  .settings-desc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

.meter-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 10px 12px;
  background: var(--td-bg-color-container);
  min-width: 0;

  .meter-card-head {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 10px;

    .meter-card-name {
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      flex: 1;
    }

    .meter-card-actions {
      display: flex;
      align-items: center;
      gap: 2px;
    }
  }

  .meter-card-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px 16px;

    .meter-card-item {
      display: flex;
      align-items: center;
      gap: 8px;
      font-size: 12px;

      .k {
        color: var(--td-text-color-secondary);
        flex: 0 0 auto;
      }

      .v {
        color: var(--td-text-color-primary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
  }
}

.meter-form {
  margin-top: 0;
  grid-column: 1 / -1;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 14px;
  background: var(--td-bg-color-secondarycontainer);

  .meter-form-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    color: var(--td-text-color-primary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
  }

  .form-item {
    label {
      display: block;
      font-size: 12px;
      color: var(--td-text-color-secondary);
      margin-bottom: 4px;

      .required {
        color: var(--td-error-color);
      }
    }

    &--full {
      margin-top: 12px;
    }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
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
</style>
