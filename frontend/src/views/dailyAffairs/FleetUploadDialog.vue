<template>
  <t-dialog :visible="visible" header="上传证照" width="640px" :footer="false" placement="center"
    class="fleet-upload-dialog" @close="onClose" @update:visible="(v: boolean) => (v || onClose())">
    <div class="fu-body">
      <!-- 证照类型选择 -->
      <div class="fu-type-row">
        <span class="fu-type-label">证照类型</span>
        <t-select v-model="selectedType" :options="typeOptions" placeholder="请选择证照类型"
          class="fu-type-select" style="width: 220px" />
        <span class="fu-type-tip">必须选择证照类型，上传后按所选类型归类提取</span>
      </div>

      <!-- 拖拽/点击选择区 -->
      <div class="fu-dropzone" :class="{ 'fu-dropzone--over': dragOver }" @click="pickFiles"
        @dragover.prevent="dragOver = true" @dragleave.prevent="dragOver = false" @drop.prevent="onDrop">
        <t-icon name="upload" size="28px" class="fu-drop-icon" />
        <div class="fu-drop-title">点击选择或拖拽文件到此处</div>
        <div class="fu-drop-sub">支持 PDF / JPG / PNG / WEBP / BMP，可多选</div>
        <input ref="fileInputRef" type="file" multiple accept=".pdf,.png,.jpg,.jpeg,.webp,.bmp" hidden
          @change="onInputChange" />
      </div>

      <!-- 文件列表 -->
      <div v-if="tasks.length" class="fu-list">
        <div v-for="t in tasks" :key="t.key" class="fu-item" :class="{ 'fu-item--failed': t.status === 'failed' }">
          <t-icon :name="taskIcon(t)" size="18px" class="fu-item-icon" :style="{ color: taskColor(t) }" />
          <div class="fu-item-main">
            <div class="fu-item-head">
              <span class="fu-item-name" :title="t.name">{{ t.name }}</span>
              <span class="fu-item-size">{{ fmtSize(t.size) }}</span>
            </div>
            <div class="fu-item-progress">
              <template v-if="t.status === 'success'">
                <span class="fu-item-status fu-item-status--ok">提取完成</span>
              </template>
              <template v-else-if="t.status === 'failed'">
                <span class="fu-item-status fu-item-status--err">{{ t.msg || '提取失败' }}</span>
              </template>
              <template v-else>
                <t-progress :percentage="t.progress" :label="false" size="small" :theme="t.status === 'failed' ? 'error' : undefined" />
                <span class="fu-item-label">{{ taskLabel(t) }}</span>
              </template>
            </div>
          </div>
          <t-button v-if="t.status === 'queued'" variant="text" size="small" shape="square" class="fu-item-del"
            @click.stop="removeTask(t.key)">
            <template #icon><t-icon name="close" size="16px" /></template>
          </t-button>
        </div>
      </div>

      <!-- 底部操作 -->
      <div class="fu-footer">
        <span class="fu-footer-tip">关闭弹窗后任务在后台继续执行</span>
        <t-button variant="outline" size="small" @click="onClose">关闭</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { uploadKnowledgeFile, getKnowledgeDetails, updateKnowledgeMetadata, listKnowledgeFiles } from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  typeOptions?: { label: string; value: string }[]
  defaultType?: string
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'done'): void
  (e: 'progress', items: UploadProgressItem[]): void
}>()

interface UpTask {
  key: string
  file: File
  name: string
  size: number
  status: 'queued' | 'uploading' | 'parsing' | 'extracting' | 'success' | 'failed'
  progress: number
  msg?: string
  kid?: string
}

/** 工具栏实时进度项：stage 对应任务阶段 */
export interface UploadProgressItem {
  name: string
  stage: 'uploading' | 'parsing' | 'extracting'
  percent: number
}

const selectedType = ref('')
const fileInputRef = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)
const tasks = ref<UpTask[]>([])
let taskSeq = 0

// 证照类型选项必须响应式：父组件 uploadTypeOptions 随分类加载/设置变更更新，
// 若用一次性求值会冻结为初始快照（分类尚未加载时只有内置类型，自定义类型缺失）
const typeOptions = computed(() => (props.typeOptions && props.typeOptions.length ? props.typeOptions : []))

// ---- 实时进度上报：任务状态/进度变化时把进行中的任务推给父组件工具栏 ----
function emitProgress() {
  const items: UploadProgressItem[] = tasks.value
    .filter((t) => t.status === 'uploading' || t.status === 'parsing' || t.status === 'extracting')
    .map((t) => ({ name: t.name, stage: t.status as UploadProgressItem['stage'], percent: Math.round(t.progress) }))
  emit('progress', items)
}

let autoCloseTimer: ReturnType<typeof setTimeout> | null = null

// 深度监听任务列表：进度实时上报；全部任务结束后延迟自动关闭弹窗（手动关闭后后台任务仍会继续，结束时同样自动关闭）
watch(tasks, (list) => {
  emitProgress()
  const allDone = list.length > 0 && list.every((t) => t.status === 'success' || t.status === 'failed')
  if (allDone) {
    if (!autoCloseTimer) {
      autoCloseTimer = setTimeout(() => {
        emit('update:visible', false)
        autoCloseTimer = null
      }, 1200)
    }
  } else if (autoCloseTimer) {
    clearTimeout(autoCloseTimer)
    autoCloseTimer = null
  }
}, { deep: true })

function onClose() { emit('update:visible', false) }
function pickFiles() { fileInputRef.value?.click() }
function onInputChange(e: Event) {
  const input = e.target as HTMLInputElement
  const files = input.files ? Array.from(input.files) : []
  input.value = ''
  addFiles(files)
}
function onDrop(e: DragEvent) {
  dragOver.value = false
  const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : []
  addFiles(files)
}

function addFiles(files: File[]) {
  if (!props.kbId) { MessagePlugin.warning('知识库尚未就绪'); return }
  // 必选证照类型：避免模型自动判定误分类
  if (!selectedType.value) { MessagePlugin.warning('请先选择证照类型'); return }
  const allowed = /\.(pdf|png|jpg|jpeg|webp|bmp)$/i
  for (const f of files) {
    if (!allowed.test(f.name)) { MessagePlugin.warning(`${f.name} 类型不支持，已跳过`); continue }
    const dup = tasks.value.some((t) => t.name === f.name && t.size === f.size)
    if (dup) continue
    taskSeq += 1
    tasks.value.push({
      key: `t${Date.now()}-${taskSeq}`,
      file: f,
      name: f.name,
      size: f.size,
      status: 'queued',
      progress: 0,
    })
  }
  startQueued()
}

function removeTask(key: string) {
  tasks.value = tasks.value.filter((t) => t.key !== key)
}

// ---- 状态展示 ----
function taskIcon(t: UpTask) {
  if (t.status === 'success') return 'check-circle-filled'
  if (t.status === 'failed') return 'error-circle-filled'
  return 'file'
}
function taskColor(t: UpTask) {
  if (t.status === 'success') return 'var(--td-success-color)'
  if (t.status === 'failed') return 'var(--td-error-color)'
  return 'var(--td-brand-color)'
}
function taskLabel(t: UpTask) {
  if (t.status === 'uploading') return `上传中 ${Math.round(t.progress)}%`
  if (t.status === 'parsing') return `解析中 ${Math.round(t.progress)}%`
  if (t.status === 'extracting') return `提取中 ${Math.round(t.progress)}%`
  return ''
}
const fmtSize = (v: number) => {
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`
  return `${(v / 1024 / 1024).toFixed(2)} MB`
}

// ---- 上传记忆（与列表页共享会话存储，防解析流程清空打标） ----
function savePending(kid: string, scope: string, certType: string) {
  try {
    const mem = JSON.parse(sessionStorage.getItem('weknora-fleet-pending-scopes') || '{}') || {}
    mem[kid] = certType ? { s: scope, t: certType } : scope
    sessionStorage.setItem('weknora-fleet-pending-scopes', JSON.stringify(mem))
  } catch { /* ignore */ }
}

// ---- 执行：上传 → 轮询解析 → 打标 → 提取（提取串行） ----
const extractQueue: (() => Promise<void>)[] = []
let extractWorkerRunning = false

function startQueued() {
  const queued = tasks.value.filter((t) => t.status === 'queued')
  queued.forEach((t) => { void runTask(t) })
}

async function runTask(t: UpTask) {
  const scope = props.scope
  const certType = selectedType.value
  try {
    t.status = 'uploading'
    t.progress = 0
    const res: any = await uploadKnowledgeFile(props.kbId, { file: t.file }, (ev: any) => {
      if (ev?.total) t.progress = Math.round((ev.loaded / ev.total) * 100)
    })
    const knowledge = res?.data || res
    const kid = knowledge?.id || knowledge?.knowledge_id
    if (!kid) throw new Error('上传响应缺少文件标识')
    t.kid = kid
    savePending(kid, scope, certType)

    // 轮询解析（无真实百分比接口，模拟递增）
    t.status = 'parsing'
    t.progress = 8
    const pollStart = Date.now()
    for (;;) {
      const kd: any = await getKnowledgeDetails(kid)
      const info = kd?.data || kd
      const ps = info?.parse_status || info?.status || ''
      if (ps === 'completed') { t.progress = 100; break }
      if (ps === 'failed') { t.status = 'failed'; t.msg = '解析失败'; return }
      const elapsed = Date.now() - pollStart
      t.progress = Math.min(92, 10 + elapsed / 1000 * 4)
      await sleep(2000)
      if (elapsed > 120000) { t.status = 'failed'; t.msg = '解析超时'; return }
    }

    // 解析完成后再打标（此时不会再被清空），然后排队提取
    try {
      const kd: any = await getKnowledgeDetails(kid)
      let meta2 = kd?.data?.custom_metadata || kd?.custom_metadata || {}
      if (meta2 && meta2.custom_metadata && typeof meta2.custom_metadata === 'object' && Object.keys(meta2).length === 1) meta2 = meta2.custom_metadata
      await updateKnowledgeMetadata(kid, certType ? { ...meta2, fleet_scope: scope, fleet_cert_type: certType } : { ...meta2, fleet_scope: scope })
    } catch { /* 打标失败由列表轮询兜底 */ }

    extractQueue.push(async () => {
      try {
        t.status = 'extracting'
        t.progress = 30
        await extractFile(kid, scope, certType)
        t.status = 'success'
        t.progress = 100
      } catch (err: any) {
        t.status = 'failed'
        t.msg = err?.message || '提取失败'
      }
    })
    drainExtractQueue()
    emit('done')
  } catch (err: any) {
    const msg = err?.message || '上传失败'
    if (msg.includes('already exists') || msg.includes('文件重复')) {
      t.status = 'failed'
      t.msg = '文件已存在，已忽略'
      // 已存在文件同样补记，供列表轮询提取
      try {
        const fl: any = await listKnowledgeFiles(props.kbId, { page: 1, page_size: 100 })
        const arr = Array.isArray(fl?.data) ? fl.data : Array.isArray(fl?.list) ? fl.list : []
        const hit = arr.find((k: any) => k.name === t.name || k.file_name === t.name)
        if (hit?.id) savePending(hit.id, scope, certType)
      } catch { /* ignore */ }
    } else {
      t.status = 'failed'
      t.msg = msg
    }
  }
}

function drainExtractQueue() {
  if (extractWorkerRunning) return
  const next = extractQueue.shift()
  if (!next) return
  extractWorkerRunning = true
  void next().finally(() => {
    extractWorkerRunning = false
    drainExtractQueue()
  })
}

async function extractFile(kid: string, scope: string, certType: string) {
  const token = localStorage.getItem('weknora_token')
  const res = await fetch(`/api/v1/knowledge-bases/${props.kbId}/knowledge/${kid}/extract-fleet-document`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
    body: JSON.stringify(certType ? { scope, cert_type: certType } : { scope }),
  })
  const j = await res.json()
  if (!res.ok) throw new Error(j?.message || '提取失败')
}

function sleep(ms: number) { return new Promise((r) => setTimeout(r, ms)) }

// 打开时：若未手动选择，默认继承父组件当前筛选的证照类型（防止漏选导致模型误判类型）
watch(() => props.visible, (v) => {
  if (v && !selectedType.value && props.defaultType) {
    selectedType.value = props.defaultType
  }
})

onBeforeUnmount(() => {
  // 组件常驻：关闭弹窗不销毁任务，任务在后台继续执行
  if (autoCloseTimer) clearTimeout(autoCloseTimer)
})
</script>

<style lang="less" scoped>
.fu-body {
  padding: 4px 2px 0;
}

.fu-type-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;

  .fu-type-label {
    font-size: 13px;
    color: var(--td-text-color-primary);
    white-space: nowrap;
  }

  .fu-type-tip {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}

.fu-dropzone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 120px;
  border: 1px dashed var(--td-component-border);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
  margin-bottom: 14px;

  &:hover,
  &--over {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }

  .fu-drop-icon {
    color: var(--td-brand-color);
  }

  .fu-drop-title {
    font-size: 13px;
    color: var(--td-text-color-primary);
  }

  .fu-drop-sub {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}

.fu-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 280px;
  overflow-y: auto;
  margin-bottom: 14px;
}

.fu-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  background: var(--td-bg-color-container);

  &--failed {
    border-color: var(--td-error-color-2);
  }

  .fu-item-icon {
    flex: none;
  }

  .fu-item-main {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .fu-item-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .fu-item-name {
    font-size: 13px;
    color: var(--td-text-color-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .fu-item-size {
    flex: none;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }

  .fu-item-progress {
    display: flex;
    align-items: center;
    gap: 8px;

    :deep(.t-progress) {
      flex: 1;
    }

    .fu-item-label {
      flex: none;
      width: 84px;
      font-size: 12px;
      color: var(--td-text-color-secondary);
    }

    .fu-item-status {
      font-size: 12px;

      &--ok { color: var(--td-success-color); }
      &--err { color: var(--td-error-color); }
    }
  }
}

.fu-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;

  .fu-footer-tip {
    flex: 1;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }
}
</style>
