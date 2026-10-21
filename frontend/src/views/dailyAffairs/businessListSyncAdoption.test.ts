import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const read = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8')

const PAGES = [
  { file: 'ContractManagement.vue', rows: 'contractRows', label: '合同' },
  { file: 'RegulationManagement.vue', rows: 'regulationRows', label: '制度' },
  { file: 'AwardPunishManagement.vue', rows: 'noticeRows', label: '公告' },
]

// 维度 7：三页删除必须统一走中台 useBusinessListSync（乐观移除 + 延迟对账 +
// 级联顺序），不能再各自维护一份定时器数组——三份手写实现行为漂移过一轮
// （合同页面缺 uploadStats 刷新、制度页面卸载时漏清定时器）。
for (const p of PAGES) {
  test(`${p.label}页删除编排统一走 useBusinessListSync`, () => {
    const src = read(p.file)
    assert.match(src, /import \{ useBusinessListSync \} from '\.\/useBusinessListSync'/, '缺少中台删除编排 import')
    assert.match(src, new RegExp(`useBusinessListSync\\(${p.rows},`), '未把本页行数据接入中台编排')
    // 必须注入自己的刷新与删除接口
    assert.match(src, /deleteFiles: \(ids\) => batchDeleteKnowledge\(kbId\.value, ids\)/)
    assert.match(src, /reloadStats: \(\) => \{/)
    // 卸载必须销毁对账定时器
    assert.match(src, /listSync\.destroy\(\)/, '组件卸载必须销毁延迟对账定时器')
    // 旧的手写实现必须彻底移除（残留会导致双重刷新 + 定时器泄漏）
    assert.equal(/deleteRefreshTimers/.test(src), false, '不得再保留手写定时器数组')
    assert.equal(/setTimeout\(reload, 2600\)/.test(src), false, '延迟对账节奏必须由中台统一')
  })
}

test('发票页仍是删除编排的基准实现（未被反向改坏）', () => {
  const src = read('InvoiceManagement.vue')
  assert.match(src, /useBusinessListSync\(invoiceRows,/)
  assert.match(src, /deletePage: \(kid, page\) => deleteInvoicePage\(kbId\.value, kid, page\)/)
  assert.match(src, /listSync\.destroy\(\)/)
})
