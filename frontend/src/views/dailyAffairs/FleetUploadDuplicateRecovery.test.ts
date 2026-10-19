import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const dialog = readFileSync(new URL('./FleetUploadDialog.vue', import.meta.url), 'utf8')

// 上传去重冲突的「智能恢复」契约。
// 背景：用户上传通行费汇总 PDF 时被「知识库中已有相同文件」拦住，而该文件其实
// 早已在库（车牌 87567 只是搜不到）。上传弹窗必须区分两种情况：
//   ① 库中文件没有可用票据（未提取 / 提取失败）→ 自动改分类 + 重新提取；
//   ② 库中文件已有正常票据 → 提示当前分类/开票日期 + 给手动重新提取入口。
// 两种情况都不允许把用户丢在一句无下一步操作的错误提示上。

test('recovery：重复冲突必须读取后端 recovery 摘要（不再只翻前 100 条猜）', () => {
  assert.match(dialog, /recovery/, '必须读取后端 recovery 摘要')
  assert.match(
    dialog,
    /err\?\.recovery|err\?\.data\?\.recovery|res\?\.recovery/,
    '必须从错误响应体取 recovery',
  )
})

test('recovery：可自动重提取时不得标记为失败/重复，要走重新提取流程', () => {
  assert.match(dialog, /can_reextract/, '必须判定 can_reextract')
  // 关键：recovery.can_reextract 为真的分支不得进入 t.status = 'duplicate'
  const block = dialog.slice(
    dialog.indexOf('can_reextract') - 200,
    dialog.indexOf('can_reextract') + 1600,
  )
  assert.doesNotMatch(block, /t\.status = 'duplicate'/, '可重提取时不得标记为 duplicate')
})

test('recovery：自动重提取必须带上用户本次选择的分类（certType）', () => {
  const idx = dialog.indexOf('can_reextract')
  assert.ok(idx > 0, '必须先判定 can_reextract')
  const block = dialog.slice(idx, idx + 2000)
  assert.match(block, /certType/, '重新提取必须携带本次选择的分类')
})

test('recovery：已有正常票据时给出可操作提示（当前分类 + 开票日期），不是死路', () => {
  assert.match(dialog, /recovery\?\.normal/, '必须识别 Normal 场景')
  assert.match(dialog, /recovery\?\.invoice_count|recovery\?\.InvoiceCount/, '必须给出已提取张数')
  assert.match(dialog, /recovery\?\.category/, '必须给出当前分类')
  assert.match(dialog, /recovery\?\.invoice_date/, '必须给出开票日期')
})

test('recovery：duplicate 状态文案必须提示可操作（不能只说"知识库中已有相同文件"）', () => {
  assert.match(dialog, /重新提取|重新上传/, '重复状态必须给出下一步操作')
})

test('recovery：不得再依赖 page_size 100 的单页扫描定位已有文件', () => {
  assert.doesNotMatch(
    dialog,
    /page_size: 100[\s\S]{0,400}find\(\(k[^)]*\) =>/,
    '不得再靠第一页 100 条名称匹配定位文件——文件多时必然漏检',
  )
})
