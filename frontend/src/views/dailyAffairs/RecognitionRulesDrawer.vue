<template>
  <div class="rr-wrap">
    <div v-if="visible" class="rr-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="rr-resize-line" />
    </div>
    <t-drawer :visible="visible" :header="`设置 · ${moduleName}`" :size="String(drawerWidth)" :footer="false"
      destroy-on-close class="recognition-rules-drawer" @close="onClose">
      <div class="rr-body">
        <div class="rr-hint">规则命中即认定；类型按优先级匹配覆盖模型结果。</div>

        <!-- 包含判定规则 -->
        <div class="rr-section">
          <div class="rr-section-head">
            <span class="rr-section-title">包含判定</span>
            <t-switch v-model="cfg.enabled" size="small" />
            <span class="rr-state">{{ cfg.enabled ? '生效' : '停用' }}</span>
            <span class="rr-spacer" />
            <t-button variant="outline" size="small" @click="addIncludeRule">
              <template #icon><t-icon name="add" size="14px" /></template>
              添加
            </t-button>
          </div>
          <div v-if="!cfg.include_rules.length" class="rr-empty">暂无规则，模型判定为准</div>
          <div v-for="(r, i) in cfg.include_rules" :key="r.id" class="rr-row" :class="{ 'rr-row--last': i === cfg.include_rules.length - 1 }">
            <div class="rr-row-main">
              <t-input v-model="r.name" placeholder="规则名" size="small" class="rr-name" />
              <t-select v-model="r.match_type" size="small" class="rr-match" :options="MATCH_TYPE_OPTS" />
              <t-textarea v-if="r.match_type === 'keyword'" v-model="keywordsText[i]" placeholder="关键词，逗号分隔"
                :autosize="{ minRows: 1, maxRows: 2 }" size="small" class="rr-keywords" @change="syncKeywords(i)" />
              <t-input v-else v-model="r.regex" placeholder="正则" size="small" class="rr-keywords" />
              <t-select v-if="r.match_type === 'keyword'" v-model="r.logic" size="small" class="rr-logic" :options="LOGIC_OPTS" />
            </div>
            <div class="rr-row-side">
              <t-switch v-model="r.enabled" size="small" />
              <t-button variant="text" size="small" shape="square" class="rr-del" @click="cfg.include_rules.splice(i, 1)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
        </div>

        <!-- 类型归类规则 -->
        <div class="rr-section">
          <div class="rr-section-head">
            <span class="rr-section-title">类型归类</span>
            <t-button variant="text" size="small" class="rr-regex-tip" :title="'正则示例'"
              @click="regexTipVisible = !regexTipVisible">
              <template #icon><t-icon name="help-circle" size="14px" /></template>
              正则示例
            </t-button>
            <span class="rr-spacer" />
            <t-button variant="outline" size="small" @click="addTypeRule">
              <template #icon><t-icon name="add" size="14px" /></template>
              添加
            </t-button>
          </div>
          <div v-if="regexTipVisible" class="rr-regex-tip-box">
            <div v-for="(t, i) in REGEX_EXAMPLES" :key="i" class="rr-regex-item">
              <code class="rr-regex-code">{{ t.pattern }}</code>
              <span class="rr-regex-desc">{{ t.desc }}</span>
            </div>
          </div>
          <div v-if="!cfg.type_rules.length" class="rr-empty">
            {{ moduleName === '发票' ? '内置关键字兜底：通行费→普通、专用→专用' : '暂无规则，使用模型结果' }}
          </div>
          <div v-for="(r, i) in cfg.type_rules" :key="r.id" class="rr-row rr-row--type" :class="{ 'rr-row--last': i === cfg.type_rules.length - 1 }">
            <div class="rr-row-main">
              <t-input v-model="r.pattern" :placeholder="r.is_regex ? '正则' : '关键词'" size="small" class="rr-pattern" />
              <t-checkbox v-model="r.is_regex" size="small" class="rr-regex-check">正则</t-checkbox>
              <t-select v-model="r.type" size="small" class="rr-type-val" :options="typeOptions" filterable placeholder="类型" />
              <t-input v-model="r.priority" type="number" size="small" class="rr-priority" :min="1" :max="99" />
            </div>
            <div class="rr-row-side">
              <t-switch v-model="r.enabled" size="small" />
              <t-button variant="text" size="small" shape="square" class="rr-del" @click="cfg.type_rules.splice(i, 1)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
        </div>

        <!-- 分类管理 -->
        <div class="rr-section">
          <div class="rr-section-head">
            <span class="rr-section-title">分类管理</span>
            <span class="rr-state">用于归类目标与筛选</span>
          </div>
          <div class="rr-type-list">
            <div v-for="(t, i) in cfg.types" :key="i" class="rr-type-chip">
              <t-input v-model="cfg.types[i]" size="small" class="rr-type-name" @change="onTypeRenamed(t, cfg.types[i], i)" />
              <t-button variant="text" size="small" shape="square" class="rr-del" :title="'删除分类'"
                @click="removeType(t, i)">
                <template #icon><t-icon name="close" size="14px" /></template>
              </t-button>
            </div>
            <div class="rr-type-chip rr-type-add">
              <t-input v-model="newType" size="small" class="rr-type-name" placeholder="新增分类" @enter="addType" />
              <t-button variant="outline" size="small" @click="addType">
                <template #icon><t-icon name="add" size="14px" /></template>
                新增
              </t-button>
            </div>
          </div>
        </div>

        <!-- 措施管理（奖惩） -->
        <div v-if="moduleName === '奖惩'" class="rr-section">
          <div class="rr-section-head">
            <span class="rr-section-title">措施管理</span>
            <span class="rr-state">编辑记录时多选加载</span>
          </div>
          <div class="rr-type-list">
            <div v-for="(m, i) in cfg.measures" :key="i" class="rr-type-chip">
              <t-select v-model="cfg.measures[i].type" size="small" class="rr-measure-type" :options="measureTypeOptions"
                filterable clearable placeholder="类型" />
              <t-input v-model="cfg.measures[i].name" size="small" class="rr-type-name" />
              <t-button variant="text" size="small" shape="square" class="rr-del" :title="'删除措施'"
                @click="removeMeasure(i)">
                <template #icon><t-icon name="close" size="14px" /></template>
              </t-button>
            </div>
            <div class="rr-type-chip rr-type-add">
              <t-select v-model="newMeasureType" size="small" class="rr-measure-type" :options="measureTypeOptions"
                filterable clearable placeholder="类型" />
              <t-input v-model="newMeasure" size="small" class="rr-type-name" placeholder="新增措施" @enter="addMeasure" />
              <t-button variant="outline" size="small" @click="addMeasure">
                <template #icon><t-icon name="add" size="14px" /></template>
                新增
              </t-button>
            </div>
          </div>
        </div>

        <div class="rr-actions">
          <t-button theme="default" variant="outline" :loading="reassessing" @click="onReassess">
            <template #icon><t-icon name="refresh" size="15px" /></template>
            重新评估存量
          </t-button>
          <t-button theme="primary" :loading="saving" @click="onSave">
            <template #icon><t-icon name="check" size="15px" /></template>
            保存
          </t-button>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { getRecognitionConfig, saveRecognitionConfig, reassessRecognition } from '@/api/knowledge-base'

interface RecognitionRule {
  id: string
  name: string
  match_type: 'keyword' | 'regex'
  keywords?: string[]
  logic?: 'AND' | 'OR'
  regex?: string
  enabled: boolean
}
interface TypeClassifyRule {
  id: string
  pattern: string
  is_regex: boolean
  type: string
  priority: number
  enabled: boolean
}
interface RecognitionConfig {
  enabled: boolean
  include_rules: RecognitionRule[]
  type_rules: TypeClassifyRule[]
  types: string[]
  measures: MeasureItem[]
}
interface MeasureItem { name: string; type: string }

const props = defineProps<{ visible: boolean; kbId: string; moduleName: '发票' | '合同' | '制度' | '奖惩' }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const MATCH_TYPE_OPTS = [
  { label: '关键词', value: 'keyword' },
  { label: '正则', value: 'regex' },
]
const LOGIC_OPTS = [
  { label: '全部（AND）', value: 'AND' },
  { label: '任一（OR）', value: 'OR' },
]
const REGEX_EXAMPLES = [
  { pattern: '^.*专用.*$', desc: '包含"专用"' },
  { pattern: '合同|协议|采购', desc: '任一出现' },
  { pattern: '^(?!.*测试).*', desc: '不含"测试"' },
  { pattern: '\\d{4}-\\d{2}-\\d{2}', desc: '日期格式' },
]

const uid = () => `r-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`

const cfg = ref<RecognitionConfig>({ enabled: true, include_rules: [], type_rules: [], types: [], measures: [] })
const keywordsText = ref<string[]>([])
const saving = ref(false)
const reassessing = ref(false)
const newType = ref('')
const newMeasure = ref('')
const newMeasureType = ref('')
const regexTipVisible = ref(false)

const typeOptions = computed(() =>
  (cfg.value.types || []).filter(Boolean).map(t => ({ label: t, value: t }))
)
const measureTypeOptions = computed(() =>
  (cfg.value.types || []).filter(Boolean).map(t => ({ label: t, value: t }))
)

const syncKeywords = (i: number) => {
  const t = keywordsText.value[i] || ''
  cfg.value.include_rules[i].keywords = t.split(/[,，\n]/).map(s => s.trim()).filter(Boolean)
}

const addIncludeRule = () => {
  cfg.value.include_rules.push({ id: uid(), name: '', match_type: 'keyword', keywords: [], logic: 'OR', regex: '', enabled: true })
  keywordsText.value.push('')
}
const addTypeRule = () => {
  cfg.value.type_rules.push({ id: uid(), pattern: '', is_regex: false, type: '', priority: cfg.value.type_rules.length + 1, enabled: true })
}

const normalize = (c: RecognitionConfig): RecognitionConfig => ({
  enabled: !!c?.enabled,
  include_rules: (c?.include_rules || []).map((r, i) => {
    const keywords = Array.isArray(r.keywords) ? r.keywords.filter(Boolean) : []
    keywordsText.value[i] = keywords.join('，')
    return {
      id: r.id || uid(), name: r.name || '', match_type: r.match_type === 'regex' ? 'regex' : 'keyword',
      keywords: keywords.length ? keywords : undefined, logic: r.logic === 'AND' ? 'AND' : 'OR',
      regex: r.regex || '', enabled: r.enabled !== false,
    }
  }),
  type_rules: (c?.type_rules || []).map((r, i) => ({
    id: r.id || uid(), pattern: r.pattern || '', is_regex: !!r.is_regex, type: r.type || '',
    priority: Number(r.priority) > 0 ? Number(r.priority) : i + 1, enabled: r.enabled !== false,
  })),
  types: Array.isArray(c?.types) ? c.types.filter(Boolean) : [],
  measures: Array.isArray(c?.measures) ? c.measures.map((m: any) => ({
    name: m.name || '', type: m.type || '',
  })).filter((m: MeasureItem) => m.name) : [],
})

const load = async () => {
  try {
    const res: any = await getRecognitionConfig(props.kbId)
    const c = res?.data || res
    cfg.value = normalize(c)
  } catch {
    cfg.value = { enabled: true, include_rules: [], type_rules: [], types: [], measures: [] }
    keywordsText.value = []
  }
}

watch(() => props.visible, (v) => { if (v) { load(); loadDrawerWidth() } })

const onClose = () => emit('update:visible', false)

const addType = () => {
  const t = (newType.value || '').trim()
  if (!t) return
  if (cfg.value.types.includes(t)) { MessagePlugin.warning('该分类已存在'); return }
  cfg.value.types.push(t)
  newType.value = ''
}
const removeType = (t: string, i: number) => {
  const refs = (cfg.value.type_rules || []).filter(r => r.type === t && r.enabled)
  if (refs.length > 0) {
    MessagePlugin.warning(`该分类正被 ${refs.length} 条启用规则引用，请先删除规则`)
    return
  }
  cfg.value.types.splice(i, 1)
}
const onTypeRenamed = (oldType: string, newTypeVal: string, i: number) => {
  const v = (newTypeVal || '').trim()
  if (!v) { cfg.value.types[i] = oldType; return }
  if (v === oldType) return
  if (cfg.value.types.some((x, j) => j !== i && x === v)) {
    MessagePlugin.warning('该分类已存在')
    cfg.value.types[i] = oldType
    return
  }
  cfg.value.type_rules.forEach(r => { if (r.type === oldType) r.type = v })
  cfg.value.measures.forEach(m => { if (m.type === oldType) m.type = v })
}

const addMeasure = () => {
  const name = (newMeasure.value || '').trim()
  if (!name) return
  if (cfg.value.measures.some(m => m.name === name)) { MessagePlugin.warning('该措施已存在'); return }
  cfg.value.measures.push({ name, type: newMeasureType.value || '' })
  newMeasure.value = ''
  newMeasureType.value = ''
}
const removeMeasure = (i: number) => {
  cfg.value.measures.splice(i, 1)
}

const onSave = async () => {
  if (!props.kbId) return
  saving.value = true
  try {
    const payload: RecognitionConfig = {
      enabled: cfg.value.enabled,
      include_rules: cfg.value.include_rules.filter(r => (r.match_type === 'regex' ? r.regex : (r.keywords?.length || 0) > 0) || r.name),
      type_rules: cfg.value.type_rules
        .filter(r => r.pattern && r.type)
        .map(r => ({ ...r, priority: Number(r.priority) > 0 ? Number(r.priority) : 1 })),
      types: (cfg.value.types || []).filter(Boolean),
      measures: (cfg.value.measures || []).filter(m => m.name).map(m => ({ name: m.name, type: m.type || '' })),
    }
    await saveRecognitionConfig(props.kbId, payload as any)
    MessagePlugin.success('已保存')
    emit('changed')
  } catch (e: any) {
    const msg = e?.response?.data?.error?.message || '保存失败，请检查规则'
    MessagePlugin.error(String(msg))
  } finally {
    saving.value = false
  }
}

const onReassess = async () => {
  if (!props.kbId) return
  reassessing.value = true
  try {
    const res: any = await reassessRecognition(props.kbId)
    const n = res?.data?.restored || 0
    if (n > 0) {
      MessagePlugin.success(`恢复 ${n} 条`)
      emit('changed')
    } else {
      MessagePlugin.info('删除历史中无命中规则')
    }
  } catch {
    MessagePlugin.error('重新评估失败')
  } finally {
    reassessing.value = false
  }
}

// ---- 抽屉宽度拖动（复刻删除历史抽屉） ----
const DRAWER_MIN_WIDTH = 620
const DRAWER_MAX_WIDTH = 1200
const DRAWER_WIDTH_KEY = 'weknora-recognition-drawer-width'
const drawerWidth = ref(760)
let resizeStartX = 0
let resizeStartWidth = 0

function drawerMaxWidth() { return Math.min(DRAWER_MAX_WIDTH, Math.max(DRAWER_MIN_WIDTH, Math.floor(window.innerWidth * 0.95))) }
function clampDrawerWidth(w: number) { return Math.max(DRAWER_MIN_WIDTH, Math.min(drawerMaxWidth(), w)) }
function loadDrawerWidth() {
  try {
    const raw = localStorage.getItem(DRAWER_WIDTH_KEY)
    const parsed = raw ? parseInt(raw, 10) : NaN
    if (!Number.isNaN(parsed)) drawerWidth.value = clampDrawerWidth(parsed)
  } catch { /* ignore */ }
}
function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartWidth = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = 'col-resize'
  document.body.style.userSelect = 'none'
}
function onResizeMove(e: MouseEvent) {
  const delta = resizeStartX - e.clientX
  drawerWidth.value = clampDrawerWidth(resizeStartWidth + delta)
}
function onResizeEnd() {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
  document.body.style.cursor = ''
  document.body.style.userSelect = ''
  try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
}
loadDrawerWidth()
onBeforeUnmount(() => {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
})
</script>

<style scoped>
.rr-body { padding: 4px 2px 24px; display: flex; flex-direction: column; gap: 16px; }
.rr-hint { font-size: 12px; color: var(--td-text-color-secondary, #666); line-height: 1.6; }
.rr-section { display: flex; flex-direction: column; gap: 0; }
.rr-section-head { display: flex; align-items: center; gap: 8px; padding: 8px 0 6px; border-bottom: 1px solid var(--td-component-border, #e7e7e7); }
.rr-section-title { font-size: 13px; font-weight: 600; }
.rr-state { font-size: 12px; color: var(--td-text-color-secondary, #666); }
.rr-spacer { flex: 1; }
.rr-empty { font-size: 12px; color: var(--td-text-color-placeholder, #999); padding: 10px 2px; }
.rr-row { display: flex; align-items: center; gap: 8px; padding: 8px 0; border-bottom: 1px dashed var(--td-component-border, #eee); }
.rr-row--last { border-bottom: none; }
.rr-row-main { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0; flex-wrap: wrap; }
.rr-row-side { display: flex; align-items: center; gap: 4px; }
.rr-name { width: 140px; }
.rr-match { width: 96px; }
.rr-keywords { flex: 1; min-width: 160px; }
.rr-logic { width: 110px; }
.rr-del { color: var(--td-error-color, #d54941); }
.rr-pattern { flex: 1; min-width: 140px; }
.rr-regex-check { margin-right: 2px; }
.rr-type-val { width: 130px; }
.rr-priority { width: 76px; }
.rr-regex-tip { color: var(--td-text-color-secondary, #666); }
.rr-regex-tip-box { padding: 8px 10px; background: var(--td-bg-color-container-hover, #f5f5f5); border-radius: 6px; display: flex; flex-direction: column; gap: 4px; margin: 6px 0 2px; }
.rr-regex-item { display: flex; align-items: center; gap: 10px; font-size: 12px; }
.rr-regex-code { font-family: Consolas, Monaco, monospace; color: var(--td-brand-color, #0052d9); background: var(--td-bg-color-container, #fff); padding: 1px 6px; border-radius: 3px; }
.rr-regex-desc { color: var(--td-text-color-secondary, #666); }
.rr-type-list { display: flex; flex-wrap: wrap; gap: 8px; padding: 10px 0 4px; }
.rr-type-chip { display: flex; align-items: center; gap: 2px; border: 1px solid var(--td-component-border, #e7e7e7); border-radius: 6px; padding: 2px 2px 2px 8px; background: var(--td-bg-color-container, #fff); }
.rr-type-name { width: 110px; }
.rr-measure-type { width: 96px; }
.rr-type-chip .rr-del { width: 24px; }
.rr-type-add { border-style: dashed; }
.rr-actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 4px; }

/* 抽屉右侧拖动把手（复刻删除历史抽屉） */
.rr-resize-handle {
  position: fixed; top: 0; bottom: 0; width: 8px; cursor: col-resize; z-index: 3100;
  display: flex; align-items: center; justify-content: center;
}
.rr-resize-line {
  width: 3px; height: 42px; border-radius: 2px;
  background: var(--td-component-border, #e7e7e7); opacity: 0; transition: opacity .2s;
}
.rr-resize-handle:hover .rr-resize-line { opacity: 1; background: var(--td-brand-color, #0052d9); }
</style>
