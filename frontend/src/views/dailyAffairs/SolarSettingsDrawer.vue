<template>
  <div class="us-wrap">
    <div v-if="visible" class="us-resize-handle" :style="{ right: `${drawerWidth}px` }" role="separator"
      :aria-label="'调整宽度'" :title="'拖动调整宽度'" @mousedown="onResizeStart">
      <div class="us-resize-line" />
    </div>
    <t-drawer v-if="visible" :visible="true" :header="'设置 · 光伏'" :size="`${drawerWidth}px`" :footer="false"
      class="solar-settings-drawer" :close-on-overlay-click="true" @close="onClose"
      @update:visible="(v: boolean) => (v || onClose())">
      <div class="us-body">
        <!-- 基本户信息 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">基本户信息</span>
            <span class="us-state">概览基础信息默认加载</span>
          </div>
          <div class="us-hint">配置默认光伏发电户档案信息，保存后账单概览「基础信息」直接加载，无需每次解析。</div>
          <div class="ba-grid">
            <div class="ba-item">
              <label class="ba-label">户号</label>
              <t-input v-model="account.account_no" size="small" placeholder="发电户号" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">户名</label>
              <t-input v-model="account.account_name" size="small" placeholder="户名" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">服务单位</label>
              <t-input v-model="account.supply_unit" size="small" placeholder="服务单位" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">并网电压</label>
              <t-input v-model="account.voltage_level" size="small" placeholder="并网电压等级" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">消纳方式</label>
              <t-input v-model="account.consumption_mode" size="small" placeholder="消纳方式" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">发电方式</label>
              <t-input v-model="account.generation_mode" size="small" placeholder="发电方式" @blur="saveAccount" />
            </div>
            <div class="ba-item ba-item--full">
              <label class="ba-label">发电地址</label>
              <t-input v-model="account.address" size="small" placeholder="发电地址" @blur="saveAccount" />
            </div>
            <div class="ba-item">
              <label class="ba-label">纳税人类型</label>
              <t-input v-model="account.taxpayer_type" size="small" placeholder="纳税人类型" @blur="saveAccount" />
            </div>
          </div>
        </div>

        <!-- 字段配置 -->
        <div class="us-section">
          <div class="us-section-head">
            <span class="us-section-title">字段配置</span>
            <span class="us-state">分组管理明细列字段，保存后自动同步</span>
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
      </div>
    </t-drawer>

    <!-- 字段分组管理抽屉 -->
    <t-drawer v-if="groupEditVisible" :visible="true" :header="`字段 · ${groupEditLabel}`" size="560px" :footer="false"
      :close-on-overlay-click="true" @close="groupEditVisible = false" @update:visible="(v: boolean) => (v || (groupEditVisible = false))">
      <div class="us-hint">列字段对应明细菜单的列；默认显示控制列显隐。</div>
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
            <t-input v-model="f.label" size="small" class="us-cfg-input" placeholder="字段名" @blur="saveGroupFields" />
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
      <div class="us-actions">
        <t-button variant="text" size="small" @click="addGroupField">
          <template #icon><t-icon name="add" size="14px" /></template>
          新增字段
        </t-button>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'

const props = defineProps<{ visible: boolean; kbId: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void; (e: 'changed'): void }>()

const ACCOUNT_KEY = 'weknora-solar-account-v1'
const CFG_KEY = 'weknora-solar-fee-configs-v1'
const DRAWER_WIDTH_KEY = 'weknora-solar-settings-width'

const drawerWidth = ref(Number(localStorage.getItem(DRAWER_WIDTH_KEY)) || 640)
const account = ref<Record<string, string>>({})
const groupEditVisible = ref(false)
const groupEditKey = ref('')
const groupEditLabel = ref('')
const groupFields = ref<any[]>([])

const FIELD_TYPE_OPTS = [
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '金额', value: 'amount' },
]

const DEFAULT_CFG: Record<string, any[]> = {
  grid: [
    { field_key: 'category', label: '费用组成', field_type: 'text', default_visible: true, sort_order: 0 },
    { field_key: 'qty', label: '计费数量', field_type: 'number', default_visible: true, sort_order: 1 },
    { field_key: 'rate', label: '电价', field_type: 'number', default_visible: true, sort_order: 2 },
    { field_key: 'fee', label: '电费', field_type: 'amount', default_visible: true, sort_order: 3 },
  ],
  subsidy: [
    { field_key: 'category', label: '费用组成', field_type: 'text', default_visible: true, sort_order: 0 },
    { field_key: 'qty', label: '计费数量', field_type: 'number', default_visible: true, sort_order: 1 },
    { field_key: 'rate', label: '电价', field_type: 'number', default_visible: true, sort_order: 2 },
    { field_key: 'fee', label: '电费', field_type: 'amount', default_visible: true, sort_order: 3 },
  ],
}

const groups = [
  { key: 'grid', label: '上网电费' },
  { key: 'subsidy', label: '发电补助' },
]

function loadStored() {
  try {
    const acc = localStorage.getItem(ACCOUNT_KEY)
    if (acc) account.value = { ...account.value, ...JSON.parse(acc) }
  } catch { /* ignore */ }
}
loadStored()

watch(() => props.visible, (v) => {
  if (v) loadStored()
})

const saveAccount = () => {
  try { localStorage.setItem(ACCOUNT_KEY, JSON.stringify(account.value)) } catch { /* ignore */ }
  emit('changed')
}

const groupFieldCount = (key: string) => loadCfg()[key]?.length || 0

function loadCfg(): Record<string, any[]> {
  try {
    const raw = localStorage.getItem(CFG_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore */ }
  return JSON.parse(JSON.stringify(DEFAULT_CFG))
}
function persistCfg(cfg: Record<string, any[]>) {
  try { localStorage.setItem(CFG_KEY, JSON.stringify(cfg)) } catch { /* ignore */ }
}

const openGroupEdit = (key: string) => {
  groupEditKey.value = key
  const g = groups.find(x => x.key === key)
  groupEditLabel.value = g?.label || key
  groupFields.value = JSON.parse(JSON.stringify(loadCfg()[key] || DEFAULT_CFG[key] || []))
  groupEditVisible.value = true
}
const saveGroupFields = () => {
  const cfg = loadCfg()
  cfg[groupEditKey.value] = groupFields.value.map((f, i) => ({ ...f, sort_order: i }))
  persistCfg(cfg)
  emit('changed')
}
const addGroupField = () => {
  groupFields.value.push({ field_key: `custom_${Date.now()}`, label: '新字段', field_type: 'text', default_visible: false, sort_order: groupFields.value.length })
  saveGroupFields()
}
const removeGroupField = (i: number) => {
  groupFields.value.splice(i, 1)
  saveGroupFields()
}
const moveGroupField = (i: number, dir: number) => {
  const j = i + dir
  if (j < 0 || j >= groupFields.value.length) return
  const tmp = groupFields.value[i]
  groupFields.value[i] = groupFields.value[j]
  groupFields.value[j] = tmp
  saveGroupFields()
}

// 拖动调整宽度（与删除历史抽屉一致）
const onResizeStart = (e: MouseEvent) => {
  e.preventDefault()
  const startX = e.clientX
  const startW = drawerWidth.value
  const onMove = (ev: MouseEvent) => {
    const w = Math.min(960, Math.max(480, startW - (ev.clientX - startX)))
    drawerWidth.value = w
  }
  const onUp = () => {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    try { localStorage.setItem(DRAWER_WIDTH_KEY, String(drawerWidth.value)) } catch { /* ignore */ }
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

const onClose = () => {
  emit('update:visible', false)
}
</script>

<style lang="less" scoped>
.us-wrap { position: relative; }
.us-resize-handle {
  position: fixed;
  top: 0;
  bottom: 0;
  width: 6px;
  z-index: 2100;
  cursor: col-resize;
  &:hover .us-resize-line, .us-resize-line:hover {
    background: var(--td-brand-color);
  }
  .us-resize-line {
    width: 2px;
    height: 100%;
    margin-left: 2px;
    background: var(--td-component-stroke);
    transition: background 0.2s;
  }
}
.us-body { display: flex; flex-direction: column; gap: 16px; }
.us-section {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 14px 16px;
}
.us-section-head { display: flex; align-items: center; gap: 8px; margin-bottom: 8px; }
.us-section-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.us-state { font-size: 12px; color: var(--td-text-color-secondary); }
.us-spacer { flex: 1; }
.us-hint { font-size: 12px; color: var(--td-text-color-secondary); margin-bottom: 12px; line-height: 1.6; }
.us-card-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.us-card {
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  padding: 12px 14px;
  cursor: pointer;
  transition: box-shadow 0.2s, border-color 0.2s;
  &:hover { border-color: var(--td-brand-color); box-shadow: 0 2px 10px rgba(0, 0, 0, 0.08); }
}
.us-card-head { margin-bottom: 8px; }
.us-card-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.us-card-line { font-size: 12px; color: var(--td-text-color-secondary); line-height: 1.8; }
.us-card-action { color: var(--td-brand-color); }
.us-empty { padding: 20px 0; text-align: center; font-size: 13px; color: var(--td-text-color-secondary); }

.ba-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.ba-item { display: flex; flex-direction: column; gap: 4px; }
.ba-item--full { grid-column: 1 / -1; }
.ba-label { font-size: 12px; color: var(--td-text-color-secondary); }

/* 字段配置 5 列表格：fixed 布局，列宽固定，内容居中，输入框不溢出 */
.us-cfg {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--td-component-stroke);
  border-radius: 6px;
  overflow: hidden;
  margin-top: 8px;
}
.us-cfg-head, .us-cfg-row {
  display: grid;
  grid-template-columns: 2fr 1fr 1fr 1.2fr 0.8fr;
  table-layout: fixed;
  align-items: center;
  text-align: center;
}
.us-cfg-head {
  background: var(--td-bg-color-secondarycontainer);
  font-size: 12px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  padding: 8px 0;
  border-bottom: 1px solid var(--td-component-stroke);
}
.us-cfg-row {
  padding: 6px 0;
  border-bottom: 1px solid var(--td-component-stroke);
  &:last-child { border-bottom: none; }
}
.us-cell { min-width: 0; padding: 0 6px; }
.us-cfg-input, .us-cfg-select { width: 100%; max-width: 100%; }
.us-cfg-switch { display: inline-flex; }
.us-cell-order { display: inline-flex; gap: 2px; align-items: center; }
.us-actions { margin-top: 12px; }
</style>
