<!--
  随单附件袋（第 78 轮）。

  定位：维保单据在现实中是**一叠纸**——主证之外还有竣工检测单、维修发票、过程照片。
  这些附件只需要「随单留存 + 页内秒看」，**完全不需要解析**。

  铁律：
    · 上传走 `/fleet/records/:id/attachments`，后端纯 BYTEA 直存、零解析触发；
    · 预览走无后缀路由 `/fleet/attachments/view?id=...`，生产 Nginx 不会 404；
    · 删除必须 `t-popconfirm` 二次确认（附件是不可恢复的原始凭证）；
    · 空值一律空白，不渲染 `—`。
-->
<template>
  <div class="attach-bag" data-attach-bag>
    <div class="attach-bag__head">
      <span class="attach-bag__title">相关附件</span>
      <span class="attach-bag__count" v-if="items.length">{{ items.length }} 个</span>
      <span class="attach-bag__hint">仅随单留存，不做解析</span>
    </div>

    <!-- 拖拽区 + 标签选择：标签统一作用于本批 -->
    <div class="attach-bag__drop" :class="{ 'is-over': dragOver }"
      @dragover.prevent="dragOver = true" @dragleave.prevent="dragOver = false"
      @drop.prevent="onDrop">
      <t-icon name="cloud-upload" size="20px" class="attach-bag__drop-icon" />
      <div class="attach-bag__drop-text">
        拖拽文件到此处，或
        <button type="button" class="attach-bag__link" data-attach-pick @click="pick">点击选择</button>
        <span class="attach-bag__drop-tip">支持 PDF / JPG / PNG / WEBP，单个 ≤ 20MB</span>
      </div>
      <t-select v-model="tag" size="small" class="attach-bag__tag" :clearable="false"
        data-attach-tag :options="tagOptions" />
      <input ref="fileInput" type="file" multiple class="attach-bag__file"
        accept="application/pdf,image/png,image/jpeg,image/webp" @change="onPick" />
    </div>

    <div v-if="loading" class="attach-bag__empty">加载中…</div>
    <div v-else-if="!items.length" class="attach-bag__empty"></div>
    <ul v-else class="attach-bag__list">
      <li v-for="a in items" :key="a.id" class="attach-bag__row" :data-attach-id="a.id">
        <span class="attach-bag__tag-chip">{{ a.tag }}</span>
        <button type="button" class="attach-bag__name" :title="a.file_name" @click="preview(a)">
          {{ a.file_name }}
        </button>
        <span class="attach-bag__size">{{ sizeText(a.file_size) }}</span>
        <button type="button" class="attach-bag__act" title="预览" @click="preview(a)">
          <t-icon name="browse" size="15px" />
        </button>
        <button type="button" class="attach-bag__act" title="下载" @click="download(a)">
          <t-icon name="download" size="15px" />
        </button>
        <t-popconfirm theme="warning" placement="left"
          content="删除后不可恢复，确认删除该附件吗？"
          :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
          @confirm="remove(a)">
          <button type="button" class="attach-bag__act is-danger" title="删除">
            <t-icon name="delete" size="15px" />
          </button>
        </t-popconfirm>
      </li>
    </ul>

    <FilePreviewDialog v-model:visible="previewVisible" :src="previewSrc"
      :file-name="previewName" :file-type="previewType" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import FilePreviewDialog from '@/components/business/FilePreviewDialog.vue'
import {
  deleteFleetAttachment, fleetAttachmentViewUrl, listFleetRecordAttachments,
  uploadFleetRecordAttachments, type FleetAttachment,
} from '@/api/fleet'

const props = withDefaults(defineProps<{
  recordId: string
  /** 只读模式：不允许上传/删除（查看型抽屉） */
  readonly?: boolean
}>(), { readonly: false })

const emit = defineEmits<{ (e: 'changed', count: number): void }>()

/** 附件标签白名单（与后端 fleetAttachmentTags 同源）。 */
const TAG_OPTIONS = ['竣工检测单', '维修发票', '结算清单', '过程照片', '其它附件']
const tagOptions = TAG_OPTIONS.map((t) => ({ label: t, value: t }))
const tag = ref('竣工检测单')

const items = ref<FleetAttachment[]>([])
const loading = ref(false)
const dragOver = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const previewVisible = ref(false)
const previewSrc = ref('')
const previewName = ref('')
const previewType = ref('')

async function load() {
  if (!props.recordId) { items.value = []; return }
  loading.value = true
  try {
    const res: any = await listFleetRecordAttachments(props.recordId)
    const list = res?.data?.items ?? res?.items ?? []
    items.value = Array.isArray(list) ? list : []
    emit('changed', items.value.length)
  } catch {
    items.value = []
  } finally {
    loading.value = false
  }
}

watch(() => props.recordId, () => { void load() }, { immediate: true })

function pick() {
  if (props.readonly) return
  fileInput.value?.click()
}

async function upload(files: File[]) {
  if (!files.length || props.readonly) return
  try {
    const res: any = await uploadFleetRecordAttachments(props.recordId, files, tag.value)
    const n = Number(res?.data?.count ?? files.length) || files.length
    MessagePlugin.success(`已上传 ${n} 个附件`)
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '附件上传失败')
  }
}

function onPick(ev: Event) {
  const input = ev.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  void upload(files)
}

function onDrop(ev: DragEvent) {
  dragOver.value = false
  if (props.readonly) return
  const files = Array.from(ev.dataTransfer?.files || [])
  void upload(files)
}

/** 预览：走无后缀路由（生产 Nginx 安全），由 FilePreviewDialog 页内渲染。 */
function preview(a: FleetAttachment) {
  previewSrc.value = fleetAttachmentViewUrl(a.id)
  previewName.value = a.file_name
  previewType.value = a.mime_type
  previewVisible.value = true
}

function download(a: FleetAttachment) {
  // 页内触发下载，不开新窗口（铁律：禁止 window.open）
  const link = document.createElement('a')
  link.href = fleetAttachmentViewUrl(a.id)
  link.download = a.file_name
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

async function remove(a: FleetAttachment) {
  try {
    await deleteFleetAttachment(a.id)
    MessagePlugin.success('已删除')
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

function sizeText(n: number): string {
  const b = Number(n) || 0
  if (b <= 0) return ''
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / 1024 / 1024).toFixed(1)} MB`
}

onMounted(() => { void load() })
onBeforeUnmount(() => { previewSrc.value = '' })

defineExpose({ reload: load })
</script>

<style lang="less" scoped>
.attach-bag {
  display: flex;
  flex-direction: column;
  gap: 8px;

  &__head {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
  }
  &__title { font-weight: 600; color: var(--td-text-color-primary); }
  &__count {
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    border-radius: 10px;
    padding: 0 8px;
    font-size: 12px;
    line-height: 18px;
  }
  &__hint { color: var(--td-text-color-placeholder); font-size: 12px; }

  &__drop {
    position: relative;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px;
    border: 1px dashed var(--td-component-border);
    border-radius: 6px;
    background: var(--td-bg-color-container);
    transition: border-color 0.2s, background 0.2s;

    &.is-over {
      border-color: var(--td-brand-color);
      background: var(--td-brand-color-light);
    }
  }
  &__drop-icon { color: var(--td-text-color-placeholder); flex: none; }
  &__drop-text { flex: 1; min-width: 0; font-size: 13px; color: var(--td-text-color-secondary); }
  &__drop-tip { display: block; font-size: 12px; color: var(--td-text-color-placeholder); margin-top: 2px; }
  &__link {
    border: none;
    background: none;
    padding: 0;
    color: var(--td-brand-color);
    cursor: pointer;
    font-size: 13px;
  }
  &__tag { width: 132px; flex: none; }
  &__file { display: none; }

  &__empty {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
    padding: 2px 0;
    min-height: 20px;
  }

  &__list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  &__row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 8px;
    border-radius: 4px;
    background: var(--td-bg-color-secondarycontainer);
    font-size: 13px;
  }
  &__tag-chip {
    flex: none;
    font-size: 12px;
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-radius: 3px;
    padding: 0 6px;
    line-height: 18px;
  }
  &__name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
    border: none;
    background: none;
    padding: 0;
    cursor: pointer;
    color: var(--td-text-color-primary);
    font-size: 13px;

    &:hover { color: var(--td-brand-color); }
  }
  &__size { flex: none; color: var(--td-text-color-placeholder); font-size: 12px; }
  &__act {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    border: none;
    border-radius: 3px;
    background: none;
    color: var(--td-text-color-secondary);
    cursor: pointer;

    &:hover { background: var(--td-bg-color-container-hover); color: var(--td-brand-color); }
    &.is-danger:hover { color: var(--td-error-color); }
  }
}
</style>
