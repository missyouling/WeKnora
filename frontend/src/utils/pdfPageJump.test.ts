import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { pageJumpZoom, withPageFragment } from './pdfPageJump.ts'

/**
 * 背景：发票详情抽屉按物理页码定位 PDF。Chrome 内置阅读器默认 fit-page，
 * 每页高度≈视口高度，末页页顶滚不到视口顶部，`#page=N` 被滚动边界夹紧，
 * 页码指示停在 N-1（6 页票据实测停在 3/6）。只有给出数值缩放，
 * 阅读器才按真实页偏移滚动。
 */

const DRAWER_W = 586 // 抽屉预览窗格实测宽（Drawer 654 - 内边距）
const DRAWER_H = 800 // @preview-max-h = calc(100vh - 200px)，1000px 窗口下实测值
const A4_W_100 = 793
const A4_H_100 = 1123

test('抽屉窗格：缩放同时满足「末页可达」与「无横向滚动」', () => {
  const zoom = pageJumpZoom(DRAWER_W, DRAWER_H)

  // 末页可达：整页高度不小于窗格高度，滚动才不会被夹紧
  assert.ok(
    A4_H_100 * (zoom / 100) >= DRAWER_H,
    `zoom=${zoom} 太小，第 6 页会像修复前一样被夹紧到 3/6`,
  )
  // 无横向滚动：页宽不超过窗格宽（等价于被 Chrome 忽略的 zoom=page-width）
  assert.ok(A4_W_100 * (zoom / 100) <= DRAWER_W + 1, `zoom=${zoom} 过大会横向溢出`)
})

test('窗格高于一页时抬高缩放，保证末页页码可达', () => {
  // 800 高的窗格需要 72%，1000 高的窗格必须抬到 90%，否则末页仍被夹紧
  assert.equal(pageJumpZoom(586, 1000), 90)
})

test('宽窗格不做无谓放大', () => {
  assert.equal(pageJumpZoom(1200, 700), 100)
  assert.equal(pageJumpZoom(900, 800), 100)
})

test('尺寸未知时退回浏览器默认缩放（不写 zoom 参数）', () => {
  assert.equal(pageJumpZoom(0, DRAWER_H), 0)
  assert.equal(pageJumpZoom(DRAWER_W, 0), 0)
  assert.equal(pageJumpZoom(Number.NaN, 800), 0)
})

test('withPageFragment 合并页码与缩放，无页码时原样返回', () => {
  const url = 'https://cos.example.com/a.pdf?q-signature=x'

  assert.equal(withPageFragment(url, 6, 73), `${url}#page=6&zoom=73`)
  assert.equal(withPageFragment(url, 6, 0), `${url}#page=6`)
  // 没有目标页码时不改缩放，保持浏览器默认阅读体验
  assert.equal(withPageFragment(url, 0, 73), url)
  assert.equal(withPageFragment('', 6, 73), '')
})

test('document-preview 的 PDF iframe 先完成布局再赋 src', () => {
  const source = readFileSync(new URL('../components/document-preview.vue', import.meta.url), 'utf8')

  // iframe 的 src 必须来自独立状态（空值时不落到 about:blank 之外），
  // 否则 src 会与容器同时创建，插件在布局稳定前就应用 #page，页码被夹紧。
  assert.match(source, /const pdfSrc = ref\(''\)/)
  assert.match(source, /:src="pdfSrc \|\| undefined"/)
  // 缩放必须按 iframe 实测尺寸计算，不能在模板里写死
  assert.match(source, /pageJumpZoom\(/)
  assert.match(source, /from '@\/utils\/pdfPageJump'/)
})
