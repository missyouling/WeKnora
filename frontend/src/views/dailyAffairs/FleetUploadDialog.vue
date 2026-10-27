<template>
  <t-dialog :visible="visible" :header="dialogTitle" width="640px" :footer="false" placement="center"
    class="fleet-upload-dialog" @close="onClose" @update:visible="(v: boolean) => (v || onClose())">
    <div class="fu-body">
      <!-- 证照类型选择 -->
      <div class="fu-type-row">
        <span class="fu-type-label">{{ typeFieldLabel }}</span>
        <t-select v-model="selectedType" :options="typeOptions" :placeholder="typeFieldLabel"
          class="fu-type-select" style="width: 220px" />
        <span class="fu-type-tip">{{ typeTip }}</span>
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
              <template v-else-if="t.status === 'duplicate'">
                <span class="fu-item-status fu-item-status--dup">{{ t.msg || '知识库中已有相同文件' }}</span>
              </template>
              <template v-else-if="t.status === 'recovered'">
                <span class="fu-item-status fu-item-status--ok">已重新提取：{{ t.msg || '' }}</span>
              </template>
              <template v-else-if="t.status === 'failed'">
                <span class="fu-item-status fu-item-status--err">{{ t.msg || '提取失败' }}</span>
              </template>
              <template v-else>
                <t-progress :percentage="t.progress" :label="false" size="small" />
                <span class="fu-item-label">{{ taskLabel(t) }}</span>
              </template>
            </div>
          </div>
          <t-button variant="text" size="small" shape="square" class="fu-item-del" title="删除该上传任务"
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
import { uploadKnowledgeFile, getKnowledgeDetails, updateKnowledgeMetadata } from '@/api/knowledge-base'
import { extractFleetDocument, extractBusinessDocument } from '@/api/fleet'
import { extractUtilityBill, extractSolarBill } from '@/api/knowledge-base'
import { sharedExtractInFlight, saveFlightSnapshot } from '@/composables/useBusinessPolling'
import { isExtractInProgress } from '@/composables/extractConflict'
import { getScope } from '../../registry'

const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  typeOptions?: any[]
  defaultType?: string
  /** 自定义弹窗标题；未传时按 scope 自动推断（invoice→上传发票，其它→上传证照） */
  title?: string
  /**
   * 提取通道。
   * - scope（默认）：按 registry 的 api.extract 分发（发票走 business，车队走 fleet）；
   * - energy-bill：电费 / 光伏账单专用管线，按所选分类调用
   *   extract-utility-bill / extract-solar-bill。
   *
   * 为什么必须显式声明而不是靠 scope：账单提取不走 business 通用管线，
   * 通用管线写的是 energy 台账结构（energy_items），账单列表读的是
   * custom_metadata.records（kind=utility_bill / solar_bill）。
   * 走错通道的表现是「解析提取都完成，但列表里没有这条」。
   */
  extractMode?: 'scope' | 'energy-bill'
}>()
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'done', typeName?: string): void
  (e: 'progress', items: UploadProgressItem[]): void
}>()

interface UpTask {
  key: string
  file: File
  name: string
  size: number
  // recovered：重复上传命中库中已有文件，已自动按其现状恢复（重新提取 / 仅提示）
  status: 'queued' | 'uploading' | 'parsing' | 'extracting' | 'success' | 'failed' | 'duplicate' | 'recovered'
  progress: number
  msg?: string
  kid?: string
  /** 用户点击 × 删除后置位：任务从列表移除，后续解析/提取阶段据此停止 */
  cancelled?: boolean
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

// 发票/合同/制度/通知公告 scope 复用车队上传弹窗：文案与提取接口全部由
// BusinessScopeRegistry 驱动（新增业务只改 registry/<scope>.ts，本文件零改动）。
const scopeDesc = computed(() => getScope(props.scope))
const isInvoice = computed(() => scopeDesc.value?.kind === 'invoice')
const dialogTitle = computed(() => props.title || `上传${scopeDesc.value?.uploadNoun || '证照'}`)
const typeFieldLabel = computed(() => scopeDesc.value?.uploadTypeLabel || '证照类型')
const typeTip = computed(() => scopeDesc.value?.uploadTypeTip || `必须选择${typeFieldLabel.value}，上传后按所选类型归类提取`)

// 证照类型选项必须响应式：父组件 uploadTypeOptions 随分类加载/设置变更更新，
// 若用一次性求值会冻结为初始快照（分类尚未加载时只有内置类型，自定义类型缺失）
const typeOptions = computed(() => (props.typeOptions && props.typeOptions.length ? props.typeOptions : []))

// 复合值 scope__name 解析：公司/司机同名类型在下拉中不冲突，上传与提取按真实 scope 走
const currentScope = computed(() => {
  const head = (selectedType.value || '').split('__', 1)[0]
  return head && head !== selectedType.value ? head : props.scope
})
const certTypeName = computed(() => {
  const parts = (selectedType.value || '').split('__')
  return parts.length > 1 ? parts.slice(1).join('__') : selectedType.value
})

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
  const allDone = list.length > 0 && list.every((t) => t.status === 'success' || t.status === 'failed' || t.status === 'duplicate' || t.status === 'recovered')
  if (allDone) {
    // 仅当本批存在真正新增（success）或自动恢复重提取（recovered）的记录时才通知
    // 父组件联动刷新；其余（重复/失败）不扰动当前列表。
    if (list.some((t) => t.status === 'success' || t.status === 'recovered')) emit('done', certTypeName.value)
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

function onClose() {
  // 关闭时清理已结束（成功/失败）的上传记录，再次打开不残留；进行中的任务保留后台继续
  tasks.value = tasks.value.filter((t) => t.status === 'uploading' || t.status === 'parsing' || t.status === 'extracting')
  emit('update:visible', false)
}
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
  // 必选类型：发票 scope 必须先选发票类型（防误分类），车队必须先选证照类型
  if (!selectedType.value) {
    MessagePlugin.warning(`请先选择${typeFieldLabel.value}`)
    return
  }
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
  // 无需二次确认：立即标记取消并从列表移除；进行中的任务在解析/提取阶段据此停止
  const t = tasks.value.find((x) => x.key === key)
  if (t) t.cancelled = true
  tasks.value = tasks.value.filter((x) => x.key !== key)
}

// ---- 状态展示 ----
function taskIcon(t: UpTask) {
  if (t.status === 'success') return 'check-circle-filled'
  if (t.status === 'recovered') return 'check-circle-filled'
  if (t.status === 'duplicate') return 'info-circle-filled'
  if (t.status === 'failed') return 'error-circle-filled'
  return 'file'
}
function taskColor(t: UpTask) {
  if (t.status === 'success') return 'var(--td-success-color)'
  if (t.status === 'recovered') return 'var(--td-success-color)'
  if (t.status === 'duplicate') return 'var(--td-warning-color)'
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

// 上传并发上限（与后端全局 LLM 提取信号量配套削峰）：
// 20 份文件同时上传 → 同时解析 → 同时触发提取，单文件内部还有批次并发，
// 叠加后必然打爆供应商限流（2026-10-20 实测 429/1302，7 份提取失败）。
// 这里只放 2 个上传任务在飞，其余排队；提取通道本身已是严格单飞（drainExtractQueue）。
const UPLOAD_CONCURRENCY = 2

async function startQueued() {
  const queued = tasks.value.filter((t) => t.status === 'queued')
  if (!queued.length) return
  let cursor = 0
  const worker = async () => {
    for (;;) {
      const t = queued[cursor++]
      if (!t) return
      await runTask(t)
    }
  }
  const lanes = Math.min(UPLOAD_CONCURRENCY, queued.length)
  await Promise.all(Array.from({ length: lanes }, worker))
}

async function runTask(t: UpTask) {
  const scope = currentScope.value
  const certType = certTypeName.value
  try {
    t.status = 'uploading'
    t.progress = 0
    // 分类铁律：把用户所选分类**随上传一起提交**，由后端在创建知识库记录的
    // 第一时间写入 custom_metadata.invoice_category。旧实现依赖 parse 完成后
    // 另发一次 PUT，会和提取落库竞态 → 同批文件交替丢失分类落入「其它票据」。
    const res: any = await uploadKnowledgeFile(props.kbId, {
      file: t.file,
      ...(certType ? { invoice_category: certType } : {}),
      ...(scope ? { fleet_scope: scope } : {}),
    }, (ev: any) => {
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
      if (t.cancelled) return // 用户已删除该任务：停止解析轮询，不再触发提取
      const kd: any = await getKnowledgeDetails(kid)
      const info = kd?.data || kd
      const ps = info?.parse_status || info?.status || ''
      if (ps === 'completed') { t.progress = 100; break }
      if (ps === 'failed') { t.status = 'failed'; t.msg = '解析失败'; return }
      const elapsed = Date.now() - pollStart
      t.progress = Math.min(92, 10 + elapsed / 1000 * 4)
      if (elapsed > 120000) t.msg = '后台解析中（可关闭，完成后自动提取）'
      await sleep(2000)
      // 不设超时退出：后端图片 VLM 解析时长不确定，继续轮询直到 completed/failed；
      // 用户关闭弹窗后，列表 loadPending 仍会在解析完成后自动补打标并提取。
    }

    // 解析完成后再打标（此时不会再被清空），然后排队提取
    if (t.cancelled) return
    try {
      const kd: any = await getKnowledgeDetails(kid)
      let meta2 = kd?.data?.custom_metadata || kd?.custom_metadata || {}
      if (meta2 && meta2.custom_metadata && typeof meta2.custom_metadata === 'object' && Object.keys(meta2).length === 1) meta2 = meta2.custom_metadata
      await updateKnowledgeMetadata(kid, certType ? { ...meta2, fleet_scope: scope, fleet_cert_type: certType } : { ...meta2, fleet_scope: scope })
    } catch { /* 打标失败由列表轮询兜底 */ }

    extractQueue.push(async () => {
      if (t.cancelled) return // 用户已删除该任务：跳过提取，不落库
      try {
        t.status = 'extracting'
        t.progress = 30
        // 共享防重：列表页轮询/手动重新提取/其它并行上传任务可能已在提取同一文件。
        // 若已有人在途，本通道不再重复提交（避免后端 extractMu 409 冲突），
        // 转轮询等待后端 extract_status 落终态后跟随完成。
        if (sharedExtractInFlight.has(kid)) {
          const es = await waitExtractSettled(kid)
          if (es === 'success') { /* 跟随成功，走下方统一置位 */ }
          else if (es === 'failed') { t.status = 'failed'; t.msg = '提取失败'; return }
          else { t.status = 'failed'; t.msg = '提取状态未知，请稍后在列表中重试'; return }
        } else {
          sharedExtractInFlight.add(kid)
          // 立即落盘：提交后若页面刷新（HMR/手动刷新），回填的快照能挡住
          // 下一轮轮询对同一文件的重复提交（否则必撞后端 extractMu 409）
          saveFlightSnapshot()
          try {
            await extractFile(kid, scope, certType)
          } catch (err: any) {
            // 409：后端 extractMu 判定在途，保留 in-flight 由列表轮询终态回收，
            // 外层 catch 走 waitExtractSettled 跟随；其他真实失败删除 in-flight
            // 允许后续重试，避免残留永久卡死。
            if (!isExtractInProgress(err)) {
              sharedExtractInFlight.delete(kid)
              // 同步落盘：否则刷新后回填回旧快照，该文件会被误判为「在途」而无法重试
              saveFlightSnapshot()
            }
            throw err
          }
          // 成功入队（POST 200 不代表提取完成）：保留 in-flight，
          // 由列表轮询在 es 到 success/failed 终态时统一回收，
          // 避免下一轮轮询看到 es=processing 且不在 in-flight 重复提交 → 409。
        }
        t.status = 'success'
        t.progress = 100
      } catch (err: any) {
        // 兜底：即便共享防重仍撞上 409（如后端多实例/旧页面实例在途），
        // 不判失败——转等待终态跟随，数据实际仍由持锁请求完成。
        if (isExtractInProgress(err)) {
          const es = await waitExtractSettled(kid).catch(() => '')
          if (es === 'success') { t.status = 'success'; t.progress = 100; return }
          if (es === 'failed') { t.status = 'failed'; t.msg = '提取失败'; return }
        }
        t.status = 'failed'
        t.msg = err?.message || '提取失败'
      }
    })
    drainExtractQueue()
  } catch (err: any) {
    const code = err?.code ?? err?.error?.code ?? err?.data?.code
    const msg = err?.message || '上传失败'
    // 复用知识库重复文件机制：后端返回 duplicate_file（409）+ recovery 摘要。
    // 关键：不再把用户丢在一句无下一步操作的错误上，而是按 recovery 智能恢复：
    //   ① can_reextract——库中文件没有可用票据 → 自动把分类改为本次所选并重新提取；
    //   ② normal——库中文件已有正常票据 → 提示当前分类/开票日期 + 手动重新提取入口；
    //   ③ 无法判定（删除中/解析失败/非票据等）→ 明确说明原因，提示重新上传。
    if (code === 'duplicate_file' || err?.status === 409 || /already exists|文件重复|重复上传/i.test(msg)) {
      const recovery: any = err?.recovery || err?.data?.recovery || null
      // 优先用 recovery 里的 knowledge id；取不到再回落到"已存在文件"响应体
      const existingId: string = recovery?.knowledge_id
        || err?.data?.id || err?.data?.knowledge_id || ''
      handleDuplicateUpload(t, recovery, existingId)
    } else {
      t.status = 'failed'
      t.msg = msg
    }
  }
}

/**
 * 上传去重冲突的智能恢复。
 *
 * 设计原则（对应「知识库源文件 ↔ 发票列表 100% 对齐」铁律）：
 * 库里有文件却提取不出票据时，用户唯一自救路径是"重新提取"，而旧逻辑用
 * 「知识库中已有相同文件」把这条路堵死了，只能重传——而重传又被哈希锁拦住，
 * 形成死循环。这里直接复用同一个 knowledge，改分类 + 触发重新提取。
 */
async function handleDuplicateUpload(t: UpTask, recovery: any, existingId: string) {
  const scope = currentScope.value
  const certType = certTypeName.value
  const reason: string = recovery?.reason || ''

  // ① 库中文件没有可用票据 → 自动改分类 + 重新提取
  if (recovery?.can_reextract) {
    t.status = 'extracting'
    t.progress = 40
    t.msg = '库中该文件未提取出票据，正在按当前分类重新提取'
    try {
      if (t.kid || existingId) {
        const kid = t.kid || existingId
        t.kid = kid
        // 先按本次所选分类归档，再触发提取（分类写进 fleet_cert_type）
        try {
          await updateKnowledgeMetadata(kid, certType
            ? { fleet_scope: scope, fleet_cert_type: certType }
            : { fleet_scope: scope })
        } catch { /* 打标失败由列表轮询兜底 */ }
        savePending(kid, scope, certType)
        if (!sharedExtractInFlight.has(kid)) {
          sharedExtractInFlight.add(kid)
          saveFlightSnapshot()
          await extractFile(kid, scope, certType)
          sharedExtractInFlight.delete(kid)
        }
      }
      t.status = 'recovered'
      t.msg = '已自动重新提取'
    } catch (e: any) {
      t.status = 'failed'
      t.msg = e?.message || '重新提取失败，请在列表中手动重试'
    }
    return
  }

  // ② 库中文件已有正常票据 → 提示当前分类/开票日期 + 手动重新提取入口
  const cat = recovery?.category ? `（分类：${recovery.category}` : ''
  const date = recovery?.invoice_date ? `，开票日期：${recovery.invoice_date}` : ''
  const count = Number(recovery?.invoice_count || 0)
  const tail = cat || date ? `${cat}${date}）` : ''
  if (recovery?.normal) {
    t.status = 'duplicate'
    t.msg = `知识库中已有相同文件${count ? `（已提取 ${count} 张` : '（已提取'}${tail}，可在列表中查看或重新提取`
    if (existingId) savePending(existingId, scope, certType)
    return
  }

  // ③ 无法自动恢复：明确原因，不误导用户重传（重传仍会被哈希锁拦住）
  t.status = 'duplicate'
  t.msg = reason
    ? `知识库中已有相同文件（${reason}）`
    : '知识库中已有相同文件，请在列表中查看或删除后重试'
  if (existingId) savePending(existingId, scope, certType)
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
  // 账单专用通道：分类决定管线（光伏账单 → extract-solar-bill，其余 → extract-utility-bill）。
  // 不能靠 scope 推断：市电与光伏共用一个知识库，「按库选接口」必然二选一错一半。
  if (props.extractMode === 'energy-bill') {
    if (/光伏/.test(certType)) {
      await extractSolarBill(props.kbId, kid)
    } else {
      await extractUtilityBill(props.kbId, kid)
    }
    return
  }
  const body = certType ? { scope, cert_type: certType } : { scope }
  if (scopeDesc.value?.api.extract === 'business') {
    await extractBusinessDocument(props.kbId, kid, body)
  } else {
    await extractFleetDocument(props.kbId, kid, body)
  }
}

// 等待后端提取状态落终态（供共享防重分支跟随在途请求）：
// 轮询 getKnowledgeDetails 直到 custom_metadata.extract_status 为 success/failed；
// 3 分钟超时兜底，避免与后端模型调用时长脱节导致死等。
const waitExtractSettled = async (kid: string, timeoutMs = 180000): Promise<string> => {
  const start = Date.now()
  for (;;) {
    const kd: any = await getKnowledgeDetails(kid)
    const info = kd?.data || kd
    let meta = info?.custom_metadata || {}
    if (meta && meta.custom_metadata && typeof meta.custom_metadata === 'object' && Object.keys(meta).length === 1) meta = meta.custom_metadata
    const es = meta?.extract_status || ''
    if (es === 'success') return 'success'
    if (es === 'failed') return 'failed'
    if (Date.now() - start > timeoutMs) return 'timeout'
    await sleep(2000)
  }
}

function sleep(ms: number) { return new Promise((r) => setTimeout(r, ms)) }

// 打开时：车队按当前筛选证照类型默认选中（防漏选）；发票 scope 类型必须手动选择（防误分类），不自动注入
watch(() => props.visible, (v) => {
  if (v && !selectedType.value && props.defaultType && !isInvoice.value) {
    selectedType.value = props.defaultType.includes('__') ? props.defaultType : (props.scope + '__' + props.defaultType)
  }
  // 剪贴板粘贴：弹窗打开时挂载全局 paste 监听（支持粘贴图片/文件直接入队）
  if (v) {
    document.addEventListener('paste', onGlobalPaste)
  } else {
    document.removeEventListener('paste', onGlobalPaste)
  }
})

// 剪贴板粘贴入口：从 clipboardData 提取图片/文件（截图粘贴上传）
function onGlobalPaste(e: ClipboardEvent) {
  const files = e.clipboardData?.files
  if (!files || !files.length) return
  addFiles(Array.from(files))
}

onBeforeUnmount(() => {
  // 组件常驻：关闭弹窗不销毁任务，任务在后台继续执行
  document.removeEventListener('paste', onGlobalPaste)
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
      &--dup { color: var(--td-warning-color); }
      &--err { color: var(--td-error-color); }
    }
  }

  // 删除（×）按钮：默认隐藏，悬浮任务行时显示；点击即删无需确认
  .fu-item-del {
    flex: none;
    opacity: 0;
    color: var(--td-text-color-placeholder);
    transition: opacity 0.2s ease;

    &:hover {
      color: var(--td-error-color);
    }
  }

  &:hover .fu-item-del,
  .fu-item-del:focus-visible {
    opacity: 1;
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
