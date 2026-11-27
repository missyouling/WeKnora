<template>
  <!--
    第 74 轮：「设置 · 能耗」平级重构；第 75 轮：能耗账单三子标签 + 费用清单多租户块标签。

    顶层同一行 7 个平级标签（能耗账单 / 提取规则 / 电费核算 / 水费核算 / 气费核算 /
    费用清单 / 工业表计），底部只保留唯一一套取消/保存。
    数据红线：只改视图与交互层，fleet_categories 的分类 ID / Key / 归属关系零改动。
  -->
  <SettingDrawer v-model:visible="drawerVisible" title="设置 · 能耗" :width="'min(920px, 96vw)'"
    :min-width="680" :max-width="1480" :storage-key="storageKey" :confirm-loading="saving"
    cancel-text="取消" confirm-text="保存" class="energy-settings-drawer"
    @cancel="onCancel" @confirm="onConfirm">
    <t-tabs v-model="activeTab" class="energy-settings-tabs" :lazy-load="true" @change="onTabChange">
      <!-- ① 能耗账单：仅保留账单分类子标签（第 76 轮精简） -->
      <t-tab-panel value="bill" label="能耗账单">
        <div class="energy-bill-switch" data-energy-bill-switch>
          <t-radio-group v-model="billTab" variant="default-filled" size="small">
            <t-radio-button v-for="t in billCategories" :key="t" :value="t">{{ t }}</t-radio-button>
          </t-radio-group>
        </div>
        <FleetCertPanel :ref="(el: any) => setPanel('bill', el)"
          scope="energy" :kb-id="kbId" lock-category-shell :category-scope="[...billCategories]"
          :initial-category="billTab" :flatten-groups="billTab === '光伏账单'" />
      </t-tab-panel>

      <!-- ② 提取规则 -->
      <t-tab-panel value="rules" label="提取规则">
        <ExtractRulePanel ref="ruleRef" scope="energy" :kb-name="'日常事务-电费'" :kb-id="kbId || ''"
          :active-category="ruleCategory" :active="activeTab === 'rules'" embedded />
      </t-tab-panel>

      <!-- ③④⑤ 三张核算表 -->
      <t-tab-panel v-for="t in ENERGY_FIELD_ONLY_TABS" :key="t.value" :value="t.value" :label="t.label">
        <FleetCertPanel :ref="(el: any) => setPanel(t.value, el)" scope="energy" :kb-id="kbId"
          lock-category-shell :category-scope="[t.category]" :initial-category="t.category" />
      </t-tab-panel>

      <!-- ⑥ 费用清单：默认分组 / 各租户分组（第 76 轮精简为两类块标签，
           租户档案与分摊子项已迁入「公司设置 → 单位与部门」） -->
      <t-tab-panel value="billing" label="费用清单">
        <div class="energy-bill-switch" data-cost-tabs>
          <t-radio-group v-model="costTab" variant="default-filled" size="small">
            <t-radio-button value="default">默认分组</t-radio-button>
            <t-radio-button v-for="t in costTenants" :key="t.id" :value="`tenant:${t.id}`">
              {{ t.label }}
            </t-radio-button>
          </t-radio-group>
        </div>
        <FleetCertPanel :ref="(el: any) => setPanel('billing', el)" scope="energy" :kb-id="kbId"
          lock-category-shell external-group-tabs :category-scope="['费用清单']"
          initial-category="费用清单" :initial-group="costTab" />
      </t-tab-panel>

      <!-- ⑦ 工业表计：电/水/气三个介质，互不串写 -->
      <t-tab-panel value="meter" label="工业表计">
        <!-- 介质标签与「新增」按钮由面板自己渲染在同一行（左标签、右按钮），
             抽屉不再单独出标签栏，避免两行错位。 -->
        <MeterSettingsPanel v-model:category="meterCategory" ref="meterRef" embedded
          @changed="onPanelChanged" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import FleetCertPanel from './FleetCertPanel.vue'
import ExtractRulePanel from './ExtractRulePanel.vue'
import MeterSettingsPanel from './MeterSettingsPanel.vue'
import { listBillingTenants } from '@/api/knowledge-base'
import { listFleetCategories } from '@/api/fleet'
import { unitShortName } from './unitLabel'
import {
  ENERGY_BILL_CATEGORIES,
  ENERGY_SETTINGS_TABS,
  energyBillCategoriesOf,
  energyTabOfCategory,
  type EnergySettingsTab,
} from './energySettingsTabs'

const props = withDefaults(defineProps<{
  visible: boolean
  kbId?: string
  initialTab?: EnergySettingsTab
  initialCategory?: string
  storageKey?: string
}>(), {
  kbId: '',
  initialTab: 'bill',
  initialCategory: '',
  storageKey: 'weknora-energy-settings-drawer-width',
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

const drawerVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const activeTab = ref<EnergySettingsTab>('bill')
/** 能耗账单的子标签：市电账单 / 光伏账单（动态，新增账单分类自动追加） */
const billTab = ref<string>('市电账单')
const billCategories = ref<string[]>([...ENERGY_BILL_CATEGORIES])
const meterCategory = ref<'electricity' | 'water' | 'gas'>('electricity')
const saving = ref(false)

/** 费用清单块标签：默认分组 / tenant:<id>（别名来自公司设置） */
const costTab = ref<string>('default')
const costTenants = ref<Array<{ id: string; name: string; label: string }>>([])

const ENERGY_FIELD_ONLY_TABS = ENERGY_SETTINGS_TABS
  .filter((t) => ['electric', 'water', 'gas'].includes(t.value))
  .map((t) => ({
    value: t.value,
    label: t.label,
    category: t.value === 'electric' ? '电费核算' : t.value === 'water' ? '水费核算' : '气费核算',
  }))

const panels = ref<Record<string, any>>({})
function setPanel(key: string, el: any) {
  if (el) panels.value[key] = el
  else delete panels.value[key]
}
const ruleRef = ref<any>(null)
const meterRef = ref<any>(null)

const ruleCategory = computed(() => (billTab.value === '光伏账单' ? '光伏账单' : '市电账单'))

/** 动态读取能耗分类，刷新「能耗账单」子标签（新增账单分类自动出现）。 */
async function loadBillCategories() {
  try {
    const res: any = await listFleetCategories({ scope: 'energy' })
    const list = energyBillCategoriesOf(Array.isArray(res?.data) ? res.data : [])
    if (list.length) {
      billCategories.value = list
      if (!list.includes(billTab.value)) billTab.value = list[0]
    }
  } catch {
    // 后端不可用时保留内置两项，抽屉不至于空标签
  }
}

function onPanelChanged() { emit('saved') }

async function loadCostTenants() {
  try {
    const res: any = await listBillingTenants()
    const list = Array.isArray(res?.data) ? res.data : []
    costTenants.value = list.map((t: any) => ({
      id: String(t.id),
      name: String(t.name || ''),
      // 别名是全局简称唯一真相源（公司设置维护）；缺失时才回退租户名
      label: String(t.alias || '').trim() || unitShortName(t.name),
    }))
  } catch { costTenants.value = [] }
}

/** 打开时定位：分类优先（页面设置按钮只传分类），市电/光伏 → 能耗账单对应子标签。 */
function syncFromProps() {
  const cat = String(props.initialCategory || '').trim()
  const bill = billCategories.value.includes(cat) ? cat : ''
  if (bill) {
    billTab.value = bill
    activeTab.value = 'bill'
    return
  }
  activeTab.value = cat ? energyTabOfCategory(cat) : (props.initialTab || 'bill')
  if (activeTab.value === 'meter') {
    meterCategory.value = cat === '水费核算' ? 'water' : cat === '气费核算' ? 'gas' : 'electricity'
  }
  if (activeTab.value === 'billing') costTab.value = 'default'
}

watch(() => props.visible, (v) => {
  if (!v) return
  saving.value = false
  syncFromProps()
  void loadCostTenants()
  void loadBillCategories()
}, { immediate: true })

watch(() => [props.initialTab, props.initialCategory], () => { if (props.visible) syncFromProps() })

onMounted(() => { void loadCostTenants(); void loadBillCategories() })

function onTabChange(val: string | number) {
  if (val === 'rules') nextTick(() => ruleRef.value?.refresh?.())
  if (val === 'billing') void loadCostTenants()
}

async function onConfirm() {
  saving.value = true
  try {
    const tab = activeTab.value
    if (tab === 'bill') {
      await panels.value.bill?.commitPendingEdit?.()
    } else if (tab === 'rules') {
      await ruleRef.value?.save?.()
    } else if (tab === 'meter') {
      await meterRef.value?.commitAll?.()
    } else if (tab === 'billing') {
      await panels.value.billing?.commitPendingEdit?.()
    } else {
      await panels.value[tab]?.commitPendingEdit?.()
    }
    emit('saved')
    emit('update:visible', false)
  } catch {
    // 面板自己提示错误，抽屉保持打开
  } finally {
    saving.value = false
  }
}

async function onCancel() {
  for (const key of Object.keys(panels.value)) {
    try { panels.value[key]?.discardPendingEdit?.() } catch { /* ignore */ }
  }
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.energy-settings-tabs {
  height: 100%;
  display: flex;
  flex-direction: column;

  :deep(.t-tabs__content) {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding-top: 12px;
  }

  :deep(.t-tabs__nav-wrap) { flex-wrap: nowrap; }
  :deep(.t-tabs__nav-item) { white-space: nowrap; }
}

.energy-bill-switch {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
</style>

<style lang="less">
.energy-settings-drawer .t-drawer__content-wrapper,
.energy-settings-drawer.t-drawer__content-wrapper {
  min-width: min(680px, 96vw);
}
</style>
