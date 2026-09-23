<template>
  <teleport to="body">
    <div v-if="visible" class="doc-drawer-resize-handle" :style="{ right: drawerWidth }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onDrawerResizeStart">
      <div class="doc-drawer-resize-line" />
    </div>
  </teleport>
  <t-drawer :visible="visible" :size="drawerWidth" :footer="false" :close-btn="true"
    :close-on-overlay-click="true" :esc-close="true" destroy-on-close
    class="fleet-settings-drawer meter-record-drawer" @close="emit('update:visible', false)"
    @update:visible="(v: boolean) => emit('update:visible', v)">
    <template #header>
      <div class="fleet-drawer-header">
        <span class="fleet-drawer-title">{{ drawerTitle }}</span>

      </div>
    </template>
    <div class="fleet-settings-body">
      <!-- 按当前菜单动态显示对应配置面板（车辆档案：证照配置 + 提取规则） -->
      <template v-if="panelKey === 'cert-vehicle'">
        <t-tabs v-model="vehicleTab" class="fleet-settings-tabs" @change="onVehicleTabChange">
          <t-tab-panel value="cert" label="证照配置">
            <CertPanel ref="vehicleCertRef" scope="vehicle" />
          </t-tab-panel>
          <t-tab-panel value="extract" label="提取规则">
            <ExtractRulePanel ref="extractPanelRef" scope="vehicle" />
          </t-tab-panel>
        </t-tabs>
      </template>
      <template v-else-if="panelKey === 'cert-driver'">
        <t-tabs v-model="driverTab" class="fleet-settings-tabs" @change="onDriverTabChange">
          <t-tab-panel value="cert" label="证照配置">
            <CertPanel ref="driverCertRef" scope="driver" />
          </t-tab-panel>
          <t-tab-panel value="extract" label="提取规则">
            <ExtractRulePanel ref="driverExtractRef" scope="driver" />
          </t-tab-panel>
        </t-tabs>
      </template>
      <template v-else-if="panelKey === 'cert-maintain'">
        <t-tabs v-model="maintainTab" class="fleet-settings-tabs" @change="onMaintainTabChange">
          <t-tab-panel value="cert" label="维保配置">
            <CertPanel ref="maintainCertRef" scope="maintain" />
          </t-tab-panel>
          <t-tab-panel value="extract" label="提取规则">
            <ExtractRulePanel ref="maintainExtractRef" scope="maintain" />
          </t-tab-panel>
        </t-tabs>
      </template>
      <template v-else-if="panelKey === 'oil'">
        <OilCardPanel />
      </template>
      <template v-else-if="panelKey === 'etc'">
        <EtcPanel />
      </template>
      <template v-else-if="panelKey === 'supplier'">
        <SupplierPanel />
      </template>
      <!-- 兜底：未映射菜单保留全量设置入口 -->
      <t-tabs v-else v-model="activeTab" class="fleet-settings-tabs">
        <t-tab-panel value="vehicle" label="车辆档案">
          <CertPanel ref="vehicleCertRef" scope="vehicle" />
        </t-tab-panel>
        <t-tab-panel value="driver" label="司机档案">
          <CertPanel ref="driverCertRef" scope="driver" />
        </t-tab-panel>
        <t-tab-panel value="maintain" label="维保管理">
          <CertPanel ref="maintainCertRef" scope="maintain" />
        </t-tab-panel>
        <t-tab-panel value="fuel" label="油卡管理">
          <OilCardPanel />
        </t-tab-panel>
        <t-tab-panel value="supplier" label="供应商">
          <SupplierPanel />
        </t-tab-panel>
        <t-tab-panel value="etc" label="ETC管理">
          <EtcPanel />
        </t-tab-panel>
      </t-tabs>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import CertPanel from './FleetCertPanel.vue'
import ExtractRulePanel from './ExtractRulePanel.vue'
import OilCardPanel from './FleetOilCardPanel.vue'
import SupplierPanel from './FleetSupplierPanel.vue'
import EtcPanel from './FleetEtcPanel.vue'

const props = defineProps<{ visible: boolean; menuKey?: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

// 菜单 → 配置面板映射（车辆档案只保留车辆档案配置，删除其他设置项）
const PANEL_MAP: Record<string, { key: string; title: string }> = {
  'vehicle-archive': { key: 'cert-vehicle', title: '车辆档案配置' },
  'driver-archive': { key: 'cert-driver', title: '司机档案配置' },
  'maintain-archive': { key: 'cert-maintain', title: '维保记录配置' },
  'fuel-charge': { key: 'oil', title: '油卡管理' },
  'road-toll': { key: 'etc', title: 'ETC管理' },
  'repair-cost': { key: 'supplier', title: '供应商' },
}
const panelKey = computed(() => PANEL_MAP[props.menuKey || '']?.key || '')
const drawerTitle = computed(() => PANEL_MAP[props.menuKey || '']?.title || '设置')

const activeTab = ref('vehicle')
const vehicleTab = ref('cert')
const driverTab = ref('cert')
const maintainTab = ref('cert')
const extractPanelRef = ref()
const driverExtractRef = ref()
const maintainExtractRef = ref()
const vehicleCertRef = ref<any>(null)
const driverCertRef = ref<any>(null)
const maintainCertRef = ref<any>(null)
// 标题栏右侧「新增」按钮：仅证照配置面板且停留在“证照配置”tab 时显示
const showAddBtn = computed(() => {
  if (panelKey.value === 'cert-vehicle') return vehicleTab.value === 'cert'
  if (panelKey.value === 'cert-driver') return driverTab.value === 'cert'
  if (panelKey.value === 'cert-maintain') return maintainTab.value === 'cert'
  if (!panelKey.value) return ['vehicle', 'driver', 'maintain'].includes(activeTab.value)
  return false
})
const activeCertRef = computed(() => {
  if (panelKey.value === 'cert-vehicle' || (!panelKey.value && activeTab.value === 'vehicle')) return vehicleCertRef.value
  if (panelKey.value === 'cert-driver' || (!panelKey.value && activeTab.value === 'driver')) return driverCertRef.value
  if (panelKey.value === 'cert-maintain' || (!panelKey.value && activeTab.value === 'maintain')) return maintainCertRef.value
  return null
})
const addBtnLabel = computed(() => activeCertRef.value?.addLabel || '新增')
function onHeaderAdd() { activeCertRef.value?.startAdd?.() }
// 切到提取规则页时重新拉取证照配置，保证字段名与证照配置实时同步
function onVehicleTabChange(val: string | number) {
  if (val === 'extract') extractPanelRef.value?.refresh()
}
function onDriverTabChange(val: string | number) {
  if (val === 'extract') driverExtractRef.value?.refresh()
}
function onMaintainTabChange(val: string | number) {
  if (val === 'extract') maintainExtractRef.value?.refresh()
}

const DRAWER_KEY = 'weknora-fleet-settings-drawer-width'
const savedWidth = parseInt(localStorage.getItem(DRAWER_KEY) || '', 10)
const drawerWidth = ref(`${Number.isFinite(savedWidth) && savedWidth >= 600 ? savedWidth : 760}px`)
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
  const w = Math.min(1200, Math.max(600, startW + (startX - e.clientX)))
  drawerWidth.value = `${w}px`
}
function onDrawerResizeEnd() {
  resizing = false
  document.removeEventListener('mousemove', onDrawerResizeMove)
  document.removeEventListener('mouseup', onDrawerResizeEnd)
  localStorage.setItem(DRAWER_KEY, drawerWidth.value)
}
onMounted(() => { activeTab.value = 'vehicle' })
</script>

<style lang="less" scoped>
.fleet-drawer-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; width: 100%; padding-right: 28px; box-sizing: border-box; }
.fleet-drawer-title { font-size: var(--td-font-size-title-medium); font-weight: 600; color: var(--td-text-color-primary); }
.fleet-settings-body { height: 100%; display: flex; flex-direction: column; min-height: 0; }
.fleet-settings-tabs { flex: 1; min-height: 0; display: flex; flex-direction: column; }
.fleet-settings-tabs :deep(.t-tabs__content) { flex: 1; min-height: 0; overflow: auto; padding-top: 16px; }

/* 抽屉拖宽手柄（与电费/电表配置抽屉一致） */
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
</style>
