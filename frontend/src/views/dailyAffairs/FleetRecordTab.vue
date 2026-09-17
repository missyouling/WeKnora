<template>
  <div class="fleet-record-container">
    <!-- 筛选工具栏 -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-date-picker v-model="filters.month" mode="month" format="YYYY-MM" value-type="YYYY-MM" placeholder="月份"
            clearable class="doc-date-picker doc-filter-field__control" @change="loadRecords" />
        </div>
        <div class="doc-filter-field">
          <t-select v-model="filters.vehicle_id" :options="vehicleOptions" filterable clearable placeholder="全部车辆"
            class="doc-filter-select doc-filter-field__control" @change="loadRecords" />
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
        <t-button theme="primary" size="small" @click="openDrawer(null)">
          <template #icon><t-icon name="add" size="14px" /></template>
          新增记录
        </t-button>
      </div>
    </div>

    <!-- 列表 -->
    <div class="doc-list-scroll fleet-list-scroll" ref="listScrollRef">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <div class="cell cell-check" role="columnheader" @click.stop>
            <t-checkbox class="doc-list-check" size="small" :checked="isAllSelected" :indeterminate="someSelected"
              @change="toggleAll" />
          </div>
          <template v-for="col in visibleCols" :key="col.key">
            <div class="cell cell-body" role="columnheader" :title="col.label">{{ col.label }}</div>
          </template>
        </div>
        <div v-if="!loading && !rows.length" class="doc-empty">
          <t-icon name="search-error" size="40px" class="doc-empty-icon" />
          <span class="doc-empty-text">暂无{{ typeMeta.label }}记录</span>
        </div>
        <template v-for="row in rows" :key="row.id">
          <div class="doc-list-row" :style="gridStyle" role="row" :class="{ selected: selectedKeys.has(row.id) }"
            @click="toggleSelect(row)" @dblclick="openDrawer(row)">
            <div class="cell cell-check" @click.stop>
              <t-checkbox class="doc-list-check" size="small" :checked="selectedKeys.has(row.id)" @change="toggleSelect(row)" />
            </div>
            <template v-for="col in visibleCols" :key="col.key">
              <div class="cell cell-body" :title="String(col.value(row) ?? '')">{{ col.value(row) }}</div>
            </template>
          </div>
        </template>
      </div>
    </div>

    <!-- 底部汇总 -->
    <div v-if="rows.length" class="doc-summary-bar" :class="{ 'is-batch-visible': selectedKeys.size }">
      <span class="doc-summary-count">共 {{ selectedKeys.size ? selectedKeys.size : rows.length }} 条</span>
      <span class="doc-summary-item">总金额 <span class="doc-summary-val">{{ fmtMoney(summaryAmount) }}</span> 元</span>
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
            <t-button theme="default" variant="outline" size="small" :disabled="selectedRows.length !== 1" @click="openDrawer(selectedRows[0])">
              <template #icon><t-icon name="edit" size="14px" /></template>编辑
            </t-button>
            <t-button theme="default" variant="outline" size="small" :loading="catalogBusy" @click="handlePrint">
              <template #icon><t-icon name="print" size="14px" /></template>打印
            </t-button>
            <t-popconfirm theme="warning" :content="`确定删除所选 ${selectedKeys.size} 条记录吗？`"
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
        :close-on-overlay-click="true" destroy-on-close class="meter-record-drawer"
        @close="onDrawerClose" @update:visible="(v: boolean) => (v || onDrawerClose())">
        <div class="meter-drawer-body">
          <div class="rec-grid">
            <div class="rec-field">
              <label>车辆 <span class="required">*</span></label>
              <t-select v-model="form.vehicle_id" :options="vehicleOptions" filterable :placeholder="'选择车辆'" @change="onVehicleChange" />
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
                <label>{{ f.label }}{{ f.required ? ' <span class="required">*</span>' : '' }}</label>
                <t-select v-if="f.type === 'select'" v-model="form[f.key]" :options="f.options || []" filterable clearable :placeholder="f.placeholder || '选填'" />
                <t-input v-else-if="f.type === 'textarea'" v-model="form[f.key]" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" :placeholder="f.placeholder || '选填'" />
                <t-date-picker v-else-if="f.type === 'date'" v-model="form[f.key]" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable :placeholder="f.placeholder || '选填'" />
                <t-input v-else v-model.number="form[f.key]" :type="f.type === 'number' ? 'number' : 'text'" :placeholder="f.placeholder || '选填'" />
              </div>
            </template>
            <div class="rec-field rec-field--wide">
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
          <span class="meter-print-title">{{ typeMeta.label }}记录预览（{{ printCount }} 条）</span>
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
import { listFleetVehicles, listFleetRecords, createFleetRecord, updateFleetRecord, deleteFleetRecord } from '@/api/fleet'
import { generateCatalogPdf, type CatalogColumn } from './useCatalogPdf'

const props = defineProps<{ recordType: string }>()
const emit = defineEmits<{ (e: 'openSettings'): void }>()

// ---------------------------------------------------------------------------
// 类型元信息与字段定义
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

const TYPES: Record<string, { label: string; fields: FieldDef[] }> = {
  maintain: {
    label: '维保',
    fields: [
      { key: 'project', label: '维保项目', required: true, placeholder: '如：更换机油' },
      { key: 'amount', label: '费用（元）', type: 'number' },
      { key: 'mileage', label: '里程（km）', type: 'number' },
      { key: 'unit', label: '维保单位', placeholder: '选填' },
    ],
  },
  fuel: {
    label: '加油',
    fields: [
      { key: 'card_id', label: '油卡', type: 'select', options: [], placeholder: '选择油卡' },
      { key: 'fuel_qty', label: '加油量（L）', type: 'number' },
      { key: 'unit_price', label: '单价（元/L）', type: 'number' },
      { key: 'amount', label: '金额（元）', type: 'number' },
      { key: 'mileage', label: '里程（km）', type: 'number' },
      { key: 'station', label: '油站', placeholder: '选填' },
    ],
  },
  insurance: {
    label: '车险',
    fields: [
      { key: 'insurance_type', label: '险种', placeholder: '如：交强险' },
      { key: 'company', label: '保险公司', placeholder: '选填' },
      { key: 'policy_no', label: '保单号', placeholder: '选填' },
      { key: 'start_date', label: '起保日期', type: 'date' },
      { key: 'end_date', label: '到期日期', type: 'date' },
      { key: 'amount', label: '保费（元）', type: 'number' },
    ],
  },
  tire: {
    label: '轮胎',
    fields: [
      { key: 'brand', label: '品牌', placeholder: '选填' },
      { key: 'spec', label: '规格', placeholder: '如：225/55R17' },
      { key: 'count', label: '数量', type: 'number' },
      { key: 'unit_price', label: '单价（元）', type: 'number' },
      { key: 'amount', label: '金额（元）', type: 'number' },
      { key: 'mileage', label: '里程（km）', type: 'number' },
    ],
  },
  material: {
    label: '辅材',
    fields: [
      { key: 'material_type', label: '类型', type: 'select', required: true,
        options: [{ label: '尿素', value: '尿素' }, { label: '篷布', value: '篷布' }, { label: '其它', value: '其它' }] },
      { key: 'quantity', label: '数量', type: 'number' },
      { key: 'unit_price', label: '单价（元）', type: 'number' },
      { key: 'amount', label: '金额（元）', type: 'number' },
    ],
  },
  violation: {
    label: '违章',
    fields: [
      { key: 'driver_id', label: '驾驶员', type: 'select', options: [], placeholder: '选择驾驶员' },
      { key: 'violation_type', label: '违章类型', placeholder: '如：违停' },
      { key: 'location', label: '地点', placeholder: '选填' },
      { key: 'points', label: '扣分', type: 'number' },
      { key: 'amount', label: '罚款（元）', type: 'number' },
      { key: 'status', label: '处理状态', type: 'select',
        options: [{ label: '未处理', value: '未处理' }, { label: '已处理', value: '已处理' }] },
    ],
  },
  'car-request': {
    label: '请车',
    fields: [
      { key: 'applicant', label: '申请人', placeholder: '选填' },
      { key: 'reason', label: '事由', placeholder: '选填', wide: true },
      { key: 'start_time', label: '开始时间', type: 'date' },
      { key: 'end_time', label: '结束时间', type: 'date' },
      { key: 'approver', label: '审批人', placeholder: '选填' },
    ],
  },
  toll: {
    label: '通行费',
    fields: [
      { key: 'road_section', label: '路段', placeholder: '如：重庆-成都' },
      { key: 'amount', label: '金额（元）', type: 'number' },
      { key: 'pay_method', label: '支付方式', type: 'select',
        options: [{ label: 'ETC', value: 'ETC' }, { label: '现金', value: '现金' }, { label: '扫码', value: '扫码' }] },
    ],
  },
}

const typeMeta = computed(() => TYPES[props.recordType] || TYPES.maintain)

// ---------------------------------------------------------------------------
// 数据
// ---------------------------------------------------------------------------
const vehicles = ref<any[]>([])
const drivers = ref<any[]>([])
const cards = ref<any[]>([])
const rows = ref<any[]>([])
const loading = ref(false)
const filters = reactive({ month: '', vehicle_id: '' as any })

const vehicleOptions = computed(() => vehicles.value.map((v: any) => ({ label: v.plate_no, value: v.id })))
const driverOptions = computed(() => drivers.value.map((d: any) => ({ label: `${d.name}（${d.license_type || '—'}）`, value: d.id })))

function vehiclePlate(id: string) {
  return vehicles.value.find((v: any) => v.id === id)?.plate_no || '—'
}
function driverName(id: string) {
  return drivers.value.find((d: any) => d.id === id)?.name || '—'
}
function cardNo(id: string) {
  return cards.value.find((c: any) => c.id === id)?.card_no || '—'
}

async function loadBase() {
  try {
    const [vr, dr, cr] = await Promise.all([listFleetVehicles(), import('@/api/fleet').then(m => m.listFleetDrivers()), import('@/api/fleet').then(m => m.listFleetFuelCards())])
    vehicles.value = vr.data || []
    drivers.value = dr.data || []
    cards.value = cr.data || []
    // 加油油卡下拉选项
    const f = TYPES.fuel.fields.find(x => x.key === 'card_id')
    if (f) f.options = cards.value.map((c: any) => ({ label: c.card_no, value: c.id }))
    const v = TYPES.violation.fields.find(x => x.key === 'driver_id')
    if (v) v.options = drivers.value.map((d: any) => ({ label: `${d.name}（${d.license_type || '—'}）`, value: d.id }))
  } catch (e) {
    console.error('load fleet base failed', e)
  }
}

async function loadRecords() {
  loading.value = true
  try {
    const res = await listFleetRecords({ type: props.recordType, month: filters.month || undefined, vehicle_id: filters.vehicle_id || undefined })
    rows.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

watch(() => props.recordType, () => { loadRecords() })
onMounted(async () => {
  await loadBase()
  loadRecords()
})

// ---------------------------------------------------------------------------
// 列定义与字段选择器
// ---------------------------------------------------------------------------
const fmtMoney = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
const fmtNum = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })

const COL_DEFS: Record<string, { key: string; label: string; def: boolean; value: (r: any) => any }[]> = {
  maintain: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'project', label: '维保项目', def: true, value: (r) => r.data?.project || '—' },
    { key: 'mileage', label: '里程(km)', def: true, value: (r) => fmtNum(r.mileage) },
    { key: 'amount', label: '费用(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'unit', label: '维保单位', def: false, value: (r) => r.data?.unit || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  fuel: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'card', label: '油卡', def: true, value: (r) => cardNo(r.data?.card_id) },
    { key: 'fuel_qty', label: '加油量(L)', def: true, value: (r) => fmtNum(r.data?.fuel_qty) },
    { key: 'unit_price', label: '单价(元/L)', def: true, value: (r) => fmtNum(r.data?.unit_price) },
    { key: 'amount', label: '金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'mileage', label: '里程(km)', def: false, value: (r) => fmtNum(r.mileage) },
    { key: 'station', label: '油站', def: false, value: (r) => r.data?.station || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  insurance: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'insurance_type', label: '险种', def: true, value: (r) => r.data?.insurance_type || '—' },
    { key: 'company', label: '保险公司', def: true, value: (r) => r.data?.company || '—' },
    { key: 'policy_no', label: '保单号', def: true, value: (r) => r.data?.policy_no || '—' },
    { key: 'start_date', label: '起保日期', def: false, value: (r) => r.data?.start_date || '—' },
    { key: 'end_date', label: '到期日期', def: false, value: (r) => r.data?.end_date || '—' },
    { key: 'amount', label: '保费(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
  tire: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'brand', label: '品牌', def: true, value: (r) => r.data?.brand || '—' },
    { key: 'spec', label: '规格', def: true, value: (r) => r.data?.spec || '—' },
    { key: 'count', label: '数量', def: true, value: (r) => fmtNum(r.data?.count) },
    { key: 'unit_price', label: '单价(元)', def: false, value: (r) => fmtNum(r.data?.unit_price) },
    { key: 'amount', label: '金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'mileage', label: '里程(km)', def: false, value: (r) => fmtNum(r.mileage) },
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
  violation: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'driver', label: '驾驶员', def: true, value: (r) => driverName(r.data?.driver_id) },
    { key: 'violation_type', label: '违章类型', def: true, value: (r) => r.data?.violation_type || '—' },
    { key: 'location', label: '地点', def: true, value: (r) => r.data?.location || '—' },
    { key: 'points', label: '扣分', def: true, value: (r) => r.data?.points ?? '—' },
    { key: 'amount', label: '罚款(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'status', label: '处理状态', def: true, value: (r) => r.data?.status || '—' },
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
  toll: [
    { key: 'record_date', label: '日期', def: true, value: (r) => r.record_date || '—' },
    { key: 'vehicle', label: '车辆', def: true, value: (r) => vehiclePlate(r.vehicle_id) },
    { key: 'road_section', label: '路段', def: true, value: (r) => r.data?.road_section || '—' },
    { key: 'amount', label: '金额(元)', def: true, value: (r) => fmtMoney(r.amount) },
    { key: 'pay_method', label: '支付方式', def: true, value: (r) => r.data?.pay_method || '—' },
    { key: 'remark', label: '备注', def: false, value: (r) => r.remark || '—' },
  ],
}

const columnDefs = computed(() => COL_DEFS[props.recordType] || [])
const COL_KEY = `weknora-fleet-${props.recordType}-columns-v1`

const visibleKeys = ref<string[]>([])
const fieldPopupVisible = ref(false)

function persistColumns() {
  localStorage.setItem(COL_KEY, JSON.stringify(visibleKeys.value))
}
function resetColumns() {
  visibleKeys.value = columnDefs.value.filter((c: any) => c.def).map((c: any) => c.key)
  persistColumns()
}
function selectAllColumns() {
  visibleKeys.value = columnDefs.value.map((c: any) => c.key)
  persistColumns()
}
function initColumns() {
  const saved = localStorage.getItem(COL_KEY)
  if (saved) {
    try {
      const arr = JSON.parse(saved)
      if (Array.isArray(arr) && arr.length) {
        visibleKeys.value = columnDefs.value.map((c: any) => c.key).filter((k: string) => arr.includes(k))
        return
      }
    } catch { /* ignore */ }
  }
  resetColumns()
}
watch(() => props.recordType, () => initColumns())
onMounted(() => initColumns())

const visibleCols = computed(() => columnDefs.value.filter((c: any) => visibleKeys.value.includes(c.key)))
const gridStyle = computed(() => {
  const n = visibleCols.value.length + 1
  const check = '44px'
  const flexes = Array(visibleCols.value.length).fill('minmax(100px, 1fr)').join(' ')
  return { gridTemplateColumns: `${check} ${flexes}`, minWidth: `${44 + visibleCols.value.length * 100}px` }
})

// ---------------------------------------------------------------------------
// 选择 / 汇总
// ---------------------------------------------------------------------------
const selectedKeys = ref<Set<string>>(new Set())
const isAllSelected = computed(() => rows.value.length > 0 && rows.value.every((r) => selectedKeys.value.has(r.id)))
const someSelected = computed(() => rows.value.some((r) => selectedKeys.value.has(r.id)))
const selectedRows = computed(() => rows.value.filter((r) => selectedKeys.value.has(r.id)))
const summaryAmount = computed(() => (selectedKeys.value.size ? selectedRows.value : rows.value).reduce((s, r) => s + (Number(r.amount) || 0), 0))

function toggleSelect(row: any) {
  if (selectedKeys.value.has(row.id)) selectedKeys.value.delete(row.id)
  else selectedKeys.value.add(row.id)
  selectedKeys.value = new Set(selectedKeys.value)
}
function toggleAll(v: boolean) {
  selectedKeys.value = v ? new Set(rows.value.map((r) => r.id)) : new Set()
}
function clearSelection() { selectedKeys.value = new Set() }

// ---------------------------------------------------------------------------
// 新增/编辑
// ---------------------------------------------------------------------------
const drawerVisible = ref(false)
const editingId = ref('')
const form = reactive<any>({})
const saving = ref(false)
const drawerTitle = computed(() => (editingId.value ? `编辑${typeMeta.value.label}记录` : `新增${typeMeta.value.label}记录`))
const formFields = computed(() => typeMeta.value.fields)

function openDrawer(row: any) {
  editingId.value = row?.id || ''
  Object.keys(form).forEach((k) => delete form[k])
  if (row) {
    Object.assign(form, JSON.parse(JSON.stringify(row)))
    Object.assign(form, row.data || {})
  } else {
    form.vehicle_id = filters.vehicle_id || ''
    form.record_date = ''
    form.record_month = filters.month || ''
  }
  drawerVisible.value = true
  // 刷新车辆/驾驶员/油卡基础数据，保证下拉选项最新（含新配置）
  loadBase()
}
function onDrawerClose() { drawerVisible.value = false }

function onVehicleChange(v: string) {
  // 联动：请车记录可选填申请人默认驾驶员？保持简单，仅记录
  void v
}
function onDateChange(v: string) {
  if (v && v.length >= 7) form.record_month = v.slice(0, 7)
}

async function saveRecord() {
  if (!form.vehicle_id) { MessagePlugin.warning('请选择车辆'); return }
  const payload: Record<string, unknown> = {
    record_type: props.recordType,
    vehicle_id: form.vehicle_id,
    record_date: form.record_date || '',
    record_month: form.record_month || (form.record_date ? form.record_date.slice(0, 7) : ''),
    amount: Number(form.amount) || 0,
    mileage: Number(form.mileage) || 0,
    remark: form.remark || '',
    data: {},
  }
  typeMeta.value.fields.forEach((f) => {
    const v = form[f.key]
    if (v !== undefined && v !== null && v !== '') payload.data[f.key] = v
  })
  saving.value = true
  try {
    if (editingId.value) await updateFleetRecord(editingId.value, payload)
    else await createFleetRecord(payload)
    MessagePlugin.success('已保存')
    drawerVisible.value = false
    loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function handleDelete() {
  const ids = selectedRows.value.map((r) => r.id)
  try {
    for (const id of ids) await deleteFleetRecord(id)
    MessagePlugin.success('已删除')
    clearSelection()
    loadRecords()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---------------------------------------------------------------------------
// 打印
// ---------------------------------------------------------------------------
const printVisible = ref(false)
const printUrl = ref('')
const printCount = ref(0)
const catalogBusy = ref(false)

async function handlePrint() {
  const rowsToPrint = selectedRows.value
  if (!rowsToPrint.length) return
  catalogBusy.value = true
  try {
    const cols: CatalogColumn[] = columnDefs.value
      .filter((c: any) => visibleKeys.value.includes(c.key))
      .map((c: any) => ({ key: c.key, label: c.label, value: c.value }))
    if (!cols.some((c) => c.key === 'record_date')) {
      cols.unshift({ key: 'record_date', label: '日期', value: (r: any) => r.record_date || '—' })
    }
    const bytes = await generateCatalogPdf({
      title: `${typeMeta.value.label}记录目录`,
      columns: cols,
      rows: rowsToPrint,
      metaLines: [`共 ${rowsToPrint.length} 条`],
    })
    if (printUrl.value) URL.revokeObjectURL(printUrl.value)
    printUrl.value = URL.createObjectURL(new Blob([bytes], { type: 'application/pdf' }))
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

// ---------------------------------------------------------------------------
// 抽屉宽度拖动 + 持久化
// ---------------------------------------------------------------------------
const DRAWER_KEY = `weknora-fleet-${props.recordType}-drawer-width`
const drawerWidth = ref(`${Number(localStorage.getItem(DRAWER_KEY)) || 720}px`)
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
.fleet-record-container { display: flex; flex-direction: column; height: 100%; min-height: 0; }
.fleet-list-scroll { flex: 1; min-height: 0; }
.doc-empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 40px 0; color: var(--td-text-color-placeholder); }
</style>
