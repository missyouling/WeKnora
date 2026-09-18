<template>
  <div class="test-input-panel">
    <!-- 输入方式 -->
    <t-tabs v-model="innerMode" class="input-tabs">
      <t-tab-panel value="text" label="粘贴文本">
        <t-textarea v-model="innerText" class="input-area" :autosize="{ minRows: 6, maxRows: 10 }"
          :disabled="disabled" placeholder="粘贴要测试的文档内容（VLM 识别出的原文或手工文本）" />
      </t-tab-panel>
      <t-tab-panel value="file" label="选择知识库文件">
        <div class="file-row">
          <t-select v-model="innerKnowledgeId" :options="fileOptions" placeholder="选择已解析完成的文件" filterable
            :disabled="disabled" class="file-select" :loading="loadingFiles" />
          <t-button variant="outline" size="small" :disabled="disabled || loadingFiles" @click="$emit('reload-files')">
            <template #icon><t-icon name="refresh" /></template>
          </t-button>
        </div>
        <p class="file-hint">选中的文件将调用 VLM 识别原文并参与提取测试</p>
      </t-tab-panel>
    </t-tabs>

    <!-- VLM 识别原文 -->
    <div class="original-block">
      <div class="original-head">
        <span class="original-title">VLM 识别原文</span>
        <t-tag v-if="originalStatus === 'loading'" size="small" theme="warning" variant="light-outline">
          <template #icon><t-icon name="loading" /></template>识别中
        </t-tag>
        <t-tag v-else-if="originalStatus === 'done'" size="small" theme="success" variant="light-outline">识别完成</t-tag>
        <t-tag v-else-if="originalStatus === 'empty'" size="small" theme="default" variant="light-outline">无可用原文</t-tag>
        <t-tag v-else size="small" theme="default" variant="light-outline">待选择</t-tag>
      </div>
      <t-textarea :model-value="originalText" class="original-area" readonly :autosize="{ minRows: 4, maxRows: 8 }"
        placeholder="选择知识库文件后，此处展示 VLM 识别出的原始文本内容" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  sourceMode: 'text' | 'file'
  text: string
  knowledgeId: string
  files: any[]
  loadingFiles: boolean
  originalText: string
  originalStatus: 'none' | 'loading' | 'done' | 'empty'
  disabled: boolean
}>()
const emit = defineEmits<{
  (e: 'update:sourceMode', v: 'text' | 'file'): void
  (e: 'update:text', v: string): void
  (e: 'update:knowledgeId', v: string): void
  (e: 'reload-files'): void
}>()

const innerMode = computed({
  get: () => props.sourceMode,
  set: (v: 'text' | 'file') => emit('update:sourceMode', v),
})
const innerText = computed({
  get: () => props.text,
  set: (v: string) => emit('update:text', v),
})
const innerKnowledgeId = computed({
  get: () => props.knowledgeId,
  set: (v: string) => emit('update:knowledgeId', v),
})
const fileOptions = computed(() =>
  props.files.map((f) => ({ label: f.file_name || f.title || f.id, value: f.id })),
)
</script>

<style lang="less" scoped>
.test-input-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  /* 限制最大高度，内部滚动，避免挤压右侧表格 */
  max-height: 540px;
  overflow-y: auto;
  padding-right: 4px;
}
.input-tabs {
  :deep(.t-tabs__nav) {
    margin-bottom: 10px;
  }
}
.input-area {
  width: 100%;
}
.file-row {
  display: flex;
  align-items: center;
  gap: 8px;
  .file-select {
    flex: 1;
  }
}
.file-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.original-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: none;
  .original-head {
    display: flex;
    align-items: center;
    gap: 8px;
    .original-title {
      font-size: 13px;
      font-weight: 500;
      color: var(--td-text-color-primary);
    }
  }
  .original-area {
    width: 100%;
    :deep(textarea) {
      font-family: 'JetBrains Mono', Consolas, Menlo, monospace;
      font-size: 12px;
      line-height: 1.6;
      background: #f9f9f9;
    }
  }
}
</style>
