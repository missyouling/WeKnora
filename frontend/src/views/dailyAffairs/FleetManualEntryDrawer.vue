<template>
  <t-drawer
    :visible="visible"
    header="手动新增记录"
    :footer="false"
    width="620px"
    placement="right"
    :close-on-overlay-click="false"
    @close="onClose"
  >
    <div class="manual-entry" @paste="onPaste">
      <t-form label-align="top" :data="form" @submit="onSubmit">
        <t-form-item label="维保类型" name="docType">
          <t-select v-model="form.docType" placeholder="请选择维保类型" @change="onTypeChange">
            <t-option v-for="t in typeOptions" :key="t" :value="t" :label="t" />
          </t-select>
        </t-form-item>

        <template v-if="form.docType">
          <t-form-item v-for="f in fieldDefs" :key="f.name" :label="f.name">
            <t-date-picker
              v-if="f.kind === 'date'"
              v-model="form.fields[f.name]"
              placeholder="请选择日期"
              style="width: 100%"
              enable-time-picker
            />
            <t-input
              v-else-if="f.kind === 'number'"
              v-model="form.fields[f.name]"
              type="number"
              :placeholder="'请输入' + f.name"
            />
            <t-input
              v-else
              v-model="form.fields[f.name]"
              :placeholder="'请输入' + f.name"
            />
          </t-form-item>

          <t-form-item label="备注" name="remark">
            <t-textarea v-model="form.remark" placeholder="备注（可选）" :autosize="{ minRows: 2, maxRows: 4 }" />
          </t-form-item>
        </template>

        <t-form-item label="附件（点击/拖拽/直接粘贴截图）">
          <div
            class="attach-drop"
            :class="{ dragover: dragOver }"
            @click="pickFile"
            @dragover.prevent="dragOver = true"
            @dragleave.prevent="dragOver = false"
            @drop.prevent="onDrop"
          >
            <t-icon name="upload" size="24px" />
            <div class="attach-tip">点击选择或拖拽文件到此处，也可直接 Ctrl+V 粘贴截图</div>
            <div class="attach-sub">支持 PDF / 图片 / Word / Excel，可多个</div>
          </div>
          <input ref="fileInputRef" type="file" multiple style="display: none"
            accept=".pdf,.png,.jpg,.jpeg,.webp,.bmp,.doc,.docx,.xls,.xlsx" @change="onInputChange" />
          <div v-if="attachments.length" class="attach-list">
            <div v-for="(a, i) in attachments" :key="i" class="attach-item">
              <t-icon :name="a.status === 'done' ? 'file' : 'loading'" size="16px" />
              <span class="attach-name">{{ a.name }}</span>
              <span v-if="a.status === 'uploading'" class="attach-pct">{{ a.percent }}%</span>
              <t-icon v-if="a.status !== 'uploading'" name="close" size="14px" class="attach-remove" @click="removeAttach(i)" />
            </div>
          </div>
        </t-form-item>
      </t-form>

      <div class="manual-footer">
        <t-button variant="outline" @click="onClose">取消</t-button>
        <t-button theme="primary" :loading="saving" :disabled="!form.docType" @click="onSubmit">
          保存
        </t-button>
      </div>
    </div>
  </t-drawer>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { createFleetRecord } from '@/api/fleet'
import { uploadKnowledgeFile } from '@/api/knowledge-base'

const props = defineProps<{ visible: boolean; kbId: string; scope?: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'saved'): void }>()

// 维保类型：与 FleetRecordList.BUILTIN_CERTS 的 maintain 内置类型保持一致
const BUILTIN: Record<string, { base: string[]; detail: string[] }> = {
  '维修工单': { base: ['工单号', '车牌号', '维修日期', '维修项目', '工时费', '材料费', '总费用'], detail: ['服务商', '备注'] },
  '二级维护': { base: ['维护日期', '车牌号', '维护项目', '维护单位', '下次维护日期'], detail: [] },
  '保险单': { base: ['保单号', '被保险人', '保险公司', '险种', '车牌号', '保额', '保费', '起保日期', '终保日期'], detail: [] },
}
const typeOptions = Object.keys(BUILTIN)

interface FieldDef { name: string; kind: 'text' | 'number' | 'date' }
const form = reactive<{ docType: string; fields: Record<string, string>; remark: string }>({
  docType: '', fields: {}, remark: '',
})
const fieldDefs = ref<FieldDef[]>([])

function fieldKind(name: string): FieldDef['kind'] {
  if (name.includes('日期')) return 'date'
  if (/(费|金额|额|价|款|里程|次|数|号)/.test(name) && !name.includes('日期') && !name.includes('单位') && !name.includes('项目')) return 'number'
  return 'text'
}
function onTypeChange() {
  form.fields = {}
  const def = BUILTIN[form.docType]
  const names = [...(def?.base || []), ...(def?.detail || [])]
  fieldDefs.value = names.filter((n) => n !== '备注').map((n) => ({ name: n, kind: fieldKind(n) }))
}

// ---- 附件 ----
const fileInputRef = ref<HTMLInputElement | null>(null)
const dragOver = ref(false)
const saving = ref(false)
interface Attach { name: string; status: 'uploading' | 'done' | 'error'; percent: number; kid: string }
const attachments = ref<Attach[]>([])

function pickFile() { fileInputRef.value?.click() }
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
function onPaste(e: ClipboardEvent) {
  const items = e.clipboardData?.files ? Array.from(e.clipboardData.files) : []
  if (items.length) { e.preventDefault(); addFiles(items) }
}
async function addFiles(files: File[]) {
  if (!props.kbId) { MessagePlugin.warning('知识库尚未就绪'); return }
  for (const f of files) {
    const att: Attach = { name: f.name, status: 'uploading', percent: 0, kid: '' }
    attachments.value.push(att)
    try {
      const res: any = await uploadKnowledgeFile(props.kbId, { file: f }, (ev: any) => {
        att.percent = Math.round((ev?.loaded / ev?.total) * 100) || 0
      })
      att.kid = res?.id || res?.data?.id || res?.data?.knowledge_id || ''
      att.status = 'done'
      att.percent = 100
    } catch (err: any) {
      att.status = 'error'
      MessagePlugin.error(`附件 ${f.name} 上传失败`)
    }
  }
}
function removeAttach(i: number) { attachments.value.splice(i, 1) }

async function onSubmit() {
  if (!form.docType) { MessagePlugin.warning('请选择维保类型'); return }
  saving.value = true
  try {
    // 日期字段值标准化
    const data: Record<string, unknown> = {}
    for (const f of fieldDefs.value) {
      const v = form.fields[f.name]
      if (v === undefined || v === '') continue
      if (f.kind === 'number') data[f.name] = Number(v)
      else data[f.name] = v
    }
    // record_date 取第一个日期字段
    const dateField = fieldDefs.value.find((f) => f.kind === 'date')
    const recordDate = dateField && form.fields[dateField.name]
      ? String(form.fields[dateField.name]).slice(0, 10) : new Date().toISOString().slice(0, 10)
    const firstDone = attachments.value.find((a) => a.status === 'done')
    await createFleetRecord({
      record_type: 'maintain-archive',
      doc_type: form.docType,
      record_date: recordDate,
      data,
      remark: form.remark,
      file_name: firstDone?.name || '',
      doc_knowledge_id: firstDone?.kid || '',
    })
    MessagePlugin.success('已保存')
    emit('saved')
    onClose()
  } catch (err: any) {
    MessagePlugin.error(err?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

function onClose() {
  form.docType = ''; form.fields = {}; form.remark = ''
  fieldDefs.value = []; attachments.value = []
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.manual-entry { padding: 0 24px 24px; }
.attach-drop {
  border: 1px dashed var(--td-component-stroke);
  border-radius: var(--td-radius-medium);
  padding: 24px;
  text-align: center;
  color: var(--td-text-color-secondary);
  cursor: pointer;
  transition: border-color .2s;
  &:hover, &.dragover { border-color: var(--td-brand-color); }
}
.attach-tip { margin-top: 8px; font-size: var(--td-font-size-body-medium); color: var(--td-text-color-primary); }
.attach-sub { margin-top: 4px; font-size: var(--td-font-size-body-small); }
.attach-list { margin-top: 8px; }
.attach-item {
  display: flex; align-items: center; gap: 8px;
  padding: 6px 8px; border-radius: var(--td-radius-small);
  background: var(--td-bg-color-secondarycontainer);
  margin-bottom: 4px;
}
.attach-name { flex: 1; font-size: var(--td-font-size-body-small); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attach-pct { font-size: var(--td-font-size-body-small); color: var(--td-brand-color); }
.attach-remove { cursor: pointer; color: var(--td-text-color-secondary); }
.manual-footer { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
</style>
