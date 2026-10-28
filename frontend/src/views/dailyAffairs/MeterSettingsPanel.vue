<template>
  <!--
    表计设置面板（电表 / 水表 / 气表）。

    来源：原 UtilityMeterTab 工具栏「表计配置」按钮打开的抽屉。
    为什么独立成组件：工具栏铁律只允许 3 个按钮（字段 / 设置 / 新增记录），
    表计档案与单价规则改为统一设置抽屉的「表计设置」Tab 内嵌渲染。
    组件只依赖 category，自身负责表计与用途的读写，变更后 emit('changed')
    让外层列表重新拉一次表计（记录表单的「表计」下拉要同步）。
  -->
  <div class="meter-settings-body">
    <div class="settings-head">
      <t-button theme="primary" size="small" @click="openMeterForm(null)">
        <template #icon><t-icon name="add" /></template>
        新增{{ meterLabel }}
      </t-button>
      <t-button variant="outline" size="small" @click="kindManageVisible = true">
        <template #icon><t-icon name="setting" /></template>
        用途管理
      </t-button>
    </div>

    <div v-if="meters.length" class="meter-search">
      <t-input v-model="panelSearch" placeholder="搜索别名 / 表号" clearable>
        <template #prefix-icon><t-icon name="search" size="14px" /></template>
      </t-input>
    </div>

    <div v-if="!meters.length && !meterFormVisible" class="meter-empty">
      <t-icon name="setting" size="40px" class="meter-empty-icon" />
      <span class="meter-empty-text">暂无{{ meterLabel }}，点击新增{{ meterLabel }}开始配置</span>
    </div>

    <div class="meter-table-wrap">
      <!-- 新增表计：表单展开在顶部 -->
      <div v-if="meterFormVisible && !meterForm.id" class="meter-form">
        <div class="meter-form-title">新增{{ meterLabel }}</div>
        <div class="form-grid">
          <div class="form-item">
            <label>别名 <span class="required">*</span></label>
            <t-input ref="meterAliasInput" v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
          </div>
          <div class="form-item">
            <label>表号</label>
            <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
          </div>
          <div class="form-item">
            <label>类型</label>
            <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" placeholder="选择类型" />
          </div>
          <div class="form-item">
            <label>用途</label>
            <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
          </div>
          <div class="form-item">
            <label>归属单位</label>
            <t-input v-model="meterForm.owner_unit" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>倍率</label>
            <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
          </div>
          <div class="form-item">
            <label>默认单价</label>
            <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
          </div>
          <div class="form-item">
            <label>使用单位</label>
            <t-input v-model="meterForm.use_unit" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>管理人员</label>
            <t-input v-model="meterForm.manager" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>联系方式</label>
            <t-input v-model="meterForm.contact" placeholder="选填" />
          </div>
          <div class="form-item">
            <label>抄表方式</label>
            <t-radio-group v-model="meterForm.meter_mode">
              <t-radio-button value="auto">自动抄表</t-radio-button>
              <t-radio-button value="manual">手动抄表</t-radio-button>
            </t-radio-group>
          </div>
          <div class="form-item">
            <label>安装日期</label>
            <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
          </div>
          <div class="form-item">
            <label>启用</label>
            <t-switch v-model="meterForm.enabled" />
          </div>
        </div>
        <div class="form-item form-item--full">
          <label>备注</label>
          <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
        </div>
        <div class="meter-form-actions">
          <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
          <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
        </div>
      </div>

      <!-- 分组标签：宿舍 / 工商业 / 自定义用途（仅导航，表格单表渲染保证拖拽稳定） -->
      <t-tabs v-if="meterGroups.length" v-model="activeMeterGroup" class="meter-settings-tabs">
        <t-tab-panel v-for="g in meterGroups" :key="g.key" :value="g.key" :label="g.label + ' ' + g.items.length" />
      </t-tabs>

      <div v-if="activeGroupItems.length" :ref="setMeterSortableRef" class="meter-table">
        <div class="meter-table-head">
          <span class="meter-drag-th"><t-icon name="move" size="14px" /></span><span>别名</span><span>表号</span><span>类型</span><span>倍率</span><span>单价</span><span>归属单位</span><span>状态</span><span>操作</span>
        </div>
        <template v-for="m in activeGroupItems" :key="m.id">
          <div class="meter-table-row" :data-id="m.id" :class="{ editing: meterFormVisible && meterForm.id === m.id }" @click="toggleMeterEdit(m)">
            <span class="meter-drag-handle" title="拖动排序" @click.stop><t-icon name="move" size="14px" /></span>
            <span class="mtr-alias">{{ m.alias }}</span>
            <span class="mtr-mono">{{ m.meter_no || '—' }}</span>
            <span class="mtr-type">{{ meterTypeLabel(m.meter_type) }} · {{ meterKindLabel(m.meter_kind) }}</span>
            <span class="mtr-mono">{{ fmtNum(m.rate) }}</span>
            <span class="mtr-mono">{{ fmtUnitPrice(m.default_unit_price) }} 元/{{ unitLabel }}</span>
            <span class="mtr-owner">{{ m.owner_unit || '—' }}</span>
            <span class="mtr-switch" @click.stop>
              <t-switch :model-value="!!m.enabled" size="small" @update:model-value="(v: any) => toggleEnabled(m, !!v)" />
            </span>
            <span class="meter-row-actions" @click.stop>
              <t-popconfirm theme="warning" :content="`确定删除${meterLabel}「${m.alias}」吗？`"
                :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
                @confirm="deleteMeter(m)">
                <t-button variant="text" size="small" class="biz-del-btn" @click.stop>
                  <template #icon><t-icon name="delete" size="16px" /></template>
                </t-button>
              </t-popconfirm>
            </span>
          </div>
          <!-- 编辑表计：点击行展开，再点折叠 -->
          <div v-if="meterFormVisible && meterForm.id === m.id" class="meter-form meter-form--inline">
            <div class="meter-form-title">编辑{{ meterLabel }}</div>
            <div class="form-grid">
              <div class="form-item">
                <label>别名 <span class="required">*</span></label>
                <t-input v-model="meterForm.alias" :placeholder="'如：1号楼' + meterLabel" />
              </div>
              <div class="form-item">
                <label>表号</label>
                <t-input v-model="meterForm.meter_no" placeholder="自动编号，可手改" />
              </div>
              <div class="form-item">
                <label>类型</label>
                <t-select v-model="meterForm.meter_type" :options="meterTypeOptions" placeholder="选择类型" />
              </div>
              <div class="form-item">
                <label>用途</label>
                <t-select v-model="meterForm.meter_kind" :options="meterKindOptions" />
              </div>
              <div class="form-item">
                <label>归属单位</label>
                <t-input v-model="meterForm.owner_unit" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>倍率</label>
                <t-input v-model.number="meterForm.rate" type="number" placeholder="默认 1" />
              </div>
              <div class="form-item">
                <label>默认单价</label>
                <t-input v-model.number="meterForm.default_unit_price" type="number" :placeholder="`元/${unitLabel}`" />
              </div>
              <div class="form-item">
                <label>使用单位</label>
                <t-input v-model="meterForm.use_unit" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>管理人员</label>
                <t-input v-model="meterForm.manager" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>联系方式</label>
                <t-input v-model="meterForm.contact" placeholder="选填" />
              </div>
              <div class="form-item">
                <label>抄表方式</label>
                <t-radio-group v-model="meterForm.meter_mode">
                  <t-radio-button value="auto">自动抄表</t-radio-button>
                  <t-radio-button value="manual">手动抄表</t-radio-button>
                </t-radio-group>
              </div>
              <div class="form-item">
                <label>安装日期</label>
                <t-date-picker v-model="meterForm.install_date" format="YYYY-MM-DD" value-type="YYYY-MM-DD" placeholder="选填" clearable />
              </div>
              <div class="form-item">
                <label>启用</label>
                <t-switch v-model="meterForm.enabled" />
              </div>
            </div>
            <div class="form-item form-item--full">
              <label>备注</label>
              <t-textarea v-model="meterForm.remark" :maxlength="500" placeholder="选填" />
            </div>
            <div class="meter-form-actions">
              <t-button variant="outline" size="small" @click="meterFormVisible = false">取消</t-button>
              <t-button theme="primary" size="small" :loading="savingMeter" @click="saveMeter">保存</t-button>
            </div>
          </div>
        </template>
      </div>
      <div v-if="!activeGroupItems.length" class="meter-empty">
        <t-icon name="setting" size="28px" class="meter-empty-icon" />
        <span class="meter-empty-text">暂无{{ meterLabel }}</span>
      </div>
    </div>

    <!-- 用途管理:内置 宿舍/工商业 可改名;公租房及其它自定义可删改增 -->
    <t-dialog v-model:visible="kindManageVisible" :header="'用途管理(' + meterLabel + ')'" :footer="false" width="480px">
      <div class="kind-manage-body">
        <p class="kind-manage-tip">内置用途(宿舍/工商业)驱动分摊计算,可改名不可删除;公租房及其它自定义用途可删除、可修改、可新增,仅分组展示不参与租户核算计算。</p>
        <div v-for="k in meterKinds" :key="k.value" class="kind-manage-row">
          <t-input v-model="k.label" size="small" @enter="() => {}">
            <template #prefix-icon><span class="kind-value-tag">{{ k.value }}</span></template>
          </t-input>
          <t-popconfirm v-if="!(k as any).builtin" theme="warning" :content="`确定删除用途「${k.label}」吗？`"
            :confirm-btn="{ content: '删除', theme: 'danger' }" :cancel-btn="{ content: '取消' }" placement="top"
            @confirm="removeCustomKind(k.value)">
            <t-button variant="text" size="small" class="biz-del-btn" @click.stop>
              <template #icon><t-icon name="delete" size="16px" /></template>
            </t-button>
          </t-popconfirm>
          <span v-else class="kind-builtin-tag">内置</span>
        </div>
        <t-button variant="outline" size="small" block @click="addCustomKind">
          <template #icon><t-icon name="add" /></template>
          新增用途
        </t-button>
        <div class="kind-manage-actions">
          <t-button variant="outline" size="small" @click="kindManageVisible = false">关闭</t-button>
          <t-button theme="primary" size="small" :loading="savingKinds" @click="saveKinds">保存</t-button>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'
import Sortable from 'sortablejs'
import { MessagePlugin } from 'tdesign-vue-next'
import {
  listUtilityMeters,
  createUtilityMeter,
  updateUtilityMeter,
  deleteUtilityMeter,
  sortUtilityMeters,
  listUtilityKinds,
  createUtilityKind,
  updateUtilityKind,
  deleteUtilityKind,
} from '@/api/knowledge-base'

type MeterCategory = 'electricity' | 'water' | 'gas'
const props = defineProps<{ category: MeterCategory }>()
const emit = defineEmits<{ (e: 'changed'): void }>()

const meterLabel = computed(() => (props.category === 'water' ? '水表' : props.category === 'electricity' ? '电表' : '气表'))
const unitLabel = computed(() => (props.category === 'water' ? '吨' : props.category === 'electricity' ? '千瓦时' : 'm³'))

const fmtNum = (v: any) => {
  const n = Number(v) || 0
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 100) / 100)
}
const fmtUnitPrice = (v: any) => {
  const n = Number(v)
  if (v === '' || v == null || Number.isNaN(n)) return ''
  return String(Math.round(n * 1000) / 1000)
}

const meters = ref<any[]>([])
const panelSearch = ref('')
const savingMeter = ref(false)
const meterFormVisible = ref(false)
const kindManageVisible = ref(false)
const meterForm = ref<any>({})

// 计量层级按类别: 电表 普通/分时; 水表 总表/分表/消防; 气表 普通
const meterTypeOptions = computed(() => {
  if (props.category === 'electricity') return [
    { label: '普通', value: 'normal' },
    { label: '分时', value: 'time' },
  ]
  if (props.category === 'water') return [
    { label: '总表', value: 'total' },
    { label: '分表', value: 'sub' },
    { label: '消防', value: 'fire' },
  ]
  return [{ label: '普通', value: 'normal' }]
})
const meterTypeLabel = (t: string) => meterTypeOptions.value.find(o => o.value === t)?.label || '普通'
const meterAliasInput = ref()

// 用途体系:内置 宿舍/工商业(锁定 value 驱动租户核算计算,label 可改名) + 用户自定义
const KIND_BUILTIN = reactive([
  { value: 'dorm', label: '宿舍', builtin: true },
  { value: 'production', label: '工商业', builtin: true },
])
type KindItem = { id?: string; value: string; label: string }
const customKinds = ref<KindItem[]>([])
const meterKinds = computed(() => [...KIND_BUILTIN, ...customKinds.value])
const meterKindOptions = computed(() => meterKinds.value.map(k => ({ label: k.label, value: k.value })))
const meterKindLabel = (k: string) => meterKinds.value.find(o => o.value === k)?.label || '未分类'

const filteredMeters = computed(() => {
  const kw = panelSearch.value.trim().toLowerCase()
  if (!kw) return meters.value
  return meters.value.filter((m: any) =>
    (m.alias || '').toLowerCase().includes(kw) || (m.meter_no || '').toLowerCase().includes(kw))
})
const meterGroups = computed(() => {
  const groups = meterKinds.value.map(k => ({ key: k.value, label: k.label, items: [] as any[] }))
  for (const m of filteredMeters.value) {
    const g = groups.find(g => g.key === m.meter_kind)
    ;(g || groups[groups.length - 1]).items.push(m)
  }
  return groups
})
const activeMeterGroup = ref<string>('dorm')
const activeGroupItems = computed(() => meterGroups.value.find(g => g.key === activeMeterGroup.value)?.items || [])
watch(meterGroups, (gs) => {
  if (!gs.find(g => g.key === activeMeterGroup.value)) {
    activeMeterGroup.value = (gs[0]?.key as any) || 'dorm'
  }
})

const sortMetersLocal = () => {
  meters.value = [...meters.value].sort((a: any, b: any) =>
    (Number(a.sort_order) || 0) - (Number(b.sort_order) || 0) ||
    (a.created_at || '').localeCompare(b.created_at || ''))
}
const loadMetersOnly = async () => {
  try {
    const res: any = await listUtilityMeters({ category: props.category })
    const mets = res?.data || res || []
    meters.value = Array.isArray(mets) ? mets : []
    sortMetersLocal()
  } catch { /* ignore */ }
}

const loadKinds = async () => {
  try {
    const res: any = await listUtilityKinds(props.category)
    const list = res?.data || []
    if (Array.isArray(list) && list.length) {
      customKinds.value = list
        .filter((k: any) => !KIND_BUILTIN.some(b => b.value === k.value))
        .map((k: any) => ({ id: k.id, value: k.value, label: k.label }))
      for (const k of list) {
        const b = KIND_BUILTIN.find(x => x.value === k.value)
        if (b && k.label) b.label = k.label
      }
    }
  } catch (e) {
    console.error('load kinds failed', e)
  }
}

const addCustomKind = async () => {
  const n = customKinds.value.length + 1
  const value = `custom${n}`
  const label = `自定义${n}`
  try {
    const res: any = await createUtilityKind(props.category, { value, label })
    const item = res?.data || res
    if (item?.id) customKinds.value.push({ id: item.id, value: item.value, label: item.label })
    else customKinds.value.push({ value, label })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '新增用途失败')
  }
}
const removeCustomKind = async (value: string) => {
  // 引用检查:有表计正在使用该用途时禁止删除
  const used = meters.value.filter(m => String(m.meter_kind) === value)
  if (used.length) {
    MessagePlugin.warning(`该用途已被 ${used.length} 个表计引用,请先删除或修改这些表计的用途后再删除`)
    return
  }
  const item = customKinds.value.find(k => k.value === value)
  if (!item) return
  if (item.id) {
    try {
      await deleteUtilityKind(item.id)
    } catch (e: any) {
      MessagePlugin.error(e?.message || '删除失败')
      return
    }
  }
  customKinds.value = customKinds.value.filter(k => k.value !== value)
}
const savingKinds = ref(false)
// 保存用途配置：内置用途 upsert 到后端（改名持久化），自定义用途按 id 更新
const saveKinds = async () => {
  savingKinds.value = true
  try {
    for (const k of meterKinds.value) {
      const label = (k.label || '').trim()
      if (!label) {
        MessagePlugin.warning('用途名称不能为空')
        return
      }
      if ((k as any).builtin) {
        await createUtilityKind(props.category, { value: k.value, label })
      } else if ((k as any).id) {
        if (label !== k.label) k.label = label
        await updateUtilityKind((k as any).id, { label })
      }
    }
    MessagePlugin.success('用途配置已保存')
    kindManageVisible.value = false
    await loadKinds()
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingKinds.value = false
  }
}

const meterNoPrefix = () => (props.category === 'water' ? 'W' : props.category === 'electricity' ? 'E' : 'G')
const nextMeterNo = () => {
  let max = 0
  for (const m of meters.value) {
    const mm = String(m.meter_no || '').match(/^[A-Za-z]*(\d+)$/)
    if (mm) max = Math.max(max, parseInt(mm[1], 10))
  }
  return meterNoPrefix() + String(max + 1).padStart(3, '0')
}

const openMeterForm = (m: any) => {
  // 新增默认用途:水表(总表/分表/消防均属工商业)默认工商业;电表/气表默认居民
  const defaultKind = props.category === 'water' ? 'production' : 'dorm'
  // 新增默认类型:电表/气表默认普通;水表类型(总表/分表/消防)由用户按需选择
  const defaultType = props.category === 'water' ? '' : 'normal'
  meterForm.value = m ? {
    id: m.id,
    alias: m.alias || '',
    meter_no: m.meter_no || '',
    // 水表历史脏数据 normal 不在合法类型内,编辑时置空引导重选
    meter_type: props.category === 'water' && !['total', 'sub', 'fire'].includes(m.meter_type)
      ? '' : (m.meter_type || ''),
    meter_kind: m.meter_kind || defaultKind,
    owner_unit: m.owner_unit || '',
    rate: Number(m.rate) > 0 ? m.rate : 1,
    default_unit_price: Number(m.default_unit_price) || 0,
    use_unit: m.use_unit || '',
    manager: m.manager || '',
    contact: m.contact || '',
    meter_mode: m.meter_mode || 'manual',
    install_date: m.install_date || '',
    remark: m.remark || '',
    enabled: m.enabled !== false,
  } : {
    id: '', alias: '', meter_no: nextMeterNo(), meter_type: defaultType,
    meter_kind: defaultKind, owner_unit: '', rate: 1, default_unit_price: 0,
    use_unit: '', manager: '', contact: '', meter_mode: 'manual',
    install_date: '', remark: '', enabled: true,
  }
  meterFormVisible.value = true
  nextTick(() => { if (!m) (meterAliasInput.value as any)?.focus?.() })
}
const toggleMeterEdit = (m: any) => {
  if (meterFormVisible.value && meterForm.value.id === m.id) {
    meterFormVisible.value = false
    return
  }
  openMeterForm(m)
}

const saveMeter = async () => {
  if (!meterForm.value.alias?.trim()) {
    MessagePlugin.warning('请填写别名')
    return
  }
  savingMeter.value = true
  try {
    const payload = {
      category: props.category,
      alias: meterForm.value.alias.trim(),
      meter_no: meterForm.value.meter_no || '',
      meter_type: meterForm.value.meter_type || '',
      meter_kind: meterForm.value.meter_kind || 'dorm',
      owner_unit: meterForm.value.owner_unit || '',
      rate: Number(meterForm.value.rate) > 0 ? Number(meterForm.value.rate) : 1,
      default_unit_price: Number(meterForm.value.default_unit_price) || 0,
      use_unit: meterForm.value.use_unit || '',
      manager: meterForm.value.manager || '',
      contact: meterForm.value.contact || '',
      meter_mode: meterForm.value.meter_mode || 'manual',
      install_date: meterForm.value.install_date || '',
      remark: meterForm.value.remark || '',
      enabled: meterForm.value.enabled !== false,
    }
    const isEdit = !!meterForm.value.id
    if (isEdit) {
      await updateUtilityMeter(meterForm.value.id, payload)
      MessagePlugin.success('已保存')
    } else {
      await createUtilityMeter(payload)
      MessagePlugin.success('已新增')
    }
    await loadMetersOnly()
    emit('changed')
    if (isEdit) {
      meterFormVisible.value = false
    } else {
      // 连续新增：保持表单展开，表号自动递增，清空其余字段并聚焦别名
      meterForm.value = {
        id: '',
        alias: '',
        meter_no: nextMeterNo(),
        meter_type: props.category === 'water' ? '' : 'normal',
        meter_kind: props.category === 'water' ? 'production' : 'dorm',
        owner_unit: '',
        rate: '',
        default_unit_price: '',
        use_unit: '',
        manager: '',
        contact: '',
        meter_mode: 'manual',
        install_date: '',
        remark: '',
        enabled: true,
      }
      meterFormVisible.value = true
      nextTick(() => { (meterAliasInput.value as any)?.focus?.() })
    }
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMeter.value = false
  }
}

const toggleEnabled = async (m: any, v: any) => {
  try {
    await updateUtilityMeter(m.id, { ...m, enabled: !!v })
    m.enabled = !!v
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}

const deleteMeter = async (m: any) => {
  try {
    await deleteUtilityMeter(m.id)
    MessagePlugin.success('已删除')
    meters.value = meters.value.filter(x => x.id !== m.id)
    emit('changed')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

// ---- 表计拖动排序（sort_order 持久化） ----
const meterSortableEl = ref<HTMLElement | null>(null)
const setMeterSortableRef = (el: any) => {
  meterSortableEl.value = Array.isArray(el) ? (el[0] || null) : (el || null)
}
let sortableInst: any = null
const initMeterSortable = () => {
  const el = meterSortableEl.value
  if (!el) return
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
  try {
    sortableInst = Sortable.create(el, {
      draggable: '.meter-table-row',
      handle: '.meter-drag-handle',
      animation: 150,
      ghostClass: 'meter-row-ghost',
      onEnd: persistMeterSort,
    })
  } catch (e) {
    console.error('[Sortable] init failed:', e)
  }
}
const persistMeterSort = async () => {
  const el = meterSortableEl.value
  if (!el) return
  const ids = [...el.querySelectorAll('.meter-table-row')]
    .map(r => r.getAttribute('data-id')).filter(Boolean) as string[]
  if (!ids.length) return
  try {
    await sortUtilityMeters(props.category, ids)
    await loadMetersOnly()
    MessagePlugin.success('排序已保存')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '排序保存失败')
    await loadMetersOnly()
  }
}
watch([activeMeterGroup, () => activeGroupItems.value], async () => {
  await nextTick()
  setTimeout(initMeterSortable, 120)
})

onMounted(async () => {
  await loadMetersOnly()
  await loadKinds()
  await nextTick()
  setTimeout(initMeterSortable, 200)
})
onBeforeUnmount(() => {
  if (sortableInst) { sortableInst.destroy(); sortableInst = null }
})
</script>

<style lang="less" scoped>
.meter-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 28px 0;
  color: var(--td-text-color-placeholder);
  font-size: 13px;

  .meter-empty-icon { color: var(--td-text-color-placeholder); }
  .meter-empty-text { color: var(--td-text-color-secondary); }
}

.kind-manage-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.kind-manage-tip {
  color: var(--td-text-color-secondary);
  font-size: 12px;
  line-height: 1.6;
  margin: 0 0 4px;
}
.kind-manage-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 4px;
}
.kind-manage-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.kind-manage-row .t-input { flex: 1; }
.kind-value-tag {
  font-size: 11px;
  color: var(--td-text-color-placeholder);
  margin-right: 4px;
}
.kind-builtin-tag {
  width: 32px;
  text-align: center;
  font-size: 12px;
  color: var(--td-text-color-placeholder);
  flex-shrink: 0;
}

.meter-search { margin-bottom: 12px; }
.meter-settings-body { padding: 4px 0 24px; }

.settings-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.meter-table-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.meter-settings-tabs {
  margin-bottom: 10px;

  :deep(.t-tabs__header) { margin-bottom: 8px; }
  :deep(.t-tabs__nav-item) { font-size: 13px; }
  :deep(.t-tabs__nav-item-wrapper) { padding: 0; margin: 0; }
  :deep(.t-tabs__nav-item:not(:first-child) .t-tabs__nav-item-wrapper) { margin-left: 8px; }
  :deep(.t-tabs__content) { overflow: visible; }
}

.meter-table {
  border: 1px solid var(--td-component-stroke);
  border-radius: 9px;
  overflow: hidden;
  background: var(--td-bg-color-container);

  .meter-table-head,
  .meter-table-row {
    display: grid;
    grid-template-columns: 0.4fr 1.4fr 1fr 1.2fr 0.7fr 1.2fr 1fr 0.7fr 1.2fr;
    align-items: center;
    gap: 8px;
    padding: 7px 12px;
    font-size: 12px;
  }

  .meter-drag-th {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--td-text-color-placeholder);
  }

  .meter-table-head {
    color: var(--td-text-color-secondary);
    background: var(--td-bg-color-container);
    border-bottom: 1px solid var(--td-component-stroke);
  }

  .meter-table-row {
    border-bottom: 1px solid var(--td-component-stroke);
    color: var(--td-text-color-primary);
    cursor: pointer;
    transition: background 0.15s;

    .meter-drag-handle {
      display: flex;
      align-items: center;
      justify-content: center;
      color: var(--td-text-color-placeholder);
      cursor: grab;

      &:active { cursor: grabbing; }
    }

    &:hover { background: var(--td-bg-color-secondarycontainer); }
    &.editing { background: var(--td-brand-color-light); }
    &:last-child { border-bottom: none; }

    .mtr-alias {
      font-weight: 500;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mtr-mono {
      font-variant-numeric: tabular-nums;
      color: var(--td-text-color-primary);
    }
    .mtr-type {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      color: var(--td-text-color-secondary);
    }
    .mtr-owner {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
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

  .meter-row-ghost {
    opacity: 0.45;
    background: var(--td-brand-color-light);
  }
}

.meter-form {
  margin-top: 0;
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
    grid-column: auto;
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

    &--full { margin-top: 12px; }
  }

  .meter-form-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }
}
</style>

<style>
/* 删除图标统一口径（16px 灰→红悬停 + 浅红底 + .15s），全系统共用一份 */
@import './styles/business-delete-btn.css';
</style>
