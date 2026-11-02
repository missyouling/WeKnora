/**
 * 图表配色：从 TDesign 主题变量实时取值（第 22 轮 P1-6）。
 *
 * 为什么不能硬编码：能耗页的「本期 / 上期占比」柱状图原先写死旧品牌蓝
 * `#0052d9` + 浅蓝 `#9ab6e8` + 深蓝字 `#12395f`，而项目主题早已切到绿色
 * `#07c05f`。于是同一屏里卡片、按钮、进度条都是绿的，只有这张图是蓝的 ——
 * 用户一眼就能看出「这块没跟上改版」。
 *
 * 做法：ECharts 不吃 CSS 变量，只能在渲染时把变量解析成具体色值。
 * 每次渲染现算（不缓存）：主题切换（暗色模式 / 品牌色调整）后下一帧就正确，
 * 缓存在这里省不了多少，却会留下「切主题后图表还是旧色」的坑。
 *
 * 兜底值与 `--td-brand-color` 的默认主题一致；变量缺失（单元测试环境没有
 * document、SSR）时用兜底，保证不返回空串导致 ECharts 渲染透明图形。
 */

/** 图表用到的全部语义色。 */
export interface ChartPalette {
  /** 主序列（本期）填充 + 数值标签 */
  brand: string
  /** 次序列（上期）填充：主色的浅一档 */
  brandWeak: string
  /** 浅色柱体上的文字：主色的深一档，保证对比度 ≥ 4.5:1 */
  brandDeep: string
  /** 图例 / 轴标签文字 */
  text: string
  /** 轴线 */
  axis: string
  /** 网格线 */
  grid: string
}

const FALLBACK: ChartPalette = {
  brand: '#07c05f',
  brandWeak: '#8fdcb4',
  brandDeep: '#04532a',
  text: '#666666',
  axis: '#dcdcdc',
  grid: '#eeeeee',
}

/** 读一个 CSS 变量（去空白）；取不到返回空串。 */
function cssVar(name: string): string {
  if (typeof window === 'undefined' || typeof document === 'undefined') return ''
  const v = getComputedStyle(document.documentElement).getPropertyValue(name)
  return String(v || '').trim()
}

/**
 * 当前主题下的图表配色。
 *
 * 变量名沿用 TDesign 约定（`--td-brand-color-{1..10}` 由浅到深）：
 *   · 本期柱 = `--td-brand-color`（品牌主色）
 *   · 上期柱 = `--td-brand-color-4`（浅一档，够区分又不像「另一个色系」）
 *   · 柱上文字 = `--td-brand-color-8`（深一档，浅柱白字对比度不足，改深字）
 */
export function chartPalette(): ChartPalette {
  return {
    brand: cssVar('--td-brand-color') || FALLBACK.brand,
    brandWeak: cssVar('--td-brand-color-4') || FALLBACK.brandWeak,
    brandDeep: cssVar('--td-brand-color-8') || FALLBACK.brandDeep,
    text: cssVar('--td-text-color-secondary') || FALLBACK.text,
    axis: cssVar('--td-component-border') || FALLBACK.axis,
    grid: cssVar('--td-component-stroke') || FALLBACK.grid,
  }
}
