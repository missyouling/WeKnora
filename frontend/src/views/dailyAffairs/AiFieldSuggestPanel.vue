<template>
  <!--
    内嵌式「智能字段生成」面板（取代原 AiFieldSuggestDialog 模态框）。
    为什么去弹窗：用户在场内改的是某个分类的字段，弹窗遮罩把"我在改哪个分类"
    的上下文盖掉了；内嵌在编辑面板顶部，上传→识别→勾选→保存视线不跳。
  -->
  <section v-if="visible" class="ai-field-panel">
    <div class="afp-head">
      <span class="afp-title"><t-icon name="star" size="15px" /> 智能字段生成</span>
      <span class="afp-sub">上传「{{ certType }}」样票，AI 识别票面字段并映射到系统字段</span>
      <div class="spacer" />
      <t-button variant="text" size="small" @click="onClose">收起</t-button>
    </div>

    <t-alert theme="info" variant="light" class="afp-alert">
      识别结果仅供参考，需你勾选并点「保存」后才会写入字段配置；样票文本会被临时缓存，供「提取规则」页校验正则。
    </t-alert>

    <!-- 输入区：识别成功后自动折叠。
         为什么折叠：粘贴模式 textarea 占 270px 以上，识别完还摊着会把下方
         「字段配置」和「保存」推到视口外，内嵌面板反而比弹窗更难用。 -->
    <template v-if="!inputCollapsed">
      <div class="afp-upload">
        <t-upload v-model:files="fileList" :auto-upload="false" :max-size="20" accept=".pdf,.png,.jpg,.jpeg,.txt"
          placeholder="支持 PDF / PNG / JPG / TXT，最大 20MB" />
        <t-button variant="text" size="small" @click="textMode = !textMode">
          {{ textMode ? '改为上传样票' : '改为粘贴票面文本' }}
        </t-button>
        <div class="spacer" />
        <t-button theme="primary" size="small" :loading="loading" :disabled="!canRun" @click="runSuggest">
          开始识别
        </t-button>
      </div>
      <t-textarea v-if="textMode" v-model="sampleText" class="afp-textarea"
        :autosize="{ minRows: 5, maxRows: 12 }" placeholder="粘贴票面识别出的文本（OCR 结果即可）" />
      <t-alert v-if="sampleText.length > 12000" theme="warning" variant="light" class="afp-alert">
        文本超过 12000 字符，后端会自动截断，可能影响后半部分字段的识别。
      </t-alert>
    </template>
    <div v-else class="afp-collapsed">
      <t-icon name="check-circle-filled" size="14px" />
      <span class="afp-collapsed-text">已用{{ textMode ? '粘贴的票面文本' : '上传的样票' }}完成识别</span>
      <div class="spacer" />
      <t-button variant="text" size="small" @click="inputCollapsed = false">重新选择样票</t-button>
      <t-button variant="text" size="small" :loading="loading" :disabled="!canRun" @click="runSuggest">重新识别</t-button>
    </div>

    <!-- 识别结果：标准 / 扩展 / 明细 三分组，统一五列（勾选｜编码｜名称｜类型｜样本） -->
    <div v-if="result" class="afp-result">
      <div class="afp-result-head">
        <span class="afp-result-title">识别结果</span>
        <span class="afp-result-tip">
          已勾选 {{ pickedCount }} 项 / 共 {{ totalCount }} 项
          <template v-if="result.sample_text_truncated">（样票文本已截断）</template>
        </span>
      </div>

      <div v-if="!totalCount" class="afp-empty">
        未能从该样票中识别出可用字段。可换一张票面更完整的样票，或改用「粘贴票面文本」。
      </div>

      <!-- 三个分组固定展示：某一组本样票没有命中时给空态，避免用户以为"少了一类"。
           结果区限高内滚：字段多时（20 项以上）不能把下方「字段配置」顶出视口。 -->
      <div class="afp-scroll">
        <template v-for="g in groups" :key="g.key">
        <div class="afp-group">
          <div class="afp-group-head">
            <span class="afp-group-title">{{ g.title }}</span>
            <span class="afp-group-hint">{{ g.hint }}</span>
          </div>
          <div class="afp-grid afp-grid--head">
            <span></span>
            <span>字段编码</span>
            <span>字段名称</span>
            <span>数据类型</span>
            <span>样本</span>
          </div>
          <div v-if="!g.rows.length" class="afp-group-empty">本样票未识别到该组字段</div>
          <div v-for="(r, i) in g.rows" :key="`${r.key}-${i}`" class="afp-grid afp-row">
            <t-checkbox v-model="r.picked" />
            <span class="afp-key" :title="r.key">{{ r.key }}</span>
            <t-input v-model="r.label" size="small" placeholder="中文表头" class="afp-label" />
            <t-select v-model="r.data_type" size="small" :options="typeOptions" class="afp-type" />
            <t-tooltip :content="r.sample || '样票中未识别到取值'" placement="top">
              <span class="afp-sample" :class="{ 'afp-sample--empty': !r.sample }">{{ r.sample || '—' }}</span>
            </t-tooltip>
          </div>
        </div>
      </template>

        <t-alert v-if="skipped.length" theme="warning" variant="light" class="afp-alert">
          有 {{ skipped.length }} 项建议未被采纳：
          <ul class="afp-skip-list">
            <li v-for="(s, i) in skipped" :key="`skip-${i}`"><b>{{ s.name }}</b> — {{ s.reason }}</li>
          </ul>
        </t-alert>
      </div>

      <div class="afp-actions">
        <span v-if="pickedSubCount && !hasItemsField" class="afp-auto-hint">
          <t-icon name="info-circle" size="13px" /> 当前分类没有「明细」字段，保存时会自动创建「项目明细」父字段
        </span>
        <div class="spacer" />
        <t-button variant="outline" size="small" @click="onClose">取消</t-button>
        <t-button theme="primary" size="small" :disabled="!pickedCount" @click="emitApply">
          并入字段配置（{{ pickedCount }}）
        </t-button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { UploadFile } from 'tdesign-vue-next'
import { aiSuggestFields, type AiSuggestFieldsResult } from '@/api/fleet'

/**
 * t-upload 的文件项直接复用 TDesign 的 UploadFile 类型。
 * 注意：原生 File 在 `.raw`（TDesign 1.x 契约），不存在 `.file`；
 * v-model 对它不生效，必须 `v-model:files`（change 首参是数组不是 context）。
 */
const props = defineProps<{
  visible: boolean
  kbId: string
  scope: string
  certType: string
  /** 当前分类是否已有「明细」类型字段（决定是否需要提示自动补建父字段）。 */
  hasItemsField?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'apply', payload: { standard: any[]; subFields: any[]; custom: any[]; sampleText: string }): void
}>()

/** 三分组共用同一行结构：勾选 / 编码(只读) / 名称(可改) / 类型(可改) / 样本。 */
interface SuggestRow {
  key: string
  label: string
  data_type: string
  sample?: string
  picked: boolean
}

const typeOptions = [
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '日期', value: 'date' },
  { label: '数组', value: 'array' },
]

const fileList = ref<UploadFile[]>([])
const sampleText = ref('')
const textMode = ref(false)
// 识别成功后折叠输入区（textarea 很高，不折叠会把字段配置顶出视口）
const inputCollapsed = ref(false)
const loading = ref(false)
const result = ref<AiSuggestFieldsResult | null>(null)
const standardRows = ref<SuggestRow[]>([])
const subRows = ref<SuggestRow[]>([])
const customRows = ref<SuggestRow[]>([])
const skipped = ref<Array<{ name: string; reason: string }>>([])

const canRun = computed(() => {
  if (loading.value) return false
  return fileList.value[0]?.raw ? true : !!sampleText.value.trim()
})

const pickedCount = computed(() =>
  standardRows.value.filter((r) => r.picked).length
  + subRows.value.filter((r) => r.picked).length
  + customRows.value.filter((r) => r.picked).length,
)
const pickedSubCount = computed(() => subRows.value.filter((r) => r.picked).length)
const totalCount = computed(() => standardRows.value.length + subRows.value.length + customRows.value.length)

const groups = computed(() => [
  {
    key: 'standard',
    title: '标准字段',
    hint: '映射到系统契约键，编码不可改（保证提取与正则补洞命中）',
    rows: standardRows.value,
  },
  {
    key: 'custom',
    title: '扩展字段',
    hint: '票面有、字典里没有的信息；编码由系统生成且只读',
    rows: customRows.value,
  },
  {
    key: 'sub',
    title: '明细字段',
    hint: '挂到「项目明细」父字段下，作为明细行输出口径',
    rows: subRows.value,
  },
])

watch(() => props.visible, (v) => {
  if (!v) resetState()
})

function resetState() {
  fileList.value = []
  sampleText.value = ''
  result.value = null
  standardRows.value = []
  subRows.value = []
  customRows.value = []
  skipped.value = []
  loading.value = false
  textMode.value = false
  inputCollapsed.value = false
}

function normalizeType(t?: string): string {
  return t === 'number' || t === 'date' || t === 'array' ? t : 'text'
}

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
    if (!data || !Array.isArray(data.standard_fields)) throw new Error('识别结果格式异常')
    result.value = data
    // 默认只勾选"票面真实出现取值"的条目：AI 猜的字段不该默认进配置
    standardRows.value = data.standard_fields
      .filter((f) => f.enable && f.sample)
      .map((f) => ({
        key: f.key,
        label: String(f.label || f.key),
        data_type: normalizeType(f.data_type),
        sample: f.sample,
        picked: true,
      }))
    customRows.value = (data.custom_fields || []).map((c) => ({
      key: String(c.key || ''),
      label: String(c.name || ''),
      data_type: normalizeType(c.data_type),
      sample: c.sample,
      // AI 猜的扩展字段（无样本值）默认不勾选
      picked: !!c.sample,
    }))
    subRows.value = (data.item_sub_fields || []).map((s) => ({
      key: s.key,
      label: String(s.label || s.key),
      data_type: normalizeType(s.data_type),
      sample: '',
      picked: true,
    }))
    // 后端丢弃的建议要能看到原因，否则用户会以为"模型漏识别了"
    skipped.value = data.skipped || []
    const n = totalCount.value
    // 识别成功即折叠输入区，把屏幕让给结果表与下方「字段配置」
    if (n) inputCollapsed.value = true
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
    standard: standardRows.value
      .filter((r) => r.picked)
      .map((r) => ({ key: r.key, label: r.label.trim() || r.key, data_type: r.data_type, enable: true, sample: r.sample })),
    subFields: subRows.value
      .filter((r) => r.picked)
      .map((r) => ({ key: r.key, label: r.label.trim() || r.key, data_type: r.data_type, width: 0 })),
    custom: customRows.value
      .filter((r) => r.picked && !!r.key && !!r.label.trim())
      .map((r) => ({ key: r.key, name: r.label.trim(), data_type: r.data_type, sample: r.sample })),
    sampleText: result.value?.sample_text || sampleText.value || '',
  })
}

function onClose() {
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.ai-field-panel {
  margin: 12px 0;
  padding: 12px;
  border: 1px solid var(--td-brand-color-light);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.afp-head {
  display: flex;
  align-items: center;
  gap: 8px;
  .afp-title {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }
  .afp-sub { font-size: 12px; color: var(--td-text-color-placeholder); }
  .spacer { flex: 1; }
}
.afp-alert { font-size: 12px; line-height: 1.6; }
.afp-skip-list { margin: 4px 0 0; padding-left: 18px; }

.afp-upload {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 6px;
  .spacer { flex: 1; }
}
.afp-textarea {
  width: 100%;
  font-family: Consolas, Monaco, 'Courier New', monospace;
  font-size: 12px;
}
/* 识别成功后的输入区折叠条：一行说明 + 重新选择/重新识别，替代原 textarea 占位 */
.afp-collapsed {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 6px;
  color: var(--td-text-color-secondary);
  .afp-collapsed-text { font-size: 12px; }
  .spacer { flex: 1; }
}
/* 结果区限高内滚：字段多时锁住面板高度，保证「字段配置」与「保存」仍在视口附近 */
.afp-scroll {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: min(46vh, 420px);
  overflow-y: auto;
  padding-right: 2px;
}

.afp-result { display: flex; flex-direction: column; gap: 10px; }
.afp-result-head { display: flex; align-items: baseline; gap: 8px; }
.afp-result-title { font-size: 13px; font-weight: 600; color: var(--td-text-color-primary); }
.afp-result-tip { font-size: 12px; color: var(--td-text-color-placeholder); }
.afp-empty { padding: 16px 0; text-align: center; font-size: 12px; color: var(--td-text-color-placeholder); }

.afp-group {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
}
.afp-group-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 6px 10px;
  background: var(--td-bg-color-secondarycontainer);
  .afp-group-title { font-size: 12px; font-weight: 600; color: var(--td-text-color-primary); }
  .afp-group-hint { font-size: 11px; color: var(--td-text-color-placeholder); }
}

/* 三分组统一五列：勾选 22 / 编码 0.9fr / 名称 1fr / 类型 96 / 样本 1fr */
.afp-group-empty {
  padding: 8px 10px;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}
.afp-grid {
  display: grid;
  grid-template-columns: 22px minmax(110px, 0.9fr) minmax(110px, 1fr) 96px minmax(80px, 1fr);
  align-items: center;
  gap: 8px;
  padding: 5px 10px;
  font-size: 12px;
  &--head {
    color: var(--td-text-color-secondary);
    border-bottom: 1px solid var(--td-component-stroke);
    font-weight: 500;
  }
}
.afp-row {
  border-bottom: 1px solid var(--td-component-stroke);
  &:last-child { border-bottom: none; }
  &:hover { background: var(--td-bg-color-secondarycontainer); }
}
.afp-key {
  font-family: Consolas, Monaco, 'Courier New', monospace;
  font-size: 11px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 4px;
  padding: 2px 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.afp-sample {
  font-size: 11px;
  color: var(--td-text-color-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  &--empty { color: var(--td-text-color-placeholder); }
}

.afp-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  .spacer { flex: 1; }
  .afp-auto-hint {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: var(--td-warning-color);
  }
}
</style>
