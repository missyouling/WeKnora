<template>
  <div class="meter-settings-body">
    <!-- 分组标题（公司证照在上、司机证照在下，带数量） -->
    <div class="group-headers">
      <div v-for="g in groupList" :key="g.key" class="group-header" :class="{ active: activeGroup === g.key }" @click="activeGroup = g.key">
        <span v-if="renamingKey === g.key" class="group-header__rename-wrap">
          <t-input v-model="renameValue" size="small" @enter="confirmRename(g)" @blur="confirmRename(g)" autofocus />
        </span>
        <span v-else class="group-header__title" title="双击重命名分组" @dblclick.stop="startRename(g)">{{ displayLabel(g) }}</span>
        <span class="group-header__count">{{ groupCount(g.key) }}</span>
        <span v-if="!g.builtin" class="group-header__del-wrap" @click.stop>
          <t-popconfirm theme="warning" :visible="delGroupKey === g.key" placement="bottom"
            content="确定删除该自定义分组吗？仅空分组可删除。"
            :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
            @confirm="doRemoveGroup(g)" @visible-change="(v: boolean) => onDelGroupVisible(g, v)">
            <t-icon name="close" size="12px" class="group-header__del" />
          </t-popconfirm>
        </span>
      </div>
      <div class="group-header group-header--add" @click="startAddGroup" title="新增分组">
        <t-icon name="add" size="14px" />
      </div>
      <div v-if="addingGroup" class="group-header group-header--adding">
        <t-input v-model="newGroupName" size="small" placeholder="输入分组名" @enter="confirmAddGroup" @blur="confirmAddGroup" autofocus />
      </div>
    </div>

    <!-- 证照类型表格（复刻电表配置表格，支持拖拽排序） -->
    <div class="meter-table-wrap">
      <div class="meter-table">
        <div class="meter-table-head">
          <span class="th-drag"></span>
          <span class="th-name">{{ group.scope === 'maintain' ? '维保项名称' : '证照名称' }}</span>
          <span>分组</span>
          <span>字段数</span>
          <span>状态</span>
          <span>操作</span>
        </div>

        <template v-for="(item, idx) in groupItems" :key="item.id">
          <div class="meter-table-row" :class="{ editing: editingId === item.id, dragging: dragIndex === idx }"
            draggable="true" @dragstart="onDragStart(idx)" @dragover.prevent="onDragOver"
            @drop.prevent="onDrop(idx)" @dragend="dragIndex = -1" @click="toggleEdit(item)">
            <span class="mtr-drag" title="拖动排序"><t-icon name="move" size="14px" /></span>
            <span class="mtr-name" :class="{ 'mtr-disabled': !item.enabled }">{{ item.name }}</span>
            <span class="mtr-scope">{{ group.label }}</span>
            <span class="mtr-fields">{{ item.fields }}</span>
            <span class="mtr-switch" @click.stop>
              <t-switch :model-value="!!item.enabled" size="small" @change="(v: any) => toggleEnabled(item, v)" />
            </span>
            <span class="meter-row-actions" @click.stop>
              <t-popconfirm v-if="!item.builtin" theme="warning" :content="`确定删除证照类型「${item.name}」吗？`"
                :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="removeItem(item)">
                <t-button variant="text" size="small" @click.stop>
                  <template #icon><t-icon name="delete" size="15px" /></template>
                </t-button>
              </t-popconfirm>
              <span v-else class="mtr-builtin">内置</span>
            </span>
          </div>
          <!-- 行内展开编辑（复刻 meter-form--inline） -->
          <div v-if="editingId === item.id" class="meter-form meter-form--inline">
            <div class="meter-form-title">编辑{{ group.label }}</div>
            <div class="form-grid">
              <div class="form-item">
                <label>{{ group.scope === 'maintain' ? '维保项名称' : '证照名称' }} <span class="required">*</span></label>
                <t-input v-model="editForm.name" @enter="commitEdit" />
              </div>
            </div>
            <!-- 字段配置：增删改 + 启用/禁用（同步字段筛选器） -->
            <div class="field-config">
              <div class="field-config-head">
                <span class="field-config-title">字段配置</span>
                <span class="field-config-tip">启用的字段在字段筛选器与列表显示，禁用后隐藏</span>
              </div>
              <div class="field-config-list">
                <div class="field-colhead">
                  <span class="fc-h-drag"></span>
                  <span class="fc-h-name">字段名</span>
                  <span class="fc-h-type">数据类型</span>
                  <span class="fc-h-switch">默认表头</span>
                  <span class="fc-h-switch">启用字段</span>
                  <span class="fc-h-op">操作</span>
                </div>
                <div v-for="(fd, i) in fieldsEditable" :key="fd.name" class="field-config-row"
                  :class="{ 'fc-dragging': fieldDragIndex === i }"
                  draggable="true" @dragstart="onFieldDragStart(i)" @dragover.prevent
                  @drop.prevent="onFieldDrop(i)" @dragend="fieldDragIndex = -1">
                  <span class="fc-drag" title="拖动排序"><t-icon name="move" size="14px" /></span>
                  <t-input v-model="fd.name" size="small" placeholder="字段名" class="fc-name" @enter="addField" />
                  <t-select v-model="fd.dataType" size="small" class="fc-type" :options="dataTypeOptions" />
                  <t-tooltip content="默认字段：重置字段筛选器时自动勾选" placement="top">
                    <t-switch :model-value="!!fd.isDefault" size="small" @change="(v: any) => (fd.isDefault = !!v)" />
                  </t-tooltip>
                  <t-switch :model-value="!!fd.enabled" size="small" @change="(v: any) => (fd.enabled = !!v)" />
                  <t-popconfirm v-if="!usedFields.has(fd.name)" theme="warning"
                    :content="`确定删除字段「${fd.name}」吗？`"
                    :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                    @confirm="removeField(fd)">
                    <t-button variant="text" size="small">
                      <template #icon><t-icon name="delete" size="15px" /></template>
                    </t-button>
                  </t-popconfirm>
                  <t-tooltip v-else :content="`该字段已被证照记录引用，不能删除`" placement="top">
                    <span class="fc-used"><t-icon name="lock-on" size="15px" /></span>
                  </t-tooltip>
                </div>
                <div class="field-config-add">
                  <t-input v-model="newFieldName" size="small" placeholder="新增字段名" class="fc-add-input"
                    @enter="addField" />
                  <t-button variant="outline" size="small" @click="addField">添加</t-button>
                </div>
              </div>
            </div>
            <div class="meter-form-actions">
              <t-button variant="outline" size="small" @click="editingId = ''">取消</t-button>
              <t-button theme="primary" size="small" :loading="saving" @click="commitEdit">保存</t-button>
            </div>
          </div>
        </template>

        <!-- 空状态：表头始终在顶部，图标与简化说明置于表头下方 -->
        <div v-if="!groupItems.length && !addVisible" class="panel-empty panel-empty--inline">
          <t-icon name="folder-open" size="26px" class="panel-empty-icon" />
          <span class="panel-empty-text">{{ group.scope === 'maintain' ? '暂无维保类型' : '暂无证照类型' }}</span>
        </div>

        <!-- 列表末尾内联新增：输入名称后自动追加到列表 -->
        <div v-if="addVisible" class="cert-add-row cert-add-row--editing" @click.stop>
          <span class="cert-add-gap"></span>
          <span class="cert-add-input">
            <t-input ref="addInputRef" v-model="addForm.name" size="small" placeholder="如：车辆购置税完税证明" @enter="commitAdd" />
          </span>
          <span class="cert-add-actions">
            <t-button variant="outline" size="small" @click="addVisible = false">取消</t-button>
            <t-button theme="primary" size="small" :loading="saving" @click="commitAdd">添加</t-button>
          </span>
        </div>
        <div v-else class="cert-add-row" @click="startAdd">
          <span class="cert-add-gap"></span>
          <span class="cert-add-name"><t-icon name="add" size="14px" /><span>新增{{ group.label }}</span></span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch, nextTick } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listFleetCategories, createFleetCategory, updateFleetCategory, deleteFleetCategory, sortFleetCategories, listFleetRecords, listFleetCertGroups, createFleetCertGroup, deleteFleetCertGroup, listFleetGroupAliases, upsertFleetGroupAlias } from '@/api/fleet'

const props = withDefaults(defineProps<{ scope?: 'vehicle' | 'driver' | 'maintain' | '' }>(), { scope: 'vehicle' })

// 分组：车辆档案=公司证照；司机档案=司机证照；维保档案=维保文件
const GROUPS: Record<string, { key: string; label: string; scope: string; groupId: string }[]> = {
  vehicle: [
    { key: 'company', label: '公司证照', scope: 'vehicle', groupId: '' },
  ],
  driver: [{ key: 'driver', label: '司机证照', scope: 'driver', groupId: '' }],
  maintain: [{ key: 'maintain', label: '维保类型', scope: 'maintain', groupId: '' }],
}
const customGroups = ref<any[]>([])
async function loadCustomGroups() {
  try {
    const parentScope = props.scope === 'driver' ? 'driver' : 'vehicle'
    const res: any = await listFleetCertGroups({ parent_scope: parentScope })
    const arr = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    const fallbackScope = props.scope === 'driver' ? 'driver' : (props.scope === 'maintain' ? 'maintain' : 'vehicle')
    customGroups.value = arr.map((g: any) => ({ key: g.id, label: g.name, scope: g.parent_scope || fallbackScope, groupId: g.id, builtin: false, id: g.id }))
  } catch { customGroups.value = [] }
}
const groupList = computed(() => {
  const builtin = (GROUPS[props.scope] || GROUPS.vehicle).map((g: any) => ({ ...g, builtin: true }))
  return [...builtin, ...customGroups.value]
})
const addingGroup = ref(false)
const savingGroup = ref(false)
const newGroupName = ref('')
function startAddGroup() {
  addingGroup.value = true
  newGroupName.value = ''
}
async function confirmAddGroup() {
  if (savingGroup.value) return
  const name = newGroupName.value.trim()
  // 同步关闭输入框并清空名称：Enter 提交后输入框卸载会触发 blur，清空可避免同次输入重复建组
  addingGroup.value = false
  newGroupName.value = ''
  if (!name) return
  savingGroup.value = true
  try {
    const parentScope = props.scope === 'driver' ? 'driver' : 'vehicle'
    const res: any = await createFleetCertGroup({ name, parent_scope: parentScope })
    const g = res?.data || res
    if (!g || !g.id) { MessagePlugin.error('创建失败：返回数据异常'); return }
    customGroups.value.push({ key: g.id, label: g.name, scope: parentScope, groupId: g.id, builtin: false, id: g.id })
    activeGroup.value = g.id
    MessagePlugin.success('分组已创建')
  } catch (e: any) { MessagePlugin.error(e?.message || '创建失败') }
  finally { savingGroup.value = false }
}
const delGroupKey = ref('')
// 受控 Popconfirm：打开前做引用判断，组内仍有证照则拦截并提示
function onDelGroupVisible(g: any, v: boolean) {
  if (v) {
    const n = groupCount(g.key)
    if (n > 0) {
      delGroupKey.value = ''
      MessagePlugin.warning(`分组「${g.label}」下还有 ${n} 个证照类型，请先移除或重新归类后再删除分组。`)
      return
    }
    delGroupKey.value = g.key
  } else if (delGroupKey.value === g.key) {
    delGroupKey.value = ''
  }
}
async function doRemoveGroup(g: any) {
  try {
    await deleteFleetCertGroup(g.id)
    customGroups.value = customGroups.value.filter((x: any) => x.key !== g.key)
    if (activeGroup.value === g.key) activeGroup.value = groupList.value[0]?.key || 'company'
    MessagePlugin.success('分组已删除')
  } catch (e: any) { MessagePlugin.error(e?.message || '删除失败') }
  finally { delGroupKey.value = '' }
}

// 内置证照类型（字段：base=默认显示，detail=详细字段；上传解析后自动更新，内置不可删除）
type CertDef = { name: string; base: string[]; detail: string[] }
const BUILTIN: Record<string, CertDef[]> = {
  vehicle: [
    {
      name: '车辆登记证书',
      base: ['登记证书编号', '车牌号码', '机动车所有人', '车辆识别代号', '车辆品牌', '车辆型号', '车辆类型', '使用性质', '登记日期', '发证日期'],
      detail: ['证书编号', '车牌号', '发证机关', '证书状态', '存放位置', '是否随车', 'VIN', '发动机号', '品牌型号', '注册日期', '报废日期', '车主信息', '产权归属', '过户记录', '备注'],
    },
    {
      name: '行驶证',
      base: ['车牌号', 'VIN码/车架号', '发动机号', '品牌型号', '车辆类型', '使用性质', '注册日期', '发证日期', '发证机关', '强制报废日期', '证件状态'],
      detail: ['备注'],
    },
    {
      name: '道路运输经营许可证',
      base: ['许可证号', '业户名称', '经营地址', '经营范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态', '备注'],
      detail: [],
    },
    {
      name: '道路运输证',
      base: ['道路运输证号', '车牌号', '经营许可证号', '车辆类型', '吨（座）位', '业户名称', '经营地址', '经营范围', '车辆尺寸', '发证日期', '有效期止', '发证机关', '上次审验日期', '下次审验日期', '技术评定等级', '备注'],
      detail: [],
    },
    {
      name: '保险单',
      base: ['保单号', '保险公司', '保险类型', '车辆ID', '车牌号', 'VIN码', '被保险人名称', '险种名称', '保额', '保费', '总保费', '起保日期', '终保日期', '保单状态', '缴费状态', '发票号', '备注'],
      detail: [],
    },
  ],
  driver: [
    {
      name: '驾驶证',
      base: ['驾驶证号', '司机姓名', '准驾车型', '初次领证日期', '有效期起', '有效期止', '发证机关', '驾驶证状态', '备注'],
      detail: [],
    },
    {
      name: '从业资格证',
      base: ['从业资格证号', '司机姓名', '从业资格类别', '准运范围', '发证机关', '发证日期', '有效期起', '有效期止', '证件状态', '备注'],
      detail: [],
    },
  ],
  maintain: [
    { name: '维修工单', base: ['工单号', '车牌号', '维修日期', '维修项目', '工时费', '材料费', '总费用'], detail: [] },
    { name: '二级维护', base: ['维护日期', '车牌号', '维护项目', '维护单位', '下次维护日期'], detail: [] },
  ],
}
function certFieldCount(b: CertDef) {
  return new Set([...b.base, ...b.detail]).size
}

const activeGroup = ref('company')
const categories = ref<Record<string, any[]>>({ vehicle: [], driver: [], maintain: [] })
const saving = ref(false)
const orderTick = ref(0)

const group = computed(() => groupList.value.find((g) => g.key === activeGroup.value) || groupList.value[0])

// 分组重命名（纯显示别名，localStorage 持久化；不影响数据关联）
const groupAliases = ref<Record<string, string>>({})
function aliasKey(g: any) { return `${g.scope}:${g.key}` }
function displayLabel(g: any) { return groupAliases.value[aliasKey(g)] || g.label }
async function loadGroupAliases() {
  try {
    const res: any = await listFleetGroupAliases()
    const arr = res?.data || res || []
    const m: Record<string, string> = {}
    for (const it of arr) m[`${it.scope}:${it.group_key}`] = it.name
    groupAliases.value = m
  } catch { groupAliases.value = {} }
}
loadGroupAliases()
const renamingKey = ref('')
const renameValue = ref('')
function startRename(g: any) { renamingKey.value = g.key; renameValue.value = displayLabel(g) }
async function confirmRename(g: any) {
  const v = renameValue.value.trim()
  if (v && v !== displayLabel(g)) {
    groupAliases.value[aliasKey(g)] = v
    try { await upsertFleetGroupAlias({ scope: g.scope, group_key: g.key, name: v }) } catch { /* 持久化失败不影响本地显示 */ }
  }
  renamingKey.value = ''
}

function buildItems(g0: any): any[] {
  void orderTick.value // 建立响应式依赖：本地拖拽顺序变化后重算
  const scope: string = g0.scope
  const gid: string = g0.groupId || ''
  // 仅取归属当前分组的分类（内置分组 group_id 为空；自定义分组按 group_id 过滤）
  const list = (categories.value[scope] || []).filter((c: any) => (c.group_id || '') === gid)
  const merged: any[] = []
  const seen = new Set<string>()
  // 内置证照类型只出现在内置分组（groupId 为空）
  if (!gid) {
    ;(BUILTIN[scope] || []).forEach((b) => {
      const found = list.find((c: any) => c.name === b.name)
      if (found) {
        merged.push({ ...found, builtin: true, fields: (Array.isArray(found.subs) && found.subs.length) ? found.subs.length : certFieldCount(b) })
      } else {
        merged.push({ id: `builtin-${b.name}`, name: b.name, enabled: true, subs: [], builtin: true, fields: certFieldCount(b) })
      }
      seen.add(b.name)
    })
  }
  list.forEach((c: any) => {
    if (!seen.has(c.name)) { merged.push({ ...c, builtin: false, fields: Array.isArray(c.subs) ? c.subs.length : 0 }); seen.add(c.name) }
  })
  // 本地持久化的拖拽顺序（按分组 key 隔离，以名称作排序键：id 会因入库/改名而失效）
  try {
    const saved = JSON.parse(localStorage.getItem(`weknora-fleet-cert-order-${g0.key}`) || '[]')
    if (Array.isArray(saved) && saved.length) {
      const byName = new Map(merged.map((m) => [m.name, m]))
      const byId = new Map(merged.map((m) => [m.id, m]))
      const ordered: any[] = []
      const pushed = new Set<any>()
      for (const key of saved) {
        let item = byName.get(key)
        // 兼容旧数据：builtin-{name} / 入库后的 uuid
        if (!item && typeof key === 'string' && key.startsWith('builtin-')) {
          item = byName.get(key.slice('builtin-'.length))
        }
        if (!item && byId.has(key)) item = byId.get(key)
        if (item && !pushed.has(item)) {
          ordered.push(item)
          pushed.add(item)
        }
      }
      const rest = merged.filter((m) => !pushed.has(m))
      return [...ordered, ...rest]
    }
  } catch { /* ignore */ }
  return merged
}
const groupItems = computed(() => buildItems(group.value))
function groupCount(key: string) {
  const g = groupList.value.find((x: any) => x.key === key)
  return g ? buildItems(g).length : 0
}

// ---- 拖拽排序（持久化到后端 sort_order）----
const dragIndex = ref(-1)
function onDragStart(i: number) { dragIndex.value = i }
function onDragOver() { /* 需要 preventDefault 以允许 drop */ }
async function onDrop(i: number) {
  const from = dragIndex.value
  dragIndex.value = -1
  if (from < 0 || from === i) return
  const list = [...groupItems.value]
  const [moved] = list.splice(from, 1)
  list.splice(i, 0, moved)
  // 完整顺序（含内置）持久化到本地：以名称作排序键（id 会因入库/改名而失效）
  try {
    localStorage.setItem(`weknora-fleet-cert-order-${group.value.key}`, JSON.stringify(list.map((it: any) => it.name)))
  } catch { /* ignore */ }
  orderTick.value++
  // 已入库项同步后端 sort_order
  const ids = list.filter((it: any) => !String(it.id).startsWith('builtin-')).map((it: any) => it.id)
  if (!ids.length) return
  try {
    await sortFleetCategories(group.value.scope, ids)
    MessagePlugin.success('顺序已保存')
    notifyCategoriesChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '排序保存失败')
    load()
  }
}

// ---- 字段级拖拽排序（仅调整编辑态 fieldsEditable 顺序，保存时随 subs 持久化）----
const fieldDragIndex = ref(-1)
function onFieldDragStart(i: number) { fieldDragIndex.value = i }
function onFieldDrop(i: number) {
  const from = fieldDragIndex.value
  fieldDragIndex.value = -1
  if (from < 0 || from === i) return
  const list = [...fieldsEditable.value]
  const [moved] = list.splice(from, 1)
  list.splice(i, 0, moved)
  fieldsEditable.value = list
}

// 分类变更后通知列表页刷新证照类型选项（上传弹窗 / 筛选框依赖 categories）
function notifyCategoriesChanged() {
  window.dispatchEvent(new CustomEvent('fleet-categories-changed'))
}

// 一次性迁移：对已入库分类，若从未设置过默认字段，则把其内置 base 字段在 subs 中标记 is_default=true 并保存。
// 之后用户在字段配置里可自行调整；以 localStorage 标记按 scope 只跑一次。
async function migrateDefaultFlags(scope: string, list: any[]) {
  try {
    const flagKey = `weknora-fleet-default-migrated-${scope}`
    if (localStorage.getItem(flagKey)) return
    const builtinList = (BUILTIN as any)[scope] || []
    for (const c of list) {
      if (!c.id || String(c.id).startsWith('builtin-')) continue
      const subs = Array.isArray(c.subs) ? c.subs : []
      if (!subs.length) continue
      if (subs.some((x: any) => x.is_default === true)) continue
      const b = builtinList.find((x: any) => x.name === c.name)
      const baseSet = new Set((b && b.base) || [])
      if (!baseSet.size) continue
      let changed = false
      const newSubs = subs.map((x: any) => {
        const isDef = baseSet.has(String(x.name))
        if (isDef) changed = true
        return { ...x, is_default: isDef }
      })
      if (changed) {
        try { await updateFleetCategory(c.id, { name: c.name, subs: newSubs, enabled: c.enabled !== false }) } catch { /* 迁移失败不阻塞 */ }
      }
    }
    localStorage.setItem(flagKey, '1')
  } catch { /* ignore */ }
}

async function load() {
  try {
    const scopes = [...new Set(groupList.value.map((g) => g.scope))]
    const resList = await Promise.all(scopes.map((s) => listFleetCategories({ scope: s })))
    scopes.forEach(async (s, i) => {
      let list = resList[i].data || []
      // 内置类型更名迁移：旧「营运证」→「道路运输经营许可证」（车辆档案组）
      if (s === 'vehicle') {
        const legacy = list.find((c: any) => c.name === '营运证')
        if (legacy?.id) {
          try {
            await updateFleetCategory(legacy.id, { name: '道路运输经营许可证', subs: legacy.subs || [], enabled: legacy.enabled })
            list = list.map((c: any) => (c.id === legacy.id ? { ...c, name: '道路运输经营许可证' } : c))
          } catch { /* 迁移失败保持原样，不阻塞 */ }
        }
      }
      categories.value[s] = list
      usedFields.value = new Set()
      migrateDefaultFlags(s, list)
    })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '证照类型加载失败')
  }
}

// ---- 字段引用判断：删除字段前检查是否已被证照记录引用 ----
const RECORD_TYPE_MAP: Record<string, string> = {
  vehicle: 'vehicle-archive', driver: 'driver-archive', maintain: 'maintain-archive',
}
const usedFields = ref<Set<string>>(new Set())
async function loadUsedFields(scope: string, certName?: string) {
  const type = RECORD_TYPE_MAP[scope]
  if (!type) return
  try {
    // 引用边界：只统计当前编辑证照类型自身记录里出现的字段，跨证照同名字段不互相牵连
    const res: any = await listFleetRecords(certName ? { type, doc_type: certName } : { type })
    const set = new Set<string>()
    ;(res.data || []).forEach((r: any) => Object.keys(r.data || {}).forEach((k) => set.add(String(k))))
    usedFields.value = set
  } catch { /* ignore */ }
}
onMounted(() => { load(); loadCustomGroups() })
watch(() => props.scope, () => {
  activeGroup.value = groupList.value[0]?.key || 'company'
  addVisible.value = false
  editingId.value = ''
  load()
})

// ---- 新增 ----
const addVisible = ref(false)
const addForm = reactive({ name: '' })
const addInputRef = ref<any>(null)
function startAdd() {
  addForm.name = ''
  addVisible.value = true
  nextTick(() => {
    addInputRef.value?.focus?.()
    document.querySelector('.cert-add-row--editing')?.scrollIntoView({ block: 'nearest' })
  })
}
// 供抽屉标题栏右侧「新增」按钮调用（FleetSettingsDrawer header）
const addLabel = computed(() => `新增${group.value.label}`)
defineExpose({ startAdd, addLabel })
async function commitAdd() {
  const name = addForm.name.trim()
  if (!name) { MessagePlugin.warning('请输入证照名称'); return }
  if (groupItems.value.some((it: any) => it.name === name)) { MessagePlugin.warning('该证照类型已存在'); return }
  saving.value = true
  try {
    const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', name, subs: [] })
    const created = res.data || res
    if (created?.id) {
      // 兜底：确保新记录带上当前分组 group_id，避免后端响应未回填时短暂错归到内置分组
      const record = { ...created, group_id: created.group_id || group.value.groupId || '' }
      categories.value[group.value.scope] = [...(categories.value[group.value.scope] || []), record]
      addVisible.value = false
      MessagePlugin.success('证照类型已创建')
      notifyCategoriesChanged()
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建失败')
  } finally {
    saving.value = false
  }
}

// ---- 编辑（点击行展开）----
const editingId = ref('')
const editForm = reactive({ name: '' })
const fieldsEditable = ref<{ name: string; enabled: boolean; isDefault: boolean; dataType: string }[]>([])
const newFieldName = ref('')
const dataTypeOptions = [
  { label: '文本', value: 'text' },
  { label: '数字', value: 'number' },
  { label: '日期', value: 'date' },
  { label: '数组', value: 'array' },
]
function toggleEdit(item: any) {
  if (editingId.value === item.id) { editingId.value = ''; usedFields.value = new Set(); return }
  editForm.name = item.name
  newFieldName.value = ''
  // 字段来源：已入库 subs（含解析提取字段）优先，否则内置 base+detail
  const b = (BUILTIN[group.value.scope] || []).find((x: any) => x.name === item.name)
  const raw = Array.isArray(item.subs) && item.subs.length
    ? item.subs
    : [...(b?.base || []), ...(b?.detail || [])].map((n) => ({ name: n, enabled: true, isDefault: false }))
  const seen = new Set<string>()
  fieldsEditable.value = raw
    .map((s: any) => ({ name: String(s.name || s), enabled: s.enabled !== false, isDefault: s.is_default === true, dataType: s.data_type || 'text' }))
    .filter((s: any) => { if (seen.has(s.name)) return false; seen.add(s.name); return true })
  editingId.value = item.id
  loadUsedFields(group.value.scope, item.name)
}
function addField() {
  const name = newFieldName.value.trim()
  if (!name) { MessagePlugin.warning('请输入字段名'); return }
  if (fieldsEditable.value.some((f) => f.name === name)) { MessagePlugin.warning('字段已存在'); return }
  fieldsEditable.value.push({ name, enabled: true, isDefault: false, dataType: 'text' })
  newFieldName.value = ''
}
function removeField(fd: any) {
  fieldsEditable.value = fieldsEditable.value.filter((f) => f !== fd)
}

async function commitEdit() {
  const item = groupItems.value.find((it: any) => it.id === editingId.value)
  if (!item) { editingId.value = ''; return }
  const name = editForm.name.trim()
  if (!name) { MessagePlugin.warning('请输入证照名称'); return }
  const subs = fieldsEditable.value
    .map((f) => ({ name: String(f.name).trim(), enabled: !!f.enabled, is_default: !!f.isDefault, data_type: f.dataType || 'text' }))
    .filter((f) => f.name)
  saving.value = true
  try {
    if (item.builtin && item.id.startsWith('builtin-')) {
      // 内置类型尚未入库：字段配置后自动入库
      const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', name, subs })
      const created = res.data || res
      if (created?.id) {
        categories.value[group.value.scope] = [...(categories.value[group.value.scope] || []), created]
        MessagePlugin.success('证照类型已创建')
        notifyCategoriesChanged()
      }
    } else {
      await updateFleetCategory(item.id, { name, subs, enabled: item.enabled })
      const list = categories.value[group.value.scope]
      const idx = list.findIndex((c: any) => c.id === item.id)
      if (idx >= 0) list[idx] = { ...list[idx], name, subs }
      MessagePlugin.success('已保存')
      notifyCategoriesChanged()
    }
    editingId.value = ''
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// ---- 启用/禁用 ----
async function toggleEnabled(item: any, v: boolean) {
  if (item.builtin && item.id.startsWith('builtin-')) {
    // 内置未入库：启用即入库
    if (v) {
      try {
        const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', name: item.name, subs: [] })
        const created = res.data || res
        if (created?.id) {
          categories.value[group.value.scope] = [...(categories.value[group.value.scope] || []), created]
          notifyCategoriesChanged()
        }
      } catch { MessagePlugin.error('启用失败') }
    }
    return
  }
  try {
    await updateFleetCategory(item.id, { name: item.name, subs: item.subs || [], enabled: v })
    const list = categories.value[group.value.scope]
    const idx = list.findIndex((c: any) => c.id === item.id)
    if (idx >= 0) list[idx] = { ...list[idx], enabled: v }
    notifyCategoriesChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

// ---- 删除（仅自定义，二次确认）----
async function removeItem(item: any) {
  try {
    await deleteFleetCategory(item.id)
    categories.value[group.value.scope] = (categories.value[group.value.scope] || []).filter((c: any) => c.id !== item.id)
    if (editingId.value === item.id) editingId.value = ''
    MessagePlugin.success('已删除')
    notifyCategoriesChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

watch(activeGroup, () => { editingId.value = ''; addVisible.value = false })
</script>

<style lang="less" scoped>
/* 复刻电表配置抽屉样式（UtilityMeterTab） */
.meter-settings-body {
  padding: 4px 0 24px;
}

.meter-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* 分组标签：复用租户核算设置标签卡样式(选中绿色下划线) */
.group-headers { display: flex; align-items: center; gap: 24px; padding: 12px 0 8px; border-bottom: 1px solid var(--td-component-stroke); margin-bottom: 12px; }
.group-header { display: flex; align-items: center; gap: 6px; cursor: pointer; padding: 4px 0; font-size: 14px; color: var(--td-text-color-secondary); border-bottom: 2px solid transparent; margin-bottom: -9px; transition: color .15s; }
.group-header:hover { color: var(--td-brand-color); }
.group-header.active { color: var(--td-brand-color); border-bottom-color: var(--td-brand-color); font-weight: 500; }
.group-header__title { font-size: 14px; }
.group-header__rename-wrap { width: 120px; }
.group-header__rename-wrap :deep(.t-input__inner) { height: 24px; font-size: 13px; padding: 0 6px; }
.group-header__count { font-size: 12px; color: var(--td-text-color-placeholder); background: var(--td-bg-color-component); border-radius: 10px; padding: 0 8px; line-height: 18px; }
.group-header.active .group-header__count { color: var(--td-brand-color); background: var(--td-brand-color-1); }
.group-header__del-wrap { display: inline-flex; align-items: center; }
.group-header__del { color: var(--td-text-color-placeholder); transition: color .15s; cursor: pointer; }
.group-header__del:hover { color: var(--td-error-color); }
/* 新增分组「+」与内置分组标签同形 */
.group-header--add { color: var(--td-text-color-secondary); }
.group-header--add:hover { color: var(--td-brand-color); }
/* 内联命名输入：去掉标签下划线占位，输入框与标签等高对齐 */
.group-header--adding { border-bottom: none; margin-bottom: 0; padding: 0; cursor: default; }
.group-header--adding:hover { color: inherit; }
.group-header--adding :deep(.t-input) { width: 132px; }
.group-header--adding :deep(.t-input .t-input__inner) { height: 24px; }
.meter-settings-tabs {
  margin-bottom: 10px;

  :deep(.t-tabs__header) {
    margin-bottom: 8px;
  }

  :deep(.t-tabs__nav-item) {
    font-size: 13px;
  }

  :deep(.t-tabs__nav-item-wrapper) {
    padding: 0;
    margin: 0;
  }

  :deep(.t-tabs__nav-item:not(:first-child) .t-tabs__nav-item-wrapper) {
    margin-left: 8px;
  }

  :deep(.t-tabs__content) {
    overflow: visible;
  }
}

.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);

  .meter-table-head,
  .meter-table-row {
    display: grid;
    grid-template-columns: 30px 1.6fr 0.9fr 0.6fr 0.6fr 0.7fr;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    font-size: 12px;
  }

  .meter-table-head {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
    font-weight: 500;
  }

  .meter-table-row {
    border-bottom: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    cursor: pointer;
    transition: background 0.15s;

    &:hover { background: var(--td-bg-color-secondarycontainer); }
    &.editing { background: var(--td-brand-color-light); }
    &.dragging { opacity: 0.5; background: var(--td-brand-color-light); }
    &:last-child { border-bottom: none; }

    .mtr-drag {
      display: flex;
      align-items: center;
      color: var(--td-text-color-placeholder);
      cursor: grab;
      &:active { cursor: grabbing; }
    }
    .mtr-name {
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      &.mtr-disabled { color: var(--td-text-color-placeholder); font-weight: 400; }
    }
    .mtr-scope {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--td-text-color-secondary);
    }
    .mtr-fields { font-variant-numeric: tabular-nums; }
    .mtr-builtin {
      font-size: 11px;
      color: var(--td-text-color-placeholder);
      background: var(--td-bg-color-secondarycontainer);
      padding: 2px 8px;
      border-radius: 4px;
    }
    .mtr-switch {
      display: flex;
      align-items: center;
      justify-self: start;
      .t-switch { width: auto; }
    }
    .meter-row-actions {
      display: flex;
      align-items: center;
      gap: 2px;
      justify-content: flex-start;
    }
  }
}

/* 新增/编辑表单（复刻 meter-form） */
.meter-form {
  grid-column: 1 / -1;
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  padding: 14px;
  background: var(--td-bg-color-secondarycontainer);

  &.meter-form--inline {
    margin: 0;
    border-left: none;
    border-right: none;
    border-bottom: none;
    border-radius: 0;
  }

  .meter-form-title {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    color: var(--td-text-color-primary);
  }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
  }

  .form-item {
    label {
      display: block;
      font-size: 12px;
      color: var(--td-text-color-secondary);
      margin-bottom: 4px;
      .required { color: var(--td-error-color); }
    }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }
}

.panel-empty {
  padding: 24px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--td-text-color-placeholder);
  font-size: 13px;
  .panel-empty-icon { opacity: 0.6; }
}

.panel-empty--inline {
  padding: 40px 0;
  background: var(--td-bg-color-container);
  font-size: var(--td-font-size-body-small);
}

/* 列表末尾内联新增证照（与表格列对齐） */
.cert-add-row {
  display: grid;
  grid-template-columns: 30px 1.6fr 0.9fr 0.6fr 0.6fr 0.7fr;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  border-top: 1px solid var(--td-component-stroke);
  cursor: pointer;
  transition: background .15s, color .15s;
  &:hover { background: var(--td-bg-color-secondarycontainer); color: var(--td-brand-color); }
  .cert-add-gap { grid-column: 1; }
  .cert-add-name { grid-column: 2; display: inline-flex; align-items: center; gap: 6px; font-weight: 500; }
  .cert-add-input { grid-column: 2; }
  .cert-add-actions { grid-column: 3 / -1; display: flex; justify-content: flex-end; gap: 8px; cursor: default; }
  &.cert-add-row--editing { cursor: default; background: var(--td-brand-color-light); }
  &.cert-add-row--editing:hover { color: var(--td-text-color-secondary); background: var(--td-brand-color-light); }
}

/* 字段配置区 */
.field-config {
  margin-top: 14px;
  border-top: 1px dashed var(--td-component-stroke);
  padding-top: 12px;

  .field-config-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin-bottom: 8px;

    .field-config-title {
      font-size: 13px;
      font-weight: 600;
      color: var(--td-text-color-primary);
    }

    .field-config-tip {
      font-size: 12px;
      color: var(--td-text-color-placeholder);
    }
  }

  .field-config-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .field-colhead {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) 100px 76px 76px 56px;
    align-items: center;
    gap: 8px;
    padding: 0 8px;
    font-size: var(--td-font-size-body-small);
    color: var(--td-text-color-secondary);
  }
  .fc-h-switch, .fc-h-op, .fc-h-type { text-align: center; white-space: nowrap; }
  .fc-h-name { text-align: left; }

  .field-config-row {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) 100px 76px 76px 56px;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    border: 1px solid var(--td-component-border);
    border-radius: 6px;
    background: var(--td-bg-color-container);

    &.fc-dragging { opacity: 0.5; background: var(--td-brand-color-light); }

    .fc-drag {
      display: flex;
      align-items: center;
      color: var(--td-text-color-placeholder);
      cursor: grab;
      &:active { cursor: grabbing; }
    }

    .fc-name {
      min-width: 0;
    }
    .fc-type {
      width: 100%;
      min-width: 0;
    }

    > *:nth-child(4),
    > *:nth-child(5) { justify-self: center; }
    > *:nth-child(6) { justify-self: center; }

  .fc-used {
      display: inline-flex;
      align-items: center;
      color: var(--td-text-color-placeholder);
      cursor: not-allowed;
    }
  }

  .field-config-add {
    display: flex;
    align-items: center;
    gap: 8px;

    .fc-add-input {
      flex: 1;
      min-width: 0;
    }
  }
}
</style>
