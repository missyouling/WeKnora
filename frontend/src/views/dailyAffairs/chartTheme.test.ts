import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { chartPalette } from './chartTheme'

/**
 * 图表配色必须跟随主题（第 22 轮 P1-6）。
 *
 * 能耗页的「本期/上期占比」柱状图原先把品牌蓝写死在 option 里，
 * 主题切绿后这张图仍是蓝的 —— 一屏里只有它是旧配色，一眼看出没跟上改版。
 *
 * 这里锁两件事：
 *   ① 没有 DOM（单测/SSR）时必须回落到主题默认色，不能返回空串
 *      （空串会让 ECharts 画出透明图形，比颜色不对更难排查）；
 *   ② 页面源码里不得再出现写死的图表色值。
 */

test('chartPalette：无 DOM 时回落到主题默认值（不得为空串）', () => {
  const c = chartPalette()
  for (const [k, v] of Object.entries(c)) {
    assert.equal(typeof v, 'string', `${k} 必须是字符串`)
    assert.ok(v.trim().length > 0, `${k} 不得为空串`)
    assert.match(v, /^#[0-9a-fA-F]{6}$/, `${k} 必须是 6 位十六进制色值，实际 ${v}`)
  }
  // 主色就是项目主题绿
  assert.equal(c.brand, '#07c05f')
  assert.notEqual(c.brand, '#0052d9', '不得再回落到旧品牌蓝')
  // 浅/深两档必须与主色区分开（否则本期与上期柱子分不出来）
  assert.notEqual(c.brandWeak, c.brand)
  assert.notEqual(c.brandDeep, c.brand)
})

test('能耗页图表不再写死色值，且确实用 chartPalette 取色', () => {
  const src = readFileSync(new URL('./UtilitiesManagement.vue', import.meta.url), 'utf8')
  assert.match(src, /import \{ chartPalette \} from '\.\/chartTheme'/, '必须引入 chartPalette')
  assert.match(src, /const c = chartPalette\(\)/, '渲染前必须取当前主题配色')
  // 旧的三个硬编码图表色不得再出现（注释里提到不算：去掉注释行再判）
  const code = src
    .split('\n')
    .filter((l) => !l.trim().startsWith('//'))
    .join('\n')
  for (const bad of ['#0052d9', '#9ab6e8', '#12395f']) {
    assert.ok(!code.includes(bad), `图表配置仍有硬编码色值 ${bad}`)
  }
})
