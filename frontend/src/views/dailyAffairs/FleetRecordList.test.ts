import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const list = readFileSync(new URL('./FleetRecordList.vue', import.meta.url), 'utf8')

test('工具栏提示条优先展示上传任务实时进度（多文件轮播），无活动任务回退列表轮询兜底', () => {
  assert.match(list, /v-if="uploadProgress\.length \|\| pendingFiles\.length"/)
  assert.match(list, /v-if="uploadProgress\.length"/)
  assert.match(list, /stageLabel\(uploadProgress\[progressIndex\]\.stage\)/)
  assert.match(list, /「\{\{ uploadProgress\[progressIndex\]\.name \}\}」进度/)
  assert.match(list, /progressIndex \+ 1 \}\}\/\{\{ uploadProgress\.length \}\}/)
  assert.match(list, /v-else>正在解析 \{\{ pendingFiles\.length \}\} 个文件/)
})

test('上传进度阶段文案：正在上传 / 正在解析 / 正在提取', () => {
  assert.match(list, /uploading: '正在上传'/)
  assert.match(list, /parsing: '正在解析'/)
  assert.match(list, /extracting: '正在提取'/)
})

test('多文件同时进行时轮播展示：3 秒切换，单任务停止轮播', () => {
  // 轮播随「解析/提取状态横幅」中台化到了 BusinessParseStatusBar（车队/发票共用），
  // 列表页只负责把进度喂进去——断言跟着实现走，避免测试锁死在已搬走的旧位置。
  const bar = readFileSync(new URL('../../components/business/BusinessParseStatusBar.vue', import.meta.url), 'utf8')
  assert.match(list, /function onUploadProgress/)
  // 车队档案页保留自己的简版横幅（无轮播），轮播只存在于中台横幅
  assert.match(list, /archive-pending-bar/)
  assert.match(bar, /setInterval\(\(\) => \{/)
  assert.match(bar, /\}, 3000\)/)
  assert.match(bar, /progressIndex\.value = \(progressIndex\.value \+ 1\) % props\.progress\.length/)
  // 单任务（或降到 1 个）时停止轮播
  assert.match(bar, /props\.progress\.length <= 1/)
  assert.match(bar, /clearInterval\(progressTimer\)/)
})

test('页面卸载时清理轮播定时器', () => {
  assert.match(list, /stopPolling\(\)/)
  assert.match(list, /stopProgressTimer\(\)/)
})

test('设置抽屉变更分类后同步刷新：监听全局事件重载分类，卸载时移除', () => {
  assert.match(list, /addEventListener\('fleet-categories-changed', onCategoriesChanged\)/)
  assert.match(list, /removeEventListener\('fleet-categories-changed', onCategoriesChanged\)/)
  assert.match(list, /async function reloadCategories\(\)/)
  assert.match(list, /function onCategoriesChanged\(\)/)
})
