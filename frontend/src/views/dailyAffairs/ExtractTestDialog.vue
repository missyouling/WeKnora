<template>
  <t-dialog :visible="visible" header="测试规则" :width="680" :close-on-overlay-click="true"
    @update:visible="(v: boolean) => emit('update:visible', v)" @close="onClose">
    <div class="extract-test-body">
      <div class="test-source">
        <t-radio-group v-model="sourceMode" variant="default-filled">
          <t-radio-button value="text">粘贴文本</t-radio-button>
          <t-radio-button value="file">选择知识库文件</t-radio-button>
        </t-radio-group>
      </div>

      <t-textarea v-if="sourceMode === 'text'" v-model="text" class="test-text" :autosize="{ minRows: 6, maxRows: 12 }" placeholder="粘贴要测试的文档内容" />
      <div v-else class="file-select">
        <t-select v-model="knowledgeId" :options="fileOptions" placeholder="选择已解析完成的文件" filterable />
        <t-loading :loading="loadingFiles" size="small" />
      </div>

      <t-loading :loading="testing" show-overlay>
        <div v-if="result" class="test-result">
          <div class="result-head">
            <span class="result-title">提取结果</span>
            <span class="doc-type">证照类型：{{ result.doc_type || '-' }}</span>
          </div>
          <div class="result-list">
            <div v-for="f in displayFields" :key="f.name" class="result-row" :class="f.missing ? 'is-missing' : 'is-ok'">
              <span class="rf-name">{{ f.name }}</span>
              <span class="rf-value">{{ f.valueText }}</span>
              <span class="rf-tag">{{ f.missing ? '缺失' : '已提取' }}</span>
            </div>
          </div>
        </div>
        <t-empty v-else-if="!testing && error" :description="error" />
      </t-loading>
    </div>

    <template #footer>
      <div class="test-footer">
        <t-button theme="primary" :loading="testing" :disabled="!canRun" @click="runTest">运行测试</t-button>
        <t-button variant="outline" @click="onClose">关闭</t-button>
      </div>
    </template>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { testExtractConfig, type ExtractFieldConfig } from '@/api/fleet'
import { listKnowledgeFiles } from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  certType: string
  fields: ExtractFieldConfig[]
  advancedEnabled: boolean
  promptTemplate: string
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const sourceMode = ref<'text' | 'file'>('text')
const text = ref('')
const knowledgeId = ref('')
const files = ref<any[]>([])
const loadingFiles = ref(false)
const testing = ref(false)
const result = ref<any>(null)
const error = ref('')

const fileOptions = computed(() =>
  files.value.map((f) => ({ label: f.file_name || f.title || f.id, value: f.id })),
)
const canRun = computed(() =>
  props.visible && (sourceMode.value === 'text' ? text.value.trim().length > 0 : !!knowledgeId.value),
)

const displayFields = computed(() => {
  const r = result.value
  if (!r) return []
  const map: Record<string, any> = r.fields || {}
  const missing = new Set<string>(Array.isArray(r.missing) ? r.missing : [])
  return (props.fields || [])
    .filter((f) => f.enabled && f.name)
    .map((f) => {
      const v = map[f.name]
      let valueText = ''
      if (Array.isArray(v)) valueText = v.join('、')
      else if (v !== null && v !== undefined) valueText = String(v)
      return { name: f.name, valueText, missing: missing.has(f.name) }
    })
})

async function loadFiles() {
  if (!props.kbId) return
  loadingFiles.value = true
  try {
    const res: any = await listKnowledgeFiles(props.kbId, { page: 1, page_size: 100, parse_status: 'completed' })
    files.value = res?.data?.list || res?.data || []
  } catch (e) {
    console.error('load files failed', e)
    files.value = []
  } finally {
    loadingFiles.value = false
  }
}

async function runTest() {
  if (!canRun.value) return
  testing.value = true
  error.value = ''
  result.value = null
  try {
    const payload: Record<string, unknown> = {
      scope: props.scope,
      cert_type: props.certType,
      fields: (props.fields || []).map((f) => ({ ...f })),
      advanced_enabled: props.advancedEnabled,
      prompt_template: props.promptTemplate,
    }
    if (sourceMode.value === 'text') payload.text = text.value
    else payload.knowledge_id = knowledgeId.value
    const res: any = await testExtractConfig(props.kbId, payload)
    result.value = res?.data
    if (!result.value) error.value = '未返回提取结果'
  } catch (e: any) {
    console.error('test extract failed', e)
    error.value = e?.message || '测试失败'
  } finally {
    testing.value = false
  }
}

function onClose() {
  emit('update:visible', false)
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      error.value = ''
      result.value = null
      loadFiles()
    }
  },
)
watch(sourceMode, (m) => {
  if (m === 'file') loadFiles()
})
</script>

<style lang="less" scoped>
.extract-test-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-height: 240px;
}
.test-text {
  width: 100%;
}
.file-select {
  display: flex;
  align-items: center;
  gap: 10px;
  :deep(.t-select) {
    flex: 1;
  }
}
.test-result {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  .result-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
    .result-title {
      font-size: 13px;
      font-weight: 500;
    }
    .doc-type {
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }
  }
  .result-list {
    max-height: 300px;
    overflow: auto;
  }
  .result-row {
    display: grid;
    grid-template-columns: 130px 1fr 56px;
    gap: 8px;
    align-items: center;
    padding: 6px 12px;
    border-bottom: 1px solid var(--td-component-stroke);
    &:last-child {
      border-bottom: none;
    }
    .rf-name {
      font-size: 12px;
      color: var(--td-text-color-secondary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .rf-value {
      font-size: 13px;
      word-break: break-all;
    }
    .rf-tag {
      font-size: 12px;
      text-align: center;
      border-radius: 4px;
      padding: 1px 0;
    }
    &.is-ok .rf-tag {
      color: var(--td-success-color);
      background: var(--td-success-color-1);
    }
    &.is-missing .rf-tag {
      color: var(--td-error-color);
      background: var(--td-error-color-1);
    }
    &.is-missing .rf-value {
      color: var(--td-text-color-placeholder);
    }
  }
}
.test-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
}
</style>
