<template>
  <SettingDrawer v-model:visible="drawerVisible" :title="'发票设置'" width="760px"
    :storage-key="'weknora-invoice-settings-drawer-width'" destroy-on-close
    class="invoice-settings-drawer">
    <t-tabs v-model="activeTab" class="invoice-settings-tabs" @change="onTabChange">
      <t-tab-panel value="fields" label="字段定义">
        <FleetCategoryPanel scope="invoice" @saved="onPanelSaved" />
      </t-tab-panel>
      <t-tab-panel value="rules" label="提取规则">
        <ExtractRulePanel ref="extractRef" scope="invoice" kb-name="日常事务-发票" :kb-id="kbId || ''" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import FleetCategoryPanel from './FleetCategoryPanel.vue'
import ExtractRulePanel from './ExtractRulePanel.vue'

const props = defineProps<{ visible: boolean; kbId?: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

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

// 字段/分类配置保存后：通知列表刷新列，并刷新提取规则面板的类型下拉
function onPanelSaved() {
  emit('saved')
  extractRef.value?.refresh()
}
</script>

<style lang="less" scoped>
.invoice-settings-tabs {
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
