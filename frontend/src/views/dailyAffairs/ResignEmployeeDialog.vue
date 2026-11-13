<template>
  <t-dialog :visible="visible" header="办理离职" width="560px" :footer="false" placement="center"
    class="resign-employee-dialog" @close="close" @update:visible="(v: boolean) => { if (!v) close() }">
    <div class="rd-body">
      <!-- 第 55 轮：单条办理时明确提示对象姓名 -->
      <div v-if="employeeName" class="rd-summary">正在为「{{ employeeName }}」办理离职，状态变更为「离职」。</div>
      <div v-else class="rd-summary">将为已选的 {{ ids.length }} 名员工办理离职，状态变更为「离职」。</div>

      <div class="rd-field">
        <label class="rd-label"><span class="rd-required">*</span>离职时间</label>
        <t-date-picker v-model="resignDate" value-type="YYYY-MM-DD" format="YYYY-MM-DD"
          clearable allow-input class="rd-control" />
      </div>

      <div class="rd-field">
        <label class="rd-label"><span class="rd-required">*</span>离职原因</label>
        <t-select v-model="reason" filterable clearable placeholder="请选择或搜索离职原因" class="rd-control">
          <t-option-group v-for="g in RESIGN_REASONS" :key="g.group" :label="g.group">
            <t-option v-for="o in g.options" :key="o" :label="o" :value="o" />
          </t-option-group>
        </t-select>
      </div>

      <div class="rd-field">
        <label class="rd-label">离职证明</label>
        <div class="rd-control rd-proof">
          <input ref="proofInputRef" type="file" accept=".pdf,.jpg,.jpeg,.png" hidden @change="onProofChange" />
          <t-button variant="outline" size="small" :loading="uploading" @click="proofInputRef?.click()">
            <template #icon><t-icon name="upload" size="14px" /></template>
            {{ proofName ? '重新上传' : '上传附件' }}
          </t-button>
          <span v-if="proofName" class="rd-proof-name" :title="proofName">
            <t-icon name="attachment" size="14px" />{{ proofName }}
            <t-icon name="close" size="14px" class="rd-proof-clear" @click="clearProof" />
          </span>
          <span v-else class="rd-hint">支持 PDF / JPG / PNG，不超过 10MB</span>
        </div>
      </div>

      <div class="rd-field">
        <label class="rd-label">备注说明</label>
        <t-textarea v-model="remark" :maxlength="500" :autosize="{ minRows: 3, maxRows: 6 }"
          placeholder="可记录工作交接、离职补偿或其它备注事项" class="rd-control" />
      </div>

      <div class="rd-footer">
        <t-button variant="outline" size="small" @click="close">取消</t-button>
        <t-button theme="primary" size="small" :loading="submitting" @click="submit">确认办理</t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { RESIGN_REASONS } from './hrRosterFields'
import * as hrApi from './hrApi'

const props = withDefaults(defineProps<{
  visible: boolean
  ids: Array<string | number>
  defaultReason?: string
  /** 第 55 轮：单选办理时显示员工姓名。 */
  employeeName?: string
}>(), { visible: false, ids: () => [], defaultReason: '', employeeName: '' })

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'done'): void
}>()

const today = () => {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const resignDate = ref(today())
const reason = ref('')
const remark = ref('')
const proofPath = ref('')
const proofName = ref('')
const uploading = ref(false)
const submitting = ref(false)
const proofInputRef = ref<HTMLInputElement | null>(null)

watch(() => props.visible, (v) => {
  if (!v) return
  resignDate.value = today()
  reason.value = props.defaultReason || ''
  remark.value = ''
  proofPath.value = ''
  proofName.value = ''
})

function clearProof() {
  proofPath.value = ''
  proofName.value = ''
}

async function onProofChange(e: Event) {
  const input = e.target as HTMLInputElement
  const f = input.files?.[0]
  input.value = ''
  if (!f) return
  if (!/\.(pdf|jpe?g|png)$/i.test(f.name)) {
    MessagePlugin.warning('离职证明仅支持 PDF / JPG / PNG 格式')
    return
  }
  if (f.size > 10 * 1024 * 1024) {
    MessagePlugin.warning('附件不能超过 10MB')
    return
  }
  uploading.value = true
  try {
    const fd = new FormData()
    fd.append('file', f)
    const res: any = await hrApi.uploadResignProof(fd)
    const data = res?.data || res || {}
    proofPath.value = String(data.path || '')
    proofName.value = f.name
  } catch (err: any) {
    MessagePlugin.error(err?.message || '附件上传失败')
  } finally {
    uploading.value = false
  }
}

async function submit() {
  if (!resignDate.value) { MessagePlugin.warning('请选择离职时间'); return }
  if (!reason.value) { MessagePlugin.warning('请选择离职原因'); return }
  if (!props.ids.length) { MessagePlugin.warning('请先勾选员工'); return }
  submitting.value = true
  try {
    await hrApi.resignRosterEmployees(props.ids, resignDate.value, reason.value, proofPath.value, remark.value)
    MessagePlugin.success(`已为 ${props.ids.length} 名员工办理离职`)
    emit('done')
    close()
  } catch (err: any) {
    MessagePlugin.error(err?.message || '办理离职失败')
  } finally {
    submitting.value = false
  }
}

function close() { emit('update:visible', false) }
</script>

<style lang="less" scoped>
.rd-body { display: flex; flex-direction: column; gap: 14px; }

.rd-summary {
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
}

.rd-field { display: flex; align-items: flex-start; gap: 10px; }

.rd-label {
  flex: 0 0 84px;
  line-height: 32px;
  color: var(--td-text-color-primary);
}

.rd-required { color: var(--td-error-color); }
.rd-control { flex: 1; min-width: 0; }

.rd-proof {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.rd-proof-name {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  color: var(--td-text-color-secondary);
}

.rd-proof-clear { cursor: pointer; }
.rd-hint { color: var(--td-text-color-placeholder); }

.rd-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 6px;
  border-top: 1px solid var(--td-component-border);
}
</style>
