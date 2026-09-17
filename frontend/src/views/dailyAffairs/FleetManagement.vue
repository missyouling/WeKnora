<template>
  <div class="fleet-management-container">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>车队管理</h2>
        <p class="header-subtitle">车辆、驾驶员与油卡台账；维保、加油、车险等记录按月管理自动汇总</p>
      </div>
      <t-button variant="outline" size="small" @click="settingsVisible = true">
        <template #icon><t-icon name="setting" size="14px" /></template>
        设置
      </t-button>
    </div>

    <!-- 主界面 -->
    <div class="fleet-main">
      <div class="fleet-layout">
        <!-- 侧边栏菜单 -->
        <div class="fleet-sidebar">
          <div v-for="item in sideItems" :key="item.key" class="fleet-side-item"
            :class="{ active: activeTab === item.key }" @click="switchTab(item.key)">
            <t-icon :name="item.icon" size="16px" /><span>{{ item.label }}</span>
          </div>
        </div>
        <!-- 内容区 -->
        <div class="fleet-content">
          <template v-if="activeTab === 'billing'">
            <FleetBillingTab />
          </template>
          <template v-else>
            <FleetRecordTab :key="activeTab" :record-type="activeTab" @open-settings="settingsVisible = true" />
          </template>
        </div>
      </div>
    </div>

    <!-- 设置抽屉：车辆管理 / 驾驶员管理 / 油卡管理 -->
    <FleetSettingsDrawer v-model:visible="settingsVisible" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import FleetRecordTab from './FleetRecordTab.vue'
import FleetBillingTab from './FleetBillingTab.vue'
import FleetSettingsDrawer from './FleetSettingsDrawer.vue'

const settingsVisible = ref(false)

const sideItems = [
  { key: 'maintain', label: '维保记录', icon: 'wrench' },
  { key: 'fuel', label: '加油记录', icon: 'fuel' },
  { key: 'insurance', label: '车险记录', icon: 'shield' },
  { key: 'tire', label: '轮胎记录', icon: 'circle' },
  { key: 'material', label: '辅材记录', icon: 'box' },
  { key: 'violation', label: '违章记录', icon: 'alert' },
  { key: 'car-request', label: '请车记录', icon: 'calendar' },
  { key: 'toll', label: '通行费', icon: 'bill' },
  { key: 'billing', label: '费用清单', icon: 'chart-bar' },
]

const activeTab = ref('maintain')
const switchTab = (key: string) => { activeTab.value = key }
</script>

<style lang="less" scoped>
.fleet-management-container {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
  box-sizing: border-box;
  padding: 20px 24px;
  overflow: hidden;

  .header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    margin-bottom: 16px;

    .header-title {
      h2 { font-size: 20px; font-weight: 600; color: var(--td-text-color-primary); margin: 0 0 6px 0; }
      .header-subtitle { font-size: 14px; color: var(--td-text-color-secondary); margin: 0; }
    }
  }

  .fleet-main {
    flex: 1;
    min-height: 0;
    display: flex;

    .fleet-layout {
      flex: 1;
      min-width: 0;
      display: flex;
      gap: 16px;
    }

    .fleet-sidebar {
      flex: 0 0 132px;
      display: flex;
      flex-direction: column;
      gap: 4px;

      .fleet-side-item {
        display: flex;
        align-items: center;
        gap: 8px;
        padding: 9px 12px;
        border-radius: 8px;
        font-size: 13px;
        color: var(--td-text-color-secondary);
        cursor: pointer;
        transition: all 0.2s;
        user-select: none;

        &:hover { background: var(--td-bg-color-container-hover); color: var(--td-text-color-primary); }
        &.active {
          background: var(--td-brand-color-light);
          color: var(--td-brand-color);
          font-weight: 600;
        }
      }
    }

    .fleet-content {
      flex: 1;
      min-width: 0;
      min-height: 0;
      background: var(--td-bg-color-container);
      border: 1px solid var(--td-component-stroke);
      border-radius: 12px;
      padding: 16px;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
  }
}
</style>
