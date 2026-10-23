<template>
  <div class="meter-settings-body" v-if="ready">
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
          <span class="th-name">{{ scopeName(group.scope) }}</span>
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
            <span class="mtr-scope">{{ displayLabel(group) }}</span>
            <span class="mtr-fields">{{ item.fields }}</span>
            <span class="mtr-switch" @click.stop>
              <t-switch :model-value="!!item.enabled" size="small" @change="(v: any) => toggleEnabled(item, v)" />
            </span>
            <span class="meter-row-actions" @click.stop>
              <t-popconfirm theme="warning" :visible="delItemPop?.id === item.id" placement="left"
                :content="`确定删除分类「${item.name}」吗？该分类的字段配置与提取规则将一并清除，且不可恢复。`"
                :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
                @confirm="onItemPopRemove" @cancel="delItemPop = null"
                @visible-change="(v: boolean) => { if (!v) delItemPop = null }">
                <t-dropdown :options="itemMenuOptions(item)" placement="bottom-right" min-column-width="120px"
                  @click="(ctx: any) => onItemMenu(item, ctx.value)">
                  <t-button variant="text" size="small" shape="square">
                    <template #icon><t-icon name="more" size="16px" /></template>
                  </t-button>
                </t-dropdown>
              </t-popconfirm>
            </span>
          </div>
          <!-- 行内展开编辑（复刻 meter-form--inline） -->
          <div v-if="editingId === item.id" class="meter-form meter-form--inline">
            <div class="meter-form-title">编辑{{ group.label }}</div>
            <div class="form-grid">
              <div class="form-item">
                <label>{{ scopeName(group.scope) }} <span class="required">*</span></label>
                <!-- AI 图标按钮：与 ⋮ 菜单「字段生成」同一功能，点击展开/再点收起 -->
                <div class="form-item-inline">
                  <t-input v-model="editForm.name" @enter="commitEdit" />
                  <t-tooltip :content="aiPanelOpen ? '收起字段生成' : '用样票生成字段'" placement="top">
                    <t-button class="ai-toggle-btn" :class="{ 'ai-toggle-btn--on': aiPanelOpen }"
                      variant="outline" shape="square" size="small" @click="toggleAiPanel">
                      <template #icon><AiSparkIcon size="16px" /></template>
                    </t-button>
                  </t-tooltip>
                </div>
              </div>
            </div>
            <!-- 字段生成：内嵌在「发票类型」下方、「字段配置」上方，
                 上传样票→识别→勾选→保存字段，直接落库。 -->
            <AiFieldSuggestPanel
              v-model:visible="aiPanelOpen"
              :kb-id="props.kbId || ''"
              :scope="group.scope"
              :cert-type="editingCertName"
              @apply="onAiFieldApply"
            />
            <!-- 字段配置：增删改 + 启用/禁用（同步字段筛选器） -->
            <div class="field-config">
              <div class="field-config-head">
                <span class="field-config-title">字段配置</span>
                <span class="field-config-tip">启用的字段在字段筛选器与列表显示，禁用后隐藏</span>
              </div>
              <!-- 业务分组切换（仅带 group 的分类显示，如市电账单的 12 个费用分项）。
                   没有 group 的分类（发票/合同/制度/公告）不渲染，视觉与改造前一致。 -->
              <div v-if="subGroupTabs.length > 1" class="fc-group-tabs">
                <span v-for="g in subGroupTabs" :key="g.key" class="fc-group-tab"
                  :class="{ active: activeSubGroup === g.key }" @click="switchSubGroup(g.key)">
                  {{ g.label }}
                  <span class="fc-group-tab__count">{{ g.count }}</span>
                </span>
              </div>
              <div class="field-config-list">
                <div class="field-colhead" :class="{ 'has-width': showWidthCol }">
                  <span class="fc-h-drag"></span>
                  <span class="fc-h-name">字段名</span>
                  <span class="fc-h-type">数据类型</span>
                  <span v-if="showWidthCol" class="fc-h-width">宽度(px)</span>
                  <span class="fc-h-switch">默认表头</span>
                  <span class="fc-h-switch">启用字段</span>
                  <span class="fc-h-op">操作</span>
                </div>
                <!-- 明细子项不再是独立的底部区块：点「项目明细」字段行就在它正下方展开，
                     子项行与主字段行共用同一套网格/行高/圆角/间距（.field-config-row）。 -->
                <template v-for="fd in fieldsVisible" :key="fd.name">
                  <div class="field-config-row"
                    :class="[{ 'has-width': showWidthCol }, { 'fc-dragging': fieldDragName === fd.name }]"
                    draggable="true" @dragstart="onFieldDragStart(fd)" @dragover.prevent
                    @drop.prevent="onFieldDrop(fd)" @dragend="onFieldDragEnd">
                    <span class="fc-drag" title="拖动排序"><t-icon name="move" size="14px" /></span>
                    <!-- items 字段：字段名即展开开关，点击在下方展开子项 -->
                    <button v-if="fd.dataType === 'items'" type="button" class="fc-name fc-name-toggle"
                      :title="`${fd.label || fieldLabelOf(fd)} · 点击${isSubFieldsOpen(fd) ? '收起' : '展开'}明细子项`"
                      @click="toggleSubFields(fd)">
                      <span class="fc-name-text">{{ fieldLabelOf(fd) }}</span>
                      <span class="fc-sub-count">{{ subFieldsOf(fd).length }}</span>
                    </button>
                    <!-- 字段名可编辑（2026-10-21）：复制/新增字段中文命名后可直接改名。
                         改名只写 label（表头文案），name 是数据身份（契约键/后端 key）不动——
                         ① 历史数据与提取映射按 name 对齐，改 name 会错位；
                         ② v-for 以 name 为 key，name 一变输入框每敲一键就重挂、丢焦点。
                         费用分组里「费用组成 / 分时时段」右侧另有细项展开按钮：
                         它们的下级不是子字段而是"行项集合"，单独渲染在下方。 -->
                    <div v-else class="fc-name-wrap">
                      <t-input :model-value="fieldEditOf(fd)" size="small" placeholder="字段名" class="fc-name"
                        :ref="(el: any) => setFieldInputRef(fd.name, el)"
                        @update:model-value="(v: string) => setFieldEdit(fd, v)" />
                      <button v-if="childGroupOf(fd)" type="button" class="fc-child-toggle"
                        :class="{ open: openChildGroup === childGroupOf(fd) }"
                        :title="`${fieldLabelOf(fd)} · 点击${openChildGroup === childGroupOf(fd) ? '收起' : '展开'}细项配置`"
                        @click="toggleChildGroup(fd)">
                        <t-icon :name="openChildGroup === childGroupOf(fd) ? 'chevron-up' : 'chevron-down'" size="14px" />
                        <span>{{ childItemsOf(childGroupOf(fd)).length }}</span>
                      </button>
                    </div>
                    <t-select v-model="fd.dataType" size="small" class="fc-type" :options="dataTypeOptions" />
                    <t-tooltip v-if="showWidthCol" content="0=按内容自适应，填写像素可固定列宽" placement="top">
                      <t-input-number v-model="fd.width" size="small" class="fc-width" :min="0" :max="400" placeholder="0=自适应" theme="normal" />
                    </t-tooltip>
                    <!-- 明细行不作为普通文本列平铺在主表头里，「默认表头」开关对它没有意义 -->
                    <span v-if="fd.dataType === 'items'" class="sub-blank"></span>
                    <t-tooltip v-else content="默认字段：重置字段筛选器时自动勾选" placement="top">
                      <t-switch :model-value="!!fd.isDefault" size="small" @change="(v: any) => (fd.isDefault = !!v)" />
                    </t-tooltip>
                    <t-switch :model-value="!!fd.enabled" size="small" @change="(v: any) => (fd.enabled = !!v)" />
                    <template v-if="isBuiltinField(fd)">
                      <t-tooltip content="系统内置字段，不可删除" placement="top">
                        <span class="fc-used"><t-icon name="lock-on" size="15px" /></span>
                      </t-tooltip>
                    </template>
                    <template v-else-if="usedFields.has(fd.name)">
                      <t-tooltip :content="`该字段已被证照记录引用，不能删除`" placement="top">
                        <span class="fc-used"><t-icon name="lock-on" size="15px" /></span>
                      </t-tooltip>
                    </template>
                    <t-popconfirm v-else theme="warning" :visible="fieldDelPop?.name === fd.name" placement="left"
                      :content="`确定删除字段「${fieldLabelOf(fd)}」吗？该字段将不可恢复。`"
                      :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
                      @confirm="onFieldPopRemove" @cancel="fieldDelPop = null"
                      @visible-change="(v: boolean) => { if (!v) fieldDelPop = null }">
                      <t-dropdown :options="fieldMenuOptions(fd)" placement="bottom-right" min-column-width="110px"
                        @click="(ctx: any) => onFieldMenu(fd, ctx.value)">
                        <t-button variant="text" size="small" shape="square">
                          <template #icon><t-icon name="more" size="15px" /></template>
                        </t-button>
                      </t-dropdown>
                    </t-popconfirm>
                  </div>

                  <!-- 明细子项内联展开区：紧贴 items 字段行，复用主字段行网格 -->
                  <div v-if="fd.dataType === 'items' && isSubFieldsOpen(fd)" class="sub-inline">
                    <div
                      v-for="(sf, si) in subFieldsOf(fd)" :key="sf.key"
                      class="field-config-row sub-row" :class="{ 'has-width': showWidthCol }">
                      <span class="fc-drag sub-drag" :title="`子项编码：${sf.key}`">{{ si + 1 }}</span>
                      <!-- 子项隐藏 key 输入：key 是数据对齐口径，新建时由系统按中文名生成 -->
                      <t-input v-model="sf.label" size="small" class="fc-name" :title="`子项编码：${sf.key}`"
                        placeholder="子项名称" />
                      <t-select v-model="sf.dataType" size="small" class="fc-type" :options="subDataTypeOptions" />
                      <t-tooltip v-if="showWidthCol" content="0=按内容自适应，填写像素可固定列宽" placement="top">
                        <t-input-number v-model="sf.width" size="small" class="fc-width" :min="0" :max="400"
                          placeholder="0=自适应" theme="normal" />
                      </t-tooltip>
                      <span class="sub-blank"></span>
                      <t-switch :model-value="subFieldEnabled(sf)" size="small"
                        @change="(v: any) => (sf.enabled = !!v)" />
                      <t-popconfirm theme="warning" placement="left"
                        :content="`确认删除子项「${sf.label || sf.key}」吗？`"
                        :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }"
                        @confirm="removeSubField(fd, sf)">
                        <t-tooltip content="删除子项" placement="top">
                          <t-button variant="text" size="small" shape="square" theme="danger">
                            <template #icon><t-icon name="delete" size="15px" /></template>
                          </t-button>
                        </t-tooltip>
                      </t-popconfirm>
                    </div>
                    <div class="field-config-add sub-inline-add">
                      <t-input v-model="newSubFieldLabel" size="small" class="fc-add-input"
                        placeholder="新增子项名称（如 规格型号）" @enter="addSubField(fd)" />
                      <t-button variant="outline" size="small" @click="addSubField(fd)">添加</t-button>
                    </div>
                  </div>

                  <!-- 细项展开区（费用组成 / 分时时段）：
                       费用组成的细项 = `<分组>-rows`（如 market-rows：零售交易电费(尖峰)、偏差电费(平)…）；
                       分时时段的细项 = `<分组>-periods`（尖 / 峰 / 平 / 谷）。
                       这些是「表格行」而不是「字段」，所以不做成独立字段行，而是挂在
                       父字段下展开 —— 否则字段定义表会被 22 条行项淹没。
                       编辑只改 label（显示名），name 是账单 row 的匹配键，改了就对不上
                       fee_items 的 name/period。 -->
                  <div v-if="childGroupOf(fd) && openChildGroup === childGroupOf(fd)" class="sub-inline">
                    <div v-if="!childItemsOf(childGroupOf(fd)).length" class="fc-child-empty">
                      暂无细项，可在下方添加（如「零售交易电费(平)」或「尖」）。
                    </div>
                    <!-- 细项行用紧凑 4 列（序号 / 名称 / 启用 / 菜单）：
                         设置抽屉内容区约 450px，塞不下主字段那套 7 列网格，
                         硬塞的结果是操作列被裁掉（真机截图右侧只剩几个点）。
                         细项不需要「数据类型/宽度/默认表头」——它们只是行标题。 -->
                    <div v-for="(ch, ci) in childItemsOf(childGroupOf(fd))" :key="ch.name"
                      class="field-config-row child-row">
                      <span class="fc-drag sub-drag" :title="`细项键：${ch.name}`">{{ ci + 1 }}</span>
                      <t-input :model-value="ch.label || ch.name" size="small" class="fc-name"
                        placeholder="细项名称" :title="`细项键：${ch.name}`"
                        @update:model-value="(v: string) => renameChild(ch, v)" />
                      <t-tooltip content="默认显示：子表默认展示该行" placement="top">
                        <t-switch :model-value="!!ch.isDefault" size="small"
                          @change="(v: any) => (ch.isDefault = !!v)" />
                      </t-tooltip>
                      <t-tooltip content="启用 / 停用该细项" placement="top">
                        <t-switch :model-value="!!ch.enabled" size="small"
                          @change="(v: any) => (ch.enabled = !!v)" />
                      </t-tooltip>
                      <span class="fc-child-ops">
                        <t-dropdown :options="childMenuOptions(ch, ci, childItemsOf(childGroupOf(fd)).length)"
                          placement="bottom-right" min-column-width="110px"
                          @click="(ctx: any) => onChildMenu(ch, ctx.value)">
                          <t-button variant="text" size="small" shape="square" title="细项操作">
                            <template #icon><t-icon name="more" size="15px" /></template>
                          </t-button>
                        </t-dropdown>
                      </span>
                    </div>
                    <div class="field-config-add sub-inline-add">
                      <t-input v-model="newChildLabel" size="small" class="fc-add-input"
                        :placeholder="childPlaceholderOf(fd)" @enter="addChild(fd)" />
                      <t-button variant="outline" size="small" @click="addChild(fd)">添加细项</t-button>
                    </div>
                  </div>
                </template>
                <div class="field-config-add">
                  <t-input v-model="newFieldName" size="small" placeholder="新增字段名" class="fc-add-input"
                    @enter="addField" />
                  <t-button variant="outline" size="small" @click="addField">添加</t-button>
                </div>
              </div>
            </div>
            <div class="meter-form-actions">
              <div class="spacer" />
              <t-button variant="outline" size="small" @click="editingId = ''">取消</t-button>
              <t-button theme="primary" size="small" :loading="saving" @click="commitEdit">保存</t-button>
            </div>
          </div>
        </template>

        <!-- 空状态：表头始终在顶部，图标与简化说明置于表头下方 -->
        <div v-if="!groupItems.length && !addVisible" class="panel-empty panel-empty--inline">
          <t-icon name="folder-open" size="26px" class="panel-empty-icon" />
          <span class="panel-empty-text">{{ emptyTextOf(group.scope) }}</span>
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
          <span class="cert-add-name"><t-icon name="add" size="14px" /><span>{{ addRowLabel }}</span></span>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, watch, nextTick, h } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { CopyIcon, DeleteIcon } from 'tdesign-icons-vue-next'
import { useExtractCatalog } from './useExtractCatalog'
import { createFleetCategory, updateFleetCategory, deleteFleetCategory, sortFleetCategories, listFleetRecords, listFleetCertGroups, createFleetCertGroup, deleteFleetCertGroup, listFleetGroupAliases, upsertFleetGroupAlias, getExtractConfig, saveExtractConfig } from '@/api/fleet'
import { getScope, labelOf, isBuiltinFieldOf } from '../../registry'
import {
  fromSubsPayload,
  toSubsPayload,
  subsPayloadFromNames,
  copyFieldUniqueName,
  subFieldEnabled,
  subFieldKeyForLabel,
  findDuplicateSubField,
  type EditableField,
  type EditableSubField,
} from './fleetCategorySubs'
import { listSubGroups } from './sixWaySync'
import AiFieldSuggestPanel from './AiFieldSuggestPanel.vue'
import AiSparkIcon from './AiSparkIcon.vue'
import { mergeAiFieldSuggestions } from './aiFieldMerge'
import { cacheSampleText } from './sampleTextCache'

const props = withDefaults(defineProps<{ scope?: 'vehicle' | 'driver' | 'maintain' | 'invoice' | 'contract' | 'regulation' | 'award_punish' | 'energy' | ''; kbId?: string }>(), { scope: 'vehicle', kbId: '' })
// 列宽配置仅业务档案 scope 显示（车队 scope 暂不接入列宽消费）
const showWidthCol = computed(() => props.scope !== 'vehicle' && props.scope !== 'driver' && props.scope !== 'maintain')

// 分组：车辆档案=公司证照；司机档案=司机证照；维保档案=维保文件；发票/合同等业务 scope=单分类
//
// energy 必须显式声明：缺了会命中下面的 `GROUPS[props.scope] || GROUPS.vehicle` 兜底，
// 于是能耗设置抽屉里显示「公司证照」——真机踩过的坑。
const GROUPS: Record<string, { key: string; label: string; scope: string; groupId: string }[]> = {
  vehicle: [
    { key: 'company', label: '公司证照', scope: 'vehicle', groupId: '' },
  ],
  driver: [{ key: 'driver', label: '司机证照', scope: 'driver', groupId: '' }],
  maintain: [{ key: 'maintain', label: '维保清单', scope: 'maintain', groupId: '' }],
  invoice: [{ key: 'invoice', label: '发票', scope: 'invoice', groupId: '' }],
  contract: [{ key: 'contract', label: '合同', scope: 'contract', groupId: '' }],
  regulation: [{ key: 'regulation', label: '制度', scope: 'regulation', groupId: '' }],
  award_punish: [{ key: 'award_punish', label: '通知', scope: 'award_punish', groupId: '' }],
  // 能耗：市电账单 / 光伏账单 / 能耗单 三个分类同属 energy scope（见后端
  // handler/energy_extract_config.go 的 EnergyScope 常量与 EnergyBillType* 定义）
  energy: [{ key: 'energy', label: '能耗账单', scope: 'energy', groupId: '' }],
}

// 分类名称列文案：统一走 BusinessScopeRegistry（新增业务只改 registry/<scope>.ts）
function scopeName(scope: string) {
  return getScope(scope)?.categoryLabel || '证照名称'
}
const emptyTextOf = (scope: string) => getScope(scope)?.emptyText || '暂无证照类型'
const namePlaceholderOf = (scope: string) => getScope(scope)?.namePlaceholder || '请输入证照名称'
const categoryNounOf = (scope: string) => getScope(scope)?.categoryNoun || '证照类型'
const customGroups = ref<any[]>([])
async function loadCustomGroups() {
  try {
    const parentScope = props.scope
    const res: any = await listFleetCertGroups({ parent_scope: parentScope })
    const arr = Array.isArray(res?.data) ? res.data : (Array.isArray(res) ? res : [])
    const fallbackScope = props.scope || 'vehicle'
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
    const parentScope = props.scope
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
// 分类字典缓存（30s TTL + 并发去重），避免每次打开设置抽屉重复拉取。
const catalog = useExtractCatalog()
const saving = ref(false)

const group = computed(() => groupList.value.find((g) => g.key === activeGroup.value) || groupList.value[0])

// 分组重命名（纯显示别名，localStorage 持久化；不影响数据关联）
const groupAliases = ref<Record<string, string>>({})
const ready = ref(false)
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

// energy scope 下并存两类分类，是两条不同的轴：
//
//	① 能耗**类型**（工业用电/生活用水/燃气费/供热费，builtin_key=energy-*）
//	   —— 供「能耗台账」上传弹窗的类型下拉用，值最终写进 utility_type 字段；
//	② 账单**类型**（市电账单/光伏账单/能耗单，builtin_key 为空）
//	   —— 字段定义与提取规则挂在这三个上，是设置抽屉要管的对象。
//
// 设置抽屉只展示 ②：类型已经是一个可配置字段（energyCatalog 的 utility_type 带
// desc/rule/regex），再做成分类会让用户在两个地方改同一件事。
const ENERGY_TYPE_SEED_KEYS = new Set([
  'energy-electricity', 'energy-water', 'energy-gas', 'energy-heat',
])

function buildItems(g0: any): any[] {
  const scope: string = g0.scope
  const gid: string = g0.groupId || ''
  // 仅取归属当前分组的分类（内置分组 group_id 为空；自定义分组按 group_id 过滤）
  const list = (categories.value[scope] || [])
    .filter((c: any) => (c.group_id || '') === gid)
    .filter((c: any) => !(scope === 'energy' && ENERGY_TYPE_SEED_KEYS.has(String(c.builtin_key || ''))))
  const merged: any[] = []
  const seen = new Set<string>()
  // 1) 已入库分类：严格按 categories 后端顺序（sort_order，用户拖动后持久化）
  list.forEach((c: any) => {
    const isBuiltin = !!c.builtin_key
    if (!seen.has(c.id)) {
      merged.push({ ...c, builtin: isBuiltin, fields: (Array.isArray(c.subs) && c.subs.length) ? c.subs.length : (isBuiltin ? certFieldCount({ name: c.builtin_key } as any) : 0) })
      seen.add(c.id)
    }
  })
  // 2) 内置未入库兜底（理论上初始化会写入全部内置，这里仅防御）：按内置顺序追加
  if (!gid) {
    (BUILTIN[scope] || []).forEach((b) => {
      const found = list.find((c: any) => c.builtin_key === b.name || c.name === b.name)
      if (!found) {
        merged.push({ id: `builtin-${b.name}`, name: b.name, enabled: true, subs: [], builtin: true, fields: certFieldCount(b) })
      }
    })
  }
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
  const scope = group.value.scope
  const gid = group.value.groupId || ''
  // 乐观更新：当前分组项按新顺序填入它们在 categories 数组中原来占据的 slot
  const arr = [...(categories.value[scope] || [])]
  const newGroupItems = list.filter((it: any) => !String(it.id).startsWith('builtin-'))
  let gi = 0
  const prev = arr
  categories.value[scope] = arr.map((c0: any) => ((c0.group_id || '') === gid ? (newGroupItems[gi++] ?? c0) : c0)) as any
  // 已入库项同步后端 sort_order
  const ids = newGroupItems.map((it: any) => it.id)
  if (!ids.length) return
  try {
    await sortFleetCategories(scope, ids)
    MessagePlugin.success('顺序已保存')
    notifyCategoriesChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '排序保存失败')
    void prev
    load()
  }
}

// ---- 字段级拖拽排序（仅调整编辑态 fieldsEditable 顺序，保存时随 subs 持久化）----
// 字段拖拽排序：按**字段身份**定位，不按下标。
//
// 为什么不能用下标：分组过滤后模板里的 i 是「当前分组内的序号」，
// 而真实顺序在扁平化的 fieldsEditable 里。用下标会把字段挪到别的分组去，
// 表现为「拖一下顺序，字段跑到隔壁分组了」。按 name 定位可避免。
const fieldDragName = ref('')
function onFieldDragStart(fd: any) { fieldDragName.value = String(fd?.name || '') }
function onFieldDragEnd() { fieldDragName.value = '' }
function onFieldDrop(target: any) {
  const fromName = fieldDragName.value
  fieldDragName.value = ''
  const toName = String(target?.name || '')
  if (!fromName || !toName || fromName === toName) return
  const list = [...fieldsEditable.value]
  const from = list.findIndex((f: any) => f.name === fromName)
  const to = list.findIndex((f: any) => f.name === toName)
  if (from < 0 || to < 0) return
  const [moved] = list.splice(from, 1)
  list.splice(to, 0, moved)
  fieldsEditable.value = list
}

// 分类变更后通知列表页刷新证照类型选项（上传弹窗 / 筛选框依赖 categories）
function notifyCategoriesChanged() {
  window.dispatchEvent(new CustomEvent('fleet-categories-changed'))
}

// 注意：这里曾经有一次性的 `migrateDefaultFlags`——在一次 GET 分类列表的副作用里
// 静默写库（给内置 base 字段打 is_default=true）。它把"读"变成了"写"，且迁移结果
// 依赖 localStorage 标记，换设备/清缓存就会重跑一遍写库。已移除：默认字段以用户
// 在字段配置里显式勾选为准（蓝图 P1-8）。

async function load() {
  try {
    const scopes = [...new Set(groupList.value.map((g) => g.scope))]
    // 走 useExtractCatalog：30s TTL + 同 scope 并发去重。
    // 打开设置抽屉平均要为 1~3 个 scope 各拉一次分类，跨区 pooler 下每次往返
    // 约 350ms，缓存后重复打开基本零请求。
    const resList = await Promise.all(scopes.map((s) => catalog.list(s)))
    scopes.forEach((s, i) => {
      const list = resList[i] || []
      categories.value[s] = list
      usedFields.value = new Set()
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
onMounted(async () => { await Promise.all([load(), loadCustomGroups(), loadGroupAliases()]); ready.value = true })
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
// 列表末尾「新增」行文案：registry 声明了 addLabel 的 scope（发票=「新增分类」）优先
const addLabel = computed(() => getScope(props.scope)?.addLabel || `新增${group.value.label}`)
const addRowLabel = computed(() => getScope(props.scope)?.addLabel || `新增${group.value.label}`)
defineExpose({ startAdd, addLabel })
async function commitAdd() {
  const name = addForm.name.trim()
  if (!name) { MessagePlugin.warning(namePlaceholderOf(props.scope)); return }
  if (groupItems.value.some((it: any) => it.name === name)) { MessagePlugin.warning(`该${categoryNounOf(props.scope)}已存在`); return }
  saving.value = true
  try {
    // enabled 显式传 true：新分类默认启用，避免后端缺省关闭导致抽屉重开后被 enabled 过滤而“消失”
    const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', name, subs: [], enabled: true })
    const created = res.data || res
    if (created?.id) {
      // 兜底：确保新记录带上当前分组 group_id，避免后端响应未回填时短暂错归到内置分组
      const record = { ...created, group_id: created.group_id || group.value.groupId || '' }
      categories.value[group.value.scope] = [...(categories.value[group.value.scope] || []), record]
      addVisible.value = false
      MessagePlugin.success('证照类型已创建')
      notifyCategoriesChanged()
      // 连贯工作流：新建即展开编辑并打开「字段生成」，引导用户上传样票一键配齐字段
      await nextTick()
      const createdItem = groupItems.value.find((it: any) => it.id === record.id)
      if (createdItem) openAiPanel(createdItem)
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
// 字段编辑态一律经 fleetCategorySubs 的纯函数与后端 subs 互转：
// 早前这里有三份内联 map（toggleEdit 读 / commitEdit 写 / duplicateItem 复制），
// 三份都漏搬 sub_fields，导致「字段定义」保存一次就把明细子列抹空。
const fieldsEditable = ref<EditableField[]>([])
const newFieldName = ref('')
const newSubFieldLabel = ref('')

// ---- 业务分组切换（能耗类 scope：市电账单下 12 个费用分项，共 163 个字段）----
//
// `activeSubGroup === ''` 表示「全部」。无 group 的分类（发票/合同/制度/公告）
// 枚举结果为空 → 胶囊条不渲染 → 行为与改造前完全一致。
const activeSubGroup = ref('')
const subGroupTabs = computed(() =>
  // `-rows` 是子表行标题（非提取字段），不进字段定义的分组胶囊条
  listSubGroups(fieldsEditable.value as any[], (k: string) => subGroupLabelOf(k))
    // -rows / -periods 是细项集合，挂在「费用组成 / 分时时段」下展开，不单独出胶囊
    .filter((g) => !g.key.endsWith('-rows') && !g.key.endsWith('-periods')),
)
/** 当前分组下可见的字段（模板里 v-for 的源）。 */
const fieldsVisible = computed(() => {
  if (!activeSubGroup.value) return fieldsEditable.value
  return fieldsEditable.value.filter((f: any) => String(f.group ?? '').trim() === activeSubGroup.value)
})
function subGroupLabelOf(key: string): string {
  const hit = fieldsEditable.value.find((f: any) => String(f.group ?? '').trim() === key)
  return String((hit as any)?.groupLabel ?? '').trim() || key
}
/**
 * 载入分类后自动选中第一个分组。
 *
 * 为什么必须有：activeSubGroup 为空时 fieldsVisible 不过滤，市电账单会把
 * 176 条 subs（20 概况 + 5×7 分项 + 22 行细项 + 28 时段…）一次性平铺出来，
 * 用户看到一屏滚动条却找不到重点；而胶囊条又没有"全部"项，等于停在非法态。
 */
function resetSubGroupSelection() {
  const groups = listSubGroups(fieldsEditable.value as any[], (k: string) => subGroupLabelOf(k))
    .filter((g) => !g.key.endsWith('-rows') && !g.key.endsWith('-periods'))
  activeSubGroup.value = groups[0]?.key || ''
  openChildGroup.value = ''
  openSubFieldName.value = ''
}

function switchSubGroup(key: string) {
  activeSubGroup.value = key
  openSubFieldName.value = ''
  openChildGroup.value = ''
}

// ---- 细项配置（费用组成 / 分时时段）----
//
// 数据结构：细项仍是 subs 条目，只是 group 指向 `<分组>-rows` / `<分组>-periods`。
// 这样不需要新表、也不需要新接口——保存分类时随 subs 一起持久化。
//
// 为什么不把细项做成独立字段行：市电账单的 market-rows 有 22 条，
// 平铺在字段定义表里会把 5 个列字段彻底淹没，用户根本找不到「计费电量」。
const openChildGroup = ref('')
const newChildLabel = ref('')

/** 该字段是否有下级细项；有则返回细项所属 group 键。 */
function childGroupOf(fd: any): string {
  if (!activeSubGroup.value) return ''
  const n = String(fd?.name || '').trim()
  // 费用组成 = 该分组的行项集合；分时时段 = 该分组的时段集合
  if (n === 'name') return `${activeSubGroup.value}-rows`
  if (n === 'period') return `${activeSubGroup.value}-periods`
  return ''
}

function childItemsOf(groupKey: string): EditableField[] {
  if (!groupKey) return []
  return fieldsEditable.value.filter((f: any) => String(f.group ?? '').trim() === groupKey)
}

function toggleChildGroup(fd: any) {
  const g = childGroupOf(fd)
  openChildGroup.value = openChildGroup.value === g ? '' : g
  openSubFieldName.value = ''
  newChildLabel.value = ''
}

function childPlaceholderOf(fd: any): string {
  return String(fd?.name || '') === 'period'
    ? '新增时段（如 尖 / 峰 / 平 / 谷）'
    : '新增细项（如 零售交易电费(平)）'
}

function addChild(fd: any) {
  const g = childGroupOf(fd)
  if (!g) return
  const label = newChildLabel.value.trim()
  if (!label) { MessagePlugin.warning('请输入细项名称'); return }
  const siblings = childItemsOf(g)
  if (siblings.some((c: any) => String(c.name).trim() === label || String(c.label || '').trim() === label)) {
    MessagePlugin.warning('细项已存在')
    return
  }
  // name 是账单行匹配键，可直接用中文（与既有 market-rows 条目一致）；
  // label 与 name 相同，后续改名只改 label。
  fieldsEditable.value.push({
    name: label,
    label,
    enabled: true,
    isDefault: true,
    dataType: 'text',
    width: 0,
    group: g,
    groupLabel: `${subGroupLabelOf(activeSubGroup.value)} · 细项`,
  } as any)
  newChildLabel.value = ''
}

/** 细项改名只动 label：name 是 fee_items 的 name/period 匹配键，改了行就取不到数。 */
function renameChild(ch: any, v: string) {
  ch.label = v
}

function removeChild(ch: any) {
  fieldsEditable.value = fieldsEditable.value.filter((f: any) => f !== ch)
}

/** 细项行操作菜单：上移/下移在端点置灰，删除单独确认。 */
function childMenuOptions(ch: any, index: number, total: number) {
  return [
    { content: '上移', value: 'up', disabled: index === 0 },
    { content: '下移', value: 'down', disabled: index >= total - 1 },
    { content: '删除细项', value: 'del' },
  ]
}
function onChildMenu(ch: any, value: string) {
  if (value === 'up') moveChild(ch, -1)
  else if (value === 'down') moveChild(ch, 1)
  else if (value === 'del') removeChild(ch)
}

function moveChild(ch: any, delta: number) {
  const g = String(ch?.group ?? '').trim()
  const list = [...fieldsEditable.value]
  const groupIdx: number[] = []
  list.forEach((f: any, i) => { if (String(f.group ?? '').trim() === g) groupIdx.push(i) })
  const pos = groupIdx.findIndex((i) => list[i] === ch)
  const target = pos + delta
  if (pos < 0 || target < 0 || target >= groupIdx.length) return
  const from = groupIdx[pos]
  const to = groupIdx[target]
  const [moved] = list.splice(from, 1)
  list.splice(to, 0, moved)
  fieldsEditable.value = list
}

// ---- 明细子项：点「项目明细」字段行在其正下方内联展开 ----
const openSubFieldName = ref('')
function isSubFieldsOpen(fd: EditableField) {
  return openSubFieldName.value === fd.name
}
function toggleSubFields(fd: EditableField) {
  openSubFieldName.value = isSubFieldsOpen(fd) ? '' : fd.name
}
function subFieldsOf(fd: EditableField): EditableSubField[] {
  return fd.subFields || []
}
// 子项可选类型不含「明细」——明细行里再套明细是无效结构
const subDataTypeOptions = [
  { label: '单行文本', value: 'text' },
  { label: '多行文本', value: 'textarea' },
  { label: '数字', value: 'number' },
  { label: '日期', value: 'date' },
]
/**
 * 系统内置字段**只有 3 个**：发票类型 / 提取状态 / 标签。
 *
 * 它们是列表渲染、筛选器与六同步的硬依赖（少一列整条链路就静默失效）；
 * 其余标准字段（发票号码/金额/明细…）、扩展字段、明细子项一律允许用户删除。
 * 用户口径明确：上一轮"17 个字段全锁"过宽，导致删不掉任何标准列。
 */
function isBuiltinField(fd: EditableField) {
  return isBuiltinFieldOf(group.value.scope, String(fd.name || ''))
}

/** 新增子项：只填中文名，key 由系统按中文名映射或递增生成。 */
function addSubField(fd: EditableField) {
  const label = newSubFieldLabel.value.trim()
  if (!label) { MessagePlugin.warning('请输入子项名称'); return }
  const subs = fd.subFields || (fd.subFields = [])
  const key = subFieldKeyForLabel(label, subs.map((s) => s.key))
  // 去重：同义键（税额 tax / tax_amount）与同列头都算重复，避免浮层出现两列同名
  const dup = findDuplicateSubField(subs, { key, label })
  if (dup) {
    MessagePlugin.warning(`子项「${dup.label || dup.key}」已存在（与「${label}」同义），无需重复添加`)
    return
  }
  subs.push({ key, label, dataType: 'text', width: 0 })
  newSubFieldLabel.value = ''
}
/**
 * 删除明细子项。
 *
 * 之前这里调用了一个已被删除的 `isBuiltinSubField`（上一轮取消子项锁定后忘了同步），
 * 每次点击都会抛 ReferenceError → 弹窗关掉了但子项还在，表现就是「删不掉」。
 * 现在按「引用相等 ∨ key 相等」移除，双保险：v-for 传进来的对象一定在数组里，
 * 但若上游克隆过数组，key 兜底仍然生效。
 */
function removeSubField(fd: EditableField, sf: EditableSubField) {
  if (!fd.subFields?.length) return
  const key = String(sf?.key || '')
  const next = fd.subFields.filter((s) => s !== sf && String(s?.key || '') !== key)
  if (next.length === fd.subFields.length) return
  fd.subFields = next
}
// 类型选项：文本细分为「单行文本 / 多行文本」（2026-10-22）。
// 单行文本 = 常规输入框，禁换行，适合短内容（编号/名称/金额…）；
// 多行文本 = 多行编辑区域，支持换行与自动折行，适合备注/摘要/说明等长内容，
// 详情抽屉、列表与打印导出均完整展示不截断。
// 两者存储格式一致（都是 string），只影响渲染控件与列表折行策略。
const dataTypeOptions = [
  { label: '单行文本', value: 'text' },
  { label: '多行文本', value: 'textarea' },
  { label: '数字', value: 'number' },
  { label: '日期', value: 'date' },
  { label: '数组', value: 'array' },
  { label: '明细', value: 'items' },
]
function toggleEdit(item: any) {
  if (editingId.value === item.id) { editingId.value = ''; usedFields.value = new Set(); return }
  editForm.name = item.name
  newFieldName.value = ''
  newSubFieldLabel.value = ''
  openSubFieldName.value = ''
  // 换分类编辑时先收起 AI 面板：它属于"上一行"的上下文
  aiPanelOpen.value = false
  // 字段来源：已入库 subs（含 sub_fields）优先，否则内置 base+detail 底稿
  const b = (BUILTIN[group.value.scope] || []).find((x: any) => x.name === item.name)
  const raw = Array.isArray(item.subs) && item.subs.length
    ? item.subs
    : subsPayloadFromNames([...(b?.base || []), ...(b?.detail || [])])
  fieldsEditable.value = fromSubsPayload(raw)
  editingId.value = item.id
  editingCertName.value = item.name
  resetSubGroupSelection()
  loadUsedFields(group.value.scope, item.name)
}
// ---- 字段行操作：竖向三点菜单（复制 / 删除，图标+文字）；删除为受控气泡，中文化文案 ----
const fieldDelPop = ref<any>(null)
// 字段名展示：自定义 label → registry 字典中文名 → 无字典 scope 回退 desc/原名
const fieldLabelOf = (fd: any) => (fd?.label || labelOf(group.value.scope, fd.name) || fd.desc || fd.name)
// 字段名输入框的显示值：与 fieldLabelOf 同源（label 优先，契约键回退字典中文名）
const fieldEditOf = (fd: any) => String(fd?.label || labelOf(group.value.scope, fd?.name || '') || fd?.name || '')
/**
 * 字段改名：只写 label（表头文案），不动 name。
 * name 是数据身份（契约键 / 后端分配 key），历史数据与提取映射都按 name 对齐；
 * 且字段行 v-for 以 name 为 key，改 name 会触发整行重挂、输入框丢焦点。
 */
function setFieldEdit(fd: any, v: string) {
  fd.label = String(v ?? '')
}
// 字段名输入框 ref 注册表：复制字段后按 name 找到新行输入框并聚焦
const fieldInputRefs = new Map<string, any>()
function setFieldInputRef(name: string, el: any) {
  if (el) fieldInputRefs.set(name, el)
  else fieldInputRefs.delete(name)
}
const fieldMenuOptions = (_fd: any) => [
  { content: '复制', value: 'copy', prefixIcon: () => h(CopyIcon, { size: '14px' }) },
  { content: '删除', value: 'delete', theme: 'error', prefixIcon: () => h(DeleteIcon, { size: '14px' }) },
]
function onFieldMenu(fd: any, value: string) {
  if (value === 'copy') void duplicateField(fd)
  else if (value === 'delete') fieldDelPop.value = fd
}
const onFieldPopRemove = async () => {
  const fd = fieldDelPop.value
  fieldDelPop.value = null
  if (fd) await removeField(fd)
}
// 复制字段：字段配置全量复制为新字段（含明细子列），提取映射由分类保存时统一落库。
// 命名走「{中文名}_副本」递增（copyFieldUniqueName），不再拼 tags_copy 英文后缀；
// 复制后自动聚焦新字段名输入框，方便立即改名。
async function duplicateField(fd: any) {
  const taken = fieldsEditable.value.flatMap((f: any) => [f.name, f.label || ''])
  const nm = copyFieldUniqueName(fieldLabelOf(fd), taken)
  fieldsEditable.value.push({
    name: nm,
    enabled: !!fd.enabled,
    isDefault: !!fd.isDefault,
    dataType: fd.dataType || 'text',
    width: Number(fd.width) || 0,
    // 复制出的字段必须留在同一分组：否则在市电账单里复制「峰电量」会掉进主表，
    // 用户切回该分组就找不到它了。
    group: (fd as any).group,
    groupLabel: (fd as any).groupLabel,
    // 子列一并复制，否则复制出的 items 字段会是个空壳
    subFields: Array.isArray(fd.subFields) ? fd.subFields.map((s: EditableSubField) => ({ ...s })) : undefined,
  })
  MessagePlugin.success('已复制字段')
  // 新行刚挂载，等一拍再聚焦（TInput 暴露 focus()）
  await nextTick()
  const el: any = fieldInputRefs.get(nm)
  if (el && typeof el.focus === 'function') el.focus()
  else (document?.querySelector?.(`.field-config-row .fc-name input`) as HTMLInputElement | null)?.focus?.()
}
function addField() {
  const name = newFieldName.value.trim()
  if (!name) { MessagePlugin.warning('请输入字段名'); return }
  if (fieldsEditable.value.some((f) => f.name === name)) { MessagePlugin.warning('字段已存在'); return }
  // 新增字段继承当前选中的分组：在「峰电量」分组里点新增，新字段就该属于峰电量，
  // 否则它会掉进主表、且切到任何分组都看不到。
  const g = activeSubGroup.value
  fieldsEditable.value.push({
    name,
    enabled: true,
    isDefault: false,
    dataType: 'text',
    width: 0,
    ...(g ? { group: g, groupLabel: subGroupLabelOf(g) } : {}),
  } as any)
  newFieldName.value = ''
}
function removeField(fd: any) {
  fieldsEditable.value = fieldsEditable.value.filter((f) => f !== fd)
}

async function commitEdit() {
  const item = groupItems.value.find((it: any) => it.id === editingId.value)
  if (!item) { editingId.value = ''; return }
  const name = editForm.name.trim()
  if (!name) { MessagePlugin.warning(namePlaceholderOf(props.scope)); return }
  const subs = toSubsPayload(fieldsEditable.value)
  // 本次保存是否由「智能生成字段」发起（发起时保存后询问是否去配置规则）
  const aiContext = pendingAiSample.value
  pendingAiSample.value = ''
  saving.value = true
  try {
    if (item.builtin && item.id.startsWith('builtin-')) {
      // 内置类型尚未入库：字段配置后自动入库，带 builtin_key 锚点（改名后仍识别为内置）
      const builtinKey = item.id.slice('builtin-'.length)
      const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', builtin_key: builtinKey, name, subs })
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
      // AI 保存路径会给出「字段已保存，已为您切换至提取规则」，这里不再叠加第二条
      if (!aiContext) MessagePlugin.success('已保存')
      notifyCategoriesChanged()
    }
    editingId.value = ''
    // 字段已落库 → 直接切到「提取规则」，只留一条 Toast（不再弹确认框、不再叠加提示）
    if (aiContext) {
      goRulesAfterAiSave(name, aiContext)
      MessagePlugin.success('字段已保存，已为您切换至提取规则')
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

// ---- 字段生成（内嵌面板，无模态遮罩）----
// aiPanelOpen 与编辑行绑定：换一行编辑时先收起，避免"看着 A 分类的字段、
// 其实面板属于 B 分类"的错位。
const aiPanelOpen = ref(false)
const editingCertName = ref('')
const pendingAiSample = ref('')

/** 展开某分类的编辑面板并打开内嵌 AI 生成区（分类行 ⋮ 菜单 / 新建分类后调用）。 */
function openAiPanel(item: any) {
  if (!item) return
  if (editingId.value !== item.id) toggleEdit(item)
  editingCertName.value = editForm.name.trim()
  aiPanelOpen.value = true
  // 等 v-if 渲染出编辑区再滚动，否则滚不到目标
  nextTick(() => {
    document.querySelector('.ai-field-panel')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
}

/** 「发票类型」右侧的 AI 图标按钮：与 ⋮ 菜单里的「字段生成」等价，再点收起。 */
function toggleAiPanel() {
  editingCertName.value = editForm.name.trim()
  aiPanelOpen.value = !aiPanelOpen.value
  if (aiPanelOpen.value) {
    nextTick(() => {
      document.querySelector('.ai-field-panel')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
    })
  }
}

/**
 * 「保存字段」= 并入编辑态 + 立刻调后端保存接口落库。
 *
 * 为什么不再让用户点第二次保存：并入只是内存动作，用户看到按钮变成"已并入 N 项"
 * 却还要滚到下面再点「保存」才落库，中途切走就白配了。这里直接衔接 commitEdit。
 * Toast 也只留一条（保存结果），不再叠加"已自动创建父字段 + 已并入 N 项"两条。
 */
function onAiFieldApply(payload: { standard: any[]; subFields: any[]; custom: any[]; sampleText: string }) {
  const merged = mergeAiFieldSuggestions(
    fieldsEditable.value,
    payload.standard,
    payload.subFields,
    payload.custom,
    payload.subFields.map((s: any) => s.key),
    // 扩展字段用后端分配的只读 key 作为身份（中文名只是可改的 label）
    payload.custom.map((c: any) => String(c.key || '').trim() || String(c.name || '').trim()),
  )
  fieldsEditable.value = merged.fields
  resetSubGroupSelection()
  pendingAiSample.value = payload.sampleText || ''
  aiPanelOpen.value = false
  // 明细父字段由 mergeAiFieldSuggestions 静默补建，不再弹提示打断流程
  if (merged.skipped.length) {
    MessagePlugin.warning(`${merged.skipped.length} 项未并入：${merged.skipped[0]?.reason || ''}`)
  }
  if (!merged.addedCount) {
    if (!merged.skipped.length) MessagePlugin.warning('没有可并入的新项（可能都已存在）')
    return
  }
  // 并入成功 → 直接落库，用户无需再点一次保存
  void commitEdit()
}

// 保存字段后**直接跳到「提取规则」**（不再弹确认框）。
//
// 用户口径：字段已落库，下一步必然是配规则，再弹一次"要不要去"纯属多一次点击。
// 样票文本先写缓存，规则面板挂载时即可带上它做正则唯一命中校验。
function goRulesAfterAiSave(certType: string, sampleText: string) {
  if (!certType) return
  if (sampleText) cacheSampleText(certType, sampleText)
  emit('ai-guide', { certType, sampleText, autoSuggest: !!sampleText })
}

const emit = defineEmits<{
  (e: 'ai-guide', payload: { certType: string; sampleText: string; autoSuggest: boolean }): void
}>()

// ---- 启用/禁用 ----
async function toggleEnabled(item: any, v: boolean) {
  if (item.builtin && item.id.startsWith('builtin-')) {
    // 内置未入库：启用即入库
    if (v) {
      try {
        const builtinKey = String(item.id).slice('builtin-'.length)
      const res = await createFleetCategory({ scope: group.value.scope, group_id: group.value.groupId || '', builtin_key: builtinKey, name: item.name, subs: [] })
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

// ---- 分类行操作：竖向三点菜单（字段生成 / 复制 / 删除，图标+文字）----
const itemMenuOptions = (item: any) => {
  const opts: any[] = [
    { content: '字段生成', value: 'ai', prefixIcon: () => h(AiSparkIcon, { size: '14px' }) },
    { content: '复制', value: 'copy', prefixIcon: () => h(CopyIcon, { size: '14px' }) },
  ]
  if (!item.builtin) opts.push({ content: '删除', value: 'delete', theme: 'error', prefixIcon: () => h(DeleteIcon, { size: '14px' }) })
  return opts
}
function onItemMenu(item: any, value: string) {
  if (value === 'ai') openAiPanel(item)
  else if (value === 'copy') void duplicateItem(item)
  else if (value === 'delete') delItemPop.value = item
}
const delItemPop = ref<any>(null)
const onItemPopRemove = async () => {
  const item = delItemPop.value
  delItemPop.value = null
  if (item) await removeItem(item)
}

// ---- 复制分类：字段（subs）与提取规则完全复制，命名「{原名}的副本{n}」 ----
async function duplicateItem(item: any) {
  const scope = group.value.scope
  const gid = group.value.groupId || ''
  const base = item.name
  const names = new Set((categories.value[scope] || []).map((c: any) => c.name))
  let n = 1
  let name = `${base}的副本${n}`
  while (names.has(name)) { n += 1; name = `${base}的副本${n}` }
  // 字段全量复制：已入库分类取 subs 原样拷贝（含明细 sub_fields）；内置未入库回退内置底稿
  let subs: any[] = []
  if (Array.isArray(item.subs) && item.subs.length) {
    subs = toSubsPayload(fromSubsPayload(item.subs))
  } else if (item.builtin) {
    const def = (BUILTIN as any)[scope] || []
    const b = def.find((x: any) => x.name === base)
    if (b) {
      subs = subsPayloadFromNames([...b.base, ...b.detail]).map((s) => ({ ...s, is_default: b.base.includes(s.name) }))
    }
  }
  saving.value = true
  try {
    const res = await createFleetCategory({ scope, group_id: gid, name, subs, enabled: true })
    const created = res.data || res
    if (!created?.id) { MessagePlugin.error('复制失败：返回数据异常'); return }
    const record = { ...created, group_id: created.group_id || gid }
    categories.value[scope] = [...(categories.value[scope] || []), record]
    // 完全复制：提取规则一并拷贝（源分类无规则时静默跳过）
    if (props.kbId) {
      try {
        const src: any = await getExtractConfig(props.kbId, scope, base)
        const cfg = src?.data
        if (cfg && Array.isArray(cfg.fields)) {
          await saveExtractConfig(props.kbId, {
            scope,
            cert_type: name,
            fields: cfg.fields.map((f: any) => ({ ...f })),
            advanced_enabled: !!cfg.advanced_enabled,
            prompt_template: cfg.prompt_template || '',
          })
        }
      } catch { /* 源分类无提取规则，跳过规则复制 */ }
    }
    MessagePlugin.success(`已复制为「${name}」`)
    notifyCategoriesChanged()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '复制失败')
  } finally {
    saving.value = false
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
    gap: var(--wk-field-grid-gap);
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

  /* 发票类型输入框 + 右侧 AI 图标按钮同行 */
  .form-item-inline {
    display: flex;
    align-items: center;
    gap: 8px;

    :deep(.t-input__wrap) { flex: 1; min-width: 0; }
  }

  /* AI 图标按钮：默认无强调，展开时主色高亮（激活态） */
  .ai-toggle-btn {
    flex: none;
    color: var(--td-text-color-secondary);

    &:hover {
      color: var(--td-brand-color);
      border-color: var(--td-brand-color);
    }

    &--on {
      color: var(--td-brand-color);
      border-color: var(--td-brand-color);
      background: var(--td-brand-color-1);
    }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    margin-top: 14px;
    .spacer { flex: 1; }
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
  gap: var(--wk-field-grid-gap);
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

  /* 业务分组胶囊条（仅带 group 的分类渲染，如市电账单的 21 个费用分项） */
  .fc-group-tabs {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    margin: 2px 0 10px;
    padding: 8px 10px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 6px;
  }

  .fc-group-tab {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 10px;
    font-size: 12px;
    line-height: 18px;
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border: 1px solid transparent;
    border-radius: 12px;
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;

    &:hover {
      color: var(--td-brand-color);
      border-color: var(--td-brand-color-light);
    }

    &.active {
      color: #fff;
      background: var(--td-brand-color);
      border-color: var(--td-brand-color);
      font-weight: 500;
    }

    .fc-group-tab__count {
      font-size: 11px;
      opacity: 0.75;
    }
  }

  /* 字段名 + 细项展开按钮（费用组成 / 分时时段） */
  .fc-name-wrap {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;

    .fc-name { flex: 1 1 auto; min-width: 0; }
  }

  .fc-child-toggle {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    flex: 0 0 auto;
    padding: 0 6px;
    height: 24px;
    font-size: 12px;
    line-height: 22px;
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    border: 1px solid transparent;
    border-radius: 4px;
    cursor: pointer;
    white-space: nowrap;
    transition: all 0.15s ease;

    &:hover { border-color: var(--td-brand-color); }
    &.open { background: var(--td-brand-color); color: #fff; }
  }

  .fc-child-type {
    display: inline-flex;
    align-items: center;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
  }

  .fc-child-move {
    font-size: 14px;
    line-height: 1;
    padding: 0 2px;
    min-width: 24px;
    color: var(--td-text-color-secondary);

    &:not(.t-is-disabled):hover { color: var(--td-brand-color); }
  }

  /* 细项行：紧凑 4 列，宽度自适应抽屉，不再横向溢出 */
  .field-config-row.child-row,
  .field-config-row.child-row.has-width {
    grid-template-columns: 24px minmax(0, 1fr) 48px 52px 36px;
    min-width: 0;
  }

  .fc-child-ops {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    justify-content: flex-end;
  }

  .fc-child-empty {
    padding: 8px 12px 8px 28px;
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 4px;
  }

  .field-config-list {
    min-width: 510px;
    overflow-x: auto;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .field-colhead {
    display: grid;
    grid-template-columns: 24px 160px 90px 64px 64px 92px;
    align-items: center;
    gap: 8px;
    padding: 0 8px;
    min-width: 612px;
    box-sizing: border-box;
    font-size: var(--td-font-size-body-small);
    color: var(--td-text-color-secondary);

    &.has-width {
      grid-template-columns: 24px 160px 90px 70px 64px 64px 92px;
    }
    > span { white-space: nowrap; }
    .fc-h-width { text-align: center; }
  }
  .fc-h-switch, .fc-h-op, .fc-h-type { text-align: center; }
  .fc-h-name { text-align: left; }

  .field-config-row {
    display: grid;
    grid-template-columns: 24px 160px 90px 64px 64px 92px;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    /* 字段名列 160px：既要放字段名输入，又要放「细项 N」展开按钮，
       110px 时输入框会被挤成「费...」（真机截图为证）。 */
    min-width: 612px;
    box-sizing: border-box;

    &.has-width {
      grid-template-columns: 24px 160px 90px 70px 64px 64px 92px;
    }
    .fc-width {
      width: 100%;
      min-width: 0;
      justify-self: center;
    }
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
    .fc-width {
      width: 100%;
      min-width: 0;
    }

    > *:nth-child(4),
    > *:nth-child(5) { justify-self: center; }
    > *:nth-child(6) { justify-self: center; }
    &.has-width > *:nth-child(4) { justify-self: stretch; }
    &.has-width > *:nth-child(7) { justify-self: center; }

  .fc-used {
      display: inline-flex;
      align-items: center;
      color: var(--td-text-color-placeholder);
      cursor: not-allowed;
    }
  .action-delete-btn {
      color: var(--td-text-color-primary);
      cursor: pointer;
      transition: color 0.2s ease;
      display: flex;
      justify-content: center;
      align-items: center;
    }
  .action-delete-btn:hover {
      color: var(--td-error-color);
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

  /* items 字段名即展开开关：去掉按钮外观，只保留 caret + 文案 + 子项数 */
  .fc-name-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    height: 32px;
    padding: 0 4px;
    border: none;
    border-radius: var(--td-radius-default);
    background: transparent;
    font-size: 13px;
    color: var(--td-text-color-primary);
    text-align: left;
    cursor: pointer;
    transition: background 0.15s;

    &:hover { background: var(--td-bg-color-secondarycontainer); }

    .fc-name-text {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .fc-sub-count {
      flex: none;
      font-size: 11px;
      line-height: 16px;
      padding: 0 6px;
      border-radius: 8px;
      color: var(--td-text-color-secondary);
      background: var(--td-bg-color-component);
      font-variant-numeric: tabular-nums;
    }
  }

  /* 明细子项内联展开区：紧贴 items 字段行，子行复用 .field-config-row 的网格/行高/圆角 */
  .sub-inline {
    display: flex;
    flex-direction: column;
    gap: 6px;
    /* 抵消 .field-config-list 的 6px 行间距，让子项区紧贴 items 字段行 */
    margin: -6px 0 0;
  }

  .sub-row {
    /* 复用主行尺寸，只换底色 + 左侧主色细线表示层级归属（不用 border-left，避免位移 1px） */
    background: var(--td-bg-color-secondarycontainer);
    box-shadow: inset 2px 0 0 var(--td-brand-color-3);

    &:hover { background: var(--td-bg-color-component); }
  }

  /* 子行的首列显示序号而非拖拽把手（子项排序不参与主字段排序） */
  .sub-drag {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 11px;
    color: var(--td-text-color-placeholder);
    cursor: default;
    font-variant-numeric: tabular-nums;
  }

  /* 「默认表头」列在子项上无意义，占位保持网格列对齐 */
  .sub-blank { display: block; }

  .sub-inline-add {
    padding-left: 24px;
    margin-top: 2px;
  }
}
</style>