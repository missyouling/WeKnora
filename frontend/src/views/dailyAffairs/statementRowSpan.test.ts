// 第 40 轮：PDF 清单「跨行合并单元格」的数据契约。
//
// 打印预览重构为 6 列（费用类别 | 费用组成 | 时段 | 计费数量 | 单价/标准 | 金额），
// rowSpans 是 StatementTable 上的可选元数据：>1 = 跨行合并、0 = 被覆盖。
// 版式引擎按它按列段画背景/横线，让合并区内部不出现分隔线。
//
// 这里锁死两件最容易回归的事：
//   ① 裁剪空行时 rowSpans 必须同步裁剪（否则合并区错位到别的行）；
//   ② 纯函数的输入不被改动（清单构造器会复用同一份 rows）。
import { strict as assert } from 'node:assert'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { isEmptyDetailRow, pruneEmptyTables, type StatementTable } from './useCatalogPdf'

const COLS = [
  { label: '费用类别', align: 'center' as const },
  { label: '费用组成' },
  { label: '时段', align: 'center' as const },
  { label: '计费数量', align: 'right' as const },
  { label: '单价/标准', align: 'right' as const },
  { label: '金额(元)', align: 'right' as const },
]

test('rowSpans：裁剪 0 行时必须与 rows 同步，长度恒等', () => {
  const t: StatementTable = {
    title: '电费分项细项',
    columns: COLS,
    rows: [
      ['(1) 市场化购电费', '零售交易电费', '尖峰', '5024.28', '0.436', '2190.59'],
      ['', '零售交易电费', '峰', '35645.33', '0.436', '15541.36'],
      ['', '辅助服务费用', '—', '—', '—', '0.00'],
      ['(4) 系统运行费用', '煤电容量电费', '—', '126600', '0.023', '2918.89'],
    ],
    rowSpans: [
      [2, 2, 1, 1, 1, 1],
      [0, 0, 1, 1, 1, 1],
      [0, 1, 1, 1, 1, 1],
      [1, 1, 1, 1, 1, 1],
    ],
    total: ['合计', '', '', '', '', '20650.84'],
  }
  const out = pruneEmptyTables([t])
  assert.equal(out.length, 1)
  // 第 3 行金额 0.00 且数量/单价都为空 → 被裁（0 元但计费数量非 0 的行必须保留）
  assert.equal(out[0].rows.length, 3)
  assert.equal(out[0].rowSpans?.length, 3, 'rowSpans 必须与裁剪后的 rows 等长')
  assert.deepEqual(out[0].rowSpans?.[0], [2, 2, 1, 1, 1, 1])
  assert.deepEqual(out[0].rowSpans?.[2], [1, 1, 1, 1, 1, 1])
  // 原对象不被改动
  assert.equal(t.rows.length, 4)
  assert.equal(t.rowSpans?.length, 4)
})

test('isEmptyDetailRow：6 列结构下只按右对齐数值列判空', () => {
  // 「费用类别」「费用组成」含「费用」二字，不能被当成金额列
  assert.equal(isEmptyDetailRow(COLS, ['(4) 系统运行费用', '辅助服务费用', '—', '126600', '0', '0.00']), false,
    '计费数量非 0 的行不能判为空（辅助服务费用 0 元行必须保留）')
  assert.equal(isEmptyDetailRow(COLS, ['(4) 系统运行费用', '辅助服务费用', '—', '—', '—', '0.00']), true)
})

test('StatementTable.rowSpans 是可选字段：不带合并元数据的表照常裁剪', () => {
  const t: StatementTable = {
    title: '水费核算明细',
    columns: COLS,
    rows: [
      ['总水表', '—', '—', '180', '5.22', '939.60'],
      ['空表', '—', '—', '—', '—', '0.00'],
    ],
    total: ['合计', '', '', '', '', '939.60'],
  }
  const out = pruneEmptyTables([t])
  assert.equal(out[0].rows.length, 1)
  assert.equal(out[0].rowSpans, undefined)
})

// ---------------------------------------------------------------------------
// 第 41 轮：大类小计内嵌「费用类别」列（\n 双行）+ 强制分页
// ---------------------------------------------------------------------------
import { splitCategoryLabel } from './useCatalogPdf'

test('splitCategoryLabel：大类 + 小计拆成两行，单行类别不产生副行', () => {
  assert.deepEqual(splitCategoryLabel('(1) 市场化购电费\n（小计 88278.66）'), {
    main: '(1) 市场化购电费', sub: '（小计 88278.66）',
  })
  assert.deepEqual(splitCategoryLabel('(2) 上网环节线损费'), {
    main: '(2) 上网环节线损费', sub: '',
  })
  assert.deepEqual(splitCategoryLabel(''), { main: '', sub: '' })
})

test('第 41 轮：PDF 渲染器支持紧凑行高与分项表后强制分页', () => {
  const src = readFileSync(new URL('./useCatalogPdf.ts', import.meta.url), 'utf8')
  assert.match(src, /S_COMPACT_ROW_MIN = 18/, '分项表必须用紧凑行高')
  assert.match(src, /const compact = !!table\.rowSpans\?\.length/, '紧凑行高只对带 rowSpans 的表生效')
  assert.match(src, /if \(table\.pageBreakAfter && tableIdx < doc\.tables\.length - 1\)/,
    '分项表后必须强制分页（且末尾不产生空白页）')
  assert.match(src, /drawLine\(two\.sub, subSize/, '小计行必须用更小字号绘制')
})
