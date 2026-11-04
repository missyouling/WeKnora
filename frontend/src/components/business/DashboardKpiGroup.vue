<template>
  <!-- 加载态骨架：父页面在异步数据源未齐时传 loading，占位避免卡片分批跳变。
       注意 v-if 会让 groupRef 在挂载时不存在 —— 箭头的事件绑定必须能事后补绑，
       否则「先 loading 后出卡片」的页面永远看不到左右箭头（见下方 bindGroup）。 -->
  <div v-if="loading && skeleton" class="dash-kpi-skeleton" aria-hidden="true">
    <div v-for="i in 5" :key="i" class="dash-kpi-skeleton__card" />
  </div>
  <div v-else class="dash-kpi-wrapper">
    <div ref="groupRef" class="dash-kpi-group">
      <div
        v-for="card in cards"
        :key="card.key"
        class="dash-kpi-card"
        :class="[card.cls, card.numCls, { 'is-active': isActive(card), 'is-static': card.static }]"
        @click="onCardClick(card)"
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
    <!-- 溢出导航：箭头定位在 wrapper（不随滚动内容移动），滚动后仍固定在视口左右边缘 -->
    <button
      v-if="canLeft"
      type="button"
      class="dash-kpi-arrow is-left"
      aria-label="向左滚动"
      @click="scrollByStep(-1)"
    >
      <t-icon name="chevron-left" />
    </button>
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
import { ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
// 概览 KPI 卡片组（中台统一组件）：
// 提炼自 InvoiceManagement.vue 的 type-card 范式 —— flex 左右布局 + 40px 圆角图标底 +
// hover 上浮 + 大字号数值（超长自动降档）。卡片点击由父级通过 card-click 事件联动业务。
//
// cards 数据形状（与发票概览 overviewCards 完全一致，接入零改动）：
//   { key, label, icon, value, unit, sub, cls, numCls, action?, typeValue? }
// - cls：视觉语义类，透传绑定（'' 中性 / is-brand 品牌 / is-warn 告警）
// - numCls：主数值降档类（'' / num-sm / num-xs），超长金额防换行
// - theme：可选语义（brand/success/warning/neutral），内部映射为对应 cls，供新模块复用
// - static：纯展示卡（如「已收录证照」总数卡），不响应点击、无 hover 指针、永不高亮
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
  /** 纯展示卡：不可点击、无 hover 上浮、不参与激活态 */
  static?: boolean
}

// activeType：父组件当前筛选的分类值（如 filterInvoiceType/filterContractType）。
// 高亮规则：action=all 卡在 activeType 为空时激活；action=type 卡在 typeValue 与 activeType 相等时激活。
// action=stat（纯统计卡）永不激活 —— 它不参与列表筛选。
const props = withDefaults(
  defineProps<{ cards: KpiCard[]; activeType?: string; loading?: boolean; skeleton?: boolean }>(),
  { skeleton: true },
)
const emit = defineEmits<{ (e: 'card-click', card: KpiCard): void }>()

// 纯展示卡直接吞掉点击：父级不收到事件，列表不会切到汇总视图
const onCardClick = (card: KpiCard) => {
  if (card.static || card.action === 'stat') return
  emit('card-click', card)
}

const isActive = (card: KpiCard): boolean => {
  if (card.static || card.action === 'stat') return false
  if (card.action === 'all') return !props.activeType
  if (card.action === 'type' && card.typeValue) return props.activeType === card.typeValue
  return false
}

// ---- 横向溢出导航：滚动条隐藏后提供左右箭头入口 ----
// 仅当内容溢出（scrollWidth > clientWidth）时箭头才出现，滚动到边界自动隐藏对应箭头。
//
// 绑定时机是这里的核心坑：父级常见写法 `:loading="!ready"` 会让 groupRef 在挂载时
// 根本不存在（v-if 分支），onMounted 里拿不到元素 → scroll 监听与 ResizeObserver
// 全都没绑上，卡片加载出来后箭头永远不出现（真机现象：7 张卡被截断且无箭头）。
// 因此改为「ref 变化 / 卡片数量变化 / loading 变化」三处都尝试补绑。
const groupRef = ref<HTMLElement>()
const canLeft = ref(false)
const canRight = ref(false)
let groupRO: ResizeObserver | null = null
let scrollRaf = 0
let boundEl: HTMLElement | null = null

const updateArrows = () => {
  const el = groupRef.value
  if (!el) return
  canLeft.value = el.scrollLeft > 2
  canRight.value = el.scrollLeft + el.clientWidth < el.scrollWidth - 2
}

const unbindGroup = () => {
  if (boundEl) {
    boundEl.removeEventListener('scroll', updateArrows)
    boundEl.removeEventListener('wheel', onWheel)
  }
  if (groupRO) { groupRO.disconnect(); groupRO = null }
  boundEl = null
}

// 滚轮横向滚动（第 32 轮）：
// 卡片溢出时（scrollWidth > clientWidth），鼠标滚轮 / 触控板的纵向增量转成横向滚动，
// 否则用户会以为右侧卡片「被截断」且不知道有箭头。未溢出时什么都不做，
// 页面纵向滚动保持原样（不能让整页在卡片区卡住）。
// passive:false 是必须的 —— preventDefault 在 passive 监听里无效。
const onWheel = (e: WheelEvent) => {
  const el = groupRef.value
  if (!el) return
  if (el.scrollWidth <= el.clientWidth + 1) return
  const delta = e.deltaY || e.deltaX
  if (!delta) return
  e.preventDefault()
  if (scrollRaf) { cancelAnimationFrame(scrollRaf); scrollRaf = 0 }
  el.scrollLeft += delta
  updateArrows()
}

const bindGroup = (el?: HTMLElement | null) => {
  if (!el) return
  if (boundEl === el) { updateArrows(); return }
  unbindGroup()
  boundEl = el
  el.addEventListener('scroll', updateArrows, { passive: true })
  el.addEventListener('wheel', onWheel, { passive: false })
  // 同时观察自身与父容器：内容变宽不一定改变自身宽度（flex 子项 min-width 固定时
  // 只有 scrollWidth 变），父容器 resize 也要重算。
  groupRO = new ResizeObserver(() => updateArrows())
  groupRO.observe(el)
  if (el.parentElement) groupRO.observe(el.parentElement)
  updateArrows()
}

// 手动 rAF 驱动滚动动画：原生 scrollBy({behavior:'smooth'}) 的滚动事件时序不可靠
// （真实窄窗/动画中断场景下箭头状态不随滚动更新），改为每帧设置 scrollLeft 并强制
// 刷新箭头状态，滚动结束保证左右箭头与边界严格同步。
// 步长固定 260px：与需求给定的滚动距离一致，且小于单卡最小宽度 200+gap，不会跳过整卡。
const SCROLL_STEP = 260
const scrollByStep = (dir: -1 | 1) => {
  const el = groupRef.value
  if (!el) return
  const target = Math.max(0, Math.min(el.scrollWidth - el.clientWidth, el.scrollLeft + SCROLL_STEP * dir))
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

// ref 就绪 / 卡片增减 / loading 结束，三处都要重新绑定 + 重算（含字体加载后的抖动兜底）
watch(groupRef, (el) => { nextTick(() => bindGroup(el as HTMLElement | null)) }, { flush: 'post' })
watch(() => props.cards.length, () => { nextTick(() => { bindGroup(groupRef.value); setTimeout(updateArrows, 80) }) })
watch(() => props.loading, () => { nextTick(() => { bindGroup(groupRef.value); setTimeout(updateArrows, 80) }) })

onMounted(() => { nextTick(() => bindGroup(groupRef.value)) })

onBeforeUnmount(() => {
  if (scrollRaf) cancelAnimationFrame(scrollRaf)
  unbindGroup()
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
.dash-kpi-wrapper {
  position: relative;
  /* 负外边距抵消内部 group 的垂直 padding，保证与旧版整体布局间距一致 */
  margin-top: -12px;
  margin-bottom: -12px;
}
.dash-kpi-group {
  display: flex;
  flex-wrap: nowrap;
  /* 卡片等高：flex 默认 align-items:stretch，但卡片是 flex-shrink:0 的块，
     高度仍由内容决定 —— 有副文本的卡（发票「已收录」/车队「历史记录」）
     比没有副文本的卡高 20px，同一行里就会出现参差。
     显式 stretch + 卡片 align-self:stretch，让所有卡统一取行高，
     这样能耗 / 发票 / 车队 12 个菜单的卡片尺寸才真正一致。 */
  align-items: stretch;
  overflow-x: auto;
  overflow-y: hidden;
  gap: 14px;
  /* 垂直/水平内边距：为 hover 上浮与 box-shadow（--td-shadow-1 最大 blur 10px）
     提供完整渲染空间，避免边缘卡片阴影/边框被 overflow 裁剪出平齐断层；
     对外布局的位移由外层 wrapper 的等量负外边距抵消 */
  padding: 12px 8px;
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
  /* 卡片统一高度（第 34 轮）：
     ① align-self:stretch —— 同一行内所有卡取同一行高（有副文本的卡不再比
        没副文本的高 20px）；
     ② min-height 固定 —— 跨**菜单/模块**也要同高。只靠 stretch 时，行高由
        该组内最高卡决定：车辆档案组有「历史记录」带副文本 → 全组 104px；
        加油充电组没有副文本 → 全组 83px。两个菜单的卡片看起来就"不一样大"。
        给一个统一最小高度后，有无副文本都是同一尺寸，副文本只是在这个高度内
        垂直居中排布。 */
  align-self: stretch;
  box-sizing: border-box;
  /* 固定尺寸（第 34 轮）：
     · height 写死 96px —— 有/无副文本、组内最高卡多高都不影响，
       跨菜单跨模块统一（发票/能耗/车队 12 菜单实测均为 96px）；
     · width 写死 220px —— 只设 min-width 时，标题长的卡（「道路运输经营许可证」）
       会被内容撑到 222px，与相邻的 200px 卡看起来"不一样宽"。
       固定宽度后标题超长走 ellipsis，卡片网格严格对齐。 */
  width: 220px;
  height: 96px;
  min-width: 220px;
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
/* 纯展示卡（如「已收录证照」总数）：不暗示可点击 —— 无手型、无上浮、无高亮边框 */
.dash-kpi-card.is-static {
  cursor: default;
}
.dash-kpi-card.is-static:hover {
  background: var(--wk-kpi-bg);
  border-color: transparent;
  transform: none;
  box-shadow: none;
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
  /* 卡片宽度固定后，超长标题（「道路运输经营许可证」）必须省略，
     否则会把 220px 撑破或换行导致卡片内容溢出 */
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
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
  /* 行高压到 16px 且不占额外外边距：卡片高度写死 96px 后，
     副文本必须在剩余空间内放下（图标 40 / 标题 18 / 数值 23 + 副文本 16 = 97，
     由 flex 居中消化 1px 误差）。 */
  line-height: 16px;
  color: var(--td-text-color-secondary);
  margin-top: 2px;
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
