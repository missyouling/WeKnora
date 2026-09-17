<template>
  <div class="fleet-billing-container">
    <!-- 筛选工具栏 -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-date-picker v-model="month" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="月份"
            clearable class="doc-date-picker doc-filter-field__control" @change="loadSummary" />
        </div>
        <div class="doc-filter-field">
          <t-select v-model="vehicleId" :options="vehicleOptions" filterable clearable placeholder="全部车辆"
            class="doc-filter-select doc-filter-field__control" @change="loadSummary" />
        </div>
        <t-button variant="outline" size="small" @click="loadSummary">
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
        <t-button theme="primary" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
          <template #icon><t-icon name="print" size="14px" /></template>
          打印
        </t-button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="doc-list-scroll fleet-billing-scroll">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <template v-for="col in visibleCols" :key="col.key">
            <div class="cell cell-body" role="columnheader" :title="col.label">{{ col.label }}</div>
          </template>
        </div>
        <div v-if="!loading && !rows.length" class="doc-empty">
          <t-icon name="search-error" size="40px" class="doc-empty-icon" />
          <span class="doc-empty-text">暂无数据，请选择月份</span>
        </div>
        <template v-for="row in rows" :key="row.vehicle_id">
          <div class="doc-list-row" :style="gridStyle" role="row">
            <template v-for="col in visibleCols" :key="col.key">
              <div class="cell cell-body" :title="String(col.value(row) ?? '')">{{ col.value(row) }}</div>
            </template>
          </div>
        </template>
      </div>
    </div>

    <!-- 底部汇总 -->
    <div v-if="rows.length" class="doc-summary-bar">
      <span class="doc-summary-count">共 {{ rows.length }} 辆车</span>
      <span class="doc-summary-item">总费用 <span class="doc-summary-val">{{ fmtMoney(totalAll) }}</span> 元</span>
    </div>

    <!-- 打印预览弹窗 -->
    <div v-if="printVisible" class="meter-print-mask">
      <div class="meter-print-dialog">
        <div class="meter-print-header">
          <span class="meter-print-title">费用清单预览（{{ month }} · {{ printCount }} 辆车）</span>
          <t-button variant="text" size="small" class="meter-print-close" @click="closePrint">
            <template #icon><t-icon name="close" /></template>
          </t-button>
        </div>
        <div class="meter-print-body">
          <iframe v-if="printUrl" :src="printUrl" class="print-preview-frame" @load="printLoaded = true" />
          <div v-else class="print-preview-loading"><t-loading size="small" text="正在生成打印预览..." /></div>
        </div>
        <div class="meter-print-footer">
          <t-button variant="outline" size="small" @click="closePrint">关闭</t-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listFleetVehicles, getFleetSummary } from '@/api/fleet'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

const month = ref('')
const vehicleId = ref('' as any)
const rows = ref<any[]>([])
const vehicles = ref<any[]>([])
const loading = ref(false)

const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.id })))

const fmtMoney = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })

const COL_DEFS = [
  { key: 'plate_no', label: '车牌号', def: true, value: (r: any) => r.plate_no || '—' },
  { key: 'vehicle_type', label: '车辆类型', def: false, value: (r: any) => r.vehicle_type || '—' },
  { key: 'fuel_amount', label: '加油(元)', def: true, value: (r: any) => fmtMoney(r.fuel_amount) },
  { key: 'maintain_amount', label: '维保(元)', def: true, value: (r: any) => fmtMoney(r.maintain_amount) },
  { key: 'insurance_amount', label: '车险(元)', def: true, value: (r: any) => fmtMoney(r.insurance_amount) },
  { key: 'tire_amount', label: '轮胎(元)', def: false, value: (r: any) => fmtMoney(r.tire_amount) },
  { key: 'material_amount', label: '辅材(元)', def: false, value: (r: any) => fmtMoney(r.material_amount) },
  { key: 'violation_amount', label: '违章罚款(元)', def: true, value: (r: any) => fmtMoney(r.violation_amount) },
  { key: 'toll_amount', label: '通行费(元)', def: true, value: (r: any) => fmtMoney(r.toll_amount) },
  { key: 'total_amount', label: '合计(元)', def: true, value: (r: any) => fmtMoney(r.total_amount) },
  { key: 'record_count', label: '记录数', def: false, value: (r: any) => r.record_count ?? '—' },
]
const columnDefs = COL_DEFS
const COL_KEY = 'weknora-fleet-billing-columns-v1'
const visibleKeys = ref<string[]>([])
const fieldPopupVisible = ref(false)

function persistColumns() { localStorage.setItem(COL_KEY, JSON.stringify(visibleKeys.value)) }
function resetColumns() { visibleKeys.value = columnDefs.filter((c) => c.def).map((c) => c.key); persistColumns() }
function selectAllColumns() { visibleKeys.value = columnDefs.map((c) => c.key); persistColumns() }
function initColumns() {
  const saved = localStorage.getItem(COL_KEY)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      if (Array.isArray(arr) && arr.length) {
        visibleKeys.value = columnDefs.map((c) => c.key).filter((k) => arr.includes(k))
        return
      }
    } catch { /* ignore */ }
  }
  resetColumns()
}
onMounted(() => { initColumns(); loadSummary() })

const visibleCols = computed(() => columnDefs.filter((c) => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => {
  const flexes = Array(visibleCols.value.length).fill('minmax(96px, 1fr)').join(' ')
  return { gridTemplateColumns: flexes, minWidth: `${visibleCols.value.length * 96}px` }
})
const totalAll = computed(() => rows.value.reduce((s, r) => s + (Number(r.total_amount) || 0), 0))

async function loadSummary() {
  if (!month.value) { rows.value = []; return }
  loading.value = true
  try {
    const res = await getFleetSummary({ month: month.value, vehicle_id: vehicleId.value || undefined })
    rows.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    const vr = await listFleetVehicles()
    vehicles.value = vr.data || []
  } catch { /* ignore */ }
})

// 打印
const printVisible = ref(false)
const printUrl = ref('')
const printCount = ref(0)
const catalogBusy = ref(false)

async function handlePrint() {
  if (!rows.value.length) { MessagePlugin.warning('当前无数据可打印'); return }
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = visibleCols.value.map((c: any) => ({ key: c.key, label: c.label, value: c.value }))
    const bytes = await generateCatalogPdf({
      title: `车辆费用清单（${month.value}）`,
      columns: cols,
      rows: rows.value,
      metaLines: [`共 ${rows.value.length} 辆车 · 总费用 ${fmtMoney(totalAll.value)} 元`],
    })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes], { type: 'application/pdf' }))
    printCount.value = rows.value.length
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
</script>

<style lang="less" scoped>
.fleet-billing-container { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.fleet-billing-scroll { flex: 1; min-height: 0; }
.doc-empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 40px 0; color: var(--td-text-color-placeholder); }
</style>
