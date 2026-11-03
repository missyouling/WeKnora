import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

// 第 26 轮：发票管理 O-1~O-8 重构的契约锁。
//
// 这批改动全是「结构迁移」类的，行为等价但实现位置变了。源断言的价值在于
// 防止后续有人把自写块又粘回来 —— 那是本轮之前的状态（三页各一份复制粘贴）。

const read = (p: string) => readFileSync(new URL(`./${p}`, import.meta.url), 'utf8')

// ---- O-1：批量工具栏统一到中台 ----

test('O-1a：BusinessFilterBar 必须透传 batch-actions slot', () => {
  const src = read('BusinessFilterBar.vue')
  assert.match(src, /<slot\s+name="batch-actions"\s*\/>/, '缺少 batch-actions slot 透传')
  // slot 必须放在 BusinessListToolbar 内部（放在外面等于没透传）
  const toolbarAt = src.indexOf('<BusinessListToolbar')
  const slotAt = src.indexOf('<slot name="batch-actions"')
  assert.ok(toolbarAt >= 0 && slotAt > toolbarAt, 'batch-actions slot 必须在 BusinessListToolbar 内')
})

for (const page of ['InvoiceManagement.vue', 'ContractManagement.vue', 'AwardPunishManagement.vue']) {
  test(`O-1b：${page} 不得再自写 .doc-batch-bar 浮条`, () => {
    const src = read(page)
    assert.doesNotMatch(
      src,
      /class="doc-batch-bar"/,
      '自写批量浮条必须迁移到中台 BusinessListToolbar 的 batch-actions slot',
    )
    assert.doesNotMatch(src, /hide-batch-bar/, '必须移除 hide-batch-bar（否则中台浮条被关掉）')
    assert.match(src, /#batch-actions/, '必须使用 batch-actions slot 承载按钮')
  })
}

test('O-1c：三页清除选择必须走 clearSelectionSafe（关闭残留确认弹窗）', () => {
  for (const page of ['InvoiceManagement.vue', 'ContractManagement.vue', 'AwardPunishManagement.vue']) {
    const src = read(page)
    // 发票页有 delPopVisible 三选一弹窗；裸 clearSelection 会让弹窗留在屏幕上
    assert.match(
      src,
      /@clear-selection="clearSelectionSafe"/,
      `${page} 的 clear-selection 必须绑 clearSelectionSafe`,
    )
  }
})

// ---- O-7：页码徽标改中性色 ----

test('O-7：页码徽标必须用中性色 token，与「提取成功」绿色解耦', () => {
  const src = read('InvoiceManagement.vue')
  const block = src.slice(src.indexOf('.row-page-badge'))
  const decl = block.slice(0, block.indexOf('}'))
  assert.doesNotMatch(decl, /--td-success-color/, '页码徽标不得再用成功色（会被误读成状态）')
  assert.match(decl, /--td-bg-color-container-hover/, '页码徽标应使用中性背景 token')
  assert.match(decl, /--td-text-color-secondary/, '页码徽标应使用次要文字 token')
})

// ---- O-8：清理历史注释 ----

test('O-8：不得残留「底部摘要栏已下线」历史注释', () => {
  const src = read('InvoiceManagement.vue')
  assert.doesNotMatch(src, /底部摘要栏已下线/, '历史下线注释应清理')
})

// ---- O-4：孤儿函数 sanitizeNum ----

test('O-4：sanitizeNum 必须已删除（第 23 轮移除 @input 后再无生产调用）', () => {
  const src = read('businessDetailForm.ts')
  assert.doesNotMatch(src, /export function sanitizeNum/, '孤儿函数应删除')
  const testSrc = read('businessDetailForm.test.ts')
  assert.doesNotMatch(testSrc, /sanitizeNum/, '对应单测应一并删除')
})
