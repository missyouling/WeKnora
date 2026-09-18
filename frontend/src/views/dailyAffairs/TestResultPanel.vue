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
      <!-- 结果提示 -->
      <t-alert v-if="error" theme="error" :message="error" close />
      <t-alert v-else-if="result" theme="success" message="提取完成，以下为字段级映射结果（可对照配置字段校验）" close />

      <template v-if="result">
        <!-- 字段映射表 -->
        <div class="table-card">
          <div class="table-head">
            <span class="table-title">字段映射（共 {{ rows.length }} 个启用字段，命中 {{ hitCount }} 个）</span>
            <span class="doc-type">证照类型：{{ result.doc_type || '-' }}</span>
          </div>
          <t-table :data="rows" :columns="columns" row-key="name" size="small" :bordered="false"
            :hover="true" table-layout="fixed" :pagination="null" :max-height="300" />
        </div>

        <!-- 原始 JSON + 实际 Prompt -->
        <div class="detail-block">
          <t-collapse v-model="activePanels" :borderless="true">
            <t-collapse-panel value="json" header="原始 JSON（模型返回）">
              <t-textarea :model-value="rawJson" readonly class="mono-area" :autosize="{ minRows: 4, maxRows: 10 }" />
            </t-collapse-panel>
            <t-collapse-panel value="prompt" header="实际 Prompt（发送给大模型的完整文本）">
              <t-textarea :model-value="promptPreview" readonly class="mono-area" :autosize="{ minRows: 8, maxRows: 16 }" />
            </t-collapse-panel>
          </t-collapse>
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import type { ExtractFieldConfig } from '@/api/fleet'

const props = defineProps<{
  testing: boolean
  error: string
  success: boolean
  result: any
  fields: ExtractFieldConfig[]
  promptPreview: string
}>()

const activePanels = ref<string[]>([])

type RowStatus = 'ok' | 'missing' | 'format'
interface FieldRow {
  name: string
  valueText: string
  status: RowStatus
}

const columns = computed(() => [
  { colKey: 'name', title: '字段名称', width: 140, ellipsis: true },
  { colKey: 'valueText', title: '提取结果', ellipsis: true },
  {
    colKey: 'status',
    title: '状态',
    width: 110,
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
  display: flex;
  flex-direction: column;
  gap: 12px;
  :deep(.t-loading) {
    border-radius: 6px;
  }
}
.loading-placeholder {
  height: 380px;
}
.table-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  .table-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
    .table-title {
      font-size: 13px;
      font-weight: 500;
    }
    .doc-type {
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }
  }
}
.detail-block {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  :deep(.t-collapse-panel__content) {
    padding: 4px 12px 12px;
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
