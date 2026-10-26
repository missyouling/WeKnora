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
        <FleetCertPanel :scope="scope" :kb-id="kbId || ''" :initial-category="initialCategory"
          @ai-guide="onAiGuide" @category-change="onCategoryChange" />
      </t-tab-panel>
      <t-tab-panel v-if="showRulesTab" value="rules" label="提取规则">
        <ExtractRulePanel ref="extractRef" :scope="scope" :kb-name="kbName" :kb-id="kbId || ''" />
      </t-tab-panel>
    </t-tabs>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, watch, onMounted, onUnmounted } from 'vue'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import FleetCertPanel from '@/views/dailyAffairs/FleetCertPanel.vue'
import ExtractRulePanel from '@/views/dailyAffairs/ExtractRulePanel.vue'
import { cacheSampleText } from '@/views/dailyAffairs/sampleTextCache'

// 标准设置抽屉（中台统一组件）：
// 「字段定义 / 提取规则」双 Tab 规范，复刻自 InvoiceSettingsDrawer ——
// 集成全局 SettingDrawer 底座 + 生命周期管理（切到提取规则 Tab 时 refresh、
// fleet-categories-changed 全局事件同步）。
//
// scope 参数化后，车队/合同/能耗等模块均可复用同一套双 Tab 骨架。
const props = withDefaults(defineProps<{
  visible: boolean
  kbId?: string
  scope?: '' | 'vehicle' | 'invoice' | 'driver' | 'maintain' | 'contract' | 'regulation' | 'award_punish' | 'energy'
  /** 打开时自动展开的分类名（能耗：当前卡片对应的分类）。 */
  initialCategory?: string
  /**
   * 允许配置「提取规则」的分类白名单（空 = 全部允许）。
   *
   * 能耗的电费核算 / 水费核算 / 气费核算 / 费用清单是**计算表**：
   * 起度、止度、倍率、分摊比例这些值由录入与公式得出，不存在"从文件提取某一列"。
   * 给它们露出规则页，用户会配一堆永远不会被消费的 Prompt。
   */
  ruleCategories?: string[]
  kbName?: string
  title?: string
  storageKey?: string
}>(), {
  scope: 'invoice',
  initialCategory: '',
  ruleCategories: () => [],
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

// 当前正在编辑的分类：进入抽屉时取 initialCategory，之后由 FleetCertPanel 广播
const currentCategory = ref('')
watch(() => props.initialCategory, (v) => { currentCategory.value = String(v || '') }, { immediate: true })
function onCategoryChange(name: string) { currentCategory.value = String(name || '') }

// 提取规则 Tab 是否可见：白名单为空 = 全部可见（发票/合同等不受影响）
const showRulesTab = computed(() => {
  const allow = props.ruleCategories || []
  if (!allow.length) return true
  const cur = currentCategory.value.trim()
  if (!cur) return true
  return allow.includes(cur)
})
// 白名单之外的分类被选中时，正停在规则页会看到一片空白 → 自动切回字段定义
watch(showRulesTab, (ok) => { if (!ok && activeTab.value === 'rules') activeTab.value = 'fields' })

function onTabChange(val: string | number) {
  if (val !== 'rules') return
  extractRef.value?.refresh()
  // 跟随「字段定义」里选中的分类：在「光伏账单」的字段页切到提取规则，
  // 期望看到的就是光伏账单的规则，而不是下拉里排第一的市电账单（真机踩过）。
  const cur = currentCategory.value.trim()
  if (cur) nextTick(() => extractRef.value?.focusCertType?.({ certType: cur }))
}

// 「字段定义 → 智能生成字段 → 保存」后由子组件冒泡上来：切到提取规则 Tab、
// 选中对应分类，并把样票文本交给规则面板触发一键智能生成规则。
function onAiGuide(payload: { certType: string; sampleText?: string; autoSuggest?: boolean }) {
  goRules(payload)
}

// goRules 切到「提取规则」Tab 并聚焦到指定分类（可选立即触发智能生成规则）。
// 暴露给父组件用：发票页等可在保存后由外部直接驱动这段联动。
function goRules(payload?: { certType: string; sampleText?: string; autoSuggest?: boolean }) {
  activeTab.value = 'rules'
  if (payload?.sampleText && payload?.certType) cacheSampleText(payload.certType, payload.sampleText)
  nextTick(() => extractRef.value?.focusCertType?.(payload))
}
defineExpose({ goRules })

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
