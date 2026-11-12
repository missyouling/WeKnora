<!--
  公司设置弹窗（第 53 轮）。

  入口：左下角用户信息菜单「个人设置」与「空间设置」之间的「公司设置」。

  复用原项目 `SettingsModalShell` 作为壳层（与「全部设置」逐像素同源：
  遮罩/12px 圆角/--td-shadow-3/208px 左栏/自绘关闭按钮），内容区直接挂
  `HrOrgPanel`（单位 + 部门的多单位配置，含拖拽排序与改名级联）。

  边界：
    · 不修改 Settings.vue / uiStore / 路由 —— 只在本组件内承载内容；
    · 手动保存（HrOrgPanel 的「保存」按钮），关闭即丢弃未保存修改；
    · 保存成功后广播 `hr-org-changed`，人事页据此重拉部门/单位下拉与列表。
-->
<template>
  <SettingsModalShell :visible="visible" title="公司设置" :nav-groups="navGroups"
    :model-value="'units'" overlay-class="all-settings-overlay" :z-index="2000" @close="close">
    <template #nav-icon="{ item }">
      <t-icon :name="item.icon" class="nav-icon" />
    </template>
    <div class="content-wrapper">
      <div class="section-header">
        <h2>单位与部门</h2>
        <p class="section-description">
          维护公司主体与下属部门。单位/部门改名会级联更新花名册与劳动合同中的对应字段；调整顺序后点击「保存」统一提交。
        </p>
      </div>
      <div class="settings-group">
        <HrOrgPanel :visible="visible" @saved="onSaved" />
      </div>
    </div>
  </SettingsModalShell>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import SettingsModalShell from '@/components/SettingsModalShell.vue'
import HrOrgPanel from '@/views/dailyAffairs/HrOrgPanel.vue'

withDefaults(defineProps<{ visible: boolean }>(), { visible: false })
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

/** 左侧导航：与设置弹窗同构（单组单节，后续可扩展「单位信息 / 部门」两节）。 */
const navGroups = computed(() => [
  {
    key: 'company',
    label: '公司设置',
    items: [{ key: 'units', label: '单位与部门', icon: 'usergroup' }],
  },
])

function close() {
  emit('update:visible', false)
}

function onSaved() {
  // 跨页面同步：人事页监听该事件重拉部门/单位下拉、列表与卡片。
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('hr-org-changed'))
  }
  emit('saved')
}
</script>

<style lang="less" scoped>
.content-wrapper {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 28px 40px 48px;
}
.section-header {
  margin-bottom: var(--td-comp-margin-l);

  h2 {
    margin: 0 0 6px;
    font-size: var(--td-font-size-title-large);
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
  .section-description {
    margin: 0;
    font-size: var(--td-font-size-body-medium);
    line-height: 1.6;
    color: var(--td-text-color-secondary);
  }
}
.settings-group {
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  padding: var(--td-comp-paddingTB-l) var(--td-comp-paddingLR-l);
}
.nav-icon {
  font-size: var(--td-font-size-headline-small);
}
</style>
