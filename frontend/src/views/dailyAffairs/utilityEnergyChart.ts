// 用能分析图表的配置构建（第 24 轮任务一：UtilitiesManagement 瘦身）。
//
// 抽出的是**纯配置对象**：给定「本期/上期分时占比行」+ 主题色，返回 ECharts option。
// 图表实例的创建、resize、dispose 仍留在页面 —— 那是 DOM 生命周期，
// 与数据无关，抽出去反而要把 ref/nextTick 一起搬，得不偿失。
//
// 抽出的价值：option 里的每个取值都能在单测里断言（配色必须来自主题变量、
// 上期缺失时不得画空柱、纵轴上限公式），而不是靠肉眼看图。
import type { EChartsOption } from 'echarts'
import type { EnergyCompareRow } from './utilityBillDetail'

/** 主题色：由页面从主题变量取出后传入，本模块不得自行取色、也不得写死色值。 */
export interface ChartColors {
  text: string
  axis: string
  grid: string
  brand: string
  brandWeak: string
  brandDeep: string
}

/** 图表容器高度（模板里 .ea-echart 用的就是这个值，改一处即可）。 */
export const ENERGY_CHART_HEIGHT = 160

/**
 * 构建「分时电量占比对比」的 ECharts option。
 *
 * 三条口径（都踩过坑）：
 *   ① 配色一律取主题变量：旧实现把品牌蓝写死成字面色值，主题切绿后这张图还是蓝的；
 *   ② 上期不存在时第二组柱子给 `null`（不是 0）—— 画成 0 柱会让用户以为上期是 0%；
 *   ③ 上期柱的标签放柱体正中（inside），不要 insideTop：柱体矮时顶端标签会压在上边缘
 *      与柱色糊成一片；浅色柱上用主色深档（≈6:1），白字只有 2.2:1 读不清。
 */
export function buildEnergyChartOption(
  rows: EnergyCompareRow[],
  prevExists: boolean,
  c: ChartColors,
): EChartsOption {
  const pctLabel = (p: any) => `${p.value}%`
  return {
    animation: true,
    animationDuration: 600,
    animationEasing: 'cubicOut' as const,
    // 图表高度收到 160px 后，顶部留白也要跟着收（42 → 30），否则绘图区被压扁
    grid: { left: 8, right: 8, top: 30, bottom: 2, containLabel: true },
    tooltip: {
      trigger: 'axis' as const,
      formatter: (ps: any) => {
        const arr = Array.isArray(ps) ? ps : [ps]
        return arr.map((p: any) => `${p.seriesName}：${p.value}%`).join('<br/>')
      },
    },
    legend: {
      data: ['本期占比', '上期占比'],
      top: 0,
      itemWidth: 12,
      itemHeight: 8,
      textStyle: { fontSize: 12, color: c.text },
    },
    xAxis: {
      type: 'category' as const,
      data: rows.map((r) => r.label),
      axisLine: { lineStyle: { color: c.axis } },
      axisLabel: { color: c.text, fontSize: 12 },
      axisTick: { show: false },
    },
    yAxis: {
      type: 'value' as const,
      // 上限留 15% 余量再向上取整到 10 的倍数：柱子顶到框线会让标签被裁
      max: (v: any) => Math.ceil((v.max * 1.15) / 10) * 10,
      axisLabel: { color: c.text, fontSize: 12, formatter: '{value}%' },
      splitLine: { lineStyle: { color: c.grid } },
    },
    series: [
      {
        name: '本期占比',
        type: 'bar' as const,
        data: rows.map((r) => r.curPct),
        barWidth: 22,
        itemStyle: { color: c.brand, borderRadius: [3, 3, 0, 0] },
        label: { show: true, position: 'top' as const, distance: 4, color: c.brand, fontSize: 10, formatter: pctLabel },
      },
      {
        name: '上期占比',
        type: 'bar' as const,
        // 无上期给 null：画成 0 柱会被误读成「上期占比 0%」
        data: rows.map((r) => (prevExists ? r.prevPct : null)),
        barWidth: 22,
        itemStyle: { color: c.brandWeak, borderRadius: [3, 3, 0, 0] },
        label: {
          show: prevExists,
          position: 'inside' as const,
          color: c.brandDeep,
          fontSize: 10,
          fontWeight: 600,
          formatter: pctLabel,
        },
      },
    ],
  }
}
