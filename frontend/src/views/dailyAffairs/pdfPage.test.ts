import assert from 'node:assert/strict'
import test from 'node:test'
import { pdfPageOf, type PdfPageRow } from './pdfPage'

// 物理页码跳页闭环（第一批次 P2-5 遗留项）：
// 列表行同时带 page（票号序号，删除接口定位键，**不能改语义**）与
// page_no（PDF 物理页，来自 SourceLocators）。跳页/跳印只能认物理页；
// page_no 缺失（老数据/纯文本引擎）才回退序号。
test('pdfPageOf 优先取物理页码', () => {
  assert.equal(pdfPageOf({ page: 2, pageNo: 10 } as PdfPageRow), 10)
  assert.equal(pdfPageOf({ page: 2, page_no: 11 } as PdfPageRow), 11)
})

test('pdfPageOf page_no 缺失时回退序号', () => {
  assert.equal(pdfPageOf({ page: 3, pageNo: 0 } as PdfPageRow), 3)
  assert.equal(pdfPageOf({ page: 3 } as PdfPageRow), 3)
})

test('pdfPageOf 两者都缺失返回 0', () => {
  assert.equal(pdfPageOf({} as PdfPageRow), 0)
  assert.equal(pdfPageOf(null), 0)
})
