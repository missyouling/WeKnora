<template>
  <SettingDrawer
    v-model:visible="drawerVisible"
    :title="title"
    width="760px"
    :storage-key="storageKey"
    destroy-on-close
    hide-footer
    class="standard-settings-drawer"
  >
    <t-tabs v-model="activeTab" class="standard-settings-tabs" @change="onTabChange">
      <t-tab-panel value="fields" label="字段定义">
        <FleetCertPanel :scope="scope" :kb-id="kbId || ''" />
      </t-tab-panel>
      <t-tab-panel value="rules" label="提取规则">
        <ExtractRulePanel ref="extractRef" :scope="scope" :kb-name="kbName" :kb-id="kbId || ''" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import FleetCertPanel from '@/views/dailyAffairs/FleetCertPanel.vue'
import ExtractRulePanel from '@/views/dailyAffairs/ExtractRulePanel.vue'

// 标准设置抽屉（中台统一组件）：
// 「字段定义 / 提取规则」双 Tab 规范，复刻自 InvoiceSettingsDrawer ——
// 集成全局 SettingDrawer 底座 + 生命周期管理（切到提取规则 Tab 时 refresh、
// fleet-categories-changed 全局事件同步）。
//
// scope 参数化后，车队/合同/能耗等模块均可复用同一套双 Tab 骨架。
const props = withDefaults(defineProps<{
  visible: boolean
  kbId?: string
  scope?: '' | 'vehicle' | 'invoice' | 'driver' | 'maintain' | 'contract' | 'regulation' | 'award_punish'
  kbName?: string
  title?: string
  storageKey?: string
}>(), {
  scope: 'invoice',
  kbName: '日常事务-发票',
  title: '发票设置',
  storageKey: 'weknora-invoice-settings-drawer-width',
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

// v-model 桥接：props 不可写，经 computed 转发 update:visible
const drawerVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const activeTab = ref('fields')
const extractRef = ref()

function onTabChange(val: string | number) {
  if (val === 'rules') extractRef.value?.refresh()
}

// FleetCertPanel 保存分类/字段后派发全局事件：刷新列表列定义与提取规则面板
function onCategoriesChanged() {
  emit('saved')
  extractRef.value?.refresh()
}
onMounted(() => window.addEventListener('fleet-categories-changed', onCategoriesChanged))
onUnmounted(() => window.removeEventListener('fleet-categories-changed', onCategoriesChanged))
</script>

<style lang="less" scoped>
.standard-settings-tabs {
  height: 100%;
  display: flex;
  flex-direction: column;
  :deep(.t-tabs__content) {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding-top: 16px;
  }
}
</style>
