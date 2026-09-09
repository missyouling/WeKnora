<template>
  <div class="us-wrap">
    <div v-if="visible" class="us-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" :header="`设置 · ${title}`" :size="`${drawerWidth}px`" :footer="false"
      class="utility-settings-drawer" @close="onClose"
      @update:visible="(v: boolean) => (v || onClose())">
      <div class="us-body">
        <!-- 基本户信息（仅电费） -->
        <div v-if="props.category === 'electricity'" class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">基本户信息</span>
            <span class="us-state">账单概览基础信息直接读取</span>
          </div>
          <div class="us-basic-grid">
            <div class="us-basic-item">
              <label>户号</label>
              <t-input v-model="basicInfo.account_no" size="small" placeholder="户号" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item">
              <label>户名</label>
              <t-input v-model="basicInfo.account_name" size="small" placeholder="户名" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item">
              <label>用电类别</label>
              <t-input v-model="basicInfo.usage_category" size="small" placeholder="用电类别" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item">
              <label>电压等级</label>
              <t-input v-model="basicInfo.voltage_level" size="small" placeholder="电压等级" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item">
              <label>市场化属性</label>
              <t-input v-model="basicInfo.market_attr" size="small" placeholder="市场化属性" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item">
              <label>供电服务单位</label>
              <t-input v-model="basicInfo.supply_unit" size="small" placeholder="供电服务单位" @change="autosaveBasicInfo" />
            </div>
            <div class="us-basic-item us-basic-item--wide">
              <label>用电地址</label>
              <t-input v-model="basicInfo.address" size="small" placeholder="用电地址" @change="autosaveBasicInfo" />
            </div>
          </div>
        </div>

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
          <div class="us-hint">默认显示控制列表列显隐；输入完成后失焦自动保存。</div>
          <div class="us-field-head">
            <span style="flex: 1.4">字段名</span>
            <span style="flex: 0.9">类型</span>
            <span style="flex: 0.7">默认显示</span>
            <span style="flex: 0.8">排序</span>
            <span style="flex: 0.5">操作</span>
          </div>
          <div v-for="(f, i) in fields" :key="f.field_key" class="us-field-row">
            <t-input v-model="f.label" size="small" class="us-field-label" placeholder="字段名" @change="scheduleSave" />
            <t-select v-model="f.field_type" size="small" class="us-field-type" :options="FIELD_TYPE_OPTS" @change="scheduleSave" />
            <t-switch v-model="f.default_visible" size="small" class="us-field-visible" @change="scheduleSave" />
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

        <!-- 分时电价（仅电费） -->
        <div v-if="props.category === 'electricity'" class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">分时电价</span>
            <span class="us-state">零售交易电费按规则单价计算</span>
            <span class="us-spacer" />
            <t-button variant="outline" size="small" @click="addTariff">
              <template #icon><t-icon name="add" size="14px" /></template>
              新增规则
            </t-button>
          </div>
          <div class="us-hint">按月份设定尖峰平谷单价，零售交易电费 = 时段电量 × 对应单价。不同月份可不同计价，如 7、8 月尖峰与峰分开计价，其它月份尖峰与峰同价（填相同值）；无匹配月份时使用默认规则。</div>
          <div v-if="!tariffs.length" class="us-empty">暂无分时电价规则，零售交易电费按账单提取标准计算</div>
          <div v-for="(t, i) in tariffs" :key="t.id" class="us-tariff-row">
            <div class="us-tariff-main">
              <t-input v-model="t.name" placeholder="规则名" size="small" class="us-tariff-name" @change="scheduleSave" />
              <t-select v-model="t.monthsArr" multiple size="small" class="us-tariff-months" :options="MONTH_OPTS"
                placeholder="适用月份" @change="syncTariffMonths(i)" />
              <div class="us-tariff-rates">
                <label>尖</label><t-input v-model="t.deep_peak_rate" type="number" size="small" class="us-rate-input" @change="scheduleSave" />
                <label>峰</label><t-input v-model="t.peak_rate" type="number" size="small" class="us-rate-input" @change="scheduleSave" />
                <label>平</label><t-input v-model="t.flat_rate" type="number" size="small" class="us-rate-input" @change="scheduleSave" />
                <label>谷</label><t-input v-model="t.valley_rate" type="number" size="small" class="us-rate-input" @change="scheduleSave" />
              </div>
            </div>
            <div class="us-tariff-side">
              <t-tooltip content="无匹配月份时兜底">
                <t-switch v-model="t.is_default" size="small" @change="scheduleSave" />
              </t-tooltip>
              <t-button variant="text" size="small" shape="square" @click="removeTariff(i)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
        </div>

        <!-- 包含判定 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">包含判定</span>
            <t-switch v-model="cfg.enabled" size="small" @change="scheduleSave" />
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
              <t-input v-model="r.name" placeholder="规则名" size="small" class="us-name" @change="scheduleSave" />
              <t-select v-model="r.match_type" size="small" class="us-match" :options="MATCH_TYPE_OPTS" @change="scheduleSave" />
              <t-textarea v-if="r.match_type === 'keyword'" v-model="keywordsText[i]" placeholder="关键词，逗号分隔"
                :autosize="{ minRows: 1, maxRows: 2 }" size="small" class="us-keywords" @change="syncKeywords(i)" />
              <t-input v-else v-model="r.regex" placeholder="正则" size="small" class="us-keywords" @change="scheduleSave" />
              <t-select v-if="r.match_type === 'keyword'" v-model="r.logic" size="small" class="us-logic" :options="LOGIC_OPTS" @change="scheduleSave" />
            </div>
            <div class="us-row-side">
              <t-switch v-model="r.enabled" size="small" @change="scheduleSave" />
              <t-button variant="text" size="small" shape="square" @click="cfg.include_rules.splice(i, 1); scheduleSave()">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
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
  listUtilityTariffRules,
  createUtilityTariffRule,
  updateUtilityTariffRule,
  getUtilityBasicInfo,
  saveUtilityBasicInfo,
  deleteUtilityTariffRule,
} from '@/api/knowledge-base'

const props = defineProps<{
  visible: boolean
  kbId: string
  category: 'electricity' | 'water' | 'gas'
}>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const CATEGORY_TITLE: Record<string, string> = { electricity: '能耗', water: '水费', gas: '气费' }
const title = computed(() => CATEGORY_TITLE[props.category] || '能耗')

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
const MONTH_OPTS = Array.from({ length: 12 }, (_, i) => ({ label: `${i + 1}月`, value: String(i + 1) }))

interface FieldItem { field_key: string; label: string; field_type: string; default_visible: boolean; sort_order: number; is_custom: boolean }
interface IncludeRule { id: string; name: string; match_type: 'keyword' | 'regex'; keywords?: string[]; logic?: 'AND' | 'OR'; regex?: string; enabled: boolean }
interface TariffRule {
  id: string
  name: string
  months: string
  monthsArr: string[]
  deep_peak_rate: string | number
  peak_rate: string | number
  flat_rate: string | number
  valley_rate: string | number
  is_default: boolean
  sort_order: number
  _new?: boolean
}

const fields = ref<FieldItem[]>([])
const cfg = ref<{ enabled: boolean; include_rules: IncludeRule[] }>({ enabled: true, include_rules: [] })
const tariffs = ref<TariffRule[]>([])
const keywordsText = ref<string[]>([])
const saving = ref(false)
// 基本户信息（电费）
const basicInfo = ref({
  account_no: '', account_name: '', usage_category: '', voltage_level: '',
  market_attr: '', supply_unit: '', address: '',
})

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
  scheduleSave()
}
const addIncludeRule = () => {
  cfg.value.include_rules.push({ id: uid(), name: '', match_type: 'keyword', keywords: [], logic: 'OR', regex: '', enabled: true })
  keywordsText.value.push('')
  scheduleSave()
}

const syncTariffMonths = (i: number) => {
  tariffs.value[i].months = (tariffs.value[i].monthsArr || []).join(',')
  scheduleSave()
}
const addTariff = () => {
  tariffs.value.push({
    id: uid(), name: '', months: '', monthsArr: [], deep_peak_rate: '', peak_rate: '', flat_rate: '', valley_rate: '',
    is_default: tariffs.value.length === 0, sort_order: tariffs.value.length, _new: true,
  })
  scheduleSave()
}
const removeTariff = (i: number) => {
  const t = tariffs.value[i]
  if (!t._new && t.id) {
    deleteUtilityTariffRule(t.id, 'electricity').catch(() => { /* 保存流程兜底 */ })
  }
  tariffs.value.splice(i, 1)
  scheduleSave()
}

const addField = () => {
  let n = 1
  const keys = new Set(fields.value.map(f => f.field_key))
  while (keys.has(`custom_${n}`)) n++
  fields.value.push({
    field_key: `custom_${n}`, label: '自定义字段', field_type: 'text',
    default_visible: false, sort_order: fields.value.length, is_custom: true,
  })
  scheduleSave()
}
const removeField = (i: number) => {
  fields.value.splice(i, 1)
  refreshOrder()
  scheduleSave()
}
const moveField = (i: number, dir: number) => {
  const j = i + dir
  if (j < 0 || j >= fields.value.length) return
  const tmp = fields.value[i]
  fields.value[i] = fields.value[j]
  fields.value[j] = tmp
  refreshOrder()
  scheduleSave()
}
const refreshOrder = () => {
  fields.value.forEach((f, i) => { f.sort_order = i })
}

// 电费内置字段（后端配置缺失时合并补全，保证新默认字段可在设置中管理）
const BUILTIN_ELECTRICITY_FIELDS: { key: string; label: string; type: string; default: boolean }[] = [
  { key: 'bill_period', label: '账单周期', type: 'text', default: true },
  { key: 'total_kwh', label: '本期电量', type: 'number', default: true },
  { key: 'total_amount', label: '本期电费', type: 'number', default: true },
  { key: 'pf_adjust_amount', label: '力调电费', type: 'number', default: true },
  { key: 'capacity_fee', label: '基本电费', type: 'number', default: true },
  { key: 'market_amount', label: '购电电费', type: 'number', default: true },
  { key: 'line_amount', label: '线损费用', type: 'number', default: true },
  { key: 'trans_amount', label: '输配电费', type: 'number', default: true },
  { key: 'sys_amount', label: '系统运行费', type: 'number', default: true },
  { key: 'govI_amount', label: '附加费', type: 'number', default: true },
  { key: 'catalog_amount', label: '目录电费（居民）', type: 'number', default: true },
  { key: 'govR_amount', label: '附加费（居民）', type: 'number', default: true },
  { key: 'account_no', label: '户号', type: 'text', default: false },
  { key: 'account_name', label: '户名', type: 'text', default: false },
  { key: 'usage_category', label: '用电类别', type: 'text', default: false },
  { key: 'voltage_level', label: '电压等级', type: 'text', default: false },
  { key: 'avg_price', label: '平均电价', type: 'number', default: false },
  { key: 'power_factor', label: '功率因素', type: 'number', default: false },
]

const load = async () => {
  try {
    const res: any = await listUtilityFieldConfigs(props.category)
    const list = res?.data || res
    // 仅保留内置字段与自定义字段，过滤历史废弃字段；label/默认显隐以新内置集为准
    const builtinKeys = new Set(BUILTIN_ELECTRICITY_FIELDS.map(f => f.key))
    fields.value = (Array.isArray(list) ? list : [])
      .filter((c: any) => c.is_custom || builtinKeys.has(c.field_key))
      .map((c: any) => {
        const b = BUILTIN_ELECTRICITY_FIELDS.find(f => f.key === c.field_key)
        return {
          field_key: c.field_key,
          label: b ? b.label : c.label,
          field_type: c.field_type || 'text',
          default_visible: b ? b.default : !!c.default_visible,
          sort_order: Number(c.sort_order) || 0,
          is_custom: !!c.is_custom,
        }
      })
    // 电费：合并后端缺失的内置字段，保证新默认字段可见可配
    if (props.category === 'electricity') {
      const keys = new Set(fields.value.map(f => f.field_key))
      const missing = BUILTIN_ELECTRICITY_FIELDS.filter(f => !keys.has(f.key))
      if (missing.length) {
        const maxOrder = fields.value.reduce((m, f) => Math.max(m, f.sort_order || 0), 0)
        fields.value.push(...missing.map((f, i) => ({
          field_key: f.key, label: f.label, field_type: f.type,
          default_visible: f.default, sort_order: maxOrder + i + 1, is_custom: false,
        })))
      }
    }
  } catch (e: any) {
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
  if (props.category === 'electricity') {
    try {
      const res: any = await listUtilityTariffRules('electricity')
      const list = res?.data || res
      tariffs.value = (Array.isArray(list) ? list : []).map((t: any) => ({
        id: t.id, name: t.name || '', months: t.months || '', monthsArr: (t.months || '').split(',').filter(Boolean),
        deep_peak_rate: t.deep_peak_rate ?? '', peak_rate: t.peak_rate ?? '', flat_rate: t.flat_rate ?? '',
        valley_rate: t.valley_rate ?? '', is_default: !!t.is_default, sort_order: Number(t.sort_order) || 0,
      }))
    } catch {
      tariffs.value = []
    }
  } else {
    tariffs.value = []
  }
  await loadBasicInfo()
}

watch(() => props.visible, (v) => { if (v) load() })

// ---- 基本户信息：加载 + 防抖自动保存 ----
const loadBasicInfo = async () => {
  if (props.category !== 'electricity') return
  try {
    const res: any = await getUtilityBasicInfo('electricity')
    const d = res?.data || {}
    Object.assign(basicInfo.value, {
      account_no: d.account_no || '', account_name: d.account_name || '',
      usage_category: d.usage_category || '', voltage_level: d.voltage_level || '',
      market_attr: d.market_attr || '', supply_unit: d.supply_unit || '', address: d.address || '',
    })
  } catch { /* 未配置时保持空 */ }
}
let basicInfoTimer: ReturnType<typeof setTimeout> | undefined
const autosaveBasicInfo = () => {
  clearTimeout(basicInfoTimer)
  basicInfoTimer = setTimeout(async () => {
    try {
      await saveUtilityBasicInfo('electricity', { ...basicInfo.value })
      MessagePlugin.success('基本户信息已保存')
    } catch (e: any) {
      MessagePlugin.error(e?.message || '保存失败')
    }
  }, 800)
}

const onClose = () => emit('update:visible', false)

// ---- 自动保存：字段/规则/电价变更防抖持久化 ----
let saveTimer: ReturnType<typeof setTimeout> | undefined
const scheduleSave = () => {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => { persistAll() }, 800)
}
const persistAll = async () => {
  if (saving.value) return
  if (fields.value.some(f => !f.label.trim())) return
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
    if (props.category === 'electricity') {
      const toNum = (v: string | number) => { const n = Number(v); return Number.isFinite(n) ? n : 0 }
      for (const t of tariffs.value) {
        const payload: Record<string, unknown> = {
          name: t.name || '分时电价规则', months: t.months, deep_peak_rate: toNum(t.deep_peak_rate),
          peak_rate: toNum(t.peak_rate), flat_rate: toNum(t.flat_rate), valley_rate: toNum(t.valley_rate),
          is_default: !!t.is_default, sort_order: t.sort_order,
        }
        if (t._new || !t.id) {
          await createUtilityTariffRule('electricity', payload)
        } else {
          await updateUtilityTariffRule(t.id, 'electricity', payload)
        }
      }
    }
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 输入/改动通过 @change 触发 scheduleSave；关闭抽屉前落盘最后改动
watch(() => props.visible, (v) => {
  if (!v) {
    clearTimeout(saveTimer)
    persistAll()
  }
})
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

.us-tariff-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 0;
  border-bottom: 1px solid var(--td-component-stroke);

  &:last-child { border-bottom: none; }

  .us-tariff-main {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 8px;

    .us-tariff-name { width: 110px; }
    .us-tariff-months { width: 170px; }

    .us-tariff-rates {
      flex: 1;
      display: flex;
      align-items: center;
      gap: 6px;

      label {
        font-size: 12px;
        color: var(--td-text-color-secondary);
      }

      .us-rate-input { width: 86px; }
    }
  }

  .us-tariff-side {
    display: flex;
    align-items: center;
    gap: 6px;
  }
}

.us-actions {
  display: flex;
  justify-content: flex-end;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
}

/* 基本户信息 */
.us-basic-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 14px;
}

.us-basic-item {
  display: flex;
  flex-direction: column;
  gap: 4px;

  label {
    font-size: 12px;
    color: var(--td-text-color-secondary);
  }
}

.us-basic-item--wide {
  grid-column: 1 / -1;
}
</style>
