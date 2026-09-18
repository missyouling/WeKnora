<template>
  <t-dialog :visible="visible" header="测试规则" :width="'80%'" :close-on-overlay-click="false" :footer="false"
    :attach="'body'" @update:visible="(v: boolean) => emit('update:visible', v)" @close="onClose">
    <div class="extract-test-layout">
      <!-- 左 5/12：输入与原文预览（限高内滚）；右 7/12：提取结果 -->
      <t-row :gutter="24">
        <t-col :span="10" class="col-left">
          <TestInputPanel v-model:source-mode="sourceMode" v-model:text="text" v-model:knowledge-id="knowledgeId"
            :files="files" :loading-files="loadingFiles" :original-text="originalText" :original-status="originalStatus"
            :disabled="testing" @reload-files="loadFiles" />
        </t-col>
        <t-col :span="14" class="col-right">
          <TestResultPanel :testing="testing" :error="error" :success="success" :result="result" :fields="props.fields"
            :prompt-preview="previewPrompt" />
        </t-col>
      </t-row>
    </div>

    <div class="test-footer">
      <t-button theme="primary" :loading="testing" :disabled="!canRun || testing" @click="runTest">
        <template #icon><t-icon name="play-circle" /></template>运行测试
      </t-button>
      <t-button v-if="success && !testing" theme="success" variant="outline" :loading="saving" @click="saveRule">
        <template #icon><t-icon name="check-circle" /></template>确认规则并保存
      </t-button>
      <t-button variant="outline" :disabled="testing || saving" @click="onClose">关闭</t-button>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { testExtractConfig, saveExtractConfig, type ExtractFieldConfig } from '@/api/fleet'
import { listKnowledgeFiles, getKnowledgeDetails } from '@/api/knowledge-base'
import TestInputPanel from './TestInputPanel.vue'
import TestResultPanel from './TestResultPanel.vue'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  certType: string
  fields: ExtractFieldConfig[]
  advancedEnabled: boolean
  promptTemplate: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'saved'): void
}>()

const sourceMode = ref<'text' | 'file'>('text')
const text = ref('')
const knowledgeId = ref('')
const files = ref<any[]>([])
const loadingFiles = ref(false)
const testing = ref(false)
const saving = ref(false)
const result = ref<any>(null)
const error = ref('')
const success = ref(false)

// VLM 识别原文：所选文件 description（VLM 摘要），选文件后拉取
const originalText = ref('')
const originalStatus = ref<'none' | 'loading' | 'done' | 'empty'>('none')

const fileOptions = computed(() =>
  files.value.map((f) => ({ label: f.file_name || f.title || f.id, value: f.id })),
)
const canRun = computed(() =>
  props.visible && (sourceMode.value === 'text' ? text.value.trim().length > 0 : !!knowledgeId.value),
)

// 防抖自动测试（切换文件 / 修改文本 500ms 后触发）
let debounceTimer: ReturnType<typeof setTimeout> | null = null
watch([text, knowledgeId, sourceMode], () => {
  originalStatus.value = 'none'
  originalText.value = ''
  if (debounceTimer) clearTimeout(debounceTimer)
  if (!props.visible) return
  debounceTimer = setTimeout(() => {
    if (canRun.value) runTest()
  }, 500)
})

// 实际发送给大模型的 Prompt 预览（与后端 BuildExtractSystemPrompt 同构）
const previewPrompt = computed(() => {
  const lines: string[] = []
  if (props.advancedEnabled && props.promptTemplate.trim()) {
    lines.push('（高级模式模板）')
    lines.push(props.promptTemplate)
    return lines.join('\n')
  }
  const kind = `fleet_${props.scope === 'driver' ? 'driver' : props.scope === 'maintain' ? 'maintain' : 'vehicle'}_document`
  lines.push(`你是一个档案信息提取助手。本文件证照类型已确定为「${props.certType}」。`)
  lines.push('输出严格 JSON（不要输出任何其他文字）：')
  lines.push(`{ "kind": "${kind}", "doc_type": "${props.certType}", "vehicle_no": "车牌号（如没有则空字符串）", "fields": {`)
  const enabled = (props.fields || []).filter((f) => f.enabled && f.name)
  enabled.forEach((f, i) => {
    lines.push(`  "${f.name}": "…"${i < enabled.length - 1 ? ',' : ''}`)
  })
  lines.push('} }')
  if (enabled.length) {
    lines.push('字段说明：')
    enabled.forEach((f) => {
      let s = `- 字段：「${f.name}」`
      if (f.desc) s += `，描述：${f.desc}`
      if (f.rule) s += `，规则：${f.rule}`
      lines.push(s)
    })
  }
  lines.push(`注意：doc_type 必须严格等于「${props.certType}」，严禁修改为其它类型。`)
  lines.push('fields 必须直接输出上面列出的字段名作为键的键值对，严禁再嵌套 properties/required/type 等 schema 包装结构。')
  lines.push('只提取上面 fields 中列出的字段：文档中真实出现的填值，未出现的内容填空字符串（数组填 []、数字填 0），不要输出未列出的额外字段。')
  lines.push('必须严格执行每个字段的「规则」：文档中出现规则中列举的同类表述时，统一归入该字段。')
  return lines.join('\n')
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

// 选择知识库文件后拉取 VLM 识别原文（description）
watch(knowledgeId, async (kid) => {
  if (!kid || sourceMode.value !== 'file') return
  originalStatus.value = 'loading'
  originalText.value = ''
  try {
    const res: any = await getKnowledgeDetails(kid)
    const desc: string = res?.data?.description || ''
    originalText.value = desc
    originalStatus.value = desc.trim() ? 'done' : 'empty'
  } catch (e) {
    console.error('load document original text failed', e)
    originalStatus.value = 'empty'
  }
})

async function runTest() {
  if (!canRun.value || testing.value) return
  testing.value = true
  error.value = ''
  result.value = null
  success.value = false
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
    if (result.value) {
      success.value = true
    } else {
      error.value = '未返回提取结果'
    }
  } catch (e: any) {
    console.error('test extract failed', e)
    error.value = e?.message || '测试失败，请检查规则配置或模型服务'
  } finally {
    testing.value = false
  }
}

async function saveRule() {
  if (saving.value) return
  saving.value = true
  try {
    const payload: Record<string, unknown> = {
      scope: props.scope,
      cert_type: props.certType,
      fields: (props.fields || []).map((f) => ({ ...f })),
      advanced_enabled: props.advancedEnabled,
      prompt_template: props.promptTemplate,
    }
    await saveExtractConfig(props.kbId, payload)
    MessagePlugin.success('提取规则已保存')
    emit('saved')
    onClose()
  } catch (e: any) {
    console.error('save extract config failed', e)
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function onClose() {
  if (testing.value || saving.value) return
  if (debounceTimer) clearTimeout(debounceTimer)
  emit('update:visible', false)
}

watch(
  () => props.visible,
  (v) => {
    if (v) {
      error.value = ''
      result.value = null
      success.value = false
      originalText.value = ''
      originalStatus.value = 'none'
      loadFiles()
    }
  },
)
</script>

<style lang="less" scoped>
.extract-test-layout {
  min-height: 400px;
  /* 左列限高，内部滚动；右列与左列对齐等高 */
  :deep(.col-left),
  :deep(.col-right) {
    height: 100%;
  }
}
.test-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 14px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 14px;
}
</style>
