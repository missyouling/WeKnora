<template>
  <div class="dash-kpi-group">
    <div
      v-for="card in cards"
      :key="card.key"
      class="dash-kpi-card"
      :class="[card.cls, card.numCls]"
      @click="$emit('card-click', card)"
    >
      <div class="dash-kpi-card__icon">
        <t-icon :name="card.icon" size="22px" />
      </div>
      <div class="dash-kpi-card__body">
        <div class="dash-kpi-card__label">{{ card.label }}</div>
        <div class="dash-kpi-card__num">
          {{ card.value }}<span v-if="card.unit">{{ card.unit }}</span>
        </div>
        <div v-if="card.sub" class="dash-kpi-card__sub">{{ card.sub }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 概览 KPI 卡片组（中台统一组件）：
// 提炼自 InvoiceManagement.vue 的 type-card 范式 —— flex 左右布局 + 40px 圆角图标底 +
// hover 上浮 + 大字号数值（超长自动降档）。卡片点击由父级通过 card-click 事件联动业务。
//
// cards 数据形状（与发票概览 overviewCards 完全一致，接入零改动）：
//   { key, label, icon, value, unit, sub, cls, numCls, action?, typeValue? }
// - cls：视觉语义类，透传绑定（'' 中性 / is-brand 品牌 / is-warn 告警）
// - numCls：主数值降档类（'' / num-sm / num-xs），超长金额防换行
// - theme：可选语义（brand/success/warning/neutral），内部映射为对应 cls，供新模块复用
export interface KpiCard {
  key: string
  label: string
  icon: string
  value: string
  unit?: string
  sub?: string
  cls?: string
  numCls?: string
  theme?: 'brand' | 'success' | 'warning' | 'neutral'
  action?: string
  typeValue?: string
}

defineProps<{ cards: KpiCard[] }>()
defineEmits<{ (e: 'card-click', card: KpiCard): void }>()
</script>

<style lang="less" scoped>
.dash-kpi-group {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(190px, 1fr));
  gap: 14px;
}
.dash-kpi-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  background: var(--wk-kpi-bg);
  border: 1px solid transparent;
  border-radius: var(--td-radius-large);
  cursor: pointer;
  transition: all 0.18s ease;
}
.dash-kpi-card:hover {
  background: var(--wk-kpi-hover-bg);
  border-color: var(--td-brand-color);
  transform: translateY(-1px);
  box-shadow: var(--wk-kpi-shadow);
}
.dash-kpi-card__icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--wk-kpi-icon-bg);
  color: var(--td-brand-color);
  box-shadow: var(--wk-kpi-icon-shadow, 0 1px 3px rgba(0, 0, 0, 0.06));
}
.dash-kpi-card__body {
  flex: 1;
  min-width: 0;
}
.dash-kpi-card__label {
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  margin-bottom: 4px;
}
.dash-kpi-card__num {
  font-size: 20px;
  font-weight: 600;
  color: var(--td-brand-color);
  white-space: nowrap;
}
.dash-kpi-card__num.num-sm { font-size: 17px; }
.dash-kpi-card__num.num-xs { font-size: 14px; }
.dash-kpi-card__num span {
  font-size: 12px;
  font-weight: 400;
  color: var(--td-text-color-secondary);
  margin-left: 2px;
}
.dash-kpi-card__sub {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-top: 4px;
}
/* 语义化视觉类（兼容既有 cls 透传 + theme 语义） */
.dash-kpi-card.is-brand .dash-kpi-card__icon,
.dash-kpi-card.theme-brand .dash-kpi-card__icon {
  background: var(--td-brand-color-1);
}
.dash-kpi-card.is-warn .dash-kpi-card__icon,
.dash-kpi-card.theme-warning .dash-kpi-card__icon {
  background: var(--td-warning-color-1);
  color: var(--td-warning-color);
}
.dash-kpi-card.is-warn .dash-kpi-card__num,
.dash-kpi-card.theme-warning .dash-kpi-card__num {
  color: var(--td-warning-color);
}
.dash-kpi-card.theme-success .dash-kpi-card__icon {
  background: var(--td-success-color-1);
  color: var(--td-success-color);
}
.dash-kpi-card.theme-success .dash-kpi-card__num {
  color: var(--td-success-color);
}
.dash-kpi-card.theme-neutral .dash-kpi-card__icon {
  background: var(--td-bg-color-container);
  color: var(--td-text-color-secondary);
}
.dash-kpi-card.theme-neutral .dash-kpi-card__num {
  color: var(--td-text-color-primary);
}
</style>
