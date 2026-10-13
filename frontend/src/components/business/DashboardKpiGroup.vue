<template>
  <!-- 加载态骨架：父页面在异步数据源未齐时传 loading，占位避免卡片分批跳变 -->
  <div v-if="loading" class="dash-kpi-skeleton" aria-hidden="true">
    <div v-for="i in 5" :key="i" class="dash-kpi-skeleton__card" />
  </div>
  <div v-else ref="groupRef" class="dash-kpi-group">
    <!-- 溢出导航：滚动条被隐藏后，纯鼠标/触摸用户仍能点击箭头将不可见卡片滚入视口 -->
    <button
      v-if="canLeft"
      type="button"
      class="dash-kpi-arrow is-left"
      aria-label="向左滚动"
      @click="scrollByStep(-1)"
    >
      <t-icon name="chevron-left" />
    </button>
    <div
      v-for="card in cards"
      :key="card.key"
      class="dash-kpi-card"
      :class="[card.cls, card.numCls, { 'is-active': isActive(card) }]"
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
    <button
      v-if="canRight"
      type="button"
      class="dash-kpi-arrow is-right"
      aria-label="向右滚动"
      @click="scrollByStep(1)"
    >
      <t-icon name="chevron-right" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
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

// activeType：父组件当前筛选的分类值（如 filterInvoiceType/filterContractType）。
// 高亮规则：action=all 卡在 activeType 为空时激活；action=type 卡在 typeValue 与 activeType 相等时激活。
const props = defineProps<{ cards: KpiCard[]; activeType?: string; loading?: boolean }>()
defineEmits<{ (e: 'card-click', card: KpiCard): void }>()

const isActive = (card: KpiCard): boolean => {
  if (card.action === 'all') return !props.activeType
  if (card.action === 'type' && card.typeValue) return props.activeType === card.typeValue
  return false
}

// ---- 横向溢出导航（方案 B）：滚动条隐藏后提供左右箭头入口 ----
// 仅当内容溢出（scrollWidth > clientWidth）时箭头才出现，滚动到边界自动隐藏对应箭头；
// 监听 scroll 与容器尺寸变化（ResizeObserver）实时刷新状态，卸载时清理监听。
const groupRef = ref<HTMLElement>()
const canLeft = ref(false)
const canRight = ref(false)
let groupRO: ResizeObserver | null = null
let scrollRaf = 0

const updateArrows = () => {
  const el = groupRef.value
  if (!el) return
  canLeft.value = el.scrollLeft > 2
  canRight.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 2
}

// 手动 rAF 驱动滚动动画：原生 scrollBy({behavior:'smooth'}) 的滚动事件时序不可靠
// （真实窄窗/动画中断场景下箭头状态不随滚动更新），改为每帧设置 scrollLeft 并强制
// 刷新箭头状态，滚动结束保证左右箭头与边界严格同步。
const scrollByStep = (dir: -1 | 1) => {
  const el = groupRef.value
  if (!el) return
  const step = el.clientWidth * 0.7
  const target = Math.max(0, Math.min(el.scrollWidth - el.clientWidth, el.scrollLeft + step * dir))
  if (Math.abs(target - el.scrollLeft) < 1) {
    updateArrows()
    return
  }
  if (scrollRaf) cancelAnimationFrame(scrollRaf)
  const start = el.scrollLeft
  const delta = target - start
  const dur = 260
  const t0 = performance.now()
  const tick = (now: number) => {
    const p = Math.min(1, (now - t0) / dur)
    // easeInOutQuad：匀速中段快、两端缓，观感与原生 smooth 一致
    const eased = p < 0.5 ? 2 * p * p : 1 - Math.pow(-2 * p + 2, 2) / 2
    el.scrollLeft = start + delta * eased
    updateArrows()
    if (p < 1) scrollRaf = requestAnimationFrame(tick)
  }
  scrollRaf = requestAnimationFrame(tick)
}

onMounted(() => {
  nextTick(() => {
    const el = groupRef.value
    if (!el) return
    updateArrows()
    el.addEventListener('scroll', updateArrows, { passive: true })
    groupRO = new ResizeObserver(() => updateArrows())
    groupRO.observe(el)
  })
})

onBeforeUnmount(() => {
  if (scrollRaf) cancelAnimationFrame(scrollRaf)
  const el = groupRef.value
  if (el) el.removeEventListener('scroll', updateArrows)
  if (groupRO) groupRO.disconnect()
})
</script>

<style lang="less" scoped>
.dash-kpi-skeleton {
  display: flex;
  gap: 14px;
  padding: 12px 8px;
}
.dash-kpi-skeleton__card {
  flex: 1 1 0;
  min-width: 0;
  height: 100px;
  border-radius: var(--td-radius-large);
  background: var(--td-bg-color-component);
}
.dash-kpi-group {
  position: relative;
  display: flex;
  flex-wrap: nowrap;
  overflow-x: auto;
  overflow-y: hidden;
  gap: 14px;
  /* 垂直/水平内边距：为 hover 上浮与 box-shadow（--td-shadow-1 最大 blur 10px）
     提供完整渲染空间，避免边缘卡片阴影/边框被 overflow 裁剪出平齐断层；
     负外边距等量抵消内边距造成的布局位移，保证与旧版外观间距完全一致 */
  padding: 12px 8px;
  margin-top: -12px;
  margin-bottom: -12px;
  /* 隐藏横向滚动条（Windows 原生滚动条占高度且不美观），保留滚轮/触控板滑动 */
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.dash-kpi-group::-webkit-scrollbar {
  display: none;
}
/* 左右溢出导航箭头：仅在内容溢出时浮现，半透明不遮卡片主体，hover 强化为主题色 */
.dash-kpi-arrow {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 2;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid var(--td-component-stroke);
  border-radius: 50%;
  background: var(--td-bg-color-container);
  color: var(--td-text-color-secondary);
  box-shadow: var(--td-shadow-1);
  cursor: pointer;
  opacity: 0.85;
  transition: all 0.18s ease;
}
.dash-kpi-arrow:hover {
  background: var(--td-bg-color-container-hover);
  color: var(--td-brand-color);
  box-shadow: var(--td-shadow-2);
  opacity: 1;
}
.dash-kpi-arrow.is-left { left: 4px; }
.dash-kpi-arrow.is-right { right: 4px; }
.dash-kpi-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  flex-shrink: 0;
  min-width: 200px;
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
/* 筛选态常驻高亮：与 hover 同级醒目（主题色边框 + 浅色底 + 阴影），
   鼠标移开后保持，作为当前列表所处分类的视觉锚点 */
.dash-kpi-card.is-active {
  background: var(--td-brand-color-light);
  border-color: var(--td-brand-color);
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
