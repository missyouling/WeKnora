<!--
  中台通用导入弹窗（第 58 轮）。

  唯一视觉标准 = 花名册-在职管理导入弹窗（图 10）：
    ① 顶部模板下载条（ⓘ 下载{模块}标准模板 + 绿色描边「下载导入模板」）
    ② 导入模式分段按钮（合并更新 / 仅新增 / 仅更新；单模式时整行隐藏）
    ③ 虚线拖拽上传区（点击选择或拖拽，支持 .xlsx / .xls，可多选）
    ④ 已选文件列表
    ⑤ 底部 [取消] + 绿色实心 [开始导入]

  契约：组件只负责交互与校验，**不直接调业务接口** —— 下载与导入都由调用方
  通过 `on-download` / `on-import` 回调注入，便于各模块复用同一套 UI。
-->
<template>
  <t-dialog :visible="visible" :header="title" width="640px" :footer="false" placement="center"
    class="standard-import-dialog" data-std-import @close="close"
    @update:visible="(v: boolean) => { if (!v) close() }">
    <div class="si-body">
      <!-- ① 模板下载 -->
      <div v-if="showTemplate" class="si-template-row">
        <div class="si-template-tip">
          <t-icon name="info-circle" size="16px" />
          <span>{{ templateHint || `下载${moduleLabel}标准模板` }}</span>
        </div>
        <t-button theme="primary" variant="outline" size="small" :loading="downloading" @click="onDownloadClick">
          <template #icon><t-icon name="download" size="14px" /></template>
          下载导入模板
        </t-button>
      </div>

      <!-- ② 导入模式 -->
      <div v-if="modeOptions.length > 1" class="si-mode-row">
        <span class="si-label">导入模式</span>
        <t-radio-group v-model="mode" size="small">
          <t-radio-button v-for="m in modeOptions" :key="m.value" :value="m.value">{{ m.label }}</t-radio-button>
        </t-radio-group>
      </div>

      <!-- ③ 拖拽上传 -->
      <div class="si-dropzone" :class="{ 'si-dropzone--over': dragOver }" @click="pickFile"
        @dragover.prevent="dragOver = true" @dragleave.prevent="dragOver = false" @drop.prevent="onDrop">
        <t-icon name="upload" size="28px" class="si-drop-icon" />
        <div class="si-drop-title">点击选择或拖拽 Excel 文件到此处</div>
        <div class="si-drop-sub">{{ acceptHint }}</div>
        <input ref="fileInputRef" type="file" :accept="accept" :multiple="multiple" hidden @change="onInputChange" />
      </div>

      <!-- ④ 已选文件 -->
      <div v-if="files.length" class="si-file-list">
        <div v-for="(f, i) in files" :key="`${f.name}-${i}`" class="si-file">
          <t-icon name="file-excel" size="18px" />
          <span class="si-file-name" :title="f.name">{{ f.name }}</span>
          <span class="si-file-size">{{ fmtSize(f.size) }}</span>
          <t-button variant="text" size="small" shape="square" title="移除文件" @click.stop="removeFile(i)">
            <template #icon><t-icon name="close" size="16px" /></template>
          </t-button>
        </div>
      </div>

      <!-- ⑤ 底部操作 -->
      <div class="si-footer">
        <t-button variant="outline" size="small" @click="close">取消</t-button>
        <t-button theme="primary" size="small" :disabled="!files.length" :loading="importing"
          data-std-import-start @click="doImport">
          开始导入
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

export interface ImportModeOption { label: string; value: string }

const props = withDefaults(defineProps<{
  visible: boolean
  /** 弹窗标题，默认「导入」。 */
  title?: string
  /** 模块名，用于「下载{模块}标准模板」文案。 */
  moduleLabel?: string
  /** 是否显示模板下载条（社保原始申报文件不需要模板时置 false）。 */
  showTemplate?: boolean
  templateHint?: string
  /** 导入模式；只给 1 项时不显示切换行（如回盘表固定覆盖更新）。 */
  modes?: ImportModeOption[]
  defaultMode?: string
  /** 允许一次选多个文件（社保正常/异常各险种一个文件）。 */
  multiple?: boolean
  accept?: string
  acceptHint?: string
  /** 下载模板（返回 Blob）。 */
  onDownload?: () => Promise<any>
  /** 执行导入，返回提示条数；抛错时由组件统一提示。 */
  onImport?: (files: File[], mode: string) => Promise<number>
}>(), {
  title: '导入',
  moduleLabel: '',
  showTemplate: true,
  templateHint: '',
  modes: () => [
    { label: '合并更新', value: 'merge' },
    { label: '仅新增', value: 'insert' },
    { label: '仅更新', value: 'update' },
  ],
  defaultMode: 'merge',
  multiple: false,
  accept: '.xlsx,.xls',
  acceptHint: '支持 .xlsx / .xls 格式',
  onDownload: undefined,
  onImport: undefined,
})

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'imported', count: number): void
}>()

const mode = ref(props.defaultMode)
const files = ref<File[]>([])
const dragOver = ref(false)
const downloading = ref(false)
const importing = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const modeOptions = computed(() => props.modes || [])

watch(() => props.visible, (v) => {
  if (v) { files.value = []; dragOver.value = false; mode.value = props.defaultMode }
})

const fmtSize = (n: number): string => {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
const isExcel = (name: string): boolean => /\.(xlsx|xls)$/i.test(String(name || ''))

function addFiles(list: File[]) {
  const ok = list.filter((f) => isExcel(f.name))
  if (!ok.length) { MessagePlugin.warning('请选择 .xlsx 或 .xls 文件'); return }
  files.value = props.multiple ? [...files.value, ...ok] : [ok[0]]
}
function pickFile() { fileInputRef.value?.click() }
function onInputChange(e: Event) {
  const input = e.target as HTMLInputElement
  addFiles(input.files ? Array.from(input.files) : [])
  input.value = ''
}
function onDrop(e: DragEvent) {
  dragOver.value = false
  addFiles(e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [])
}
function removeFile(i: number) { files.value = files.value.filter((_, idx) => idx !== i) }

function downloadBlob(blob: any, name: string) {
  const b = blob instanceof Blob
    ? blob
    : new Blob([blob], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
  const url = URL.createObjectURL(b)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  a.click()
  URL.revokeObjectURL(url)
}

async function onDownloadClick() {
  if (!props.onDownload) return
  downloading.value = true
  try {
    downloadBlob(await props.onDownload(), `${props.moduleLabel || '导入'}模板.xlsx`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '模板下载失败')
  } finally {
    downloading.value = false
  }
}

async function doImport() {
  if (!files.value.length || !props.onImport) return
  importing.value = true
  try {
    const count = await props.onImport(files.value, mode.value)
    MessagePlugin.success(`成功导入 ${count} 条 ${props.moduleLabel}记录`)
    emit('imported', count)
    close()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '导入失败，请检查文件格式')
  } finally {
    importing.value = false
  }
}

function close() {
  emit('update:visible', false)
  files.value = []
  dragOver.value = false
}
</script>

<style lang="less" scoped>
.si-body { display: flex; flex-direction: column; gap: 12px; }
.si-template-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
}
.si-template-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--td-text-color-secondary);
}
.si-mode-row { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.si-label { color: var(--td-text-color-secondary); }
.si-dropzone {
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

  &:hover,
  &--over {
    border-color: var(--td-brand-color);
    background: var(--td-brand-color-light);
  }
}
.si-drop-icon { color: var(--td-brand-color); }
.si-drop-title { color: var(--td-text-color-primary); }
.si-drop-sub { color: var(--td-text-color-placeholder); }
.si-file-list { display: flex; flex-direction: column; gap: 6px; max-height: 150px; overflow-y: auto; }
.si-file {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
}
.si-file-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.si-file-size { color: var(--td-text-color-secondary); }
.si-footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; }
</style>
