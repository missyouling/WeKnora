<template>
  <div class="test-result-panel">
    <!-- 初始空态 -->
    <t-empty v-if="!testing && !result && !error" description="尚未运行测试，配置好输入后点击「运行测试」，或将自动触发测试" />

    <!-- 测试中 -->
    <t-loading v-else-if="testing" :loading="true" show-overlay style="min-height: 380px">
      <div class="loading-placeholder" />
    </t-loading>

    <!-- 测试完成 -->
    <template v-else>
      <!-- 结果提示：几秒后自动关闭 -->
      <t-alert v-if="alertVisible && error" theme="error" :message="error" close @close="alertVisible = false" />
      <t-alert v-else-if="alertVisible && result" theme="success" message="提取完成" close @close="alertVisible = false" />

      <template v-if="result">
        <!-- 手风琴折叠：字段映射 / 原始 JSON / 实际 Prompt，窗口高度固定、面板内垂直滚动 -->
        <div class="accordion-block">
          <div class="acc-panel" :class="{ 'acc-open': activePanels[0] === 'map' }">
            <div class="acc-head" @click="togglePanel('map')">
              <t-icon :name="activePanels[0] === 'map' ? 'chevron-down' : 'chevron-right'" class="acc-icon" />
              <span class="acc-title">字段映射（命中 {{ hitCount }}/{{ rows.length }}）</span>
              <span class="acc-doc-type">{{ result.doc_type || '-' }}</span>
            </div>
            <div v-show="activePanels[0] === 'map'" class="acc-body">
              <t-table :data="rows" :columns="columns" row-key="name" size="small" :bordered="false"
                :hover="true" table-layout="fixed" :pagination="null" />
            </div>
          </div>
          <div class="acc-panel" :class="{ 'acc-open': activePanels[0] === 'json' }">
            <div class="acc-head" @click="togglePanel('json')">
              <t-icon :name="activePanels[0] === 'json' ? 'chevron-down' : 'chevron-right'" class="acc-icon" />
              <span class="acc-title">原始 JSON（模型返回）</span>
            </div>
            <div v-show="activePanels[0] === 'json'" class="acc-body">
              <t-textarea :model-value="rawJson" readonly class="mono-area" :autosize="{ minRows: 4, maxRows: 12 }" />
            </div>
          </div>
          <div class="acc-panel" :class="{ 'acc-open': activePanels[0] === 'prompt' }">
            <div class="acc-head" @click="togglePanel('prompt')">
              <t-icon :name="activePanels[0] === 'prompt' ? 'chevron-down' : 'chevron-right'" class="acc-icon" />
              <span class="acc-title">实际 Prompt（发送给大模型的完整文本）</span>
            </div>
            <div v-show="activePanels[0] === 'prompt'" class="acc-body">
              <t-textarea :model-value="promptPreview" readonly class="mono-area" :autosize="{ minRows: 8, maxRows: 18 }" />
            </div>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import type { ExtractFieldConfig } from '@/api/fleet'

const props = defineProps<{
  testing: boolean
  error: string
  success: boolean
  result: any
  fields: ExtractFieldConfig[]
  promptPreview: string
}>()

// 手风琴：默认展开字段映射，同一时间只展开一个面板（自实现，避免 TDesign collapse 受控模式不稳定）
const activePanels = ref<string[]>(['map'])
function togglePanel(v: string) {
  activePanels.value = activePanels.value[0] === v ? [] : [v]
}

// 结果提示自动关闭：测试完成数秒后隐藏
const alertVisible = ref(false)
let alertTimer: ReturnType<typeof setTimeout> | null = null
watch(
  () => [props.success, props.error],
  () => {
    if (props.success || props.error) {
      alertVisible.value = true
      if (alertTimer) clearTimeout(alertTimer)
      alertTimer = setTimeout(() => {
        alertVisible.value = false
      }, 4000)
    }
  },
)
onBeforeUnmount(() => {
  if (alertTimer) clearTimeout(alertTimer)
})

type RowStatus = 'ok' | 'missing' | 'format'
interface FieldRow {
  name: string
  valueText: string
  status: RowStatus
}

const columns = computed(() => [
  { colKey: 'name', title: '字段名称', width: 130, ellipsis: true },
  { colKey: 'valueText', title: '提取结果', ellipsis: true },
  {
    colKey: 'status',
    title: '状态',
    width: 96,
    align: 'center' as const,
    cell: (h: any, { row }: any) =>
      h('span', {}, [
        h('t-tag', { props: { size: 'small', theme: statusTheme(row.status), variant: 'light-outline' } }, statusLabel(row.status)),
      ]),
  },
])

const rows = computed<FieldRow[]>(() => {
  const r = props.result
  if (!r) return []
  const map: Record<string, any> = r.fields || {}
  const missing = new Set<string>(Array.isArray(r.missing) ? r.missing : [])
  return (props.fields || [])
    .filter((f) => f.enabled && f.name)
    .map((f) => {
      const v = map[f.name]
      const isEmpty = v === null || v === undefined || v === '' || (Array.isArray(v) && v.length === 0)
      const valueText = Array.isArray(v) ? v.join('、') : isEmpty ? '' : String(v)
      let status: RowStatus = isEmpty || missing.has(f.name) ? 'missing' : 'ok'
      if (status === 'ok' && isFormatAbnormal(f, v)) status = 'format'
      return { name: f.name, valueText, status }
    })
})

const hitCount = computed(() => rows.value.filter((r) => r.status === 'ok').length)

const rawJson = computed(() => {
  try {
    return JSON.stringify(props.result?.fields ?? {}, null, 2)
  } catch {
    return props.result?.fields ? String(props.result.fields) : ''
  }
})

function statusTheme(s: RowStatus): string {
  if (s === 'ok') return 'success'
  if (s === 'format') return 'warning'
  return 'danger'
}
function statusLabel(s: RowStatus): string {
  if (s === 'ok') return '已提取'
  if (s === 'format') return '格式异常'
  return '未提取到'
}

// 格式异常判定：按字段配置的数据类型检查已提取值
function isFormatAbnormal(f: ExtractFieldConfig, v: any): boolean {
  const t = (f.type || 'string').toLowerCase()
  if (t === 'number' || t === 'int' || t === 'float') return typeof v === 'string' && v.trim() !== '' && Number.isNaN(Number(v))
  if (t === 'date') return typeof v === 'string' && v.trim() !== '' && !/^\d{4}-\d{2}-\d{2}/.test(v.trim())
  if (t === 'array') return v !== null && v !== undefined && !Array.isArray(v)
  return false
}
</script>

<style lang="less" scoped>
.test-result-panel {
  min-height: 380px;
  /* 窗口高度固定：面板内垂直滚动，不改变弹窗整体高度，避免底部横向滚动条 */
  max-height: 460px;
  overflow-y: auto;
  overflow-x: hidden;
  padding-right: 4px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  :deep(.t-loading) {
    border-radius: 6px;
  }
}
.loading-placeholder {
  height: 380px;
}
.accordion-block {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  .acc-panel {
    border-bottom: 1px solid var(--td-component-stroke);
    &:last-child {
      border-bottom: none;
    }
    .acc-head {
      display: flex;
      align-items: center;
      gap: 6px;
      padding: 8px 12px;
      cursor: pointer;
      user-select: none;
      background: var(--td-bg-color-container);
      &:hover {
        background: var(--td-bg-color-container-hover);
      }
      .acc-icon {
        font-size: 14px;
        color: var(--td-text-color-secondary);
        transition: transform 0.2s;
      }
      .acc-title {
        font-size: 13px;
        font-weight: 500;
        color: var(--td-text-color-primary);
        white-space: nowrap;
      }
      .acc-doc-type {
        margin-left: auto;
        font-size: 12px;
        color: var(--td-text-color-secondary);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
    .acc-body {
      padding: 4px 12px 10px;
      /* 固定列宽：字段名称/状态不随内容挤压换行 */
      :deep(.t-table__th-cell-content),
      :deep(.t-table__td-cell) {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }
    }
  }
}
.mono-area {
  :deep(textarea) {
    font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
    font-size: 12px;
    line-height: 1.6;
    background: #f9f9f9;
  }
}
</style>
