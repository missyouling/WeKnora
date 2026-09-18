<template>
  <div class="fleet-management-container">
    <!-- 顶部 -->
    <div class="header">
      <div class="header-title">
        <h2>车队管理</h2>
        <p class="header-subtitle">车辆/司机/维保档案自动解析，轮胎、年检、成本与费用清单统一管理</p>
      </div>
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
          <div class="fleet-side-group">
            <div class="fleet-side-group-head" @click="costOpen = !costOpen">
              <t-icon name="money" size="16px" /><span>成本管理</span>
              <t-icon :name="costOpen ? 'chevron-up' : 'chevron-down'" size="14px" class="fleet-side-group-arrow" />
            </div>
            <transition name="side-group">
              <div v-if="costOpen" class="fleet-side-group-items">
                <div v-for="item in costItems" :key="item.key" class="fleet-side-item fleet-side-item--sub"
                  :class="{ active: activeTab === item.key }" @click="switchTab(item.key)">
                  <t-icon :name="item.icon" size="15px" /><span>{{ item.label }}</span>
                </div>
              </div>
            </transition>
          </div>
        </div>
        <!-- 内容区：所有菜单统一复刻电费核算内容页布局，仅字段按类型替换 -->
        <div class="fleet-content">
          <FleetRecordList :key="activeTab" :record-type="activeTab" @open-settings="settingsVisible = true" />
        </div>
      </div>
    </div>

    <!-- 设置抽屉：按当前菜单动态显示对应配置 -->
    <FleetSettingsDrawer v-model:visible="settingsVisible" :menu-key="activeTab" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import FleetRecordList from './FleetRecordList.vue'
import FleetSettingsDrawer from './FleetSettingsDrawer.vue'

const settingsVisible = ref(false)

const sideItems = [
  { key: 'vehicle-archive', label: '车辆档案', icon: 'view-module' },
  { key: 'driver-archive', label: '司机档案', icon: 'user' },
  { key: 'maintain-archive', label: '维保管理', icon: 'tools' },
  { key: 'tire', label: '轮胎管理', icon: 'circle' },
  { key: 'inspection', label: '年检管理', icon: 'check-rectangle' },
]

const costItems = [
  { key: 'fuel-charge', label: '加油充电', icon: 'thunder' },
  { key: 'road-toll', label: '路桥费', icon: 'map' },
  { key: 'repair-cost', label: '维修保养费', icon: 'tools' },
  { key: 'insurance-claim', label: '保险理赔', icon: 'wallet' },
  { key: 'violation', label: '违章处理', icon: 'flag' },
  { key: 'material', label: '辅材记录', icon: 'tag' },
  { key: 'car-request', label: '请车记录', icon: 'calendar' },
]

const costOpen = ref(true)

const activeTab = ref('vehicle-archive')
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
      flex: 0 0 140px;
      display: flex;
      flex-direction: column;
      gap: 4px;
      overflow-y: auto;

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
        white-space: nowrap;

        &:hover { background: var(--td-bg-color-container-hover); color: var(--td-text-color-primary); }
        &.active { background: var(--td-brand-color-light); color: var(--td-brand-color); font-weight: 500; }
        &--sub { padding-left: 26px; font-size: 12.5px; }
      }

      .fleet-side-group {
        display: flex;
        flex-direction: column;
        gap: 2px;
        margin-top: 4px;

        .fleet-side-group-head {
          display: flex;
          align-items: center;
          gap: 8px;
          padding: 9px 12px;
          border-radius: 8px;
          font-size: 13px;
          color: var(--td-text-color-primary);
          cursor: pointer;
          user-select: none;
          font-weight: 500;

          .fleet-side-group-arrow { margin-left: auto; color: var(--td-text-color-placeholder); }
        }

        .fleet-side-group-items {
          display: flex;
          flex-direction: column;
          gap: 2px;
        }
      }
    }

    .fleet-content {
      flex: 1;
      min-width: 0;
      min-height: 0;
      display: flex;
      flex-direction: column;
    }
  }
}
.side-group-enter-active, .side-group-leave-active { transition: opacity .15s ease; }
.side-group-enter-from, .side-group-leave-to { opacity: 0; }
</style>
