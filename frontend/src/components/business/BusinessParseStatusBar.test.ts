import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const src = readFileSync(new URL('./BusinessParseStatusBar.vue', import.meta.url), 'utf8')

test('状态条不再显示「已用 X 分 Y 秒」（与百分比混排会让人以为卡死）', () => {
  assert.equal(/已用/.test(src), false, '不允许出现「已用」耗时文案')
  assert.equal(/formatElapsed/.test(src), false, '耗时格式化函数必须删除')
})

test('状态条统一文案：阶段 + 文件名 + 剩余份数', () => {
  assert.match(src, /const statusText = computed/)
  assert.match(src, /（剩余 \$\{rest\} 份）/)
  // 上传阶段才展示真实百分比，解析/提取阶段不猜进度
  assert.match(src, /p\.stage === 'uploading' && p\.percent !== null/)
})

test('进行中计数用 computed，文件到终态后横幅立即消失', () => {
  assert.match(src, /const liveCount = computed/)
})
