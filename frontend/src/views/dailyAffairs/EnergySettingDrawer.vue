<template>
  <!--
    第 74 轮：「设置 · 能耗」平级重构。

    旧结构：字段定义（二级外壳）→ 分类表格（市电/光伏/电费/水费/气费/费用清单）
            + 提取规则 + 专属 Tab（光伏设置/工业表计/清单设置），
            抽屉内外各有一套取消/保存，用户要在两层 Tab 之间来回找配置。
    新结构：顶层同一行 7 个平级标签（能耗账单 / 提取规则 / 电费核算 / 水费核算 /
            气费核算 / 费用清单 / 工业表计），底部只保留唯一一套取消/保存。

    数据红线：只改视图与交互层，fleet_categories 的分类 ID / Key / 归属关系零改动
    （历史账单、提取规则与 123/151/86 条核算记录全部不受影响）。
  -->
  <SettingDrawer v-model:visible="drawerVisible" title="设置 · 能耗" :width="'min(920px, 96vw)'"
    :min-width="680" :max-width="1480" :storage-key="storageKey" :confirm-loading="saving"
    cancel-text="取消" confirm-text="保存" class="energy-settings-drawer"
    @cancel="onCancel" @confirm="onConfirm">
    <t-tabs v-model="activeTab" class="energy-settings-tabs" :lazy-load="true" @change="onTabChange">
      <!-- ① 能耗账单：市电账单 / 光伏账单 二级切换，直接给字段列表 -->
      <t-tab-panel value="bill" label="能耗账单">
        <div class="energy-bill-switch" data-energy-bill-switch>
          <t-radio-group v-model="billCategory" variant="default-filled" size="small">
            <t-radio-button v-for="c in ENERGY_BILL_CATEGORIES" :key="c" :value="c">{{ c }}</t-radio-button>
          </t-radio-group>
          <span class="energy-bill-hint">
            {{ billCategory === '光伏账单'
              ? '全部字段平铺展示（已删除派生字段与账单概况两个分组外壳）'
              : '字段分组默认折叠，点击分组查看字段' }}
          </span>
        </div>
        <FleetCertPanel :ref="(el: any) => setPanel('bill', el)" scope="energy" :kb-id="kbId"
          lock-category-shell :category-scope="[...ENERGY_BILL_CATEGORIES]"
          :initial-category="billCategory"
          :flatten-groups="billCategory === '光伏账单'"
          :collapse-groups="energyCollapsesGroupsByDefault(billCategory)" />
        <!-- 光伏账单的基本户信息：原「光伏设置」Tab 的归宿。保存由抽屉底栏统一提交。 -->
        <div v-if="billCategory === '光伏账单'" class="energy-solar-block">
          <SolarSettingsPanel ref="solarRef" :kb-id="kbId" embedded @changed="onPanelChanged" />
        </div>
      </t-tab-panel>

      <!-- ② 提取规则：只有市电/光伏是 AI 提取的账单 -->
      <t-tab-panel value="rules" label="提取规则">
        <ExtractRulePanel ref="ruleRef" scope="energy" :kb-name="'日常事务-电费'" :kb-id="kbId || ''"
          :active-category="billCategory" :active="activeTab === 'rules'" embedded />
      </t-tab-panel>

      <!-- ③④⑤ 三张核算表：各自一个平级标签，直出该分类的字段配置 -->
      <t-tab-panel v-for="t in ENERGY_FIELD_ONLY_TABS" :key="t.value" :value="t.value" :label="t.label">
        <FleetCertPanel :ref="(el: any) => setPanel(t.value, el)" scope="energy" :kb-id="kbId"
          lock-category-shell :category-scope="[t.category]" :initial-category="t.category" />
      </t-tab-panel>

      <!-- ⑥ 费用清单：字段配置 + 清单设置（租户档案 / 分摊子项） -->
      <t-tab-panel value="billing" label="费用清单">
        <FleetCertPanel :ref="(el: any) => setPanel('billing', el)" scope="energy" :kb-id="kbId"
          lock-category-shell :category-scope="['费用清单']" initial-category="费用清单" />
        <div class="energy-sub-block">
          <div class="energy-sub-title">清单设置</div>
          <TenantSettingsPanel ref="tenantRef" :kb-id="kbId || ''" embedded @changed="onPanelChanged" />
        </div>
      </t-tab-panel>

      <!-- ⑦ 工业表计：电/水/气三个介质的表计档案 -->
      <t-tab-panel value="meter" label="工业表计">
        <div class="energy-bill-switch" data-energy-meter-switch>
          <t-radio-group v-model="meterCategory" variant="default-filled" size="small">
            <t-radio-button value="electricity">电表</t-radio-button>
            <t-radio-button value="water">水表</t-radio-button>
            <t-radio-button value="gas">气表</t-radio-button>
          </t-radio-group>
          <span class="energy-bill-hint">表计档案与单价规则，保存后核算列表立即同步</span>
        </div>
        <MeterSettingsPanel ref="meterRef" :category="meterCategory" embedded @changed="onPanelChanged" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import FleetCertPanel from './FleetCertPanel.vue'
import ExtractRulePanel from './ExtractRulePanel.vue'
import MeterSettingsPanel from './MeterSettingsPanel.vue'
import TenantSettingsPanel from './TenantSettingsPanel.vue'
import SolarSettingsPanel from './SolarSettingsPanel.vue'
import {
  ENERGY_BILL_CATEGORIES,
  ENERGY_SETTINGS_TABS,
  energyBillCategoryOf,
  energyCollapsesGroupsByDefault,
  energyTabOfCategory,
  type EnergyBillCategory,
  type EnergySettingsTab,
} from './energySettingsTabs'

const props = withDefaults(defineProps<{
  visible: boolean
  kbId?: string
  /** 打开时要激活的顶层标签（由页面当前子视图决定）。 */
  initialTab?: EnergySettingsTab
  /** 打开时要定位的分类（市电/光伏/电费核算/水费核算/气费核算/费用清单）。 */
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
const billCategory = ref<EnergyBillCategory>('市电账单')
const meterCategory = ref<'electricity' | 'water' | 'gas'>('electricity')
const saving = ref(false)

/** 三个核算表标签的元数据（值与分类名 1:1）。 */
const ENERGY_FIELD_ONLY_TABS = ENERGY_SETTINGS_TABS
  .filter((t) => ['electric', 'water', 'gas'].includes(t.value))
  .map((t) => ({
    value: t.value,
    label: t.label,
    category: t.value === 'electric' ? '电费核算' : t.value === 'water' ? '水费核算' : '气费核算',
  }))

// ---- 面板实例（按标签懒挂载，函数 ref 收进 map） ----
const panels = ref<Record<string, any>>({})
function setPanel(key: string, el: any) {
  if (el) panels.value[key] = el
  else delete panels.value[key]
}
const ruleRef = ref<any>(null)
const meterRef = ref<any>(null)
const tenantRef = ref<any>(null)
const solarRef = ref<any>(null)

function onPanelChanged() {
  emit('saved')
}

/**
 * 打开时定位：分类优先于标签（页面「设置」按钮只传分类）。
 * 市电/光伏 → 能耗账单 + 对应二级切换；其余 → 直接激活同名平级标签。
 */
function syncFromProps() {
  const cat = String(props.initialCategory || '').trim()
  const bill = energyBillCategoryOf(cat)
  if (bill) {
    billCategory.value = bill
    activeTab.value = 'bill'
    return
  }
  activeTab.value = cat ? energyTabOfCategory(cat) : (props.initialTab || 'bill')
  if (activeTab.value === 'meter') {
    // 工业表计标签：从哪张核算表进来就停在哪个介质
    meterCategory.value = cat === '水费核算' ? 'water' : cat === '气费核算' ? 'gas' : 'electricity'
  }
}

watch(() => props.visible, (v) => {
  if (!v) return
  saving.value = false
  syncFromProps()
}, { immediate: true })

// 抽屉保持打开时父级切换定位（极少见，但 KPI 卡片可能连点）
watch(() => [props.initialTab, props.initialCategory], () => { if (props.visible) syncFromProps() })

function onTabChange(val: string | number) {
  if (val === 'rules') nextTick(() => ruleRef.value?.refresh?.())
}

/**
 * 保存：只提交**当前标签**的草稿，成功后关闭抽屉。
 * 面板内部失败会自行提示（校验错误），此时抽屉保持打开让用户修正。
 */
async function onConfirm() {
  saving.value = true
  try {
    const tab = activeTab.value
    if (tab === 'bill') {
      await panels.value.bill?.commitPendingEdit?.()
      if (billCategory.value === '光伏账单') await solarRef.value?.commit?.()
    } else if (tab === 'rules') {
      await ruleRef.value?.save?.()
    } else if (tab === 'meter') {
      await meterRef.value?.commitAll?.()
    } else {
      await panels.value[tab]?.commitPendingEdit?.()
      if (tab === 'billing') await tenantRef.value?.commitAll?.()
    }
    emit('saved')
    emit('update:visible', false)
  } catch {
    // 由面板自己提示错误，抽屉保持打开
  } finally {
    saving.value = false
  }
}

/** 取消：丢弃各标签的行内草稿并关闭（已落库的配置不受影响）。 */
async function onCancel() {
  try { await tenantRef.value?.discardAll?.() } catch { /* ignore */ }
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

  /* 顶层 7 个平级标签必须同一行，绝不换行 */
  :deep(.t-tabs__nav-wrap) {
    flex-wrap: nowrap;
  }

  :deep(.t-tabs__nav-item) {
    white-space: nowrap;
  }
}

.energy-bill-switch {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.energy-bill-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.energy-solar-block {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}

.energy-sub-block {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}

.energy-sub-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  margin-bottom: 8px;
}
</style>

<style lang="less">
/* 抽屉是 teleport 出去的组件，scoped 命不中内部节点 */
.energy-settings-drawer .t-drawer__content-wrapper,
.energy-settings-drawer.t-drawer__content-wrapper {
  min-width: min(680px, 96vw);
}
</style>
