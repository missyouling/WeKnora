<template>
  <!-- 解析/提取状态横幅（证照档案同款）：优先展示上传任务的实时进度（多文件轮播），
       无活动上传任务时回退到列表轮询兜底（正在解析 N 个文件）。 -->
  <div v-if="liveCount || pendingFiles.length" class="parse-status-bar">
    <t-loading size="small" />
    <template v-if="progress.length">
      <span>{{ progressText(progress[progressIndex]) }}</span>
      <span v-if="progress.length > 1" class="parse-status-count">{{ progressIndex + 1 }}/{{ progress.length }}</span>
    </template>
    <span v-else>正在解析 {{ pendingFiles.length }} 个文件，解析完成后自动提取字段...</span>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'

export interface ParseProgressItem {
  name: string
  stage: 'uploading' | 'parsing' | 'extracting'
  /** null = 该阶段无真实进度数据（如提取阶段后端只返回状态枚举），只展示"进行中" */
  percent: number | null
}
export interface ParseStatusFile {
  id: string
  file_name?: string
  title?: string
  parse_status?: string
  [k: string]: any
}

const props = defineProps<{
  progress: ParseProgressItem[]
  pendingFiles: ParseStatusFile[]
}>()

const STAGE_LABEL: Record<string, string> = { uploading: '正在上传', parsing: '正在解析', extracting: '正在提取' }
function stageLabel(stage: string) { return STAGE_LABEL[stage] || stage }
// 有真实百分比才显示数字；提取阶段后端没有进度数据，显示"进行中"而不是假百分比。
function progressText(p: ParseProgressItem) {
  const head = `${stageLabel(p.stage)}「${p.name}」`
  return p.percent === null ? `${head}进行中…` : `${head}进度 ${p.percent}%`
}

// 兜底计数：仅统计真正进行中的文件（pending/processing/finalizing/提取中），
// 解析失败的文件由历史记录抽屉承接人工处理，不在横幅计数内
const liveCount = ref(0)
watch(() => props.pendingFiles, (v) => {
  liveCount.value = v.filter(f => {
    const ps = f.parse_status || ''
    const es = f.custom_metadata?.extract_status || ''
    if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return true
    if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) return true
    return false
  }).length
}, { immediate: true })

// 多文件同时上传/解析时 3s 轮播切换展示
const progressIndex = ref(0)
let progressTimer: ReturnType<typeof setInterval> | null = null
watch(() => props.progress, (v) => {
  if (v.length <= 1) {
    progressIndex.value = 0
    if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
    return
  }
  if (progressTimer) return
  progressTimer = setInterval(() => {
    if (props.progress.length <= 1) {
      if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
      return
    }
    progressIndex.value = (progressIndex.value + 1) % props.progress.length
  }, 3000)
}, { immediate: true })

function stopProgressTimer() {
  if (progressTimer) { clearInterval(progressTimer); progressTimer = null }
}
onBeforeUnmount(stopProgressTimer)
</script>

<style lang="less" scoped>
.parse-status-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  margin: 0 0 10px;
  background: var(--td-brand-color-light);
  color: var(--td-brand-color);
  border-radius: var(--td-radius-large);
  font-size: var(--td-font-size-body-small);

  .parse-status-count {
    margin-left: auto;
    font-size: var(--td-font-size-body-small);
    opacity: 0.75;
  }
}
</style>
