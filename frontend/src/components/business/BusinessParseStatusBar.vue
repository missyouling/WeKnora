<template>
  <!-- 解析/提取状态横幅（证照档案同款）：优先展示上传任务的实时进度（多文件轮播），
       无活动上传任务时回退到列表轮询兜底（正在解析 N 个文件）。 -->
  <div v-if="liveCount || pendingFiles.length" class="parse-status-bar">
    <t-loading size="small" />
    <span class="parse-status-text">{{ statusText }}</span>
    <span v-if="multiCount > 1" class="parse-status-count">{{ progressIndex + 1 }}/{{ multiCount }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'

export interface ParseProgressItem {
  name: string
  stage: 'uploading' | 'parsing' | 'extracting'
  /** null = 该阶段无真实进度数据（如提取阶段后端只返回状态枚举），不展示百分比 */
  percent: number | null
  /** 耗时秒数：保留字段以兼容父组件入参，但**不展示** —— 耗时与百分比混排
   *  会让用户以为卡死（见 statusText 注释）。 */
  elapsedSec?: number
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

// 兜底计数：仅统计真正进行中的文件（pending/processing/finalizing/提取中），
// 解析失败的文件由历史记录抽屉承接人工处理，不在横幅计数内。
// 用 computed 而不是 watch+ref：watch 只在数组引用变化时触发，后端返回同一批
// 文件但状态已终态时不会重算，横幅就「挂在页面上不消失」。
const liveCount = computed(() => props.pendingFiles.filter(f => {
  const ps = f.parse_status || ''
  const es = f.custom_metadata?.extract_status || ''
  if (ps === 'pending' || ps === 'processing' || ps === 'finalizing') return true
  if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) return true
  return false
}).length)

// 轮播份数：上传进度条数可能多于待解析文件数（上传完成才开始解析）
const multiCount = computed(() => Math.max(props.progress.length, liveCount.value))

// 统一文案：**绝不显示耗时读数**。
// 旧实现没有真实百分比时改显耗时，于是同一份文件一会儿「进度 100%」、
// 一会儿「耗时 11 分 3 秒」，用户以为卡死。现在只保留两种口径：
//   上传阶段 → 真实百分比（后端也确实给的是百分比）；
//   解析/提取 → 文件名 + 剩余份数（不猜进度、不报耗时）。
const statusText = computed(() => {
  const p = props.progress[progressIndex.value]
  if (!p) {
    const n = props.pendingFiles.length || liveCount.value
    return `正在解析/提取 ${n} 个文件，完成后自动提取字段...`
  }
  const head = `${stageLabel(p.stage)}「${p.name}」`
  if (p.stage === 'uploading' && p.percent !== null) return `${head} ${p.percent}%`
  const rest = multiCount.value - 1
  return rest > 0 ? `${head}（剩余 ${rest} 份）...` : `${head}...`
})

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

  .parse-status-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .parse-status-count {
    margin-left: auto;
    font-size: var(--td-font-size-body-small);
    opacity: 0.75;
  }
}
</style>
