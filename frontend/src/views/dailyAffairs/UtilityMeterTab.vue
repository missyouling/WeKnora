<template>
  <div class="utility-meter-tab">
    <!-- 工具栏 -->
    <div class="doc-filter-bar">
      <div class="doc-filter-bar__leading">
        <div class="doc-filter-field">
          <t-date-picker v-model="month" placeholder="月份" format="YYYY-MM" value-type="YYYY-MM" clearable class="doc-date-picker doc-filter-field__control" @change="load" />
        </div>
        <t-button variant="outline" size="small" @click="load">
          <template #icon><t-icon name="refresh" size="14px" /></template>
        </t-button>
      </div>
      <div class="doc-filter-bar__trailing">
        <t-button theme="primary" size="small" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          新增{{ categoryLabel }}记录
        </t-button>
      </div>
    </div>

    <!-- 列表（自绘 grid，数据少时只包裹记录行，超出滚动） -->
    <div class="doc-list-scroll meter-list-scroll" ref="listScrollRef">
      <div class="doc-list-view">
        <div class="doc-list-header" :style="gridStyle" role="row">
          <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`" role="columnheader">
            {{ col.label }}
          </div>
          <div class="cell cell-actions" role="columnheader">操作</div>
        </div>
        <div class="doc-list-body">
          <div v-for="row in displayRows" :key="row.id" class="doc-list-row" :style="gridStyle" role="row" @click="openEdit(row)">
            <div v-for="col in visibleColDefs" :key="col.key" class="cell" :class="`cell-${col.key}`">
              <span v-if="col.key === 'month'" class="row-mono">{{ row.month }}</span>
              <span v-else-if="col.key === 'meter_count'" class="row-text">{{ row.meter_count }}</span>
              <span v-else-if="col.key === 'total_usage'" class="row-mono">{{ fmtNum(row.total_usage) }}</span>
              <span v-else-if="col.key === 'total_amount'" class="row-mono">{{ fmtMoney(row.total_amount) }}</span>
              <span v-else class="row-text" :title="String(row[col.key] ?? '')">{{ row[col.key] }}</span>
            </div>
            <div class="cell cell-actions" @click.stop>
              <t-dropdown :options="rowActions" @click="(e: any) => onRowAction(e, row)">
                <t-button variant="text" size="small">
                  <template #icon><t-icon name="more" size="16px" /></template>
                </t-button>
              </t-dropdown>
            </div>
          </div>
          <div v-if="!loading && !displayRows.length" class="meter-empty">
            <t-icon name="search-error" size="40px" class="meter-empty-icon" />
            <span class="meter-empty-text">暂无数据</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 编辑抽屉（v-if 挂载 + Teleport 到 body：仅打开时渲染，避免 t-tabs 面板内预渲染导致的误显示/无法关闭） -->
    <teleport to="body">
      <t-drawer
        v-if="drawerVisible"
        :visible="true"
        :header="drawerTitle"
        :size="drawerWidth"
        :footer="false"
        :close-on-overlay-click="true"
        @close="onDrawerClose"
        @update:visible="(v: boolean) => (drawerVisible = v)"
        @mousedown="onDrawerMouseDown"
        @mousemove="onDrawerMouseMove"
        @mouseup="onDrawerMouseUp"
      >
        <div class="meter-drawer-body">
          <div class="setting-row">
            <div class="setting-info">
              <label>月份 <span class="required">*</span></label>
              <p class="desc">格式 YYYY-MM，同类型同月份唯一</p>
            </div>
            <div class="setting-control">
              <t-date-picker v-model="form.month" format="YYYY-MM" value-type="YYYY-MM" placeholder="选择月份" />
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>表计明细</label>
              <p class="desc">用量 = 期末 − 期初，金额 = 用量 × 单价，自动汇总</p>
            </div>
            <div class="setting-control">
              <div v-for="(it, idx) in form.items" :key="idx" class="meter-item-row">
                <t-input v-model="it.meter_name" placeholder="表名" class="meter-name-input" />
                <t-input v-model="it.start_reading" type="number" placeholder="期初" class="meter-num-input" @change="recalc" />
                <t-input v-model="it.end_reading" type="number" placeholder="期末" class="meter-num-input" @change="recalc" />
                <t-input v-model="it.unit_price" type="number" placeholder="单价" class="meter-num-input" @change="recalc" />
                <span class="meter-calc">{{ fmtNum(usageOf(it)) }}</span>
                <span class="meter-calc">{{ fmtMoney(amountOf(it)) }}</span>
                <t-button variant="text" size="small" @click="removeItem(idx)">
                  <template #icon><t-icon name="delete" size="16px" /></template>
                </t-button>
              </div>
              <div class="meter-item-actions">
                <t-button variant="text" size="small" @click="addItem">
                  <template #icon><t-icon name="add" size="16px" /></template>
                  添加表计
                </t-button>
              </div>
              <div class="meter-totals">
                <span>表计 {{ form.items.length }} 个</span>
                <span>总用量 {{ fmtNum(formTotalUsage) }}</span>
                <span>总金额 {{ fmtMoney(formTotalAmount) }} 元</span>
              </div>
            </div>
          </div>

          <div class="setting-row">
            <div class="setting-info">
              <label>备注</label>
            </div>
            <div class="setting-control">
              <t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" />
            </div>
          </div>
        </div>

        <div class="meter-drawer-footer">
          <t-button variant="outline" size="small" @click="drawerVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="saving" @click="save">
            <template #icon><t-icon name="check" size="14px" /></template>
            保存
          </t-button>
        </div>
      </t-drawer>
    </teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeterRecords,
  createUtilityMeterRecord,
  updateUtilityMeterRecord,
  deleteUtilityMeterRecord,
  listUtilityFieldConfigs,
} from '@/api/knowledge-base'

const props = defineProps<{
  category: 'water' | 'gas'
}>()

const categoryLabel = computed(() => (props.category === 'water' ? '水费' : '气费'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : 'm³'))

interface ColDef { key: string; label: string; default: boolean; w: string }
const FALLBACK_COLS: ColDef[] = [
  { key: 'month', label: '月份', default: true, w: '1.2fr' },
  { key: 'meter_count', label: '表计数', default: true, w: '0.8fr' },
  { key: 'total_usage', label: '总用量', default: true, w: '1fr' },
  { key: 'total_amount', label: '总金额', default: true, w: '1fr' },
  { key: 'remark', label: '备注', default: true, w: '2fr' },
]
const STORAGE_KEY = computed(() => `weknora-utility-${props.category}-columns-v1`)

const columnDefs = ref<ColDef[]>(FALLBACK_COLS)
const visibleColKeys = ref<string[]>(loadStoredColumns())
const visibleColDefs = computed(() => columnDefs.value.filter(c => visibleColKeys.value.includes(c.key)))
const gridStyle = computed(() => ({
  gridTemplateColumns: `${visibleColDefs.value.map(c => c.w).join(' ')} 80px`,
}))

function loadStoredColumns(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY.value)
    if (raw) {
      const arr = JSON.parse(raw)
      if (Array.isArray(arr) && arr.length) return arr
    }
  } catch { /* ignore */ }
  return columnDefs.value.filter(c => c.default).map(c => c.key)
}

const loadFieldConfigs = async () => {
  try {
    const res: any = await listUtilityFieldConfigs(props.category)
    const list = res?.data || res
    if (Array.isArray(list) && list.length) {
      columnDefs.value = list.map((c: any) => ({
        key: c.field_key,
        label: c.label,
        default: !!c.default_visible,
        w: colWidth(c.field_key),
      }))
    }
  } catch { /* 字段配置加载失败用内置默认 */ }
}

function colWidth(key: string): string {
  if (key === 'month') return '1.2fr'
  if (key === 'meter_count') return '0.8fr'
  if (key === 'total_usage' || key === 'total_amount') return '1fr'
  return '2fr'
}

const rows = ref<any[]>([])
const displayRows = ref<any[]>([])
const loading = ref(true)
const month = ref<string | undefined>(undefined)
const listScrollRef = ref<HTMLElement>()

const load = async () => {
  loading.value = true
  try {
    const res: any = await listUtilityMeterRecords({ category: props.category, month: month.value || undefined })
    rows.value = (res?.data || res || []) as any[]
    displayRows.value = rows.value
  } catch (e: any) {
    MessagePlugin.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}


// ---- 抽屉 ----
const drawerVisible = ref(false)
const saving = ref(false)
const editingId = ref('')
const form = ref<{ month: string; remark: string; items: any[] }>({ month: '', remark: '', items: [] })
const drawerTitle = computed(() => (editingId.value ? '编辑' + categoryLabel.value + '记录' : '新增' + categoryLabel.value + '记录'))

// 抽屉宽度拖动记忆
const drawerWidth = ref<string>(loadDrawerWidth())
function loadDrawerWidth(): string {
  try { return localStorage.getItem('weknora-utility-drawer-width') || '640px' } catch { return '640px' }
}
let dragging = false
let startX = 0
let startW = 640
const onDrawerMouseDown = (e: MouseEvent) => {
  const rect = (e.target as HTMLElement).closest('.t-drawer__header')
  if (!rect) return
  const x = window.innerWidth - (e.clientX || 0)
  if (x > 0 && x < 20) {
    dragging = true
    startX = e.clientX
    startW = parseInt(drawerWidth.value) || 640
  }
}
const onDrawerMouseMove = (e: MouseEvent) => {
  if (!dragging) return
  const w = Math.min(1000, Math.max(480, startW + (startX - e.clientX)))
  drawerWidth.value = w + 'px'
}
const onDrawerMouseUp = () => {
  if (dragging) {
    dragging = false
    try { localStorage.setItem('weknora-utility-drawer-width', drawerWidth.value) } catch { /* ignore */ }
  }
}

const openCreate = () => {
  editingId.value = ''
  form.value = { month: '', remark: '', items: [{ meter_name: '', start_reading: 0, end_reading: 0, unit_price: 0, remark: '' }] }
  drawerVisible.value = true
}

const onDrawerClose = () => {
  drawerVisible.value = false
}

const openEdit = (row: any) => {
  editingId.value = row.id
  form.value = {
    month: row.month || '',
    remark: row.remark || '',
    items: (row.items || []).map((it: any) => ({
      meter_name: it.meter_name || '',
      start_reading: Number(it.start_reading) || 0,
      end_reading: Number(it.end_reading) || 0,
      unit_price: Number(it.unit_price) || 0,
      remark: it.remark || '',
    })),
  }
  if (!form.value.items.length) addItem()
  drawerVisible.value = true
}

const addItem = () => {
  form.value.items.push({ meter_name: '', start_reading: 0, end_reading: 0, unit_price: 0, remark: '' })
}
const removeItem = (idx: number) => {
  form.value.items.splice(idx, 1)
  recalc()
}

const num = (v: any) => Number(v) || 0
const usageOf = (it: any) => Math.round((num(it.end_reading) - num(it.start_reading)) * 100) / 100
const amountOf = (it: any) => Math.round(usageOf(it) * num(it.unit_price) * 100) / 100
const formTotalUsage = computed(() => Math.round(form.value.items.reduce((s, it) => s + usageOf(it), 0) * 100) / 100)
const formTotalAmount = computed(() => Math.round(form.value.items.reduce((s, it) => s + amountOf(it), 0) * 100) / 100)
const recalc = () => { /* 响应式自动重算 */ }

const save = async () => {
  if (!form.value.month) {
    MessagePlugin.warning('请选择月份')
    return
  }
  saving.value = true
  try {
    const payload: any = {
      category: props.category,
      month: form.value.month,
      remark: form.value.remark || '',
      items: form.value.items.map((it: any) => ({
        meter_name: it.meter_name || '',
        start_reading: num(it.start_reading),
        end_reading: num(it.end_reading),
        unit_price: num(it.unit_price),
        remark: it.remark || '',
      })),
    }
    if (editingId.value) {
      await updateUtilityMeterRecord(editingId.value, payload)
      MessagePlugin.success('已保存')
    } else {
      await createUtilityMeterRecord(payload)
      MessagePlugin.success('已新增')
    }
    drawerVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 行操作
const rowActions = computed(() => [
  { content: '编辑', value: 'edit' },
  { content: '删除', value: 'delete' },
])
const onRowAction = async (e: any, row: any) => {
  if (e.value === 'edit') {
    openEdit(row)
  } else if (e.value === 'delete') {
    const confirm = await MessagePlugin.confirm(`确认删除 ${row.month} 记录？`, { confirmBtn: '删除', cancelBtn: '取消' })
    if (confirm) {
      await deleteUtilityMeterRecord(row.id)
      MessagePlugin.success('已删除')
      await load()
    }
  }
}

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtMoney = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}

onMounted(() => {
  loadFieldConfigs()
  load()
})
</script>

<style lang="less" scoped>
.utility-meter-tab {
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

    &--search {
      min-width: 220px;
    }

    .doc-search {
      width: 220px;
    }

    .doc-date-picker {
      width: 140px;
    }
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

.cell-actions {
  justify-content: flex-end;
}

.row-mono,
.row-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-mono {
  font-family: var(--app-font-family);
}

.meter-list-scroll {
  flex: 0 1 auto;
  max-height: calc(100vh - 320px);
  min-width: 0;
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

.meter-drawer-body {
  padding: 4px 0 24px;
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

.meter-item-row {
  display: grid;
  grid-template-columns: 1.4fr 0.9fr 0.9fr 0.9fr 0.9fr 1fr auto;
  gap: 6px;
  align-items: center;
  margin-bottom: 6px;

  .meter-name-input,
  .meter-num-input {
    width: 100%;
  }

  .meter-calc {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    text-align: right;
    white-space: nowrap;
  }
}

.meter-item-actions {
  margin-top: 4px;
}

.meter-totals {
  display: flex;
  gap: 20px;
  margin-top: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--td-bg-color-container-hover);
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.meter-drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}
</style>
