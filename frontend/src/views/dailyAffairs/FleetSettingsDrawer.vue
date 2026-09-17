<template>
  <div class="fld-wrap">
    <teleport to="body">
      <div v-if="visible" class="fld-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
        :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
        <div class="fld-resize-line" />
      </div>
    </teleport>
    <t-drawer v-if="visible" :visible="true" :header="'设置 · 车队管理'" :size="`${drawerWidth}px`" :footer="false"
      :close-on-overlay-click="true" destroy-on-close class="fleet-settings-drawer"
      @close="onClose" @update:visible="(v: boolean) => (v || onClose())">
      <div class="fld-body">
        <t-tabs v-model="activeTab" class="fld-tabs">
          <t-tab-panel value="vehicle" :label="`车辆管理 ${vehicles.length}`" />
          <t-tab-panel value="driver" :label="`驾驶员管理 ${drivers.length}`" />
          <t-tab-panel value="card" :label="`油卡管理 ${cards.length}`" />
        </t-tabs>

        <!-- ==================== 车辆管理 ==================== -->
        <div v-show="activeTab === 'vehicle'" class="fld-panel">
          <div class="fld-head">
            <t-button theme="primary" size="small" @click="openForm('vehicle', null)">
              <template #icon><t-icon name="add" /></template>新增车辆
            </t-button>
            <t-input v-model="vehicleSearch" placeholder="搜索车牌 / 责任人" clearable class="fld-search">
              <template #prefix-icon><t-icon name="search" size="14px" /></template>
            </t-input>
          </div>
          <div v-if="formVisible && !form.id && form._kind === 'vehicle'" class="fld-form fld-form--inline">
            <div class="fld-form-title">新增车辆</div>
            <div class="fld-grid">
              <div class="fld-item"><label>车牌号 <span class="req">*</span></label><t-input v-model="form.plate_no" placeholder="如：渝A12345" /></div>
              <div class="fld-item"><label>车辆类型</label><t-select v-model="form.vehicle_type" :options="vehicleTypeOptions" placeholder="选择类型" /></div>
              <div class="fld-item"><label>品牌型号</label><t-input v-model="form.brand_model" placeholder="选填" /></div>
              <div class="fld-item"><label>吨位</label><t-input v-model.number="form.load_tonnage" type="number" placeholder="选填" /></div>
              <div class="fld-item"><label>座位数</label><t-input v-model.number="form.seat_count" type="number" placeholder="选填" /></div>
              <div class="fld-item"><label>购置日期</label><t-date-picker v-model="form.purchase_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable placeholder="选填" /></div>
              <div class="fld-item"><label>使用部门</label><t-input v-model="form.department" placeholder="选填" /></div>
              <div class="fld-item"><label>责任人</label><t-input v-model="form.manager" placeholder="选填" /></div>
              <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
            </div>
            <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
            <div class="fld-form-actions">
              <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
              <t-button theme="primary" size="small" :loading="saving" @click="saveItem('vehicle')">保存</t-button>
            </div>
          </div>
          <div v-if="vehicleList.length" :ref="setSortable('vehicle')" class="fld-table">
            <div class="fld-table-head">
              <span class="fld-drag"><t-icon name="move" size="14px" /></span>
              <span>车牌号</span><span>类型</span><span>品牌型号</span><span>使用部门</span><span>责任人</span><span>状态</span><span>操作</span>
            </div>
            <template v-for="v in vehicleList" :key="v.id">
              <div class="fld-row" :data-id="v.id" :class="{ editing: formVisible && form.id === v.id }" @click="toggleEdit('vehicle', v)">
                <span class="fld-drag-handle" title="拖动排序" @click.stop><t-icon name="move" size="14px" /></span>
                <span class="fld-plate">{{ v.plate_no || '—' }}</span>
                <span>{{ v.vehicle_type || '—' }}</span>
                <span>{{ v.brand_model || '—' }}</span>
                <span>{{ v.department || '—' }}</span>
                <span>{{ v.manager || '—' }}</span>
                <span class="fld-switch" @click.stop>
                  <t-switch :model-value="!!v.enabled" size="small" @change="(val: any) => toggleEnabled('vehicle', v, val)" />
                </span>
                <span class="fld-row-actions" @click.stop>
                  <t-popconfirm theme="warning" :content="`确定删除车辆「${v.plate_no}」吗？`"
                    :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="removeItem('vehicle', v)">
                    <t-button variant="text" size="small" @click.stop>
                      <template #icon><t-icon name="delete" size="15px" /></template>
                    </t-button>
                  </t-popconfirm>
                </span>
              </div>
              <div v-if="formVisible && form.id === v.id" class="fld-form fld-form--inline">
                <div class="fld-form-title">编辑车辆</div>
                <div class="fld-grid">
                  <div class="fld-item"><label>车牌号 <span class="req">*</span></label><t-input v-model="form.plate_no" placeholder="如：渝A12345" /></div>
                  <div class="fld-item"><label>车辆类型</label><t-select v-model="form.vehicle_type" :options="vehicleTypeOptions" placeholder="选择类型" /></div>
                  <div class="fld-item"><label>品牌型号</label><t-input v-model="form.brand_model" placeholder="选填" /></div>
                  <div class="fld-item"><label>吨位</label><t-input v-model.number="form.load_tonnage" type="number" placeholder="选填" /></div>
                  <div class="fld-item"><label>座位数</label><t-input v-model.number="form.seat_count" type="number" placeholder="选填" /></div>
                  <div class="fld-item"><label>购置日期</label><t-date-picker v-model="form.purchase_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable placeholder="选填" /></div>
                  <div class="fld-item"><label>使用部门</label><t-input v-model="form.department" placeholder="选填" /></div>
                  <div class="fld-item"><label>责任人</label><t-input v-model="form.manager" placeholder="选填" /></div>
                  <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
                </div>
                <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
                <div class="fld-form-actions">
                  <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
                  <t-button theme="primary" size="small" :loading="saving" @click="saveItem('vehicle')">保存</t-button>
                </div>
              </div>
            </template>
          </div>
          <div v-if="!vehicleList.length" class="fld-empty"><t-icon name="car" size="40px" /><span>暂无车辆，点击新增车辆开始配置</span></div>
        </div>

        <!-- ==================== 驾驶员管理 ==================== -->
        <div v-show="activeTab === 'driver'" class="fld-panel">
          <div class="fld-head">
            <t-button theme="primary" size="small" @click="openForm('driver', null)">
              <template #icon><t-icon name="add" /></template>新增驾驶员
            </t-button>
            <t-input v-model="driverSearch" placeholder="搜索姓名 / 驾驶证号" clearable class="fld-search">
              <template #prefix-icon><t-icon name="search" size="14px" /></template>
            </t-input>
          </div>
          <div v-if="formVisible && !form.id && form._kind === 'driver'" class="fld-form fld-form--inline">
            <div class="fld-form-title">新增驾驶员</div>
            <div class="fld-grid">
              <div class="fld-item"><label>姓名 <span class="req">*</span></label><t-input v-model="form.name" placeholder="姓名" /></div>
              <div class="fld-item"><label>驾驶证号</label><t-input v-model="form.license_no" placeholder="选填" /></div>
              <div class="fld-item"><label>准驾车型</label><t-input v-model="form.license_type" placeholder="如：C1" /></div>
              <div class="fld-item"><label>联系电话</label><t-input v-model="form.phone" placeholder="选填" /></div>
              <div class="fld-item"><label>入职日期</label><t-date-picker v-model="form.hire_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable placeholder="选填" /></div>
              <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
            </div>
            <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
            <div class="fld-form-actions">
              <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
              <t-button theme="primary" size="small" :loading="saving" @click="saveItem('driver')">保存</t-button>
            </div>
          </div>
          <div v-if="driverList.length" :ref="setSortable('driver')" class="fld-table">
            <div class="fld-table-head">
              <span class="fld-drag"><t-icon name="move" size="14px" /></span>
              <span>姓名</span><span>驾驶证号</span><span>准驾车型</span><span>联系电话</span><span>入职日期</span><span>状态</span><span>操作</span>
            </div>
            <template v-for="d in driverList" :key="d.id">
              <div class="fld-row" :data-id="d.id" :class="{ editing: formVisible && form.id === d.id }" @click="toggleEdit('driver', d)">
                <span class="fld-drag-handle" title="拖动排序" @click.stop><t-icon name="move" size="14px" /></span>
                <span class="fld-plate">{{ d.name || '—' }}</span>
                <span>{{ d.license_no || '—' }}</span>
                <span>{{ d.license_type || '—' }}</span>
                <span>{{ d.phone || '—' }}</span>
                <span>{{ d.hire_date || '—' }}</span>
                <span class="fld-switch" @click.stop>
                  <t-switch :model-value="!!d.enabled" size="small" @change="(val: any) => toggleEnabled('driver', d, val)" />
                </span>
                <span class="fld-row-actions" @click.stop>
                  <t-popconfirm theme="warning" :content="`确定删除驾驶员「${d.name}」吗？`"
                    :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="removeItem('driver', d)">
                    <t-button variant="text" size="small" @click.stop>
                      <template #icon><t-icon name="delete" size="15px" /></template>
                    </t-button>
                  </t-popconfirm>
                </span>
              </div>
              <div v-if="formVisible && form.id === d.id" class="fld-form fld-form--inline">
                <div class="fld-form-title">编辑驾驶员</div>
                <div class="fld-grid">
                  <div class="fld-item"><label>姓名 <span class="req">*</span></label><t-input v-model="form.name" placeholder="姓名" /></div>
                  <div class="fld-item"><label>驾驶证号</label><t-input v-model="form.license_no" placeholder="选填" /></div>
                  <div class="fld-item"><label>准驾车型</label><t-input v-model="form.license_type" placeholder="如：C1" /></div>
                  <div class="fld-item"><label>联系电话</label><t-input v-model="form.phone" placeholder="选填" /></div>
                  <div class="fld-item"><label>入职日期</label><t-date-picker v-model="form.hire_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" clearable placeholder="选填" /></div>
                  <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
                </div>
                <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
                <div class="fld-form-actions">
                  <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
                  <t-button theme="primary" size="small" :loading="saving" @click="saveItem('driver')">保存</t-button>
                </div>
              </div>
            </template>
          </div>
          <div v-if="!driverList.length" class="fld-empty"><t-icon name="user" size="40px" /><span>暂无驾驶员，点击新增驾驶员开始配置</span></div>
        </div>

        <!-- ==================== 油卡管理 ==================== -->
        <div v-show="activeTab === 'card'" class="fld-panel">
          <div class="fld-head">
            <t-button theme="primary" size="small" @click="openForm('card', null)">
              <template #icon><t-icon name="add" /></template>新增油卡
            </t-button>
            <t-input v-model="cardSearch" placeholder="搜索卡号 / 油站" clearable class="fld-search">
              <template #prefix-icon><t-icon name="search" size="14px" /></template>
            </t-input>
          </div>
          <div v-if="formVisible && !form.id && form._kind === 'card'" class="fld-form fld-form--inline">
            <div class="fld-form-title">新增油卡</div>
            <div class="fld-grid">
              <div class="fld-item"><label>卡号 <span class="req">*</span></label><t-input v-model="form.card_no" placeholder="卡号" /></div>
              <div class="fld-item"><label>所属车辆</label><t-select v-model="form.vehicle_id" :options="vehicleOptions" filterable clearable placeholder="选择车辆" /></div>
              <div class="fld-item"><label>所属驾驶员</label><t-select v-model="form.driver_id" :options="driverOptions" filterable clearable placeholder="选择驾驶员" /></div>
              <div class="fld-item"><label>油站</label><t-input v-model="form.station" placeholder="选填" /></div>
              <div class="fld-item"><label>面额</label><t-input v-model.number="form.face_value" type="number" placeholder="选填" /></div>
              <div class="fld-item"><label>余额</label><t-input v-model.number="form.balance" type="number" placeholder="选填" /></div>
              <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
            </div>
            <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
            <div class="fld-form-actions">
              <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
              <t-button theme="primary" size="small" :loading="saving" @click="saveItem('card')">保存</t-button>
            </div>
          </div>
          <div v-if="cardList.length" class="fld-table">
            <div class="fld-table-head">
              <span class="fld-drag"><t-icon name="move" size="14px" /></span>
              <span>卡号</span><span>所属车辆</span><span>所属驾驶员</span><span>油站</span><span>面额</span><span>余额</span><span>状态</span><span>操作</span>
            </div>
            <template v-for="c in cardList" :key="c.id">
              <div class="fld-row" :data-id="c.id" :class="{ editing: formVisible && form.id === c.id }" @click="toggleEdit('card', c)">
                <span class="fld-drag-handle" @click.stop><t-icon name="move" size="14px" /></span>
                <span class="fld-plate">{{ c.card_no || '—' }}</span>
                <span>{{ vehicleName(c.vehicle_id) }}</span>
                <span>{{ driverName(c.driver_id) }}</span>
                <span>{{ c.station || '—' }}</span>
                <span class="fld-mono">{{ fmtMoney(c.face_value) }}</span>
                <span class="fld-mono">{{ fmtMoney(c.balance) }}</span>
                <span class="fld-switch" @click.stop>
                  <t-switch :model-value="!!c.enabled" size="small" @change="(val: any) => toggleEnabled('card', c, val)" />
                </span>
                <span class="fld-row-actions" @click.stop>
                  <t-popconfirm theme="warning" :content="`确定删除油卡「${c.card_no}」吗？`"
                    :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="removeItem('card', c)">
                    <t-button variant="text" size="small" @click.stop>
                      <template #icon><t-icon name="delete" size="15px" /></template>
                    </t-button>
                  </t-popconfirm>
                </span>
              </div>
              <div v-if="formVisible && form.id === c.id" class="fld-form fld-form--inline">
                <div class="fld-form-title">编辑油卡</div>
                <div class="fld-grid">
                  <div class="fld-item"><label>卡号 <span class="req">*</span></label><t-input v-model="form.card_no" placeholder="卡号" /></div>
                  <div class="fld-item"><label>所属车辆</label><t-select v-model="form.vehicle_id" :options="vehicleOptions" filterable clearable placeholder="选择车辆" /></div>
                  <div class="fld-item"><label>所属驾驶员</label><t-select v-model="form.driver_id" :options="driverOptions" filterable clearable placeholder="选择驾驶员" /></div>
                  <div class="fld-item"><label>油站</label><t-input v-model="form.station" placeholder="选填" /></div>
                  <div class="fld-item"><label>面额</label><t-input v-model.number="form.face_value" type="number" placeholder="选填" /></div>
                  <div class="fld-item"><label>余额</label><t-input v-model.number="form.balance" type="number" placeholder="选填" /></div>
                  <div class="fld-item"><label>启用</label><t-switch v-model="form.enabled" /></div>
                </div>
                <div class="fld-item fld-item--full"><label>备注</label><t-textarea v-model="form.remark" :maxlength="500" placeholder="选填" /></div>
                <div class="fld-form-actions">
                  <t-button variant="outline" size="small" @click="formVisible = false">取消</t-button>
                  <t-button theme="primary" size="small" :loading="saving" @click="saveItem('card')">保存</t-button>
                </div>
              </div>
            </template>
          </div>
          <div v-if="!cardList.length" class="fld-empty"><t-icon name="credit-card" size="40px" /><span>暂无油卡，点击新增油卡开始配置</span></div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import Sortable from 'sortablejs'
import {
  listFleetVehicles, createFleetVehicle, updateFleetVehicle, deleteFleetVehicle, sortFleetVehicles,
  listFleetDrivers, createFleetDriver, updateFleetDriver, deleteFleetDriver, sortFleetDrivers,
  listFleetFuelCards, createFleetFuelCard, updateFleetFuelCard, deleteFleetFuelCard,
} from '@/api/fleet'

const props = defineProps<{ visible: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const activeTab = ref('vehicle')
const vehicles = ref<any[]>([])
const drivers = ref<any[]>([])
const cards = ref<any[]>([])
const vehicleSearch = ref('')
const driverSearch = ref('')
const cardSearch = ref('')
const formVisible = ref(false)
const form = reactive<any>({})
const saving = ref(false)

const vehicleTypeOptions = [
  { label: '轿车', value: '轿车' }, { label: 'SUV', value: 'SUV' },
  { label: '货车', value: '货车' }, { label: '客车', value: '客车' },
  { label: '皮卡', value: '皮卡' }, { label: '面包车', value: '面包车' },
  { label: '其它', value: '其它' },
]

const vehicleList = computed(() => {
  const q = vehicleSearch.value.trim()
  if (!q) return vehicles.value
  return vehicles.value.filter((v: any) => (v.plate_no || '').includes(q) || (v.manager || '').includes(q))
})
const driverList = computed(() => {
  const q = driverSearch.value.trim()
  if (!q) return drivers.value
  return drivers.value.filter((d: any) => (d.name || '').includes(q) || (d.license_no || '').includes(q))
})
const cardList = computed(() => {
  const q = cardSearch.value.trim()
  if (!q) return cards.value
  return cards.value.filter((c: any) => (c.card_no || '').includes(q) || (c.station || '').includes(q))
})

const vehicleOptions = computed(() => vehicles.value.filter((v: any) => v.enabled !== false)
  .map((v: any) => ({ label: v.plate_no, value: v.id })))
const driverOptions = computed(() => drivers.value.filter((d: any) => d.enabled !== false)
  .map((d: any) => ({ label: `${d.name}${d.license_type ? `（${d.license_type}）` : ''}`, value: d.id })))

const vehicleName = (id: string) => vehicles.value.find((v: any) => v.id === id)?.plate_no || '—'
const driverName = (id: string) => drivers.value.find((d: any) => d.id === id)?.name || '—'
const fmtMoney = (v: any) => (v === null || v === undefined || v === '') ? '—' : Number(v).toLocaleString('zh-CN', { maximumFractionDigits: 2 })

// ---------- 加载 ----------
async function loadAll() {
  try {
    const [vr, dr, cr] = await Promise.all([listFleetVehicles(), listFleetDrivers(), listFleetFuelCards()])
    vehicles.value = vr.data || []
    drivers.value = dr.data || []
    cards.value = cr.data || []
  } catch (e) {
    console.error('load fleet settings failed', e)
  }
}

watch(() => props.visible, (v) => {
  if (v) {
    loadAll()
  } else {
    formVisible.value = false
  }
})
onMounted(() => { if (props.visible) loadAll() })

const onClose = () => emit('update:visible', false)

// ---------- 表单 ----------
function openForm(kind: string, item: any) {
  formVisible.value = true
  Object.keys(form).forEach((k) => delete form[k])
  if (item) {
    Object.assign(form, JSON.parse(JSON.stringify(item)))
  } else if (kind === 'vehicle') {
    Object.assign(form, { enabled: true, vehicle_type: '轿车' })
  } else if (kind === 'driver') {
    Object.assign(form, { enabled: true })
  } else {
    Object.assign(form, { enabled: true })
  }
  form._kind = kind
}

function toggleEdit(kind: string, item: any) {
  if (formVisible.value && form.id === item.id) {
    formVisible.value = false
    return
  }
  openForm(kind, item)
}

async function saveItem(kind: string) {
  const payload: Record<string, unknown> = { ...form }
  delete payload._kind
  saving.value = true
  try {
    if (kind === 'vehicle') {
      if (!payload.plate_no) { MessagePlugin.warning('车牌号不能为空'); return }
      if (form.id) await updateFleetVehicle(form.id, payload)
      else await createFleetVehicle(payload)
    } else if (kind === 'driver') {
      if (!payload.name) { MessagePlugin.warning('姓名不能为空'); return }
      if (form.id) await updateFleetDriver(form.id, payload)
      else await createFleetDriver(payload)
    } else {
      if (!payload.card_no) { MessagePlugin.warning('卡号不能为空'); return }
      if (form.id) await updateFleetFuelCard(form.id, payload)
      else await createFleetFuelCard(payload)
    }
    MessagePlugin.success('已保存')
    formVisible.value = false
    await loadAll()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(kind: string, item: any, val: boolean) {
  try {
    if (kind === 'vehicle') await updateFleetVehicle(item.id, { ...item, enabled: val })
    else if (kind === 'driver') await updateFleetDriver(item.id, { ...item, enabled: val })
    else await updateFleetFuelCard(item.id, { ...item, enabled: val })
    item.enabled = val
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}

async function removeItem(kind: string, item: any) {
  try {
    if (kind === 'vehicle') await deleteFleetVehicle(item.id)
    else if (kind === 'driver') await deleteFleetDriver(item.id)
    else await deleteFleetFuelCard(item.id)
    MessagePlugin.success('已删除')
    await loadAll()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---------- 拖拽排序 ----------
const sortableRefs: Record<string, any> = {}
function setSortable(kind: string) {
  return (el: any) => {
    if (!el) return
    sortableRefs[kind] = el
  }
}

let sortableInstances: Record<string, Sortable> = {}
function initSortables() {
  Object.keys(sortableRefs).forEach((k) => {
    const el = sortableRefs[k]
    if (!el) return
    if (sortableInstances[k]) sortableInstances[k].destroy()
    sortableInstances[k] = new Sortable(el, {
      handle: '.fld-drag-handle',
      animation: 150,
      onEnd: async () => {
        const ids: string[] = [...el.querySelectorAll('.fld-row')].map((r: any) => r.dataset.id)
        try {
          if (k === 'vehicle') await sortFleetVehicles(ids)
          else await sortFleetDrivers(ids)
          await loadAll()
        } catch (e: any) {
          MessagePlugin.error(e?.message || '排序保存失败')
        }
      },
    })
  })
}

watch(activeTab, () => { setTimeout(() => initSortables(), 60) })
watch(() => props.visible, (v) => {
  if (v) setTimeout(() => initSortables(), 200)
})
onBeforeUnmount(() => {
  Object.values(sortableInstances).forEach((s) => s?.destroy())
  sortableInstances = {}
})

// ---------- 抽屉宽度拖动 + 持久化 ----------
const DRAWER_KEY = 'weknora-fleet-drawer-width'
const drawerWidth = ref(Number(localStorage.getItem(DRAWER_KEY)) || 880)
let resizing = false
let startX = 0
let startW = 0

function onResizeStart(e: MouseEvent) {
  resizing = true
  startX = e.clientX
  startW = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
function onResizeMove(e: MouseEvent) {
  if (!resizing) return
  const w = Math.min(1400, Math.max(640, startW + (startX - e.clientX)))
  drawerWidth.value = w
}
function onResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
  localStorage.setItem(DRAWER_KEY, String(drawerWidth.value))
}
</script>

<style lang="less" scoped>
.fld-wrap { display: contents; }
.fld-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 6px;
  cursor: col-resize;
  z-index: 3100;
  .fld-resize-line {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 2px;
    width: 2px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity 0.15s;
  }
  &:hover .fld-resize-line, &.active .fld-resize-line { opacity: 0.6; }
}
.fld-body { display: flex; flex-direction: column; height: 100%; overflow: hidden; }
.fld-tabs { margin-bottom: 12px; }
.fld-panel { flex: 1; min-height: 0; overflow-y: auto; padding-right: 4px; }
.fld-head { display: flex; gap: 8px; align-items: center; margin-bottom: 12px;
  .fld-search { flex: 1; max-width: 320px; }
}
.fld-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
  .fld-table-head, .fld-row {
    display: grid;
    grid-template-columns: 28px 1.2fr 0.9fr 1.4fr 1fr 1fr 0.7fr 56px;
    align-items: center;
    font-size: 13px;
  }
  .fld-table-head {
    background: var(--td-bg-color-component);
    color: var(--td-text-color-secondary);
    font-weight: 500;
    padding: 8px 10px;
  }
  .fld-row {
    padding: 7px 10px;
    border-top: 1px solid var(--td-component-stroke);
    cursor: pointer;
    transition: background 0.15s;
    &:hover { background: var(--td-bg-color-container-hover); }
    &.editing { background: var(--td-brand-color-light); }
    .fld-plate { font-weight: 600; color: var(--td-brand-color); }
    .fld-mono { font-variant-numeric: tabular-nums; }
  }
}
.fld-drag-handle { cursor: grab; color: var(--td-text-color-placeholder); display: flex; }
.fld-switch { display: flex; }
.fld-row-actions { display: flex; gap: 2px; }
.fld-form {
  border: 1px solid var(--td-brand-color);
  border-radius: 8px;
  padding: 12px;
  margin-top: 6px;
  background: var(--td-bg-color-container);
  .fld-form-title { font-size: 13px; font-weight: 600; margin-bottom: 10px; color: var(--td-text-color-primary); }
  .fld-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px 12px; }
  .fld-item { display: flex; flex-direction: column; gap: 4px;
    label { font-size: 12px; color: var(--td-text-color-secondary); .req { color: var(--td-error-color); } }
    &--full { grid-column: 1 / -1; }
  }
  .fld-form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
}
.fld-empty { display: flex; flex-direction: column; align-items: center; gap: 10px; color: var(--td-text-color-placeholder); padding: 40px 0; }
</style>
