<template>
  <t-dialog :visible="visible" header="智能字段生成" width="720px" :footer="false"
    :close-on-overlay-click="false" @close="onClose">
    <div v-if="certType" class="ai-field-body">
      <t-alert theme="info" variant="light" class="ai-field-alert">
        上传一张「{{ certType }}」的真实票面，AI 会识别票面上有哪些字段并映射到系统标准字段。
        <strong>识别结果仅供参考，需你勾选并保存后才会写入字段配置</strong>；样票文本会被临时缓存，供「提取规则」页校验正则。
      </t-alert>

      <!-- 上传区 -->
      <div class="ai-upload">
        <t-upload v-model:files="fileList" :auto-upload="false" :max-size="20" accept=".pdf,.png,.jpg,.jpeg,.txt"
          placeholder="支持 PDF / PNG / JPG / TXT，最大 20MB" />
      </div>

      <div class="ai-actions">
        <t-button variant="outline" size="small" @click="textMode = !textMode">
          {{ textMode ? '改为上传样票' : '改为粘贴票面文本' }}
        </t-button>
        <div class="spacer" />
        <t-button variant="outline" size="small" @click="onClose">取消</t-button>
        <t-button theme="primary" size="small" :loading="loading" :disabled="!canRun" @click="runSuggest">
          开始识别
        </t-button>
      </div>

      <t-textarea v-if="textMode" v-model="sampleText" class="ai-textarea"
        :autosize="{ minRows: 6, maxRows: 14 }" placeholder="粘贴票面识别出的文本（OCR 结果即可）" />
      <t-alert v-if="sampleText.length > 12000" theme="warning" variant="light" class="ai-field-alert">
        文本超过 12000 字符，后端会自动截断，可能影响后半部分字段的识别。
      </t-alert>
    </div>

    <!-- 识别结果：三类建议 + 可勾选 / 可改名 -->
    <div v-if="result" class="ai-result">
      <div class="ai-result-head">
        <span class="ai-result-title">识别结果</span>
        <span class="ai-result-tip">
          已勾选 {{ pickedCount }} 项 / 共 {{ totalCount }} 项
          <template v-if="result.sample_text_truncated">（样票文本已截断）</template>
        </span>
      </div>

      <div v-if="!standardPicked.length && !subFields.length && !customFields.length"
        class="ai-result-empty">
        未能从该样票中识别出可用字段。可换一张票面更完整的样票，或改用「粘贴票面文本」。
      </div>

      <!-- ① 标准字段 -->
      <div v-if="standardPicked.length" class="ai-block">
        <div class="ai-block-title">
          标准字段（勾选后按系统契约键新增/启用）
        </div>
        <div v-for="f in standardPicked" :key="f.key" class="ai-row">
          <t-checkbox v-model="f.picked" />
          <span class="ai-row-label">{{ labelOf(f) }}</span>
          <t-tag size="small" variant="outline" class="ai-row-key">{{ f.key }}</t-tag>
          <span class="ai-row-sample" :class="{ 'ai-sample-empty': !f.sample }">
            {{ f.sample || '样票中未识别到取值' }}
          </span>
        </div>
      </div>

      <!-- ② 明细子列 -->
      <div v-if="subFields.length" class="ai-block">
        <div class="ai-block-title">
          明细子列（挂到「明细」类型字段上，可改键名与列头）
        </div>
        <div v-if="!hasItemsField" class="ai-warn">
          当前分类没有「明细」类型字段，子列将无法保存。建议先在字段配置里把某个字段的数据类型改为「明细」。
        </div>
        <div v-for="(s, i) in subFields" :key="`${s.key}-${i}`" class="ai-row">
          <t-checkbox v-model="s.picked" />
          <t-input v-model="s.key" size="small" class="ai-sub-key" placeholder="子列键（英文）" />
          <t-input v-model="s.label" size="small" class="ai-sub-label" placeholder="列头显示名" />
        </div>
      </div>

      <!-- ③ 自定义字段 -->
      <div v-if="customFields.length" class="ai-block">
        <div class="ai-block-title">
          扩展字段（票面有、字典里没有的信息）
        </div>
        <div v-for="(c, i) in customFields" :key="`${c.name}-${i}`" class="ai-row">
          <t-checkbox v-model="c.picked" />
          <t-input v-model="c.name" size="small" class="ai-custom-name" placeholder="字段名" />
          <t-tag size="small" variant="outline" :theme="c.store === 'remark' ? 'warning' : 'default'">
            {{ c.store === 'remark' ? '并入备注' : '独立扩展字段' }}
          </t-tag>
          <span class="ai-row-sample" :class="{ 'ai-sample-empty': !c.sample }">
            {{ c.sample || '样票中未识别到取值' }}
          </span>
        </div>
      </div>

      <t-alert v-if="skipped.length" theme="warning" variant="light" class="ai-field-alert">
        有 {{ skipped.length }} 项建议未被采纳：
        <ul class="ai-skip-list">
          <li v-for="(s, i) in skipped" :key="`skip-${i}`"><b>{{ s.name }}</b> — {{ s.reason }}</li>
        </ul>
      </t-alert>

      <div class="ai-actions">
        <t-button variant="outline" size="small" @click="onClose">取消</t-button>
        <t-button theme="primary" size="small" :disabled="!pickedCount" @click="emitApply">
          并入字段配置（{{ pickedCount }}）
        </t-button>
      </div>
    </div>
  </t-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { UploadFile } from 'tdesign-vue-next'
import { aiSuggestFields, type AiSuggestFieldsResult } from '@/api/fleet'
import { aiStandardFieldLabel } from './aiFieldMerge'

/**
 * t-upload 的文件项直接复用 TDesign 的 UploadFile 类型。
 * 注意：原生 File 在 `.raw`（TDesign 1.x 契约），不存在 `.file` ——
 * 之前误读 `.file` 导致上传后「开始识别」永远置灰，UI 实测才发现。
 */

const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  certType: string
  /** 当前分类是否已有「明细」类型字段（决定明细子列能否保存）。 */
  hasItemsField?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'apply', payload: { standard: any[]; subFields: any[]; custom: any[]; sampleText: string }): void
}>()

interface StandardRow { key: string; label?: string; data_type?: string; sample?: string; picked: boolean }
interface SubRow { key: string; label?: string; data_type?: string; width?: number; picked: boolean }
interface CustomRow { name: string; data_type?: string; sample?: string; store?: string; picked: boolean }

const fileList = ref<UploadFile[]>([])
const sampleText = ref('')
const textMode = ref(false)
const loading = ref(false)
const result = ref<AiSuggestFieldsResult | null>(null)
const standardPicked = ref<StandardRow[]>([])
const subFields = ref<SubRow[]>([])
const customFields = ref<CustomRow[]>([])
const skipped = ref<Array<{ name: string; reason: string }>>([])

const canRun = computed(() => {
  if (loading.value) return false
  const file = fileList.value[0]?.raw
  return file ? true : !!sampleText.value.trim()
})
const pickedCount = computed(
  () => standardPicked.value.filter((r) => r.picked).length
    + subFields.value.filter((r) => r.picked).length
    + customFields.value.filter((r) => r.picked).length,
)
const totalCount = computed(
  () => standardPicked.value.length + subFields.value.length + customFields.value.length,
)

function labelOf(f: StandardRow): string {
  return aiStandardFieldLabel(f.key, f.label)
}


watch(() => props.visible, (v) => {
  if (!v) {
    fileList.value = []
    sampleText.value = ''
    result.value = null
    standardPicked.value = []
    subFields.value = []
    customFields.value = []
    skipped.value = []
    loading.value = false
    textMode.value = false
  }
})

async function runSuggest() {
  if (!props.kbId) { MessagePlugin.error('缺少知识库 ID'); return }
  loading.value = true
  try {
    const file = textMode.value ? null : (fileList.value[0]?.raw || null)
    const res: any = await aiSuggestFields(props.kbId, {
      scope: props.scope || 'invoice',
      cert_type: props.certType,
      file,
      text: textMode.value ? sampleText.value : undefined,
    })
    const data = (res?.data || res) as AiSuggestFieldsResult
    if (!data || !Array.isArray(data.standard_fields)) {
      throw new Error('识别结果格式异常')
    }
    result.value = data
    // 默认只勾选"票面真实出现取值"的条目：AI 猜的字段不该默认进配置
    standardPicked.value = data.standard_fields
      .filter((f) => f.enable && f.sample)
      .map((f) => ({ ...f, picked: true }))
    subFields.value = (data.item_sub_fields || []).map((s) => ({ ...s, picked: true }))
    customFields.value = (data.custom_fields || []).map((c) => ({ ...c, picked: !!c.sample }))
    // 后端丢弃的建议要能看到原因，否则用户会以为"模型漏识别了"
    skipped.value = data.skipped || []
    const n = totalCount.value
    if (!n) MessagePlugin.warning('未能从该样票识别出字段，建议换一张票面更完整的样票')
    else MessagePlugin.success(`识别完成，共 ${n} 项建议`)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '识别失败')
  } finally {
    loading.value = false
  }
}

function emitApply() {
  emit('apply', {
    standard: standardPicked.value.filter((r) => r.picked),
    subFields: subFields.value.filter((r) => r.picked),
    custom: customFields.value.filter((r) => r.picked && !!r.name.trim()),
    sampleText: result.value?.sample_text || sampleText.value || '',
  })
}

function onClose() {
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.ai-field-body,
.ai-result {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.ai-field-alert {
  font-size: 12px;
  line-height: 1.6;
}

.ai-upload {
  padding: 12px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 8px;
}

.ai-textarea {
  width: 100%;
  font-family: Consolas, Monaco, 'Courier New', monospace;
  font-size: 12px;
}

.ai-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  .spacer { flex: 1; }
}

.ai-result-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.ai-result-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.ai-result-tip {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.ai-result-empty {
  padding: 20px 0;
  text-align: center;
  font-size: 13px;
  color: var(--td-text-color-placeholder);
}

.ai-block {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 10px 12px;
}
.ai-block-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-secondary);
  margin-bottom: 8px;
}
.ai-warn {
  font-size: 12px;
  color: var(--td-warning-color);
  margin-bottom: 8px;
}

.ai-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  border-bottom: 1px dashed var(--td-component-stroke);
  &:last-child { border-bottom: none; }
}

.ai-row-label {
  min-width: 96px;
  font-size: 13px;
  color: var(--td-text-color-primary);
}
.ai-row-key {
  font-family: Consolas, Monaco, monospace;
  font-size: 11px;
}
.ai-row-sample {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ai-skip-list {
  margin: 4px 0 0;
  padding-left: 18px;
  li { line-height: 1.7; }
}

.ai-sample-empty {
  font-style: italic;
  color: var(--td-text-color-placeholder);
}

.ai-sub-key {
  width: 180px;
  flex: 0 0 180px;
}
.ai-sub-label {
  flex: 1;
  min-width: 0;
}
.ai-custom-name {
  width: 160px;
  flex: 0 0 160px;
}
</style>