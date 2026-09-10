<template>
  <div class="us-wrap">
    <div v-if="visible" class="us-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" :header="`设置 · ${title}`" :size="`${drawerWidth}px`" :footer="false"
      class="utility-settings-drawer" :close-on-overlay-click="true" @close="onClose"
      @update:visible="(v: boolean) => (v || onClose())">
      <div class="us-body">
        <!-- 基本户（仅电费） -->
        <div v-if="props.category === 'electricity'" class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">基本户</span>
            <span class="us-state">账单按户号自动匹配，匹配失败用默认户</span>
            <span class="us-spacer" />
            <t-button variant="outline" size="small" @click="openAccEdit(null)">
              <template #icon><t-icon name="add" size="14px" /></template>
              新增基本户
            </t-button>
          </div>
          <div v-if="!accounts.length" class="us-empty">暂无基本户，点击新增</div>
          <div class="us-card-grid">
            <div v-for="a in accounts" :key="a.id" class="us-card" @click="openAccEdit(a)">
              <div class="us-card-head">
                <span class="us-card-title">{{ a.name || '未命名户' }}</span>
                <span v-if="a.is_default" class="us-card-tag">默认</span>
              </div>
              <div class="us-card-line">户号：{{ a.account_no || '—' }}</div>
              <div class="us-card-line">电能表：{{ a.meter_no || '—' }}</div>
              <div class="us-card-line">倍率：{{ a.ratio ? a.ratio : '—' }}</div>
            </div>
          </div>
        </div>

        <!-- 字段配置（按分组卡片） -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">字段配置</span>
            <span class="us-state">分组管理各菜单列字段，保存后自动同步</span>
          </div>
          <div class="us-hint">点击分组卡片管理字段：新增、删除、排序、默认显示；删除字段不影响历史记录数据。</div>
          <div class="us-card-grid">
            <div v-for="g in groups" :key="g.key" class="us-card" @click="openGroupEdit(g.key)">
              <div class="us-card-head">
                <span class="us-card-title">{{ g.label }}</span>
              </div>
              <div class="us-card-line">{{ groupFieldCount(g.key) }} 个字段</div>
              <div class="us-card-line us-card-action">点击管理</div>
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
          <div class="us-hint">模型判定非账单但规则命中 → 认定为账单，置待补录。</div>
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

    <!-- 基本户编辑抽屉 -->
    <t-drawer v-if="accEditVisible" :visible="true" :header="accForm.id ? '编辑基本户' : '新增基本户'" :size="'480px'" :footer="false"
      :close-on-overlay-click="true" @close="accEditVisible = false" @update:visible="(v: boolean) => (v || (accEditVisible = false))">
      <div class="us-basic-grid">
        <div class="us-basic-item">
          <label>名称</label>
          <t-input v-model="accForm.name" size="small" placeholder="自定义名称" />
        </div>
        <div class="us-basic-item">
          <label>户号</label>
          <t-input v-model="accForm.account_no" size="small" placeholder="户号" />
        </div>
        <div class="us-basic-item">
          <label>户名</label>
          <t-input v-model="accForm.account_name" size="small" placeholder="户名" />
        </div>
        <div class="us-basic-item">
          <label>用电类别</label>
          <t-input v-model="accForm.usage_category" size="small" placeholder="用电类别" />
        </div>
        <div class="us-basic-item">
          <label>电压等级</label>
          <t-input v-model="accForm.voltage_level" size="small" placeholder="电压等级" />
        </div>
        <div class="us-basic-item">
          <label>市场化属性</label>
          <t-input v-model="accForm.market_attr" size="small" placeholder="市场化属性" />
        </div>
        <div class="us-basic-item">
          <label>供电服务单位</label>
          <t-input v-model="accForm.supply_unit" size="small" placeholder="供电服务单位" />
        </div>
        <div class="us-basic-item">
          <label>电能表编号</label>
          <t-input v-model="accForm.meter_no" size="small" placeholder="电能表编号" />
        </div>
        <div class="us-basic-item">
          <label>倍率</label>
          <t-input v-model="accForm.ratio" type="number" size="small" placeholder="倍率" />
        </div>
        <div class="us-basic-item us-basic-item--wide">
          <label>用电地址</label>
          <t-input v-model="accForm.address" size="small" placeholder="用电地址" />
        </div>
      </div>
      <div class="us-basic-foot">
        <label class="us-default-label">
          <t-checkbox v-model="accForm.is_default" size="small">设为默认户</t-checkbox>
        </label>
        <t-button v-if="accForm.id" variant="text" theme="danger" size="small" @click="confirmDeleteAcc">
          <template #icon><t-icon name="delete" size="15px" /></template>
          删除
        </t-button>
      </div>
      <div class="us-actions">
        <t-button variant="outline" size="small" @click="accEditVisible = false">取消</t-button>
        <t-button size="small" @click="saveAcc">保存</t-button>
      </div>
    </t-drawer>

    <!-- 字段分组管理抽屉 -->
    <t-drawer v-if="groupEditVisible" :visible="true" :header="`字段 · ${groupEditLabel}`" :size="'520px'" :footer="false"
      :close-on-overlay-click="true" @close="groupEditVisible = false" @update:visible="(v: boolean) => (v || (groupEditVisible = false))">
      <div class="us-hint">列字段对应菜单列表的列；默认显示控制列显隐。行字段对应菜单列表的行标题，可改名、排序、增删，删除行仅隐藏不影响历史数据。</div>
      <div class="us-group-tabs">
        <span class="us-group-tab" :class="{ active: groupTab === 'cols' }" @click="groupTab = 'cols'">列字段配置</span>
        <span v-if="hasRowGroup" class="us-group-tab" :class="{ active: groupTab === 'rows' }" @click="groupTab = 'rows'">行字段配置</span>
      </div>
      <!-- 列字段配置 -->
      <template v-if="groupTab === 'cols'">
        <div class="us-cfg">
          <div class="us-cfg-head">
            <span class="us-col-label">字段名</span>
            <span class="us-col-type">类型</span>
            <span class="us-col-visible">默认显示</span>
            <span class="us-col-order">排序</span>
            <span class="us-col-del">操作</span>
          </div>
          <div v-for="(f, i) in groupFields" :key="f.field_key" class="us-cfg-row">
            <div class="us-cell us-col-label">
              <t-input v-model="f.label" size="small" class="us-cfg-input" placeholder="字段名" @change="saveGroupFields" />
            </div>
            <div class="us-cell us-col-type">
              <t-select v-model="f.field_type" size="small" class="us-cfg-select" :options="FIELD_TYPE_OPTS" @change="saveGroupFields" />
            </div>
            <div class="us-cell us-col-visible">
              <t-switch v-model="f.default_visible" size="small" class="us-cfg-switch" @change="saveGroupFields" />
            </div>
            <div class="us-cell us-col-order">
              <div class="us-cell-order">
                <t-button variant="text" size="small" shape="square" :disabled="i === 0" @click="moveGroupField(i, -1)">
                  <template #icon><t-icon name="arrow-up" size="14px" /></template>
                </t-button>
                <t-button variant="text" size="small" shape="square" :disabled="i === groupFields.length - 1" @click="moveGroupField(i, 1)">
                  <template #icon><t-icon name="arrow-down" size="14px" /></template>
                </t-button>
              </div>
            </div>
            <div class="us-cell us-col-del">
              <t-button variant="text" size="small" shape="square" @click="removeGroupField(i)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
          <div v-if="!groupFields.length" class="us-empty">暂无字段，点击下方新增</div>
        </div>
      </template>
      <!-- 行字段配置（与列字段同 5 列，类型固定文本） -->
      <template v-else>
        <div class="us-cfg">
          <div class="us-cfg-head">
            <span class="us-col-label">字段名</span>
            <span class="us-col-type">类型</span>
            <span class="us-col-visible">默认显示</span>
            <span class="us-col-order">排序</span>
            <span class="us-col-del">操作</span>
          </div>
          <div v-for="(f, i) in rowFields" :key="f.field_key" class="us-cfg-row">
            <div class="us-cell us-col-label">
              <t-input v-model="f.label" size="small" class="us-cfg-input" placeholder="行标题" @change="saveGroupFields" />
            </div>
            <div class="us-cell us-col-type"><span class="us-fixed-type">文本</span></div>
            <div class="us-cell us-col-visible">
              <t-switch v-model="f.default_visible" size="small" class="us-cfg-switch" @change="saveGroupFields" />
            </div>
            <div class="us-cell us-col-order">
              <div class="us-cell-order">
                <t-button variant="text" size="small" shape="square" :disabled="i === 0" @click="moveGroupField(i, -1)">
                  <template #icon><t-icon name="arrow-up" size="14px" /></template>
                </t-button>
                <t-button variant="text" size="small" shape="square" :disabled="i === rowFields.length - 1" @click="moveGroupField(i, 1)">
                  <template #icon><t-icon name="arrow-down" size="14px" /></template>
                </t-button>
              </div>
            </div>
            <div class="us-cell us-col-del">
              <t-button variant="text" size="small" shape="square" @click="removeGroupField(i)">
                <template #icon><t-icon name="delete" size="15px" /></template>
              </t-button>
            </div>
          </div>
          <div v-if="!rowFields.length" class="us-empty">暂无行，点击下方新增</div>
        </div>
      </template>
      <div class="us-actions">
        <t-button variant="outline" size="small" @click="addGroupField">
          <template #icon><t-icon name="add" size="14px" /></template>
          {{ groupTab === 'rows' ? '新增行' : '新增字段' }}
        </t-button>
        <t-button size="small" @click="groupEditVisible = false">完成</t-button>
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
  listUtilityFieldConfigsByGroup,
  getRecognitionConfig,
  saveRecognitionConfig,
  listUtilityBasicAccounts,
  createUtilityBasicAccount,
  updateUtilityBasicAccount,
  deleteUtilityBasicAccount,
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

interface FieldItem { field_key: string; label: string; field_type: string; default_visible: boolean; sort_order: number; is_custom: boolean }
interface IncludeRule { id: string; name: string; match_type: 'keyword' | 'regex'; keywords?: string[]; logic?: 'AND' | 'OR'; regex?: string; enabled: boolean }
interface BasicAccount { id?: string; name: string; account_no: string; account_name: string; usage_category: string; voltage_level: string; market_attr: string; supply_unit: string; address: string; meter_no: string; ratio: number | string; is_default: boolean }

// 电费字段配置分组（对应各费用菜单）
const GROUP_DEFS = [
  { key: 'overview', label: '账单概况' },
  { key: 'market', label: '市场化购电费' },
  { key: 'line', label: '上网环节线损费' },
  { key: 'trans', label: '输配电量电费' },
  { key: 'sys', label: '系统运行费' },
  { key: 'gov-industrial', label: '政府基金及附加（工商业）' },
  { key: 'catalog', label: '目录电费（居民）' },
  { key: 'gov-residential', label: '政府基金及附加（居民）' },
  { key: 'capacity', label: '输配容（需）量' },
  { key: 'pf', label: '功率因素调整' },
  { key: 'meter', label: '电量明细（工商业）' },
  { key: 'resident-meter', label: '电量明细（居民）' },
]
// 有行标题的列分组 → 行配置分组（行字段配置 tab 的数据来源）
const ROW_GROUP_MAP: Record<string, string> = {
  meter: 'meter-rows',
  'resident-meter': 'resident-meter-rows',
  overview: 'overview-rows',
  market: 'market-rows',
  trans: 'trans-rows',
  sys: 'sys-rows',
  'gov-industrial': 'gov-industrial-rows',
  catalog: 'catalog-rows',
  'gov-residential': 'gov-residential-rows',
}

const accounts = ref<BasicAccount[]>([])
const cfg = ref<{ enabled: boolean; include_rules: IncludeRule[] }>({ enabled: true, include_rules: [] })
const keywordsText = ref<string[]>([])
const saving = ref(false)

const groups = computed(() => {
  if (props.category === 'electricity') return GROUP_DEFS
  return [{ key: '', label: '通用字段' }]
})
const groupFieldCount = (key: string) => groupCounts.value[key] ?? 0
const groupCounts = ref<Record<string, number>>({})
const hasRowGroup = computed(() => !!ROW_GROUP_MAP[groupEditKey.value])

const uid = () => `r-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`

// 抽屉宽度拖动
const DRAWER_WIDTH_KEY = 'weknora-utility-settings-drawer-width'
const drawerWidth = ref(loadWidth())
function loadWidth(): number {
  try { return Number(localStorage.getItem(DRAWER_WIDTH_KEY)) || 820 } catch { return 820 }
}
let resizing = false
let startX = 0
let startW = 820
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

// ---- 基本户 ----
const loadAccounts = async () => {
  if (props.category !== 'electricity') return
  try {
    const res: any = await listUtilityBasicAccounts('electricity')
    accounts.value = (res?.data || res || []).map((a: any) => ({
      id: a.id, name: a.name || '', account_no: a.account_no || '', account_name: a.account_name || '',
      usage_category: a.usage_category || '', voltage_level: a.voltage_level || '',
      market_attr: a.market_attr || '', supply_unit: a.supply_unit || '', address: a.address || '',
      meter_no: a.meter_no || '', ratio: a.ratio ?? '', is_default: !!a.is_default,
    }))
  } catch { accounts.value = [] }
}
const emptyAcc = (): BasicAccount => ({
  name: '', account_no: '', account_name: '', usage_category: '', voltage_level: '',
  market_attr: '', supply_unit: '', address: '', meter_no: '', ratio: '', is_default: false,
})
const accEditVisible = ref(false)
const accForm = ref<BasicAccount>(emptyAcc())
const openAccEdit = (a: BasicAccount | null) => {
  accForm.value = a ? { ...a } : emptyAcc()
  accEditVisible.value = true
}
const saveAcc = async () => {
  const payload = { ...accForm.value, ratio: Number(accForm.value.ratio) || 0 }
  try {
    if (accForm.value.id) {
      await updateUtilityBasicAccount(accForm.value.id, 'electricity', payload)
    } else {
      await createUtilityBasicAccount('electricity', payload)
    }
    MessagePlugin.success('已保存')
    accEditVisible.value = false
    await loadAccounts()
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}
const confirmDeleteAcc = async () => {
  const ok = await MessagePlugin.confirm('删除该基本户？历史账单记录保留原数据。', {
    theme: 'warning', confirmBtn: '删除', cancelBtn: '取消',
  })
  if (!ok) return
  try {
    await deleteUtilityBasicAccount(accForm.value.id!, 'electricity')
    MessagePlugin.success('已删除')
    accEditVisible.value = false
    await loadAccounts()
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 字段分组管理 ----
const groupEditVisible = ref(false)
const groupEditLabel = ref('')
const groupEditKey = ref('')
const groupTab = ref<'cols' | 'rows'>('cols')
const groupFields = ref<FieldItem[]>([])
const rowFields = ref<FieldItem[]>([])
const toFieldItem = (c: any): FieldItem => ({
  field_key: c.field_key, label: c.label || c.field_key,
  field_type: c.field_type || 'text', default_visible: !!c.default_visible,
  sort_order: Number(c.sort_order) || 0, is_custom: !!c.is_custom,
})
const openGroupEdit = async (key: string) => {
  const g = GROUP_DEFS.find(x => x.key === key)
  groupEditLabel.value = g ? g.label : '通用字段'
  groupEditKey.value = key
  groupTab.value = 'cols'
  groupFields.value = []
  rowFields.value = []
  groupEditVisible.value = true
  try {
    const res: any = await listUtilityFieldConfigsByGroup(props.category, key)
    const list = res?.data || res
    groupFields.value = (Array.isArray(list) ? list : []).map(toFieldItem)
  } catch { groupFields.value = [] }
  const rg = ROW_GROUP_MAP[key]
  if (rg) {
    try {
      const res2: any = await listUtilityFieldConfigsByGroup(props.category, rg)
      const list2 = res2?.data || res2
      rowFields.value = (Array.isArray(list2) ? list2 : []).map(toFieldItem)
    } catch { rowFields.value = [] }
  }
}
const saveGroupFields = () => {
  if (saving.value) return
  saving.value = true
  const isRow = groupTab.value === 'rows' && hasRowGroup.value
  const target = isRow ? ROW_GROUP_MAP[groupEditKey.value] : groupEditKey.value
  const src = isRow ? rowFields.value : groupFields.value
  saveUtilityFieldConfigs(props.category, src.map((f, i) => ({
    field_key: f.field_key, label: f.label.trim(), field_type: f.field_type || 'text',
    default_visible: !!f.default_visible, sort_order: i, is_custom: !!f.is_custom,
  })), target)
    .then(() => {
      loadGroupCounts()
      emit('changed')
    })
    .catch((e: any) => MessagePlugin.error(e?.message || '保存失败'))
    .finally(() => { saving.value = false })
}
const addGroupField = () => {
  const isRow = groupTab.value === 'rows' && hasRowGroup.value
  const list = isRow ? rowFields.value : groupFields.value
  let n = 1
  const keys = new Set(list.map(f => f.field_key))
  while (keys.has(`custom_${n}`)) n++
  const key = `custom_${n}`
  list.push({
    field_key: key, label: isRow ? `新行${n}` : '自定义字段', field_type: 'text',
    default_visible: false, sort_order: list.length, is_custom: true,
  })
  saveGroupFields()
}
const removeGroupField = (i: number) => {
  const isRow = groupTab.value === 'rows' && hasRowGroup.value
  const list = isRow ? rowFields.value : groupFields.value
  const f = list[i]
  MessagePlugin.confirm(`停用「${f.label}」？历史记录保留原数据，新上传不再显示。`, {
    theme: 'warning', confirmBtn: '停用', cancelBtn: '取消',
  }).then((ok) => {
    if (!ok) return
    list.splice(i, 1)
    list.forEach((x, j) => { x.sort_order = j })
    saveGroupFields()
  })
}
const moveGroupField = (i: number, dir: number) => {
  const isRow = groupTab.value === 'rows' && hasRowGroup.value
  const list = isRow ? rowFields.value : groupFields.value
  const j = i + dir
  if (j < 0 || j >= list.length) return
  const tmp = list[i]
  list[i] = list[j]
  list[j] = tmp
  list.forEach((x, k) => { x.sort_order = k })
  saveGroupFields()
}

// ---- 分组字段计数 ----
const loadGroupCounts = async () => {
  if (props.category !== 'electricity') return
  try {
    const res: any = await listUtilityFieldConfigs(props.category)
    const list = res?.data || res || []
    const counts: Record<string, number> = {}
    const keys = new Set<string>()
    ;(Array.isArray(list) ? list : []).forEach((c: any) => {
      const g = c.group || ''
      counts[g] = (counts[g] || 0) + 1
      keys.add(c.field_key)
    })
    groupCounts.value = counts
    void keys
  } catch { /* ignore */ }
}

// ---- 加载 ----
const load = async () => {
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
  await loadAccounts()
  await loadGroupCounts()
}

watch(() => props.visible, (v) => { if (v) load() })

const onClose = () => emit('update:visible', false)

// ---- 自动保存：包含判定防抖持久化 ----
let saveTimer: ReturnType<typeof setTimeout> | undefined
const scheduleSave = () => {
  clearTimeout(saveTimer)
  saveTimer = setTimeout(() => { persistAll() }, 800)
}
const persistAll = async () => {
  if (saving.value) return
  saving.value = true
  try {
    await saveRecognitionConfig(props.kbId, {
      enabled: cfg.value.enabled,
      include_rules: cfg.value.include_rules.map(r => ({
        id: r.id, name: r.name, match_type: r.match_type,
        keywords: r.match_type === 'keyword' ? (r.keywords || []) : undefined,
        logic: r.logic, regex: r.regex, enabled: r.enabled,
      })),
    })
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 关闭抽屉前落盘最后改动
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

/* 卡片网格（基本户 / 字段分组） */
.us-card-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}

.us-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  padding: 12px 14px;
  cursor: pointer;
  transition: all .15s;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 2px 8px rgba(0, 0, 0, .06);
    transform: translateY(-1px);
  }

  .us-card-head {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;

    .us-card-title {
      font-size: 14px;
      font-weight: 600;
      color: var(--td-text-color-primary);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }

    .us-card-tag {
      flex-shrink: 0;
      font-size: 11px;
      color: var(--td-brand-color);
      border: 1px solid var(--td-brand-color);
      border-radius: 4px;
      padding: 0 5px;
      line-height: 16px;
    }
  }

  .us-card-line {
    font-size: 12px;
    color: var(--td-text-color-secondary);
    line-height: 1.7;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .us-card-action {
    color: var(--td-brand-color);
  }
}

/* 字段配置表：table-layout fixed，固定列宽，标题与内容逐列居中 */
.us-cfg {
  display: table;
  table-layout: fixed;
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  color: var(--td-text-color-secondary);
}

.us-cfg-head,
.us-cfg-row {
  display: table-row;
}

.us-cfg-head {
  > span {
    display: table-cell;
    vertical-align: middle;
    text-align: center;
    padding: 6px 2px;
    border-bottom: 1px solid var(--td-component-stroke);
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
}

.us-cfg-row {
  > .us-cell {
    display: table-cell;
    vertical-align: middle;
    text-align: center;
    padding: 3px 2px;
    overflow: hidden;
  }
}

/* 固定列宽（字段名 / 类型 / 默认显示 / 排序 / 操作） */
.us-col-label { width: 30%; }
.us-col-type { width: 20%; }
.us-col-visible { width: 16%; }
.us-col-order { width: 21%; }
.us-col-del { width: 13%; }

/* 输入框 / 下拉框：不超出单元格，文本居中 */
.us-cfg-input,
.us-cfg-select {
  width: 100% !important;
  max-width: 100% !important;
  min-width: 0 !important;
}

.us-cfg-input {
  :deep(input) {
    text-align: center;
    padding: 0 4px;
  }
}

.us-cfg-select {
  :deep(.t-select__single) {
    justify-content: center;
  }

  :deep(.t-select__wrap) {
    width: 100%;
  }
}

/* 开关：保持固有宽度不拉长，单元格内居中 */
.us-cfg-switch {
  display: inline-flex;
  width: auto !important;
  min-width: 0 !important;
  margin: 0 auto;
}

/* 排序按钮组 / 删除按钮：居中且不换行 */
.us-cell-order {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
}

.us-fixed-type {
  display: inline-block;
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 24px;
}

.us-group-tabs {
  display: flex;
  gap: 4px;
  margin: 4px 0 10px;
  padding: 2px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;
  width: fit-content;
}

.us-group-tab {
  padding: 4px 14px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s ease;

  &:hover {
    color: var(--td-brand-color);
  }

  &.active {
    color: var(--td-brand-color);
    background: var(--td-bg-color-container);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
    font-weight: 500;
  }
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
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid var(--td-component-stroke);
  margin-top: 12px;
}

/* 基本户信息表单 */
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

  &.us-basic-item--wide {
    grid-column: 1 / -1;
  }
}

.us-basic-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 14px;
}
</style>
