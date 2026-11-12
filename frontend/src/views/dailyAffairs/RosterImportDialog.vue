<template>
  <t-dialog :visible="visible" header="导入" width="640px" :footer="false" placement="center"
    class="roster-import-dialog" @close="close" @update:visible="(v: boolean) => { if (!v) close() }">
    <div class="ri-body">
      <!-- 模板下载栏：第 52 轮按模块生成各自专属的 3-Sheet 标准模板 -->
      <div class="ri-template-row">
        <div class="ri-template-tip">
          <t-icon name="info-circle" size="16px" />
          <span>下载{{ moduleLabel }}标准模板</span>
        </div>
        <t-button theme="primary" variant="outline" size="small" :loading="downloading" @click="downloadTemplate">
          <template #icon><t-icon name="download" size="14px" /></template>
          下载导入模板
        </t-button>
      </div>

      <!-- 导入模式 -->
      <div class="ri-mode-row">
        <span class="ri-label">导入模式</span>
        <t-radio-group v-model="mode" size="small">
          <t-radio-button value="merge">合并更新</t-radio-button>
          <t-radio-button value="insert">仅新增</t-radio-button>
          <t-radio-button value="update">仅更新</t-radio-button>
        </t-radio-group>
      </div>

      <!-- 拖拽 / 点击上传区：视觉对齐发票管理上传弹窗 -->
      <div class="ri-dropzone" :class="{ 'ri-dropzone--over': dragOver }" @click="pickFile"
        @dragover.prevent="dragOver = true" @dragleave.prevent="dragOver = false" @drop.prevent="onDrop">
        <t-icon name="upload" size="28px" class="ri-drop-icon" />
        <div class="ri-drop-title">点击选择或拖拽 Excel 文件到此处</div>
        <div class="ri-drop-sub">支持 .xlsx / .xls 格式</div>
        <input ref="fileInputRef" type="file" accept=".xlsx,.xls" hidden @change="onInputChange" />
      </div>

      <!-- 已选文件 -->
      <div v-if="file" class="ri-file">
        <t-icon name="file-excel" size="18px" />
        <span class="ri-file-name" :title="file.name">{{ file.name }}</span>
        <span class="ri-file-size">{{ fmtSize(file.size) }}</span>
        <t-button variant="text" size="small" shape="square" title="移除文件" @click.stop="clearFile">
          <template #icon><t-icon name="close" size="16px" /></template>
        </t-button>
      </div>

      <!-- 底部操作 -->
      <div class="ri-footer">
        <t-button variant="outline" size="small" @click="close">取消</t-button>
        <t-button theme="primary" size="small" :disabled="!file" :loading="importing" @click="doImport">
          开始导入
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import * as hrApi from './hrApi'
import { HR_TEMPLATE_NAMES, type HrModule } from './hrFieldSchema'

const props = withDefaults(defineProps<{
  visible: boolean
  /** 第 52 轮：三套独立模板（花名册 / 离职管理 / 劳动合同）。 */
  module?: HrModule
}>(), { visible: false, module: 'roster' })

const templateName = computed(() => HR_TEMPLATE_NAMES[props.module] || HR_TEMPLATE_NAMES.roster)
const moduleLabel = computed(() => ({ roster: '花名册', resign: '离职管理', contract: '劳动合同' })[props.module] || '花名册')
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'imported', count: number): void
}>()

const mode = ref<'merge' | 'insert' | 'update'>('merge')

const file = ref<File | null>(null)
const dragOver = ref(false)
const downloading = ref(false)
const importing = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)

function fmtSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function isExcel(name: string): boolean {
  return /\.(xlsx|xls)$/i.test(String(name || ''))
}

function addFiles(files: File[]) {
  const picked = files.find((f) => isExcel(f.name))
  if (!picked) {
    MessagePlugin.warning('请选择 .xlsx 或 .xls 文件')
    return
  }
  file.value = picked
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
function clearFile() { file.value = null }

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

async function downloadTemplate() {
  downloading.value = true
  try {
    const blob = await hrApi.getHrTemplate(props.module)
    downloadBlob(blob, templateName.value)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '模板下载失败')
  } finally {
    downloading.value = false
  }
}

async function doImport() {
  if (!file.value) return
  importing.value = true
  try {
    const fd = new FormData()
    fd.append('file', file.value)
    fd.append('mode', mode.value)
    fd.append('default_status', '在职')
    const res: any = await hrApi.importHrModule(props.module, fd)
    const data = res?.data || res || {}
    const count = Number(data.inserted || 0) + Number(data.updated || 0)
    MessagePlugin.success(`成功导入 ${count} 条${moduleLabel.value}记录`)
    emit('imported', count)
    close()
  } catch (e: any) {
    const conflicts = e?.response?.data?.conflicts || e?.conflicts || e?.data?.conflicts
    if (Array.isArray(conflicts) && conflicts.length) {
      MessagePlugin.warning(`检测到 ${conflicts.length} 条离职冲突，请勾选「强制流转」或调整名单`)
    } else {
      MessagePlugin.error(e?.message || '导入失败，请检查文件格式')
    }
  } finally {
    importing.value = false
  }
}

function close() {
  emit('update:visible', false)
  file.value = null
  dragOver.value = false
}
</script>

<style lang="less" scoped>
.ri-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ri-template-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
}

.ri-template-tip {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--td-text-color-secondary);
}

.ri-mode-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.ri-label,
.ri-mode-tip,
.ri-footer-tip {
  color: var(--td-text-color-secondary);
}

.ri-dropzone {
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

.ri-drop-icon { color: var(--td-brand-color); }
.ri-drop-title { color: var(--td-text-color-primary); }
.ri-drop-sub { color: var(--td-text-color-placeholder); }

.ri-file {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
}

.ri-file-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ri-file-size { color: var(--td-text-color-secondary); }

.ri-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.ri-footer-tip {
  flex: 1;
  min-width: 0;
}
</style>
