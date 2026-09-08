<template>
  <div class="us-wrap">
    <div v-if="visible" class="us-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer :visible="visible" :header="`设置 · ${title}`" :size="String(drawerWidth)" :footer="false"
      destroy-on-close class="utility-settings-drawer" @close="onClose">
      <div class="us-body">
        <!-- 字段配置 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">字段配置</span>
            <span class="us-state">列表与编辑表单按此加载</span>
            <span class="us-spacer" />
            <t-button variant="outline" size="small" @click="addField">
              <template #icon><t-icon name="add" size="14px" /></template>
              新增字段
            </t-button>
          </div>
          <div class="us-hint">默认显示控制列表列显隐；修改保存后列表字段自动刷新。</div>
          <div class="us-field-head">
            <span style="flex: 1.4">字段名</span>
            <span style="flex: 0.9">类型</span>
            <span style="flex: 0.7">默认显示</span>
            <span style="flex: 0.8">排序</span>
            <span style="flex: 0.5">操作</span>
          </div>
          <div v-for="(f, i) in fields" :key="f.field_key" class="us-field-row">
            <t-input v-model="f.label" size="small" class="us-field-label" placeholder="字段名" />
            <t-select v-model="f.field_type" size="small" class="us-field-type" :options="FIELD_TYPE_OPTS" />
            <t-switch v-model="f.default_visible" size="small" class="us-field-visible" />
            <div class="us-field-order">
              <t-button variant="text" size="small" shape="square" :disabled="i === 0" @click="moveField(i, -1)">
                <template #icon><t-icon name="arrow-up" size="14px" /></template>
              </t-button>
              <t-button variant="text" size="small" shape="square" :disabled="i === fields.length - 1" @click="moveField(i, 1)">
                <template #icon><t-icon name="arrow-down" size="14px" /></template>
              </t-button>
            </div>
            <div class="us-field-del">
              <t-button v-if="f.is_custom" variant="text" size="small" shape="square" @click="removeField(i)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
          <div v-if="!fields.length" class="us-empty">暂无字段配置</div>
        </div>

        <!-- 包含判定 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">包含判定</span>
            <t-switch v-model="cfg.enabled" size="small" />
            <span class="us-state">{{ cfg.enabled ? '生效' : '停用' }}</span>
            <span class="us-spacer" />
            <t-button variant="outline" size="small" @click="addIncludeRule">
              <template #icon><t-icon name="add" size="14px" /></template>
              添加
            </t-button>
          </div>
          <div class="us-hint">模型判定非账单但规则命中 → 认定为账单，置待补录。默认关键词：电费账单、电量、电费、千瓦时。</div>
          <div v-if="!cfg.include_rules.length" class="us-empty">暂无规则，模型判定为准</div>
          <div v-for="(r, i) in cfg.include_rules" :key="r.id" class="us-row">
            <div class="us-row-main">
              <t-input v-model="r.name" placeholder="规则名" size="small" class="us-name" />
              <t-select v-model="r.match_type" size="small" class="us-match" :options="MATCH_TYPE_OPTS" />
              <t-textarea v-if="r.match_type === 'keyword'" v-model="keywordsText[i]" placeholder="关键词，逗号分隔"
                :autosize="{ minRows: 1, maxRows: 2 }" size="small" class="us-keywords" @change="syncKeywords(i)" />
              <t-input v-else v-model="r.regex" placeholder="正则" size="small" class="us-keywords" />
              <t-select v-if="r.match_type === 'keyword'" v-model="r.logic" size="small" class="us-logic" :options="LOGIC_OPTS" />
            </div>
            <div class="us-row-side">
              <t-switch v-model="r.enabled" size="small" />
              <t-button variant="text" size="small" shape="square" @click="cfg.include_rules.splice(i, 1)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
        </div>

        <div class="us-actions">
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
import {
  listUtilityFieldConfigs,
  saveUtilityFieldConfigs,
  getRecognitionConfig,
  saveRecognitionConfig,
} from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  category: 'electricity' | 'water' | 'gas'
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const CATEGORY_TITLE: Record<string, string> = { electricity: '电费', water: '水费', gas: '气费' }
const title = computed(() => CATEGORY_TITLE[props.category] || '水电气')

const FIELD_TYPE_OPTS = [
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '金额', value: 'amount' },
  { label: '日期', value: 'date' },
]
const MATCH_TYPE_OPTS = [
  { label: '关键词', value: 'keyword' },
  { label: '正则', value: 'regex' },
]
const LOGIC_OPTS = [
  { label: '全部（AND）', value: 'AND' },
  { label: '任一（OR）', value: 'OR' },
]

interface FieldItem { field_key: string; label: string; field_type: string; default_visible: boolean; sort_order: number; is_custom: boolean }
interface IncludeRule { id: string; name: string; match_type: 'keyword' | 'regex'; keywords?: string[]; logic?: 'AND' | 'OR'; regex?: string; enabled: boolean }

const fields = ref<FieldItem[]>([])
const cfg = ref<{ enabled: boolean; include_rules: IncludeRule[] }>({ enabled: true, include_rules: [] })
const keywordsText = ref<string[]>([])
const saving = ref(false)

const uid = () => `r-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`

// 抽屉宽度拖动
const DRAWER_WIDTH_KEY = 'weknora-utility-settings-drawer-width'
const drawerWidth = ref(loadWidth())
function loadWidth(): number {
  try { return Number(localStorage.getItem(DRAWER_WIDTH_KEY)) || 760 } catch { return 760 }
}
let resizing = false
let startX = 0
let startW = 760
const onResizeStart = (e: MouseEvent) => {
  resizing = true
  startX = e.clientX
  startW = drawerWidth.value
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
const onResizeMove = (e: MouseEvent) => {
  if (!resizing) return
  drawerWidth.value = Math.min(1100, Math.max(560, startW + (startX - e.clientX)))
}
const onResizeEnd = () => {
  if (!resizing) return
  resizing = false
  try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}
onBeforeUnmount(() => {
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
})

const syncKeywords = (i: number) => {
  const t = keywordsText.value[i] || ''
  cfg.value.include_rules[i].keywords = t.split(/[,，\n]/).map(s => s.trim()).filter(Boolean)
}
const addIncludeRule = () => {
  cfg.value.include_rules.push({ id: uid(), name: '', match_type: 'keyword', keywords: [], logic: 'OR', regex: '', enabled: true })
  keywordsText.value.push('')
}

const addField = () => {
  let n = 1
  const keys = new Set(fields.value.map(f => f.field_key))
  while (keys.has(`custom_${n}`)) n++
  fields.value.push({
    field_key: `custom_${n}`, label: '自定义字段', field_type: 'text',
    default_visible: false, sort_order: fields.value.length, is_custom: true,
  })
}
const removeField = (i: number) => {
  fields.value.splice(i, 1)
  refreshOrder()
}
const moveField = (i: number, dir: number) => {
  const j = i + dir
  if (j < 0 || j >= fields.value.length) return
  const tmp = fields.value[i]
  fields.value[i] = fields.value[j]
  fields.value[j] = tmp
  refreshOrder()
}
const refreshOrder = () => {
  fields.value.forEach((f, i) => { f.sort_order = i })
}

const load = async () => {
  try {
    const res: any = await listUtilityFieldConfigs(props.category)
    const list = res?.data || res
    fields.value = (Array.isArray(list) ? list : []).map((c: any) => ({
      field_key: c.field_key, label: c.label, field_type: c.field_type || 'text',
      default_visible: !!c.default_visible, sort_order: Number(c.sort_order) || 0, is_custom: !!c.is_custom,
    }))
    // 内置字段（非 custom）不允许删除
  } catch {
    fields.value = []
  }
  try {
    const res: any = await getRecognitionConfig(props.kbId)
    const c = res?.data || res
    cfg.value = {
      enabled: !!c?.enabled,
      include_rules: (c?.include_rules || []).map((r: any, i: number) => {
        const keywords = Array.isArray(r.keywords) ? r.keywords.filter(Boolean) : []
        keywordsText.value[i] = keywords.join('，')
        return {
          id: r.id || uid(), name: r.name || '', match_type: r.match_type === 'regex' ? 'regex' : 'keyword',
          keywords: keywords.length ? keywords : undefined, logic: r.logic === 'AND' ? 'AND' : 'OR',
          regex: r.regex || '', enabled: r.enabled !== false,
        }
      }),
    }
  } catch {
    cfg.value = { enabled: true, include_rules: [] }
    keywordsText.value = []
  }
}

watch(() => props.visible, (v) => { if (v) load() })

const onClose = () => emit('update:visible', false)

const onSave = async () => {
  if (fields.value.some(f => !f.label.trim())) {
    MessagePlugin.warning('字段名不能为空')
    return
  }
  saving.value = true
  try {
    await saveUtilityFieldConfigs(props.category, fields.value.map((f, i) => ({
      field_key: f.field_key, label: f.label.trim(), field_type: f.field_type,
      default_visible: !!f.default_visible, sort_order: i, is_custom: !!f.is_custom,
    })))
    await saveRecognitionConfig(props.kbId, {
      enabled: cfg.value.enabled,
      include_rules: cfg.value.include_rules.map(r => ({
        id: r.id, name: r.name, match_type: r.match_type,
        keywords: r.match_type === 'keyword' ? (r.keywords || []) : undefined,
        logic: r.logic, regex: r.regex, enabled: r.enabled,
      })),
    })
    MessagePlugin.success('设置已保存')
    emit('changed')
    onClose()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<style lang="less" scoped>
.us-wrap { position: relative; }
.us-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  z-index: 3100;
  width: 8px;
  cursor: col-resize;

  .us-resize-line {
    position: absolute;
    top: 0;
    bottom: 0;
    left: 3px;
    width: 2px;
    background: var(--td-brand-color);
    opacity: 0;
    transition: opacity .2s;
  }

  &:hover .us-resize-line {
    opacity: .6;
  }
}

.us-body {
  padding: 4px 0 24px;
}

.us-section {
  margin-bottom: 28px;
}

.us-section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;

  .us-section-title {
    font-size: 15px;
    font-weight: 600;
    color: var(--td-text-color-primary);
  }

  .us-state {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }

  .us-spacer {
    flex: 1;
  }
}

.us-hint {
  font-size: 12px;
  color: var(--td-text-color-secondary);
  margin-bottom: 10px;
  line-height: 1.5;
}

.us-empty {
  padding: 16px;
  text-align: center;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  border: 1px dashed var(--td-component-stroke);
  border-radius: 6px;
}

.us-field-head,
.us-field-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 4px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.us-field-head {
  border-bottom: 1px solid var(--td-component-stroke);
  margin-bottom: 4px;
}

.us-field-row {
  .us-field-label { flex: 1.4; }
  .us-field-type { flex: 0.9; }
  .us-field-visible { flex: 0.7; justify-content: flex-start; }
  .us-field-order { flex: 0.8; display: flex; gap: 2px; }
  .us-field-del { flex: 0.5; }
}

.us-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child { border-bottom: none; }

  .us-row-main {
    flex: 1;
    display: flex;
    gap: 6px;
    align-items: center;

    .us-name { width: 110px; }
    .us-match { width: 90px; }
    .us-keywords { flex: 1; }
    .us-logic { width: 100px; }
  }

  .us-row-side {
    display: flex;
    gap: 4px;
    align-items: center;
  }
}

.us-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}
</style>
