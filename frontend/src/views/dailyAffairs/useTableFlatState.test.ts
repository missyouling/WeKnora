/**
 * 第 37 轮：列表空/加载态横向滚动条判据。
 *
 * 规则：`loading === true` 或 行数 === 0 → flat（隐藏横向滚动条）。
 * 只有「加载完成 && 有数据」才允许出现横向滚动条。
 */
import test from 'node:test'
import assert from 'node:assert/strict'
import { ref } from 'vue'
import { useTableFlatState, flatClassOf } from './useTableFlatState'

test('loading 中 → flat（即使已有旧数据）', () => {
  const loading = ref(true)
  const rows = ref([1, 2, 3])
  const { isTableFlat } = useTableFlatState({ loading, rows })
  assert.equal(isTableFlat.value, true)
})

test('0 行 → flat', () => {
  const loading = ref(false)
  const rows = ref<number[]>([])
  const { isTableFlat } = useTableFlatState({ loading, rows })
  assert.equal(isTableFlat.value, true)
})

test('加载完成且有数据 → 不 flat（允许横向滚动）', () => {
  const loading = ref(false)
  const rows = ref([1, 2])
  const { isTableFlat } = useTableFlatState({ loading, rows })
  assert.equal(isTableFlat.value, false)
})

test('行数传数字也认（部分页面只给 count）', () => {
  assert.equal(useTableFlatState({ loading: false, rows: 0 }).isTableFlat.value, true)
  assert.equal(useTableFlatState({ loading: false, rows: 5 }).isTableFlat.value, false)
})

test('loading 传布尔（props 场景）也认', () => {
  assert.equal(useTableFlatState({ loading: true, rows: [1] }).isTableFlat.value, true)
  assert.equal(useTableFlatState({ loading: false, rows: [1] }).isTableFlat.value, false)
})

test('flatClassOf：flat 时挂 is-table-flat，否则只有 doc-list-view', () => {
  assert.equal(flatClassOf(true), 'doc-list-view is-table-flat')
  assert.equal(flatClassOf(false), 'doc-list-view')
})
