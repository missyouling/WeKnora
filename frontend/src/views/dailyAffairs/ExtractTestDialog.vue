<template>
  <t-dialog :visible="visible" header="测试规则" :width="'72%'" :close-on-overlay-click="false" :footer="false"
    :attach="'body'" @update:visible="(v: boolean) => emit('update:visible', v)" @close="onClose">
    <div class="extract-test-layout">
      <!-- 左 5/12：输入与原文预览（限高内滚）；右 7/12：提取结果 -->
      <t-row :gutter="24">
        <t-col :span="5" class="col-left">
          <TestInputPanel v-model:source-mode="sourceMode" v-model:text="text" v-model:knowledge-id="knowledgeId"
            :files="files" :loading-files="loadingFiles" :original-text="originalText" :original-status="originalStatus"
            :disabled="testing" @reload-files="loadFiles" />
        </t-col>
        <t-col :span="7" class="col-right">
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

type Mode = 'text' | 'file'
const sourceMode = ref<Mode>('text')
const text = ref('')
const knowledgeId = ref('')
const files = ref<any[]>([])
const loadingFiles = ref(false)
const testing = ref(false)
const saving = ref(false)

// 提取结果按测试方式隔离：文件提取测试 / 文本提取测试互不串扰
const resultMap = ref<Record<Mode, any>>({ text: null, file: null })
const errorMap = ref<Record<Mode, string>>({ text: '', file: '' })
const successMap = ref<Record<Mode, boolean>>({ text: false, file: false })

// VLM 识别原文：文本模式展示粘贴文本；文件模式展示所选文件 description
const originalText = ref('')
const originalStatus = ref<'none' | 'loading' | 'done' | 'empty' | 'preview'>('none')

const fileOptions = computed(() =>
  files.value.map((f) => ({ label: f.file_name || f.title || f.id, value: f.id })),
)
const canRun = computed(() =>
  props.visible && (sourceMode.value === 'text' ? text.value.trim().length > 0 : !!knowledgeId.value),
)
const result = computed(() => resultMap.value[sourceMode.value])
const error = computed(() => errorMap.value[sourceMode.value])
const success = computed(() => successMap.value[sourceMode.value])

// 文本提取测试：输入变化 500ms 防抖自动触发；切换标签不触发
let debounceTimer: ReturnType<typeof setTimeout> | null = null
watch(text, () => {
  originalText.value = text.value
  originalStatus.value = 'preview'
  if (debounceTimer) clearTimeout(debounceTimer)
  if (!props.visible || sourceMode.value !== 'text') return
  debounceTimer = setTimeout(() => {
    if (canRun.value) runTest()
  }, 500)
})

// 文件提取测试：拉取 VLM 原文（不自动测试，点击「运行测试」手动触发）
async function loadOriginal(kid: string) {
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
}

watch(knowledgeId, async (kid) => {
  if (!kid || sourceMode.value !== 'file') return
  await loadOriginal(kid)
})

// 切换标签：仅同步原文预览，保持弹窗尺寸一致，不清空各自测试结果
watch(sourceMode, () => {
  if (sourceMode.value === 'text') {
    originalText.value = text.value
    originalStatus.value = 'preview'
  } else if (knowledgeId.value) {
    void loadOriginal(knowledgeId.value)
  } else {
    originalText.value = ''
    originalStatus.value = 'none'
  }
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

async function runTest() {
  if (!canRun.value || testing.value) return
  testing.value = true
  errorMap.value[sourceMode.value] = ''
  resultMap.value[sourceMode.value] = null
  successMap.value[sourceMode.value] = false
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
    const data = res?.data
    if (data) {
      resultMap.value[sourceMode.value] = data
      successMap.value[sourceMode.value] = true
    } else {
      errorMap.value[sourceMode.value] = '未返回提取结果'
    }
  } catch (e: any) {
    console.error('test extract failed', e)
    errorMap.value[sourceMode.value] = e?.message || '测试失败，请检查规则配置或模型服务'
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
      resultMap.value = { text: null, file: null }
      errorMap.value = { text: '', file: '' }
      successMap.value = { text: false, file: false }
      originalText.value = text.value
      originalStatus.value = 'preview'
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
